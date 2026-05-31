package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

const starterNotes = `# Backlot notes

Private project notes for this repository.
`

func runAttach(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("attach", stderr)
	rootFlag := fs.String("root", "", "Backlot root path")
	linkName := fs.String("link-name", ".backlot", "link name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}
	if err := paths.ValidateLinkName(*linkName); err != nil {
		return err
	}

	root, err := paths.BacklotRoot(*rootFlag)
	if err != nil {
		return err
	}
	if !gitutil.IsGitRepoRoot(root) {
		return fmt.Errorf("Backlot root %s is not initialized; run backlot init first", root)
	}

	current, err := cwd()
	if err != nil {
		return err
	}
	repoRoot, err := gitutil.RepoRoot(current)
	if err != nil {
		return err
	}
	origin, err := gitutil.OriginURL(repoRoot)
	if err != nil {
		return err
	}
	key, err := gitutil.NormalizeOrigin(origin)
	if err != nil {
		return err
	}

	stateDir := paths.ProjectStateDir(root, key)
	if err := ensureStarterState(stateDir); err != nil {
		return err
	}
	if err := paths.EnsureManagedSymlink(filepath.Join(repoRoot, *linkName), stateDir); err != nil {
		return err
	}
	if err := paths.EnsureExclude(repoRoot, *linkName); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Attached Backlot\n")
	fmt.Fprintf(stdout, "Project key: %s\n", key)
	fmt.Fprintf(stdout, "State dir:   %s\n", stateDir)
	fmt.Fprintf(stdout, "Link:        %s -> %s\n", *linkName, stateDir)
	return nil
}

// ensureStarterState creates starter files only when Backlot creates this
// project's private folder for the first time. Existing project folders are
// left untouched so attach never reimposes a layout the user changed.
func ensureStarterState(stateDir string) error {
	info, err := os.Stat(stateDir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("Backlot state path %s exists and is not a directory", stateDir)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	notesPath := filepath.Join(stateDir, "notes.md")
	if err := os.WriteFile(notesPath, []byte(starterNotes), 0o644); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(stateDir, "llm"), 0o755); err != nil {
		return err
	}
	return os.Mkdir(filepath.Join(stateDir, "scratch"), 0o755)
}
