package commands

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/massivemoose/backlot/internal/gitutil"
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

func TestHelpDoesNotAdvertiseCustomLinkName(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("help exit code = %d, stderr = %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "--link-name") {
		t.Fatalf("help output advertises custom link names:\n%s", out.String())
	}
}

func TestAttachRejectsCustomLinkName(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state, "--link-name", ".private"}, &out, &errOut); code == 0 {
			t.Fatalf("attach accepted custom link name, stdout = %s", out.String())
		}
		if !strings.Contains(errOut.String(), "custom link names are not supported") {
			t.Fatalf("attach stderr = %q, want custom-link rejection", errOut.String())
		}
	})
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
	if _, err := os.Stat(filepath.Join(root, ".backlot-root")); err != nil {
		t.Fatalf("archive marker missing: %v", err)
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

func TestInitRejectsExistingNonBacklotGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := filepath.Join(t.TempDir(), "public")
	mustRunGit(t, filepath.Dir(root), "init", root)
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", root}, &out, &errOut); code == 0 {
		t.Fatalf("init accepted existing non-Backlot Git repo, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "is not a Backlot archive") {
		t.Fatalf("init stderr = %q, want non-Backlot archive error", errOut.String())
	}
}

func TestInitRejectsRootInsideCurrentProjectRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	project := filepath.Join(t.TempDir(), "project")
	mustRunGit(t, filepath.Dir(project), "init", project)
	nestedRoot := filepath.Join(project, ".state")
	withChdir(t, project, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"init", "--root", nestedRoot}, &out, &errOut); code == 0 {
			t.Fatalf("init accepted root inside project repo, stdout = %s", out.String())
		}
		if !strings.Contains(errOut.String(), "inside current Git repo") {
			t.Fatalf("init stderr = %q, want inside-project error", errOut.String())
		}
	})
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

func TestCloneCreatesBacklotRootFromRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	root := filepath.Join(tmp, "state")
	var out, errOut bytes.Buffer
	if code := Run([]string{"clone", remote, "--root", root}, &out, &errOut); code != 0 {
		t.Fatalf("clone exit code = %d, stderr = %s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("cloned root is not a Git repo: %v", err)
	}
	if got := runGitOutput(t, root, "remote", "get-url", "origin"); got != remote {
		t.Fatalf("origin = %q, want %q", got, remote)
	}
	if _, err := os.Stat(filepath.Join(root, "README.md")); err != nil {
		t.Fatalf("cloned archive missing README.md: %v", err)
	}
	if !strings.Contains(out.String(), "Cloned Backlot archive") {
		t.Fatalf("clone output = %q, want success message", out.String())
	}
}

func TestCloneAllowsExistingEmptyRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	root := filepath.Join(tmp, "empty")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := Run([]string{"clone", remote, "--root", root}, &out, &errOut); code != 0 {
		t.Fatalf("clone exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := runGitOutput(t, root, "remote", "get-url", "origin"); got != remote {
		t.Fatalf("origin = %q, want %q", got, remote)
	}
}

func TestCloneRejectsExistingNonEmptyRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	root := filepath.Join(tmp, "state")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(root, "local.txt")
	if err := os.WriteFile(sentinel, []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := Run([]string{"clone", remote, "--root", root}, &out, &errOut); code == 0 {
		t.Fatalf("clone succeeded with non-empty root, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "already exists and is not empty") {
		t.Fatalf("clone stderr = %q, want non-empty root error", errOut.String())
	}
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "local\n" {
		t.Fatalf("sentinel changed to %q", string(data))
	}
}

func TestCloneRejectsInitCreatedRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	root := filepath.Join(tmp, "state")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", root}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"clone", remote, "--root", root}, &out, &errOut); code == 0 {
		t.Fatalf("clone succeeded over init-created root, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "Move it aside or choose another root with --root") {
		t.Fatalf("clone stderr = %q, want move-aside guidance", errOut.String())
	}
}

func TestCloneHybridArgs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing url",
			args:    []string{"clone"},
			wantErr: "error: missing archive URL",
		},
		{
			name:    "too many args",
			args:    []string{"clone", "one", "two"},
			wantErr: "error: too many arguments",
		},
		{
			name:    "remote flag is unsupported",
			args:    []string{"clone", "--remote", "flag"},
			wantErr: "flag provided but not defined: -remote",
		},
		{
			name: "positional then root success",
			args: []string{"clone", remote, "--root", filepath.Join(tmp, "pos")},
		},
		{
			name: "root then positional success",
			args: []string{"clone", "--root", filepath.Join(tmp, "root-first"), remote},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run(tt.args, &out, &errOut)
			if tt.wantErr != "" {
				if code == 0 {
					t.Errorf("expected error code, got 0")
				}
				if !strings.Contains(errOut.String(), tt.wantErr) {
					t.Errorf("expected error %q, got %q", tt.wantErr, errOut.String())
				}
			} else {
				if code != 0 {
					t.Errorf("expected code 0, got %d, stderr: %s", code, errOut.String())
				}
			}
		})
	}
}

func TestCloneThenAttachPublicRepo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	root := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	var out, errOut bytes.Buffer
	if code := Run([]string{"clone", remote, "--root", root}, &out, &errOut); code != 0 {
		t.Fatalf("clone exit code = %d, stderr = %s", code, errOut.String())
	}
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	withChdir(t, public, func() {
		out.Reset()
		errOut.Reset()
		if code := Run([]string{"attach", "--root", root}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	stateDir := filepath.Join(root, "github.com", "massivemoose", "ovek")
	linkTarget, err := os.Readlink(filepath.Join(public, ".backlot"))
	if err != nil {
		t.Fatalf(".backlot is not a symlink: %v", err)
	}
	if linkTarget != stateDir {
		t.Fatalf(".backlot target = %q, want %q", linkTarget, stateDir)
	}
	if got := runGitOutput(t, public, "status", "--short"); got != "" {
		t.Fatalf("public repo status = %q, want clean", got)
	}
}

func TestAttachCreatesStateSymlinkAndExclude(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	var out, errOut bytes.Buffer
	withChdir(t, public, func() {
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})
	if !strings.Contains(out.String(), "Starter:     built-in defaults") {
		t.Fatalf("attach output = %q, want built-in starter message", out.String())
	}

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

func TestAttachCopiesCustomStarterTemplate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	starter := filepath.Join(state, ".starter")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	if err := os.MkdirAll(filepath.Join(starter, "llm", "prompts"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(starter, "roadmap.md"), []byte("# Roadmap\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(starter, ".secret-note"), []byte("hidden\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(starter, "llm", "prompts", "review.md"), []byte("review\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	withChdir(t, public, func() {
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})
	if !strings.Contains(out.String(), "Starter:     "+starter) {
		t.Fatalf("attach output = %q, want custom starter path %s", out.String(), starter)
	}

	stateDir := filepath.Join(state, "github.com", "massivemoose", "ovek")
	for path, want := range map[string]string{
		filepath.Join(stateDir, "roadmap.md"):                  "# Roadmap\n",
		filepath.Join(stateDir, ".secret-note"):                "hidden\n",
		filepath.Join(stateDir, "llm", "prompts", "review.md"): "review\n",
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected copied starter file %s: %v", path, err)
		}
		if string(got) != want {
			t.Fatalf("copied starter file %s = %q, want %q", path, string(got), want)
		}
	}
	for _, path := range []string{
		filepath.Join(stateDir, "notes.md"),
		filepath.Join(stateDir, "scratch"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("attach added built-in starter path despite custom template %s: %v", path, err)
		}
	}
	if got := runGitOutput(t, public, "status", "--short"); got != "" {
		t.Fatalf("public repo status = %q, want clean", got)
	}
}

func TestAttachEmptyStarterTemplateCreatesEmptyStateDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	if err := os.Mkdir(filepath.Join(state, ".starter"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	withChdir(t, public, func() {
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})
	if !strings.Contains(out.String(), "Starter:     "+filepath.Join(state, ".starter")) {
		t.Fatalf("attach output = %q, want empty custom starter path", out.String())
	}

	stateDir := filepath.Join(state, "github.com", "massivemoose", "ovek")
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("empty .starter created project files: %v", entries)
	}
}

func TestAttachDoesNotCopyStarterIntoExistingStateDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	stateDir := filepath.Join(state, "github.com", "massivemoose", "ovek")
	starter := filepath.Join(state, ".starter")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	if err := os.MkdirAll(starter, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(starter, "template.md"), []byte("template\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "custom.md"), []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	withChdir(t, public, func() {
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})
	if !strings.Contains(out.String(), "Starter:     existing archive (contents unchanged)") {
		t.Fatalf("attach output = %q, want unchanged starter message", out.String())
	}

	if _, err := os.Stat(filepath.Join(stateDir, "template.md")); !os.IsNotExist(err) {
		t.Fatalf("attach copied .starter into existing state dir: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(stateDir, "custom.md")); err != nil || string(got) != "custom\n" {
		t.Fatalf("custom state file changed: got %q, err %v", string(got), err)
	}
}

func TestAttachRejectsSymlinkInStarterTemplate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	starter := filepath.Join(state, ".starter")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	if err := os.MkdirAll(starter, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp", filepath.Join(starter, "link")); err != nil {
		t.Fatal(err)
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code == 0 {
			t.Fatalf("attach succeeded with symlink in starter template, stdout = %s", out.String())
		}
		if !strings.Contains(errOut.String(), ".starter contains unsupported entry") {
			t.Fatalf("attach stderr = %q, want unsupported starter entry error", errOut.String())
		}
	})
}

func TestAttachDoesNotRecreateDeletedStarterFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("first attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	stateDir := filepath.Join(state, "github.com", "massivemoose", "ovek")
	for _, path := range []string{
		filepath.Join(stateDir, "notes.md"),
		filepath.Join(stateDir, "llm"),
		filepath.Join(stateDir, "scratch"),
	} {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("second attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	for _, path := range []string{
		filepath.Join(stateDir, "notes.md"),
		filepath.Join(stateDir, "llm"),
		filepath.Join(stateDir, "scratch"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("attach recreated deleted starter path %s: %v", path, err)
		}
	}
}

func TestAttachPreservesExistingCustomStateDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	stateDir := filepath.Join(state, "github.com", "massivemoose", "ovek")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "custom.md"), []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	for _, path := range []string{
		filepath.Join(stateDir, "notes.md"),
		filepath.Join(stateDir, "llm"),
		filepath.Join(stateDir, "scratch"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("attach added starter path to custom state dir %s: %v", path, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(stateDir, "custom.md")); err != nil || string(got) != "custom\n" {
		t.Fatalf("custom state file changed: got %q, err %v", string(got), err)
	}
}

func TestAttachPreservesExistingEmptyStateDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	stateDir := filepath.Join(state, "github.com", "massivemoose", "ovek")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("attach added starter paths to existing empty state dir: %v", entries)
	}
}

func TestAttachRejectsStatePathThatIsFile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	stateDir := filepath.Join(state, "github.com", "massivemoose", "ovek")
	mustRunBacklotInit(t, state)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	if err := os.MkdirAll(filepath.Dir(stateDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateDir, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", state}, &out, &errOut); code == 0 {
			t.Fatalf("attach succeeded with file at state path, stdout = %s", out.String())
		}
		if !strings.Contains(errOut.String(), "exists and is not a directory") {
			t.Fatalf("attach stderr = %q, want not-directory error", errOut.String())
		}
	})
}

func TestDetachRemovesManagedSymlinkAndExcludeEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	stateDir := filepath.Join(state, "github.com", "massivemoose", "ovek")
	public := filepath.Join(tmp, "public")
	mustRunGit(t, tmp, "init", public)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stateDir, filepath.Join(public, ".backlot")); err != nil {
		t.Fatal(err)
	}
	excludePath := filepath.Join(public, ".git", "info", "exclude")
	exclude := "# local excludes\n.backlot\n.backlot/\n*.log\n"
	if err := os.WriteFile(excludePath, []byte(exclude), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(public, ".git", "hooks", "pre-commit")
	hookData := []byte("# custom hook\n")
	if err := os.WriteFile(hook, hookData, 0o755); err != nil {
		t.Fatal(err)
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"detach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("detach exit code = %d, stderr = %s", code, errOut.String())
		}
		if !strings.Contains(out.String(), "Detached Backlot") {
			t.Fatalf("detach output = %q, want success message", out.String())
		}
	})

	if _, err := os.Lstat(filepath.Join(public, ".backlot")); !os.IsNotExist(err) {
		t.Fatalf(".backlot still exists after detach: %v", err)
	}
	if _, err := os.Stat(stateDir); err != nil {
		t.Fatalf("detach removed private state dir: %v", err)
	}
	gotExclude, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotExclude) != "# local excludes\n*.log\n" {
		t.Fatalf("exclude contents = %q, want unrelated entries preserved", string(gotExclude))
	}
	gotHook, err := os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotHook) != string(hookData) {
		t.Fatalf("detach changed pre-commit hook: %q", string(gotHook))
	}
}

func TestDetachRemovesBrokenManagedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "deleted-state")
	public := filepath.Join(tmp, "public")
	mustRunGit(t, tmp, "init", public)
	if err := os.Symlink(filepath.Join(state, "github.com", "massivemoose", "ovek"), filepath.Join(public, ".backlot")); err != nil {
		t.Fatal(err)
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"detach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("detach exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	if _, err := os.Lstat(filepath.Join(public, ".backlot")); !os.IsNotExist(err) {
		t.Fatalf("broken .backlot symlink still exists after detach: %v", err)
	}
}

func TestDetachMissingLinkCleansExcludeWithoutOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	public := filepath.Join(tmp, "public")
	mustRunGit(t, tmp, "init", public)
	excludePath := filepath.Join(public, ".git", "info", "exclude")
	if err := os.WriteFile(excludePath, []byte(".backlot\n.keep\n.backlot/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"detach", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("detach exit code = %d, stderr = %s", code, errOut.String())
		}
		if !strings.Contains(out.String(), "No .backlot link found") {
			t.Fatalf("detach output = %q, want missing link message", out.String())
		}
	})

	gotExclude, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotExclude) != ".keep\n" {
		t.Fatalf("exclude contents = %q, want Backlot entries removed", string(gotExclude))
	}
}

func TestDetachRefusesUnmanagedBacklotPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tests := []struct {
		name  string
		setup func(t *testing.T, public string, state string)
	}{
		{
			name: "directory",
			setup: func(t *testing.T, public string, state string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(public, ".backlot"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "outside symlink",
			setup: func(t *testing.T, public string, state string) {
				t.Helper()
				outside := filepath.Join(filepath.Dir(state), "outside")
				if err := os.Mkdir(outside, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(public, ".backlot")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			state := filepath.Join(tmp, "state")
			public := filepath.Join(tmp, "public")
			mustRunGit(t, tmp, "init", public)
			excludePath := filepath.Join(public, ".git", "info", "exclude")
			if err := os.WriteFile(excludePath, []byte(".backlot\n.backlot/\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			tt.setup(t, public, state)

			withChdir(t, public, func() {
				var out, errOut bytes.Buffer
				if code := Run([]string{"detach", "--root", state}, &out, &errOut); code == 0 {
					t.Fatalf("detach succeeded for unmanaged path, stdout = %s", out.String())
				}
				if !strings.Contains(errOut.String(), "is not managed by Backlot") {
					t.Fatalf("detach stderr = %q, want unmanaged error", errOut.String())
				}
			})

			if _, err := os.Lstat(filepath.Join(public, ".backlot")); err != nil {
				t.Fatalf(".backlot was removed despite refusal: %v", err)
			}
			gotExclude, err := os.ReadFile(excludePath)
			if err != nil {
				t.Fatal(err)
			}
			if string(gotExclude) != ".backlot\n.backlot/\n" {
				t.Fatalf("exclude changed despite refusal: %q", string(gotExclude))
			}
		})
	}
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
		if !strings.Contains(errOut.String(), "inside Git repo") {
			t.Fatalf("attach stderr = %q, want inside-repo message", errOut.String())
		}
	})
}

func TestAttachRejectsBacklotRootEqualToProjectRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	public := filepath.Join(t.TempDir(), "public")
	mustRunGit(t, filepath.Dir(public), "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")

	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"attach", "--root", public}, &out, &errOut); code == 0 {
			t.Fatalf("attach accepted project repo as Backlot root, stdout = %s", out.String())
		}
		if !strings.Contains(errOut.String(), "inside current Git repo") {
			t.Fatalf("attach stderr = %q, want inside-project message", errOut.String())
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
	mustRunGit(t, state, "add", "README.md", ".backlot-root")
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
	mustRunGit(t, state, "add", "README.md", ".backlot-root")
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

func TestSyncCleanWithExistingUpstreamPullsRemoteAheadChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	stateA := filepath.Join(tmp, "state-a")
	stateB := filepath.Join(tmp, "state-b")
	var out, errOut bytes.Buffer
	if code := Run([]string{"clone", remote, "--root", stateA}, &out, &errOut); code != 0 {
		t.Fatalf("clone A exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, stateA)
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"clone", remote, "--root", stateB}, &out, &errOut); code != 0 {
		t.Fatalf("clone B exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, stateB)

	notes := filepath.Join(stateA, "github.com", "massivemoose", "ovek", "notes.md")
	if err := os.MkdirAll(filepath.Dir(notes), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notes, []byte("from A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", stateA, "-m", "A update"}, &out, &errOut); code != 0 {
		t.Fatalf("sync A exit code = %d, stderr = %s", code, errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", stateB}, &out, &errOut); code != 0 {
		t.Fatalf("sync B exit code = %d, stderr = %s", code, errOut.String())
	}
	got, err := os.ReadFile(filepath.Join(stateB, "github.com", "massivemoose", "ovek", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from A\n" {
		t.Fatalf("state B notes = %q, want pulled remote change", string(got))
	}
}

func TestSyncCleanWithExistingUpstreamPushesLocalAheadCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	state := filepath.Join(tmp, "state")
	var out, errOut bytes.Buffer
	if code := Run([]string{"clone", remote, "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("clone exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, state)
	if err := os.WriteFile(filepath.Join(state, "local.md"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, state, "add", "local.md")
	mustRunGit(t, state, "commit", "-m", "Local ahead")

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("sync exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := runGitOutput(t, remote, "log", "-1", "--pretty=%s"); got != "Local ahead" {
		t.Fatalf("remote last commit = %q, want local-ahead commit", got)
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
	mustRunGit(t, state, "add", "README.md", ".backlot-root")
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

func TestSyncConflictPrintsRecoveryAndRerunRefuses(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	stateB, _, errOut := createInterruptedSync(t)
	stderr := errOut.String()
	for _, want := range []string{
		"Backlot sync hit a Git conflict",
		"backlot sync --continue",
		"backlot sync --abort",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("conflict stderr missing %q:\n%s", want, stderr)
		}
	}

	var out bytes.Buffer
	errOut.Reset()
	if code := Run([]string{"sync", "--root", stateB}, &out, &errOut); code == 0 {
		t.Fatalf("sync rerun succeeded during unfinished rebase, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "unfinished Git operation") {
		t.Fatalf("rerun stderr = %q, want unfinished operation guidance", errOut.String())
	}
}

func TestSyncConflictShowsProjectFriendlyRecoveryPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	_, _, errOut := createAttachedInterruptedSync(t)
	stderr := errOut.String()
	if !strings.Contains(stderr, "edit .backlot/notes.md") {
		t.Fatalf("conflict stderr = %q, want project-friendly .backlot path", stderr)
	}
	recovery := stderr[strings.Index(stderr, "Backlot sync hit a Git conflict"):]
	if strings.Contains(recovery, "git -C ") {
		t.Fatalf("recovery instructions include raw git command:\n%s", recovery)
	}
}

func TestSyncConflictOutsideAttachedProjectShowsArchiveFallback(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	state, _, errOut := createInterruptedSync(t)
	stderr := errOut.String()
	if !strings.Contains(stderr, "edit the conflicted files under "+state) {
		t.Fatalf("conflict stderr = %q, want archive-root fallback", stderr)
	}
	if strings.Contains(stderr, "edit .backlot/notes.md") {
		t.Fatalf("conflict stderr unexpectedly used project path outside attached project:\n%s", stderr)
	}
}

func TestSyncAbortAbortsInterruptedRebase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	state, _, _ := createInterruptedSync(t)
	var out, errOut bytes.Buffer
	if code := Run([]string{"sync", "--root", state, "--abort"}, &out, &errOut); code != 0 {
		t.Fatalf("sync --abort exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Backlot sync aborted.") {
		t.Fatalf("sync --abort output = %q, want aborted message", out.String())
	}
	if _, err := os.Stat(filepath.Join(state, ".git", "rebase-merge")); !os.IsNotExist(err) {
		t.Fatalf("rebase still in progress after abort: %v", err)
	}
	if got := runGitOutput(t, state, "status", "--short"); got != "" {
		t.Fatalf("state status after abort = %q, want clean", got)
	}
}

func TestSyncContinueContinuesRebaseAndPushes(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	state, remote, _ := createInterruptedSync(t)
	notes := filepath.Join(state, "github.com", "massivemoose", "ovek", "notes.md")
	if err := os.WriteFile(notes, []byte("resolved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"sync", "--root", state, "--continue"}, &out, &errOut); code != 0 {
		t.Fatalf("sync --continue exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Backlot state synced.") {
		t.Fatalf("sync --continue output = %q, want synced message", out.String())
	}
	if got := runGitOutput(t, remote, "log", "-1", "--pretty=%s"); got != "B update" {
		t.Fatalf("remote last commit = %q, want rebased local commit", got)
	}
	got, err := os.ReadFile(filepath.Join(state, "github.com", "massivemoose", "ovek", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "resolved\n" {
		t.Fatalf("resolved state file = %q, want resolved content", string(got))
	}
}

func TestSyncContinueWithoutInterruptedSyncFailsClearly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	state := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, state)
	var out, errOut bytes.Buffer
	if code := Run([]string{"sync", "--root", state, "--continue"}, &out, &errOut); code == 0 {
		t.Fatalf("sync --continue succeeded without interrupted sync, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "no interrupted Backlot sync") {
		t.Fatalf("sync --continue stderr = %q, want no-interrupted-sync guidance", errOut.String())
	}
}

func TestSyncRejectsContinueAndAbortTogether(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Run([]string{"sync", "--continue", "--abort"}, &out, &errOut); code == 0 {
		t.Fatalf("sync accepted --continue and --abort together, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "choose only one") {
		t.Fatalf("sync stderr = %q, want mutually exclusive guidance", errOut.String())
	}
}

func TestSyncNoUpstreamExistingRemoteBranchFailsClearly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "state")
	remote := filepath.Join(tmp, "remote.git")
	seed := filepath.Join(tmp, "seed")
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", state}, &out, &errOut); code != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, state)
	branch := runGitOutput(t, state, "branch", "--show-current")
	mustRunGit(t, tmp, "init", "--bare", remote)
	mustRunGit(t, tmp, "init", "-b", branch, seed)
	configureGitIdentity(t, seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("remote\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, seed, "add", "README.md")
	mustRunGit(t, seed, "commit", "-m", "Remote archive")
	mustRunGit(t, seed, "remote", "add", "origin", remote)
	mustRunGit(t, seed, "push", "origin", branch)
	mustRunGit(t, state, "remote", "add", "origin", remote)
	mustRunGit(t, state, "add", "README.md", ".backlot-root")
	mustRunGit(t, state, "commit", "-m", "Local archive")

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", state}, &out, &errOut); code == 0 {
		t.Fatalf("sync succeeded against existing remote branch without upstream, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "already has a remote branch") {
		t.Fatalf("sync stderr = %q, want existing remote branch guidance", errOut.String())
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
	if !strings.Contains(stderr, "fetch failed while syncing Backlot root") {
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
	mustRunBacklotInit(t, state)
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
	mustRunBacklotInit(t, state)
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

func TestDoctorReturnsNonzeroForBrokenSetup(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	tmp := t.TempDir()
	state := filepath.Join(tmp, "missing-state")
	public := filepath.Join(tmp, "public")
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"doctor", "--root", state}, &out, &errOut); code == 0 {
			t.Fatalf("doctor returned success for broken setup, stdout = %s", out.String())
		}
		if !strings.Contains(out.String(), "✗ Backlot root exists") {
			t.Fatalf("doctor output = %q, want failed Backlot root check", out.String())
		}
	})
}

func TestStatusReportsInterruptedSync(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	state, public, _ := createAttachedInterruptedSync(t)
	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"status", "--root", state}, &out, &errOut); code != 0 {
			t.Fatalf("status exit code = %d, stderr = %s", code, errOut.String())
		}
		text := out.String()
		if !strings.Contains(text, "State repo:    sync interrupted") {
			t.Fatalf("status output = %q, want interrupted state", text)
		}
		if !strings.Contains(text, "Recovery:      resolve conflicts in .backlot/ and run backlot sync --continue") {
			t.Fatalf("status output = %q, want recovery guidance", text)
		}
	})
}

func TestDoctorReportsInterruptedSync(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	state, public, _ := createAttachedInterruptedSync(t)
	withChdir(t, public, func() {
		var out, errOut bytes.Buffer
		if code := Run([]string{"doctor", "--root", state}, &out, &errOut); code == 0 {
			t.Fatalf("doctor succeeded during interrupted sync, stdout = %s", out.String())
		}
		text := out.String()
		for _, want := range []string{
			"✗ Backlot sync was interrupted by a conflict",
			"resolve conflicts in .backlot/",
			"backlot sync --continue",
			"backlot sync --abort",
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
	if _, err := gitutil.RunGit(dir, args...); err != nil {
		t.Fatal(err)
	}
}

func mustRunBacklotInit(t *testing.T, root string) {
	t.Helper()
	var out, errOut bytes.Buffer
	if code := Run([]string{"init", "--root", root}, &out, &errOut); code != 0 {
		t.Fatalf("init Backlot root %s exit code = %d, stderr = %s", root, code, errOut.String())
	}
}

func configureGitIdentity(t *testing.T, dir string) {
	t.Helper()
	mustRunGit(t, dir, "config", "user.name", "Backlot Test")
	mustRunGit(t, dir, "config", "user.email", "backlot@example.invalid")
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitutil.RunGit(dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func createInterruptedSync(t *testing.T) (string, string, bytes.Buffer) {
	t.Helper()
	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	stateA := filepath.Join(tmp, "state-a")
	stateB := filepath.Join(tmp, "state-b")
	var out, errOut bytes.Buffer
	if code := Run([]string{"clone", remote, "--root", stateA}, &out, &errOut); code != 0 {
		t.Fatalf("clone A exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, stateA)
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"clone", remote, "--root", stateB}, &out, &errOut); code != 0 {
		t.Fatalf("clone B exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, stateB)

	notesA := filepath.Join(stateA, "github.com", "massivemoose", "ovek", "notes.md")
	if err := os.MkdirAll(filepath.Dir(notesA), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notesA, []byte("from A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", stateA, "-m", "A update"}, &out, &errOut); code != 0 {
		t.Fatalf("sync A exit code = %d, stderr = %s", code, errOut.String())
	}

	notesB := filepath.Join(stateB, "github.com", "massivemoose", "ovek", "notes.md")
	if err := os.MkdirAll(filepath.Dir(notesB), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notesB, []byte("from B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", stateB, "-m", "B update"}, &out, &errOut); code == 0 {
		t.Fatalf("sync B succeeded despite conflict, stdout = %s", out.String())
	}
	return stateB, remote, errOut
}

func createAttachedInterruptedSync(t *testing.T) (string, string, bytes.Buffer) {
	t.Helper()
	tmp := t.TempDir()
	remote := createBacklotArchive(t, tmp)
	stateA := filepath.Join(tmp, "state-a")
	stateB := filepath.Join(tmp, "state-b")
	public := filepath.Join(tmp, "public")
	var out, errOut bytes.Buffer
	if code := Run([]string{"clone", remote, "--root", stateA}, &out, &errOut); code != 0 {
		t.Fatalf("clone A exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, stateA)
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"clone", remote, "--root", stateB}, &out, &errOut); code != 0 {
		t.Fatalf("clone B exit code = %d, stderr = %s", code, errOut.String())
	}
	configureGitIdentity(t, stateB)
	mustRunGit(t, tmp, "init", public)
	mustRunGit(t, public, "remote", "add", "origin", "git@github.com:massivemoose/ovek.git")
	withChdir(t, public, func() {
		out.Reset()
		errOut.Reset()
		if code := Run([]string{"attach", "--root", stateB}, &out, &errOut); code != 0 {
			t.Fatalf("attach exit code = %d, stderr = %s", code, errOut.String())
		}
	})

	notesA := filepath.Join(stateA, "github.com", "massivemoose", "ovek", "notes.md")
	if err := os.MkdirAll(filepath.Dir(notesA), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notesA, []byte("from A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"sync", "--root", stateA, "-m", "A update"}, &out, &errOut); code != 0 {
		t.Fatalf("sync A exit code = %d, stderr = %s", code, errOut.String())
	}

	notesB := filepath.Join(public, ".backlot", "notes.md")
	if err := os.WriteFile(notesB, []byte("from B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withChdir(t, public, func() {
		out.Reset()
		errOut.Reset()
		if code := Run([]string{"sync", "--root", stateB, "-m", "B update"}, &out, &errOut); code == 0 {
			t.Fatalf("sync B succeeded despite conflict, stdout = %s", out.String())
		}
	})
	return stateB, public, errOut
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

func createBacklotArchive(t *testing.T, tmp string) string {
	t.Helper()
	remote := filepath.Join(tmp, "archive.git")
	seed := filepath.Join(tmp, "seed")
	mustRunGit(t, tmp, "init", "--bare", remote)
	mustRunGit(t, tmp, "init", "-b", "main", seed)
	configureGitIdentity(t, seed)
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("# Backlot archive\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, ".backlot-root"), []byte(archiveMarker), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRunGit(t, seed, "add", "README.md", ".backlot-root")
	mustRunGit(t, seed, "commit", "-m", "Initial archive")
	mustRunGit(t, seed, "remote", "add", "origin", remote)
	mustRunGit(t, seed, "push", "origin", "main")
	mustRunGit(t, remote, "symbolic-ref", "HEAD", "refs/heads/main")
	return remote
}
