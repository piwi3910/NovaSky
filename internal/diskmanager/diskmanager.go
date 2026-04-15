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
