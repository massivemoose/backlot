package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/massivemoose/backlot/internal/paths"
)

func runStarter(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printStarterUsage(stderr)
		return flag.ErrHelp
	}
	switch args[0] {
	case "apply":
		return runStarterApply(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown starter command %q", args[0])
	}
}

func runStarterApply(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("starter apply", stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  backlot starter apply [--root PATH] [--dry-run]")
	}
	rootFlag := fs.String("root", "", "Backlot root path")
	dryRun := fs.Bool("dry-run", false, "show changes without writing files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}

	root, err := paths.BacklotRoot(*rootFlag)
	if err != nil {
		return err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
		return err
	}
	starterDir := filepath.Join(root, ".starter")
	starterInfo, err := os.Stat(starterDir)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("Backlot starter template %s does not exist", starterDir)
	}
	if err != nil {
		return err
	}
	if !starterInfo.IsDir() {
		return fmt.Errorf("Backlot starter template %s exists and is not a directory", starterDir)
	}
	if err := validateStarterTemplate(starterDir); err != nil {
		return err
	}

	workspaces, err := discoverProjectWorkspaces(root)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Backlot starter apply")
	fmt.Fprintf(stdout, "Backlot root: %s\n", root)
	fmt.Fprintf(stdout, "Starter: %s\n", starterDir)
	if *dryRun {
		fmt.Fprintln(stdout, "Dry run: yes")
	} else {
		fmt.Fprintln(stdout, "Dry run: no")
	}
	fmt.Fprintf(stdout, "Workspaces: %d\n", len(workspaces))
	if len(workspaces) > 0 {
		fmt.Fprintln(stdout)
	}
	for _, workspace := range workspaces {
		stats, err := applyStarterToWorkspace(starterDir, workspace, *dryRun)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, workspace)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "  %s: added=%d skipped=%d conflicts=%d\n", filepath.ToSlash(rel), stats.Added, stats.Skipped, stats.Conflicts)
	}
	if *dryRun {
		fmt.Fprintln(stdout, "\nDry run - no changes applied")
	}
	return nil
}

func printStarterUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: backlot starter <command> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  apply    [--root PATH] [--dry-run]")
}

func discoverProjectWorkspaces(root string) ([]string, error) {
	var workspaces []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		switch entry.Name() {
		case ".git", ".starter":
			return filepath.SkipDir
		}
		markerPath := filepath.Join(path, projectMarkerName)
		info, err := os.Lstat(markerPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Backlot project marker %s exists and is not a regular file", markerPath)
		}
		workspaces = append(workspaces, path)
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(workspaces)
	return workspaces, nil
}

type starterApplyStats struct {
	Added     int
	Skipped   int
	Conflicts int
}

func applyStarterToWorkspace(starterDir, workspace string, dryRun bool) (starterApplyStats, error) {
	var stats starterApplyStats
	err := filepath.WalkDir(starterDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == starterDir {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(starterDir, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(workspace, rel)
		dstInfo, err := os.Lstat(dst)
		if err == nil {
			if starterPathConflicts(info, dstInfo) {
				stats.Conflicts++
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			stats.Skipped++
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		stats.Added++
		if dryRun {
			return nil
		}
		if info.IsDir() {
			return os.Mkdir(dst, info.Mode().Perm())
		}
		return copyStarterFile(path, dst, info.Mode().Perm())
	})
	return stats, err
}

func starterPathConflicts(starterInfo, dstInfo os.FileInfo) bool {
	if starterInfo.IsDir() {
		return !dstInfo.IsDir()
	}
	if starterInfo.Mode().IsRegular() {
		return !dstInfo.Mode().IsRegular()
	}
	return true
}
