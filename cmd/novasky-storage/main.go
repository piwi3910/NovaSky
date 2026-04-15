package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

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
					Endpoint  string `json:"endpoint"` // for MinIO compatibility
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
				syncToS3(ctx, exportDir, storageCfg.S3.Bucket, storageCfg.S3.Region, storageCfg.S3.AccessKey, storageCfg.S3.SecretKey, storageCfg.S3.Endpoint)
			default:
				continue
			}
		}
	}
}

func syncToS3(ctx context.Context, srcDir, bucket, region, accessKey, secretKey, endpoint string) {
	if bucket == "" {
		log.Println("[storage] S3 bucket not configured")
		return
	}
	if region == "" {
		region = "us-east-1"
	}

	// Build AWS config with static credentials
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		log.Printf("[storage] Failed to load AWS config: %v", err)
		return
	}

	// Build S3 client options (custom endpoint for MinIO)
	s3Opts := []func(*s3.Options){}
	if endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = true // MinIO requires path-style
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	// Walk export directory and upload new files
	uploaded := 0
	filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}

		// Only upload FITS and JPEG files
		ext := filepath.Ext(path)
		if ext != ".fits" && ext != ".jpg" && ext != ".jpeg" {
			return nil
		}

		// Date-based S3 key: novasky/YYYY/MM/DD/filename
		modTime := info.ModTime()
		s3Key := fmt.Sprintf("novasky/%s/%s", modTime.Format("2006/01/02"), filepath.Base(path))

		// Check if object already exists with same size (skip re-upload)
		headOut, headErr := client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: &bucket,
			Key:    &s3Key,
		})
		if headErr == nil && headOut.ContentLength != nil && *headOut.ContentLength == info.Size() {
			return nil // already uploaded
		}

		// Open and upload
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		_, err = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: &bucket,
			Key:    &s3Key,
			Body:   f,
		})
		if err != nil {
			log.Printf("[storage] S3 upload failed %s: %v", s3Key, err)
			return nil
		}

		uploaded++
		log.Printf("[storage] S3 uploaded: %s", s3Key)
		return nil
	})

	if uploaded > 0 {
		log.Printf("[storage] S3 sync complete: %d files uploaded", uploaded)
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
