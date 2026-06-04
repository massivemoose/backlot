package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestNormalizeOriginRejectsUnsafePathSegments(t *testing.T) {
	tests := []string{
		"git@github.com:massivemoose/../private.git",
		"https://github.com/massivemoose//backlot.git",
		"https://github.com/massivemoose/backlot%0Astate.git",
		"git@github.com:massivemoose/back\\lot.git",
		"git@github.com:massivemoose/back:lot.git",
	}

	for _, remote := range tests {
		if _, err := NormalizeOrigin(remote); err == nil {
			t.Fatalf("NormalizeOrigin(%q) accepted unsafe remote", remote)
		}
	}
}

func TestRunGitIgnoresRepoRoutingEnvironment(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	repoA := filepath.Join(tmp, "repo-a")
	repoB := filepath.Join(tmp, "repo-b")
	mustRunGitCommand(t, tmp, "init", repoA)
	mustRunGitCommand(t, tmp, "init", repoB)
	t.Setenv("GIT_DIR", filepath.Join(repoB, ".git"))
	t.Setenv("GIT_WORK_TREE", repoB)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(tmp, "foreign-index"))

	got, err := RunGit(repoA, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("RunGit returned error with repo-routing env set: %v", err)
	}
	want, err := filepath.EvalSymlinks(repoA)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("RunGit used routed repo = %q, want %q", got, repoA)
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

func TestHasStagedChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := t.TempDir()
	mustRunGitCommand(t, repo, "init")

	changed, err := HasStagedChanges(repo)
	if err != nil {
		t.Fatalf("HasStagedChanges clean returned error: %v", err)
	}
	if changed {
		t.Fatal("HasStagedChanges clean = true, want false")
	}

	if err := os.WriteFile(filepath.Join(repo, "notes.md"), []byte("private\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGitCommand(t, repo, "add", "notes.md")

	changed, err = HasStagedChanges(repo)
	if err != nil {
		t.Fatalf("HasStagedChanges staged returned error: %v", err)
	}
	if !changed {
		t.Fatal("HasStagedChanges staged = false, want true")
	}
}

func mustRunGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
