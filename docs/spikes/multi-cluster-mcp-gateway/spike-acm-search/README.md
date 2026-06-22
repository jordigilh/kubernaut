# Spike: ACM Search GraphQL Contract Validation

**Date**: 2026-06-22
**Cluster**: OCP 4.21.5 (parodos.dev)
**ACM Version**: 2.17.0

## Goal

Validate the ACM Search GraphQL API contract documented in ADR-068 against a
live ACM instance. Answer four yes/no questions to raise confidence from 55% to
90%+ before implementing the ACM adapter.

## Results Summary

| Step | Question | Result | Notes |
|------|----------|--------|-------|
| S1 | Can we install ACM on OCP? | **PASS** | ACM 2.17.0 installed, MCH Running in ~5 min |
| S2 | Does `search` return `{ searchResult: { count } }`? | **PASS** | Exact match with ADR-068 |
| S3 | Does the response shape match ADR-068? | **PASS** | Positive (count=1) and negative (count=0) confirmed |
| S4 | Does `search` return `{ searchResult: { items } }` for clusters? | **PASS** | Items are flat `map[string]string` |
| S5 | Can a ServiceAccount authenticate with bearer token? | **PASS** | SA tokens authenticate (HTTP 200); authorization requires `cluster-admin` |
| S6 | Go client approach? | **Option A** | Hand-rolled HTTP+JSON; zero new dependencies |

### Confidence Reassessment

```
Spike S-ACM-SEARCH Results:
  S1 (Install):      PASS — ACM 2.17.0, MCH Running, 28 pods healthy
  S2 (scopeCheck):   PASS — exact contract match: {"data":{"searchResult":[{"count":1}]}}
  S3 (Response):     PASS — positive count=1, negative count=0, unknown-cluster count=0
  S4 (Cluster list): PASS — items are flat map[string]string with name, cluster, kind fields
  S5 (Auth flow):    PASS — SA token authenticates; requires cluster-admin for result visibility
  S6 (Go client):    Option A — hand-rolled HTTP+JSON (2 static queries, flat responses)

  Initial spike confidence: 55% -> 92%
  Decision: PROCEED to Phase 5 implementation
```

### Production RBAC Validation (2026-06-22)

Following Phase 5 implementation, the remaining 8% risk (unvalidated production RBAC
without cluster-admin) was closed by a live validation on the same OCP 4.21.5 / ACM 2.17.0
cluster. Key findings:

| Step | Test | Result | Detail |
|------|------|--------|--------|
| V1 | Preflight | PASS | Cluster accessible, ACM 2.17.0 Running |
| V2 | Create scoped SA (no cluster-admin) | PASS | SA with `global-search-user` + `kubernaut-fleet-search-reader` |
| V3 | Baseline: FineGrainedRbac=false | PASS | `count: 0` as expected |
| V4 | Enable FineGrainedRbac=true via MCH | PASS | Enabled via MCH component override; search-api shows `FineGrainedRbac: true` |
| V5 | Scoped RBAC + FineGrainedRbac=true | FAIL | `userpermissions` API returned empty — need managed cluster visibility |
| V6a | Add `userpermissions` list access | PASS | Cleared the "refreshing user permissions" error |
| V6b | `view` RoleBinding in managed cluster ns | **PASS** | `count: 1` — production model validated |

```
Final confidence: 92% -> 100%
Production RBAC model: VALIDATED
```

**Critical finding**: The `userpermissions` virtual API (served by `ocm-proxyserver`)
only recognizes the built-in Kubernetes aggregate roles (`admin`, `view`, `edit`) when
bound in managed cluster namespaces. Custom ClusterRoles are invisible to this API.
The `view` RoleBinding in the managed cluster namespace is the minimum-privilege
mechanism for granting search result visibility.

**What does NOT work**:
- Custom ClusterRole bound in managed cluster namespace → `userpermissions` returns empty
- `MulticlusterRoleAssignment` CRD alone → creates ClusterPermission but doesn't affect `userpermissions`
- Search CR annotation `fine-grained-rbac: "true"` → does not propagate to search-api pod
- Search CR spec field `FineGrainedRbac: true` → unknown field, ignored

**What works (validated)**:
1. MCH component `fine-grained-rbac` enabled → search-api shows `FineGrainedRbac: true`
2. `global-search-user` ClusterRoleBinding → API endpoint access
3. `userpermissions` list ClusterRole + ClusterRoleBinding → permission resolution
4. `view` RoleBinding in managed cluster namespace → search result visibility

Full production setup guide in ADR-068.

## Detailed Findings

### S1: ACM Installation

- **Namespace**: `open-cluster-management`
- **Channel**: `release-2.17`
- **MCH reconciliation**: ~5 minutes (brief transient `Error` phase during CRD setup is normal)
- **local-cluster**: Joined=True, Available=True
- **Search stack**: search-api (4010/TCP), search-indexer (3010/TCP), search-collector (5010/TCP), search-postgres (5432/TCP)

### S2-S3: GraphQL Contract Validation

**scopeCheck query** (positive case):
```graphql
query scopeCheck($input: [SearchInput]) {
  searchResult: search(input: $input) { count }
}
```

Variables:
```json
{
  "input": [{
    "filters": [
      {"property": "kind",      "values": ["Deployment"]},
      {"property": "name",      "values": ["nginx"]},
      {"property": "namespace", "values": ["spike-acm-test"]},
      {"property": "cluster",   "values": ["local-cluster"]},
      {"property": "label",     "values": ["kubernaut.ai/managed=true"]}
    ],
    "limit": 1
  }]
}
```

**Actual response**: `{"data":{"searchResult":[{"count":1}]}}`
**ADR-068 expected**: `{"data":{"searchResult":[{"count":1}]}}`
**Delta**: None. Exact match.

**Negative case** (nonexistent resource): `{"data":{"searchResult":[{"count":0}]}}`
**Unknown cluster**: `{"data":{"searchResult":[{"count":0}]}}`

### S4: Cluster Listing

**Cluster listing query**:
```graphql
query listClusters($input: [SearchInput]) {
  searchResult: search(input: $input) { items }
}
```

**Actual item shape** (key fields for the adapter):

| Field | Type | Example Value | Usage |
|-------|------|---------------|-------|
| `name` | string | `"local-cluster"` | Cluster identifier |
| `cluster` | string | `"local-cluster"` | Same as name |
| `kind` | string | `"Cluster"` | Resource type discriminator |
| `apiEndpoint` | string | `"https://api.dev.redhat-internal.com:6443"` | Informational |
| `kubernetesVersion` | string | `"v1.34.4"` | Informational |
| `label` | string | semicolon-delimited key=value pairs | Contains `name=local-cluster` |
| `ManagedClusterConditionAvailable` | string | `"True"` | Cluster health |
| `ManagedClusterJoined` | string | `"True"` | Registration status |
| `HubAcceptedManagedCluster` | string | `"True"` | Hub acceptance |

All item fields are **string-typed** (flat `map[string]string`). No nested objects.

### S5: Auth Flow

**Authentication**: SA bearer tokens are accepted by the Search API (HTTP 200).

**Authorization model** (critical finding):

1. **Default mode** (`FineGrainedRbac: false`): The Search API uses a coarse
   authorization model. Only users with `cluster-admin` privileges can see
   search results. Non-admin SAs receive empty result sets (count=0) regardless
   of their Kubernetes RBAC bindings.

2. **Fine-grained mode** (`FineGrainedRbac: true`): The Search API checks the
   virtual `userpermissions.clusterview.open-cluster-management.io` API. This
   API is populated by ACM based on group membership (e.g.,
   `system:cluster-admins`), not standard ClusterRoleBindings. Narrower access
   requires investigation into ACM's `MulticlusterRoleAssignment` CRD
   (`rbac.open-cluster-management.io/v1beta1`).

**Spike RBAC** (used during validation — NOT for production):
```yaml
# This cluster-admin binding was used only for spike validation.
# Production deployments MUST use scoped RBAC.
# See ADR-068 "ACM Search Production Setup Guide" for the production setup.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-search-reader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: kubernaut-search-reader
  namespace: open-cluster-management
```

**Production RBAC**: Use `FineGrainedRbac: true` + scoped `list` on only the
resource types Kubernaut manages + `global-search-user` for API access.
Full step-by-step guide in ADR-068 and `rbac.yaml` in this directory.

**ACM ClusterRoles for Search** (documented, not `open-cluster-management:search-access`):
- `global-search-user` — grants `get` on `searches` and `searches/allManagedData`
- `search` — internal role used by the search-api pod itself

**TLS**:
- Server certificate: `CN=search-search-api.open-cluster-management.svc`
- Issuer: `CN=openshift-service-serving-signer` (OpenShift service CA)
- Secret: `search-api-certs` in `open-cluster-management` namespace
- In-cluster: inject service CA into a ConfigMap with annotation
  `service.beta.openshift.io/inject-cabundle: "true"`

### S6: Go Client Decision

**Decision**: Option A — Hand-rolled HTTP+JSON

**Rationale**:
- Response shape is trivially flat (confirmed by S2-S4)
- Only 2 static queries with variable substitution
- Zero new dependencies
- Consistent with existing `pkg/fleet/fmc/http_client.go` pattern

**Go type sketch**:
```go
type graphQLRequest struct {
    Query     string         `json:"query"`
    Variables map[string]any `json:"variables"`
}

type searchResponse struct {
    Data struct {
        SearchResult []struct {
            Count int                 `json:"count"`
            Items []map[string]string `json:"items,omitempty"`
        } `json:"searchResult"`
    } `json:"data"`
    Errors []struct {
        Message string `json:"message"`
    } `json:"errors,omitempty"`
}
```
