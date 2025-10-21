# DD-INFRA-001: Consolidated Namespace Strategy

## Status
**✅ APPROVED** (2025-10-21)
**Last Reviewed**: 2025-10-21
**Confidence**: 95%

### Context & Problem

The original namespace strategy (`docs/architecture/NAMESPACE_STRATEGY.md`) used a dual-namespace approach:
- `prometheus-alerts-slm`: Stateless HTTP services
- `kubernaut-system`: CRD controllers

**Problem**: This created unnecessary complexity and confusion about service placement.

**Key Requirements**:
- Simplify namespace strategy
- Consolidate all Kubernaut platform services into a single namespace
- Isolate notification services for security and operational reasons

### Alternatives Considered

#### Alternative 1: Keep Dual-Namespace (prometheus-alerts-slm + kubernaut-system)
**Approach**: Maintain current split between stateless services and controllers

**Pros**:
- ✅ Clear separation between service types
- ✅ Already documented and implemented in some services

**Cons**:
- ❌ Increases operational complexity
- ❌ Confusing service discovery (services.prometheus-alerts-slm vs services.kubernaut-system)
- ❌ Harder to manage RBAC and NetworkPolicies across namespaces
- ❌ Project name confusion (`prometheus-alerts-slm` vs `kubernaut`)

**Confidence**: 60% (rejected - too complex)

---

#### Alternative 2: Single Namespace (kubernaut-system only)
**Approach**: Consolidate ALL services and controllers into `kubernaut-system`

**Pros**:
- ✅ Maximum simplicity
- ✅ Easy service discovery (all services in one namespace)
- ✅ Simplified RBAC and NetworkPolicies
- ✅ Clear project identity (kubernaut)

**Cons**:
- ❌ No isolation for notification services
- ❌ Notifications handle sensitive escalation data
- ❌ Harder to apply different security policies

**Confidence**: 70% (rejected - lacks notification isolation)

---

#### Alternative 3: Consolidated with Notification Isolation (APPROVED)
**Approach**: Use two namespaces with clear rationale
- `kubernaut-system`: ALL services and controllers
- `kubernaut-notifications`: ONLY notification services

**Pros**:
- ✅ Simple default: everything goes to `kubernaut-system`
- ✅ Clear exception: notifications isolated for security
- ✅ Easy service discovery within main namespace
- ✅ Notification isolation enables:
  - Stricter egress NetworkPolicies
  - Separate RBAC for sensitive escalation logic
  - Independent scaling and monitoring
- ✅ Future-proof for notification service expansion

**Cons**:
- ⚠️ Two namespaces (minimal complexity vs Alternative 1)

**Confidence**: 95% (approved)

---

### Decision

**APPROVED: Alternative 3** - Consolidated Namespace Strategy with Notification Isolation

**Rationale**:
1. **Simplicity First**: Default to `kubernaut-system` for all services
2. **Security Isolation**: Notifications handle sensitive escalation → deserve isolation
3. **Operational Clarity**: Easy to remember "everything in system, except notifications"
4. **Future-Proof**: Allows notification service expansion without namespace sprawl

**Key Insight**: The right number of namespaces is "as few as possible, as many as necessary". Two namespaces with clear rationale is superior to arbitrary splitting by service type.

### Implementation

**Namespace Definitions**:

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-system
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: platform
    app.kubernetes.io/part-of: kubernaut
---
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-notifications
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: notifications
    app.kubernetes.io/part-of: kubernaut
```

**Service Allocation**:

| Service/Controller | Namespace | Rationale |
|---|---|---|
| **Gateway** | `kubernaut-system` | Core API gateway |
| **Context API** | `kubernaut-system` | Historical incident context |
| **Data Storage** | `kubernaut-system` | Audit trail persistence |
| **HolmesGPT API** | `kubernaut-system` | AI analysis service |
| **Dynamic Toolset** | `kubernaut-system` | Tool configuration service |
| **Notification** | `kubernaut-notifications` | ⚠️ **ISOLATED** - Handles sensitive escalations |
| **RemediationOrchestrator** | `kubernaut-system` | CRD controller |
| **RemediationProcessor** | `kubernaut-system` | CRD controller |
| **AIAnalysis** | `kubernaut-system` | CRD controller |
| **WorkflowExecution** | `kubernaut-system` | CRD controller |
| **KubernetesExecutor** | `kubernaut-system` | CRD controller |

**Infrastructure Resources** (owned by Data Storage Service):
| Resource | Namespace | Owner | Used By |
|---|---|---|---|
| **PostgreSQL + pgvector** | `kubernaut-system` | Data Storage Service | Data Storage, Context API |
| **Redis** (Context API cache) | `kubernaut-system` | Context API | Context API |

### Cross-Namespace Communication

**kubernaut-system → kubernaut-notifications**:
```yaml
# Services in kubernaut-system call Notification Service
apiVersion: v1
kind: Service
metadata:
  name: notification
  namespace: kubernaut-notifications
# Accessed from kubernaut-system as:
# notification.kubernaut-notifications.svc.cluster.local:8088
```

**kubernaut-notifications → kubernaut-system**:
```yaml
# Notification Service can read CRDs in kubernaut-system
# Uses ServiceAccount with ClusterRole for cross-namespace access
```

**Authentication**:
All cross-namespace communication uses **Kubernetes ServiceAccount tokens** validated via TokenReviewer API.

### Consequences

**Positive**:
- ✅ **Simple**: Default namespace is `kubernaut-system`
- ✅ **Secure**: Notifications isolated for sensitive escalation handling
- ✅ **Discoverable**: Easy service DNS (`service.kubernaut-system.svc.cluster.local`)
- ✅ **Future-Proof**: Can add more notification-related services to `kubernaut-notifications`
- ✅ **Operational**: Single namespace for most operations and troubleshooting

**Negative**:
- ⚠️ **Cross-Namespace DNS**: Services need full DNS for notification calls
  - **Mitigation**: Document cross-namespace service discovery
- ⚠️ **RBAC Complexity**: Notification ServiceAccount needs ClusterRole for CRD access
  - **Mitigation**: Use least-privilege RBAC with explicit permissions

**Neutral**:
- 🔄 **Migration**: Existing services in `prometheus-alerts-slm` need migration to `kubernaut-system`
- 🔄 **Documentation**: Update all implementation plans and deployment guides

### Validation Results

**Confidence Assessment Progression**:
- Initial assessment: 85% confidence
- After RBAC analysis: 90% confidence
- After notification isolation benefits: 95% confidence

**Key Validation Points**:
- ✅ Service discovery simplified (single primary namespace)
- ✅ Notification isolation enables stricter security policies
- ✅ Cross-namespace communication well-understood (ServiceAccount + TokenReviewer)

### Related Decisions
- **Supersedes**: `docs/architecture/NAMESPACE_STRATEGY.md` (dual-namespace with prometheus-alerts-slm)
- **Builds On**: Kubernetes namespace best practices
- **Supports**:
  - BR-PLATFORM-001 (Infrastructure management)
  - BR-SECURITY-001 (Security isolation)

### Review & Evolution

**When to Revisit**:
- If notification services grow beyond 3-5 services (may need sub-namespacing)
- If additional isolation requirements emerge (e.g., multi-tenancy)
- If cross-namespace communication becomes a performance bottleneck

**Success Metrics**:
- Reduced deployment complexity (target: <5 min to deploy new service)
- Clear namespace allocation (target: 100% of new services correctly placed)
- No security incidents related to notification isolation (target: 0)

---

**Approved**: October 21, 2025
**Reviewers**: Kubernaut Architecture Team
**Implementation Target**: Immediate (for new services), Phased migration (for existing services)

