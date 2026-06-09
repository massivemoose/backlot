package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/massivemoose/backlot/internal/autosync"
	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

const defaultAutosyncInterval = 15 * time.Minute

var errAutosyncNotLoaded = errors.New("auto-sync LaunchAgent is not loaded")

var (
	autosyncExecutable = os.Executable
	autosyncGOOS       = runtime.GOOS
	autosyncUID        = os.Getuid
	autosyncLaunchctl  = runLaunchctl
)

func autosyncRouter(stdout, stderr io.Writer) *chomp.Router {
	return chomp.NewRouter(
		"autosync",
		"Manage automatic Backlot sync.",
		runnableCommand{
			name:    "enable",
			summary: "Enable automatic sync",
			run:     func(args []string) error { return runAutosyncEnable(args, stdout, stderr) },
			usage:   printAutosyncEnableUsage,
		},
		runnableCommand{
			name:    "disable",
			summary: "Disable automatic sync",
			run:     func(args []string) error { return runAutosyncDisable(args, stdout, stderr) },
			usage:   printAutosyncDisableUsage,
		},
		runnableCommand{
			name:    "run",
			summary: "Run managed automatic sync",
			hidden:  true,
			run:     func(args []string) error { return runAutosyncManagedRun(args, stderr) },
			usage:   printAutosyncRunUsage,
		},
		runnableCommand{
			name:    "status",
			summary: "Show automatic sync status",
			run:     func(args []string) error { return runAutosyncStatus(args, stdout, stderr) },
			usage:   printAutosyncStatusUsage,
		},
	)
}

func runAutosyncEnable(args []string, stdout, stderr io.Writer) error {
	result, err := autosyncEnableSpec().Parse(args)
	if err != nil {
		return err
	}
	if err := requireAutosyncPlatform(); err != nil {
		return err
	}
	interval, err := time.ParseDuration(result.String("interval"))
	if err != nil {
		return fmt.Errorf("parse autosync interval: %w", err)
	}
	if interval < time.Minute {
		return fmt.Errorf("autosync interval must be at least 1m")
	}
	root, err := resolveAutosyncRoot(result.String("root"))
	if err != nil {
		return err
	}
	state, err := detectSyncState(root)
	if err != nil {
		return err
	}
	if state.Interrupted() {
		return fmt.Errorf("Backlot sync is interrupted; resolve or abort it before enabling auto-sync")
	}
	home, managedPaths, err := resolveAutosyncPaths(root)
	if err != nil {
		return err
	}
	_ = home
	binary, err := resolveAutosyncExecutable()
	if err != nil {
		return err
	}
	if err := verifyAutosyncOwnership(managedPaths, true); err != nil {
		return err
	}
	loaded, err := autosyncLoaded(managedPaths.Label)
	if err != nil {
		return fmt.Errorf("inspect existing auto-sync LaunchAgent: %w", err)
	}
	previous, err := captureAutosyncManagedFiles(managedPaths)
	if err != nil {
		return err
	}
	if loaded && !previous.plist.exists {
		return fmt.Errorf("loaded auto-sync LaunchAgent %s has no managed plist to update safely", managedPaths.Label)
	}
	config := autosync.Config{
		SchemaVersion:   autosync.SchemaVersion,
		ManagedBy:       autosync.ManagedBy,
		Root:            managedPaths.Root,
		Binary:          binary,
		Label:           managedPaths.Label,
		PlistPath:       managedPaths.PlistPath,
		IntervalSeconds: int(interval / time.Second),
	}
	plist, err := autosync.RenderLaunchAgent(managedPaths, config)
	if err != nil {
		return err
	}
	if err := autosync.WriteConfig(managedPaths.ConfigPath, config); err != nil {
		return rollbackAutosyncEnable(managedPaths, previous, false, err)
	}
	if err := writeAutosyncManagedFile(managedPaths.PlistPath, plist); err != nil {
		return rollbackAutosyncEnable(managedPaths, previous, false, err)
	}
	if loaded {
		if err := autosyncLaunchctl("bootout", autosyncServiceTarget(managedPaths.Label)); err != nil {
			return rollbackAutosyncEnable(managedPaths, previous, false,
				fmt.Errorf("unload existing auto-sync LaunchAgent: %w", err))
		}
	}
	if err := autosyncLaunchctl("bootstrap", autosyncDomainTarget(), managedPaths.PlistPath); err != nil {
		return rollbackAutosyncEnable(managedPaths, previous, loaded,
			fmt.Errorf("load auto-sync LaunchAgent: %w", err))
	}
	fmt.Fprintln(stdout, "Auto-sync enabled.")
	fmt.Fprintf(stdout, "Root:           %s\n", managedPaths.Root)
	fmt.Fprintf(stdout, "Interval:       %s\n", interval)
	fmt.Fprintln(stdout, "First run:      immediate")
	if !gitutil.HasOrigin(managedPaths.Root) {
		fmt.Fprintln(stdout, "Warning: archive is local-only; automatic commits will not be backed up remotely.")
	}
	return nil
}

func runAutosyncDisable(args []string, stdout, stderr io.Writer) error {
	rootFlag, err := parseAutosyncRootOnly("disable", args)
	if err != nil {
		return err
	}
	if err := requireAutosyncPlatform(); err != nil {
		return err
	}
	root, err := resolveAutosyncManagedRoot(rootFlag)
	if err != nil {
		return err
	}
	_, managedPaths, err := resolveAutosyncPaths(root)
	if err != nil {
		return err
	}
	configExists, err := autosyncManagedConfigExists(managedPaths)
	if err != nil {
		return err
	}
	if !configExists {
		if _, err := os.Lstat(managedPaths.PlistPath); err == nil {
			return fmt.Errorf("auto-sync LaunchAgent %s is not managed by Backlot", managedPaths.PlistPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Fprintln(stdout, "Auto-sync is already disabled.")
		return nil
	}
	if err := verifyAutosyncOwnership(managedPaths, true); err != nil {
		return err
	}
	loaded, err := autosyncLoaded(managedPaths.Label)
	if err != nil {
		return fmt.Errorf("inspect auto-sync LaunchAgent: %w", err)
	}
	if loaded {
		if err := autosyncLaunchctl("bootout", autosyncServiceTarget(managedPaths.Label)); err != nil {
			return fmt.Errorf("unload auto-sync LaunchAgent: %w", err)
		}
	}
	if err := os.Remove(managedPaths.PlistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := autosync.RemoveRuntime(managedPaths); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Auto-sync disabled.")
	return nil
}

func runAutosyncStatus(args []string, stdout, stderr io.Writer) error {
	rootFlag, err := parseAutosyncRootOnly("status", args)
	if err != nil {
		return err
	}
	if err := requireAutosyncPlatform(); err != nil {
		return err
	}
	root, err := resolveAutosyncManagedRoot(rootFlag)
	if err != nil {
		return err
	}
	_, managedPaths, err := resolveAutosyncPaths(root)
	if err != nil {
		return err
	}
	configExists, err := autosyncManagedConfigExists(managedPaths)
	if err != nil {
		return err
	}
	if !configExists {
		if _, err := os.Lstat(managedPaths.PlistPath); err == nil {
			return fmt.Errorf("auto-sync LaunchAgent %s is not managed by Backlot", managedPaths.PlistPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		fmt.Fprintln(stdout, "Auto-sync:     disabled")
		fmt.Fprintf(stdout, "Root:          %s\n", managedPaths.Root)
		return nil
	}
	if err := verifyAutosyncOwnership(managedPaths, true); err != nil {
		return err
	}
	config, err := autosync.LoadConfig(managedPaths.ConfigPath)
	if err != nil {
		return err
	}
	loaded, err := autosyncLoaded(managedPaths.Label)
	if err != nil {
		return fmt.Errorf("inspect auto-sync LaunchAgent: %w", err)
	}
	fmt.Fprintln(stdout, "Auto-sync:     enabled")
	fmt.Fprintf(stdout, "Loaded:        %s\n", yesNo(loaded))
	if _, err := os.Stat(managedPaths.PlistPath); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(stdout, "LaunchAgent:   missing")
	} else if err != nil {
		return err
	} else {
		fmt.Fprintln(stdout, "LaunchAgent:   present")
	}
	fmt.Fprintf(stdout, "Root:          %s\n", managedPaths.Root)
	fmt.Fprintf(stdout, "Interval:      %s\n", time.Duration(config.IntervalSeconds)*time.Second)
	fmt.Fprintf(stdout, "Binary:        %s\n", config.Binary)
	if _, err := os.Stat(config.Binary); err != nil {
		fmt.Fprintln(stdout, "Binary status: missing")
	}
	if gitutil.HasOrigin(managedPaths.Root) {
		origin, _ := gitutil.OriginURL(managedPaths.Root)
		fmt.Fprintf(stdout, "Origin:        %s\n", origin)
	} else {
		fmt.Fprintln(stdout, "Origin:        local-only")
	}
	state, err := loadManagedAutosyncState(managedPaths.StatePath)
	if err != nil {
		return err
	}
	printAutosyncState(stdout, managedPaths, state)
	return nil
}

func runAutosyncManagedRun(args []string, stderr io.Writer) error {
	rootFlag, err := parseAutosyncRootOnly("run", args)
	if err != nil {
		return err
	}
	root, err := paths.BacklotRoot(rootFlag)
	if err != nil {
		return err
	}
	return runManagedAutosync(root)
}

func parseAutosyncRootOnly(command string, args []string) (string, error) {
	result, err := autosyncRootOnlySpec(command).Parse(args)
	if err != nil {
		return "", err
	}
	return result.String("root"), nil
}

func autosyncEnableSpec() *chomp.Spec {
	return chomp.New("backlot", "autosync", "enable").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		String("interval", chomp.ValueName("duration"), chomp.Default(defaultAutosyncInterval.String()), chomp.Description("automatic sync interval")).
		Positionals(0, 0)
}

func autosyncRootOnlySpec(command string) *chomp.Spec {
	return chomp.New("backlot", "autosync", command).
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		Positionals(0, 0)
}

func printAutosyncEnableUsage(w io.Writer) {
	printSpecUsage(w, autosyncEnableSpec())
}

func printAutosyncDisableUsage(w io.Writer) {
	printSpecUsage(w, autosyncRootOnlySpec("disable"))
}

func printAutosyncStatusUsage(w io.Writer) {
	printSpecUsage(w, autosyncRootOnlySpec("status"))
}

func printAutosyncRunUsage(w io.Writer) {
	printSpecUsage(w, autosyncRootOnlySpec("run"))
}

func resolveAutosyncRoot(rootFlag string) (string, error) {
	root, err := resolveAutosyncManagedRoot(rootFlag)
	if err != nil {
		return "", err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
		return "", err
	}
	return root, nil
}

func resolveAutosyncManagedRoot(rootFlag string) (string, error) {
	root, err := paths.BacklotRoot(rootFlag)
	if err != nil {
		return "", err
	}
	if err := ensureRootOutsideCurrentProject(root); err != nil {
		return "", err
	}
	return root, nil
}

func resolveAutosyncPaths(root string) (string, autosync.Paths, error) {
	home, err := autosyncHomeDir()
	if err != nil {
		return "", autosync.Paths{}, err
	}
	managedPaths, err := autosync.ResolvePaths(home, root)
	return home, managedPaths, err
}

func resolveAutosyncExecutable() (string, error) {
	binary, err := autosyncExecutable()
	if err != nil {
		return "", err
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(binary); err == nil {
		binary = resolved
	}
	if strings.Contains(binary, string(filepath.Separator)+"go-build") {
		return "", fmt.Errorf("cannot enable auto-sync from an ephemeral go run binary; install Backlot first")
	}
	info, err := os.Stat(binary)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("Backlot binary %s is not a regular file", binary)
	}
	return binary, nil
}

func verifyAutosyncOwnership(managedPaths autosync.Paths, allowMissing bool) error {
	config, err := autosync.LoadConfig(managedPaths.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		if _, plistErr := os.Lstat(managedPaths.PlistPath); plistErr == nil {
			return fmt.Errorf("auto-sync LaunchAgent %s is not managed by Backlot", managedPaths.PlistPath)
		} else if !errors.Is(plistErr, os.ErrNotExist) {
			return plistErr
		}
		if allowMissing {
			return nil
		}
		return err
	}
	if err != nil {
		return err
	}
	if err := autosync.ValidateManagedConfig(config, managedPaths); err != nil {
		return err
	}
	info, err := os.Lstat(managedPaths.ConfigPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("managed auto-sync file %s is not a regular file", managedPaths.ConfigPath)
	}
	info, err = os.Lstat(managedPaths.PlistPath)
	if errors.Is(err, os.ErrNotExist) && allowMissing {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("managed auto-sync file %s is not a regular file", managedPaths.PlistPath)
	}
	expected, err := autosync.RenderLaunchAgent(managedPaths, config)
	if err != nil {
		return err
	}
	actual, err := os.ReadFile(managedPaths.PlistPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf("auto-sync LaunchAgent %s is not managed by Backlot", managedPaths.PlistPath)
	}
	return nil
}

func autosyncManagedConfigExists(managedPaths autosync.Paths) (bool, error) {
	info, err := os.Lstat(managedPaths.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("managed auto-sync file %s is not a regular file", managedPaths.ConfigPath)
	}
	return true, nil
}

func writeAutosyncManagedFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

type autosyncFileSnapshot struct {
	path   string
	data   []byte
	exists bool
}

type autosyncManagedSnapshot struct {
	config autosyncFileSnapshot
	plist  autosyncFileSnapshot
}

func captureAutosyncManagedFiles(managedPaths autosync.Paths) (autosyncManagedSnapshot, error) {
	config, err := captureAutosyncFile(managedPaths.ConfigPath)
	if err != nil {
		return autosyncManagedSnapshot{}, err
	}
	plist, err := captureAutosyncFile(managedPaths.PlistPath)
	if err != nil {
		return autosyncManagedSnapshot{}, err
	}
	return autosyncManagedSnapshot{config: config, plist: plist}, nil
}

func captureAutosyncFile(path string) (autosyncFileSnapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return autosyncFileSnapshot{path: path}, nil
	}
	if err != nil {
		return autosyncFileSnapshot{}, err
	}
	return autosyncFileSnapshot{path: path, data: data, exists: true}, nil
}

func rollbackAutosyncEnable(managedPaths autosync.Paths, previous autosyncManagedSnapshot, reloadPrevious bool, cause error) error {
	if err := restoreAutosyncFile(previous.config); err != nil {
		return fmt.Errorf("%w; restore previous auto-sync configuration: %v", cause, err)
	}
	if err := restoreAutosyncFile(previous.plist); err != nil {
		return fmt.Errorf("%w; restore previous auto-sync LaunchAgent: %v", cause, err)
	}
	if reloadPrevious && previous.plist.exists {
		if err := autosyncLaunchctl("bootstrap", autosyncDomainTarget(), managedPaths.PlistPath); err != nil {
			return fmt.Errorf("%w; reload previous auto-sync LaunchAgent: %v", cause, err)
		}
	}
	return cause
}

func restoreAutosyncFile(snapshot autosyncFileSnapshot) error {
	if snapshot.exists {
		return writeAutosyncManagedFile(snapshot.path, snapshot.data)
	}
	if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func autosyncLoaded(label string) (bool, error) {
	err := autosyncLaunchctl("print", autosyncServiceTarget(label))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errAutosyncNotLoaded) {
		return false, nil
	}
	return false, err
}

func autosyncDomainTarget() string {
	return "gui/" + strconv.Itoa(autosyncUID())
}

func autosyncServiceTarget(label string) string {
	return autosyncDomainTarget() + "/" + label
}

func runLaunchctl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/bin/launchctl", args...).CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if len(args) > 0 && args[0] == "print" && errors.As(err, &exitErr) && exitErr.ExitCode() == 113 {
			return errAutosyncNotLoaded
		}
		return fmt.Errorf("launchctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func requireAutosyncPlatform() error {
	if autosyncGOOS != "darwin" {
		return fmt.Errorf("auto-sync scheduler management is currently supported on macOS only")
	}
	return nil
}

func printAutosyncState(stdout io.Writer, managedPaths autosync.Paths, state autosync.State) {
	if !state.LastRun.IsZero() {
		fmt.Fprintf(stdout, "Last run:      %s\n", state.LastRun.Format(time.RFC3339))
	}
	if state.Result != "" {
		fmt.Fprintf(stdout, "Last result:   %s\n", state.Result)
	}
	if !state.LastSuccess.IsZero() {
		fmt.Fprintf(stdout, "Last success:  %s\n", state.LastSuccess.Format(time.RFC3339))
	}
	if state.PausedReason != "" {
		fmt.Fprintf(stdout, "Paused:        %s\n", state.PausedReason)
	}
	for _, conflict := range state.ConflictPaths {
		fmt.Fprintf(stdout, "Conflict:      %s\n", conflict)
	}
	if state.ConsecutiveFailures > 0 {
		fmt.Fprintf(stdout, "Failures:      %d (%s)\n", state.ConsecutiveFailures, state.FailureCategory)
	}
	if state.LastError != "" {
		fmt.Fprintf(stdout, "Last error:    %s\n", state.LastError)
		fmt.Fprintf(stdout, "Error log:     %s\n", managedPaths.LogPath)
	}
	if state.RecoveryCommand != "" {
		fmt.Fprintf(stdout, "Recovery:      %s\n", state.RecoveryCommand)
	}
	if !state.LastNotification.IsZero() {
		fmt.Fprintf(stdout, "Last alert:    %s\n", state.LastNotification.Format(time.RFC3339))
	}
	if state.LastNotificationError != "" {
		fmt.Fprintf(stdout, "Alert error:   %s\n", state.LastNotificationError)
	}
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
