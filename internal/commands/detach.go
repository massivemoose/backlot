package commands

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

func runDetach(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("detach", stderr)
	rootFlag := fs.String("root", "", "Backlot root path")
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
	current, err := cwd()
	if err != nil {
		return err
	}
	repoRoot, err := gitutil.RepoRoot(current)
	if err != nil {
		return err
	}

	removed, err := paths.RemoveManagedSymlinkUnderRoot(filepath.Join(repoRoot, ".backlot"), root)
	if err != nil {
		return err
	}
	if err := paths.RemoveExclude(repoRoot, ".backlot"); err != nil {
		return err
	}

	if removed {
		fmt.Fprintln(stdout, "Detached Backlot")
		fmt.Fprintln(stdout, "Link:        removed .backlot")
	} else {
		fmt.Fprintln(stdout, "No .backlot link found.")
	}
	fmt.Fprintln(stdout, "Exclude:     removed .backlot entries")
	fmt.Fprintf(stdout, "Archive:     left unchanged at %s\n", root)
	return nil
}
