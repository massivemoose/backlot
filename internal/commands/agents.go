package commands

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/massivemoose/backlot/internal/agents"
	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
)

func runAgents(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printAgentsUsage(stderr)
		return flag.ErrHelp
	}
	switch args[0] {
	case "setup":
		return runAgentsSetup(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown agents command %q", args[0])
	}
}

func runAgentsSetup(args []string, stdout, stderr io.Writer) error {
	fs := newFlagSet("agents setup", stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage:")
		fmt.Fprintln(stderr, "  backlot agents setup [--root PATH] [--tool codex|claude] [--apply]")
	}
	rootFlag := fs.String("root", "", "Backlot root path")
	toolFlag := fs.String("tool", "", "agent tool")
	applyFlag := fs.Bool("apply", false, "apply persistent config changes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return flag.ErrHelp
	}
	if *applyFlag && *toolFlag == "" {
		return fmt.Errorf("--apply requires --tool")
	}

	root, err := paths.BacklotRoot(*rootFlag)
	if err != nil {
		return err
	}
	repoRoot, err := currentRepoOrCWD()
	if err != nil {
		return err
	}
	env, err := agents.DefaultEnvironment()
	if err != nil {
		return err
	}

	selected, err := selectedAgents(*toolFlag)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Backlot agent setup")
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Backlot root: %s\n", root)
	fmt.Fprintln(stdout)
	if *applyFlag {
		result, err := selected[0].ApplyConfig(env, root, time.Now())
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s\n", selected[0].Name())
		fmt.Fprintf(stdout, "Config: %s\n", result.ConfigPath)
		if result.BackupPath != "" {
			fmt.Fprintf(stdout, "Backup: %s\n", result.BackupPath)
		}
		if result.Changed {
			fmt.Fprintln(stdout, "Updated: yes")
		} else {
			fmt.Fprintln(stdout, "Updated: no")
		}
		fmt.Fprintln(stdout, result.Message)
		return nil
	}

	for i, agent := range selected {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		status := agent.ConfigStatus(env, root)
		fmt.Fprintf(stdout, "%s (%s)\n", agent.Name(), agent.ID())
		fmt.Fprintf(stdout, "Config: %s\n", status.ConfigPath)
		if status.HasGrant {
			fmt.Fprintln(stdout, "Status: configured")
		} else if status.Exists {
			fmt.Fprintln(stdout, "Status: config found, Backlot root not configured")
		} else {
			fmt.Fprintln(stdout, "Status: config not found")
		}
		fmt.Fprintf(stdout, "One-session: %s\n", agent.OneSessionCommand(repoRoot, root))
		if *toolFlag == "" {
			fmt.Fprintf(stdout, "Apply: backlot agents setup --tool %s --apply\n", agent.ID())
			continue
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, agent.PersistentInstructions(root))
	}
	return nil
}

func printAgentsUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: backlot agents <command> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  setup    [--root PATH] [--tool codex|claude] [--apply]")
}

func selectedAgents(tool string) ([]agents.Agent, error) {
	if tool == "" {
		return agents.All(), nil
	}
	agent, ok := agents.ByID(tool)
	if !ok {
		return nil, fmt.Errorf("unsupported agent tool %q", tool)
	}
	return []agents.Agent{agent}, nil
}

func currentRepoOrCWD() (string, error) {
	current, err := cwd()
	if err != nil {
		return "", err
	}
	if repoRoot, err := gitutil.RepoRoot(current); err == nil {
		return repoRoot, nil
	}
	return current, nil
}
