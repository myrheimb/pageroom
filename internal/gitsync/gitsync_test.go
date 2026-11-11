package gitsync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/myrheimb/pageroom/internal/merge"
)

const post = "---\ntitle: Post\n---\n\nFirst paragraph.\n\nSecond paragraph.\n\nThird paragraph.\n"

// setup creates a bare "online" repository with one published post and
// returns it together with a function that clones a new teammate.
func setup(t *testing.T) (remote string, clone func(name string) *Repo) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	if _, err := git.PlainInitWithOptions(remote, &git.PlainInitOptions{Bare: true,
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}}); err != nil {
		t.Fatal(err)
	}
	seed, err := Init(filepath.Join(root, "seed"), Options{Author: author("Seed")})
	if err != nil {
		t.Fatal(err)
	}
	_ = seed.SetRemote(remote)
	write(t, seed, "content/post.md", post)
	if out, err := seed.Publish(context.Background(), "seed"); err != nil || out.State != "published" {
		t.Fatalf("seed publish: %+v %v", out, err)
	}
	return remote, func(name string) *Repo {
		r, err := Clone(context.Background(), remote, filepath.Join(root, name), Options{Author: author(name)})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
}

func author(n string) func() (string, string) {
	return func() (string, string) { return n, strings.ToLower(n) + "@example.com" }
}

func write(t *testing.T, r *Repo, p, s string) {
	t.Helper()
	full := filepath.Join(r.Dir(), p)
	_ = os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, r *Repo, p string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.Dir(), p))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func publish(t *testing.T, r *Repo, want string) *Outcome {
	t.Helper()
	out, err := r.Publish(context.Background(), "edit")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if out.State != want {
		t.Fatalf("publish state = %s, want %s (%+v)", out.State, want, out)
	}
	return out
}

func TestConcurrentEditsToDifferentParagraphsBothSurvive(t *testing.T) {
	_, clone := setup(t)
	anna, ben := clone("Anna"), clone("Ben")

	write(t, anna, "content/post.md", strings.Replace(post, "First paragraph.", "First paragraph by Anna.", 1))
	write(t, ben, "content/post.md", strings.Replace(post, "Third paragraph.", "Third paragraph by Ben.", 1))
	publish(t, anna, "published")
	publish(t, ben, "published") // Ben never synced: must merge, not overwrite

	check := clone("Check")
	got := read(t, check, "content/post.md")
	if !strings.Contains(got, "by Anna") || !strings.Contains(got, "by Ben") {
		t.Fatalf("a teammate's work was lost:\n%s", got)
	}
	if ch, _ := ben.Changes(); len(ch) != 0 {
		t.Fatalf("Ben should be fully published, still has %v", ch)
	}
}

func TestSameParagraphConflictTouchesNothingUntilResolved(t *testing.T) {
	_, clone := setup(t)
	anna, ben := clone("Anna"), clone("Ben")
	annaText := strings.Replace(post, "Second paragraph.", "Anna's version.", 1)
	benText := strings.Replace(post, "Second paragraph.", "Ben's version.", 1)
	write(t, anna, "content/post.md", annaText)
	write(t, ben, "content/post.md", benText)
	publish(t, anna, "published")

	out := publish(t, ben, "conflicts")
	if len(out.Conflicts) != 1 || out.Conflicts[0].Path != "content/post.md" || out.Conflicts[0].TheirAuthor != "Anna" {
		t.Fatalf("conflicts = %+v", out.Conflicts)
	}
	if read(t, ben, "content/post.md") != benText {
		t.Fatal("Ben's file was modified before he decided")
	}

	res, err := ben.Resolve(context.Background(), map[string]Choice{
		"content/post.md": {Hunks: []merge.Resolution{merge.KeepBoth}},
	})
	if err != nil || res.State != "published" {
		t.Fatalf("resolve: %+v %v", res, err)
	}
	got := read(t, clone("Check"), "content/post.md")
	if !strings.Contains(got, "Anna's version.") || !strings.Contains(got, "Ben's version.") {
		t.Fatalf("keep both failed:\n%s", got)
	}
}

func TestResolveRefusesIfFileChangedMeanwhile(t *testing.T) {
	_, clone := setup(t)
	anna, ben := clone("Anna"), clone("Ben")
	write(t, anna, "content/post.md", "A\n")
	write(t, ben, "content/post.md", "B\n")
	publish(t, anna, "published")
	publish(t, ben, "conflicts")
	write(t, ben, "content/post.md", "B, edited further\n")
	if _, err := ben.Resolve(context.Background(), map[string]Choice{"content/post.md": {Whole: "theirs"}}); err == nil {
		t.Fatal("expected refusal")
	}
	if read(t, ben, "content/post.md") != "B, edited further\n" {
		t.Fatal("newer edits lost")
	}
}

func TestStalePushIsRejectedNotForced(t *testing.T) {
	_, clone := setup(t)
	anna, ben := clone("Anna"), clone("Ben")
	write(t, anna, "content/a.md", "a\n")
	publish(t, anna, "published")

	// Ben commits without fetching and pushes with an out-of-date lease,
	// exactly what would happen if Anna published mid-way through his push.
	write(t, ben, "content/b.md", "b\n")
	stale, _ := ben.r.Reference(ben.remoteRef(), true)
	if _, err := ben.commitAll("b"); err != nil {
		t.Fatal(err)
	}
	err := ben.push(context.Background(), stale.Hash())
	if err == nil || !isRaceRejection(err) {
		t.Fatalf("stale push must be rejected as a race, got %v", err)
	}
	// The normal path recovers: fetch, merge, push.
	publish(t, ben, "published")
	c := clone("Check")
	read(t, c, "content/a.md")
	read(t, c, "content/b.md")
}

func TestSyncBringsInChangesAndKeepsLocalEdits(t *testing.T) {
	_, clone := setup(t)
	anna, ben := clone("Anna"), clone("Ben")
	write(t, anna, "content/new.md", "new page\n")
	_ = os.Remove(filepath.Join(anna.Dir(), "content/post.md"))
	publish(t, anna, "published")

	write(t, ben, "content/draft.md", "Ben's draft\n")
	out, err := ben.Sync(context.Background())
	if err != nil || out.State != "updated" {
		t.Fatalf("sync: %+v %v", out, err)
	}
	if read(t, ben, "content/new.md") != "new page\n" || read(t, ben, "content/draft.md") != "Ben's draft\n" {
		t.Fatal("sync lost or missed files")
	}
	if _, err := os.Stat(filepath.Join(ben.Dir(), "content/post.md")); !os.IsNotExist(err) {
		t.Fatal("remote deletion not applied")
	}
	ch, _ := ben.Changes()
	if len(ch) != 1 || ch[0].Path != "content/draft.md" || ch[0].Kind != Added {
		t.Fatalf("changes = %+v", ch)
	}
}

func TestDeleteVersusEditIsAConflict(t *testing.T) {
	_, clone := setup(t)
	anna, ben := clone("Anna"), clone("Ben")
	_ = os.Remove(filepath.Join(anna.Dir(), "content/post.md"))
	publish(t, anna, "published")
	write(t, ben, "content/post.md", post+"\nMore.\n")
	out := publish(t, ben, "conflicts")
	if !out.Conflicts[0].TheirsDeleted {
		t.Fatalf("%+v", out.Conflicts[0])
	}
	res, err := ben.Resolve(context.Background(), map[string]Choice{"content/post.md": {Whole: "ours"}})
	if err != nil || res.State != "published" {
		t.Fatalf("%+v %v", res, err)
	}
	if !strings.Contains(read(t, clone("Check"), "content/post.md"), "More.") {
		t.Fatal("kept edit missing")
	}
}

func TestSecretsBlockPublishing(t *testing.T) {
	_, clone := setup(t)
	anna := clone("Anna")
	write(t, anna, ".env", "TOKEN=abc")
	out := publish(t, anna, "blocked")
	if len(out.Secrets) != 1 || out.Secrets[0].Path != ".env" {
		t.Fatalf("%+v", out.Secrets)
	}
}

func TestUntrackedLocalFileIsNeverOverwritten(t *testing.T) {
	_, clone := setup(t)
	anna, ben := clone("Anna"), clone("Ben")
	write(t, anna, "static/logo.svg", "<svg>anna</svg>")
	publish(t, anna, "published")
	write(t, ben, "static/logo.svg", "<svg>ben</svg>")
	out, err := ben.Sync(context.Background())
	if err != nil || out.State != "conflicts" {
		t.Fatalf("%+v %v", out, err)
	}
	if read(t, ben, "static/logo.svg") != "<svg>ben</svg>" {
		t.Fatal("local file overwritten")
	}
}

func TestDiscardAndHistory(t *testing.T) {
	_, clone := setup(t)
	anna := clone("Anna")
	write(t, anna, "content/post.md", "oops\n")
	if err := anna.Discard("content/post.md"); err != nil {
		t.Fatal(err)
	}
	if read(t, anna, "content/post.md") != post {
		t.Fatal("discard did not restore")
	}
	h, err := anna.History("content/post.md", 10)
	if err != nil || len(h) != 1 || h[0].Author != "Seed" {
		t.Fatalf("history %+v %v", h, err)
	}
	b, _ := anna.FileAt(h[0].Commit, "content/post.md")
	if string(b) != post {
		t.Fatal("FileAt mismatch")
	}
}

func TestFirstPublishToEmptyRemote(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	_, _ = git.PlainInitWithOptions(remote, &git.PlainInitOptions{Bare: true, InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	r, _ := Init(filepath.Join(root, "site"), Options{Author: author("New")})
	write(t, r, "content/a.md", "hello\n")
	if err := r.CommitAll("Create site"); err != nil {
		t.Fatal(err)
	}
	if ch, _ := r.Changes(); len(ch) != 0 {
		t.Fatalf("new site should start clean: %v", ch)
	}
	_ = r.SetRemote(remote)
	publish(t, r, "published")
	other, err := Clone(context.Background(), remote, filepath.Join(root, "other"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, other, "content/a.md") != "hello\n" {
		t.Fatal("content not pushed")
	}
}

func TestFriendlyErrors(t *testing.T) {
	for _, test := range []struct{ message, code string }{
		{"command error on refs/heads/main: refusing to allow a Personal Access Token to create or update workflow `.github/workflows/pages.yml` without `workflow` scope", "workflow-permission"},
		{"command error on refs/heads/main: refusing to allow a GitHub App to create or update workflow `.github/workflows/pages.yml` without `workflows` permission", "workflow-permission"},
		{"command error on refs/heads/main: protected branch hook declined", "protected"},
		{"dial tcp: lookup github.com: no such host", "offline"},
	} {
		var userError *UserError
		if !errors.As(friendly(errors.New(test.message)), &userError) || userError.Code != test.code {
			t.Errorf("%q: got %v, want code %s", test.message, userError, test.code)
		}
	}
}

func TestMovingToANewRepositoryPublishesEverything(t *testing.T) {
	_, clone := setup(t)
	anna := clone("Anna")
	if err := anna.RemoveRemote(); err != nil {
		t.Fatal(err)
	}
	if anna.RemoteURL() != "" || anna.Published() {
		t.Fatalf("still connected: %q, published %v", anna.RemoteURL(), anna.Published())
	}

	moved := filepath.Join(t.TempDir(), "moved.git")
	if _, err := git.PlainInitWithOptions(moved, &git.PlainInitOptions{Bare: true,
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}}); err != nil {
		t.Fatal(err)
	}
	if err := anna.SetRemote(moved); err != nil {
		t.Fatal(err)
	}
	publish(t, anna, "published")
	if !anna.Published() {
		t.Error("should count as published after the first publish")
	}
	check, err := Clone(context.Background(), moved, filepath.Join(t.TempDir(), "check"), Options{Author: author("Check")})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, check, "content/post.md") != post {
		t.Fatal("the site didn't arrive in the new repository")
	}
}

func TestPublishRetriesACommitThatNeverReachedTheRemote(t *testing.T) {
	_, clone := setup(t)
	anna := clone("Anna")

	// A publish that committed but couldn't push, e.g. a rejected sign-in.
	write(t, anna, "content/post.md", strings.Replace(post, "First", "Fixed", 1))
	if _, err := anna.commitAll("Fix the typo"); err != nil {
		t.Fatal(err)
	}
	if pending, err := anna.PendingPublish(); pending != "Fix the typo" || err != nil {
		t.Fatalf("pending = %q, %v", pending, err)
	}
	publish(t, anna, "published")
	if pending, _ := anna.PendingPublish(); pending != "" {
		t.Fatalf("still pending after publishing: %q", pending)
	}
	if got := read(t, clone("Check"), "content/post.md"); !strings.Contains(got, "Fixed paragraph.") {
		t.Fatalf("the retried publish didn't arrive:\n%s", got)
	}
}

func TestPendingPublishIsEmptyWithNewerEdits(t *testing.T) {
	_, clone := setup(t)
	anna := clone("Anna")

	write(t, anna, "content/post.md", strings.Replace(post, "First", "Fixed", 1))
	if _, err := anna.commitAll("Fix the typo"); err != nil {
		t.Fatal(err)
	}
	write(t, anna, "content/new.md", post)
	if pending, err := anna.PendingPublish(); pending != "" || err != nil {
		t.Fatalf("pending = %q, %v", pending, err)
	}
	publish(t, anna, "published")
	check := clone("Check")
	if !strings.Contains(read(t, check, "content/post.md"), "Fixed paragraph.") || read(t, check, "content/new.md") != post {
		t.Fatal("both the earlier commit and the newer edit should be online")
	}
}
