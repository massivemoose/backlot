package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

func runSync(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("sync", stderr)
	rootFlag := fs.String("root", "", "Backlot root path")
	message := fs.String("m", "Update backlot state", "commit message")
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
	if !gitutil.IsGitRepoRoot(root) {
		return fmt.Errorf("Backlot root %s is not initialized; run backlot init first", root)
	}
	status, err := gitutil.RunGit(root, "status", "--short")
	if err != nil {
		return syncGitError("status check", root, err)
	}
	if strings.TrimSpace(status) == "" {
		fmt.Fprintln(stdout, "Backlot state is clean.")
		return nil
	}

	if _, err := gitutil.RunGit(root, "add", "-A"); err != nil {
		return syncGitError("staging private state", root, err)
	}
	if _, err := gitutil.RunGit(root, "commit", "-m", *message); err != nil {
		if !isNothingToCommit(err) {
			return syncGitError("committing private state", root, err)
		}
		fmt.Fprintln(stdout, "Backlot state is clean.")
		return nil
	}
	if !gitutil.HasOrigin(root) {
		fmt.Fprintln(stdout, "No origin remote configured; committed locally and skipped push.")
		return nil
	}
	if _, err := gitutil.RunGit(root, "pull", "--rebase"); err != nil {
		return syncGitError("pull --rebase", root, err)
	}
	if _, err := gitutil.RunGit(root, "push"); err != nil {
		return syncGitError("push", root, err)
	}
	fmt.Fprintln(stdout, "Backlot state synced.")
	return nil
}

func syncGitError(operation string, root string, err error) error {
	return fmt.Errorf("%s failed while syncing Backlot root %s: %w", operation, root, err)
}

func isNothingToCommit(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return errors.Is(err, nil) || strings.Contains(text, "nothing to commit") || strings.Contains(text, "no changes added to commit")
}
