# Context API to AI/ML Service BR Renaming Map

**Date**: November 8, 2025
**Purpose**: Document BR renaming during migration from Context API to AI/ML Service
**Rationale**: ADR-034 mandates `BR-AI-` prefix for AI/ML Service BRs

---

## 📋 **BR Renaming Map**

| Old BR ID (Context API) | New BR ID (AI/ML Service) | Description |
|---|---|---|
| **BR-CONTEXT-016** | **BR-AI-016** | Investigation Complexity Assessment |
| **BR-CONTEXT-017** | **BR-AI-017** | (Reserved) |
| **BR-CONTEXT-018** | **BR-AI-018** | (Reserved) |
| **BR-CONTEXT-019** | **BR-AI-019** | (Reserved) |
| **BR-CONTEXT-020** | **BR-AI-020** | (Reserved) |
| **BR-CONTEXT-021** | **BR-AI-021** | Context Adequacy Validation |
| **BR-CONTEXT-022** | **BR-AI-022** | Context Sufficiency Scoring |
| **BR-CONTEXT-023** | **BR-AI-023** | Additional Context Triggering |
| **BR-CONTEXT-024** | **BR-AI-024** | (Reserved) |
| **BR-CONTEXT-025** | **BR-AI-025** | AI Model Self-Assessment |
| **BR-CONTEXT-026** | **BR-AI-026** | (Reserved) |
| **BR-CONTEXT-027** | **BR-AI-027** | (Reserved) |
| **BR-CONTEXT-028** | **BR-AI-028** | (Reserved) |
| **BR-CONTEXT-029** | **BR-AI-029** | (Reserved) |
| **BR-CONTEXT-030** | **BR-AI-030** | (Reserved) |
| **BR-CONTEXT-031** | **BR-AI-031** | (Reserved) |
| **BR-CONTEXT-032** | **BR-AI-032** | (Reserved) |
| **BR-CONTEXT-033** | **BR-AI-033** | (Reserved) |
| **BR-CONTEXT-034** | **BR-AI-034** | (Reserved) |
| **BR-CONTEXT-035** | **BR-AI-035** | (Reserved) |
| **BR-CONTEXT-036** | **BR-AI-036** | (Reserved) |
| **BR-CONTEXT-037** | **BR-AI-037** | (Reserved) |
| **BR-CONTEXT-038** | **BR-AI-038** | (Reserved) |
| **BR-CONTEXT-039** | **BR-AI-039** | Performance Correlation Monitoring |
| **BR-CONTEXT-040** | **BR-AI-040** | Performance Degradation Detection |
| **BR-CONTEXT-041** | **BR-AI-041** | (Reserved) |
| **BR-CONTEXT-042** | **BR-AI-042** | (Reserved) |
| **BR-CONTEXT-043** | **BR-AI-043** | (Reserved) |
| **BR-CONTEXT-OPT-001** | **BR-AI-OPT-001** | Context Optimization for Simple Investigations |
| **BR-CONTEXT-OPT-002** | **BR-AI-OPT-002** | Context Optimization for Medium Investigations |
| **BR-CONTEXT-OPT-003** | **BR-AI-OPT-003** | Context Optimization for Complex Investigations |
| **BR-CONTEXT-OPT-004** | **BR-AI-OPT-004** | Context Optimization Performance Validation |

---

## 🔍 **Migration Rationale**

### **Why These BRs Belong to AI/ML Service**

1. **Implementation Location**: All implemented in `pkg/ai/llm/` (AI/ML Service code)
2. **Test Location**: All tested in `test/unit/ai/llm/integration_test.go` (AI/ML Service tests)
3. **Architectural Role**: Context API is a **data provider** (REST API), not an LLM caller
4. **Service Boundary**: AI/ML Service owns all LLM interaction and optimization logic

### **ADR-034 Compliance**

Per ADR-034 Business Requirement Template Standard:
- **AI/ML Service** uses prefix: `BR-AI-`
- **Context API** uses prefix: `BR-CONTEXT-` (for REST API functionality only)

---

## 📝 **Documents Requiring Updates**

### **Context API Documentation**
- ✅ `docs/services/stateless/context-api/BUSINESS_REQUIREMENTS.md` - Remove AI/LLM section
- ⏳ `docs/services/stateless/context-api/BR_MAPPING.md` - Remove AI/LLM mappings
- ⏳ `docs/services/stateless/context-api/implementation/IMPLEMENTATION_PLAN_V2.12.md` - Update references

### **AI/ML Service Documentation** (Future)
- ⏳ Create `docs/services/stateless/ai-ml/BUSINESS_REQUIREMENTS.md` with renamed BRs
- ⏳ Create `docs/services/stateless/ai-ml/BR_MAPPING.md` with renamed BRs

---

## ✅ **Validation**

**Integration Coverage Impact**:
- **Before Migration**: Context API integration coverage = 31% (8 BRs / 26 total)
- **After Migration**: Context API integration coverage = 53% (8 BRs / 15 total)
- **Result**: ✅ Meets >50% target for microservices

**Test Coverage Preserved**:
- All tests remain in `test/unit/ai/llm/integration_test.go` (no code changes)
- Only documentation and BR references updated

---

**Status**: Ready for execution
**Confidence**: 100% (no code changes, documentation-only migration)

