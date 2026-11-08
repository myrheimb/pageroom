// Package secrets keeps credentials in the operating system's secure store
// (macOS Keychain, Windows Credential Manager, Linux Secret Service) and
// keeps them out of files, logs, errors and the UI.
package secrets

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

const service = "Pageroom"

// ErrNotFound is returned when no credential is stored for a host.
var ErrNotFound = errors.New("no saved sign-in for this host")

// Credential is what Pageroom stores per git host. It is only ever kept in
// the OS keychain and in memory, and is never sent to the web UI.
type Credential struct {
	Kind         string    `json:"kind"` // "oauth", "token" or "basic"
	Username     string    `json:"username"`
	Secret       string    `json:"secret"` // token or password
	RefreshToken string    `json:"refreshToken,omitempty"`
	Expires      time.Time `json:"expires,omitempty"`
}

// Backend is the raw key/value secret storage.
type Backend interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

type Keyring struct{}

func (Keyring) Get(key string) (string, error) {
	v, err := keyring.Get(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}
func (Keyring) Set(key, value string) error { return keyring.Set(service, key, value) }
func (Keyring) Delete(key string) error {
	err := keyring.Delete(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Store saves credentials keyed by git host and registers every secret it
// handles with the redactor.
type Store struct {
	Backend  Backend
	Redactor *Redactor
}

// HostKey normalises a remote URL to "git:https://host" so one sign-in covers
// every repository on that host.
func HostKey(remote string) string {
	if u, err := url.Parse(remote); err == nil && u.Host != "" {
		return "git:" + strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Hostname())
	}
	// scp-style: git@github.com:owner/repo.git
	if at := strings.Index(remote, "@"); at >= 0 {
		host, _, _ := strings.Cut(remote[at+1:], ":")
		return "git:ssh://" + strings.ToLower(host)
	}
	return "git:" + strings.ToLower(remote)
}

func (s *Store) Get(remote string) (*Credential, error) {
	raw, err := s.Backend.Get(HostKey(remote))
	if err != nil {
		return nil, err
	}
	var c Credential
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, errors.New("saved sign-in is unreadable; please sign in again")
	}
	s.Redactor.Add(c.Secret, c.RefreshToken)
	return &c, nil
}

func (s *Store) Save(remote string, c Credential) error {
	s.Redactor.Add(c.Secret, c.RefreshToken)
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return s.Backend.Set(HostKey(remote), string(b))
}

func (s *Store) Delete(remote string) error { return s.Backend.Delete(HostKey(remote)) }
