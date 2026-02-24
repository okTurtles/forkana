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
- Git (for cloning the repository)
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
# Install Docker from Fedora repos (or use the official Docker repo)
sudo dnf install -y docker docker-compose-plugin

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
> skip this step — the deploy user's directories are created there instead.

```bash
mkdir -p ~/forkana/{data,data/git,data/custom,config,postgres,compose}
chmod 0755 ~/forkana/data ~/forkana/data/git ~/forkana/data/custom ~/forkana/config
```

</details>

---

## Server Setup (CI/CD)

This section configures the VM for the **build-on-server** deployment model.
GitHub Actions triggers a deploy via SSH; the VM builds the image from source,
pushes it to a local Docker registry, and deploys with a digest-pinned compose
override.

<details>

### 1. Create a Dedicated Deploy User

The deploy user **must** have UID 1000 so that directories it creates are
automatically owned by the container user (git:git, also UID 1000).

#### Debian/Ubuntu

```bash
sudo adduser --system --group --shell /bin/bash --uid 1000 forkana-deploy
sudo usermod -aG docker forkana-deploy
```

#### Fedora

```bash
sudo useradd --system --create-home --shell /bin/bash --uid 1000 forkana-deploy
sudo usermod -aG docker forkana-deploy
```

### 2. Set Up the Deploy Directory and Git Repository

All paths live under the deploy user's home directory — no root-owned
directories and no `sudo` required during deployments.

```bash
# Run these as the forkana-deploy user (or sudo -u forkana-deploy)
sudo -u forkana-deploy bash -c '
  mkdir -p ~/forkana/{repo,compose,data,data/git,data/custom,config,postgres}
  chmod 0755 ~/forkana/data ~/forkana/data/git ~/forkana/data/custom ~/forkana/config
  git clone https://github.com/okTurtles/forkana.git ~/forkana/repo
'
```

### 3. Install the Deploy Script

```bash
sudo -u forkana-deploy cp ~/forkana/repo/docker/forkana/deploy.sh ~/forkana/deploy.sh
sudo -u forkana-deploy chmod 755 ~/forkana/deploy.sh
```

> **Note:** The deploy script is also updated automatically during each
> deployment (it copies `dev.yml` from the checked-out commit). To update
> `deploy.sh` itself, pull the latest version from the repo.

### 4. Configure `authorized_keys` with Forced-Command Restrictions

Create `~forkana-deploy/.ssh/authorized_keys` with the GitHub Actions public
key. The `command=` directive restricts the key to running the deploy script
only:

```bash
sudo -u forkana-deploy mkdir -p ~forkana-deploy/.ssh
sudo chmod 700 ~forkana-deploy/.ssh
```

Add the following single line (replace `ssh-ed25519 AAAA...` with the actual
public key):

```
command="~/forkana/deploy.sh",restrict ssh-ed25519 AAAA... github-actions-deploy
```

```bash
sudo chmod 600 ~forkana-deploy/.ssh/authorized_keys
sudo chown -R forkana-deploy:forkana-deploy ~forkana-deploy/.ssh
```

**Security notes:**
- `command=` forces every SSH session with this key to execute `deploy.sh`.
  The commit SHA is available to the script via `$SSH_ORIGINAL_COMMAND`.
- `no-port-forwarding,no-pty,no-agent-forwarding,no-X11-forwarding` prevent
  tunnelling, interactive shells, and agent hijacking.

### 5. Start the Local Docker Registry

The registry is managed by `docker compose` via `dev.yml`. For the initial
bootstrap (before the first deploy), start it manually:

```bash
docker compose -f ~/forkana/repo/docker/forkana/dev.yml up -d registry
```

Verify it is running:

```bash
curl -sf http://127.0.0.1:5000/v2/ && echo "Registry OK"
```

The registry binds to `127.0.0.1:5000` only and is **not** publicly
accessible. Data persists in the `registry-data` Docker volume.

### 6. Obtain the SSH Host Key for GitHub

Pin the VM's SSH host key in the GitHub Actions workflow to prevent MITM
attacks. Run on the VM:

```bash
# Print the host key entry (use the key type matching your server, usually ed25519)
ssh-keyscan -t ed25519 <VM_IP_OR_HOSTNAME> 2>/dev/null
```

Copy the output and store it as the `DEPLOY_SSH_KNOWN_HOSTS` secret in the
GitHub repository settings. The workflow uses `StrictHostKeyChecking=yes` with
this value — **never** use `StrictHostKeyChecking=no`.

### 7. Required GitHub Secrets

| Secret | Description |
|---|---|
| `DEPLOY_HOST` | VM IP address or hostname |
| `DEPLOY_USER` | `forkana-deploy` |
| `DEPLOY_SSH_KEY` | Private key (ed25519 recommended) for the deploy user |
| `DEPLOY_SSH_KNOWN_HOSTS` | Output of `ssh-keyscan` from step 6 |

</details>

---

## Environment Configuration

### Create Environment File

Create `~/forkana/compose/.env` with the commands below. Compose reads `.env`
from the project directory automatically.

The file requires three variables:

| Variable | Description |
|----------|-------------|
| `POSTGRES_PASSWORD` | PostgreSQL password (random, never reused) |
| `FORKANA_DOMAIN` | Your domain without `https://` prefix |
| `FORKANA_SECRET_KEY` | 64-char hex key for session encryption |

Generate the file (replace `dev.forkana.org` with your actual domain):

```bash
# Create .env with generated secrets (overwrites any existing file)
echo "POSTGRES_PASSWORD=$(openssl rand -base64 24)"  >  ~/forkana/compose/.env
echo "FORKANA_DOMAIN=dev.forkana.org"                 >> ~/forkana/compose/.env
echo "FORKANA_SECRET_KEY=$(openssl rand -hex 32)"     >> ~/forkana/compose/.env

# Lock down permissions — only the deploy user should read this file
chmod 600 ~/forkana/compose/.env
```

Verify the result:

```bash
cat ~/forkana/compose/.env
```

---

## Running Forkana

Automated deployments are handled by `deploy.sh` (triggered via GitHub Actions).
For manual operations, use the compose files in `~/forkana/compose/`:

### Start the Services

```bash
cd ~/forkana/compose
docker compose -f dev.yml -f compose.override.yml up -d
```

### Verify Services Are Running

```bash
cd ~/forkana/compose
docker compose -f dev.yml -f compose.override.yml ps
docker compose -f dev.yml -f compose.override.yml logs -f forkana
```

### Initialize the Database (First Run Only)

On first startup, Forkana will automatically:
1. Create the database schema
2. Generate internal tokens
3. Set up default configuration

Watch the logs to ensure initialization completes:

```bash
cd ~/forkana/compose
docker compose -f dev.yml -f compose.override.yml logs -f forkana | grep -i "starting"
```

#### Create admin user

```bash
cd ~/forkana/compose
docker compose -f dev.yml -f compose.override.yml exec forkana gitea admin user create \
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

The repository includes a ready-made Nginx configuration at [`docker/forkana/nginx.conf`](nginx.conf).
Copy it to your server and update the `server_name` and SSL paths to match your domain:

#### Debian/Ubuntu

```bash
# Copy the config
sudo cp docker/forkana/nginx.conf /etc/nginx/sites-available/forkana

# Edit the config to replace dev.forkana.example with your actual domain
sudo sed -i 's/dev.forkana.example/your-domain.example/g' /etc/nginx/sites-available/forkana

# Enable the site
sudo ln -s /etc/nginx/sites-available/forkana /etc/nginx/sites-enabled/
```

#### Fedora

Fedora uses `/etc/nginx/conf.d/` instead of `sites-available/sites-enabled`:

```bash
# Copy the config
sudo cp docker/forkana/nginx.conf /etc/nginx/conf.d/forkana.conf

# Edit the config to replace dev.forkana.example with your actual domain
sudo sed -i 's/dev.forkana.example/your-domain.example/g' /etc/nginx/conf.d/forkana.conf
```

### Enable the Site and Obtain SSL Certificate

```bash
# Test configuration
sudo nginx -t

# Obtain SSL certificate (before enabling HTTPS block)
# First, temporarily comment out the SSL server block and run:
sudo certbot --nginx -d your-domain.example

# Reload Nginx
sudo systemctl reload nginx
```

---

## Health Checks & Verification

<details>

### Verify Application Health

```bash
# Check the health endpoint
curl -f http://localhost:3000/api/healthz

# Check via HTTPS (after proxy setup)
curl -f https://your-domain.example/api/healthz
```

### Verify Database Connection

```bash
cd ~/forkana/compose

# Check PostgreSQL is accessible
docker compose -f dev.yml -f compose.override.yml exec postgres pg_isready -U forkana -d forkana

# Check Forkana can connect
docker compose -f dev.yml -f compose.override.yml logs forkana | grep -i database
```

### Verify All Services

```bash
cd ~/forkana/compose
docker compose -f dev.yml -f compose.override.yml ps

# Expected output:
# NAME               STATUS                   PORTS
# forkana            Up (healthy)             127.0.0.1:3000->3000/tcp
# forkana-postgres   Up (healthy)             5432/tcp
# forkana-registry   Up (healthy)             127.0.0.1:5000->5000/tcp
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

All compose commands below assume `cd ~/forkana/compose` first.

```bash
# All services
docker compose -f dev.yml -f compose.override.yml logs -f

# Forkana only
docker compose -f dev.yml -f compose.override.yml logs -f forkana

# PostgreSQL only
docker compose -f dev.yml -f compose.override.yml logs -f postgres

# Last 100 lines
docker compose -f dev.yml -f compose.override.yml logs --tail=100 forkana
```

### Common Issues

#### Container Won't Start

```bash
docker compose -f dev.yml -f compose.override.yml logs forkana | tail -50

# Verify permissions (all bind-mount dirs must be owned by UID 1000)
ls -la ~/forkana/data
ls -la ~/forkana/config
ls -la ~/forkana/postgres

# Fix permissions if needed (deploy user must be UID 1000)
chown -R 1000:1000 ~/forkana/data ~/forkana/config ~/forkana/postgres
```

#### Database Connection Failed

```bash
docker compose -f dev.yml -f compose.override.yml ps postgres
docker compose -f dev.yml -f compose.override.yml logs postgres

# Test connection manually
docker compose -f dev.yml -f compose.override.yml exec postgres psql -U forkana -d forkana -c "SELECT 1;"
```

#### 502 Bad Gateway from Nginx

```bash
docker compose -f dev.yml -f compose.override.yml ps forkana
curl http://localhost:3000/api/healthz

# Check Nginx error logs
sudo tail -f /var/log/nginx/error.log
```

#### SSL Certificate Issues

```bash
sudo certbot renew --dry-run
sudo certbot certificates
```

### Reset and Rebuild

```bash
cd ~/forkana/compose

# Stop all services
docker compose -f dev.yml -f compose.override.yml down

# Remove data (CAUTION: destroys all data)
rm -rf ~/forkana/data/* ~/forkana/config/* ~/forkana/postgres/*

# Rebuild and restart
docker compose -f dev.yml -f compose.override.yml up -d
```

</details>

---

## Maintenance
<details>

### Updating Forkana

Deployments are automated via GitHub Actions. To deploy manually:

```bash
# Run the deploy script with a specific commit SHA
~/forkana/deploy.sh <commit-sha>

# Or re-run a previous GitHub Actions workflow from the Actions tab.

# Clean up old images
docker image prune -f
```

**Local testing:** When run from inside a git checkout of the repository,
`deploy.sh` automatically detects the repo root and skips the git
fetch/checkout step, building directly from the working tree:

```bash
./docker/forkana/deploy.sh "$(git rev-parse HEAD)"
```

### Backup

```bash
cd ~/forkana/compose

# Stop Forkana (optional, for consistent backup)
docker compose -f dev.yml -f compose.override.yml stop forkana

# Backup PostgreSQL
docker compose -f dev.yml -f compose.override.yml exec -T postgres pg_dump -U forkana forkana > backup-$(date +%Y%m%d).sql

# Backup data directory
tar -czf forkana-data-$(date +%Y%m%d).tar.gz -C ~/forkana data

# Restart Forkana
docker compose -f dev.yml -f compose.override.yml start forkana
```

### Restore from Backup

```bash
cd ~/forkana/compose

# Stop services
docker compose -f dev.yml -f compose.override.yml down

# Restore PostgreSQL
docker compose -f dev.yml -f compose.override.yml up -d postgres
cat backup-YYYYMMDD.sql | docker compose -f dev.yml -f compose.override.yml exec -T postgres psql -U forkana forkana

# Restore data directory
tar -xzf forkana-data-YYYYMMDD.tar.gz -C ~/forkana

# Start Forkana
docker compose -f dev.yml -f compose.override.yml up -d
```

### Systemd Service (Optional)

Create `/etc/systemd/system/forkana.service` for automatic startup:

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
WorkingDirectory=%h/forkana/compose
ExecStart=/usr/bin/docker compose -f dev.yml -f compose.override.yml up -d
ExecStop=/usr/bin/docker compose -f dev.yml -f compose.override.yml down
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

<summary><h2>Additional Configuration</h2></summary>
<details>

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
      - "127.0.0.1:3000:3000"
      - "2222:2222"  # Add SSH port
    environment:
      DISABLE_SSH: "false"
      SSH_PORT: "2222"
      SSH_DOMAIN: your-domain.example
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
- [ ] Deploy SSH key restricted with `command=` and `no-port-forwarding,no-pty,no-agent-forwarding`
- [ ] SSH host key pinned in GitHub secrets (`DEPLOY_SSH_KNOWN_HOSTS`)
- [ ] Local registry bound to `127.0.0.1:5000` only (not publicly accessible)
