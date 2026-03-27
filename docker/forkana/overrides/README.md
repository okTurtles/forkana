# Optional Compose Overrides

This directory contains **non-default** Docker Compose overrides for Forkana.

- `external-postgres.override.yml`: run Forkana with an existing Postgres.
- `logging.override.yml`: switch from default `json-file` logging to another Docker logging driver.

`dev.yml` remains the default stack and already sets safe `json-file` rotation.

## Usage Pattern

Always layer files in this order:

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/<override-file>.yml \
  up -d --force-recreate
```

Use `--force-recreate` after changing logging drivers/options so Docker applies the new log config.

### Combined Optional Overrides

Use this when you want both an existing external Postgres and a non-default
logging driver:

```bash
docker compose --env-file ~/forkana/compose/.env \
  -f ~/forkana/compose/dev.yml \
  -f ~/forkana/compose/compose.override.yml \
  -f ~/forkana/compose/external-postgres.override.yml \
  -f ~/forkana/compose/logging.override.yml \
  up -d --force-recreate
```

## Logging Driver Examples

Edit `logging.override.yml` and set only options supported by your selected driver.

### local (recommended non-default)

```yaml
services:
  registry:
    logging:
      driver: local
  postgres:
    logging:
      driver: local
  forkana:
    logging:
      driver: local
```

### journald

```yaml
services:
  registry:
    logging:
      driver: journald
      options:
        tag: "forkana/{{.Name}}"
  postgres:
    logging:
      driver: journald
      options:
        tag: "forkana/{{.Name}}"
  forkana:
    logging:
      driver: journald
      options:
        tag: "forkana/{{.Name}}"
```

### fluentd

```yaml
services:
  registry:
    logging:
      driver: fluentd
      options:
        fluentd-address: "127.0.0.1:24224"
        tag: "forkana.registry"
  postgres:
    logging:
      driver: fluentd
      options:
        fluentd-address: "127.0.0.1:24224"
        tag: "forkana.postgres"
  forkana:
    logging:
      driver: fluentd
      options:
        fluentd-address: "127.0.0.1:24224"
        tag: "forkana.app"
```

## Notes

- The default `json-file` settings in `dev.yml` approximate `rotate 14` with:
  - `max-file=14`
  - `max-size=10m` (size-based trigger; Docker has no native daily rotation)
  - `compress=true`
- `logrotate` semantics like `daily`, `missingok`, `notifempty`, `create 640`, and `delaycompress` are not directly available in Docker logging driver options.
