// Package fsutil holds the small filesystem primitives every other package
// relies on to never corrupt or silently overwrite a user's files.
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrOutsideRoot = errors.New("path is outside the site folder")

// SafeJoin joins rel onto root and guarantees the result stays inside root,
// even if rel contains "..", absolute segments or symlinked parents.
func SafeJoin(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty path")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", ErrOutsideRoot
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	full := filepath.Join(absRoot, cleanRel)
	if !within(absRoot, full) {
		return "", ErrOutsideRoot
	}
	// Resolve symlinks on the deepest existing ancestor so a symlink inside the
	// site cannot point writes at the rest of the disk.
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", err
	}
	existing := full
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}
	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	if !within(realRoot, realExisting) {
		return "", ErrOutsideRoot
	}
	return full, nil
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// WriteFileAtomic writes to a temp file in the same directory and renames it
// into place, so a crash never leaves a half-written file. The file keeps its
// permissions.
func WriteFileAtomic(path string, data []byte, defaultPerm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	perm := defaultPerm
	if st, err := os.Stat(path); err == nil {
		perm = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".pageroom-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// CreateExclusive creates path only if it does not exist yet. It never
// overwrites. Callers pick a new name on os.ErrExist.
func CreateExclusive(path string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	return f.Close()
}

// UniquePath returns path, or path with "-2", "-3"... inserted before the
// extension, choosing the first name that does not exist on disk.
func UniquePath(path string) string {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
