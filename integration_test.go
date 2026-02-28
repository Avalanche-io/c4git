package c4git_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestIntegrationGitCleanSmudge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Build c4git binary.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "c4git")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/c4git")
	build.Dir = projectRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Create a fresh git repo.
	repoDir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	git("init")
	git("config", "user.email", "test@test.com")
	git("config", "user.name", "Test")

	// Run c4git init.
	initCmd := exec.Command(binPath, "init")
	initCmd.Dir = repoDir
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("c4git init: %v\n%s", err, out)
	}

	// Verify init created expected files.
	for _, path := range []string{".c4/store/c4", ".c4git.yaml", ".gitattributes", ".gitignore"} {
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

func projectRoot(t *testing.T) string {
	t.Helper()
	// Find project root by looking for go.mod.
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
