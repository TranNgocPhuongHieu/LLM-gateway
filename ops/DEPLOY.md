# LLM Gateway deployment

The gateway is deployed as the `llm-gateway` service from the external Compose
project in the parent `LLM-gateway/` directory. Single-instance updates require
an approved maintenance window; this is not a zero-downtime deployment.

From `LLM-gateway/`:

```sh
# Local build only; prints an immutable image ID and never starts the gateway.
sh source/scripts/build-candidate.sh
```

The build uses an explicit runtime-file allowlist over `git archive HEAD`,
including staged contents, excluding dirty affinity settings and credentials.
Review that allowlist whenever the approved runtime patch changes. Keep the
printed `sha256:...` ID; do not pass a mutable tag to the deployment script.

## Required gates (no defaults)

`scripts/deploy.sh` requires `CANDIDATE_IMAGE`, `MAINTENANCE_GATE`,
`PAYMENT_GATE`, and `BUSINESS_SMOKE`. These are paths to reviewed executable
files, not shell command strings. The script refuses to proceed without them.
No production gate is supplied by this patch.

`scripts/edge-gate.py` is the reference single-host gate implementation for an
approved first rollout. Start it as a separate process with a persistent local
SQLite path, route only `llm.trnhieu.dev` to its listening port, and point its
backend at the existing gateway. Set `EDGE_GATE_TOKEN` and use the same script
for the reviewed `enter`, `check`, and `leave` wrappers:
`scripts/edge-maintenance-gate.sh` (`MAINTENANCE_GATE`) and
`scripts/edge-payment-gate.sh` (`PAYMENT_GATE`). Its local test is
`python3 scripts/test-edge-gate.py` (loopback ports only). Do not expose its
control paths publicly. The script is not activated by Compose/Cloudflared in
this change; wiring that route is an explicit production operation.

The gate must implement:

- `enter`: reject new relay traffic without terminating established SSE or
  WebSocket connections. Durably retain payment callbacks for replay, or use a
  provider-approved pause/retry procedure verified for every payment method.
  Never acknowledge a callback that has not been durably stored/settled.
- `check`: prove zero active application requests, no in-flight payment
  transactions or refunds, and no active task claims on the OLD runtime.
- `leave`: reopen traffic idempotently, including after a partial `enter`.

`PAYMENT_GATE` is separate and fail-closed. Its `enter` must stop new payment
callbacks from being acknowledged during the listener outage and durably spool
them (or prove a provider-supported pause). Its `check` must prove that the
spool is durable/replayable and that Binance checkpoint/lease recovery is
healthy. Its `leave` must replay the spool to the new listener and fail if any
callback is not durably acknowledged. An in-memory queue or a shell flag is not
evidence.

The reference edge gate stores callback body, path, and headers in SQLite using
a SHA-256 event key (`insert or ignore`), acknowledges only after the durable
write, and replays pending rows before `leave` succeeds. It is a delivery
buffer, not a replacement for provider verification or the existing idempotent
settlement code. Back up/protect the spool path and monitor replay failures.

Payment-path evidence from source: SePay IPN records authenticated provider
identifiers before upstream verification and returns 503 on transient failures;
the minute reconciler retries pending rows with durable `next_reconcile_at` and
up to five attempts. The bank webhook handler is idempotent but has no local
durable spool or application retry. No SePay provider retry policy is configured
in this repository, so delivery after listener downtime is `UNKNOWN` until the
merchant dashboard/contract is verified. Binance polling persists a per-network
checkpoint with a lease and a 30-minute overlap, so a restart re-reads history
without double settlement; provider/API recovery is still a gate check.

For the FIRST rollout the old process lacks the drain endpoint. Use an
external, hostname-scoped admission proxy as `MAINTENANCE_GATE`: `enter` must
reject only new API/relay requests, preserve established SSE/WebSocket
connections, and report active connections; `check` waits for zero active
connections; `leave` restores the route idempotently. Payment callbacks must be
handled by the separate `PAYMENT_GATE` while the listener is down. A notice
alone, an active counter sampled at zero, or a quiet time of day is NOT an
admission gate. Without both gates, this first rollout remains NO-GO. Do not
implement a second gateway sharing production DB merely to work around this
condition.

Subsequent runtimes optionally support token-authenticated POST
`/internal/drain?action=begin|status|cancel` (`DRAIN_TOKEN` must be provisioned
separately with authorization). Admission and active counting share a mutex;
the response contains `active_requests`. `begin` sets a ten-minute lease;
`status` does not renew it; `cancel` or lease expiry restores admission.
SePay callbacks remain admitted and counted while draining. Keep an external
payment delivery gate during the actual listener outage. No additional key
has been provisioned by this patch.

## Replace and recovery

The script locks concurrent deploys, validates Compose project/service labels,
records old/candidate image IDs and dependency identities in `.deploy-state`,
and retains a uniquely tagged old image. It uses `docker stop --timeout=-1`
before `compose up --no-deps --pull never --force-recreate llm-gateway`.
This prevents overlapping workers and avoids Docker SIGKILL. A hung task can
therefore stall maintenance indefinitely: investigate, do not automatically kill
billing or start another instance. Compose's 210s grace is only a fallback for
other commands; use this script's workflow.

After replacement, image identity, dependency identity, Docker health and
`/readyz` must pass. Then the required business smoke runs. Failure attempts
rollback by the OLD IMAGE ID, waits for readiness and reruns smoke. Failed
rollback keeps maintenance closed and retains the state file. An unresolved
state file blocks another rollout. SIGINT/TERM before replacement releases the
gate; after replacement it attempts rollback. SIGKILL/host loss cannot run a
trap: recover manually from the state record, preserving the maintenance gate.

On success the state record is renamed `.deploy-state.completed-*`. Archive
the selected image ID with the release. Any future manual Compose invocation
must also set `LLM_GATEWAY_IMAGE` to the selected ID; its fallback `latest` is
not a persistent release pointer and may otherwise select a different image.

PostgreSQL (`new-api-postgres`) and Redis (`new-api-redis`) are protected by
container-ID checks and are never recreated by the wrapper.

`scripts/smoke-relay.py` is an explicit, potentially billable test:
set `SMOKE_BASE_URL`, `SMOKE_KEY_FILE`, `SMOKE_MODEL`, `SMOKE_EXPECTED_QUOTA`.
Use an otherwise idle dedicated test key, pinned to a mock channel, with known
quota delta. It checks unauthorized HTTP status, authenticated SSE content,
unique terminal DONE and raw token quota conservation. It does not prove wallet
settlement/worker health. `BUSINESS_SMOKE` must additionally verify those via
the approved test account/logs and worker/lease state. Do not call real paid
providers or mutate customer balances as an implicit smoke test.

Shutdown waits for admitted handlers (including upgraded connections), joined
SePay/Binance workers, claimed system tasks and tracked async refund/cache
writes; then the batch updater is stopped and pending quota updates flushed.
DB failures still retain existing error handling; crash-safe financial recovery
across SIGKILL/DB failure is NOT guaranteed by an in-memory wait group.

## Isolated rehearsal

Build the fixture with Go 1.26: `go test -race -c -o
.local-tests/rollout-fixture.test .`, then run `python3 scripts/test-deploy.py`.
The fixture uses production admission/BillingSession/shutdown helper, separate
PostgreSQL 15 and Redis 7, a dedicated Compose project (`llm-gateway-rehearsal`)
and mock workers/webhooks; its fixture images are removed after the run.
It is NOT the full production entrypoint, provider API or real payment poller.
The rehearsal checks active-SSE SIGTERM, deploy, rollback, exact quota,
webhook delivery and failure-after-gate recovery. Only its own containers and
disposable database are removed. Production volumes/networks are not attached.

Useful test overrides: `COMPOSE_FILE`, `COMPOSE_PROJECT_NAME`, `CONTAINER`,
`POSTGRES_CONTAINER`, `REDIS_CONTAINER`, `DEPLOY_LOCK`, `DEPLOY_STATE`,
`HEALTH_TIMEOUT`, `BASE_URL`, `ROLLBACK_TAG` (default `llm-gateway:rollback`).
Never put credentials in command output.
