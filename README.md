# VPS — Server Deployment & Management CLI

A Go-based CLI tool for managing VPS infrastructure and deploying containerized applications. Replaces fragile bash deployment scripts with typed configuration, structured error handling, automatic rollback, and Caddy reverse proxy management.

Two binaries handle separate concerns:
- **`server`** — VPS infrastructure: initialize the server, manage Caddy, register applications
- **`app`** — Application lifecycle: build, migrate, deploy, health check, rollback

---

## Features

- **Caddy Reverse Proxy** — Automatic Caddyfile generation from registered apps with internal TLS, hot-reload on app add/remove
- **Docker Compose Orchestration** — Build and manage multi-service stacks (app, PostgreSQL, Dragonfly KV) with health checks
- **Two Deployment Paths** — Build on the server (`app run`) or pull a pre-built image from GHCR (`app pull`); both produce the same production image
- **Zero-Downtime Deploys** — Build or pull new image, restart app, verify health — rollback automatically on failure
- **Commit-SHA Tracking** — Tags images with commit SHA; skips redundant deploys when the same commit is already running; override with `--force`
- **Registry Integration** — Build production images in GitHub Actions, push to GHCR, pull to any server — no on-server compilation needed
- **Rollback** — Stores previous image ID and compose file; one command to restore
- **Conditional Infrastructure** — KV service (Dragonfly) only provisioned when `DRAGONFLY_VERSION` is present in `.versions`
- **Resource Monitoring** — `app status` and `server status` show CPU, memory, and PID usage per container
- **Health Status** — `app status` and `server status` show Docker healthcheck status (healthy/unhealthy/starting) per container
- **Restart** — `app restart` restarts app containers with health check; `server restart` restarts Caddy and all app containers
- **Caddy Network Visibility** — `server status` lists all containers connected to the Caddy network with resource usage
- **Embedded Templates** — Caddy docker-compose.yml is compiled into the binary; no source tree needed on the server
- **Cross-Compiled Static Binaries** — Single `scp` to deploy; no runtime dependencies beyond Docker

---

## Usage

> Throughout the usage and installation examples, the `forge` repo is refrenced. for example instead of:

```bash
# 2. Register an application
server app add `<appname>` --repo `<repository>` --domain `<domain>`
```
> you'll see (replace where appropriate)
```bash
# 2. Register an application
server app add forge --repo https://github.com/go-sum/forge.git --domain forge.home
```

### Initial Server Setup

```bash
# 1. Initialize VPS structure and start Caddy
server setup

# 2. Register an application
server app add `<appname>` --repo `<repository>` --domain `<domain>`
## Example:
server app add forge --repo https://github.com/go-sum/forge.git --domain forge.home

# 3. Create the app's .env file with production secrets
vim /opt/apps/`<appname>`/.env
## Example:
vim /opt/apps/forge/.env

# 4. Provision infrastructure (PostgreSQL, and Dragonfly if DRAGONFLY_VERSION is in .versions)
export GITHUB_ACCESS_TOKEN=ghp_...
app setup `<appname>`
## Example:
app setup forge
```

### Deployment options

Two deployment paths are possible:

#### Option A: Build on Server (`app run`)

Clones the repo on the server, builds the production tools image (cached), builds the app image, and restarts. Best for fast iteration when the server has sufficient resources.

```bash
# Deploy latest commit from configured branch
app run forge

# Deploy from a different branch
app run forge --branch staging

# Force deploy even if same commit is already running
app run forge --force
```

#### Option B: Pull from Registry (`app pull`)

Pulls a pre-built image from GHCR and restarts. No compilation on the server — only a sparse checkout of orchestration files (compose, db/, .versions). Best for production servers with limited resources or when build consistency matters.

```bash
# Prerequisites (one-time):
# 1. Configure registry in app.yaml (see Configuration below)
# 2. Set GHCR_TOKEN env var on the server (PAT with read:packages scope)
# 3. Trigger a CI build via GitHub Actions (workflow_dispatch or tag push)

# Pull latest image from registry
app pull forge

# Pull a specific tagged version
app pull forge --tag v1.0

# Pull a specific commit build
app pull forge --tag abc123f
```

#### Rollback (works with both options)

```bash
app rollback forge
```

### Restart

```bash
# Restart a specific app's containers (with health check)
app restart forge

# Restart Caddy and all registered app containers
server restart
```

### Monitoring

```bash
# App deployment status with resource usage and health
app status forge

# VPS-wide overview: Caddy, network, all apps with health
server status
```

### Managing Applications

```bash
# List all registered apps
server app list

# Add a new app
server app add myapp \
  --repo https://github.com/user/myapp.git \
  --domain myapp.example.com \
  --branch main \
  --port 3000

# Remove an app (removes config and updates Caddyfile)
server app remove myapp
```

---

## Installation

### Prerequisites

- Docker and Docker Compose plugin (target server)
- Git (target server)

### Quick Install (recommended)

Download the latest release and install to `/opt/vps/bin/` with symlinks in `/usr/local/bin/`:

```bash
curl -fsSL https://raw.githubusercontent.com/go-sum/vps/main/scripts/install_update.sh | bash
```

This detects your OS and architecture automatically. Run the same command to update to the latest version.

### Build from Source

```bash
git clone https://github.com/go-sum/vps.git
cd vps

# Cross-compile for x86_64 Linux (default)
make build

# Deploy to server
mkdir -p /opt/vps/bin
scp bin/server bin/app user@server:/opt/vps/bin/
ln -s /opt/vps/bin/server /usr/local/bin/server
ln -s /opt/vps/bin/app /usr/local/bin/app
```

---

### Command Reference

#### `server`

| Command | Description |
|---------|-------------|
| `server setup` | Create directory structure, write Caddy config, start Caddy container |
| `server status` | Show Caddy status, network containers, and per-app resource usage with health |
| `server restart` | Restart Caddy and all registered app containers |
| `server app add <name>` | Register an app and update Caddy reverse proxy |
| `server app list` | List all registered apps |
| `server app remove <name>` | Remove an app and update Caddy |

**`server app add` flags:**

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--repo` | Yes | — | Git repository URL |
| `--domain` | Yes | — | Domain name for Caddy |
| `--branch` | No | `main` | Git branch to deploy |
| `--port` | No | `8080` | Upstream container port |
| `--internal-tls` | No | `true` | Use Caddy's internal TLS |

#### `app`

| Command | Description |
|---------|-------------|
| `app setup <app>` | Clone repo, build infrastructure images (db, kv if configured), start services |
| `app run <app>` | Build tools + app image on server, restart, health check, rollback on failure |
| `app pull <app>` | Pull pre-built image from GHCR, fetch artifacts, restart, health check, rollback on failure |
| `app status <app>` | Show deployment state, network connectivity, container resource usage with health |
| `app restart <app>` | Restart app containers, reconnect to Caddy network, run health check |
| `app rollback <app>` | Restore previous image and compose file, restart app, re-run health check |

**`app run` flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--force` | `false` | Deploy even if the same commit SHA is already deployed |
| `--branch` | (from app.yaml) | Override the configured branch for this deploy |

**`app pull` flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--tag` | `latest` | Image tag to pull (e.g., `v1.0`, commit SHA, or `latest`) |

**Global flag (both binaries):**

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `/opt/vps/vps.yaml` | Path to VPS configuration file |

---

## Technologies

| Technology | Purpose |
|------------|---------|
| [Go](https://go.dev/) | CLI implementation, cross-compilation |
| [Docker](https://www.docker.com/) | Container builds and orchestration (BuildKit secrets for private modules) |
| [Docker Compose](https://docs.docker.com/compose/) | Multi-service stack management |
| [Caddy](https://caddyserver.com/) | Reverse proxy with automatic TLS |

---

## Configuration

### VPS Config (`/opt/vps/vps.yaml`)

Created by `server setup`. Controls the directory layout and network settings.

```yaml
base_dir: /opt                # Root directory
caddy_dir: /opt/vps/caddy     # Caddy compose file and Caddyfile
apps_dir: /opt/apps           # Per-app deployment directories
caddy_network: caddy_net      # Docker network connecting Caddy to apps
```

### App Config (`/opt/apps/<name>/app.yaml`)

Created by `server app add`. Controls how the app is built, deployed, and proxied.

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
registry_image: ghcr.io/go-sum/forge                # GHCR image path (required for app pull)
registry_token_env: GHCR_TOKEN                      # Env var name for registry auth (default: GHCR_TOKEN)
```

### Environment Variables

| Variable | Required By | Description |
|----------|-------------|-------------|
| `GITHUB_ACCESS_TOKEN` | `app run`, `app setup` | GitHub PAT for cloning private repos and downloading private Go modules during on-server builds |
| `GHCR_TOKEN` | `app pull` | GitHub PAT with `read:packages` scope for pulling images from GHCR |

### App `.env` File (`/opt/apps/<name>/.env`)

Must be created manually before running `app setup`. Contains production secrets read by Docker Compose and the application:

```env
PGUSER=postgres
PGPASSWORD=secret
PGDATABASE=mydb
# ... application-specific secrets
```

### Repository Requirements

The application repository must contain:

| File/Directory | Purpose |
|----------------|---------|
| `.versions` | Version pins (`GO_VERSION=1.26`, `PG_VERSION=18`, etc.) — all non-empty values passed as `--build-arg` |
| `docker-compose.yml` | Production service definitions (app, db, kv) |
| `docker/app/Dockerfile` | Multi-stage app build with `production_target` target — references pre-built `forge-tools:prod` image for production toolchain |
| `docker/tools/` | Toolchain Dockerfile (`dev_tools` and `production_tools` targets), installation scripts (`dev-tools.sh`, `prod-tools.sh`) |
| `docker/postgres/Dockerfile` | PostgreSQL image build |
| `docker/dragonfly/Dockerfile` | Dragonfly KV image build (only required if `DRAGONFLY_VERSION` is in `.versions`) |
| `db/migrations/` | Versioned SQL migration files (applied by the app on startup when `auto_migrate: true`) |
| `db/init/01-extensions.sql` | PostgreSQL extensions (citext) |

Both `app run` and `app pull` copy the `docker/` directory to the app directory. `app run` builds the production tools image (`docker/tools/Dockerfile` target `production_tools`) on the server before building the app — this image is cached and only rebuilds when Hugo or Tailwind versions change. `app pull` skips all building and pulls a pre-built image from GHCR instead.

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
        ├── .versions_hash          # SHA256 of .versions at last setup
        ├── docker-compose.yml      # Copied from repo at deploy time
        ├── docker-compose.yml.prev # Previous version (for rollback)
        ├── docker/                 # Copied from repo (app/, tools/, postgres/, dragonfly/)
        └── db/                     # Copied from repo
```

---

## Deploy Workflows

Both workflows share the same restart, health check, and rollback logic. They differ only in how the production image is obtained.

### `app run` — Build on Server

1. **Validate** — Check Docker, .env file, GitHub token, running infrastructure (db, kv)
2. **Clone** — Shallow clone (`--depth 1`) of the configured branch to a temp directory
3. **Parse Versions** — Read all `KEY=VALUE` pairs from `.versions`; filter empty values
4. **SHA Check** — Skip if already deployed at this commit (unless `--force`)
5. **Save State** — Record current image ID and compose file (`.prev`) for rollback
6. **Build Tools** — `docker build --target production_tools` from `docker/tools/Dockerfile` — cached unless Hugo or Tailwind versions change
7. **Build App** — `docker build --target production_target` from `docker/app/Dockerfile` with version build-args, tools image reference, and GitHub token as a BuildKit secret
8. **Tag** — Tag new image as `<app>:<sha>` and `<app>:latest`
9. **Copy Artifacts** — Copy compose file, `docker/`, and `db/` to app directory
10. **Restart** — First deploy: `docker compose up -d`; subsequent: `docker compose up -d --force-recreate --no-deps app`
11. **Network** — Connect app container to Caddy network
12. **Health Check** — Poll health URL up to `health_retries` times at 2s intervals with 5s timeout per request
13. **Record** — Write deployed SHA; clear rollback state
14. **Rollback** (on failure) — Show last 50 lines of app logs, restore previous image and compose file, restart

### `app pull` — Pull from Registry

1. **Validate** — Check Docker, `registry_image` configured, `GHCR_TOKEN` set, running infrastructure (db, kv)
2. **Login** — Authenticate to GHCR via `docker login --password-stdin`
3. **Pull** — `docker pull ghcr.io/<owner>/<app>:<tag>`
4. **Save State** — Record current image ID and compose file (`.prev`) for rollback
5. **Tag** — Tag pulled image as `<app>:latest`
6. **Fetch Artifacts** — Sparse git checkout of `docker-compose.yml`, `docker/`, `db/`, `.versions` (~100KB)
7. **Copy Artifacts** — Copy fetched files to app directory
8. **Parse Versions** — Read `.versions` for compose environment
9. **Restart** — Same as `app run` step 10
10. **Network** — Connect app container to Caddy network
11. **Health Check** — Same as `app run` step 12
12. **Record** — Write deployed tag; clear rollback state
13. **Rollback** (on failure) — Same as `app run` step 14

### GitHub Actions CI Build

The repository includes a workflow (`.github/workflows/build-image.yml`) that builds the production image and pushes it to GHCR. It is triggered by:

- **Manual dispatch** (`workflow_dispatch`) — with an optional `tag` input
- **Tag push** (`v*`) — uses the git tag as the image tag

The workflow builds the same `production_target` as `app run`, tags the image as both `:<tag>` and `:latest`, and cleans up old versions (keeps the last 5).

**Required GitHub Secrets:**

| Secret | Description |
|--------|-------------|
| `PACKAGES_TOKEN` | GitHub PAT with `read:packages` (private Go modules) + `write:packages` (GHCR push) |

`GITHUB_TOKEN` handles GHCR authentication for push. 
`PACKAGES_TOKEN` is passed as a BuildKit secret for downloading private Go modules during the build.

Migrations are handled by the application itself on startup when `auto_migrate: true` is set in its config — the deploy tool does not run migrations directly.

---

## GitHub Quick Reference

### Creating a Personal Access Token (PAT)

Go to **Settings > Developer settings > Personal access tokens > Fine-grained tokens** and create two tokens:

1. **`PACKAGES_TOKEN`** (for CI / `app run`) — scopes: `read:packages`, `write:packages`
2. **`GHCR_TOKEN`** (for `app pull`) — scope: `read:packages` only

See [Managing your personal access tokens](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens) for detailed instructions.

### Adding Repository Secrets

Go to your repo **Settings > Secrets and variables > Actions > New repository secret** and add `PACKAGES_TOKEN` with the CI token value.

See [Using secrets in GitHub Actions](https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions) for detailed instructions.

### Triggering a Workflow Manually

Go to your repo **Actions** tab, select the **Build and Push Production Image** workflow, click **Run workflow**, optionally enter a tag, and click the green **Run workflow** button.

See [Manually running a workflow](https://docs.github.com/en/actions/managing-workflow-runs-and-deployments/managing-workflow-runs/manually-running-a-workflow) for detailed instructions.

### Pushing a Tag to Trigger a Build

```bash
git tag v1.0.0
git push origin v1.0.0
# or use
make release 
```

This triggers the workflow automatically and tags the image as `v1.0.0`. 

See [Git Basics - Tagging](https://git-scm.com/book/en/v2/Git-Basics-Tagging) for more on tag management.
