package commands

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
)

const preCommitHook = `#!/bin/sh
if git diff --cached --name-only | grep -q '^\.backlot'; then
  echo "Backlot: refusing to commit .backlot private workspace files."
  exit 1
fi
`

func runProtect(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("protect", stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  backlot protect")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Example:")
		fmt.Fprintln(stderr, "  backlot protect")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}

	current, err := cwd()
	if err != nil {
		return err
	}
	repoRoot, err := gitutil.RepoRoot(current)
	if err != nil {
		return err
	}
	hookPath, err := gitutil.GitPath(repoRoot, "hooks/pre-commit")
	if err != nil {
		return err
	}
	if _, err := os.Stat(hookPath); err == nil {
		fmt.Fprintf(stdout, "A pre-commit hook already exists at %s.\n", hookPath)
		fmt.Fprintln(stdout, "Backlot did not overwrite it.")
		fmt.Fprintln(stdout, "To add the Backlot guard, append this script to the existing hook or add the equivalent check to your hook manager:")
		fmt.Fprint(stdout, preCommitHook)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(hookPath, []byte(preCommitHook), 0o755); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Installed Backlot pre-commit hook at %s\n", hookPath)
	return nil
}
