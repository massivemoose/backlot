package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/massivemoose/chomp"
)

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
	router := backlotRouter(stdout, stderr, build)
	if err := router.Run(context.Background(), args); err != nil {
		if command, ok := chomp.UsageCommand(err); ok {
			command.Usage(stdout)
			return 0
		}
		if errors.Is(err, chomp.ErrUsage) || errors.Is(err, chomp.ErrHelp) {
			router.Usage(stdout)
			return 0
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

type runnableCommand struct {
	name    string
	summary string
	hidden  bool
	run     func([]string) error
	usage   func(io.Writer)
}

func (command runnableCommand) Name() string { return command.name }

func (command runnableCommand) Summary() string { return command.summary }

func (command runnableCommand) Hidden() bool { return command.hidden }

func (command runnableCommand) Run(_ context.Context, args []string) error {
	return command.run(args)
}

func (command runnableCommand) Usage(w io.Writer) {
	command.usage(w)
}

func backlotRouter(stdout, stderr io.Writer, build BuildInfo) *chomp.Router {
	return chomp.NewRouter(
		"backlot",
		"Backlot private workspace manager.",
		agentsRouter(stdout, stderr),
		runnableCommand{
			name:    "attach",
			summary: "Attach Backlot to the current Git repository",
			run:     func(args []string) error { return runAttach(args, stdout, stderr) },
			usage:   printAttachUsage,
		},
		autosyncRouter(stdout, stderr),
		runnableCommand{
			name:    "clone",
			summary: "Clone a Backlot archive",
			run:     func(args []string) error { return runClone(args, stdout, stderr) },
			usage:   printCloneUsage,
		},
		runnableCommand{
			name:    "detach",
			summary: "Detach Backlot from the current Git repository",
			run:     func(args []string) error { return runDetach(args, stdout, stderr) },
			usage:   printDetachUsage,
		},
		runnableCommand{
			name:    "doctor",
			summary: "Check Backlot setup",
			run:     func(args []string) error { return runDoctor(args, stdout, stderr) },
			usage:   printDoctorUsage,
		},
		runnableCommand{
			name:    "decrypt",
			summary: "Decrypt a Backlot archive blob",
			hidden:  true,
			run:     func(args []string) error { return runDecrypt(args, os.Stdin, stdout) },
			usage:   printDecryptUsage,
		},
		runnableCommand{
			name:    "encrypt",
			summary: "Encrypt a Backlot archive blob",
			hidden:  true,
			run:     func(args []string) error { return runEncrypt(args, os.Stdin, stdout) },
			usage:   printEncryptUsage,
		},
		runnableCommand{
			name:    "init",
			summary: "Initialize a Backlot archive",
			run:     func(args []string) error { return runInit(args, stdout, stderr) },
			usage:   printInitUsage,
		},
		runnableCommand{
			name:    "lock",
			summary: "Encrypt the Backlot archive",
			run:     func(args []string) error { return runLock(args, stdout, stderr) },
			usage:   printLockUsage,
		},
		runnableCommand{
			name:    "protect",
			summary: "Install a pre-commit private-state guard",
			run:     func(args []string) error { return runProtect(args, stdout, stderr) },
			usage:   printProtectUsage,
		},
		starterRouter(stdout, stderr),
		runnableCommand{
			name:    "status",
			summary: "Show Backlot status",
			run:     func(args []string) error { return runStatus(args, stdout, stderr) },
			usage:   printStatusUsage,
		},
		runnableCommand{
			name:    "sync",
			summary: "Sync the Backlot archive",
			run:     func(args []string) error { return runSync(args, stdout, stderr) },
			usage:   printSyncUsage,
		},
		runnableCommand{
			name:    "unlock",
			summary: "Unlock an encrypted Backlot archive",
			run:     func(args []string) error { return runUnlock(args, stdout, stderr) },
			usage:   printUnlockUsage,
		},
		runnableCommand{
			name:    "version",
			summary: "Show Backlot version",
			run:     func(args []string) error { return runVersion(args, stdout, stderr, build) },
			usage:   printVersionUsage,
		},
	)
}

func printSpecUsage(w io.Writer, spec *chomp.Spec) {
	fmt.Fprint(w, spec.Usage())
}

func cwd() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return dir, nil
}
