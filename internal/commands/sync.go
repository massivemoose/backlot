package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/massivemoose/backlot/internal/autosync"
	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

func runSync(args []string, stdout, stderr io.Writer) error {
	result, err := syncSpec().Parse(args)
	if err != nil {
		return err
	}
	continueFlag := result.Bool("continue")
	abortFlag := result.Bool("abort")
	if continueFlag && abortFlag {
		return fmt.Errorf("choose only one of --continue or --abort")
	}
	if (continueFlag || abortFlag) && result.IsSet("message") {
		return fmt.Errorf("-m is only supported for normal backlot sync")
	}

	root, err := paths.BacklotRoot(result.String("root"))
	if err != nil {
		return err
	}
	if err := ensureRootOutsideCurrentProject(root); err != nil {
		return err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
		return err
	}
	release, err := acquireSyncLock(root)
	if err != nil {
		return err
	}
	defer release()
	quiet := result.Bool("quiet")
	if abortFlag {
		if err := runSyncAbort(root, stdout, quiet); err != nil {
			return err
		}
		return recordManualSyncAbort(root)
	}
	if continueFlag {
		if err := runSyncContinue(root, stdout, quiet); err != nil {
			return err
		}
		return recordManualSyncSuccess(root)
	}
	if err := runNormalSync(root, result.String("message"), stdout, quiet); err != nil {
		return err
	}
	return recordManualSyncSuccess(root)
}

func syncSpec() *chomp.Spec {
	return chomp.New("backlot", "sync").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		String("message", chomp.Short('m'), chomp.ValueName("message"), chomp.Default("Update backlot state"), chomp.Description("commit message")).
		Bool("continue", chomp.Description("continue an interrupted Backlot sync")).
		Bool("abort", chomp.Description("abort an interrupted Backlot sync")).
		Bool("quiet", chomp.Description("suppress normal sync output")).
		Positionals(0, 0)
}

func printSyncUsage(w io.Writer) {
	printSpecUsage(w, syncSpec())
}

func runNormalSync(root, message string, stdout io.Writer, quiet bool) error {
	if stateDir, ok := currentAttachedProjectStateDir(root); ok {
		if err := ensureProjectMarker(stateDir); err != nil {
			return err
		}
	}
	if err := ensureNoGitOperationInProgress(root); err != nil {
		return err
	}
	if err := ensureArchiveEncryptionReady(root); err != nil {
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
			syncPrintln(stdout, quiet, "Backlot state is clean.")
			return nil
		}
		if !upstream {
			if err := pushFirstUpstream(root); err != nil {
				return err
			}
			syncPrintln(stdout, quiet, "Backlot state synced.")
			return nil
		}
		if _, err := gitutil.RunGit(root, "pull", "--rebase"); err != nil {
			return syncRebaseError(root, err)
		}
		if err := ensureArchiveEncryptionReady(root); err != nil {
			return err
		}
		if _, err := gitutil.RunGit(root, "push"); err != nil {
			return syncGitError("push", root, err)
		}
		syncPrintln(stdout, quiet, "Backlot state synced.")
		return nil
	}

	if _, err := gitutil.RunGit(root, "add", "-A"); err != nil {
		return syncGitError("staging private state", root, err)
	}
	staged, err := gitutil.HasStagedChanges(root)
	if err != nil {
		return syncGitError("staged change check", root, err)
	}
	if staged {
		if _, err := gitutil.RunGit(root, "commit", "-m", message); err != nil {
			return syncGitError("committing private state", root, err)
		}
	} else {
		if !hasOrigin {
			syncPrintln(stdout, quiet, "Backlot state is clean.")
			return nil
		}
	}
	if !hasOrigin {
		syncPrintln(stdout, quiet, "No origin remote configured; committed locally and skipped push.")
		return nil
	}
	if !upstream {
		if err := pushFirstUpstream(root); err != nil {
			return err
		}
		syncPrintln(stdout, quiet, "Backlot state synced.")
		return nil
	}
	if _, err := gitutil.RunGit(root, "pull", "--rebase"); err != nil {
		return syncRebaseError(root, err)
	}
	if err := ensureArchiveEncryptionReady(root); err != nil {
		return err
	}
	if _, err := gitutil.RunGit(root, "push"); err != nil {
		return syncGitError("push", root, err)
	}
	syncPrintln(stdout, quiet, "Backlot state synced.")
	return nil
}

func runSyncAbort(root string, stdout io.Writer, quiet bool) error {
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
	syncPrintln(stdout, quiet, "Backlot sync aborted.")
	return nil
}

func runSyncContinue(root string, stdout io.Writer, quiet bool) error {
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
	syncPrintln(stdout, quiet, "Backlot state synced.")
	return nil
}

func syncPrintln(stdout io.Writer, quiet bool, text string) {
	if !quiet {
		fmt.Fprintln(stdout, text)
	}
}

type syncState struct {
	InProgress bool
	Operation  string
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

type syncFailure struct {
	Category   string
	Conflict   bool
	Conflicts  []string
	Operation  string
	LocalHead  string
	RemoteHead string
	err        error
}

func (e *syncFailure) Error() string {
	return e.err.Error()
}

func (e *syncFailure) Unwrap() error {
	return e.err
}

func syncGitError(operation string, root string, err error) error {
	return &syncFailure{
		Category: operation,
		err:      fmt.Errorf("%s failed while syncing Backlot root %s: %w", operation, root, err),
	}
}

func ensureArchiveEncryptionReady(root string) error {
	state := collectArchiveEncryptionState(root)
	switch state.Status {
	case encryptionDisabled, encryptionUnlocked:
		return nil
	case encryptionLocked, encryptionMisconfigured:
		message := state.Problem
		if message == "" {
			message = "Backlot archive encryption is not ready"
		}
		if state.Err != nil {
			message += ": " + state.Err.Error()
		}
		if state.Recovery != "" {
			message += "\nRecovery: " + state.Recovery
		}
		return &syncFailure{
			Category: "encryption",
			err:      fmt.Errorf("%s", message),
		}
	default:
		return nil
	}
}

func syncRebaseError(root string, err error) error {
	failure := &syncFailure{
		Category: "pull --rebase",
		err:      fmt.Errorf("%w\n\n%s", syncGitError("pull --rebase", root, err), syncRecoveryInstructions(root)),
	}
	state, stateErr := detectSyncState(root)
	if stateErr == nil {
		failure.Operation = state.Operation
		failure.Conflicts = append([]string(nil), state.Conflicts...)
		failure.Conflict = state.Operation == "rebase" && len(state.Conflicts) > 0
	}
	failure.LocalHead, _ = gitutil.RunGit(root, "rev-parse", "ORIG_HEAD")
	failure.RemoteHead, _ = gitutil.RunGit(root, "rev-parse", "@{u}")
	return failure
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
			switch name {
			case "rebase-apply", "rebase-merge":
				state.Operation = "rebase"
			case "MERGE_HEAD":
				state.Operation = "merge"
			case "CHERRY_PICK_HEAD":
				state.Operation = "cherry-pick"
			case "REVERT_HEAD":
				state.Operation = "revert"
			}
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

func recordManualSyncSuccess(root string) error {
	return updateManagedAutosyncState(root, func(state *autosync.State) {
		state.RecordSuccess(autosyncNow())
	})
}

func recordManualSyncAbort(root string) error {
	return updateManagedAutosyncState(root, func(state *autosync.State) {
		state.RecordAbortRecovery(autosyncNow())
	})
}

func updateManagedAutosyncState(root string, update func(*autosync.State)) error {
	home, err := autosyncHomeDir()
	if err != nil {
		return nil
	}
	managedPaths, err := autosync.ResolvePathsForPlatform(home, root, autosyncGOOS)
	if err != nil {
		return nil
	}
	config, err := autosync.LoadConfig(managedPaths.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := autosync.ValidateManagedConfig(config, managedPaths); err != nil {
		return err
	}
	state, err := autosync.LoadState(managedPaths.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		state = autosync.State{}
	} else if err != nil {
		return err
	}
	update(&state)
	return autosync.WriteState(managedPaths.StatePath, state)
}
