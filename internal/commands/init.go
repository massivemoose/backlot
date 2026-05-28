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
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if !gitutil.IsGitRepoRoot(root) {
		if _, err := gitutil.RunGit(root, "init"); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Initialized Backlot state repo at %s\n", root)
	} else {
		fmt.Fprintf(stdout, "Backlot state repo already initialized at %s\n", root)
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
		if gitutil.HasOrigin(root) {
			fmt.Fprintln(stdout, "origin already exists; leaving it unchanged.")
		} else if _, err := gitutil.RunGit(root, "remote", "add", "origin", *remoteFlag); err != nil {
			return err
		} else {
			fmt.Fprintf(stdout, "Added origin %s\n", *remoteFlag)
		}
	}
	return nil
}
