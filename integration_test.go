package c4git_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	c4store "github.com/Avalanche-io/c4/store"
)

// testRepo sets up a fresh git repo with c4git initialized, returning the
// repo directory, a git helper, and a c4git helper.
func testRepo(t *testing.T) (repoDir string, git func(args ...string) string, c4git func(args ...string) string) {
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
	env := append(os.Environ(), "PATH="+binDir+pathSep+os.Getenv("PATH"))

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

	git("init", "-b", "main")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test")
	c4git("init")

	return repoDir, git, c4git
}

func TestIntegrationGitCleanSmudge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, _ := testRepo(t)

	// Verify init created expected files.
	for _, path := range []string{".c4/store", ".c4git.yaml", ".gitattributes", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(repoDir, path)); err != nil {
			t.Fatalf("init did not create %s: %v", path, err)
		}
	}

	// Create a test .exr file (matched by default patterns).
	testContent := []byte("FAKE EXR CONTENT FOR TESTING - this would be a large binary file")
	if err := os.WriteFile(filepath.Join(repoDir, "hero.exr"), testContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Also create a non-filtered file.
	if err := os.WriteFile(filepath.Join(repoDir, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Commit everything.
	git("add", ".gitattributes", ".gitignore", ".c4git.yaml", "hero.exr", "readme.txt")
	git("commit", "-m", "initial commit")

	// Verify that the committed content for hero.exr is a C4 ID.
	blobContent := git("show", "HEAD:hero.exr")
	idStr := strings.TrimSpace(blobContent)
	if _, err := c4.Parse(idStr); err != nil {
		t.Fatalf("committed content is not a valid C4 ID: %q (%v)", idStr, err)
	}

	// Verify the ID matches the original content.
	expectedID := c4.Identify(bytes.NewReader(testContent))
	if idStr != expectedID.String() {
		t.Fatalf("ID mismatch:\n  got  %s\n  want %s", idStr, expectedID.String())
	}

	// Verify the non-filtered file is unchanged.
	readmeBlob := git("show", "HEAD:readme.txt")
	if strings.TrimSpace(readmeBlob) != "hello" {
		t.Fatalf("non-filtered file was modified: %q", readmeBlob)
	}

	// Verify working tree has original content (smudge restored it).
	workingContent, err := os.ReadFile(filepath.Join(repoDir, "hero.exr"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(workingContent, testContent) {
		t.Fatal("working tree content doesn't match original")
	}

	// Force re-checkout to verify smudge works from scratch.
	git("checkout", "--", "hero.exr")
	reCheckout, err := os.ReadFile(filepath.Join(repoDir, "hero.exr"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reCheckout, testContent) {
		t.Fatal("re-checkout content doesn't match original")
	}
}

func TestIntegrationStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, c4git := testRepo(t)

	// Create managed and unmanaged files.
	testContent := []byte("status test content")
	os.WriteFile(filepath.Join(repoDir, "hero.exr"), testContent, 0644)
	os.WriteFile(filepath.Join(repoDir, "readme.txt"), []byte("hello"), 0644)

	git("add", ".gitattributes", ".gitignore", ".c4git.yaml", "hero.exr", "readme.txt")
	git("commit", "-m", "initial commit")

	// Run status — should show hero.exr as managed, ok.
	out := c4git("status")
	if !strings.Contains(out, "hero.exr") {
		t.Fatalf("status should list hero.exr, got:\n%s", out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("status should show 'ok' for hero.exr, got:\n%s", out)
	}
	if strings.Contains(out, "readme.txt") {
		t.Fatalf("status should not list readme.txt, got:\n%s", out)
	}
	if !strings.Contains(out, "1 managed files") {
		t.Fatalf("status should report 1 managed file, got:\n%s", out)
	}
}

func TestIntegrationVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, c4git := testRepo(t)

	testContent := []byte("verify test content")
	os.WriteFile(filepath.Join(repoDir, "hero.exr"), testContent, 0644)
	git("add", ".gitattributes", ".gitignore", ".c4git.yaml", "hero.exr")
	git("commit", "-m", "initial commit")

	// Verify should report "ok".
	out := c4git("verify")
	if !strings.Contains(out, "ok") {
		t.Fatalf("verify should report ok, got:\n%s", out)
	}

	// Modify the working tree file.
	os.WriteFile(filepath.Join(repoDir, "hero.exr"), []byte("modified content"), 0644)
	out = c4git("verify")
	if !strings.Contains(out, "modified") {
		t.Fatalf("verify should report modified, got:\n%s", out)
	}

	// Delete the file.
	os.Remove(filepath.Join(repoDir, "hero.exr"))
	out = c4git("verify")
	if !strings.Contains(out, "missing") {
		t.Fatalf("verify should report missing, got:\n%s", out)
	}
}

func TestIntegrationGC(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, c4git := testRepo(t)

	// Create and commit a managed file.
	testContent := []byte("gc test content")
	os.WriteFile(filepath.Join(repoDir, "hero.exr"), testContent, 0644)
	git("add", ".gitattributes", ".gitignore", ".c4git.yaml", "hero.exr")
	git("commit", "-m", "initial commit")

	// Add an unreferenced object directly to the store using TreeStore.
	unreferencedContent := []byte("orphaned content that nobody references")
	unreferencedID := c4.Identify(bytes.NewReader(unreferencedContent))
	storePath := filepath.Join(repoDir, ".c4", "store")
	s, err := c4store.NewTreeStore(storePath)
	if err != nil {
		t.Fatal(err)
	}
	wc, err := s.Create(unreferencedID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(wc, bytes.NewReader(unreferencedContent)); err != nil {
		wc.Close()
		t.Fatal(err)
	}
	if err := wc.Close(); err != nil {
		t.Fatal(err)
	}

	// Dry run — should identify 1 unreferenced object.
	out := c4git("gc")
	if !strings.Contains(out, "Would remove 1 unreferenced") {
		t.Fatalf("gc dry run should find 1 unreferenced object, got:\n%s", out)
	}

	// Verify the unreferenced object still exists.
	if !s.Has(unreferencedID) {
		t.Fatal("unreferenced object should still exist after dry run")
	}

	// Force — should actually remove.
	out = c4git("gc", "--force")
	if !strings.Contains(out, "Removed 1 unreferenced") {
		t.Fatalf("gc --force should remove 1 object, got:\n%s", out)
	}

	// Verify it's gone.
	if s.Has(unreferencedID) {
		t.Fatal("unreferenced object should be removed after gc --force")
	}

	// Verify the referenced object is still there.
	referencedID := c4.Identify(bytes.NewReader(testContent))
	if !s.Has(referencedID) {
		t.Fatal("referenced object should still exist after gc")
	}
}

func TestIntegrationGCSafety(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	repoDir, git, c4git := testRepo(t)

	// Initial commit with config files.
	git("add", ".gitattributes", ".gitignore", ".c4git.yaml")
	git("commit", "-m", "initial commit")

	// Stage a managed file but don't commit.
	testContent := []byte("staged but uncommitted content")
	os.WriteFile(filepath.Join(repoDir, "staged.exr"), testContent, 0644)
	git("add", "staged.exr")

	// GC should NOT consider this unreferenced.
	out := c4git("gc")
	if strings.Contains(out, "Would remove") {
		t.Fatalf("gc should not mark staged file as unreferenced, got:\n%s", out)
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root")
		}
		dir = parent
	}
}
