# DD-PROD-001: Production Readiness Checklist Standard

**Status**: ✅ Approved
**Date**: 2025-12-07
**Decision Makers**: Engineering Team
**Impact**: High (affects all services before production deployment)

---

## Context

Different services have inconsistent production readiness assessment approaches:
- AIAnalysis has a detailed 109-point scoring system
- WorkflowExecution has a simple checklist without formal scoring
- Other services lack standardized assessment criteria

This inconsistency leads to:
1. Varying quality thresholds across services
2. Difficulty comparing service readiness
3. Risk of deploying underprepared services
4. No standardized sign-off process

---

## Decision

**All Kubernaut services MUST complete a standardized Production Readiness Checklist** before deployment to production. The checklist follows a weighted scoring system with common categories applicable to all services plus service-specific sections.

### Scoring System

| Score | Decision |
|-------|----------|
| 100+ | ✅ **Production Ready** |
| 90-99 | ⚠️ **Conditionally Ready** (document limitations) |
| <90 | ❌ **Not Ready** (address gaps before deployment) |

---

## Standard Template

### 1. Functional Validation (35 points)

#### 1.1 Critical Path Testing (15 points)
| Test | BR Reference | Score | Evidence |
|------|-------------|-------|----------|
| Happy path - Complete primary workflow | BR-XXX-001 | /5 | [Test file, result] |
| Error recovery - Transient failure with retry | BR-XXX-024 | /5 | [Test file, result] |
| Permanent failure - After max retries | Service-specific | /5 | [Test file, result] |

#### 1.2 Edge Cases and Boundary Conditions (10 points)
| Test | Score | Evidence |
|------|-------|----------|
| Empty/nil inputs handling | /2 | [Test file] |
| Large payloads (stress test) | /2 | [Test file] |
| Concurrent operations (race conditions) | /2 | [Test file] |
| Input validation (invalid data rejected) | /2 | [Test file] |
| Recovery flow (after partial failure) | /2 | [Test file] |

#### 1.3 Graceful Degradation (10 points)
| Test | Score | Evidence |
|------|-------|----------|
| External service unavailable - Retries, then fails gracefully | /3 | [Test file] |
| Database/storage unavailable - Handles error | /3 | [Test file] |
| Configuration missing - Uses defaults or fails safely | /2 | [Test file] |
| Rate limiting - Backs off appropriately | /2 | [Test file] |

---

### 2. Operational Validation (29 points)

#### 2.1 Observability - Metrics (13 points)
| Metric | Present | Recording | Labels | Score |
|--------|---------|-----------|--------|-------|
| `{service}_reconciliations_total` or `{service}_requests_total` | ✅/❌ | ✅/❌ | status | /3 |
| `{service}_duration_seconds` (histogram) | ✅/❌ | ✅/❌ | operation | /3 |
| `{service}_errors_total` (counter) | ✅/❌ | ✅/❌ | error_type | /3 |
| `{service}_active` (gauge) | ✅/❌ | ✅/❌ | - | /2 |
| Service-specific business metrics | ✅/❌ | ✅/❌ | - | /2 |

**Validation Command**:
```bash
curl -s localhost:9090/metrics | grep {service}_ | wc -l
# Expected: 5+ metrics
```

#### 2.2 Observability - Logging (6 points)
| Requirement | Score | Evidence |
|-------------|-------|----------|
| Structured logging using `logr.Logger` (Go) or structured JSON (Python) | /2 | Code review |
| Log levels appropriate (Info for normal, Error for failures) | /2 | Code review |
| Context propagation (name, namespace, correlation ID in all logs) | /2 | Log output |

#### 2.3 Health Checks (6 points)
| Endpoint | Expected | Score | Evidence |
|----------|----------|-------|----------|
| `GET /healthz` | 200 OK | /3 | `curl localhost:8081/healthz` |
| `GET /readyz` | 200 OK (or 503 if unhealthy) | /3 | `curl localhost:8081/readyz` |

#### 2.4 Graceful Shutdown (4 points)
| Requirement | Score | Evidence |
|-------------|-------|----------|
| SIGTERM handling - In-flight operations complete | /2 | Manual test |
| Shutdown timeout - Configurable grace period | /2 | Code review |

---

### 3. Security Validation (15 points)

#### 3.1 RBAC Permissions (8 points)
| Requirement | Score | Evidence |
|-------------|-------|----------|
| Minimal permissions - Only required verbs | /3 | `config/rbac/role.yaml` |
| No wildcard permissions (`*`) | /2 | Code review |
| ServiceAccount with role binding | /3 | Manifest review |

**RBAC Audit Command**:
```bash
kubectl auth can-i --list --as=system:serviceaccount:{namespace}:{service-account}
```

#### 3.2 Secret Management (7 points)
| Requirement | Score | Evidence |
|-------------|-------|----------|
| No hardcoded secrets in code | /3 | Code review |
| Secrets from K8s Secrets or environment | /2 | Deployment manifest |
| No secrets logged | /2 | Log output review |

---

### 4. Performance Validation (15 points)

#### 4.1 Latency (10 points)
| Metric | Target | Actual | Score |
|--------|--------|--------|-------|
| P50 operation latency | <5s | Xs | /3 |
| P99 operation latency | <30s | Xs | /3 |
| External API call latency (P95) | <60s | Xs | /4 |

**Benchmark Command**:
```bash
go test -bench=Benchmark -benchmem ./internal/...
# OR for Python:
pytest --benchmark-only tests/benchmark/
```

#### 4.2 Throughput (5 points)
| Metric | Target | Actual | Score |
|--------|--------|--------|-------|
| Concurrent operations | 10+ | X | /3 |
| Operations per minute | Service-specific | X | /2 |

---

### 5. Deployment Validation (15 points)

#### 5.1 Kubernetes Manifests (9 points)
| Manifest | Present | Valid | Score |
|----------|---------|-------|-------|
| Deployment with resource limits | ✅/❌ | ✅/❌ | /3 |
| ConfigMap for configuration | ✅/❌ | ✅/❌ | /2 |
| Service for metrics/health | ✅/❌ | ✅/❌ | /2 |
| RBAC (Role, RoleBinding, ServiceAccount) | ✅/❌ | ✅/❌ | /2 |

#### 5.2 Probes Configuration (6 points)
| Probe | Configured | Thresholds | Score |
|-------|------------|------------|-------|
| Liveness | ✅/❌ | periodSeconds: 10, failureThreshold: 3 | /3 |
| Readiness | ✅/❌ | periodSeconds: 5, failureThreshold: 3 | /3 |

---

### 6. ADR/DD Compliance (MANDATORY - Pass/Fail)

**All relevant architectural decisions MUST be verified before production deployment.**

| ADR/DD | Compliance | Evidence |
|--------|------------|----------|
| **ADR-027**: Red Hat UBI9 Base Images | ✅/❌ | Dockerfile review |
| **ADR-004**: Fake K8s Client for Unit Tests | ✅/❌ | Test code review |
| **ADR-030**: Tekton CRD Check at Startup (if applicable) | ✅/❌ | Code review |
| **ADR-032**: Data Access Layer Isolation | ✅/❌ | Architecture review |
| **DD-005**: Observability Standards | ✅/❌ | Metrics/logging review |
| **DD-007**: Graceful Shutdown Pattern | ✅/❌ | Code review |
| Service-specific DDs | ✅/❌ | [List specific DDs] |

**Note**: Any ❌ in ADR/DD compliance blocks production deployment.

---

### 7. Documentation Quality (Bonus: 10 points)

| Document | Present | Quality | Score |
|----------|---------|---------|-------|
| README.md comprehensive | ✅/❌ | Good/Needs Work | /3 |
| Design Decisions (DD-XXX format) | ✅/❌ | Good/Needs Work | /2 |
| Testing Strategy reflects implementation | ✅/❌ | Good/Needs Work | /2 |
| Troubleshooting Guide | ✅/❌ | Good/Needs Work | /3 |

---

## Service-Specific Sections

Each service MUST add domain-specific validation sections. Examples:

### CRD Controllers
- CRD schema validation
- Reconciliation loop testing
- Finalizer behavior
- Status subresource updates
- Watch/cache behavior

### Stateless Services (REST APIs)
- API contract validation (OpenAPI)
- Request/response validation
- Authentication/authorization
- Rate limiting behavior

### AI/ML Services
- Model inference latency
- Prompt/response validation
- Fallback behavior
- Cost monitoring

---

## Score Summary Template

| Category | Score | Target | Status |
|----------|-------|--------|--------|
| Functional Validation | /35 | 32+ | ✅/⚠️/❌ |
| Operational Validation | /29 | 27+ | ✅/⚠️/❌ |
| Security Validation | /15 | 14+ | ✅/⚠️/❌ |
| Performance Validation | /15 | 13+ | ✅/⚠️/❌ |
| Deployment Validation | /15 | 14+ | ✅/⚠️/❌ |
| ADR/DD Compliance | Pass/Fail | Pass | ✅/❌ |
| **TOTAL** | **/109** | **100+** | ✅/⚠️/❌ |
| Documentation (Bonus) | /10 | 8+ | ✅/⚠️/❌ |

---

## Production Readiness Decision

### Final Decision
**Score**: XX/109
**ADR/DD Compliance**: Pass/Fail
**Decision**: ✅ Production Ready | ⚠️ Conditionally Ready | ❌ Not Ready

### Conditions (if Conditionally Ready)
1. [Condition 1 - what must be addressed post-deployment]
2. [Condition 2 - what must be addressed post-deployment]

### Sign-off
| Role | Name | Date | Signature |
|------|------|------|-----------|
| Tech Lead | | | |
| QA | | | |
| Platform | | | |

---

## Implementation

### File Location
Each service creates their production readiness checklist at:
```
docs/services/{service-type}/{service-name}/implementation/PRODUCTION_READINESS_CHECKLIST.md
```

### Template Usage
1. Copy this template to your service directory
2. Fill in service-specific sections
3. Complete assessment before production deployment
4. Obtain required sign-offs

---

## Consequences

### Benefits
- ✅ Consistent quality thresholds across all services
- ✅ Clear production readiness criteria
- ✅ Auditable sign-off process
- ✅ ADR/DD compliance enforcement
- ✅ Reduced production incidents

### Trade-offs
- ⚠️ Additional documentation overhead
- ⚠️ Assessment takes ~2-4 hours per service
- ⚠️ May delay deployments if gaps found

---

## References

- [AIAnalysis Production Readiness Checklist](../../services/crd-controllers/02-aianalysis/implementation/PRODUCTION_READINESS_CHECKLIST.md) - Original template
- [DD-005: Observability Standards](DD-005-OBSERVABILITY-STANDARDS.md)
- [DD-007: Graceful Shutdown Pattern](DD-007-kubernetes-aware-graceful-shutdown.md)
- [ADR-027: UBI9 Base Images](ADR-027-multi-architecture-build-strategy.md)



