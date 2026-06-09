package commands

import (
	"fmt"
	"io"
	"time"

	"github.com/massivemoose/backlot/internal/agents"
	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

func agentsRouter(stdout, stderr io.Writer) *chomp.Router {
	return chomp.NewRouter(
		"agents",
		"Manage agent tool setup.",
		runnableCommand{
			name:    "setup",
			summary: "Show or apply agent configuration",
			run:     func(args []string) error { return runAgentsSetup(args, stdout, stderr) },
			usage:   printAgentsSetupUsage,
		},
	)
}

func runAgentsSetup(args []string, stdout, stderr io.Writer) error {
	result, err := agentsSetupSpec().Parse(args)
	if err != nil {
		return err
	}
	rootFlag := result.String("root")
	toolFlag := result.String("tool")
	applyFlag := result.Bool("apply")
	if applyFlag && toolFlag == "" {
		return fmt.Errorf("--apply requires --tool")
	}

	root, err := paths.BacklotRoot(rootFlag)
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

	selected, err := selectedAgents(toolFlag)
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Backlot agent setup")
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Backlot root: %s\n", root)
	fmt.Fprintln(stdout)
	if applyFlag {
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
		if toolFlag == "" {
			fmt.Fprintf(stdout, "Apply: backlot agents setup --tool %s --apply\n", agent.ID())
			continue
		}
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, agent.PersistentInstructions(root))
	}
	return nil
}

func agentsSetupSpec() *chomp.Spec {
	return chomp.New("backlot", "agents", "setup").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		String("tool", chomp.ValueName("codex|claude"), chomp.Description("agent tool")).
		Bool("apply", chomp.Description("apply persistent config changes")).
		Positionals(0, 0)
}

func printAgentsSetupUsage(w io.Writer) {
	printSpecUsage(w, agentsSetupSpec())
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
