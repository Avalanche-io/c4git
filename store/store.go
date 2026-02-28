package store

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Avalanche-io/c4"
)

// PrefixedFolder is a content-addressed store using a prefixed directory layout.
// The string value is the base path (e.g. ".c4/store/c4").
// Files are stored at {base}/{prefix}/{c4id} where prefix is characters 2-3
// of the C4 ID string (the first two varying characters after "c4").
type PrefixedFolder string

func (s PrefixedFolder) path(id c4.ID) string {
	str := id.String()
	return filepath.Join(string(s), str[2:4], str)
}

func (s PrefixedFolder) dir(id c4.ID) string {
	str := id.String()
	return filepath.Join(string(s), str[2:4])
}

// Has reports whether the store contains the given ID.
func (s PrefixedFolder) Has(id c4.ID) bool {
	_, err := os.Stat(s.path(id))
	return err == nil
}

// Open returns a reader for the content identified by id.
func (s PrefixedFolder) Open(id c4.ID) (io.ReadCloser, error) {
	return os.Open(s.path(id))
}

// Create returns a writer that stages content to a temp file and atomically
// moves it into place on Close. If the ID already exists, Close is a no-op.
func (s PrefixedFolder) Create(id c4.ID) (io.WriteCloser, error) {
	dir := s.dir(id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(dir, ".c4tmp-*")
	if err != nil {
		return nil, err
	}
	return &stageWriter{tmp: tmp, dst: s.path(id)}, nil
}

// Import atomically moves an existing file into the store at the correct
// prefixed path. If the ID already exists, the source file is removed.
func (s PrefixedFolder) Import(id c4.ID, src string) error {
	dst := s.path(id)
	if s.Has(id) {
		return os.Remove(src)
	}
	dir := s.dir(id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// Remove deletes the content identified by id.
func (s PrefixedFolder) Remove(id c4.ID) error {
	return os.Remove(s.path(id))
}

// stageWriter writes to a temp file, then renames on Close.
type stageWriter struct {
	tmp *os.File
	dst string
}

func (w *stageWriter) Write(p []byte) (int, error) {
	return w.tmp.Write(p)
}

func (w *stageWriter) Close() error {
	name := w.tmp.Name()
	if err := w.tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if _, err := os.Stat(w.dst); err == nil {
		return os.Remove(name)
	}
	return os.Rename(name, w.dst)
}
