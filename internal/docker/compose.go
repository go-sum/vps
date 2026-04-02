package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ComposeOpts holds the common flags for docker compose commands.
type ComposeOpts struct {
	ProjectDir  string
	ProjectName string
	EnvFile     string
	Env         map[string]string // extra env vars injected into the subprocess (e.g. from .versions)
}

// composeBase returns the base docker compose command with project flags.
func (o ComposeOpts) composeBase() []string {
	args := []string{"compose"}
	if o.ProjectDir != "" {
		args = append(args, "--project-directory", o.ProjectDir)
	}
	if o.ProjectName != "" {
		args = append(args, "-p", o.ProjectName)
	}
	if o.EnvFile != "" {
		args = append(args, "--env-file", o.EnvFile)
	}
	return args
}

// ComposeUp starts services. If wait is true, blocks until healthy.
func ComposeUp(ctx context.Context, opts ComposeOpts, services []string, wait bool) error {
	args := opts.composeBase()
	args = append(args, "up", "-d")
	if wait {
		args = append(args, "--wait")
	}
	args = append(args, services...)
	return composeRun(ctx, args, opts.Env)
}

// ComposeRecreate force-recreates specific services without pulling deps.
func ComposeRecreate(ctx context.Context, opts ComposeOpts, services []string) error {
	args := opts.composeBase()
	args = append(args, "up", "-d", "--force-recreate", "--no-deps")
	args = append(args, services...)
	return composeRun(ctx, args, opts.Env)
}

// ComposeDown stops and removes all services.
func ComposeDown(ctx context.Context, opts ComposeOpts) error {
	args := opts.composeBase()
	args = append(args, "down")
	return composeRun(ctx, args, opts.Env)
}

// ComposeStop stops specific services.
func ComposeStop(ctx context.Context, opts ComposeOpts, services []string) error {
	args := opts.composeBase()
	args = append(args, "stop")
	args = append(args, services...)
	return composeRun(ctx, args, opts.Env)
}

// ComposeBuild builds specific services.
func ComposeBuild(ctx context.Context, opts ComposeOpts, services []string) error {
	args := opts.composeBase()
	args = append(args, "build")
	args = append(args, services...)
	return composeRun(ctx, args, opts.Env)
}

// ComposeLogs prints recent logs for a service.
func ComposeLogs(ctx context.Context, opts ComposeOpts, service string, tail int) error {
	args := opts.composeBase()
	args = append(args, "logs", fmt.Sprintf("--tail=%d", tail), service)
	return composeRun(ctx, args, opts.Env)
}

// ComposePS returns the output of docker compose ps for a service.
func ComposePS(ctx context.Context, opts ComposeOpts, services []string) (string, error) {
	args := opts.composeBase()
	args = append(args, "ps")
	args = append(args, services...)
	return composeOutput(ctx, args, opts.Env)
}

// ComposeIsRunning checks if a service has a running container.
func ComposeIsRunning(ctx context.Context, opts ComposeOpts, service string) (bool, error) {
	args := opts.composeBase()
	args = append(args, "ps", "--status", "running", "-q", service)

	out, err := composeOutput(ctx, args, opts.Env)
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(out) != "", nil
}

func composeRun(ctx context.Context, args []string, env map[string]string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = mergeEnv(env)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %w", strings.Join(args[:2], " "), err)
	}
	return nil
}

func composeOutput(ctx context.Context, args []string, env map[string]string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = mergeEnv(env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", strings.Join(args[:2], " "), err, stderr.String())
	}
	return stdout.String(), nil
}

// mergeEnv returns os.Environ() with extra vars appended.
func mergeEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}
