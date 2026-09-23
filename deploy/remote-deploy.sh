#!/usr/bin/env bash
# Remote deploy script for alpha_proxy.
# Runs on the production server. Expects to be invoked from the deploy workflow
# with three arguments: SHA, API image, ML image.
#
# Usage:
#   remote-deploy.sh <sha> <api_image> <ml_image>
#
# The script never reads or prints the contents of /opt/alpha-proxy/.env.

set -Eeuo pipefail

DEPLOY_DIR="/opt/alpha-proxy"
ENV_FILE="$DEPLOY_DIR/.env"
DEPLOY_ENV_FILE="$DEPLOY_DIR/deploy.env"
COMPOSE_FILE="$DEPLOY_DIR/compose.yaml"
TMP_DIR="$(mktemp -d)"
BACKUP_COMPOSE="$TMP_DIR/compose.yaml.bak"
BACKUP_DEPLOY_ENV="$TMP_DIR/deploy.env.bak"
DEPLOY_ENV_TMP="$DEPLOY_DIR/deploy.env.tmp"

TRANSACTION_ACTIVE=0
HAD_DEPLOY_ENV=0

cleanup() {
  rm -rf "$TMP_DIR"
  rm -f "$DEPLOY_ENV_TMP"
}
trap cleanup EXIT

log() {
  echo "[deploy] $*"
}

fail() {
  echo "[deploy] ERROR: $*" >&2
  exit 1
}

rollback() {
  # Disable the ERR trap to avoid recursion while rolling back.
  trap - ERR
  log "rolling back to previous version"
  if [ -f "$BACKUP_COMPOSE" ]; then
    cp "$BACKUP_COMPOSE" "$COMPOSE_FILE"
  fi
  if [ "$HAD_DEPLOY_ENV" -eq 1 ]; then
    cp "$BACKUP_DEPLOY_ENV" "$DEPLOY_ENV_FILE"
    chmod 600 "$DEPLOY_ENV_FILE"
    docker compose --project-directory "$DEPLOY_DIR" \
      --env-file "$ENV_FILE" --env-file "$DEPLOY_ENV_FILE" \
      -f "$COMPOSE_FILE" up -d --no-build --wait --wait-timeout 240 \
      || log "rollback compose up failed; manual intervention required"
  else
    rm -f "$DEPLOY_ENV_FILE"
    docker compose --project-directory "$DEPLOY_DIR" \
      --env-file "$ENV_FILE" \
      -f "$COMPOSE_FILE" up -d --no-build --wait --wait-timeout 240 \
      || log "rollback compose up failed; manual intervention required"
  fi
}

on_error() {
  local rc=$?
  if [ "$TRANSACTION_ACTIVE" -eq 1 ]; then
    TRANSACTION_ACTIVE=0
    rollback
  fi
  exit "$rc"
}

# --- Arguments -------------------------------------------------------------
if [ "$#" -ne 3 ]; then
  fail "usage: remote-deploy.sh <sha> <api_image> <ml_image>"
fi
SHA="$1"
API_IMAGE="$2"
ML_IMAGE="$3"

# --- Preconditions ---------------------------------------------------------
if ! [[ "$SHA" =~ ^[0-9a-f]{40}$ ]]; then
  fail "SHA must be exactly 40 lowercase hex characters"
fi
[ -f "$ENV_FILE" ] || fail "$ENV_FILE does not exist; create it before deploying"
[ -f "$COMPOSE_FILE" ] || fail "$COMPOSE_FILE does not exist; a baseline version is required for rollback"
[ -f "$DEPLOY_DIR/compose.yaml.new" ] || fail "compose.yaml.new was not uploaded by the workflow"
command -v docker >/dev/null 2>&1 || fail "docker is not installed"
docker compose version >/dev/null 2>&1 || fail "docker compose is not available"

# --- Backup current state --------------------------------------------------
cp "$COMPOSE_FILE" "$BACKUP_COMPOSE"
if [ -f "$DEPLOY_ENV_FILE" ]; then
  HAD_DEPLOY_ENV=1
  cp "$DEPLOY_ENV_FILE" "$BACKUP_DEPLOY_ENV"
fi

# --- Transaction begins ----------------------------------------------------
TRANSACTION_ACTIVE=1
trap on_error ERR

# --- Install new compose.yaml atomically -----------------------------------
mv "$DEPLOY_DIR/compose.yaml.new" "$COMPOSE_FILE"

# --- Write deploy.env atomically -------------------------------------------
umask 077
cat > "$DEPLOY_ENV_TMP" <<EOF
API_IMAGE=$API_IMAGE:$SHA
ML_IMAGE=$ML_IMAGE:$SHA
EOF
chmod 600 "$DEPLOY_ENV_TMP"
mv "$DEPLOY_ENV_TMP" "$DEPLOY_ENV_FILE"

# --- Pull and start --------------------------------------------------------
log "pulling images"
docker compose --project-directory "$DEPLOY_DIR" \
  --env-file "$ENV_FILE" --env-file "$DEPLOY_ENV_FILE" \
  -f "$COMPOSE_FILE" pull api ml

log "starting services"
docker compose --project-directory "$DEPLOY_DIR" \
  --env-file "$ENV_FILE" --env-file "$DEPLOY_ENV_FILE" \
  -f "$COMPOSE_FILE" up -d --no-build --wait --wait-timeout 240

# --- Smoke test ------------------------------------------------------------
log "smoke test: /healthz"
docker compose --project-directory "$DEPLOY_DIR" \
  --env-file "$ENV_FILE" --env-file "$DEPLOY_ENV_FILE" \
  -f "$COMPOSE_FILE" exec -T api curl --fail --silent --show-error \
  http://127.0.0.1:8080/healthz

log "smoke test: /readyz"
docker compose --project-directory "$DEPLOY_DIR" \
  --env-file "$ENV_FILE" --env-file "$DEPLOY_ENV_FILE" \
  -f "$COMPOSE_FILE" exec -T api curl --fail --silent --show-error \
  http://127.0.0.1:8080/readyz

# --- Transaction complete --------------------------------------------------
TRANSACTION_ACTIVE=0
trap - ERR

log "deploy of $SHA completed successfully"
