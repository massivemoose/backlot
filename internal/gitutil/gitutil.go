package gitutil

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func RunGit(dir string, args ...string) (string, error) {
	allArgs := []string{"-c", "core.fsmonitor=false"}
	if dir != "" {
		allArgs = append(allArgs, "-C", dir)
	}
	allArgs = append(allArgs, args...)
	cmd := exec.Command("git", allArgs...)
	cmd.Env = sanitizedGitEnv(os.Environ())
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(allArgs, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(allArgs, " "), err, text)
	}
	return text, nil
}

func sanitizedGitEnv(env []string) []string {
	blocked := map[string]bool{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_COMMON_DIR":                   true,
		"GIT_DIR":                          true,
		"GIT_INDEX_FILE":                   true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_PREFIX":                       true,
		"GIT_QUARANTINE_PATH":              true,
		"GIT_WORK_TREE":                    true,
	}
	cleaned := make([]string, 0, len(env))
	for _, item := range env {
		key, _, ok := strings.Cut(item, "=")
		if ok && blocked[key] {
			continue
		}
		cleaned = append(cleaned, item)
	}
	return cleaned
}

func RepoRoot(cwd string) (string, error) {
	root, err := RunGit(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("current directory is not inside a Git repo")
	}
	return root, nil
}

func OriginURL(repoRoot string) (string, error) {
	origin, err := RunGit(repoRoot, "remote", "get-url", "origin")
	if err != nil {
		return "", errors.New("repo does not have an origin remote")
	}
	return origin, nil
}

func IsGitRepo(dir string) bool {
	_, err := RunGit(dir, "rev-parse", "--git-dir")
	return err == nil
}

func IsGitRepoRoot(dir string) bool {
	root, err := RunGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return false
	}
	absDir, err := canonicalPath(dir)
	if err != nil {
		return false
	}
	absRoot, err := canonicalPath(root)
	if err != nil {
		return false
	}
	return absDir == absRoot
}

func GitPath(repoRoot string, path string) (string, error) {
	gitPath, err := RunGit(repoRoot, "rev-parse", "--git-path", path)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(gitPath) {
		return filepath.Clean(gitPath), nil
	}
	return filepath.Clean(filepath.Join(repoRoot, gitPath)), nil
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func HasOrigin(dir string) bool {
	_, err := OriginURL(dir)
	return err == nil
}

func NormalizeOrigin(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", errors.New("empty remote")
	}

	if strings.Contains(remote, "://") {
		return normalizeURLRemote(remote)
	}

	if host, path, ok := splitSCPRemote(remote); ok {
		return normalizeHostPath(host, path)
	}

	return "", fmt.Errorf("unsupported remote URL %q", remote)
}

func normalizeURLRemote(remote string) (string, error) {
	parsed, err := url.Parse(remote)
	if err != nil {
		return "", fmt.Errorf("parse remote URL: %w", err)
	}
	if parsed.Host == "" || parsed.Path == "" {
		return "", fmt.Errorf("unsupported remote URL %q", remote)
	}
	switch parsed.Scheme {
	case "https", "http", "ssh", "git":
	default:
		return "", fmt.Errorf("unsupported remote scheme %q", parsed.Scheme)
	}
	return normalizeHostPath(parsed.Hostname(), parsed.Path)
}

func splitSCPRemote(remote string) (string, string, bool) {
	colon := strings.Index(remote, ":")
	if colon <= 0 {
		return "", "", false
	}
	prefix := remote[:colon]
	path := remote[colon+1:]
	if strings.Contains(prefix, "/") || path == "" {
		return "", "", false
	}
	host := prefix
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	if host == "" {
		return "", "", false
	}
	return host, path, true
}

func normalizeHostPath(host, path string) (string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" || !strings.Contains(path, "/") {
		return "", fmt.Errorf("remote does not include host and owner/repo path")
	}
	for _, segment := range strings.Split(path, "/") {
		if err := validateRemotePathSegment(segment); err != nil {
			return "", err
		}
	}
	return host + "/" + path, nil
}

func validateRemotePathSegment(segment string) error {
	if segment == "" || segment == "." || segment == ".." {
		return fmt.Errorf("remote contains unsafe path segment %q", segment)
	}
	if strings.ContainsAny(segment, `\:*?"<>|`) {
		return fmt.Errorf("remote contains unsafe path segment %q", segment)
	}
	for _, r := range segment {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("remote contains unsafe path segment %q", segment)
		}
	}
	return nil
}
