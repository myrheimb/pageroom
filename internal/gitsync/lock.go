package gitsync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// processLock stops two Pageroom windows (or a crashed earlier run) from
// syncing the same repository at the same time.
type processLock struct{ path string }

var errBusy = errors.New("another Pageroom window is publishing this site right now; try again in a moment")

func acquireLock(gitDir string) (*processLock, error) {
	p := filepath.Join(gitDir, "pageroom-sync.lock")
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			fmt.Fprintf(f, "%d %d", os.Getpid(), time.Now().Unix())
			f.Close()
			return &processLock{path: p}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if !stale(p) {
			return nil, errBusy
		}
		_ = os.Remove(p)
	}
	return nil, errBusy
}

// A lock is stale if it is older than 10 minutes: no sync takes that long,
// and it means a previous run crashed.
func stale(p string) bool {
	b, err := os.ReadFile(p)
	if err != nil {
		return true
	}
	fields := strings.Fields(string(b))
	if len(fields) != 2 {
		return true
	}
	ts, err := strconv.ParseInt(fields[1], 10, 64)
	return err != nil || time.Since(time.Unix(ts, 0)) > 10*time.Minute
}

func (l *processLock) release() { _ = os.Remove(l.path) }
