package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/myrheimb/pageroom/internal/site"
)

// watcher notices files changed outside Pageroom (a developer in VS Code,
// a sync) and tells the UI to refresh, debounced. onChange learns whether
// the site preferences file was among the changes.
type watcher struct {
	w    *fsnotify.Watcher
	stop chan struct{}
	once sync.Once
}

func newWatcher(root string, onChange func(prefsChanged bool)) (*watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &watcher{w: fw, stop: make(chan struct{})}
	add := func(dir string) {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			name := d.Name()
			if p != dir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "public" || name == "resources") {
				return filepath.SkipDir
			}
			_ = fw.Add(p)
			return nil
		})
	}
	_ = fw.Add(root)
	for _, d := range []string{"content", "static", "assets", "config", "data"} {
		add(filepath.Join(root, d))
	}
	var prefsTouched atomic.Bool
	go func() {
		var timer *time.Timer
		for {
			select {
			case <-w.stop:
				return
			case ev, ok := <-fw.Events:
				if !ok {
					return
				}
				if strings.Contains(filepath.Base(ev.Name), ".pageroom-") {
					continue // our own atomic-write temp files
				}
				if filepath.Base(ev.Name) == site.PrefsFile {
					prefsTouched.Store(true)
				}
				if ev.Op&fsnotify.Create != 0 {
					if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
						add(ev.Name)
					}
				}
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(400*time.Millisecond, func() { onChange(prefsTouched.Swap(false)) })
			case <-fw.Errors:
			}
		}
	}()
	return w, nil
}

func (w *watcher) Close() {
	w.once.Do(func() {
		close(w.stop)
		w.w.Close()
	})
}

func goos() string { return runtime.GOOS }
