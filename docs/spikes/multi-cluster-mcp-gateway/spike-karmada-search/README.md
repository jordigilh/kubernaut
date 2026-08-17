# Spike: Karmada Search API Contract Validation (Source-Code Review)

**Date**: 2026-08-17
**Method**: Source-code review of `karmada-io/karmada` upstream (commit `08f8a2016f20fc68544eb7cf66f360620db859b0`, `master`, cloned 2026-08-17). No live Karmada cluster deployed this round — see [Deferred: Live Cluster Validation](#deferred-live-cluster-validation).
**Scope note**: This spike evaluates Karmada strictly as a peer to ACM/Rancher/Clusterpedia on the **registry/scope axis** (`scope.ScopeChecker`, ADR-068). It does **not** touch, and is not evaluating, any replacement for the MCP Gateway or the per-cluster K8s/OCP MCP servers — those remain required infrastructure regardless of which `scope.ScopeChecker` backend is selected.

## Goal

Determine whether `karmada-search`'s query API can answer ADR-068's `ScopeChecker.IsManagedResource(ctx, ResourceIdentity{ClusterID, Group, Version, Kind, Namespace, Name})` question — i.e., "does this specific resource, on this specific cluster, carry `kubernaut.ai/managed=true`" — the same way the FMC/ACM/Rancher/Clusterpedia backends already do (see ADR-068 Backends A-D).

## Results Summary

| Step | Question | Result | Notes |
|------|----------|--------|-------|
| S1 | Is `karmada-search` deployed by default? | **PASS** | Deployed by `hack/deploy-karmada.sh` (called from `hack/local-up-karmada.sh`) as a core control-plane component with its own APIService, cert, and etcd-client cert — not an opt-in addon |
| S2 | Does the query API take an explicit `?cluster=` parameter to disambiguate? | **NO** | Neither of `search.karmada.io/v1alpha1`'s two connectors (`Search`, `Proxying`) accepts a cluster parameter in the request |
| S3 | Does the `Proxying` connector disambiguate same-named resources across clusters? | **FAIL** (confirmed in official docs) | Karmada's own FAQ: *"proxy cannot discern the resources with same name across clusters. So get/update/patch/delete and subresources requests will return a conflict error."* This connector is **not usable** for kubernaut's exact-cluster scope check as-is |
| S4 | Does the `Search` (cache) connector disambiguate same-named resources across clusters? | **PASS — via a different mechanism** | `pkg/registry/search/storage/cache.go`'s `getObjectItemsFromClusters` queries **every** registered cluster's local informer lister independently, and returns a `List` (even for a GET-by-name request) with each item annotated `resource.karmada.io/cached-from-cluster: <clusterName>` — filtering that list client-side by the annotation resolves to one cluster's copy, with no conflict error at all |
| S5 | Auth model | **PASS** | Standard Kubernetes aggregated `APIService` (`v1alpha1.search.karmada.io` → `Service karmada-search.karmada-system`) — same RBAC/`SubjectAccessReview` chain as `karmada-apiserver` itself. Structurally identical to **Clusterpedia's** (ADR-068 Backend D) in-cluster-SA auth model, not ACM's GraphQL+bearer-token model |
| S6 | Go client approach | **`client-go` REST client, zero new deps** | Same shape as the existing Clusterpedia adapter sketch in ADR-068 — a raw `GET` against the aggregated API path, unmarshal a `List` |

## Confidence Reassessment

```
Spike Karmada-Search Results:
  S1 (Deployed by default):     PASS — core component, not an addon
  S2 (Cluster-scoped query):    NO explicit param — resolved via S4 instead
  S3 (Proxying disambiguation): FAIL — documented conflict-error on name collision
  S4 (Search/cache disambig.):  PASS — per-item cluster-source annotation, no collision
  S5 (Auth):                    PASS — standard aggregated-APIService RBAC
  S6 (Go client):                Option A — client-go REST GET, no new deps

  Initial confidence (docs-only, before code review): ~40%
    -- the "conflict error" FAQ entry read like a hard blocker
  Post source-code-review confidence: ~80%
    -- found IsManagedResource maps cleanly to the *different*, non-colliding
       Search/cache connector, not the Proxying connector the docs page
       foregrounds
  Remaining 20% gap: never exercised against a live karmada-search instance
  Decision: GO for a live-cluster follow-up spike; NOT YET for implementation
```

## Detailed Findings

### S1: Default Deployment

`hack/deploy-karmada.sh` (invoked by `hack/local-up-karmada.sh`) generates a dedicated cert (`karmada-search`, `karmada-search-etcd-client`), deploys the `karmada-search` component, waits for it to become ready, then applies `artifacts/deploy/karmada-search-apiservice.yaml` and waits for the `v1alpha1.search.karmada.io` `APIService` to report `Available`. It is part of the standard control-plane bring-up, not a separate `karmadactl addons enable` step.

### S2/S3: The `Proxying` Connector — Confirmed Unsuitable As-Is

From the official docs (`karmada.io/docs/userguide/globalview/proxy-global-resource/`), FAQ:

> **What will happen when I access resource with same name across clusters.**
> In this stage, proxy cannot discern the resources with same name across clusters. So get/update/patch/delete and subresources requests will return a conflict error. When list resources, the resources with same name will be returned in item list.

This connector (`search.karmada.io/v1alpha1/proxying/karmada/proxy/...`) is designed for `kubectl`-transparent passthrough (including subresources like pod `log`/`exec`) governed by a `ResourceRegistry`, and it works for **list** operations, but a **get**-by-name request against two clusters that both happen to have e.g. `Pod/nginx-abc` in the same namespace returns an HTTP conflict, not a disambiguated result. Since kubernaut's scope check is inherently a "get exactly this one resource on exactly this one cluster" query, this connector is ruled out for that specific use case — independent of anything Karmada might fix about it later, kubernaut should not build the adapter against a connector whose own maintainers document this failure mode as expected behavior.

### S4: The `Search` (Cache) Connector — The Actual Fit

Reading `pkg/registry/search/storage/search.go` and `cache.go` directly (not just docs) surfaced a second, independent connector under the same API group that the docs page discusses far less prominently:

```
GET /apis/search.karmada.io/v1alpha1/search/cache/<k8s-native-resource-path>
```

Its handler (`SearchREST.newCacheHandler` → `getObjectItemsFromClusters`) does this, verbatim from source:

```go
for _, cluster := range clusters {
    singleClusterManger := r.multiClusterInformerManager.GetSingleClusterManager(cluster.Name)
    ...
    if len(name) > 0 {
        resourceObject, err = objLister.ByNamespace(namespace).Get(name)   // or objLister.Get(name) for cluster-scoped
        if err != nil { continue }                                        // not found on THIS cluster — just skip it
        items = append(items, addAnnotationWithClusterName([]runtime.Object{resourceObject}, cluster.Name)...)
    }
    ...
}
```

Every matching cluster's copy is appended to `items`, each annotated via `addAnnotationWithClusterName` with:

```go
annotations[clusterv1alpha1.CacheSourceAnnotationKey] = clusterName
// clusterv1alpha1.CacheSourceAnnotationKey = "resource.karmada.io/cached-from-cluster"
```

So a request that looks like a "get one object" returns a **List** (`reqResponse{Items []runtime.Object}`), one entry per cluster where that name/namespace/kind exists — with **no collision error**, because the connector never tries to collapse the results into a single object. This sidesteps the Proxying connector's documented failure mode entirely.

### Query Contract (as-built, for the adapter)

**Request** — check whether `Deployment/nginx` in namespace `production` is managed on cluster `prod-east`:

```
GET /apis/search.karmada.io/v1alpha1/search/cache/apis/apps/v1/namespaces/production/deployments/nginx
```

**Response shape**:

```json
{
  "apiVersion": "apps/v1",
  "kind": "DeploymentList",
  "items": [
    {
      "metadata": {
        "name": "nginx",
        "namespace": "production",
        "labels": {"kubernaut.ai/managed": "true"},
        "annotations": {"resource.karmada.io/cached-from-cluster": "prod-east"}
      }
    }
  ]
}
```

**Adapter logic**:
1. `GET` the path above.
2. Filter `items` for `annotations["resource.karmada.io/cached-from-cluster"] == req.ClusterID`.
3. If found, return `labels["kubernaut.ai/managed"] == "true"`.
4. If not found for that cluster: return `false` — **but see the caveat below**, since this is indistinguishable from "not yet cached."

### Critical Caveat: Requires Pre-Registered `ResourceRegistry` Objects

`karmada-search` only caches what a `ResourceRegistry` (`search.karmada.io/v1alpha1`) tells it to, via `spec.resourceSelectors` (GVK + optional namespace) and `spec.targetCluster`. Unlike ACM Search (indexes broadly by default) or FMC (polls the MCP Gateway directly for whatever kubernaut asks about), a Karmada adapter would require the operator to pre-register **every GVK kubernaut might ever scope-check** — Deployments, Pods, StatefulSets, Jobs, Tekton `PipelineRuns`, etc. — the same category of upfront config Clusterpedia's `PediaCluster` sync list already demands (ADR-068 Backend D). An un-registered GVK doesn't error — it silently returns zero items, which is indistinguishable from "exists but not managed." Any implementation of this adapter must fail loud at startup if expected GVKs aren't covered by a `ResourceRegistry` (same fail-closed philosophy as DD-FLEET-003), rather than let this surface as a silent false-negative scope check.

### S5: Auth Model

`v1alpha1.search.karmada.io` is registered as a standard Kubernetes aggregated `APIService` pointing at the `karmada-search` Service. This means access control runs through the same `TokenReview`/`SubjectAccessReview` chain as any other Kubernetes API resource — a Kubernaut ServiceAccount just needs an RBAC grant on the `search.karmada.io` API group, no separate bearer-token/GraphQL auth layer like ACM Search requires. This is the same auth shape as Clusterpedia (Backend D), not ACM (Backend B).

### S6: Go Client Decision

**Option A** (matching the ACM/Clusterpedia precedent): hand-rolled, using `client-go`'s generic REST client (`rest.Interface` / raw `Get().AbsPath(...)`) against the karmada-apiserver kubeconfig — no GraphQL, no new dependency. Response unmarshaling is a plain `runtime.Object` list decode, same pattern already used for Clusterpedia.

## Backend Classification for ADR-068

| Capability | Karmada (this spike) | Clusterpedia (existing Backend D) | ACM Search (existing Backend B) |
|---|---|---|---|
| Query mechanism | K8s Aggregated API (`search.karmada.io`), in-memory informer cache | K8s Aggregated API (`clusterpedia.io`), Postgres/MySQL-backed index | GraphQL (`search-api` service) |
| Auth | In-cluster SA, standard RBAC | In-cluster SA, standard RBAC | SA bearer token, ACM-specific RBAC (`userpermissions`) |
| Pre-registration required | `ResourceRegistry` per GVK/cluster | `PediaCluster` per cluster (with sync resource list) | None (broad indexing by default) |
| Estimated latency | Likely low-single-digit ms (pure informer-cache read, no external DB) — needs live measurement | p95 ~5-30ms | p95 ~10-50ms |

**Conclusion**: Karmada is architecturally much closer to **Clusterpedia** than to ACM — both are aggregated-API-on-the-hub adapters requiring upfront per-GVK/per-cluster registration, differing mainly in backing store (informer cache vs. external DB) and maturity/governance (Karmada is CNCF-incubating; Clusterpedia is CNCF sandbox). This confirms the framing from the prior discussion: Karmada would be a `pkg/fleet/controlplane/karmada/` adapter implementing `scope.ScopeChecker`, selected via `fleet.backend: "karmada"` in the existing factory (`pkg/fleet/scope_factory.go`) — zero changes to GW, RO, KA, SP, WE, AF, or EM, and zero interaction with the MCP Gateway or per-cluster K8s/OCP MCP servers.

## Deferred: Live Cluster Validation

Not performed this round. `hack/local-up-karmada.sh` builds ~10 Go binaries from source and provisions 4 Kind clusters (1 host + 2 push-mode + 1 pull-mode member) — a heavy footprint to add alongside this session's already-running `kind` and `kubernaut-2159-smoke` Kind clusters, and one whose build+bootstrap time alone risks exceeding a 2-hour spike time-box. Recommended as a follow-up spike, mirroring `spike-acm-search`'s three-phase progression (docs/code → scoped-RBAC live validation → real production Go client validation):

1. Deploy via `hack/local-up-karmada.sh` (or the cheaper `karmadactl init` against a single existing cluster + `karmadactl join` for one member, if a full 4-cluster topology isn't needed to answer the open question).
2. Apply a `ResourceRegistry` for one test GVK across all member clusters.
3. `curl`/`kubectl get --raw` the `search/cache/...` path for a resource that exists on exactly one member cluster; confirm the annotation + label shape matches this document.
4. Seed a same-named/same-namespaced resource on **two** member clusters and repeat step 3 — this is the single most important behavior to confirm live, since it's the crux of the S3/S4 finding that makes this adapter viable at all.
5. Time a handful of requests to establish the real latency class for the ADR-068 Backend Comparison Matrix.
6. Validate the RBAC grant needed (exact API group/resource/verb) with a scoped (non-`cluster-admin`) ServiceAccount, the same way the ACM spike's "Production RBAC Validation" phase did.

## Decision

**GO for a follow-up live-cluster spike; NOT YET for implementation.** This source-code review resolved the central open question — whether Karmada's query API can disambiguate same-named resources across clusters for an exact-cluster scope check — with a concrete, positive answer, but through a different connector (`Search`/cache) than the one Karmada's own documentation foregrounds (`Proxying`, which is confirmed unsuitable). That divergence between docs and actual behavior is exactly the kind of thing this project's spike discipline exists to catch before committing to `pkg/fleet/controlplane/karmada/client.go`.
