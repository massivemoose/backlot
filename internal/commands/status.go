package commands

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

func runStatus(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("status", stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  backlot status [--root PATH]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Example:")
		fmt.Fprintln(stderr, "  backlot status")
	}
	rootFlag := fs.String("root", "", "Backlot root path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}

	info, err := collectProjectInfo(*rootFlag)
	if err != nil {
		return err
	}

	excluded := "no"
	if info.Excluded {
		excluded = "yes"
	}

	fmt.Fprintln(stdout, "Backlot")
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Public repo:   %s\n", info.RepoRoot)
	fmt.Fprintf(stdout, "Origin:        %s\n", info.Origin)
	fmt.Fprintf(stdout, "Project key:   %s\n", info.ProjectKey)
	fmt.Fprintf(stdout, "State dir:     %s\n", info.StateDir)
	fmt.Fprintf(stdout, "Link:          %s\n", info.LinkDescription)
	fmt.Fprintf(stdout, "Excluded:      %s\n", excluded)
	fmt.Fprintf(stdout, "State repo:    %s\n", info.StateRepo)
	if info.Recovery != "" {
		fmt.Fprintf(stdout, "Recovery:      %s\n", info.Recovery)
	}
	if info.Autosync != "" {
		fmt.Fprintf(stdout, "Auto-sync:     %s\n", info.Autosync)
	}
	if info.AutosyncRecovery != "" {
		fmt.Fprintf(stdout, "Auto recovery: %s\n", info.AutosyncRecovery)
	}
	return nil
}

type projectInfo struct {
	RootFlag         string
	BacklotRoot      string
	RepoRoot         string
	Origin           string
	ProjectKey       string
	StateDir         string
	LinkDescription  string
	Excluded         bool
	StateRepo        string
	Recovery         string
	Autosync         string
	AutosyncRecovery string
}

func collectProjectInfo(rootFlag string) (projectInfo, error) {
	var info projectInfo
	var err error
	info.RootFlag = rootFlag
	info.BacklotRoot, err = paths.BacklotRoot(rootFlag)
	if err != nil {
		return info, err
	}
	current, err := cwd()
	if err != nil {
		return info, err
	}
	info.RepoRoot, err = gitutil.RepoRoot(current)
	if err != nil {
		return info, err
	}
	if err := ensureRootOutsideProject(info.BacklotRoot, info.RepoRoot); err != nil {
		return info, err
	}
	if err := requireBacklotArchiveRoot(info.BacklotRoot); err != nil {
		return info, err
	}
	info.Origin, err = gitutil.OriginURL(info.RepoRoot)
	if err != nil {
		return info, err
	}
	info.ProjectKey, err = gitutil.NormalizeOrigin(info.Origin)
	if err != nil {
		return info, err
	}
	info.StateDir = paths.ProjectStateDir(info.BacklotRoot, info.ProjectKey)
	info.LinkDescription = paths.LinkDescription(filepath.Join(info.RepoRoot, ".backlot"), info.StateDir)
	info.Excluded, _ = paths.ExcludeContains(info.RepoRoot, ".backlot")
	info.StateRepo = stateRepoStatus(info.BacklotRoot)
	if info.StateRepo == "sync interrupted" {
		info.Recovery = syncRecoverySummary()
	}
	if health, err := collectAutosyncHealth(info.BacklotRoot); err != nil {
		info.Autosync = "error: " + err.Error()
	} else if health.Enabled {
		info.Autosync = health.Summary
		info.AutosyncRecovery = health.Recovery
	}
	return info, nil
}

func stateRepoStatus(root string) string {
	if _, err := os.Stat(root); err != nil {
		return "missing"
	}
	if !gitutil.IsGitRepoRoot(root) {
		return "not initialized"
	}
	state, err := detectSyncState(root)
	if err != nil {
		return "error"
	}
	if state.Interrupted() {
		return "sync interrupted"
	}
	status, err := gitutil.RunGit(root, "status", "--short")
	if err != nil {
		return "error"
	}
	if strings.TrimSpace(status) == "" {
		return "clean"
	}
	return "dirty"
}

func syncRecoverySummary() string {
	return "resolve conflicts in .backlot/ and run backlot sync --continue"
}
