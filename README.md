# Forkana

[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/okTurtles/forkana)

## Getting started

### Prerequisites

- Go 1.25. See https://go.dev/doc/manage-install
- node
- pnpm

### Installation

Add `custom/conf/app.ini`. See details below for file content. Make sure to properly set `WORK_PATH`.

<details>

```ini
WORK_PATH = /path/to/forkana ; the only line which needs modification

APP_NAME = Forkana

[server]
PROTOCOL = http
DOMAIN = localhost
HTTP_PORT = 3000
ROOT_URL = http://localhost:3000/
RUN_MODE = dev
; LANDING_PAGE = explore

[database]
DB_TYPE = sqlite3
PATH = data/gitea.db

[security]
INSTALL_LOCK = true
SECRET_KEY = changeme
INTERNAL_TOKEN = eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYmYiOjE3NTY3NDU2NjZ9.luRdwGyyCdO0dyjghYinzVgC7Uu8JXTlst2HkrjE80k

[oauth2]
JWT_SECRET = 9l225INhfQSZkuiCA1bu3rDvR3TDf6DckPy0m3qAGmE

[ui]
DEFAULT_THEME = gitea-light
EXPLORE_PAGING_DEFAULT_SORT = alphabetically
```

</details>

Then run

```bash
$ pnpm install
```

To build the project:

```bash
$ TAGS="bindata sqlite sqlite_unlock_notify" make build
```

For troubleshooting, see the details below:

<details>

Note that it might be necessary, depending on your system's configuration, to prepend a `GO` specification (indicating the name of the executable, if different from just `go`).

```bash
$ GO=go1.25.2 TAGS="bindata sqlite sqlite_unlock_notify" make build
```

Also, in some situations, one might encounter a network connectivity issue with IPv6. The Go proxy is trying to connect over IPv6 and failing with "socket is not connected" errors.
The solution is to modify the command by prefixing two additional vars:

```bash
$ GODEBUG="netdns=go+4" GOPROXY="direct" TAGS="bindata sqlite sqlite_unlock_notify" make build
```

Do the same for the following `make watch` command.

</details>

### Starting the application for development

To run the project:

```bash
$ TAGS="sqlite sqlite_unlock_notify" make watch
```

Note that you need to build once in any case, before running continuously with watch.

Finally, visit http://localhost:3000 and you are ready to go to fork with Forkana!

**Note** that the expectation is that you see the landing page of Forkana, **not** the database setup dialog of Gitea. If you see it, something went wrong.  Using `sqlite` should prevent precisely this during initial setup.

### Populate

Forkana includes tools to automatically populate your instance with content. For more information, see the details below:

<details>

#### Configuration

Prior to running this command, make sure you have created an API token for your user. See the [API token configuration guide](custom/services/article-creator/README.md#required-configuration) for instructions.

The `make populate` command accepts the following environment variables:

- `GITEA_URL` (required) - Your Forkana instance URL
- `GITEA_TOKEN` (required) - API token with repository creation permissions
- `ARTICLE_COUNT` (optional, default: 50) - Number of articles to fetch
- `CATEGORY` (optional) - Wikipedia category to fetch from (e.g., "Category:Physics")
- `PRIVATE` (optional, default: false) - Set to "true" to create private repositories

To populate your Forkana instance with 50 random Wikipedia articles:

```bash
GITEA_URL=http://localhost:3000 GITEA_TOKEN=your_api_token make populate
```

#### Detailed Documentation

For more information about the individual tools:

- [wiki2md](custom/services/wiki2md/README.md) - Fetch and convert Wikipedia articles to Markdown
- [article-creator](custom/services/article-creator/README.md) - Create Forkana repositories from Markdown files

</details>

### Clean up

To clean up

```bash
$ make clean-all
$ rm -rf data # to remove the sqlite database
```

### Run tests

Run e2e tests

```bash
$ make test-e2e-sqlite
```

Run all sqlite tests:

```bash
$ make test-sqlite
```

Run all pgsql tests:

```bash
$ TEST_MINIO_ENDPOINT=localhost:9000 TEST_PGSQL_HOST=localhost:5432 TEST_PGSQL_DBNAME=test TEST_PGSQL_USERNAME=postgres TEST_PGSQL_PASSWORD=postgres make test-pgsql
```

Requires Docker containers `gitea-pgsql` and `gitea-minio` running.

```bash
$ docker run -d --name gitea-pgsql -e POSTGRES_DB=test -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:14
$ docker run -d --name gitea-minio -e MINIO_ROOT_USER=123456 -e MINIO_ROOT_PASSWORD=12345678 -p 9000:9000 bitnamilegacy/minio:2023.8.31
```

Run a single psql test (see details):

<details>

```bash
$ TEST_MINIO_ENDPOINT=localhost:9000 TEST_PGSQL_HOST=localhost:5432 TEST_PGSQL_DBNAME=test TEST_PGSQL_USERNAME=postgres TEST_PGSQL_PASSWORD=postgres make test-pgsql#TestJobWithNeeds
```

</details>

# Gitea

Forkana is forked from [go-gitea/gitea](https://github.com/go-gitea/gitea).
