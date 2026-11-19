# Issue #2670 — Implementation Plan: restore label-based scope for cluster-scoped targets

Status: implemented on branch `work/2670-cluster-scoped-restore`; PR pending. Author: implementation
agent. Date: 2026-11-19.
Revision 25 (implementation complete; review rounds 1-30).

Business requirements: BR-SCOPE-001 (FR-SCOPE-004 Option A), BR-SCOPE-002, BR-SCOPE-010,
BR-INTEGRATION-065. Controls: FedRAMP AC-4, AC-6, SI-10; SOC2 CC8.1 (change management only —
no audit event is added, so DD-AUDIT-003 is unchanged). Design records: ADR-053, ADR-068,
DD-FLEET-015, DD-GATEWAY-CLUSTER-SCOPE-SIGNALS.

## 1. Problem

PR #2637 (`552e47dd0`) implemented the #2593 DD-FLEET-015 amendment with an over-broad
boundary. Three `Check` implementations refuse every cluster-scoped target before any label
evidence is read:

| # | File | Lines | Provenance |
|---|------|-------|-----------|
| 1 | `pkg/shared/scope/manager.go` (`Check`) | 307-309 | #2637 |
| 2 | `pkg/fleet/scopecache/client.go` (`Check`) | 103-111 | #2637 |
| 3 | `pkg/fleet/acm/client.go` (`Check`) | 93-95 | #2637 (`552e47dd0`) — replaced the previous level-1 label decision at this site |

Effect: a Node or PersistentVolume carrying `kubernaut.ai/managed: "true"` returns
`ScopeUnsupported`, so its RemediationRequest is refused. All three lanes are the same #2637
regression; the pre-#2637 ACM implementation already computed
`clusterScoped := r.Namespace == "" || IsClusterScopedKind(r.Kind)` and passed it to
`scope.ResolveLabels`, which is exactly what §4.3 restores. ADR-053 §2 (lines 96-101) still
defines the labelable-class algorithm: "Is resource cluster-scoped (Node, PersistentVolume)?
YES → Check resource label ONLY … `true` → MANAGED, `false` → UNMANAGED, No label → UNMANAGED
(safe default)".

## 2. Recorded decisions this plan implements

### D1 — non-labelable pseudo-target boundary

`DD-GATEWAY-CLUSTER-SCOPE-SIGNALS.md`, §Part 2 (Proposed; the DD registry row in
`docs/architecture/DESIGN_DECISIONS.md` that names "Cluster-Scoped Signal Support (`ClusterScope`
pseudo-Kind + opt-in mechanism)"):

- Alternative 2B — "Reserved pseudo-Kind constant `"ClusterScope"` — CHOSEN"
- "When detection is `resolved=false`: `TargetResource.Kind = "ClusterScope"`,
  `TargetResource.Name = <alertname>`, `TargetResource.Namespace = <extracted namespace, or empty>`"
- §The Gap — "`ClusterScope` pseudo-resources have no backing K8s object and frequently no
  namespace"
- its wiring manifest already names `pkg/shared/scope/manager.go`

Therefore: `ScopeUnsupported` + `cluster_scoped_target_unsupported` is returned **only** for
`Kind == "ClusterScope"`. Every other empty-namespace identity is a real cluster-scoped target
resolved presence-first. A new shared constant `scope.PseudoKindClusterScope` carries the
reserved string in one place; it does not exist in production code today (verified by an
exhaustive search for the `"ClusterScope"` literal in `**/*.go`).

Rejected alternative: treat "unknown kind + empty namespace" as unsupported. It is ruled out by
DD-FLEET-015 (Scope contract: "unknown kinds … remain unavailable") and by the issue's
labeled-CRD row.

**Reachability.** No producer of `Kind: "ClusterScope"` exists in Go code today (exhaustive
literal search), so the boundary is currently unreachable on both lanes. Both fleet entry
points also GVK-validate before delegating — `pkg/fleet/federated_checker.go:110-116`
(`invalid_remote_gvk`) and `pkg/fleet/fmc/handler.go` (HTTP 400) — and the pseudo-kind carries
no apiVersion. The scopecache and ACM guards are therefore defense in depth, unit-tested
directly; a future producer that supplies an apiVersion would reach them.

### D2 — cluster-scoped target that does not exist

DD-FLEET-015 §Scope contract: "Local target presence precedes namespace inheritance.
**Missing targets, unknown kinds and missing namespace evidence remain unavailable.** … Present
unlabeled targets may inherit an observed namespace label … Only observed absent opt-in yields
none."

`ADR-053:8-13` (amendment header) explicitly supersedes "this ADR's older … missing-target/
namespace fallback". Therefore an absent cluster-scoped object is `ScopeUnavailable`
(`observation_missing` locally, `cache_miss` fleet-side), never `verified_unmanaged`. The quoted
"unknown kinds … remain unavailable" clause covers a kind the lane cannot observe at all (no GVK,
no discovery entry, or a kind the FMC syncer does not list); an unknown kind that *is* observable
is a real cluster-scoped CRD and is decided by its label (§3).

**Owner confirmation, in writing:** the decision form for this issue selected "Unavailable /
observation_missing" for D2 on 2026-11-19. The issue's "present-but-unlabeled **or absent** →
`verified_unmanaged`" is therefore read as "label absent" (see §10, acceptance mapping).

### D3 — Direct ACM lane

DD-FLEET-015 Scope contract: "**Direct ACM** uses exact, unambiguous GVK/identity and actual
Search label projections, not boolean conversion; empty/incomplete/mismatched/missing-label
evidence is unavailable. Explicitly present empty label projection can prove label absence.
Invalid/duplicate labels cannot inherit." No cluster-scoped carve-out exists in that clause or
in ADR-053. The existing unit test `UT-ACM-2592-002` is titled "defaults an unlabeled
cluster-scoped resource to unmanaged" while asserting `ScopeUnsupported` — the refusal
contradicts its own recorded intent, and the pre-#2637 implementation already did the level-1
resolution. Included because it is the same regression, not new scope.

### D4 — which cluster-scoped kinds may be remediation targets

Owner questions (2026-11-19): are the 23 static kinds available everywhere, and does restoring
them over-provision the default roles?

- The full inventory (23 entries; this is the literal list the ADR-053 section added by this issue
  publishes and `UT-SCOPE-2670-012` transcribes):
  - **Upstream Kubernetes, shipped by default (19):** `Node`, `PersistentVolume`, `Namespace`,
    `ClusterRole`, `ClusterRoleBinding`, `StorageClass`, `CustomResourceDefinition`,
    `IngressClass`, `PriorityClass`, `ValidatingWebhookConfiguration`,
    `MutatingWebhookConfiguration`, `CSIDriver`, `CSINode`, `VolumeAttachment`, `RuntimeClass`,
    `APIService`, `CertificateSigningRequest`, `FlowSchema`, `PriorityLevelConfiguration`.
    (`APIService`, the CSI kinds and the flowcontrol kinds can be disabled in a stripped-down
    apiserver; absence only makes them unobservable.)
  - **OpenShift-only (4):** `ClusterOperator`, `ClusterVersion`, `Infrastructure`,
    `ClusterNetwork`.
- `clusterScopedKinds` is a **static heuristic**, not discovery (`manager.go:104-130`: "intentionally
  limited"). **There is no assurance any entry exists in a given cluster** — and none is required:
  an absent kind is unobservable, so it can never return `verified_managed`. Classification decides
  *how* an identity is resolved, never *whether* it authorizes.
- The label is the authorization gate; read access bounds which objects can be observed at all.
  Per lane:
  - **Local (GW/RO)**: the operator's roles — Gateway and AF read `nodes`, `pods`, `services`,
    `persistentvolumes`, `persistentvolumeclaims` (`gatewayTargetResourceReadRules`,
    `kubernaut-operator/internal/resources/inventory_rbac.go:99-113`), RO reads `pods`, `nodes`,
    `services`, `namespaces`, `persistentvolumes`, `configmaps`
    (`remediationOrchestratorClusterRole`, `internal/resources/rbac.go:1376`). `DD-FLEET-011`
    §Decision item 1 commits the operator to "define and maintain a shared target-resource read
    configuration for GW, RO and FMC, derived from the same default target-read rules and
    administrator-configured `spec.additionalClusterRoles` extensions"; its §Context records that
    the older GW/KA/EM-only binding is "existing operator behavior, not an implemented GW/RO/FMC
    parity contract". Parity is therefore operator-owned and already decided, not a gap here.
  - **Fenced FMC**: `Node` and `Namespace`, the syncer's `DefaultResourceKinds`
    (`pkg/fleet/fmc/syncer.go:113`).
  - **Direct ACM**: no per-kind RBAC. Two RBAC layers apply — the caller's K8s RBAC and the
    built-in `view` ClusterRole bound in each managed cluster's hub namespace (ADR-068 §ACM Search
    RBAC) — and ACM's `fine-grained-rbac` setting gates the second layer: it defaults to `false`,
    in which case Search "requires `cluster-admin` for any search results", and the per-managed-
    cluster visibility model only takes effect once it is enabled. Either way the authorizable set
    is broader than the fenced lane's explicit kind allowlist; that is pre-existing ACM design,
    recorded as residual risk (§10), not changed here.
- **`Namespace` is a legitimate target**, not an over-provisioning case: an alert can be *about* a
  namespace (stuck terminating, quota exhausted, namespace-level SLO), and its
  `kubernaut.ai/managed` label lives on the object, so ADR-053 §2 level 1 applies exactly as it does
  for a Node. It adds no new authorization surface: the same label is already level-2 inheritance
  evidence for unlabeled resources inside the namespace — pre-existing ADR-053 behavior, unaffected
  by this fix.

**Decision (owner, 2026-11-19):** keep it as it is — no kind allowlist. The access is
**read-only authorization**, not mutation: the local lane issues a metadata-only
`Get` on `PartialObjectMetadata`, the fenced lane reads the cache, and the ACM lane issues a
Search query; no new verb beyond `get/list/watch` is required, so the least-privilege exposure is
"may read the object", not "may change it". The owner requires the recognized set to be
**documented explicitly** rather than left implicit in the map (§8). A kind-level allowlist, if
wanted later, belongs with the fleet-side discussion in §11 and needs its own DD.

### D5 — `PersistentVolumeClaim` and the namespaced target-read parity

Owner request (2026-11-19): "GW/RO/FMC should have get/list/watch for PVC as a cluster role, same as
deployment and configmap — if it's not there already."

**PVC is namespaced**, so it does not belong in `clusterScopedKinds`: the new guard
`Namespace != "" && isClusterScoped(Kind)` would return `ScopeInvalid` for every real PVC, and
`clusterScopedIdentity()` would suppress the level-2 namespace inheritance PVCs use today. What the
request actually needs, verified against `kubernaut-operator`:

| Component | `deployments` | `configmaps` | `persistentvolumes` | `persistentvolumeclaims` |
|---|---|---|---|---|
| Gateway | yes (`rbac.go:1115` + shared rules) | no cluster-wide (operator-namespace Role only, `rbac.go:403-407`) | yes | **yes** (`inventory_rbac.go:102`) |
| API Frontend | yes (get/list projection) | yes | yes | **yes** (`rbac.go:1599`) |
| RO | yes (`rbac.go:1377`) | yes | yes | **no** (`rbac.go:1376`) |
| Effectiveness Monitor | yes, get/list (`rbac.go:1456`) | yes | — | yes (`rbac.go:1455`) |
| Kubernaut Agent | yes | yes | yes | yes (`rbac.go:1195`) |
| FMC (own SA) | — | — | — | — (no hub target reads) |

- **The only gap is RO**: `remediationOrchestratorClusterRole` (`rbac.go:1376`) lists `pods`,
  `nodes`, `services`, `namespaces`, `persistentvolumes`, `configmaps` but not
  `persistentvolumeclaims`, while its apps rule (`:1377`) already grants deployments. Add
  `persistentvolumeclaims` to that core rule (get/list/watch) in `kubernaut-operator`, with its
  RBAC test updated — a companion change in that repository, not this PR.
- **Gateway and API Frontend already have it**; nothing to do.
- **FMC needs nothing**: its own SA reads no target resources
  (`fleetMetadataCacheClusterRole`, `fleetmetadatacache.go:297-309`, contains only MCP-gateway CRD
  rules). It observes targets over the MCP Gateway, and its fleet-caller group is bound to the
  built-in `view` ClusterRole (`rbac.go:240`), which already includes PVC. A grant on FMC's SA
  would be unused and would only widen the surface.

Scope-side consequences in this repo:

1. **Classification**: `PersistentVolumeClaim` is missing from `kindToGroup`
   (`manager.go:88-102`), so `IsKnownNamespacedKind` does not cover it and the empty-namespace
   guard cannot catch a namespace-less PVC. Add `"PersistentVolumeClaim": ""`; behavior for a
   normal PVC is unchanged (level 1 → level 2). Side effect: the exported `ScopeGVKs()`
   (`pkg/shared/scope/cache.go:31-42`) gains the PVC GVK. That function has no production caller
   (only `pkg/shared/coverage_668_test.go`), and its "grant read on every GVK" comment is already
   aspirational — `Secret` is in the same map without a cluster-wide read grant — so the comment
   must not be presented as a verified invariant.
2. **Do not extend the second map**: `pkg/shared/k8s/ownerchain/owner_resolver.go:61-63` keeps its
   own copy documented as "same mapping as `kindToGroup`", and `OwnerChainCacheObjects()` feeds
   Gateway's eager informer set (`pkg/gateway/server_constructors.go:276,308-314`). Leave it
   unchanged (no scope expansion), correct that comment in this PR, and record the consequence: a
   PVC target check on the Gateway lane uses a lazy informer on first use rather than being
   covered by startup `WaitForCacheSync`.
3. **Documentation**: the ADR-053 section publishes two sets — the 23 cluster-scoped kinds and the
   namespaced kinds the lanes recognize — with PVC in the namespaced set.
4. **Test**: `UT-SCOPE-2670-013` proves the namespaced hierarchy still applies to PVC. All three
   cases carry `Version: "v1"` (Group `""`), because guard 3 rejects a bare kind as
   `invalid_target_identity`: labeled → `verified_managed` / `resource-label`; unlabeled in a
   managed namespace → `verified_managed` / `namespace-label`; empty namespace → `ScopeInvalid` /
   `namespaced_target_without_namespace`. Fixture: `corev1.PersistentVolumeClaim` via
   `WithObjects` (`corev1` is already registered in the suite).

| Target | Authorization |
|--------|--------------|
| Real cluster-scoped object (Node, PV, cluster-scoped CRD) with observed `kubernaut.ai/managed` | `verified_managed` / `verified_unmanaged`, provenance `resource-label` |
| Real cluster-scoped object present, label absent | `verified_unmanaged`, provenance `none` (no namespace exists to inherit) — local and fleet-cache lanes. The direct-ACM lane returns `ScopeUnavailable` / `search_observation_unavailable` unless the Search item carries an explicit `label` projection: missing label evidence is not absence (§2 D3) |
| Real cluster-scoped object, invalid label value | `ScopeInvalid`, `invalid_resource_label` (fail closed, SI-10) |
| Real cluster-scoped object not observed | `ScopeUnavailable`, `observation_missing` (local) / `cache_miss` (fleet) |
| Real cluster-scoped object the caller may not read | GW/RO (cached) → retryable `ScopeUnavailable` / `scope_lookup_error`; API Frontend (direct client) → terminal `ScopeUnauthorized` / `scope_access_denied` (§3 RBAC reality check) |
| *Known* cluster-scoped kind with a non-empty namespace | `ScopeInvalid`, `invalid_cluster_scoped_namespace` (SI-10; matches `inventory.ValidateResourceIdentity`) |
| `Kind == "ClusterScope"` (cluster-level intent, no backing object) | `ScopeUnsupported`, `cluster_scoped_target_unsupported` (all three lanes; §2 D1) |
| `Namespace` object as the target (label on the object itself) | level 1, same as any cluster-scoped object: `verified_managed` / `verified_unmanaged` from its own label (§2 D4); its inheritance role for workloads inside it is unaffected |
| Namespaced target | unchanged (level 2 inheritance preserved) |
| `PersistentVolumeClaim` target | namespaced: `verified_managed` / `verified_unmanaged` from its own label, else level-2 namespace inheritance; `ScopeInvalid` / `namespaced_target_without_namespace` when the identity carries no namespace (§2 D5) |
| *Unknown* kind with an empty namespace (cluster-scoped CRD), observed | level 1 only: label decides — `verified_managed` / `verified_unmanaged` (`resource-label`), or `verified_unmanaged` / `none` when the label is absent |
| *Known namespaced* kind with an empty namespace (e.g. `Deployment` with `Namespace: ""`) | `ScopeInvalid`, `namespaced_target_without_namespace` — terminal, not a retryable observation gap (new guard; §4.1 item 4.5) |
| *Unknown* kind carrying a non-empty namespace | unchanged from `main` (namespace inheritance); pre-existing ambiguity, §10/§11 |

**RBAC reality check.** The deployed authorization is built by `kubernaut-operator`, not by this
repo's generated `config/rbac/role.yaml`. The generated file is controller-gen output from
`+kubebuilder:rbac` markers, and no marker ever declared `persistentvolumes` (`git log -S` finds
neither a marker nor a historical entry) — Node and namespaces appear only because SignalProcessing
reads them (`signalprocessing_controller.go:109,111`). The operator's roles do grant PV:
`gatewayTargetResourceReadRules()` gives Gateway `nodes, pods, services, persistentvolumes,
persistentvolumeclaims` get/list/watch (`kubernaut-operator/internal/resources/inventory_rbac.go:99-113`)
and AF projects its get/list subset, while `remediationOrchestratorClusterRole` grants RO
`pods, nodes, services, namespaces, persistentvolumes, configmaps`
(`kubernaut-operator/internal/resources/rbac.go:1376`). Of the 23 recognized kinds, a deployed hub
can therefore read **Node, PersistentVolume and Namespace** on the local lane — exactly the three
the issue names. The other twenty fail closed, with a state that depends on the reader:
- **GW and RO** are informer-backed (`pkg/gateway/server_constructors.go:539`,
  `cmd/remediationorchestrator/main.go:455`), so a kind with no list/watch permission never syncs
  and the read ends in the retryable `ScopeUnavailable` / `scope_lookup_error`
  (`manager.go:61-64`, `:335-343`).
- **API Frontend** injects a direct client (`cmd/apifrontend/backend_deps.go:268` →
  `scope.NewManager(deps.k8sTypedClient)`), so the API server answers 403 immediately and the
  result is the terminal `ScopeUnauthorized` / `scope_access_denied` (`manager.go:336-338`).

Node, PV and Namespace are reachable end to end on the local lane with no RBAC follow-up; fleet PV
observation remains out of scope (§11) because the syncer's `DefaultResourceKinds` cover only Node
and Namespace.

## 4. Production changes

### 4.1 `pkg/shared/scope/manager.go`

1. Add to the existing `const` block, citing DD-GATEWAY-CLUSTER-SCOPE-SIGNALS Alternative 2B
   and the SI-10 identity rule:
   `PseudoKindClusterScope = "ClusterScope"`,
   `ReasonClusterScopedTargetUnsupported = "cluster_scoped_target_unsupported"`,
   `ReasonInvalidClusterScopedNamespace = "invalid_cluster_scoped_namespace"` and
   `ReasonNamespacedTargetWithoutNamespace = "namespaced_target_without_namespace"`,
   `ReasonInvalidResourceLabel = "invalid_resource_label"` and
   `ReasonObservationMissing = "observation_missing"`. All three
   lanes set them; only the local lane makes them observable, because `ScopeResult.Reason` is
   `json:"-"` (DD-FLEET-015) — the same qualifier `docs/UPGRADE.md` already documents.
   `scopecache` exports `ReasonCacheMiss` for the same purpose on the fenced lane.
2. Add two unexported derivation helpers, single caller at GREEN:
   `clusterScopedIdentity(resource) bool { return isClusterScoped(resource.Kind) || resource.Namespace == "" }`
   (the `resolveObserved` argument) and `knownNamespacedKindWithoutNamespace(resource) bool`,
   which is true only when the kind is in `kindToGroup`, is not cluster-scoped, and the namespace
   is empty. The second one keeps a malformed identity terminal instead of turning it into a
   retryable `observation_missing` (§3, §10).
3. Export `IsKnownNamespacedKind(kind string) bool` for the fleet and ACM lanes, derived from
   `kindToGroup` minus `clusterScopedKinds`; it is the same knowledge `IsClusterScopedKind`
   already exposes and keeps the three lanes on one classification.
4. Add `"PersistentVolumeClaim": ""` to `kindToGroup` (§2 D5) so PVC is a known namespaced kind:
   the empty-namespace guard covers it. A normal PVC target's behavior is unchanged (level 1 →
   level 2). Side effect: `ScopeGVKs()` (`pkg/shared/scope/cache.go:31-42`) gains the PVC GVK;
   its "grant read on every GVK" comment remains aspirational (`Secret` still has no GW/RO
   cluster-wide grant even after the companion change), so do not restate it as an invariant.
5. `Check` guard order:
   1. `ClusterID != "" || Kind == "" || Name == ""` → `invalid_resource_identity` (unchanged)
   2. `Kind == PseudoKindClusterScope` → `ScopeUnsupported` / `ReasonClusterScopedTargetUnsupported`
      (must precede GVK validation: the pseudo-target carries no apiVersion)
   3. `GVKForIdentity` error → `invalid_target_identity` (unchanged)
   4. `resource.Namespace != "" && isClusterScoped(resource.Kind)` →
      `ScopeInvalid` / `ReasonInvalidClusterScopedNamespace`
   5. `knownNamespacedKindWithoutNamespace(resource)` → `ScopeInvalid` /
      `ReasonNamespacedTargetWithoutNamespace`
   6. otherwise `resolution, err := m.resolveObserved(ctx, resource, clusterScopedIdentity(resource))`
6. `resolveObserved` gains the `clusterScoped bool` parameter (single caller: `Check`). Its
   early return becomes
   `if resourceState == LabelStateTrue || resourceState == LabelStateFalse || resourceState == LabelStateInvalid || clusterScoped { return resolveLabelStates(resourceState, LabelStateAbsent, clusterScoped), nil }`
   so a cluster-scoped target never reads a Namespace. The final `resolveLabelStates` call at
   `manager.go:289` passes the parameter; on that path it is necessarily `false`, because the
   early return above already handled every `clusterScoped` case.
7. Package doc comment: note the pseudo-target exception next to the existing
   "For cluster-scoped resources (namespace==\"\"), only the resource label is checked".

`Resolve`, `ResolveWithResourceLabels` and `resolveWithResourceLabels` keep their behavior. Their
inline `isClusterScoped(kind) || Namespace == ""` derivations (`manager.go:236`, `:357`) are
replaced by the helper in REFACTOR, behavior-identical.

### 4.2 `pkg/fleet/scopecache/client.go`

1. Replace the blanket guard (103-111) with, in order:
   1. pseudo-kind → `ScopeUnsupported` / `scope.ReasonClusterScopedTargetUnsupported`
      (defense in depth, §2 D1)
   2. the existing GVK validation (moved after 1)
   3. `Namespace != "" && scope.IsClusterScopedKind(Kind)` → `ScopeInvalid` /
      `scope.ReasonInvalidClusterScopedNamespace`
   4. `Namespace == "" && scope.IsKnownNamespacedKind(Kind)` → `ScopeInvalid` /
      `scope.ReasonNamespacedTargetWithoutNamespace` (same terminal-vs-retryable rule as 4.1)
2. No other change: `mapFencedRecord` (237-240) and `checkNamespaceObservation` (317-319)
   already return `verified_unmanaged` / `source none` for cluster-scoped identities once
   reached. Both guards stay: they are the internal namespace-inheritance reads whose existing
   behavior the issue requires to keep.

### 4.3 `pkg/fleet/acm/client.go`

1. Pseudo-kind → `ScopeUnsupported` / `scope.ReasonClusterScopedTargetUnsupported`
   (defense in depth; same production reachability caveat as §2 D1).
2. the existing GVK validation (moved after 1).
3. `Namespace != "" && scope.IsClusterScopedKind(Kind)` → `ScopeInvalid` /
   `scope.ReasonInvalidClusterScopedNamespace`.
4. `Namespace == "" && scope.IsKnownNamespacedKind(Kind)` → `ScopeInvalid` /
   `scope.ReasonNamespacedTargetWithoutNamespace` (same terminal-vs-retryable rule as 4.1).
5. `clusterScoped := r.Namespace == "" || scope.IsClusterScopedKind(r.Kind)`; pass it to
   `scope.ResolveLabels(resourceLabels, nil, clusterScoped)` and skip the Namespace Search read
   when `clusterScoped` (level 1 only). `buildFilters` already omits the namespace filter when
   `r.Namespace == ""`, so the Search request needs no change.

### 4.4 Gateway: target identity, normalization and dedup

`validateScope` built the scope identity from `signal.Namespace` (the alert's namespace) while the
RemediationRequest carries `signal.Resource.Namespace` (`crd_creator.go` `buildTargetResource`).
For a cluster-scoped kind the two differ, and the new terminal guard would turn a labeled Node into
`ScopeInvalid` → a 5xx instead of admission. `scopeIdentityFor` now derives the namespace once
(`scopeTargetNamespace`), writes it back onto the signal, and builds the identity from it, so the
Gateway checks exactly the identity RO re-checks.

`PrometheusAdapter.extractAlertResource` blanks the namespace for a known cluster-scoped kind before
fingerprint resolution. That keeps the dedup key, the RR target and the scope identity aligned, and
matches the API Frontend's documented invariant that GW and AF hash the same target. It supersedes
#1371's "honor an explicit namespace label" for cluster-scoped kinds (`UT-GW-1371-014` updated).

## 5. TDD phases

Every phase starts with a failing test (target + focus), captured as RED evidence for the PR
body.

| Phase | RED (failing) | GREEN | REFACTOR |
|-------|---------------|-------|----------|
| 1. Local lane | `pkg/shared/scope/manager_test.go`: `UT-SCOPE-2670-001..011` and `..013`; `pkg/shared/scope/cluster_scoped_kinds_test.go` (new, `package scope`): `UT-SCOPE-2670-012`, `..014` | `manager.go` §4.1 | replace the two remaining inline `isClusterScoped(...) \|\| Namespace == ""` derivations (`Resolve`, `ResolveWithResourceLabels`) with the GREEN helper (behavior-identical, three callers), and rewrite the guard comment |
| 2. Fleet lane UT | `client_red_test.go` (`UT-FMC-2670-001..005`), `scopecache_test.go` (`UT-FLEET-SC-016`); the shared `mockCacheReader` is upgraded to `store map[string]scope.LabelState` so `Absent`/`Invalid` observations are expressible, and `UT-FMC-2670-005` covers the fenced status-store lane FMC actually serves from | `client.go` §4.2 | delete the refusal comment that is now wrong; migrate the remaining `bool` fixture call sites to the label-state mock |
| 3. ACM lane UT + IT | `pkg/fleet/acm/client_test.go`: rewrite `UT-ACM-2592-002`, add `UT-ACM-2670-001..002`; `test/integration/acm/acm_factory_test.go`: add `IT-ACM-2670-001` | `acm/client.go` §4.3 (phase exits only when both the UTs and `IT-ACM-2670-001` pass) | extract the four terminal guards into `classifyIdentity` (gocyclo 20 → below the 15 threshold), collapse the namespace read into the `!clusterScoped` branch (§4.3 item 5) keeping the level-2 read for namespaced targets |
| 4. Fleet IT | `fmc_broker_readiness_test.go` `IT-FMC-DEFERRED-001`; `valkey_scope_cache_test.go` `IT-FLEET-VALKEY-007` | no production change expected — `mapFencedRecord` already returns `none` for cluster-scoped identities (`pkg/fleet/scopecache/client.go:237-240`); fix in §4.2 only if the IT exposes an ordering defect | N/A unless phase 4 exposes duplication |
| 5. RO IT | `ea_creation_integration_test.go` `IT-RO-192-001` restored to its pre-#2637 body | covered by phase 1 (no new code) | N/A |
| 6. Docs | — | §8 | N/A |

REFACTOR phases name a concrete change; build/lint/test is the safety net, not the content.

## 5b. Companion change — `kubernaut-operator`

Lands in the `kubernaut-operator` repository as its own PR (this PR cannot touch that repo):

1. RED: extend `internal/resources/rbac_test.go` to assert
   `remediationOrchestratorClusterRole` grants `persistentvolumeclaims` get/list/watch.
2. GREEN: add `persistentvolumeclaims` to the core rule at `internal/resources/rbac.go:1376`.
3. Nothing else: Gateway/AF already grant PVC, EM/KA already do, FMC's own SA performs no hub
   target reads, and the fleet-caller group already has PVC through the built-in `view` role
   (§2 D5).

## 6. Scenario matrix (requirements-based)

### Unit — `pkg/shared/scope` (local lane, `Manager.Check`)

| ID | Scenario | Expected | Critical |
|----|----------|----------|----------|
| UT-SCOPE-2670-001 | labeled Node | `verified_managed` / `resource-label` | yes (the regression) |
| UT-SCOPE-2670-002 | unlabeled Node with a same-named managed Namespace | `verified_unmanaged` / `none` | yes (AC-4/AC-6) |
| UT-SCOPE-2670-003 | Node labeled `"false"` | `verified_unmanaged` / `resource-label` | |
| UT-SCOPE-2670-004 | Node label `"yes"` | `ScopeInvalid` / `invalid_resource_label` | yes (SI-10) |
| UT-SCOPE-2670-005 | absent Node | `ScopeUnavailable` / `observation_missing`, retryable | yes |
| UT-SCOPE-2670-006 | Node with `Namespace: "default"` | `ScopeInvalid` / `invalid_cluster_scoped_namespace` | yes (SI-10) |
| UT-SCOPE-2670-007 | labeled PersistentVolume | `verified_managed` / `resource-label` | |
| UT-SCOPE-2670-008 | labeled cluster-scoped CRD (`storage.example.com/v1 ClusterVolume`, fixture `&unstructured.Unstructured{}` added through `WithObjects`, no scheme registration) | `verified_managed` / `resource-label` | yes (labeled CRD intent) |
| UT-SCOPE-2670-009 | `Kind: ClusterScope` | `ScopeUnsupported` / `cluster_scoped_target_unsupported` | yes (D1 boundary) |
| UT-SCOPE-2670-010 | known namespaced kind (`Deployment`) with `Namespace: ""`; unknown kind **with explicit group/version** (`storage.example.com/v1`, `ClusterVolume`) and `Namespace: ""`, not observed (a bare kind would fail guard 3 as `invalid_target_identity`) | `ScopeInvalid` / `namespaced_target_without_namespace`; `ScopeUnavailable` / `observation_missing` | yes (prevents a retry loop for malformed input) |
| UT-SCOPE-2670-011 | `Namespace` object as the target (not inheritance evidence), labeled managed | `verified_managed` / `resource-label` — §2 D4: a namespace-level alert is a legitimate target; the label on the object applies at level 1 | yes (regression guard: a later "narrow the kind set" change must edit this test deliberately) |
| UT-SCOPE-2670-012 | the recognized cluster-scoped inventory itself | `clusterScopedKinds` equals the 23-entry list the ADR-053 section publishes (added by this issue, §8) — adding or removing a kind must change this test and the doc together. Needs its own internal `package scope` spec file (the map is unexported), not `scope_test` | yes (keeps the documented authorization surface honest) |
| UT-SCOPE-2670-013 | `PersistentVolumeClaim` target (all cases `Version: "v1"`, Group `""`, fixture `corev1.PersistentVolumeClaim`): labeled; unlabeled in a managed namespace; empty namespace | `verified_managed` / `resource-label`; `verified_managed` / `namespace-label`; `ScopeInvalid` / `namespaced_target_without_namespace` — proves PVC keeps the namespaced 2-level hierarchy (§2 D5) | yes (owner-requested kind) |
| UT-SCOPE-2670-014 | namespaced kinds (`Pod`, `Deployment`, `Service`, `ConfigMap`, `Secret`, `Job`, `CronJob`, `PersistentVolumeClaim`, `HorizontalPodAutoscaler`, `Ingress`, `PodDisruptionBudget`) are never classified as cluster-scoped and are known-namespaced | `IsClusterScopedKind` false for each; `IsKnownNamespacedKind` true (so a namespace-less identity is terminal) | yes (SI-10) |
| all pre-existing `pkg/shared/scope` specs in `manager_test.go` | namespaced hierarchy, invalid identity, error paths | unchanged — the namespaced specs never reach a new guard; the cluster-scoped specs (`UT-SCOPE-001-013/014/015`) drive `Resolve`, which never had the refusal | regression |

`UT-SCOPE-001-013/014/015` stay as-is: they drive `Resolve`, which never had the refusal.


### Unit — `pkg/fleet/scopecache`

Contract cases for the `ObservationReader` lane (`NewClient`), which no production constructor
uses: `pkg/fleet/scope_factory.go` composes `fmc.NewHTTPClient` or `acm.NewClient`, and the FMC
service uses `NewClientWithStatusStore`. The reader lane is consumed by `test/integration/gateway/fleet_scope_test.go`, `test/integration/gateway/fleet_signal_test.go`, `test/integration/remediationorchestrator/ro_fleet_scope_test.go`, `test/integration/fleetmetadatacache/fmc_http_api_test.go`, `test/integration/fleetmetadatacache/fmc_e2e_test.go` and `test/integration/fleetmetadatacache/valkey_scope_cache_test.go` (the IT-FLEET-VALKEY-007 chain), so its contract must match the status-store lane. The shared mock (`pkg/fleet/scopecache/scopecache_test.go:35-57`) currently maps `key → bool` and can express only `true`/`false`; phase 2 upgrades it to a `map[string]scope.LabelState` (keeping the `bool` call sites migrated) before the `Absent` and `Invalid` rows below can go RED.

| ID | Scenario | Expected |
|----|----------|----------|
| UT-FMC-2670-001 (replaces UT-FMC-DEFERRED-001) | Node / PV / cluster-scoped CRD (`CustomClusterResource`) / `Namespace` as target, observed `true` | `verified_managed` / `resource-label` (§2 D4: a Namespace *target* is level 1, distinct from the Namespace reads used for inheritance in §4.2) |
| UT-FMC-2670-002 | Node **present** with `LabelStateAbsent`, plus a managed Namespace key; unknown cluster-scoped kind **present** with `LabelStateAbsent` | `verified_unmanaged` / `none` (namespace key never consulted) |
| UT-FMC-2670-003 | Node observed `false`; Node observed `invalid` | `verified_unmanaged` / `resource-label`; `ScopeInvalid` / `invalid_resource_label` |
| UT-FMC-2670-004 | Node missing record; Node with a namespace; known namespaced kind with an empty namespace; `Kind: ClusterScope` | `ScopeUnavailable` / `cache_miss`; `ScopeInvalid` / `invalid_cluster_scoped_namespace`; `ScopeInvalid` / `namespaced_target_without_namespace`; `ScopeUnsupported` / `cluster_scoped_target_unsupported` (defense in depth, §2 D1) |
| UT-FMC-2670-005 | fenced status-store lane (`NewClientWithStatusStore`, the production constructor): Node with `true` / `false` / `absent` / `invalid` | `verified_managed` / `verified_unmanaged` / `verified_unmanaged`+`none` / `ScopeInvalid`; each result passes `Validate()` |
| UT-FLEET-SC-016 (rewritten) | Node **present** with a `LabelStateAbsent` observation and a coincidentally matching namespace key | `verified_unmanaged` / `none`, namespace key never consulted |

Fleet production observation of cluster-scoped **targets** covers Node and Namespace today
(Namespace is additionally read as inheritance evidence, and `buildObservedResource` writes it at
the same key a Namespace *target* check reads): `pkg/fleet/fmc/syncer.go` `kindToGVK` and
`DefaultResourceKinds`, and `pkg/fleet/mcpclient/observation.go` `isSupportedObservedGVK` accept
Deployment/StatefulSet/DaemonSet/Pod/Service/Node/Namespace. The PV and CRD entries above prove
the **reader contract**, not a production observation; fleet PV/CRD observation is out of scope
(§11).

### Unit — `pkg/fleet/acm`

| ID | Scenario | Expected |
|----|----------|----------|
| UT-ACM-2592-002 (rewritten) | Node with an explicit empty label projection | `verified_unmanaged` / `none` |
| UT-ACM-2670-001 | Node and `Namespace` (as target) with `kubernaut.ai/managed=true` projections | `verified_managed` / `resource-label` |
| UT-ACM-2670-002 | Node with invalid label; Node with a missing label projection and with `label: null`; Node with a namespace; namespaced kind without a namespace; `ClusterScope` | `ScopeInvalid` / `invalid_resource_label`; `ScopeUnavailable` (evidence is not absence); `ScopeInvalid` / `invalid_cluster_scoped_namespace`; `ScopeInvalid` / `namespaced_target_without_namespace`; `ScopeUnsupported` / `cluster_scoped_target_unsupported` |

### Integration (wiring + controls)

| ID | Proves |
|----|--------|
| IT-FMC-DEFERRED-001 (rewritten) | Full fleet chain (MCP observation → FMC syncer → Valkey fenced store → `Client.Check` → HTTP handler → HTTP client): managed Node authorizes (200, was 422), present-unlabeled Node is `verified_unmanaged`/`none` (200), namespaced inheritance preserved. The fixture at `fmc_broker_readiness_test.go:128-139` emits every non-Pod kind with `kubernaut.ai/managed: "true"`; it gains a label-state switch (for example a `nodeLabelAbsent atomic.Bool`) so the unlabeled case is expressible |
| IT-FLEET-VALKEY-007 (rewritten) | Real Valkey chain: a **present** Node with an absent-label observation yields `verified_unmanaged`/`none`; a coincidentally matching Namespace key is never consulted (AC-6) |
| IT-RO-192-001 (restored) | Local lane through RO: a managed-labeled Node passes the scope gate, the RR reaches Processing, SP is created, SP failure drives `PhaseFailed`, and no EA is created (#240 contract) |
| IT-ACM-2670-001 (new) | Direct ACM backend composed by the production factory: a `Node` identity with `label: ""` resolves `verified_unmanaged`/`none`, and with `label: "kubernaut.ai/managed=true"` resolves `verified_managed`/`resource-label` — the cluster-scoped branch of `acm.Client.Check` through real wiring |

Scenario coverage: 23 unit scenario IDs (local 14 / fleet 6 / ACM 3) + 4 integration scenarios,
all business outcomes, no duplicate assertions. Critical scenarios: 100%.

## 7. Wiring manifest

| Component | Production entry point | Wiring code location | IT |
|-----------|----------------------|---------------------|----|
| `scope.Manager.Check` cluster-scoped resolution | RO remediation-authorization gate | `pkg/remediationorchestrator/routing/blocking.go` `checkScopeForResource` | IT-RO-192-001 |
| `scope.Manager.Check` cluster-scoped resolution | Gateway signal ingestion | `pkg/gateway/signal_ingestion.go` `validateScope` | `UT-GW-2670-001/002` (identity normalization); no cluster-scoped IT |
| `scope.Manager.Check` cluster-scoped resolution | API Frontend RR creation tool | `pkg/apifrontend/tools/scope_helpers.go` `checkRRScope` | **none — same injected checker (§10)** |
| `scopecache.Client.Check` | FMC `GET /api/v1/scope/check` | `pkg/fleet/fmc/handler.go` `handleScopeCheck` | IT-FMC-DEFERRED-001 |
| `acm.Client.Check` | Fleet backend selection | `pkg/fleet/scope_factory.go:158` | IT-ACM-2670-001 (run with `go test ./test/integration/acm/...`; the suite is outside the CI integration matrix, §9) |

The gateway and API Frontend rows intentionally report no dedicated IT rather than borrowing the
fleet IT: each calls the same injected `ScopeChecker.Check` through an unchanged call site, but no
cluster-scoped admission IT exists for them today. `policyread/authorizeInventoryTarget` is
excluded on purpose: it only calls `Check` for a non-local cluster (`service.go:333-335`), and its
local path (`authorizeTarget`) uses the RBAC `Authorizer.CheckTarget` seam and never calls `Check`,
so the new logic cannot be reached from it. See §10 for the risk.

## 8. Documentation changes

| File | Change |
|------|--------|
| `docs/UPGRADE.md` 56-75 | Replace "Cluster-scoped targets remain unsupported" with the restored Option A behavior and the precise pseudo-resource boundary; record the #2637→#2670 window and that `Reason` stays local-lane-only |
| `docs/architecture/decisions/DD-FLEET-015-owned-watch-and-enum-scope.md` 68-70 | Amend "Cluster-scoped target checks remain unsupported" (the clause sits on line 69) → level-1 label authorization for labelable targets; `ScopeUnsupported` only for the non-labelable pseudo-kind |
| `docs/architecture/decisions/ADR-053-resource-scope-management.md` 13 and the "V1.0 Focuses on Namespaced Resources" bullet | Amend the 2026-11-16 sentence "cluster-scoped target authorization stays deferred" to point at #2670, and the bullet "Cluster-scoped resources (Nodes, PVs) deferred to V2.0" to state Option A for labelable targets, Option B deferred |
| `docs/architecture/decisions/ADR-053-resource-scope-management.md` — new §"Recognized kinds" | **Owner requirement**: publish two sets — the authoritative 23-entry cluster-scoped inventory exactly as listed in §2 D4 (19 upstream-default + 4 OpenShift-only) and the namespaced kinds the lanes recognize (including `PersistentVolumeClaim`, §2 D5) — with the read-only statement (metadata `Get` on `PartialObjectMetadata`, cache read, ACM Search query — no mutating verb) and the per-lane read bound (GW/AF shared target reads and the RO role, both including `persistentvolumes`; the FMC syncer's `DefaultResourceKinds`; ADR-068 visibility for ACM) |
| `pkg/shared/scope/manager.go` comment above `var kindToGroup` (`manager.go:84-102`) | Note that `PersistentVolumeClaim` is namespaced and why it is in this map rather than `clusterScopedKinds` (§2 D5) |
| `pkg/shared/k8s/ownerchain/owner_resolver.go` comment (61-63) | Correct "same mapping as `kindToGroup`": `PersistentVolumeClaim` is deliberately not added to the eager owner-chain informer set (§2 D5) |
| `pkg/shared/scope/manager.go` comment above `var clusterScopedKinds` (`manager.go:104-130`) | Point the inventory at the ADR-053 section and state that these reads are authorization-only (metadata, no mutation) |
| `docs/architecture/decisions/ADR-068-fleet-federation-architecture.md` 911-919 | Same amendment: "cluster-scoped target checks remain unsupported" |
| `docs/requirements/BR-SCOPE-001-resource-scope-management.md` 12-20 and the "❌ Cluster-Scoped Resources: Nodes, PersistentVolumes (deferred to V2.0)" bullet | Same amendment inside the approved 2026-11-16 block, and in the out-of-scope list |
| `docs/tests/2593/TEST_PLAN.md` 261 and 973 | Note the #2670 supersession of S10 ("Deferred cluster-scoped target") and of the "no `verified_managed` cluster-scoped claim" assumption; record the D2 acceptance reading |
| `docs/tests/2660/TEST_PLAN.md` 43 | Note the #2670 supersession of "cluster-scoped targets stay `ScopeUnsupported`" (labelable targets authorize; `ScopeUnsupported` reserved for `ClusterScope`) |
| `test/integration/gateway/21_crd_lifecycle_integration_test.go` 196-201 | Remove the stale "now targets a Pod because Node is unsupported" rationale |

`BR-SCOPE-001` FR-SCOPE-004 is amended rather than left alone: its heading drops
"(V2.0 - Future)" and its **Status** records Option A as implemented (#2670) with Option B
(`RemediationScope`) deferred to V2.0.

## 9. Validation

```bash
go build ./...
golangci-lint run --timeout=15m
make test-unit-shared-packages
make test-unit-fleetmetadatacache
make test-integration-fleetmetadatacache
go test ./test/integration/acm/... -count=1   # IT-ACM-2670-001; plain Ginkgo suite, and the generic
                                              # test-integration-% rule would build a bogus coverpkg (Makefile:338-341)
make test-integration-remediationorchestrator GINKGO_FOCUS='IT-RO-192-001'
make lint lint-test-patterns lint-business-integration lint-tdd-compliance
```

envtest: `~/go/bin/setup-envtest use 1.37 -p path`. Podman and Docker are both available for the
RO suite. RED/GREEN evidence is captured per phase for the PR body.

## 10. Risks, mitigations, acceptance mapping

| Risk | Mitigation |
|------|-----------|
| Gateway and API Frontend authorized path have no cluster-scoped IT | All three call the same injected `ScopeChecker.Check`; the call sites are unchanged and the decision branch is proven at UT + fleet IT + RO IT. The manifest reports the gap instead of claiming coverage. |
| An *unknown* kind carrying a non-empty namespace can inherit a namespace label | **Pre-existing, not introduced here**: `manager.go:307` only refuses `Namespace == "" \|\| isClusterScoped(Kind)`, so such an identity already takes the inheritance path on `main`. It also cannot be corrected without a RESTMapper (a design change → CHECKPOINT DD). Flagged for a follow-up issue (§11); the fleet lane is unaffected (namespace-scoped cache key → miss → unavailable). |
| Regression toward cluster-wide privilege | Identities with `Namespace == ""` or a known cluster-scoped kind never read a Namespace label — proven by UT-SCOPE-2670-002 and IT-FLEET-VALKEY-007 (AC-6). The unknown-kind case above is the documented exception. |
| `ClusterScope` constant could be seen as speculative | Required by the issue ("reserved for genuinely non-labelable pseudo-targets") and recorded in DD-GATEWAY-CLUSTER-SCOPE-SIGNALS; producers land with that DD. |
| Fleet PV/CRD targets are not observable in production | Documented in §6/§11; an unobserved target is `cache_miss`, never an authorization. |
| `Namespace` as a target was the suspected over-provisioning | **Resolved as legitimate** (§2 D4): an alert can be about a namespace, its own label applies at level 1, and it adds no new surface — that label is already level-2 inheritance evidence for the namespace's unlabeled children. `UT-SCOPE-2670-011` and `UT-ACM-2670-001` pin it. |
| The static `clusterScopedKinds` list mixes portable, OpenShift-only and optional kinds — no availability assurance | Availability is not the gate: an absent kind is unobservable, so it can never return `verified_managed`. Access is read-only authorization (metadata `Get`/cache read/Search query, no mutating verb), and the recognized set is now documented in ADR-053 with `UT-SCOPE-2670-012` pinning code to doc (§2 D4). |
| The direct-ACM lane has no per-kind RBAC: with `fine-grained-rbac` enabled its bound is per-cluster `view` visibility, and at the default `false` the reader must hold `cluster-admin` — either way broader than the fenced lane's `DefaultResourceKinds` allowlist | Pre-existing ACM design (ADR-068 §ACM Search RBAC), unchanged by this fix; recorded here and offered as a fleet-side follow-up in §11. No code scope change in #2670. |
| PV was believed unreadable on a deployed hub | **Corrected**: the operator's roles grant it (GW/AF shared target reads, RO `rbac.go:1376`). This repo's `config/rbac/role.yaml` lacks it only because no `+kubebuilder:rbac` marker ever declared it; that file is controller-gen output, not the deployed authorization. No RBAC follow-up is required for Node, PV or Namespace. |
| RO integration suite runtime (~10 min, PostgreSQL/Redis/DS) | Run focused; report honestly if the environment cannot start the stack. |

**Acceptance mapping for the issue's target table.** "present-but-unlabeled or absent →
`verified_unmanaged` / `ScopeSourceNone`" is implemented as: present-but-unlabeled → verified
unmanaged/none; label absent → verified unmanaged/none; **object absent** → unavailable
(`observation_missing`/`cache_miss`) per D2 and the owner's written confirmation. This is stated
in the PR body so the PR is not judged against a criterion it deliberately changed.

Rollback: revert the PR atomically. No schema, CRD, RBAC, audit-event, or wire-format change.

## 11. Out of scope

- Adding PersistentVolume / cluster-scoped CRDs to FMC observation (`kindToGVK`,
  `DefaultResourceKinds`, `isSupportedObservedGVK`, `buildObservedResource`) — an unobserved
  target correctly yields `cache_miss`.
- **Follow-up issue (to file, optional):** a fleet-side kind allowlist for the direct-ACM backend
  if kind-level restriction is wanted there; ADR-068's visibility model is cluster/namespace
  granular. Needs its own DD.
- **Follow-up issue (to file):** the static kind map cannot prove cluster-scopedness in either
  direction. An unknown kind carrying a non-empty namespace can inherit a namespace scope label,
  and a namespaced kind outside the map (e.g. `Route`) carrying no namespace is resolved at level 1
  and lands in retryable `observation_missing` instead of a terminal rejection
  (§10, ADR-053 §2a). Needs a RESTMapper-based kind resolver (CHECKPOINT DD).
- **Follow-up issue (to file):** on the fenced lane an invalid `kubernaut.ai/managed` observation
  is rejected by `pkg/fleet/fmc/syncer.go` `buildObservedResource`, so the last committed record
  keeps serving until a later repair replaces it or its TTL expires (45s default). Removing the
  authorization instead of retaining the last-good snapshot changes the DD-FLEET-016 staged-repair
  contract for every kind (namespaced targets included) and needs its own design decision.
  ADR-053 §"Recognized kinds" records the current window. Not part of #2670.
- Implementing DD-GATEWAY-CLUSTER-SCOPE-SIGNALS (typed detection,
  `IsClusterScopeSignalAllowed`, cluster-wide opt-in label/annotation); only the reserved
  pseudo-kind is honored here.
- The `RemediationScope` CRD (FR-SCOPE-004 Option B).
- `pkg/fleet/acm/inventory.go` cluster-scoped inventory filtering (listing, not authorization).

## 12. Confidence

96%. Every behavior and boundary traces to a written record (ADR-053 §2 and its amendment
header, ADR-068, DD-FLEET-015 §Scope contract, DD-GATEWAY-CLUSTER-SCOPE-SIGNALS Alternatives
2B/3D, DD-FLEET-011 §Decision for the shared GW/RO/FMC target-read commitment), and D2 is
separately confirmed by the owner. D4 records the owner's security review: no kind allowlist, no
availability requirement (unobservable kinds cannot authorize), access is read-only authorization
rather than mutation, `Namespace` targets are legitimate, and — per the owner — the recognized
23-kind inventory is documented explicitly in ADR-053 and pinned by `UT-SCOPE-2670-012`.
`PersistentVolumeClaim` is handled as the namespaced kind it is (§2 D5), not added to the
cluster-scoped map; the only missing read grant in the operator is RO's `persistentvolumeclaims`
and it lands as the companion change in §5b. Two
explicit deductions from the project's integration target ("100% of wiring points"): (1) **two of
five wiring points** (Gateway `validateScope`, API Frontend `checkRRScope`) have no cluster-scoped
IT of their own — the Gateway has `UT-GW-2670-001/002` proving its identity normalization, both call
the identical injected checker, proven at UT + fleet IT + RO IT — and
(2) the direct-ACM lane's authorizable kind set is bounded only by ACM visibility (pre-existing,
§10). The blast radius is three guard sites and six known callers; the fleet fenced path needs no
production change beyond the guard removal. Remaining risk is the documented pre-existing
unknown-kind ambiguity, the ACM integration suite being outside the CI matrix (its wiring proof is
a manual run), and the RO suite environment.
