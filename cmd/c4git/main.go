package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Avalanche-io/c4git/config"
	"github.com/Avalanche-io/c4git/filter"
	c4store "github.com/Avalanche-io/c4/store"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: c4git <init|clean|smudge|status|verify|gc|version>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "init":
		if err := runInit(); err != nil {
			fmt.Fprintf(os.Stderr, "c4git init: %v\n", err)
			os.Exit(1)
		}
	case "clean":
		if err := runClean(); err != nil {
			fmt.Fprintf(os.Stderr, "c4git clean: %v\n", err)
			os.Exit(1)
		}
	case "smudge":
		if err := runSmudge(); err != nil {
			fmt.Fprintf(os.Stderr, "c4git smudge: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := runStatus(); err != nil {
			fmt.Fprintf(os.Stderr, "c4git status: %v\n", err)
			os.Exit(1)
		}
	case "verify":
		if err := runVerify(); err != nil {
			fmt.Fprintf(os.Stderr, "c4git verify: %v\n", err)
			os.Exit(1)
		}
	case "gc":
		force := len(os.Args) > 2 && os.Args[2] == "--force"
		if err := runGC(force); err != nil {
			fmt.Fprintf(os.Stderr, "c4git gc: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("c4git 1.0.0")
	default:
		fmt.Fprintf(os.Stderr, "c4git: unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}

// openStore returns the content store, preferring the global store.
// The quiet parameter suppresses the fallback warning (used by clean/smudge
// which run on every git operation and shouldn't be noisy).
func openStore() (*c4store.TreeStore, error) {
	return openStoreQuiet(false)
}

func openStoreQuiet(quiet bool) (*c4store.TreeStore, error) {
	// Prefer the user's configured global store (C4_STORE / ~/.c4/config).
	if s, err := c4store.OpenConfigured(); err == nil && s != nil {
		return s, nil
	}
	// Fall back to repo-local .c4/store.
	cfg, err := config.Load(".")
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	s, err := c4store.NewTreeStore(cfg.Stores[0].Path)
	if err != nil {
		return nil, err
	}
	if !quiet {
		fmt.Fprintf(os.Stderr, "c4git: using repo-local store %s\n", cfg.Stores[0].Path)
		fmt.Fprintf(os.Stderr, "       To share across repos: export C4_STORE=~/.c4/store\n")
	}
	return s, nil
}

func runClean() error {
	s, err := openStoreQuiet(true)
	if err != nil {
		return err
	}
	return filter.Clean(os.Stdin, os.Stdout, s)
}

func runSmudge() error {
	s, err := openStoreQuiet(true)
	if err != nil {
		return err
	}
	return filter.Smudge(os.Stdin, os.Stdout, s)
}

func runInit() error {
	// Check that we're inside a git repository.
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("not a git repository (run 'git init' first)")
	}

	cfg := config.Default()
	storePath := cfg.Stores[0].Path

	// 1. Create store directory.
	if err := os.MkdirAll(storePath, 0755); err != nil {
		return fmt.Errorf("creating store: %w", err)
	}

	// 2. Write default config.
	if err := cfg.Write("."); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// 3. Ensure .c4 is in .gitignore.
	if err := ensureGitignore(".gitignore", ".c4"); err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}

	// 4. Write .gitattributes entries.
	if err := ensureGitattributes(".gitattributes", cfg.Patterns); err != nil {
		return fmt.Errorf("updating .gitattributes: %w", err)
	}

	// 5. Configure git filter.
	cmds := [][]string{
		{"git", "config", "filter.c4.clean", "c4git clean %f"},
		{"git", "config", "filter.c4.smudge", "c4git smudge %f"},
		{"git", "config", "filter.c4.required", "true"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git config: %w", err)
		}
	}

	fmt.Println("c4git initialized")
	return nil
}

// ensureGitignore appends entry to path if not already present.
func ensureGitignore(path, entry string) error {
	if hasLine(path, entry) {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Ensure we start on a new line.
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 0 {
		fmt.Fprintln(f)
	}
	fmt.Fprintln(f, entry)
	return nil
}

// ensureGitattributes appends filter=c4 entries for patterns not already present.
func ensureGitattributes(path string, patterns []string) error {
	existing := make(map[string]bool)
	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.Contains(line, "filter=c4") {
				parts := strings.Fields(line)
				if len(parts) > 0 {
					existing[parts[0]] = true
				}
			}
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Ensure we start on a new line if file has content.
	info, err := f.Stat()
	if err != nil {
		return err
	}
	needNewline := info.Size() > 0

	for _, pat := range patterns {
		if existing[pat] {
			continue
		}
		if needNewline {
			fmt.Fprintln(f)
			needNewline = false
		}
		fmt.Fprintf(f, "%s filter=c4\n", pat)
	}
	return nil
}

// hasLine reports whether path contains a line matching s exactly.
func hasLine(path, s string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == s {
			return true
		}
	}
	// On scan error, conservatively return false (entry will be re-added).
	return false
}
