# Test Plan — #1477: Strip Namespace for Cluster-Scoped Resources

**IEEE 829 Compliant** | **Issue**: [#1477](https://github.com/jordigilh/kubernaut/issues/1477) | **Milestone**: v1.6

## 1. Test Plan Identifier

TP-1477-CLUSTER-SCOPE-NAMESPACE-STRIP

## 2. Introduction

### 2.1 Purpose

When the LLM agent provides a namespace for a cluster-scoped Kubernetes resource (e.g., `ClusterOperator`, `Node`), 4 AF tools either reject the call with a hard error or query the wrong scope. This wastes an LLM tool call round-trip and produces unnecessary error noise. The fix replaces hard errors with self-healing namespace stripping: detect cluster scope via RESTMapper, clear the namespace, log a warning, and proceed.

### 2.2 Objectives

1. **Self-healing normalization**: Strip namespace for cluster-scoped resources and proceed without error.
2. **Scope-aware routing**: Use cluster-scoped client path (no `.Namespace()`) when resource is not namespaced.
3. **Observability**: Log namespace stripping with kind, apiVersion, and stripped namespace value (AU-3).
4. **No regression**: Namespaced resources continue to require a namespace (hard error on missing).
5. **Accurate scope detection**: Use RESTMapper (dynamic) instead of heuristic (empty namespace check).

### 2.3 Business Requirements

- BR-AF-INPUT-030: Self-healing input normalization for cluster-scoped resources
- BR-AF-INPUT-010: Input validation (namespaced resource without namespace remains a hard error)

## 3. Features to be Tested

- F-1: `HandleInvestigateAlert` strips namespace for cluster-scoped resource and proceeds
- F-2: `HandleKubectlGet` uses cluster-scoped client when RESTMapper identifies non-namespaced kind
- F-3: `HandleKubectlList` uses cluster-scoped client when RESTMapper identifies non-namespaced kind
- F-4: `HandleInvestigateMCP` sets `ClusterScoped: true` based on RESTMapper (not heuristic)
- F-5: Namespaced resource without namespace still returns hard error (no regression)
- F-6: Warning log emitted on namespace stripping (AU-3 observability)

## 4. Features Not to be Tested

- KA's `scopedClient` / `normalizeNamespace` (already correct)
- Gateway CRD creation logic (already handles cluster-scoped correctly)
- Signal Processing owner chain (uses static map, separate concern)
- LLM prompt improvements (defense-in-depth, separate PR)

## 5. Approach

### Test Pyramid

| Tier | Scope | Count |
|---|---|---|
| Unit | Tool-level logic with fake RESTMapper / fake dynamic client | 6 |
| Integration | Production dispatch path through MCP bridge | 3 |

### FedRAMP Control Mapping

| Control | Objective | Behavioral Assurance | Test IDs |
|---|---|---|---|
| SC-5 | DoS Protection | Self-healing prevents wasted LLM round-trips; availability preserved | UT-AF-1477-001, UT-AF-1477-002, UT-AF-1477-003 |
| SI-10 | Information Input Validation | Invalid scope combination normalized (not silently accepted or hard-rejected) | UT-AF-1477-001, UT-AF-1477-005 |
| AU-3 | Content of Audit Records | Namespace stripping logged with kind, apiVersion, stripped namespace | UT-AF-1477-006 |

## 6. Test Cases

### 6.1 Unit Tests

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| UT-AF-1477-001 | `HandleInvestigateAlert` strips namespace for cluster-scoped resource | `Node` + `namespace=default` -> result succeeds, no error, RR created with `ClusterScoped: true` | SC-5, SI-10 |
| UT-AF-1477-002 | `HandleKubectlGet` succeeds for cluster-scoped resource with namespace | `Node` + `name=worker-1` + `namespace=default` -> Get succeeds using cluster-scoped client | SC-5 |
| UT-AF-1477-003 | `HandleKubectlList` succeeds for cluster-scoped resource with namespace | `Namespace` + `namespace=default` -> List succeeds using cluster-scoped client | SC-5 |
| UT-AF-1477-004 | `HandleInvestigateMCP` sets ClusterScoped correctly via RESTMapper | `Node` + `namespace=kube-system` -> RR created with `ClusterScoped: true` | SI-10 |
| UT-AF-1477-005 | Namespaced resource without namespace still errors | `Deployment` + `namespace=""` -> hard error "namespaced but namespace was not provided" | SI-10 |
| UT-AF-1477-006 | Namespace stripping is logged | Logger captures warning with kind, apiVersion, stripped_namespace fields | AU-3 |

### 6.2 Integration Tests

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| IT-AF-1477-001 | `kubernaut_investigate_alert` through MCP bridge with cluster-scoped+namespace | Tool call dispatched through production bridge -> succeeds, namespace stripped | SC-5, SI-10 |
| IT-AF-1477-002 | `kubectl_get` through MCP bridge with cluster-scoped+namespace | Tool call with `Node/worker-1/default` -> Get succeeds | SC-5 |
| IT-AF-1477-003 | `kubectl_list` through MCP bridge with cluster-scoped+namespace | Tool call with `Namespace/default` -> List succeeds | SC-5 |

## 7. Test Environment

### Unit Tests
- Fake `meta.RESTMapper` (from `meta.NewDefaultRESTMapper`) with Node/Namespace registered as cluster-scoped
- Fake `dynamic.Interface` (from `dynamicfake.NewSimpleDynamicClient`) with pre-populated resources
- Ginkgo/Gomega BDD framework

### Integration Tests
- Real MCP bridge handler stack (same as `mcp_bridge_integration_test.go`)
- `httptest.NewServer` with production router/middleware
- Fake K8s clients wired through production config path

## 8. Pass/Fail Criteria

- All 9 test cases pass (6 UT + 3 IT)
- Zero regressions in existing tests (`af_investigate_alert_test.go`, `kubectl_get_test.go`, `kubectl_list_test.go`)
- `go build ./...` clean
- `golangci-lint run --timeout=5m` clean

## 9. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| RESTMapper unavailable at runtime | Low | Low | Fail-open: if mapper is nil or lookup errors, preserve original behavior |
| Namespace stripping masks legitimate errors | Low | Medium | Warning log ensures SRE visibility; metrics counter on strip events |
| Existing tests expect error on cluster-scoped+namespace | Certain | Low | Update UT-AF-1372-056 to assert strip behavior |
