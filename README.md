# VPS — Server Deployment & Management CLI

A Go-based CLI tool for managing VPS infrastructure and deploying containerized applications. Replaces fragile bash deployment scripts with typed configuration, structured error handling, automatic rollback, and Caddy reverse proxy management.

Two binaries handle separate concerns:
- **`admin`** — VPS infrastructure: initialize the server, manage Caddy, register applications
- **`deploy`** — Application lifecycle: build, migrate, deploy, health check, rollback

---

## Features

- **Caddy Reverse Proxy** — Automatic Caddyfile generation from registered apps with internal TLS, hot-reload on app add/remove
- **Docker Compose Orchestration** — Build and manage multi-service stacks (app, PostgreSQL, Dragonfly KV) with health checks
- **Zero-Downtime Deploys** — Build new image, restart app, verify health — rollback automatically on failure
- **Commit-SHA Tracking** — Skips redundant deploys when the same commit is already running; override with `--force`
- **Rollback** — Stores previous image ID and compose file; one command to restore
- **Resource Monitoring** — `deploy status` and `admin status` show CPU, memory, and PID usage per container
- **Caddy Network Visibility** — `admin status` lists all containers connected to the Caddy network with resource usage
- **Embedded Templates** — Caddy docker-compose.yml is compiled into the binary; no source tree needed on the server
- **Cross-Compiled Static Binaries** — Single `scp` to deploy; no runtime dependencies beyond Docker

---

## Installation

### Prerequisites

- Go 1.26+ (build machine only)
- Docker and Docker Compose plugin (target server)
- Git (target server)

### Build

```bash
# Clone the repository
git clone https://github.com/go-sum/vps.git
cd vps

# Build for the local machine
go build -o bin/admin ./cmd/admin
go build -o bin/deploy ./cmd/deploy

# Cross-compile for x86_64 Linux (e.g., Debian VPS)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/admin ./cmd/admin
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/deploy ./cmd/deploy
```

### Deploy to Server

```bash
scp bin/admin bin/deploy user@server:/usr/local/bin/
```

---

## Usage

### Initial Server Setup

```bash
# 1. Initialize VPS structure and start Caddy
admin setup

# 2. Register an application
admin app add forge \
  --repo https://github.com/go-sum/forge.git \
  --domain forge.home

# 3. Create the app's .env file with production secrets
vim /opt/apps/forge/.env

# 4. Provision infrastructure (PostgreSQL, Dragonfly)
export GITHUB_ACCESS_TOKEN=ghp_...
deploy setup forge

# 5. Deploy the application
deploy run forge
```

### Subsequent Deployments

```bash
# Deploy latest commit from configured branch
deploy run forge

# Deploy from a different branch
deploy run forge --branch staging

# Force deploy even if same commit is already running
deploy run forge --force

# Roll back to previous version
deploy rollback forge
```

### Monitoring

```bash
# App deployment status with resource usage
deploy status forge

# VPS-wide overview: Caddy, network, all apps
admin status
```

### Managing Applications

```bash
# List all registered apps
admin app list

# Add a new app
admin app add myapp \
  --repo https://github.com/user/myapp.git \
  --domain myapp.example.com \
  --branch main \
  --port 3000

# Remove an app (removes config and updates Caddyfile)
admin app remove myapp
```

---

### Command Reference

#### `admin`

| Command | Description |
|---------|-------------|
| `admin setup` | Create directory structure, write Caddy config, start Caddy container |
| `admin status` | Show Caddy status, network containers, and per-app resource usage |
| `admin app add <name>` | Register an app and update Caddy reverse proxy |
| `admin app list` | List all registered apps |
| `admin app remove <name>` | Remove an app and update Caddy |

**`admin app add` flags:**

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--repo` | Yes | — | Git repository URL |
| `--domain` | Yes | — | Domain name for Caddy |
| `--branch` | No | `main` | Git branch to deploy |
| `--port` | No | `8080` | Upstream container port |
| `--internal-tls` | No | `true` | Use Caddy's internal TLS |

#### `deploy`

| Command | Description |
|---------|-------------|
| `deploy setup <app>` | Clone repo, build infrastructure images (db, kv), start services |
| `deploy run <app>` | Build app image, restart, health check, rollback on failure |
| `deploy status <app>` | Show deployment state, network connectivity, container resource usage |
| `deploy rollback <app>` | Restore previous image and compose file, restart app |

**`deploy run` flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Deploy even if the same commit SHA is already deployed |
| `--branch` | (from app.yaml) | Override the configured branch for this deploy |

**Global flag (both binaries):**

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `/opt/vps/vps.yaml` | Path to VPS configuration file |

---

## Technologies

| Technology | Purpose |
|------------|---------|
| [Go](https://go.dev/) | CLI implementation, cross-compilation |
| [Docker](https://www.docker.com/) | Container builds and orchestration |
| [Docker Compose](https://docs.docker.com/compose/) | Multi-service stack management |
| [Caddy](https://caddyserver.com/) | Reverse proxy with automatic TLS |

---

## Configuration

### VPS Config (`/opt/vps/vps.yaml`)

Created by `admin setup`. Controls the directory layout and network settings.

```yaml
base_dir: /opt                # Root directory
caddy_dir: /opt/vps/caddy     # Caddy compose file and Caddyfile
apps_dir: /opt/apps           # Per-app deployment directories
caddy_network: caddy_net      # Docker network connecting Caddy to apps
```

### App Config (`/opt/apps/<name>/app.yaml`)

Created by `admin app add`. Controls how the app is built, deployed, and proxied.

```yaml
name: forge                                         # App identifier
repo: https://github.com/go-sum/forge.git           # Git clone URL
branch: main                                        # Branch to deploy
domain: forge.home                                  # Caddy domain
upstream_port: 8080                                 # Container port
project_name: forge-prod                            # Docker Compose project name
compose_file: docker-compose.yml                    # Compose file in repo
env_file: .env                                      # Secrets file (relative to app dir)
health_url: https://localhost/health                # Health check endpoint (empty = skip)
health_retries: 30                                  # Max health check attempts (2s interval)
internal_tls: true                                  # Caddy internal TLS
github_token_env: GITHUB_ACCESS_TOKEN               # Env var name for GitHub PAT
```

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_ACCESS_TOKEN` | Yes (deploy) | GitHub PAT for cloning private repos and downloading private Go modules |

### App `.env` File (`/opt/apps/<name>/.env`)

Must be created manually before running `deploy setup`. Contains production secrets read by Docker Compose and the application:

```env
DATABASE_URL=postgres://postgres:secret@db:5432/mydb?sslmode=disable
PGUSER=postgres
PGPASSWORD=secret
PGDATABASE=mydb
# ... application-specific secrets
```

### Repository Requirements

The application repository must contain:

| File/Directory | Purpose |
|----------------|---------|
| `.versions` | Version pins (`GO_VERSION=1.26`, `PG_VERSION=18`, etc.) |
| `docker-compose.yml` | Service definitions (app, db, kv) |
| `docker/app/Dockerfile` | Multi-stage build with `production_target` target |
| `docker/postgres/Dockerfile` | PostgreSQL image build |
| `docker/dragonfly/Dockerfile` | Dragonfly KV image build |
| `db/migrations/` | Versioned SQL migration files (applied by app on startup via goose) |
| `db/init/01-extensions.sql` | PostgreSQL extensions (citext) |

---

## Requirements

### Build Machine
- Go 1.26+

### Target Server
- Linux (x86_64 or arm64)
- Docker Engine 24+
- Docker Compose v2 plugin
- Git
- Outbound HTTPS access (for git clone, Docker Hub pulls)

### Network
- Ports 80 and 443 open for Caddy (HTTP/HTTPS)
- Application port (default 8080) open only if direct access is needed

---

## Directory Layout

After setup, the server filesystem looks like:

```
/opt/
├── vps/
│   ├── vps.yaml                    # VPS configuration
│   └── caddy/
│       ├── docker-compose.yml      # Caddy service definition
│       └── Caddyfile               # Generated reverse proxy config
└── apps/
    └── forge/
        ├── app.yaml                # App configuration
        ├── .env                    # Production secrets
        ├── .deployed_sha           # Current deployed commit
        ├── .previous_image_id      # Rollback image reference
        ├── docker-compose.yml      # Copied from repo at deploy time
        ├── docker-compose.yml.prev # Previous version (for rollback)
        ├── docker/                 # Copied from repo
        └── db/                     # Copied from repo
```

---

## Deploy Workflow

The `deploy run` command performs these steps in order:

1. **Validate** — Check Docker, .env file, GitHub token, running infrastructure
2. **Clone** — Shallow clone of the configured branch to a temp directory
3. **SHA Check** — Skip if already deployed at this commit (unless `--force`)
4. **Save State** — Record current image ID and compose file for rollback
5. **Build** — `docker build --target production_target` with version build-args and GitHub token secret
6. **Tag** — Tag new image as `<app>:latest`
7. **Copy Artifacts** — Copy compose file, docker/, db/ to app directory
8. **Restart** — `docker compose up -d --force-recreate --no-deps app`
9. **Migrate** — App applies pending goose migrations on startup (`auto_migrate: true`)
10. **Network** — Connect app container to Caddy network
11. **Health Check** — Poll health URL up to 30 times at 2s intervals
12. **Record** — Write deployed SHA; clear rollback state
13. **Rollback** (on failure) — Restore previous image, compose file, and restart
