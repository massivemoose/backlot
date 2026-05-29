package commands

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHelpExitsSuccessfully(t *testing.T) {
	tests := [][]string{
		{"--help"},
		{"-h"},
		{"init", "--help"},
	}

	for _, args := range tests {
		var out, errOut bytes.Buffer
		if code := Run(args, &out, &errOut); code != 0 {
			t.Fatalf("Run(%v) exit code = %d, stderr = %s", args, code, errOut.String())
		}
	}
}

func TestVersionOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("version exit code = %d, stderr = %s", code, errOut.String())
	}
	want := "backlot dev\ncommit: unknown\ndate: unknown\n"
	if out.String() != want {
		t.Fatalf("version output = %q, want %q", out.String(), want)
	}

	out.Reset()
	errOut.Reset()
	build := BuildInfo{Version: "v0.1.0", Commit: "testsha", Date: "2026-05-28T00:00:00Z"}
	if code := RunWithBuildInfo([]string{"version"}, &out, &errOut, build); code != 0 {
		t.Fatalf("version with build info exit code = %d, stderr = %s", code, errOut.String())
	}
	want = "backlot v0.1.0\ncommit: testsha\ndate: 2026-05-28T00:00:00Z\n"
	if out.String() != want {
		t.Fatalf("version with build info output = %q, want %q", out.String(), want)
	}
}

func TestInitCreatesGitRepoAndPreservesExistingOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := filepath.Join(t.TempDir(), "state")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", root, "--remote", "git@example.com:one/state.git"}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("state root is not a git repo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "README.md")); err != nil {
		t.Fatalf("README.md missing: %v", err)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"init", "--root", root, "--remote", "git@example.com:two/state.git"}, &out, &errOut); code != 0 {
		t.Fatalf("second init exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "origin already exists") {
		t.Fatalf("second init output = %q, want origin already exists message", out.String())
	}
	if !strings.Contains(out.String(), "git@example.com:one/state.git") {
		t.Fatalf("second init output = %q, want existing origin URL", out.String())
	}

	got := runGitOutput(t, root, "remote", "get-url", "origin")
	if got != "git@example.com:one/state.git" {
		t.Fatalf("origin = %q, want original remote", got)
	}
}

func TestInitRemoteSetsOriginOnExistingRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := filepath.Join(t.TempDir(), "state")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", root}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	remote := "git@example.com:you/backlot-archive.git"
	if code := Run([]string{"init", "--root", root, "--remote", remote}, &out, &errOut); code != 0 {
		t.Fatalf("init remote exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := runGitOutput(t, root, "remote", "get-url", "origin"); got != remote {
		t.Fatalf("origin = %q, want %q", got, remote)
	}
}

func TestAttachCreatesStateSymlinkAndExclude(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	mustRunGit(t, tmp, "init", state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	stateDir := filepath.Join(state, "github.com", "massivemoose", "ovek")
	for _, path := range []string{
		filepath.Join(stateDir, "notes.md"),
		filepath.Join(stateDir, "llm"),
		filepath.Join(stateDir, "scratch"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected attach-created path %s: %v", path, err)
		}
	}

	linkTarget, err := os.Readlink(filepath.Join(public, ".backlot"))
	if err != nil {
		t.Fatalf(".backlot is not a symlink: %v", err)
	}
	if linkTarget != stateDir {
		t.Fatalf(".backlot target = %q, want %q", linkTarget, stateDir)
	}

	excludeData, err := os.ReadFile(filepath.Join(public, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(excludeData), ".backlot/") != 1 {
		t.Fatalf("exclude contents = %q, want one .backlot/ entry", string(excludeData))
	}
	if countExcludeLine(string(excludeData), ".backlot") != 1 {
		t.Fatalf("exclude contents = %q, want one .backlot entry", string(excludeData))
	}
	if got := runGitOutput(t, public, "status", "--short"); got != "" {
		t.Fatalf("public repo status = %q, want attached .backlot symlink ignored", got)
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"status", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("status exit code = %d, stderr = %s", code, errOut.String())
		}
		text := out.String()
		for _, want := range []string{
			"Project key:   github.com/massivemoose/ovek",
			"Excluded:      yes",
			"State repo:    dirty",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("status output missing %q:\n%s", want, text)
			}
		}
	})
}

func TestAttachRequiresInitializedBacklotRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code == 0 {
			t.Fatalf("attach succeeded before init, stdout = %s", out.String())
		}
		if !strings.Contains(errOut.String(), "run backlot init first") {
			t.Fatalf("attach stderr = %q, want init-first message", errOut.String())
		}
	})
}

func TestAttachRejectsBacklotRootInsideAnotherGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	state := filepath.Join(parent, "state")
	public := filepath.Join(tmp, "public")
	mustRunGit(t, tmp, "init", parent)
	if err := os.Mkdir(state, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code == 0 {
			t.Fatalf("attach accepted parent repo subdirectory as Backlot root, stdout = %s", out.String())
		}
		if !strings.Contains(errOut.String(), "run backlot init first") {
			t.Fatalf("attach stderr = %q, want init-first message", errOut.String())
		}
	})
}

func TestProtectCreatesButDoesNotOverwriteHook(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := filepath.Join(t.TempDir(), "repo")
	mustRunGit(t, filepath.Dir(repo), "init", repo)

	withChdir(t, repo, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"protect"}, &out, &errOut); code != 0 {
			t.Fatalf("protect exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	hook := filepath.Join(repo, ".git", "hooks", "pre-commit")
	data, err := os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "refusing to commit .backlot") {
		t.Fatalf("hook contents missing guard: %q", string(data))
	}

	if err := os.WriteFile(hook, []byte("# custom hook\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	withChdir(t, repo, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"protect"}, &out, &errOut); code != 0 {
			t.Fatalf("second protect exit code = %d, stderr = %s", code, errOut.String())
		}
		if !strings.Contains(out.String(), "already exists") {
			t.Fatalf("second protect output = %q, want already exists message", out.String())
		}
	})
	data, err = os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# custom hook\n" {
		t.Fatalf("protect overwrote existing hook: %q", string(data))
	}
}

func TestAttachAndProtectHandleGitFileWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	mainRepo := filepath.Join(tmp, "main")
	worktree := filepath.Join(tmp, "worktree")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	mustRunGit(t, tmp, "init", mainRepo)
	configureGitIdentity(t, mainRepo)
	if err := os.WriteFile(filepath.Join(mainRepo, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, mainRepo, "add", "README.md")
	mustRunGit(t, mainRepo, "commit", "-m", "Initial")
	mustRunGit(t, mainRepo, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	mustRunGit(t, mainRepo, "worktree", "add", "-b", "feature", worktree)

	withChdir(t, worktree, func() {
		out.Reset()
		errOut.Reset()
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
		excludePath := runGitOutput(t, worktree, "rev-parse", "--git-path", "info/exclude")
		excludeData, err := os.ReadFile(excludePath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(excludeData), ".backlot/") {
			t.Fatalf("worktree exclude = %q, want .backlot/", string(excludeData))
		}
		if _, err := os.Stat(filepath.Join(worktree, ".git", "info", "exclude")); err == nil {
			t.Fatal("attach created exclude below .git file path")
		}

		out.Reset()
		errOut.Reset()
		if code := Run([]string{"protect"}, &out, &errOut); code != 0 {
			t.Fatalf("protect exit code = %d, stderr = %s", code, errOut.String())
		}
		hookPath := runGitOutput(t, worktree, "rev-parse", "--git-path", "hooks/pre-commit")
		hookData, err := os.ReadFile(hookPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(hookData), "refusing to commit .backlot") {
			t.Fatalf("worktree hook missing guard: %q", string(hookData))
		}
	})
}

func TestSyncCleanNoRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	state := filepath.Join(t.TempDir(), "state")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	mustRunGit(t, state, "add", "README.md")
	cmd := exec.Command("git", "-c", "core.fsmonitor=false", "-C", state, "-c", "user.name=Backlot Test", "-c", "user.email=backlot@example.invalid", "commit", "-m", "Initial state")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit initial state failed: %v\n%s", err, string(output))
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("sync exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Backlot state is clean.") {
		t.Fatalf("sync output = %q, want clean message", out.String())
	}
}

func TestSyncDirtyNoRemoteCommitsLocallyOnlyInBacklotRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, state)
	mustRunGit(t, tmp, "init", public)
	configureGitIdentity(t, public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	if err := os.WriteFile(filepath.Join(public, "public.txt"), []byte("do not stage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "notes.md"), []byte("private\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	withChdir(t, public, func() {
		out.Reset()
		errOut.Reset()
		if code := Run([]string{"sync", "--root", state, "-m", "Update private notes"}, &out, &errOut); code != 0 {
			t.Fatalf("sync exit code = %d, stderr = %s", code, errOut.String())
		}
		if !strings.Contains(out.String(), "No origin remote configured; committed locally and skipped push.") {
			t.Fatalf("sync output = %q, want no-origin skip message", out.String())
		}
	})

	if got := runGitOutput(t, state, "status", "--short"); got != "" {
		t.Fatalf("state repo status = %q, want clean", got)
	}
	if got := runGitOutput(t, state, "log", "-1", "--pretty=%s"); got != "Update private notes" {
		t.Fatalf("state repo last commit = %q, want sync message", got)
	}
	if got := runGitOutput(t, public, "status", "--short"); got != "?? public.txt" {
		t.Fatalf("public repo status = %q, want untracked file untouched", got)
	}
}

func TestSyncDirtyWithOriginSetsUpstreamOnFirstPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	remote := filepath.Join(tmp, "remote.git")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, state)
	mustRunGit(t, tmp, "init", "--bare", remote)
	mustRunGit(t, state, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(state, "notes.md"), []byte("private\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", state, "-m", "Initial Backlot archive"}, &out, &errOut); code != 0 {
		t.Fatalf("sync exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Backlot state synced.") {
		t.Fatalf("sync output = %q, want synced message", out.String())
	}
	branch := runGitOutput(t, state, "branch", "--show-current")
	if got := runGitOutput(t, state, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); got != "origin/"+branch {
		t.Fatalf("upstream = %q, want origin/%s", got, branch)
	}
	if got := runGitOutput(t, state, "status", "--short"); got != "" {
		t.Fatalf("state repo status = %q, want clean", got)
	}
}

func TestSyncCleanWithOriginSetsUpstreamOnFirstPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	remote := filepath.Join(tmp, "remote.git")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, state)
	mustRunGit(t, tmp, "init", "--bare", remote)
	mustRunGit(t, state, "remote", "add", "origin", remote)
	mustRunGit(t, state, "add", "README.md")
	mustRunGit(t, state, "commit", "-m", "Initial Backlot archive")

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("sync exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Backlot state synced.") {
		t.Fatalf("sync output = %q, want synced message", out.String())
	}
	branch := runGitOutput(t, state, "branch", "--show-current")
	if got := runGitOutput(t, state, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); got != "origin/"+branch {
		t.Fatalf("upstream = %q, want origin/%s", got, branch)
	}
}

func TestSyncDirtyWithExistingUpstreamPullsAndPushes(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	remote := filepath.Join(tmp, "remote.git")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, state)
	mustRunGit(t, tmp, "init", "--bare", remote)
	mustRunGit(t, state, "remote", "add", "origin", remote)
	mustRunGit(t, state, "add", "README.md")
	mustRunGit(t, state, "commit", "-m", "Initial Backlot archive")
	branch := runGitOutput(t, state, "branch", "--show-current")
	mustRunGit(t, state, "push", "-u", "origin", branch)
	if err := os.WriteFile(filepath.Join(state, "notes.md"), []byte("private\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", state, "-m", "Update private notes"}, &out, &errOut); code != 0 {
		t.Fatalf("sync exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Backlot state synced.") {
		t.Fatalf("sync output = %q, want synced message", out.String())
	}
	if got := runGitOutput(t, remote, "log", "-1", "--pretty=%s"); got != "Update private notes" {
		t.Fatalf("remote last commit = %q, want sync commit", got)
	}
}

func TestSyncRemoteFailureIncludesOperationContext(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	missingRemote := filepath.Join(tmp, "missing-remote")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, state)
	mustRunGit(t, state, "remote", "add", "origin", missingRemote)
	if err := os.WriteFile(filepath.Join(state, "notes.md"), []byte("private\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", state, "-m", "Update private notes"}, &out, &errOut); code == 0 {
		t.Fatalf("sync succeeded with missing remote, stdout = %s", out.String())
	}
	stderr := errOut.String()
	if !strings.Contains(stderr, "first push failed while syncing Backlot root") {
		t.Fatalf("sync stderr missing operation context:\n%s", stderr)
	}
	if !strings.Contains(stderr, "git -c core.fsmonitor=false -C") {
		t.Fatalf("sync stderr did not preserve underlying git error:\n%s", stderr)
	}
}

func TestStatusDetectsWrongSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	wrong := filepath.Join(tmp, "wrong")
	mustRunGit(t, tmp, "init", state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	if err := os.Mkdir(wrong, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(wrong, filepath.Join(public, ".backlot")); err != nil {
		t.Fatal(err)
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"status", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("status exit code = %d, stderr = %s", code, errOut.String())
		}
		if !strings.Contains(out.String(), "wrong target") {
			t.Fatalf("status output = %q, want wrong target", out.String())
		}
	})
}

func TestStatusDetectsMissingAndBrokenSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	mustRunGit(t, tmp, "init", state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"status", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("status exit code = %d, stderr = %s", code, errOut.String())
		}
		if !strings.Contains(out.String(), "Link:          missing") {
			t.Fatalf("status output = %q, want missing link", out.String())
		}
	})

	if err := os.Symlink(filepath.Join(tmp, "does-not-exist"), filepath.Join(public, ".backlot")); err != nil {
		t.Fatal(err)
	}
	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"status", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("status exit code = %d, stderr = %s", code, errOut.String())
		}
		if !strings.Contains(out.String(), "broken") {
			t.Fatalf("status output = %q, want broken link", out.String())
		}
	})
}

func TestDoctorReportsHealthyAttachedRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	withChdir(t, public, func() {
		out.Reset()
		errOut.Reset()
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
		out.Reset()
		errOut.Reset()
		if code := Run([]string{"doctor", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("doctor exit code = %d, stderr = %s", code, errOut.String())
		}
		text := out.String()
		for _, want := range []string{
			"✓ git found",
			"✓ inside Git repo",
			"✓ Backlot root exists",
			"✓ Backlot root is a Git repo",
			"✓ .backlot symlink exists",
			"✓ .backlot points to expected target",
			"✓ .git/info/exclude ignores .backlot",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("doctor output missing %q:\n%s", want, text)
			}
		}
	})
}

func TestAttachDoesNotWriteGitignore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	withChdir(t, public, func() {
		out.Reset()
		errOut.Reset()
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	if _, err := os.Stat(filepath.Join(public, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf(".gitignore exists after attach: %v", err)
	}
}

func withChdir(t *testing.T, dir string, fn func()) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.fsmonitor=false", "-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git -C %s %s failed: %v\n%s", dir, strings.Join(args, " "), err, string(output))
	}
}

func configureGitIdentity(t *testing.T, dir string) {
	t.Helper()
	mustRunGit(t, dir, "config", "user.name", "Backlot Test")
	mustRunGit(t, dir, "config", "user.email", "backlot@example.invalid")
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.fsmonitor=false", "-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git -C %s %s failed: %v\n%s", dir, strings.Join(args, " "), err, string(output))
	}
	return strings.TrimSpace(string(output))
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
