#!/bin/sh
# Approved maintenance window only; build separately before taking traffic down.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
COMPOSE_FILE=${COMPOSE_FILE:-$ROOT/docker-compose.yml}
PROJECT=${COMPOSE_PROJECT_NAME:-newapi}
CONTAINER=${CONTAINER:-llm-gateway}
POSTGRES_CONTAINER=${POSTGRES_CONTAINER:-new-api-postgres}
REDIS_CONTAINER=${REDIS_CONTAINER:-new-api-redis}
HEALTH_TIMEOUT=${HEALTH_TIMEOUT:-120}
# Fixed tag: each deploy moves it to the previous image; .deploy-state keeps the ID.
ROLLBACK_TAG=${ROLLBACK_TAG:-llm-gateway:rollback}
fail() { echo "deploy: $*" >&2; exit 1; }
compose() { docker compose -p "$PROJECT" -f "$COMPOSE_FILE" "$@"; }
# Executable gate: enter rejects NEW relay traffic without dropping established
# streams and durably preserves webhooks; check proves workers/refunds quiescent.
: "${MAINTENANCE_GATE:?required executable supporting enter/check/leave}"
: "${PAYMENT_GATE:?required executable supporting enter/check/leave for webhook replay and worker recovery}"
: "${BUSINESS_SMOKE:?required executable verifying test-key SSE/quota/workers}"
: "${CANDIDATE_IMAGE:?required local sha256 image ID}"
case "$CANDIDATE_IMAGE" in sha256:*) ;; *) fail 'candidate must be an immutable local image ID';; esac
[ -x "$MAINTENANCE_GATE" ] && [ -x "$PAYMENT_GATE" ] && [ -x "$BUSINESS_SMOKE" ] || fail 'maintenance/payment gate and smoke must be executable'
command -v flock >/dev/null || fail 'flock required'
exec 9>"${DEPLOY_LOCK:-$ROOT/.deploy.lock}"
flock -n 9 || fail 'another rollout holds the deployment lock'
compose config --quiet
candidate=$(docker image inspect "$CANDIDATE_IMAGE" --format '{{.Id}}')
[ "$candidate" = "$CANDIDATE_IMAGE" ] || fail 'candidate ID mismatch'
old=$(docker inspect "$CONTAINER" --format '{{.Image}}')
[ "$(docker inspect "$CONTAINER" --format '{{index .Config.Labels "com.docker.compose.project"}}')" = "$PROJECT" ] || fail 'container belongs to a different Compose project'
[ "$(docker inspect "$CONTAINER" --format '{{index .Config.Labels "com.docker.compose.service"}}')" = llm-gateway ] || fail 'container is not the gateway service'
pg=$(docker inspect "$POSTGRES_CONTAINER" --format '{{.Id}}')
redis=$(docker inspect "$REDIS_CONTAINER" --format '{{.Id}}')
state=${DEPLOY_STATE:-$ROOT/.deploy-state}
[ ! -e "$state" ] || fail "unresolved previous rollout: $state"
umask 077
printf 'previous=%s\ncandidate=%s\n' "$old" "$candidate" > "$state"
printf 'project=%s\ncontainer=%s\npostgres=%s\nredis=%s\n' "$PROJECT" "$CONTAINER" "$pg" "$redis" >> "$state"
docker image tag "$old" "$ROLLBACK_TAG"
gated=false
payment_gated=false
changed=false
committed=false
deps() {
  [ "$(docker inspect "$POSTGRES_CONTAINER" --format '{{.Id}}')" = "$pg" ] &&
  [ "$(docker inspect "$REDIS_CONTAINER" --format '{{.Id}}')" = "$redis" ]
}
wait_ready() {
  deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if [ "$(docker inspect "$CONTAINER" --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}')" = healthy ]; then
      curl --fail --silent --max-time 3 "${BASE_URL:-http://127.0.0.1:3000}/readyz" | grep -q '"status":"ready"' && return 0
    fi
    sleep 1
  done
  return 1
}
replace() {
  # Never SIGKILL billing. A hung shutdown requires intervention, not a second worker.
  if docker inspect "$CONTAINER" >/dev/null 2>&1; then
    docker stop --timeout=-1 "$CONTAINER" >/dev/null || return 1
    [ "$(docker inspect "$CONTAINER" --format '{{.State.Running}}')" = false ] || return 1
  fi
  LLM_GATEWAY_IMAGE="$1" compose up -d --no-deps --pull never --force-recreate llm-gateway || return 1
  [ "$(docker inspect "$CONTAINER" --format '{{.Image}}')" = "$1" ] || return 1
  deps && wait_ready
}
finish() {
  rc=$?
  trap - EXIT HUP INT TERM
  if [ "$committed" = false ] && [ "$changed" = true ]; then
    echo 'deploy: attempting rollback; maintenance remains active' >&2
    if ! replace "$old" || ! "$BUSINESS_SMOKE" || ! deps; then
      echo "deploy: ROLLBACK FAILED; keep maintenance active; recovery state: $state" >&2
      exit 1
    fi
  fi
  if [ "$gated" = true ]; then
    "$MAINTENANCE_GATE" leave || { echo "deploy: maintenance gate release failed; recovery state: $state" >&2; exit 1; }
  fi
  if [ "$payment_gated" = true ]; then
    "$PAYMENT_GATE" leave || { echo "deploy: payment gate release/replay failed; recovery state: $state" >&2; exit 1; }
  fi
  if [ "$rc" = 0 ]; then mv "$state" "$state.completed-$(date +%s)-$$"; fi
  exit "$rc"
}
trap finish EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP
payment_gated=true
"$PAYMENT_GATE" enter
"$PAYMENT_GATE" check
gated=true
"$MAINTENANCE_GATE" enter
"$MAINTENANCE_GATE" check
changed=true
replace "$candidate" || fail 'candidate failed readiness/identity checks'
"$BUSINESS_SMOKE" || fail 'business smoke failed'
deps || fail 'dependency identity changed'
committed=true
echo "deploy: verified candidate $candidate; previous $old"
