package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

const rootReadme = `# Backlot state

Private Backlot workspace state.
`

func runInit(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("init", stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  backlot init [--root PATH] [--remote URL]")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Examples:")
		fmt.Fprintln(stderr, "  backlot init")
		fmt.Fprintln(stderr, "  backlot init --remote git@github.com:you/backlot-archive.git")
	}
	rootFlag := fs.String("root", "", "Backlot root path")
	remoteFlag := fs.String("remote", "", "origin remote URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}

	root, err := paths.BacklotRoot(*rootFlag)
	if err != nil {
		return err
	}
	if err := ensureRootOutsideCurrentProject(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if !gitutil.IsGitRepoRoot(root) {
		if _, err := gitutil.RunGit(root, "init"); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Initialized Backlot state repo at %s\n", root)
	} else {
		if !isBacklotArchiveRoot(root) {
			return fmt.Errorf("Backlot root %s is not a Backlot archive; move it aside or choose another root with --root", root)
		}
		fmt.Fprintf(stdout, "Backlot state repo already initialized at %s\n", root)
	}
	if err := ensureArchiveMarker(root); err != nil {
		return err
	}

	readmePath := filepath.Join(root, "README.md")
	if _, err := os.Stat(readmePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(readmePath, []byte(rootReadme), 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if *remoteFlag != "" {
		if origin, err := gitutil.OriginURL(root); err == nil {
			fmt.Fprintf(stdout, "origin already exists (%s); leaving it unchanged.\n", origin)
		} else if _, err := gitutil.RunGit(root, "remote", "add", "origin", *remoteFlag); err != nil {
			return err
		} else {
			fmt.Fprintf(stdout, "Added origin %s\n", *remoteFlag)
		}
	}
	return nil
}
