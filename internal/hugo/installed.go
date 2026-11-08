package hugo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/myrheimb/pageroom/internal/fsutil"
)

type Installed struct {
	Path     string `json:"path"`
	Version  string `json:"version"`
	Extended bool   `json:"extended"`
}

var versionLine = regexp.MustCompile(`hugo v(\d+\.\d+\.\d+)(\S*)`)

// parseVersion reads `hugo version` output, e.g.
// "hugo v0.168.0+extended+withdeploy darwin/arm64 BuildDate=unknown".
func parseVersion(out string) (version string, extended bool, ok bool) {
	m := versionLine.FindStringSubmatch(out)
	if m == nil {
		return "", false, false
	}
	return m[1], strings.Contains(m[2], "extended"), true
}

// candidatePaths lists where Hugo is commonly installed. Apps started from
// Finder or the Start menu don't get the shell's PATH.
func candidatePaths() []string {
	home, _ := os.UserHomeDir()
	user := os.Getenv("USER")
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		return []string{
			filepath.Join(home, "scoop", "shims", "hugo.exe"),
			`C:\ProgramData\chocolatey\bin\hugo.exe`,
			filepath.Join(local, "Microsoft", "WinGet", "Links", "hugo.exe"),
			filepath.Join(home, "go", "bin", "hugo.exe"),
		}
	}
	return []string{
		"/opt/homebrew/bin/hugo",                           // Homebrew (Apple silicon)
		"/usr/local/bin/hugo",                              // Homebrew (Intel), manual installs
		"/opt/local/bin/hugo",                              // MacPorts
		filepath.Join(home, ".nix-profile", "bin", "hugo"), // Nix (single user)
		"/etc/profiles/per-user/" + user + "/bin/hugo",     // nix-darwin / NixOS home-manager
		"/run/current-system/sw/bin/hugo",                  // nix-darwin / NixOS system
		"/nix/var/nix/profiles/default/bin/hugo",           // Nix (multi-user default profile)
		filepath.Join(home, ".local", "state", "nix", "profile", "bin", "hugo"),
		filepath.Join(home, "go", "bin", "hugo"),
		filepath.Join(home, ".local", "bin", "hugo"),
		filepath.Join(home, "bin", "hugo"),
		"/snap/bin/hugo",
		"/usr/bin/hugo",
	}
}

// loginShellHugo asks the user's login shell where hugo is, which picks up
// PATH set in .zprofile/.bash_profile and similar.
func loginShellHugo() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-l", "-c", "command -v hugo").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	p := strings.TrimSpace(lines[len(lines)-1])
	if filepath.IsAbs(p) {
		return p
	}
	return ""
}

var (
	installedOnce sync.Once
	installedList []Installed
)

// FindInstalled returns every distinct Hugo installed on this computer.
// The scan runs once per app session (asking the login shell is slow).
func FindInstalled() []Installed {
	installedOnce.Do(func() { installedList = scanInstalled() })
	return installedList
}

func scanInstalled() []Installed {
	var paths []string
	if p, err := exec.LookPath("hugo"); err == nil {
		paths = append(paths, p)
	}
	if p := loginShellHugo(); p != "" {
		paths = append(paths, p)
	}
	paths = append(paths, candidatePaths()...)

	seen := map[string]bool{}
	var out []Installed
	for _, p := range paths {
		if !fsutil.Exists(p) {
			continue
		}
		real, err := filepath.EvalSymlinks(p)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		b, err := exec.CommandContext(ctx, p, "version").Output()
		cancel()
		if err != nil {
			continue
		}
		if v, ext, ok := parseVersion(string(b)); ok {
			out = append(out, Installed{Path: p, Version: v, Extended: ext})
		}
	}
	return out
}
