package diskmanager

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

// CheckAndClean removes oldest frames when disk usage exceeds threshold
func CheckAndClean(spoolDir string, maxGB float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(spoolDir, &stat); err != nil {
		log.Printf("[disk] Failed to stat %s: %v", spoolDir, err)
		return
	}

	totalGB := float64(stat.Blocks*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	usedGB := float64((stat.Blocks-stat.Bfree)*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	freeGB := totalGB - usedGB

	if freeGB > maxGB {
		return // plenty of space
	}

	log.Printf("[disk] Low space: %.1f GB free (threshold: %.1f GB). Cleaning...", freeGB, maxGB)

	// Find all FITS files sorted by age (oldest first)
	var files []string
	filepath.Walk(spoolDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".fits" {
			files = append(files, path)
		}
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		fi, _ := os.Stat(files[i])
		fj, _ := os.Stat(files[j])
		return fi.ModTime().Before(fj.ModTime())
	})

	// Don't delete files from today
	today := time.Now().Format("2006-01-02")
	deleted := 0
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		if fi.ModTime().Format("2006-01-02") == today {
			continue // never delete today's frames
		}

		// Delete FITS + corresponding JPEG
		os.Remove(f)
		jpegPath := f[:len(f)-5] + ".jpg"
		os.Remove(jpegPath)
		deleted++

		// Check if we have enough space now
		syscall.Statfs(spoolDir, &stat)
		freeNow := float64((stat.Bfree)*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
		if freeNow > maxGB*2 {
			break
		}
	}

	if deleted > 0 {
		log.Printf("[disk] Cleaned %d old frame files", deleted)
	}
}

// CleanByRetention deletes files older than maxDays
func CleanByRetention(dir string, maxDays int) {
	if maxDays <= 0 {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -maxDays)
	deleted := 0

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".fits" && ext != ".jpg" && ext != ".jpeg" {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(path)
			deleted++
		}
		return nil
	})

	if deleted > 0 {
		log.Printf("[disk] Retention cleanup: removed %d files older than %d days", deleted, maxDays)
	}
}

// CleanBySize deletes oldest files until total directory size is under maxSizeGB
func CleanBySize(dir string, maxSizeGB float64) {
	if maxSizeGB <= 0 {
		return
	}

	type fileEntry struct {
		path    string
		size    int64
		modTime time.Time
	}

	var files []fileEntry
	var totalSize int64

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".fits" && ext != ".jpg" && ext != ".jpeg" {
			return nil
		}
		files = append(files, fileEntry{path: path, size: info.Size(), modTime: info.ModTime()})
		totalSize += info.Size()
		return nil
	})

	maxBytes := int64(maxSizeGB * 1024 * 1024 * 1024)
	if totalSize <= maxBytes {
		return
	}

	// Sort oldest first
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	// Don't delete files from today
	today := time.Now().Format("2006-01-02")
	deleted := 0
	for _, f := range files {
		if totalSize <= maxBytes {
			break
		}
		if f.modTime.Format("2006-01-02") == today {
			continue
		}
		os.Remove(f.path)
		totalSize -= f.size
		deleted++
	}

	if deleted > 0 {
		log.Printf("[disk] Size cleanup: removed %d files to get under %.1f GB", deleted, maxSizeGB)
	}
}

// DirStats returns oldest and newest file timestamps and total size for a directory
func DirStats(dir string) (oldest, newest time.Time, totalSizeGB float64, fileCount int) {
	var totalBytes int64

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".fits" && ext != ".jpg" && ext != ".jpeg" {
			return nil
		}
		fileCount++
		totalBytes += info.Size()
		if oldest.IsZero() || info.ModTime().Before(oldest) {
			oldest = info.ModTime()
		}
		if newest.IsZero() || info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})

	totalSizeGB = float64(totalBytes) / (1024 * 1024 * 1024)
	return
}

// GetUsage returns disk usage info for a path
func GetUsage(path string) (totalGB, usedGB, freeGB float64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, 0
	}
	totalGB = float64(stat.Blocks*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	freeGB = float64(stat.Bfree*uint64(stat.Bsize)) / (1024 * 1024 * 1024)
	usedGB = totalGB - freeGB
	return
}
