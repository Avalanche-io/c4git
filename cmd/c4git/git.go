package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Avalanche-io/c4"
)

// managedFile is a tracked file whose git blob content is a C4 ID.
type managedFile struct {
	Path string
	ID   c4.ID
}

// blobRef pairs a git blob hash with its working tree path.
type blobRef struct {
	hash string
	path string
}

// managedFiles returns all stage-0 tracked files whose blob content is a C4 ID.
// It uses git ls-files and git cat-file --batch for efficient bulk reads.
func managedFiles() ([]managedFile, error) {
	lsCmd := exec.Command("git", "ls-files", "-s")
	lsOut, err := lsCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}

	var refs []blobRef
	scanner := bufio.NewScanner(bytes.NewReader(lsOut))
	for scanner.Scan() {
		// Format: "<mode> <hash> <stage>\t<path>"
		line := scanner.Text()
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		fields := strings.Fields(line[:tab])
		if len(fields) != 3 || fields[2] != "0" {
			continue
		}
		refs = append(refs, blobRef{hash: fields[1], path: line[tab+1:]})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, nil
	}

	// Build batch input.
	var input strings.Builder
	for _, r := range refs {
		input.WriteString(r.hash)
		input.WriteByte('\n')
	}

	catCmd := exec.Command("git", "cat-file", "--batch")
	catCmd.Stdin = strings.NewReader(input.String())
	catOut, err := catCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git cat-file: %w", err)
	}

	return parseBatchOutput(catOut, refs)
}

func parseBatchOutput(data []byte, refs []blobRef) ([]managedFile, error) {
	var result []managedFile
	buf := bytes.NewBuffer(data)

	for _, ref := range refs {
		header, err := buf.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return result, nil
			}
			return result, fmt.Errorf("reading batch output for %s: %w", ref.path, err)
		}
		fields := strings.Fields(strings.TrimSpace(header))
		if len(fields) < 3 || fields[1] != "blob" {
			continue
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}

		if size < 90 || size > 91 {
			buf.Next(size + 1) // content + trailing newline
			continue
		}

		content := make([]byte, size)
		n, _ := buf.Read(content)
		buf.ReadByte() // trailing newline

		if n != size {
			continue
		}

		idStr := strings.TrimSpace(string(content))
		if len(idStr) != 90 {
			continue
		}
		id, err := c4.Parse(idStr)
		if err != nil {
			continue
		}
		result = append(result, managedFile{Path: ref.path, ID: id})
	}
	return result, nil
}
