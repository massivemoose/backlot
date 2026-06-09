package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

const rootReadme = `# Backlot state

Private Backlot workspace state.
`

func runInit(args []string, stdout, stderr io.Writer) error {
	result, err := initSpec().Parse(args)
	if err != nil {
		return err
	}

	root, err := paths.BacklotRoot(result.String("root"))
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

	if remoteFlag := result.String("remote"); remoteFlag != "" {
		if origin, err := gitutil.OriginURL(root); err == nil {
			fmt.Fprintf(stdout, "origin already exists (%s); leaving it unchanged.\n", origin)
		} else if _, err := gitutil.RunGit(root, "remote", "add", "origin", remoteFlag); err != nil {
			return err
		} else {
			fmt.Fprintf(stdout, "Added origin %s\n", remoteFlag)
		}
	}
	return nil
}

func initSpec() *chomp.Spec {
	return chomp.New("backlot", "init").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		String("remote", chomp.ValueName("url"), chomp.Description("origin remote URL")).
		Positionals(0, 0)
}

func printInitUsage(w io.Writer) {
	printSpecUsage(w, initSpec())
}
