# RFC 7807 + Graceful Shutdown Implementation Summary

**Date**: 2025-11-09
**Branch**: `feat/rfc7807-graceful-shutdown-services`
**Status**: ⏸️ **READY FOR REVIEW AND APPROVAL**

---

## 🎯 Overview

This document summarizes the implementation plans for RFC 7807 error responses and graceful shutdown across **Dynamic Toolset (Go)** and **HolmesGPT API (Python/FastAPI)** services.

**Goal**: Achieve production readiness parity with Gateway and Context API services.

---

## 📊 Services Covered

| Service | Language | RFC 7807 | Graceful Shutdown | Total Tests | Timeline |
|---------|----------|----------|-------------------|-------------|----------|
| **Dynamic Toolset** | Go | ✅ 6 tests | ✅ 8 tests | 14 tests | 2 days (16 hours) |
| **HolmesGPT API** | Python/FastAPI | ✅ 7 tests | ✅ 2 tests | 9 tests | 1 day (8 hours) |
| **TOTAL** | - | **13 tests** | **10 tests** | **23 tests** | **3 days (24 hours)** |

---

## 🔗 Reference Services (Role Models)

### Gateway Service (Go)
**RFC 7807 Implementation**:
- **File**: `pkg/gateway/errors/rfc7807.go`
- **Pattern**: Struct with all required fields + helper functions
- **Tests**: `test/integration/gateway/priority1_error_propagation_test.go`
- **Test Count**: 3 integration tests
- **Key Features**:
  - Request ID tracking
  - Error type URI constants
  - Content-Type: `application/problem+json`
  - All required RFC 7807 fields

**Graceful Shutdown**: Not yet implemented (Dynamic Toolset will be first Go service)

---

### Context API (Go)
**RFC 7807 Implementation**:
- **File**: `pkg/contextapi/errors/rfc7807.go`
- **Pattern**: Same as Gateway + `IsRFC7807Error()` helper
- **Key Features**:
  - Error type preservation (no `fmt.Errorf` wrapping)
  - Gateway timeout and bad gateway errors
  - Integration with Data Storage Service

**Graceful Shutdown (DD-007)**:
- **File**: `pkg/contextapi/server/server.go` (lines 316-435)
- **Tests**: `test/integration/contextapi/13_graceful_shutdown_test.go`
- **Test Count**: **8 integration tests** (P0-P2 priority)
- **Pattern**: 4-step Kubernetes-aware shutdown
  1. Set `isShuttingDown` flag (atomic.Bool)
  2. Wait 5 seconds for endpoint removal propagation
  3. Drain in-flight HTTP connections (30s timeout)
  4. Close resources (cache connections)
- **Key Features**:
  - Readiness probe returns 503 during shutdown (RFC 7807 format)
  - Liveness probe remains healthy during shutdown
  - SIGTERM/SIGINT signal handling
  - Concurrent shutdown safety
  - Comprehensive logging

**Production Results**:
- **Error Rate**: 0% during rolling updates (vs 5-10% baseline)
- **Shutdown Duration**: 5-7 seconds (consistent)
- **Resource Leaks**: 0 connection leaks detected

---

## 📋 Implementation Plans

### Dynamic Toolset Service (Go)

**Plan**: `docs/services/stateless/dynamic-toolset/implementation/IMPLEMENTATION_PLAN_V2.2_RFC7807_GRACEFUL_SHUTDOWN.md`

**Version**: v2.2 (supersedes v2.1)

**New Business Requirements**:
- **BR-TOOLSET-039**: RFC 7807 Error Response Standard (P1)
- **BR-TOOLSET-040**: Graceful Shutdown with Signal Handling (P0)

**Day 14: RFC 7807 (8 hours)**:
- Phase 1: DO-RED (2 hours) - 6 failing integration tests
- Phase 2: DO-GREEN (3 hours) - RFC 7807 implementation
- Phase 3: DO-REFACTOR (2 hours) - Request ID middleware, metrics
- Phase 4: CHECK (1 hour) - Validation & documentation

**Day 15: Graceful Shutdown (8 hours)**:
- Phase 1: DO-RED (2 hours) - 8 failing integration tests (matching Context API)
- Phase 2: DO-GREEN (4 hours) - DD-007 4-step pattern implementation
- Phase 3: DO-REFACTOR (1.5 hours) - Signal handling, metrics
- Phase 4: CHECK (30 min) - Final validation

**Test Coverage**:
- **RFC 7807**: 6 integration tests
  1. Unsupported Media Type (415)
  2. Method Not Allowed (405)
  3. Service Unavailable during shutdown (503)
  4. All required fields validation
  5. Error type URI format validation
  6. Request ID inclusion

- **Graceful Shutdown**: 8 integration tests (EXACT MATCH to Context API)
  1. Readiness probe coordination (P0)
  2. Liveness probe during shutdown (P0)
  3. In-flight request completion (P0)
  4. Resource cleanup (P1)
  5. Shutdown timing (5s wait) (P1)
  6. Shutdown timeout respect (P1)
  7. Concurrent shutdown safety (P2)
  8. Shutdown logging (P2)

**Implementation Files**:
- `pkg/toolset/errors/rfc7807.go` (new)
- `pkg/toolset/middleware/request_id.go` (new)
- `pkg/toolset/server/server.go` (modify - add Shutdown method)
- `cmd/dynamictoolset/main.go` (modify - add signal handlers)
- `test/integration/toolset/rfc7807_compliance_test.go` (new)
- `test/integration/toolset/graceful_shutdown_test.go` (new)

**Confidence**: 95%

---

### HolmesGPT API Service (Python/FastAPI)

**Plan**: `docs/services/stateless/holmesgpt-api/IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md`

**Version**: v3.1 (extends v3.0 production-ready base)

**New Business Requirements**:
- **BR-HAPI-110**: RFC 7807 Error Response Standard (P1)
- **BR-HAPI-111**: Graceful Shutdown with Signal Handling (P0)

**Phase 1: RFC 7807 (4 hours)**:
- Task 1.1: DO-RED (1.5 hours) - 7 failing unit tests
- Task 1.2: DO-GREEN (2 hours) - RFC 7807 implementation
- Task 1.3: DO-REFACTOR (30 min) - Request ID middleware, metrics

**Phase 2: Graceful Shutdown (2 hours)**:
- Task 2.1: DO-RED (30 min) - 2 failing integration tests
- Task 2.2: DO-GREEN (1 hour) - Signal handling implementation
- Task 2.3: DO-REFACTOR (30 min) - Configurable timeout, metrics

**Phase 3: Testing & Documentation (2 hours)**:
- Task 3.1: Run all tests (30 min)
- Task 3.2: Update documentation (1 hour)
- Task 3.3: Confidence assessment (30 min)

**Test Coverage**:
- **RFC 7807**: 7 unit tests
  1. Validation error format (422)
  2. Content-Type header validation
  3. Required fields validation
  4. Internal error format (500)
  5. Service unavailable format (503)
  6. Error type URI format
  7. Request ID inclusion

- **Graceful Shutdown**: 2 integration tests
  1. SIGTERM graceful shutdown
  2. SIGINT graceful shutdown

**Implementation Files**:
- `holmesgpt-api/src/errors.py` (enhance existing)
- `holmesgpt-api/src/main.py` (add exception handlers + signal handlers)
- `holmesgpt-api/src/extensions/health.py` (update for shutdown)
- `holmesgpt-api/tests/unit/test_rfc7807_errors.py` (new)
- `holmesgpt-api/tests/integration/test_graceful_shutdown.py` (new)

**Confidence**: 95%

---

## 🔑 Key Design Decisions

### DD-004: RFC 7807 Error Response Standard
**Authority**: Mandates RFC 7807 for all HTTP services
**Scope**: Gateway, Context API, Data Storage, Dynamic Toolset, HolmesGPT API
**Status**: ✅ Approved (Production Standard)

**Required Fields**:
- `type`: URI reference (e.g., `https://kubernaut.io/errors/validation-error`)
- `title`: Short summary (e.g., "Bad Request")
- `detail`: Human-readable explanation
- `status`: HTTP status code
- `instance`: Request path
- `request_id`: Request tracing (optional extension)

**Content-Type**: `application/problem+json`

---

### DD-007: Kubernetes-Aware Graceful Shutdown Pattern
**Authority**: Mandates 4-step shutdown for all HTTP services
**Scope**: Context API (implemented), Dynamic Toolset (pending), HolmesGPT API (pending)
**Status**: ✅ Approved (Production Standard)

**4-Step Pattern**:
1. **Set Shutdown Flag**: `isShuttingDown = true` (atomic)
2. **Wait 5 Seconds**: Kubernetes endpoint removal propagation
3. **Drain Connections**: HTTP server shutdown (30s timeout)
4. **Close Resources**: Cache, database, Kubernetes client cleanup

**Key Principles**:
- **Explicit State Management**: Use shutdown flag, not implicit listener state
- **Graceful Degradation**: Best-effort resource cleanup (non-fatal)
- **Standards Compliance**: RFC 7807 for shutdown errors
- **Observable Shutdown**: Detailed logging at each step

**Production Validation** (Context API):
- **Before DD-007**: 5-10% error rate during rolling updates
- **After DD-007**: 0% error rate during rolling updates
- **Shutdown Duration**: 5-7 seconds (consistent)
- **Resource Leaks**: 0 connection leaks

---

## 📊 Test Coverage Comparison

| Service | RFC 7807 Tests | Graceful Shutdown Tests | Total | Confidence |
|---------|----------------|-------------------------|-------|------------|
| **Gateway** | 3 integration | - | 3 | 95% |
| **Context API** | E2E coverage | 8 integration | 8+ | 90% |
| **Data Storage** | - | - | - | - |
| **Dynamic Toolset** | 6 integration | 8 integration | 14 | 95% |
| **HolmesGPT API** | 7 unit | 2 integration | 9 | 95% |

**Target**: Dynamic Toolset and HolmesGPT API will match or exceed Gateway/Context API test coverage.

---

## 🚨 Critical Pitfalls to Avoid

### From Context API Experience

**❌ Pitfall #1: Wrapping RFC 7807 Errors with `fmt.Errorf`**
```go
// WRONG: Breaks type assertion
return nil, 0, fmt.Errorf("Data Storage unavailable: %w", rfc7807Err)

// CORRECT: Preserve RFC7807Error type
return nil, 0, rfc7807Err
```

**❌ Pitfall #2: Testing Only Behavior, Not Correctness**
```go
// WRONG: Weak assertion
Expect(total).To(BeNumerically(">", 0))

// CORRECT: Validate exact value
Expect(total).To(Equal(10000))
```

**❌ Pitfall #3: Hardcoded Shutdown Delays**
```go
// WRONG: Magic number
time.Sleep(5 * time.Second)

// CORRECT: Documented constant
const EndpointRemovalPropagationDelay = 5 * time.Second // DD-007: Kubernetes endpoint removal
time.Sleep(EndpointRemovalPropagationDelay)
```

---

## ✅ Success Criteria

### Functional Requirements
- [ ] All HTTP error responses use RFC 7807 format
- [ ] Content-Type header set to `application/problem+json`
- [ ] All required RFC 7807 fields present
- [ ] SIGTERM and SIGINT handled gracefully
- [ ] 4-step DD-007 shutdown pattern implemented
- [ ] Readiness probe returns 503 during shutdown
- [ ] Liveness probe remains healthy during shutdown
- [ ] 5-second endpoint removal propagation delay
- [ ] In-flight requests complete before shutdown

### Testing Requirements
- [ ] Dynamic Toolset: 14 new tests pass (6 RFC 7807 + 8 graceful shutdown)
- [ ] HolmesGPT API: 9 new tests pass (7 RFC 7807 + 2 graceful shutdown)
- [ ] All existing tests pass (no regressions)
- [ ] Total: 23 new tests passing

### Documentation Requirements
- [ ] BR-TOOLSET-039 and BR-TOOLSET-040 documented
- [ ] BR-HAPI-110 and BR-HAPI-111 documented
- [ ] BR_MAPPING.md updated for both services
- [ ] Implementation notes in code comments
- [ ] DD-004 and DD-007 referenced in code

### Quality Requirements
- [ ] No lint errors
- [ ] Error messages are descriptive and actionable
- [ ] Shutdown logs are comprehensive
- [ ] Confidence assessment ≥ 90% for both services

---

## 📈 Timeline & Milestones

| Day | Service | Phase | Milestone | Duration |
|-----|---------|-------|-----------|----------|
| **1** | Dynamic Toolset | RFC 7807 | 6 tests + implementation | 8 hours |
| **2** | Dynamic Toolset | Graceful Shutdown | 8 tests + implementation | 8 hours |
| **3** | HolmesGPT API | RFC 7807 + Shutdown | 9 tests + implementation | 8 hours |

**Total**: 3 days (24 hours)

---

## 🔗 Related Documents

### Authority Documents
- **DD-004**: RFC 7807 Error Response Standard
- **DD-007**: Kubernetes-Aware Graceful Shutdown Pattern
- **RFC 7807**: Problem Details for HTTP APIs (IETF standard)

### Reference Implementations
- **Gateway**: `pkg/gateway/errors/rfc7807.go` (RFC 7807 reference)
- **Context API**: `pkg/contextapi/errors/rfc7807.go` (RFC 7807 reference)
- **Context API**: `pkg/contextapi/server/server.go` (DD-007 reference)
- **Context API**: `test/integration/contextapi/13_graceful_shutdown_test.go` (8 tests reference)

### Implementation Plans
- **Dynamic Toolset**: `IMPLEMENTATION_PLAN_V2.2_RFC7807_GRACEFUL_SHUTDOWN.md`
- **HolmesGPT API**: `IMPLEMENTATION_PLAN_V3.1_RFC7807_GRACEFUL_SHUTDOWN.md`

---

## 🎯 Next Steps

1. **User Review**: Review and approve both implementation plans
2. **Implementation**: Execute plans following TDD methodology
3. **Testing**: Run all tests (unit + integration)
4. **Documentation**: Update README.md with production features
5. **PR Creation**: Create PR for review and merge
6. **Deployment**: Test in Kubernetes with rolling updates
7. **Monitoring**: Verify error metrics and shutdown logs

---

## 📝 Notes

- **Test Parity**: Dynamic Toolset graceful shutdown tests match Context API exactly (8 tests)
- **Reference Services**: Gateway and Context API are the role models for implementation and test coverage
- **Production Safety**: DD-007 pattern proven to achieve 0% error rate during rolling updates
- **Confidence**: Both services target 95% confidence based on proven reference implementations

---

**Status**: ⏸️ **READY FOR REVIEW AND APPROVAL**
**Branch**: `feat/rfc7807-graceful-shutdown-services`
**Approval Required From**: Technical Lead / Product Owner
**Implementation Start**: Upon approval
**Estimated Completion**: 3 business days after approval

---

**Document Author**: AI Assistant
**Document Reviewer**: [Pending]
**Document Approver**: [Pending]
**Last Updated**: 2025-11-09

