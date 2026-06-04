package commands

import (
	"context"
	"errors"
	"flag"
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
)

const defaultAutosyncInterval = 15 * time.Minute

var (
	autosyncExecutable = os.Executable
	autosyncGOOS       = runtime.GOOS
	autosyncUID        = os.Getuid
	autosyncLaunchctl  = runLaunchctl
)

func runAutosync(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printAutosyncUsage(stderr)
		return flag.ErrHelp
	}
	switch args[0] {
	case "enable":
		return runAutosyncEnable(args[1:], stdout, stderr)
	case "disable":
		return runAutosyncDisable(args[1:], stdout, stderr)
	case "status":
		return runAutosyncStatus(args[1:], stdout, stderr)
	case "run":
		return runAutosyncManagedRun(args[1:], stderr)
	case "--help", "-h":
		printAutosyncUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown autosync command %q", args[0])
	}
}

func printAutosyncUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  backlot autosync enable [--root PATH] [--interval DURATION]")
	fmt.Fprintln(w, "  backlot autosync disable [--root PATH]")
	fmt.Fprintln(w, "  backlot autosync status [--root PATH]")
}

func runAutosyncEnable(args []string, stdout, stderr io.Writer) error {
	if err := requireAutosyncPlatform(); err != nil {
		return err
	}
	fs := newFlagSet("autosync enable", stderr)
	fs.Usage = func() { printAutosyncUsage(stderr) }
	rootFlag := fs.String("root", "", "Backlot root path")
	intervalFlag := fs.String("interval", defaultAutosyncInterval.String(), "automatic sync interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}
	interval, err := time.ParseDuration(*intervalFlag)
	if err != nil {
		return fmt.Errorf("parse autosync interval: %w", err)
	}
	if interval < time.Minute {
		return fmt.Errorf("autosync interval must be at least 1m")
	}
	root, err := resolveAutosyncRoot(*rootFlag)
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
		return err
	}
	if err := writeAutosyncManagedFile(managedPaths.PlistPath, plist); err != nil {
		return err
	}
	if autosyncLoaded(managedPaths.Label) {
		if err := autosyncLaunchctl("bootout", autosyncServiceTarget(managedPaths.Label)); err != nil {
			return fmt.Errorf("unload existing auto-sync LaunchAgent: %w", err)
		}
	}
	if err := autosyncLaunchctl("bootstrap", autosyncDomainTarget(), managedPaths.PlistPath); err != nil {
		return fmt.Errorf("load auto-sync LaunchAgent: %w", err)
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
	if err := requireAutosyncPlatform(); err != nil {
		return err
	}
	rootFlag, err := parseAutosyncRootOnly("autosync disable", args, stderr)
	if err != nil {
		return err
	}
	root, err := resolveAutosyncRoot(rootFlag)
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
	if err := verifyAutosyncOwnership(managedPaths, false); err != nil {
		return err
	}
	if autosyncLoaded(managedPaths.Label) {
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
	if err := requireAutosyncPlatform(); err != nil {
		return err
	}
	rootFlag, err := parseAutosyncRootOnly("autosync status", args, stderr)
	if err != nil {
		return err
	}
	root, err := resolveAutosyncRoot(rootFlag)
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
	if err := verifyAutosyncOwnership(managedPaths, false); err != nil {
		return err
	}
	config, err := autosync.LoadConfig(managedPaths.ConfigPath)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Auto-sync:     enabled")
	fmt.Fprintf(stdout, "Loaded:        %s\n", yesNo(autosyncLoaded(managedPaths.Label)))
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
	rootFlag, err := parseAutosyncRootOnly("autosync run", args, stderr)
	if err != nil {
		return err
	}
	root, err := paths.BacklotRoot(rootFlag)
	if err != nil {
		return err
	}
	return runManagedAutosync(root)
}

func parseAutosyncRootOnly(name string, args []string, stderr io.Writer) (string, error) {
	fs := newFlagSet(name, stderr)
	rootFlag := fs.String("root", "", "Backlot root path")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	if fs.NArg() != 0 {
		return "", flag.ErrHelp
	}
	return *rootFlag, nil
}

func resolveAutosyncRoot(rootFlag string) (string, error) {
	root, err := paths.BacklotRoot(rootFlag)
	if err != nil {
		return "", err
	}
	if err := ensureRootOutsideCurrentProject(root); err != nil {
		return "", err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
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
	for _, path := range []string{managedPaths.ConfigPath, managedPaths.PlistPath} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && allowMissing && path == managedPaths.PlistPath {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("managed auto-sync file %s is not a regular file", path)
		}
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

func autosyncLoaded(label string) bool {
	return autosyncLaunchctl("print", autosyncServiceTarget(label)) == nil
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
