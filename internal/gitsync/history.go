package gitsync

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type Revision struct {
	Commit  string    `json:"commit"`
	Author  string    `json:"author"`
	When    time.Time `json:"when"`
	Message string    `json:"message"`
}

// History lists published versions of a file, newest first.
func (r *Repo) History(path string, limit int) ([]Revision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	head, err := r.headCommit()
	if err != nil || head == nil {
		return nil, err
	}
	iter, err := r.r.Log(&git.LogOptions{From: head.Hash, FileName: &path})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []Revision
	for len(out) < limit {
		c, err := iter.Next()
		if err != nil {
			break
		}
		msg, _, _ := strings.Cut(c.Message, "\n")
		out = append(out, Revision{Commit: c.Hash.String(), Author: c.Author.Name, When: c.Author.When, Message: msg})
	}
	return out, nil
}

func (r *Repo) FileAt(commit, path string) ([]byte, error) {
	c, err := r.r.CommitObject(plumbing.NewHash(commit))
	if err != nil {
		return nil, err
	}
	f, err := c.File(path)
	if err != nil {
		return nil, err
	}
	s, err := f.Contents()
	return []byte(s), err
}

func (r *Repo) blob(h plumbing.Hash) ([]byte, bool) {
	if h.IsZero() {
		return nil, false
	}
	b, err := r.r.BlobObject(h)
	if err != nil {
		return nil, false
	}
	rd, err := b.Reader()
	if err != nil {
		return nil, false
	}
	defer func() { _ = rd.Close() }()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rd); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func (r *Repo) readWorking(p string) ([]byte, bool) {
	b, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(p)))
	if err != nil {
		return nil, false
	}
	return b, true
}

func treeFiles(c *object.Commit) map[string]plumbing.Hash {
	out := map[string]plumbing.Hash{}
	if c == nil {
		return out
	}
	t, err := c.Tree()
	if err != nil {
		return out
	}
	_ = t.Files().ForEach(func(f *object.File) error {
		out[f.Name] = f.Hash
		return nil
	})
	return out
}

func union(a, b map[string]plumbing.Hash) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}
