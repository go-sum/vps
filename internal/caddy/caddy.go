package caddy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/caasmo/vps/internal/config"
)

// GenerateCaddyfile builds a complete Caddyfile from all registered apps.
// Each app gets a server block with reverse_proxy pointing to its upstream.
func GenerateCaddyfile(apps []config.AppConfig) string {
	var b strings.Builder

	for i, app := range apps {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(app.Domain)
		b.WriteString(" {\n")
		if app.InternalTLS {
			b.WriteString("\ttls internal\n")
		}
		b.WriteString(fmt.Sprintf("\treverse_proxy %s:%d\n", app.UpstreamHost(), app.UpstreamPort))
		b.WriteString("}\n")
	}

	return b.String()
}

// WriteCaddyfile writes the Caddyfile to the Caddy config directory.
func WriteCaddyfile(caddyDir string, content string) error {
	path := filepath.Join(caddyDir, "Caddyfile")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write Caddyfile: %w", err)
	}
	return nil
}

// Reload tells Caddy to reload its configuration.
// Assumes Caddy is running in a container named "caddy".
func Reload(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "exec", "caddy",
		"caddy", "reload", "--config", "/etc/caddy/Caddyfile")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("caddy reload: %w", err)
	}
	return nil
}

// RegenerateCaddyfile reads all app configs, generates a new Caddyfile,
// writes it, and reloads Caddy.
func RegenerateCaddyfile(ctx context.Context, vpsCfg config.VPSConfig) error {
	apps, err := config.ListApps(vpsCfg.AppsDir)
	if err != nil {
		return fmt.Errorf("list apps for Caddyfile: %w", err)
	}

	content := GenerateCaddyfile(apps)

	if err := WriteCaddyfile(vpsCfg.CaddyDir, content); err != nil {
		return err
	}

	if err := Reload(ctx); err != nil {
		return err
	}

	return nil
}
