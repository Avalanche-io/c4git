package filter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4git/store"
)

func tmpStore(t *testing.T) store.PrefixedFolder {
	t.Helper()
	return store.PrefixedFolder(t.TempDir())
}

func TestCleanSmudgeRoundTrip(t *testing.T) {
	s := tmpStore(t)
	original := "This is a large media file with some binary content \x00\x01\x02"

	// Clean: content → C4 ID
	var cleanOut bytes.Buffer
	if err := Clean(strings.NewReader(original), &cleanOut, s); err != nil {
		t.Fatal(err)
	}
	idStr := strings.TrimSpace(cleanOut.String())

	// Verify it's a valid C4 ID.
	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("clean output is not a valid C4 ID: %v", err)
	}

	// Verify the ID matches what c4.Identify produces.
	expectedID := c4.Identify(strings.NewReader(original))
	if idStr != expectedID.String() {
		t.Fatalf("ID mismatch:\n  got  %s\n  want %s", idStr, expectedID.String())
	}

	// Smudge: C4 ID → content
	var smudgeOut bytes.Buffer
	if err := Smudge(strings.NewReader(idStr+"\n"), &smudgeOut, s); err != nil {
		t.Fatal(err)
	}

	if smudgeOut.String() != original {
		t.Fatalf("round-trip failed:\n  got  %q\n  want %q", smudgeOut.String(), original)
	}
}

func TestCleanIdempotent(t *testing.T) {
	s := tmpStore(t)
	content := "some content"

	// First clean.
	var out1 bytes.Buffer
	if err := Clean(strings.NewReader(content), &out1, s); err != nil {
		t.Fatal(err)
	}
	id1 := strings.TrimSpace(out1.String())

	// Second clean (re-clean the ID).
	var out2 bytes.Buffer
	if err := Clean(strings.NewReader(id1+"\n"), &out2, s); err != nil {
		t.Fatal(err)
	}
	id2 := strings.TrimSpace(out2.String())

	if id1 != id2 {
		t.Fatalf("clean is not idempotent:\n  first:  %s\n  second: %s", id1, id2)
	}
}

func TestCleanIdempotentBareID(t *testing.T) {
	s := tmpStore(t)
	content := "bare id test"

	var out1 bytes.Buffer
	Clean(strings.NewReader(content), &out1, s)
	id := strings.TrimSpace(out1.String())

	// Re-clean with bare ID (no trailing newline).
	var out2 bytes.Buffer
	if err := Clean(strings.NewReader(id), &out2, s); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out2.String()) != id {
		t.Fatal("bare ID re-clean failed")
	}
}

func TestSmudgeMissingContent(t *testing.T) {
	s := tmpStore(t)
	// A valid C4 ID for content that's not in the store.
	id := c4.Identify(strings.NewReader("not stored"))

	var out bytes.Buffer
	if err := Smudge(strings.NewReader(id.String()+"\n"), &out, s); err != nil {
		t.Fatal(err)
	}
	// Should pass ID through as-is.
	if strings.TrimSpace(out.String()) != id.String() {
		t.Fatal("missing content should pass ID through")
	}
}

func TestSmudgeInvalidInput(t *testing.T) {
	s := tmpStore(t)

	var out bytes.Buffer
	if err := Smudge(strings.NewReader("not a c4 id\n"), &out, s); err != nil {
		t.Fatal(err)
	}
	if out.String() != "not a c4 id\n" {
		t.Fatalf("invalid input should pass through unchanged, got %q", out.String())
	}
}

func TestCleanLargeContent(t *testing.T) {
	s := tmpStore(t)
	// 1MB of data to verify streaming works.
	data := bytes.Repeat([]byte("ABCDEFGHIJ"), 100_000)

	var out bytes.Buffer
	if err := Clean(bytes.NewReader(data), &out, s); err != nil {
		t.Fatal(err)
	}

	idStr := strings.TrimSpace(out.String())
	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("output is not a valid C4 ID: %v", err)
	}

	// Verify round-trip.
	var smudgeOut bytes.Buffer
	if err := Smudge(strings.NewReader(idStr+"\n"), &smudgeOut, s); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(smudgeOut.Bytes(), data) {
		t.Fatal("large content round-trip failed")
	}
}
