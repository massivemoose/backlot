package main

import (
	"os"

	"github.com/massivemoose/backlot/internal/commands"
)

func main() {
	os.Exit(commands.Run(os.Args[1:], os.Stdout, os.Stderr))
}
