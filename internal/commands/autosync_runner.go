package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/massivemoose/backlot/internal/autosync"
	"github.com/massivemoose/backlot/internal/gitutil"
)

var (
	autosyncHomeDir     = os.UserHomeDir
	autosyncNow         = time.Now
	autosyncNotify      = notifyMacOS
	autosyncAbortRebase = func(root string) error {
		_, err := gitutil.RunGit(root, "rebase", "--abort")
		return err
	}
)

func runManagedAutosync(root string) error {
	if err := requireBacklotArchiveRoot(root); err != nil {
		return err
	}
	home, err := autosyncHomeDir()
	if err != nil {
		return err
	}
	managedPaths, err := autosync.ResolvePaths(home, root)
	if err != nil {
		return err
	}
	config, err := autosync.LoadConfig(managedPaths.ConfigPath)
	if err != nil {
		return err
	}
	if err := autosync.ValidateManagedConfig(config, managedPaths); err != nil {
		return err
	}
	state, err := loadManagedAutosyncState(managedPaths.StatePath)
	if err != nil {
		return err
	}
	now := autosyncNow()
	if state.Paused() {
		state.RecordSkippedPaused(now)
		return autosync.WriteState(managedPaths.StatePath, state)
	}

	release, err := acquireSyncLock(managedPaths.Root)
	if errors.Is(err, errSyncBusy) {
		state.RecordSkippedBusy(now)
		return autosync.WriteState(managedPaths.StatePath, state)
	}
	if err != nil {
		return err
	}
	defer release()

	err = runNormalSync(managedPaths.Root, "Update backlot state", io.Discard, true)
	if err == nil {
		state.RecordSuccess(now)
		if err := autosync.WriteState(managedPaths.StatePath, state); err != nil {
			return err
		}
		return removeAutosyncLog(managedPaths.LogPath)
	}

	var failure *syncFailure
	if errors.As(err, &failure) && failure.Conflict && failure.Operation == "rebase" {
		return handleManagedConflict(managedPaths, state, failure, now)
	}
	category := "sync"
	if errors.As(err, &failure) && failure.Category != "" {
		category = failure.Category
	}
	notify := state.RecordFailure(now, category, err.Error())
	if writeErr := writeAutosyncLog(managedPaths.LogPath, err.Error()); writeErr != nil {
		return writeErr
	}
	if err := autosync.WriteState(managedPaths.StatePath, state); err != nil {
		return err
	}
	if notify {
		return sendManagedNotification(managedPaths, &state,
			"Backlot auto-sync needs attention",
			fmt.Sprintf("Backlot auto-sync for %s has failed %d times. Run: backlot autosync status --root %s", managedPaths.Root, state.ConsecutiveFailures, managedPaths.Root),
			now,
		)
	}
	return nil
}

func handleManagedConflict(managedPaths autosync.Paths, state autosync.State, failure *syncFailure, now time.Time) error {
	abortErr := autosyncAbortRebase(managedPaths.Root)
	var notify bool
	var message string
	if abortErr == nil {
		notify = state.RecordConflict(now, failure.Conflicts, failure.LocalHead, failure.RemoteHead, "backlot sync")
		message = "Backlot auto-sync paused because local and remote changes conflict.\nRun: backlot sync"
	} else {
		notify = state.RecordUrgentRecovery(now, failure.Conflicts, failure.LocalHead, failure.RemoteHead,
			fmt.Sprintf("automatic rebase abort failed: %v", abortErr), "backlot sync --abort")
		message = "Backlot auto-sync hit a conflict and could not clean it up.\nRun: backlot sync --abort"
	}
	if err := writeAutosyncLog(managedPaths.LogPath, failure.Error()); err != nil {
		return err
	}
	if err := autosync.WriteState(managedPaths.StatePath, state); err != nil {
		return err
	}
	if notify {
		return sendManagedNotification(managedPaths, &state, "Backlot auto-sync paused", message, now)
	}
	return nil
}

func sendManagedNotification(managedPaths autosync.Paths, state *autosync.State, title, body string, now time.Time) error {
	state.RecordNotification(now, nil)
	if err := autosync.WriteState(managedPaths.StatePath, *state); err != nil {
		return err
	}
	notifyErr := autosyncNotify(title, body)
	if notifyErr != nil {
		state.LastNotificationError = notifyErr.Error()
		return autosync.WriteState(managedPaths.StatePath, *state)
	}
	return nil
}

func loadManagedAutosyncState(path string) (autosync.State, error) {
	state, err := autosync.LoadState(path)
	if errors.Is(err, os.ErrNotExist) {
		return autosync.State{}, nil
	}
	return state, err
}

func writeAutosyncLog(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(text)+"\n"), 0o600)
}

func removeAutosyncLog(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func notifyMacOS(title, body string) error {
	const script = `on run argv
display notification (item 2 of argv) with title (item 1 of argv)
end run`
	cmd := exec.Command("/usr/bin/osascript", "-e", script, title, body)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("osascript notification failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
