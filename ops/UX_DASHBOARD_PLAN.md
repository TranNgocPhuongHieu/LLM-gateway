# UX & Dashboard improvement plan

Status: implementation in worktree, NOT deployed. Deploy needs separate approval.
Ships together with the pending SePay manual-confirm change.

## Scope

| ID | Item | Owner stream |
|---|---|---|
| 1 | i18n: missing keys + hard-coded Vietnamese moved to `t()` | A (code) + main (locales) |
| 2 | `/user-agreement` renders configured content; `/about`, `/rankings` go to `/` (no more 403) | A |
| 3 | ⌘K command menu hides admin-only items for non-admins | A |
| 4 | Sidebar badge with "needs review" top-up count; sidebar label "Dashboard" → "Usage history" | B |
| 4a | "Needs review" (badge, tab, summary) = unsuccessful top-ups with USDT order in review/partial/matched/expired OR SePay payment `verification_status = review` (user decision 2026-11-19) | B |
| 5 | Top-up management: server-side search + pagination (no 10,000-row client load) | B |
| 6 | Hide Inviter/Invitee reward fields; remove duplicate `/usage-logs/audit` page (redirect to `/request-logs`) | A |
| 7 | Overview: balance ≈ VND, cost column in recent 6 rows, unit on quota warning threshold | D |
| 8 | API key form: Quota mode / Group / Auto group order / Cross-group retry moved under Advanced | D |
| D1 | Overview KPI row: balance (credit + ≈VND), spend today, spend this month (± vs same period last month), projected run-out date | D |
| D2 | Daily cost chart, stacked by model, toggle by API key | D |
| D3 | One shared time-range picker on Overview: Today / 7d / 30d / This month / Custom (≤31 days) | D |
| D4 | Low-balance banner with "Top up" button; threshold shown with unit | D |
| D5 | Spend breakdown table by model and by API key (backend endpoint) | C (API) + D (UI) |
| D6a | Balance rejections (`insufficient_user_quota`, `pre_consume_token_quota_failed`) are written to the error log at most once per user and code per minute (in-memory, per node) so D6 can show them (user decision 2026-11-19) | main |
| D6 | "Recent errors" card grouped by cause with a fix hint (backend endpoint) | C (API) + D (UI) |

Dropped: per-request cost explanation (old D7) — user decision 2026-11-19.
Deferred: cache-savings card (D8), monthly CSV export (D9), overview/dashboard full merge, revenue/margin report.

## API contracts (new, additive, UserAuth, own data only)

### GET /api/data/self/breakdown
Query: `start_timestamp`, `end_timestamp` (unix seconds, required, end ≥ start, span ≤ 2,678,400 s = 31 days),
`group_by` = `model` | `token` (required).
Source: `quota_data` (hourly aggregates, flushed every `DataExportInterval` minutes, so "today" can lag a few minutes).

```json
{ "success": true, "message": "",
  "data": { "total": { "quota": 0, "count": 0, "token_used": 0 },
            "items": [ { "key": "gpt-4o", "label": "gpt-4o", "quota": 0, "count": 0, "token_used": 0 } ] } }
```
- `items` sorted by `quota` desc. `group_by=token`: `key` = token id as string, `label` = token name of a token
  owned by the caller, or `""` when the token was deleted (UI shows "Key #id (deleted)"). Never returns other users' token names.
- Errors: `success:false` with message for invalid params / span.

### GET /api/log/self/error_summary
Query: `start_timestamp`, `end_timestamp` (required, span ≤ 31 days).
Scans the caller's error logs (type = error), excluding intermediate retry attempts (`request_final:false`),
newest first, capped at 5,000 rows (`truncated: true` when capped). Classification is server-side; raw content is never returned.

```json
{ "success": true, "message": "",
  "data": { "total": 0, "truncated": false,
            "categories": [ { "category": "insufficient_balance", "count": 0, "last_at": 0, "model_name": "..." } ] } }
```
Categories (first match wins, order matters): `insufficient_balance`, `key_quota_exhausted`, `rate_limited`,
`model_unavailable`, `content_blocked`, `auth_failed`, `invalid_request`, `server_error` (fallback).
`model_name` = model of the most recent error in that category. Sorted by count desc.

## Implementation streams (parallel, disjoint files)

- A — display fixes: routes/about, rankings, user-agreement, command-menu, quota-settings-section, usage-logs/audit route,
  xpiki-quota hard-coded strings, common-logs-columns, chart "Tổng:" default.
- B — admin top-up: topup-management page/table, hooks/use-sidebar-data.ts (badge + label).
- C — backend: model/usedata.go, controller/usedata.go, model/log.go (or new model/log_error_summary.go), controller/log.go, router/api-router.go + Go tests.
- D — customer overview: dashboard overview components (new KPI row, cost chart, breakdown, errors card, range picker,
  banner), recent-usage-panel, profile notification tab, keys mutate drawer.
- Main session — vi.json / en.json for all streams (single writer), review, port to release tree, full checks.

Rules: preserve dirty WIP; no commits; no locale edits by streams; no DB/docker; no deploy.

## Test plan

- Go: unit tests for both endpoints (valid, invalid params, span limit, other user's data isolation, deleted token label,
  classification table, intermediate-retry exclusion, truncation). `GOTOOLCHAIN=go1.26.10 go test ./model/ ./controller/ ./service/`, `go vet`.
- Web: vitest for each changed component (render, empty, error, role gating); `tsgo -b`; oxlint on changed files.
- Locale check: every new `t()` key exists in vi.json and en.json.
- Release tree: apply the same patch to the release source used for builds and rerun Go + web checks there.
- Not covered until deploy: real browser walkthrough, prod data volumes.

## Rollback

Frontend/API changes are additive; no schema change. Rollback = run `llm-gateway:rollback` image (existing recipe).
