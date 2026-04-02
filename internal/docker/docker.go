package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// BuildOpts configures a docker build invocation.
type BuildOpts struct {
	ContextDir string
	Dockerfile string
	Target     string
	Tag        string
	BuildArgs  map[string]string
	Labels     map[string]string
	Secrets    []string // e.g. "id=github_token,env=GITHUB_ACCESS_TOKEN"
}

// RunOpts configures a docker run invocation.
type RunOpts struct {
	Image      string
	Cmd        []string
	Env        map[string]string
	Volumes    []string // "host:container" pairs
	WorkDir    string
	Network    string
	Remove     bool
}

// Build runs docker build with the given options.
func Build(ctx context.Context, opts BuildOpts) error {
	args := []string{"build"}

	if opts.Target != "" {
		args = append(args, "--target", opts.Target)
	}
	if opts.Dockerfile != "" {
		args = append(args, "--file", opts.Dockerfile)
	}
	if opts.Tag != "" {
		args = append(args, "--tag", opts.Tag)
	}
	for k, v := range opts.BuildArgs {
		args = append(args, "--build-arg", k+"="+v)
	}
	for k, v := range opts.Labels {
		args = append(args, "--label", k+"="+v)
	}
	for _, s := range opts.Secrets {
		args = append(args, "--secret", s)
	}
	args = append(args, opts.ContextDir)

	return run(ctx, args...)
}

// Tag tags a Docker image.
func Tag(ctx context.Context, src, dst string) error {
	return run(ctx, "tag", src, dst)
}

// InspectLabel returns the value of a label on a Docker image or container.
// Returns empty string if the image/label does not exist.
func InspectLabel(ctx context.Context, target, label string) (string, error) {
	format := fmt.Sprintf("{{index .Config.Labels %q}}", label)
	out, err := output(ctx, "inspect", "--format", format, target)
	if err != nil {
		return "", nil // image doesn't exist
	}
	return strings.TrimSpace(out), nil
}

// InspectID returns the image ID for the given image name.
// Returns empty string if the image does not exist.
func InspectID(ctx context.Context, image string) (string, error) {
	out, err := output(ctx, "inspect", "--format", "{{.Id}}", image)
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// Run executes docker run with the given options.
func Run(ctx context.Context, opts RunOpts) error {
	args := []string{"run"}

	if opts.Remove {
		args = append(args, "--rm")
	}
	if opts.WorkDir != "" {
		args = append(args, "-w", opts.WorkDir)
	}
	if opts.Network != "" {
		args = append(args, "--network", opts.Network)
	}
	for _, v := range opts.Volumes {
		args = append(args, "-v", v)
	}
	for k, v := range opts.Env {
		args = append(args, "-e", k+"="+v)
	}

	args = append(args, opts.Image)
	args = append(args, opts.Cmd...)

	return run(ctx, args...)
}

// Exec runs a command inside a running container.
func Exec(ctx context.Context, container string, cmd []string) error {
	args := append([]string{"exec", container}, cmd...)
	return run(ctx, args...)
}

// NetworkConnect connects a container to a network.
// Suppresses output — returns error only.
func NetworkConnect(ctx context.Context, network, container string) error {
	_, err := output(ctx, "network", "connect", network, container)
	return err
}

// ContainerStats holds resource usage for a container.
type ContainerStats struct {
	Name     string
	Status   string
	CPU      string
	Memory   string
	MemLimit string
	PIDs     string
	Uptime   string
	Image    string
}

// Stats returns resource usage for a running container.
// Returns zero-valued struct fields if the container is not running.
func Stats(ctx context.Context, container string) (ContainerStats, error) {
	// docker stats --no-stream gives a one-shot snapshot.
	format := "{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.PIDs}}"
	out, err := output(ctx, "stats", "--no-stream", "--format", format, container)
	if err != nil {
		return ContainerStats{Name: container}, nil
	}

	parts := strings.Split(strings.TrimSpace(out), "\t")
	cs := ContainerStats{Name: container}
	if len(parts) >= 4 {
		cs.CPU = parts[1]
		// MemUsage is "X / Y" — split into usage and limit.
		if mem, limit, ok := strings.Cut(parts[2], " / "); ok {
			cs.Memory = strings.TrimSpace(mem)
			cs.MemLimit = strings.TrimSpace(limit)
		} else {
			cs.Memory = parts[2]
		}
		cs.PIDs = parts[3]
	}

	// Get status and uptime from inspect.
	statusOut, err := output(ctx, "inspect", "--format",
		"{{.State.Status}}\t{{.State.StartedAt}}\t{{.Config.Image}}", container)
	if err == nil {
		sp := strings.Split(strings.TrimSpace(statusOut), "\t")
		if len(sp) >= 3 {
			cs.Status = sp[0]
			cs.Uptime = sp[1]
			cs.Image = sp[2]
		}
	}

	return cs, nil
}

// NetworkContainers returns the names of containers connected to a network.
func NetworkContainers(ctx context.Context, network string) ([]string, error) {
	out, err := output(ctx, "network", "inspect", "--format",
		"{{range $k, $v := .Containers}}{{$v.Name}}\n{{end}}", network)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// Info checks that the Docker daemon is accessible.
func Info(ctx context.Context) error {
	return run(ctx, "info")
}

// ComposeVersion checks that docker compose is available.
func ComposeVersion(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// run executes a docker command, forwarding stdout/stderr.
func run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %w", args[0], err)
	}
	return nil
}

// output executes a docker command and returns its stdout.
func output(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %w: %s", args[0], err, stderr.String())
	}
	return stdout.String(), nil
}
