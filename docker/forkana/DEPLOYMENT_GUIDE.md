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

> **Important:** If UID 1000 is already taken on your system (`getent passwd
> 1000`), you must resolve that conflict before continuing — either remove or
> reassign the existing user. The container bind-mounts depend on UID 1000
> ownership.

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

### 2. Verify/Fix Deploy User Home Directory

Some distributions may not create the home directory, or may leave it owned
by root. Run these commands as root to ensure the home directory exists with
correct ownership:

```bash
# Determine the home directory from the passwd entry
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"
echo "Deploy user home: $DEPLOY_HOME"

# Create the home directory if missing, set correct ownership/permissions
sudo install -d -o forkana-deploy -g forkana-deploy -m 0755 "$DEPLOY_HOME"

# Verify
ls -ld "$DEPLOY_HOME"
# Expected: drwxr-xr-x ... forkana-deploy forkana-deploy ... /home/forkana-deploy
```

### 3. Set Up the Deploy Directory and Git Repository

All paths live under the deploy user's home directory — no root-owned
directories and no `sudo` required during deployments.

```bash
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"

sudo -Hiu forkana-deploy bash -lc "
  mkdir -p $DEPLOY_HOME/forkana/{repo,compose,data,data/git,data/custom,config,postgres}
  chmod 0755 $DEPLOY_HOME/forkana/data $DEPLOY_HOME/forkana/data/git \
             $DEPLOY_HOME/forkana/data/custom $DEPLOY_HOME/forkana/config
"

# Clone the repo (skip if already cloned)
if [ ! -d "$DEPLOY_HOME/forkana/repo/.git" ]; then
  sudo -Hiu forkana-deploy git clone \
    https://github.com/okTurtles/forkana.git "$DEPLOY_HOME/forkana/repo"
fi
```

### 4. Install the Deploy Script

```bash
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"

sudo -Hiu forkana-deploy cp \
  "$DEPLOY_HOME/forkana/repo/docker/forkana/deploy.sh" \
  "$DEPLOY_HOME/forkana/deploy.sh"
sudo -Hiu forkana-deploy chmod 755 "$DEPLOY_HOME/forkana/deploy.sh"
```

> **Note:** The deploy script is also updated automatically during each
> deployment (it copies `dev.yml` from the checked-out commit). To update
> `deploy.sh` itself, pull the latest version from the repo.

### 5. Configure `authorized_keys` with Forced-Command Restrictions

Create the deploy user's `authorized_keys` with the GitHub Actions public
key. The `command=` directive restricts the key to running the deploy script
only:

```bash
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"

# Create .ssh directory with correct ownership and permissions
sudo install -d -o forkana-deploy -g forkana-deploy -m 0700 "$DEPLOY_HOME/.ssh"
```

Add the following single line to `$DEPLOY_HOME/.ssh/authorized_keys` (replace
`ssh-ed25519 AAAA...` with the actual public key):

```
command="~/forkana/deploy.sh",restrict ssh-ed25519 AAAA... github-actions-deploy
```

```bash
# Set correct ownership and permissions on authorized_keys
sudo chown forkana-deploy:forkana-deploy "$DEPLOY_HOME/.ssh/authorized_keys"
sudo chmod 600 "$DEPLOY_HOME/.ssh/authorized_keys"
```

**Security notes:**
- `command=` forces every SSH session with this key to execute `deploy.sh`.
  The commit SHA is available to the script via `$SSH_ORIGINAL_COMMAND`.
- `no-port-forwarding,no-pty,no-agent-forwarding,no-X11-forwarding` prevent
  tunnelling, interactive shells, and agent hijacking.

### 6. Start the Local Docker Registry

The registry is managed by `docker compose` via `dev.yml`. For the initial
bootstrap (before the first deploy), start it manually:

```bash
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"

docker compose -f $DEPLOY_HOME/forkana/repo/docker/forkana/dev.yml up -d registry
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
this value — **never** use `StrictHostKeyChecking=no`.

### 8. Required GitHub Secrets

| Secret | Description |
|---|---|
| `DEPLOY_HOST` | VM IP address or hostname |
| `DEPLOY_USER` | `forkana-deploy` |
| `DEPLOY_SSH_KEY` | Private key (ed25519 recommended) for the deploy user |
| `DEPLOY_SSH_KNOWN_HOSTS` | Output of `ssh-keyscan` from step 7 |

</details>

---

## Environment Configuration

### Create Environment File

Create `$DEPLOY_HOME/forkana/compose/.env` with the commands below. Compose reads `.env`
from the project directory automatically.

The file requires **five** variables:

| Variable | Description |
|----------|-------------|
| `POSTGRES_PASSWORD` | PostgreSQL password (random, never reused) |
| `FORKANA_DOMAIN` | Your domain without `https://` prefix |
| `FORKANA_SECRET_KEY` | 64-char hex key for session encryption |
| `FORKANA_INTERNAL_TOKEN` | Internal API token (base64, 32+ chars) |
| `FORKANA_JWT_SECRET` | OAuth2 JWT signing secret (base64, 32+ chars) |

Generate the file (replace `dev.forkana.org` with your actual domain):

```bash
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"

# Create .env with generated secrets (overwrites any existing file)
echo "POSTGRES_PASSWORD=$(head -c 18 /dev/urandom | base64)"  >  $DEPLOY_HOME/forkana/compose/.env
echo "FORKANA_DOMAIN=dev.forkana.org"                          >> $DEPLOY_HOME/forkana/compose/.env
echo "FORKANA_SECRET_KEY=$(od -An -tx1 -N32 /dev/urandom | tr -d ' \n')" >> $DEPLOY_HOME/forkana/compose/.env
echo "FORKANA_INTERNAL_TOKEN=$(head -c 32 /dev/urandom | base64)" >> $DEPLOY_HOME/forkana/compose/.env
echo "FORKANA_JWT_SECRET=$(head -c 32 /dev/urandom | base64)" >> $DEPLOY_HOME/forkana/compose/.env

# Lock down permissions — only the deploy user should read this file
chmod 600 $DEPLOY_HOME/forkana/compose/.env
```

Verify the result:

```bash
cat $DEPLOY_HOME/forkana/compose/.env
```

---

## Running Forkana

Automated deployments are handled by `deploy.sh` (triggered via GitHub Actions).
For manual operations, use the compose files in `~/forkana/compose/`.

> **User context:** All commands in this section (and in Health Checks,
> Maintenance, and most of Troubleshooting) should be run **as the deploy
> user**. Either log in as `forkana-deploy` or prefix your session with
> `sudo -iu forkana-deploy`. This ensures `~` expands to
> `/home/forkana-deploy`.

### Start the Services

> **Note:** When running commands via `sudo -Hiu forkana-deploy`, Docker Compose
> may not auto-load the `.env` file. Use `--env-file` explicitly:
> ```bash
> docker compose --env-file ~/forkana/compose/.env -f dev.yml -f compose.override.yml up -d
> ```

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
However, for the initial setup, use a simple HTTP-only config and let Certbot
configure SSL automatically:

#### Fedora

Fedora uses `/etc/nginx/conf.d/` instead of `sites-available/sites-enabled`:

```bash
# Create HTTP-only config for initial Certbot setup
sudo tee /etc/nginx/conf.d/forkana.conf << 'EOF'
server {
    listen 80;
    listen [::]:80;
    server_name your-domain.example;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        client_max_body_size 100M;
    }
}
EOF

# Edit to set your actual domain
sudo sed -i 's/your-domain.example/your-actual-domain.com/g' /etc/nginx/conf.d/forkana.conf
```

#### Debian/Ubuntu

```bash
# Create HTTP-only config for initial Certbot setup
sudo tee /etc/nginx/sites-available/forkana << 'EOF'
server {
    listen 80;
    listen [::]:80;
    server_name your-domain.example;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        client_max_body_size 100M;
    }
}
EOF

# Edit to set your actual domain
sudo sed -i 's/your-domain.example/your-actual-domain.com/g' /etc/nginx/sites-available/forkana

# Enable the site
sudo ln -s /etc/nginx/sites-available/forkana /etc/nginx/sites-enabled/
```

### Enable the Site and Obtain SSL Certificate

```bash
# Test configuration
sudo nginx -t

# Enable and start nginx
sudo systemctl enable nginx
sudo systemctl start nginx

# Obtain SSL certificate (Certbot will automatically update the nginx config)
sudo certbot --nginx -d your-domain.example
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
docker compose -f dev.yml -f compose.override.yml logs forkana | tail -50
```

```bash
# Verify permissions (run as root — ~/forkana would expand to /root)
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"

ls -la $DEPLOY_HOME/forkana/data
ls -la $DEPLOY_HOME/forkana/config
ls -la $DEPLOY_HOME/forkana/postgres

# Fix permissions if needed (deploy user must be UID 1000)
chown -R 1000:1000 $DEPLOY_HOME/forkana/data $DEPLOY_HOME/forkana/config $DEPLOY_HOME/forkana/postgres
```

#### Deploy User Commands Fail with "Permission denied"

If `sudo -u forkana-deploy` commands fail with errors like
`mkdir: cannot create directory '/home/forkana-deploy': Permission denied`,
the deploy user's home directory is either missing or owned by root. This is
common on Fedora with system users. Run the
[Verify/Fix Deploy User Home Directory](#2-verifyfix-deploy-user-home-directory)
step to repair it:

```bash
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"
sudo install -d -o forkana-deploy -g forkana-deploy -m 0755 "$DEPLOY_HOME"
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
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"

# Check directory ownership
ls -la $DEPLOY_HOME/forkana/data $DEPLOY_HOME/forkana/config $DEPLOY_HOME/forkana/postgres

# Expected: all directories owned by UID 1000 (or forkana-deploy)
# drwxr-xr-x  1000  1000  ... data/
```

**Solution:**
```bash
DEPLOY_HOME="$(getent passwd forkana-deploy | cut -d: -f6)"

# Fix ownership on all bind-mount directories
sudo chown -R 1000:1000 $DEPLOY_HOME/forkana/data $DEPLOY_HOME/forkana/config $DEPLOY_HOME/forkana/postgres

# Verify the deploy user has UID 1000
getent passwd forkana-deploy | cut -d: -f3
# Should output: 1000
```

> **Why UID 1000?** The container's `git` user is UID 1000. The deploy user is created with UID 1000 so directories it creates are automatically accessible to the container.

#### Testing in a Container (Limitations)

When validating the deployment guide in a container (e.g., for preflight testing on Mac M1), be aware of these limitations:

| Step | Container-Testable | Notes |
|------|-------------------|-------|
| Package installation | ✅ Yes | All `dnf`/`apt` commands work |
| User creation | ✅ Yes | `useradd`/`adduser` work |
| Directory setup | ✅ Yes | All `mkdir`/`chmod` work |
| Git clone | ✅ Yes | Works if network available |
| Deploy script logic | ✅ Yes | Can validate script syntax |
| `systemctl` commands | ❌ No | Requires systemd (VPS-only) |
| Docker daemon | ❌ No | Requires Docker-in-Docker or VPS |
| Nginx reverse proxy | ⚠️ Partial | Can install, but no systemd |
| SSL certificates | ❌ No | Requires public domain |

**Container testing commands:**
```bash
# Run Fedora container for testing
docker run --rm -it --platform linux/amd64 fedora:43 bash

# Inside container, run through the guide steps
# Skip systemctl, Docker daemon, and Nginx/systemd commands
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

#### Fedora-Specific: Nginx Proxy Permission Denied

**Symptoms:**
```
connect() to 127.0.0.1:3000 failed (13: Permission denied) while connecting to upstream
```

**Cause:** SELinux blocks nginx from making network connections by default.

**Solution:**
```bash
# Enable nginx to connect to network services
setsebool -P httpd_can_network_connect 1
sudo systemctl restart nginx
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
