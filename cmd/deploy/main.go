package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/caasmo/vps/internal/config"
	"github.com/caasmo/vps/internal/docker"
	"github.com/caasmo/vps/internal/health"
	"github.com/caasmo/vps/internal/repo"
	"github.com/caasmo/vps/internal/state"
)

var configPath string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	root := &cobra.Command{
		Use:   "deploy",
		Short: "Application deployment",
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to vps.yaml (default: /opt/vps/vps.yaml)")

	root.AddCommand(setupCmd())
	root.AddCommand(runCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(rollbackCmd())

	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func loadConfigs(appName string) (config.VPSConfig, config.AppConfig, error) {
	path := configPath
	if path == "" {
		path = config.ConfigPath(config.DefaultBaseDir)
	}
	vpsCfg, err := config.LoadVPSConfig(path)
	if err != nil {
		return config.VPSConfig{}, config.AppConfig{}, err
	}

	appDir := vpsCfg.AppDir(appName)
	appCfg, err := config.LoadAppConfig(appDir)
	if err != nil {
		return vpsCfg, config.AppConfig{}, fmt.Errorf("app %q not found: %w", appName, err)
	}

	return vpsCfg, appCfg, nil
}

func composeOpts(vpsCfg config.VPSConfig, appCfg config.AppConfig) docker.ComposeOpts {
	appDir := vpsCfg.AppDir(appCfg.Name)
	return docker.ComposeOpts{
		ProjectDir:  appDir,
		ProjectName: appCfg.ProjectName,
		EnvFile:     filepath.Join(appDir, appCfg.EnvFile),
	}
}

// ── setup ─────────────────────────────────────────────────────────────────────

func setupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup <app-name>",
		Short: "Initial deployment: build infrastructure and start services",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return runSetup(ctx, args[0])
		},
	}
}

func runSetup(ctx context.Context, appName string) error {
	vpsCfg, appCfg, err := loadConfigs(appName)
	if err != nil {
		return err
	}
	appDir := vpsCfg.AppDir(appName)

	// Validate prerequisites.
	fmt.Println("==> Validating prerequisites")
	if err := docker.Info(ctx); err != nil {
		return fmt.Errorf("docker is not accessible: %w", err)
	}
	if err := docker.ComposeVersion(ctx); err != nil {
		return fmt.Errorf("docker compose is not available: %w", err)
	}

	envPath := filepath.Join(appDir, appCfg.EnvFile)
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return fmt.Errorf("%s not found — create it with production secrets", envPath)
	}

	token := os.Getenv(appCfg.GithubToken)
	if token == "" {
		return fmt.Errorf("%s environment variable is required", appCfg.GithubToken)
	}

	// Clone repository.
	tmpDir, err := os.MkdirTemp("", "vps-deploy-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "src")
	fmt.Printf("==> Cloning %s (branch: %s)\n", appCfg.Repo, appCfg.Branch)
	sha, err := repo.Clone(ctx, appCfg.Repo, appCfg.Branch, srcDir)
	if err != nil {
		return err
	}
	fmt.Printf("    Commit: %.12s\n", sha)

	// Parse versions.
	versions, err := repo.ParseVersions(srcDir)
	if err != nil {
		return err
	}

	// Copy artifacts to app directory.
	fmt.Println("==> Copying artifacts")
	if err := copyArtifacts(srcDir, appDir, appCfg.ComposeFile); err != nil {
		return err
	}

	// Build infrastructure images.
	opts := composeOpts(vpsCfg, appCfg)
	opts.Env = versions // inject .versions into compose subprocess environment

	fmt.Printf("==> Building database image (PG %s)\n", versions["PG_VERSION"])
	if err := docker.ComposeBuild(ctx, opts, []string{"db"}); err != nil {
		return fmt.Errorf("build db: %w", err)
	}

	if _, ok := versions["DRAGONFLY_VERSION"]; ok {
		fmt.Printf("==> Building KV image (Dragonfly %s)\n", versions["DRAGONFLY_VERSION"])
		if err := docker.ComposeBuild(ctx, opts, []string{"kv"}); err != nil {
			return fmt.Errorf("build kv: %w", err)
		}
	}

	// Start infrastructure services.
	fmt.Println("==> Starting database and KV services")
	if err := docker.ComposeUp(ctx, opts, []string{"db", "kv"}, true); err != nil {
		return fmt.Errorf("start infra: %w", err)
	}
	fmt.Println("    Database and KV are healthy")

	// Record state.
	versionsHash, err := hashFile(filepath.Join(srcDir, ".versions"))
	if err == nil {
		_ = state.WriteVersionsHash(appDir, versionsHash)
	}

	fmt.Println("")
	fmt.Println("==> Infrastructure provisioning complete")
	fmt.Printf("    App dir:  %s\n", appDir)
	fmt.Println("")
	fmt.Println("    Next: deploy run " + appName)
	return nil
}

// ── run ───────────────────────────────────────────────────────────────────────

func runCmd() *cobra.Command {
	var (
		force  bool
		branch string
	)

	cmd := &cobra.Command{
		Use:   "run <app-name>",
		Short: "Deploy application: build, migrate, restart with health check",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return runDeploy(ctx, args[0], force, branch)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Deploy even if already at the same commit")
	cmd.Flags().StringVar(&branch, "branch", "", "Override branch from app config")

	return cmd
}

func runDeploy(ctx context.Context, appName string, force bool, branchOverride string) error {
	vpsCfg, appCfg, err := loadConfigs(appName)
	if err != nil {
		return err
	}
	appDir := vpsCfg.AppDir(appName)
	opts := composeOpts(vpsCfg, appCfg)

	// Validate.
	fmt.Println("==> Validating prerequisites")
	if err := docker.Info(ctx); err != nil {
		return fmt.Errorf("docker is not accessible: %w", err)
	}

	envPath := filepath.Join(appDir, appCfg.EnvFile)
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		return fmt.Errorf("%s not found", envPath)
	}

	token := os.Getenv(appCfg.GithubToken)
	if token == "" {
		return fmt.Errorf("%s environment variable is required", appCfg.GithubToken)
	}

	// Check infrastructure is running.
	for _, svc := range []string{"db", "kv"} {
		running, _ := docker.ComposeIsRunning(ctx, opts, svc)
		if !running {
			return fmt.Errorf("%s is not running — run 'deploy setup %s' first", svc, appName)
		}
	}

	// Clone.
	branch := appCfg.Branch
	if branchOverride != "" {
		branch = branchOverride
	}

	tmpDir, err := os.MkdirTemp("", "vps-deploy-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "src")
	fmt.Printf("==> Cloning %s (branch: %s)\n", appCfg.Repo, branch)
	sha, err := repo.Clone(ctx, appCfg.Repo, branch, srcDir)
	if err != nil {
		return err
	}
	fmt.Printf("    Commit: %.12s\n", sha)

	// Check if already deployed.
	prevState := state.ReadState(appDir)
	if !force && sha == prevState.DeployedSHA {
		fmt.Printf("==> Already deployed at %.12s, skipping\n", sha)
		fmt.Println("    Use --force to override")
		return nil
	}

	// Parse versions.
	versions, err := repo.ParseVersions(srcDir)
	if err != nil {
		return err
	}
	opts.Env = versions // inject .versions into compose subprocess environment

	// Save previous state for rollback.
	prevImageID, _ := docker.InspectID(ctx, appCfg.AppImage()+":latest")
	if prevImageID != "" {
		_ = state.WritePreviousImageID(appDir, prevImageID)
	}

	composeDst := filepath.Join(appDir, appCfg.ComposeFile)
	if _, err := os.Stat(composeDst); err == nil {
		copyFile(composeDst, composeDst+".prev")
	}

	// Build production image.
	fmt.Printf("==> Building production image (commit: %.12s)\n", sha)
	if err := docker.Build(ctx, docker.BuildOpts{
		ContextDir: srcDir,
		Dockerfile: filepath.Join(srcDir, "docker", "app", "Dockerfile"),
		Target:     "production_target",
		Tag:        appCfg.AppImage() + ":" + sha,
		BuildArgs:  repo.BuildArgsFromVersions(versions),
		Secrets:    []string{"id=github_token,env=" + appCfg.GithubToken},
	}); err != nil {
		return fmt.Errorf("build app: %w", err)
	}

	if err := docker.Tag(ctx, appCfg.AppImage()+":"+sha, appCfg.AppImage()+":latest"); err != nil {
		return fmt.Errorf("tag app image: %w", err)
	}

	// Copy artifacts.
	fmt.Println("==> Copying artifacts")
	if err := copyArtifacts(srcDir, appDir, appCfg.ComposeFile); err != nil {
		return err
	}

	// Migrations are applied by the app on startup (auto_migrate: true in config).
	// Extensions are installed by db/init/01-extensions.sql at container creation.

	// Start or restart app.
	if prevImageID == "" {
		fmt.Println("==> Starting app service")
		if err := docker.ComposeUp(ctx, opts, []string{"app"}, false); err != nil {
			return fmt.Errorf("start app: %w", err)
		}
	} else {
		fmt.Println("==> Restarting app service")
		if err := docker.ComposeRecreate(ctx, opts, []string{"app"}); err != nil {
			return fmt.Errorf("restart app: %w", err)
		}
	}

	// Connect app to Caddy network.
	containerName := appCfg.ProjectName + "-app-1"
	_ = docker.NetworkConnect(ctx, vpsCfg.CaddyNetwork, containerName)

	// Health check.
	if appCfg.HealthURL != "" {
		fmt.Println("==> Waiting for health check...")
		if err := health.Check(ctx, appCfg.HealthURL, appCfg.HealthRetries, 2*time.Second); err != nil {
			fmt.Printf("ERROR: %v\n", err)

			// Show logs.
			fmt.Println("==> Recent logs:")
			_ = docker.ComposeLogs(ctx, opts, "app", 50)

			// Rollback.
			if prevImageID != "" {
				fmt.Println("==> Rolling back to previous version...")
				_ = docker.ComposeStop(ctx, opts, []string{"app"})
				_ = docker.Tag(ctx, prevImageID, appCfg.AppImage()+":latest")
				if _, err := os.Stat(composeDst + ".prev"); err == nil {
					copyFile(composeDst+".prev", composeDst)
				}
				_ = docker.ComposeRecreate(ctx, opts, []string{"app"})
				_ = docker.NetworkConnect(ctx, vpsCfg.CaddyNetwork, containerName)
				fmt.Println("    Rolled back to previous version")
			} else {
				fmt.Println("    No previous version to roll back to (initial deploy)")
			}
			return fmt.Errorf("deployment failed: health check did not pass")
		}
	}

	// Record successful deployment.
	_ = state.WriteDeployedSHA(appDir, sha)
	_ = state.ClearPreviousImageID(appDir)

	fmt.Println("")
	fmt.Println("==> Deployment complete")
	fmt.Printf("    Branch:  %s\n", branch)
	fmt.Printf("    Commit:  %.12s\n", sha)
	if appCfg.Domain != "" {
		scheme := "http"
		if appCfg.InternalTLS {
			scheme = "https"
		}
		fmt.Printf("    URL:     %s://%s\n", scheme, appCfg.Domain)
	}
	return nil
}

// ── status ────────────────────────────────────────────────────────────────────

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <app-name>",
		Short: "Show deployment status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			vpsCfg, appCfg, err := loadConfigs(args[0])
			if err != nil {
				return err
			}
			appDir := vpsCfg.AppDir(appCfg.Name)

			st := state.ReadState(appDir)

			fmt.Printf("App:      %s\n", appCfg.Name)
			fmt.Printf("Domain:   %s\n", appCfg.Domain)
			fmt.Printf("Branch:   %s\n", appCfg.Branch)
			fmt.Printf("Repo:     %s\n", appCfg.Repo)

			if st.DeployedSHA != "" {
				fmt.Printf("Deployed: %.12s\n", st.DeployedSHA)
			} else {
				fmt.Println("Deployed: (not yet deployed)")
			}

			// Check Caddy network connectivity.
			containerName := appCfg.ProjectName + "-app-1"
			caddyContainers, _ := docker.NetworkContainers(ctx, vpsCfg.CaddyNetwork)
			onCaddy := false
			for _, c := range caddyContainers {
				if c == containerName {
					onCaddy = true
					break
				}
			}
			if onCaddy {
				fmt.Printf("Network:  connected to %s\n", vpsCfg.CaddyNetwork)
			} else {
				fmt.Printf("Network:  NOT connected to %s\n", vpsCfg.CaddyNetwork)
			}

			// Container resource usage.
			fmt.Println("")
			services := map[string]string{
				"app": appCfg.ProjectName + "-app-1",
				"db":  appCfg.ProjectName + "-db-1",
				"kv":  appCfg.ProjectName + "-kv-1",
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SERVICE\tSTATUS\tCPU\tMEMORY\tPIDs")
			for _, svc := range []string{"app", "db", "kv"} {
				container := services[svc]
				cs, _ := docker.Stats(ctx, container)
				status := cs.Status
				if status == "" {
					status = "stopped"
				}
				mem := cs.Memory
				if cs.MemLimit != "" {
					mem = cs.Memory + " / " + cs.MemLimit
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", svc, status, cs.CPU, mem, cs.PIDs)
			}
			w.Flush()

			return nil
		},
	}
}

// ── rollback ──────────────────────────────────────────────────────────────────

func rollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <app-name>",
		Short: "Roll back to the previous deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			vpsCfg, appCfg, err := loadConfigs(args[0])
			if err != nil {
				return err
			}
			appDir := vpsCfg.AppDir(appCfg.Name)
			opts := composeOpts(vpsCfg, appCfg)

			st := state.ReadState(appDir)
			if st.PreviousImageID == "" {
				return fmt.Errorf("no previous deployment to roll back to")
			}

			fmt.Println("==> Rolling back to previous version")

			_ = docker.ComposeStop(ctx, opts, []string{"app"})
			if err := docker.Tag(ctx, st.PreviousImageID, appCfg.AppImage()+":latest"); err != nil {
				return fmt.Errorf("restore previous image: %w", err)
			}

			composePath := filepath.Join(appDir, appCfg.ComposeFile)
			if _, err := os.Stat(composePath + ".prev"); err == nil {
				copyFile(composePath+".prev", composePath)
			}

			if err := docker.ComposeRecreate(ctx, opts, []string{"app"}); err != nil {
				return fmt.Errorf("restart app: %w", err)
			}

			containerName := appCfg.ProjectName + "-app-1"
			_ = docker.NetworkConnect(ctx, vpsCfg.CaddyNetwork, containerName)

			// Health check after rollback.
			if appCfg.HealthURL != "" {
				fmt.Println("==> Waiting for health check...")
				if err := health.Check(ctx, appCfg.HealthURL, appCfg.HealthRetries, 2*time.Second); err != nil {
					return fmt.Errorf("rollback health check failed: %w", err)
				}
			}

			_ = state.ClearPreviousImageID(appDir)

			fmt.Println("==> Rollback complete")
			return nil
		},
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// copyArtifacts copies compose file, docker/ and db/ from source to deploy dir.
func copyArtifacts(srcDir, deployDir, composeFile string) error {
	// Copy compose file.
	if err := copyFile(
		filepath.Join(srcDir, composeFile),
		filepath.Join(deployDir, composeFile),
	); err != nil {
		return fmt.Errorf("copy compose file: %w", err)
	}

	// Copy docker/ directory.
	if err := copyDir(filepath.Join(srcDir, "docker"), filepath.Join(deployDir, "docker")); err != nil {
		return fmt.Errorf("copy docker dir: %w", err)
	}

	// Copy db/ directory.
	if err := copyDir(filepath.Join(srcDir, "db"), filepath.Join(deployDir, "db")); err != nil {
		return fmt.Errorf("copy db dir: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
