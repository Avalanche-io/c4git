package store

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
)

func testID(t *testing.T, content string) c4.ID {
	t.Helper()
	return c4.Identify(bytes.NewReader([]byte(content)))
}

func TestPrefixedFolderLayout(t *testing.T) {
	dir := t.TempDir()
	s := PrefixedFolder(dir)
	content := "hello world"
	id := testID(t, content)
	idStr := id.String()

	wc, err := s.Create(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(wc, content); err != nil {
		t.Fatal(err)
	}
	if err := wc.Close(); err != nil {
		t.Fatal(err)
	}

	// Verify prefixed directory layout: {base}/{chars 2-3}/{full id}
	expected := filepath.Join(dir, idStr[2:4], idStr)
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected file at %s: %v", expected, err)
	}
}

func TestHasOpenRemove(t *testing.T) {
	dir := t.TempDir()
	s := PrefixedFolder(dir)
	content := "test content"
	id := testID(t, content)

	if s.Has(id) {
		t.Fatal("Has should be false before Create")
	}

	wc, err := s.Create(id)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(wc, content)
	wc.Close()

	if !s.Has(id) {
		t.Fatal("Has should be true after Create")
	}

	rc, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("got %q, want %q", got, content)
	}

	if err := s.Remove(id); err != nil {
		t.Fatal(err)
	}
	if s.Has(id) {
		t.Fatal("Has should be false after Remove")
	}
}

func TestCreateIdempotent(t *testing.T) {
	dir := t.TempDir()
	s := PrefixedFolder(dir)
	content := "idempotent"
	id := testID(t, content)

	// First create.
	wc, err := s.Create(id)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(wc, content)
	wc.Close()

	// Second create with same ID — should succeed without error.
	wc2, err := s.Create(id)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(wc2, "different data")
	if err := wc2.Close(); err != nil {
		t.Fatal(err)
	}

	// Original content should be preserved.
	rc, _ := s.Open(id)
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != content {
		t.Fatalf("idempotent Create overwrote content: got %q", got)
	}
}

func TestImport(t *testing.T) {
	dir := t.TempDir()
	s := PrefixedFolder(dir)
	content := "import test"
	id := testID(t, content)

	// Write a temp file.
	tmp, err := os.CreateTemp(dir, "import-*")
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(tmp, content)
	tmp.Close()

	if err := s.Import(id, tmp.Name()); err != nil {
		t.Fatal(err)
	}
	if !s.Has(id) {
		t.Fatal("Has should be true after Import")
	}

	// Source file should be gone.
	if _, err := os.Stat(tmp.Name()); !os.IsNotExist(err) {
		t.Fatal("source file should have been renamed away")
	}
}

func TestImportIdempotent(t *testing.T) {
	dir := t.TempDir()
	s := PrefixedFolder(dir)
	content := "import idempotent"
	id := testID(t, content)

	// First import.
	tmp1, _ := os.CreateTemp(dir, "imp1-*")
	io.WriteString(tmp1, content)
	tmp1.Close()
	s.Import(id, tmp1.Name())

	// Second import — source should be removed, store unchanged.
	tmp2, _ := os.CreateTemp(dir, "imp2-*")
	io.WriteString(tmp2, "other data")
	tmp2.Close()
	if err := s.Import(id, tmp2.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tmp2.Name()); !os.IsNotExist(err) {
		t.Fatal("duplicate import should have removed source")
	}

	rc, _ := s.Open(id)
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != content {
		t.Fatalf("idempotent Import changed content: got %q", got)
	}
}
