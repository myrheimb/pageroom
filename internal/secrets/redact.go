package secrets

import (
	"net/url"
	"regexp"
	"strings"
	"sync"
)

type Redactor struct {
	mu    sync.RWMutex
	known []string
}

var tokenShapes = regexp.MustCompile(
	`(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_\-]{20,}|` +
		`://[^/\s:@]+:[^/\s@]+@)`)

// Add registers secrets that must never appear in output.
func (r *Redactor) Add(secrets ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range secrets {
		if len(s) >= 6 {
			r.known = append(r.known, s)
		}
	}
}

// String returns s with every known secret and token-shaped string removed.
func (r *Redactor) String(s string) string {
	if r != nil {
		r.mu.RLock()
		for _, k := range r.known {
			s = strings.ReplaceAll(s, k, "•••")
		}
		r.mu.RUnlock()
	}
	return tokenShapes.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "://") {
			return "://•••@"
		}
		return "•••"
	})
}

func (r *Redactor) Error(err error) error {
	if err == nil {
		return nil
	}
	return redactedError{msg: r.String(err.Error()), err: err}
}

type redactedError struct {
	msg string
	err error
}

func (e redactedError) Error() string { return e.msg }
func (e redactedError) Unwrap() error { return e.err }

// StripURLCredentials removes user:password from a URL. It returns the clean
// URL and the credentials, so they can go to the keychain instead of
// .git/config.
func StripURLCredentials(raw string) (clean, user, pass string, changed bool) {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return raw, "", "", false
	}
	user = u.User.Username()
	pass, hasPass := u.User.Password()
	if !hasPass && user == "" {
		return raw, "", "", false
	}
	if !hasPass { // https://TOKEN@github.com/... form
		pass, user = user, "x-access-token"
	}
	u.User = nil
	return u.String(), user, pass, true
}
