package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

func runSync(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("sync", stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  backlot sync [--root PATH] [-m MESSAGE]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Examples:")
		fmt.Fprintln(stderr, "  backlot sync")
		fmt.Fprintln(stderr, "  backlot sync -m \"Update private notes\"")
	}
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
	if err := ensureRootOutsideCurrentProject(root); err != nil {
		return err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
		return err
	}
	if err := ensureNoGitOperationInProgress(root); err != nil {
		return err
	}
	status, err := gitutil.RunGit(root, "status", "--short")
	if err != nil {
		return syncGitError("status check", root, err)
	}
	dirty := strings.TrimSpace(status) != ""
	hasOrigin := gitutil.HasOrigin(root)
	upstream := hasUpstream(root)
	if hasOrigin {
		if _, err := gitutil.RunGit(root, "fetch", "origin"); err != nil {
			return syncGitError("fetch", root, err)
		}
	}

	if !dirty {
		if !hasOrigin {
			fmt.Fprintln(stdout, "Backlot state is clean.")
			return nil
		}
		if !upstream {
			if err := pushFirstUpstream(root); err != nil {
				return err
			}
			fmt.Fprintln(stdout, "Backlot state synced.")
			return nil
		}
		if _, err := gitutil.RunGit(root, "pull", "--rebase"); err != nil {
			return syncRebaseError(root, err)
		}
		if _, err := gitutil.RunGit(root, "push"); err != nil {
			return syncGitError("push", root, err)
		}
		fmt.Fprintln(stdout, "Backlot state synced.")
		return nil
	}

	if _, err := gitutil.RunGit(root, "add", "-A"); err != nil {
		return syncGitError("staging private state", root, err)
	}
	if _, err := gitutil.RunGit(root, "commit", "-m", *message); err != nil {
		if !isNothingToCommit(err) {
			return syncGitError("committing private state", root, err)
		}
		if !hasOrigin {
			fmt.Fprintln(stdout, "Backlot state is clean.")
			return nil
		}
	}
	if !hasOrigin {
		fmt.Fprintln(stdout, "No origin remote configured; committed locally and skipped push.")
		return nil
	}
	if !upstream {
		if err := pushFirstUpstream(root); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Backlot state synced.")
		return nil
	}
	if _, err := gitutil.RunGit(root, "pull", "--rebase"); err != nil {
		return syncRebaseError(root, err)
	}
	if _, err := gitutil.RunGit(root, "push"); err != nil {
		return syncGitError("push", root, err)
	}
	fmt.Fprintln(stdout, "Backlot state synced.")
	return nil
}

func hasUpstream(root string) bool {
	_, err := gitutil.RunGit(root, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	return err == nil
}

func pushFirstUpstream(root string) error {
	if !hasCommit(root) {
		return fmt.Errorf("Backlot root %s has no commits to push; add private state and run backlot sync again", root)
	}
	branch, err := currentBranch(root)
	if err != nil {
		return err
	}
	remoteRef := "refs/remotes/origin/" + branch
	if _, err := gitutil.RunGit(root, "rev-parse", "--verify", remoteRef); err == nil {
		if _, err := gitutil.RunGit(root, "merge-base", "--is-ancestor", remoteRef, "HEAD"); err != nil {
			return fmt.Errorf("origin already has a remote branch %s that is not an ancestor of local HEAD; use backlot clone for existing archives or choose an empty archive remote", branch)
		}
	}
	if _, err := gitutil.RunGit(root, "push", "-u", "origin", branch); err != nil {
		return syncGitError("first push", root, err)
	}
	return nil
}

func hasCommit(root string) bool {
	_, err := gitutil.RunGit(root, "rev-parse", "--verify", "HEAD")
	return err == nil
}

func currentBranch(root string) (string, error) {
	branch, err := gitutil.RunGit(root, "branch", "--show-current")
	if err != nil {
		return "", syncGitError("current branch check", root, err)
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("Backlot root %s is not on a branch; cannot set upstream for first push", root)
	}
	return branch, nil
}

func syncGitError(operation string, root string, err error) error {
	return fmt.Errorf("%s failed while syncing Backlot root %s: %w", operation, root, err)
}

func syncRebaseError(root string, err error) error {
	return fmt.Errorf("%w\n\n%s", syncGitError("pull --rebase", root, err), syncRecoveryInstructions(root))
}

func syncRecoveryInstructions(root string) string {
	return fmt.Sprintf(`Backlot sync hit a Git conflict in the private archive.
Resolve it manually:
  git -C %s status
  edit the conflicted files under %s
  git -C %s add <PATH>
  git -C %s rebase --continue
Or abort the sync:
  git -C %s rebase --abort`, root, root, root, root, root)
}

func ensureNoGitOperationInProgress(root string) error {
	checks := []string{
		"MERGE_HEAD",
		"CHERRY_PICK_HEAD",
		"REVERT_HEAD",
		"rebase-apply",
		"rebase-merge",
	}
	for _, name := range checks {
		path, err := gitutil.GitPath(root, name)
		if err != nil {
			return syncGitError("git state check", root, err)
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("Backlot root %s has an unfinished Git operation.\n\n%s", root, syncRecoveryInstructions(root))
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	conflicts, err := gitutil.RunGit(root, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return syncGitError("conflict check", root, err)
	}
	if strings.TrimSpace(conflicts) != "" {
		return fmt.Errorf("Backlot root %s has unresolved conflicts.\n\n%s", root, syncRecoveryInstructions(root))
	}
	return nil
}

func isNothingToCommit(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return errors.Is(err, nil) || strings.Contains(text, "nothing to commit") || strings.Contains(text, "no changes added to commit")
}
