# Test Plan: AF Policy Scope and Managed-Resource Read Projection

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2584-v1.0
**Feature**: Versioned, read-only, authenticated API Frontend projections for policy scope, policy evaluation metadata, and managed-resource inventory
**Version**: 1.5
**Created**: 2026-11-01
**Author**: Kubernaut Team
**Status**: Implementation complete on branch; scoped AF validation passed; cross-repository qualification pending
**Branch**: `openchamber/issue-2584-console-policy-read`

> Production code and tests are present on this branch. Authorization, historical-source,
> and typed-audit decisions were approved before implementation. This record separates
> completed AF evidence from deferred Console/deployment qualification.
>
> **Console metadata decision (2026-11-03/04):** The user approved removing
> Console freshness objects and freshness-marker rejection. The wire contract
> exposes no count, total, reload, snapshot, freshness, or page-observation
> metadata; the Console owns render time. Historical evaluation timestamps remain
> source record timestamps. Explicit backend errors, identity, authorization, and
> detected incomplete results remain fail-closed. See DD-AF-018 and
> `IT-AF-2593-PUBLIC-METADATA-001/002/003`.

---

## 1. Introduction

### 1.1 Purpose

This plan verifies the upstream API Frontend contract required by the standalone
Console's `policies.scope` view. It proves that an authenticated interactive user can
obtain only a sanitized, versioned projection of the effective Kubernaut scope and
  managed-resource inventory through AF, without browser tokens, direct browser access to
  DataStorage, raw Rego, secrets, private reasoning, arbitrary payloads, or cross-scope
  records. It also proves that incomplete, unavailable, forbidden, and ambiguous
reads fail closed and that existing A2A/MCP and service-account paths remain unchanged.

### 1.2 Objectives

1. Prove the OpenAPI, configuration, pagination, completeness, and read-state
   read contract is deterministic and versioned.
2. Prove resource-label precedence, namespace inheritance, cluster-scoped no-inheritance,
   and missing/invalid-label safe defaults match BR-SCOPE-001 and ADR-053.
3. Prove verified user identity, named read capabilities, and target-scope authorization
   are enforced server-side; invalid filters and opaque IDs cannot widen access.
4. Prove current policy metadata is separated from historical evaluation metadata and that
   no raw Rego, ConfigMap data, token, secret, or private reasoning crosses the response boundary.
5. Prove the production router wiring and operator-provided `policyRead` references work
   without changing A2A/MCP behavior.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|---|---:|---|
| P0 scenario pass rate | 100% | Focused Ginkgo/Gomega suites |
| In-scope unit business scenarios | >=90%; critical scenarios 100% | BR/scenario matrix, not raw line percentage |
| AF wiring points | 100% | Wiring Manifest and production-path ITs |
| Applicable control objectives | 100% assessed | AC-4, AC-6, AU-2/AU-3, SC-8, SI-10, CC7.2 mapping |
| Existing A2A/MCP regressions | 0 | Existing AF suites plus focused regression IT |
| Raw sensitive data in projection | 0 occurrences | Structural response assertions and negative fixtures |
| Typed Console audit persistence | 100% of authenticated read outcomes | Live DataStorage write/query and failure-observability IT |
| `apifrontend` audit metric category | 100% recognized, never `other` | DataStorage normalization unit test |

---

## 2. References

### 2.1 Authority

- Issue [#2584](https://github.com/jordigilh/kubernaut/issues/2584): Expose scoped policy and inventory reads for the interactive console.
- [BR-SCOPE-001](../../requirements/BR-SCOPE-001-resource-scope-management.md): Resource Scope Management.
- [BR-SECURITY-016](../../requirements/BR-SECURITY-016-kubernetes-rbac-enforcement.md): Kubernetes RBAC Enforcement for REST API Endpoints.
- [BR-AI-085](../../requirements/BR-AI-085-rego-policy-input-schema.md): Rego Policy Input Schema for Approval Decisions.
- [BR-AI-088](../../requirements/BR-AI-088-configurable-confidence-thresholds.md): Operator-Configurable Confidence Thresholds.
- BR-AUDIT-005 v2.0: Core audit trace and attribution requirement, as defined by `AGENTS.md`.
- [ADR-053](../../architecture/decisions/ADR-053-resource-scope-management.md) and the shared `pkg/shared/scope` implementation.
- [DD-AUDIT-003](../../architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md) and [ADR-034](../../architecture/decisions/ADR-034-unified-audit-table-design.md).
- `docs/testing/TEST_PLAN_TEMPLATE.md`.

### 2.2 Cross-Repository References

- Console issue [#164](https://github.com/jordigilh/kubernaut-console/issues/164), which consumes this contract.
- Console foundation PR [#162](https://github.com/jordigilh/kubernaut-console/pull/162), which intentionally leaves this read path fixture-only.
- Console proposal [#855](https://github.com/jordigilh/kubernaut/issues/855).
- Operator PR [#604](https://github.com/jordigilh/kubernaut-operator/pull/604), merged as `b29f3c89058f0a96f9464788b6a99b2bc9581311`.

---

## 3. Risks and Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|---|---|---|---|---|---|
| R1 | REST route is reachable without the verified user or read capability | Unauthorized policy/inventory disclosure | Medium | IT-AF-2584-004, IT-AF-2584-005 | Run the real post-auth router and SAR path; reject missing identity and service-account callers on console routes |
| R2 | A namespace or resource filter widens the requested scope | Cross-tenant data disclosure | Medium | UT-AF-2584-005, IT-AF-2584-005 | Exact filter validation, target SARs, no offset pagination, and no fallback from invalid identity to broad list |
| R3 | Resource label, namespace label, and cluster scope are evaluated in the wrong order | Incorrect remediation boundary shown to operators | Medium | UT-AF-2584-003, UT-AF-2584-004, IT-AF-2584-002 | Reuse structured shared-scope resolution and assert provenance, not only the boolean result |
| R4 | Raw Rego, ConfigMap keys, secrets, arbitrary labels, or private reasoning leak through a projection | Security/compliance violation | Medium | UT-AF-2584-002, UT-AF-2584-009, IT-AF-2584-003 | Typed allowlist projection and negative response assertions |
| R5 | Current policy metadata is confused with historical evaluation metadata | Misleading governance/audit display | Medium | UT-AF-2584-006, IT-AF-2584-003 | Separate response fields and fixtures with different revisions/hashes |
| R6 | An incomplete or unavailable backend returns records or counts | False operational state and information leakage | High | UT-AF-2584-008, IT-AF-2584-006 | Error-state envelope contains no items, counts, or provenance; successful pages require complete source reads |
| R7 | Remote-cluster reads silently fall back to the hub cluster | Wrong-cluster data is presented as requested data | Medium | IT-AF-2584-008, IT-FLEET-2584-001/002 | Explicit cluster identity, FMC/ACM control-plane selection, existing `ScopeChecker`, and no local fallback |
| R8 | New router/config wiring changes A2A/MCP or robot behavior | Existing interactive/agent flows regress | Low | IT-AF-2584-009, existing AF suites | Additive route registration and regression coverage for existing protocol paths |

### 3.1 Risk-to-Test Traceability

R1, R2, R6, and R7 are critical security/data-integrity risks. Each has both unit
decision coverage and a production-path integration test. No implementation may be
declared GREEN while any of those ITs is absent or failing.

---

## 4. Scope

### 4.1 Features to be Tested

- `pkg/apifrontend/config`: typed `policyRead` configuration and validation.
- `pkg/apifrontend/handler`: authenticated AF REST handlers and router registration.
- `pkg/apifrontend/auth`: named read-capability and target-scope authorization.
- New policy-read projection package: policy ConfigMap metadata, scope provenance,
  historical evaluation matching, inventory identity, pagination, and state mapping.
- `pkg/shared/scope`: structured scope resolution preserving existing `IsManagedResource` behavior.
- `cmd/apifrontend`: production dependency and router wiring.
- `api/apifrontend/openapi/apifrontend-v1.yaml`: schemas, paths, error states, and security contract.
- AF audit emission and DataStorage adapter mapping for attributable read attempts.
- Operator configuration compatibility with merged PR #604's `policyRead` block.

### 4.2 Features Not to be Tested

- Policy editing, policy testing, policy deployment, or policy evaluation changes.
- Raw Rego or ConfigMap content retrieval.
- Direct browser access to DataStorage, Kubernetes, or internal service endpoints.
- New A2A/MCP tools, changed agent behavior, workflow execution, approval mutation, or RAR decisions.
- Console UI implementation tracked by `kubernaut-console#164`; only its post-contract E2E handoff is specified here.
- Arbitrary Kubernetes object payloads, Secret contents, token contents, annotations, or private reasoning.
- A new historical index. History matching remains bounded by existing AIAnalysis and SignalProcessing CRD fields.

### 4.3 Contract Decisions Requiring Approval

| Decision | Recommended contract | Alternative | Approval effect |
|---|---|---|---|
| Boundary | Authenticated AF REST under `/api/v1/console/...` | Separate user API/BFF or direct browser API calls | REST is required for the planned tests; direct browser-to-DS is rejected |
| Threshold | Every new deployment must provide the AIAnalysis approval threshold; the operator injects the validated value into AF `policyRead` for read-only display | Parse `aianalysis-config` or infer a custom Rego default | The configured value is sourced from existing `spec.aiAnalysis.confidenceThreshold`; missing/invalid values fail deployment/readiness |
| Authorization | Named read capabilities plus user/target SARs; optional coarse `kubernaut.ai/console` gate remains additive | Coarse console gate only | **Approved**; coarse-only is rejected as insufficiently granular and optional by current config |
| Service accounts | Console REST accepts verified interactive users only; A2A/MCP service accounts remain unchanged | Permit service accounts on the console route | **Approved**; any exception requires a separate machine-read contract and tests |
| Fleet inventory source | Local Kubernetes clients for `local`; non-local list operations unavailable in this release; existing typed FMC/ACM exact/get and `ScopeChecker` behavior remains unchanged | AF Console path reads remote Kubernetes/MCP directly or enables an unqualified remote list | **Approved**; AF must not create a second remote-cluster access path or delegate browser tokens |

The threshold direction is approved: v1 does not expose a nullable `unknown` threshold.
The operator's existing `spec.aiAnalysis.confidenceThreshold` is the single source of
truth, and the AF `policyRead` value is derived from it. Missing or invalid values fail
operator validation/reconciliation and AF startup/readiness rather than silently using a
Rego default. The API describes this as the configured/supplied approval threshold, not
the universal effective threshold of arbitrary custom Rego. The implementation phase
also uses the approved named capabilities, local target SARs, existing FMC/ACM managed-
scope checks for remote targets, and the approved minimal SignalProcessing `get/list`
RBAC. This preserves least privilege and gives the Console a server-side user/scope
boundary rather than treating a browser session as a permission. AF config reload
validates changed `policyRead` content for drift detection but applies it only after
restart; no live projection dependency is mutated by the watcher.

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=90% of defined in-scope business scenarios; all critical safety,
  authorization, privacy, and data-integrity scenarios covered.
- **Integration**: 100% of Wiring Manifest rows and all applicable controls assessed.
- **E2E**: Console/deployed qualification is a cross-repository dependency. It must prove
  same-origin proxy access, browser-token absence, exact navigation, and fail-closed state mapping.
- Raw line coverage remains diagnostic. It does not replace requirements-based scenario coverage.

The current release proves the operator-facing audit baseline only: typed read-event
persistence/retrieval, safe attribution, failure observability, and bounded metric
classification. Existing DataStorage integrity, retention, legal-hold, export, and
transport controls are inherited rather than re-certified here. Deployment-specific
SOC2 evidence—HMAC/backup proof, dedicated audit-reader reviews, denied-read/unusual-scope
alerts, review cadence, and assessment-period samples—is deferred unless certification
scope is explicitly approved.

### 5.2 Two-Tier Minimum

Every in-scope BR has UT plus IT coverage. E2E is required for the cross-repository
Console acceptance path and is not a substitute for AF wiring tests.

### 5.3 Test Method

- Ginkgo/Gomega BDD for all new business tests.
- Unit tests may use typed fakes for external Kubernetes, SAR, fleet, and audit dependencies.
- Integration tests use `envtest`/real Kubernetes API behavior, `httptest` HTTP servers,
  the production router, and real middleware composition; no mocked production dispatch path.
- Test fixtures contain deliberately different current policy revisions, historical hashes,
  labels, namespaces, cluster identities, and forbidden records.
- Every response assertion checks both positive fields and prohibited fields.
- Local fixtures use the hub Kubernetes API; fleet fixtures exercise the typed FMC/ACM
  control-plane projection and the existing AF `ScopeChecker`. No Console integration
  test uses a direct remote MCP/Kubernetes reader or browser token.

### 5.4 Read-State Contract

The shared state vocabulary exposes these eight logical states:

1. `loading` — Console adapter state before a request; never emitted by AF.
2. `success` — complete non-empty response, HTTP 200.
3. `empty` — complete response with zero items, HTTP 200, no `total` field.
4. `partial` — one or more required reads/joins/traversals incomplete, HTTP 503, no data.
5. `stale` — reserved shared state for qualified backends; the local release does not
   derive this state from a freshness marker or expose freshness metadata, HTTP 503, no data.
6. `unavailable` — required backend/configuration cannot be read, HTTP 503, no data.
7. `forbidden` — authenticated caller lacks the endpoint or requested scope permission,
   HTTP 403, no records, counts, or provenance.
8. `not_found` — exact requested identity does not exist, HTTP 404, no related metadata.

Invalid filters are HTTP 400 RFC 7807 errors and are not converted to an empty result.
For `partial`, `stale`, `unavailable`, `forbidden`, and `not_found`, the response body
contains only the state, status, request ID, and a safe generic message.

### 5.5 Pass/Fail Criteria

**PASS** requires:

1. All P0 scenarios pass.
2. All wiring rows have passing IT evidence.
3. No response in a denied/error state contains an item, count, source provenance, raw
   policy, secret, token, or private reasoning.
4. Existing affected AF tests pass, `go build ./...` passes, and no new lint findings exist.
5. The approved cross-repository Console acceptance scenarios pass before release qualification.

**FAIL** is any P0 failure, any cross-scope or sensitive-data leak, any partial/stale
response containing records, any unapproved contract deviation, or any A2A/MCP regression.

### 5.6 Suspension and Resumption

Suspend RED/GREEN when the operator contract is unavailable, required CRDs cannot be
installed in envtest, the build is broken, or more than one failure class indicates a
shared infrastructure cause. Resume when the dependency is merged/available or the
infrastructure/root cause is fixed and the affected focused suite is rerun.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File/package | Functions/behavior |
|---|---|
| `pkg/apifrontend/config` | `PolicyRead` YAML decode, required names, required threshold/range validation |
| New policy-read package | canonical identity, label precedence, projection allowlist, history join, cursor, state mapping |
| `pkg/apifrontend/auth` | capability names, user/service-account distinction, target SAR request construction |
| `pkg/shared/scope` | structured provenance resolution and compatibility wrapper |
| `pkg/fleet/fmc`, `pkg/fleet/acm` | canonical sanitized inventory page/detail adapters and fail-closed control-plane errors |
| `pkg/apifrontend/audit` | read action/outcome attribution and safe detail filtering |

### 6.2 Integration-Testable Code

| File/package | Wiring |
|---|---|
| `pkg/apifrontend/handler` | production route, auth middleware, handler, response/error serialization |
| `pkg/apifrontend/handler/router.go` | additive `/api/v1/console/...` route registration |
| `cmd/apifrontend/main.go` | handler construction and `RouterConfig` wiring |
| `cmd/apifrontend/backend_deps.go` | local typed/dynamic readers, FMC/ACM inventory adapters, scope resolver, SAR dependencies |
| `pkg/fleet/fmc`, `pkg/fleet/acm` | control-plane inventory endpoints/query transport and typed response mapping |
| `api/apifrontend/openapi/apifrontend-v1.yaml` | schema and operation contract validation |
| Operator contract fixture | merged `policyRead` names and explicit custom-name behavior |

### 6.3 Version Identification

| Item | Version/commit | Notes |
|---|---|---|
| Kubernaut | `origin/main` at `16bb28c6e` before implementation | Clean preflight baseline |
| Operator | PR #604 merge `b29f3c89058f0a96f9464788b6a99b2bc9581311` | Names-only `policyRead` contract |
| Console | PR #162 merged; issue #164 open | Consumer remains blocked on this contract |

---

## 7. BR and Control Coverage Matrix

| Requirement/control | Business outcome | Unit | Integration | E2E/qualification |
|---|---|---|---|---|
| BR-SCOPE-001 | Correct opt-in scope and provenance | UT-AF-2584-003/004 | IT-AF-2584-002/005/006 | E2E-CONSOLE-2584-002 |
| BR-SECURITY-016 | Authenticated, authorized REST reads only | UT-AF-2584-010 | IT-AF-2584-004/005 | E2E-CONSOLE-2584-003 |
| BR-AI-085 | Policy decision attribution is typed and sanitized | UT-AF-2584-002/006/009 | IT-AF-2584-003 | E2E-CONSOLE-2584-002 |
| BR-AI-088 | The configured approval threshold is required and consistent across operator, AIAnalysis, AF, and Console | UT-AF-2584-001/006 | IT-AF-2584-003 | E2E-CONSOLE-2584-002 |
| BR-AUDIT-005 | Read attribution is reconstructable without payload leakage | UT-AF-2584-011 | IT-AF-2584-010 | Deployment audit check |
| FedRAMP AC-4/AC-6 | Filter and target scope are enforced | UT-AF-2584-005/010 | IT-AF-2584-004/005 | E2E-CONSOLE-2584-003 |
| FedRAMP AU-2/AU-3, SOC2 CC7.2 | Auth/read outcomes carry actor, action, outcome, correlation/request ID | UT-AF-2584-011 | IT-AF-2584-010 | Deployment audit check |
| FedRAMP SC-8 | Same-origin/TLS route does not create a browser bearer-token path | — | IT-AF-2584-001/009 | E2E-CONSOLE-2584-001 |
| FedRAMP SI-10 | Invalid filters and IDs are rejected without widening scope | UT-AF-2584-005 | IT-AF-2584-005 | E2E-CONSOLE-2584-002 |
| Operator audit baseline | Typed read events are safe, persisted, queryable, failure-observable, and classified as `apifrontend` | UT-AF-2584-011, UT-DS-2584-001 | IT-AF-2584-012 | Deployment audit check |

---

## 8. Test Scenarios

### 8.1 Unit Tests

| ID | Business outcome | Phase |
|---|---|---|
| `UT-AF-2584-001` | Required operator threshold is represented exactly; missing, malformed, or out-of-range values fail closed and are never inferred from raw policy | **PASS** — config and finite-value regression tests |
| `UT-AF-2584-002` | Current policy identity/revision is projected without Rego source or arbitrary ConfigMap data | **PASS** — sanitized policy projection tests |
| `UT-AF-2584-003` | Resource label overrides namespace label and reports resource provenance | **PASS** — structured scope provenance tests |
| `UT-AF-2584-004` | Cluster-scoped resources do not inherit namespace scope; invalid labels are treated as absent, so namespaced resources may inherit a valid namespace label; no valid labels remain unmanaged | **PASS** — scope and local inventory tests |
| `UT-AF-2584-005` | Canonical cluster/GVK/namespace/name filters and opaque IDs reject malformed or widening input | **PASS** — inventory contract and handler tests |
| `UT-AF-2584-006` | Historical hash/rules/decision remain separate from current policy revision/threshold; bounded matching is deterministic | **PASS** — policy/history projection tests |
| `UT-AF-2584-007` | Cursor pagination is stable, bounded, and never emits a total count | **PASS** — canonical inventory pagination tests |
| `UT-AF-2584-008` | Complete, empty, partial, stale, unavailable, forbidden, and not-found states map deterministically | **PASS** — shared state contract, stale/partial audit, and fail-closed mapping tests |
| `UT-AF-2584-009` | Projection allowlist excludes secrets, tokens, private reasoning, raw policy, and arbitrary payloads | **PASS** — response serialization and adapter allowlist tests |
| `UT-AF-2584-010` | Missing identity, service-account identity, missing capability, and denied target SAR fail closed | **PASS** — authorization and handler denial tests |
| `UT-AF-2584-011` | Typed audit details contain actor/action/outcome/request scope but no returned payload or secret | **PASS** — typed audit adapter tests |
| `UT-DS-2584-001` | DataStorage preserves `apifrontend` as a bounded metric category instead of normalizing it to `other` | **PASS** — DataStorage metric-category test |
| `UT-AF-2592-METADATA-001` | Local inventory construction requires the metadata-only client and never falls back to full-object listing | **PASS** |
| `UT-AF-2592-PAGE-001` | Local inventory traverses Kubernetes metadata pages with `Limit`/`Continue` before canonical sorting and paging | **PASS** |
| `UT-AF-2592-PAGE-002` | Local metadata traversal defaults to a positive internal item-count hint independent of the Console page limit | **PASS** |
| `UT-AF-2592-KINDS-001` | Default local inventory excludes Secrets and ConfigMaps while explicit policy-source ConfigMap reads remain separate | **PASS** |
| `UT-AF-2592-CONTINUATION-001` | Repeated or expired continuation fails closed with no retained rows or cursor | **PASS** |
| `UT-AF-2592-CANCEL-001` | Cancellation between local backend pages returns a state-only failure | **PASS** |
| `UT-AF-2592-PROJECT-001` | Only canonical projections survive each metadata-page discard; labels and backend fields do not leak | **PASS** |
| `UT-AF-2592-BUDGET-001` | Retained canonical projections cannot exceed the local working-set budget | **PASS** |
| `UT-AF-2592-SEMAPHORE-001` | Cancellable per-reader concurrency bounds are shared by local `List` and `Get` | **PASS** |
| `UT-AF-2584-HTTPS-BODY-001` | The dedicated policy-read HTTPS transport stops an oversized decrypted Kubernetes response before unbounded client-go decoding | **PASS** |

### 8.2 Integration Tests

| ID | Business outcome | Phase |
|---|---|---|
| `IT-AF-2584-001` | A real authenticated request reaches the production router and returns a complete local projection using operator `policyRead` names | **PASS** — production wiring and route-tier tests; deployed qualification remains separate |
| `IT-AF-2584-002` | A real envtest inventory read reports exact resource identity, effective scope, precedence, provenance, and links | **PASS** — local inventory and dependency-wiring tests |
| `IT-AF-2584-003` | A real policy read joins ConfigMap metadata with bounded AIAnalysis/SignalProcessing history while keeping current/history fields separate | **PASS** — typed source and projection tests |
| `IT-AF-2584-004` | Unauthorized capability or target-scope SAR returns forbidden with no records, counts, provenance, or existence detail | **PASS** — authenticated handler denial tests |
| `IT-AF-2584-005` | Invalid filters, cross-cluster IDs, cross-namespace IDs, and guessed direct IDs cannot widen the result | **PASS** — filter, opaque-ID, and target-denial tests |
| `IT-AF-2584-006` | Missing config, ConfigMap permission failure, history permission failure, unavailable source, and incomplete join return state-only failure responses | **PASS** — fail-closed source/state tests; live deployment qualification remains separate |
| `IT-AF-2584-007` | Cursor page 1 and page 2 are deterministic; generated links preserve exact cluster/resource scope | **PASS** — bounded inventory/link tests |
| `IT-AF-2584-008` | Explicit remote exact reads use the FMC/ACM fleet control-plane projection and existing `ScopeChecker`; remote failure never falls back to local Kubernetes data, while remote lists remain unavailable | **PASS** — focused remote AF 2/2 and FMC/ACM adapter suites |
| `IT-AF-2584-009` | Existing A2A/MCP and verified service-account behavior remain unchanged after route/config wiring | **PASS** — broad affected AF package regression suite |
| `IT-AF-2584-010` | Every authenticated read outcome carries correct actor type/ID, action, outcome/state, request ID, and correlation through the typed DataStorage audit union | **PASS** — live typed audit persistence/retrieval evidence |
| `IT-AF-2584-011` | OpenAPI validation accepts every success/error schema and rejects undocumented fields or unversioned paths | **PASS** — OpenAPI Generator validation; unused-model warnings only |
| `IT-AF-2584-012` | **First RED gate:** a live DataStorage audit write is queryable by correlation/request ID, preserves required actor/action/outcome fields, excludes sensitive payloads, and makes persistence failure observable | **PASS** — live DataStorage integration evidence |
| `IT-FLEET-2584-001` | FMC's typed inventory projection returns only canonical managed-resource identity/scope fields, preserves cursor/state metadata, and rejects unsupported or widening filters | **PASS** — focused FMC adapter suite |
| `IT-FLEET-2584-002` | ACM's typed inventory projection matches the FMC canonical model and returns state-only failure on query/auth/backend errors | **NOT QUALIFIED** — fixture suite passed, but live ACM spike exposed incompatible filters, label/ID representation, and scope behavior; see §16.3 |
| `IT-AF-2592-LOCAL-ROUTER-001` | The production AF router serves the local metadata projection without exposing page observation or other count/reload/freshness/snapshot metadata | **PASS** |
| `IT-AF-2592-RESPONSE-001` | An oversized serialized Console projection becomes state-only unavailable before any success body is written | **PASS** |
| `IT-AF-2584-AUDIT-001/002` | Malformed authenticated requests and oversized serialized responses each produce exactly one final-outcome audit event | **PASS** |

### 8.3 Cross-Repository E2E Qualification

| ID | Business outcome | Owner | Phase |
|---|---|---|---|
| `E2E-CONSOLE-2584-001` | Console calls same-origin AF through OAuth2 Proxy with no browser bearer token and no direct DataStorage request | Console #164 | Deferred until AF contract lands |
| `E2E-CONSOLE-2584-002` | `policies.scope` navigates to the exact managed-resource identity and maps all eight read states honestly | Console #164 | Deferred until AF contract lands |
| `E2E-CONSOLE-2584-003` | Deployed user/group RBAC permits only approved read capabilities and blocks cross-scope access | Console/operator qualification | Deferred until auth decision lands |

### 8.4 Tier Skip Rationale

No AF tier is skipped. Console browser/deployed E2E is deferred only because it belongs
to `kubernaut-console#164` and requires this approved contract; AF unit and integration
tests must pass independently first.

---

## 9. Detailed P0 Test Cases

### UT-AF-2584-003: Scope precedence and provenance

**BR**: BR-SCOPE-001
**Priority**: P0
**Type**: Unit
**File**: New policy-read/scope Ginkgo spec

**Given** a namespaced Deployment has `kubernaut.ai/managed=false` and its namespace
has `kubernaut.ai/managed=true`.
**When** the scope resolver evaluates the exact identity.
**Then** it returns `included=false`, `source=resource`, resource label `false`, and
does not inherit the namespace label. Re-run with resource label absent and assert
`included=true`, `source=namespace`. Re-run with both absent and assert unmanaged.

**Acceptance criteria**:

- Resource label always wins when valid.
- Namespace fallback is used only for namespaced resources without a valid resource label.
- The response contains provenance only after the exact identity is authorized.

### IT-AF-2584-004: Authorization and no-leak denial

**BR/control**: BR-SECURITY-016, AC-4, AC-6, SI-10
**Priority**: P0
**Type**: Integration
**File**: AF handler/router integration spec

**Given** a real router, verified user identity, one allowed resource, one denied
resource, and SAR reactions for capability and target checks.
**When** the caller requests the denied resource, a cross-namespace filter, and a
guessed opaque ID.
**Then** the server returns the documented forbidden/not-found state without `items`,
`total`, `nextCursor`, resource labels, policy names, or provenance. Existing allowed
reads remain available to the same user.

### IT-AF-2584-006: Fail-closed source states

**BR/control**: BR-SCOPE-001, BR-AI-085, BR-SECURITY-016, SI-10
**Priority**: P0
**Type**: Integration
**File**: AF handler/backend integration spec

Exercise missing `policyRead`, missing policy ConfigMap, forbidden history source,
backend timeout, unavailable cached source, and incomplete history join independently.
Each case must return only the corresponding state envelope and safe request ID. No
case may return a partial `200`, partial records, a count, a source reference, or a
fallback result from another cluster/source.

### IT-AF-2584-009: Existing protocol compatibility

**BR/control**: BR-INTERACTIVE-010, BR-SECURITY-016
**Priority**: P0
**Type**: Integration
**File**: Existing AF router/protocol wiring suites plus focused regression spec

**Given** the new console routes are registered and a verified service-account token is
used for existing A2A/MCP calls.
**When** the caller invokes the existing protocol endpoints and separately calls the
console read route.
**Then** A2A/MCP behavior and service-account attribution remain unchanged, while the
console route rejects the service account unless a separately approved machine-read
contract exists.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- Ginkgo v2/Gomega.
- Typed fakes for ConfigMap, CRD, dynamic resource, fleet, SAR, and audit dependencies.
- No `testing.T` business-logic tests; native fuzz/benchmark exceptions do not apply.

### 10.2 Integration Tests

- Ginkgo/Gomega with envtest CRDs for AIAnalysis, SignalProcessing, and representative
  Kubernetes inventory kinds.
- `httptest` server and the real AF router, auth middleware, projection handler, and
  OpenAPI response serialization.
- Real envtest API reads for wiring; no mocked production dispatch path.
- Separate local Kubernetes and FMC/ACM control-plane fixtures; no implicit cluster
  fallback and no direct remote MCP reader in the Console path.

### 10.3 Cross-Repository Qualification

- Console standalone test harness with OAuth2 Proxy session and browser network inspection.
- Deployed Kubernetes/Operator environment with user groups, SAR/RBAC bindings, policy
  ConfigMaps, managed/unmanaged labels, and a configured FMC or ACM control plane for
  remote exact/get and scope-check qualification. Remote Console list qualification is
  deferred; the Console path must not require direct remote MCP access.

---

## 11. Dependencies and Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact | Workaround |
|---|---|---|---|---|
| Threshold contract | Operator follow-up | Direction approved; operator rollout pending | Operator/AF must share one required configured value | Derive AF `policyRead` from `spec.aiAnalysis.confidenceThreshold`; do not infer Rego defaults |
| Authorization contract | Design/security | Approved in DD-AF-018 | Named capabilities plus local target SARs and existing FMC/ACM managed-scope checks for remote targets | Implement the approved split; do not delegate user tokens to remote Kubernetes |
| Operator PR #604 | Cross-repo code | Merged | Supplies policy names | Pin contract to merge SHA |
| SignalProcessing history RBAC | Cross-repo security | AF reader complete; operator RBAC follow-up pending | Required for complete history projection in deployment | Add only AF ServiceAccount `get/list` on `signalprocessings`; no write verbs |
| FMC/ACM inventory projection | Cross-repo architecture/implementation | AF adapters complete; live control-plane deployment pending | Required for remote Console exact/get and scope-check qualification; list remains unavailable | Preserve the shared sanitized contract and adapter ITs; no direct remote Kubernetes/MCP reader |
| Console #164 | Cross-repo consumer | Open/blocked | Blocks browser/deployed acceptance | Run AF focused suites first |

### 11.2 Execution Order

1. **Approval/design**: record the approved authorization/history/audit and FMC/ACM
   inventory-boundary decisions in DD-AF-018.
2. **RED**: write contract, scope, privacy, authorization, state, and route wiring tests first.
3. **GREEN**: wire config, local/control-plane readers, auth, routes, and minimal projection until UT and IT pass.
4. **REFACTOR**: consolidate scope provenance, sanitization, pagination, error mapping, and audit details.
5. **WIRING VERIFICATION**: execute every manifest row through the production router.
6. **Cross-repository qualification**: hand the frozen OpenAPI/auth/state contract to Console #164.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|---|---|---|
| Test plan | `docs/tests/2584/TEST_PLAN.md` | Approved test strategy and traceability |
| Implementation plan | `docs/tests/2584/IMPLEMENTATION_PLAN.md` | Approved TDD and wiring plan |
| AF unit suites | `pkg/apifrontend/{policyread,auth,scope}/` | Business and security decision tests |
| AF integration suites | `pkg/apifrontend/handler/`, `cmd/apifrontend/` or `test/integration/apifrontend/` | Real production-path wiring tests |
| Fleet control-plane suites | `pkg/fleet/fmc/`, `pkg/fleet/acm/` | Sanitized exact/detail transport and fail-closed behavior; Console list remains gated |
| OpenAPI contract | `api/apifrontend/openapi/apifrontend-v1.yaml` | Versioned route/schema contract |
| Coverage/control matrix | CI/PR evidence and this plan | Requirements-based proof and control assessment |
| Console handoff | Issue #164 / linked contract artifact | Same-origin consumer and deployed acceptance inputs |

---

## 13. Execution Commands

```bash
# Fast compile gate
go build ./...

# Focused AF unit/integration suites after implementation
go test ./pkg/apifrontend/... ./cmd/apifrontend/... -ginkgo.v -count=1

# Full repository unit/integration gate
make test

# Static quality gates
golangci-lint run --timeout=5m
git diff --check
```

The exact focused package command may be adjusted to the final package location, but
must remain a production-path Ginkgo/Gomega invocation and must not replace ITs with
unit-only handler calls.

---

## 14. Wiring Verification

| Code path | Production entry point | Exit point | Wiring IT | Status |
|---|---|---|---|---|
| Typed `policyRead` config | AF ConfigMap load in `cmd/apifrontend` | Validated runtime config | IT-AF-2584-001 | **PASS** |
| Policy scope handler | `GET /api/v1/console/policies/scope` | Sanitized 200/state response | IT-AF-2584-001/003 | **PASS** |
| Managed inventory list | `GET /api/v1/console/inventory/managed-resources` | Paginated identity projection | IT-AF-2584-002/007 | **PASS** |
| Managed inventory exact read | `GET /api/v1/console/inventory/managed-resources/{id}` | Exact authorized identity | IT-AF-2584-005 | **PASS** |
| User/capability authorization | Post-auth console route | 200 or state-only denial | IT-AF-2584-004/005 | **PASS** |
| Scope provenance | Shared resolver → projection | Effective source/inclusion | IT-AF-2584-002 | **PASS** |
| Historical metadata | CRD readers → projection | Separate sanitized history | IT-AF-2584-003/006 | **PASS** |
| Fleet inventory projection | FMC/ACM control plane → AF adapter | Canonical remote identity/scope page | IT-FLEET-2584-001/002, IT-AF-2584-008 | **PASS** |
| Audit attribution | Handler → AF audit emitter → adapter | Actor/action/outcome event | IT-AF-2584-010 | **PASS** |
| Audit persistence/retrieval | AF adapter → DataStorage audit store → query | Searchable typed event and observable failure | IT-AF-2584-012 | **PASS** |
| `apifrontend` metric category | DataStorage audit handler | Bounded category/lag metric | UT-DS-2584-001 | **PASS** |
| OpenAPI contract | YAML paths/components | Validated wire shape | IT-AF-2584-011 | **PASS** |

Unit tests do not count as wiring proof.

---

## 15. Implemented Test Additions

| Location | Evidence added | Reason |
|---|---|---|---|
| `pkg/apifrontend/handler/router*_test.go` | Additive Console route assertions retain existing paths and middleware tiers | Prove no A2A/MCP route replacement |
| `pkg/apifrontend/config/*_test.go` | Valid, malformed, missing-threshold, and operator-rendered `policyRead` fixtures | Validate the clean-break contract and fail-closed reads |
| `cmd/apifrontend/main_wiring_test.go` | Local readers, FMC/ACM adapters, existing `ScopeChecker`, and auth dependencies are wired without `FleetReaderFactory` | CHECKPOINT W |
| `pkg/apifrontend/audit/*_test.go` | Typed read actions, state-aware outcomes, safe payloads, and DataStorage union mapping | AU-2/AU-3 attribution |
| `pkg/datastorage/server/audit_events_handler_test.go` | `UT-DS-2584-001` preserves the bounded `apifrontend` metric category | Operator audit observability |
| OpenAPI validation | Console paths, states, security, and closed response schemas validate; only unused-model recommendations remain | Versioned Console handoff |

---

## 16. Validation Evidence

### 16.1 Completed AF Gates

| Gate | Result | Evidence |
|---|---|---|
| Changed-package tests | **PASS** | `go test ./pkg/apifrontend/... ./cmd/apifrontend/... ./pkg/fleet/... ./pkg/shared/scope ./pkg/datastorage/server -count=1` |
| Full compile/build | **PASS** | `go test ./... -run '^$' -timeout=30s`; `go build ./...` |
| Static quality | **PASS** | `golangci-lint run --timeout=5m` — 0 issues; `git diff --check` |
| Remote adapter qualification | **PASS** | FMC/ACM focused suites and AF remote focused integration, 2/2 specs |
| Typed DataStorage audit | **PASS** | Live write/query and failure-observability evidence for `IT-AF-2584-012` |

### 16.2 Explicitly Deferred or Blocked Evidence

- Fleet-only AF E2E could not start: its suite resets `AF_E2E_HOST_PORT_OFFSET` to `0`,
  while unrelated `fullpipeline-e2e` owns host port `8088`. The active unrelated cluster
  was not stopped or removed. Both the default and offset attempts recorded the same
  `listen tcp4 :8088: bind: address already in use` failure.
- The standalone local AF E2E run exercised 231 specs: 220 passed and 11 Fleet-only
  specifications failed because they ran in the local lane. Those failures are not
  counted as Issue #2584 projection failures or as a clean full-E2E pass.
- Console browser/deployed qualification remains owned by `kubernaut-console#164`.
- `make test`, operator threshold/RBAC deployment proof, live FMC/ACM deployment proof,
  and SOC2 certification evidence are outside this scoped branch validation.

### 16.3 Live ACM 2.17.3 Spike and Regression Plan

On the user-approved `kubernaut492-sno-b` SNO cluster, a live ACM Search query
against three ConfigMaps and their managed namespace established these results:

- `label=kubernaut.ai/managed=true` returned the explicitly managed ConfigMap;
  the current adapter's `label.kubernaut.ai/managed=true` property returned none.
- Items represent labels as a semicolon-separated `label` string, and their `_uid`
  includes a slash. The adapter's current label keys and opaque-ID validation do
  not match that observed representation.
- A direct managed-label filter did not return an unlabelled ConfigMap in a
  managed namespace. Namespace inheritance therefore requires a separate scoped
  join; an explicit resource `false` must override namespace `true`.
- A query with `limit=1` returned five as `count` and one item, contrary to the
  vendored schema's warning that count equals item length when both are requested.
  Count semantics and supported pagination must be verified per backend version;
  no unproven count assumption may mark a truncated traversal complete.
- The adapter now returns a state-only `partial` page when `count` exceeds the
  number of returned items, and rejects contradictory smaller counts; exact reads
  do not map a detected partial page to not-found. `UT-ACM-2584-LIVE-003/007/008`
  and `IT-FLEET-2584-LIVE-003` prove those safeguards. Matching counts alone do
  not prove complete visibility or freshness on all supported ACM versions.
- A live unprivileged Search request for a known existing ConfigMap returned
  `count: 0`, empty items, and no messages. The same identity did not regain
  visibility during a short post-binding probe, so that probe is not evidence of
  successful privileged access; earlier privileged queries established the item
  exists. Search returned `searchSchema.allProperties` without a synchronization
  watermark. Query time is not evidence of the index's source freshness.
- The existing ACM `ScopeChecker` query pattern matched the explicitly unmanaged
  ConfigMap because it tests resource identity/existence without a managed-label
  filter. It cannot independently establish effective managed scope as currently
  implemented. The query pattern was tested live; the Go client itself was not
  exercised in this spike.
- An opaque `resource_id` filter returned no item for a synthetic canonical ID;
  that alone does not prove how ACM handles every unsupported property or ID.

**Required RED scenarios before correcting the adapter:**

1. `UT-ACM-2584-LIVE-001` / `IT-FLEET-2584-LIVE-001`: real Search-shaped flat
   labels, explicit true/false, invalid/absent labels, namespace inheritance, and
   cluster-scoped no-inheritance; no raw label string in the projection.
2. `UT-ACM-2584-LIVE-002` / `IT-FLEET-2584-LIVE-002`: explicit cluster, stable
   opaque exact IDs, exact identity mismatch and denied/unmanaged scope fail
   closed; production AF dispatch retains the required remote scope check.
3. `UT-ACM-2584-LIVE-003` / `IT-FLEET-2584-LIVE-003`: limits, complete traversal,
   cursor behavior, source freshness, and partial/error responses never claim
   completeness without evidence; exercise Search count/version behavior.
4. `UT-ACM-2584-LIVE-004` / `IT-FLEET-2584-LIVE-004`: Search transport/auth
   failure and insufficient result visibility are not misrepresented as empty
   or as an authoritative managed decision.

The user approved correcting the shared checker and a strict error-preserving
Console path, with legacy callers retaining fail-closed error behavior. The
production Go ACM checker was subsequently exercised against the live Search
service: explicit true returned managed, explicit false returned unmanaged, and
an unlabelled resource inherited its namespace's true label. This is checker
evidence only; it does not qualify the inventory adapter, production AF dispatch,
bounded traversal, or a least-privilege ACM Search deployment. The live spike used
a short-lived privileged test identity, whose temporary cluster-role binding was
removed afterward. See `IT-ACM-2584-STRICT-001` for factory-level strict/legacy
error-path coverage. Existing fixture-only inventory PASS is not live evidence.

The user subsequently approved a temporary fail-closed ACM Console inventory
gate. `IT-AF-2584-ACM-FAIL-CLOSED` exercises production dependency construction
and verifies remote inventory list, exact read, and policy-scope list return
`unavailable` with no records. `UT-AF-2584-FMC-READER` verifies that FMC wiring
remains enabled. The ACM shared scope checker is unaffected by the inventory
gate. Live least-privilege visibility, freshness, pagination, and production AF
journey evidence are required before enabling ACM inventory. The subsequent
snapshot decision supersedes the source-freshness requirement stated above.

#### ACM scope and registry follow-up (2026-11-03)

The SNO-b ACM 2.17.3 Search spike used a short-lived, scoped identity with
fine-grained RBAC and TLS verification. A cluster-wide explicit-label query
returned the known managed ConfigMap and the managed Namespace; in a separate
namespace-label transition, the explicitly managed ConfigMap remained visible
when its namespace was set to false. These observations support the planned
union of managed-namespace resources and explicitly managed resources. They do
not establish full fleet visibility, bounded freshness, a consistent paginated
snapshot, or a live gateway-registry identity match. The SNO had no supported
gateway registration CRDs. The temporary privileges were removed and the MCH
fine-grained RBAC setting and Search availability were restored.

The user requires the managed gateway registration ID, ACM cluster name, and
Console cluster ID to be identical; no automatic mapping or alias is authorized.
AF's ACM scope factory now receives the existing gateway-neutral cluster
registry lookup; an absent, unready, or unknown registry entry fails closed.
An ACM Search miss is ambiguous, so the strict path returns an error for an
invisible resource or namespace, while the legacy scope API returns false and
nil error. This does not establish freshness of positive Search results.

`IT-ACM-2592-EMPTY-001/002`, `IT-AF-2592-EMPTY-001`, and
`IT-AF-2593-REGISTRY-001/002` passed after failing in RED. The registry wiring
test uses a real Envoy registry with a fake Kubernetes client; the matching-ID
cases `IT-AF-2593-REGISTRY-003` exercise both supported registry adapters.
`UT-ACM-2592-001/002` and `UT-REG-2593-001/002` cover ambiguous misses,
cluster-scoped defaults, and unavailable registries. These are targeted
business/control checks, not a complete requirements-coverage assessment or
proof of a production HTTP journey. The ACM Console inventory gate remains
in place, and no new audit event is introduced by this checker change.

#### Bounded inventory prototype (2026-11-03)

The gated adapter now queries managed namespaces, explicitly managed resources,
and resources in managed namespaces; it resolves effective labels with the
shared resolver and uses canonical identity hashes for exact matching. An
explicitly managed resource in another namespace requires a visible, unique
namespace result. Each Search request has a 1,000-item bound; a full batch or a
reported truncation produces a state-only partial result, including for exact
reads. Sorted Console pages use the last resource ID as a cursor after each
bounded traversal; each request is a new query-time view, not a transactional
snapshot spanning requests.

`UT-ACM-2593-UNION-001` and `UT-ACM-2593-EXACT-001` failed before this
implementation and pass afterward. `UT-ACM-2593-BOUND-001` and
`UT-ACM-2593-VISIBILITY-001` exercise the bound and missing namespace; they were
added after implementation, not observed in RED. The union scenario also checks
inheritance, invalid resource-label fallback, opt-in/out overrides, deduplication,
cluster-scoped inclusion, cursor paging and an inherited exact lookup. These are
fixture-level tests, not a complete scenario-coverage assessment.

Search can silently hide records, and multiple Search requests are not an atomic
snapshot. A result below the bound does not prove visibility or consistency under
mutation. The gate therefore remains closed; gateway registration identity and
the production AF/live Search journey have not been qualified on SNO-b.

#### Scoped adapter scenario assessment (2026-11-03)

The following 16 meaningful scenarios are in scope for the current adapter;
14 have passing fixture-level assertions. The remaining two are not waived,
so the 90% scenario target and critical-scenario gate are not met.

| Scenario | Evidence / status |
|---|---|
| Unlabelled resource inherits managed namespace | `UT-ACM-2593-UNION-001` PASS |
| Resource false overrides managed namespace | `UT-ACM-2593-UNION-001` PASS |
| Resource true overrides unmanaged namespace | `UT-ACM-2593-UNION-001` PASS |
| Invalid resource label inherits managed namespace | `UT-ACM-2593-UNION-001` PASS |
| Explicitly managed cluster-scoped object included | `UT-ACM-2593-UNION-001` PASS |
| Stable fixture paging, filtered results and invalid cursor | `UT-ACM-2593-UNION-001` PASS |
| Namespace, kind and resource-name filters narrow large Search results | `UT-ACM-2593-FILTER-001` PASS |
| Canonical exact lookup succeeds | `UT-ACM-2593-EXACT-001` PASS |
| Wrong or missing cluster identity rejected | `UT-ACM-2593-IDENTITY-002`, existing cross-cluster tests PASS |
| Missing namespace visibility fails closed | `UT-ACM-2593-VISIBILITY-001` PASS |
| Detected truncation/bound does not return data or false not-found | `UT-ACM-2584-LIVE-003/007`, `UT-ACM-2593-BOUND-001` PASS |
| Conflicting labels in two query results fail closed | `UT-ACM-2593-CONSISTENCY-001` PASS |
| GraphQL failure fails closed | ACM canonical inventory backend failure test PASS |
| Exact absent resource in a known-empty fixture | `UT-ACM-2593-EXACT-002` PASS; fixture completeness is assumed, not inferred from ACM |
| Changes during traversal that do not create conflicting duplicates | Not detectable by current adapter; not proven |
| Silent Search filtering versus genuinely empty cluster | Not distinguishable from Search alone; not proven |

These are adapter unit scenarios, not 16 independent test functions; some tests
prove more than one business outcome. The audit assertion in
`IT-FLEET-2584-LIVE-003` checks a partial-state event through the service and
real ACM client with an in-memory emitter, not persistence or a live gateway.
`UT-ACM-2593-CONSISTENCY-001`, `UT-ACM-2593-EXACT-002` and the added filter/cursor/audit assertions were
added after the underlying behavior; no prior RED is claimed for them.
`IT-AF-2593-PARTIAL-001` exercises the Console HTTP handler, policy service,
and real ACM adapter against an HTTP Search fixture and verifies that a partial
response leaks no records. `IT-AF-2593-DENY-001` verifies that capability
denial prevents the adapter from contacting Search. Both pass; neither is a
production factory or live-cluster journey, and neither was observed RED.

The user accepts bounded results and identifies ACM as the authoritative tool
for deeper inventory browsing; the Console is a convenience query view, not the
single source of truth. Namespace, resource name and kind filters are now sent
to the relevant ACM resource queries before the traversal limit is applied;
`UT-ACM-2593-FILTER-001` was observed RED then GREEN using over 1,000 unrelated
objects, and `IT-AF-2593-FILTER-001` verifies HTTP-to-adapter filter dispatch
against a fixture (added after implementation, without a RED observation).
The managed-namespace discovery query remains independently bounded, so a kind
or resource-name filter alone does not guarantee success when that query itself
reaches 1,000 namespaces; a namespace filter narrows discovery. Exact opaque-ID
lookups are also bounded. These are detected partial results, not claims that
ACM contains no additional records.

**Traversal investigation:** ACM Search's upstream `release-2.17`
[`search.go`](https://github.com/stolostron/search-v2-api/blob/release-2.17/pkg/resolver/search.go)
implements `orderBy: "uid asc"` against its `uid` column and returns that value
as `_uid`. This is a possible ordering key without duplicate *names*, but the
source branch has not been matched to the deployed image, and uniqueness,
offset behavior, and mutations have not been qualified for the target service.
No offset pagination has been implemented on that basis. The current 1,000-item
Search and union limits deliberately fail closed rather than skip records.

**Read-only gateway deployment preflight (2026-11-03):** On SNO-b, the
`redhat-operators` PackageManifest for `rhcl-operator` reports default
`stable` channel current CSV `rhcl-operator.v1.4.3`; `stable-1.5` reports
`rhcl-operator.v1.5.0`. The MCP Gateway v1.0.2 OpenShift deployment script
requires RHCL 1.5.0 or newer but its example Subscription selects `stable`
with automatic install approval. The example cannot be used unchanged on this
cluster. The v1.0.2 Git tag resolved to commit
`e48ae51cfa52a57f93f334bdadbe8ce58ef5a9f6` at preflight. Upstream manifests
also declare a cluster-scoped GatewayClass `openshift-default` and an
all-namespaces operator group. This is read-only catalog and source evidence,
not proof that RHCL 1.5.0 installs or supports the intended MCP Gateway release.
No resources were created or modified. Exact artifact ownership and rollback,
release compatibility and explicit installation approval remain prerequisites.

#### Approved unknown-scope listing policy (2026-11-04)

For a managed-only query, if a user-visible resource has an absent or invalid
resource managed label and the Namespace metadata needed for inheritance is
inaccessible, its scope is unknown. Return a generic incomplete/partial state
with no rows or total, including when other resources have known scope. Do not
guess, expose hidden namespace labels, or use privileged namespace enrichment.
Explicit resource `managed=true` and `managed=false` remain decisive without
Namespace access, subject to independent user authorization and other gates.

This is an approved contract decision, not evidence of implementation or a test
pass. The historical bounded prototype and its results above remain as recorded;
the production ACM inventory gate remains closed pending qualification.

| Future scenario | Required outcome |
|---|---|
| `UT-AF-2584-VISIBILITY-001` | Visible resources with explicit true or false resolve independently of inaccessible Namespace metadata; neither requires namespace enrichment. |
| `UT-AF-2584-VISIBILITY-002` | Each absent/invalid label case with inaccessible Namespace yields unknown and a generic partial managed-only result with no rows or total, including alongside otherwise returnable resources. |
| `IT-AF-2584-VISIBILITY-001` | Production list dispatch preserves this state-only response and leaks neither rows, totals nor hidden namespace metadata; separately exercise applicable local and fleet paths. |

These are planned scenarios, not passing tests; authorization and actual backend
visibility must be separately qualified.

### 16.4 Local inventory memory-containment contract (2026-11-04)

For `cluster_id=local`, the Console inventory reader is metadata-only. It requires a
Kubernetes `metadata.Interface`, reads `PartialObjectMetadata`, traverses server pages
with a centralized positive internal item-count hint plus `Continue`, projects each item
immediately to the canonical sanitized identity/scope model, and discards the backend page
before continuing. The hint is an internal traversal chunk, not the Console page limit or
an inventory total. A missing metadata client never falls back to dynamic full-object
listing. Repeated/non-progressing or expired continuation, cancellation, and incomplete
traversal return state-only failure.

The provisional containment defaults are 32 MiB of retained canonical working set, a
positive internal backend item-count hint aligned with the inventory traversal bound, a
16 MiB per-decrypted-response limit on the dedicated policy-read HTTPS client, a 2 MiB
pre-write serialized-response limit, a 15-second cancellable operation timeout, and two
concurrent local reads per AF replica. The response-body limit is enforced before client-go
decoding and preserves the existing TLS/HTTPS configuration, but it is not a complete process
RSS ceiling because transport, decoder, and runtime overhead remain. The retained projection
budget, positive item-count hint, response-body limit, operation cancellation, semaphore, and
pre-write response check are the enforceable safeguards.

Console policy/inventory list operations are local-only for this release; ACM/FMC list
operations remain unavailable. Existing exact/get behavior is unchanged except for the
common response safety guard. These bounds do not introduce public count/total/reload/
snapshot/freshness or page-level `observed_at` semantics. The Console makes no
cross-request snapshot or source-freshness claim.

The containment tests listed in §8 prove metadata-client enforcement, positive default
page hint, Gateway-aligned default kinds, page traversal, continuation failure,
cancellation, projection discard, retained-byte exhaustion, shared List/Get concurrency
cancellation, pre-write response rejection, and production-router local dispatch. They do not
qualify ACM/FMC list visibility or completeness; that gate remains closed.

## 17. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-11-01 | Initial approval-gated plan; records AF boundary, operator PR #604 contract, state model, scenarios, and unresolved threshold/auth decisions |
| 1.1 | 2026-11-01 | Threshold direction approved: required operator-sourced read-only value; nullable/unknown fallback removed |
| 1.2 | 2026-11-01 | Named capabilities/target SARs, minimal SignalProcessing read RBAC, and typed audit events approved |
| 1.3 | 2026-11-01 | Current operator release boundary added; first RED gates are typed audit persistence/retrieval and `apifrontend` category normalization; broader SOC2 evidence deferred |
| 1.4 | 2026-11-01 | Implementation and scoped validation completed; AF gates, adapter/audit evidence, Fleet E2E port collision, and cross-repository deferrals recorded |
| 1.5 | 2026-11-04 | Local metadata-only inventory containment, client-go page-allocation limitation, bounded working set, cancellation/concurrency guard, response-size guard, and local-only list contract recorded |
| 1.6 | 2026-11-05 | Added the 16 MiB decrypted Kubernetes response-body containment test and documented its HTTPS-preserving, non-RSS-complete scope |
