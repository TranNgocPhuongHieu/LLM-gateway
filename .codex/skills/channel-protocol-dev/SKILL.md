---
name: channel-protocol-dev
description: Develop NewAPI channels, protocols, relay adaptors, relaykit conversions, pricing/model touchpoints, or web features tied to backend contracts; not infrastructure-only work.
metadata:
  short-description: NewAPI code and context routing
---

# NewAPI code development

Use for channel/protocol, relay/`relaykit`, pricing/quota/model, auth/routing,
or `web/src/features` contract changes. Do not use for infrastructure-only
work.

## Required first step

From `NewAPI/source`, read `AGENTS.md`, then run the relevant context command
before opening large files:

```bash
bash scripts/dev-context.sh channel-types       # channel/protocol
bash scripts/dev-context.sh relay claude        # provider adaptor
bash scripts/dev-context.sh controller-map      # channel/relay UI blocks
bash scripts/dev-context.sh pricing             # pricing/quota/model
bash scripts/dev-context.sh domain auth-routing  # backend domain
bash scripts/dev-context.sh web channels         # frontend feature
bash scripts/dev-context.sh overview             # broad orientation
```

Use only provider, feature, and domain names that exist in the checkout.

## Safety and verification

- Keep code and infrastructure separate: no Docker administration, PostgreSQL
  queries, or `operate-infrastructure` skill reads for programming tasks.
- Preserve dirty worktree changes and public contracts: routes, JSON keys, DB
  fields, relay formats, and `vi.json`.
- Trace callers, registries, DTO persistence, frontend transforms, and failure
  paths before editing; add a focused test for changed behavior.
- Run the narrowest applicable checks, typically `go build ./...` and the web
  typecheck/build script. Report unavailable or failing checks exactly.
- `relaykit` is a separate Go module; run its tests with `(cd relaykit && go test ./...)`.
