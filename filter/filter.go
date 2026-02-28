package filter

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4git/store"
)

// Clean reads content from r and writes the C4 ID to w, storing the original
// content in s. If the input is already a C4 ID, it passes through unchanged.
func Clean(r io.Reader, w io.Writer, s store.PrefixedFolder) error {
	// Read enough to check for idempotent re-clean (ID + optional newline).
	peek := make([]byte, 92)
	n, peekErr := io.ReadFull(r, peek)

	if n >= 90 && n <= 91 && (peekErr == io.ErrUnexpectedEOF || peekErr == io.EOF) {
		idStr := strings.TrimRight(string(peek[:n]), "\n")
		if len(idStr) == 90 {
			if _, err := c4.Parse(idStr); err == nil {
				fmt.Fprintln(w, idStr)
				return nil
			}
		}
	}

	// Not a C4 ID — process content through single-pass streaming.
	combined := io.MultiReader(bytes.NewReader(peek[:n]), r)

	tmp, err := os.CreateTemp(string(s), ".c4clean-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { os.Remove(tmpName) }() // cleanup on any error path

	tee := io.TeeReader(combined, tmp)
	id := c4.Identify(tee)

	if err := tmp.Close(); err != nil {
		return err
	}

	if err := s.Import(id, tmpName); err != nil {
		return err
	}

	fmt.Fprintln(w, id.String())
	return nil
}

// Smudge reads a C4 ID from r and writes the original content to w,
// fetching it from s. If the content is not found, the ID is written
// through as-is with a warning on stderr.
func Smudge(r io.Reader, w io.Writer, s store.PrefixedFolder) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	idStr := strings.TrimSpace(string(raw))

	id, err := c4.Parse(idStr)
	if err != nil {
		// Not a valid C4 ID — pass through unchanged.
		_, wErr := w.Write(raw)
		return wErr
	}

	rc, err := s.Open(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "c4git: content not found for %s, passing ID through\n", idStr)
		fmt.Fprintln(w, idStr)
		return nil
	}
	defer rc.Close()

	_, err = io.Copy(w, rc)
	return err
}
