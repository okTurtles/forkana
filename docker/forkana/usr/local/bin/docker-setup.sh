#!/bin/bash
# Forkana Docker Setup Script
# Based on docker/rootless setup with Forkana-specific defaults

# Prepare git folder
mkdir -p ${HOME} && chmod 0700 ${HOME}
if [ ! -w ${HOME} ]; then echo "${HOME} is not writable"; exit 1; fi

# Prepare custom folder
mkdir -p ${GITEA_CUSTOM} && chmod 0700 ${GITEA_CUSTOM}

# Prepare temp folder
mkdir -p ${GITEA_TEMP} && chmod 0700 ${GITEA_TEMP}
if [ ! -w ${GITEA_TEMP} ]; then echo "${GITEA_TEMP} is not writable"; exit 1; fi

# Prepare config file
if [ ! -f ${GITEA_APP_INI} ]; then

    # Prepare config file folder
    GITEA_APP_INI_DIR=$(dirname ${GITEA_APP_INI})
    mkdir -p ${GITEA_APP_INI_DIR} && chmod 0700 ${GITEA_APP_INI_DIR}
    if [ ! -w ${GITEA_APP_INI_DIR} ]; then echo "${GITEA_APP_INI_DIR} is not writable"; exit 1; fi

    # Set INSTALL_LOCK to true only if SECRET_KEY is not empty and
    # INSTALL_LOCK is empty
    if [ -n "$SECRET_KEY" ] && [ -z "$INSTALL_LOCK" ]; then
        INSTALL_LOCK=true
    fi

    # Forkana-specific defaults (PostgreSQL, production mode)
    APP_NAME=${APP_NAME:-"Forkana"} \
    RUN_MODE=${RUN_MODE:-"prod"} \
    RUN_USER=${USER:-"git"} \
    DOMAIN=${DOMAIN:-"localhost"} \
    SSH_DOMAIN=${SSH_DOMAIN:-"localhost"} \
    HTTP_PORT=${HTTP_PORT:-"3000"} \
    ROOT_URL=${ROOT_URL:-""} \
    DISABLE_SSH=${DISABLE_SSH:-"true"} \
    SSH_PORT=${SSH_PORT:-"2222"} \
    SSH_LISTEN_PORT=${SSH_LISTEN_PORT:-$SSH_PORT} \
    LFS_START_SERVER=${LFS_START_SERVER:-"true"} \
    DB_TYPE=${DB_TYPE:-"postgres"} \
    DB_HOST=${DB_HOST:-"postgres:5432"} \
    DB_NAME=${DB_NAME:-"forkana"} \
    DB_USER=${DB_USER:-"forkana"} \
    DB_PASSWD=${DB_PASSWD:-""} \
    DB_SSL_MODE=${DB_SSL_MODE:-"disable"} \
    INSTALL_LOCK=${INSTALL_LOCK:-"true"} \
    DISABLE_REGISTRATION=${DISABLE_REGISTRATION:-"true"} \
    REQUIRE_SIGNIN_VIEW=${REQUIRE_SIGNIN_VIEW:-"false"} \
    SECRET_KEY=${SECRET_KEY:-""} \
    envsubst < /etc/templates/app.ini > ${GITEA_APP_INI}
fi

# Replace app.ini settings with env variables in the form GITEA__SECTION_NAME__KEY_NAME
environment-to-ini --config ${GITEA_APP_INI}

