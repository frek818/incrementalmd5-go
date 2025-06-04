package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	MD5TimestampFile = ".md5sum-timestamp"
)

func main() {
	totalStart := time.Now()
	var dir, output string
	flag.StringVar(&dir, "dir", ".", "Directory to process")
	flag.StringVar(&output, "output", "md5sums.txt", "Output file path")
	flag.Parse()

	targetDir, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("Invalid directory: %v", err)
	}
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		log.Fatalf("Directory does not exist: %s", targetDir)
	}

	outputPath, err := filepath.Abs(output)
	if err != nil {
		log.Fatalf("Invalid output path: %v", err)
	}

	existingChecksums := readChecksums(outputPath)
	newChecksums := make(map[string]string)
	for k, v := range existingChecksums {
		newChecksums[k] = v
	}

	timestampPath := filepath.Join(targetDir, MD5TimestampFile)
	lastRun := getLastRunTime(timestampPath)

	changed := false
	neededUpdate := false
	processedCount := 0
	processingStart := time.Now()

	buf := make([]byte, 8192)

	filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(targetDir, path)
		if err != nil {
			log.Printf("Relative path error: %s - %v", path, err)
			return nil
		}

		log.Printf("Checking %s", relPath)

		if strings.HasSuffix(relPath, MD5TimestampFile) {
			log.Println("SKIPPING")
			return nil
		}

		needsUpdate := info.ModTime().After(lastRun) || !fileExistsInChecksums(relPath, existingChecksums)
		if needsUpdate {
			sum, err := fileMD5(path, buf)
			if err != nil {
				log.Printf("Checksum failed: %s - %v", path, err)
				return nil
			}

			if existingChecksums[relPath] != sum {
				changed = true
				newChecksums[relPath] = sum
				processedCount++
			}
			neededUpdate = true
		}
		return nil
	})

	processingDuration := time.Since(processingStart)

	if !changed && mapsEqual(existingChecksums, newChecksums) {
		log.Printf("No changes detected. Existing file preserved: %s", outputPath)
		log.Printf("Total duration: %v", time.Since(totalStart))

		if neededUpdate {
			log.Printf("Updated last run: %s", timestampPath)
			updateLastRun(timestampPath)
		}
		return
	}

	if err := writeChecksums(outputPath, newChecksums); err != nil {
		log.Fatal(err)
	}
	updateLastRun(timestampPath)

	// Print updated checksums file contents
	log.Println("\nUpdated checksums:")
	if content, err := os.ReadFile(outputPath); err == nil {
		fmt.Print(string(content))
	} else {
		log.Printf("Failed to read output file: %v", err)
	}

	log.Printf("\nProcessed %d files in %v", processedCount, processingDuration)
	log.Printf("Total duration: %v | Entries: %d", time.Since(totalStart), len(newChecksums))
}

func fileMD5(path string, buf []byte) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.CopyBuffer(hash, file, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readChecksums(path string) map[string]string {
	checksums := make(map[string]string)
	file, err := os.Open(path)
	if err != nil {
		return checksums
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) == 2 {
			checksums[parts[1]] = parts[0]
		}
	}
	return checksums
}

func writeChecksums(path string, checksums map[string]string) error {
	tmpPath := path + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer file.Close()

	paths := make([]string, 0, len(checksums))
	for path := range checksums {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		if _, err := fmt.Fprintf(file, "%s  %s\n", checksums[path], path); err != nil {
			return err
		}
	}

	return os.Rename(tmpPath, path)
}

func getLastRunTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func updateLastRun(path string) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatal(err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal(err)
	}
	file.Close()
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		log.Fatal(err)
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if bv, exists := b[k]; !exists || bv != av {
			return false
		}
	}
	return true
}

func fileExistsInChecksums(path string, checksums map[string]string) bool {
	_, exists := checksums[path]
	return exists
}
