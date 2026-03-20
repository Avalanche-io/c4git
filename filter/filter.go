package filter

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/Avalanche-io/c4"
)

// Store is the minimal interface the filter needs from a content store.
type Store interface {
	Has(id c4.ID) bool
	Open(id c4.ID) (io.ReadCloser, error)
	Put(r io.Reader) (c4.ID, error)
}

// Clean reads content from r and writes the bare C4 ID (exactly 90 bytes,
// no newline) to w, storing the original content in s. If the input is
// already a bare C4 ID, it passes through unchanged.
func Clean(r io.Reader, w io.Writer, s Store) error {
	// Read enough to check for idempotent re-clean.
	// A bare C4 ID is exactly 90 bytes. Accept 90 (bare) or 91 (with newline).
	peek := make([]byte, 92)
	n, peekErr := io.ReadFull(r, peek)

	if n >= 90 && n <= 91 && (peekErr == io.ErrUnexpectedEOF || peekErr == io.EOF) {
		idStr := strings.TrimRight(string(peek[:n]), "\n")
		if len(idStr) == 90 {
			if _, err := c4.Parse(idStr); err == nil {
				_, wErr := io.WriteString(w, idStr)
				return wErr
			}
		}
	}

	// Not a C4 ID — store the content and write the bare ID (exactly 90 bytes).
	combined := io.MultiReader(bytes.NewReader(peek[:n]), r)

	id, err := s.Put(combined)
	if err != nil {
		return err
	}

	_, wErr := io.WriteString(w, id.String())
	return wErr
}

// Smudge reads a C4 ID from r and writes the original content to w,
// fetching it from s. If the content is not found, the bare ID is
// written through as-is with a warning on stderr.
func Smudge(r io.Reader, w io.Writer, s Store) error {
	raw, err := io.ReadAll(io.LimitReader(r, 256))
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
		os.Stderr.WriteString("c4git: content not found for " + idStr + ", passing ID through\n")
		_, wErr := io.WriteString(w, idStr)
		return wErr
	}
	defer rc.Close()

	_, err = io.Copy(w, rc)
	return err
}
