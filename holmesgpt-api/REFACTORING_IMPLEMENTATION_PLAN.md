# HAPI Code Refactoring Implementation Plan

**Date**: 2025-12-14
**Status**: ⏳ **AWAITING APPROVAL**
**Estimated Effort**: 15-25 hours (2-3 days)
**Risk Level**: **LOW** (comprehensive test coverage validates no regressions)

---

## 🎯 **REFACTORING GOALS**

1. **Improve Maintainability**: Split 3 large files (4,428 lines) into 30 focused modules (200-400 lines each)
2. **Reduce Complexity**: Decrease nesting, extract constants, remove duplication
3. **Preserve Functionality**: Zero behavioral changes - refactoring only
4. **Maintain Test Coverage**: All 651+ tests must pass after refactoring

---

## 📋 **PHASE 1: SPLIT LARGE FILES** (10-15 hours)

### **Task 1.1: Split `src/extensions/incident.py` (1,593 lines → 5 modules)**

**Estimated Effort**: 4-6 hours

#### **New Module Structure**:

```
src/extensions/incident/
├── __init__.py (re-exports for backward compatibility)
├── endpoint.py (300 lines)
│   ├── FastAPI router and endpoint definition
│   ├── @router.post("/incident/analyze")
│   └── Request/response handling
│
├── llm_integration.py (400 lines)
│   ├── analyze_incident() - main business logic
│   ├── HolmesGPT SDK integration
│   ├── LLM self-correction loop (BR-HAPI-197, DD-HAPI-002 v1.2)
│   ├── MinimalDAL class
│   └── _create_data_storage_client()
│
├── prompt_builder.py (400 lines)
│   ├── _create_incident_investigation_prompt()
│   ├── _build_cluster_context_section()
│   ├── _build_mcp_filter_instructions()
│   └── _build_validation_error_feedback()
│
├── result_parser.py (300 lines)
│   ├── _parse_and_validate_investigation_result()
│   ├── _parse_investigation_result() (deprecated)
│   ├── _is_target_in_owner_chain()
│   └── _determine_human_review_reason()
│
└── constants.py (100 lines)
    ├── MAX_VALIDATION_ATTEMPTS = 3
    ├── Severity level descriptions
    ├── Risk tolerance guidance
    └── Priority descriptions
```

#### **Backward Compatibility**:
```python
# src/extensions/incident/__init__.py
"""
Incident Analysis Endpoint

Business Requirements: BR-HAPI-002 (Incident Analysis)
Design Decision: DD-RECOVERY-003 (DetectedLabels for workflow filtering)

This package was refactored from a single 1,593-line file into focused modules.
Imports are re-exported for backward compatibility.
"""

# Re-export everything for backward compatibility
from .endpoint import router, incident_analyze_endpoint
from .llm_integration import analyze_incident
from .constants import MAX_VALIDATION_ATTEMPTS

__all__ = [
    "router",
    "incident_analyze_endpoint",
    "analyze_incident",
    "MAX_VALIDATION_ATTEMPTS"
]
```

#### **Test Updates**:
- Update imports in test files: `from src.extensions.incident import analyze_incident`
- No test logic changes required (all tests should pass)

---

### **Task 1.2: Split `src/extensions/recovery.py` (1,726 lines → 5 modules)**

**Estimated Effort**: 4-6 hours

#### **New Module Structure**:

```
src/extensions/recovery/
├── __init__.py (re-exports for backward compatibility)
├── endpoint.py (300 lines)
│   ├── FastAPI router and endpoint definition
│   ├── @router.post("/recovery/analyze")
│   └── Request/response handling
│
├── llm_integration.py (400 lines)
│   ├── analyze_recovery() - main business logic
│   ├── HolmesGPT SDK integration
│   ├── Recovery analysis logic
│   ├── MinimalDAL class
│   └── _create_data_storage_client()
│
├── prompt_builder.py (400 lines)
│   ├── _create_recovery_analysis_prompt()
│   ├── _build_previous_execution_section()
│   ├── _build_execution_failure_context()
│   └── Recovery-specific context builders
│
├── result_parser.py (300 lines)
│   ├── _parse_recovery_result()
│   ├── _validate_recovery_strategy()
│   └── Result transformation logic
│
└── constants.py (100 lines)
    ├── MAX_RECOVERY_ATTEMPTS
    ├── Recovery strategy descriptions
    └── Risk assessment constants
```

#### **Backward Compatibility**:
```python
# src/extensions/recovery/__init__.py
from .endpoint import router, recovery_analyze_endpoint
from .llm_integration import analyze_recovery

__all__ = [
    "router",
    "recovery_analyze_endpoint",
    "analyze_recovery"
]
```

---

### **Task 1.3: Split `src/toolsets/workflow_catalog.py` (1,110 lines → 5 modules)**

**Estimated Effort**: 3-4 hours

#### **New Module Structure**:

```
src/toolsets/workflow_catalog/
├── __init__.py (re-exports for backward compatibility)
├── toolset.py (200 lines)
│   ├── WorkflowCatalogToolset class (HolmesGPT SDK interface)
│   ├── SearchWorkflowCatalogTool class
│   └── Public API methods
│
├── search_client.py (300 lines)
│   ├── _search_workflows() - OpenAPI client integration
│   ├── Error handling (ApiException, NotFoundException)
│   └── HTTP request/response logic
│
├── filter_builder.py (300 lines)
│   ├── _build_filters_from_query()
│   ├── _parse_structured_query()
│   ├── Query parsing logic (DD-LLM-001)
│   └── Filter construction (DD-WORKFLOW-001 v1.6)
│
├── detected_labels.py (200 lines)
│   ├── _validate_detected_labels_against_rca()
│   ├── DetectedLabels 100% safe validation (DD-WORKFLOW-001 v1.7)
│   ├── Owner chain comparison
│   └── Cluster-scoped resource handling
│
└── result_transformer.py (100 lines)
    ├── _transform_api_response()
    ├── Workflow result formatting (DD-WORKFLOW-002 v3.0)
    └── JSON serialization
```

#### **Backward Compatibility**:
```python
# src/toolsets/workflow_catalog/__init__.py
from .toolset import WorkflowCatalogToolset, SearchWorkflowCatalogTool

__all__ = [
    "WorkflowCatalogToolset",
    "SearchWorkflowCatalogTool"
]
```

---

## 📋 **PHASE 2: EXTRACT SHARED CODE** (4-6 hours)

### **Task 2.1: Extract Audit Store Factory**

**Estimated Effort**: 30 minutes

#### **Create: `src/audit/store_factory.py`**

```python
"""
Audit Store Factory - Single source of truth for audit store initialization.

Business Requirement: BR-AUDIT-005 (Workflow Selection Audit Trail)
Architecture: ADR-038 (Async Buffered Audit Ingestion)

This factory provides a global singleton audit store instance, eliminating
duplication across incident.py, recovery.py, and postexec.py.
"""

import os
import logging
from typing import Optional

from src.audit import BufferedAuditStore, AuditConfig

logger = logging.getLogger(__name__)

_audit_store: Optional[BufferedAuditStore] = None


def get_audit_store() -> Optional[BufferedAuditStore]:
    """
    Get or initialize the global audit store singleton (ADR-038).

    Configuration:
    - DATA_STORAGE_URL: Data Storage Service endpoint
    - Buffer size: 10,000 events
    - Batch size: 50 events
    - Flush interval: 5.0 seconds

    Returns:
        BufferedAuditStore instance or None if initialization fails
    """
    global _audit_store
    if _audit_store is None:
        data_storage_url = os.getenv("DATA_STORAGE_URL", "http://data-storage:8080")
        try:
            _audit_store = BufferedAuditStore(
                data_storage_url=data_storage_url,
                config=AuditConfig(
                    buffer_size=10000,
                    batch_size=50,
                    flush_interval_seconds=5.0
                )
            )
            logger.info(f"BR-AUDIT-005: Initialized global audit store - url={data_storage_url}")
        except Exception as e:
            logger.warning(f"BR-AUDIT-005: Failed to initialize audit store: {e}")
    return _audit_store
```

#### **Update Files**:
- `src/extensions/incident/llm_integration.py`: `from src.audit.store_factory import get_audit_store`
- `src/extensions/recovery/llm_integration.py`: `from src.audit.store_factory import get_audit_store`
- `src/extensions/postexec.py`: `from src.audit.store_factory import get_audit_store` (V1.1)

---

### **Task 2.2: Extract Application Constants**

**Estimated Effort**: 1 hour

#### **Create: `src/constants.py`**

```python
"""
Application Constants - Single source of truth for magic numbers.

Business Requirements:
- BR-HAPI-036 (HTTP Server)
- BR-AUDIT-005 (Audit Trail)
- BR-HAPI-197 (Human Review)
- BR-HAPI-250 (Workflow Catalog)
"""

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# HTTP STATUS CODES (RFC 7807)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
HTTP_OK = 200
HTTP_CREATED = 201
HTTP_BAD_REQUEST = 400
HTTP_UNAUTHORIZED = 401
HTTP_FORBIDDEN = 403
HTTP_NOT_FOUND = 404
HTTP_INTERNAL_ERROR = 500
HTTP_SERVICE_UNAVAILABLE = 503

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# AUDIT CONFIGURATION (ADR-038)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
AUDIT_BUFFER_SIZE = 10000  # Max events before forced flush
AUDIT_BATCH_SIZE = 50  # Events per batch write
AUDIT_FLUSH_INTERVAL_SECONDS = 5.0  # Seconds between flushes

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# LLM CONFIGURATION
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
LLM_MAX_VALIDATION_ATTEMPTS = 3  # BR-HAPI-197: Max self-correction attempts
LLM_DEFAULT_TIMEOUT_SECONDS = 60  # BR-HAPI-026: LLM call timeout
LLM_DEFAULT_MAX_RETRIES = 3  # BR-HAPI-026: LLM retry count

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# WORKFLOW CATALOG CONFIGURATION (BR-HAPI-250)
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
WORKFLOW_SEARCH_MAX_RESULTS = 10  # top_k limit
WORKFLOW_SEARCH_TIMEOUT_SECONDS = 10  # Data Storage timeout
WORKFLOW_MIN_SIMILARITY = 0.3  # 30% minimum similarity threshold

# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
# CONFIDENCE THRESHOLDS
# ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
CONFIDENCE_LOW_THRESHOLD = 0.7  # Below this triggers needs_human_review
CONFIDENCE_HIGH_THRESHOLD = 0.9  # Above this indicates high confidence
```

#### **Update Files**:
- Replace all hardcoded constants with imports from `src.constants`
- Update `src/audit/store_factory.py` to use `AUDIT_*` constants
- Update `src/extensions/incident/constants.py` to use `LLM_MAX_VALIDATION_ATTEMPTS`

---

### **Task 2.3: Reduce Auth Middleware Nesting**

**Estimated Effort**: 1-2 hours

#### **Current Pattern** (nested):
```python
if token:
    try:
        if validate_token(token):
            try:
                user = get_user(token)
                if user:
                    if user.is_active:
                        return user
                    else:
                        raise HTTPException(...)
                else:
                    raise HTTPException(...)
            except Exception as e:
                logger.error(...)
                raise HTTPException(...)
        else:
            raise HTTPException(...)
    except Exception as e:
        logger.error(...)
        raise HTTPException(...)
else:
    raise HTTPException(...)
```

#### **Refactored Pattern** (guard clauses):
```python
# Guard clause pattern reduces nesting from 6 levels to 1-2 levels

if not token:
    raise HTTPException(status_code=HTTP_UNAUTHORIZED, detail="Missing authorization token")

try:
    if not validate_token(token):
        raise HTTPException(status_code=HTTP_UNAUTHORIZED, detail="Invalid token")

    user = get_user(token)
    if not user:
        raise HTTPException(status_code=HTTP_UNAUTHORIZED, detail="User not found")

    if not user.is_active:
        raise HTTPException(status_code=HTTP_FORBIDDEN, detail="User inactive")

    return user

except HTTPException:
    raise  # Re-raise auth exceptions
except Exception as e:
    logger.error(f"Auth error: {e}")
    raise HTTPException(status_code=HTTP_INTERNAL_ERROR, detail="Internal auth error")
```

---

### **Task 2.4: Convert String Concatenation to f-strings**

**Estimated Effort**: 15 minutes (automated)

#### **Script**:
```bash
#!/bin/bash
# Automated f-string conversion for business code only

for file in $(find src -name "*.py" -type f | grep -v "src/clients/"); do
    # This is a placeholder - actual conversion requires careful AST analysis
    # to avoid breaking intentional string concatenation
    echo "Review: $file"
done
```

**Manual Review Required**: Some string concatenation is intentional (e.g., logging formatters)

---

## 📋 **PHASE 3: VALIDATION & TESTING** (1-2 hours)

### **Task 3.1: Run Full Test Suite**

```bash
# Unit tests (must pass 100%)
cd holmesgpt-api
python3 -m pytest tests/unit -v --tb=short

# Integration tests (must pass 100%)
./tests/integration/setup_workflow_catalog_integration.sh
python3 -m pytest tests/integration -v --tb=short

# E2E tests (must pass 100%)
python3 -m pytest tests/e2e -v --tb=short
```

**Acceptance Criteria**:
- ✅ 651+ tests pass (100%)
- ✅ Zero behavioral changes
- ✅ Zero lint errors
- ✅ All imports resolve correctly

---

### **Task 3.2: Code Quality Validation**

```bash
# Check imports
python3 -c "from src.extensions.incident import analyze_incident; print('✅ incident imports OK')"
python3 -c "from src.extensions.recovery import analyze_recovery; print('✅ recovery imports OK')"
python3 -c "from src.toolsets.workflow_catalog import WorkflowCatalogToolset; print('✅ workflow_catalog imports OK')"
python3 -c "from src.audit.store_factory import get_audit_store; print('✅ audit store_factory imports OK')"

# Check lint compliance
flake8 src/extensions/incident/
flake8 src/extensions/recovery/
flake8 src/toolsets/workflow_catalog/
flake8 src/audit/store_factory.py
flake8 src/constants.py

# Check type hints
mypy src/extensions/incident/
mypy src/extensions/recovery/
mypy src/toolsets/workflow_catalog/
```

---

## 📊 **EFFORT BREAKDOWN**

| Phase | Task | Effort | Risk |
|-------|------|--------|------|
| **Phase 1** | Split incident.py | 4-6 hours | LOW |
| **Phase 1** | Split recovery.py | 4-6 hours | LOW |
| **Phase 1** | Split workflow_catalog.py | 3-4 hours | LOW |
| **Phase 2** | Extract audit store factory | 30 min | MINIMAL |
| **Phase 2** | Extract constants | 1 hour | MINIMAL |
| **Phase 2** | Reduce auth nesting | 1-2 hours | LOW |
| **Phase 2** | f-string conversion | 15 min | MINIMAL |
| **Phase 3** | Test suite validation | 1-2 hours | MINIMAL |
| **TOTAL** | | **15-25 hours** | **LOW** |

---

## 🚀 **EXECUTION APPROACH**

### **Option A: Incremental (Recommended)**

1. **Day 1**: Phase 1 - Task 1.1 (split incident.py)
   - Create new modules
   - Update imports
   - Run tests
   - Commit

2. **Day 1-2**: Phase 1 - Tasks 1.2, 1.3 (split recovery.py, workflow_catalog.py)
   - Create new modules
   - Update imports
   - Run tests
   - Commit

3. **Day 2**: Phase 2 - All tasks (extract shared code)
   - Create store_factory.py, constants.py
   - Refactor auth middleware
   - Convert f-strings
   - Run tests
   - Commit

4. **Day 3**: Phase 3 - Validation
   - Full test suite (unit + integration + E2E)
   - Code quality checks
   - Final commit

### **Option B: All-at-Once**

- Complete all phases in 2-3 days
- Single final commit
- Higher risk of merge conflicts

---

## ✅ **SUCCESS CRITERIA**

1. ✅ All 651+ tests pass (100%)
2. ✅ Zero behavioral changes
3. ✅ Zero lint errors
4. ✅ All imports resolve correctly
5. ✅ Backward compatibility maintained
6. ✅ File count: 3 large files → 30 focused modules
7. ✅ Average file size: 1,500 lines → 200-400 lines
8. ✅ Technical debt reduction: 10% → <5%

---

## 🎯 **NEXT STEPS**

### **DECISION REQUIRED**:

Please choose one of the following:

1. **Approve Full Plan** - Proceed with all 3 phases (15-25 hours)
2. **Approve Phase 1 Only** - Split files first, then reassess (10-15 hours)
3. **Modify Plan** - Suggest changes before proceeding
4. **Cancel Refactoring** - Ship v1.0 as-is, refactor post-release

**Please confirm your choice before I proceed.**

---

**End of Refactoring Implementation Plan**





