package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/piwi3910/NovaSky/internal/config"
	"github.com/piwi3910/NovaSky/internal/db"
	novaskyRedis "github.com/piwi3910/NovaSky/internal/redis"
)

func main() {
	log.Println("[storage] Starting...")
	db.Init()
	novaskyRedis.Init()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { c := make(chan os.Signal, 1); signal.Notify(c, syscall.SIGINT, syscall.SIGTERM); <-c; cancel() }()

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	exportDir := os.Getenv("EXPORT_DIR")
	if exportDir == "" {
		exportDir = "/home/piwi/novasky-data/export"
	}

	log.Printf("[storage] Service started, watching: %s", exportDir)

	// Periodic sync check
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var storageCfg struct {
				NFS *struct{ MountPoint string `json:"mountPoint"` } `json:"nfs"`
				SMB *struct {
					Server string `json:"server"`
					Share  string `json:"share"`
				} `json:"smb"`
				S3 *struct {
					Bucket string `json:"bucket"`
					Region string `json:"region"`
				} `json:"s3"`
				Enabled bool `json:"enabled"`
			}
			cfg.Get("storage.remote", &storageCfg)

			if !storageCfg.Enabled {
				continue
			}

			// Count files to sync
			entries, _ := filepath.Glob(filepath.Join(exportDir, "*", "*.fits"))
			if len(entries) > 0 {
				log.Printf("[storage] %d files to sync (stub)", len(entries))
				// TODO: implement NFS copy, SMB mount+copy, S3 upload
			}
		}
	}
}
