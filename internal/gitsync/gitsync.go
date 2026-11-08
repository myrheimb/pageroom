// Package gitsync turns git into two verbs, "Get latest" and "Publish", with
// one hard rule: nobody's work is overwritten.
//
// The working folder is "ours", the remote branch is "theirs", and their
// merge-base is the common ancestor. Files only they changed are brought in,
// files only we changed are kept, and files both changed are merged paragraph
// by paragraph. A real overlap is returned as a Conflict and nothing on disk
// is touched. Pushes are never forced and fail if the remote moved, so a
// teammate publishing at the same moment makes us retry with their work
// merged in.
package gitsync

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/myrheimb/pageroom/internal/merge"
	"github.com/myrheimb/pageroom/internal/secrets"
)

type AuthProvider func(remoteURL string) (transport.AuthMethod, error)

type Options struct {
	RemoteName string // default "origin"
	Branch     string // default: the current branch
	Auth       AuthProvider
	Author     func() (name, email string)
	Redactor   *secrets.Redactor
}

type Repo struct {
	dir    string
	r      *git.Repository
	opt    Options
	mu     sync.Mutex
	parked *parkedMerge
}

type ChangeKind string

const (
	Added    ChangeKind = "added"
	Modified ChangeKind = "modified"
	Deleted  ChangeKind = "deleted"
)

type Change struct {
	Path string     `json:"path"`
	Kind ChangeKind `json:"kind"`
}

// Conflict is a file both people changed in overlapping ways.
type Conflict struct {
	Path          string       `json:"path"`
	Binary        bool         `json:"binary"`
	OursDeleted   bool         `json:"oursDeleted"`
	TheirsDeleted bool         `json:"theirsDeleted"`
	LocalInWay    bool         `json:"localInWay"` // an unpublished local file has the same name as an incoming one
	Hunks         []merge.Hunk `json:"hunks,omitempty"`
	TheirAuthor   string       `json:"theirAuthor,omitempty"`

	base, ours, theirs []byte
	oursExists         bool
}

// Choice resolves one conflicted file.
type Choice struct {
	Whole string             `json:"whole,omitempty"` // "ours", "theirs", "both" (binary: both keeps theirs as a copy)
	Hunks []merge.Resolution `json:"hunks,omitempty"` // per conflicting hunk, for text files
}

type Outcome struct {
	State     string            `json:"state"` // "up-to-date", "updated", "published", "conflicts", "blocked", "offline"
	Incoming  []string          `json:"incoming,omitempty"`
	Conflicts []Conflict        `json:"conflicts,omitempty"`
	Secrets   []secrets.Finding `json:"secrets,omitempty"`
	Commit    string            `json:"commit,omitempty"`
	Message   string            `json:"message,omitempty"`
}

type parkedMerge struct {
	remote    plumbing.Hash
	conflicts []Conflict
	publish   bool
	message   string
}

var ErrNoRemote = errors.New("this site is not connected to an online repository yet")

func isBinary(b []byte) bool {
	n := min(len(b), 8000)
	return bytes.IndexByte(b[:n], 0) >= 0 || !utf8.Valid(b[:n])
}

func removeEmptyParents(root, dir string) {
	for dir != root && strings.HasPrefix(dir, root) {
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// friendly rewrites common git errors in words a non-developer understands.
func friendly(err error) error {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed),
		strings.Contains(s, "authentication required"), strings.Contains(s, "authorization failed"):
		return &UserError{Code: "auth", Msg: "The online repository didn't accept your sign-in. Please sign in again.", Err: err}
	case errors.Is(err, transport.ErrRepositoryNotFound), strings.Contains(s, "repository not found"):
		return &UserError{Code: "not-found", Msg: "That repository couldn't be found, or your account doesn't have access to it.", Err: err}
	case strings.Contains(s, "no such host"), strings.Contains(s, "network is unreachable"), strings.Contains(s, "connection refused"),
		strings.Contains(s, "i/o timeout"), strings.Contains(s, "dial tcp"):
		return &UserError{Code: "offline", Msg: "Couldn't reach the internet. Your changes are saved on this computer and will be published when you're back online.", Err: err}
	case strings.Contains(s, "to create or update workflow"):
		return &UserError{Code: "workflow-permission", Msg: "Your GitHub sign-in isn't allowed to change workflows, which publishing to GitHub Pages needs. If you use an access token, edit it on GitHub to add the “Workflows: Read and write” permission, then publish again.", Err: err}
	case strings.Contains(s, "protected branch"), strings.Contains(s, "pre-receive hook declined"):
		return &UserError{Code: "protected", Msg: "This site's repository doesn't allow direct publishing to its main branch. Ask the site's developer to allow it, or to set up a review workflow.", Err: err}
	}
	return err
}

// UserError carries a plain-language message plus a machine code the UI can
// react to (e.g. "auth" opens the sign-in dialog).
type UserError struct {
	Code string
	Msg  string
	Err  error
}

func (e *UserError) Error() string { return e.Msg }
func (e *UserError) Unwrap() error { return e.Err }
