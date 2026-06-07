package commands

import (
	"bytes"
	"errors"
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
