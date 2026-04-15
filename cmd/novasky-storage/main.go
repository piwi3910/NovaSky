package main

import (
	"context"
	"io"
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
	novaskyRedis.StartHealthReporter(ctx, "storage")

	cfg := config.NewManager()
	cfg.Subscribe(ctx)

	exportDir := os.Getenv("EXPORT_DIR")
	if exportDir == "" {
		exportDir = "/home/piwi/novasky-data/export"
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log.Printf("[storage] Service ready, watching: %s", exportDir)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var storageCfg struct {
				Enabled bool   `json:"enabled"`
				Type    string `json:"type"` // nfs, smb, s3
				NFS     struct {
					MountPoint string `json:"mountPoint"`
				} `json:"nfs"`
				S3 struct {
					Bucket    string `json:"bucket"`
					Region    string `json:"region"`
					AccessKey string `json:"accessKey"`
					SecretKey string `json:"secretKey"`
				} `json:"s3"`
			}
			cfg.Get("storage.remote", &storageCfg)

			if !storageCfg.Enabled {
				continue
			}

			switch storageCfg.Type {
			case "nfs":
				syncToNFS(exportDir, storageCfg.NFS.MountPoint)
			case "s3":
				log.Printf("[storage] S3 sync configured but not yet implemented (bucket: %s)", storageCfg.S3.Bucket)
				// TODO: implement S3 upload using AWS SDK or raw REST API
			default:
				continue
			}
		}
	}
}

func syncToNFS(srcDir, destDir string) {
	if destDir == "" {
		log.Println("[storage] NFS mount point not configured")
		return
	}

	// Walk export directory and copy new files
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(srcDir, path)
		destPath := filepath.Join(destDir, relPath)

		// Skip if destination exists and same size
		if di, err := os.Stat(destPath); err == nil && di.Size() == info.Size() {
			return nil
		}

		// Copy file
		os.MkdirAll(filepath.Dir(destPath), 0755)
		src, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer src.Close()

		dst, err := os.Create(destPath)
		if err != nil {
			return nil
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		if err != nil {
			log.Printf("[storage] Copy failed %s: %v", relPath, err)
		} else {
			log.Printf("[storage] Synced: %s", relPath)
		}
		return nil
	})
}
