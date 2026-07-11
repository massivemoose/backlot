package commands

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncQuietSuppressesNormalOutput(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	state := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, state)
	configureGitIdentity(t, state)
	var out, errOut bytes.Buffer
	if code := Run([]string{"sync", "--root", state, "--quiet"}, &out, &errOut); code != 0 {
		t.Fatalf("sync --quiet exit code = %d, stderr = %s", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("sync --quiet stdout = %q, want empty", out.String())
	}
}

func TestSyncQuietPreservesErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"sync", "--root", filepath.Join(t.TempDir(), "missing"), "--quiet"}, &out, &errOut); code == 0 {
		t.Fatalf("sync --quiet missing root exited 0, stdout = %s", out.String())
	}
	if out.Len() != 0 {
		t.Fatalf("sync --quiet error stdout = %q, want empty", out.String())
	}
	if !strings.Contains(errOut.String(), "not initialized") {
		t.Fatalf("sync --quiet stderr = %q, want initialization error", errOut.String())
	}
}

func TestAcquireSyncLockRejectsConcurrentHolder(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	state := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, state)

	release, err := acquireSyncLock(state)
	if err != nil {
		t.Fatalf("acquireSyncLock first returned error: %v", err)
	}
	defer release()

	if _, err := acquireSyncLock(state); !errors.Is(err, errSyncBusy) {
		t.Fatalf("acquireSyncLock second error = %v, want errSyncBusy", err)
	}
}

func TestDetectSyncStatePreservesNewlineConflictPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	name := "line\nbreak.md"
	mustRunGit(t, repo, "init")
	configureGitIdentity(t, repo)
	if err := os.WriteFile(filepath.Join(repo, name), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, repo, "add", "-A")
	mustRunGit(t, repo, "commit", "-m", "base")
	mustRunGit(t, repo, "switch", "-c", "other")
	if err := os.WriteFile(filepath.Join(repo, name), []byte("other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, repo, "commit", "-am", "other")
	mustRunGit(t, repo, "switch", "-")
	if err := os.WriteFile(filepath.Join(repo, name), []byte("current\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, repo, "commit", "-am", "current")
	cmd := exec.Command("git", "-c", "core.fsmonitor=false", "-C", repo, "merge", "other")
	if err := cmd.Run(); err == nil {
		t.Fatal("merge unexpectedly succeeded")
	}

	state, err := detectSyncState(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Conflicts) != 1 || state.Conflicts[0] != name {
		t.Fatalf("conflicts = %q, want [%q]", state.Conflicts, name)
	}
}
