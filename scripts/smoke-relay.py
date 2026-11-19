#!/usr/bin/env python3
"""Opt-in billable smoke. Use a dedicated test key pinned to a mock channel.

No default URL/model/key; the operator must explicitly choose the environment.
Never print credentials, SSE payloads or customer data.
"""
import http.client
import json
import os
from pathlib import Path
from urllib.parse import urlsplit


def main():
    target = urlsplit(os.environ["SMOKE_BASE_URL"])
    if target.scheme not in ("http", "https") or target.username or target.query or target.path not in ("", "/"):
        raise ValueError("SMOKE_BASE_URL must be an HTTP(S) origin without credentials")
    key = Path(os.environ["SMOKE_KEY_FILE"]).read_text().strip()
    if not key or "\n" in key or "\r" in key:
        raise ValueError("invalid test key")
    cls = http.client.HTTPSConnection if target.scheme == "https" else http.client.HTTPConnection

    def request(path, payload=None, auth=True):
        conn = cls(target.hostname, target.port, timeout=int(os.environ.get("SMOKE_TIMEOUT", "240")))
        headers = {"Content-Type": "application/json"}
        if auth:
            headers["Authorization"] = "Bearer " + key
        try:
            conn.request("POST" if payload is not None else "GET", path, json.dumps(payload) if payload is not None else None, headers)
            resp = conn.getresponse()
            body = resp.read(2_000_001)
            if len(body) > 2_000_000:
                raise ValueError("smoke response exceeded bound")
            return resp.status, resp.getheader("Content-Type", ""), body
        finally:
            conn.close()

    def quota():
        status, _, body = request("/api/usage/token/")
        if status != 200:
            raise ValueError("quota endpoint failed")
        data = json.loads(body)["data"]
        return int(data["total_used"]), int(data["total_available"])

    if request("/readyz", auth=False)[0] != 200:
        raise ValueError("readiness failed")
    before = quota()
    payload = {"model": os.environ["SMOKE_MODEL"], "messages": [{"role": "user", "content": "Reply OK"}], "stream": True, "max_tokens": 8}
    if request("/v1/chat/completions", payload, auth=False)[0] not in (401, 403):
        raise ValueError("unauthenticated request was not rejected")
    status, content_type, body = request("/v1/chat/completions", payload)
    if status != 200 or "text/event-stream" not in content_type:
        raise ValueError("authenticated SSE status/type failed")
    events = [line[5:].strip() for line in body.decode().splitlines() if line.startswith("data:")]
    if events.count("[DONE]") != 1 or not events or events[-1] != "[DONE]":
        raise ValueError("SSE missing unique terminal DONE")
    token = False
    for event in events[:-1]:
        data = json.loads(event)
        if data.get("error"):
            raise ValueError("SSE error envelope")
        token |= any(choice.get("delta", {}).get("content") for choice in data.get("choices", []))
    if not token:
        raise ValueError("SSE emitted no content")
    after = quota()
    expected = int(os.environ["SMOKE_EXPECTED_QUOTA"])
    if expected < 0 or after[0] - before[0] != expected or before[1] - after[1] != expected:
        raise ValueError("test-key quota delta mismatch")
    if before[0] + before[1] != after[0] + after[1]:
        raise ValueError("quota conservation failed")
    print("smoke-relay: readiness, auth rejection, SSE terminal and quota delta PASS")


if __name__ == "__main__":
    main()
