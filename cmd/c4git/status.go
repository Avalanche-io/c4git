package main

import "fmt"

func runStatus() error {
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

	fmt.Println("Managed files:")
	var inStore, missing int
	for _, f := range files {
		name := f.Path
		short := f.ID.String()[:12] + "..."
		status := "ok"
		if !s.Has(f.ID) {
			status = "missing"
			missing++
		} else {
			inStore++
		}
		fmt.Printf("  %-30s %s  %s\n", name, short, status)
	}

	fmt.Printf("\n%d managed files (%d in store, %d missing)\n", len(files), inStore, missing)
	return nil
}
