package main

import (
	"fmt"
	"os"

	"github.com/Avalanche-io/c4"
)

func runVerify() error {
	s, err := openStore()
	if err != nil {
		return err
	}

	files, err := managedFiles()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("No managed files.")
		return nil
	}

	var ok, modified, notRestored, missing int
	for _, f := range files {
		info, err := os.Stat(f.Path)
		if err != nil {
			fmt.Printf("  %-30s missing\n", f.Path)
			missing++
			continue
		}

		// Check if file content is the C4 ID string (smudge didn't restore).
		if info.Size() == 90 || info.Size() == 91 {
			data, err := os.ReadFile(f.Path)
			if err == nil {
				idStr := string(data)
				if len(idStr) > 0 && idStr[len(idStr)-1] == '\n' {
					idStr = idStr[:len(idStr)-1]
				}
				if _, parseErr := c4.Parse(idStr); parseErr == nil {
					fmt.Printf("  %-30s not restored\n", f.Path)
					notRestored++
					continue
				}
			}
		}

		// Compute actual C4 ID and compare.
		file, err := os.Open(f.Path)
		if err != nil {
			fmt.Printf("  %-30s missing\n", f.Path)
			missing++
			continue
		}
		actual := c4.Identify(file)
		file.Close()

		if actual.IsNil() {
			fmt.Printf("  %-30s error (could not compute ID)\n", f.Path)
			missing++
			continue
		}

		if actual == f.ID {
			fmt.Printf("  %-30s ok\n", f.Path)
			ok++
		} else if !s.Has(f.ID) {
			fmt.Printf("  %-30s missing from store\n", f.Path)
			missing++
		} else {
			fmt.Printf("  %-30s modified\n", f.Path)
			modified++
		}
	}

	fmt.Printf("\nVerified: %d ok", ok)
	if modified > 0 {
		fmt.Printf(", %d modified", modified)
	}
	if notRestored > 0 {
		fmt.Printf(", %d not restored", notRestored)
	}
	if missing > 0 {
		fmt.Printf(", %d missing", missing)
	}
	fmt.Println()
	return nil
}
