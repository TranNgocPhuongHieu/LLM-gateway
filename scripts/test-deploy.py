#!/usr/bin/env python3
"""Docker rehearsal only: dedicated project, internal network, disposable PG/Redis.

Build .local-tests/rollout-fixture.test with Go 1.26 before running this script.
This is not a test of real SePay/Binance or the production gateway entrypoint.
"""
import concurrent.futures
import json
import os
from pathlib import Path
import subprocess
import shutil
import tempfile
import threading
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen

root = Path(__file__).resolve().parents[1]
# Fixed names (no PID/timestamp suffix) so reruns replace, never accumulate.
project = "llm-gateway-rehearsal"
tags = [project+":old", project+":new", project+":rollback"]
env = dict(os.environ, TEST_GATEWAY=project+"-gateway", TEST_POSTGRES=project+"-pg", TEST_REDIS=project+"-redis", TEST_PORT=os.environ.get("REHEARSAL_PORT", "39187"))
compose = ["docker", "compose", "-p", project, "-f", str(root/"scripts/testdata/rollout-compose.yml")]

def run(args, **kwargs):
    return subprocess.run(args, env=env, text=True, check=True, **kwargs)

def output(args):
    return run(args, stdout=subprocess.PIPE).stdout.strip()

def http(path, payload=None, auth=True):
    headers = {"Authorization": "Bearer local-test-key"} if auth else {}
    req = Request(env["BASE_URL"]+path, data=payload, headers=headers)
    try:
        with urlopen(req, timeout=20) as response:
            return response.status, response.read()
    except HTTPError as e:
        return e.code, e.read()

def ready():
    last = None
    for _ in range(150):
        try:
            last = http("/readyz")
            if last[0] == 200:
                return
        except OSError as e:
            last = str(e)
        time.sleep(.1)
    raise AssertionError(f"readiness timeout: {last}")

def image():
    return output(["docker", "inspect", env["TEST_GATEWAY"], "--format", "{{.Image}}"])

env.update(BASE_URL="http://127.0.0.1:"+env["TEST_PORT"], SMOKE_BASE_URL="http://127.0.0.1:"+env["TEST_PORT"], SMOKE_MODEL="mock", SMOKE_EXPECTED_QUOTA="140", SMOKE_SCRIPT=str(root/"scripts/smoke-relay.py"), MAINTENANCE_GATE=str(root/"scripts/testdata/rollout-gate.py"), BUSINESS_SMOKE=str(root/"scripts/testdata/rollout-smoke.sh"), CONTAINER=env["TEST_GATEWAY"], POSTGRES_CONTAINER=env["TEST_POSTGRES"], REDIS_CONTAINER=env["TEST_REDIS"], COMPOSE_FILE=str(root/"scripts/testdata/rollout-compose.yml"), COMPOSE_PROJECT_NAME=project, ROLLBACK_TAG=tags[2], HEALTH_TIMEOUT="30")
for p in ("scripts/testdata/rollout-gate.py", "scripts/testdata/rollout-payment-gate.py", "scripts/testdata/rollout-smoke.sh"):
    (root/p).chmod(0o755)
try:
    ids = []
    for tag in tags[:2]:
        rev = tag.rsplit(":", 1)[1]
        run(["docker", "build", "--build-arg", "REV="+rev, "-f", str(root/"scripts/testdata/rollout.Dockerfile"), "-t", tag, str(root/".local-tests")], stdout=subprocess.DEVNULL)
        ids.append(output(["docker", "image", "inspect", tag, "--format", "{{.Id}} "]).strip())
    env["LLM_GATEWAY_IMAGE"] = ids[0]
    run(compose+["up", "-d"], stdout=subprocess.DEVNULL)
    ready()
    # SIGTERM during active SSE exercises the production shutdown helper,
    # separately from the pre-recreate admission drain.
    with concurrent.futures.ThreadPoolExecutor() as pool:
        pending = pool.submit(http, "/v1/chat/completions", b"{}")
        time.sleep(.2)
        webhook_pending = pool.submit(http, "/api/payment/sepay/webhook", b"{}")
        assert http("/refund", b"{}")[0] == 202
        started = time.monotonic()
        run(["docker", "stop", "--timeout=-1", env["TEST_GATEWAY"]], stdout=subprocess.DEVNULL)
        assert pending.result()[1].count(b"[DONE]") == 1
        assert webhook_pending.result()[0] == 200
        assert output(["docker", "inspect", env["TEST_GATEWAY"], "--format", "{{.State.ExitCode}}"]) == "0"
        print(f"PASS: SIGTERM during active SSE/webhook/refund; clean exit after {time.monotonic()-started:.3f}s", flush=True)
    run(compose+["up", "-d", "--no-deps", "llm-gateway"], stdout=subprocess.DEVNULL)
    ready()
    with tempfile.TemporaryDirectory(prefix="llm-gateway-rollout-") as tmp:
        Path(tmp, "key").write_text("local-test-key")
        env.update(SMOKE_KEY_FILE=str(Path(tmp, "key")), DEPLOY_LOCK=str(Path(tmp, "lock")), SMOKE_FAILURE_MARKER=str(Path(tmp, "smoke-failed")), PAYMENT_GATE_STATE=str(Path(tmp, "payment-gate")), PAYMENT_GATE=str(root/"scripts/testdata/rollout-payment-gate.py"))
        # Script failure after gate entry MUST release drain, with no recreate.
        env.update(CANDIDATE_IMAGE=ids[1], DEPLOY_STATE=str(Path(tmp, "failed-gate")), FAIL_GATE_CHECK="1")
        failed = subprocess.run(["sh", str(root/"scripts/deploy.sh")], env=env)
        assert failed.returncode != 0 and image() == ids[0]
        assert Path(env["PAYMENT_GATE_STATE"]).read_text() == "released\n"
        ready()
        env.pop("FAIL_GATE_CHECK")
        print("PASS: gate-check failure released drain and preserved old instance", flush=True)
        # Keep a live SSE while deployment begins. The gate waits for it; webhook
        # and async refund are admitted and must survive the replacement.
        with concurrent.futures.ThreadPoolExecutor() as pool:
            stream = pool.submit(http, "/v1/chat/completions", b"{}")
            time.sleep(.2)
            webhook = pool.submit(http, "/api/payment/sepay/webhook", b"{}")
            assert http("/refund", b"{}")[0] == 202
            env["DEPLOY_STATE"] = str(Path(tmp, "success"))
            unavailable = []
            stop = threading.Event()
            def probe():
                while not stop.is_set():
                    try:
                        if http("/readyz")[0] != 200: unavailable.append(time.monotonic())
                    except OSError: unavailable.append(time.monotonic())
                    time.sleep(.05)
            watcher = threading.Thread(target=probe)
            watcher.start()
            started = time.monotonic()
            try:
                run(["sh", str(root/"scripts/deploy.sh")])
            finally:
                stop.set()
                watcher.join()
            status, body = stream.result()
            assert status == 200 and body.count(b"[DONE]") == 1
            assert webhook.result()[0] == 200
            assert image() == ids[1]
            outage = unavailable[-1]-unavailable[0] if len(unavailable)>1 else 0
            print(f"PASS: deploy with active SSE/webhook/refund; elapsed={time.monotonic()-started:.3f}s readiness-unavailable={outage:.3f}s (50ms sampling)", flush=True)
        # Rollback triggered by candidate business-smoke failure.
        env.update(CANDIDATE_IMAGE=ids[0], DEPLOY_STATE=str(Path(tmp, "rollback")), FAIL_CANDIDATE_SMOKE="1")
        failed = subprocess.run(["sh", str(root/"scripts/deploy.sh")], env=env)
        assert failed.returncode != 0 and image() == ids[1]
        ready()
        used = json.loads(http("/api/usage/token/")[1])["data"]
        assert used == {"total_used": 560, "total_available": 9440}, used
        delivered = output(["docker", "exec", env["TEST_REDIS"], "redis-cli", "get", "mock-webhook-delivered"])
        stopped = output(["docker", "exec", env["TEST_REDIS"], "redis-cli", "get", "mock-worker-stopped"])
        assert delivered == "2" and stopped == "4", (delivered, stopped)
        print("PASS: rollback identity/readiness/smoke; quota=9440; 4 settlements exactly once; refund net=0; webhooks=2; joined workers=4; no duplicate worker lock", flush=True)
        # Candidate recreation can fail after the old container was removed.
        # Rollback must still create the old IMAGE, not require the container.
        wrapper = Path(tmp, "bin")
        wrapper.mkdir()
        shutil.copyfile(root/"scripts/testdata/rollout-docker.sh", wrapper/"docker")
        (wrapper/"docker").chmod(0o755)
        env.pop("FAIL_CANDIDATE_SMOKE")
        env.update(REAL_DOCKER=shutil.which("docker"), PATH=str(wrapper)+os.pathsep+env["PATH"], FAIL_RECREATE="1", RECREATE_FAILURE_MARKER=str(Path(tmp, "recreate-failed")), DEPLOY_STATE=str(Path(tmp, "missing-container")))
        failed = subprocess.run(["sh", str(root/"scripts/deploy.sh")], env=env)
        assert failed.returncode != 0 and image() == ids[1]
        ready()
        used = json.loads(http("/api/usage/token/")[1])["data"]
        assert used == {"total_used": 700, "total_available": 9300}, used
        print("PASS: rollback after removal/recreate failure; readiness recovered; final quota=9300 (5 x 140), no duplicate refund", flush=True)
finally:
    if "LLM_GATEWAY_IMAGE" in env:
        subprocess.run(compose+["logs", "--no-color", "llm-gateway"], env=env)
        # This dedicated project owns only disposable resources created above.
        subprocess.run(compose+["down", "--volumes"], env=env)
    # Fixture images are disposable; only this project's tags are removed.
    subprocess.run(["docker", "image", "rm", "--force", *tags], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
