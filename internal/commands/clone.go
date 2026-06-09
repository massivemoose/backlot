package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

func runClone(args []string, stdout, stderr io.Writer) error {
	result, err := cloneSpec().Parse(args)
	if err != nil {
		return err
	}

	root, err := paths.BacklotRoot(result.String("root"))
	if err != nil {
		return err
	}
	if err := ensureCloneTarget(root); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return err
	}
	if err := runGitClone(result.Positional(0), root); err != nil {
		return err
	}
	if !gitutil.IsGitRepoRoot(root) {
		return fmt.Errorf("cloned Backlot root %s is not a Git repository", root)
	}
	if !isBacklotArchiveRoot(root) {
		return fmt.Errorf("cloned Backlot root %s is not a Backlot archive", root)
	}
	origin, err := gitutil.OriginURL(root)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Cloned Backlot archive")
	fmt.Fprintf(stdout, "Root:   %s\n", root)
	fmt.Fprintf(stdout, "Origin: %s\n", origin)
	return nil
}

func cloneSpec() *chomp.Spec {
	return chomp.New("backlot", "clone").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		Positionals(1, 1, "archive-url")
}

func printCloneUsage(w io.Writer) {
	printSpecUsage(w, cloneSpec())
}

func ensureCloneTarget(root string) error {
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("Backlot root %s already exists and is not a directory.\nMove it aside or choose another root with --root.", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("Backlot root %s already exists and is not empty.\nMove it aside or choose another root with --root.", root)
	}
	return nil
}

func runGitClone(remote, root string) error {
	_, err := gitutil.RunGit("", "clone", remote, root)
	return err
}
