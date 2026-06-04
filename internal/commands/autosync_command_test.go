package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/massivemoose/backlot/internal/autosync"
)

func TestAutosyncEnableStatusDisable(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, root)
	binary := filepath.Join(t.TempDir(), "backlot")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	loaded := false
	restore := stubAutosyncCommandEnvironment(t, home, binary, func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		switch args[0] {
		case "print":
			if loaded {
				return nil
			}
			return errors.New("not loaded")
		case "bootstrap":
			loaded = true
			return nil
		case "bootout":
			loaded = false
			return nil
		default:
			return nil
		}
	})
	defer restore()

	var out, errOut bytes.Buffer
	if code := Run([]string{"autosync", "enable", "--root", root, "--interval", "20m"}, &out, &errOut); code != 0 {
		t.Fatalf("autosync enable exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "Auto-sync enabled.") || !strings.Contains(out.String(), "local-only") {
		t.Fatalf("enable output = %q, want enabled and local-only warning", out.String())
	}
	managedPaths, err := autosync.ResolvePaths(home, root)
	if err != nil {
		t.Fatal(err)
	}
	config, err := autosync.LoadConfig(managedPaths.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalBinary, err := filepath.EvalSymlinks(binary)
	if err != nil {
		t.Fatal(err)
	}
	if config.IntervalSeconds != 1200 || config.Binary != canonicalBinary {
		t.Fatalf("config = %+v, want interval 1200 and binary %s", config, canonicalBinary)
	}
	if _, err := os.Stat(managedPaths.PlistPath); err != nil {
		t.Fatalf("LaunchAgent missing after enable: %v", err)
	}
	if len(calls) != 2 || calls[0][0] != "print" || calls[1][0] != "bootstrap" {
		t.Fatalf("launchctl calls = %v, want print then bootstrap", calls)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"autosync", "status", "--root", root}, &out, &errOut); code != 0 {
		t.Fatalf("autosync status exit code = %d, stderr = %s", code, errOut.String())
	}
	for _, want := range []string{"Auto-sync:     enabled", "Loaded:        yes", "Interval:      20m0s", "Origin:        local-only"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"autosync", "enable", "--root", root, "--interval", "30m"}, &out, &errOut); code != 0 {
		t.Fatalf("autosync re-enable exit code = %d, stderr = %s", code, errOut.String())
	}
	if len(calls) != 6 || calls[3][0] != "print" || calls[4][0] != "bootout" || calls[5][0] != "bootstrap" {
		t.Fatalf("launchctl calls after re-enable = %v, want print/bootout/bootstrap", calls)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"autosync", "disable", "--root", root}, &out, &errOut); code != 0 {
		t.Fatalf("autosync disable exit code = %d, stderr = %s", code, errOut.String())
	}
	if _, err := os.Stat(managedPaths.PlistPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("LaunchAgent still exists after disable: %v", err)
	}
	if _, err := os.Stat(managedPaths.RuntimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime still exists after disable: %v", err)
	}
}

func TestAutosyncEnableValidatesIntervalAndConflictState(t *testing.T) {
	home := t.TempDir()
	binary := filepath.Join(t.TempDir(), "backlot")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	restore := stubAutosyncCommandEnvironment(t, home, binary, func(...string) error { return errors.New("not loaded") })
	defer restore()

	root := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, root)
	var out, errOut bytes.Buffer
	if code := Run([]string{"autosync", "enable", "--root", root, "--interval", "30s"}, &out, &errOut); code == 0 {
		t.Fatalf("autosync enable accepted short interval, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "at least 1m") {
		t.Fatalf("short interval stderr = %q", errOut.String())
	}

	state, _, _ := createInterruptedSync(t)
	out.Reset()
	errOut.Reset()
	if code := Run([]string{"autosync", "enable", "--root", state}, &out, &errOut); code == 0 {
		t.Fatalf("autosync enable accepted interrupted sync, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "resolve or abort") {
		t.Fatalf("interrupted enable stderr = %q", errOut.String())
	}
}

func TestAutosyncRefusesForeignManagedFiles(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, root)
	binary := filepath.Join(t.TempDir(), "backlot")
	if err := os.WriteFile(binary, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	restore := stubAutosyncCommandEnvironment(t, home, binary, func(...string) error { return errors.New("not loaded") })
	defer restore()

	managedPaths, err := autosync.ResolvePaths(home, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(managedPaths.PlistPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedPaths.PlistPath, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Run([]string{"autosync", "enable", "--root", root}, &out, &errOut); code == 0 {
		t.Fatalf("autosync enable overwrote foreign plist, stdout = %s", out.String())
	}
	if !strings.Contains(errOut.String(), "not managed by Backlot") {
		t.Fatalf("foreign plist stderr = %q", errOut.String())
	}
}

func stubAutosyncCommandEnvironment(t *testing.T, home, binary string, launchctl func(...string) error) func() {
	t.Helper()
	oldHome := autosyncHomeDir
	oldExecutable := autosyncExecutable
	oldLaunchctl := autosyncLaunchctl
	oldGOOS := autosyncGOOS
	oldUID := autosyncUID
	autosyncHomeDir = func() (string, error) { return home, nil }
	autosyncExecutable = func() (string, error) { return binary, nil }
	autosyncLaunchctl = launchctl
	autosyncGOOS = "darwin"
	autosyncUID = func() int { return 501 }
	return func() {
		autosyncHomeDir = oldHome
		autosyncExecutable = oldExecutable
		autosyncLaunchctl = oldLaunchctl
		autosyncGOOS = oldGOOS
		autosyncUID = oldUID
	}
}
