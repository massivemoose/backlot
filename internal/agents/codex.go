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

const codexBacklotPermissionProfile = "workspace-backlot"

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

%s`, codexPermissionProfileConfig(backlotRoot))
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
	if codexConfigHasWritableRoot(text, backlotRoot) {
		return text, false, nil
	}
	if codexConfigHasDefaultPermissions(text) {
		return "", false, fmt.Errorf("Codex default_permissions is already set; paste this manually if you want Backlot added:\n%s", codexAgent{}.PersistentInstructions(backlotRoot))
	}
	if strings.TrimSpace(text) == "" {
		return codexPermissionProfileConfig(backlotRoot), true, nil
	}

	lines := strings.SplitAfter(text, "\n")
	if tableStart := codexFirstTableLine(lines); tableStart >= 0 {
		updated := append([]string{}, lines[:tableStart]...)
		if len(updated) > 0 && !strings.HasSuffix(updated[len(updated)-1], "\n") {
			updated[len(updated)-1] += "\n"
		}
		updated = append(updated, "default_permissions = \""+codexBacklotPermissionProfile+"\"\n\n")
		updated = append(updated, lines[tableStart:]...)
		if !strings.HasSuffix(updated[len(updated)-1], "\n") {
			updated[len(updated)-1] += "\n"
		}
		updated = append(updated, "\n"+codexPermissionProfileTables(backlotRoot))
		return strings.Join(updated, ""), true, nil
	}

	separator := ""
	if !strings.HasSuffix(text, "\n") {
		separator = "\n"
	}
	return text + separator + "\n" + codexPermissionProfileConfig(backlotRoot), true, nil
}

func codexConfigHasWritableRoot(text, backlotRoot string) bool {
	if codexPermissionProfileHasWritableRoot(text, backlotRoot) {
		return true
	}
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

func codexPermissionProfileConfig(backlotRoot string) string {
	return fmt.Sprintf(`default_permissions = %q

%s`, codexBacklotPermissionProfile, codexPermissionProfileTables(backlotRoot))
}

func codexPermissionProfileTables(backlotRoot string) string {
	return fmt.Sprintf(`[permissions.%s.workspace_roots]
%q = true

[permissions.%s.filesystem]
":minimal" = "read"
":tmpdir" = "write"
":slash_tmp" = "write"

[permissions.%s.filesystem.":workspace_roots"]
"." = "write"
".git" = "read"
".codex" = "read"
".agents" = "read"
`, codexBacklotPermissionProfile, backlotRoot, codexBacklotPermissionProfile, codexBacklotPermissionProfile)
}

func codexConfigHasDefaultPermissions(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		key, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "default_permissions" {
			return true
		}
	}
	return false
}

func codexPermissionProfileHasWritableRoot(text, backlotRoot string) bool {
	if !strings.Contains(text, `default_permissions = "`+codexBacklotPermissionProfile+`"`) {
		return false
	}
	lines := strings.SplitAfter(text, "\n")
	tableStart, tableEnd := codexTableRange(lines, "[permissions."+codexBacklotPermissionProfile+".workspace_roots]")
	if tableStart < 0 {
		return false
	}
	for i := tableStart + 1; i < tableEnd; i++ {
		key, value, ok := strings.Cut(strings.TrimSpace(lines[i]), "=")
		if !ok || strings.TrimSpace(value) != "true" {
			continue
		}
		unquoted, err := strconv.Unquote(strings.TrimSpace(key))
		if err == nil && unquoted == backlotRoot {
			return true
		}
	}
	return false
}

func codexSandboxTableRange(lines []string) (int, int) {
	return codexTableRange(lines, "[sandbox_workspace_write]")
}

func codexFirstTableLine(lines []string) int {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			return i
		}
	}
	return -1
}

func codexTableRange(lines []string, table string) (int, int) {
	tableStart := -1
	tableEnd := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == table {
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
