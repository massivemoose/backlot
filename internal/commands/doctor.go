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

func runDoctor(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("doctor", stderr)
	rootFlag := fs.String("root", "", "Backlot root path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}

	fmt.Fprintln(stdout, "Backlot doctor")
	fmt.Fprintln(stdout)

	if _, err := exec.LookPath("git"); err == nil {
		pass(stdout, "git found")
	} else {
		fail(stdout, "git found")
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
			fail(stdout, "inside Git repo")
		}
	} else {
		fail(stdout, "inside Git repo")
	}

	if rootErr == nil {
		if _, err := os.Stat(root); err == nil {
			pass(stdout, "Backlot root exists")
		} else {
			fail(stdout, "Backlot root exists")
		}
		if gitutil.IsGitRepoRoot(root) {
			pass(stdout, "Backlot root is a Git repo")
		} else {
			fail(stdout, "Backlot root is a Git repo")
		}
	} else {
		fail(stdout, "Backlot root exists")
		fail(stdout, "Backlot root is a Git repo")
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
			fail(stdout, ".backlot symlink exists")
		}
		if stateDir != "" && paths.LinkPointsTo(linkPath, stateDir) {
			pass(stdout, ".backlot points to expected target")
		} else {
			fail(stdout, ".backlot points to expected target")
		}
		if ok, err := paths.ExcludeContains(repoRoot, ".backlot"); err == nil && ok {
			pass(stdout, ".git/info/exclude contains .backlot/")
		} else {
			fail(stdout, ".git/info/exclude contains .backlot/")
		}
	} else {
		fail(stdout, ".backlot symlink exists")
		fail(stdout, ".backlot points to expected target")
		fail(stdout, ".git/info/exclude contains .backlot/")
	}
	return nil
}

func pass(w io.Writer, text string) {
	fmt.Fprintf(w, "✓ %s\n", text)
}

func fail(w io.Writer, text string) {
	fmt.Fprintf(w, "✗ %s\n", text)
}
