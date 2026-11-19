# API-only release (not authorized for production rollout)

Export with `sh scripts/build-api-candidate.sh NEW_DIRECTORY`. The exporter
starts at the recorded HEAD and overlays the allowlist in that script: seven
runtime files (`relay/channel/api_request.go`, `controller/relay.go`,
`logger/logger.go`, `relay/channel/openai/helper.go`,
`relay/channel/claude/relay-claude.go`, `relay/helper/common.go`,
`relay/helper/stream_scanner.go`) plus their regression tests. The main changes:

- `relay/channel/api_request.go`: remove the heartbeat before upstream headers;
  keep the existing stream-scanner heartbeat after successful headers.
- `controller/relay.go`: prohibit replay after response chunks or committed SSE.
- `logger/logger.go`: atomic counters for concurrent logging/rotation scheduling.

API regression tests are included. Billing/payment/quota, shutdown/drain, Edge
Gate, dirty affinity settings, and deployment-script changes are not overlaid.
Existing HEAD behavior is retained, including existing billing and shutdown
limitations. This is not a claim that those limitations have been fixed.

Review `runtime.patch`, `overlay-files`, `base-commit`, and `source.sha256` before
testing/building the exported context. Build using its unchanged Dockerfile,
record both the build result and Docker's resolved immutable image ID. Keep the
source snapshot, logs, frontend output and old production image for rollback.
Do not build from the dirty source directory. The exporter never deploys.

## Single-instance operating procedure, pending approval

Do not use the existing gate-dependent `scripts/deploy.sh`, the Edge Gate
scripts, or fake no-op gates for this release. No new proxy is required.

1. Obtain explicit maintenance approval and a bounded request-loss budget.
   Verify deployed identity, Compose project/service, immutable rollback image,
   PG/Redis identities, rendered configuration and environment compatibility.
   Do not silently apply the unrelated dirty Compose/drain changes.
2. Stop new test/customer jobs and new payment initiation operationally; let
   active SSE/WS complete. Notification and sampled zero requests are not an
   atomic admission barrier. If quiescence cannot be established, stop rollout.
3. Establish an independent, complete payment transaction record covering the
   whole deployment AND potential rollback window, with verified supported
   recovery for missed callbacks and expired orders. Do not assume provider
   retry. Existing QR codes/transfers remain a risk even after checkout stops.
4. Stop the old gateway only after financial work is known quiescent. Wait for
   confirmed process exit before starting its replacement; never overlap payment
   workers or force-kill a hung financial operation to meet the window.
5. With an approved reviewed Compose configuration pinned to the candidate ID,
   recreate only `llm-gateway` using `--no-deps --pull never`. Do not build here,
   change routes, or restart PostgreSQL/Redis.
6. Verify image identity, readiness, authenticated test-key SSE/terminal events,
   HTTP errors, safe retry, logs, test-account usage and worker recovery. Any
   billable smoke needs separate approval. Compare PG/Redis container IDs.
7. On failure, wait for candidate exit and recreate only the gateway pinned to
   the captured old ID; repeat readiness and business checks. Do not reverse
   quota or database transactions. Reconcile payment IDs through the existing
   authorized workflow before ending maintenance.

Downtime includes stop/start/readiness/smoke and possibly rollback; no measured
production bound is available. New requests/webhooks may fail while offline;
streams exceeding the existing shutdown deadline may be interrupted. HTTP
shutdown does not join the existing payment workers or asynchronous refunds.

## Stop conditions / current NO-GO

Cannot promise uninterrupted payments in a shared process. Merchant webhook
retry/replay and recovery after order expiry have not been independently proven
here. SePay reconciliation requires verifiable transaction evidence; bank
webhooks have no independent durable receiver in this release. Binance history
checkpoints alone do not prove API health or settlement of expired orders.
Missing payment recovery, unknown active financial work, unresolved SSE/WS,
missing rollback image, config drift or failed verification means NO-GO.

The backend heartbeat fix cannot remove an edge/provider timeout while upstream
headers are absent. Local mock tests are not Cloudflare/Ckey validation.

## Verification on 2026-11-06

Snapshot: `/tmp/newapi-api-release-20261106`, base commit
`3a2e13eb6f34f43d39a719dd8e5e20bd44c31556`.
Built immutable image:
`sha256:efda7bc122851eb88b9f5b82afb5dc7a1e40bcf10a3f0520d3c86b0335df0920`.

Go 1.26.10 tests run with Docker networking disabled and local mock/SQLite:

- Full `go test ./...`: PASS after generating the frontend embed via Docker's
  unchanged frontend build target. Initial attempt lacked `web/dist`.
- Full `go test -race ./...`: FAIL in
  `TestRetiredSubscriptionsUseWalletFunding`. Cleanup writes `RedisEnabled`
  while an async refund's cache operation still reads it. Both paths are HEAD
  code, not API overlays. Removing the billing overlay re-exposes this race;
  it does not prove production double charging. No billing fix was applied.
- Focused controller/logger/relay race tests: PASS, including existing SSE,
  watchdog, HTTP error, fallback, cancellation, truncation and 300-stream tests.
- Separate relaykit module unit/race commands: PASS (packages have no tests).

Logs: `verification.log`, `race-initial.log`, `api-race.log`, `build.log`.
The delayed-header watchdog test cancels the request early; its 126-second
mock delay must not be reported as successful streaming across Cloudflare's
timeout boundary. No new five-minute soak or live provider validation ran.
Current decision remains NO-GO: full race failure plus unproven shared-process
payment shutdown/recovery. Do not deploy this image without resolving or
explicitly reviewing those blockers in a separately authorized scope.
