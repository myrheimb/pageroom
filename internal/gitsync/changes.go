package gitsync

import (
	"errors"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func (r *Repo) Changes() ([]Change, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.changes()
}

func (r *Repo) changes() ([]Change, error) {
	wt, err := r.r.Worktree()
	if err != nil {
		return nil, err
	}
	st, err := wt.Status()
	if err != nil {
		return nil, err
	}
	var out []Change
	for p, s := range st {
		code := s.Worktree
		if code == git.Unmodified {
			code = s.Staging
		}
		switch code {
		case git.Unmodified:
			continue
		case git.Untracked, git.Added:
			out = append(out, Change{Path: p, Kind: Added})
		case git.Deleted:
			out = append(out, Change{Path: p, Kind: Deleted})
		default:
			out = append(out, Change{Path: p, Kind: Modified})
		}
	}
	// Committed-but-unpushed work (e.g. a publish that failed mid-push) is
	// still unpublished from the user's point of view.
	if ahead, _ := r.unpushedPaths(); len(ahead) > 0 {
		seen := map[string]bool{}
		for _, c := range out {
			seen[c.Path] = true
		}
		for _, p := range ahead {
			if !seen[p] {
				out = append(out, Change{Path: p, Kind: Modified})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// PendingPublish returns the message of a publish that was saved here but
// never reached the online repository, if publishing now would only retry
// it. It is empty when there is nothing to retry or there are newer edits.
func (r *Repo) PendingPublish() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ahead, err := r.unpushedPaths()
	if err != nil || len(ahead) == 0 {
		return "", err
	}
	dirty, err := r.dirtyPaths()
	if err != nil || len(dirty) > 0 {
		return "", err
	}
	head, err := r.headCommit()
	if err != nil {
		return "", err
	}
	subject, _, _ := strings.Cut(head.Message, "\n")
	return subject, nil
}

func (r *Repo) unpushedPaths() ([]string, error) {
	head, err := r.headCommit()
	if err != nil || head == nil {
		return nil, err
	}
	ref, err := r.r.Reference(r.remoteRef(), true)
	if err != nil {
		return nil, nil
	}
	if ref.Hash() == head.Hash {
		return nil, nil
	}
	remote, err := r.r.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}
	if ok, _ := head.IsAncestor(remote); ok {
		return nil, nil
	}
	bases, err := head.MergeBase(remote)
	if err != nil || len(bases) == 0 {
		return nil, err
	}
	a, b := treeFiles(bases[0]), treeFiles(head)
	var out []string
	for p := range union(a, b) {
		if a[p] != b[p] {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *Repo) headCommit() (*object.Commit, error) {
	ref, err := r.r.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil // brand-new repository without commits
	}
	if err != nil {
		return nil, err
	}
	return r.r.CommitObject(ref.Hash())
}
