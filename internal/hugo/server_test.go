package hugo

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrheimb/pageroom/internal/starter"
)

// Runs only where a Hugo binary is installed (CI installs one).
func TestPreviewServerAgainstStarterSite(t *testing.T) {
	bin, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("hugo not installed")
	}
	root := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(root, "Preview Test", DefaultVersion); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	srv, err := StartServer(context.Background(), bin, root, work)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	if !strings.HasPrefix(srv.URL, "http://127.0.0.1:") {
		t.Fatalf("preview must listen on loopback only, got %s", srv.URL)
	}
	resp, err := http.Get(srv.URL + "posts/welcome/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Hello, world") {
		t.Fatalf("status %d body %.200s", resp.StatusCode, body)
	}
	for _, junk := range []string{".hugo_build.lock", "public", "resources"} {
		if _, err := os.Stat(filepath.Join(root, junk)); err == nil {
			t.Errorf("preview wrote %s into the site folder", junk)
		}
	}
	if err := Check(context.Background(), bin, root, work, "", ""); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestEveryStarterThemeBuilds(t *testing.T) {
	bin, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("hugo not installed")
	}
	root := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(root, "Theme Test", DefaultVersion); err != nil {
		t.Fatal(err)
	}
	themes, err := os.ReadDir(filepath.Join(root, "themes"))
	if err != nil {
		t.Fatal(err)
	}

	for _, theme := range themes {
		t.Run(theme.Name(), func(t *testing.T) {
			if err := Check(context.Background(), bin, root, t.TempDir(), theme.Name(), ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckSaysWhyAThemeFails(t *testing.T) {
	bin, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("hugo not installed")
	}
	root := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(root, "Check Test", DefaultVersion); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()

	broken := filepath.Join(root, "themes", "broken", "layouts", "index.html")
	os.MkdirAll(filepath.Dir(broken), 0o755)
	os.WriteFile(broken, []byte(`{{ errorf "this theme is broken" }}`), 0o644)
	if err := Check(context.Background(), bin, root, work, "broken", ""); err == nil || !strings.Contains(err.Error(), "this theme is broken") {
		t.Fatalf("check of a broken theme: %v", err)
	}
}

func TestBuildReplacesTheLastBuildForProduction(t *testing.T) {
	bin, err := exec.LookPath("hugo")
	if err != nil {
		t.Skip("hugo not installed")
	}
	root := filepath.Join(t.TempDir(), "site")
	if err := starter.Create(root, "Build Test", DefaultVersion); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(root, "layouts"), 0o755)
	os.WriteFile(filepath.Join(root, "layouts", "home.html"), []byte(`{{ hugo.Environment }}`), 0o644)
	dest := t.TempDir()
	os.WriteFile(filepath.Join(dest, "deleted-page.html"), []byte("old"), 0o644)

	if err := Build(context.Background(), bin, root, t.TempDir(), dest); err != nil {
		t.Fatal(err)
	}
	if home, _ := os.ReadFile(filepath.Join(dest, "index.html")); string(home) != "production" {
		t.Errorf("built for %q", home)
	}
	if _, err := os.Stat(filepath.Join(dest, "deleted-page.html")); err == nil {
		t.Error("a page from the last build is still there")
	}
}

func TestCheckRefusesBadThemeNames(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "themes", "good"), 0o755)

	for _, name := range []string{"--config=/etc/passwd", "../outside", "a/b", ".hidden", "missing"} {
		if err := Check(context.Background(), "hugo-not-run", root, t.TempDir(), name, ""); err == nil || !strings.Contains(err.Error(), "theme") {
			t.Errorf("theme %q: %v", name, err)
		}
	}
}

func TestPreviewEnvKeepsCredentialsOut(t *testing.T) {
	t.Setenv("AWS_SECRET_ACCESS_KEY", "x")
	t.Setenv("SSH_AUTH_SOCK", "/tmp/agent")
	t.Setenv("HUGO_PARAMS_FOO", "bar")

	env := strings.Join(previewEnv(), "\n")
	for _, leaked := range []string{"AWS_SECRET_ACCESS_KEY", "SSH_AUTH_SOCK"} {
		if strings.Contains(env, leaked) {
			t.Errorf("%s reaches Hugo", leaked)
		}
	}
	if !strings.Contains(env, "HUGO_PARAMS_FOO=bar") || !strings.Contains(env, "HUGO_ENVIRONMENT=development") {
		t.Errorf("Hugo settings missing: %s", env)
	}
}
