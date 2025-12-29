# Data Storage Write API - Implementation Decisions

**Date**: November 2, 2025  
**Status**: ✅ **APPROVED**  
**Approach**: Option A - Discovery-First (2.5 days discovery + 10 days implementation)

---

## 🎯 **Critical Decisions Made**

### **Decision #1: pgvector Embedding Scope** ✅
**Choice**: **1a - AIAnalysis only**

**Rationale**:
- V2.0 RAR (Remediation Analysis Report) generation requires semantic search over AI investigation results
- Investigation text, root cause hypotheses, and recommendations benefit most from embeddings
- Other audit types are primarily structured data (timestamps, status, metrics)
- Reduces complexity and storage overhead

**Impact**:
- Only `ai_analysis_audit` table needs `embedding vector(1536)` column
- Embedding generation pipeline scoped to AIAnalysis audit writes only
- Estimated storage: ~6KB per AI investigation (1536 floats × 4 bytes)
- Day 4 (Embedding Generation) reduced to 4 hours (from 8 hours if all types)

**Confidence**: 90% (based on V2.0 RAR semantic search requirements)

---

### **Decision #2: Error Recovery Pattern** ✅
**Choice**: **2c - Dead Letter Queue (async retry)**

**Rationale**:
- Aligns with ADR-032 "No Audit Loss" mandate (line 61)
- Ensures service availability (reconciliation loops don't block on audit writes)
- Provides audit trail even during Data Storage Service outages
- Enables eventual consistency for audit data

**Architecture**:
```
Service → (Write Fails) → Dead Letter Queue (Redis Streams)
                                  ↓
                          Async Retry Worker
                                  ↓
                          Data Storage Service
```

**Implementation Details**:
- Use Redis Streams as DLQ (already in infrastructure)
- Max retention: 7 days (after which, alert fires for manual investigation)
- Retry strategy: Exponential backoff (1m, 5m, 15m, 1h, 4h, 24h)
- Monitoring: Alert if DLQ depth > 100 messages

**Impact**:
- **New Component**: `pkg/datastorage/dlq/` - DLQ client library
- **New Service**: Optional async retry worker (can run as sidecar or separate pod)
- **Day 5 Refactor**: Add DLQ fallback to HTTP handlers (+2 hours)
- **Day 7 Integration Tests**: Add DLQ failure scenarios (+1 hour)

**Confidence**: 85% (proven pattern, requires infrastructure setup)

---

### **Decision #3: Performance Requirements** ✅
**Choice**: **3b - p95 <1s, 50 writes/sec (balanced)**

**Performance SLA**:
| Metric | Target | Measurement |
|--------|--------|-------------|
| **Latency (p50)** | <250ms | Time from HTTP request to 201 Created response |
| **Latency (p95)** | <1s | Includes database write + (optional) embedding generation |
| **Latency (p99)** | <2s | Acceptable for occasional slow operations |
| **Throughput (normal)** | 10 writes/sec | Average across 6 endpoints during normal operations |
| **Throughput (peak)** | 50 writes/sec | During incident storms (3x normal load) |
| **Throughput (burst)** | 100 writes/sec | Short burst handling (10 seconds) |

**Database Sizing**:
- **Connection Pool**: 20 connections (sufficient for 50 writes/sec)
- **Query Timeout**: 5 seconds (5x p95 latency)
- **Circuit Breaker**: Trip at 10 consecutive failures or p95 > 3s

**Impact**:
- **Day 10 Metrics**: Add histogram for write latency (p50/p95/p99)
- **Day 10 Metrics**: Add counter for throughput (writes/sec)
- **Day 11 Production Readiness**: Document performance SLA
- **Integration Tests**: Add load testing scenario (50 concurrent writes)

**Confidence**: 95% (based on estimated load analysis + proven database patterns)

---

### **Decision #4: Authentication** ✅
**Choice**: **4c - No auth initially (trust internal network)**

**Rationale**:
- Consistent with existing internal service pattern (Context API, Gateway)
- Data Storage Service runs in secure Kubernetes cluster with network policies
- Services communicate over internal ClusterIP (not exposed externally)
- Can add authentication later without breaking changes (API versioning)

**Security Controls** (without authentication):
1. **Network Policies**: Only allow traffic from known service namespaces
2. **Service Mesh**: Consider future Istio integration for mTLS (V1.1+)
3. **Input Validation**: Strict RFC 7807 validation prevents injection attacks
4. **Audit of Audits**: Log which service IP wrote which audit record
5. **Rate Limiting**: Per-service IP rate limiting (50 req/sec per service)

**Future Authentication Path** (V1.1+):
- Add `Authorization: Bearer <token>` header requirement
- Use Kubernetes Service Account tokens
- Maintain backward compatibility with API versioning (`/api/v1` no auth, `/api/v2` requires auth)

**Impact**:
- **OpenAPI Spec**: Auth section marked as "NOT REQUIRED (V1.0)" with future note
- **HTTP Handlers**: No auth middleware (Day 4)
- **Integration Tests**: No auth setup required (Day 7)
- **Production Readiness**: Document network security requirements (Day 11)

**Confidence**: 90% (proven internal service pattern, acceptable for V1.0)

---

## 📋 **Implementation Approach**

**Selected**: **Option A - Discovery-First**

**Timeline**:
- **Phase 0: Pre-Implementation Discovery** - 2.5 days (19.5 hours)
- **Phase 1-3: TDD Implementation** - 10 days (80 hours)
- **Total**: 12.5 days

**Rationale**:
- Resolves all 8 confidence gaps upfront
- Prevents mid-implementation blockers
- Maintains 100% confidence throughout
- Proven pattern from Context API success

---

## 🔄 **Phase 0: Discovery Tasks** (2.5 days)

### **Day 0.1: Critical Path Discovery** (8 hours)

**Tasks**:
1. **GAP #2**: Create database migrations (4h)
   - File: `migrations/010_audit_write_api.sql`
   - 6 audit tables with partitioning
   - pgvector column for `ai_analysis_audit` only
   - Indexes, constraints, triggers

2. **GAP #1**: Document Notification Controller audit schema (2h)
   - File: `docs/services/crd-controllers/06-notification/database-integration.md`
   - Schema: `notification_audit` table
   - Fields: notification_id, remediation_id, channel, status, etc.

3. **GAP #5**: Define error recovery ADR (2h)
   - File: `docs/architecture/decisions/DD-009-audit-write-error-recovery.md`
   - DLQ architecture with Redis Streams
   - Retry strategy and monitoring

**Deliverables**: 3 files created, migrations testable

---

### **Day 0.2: High-Priority Discovery** (6.5 hours)

**Tasks**:
1. **GAP #3**: Clarify pgvector requirements (3h)
   - Review V2.0 RAR requirements (BR-REMEDIATION-ANALYSIS-001 to 004)
   - Validate: Only AIAnalysis needs embeddings (Decision 1a)
   - Document embedding generation pipeline

2. **GAP #6**: Define performance requirements (2h)
   - File: `docs/services/stateless/data-storage/performance-requirements.md`
   - Document SLA (Decision 3b: p95 <1s, 50 writes/sec)
   - Database sizing guidance

3. **GAP #8**: Complete Effectiveness Monitor schema (1.5h)
   - Review `migrations/006_effectiveness_assessment.sql`
   - Define complete `EffectivenessScore` struct
   - Validate OpenAPI schema compatibility

**Deliverables**: 2 files created, embedding pipeline designed, performance SLA documented

---

### **Day 0.3: Parallel Discovery** (5 hours)

**Tasks**:
1. **GAP #7**: Document authentication decision (2h)
   - Update ADR-032 v1.1 with "No Auth (V1.0)" decision
   - Document network security requirements
   - Plan V1.1 authentication migration path

2. **GAP #4**: Create service integration checklist (2h)
   - File: `docs/services/stateless/data-storage/service-integration-checklist.md`
   - Validate 6 services ready to write audit data
   - Update service docs to show REST API client pattern

3. **Final Review**: Validate 100% confidence (1h)
   - Review all 8 gap resolutions
   - Confirm no remaining blockers
   - Update confidence assessment

**Deliverables**: 2 files created, 6 services validated, 100% confidence achieved

---

## ✅ **Success Criteria for Phase 0**

**Completion Checklist**:
- [ ] `migrations/010_audit_write_api.sql` created and tested (GAP #2)
- [ ] Notification Controller audit schema documented (GAP #1)
- [ ] DD-009 error recovery ADR created (GAP #5)
- [ ] pgvector requirements clarified (AIAnalysis only) (GAP #3)
- [ ] Performance requirements documented (p95 <1s, 50 writes/sec) (GAP #6)
- [ ] Effectiveness Monitor schema completed (GAP #8)
- [ ] Authentication decision documented (no auth V1.0) (GAP #7)
- [ ] Service integration checklist created (GAP #4)
- [ ] All 8 confidence gaps resolved
- [ ] **Confidence: 100%**

**Gate to Phase 1**: All checkboxes must be ✅ before starting Day 1 implementation

---

## 📊 **Confidence Tracking**

| Phase | Start Confidence | End Confidence | Gap Resolved |
|-------|-----------------|----------------|--------------|
| **Before Phase 0** | 90% | - | - |
| **Day 0.1 Complete** | 90% | 95% | GAP #1, #2, #5 (+5%) |
| **Day 0.2 Complete** | 95% | 98% | GAP #3, #6, #8 (+3%) |
| **Day 0.3 Complete** | 98% | 100% | GAP #4, #7 (+2%) |
| **Phase 1 Start** | **100%** | **100%** | **All gaps resolved** |

---

## 🚀 **Ready to Proceed**

**Phase 0: Day 0.1 begins now** with:
1. Create `migrations/010_audit_write_api.sql` (4h)
2. Document Notification Controller audit schema (2h)
3. Define DD-009 error recovery ADR (2h)

**Expected completion**: 8 hours (1 day)  
**Next milestone**: Day 0.2 (6.5 hours)  
**Final milestone**: Day 0.3 (5 hours)  
**Phase 0 complete**: 19.5 hours (2.5 days) → **100% confidence achieved**

