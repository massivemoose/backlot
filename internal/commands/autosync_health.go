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
	health := autosyncHealth{Enabled: true, Summary: "enabled"}
	state, err := autosync.LoadState(managedPaths.StatePath)
	if errors.Is(err, os.ErrNotExist) {
		return health, nil
	}
	if err != nil {
		return autosyncHealth{}, err
	}
	if state.PausedReason != "" {
		health.Summary = "paused: " + state.PausedReason
		health.Problem = "Auto-sync is paused: " + state.PausedReason
		health.Recovery = state.RecoveryCommand
		return health, nil
	}
	if state.Result == autosync.ResultFailed {
		health.Summary = "failed: " + state.FailureCategory
		health.Problem = "Auto-sync last run failed: " + state.FailureCategory
		health.Recovery = fmt.Sprintf("backlot autosync status --root %s", root)
	}
	return health, nil
}
