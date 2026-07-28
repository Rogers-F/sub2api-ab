# OpenAI Reasoning Effort Synchronization Design

**Date:** 2026-07-28
**Status:** Approved for planning
**Scope:** OpenAI platform reasoning-effort handling only

## Context

The current fork contains GPT-5.6 model and pricing support, and the frontend can
render `Max`, but the backend usage metadata normalizer only recognizes efforts
through `xhigh`. A request carrying `reasoning.effort = "max"` can therefore be
forwarded correctly while its usage record stores no effort and the UI displays
`-`.

The fork diverged from upstream Sub2API at `v0.1.115` and has substantial local
changes. Upstream later added several related fixes and a group-level OpenAI
reasoning policy. Directly merging the upstream commit stack would mix unrelated
OpenAI refactors into this fork, so this design ports the upstream semantics into
the current architecture.

Primary upstream references:

- PR `#3909` / commit `80b3d4c1`: GPT-5.6 `max` support and compact handling.
- Commit `c3ae5fc3`: model-candidate fallback for usage metadata extraction.
- Commit `b9b013a0`: mapped-model candidate handling for WebSocket passthrough.
- Commit `6af622c3`: OpenAI group reasoning ceilings and exact mappings.
- Commit `6c93f01c`: policy validation hardening.
- Commit `a3622274`: remove incompatible reasoning fields during cross-mode
  account failover.

## Goals

1. Preserve and record `max` for GPT-5.6 requests across all OpenAI forwarding
   modes.
2. Keep non-GPT-5.6 compatibility behavior by representing incoming `max` as
   `xhigh` in normalized usage metadata.
3. Derive effort metadata from the original model suffix when an upstream or
   billing model has already had its suffix removed.
4. Normalize OpenAI OAuth compact requests from GPT-5.6 `max` to `xhigh` while
   leaving ordinary Responses and API-key upstream requests unchanged.
5. Add per-group OpenAI reasoning ceilings and exact mappings.
6. Apply the same group policy to HTTP Responses, Chat Completions, and every
   WebSocket turn.
7. Persist, expose, edit, cache, and invalidate group policy fields consistently.
8. Keep the change isolated to groups whose platform is `openai`.

## Excluded Work

- General synchronization of all upstream OpenAI gateway changes.
- OpenAI Live, image-generation, billing, quota, scheduler, and unrelated
  compact protocol changes.
- Reasoning-content conversion changes for DeepSeek, Grok, Gemini, or
  Anthropic.
- Treating `ultra` as an upstream API effort. Codex uses `ultra` as a client
  orchestration mode and sends `max` to the OpenAI request path.
- Automatically injecting a reasoning effort when the client omitted one.

## Supported Values

The persisted group-policy order is:

1. `minimal`
2. `low`
3. `medium`
4. `high`
5. `xhigh`
6. `max`

Aliases such as `x-high`, `x_high`, and `extra-high` normalize to `xhigh`.
`none`, `ultra`, empty mapping endpoints, unknown values, and duplicate mapping
sources are rejected by the group-policy API.

An empty maximum means that the group imposes no ceiling. An empty mappings
array means that the group performs no explicit rewrite.

## Data Model

Add two fields to `groups`:

```sql
max_reasoning_effort VARCHAR(20) NOT NULL DEFAULT ''
reasoning_effort_mappings JSONB NOT NULL DEFAULT '[]'::jsonb
```

The current fork's latest migration is `131`, so the new migration is numbered
`132_add_group_reasoning_effort_policy.sql`. It uses `ADD COLUMN IF NOT EXISTS`
to support mixed deployment histories.

The Ent group schema gains:

- `max_reasoning_effort`: optional/default-empty string with a 20-character
  limit.
- `reasoning_effort_mappings`: JSON array of `{from, to}` objects, defaulting
  to an empty array.

A shared domain type represents one exact mapping:

```go
type ReasoningEffortMapping struct {
    From string `json:"from"`
    To   string `json:"to"`
}
```

Generated Ent code is regenerated from the schema instead of manually edited.

## Policy Semantics

Policy application is deterministic:

1. Inspect both supported request shapes: `reasoning.effort` and
   `reasoning_effort`.
2. Ignore omitted, empty, and non-string fields.
3. Canonicalize the explicit client value for matching.
4. Apply at most one exact mapping based on the original canonical value.
5. Do not chain mappings.
6. If the mapped value is recognized and ranks above the configured ceiling,
   replace it with the ceiling.
7. Preserve unknown future effort strings unless an exact mapping targets
   them; current validation only allows known values as mapping sources and
   destinations.
8. Never raise a lower effort to the ceiling.
9. Never create an effort field when the client omitted it.

Examples:

| Request | Mapping | Ceiling | Effective value |
|---|---|---|---|
| `max` | none | `xhigh` | `xhigh` |
| `max` | `max -> high` | `xhigh` | `high` |
| `max` | `max -> xhigh`, `xhigh -> low` | none | `xhigh` |
| `low` | none | `high` | `low` |
| omitted | none | `low` | omitted |

## Model-Aware Usage Normalization

Usage metadata extraction accepts an ordered list of model candidates:

1. final upstream model;
2. billing or mapped model;
3. original client-requested model.

For an explicit body effort, the first non-empty model candidate determines
whether `max` remains distinct. For requests without an explicit effort, each
candidate is inspected in order for a suffix such as `-high`, `-xhigh`, or
`-max`.

Normalization rules:

- GPT-5.6 Sol, Terra, Luna, their dated forms, and provider-prefixed forms keep
  explicit or suffix-derived `max`.
- Other recognized OpenAI-compatible models normalize `max` to `xhigh` for
  usage metadata compatibility.
- `none` and `minimal` continue to produce no stored usage effort where the
  existing usage model treats them as absent.
- `low`, `medium`, `high`, and `xhigh` remain unchanged.

This metadata logic does not independently mutate the forwarded body.

## Request Flow

### HTTP Responses

After authentication and body reading, apply the authenticated group's policy
before model extraction, channel mapping, account selection, and forwarding.
The modified body becomes the single request body used by downstream retries so
all attempts observe the same group policy.

### HTTP Chat Completions

Apply the same policy to the incoming Chat Completions body before choosing the
raw Chat Completions path or converting to Responses. Both nested and flat
effort forms remain supported.

### WebSocket Responses

Store the authenticated group's normalized policy in the WebSocket forwarding
session. Apply it independently to every incoming turn before turn-level model
mapping and forwarding. This covers clients that change effort between turns.

Usage metadata for each turn receives the mapped model and original model as
ordered candidates so suffix-derived effort survives model normalization.

### OAuth Compact

For OpenAI OAuth accounts only, requests sent to the ChatGPT compact endpoint
with a GPT-5.6 model and `reasoning.effort = "max"` are rewritten to `xhigh`.
Ordinary Responses requests, API-key accounts, remote-compaction-v2 turns, and
other platforms preserve `max`.

The usage result records the effective compact value (`xhigh`).

### Cross-Mode Failover

When a retry crosses into a forwarding mode that does not support the request's
reasoning shape, remove the foreign reasoning field before the next attempt.
The original immutable body remains available so same-mode retries preserve the
client's intended effort and group policy.

## Group Administration

Create and update APIs accept:

```json
{
  "max_reasoning_effort": "xhigh",
  "reasoning_effort_mappings": [
    {"from": "max", "to": "xhigh"}
  ]
}
```

Validation behavior:

- Policy fields are accepted only when `platform` is `openai`.
- Create and update canonicalize values before persistence.
- Changing a group away from OpenAI clears both policy fields.
- At most 64 mappings are accepted.
- Mapping values are bounded to 64 characters before canonical validation.
- Duplicate sources are rejected case-insensitively after normalization.
- API responses return canonical values.

Group duplication copies the normalized policy. Repository mapping, shallow and
admin DTOs, and all group read paths carry both fields.

## Authentication Cache

`APIKeyAuthGroupSnapshot` includes both policy fields. Snapshot creation and
rehydration deep-copy the mappings slice so cache entries cannot share mutable
mapping data.

Create, update, platform change, and group duplication invalidate the same
group-scoped API-key authentication cache used by existing group routing
fields. Cache version tests verify old snapshots are refreshed rather than
silently dropping the new fields.

## Management UI

The group create/edit form displays a Reasoning Effort Policy section only for
OpenAI groups.

Controls:

- Maximum effort selector with `Unlimited`, `Minimal`, `Low`, `Medium`, `High`,
  `XHigh`, and `Max`.
- Mapping rows with source and destination selectors.
- Add/remove mapping actions.
- Inline validation for incomplete rows and duplicate sources.

Switching the form platform away from OpenAI clears the form policy state.
Opening an existing OpenAI group hydrates canonical mappings, and submit payloads
contain independent arrays to avoid mutating table-row data.

The current fork uses monolithic `en.ts` and `zh.ts` locale files, so upstream
split-locale changes are adapted to the local i18n layout.

## Error Handling

- Invalid policy API input returns the existing admin bad-request response
  shape with a specific validation message.
- Malformed request JSON continues through the existing gateway parse error
  path; policy application never masks it.
- A low-level JSON rewrite failure leaves the original field unchanged and
  allows existing validation/forwarding behavior to decide the request.
- Database migration defaults keep existing groups behaviorally unchanged.
- Empty or stale policy data loaded from storage is sanitized to the neutral
  empty policy before use.

## Test Strategy

### Backend unit tests

- Canonical value normalization and alias handling.
- Maximum ordering, exact mapping, non-chaining, duplicate rejection, and
  OpenAI-only validation.
- Both nested and flat request shapes.
- Omitted and future values.
- GPT-5.6 `max` preservation and non-GPT-5.6 `max -> xhigh` metadata behavior.
- Original-model suffix fallback after OAuth/model normalization.
- Compact downgrade matrix by account type and endpoint.

### Backend integration-path tests

- Responses HTTP applies group mapping and ceiling before forwarding.
- Chat Completions raw and converted paths record the effective effort.
- WebSocket applies policy per turn and records mapped/original model metadata.
- Group create, update, duplicate, platform change, repository mapping, and DTO
  output preserve canonical policy.
- Authentication cache snapshot round-trip and invalidation include the policy.
- Migration succeeds on both empty and already-columned databases.

### Frontend tests

- Controls appear only for OpenAI groups.
- Create/edit hydration and payload serialization.
- Add/remove mapping rows.
- Duplicate/incomplete mapping validation.
- Platform change clears policy state.
- Existing group settings remain unaffected.

### Verification commands

Focused tests run first during each red-green cycle. Final verification runs:

```bash
cd backend && go test ./internal/service ./internal/handler/admin ./internal/handler ./internal/repository
cd frontend && pnpm exec vitest run src/views/admin/__tests__/groupsReasoningEffort.spec.ts
cd frontend && pnpm run type-check
```

The complete backend unit suite is run with the repository's documented unit
tag after focused tests pass:

```bash
cd backend && go test -tags=unit ./...
```

## Rollout and Compatibility

The migration is additive and defaults to neutral behavior, so existing OpenAI
groups preserve current forwarding behavior until an administrator configures a
policy. Deployments apply the database migration before serving binaries that
write the new fields.

Usage records created before this change remain unchanged. New GPT-5.6 `max`
requests begin storing `max`, so historical `-` rows are not retroactively
rewritten.

## Acceptance Criteria

1. A normal GPT-5.6 Sol request configured with Codex `max` reaches the upstream
   with `max` and produces a usage row displaying `Max`.
2. A GPT-5.6 OAuth compact request reaches the compact endpoint with `xhigh` and
   records `XHigh`.
3. Model-suffix efforts remain visible after model mapping strips the suffix.
4. A configured group mapping and ceiling produce the same effective value in
   HTTP and WebSocket paths.
5. Groups from every other platform expose and apply a neutral empty policy.
6. Updating a group policy takes effect after authentication cache invalidation.
7. Existing OpenAI requests with no explicit effort retain upstream defaults.
8. Focused backend/frontend tests, type checking, and the full backend unit suite
   pass with no new failures.
