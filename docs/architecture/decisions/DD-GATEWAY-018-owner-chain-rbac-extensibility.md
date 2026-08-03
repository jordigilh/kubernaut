# DD-GATEWAY-018: Owner-Chain-Resolution RBAC — Minimal Universal Defaults + Bring-Your-Own ClusterRole

## Status

**✅ ACCEPTED** (2026-08-02)
**Last Reviewed**: 2026-08-02
**Confidence**: 95%

---

## Context & Problem

### Problem Statement

Gateway, EffectivenessMonitor (EM), and KubernautAgent (KA) all perform owner-chain resolution — walking Kubernetes owner references to correlate a Pod/alert-label resource kind up to its owning controller (Deployment, OLM `Subscription`, Istio `AuthorizationPolicy`, ArgoCD `Application`, etc.) for fingerprinting and investigation. When the service's own ServiceAccount lacks `get`/`list`/`watch` on a resource kind that appears in this chain, resolution fails and the signal is **silently dropped** — no `RemediationRequest` is ever created (issue [#1069](https://github.com/jordigilh/kubernaut/issues/1069)).

`kubernaut-operator` closed this gap for Gateway/EM by hardcoding a broad, ever-growing `PolicyRule` list (`ownerChainResolutionRules()`, `internal/resources/rbac.go:708-727`) covering PDB, OLM, Istio, cert-manager, ArgoCD, OpenShift Routes, and KubeVirt/CDI — unconditionally merged into both ClusterRoles. This repo's own Helm chart has **no such list at all for Gateway** (confirmed empty on both `main` and `release/v1.5`) and only partial coverage for EM/KA.

Simply porting the operator's hardcoded list into the Helm chart (the originally-proposed fix) was challenged during design review on two grounds:

1. **Doesn't scale.** The set of ecosystem CRDs a cluster might run is effectively unbounded (Kafka/Strimzi, Knative, arbitrary custom app CRDs, ...). A hardcoded list — no matter how large — always has a next gap, requiring a Kubernaut code change and release for every new ecosystem an operator wants covered.
2. **Least-privilege / compliance conflict.** Unconditionally granting cluster-wide read on OLM/Istio/cert-manager/ArgoCD/KubeVirt CRDs forces permission surface onto clusters that don't run those ecosystems and may not want Kubernaut watching them at all, even if unused — in tension with this project's existing least-privilege patterns (namespace-scoped RBAC toggle, #1686/BR-RBAC-020) and FedRAMP AC-6 (least privilege).

### Key Requirements

1. Let operators define exactly which additional resource kinds Gateway/EM/KA can read, without Kubernaut needing to enumerate or maintain an ecosystem list.
2. Must not require enumerating a fixed set of toggles — the set of possible resource kinds is unbounded.
3. Must not regress the low-friction "it just works" experience for the most universal, non-ecosystem-specific case that caused the original signal-drop bug (PDB).
4. Declarative / GitOps-friendly: expressed in the same `values.yaml`/CR as the rest of the deployment, with automatic lifecycle management (pruning on removal) and validation feedback (does the referenced ClusterRole actually exist).

### Trigger

Issue [#1069](https://github.com/jordigilh/kubernaut/issues/1069) (RBAC gaps silently drop signals) + design review pushback on the initially-proposed fix (porting the operator's hardcoded ecosystem list verbatim into the Helm chart).

---

## Alternatives Considered

### Alternative 1: Port the operator's hardcoded ecosystem list into the Helm chart verbatim (status quo direction)

**Approach**: Copy `ownerChainResolutionRules()` unconditionally into Gateway's and EM's Helm-chart ClusterRoles, matching what the operator already does.

**Pros**:
- ✅ Zero setup burden for the ecosystems already covered — works out of the box
- ✅ Minimal implementation effort, reuses an already-written, already-tested rule set

**Cons**:
- ❌ Unconditionally grants list/watch on OLM/Istio/cert-manager/ArgoCD/KubeVirt/Routes CRDs even on clusters that don't run those ecosystems or don't want Kubernaut watching them — least-privilege violation, larger audit surface
- ❌ Doesn't scale: any resource kind outside this fixed list (Kafka/Strimzi, Knative, custom app CRDs) still requires a Kubernaut code change + release to support

**Confidence**: 40% (rejected)

---

### Alternative 2: Per-ecosystem opt-in/opt-out toggles

**Approach**: One boolean per known ecosystem (`ownerChainResolution.olm.enabled`, `.istio.enabled`, `.certManager.enabled`, `.argocd.enabled`, `.kubevirt.enabled`, `.routes.enabled`), each independently gating its `PolicyRule` block.

**Pros**:
- ✅ Precise, least-privilege — operators disable exactly the ecosystems they don't run
- ✅ Still zero-setup for the ecosystems an operator does want

**Cons**:
- ❌ Same fundamental scaling problem as Alternative 1, just discretized into more knobs — still covers nothing outside the toggled list (Kafka, Knative, custom CRDs remain unreachable without a code change)
- ❌ Adds 5-6+ new schema fields × up to 3 services to design, document, and test, for a problem that's open-ended by nature

**Confidence**: 35% (rejected)

---

### Alternative 3a: Bind Gateway to the built-in `view` ClusterRole (matches existing #545 pattern)

**Approach**: Add a `gateway-view` `ClusterRoleBinding` binding Gateway's ServiceAccount to Kubernetes' built-in aggregated `view` ClusterRole — the exact pattern RemediationOrchestrator and EffectivenessMonitor already use (issue [#545](https://github.com/jordigilh/kubernaut/issues/545), `docs/tests/545/TEST_PLAN.md`, `effectivenessmonitor-view` / `remediationorchestrator-view` bindings).

**Discovery during spike**: Kubernetes' built-in `view` ClusterRole aggregates rules from any `ClusterRole` labeled `rbac.authorization.k8s.io/aggregate-to-view: "true"` — a live, controller-managed mechanism, not a static list (confirmed against `kubernetes/kubernetes` upstream source; PDB read was added to `view` via [kubernetes/kubernetes#52654](https://github.com/kubernetes/kubernetes/pull/52654) and ships in the `system:aggregate-to-view` bootstrap ClusterRole). "Well-behaved" ecosystem operators (cert-manager, Istio, KubeVirt) publish their own aggregated ClusterRoles with this label, so `view` transparently gains read access to their CRDs — **only on clusters where those operators are actually installed**. This is a stronger least-privilege property than a static hardcoded list: the effective permission set is contingent on what's actually present on a given cluster, not on what Kubernetes might install elsewhere.

**Pros**:
- ✅ Directly fixes #1069's concretely-reported symptom (Gateway `poddisruptionbudgets.policy is forbidden`) — `view` includes PDB by default
- ✅ Zero new schema/config surface — a single `ClusterRoleBinding`, identical in shape to two already-shipped, already-tested bindings in this exact codebase
- ✅ Gains cert-manager/Istio/KubeVirt coverage "for free" via aggregation, self-limited to ecosystems actually installed on that cluster
- ✅ Lowest possible implementation risk — copies a reviewed, tested, already-in-production pattern verbatim

**Cons**:
- ❌ Does not cover ecosystems that don't ship `aggregate-to-view` labels (OLM `Subscription`/`ClusterServiceVersion`/`InstallPlan`, ArgoCD `Application` — ArgoCD manages its own internal RBAC and doesn't aggregate to K8s `view`) — still needs a separate mechanism for those

**Confidence**: 95% (recommended, as the PRIMARY fix for #1069 itself)

---

### Alternative 3b: Shrink built-in defaults to universal kinds only + generalize bring-your-own-ClusterRole across all 3 services

**Approach**: Two parts.
1. Reduce the mandatory/built-in owner-chain RBAC default for Gateway/EM to **only** genuinely universal, non-ecosystem-specific Kubernetes-core kinds (`policy/poddisruptionbudgets` plus the existing `networking.k8s.io` core rules) — remove the unconditional OLM/Istio/cert-manager/ArgoCD/KubeVirt/Routes grant.
2. Generalize the reference-by-name mechanism that already exists for KubernautAgent in `kubernaut-operator` (`AdditionalClusterRoleBindings []string`, `kubernaut-operator/api/v1alpha1/kubernaut_types.go:806`, rendered by `AdditionalAgentCRB`/`AdditionalAgentCRBName`, `rbac.go:475-511`) — currently KA-only and operator-only — to Gateway and EM too, and introduce the equivalent in this repo's Helm chart (net-new there; confirmed zero existing `additionalClusterRoleBindings` support in `charts/kubernaut/` today for any service). Operators create their own `ClusterRole` for whichever ecosystem(s) they run (OLM, Istio, cert-manager, ArgoCD, Kafka, Knative, custom CRDs — anything) and reference its name in `<service>.additionalClusterRoleBindings: [...]`.

**Pros**:
- ✅ Scales to unbounded/unanticipated resource kinds — the operator writes the exact `ClusterRole` they need, no Kubernaut release required
- ✅ No unwanted permission surface — clusters that don't run an ecosystem never get a grant for it
- ✅ Reuses an already-proven, already-tested pattern (KA's existing mechanism) rather than inventing new mechanics
- ✅ GitOps-friendly: declared in the same `values.yaml`/CR, with automatic pruning and missing-role validation — the value-add over an operator manually creating their own out-of-band `ClusterRoleBinding` (which already works today with zero Kubernaut code, per #1069's own documented workaround)
- ✅ Publishing sample `ClusterRole` snippets for common ecosystems (OLM, Istio, cert-manager, ArgoCD) in docs keeps the common case copy-paste-simple

**Cons**:
- ❌ Operators who want the common ecosystems must do one extra step (create + reference a `ClusterRole`) instead of getting it for free — mitigated by documented sample snippets
- ❌ Two coordinated changes needed (shrink defaults + add the new knob) rather than one

**Confidence**: 90% (recommended)

---

### Alternative 4: Remove all built-in defaults, including PDB

**Approach**: Require bring-your-own-`ClusterRole` for every resource kind, including `poddisruptionbudgets`.

**Pros**:
- ✅ Maximally minimal permission footprint by default

**Cons**:
- ❌ PDB is a universal, non-ecosystem-specific vanilla-Kubernetes primitive — it's the exact resource kind that caused the original signal-drop bug in #1069, on every cluster type, OCP or not. Forcing extra setup for something this fundamental has no security benefit (it's not a sensitive or ecosystem-specific grant) and directly regresses the "it just works" bar this fix exists to restore

**Confidence**: 25% (rejected)

---

## Decision

**APPROVED: Alternative 3a (primary fix) + Alternative 3b's bring-your-own-ClusterRole knob (secondary, for what 3a doesn't cover)**

**Rationale**:
1. **Unbounded problem, bounded-list solutions don't work.** Both Alternative 1 (one big list) and Alternative 2 (discretized toggles) are variations of the same "enumerate every ecosystem" strategy, which cannot keep pace with the actual variety of CRDs operators run.
2. **`view` binding is the right primary fix — it's already a proven pattern here.** Alternative 3a directly fixes #1069's concrete PDB bug using a mechanism this codebase already ships and tests (issue #545) — no new schema, minimal risk, and it self-limits to whatever ecosystems are actually installed on a given cluster (an emergent least-privilege property, not a static grant).
3. **The bring-your-own-ClusterRole knob covers what `view` structurally cannot** — OLM `Subscription`/`InstallPlan` and ArgoCD `Application` don't ship `aggregate-to-view` labels (ArgoCD manages its own RBAC internally), so operators who run those need an explicit, declarative way to grant access. The core *capability* already exists natively (a cluster-admin can bind an arbitrary `ClusterRole` today, with zero Kubernaut code — #1069's own documented "Workaround"); Kubernaut's job is to make declaring it part of the same GitOps-tracked deployment config, with lifecycle management and validation, which the existing KA-only operator mechanism already proves out.
4. **Minimize blast radius.** Existing explicit PDB/Istio rules already present on EM/KA's Helm-chart ClusterRoles are left untouched in this pass — removing them is a separate, lower-urgency cleanup (belt-and-suspenders redundancy with the new `view` binding, not a correctness problem) that shouldn't block fixing #1069's active bug.

**Key Insight**: The right question was never "which resource kinds should Kubernaut hardcode/toggle" — it's "how thin can Kubernaut's own opinion be, while still making the already-existing native RBAC-composability capability convenient and GitOps-safe." `view`'s aggregation mechanism already answers that for well-behaved ecosystem operators; the bring-your-own-ClusterRole knob answers it for everything else.

## Implementation

**Scope for this repository (`kubernaut`)**: the Helm chart (`charts/kubernaut/`) only — this is where #1069's actual customer-facing bug lives.

**Primary Implementation Files**:
- `charts/kubernaut/templates/gateway/gateway.yaml` — new `gateway-view` `ClusterRoleBinding` (mirrors `effectivenessmonitor-view`/`remediationorchestrator-view` from #545); new `gateway.additionalClusterRoleBindings` values-driven `ClusterRoleBinding` block
- `charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml` — new `effectivenessmonitor.additionalClusterRoleBindings` block (already has the `view` binding from #545)
- `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml` — new `kubernautAgent.additionalClusterRoleBindings` block, for parity with the operator's existing KA-only mechanism
- `charts/kubernaut/templates/_helpers.tpl` — shared `kubernaut.additionalClusterRoleBindings` named template (name-safe CRB naming, matching the operator's `AdditionalAgentCRBName` truncation/hashing logic for consistency)
- `charts/kubernaut/values.schema.json` / `values.yaml` — new `<service>.additionalClusterRoleBindings: []` list-of-string field per service
- `charts/kubernaut/tests/owner_chain_rbac_extensibility_test.yaml` — helm-unittest coverage (IT-HELM-1069-001..004)
- `docs/installation/02-configure-services.md` (follow-up) — sample `ClusterRole` snippets for OLM, ArgoCD

**Not in scope for this repo**: `kubernaut-operator`. [jordigilh/kubernaut-operator#277](https://github.com/jordigilh/kubernaut-operator/issues/277) (v1.6) tracks the same redesign for that repo's maintainers to decide on independently.

### Addendum: `global.additionalClusterRoleBindings` (2026-08-02, post-implementation)

The initial implementation gave each of Gateway/EM/KA an independent `additionalClusterRoleBindings`
list, requiring the same `ClusterRole` name to be repeated three times for the common case where all
three services need identical ecosystem visibility (they inspect the same owner-chain/target resource
at different pipeline stages). Added `global.additionalClusterRoleBindings` — merged (deduplicated via
`concat ... | uniq`) with each service's own list at each of the three call sites — as the "write once,
applies everywhere" default, mirroring this chart's existing `global.fleet.oauth2` + per-service-override
convention. The per-service fields remain for the one legitimate asymmetric case already established in
this codebase: `kubernautAgent` is documented (`BR-PLATFORM-005`) as the highest-risk, LLM-driven
component with deliberately more restrictive defaults than Gateway/EM, so an operator may want to grant
an ecosystem to Gateway/EM via `global.additionalClusterRoleBindings` while withholding it from KA, or
vice versa, by using only the per-service field.

## Consequences

**Positive**:
- ✅ Resolves #1069's actual bug (PDB-owned resources silently dropped) for Helm-chart Gateway deployments
- ✅ No permission bloat — Gateway/EM never gain OLM/Istio/cert-manager/ArgoCD/KubeVirt access unless an operator explicitly opts in
- ✅ Scales to any future ecosystem without a Kubernaut code change
- ✅ Achieves Helm-chart/operator parity on the *mechanism* (bring-your-own-ClusterRole), even though the operator's current unconditional list isn't being replicated

**Negative**:
- ⚠️ Operators who relied on (or expected) automatic OLM/Istio/cert-manager/ArgoCD coverage will need to add one `ClusterRole` + one values.yaml entry — **Mitigation**: documented sample snippets make this copy-paste
- ⚠️ `kubernaut-operator`'s current unconditional grant becomes inconsistent with this repo's stance until/unless that repo's maintainers adopt the same redesign — **Mitigation**: tracked via a dedicated follow-up issue in that repo, not silently diverging

**Neutral**:
- 🔄 EM's/KA's existing Helm-chart PDB/Istio rules will be superseded by the new opt-in mechanism rather than removed outright without a replacement path

## Related Decisions

- **Related**: [DD-FLEET-001](DD-FLEET-001-fleet-scope-and-owner-chain.md) (owner chain resolution for fleet/remote clusters — a different owner-chain concern, not superseded by this decision)
- **Builds On**: #1686/BR-RBAC-020 (namespace-scoped RBAC least-privilege toggle pattern)
- **Supports**: Issue [#1069](https://github.com/jordigilh/kubernaut/issues/1069), [#1860](https://github.com/jordigilh/kubernaut/issues/1860)

## Review & Evolution

**When to Revisit**:
- If operator feedback shows the extra ClusterRole-creation step is a significant adoption barrier for common ecosystems
- If `kubernaut-operator` maintainers decide not to adopt the generalized knob, creating a lasting Helm-chart/operator behavioral divergence worth reconciling differently

**Success Metrics**:
- Gateway's Helm-chart ClusterRole grants PDB read access by default (fixes #1069) with zero unconditional ecosystem-specific grants
- `additionalClusterRoleBindings` available and helm-unittest-covered on all 3 services (Gateway, EM, KA)
