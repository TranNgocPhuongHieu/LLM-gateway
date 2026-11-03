#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-$ROOT/docker-compose.yml}
PROJECT=${COMPOSE_PROJECT_NAME:-newapi}
SERVICE=${SERVICE:-llm-gateway}
IMAGE=${IMAGE:-llm-gateway:latest}
ROLLBACK_TAG=${ROLLBACK_TAG:-llm-gateway:rollback}
HEALTH_TIMEOUT=${HEALTH_TIMEOUT:-90}

fail() { echo "deploy: $*" >&2; exit 1; }

command -v docker >/dev/null 2>&1 || fail "docker is required"
[ -f "$COMPOSE_FILE" ] || fail "compose file not found: $COMPOSE_FILE"

compose() { docker compose -p "$PROJECT" -f "$COMPOSE_FILE" "$@"; }

compose config --quiet
old_image_id=$(docker inspect "$SERVICE" --format '{{.Image}}') || fail "running service $SERVICE not found"
postgres_id=$(docker inspect new-api-postgres --format '{{.Id}}') || fail "new-api-postgres not found"
redis_id=$(docker inspect new-api-redis --format '{{.Id}}') || fail "new-api-redis not found"
docker image tag "$old_image_id" "$ROLLBACK_TAG"

rollback() {
  echo "deploy: smoke test failed; rolling back $SERVICE" >&2
  docker image tag "$ROLLBACK_TAG" "$IMAGE"
  compose up -d --no-deps --force-recreate "$SERVICE" >/dev/null
  "$ROOT/source/scripts/smoke-test.sh" || echo "deploy: rollback smoke test failed" >&2
}

if ! docker build --tag "$IMAGE" "$ROOT/source"; then
  fail "image build failed; running service was not changed"
fi

if ! compose up -d --no-deps --force-recreate "$SERVICE"; then
  rollback
  fail "service recreate failed"
fi

deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))
while :; do
  health=$(docker inspect "$SERVICE" --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}')
  [ "$health" = healthy ] && break
  [ "$health" = unhealthy ] && { rollback; fail "service became unhealthy"; }
  [ "$(date +%s)" -ge "$deadline" ] && { rollback; fail "healthcheck timed out"; }
  sleep 2
done

if ! "$ROOT/source/scripts/smoke-test.sh"; then
  rollback
  fail "post-deploy smoke test failed"
fi

[ "$(docker inspect new-api-postgres --format '{{.Id}}')" = "$postgres_id" ] || fail "postgres container changed"
[ "$(docker inspect new-api-redis --format '{{.Id}}')" = "$redis_id" ] || fail "redis container changed"

echo "deploy: completed successfully; $SERVICE updated, postgres and redis unchanged"
