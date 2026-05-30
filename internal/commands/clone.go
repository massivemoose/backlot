package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

func runClone(args []string, stdout, stderr io.Writer) error {
	remote, rootFlag, err := parseCloneArgs(args, stderr)
	if err != nil {
		return err
	}

	root, err := paths.BacklotRoot(rootFlag)
	if err != nil {
		return err
	}
	if err := ensureCloneTarget(root); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return err
	}
	if err := runGitClone(remote, root); err != nil {
		return err
	}
	if !gitutil.IsGitRepoRoot(root) {
		return fmt.Errorf("cloned Backlot root %s is not a Git repository", root)
	}
	origin, err := gitutil.OriginURL(root)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Cloned Backlot archive")
	fmt.Fprintf(stdout, "Root:   %s\n", root)
	fmt.Fprintf(stdout, "Origin: %s\n", origin)
	return nil
}

func parseCloneArgs(args []string, stderr io.Writer) (string, string, error) {
	var archiveURL string
	var rootFlag string

	fs := newFlagSet("clone", stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  backlot clone <archive-url>")
		fmt.Fprintln(stderr)
		fmt.Fprintln(stderr, "Example:")
		fmt.Fprintln(stderr, "  backlot clone git@github.com:you/backlot-archive.git")
	}
	fs.StringVar(&rootFlag, "root", "", "Backlot root path")

	// Pre-process args to allow positional <archive-url> before or after flags.
	// Go's flag package stops at the first non-flag argument.
	// We want to extract one non-flag argument as the archiveURL.
	var remainingArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") && archiveURL == "" {
			archiveURL = arg
		} else {
			remainingArgs = append(remainingArgs, arg)
			if (arg == "--root" || arg == "-root") && i+1 < len(args) {
				remainingArgs = append(remainingArgs, args[i+1])
				i++
			}
		}
	}

	if err := fs.Parse(remainingArgs); err != nil {
		return "", "", err
	}

	if fs.NArg() > 0 {
		fs.Usage()
		return "", "", fmt.Errorf("error: too many arguments")
	}

	if archiveURL == "" {
		fs.Usage()
		return "", "", fmt.Errorf("error: missing archive URL")
	}

	return archiveURL, rootFlag, nil
}

func ensureCloneTarget(root string) error {
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("Backlot root %s already exists and is not a directory.\nMove it aside or choose another root with --root.", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("Backlot root %s already exists and is not empty.\nMove it aside or choose another root with --root.", root)
	}
	return nil
}

func runGitClone(remote, root string) error {
	_, err := gitutil.RunGit("", "clone", remote, root)
	return err
}
