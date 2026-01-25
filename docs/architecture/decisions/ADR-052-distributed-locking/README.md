# ADR-052: Distributed Locking Pattern - Document Index

**Status**: ✅ APPROVED
**Last Updated**: January 18, 2026

## Quick Links

### Core ADR Documents
- **[ADR-052: Distributed Locking Pattern](ADR-052-distributed-locking-pattern.md)** - Main ADR
- **[ADR-052 Addendum 001: Exponential Backoff with Jitter](ADR-052-ADDENDUM-001-exponential-backoff-jitter.md)** - Gateway retry strategy fix (Jan 2026)

---

## Directory Structure

```
ADR-052-distributed-locking/
├── README.md (this file)
├── ADR-052-distributed-locking-pattern.md (main ADR)
├── ADR-052-ADDENDUM-001-exponential-backoff-jitter.md
├── implementation-plans/
│   ├── gateway-implementation-plan-v1.0.md
│   └── remediation-orchestrator-implementation-plan-v1.0.md
├── test-plans/
│   ├── gateway-test-plan-v1.0.md
│   └── remediation-orchestrator-test-plan-v1.0.md
├── analysis/
│   ├── gateway-race-condition-gap-analysis-dec-30-2025.md
│   ├── remediation-orchestrator-race-condition-analysis-dec-30-2025.md
│   ├── cross-team-pattern-dec-30-2025.md
│   └── dd-to-adr-conversion-dec-30-2025.md
├── handoff/
│   ├── gateway-ready-for-implementation-dec-30-2025.md
│   └── remediation-orchestrator-plans-complete-dec-30-2025.md
└── triage/
    ├── gateway-distributed-lock-triage-jan-18-2026.md
    ├── gateway-api-server-impact-analysis-jan-18-2026.md
    └── gateway-implementation-progress-jan-18-2026.md
```

---

## Document Categories

### 📋 Core ADR Documents (2)
Main decision records and amendments.

- **ADR-052-distributed-locking-pattern.md** - Original ADR (Dec 30, 2025)
- **ADR-052-ADDENDUM-001-exponential-backoff-jitter.md** - Retry strategy fix (Jan 18, 2026)

### 🔧 Implementation Plans (2)
Service-specific implementation guides.

- **implementation-plans/gateway-implementation-plan-v1.0.md** - Gateway service implementation
- **implementation-plans/remediation-orchestrator-implementation-plan-v1.0.md** - RO service implementation

### ✅ Test Plans (2)
Comprehensive test strategies for each service.

- **test-plans/gateway-test-plan-v1.0.md** - Gateway unit/integration/E2E tests
- **test-plans/remediation-orchestrator-test-plan-v1.0.md** - RO unit/integration/E2E tests

### 🔍 Analysis Documents (4)
Root cause analysis and pattern research.

- **analysis/gateway-race-condition-gap-analysis-dec-30-2025.md** - Gateway race condition deep dive
- **analysis/remediation-orchestrator-race-condition-analysis-dec-30-2025.md** - RO race condition deep dive
- **analysis/cross-team-pattern-dec-30-2025.md** - Cross-team coordination document
- **analysis/dd-to-adr-conversion-dec-30-2025.md** - Design decision to ADR conversion rationale

### 🤝 Handoff Documents (2)
Team handoff and completion summaries.

- **handoff/gateway-ready-for-implementation-dec-30-2025.md** - Gateway implementation readiness
- **handoff/remediation-orchestrator-plans-complete-dec-30-2025.md** - RO planning completion

### 🚨 Triage Documents (3)
Recent investigations and progress tracking (Jan 2026).

- **triage/gateway-distributed-lock-triage-jan-18-2026.md** - Gateway lock retry bug triage
- **triage/gateway-api-server-impact-analysis-jan-18-2026.md** - K8s API impact analysis
- **triage/gateway-implementation-progress-jan-18-2026.md** - Gateway implementation progress

---

## Timeline

### December 30, 2025 - Original ADR + Planning
- ✅ ADR-052 approved
- ✅ Gateway & RO race condition analysis completed
- ✅ Implementation plans and test plans finalized
- ✅ Cross-team coordination document published

### January 18, 2026 - Gateway Retry Strategy Fix
- 🔍 **Discovery**: E2E test triage revealed Gateway lock retry issues
- 🛠️ **Root Cause**: Unbounded recursion, fixed backoff, no retry limit, thundering herd risk
- ✅ **Fix Applied**: Exponential backoff with jitter using `pkg/shared/backoff`
- ✅ **Addendum Published**: ADR-052-ADDENDUM-001-exponential-backoff-jitter.md

---

## Business Requirements

### Gateway Service
- **BR-GATEWAY-190**: Multi-Replica Deduplication Safety

### RemediationOrchestrator Service
- **BR-ORCH-050**: Multi-Replica Resource Lock Safety

---

## Implementation Status

### Gateway Service
| Component | Status | Document |
|---|---|---|
| **Design** | ✅ Complete | ADR-052, Addendum 001 |
| **Implementation** | ✅ Complete | `pkg/gateway/processing/distributed_lock.go` |
| **Retry Strategy** | ✅ Fixed (Jan 2026) | `pkg/gateway/server.go` (exponential backoff) |
| **Unit Tests** | ⏳ In Progress | To be completed |
| **Integration Tests** | ✅ Complete | `test/integration/gateway/` |
| **E2E Tests** | ✅ Complete | `test/e2e/gateway/` |

### RemediationOrchestrator Service
| Component | Status | Document |
|---|---|---|
| **Design** | ✅ Complete | ADR-052 |
| **Implementation** | 🚧 Planned | `pkg/remediationorchestrator/locking/` (future) |
| **Unit Tests** | 🚧 Planned | To be implemented |
| **Integration Tests** | 🚧 Planned | To be implemented |
| **E2E Tests** | 🚧 Planned | To be implemented |

---

## Key Learnings

### Gateway Retry Strategy (Addendum 001)

**Problem Identified** (Jan 18, 2026):
1. ❌ Unbounded recursion (stack overflow risk)
2. ❌ No retry limit (potential infinite loop)
3. ❌ Fixed backoff (thundering herd risk)
4. ❌ No jitter (synchronized K8s API load spikes)

**Solution Applied**:
1. ✅ Iterative loop (constant stack usage)
2. ✅ Max 10 retries (~7.5s total wait)
3. ✅ Exponential backoff (100ms → 1s)
4. ✅ ±10% jitter (anti-thundering herd)

**Result**:
- Production-ready retry strategy reusing `pkg/shared/backoff`
- Prevents stack overflow, timeouts, and API load spikes
- Aligns with Notification v3.1 proven patterns

---

## Related Components

### Shared Implementation
- **Backoff Utility**: `pkg/shared/backoff/backoff.go`
- **Backoff Tests**: `test/unit/shared/backoff/backoff_test.go`

### Gateway Implementation
- **Lock Manager**: `pkg/gateway/processing/distributed_lock.go`
- **Server Integration**: `pkg/gateway/server.go` (lines 992-1083)

### RemediationOrchestrator (Future)
- **Lock Manager**: `pkg/remediationorchestrator/locking/distributed_lock.go` (planned)
- **Reconciler Integration**: `internal/controller/remediationorchestrator/reconciler.go` (planned)

---

## References

### Parent ADR
- [ADR-001: CRD Microservices Architecture](../ADR-001-crd-microservices-architecture.md)

### Related ADRs
- [ADR-015: Alert-to-Signal Naming Migration](../ADR-015-alert-to-signal-naming-migration.md)
- [ADR-030: Service Configuration Management](../ADR-030-service-configuration-management.md)

### Design Decisions
- **DD-GATEWAY-011**: Status-Based Deduplication
- **DD-GATEWAY-013**: K8s Lease-Based Distributed Locking
- **DD-SHARED-001**: Shared Backoff Utility (to be created)

### External References
- [Kubernetes Lease API](https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/lease-v1/)
- [AWS: Exponential Backoff and Jitter](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
- [Google Cloud: Retry Pattern Best Practices](https://cloud.google.com/architecture/scalable-and-resilient-apps#retry_pattern)

---

**Maintained By**: Platform Team
**Contact**: See ADR-052 main document for author information
**Last Review**: January 18, 2026
**Next Review**: After Gateway E2E test validation (Jan 19, 2026)
