# DD-HOLMESGPT-012: Minimal Internal Service Architecture

## Status
**✅ APPROVED** (2025-10-17)
**Last Reviewed**: 2025-10-17
**Confidence**: 95%

---

## Context & Problem

**Problem**: HolmesGPT API implementation drifted from original "thin wrapper" design to full API Gateway with enterprise features.

**Key Facts**:
- Service is **internal-only** (not exposed outside namespace)
- Network policies restrict access to specific Kubernaut services only
- All core business logic (74 tests) is 100% complete
- 104 infrastructure tests (58.4% of total) provide minimal value for internal service

**Discovery**:
```
Core Business Logic:    74/74 tests passing (100%) ✅
Infrastructure:         89/104 tests passing (85.6%)
Over-engineering:       58.4% of implementation effort
```

---

## Alternatives Considered

### Alternative 1: Full API Gateway (Current Implementation)

**Approach**: Implement all 185 BRs with enterprise-grade features

**Features**:
- Enterprise rate limiting (23 tests)
- Multi-method authentication (26 tests)
- Advanced request validation (23 tests)
- Extensive health endpoints (32 tests)
- CORS, TLS enforcement, security headers
- Role-based access control (RBAC)

**Pros**:
- ✅ Production-ready for external exposure
- ✅ Handles high-traffic scenarios
- ✅ Defense-in-depth security

**Cons**:
- ❌ 10+ days implementation time
- ❌ Over-engineered for internal service
- ❌ Technical debt from unused features
- ❌ Maintenance burden
- ❌ Network policies already provide access control

**Confidence**: 20% (rejected - wrong tool for the job)

---

### Alternative 2: Minimal Viable Internal Service (RECOMMENDED)

**Approach**: Core business logic + essential internal service features

**Features**:
- ✅ Core investigation endpoints (15 BRs)
- ✅ Recovery analysis (6 BRs)
- ✅ Post-execution analysis (5 BRs)
- ✅ HolmesGPT SDK integration (5 BRs)
- ✅ Basic health/readiness (2 BRs)
- ✅ K8s ServiceAccount authentication (2 BRs)
- ✅ Basic HTTP server (10 BRs)

**Deferred to v2.0** (if needed):
- ⏸️ Rate limiting (network policies handle this)
- ⏸️ Multi-method authentication (K8s RBAC sufficient)
- ⏸️ Advanced validation (Pydantic sufficient)
- ⏸️ CORS (internal service)
- ⏸️ Extensive health tests (basic probes sufficient)

**Pros**:
- ✅ **Matches architectural intent** ("thin wrapper")
- ✅ **3-4 days implementation** vs. 10+ days
- ✅ **Zero technical debt** from unused features
- ✅ Network policies provide access control
- ✅ K8s RBAC provides authentication
- ✅ Service mesh handles TLS
- ✅ 100% core business logic complete

**Cons**:
- ⚠️ Not suitable for external exposure (but not needed)
- ⚠️ Requires v2.0 if usage changes (acceptable)

**Confidence**: 95% (approved - correct tool for the job)

---

### Alternative 3: Hybrid Approach

**Approach**: Core + some infrastructure features

**Features**:
- Core business logic (31 BRs)
- Basic auth + health (10 BRs)
- Some validation (10 BRs)
- Some rate limiting (5 BRs)

**Pros**:
- ✅ Middle ground
- ✅ Some future-proofing

**Cons**:
- ❌ Still over-engineered
- ❌ 5-6 days implementation
- ❌ Unnecessary complexity

**Confidence**: 50% (rejected - still too much)

---

## Decision

**APPROVED: Alternative 2 - Minimal Viable Internal Service**

**Rationale**:

1. **Architectural Alignment**: Service is documented as "REST wrapper" around HolmesGPT SDK
2. **Network Isolation**: Network policies restrict access to authorized services only
3. **K8s Native Security**: ServiceAccount tokens + RBAC provide authentication/authorization
4. **Service Mesh**: TLS and mTLS handled by Istio/Linkerd
5. **Core Value Complete**: 100% of business logic (74 tests) already passing
6. **Time Efficiency**: 60% time savings (3-4 days vs. 10+ days)
7. **Zero Technical Debt**: No unused features to maintain

**Key Insight**: **Network policies are the correct layer for access control in Kubernetes, not application-level rate limiting.**

---

## Implementation

### Core Features (45 BRs, ~90 tests)

**Investigation Endpoints**:
- `POST /api/v1/investigate` - AI investigation
- `POST /api/v1/recovery/analyze` - Recovery strategies
- `POST /api/v1/postexec/analyze` - Effectiveness analysis

**Supporting**:
- `GET /health` - Liveness probe
- `GET /ready` - Readiness probe
- `GET /metrics` - Prometheus metrics

**Authentication**:
- Kubernetes ServiceAccount token validation
- Basic role checking (readonly vs. operator)

**Implementation Files**:
```
holmesgpt-api/
├── src/
│   ├── main.py                    # FastAPI app
│   ├── extensions/
│   │   ├── recovery.py           # Recovery endpoint
│   │   ├── postexec.py           # Post-exec endpoint
│   │   └── health.py             # Health endpoints
│   ├── models/
│   │   ├── recovery_models.py    # Recovery types
│   │   └── postexec_models.py    # Post-exec types
│   └── middleware/
│       └── auth.py               # K8s ServiceAccount auth only
└── tests/
    └── unit/
        ├── test_recovery.py      # 27 tests
        ├── test_postexec.py      # 24 tests
        ├── test_models.py        # 23 tests
        └── test_health.py        # 10-15 tests (reduced)
```

### Removed Features (140 BRs, ~88 tests)

**Removed Code**:
```
holmesgpt-api/
├── src/middleware/
│   ├── ratelimit.py              # ❌ REMOVED - Network policies handle this
│   └── validation.py             # ❌ REMOVED - Pydantic sufficient
└── tests/unit/
    ├── test_ratelimit_middleware.py  # ❌ REMOVED - 23 tests
    ├── test_security_middleware.py   # ❌ REMOVED - 26 tests
    └── test_validation.py            # ❌ REMOVED - 23 tests
```

**Deferred BRs** (to v2.0 if needed):
- BR-HAPI-066 to 125: Advanced security (60 BRs)
- BR-HAPI-126 to 145: Performance/rate limiting (20 BRs)
- BR-HAPI-146 to 165: Advanced configuration (20 BRs)
- BR-HAPI-180 to 185: Advanced validation (6 BRs)

---

## Consequences

### Positive
- ✅ **60% time savings**: 3-4 days vs. 10+ days implementation
- ✅ **Zero technical debt**: No unused features to maintain
- ✅ **Architectural alignment**: Matches "thin wrapper" design intent
- ✅ **K8s native security**: Uses platform features correctly
- ✅ **Simpler codebase**: Easier to understand and maintain
- ✅ **Faster deployment**: Production-ready immediately

### Negative
- ⚠️ **Not externally exposable**: Cannot be used outside cluster without significant rework
  - **Mitigation**: Document as internal-only service, add v2.0 plan if external exposure needed
- ⚠️ **Limited rate limiting**: No application-level throttling
  - **Mitigation**: Network policies + K8s resource quotas + service mesh provide sufficient control
- ⚠️ **Basic health checks**: No extensive dependency health monitoring
  - **Mitigation**: K8s liveness/readiness probes + basic SDK health check sufficient

### Neutral
- 🔄 **V2.0 may require features**: If usage pattern changes (external exposure)
- 🔄 **Prometheus metrics**: Basic metrics only, no detailed rate limit metrics
- 🔄 **Authentication**: Single method (K8s ServiceAccount) instead of multiple

---

## Validation Results

### Architectural Consistency Check

**Question**: Is this service internal or external?
**Answer**: Internal-only (confirmed by user)

**Question**: Who controls access?
**Answer**: Network policies + K8s RBAC (platform level)

**Question**: What's the core purpose?
**Answer**: Expose HolmesGPT SDK via REST (thin wrapper)

**Alignment**: ✅ 100% - Minimal service matches all answers

---

### Code Metrics

**Before** (Full API Gateway):
- Total Tests: 178
- Core Business: 74 (41.6%)
- Infrastructure: 104 (58.4%)
- Implementation Time: 10+ days

**After** (Minimal Internal Service):
- Total Tests: 90 (~50% reduction)
- Core Business: 74 (82.2%)
- Infrastructure: 16 (17.8%)
- Implementation Time: 3-4 days (60% savings)

**Value Retained**: 100% (all core business logic)

---

### Confidence Assessment Progression

- **Initial assessment**: 30% confidence (architectural drift detected)
- **After analysis**: 95% confidence (correct approach confirmed)
- **After user approval**: 95% confidence (network policies + K8s RBAC confirmed)

**Key Validation Points**:
- ✅ Service is internal-only
- ✅ Network policies handle access control
- ✅ K8s RBAC handles authentication
- ✅ Service mesh handles TLS
- ✅ All core business logic complete

---

## Related Decisions

- **Builds On**: DD-HOLMESGPT-011 (Authentication Strategy) - Simplified to K8s ServiceAccount only
- **Supersedes**: Original 185 BR plan - Reduced to 45 essential BRs
- **Supports**:
  - BR-HAPI-001 to 015 (Investigation)
  - BR-HAPI-RECOVERY-001 to 006 (Recovery)
  - BR-HAPI-POSTEXEC-001 to 005 (Post-execution)

---

## Review & Evolution

### When to Revisit

**Trigger v2.0 Planning If**:
- Service needs external exposure (API Gateway)
- Multiple tenants require isolation (rate limiting)
- Compliance requires advanced security features
- Performance requires distributed rate limiting

**Success Metrics** (v1.0):
- Core tests: 74/74 passing (100%) ✅
- Production deployment: < 1 week ✅
- Zero security incidents: K8s RBAC + network policies
- API latency: < 5s (p95)

**Evolution Path**:
```
v1.0: Minimal Internal Service (current)
  ↓
v1.5: Add Redis caching (if needed)
  ↓
v2.0: Add rate limiting + advanced security (if external exposure)
```

---

## Network Policy Example

**Access Control at Platform Level**:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: holmesgpt-api-ingress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: holmesgpt-api
  policyTypes:
  - Ingress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: aianalysis-controller     # Only AIAnalysis Controller
    - podSelector:
        matchLabels:
          app: effectiveness-monitor     # Only Effectiveness Monitor
    ports:
    - protocol: TCP
      port: 8080
```

**This provides**:
- Zero-trust network segmentation
- Service-to-service authorization
- No application-level rate limiting needed

---

**Document Status**: ✅ Complete
**Approved By**: User (2025-10-17)
**Implementation Status**: In Progress
**Next Steps**: Remove unnecessary code and tests

