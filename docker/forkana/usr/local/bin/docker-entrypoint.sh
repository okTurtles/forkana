#!/bin/sh
# Forkana Docker Entrypoint
# Based on docker/rootless entrypoint with Forkana-specific additions

# Protect against buggy runc in docker <20.10.6 causing problems with Alpine >= 3.14
if [ ! -x /bin/sh ]; then
  echo "Executable test for /bin/sh failed. Your Docker version is too old to run Alpine 3.14+ and Forkana. You must upgrade Docker.";
  exit 1;
fi

# Wait for PostgreSQL to be ready (if configured)
if [ "$DB_TYPE" = "postgres" ] && [ -n "$DB_HOST" ]; then
    echo "Waiting for PostgreSQL at ${DB_HOST}..."
    # Extract host and port from DB_HOST (format: host:port)
    PG_HOST=$(echo "$DB_HOST" | cut -d: -f1)
    PG_PORT=$(echo "$DB_HOST" | cut -d: -f2)
    PG_PORT=${PG_PORT:-5432}
    
    MAX_RETRIES=30
    RETRY_COUNT=0
    while ! pg_isready -h "$PG_HOST" -p "$PG_PORT" -q 2>/dev/null; do
        RETRY_COUNT=$((RETRY_COUNT + 1))
        if [ $RETRY_COUNT -ge $MAX_RETRIES ]; then
            echo "ERROR: PostgreSQL not available after ${MAX_RETRIES} attempts"
            exit 1
        fi
        echo "PostgreSQL not ready, waiting... (attempt ${RETRY_COUNT}/${MAX_RETRIES})"
        sleep 2
    done
    echo "PostgreSQL is ready!"
fi

if [ -x /usr/local/bin/docker-setup.sh ]; then
    /usr/local/bin/docker-setup.sh || { echo 'docker setup failed' ; exit 1; }
fi

if [ $# -gt 0 ]; then
    exec "$@"
else
    exec /usr/local/bin/gitea -c ${GITEA_APP_INI} web
fi

