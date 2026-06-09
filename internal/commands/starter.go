package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

func starterRouter(stdout, stderr io.Writer) *chomp.Router {
	return chomp.NewRouter(
		"starter",
		"Manage starter templates.",
		runnableCommand{
			name:    "apply",
			summary: "Apply starter template files to workspaces",
			run:     func(args []string) error { return runStarterApply(args, stdout, stderr) },
			usage:   printStarterApplyUsage,
		},
	)
}

func runStarterApply(args []string, stdout, stderr io.Writer) error {
	result, err := starterApplySpec().Parse(args)
	if err != nil {
		return err
	}

	root, err := paths.BacklotRoot(result.String("root"))
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
	if result.Bool("dry-run") {
		fmt.Fprintln(stdout, "Dry run: yes")
	} else {
		fmt.Fprintln(stdout, "Dry run: no")
	}
	fmt.Fprintf(stdout, "Workspaces: %d\n", len(workspaces))
	if len(workspaces) > 0 {
		fmt.Fprintln(stdout)
	}
	for _, workspace := range workspaces {
		stats, err := applyStarterToWorkspace(starterDir, workspace, result.Bool("dry-run"))
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, workspace)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "  %s: added=%d skipped=%d conflicts=%d\n", filepath.ToSlash(rel), stats.Added, stats.Skipped, stats.Conflicts)
	}
	if result.Bool("dry-run") {
		fmt.Fprintln(stdout, "\nDry run - no changes applied")
	}
	return nil
}

func starterApplySpec() *chomp.Spec {
	return chomp.New("backlot", "starter", "apply").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		Bool("dry-run", chomp.Description("show changes without writing files")).
		Positionals(0, 0)
}

func printStarterApplyUsage(w io.Writer) {
	printSpecUsage(w, starterApplySpec())
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
