# Implementation Plan: Issue #2584 AF Policy and Managed-Resource Read Projection

**Status**: In progress — ACM Console inventory temporarily unavailable pending live qualification
**Branch**: `openchamber/issue-2584-console-policy-read`
**Business requirements**: BR-SCOPE-001, BR-SECURITY-016, BR-AI-085, BR-AI-088, BR-AUDIT-005
**Test plan**: [TP-2584-v1.0](TEST_PLAN.md)
**Related Console issue**: `kubernaut-console#164`
**Operator dependency**: PR #604, merged as `b29f3c89058f0a96f9464788b6a99b2bc9581311`

> The contract decisions in Section 3 were approved before implementation. Production
> code and tests are present on this branch. Live ACM Search testing exposed inventory
> contract gaps; the user approved a temporary fail-closed gate while those gaps are
> resolved. ACM inventory qualification remains an implementation blocker, alongside
> the separately scoped cross-repository deployment evidence.
>
> **Console metadata decision (2026-11-03/04):** The user approved query-time
> reads without a Console API freshness object, freshness-marker rejection,
> snapshot identifier, count/total, reload metadata, or page-observation
> timestamp. The Console owns render time; historical evaluation timestamps
> remain source record timestamps. Detected backend errors, identity mismatches,
> authorization failures, and incomplete traversal remain fail-closed.

---

## 1. Objective and Completion Contract

Deliver a versioned API Frontend REST read surface that lets an authenticated interactive
Console user inspect a sanitized projection of:

- effective Kubernaut resource scope and provenance;
- current AIAnalysis and SignalProcessing policy identity/revision metadata;
- the required operator-supplied AIAnalysis approval threshold;
- bounded historical evaluation metadata from existing CRD fields; and
- stable links to the shared managed-resource inventory.

The feature is complete only when:

1. `/api/v1/console/...` is authenticated, same-origin compatible, and represented in OpenAPI.
2. Every requested cluster/resource/namespace identity is authorized server-side.
3. Resource-label precedence, namespace inheritance, and cluster-scoped safe defaults are preserved.
4. Raw Rego, secrets, tokens, arbitrary payloads, private reasoning, and internal URLs never appear.
5. `success`, `empty`, `partial`, `unavailable`, `forbidden`, and `not_found` are explicit;
   `loading` remains a Console client state. Failure states contain no data or counts;
   the shared `stale` state is not derived from local freshness metadata.
6. Existing A2A/MCP and service-account protocol behavior is unchanged.
7. AF unit, integration, wiring, build, lint, and control tests pass; Console #164 receives the frozen contract.

### 1.1 Preflight Evidence

- AF currently has authenticated A2A/MCP routes and advisory `/a2a/access`, but no typed
  policy/scope/inventory REST surface.
- `auth.UserIdentityFromContext` provides verified user identity and an explicit
  `IsServiceAccount` marker; the Console's `ProxyAuthProvider.getToken()` remains empty.
- `pkg/shared/scope.Manager` already implements the required precedence and safe default,
  but returns only a boolean; provenance needs a structured result that preserves the current API.
- AF already has local dynamic/typed Kubernetes clients, REST mapping, the existing
  FMC/ACM-backed `ScopeChecker`, fleet reader hooks for agentic/MCP tools, and ConfigMap
  access. Console inventory must use the local clients or a new typed FMC/ACM control-plane
  projection; it must not reuse the Console path's fleet reader hooks. Operator PR #604
  injects the two effective policy names under `policyRead` without parsing service runtime
  ConfigMaps.
- The existing `ScopeChecker` proves an exact remote managed-scope decision but does not
  enumerate resources or return provenance. FMC and ACM retain the shared sanitized
  exact/detail projection contract; remote list qualification is outside this release.
- AIAnalysis stores target identity and policy-evaluation fields. SignalProcessing stores
  target identity, cluster ID, phase/classification, and policy hash, but not a dedicated
  target index or guaranteed matched-rule list.
- Existing AF operator RBAC must be rechecked for `signalprocessings` read access if v1
  promises SignalProcessing historical evaluation metadata. PR #604's no-new-RBAC claim
  applies to named policy ConfigMap reads, not necessarily CRD history.

**Preflight confidence**: 95% for the boundary, source ownership, scope rules, AF wiring
seams, threshold source, and the approved FMC/ACM control-plane direction; 90% for the
new control-plane inventory payload and SignalProcessing-history wiring until their RED
contracts are written.

---

## 2. Scope and Non-Goals

### 2.1 In Scope

- AF config for operator-rendered `policyRead` names and the required threshold metadata.
- Three read operations:
  - `GET /api/v1/console/policies/scope`
  - `GET /api/v1/console/inventory/managed-resources`
  - `GET /api/v1/console/inventory/managed-resources/{resourceID}`
- Typed OpenAPI schemas and RFC 7807 invalid-request errors.
- Local inventory and policy-scope reads for `cluster_id=local`; non-local list operations
  are unavailable in this release. Existing typed FMC/ACM exact/get and scope-check
  behavior remains unchanged; no direct Console reads through MCP or remote Kubernetes
  clients.
- Structured scope provenance built on existing `pkg/shared/scope` rules.
- Current policy metadata from only the configured ConfigMap name/key and a safe revision hash.
- Bounded historical matching against existing AIAnalysis/SignalProcessing CRD fields.
- Cursor pagination, completeness, exact filters, safe opaque resource IDs, and links.
- Named read capabilities, target-scope SARs, service-account separation, and audit attribution.
- Operator/Console contract handoff and any separately approved operator RBAC/threshold follow-up.
- Existing FMC/ACM exact/detail projection contract and AF adapters; remote list qualification
  remains outside this release.

### 2.2 Out of Scope

- Policy editing, testing, deployment, or Rego evaluation in AF.
- Direct browser-to-Kubernetes or browser-to-DataStorage access.
- Direct AF Console access to remote Kubernetes/MCP clusters; remote inventory is owned by
  the fleet control plane.
- New A2A/MCP tools or changes to existing agentic behavior.
- Full labels/annotations, object specs/status, Secret/ConfigMap data, tokens, internal
  service URLs, LLM prompts, private reasoning, or arbitrary audit payloads.
- A new historical index or changes to AIAnalysis/SignalProcessing CRD schemas in this issue.
- Cross-cluster aggregation without an explicit cluster identity and complete source reads.

---

## 3. Approval Gates and Recommended Decisions

### 3.1 Boundary — Recommended

Use AF REST under `/api/v1/console/...`. The route is authenticated by existing AF
middleware and is intended to be reached through the same-origin OAuth2 Proxy path. The
Console stores no bearer token and never calls DataStorage directly. A2A/MCP routes and
their service-account delegation remain separate.

### 3.2 Threshold Contract — Direction Approved

Every new v1 deployment must provide the threshold already owned by AIAnalysis:
`spec.aiAnalysis.confidenceThreshold`. The operator continues rendering that value into
AIAnalysis `rego.confidenceThreshold` and additionally injects the same validated value
into AF's generated `policyRead` block:

```yaml
policyRead:
  aiAnalysis:
    configMapName: aianalysis-policy
    confidenceThreshold: "0.85"
  signalProcessing:
    configMapName: signalprocessing-policy
```

`policyRead.aiAnalysis.confidenceThreshold` is derived output, not a second user-editable
source of truth. The operator must require a finite decimal in `(0.0, 1.0]` and reject a
missing or invalid value during validation/reconciliation. AF repeats the validation at
startup and does not become ready when the required value is absent or invalid.

The read projection returns a required, non-null configured/supplied approval threshold
with source `operator-config`. It does not return `unknown` in v1. The field must not be
described as the universal effective threshold of arbitrary Rego: custom policy logic
may apply different rules, and AIAnalysis's separate `lowConfidenceFloor` is not this
approval threshold. If the Console later needs both values, that is a separate explicit
contract addition.

AF must not parse `aianalysis-config`, inspect arbitrary Rego to discover defaults, or
claim a policy default that the operator did not supply. Existing deployments upgraded
to this clean-break contract must provide the field before reconciliation succeeds;
backward-compatible inference is intentionally not supported.

### 3.3 Authorization Contract — Recommended

Require all of the following for Console REST reads:

1. Verified AF identity from middleware; missing identity is denied.
2. `UserIdentity.IsServiceAccount == false` on the interactive `/api/v1/console` surface.
3. Named capability SARs:
   - `kubernaut_read_policy_scope`
   - `kubernaut_list_managed_resources`
   - `kubernaut_get_managed_resource`
4. For local targets, user/target resource SARs for the exact hub
   group/version/resource, namespace, and name being returned. List requests use list
   permission for the requested scope; exact detail uses get permission.
5. For remote targets, the existing AF `ScopeChecker` proves the exact managed scope
   through the configured FMC or ACM control plane. This is an AF service-identity
   transport check, not delegation of the interactive user's remote Kubernetes token.
6. The existing optional `kubernaut.ai/console` gate, when enabled, remains additive.

Coarse console access alone is rejected: it is optional and does not establish
resource-level authorization. Trusting only the AF ServiceAccount is also rejected,
because that would collapse all users into one cluster visibility boundary.

**Approved**: use named capabilities plus local target SARs and the existing FMC/ACM
managed-scope check for remote targets. The optional coarse console gate remains
additive, and service accounts remain on the existing A2A/MCP paths only. User-token
delegation to remote Kubernetes is not part of #2584.

### 3.4 Historical SignalProcessing Contract — Approved

The v1 response includes historical metadata only where the existing CRD persists it.
Option A is approved: the operator grants the AF ServiceAccount minimal read-only
`get/list` access to `signalprocessings`, with RBAC tests and no write verbs.

AF must never return a partial record that implies complete history when a required source
is unreadable.

### 3.5 Design Decision Gate

`DD-AF-018` is accepted in `docs/architecture/decisions/` and records the selected
boundary, alternatives, authorization model, threshold ownership, historical limits,
and security consequences. The implementation was started only after the contract was
frozen and the RED tests were written.

---

## 4. API Contract to Freeze Before RED

### 4.1 Routes and Scope Identity

```text
GET /api/v1/console/policies/scope
GET /api/v1/console/inventory/managed-resources
GET /api/v1/console/inventory/managed-resources/{resourceID}
```

Every list/exact request requires `cluster_id`. `local` maps to the hub cluster. Policy-
scope and managed-resource list operations support only `local` in this release; any
non-local list request returns state-only `unavailable`. Non-local exact reads retain
the configured FMC/ACM control-plane projection and existing managed-scope checker;
AF never selects a remote Kubernetes reader for this Console path. A missing, unknown,
unavailable, or unauthorized cluster never falls back to local or selects a best-effort
cluster.

Supported filters are `cluster_id`, `namespace`, `group`, `version`, `kind`, `name`,
`resource_id`, `limit`, and opaque `cursor`. Unsupported, duplicated, malformed, or
contradictory filters return HTTP 400. There is no offset pagination and no `total`.
Default `limit` is 50; maximum is 100. Results sort deterministically by cluster,
group/version/kind, namespace, name, and resource ID.

For non-local exact reads, AF uses the existing narrow `FleetInventoryReader` abstraction.
FMC and ACM remain separate transports for the canonical detail model: cluster and GVK
identity, namespace/name, effective managed state, resource/namespace label state, and
provenance. They must not return full Kubernetes objects, arbitrary labels/annotations,
or policy contents. The existing `ScopeChecker` remains the exact-target managed-scope
check and must be applied before a remote exact response is returned. A control-plane
timeout or permission failure is a state-only AF response; AF does not retry by switching
to the local Kubernetes reader.

### 4.2 Success Projection

The exact casing and allowed nullable fields must be frozen in
`api/apifrontend/openapi/apifrontend-v1.yaml` before RED. The projection is typed and
allowlisted; the example below is illustrative, not permission to return source data:

```json
{
  "apiVersion": "console.kubernaut.ai/v1",
  "kind": "PolicyScopeList",
  "state": "success",
  "completeness": "complete",
  "items": [{
    "id": "opaque-stable-resource-id",
    "resource": {
      "clusterId": "local",
      "apiVersion": "apps/v1",
      "kind": "Deployment",
      "namespace": "payments",
      "name": "checkout"
    },
    "scope": {
      "included": true,
      "source": "resource-label",
      "resourceLabel": "true",
      "namespaceLabel": "true",
      "clusterScoped": false
    },
    "currentPolicies": [{
      "type": "aiAnalysis",
      "configMapName": "aianalysis-policy",
      "configMapKey": "approval.rego",
      "revision": "sha256:...",
      "threshold": {"value": 0.85, "source": "operator-config", "kind": "rego-approval"}
    }],
    "historicalEvaluations": [{
      "source": "AIAnalysis",
      "observedAt": "2026-11-01T19:59:00Z",
      "policyHash": "...",
      "matchedRules": ["production_requires_review"],
      "decision": "ManualReviewRequired"
    }],
    "inventoryLink": "/api/v1/console/inventory/managed-resources/opaque-stable-resource-id"
  }],
  "nextCursor": null
}
```

SignalProcessing history exposes only fields actually persisted. Missing rules or
decision data is `null`/unknown, never reconstructed from Rego guesses. The
`historicalEvaluations[].observedAt` value is the source record timestamp; it is not
a page observation time or a Console freshness marker.

### 4.3 Read-State and Error Contract

| State | HTTP | Returned data |
|---|---:|---|
| `success` | 200 | Complete items and cursor |
| `empty` | 200 | Empty items, no total |
| `partial` | 503 | State, request ID, generic message only |
| `stale` | 503 | Reserved shared state; no local freshness metadata is exposed |
| `unavailable` | 503 | State, request ID, generic message only |
| `forbidden` | 403 | State, request ID, generic message only |
| `not_found` | 404 | State, request ID, generic message only |
| `loading` | client-only | AF never emits it |

For an exact resource ID, target-SAR denial is intentionally indistinguishable from
not-found to prevent existence probing. Capability denial is `forbidden`. Error bodies
never include counts, policy names, ConfigMap names, labels, namespaces, or provenance.

---

## 5. Proposed Design

### 5.1 Configuration

Extend `pkg/apifrontend/config.Config` with a typed `PolicyRead` block matching the
operator-rendered YAML. The default policy keys are fixed by policy type:

| Policy | ConfigMap source | Key |
|---|---|---|
| AIAnalysis | `policyRead.aiAnalysis.configMapName` | `approval.rego` |
| SignalProcessing | `policyRead.signalProcessing.configMapName` | `policy.rego` |

AF reads only configured ConfigMaps in the AF/operator namespace. A missing name,
invalid name, missing key, missing required threshold, or read error is a source
failure; it never falls back to a hard-coded name or runtime-config parse.
The existing AF config watcher validates changed content for drift detection only;
`policyRead` changes are startup-only and require an AF restart before they apply.

### 5.2 Projection Service

Create a focused `pkg/apifrontend/policyread` package rather than placing business logic
in HTTP handlers. Use narrow interfaces for:

- policy ConfigMap metadata/data-key access;
- typed AIAnalysis and SignalProcessing list access;
- local/fleet inventory reads;
- structured scope resolution;
- capability/resource authorization; and
- audit emission.

The service sequence is:

1. Validate identity, service-account status, cluster ID, filters, limit, and cursor before broad reads.
2. Authorize the endpoint capability and requested list/exact target scope.
3. Select the explicit local Kubernetes reader for list/policy-scope operations, or the
   existing typed FMC/ACM path for non-local exact reads; never fall back across clusters
   or switch to AF's MCP reader.
4. Enumerate only an allowlisted, non-sensitive inventory kind set and project identities.
   Non-local list operations remain unavailable in this release.
5. Resolve scope with resource-label precedence, namespace fallback only for namespaced resources,
   and safe unmanaged defaults.
6. Read current policy metadata and calculate a non-reversible `sha256:` revision from the
   configured policy key without returning policy contents.
7. Match bounded historical CRD records by exact target identity and cluster attribution.
8. Require all required reads and joins to complete. On partial/unavailable results,
   discard all records and return a state-only response.
9. Emit an attributable audit event with normalized action/outcome/filter fingerprint,
   never response payload or raw policy.

### 5.3 Scope Provenance

Extend `pkg/shared/scope` with a structured resolution result and keep
`IsManagedResource` as a compatibility wrapper. The structured result preserves:

- included/managed boolean;
- resource label state: true, false, absent, or invalid;
- namespace label state: true, false, absent, or invalid;
- source: resource-label, namespace-label, none, or unavailable;
- cluster-scoped flag; and
- safe-default reason for internal logs only.

Invalid labels are recorded as invalid internal input but treated as absent for the
effective decision. A namespaced resource with an invalid/absent resource label may
inherit a valid namespace label; a cluster-scoped resource never inherits namespace
scope. If no valid label remains, the effective result is unmanaged.

Do not implement a second scope algorithm in AF.

### 5.4 Historical Evaluation Matching

Use only persisted fields:

- AIAnalysis `Spec.AnalysisRequest.TargetResource`, `Spec.ClusterID`, and
  `Status.Approval.ApprovalContext.PolicyEvaluation` fields.
- SignalProcessing `Spec.Signal.TargetResource`, `Spec.Signal.ClusterID`, and
  `Status.Phase`/`Status.CompletionTime` plus
  `Status.SignalClassification.PolicyHash`/classification fields.
- Existing API-version/group mapping only where unambiguous.

Sort newest observed/completed records first and cap history per projection. If a
required source cannot be completely read, return `partial` with no items. No arbitrary
name, alert text, or inferred relationship may join records.

### 5.5 Authorization Adapter

Add a focused Console read authorization adapter around existing AF identity and SAR
infrastructure. It must:

- use the verified user/groups from request context;
- reject service accounts on the interactive surface;
- check named capability SARs;
- check exact local target/list resource SARs;
- use the existing AF `ScopeChecker` for exact remote managed-scope checks through
  FMC or ACM;
- preserve the existing `ConsoleAuthorizer` behavior for `/a2a/access`; and
- avoid changing A2A/MCP tool authorization.

Do not pass browser tokens to DataStorage or to remote Kubernetes/MCP. The AF
ServiceAccount is a backend transport identity, not the caller identity. The fleet
control-plane inventory contract is separately authenticated and returns only the
allowlisted projection.

### 5.6 Router and Production Wiring

Add an additive Console read handler field to `handler.RouterConfig` and register the
versioned Console path through the existing authenticated middleware chain in
`pkg/apifrontend/handler/router.go`. Preserve all existing routes and middleware order.

Construct the handler and its source/auth dependencies in the existing production path.
The local source uses AF's Kubernetes clients; the remote source uses a typed FMC/ACM
control-plane inventory adapter and the already-wired `ScopeChecker`:

- `cmd/apifrontend/main.go:buildHandlerDeps`;
- `cmd/apifrontend/main.go:buildRouterConfig`; and
- `cmd/apifrontend/backend_deps.go:backendDeps`/`buildBackendDeps`.

The route must be exercised through `cmd/apifrontend`, not only by calling a handler
directly in a package test.

### 5.7 Audit and Metrics

Extend the existing AF/DataStorage typed audit union with Console read actions; do not
use the adapter's logged-only untyped-event path. Emit one typed event for every
authenticated Console read request, including successful, forbidden, not-found,
partial, and unavailable outcomes. Unauthenticated requests remain covered by
the existing authentication events.

Use AF event types `console.policy_scope_read`,
`console.managed_resource_list_read`, and `console.managed_resource_get_read`; the
adapter persists them with its existing `apifrontend.` prefix. Keep the existing
`success`/`failure` audit outcome enum and carry the detailed bounded state in the typed
payload.

Each event carries actor type/ID, endpoint operation, cluster ID, request/correlation ID,
result state/outcome, and a bounded normalized filter or resource fingerprint. It must
not carry returned records, item payloads, raw policy text, secrets, tokens, private
reasoning, or arbitrary query values. Do not emit one event per returned record.

Add typed event constants, action/outcome mappings, payload builders, OpenAPI union
schemas, generated client types, and adapter tests for policy-scope reads, managed-
resource list reads, and managed-resource exact reads. DataStorage persistence failure
must remain observable and must not turn a successful business response into a silent
audit gap.

Metrics count outcome classes without labels containing user names, resource names,
policy contents, or unbounded filters. Existing auth-denied events remain compatible.

### 5.8 Current Operator Evidence Boundary

The current release target is operator-facing governance and observability, not a SOC 2
certification package. The mandatory audit baseline for this issue is:

1. Every authenticated Console read outcome is represented by a typed AF/DataStorage
   event with safe attribution and bounded scope details.
2. A live DataStorage write is queryable by correlation/request ID.
3. Audit persistence failure is observable without silently changing the business response.
4. `apifrontend` is preserved as a bounded DataStorage metric category rather than being
   normalized to `other`.

Existing DataStorage integrity, retention, legal-hold, export, and transport controls are
inherited; #2584 does not re-certify them. Deployment-specific HMAC/backup evidence,
dedicated audit-reader access reviews, denied-read/unusual-scope alerting and review
cadence, and assessment-period operating-effectiveness samples are deferred until the
project explicitly elects to pursue SOC 2 certification.

---

## 6. TDD Execution Plan

### Phase 0 — Approval and Design Decision — **COMPLETE**

- Carry the approved threshold, named-capability/target-SAR, SignalProcessing RBAC, and
  typed-audit decisions into `DD-AF-018`; no additional contract approval is pending.
- Use the accepted `DD-AF-018` as the implementation authority; do not create a second
  design decision for the same boundary.
- Freeze OpenAPI paths, fields, nullable values, state/status mapping, capability names,
  internal traversal hints, page limits, and redaction rules.

### Phase 1 — RED: Tests First — **COMPLETE**

Write failing Ginkgo/Gomega tests before production implementation:

- config decode/validation and exact operator YAML fixtures;
- OpenAPI paths, schemas, state enum, and no-undocumented-fields contract;
- scope precedence/provenance and canonical resource identity;
- projection sanitization and current/history separation;
- cursor, completeness, and state mapping;
- capability, service-account, target-SAR, and no-leak denial behavior;
- production router ITs for every wiring point; and
- A2A/MCP regression characterization.

**First RED slice (must fail before broader projection implementation):**

1. Add typed Console audit event constants, payload-builder, OpenAPI-union, and generated
   client expectations; prove the current adapter skips the new events until registered.
2. Add `IT-AF-2584-012` for live DataStorage persistence, correlation/request retrieval,
   safe payload boundaries, and observable persistence failure.
3. Add `UT-DS-2584-001` proving `apifrontend` remains a recognized bounded metric category
   and does not normalize to `other`.

RED is incomplete until both UT and IT tests fail for the intended missing behavior.

### Phase 2 — GREEN: Minimal Wiring and Behavior — **COMPLETE**

Implement the smallest path that makes RED pass, in this order:

1. Extend the typed audit union/adapter and fix `apifrontend` category normalization.
2. Add typed config and additive route registration without modifying existing protocol handlers.
3. Add projection interfaces and construct real local plus FMC/ACM control-plane
   dependencies in `cmd/apifrontend`.
4. Add identity/service-account/capability checks, local target SARs, the existing remote
   `ScopeChecker` checks, and explicit local/exact fleet-control-plane selection.
5. Add structured scope resolution, typed sanitized response models, policy metadata, bounded
   history, cursor paging, state mapping, OpenAPI schemas, and audit wiring.
6. Run CHECKPOINT W immediately; a route that exists only in a unit test is not GREEN.

### Phase 3 — REFACTOR: Behavior-Preserving Quality Work — **COMPLETE**

Improve existing implementation only:

- centralize canonical identity/filter validation;
- deduplicate state/error serialization and sensitive-field allowlists;
- keep scope provenance in shared scope rather than copy/paste logic;
- make history bounds, timeouts, internal traversal hints, and page limits explicit; and
- fix error context, ignored errors, `any` payload escapes, context-in-struct fields,
  goroutine exits, deep nesting, and oversized function signatures.

Run `go build ./...`, `go test ./... -run '^$' -timeout=30s`, focused suites, and lint
after the refactor. Validation proves safety; it is not the refactor itself.

### Phase 4 — Wiring Verification — **COMPLETE (SCOPED AF EVIDENCE)**

Trace each operation from the real HTTPS router through middleware, authorization, reader,
projection, response, and audit exit. Every row in Section 7 requires a passing IT.

### Phase 5 — Cross-Repository Qualification — **DEFERRED/BLOCKED**

Publish the frozen OpenAPI, capability names, same-origin path, state map, and safe
response examples to Console #164. Console replaces its fixture-only seam and runs
browser, accessibility, and deployed qualification without adding a second transport.

---

## 7. Wiring Manifest

| Component | Production entry point | Wiring location | IT proof |
|---|---|---|---|
| `config.PolicyRead` | AF ConfigMap load | `pkg/apifrontend/config/config.go` and startup load | IT-AF-2584-001 |
| Policy scope handler | `GET /api/v1/console/policies/scope` | `pkg/apifrontend/handler/router.go` and handler construction | IT-AF-2584-001/003 |
| Inventory list handler | `GET /api/v1/console/inventory/managed-resources` | same router/handler construction | IT-AF-2584-002/007 |
| Inventory exact handler | `.../managed-resources/{resourceID}` | same router/handler construction | IT-AF-2584-005 |
| Local dynamic reader | explicit local request | `cmd/apifrontend/backend_deps.go` | IT-AF-2584-002 |
| Fleet inventory reader | explicit remote exact/get request | typed FMC/ACM control-plane adapter plus existing `ScopeChecker` | IT-AF-2584-008 / IT-FLEET-2584-001/002 |
| Structured scope resolver | inventory/policy projection | `pkg/shared/scope` adapter | IT-AF-2584-002 |
| Capability/target authorization | post-auth Console request | `pkg/apifrontend/auth` and handler | IT-AF-2584-004/005 |
| Historical CRD reader | policy scope request | typed client dependency construction | IT-AF-2584-003/006 |
| Audit attribution | successful/denied read | AF audit emitter/adapter | IT-AF-2584-010 |
| Typed audit persistence/retrieval | authenticated Console read outcome | AF adapter → DataStorage union/store/query | IT-AF-2584-012 |
| `apifrontend` metric category | DataStorage audit write | category normalization/lag metric | UT-DS-2584-001 |
| OpenAPI contract | client/server schema | `api/apifrontend/openapi/apifrontend-v1.yaml` | IT-AF-2584-011 |

**CHECKPOINT W**: any component with no production caller and no passing IT is an
implementation failure, not a deferred cleanup item.

---

## 8. File and Repository Change Plan

### 8.1 Kubernaut Repository

1. `pkg/apifrontend/config/config.go` and tests: add `PolicyRead` typed config and validation.
2. `pkg/apifrontend/auth` and tests: add named capability/target authorization without changing existing MCP/A2A checks.
3. `pkg/shared/scope`: add structured provenance while preserving current callers.
4. New `pkg/apifrontend/policyread`: source adapters, canonical IDs, projection service,
   history matcher, pagination, completeness, and unit tests.
5. `pkg/apifrontend/handler`: three GET operations, state/error serialization, and tests.
6. `pkg/apifrontend/handler/router.go`: additive route registration and validation.
7. `cmd/apifrontend/main.go` and `backend_deps.go`: dependency construction and RouterConfig wiring.
8. `pkg/apifrontend/audit`: approved read action mapping and safe payload tests.
9. `pkg/fleet/fmc` and `pkg/fleet/acm`: preserve the shared sanitized exact/detail
   control-plane contract and existing `ScopeChecker` behavior; remote list qualification
   remains gated.
10. `api/apifrontend/openapi/apifrontend-v1.yaml`: paths, schemas, security, errors, and examples.
11. Focused AF/fleet integration tests and generated contract artifacts required by the repository.

### 8.2 Operator Repository Follow-Up

- Keep PR #604's names as the baseline and extend its generated `policyRead` block with
  the required value derived from `spec.aiAnalysis.confidenceThreshold`.
- Make the operator field required for new v1 deployments and validate `(0.0, 1.0]`.
- Do not create an independent AF override, parse policy source, or invent a default.
- Add the named read capabilities to selected built-in personas/custom-role documentation.
- Add only AF ServiceAccount `get/list` on `signalprocessings`, with RBAC tests.
- Do not grant projection write access or create a second policy-reader ServiceAccount.

### 8.3 Console Repository Handoff

Provide `kubernaut-console#164` with the final route/schema, auth capability semantics,
same-origin proxy path, state-to-UI mapping, opaque-link rules, and safe response examples.
The Console must not store browser tokens or call internal services directly.

---

## 9. Security and Compliance Controls

| Control | Implementation proof |
|---|---|
| AC-4 information flow | Exact cluster/GVK/namespace/name filters, target SARs, no fallback, no broadening on invalid input |
| AC-6 least privilege | Named read capabilities, service-account separation, minimal AF reads, no projection write permission |
| AU-2/AU-3 | Read outcomes carry action, outcome, actor type/ID, request/correlation ID, and safe scope metadata |
| AU-9/AU-11 | Existing DataStorage integrity and retention path remains authoritative; no mutable local audit payload |
| SC-8 | Existing TLS/authenticated AF ingress and same-origin proxy route; no token exposure in browser |
| SI-10 | Strict filter/ID/schema validation and no arbitrary object/query payloads |
| SOC2 CC7.2 | Deterministic read attribution and completeness/state outcomes support investigation |
| Operator audit baseline | Typed read persistence/retrieval, safe payloads, failure observability, and bounded `apifrontend` metric category |
| SOC2 certification evidence | **Deferred**: deployment HMAC/backup proof, dedicated audit-reader reviews, denied-read/unusual-scope monitoring, review cadence, and assessment-period samples |

This feature does not claim that a policy read reconstructs the full remediation
lifecycle required by BR-AUDIT-005. It adds only the read attribution needed for the
Console governance view and preserves existing audit behavior.

The current release also does not claim SOC 2 certification. Certification-specific
operating-effectiveness evidence remains a separately authorized follow-up.

---

## 10. Risks and Alternatives

| Risk or alternative | Decision/mitigation |
|---|---|
| Direct browser-to-DataStorage or Kubernetes | Rejected: violates same-origin/BFF ownership and token boundary |
| AF parses `aianalysis-config` or Rego | Rejected: wrong owner, raw-policy exposure, custom-default ambiguity |
| Coarse `kubernaut.ai/console` only | Rejected: optional and not resource-scoped |
| AF ServiceAccount trust only | Rejected: user scopes collapse into one service identity |
| New historical index | Deferred: existing CRDs support bounded v1 matching; separate DD required for indexing |
| Partial records with warning | Rejected: partial/stale/unavailable responses must fail closed |
| Cross-cluster list without explicit ID | Rejected: attribution ambiguity and silent local-fallback risk |
| SignalProcessing history without RBAC | Approved minimal AF ServiceAccount `get/list`; no write verbs |

Residual risks after implementation are the lack of a dedicated historical index and the
fact that SignalProcessing may not persist every desired evaluation field. The response
must represent those limitations explicitly or fail closed; it must not synthesize data.

---

## 11. Validation and Completion Gates

Before implementation is declared complete:

1. `go build ./...` passes.
2. `golangci-lint run --timeout=5m` has no new findings.
3. Focused AF unit/integration suites pass; `make test` passes or unrelated baseline failures are recorded separately.
4. All critical scenarios in TP-2584 pass; unit scenario coverage is at least 90%.
5. Every Wiring Manifest row has a production caller and passing IT.
6. OpenAPI paths/schemas and generated artifacts are stable and reviewed.
7. No raw sensitive fields occur in success or error response fixtures.
8. Audit mapping contains required actor/action/outcome/request fields and safe details;
   live DataStorage retrieval, safe payload boundaries, and audit-pipeline failure
   observability are proven.
9. `apifrontend` is preserved as a bounded DataStorage metric category; operator contract
   tests pass for default/custom names, the required threshold, and minimal
   SignalProcessing read RBAC; no write permission is introduced.
10. Console #164 receives the frozen contract and completes its same-origin/deployed acceptance evidence.
11. SOC2 certification-specific evidence is not treated as a release blocker unless the
    project explicitly authorizes that certification scope.

### 11.1 Scoped Validation Record (2026-11-01)

**Update 2026-11-02:** ACM Search fixture results in the historical record below
do not qualify the inventory adapter. Production AF deliberately leaves ACM
Console inventory unwired; authorized remote inventory and policy-scope reads
return `unavailable`. The shared ACM scope checker remains wired. Current live
observations and the remaining proof are in `TEST_PLAN.md` §16.3. Focused
production-wiring tests, affected-package tests, build, compile-only tests,
lint, and `git diff --check` passed after the gate was added.

| Gate | Result | Evidence |
|---|---|---|
| Changed-package tests | **PASS** | `go test ./pkg/apifrontend/... ./cmd/apifrontend/... ./pkg/fleet/... ./pkg/shared/scope ./pkg/datastorage/server -count=1` |
| Full compile/build | **PASS** | `go test ./... -run '^$' -timeout=30s`; `go build ./...` |
| Static quality | **PASS** | `golangci-lint run --timeout=5m` — 0 issues; `git diff --check` |
| FMC/ACM inventory adapters | **PASS** | `IT-FLEET-2584-001/002` focused adapter suites |
| Typed DataStorage audit persistence | **PASS** | `IT-AF-2584-012` live write/query and failure-observability evidence |
| Remote AF focused integration | **PASS** | 2/2 focused specs passed with reusable image tag `af2584-it` |
| Fleet-only AF E2E | **BLOCKED** | The suite forcibly resets `AF_E2E_HOST_PORT_OFFSET` to `0`; unrelated `fullpipeline-e2e` owns host port `8088`. No active cluster was stopped or removed. |
| Console browser/deployed qualification | **DEFERRED** | Owned by `kubernaut-console#164` |
| Operator threshold/RBAC and live FMC/ACM deployment evidence | **DEFERRED** | Cross-repository follow-up; the AF contract and local/adapter tests are complete |

The standalone local AF E2E run exercised 231 specs: 220 passed and 11 failed. All
11 failures were Fleet-only AF specifications executed in the local lane, not the
Issue #2584 projection scenarios. This result is retained as environmental evidence,
not reported as a clean full-E2E pass. The Fleet-only lane was separately blocked
before test execution by the port collision above.

`make test` and Console browser acceptance were not claimed as complete in this scoped
branch validation. Existing DataStorage integrity/retention controls and SOC2
certification evidence remain outside this issue's release boundary.

### Confidence Assessment

**Plan confidence: 97%; implementation confidence: 96%.** The AF boundary, source ownership, current scope behavior,
operator PR #604, existing identity/SAR seams, production router construction points,
threshold source, authorization model, SignalProcessing RBAC direction, typed audit
extension, OpenAPI union schemas, generated client artifacts, and safe audit payload
fields are implemented and validated. Remaining risk is cross-repository qualification:
operator rollout/RBAC, live FMC/ACM deployment evidence, Console acceptance, and the
Fleet-only E2E port collision.

---

## 12. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-11-01 | Initial approval-gated implementation plan with contract, TDD phases, wiring manifest, controls, and cross-repository follow-up |
| 1.1 | 2026-11-01 | Threshold direction approved: required operator-sourced value; nullable/unknown fallback removed |
| 1.2 | 2026-11-01 | Named capabilities/target SARs, minimal SignalProcessing read RBAC, and typed audit events approved |
| 1.3 | 2026-11-01 | Operator release boundary set: first RED slice is typed audit persistence/retrieval plus `apifrontend` category normalization; broader SOC2 evidence deferred |
| 1.4 | 2026-11-01 | Implementation completed; scoped build/test/lint and adapter/audit evidence recorded; Fleet-only E2E port collision and cross-repository deferrals documented |
| 1.5 | 2026-11-04 | Local Console inventory containment finalized: metadata-only required client, paginated projection/discard, bounded working set, cancellation/concurrency limits, pre-write response guard, and no public page-freshness/count semantics |
| 1.6 | 2026-11-05 | Added the dedicated policy-read HTTPS response-body limiter: 16 MiB per decrypted Kubernetes response before client-go decoding, while retaining the 2 MiB serialized Console response guard and documenting the remaining process-RSS limitation |

## 13. Scope visibility clarification for review (2026-11-04)

The following user clarification applies to the #2584/#2593 read projection and
supersedes any contrary proposed behavior, not the historical test evidence:

- For a separately authorized, user-visible resource, an explicit resource
  `kubernaut.ai/managed=true` establishes managed scope regardless of namespace
  label or access. Explicit `false` likewise takes precedence and excludes it
  from managed scope. Other authorization and source-completeness gates remain.
- Namespace object visibility and resource visibility are independent. A
  resource's returned `metadata.namespace` may be displayed as part of that
  authorized resource without exposing the inaccessible Namespace object or
  fetching, inferring, or disclosing its hidden labels. Never augment results
  with inaccessible resources or namespace metadata.
- With an absent or invalid resource label and an inaccessible namespace,
  inheritance cannot be established; do not claim a known managed or unmanaged
  decision from the unavailable namespace label. The approved managed-only query
  behavior is a generic incomplete/partial state with no rows or total when any
  such user-visible resource is encountered. Do not guess or use privileged
  namespace enrichment. This decision is not yet implemented or qualified; existing
  fail-closed completeness gates are not relaxed.

Review impact: the bounded ACM prototype's visible-namespace requirement in
`TEST_PLAN.md` §16.3 is historical evidence, not the desired explicit-label
contract. The ACM gate stays closed pending qualification. Before implementation,
review the shared manager and local/fleet adapters, the authorization boundary,
and wire-format representation of unavailable namespace provenance; do not
conflate unavailable with a confirmed absent label. Apply the project's design
approval gate to any architectural or API contract changes.

Required future tests: exercise explicit true and false with a visible resource
and inaccessible namespace; absent/invalid labels with inaccessible namespace,
including generic partial responses with no rows or total even alongside known rows;
per-resource authorization and no namespace-label or hidden-resource leakage;
local list/exact and remote exact production dispatch; and existing partial/stale and
completeness protections. Console consumer tests should render only returned
authorized identity/provenance and must not infer namespace labels or scope.

## 14. Local inventory memory-containment addendum (2026-11-04)

The final local-read contract is narrower than the original generic inventory design:

- `cluster_id=local` list and exact reads use a required Kubernetes
  `metadata.Interface` and `PartialObjectMetadata`; missing metadata-client construction
  fails closed and never falls back to dynamic full-object listing.
- Metadata pages use Kubernetes `Limit`/`Continue` traversal with a centralized positive
  internal item-count hint for each backend request. The hint is an internal traversal
  chunk, not the Console page limit or an inventory total. Each item is projected
  immediately to the canonical sanitized identity/scope model and the page is discarded
  before the next request. Repeated/non-progressing or expired continuation and
  cancellation return state-only `unavailable` results.
- Local retained canonical projections are bounded to 32 MiB per operation. The local
  reader has a cancellable 15-second operation timeout and a process-local semaphore
  allowing two concurrent `List`/`Get` operations per API Frontend replica.
- The dedicated policy-read REST/metadata client preserves HTTPS/TLS and authentication while
  limiting each decrypted Kubernetes response body to 16 MiB before client-go decoding. This
  is a provisional client-side body bound, not a complete process RSS ceiling; transport,
  decoder, and runtime overhead remain outside it. `BackendPageLimit` remains an internal
  item-count hint only; the response-body limit, retained canonical working-set budget,
  operation cancellation, semaphore, and pre-write response guard are the enforceable
  containment safeguards.
- Successful serialized Console responses are checked before writing; responses over
  2 MiB become state-only `unavailable` responses rather than partial bodies.
- Console policy/inventory list operations are local-only for this release. ACM/FMC list
  operations remain unavailable; existing remote exact/get behavior is unchanged except
  for the common response safety guard.
- These are implementation safeguards, not public count/total/reload/snapshot/freshness
  or page-level `observed_at` semantics. The response has no total/count field and makes
  no cross-request snapshot or source-freshness claim. Historical evaluation timestamps
  remain source timestamps, not freshness markers.

Containment evidence is recorded by `UT-AF-2592-METADATA-001`,
`UT-AF-2592-PAGE-001/002`, `UT-AF-2592-KINDS-001`, `UT-AF-2592-CONTINUATION-001`,
`UT-AF-2592-CANCEL-001`, `UT-AF-2592-PROJECT-001`,
`UT-AF-2592-BUDGET-001`, `UT-AF-2592-SEMAPHORE-001`,
`IT-AF-2592-RESPONSE-001`, and
`IT-AF-2592-LOCAL-ROUTER-001`. The production dependency and route wiring remain
covered by the existing Checkpoint W tests; ACM/FMC list qualification remains closed.
