# DD-GATEWAY-007: Fallback Namespace Strategy for Cluster-Scoped Signals

## Status
**DEPRECATED** (February 2026)
**Originally Approved**: 2025-10-31
**Deprecated**: February 8, 2026
**Superseded By**: [ADR-053: Resource Scope Management](ADR-053-resource-scope-management.md)

> **DEPRECATION NOTICE**: This design decision has been deprecated. The namespace
> fallback feature has been removed from the Gateway codebase. ADR-053 Resource
> Scope Management now handles unmanaged namespaces by rejecting signals upstream
> (at the Gateway scope validation layer), before CRD creation is ever attempted.
> The Remediation Orchestrator (BR-SCOPE-010) also blocks unmanaged resources,
> making any fallback-created RemediationRequests useless.
>
> **Reason**: Scope validation makes namespace fallback redundant and creates
> technical debt. If a namespace does not exist or is not labeled with
> `kubernaut.ai/managed=true`, the signal is rejected with HTTP 200 (informational
> rejection). This is the correct behavior -- creating a RemediationRequest in a
> fallback namespace for a resource whose namespace no longer exists would only
> cause the RO to fail when it double-checks the managed label on the resource.
>
> **Code Removal**: All fallback logic (`handleNamespaceNotFoundError`,
> `isNamespaceNotFoundError`, `FallbackNamespace` config field, `GetPodNamespace()`)
> has been removed from `pkg/gateway/processing/crd_creator.go`,
> `pkg/gateway/config/config.go`, and `pkg/gateway/server.go`.
>
> **Test Updates**: E2E tests 22 and 27 (dedicated fallback tests) have been removed.
> Unit and integration tests referencing fallback have been updated.

---

## Context & Problem

When the Gateway service receives a signal (alert or event) for a Kubernetes namespace that doesn't exist, it must decide where to create the RemediationRequest CRD. This scenario occurs in two primary cases:

1. **Cluster-Scoped Signals**: Alerts about cluster-level resources (e.g., `NodeNotReady`, `ClusterMemoryPressure`) that don't have a specific namespace
2. **Invalid Namespace References**: Alerts referencing namespaces that have been deleted or never existed

**Key Requirements**:
- **Multi-Tenancy**: Preserve namespace isolation for security and RBAC
- **Cluster-Scoped Support**: Handle cluster-level alerts gracefully
- **Audit Trail**: Maintain origin namespace information for troubleshooting
- **Infrastructure Consistency**: Align with Kubernaut's architectural patterns

**Current Behavior** (pre-v2.22):
- Fallback namespace: `default`
- No labels to preserve origin namespace
- No distinction between cluster-scoped and invalid namespace scenarios

---

## Alternatives Considered

### Alternative 1: Always Use Origin Namespace (Current Behavior)
**Approach**: Create CRD in the signal's origin namespace, fail if namespace doesn't exist

**Pros**:
- ✅ Perfect multi-tenancy (namespace isolation)
- ✅ Simple RBAC (per-namespace permissions)
- ✅ Clear ownership (CRD in same namespace as alert)

**Cons**:
- ❌ Fails for cluster-scoped signals (no namespace)
- ❌ Fails for deleted namespaces (alert fired before deletion)
- ❌ Blocks remediation for valid cluster-level issues

**Confidence**: 60% (rejected - doesn't handle cluster-scoped signals)

---

### Alternative 2: Fallback to `default` Namespace (Pre-v2.22)
**Approach**: Create CRD in `default` namespace when origin namespace doesn't exist

**Pros**:
- ✅ Simple implementation (single fallback)
- ✅ Handles cluster-scoped signals
- ✅ Prevents CRD creation failures

**Cons**:
- ❌ Pollutes `default` namespace (not infrastructure-focused)
- ❌ Inconsistent with Kubernaut architecture (Kubernaut uses `kubernaut-system`)
- ❌ No audit trail (origin namespace lost)
- ❌ RBAC complexity (operators need access to `default`)

**Confidence**: 70% (rejected - architectural inconsistency)

---

### Alternative 3: Fallback to `kubernaut-system` with Labels (APPROVED)
**Approach**: Create CRD in `kubernaut-system` namespace with labels preserving origin information

**Pros**:
- ✅ Infrastructure consistency (aligns with Kubernaut architecture)
- ✅ Audit trail (labels preserve origin namespace)
- ✅ Cluster-scoped signal support (proper home for cluster-level alerts)
- ✅ Clear RBAC (Kubernaut infrastructure namespace)
- ✅ Distinguishes cluster-scoped vs invalid namespace (via labels)

**Cons**:
- ⚠️ Requires label management (minimal complexity)
- ⚠️ Operators need access to `kubernaut-system` (already required for Kubernaut)

**Confidence**: 95% (approved)

---

## Decision

**APPROVED: Alternative 3** - Fallback to `kubernaut-system` Namespace with Labels

**Rationale**:
1. **Infrastructure Consistency**: `kubernaut-system` is the proper home for Kubernaut infrastructure components
2. **Audit Trail**: Labels preserve origin namespace for troubleshooting and audit
3. **Cluster-Scoped Support**: Handles cluster-level alerts (NodeNotReady, etc.) gracefully
4. **RBAC Alignment**: Operators already have access to `kubernaut-system` for Kubernaut operations

**Key Insight**: Cluster-scoped signals are infrastructure concerns, not application concerns. Placing them in `kubernaut-system` aligns with their nature.

---

## Implementation

### Primary Implementation Files

**File**: `pkg/gateway/processing/crd_creator.go`
**Function**: `CreateRemediationRequest()`

**Changes**:
1. Change fallback namespace from `"default"` to `"kubernaut-system"`
2. Add labels to preserve origin namespace information
3. Update log messages to reflect new fallback namespace

**Code Changes**:
```go
// Before (v2.21 and earlier)
if strings.Contains(err.Error(), "namespaces") && strings.Contains(err.Error(), "not found") {
    rr.Namespace = "default"  // ❌ Old fallback
    // No labels added
}

// After (v2.22+)
if strings.Contains(err.Error(), "namespaces") && strings.Contains(err.Error(), "not found") {
    rr.Namespace = "kubernaut-system"  // ✅ New fallback

    // Add labels to preserve origin namespace information
    rr.Labels["kubernaut.io/origin-namespace"] = signal.Namespace
    rr.Labels["kubernaut.io/cluster-scoped"] = "true"
}
```

### Data Flow

**Scenario 1: Cluster-Scoped Signal (NodeNotReady)**
```
1. Signal received: namespace="" (cluster-scoped)
2. CRD creation attempted in namespace="" → fails (namespace not found)
3. Fallback triggered: CRD created in "kubernaut-system"
4. Labels added:
   - kubernaut.io/origin-namespace: "" (empty for cluster-scoped)
   - kubernaut.io/cluster-scoped: "true"
```

**Scenario 2: Invalid Namespace (Deleted After Alert)**
```
1. Signal received: namespace="deleted-app"
2. CRD creation attempted in "deleted-app" → fails (namespace not found)
3. Fallback triggered: CRD created in "kubernaut-system"
4. Labels added:
   - kubernaut.io/origin-namespace: "deleted-app" (preserved for audit)
   - kubernaut.io/cluster-scoped: "true"
```

### Label Schema

| Label | Purpose | Example Values |
|-------|---------|----------------|
| `kubernaut.io/origin-namespace` | Preserve original namespace from signal | `"production"`, `"deleted-app"`, `""` (cluster-scoped) |
| `kubernaut.io/cluster-scoped` | Indicate CRD is for cluster-level issue | `"true"` (always set for fallback CRDs) |

---

## Consequences

### Positive
- ✅ **Infrastructure Alignment**: CRDs in proper infrastructure namespace (`kubernaut-system`)
- ✅ **Audit Trail**: Origin namespace preserved in labels for troubleshooting
- ✅ **Cluster-Scoped Support**: Handles cluster-level alerts gracefully
- ✅ **RBAC Simplicity**: Operators already have access to `kubernaut-system`
- ✅ **Clear Semantics**: `cluster-scoped` label makes intent explicit

### Negative
- ⚠️ **Label Management**: Requires label handling logic (minimal complexity)
- ⚠️ **Migration**: Existing CRDs in `default` namespace not automatically migrated (acceptable - pre-release)

### Neutral
- 🔄 **RBAC Requirement**: Operators need `kubernaut-system` access (already required)
- 🔄 **Query Pattern**: Operators query `kubernaut-system` for cluster-scoped issues

---

## Validation Results

### Test Coverage
**File**: `test/integration/gateway/error_handling_test.go`
**Test**: `handles namespace not found by using kubernaut-system namespace fallback`

**Validation Points**:
- ✅ CRD created in `kubernaut-system` namespace (not `default`)
- ✅ Label `kubernaut.io/cluster-scoped` set to `"true"`
- ✅ Label `kubernaut.io/origin-namespace` preserves original namespace
- ✅ Graceful fallback (no error returned to client)

**Test Results**: ✅ All validation points passing

### Confidence Assessment Progression
- Initial assessment: 85% confidence (architectural alignment)
- After implementation review: 90% confidence (clean implementation)
- After test validation: 95% confidence (production-ready)

---

## Related Decisions
- **Builds On**: [DD-GATEWAY-001](DD-GATEWAY-001-adapter-endpoints.md) - Adapter architecture
- **Supports**: BR-GATEWAY-011 (CRD creation), BR-GATEWAY-023 (namespace handling)
- **Related**: [ADR-015](../adrs/ADR-015-signal-terminology.md) - Signal terminology

---

## Review & Evolution

### When to Revisit
- If cluster-scoped signal volume exceeds 10% of total signals (may need dedicated namespace)
- If RBAC requirements change (e.g., operators shouldn't access `kubernaut-system`)
- If audit requirements change (e.g., need separate audit CRDs)

### Success Metrics
- **Cluster-Scoped Signal Handling**: 100% success rate (no CRD creation failures)
- **Audit Trail Accuracy**: 100% of fallback CRDs have origin namespace labels
- **Operator Satisfaction**: Positive feedback on troubleshooting cluster-scoped issues

---

## Migration Guide

### For Operators

**Query Cluster-Scoped CRDs**:
```bash
# Find all cluster-scoped RemediationRequests
kubectl get remediationrequests -n kubernaut-system -l kubernaut.io/cluster-scoped=true

# Find CRDs for specific origin namespace
kubectl get remediationrequests -n kubernaut-system -l kubernaut.io/origin-namespace=production
```

**No Action Required**:
- Existing CRDs in `default` namespace remain (pre-release, no migration needed)
- New cluster-scoped signals automatically use `kubernaut-system`

### For Developers

**Update Tests**:
```go
// Before (v2.21)
Expect(crd.Namespace).To(Equal("default"))

// After (v2.22)
Expect(crd.Namespace).To(Equal("kubernaut-system"))
Expect(crd.Labels["kubernaut.io/cluster-scoped"]).To(Equal("true"))
Expect(crd.Labels["kubernaut.io/origin-namespace"]).To(Equal(originalNamespace))
```

---

## References

### Implementation Files
- `pkg/gateway/processing/crd_creator.go` (lines 236-255)
- `test/integration/gateway/error_handling_test.go` (lines 277-358)
- `test/integration/gateway/suite_test.go` (line 44)

### Documentation
- [FALLBACK_NAMESPACE_CHANGE_IMPACT.md](../services/stateless/gateway-service/FALLBACK_NAMESPACE_CHANGE_IMPACT.md) - Impact analysis
- [GATEWAY_PRIORITY1_TESTS_COMPLETE.md](../services/stateless/gateway-service/GATEWAY_PRIORITY1_TESTS_COMPLETE.md) - Test implementation summary

### Business Requirements
- BR-GATEWAY-011: RemediationRequest CRD creation
- BR-GATEWAY-023: Namespace handling and validation

---

**Document Maintainer**: Kubernaut Architecture Team
**Created**: 2025-10-31
**Status**: **DEPRECATED** (February 2026) - Superseded by ADR-053 Resource Scope Management
**Version**: 1.1 (deprecation update)

