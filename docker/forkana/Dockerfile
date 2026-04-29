# Forkana Private Preview Dockerfile
# Based on Dockerfile.rootless with Forkana-specific modifications

# Build stage
FROM docker.io/library/golang:1.25-alpine3.22 AS build-env

ARG GOPROXY
ENV GOPROXY=${GOPROXY:-direct}

ARG GITEA_VERSION
ARG TAGS="sqlite sqlite_unlock_notify"
ENV TAGS="bindata timetzdata $TAGS"
ARG CGO_EXTRA_CFLAGS

# Build deps
RUN apk --no-cache add \
    build-base \
    git \
    nodejs \
    npm \
    && npm install -g pnpm@10 \
    && rm -rf /var/cache/apk/*

# Setup repo
COPY . ${GOPATH}/src/code.gitea.io/gitea
WORKDIR ${GOPATH}/src/code.gitea.io/gitea

# Checkout version if set
RUN if [ -n "${GITEA_VERSION}" ]; then git checkout "${GITEA_VERSION}"; fi \
 && make clean-all build

# Build env-to-ini utility
RUN go build contrib/environment-to-ini/environment-to-ini.go

# Copy local files (Forkana-specific scripts and templates).
# Only the runtime subtrees are staged; deployment artifacts (Dockerfile,
# deploy*.sh, dev.yml, nginx.conf, DEPLOYMENT_GUIDE.md, etc.) must not ship
# inside the runtime image.
COPY docker/forkana/etc /tmp/local/etc
COPY docker/forkana/usr /tmp/local/usr

# Stage Forkana custom overrides (options, public, services, templates) for the
# runtime image. Uses conditional copies so the build succeeds even if a
# directory is missing from the build context (e.g. excluded by .dockerignore).
RUN mkdir -p /tmp/custom-defaults \
 && cd /go/src/code.gitea.io/gitea/custom \
 && for dir in options public services templates; do \
      [ -d "$dir" ] && cp -r "$dir" /tmp/custom-defaults/ || true; \
    done

# Set permissions
RUN chmod 755 /tmp/local/usr/local/bin/docker-entrypoint.sh \
              /tmp/local/usr/local/bin/docker-setup.sh \
              /tmp/local/usr/local/bin/gitea \
              /go/src/code.gitea.io/gitea/gitea \
              /go/src/code.gitea.io/gitea/environment-to-ini

# Runtime stage
FROM docker.io/library/alpine:3.22
LABEL maintainer="Forkana Maintainers <maintainers@forkana.org>"

# Expose HTTP port only (SSH disabled for preview)
# Port 2222 can be enabled if SSH is needed
EXPOSE 3000

RUN apk --no-cache add \
    bash \
    ca-certificates \
    dumb-init \
    gettext \
    git \
    curl \
    gnupg \
    openssh-keygen \
    postgresql-client \
    && rm -rf /var/cache/apk/*

# Create git user (UID 1000 for rootless operation)
RUN addgroup \
    -S -g 1000 \
    git && \
  adduser \
    -S -H -D \
    -h /var/lib/gitea/git \
    -s /bin/bash \
    -u 1000 \
    -G git \
    git

RUN mkdir -p /var/lib/gitea /etc/gitea
RUN chown git:git /var/lib/gitea /etc/gitea

COPY --from=build-env /tmp/local /
COPY --from=build-env --chown=root:root /go/src/code.gitea.io/gitea/gitea /app/gitea/gitea
COPY --from=build-env --chown=root:root /go/src/code.gitea.io/gitea/environment-to-ini /usr/local/bin/environment-to-ini

# Bake Forkana custom overrides (options, public, services, templates) into the image.
# public/ carries custom CSS (content.custom.css, navbar.custom.css, etc.) and images.
# Placed outside both VOLUME paths (/var/lib/gitea, /etc/gitea) so they survive
# volume mounts. docker-setup.sh syncs them into GITEA_CUSTOM on every startup.
COPY --from=build-env --chown=1000:1000 /tmp/custom-defaults/ /usr/local/share/gitea/custom-defaults/

# Run as non-root user (git:git)
USER 1000:1000

# Environment configuration
ENV GITEA_WORK_DIR=/var/lib/gitea
ENV GITEA_CUSTOM=/var/lib/gitea/custom
ENV GITEA_TEMP=/tmp/gitea
ENV TMPDIR=/tmp/gitea
ENV GITEA_APP_INI=/etc/gitea/app.ini
ENV HOME="/var/lib/gitea/git"

# Forkana-specific defaults
ENV APP_NAME="Forkana"
ENV RUN_MODE="prod"

VOLUME ["/var/lib/gitea", "/etc/gitea"]
WORKDIR /var/lib/gitea

# Health check endpoint
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:3000/api/healthz || exit 1

ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD []
