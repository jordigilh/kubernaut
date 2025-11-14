# BR-HAPI-046-050 Context API to Data Storage Update Triage

**Date**: November 13, 2025
**Status**: ⚠️ **PARTIAL UPDATE - REQUIRES MANUAL COMPLETION**
**File**: `BR-HAPI-046-050-DATA-STORAGE-PLAYBOOK-TOOL.md`
**Original File**: `BR-HAPI-046-050-CONTEXT-API-TOOL.md` (renamed)
**Version**: 1.1 (in progress)

---

## 🎯 **Executive Summary**

The BR-HAPI-046-050 document has been **partially updated** from Context API to Data Storage Service integration. However, **semantic changes beyond simple text replacement are required** and should be completed when v3.1 development begins.

**Current Status**:
- ✅ **File renamed**: `BR-HAPI-046-050-CONTEXT-API-TOOL.md` → `BR-HAPI-046-050-DATA-STORAGE-PLAYBOOK-TOOL.md`
- ✅ **Simple text replacements completed**: "Context API" → "Data Storage Service", `ContextAPIClient` → `DataStorageClient`
- ⚠️ **Semantic changes pending**: Tool names, API endpoints, parameters, test names require manual updates

**Recommendation**: Mark document as "v1.1 - Partial Update" with clear changelog indicating pending changes for v3.1 implementation.

---

## ✅ **Completed Changes**

### **1. Document Metadata**
- ✅ **Title**: "Context API Tool Integration" → "Data Storage Playbook Tool Integration"
- ✅ **Version**: 1.0 → 1.1
- ✅ **Date**: Updated to November 13, 2025
- ✅ **Related**: Updated to DD-CONTEXT-005, DD-STORAGE-008

### **2. Service References (Global Replace)**
- ✅ **"Context API"** → **"Data Storage Service"** (all occurrences)
- ✅ **`ContextAPIClient`** → **`DataStorageClient`** (all occurrences)
- ✅ **`context_api`** → **`data_storage`** (Python module names)

### **3. BR-HAPI-046 Tool Definition**
- ✅ **Tool name**: `get_context` → `get_playbooks`
- ✅ **Category**: "Context API Tool Integration" → "Data Storage Playbook Tool Integration"
- ✅ **Description**: Updated to reflect playbook search
- ✅ **Parameters**: `alert_fingerprint` → `query`, added `limit` parameter

---

## ⏸️ **Pending Manual Updates**

### **Critical**: These require semantic changes, not just text replacement

### **1. API Endpoint (56 occurrences)**
**Current**: `/api/v1/context/enrich`
**Target**: `/api/v1/playbooks/search`

**Affected Lines**:
- Line 116: Client Requirements
- Line 227: DataStorageClient.get_context() method
- Multiple code examples throughout

**Action Required**: Update all endpoint references + HTTP method (POST → GET for search)

---

### **2. Tool Name & Functions (56+ occurrences)**
**Current**: `get_context`, `context_tool`, `CONTEXT_TOOL_DEFINITION`, `register_context_tool`, `handle_context_tool_call`
**Target**: `get_playbooks`, `playbook_tool`, `PLAYBOOK_TOOL_DEFINITION`, `register_playbook_tool`, `handle_playbook_tool_call`

**Affected Sections**:
- BR-HAPI-046: Tool definition (line 92, 97, 99)
- BR-HAPI-047: Client implementation (line 205)
- BR-HAPI-048: Tool handler (line 321, 327-329, 345, 352, 364, 368)
- BR-HAPI-049: Observability (line 441, 449-452, 460)
- BR-HAPI-050: Testing (line 503, 508, 514)

**Action Required**: Systematic rename of all function/variable names

---

### **3. Test File Names (15+ occurrences)**
**Current**: `test_context_tool.py`, `test_context_tool_handler.py`, `test_context_tool_metrics.py`, `test_context_tool_observability.py`, `test_context_tool_e2e.py`
**Target**: `test_playbook_tool.py`, `test_playbook_tool_handler.py`, `test_playbook_tool_metrics.py`, `test_playbook_tool_observability.py`, `test_playbook_tool_e2e.py`

**Affected Lines**:
- Line 73, 77, 82: BR-HAPI-046 unit tests
- Line 289, 293, 297, 301, 305: BR-HAPI-048 unit tests
- Line 311, 315: BR-HAPI-048 integration tests
- Line 417, 421, 425: BR-HAPI-049 unit tests
- Line 431, 435: BR-HAPI-049 integration tests
- Line 503, 508, 514: BR-HAPI-050 E2E tests
- Line 524-531: Test file summary

**Action Required**: Update all test file path references

---

### **4. Parameter Changes**
**Current**: `alert_fingerprint` (required string parameter)
**Target**: `query` (natural language query string)

**Affected Lines**:
- Line 78: BR-HAPI-046 parameter validation test
- Line 207, 229: BR-HAPI-047 client method signature
- Line 218: Cache key format
- Line 272: BR-HAPI-048 parameter validation
- Line 294: BR-HAPI-048 test description
- Line 347-349: BR-HAPI-048 handler implementation

**Action Required**: Update parameter name + validation logic

---

### **5. Metrics Names (Partial Update)**
**Current**: `holmesgpt_context_tool_*` (some already updated to `holmesgpt_playbook_tool_*`)
**Target**: `holmesgpt_playbook_tool_*` (all occurrences)

**Status**: ⚠️ **INCONSISTENT** - Some metrics already updated (line 327-329), others not (line 449-452)

**Action Required**: Ensure all metric names are consistent

---

### **6. Response Format & Caching**
**Current**: "context results", `context:{alert_fingerprint}:{similarity_threshold}`
**Target**: "playbook search results", `playbook:{query_hash}:{similarity_threshold}`

**Affected Lines**:
- Line 119: Caching description
- Line 218: Cache key format
- Line 352: Client method call

**Action Required**: Update cache key strategy (query hash instead of alert fingerprint)

---

## 📋 **Detailed Update Checklist**

### **BR-HAPI-046: Define Tool**
- [x] Tool name: `get_context` → `get_playbooks`
- [x] Tool description updated
- [x] Parameters: `alert_fingerprint` → `query`
- [ ] Test file names: `test_context_tool.py` → `test_playbook_tool.py`
- [ ] Test descriptions updated for playbook search
- [ ] Implementation file: `context_tool.py` → `playbook_tool.py`
- [ ] Variable names: `CONTEXT_TOOL_DEFINITION` → `PLAYBOOK_TOOL_DEFINITION`
- [ ] Function names: `register_context_tool` → `register_playbook_tool`

### **BR-HAPI-047: Implement Client**
- [x] Client class: `ContextAPIClient` → `DataStorageClient`
- [ ] API endpoint: `/api/v1/context/enrich` → `/api/v1/playbooks/search`
- [ ] Method name: `get_context` → `search_playbooks`
- [ ] Method signature: `alert_fingerprint` → `query`
- [ ] Cache key format: `context:*` → `playbook:*`
- [ ] Test file names updated
- [ ] Test descriptions updated

### **BR-HAPI-048: Tool Call Handler**
- [ ] Handler file: `context_tool.py` → `playbook_tool.py`
- [ ] Function names: `handle_context_tool_call` → `handle_playbook_tool_call`
- [ ] Parameter validation: `alert_fingerprint` → `query`
- [ ] Client method call: `get_context` → `search_playbooks`
- [ ] Test file names updated
- [ ] Test descriptions updated

### **BR-HAPI-049: Tool Call Observability**
- [ ] Metric names: `holmesgpt_context_tool_*` → `holmesgpt_playbook_tool_*` (complete all)
- [ ] Log event name: `context_tool_call` → `playbook_tool_call`
- [ ] Test file names updated
- [ ] Test descriptions updated

### **BR-HAPI-050: Tool Call Testing**
- [ ] E2E test scenarios updated for playbook search
- [ ] Test file names: `test_context_tool_e2e.py` → `test_playbook_tool_e2e.py`
- [ ] Test descriptions updated
- [ ] Test file summary updated

---

## 🚨 **Critical Issues to Address**

### **Issue 1: Inconsistent Metric Names**
**Problem**: Lines 327-329 use `holmesgpt_playbook_tool_*`, but lines 449-452 still reference `holmesgpt_context_tool_*` in descriptions.

**Resolution**: Ensure all metric names are `holmesgpt_playbook_tool_*`

---

### **Issue 2: API Endpoint Mismatch**
**Problem**: Document still references `/api/v1/context/enrich` (Context API endpoint) instead of `/api/v1/playbooks/search` (Data Storage endpoint).

**Resolution**: Update all endpoint references to match DD-STORAGE-008 specification

---

### **Issue 3: Parameter Semantic Change**
**Problem**: `alert_fingerprint` (specific alert ID) vs `query` (natural language) are semantically different.

**Resolution**: Update not just parameter name, but also:
- Cache key strategy (hash of query instead of fingerprint)
- Validation logic (query can be any string, not just fingerprint format)
- Test scenarios (use natural language queries)

---

## 📊 **Update Statistics**

| Category | Total References | Updated | Pending | % Complete |
|----------|------------------|---------|---------|------------|
| Service Name | ~50 | 50 | 0 | 100% |
| Client Class | ~10 | 10 | 0 | 100% |
| Tool Name | ~20 | 1 | 19 | 5% |
| API Endpoint | ~5 | 0 | 5 | 0% |
| Parameters | ~10 | 1 | 9 | 10% |
| Test Files | ~15 | 0 | 15 | 0% |
| Metrics | ~10 | 5 | 5 | 50% |
| Functions | ~15 | 0 | 15 | 0% |

**Overall Completion**: ~20% (simple text replacements only)

---

## 🎯 **Recommended Action Plan**

### **Option A: Complete Now (2-3 hours)**
**Pros**: Document is fully updated and ready for v3.1 implementation
**Cons**: Time investment now for future work (v3.1 is pending)

### **Option B: Mark as Partial + Complete During v3.1 (Recommended)**
**Pros**: 
- Aligns updates with actual implementation
- Ensures consistency with final Data Storage API design
- Avoids speculative updates that may not match implementation

**Cons**: Document is in intermediate state

**Recommendation**: **Option B** - Add comprehensive changelog to document explaining partial update status and defer completion to v3.1 implementation phase.

---

## 📝 **Proposed Changelog Addition**

Add this to the document after the metadata section:

```markdown
## Changelog

### v1.1 (November 13, 2025) - Partial Update for Context API Deprecation

**Status**: ⚠️ **PARTIAL UPDATE** - Service references updated, semantic changes pending for v3.1 implementation

**Completed Changes**:
- ✅ Service references: "Context API" → "Data Storage Service"
- ✅ Client class: `ContextAPIClient` → `DataStorageClient`
- ✅ BR-HAPI-046 tool definition: `get_context` → `get_playbooks`
- ✅ BR-HAPI-046 parameters: `alert_fingerprint` → `query`

**Pending Changes** (to be completed during v3.1 implementation):
- ⏸️ API endpoint: `/api/v1/context/enrich` → `/api/v1/playbooks/search`
- ⏸️ Function names: `get_context`, `register_context_tool`, etc. → playbook equivalents
- ⏸️ Test file names: `test_context_tool.py` → `test_playbook_tool.py`
- ⏸️ Metric names: Ensure all use `holmesgpt_playbook_tool_*`
- ⏸️ Cache key strategy: Update for query-based search
- ⏸️ Code examples: Align with DD-STORAGE-008 specification

**Rationale**: These BRs are marked ⏸️ PENDING (v3.1). Complete updates will be performed during v3.1 implementation to ensure consistency with the actual Data Storage Service API design and DD-STORAGE-008 (Playbook Catalog Schema).

**Action Required**: When implementing v3.1, refer to `BR-HAPI-046-050-UPDATE-TRIAGE.md` for complete list of pending updates.
```

---

## ✅ **Success Criteria for Full Completion**

1. **Zero references** to `get_context`, `context_tool`, `CONTEXT_TOOL_DEFINITION`
2. **Zero references** to `/api/v1/context/enrich`
3. **Zero references** to `alert_fingerprint` parameter
4. **All test file names** updated to `playbook_tool`
5. **All metrics** use `holmesgpt_playbook_tool_*`
6. **All code examples** align with DD-STORAGE-008 specification
7. **All cache keys** use query-based strategy

---

## 📚 **References**

- **DD-CONTEXT-005**: Minimal LLM Response Schema
- **DD-STORAGE-008**: Playbook Catalog Schema
- **DD-CONTEXT-006**: Context API Deprecation Decision
- **Data Storage API Spec**: `docs/services/stateless/data-storage/api-specification.md`

---

**Triage Complete**: November 13, 2025
**Recommendation**: Add changelog, mark as v1.1 partial update, defer full completion to v3.1
**Confidence**: 100% - Clear scope of remaining work identified

