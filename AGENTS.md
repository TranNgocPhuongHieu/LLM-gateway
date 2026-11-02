# NewAPI Context Map

## 1. Guardrails, Codex Auto-Router and context CLI

- Code work is separate from infrastructure: do not read `operate-infrastructure/SKILL.md`, run `docker exec`, or query PostgreSQL/DB unless live infrastructure debugging is explicitly requested.
- This project is Codex-only and project-scoped. Do not read the global infrastructure skill or re-read the local channel skill during ordinary code work; use this `AGENTS.md` and `scripts/dev-context.sh`.
- For a short natural-language prompt, run the first matching context command: channel/protocol → `channel-types` or `relay <provider>`; channel models/mapping → `controller-map` then `web channels`; connection/key/base URL/header override → `web channels`; pricing/ratio/quota/billing → `pricing` then `web pricing`; UI/i18n → `web <feature>` then `i18n <prefix>`.
- If the worktree has edits, run `wip` once before patching; do not repeatedly inspect `git show HEAD`.
- Preserve dirty worktree changes. Treat API routes, persisted JSON keys, DB fields, relay formats, and `web/src/i18n/locales/vi.json` as contracts.
- Context-first: run the matching command before opening a large file; prefer the output's line numbers over repeated dumps.
- `relaykit` is a separate Go module; its standard check is `(cd relaykit && go test ./...)`.

| Command | Use |
|---|---|
| `overview` | Backend package files and top-level frontend features |
| `channel-types` | Channel/API/endpoint constants, model metadata, channel UI gates |
| `relay <provider>` | Provider adaptor plus relayconvert symbols |
| `controller-map` | Channel/relay function ranges and drawer blocks |
| `pricing` | Backend pricing/quota/billing and frontend pricing exports |
| `domain <name>` | `auth-routing`, `models`, `users-groups`, or `logs-settings` |
| `web [feature]` | Shared frontend directories or one feature's files/exports |
| `wip` | Dirty-worktree status, diff stat, and hunk headers |
| `i18n <prefix>` | Matching Vietnamese locale keys with line numbers |

## 2. Channel, relay and protocol map

### 2.1 Channel type, protocol and persistence

| Path | Core symbols | Role |
|---|---|---|
| `constant/channel.go`, `constant/api_type.go` | `ChannelType*`, `ChannelTypeNames`, `ChannelBaseURLs`, `APIType*` | Provider identity, names, built-in bases, adaptor identity. |
| `common/api_type.go` | `ChannelType2APIType`, `SupportsResponsesCompact` | Channel-to-adaptor mapping and compact Responses capability. |
| `constant/endpoint_type.go`, `common/endpoint_defaults.go` | `EndpointType*`, `EndpointInfo`, `GetDefaultEndpointInfo` | Shared endpoint identities and default upstream paths. |
| `model/channel.go` | `Channel`, `ChannelInfo`, `ValidateSettings`, `Get/SetSetting`, `Get/SetOtherSettings` | Channel persistence and JSON settings validation. |
| `common/channel_spec.go` | `ChannelSpec`, `GetChannelSpec` | Central static channel/API/name/base URL contract without import cycles. |
| `relaykit/dto/channel_settings.go` | `ChannelSettings`, `ChannelOtherSettings`, `ForceFormat`, `PassThroughBodyEnabled` | Current settings DTOs; `model/user_settings.go` is absent. |
| `model/model_meta.go`, `model/model_extra.go`, `service/channel.go` | `Model`, `ModelConnection`, `GetModelSupportEndpointTypes`, channel helpers | Model metadata and endpoint capability consumers; requested `model/model.go` is absent. |
| `controller/channel.go` | `GetAllChannels`, `GetChannel`, `AddChannel`, `UpdateChannel`, `DeleteChannel`, `DeleteChannelBatch` | Main channel CRUD handlers. |
| `controller/channel_validate.go` | `validateChannel` | Shared add/update/settings validation. |
| `controller/channel_models.go` | `FetchModels`, `FetchUpstreamModels`, `GetTagModels`, model response DTOs | Model preview, upstream model retrieval, and channel model views. |
| `controller/channel_ops.go` | `ManageMultiKeys`, tag/status/batch helpers, `CopyChannel` | Non-CRUD channel operations and auxiliary management handlers. |

### 2.2 Relay, adaptor and conversion

| Path | Core symbols | Role |
|---|---|---|
| `router/relay-router.go` | `SetRelayRouter` | `/v1/messages`, chat/completions, responses, images, audio, embeddings, rerank, Gemini. |
| `controller/relay.go` | `Relay`, `relayHandler`, `getChannel`, `shouldRetry`, `writeRelayErrorResponse` | Validate, create `RelayInfo`, select/retry channel, bill, return errors. |
| `relay/relay_adaptor.go` | `GetAdaptor` | Maps API type to Claude/OpenAI adaptor. |
| `relay/common/relay_info.go`, `relay/common/relay_utils.go` | `RelayInfo`, `GenRelayInfo`, conversion-chain methods, URL/log helpers | Request metadata, conversion state, safe URL and API-version helpers. |
| `relay/channel/claude/`, `relay/channel/openai/` | `Adaptor`, `Convert*`, `GetRequestURL`, `SetupRequestHeader`, `DoRequest`, `DoResponse`, stream handlers | Provider request/response adaptors; DTOs live in `relaykit/dto/`. |
| `relaykit/relayconvert/request_compat.go`, `response_compat.go` | `ConvertRequest*`, `ConvertResponse*`, stream states | Public OpenAI/Claude/Gemini/Responses conversion facade. |
| `relaykit/relayconvert/request_registry.go`, `response_registry.go` | `ConvertRequest`, `ConvertRequestVia`, `ConvertResponse`, `ConvertStreamResponse` | Registered route selection and execution. |

### 2.3 Frontend channel form

| Path | Core symbols | Role |
|---|---|---|
| `web/src/features/channels/constants.ts` | `CHANNEL_TYPE_OPTIONS`, `MODEL_FETCHABLE_TYPES`, `*_FIELD_PASSTHROUGH_TYPES` | Provider choices and conditional fields. |
| `web/src/features/channels/types.ts` | `Channel`, `ChannelSettings`, `ChannelOtherSettings`, `AddChannelRequest` | API, settings, and form contracts. |
| `web/src/features/channels/lib/channel-form.ts` | `channelFormSchema`, defaults, `transform*`, `buildSettingJSON` | Validation, defaults, setting split, payload transforms. |
| `web/src/features/channels/lib/channel-utils.ts` | `parseChannelSettings`, `parseChannelOtherSettings`, display/validation helpers | Shared form/table utilities. |
| `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | `ChannelMutateDrawer`, `currentType`, `onSubmit`, `*Section` | Add/edit UI shell, form state, submit, and layout; current path is under `components/drawers/`. |
| `web/src/features/channels/components/drawers/channel-mutate-protocol-sections.tsx` | `Channel*Field` components | Typed protocol, passthrough, prompt, format, and HTTP fields; form state is passed explicitly. |
| `web/src/features/channels/components/drawers/channel-mutate-model-sections.tsx` | `buildChannelModelSections` | Typed model list, mapping, upstream detection, and discovery sections. |
| `web/src/features/channels/components/drawers/channel-mutate-connection-sections.tsx` | `buildChannelConnectionSections` | Typed channel name/key/base URL/auth/override and connection sections. |
| `web/src/features/channels/components/dialogs/param-override-editor-dialog.tsx` | `ParamOverrideEditorDialog` | Dialog shell for parameter override editing. |
| `web/src/features/channels/components/dialogs/param-override-types-utils.ts` | Typed presets, types, and pure helpers | Extracted parameter override editor utilities. |
| `web/src/features/channels/components/dialogs/param-override-subcomponents.tsx` | Typed panel boundary | Extension point for parameter override subcomponents. |
| `web/src/features/xpiki-quota/index.tsx` | `XPikiQuotaPage`, `XPikiRequestLogsPage` | XPiki quota feature entrypoint. |
| `web/src/features/xpiki-quota/xpiki-quota-utils.ts` | XPiki formatters and table helpers | Shared quota utility functions. |
| `web/src/features/xpiki-quota/xpiki-quota-components.tsx` | Typed quota component boundary | Extension point for quota tables/dialogs. |
| `web/src/i18n/locales/vi.json` | Channel/protocol/base URL/key/validation keys | Vietnamese UI contract. |

## 3. Project-wide domain map

### 3.1 Routing, auth and distribution

| Path | Core symbols | Role |
|---|---|---|
| `router/*.go` | `Set*Router` | Main, API, web, authz, channel, relay, dashboard, pricing routes. |
| `middleware/auth.go`, `distributor.go`, `rate-limit.go`, `model-rate-limit.go`, `turnstile-check.go` | auth/distribution/rate-limit middleware | Token auth, channel selection context, abuse and model limits. |
| `model/ability.go`, `service/channel_select.go`, `service/channel.go` | ability/channel selection helpers | Group, weight, priority, retry and channel eligibility. |

### 3.2 Pricing, quota, billing and comparison

| Path | Core symbols | Role |
|---|---|---|
| `setting/ratio_setting/*.go` | model/completion/cache/group ratio getters | Configured model and group ratios. |
| `setting/billing_setting/*.go`, `model/pricing*.go` | billing modes/expressions, pricing refresh | Ratio and tiered-expression policy. |
| `service/quota.go`, `text_quota.go`, `billing*.go`, `tiered_settle.go`, `violation_fee.go` | quota/billing settlement | Pre-consume, post-consume, fees, and usage. |
| `relay/helper/price.go`, `relay/common/billing.go`, `controller/pricing*.go`, `controller/billing.go` | price/billing API helpers | Request pricing and admin pricing endpoints. |

### 3.3 Models catalog and sync

| Path | Core symbols | Role |
|---|---|---|
| `model/model_meta.go`, `model/vendor_meta.go`, `model/model_metadata_sync.go` | `Model`, `Vendor`, search/sync functions | Catalog metadata, vendors, channel associations, sync state. |
| `controller/model.go`, `model_meta.go`, `model_sync.go`, `vendor_meta.go`, `missing_models.go` | list/sync/vendor handlers | Catalog APIs and upstream model reconciliation. |
| `service/channel.go`, `service/request_converter.go`, `common/endpoint*.go` | model/channel endpoint helpers | Endpoint capability and upstream model support. |

### 3.4 Tokens, users, groups and topup

| Path | Core symbols | Role |
|---|---|---|
| `model/token.go`, `controller/token.go`, `service/auth_token.go` | token CRUD/auth helpers | API token lifecycle and authentication. |
| `model/user.go`, `controller/user.go`, `service/auth_session.go` | user/session functions | User identity, sessions, roles and access. |
| `model/group.go`, `controller/group.go`, `service/group.go` | group functions | Group membership and quota policy. |
| `model/redemption.go`, `model/topup.go`, `controller/redemption*.go`, `controller/topup*.go` | redemption/topup/payment handlers | Credits, wallet funding, payment callbacks. |
| `model/user_oauth_binding.go`, `controller/oauth.go` (if present), `controller/secure_verification.go`, `model/passkey.go` | OAuth/verification/passkey | Login and sensitive-operation verification; use actual files from `overview`. |

### 3.5 Logs, analytics and system settings

| Path | Core symbols | Role |
|---|---|---|
| `model/log.go`, `model/log_other.go`, `controller/log.go`, `service/log_info_generate.go` | log models and handlers | Usage/error logs and safe log projections. |
| `model/usedata*.go`, `controller/usedata.go`, `controller/performance.go`, `service/rankings.go` | usage/statistics functions | Dashboard, usage, ranking and performance data. |
| `controller/option.go`, `setting/operation_setting/*.go`, `setting/system_setting/*.go` | option/config settings | Runtime, quota, monitor, legal and system settings. |

### 3.6 Frontend feature map

| Feature/path | Backend touchpoint |
|---|---|
| `auth`, `security`, `setup`, `keys`, `profile`, `users` | auth/session, user/token, verification, setup APIs |
| `channels`, `models`, `model-pricing`, `pricing` | channel/model metadata, pricing and ratio APIs |
| `wallet`, `xpiki-quota`, `usage-logs`, `request-logs` routes | billing, quota, logs and usage APIs |
| `dashboard`, `performance-metrics`, `system-info` | statistics, health and system info APIs |
| `system-settings`, `redemption-codes` routes | options, billing, operation and redemption APIs |
| `home`, `docs`, `errors` | public UI, documentation and error surfaces |
| `routes/`, `components/`, `lib/`, `stores/`, `hooks/`, `i18n/locales/vi.json` | Shared routing, UI, state, helpers and localization |

## 4. Standard operating procedures

### SOP 1 — Channel type or protocol
- Run `channel-types`; verify actual constants, endpoint defaults and adaptor mapping.
- Trace router → `Relay` → `RelayInfo` → adaptor → converter; update only owning layers.
- Synchronize DTO, frontend gates/transforms, and `vi.json`; add request/response/stream tests.
- Run targeted Go/web checks and inspect the diff.

### SOP 2 — Relay conversion or channel setting
- Run `relay <provider>` and identify request vs response registry routes.
- Preserve JSON keys/defaults in `relaykit/dto` and `model/channel.go` validation.
- Round-trip frontend parsing/building and test omitted, invalid, stream, and failure paths.
- Run `go build ./...` or the narrowest equivalent plus web typecheck/build.

### SOP 3 — Pricing, quota, ratio or model comparison
- Run `pricing`; separate ratio, tiered expression, quota, provider cost, and wallet deduction.
- Trace setting → service settlement → relay billing → controller/UI API.
- Preserve thresholds, units, snapshots, and no-retroactive behavior; add regression tests.
- Verify frontend draft/save and backend calculation independently.

### SOP 4 — Routing, auth, catalog, identity or settings
- Run `domain <name>`; confirm route registration, middleware order, model/user ownership, and settings scope.
- Trace callers, permissions, persistence fields, cache/side effects, and external consumers.
- Test unauthorized, invalid, empty, and failure paths without weakening checks.
- Keep migrations/deploy/DB operations out of code work unless explicitly authorized.

### SOP 5 — Frontend/backend synchronization
- Run `web [feature]`; compare API payload, persisted container (`setting` vs `settings`), and locale keys.
- Update types/schema, defaults, transforms, UI gates, loading/error states, and `vi.json` together.
- Preserve edit-time key secrecy and unrelated settings; test create/update round trips.
- Run frontend typecheck/build and review the final diff.
