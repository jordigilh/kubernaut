# DD-IDENTITY-001: Group-Aware Resource Identity Across Severity Triage and the Execution Plane

**Status**: ✅ APPROVED (2026-11-17, maintainer sign-off in PLAN-2658.md §2)
**Issue**: [#2658](https://github.com/jordigilh/kubernaut/issues/2658)
**Related**: DD-AF-012, DD-AF-003, DD-AF-016, DD-WE-001, DD-RO-001, DD-RO-002, DD-CONTRACT-002, DD-K8S-001, DD-GATEWAY-019, DD-AUDIT-003, ADR-034
**Business requirements**: BR-SEVERITY-001 (AC-8), BR-ORCH-042 + ADR-052, BR-AUDIT-005, BR-GATEWAY-058-A
**Controls**: FedRAMP SI-10, AU-2/AU-3/AU-12; SOC2 CC7.2; AC-4 (DD-RO-002 / DD-WE-001)
**Implementation plan**: `PLAN-2658.md` v12; test plan `docs/testing/2658/TEST_PLAN.md`

---

## Context

Kubernetes Kinds are not unique across API groups: `Route` exists in
`route.openshift.io` and `serving.knative.dev`, `Event` in the core group and
`events.k8s.io`, `Ingress` in `networking.k8s.io` and `extensions`. Every
identity Kubernaut built from `namespace/kind/name` alone therefore conflated
two distinct resources. The consequences were not cosmetic:

1. **Severity triage** correlated alerts and rules by `namespace/kind/name` only,
   so a cross-group sibling's alert could supply the target's severity (the
   "group-blind severity" defect fixed by PR #2653's fingerprint work and this
   record's severity half).
2. **The execution plane** keyed the busy gate, cooldown gate, approval lock,
   WorkflowExecution target, audit join key and DataStorage history query on the
   same three-part string, so two siblings shared one lock and one history chain.

The severity half is independently valuable; the execution half is the larger
blast radius.

## Decision

### D1 — Severity triage adjudication

**Semantics (approved)**: fail closed only when an actual cross-group sibling
exists (same namespace/clusterspace, same kind+name, different GVK). A unique
instance adjudicates normally; a Kind registered in a single group is unchanged.

1. **Fast path**: a Kind enumerating to one API group is never gated. Enumeration
   deliberately bypasses the static Kind→GVK table (the table answers with one
   well-known GVK and would hide the second group). Two enumerators exist:
   - **discovery-backed (preferred, production)**: `ServerGroupsAndResources`
     gives the exact Kind→plural→group→scope data, so irregular plurals
     (`Gateway`, `Endpoints`, `Policy`) resolve correctly and nothing is guessed.
     AF already builds this discovery client for its RESTMapper.
   - **RESTMapper-backed (fallback)**: used only where no discovery client is
     wired (tests, minimal deployments). It asks the mapper about a guessed
     plural resource name, so an irregular plural cannot be enumerated.
2. **Cannot-enumerate fails closed** (`ErrGroupEnumerationUnavailable`): no
   enumerator/client, discovery unavailable, the Kind not served at all, or (on
   the fallback path only) a plural the guess cannot derive. This is reported as
   `group_unscopable`, never as a silent gate skip.
3. **Multi-group Kind, hub-local target**: one existence GET per other group.
   A sibling found ⇒ `Ambiguous=true`, `AmbiguityReason="group_unscopable"`,
   `CandidateAPIGroups` listing the groups in which siblings were found.
4. **Resolution path**: the LLM-visible `confirmed_api_group` on each of the three
   AF tools maps to the internal `CreateRRArgs.ConfirmedAmbiguousAPIGroup`
   (`json:"-"`), normalized by `scope.NormalizeAPIGroup`.
   - confirming the **target's own** group bypasses the gate;
   - confirming a **sibling's** group yields the non-error
     `alert_belongs_to_sibling` result (severity must not be inherited);
   - the confirmation is honoured **before** the sibling probe, so an
     unavailable check remains resolvable by the user (no dead end).
5. **Per-reason adjudication**: `resolvesAmbiguity` switches on
   `AmbiguityReason`. `cluster_scoped_only` is resolvable only by
   `confirmed_signal_name`; `group_unscopable` only by `confirmed_api_group`.
   Neither confirmation class can bypass the other's gate, and an alert name is
   cleared from a group-unscopable candidate so it can never be used as a bypass.
   When both are supplied, the reason decides (there is exactly one reason).
6. **`APIGroupKnown`**: with a mandatory apiVersion the *core* group is a known
   group (a genuine sibling group for multi-group Kinds), so an empty `APIGroup`
   cannot by itself distinguish "core" from "no group information". The triager
   takes an explicit flag; production callers always set it.
7. **Fleet mode**: adjudicate as today plus a `sibling_check_unavailable`
   observation — no per-cluster existence reader exists in AF (`ClusterLister` is
   list-only). Documented deviation; a fleet reader is future work.
8. **Client choice**: the checker uses the direct (non-circuit-breaker) dynamic
   client, so a CB-open state cannot masquerade as "no sibling". The mapper is
   already discovery-backed and non-CB.
9. **Audit**: the decision is persisted (`severity_triage.ambiguous`) with
   `api_group`, `ambiguity_reason`, the candidate identity/severity and
   `candidate_api_groups`; `severity_triage.completed` carries `api_group` and the
   reconciled `source_tier`.
10. **Reason-driven messages**: the agent message and
    `AmbiguousSeverityError.Error()` both derive from `AmbiguityReason`; the
    system prompt gives one executing instruction per reason.
11. **Future lift**: an `api_group` alert label (removing the need for
    confirmation in the common case) is deferred; DD-AF-012's Tier 1.5/2 `Labels`
    compatibility note still applies.

### D2 — Execution identity format

One canonical identity, used by every persisted and user-visible surface:

```
namespace/group/version/Kind/name   # namespaced
group/version/Kind/name             # cluster-scoped (no empty segment)
```

- the **core** group renders as `core` (`prod/core/v1/Pod/api`);
- an apiVersion that could not be established renders `unresolved` in **both**
  the group and version slots (`prod/unresolved/unresolved/Route/web`), which can
  never equal a genuine core resource's identity;
- `scope.BuildTargetIdentity` composes it and `scope.ParseTargetIdentity` splits
  it, validating every slot (namespace DNS label, group DNS subdomain, version
  shape, Kind shape, name DNS subdomain) so a mis-split string is rejected
  rather than read as a different target.

The group and version are **ordinary path parts of the coordinates**, exactly as
Kubernetes identifies a resource (GVK + namespace + name). Nothing is annotated
onto the string: no `@`-style suffix, no out-of-band sentinel glued to a name.
Version is part of the identity (`autoscaling/v1` and `autoscaling/v2` are
different resources); the dedup fingerprint keeps its own group-only, version-
excluded rule (DD-GATEWAY-019), which is a deduplication decision, not an
identity one.

Consumers:

- **WFE spec** (`creator.ResolveTargetIdentity`): RCA `RemediationTarget` when it
  names kind+name, else `RR.Spec.TargetResource`; apiVersion precedence is the
  RCA target's, else the RR's. One resolver feeds the WFE string, the
  gate/lock input (`formatRemediationTargetString`) and the RO audit
  `target_resource`, so they cannot diverge. Documented exception: when the
  AIAnalysis carries no usable RCA remediation target,
  `formatRemediationTargetString` returns `""` while the WFE spec and audit
  key fall back to the RR target — the busy/cooldown gates and the
  approval/lock path then skip (no WFE matches an empty key), preserving the
  pre-#2658 behavior rather than changing gate semantics here.
- **WFE validator**: `scope.ParseTargetIdentity` is the gate; the CRD pattern is
  the 4-or-5-part alternation, so an over-long or legacy string is rejected.
- **Busy / cooldown / approval lock / distributed lock**: all keyed on this one
  string, so a cross-group or cross-version pair gets distinct keys and a
  genuine sibling can never be mistaken for the same target.
- **Parser**: `routing.parseTargetResource` returns the structured
  `TargetResource` (namespace/kind/name/apiVersion) from the identity.
- **Tekton**: labels sanitize the path separators; `TARGET_RESOURCE` carries the
  identity verbatim (bundle-facing contract change, accepted).
- **`ResourceIdentifier.String()`**: the same identity with the Kind's own
  casing; it is the `target_resource` value of `routing.blocked` audit events.
- **Notifications**: unchanged two-part `Kind/Name` (the notification contract's
  own shape); no group annotation is appended.
- **KA evidence handoff**: the filter compares the identity's **coordinates**
  (`scope.ParseTargetIdentity`) against the signal's namespace/kind/name, so a
  group-bearing history result is not misread as "another target's"; cross-target
  and cross-namespace history stays fail-closed.

### D3 — Migration stance

No migration and no backwards compatibility for WFE objects. **Accepted and
documented**: during an upgrade window, pre-upgrade WFEs (old-format key) are
invisible to the busy/cooldown/approval gates, so the busy gate fails **open** for
them, permitting a concurrent execution DD-WE-001/DD-RO-002 normally forbid.
Clean installs and short-lived WFEs make this acceptable. The audit-row re-key
breaks Tier-2 long-window chains once at upgrade — accepted. The dual-key
transition (query both formats for one release) was considered and rejected as
backwards-compat work.

### D4 — Gateway / audit identity

1. The RO `remediation.workflow_created` `target_resource` is the canonical
   identity (`creator.TargetIdentity.AuditKey`) — the same string the WFE and
   DataStorage use, group and version included as path parts.
2. GW `correlation_id` is `alertname:` + the canonical identity
   (`alertname:namespace/group/version/Kind/name`), built by the same
   `scope.BuildTargetIdentity` the rest of the system uses. Two siblings in
   different API groups therefore have distinct correlation IDs, and the
   remainder after the first `:` parses with `scope.ParseTargetIdentity`.
   (An intermediate version dropped the group from this string; that was
   rejected — it reinstated the group-blind ambiguity this record removes.)
3. BR-GATEWAY-058-A records the format and the reason. AF's audit correlation
   IDs are RR names / request ids, not readable signal identities, so the change
   does not touch them; any future readable AF id must use the same builder.

### D-B — DataStorage history parameter semantics

`targetApiVersion` is **required** on `getRemediationHistoryContext` and parsed
with `q.Has`: absent → 400, empty → 400. Empty is not a valid value because it is
indistinguishable from "unknown", which is a documented normal state at three
producers (RR target without apiVersion, RCA target `apiVersion` optional, KA
`EnrichRequest.APIVersion` empty when unknown). An empty parameter that silently
matched only core rows is exactly the fail-open this issue removes.

- **Wire spelling**: `v1` (or `core/v1`) for the core group, `group/version`
  otherwise, `unresolved` for the unknown state; `scope.WireAPIVersion`
  normalizes and rejects anything else (SI-10).
- **Composition**: the handler builds the same identity string the RO emitter
  writes (`scope.BuildTargetIdentity`) and the repository matches it with a
  **single equality predicate** on both the anchor and the recursive CTE term.
  Group and version being part of the key is what makes equality sufficient:
  no `LIKE`, no prefix index, no `@unresolved` residual, no sibling rows.
- **No migration**: the previous plan's `text_pattern_ops` expression index and
  its `@`-based sentinel are gone with the suffix.
- **Callers**: all four updated in the same change (RO routing adapter, AF agent
  history tool + params struct, KA enrichment params struct, KA
  `get_namespaced_resource_context` / `get_cluster_resource_context` with
  `api_version` in both LLM schemas). The KA resource-context tools key on the
  owner-chain root and carry its `apiVersion` (falling back to the input only
  when the chain is empty), and pass it to both the history query and the
  spec-hash computation.
- The break (required parameter + identity re-key) is deliberate under the
  approved no-backcompat stance.

## Dedup identity vs. narrative identity (why the vocabularies differ)

Three strings describe a target; each answers a different question, and they are
deliberately not the same string:

| String | Composition | Answers |
|---|---|---|
| Dedup fingerprint (`CalculateClusterAwareFingerprint`, GW + AF) | `sha256([clusterID:]group:namespace:kind:name)` | "is this the same object?" (must merge two observations of one resource) |
| Audit correlation ID (GW readable, pre-RR window only) | `alertname:` + canonical identity | "which incident narrative is this?" (must separate two alerts on one resource) |
| Canonical identity (WFE, RO audit, DataStorage, gates, locks) | `namespace/group/version/Kind/name` | "which exact resource coordinates?" |

**Version is in the identity and the correlation ID, but not in the fingerprint.**
Kubernetes serves one object under several versions (`autoscaling/v1` and
`autoscaling/v2` address the same HorizontalPodAutoscaler), so a version-bearing
hash would fingerprint one object twice — the alert path and the RR path would
miss each other's dedup and permit duplicate concurrent remediation. Group is in
the hash because two objects *can* share namespace/kind/name across groups
(`Route` in `route.openshift.io` vs `serving.knative.dev`), which is the defect
#2658 removes.

Known, accepted trade-off of group in the hash: during a deprecated-group
migration (e.g. `extensions` → `networking.k8s.io` Ingress) the *same* object can
be observed under two group spellings and therefore hash differently for the
duration of the migration. That window is transient and operator-visible;
group-blindness was a permanent, silent misidentification.

The correlation ID keeps the version because it only ever adds narrative lines
(two versions of one object = two audit rows), where merging them (the
fingerprint's behaviour) would be wrong for forensics. If a future change makes
the two vocabularies identical, it must choose which failure to accept: silent
sibling collisions (version dropped from the hash) or duplicate remediation of one
object (version added to the hash).

## Canonicalization

`scope.ParseAPIVersion` is the single choke point: `core/v1` canonicalizes to
`("", "v1")`, and every other `core/<version>` spelling is rejected with a named
reason. The alias set is closed so `core/v2` cannot bypass the group gate for a
core target. `NormalizeAPIGroup` is the 3-state normalizer
(`StateResolvedGroup | StateScopeUnknown | StateInvalid`) used by confirmations,
the DS parameter and the identity builders; no consumer branches on a sentinel
group string. `ResolveGVKWithAPIVersion` delegates to `ParseAPIVersion` so the
effectiveness-monitor path inherits canonicalization (see DD-K8S-001).

The AF tool gate (`validate.APIVersion`) rejects `core/<v>` explicitly with a
"use v1 for core resources" message, so the raw spelling is never persisted.

## Rejected alternatives

| Alternative | Why rejected |
|---|---|
| Alert-label lockstep (`api_group` label on every alert) now | Requires coordinated rule-authoring changes across every monitored cluster before any benefit; deferred as D1.11 |
| Discovery-only gate (no existence check) | A Kind registered in several groups is common and perfectly healthy; gating on cardinality alone blocks every Ingress |
| Alert-name confirmation for group ambiguity | An alert name says nothing about which group the alert's resource lives in — unsound as a group resolver |
| `SignalNameOverride` as the resolution | Retracted: it does not bypass triage, so it cannot resolve a severity ambiguity |
| Dual-key WFE transition (query both formats for one release) | Backwards-compat work for short-lived objects; the fail-open window is documented instead (D3) |
| Suffix-annotated identity (`ns/kind/name@group`, the v12 plan's D2) | Chosen during planning to keep the old 2/3-part grammar untouched, but a group glued onto a name is an annotation, not a field; replaced by the canonical `namespace/group/version/Kind/name` path (maintainer decision, 2026-11-18) |
| Dual key in the DS join | With group+version inside the key, one equality predicate is both precise and simple; a second key would only re-admit sibling rows |

## Consequences

**Positive**: cross-group pairs are distinguishable in severity decisions,
execution locks, audit traces and history queries; the audit trail records *why*
a decision was ambiguous; an unavailable check is visible rather than silently
permissive.

**Negative / accepted**: one re-key of WFE identities and DS history joins at
upgrade (D3); `TARGET_RESOURCE` value change for bundles; an additional existence
GET per other API group during triage (one per group, cached discovery).

**Operational**: AF needs GET permission on the additional candidate groups'
resources for the existence check (RBAC for these GETs is **unverified in-repo** —
no AF RBAC manifests live in this repository; must be confirmed at deployment).
With the discovery-backed enumerator (production default) irregular plurals are
enumerated exactly; the fail-closed irregular-plural case remains only on the
mapper fallback path, where the ambiguity is still resolvable by confirming the
target's own API group.

## Prerequisite

PR #2653 delivers the group-aware `CalculateClusterAwareFingerprint` (group-only
segment, version-excluded, groupless byte-identity), `CheckExistingRRArgs.api_version`
and the `validate/k8s.go` → `scope` delegation. It is the merge-order prerequisite
for execution-half E2E. RO-side integration tests construct RemediationRequests
directly and do not depend on it.
