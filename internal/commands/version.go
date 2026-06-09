package commands

import (
	"fmt"
	"io"

	"github.com/massivemoose/chomp"
)

func runVersion(args []string, stdout, stderr io.Writer, build BuildInfo) error {
	if _, err := versionSpec().Parse(args); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "backlot %s\n", build.Version)
	fmt.Fprintf(stdout, "commit: %s\n", build.Commit)
	fmt.Fprintf(stdout, "date: %s\n", build.Date)
	return nil
}

func versionSpec() *chomp.Spec {
	return chomp.New("backlot", "version").
		Positionals(0, 0)
}

func printVersionUsage(w io.Writer) {
	printSpecUsage(w, versionSpec())
}
