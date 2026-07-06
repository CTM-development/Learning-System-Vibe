package mdsync

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch runs SyncAll whenever files under the notes directory change,
// debounced so editor save bursts (and our own anchor write-backs) coalesce
// into one run. Blocks until ctx is cancelled.
func (s *Syncer) Watch(ctx context.Context, debounce time.Duration) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	addDirs := func(root string) {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			if err := watcher.Add(path); err != nil {
				log.Printf("watch %s: %v", path, err)
			}
			return nil
		})
	}
	addDirs(s.NotesDir)

	var timer *time.Timer
	fire := make(chan struct{}, 1)

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// New directories need watching too.
			if ev.Has(fsnotify.Create) {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					addDirs(ev.Name)
				}
			}
			relevant := strings.EqualFold(filepath.Ext(ev.Name), ".md") ||
				ev.Has(fsnotify.Create) || ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename)
			if !relevant {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(debounce, func() {
				select {
				case fire <- struct{}{}:
				default:
				}
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("watcher error: %v", err)
		case <-fire:
			if res, err := s.SyncAll(); err != nil {
				log.Printf("auto-sync failed: %v", err)
			} else if res.CardsCreated+res.CardsUpdated+res.CardsOrphaned+res.AnchorsWritten > 0 {
				log.Printf("auto-sync: %+v", res)
			}
		}
	}
}
