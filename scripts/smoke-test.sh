#!/bin/sh
set -eu

BASE_URL=${BASE_URL:-http://127.0.0.1:3000}
PUBLIC_URL=${PUBLIC_URL:-https://llm.trnhieu.dev/quota}
CONTAINER=${CONTAINER:-llm-gateway}

fail() { echo "smoke-test: $*" >&2; exit 1; }

ready_body=$(curl --fail --silent --show-error --max-time "${HTTP_TIMEOUT:-10}" "$BASE_URL/readyz") || fail "readyz request failed"
printf '%s' "$ready_body" | grep -q '"status":"ready"' || fail "readyz did not report ready"

public_html=$(mktemp)
trap 'rm -f "$public_html"' EXIT HUP INT TERM
public_status=$(curl --silent --show-error --max-time "${HTTP_TIMEOUT:-20}" -o "$public_html" -w '%{http_code}' "$PUBLIC_URL") || fail "public quota request failed"
[ "$public_status" = 200 ] || fail "public quota returned HTTP $public_status"
grep -qi '<html' "$public_html" || fail "public quota did not return HTML"

container_state=$(docker inspect "$CONTAINER" --format '{{.State.Status}}')
[ "$container_state" = running ] || fail "$CONTAINER state is $container_state"
container_health=$(docker inspect "$CONTAINER" --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}')
[ "$container_health" = healthy ] || fail "$CONTAINER health is $container_health"

echo "smoke-test: readyz, public quota, and container health passed"
