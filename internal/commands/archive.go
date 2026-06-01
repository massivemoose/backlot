package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/massivemoose/backlot/internal/gitutil"
)

const archiveMarkerName = ".backlot-root"

const archiveMarker = `# Backlot archive root

This file marks this Git repository as a Backlot private state archive.
`

func ensureArchiveMarker(root string) error {
	path := filepath.Join(root, archiveMarkerName)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, []byte(archiveMarker), 0o644)
}

func isBacklotArchiveRoot(root string) bool {
	info, err := os.Stat(filepath.Join(root, archiveMarkerName))
	return err == nil && !info.IsDir()
}

func requireBacklotArchiveRoot(root string) error {
	if !gitutil.IsGitRepoRoot(root) {
		if repoRoot, err := gitutil.RepoRoot(root); err == nil && !samePath(root, repoRoot) && !isBacklotArchiveRoot(repoRoot) {
			return fmt.Errorf("Backlot root %s is inside Git repo %s; choose a root outside the project repo", root, repoRoot)
		}
		return fmt.Errorf("Backlot root %s is not initialized; run backlot init first", root)
	}
	if !isBacklotArchiveRoot(root) {
		return fmt.Errorf("Backlot root %s is not a Backlot archive; move it aside or choose another root with --root", root)
	}
	return nil
}

func ensureRootOutsideCurrentProject(root string) error {
	current, err := cwd()
	if err != nil {
		return err
	}
	repoRoot, err := gitutil.RepoRoot(current)
	if err != nil {
		return nil
	}
	if isBacklotArchiveRoot(repoRoot) && samePath(root, repoRoot) {
		return nil
	}
	if pathWithinOrEqual(root, repoRoot) {
		return fmt.Errorf("Backlot root %s is inside current Git repo %s; choose a root outside the project repo", root, repoRoot)
	}
	return nil
}

func ensureRootOutsideProject(root, repoRoot string) error {
	if pathWithinOrEqual(root, repoRoot) {
		return fmt.Errorf("Backlot root %s is inside current Git repo %s; choose a root outside the project repo", root, repoRoot)
	}
	return nil
}

func samePath(a, b string) bool {
	return comparablePath(a) == comparablePath(b)
}

func pathWithinOrEqual(path, root string) bool {
	pathAbs := comparablePath(path)
	rootAbs := comparablePath(root)
	if pathAbs == "" || rootAbs == "" {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func comparablePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved)
	}
	var suffix []string
	current := filepath.Clean(abs)
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			parts := append([]string{resolved}, suffix...)
			return filepath.Clean(filepath.Join(parts...))
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(abs)
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
}
