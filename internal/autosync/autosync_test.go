package autosync

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolvePathsUsesStableCanonicalRootIdentity(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "archive")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "archive-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}

	direct, err := ResolvePathsForPlatform(home, root, "darwin")
	if err != nil {
		t.Fatalf("ResolvePaths direct returned error: %v", err)
	}
	linked, err := ResolvePathsForPlatform(home, link, "darwin")
	if err != nil {
		t.Fatalf("ResolvePaths linked returned error: %v", err)
	}
	if direct.ID != linked.ID || direct.Root != linked.Root {
		t.Fatalf("canonical paths differ: direct=%+v linked=%+v", direct, linked)
	}
	if filepath.Base(direct.PlistPath) != direct.Label+".plist" {
		t.Fatalf("plist path = %q, want label-based filename", direct.PlistPath)
	}
	if filepath.Dir(direct.ConfigPath) != direct.RuntimeDir {
		t.Fatalf("config path = %q, want runtime dir %q", direct.ConfigPath, direct.RuntimeDir)
	}
}

func TestResolvePathsAllowsMissingRoot(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "missing")

	paths, err := ResolvePathsForPlatform(home, root, "darwin")
	if err != nil {
		t.Fatalf("ResolvePaths returned error for missing root: %v", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(parent, filepath.Base(root))
	if paths.Root != want {
		t.Fatalf("root = %q, want %q", paths.Root, want)
	}
}

func TestResolvePathsForLinuxUsesSystemdUserPaths(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "archive")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	paths, err := ResolvePathsForPlatform(home, root, "linux")
	if err != nil {
		t.Fatalf("ResolvePathsForPlatform returned error: %v", err)
	}
	if paths.Scheduler != SchedulerSystemdUser {
		t.Fatalf("Scheduler = %q, want %q", paths.Scheduler, SchedulerSystemdUser)
	}
	if paths.RuntimeDir != filepath.Join(home, ".local", "state", "backlot", "autosync", paths.ID) {
		t.Fatalf("RuntimeDir = %q, want XDG state dir", paths.RuntimeDir)
	}
	if paths.ConfigPath != filepath.Join(paths.RuntimeDir, "config.json") {
		t.Fatalf("ConfigPath = %q, want runtime config", paths.ConfigPath)
	}
	if paths.StatePath != filepath.Join(paths.RuntimeDir, "last-run.json") {
		t.Fatalf("StatePath = %q, want runtime state", paths.StatePath)
	}
	if paths.LogPath != filepath.Join(home, ".local", "state", "backlot", "autosync", "autosync-"+paths.ID+".log") {
		t.Fatalf("LogPath = %q, want XDG state log", paths.LogPath)
	}
	if paths.ServiceName != paths.Label+".service" {
		t.Fatalf("ServiceName = %q, want label-based service name", paths.ServiceName)
	}
	if paths.TimerName != paths.Label+".timer" {
		t.Fatalf("TimerName = %q, want label-based timer name", paths.TimerName)
	}
	if paths.ServicePath != filepath.Join(home, ".config", "systemd", "user", paths.ServiceName) {
		t.Fatalf("ServicePath = %q, want user systemd service path", paths.ServicePath)
	}
	if paths.TimerPath != filepath.Join(home, ".config", "systemd", "user", paths.TimerName) {
		t.Fatalf("TimerPath = %q, want user systemd timer path", paths.TimerPath)
	}
	if paths.PlistPath != "" {
		t.Fatalf("PlistPath = %q, want empty on Linux", paths.PlistPath)
	}
}

func TestConfigRoundTripAndManagedOwnership(t *testing.T) {
	paths, err := ResolvePathsForPlatform(t.TempDir(), t.TempDir(), "darwin")
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		SchemaVersion:   SchemaVersion,
		ManagedBy:       ManagedBy,
		Root:            paths.Root,
		Binary:          "/usr/local/bin/backlot",
		Label:           paths.Label,
		PlistPath:       paths.PlistPath,
		IntervalSeconds: 900,
	}
	if err := WriteConfig(paths.ConfigPath, config); err != nil {
		t.Fatalf("WriteConfig returned error: %v", err)
	}
	got, err := LoadConfig(paths.ConfigPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got != config {
		t.Fatalf("LoadConfig = %+v, want %+v", got, config)
	}
	if err := ValidateManagedConfig(got, paths); err != nil {
		t.Fatalf("ValidateManagedConfig returned error: %v", err)
	}

	got.ManagedBy = "someone-else"
	if err := ValidateManagedConfig(got, paths); err == nil {
		t.Fatal("ValidateManagedConfig accepted foreign ownership")
	}
	got = config
	got.PlistPath = filepath.Join(t.TempDir(), "foreign.plist")
	if err := ValidateManagedConfig(got, paths); err == nil {
		t.Fatal("ValidateManagedConfig accepted foreign plist path")
	}
}

func TestSystemdConfigRoundTripAndManagedOwnership(t *testing.T) {
	paths, err := ResolvePathsForPlatform(t.TempDir(), t.TempDir(), "linux")
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		SchemaVersion:   SchemaVersion,
		ManagedBy:       ManagedBy,
		Scheduler:       SchedulerSystemdUser,
		Root:            paths.Root,
		Binary:          "/usr/local/bin/backlot",
		Label:           paths.Label,
		ServiceName:     paths.ServiceName,
		TimerName:       paths.TimerName,
		ServicePath:     paths.ServicePath,
		TimerPath:       paths.TimerPath,
		IntervalSeconds: 900,
	}
	if err := WriteConfig(paths.ConfigPath, config); err != nil {
		t.Fatalf("WriteConfig returned error: %v", err)
	}
	got, err := LoadConfig(paths.ConfigPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got != config {
		t.Fatalf("LoadConfig = %+v, want %+v", got, config)
	}
	if err := ValidateManagedConfig(got, paths); err != nil {
		t.Fatalf("ValidateManagedConfig returned error: %v", err)
	}

	got.ServicePath = filepath.Join(t.TempDir(), "foreign.service")
	if err := ValidateManagedConfig(got, paths); err == nil {
		t.Fatal("ValidateManagedConfig accepted foreign service path")
	}
}

func TestMissingSchedulerConfigDefaultsToLaunchd(t *testing.T) {
	paths, err := ResolvePathsForPlatform(t.TempDir(), t.TempDir(), "darwin")
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		SchemaVersion:   SchemaVersion,
		ManagedBy:       ManagedBy,
		Root:            paths.Root,
		Binary:          "/usr/local/bin/backlot",
		Label:           paths.Label,
		PlistPath:       paths.PlistPath,
		IntervalSeconds: 900,
	}
	if err := ValidateManagedConfig(config, paths); err != nil {
		t.Fatalf("ValidateManagedConfig rejected legacy launchd config: %v", err)
	}
}

func TestStateFailureNotificationPolicy(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	var state State
	if state.RecordFailure(now, "fetch", "offline") {
		t.Fatal("first failure requested notification")
	}
	if state.RecordFailure(now.Add(time.Minute), "fetch", "offline") {
		t.Fatal("second failure requested notification")
	}
	if !state.RecordFailure(now.Add(2*time.Minute), "fetch", "offline") {
		t.Fatal("third failure did not request notification")
	}
	state.RecordNotification(now.Add(2*time.Minute), nil)
	if state.RecordFailure(now.Add(3*time.Minute), "fetch", "offline") {
		t.Fatal("repeated failure requested duplicate notification")
	}
	if state.RecordFailure(now.Add(4*time.Minute), "push", "denied") {
		t.Fatal("first changed-category failure requested notification")
	}
	if state.RecordFailure(now.Add(5*time.Minute), "push", "denied") {
		t.Fatal("second changed-category failure requested notification")
	}
	if !state.RecordFailure(now.Add(6*time.Minute), "push", "denied") {
		t.Fatal("third changed-category failure did not request notification")
	}
}

func TestStateConflictPausesOnceAndSuccessClearsFailures(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	var state State
	if !state.RecordConflict(now, []string{"notes.md"}, "local", "remote", "backlot sync") {
		t.Fatal("first conflict did not request notification")
	}
	state.RecordNotification(now, errors.New("notifications denied"))
	if state.RecordConflict(now.Add(time.Minute), []string{"notes.md"}, "local", "remote", "backlot sync") {
		t.Fatal("repeated conflict requested duplicate notification")
	}
	if state.PausedReason != PauseConflict {
		t.Fatalf("PausedReason = %q, want %q", state.PausedReason, PauseConflict)
	}

	state.RecordSuccess(now.Add(2 * time.Minute))
	if state.PausedReason != "" || state.ConsecutiveFailures != 0 || state.FailureCategory != "" {
		t.Fatalf("RecordSuccess did not clear failure state: %+v", state)
	}
	if !state.LastNotification.IsZero() || state.LastNotificationError != "" || state.LastNotificationCategory != "" {
		t.Fatalf("RecordSuccess did not clear previous alert state: %+v", state)
	}
	if state.LastSuccess.IsZero() {
		t.Fatal("RecordSuccess did not set LastSuccess")
	}
}

func TestStateRoundTripAndRuntimeRemoval(t *testing.T) {
	paths, err := ResolvePathsForPlatform(t.TempDir(), t.TempDir(), "darwin")
	if err != nil {
		t.Fatal(err)
	}
	state := State{Result: ResultSuccess, LastRun: time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)}
	if err := WriteState(paths.StatePath, state); err != nil {
		t.Fatalf("WriteState returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.LogPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.LogPath, []byte("error"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(paths.StatePath)
	if err != nil {
		t.Fatalf("LoadState returned error: %v", err)
	}
	if got.Result != ResultSuccess || !got.LastRun.Equal(state.LastRun) {
		t.Fatalf("LoadState = %+v, want %+v", got, state)
	}
	if err := RemoveRuntime(paths); err != nil {
		t.Fatalf("RemoveRuntime returned error: %v", err)
	}
	if _, err := os.Stat(paths.RuntimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime dir still exists: %v", err)
	}
	if _, err := os.Stat(paths.LogPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("log still exists: %v", err)
	}
}
