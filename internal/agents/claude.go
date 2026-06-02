package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type claudeAgent struct{}

func (claudeAgent) ID() string {
	return "claude"
}

func (claudeAgent) Name() string {
	return "Claude Code"
}

func (claudeAgent) OneSessionCommand(repoRoot, backlotRoot string) string {
	return fmt.Sprintf("claude --add-dir %s", backlotRoot)
}

func (claudeAgent) PersistentInstructions(backlotRoot string) string {
	return fmt.Sprintf(`Add this to ~/.claude/settings.json:

{
  "permissions": {
    "additionalDirectories": [
      %q
    ]
  }
}
`, backlotRoot)
}

func (claudeAgent) ConfigStatus(env Environment, backlotRoot string) ConfigStatus {
	path := claudeConfigPath(env)
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigStatus{ConfigPath: path, Message: "config not found"}
	}
	return ConfigStatus{
		ConfigPath: path,
		Exists:     true,
		HasGrant:   stringContainsJSONPath(data, backlotRoot),
		Message:    "config found",
	}
}

func (claudeAgent) ApplyConfig(env Environment, backlotRoot string, now time.Time) (ApplyResult, error) {
	if err := ensureGrantRootSafe(env, backlotRoot); err != nil {
		return ApplyResult{}, err
	}
	path := claudeConfigPath(env)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data = []byte("{}")
	} else if err != nil {
		return ApplyResult{}, err
	}
	updated, changed, err := updateClaudeSettings(data, backlotRoot)
	if err != nil {
		return ApplyResult{}, err
	}
	if !changed {
		return ApplyResult{ConfigPath: path, Changed: false, Message: "Backlot root is already configured."}, nil
	}
	backup, err := backupFile(path, now)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ApplyResult{}, err
	}
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{ConfigPath: path, BackupPath: backup, Changed: true, Message: "Updated Claude config."}, nil
}

func claudeConfigPath(env Environment) string {
	return filepath.Join(env.HomeDir, ".claude", "settings.json")
}

func updateClaudeSettings(data []byte, backlotRoot string) ([]byte, bool, error) {
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, false, err
	}
	permissions, _ := settings["permissions"].(map[string]any)
	if permissions == nil {
		permissions = map[string]any{}
		settings["permissions"] = permissions
	}

	var dirs []string
	if existing, ok := permissions["additionalDirectories"].([]any); ok {
		for _, value := range existing {
			if text, ok := value.(string); ok {
				dirs = append(dirs, text)
			}
		}
	}
	if containsString(dirs, backlotRoot) {
		return data, false, nil
	}
	dirs = append(dirs, backlotRoot)
	permissions["additionalDirectories"] = dirs

	updated, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(updated, '\n'), true, nil
}

func stringContainsJSONPath(data []byte, backlotRoot string) bool {
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return false
	}
	permissions, _ := settings["permissions"].(map[string]any)
	if permissions == nil {
		return false
	}
	existing, _ := permissions["additionalDirectories"].([]any)
	for _, value := range existing {
		if text, ok := value.(string); ok && text == backlotRoot {
			return true
		}
	}
	return false
}
