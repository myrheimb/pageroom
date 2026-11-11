package gitsync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/myrheimb/pageroom/internal/fsutil"
	"github.com/myrheimb/pageroom/internal/merge"
)

// integrate merges the remote commit into the working folder. On conflicts
// nothing is written. overrides holds resolved content per path (a nil
// value deletes).
//
//nolint:gocyclo // The three-way merge reads top to bottom, and splitting it would hide the order.
func (r *Repo) integrate(remote plumbing.Hash, overrides map[string]*[]byte) ([]string, []Conflict, error) {
	head, err := r.headCommit()
	if err != nil {
		return nil, nil, err
	}
	if head != nil && head.Hash == remote {
		return nil, nil, nil
	}
	remoteCommit, err := r.r.CommitObject(remote)
	if err != nil {
		return nil, nil, err
	}
	if head != nil {
		if ok, _ := remoteCommit.IsAncestor(head); ok {
			return nil, nil, nil // we are ahead, so there is nothing new from them
		}
	}

	var baseFiles, headFiles map[string]plumbing.Hash
	if head != nil {
		headFiles = treeFiles(head)
		if bases, err := head.MergeBase(remoteCommit); err == nil && len(bases) > 0 {
			baseFiles = treeFiles(bases[0])
		}
	}
	theirFiles := treeFiles(remoteCommit)

	dirty, err := r.dirtyPaths()
	if err != nil {
		return nil, nil, err
	}
	oursChanged := func(p string) bool { return dirty[p] || headFiles[p] != baseFiles[p] }

	type write struct {
		path string
		data []byte // nil = delete
	}
	var plan []write
	var incoming []string
	var conflicts []Conflict

	paths := union(baseFiles, theirFiles)
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		if theirFiles[p] != baseFiles[p] {
			sorted = append(sorted, p)
		}
	}
	sort.Strings(sorted)

	for _, p := range sorted {
		if ov, ok := overrides[p]; ok {
			if ov == nil {
				plan = append(plan, write{path: p})
			} else {
				plan = append(plan, write{path: p, data: *ov})
			}
			continue
		}
		base, baseOK := r.blob(baseFiles[p])
		theirs, theirsOK := r.blob(theirFiles[p])
		ours, oursOK := r.readWorking(p)

		if !oursChanged(p) {
			// Safety net for ignored or otherwise untracked local files
			// sitting where an incoming file wants to go.
			if oursOK && (!baseOK || !bytes.Equal(ours, base)) && !(theirsOK && bytes.Equal(ours, theirs)) {
				conflicts = append(conflicts, Conflict{Path: p, LocalInWay: true, Binary: isBinary(ours) || isBinary(theirs),
					base: base, ours: ours, theirs: theirs, oursExists: true, TheirAuthor: remoteCommit.Author.Name})
				continue
			}
			if theirsOK {
				plan = append(plan, write{path: p, data: theirs})
			} else {
				plan = append(plan, write{path: p})
			}
			incoming = append(incoming, p)
			continue
		}

		// Both sides changed this file.
		if oursOK == theirsOK && bytes.Equal(ours, theirs) {
			continue
		}
		c := Conflict{Path: p, base: base, ours: ours, theirs: theirs, oursExists: oursOK, TheirAuthor: remoteCommit.Author.Name}
		switch {
		case !oursOK || !theirsOK:
			c.OursDeleted, c.TheirsDeleted = !oursOK, !theirsOK
			conflicts = append(conflicts, c)
		case isBinary(ours) || isBinary(theirs) || isBinary(base):
			c.Binary = true
			conflicts = append(conflicts, c)
		default:
			m := merge.Merge(string(base), string(ours), string(theirs))
			if m.HasConflicts() {
				c.Hunks = m.Hunks
				conflicts = append(conflicts, c)
				continue
			}
			plan = append(plan, write{path: p, data: []byte(m.Text(nil))})
			incoming = append(incoming, p)
		}
	}
	if len(conflicts) > 0 {
		return nil, conflicts, nil
	}

	for _, w := range plan {
		full, err := fsutil.SafeJoin(r.dir, w.path)
		if err != nil {
			return nil, nil, err
		}
		if w.data == nil {
			if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, nil, err
			}
			removeEmptyParents(r.dir, filepath.Dir(full))
			continue
		}
		if err := fsutil.WriteFileAtomic(full, w.data, 0o644); err != nil {
			return nil, nil, err
		}
	}

	// Move the branch and index to the remote commit but leave the working
	// folder alone. It holds their changes plus ours, so whatever differs from
	// the remote is our unpublished work.
	if err := r.r.Storer.SetReference(plumbing.NewHashReference(r.branchRef(), remote)); err != nil {
		return nil, nil, err
	}
	if head == nil {
		if err := r.r.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, r.branchRef())); err != nil {
			return nil, nil, err
		}
	}
	wt, err := r.r.Worktree()
	if err != nil {
		return nil, nil, err
	}
	if err := wt.Reset(&git.ResetOptions{Commit: remote, Mode: git.MixedReset}); err != nil {
		return nil, nil, err
	}
	return incoming, nil, nil
}

func (r *Repo) dirtyPaths() (map[string]bool, error) {
	wt, err := r.r.Worktree()
	if err != nil {
		return nil, err
	}
	st, err := wt.Status()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for p, s := range st {
		if s.Worktree != git.Unmodified || s.Staging != git.Unmodified {
			out[p] = true
		}
	}
	return out, nil
}

// Resolve applies the user's choices to the parked conflicts and continues
// whatever was interrupted (a Sync or a Publish).
func (r *Repo) Resolve(ctx context.Context, choices map[string]Choice) (*Outcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pm := r.parked
	if pm == nil {
		return nil, errors.New("there is nothing waiting to be resolved")
	}
	overrides := map[string]*[]byte{}
	for _, c := range pm.conflicts {
		// If the user kept editing while deciding, start over rather than
		// risk losing those edits.
		cur, ok := r.readWorking(c.Path)
		if ok != c.oursExists || !bytes.Equal(cur, c.ours) {
			r.parked = nil
			return nil, fmt.Errorf("%s changed while you were deciding; please publish again", c.Path)
		}
		ch, ok := choices[c.Path]
		if !ok {
			return nil, fmt.Errorf("please choose what to keep for %s", c.Path)
		}
		data, extra, err := resolveOne(c, ch)
		if err != nil {
			return nil, err
		}
		overrides[c.Path] = data
		for p, d := range extra {
			if err := fsutil.WriteFileAtomic(filepath.Join(r.dir, filepath.FromSlash(p)), d, 0o644); err != nil {
				return nil, err
			}
		}
	}
	r.parked = nil
	// Apply the decisions to the version they were made for. If someone
	// published since, the publish below merges their work on top.
	lock, err := acquireLock(r.gitDir())
	if err != nil {
		return nil, err
	}
	incoming, conflicts, err := r.integrate(pm.remote, overrides)
	lock.release()
	if err != nil {
		return nil, err
	}
	if len(conflicts) > 0 {
		r.parked = &parkedMerge{remote: pm.remote, conflicts: conflicts, publish: pm.publish, message: pm.message}
		return &Outcome{State: "conflicts", Conflicts: conflicts}, nil
	}
	if pm.publish {
		out, err := r.publish(ctx, pm.message)
		if out != nil {
			out.Incoming = append(incoming, out.Incoming...)
		}
		return out, err
	}
	return &Outcome{State: "updated", Incoming: incoming}, nil
}

// resolveOne returns the content to keep for the conflicted path (nil =
// delete) plus extra files to create (a "keep both" copy for binaries).
func resolveOne(c Conflict, ch Choice) (*[]byte, map[string][]byte, error) {
	keep := func(b []byte, exists bool) *[]byte {
		if !exists {
			return nil
		}
		cp := append([]byte(nil), b...)
		return &cp
	}
	theirsExists := !c.TheirsDeleted
	if len(c.Hunks) > 0 && ch.Whole == "" {
		text := merge.Result{Hunks: c.Hunks}.Text(ch.Hunks)
		b := []byte(text)
		return &b, nil, nil
	}
	switch ch.Whole {
	case "ours":
		return keep(c.ours, c.oursExists), nil, nil
	case "theirs":
		return keep(c.theirs, theirsExists), nil, nil
	case "both":
		if !c.oursExists || !theirsExists {
			// keeping "both" of a deleted and an edited file means keeping the edit
			if c.oursExists {
				return keep(c.ours, true), nil, nil
			}
			return keep(c.theirs, true), nil, nil
		}
		if c.Binary || c.LocalInWay {
			ext := filepath.Ext(c.Path)
			copyPath := strings.TrimSuffix(c.Path, ext) + "-mine" + ext
			return keep(c.theirs, true), map[string][]byte{copyPath: c.ours}, nil
		}
		m := merge.Merge(string(c.base), string(c.ours), string(c.theirs))
		choices := make([]merge.Resolution, len(m.Hunks))
		for i := range choices {
			choices[i] = merge.KeepBoth
		}
		b := []byte(m.Text(choices))
		return &b, nil, nil
	}
	return nil, nil, fmt.Errorf("please choose what to keep for %s", c.Path)
}

// Discard throws away unpublished changes to one file, restoring the last
// published version (or removing a file that was never published).
func (r *Repo) Discard(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	full, err := fsutil.SafeJoin(r.dir, path)
	if err != nil {
		return err
	}
	head, err := r.headCommit()
	if err != nil {
		return err
	}
	if head != nil {
		if f, err := head.File(path); err == nil {
			content, err := f.Contents()
			if err != nil {
				return err
			}
			return fsutil.WriteFileAtomic(full, []byte(content), 0o644)
		}
	}
	return os.Remove(full)
}
