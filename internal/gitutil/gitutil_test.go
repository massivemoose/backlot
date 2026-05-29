package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNormalizeOrigin(t *testing.T) {
	tests := map[string]string{
		"git@github.com:massivemoose/ovek.git":       "github.com/massivemoose/ovek",
		"https://github.com/massivemoose/ovek.git":   "github.com/massivemoose/ovek",
		"https://github.com/massivemoose/ovek":       "github.com/massivemoose/ovek",
		"ssh://git@github.com/massivemoose/ovek.git": "github.com/massivemoose/ovek",
		"git@GitHub.com:MassiveMoose/Ovek.git":       "github.com/MassiveMoose/Ovek",
	}

	for remote, want := range tests {
		got, err := NormalizeOrigin(remote)
		if err != nil {
			t.Fatalf("NormalizeOrigin(%q) returned error: %v", remote, err)
		}
		if got != want {
			t.Fatalf("NormalizeOrigin(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestNormalizeOriginRejectsUnsupportedRemote(t *testing.T) {
	if _, err := NormalizeOrigin("/tmp/local/repo"); err == nil {
		t.Fatal("NormalizeOrigin accepted a local path remote")
	}
}

func TestIsGitRepoRootRejectsSubdirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := t.TempDir()
	subdir := filepath.Join(repo, "nested")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repo, "init")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, string(output))
	}

	if !IsGitRepoRoot(repo) {
		t.Fatal("IsGitRepoRoot rejected actual repo root")
	}
	if IsGitRepoRoot(subdir) {
		t.Fatal("IsGitRepoRoot accepted subdirectory inside a repo")
	}
}
