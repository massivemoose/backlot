package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

func runSync(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("sync", stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  backlot sync [--root PATH] [-m MESSAGE]")
		fmt.Fprintln(stderr, "  backlot sync [--root PATH] --continue")
		fmt.Fprintln(stderr, "  backlot sync [--root PATH] --abort")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Examples:")
		fmt.Fprintln(stderr, "  backlot sync")
		fmt.Fprintln(stderr, "  backlot sync -m \"Update private notes\"")
		fmt.Fprintln(stderr, "  # Continue after resolving a conflict:")
		fmt.Fprintln(stderr, "  backlot sync --continue")
		fmt.Fprintln(stderr, "  # Abort an interrupted sync:")
		fmt.Fprintln(stderr, "  backlot sync --abort")
	}
	rootFlag := fs.String("root", "", "Backlot root path")
	message := fs.String("m", "Update backlot state", "commit message")
	continueFlag := fs.Bool("continue", false, "continue an interrupted Backlot sync")
	abortFlag := fs.Bool("abort", false, "abort an interrupted Backlot sync")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}
	if *continueFlag && *abortFlag {
		return fmt.Errorf("choose only one of --continue or --abort")
	}
	messageProvided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "m" {
			messageProvided = true
		}
	})
	if (*continueFlag || *abortFlag) && messageProvided {
		return fmt.Errorf("-m is only supported for normal backlot sync")
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
	if *abortFlag {
		return runSyncAbort(root, stdout)
	}
	if *continueFlag {
		return runSyncContinue(root, stdout)
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

func runSyncAbort(root string, stdout io.Writer) error {
	state, err := detectSyncState(root)
	if err != nil {
		return err
	}
	if !state.Interrupted() {
		return fmt.Errorf("no interrupted Backlot sync to abort")
	}
	if _, err := gitutil.RunGit(root, "rebase", "--abort"); err != nil {
		return syncGitError("rebase --abort", root, err)
	}
	fmt.Fprintln(stdout, "Backlot sync aborted.")
	return nil
}

func runSyncContinue(root string, stdout io.Writer) error {
	state, err := detectSyncState(root)
	if err != nil {
		return err
	}
	if !state.Interrupted() {
		return fmt.Errorf("no interrupted Backlot sync to continue")
	}
	if _, err := gitutil.RunGit(root, "add", "-A"); err != nil {
		return syncGitError("staging resolved conflicts", root, err)
	}
	if _, err := gitutil.RunGit(root, "-c", "core.editor=true", "rebase", "--continue"); err != nil {
		return syncGitError("rebase --continue", root, err)
	}
	if _, err := gitutil.RunGit(root, "push"); err != nil {
		return syncGitError("push", root, err)
	}
	fmt.Fprintln(stdout, "Backlot state synced.")
	return nil
}

type syncState struct {
	InProgress bool
	Conflicts  []string
}

func (s syncState) Interrupted() bool {
	return s.InProgress || len(s.Conflicts) > 0
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
	var b strings.Builder
	b.WriteString("Backlot sync hit a Git conflict in the private archive.\n")
	targets := recoveryEditTargets(root)
	if len(targets) > 0 {
		b.WriteString("Resolve the conflicted files:\n")
		for _, target := range targets {
			fmt.Fprintf(&b, "  edit %s\n", target)
		}
	} else {
		fmt.Fprintf(&b, "Resolve it manually:\n  edit the conflicted files under %s\n", root)
	}
	b.WriteString("Then run:\n  backlot sync --continue\n")
	b.WriteString("Or abort the sync:\n  backlot sync --abort")
	return b.String()
}

func recoveryEditTargets(root string) []string {
	state, err := detectSyncState(root)
	if err != nil || len(state.Conflicts) == 0 {
		return nil
	}
	stateDir, ok := currentAttachedProjectStateDir(root)
	if !ok {
		return nil
	}
	var targets []string
	for _, conflict := range state.Conflicts {
		if target, ok := projectFacingConflictPath(root, stateDir, conflict); ok {
			targets = append(targets, target)
		}
	}
	return targets
}

func currentAttachedProjectStateDir(root string) (string, bool) {
	current, err := cwd()
	if err != nil {
		return "", false
	}
	repoRoot, err := gitutil.RepoRoot(current)
	if err != nil {
		return "", false
	}
	origin, err := gitutil.OriginURL(repoRoot)
	if err != nil {
		return "", false
	}
	key, err := gitutil.NormalizeOrigin(origin)
	if err != nil {
		return "", false
	}
	stateDir := paths.ProjectStateDir(root, key)
	linkTarget, err := filepath.EvalSymlinks(filepath.Join(repoRoot, ".backlot"))
	if err != nil {
		return "", false
	}
	stateTarget, err := filepath.EvalSymlinks(stateDir)
	if err != nil {
		return "", false
	}
	if linkTarget != stateTarget {
		return "", false
	}
	return stateDir, true
}

func projectFacingConflictPath(root, stateDir, conflict string) (string, bool) {
	fullPath := filepath.Join(root, filepath.FromSlash(conflict))
	rel, err := filepath.Rel(stateDir, fullPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(".backlot", rel)), true
}

func ensureNoGitOperationInProgress(root string) error {
	state, err := detectSyncState(root)
	if err != nil {
		return err
	}
	if state.InProgress {
		return fmt.Errorf("Backlot root %s has an unfinished Git operation.\n\n%s", root, syncRecoveryInstructions(root))
	}
	if len(state.Conflicts) > 0 {
		return fmt.Errorf("Backlot root %s has unresolved conflicts.\n\n%s", root, syncRecoveryInstructions(root))
	}
	return nil
}

func detectSyncState(root string) (syncState, error) {
	state := syncState{}
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
			return state, syncGitError("git state check", root, err)
		}
		if _, err := os.Stat(path); err == nil {
			state.InProgress = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return state, err
		}
	}
	conflicts, err := gitutil.RunGit(root, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return state, syncGitError("conflict check", root, err)
	}
	for _, line := range strings.Split(conflicts, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			state.Conflicts = append(state.Conflicts, line)
		}
	}
	return state, nil
}

func isNothingToCommit(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "nothing to commit") || strings.Contains(text, "no changes added to commit")
}
