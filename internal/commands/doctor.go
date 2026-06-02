package commands

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

const agentSetupDocsURL = "https://github.com/massivemoose/backlot/blob/main/docs/agents.md"

func runDoctor(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("doctor", stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  backlot doctor [--root PATH]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Example:")
		fmt.Fprintln(stderr, "  backlot doctor")
	}
	rootFlag := fs.String("root", "", "Backlot root path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}

	fmt.Fprintln(stdout, "Backlot doctor")
	fmt.Fprintln(stdout)

	failures := 0
	failCheck := func(text string) {
		failures++
		fail(stdout, text)
	}

	if _, err := exec.LookPath("git"); err == nil {
		pass(stdout, "git found")
	} else {
		failCheck("git found")
	}

	root, rootErr := paths.BacklotRoot(*rootFlag)
	current, cwdErr := cwd()
	repoRoot := ""
	origin := ""
	projectKey := ""
	stateDir := ""
	if cwdErr == nil {
		if foundRoot, err := gitutil.RepoRoot(current); err == nil {
			repoRoot = foundRoot
			pass(stdout, "inside Git repo")
			if foundOrigin, err := gitutil.OriginURL(repoRoot); err == nil {
				origin = foundOrigin
				if key, err := gitutil.NormalizeOrigin(origin); err == nil {
					projectKey = key
				}
			}
		} else {
			failCheck("inside Git repo")
		}
	} else {
		failCheck("inside Git repo")
	}

	if rootErr == nil {
		if _, err := os.Stat(root); err == nil {
			pass(stdout, "Backlot root exists")
		} else {
			failCheck("Backlot root exists")
		}
		if gitutil.IsGitRepoRoot(root) {
			pass(stdout, "Backlot root is a Git repo")
			if origin, err := gitutil.OriginURL(root); err == nil {
				info(stdout, fmt.Sprintf("Backlot archive origin: %s", origin))
			} else {
				info(stdout, "Backlot archive origin: local-only (no origin)")
			}
		} else {
			failCheck("Backlot root is a Git repo")
		}
		if isBacklotArchiveRoot(root) {
			pass(stdout, "Backlot root is a Backlot archive")
		} else {
			failCheck("Backlot root is a Backlot archive")
		}
		if gitutil.IsGitRepoRoot(root) {
			if state, err := detectSyncState(root); err == nil && state.Interrupted() {
				failCheck("Backlot sync was interrupted by a conflict")
				fmt.Fprintf(stdout, "  Recovery: %s or backlot sync --abort\n", syncRecoverySummary())
			}
		}
	} else {
		failCheck("Backlot root exists")
		failCheck("Backlot root is a Git repo")
		failCheck("Backlot root is a Backlot archive")
	}
	if rootErr == nil {
		info(stdout, fmt.Sprintf("Backlot root: %s", root))
		info(stdout, fmt.Sprintf("Agent setup: see %s or run backlot agents setup", agentSetupDocsURL))
	}

	if root != "" && projectKey != "" {
		stateDir = paths.ProjectStateDir(root, projectKey)
	}
	linkPath := ""
	if repoRoot != "" {
		linkPath = filepath.Join(repoRoot, ".backlot")
	}
	if linkPath != "" {
		if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
			pass(stdout, ".backlot symlink exists")
		} else {
			failCheck(".backlot symlink exists")
		}
		if stateDir != "" && paths.LinkPointsTo(linkPath, stateDir) {
			pass(stdout, ".backlot points to expected target")
		} else {
			failCheck(".backlot points to expected target")
		}
		if ok, err := paths.ExcludeContains(repoRoot, ".backlot"); err == nil && ok {
			pass(stdout, ".git/info/exclude ignores .backlot")
		} else {
			failCheck(".git/info/exclude ignores .backlot")
		}
	} else {
		failCheck(".backlot symlink exists")
		failCheck(".backlot points to expected target")
		failCheck(".git/info/exclude ignores .backlot")
	}
	if failures > 0 {
		return fmt.Errorf("doctor found %d problem(s)", failures)
	}
	return nil
}

func pass(w io.Writer, text string) {
	fmt.Fprintf(w, "✓ %s\n", text)
}

func fail(w io.Writer, text string) {
	fmt.Fprintf(w, "✗ %s\n", text)
}

func info(w io.Writer, text string) {
	fmt.Fprintf(w, "• %s\n", text)
}
