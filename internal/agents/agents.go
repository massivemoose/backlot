package agents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Environment struct {
	HomeDir   string
	CodexHome string
}

type ConfigStatus struct {
	ConfigPath string
	Exists     bool
	HasGrant   bool
	Message    string
}

type ApplyResult struct {
	ConfigPath string
	BackupPath string
	Changed    bool
	Message    string
}

type Agent interface {
	ID() string
	Name() string
	OneSessionCommand(repoRoot, backlotRoot string) string
	PersistentInstructions(backlotRoot string) string
	ConfigStatus(env Environment, backlotRoot string) ConfigStatus
	ApplyConfig(env Environment, backlotRoot string, now time.Time) (ApplyResult, error)
}

func All() []Agent {
	return []Agent{
		codexAgent{},
		claudeAgent{},
	}
}

func ByID(id string) (Agent, bool) {
	for _, agent := range All() {
		if agent.ID() == id {
			return agent, true
		}
	}
	return nil, false
}

func DefaultEnvironment() (Environment, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Environment{}, err
	}
	return Environment{
		HomeDir:   home,
		CodexHome: os.Getenv("CODEX_HOME"),
	}, nil
}

func backupFile(path string, now time.Time) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	backup := fmt.Sprintf("%s.backlot-%s.bak", path, now.UTC().Format("20060102150405"))
	if _, err := os.Stat(backup); err == nil {
		return "", fmt.Errorf("backup path already exists: %s", backup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data = nil
	} else if err != nil {
		return "", err
	}
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		return "", err
	}
	return backup, nil
}

func ensureGrantRootSafe(env Environment, backlotRoot string) error {
	cleanRoot := filepath.Clean(backlotRoot)
	if cleanRoot == string(filepath.Separator) {
		return errors.New("refusing to grant filesystem root to an agent")
	}
	if env.HomeDir != "" && cleanRoot == filepath.Clean(env.HomeDir) {
		return errors.New("refusing to grant the entire home directory to an agent")
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
