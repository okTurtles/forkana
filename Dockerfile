# Build stage
# Alpine 3.23 ships npm >= 11.10.0, required for the min-release-age age gate below.
FROM docker.io/library/golang:1.25-alpine3.23 AS build-env

# Default was "direct", but gitea.com blocks CI/datacenter traffic with 403s
# (e.g. when fetching the gitea.com/gitea/go-xsd-duration replace module),
# breaking Docker builds. Going through proxy.golang.org avoids hitting
# gitea.com directly since the proxy serves modules from its cache.
ARG GOPROXY
ENV GOPROXY=${GOPROXY:-https://proxy.golang.org,direct}

# The golang images set GOTOOLCHAIN=local, which ignores the toolchain
# directive in go.mod. Override it so the build uses the pinned toolchain.
ENV GOTOOLCHAIN=go1.25.12+auto

ARG GITEA_VERSION
ARG TAGS="sqlite sqlite_unlock_notify"
ENV TAGS="bindata timetzdata $TAGS"
ARG CGO_EXTRA_CFLAGS

# Build deps
# pnpm bootstrap is age-gated with --min-release-age to match the 14-day
# minimumReleaseAge policy in pnpm-workspace.yaml (repo .npmrc is not yet
# copied at this point). pnpm >= 10.26.0 is required.
RUN apk --no-cache add \
    build-base \
    git \
    nodejs \
    npm \
    && npm install -g --min-release-age=14 "pnpm@^10.26.0" \
    && rm -rf /var/cache/apk/*

# Setup repo
COPY . ${GOPATH}/src/code.gitea.io/gitea
WORKDIR ${GOPATH}/src/code.gitea.io/gitea

# Checkout version if set
RUN if [ -n "${GITEA_VERSION}" ]; then git checkout "${GITEA_VERSION}"; fi \
 && make clean-all build

# Begin env-to-ini build
RUN go build contrib/environment-to-ini/environment-to-ini.go

# Copy local files
COPY docker/root /tmp/local

# Set permissions
RUN chmod 755 /tmp/local/usr/bin/entrypoint \
              /tmp/local/usr/local/bin/gitea \
              /tmp/local/etc/s6/gitea/* \
              /tmp/local/etc/s6/openssh/* \
              /tmp/local/etc/s6/.s6-svscan/* \
              /go/src/code.gitea.io/gitea/gitea \
              /go/src/code.gitea.io/gitea/environment-to-ini

FROM docker.io/library/alpine:3.22
LABEL maintainer="maintainers@gitea.io"

EXPOSE 22 3000

RUN apk --no-cache add \
    bash \
    ca-certificates \
    curl \
    gettext \
    git \
    linux-pam \
    openssh \
    s6 \
    sqlite \
    su-exec \
    gnupg \
    && rm -rf /var/cache/apk/*

RUN addgroup \
    -S -g 1000 \
    git && \
  adduser \
    -S -H -D \
    -h /data/git \
    -s /bin/bash \
    -u 1000 \
    -G git \
    git && \
  echo "git:*" | chpasswd -e

ENV USER=git
ENV GITEA_CUSTOM=/data/gitea

VOLUME ["/data"]

ENTRYPOINT ["/usr/bin/entrypoint"]
CMD ["/usr/bin/s6-svscan", "/etc/s6"]

COPY --from=build-env /tmp/local /
COPY --from=build-env /go/src/code.gitea.io/gitea/gitea /app/gitea/gitea
COPY --from=build-env /go/src/code.gitea.io/gitea/environment-to-ini /usr/local/bin/environment-to-ini
