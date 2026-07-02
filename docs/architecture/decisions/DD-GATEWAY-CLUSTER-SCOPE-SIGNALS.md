# DD-GATEWAY-CLUSTER-SCOPE-SIGNALS: Cluster-Scoped Signal Support

**Status**: Proposed
**Decision Date**: 2026-07-01
**Version**: 1.0
**Confidence**: 90%
**Deciders**: Architecture Team
**Applies To**: Gateway (GW), SignalProcessing (SP), Kubernaut Agent (KA), Remediation Orchestrator (RO)

**Related Business Requirements**:
- BR-GATEWAY-19x (TBD): Cluster-Scoped Signal Support -- Gateway admits Kind-unresolved signals as a distinct category with collision-free fingerprinting
- Extends BR-GATEWAY-TARGET-RESOURCE-VALIDATION
- BR-SCOPE-001: Resource Scope Management -- Opt-In Model
- BR-SCOPE-002: Gateway Signal Filtering
- References BR-SP-112: Cluster-Scoped Label Exposure (narrower -- covers *known* cluster-scoped kinds like Node/PV/ClusterRole, not *unresolvable* Kind)

**Related Design Decisions**:
- ADR-053: Resource Scope Management Architecture

**Related Issues**:
- #1521: Cluster-Scoped Signal Support (planning/tracking issue)
- #673: Gateway `validateResourceInfo` failure surfaces as HTTP 500 instead of 400
- #524: Conditional `TARGET_RESOURCE_*` parameter injection based on workflow-schema declaration
- #1110 / BR-SP-112: Cluster-scoped label exposure for known Kinds

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-07-01 | Architecture Team | Initial design: `ClusterScope` pseudo-Kind detection/representation/fingerprint (from issue #1521), plus the cluster-wide opt-in mechanism (label + allowlist annotation on `kube-system`) closing a gap not covered in the original issue. |

---

## Context & Problem

### Current State

Gateway rejects any alert whose target `Kind` cannot be resolved to a real Kubernetes API type (e.g. `Watchdog`, `ClusterNotUpgradeable`-style alerts that carry no per-resource label). `PrometheusAdapter.extractTargetResource` already *detects* these alerts and tags them with the sentinel `Kind="Unknown"`, `Name="unknown"` (`pkg/gateway/adapters/prometheus_adapter.go:453-515`), but `CRDCreator.validateResourceInfo` (`pkg/gateway/processing/crd_creator.go:544-560`) hard-rejects them. Net effect: these signals are silently dropped today, even though Alertmanager and OCP predefined alerts both support genuinely resourceless alerts as a normal pattern.

### Problem Statement

Admit Kind-unresolvable alerts through the pipeline for investigation/notification (and, where a real remediation target legitimately exists, all the way to automated remediation) without:

1. Fingerprint collisions between distinct alert types in the same namespace (the fingerprint formula deliberately excludes `alertname` per BR-GATEWAY-004, so `SHA256(ns:Unknown:unknown)` is identical for every such alert today)
2. Creating invalid `WorkflowExecution`s against workflows that require a real resource
3. Exposing an ambiguous, spoofable string sentinel (`Kind == "" || EqualFold(Kind, "unknown")`) to operators or the LLM
4. **Silently expanding remediation scope with no way for operators to opt in or control which specific resourceless alert types are trusted** (identified during this design review -- not covered in the original issue's wiring manifest)

### Downstream Tolerance (Already Verified)

- **SignalProcessing**: `K8sEnricher.Enrich`'s `default` branch (`pkg/signalprocessing/enricher/k8s_enricher.go:117-155,391-408`) already gracefully degrades any unmatched Kind to `enrichNamespaceOnly`, returning `DegradedMode=true` with no error, even when namespace is also empty.
- **KubernautAgent**: `InjectRemediationTarget`/`InjectTargetResourceParameters` (`internal/kubernautagent/investigator/investigator_phases.go:403-487`) already no-ops gracefully when `Kind == ""`.
- **AIAnalysis/RemediationOrchestrator**: a full notify-only pathway exists requiring no target resource at all (`Actionability="NotActionable"`, `ReasonWorkflowNotNeeded`), routed straight to `Outcome="NoActionRequired"` or a `NotificationRequest` without ever creating a `WorkflowExecution`.
- **WorkflowExecution**: `TargetResource` is a hard-required 2-3 part locking string (`DD-WE-001`), not necessarily a literal API object the Job executor operates on -- no resourceless exception needed.

This is a smaller change than "support across all services" initially sounded like -- most new logic is concentrated in Gateway.

---

## Decision Drivers

1. **Safety over convenience**: resourceless alerts can, per the existing conditional-injection design (#524), legitimately reach real `WorkflowExecution`s when a cluster-wide-compatible workflow is selected. The opt-in mechanism must not let this happen by accident.
2. **Reuse over invention**: prefer existing fingerprint/CRD/scope-management primitives over new fields, new CRDs, or new formulas.
3. **No silent scope expansion**: any mechanism touching `kubernaut.ai`-labeled opt-in must not alter the managed/unmanaged status of unrelated, already-labeled resources (ADR-053).
4. **Deny-by-default**: every gate in the admission path (Kind resolution, scope opt-in, workflow-schema match) fails closed, not open.

---

## Alternatives Considered

### Part 1: Detection -- Typed Result vs. String Sentinel

#### Alternative 1A: Keep `Kind="Unknown"` string sentinel, relax `validateResourceInfo` -- REJECTED

**Cons**: Perpetuates string-sentinel fragility (an operator or adapter bug could produce a literal `"Unknown"` Kind and be silently treated as intentionally resourceless); no clean way to distinguish "intentionally cluster-scoped" from "extraction bug."

**Confidence**: 10% (rejected)

#### Alternative 1B: Typed `(kind, name string, resolved bool)` result from `extractTargetResource` -- CHOSEN

**Approach**: Replace the string-sentinel check with a typed result from the existing candidate-scoring logic in `extractTargetResource`.

**Pros**: Closes the "operator overwrites Unknown" spoofing/robustness concern; `resolved=false` becomes an explicit, non-ambiguous signal.

**Confidence**: 95%

### Part 2: Representation -- Pseudo-Kind vs. New TargetType

#### Alternative 2A: New `TargetType` enum value (`"cluster"`) -- REJECTED

**Cons**: These signals are genuinely Kubernetes-origin; conflating with the external-cloud-provider meaning of `TargetType` (per `DD-GATEWAY-NON-K8S-SIGNALS`) would be confusing.

**Confidence**: 10% (rejected)

#### Alternative 2B: Reserved pseudo-Kind constant `"ClusterScope"` -- CHOSEN

**Approach**: When detection is `resolved=false`: `TargetResource.Kind = "ClusterScope"`, `TargetResource.Name = <alertname>`, `TargetResource.Namespace = <extracted namespace, or empty>`. No new CRD fields needed -- `ResourceIdentifier.Kind`/`.Name` have no CRD-level required/enum constraint today (`api/remediation/v1alpha1/remediationrequest_types.go:398-418`), only Go-level validation.

**Pros**: No CRD schema change; fingerprint reuses `CalculateClusterAwareFingerprint` unchanged by feeding it `{Namespace, Kind:"ClusterScope", Name:alertname}` directly, bypassing the owner-chain walk entirely (no owner chain to resolve for a pseudo-object). Result: `SHA256(clusterID:namespace:ClusterScope:alertname)` -- no new fingerprint formula needed.

**Confidence**: 95%

### Part 3: Cluster-Wide Opt-In Mechanism for `ClusterScope` Signals

#### The Gap

`ClusterScope` pseudo-resources have no backing K8s object and frequently no namespace. `scope.Manager.IsManagedResource` has nothing to check a label on: `checkResourceLabel` skips unknown kinds (`kind` not in `kindToGroup`), and `namespace == ""` immediately short-circuits to the cluster-scoped-unmanaged-default branch (`pkg/shared/scope/manager.go:177-180`). Under Alternatives 1B/2B alone, there is no way for an operator to opt a cluster into `ClusterScope` signals at all -- every one would be rejected as unmanaged, defeating the purpose of admitting them in the first place.

#### Alternative 3A: Reuse `kubernaut.ai/managed` on `Namespace/kube-system` -- REJECTED

**Approach**: When `Kind == "ClusterScope"`, substitute the identity check to `{Kind: "Namespace", Name: "kube-system"}` and reuse the existing `kubernaut.ai/managed` label/hierarchy unchanged.

**Pros**: Zero new label key, zero new code path -- `Manager.IsManagedResource` already handles `Namespace` as a first-class kind. `kube-system` is a guaranteed-to-exist, undeletable, cluster-scoped singleton in every conformant Kubernetes cluster.

**Cons**: `kube-system` is a **real** namespace running real system workloads (CoreDNS, kube-proxy, etc.). ADR-053's 2-level hierarchy treats a namespace label as inheritance for every unlabeled resource in that namespace. Labeling `kube-system` as `kubernaut.ai/managed=true` would silently make every unlabeled system pod in it eligible for automated remediation -- a scope expansion the operator almost certainly did not intend when opting into `Watchdog`-style cluster alerts.

**Confidence**: 10% (rejected -- unacceptable blast radius)

#### Alternative 3B: Dedicated kubernaut-owned cluster-scoped CRD singleton -- DEFERRED

**Approach**: Introduce a new cluster-scoped CRD (e.g. `KubernautClusterConfig`) with exactly one instance carrying explicit fields for cluster-scope-signal configuration.

**Pros**: Most self-documenting/explicit option; room to grow into a general cluster-level config surface later.

**Cons**: New CRD + new cluster-scoped RBAC + install-manifest changes + its own controller/validation -- disproportionate lift for what is currently a two-field flag. Per `AGENTS.md` CHECKPOINT DD, a new CRD interaction pattern requires its own dedicated design-decision review -- scope creep for this DD. Delays #1521, which is explicitly scoped as design-only with no implementation yet.

**Confidence**: N/A (deferred -- revisit only if a richer cluster-level config surface becomes a real requirement)

#### Alternative 3C: Single boolean label, no per-alert granularity -- REJECTED

**Approach**: `kubernaut.ai/cluster-managed=true` (or similarly named) admits *any* `ClusterScope` signal once set, via a label decoupled from the `kubernaut.ai/managed` inheritance chain.

**Cons**: The RO workflow-schema-aware gating logic (see Decision below) allows a `ClusterScope` signal to reach a real `WorkflowExecution` when a cluster-wide-compatible workflow is selected -- so a blanket switch isn't just enabling notifications, it can enable automated remediation for alert types the operator never individually reviewed (e.g. `ClusterNotUpgradeable` alongside benign `Watchdog` heartbeats). Also squats the `cluster-managed` name on a narrow feature, foreclosing a future broader "manage everything" mode under that name.

**Confidence**: 30% (rejected -- insufficient granularity for a category that includes alerts capable of reaching real remediation)

#### Alternative 3D: Master switch label + mandatory per-alert allowlist annotation, explicit wildcard for "all" -- CHOSEN

**Approach**: Decouple the mechanism entirely from `kubernaut.ai/managed`:

| Field | Type | Key | Semantics |
|---|---|---|---|
| Master switch | Label | `kubernaut.ai/cluster-scope-signals=true` | Necessary but not sufficient |
| Allowlist | Annotation | `kubernaut.ai/cluster-scope-alerts` | **Mandatory** once the switch is `true`. Comma-separated `alertname` values, or the literal wildcard `"*"` |

Both live on `Namespace/kube-system`, read via a new, narrow function (`Manager.IsClusterScopeSignalAllowed(ctx, alertname)`) that is **not** part of the `IsManagedResource` 2-level hierarchy and has zero effect on namespace-inheritance for real workloads in `kube-system`.

**Precedence** (deny-by-default at every step):
1. Switch label absent or `!= "true"` -> reject (unchanged 400 rejection path, same UX as today)
2. Switch `true`, annotation absent or empty -> reject -- no implicit "allow all," nothing is admitted until explicitly whitelisted
3. Switch `true`, annotation `== "*"` -> admit any `ClusterScope` `alertname` (explicit, auditable opt-in to everything -- the operator consciously typed the wildcard; it is never a default)
4. Switch `true`, annotation `== "Watchdog,ClusterNotUpgradeable"` (etc.) -> admit only listed `alertname` values; anything else rejected even with the switch on

```bash
# Enable only Watchdog (safe heartbeat, never resolves a real target)
kubectl label namespace kube-system kubernaut.ai/cluster-scope-signals=true
kubectl annotate namespace kube-system kubernaut.ai/cluster-scope-alerts="Watchdog"

# Opt into everything explicitly
kubectl annotate namespace kube-system kubernaut.ai/cluster-scope-alerts="*" --overwrite
```

**Pros**:
- Deny-by-default at every layer -- no accidental blanket admission
- Fine-grained control over exactly which alert names are trusted to potentially reach real remediation, vs. notification-only heartbeats
- Avoids the Kubernetes label-value constraint -- label values must be <=63 characters and may only contain alphanumerics, `-`, `_`, `.` (no commas), per the [official Kubernetes labels documentation](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set) -- by putting the variable-length list in an annotation. Annotation values have no character-set restriction and a combined 256 KiB budget per object, per the [official Kubernetes annotations documentation](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/) -- effectively unlimited headroom for a list of alert names.
- Loses no functionality versus a label-only design: the lookup is always a direct `client.Get(Namespace{Name: "kube-system"})`, never a label-selector `List()`/`Watch()`, so there is no need for the allowlist to be selector-queryable.
- `kubernaut.ai/cluster-scope-signals` naming stays scoped to this feature, leaving `kubernaut.ai/cluster-managed` free for a future broader wildcard-remediation mode under a different name.
- No new CRD, no new controller, no new install-time object. Zero new RBAC: both Gateway's `ClusterRole` (`deploy/gateway/01-rbac.yaml:35-43`) and the shared-manager `ClusterRole` (`config/rbac/role.yaml:6-17`, generated from `internal/controller/signalprocessing/signalprocessing_controller.go:109`) already grant cluster-wide `get;list;watch` on `namespaces`.

**Cons**:
- Two fields to document/reason about instead of one -- mitigated: switch + allowlist is a well-understood two-tier pattern, and omitting the annotation entirely is the correct fail-safe (nothing is ever admitted)
- Annotation values aren't validated by the K8s API schema (any string is legal) -- Gateway must defensively parse the CSV/wildcard and reject malformed values with a clear error rather than silently failing open or closed

**Confidence**: 90%

**Grounding for `kube-system` as the cluster-scoped anchor** (validated against primary sources):

1. **Kubernetes upstream** ([`kubernetes/kubernetes#77487`](https://github.com/kubernetes/kubernetes/issues/77487), 2019): API-machinery lead Daniel Smith (`@lavalamp`) -- *"Current best practice is to use the kube-system UID"* -- in direct response to "Is there a way to find a unique id for kubernetes cluster?"
2. **OpenTelemetry semantic conventions** (ratified spec, [`k8s.cluster.uid`](https://opentelemetry.io/docs/specs/semconv/resource/k8s/)): *"A pseudo-ID for the cluster, set to the UID of the `kube-system` namespace... will exist for the lifetime of the cluster... will only change if the cluster is rebuilt."*
3. **Production implementation** ([`open-telemetry/opentelemetry-collector-contrib#21974`](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/21974) -> merged PR #23668): the `k8sattributes` processor ships this convention in real deployments, sourced explicitly from #1 and #2.
4. **Independent adopter confirmation** ([`PrefectHQ/prefect#9851`](https://github.com/PrefectHQ/prefect/issues/9851)): confirms the same rationale ("`kube-system` namespace always exists -- cannot be deleted -- and has a unique ID"), citing a public endorsement from Kubernetes networking SIG lead Tim Hockin (`@thockin`). Also documents an RBAC failure mode (`namespaces "kube-system" is forbidden`) independently verified **not** to apply to Kubernaut, since both `ClusterRole`s above already grant cluster-wide `namespaces` access.

None of these sources describe *labeling* `kube-system` for feature opt-in specifically -- that pattern (Alternative 3D) is Kubernaut's own design. What they establish is the narrower, load-bearing fact this decision depends on: `kube-system` is the community-recognized, upstream-endorsed proxy for "the cluster as a singleton object," which is the property that makes it a valid label/annotation anchor.

---

## Decision

**Chosen: Alternative 1B (typed detection) + Alternative 2B (`ClusterScope` pseudo-Kind) + Alternative 3D (master switch label + mandatory allowlist annotation on `kube-system`).**

### Scope Boundary: WFE Is Conditionally Reachable, Not Categorically Excluded

`InjectRemediationTarget` already lets the LLM's own investigation *override* the signal's root identity. `ClusterScope` is only the signal-level identity used for fingerprinting/dedup/opt-in at ingestion -- it does not bind what the LLM ultimately investigates or recommends:

- **`Watchdog`-style heartbeats**: by construction there's never a real target to find. `RemediationTarget.Kind` stays `ClusterScope`; outcome is `NotActionable`/`WorkflowNotNeeded`.
- **`ClusterNotUpgradeable`-style alerts**: genuinely different -- no per-resource label on the alert, but investigation is likely to resolve a real object (a specific `ClusterOperator`, a stuck `Node`, a blocking `CertificateSigningRequest`). WFE's existing 2-part format (`{kind}/{name}`) handles these with zero new code once resolved.

**Gating logic must not conflate "no resource resolved" with "no remediation possible."** The question is not "did investigation resolve a Kind" but "does the *selected workflow* require a specific resource, and if so, was one found?":

| Selected workflow requires `TARGET_RESOURCE_KIND`/`NAME`? | `RemediationTarget.Kind` resolved to a real object? | Outcome |
|---|---|---|
| Yes | No (still `ClusterScope`) | Genuine mismatch -> force `WorkflowNotNeeded`/`NeedsHumanReview` |
| No (cluster-scope-compatible workflow) | No | `ClusterScope`/`<alertname>` is a legitimate `WFE.Spec.TargetResource` locking string -> proceed normally |
| Yes / No | Yes (e.g. `ClusterOperator/etcd`) | Already-working path -> proceed normally |

**Defense in depth -- `ClusterScope` must never be presented to, or treated by, the LLM as a real fetchable Kind, at two independent layers:**

1. **Prompt-level**: when `TargetResource.Kind == "ClusterScope"`, omit the structured target-resource section entirely (showing `Kind: ClusterScope` invites a `get_resource_context(kind="ClusterScope")` tool call that 404s) and substitute a neutral statement: *"This alert is a cluster-wide condition not tied to any specific Kubernetes resource."* Must not presuppose the outcome (must not imply "no remediation exists").
2. **Code-level**: the RO workflow-schema-aware guard (table above) is the mandatory backstop regardless of prompt compliance.

### Admission Flow

```
Alert (no resource label)
    │
    ▼
extractTargetResource() -> resolved=false          [Alternative 1B]
    │
    ▼
Construct pseudo-resource: Kind="ClusterScope",
  Name=<alertname>, Namespace=<extracted-or-empty>  [Alternative 2B]
    │
    ▼
Scope check: IsClusterScopeSignalAllowed(ctx, alertname)  [Alternative 3D]
    │  reads Namespace/kube-system:
    │    label  kubernaut.ai/cluster-scope-signals
    │    annot  kubernaut.ai/cluster-scope-alerts
    │
    ├─ not allowed -> reject (400, same path as today's validateResourceInfo)
    │
    ▼ allowed
Fingerprint via CalculateClusterAwareFingerprint(
  {Namespace, Kind:"ClusterScope", Name:alertname})   [owner-chain walk bypassed]
    │
    ▼
RR created -> SP (degraded-mode enrichment, existing code)
    │
    ▼
KA investigation (prompt omits structured Kind/Name)  [defense-in-depth layer 1]
    │
    ▼
RO WFE-creation gate: workflow-schema-aware guard      [defense-in-depth layer 2]
    │
    ├─ workflow needs resource, none resolved -> WorkflowNotNeeded / NeedsHumanReview
    ├─ workflow needs no resource -> WFE with ClusterScope/<alertname> locking key
    └─ resource resolved by investigation -> normal path
```

### Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| Typed Kind-resolution result (replaces string sentinel) | Gateway signal ingestion | `pkg/gateway/adapters/prometheus_adapter.go` `extractTargetResource` | IT-GW-CLS-001 |
| `ClusterScope` pseudo-resource construction | Gateway signal ingestion | `pkg/gateway/adapters/prometheus_adapter.go` (adapter `Parse`) | IT-GW-CLS-002 |
| Fingerprint bypass for `ClusterScope` (skip owner-resolution) | Gateway fingerprint computation | `pkg/gateway/types/fingerprint.go` (branch in `ResolveFingerprintWithCluster` or new thin wrapper) | IT-GW-CLS-003 |
| `validateResourceInfo` relaxation for `ClusterScope` category | Gateway CRD creation | `pkg/gateway/processing/crd_creator.go:528-560` | IT-GW-CLS-004 |
| **Cluster-wide opt-in check (`IsClusterScopeSignalAllowed`)** | **Gateway signal ingestion, before CRD creation** | **`pkg/shared/scope/manager.go` (new function) + call site in `pkg/gateway/processing/crd_creator.go`** | **IT-GW-CLS-005** |
| SP degraded-mode path verification for `ClusterScope` Kind | SignalProcessing enrichment | `pkg/signalprocessing/enricher/k8s_enricher.go:391-408` (existing code, new test coverage) | IT-SP-CLS-006 |
| KA prompt: omit structured Kind/Name, substitute neutral natural-language statement | KubernautAgent investigation prompt construction | `internal/kubernautagent/prompt/` (prompt/system-message builder) | IT-KA-CLS-007 |
| RO workflow-schema-aware guard | RemediationOrchestrator WFE creation gate | `pkg/remediationorchestrator/creator/workflowexecution.go` (`resolveTargetResource`) | IT-RO-CLS-008 |
| RO routing confirmation (`WorkflowNotNeeded` path reachable end-to-end, including via the new guard) | RemediationOrchestrator | `pkg/remediationorchestrator/handler/aianalysis.go:136-200` (existing code, new E2E test) | E2E-CLS-009 |

---

## Consequences

### Positive Consequences

1. `Watchdog`/`ClusterNotUpgradeable`-style alerts gain a real, collision-free admission path instead of being silently dropped
2. No new CRD fields, no new fingerprint formula -- reuses `CalculateClusterAwareFingerprint` and the existing `ResourceIdentifier` schema
3. Cluster-wide opt-in is deny-by-default at every layer (Kind resolution, scope check, workflow-schema gate) -- no path silently expands remediation scope
4. Zero new RBAC required for the opt-in check (existing `ClusterRole`s already grant cluster-wide `namespaces` access)
5. `kubernaut.ai/managed` semantics for real workloads in `kube-system` remain completely unaffected

### Negative Consequences

1. Two new label/annotation keys for operators to learn (`kubernaut.ai/cluster-scope-signals`, `kubernaut.ai/cluster-scope-alerts`)
   - **Mitigation**: documented together as a single "cluster-wide signals" feature, with `kubectl` examples
2. Annotation values are unvalidated by the K8s API schema
   - **Mitigation**: Gateway parses defensively and rejects malformed values with a clear 400 error rather than failing open or closed silently
3. GW cluster-scope fingerprint fix changes existing dedup keys for previously-rejected-then-relaxed alert types
   - **Mitigation**: brief dedup window on rollout is acceptable; no persisted state to migrate since these signals were previously rejected outright

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Custom alerting rule with no resource labels, not in the allowlist | Low | Alert dropped by GW (expected -- deny-by-default) | Documentation: recommend including at least one resource label in custom rules; explicit allowlist entry if intentional |
| AF/LLM cannot determine target for cluster-scoped alert | Low | LLM falls back to broader investigation | `list_alerts` provides context; prompt instructs use of kubectl to find affected resources |
| Operator sets wildcard (`"*"`) without understanding the WFE-reachability implication | Medium | Broader-than-intended automated remediation eligibility | Documentation must explicitly call out that `"*"` can reach real `WorkflowExecution`s, not just notifications; recommend explicit per-alert lists |

---

## Compliance

| Requirement | Status | Notes |
|-------------|--------|-------|
| BR-GATEWAY-19x (TBD) | Planned | Cluster-Scoped Signal Support end-to-end (GW admission through RO gating) |
| BR-SCOPE-001 | Extended | New `IsClusterScopeSignalAllowed` path is additive; existing `IsManagedResource` 2-level hierarchy is unmodified |
| BR-SCOPE-002 | Extended | Gateway signal filtering gains the `ClusterScope` category |

---

## Validation Strategy

1. **Unit tests**: `extractTargetResource` typed-result behavior; `IsClusterScopeSignalAllowed` precedence (switch absent/false, annotation absent/empty, wildcard, explicit list, malformed value)
2. **Integration tests**: IT-GW-CLS-001..005 (Gateway), IT-SP-CLS-006 (SignalProcessing degraded mode), IT-KA-CLS-007 (prompt omission), IT-RO-CLS-008 (workflow-schema guard)
3. **E2E tests** (per original issue #1521 implementation plan): (a) `Watchdog`-style signal reaches `NotificationRequest`/`NoActionRequired` with no WFE created; (b) cluster-upgrade-failure-style signal with a real resolved target proceeds normally to WFE; (c) cluster-upgrade-failure-style signal with no resolved object but a cluster-wide workflow selected proceeds to WFE using `ClusterScope`/`<alertname>` as the locking target

---

## References

- Issue #1521: Cluster-Scoped Signal Support: admit Kind-unresolvable alerts (Watchdog, ClusterNotUpgradeable) through the pipeline
- #673: Gateway `validateResourceInfo` failure surfaces as HTTP 500 instead of 400
- #524: Conditional `TARGET_RESOURCE_*` parameter injection based on workflow-schema declaration
- #1110 / BR-SP-112: Cluster-scoped label exposure for known Kinds
- `pkg/shared/scope/manager.go`: existing 2-level scope hierarchy (ADR-053)
- `deploy/gateway/01-rbac.yaml:35-43`, `config/rbac/role.yaml:6-17`: existing cluster-wide `namespaces` RBAC grants
- [Kubernetes Labels and Selectors -- syntax and character set](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set)
- [Kubernetes Annotations -- syntax and character set](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/)
- [`kubernetes/kubernetes#77487`](https://github.com/kubernetes/kubernetes/issues/77487) -- "Unique Id for cluster"
- [OpenTelemetry semantic conventions -- `k8s.cluster.uid`](https://opentelemetry.io/docs/specs/semconv/resource/k8s/)
- [`open-telemetry/opentelemetry-collector-contrib#21974`](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/21974) -- production implementation
- [`PrefectHQ/prefect#9851`](https://github.com/PrefectHQ/prefect/issues/9851) -- independent adopter confirmation + RBAC pitfall

---

**Document Version**: 1.0
**Last Updated**: 2026-07-01
