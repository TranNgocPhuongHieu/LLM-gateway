#!/bin/sh
set -eu
# Fail only the candidate's first check; permit the rollback smoke to run.
if [ "${FAIL_CANDIDATE_SMOKE:-0}" = 1 ] && [ ! -e "$SMOKE_FAILURE_MARKER" ]; then
  touch "$SMOKE_FAILURE_MARKER"
  exit 1
fi
exec python3 "$SMOKE_SCRIPT"
