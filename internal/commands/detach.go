package commands

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

func runDetach(args []string, stdout, stderr io.Writer) error {
	result, err := detachSpec().Parse(args)
	if err != nil {
		return err
	}

	root, err := paths.BacklotRoot(result.String("root"))
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

func detachSpec() *chomp.Spec {
	return chomp.New("backlot", "detach").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		Positionals(0, 0)
}

func printDetachUsage(w io.Writer) {
	printSpecUsage(w, detachSpec())
}
