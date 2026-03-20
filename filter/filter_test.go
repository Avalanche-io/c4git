package filter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	c4store "github.com/Avalanche-io/c4/store"
)

func tmpStore(t *testing.T) *c4store.TreeStore {
	t.Helper()
	s, err := c4store.NewTreeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCleanProducesExactly90Bytes(t *testing.T) {
	s := tmpStore(t)

	var out bytes.Buffer
	if err := Clean(strings.NewReader("hello world"), &out, s); err != nil {
		t.Fatal(err)
	}

	if out.Len() != 90 {
		t.Fatalf("clean output is %d bytes, want exactly 90", out.Len())
	}
	if _, err := c4.Parse(out.String()); err != nil {
		t.Fatalf("clean output is not a valid C4 ID: %v", err)
	}
}

func TestCleanSmudgeRoundTrip(t *testing.T) {
	s := tmpStore(t)
	original := "This is a large media file with some binary content \x00\x01\x02"

	// Clean: content → bare C4 ID (90 bytes)
	var cleanOut bytes.Buffer
	if err := Clean(strings.NewReader(original), &cleanOut, s); err != nil {
		t.Fatal(err)
	}
	idStr := cleanOut.String()

	if len(idStr) != 90 {
		t.Fatalf("clean output is %d bytes, want 90", len(idStr))
	}

	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("clean output is not a valid C4 ID: %v", err)
	}

	// Verify the ID matches what c4.Identify produces.
	expectedID := c4.Identify(strings.NewReader(original))
	if idStr != expectedID.String() {
		t.Fatalf("ID mismatch:\n  got  %s\n  want %s", idStr, expectedID.String())
	}

	// Smudge: C4 ID → content (accepts bare ID)
	var smudgeOut bytes.Buffer
	if err := Smudge(strings.NewReader(idStr), &smudgeOut, s); err != nil {
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
	id1 := out1.String()

	// Second clean (re-clean the bare ID).
	var out2 bytes.Buffer
	if err := Clean(strings.NewReader(id1), &out2, s); err != nil {
		t.Fatal(err)
	}
	id2 := out2.String()

	if id1 != id2 {
		t.Fatalf("clean is not idempotent:\n  first:  %s\n  second: %s", id1, id2)
	}
}

func TestCleanIdempotentWithNewline(t *testing.T) {
	s := tmpStore(t)
	content := "newline test"

	var out1 bytes.Buffer
	Clean(strings.NewReader(content), &out1, s)
	id := out1.String()

	// Re-clean with trailing newline (git may add one).
	var out2 bytes.Buffer
	if err := Clean(strings.NewReader(id+"\n"), &out2, s); err != nil {
		t.Fatal(err)
	}
	if out2.String() != id {
		t.Fatalf("re-clean with newline changed ID:\n  got  %q\n  want %q", out2.String(), id)
	}
}

func TestSmudgeMissingContent(t *testing.T) {
	s := tmpStore(t)
	id := c4.Identify(strings.NewReader("not stored"))

	var out bytes.Buffer
	if err := Smudge(strings.NewReader(id.String()), &out, s); err != nil {
		t.Fatal(err)
	}
	// Should pass bare ID through (90 bytes, no newline).
	if out.String() != id.String() {
		t.Fatalf("missing content should pass bare ID through, got %q", out.String())
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
	data := bytes.Repeat([]byte("ABCDEFGHIJ"), 100_000)

	var out bytes.Buffer
	if err := Clean(bytes.NewReader(data), &out, s); err != nil {
		t.Fatal(err)
	}

	if out.Len() != 90 {
		t.Fatalf("clean output is %d bytes, want 90", out.Len())
	}

	idStr := out.String()
	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("output is not a valid C4 ID: %v", err)
	}

	// Verify round-trip.
	var smudgeOut bytes.Buffer
	if err := Smudge(strings.NewReader(idStr), &smudgeOut, s); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(smudgeOut.Bytes(), data) {
		t.Fatal("large content round-trip failed")
	}
}
