#!/usr/bin/env python3
"""Isolated payment gate fixture; never use as the production payment gate."""
import os
import sys
from pathlib import Path

state = Path(os.environ["PAYMENT_GATE_STATE"])
action = sys.argv[1]
if action == "enter":
    state.write_text("paused\n")
elif action == "check":
    if os.environ.get("FAIL_PAYMENT_GATE") == "1":
        raise SystemExit(1)
    if not state.exists() or state.read_text() != "paused\n":
        raise SystemExit("payment gate was not entered")
elif action == "leave":
    state.write_text("released\n")
else:
    raise SystemExit("unknown action")
