#!/bin/sh
set -eu
SOURCE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
context=$(mktemp -d)
trap 'rm -rf "$context"' EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM
git -C "$SOURCE" archive HEAD | tar -x -C "$context"
# Exact working-tree files include staged changes; exclude credentials and affinity edits.
for file in main.go controller/relay.go controller/misc.go controller/drain.go \
  logger/logger.go model/user.go model/token.go model/utils.go common/shutdown.go \
  relay/channel/api_request.go service/billing_session.go service/funding_source.go \
  service/system_task.go router/main.go; do
  cp "$SOURCE/$file" "$context/$file"
done
tag="llm-gateway:candidate"
docker build --tag "$tag" "$context" >&2
docker image inspect "$tag" --format '{{.Id}}'
