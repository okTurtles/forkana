# Forkana Deployment Guide

This guide covers deploying Forkana to a single Linux VM for private dev instance. It assumes you have SSH access to a fresh Linux server and basic Docker knowledge.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Server Preparation](#server-preparation)
3. [Pulling the Docker Image](#pulling-the-docker-image)
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

- **OS:** Ubuntu 22.04 LTS or Debian 12 (recommended)
- **CPU:** 2+ cores
- **RAM:** 4GB minimum (8GB recommended)
- **Disk:** 40GB+ SSD
- **Network:** Public IP with ports 80, 443 open

### Software Requirements

- Docker 24.0+ with Docker Compose v2
- Git (for cloning the repository)
- A domain name pointing to your server's IP

</details>

---

## Server Preparation

<details>

### 1. Update System Packages

```bash
sudo apt update && sudo apt upgrade -y
```

### 2. Install Docker

```bash
# Install Docker using the official script
curl -fsSL https://get.docker.com | sudo sh

# Add your user to the docker group
sudo usermod -aG docker $USER

# Log out and back in, then verify
docker --version
docker compose version
```

### 3. Create Directory Structure

```bash
# Create Forkana directories
sudo mkdir -p /opt/forkana/{data,config,postgres}
sudo chown -R 1000:1000 /opt/forkana/data /opt/forkana/config
sudo chown -R 999:999 /opt/forkana/postgres
```

</details>

---

## Pulling the Docker Image

Pull the pre-built image from Docker Hub:

```bash
docker pull okturtles/forkana:latest
docker tag okturtles/forkana:latest forkana:latest
```

---

## Environment Configuration

### Create Environment File

Create `/opt/forkana/.env`:

```bash
# PostgreSQL password (generate a strong random password)
POSTGRES_PASSWORD=your-secure-postgres-password-here

# Forkana domain (without https://)
FORKANA_DOMAIN=dev.forkana.org

# Secret key for session encryption (generate with: openssl rand -hex 32)
FORKANA_SECRET_KEY=your-64-character-hex-secret-key-here
```

### Generate Secure Secrets

```bash
# Generate PostgreSQL password
echo "POSTGRES_PASSWORD=$(openssl rand -base64 24)" >> /opt/forkana/.env

# Generate Forkana secret key
echo "FORKANA_SECRET_KEY=$(openssl rand -hex 32)" >> /opt/forkana/.env
```

### Secure the Environment File

```bash
chmod 600 /opt/forkana/.env
```

---

## Running Forkana

### Start the Services

```bash
cd /opt/forkana
docker compose -f docker/forkana/local.yml up -d
```

### Verify Services Are Running

```bash
docker compose -f docker/forkana/local.yml ps
docker compose -f docker/forkana/local.yml logs -f forkana
```

### Initialize the Database (First Run Only)

On first startup, Forkana will automatically:
1. Create the database schema
2. Generate internal tokens
3. Set up default configuration

Watch the logs to ensure initialization completes:

```bash
docker compose -f docker/forkana/local.yml logs -f forkana | grep -i "starting"
```

#### Create admin user

```bash
docker compose -f docker/forkana/local.yml exec forkana gitea admin user create \
  --username admin --password your-password-here --email admin@forkana.org --admin
```

---

## Reverse Proxy Setup

### Install Nginx and Certbot

```bash
sudo apt install -y nginx certbot python3-certbot-nginx
```

### Deploy the Nginx Configuration

The repository includes a ready-made Nginx configuration at [`docker/forkana/nginx.conf`](nginx.conf).
Copy it to your server and update the `server_name` and SSL paths to match your domain:

```bash
# Copy the config (adjust the source path to where you cloned the repo)
sudo cp docker/forkana/nginx.conf /etc/nginx/sites-available/forkana

# Edit the config to replace dev.forkana.example with your actual domain
sudo sed -i 's/dev.forkana.example/your-domain.example/g' /etc/nginx/sites-available/forkana
```

### Enable the Site and Obtain SSL Certificate

```bash
# Enable the site
sudo ln -s /etc/nginx/sites-available/forkana /etc/nginx/sites-enabled/

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
# Check PostgreSQL is accessible
docker compose -f docker/forkana/local.yml exec postgres pg_isready -U forkana -d forkana

# Check Forkana can connect
docker compose -f docker/forkana/local.yml logs forkana | grep -i database
```

### Verify All Services

```bash
# Check container status
docker compose -f docker/forkana/local.yml ps

# Expected output:
# NAME              STATUS                   PORTS
# forkana           Up (healthy)             127.0.0.1:3000->3000/tcp
# forkana-postgres  Up (healthy)             5432/tcp
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

```bash
# All services
docker compose -f docker/forkana/local.yml logs -f

# Forkana only
docker compose -f docker/forkana/local.yml logs -f forkana

# PostgreSQL only
docker compose -f docker/forkana/local.yml logs -f postgres

# Last 100 lines
docker compose -f docker/forkana/local.yml logs --tail=100 forkana
```

### Common Issues

#### Container Won't Start

```bash
# Check for errors
docker compose -f docker/forkana/local.yml logs forkana | tail -50

# Verify permissions
ls -la /opt/forkana/data
ls -la /opt/forkana/config

# Fix permissions if needed
sudo chown -R 1000:1000 /opt/forkana/data /opt/forkana/config
```

#### Database Connection Failed

```bash
# Verify PostgreSQL is running
docker compose -f docker/forkana/local.yml ps postgres

# Check PostgreSQL logs
docker compose -f docker/forkana/local.yml logs postgres

# Test connection manually
docker compose -f docker/forkana/local.yml exec postgres psql -U forkana -d forkana -c "SELECT 1;"
```

#### 502 Bad Gateway from Nginx

```bash
# Verify Forkana is running and healthy
docker compose -f docker/forkana/local.yml ps forkana
curl http://localhost:3000/api/healthz

# Check Nginx error logs
sudo tail -f /var/log/nginx/error.log
```

#### SSL Certificate Issues

```bash
# Renew certificate manually
sudo certbot renew --dry-run

# Check certificate status
sudo certbot certificates
```

### Reset and Rebuild

```bash
# Stop all services
docker compose -f docker/forkana/local.yml down

# Remove data (CAUTION: destroys all data)
sudo rm -rf /opt/forkana/data/* /opt/forkana/config/*

# Rebuild and restart
docker compose -f docker/forkana/local.yml up -d
```

</details>

---

## Maintenance
<details>

### Updating Forkana

```bash
cd /opt/forkana

# Pull latest image from Docker Hub
docker pull okturtles/forkana:latest
docker tag okturtles/forkana:latest forkana:latest

# Restart with new image
docker compose -f docker/forkana/local.yml up -d --remove-orphans

# Clean up old images
docker image prune -f
```

### Backup

```bash
# Stop Forkana (optional, for consistent backup)
docker compose -f docker/forkana/local.yml stop forkana

# Backup PostgreSQL
docker compose -f docker/forkana/local.yml exec postgres pg_dump -U forkana forkana > backup-$(date +%Y%m%d).sql

# Backup data directory
sudo tar -czf forkana-data-$(date +%Y%m%d).tar.gz /opt/forkana/data

# Restart Forkana
docker compose -f docker/forkana/local.yml start forkana
```

### Restore from Backup

```bash
# Stop services
docker compose -f docker/forkana/local.yml down

# Restore PostgreSQL
docker compose -f docker/forkana/local.yml up -d postgres
cat backup-YYYYMMDD.sql | docker compose -f docker/forkana/local.yml exec -T postgres psql -U forkana forkana

# Restore data directory
sudo tar -xzf forkana-data-YYYYMMDD.tar.gz -C /

# Start Forkana
docker compose -f docker/forkana/local.yml up -d
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
WorkingDirectory=/opt/forkana
ExecStart=/usr/bin/docker compose -f docker/forkana/local.yml up -d
ExecStop=/usr/bin/docker compose -f docker/forkana/local.yml down
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

To enable SSH for Git operations, update `docker/forkana/local.yml`:

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
- [ ] Docker images pulled from Docker Hub (`okturtles/forkana`) are verified
