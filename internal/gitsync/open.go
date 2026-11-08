package gitsync

import (
	"context"
	"errors"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/myrheimb/pageroom/internal/secrets"
)

func Open(dir string, opt Options) (*Repo, error) {
	r, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: false})
	if err != nil {
		return nil, err
	}
	if opt.RemoteName == "" {
		opt.RemoteName = "origin"
	}
	if opt.Redactor == nil {
		opt.Redactor = &secrets.Redactor{}
	}
	return &Repo{dir: dir, r: r, opt: opt}, nil
}

// Init creates a new repository with a "main" branch.
func Init(dir string, opt Options) (*Repo, error) {
	_, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")},
	})
	if err != nil {
		return nil, err
	}
	return Open(dir, opt)
}

func Clone(ctx context.Context, remoteURL, dir string, opt Options) (*Repo, error) {
	var auth transport.AuthMethod
	if opt.Auth != nil {
		var err error
		if auth, err = opt.Auth(remoteURL); err != nil {
			return nil, err
		}
	}
	_, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{URL: remoteURL, Auth: auth})
	if err != nil {
		_ = os.RemoveAll(dir)
		if opt.Redactor != nil {
			err = opt.Redactor.Error(err)
		}
		return nil, friendly(err)
	}
	return Open(dir, opt)
}

// CommitAll records every current file as a commit (used for a brand-new
// site, so it starts with nothing "unpublished").
func (r *Repo) CommitAll(message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, err := r.commitAll(message)
	return err
}

func (r *Repo) Dir() string { return r.dir }

func (r *Repo) RemoteURL() string {
	rem, err := r.r.Remote(r.opt.RemoteName)
	if err != nil || len(rem.Config().URLs) == 0 {
		return ""
	}
	return rem.Config().URLs[0]
}

// SetRemote points the repository at remoteURL (creating the remote if needed).
func (r *Repo) SetRemote(remoteURL string) error {
	_ = r.r.DeleteRemote(r.opt.RemoteName)
	_, err := r.r.CreateRemote(&config.RemoteConfig{Name: r.opt.RemoteName, URLs: []string{remoteURL}})
	return err
}

// RemoveRemote disconnects the online repository, and forgets what was
// published there, so the next publish to a new one sends everything.
func (r *Repo) RemoveRemote() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.r.Storer.RemoveReference(r.remoteRef()); err != nil {
		return err
	}
	if err := r.r.DeleteRemote(r.opt.RemoteName); err != nil && !errors.Is(err, git.ErrRemoteNotFound) {
		return err
	}
	return nil
}

// Published tells whether this branch has ever reached the online repository.
func (r *Repo) Published() bool {
	_, err := r.r.Reference(r.remoteRef(), true)
	return err == nil
}

func (r *Repo) Branch() string {
	if r.opt.Branch != "" {
		return r.opt.Branch
	}
	if head, err := r.r.Reference(plumbing.HEAD, false); err == nil && head.Type() == plumbing.SymbolicReference {
		return head.Target().Short()
	}
	return "main"
}

func (r *Repo) branchRef() plumbing.ReferenceName { return plumbing.NewBranchReferenceName(r.Branch()) }
func (r *Repo) remoteRef() plumbing.ReferenceName {
	return plumbing.NewRemoteReferenceName(r.opt.RemoteName, r.Branch())
}

// LocalSettings reads what Pageroom keeps about this copy of the site in
// .git/config, under [pageroom "name"]. It's never published.
func (r *Repo) LocalSettings(name string) map[string]string {
	cfg, err := r.r.Config()
	if err != nil || !cfg.Raw.Section("pageroom").HasSubsection(name) {
		return nil
	}
	values := map[string]string{}
	for _, option := range cfg.Raw.Section("pageroom").Subsection(name).Options {
		values[option.Key] = option.Value
	}
	return values
}

// SaveLocalSettings replaces the settings LocalSettings reads, or removes
// them when values is empty.
func (r *Repo) SaveLocalSettings(name string, values map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cfg, err := r.r.Config()
	if err != nil {
		return err
	}
	section := cfg.Raw.Section("pageroom").RemoveSubsection(name)
	if len(values) > 0 {
		subsection := section.Subsection(name)
		for key, value := range values {
			subsection.SetOption(key, value)
		}
	}
	return r.r.SetConfig(cfg)
}
