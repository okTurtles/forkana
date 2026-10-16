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

# Outgoing mail goes through msmtp (sendmail protocol) and is enabled only when an
# operator has placed a relay config in the config volume. The file holds the relay
# password, so msmtp requires it to be owned by us and not readable by others.
# Values already set via GITEA__mailer__* in the environment take precedence, and
# ENABLED is always written so removing the file turns mail off on the next boot.
MSMTP_CONFIG=$(dirname ${GITEA_APP_INI})/msmtprc
if [ -f "${MSMTP_CONFIG}" ] && [ ! -O "${MSMTP_CONFIG}" ]; then
    echo "${MSMTP_CONFIG} is not owned by $(id -un); outgoing mail stays disabled" >&2
    export GITEA__mailer__ENABLED=${GITEA__mailer__ENABLED:-false}
elif [ -f "${MSMTP_CONFIG}" ]; then
    chmod 0600 "${MSMTP_CONFIG}"
    export GITEA__mailer__ENABLED=${GITEA__mailer__ENABLED:-true}
    export GITEA__mailer__PROTOCOL=${GITEA__mailer__PROTOCOL:-sendmail}
    export GITEA__mailer__SENDMAIL_PATH=${GITEA__mailer__SENDMAIL_PATH:-/usr/bin/msmtp}
    export GITEA__mailer__SENDMAIL_ARGS=${GITEA__mailer__SENDMAIL_ARGS:-"--file=${MSMTP_CONFIG}"}
    if [ -z "${GITEA__mailer__FROM}" ]; then
        # Default to the relay account's sender so the envelope matches what the relay allows.
        MSMTP_FROM=$(awk '$1 == "from" { print $2; exit }' "${MSMTP_CONFIG}")
        if [ -n "${MSMTP_FROM}" ]; then
            export GITEA__mailer__FROM="${APP_NAME:-Forkana} <${MSMTP_FROM}>"
        fi
    fi
else
    export GITEA__mailer__ENABLED=${GITEA__mailer__ENABLED:-false}
fi

# Replace app.ini settings with env variables in the form GITEA__SECTION_NAME__KEY_NAME
environment-to-ini --config ${GITEA_APP_INI}

