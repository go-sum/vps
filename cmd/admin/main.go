package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/caasmo/vps/internal/caddy"
	"github.com/caasmo/vps/internal/config"
	"github.com/caasmo/vps/internal/docker"
)

var configPath string

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	root := &cobra.Command{
		Use:   "admin",
		Short: "VPS server management",
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to vps.yaml (default: /opt/vps/vps.yaml)")

	root.AddCommand(setupCmd())
	root.AddCommand(statusCmd())
	root.AddCommand(appCmd())

	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func loadVPSConfig() (config.VPSConfig, error) {
	path := configPath
	if path == "" {
		path = config.ConfigPath(config.DefaultBaseDir)
	}
	return config.LoadVPSConfig(path)
}

func resolveConfigPath(cfg config.VPSConfig) string {
	if configPath != "" {
		return configPath
	}
	return config.ConfigPath(cfg.BaseDir)
}

// ── setup ─────────────────────────────────────────────────────────────────────

func setupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Initialize VPS directory structure and start Caddy",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cfg := config.DefaultVPSConfig()

			// Validate Docker is available.
			fmt.Println("==> Validating prerequisites")
			if err := docker.Info(ctx); err != nil {
				return fmt.Errorf("docker is not accessible: %w", err)
			}
			if err := docker.ComposeVersion(ctx); err != nil {
				return fmt.Errorf("docker compose is not available: %w", err)
			}

			// Create directory structure.
			fmt.Println("==> Creating directory structure")
			for _, dir := range []string{cfg.BaseDir, cfg.AppsDir, cfg.CaddyDir} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("create %s: %w", dir, err)
				}
				fmt.Printf("    %s\n", dir)
			}

			// Write Caddy docker-compose.yml from embedded template.
			fmt.Println("==> Writing Caddy configuration")
			dst := filepath.Join(cfg.CaddyDir, "docker-compose.yml")
			if err := os.WriteFile(dst, caddyComposeYML, 0o644); err != nil {
				return fmt.Errorf("write caddy compose: %w", err)
			}

			// Generate Caddyfile from any existing apps, or write a placeholder.
			apps, _ := config.ListApps(cfg.AppsDir)
			if len(apps) > 0 {
				content := caddy.GenerateCaddyfile(apps)
				if err := caddy.WriteCaddyfile(cfg.CaddyDir, content); err != nil {
					return err
				}
				fmt.Printf("    Caddyfile: %d app(s) configured\n", len(apps))
			} else {
				if err := caddy.WriteCaddyfile(cfg.CaddyDir, "# No apps registered yet\n"); err != nil {
					return err
				}
			}

			// Save VPS config only if it doesn't already exist.
			cfgPath := resolveConfigPath(cfg)
			if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
				if err := config.SaveVPSConfig(cfgPath, cfg); err != nil {
					return err
				}
				fmt.Printf("    Config: %s (created)\n", cfgPath)
			} else {
				fmt.Printf("    Config: %s (exists, not overwritten)\n", cfgPath)
			}

			// Start Caddy (without --wait; the compose healthcheck hits
			// localhost but Caddy only responds to configured domains).
			fmt.Println("==> Starting Caddy")
			caddyOpts := docker.ComposeOpts{
				ProjectDir:  cfg.CaddyDir,
				ProjectName: "caddy",
			}
			if err := docker.ComposeUp(ctx, caddyOpts, []string{"caddy"}, false); err != nil {
				return fmt.Errorf("start caddy: %w", err)
			}

			fmt.Println("")
			fmt.Println("==> VPS setup complete")
			fmt.Printf("    Base dir: %s\n", cfg.BaseDir)
			fmt.Println("    Next: admin app add <name> --repo <url> --domain <domain>")
			return nil
		},
	}
}

//go:embed caddy-compose.yml
var caddyComposeYML []byte

// ── status ────────────────────────────────────────────────────────────────────

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show Caddy and network status for all apps",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			vpsCfg, err := loadVPSConfig()
			if err != nil {
				return err
			}

			// Caddy container status.
			fmt.Println("Caddy:")
			cs, _ := docker.Stats(ctx, "caddy")
			if cs.Status == "" {
				cs.Status = "stopped"
			}
			fmt.Printf("  Status:  %s\n", cs.Status)
			if cs.CPU != "" {
				fmt.Printf("  CPU:     %s\n", cs.CPU)
			}
			if cs.Memory != "" {
				mem := cs.Memory
				if cs.MemLimit != "" {
					mem += " / " + cs.MemLimit
				}
				fmt.Printf("  Memory:  %s\n", mem)
			}
			if cs.Image != "" {
				fmt.Printf("  Image:   %s\n", cs.Image)
			}

			// Caddy network.
			fmt.Println("")
			fmt.Printf("Network: %s\n", vpsCfg.CaddyNetwork)
			containers, _ := docker.NetworkContainers(ctx, vpsCfg.CaddyNetwork)
			if len(containers) == 0 {
				fmt.Println("  (no containers connected)")
			} else {
				for _, c := range containers {
					cs, _ := docker.Stats(ctx, c)
					status := cs.Status
					if status == "" {
						status = "unknown"
					}
					mem := cs.Memory
					if mem == "" {
						mem = "-"
					}
					cpu := cs.CPU
					if cpu == "" {
						cpu = "-"
					}
					fmt.Printf("  %-30s  %s  cpu=%s  mem=%s\n", c, status, cpu, mem)
				}
			}

			// Per-app summary.
			apps, _ := config.ListApps(vpsCfg.AppsDir)
			if len(apps) > 0 {
				fmt.Println("")
				fmt.Println("Apps:")
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "  NAME\tDOMAIN\tSTATUS\tCPU\tMEMORY")
				for _, app := range apps {
					container := app.ProjectName + "-app-1"
					cs, _ := docker.Stats(ctx, container)
					status := cs.Status
					if status == "" {
						status = "stopped"
					}
					mem := cs.Memory
					if mem == "" {
						mem = "-"
					}
					cpu := cs.CPU
					if cpu == "" {
						cpu = "-"
					}
					fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", app.Name, app.Domain, status, cpu, mem)
				}
				w.Flush()
			}

			return nil
		},
	}
}

// ── app ───────────────────────────────────────────────────────────────────────

func appCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage applications",
	}
	cmd.AddCommand(appAddCmd())
	cmd.AddCommand(appListCmd())
	cmd.AddCommand(appRemoveCmd())
	return cmd
}

func appAddCmd() *cobra.Command {
	var (
		repo        string
		branch      string
		domain      string
		port        int
		internalTLS bool
	)

	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Register a new application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]

			vpsCfg, err := loadVPSConfig()
			if err != nil {
				return err
			}

			appDir := vpsCfg.AppDir(name)
			if _, err := os.Stat(filepath.Join(appDir, config.AppConfigFile)); err == nil {
				return fmt.Errorf("app %q already exists at %s", name, appDir)
			}

			appCfg := config.AppConfig{
				Name:        name,
				Repo:        repo,
				Branch:      branch,
				Domain:      domain,
				InternalTLS: internalTLS,
			}
			if port != 0 {
				appCfg.UpstreamPort = port
			}
			appCfg.ApplyDefaults()

			fmt.Printf("==> Registering app %q\n", name)
			if err := config.SaveAppConfig(appDir, appCfg); err != nil {
				return err
			}
			fmt.Printf("    Config: %s\n", filepath.Join(appDir, config.AppConfigFile))

			// Regenerate Caddyfile with the new app.
			fmt.Println("==> Updating Caddy configuration")
			if err := caddy.RegenerateCaddyfile(ctx, vpsCfg); err != nil {
				// Non-fatal if Caddy isn't running yet.
				fmt.Printf("    Warning: %v\n", err)
				fmt.Println("    Run 'admin setup' first, or reload Caddy manually")
			}

			fmt.Println("")
			fmt.Printf("==> App %q registered\n", name)
			fmt.Printf("    Domain:   %s\n", appCfg.Domain)
			fmt.Printf("    Upstream: %s:%d\n", appCfg.UpstreamHost(), appCfg.UpstreamPort)
			fmt.Println("    Next: deploy setup " + name)
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Git repository URL (required)")
	cmd.Flags().StringVar(&branch, "branch", "main", "Git branch to deploy")
	cmd.Flags().StringVar(&domain, "domain", "", "Domain name for Caddy (required)")
	cmd.Flags().IntVar(&port, "port", 0, "Upstream port (default: 8080)")
	cmd.Flags().BoolVar(&internalTLS, "internal-tls", true, "Use Caddy internal TLS")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("domain")

	return cmd
}

func appListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered applications",
		RunE: func(cmd *cobra.Command, args []string) error {
			vpsCfg, err := loadVPSConfig()
			if err != nil {
				return err
			}

			apps, err := config.ListApps(vpsCfg.AppsDir)
			if err != nil {
				return err
			}

			if len(apps) == 0 {
				fmt.Println("No apps registered")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tDOMAIN\tBRANCH\tREPO")
			for _, app := range apps {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", app.Name, app.Domain, app.Branch, app.Repo)
			}
			return w.Flush()
		},
	}
}

func appRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a registered application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]

			vpsCfg, err := loadVPSConfig()
			if err != nil {
				return err
			}

			appDir := vpsCfg.AppDir(name)
			if _, err := os.Stat(appDir); os.IsNotExist(err) {
				return fmt.Errorf("app %q not found at %s", name, appDir)
			}

			fmt.Printf("==> Removing app %q\n", name)
			if err := os.RemoveAll(appDir); err != nil {
				return fmt.Errorf("remove app dir: %w", err)
			}

			// Regenerate Caddyfile without the removed app.
			fmt.Println("==> Updating Caddy configuration")
			if err := caddy.RegenerateCaddyfile(ctx, vpsCfg); err != nil {
				fmt.Printf("    Warning: %v\n", err)
			}

			fmt.Printf("    App %q removed\n", name)
			return nil
		},
	}
}
