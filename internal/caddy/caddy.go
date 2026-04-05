package caddy

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/go-sum/vps/internal/config"
)

//go:embed caddyfile.tmpl
var caddyfileTmpl string

var tmpl = template.Must(template.New("Caddyfile").Parse(caddyfileTmpl))

// templateData is the context passed to the Caddyfile template.
type templateData struct {
	Apps []templateApp
}

// templateApp exposes the fields the template needs.
// This decouples the template from config.AppConfig's method names.
type templateApp struct {
	Domain       string
	InternalTLS  bool
	UpstreamHost string
	UpstreamPort int
}

// GenerateCaddyfile builds a complete Caddyfile from all registered apps
// by executing the embedded caddyfile.tmpl template.
func GenerateCaddyfile(apps []config.AppConfig) string {
	data := templateData{Apps: make([]templateApp, len(apps))}
	for i, app := range apps {
		data.Apps[i] = templateApp{
			Domain:       app.Domain,
			InternalTLS:  app.InternalTLS,
			UpstreamHost: app.UpstreamHost(),
			UpstreamPort: app.UpstreamPort,
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// Template is compiled at init; execution errors indicate a bug.
		panic(fmt.Sprintf("caddyfile template: %v", err))
	}
	return buf.String()
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
