package repo

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Clone performs a shallow git clone of a repo at a specific branch into dest.
// Returns the HEAD commit SHA.
func Clone(ctx context.Context, repoURL, branch, dest string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", branch, repoURL, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git clone %s (branch %s): %w", repoURL, branch, err)
	}

	sha, err := revParse(ctx, dest)
	if err != nil {
		return "", err
	}
	return sha, nil
}

// revParse returns HEAD's full SHA for the repo at dir.
func revParse(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// ParseVersions reads a KEY=VALUE file (like .versions) and returns a map.
// Empty values and comments (#) are included as-is (empty string for blank values).
func ParseVersions(dir string) (map[string]string, error) {
	path := filepath.Join(dir, ".versions")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open .versions: %w", err)
	}
	defer f.Close()

	versions := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		versions[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read .versions: %w", err)
	}
	return versions, nil
}

// BuildArgsFromVersions converts a versions map into Docker --build-arg key=value pairs.
func BuildArgsFromVersions(versions map[string]string) map[string]string {
	args := make(map[string]string, len(versions))
	for k, v := range versions {
		if v != "" {
			args[k] = v
		}
	}
	return args
}
