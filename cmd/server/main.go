// Command server runs the learning system: SQLite-backed API plus the
// embedded React frontend, served as a single binary on the LAN.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/api"
	"github.com/CTM-development/learning-system-vibe/internal/config"
	"github.com/CTM-development/learning-system-vibe/internal/llm"
	"github.com/CTM-development/learning-system-vibe/internal/mdsync"
	"github.com/CTM-development/learning-system-vibe/internal/sources"
	"github.com/CTM-development/learning-system-vibe/internal/srs"
	"github.com/CTM-development/learning-system-vibe/internal/store"
	"github.com/CTM-development/learning-system-vibe/web"
)

var version = "dev" // overridden at build time via -ldflags "-X main.version=..."

func main() {
	configPath := flag.String("config", "", "path to YAML config file (optional)")
	flag.Parse()

	if err := run(*configPath); err != nil {
		log.Fatal(err)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	for _, dir := range []string{cfg.NotesDir, cfg.AttachmentsDir, filepath.Dir(cfg.DBPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	syncer := &mdsync.Syncer{Store: st, NotesDir: cfg.NotesDir}
	if res, err := syncer.SyncAll(); err != nil {
		log.Printf("initial sync failed: %v", err)
	} else {
		log.Printf("initial sync: %+v", res)
	}

	watchCtx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()
	go func() {
		if err := syncer.Watch(watchCtx, time.Second); err != nil {
			log.Printf("watcher stopped: %v", err)
		}
	}()

	// Daily DB snapshots: the review history is the one thing that isn't a
	// plain file on disk, so it gets its own safety net.
	if cfg.BackupsDir != "" {
		backup := func() {
			if path, err := st.Backup(cfg.BackupsDir); err != nil {
				log.Printf("db backup failed: %v", err)
			} else if path != "" {
				log.Printf("db backup written: %s", path)
			}
		}
		backup()
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					backup()
				case <-watchCtx.Done():
					return
				}
			}
		}()
	}

	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: (&api.Server{
			Store:     st,
			Syncer:    syncer,
			Scheduler: srs.NewScheduler(),
			Sources:   &sources.Manager{Store: st, AttachmentsDir: cfg.AttachmentsDir},
			LLM:       &llm.Client{APIKey: cfg.OpenRouterAPIKey, BaseURL: cfg.LLMBaseURL},
			Config:    cfg,
			Version:   version,
		}).Handler(dist),
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("learning system %s listening on http://0.0.0.0:%d (notes: %s, db: %s)",
			version, cfg.Port, cfg.NotesDir, cfg.DBPath)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case <-stop:
		log.Print("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}
