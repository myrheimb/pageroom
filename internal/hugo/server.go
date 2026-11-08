package hugo

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Server is a running `hugo server` used for the live preview. It only
// listens on the loopback interface, never on the network.
type Server struct {
	cmd    *exec.Cmd
	URL    string
	cancel context.CancelFunc
	mu     sync.Mutex
	log    []string
	done   chan struct{}
	err    error
}

// StartServer launches a live preview for the site at root. workDir holds
// Hugo's cache and generated resources (HUGO_RESOURCEDIR, since the
// --resourceDir flag was removed), so previews never add files to the user's
// repository.
func StartServer(ctx context.Context, bin, root, workDir string) (*Server, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	args := []string{
		"server",
		"--source", root,
		"--bind", "127.0.0.1",
		"--port", fmt.Sprint(port),
		"--baseURL", url,
		"--appendPort=false",
		"--buildDrafts", "--buildFuture",
		"--disableFastRender",
		"--noBuildLock",
		"--renderToMemory",
		"--cacheDir", filepath.Join(workDir, "cache"),
		"--navigateToChanged=false",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = root
	cmd.Env = append(previewEnv(), "HUGO_RESOURCEDIR="+filepath.Join(workDir, "resources"))
	setProcessGroup(cmd)
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	s := &Server{cmd: cmd, URL: url, cancel: cancel, done: make(chan struct{})}
	go s.capture(stdout)
	go func() {
		s.err = cmd.Wait()
		close(s.done)
	}()
	if err := s.waitReady(port, 90*time.Second); err != nil {
		s.Stop()
		return nil, err
	}
	return s, nil
}

// previewEnvNames are the only variables Hugo and theme code get to see.
// Everything else (cloud credentials, SSH_AUTH_SOCK...) stays behind.
var previewEnvNames = map[string]bool{
	"APPDATA": true, "COMSPEC": true, "GOCACHE": true, "GOMODCACHE": true, "GOPATH": true,
	"GOPRIVATE": true, "GOPROXY": true, "HOME": true, "HOMEDRIVE": true, "HOMEPATH": true,
	"LANG": true, "LOCALAPPDATA": true, "PATH": true, "PATHEXT": true, "PROGRAMDATA": true,
	"PROGRAMFILES": true, "SYSTEMROOT": true, "TEMP": true, "TMP": true, "TMPDIR": true,
	"TZ": true, "USER": true, "USERNAME": true, "USERPROFILE": true, "WINDIR": true,
}

// previewEnv is the allowlisted part of the user's environment, plus the
// HUGO_ settings Hugo itself reads.
func previewEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		if previewEnvNames[upper] || strings.HasPrefix(upper, "LC_") || strings.HasPrefix(upper, "HUGO_") {
			out = append(out, kv)
		}
	}

	return append(out, "HUGO_ENVIRONMENT=development")
}

var errorLine = regexp.MustCompile(`(?i)\b(error|err:)\b`)

func (s *Server) capture(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		s.mu.Lock()
		s.log = append(s.log, sc.Text())
		if len(s.log) > 400 {
			s.log = s.log[len(s.log)-400:]
		}
		s.mu.Unlock()
	}
}

// Errors returns recent error lines from Hugo (bad template, broken
// shortcode...), so the UI can show what's wrong in plain view.
func (s *Server) Errors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for i := len(s.log) - 1; i >= 0 && len(out) < 10; i-- {
		l := s.log[i]
		if strings.Contains(l, "Rebuilt in") || strings.Contains(l, "Change detected") {
			break // everything before the last good rebuild is old news
		}
		if errorLine.MatchString(l) {
			out = append([]string{l}, out...)
		}
	}
	return out
}

func (s *Server) waitReady(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-s.done:
			msg := strings.Join(s.Errors(), "\n")
			if msg == "" {
				s.mu.Lock()
				msg = strings.Join(s.log[max(0, len(s.log)-5):], "\n")
				s.mu.Unlock()
			}
			return fmt.Errorf("the preview couldn't start:\n%s", msg)
		default:
		}
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
		if err == nil {
			c.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("the preview took too long to start")
}

func (s *Server) Stop() {
	if s == nil {
		return
	}
	killProcessGroup(s.cmd)
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
	}
}

func (s *Server) Running() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Check builds the site, optionally with another theme, and returns Hugo's
// error output if it fails. The pages go to publishDir, or stay in memory
// when it's empty.
func Check(ctx context.Context, bin, root, workDir, theme, publishDir string) error {
	if theme != "" {
		if err := checkThemeName(root, theme); err != nil {
			return err
		}
	}

	// Not --quiet: some Hugo versions then fail without saying why.
	args := []string{"--source", root, "--noBuildLock", "--cacheDir", filepath.Join(workDir, "cache")}
	if publishDir == "" {
		args = append(args, "--renderToMemory")
	} else {
		args = append(args, "--destination", publishDir)
	}
	if theme != "" {
		args = append(args, "--theme", theme)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = root
	// Generated images go to Pageroom's cache, never into the user's repository.
	cmd.Env = append(previewEnv(), "HUGO_RESOURCEDIR="+filepath.Join(workDir, "resources-check"))
	return hugoErrors(cmd.CombinedOutput())
}

// checkThemeName refuses anything that is not the name of an installed theme
// folder, so a name can never reach Hugo as a flag or a path.
func checkThemeName(root, theme string) error {
	if theme != filepath.Base(theme) || strings.HasPrefix(theme, "-") || strings.HasPrefix(theme, ".") {
		return fmt.Errorf("invalid theme name %q", theme)
	}
	if info, err := os.Stat(filepath.Join(root, "themes", theme)); err != nil || !info.IsDir() {
		return fmt.Errorf("theme %q is not installed", theme)
	}

	return nil
}

// Build builds the live site into dest, for production, replacing what an
// earlier build left there.
func Build(ctx context.Context, bin, root, workDir, dest string) error {
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	args := []string{"--source", root, "--noBuildLock", "--cacheDir", filepath.Join(workDir, "cache"), "--destination", dest}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = root
	cmd.Env = append(previewEnv(), "HUGO_ENVIRONMENT=production", "HUGO_RESOURCEDIR="+filepath.Join(workDir, "resources-build"))
	return hugoErrors(cmd.CombinedOutput())
}

// hugoErrors keeps the lines of Hugo's output that explain a failed build.
func hugoErrors(out []byte, err error) error {
	if err == nil {
		return nil
	}

	var problems []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.ToUpper(line), "ERROR") {
			problems = append(problems, line)
		}
	}
	if len(problems) == 0 {
		return fmt.Errorf("%s", cmp.Or(strings.TrimSpace(string(out)), err.Error()))
	}
	return errors.New(strings.Join(problems, "\n"))
}
