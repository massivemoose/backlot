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
	autosyncSystemctl  = runSystemctl
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
	scheduler, err := currentAutosyncScheduler()
	if err != nil {
		return err
	}
	interval := result.Duration("interval")
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
	loaded, err := scheduler.active(managedPaths)
	if err != nil {
		return fmt.Errorf("inspect existing auto-sync %s: %w", scheduler.managedFileNoun(), err)
	}
	previous, err := captureAutosyncManagedFiles(managedPaths, scheduler)
	if err != nil {
		return err
	}
	if loaded && !previous.hasSchedulerFiles() {
		return fmt.Errorf("loaded auto-sync %s %s has no managed files to update safely", scheduler.managedFileNoun(), managedPaths.Label)
	}
	config := autosyncConfigForScheduler(scheduler, managedPaths, binary, int(interval/time.Second))
	rendered, err := scheduler.renderFiles(managedPaths, config)
	if err != nil {
		return err
	}
	if err := autosync.WriteConfig(managedPaths.ConfigPath, config); err != nil {
		return rollbackAutosyncEnable(managedPaths, scheduler, previous, false, err)
	}
	for _, file := range rendered {
		if err := writeAutosyncManagedFile(file.path, file.data); err != nil {
			return rollbackAutosyncEnable(managedPaths, scheduler, previous, false, err)
		}
	}
	if loaded {
		if err := scheduler.unload(managedPaths); err != nil {
			return rollbackAutosyncEnable(managedPaths, scheduler, previous, false,
				fmt.Errorf("unload existing auto-sync %s: %w", scheduler.managedFileNoun(), err))
		}
	}
	if err := scheduler.load(managedPaths); err != nil {
		return rollbackAutosyncEnable(managedPaths, scheduler, previous, loaded,
			fmt.Errorf("load auto-sync %s: %w", scheduler.managedFileNoun(), err))
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

func autosyncConfigForScheduler(scheduler autosyncScheduler, managedPaths autosync.Paths, binary string, intervalSeconds int) autosync.Config {
	config := autosync.Config{
		SchemaVersion:   autosync.SchemaVersion,
		ManagedBy:       autosync.ManagedBy,
		Scheduler:       scheduler.kind(),
		Root:            managedPaths.Root,
		Binary:          binary,
		Label:           managedPaths.Label,
		IntervalSeconds: intervalSeconds,
	}
	switch scheduler.kind() {
	case autosync.SchedulerLaunchd:
		config.PlistPath = managedPaths.PlistPath
	case autosync.SchedulerSystemdUser:
		config.ServiceName = managedPaths.ServiceName
		config.TimerName = managedPaths.TimerName
		config.ServicePath = managedPaths.ServicePath
		config.TimerPath = managedPaths.TimerPath
	}
	return config
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
	scheduler, err := currentAutosyncScheduler()
	if err != nil {
		return err
	}
	configExists, err := autosyncManagedConfigExists(managedPaths)
	if err != nil {
		return err
	}
	if !configExists {
		if path, exists, err := firstExistingSchedulerFile(scheduler, managedPaths); err != nil {
			return err
		} else if exists {
			return fmt.Errorf("auto-sync %s %s is not managed by Backlot", scheduler.managedFileNoun(), path)
		}
		fmt.Fprintln(stdout, "Auto-sync is already disabled.")
		return nil
	}
	if err := verifyAutosyncOwnership(managedPaths, true); err != nil {
		return err
	}
	config, err := autosync.LoadConfig(managedPaths.ConfigPath)
	if err != nil {
		return err
	}
	scheduler, err = autosyncSchedulerForConfig(config)
	if err != nil {
		return err
	}
	loaded, err := scheduler.active(managedPaths)
	if err != nil {
		return fmt.Errorf("inspect auto-sync %s: %w", scheduler.managedFileNoun(), err)
	}
	shouldUnload := loaded
	if !shouldUnload && scheduler.kind() == autosync.SchedulerSystemdUser {
		if _, err := os.Lstat(managedPaths.TimerPath); err == nil {
			shouldUnload = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if shouldUnload {
		if err := scheduler.unload(managedPaths); err != nil {
			return fmt.Errorf("unload auto-sync %s: %w", scheduler.managedFileNoun(), err)
		}
	}
	for _, path := range scheduler.managedFilePaths(managedPaths) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if scheduler.kind() == autosync.SchedulerSystemdUser {
		if err := autosyncSystemctl("--user", "daemon-reload"); err != nil {
			return fmt.Errorf("reload auto-sync %s: %w", scheduler.managedFileNoun(), err)
		}
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
	scheduler, err := currentAutosyncScheduler()
	if err != nil {
		return err
	}
	configExists, err := autosyncManagedConfigExists(managedPaths)
	if err != nil {
		return err
	}
	if !configExists {
		if path, exists, err := firstExistingSchedulerFile(scheduler, managedPaths); err != nil {
			return err
		} else if exists {
			return fmt.Errorf("auto-sync %s %s is not managed by Backlot", scheduler.managedFileNoun(), path)
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
	scheduler, err = autosyncSchedulerForConfig(config)
	if err != nil {
		return err
	}
	loaded, err := scheduler.active(managedPaths)
	if err != nil {
		return fmt.Errorf("inspect auto-sync %s: %w", scheduler.managedFileNoun(), err)
	}
	fmt.Fprintln(stdout, "Auto-sync:     enabled")
	fmt.Fprintf(stdout, "Scheduler:     %s\n", scheduler.displayName())
	fmt.Fprintf(stdout, "%-15s%s\n", scheduler.activeLabel()+":", yesNo(loaded))
	if err := printSchedulerFileStatus(stdout, scheduler, managedPaths); err != nil {
		return err
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
		Duration("interval", chomp.Default(defaultAutosyncInterval.String()), chomp.Description("automatic sync interval")).
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
	managedPaths, err := autosync.ResolvePathsForPlatform(home, root, autosyncGOOS)
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

type autosyncRenderedFile struct {
	path string
	data []byte
}

type autosyncScheduler interface {
	kind() string
	displayName() string
	activeLabel() string
	managedFileNoun() string
	managedFilePaths(autosync.Paths) []string
	renderFiles(autosync.Paths, autosync.Config) ([]autosyncRenderedFile, error)
	active(autosync.Paths) (bool, error)
	load(autosync.Paths) error
	unload(autosync.Paths) error
	reloadPrevious(autosync.Paths) error
}

type launchdAutosyncScheduler struct{}

func (launchdAutosyncScheduler) kind() string { return autosync.SchedulerLaunchd }

func (launchdAutosyncScheduler) displayName() string { return "launchd LaunchAgent" }

func (launchdAutosyncScheduler) activeLabel() string { return "Loaded" }

func (launchdAutosyncScheduler) managedFileNoun() string { return "LaunchAgent" }

func (launchdAutosyncScheduler) managedFilePaths(paths autosync.Paths) []string {
	return []string{paths.PlistPath}
}

func (launchdAutosyncScheduler) renderFiles(paths autosync.Paths, config autosync.Config) ([]autosyncRenderedFile, error) {
	plist, err := autosync.RenderLaunchAgent(paths, config)
	if err != nil {
		return nil, err
	}
	return []autosyncRenderedFile{{path: paths.PlistPath, data: plist}}, nil
}

func (launchdAutosyncScheduler) active(paths autosync.Paths) (bool, error) {
	return autosyncLoaded(paths.Label)
}

func (launchdAutosyncScheduler) load(paths autosync.Paths) error {
	return autosyncLaunchctl("bootstrap", autosyncDomainTarget(), paths.PlistPath)
}

func (launchdAutosyncScheduler) unload(paths autosync.Paths) error {
	return autosyncLaunchctl("bootout", autosyncServiceTarget(paths.Label))
}

func (scheduler launchdAutosyncScheduler) reloadPrevious(paths autosync.Paths) error {
	return scheduler.load(paths)
}

type systemdAutosyncScheduler struct{}

func (systemdAutosyncScheduler) kind() string { return autosync.SchedulerSystemdUser }

func (systemdAutosyncScheduler) displayName() string { return "systemd user timer" }

func (systemdAutosyncScheduler) activeLabel() string { return "Active" }

func (systemdAutosyncScheduler) managedFileNoun() string { return "systemd unit" }

func (systemdAutosyncScheduler) managedFilePaths(paths autosync.Paths) []string {
	return []string{paths.ServicePath, paths.TimerPath}
}

func (systemdAutosyncScheduler) renderFiles(paths autosync.Paths, config autosync.Config) ([]autosyncRenderedFile, error) {
	service, err := autosync.RenderSystemdService(paths, config)
	if err != nil {
		return nil, err
	}
	timer, err := autosync.RenderSystemdTimer(paths, config)
	if err != nil {
		return nil, err
	}
	return []autosyncRenderedFile{
		{path: paths.ServicePath, data: service},
		{path: paths.TimerPath, data: timer},
	}, nil
}

func (systemdAutosyncScheduler) active(paths autosync.Paths) (bool, error) {
	err := autosyncSystemctl("--user", "is-active", "--quiet", paths.TimerName)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errAutosyncNotLoaded) {
		return false, nil
	}
	return false, err
}

func (systemdAutosyncScheduler) load(paths autosync.Paths) error {
	if err := autosyncSystemctl("--user", "daemon-reload"); err != nil {
		return err
	}
	return autosyncSystemctl("--user", "enable", "--now", paths.TimerName)
}

func (systemdAutosyncScheduler) unload(paths autosync.Paths) error {
	return autosyncSystemctl("--user", "disable", "--now", paths.TimerName)
}

func (scheduler systemdAutosyncScheduler) reloadPrevious(paths autosync.Paths) error {
	return scheduler.load(paths)
}

func currentAutosyncScheduler() (autosyncScheduler, error) {
	switch autosyncGOOS {
	case "darwin":
		return launchdAutosyncScheduler{}, nil
	case "linux":
		return systemdAutosyncScheduler{}, nil
	default:
		return nil, fmt.Errorf("auto-sync scheduler management is currently supported on macOS and Linux only")
	}
}

func autosyncSchedulerForConfig(config autosync.Config) (autosyncScheduler, error) {
	switch configScheduler(config) {
	case autosync.SchedulerLaunchd:
		return launchdAutosyncScheduler{}, nil
	case autosync.SchedulerSystemdUser:
		return systemdAutosyncScheduler{}, nil
	default:
		return nil, fmt.Errorf("unsupported autosync scheduler %q", config.Scheduler)
	}
}

func configScheduler(config autosync.Config) string {
	if config.Scheduler == "" {
		return autosync.SchedulerLaunchd
	}
	return config.Scheduler
}

func verifyAutosyncOwnership(managedPaths autosync.Paths, allowMissing bool) error {
	scheduler, err := currentAutosyncScheduler()
	if err != nil {
		return err
	}
	config, err := autosync.LoadConfig(managedPaths.ConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		if path, exists, fileErr := firstExistingSchedulerFile(scheduler, managedPaths); fileErr != nil {
			return fileErr
		} else if exists {
			return fmt.Errorf("auto-sync %s %s is not managed by Backlot", scheduler.managedFileNoun(), path)
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
	scheduler, err = autosyncSchedulerForConfig(config)
	if err != nil {
		return err
	}
	info, err := os.Lstat(managedPaths.ConfigPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("managed auto-sync file %s is not a regular file", managedPaths.ConfigPath)
	}
	expected, err := scheduler.renderFiles(managedPaths, config)
	if err != nil {
		return err
	}
	for _, file := range expected {
		info, err := os.Lstat(file.path)
		if errors.Is(err, os.ErrNotExist) && allowMissing {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("managed auto-sync file %s is not a regular file", file.path)
		}
		actual, err := os.ReadFile(file.path)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, file.data) {
			return fmt.Errorf("auto-sync %s %s is not managed by Backlot", scheduler.managedFileNoun(), file.path)
		}
	}
	return nil
}

func firstExistingSchedulerFile(scheduler autosyncScheduler, managedPaths autosync.Paths) (string, bool, error) {
	for _, path := range scheduler.managedFilePaths(managedPaths) {
		if path == "" {
			continue
		}
		if _, err := os.Lstat(path); err == nil {
			return path, true, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
	}
	return "", false, nil
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
	files []autosyncFileSnapshot
}

func captureAutosyncManagedFiles(managedPaths autosync.Paths, scheduler autosyncScheduler) (autosyncManagedSnapshot, error) {
	paths := append([]string{managedPaths.ConfigPath}, scheduler.managedFilePaths(managedPaths)...)
	snapshot := autosyncManagedSnapshot{files: make([]autosyncFileSnapshot, 0, len(paths))}
	for _, path := range paths {
		file, err := captureAutosyncFile(path)
		if err != nil {
			return autosyncManagedSnapshot{}, err
		}
		snapshot.files = append(snapshot.files, file)
	}
	return snapshot, nil
}

func (snapshot autosyncManagedSnapshot) hasSchedulerFiles() bool {
	for i, file := range snapshot.files {
		if i == 0 {
			continue
		}
		if file.exists {
			return true
		}
	}
	return false
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

func rollbackAutosyncEnable(managedPaths autosync.Paths, scheduler autosyncScheduler, previous autosyncManagedSnapshot, reloadPrevious bool, cause error) error {
	for _, file := range previous.files {
		if err := restoreAutosyncFile(file); err != nil {
			return fmt.Errorf("%w; restore previous auto-sync file %s: %v", cause, file.path, err)
		}
	}
	if reloadPrevious && previous.hasSchedulerFiles() {
		if err := scheduler.reloadPrevious(managedPaths); err != nil {
			return fmt.Errorf("%w; reload previous auto-sync %s: %v", cause, scheduler.managedFileNoun(), err)
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

func runSystemctl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if len(args) >= 4 && args[0] == "--user" && args[1] == "is-active" && args[2] == "--quiet" &&
			errors.As(err, &exitErr) && (exitErr.ExitCode() == 3 || exitErr.ExitCode() == 4) {
			return errAutosyncNotLoaded
		}
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func requireAutosyncPlatform() error {
	if autosyncGOOS != "darwin" && autosyncGOOS != "linux" {
		return fmt.Errorf("auto-sync scheduler management is currently supported on macOS and Linux only")
	}
	return nil
}

func printSchedulerFileStatus(stdout io.Writer, scheduler autosyncScheduler, managedPaths autosync.Paths) error {
	switch scheduler.kind() {
	case autosync.SchedulerLaunchd:
		if _, err := os.Stat(managedPaths.PlistPath); errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(stdout, "LaunchAgent:   missing")
		} else if err != nil {
			return err
		} else {
			fmt.Fprintln(stdout, "LaunchAgent:   present")
		}
	case autosync.SchedulerSystemdUser:
		if err := printFilePresence(stdout, "Service", managedPaths.ServicePath); err != nil {
			return err
		}
		if err := printFilePresence(stdout, "Timer", managedPaths.TimerPath); err != nil {
			return err
		}
	}
	return nil
}

func printFilePresence(stdout io.Writer, label, path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(stdout, "%-15s%s\n", label+":", "missing")
	} else if err != nil {
		return err
	} else {
		fmt.Fprintf(stdout, "%-15s%s\n", label+":", "present")
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
