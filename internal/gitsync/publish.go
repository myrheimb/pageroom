package gitsync

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

func (r *Repo) auth() (transport.AuthMethod, error) {
	if r.opt.Auth == nil {
		return nil, nil
	}
	return r.opt.Auth(r.RemoteURL())
}

// fetch updates the remote-tracking ref and returns its hash (zero if the
// remote branch does not exist yet, e.g. an empty new repository).
func (r *Repo) fetch(ctx context.Context) (plumbing.Hash, error) {
	if r.RemoteURL() == "" {
		return plumbing.ZeroHash, ErrNoRemote
	}
	auth, err := r.auth()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	spec := config.RefSpec(fmt.Sprintf("+%s:%s", r.branchRef(), r.remoteRef()))
	err = r.r.FetchContext(ctx, &git.FetchOptions{RemoteName: r.opt.RemoteName, RefSpecs: []config.RefSpec{spec}, Auth: auth})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		if isMissingRemoteBranch(err) {
			return plumbing.ZeroHash, nil
		}
		return plumbing.ZeroHash, friendly(r.opt.Redactor.Error(err))
	}
	ref, err := r.r.Reference(r.remoteRef(), true)
	if err != nil {
		return plumbing.ZeroHash, nil
	}
	return ref.Hash(), nil
}

func isMissingRemoteBranch(err error) bool {
	s := err.Error()
	return strings.Contains(s, "couldn't find remote ref") || errors.Is(err, transport.ErrEmptyRemoteRepository)
}

// Sync brings in other people's published changes without publishing ours.
func (r *Repo) Sync(ctx context.Context) (*Outcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lock, err := acquireLock(r.gitDir())
	if err != nil {
		return nil, err
	}
	defer lock.release()

	remote, err := r.fetch(ctx)
	if err != nil {
		return nil, err
	}
	if remote.IsZero() {
		return &Outcome{State: "up-to-date"}, nil
	}
	incoming, conflicts, err := r.integrate(remote, nil)
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		r.parked = &parkedMerge{remote: remote, conflicts: conflicts}
		return &Outcome{State: "conflicts", Conflicts: conflicts}, nil
	}
	if len(incoming) == 0 {
		return &Outcome{State: "up-to-date"}, nil
	}
	return &Outcome{State: "updated", Incoming: incoming}, nil
}

// Publish integrates remote changes, checks for secrets, commits and pushes.
func (r *Repo) Publish(ctx context.Context, message string) (*Outcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.publish(ctx, message)
}

func (r *Repo) publish(ctx context.Context, message string) (*Outcome, error) {
	lock, err := acquireLock(r.gitDir())
	if err != nil {
		return nil, err
	}
	defer lock.release()

	var incoming []string
	for attempt := 0; attempt < 4; attempt++ {
		remote, err := r.fetch(ctx)
		if err != nil {
			return nil, err
		}
		if !remote.IsZero() {
			in, conflicts, err := r.integrate(remote, nil)
			if err != nil {
				return nil, err
			}
			incoming = append(incoming, in...)
			if len(conflicts) > 0 {
				r.parked = &parkedMerge{remote: remote, conflicts: conflicts, publish: true, message: message}
				return &Outcome{State: "conflicts", Conflicts: conflicts, Incoming: incoming}, nil
			}
		}
		changes, err := r.changes()
		if err != nil {
			return nil, err
		}
		if len(changes) == 0 {
			head, err := r.headCommit()
			if err != nil {
				return nil, err
			}
			if head == nil || !remote.IsZero() {
				return &Outcome{State: "up-to-date", Incoming: incoming, Message: "Everything is already published."}, nil
			}
			// First publish to a brand-new, empty online repository.
			if err := r.push(ctx, remote); err != nil {
				return nil, err
			}
			return &Outcome{State: "published", Commit: head.Hash.String()}, nil
		}
		if findings := r.scan(changes); len(findings) > 0 {
			return &Outcome{State: "blocked", Secrets: findings, Incoming: incoming}, nil
		}

		commit, err := r.commitAll(message)
		if errors.Is(err, git.ErrEmptyCommit) {
			// An earlier publish saved its commit here but couldn't upload it.
			head, headErr := r.headCommit()
			if headErr != nil {
				return nil, headErr
			}
			commit, err = head.Hash, nil
		}
		if err != nil {
			return nil, err
		}
		err = r.push(ctx, remote)
		if err == nil {
			return &Outcome{State: "published", Commit: commit.String(), Incoming: incoming}, nil
		}
		if !isRaceRejection(err) {
			return nil, err
		}
		// Someone published between our fetch and our push. Loop: fetch
		// again, merge their work in, and try once more.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt*400) * time.Millisecond):
		}
	}
	return nil, errors.New("the site is being changed very frequently right now; please try publishing again in a minute")
}

func (r *Repo) push(ctx context.Context, expectedRemote plumbing.Hash) error {
	auth, err := r.auth()
	if err != nil {
		return err
	}
	opts := &git.PushOptions{
		RemoteName: r.opt.RemoteName,
		RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf("%s:%s", r.branchRef(), r.branchRef()))},
		Auth:       auth,
	}
	if !expectedRemote.IsZero() {
		// compare-and-swap: only update the branch if it is still exactly
		// what we merged with
		opts.RequireRemoteRefs = []config.RefSpec{config.RefSpec(fmt.Sprintf("%s:%s", expectedRemote, r.branchRef()))}
	}
	err = r.r.PushContext(ctx, opts)
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	if err != nil {
		return friendly(r.opt.Redactor.Error(err))
	}
	return nil
}

func isRaceRejection(err error) bool {
	s := err.Error()
	return errors.Is(err, git.ErrNonFastForwardUpdate) || strings.Contains(s, "required to be") ||
		strings.Contains(s, "non-fast-forward") || strings.Contains(s, "fetch first") ||
		strings.Contains(s, "stale info") || strings.Contains(s, "someone else published")
}

func (r *Repo) commitAll(message string) (plumbing.Hash, error) {
	wt, err := r.r.Worktree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return plumbing.ZeroHash, err
	}
	name, email := "Pageroom user", "pageroom@localhost"
	if r.opt.Author != nil {
		if n, e := r.opt.Author(); n != "" {
			name, email = n, e
		}
	}
	if strings.TrimSpace(message) == "" {
		message = "Update site"
	}
	sig := &object.Signature{Name: name, Email: email, When: time.Now()}
	return wt.Commit(message+"\n\nPublished with Pageroom", &git.CommitOptions{Author: sig, Committer: sig, AllowEmptyCommits: false})
}

func (r *Repo) gitDir() string { return filepath.Join(r.dir, ".git") }
