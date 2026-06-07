package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/massivemoose/backlot/internal/autosync"
)

type autosyncHealth struct {
	Enabled  bool
	Summary  string
	Problem  string
	Recovery string
}

func collectAutosyncHealth(root string) (autosyncHealth, error) {
	home, err := autosyncHomeDir()
	if err != nil {
		return autosyncHealth{}, err
	}
	managedPaths, err := autosync.ResolvePaths(home, root)
	if err != nil {
		return autosyncHealth{}, err
	}
	config, err := autosync.LoadConfig(managedPaths.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return autosyncHealth{}, nil
	}
	if err != nil {
		return autosyncHealth{}, err
	}
	if err := autosync.ValidateManagedConfig(config, managedPaths); err != nil {
		return autosyncHealth{}, err
	}
	if err := verifyAutosyncOwnership(managedPaths, true); err != nil {
		return autosyncHealth{}, err
	}
	health := autosyncHealth{Enabled: true, Summary: "enabled"}
	if info, err := os.Lstat(managedPaths.PlistPath); errors.Is(err, os.ErrNotExist) {
		health.Summary = "configured but LaunchAgent is missing"
		health.Problem = "Auto-sync LaunchAgent file is missing"
		health.Recovery = fmt.Sprintf("backlot autosync enable --root %s", root)
	} else if err != nil {
		return autosyncHealth{}, err
	} else if !info.Mode().IsRegular() {
		return autosyncHealth{}, fmt.Errorf("managed auto-sync file %s is not a regular file", managedPaths.PlistPath)
	}
	if _, err := os.Stat(config.Binary); errors.Is(err, os.ErrNotExist) {
		health.Summary = "configured but binary is missing"
		health.Problem = "Auto-sync binary is missing"
		health.Recovery = fmt.Sprintf("backlot autosync enable --root %s", root)
	} else if err != nil {
		return autosyncHealth{}, err
	}
	loaded, err := autosyncLoaded(managedPaths.Label)
	if err != nil {
		return autosyncHealth{}, fmt.Errorf("inspect auto-sync LaunchAgent: %w", err)
	}
	if !loaded {
		health.Summary = "configured but not loaded"
		health.Problem = "Auto-sync LaunchAgent is not loaded"
		health.Recovery = fmt.Sprintf("backlot autosync enable --root %s", root)
	}
	state, err := autosync.LoadState(managedPaths.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return health, nil
	}
	if err != nil {
		return autosyncHealth{}, err
	}
	if state.PausedReason != "" {
		pausedSummary := "paused: " + state.PausedReason
		pausedProblem := "Auto-sync is paused: " + state.PausedReason
		if health.Problem != "" {
			health.Summary = pausedSummary + "; " + health.Summary
			health.Problem = pausedProblem + "; " + health.Problem
			health.Recovery = combineAutosyncRecovery(state.RecoveryCommand, health.Recovery)
		} else {
			health.Summary = pausedSummary
			health.Problem = pausedProblem
			health.Recovery = state.RecoveryCommand
		}
		return health, nil
	}
	if state.Result == autosync.ResultFailed {
		health.Summary = "failed: " + state.FailureCategory
		health.Problem = "Auto-sync last run failed: " + state.FailureCategory
		health.Recovery = fmt.Sprintf("backlot autosync status --root %s", root)
	}
	return health, nil
}

func combineAutosyncRecovery(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" || second == first {
		return first
	}
	return first + "; then " + second
}
