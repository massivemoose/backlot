package autosync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	SchemaVersion = 1
	ManagedBy     = "backlot"

	ResultSuccess       = "success"
	ResultFailed        = "failed"
	ResultSkippedBusy   = "skipped-busy"
	ResultSkippedPaused = "skipped-paused"

	PauseConflict       = "conflict"
	PauseUrgentRecovery = "urgent-recovery"

	conflictNotificationCategory = "conflict"
)

type Paths struct {
	ID         string
	Label      string
	Root       string
	RuntimeDir string
	ConfigPath string
	StatePath  string
	LogPath    string
	PlistPath  string
}

type Config struct {
	SchemaVersion   int    `json:"schema_version"`
	ManagedBy       string `json:"managed_by"`
	Root            string `json:"root"`
	Binary          string `json:"binary"`
	Label           string `json:"label"`
	PlistPath       string `json:"plist_path"`
	IntervalSeconds int    `json:"interval_seconds"`
}

type State struct {
	LastRun                  time.Time `json:"last_run,omitempty"`
	LastSuccess              time.Time `json:"last_success,omitempty"`
	Result                   string    `json:"result,omitempty"`
	PausedReason             string    `json:"paused_reason,omitempty"`
	ConsecutiveFailures      int       `json:"consecutive_failures,omitempty"`
	FailureCategory          string    `json:"failure_category,omitempty"`
	LastError                string    `json:"last_error,omitempty"`
	LastNotification         time.Time `json:"last_notification,omitempty"`
	LastNotificationError    string    `json:"last_notification_error,omitempty"`
	LastNotificationCategory string    `json:"last_notification_category,omitempty"`
	PendingNotification      string    `json:"pending_notification,omitempty"`
	ConflictPaths            []string  `json:"conflict_paths,omitempty"`
	LocalHead                string    `json:"local_head,omitempty"`
	RemoteHead               string    `json:"remote_head,omitempty"`
	RecoveryCommand          string    `json:"recovery_command,omitempty"`
}

func ResolvePaths(homeDir, root string) (Paths, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, err
	}
	canonicalRoot, err := canonicalPath(absRoot)
	if err != nil {
		return Paths{}, err
	}
	canonicalRoot = filepath.Clean(canonicalRoot)
	sum := sha256.Sum256([]byte(canonicalRoot))
	id := hex.EncodeToString(sum[:8])
	label := "com.massivemoose.backlot.autosync." + id
	runtimeDir := filepath.Join(homeDir, "Library", "Application Support", "Backlot", "autosync", id)
	return Paths{
		ID:         id,
		Label:      label,
		Root:       canonicalRoot,
		RuntimeDir: runtimeDir,
		ConfigPath: filepath.Join(runtimeDir, "config.json"),
		StatePath:  filepath.Join(runtimeDir, "last-run.json"),
		LogPath:    filepath.Join(homeDir, "Library", "Logs", "Backlot", "autosync-"+id+".log"),
		PlistPath:  filepath.Join(homeDir, "Library", "LaunchAgents", label+".plist"),
	}, nil
}

func canonicalPath(path string) (string, error) {
	current := filepath.Clean(path)
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			parts := append([]string{resolved}, suffix...)
			return filepath.Join(parts...), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path), nil
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
}

func ValidateManagedConfig(config Config, paths Paths) error {
	if config.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported autosync configuration schema version %d", config.SchemaVersion)
	}
	if config.ManagedBy != ManagedBy {
		return fmt.Errorf("autosync configuration is not managed by Backlot")
	}
	if config.Root != paths.Root || config.Label != paths.Label || config.PlistPath != paths.PlistPath {
		return fmt.Errorf("autosync configuration does not match Backlot root %s", paths.Root)
	}
	return nil
}

func LoadConfig(path string) (Config, error) {
	var config Config
	if err := readJSON(path, &config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func WriteConfig(path string, config Config) error {
	return writeJSON(path, config)
}

func LoadState(path string) (State, error) {
	var state State
	if err := readJSON(path, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func WriteState(path string, state State) error {
	return writeJSON(path, state)
}

func RemoveRuntime(paths Paths) error {
	if err := os.RemoveAll(paths.RuntimeDir); err != nil {
		return err
	}
	if err := os.Remove(paths.LogPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *State) RecordSuccess(now time.Time) {
	s.LastRun = now
	s.LastSuccess = now
	s.Result = ResultSuccess
	s.PausedReason = ""
	s.ConsecutiveFailures = 0
	s.FailureCategory = ""
	s.LastError = ""
	s.LastNotification = time.Time{}
	s.LastNotificationError = ""
	s.LastNotificationCategory = ""
	s.PendingNotification = ""
	s.ConflictPaths = nil
	s.LocalHead = ""
	s.RemoteHead = ""
	s.RecoveryCommand = ""
}

func (s *State) RecordFailure(now time.Time, category, message string) bool {
	s.LastRun = now
	s.Result = ResultFailed
	s.LastError = message
	s.PausedReason = ""
	s.ConflictPaths = nil
	s.LocalHead = ""
	s.RemoteHead = ""
	s.RecoveryCommand = ""
	if s.FailureCategory == category {
		s.ConsecutiveFailures++
	} else {
		s.FailureCategory = category
		s.ConsecutiveFailures = 1
	}
	if s.ConsecutiveFailures >= 3 && s.LastNotificationCategory != category {
		s.PendingNotification = category
		return true
	}
	return false
}

func (s *State) RecordConflict(now time.Time, paths []string, localHead, remoteHead, recoveryCommand string) bool {
	notify := s.PausedReason != PauseConflict || s.LastNotificationCategory != conflictNotificationCategory
	s.LastRun = now
	s.Result = ResultFailed
	s.PausedReason = PauseConflict
	s.FailureCategory = conflictNotificationCategory
	s.ConsecutiveFailures = 1
	s.LastError = "local and remote Backlot changes conflict"
	s.ConflictPaths = append([]string(nil), paths...)
	s.LocalHead = localHead
	s.RemoteHead = remoteHead
	s.RecoveryCommand = recoveryCommand
	if notify {
		s.PendingNotification = conflictNotificationCategory
	}
	return notify
}

func (s *State) RecordUrgentRecovery(now time.Time, paths []string, localHead, remoteHead, message, recoveryCommand string) bool {
	s.LastRun = now
	s.Result = ResultFailed
	s.PausedReason = PauseUrgentRecovery
	s.FailureCategory = conflictNotificationCategory
	s.ConsecutiveFailures = 1
	s.LastError = message
	s.ConflictPaths = append([]string(nil), paths...)
	s.LocalHead = localHead
	s.RemoteHead = remoteHead
	s.RecoveryCommand = recoveryCommand
	if s.LastNotificationCategory != conflictNotificationCategory {
		s.PendingNotification = conflictNotificationCategory
		return true
	}
	return false
}

func (s *State) RecordSkippedBusy(now time.Time) {
	s.LastRun = now
	s.Result = ResultSkippedBusy
}

func (s *State) RecordSkippedPaused(now time.Time) {
	s.LastRun = now
	s.Result = ResultSkippedPaused
}

func (s *State) RecordAbortRecovery(now time.Time) {
	if s.PausedReason != PauseUrgentRecovery {
		return
	}
	s.LastRun = now
	s.Result = ResultFailed
	s.PausedReason = PauseConflict
	s.LastError = "local and remote Backlot changes conflict"
	s.RecoveryCommand = "backlot sync"
}

func (s *State) RecordNotification(now time.Time, err error) {
	s.LastNotification = now
	s.LastNotificationCategory = s.PendingNotification
	s.PendingNotification = ""
	s.LastNotificationError = ""
	if err != nil {
		s.LastNotificationError = err.Error()
	}
}

func (s State) Paused() bool {
	return s.PausedReason != ""
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
