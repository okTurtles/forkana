# Forkana Deployment Guide

This guide covers deploying Forkana to a single Linux VM for private dev instance. It assumes you have SSH access to a fresh Linux server and basic Docker knowledge.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Server Preparation](#server-preparation)
3. [Server Setup (CI/CD)](#server-setup-cicd)
4. [Environment Configuration](#environment-configuration)
5. [Running Forkana](#running-forkana)
6. [Reverse Proxy Setup](#reverse-proxy-setup)
7. [Health Checks & Verification](#health-checks--verification)
8. [Troubleshooting](#troubleshooting)
9. [Maintenance](#maintenance)

---

## Prerequisites

<details>

### Server Requirements

- **OS:** Ubuntu 22.04 LTS, Debian 12, or Fedora 39+ (recommended)
- **CPU:** 2+ cores
- **RAM:** 4GB minimum (8GB recommended)
- **Disk:** 40GB+ SSD
- **Network:** Public IP with ports 80, 443 open

### Software Requirements

- Docker 24.0+ with Docker Compose v2
- `curl`, `jq` (for deploy scripts and initial setup)
- A domain name pointing to your server's IP

> **Note:** The deployment script (`deploy.sh`) auto-detects the host OS
> via `/etc/os-release` and delegates to the appropriate OS-specific script.
> After initial server setup, deployments work identically on all supported
> platforms.

</details>

---

## Server Preparation

<details>

### 1. Update System Packages

#### Debian/Ubuntu

```bash
sudo apt update && sudo apt upgrade -y
```

#### Fedora

```bash
sudo dnf upgrade --refresh -y
```

### 2. Install Docker

#### Debian/Ubuntu

```bash
# Install Docker using the official script
curl -fsSL https://get.docker.com | sudo sh

# Add your user to the docker group
sudo usermod -aG docker $USER

# Log out and back in, then verify
docker --version
docker compose version
```

#### Fedora

```bash
# Add Docker's official repo (DNF5 syntax for Fedora 41+)
sudo dnf config-manager addrepo \
  --from-repofile=https://download.docker.com/linux/fedora/docker-ce.repo

# Install Docker
sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin

# Start and enable Docker
sudo systemctl start docker
sudo systemctl enable docker

# Add your user to the docker group
sudo usermod -aG docker $USER

# Log out and back in, then verify
docker --version
docker compose version
```

### 3. Install Additional Dependencies

#### Debian/Ubuntu

```bash
sudo apt install -y git jq curl
```

#### Fedora

```bash
sudo dnf install -y git jq curl
```

### 4. Create Directory Structure

> **CI/CD note:** If you are using the CI/CD deployment model (next section),
> skip this step - the deploy user's directories are created there instead.

```bash
mkdir -p ~/forkana/{compose,data,data/git,data/custom,config,postgres,images}
chmod 0755 ~/forkana/data ~/forkana/data/git ~/forkana/data/custom \
           ~/forkana/config ~/forkana/images
```

> **Note:** `postgres/` is intentionally omitted from `chmod` - the
> postgres container takes ownership of its data directory on first
> start (UID 999 / 70) and a later `chmod 0755` by the host user
> would break subsequent startups. See
> [UID 1000 Permission Issues](#uid-1000-permission-issues-bind-mount-directories)
> in Troubleshooting.

</details>

---

## Server Setup (CI/CD)

This section configures the VM for CI/CD deployment. GitHub Actions builds
the Docker image, transfers it to the VPS as a pre-built tarball, and
triggers deployment via SSH. The VPS loads the image, pushes it to a local
Docker registry, and deploys with a digest-pinned compose override.

### Deployment Flow

Every push to the configured deploy branch (or a manual
`workflow_dispatch`) runs the pipeline below. Steps 1-3 execute on
the GitHub Actions runner; steps 4-6 execute on the VM under the
deploy user via `~/forkana/deploy.sh` (which sources
`deploy_common.sh`).

1. **Build.** The runner builds the Docker image from the checked-out
   commit and exports it as a gzipped tarball
   (`docker save forkana:${sha} | gzip > forkana-${sha:0:7}.tar.gz`).
2. **Transfer.** The tarball is copied to the VM with SCP, landing at
   `~/forkana/images/forkana-<7-char-sha>.tar.gz`.
3. **Trigger.** The runner opens an SSH connection with the deploy
   key; `authorized_keys` pins a `command="~/forkana/deploy.sh"`
   forced-command restriction (see
   [step 5](#5-configure-authorized_keys-with-forced-command-restrictions)),
   so every session runs the deploy script regardless of what the
   client requests. The commit SHA is passed via
   `$SSH_ORIGINAL_COMMAND`.
4. **Load & push.** `deploy_common.sh` `docker load`s the tarball and
   pushes the image to a local registry at
   `127.0.0.1:${REGISTRY_PORT}` (loopback-bound, not publicly
   reachable). The push output yields the registry digest that gets
   pinned in step 5.
5. **Pin.** The script rewrites `~/forkana/compose/compose.override.yml`
   to pin the `forkana` service to the push digest
   (`image: 127.0.0.1:${REGISTRY_PORT}/forkana@sha256:…`) and runs
   `docker compose up -d` on the pinned override. Registry and
   postgres are left alone unless their definitions changed.
6. **Verify.** The script polls
   `http://127.0.0.1:${FORKANA_HOST_PORT}/api/healthz` for up to
   150 s. On success the previous override snapshot is removed and
   the workflow run is marked green. On failure the script restores
   the previous override from `compose.override.yml.prev`, re-runs
   compose for `forkana` only, and exits 1 - see
   [Automatic Rollback on Health-Check Failure](#automatic-rollback-on-health-check-failure)
   in Maintenance.

The numbered steps that follow configure the VM so this pipeline can
run end-to-end.

<details>

### 1. Identify or Create the Deploy User (UID 1000)

The deploy user **must** have UID 1000 so that directories it creates are
automatically owned by the container user (git:git, also UID 1000).

You do **not** need to create a user specifically named `forkana-deploy`.
On many servers, UID 1000 is already assigned to the first normal user
created during OS setup. Check with:

```bash
getent passwd 1000
```

If that returns a user, you can use it as the deploy user - just add it to the
`docker` group and use its username wherever `forkana-deploy` appears in this
guide.

> **Important:** If UID 1000 is taken by a user you cannot repurpose, you must
> resolve that conflict before continuing - either remove or reassign the
> existing user. The container bind-mounts depend on UID 1000 ownership.

If no UID 1000 user exists, create one:

#### Debian/Ubuntu

```bash
sudo adduser --system --group --shell /bin/bash --uid 1000 \
  --home /home/forkana-deploy forkana-deploy
sudo usermod -aG docker forkana-deploy
```

#### Fedora

```bash
sudo useradd --create-home --shell /bin/bash --uid 1000 \
  --home-dir /home/forkana-deploy forkana-deploy
sudo usermod -aG docker forkana-deploy
```

> **Note:** Throughout this guide, `forkana-deploy` is used as a placeholder
> name. Substitute your actual UID 1000 username wherever it appears.

### 2. Verify/Fix Deploy User Home Directory

Some distributions may not create the home directory, or may leave it owned
by root. Run these commands as root to ensure the home directory exists with
correct ownership:

```bash
DEPLOY_USER="forkana-deploy"  # ← replace with your UID 1000 username

# Determine the home directory from the passwd entry
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"
echo "Deploy user home: $DEPLOY_HOME"

# Create the home directory if missing, set correct ownership/permissions
sudo install -d -o "${DEPLOY_USER}" -g "${DEPLOY_USER}" -m 0755 "$DEPLOY_HOME"

# Verify
ls -ld "$DEPLOY_HOME"
# Expected: drwxr-xr-x ... <deploy-user> <deploy-user> ... /home/<deploy-user>
```

### 3. Set Up the Deploy Directory

All paths live under the deploy user's home directory - no root-owned
directories and no `sudo` required during deployments.

The repository is **not** cloned on the server. GitHub Actions builds the
Docker image and transfers it as a pre-built tarball. The VPS only needs
the directory structure and deploy scripts.

```bash
DEPLOY_USER="forkana-deploy"  # ← replace with your UID 1000 username
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"

sudo -Hiu "${DEPLOY_USER}" bash -lc "
  mkdir -p $DEPLOY_HOME/forkana/{compose,data,data/git,data/custom,config,postgres,images}
  chmod 0755 $DEPLOY_HOME/forkana/data $DEPLOY_HOME/forkana/data/git \
             $DEPLOY_HOME/forkana/data/custom $DEPLOY_HOME/forkana/config \
             $DEPLOY_HOME/forkana/images
"
```

### 4. Install the Deploy Scripts

`deploy.sh` is an OS-detecting wrapper that delegates to `deploy_debian.sh` or
`deploy_fedora.sh` (which both source `deploy_common.sh`). `cleanup-images.sh`
prunes old tarballs from `~/forkana/images` and is scheduled via cron (see
[Maintenance](#cleanup-old-image-tarballs)). Download them from the
repository:

```bash
DEPLOY_USER="forkana-deploy"  # ← replace with your UID 1000 username
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"
BRANCH="master"               # keep in sync with vars.DEPLOY_BRANCH if set (see step 8)
BASE_URL="https://raw.githubusercontent.com/okTurtles/forkana/${BRANCH}/docker/forkana"

for script in deploy.sh deploy_common.sh deploy_debian.sh deploy_fedora.sh cleanup-images.sh; do
  sudo -Hiu "${DEPLOY_USER}" curl -fsSL \
    "${BASE_URL}/${script}" -o "$DEPLOY_HOME/forkana/${script}"
done
sudo -Hiu "${DEPLOY_USER}" chmod 755 \
  "$DEPLOY_HOME/forkana"/deploy*.sh "$DEPLOY_HOME/forkana/cleanup-images.sh"
```

> **Note:** The compose base file is `~/forkana/compose/dev.yml`.
> Runtime overrides and secrets stay in `~/forkana/compose/`
> (`compose.override.yml` and `.env`). The deploy scripts themselves are
> **not** updated automatically - to update them, re-run the download step
> above. `deploy_common.sh` rewrites
> `~/forkana/compose/compose.override.yml` with the pinned image digest
> on each deployment.

### 5. Configure `authorized_keys` with Forced-Command Restrictions

Create the deploy user's `authorized_keys` with the GitHub Actions public
key. The `command=` directive restricts the key to running the deploy script
only:

```bash
DEPLOY_USER="forkana-deploy"  # replace with your UID 1000 username
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"

# Create .ssh directory with correct ownership and permissions
sudo install -d -o "${DEPLOY_USER}" -g "${DEPLOY_USER}" -m 0700 "$DEPLOY_HOME/.ssh"
```

Add the following single line to `$DEPLOY_HOME/.ssh/authorized_keys` (replace
`ssh-ed25519 AAAA...` with the actual public key):

```
command="~/forkana/deploy.sh",restrict ssh-ed25519 AAAA... github-actions-deploy
```

```bash
# Set correct ownership and permissions on authorized_keys
sudo touch "$DEPLOY_HOME/.ssh/authorized_keys"
sudo chown "${DEPLOY_USER}:${DEPLOY_USER}" "$DEPLOY_HOME/.ssh/authorized_keys"
sudo chmod 600 "$DEPLOY_HOME/.ssh/authorized_keys"
```

**Security notes:**
- `command=` forces every SSH session with this key to execute `deploy.sh`.
  The commit SHA is available to the script via `$SSH_ORIGINAL_COMMAND`.
- `restrict` disables port forwarding, PTY allocation, agent forwarding,
  and X11 forwarding - preventing tunnelling, interactive shells, and
  agent hijacking.

### 6. Start the Local Docker Registry

The registry is managed by `docker compose` via `dev.yml`. For the initial
bootstrap (before the first deploy), start it manually:

```bash
DEPLOY_USER="forkana-deploy"  # replace with your UID 1000 username
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"
BRANCH="master"               # keep in sync with vars.DEPLOY_BRANCH if set (see step 8)

sudo -Hiu "${DEPLOY_USER}" curl -fsSL \
  "https://raw.githubusercontent.com/okTurtles/forkana/${BRANCH}/docker/forkana/dev.yml" \
  -o "$DEPLOY_HOME/forkana/compose/dev.yml"

sudo -Hiu "${DEPLOY_USER}" docker compose -f "$DEPLOY_HOME/forkana/compose/dev.yml" up -d registry
```

Verify it is running:

```bash
curl -sf http://127.0.0.1:5000/v2/ && echo "Registry OK"
```

The registry binds to `127.0.0.1:5000` only and is **not** publicly
accessible. Data persists in the `registry-data` Docker volume.

### 7. Obtain the SSH Host Key for GitHub

Pin the VM's SSH host key in the GitHub Actions workflow to prevent MITM
attacks. Run on the VM:

```bash
# Print the host key entry (use the key type matching your server, usually ed25519)
ssh-keyscan -t ed25519 <VM_IP_OR_HOSTNAME> 2>/dev/null
```

Copy the output and store it as the `DEPLOY_SSH_KNOWN_HOSTS` secret in the
GitHub repository settings. The workflow uses `StrictHostKeyChecking=yes` with
this value - **never** use `StrictHostKeyChecking=no`.

### 8. Required GitHub Secrets and Variables

| Secret | Description                                                  |
|---|--------------------------------------------------------------|
| `DEPLOY_HOST` | VM IP address or hostname                                    |
| `DEPLOY_USER` | Username of the UID 1000 deploy user (e.g. `forkana-deploy`) |
| `DEPLOY_SSH_KEY` | Private key (ed25519 recommended) - see workflow below       |
| `DEPLOY_SSH_KNOWN_HOSTS` | Output of `ssh-keyscan` from step 7                          |

**Optional repository variables:**

| Variable | Description                                                                                                                                                                                                                                                        |
|---|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `DEPLOY_BRANCH` | If set, the workflow's preflight guard rejects `push`-triggered runs whose branch name does not match this value. Leave unset to deploy from whatever branches are listed under `on.push.branches` in the workflow. Keep aligned with the `BRANCH=` values used in steps 4 and 6. |

Repository variables are configured under **Settings > Secrets and variables > Actions > Variables** (distinct from the *Secrets* tab above).

**SSH key workflow:**

1. Generate a dedicated key pair locally:
   ```bash
   ssh-keygen -t ed25519 -C "github-actions-deploy" -f deploy_key -N ""
   ```
2. Install the **public** key (`deploy_key.pub`) on the server - see
   [step 5](#5-configure-authorized_keys-with-forced-command-restrictions).
3. Copy the **private** key contents into GitHub: go to **Settings > Secrets
   and variables > Actions > New repository secret**, name it
   `DEPLOY_SSH_KEY`, and paste the full contents of `deploy_key`.
4. The workflow YAML loads `DEPLOY_SSH_KEY` at deploy time to SSH into the
   server.

> **Security:** Only the public key goes on the server; only the private key
> goes into GitHub Secrets. Never commit the private key to the repository.
> Delete the local `deploy_key` file after storing it in GitHub Secrets.
>
> See [Using secrets in GitHub Actions](https://docs.github.com/en/actions/how-tos/write-workflows/choose-what-workflows-do/use-secrets).

</details>

---

## Environment Configuration

### Create Environment File

Create `$DEPLOY_HOME/forkana/compose/.env` with the commands below. Compose reads `.env`
from the project directory automatically.

The file requires **five** variables (plus one optional):

| Variable | Description |
|----------|-------------|
| `POSTGRES_PASSWORD` | PostgreSQL password (random, never reused) |
| `FORKANA_DOMAIN` | Your domain without `https://` prefix |
| `FORKANA_SECRET_KEY` | 64-char hex key for session encryption |
| `FORKANA_INTERNAL_TOKEN` | Internal API token (base64, 32+ chars) |
| `FORKANA_JWT_SECRET` | OAuth2 JWT signing secret (base64, 32+ chars) |
| `FORKANA_HOST_PORT` | *(optional)* Host port for the Forkana service (default: `3000`) |

For an external database setup (optional), you can also set:

| Variable | Description |
|----------|-------------|
| `FORKANA_DB_HOST` | External Postgres host:port (for example `host.docker.internal:5432` or `db.example.com:5432`) |
| `FORKANA_DB_NAME` | Database name (defaults to `forkana`) |
| `FORKANA_DB_USER` | Database user (defaults to `forkana`) |
| `FORKANA_DB_PASSWORD` | Database password for the external Postgres user |
| `FORKANA_DB_SSL_MODE` | Postgres SSL mode (`disable`, `require`, etc.; defaults to `disable`) |

Generate the file (replace `dev.forkana.org` with your actual domain):

```bash
DEPLOY_USER="forkana-deploy"  # replace with your UID 1000 username
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"

# Create .env with generated secrets (overwrites any existing file).
# openssl rand produces single-line output; tr -d '\n' guards against any
# trailing newline so values never wrap and break .env parsing.
{
  printf 'POSTGRES_PASSWORD=%s\n'      "$(openssl rand -base64 24 | tr -d '\n')"
  printf 'FORKANA_DOMAIN=%s\n'         "dev.forkana.org"
  printf 'FORKANA_SECRET_KEY=%s\n'     "$(openssl rand -hex 32)"
  printf 'FORKANA_INTERNAL_TOKEN=%s\n' "$(openssl rand -base64 32 | tr -d '\n')"
  printf 'FORKANA_JWT_SECRET=%s\n'     "$(openssl rand -base64 32 | tr -d '\n')"
} > "$DEPLOY_HOME/forkana/compose/.env"

# Lock down permissions - only the deploy user should read this file
chmod 600 "$DEPLOY_HOME/forkana/compose/.env"
```

Verify the result:

```bash
# Verify the file exists and has correct permissions
ls -l $DEPLOY_HOME/forkana/compose/.env

# Verify all required keys are present without revealing values
for key in POSTGRES_PASSWORD FORKANA_DOMAIN FORKANA_SECRET_KEY FORKANA_INTERNAL_TOKEN FORKANA_JWT_SECRET; do
  grep -q "^${key}=" "$DEPLOY_HOME/forkana/compose/.env" || echo "Missing ${key}"
done
```

---

## Running Forkana

Automated deployments are handled by `deploy.sh` (triggered via GitHub Actions).
For manual operations, use:
- base file: `~/forkana/compose/dev.yml`
- override file: `~/forkana/compose/compose.override.yml`
- env file: `~/forkana/compose/.env`
- optional logging override: `~/forkana/compose/logging.override.yml`

Optional compose override files (e.g. `external-postgres.override.yml`,
`logging.override.yml`) are not part of the automated deployment. Copy them
from the repository's `docker/forkana/overrides/` directory to
`~/forkana/compose/` on the VM once (e.g. via `curl` from raw GitHub, or
`scp` from a local checkout) before layering them into the manual commands
below. See `docker/forkana/overrides/README.md` in the repository for
ready-made examples.

> **User context:** All commands in this section (and in Health Checks,
> Maintenance, and most of Troubleshooting) should be run **as the deploy
> user** (your UID 1000 user). Either log in as that user directly or prefix
> your session with `sudo -iu <deploy-user>`. This ensures `~` expands to
> the deploy user's home directory.

### Start the Services

> **Note:** When running commands via `sudo -Hiu <deploy-user>`, Docker Compose
> may not auto-load the `.env` file. Use `--env-file` explicitly:
> ```bash
> docker compose --env-file ~/forkana/compose/.env -f ~/forkana/compose/dev.yml -f ~/forkana/compose/compose.override.yml up -d
> ```

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  up -d
```

### Use an Existing Postgres (Optional)

Use `overrides/external-postgres.override.yml` to point Forkana at an existing Postgres
instance and keep the bundled `postgres` service disabled by default.

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/external-postgres.override.yml \
  up -d
```

If you also want to start the bundled `postgres` service for debugging, enable
its profile explicitly:

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/external-postgres.override.yml \
  --profile local-db \
  up -d
```

### Use a Different Docker Logging Driver (Optional)

`dev.yml` defaults to `json-file` with size-based rotation
(`max-size: 50m`, `max-file: 14`, `compress: true`) as a safe baseline.
The default `50m` and `14` align with the daemon-level rotation
documented in
`custom/docs/ongoing/deploy/ci-cd/done/research/log-rotation.md`, so an
unmodified deploy persists roughly 14 × 50 MB of rotated logs per
service in `/var/lib/docker/containers/<id>/`. Override the values via
`DOCKER_LOG_MAX_SIZE` / `DOCKER_LOG_MAX_FILE` in `.env` if you need a
different retention budget.

If you need a different driver entirely, include
`overrides/logging.override.yml` and set `DOCKER_LOG_DRIVER` in your `.env` or shell.
See `docker/forkana/overrides/README.md` in the repository for `local`,
`journald`, and `fluentd` examples.

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/logging.override.yml \
  up -d --force-recreate
```

Important when switching drivers:
- Keep the same compose file set for `up`, `down`, `logs`, and `exec`.
- Recreate containers after driver changes (`--force-recreate`) so Docker
  applies the new logging driver.
- `json-file` options (`max-size`, `max-file`, `compress`) are driver-specific;
  if you use another driver, configure only options supported by that driver
  inside `overrides/logging.override.yml`.

To revert to the default logger, remove `overrides/logging.override.yml` from the command
(or set `DOCKER_LOG_DRIVER=json-file` and add json-file options explicitly in
`overrides/logging.override.yml`).

### Persisting Rotated Logs Across Redeploys

Docker stores `json-file` logs under
`/var/lib/docker/containers/<container-id>/`. When a container is
recreated (e.g. by every CI/CD deploy that re-runs `docker compose up`)
the old container's directory is eligible for removal, which also
removes its rotated log files. The Forkana defaults bound on-disk
size per container but do **not** retain history across deploys on
their own.

If you need rotated logs to survive deploys, pick one of the
strategies in
`custom/docs/ongoing/deploy/ci-cd/done/research/log-rotation.md`
(daemon-level rotation in `/etc/docker/daemon.json`, periodic
`docker logs --since` exports, or shipping to Loki / fluent-bit).
The compose-level defaults here are intentionally minimal so they
compose cleanly with whichever option you choose.



```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  ps
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  logs -f forkana
```

### Initialize the Database (First Run Only)

On first startup, Forkana will automatically:
1. Create the database schema
2. Generate internal tokens
3. Set up default configuration

Watch the logs to ensure initialization completes:

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  logs -f forkana | grep -i "starting"
```

#### Create admin user

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  exec forkana gitea admin user create \
  --username admin --password your-password-here --email admin@forkana.org --admin
```

---

## Reverse Proxy Setup

### Install Nginx and Certbot

#### Debian/Ubuntu

```bash
sudo apt install -y nginx certbot python3-certbot-nginx
```

#### Fedora

```bash
sudo dnf install -y nginx certbot python3-certbot-nginx
```

### Deploy the Nginx Configuration

The repository includes a ready-made Nginx configuration at
[`docker/forkana/nginx.conf`](nginx.conf) - treat it as the canonical source
rather than copy-pasting server blocks into this guide. Download it to the
VPS with `curl`, substitute `dev.forkana.example` for your actual domain, and
adjust the `proxy_pass` port if you changed `FORKANA_HOST_PORT` in `.env`.

For the initial setup, deploy only the HTTP server block. Certbot will add
the HTTPS block automatically when obtaining the certificate.

```bash
FORKANA_DOMAIN="your-actual-domain.com"
BRANCH="master"
NGINX_RAW_URL="https://raw.githubusercontent.com/okTurtles/forkana/${BRANCH}/docker/forkana/nginx.conf"

# Stage the reference config locally, then strip to the first server { ... }
# block for the Certbot bootstrap (Certbot rewrites the file to add the HTTPS
# block). The sed range extracts from the first `server {` line to its
# matching closing `}` and quits, so reformatting nginx.conf (extra comments,
# blank lines, additional directives inside the HTTP block) does not break
# this step.
curl -fsSL "${NGINX_RAW_URL}" -o /tmp/forkana.nginx.conf
sed -n '/^server {/,/^}/{p;/^}/q;}' /tmp/forkana.nginx.conf \
  | sed "s/dev.forkana.example/${FORKANA_DOMAIN}/g" > /tmp/forkana.nginx.http.conf
```

#### Fedora

Fedora uses `/etc/nginx/conf.d/` instead of `sites-available/sites-enabled`:

```bash
sudo install -m 0644 /tmp/forkana.nginx.http.conf /etc/nginx/conf.d/forkana.conf

# Fedora + SELinux: allow nginx to proxy to the Forkana host port
sudo setsebool -P httpd_can_network_connect 1
```

#### Debian/Ubuntu

```bash
sudo install -m 0644 /tmp/forkana.nginx.http.conf /etc/nginx/sites-available/forkana
sudo ln -sf /etc/nginx/sites-available/forkana /etc/nginx/sites-enabled/forkana
```

> **After Certbot:** Once you have a certificate, replace the site file with
> the full `/tmp/forkana.nginx.conf` (still domain-substituted) so the HSTS,
> security headers, and WebSocket/`proxy_pass` tuning from `nginx.conf` take
> effect. Certbot leaves the SSL directives it injected earlier intact.

### Enable the Site and Obtain SSL Certificate

```bash
# Test configuration
sudo nginx -t

# Enable and start nginx
sudo systemctl enable nginx
sudo systemctl start nginx

# Obtain SSL certificate (Certbot will automatically update the nginx config)
sudo certbot --nginx -d "$FORKANA_DOMAIN"

# Enable automatic renewal timer (needed on some distros/images)
sudo systemctl enable --now certbot-renew.timer
```

Certbot will automatically:
1. Obtain the SSL certificate
2. Add the HTTPS server block with proper SSL configuration
3. Set up HTTP-to-HTTPS redirect

---

## Health Checks & Verification

<details>

### Verify Application Health

```bash
# Check the health endpoint (use your configured FORKANA_HOST_PORT, default 3000)
curl -f http://localhost:${FORKANA_HOST_PORT:-3000}/api/healthz

# Check via HTTPS (after proxy setup)
curl -f https://your-domain.example/api/healthz
```

### Verify Database Connection

For the default (bundled Postgres) setup:

```bash
# Check PostgreSQL is accessible
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  exec postgres pg_isready -U forkana -d forkana

# Check Forkana can connect
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  logs forkana | grep -i database
```

For an existing external Postgres (with `overrides/external-postgres.override.yml`):

```bash
# Confirm Forkana is using the external DB host override
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/external-postgres.override.yml \
  exec forkana printenv GITEA__database__HOST

# Check Forkana DB connection messages
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/external-postgres.override.yml \
  logs forkana | grep -i database
```

### Verify All Services

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  ps

# Expected output (default local DB; ports depend on FORKANA_HOST_PORT and REGISTRY_PORT):
# NAME               STATUS                   PORTS
# forkana            Up (healthy)             127.0.0.1:<host-port>->3000/tcp
# forkana-postgres   Up (healthy)             5432/tcp
# registry           Up (healthy)             127.0.0.1:<registry-port>->5000/tcp

# With overrides/external-postgres.override.yml, forkana-postgres is typically absent
# unless you explicitly enable --profile local-db.
```

### Test Web Access

1. Open `https://your-domain.example` in a browser
2. You should see the Forkana login page
3. Login with the admin account created earlier

</details>

---

## Troubleshooting

<details>

### View Logs

All compose commands below use explicit paths for `dev.yml`,
`compose.override.yml`, and `.env`.

```bash
# All services
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  logs -f

# Forkana only
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  logs -f forkana

# PostgreSQL only (bundled DB mode)
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  logs -f postgres

# Forkana logs (external DB mode)
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/external-postgres.override.yml \
  logs -f forkana

# Last 100 lines
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  logs --tail=100 forkana
```

### Common Issues

#### `systemctl` Command Not Found or "Failed to connect to bus"

**Symptoms:**
```
System has not been booted with systemd as init system (PID 1).
Can't operate.
```

**Cause:** You're running in a container or minimal VM without systemd.

**Solution:** This is expected in containers. On a real VPS, systemd is the default init system. If testing in a container, skip `systemctl` commands and manage services directly:

```bash
# Instead of: sudo systemctl start docker
sudo dockerd &

# Instead of: sudo systemctl reload nginx
sudo nginx -s reload
```

> **Note:** Full deployment testing requires a real VPS with systemd. Containers can only validate package installation, user creation, and script logic.

#### Docker Daemon Not Running

**Symptoms:**
```
Cannot connect to the Docker daemon at unix:///var/run/docker.sock.
Is the docker daemon running?
```

**Cause:** Docker daemon is not started (common in containers without Docker-in-Docker).

**Solution:**

On a VPS:
```bash
sudo systemctl start docker
sudo systemctl enable docker
```

In a container (testing only):
```bash
# Start Docker daemon in background
sudo dockerd &

# Wait for daemon to be ready
sleep 5
docker info
```

#### Container Won't Start

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  logs forkana | tail -50
```

```bash
# Verify permissions (run as root — ~/forkana would expand to /root)
DEPLOY_USER="forkana-deploy"  # replace with your UID 1000 username
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"

ls -la $DEPLOY_HOME/forkana/data
ls -la $DEPLOY_HOME/forkana/config
ls -la $DEPLOY_HOME/forkana/postgres

# Fix permissions if needed (deploy user must be UID 1000).
# Do NOT include $DEPLOY_HOME/forkana/postgres here: after the first
# deployment that directory is owned by the postgres container's
# internal user (UID 999 on debian-based postgres, 70 on alpine).
# Chowning it back to 1000:1000 will break postgres startup.
chown -R 1000:1000 $DEPLOY_HOME/forkana/data $DEPLOY_HOME/forkana/config
```

#### Deploy User Commands Fail with "Permission denied"

If `sudo -u <deploy-user>` commands fail with errors like
`mkdir: cannot create directory '/home/<deploy-user>': Permission denied`,
the deploy user's home directory is either missing or owned by root. This is
common on Fedora with system users. Run the
[Verify/Fix Deploy User Home Directory](#2-verifyfix-deploy-user-home-directory)
step to repair it:

```bash
DEPLOY_USER="forkana-deploy"  # replace with your UID 1000 username
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"
sudo install -d -o "${DEPLOY_USER}" -g "${DEPLOY_USER}" -m 0755 "$DEPLOY_HOME"
```

#### UID 1000 Permission Issues (Bind-Mount Directories)

**Symptoms:**
```
forkana: permission denied
mkdir: cannot create directory '/data/git': Permission denied
```

**Cause:** Forkana containers run as UID 1000 (git user). All bind-mounted directories must be owned by UID 1000, or the container will fail to write.

**Diagnosis:**
```bash
DEPLOY_USER="forkana-deploy"  # replace with your UID 1000 username
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"

# Check directory ownership
ls -la $DEPLOY_HOME/forkana/data $DEPLOY_HOME/forkana/config $DEPLOY_HOME/forkana/postgres

# Expected: data/ and config/ owned by UID 1000; postgres/ owned by
# the postgres container's internal user after the first deployment
# (UID 999 on debian-based postgres, 70 on alpine). That is normal
# and must not be changed back to 1000:1000.
# drwxr-xr-x  1000  1000  ... data/
# drwxr-xr-x  1000  1000  ... config/
# drwx------   999   999  ... postgres/
```

**Solution:**
```bash
DEPLOY_USER="forkana-deploy"  # replace with your UID 1000 username
DEPLOY_HOME="$(getent passwd "${DEPLOY_USER}" | cut -d: -f6)"

# Fix ownership on Forkana-managed bind-mounts only. Leave
# $DEPLOY_HOME/forkana/postgres untouched: it is managed by the
# postgres container's internal user (UID 999 / 70) after initdb.
sudo chown -R 1000:1000 $DEPLOY_HOME/forkana/data $DEPLOY_HOME/forkana/config

# Verify the deploy user has UID 1000
getent passwd "${DEPLOY_USER}" | cut -d: -f3
# Should output: 1000
```

> **Why UID 1000?** The container's `git` user is UID 1000. The deploy user is created with UID 1000 so directories it creates are automatically accessible to the container.

#### Database Connection Failed

For the default (bundled Postgres) setup:

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  ps postgres
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  logs postgres

# Test connection manually
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  exec postgres psql -U forkana -d forkana -c "SELECT 1;"
```

For external Postgres mode (`overrides/external-postgres.override.yml`):

```bash
# Confirm the DB host/credentials are wired into the running forkana container
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/external-postgres.override.yml \
  exec forkana printenv GITEA__database__HOST GITEA__database__NAME GITEA__database__USER

# Inspect DB-related startup/runtime errors from forkana
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/external-postgres.override.yml \
  logs forkana | grep -Ei "database|postgres|sql|pq|connection"
```

#### 502 Bad Gateway from Nginx

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  ps forkana
curl http://localhost:${FORKANA_HOST_PORT:-3000}/api/healthz

# Check Nginx error logs
sudo tail -f /var/log/nginx/error.log
```

#### Fedora-Specific: Nginx Proxy Permission Denied

**Symptoms:**
```
connect() to 127.0.0.1:3000 failed (13: Permission denied) while connecting to upstream
```

**Cause:** SELinux blocks nginx from making network connections by default.

**Solution:**
```bash
# Enable nginx to connect to network services
sudo setsebool -P httpd_can_network_connect 1
sudo systemctl restart nginx
```

#### SSL Certificate Issues

```bash
sudo certbot renew --dry-run
sudo certbot certificates
```

#### CSRF Failures, Auth Errors, or Redirect Loops Behind Nginx

**Symptoms:**
- Login forms return 400 Bad Request (CSRF token mismatch)
- Redirect loops between HTTP and HTTPS
- All requests in Forkana logs show the Docker gateway IP instead of real client IPs

**Cause:** Forkana uses the `REVERSE_PROXY_TRUSTED_PROXIES` setting to decide
which IPs are allowed to set `X-Forwarded-For` and `X-Forwarded-Proto` headers.
If the nginx reverse proxy's traffic arrives from a Docker network IP that falls
outside the trusted list, Forkana silently ignores those headers. This breaks
scheme detection (Forkana thinks the request is HTTP when it's actually HTTPS),
which in turn breaks CSRF validation and can cause redirect loops.

Fresh deployments should not hit this issue: the Docker network subnet is pinned
to `172.30.0.0/16` in `dev.yml`, and the `app.ini` template
trusts exactly that subnet (`127.0.0.0/8,::1/128,172.30.0.0/16`). However,
instances deployed before the subnet was pinned will have a Docker-assigned
subnet (e.g. `172.18.0.0/16`) that may not match the trusted proxy list.
Existing Docker networks are not retroactively updated by Docker Compose - the
network must be recreated for the pinned subnet to take effect.

**Diagnosis:**

```bash
# Find the subnet Docker assigned to the forkana network
docker network inspect forkana_forkana-network --format '{{range .IPAM.Config}}{{.Subnet}}{{end}}'
# Example output: 172.18.0.0/16  (should be 172.30.0.0/16 on fresh deployments)

# Confirm the gateway IP that traffic arrives from
docker inspect forkana --format '{{range .NetworkSettings.Networks}}Gateway: {{.Gateway}}{{end}}'
# Example output: Gateway: 172.18.0.1

# Check the current trusted proxy setting
docker exec forkana cat /etc/gitea/app.ini | grep REVERSE_PROXY_TRUSTED
```

If the gateway IP is not covered by the `REVERSE_PROXY_TRUSTED_PROXIES` CIDRs,
that's the problem.

**Fix (running instance):**

Edit the `app.ini` on the **host volume mount** (not `/etc/gitea/app.ini` on the
host root filesystem - that path only exists inside the container):

```bash
nano ~/forkana/config/app.ini
```

In the `[security]` section, update the trusted proxies to include the actual
Docker subnet discovered above:

```ini
REVERSE_PROXY_TRUSTED_PROXIES = 127.0.0.0/8,::1/128,172.18.0.0/16
```

Replace `172.18.0.0/16` with whatever subnet `docker network inspect` reported.

Then restart the container:

```bash
docker restart forkana
```

> **Note:** If the `~/forkana/compose/` directory appears empty, you may be
> logged in as a different user than the deploy user (your UID 1000 user). The
> `docker restart forkana` command works regardless of which user you're logged
> in as, and is a reliable alternative to `docker compose restart`.

**Verify the fix:**

```bash
docker exec forkana cat /etc/gitea/app.ini | grep REVERSE_PROXY_TRUSTED
curl -sf http://127.0.0.1:${FORKANA_HOST_PORT:-3000}/api/healthz && echo "healthy"
```

Then test login in a browser to confirm CSRF errors are gone.

**Persistence across container recreation:**

Editing `app.ini` directly survives container restarts (`docker restart`) but
**not** full container recreations (`docker compose up` that rebuilds the
container). The `app.ini` template is re-rendered from environment variables on
first boot. To make the setting permanent across recreations, add the
`GITEA__`-prefixed environment variable override to the compose configuration
(e.g. in `compose.override.yml` or `dev.yml`):

```yaml
services:
  forkana:
    environment:
      GITEA__security__REVERSE_PROXY_TRUSTED_PROXIES: "127.0.0.0/8,::1/128,172.18.0.0/16"
```

Replace `172.18.0.0/16` with the actual Docker subnet.

The `GITEA__` prefix overrides are applied by `environment-to-ini` on every
container startup, even when `app.ini` already exists on the volume.

**Permanent fix (recreate the Docker network):**

If you want the instance to use the pinned `172.30.0.0/16` subnet (matching the
current `dev.yml` and `app.ini` template defaults), you need to recreate the
Docker network. This requires brief downtime:

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  down
docker network rm forkana_forkana-network
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  up -d
```

After recreation, verify the subnet is now `172.30.0.0/16`:

```bash
docker network inspect forkana_forkana-network --format '{{range .IPAM.Config}}{{.Subnet}}{{end}}'
# Expected output: 172.30.0.0/16
```

### Reset and Rebuild

```bash
# Stop all services
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  down

# Remove data (CAUTION: destroys all data)
rm -rf ~/forkana/data/* ~/forkana/config/* ~/forkana/postgres/*

# Rebuild and restart
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  up -d
```

</details>

---

## Maintenance
<details>

### Updating / Rolling Back Forkana

Deployments are automated via GitHub Actions on pushes to the
configured deploy branch (see [`DEPLOY_BRANCH`](#8-required-github-secrets-and-variables)).
Three paths to redeploy or roll back, in order of preference:

1. **Re-run a prior successful workflow run.** Open the *Actions* tab,
   pick the green run you want to revert to, and use *Re-run all jobs*.
   The original commit is rebuilt in CI and redeployed. No VM access
   required.

2. **Manual workflow dispatch with a commit SHA.** Go to
   *Actions > deploy-forkana-dev > Run workflow*, paste a 40-char
   commit SHA into the *Optional 40-char commit SHA to deploy* input,
   and run. CI rebuilds that commit and redeploys it. No VM access
   required; the preferred path for targeted rollbacks.

3. **Direct SSH (emergency only).** When GitHub Actions is
   unavailable:

   ```bash
   ~/forkana/deploy.sh <commit-sha>

   # Clean up old image layers afterwards
   docker image prune -f
   ```

   Requires a previously-transferred tarball at
   `~/forkana/images/forkana-<7-char-sha>.tar.gz` - the 7-character
   prefix must match the first 7 characters of the full commit SHA
   passed as argument. Tarballs land there as part of every CI
   deploy; `cleanup-images.sh` (see below) keeps only the most
   recent N.

#### Automatic Rollback on Health-Check Failure

After every deploy, `deploy_common.sh` (Step 9) polls
`http://127.0.0.1:${FORKANA_HOST_PORT}/api/healthz` for up to 150 s
(30 attempts × 5 s). If the service never becomes healthy the script:

- Emits a `::error::` annotation to the GitHub Actions job log
  identifying the failing commit and pinned image ref.
- Dumps the last 100 lines of the `forkana` container logs.
- Restores the previous `compose.override.yml` from the
  `compose.override.yml.prev` sidecar snapshot (written just before
  Step 7 generates the new override) and re-runs
  `docker compose up -d forkana` so the previously-healthy image
  becomes active again. `postgres` and `registry` are left alone.
- Exits 1 so the workflow run is marked failed.

On a first-ever deploy there is no prior override to restore; the
script logs `No previous override to roll back to (first deploy?)`
and exits 1 without replacing the current override. In that case,
redeploy a known-good SHA via path 2 above.

**Local testing:** The deploy script expects a pre-built image tarball at
`~/forkana/images/forkana-<7-char-sha>.tar.gz`. When run from inside a git
checkout, it automatically copies `dev.yml` from the working tree. Build
and place the tarball before invoking the script:

```bash
# 1. Build the image locally
COMMIT_SHA="$(git rev-parse HEAD)"
docker build --file docker/forkana/Dockerfile \
  --tag "forkana:${COMMIT_SHA}" .

# 2. Save as a compressed tarball
mkdir -p ~/forkana/images
docker save "forkana:${COMMIT_SHA}" | gzip \
  > ~/forkana/images/forkana-"${COMMIT_SHA:0:7}".tar.gz

# 3. Run the deploy script
./docker/forkana/deploy.sh "${COMMIT_SHA}"
```

### Cleanup Old Image Tarballs

Each deployment transfers a ~500MB tarball to `~/forkana/images/`. Without
periodic pruning the deploy user's home directory will fill up.
`cleanup-images.sh` keeps the most recent N tarballs (default 3) and removes
older ones; it is safe to run repeatedly and does nothing when the directory
holds fewer than N entries.

Installed alongside the deploy scripts (see
[Install the Deploy Scripts](#4-install-the-deploy-scripts)), the script
lives at `~/forkana/cleanup-images.sh` on the VM.

Schedule it via the deploy user's crontab (run as the deploy user):

```bash
# Runs daily at 02:00 UTC, keeping the last 3 image tarballs.
( crontab -l 2>/dev/null; \
  echo '0 2 * * * $HOME/forkana/cleanup-images.sh' ) | crontab -
```

Run it once manually to confirm it works:

```bash
~/forkana/cleanup-images.sh           # keep 3 (default)
~/forkana/cleanup-images.sh 5         # keep 5
```

> **Untagged image layers:** `cleanup-images.sh` only removes tarballs on
> disk. To reclaim Docker layer storage for images that are no longer
> referenced by `compose.override.yml`, occasionally run
> `docker image prune -f` as the deploy user.

### Backup

```bash
# Stop Forkana (optional, for consistent backup)
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  stop forkana

# Backup PostgreSQL
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  exec -T postgres pg_dump -U forkana forkana > backup-$(date +%Y%m%d).sql

# Backup data directory
tar -czf forkana-data-$(date +%Y%m%d).tar.gz -C ~/forkana data

# Restart Forkana
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  start forkana
```

### Restore from Backup

```bash
# Stop services
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  down

# Restore PostgreSQL
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  up -d postgres
cat backup-YYYYMMDD.sql | docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  exec -T postgres psql -U forkana forkana

# Restore data directory
tar -xzf forkana-data-YYYYMMDD.tar.gz -C ~/forkana

# Start Forkana
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  up -d
```

### Systemd Service (Optional)

Create `/etc/systemd/system/forkana.service` for automatic startup.
Replace `forkana-deploy` and its home directory path with your UID 1000
username and home directory:

```ini
[Unit]
Description=Forkana Docker Compose
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
User=forkana-deploy
Group=forkana-deploy
WorkingDirectory=/home/forkana-deploy/forkana/compose
ExecStart=/usr/bin/docker compose --env-file /home/forkana-deploy/forkana/compose/.env -f /home/forkana-deploy/forkana/compose/dev.yml -f /home/forkana-deploy/forkana/compose/compose.override.yml up -d
ExecStop=/usr/bin/docker compose --env-file /home/forkana-deploy/forkana/compose/.env -f /home/forkana-deploy/forkana/compose/dev.yml -f /home/forkana-deploy/forkana/compose/compose.override.yml down
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
```

Enable the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable forkana
sudo systemctl start forkana
```

</details>

---

<details>
<summary><h2>Additional Configuration</h2></summary>

### Environment Variable Reference

All Forkana settings can be overridden using environment variables with the format:
`GITEA__SECTION__KEY=value`

Examples:

```bash
# Set log level to debug
GITEA__log__LEVEL=debug

# Enable email notifications
GITEA__mailer__ENABLED=true
GITEA__mailer__SMTP_ADDR=smtp.example.com
GITEA__mailer__SMTP_PORT=587

# Configure OAuth2
GITEA__oauth2__JWT_SECRET=your-jwt-secret
```

### Enabling SSH Access

To enable SSH for Git operations, update `docker/forkana/dev.yml`:

```yaml
services:
  forkana:
    ports:
      - "127.0.0.1:${FORKANA_HOST_PORT:-3000}:3000"
      - "2222:2222"  # Add SSH port
    environment:
      GITEA__server__DISABLE_SSH: "false"
      GITEA__server__START_SSH_SERVER: "true"
      GITEA__server__SSH_PORT: "2222"
      GITEA__server__SSH_DOMAIN: your-domain.example
```

</details>

---

## Security Checklist

- [ ] Strong passwords generated for PostgreSQL and SECRET_KEY
- [ ] Environment file permissions set to 600
- [ ] HTTPS enabled with valid SSL certificate
- [ ] Firewall configured (only ports 80, 443, and optionally 2222)
- [ ] Regular backups scheduled
- [ ] `DISABLE_REGISTRATION` set to true (invite-only access)
- [ ] Nginx reverse proxy security headers configured (see `docker/forkana/nginx.conf`)
- [ ] Deploy SSH key restricted with `command=` and `restrict`
- [ ] SSH host key pinned in GitHub secrets (`DEPLOY_SSH_KNOWN_HOSTS`)
- [ ] Local registry bound to `127.0.0.1:5000` only (not publicly accessible)
