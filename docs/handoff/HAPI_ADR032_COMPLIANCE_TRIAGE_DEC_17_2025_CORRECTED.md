# HAPI ADR-032 Compliance Triage (CORRECTED)

**Date**: December 17, 2025
**Service**: HolmesGPT API Service (HAPI)
**Triage Authority**: ADR-032 v1.3 (Mandatory Audit Requirements)
**Cross-Reference**: DD-AUDIT-003 (Service Audit Trace Requirements)
**Status**: ❌ **NON-COMPLIANT** - ADR-032 violations + DD-AUDIT-003 incorrect
**Revision**: v2.0 (Corrects initial misanalysis)

---

## 🚨 **Correction Notice**

**Initial Analysis ERROR**: First triage incorrectly stated HAPI audit duplicates AA audit.

**CORRECTED Finding**: HAPI and AA audit **DIFFERENT layers**:
- **AA**: Audits calling HAPI HTTP service (`aianalysis.holmesgpt.call`)
- **HAPI**: Audits calling external LLM providers (`aiagent.llm.request`, `aiagent.llm.response`)

**Result**: HAPI **MUST** have audit AND fix ADR-032 violations.

---

## 🎯 **Executive Summary**

### **Finding**: HAPI Has Mandatory Audit with ADR-032 Violations

**Status**: ❌ **P0 NON-COMPLIANT**
- ✅ HAPI **correctly** has audit integration (different layer than AA)
- ❌ Audit implementation **violates ADR-032 §1-§2** (graceful degradation)
- ❌ DD-AUDIT-003 is **INCORRECT** (claims HAPI shouldn't audit)

### **Impact**: High (Compliance Violation)

- ❌ **ADR-032 violations**: 9 graceful degradation patterns
- ❌ **Audit loss risk**: Operations succeed without LLM audit trail
- ⚠️ **Design doc incorrect**: DD-AUDIT-003 needs update
- ✅ **Easy fix**: Make audit ADR-032 compliant (1 hour)

### **Recommended Action**: Fix ADR-032 Violations + Update DD-AUDIT-003

**Make HAPI audit ADR-032 compliant AND update DD-AUDIT-003 to classify HAPI as P1.**

---

## 📊 **Audit Layer Analysis** ✅ **CORRECTED**

### **AA vs HAPI Audit Events - DIFFERENT Layers**

| Service | Layer | What It Audits | Example Event Type |
|---------|-------|----------------|-------------------|
| **AA (AI Analysis)** | **Controller/Orchestration** | HTTP call TO HAPI service | `aianalysis.holmesgpt.call` |
| | | CRD lifecycle | `aianalysis.phase.transition` |
| | | Analysis completion | `aianalysis.analysis.completed` |
| | | Approval decisions | `aianalysis.approval.decision` |
| **HAPI** | **LLM Provider Integration** | Prompt TO OpenAI/Anthropic | `aiagent.llm.request` |
| | | Response FROM LLM provider | `aiagent.llm.response` |
| | | LLM tool invocations | `aiagent.llm.tool_call` |
| | | Validation retries | `aiagent.workflow.validation_attempt` |

### **Why These Are NOT Duplicates**

**AA Audit Example**:
```
event_type: aianalysis.holmesgpt.call
event_data: {
  "endpoint": "http://holmesgpt-api:8080/api/v1/incident/analyze",
  "status_code": 200,
  "duration_ms": 45000
}
```
**Captures**: HTTP call to HAPI service (service-to-service interaction)

**HAPI Audit Example**:
```
event_type: llm_request
event_data: {
  "model": "claude-3-5-sonnet",
  "prompt_length": 15000,
  "prompt_preview": "Analyze this Kubernetes pod crash...",
  "toolsets_enabled": ["kubernetes/core", "workflow_catalog"]
}
```
**Captures**: LLM API call to Anthropic/OpenAI (external AI provider)

**Conclusion**: ✅ **Different layers, both required** for complete audit trail.

---

## 🔍 **ADR-032 Compliance Analysis**

### **Service Classification**

| Service | Actual Classification | DD-AUDIT-003 Classification | Recommended |
|---------|----------------------|----------------------------|-------------|
| **HAPI** | ❌ Not in ADR-032 §3 | ❌ "NO audit needed" | ✅ **P1 SHOULD audit** |

**Rationale for P1 (not P0)**:
- ⚠️ Not business-critical (wrapper service per DD-AUDIT-003 original analysis)
- ✅ Operational visibility (LLM interaction tracking)
- ✅ Debugging value (LLM failures, token usage, tool calls)
- ✅ Cost tracking (LLM API costs)
- ⚠️ Graceful degradation allowed for P1 services per ADR-032 §3

**CORRECTION**: Change DD-AUDIT-003 from "NO audit" → "P1 SHOULD audit"

---

## ❌ **ADR-032 Violations Detected**

### **Current Implementation Status**

#### ✅ **What HAPI Has**:
1. ✅ Audit store factory (`src/audit/factory.py`)
2. ✅ Buffered audit store (`src/audit/buffered_store.py`)
3. ✅ Audit events (`src/audit/events.py`) - **4 event types**
4. ✅ OpenAPI client integration (Phase 2b complete)
5. ✅ Audit calls in business logic (`incident/`, `recovery/`)

#### ❌ **ADR-032 Violations** (IF Audit Is P0/Mandatory):

| Violation | Location | ADR-032 Section | Severity |
|-----------|----------|-----------------|----------|
| **Graceful init degradation** | `src/audit/factory.py:67-68` | §1 "No Audit Loss" | ❌ P0 |
| **Silent skip on None** | `src/extensions/incident/llm_integration.py:377` | §1 "No Audit Loss" | ❌ P0 |
| **Silent skip on None** | `src/extensions/incident/llm_integration.py:408` | §1 "No Audit Loss" | ❌ P0 |
| **Silent skip on None** | `src/extensions/incident/llm_integration.py:451` | §1 "No Audit Loss" | ❌ P0 |
| **Silent skip on None** | `src/extensions/incident/llm_integration.py:509` | §1 "No Audit Loss" | ❌ P0 |
| **Silent skip on None** | `src/extensions/recovery/llm_integration.py:327` | §1 "No Audit Loss" | ❌ P0 |
| **Silent skip on None** | `src/extensions/recovery/llm_integration.py:362` | §1 "No Audit Loss" | ❌ P0 |
| **Silent skip on None** | `src/extensions/recovery/llm_integration.py:390` | §1 "No Audit Loss" | ❌ P0 |
| **No startup crash** | `src/main.py` (entire file) | §2 "No Recovery Allowed" | ❌ P0 |

**Total**: **9 ADR-032 violations**

---

## 🔧 **Required Fixes**

### **Fix 1: Make Audit Initialization Mandatory**

**File**: `src/audit/factory.py`

**Current** (Violates ADR-032 §2):
```python
def get_audit_store() -> Optional[BufferedAuditStore]:
    try:
        _audit_store = BufferedAuditStore(...)
    except Exception as e:
        logger.warning(f"Failed to initialize audit store: {e}")
        # ❌ Returns None - graceful degradation
    return _audit_store
```

**Required** (ADR-032 §2 Compliant):
```python
def get_audit_store() -> BufferedAuditStore:  # No Optional
    """
    Get or initialize the audit store singleton.

    Per ADR-032 §1: Audit is MANDATORY for LLM interactions (P1 service)
    Per ADR-032 §2: Service MUST crash if audit cannot be initialized

    Returns:
        BufferedAuditStore singleton

    Raises:
        SystemExit: If audit store cannot be initialized (ADR-032 §2)
    """
    global _audit_store
    if _audit_store is None:
        data_storage_url = os.getenv("DATA_STORAGE_URL", "http://data-storage:8080")
        try:
            _audit_store = BufferedAuditStore(...)
            logger.info(f"BR-AUDIT-005: Initialized audit store - url={data_storage_url}")
        except Exception as e:
            # ✅ COMPLIANT: Crash immediately per ADR-032 §2
            logger.error(
                f"FATAL: Failed to initialize audit store - audit is MANDATORY per ADR-032 §2: {e}"
            )
            sys.exit(1)  # Crash - NO RECOVERY ALLOWED
    return _audit_store
```

**Changes**:
- ✅ Remove `Optional` from return type
- ✅ Change `logger.warning` → `logger.error`
- ✅ Add `sys.exit(1)` on failure
- ✅ Update docstring to reference ADR-032 §2

---

### **Fix 2: Replace Silent Skip with Error Checks**

**Files**: `src/extensions/incident/llm_integration.py` (4 locations), `src/extensions/recovery/llm_integration.py` (3 locations)

**Current** (Violates ADR-032 §1):
```python
# ❌ VIOLATION: Silent skip if None
if audit_store:
    audit_store.store_audit(create_llm_request_event(...))
```

**Required** (ADR-032 §1 Compliant):
```python
# ✅ COMPLIANT: Error if None per ADR-032 §1
if audit_store is None:
    logger.error(
        "CRITICAL: audit_store is None - audit is MANDATORY per ADR-032 §1",
        extra={
            "incident_id": incident_id,
            "remediation_id": remediation_id,
        }
    )
    raise RuntimeError("audit_store is None - audit is MANDATORY per ADR-032 §1")

# Non-blocking fire-and-forget (ADR-038 pattern)
audit_store.store_audit(create_llm_request_event(...))
```

**Changes**:
- ✅ Replace `if audit_store:` with `if audit_store is None:` + error
- ✅ Raise `RuntimeError` to fail request
- ✅ Log at ERROR level with context
- ✅ Reference ADR-032 §1 in error message

**Locations to Update** (7 total):
1. `incident/llm_integration.py:377` - LLM request audit
2. `incident/llm_integration.py:408` - LLM response audit
3. `incident/llm_integration.py:451` - Tool call audit
4. `incident/llm_integration.py:509` - Validation attempt audit
5. `recovery/llm_integration.py:327` - LLM request audit
6. `recovery/llm_integration.py:362` - LLM response audit
7. `recovery/llm_integration.py:390` - Tool call audit

---

### **Fix 3: Add Startup Audit Validation**

**File**: `src/main.py`

**Current**: No audit validation at startup

**Required** (ADR-032 §2 Compliant):
```python
@app.on_event("startup")
async def startup_event():
    global config_manager

    logger.info(f"Starting {config.get('service_name', 'holmesgpt-api')} v{config.get('version', '1.0.0')}")

    # ✅ COMPLIANT: Validate audit at startup (ADR-032 §2)
    # Per ADR-032 §3: HAPI is P1 service - audit is MANDATORY for LLM interactions
    from src.audit.factory import get_audit_store
    try:
        audit_store = get_audit_store()  # Will crash if init fails
        logger.info({
            "event": "audit_store_initialized",
            "status": "mandatory_per_adr_032",
            "classification": "P1",
        })
    except Exception as e:
        logger.error(f"FATAL: Audit initialization failed - service cannot start per ADR-032 §2: {e}")
        sys.exit(1)  # Crash immediately - Kubernetes will restart pod

    # ... rest of startup logic ...
```

**Changes**:
- ✅ Add audit initialization validation
- ✅ Crash with `sys.exit(1)` if audit unavailable
- ✅ Log classification (P1) and ADR-032 reference

---

## 📋 **Update DD-AUDIT-003**

### **Current DD-AUDIT-003 Entry** ❌ **INCORRECT**

> #### 10. HolmesGPT API Service ❌
>
> **Status**: ⚠️ **NO** audit traces needed (delegated to AI Analysis Controller)
>
> **Rationale**:
> - ❌ **Wrapper Service**: Thin wrapper around HolmesGPT SDK
> - ❌ **No State Changes**: Only proxies requests to external LLM
> - ❌ **Audit Responsibility**: AI Analysis Controller audits LLM interactions

**Why This Is Wrong**:
- AA audits calling HAPI service (HTTP layer)
- HAPI audits calling LLM providers (LLM layer)
- These are DIFFERENT audit layers

---

### **Required DD-AUDIT-003 Update** ✅ **CORRECTED**

> #### 10. HolmesGPT API Service ✅
>
> **Status**: ✅ **SHOULD** generate audit traces (P1 - operational visibility)
>
> **Rationale**:
> - ✅ **LLM Provider Integration**: Audits external LLM API calls (OpenAI, Anthropic, etc.)
> - ✅ **Different Layer**: AA audits calling HAPI (HTTP), HAPI audits calling LLM (AI provider)
> - ✅ **Debugging Value**: Critical for troubleshooting LLM failures, token usage, tool calls
> - ✅ **Cost Tracking**: LLM API costs need monitoring
> - ✅ **Compliance**: AI decision-making requires audit trail (AI Act, SOC 2)
> - ⚠️ **Not Business-Critical**: Wrapper service (P1, not P0)
>
> **Audit Events**:
>
> | Event Type | Description | Priority |
> |------------|-------------|----------|
> | `aiagent.llm.request` | LLM prompt sent to external provider | P1 |
> | `aiagent.llm.response` | LLM response received from provider | P1 |
> | `aiagent.llm.tool_call` | LLM tool invocation | P1 |
> | `aiagent.workflow.validation_attempt` | Validation retry event | P1 |
>
> **Industry Precedent**: OpenAI API logs, Anthropic Claude logs, AWS Bedrock audit logs
>
> **Expected Volume**: 1,000 events/day, 30 MB/month
>
> **Classification**: P1 (SHOULD audit) - graceful degradation allowed if audit unavailable

**Changes**:
- ✅ Status: ❌ NO → ✅ SHOULD (P1)
- ✅ Add LLM provider integration rationale
- ✅ Clarify audit layer separation (HAPI≠AA)
- ✅ Add audit event table
- ✅ Classify as P1 (not P0)

---

## 🎯 **Resolution Plan**

### **Option A: Make HAPI Audit ADR-032 Compliant** ✅ **RECOMMENDED**

**Rationale**: HAPI audits different layer than AA, audit is required

**Changes Required**:
1. ✅ **Update** `src/audit/factory.py` - crash on init failure (ADR-032 §2)
2. ✅ **Remove** `Optional` from return type
3. ✅ **Replace** `if audit_store:` with error checks in 7 locations (ADR-032 §1)
4. ✅ **Add** startup audit validation in `src/main.py` (ADR-032 §2)
5. ✅ **Update** DD-AUDIT-003 to classify HAPI as P1
6. ✅ **Update** ADR-032 §3 to add HAPI row

**Effort**: 1 hour

**Benefits**:
- ✅ ADR-032 compliant
- ✅ Complete audit trail (AA + HAPI layers)
- ✅ LLM interaction visibility
- ✅ Cost tracking for LLM API calls
- ✅ Debugging support for LLM failures

**Risks**: None (correct design)

---

### **Option B: Remove HAPI Audit** ❌ **NOT RECOMMENDED**

**Rationale**: Would create audit gap at LLM provider layer

**Risks**:
- ❌ Lose LLM interaction audit trail
- ❌ No visibility into external LLM calls
- ❌ Cannot track LLM API costs
- ❌ Debugging LLM failures becomes harder
- ❌ Violates "complete audit trail" principle

---

## 📊 **Implementation Plan**

### **Phase 1: Fix ADR-032 Violations** (45 min)

#### **Step 1: Fix Audit Factory** (10 min)

```bash
# File: src/audit/factory.py
# Changes:
# 1. Remove Optional from return type
# 2. Add sys.exit(1) on failure
# 3. Update docstring
```

**Updated Code**:
```python
import sys  # Add import

def get_audit_store() -> BufferedAuditStore:  # Remove Optional
    """
    Per ADR-032 §2: Audit is MANDATORY - service MUST crash if init fails
    """
    global _audit_store
    if _audit_store is None:
        try:
            _audit_store = BufferedAuditStore(...)
            logger.info("BR-AUDIT-005: Initialized audit store")
        except Exception as e:
            logger.error(f"FATAL: ADR-032 §2 - audit init failed: {e}")
            sys.exit(1)  # Crash - NO RECOVERY
    return _audit_store
```

#### **Step 2: Fix Silent Skips** (25 min)

**Files to Update** (7 locations):
- `src/extensions/incident/llm_integration.py` (4 locations)
- `src/extensions/recovery/llm_integration.py` (3 locations)

**Pattern Replacement**:
```python
# OLD (violates ADR-032 §1)
if audit_store:
    audit_store.store_audit(event)

# NEW (ADR-032 §1 compliant)
if audit_store is None:
    logger.error("CRITICAL: audit_store is None - MANDATORY per ADR-032 §1")
    raise RuntimeError("audit is MANDATORY per ADR-032 §1")
audit_store.store_audit(event)
```

#### **Step 3: Add Startup Validation** (10 min)

```python
# File: src/main.py
@app.on_event("startup")
async def startup_event():
    # Add audit validation
    from src.audit.factory import get_audit_store
    try:
        audit_store = get_audit_store()
        logger.info({"event": "audit_initialized", "classification": "P1"})
    except Exception as e:
        logger.error(f"FATAL: ADR-032 §2 - audit init failed: {e}")
        sys.exit(1)
```

---

### **Phase 2: Update Documentation** (15 min)

#### **Step 1: Update DD-AUDIT-003** (5 min)

```markdown
# File: docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md

# Change HAPI entry from "NO audit" to "SHOULD audit (P1)"
# Add audit event table
# Clarify layer separation
```

#### **Step 2: Update ADR-032 §3** (5 min)

```markdown
# File: docs/architecture/decisions/ADR-032-data-access-layer-isolation.md

# Add HAPI row to service classification table:
| **HAPI** | ✅ SHOULD (P1) | ❌ NO | ✅ YES (by design) | src/main.py:315 |
```

#### **Step 3: Update This Triage** (5 min)

Mark as **RESOLVED** with implementation date.

---

## ✅ **Verification Checklist**

### **Pre-Implementation**:
- [ ] Confirm Option A (Fix Violations) is approved
- [ ] Review ADR-032 v1.3 §1-§4
- [ ] Backup current implementation (git branch)

### **Phase 1: ADR-032 Fixes**:
- [ ] Remove `Optional` from `get_audit_store()` return type
- [ ] Add `sys.exit(1)` in factory.py on init failure
- [ ] Replace `if audit_store:` with error checks (7 locations)
- [ ] Add startup validation in main.py
- [ ] Add ADR-032 references in error messages

### **Phase 2: Documentation**:
- [ ] Update DD-AUDIT-003: NO → SHOULD (P1)
- [ ] Update ADR-032 §3: Add HAPI row
- [ ] Update BR-AUDIT-005: Clarify HAPI scope
- [ ] Mark this triage as RESOLVED

### **Post-Implementation**:
- [ ] Unit tests pass (100%)
- [ ] Integration tests pass (100%)
- [ ] Service crashes if audit init fails (verify with test)
- [ ] Service crashes if audit store is None (verify with test)
- [ ] Application logs confirm mandatory audit

---

## 📚 **Updated Service Classification Matrix**

| Service | ADR-032 Classification | Current Code | Compliance Status | Fix Required |
|---------|------------------------|--------------|-------------------|--------------|
| **HAPI** | ✅ P1 SHOULD audit | ⚠️ Has audit with violations | ❌ **NON-COMPLIANT** | Fix violations |
| **AA** | ✅ P0 MUST audit | ✅ Graceful degradation (optional) | ✅ **COMPLIANT** | None |

**Key Difference**:
- **AA** (P0): Optional audit (graceful degradation allowed per design)
- **HAPI** (P1): Mandatory audit for LLM interactions (ADR-032 §2)

---

## 🎯 **Key Takeaways**

### **For HAPI Team**

1. ✅ **HAPI audit IS required** - audits different layer than AA
2. ❌ **Current implementation violates ADR-032** (9 violations)
3. ✅ **Recommended action**: Fix violations (Option A) - 1 hour effort
4. ✅ **Service classification**: P1 (SHOULD audit)
5. ✅ **Complete audit trail**: AA (HTTP) + HAPI (LLM)

### **For Platform Team**

1. ❌ **DD-AUDIT-003 is incorrect** - needs update
2. ✅ **HAPI and AA audit different layers** - both required
3. ✅ **Add HAPI to ADR-032 §3** as P1 service
4. ✅ **Option A is correct design** - fix violations

### **For Compliance/Audit Team**

1. ✅ **Complete audit trail requires both layers**:
   - AA: HTTP calls to HAPI
   - HAPI: LLM API calls to providers
2. ❌ **Current HAPI audit has compliance gaps** (graceful degradation)
3. ✅ **Fix provides complete LLM audit trail** (cost tracking, debugging)
4. ✅ **Recommend**: Approve Option A (Fix ADR-032 violations)

---

## 📚 **Related Documents**

| Document | Relationship | Update Required |
|----------|-------------|-----------------|
| **ADR-032 v1.3** | Mandatory audit requirements | ✅ Add HAPI to §3 |
| **DD-AUDIT-003** | Service audit trace requirements | ✅ Change HAPI: NO → P1 |
| **BR-AUDIT-005** | Workflow selection audit trail | ✅ Clarify HAPI scope |
| **ADR-032-MANDATORY-AUDIT-UPDATE.md** | ADR-032 update summary | ✅ Add HAPI violations |

---

**Prepared by**: Jordi Gil
**Triage Date**: December 17, 2025
**Revision**: v2.0 (Corrected)
**Recommended Resolution**: Option A (Fix ADR-032 Violations)
**Estimated Effort**: 1 hour
**Status**: ⚠️ **AWAITING APPROVAL** - Option A recommended


