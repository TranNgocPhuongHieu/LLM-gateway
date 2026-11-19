#!/usr/bin/env python3
"""Mock gate ONLY. Real production gate additionally needs durable webhook delivery."""
import json
import os
import sys
import time
from urllib.request import Request, urlopen

action = {"enter": "begin", "leave": "cancel", "check": "status"}[sys.argv[1]]
def call():
    req = Request(os.environ["BASE_URL"]+"/internal/drain?action="+action, method="POST", headers={"X-Drain-Token": "local-drain-test"})
    with urlopen(req, timeout=3) as response:
        return json.load(response)
if action != "status":
    call()
elif os.environ.get("FAIL_GATE_CHECK") == "1":
    sys.exit(1)
else:
    deadline = time.monotonic() + 15
    while call()["active_requests"]:
        if time.monotonic() > deadline:
            sys.exit(1)
        time.sleep(.05)
