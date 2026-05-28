package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

type commandFunc func([]string, io.Writer, io.Writer) error

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	commands := map[string]commandFunc{
		"init":    runInit,
		"attach":  runAttach,
		"status":  runStatus,
		"sync":    runSync,
		"protect": runProtect,
		"doctor":  runDoctor,
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
	if err := cmd(args[1:], stdout, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 2
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: backlot <command> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init     [--root PATH] [--remote URL]")
	fmt.Fprintln(w, "  attach   [--root PATH] [--link-name .backlot]")
	fmt.Fprintln(w, "  status   [--root PATH]")
	fmt.Fprintln(w, "  sync     [--root PATH] [-m MESSAGE]")
	fmt.Fprintln(w, "  protect")
	fmt.Fprintln(w, "  doctor   [--root PATH]")
}

func cwd() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return dir, nil
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}
