package main

import (
	"os"

	"github.com/massivemoose/backlot/internal/commands"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	build := commands.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}
	os.Exit(commands.RunWithBuildInfo(os.Args[1:], os.Stdout, os.Stderr, build))
}
