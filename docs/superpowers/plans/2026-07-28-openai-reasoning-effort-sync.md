# OpenAI Reasoning Effort Synchronization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Synchronize upstream OpenAI reasoning-effort behavior so GPT-5.6 `max`, group-level mappings/ceilings, compact requests, HTTP/WebSocket forwarding, failover, persistence, cache, usage metadata, and the admin UI behave consistently.

**Architecture:** Add a small domain value object plus a pure policy module, persist the policy on OpenAI groups, and apply it at each OpenAI ingress before model routing. Keep request mutation separate from model-aware usage normalization, pass ordered model candidates to usage extraction, and deep-copy policy slices at repository/cache/UI boundaries.

**Tech Stack:** Go 1.26, Gin, Ent, PostgreSQL JSONB, gjson/sjson, testify, Vue 3, TypeScript, Vitest, vue-i18n.

---

## File map

- `backend/internal/domain/reasoning_effort.go`: shared `{from,to}` mapping value.
- `backend/internal/service/openai_reasoning_effort_policy.go`: canonicalization, validation, sanitization, mapping, ceiling, and request-body mutation.
- `backend/internal/service/openai_reasoning_effort_policy_test.go`: pure policy contract.
- `backend/migrations/132_add_group_reasoning_effort_policy.sql`: additive group columns.
- `backend/ent/schema/group.go` and generated `backend/ent/**`: persistence schema and generated accessors.
- `backend/internal/service/group.go`, `admin_service.go`, `admin_service_group_test.go`: service fields, create/update validation, and cache invalidation behavior. This fork has no group-duplication API to extend.
- `backend/internal/repository/group_repo.go`, `api_key_repo.go`: persistence writes and entity mapping.
- `backend/internal/handler/admin/group_handler.go`, `backend/internal/handler/dto/{types,mappers}.go`: admin input/output contract.
- `backend/internal/service/api_key_auth_cache.go`, `api_key_auth_cache_impl.go` and cache tests: versioned, deep-copied policy snapshot.
- `backend/internal/service/openai_gateway_service.go`: model-aware effort normalization and compact mutation.
- `backend/internal/handler/openai_gateway_handler.go`, `openai_chat_completions.go`: HTTP ingress policy.
- `backend/internal/service/openai_ws_forwarder.go`: per-turn policy and model-candidate metadata.
- `backend/internal/handler/openai_gateway_reasoning_failover.go`: failover-mode reasoning cleanup decision.
- `frontend/src/types/index.ts`: API and form types.
- `frontend/src/views/admin/groupsReasoningEffort.ts`: pure form normalization/validation/serialization helpers.
- `frontend/src/components/admin/group/ReasoningEffortPolicyFields.vue`: OpenAI-only controls.
- `frontend/src/views/admin/GroupsView.vue`: create/edit hydration and payload integration.
- `frontend/src/i18n/locales/{en,zh}.ts`: local monolithic locale strings.
- `frontend/src/views/admin/__tests__/groupsReasoningEffort.spec.ts`: pure helpers and mounted form behavior.

### Task 1: Pure reasoning-effort policy

**Files:**
- Create: `backend/internal/domain/reasoning_effort.go`
- Create: `backend/internal/service/openai_reasoning_effort_policy.go`
- Create: `backend/internal/service/openai_reasoning_effort_policy_test.go`
- Modify: `backend/internal/service/group.go`

- [ ] **Step 1: Write the failing policy tests**

Cover canonical aliases, the ordered values `minimal..max`, rejection of `none`/`ultra`, 64-entry and 64-character bounds, duplicate normalized sources, OpenAI-only validation, nested/flat request fields, mapping-before-cap, non-chaining, omitted fields, non-string fields, and unknown future values. The central table must include:

```go
func TestApplyOpenAIReasoningEffortPolicy(t *testing.T) {
    tests := []struct {
        name, body, max, path, want string
        mappings []ReasoningEffortMapping
        changed bool
    }{
        {name: "nested caps max", body: `{"reasoning":{"effort":"max"}}`, max: "xhigh", path: "reasoning.effort", want: "xhigh", changed: true},
        {name: "mapping runs before cap", body: `{"reasoning_effort":"MAX"}`, max: "xhigh", mappings: []ReasoningEffortMapping{{From: "max", To: "high"}}, path: "reasoning_effort", want: "high", changed: true},
        {name: "mapping does not chain", body: `{"reasoning_effort":"max"}`, mappings: []ReasoningEffortMapping{{From: "max", To: "xhigh"}, {From: "xhigh", To: "low"}}, path: "reasoning_effort", want: "xhigh", changed: true},
        {name: "omission stays omitted", body: `{"model":"gpt-5.6"}`, max: "low", path: "reasoning_effort", want: "", changed: false},
        {name: "future value stays intact", body: `{"reasoning_effort":"future"}`, max: "low", path: "reasoning_effort", want: "future", changed: false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, changed := ApplyOpenAIReasoningEffortPolicy([]byte(tt.body), tt.max, tt.mappings)
            require.Equal(t, tt.changed, changed)
            require.Equal(t, tt.want, gjson.GetBytes(got, tt.path).String())
        })
    }
}
```

- [ ] **Step 2: Run the focused test and observe RED**

Run: `cd backend && go test ./internal/service -run 'Test(NormalizeMaxReasoningEffort|NormalizeReasoningEffortMappings|ApplyOpenAIReasoningEffortPolicy)' -count=1 -v`

Expected: build failure naming the missing policy functions/types.

- [ ] **Step 3: Implement the minimal policy module**

Define the shared type and service alias:

```go
// internal/domain/reasoning_effort.go
type ReasoningEffortMapping struct {
    From string `json:"from"`
    To   string `json:"to"`
}

// internal/service/group.go
type ReasoningEffortMapping = domain.ReasoningEffortMapping
```

Implement `NormalizeMaxReasoningEffort`, `normalizeMaxReasoningEffortForPlatform`, `NormalizeReasoningEffortMappings`, `sanitizeGroupReasoningEffortPolicy`, and `ApplyOpenAIReasoningEffortPolicy`. Check raw trimmed mapping lengths before canonicalization, use ranks `minimal=1` through `max=6`, capture the original canonical value once so mappings do not chain, then cap recognized output. Do not inject fields or replace malformed/non-string fields.

- [ ] **Step 4: Run GREEN and formatting**

Run: `cd backend && gofmt -w internal/domain/reasoning_effort.go internal/service/openai_reasoning_effort_policy.go internal/service/openai_reasoning_effort_policy_test.go internal/service/group.go && go test ./internal/service -run 'Test(NormalizeMaxReasoningEffort|NormalizeReasoningEffortMappings|ApplyOpenAIReasoningEffortPolicy)' -count=1 -v`

Expected: all focused policy tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/domain/reasoning_effort.go backend/internal/service/group.go backend/internal/service/openai_reasoning_effort_policy.go backend/internal/service/openai_reasoning_effort_policy_test.go
git commit -m "feat(openai): add group reasoning effort policy"
```

### Task 2: Persist and expose group policy

**Files:**
- Create: `backend/migrations/132_add_group_reasoning_effort_policy.sql`
- Modify: `backend/ent/schema/group.go`
- Regenerate: `backend/ent/**`
- Modify: `backend/internal/service/group.go`
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/service/admin_service_group_test.go`
- Modify: `backend/internal/repository/group_repo.go`
- Modify: `backend/internal/repository/api_key_repo.go`
- Modify: `backend/internal/handler/admin/group_handler.go`
- Create: `backend/internal/handler/admin/group_handler_reasoning_effort_test.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `backend/internal/service/api_key_auth_cache.go`
- Modify: `backend/internal/service/api_key_auth_cache_impl.go`
- Modify: `backend/internal/service/api_key_auth_cache_version_test.go`
- Modify: `backend/internal/service/api_key_service_cache_test.go`

- [ ] **Step 1: Write failing service, handler, DTO, and cache tests**

Add tests proving create/update canonicalize `MAX` and `x-high`, reject non-OpenAI policy and duplicate mappings, clear policy when a group leaves OpenAI, emit canonical fields in the admin DTO, and deep-copy both snapshot directions. The local fork does not expose the upstream group-duplication workflow. A cache mutation assertion must follow this shape:

```go
snapshot := svc.snapshotFromAPIKey(apiKey)
apiKey.Group.ReasoningEffortMappings[0].To = "low"
require.Equal(t, "xhigh", snapshot.Group.ReasoningEffortMappings[0].To)

restored := svc.snapshotToAPIKey("key", snapshot)
snapshot.Group.ReasoningEffortMappings[0].To = "medium"
require.Equal(t, "xhigh", restored.Group.ReasoningEffortMappings[0].To)
```

- [ ] **Step 2: Run focused tests and observe RED**

Run: `cd backend && go test ./internal/service ./internal/handler/admin -run 'ReasoningEffort|AuthCache.*Group' -count=1 -v`

Expected: compilation failures for absent group/input/snapshot fields.

- [ ] **Step 3: Add the migration and Ent schema**

```sql
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS max_reasoning_effort VARCHAR(20) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS reasoning_effort_mappings JSONB NOT NULL DEFAULT '[]'::jsonb;
```

Add schema fields:

```go
field.String("max_reasoning_effort").MaxLen(20).Default(""),
field.JSON("reasoning_effort_mappings", []domain.ReasoningEffortMapping{}).
    Default([]domain.ReasoningEffortMapping{}).
    SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
```

Run: `cd backend && go generate ./ent`

Expected: Ent accessors/mutations/schema include both columns.

- [ ] **Step 4: Thread fields through service, repository, handler, DTO, and cache**

Add the exact fields to every boundary:

```go
MaxReasoningEffort      string
ReasoningEffortMappings []ReasoningEffortMapping
```

On create/update, call `normalizeMaxReasoningEffortForPlatform` and `NormalizeReasoningEffortMappings` before persistence. For a non-OpenAI platform with empty inputs persist neutral values; when platform changes away from OpenAI force `""` and `[]`. Repository writes use Ent setters; entity mapping calls `sanitizeGroupReasoningEffortPolicy`. Admin handlers bind `max_reasoning_effort` and `reasoning_effort_mappings`; admin DTOs emit both. Snapshot version increments by one, and snapshot conversion uses `append([]ReasoningEffortMapping(nil), source...)` in both directions.

- [ ] **Step 5: Run GREEN plus repository compilation**

Run: `cd backend && gofmt -w ent/schema/group.go internal/domain/reasoning_effort.go internal/service internal/repository internal/handler/admin internal/handler/dto && go test ./internal/service ./internal/handler/admin ./internal/repository -run 'ReasoningEffort|AuthCache.*Group|Group.*Mapping' -count=1 -v`

Expected: focused tests pass and all three packages compile.

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/132_add_group_reasoning_effort_policy.sql backend/ent backend/internal/service backend/internal/repository backend/internal/handler/admin backend/internal/handler/dto
git commit -m "feat(groups): persist OpenAI reasoning effort policy"
```

### Task 3: GPT-5.6-aware usage normalization and OAuth compact

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/openai_gateway_service_hotpath_test.go`
- Create: `backend/internal/service/openai_gpt56_max_test.go`
- Create: `backend/internal/service/openai_reasoning_effort_candidates_test.go`
- Modify: `backend/internal/service/openai_oauth_passthrough_test.go`

- [ ] **Step 1: Write failing model matrix and compact tests**

Test explicit and suffix-derived `max` for GPT-5.6 Sol/Terra/Luna, dated forms, and provider-prefixed forms. Assert non-GPT-5.6 `max` records `xhigh`, `minimal`/`none` remain absent, and ordered model candidates recover `gpt-5.6-sol-max` after the final model loses its suffix. Add compact cases that downgrade only GPT-5.6 OAuth `/compact` requests:

```go
tests := []struct { model, effort string; compact, oauth bool; want string }{
    {model: "gpt-5.6-sol", effort: "max", compact: true, oauth: true, want: "xhigh"},
    {model: "gpt-5.6-sol", effort: "max", compact: false, oauth: true, want: "max"},
    {model: "gpt-5.6-sol", effort: "max", compact: true, oauth: false, want: "max"},
    {model: "gpt-5.4", effort: "max", compact: true, oauth: true, want: "max"},
}
```

- [ ] **Step 2: Run focused tests and observe RED**

Run: `cd backend && go test ./internal/service -run 'Test.*(GPT56|ReasoningEffortCandidates|Compact.*Max|ExtractOpenAIReasoningEffort)' -count=1 -v`

Expected: `max` is empty or normalized incorrectly and candidate-list calls do not compile.

- [ ] **Step 3: Implement model-aware extraction**

Change map/body extractors to variadic model candidates. Explicit values use the first non-empty candidate to decide whether `max` stays `max`; suffix fallback scans candidates in order. Add a focused predicate that lowercases, strips provider prefixes, removes an effort suffix, and recognizes the GPT-5.6 Sol/Terra/Luna families and dated variants. Preserve existing `low`/`medium`/`high`/`xhigh`; map compatibility `max` to `xhigh` only in usage metadata.

- [ ] **Step 4: Implement OAuth compact downgrade**

Extend compact normalization with the effective request model. For OAuth passthrough and the ChatGPT `/compact` endpoint, rewrite nested `reasoning.effort=max` to `xhigh` only when the model predicate is GPT-5.6. Feed the rewritten body to usage extraction so recorded metadata is `xhigh`. Leave API-key and ordinary Responses bodies intact.

- [ ] **Step 5: Run GREEN**

Run: `cd backend && gofmt -w internal/service/openai_gateway_service.go internal/service/openai_*test.go && go test ./internal/service -run 'Test.*(GPT56|ReasoningEffortCandidates|Compact.*Max|ExtractOpenAIReasoningEffort)' -count=1 -v`

Expected: the full focused model/compact matrix passes.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_service_hotpath_test.go backend/internal/service/openai_gpt56_max_test.go backend/internal/service/openai_reasoning_effort_candidates_test.go backend/internal/service/openai_oauth_passthrough_test.go
git commit -m "fix(openai): preserve GPT-5.6 max reasoning metadata"
```

### Task 4: Apply policy to HTTP Responses and Chat Completions

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_chat_completions.go`
- Create: `backend/internal/handler/openai_reasoning_effort_policy_test.go`
- Modify: `backend/internal/service/gateway_forward_as_chat_completions_test.go`
- Modify: `backend/internal/service/gateway_forward_as_responses_test.go`

- [ ] **Step 1: Write failing HTTP ingress tests**

Build authenticated OpenAI groups with `max -> high` plus an `xhigh` ceiling. Assert Responses nested fields and Chat Completions flat fields are rewritten before downstream model extraction. Include both raw Chat Completions and converted-to-Responses paths, omission preservation, malformed JSON returning the existing parse error, and a non-OpenAI group remaining neutral.

- [ ] **Step 2: Run focused tests and observe RED**

Run: `cd backend && go test ./internal/handler ./internal/service -run 'Test.*ReasoningEffortPolicy.*(Responses|ChatCompletions)' -count=1 -v`

Expected: forwarded bodies still contain the original effort.

- [ ] **Step 3: Apply policy once at HTTP ingress**

Immediately after authentication/body read, replace the local body with:

```go
if apiKey.Group != nil && apiKey.Group.Platform == service.PlatformOpenAI {
    body, _ = service.ApplyOpenAIReasoningEffortPolicy(
        body,
        apiKey.Group.MaxReasoningEffort,
        apiKey.Group.ReasoningEffortMappings,
    )
}
```

All subsequent model parsing, conversion, account selection, retries, and usage extraction must consume this single effective body. Do not apply the mapping a second time inside a retry.

- [ ] **Step 4: Run GREEN**

Run: `cd backend && gofmt -w internal/handler/openai_gateway_handler.go internal/handler/openai_chat_completions.go internal/handler/openai_reasoning_effort_policy_test.go && go test ./internal/handler ./internal/service -run 'Test.*ReasoningEffortPolicy.*(Responses|ChatCompletions)' -count=1 -v`

Expected: raw and converted HTTP cases pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_chat_completions.go backend/internal/handler/openai_reasoning_effort_policy_test.go backend/internal/service/gateway_forward_as_chat_completions_test.go backend/internal/service/gateway_forward_as_responses_test.go
git commit -m "feat(openai): enforce reasoning policy on HTTP ingress"
```

### Task 5: Apply policy per WebSocket turn and retain model candidates

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/service/openai_ws_forwarder.go`
- Create: `backend/internal/service/openai_ws_reasoning_effort_policy_test.go`

- [ ] **Step 1: Write failing WebSocket tests**

Exercise at least two turns on one session: first `max`, then `low`. Assert the policy is independently applied on both turns, omission on a later turn does not inherit effort, and mapped/original model candidates preserve a `-max` suffix after channel mapping strips it. Cover both map-based and byte-payload WS paths in this fork.

- [ ] **Step 2: Run focused tests and observe RED**

Run: `cd backend && go test ./internal/service ./internal/handler -run 'Test.*WS.*ReasoningEffort' -count=1 -v`

Expected: per-turn payloads retain the unbounded value or usage loses suffix metadata.

- [ ] **Step 3: Carry and apply the group policy**

Store the authenticated group's normalized maximum and a copied mapping slice in the forwarding session/context. At each inbound turn, invoke `ApplyOpenAIReasoningEffortPolicy` before turn-level model mapping. Pass usage extraction candidates in this order: final upstream model, mapped/billing model, original client model. Keep the original client model immutable for suffix recovery.

- [ ] **Step 4: Run GREEN**

Run: `cd backend && gofmt -w internal/handler/openai_gateway_handler.go internal/service/openai_ws_forwarder.go internal/service/openai_ws_reasoning_effort_policy_test.go && go test ./internal/service ./internal/handler -run 'Test.*WS.*ReasoningEffort' -count=1 -v`

Expected: all per-turn and candidate tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/openai_gateway_handler.go backend/internal/service/openai_ws_forwarder.go backend/internal/service/openai_ws_reasoning_effort_policy_test.go
git commit -m "feat(openai): enforce reasoning policy on WebSocket turns"
```

### Task 6: Strip foreign reasoning shapes during cross-mode failover

**Files:**
- Create: `backend/internal/handler/openai_gateway_reasoning_failover.go`
- Create: `backend/internal/handler/openai_gateway_reasoning_failover_test.go`
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Create: `backend/internal/service/openai_gateway_request_body_reasoning_test.go`

- [ ] **Step 1: Write failing cleanup tests**

Table-test same-mode retries, passthrough-to-non-passthrough failover, single-object and array `input` shapes, and bodies containing both encrypted and ordinary reasoning items. Assert same-mode retries preserve the policy-mutated body while a mode change removes only provider-specific encrypted reasoning items.

- [ ] **Step 2: Run focused tests and observe RED**

Run: `cd backend && go test ./internal/handler ./internal/service -run 'Test(OpenAIReasoningFailover|SanitizeOpenAICrossModeFailoverReasoning)' -count=1 -v`

Expected: missing helper symbols or foreign encrypted reasoning items remain.

- [ ] **Step 3: Implement immutable-source retry cleanup**

Add a request-body helper that decodes a fresh copy and removes complete `input` items whose type is `reasoning` and that carry `encrypted_content`. In the retry loop, derive each attempt body from the immutable policy-applied body; after a passthrough attempt, strip those provider-specific items for every later non-passthrough attempt. Never mutate the shared source body in place.

- [ ] **Step 4: Run GREEN**

Run: `cd backend && gofmt -w internal/handler/openai_gateway_reasoning_failover.go internal/handler/openai_gateway_reasoning_failover_test.go internal/handler/openai_gateway_handler.go internal/service/openai_gateway_service.go internal/service/openai_gateway_request_body_reasoning_test.go && go test ./internal/handler ./internal/service -run 'Test(OpenAIReasoningFailover|SanitizeOpenAICrossModeFailoverReasoning)' -count=1 -v`

Expected: cleanup matrix passes.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handler/openai_gateway_reasoning_failover.go backend/internal/handler/openai_gateway_reasoning_failover_test.go backend/internal/handler/openai_gateway_handler.go backend/internal/service/openai_gateway_service.go backend/internal/service/openai_gateway_request_body_reasoning_test.go
git commit -m "fix(openai): clean reasoning fields across failover modes"
```

### Task 7: Add the OpenAI group policy management UI

**Files:**
- Modify: `frontend/src/types/index.ts`
- Create: `frontend/src/views/admin/groupsReasoningEffort.ts`
- Create: `frontend/src/components/admin/group/ReasoningEffortPolicyFields.vue`
- Modify: `frontend/src/views/admin/GroupsView.vue`
- Modify: `frontend/src/components/common/Select.vue`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Create: `frontend/src/views/admin/__tests__/groupsReasoningEffort.spec.ts`

- [ ] **Step 1: Write failing helper/component tests**

Test canonical hydration, independent array cloning, create/edit serialization, maximum options including `max`, add/remove rows, incomplete-row and duplicate-source errors, OpenAI-only rendering, and clearing form policy when the platform changes away from OpenAI.

- [ ] **Step 2: Run focused Vitest and observe RED**

Run: `cd frontend && ./node_modules/.bin/vitest run src/views/admin/__tests__/groupsReasoningEffort.spec.ts`

Expected: missing module/component or failed field assertions.

- [ ] **Step 3: Add types and pure helpers**

```ts
export type ReasoningEffortPolicyValue = 'minimal' | 'low' | 'medium' | 'high' | 'xhigh' | 'max'
export interface ReasoningEffortMapping { from: ReasoningEffortPolicyValue; to: ReasoningEffortPolicyValue }
```

Add optional `max_reasoning_effort` and `reasoning_effort_mappings` to `Group`, `CreateGroupRequest`, and `UpdateGroupRequest`. Helpers return fresh arrays and validate complete rows plus normalized unique sources.

- [ ] **Step 4: Build controls and integrate forms**

Render the component only when `platform === 'openai'`. Provide Unlimited plus all six ranked values, mapping row selectors, add/remove buttons, and inline errors. Hydrate edit state with cloned canonical mappings; submit cloned arrays. Watch platform changes and clear maximum/mappings when leaving OpenAI. Add matching English and Chinese keys under `admin.groups.reasoningEffort` in the existing monolithic locale files.

- [ ] **Step 5: Run GREEN and typecheck**

Run: `cd frontend && ./node_modules/.bin/vitest run src/views/admin/__tests__/groupsReasoningEffort.spec.ts && ./node_modules/.bin/vue-tsc --noEmit`

Expected: focused tests and Vue TypeScript check pass.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/views/admin/groupsReasoningEffort.ts frontend/src/components/admin/group/ReasoningEffortPolicyFields.vue frontend/src/views/admin/GroupsView.vue frontend/src/components/common/Select.vue frontend/src/i18n/locales/en.ts frontend/src/i18n/locales/zh.ts frontend/src/views/admin/__tests__/groupsReasoningEffort.spec.ts
git commit -m "feat(admin): configure OpenAI reasoning effort policy"
```

### Task 8: Full verification and scope audit

**Files:**
- Verify all changed files
- Update: `docs/superpowers/specs/2026-07-28-openai-reasoning-effort-sync-design.md` only if implemented behavior differs from the approved text

- [ ] **Step 1: Run focused backend suites**

Run: `cd backend && go test ./internal/service ./internal/handler/admin ./internal/handler ./internal/repository -count=1`

Expected: zero failures.

- [ ] **Step 2: Run the complete backend unit suite**

Run: `cd backend && go test -tags=unit ./... -count=1`

Expected: zero failures.

- [ ] **Step 3: Run frontend verification**

Run: `cd frontend && ./node_modules/.bin/vitest run src/views/admin/__tests__/groupsReasoningEffort.spec.ts src/views/admin/__tests__/groupsMessagesDispatch.spec.ts && ./node_modules/.bin/vue-tsc --noEmit`

Expected: all selected tests and typecheck pass.

- [ ] **Step 4: Audit generated files and scope**

Run:

```bash
git status --short
git diff --check
git diff --stat 00e808b0..HEAD
git diff 00e808b0..HEAD -- backend/internal frontend/src backend/migrations backend/ent/schema
```

Expected: no whitespace errors, no unrelated platform behavior changes, migration number `132`, and all acceptance criteria trace to tests.

- [ ] **Step 5: Commit any verification-only corrections**

```bash
git add -A ':!frontend/node_modules'
git commit -m "test(openai): complete reasoning effort coverage"
```

Skip this commit when verification produces no corrections.
