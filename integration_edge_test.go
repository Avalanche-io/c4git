package c4git_test

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4git/filter"
	c4store "github.com/Avalanche-io/c4/store"
)

// testRepoWithErr is like testRepo but also returns a gitErr helper that
// returns error instead of calling t.Fatal.
func testRepoWithErr(t *testing.T) (
	repoDir string,
	git func(args ...string) string,
	c4git func(args ...string) string,
	c4gitErr func(args ...string) (string, error),
) {
	t.Helper()

	binDir := t.TempDir()
	binName := "c4git"
	if runtime.GOOS == "windows" {
		binName = "c4git.exe"
	}
	binPath := filepath.Join(binDir, binName)
	build := exec.Command("go", "build", "-o", binPath, "./cmd/c4git")
	build.Dir = projectRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	repoDir = t.TempDir()
	pathSep := ":"
	if runtime.GOOS == "windows" {
		pathSep = ";"
	}
	// Ensure tests use repo-local store, not any global C4_STORE config.
	env := append(os.Environ(),
		"PATH="+binDir+pathSep+os.Getenv("PATH"),
		"C4_STORE=",
		"HOME="+t.TempDir(),
	)

	git = func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	c4git = func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("c4git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	c4gitErr = func(args ...string) (string, error) {
		cmd := exec.Command(binPath, args...)
		cmd.Dir = repoDir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	git("init", "-b", "main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test")
	c4git("init")

	return repoDir, git, c4git, c4gitErr
}

// writeFile is a helper that writes content to a file in repoDir.
func writeFile(t *testing.T, dir, name string, content []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), content, 0644); err != nil {
		t.Fatal(err)
	}
}

// readFile is a helper that reads a file from repoDir.
func readFile(t *testing.T, dir, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// commitAll stages given files plus the standard config files, and commits.
func commitAll(t *testing.T, git func(...string) string, files ...string) {
	t.Helper()
	args := append([]string{"add", ".gitattributes", ".gitignore", ".c4git.yaml"}, files...)
	git(args...)
	git("commit", "-m", "commit")
}

// TestBasicRoundTrip tests: init repo, add file, commit, verify working tree
// has real content, verify git blob has 90-byte C4 ID.
func TestBasicRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	content := []byte("This is a test file with some real content for round-trip testing.")
	writeFile(t, repoDir, "test.exr", content)
	commitAll(t, git, "test.exr")

	// Verify git blob is a 90-byte C4 ID.
	blobContent := git("show", "HEAD:test.exr")
	idStr := strings.TrimSpace(blobContent)
	if len(idStr) != 90 {
		t.Fatalf("blob content length = %d, want 90", len(idStr))
	}
	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("blob is not a valid C4 ID: %q (%v)", idStr, err)
	}

	// Verify the ID matches the expected C4 hash.
	expected := c4.Identify(bytes.NewReader(content))
	if idStr != expected.String() {
		t.Fatalf("ID mismatch: got %s, want %s", idStr, expected.String())
	}

	// Verify working tree has original content.
	got := readFile(t, repoDir, "test.exr")
	if !bytes.Equal(got, content) {
		t.Fatal("working tree content does not match original")
	}

	// Force re-checkout to verify smudge works from scratch.
	git("checkout", "--", "test.exr")
	got = readFile(t, repoDir, "test.exr")
	if !bytes.Equal(got, content) {
		t.Fatal("re-checkout content does not match original")
	}
}

// TestBinaryContent tests that null bytes and arbitrary binary data survive
// the clean/smudge round-trip exactly.
func TestBinaryContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	// Build binary content with null bytes, high bytes, and every byte value.
	var content bytes.Buffer
	content.WriteString("BINARY HEADER\x00\x00")
	for i := 0; i < 256; i++ {
		content.WriteByte(byte(i))
	}
	// Add more random-ish binary data.
	content.Write(bytes.Repeat([]byte{0x00, 0xFF, 0x7F, 0x80}, 100))

	writeFile(t, repoDir, "binary.exr", content.Bytes())
	commitAll(t, git, "binary.exr")

	// Verify blob is a C4 ID.
	blobContent := git("show", "HEAD:binary.exr")
	idStr := strings.TrimSpace(blobContent)
	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("blob is not a valid C4 ID: %v", err)
	}

	// Verify working tree content is byte-for-byte identical.
	got := readFile(t, repoDir, "binary.exr")
	if !bytes.Equal(got, content.Bytes()) {
		t.Fatalf("binary round-trip failed: got %d bytes, want %d bytes", len(got), content.Len())
	}

	// Force re-checkout and re-verify.
	git("checkout", "--", "binary.exr")
	got = readFile(t, repoDir, "binary.exr")
	if !bytes.Equal(got, content.Bytes()) {
		t.Fatal("binary re-checkout content does not match original")
	}
}

// TestEmptyFile tests what happens when an empty file goes through the filter.
func TestEmptyFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	writeFile(t, repoDir, "empty.exr", []byte{})
	commitAll(t, git, "empty.exr")

	// Verify blob is a valid C4 ID (the C4 of empty content).
	blobContent := git("show", "HEAD:empty.exr")
	idStr := strings.TrimSpace(blobContent)
	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("empty file blob is not a valid C4 ID: %v", err)
	}

	// Verify the ID matches c4.Identify of empty content.
	expected := c4.Identify(bytes.NewReader([]byte{}))
	if idStr != expected.String() {
		t.Fatalf("empty file ID mismatch: got %s, want %s", idStr, expected.String())
	}

	// Working tree should have empty content restored.
	got := readFile(t, repoDir, "empty.exr")
	if len(got) != 0 {
		t.Fatalf("empty file should be empty after checkout, got %d bytes", len(got))
	}
}

// TestFileExactly90Bytes tests that a file whose real content happens to be
// exactly 90 bytes (the same length as a C4 ID) is not misidentified as a
// C4 ID by the clean filter.
func TestFileExactly90Bytes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	// 90 bytes of content that is NOT a valid C4 ID.
	content := bytes.Repeat([]byte("ABCDEFGHIJ"), 9) // 90 bytes
	if len(content) != 90 {
		t.Fatalf("test setup error: content is %d bytes, want 90", len(content))
	}
	writeFile(t, repoDir, "exact90.exr", content)
	commitAll(t, git, "exact90.exr")

	// Verify blob is a C4 ID (the content was stored, not passed through).
	blobContent := git("show", "HEAD:exact90.exr")
	idStr := strings.TrimSpace(blobContent)
	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("blob is not a valid C4 ID: %v", err)
	}

	// The blob should NOT be the raw content itself.
	if idStr == string(content) {
		t.Fatal("clean filter passed 90-byte content through as-is instead of storing it")
	}

	// Verify the ID matches c4.Identify of the original content.
	expected := c4.Identify(bytes.NewReader(content))
	if idStr != expected.String() {
		t.Fatalf("ID mismatch: got %s, want %s", idStr, expected.String())
	}

	// Verify working tree has original content.
	got := readFile(t, repoDir, "exact90.exr")
	if !bytes.Equal(got, content) {
		t.Fatal("90-byte content not round-tripped correctly")
	}
}

// TestContentStartingWithC4 tests that content starting with "c4" but not
// being a valid C4 ID does not confuse the clean filter.
func TestContentStartingWithC4(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	// Various tricky inputs that start with "c4" but aren't valid C4 IDs.
	cases := []struct {
		name    string
		content []byte
	}{
		{"short_c4", []byte("c4 this is not an ID but starts with c4")},
		{"c4_with_garbage", append([]byte("c4"), bytes.Repeat([]byte("X"), 88)...)}, // 90 bytes starting with c4
		{"c4_prefix_long", append([]byte("c4"), bytes.Repeat([]byte("Z"), 200)...)}, // >90 bytes
	}

	for _, tc := range cases {
		writeFile(t, repoDir, tc.name+".exr", tc.content)
	}

	var fileArgs []string
	for _, tc := range cases {
		fileArgs = append(fileArgs, tc.name+".exr")
	}
	commitAll(t, git, fileArgs...)

	for _, tc := range cases {
		// Verify blob is a valid C4 ID.
		blobContent := git("show", "HEAD:"+tc.name+".exr")
		idStr := strings.TrimSpace(blobContent)
		if _, err := c4.Parse(idStr); err != nil {
			t.Fatalf("%s: blob is not a valid C4 ID: %v", tc.name, err)
		}

		// Verify round-trip.
		got := readFile(t, repoDir, tc.name+".exr")
		if !bytes.Equal(got, tc.content) {
			t.Fatalf("%s: content not round-tripped correctly", tc.name)
		}
	}
}

// TestLargeFile tests a 10MB+ file through the clean/smudge filter.
func TestLargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	// Generate 10MB of deterministic content (not random, so test is reproducible).
	const size = 10 * 1024 * 1024 // 10 MB
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 251) // prime modulus for variety
	}

	writeFile(t, repoDir, "large.exr", content)
	commitAll(t, git, "large.exr")

	// Verify blob is a C4 ID.
	blobContent := git("show", "HEAD:large.exr")
	idStr := strings.TrimSpace(blobContent)
	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("blob is not a valid C4 ID: %v", err)
	}

	// Verify the ID is correct.
	expected := c4.Identify(bytes.NewReader(content))
	if idStr != expected.String() {
		t.Fatalf("ID mismatch for large file")
	}

	// Verify working tree round-trip.
	got := readFile(t, repoDir, "large.exr")
	if !bytes.Equal(got, content) {
		t.Fatal("large file round-trip failed")
	}

	// Force re-checkout.
	git("checkout", "--", "large.exr")
	got = readFile(t, repoDir, "large.exr")
	if !bytes.Equal(got, content) {
		t.Fatal("large file re-checkout failed")
	}
}

// TestMultipleFiles tests adding several files, committing, and verifying
// all are stored and recoverable.
func TestMultipleFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, c4git, _ := testRepoWithErr(t)

	files := map[string][]byte{
		"alpha.exr":  []byte("content of alpha"),
		"beta.exr":   []byte("content of beta"),
		"gamma.dpx":  []byte("content of gamma"),
		"delta.mov":  []byte("content of delta"),
		"epsilon.mp4": []byte("content of epsilon"),
	}

	var names []string
	for name, content := range files {
		writeFile(t, repoDir, name, content)
		names = append(names, name)
	}
	commitAll(t, git, names...)

	// Verify all blobs are C4 IDs.
	for name, content := range files {
		blobContent := git("show", "HEAD:"+name)
		idStr := strings.TrimSpace(blobContent)
		if _, err := c4.Parse(idStr); err != nil {
			t.Fatalf("%s: blob is not a valid C4 ID: %v", name, err)
		}
		expected := c4.Identify(bytes.NewReader(content))
		if idStr != expected.String() {
			t.Fatalf("%s: ID mismatch", name)
		}
	}

	// Verify all working tree files have correct content.
	for name, content := range files {
		got := readFile(t, repoDir, name)
		if !bytes.Equal(got, content) {
			t.Fatalf("%s: working tree content mismatch", name)
		}
	}

	// Verify status reports correct count.
	out := c4git("status")
	if !strings.Contains(out, "5 managed files") {
		t.Fatalf("status should report 5 managed files, got:\n%s", out)
	}
}

// TestModifyAndRecommit tests changing a file's content, committing again,
// and verifying both versions are in the store.
func TestModifyAndRecommit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	contentV1 := []byte("version 1 of the file")
	writeFile(t, repoDir, "evolving.exr", contentV1)
	commitAll(t, git, "evolving.exr")

	// Record v1 ID.
	blobV1 := strings.TrimSpace(git("show", "HEAD:evolving.exr"))
	idV1, err := c4.Parse(blobV1)
	if err != nil {
		t.Fatalf("v1 blob is not a valid C4 ID: %v", err)
	}

	// Modify and recommit.
	contentV2 := []byte("version 2 of the file with different content")
	writeFile(t, repoDir, "evolving.exr", contentV2)
	git("add", "evolving.exr")
	git("commit", "-m", "update evolving.exr")

	// Record v2 ID.
	blobV2 := strings.TrimSpace(git("show", "HEAD:evolving.exr"))
	idV2, err := c4.Parse(blobV2)
	if err != nil {
		t.Fatalf("v2 blob is not a valid C4 ID: %v", err)
	}

	// IDs should differ.
	if idV1 == idV2 {
		t.Fatal("v1 and v2 should have different C4 IDs")
	}

	// Both versions should exist in the store.
	storePath := filepath.Join(repoDir, ".c4", "store")
	s, err := c4store.NewTreeStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Has(idV1) {
		t.Fatal("store should still contain v1 after recommit")
	}
	if !s.Has(idV2) {
		t.Fatal("store should contain v2 after recommit")
	}

	// Working tree should have v2 content.
	got := readFile(t, repoDir, "evolving.exr")
	if !bytes.Equal(got, contentV2) {
		t.Fatal("working tree should have v2 content")
	}

	// Checkout v1 commit and verify v1 content is restored.
	git("checkout", "HEAD~1", "--", "evolving.exr")
	got = readFile(t, repoDir, "evolving.exr")
	if !bytes.Equal(got, contentV1) {
		t.Fatal("checking out v1 should restore v1 content")
	}
}

// TestBranchAndCheckout tests creating branches with different file content,
// switching between them, and verifying correct content is restored.
func TestBranchAndCheckout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	// Initial commit with a file on main.
	contentMain := []byte("main branch content")
	writeFile(t, repoDir, "scene.exr", contentMain)
	commitAll(t, git, "scene.exr")

	// Create a feature branch with different content.
	git("checkout", "-b", "feature")
	contentFeature := []byte("feature branch content with more detail")
	writeFile(t, repoDir, "scene.exr", contentFeature)
	git("add", "scene.exr")
	git("commit", "-m", "feature update")

	// Also add a new file only on feature branch.
	contentExtra := []byte("extra file only on feature branch")
	writeFile(t, repoDir, "extra.exr", contentExtra)
	git("add", "extra.exr")
	git("commit", "-m", "add extra file")

	// Verify feature branch content.
	got := readFile(t, repoDir, "scene.exr")
	if !bytes.Equal(got, contentFeature) {
		t.Fatal("feature branch should have feature content")
	}
	got = readFile(t, repoDir, "extra.exr")
	if !bytes.Equal(got, contentExtra) {
		t.Fatal("feature branch should have extra file")
	}

	// Switch to main branch.
	git("checkout", "-")
	got = readFile(t, repoDir, "scene.exr")
	if !bytes.Equal(got, contentMain) {
		t.Fatal("main branch should have main content after checkout")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "extra.exr")); err == nil {
		t.Fatal("extra.exr should not exist on main branch")
	}

	// Switch back to feature.
	git("checkout", "feature")
	got = readFile(t, repoDir, "scene.exr")
	if !bytes.Equal(got, contentFeature) {
		t.Fatal("feature branch should have feature content after re-checkout")
	}
	got = readFile(t, repoDir, "extra.exr")
	if !bytes.Equal(got, contentExtra) {
		t.Fatal("extra.exr should be restored on feature branch")
	}
}

// TestGCAfterBranchDelete tests that gc identifies unreferenced content
// after deleting a branch.
func TestGCAfterBranchDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, c4git, _ := testRepoWithErr(t)

	// Initial commit on main.
	contentMain := []byte("main content that stays")
	writeFile(t, repoDir, "keep.exr", contentMain)
	commitAll(t, git, "keep.exr")

	// Create a branch with unique content.
	git("checkout", "-b", "ephemeral")
	contentEphemeral := []byte("ephemeral content that should be GC'd")
	writeFile(t, repoDir, "temp.exr", contentEphemeral)
	git("add", "temp.exr")
	git("commit", "-m", "add temp file")

	// Both IDs should be in store now.
	ephemeralID := c4.Identify(bytes.NewReader(contentEphemeral))
	storePath := filepath.Join(repoDir, ".c4", "store")
	s, err := c4store.NewTreeStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Has(ephemeralID) {
		t.Fatal("store should contain ephemeral content")
	}

	// Switch back to main and delete the branch.
	git("checkout", "-")
	git("branch", "-D", "ephemeral")

	// Expire reflog so git truly forgets the branch.
	git("reflog", "expire", "--expire=now", "--all")

	// GC dry run should find unreferenced objects.
	out := c4git("gc")
	if !strings.Contains(out, "Would remove") {
		t.Fatalf("gc should find unreferenced objects after branch delete, got:\n%s", out)
	}

	// GC force should remove them.
	out = c4git("gc", "--force")
	if !strings.Contains(out, "Removed") {
		t.Fatalf("gc --force should remove unreferenced objects, got:\n%s", out)
	}

	// Ephemeral content should be gone.
	if s.Has(ephemeralID) {
		t.Fatal("ephemeral content should be removed after gc")
	}

	// Main content should still be there.
	mainID := c4.Identify(bytes.NewReader(contentMain))
	if !s.Has(mainID) {
		t.Fatal("main content should still exist after gc")
	}
}

// TestStatusManagedFileCount verifies status reports the correct number of
// managed files after init and several commits.
func TestStatusManagedFileCount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, c4git, _ := testRepoWithErr(t)

	// Initially no managed files (just config).
	git("add", ".gitattributes", ".gitignore", ".c4git.yaml")
	git("commit", "-m", "init")
	out := c4git("status")
	if !strings.Contains(out, "No managed files") {
		t.Fatalf("status should show no managed files, got:\n%s", out)
	}

	// Add one managed file.
	writeFile(t, repoDir, "one.exr", []byte("one"))
	git("add", "one.exr")
	git("commit", "-m", "add one")
	out = c4git("status")
	if !strings.Contains(out, "1 managed files") {
		t.Fatalf("status should report 1, got:\n%s", out)
	}

	// Add two more.
	writeFile(t, repoDir, "two.exr", []byte("two"))
	writeFile(t, repoDir, "three.dpx", []byte("three"))
	git("add", "two.exr", "three.dpx")
	git("commit", "-m", "add two and three")
	out = c4git("status")
	if !strings.Contains(out, "3 managed files") {
		t.Fatalf("status should report 3, got:\n%s", out)
	}

	// A non-managed file should not affect the count.
	writeFile(t, repoDir, "readme.txt", []byte("not managed"))
	git("add", "readme.txt")
	git("commit", "-m", "add readme")
	out = c4git("status")
	if !strings.Contains(out, "3 managed files") {
		t.Fatalf("status should still report 3, got:\n%s", out)
	}
}

// TestVerifyIntegrity tests that verify detects ok, modified, and missing
// states for managed files.
func TestVerifyIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, c4git, _ := testRepoWithErr(t)

	contentA := []byte("file A for verification")
	contentB := []byte("file B for verification")
	writeFile(t, repoDir, "a.exr", contentA)
	writeFile(t, repoDir, "b.exr", contentB)
	commitAll(t, git, "a.exr", "b.exr")

	// All should be ok initially.
	out := c4git("verify")
	if !strings.Contains(out, "2 ok") {
		t.Fatalf("verify should report 2 ok, got:\n%s", out)
	}
	if strings.Contains(out, "modified") {
		t.Fatalf("verify should not report modified initially, got:\n%s", out)
	}

	// Modify a.exr in the working tree.
	writeFile(t, repoDir, "a.exr", []byte("tampered content"))
	out = c4git("verify")
	if !strings.Contains(out, "modified") {
		t.Fatalf("verify should detect modification, got:\n%s", out)
	}
	if !strings.Contains(out, "1 ok") {
		t.Fatalf("verify should report 1 ok for unmodified file, got:\n%s", out)
	}

	// Delete b.exr.
	os.Remove(filepath.Join(repoDir, "b.exr"))
	out = c4git("verify")
	if !strings.Contains(out, "missing") {
		t.Fatalf("verify should detect missing file, got:\n%s", out)
	}

	// Corrupt the store by removing the backing content for a.exr.
	// First restore a.exr to its original content so verify checks the store.
	writeFile(t, repoDir, "a.exr", contentA)
	out = c4git("verify")
	// a.exr should be ok since content matches.
	if !strings.Contains(out, "ok") {
		t.Fatalf("verify should show ok after restoring content, got:\n%s", out)
	}

	// Now corrupt the store: remove the actual stored content for A.
	idA := c4.Identify(bytes.NewReader(contentA))
	storePath := filepath.Join(repoDir, ".c4", "store")
	s, err := c4store.NewTreeStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(idA); err != nil {
		t.Fatal(err)
	}

	// Tamper with a.exr so it mismatches.
	writeFile(t, repoDir, "a.exr", []byte("different content now"))
	out = c4git("verify")
	if !strings.Contains(out, "missing") {
		t.Fatalf("verify should detect missing from store, got:\n%s", out)
	}
}

// TestConcurrentClean tests that two files being cleaned simultaneously
// don't interfere with each other (git may parallelize filter operations).
func TestConcurrentClean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	// Create multiple files so git might parallelize the clean filter.
	const fileCount = 20
	contents := make(map[string][]byte, fileCount)
	var names []string
	for i := 0; i < fileCount; i++ {
		name := filepath.Join(repoDir, strings.Replace("file_NNN.exr", "NNN", strings.Repeat("x", i+1), 1))
		// Use index-based name for uniqueness.
		baseName := "file_" + string(rune('a'+i)) + ".exr"
		if i >= 26 {
			baseName = "file_" + string(rune('A'+i-26)) + ".exr"
		}
		name = baseName
		data := make([]byte, 1024+i*512) // varying sizes
		for j := range data {
			data[j] = byte((i*17 + j*31) % 256)
		}
		contents[name] = data
		writeFile(t, repoDir, name, data)
		names = append(names, name)
	}

	commitAll(t, git, names...)

	// Verify all files round-tripped correctly.
	for name, expected := range contents {
		got := readFile(t, repoDir, name)
		if !bytes.Equal(got, expected) {
			t.Fatalf("%s: content mismatch after concurrent commit", name)
		}

		blobContent := git("show", "HEAD:"+name)
		idStr := strings.TrimSpace(blobContent)
		if _, err := c4.Parse(idStr); err != nil {
			t.Fatalf("%s: blob is not a valid C4 ID: %v", name, err)
		}

		expectedID := c4.Identify(bytes.NewReader(expected))
		if idStr != expectedID.String() {
			t.Fatalf("%s: ID mismatch", name)
		}
	}
}

// TestConcurrentFilterDirect tests concurrent clean/smudge operations directly
// against the store using goroutines.
func TestConcurrentFilterDirect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	storePath := t.TempDir()
	s, err := c4store.NewTreeStore(storePath)
	if err != nil {
		t.Fatal(err)
	}

	// Generate unique content for each goroutine.
	const goroutines = 20
	inputs := make([][]byte, goroutines)
	for i := range inputs {
		inputs[i] = make([]byte, 4096)
		rand.Read(inputs[i])
	}

	// Clean all concurrently.
	ids := make([]string, goroutines)
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var out bytes.Buffer
			err := cleanFn(bytes.NewReader(inputs[idx]), &out, s)
			if err != nil {
				errs[idx] = err
				return
			}
			ids[idx] = out.String()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d clean error: %v", i, err)
		}
	}

	// Smudge all concurrently and verify content.
	results := make([][]byte, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var out bytes.Buffer
			err := smudgeFn(strings.NewReader(ids[idx]), &out, s)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = out.Bytes()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d smudge error: %v", i, err)
		}
	}

	for i := range inputs {
		if !bytes.Equal(results[i], inputs[i]) {
			t.Fatalf("goroutine %d: content mismatch after concurrent round-trip", i)
		}
	}
}

// TestDuplicateContent tests that two files with identical content share
// the same store entry.
func TestDuplicateContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	content := []byte("identical content shared between files")
	writeFile(t, repoDir, "copy1.exr", content)
	writeFile(t, repoDir, "copy2.exr", content)
	commitAll(t, git, "copy1.exr", "copy2.exr")

	// Both blobs should have the same C4 ID.
	id1 := strings.TrimSpace(git("show", "HEAD:copy1.exr"))
	id2 := strings.TrimSpace(git("show", "HEAD:copy2.exr"))
	if id1 != id2 {
		t.Fatalf("duplicate files should have same C4 ID:\n  copy1: %s\n  copy2: %s", id1, id2)
	}

	// Both working tree files should have correct content.
	got1 := readFile(t, repoDir, "copy1.exr")
	got2 := readFile(t, repoDir, "copy2.exr")
	if !bytes.Equal(got1, content) || !bytes.Equal(got2, content) {
		t.Fatal("duplicate content not restored correctly")
	}
}

// TestNonFilteredFilesUntouched verifies that files not matching the
// gitattributes patterns are never touched by the filter.
func TestNonFilteredFilesUntouched(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	// These extensions should NOT be filtered.
	untouched := map[string][]byte{
		"script.py":   []byte("print('hello')"),
		"notes.txt":   []byte("some notes"),
		"code.go":     []byte("package main"),
		"data.json":   []byte(`{"key": "value"}`),
		"Makefile":    []byte("all:\n\techo ok"),
	}

	// This one should be filtered.
	filtered := map[string][]byte{
		"scene.exr": []byte("EXR binary data here"),
	}

	var names []string
	for name, content := range untouched {
		writeFile(t, repoDir, name, content)
		names = append(names, name)
	}
	for name, content := range filtered {
		writeFile(t, repoDir, name, content)
		names = append(names, name)
	}
	commitAll(t, git, names...)

	// Untouched files should be stored as-is in git.
	for name, content := range untouched {
		blobContent := git("show", "HEAD:"+name)
		// The blob should be the raw content (no C4 ID transformation).
		if strings.TrimRight(blobContent, "\n") != strings.TrimRight(string(content), "\n") {
			// For binary-safe comparison, just check it's NOT a C4 ID.
			idStr := strings.TrimSpace(blobContent)
			if _, err := c4.Parse(idStr); err == nil {
				t.Fatalf("%s: non-filtered file should not be a C4 ID", name)
			}
		}
	}

	// Filtered file should be a C4 ID.
	for name := range filtered {
		blobContent := git("show", "HEAD:"+name)
		idStr := strings.TrimSpace(blobContent)
		if _, err := c4.Parse(idStr); err != nil {
			t.Fatalf("%s: filtered file should be a C4 ID: %v", name, err)
		}
	}
}

// TestGitDiff verifies that git diff works correctly with the filter.
func TestGitDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	contentV1 := []byte("version 1 content for diff test")
	writeFile(t, repoDir, "diff.exr", contentV1)
	commitAll(t, git, "diff.exr")

	// Modify the file.
	contentV2 := []byte("version 2 content for diff test (modified)")
	writeFile(t, repoDir, "diff.exr", contentV2)

	// git diff should show something (the working tree has different content).
	out := git("diff", "--name-only")
	if !strings.Contains(out, "diff.exr") {
		t.Fatalf("git diff should show diff.exr as modified, got:\n%s", out)
	}

	// Stage and commit the change.
	git("add", "diff.exr")
	git("commit", "-m", "update diff file")

	// After commit, diff should be clean.
	out = git("diff", "--name-only")
	if strings.TrimSpace(out) != "" {
		t.Fatalf("git diff should be clean after commit, got:\n%s", out)
	}
}

// TestInitIdempotent verifies that running c4git init twice doesn't corrupt
// anything or duplicate .gitattributes entries.
func TestInitIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, _, c4git, _ := testRepoWithErr(t)

	// Read .gitattributes after first init.
	attrs1 := readFile(t, repoDir, ".gitattributes")

	// Run init again.
	c4git("init")

	// .gitattributes should not have duplicated entries.
	attrs2 := readFile(t, repoDir, ".gitattributes")
	if !bytes.Equal(attrs1, attrs2) {
		t.Fatalf("second init modified .gitattributes:\nbefore:\n%s\nafter:\n%s", attrs1, attrs2)
	}
}

// TestRandomBinaryData tests random binary content to catch any byte-level issues.
func TestRandomBinaryData(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _, _ := testRepoWithErr(t)

	// Generate truly random content.
	content := make([]byte, 8192)
	if _, err := io.ReadFull(rand.Reader, content); err != nil {
		t.Fatal(err)
	}

	writeFile(t, repoDir, "random.exr", content)
	commitAll(t, git, "random.exr")

	// Verify round-trip.
	got := readFile(t, repoDir, "random.exr")
	if !bytes.Equal(got, content) {
		t.Fatal("random binary data round-trip failed")
	}

	// Force re-checkout.
	git("checkout", "--", "random.exr")
	got = readFile(t, repoDir, "random.exr")
	if !bytes.Equal(got, content) {
		t.Fatal("random binary data re-checkout failed")
	}
}

// cleanFn and smudgeFn are wrappers to call the filter package directly
// for the concurrent test without needing to export from cmd/c4git.
func cleanFn(r io.Reader, w io.Writer, s *c4store.TreeStore) error {
	return filter.Clean(r, w, s)
}

func smudgeFn(r io.Reader, w io.Writer, s *c4store.TreeStore) error {
	return filter.Smudge(r, w, s)
}
