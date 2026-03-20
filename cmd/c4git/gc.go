package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4git/config"
	c4store "github.com/Avalanche-io/c4/store"
)

func runGC(force bool) error {
	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	storePath := cfg.Stores[0].Path
	s, err := c4store.NewTreeStore(storePath)
	if err != nil {
		return err
	}

	// 1. Find all referenced IDs from git history + reflog.
	referenced, err := referencedIDs()
	if err != nil {
		return err
	}

	// 2. Also include staged files (prevents deleting uncommitted work).
	staged, err := managedFiles()
	if err != nil {
		return err
	}
	for _, f := range staged {
		referenced[f.ID.String()] = true
	}

	fmt.Fprintf(os.Stderr, "Scanning history... %d referenced IDs\n", len(referenced))

	// 3. Walk store to find all stored objects.
	var stored []c4.ID
	err = walkStore(storePath, func(id c4.ID) error {
		stored = append(stored, id)
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Scanning store... %d objects\n", len(stored))

	// 4. Compute unreferenced set.
	var unreferenced []c4.ID
	var totalSize int64
	for _, id := range stored {
		if referenced[id.String()] {
			continue
		}
		unreferenced = append(unreferenced, id)
		if rc, err := s.Open(id); err == nil {
			if f, ok := rc.(*os.File); ok {
				if info, err := f.Stat(); err == nil {
					totalSize += info.Size()
				}
			}
			rc.Close()
		}
	}

	if len(unreferenced) == 0 {
		fmt.Println("No unreferenced objects.")
		return nil
	}

	if !force {
		fmt.Printf("Would remove %d unreferenced objects (%s). Run with --force to delete.\n",
			len(unreferenced), formatSize(totalSize))
		return nil
	}

	var removed int
	var freedSize int64
	for _, id := range unreferenced {
		var size int64
		if rc, err := s.Open(id); err == nil {
			if f, ok := rc.(*os.File); ok {
				if info, err := f.Stat(); err == nil {
					size = info.Size()
				}
			}
			rc.Close()
		}
		if err := s.Remove(id); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not remove %s: %v\n", id.String()[:12], err)
			continue
		}
		removed++
		freedSize += size
	}
	fmt.Printf("Removed %d unreferenced objects (%s).\n", removed, formatSize(freedSize))
	return nil
}

// walkStore walks a TreeStore's directory tree, calling fn for each valid C4 ID
// found. The trie structure is traversed recursively: directories with 2-char
// names are interior nodes, files with parseable C4 ID names are content.
func walkStore(root string, fn func(c4.ID) error) error {
	return walkDir(root, fn)
}

func walkDir(dir string, fn func(c4.ID) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			if err := walkDir(filepath.Join(dir, entry.Name()), fn); err != nil {
				return err
			}
			continue
		}
		// Skip temp files.
		name := entry.Name()
		if len(name) > 0 && name[0] == '.' {
			continue
		}
		id, err := c4.Parse(name)
		if err != nil {
			continue
		}
		if err := fn(id); err != nil {
			return err
		}
	}
	return nil
}

// referencedIDs scans all reachable objects in git history and returns
// the set of C4 IDs referenced by any blob.
func referencedIDs() (map[string]bool, error) {
	// Get all reachable objects.
	revCmd := exec.Command("git", "rev-list", "--all", "--reflog", "--objects")
	revOut, err := revCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git rev-list: %w", err)
	}

	// Extract object hashes.
	var hashes []string
	scanner := bufio.NewScanner(bytes.NewReader(revOut))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 {
			hashes = append(hashes, fields[0])
		}
	}

	// Filter to blobs of size 90-91 via batch-check.
	var checkInput strings.Builder
	for _, h := range hashes {
		checkInput.WriteString(h)
		checkInput.WriteByte('\n')
	}

	checkCmd := exec.Command("git", "cat-file", "--batch-check")
	checkCmd.Stdin = strings.NewReader(checkInput.String())
	checkOut, err := checkCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git cat-file --batch-check: %w", err)
	}

	// Find blobs of size 90-91.
	var blobHashes []string
	scanner = bufio.NewScanner(bytes.NewReader(checkOut))
	for scanner.Scan() {
		// Format: "<hash> <type> <size>"
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 || fields[1] != "blob" {
			continue
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 90 || size > 91 {
			continue
		}
		blobHashes = append(blobHashes, fields[0])
	}

	if len(blobHashes) == 0 {
		return make(map[string]bool), nil
	}

	// Read matching blobs and parse as C4 IDs.
	var batchInput strings.Builder
	for _, h := range blobHashes {
		batchInput.WriteString(h)
		batchInput.WriteByte('\n')
	}

	catCmd := exec.Command("git", "cat-file", "--batch")
	catCmd.Stdin = strings.NewReader(batchInput.String())
	catOut, err := catCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git cat-file --batch: %w", err)
	}

	result := make(map[string]bool)
	buf := bytes.NewBuffer(catOut)
	for range blobHashes {
		header, err := buf.ReadString('\n')
		if err != nil {
			break
		}
		fields := strings.Fields(strings.TrimSpace(header))
		if len(fields) < 3 {
			continue
		}
		size, _ := strconv.Atoi(fields[2])
		content := make([]byte, size)
		n, _ := buf.Read(content)
		buf.ReadByte() // trailing newline

		if n != size {
			continue
		}
		idStr := strings.TrimSpace(string(content))
		if len(idStr) == 90 {
			if _, err := c4.Parse(idStr); err == nil {
				result[idStr] = true
			}
		}
	}
	return result, nil
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
