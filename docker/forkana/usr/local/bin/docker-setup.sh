#!/bin/bash
# Forkana Docker Setup Script
# Based on docker/rootless setup with Forkana-specific defaults

# Prepare git folder
mkdir -p ${HOME} && chmod 0700 ${HOME}
if [ ! -w ${HOME} ]; then echo "${HOME} is not writable"; exit 1; fi

# GITEA_CUSTOM is baked into the image (see Dockerfile) and points outside the
# volume paths, so no per-boot sync into the data volume is needed; each image
# already ships its matching custom overrides.

# Prepare temp folder
mkdir -p ${GITEA_TEMP} && chmod 0700 ${GITEA_TEMP}
if [ ! -w ${GITEA_TEMP} ]; then echo "${GITEA_TEMP} is not writable"; exit 1; fi

# Prepare config file
if [ ! -f ${GITEA_APP_INI} ]; then

    # Prepare config file folder
    GITEA_APP_INI_DIR=$(dirname ${GITEA_APP_INI})
    mkdir -p ${GITEA_APP_INI_DIR} && chmod 0700 ${GITEA_APP_INI_DIR}
    if [ ! -w ${GITEA_APP_INI_DIR} ]; then echo "${GITEA_APP_INI_DIR} is not writable"; exit 1; fi

    # Defaults for settings still using $VARIABLE placeholders in the template.
    # Settings managed by GITEA__-prefixed env vars (server, database, security,
    # service) use hardcoded defaults in the template and are overridden by
    # environment-to-ini on every startup.
    APP_NAME=${APP_NAME:-"Forkana"} \
    RUN_MODE=${RUN_MODE:-"prod"} \
    RUN_USER=${USER:-"git"} \
    SSH_DOMAIN=${SSH_DOMAIN:-"localhost"} \
    HTTP_PORT=${HTTP_PORT:-"3000"} \
    START_SSH_SERVER=${START_SSH_SERVER:-"false"} \
    SSH_PORT=${SSH_PORT:-"2222"} \
    SSH_LISTEN_PORT=${SSH_LISTEN_PORT:-$SSH_PORT} \
    LFS_START_SERVER=${LFS_START_SERVER:-"true"} \
    REQUIRE_SIGNIN_VIEW=${REQUIRE_SIGNIN_VIEW:-"false"} \
    envsubst < /etc/templates/app.ini > ${GITEA_APP_INI}
fi

# Replace app.ini settings with env variables in the form GITEA__SECTION_NAME__KEY_NAME
environment-to-ini --config ${GITEA_APP_INI}

