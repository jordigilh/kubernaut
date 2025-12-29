# ⚠️ **DEPRECATED** - Dynamic Toolset Service

**Status**: ❌ **CODE DELETED - DOCUMENTATION ONLY**
**Date**: December 20, 2025
**Reason**: Deferred to V2.0 per DD-016 - Rebuild planned with current development standards
**Authority**: [DD-016 - Dynamic Toolset V2.0 Deferral](../../../architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md)

---

## 📋 **Deletion Summary**

### **What Was Deleted**

| Component | Location | Size | Reason |
|-----------|----------|------|--------|
| **Implementation Code** | `pkg/toolset/` | ~1,722 LOC (23 files) | Outdated development methodology |
| **Service Entry Point** | `cmd/dynamictoolset/main.go` | ~192 LOC | No V1.0 usage |
| **Test Files** | `test/*/toolset/` | 2 Go test files | Minimal coverage |

### **What Was Preserved**

| Component | Location | Reason |
|-----------|----------|--------|
| **Documentation** | `docs/services/stateless/dynamic-toolset/` | Historical reference, V2.0 planning |
| **Deployment Manifests** | `deploy/dynamic-toolset/` | Reference architecture for V2.0 |
| **Design Decisions** | `docs/architecture/decisions/DD-016-*.md` | Authoritative deferral rationale |
| **Business Requirements** | `docs/services/stateless/dynamic-toolset/BUSINESS_REQUIREMENTS.md` | V2.0 scope planning |

---

## 🎯 **Why Deletion vs. Preservation?**

### **Decision Rationale**

**Rebuild Cost < Refactor Cost**:
- Original code: ~1,722 LOC
- Estimated rebuild with current standards: 2-3 days
- Estimated refactor to current standards: 1-2 weeks
- **Verdict**: More efficient to rebuild in V2.0 using current APDC methodology, SOC2 patterns, and testing standards

**Development Methodology Evolution**:
Since Dynamic Toolset was developed, Kubernaut has adopted:
- ✅ APDC Framework (Analysis-Plan-Do-Check)
- ✅ SOC2-compliant audit traces (ADR-034)
- ✅ OpenAPI client mandatory (DD-API-001)
- ✅ P0 maturity requirements (testutil.ValidateAuditEvent)
- ✅ Defense-in-depth testing pyramid
- ✅ RFC 7807 error standard (DD-004)

**Refactoring the old code to these standards would require**:
- Complete audit integration overhaul
- OpenAPI client migration
- Test suite rewrite (only 2 test files existed)
- Error handling standardization
- Logging framework update (DD-005)

**Conclusion**: Clean rebuild in V2.0 is faster and produces higher quality code.

---

## 📚 **Historical Context**

### **Original Purpose**

The Dynamic Toolset Service was designed to provide automatic service discovery for HolmesGPT-API, enabling AI investigations to leverage all available observability tools (Prometheus, Grafana, Jaeger, Elasticsearch, etc.) based on what's deployed in the Kubernetes cluster.

### **Why Deferred to V2.0**

**From DD-016**:

1. **Current Redundancy**: HolmesGPT-API already contains built-in Prometheus discovery logic. Since V1.0 only requires Prometheus integration, a separate Dynamic Toolset Service is redundant.

2. **Future Relevance**: When V2.0 expands to identify other observability services (Grafana, Jaeger, Elasticsearch), Dynamic Toolset becomes valuable as a centralized discovery component.

3. **V1.0 Scope**: V1.0 focuses on Prometheus-only observability, which HolmesGPT-API handles natively.

4. **Architectural Evolution**: Deferring allows V2.0 design to benefit from V1.0 operational learnings.

---

## 🚀 **V2.0 Rebuild Plan**

### **When to Rebuild**

**Triggers** (DD-016):
1. V2.0 roadmap includes multi-service observability expansion (Grafana, Jaeger, Elasticsearch)
2. HolmesGPT-API service discovery complexity exceeds built-in Prometheus logic
3. V1.0 production validation complete and operational experience informs requirements

### **V2.0 Implementation Approach**

**Phase 1: Requirements Analysis** (APDC Analysis):
1. Review preserved documentation for business requirements
2. Analyze V1.0 production feedback for multi-service discovery needs
3. Define V2.0 observability scope (which services to discover)
4. Assess integration patterns with HolmesGPT-API v2.x

**Phase 2: Design Planning** (APDC Plan):
1. Design SOC2-compliant audit traces for discovery events
2. Plan OpenAPI client integration for service interactions
3. Define P0 maturity requirements and test strategy
4. Create DD entries for architectural decisions

**Phase 3: TDD Implementation** (APDC Do):
1. RED: Write unit tests for detector interfaces (70%+ coverage)
2. GREEN: Implement minimal detector logic (Prometheus, Grafana, Jaeger)
3. REFACTOR: Add sophisticated discovery patterns and caching
4. Integration tests with real Kubernetes clusters
5. E2E tests with HolmesGPT-API integration

**Phase 4: Validation** (APDC Check):
1. Verify SOC2 compliance (100% audit trail)
2. Confirm P0 maturity requirements met
3. Validate against V2.0 business requirements
4. Performance testing and optimization

**Estimated Timeline**: 2-3 weeks (vs. 4-6 weeks for refactoring old code)

---

## 🔗 **Preserved Documentation**

### **For V2.0 Planning**

| Document | Purpose | Location |
|----------|---------|----------|
| **Business Requirements** | V2.0 scope definition | `BUSINESS_REQUIREMENTS.md` |
| **API Specification** | REST endpoint design | `api-specification.md` |
| **Service Discovery Patterns** | Detector architecture | `service-discovery.md` |
| **Implementation Plan** | Historical reference | `implementation/IMPLEMENTATION_PLAN_ENHANCED.md` |
| **Testing Strategy** | Test approach patterns | `testing-strategy.md` |

### **For Historical Reference**

| Document | Purpose | Location |
|----------|---------|----------|
| **DD-016** | Deferral rationale | `docs/architecture/decisions/DD-016-dynamic-toolset-v2-deferral.md` |
| **Deployment Manifests** | K8s resource patterns | `deploy/dynamic-toolset/` |
| **Operations Runbook** | Operational insights | `OPERATIONS_RUNBOOK.md` |

---

## ✅ **V1.0 Status**

| Aspect | Status | Notes |
|--------|--------|-------|
| **Code** | ❌ Deleted | Rebuild planned for V2.0 |
| **Documentation** | ✅ Preserved | Historical reference + V2.0 planning |
| **Deployment Manifests** | ✅ Preserved | Reference architecture |
| **Business Requirements** | ✅ Preserved | V2.0 scope definition |
| **Tests** | ❌ Deleted | New tests required for V2.0 |

---

## 📞 **Questions?**

**Why not keep the code?**
- Development methodology has evolved significantly
- Refactoring old code would take longer than rebuilding
- Clean V1.0 release without unused services
- V2.0 rebuild will use current best practices from day one

**What if we need it before V2.0?**
- Git history preserves all deleted code
- Recovery: `git checkout [commit-before-deletion] -- pkg/toolset cmd/dynamictoolset`
- Estimated recovery time: < 1 hour
- Last commit with code: [commit hash from deletion commit]

**How do I reference the old code?**
- See git history for implementation patterns
- Detector interfaces: Good reference for V2.0 design
- ConfigMap builder: Reusable patterns

---

**Created**: December 20, 2025
**Authority**: DD-016, User Decision (Dec 20, 2025)
**Confidence**: 90% - Clean rebuild more efficient than refactoring outdated code
**Next Review**: Before V2.0 planning (Q3 2026)












