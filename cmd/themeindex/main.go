// Command themeindex rebuilds the theme gallery's index from the themes
// listed on themes.gohugo.io. It keeps themes that build with plain Hugo
// against the starter site and show its home page text or latest post.
//
// Run it with `task theme-index`. It tests with the Hugo that Pageroom
// downloads for new sites. Theme templates run under Hugo's security policy,
// without credentials in the environment.
package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/myrheimb/pageroom/internal/fsutil"
	"github.com/myrheimb/pageroom/internal/hugo"
	"github.com/myrheimb/pageroom/internal/starter"
	"github.com/myrheimb/pageroom/internal/themes"
)

const themeList = "https://raw.githubusercontent.com/gohugoio/hugoThemesSiteBuilder/main/themes.txt"

var client = &http.Client{Timeout: 5 * time.Minute}

func main() {
	out := flag.String("out", "internal/themes/themes.json", "where to write the index")
	list := flag.String("list", themeList, "URL or file with theme module paths, one per line")
	hugoBin := flag.String("hugo", "", "the Hugo "+hugo.DefaultVersion+" to test-build themes with, downloaded if empty")
	jobs := flag.Int("jobs", runtime.NumCPU(), "themes to check at once")
	flag.Parse()

	if err := run(context.Background(), *out, *list, *hugoBin, *jobs); err != nil {
		fmt.Fprintln(os.Stderr, "themeindex:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, out, list, hugoBin string, jobs int) error {
	if hugoBin == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return err
		}
		manager := &hugo.Manager{CacheDir: filepath.Join(cache, "Pageroom", "hugo")}
		if hugoBin, err = manager.Download(ctx, hugo.DefaultVersion); err != nil {
			return err
		}
	}
	version, err := hugoVersion(hugoBin)
	if err != nil {
		return err
	}
	// The index records one Hugo version, and a theme that builds with an
	// older Hugo can fail with the one Pageroom gives new sites.
	if version != hugo.DefaultVersion {
		return fmt.Errorf("%s is Hugo %s, but Pageroom uses %s", hugoBin, version, hugo.DefaultVersion)
	}
	modules, err := readList(ctx, list)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Checking %d themes with Hugo %s…\n", len(modules), version)
	results := make([]*themes.Theme, len(modules))
	limit := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	for index, module := range modules {
		wg.Go(func() {
			limit <- struct{}{}
			defer func() { <-limit }()

			theme, err := check(ctx, hugoBin, version, module)
			if err != nil {
				fmt.Fprintf(os.Stderr, "skip %s: %v\n", module, err)
				return
			}
			results[index] = &theme
		})
	}
	wg.Wait()

	index := themes.Index{HugoVersion: version, Themes: []themes.Theme{}}
	for _, theme := range results {
		if theme != nil {
			index.Themes = append(index.Themes, *theme)
		}
	}
	nameDuplicates(index.Themes)
	sort.Slice(index.Themes, func(left, right int) bool { return index.Themes[left].Name < index.Themes[right].Name })

	printChanges(out, index.Themes, len(modules))
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(out, append(data, '\n'), 0o644)
}

var versionPattern = regexp.MustCompile(`v(\d+\.\d+\.\d+)`)

func hugoVersion(bin string) (string, error) {
	output, err := exec.Command(bin, "version").Output()
	if err != nil {
		return "", fmt.Errorf("running %s: %w", bin, err)
	}
	match := versionPattern.FindSubmatch(output)
	if match == nil || !bytes.Contains(output, []byte("extended")) {
		return "", fmt.Errorf("%s isn't the extended edition of Hugo: %s", bin, output)
	}
	return string(match[1]), nil
}

func readList(ctx context.Context, list string) ([]string, error) {
	var data []byte
	var err error
	if strings.HasPrefix(list, "https://") {
		data, err = download(ctx, list)
	} else {
		data, err = os.ReadFile(list)
	}
	if err != nil {
		return nil, err
	}

	var modules []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			modules = append(modules, line)
		}
	}
	return modules, scanner.Err()
}

func download(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, themes.MaxDownload+1))
	if len(data) > themes.MaxDownload {
		return nil, fmt.Errorf("the download is bigger than %d MB", themes.MaxDownload>>20)
	}
	return data, err
}

type themeMeta struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	License     string   `toml:"license"`
	Demosite    string   `toml:"demosite"`
	Tags        []string `toml:"tags"`
	MinVersion  any      `toml:"min_version"`
	Author      any      `toml:"author"`
	Authors     []struct {
		Name string `toml:"name"`
	} `toml:"authors"`
}

func check(ctx context.Context, hugoBin, version, module string) (themes.Theme, error) {
	repo, rest := splitModule(module)
	if _, err := archiveURL(repo, ""); err != nil {
		return themes.Theme{}, err
	}
	commit, err := headCommit(ctx, repo)
	if err != nil {
		return themes.Theme{}, err
	}
	archiveLink, _ := archiveURL(repo, commit)
	archive, err := download(ctx, archiveLink)
	if err != nil {
		return themes.Theme{}, err
	}

	scratch, err := os.MkdirTemp("", "themeindex-")
	if err != nil {
		return themes.Theme{}, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	subdir, err := findSubdir(archive, rest, filepath.Join(scratch, "repo"))
	if err != nil {
		return themes.Theme{}, err
	}
	site := filepath.Join(scratch, "site")
	if err := starter.Create(site, "Theme check", version); err != nil {
		return themes.Theme{}, err
	}
	name := path.Base(repo)
	themeDir := filepath.Join(site, "themes", name)
	sum, updated, err := themes.Extract(bytes.NewReader(archive), subdir, themeDir)
	if err != nil {
		return themes.Theme{}, err
	}

	data, err := os.ReadFile(filepath.Join(themeDir, "theme.toml"))
	if err != nil {
		return themes.Theme{}, errors.New("no theme.toml")
	}
	var meta themeMeta
	if err := toml.Unmarshal(data, &meta); err != nil {
		return themes.Theme{}, fmt.Errorf("unreadable theme.toml: %w", err)
	}
	if strings.TrimSpace(meta.License) == "" {
		return themes.Theme{}, errors.New("no license in theme.toml")
	}
	thumbnail := findImage(themeDir, "tn")
	if thumbnail == "" {
		return themes.Theme{}, errors.New("no thumbnail")
	}

	public := filepath.Join(scratch, "public")
	if err := hugo.Check(ctx, hugoBin, site, filepath.Join(scratch, "work"), name, public); err != nil {
		return themes.Theme{}, fmt.Errorf("build failed: %s", buildError(err))
	}
	// A theme can build and still show an empty home page when it expects other
	// sections or settings than a new site has. The home page's own text is
	// enough, since not every site is a blog.
	home, err := os.ReadFile(filepath.Join(public, "index.html"))
	if err != nil || !bytes.Contains(home, []byte("Welcome to your new website")) && !bytes.Contains(home, []byte("Hello, world")) {
		return themes.Theme{}, errors.New("the home page shows neither its own text nor the latest post")
	}

	theme := themes.Theme{
		Name:        name,
		Title:       cmp.Or(strings.TrimSpace(meta.Name), name),
		Description: strings.TrimSpace(meta.Description),
		Author:      authorName(meta.Author),
		Tags:        meta.Tags,
		License:     strings.TrimSpace(meta.License),
		Demo:        meta.Demosite,
		Updated:     updated,
		Module:      module,
		Commit:      commit,
		Archive:     archiveLink,
		Subdir:      subdir,
		SHA256:      sum,
		Thumbnail:   rawURL(repo, commit, path.Join(subdir, thumbnail)),
	}
	if theme.Author == "" && len(meta.Authors) > 0 {
		theme.Author = meta.Authors[0].Name
	}
	if meta.MinVersion != nil {
		theme.MinHugoVersion = fmt.Sprint(meta.MinVersion)
	}
	if screenshot := findImage(themeDir, "screenshot"); screenshot != "" {
		theme.Screenshot = rawURL(repo, commit, path.Join(subdir, screenshot))
	}
	return theme, nil
}

// findImage returns the path of images/name.png or images/name.jpg in the
// theme, the formats themes.gohugo.io accepts, or "" if there is neither.
func findImage(themeDir, name string) string {
	for _, file := range []string{"images/" + name + ".png", "images/" + name + ".jpg"} {
		if fsutil.Exists(filepath.Join(themeDir, filepath.FromSlash(file))) {
			return file
		}
	}
	return ""
}

// authorName reads theme.toml's author, which themes write as a table with
// a name or as a plain string.
func authorName(author any) string {
	switch author := author.(type) {
	case string:
		return strings.TrimSpace(author)
	case map[string]any:
		name, _ := author["name"].(string)
		return strings.TrimSpace(name)
	}
	return ""
}

// buildError keeps the end of Hugo's error, where the cause is. The start is
// a chain of long template paths.
func buildError(err error) string {
	message := []rune(strings.Join(strings.Fields(err.Error()), " "))
	if len(message) > 400 {
		return "…" + string(message[len(message)-400:])
	}
	return string(message)
}

// splitModule separates a module path like github.com/owner/repo/v2 into
// the repository and what follows it.
func splitModule(module string) (repo, rest string) {
	parts := strings.SplitN(module, "/", 4)
	if len(parts) < 4 {
		return module, ""
	}
	return strings.Join(parts[:3], "/"), parts[3]
}

var majorVersion = regexp.MustCompile(`^v\d+$`)

// findSubdir finds the theme inside the archive. The rest of a module path
// is a folder, or a Go major version like /v2 of a module at the root.
func findSubdir(archive []byte, rest, scratch string) (string, error) {
	if rest == "" {
		return "", nil
	}
	if _, _, err := themes.Extract(bytes.NewReader(archive), "", scratch); err != nil {
		return "", err
	}
	if fsutil.Exists(filepath.Join(scratch, filepath.FromSlash(rest), "theme.toml")) {
		return rest, nil
	}
	if majorVersion.MatchString(rest) {
		return "", nil
	}
	return "", fmt.Errorf("no theme in %s", rest)
}

func headCommit(ctx context.Context, repo string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	command := exec.CommandContext(ctx, "git", "ls-remote", "https://"+repo, "HEAD")
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.Output()
	commit, _, _ := strings.Cut(string(output), "\t")
	if err != nil || len(commit) != 40 {
		return "", fmt.Errorf("can't find the latest commit: %v", err)
	}
	return commit, nil
}

func archiveURL(repo, commit string) (string, error) {
	host, _, _ := strings.Cut(repo, "/")
	switch host {
	case "github.com", "codeberg.org":
		return "https://" + repo + "/archive/" + commit + ".tar.gz", nil
	case "gitlab.com":
		return fmt.Sprintf("https://%s/-/archive/%s/%s-%s.tar.gz", repo, commit, path.Base(repo), commit), nil
	}
	return "", fmt.Errorf("%s isn't a supported host", host)
}

func rawURL(repo, commit, file string) string {
	host, ownerRepo, _ := strings.Cut(repo, "/")
	switch host {
	case "github.com":
		return "https://raw.githubusercontent.com/" + ownerRepo + "/" + commit + "/" + file
	case "gitlab.com":
		return "https://" + repo + "/-/raw/" + commit + "/" + file
	}
	return "https://" + repo + "/raw/commit/" + commit + "/" + file
}

// nameDuplicates gives themes whose repositories share a name a folder
// prefixed with the owner, so they can be installed side by side.
func nameDuplicates(list []themes.Theme) {
	counts := map[string]int{}
	for _, theme := range list {
		counts[theme.Name]++
	}
	for index, theme := range list {
		if counts[theme.Name] > 1 {
			owner := strings.Split(theme.Module, "/")[1]
			list[index].Name = owner + "-" + theme.Name
		}
	}
}

// printChanges summarizes the difference from the current index so the
// person running this knows what they are about to commit.
func printChanges(out string, list []themes.Theme, checked int) {
	var previous themes.Index
	if data, err := os.ReadFile(out); err == nil {
		_ = json.Unmarshal(data, &previous)
	}
	before := map[string]bool{}
	for _, theme := range previous.Themes {
		before[theme.Module] = true
	}

	var added []string
	for _, theme := range list {
		if !before[theme.Module] {
			added = append(added, theme.Module)
		}
		delete(before, theme.Module)
	}
	removed := slices.Sorted(maps.Keys(before))

	fmt.Fprintf(os.Stderr, "\n%d of %d themes passed.\n", len(list), checked)
	fmt.Fprintf(os.Stderr, "Added (%d): %s\n", len(added), strings.Join(added, ", "))
	fmt.Fprintf(os.Stderr, "Removed (%d): %s\n", len(removed), strings.Join(removed, ", "))
}
