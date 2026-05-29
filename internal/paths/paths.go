package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/massivemoose/backlot/internal/gitutil"
)

func BacklotRoot(flagValue string) (string, error) {
	value := strings.TrimSpace(flagValue)
	if value == "" {
		value = strings.TrimSpace(os.Getenv("BACKLOT_ROOT"))
	}
	if value == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		value = filepath.Join(home, ".backlot")
	}
	return cleanUserPath(value)
}

func ProjectStateDir(root, key string) string {
	return filepath.Join(root, filepath.FromSlash(key))
}

func ValidateLinkName(linkName string) error {
	if linkName == "" {
		return errors.New("link name cannot be empty")
	}
	if filepath.IsAbs(linkName) {
		return errors.New("link name must be relative")
	}
	if linkName == "." || linkName == ".." {
		return errors.New("link name must be a single path segment")
	}
	if strings.ContainsAny(linkName, `/\`) {
		return errors.New("link name must be a single path segment")
	}
	return nil
}

func EnsureExclude(repoRoot string, linkName string) error {
	if err := ValidateLinkName(linkName); err != nil {
		return err
	}
	excludePath, err := gitutil.GitPath(repoRoot, "info/exclude")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(excludePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	text := string(data)
	entries := []string{linkName, linkName + "/"}
	if excludeHasEntry(text, entries[0]) && excludeHasEntry(text, entries[1]) {
		return nil
	}

	var builder strings.Builder
	builder.Write(data)
	if len(data) > 0 && !strings.HasSuffix(text, "\n") {
		builder.WriteByte('\n')
	}
	for _, entry := range entries {
		if excludeHasEntry(text, entry) {
			continue
		}
		builder.WriteString(entry)
		builder.WriteByte('\n')
	}
	return os.WriteFile(excludePath, []byte(builder.String()), 0o644)
}

func ExcludeContains(repoRoot string, linkName string) (bool, error) {
	if err := ValidateLinkName(linkName); err != nil {
		return false, err
	}
	excludePath, err := gitutil.GitPath(repoRoot, "info/exclude")
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(excludePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return excludeHasEntry(string(data), linkName), nil
}

func EnsureManagedSymlink(linkPath, target string) error {
	target, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	info, err := os.Lstat(linkPath)
	if errors.Is(err, os.ErrNotExist) {
		return os.Symlink(target, linkPath)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s already exists and is not managed by Backlot.\nMove it or choose another link name with --link-name.", filepath.Base(linkPath))
	}

	current, err := os.Readlink(linkPath)
	if err != nil {
		return err
	}
	currentAbs := current
	if !filepath.IsAbs(currentAbs) {
		currentAbs = filepath.Join(filepath.Dir(linkPath), currentAbs)
	}
	currentAbs = filepath.Clean(currentAbs)
	if currentAbs != filepath.Clean(target) {
		return fmt.Errorf("%s already exists and is not managed by Backlot.\nMove it or choose another link name with --link-name.", filepath.Base(linkPath))
	}
	return nil
}

func LinkDescription(linkPath, expectedTarget string) string {
	info, err := os.Lstat(linkPath)
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	if err != nil {
		return "error: " + err.Error()
	}
	name := filepath.Base(linkPath)
	if info.Mode()&os.ModeSymlink == 0 {
		return name + " exists but is not a symlink"
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		return "error: " + err.Error()
	}
	targetAbs := target
	if !filepath.IsAbs(targetAbs) {
		targetAbs = filepath.Join(filepath.Dir(linkPath), targetAbs)
	}
	targetAbs = filepath.Clean(targetAbs)
	expectedAbs := filepath.Clean(expectedTarget)
	if _, err := os.Stat(linkPath); err != nil {
		return fmt.Sprintf("%s -> %s (broken)", name, target)
	}
	if targetAbs != expectedAbs {
		return fmt.Sprintf("%s -> %s (wrong target)", name, target)
	}
	return fmt.Sprintf("%s -> %s", name, targetAbs)
}

func LinkPointsTo(linkPath, expectedTarget string) bool {
	info, err := os.Lstat(linkPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	target, err := os.Readlink(linkPath)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(linkPath), target)
	}
	return filepath.Clean(target) == filepath.Clean(expectedTarget)
}

func cleanUserPath(value string) (string, error) {
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if value == "~" {
			value = home
		} else {
			value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
		}
	}
	if !filepath.IsAbs(value) {
		abs, err := filepath.Abs(value)
		if err != nil {
			return "", err
		}
		value = abs
	}
	return filepath.Clean(value), nil
}

func excludeHasEntry(text, entry string) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == entry {
			return true
		}
	}
	return false
}
