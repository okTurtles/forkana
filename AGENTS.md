# AGENTS.md

Guide for AI agents working in the Forkana codebase.

## Project Overview

Forkana is a fork of Gitea (self-hosted git service) that's been repurposed to act as a new type of online encyclopedia. In Forkana, each subject can have multiple articles associated with it, and each article is represented as a git repository that contains a single file (a README essentially) that acts as the article's content. It includes custom tools for populating the instance with Wikipedia content.

## Prerequisites

- **Go**: 1.25.1+ (see https://go.dev/doc/manage-install)
- **Node.js**: 22.6.0+
- **pnpm**: 10.0.0+
- **git-lfs**: Required for binary assets
- **Make**: Build system

## Essential Commands

### Building

```bash
# Initial setup
pnpm install                                    # Install frontend dependencies
TAGS="bindata sqlite sqlite_unlock_notify" make build   # Full build (required once)

# Incremental development
make backend                                    # Build Go backend only
make frontend                                   # Build frontend assets (webpack)
make watch                                      # Build and watch for changes
```

### Running

```bash
# Development (after initial build)
TAGS="sqlite sqlite_unlock_notify" make watch   # Run with hot reload

# Application runs at http://localhost:3000
```

### Testing

```bash
# All tests
make test                                       # Frontend + backend unit tests
make test-backend                               # Go unit tests only
make test-frontend                              # Vitest frontend tests

# Integration tests (by database)
make test-sqlite                                # SQLite integration tests
make test-pgsql                                 # PostgreSQL integration tests

# E2E tests
make test-e2e                                   # All Playwright E2E tests
make test-e2e-sqlite                            # E2E tests with SQLite

# Specific test by name
make test#TestSpecificName                      # Run specific Go test

# Docker containers for integration tests (PostgreSQL + MinIO)
docker run -d --name gitea-pgsql -e POSTGRES_DB=test -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:14
docker run -d --name gitea-minio -e MINIO_ROOT_USER=123456 -e MINIO_ROOT_PASSWORD=12345678 -p 9000:9000 bitnamilegacy/minio:2023.8.31
```

### Linting

```bash
make lint                                       # All linting
make lint-backend                               # Go linting (golangci-lint)
make lint-frontend                              # Frontend linting
make lint-js                                    # ESLint + vue-tsc
make lint-css                                   # Stylelint
make lint-spell                                 # Misspell check
make checks                                     # Various consistency checks
```

### Cleanup

```bash
make clean-all                                  # Clean build artifacts
rm -rf data                                     # Remove SQLite database
```

## Code Organization

```
cmd/                    # CLI commands (urfave/cli/v3)
├── main.go             # CLI entry point
models/                 # Database models and data access
├── fixtures/           # Test fixtures (YAML format)
modules/                # Core functionality (git, json, log, etc.)
services/               # Business logic layer
routers/                # HTTP routing
├── api/                # REST API endpoints
├── web/                # Web UI routes
templates/              # Go templates for UI
web_src/                # Frontend source
├── js/                 # JavaScript/TypeScript
├── css/                # Styling
├── svg/                # SVG assets
tests/
├── integration/        # Integration tests
├── e2e/                # End-to-end tests (Playwright)
custom/services/        # Forkana-specific tools
├── wiki2md/            # Wikipedia article fetcher
├── article-creator/    # Gitea repository creator
```

## Code Style and Conventions

### Import Aliasing

Use descriptive import aliases for clarity:

```go
git_model "code.gitea.io/gitea/models/git"
user_model "code.gitea.io/gitea/models/user"
repo_model "code.gitea.io/gitea/models/repo"
```

### JSON Module (Critical)

**MUST** use the custom JSON module, NOT the standard library:

```go
// Correct
import "code.gitea.io/gitea/modules/json"

// WRONG - will fail linting
import "encoding/json"
```

This is enforced by `depguard` in `.golangci.yml`.

### Forbidden Imports (depguard rules)

The linter forbids these packages:

- `encoding/json` → Use `code.gitea.io/gitea/modules/json`
- `io/ioutil` → Use `io` or `os` directly
- `golang.org/x/exp` → Experimental and unreliable; avoid in production code
- `code.gitea.io/gitea/modules/git/internal` → Use public `AddXxx` helper functions instead
- `gopkg.in/ini.v1` → Use Forkana’s/Gitea’s built-in configuration system instead
- `gitea.com/go-chi/cache` → Use Forkana’s built-in cache system instead

### Error Handling

Use structured logging for errors:

```go
log.Error("operation failed: %v", err)
```

### CLI Commands

Use `urfave/cli/v3` for CLI structure. See `cmd/main.go` for patterns.

## Testing Approach

### Unit Tests

- Go tests use `stretchr/testify` for assertions
- Frontend tests use Vitest
- Test fixtures stored as YAML in `models/fixtures/`

### Integration Tests

- Require database-specific tags: `test-sqlite`, `test-pgsql`, etc.
- Use Docker containers for PostgreSQL/MySQL/MSSQL
- Run with: `TEST_PGSQL_HOST=localhost:5432 TEST_PGSQL_DBNAME=test ...`

### E2E Tests

- Playwright-based end-to-end tests
- Located in `tests/e2e/`
- Test real browser interactions

### Build Tags

Tests use build tags for database backends:

- `sqlite` / `sqlite_unlock_notify`
- `pgsql`

## Custom Forkana Tools

### wiki2md

Fetches Wikipedia articles and converts to Markdown with YAML front matter.

See: `custom/services/wiki2md/README.md`

### article-creator

Creates Gitea repositories from Markdown files via API.

See: `custom/services/article-creator/README.md`

### Populate Instance

Combined workflow to populate Forkana with Wikipedia content:

```bash
GITEA_URL=http://localhost:3000 GITEA_TOKEN=your_api_token make populate
```

Environment variables:

- `GITEA_URL` (required) - Forkana instance URL
- `GITEA_TOKEN` (required) - API token with repo creation permissions
- `ARTICLE_COUNT` (optional, default: 50) - Number of articles
- `CATEGORY` (optional) - Wikipedia category to fetch from
- `PRIVATE` (optional) - Set "true" for private repos

## Configuration

### Required Config File

Create `custom/conf/app.ini` before first run:

```ini
WORK_PATH = /path/to/forkana

APP_NAME = Forkana

[server]
PROTOCOL = http
DOMAIN = localhost
HTTP_PORT = 3000
ROOT_URL = http://localhost:3000/
RUN_MODE = dev

[database]
DB_TYPE = sqlite3
PATH = data/gitea.db

[security]
INSTALL_LOCK = true
SECRET_KEY = changeme
INTERNAL_TOKEN = <generate-a-token>

[oauth2]
JWT_SECRET = <generate-a-secret>
```

## Important Gotchas

1. **Build before watch**: Run `make build` once before `make watch`
2. **IPv6 issues**: If Go proxy fails, use: `GODEBUG="netdns=go+4" GOPROXY="direct" make build`
3. **Go version**: May need to specify: `GO=go1.25.2 make build`
4. **No database setup dialog**: Using sqlite tags prevents the Gitea setup wizard
5. **Module path**: Go module is `code.gitea.io/gitea` (not github.com/okTurtles/forkana)

## CI/CD

See `.github/workflows/pull-compliance.yml` for official CI pipeline:

- Runs on pull requests
- Executes lint-backend, lint-frontend, checks
- Runs backend and frontend tests
- Builds the application
