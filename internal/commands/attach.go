package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

var builtInStarterFiles = map[string]string{
	"handoff.md": "# Handoff\n\nEnd-of-task handoff for the next session.\n",
	"state.md":   "# State\n\nCompact current project state for agents.\n",
	"roadmap.md": "# Roadmap\n\nPrivate roadmap and task list.\n",
	"files.md":   "# Files\n\nMap of important files and directories.\n",
}

func runAttach(args []string, stdout, stderr io.Writer) error {
	result, err := attachSpec().Parse(args)
	if err != nil {
		return err
	}
	linkName := result.String("link-name")
	if err := paths.ValidateLinkName(linkName); err != nil {
		return err
	}

	root, err := paths.BacklotRoot(result.String("root"))
	if err != nil {
		return err
	}

	current, err := cwd()
	if err != nil {
		return err
	}
	repoRoot, err := gitutil.RepoRoot(current)
	if err != nil {
		return err
	}
	if err := ensureRootOutsideProject(root, repoRoot); err != nil {
		return err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
		return err
	}
	origin, err := gitutil.OriginURL(repoRoot)
	if err != nil {
		return err
	}
	key, err := gitutil.NormalizeOrigin(origin)
	if err != nil {
		return err
	}

	stateDir := paths.ProjectStateDir(root, key)
	starter, err := ensureStarterState(root, stateDir)
	if err != nil {
		return err
	}
	if err := ensureProjectMarker(stateDir); err != nil {
		return err
	}
	if err := paths.EnsureManagedSymlink(filepath.Join(repoRoot, linkName), stateDir); err != nil {
		return err
	}
	if err := paths.EnsureExclude(repoRoot, linkName); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Attached Backlot\n")
	fmt.Fprintf(stdout, "Project key: %s\n", key)
	fmt.Fprintf(stdout, "State dir:   %s\n", stateDir)
	fmt.Fprintf(stdout, "Link:        %s -> %s\n", linkName, stateDir)
	fmt.Fprintf(stdout, "Starter:     %s\n", starter)
	return nil
}

func attachSpec() *chomp.Spec {
	return chomp.New("backlot", "attach").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		String("link-name", chomp.ValueName("name"), chomp.Default(".backlot"), chomp.Description("link name")).
		Positionals(0, 0)
}

func printAttachUsage(w io.Writer) {
	printSpecUsage(w, attachSpec())
}

// ensureStarterState adds starter content only when Backlot creates this
// project's private folder for the first time. It copies root/.starter when
// present, falls back to built-in starters otherwise, and leaves existing
// project folders untouched so attach never reimposes a layout the user changed.
func ensureStarterState(root, stateDir string) (string, error) {
	info, err := os.Stat(stateDir)
	if err == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("Backlot state path %s exists and is not a directory", stateDir)
		}
		return "existing archive (contents unchanged)", nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	starterDir := filepath.Join(root, ".starter")
	starterInfo, err := os.Stat(starterDir)
	if err == nil {
		if !starterInfo.IsDir() {
			return "", fmt.Errorf("Backlot starter template %s exists and is not a directory", starterDir)
		}
		if err := validateStarterTemplate(starterDir); err != nil {
			return "", err
		}
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			return "", err
		}
		if err := copyStarterTemplate(starterDir, stateDir); err != nil {
			return "", err
		}
		return starterDir, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", err
	}
	for name, content := range builtInStarterFiles {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	if err := os.Mkdir(filepath.Join(stateDir, "plans"), 0o755); err != nil {
		return "", err
	}
	return "built-in defaults", nil
}

func validateStarterTemplate(starterDir string) error {
	return filepath.WalkDir(starterDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == starterDir {
			return nil
		}
		rel, err := filepath.Rel(starterDir, path)
		if err != nil {
			return err
		}
		if rel == projectMarkerName {
			return fmt.Errorf(".starter cannot contain %s; Backlot manages this marker", projectMarkerName)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsDir() || info.Mode().IsRegular() {
			return nil
		}
		return fmt.Errorf(".starter contains unsupported entry %s", path)
	})
}

func copyStarterTemplate(starterDir, stateDir string) error {
	return filepath.WalkDir(starterDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == starterDir {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(starterDir, path)
		if err != nil {
			return err
		}
		if rel == projectMarkerName {
			return fmt.Errorf(".starter cannot contain %s; Backlot manages this marker", projectMarkerName)
		}
		dst := filepath.Join(stateDir, rel)
		if info.IsDir() {
			return os.Mkdir(dst, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf(".starter contains unsupported entry %s", path)
		}
		return copyStarterFile(path, dst, info.Mode().Perm())
	})
}

func copyStarterFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Chmod(mode)
}
