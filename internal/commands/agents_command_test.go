package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestAgentsSetupPreviewListsSupportedAgents(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"agents", "setup", "--root", "/state"}, &out, &errOut); code != 0 {
		t.Fatalf("agents setup exit code = %d, stderr = %s", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{
		"Backlot agent setup",
		"Backlot root: /state",
		"Codex CLI",
		"Claude Code",
		"Status:",
		"backlot agents setup --tool codex --apply",
		"backlot agents setup --tool claude --apply",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("agents setup output missing %q:\n%s", want, text)
		}
	}
}

func TestAgentsSetupToolPreviewPrintsExactInstructions(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"agents", "setup", "--root", "/state", "--tool", "codex"}, &out, &errOut); code != 0 {
		t.Fatalf("agents setup --tool exit code = %d, stderr = %s", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{
		"Codex CLI",
		"codex --cd",
		"--add-dir /state",
		"~/.codex/config.toml",
		`default_permissions = "workspace-backlot"`,
		`"/state" = true`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("codex setup output missing %q:\n%s", want, text)
		}
	}
}

func TestAgentsSetupApplyRequiresTool(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"agents", "setup", "--root", "/state", "--apply"}, &out, &errOut); code == 0 {
		t.Fatalf("agents setup --apply exited 0, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "--apply requires --tool") {
		t.Fatalf("stderr = %q, want --tool requirement", errOut.String())
	}
}

func TestDoctorPrintsAgentSetupHintWithoutFailing(t *testing.T) {
	state, public := createAttachedRepoForAgentTests(t)

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"doctor", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("doctor exit code = %d, stderr = %s", code, errOut.String())
		}
		text := out.String()
		for _, want := range []string{
			"Backlot root: " + state,
			"Agent setup: see https://github.com/massivemoose/backlot/blob/main/docs/agents.md or run backlot agents setup",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("doctor output missing %q:\n%s", want, text)
			}
		}
	})
}

func createAttachedRepoForAgentTests(t *testing.T) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	state := tmp + "/state"
	public := tmp + "/public"
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/backlot.git")
	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})
	return state, public
}
