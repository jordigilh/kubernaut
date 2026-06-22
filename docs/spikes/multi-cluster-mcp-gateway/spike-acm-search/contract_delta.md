# Contract Delta: ADR-068 vs Actual ACM Search API (v2.17.0)

## Summary

The ADR-068 GraphQL contract is **accurate** for the core query shapes. Two
non-breaking deltas were identified.

## scopeCheck Query

| Aspect | ADR-068 | Actual (v2.17.0) | Delta |
|--------|---------|-------------------|-------|
| Top-level field | `data.searchResult` (array) | `data.searchResult` (array) | None |
| Element shape | `{ count: int }` | `{ count: int }` | None |
| Positive case | `count > 0` | `count: 1` | None |
| Negative case | `count == 0` | `count: 0` | None |
| Label filter | `kubernaut.ai/managed=true` | Works correctly | None |
| Unknown cluster | Not specified | `count: 0` (no error) | Graceful |

## Cluster Listing Query

| Aspect | ADR-068 | Actual (v2.17.0) | Delta |
|--------|---------|-------------------|-------|
| Top-level field | `data.searchResult` (array) | `data.searchResult` (array) | None |
| Element shape | `{ items: [...] }` | `{ items: [...] }` | None |
| Item type | Not specified | flat `map[string]string` | **NEW INFO** |
| Cluster ID field | Not specified | `name` and `cluster` (both present, same value) | **NEW INFO** |
| Status fields | Not specified | `ManagedClusterConditionAvailable`, `ManagedClusterJoined`, etc. | **NEW INFO** |

### Delta D1: Item Shape (Non-breaking, Additive)

ADR-068 did not specify the exact item fields for cluster listing. The actual
item shape is a flat `map[string]string` with the following key fields:

- `name` — cluster identifier (e.g., "local-cluster")
- `cluster` — same as `name`
- `kind` — always "Cluster" for this query
- `apiEndpoint` — Kubernetes API URL
- `kubernetesVersion` — cluster K8s version
- `label` — semicolon-delimited label string
- `ManagedClusterConditionAvailable` — "True"/"False"
- `ManagedClusterJoined` — "True"/"False"
- `HubAcceptedManagedCluster` — "True"/"False"

**Action**: Update ADR-068 Section 6.3 with the validated item field list.

### Delta D2: RBAC Model (Non-breaking, Deployment Concern)

ADR-068 referenced `open-cluster-management:search-access` as the ClusterRole
for search access. This ClusterRole **does not exist** in ACM 2.17.0.

Actual ClusterRoles:
- `global-search-user` — grants `get` on `searches` and `searches/allManagedData`
- `search` — internal role for the search-api pod

Additionally, the authorization model uses an internal `userpermissions` virtual
API that requires `cluster-admin` for result visibility in the default
configuration (`FineGrainedRbac: false`).

**Action**: Update ADR-068 Section 6.4 with:
1. Correct ClusterRole name (`global-search-user`)
2. Note that `cluster-admin` is the minimum viable RBAC
3. Document the `FineGrainedRbac` setting and `userpermissions` API

## GraphQL Endpoint

| Aspect | ADR-068 | Actual (v2.17.0) | Delta |
|--------|---------|-------------------|-------|
| Path | `/searchapi/graphql` | `/searchapi/graphql` | None |
| Protocol | HTTPS | HTTPS (TLSv1.3) | None |
| Port | 4010 | 4010 | None |
| Service name | `search-search-api` | `search-search-api` | None |

## TLS (New Information)

Not covered in ADR-068. Actual findings:

- Server cert: `CN=search-search-api.open-cluster-management.svc`
- Issuer: `openshift-service-serving-signer`
- Validity: 2 years (auto-rotated by OpenShift)
- In-cluster CA: inject via `service.beta.openshift.io/inject-cabundle: "true"` ConfigMap annotation

**Action**: Add TLS CA configuration to ADR-068 deployment section.
