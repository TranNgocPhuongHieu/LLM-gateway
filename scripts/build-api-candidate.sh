#!/bin/sh
# Export a reviewable API-only snapshot without changing the dirty worktree.
set -eu
SOURCE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT=${1:?usage: build-api-candidate.sh NEW_OUTPUT_DIRECTORY}
if [ -e "$OUT" ]; then
  echo "output already exists: $OUT" >&2
  exit 1
fi
mkdir -p "$OUT/context"
git -C "$SOURCE" rev-parse HEAD > "$OUT/base-commit"
git -C "$SOURCE" archive HEAD | tar -x -C "$OUT/context"
# Runtime allowlist: no billing, quota, payment, drain, or deployment changes.
for file in controller/relay.go logger/logger.go relay/channel/api_request.go \
  controller/relay_stream_fallback_test.go logger/logger_concurrency_test.go \
  relay/channel/first_response_timeout_test.go \
  relay/channel/openai/relay_stream_stress_test.go \
  relay/channel/openai/helper.go relay/channel/claude/relay-claude.go \
  relay/helper/common.go relay/helper/stream_scanner.go relay/helper/stream_scanner_test.go; do
  cp "$SOURCE/$file" "$OUT/context/$file"
  printf '%s\n' "$file" >> "$OUT/overlay-files"
done
# Keep the pre-existing production Dockerfile and deployment scripts at HEAD.
git -C "$SOURCE" diff HEAD -- controller/relay.go logger/logger.go \
  relay/channel/api_request.go relay/channel/openai/helper.go \
  relay/channel/claude/relay-claude.go relay/helper/common.go \
  relay/helper/stream_scanner.go > "$OUT/runtime.patch"
(cd "$OUT/context" && find . -type f -print0 | sort -z | xargs -0 sha256sum) > "$OUT/source.sha256"
printf 'API-only context: %s/context\n' "$OUT"
printf 'Review and test this context before building; no deployment is performed.\n'
