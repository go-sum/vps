package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const AppConfigFile = "app.yaml"

// AppConfig holds per-app deployment configuration.
type AppConfig struct {
	Name          string `yaml:"name"`
	Repo          string `yaml:"repo"`
	Branch        string `yaml:"branch"`
	Domain        string `yaml:"domain"`
	UpstreamPort  int    `yaml:"upstream_port"`
	ProjectName   string `yaml:"project_name"`
	ComposeFile   string `yaml:"compose_file"`
	EnvFile       string `yaml:"env_file"`
	HealthURL     string `yaml:"health_url"`
	HealthRetries int    `yaml:"health_retries"`
	SchemaFile    string `yaml:"schema_file"`
	SchemaTimeout int    `yaml:"schema_timeout"`
	InternalTLS   bool   `yaml:"internal_tls"`
	GithubToken   string `yaml:"github_token_env"`
}

// ApplyDefaults fills in zero-valued fields with sensible defaults.
func (a *AppConfig) ApplyDefaults() {
	if a.Branch == "" {
		a.Branch = "main"
	}
	if a.UpstreamPort == 0 {
		a.UpstreamPort = 8080
	}
	if a.ProjectName == "" {
		a.ProjectName = a.Name + "-prod"
	}
	if a.ComposeFile == "" {
		a.ComposeFile = "docker-compose.yml"
	}
	if a.EnvFile == "" {
		a.EnvFile = ".env"
	}
	if a.HealthRetries == 0 {
		a.HealthRetries = 30
	}
	if a.SchemaFile == "" {
		a.SchemaFile = "db/sql/schema.sql"
	}
	if a.SchemaTimeout == 0 {
		a.SchemaTimeout = 120
	}
	if a.GithubToken == "" {
		a.GithubToken = "GITHUB_ACCESS_TOKEN"
	}
}

// UpstreamHost returns the Docker container hostname for Caddy to reverse proxy to.
// Convention: <project_name>-app-1
func (a AppConfig) UpstreamHost() string {
	return a.ProjectName + "-app-1"
}

// ToolsImage returns the Docker image name for the tools container.
func (a AppConfig) ToolsImage() string {
	return a.ProjectName + "-tools"
}

// AppImage returns the Docker image name used for the app.
func (a AppConfig) AppImage() string {
	return a.Name
}

// LoadAppConfig reads an app config from the given directory.
func LoadAppConfig(appDir string) (AppConfig, error) {
	path := filepath.Join(appDir, AppConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return AppConfig{}, fmt.Errorf("read app config %s: %w", path, err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return AppConfig{}, fmt.Errorf("parse app config %s: %w", path, err)
	}

	cfg.ApplyDefaults()
	return cfg, nil
}

// SaveAppConfig writes the app config to the given directory.
func SaveAppConfig(appDir string, cfg AppConfig) error {
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return fmt.Errorf("create app dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal app config: %w", err)
	}

	path := filepath.Join(appDir, AppConfigFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write app config: %w", err)
	}
	return nil
}

// ListApps returns all app configs found under the apps directory.
func ListApps(appsDir string) ([]AppConfig, error) {
	entries, err := os.ReadDir(appsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list apps dir: %w", err)
	}

	var apps []AppConfig
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		appDir := filepath.Join(appsDir, entry.Name())
		cfg, err := LoadAppConfig(appDir)
		if err != nil {
			continue // skip directories without valid app.yaml
		}
		apps = append(apps, cfg)
	}
	return apps, nil
}
