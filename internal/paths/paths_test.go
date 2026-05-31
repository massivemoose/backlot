package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/massivemoose/backlot/internal/gitutil"
)

func TestBacklotRootResolution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BACKLOT_ROOT", "")

	explicit := filepath.Join(t.TempDir(), "explicit")
	got, err := BacklotRoot(explicit)
	if err != nil {
		t.Fatalf("BacklotRoot explicit returned error: %v", err)
	}
	if got != explicit {
		t.Fatalf("BacklotRoot explicit = %q, want %q", got, explicit)
	}

	envRoot := filepath.Join(t.TempDir(), "env")
	t.Setenv("BACKLOT_ROOT", envRoot)
	got, err = BacklotRoot("")
	if err != nil {
		t.Fatalf("BacklotRoot env returned error: %v", err)
	}
	if got != envRoot {
		t.Fatalf("BacklotRoot env = %q, want %q", got, envRoot)
	}

	t.Setenv("BACKLOT_ROOT", "")
	got, err = BacklotRoot("~/state")
	if err != nil {
		t.Fatalf("BacklotRoot tilde returned error: %v", err)
	}
	if want := filepath.Join(home, "state"); got != want {
		t.Fatalf("BacklotRoot tilde = %q, want %q", got, want)
	}

	got, err = BacklotRoot("")
	if err != nil {
		t.Fatalf("BacklotRoot default returned error: %v", err)
	}
	if want := filepath.Join(home, ".backlot"); got != want {
		t.Fatalf("BacklotRoot default = %q, want %q", got, want)
	}
}

func TestEnsureExcludeIsIdempotentAndPreservesContent(t *testing.T) {
	repo := t.TempDir()
	mustRunGit(t, repo, "init")
	excludePath := runGitPath(t, repo, "info/exclude")
	if err := os.WriteFile(excludePath, []byte("# local excludes"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureExclude(repo, ".backlot"); err != nil {
		t.Fatalf("EnsureExclude first call returned error: %v", err)
	}
	if err := EnsureExclude(repo, ".backlot"); err != nil {
		t.Fatalf("EnsureExclude second call returned error: %v", err)
	}

	data, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "# local excludes\n") {
		t.Fatalf("exclude content did not preserve existing line with newline: %q", text)
	}
	if count := strings.Count(text, ".backlot/"); count != 1 {
		t.Fatalf("exclude contains .backlot/ %d times, want 1: %q", count, text)
	}
	if count := countExcludeLine(text, ".backlot"); count != 1 {
		t.Fatalf("exclude contains .backlot %d times, want 1: %q", count, text)
	}

	ok, err := ExcludeContains(repo, ".backlot")
	if err != nil {
		t.Fatalf("ExcludeContains returned error: %v", err)
	}
	if !ok {
		t.Fatal("ExcludeContains returned false after EnsureExclude")
	}
}

func countExcludeLine(text, want string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == want {
			count++
		}
	}
	return count
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := gitutil.RunGit(dir, args...); err != nil {
		t.Fatal(err)
	}
}

func runGitPath(t *testing.T, dir string, path string) string {
	t.Helper()
	got, err := gitutil.GitPath(dir, path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestEnsureManagedSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "state")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".backlot")

	if err := EnsureManagedSymlink(link, target); err != nil {
		t.Fatalf("EnsureManagedSymlink first call returned error: %v", err)
	}
	if err := EnsureManagedSymlink(link, target); err != nil {
		t.Fatalf("EnsureManagedSymlink idempotent call returned error: %v", err)
	}
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("symlink target = %q, want %q", got, target)
	}

	wrongTarget := filepath.Join(dir, "other")
	if err := os.Mkdir(wrongTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	wrongLink := filepath.Join(dir, "wrong")
	if err := os.Symlink(wrongTarget, wrongLink); err != nil {
		t.Fatal(err)
	}
	err = EnsureManagedSymlink(wrongLink, target)
	if err == nil || !strings.Contains(err.Error(), "not managed by Backlot") {
		t.Fatalf("EnsureManagedSymlink wrong target error = %v, want managed error", err)
	}
}
