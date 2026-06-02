package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type codexAgent struct{}

func (codexAgent) ID() string {
	return "codex"
}

func (codexAgent) Name() string {
	return "Codex CLI"
}

func (codexAgent) OneSessionCommand(repoRoot, backlotRoot string) string {
	return fmt.Sprintf("codex --cd %s --add-dir %s", repoRoot, backlotRoot)
}

func (codexAgent) PersistentInstructions(backlotRoot string) string {
	return fmt.Sprintf(`Add this to ~/.codex/config.toml:

[sandbox_workspace_write]
writable_roots = [%q]
`, backlotRoot)
}

func (codexAgent) ConfigStatus(env Environment, backlotRoot string) ConfigStatus {
	path := codexConfigPath(env)
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigStatus{ConfigPath: path, Message: "config not found"}
	}
	return ConfigStatus{
		ConfigPath: path,
		Exists:     true,
		HasGrant:   codexConfigHasWritableRoot(string(data), backlotRoot),
		Message:    "config found",
	}
}

func (codexAgent) ApplyConfig(env Environment, backlotRoot string, now time.Time) (ApplyResult, error) {
	if err := ensureGrantRootSafe(env, backlotRoot); err != nil {
		return ApplyResult{}, err
	}
	path := codexConfigPath(env)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data = nil
	} else if err != nil {
		return ApplyResult{}, err
	}
	updated, changed, err := updateCodexConfig(string(data), backlotRoot)
	if err != nil {
		return ApplyResult{ConfigPath: path, Changed: false, Message: codexAgent{}.PersistentInstructions(backlotRoot)}, err
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
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{ConfigPath: path, BackupPath: backup, Changed: true, Message: "Updated Codex config."}, nil
}

func codexConfigPath(env Environment) string {
	if strings.TrimSpace(env.CodexHome) != "" {
		return filepath.Join(env.CodexHome, "config.toml")
	}
	return filepath.Join(env.HomeDir, ".codex", "config.toml")
}

func updateCodexConfig(text, backlotRoot string) (string, bool, error) {
	quoted := fmt.Sprintf("%q", backlotRoot)
	if codexConfigHasWritableRoot(text, backlotRoot) {
		return text, false, nil
	}
	if strings.TrimSpace(text) == "" {
		return "[sandbox_workspace_write]\nwritable_roots = [" + quoted + "]\n", true, nil
	}

	lines := strings.SplitAfter(text, "\n")
	tableStart, tableEnd := codexSandboxTableRange(lines)
	if tableStart < 0 {
		separator := ""
		if !strings.HasSuffix(text, "\n") {
			separator = "\n"
		}
		return text + separator + "\n[sandbox_workspace_write]\nwritable_roots = [" + quoted + "]\n", true, nil
	}

	for i := tableStart + 1; i < tableEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) != "writable_roots" {
			continue
		}
		if trimmed == "writable_roots = []" {
			lines[i] = strings.Replace(lines[i], "[]", "["+quoted+"]", 1)
			return strings.Join(lines, ""), true, nil
		}
		return "", false, fmt.Errorf("Codex writable_roots is already set; paste this manually if you want Backlot added:\n%s", codexAgent{}.PersistentInstructions(backlotRoot))
	}

	insert := "writable_roots = [" + quoted + "]\n"
	updated := append([]string{}, lines[:tableStart+1]...)
	if !strings.HasSuffix(updated[len(updated)-1], "\n") {
		updated[len(updated)-1] += "\n"
	}
	updated = append(updated, insert)
	updated = append(updated, lines[tableStart+1:]...)
	return strings.Join(updated, ""), true, nil
}

func codexConfigHasWritableRoot(text, backlotRoot string) bool {
	lines := strings.SplitAfter(text, "\n")
	tableStart, tableEnd := codexSandboxTableRange(lines)
	if tableStart < 0 {
		return false
	}
	for i := tableStart + 1; i < tableEnd; i++ {
		key, value, ok := strings.Cut(strings.TrimSpace(lines[i]), "=")
		if !ok || strings.TrimSpace(key) != "writable_roots" {
			continue
		}
		return codexWritableRootsValueHasPath(value, backlotRoot)
	}
	return false
}

func codexSandboxTableRange(lines []string) (int, int) {
	tableStart := -1
	tableEnd := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[sandbox_workspace_write]" {
			tableStart = i
			continue
		}
		if tableStart >= 0 && i > tableStart && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			tableEnd = i
			break
		}
	}
	return tableStart, tableEnd
}

func codexWritableRootsValueHasPath(value, backlotRoot string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") {
		return false
	}
	end := strings.Index(value, "]")
	if end < 0 {
		return false
	}
	for _, entry := range strings.Split(value[1:end], ",") {
		unquoted, err := strconv.Unquote(strings.TrimSpace(entry))
		if err == nil && unquoted == backlotRoot {
			return true
		}
	}
	return false
}
