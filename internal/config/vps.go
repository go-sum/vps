package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	DefaultBaseDir      = "/opt"
	DefaultCaddyNetwork = "caddy_net"
	VPSConfigFile       = "vps.yaml"
)

// VPSConfig holds VPS-level configuration.
type VPSConfig struct {
	BaseDir      string `yaml:"base_dir"`
	CaddyDir     string `yaml:"caddy_dir"`
	AppsDir      string `yaml:"apps_dir"`
	CaddyNetwork string `yaml:"caddy_network"`
}

// DefaultVPSConfig returns a VPSConfig with sensible defaults.
// Layout under /opt:
//
//	/opt/vps/vps.yaml     — config file
//	/opt/vps/caddy/       — Caddy compose + Caddyfile
//	/opt/apps/<name>/     — per-app deploy dirs
func DefaultVPSConfig() VPSConfig {
	return VPSConfig{
		BaseDir:      DefaultBaseDir,
		CaddyDir:     filepath.Join(DefaultBaseDir, "vps", "caddy"),
		AppsDir:      filepath.Join(DefaultBaseDir, "apps"),
		CaddyNetwork: DefaultCaddyNetwork,
	}
}

// LoadVPSConfig reads a VPS config from the given path.
// If the file does not exist, it returns defaults.
func LoadVPSConfig(path string) (VPSConfig, error) {
	cfg := DefaultVPSConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read vps config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse vps config: %w", err)
	}

	cfg.applyDefaults()
	return cfg, nil
}

// SaveVPSConfig writes the config to disk as YAML.
func SaveVPSConfig(path string, cfg VPSConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal vps config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write vps config: %w", err)
	}
	return nil
}

// ConfigPath returns the full path to vps.yaml.
// Config lives at <base_dir>/vps/vps.yaml.
func ConfigPath(baseDir string) string {
	return filepath.Join(baseDir, "vps", VPSConfigFile)
}

// AppDir returns the directory for a specific app.
func (c VPSConfig) AppDir(name string) string {
	return filepath.Join(c.AppsDir, name)
}

func (c *VPSConfig) applyDefaults() {
	if c.BaseDir == "" {
		c.BaseDir = DefaultBaseDir
	}
	if c.CaddyDir == "" {
		c.CaddyDir = filepath.Join(c.BaseDir, "vps", "caddy")
	}
	if c.AppsDir == "" {
		c.AppsDir = filepath.Join(c.BaseDir, "apps")
	}
	if c.CaddyNetwork == "" {
		c.CaddyNetwork = DefaultCaddyNetwork
	}
}
