package commands

import (
	"flag"
	"fmt"
	"io"
)

func runVersion(args []string, stdout, stderr io.Writer, build BuildInfo) error {
	fs := newFlagSet("version", stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}

	fmt.Fprintf(stdout, "backlot %s\n", build.Version)
	fmt.Fprintf(stdout, "commit: %s\n", build.Commit)
	fmt.Fprintf(stdout, "date: %s\n", build.Date)
	return nil
}
