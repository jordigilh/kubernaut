# DD-AF-018: Authenticated Console Policy and Managed-Resource Read Projection

**Status**: ✅ Accepted — local Console projection implemented; ACM inventory qualification remains gated
**Date**: 2026-11-01
**Issue**: [#2584](https://github.com/jordigilh/kubernaut/issues/2584)
**Related**: [BR-SCOPE-001](../../requirements/BR-SCOPE-001-resource-scope-management.md), [BR-SECURITY-016](../../requirements/BR-SECURITY-016-kubernetes-rbac-enforcement.md), [BR-AI-085](../../requirements/BR-AI-085-rego-policy-input-schema.md), [BR-AI-088](../../requirements/BR-AI-088-configurable-confidence-thresholds.md), BR-AUDIT-005, [ADR-053](ADR-053-resource-scope-management.md), [ADR-034](ADR-034-unified-audit-table-design.md), [ADR-068](ADR-068-fleet-federation-architecture.md), [kubernaut-console#164](https://github.com/jordigilh/kubernaut-console/issues/164), [kubernaut-operator#604](https://github.com/jordigilh/kubernaut-operator/pull/604)

---

## Context

The standalone Console needs a server-owned, read-only projection for its
`policies.scope` view. API Frontend currently exposes authenticated A2A/MCP protocol
endpoints and the advisory `/a2a/access` check, but no typed policy, scope, or managed
inventory read surface. The Console uses an OAuth2 Proxy httpOnly session and must not
store a browser bearer token or call DataStorage/Kubernetes directly.

The projection must connect policy identity and historical evaluation metadata to an
exact managed-resource identity without exposing raw Rego, ConfigMap contents, secrets,
tokens, internal endpoints, private reasoning, or arbitrary payloads. It must also
preserve the existing scope precedence and keep A2A/MCP service-account behavior unchanged.

The approved contract is recorded with the issue-specific test and implementation plans:

- [`TEST_PLAN.md`](../../tests/2584/TEST_PLAN.md)
- [`IMPLEMENTATION_PLAN.md`](../../tests/2584/IMPLEMENTATION_PLAN.md)

## Clarifying addendum: interactive visibility and scope (2026-11-04)

**Direction approved in principle; implementation, API contract and authorization
qualification remain pending.** This addendum identifies a proposed change to the
accepted remote authorization model in §3 below; it does not silently replace that
decision or assert that the new behavior is implemented. Resolve the model and its
tests before release. The production ACM Console inventory gate remains closed.

- Interactive results must contain only resources visible to the user. Prefer ACM
  Search with user-scoped authorization, subject to qualifying token acceptance,
  audience and actual per-user authorization end to end. FMC authorization is a
  separate qualification, not implied by an ACM result. Do not enrich results with
  privileged Namespace reads or fabricate namespace labels.
- A visible resource's `metadata.namespace` is returned resource data; it does not
  prove that the user can read the Namespace object. Denial of a Namespace object
  likewise does not establish denial of a Pod or other namespaced resource.
- In the ADR-053 precedence model, an explicit valid resource
  `kubernaut.ai/managed=true` or `false` decides scope without namespace access.
  If the resource label is absent or invalid and the namespace is invisible, the
  inherited scope is **unknown**, not false. If a managed-only query encounters
  such a user-visible resource, return a generic incomplete/partial state with no
  rows or total; do not guess or use privileged namespace enrichment. This policy
  is approved but not yet implemented or qualified. ADR-053's safe default for
  known unlabeled inputs is not evidence that an inaccessible namespace is unlabeled.
- Default eligible target kinds should align with Gateway-supported targets and
  remain extensible. Return only a bounded sanitized projection, never sensitive
  contents. AF owns the authenticated, versioned API contract and authorization;
  the Console consumes its states and identities without evaluating scope itself,
  storing browser tokens, or directly accessing Kubernetes or DataStorage.
- The Console renders each request result at query time. Complete and empty pages expose
  the backend observation time and bounded freshness state for that read, but the API
  exposes no snapshot identifier, cross-request snapshot guarantee, or synchronization
  watermark. Its exact API semantics remain subject to the local-only release contract below.

## Decision

### 1. AF owns the user read boundary

Expose the projection through authenticated AF REST routes:

```text
GET /api/v1/console/policies/scope
GET /api/v1/console/inventory/managed-resources
GET /api/v1/console/inventory/managed-resources/{resourceID}
```

The routes use the existing AF authentication and same-origin ingress path. They are
additive and do not replace or alter A2A, MCP, or `/a2a/access` behavior.

Every request has an explicit cluster identity. `local` identifies the hub cluster and
uses AF's local Kubernetes clients. Console policy-scope and managed-resource list
operations are local-only for this release; a non-local list request returns a
state-only `unavailable` response and never falls back to the hub. Non-local exact
reads retain the existing typed FMC/ACM control-plane path and managed-scope check.
The Console path does **not** use AF's MCP `FleetReaderFactory` and does not connect
directly to a remote Kubernetes cluster. The existing AF `ScopeChecker` remains the
remote exact-target managed-scope authorization seam; it already routes to FMC or ACM.
Missing, unknown, unavailable, or unauthorized cluster identity never falls back to
local data.

#### ACM live-validation correction (2026-11-02)

The user approved correcting the existing shared ACM `ScopeChecker` together with
the Console ACM inventory adapter, rather than adding a Console-only second scope
algorithm. The existing checker remains the exact remote scope-check seam. ACM
Search results must be interpreted through the shared ADR-053 resource/namespace
precedence rules, including explicit false, invalid-label provenance, inheritance
only for namespaced resources, and safe defaults. The inventory projection must
use ACM's observed typed Search representation, preserve explicit cluster and
opaque identity, and not claim completeness without evidence. This release does not
enable remote inventory list projection; exact/get and scope-check behavior remain
unchanged.

The live ACM 2.17.3 spike found that the existing checker query matched an
explicitly unmanaged object and that existing inventory fixture shapes did not
match Search. Gateway and Remediation Orchestrator also consume this shared
checker. The existing federated wrapper currently collapses remote errors to
`false, nil`, so a Console caller cannot distinguish backend failure from a
genuine not-managed result. These are implementation and regression risks, not
evidence that the approved boundary is complete. Detailed live observations and
required RED scenarios are in `docs/tests/2584/TEST_PLAN.md` §16.3.

The strict Console scope path is additive. Legacy `IsManagedResource` callers retain
their existing fail-closed behavior and compatibility signature; they do not consume
the Console-only error-preserving result. Both paths use the same shared managed-scope
decision and backend routing.

**Alternatives considered:** a Console-only additional scope check was rejected
because it leaves the shared checker producing false managed decisions and
duplicates security policy. Trusting Search identity/existence or treating an
incomplete page as complete was rejected because neither proves managed scope.
The user approved an error-preserving strict scope path for Console while keeping
the existing fail-closed `IsManagedResource` behavior for Gateway, Remediation
Orchestrator, and other existing callers. Both paths must share the same managed
scope decision, backend, and cluster routing; a health probe is not a substitute
for propagating the outcome of the actual check. Bounded pagination remains
subject to evidence-based preflight; no new authorization or data-access boundary
is authorized by this correction.

**Temporary ACM inventory gate (2026-11-02):** the user approved returning
`unavailable` for ACM-backed Console inventory and policy-scope reads until the
adapter can qualify Search visibility, identity and completeness. The
production inventory factory does not wire the unqualified ACM reader; AF logs
the condition and does not fall back to a different cluster. The shared ACM
scope checker remains wired for existing consumers. This is a temporary
fail-closed release limitation, not completed ACM inventory functionality.

#### Console result metadata (2026-11-03/05)

Complete and empty policy-scope and managed-resource pages expose the backend
observation time and the existing bounded `freshness` envelope. These values describe
the completed read that produced the response; they do not claim a transactional
snapshot, cross-request consistency, or an ACM synchronization watermark. A backend
freshness value does not automatically change a successful read into the shared
`stale` state. Detected identity, authorization, backend, and incomplete-traversal
failures remain state-only and fail closed.

Alternatives considered: hiding observation/freshness metadata was rejected because
Issue #2584 requires the Console to display observed time and freshness/completeness;
inventing a synchronization watermark or snapshot identifier was rejected because no
qualified backend provides one.

### Local inventory memory-containment contract (2026-11-04)

The local `cluster_id=local` inventory path is local-only and metadata-only. It uses a required
`metadata.Interface` and Kubernetes `PartialObjectMetadata` pages; construction fails with
`local inventory metadata client is required` when that dependency is absent. The reader never
falls back to dynamic full-object listing. ACM/FMC list operations remain unavailable through
the Console policy/inventory list surface. Existing exact/get behavior is otherwise unchanged,
apart from the common serialized-response safety guard; local exact reads use the same bounded
metadata projection as local list reads.

Local traversal requests the server's `Limit` and `Continue` protocol with a centralized positive
internal item-count hint for every metadata request. The hint is an internal traversal chunk,
independent of the Console's public page limit or any inventory total. Each metadata item is
immediately reduced to the canonical sanitized identity/scope projection, then the backend page
is discarded before the next page is requested.
Repeated or non-progressing continuation, expired continuation, cancellation, and any incomplete
read fail closed with a state-only response. A caller cursor that no longer exists because the
working set drifted is tolerated by rerunning the current deterministic sorted projection; it is
not treated as caller input that can widen the query.

The provisional process-local safety defaults are:

- 32 MiB of retained canonical projections per local operation;
- 2 MiB for a serialized successful Console response, checked before the first byte is written;
- 16 MiB per decrypted Kubernetes API response for the dedicated policy-read client,
  enforced before client-go decodes the response body;
- a 15-second cancellable operation timeout, including semaphore wait; and
- two concurrent local inventory reads per API Frontend replica, shared by `List` and `Get`.

The dedicated policy-read REST/metadata client preserves the existing HTTPS endpoint, TLS
configuration, authentication, and proxy behavior while applying a provisional 16 MiB
client-side limit to each decrypted response body. A response that exceeds that limit fails
closed before client-go can decode an unbounded body. This is not a complete process RSS ceiling:
transport buffers, decoder allocations, runtime overhead, and other AF work remain outside the
body limit. The other enforceable safeguards are the positive internal item-count hint, required
metadata client, immediate projection/discard, retained canonical working-set bound, operation
cancellation, concurrency bound, and pre-write serialized-response check. A serialized Console
response that exceeds 2 MiB becomes a state-only `unavailable` response; no partial success body
is written.

These implementation bounds do not add public count, total, reload, or snapshot semantics.
Complete and empty responses expose the per-read `observed_at`/`observedAt` timestamp and
bounded `freshness` envelope; they do not claim a cross-request transactional snapshot or an
ACM synchronization watermark. Historical evaluation timestamps, where already persisted and
explicitly projected, remain source record timestamps rather than page-observation timestamps.

### 2. Runtime ConfigMaps are the policy metadata source

AF reads the existing operator-generated `aianalysis-config` and
`signalprocessing-config` ConfigMaps in the AF namespace for each policy-scope
operation. It reads only the allowlisted `config.yaml` key and the following typed
fields:

| Policy | Runtime metadata | Policy ConfigMap/key |
|---|---|---|
| AIAnalysis | `rego.confidenceThreshold` | Existing `aianalysis-policy` / `approval.rego` convention |
| SignalProcessing | `classifier.regoConfigMapName` and `classifier.regoConfigMapKey` | The referenced name / `policy.rego` |

The threshold must be finite and in `(0.0, 1.0]`; the SignalProcessing name/key must
be present and valid. AF hashes the referenced policy bytes for a revision and never
returns their contents. Missing, malformed, unauthorized, or inconsistent runtime
metadata fails closed for the operation. AF does not cache these ConfigMap reads.

Current AIAnalysis runtime configuration exposes the mounted policy path but not a
ConfigMap name. AF therefore uses the existing `aianalysis-policy` convention and
fails closed when that ConfigMap/key is unavailable; it does not enumerate ConfigMaps
or guess a custom name. The operator team owns the follow-up contract that will publish
the stable AIA policy reference and remove this limitation and the generated legacy
`policyRead` duplication. That follow-up must be a new approved operator change; this
PR does not extend operator PR #615.

The legacy `policyRead` block remains decodable for compatibility but is ignored for
policy metadata discovery and is not validated as a second source of truth. AF does
not parse arbitrary Rego or infer a Rego default.

### 3. Authorization is per user and per target

Interactive Console reads require:

1. A verified AF user identity.
2. A non-service-account identity on the `/api/v1/console` surface.
3. Named capability SARs:
   - `kubernaut_read_policy_scope`
   - `kubernaut_list_managed_resources`
   - `kubernaut_get_managed_resource`
4. For a local target, a list or exact-target SAR for the hub
   group/version/resource/namespace/name.
5. For a remote target, the existing AF `ScopeChecker` must prove the exact
   cluster/GVK/namespace/name is in Kubernaut's managed scope through FMC or ACM.
   This check uses AF's backend service identity; the current fleet control-plane
   contract does not delegate the interactive user's Kubernetes token.
6. The optional coarse `kubernaut.ai/console` gate when configured.

The AF ServiceAccount is a backend transport identity, never a substitute for the
interactive user's named Console capability. The remote fleet-scope proof establishes
Kubernaut ownership of the exact target, not the user's independent remote-cluster
RBAC. User-token delegation to remote Kubernetes is explicitly not part of this issue.
Existing A2A/MCP service-account paths are unchanged.

### 4. Historical SignalProcessing reads use minimal RBAC

The v1 projection may expose only fields persisted by the existing CRDs. The operator
grants the AF ServiceAccount only `get` and `list` on `signalprocessings`, with explicit
RBAC tests and no write verbs. If a required source cannot be read completely, AF returns
a state-only failure response rather than partial records.

### 5. The projection is typed, sanitized, and fail-closed

The response contains typed resource identity, scope/provenance, current policy identity
and revision, the required configured threshold, bounded historical evaluation fields,
completeness, and validated inventory links. Local list/policy-scope projections use the
hub Kubernetes API; non-local list operations are unavailable for this release. Non-local
exact reads use only the typed FMC/ACM inventory projection and existing managed-scope check.
Neither path returns raw policy source, arbitrary ConfigMap keys/data, full Kubernetes
objects, secrets, tokens, private reasoning, or unbounded labels/annotations.

The shared state vocabulary includes success, empty, partial, stale, unavailable, forbidden,
and not-found states. `loading` is a Console client state only. Local reads do not derive
`stale` from freshness metadata. Partial, stale, unavailable, forbidden, and not-found
responses contain no records, counts, policy names, labels, or provenance. Invalid filters
return a structured 400 and never become an empty result.

Pagination is cursor-based, bounded, deterministic, and has no total-count field.

### 6. All authenticated Console reads use typed audit events

Extend the existing AF/DataStorage audit union; do not use the current logged-only path
for events without a typed payload. Add typed actions for policy-scope reads, managed
resource list reads, and managed-resource exact reads.

The AF event types are:

- `console.policy_scope_read`;
- `console.managed_resource_list_read`; and
- `console.managed_resource_get_read`.

The persisted event types use the existing `apifrontend.` prefix. The existing audit
outcome enum remains stable (`success` or `failure`); denied, not-found, partial, stale,
and unavailable distinctions are carried by the typed payload's bounded `result_state`.

Emit one typed event for every authenticated Console read request, including success,
forbidden, not-found, partial, stale, and unavailable outcomes. Unauthenticated
requests remain covered by the existing authentication events. Do not emit one event per
returned record.

Each event carries, at minimum:

- `event_id`, event type, action, and outcome;
- actor type and verified actor ID;
- request/correlation ID and timestamp;
- operation, cluster ID, and bounded normalized filter/resource fingerprint; and
- safe result state or standardized error details.

Audit payloads must not contain returned records, item payloads, raw Rego, secrets,
tokens, private reasoning, or arbitrary query values. DataStorage remains authoritative
for audit integrity and retention; persistence failures remain observable. AF emission
is non-blocking and initially buffered in memory. `Flush` is used by live persistence
tests and graceful shutdown to wait for the queued write attempt; it is not a crash-
durability guarantee before the flush. Buffer drops and write failures remain observable
through the existing store error/metric/log paths.

AF policy-scope reads resolve runtime ConfigMap metadata per operation. The existing AF
configuration watcher may continue to decode legacy `policyRead` content for compatibility,
but changing that block does not alter policy-read behavior and does not require an AF
restart. Runtime ConfigMap changes are visible on the next operation, subject to the
Kubernetes API's normal read semantics.

This supports SOC 2 CC7.2 investigation/monitoring and FedRAMP AU-2/AU-3 event and
content requirements. Typed events alone are not a complete SOC 2 certification: the
existing DataStorage protection, retention, restricted audit access, review, and
alerting controls remain required.

### Control Evidence Boundary

For the current operator release, the required audit baseline beyond event emission is:

1. **Completeness**: every authenticated Console read operation and outcome is emitted,
   and the event population is queryable by correlation/request ID.
2. **Safe persistence**: typed DataStorage validation, bounded payloads, and observable
   persistence failure prevent a silent audit gap.
3. **Operator observability**: the `apifrontend` category remains a bounded metric label
   so operators can distinguish AF audit lag from unrelated services.

The following evidence remains useful but is not a #2584 release blocker unless the project
explicitly chooses a SOC 2 certification scope:

4. **Integrity and protection**: DataStorage's append-only/integrity controls, restricted
   audit-reader access, backup, and service-to-service transport protections apply to
   these events; AF must surface persistence failures rather than silently discard them.
5. **Retention and retrieval**: the applicable retention policy, legal hold behavior, and
   an operator/auditor retrieval procedure cover the new event types.
6. **Monitoring and review**: denied-read bursts, unusual scope access, and audit-pipeline
   failures have dashboards/alerts, an owner, review cadence, and escalation evidence.
7. **Operating effectiveness**: automated tests, RBAC/access reviews, change approvals,
   incident/runbook records, and sampled audit evidence demonstrate the controls operated
   throughout the assessment period.

This feature provides operator auditability and contributes to CC7.2 and FedRAMP
AU-2/AU-3. It does not claim a SOC 2 attestation. AU-9/AU-11 protection and retention
remain inherited DataStorage controls; their deployment-specific certification evidence
is a future scope decision. The implementation change itself remains subject to the
project's CC8.1 change-management process.

## Alternatives Considered

### Direct browser access to DataStorage or Kubernetes — ❌ Rejected

It would expose internal API boundaries, complicate token/audience trust, bypass AF's
user/scope authorization, and make the Console responsible for sensitive credentials.

### Coarse Console authorization only — ❌ Rejected

The existing coarse gate is optional and does not prove named endpoint permission or
authorization for the exact requested resource.

### AF maintains a second policy metadata block — ❌ Rejected

The legacy `policyRead` block is compatibility-only. AF reads the operator-owned runtime
ConfigMaps used by AIAnalysis and SignalProcessing so policy metadata is not duplicated
in a second operator-owned source.

### AF parses arbitrary Rego or infers defaults — ❌ Rejected

AF reads only the small allowlisted runtime metadata fields and hashes fixed policy keys.
It must not interpret arbitrary customer policy source or claim a Rego default that the
operator did not publish.

### Nullable threshold with an `unknown` fallback — ❌ Rejected

The approved v1 deployment contract requires the threshold. A missing value is a
configuration failure, not a valid read state. No backward-compatible inference is
needed or permitted.

### Skip typed audit union changes — ❌ Rejected

The existing AF adapter intentionally skips persistence for events without typed payload
schemas. Leaving Console reads on that path would make user read activity unavailable to
the authoritative audit query path and would weaken AU-2/AU-3 evidence. The union,
generated client, AF event registry, payload builders, and tests must be extended together.

### Emit one audit event per returned record — ❌ Rejected

It creates unbounded event volume and duplicates sensitive inventory identity. One event
per authenticated request records the user action and result without copying the response.

### AF reads a remote Kubernetes cluster directly — ❌ Rejected

AF is colocated with the other Kubernaut services in the hub/control-plane cluster.
Direct MCP/remote-Kubernetes reads would create a second Console-specific fleet access
path and would not carry the interactive user's remote-cluster authorization. Remote
inventory is therefore owned by the existing fleet control plane: FMC and ACM expose
the same bounded identity/scope projection contract, and AF consumes that contract.

### Omit SignalProcessing RBAC and silently degrade history — ❌ Rejected

The approved v1 contract includes complete historical reads where the fields exist. The
operator therefore grants only AF `get/list` access, and AF fails closed when a required
history source is unavailable.

## Consequences

### Positive

- The Console receives one same-origin, versioned, user-scoped read boundary.
- Threshold display cannot drift from the value used by AIAnalysis.
- Exact target SARs preserve per-user and per-resource least privilege.
- FMC/ACM remain the single fleet boundary for remote exact reads and exact managed-scope
  checks; remote Console list operations remain unavailable and AF does not create a direct
  remote-cluster reader.
- Read attempts are attributable through the same typed DataStorage audit path as other
  AF business/security events.
- A missing backend or incomplete history cannot produce a misleading partial view.

### Trade-offs

- The operator repository must publish a stable AIAnalysis policy ConfigMap reference in
  runtime configuration, then remove the generated legacy `policyRead` duplication;
  this follow-up is owned by the operator team and is not an extension of PR #615.
- Remote Console list qualification remains gated on shared FMC/ACM inventory evidence; existing
  remote exact/get and scope-check behavior remains unchanged.
- DataStorage OpenAPI and generated client artifacts must change for the typed read events.
- Historical matching remains bounded by existing CRD fields until a separately approved
  index exists.
- The Console must map explicit AF read states rather than treating every non-200 as empty.

## Wiring Manifest

| Component | Production entry point | Wiring location | Integration proof |
|---|---|---|---|
| Runtime policy metadata | Operator-generated AIA/SP runtime ConfigMaps | AF policy source and policy-scope request path | UT-AF-2584-006 / IT-AF-2584-001 |
| Console policy read | `GET /api/v1/console/policies/scope` | AF router and handler construction | IT-AF-2584-001/003 |
| Managed inventory list | `GET /api/v1/console/inventory/managed-resources` | AF router and projection service | IT-AF-2584-002/007 |
| Managed inventory exact read | `.../managed-resources/{resourceID}` | AF router and projection service | IT-AF-2584-005 |
| User/capability/target authorization | Authenticated Console request | AF auth adapter and SAR checker | IT-AF-2584-004/005 |
| Fleet inventory projection | Remote Console exact/get read | FMC and ACM control-plane adapters | IT-FLEET-2584-001/002 |
| SignalProcessing history | Policy projection | AF typed client plus operator RBAC | IT-AF-2584-003/006 |
| Typed read audit events | Every authenticated read outcome | AF audit registry/adapter and DataStorage union | IT-AF-2584-010/011 |

## Implementation Status

The contract is accepted and implemented on branch `openchamber/issue-2584-console-policy-read`
using wiring-first RED/GREEN/REFACTOR. The implementation includes the authenticated AF
REST handlers, local Kubernetes projection, typed FMC/ACM inventory adapters, named
capability plus target authorization, structured scope provenance, typed audit events,
OpenAPI schemas, generated DataStorage client artifacts, and production dependency wiring.

Policy metadata is resolved from the existing runtime ConfigMaps on every policy-scope
operation. Legacy `policyRead` values remain decodable but are not consumed. The AIA
custom policy-name gap is intentionally fail-closed and is assigned to the operator
follow-up contract rather than guessed in AF.

Scoped validation passed for affected packages, full compilation/build, lint, typed audit
persistence, and FMC/ACM adapter behavior. Fleet-only AF E2E is blocked by an unrelated
host-port `8088` collision with `fullpipeline-e2e`; Console browser/deployed acceptance
remains deferred to `kubernaut-console#164`. See
[`IMPLEMENTATION_PLAN.md`](../../tests/2584/IMPLEMENTATION_PLAN.md) and
[`TEST_PLAN.md`](../../tests/2584/TEST_PLAN.md) for the evidence boundary.
