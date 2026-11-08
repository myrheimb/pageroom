package gitsync

import (
	"os"
	"path/filepath"

	"github.com/myrheimb/pageroom/internal/secrets"
)

func (r *Repo) Scan() ([]secrets.Finding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	changes, err := r.changes()
	if err != nil {
		return nil, err
	}
	return r.scan(changes), nil
}

func (r *Repo) scan(changes []Change) []secrets.Finding {
	var out []secrets.Finding
	for _, c := range changes {
		if c.Kind == Deleted {
			continue
		}
		b, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(c.Path)))
		if err != nil || len(b) > 20<<20 {
			continue
		}
		out = append(out, secrets.ScanFile(c.Path, b)...)
	}
	return out
}
