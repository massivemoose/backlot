package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSupportedAgentsRenderCommandsAndInstructions(t *testing.T) {
	tests := []struct {
		id              string
		name            string
		commandContains string
		configContains  []string
	}{
		{
			id:              "codex",
			name:            "Codex CLI",
			commandContains: "codex --cd /repo --add-dir /state",
			configContains: []string{
				`~/.codex/config.toml`,
				`[sandbox_workspace_write]`,
				`writable_roots = ["/state"]`,
			},
		},
		{
			id:              "claude",
			name:            "Claude Code",
			commandContains: "claude --add-dir /state",
			configContains: []string{
				`~/.claude/settings.json`,
				`"additionalDirectories": [`,
				`"/state"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			agent, ok := ByID(tt.id)
			if !ok {
				t.Fatalf("ByID(%q) not found", tt.id)
			}
			if agent.Name() != tt.name {
				t.Fatalf("Name() = %q, want %q", agent.Name(), tt.name)
			}
			if got := agent.OneSessionCommand("/repo", "/state"); !strings.Contains(got, tt.commandContains) {
				t.Fatalf("OneSessionCommand() = %q, want it to contain %q", got, tt.commandContains)
			}
			instructions := agent.PersistentInstructions("/state")
			for _, want := range tt.configContains {
				if !strings.Contains(instructions, want) {
					t.Fatalf("PersistentInstructions() missing %q:\n%s", want, instructions)
				}
			}
		})
	}
}

func TestCodexApplyConfigCreatesBackupAndAvoidsDuplicateRoots(t *testing.T) {
	agent, ok := ByID("codex")
	if !ok {
		t.Fatal("codex agent missing")
	}
	home := t.TempDir()
	env := Environment{HomeDir: home}
	when := time.Date(2026, 6, 2, 12, 34, 56, 0, time.UTC)

	result, err := agent.ApplyConfig(env, "/state", when)
	if err != nil {
		t.Fatalf("ApplyConfig() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("ApplyConfig() Changed = false, want true")
	}
	if result.BackupPath == "" {
		t.Fatal("ApplyConfig() did not report a backup path")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	configPath := filepath.Join(home, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := string(data); !strings.Contains(got, `writable_roots = ["/state"]`) {
		t.Fatalf("config missing writable root:\n%s", got)
	}

	result, err = agent.ApplyConfig(env, "/state", when)
	if err != nil {
		t.Fatalf("second ApplyConfig() error = %v", err)
	}
	if result.Changed {
		t.Fatal("second ApplyConfig() Changed = true, want false for duplicate root")
	}
	data, err = os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after second apply: %v", err)
	}
	if count := strings.Count(string(data), "/state"); count != 1 {
		t.Fatalf("config contains /state %d times, want once:\n%s", count, data)
	}
}

func TestApplyConfigRefusesBroadRoots(t *testing.T) {
	home := t.TempDir()
	env := Environment{HomeDir: home}
	for _, agentID := range []string{"codex", "claude"} {
		t.Run(agentID, func(t *testing.T) {
			agent, ok := ByID(agentID)
			if !ok {
				t.Fatalf("%s agent missing", agentID)
			}
			if _, err := agent.ApplyConfig(env, home, time.Now()); err == nil {
				t.Fatal("ApplyConfig() error = nil, want refusal for home directory grant")
			}
			if _, err := agent.ApplyConfig(env, string(filepath.Separator), time.Now()); err == nil {
				t.Fatal("ApplyConfig() error = nil, want refusal for filesystem root grant")
			}
		})
	}
}

func TestCodexApplyConfigRefusesComplexWritableRoots(t *testing.T) {
	agent, ok := ByID("codex")
	if !ok {
		t.Fatal("codex agent missing")
	}
	home := t.TempDir()
	configDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	original := "[sandbox_workspace_write]\nwritable_roots = [\"/existing\", \"/other\"]\n"
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := agent.ApplyConfig(Environment{HomeDir: home}, "/state", time.Now())
	if err == nil {
		t.Fatal("ApplyConfig() error = nil, want refusal for complex TOML")
	}
	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != original {
		t.Fatalf("config changed after refusal:\n%s", data)
	}
}

func TestCodexApplyConfigInsertsWritableRootsAfterTableWithoutTrailingNewline(t *testing.T) {
	agent, ok := ByID("codex")
	if !ok {
		t.Fatal("codex agent missing")
	}
	home := t.TempDir()
	configDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[sandbox_workspace_write]"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := agent.ApplyConfig(Environment{HomeDir: home}, "/state", time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ApplyConfig() error = %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "[sandbox_workspace_write]\nwritable_roots = [\"/state\"]\n"
	if string(data) != want {
		t.Fatalf("config = %q, want %q", data, want)
	}
}

func TestClaudeApplyConfigMergesPermissions(t *testing.T) {
	agent, ok := ByID("claude")
	if !ok {
		t.Fatal("claude agent missing")
	}
	home := t.TempDir()
	settingsDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(settingsDir, "settings.json")
	original := `{"theme":"dark","permissions":{"defaultMode":"acceptEdits"}}`
	if err := os.WriteFile(settingsPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := agent.ApplyConfig(Environment{HomeDir: home}, "/state", time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ApplyConfig() error = %v", err)
	}
	if !result.Changed {
		t.Fatal("ApplyConfig() Changed = false, want true")
	}
	if result.BackupPath == "" {
		t.Fatal("ApplyConfig() did not report a backup path")
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"theme": "dark"`, `"defaultMode": "acceptEdits"`, `"additionalDirectories"`, `"/state"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("settings missing %q:\n%s", want, text)
		}
	}
}
