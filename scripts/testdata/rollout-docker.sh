#!/bin/sh
set -eu
# Test-only fault injection after Docker has removed the stopped old container.
case "$CONTAINER" in llm-gateway-rehearsal-*) ;; *) exit 99;; esac
case " $* " in
  *' compose '*' up '*)
    if [ "${FAIL_RECREATE:-0}" = 1 ] && [ ! -e "$RECREATE_FAILURE_MARKER" ]; then
      touch "$RECREATE_FAILURE_MARKER"
      "$REAL_DOCKER" rm "$CONTAINER" >/dev/null
      exit 1
    fi
    ;;
esac
exec "$REAL_DOCKER" "$@"
