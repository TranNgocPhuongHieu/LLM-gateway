#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
case "$1" in enter) action=payment-enter;; check) action=payment-check;; leave) action=payment-leave;; *) exit 2;; esac
exec python3 "$ROOT/scripts/edge-gate.py" "$action"
