package commands

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/massivemoose/backlot/internal/autosync"
	"github.com/massivemoose/backlot/internal/gitutil"
)

func TestManagedAutosyncAbortsConflictPausesAndNotifiesOnce(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	home := t.TempDir()
	stateA, stateB, notesB := createAutosyncConflictSetup(t)
	writeManagedAutosyncConfig(t, home, stateB)

	var notifications []string
	restore := stubAutosyncEnvironment(t, home, func(_, body string) error {
		notifications = append(notifications, body)
		return nil
	})
	defer restore()

	if err := runManagedAutosync(stateB); err != nil {
		t.Fatalf("runManagedAutosync conflict returned error: %v", err)
	}
	if got := runGitOutput(t, stateB, "status", "--short"); got != "" {
		t.Fatalf("state status after managed conflict = %q, want clean after abort", got)
	}
	if got := readAutosyncState(t, home, stateB); got.PausedReason != autosync.PauseConflict {
		t.Fatalf("PausedReason = %q, want %q", got.PausedReason, autosync.PauseConflict)
	} else if len(got.ConflictPaths) != 1 || !strings.HasSuffix(got.ConflictPaths[0], "notes.md") {
		t.Fatalf("ConflictPaths = %v, want notes.md", got.ConflictPaths)
	}
	if len(notifications) != 1 || !strings.Contains(notifications[0], "Run: backlot sync") {
		t.Fatalf("notifications = %v, want one actionable conflict notification", notifications)
	}

	if err := runManagedAutosync(stateB); err != nil {
		t.Fatalf("second runManagedAutosync returned error: %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("second paused run sent notification, notifications = %v", notifications)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"sync", "--root", stateB}, &out, &errOut); code == 0 {
		t.Fatalf("manual sync unexpectedly avoided conflict, stdout = %s", out.String())
	}
	if err := os.WriteFile(notesB, []byte("resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", stateB, "--continue"}, &out, &errOut); code != 0 {
		t.Fatalf("manual sync --continue exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := readAutosyncState(t, home, stateB); got.PausedReason != "" || got.Result != autosync.ResultSuccess {
		t.Fatalf("manual sync did not clear pause: %+v", got)
	}
	_ = stateA
}

func TestManagedAutosyncNotifiesAfterThreeFailures(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	home := t.TempDir()
	state := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, state)
	configureGitIdentity(t, state)
	mustRunGit(t, state, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing"))
	writeManagedAutosyncConfig(t, home, state)

	notifications := 0
	restore := stubAutosyncEnvironment(t, home, func(_, _ string) error {
		notifications++
		return errors.New("notifications denied")
	})
	defer restore()

	for i := 0; i < 4; i++ {
		if err := runManagedAutosync(state); err != nil {
			t.Fatalf("runManagedAutosync failure %d returned error: %v", i+1, err)
		}
	}
	got := readAutosyncState(t, home, state)
	if got.ConsecutiveFailures != 4 {
		t.Fatalf("ConsecutiveFailures = %d, want 4", got.ConsecutiveFailures)
	}
	if notifications != 1 {
		t.Fatalf("notifications = %d, want one after persistent failure", notifications)
	}
	if got.LastNotificationError == "" {
		t.Fatal("notification delivery failure was not recorded")
	}
}

func TestManagedAutosyncRecordsUrgentRecoveryWhenAbortFails(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	home := t.TempDir()
	_, state, _ := createAutosyncConflictSetup(t)
	writeManagedAutosyncConfig(t, home, state)

	var notification string
	restore := stubAutosyncEnvironment(t, home, func(_, body string) error {
		notification = body
		return nil
	})
	defer restore()
	oldAbort := autosyncAbortRebase
	autosyncAbortRebase = func(string) error { return errors.New("abort failed") }
	defer func() { autosyncAbortRebase = oldAbort }()

	if err := runManagedAutosync(state); err != nil {
		t.Fatalf("runManagedAutosync returned error: %v", err)
	}
	got := readAutosyncState(t, home, state)
	if got.PausedReason != autosync.PauseUrgentRecovery {
		t.Fatalf("PausedReason = %q, want %q", got.PausedReason, autosync.PauseUrgentRecovery)
	}
	if got.RecoveryCommand != "backlot sync --abort" {
		t.Fatalf("RecoveryCommand = %q, want backlot sync --abort", got.RecoveryCommand)
	}
	if !strings.Contains(notification, "backlot sync --abort") {
		t.Fatalf("notification = %q, want abort guidance", notification)
	}
	if syncState, err := detectSyncState(state); err != nil {
		t.Fatal(err)
	} else if !syncState.Interrupted() {
		t.Fatal("abort failure did not leave interrupted sync state")
	}

	autosyncAbortRebase = func(root string) error {
		_, err := gitutil.RunGit(root, "rebase", "--abort")
		return err
	}
	var out, errOut bytes.Buffer
	if code := Run([]string{"sync", "--root", state, "--abort"}, &out, &errOut); code != 0 {
		t.Fatalf("manual sync --abort exit code = %d, stderr = %s", code, errOut.String())
	}
	got = readAutosyncState(t, home, state)
	if got.PausedReason != autosync.PauseConflict || got.RecoveryCommand != "backlot sync" {
		t.Fatalf("manual abort did not transition urgent recovery to conflict pause: %+v", got)
	}
}

func TestManagedAutosyncPersistsFailureWhenLogWriteFails(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	home := t.TempDir()
	state := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, state)
	configureGitIdentity(t, state)
	mustRunGit(t, state, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing"))
	writeManagedAutosyncConfig(t, home, state)

	restore := stubAutosyncEnvironment(t, home, func(_, _ string) error { return nil })
	defer restore()
	oldWriteLog := autosyncWriteLog
	autosyncWriteLog = func(string, string) error { return errors.New("log denied") }
	defer func() { autosyncWriteLog = oldWriteLog }()

	if err := runManagedAutosync(state); err != nil {
		t.Fatalf("runManagedAutosync returned error after log failure: %v", err)
	}
	got := readAutosyncState(t, home, state)
	if got.Result != autosync.ResultFailed || got.ConsecutiveFailures != 1 {
		t.Fatalf("failure state was not persisted after log failure: %+v", got)
	}
}

func TestManagedAutosyncCountsArchivePreflightFailures(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, state)
	writeManagedAutosyncConfig(t, home, state)
	if err := os.RemoveAll(state); err != nil {
		t.Fatal(err)
	}

	notifications := 0
	restore := stubAutosyncEnvironment(t, home, func(_, _ string) error {
		notifications++
		return nil
	})
	defer restore()

	for i := 0; i < 3; i++ {
		if err := runManagedAutosync(state); err != nil {
			t.Fatalf("runManagedAutosync preflight failure %d returned error: %v", i+1, err)
		}
	}
	got := readAutosyncState(t, home, state)
	if got.FailureCategory != "archive" || got.ConsecutiveFailures != 3 {
		t.Fatalf("preflight failure state = %+v, want archive count 3", got)
	}
	if notifications != 1 {
		t.Fatalf("preflight notifications = %d, want 1", notifications)
	}
}

func TestManagedAutosyncBusyPreservesExistingFailureState(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, state)
	writeManagedAutosyncConfig(t, home, state)
	managedPaths, err := darwinAutosyncPaths(t, home, state)
	if err != nil {
		t.Fatal(err)
	}
	existing := autosync.State{
		Result:              autosync.ResultFailed,
		FailureCategory:     "fetch",
		ConsecutiveFailures: 2,
		LastError:           "offline",
	}
	if err := autosync.WriteState(managedPaths.StatePath, existing); err != nil {
		t.Fatal(err)
	}
	release, err := acquireSyncLock(state)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	restore := stubAutosyncEnvironment(t, home, func(_, _ string) error { return nil })
	defer restore()

	if err := runManagedAutosync(state); err != nil {
		t.Fatalf("runManagedAutosync busy returned error: %v", err)
	}
	got := readAutosyncState(t, home, state)
	if got.Result != autosync.ResultFailed || got.ConsecutiveFailures != 2 || got.LastError != "offline" {
		t.Fatalf("busy run overwrote existing failure state: %+v", got)
	}
}

func TestNotifyAutosyncLinuxIgnoresMissingNotifySend(t *testing.T) {
	oldGOOS := autosyncGOOS
	oldLookPath := autosyncLookPath
	autosyncGOOS = "linux"
	autosyncLookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() {
		autosyncGOOS = oldGOOS
		autosyncLookPath = oldLookPath
	}()

	if err := notifyAutosync("Backlot", "needs attention"); err != nil {
		t.Fatalf("notifyAutosync returned error for missing notify-send: %v", err)
	}
}

func writeManagedAutosyncConfig(t *testing.T, home, root string) {
	t.Helper()
	managedPaths, err := darwinAutosyncPaths(t, home, root)
	if err != nil {
		t.Fatal(err)
	}
	config := autosync.Config{
		SchemaVersion:   autosync.SchemaVersion,
		ManagedBy:       autosync.ManagedBy,
		Root:            managedPaths.Root,
		Binary:          "/usr/local/bin/backlot",
		Label:           managedPaths.Label,
		PlistPath:       managedPaths.PlistPath,
		IntervalSeconds: 900,
	}
	if err := autosync.WriteConfig(managedPaths.ConfigPath, config); err != nil {
		t.Fatal(err)
	}
}

func readAutosyncState(t *testing.T, home, root string) autosync.State {
	t.Helper()
	managedPaths, err := darwinAutosyncPaths(t, home, root)
	if err != nil {
		t.Fatal(err)
	}
	state, err := autosync.LoadState(managedPaths.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func stubAutosyncEnvironment(t *testing.T, home string, notifier func(string, string) error) func() {
	t.Helper()
	oldHome := autosyncHomeDir
	oldNow := autosyncNow
	oldNotify := autosyncNotify
	oldGOOS := autosyncGOOS
	autosyncHomeDir = func() (string, error) { return home, nil }
	autosyncNow = func() time.Time { return time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC) }
	autosyncNotify = notifier
	autosyncGOOS = "darwin"
	return func() {
		autosyncHomeDir = oldHome
		autosyncNow = oldNow
		autosyncNotify = oldNotify
		autosyncGOOS = oldGOOS
	}
}

func createAutosyncConflictSetup(t *testing.T) (string, string, string) {
	t.Helper()
	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	stateA := filepath.Join(tmp, "state-a")
	stateB := filepath.Join(tmp, "state-b")
	var out, errOut bytes.Buffer
	if code := Run([]string{"clone", remote, "--root", stateA}, &out, &errOut); code != 0 {
		t.Fatalf("clone A exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, stateA)
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"clone", remote, "--root", stateB}, &out, &errOut); code != 0 {
		t.Fatalf("clone B exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, stateB)

	notesA := filepath.Join(stateA, "github.com", "massivemoose", "ovek", "notes.md")
	if err := os.MkdirAll(filepath.Dir(notesA), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notesA, []byte("from A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", stateA, "-m", "A update"}, &out, &errOut); code != 0 {
		t.Fatalf("sync A exit code = %d, stderr = %s", code, errOut.String())
	}

	notesB := filepath.Join(stateB, "github.com", "massivemoose", "ovek", "notes.md")
	if err := os.MkdirAll(filepath.Dir(notesB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notesB, []byte("from B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return stateA, stateB, notesB
}
