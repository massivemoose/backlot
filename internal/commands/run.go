package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

type commandFunc func([]string, io.Writer, io.Writer) error

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

var defaultBuildInfo = BuildInfo{
	Version: "dev",
	Commit:  "unknown",
	Date:    "unknown",
}

func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithBuildInfo(args, stdout, stderr, defaultBuildInfo)
}

func RunWithBuildInfo(args []string, stdout, stderr io.Writer, build BuildInfo) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	if args[0] == "--help" || args[0] == "-h" {
		printUsage(stdout)
		return 0
	}

	commands := map[string]commandFunc{
		"init":    runInit,
		"clone":   runClone,
		"attach":  runAttach,
		"detach":  runDetach,
		"status":  runStatus,
		"sync":    runSync,
		"protect": runProtect,
		"doctor":  runDoctor,
		"version": func(args []string, stdout, stderr io.Writer) error {
			return runVersion(args, stdout, stderr, build)
		},
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
	if err := cmd(args[1:], stdout, stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
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
	fmt.Fprintln(w, "  clone    <archive-url> [--root PATH]")
	fmt.Fprintln(w, "  attach   [--root PATH]")
	fmt.Fprintln(w, "  detach   [--root PATH]")
	fmt.Fprintln(w, "  status   [--root PATH]")
	fmt.Fprintln(w, "  sync     [--root PATH] [-m MESSAGE|--continue|--abort]")
	fmt.Fprintln(w, "  protect")
	fmt.Fprintln(w, "  doctor   [--root PATH]")
	fmt.Fprintln(w, "  version")
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
