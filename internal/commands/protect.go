package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/chomp"
)

const preCommitHook = `#!/bin/sh
if git diff --cached --name-only | grep -q '^\.backlot'; then
  echo "Backlot: refusing to commit .backlot private workspace files."
  exit 1
fi
`

func runProtect(args []string, stdout, stderr io.Writer) error {
	if _, err := protectSpec().Parse(args); err != nil {
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

func protectSpec() *chomp.Spec {
	return chomp.New("backlot", "protect").
		Positionals(0, 0)
}

func printProtectUsage(w io.Writer) {
	printSpecUsage(w, protectSpec())
}
