package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseScriptRejectsInvalidVersion(t *testing.T) {
	result := runReleaseScript(t, ".", os.Environ(), "0.1.2")
	if result.err == nil {
		t.Fatal("release script accepted version without v prefix")
	}
	if !strings.Contains(result.output, "Usage: scripts/release vMAJOR.MINOR.PATCH") {
		t.Fatalf("release output = %q, want usage", result.output)
	}
}

func TestReleaseScriptRequiresGoreleaser(t *testing.T) {
	repo := prepareReleaseRepo(t)
	path := fakeToolPath(t, false)
	result := runReleaseScript(t, repo.work, append(os.Environ(), "PATH="+path), "v9.9.9")
	if result.err == nil {
		t.Fatal("release script succeeded without goreleaser on PATH")
	}
	if !strings.Contains(result.output, "missing required command: goreleaser") {
		t.Fatalf("release output = %q, want missing goreleaser", result.output)
	}
}

func TestReleaseScriptRejectsDirtyWorktree(t *testing.T) {
	repo := prepareReleaseRepo(t)
	if err := os.WriteFile(filepath.Join(repo.work, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := fakeToolPath(t, true)
	result := runReleaseScript(t, repo.work, append(os.Environ(), "PATH="+path), "v9.9.9")
	if result.err == nil {
		t.Fatal("release script succeeded with dirty worktree")
	}
	if !strings.Contains(result.output, "working tree is not clean") {
		t.Fatalf("release output = %q, want dirty worktree error", result.output)
	}
}

func TestReleaseScriptRejectsExistingLocalTag(t *testing.T) {
	repo := prepareReleaseRepo(t)
	mustRun(t, repo.work, "git", "tag", "v9.9.9")
	path := fakeToolPath(t, true)
	result := runReleaseScript(t, repo.work, append(os.Environ(), "PATH="+path), "v9.9.9")
	if result.err == nil {
		t.Fatal("release script succeeded with existing local tag")
	}
	if !strings.Contains(result.output, "tag v9.9.9 already exists locally") {
		t.Fatalf("release output = %q, want local tag error", result.output)
	}
}

func TestReleaseScriptRejectsHeadNotAtOriginMain(t *testing.T) {
	repo := prepareReleaseRepo(t)
	if err := os.WriteFile(filepath.Join(repo.work, "next.txt"), []byte("next\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, repo.work, "git", "add", "next.txt")
	mustRun(t, repo.work, "git", "commit", "-m", "Unreleased")
	path := fakeToolPath(t, true)
	result := runReleaseScript(t, repo.work, append(os.Environ(), "PATH="+path), "v9.9.9")
	if result.err == nil {
		t.Fatal("release script succeeded when HEAD was not origin/main")
	}
	if !strings.Contains(result.output, "current HEAD must match origin/main") {
		t.Fatalf("release output = %q, want origin/main error", result.output)
	}
}

func TestReleaseScriptTagsAndPushesWithEmptyTapToken(t *testing.T) {
	repo := prepareReleaseRepo(t)
	path := fakePublishingToolPath(t)
	result := runReleaseScript(t, repo.work, append(os.Environ(), "PATH="+path), "v9.9.9")
	if result.err != nil {
		t.Fatalf("release script failed: %v\n%s", result.err, result.output)
	}
	if !strings.Contains(result.output, "Pushed release tag v9.9.9.") {
		t.Fatalf("release output = %q, want pushed tag message", result.output)
	}
	mustRun(t, repo.work, "git", "fetch", "--tags", "origin")
	if got := runOutput(t, repo.work, "git", "rev-parse", "refs/tags/v9.9.9"); got == "" {
		t.Fatal("remote release tag was not pushed")
	}
}

type scriptResult struct {
	output string
	err    error
}

func runReleaseScript(t *testing.T, dir string, env []string, args ...string) scriptResult {
	t.Helper()
	scriptsDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(scriptsDir, "release"), args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return scriptResult{output: string(output), err: err}
}

type releaseRepo struct {
	work string
}

func prepareReleaseRepo(t *testing.T) releaseRepo {
	t.Helper()
	tmp := t.TempDir()
	remote := filepath.Join(tmp, "remote.git")
	work := filepath.Join(tmp, "work")
	mustRun(t, tmp, "git", "init", "--bare", remote)
	mustRun(t, tmp, "git", "init", "-b", "main", work)
	mustRun(t, work, "git", "config", "user.name", "Backlot Test")
	mustRun(t, work, "git", "config", "user.email", "backlot@example.invalid")
	if err := os.WriteFile(filepath.Join(work, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, work, "git", "add", "README.md")
	mustRun(t, work, "git", "commit", "-m", "Initial")
	mustRun(t, work, "git", "remote", "add", "origin", remote)
	mustRun(t, work, "git", "push", "-u", "origin", "main")
	return releaseRepo{work: work}
}

func fakeToolPath(t *testing.T, includeGoreleaser bool) string {
	t.Helper()
	bin := t.TempDir()
	linkTool(t, bin, "git")
	linkTool(t, bin, "go")
	if includeGoreleaser {
		writeExecutable(t, filepath.Join(bin, "goreleaser"), "#!/bin/sh\nexit 0\n")
	}
	return bin
}

func fakePublishingToolPath(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	linkTool(t, bin, "git")
	writeExecutable(t, filepath.Join(bin, "go"), "#!/bin/sh\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "goreleaser"), "#!/bin/sh\nif [ \"${TAP_GITHUB_TOKEN+x}\" != x ]; then\n  echo TAP_GITHUB_TOKEN not set >&2\n  exit 42\nfi\nexit 0\n")
	return bin
}

func linkTool(t *testing.T, bin, name string) {
	t.Helper()
	tool, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not installed", name)
	}
	target := filepath.Join(bin, name)
	if runtime.GOOS == "windows" {
		t.Skip("release script is macOS/Linux-oriented")
	}
	if err := os.Symlink(tool, target); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, string(output))
	}
}

func runOutput(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, string(output))
	}
	return strings.TrimSpace(string(output))
}
