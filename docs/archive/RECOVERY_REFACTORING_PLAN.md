# [Deprecated - Issue #180] Recovery.py Refactoring Plan

> **DEPRECATED**: Recovery flow removed in Issue #180. The `src/extensions/recovery/` module
> and all associated endpoints have been deleted. This plan is retained for historical reference only.

## 📊 **Current State**

- **File**: `src/extensions/recovery.py`
- **Size**: 1,704 lines
- **Status**: Well-organized with clear sections
- **Complexity**: High (recovery scenarios more complex than incident)

---

## 🎯 **Refactoring Strategy**

Split `recovery.py` into 5 focused modules following the same pattern as `incident.py`:

### **Module Structure**

```
src/extensions/recovery/
├── __init__.py           # Backward compatibility & exports
├── constants.py          # Constants & MinimalDAL class
├── prompt_builder.py     # All prompt-building functions
├── result_parser.py      # All parsing & extraction functions
├── llm_integration.py    # Holmes config & analyze_recovery
└── endpoint.py           # FastAPI router
```

---

## 📁 **Detailed Module Breakdown**

### **1. `constants.py` (~150 lines)**

**Purpose**: Constants and shared classes

**Contents**:
- `MinimalDAL` class (lines 56-118)
- Any recovery-specific constants

**Dependencies**: None (base module)

---

### **2. `prompt_builder.py` (~650 lines)**

**Purpose**: All prompt construction logic

**Functions to Extract**:
- `_get_failure_reason_guidance()` (lines 119-241)
- `_build_cluster_context_section()` (lines 242-306)
- `_build_mcp_filter_instructions()` (lines 307-391)
- `_create_recovery_investigation_prompt()` (lines 392-635)
- `_create_investigation_prompt()` (lines 740-1110)

**Dependencies**: `constants.py`

**Key Patterns**:
- Kubernetes reason code mapping
- DetectedLabels to natural language
- MCP filter instructions
- Previous execution context formatting

---

### **3. `result_parser.py` (~400 lines)**

**Purpose**: Investigation result parsing and validation

**Functions to Extract**:
- `_parse_investigation_result()` (lines 1111-1163)
- `_parse_recovery_specific_result()` (lines 1164-1236)
- `_extract_strategies_from_analysis()` (lines 1237-1308)
- `_extract_warnings_from_analysis()` (lines 1309+)

**Dependencies**: `constants.py`

**Key Patterns**:
- JSON extraction from LLM responses
- Strategy parsing
- Warning extraction
- Recovery-specific fields

---

### **4. `llm_integration.py` (~400 lines)**

**Purpose**: HolmesGPT SDK integration & main business logic

**Functions to Extract**:
- `_get_holmes_config()` (lines 636-739)
- `analyze_recovery()` (main async function)
- Audit store integration

**Dependencies**:
- `constants.py`
- `prompt_builder.py`
- `result_parser.py`
- `src.audit` (shared)

**Key Patterns**:
- Holmes SDK Config initialization
- WorkflowCatalogToolset registration
- Custom labels & detected labels handling
- Audit trail integration
- LLM API calls

---

### **5. `endpoint.py` (~60 lines)**

**Purpose**: FastAPI router definition

**Contents**:
- FastAPI router initialization
- `/recovery/analyze` endpoint
- Request/response model handling

**Dependencies**: `llm_integration.py`

---

### **6. `__init__.py` (~120 lines)**

**Purpose**: Package exports & backward compatibility

**Contents**:
- Re-export all public functions
- Re-export with `_` prefix for backward compatibility
- `__all__` list for explicit exports

**Pattern** (same as `incident/__init__.py`):
```python
from .endpoint import router, recovery_analyze_endpoint
from .llm_integration import analyze_recovery, MinimalDAL
from .constants import ...
from .prompt_builder import ...
from .result_parser import ...

# Backward compatibility
_analyze_recovery = analyze_recovery
_get_holmes_config = _get_holmes_config
# ... etc

__all__ = [
    "router",
    "analyze_recovery",
    "_analyze_recovery",  # backward compat
    # ... etc
]
```

---

## 🧪 **Testing Strategy**

### **Unit Tests Requiring Updates**

**Search Pattern**:
```bash
grep -r "from src.extensions.recovery import" tests/unit/
```

**Expected Impact**:
- Tests importing private functions (`_function_name`) will continue to work
- No test logic changes required
- Only import paths remain valid

### **Validation Steps**

1. Create all 5 modules + `__init__.py`
2. Delete original `recovery.py`
3. Run unit tests: `pytest tests/unit/ -k recovery -v`
4. Run full test suite: `pytest tests/unit/ -v`
5. Verify backward compatibility

---

## ⚡ **Implementation Steps**

### **Phase 1: Create New Modules** (2 hours)

1. ✅ Create `src/extensions/recovery/` directory
2. ⏳ Create `constants.py` with `MinimalDAL`
3. ⏳ Create `prompt_builder.py` with all prompt functions
4. ⏳ Create `result_parser.py` with all parsing functions
5. ⏳ Create `llm_integration.py` with main logic
6. ⏳ Create `endpoint.py` with FastAPI router
7. ⏳ Create `__init__.py` with exports

### **Phase 2: Update Imports** (30 minutes)

1. ⏳ Update `src/main.py` to import from package
2. ⏳ Verify no other files import from `recovery.py`

### **Phase 3: Test & Validate** (30 minutes)

1. ⏳ Run unit tests
2. ⏳ Fix any import issues
3. ⏳ Verify backward compatibility
4. ⏳ Run full test suite

### **Phase 4: Cleanup** (15 minutes)

1. ⏳ Delete original `recovery.py`
2. ⏳ Update any documentation references
3. ⏳ Final test run

---

## 📊 **Expected Benefits**

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Largest File** | 1,704 lines | ~650 lines (prompt_builder) | **Maintainability ⬆️** |
| **Module Count** | 1 monolithic | 5 focused modules | **Organization ⬆️** |
| **Function Locality** | Mixed concerns | Single Responsibility | **Clarity ⬆️** |
| **Test Impact** | N/A | 0 tests need changes | **Compatibility ✅** |

---

## 🔗 **Reference Implementation**

See `src/extensions/incident/` for complete refactoring example:
- Successfully split 1,593 lines → 5 modules
- ✅ All 58 incident tests passing
- ✅ 100% backward compatibility
- ✅ Zero breaking changes

---

## 📝 **Notes**

### **Why This is Lower Priority**

1. `recovery.py` is already well-organized with clear sections
2. Lower change frequency than `incident.py`
3. Smaller team working on recovery scenarios

### **When to Prioritize**

1. Team grows and multiple developers need to work on recovery logic
2. Adding new recovery scenarios increases complexity
3. Testing becomes harder due to file size

---

## ✅ **Approval Gate**

**Question for User**:

Given that:
- ✅ `incident.py` refactoring is complete (1,593 lines → 5 modules, all tests passing)
- ✅ Pattern is proven and documented
- ⏳ `recovery.py` is next largest file (1,704 lines)
- ⏱️ Estimated time: 3-4 hours for full implementation

**Should we proceed with `recovery.py` refactoring now?**

**Options**:
1. **Proceed now** - Complete refactoring in this session (3-4 hours)
2. **Ship v1.0 first** - Defer to post-release (recommended)
3. **Create GitHub issue** - Document for future sprint

---

**Status**: ⏸️ **AWAITING USER DECISION**

**Recommendation**: Ship v1.0 HAPI with `incident.py` refactoring complete, defer `recovery.py` to post-release maintenance window.

**Rationale**:
- Primary goal (reduce technical debt) achieved with `incident.py` refactoring
- `recovery.py` is well-organized enough for current team size
- Focus on shipping v1.0 and gathering production feedback
- Revisit based on actual maintenance pain points

---

**Created**: 2025-12-14
**Author**: Cursor AI Assistant
**Related**: `REFACTORING_COMPLETE_SUMMARY.md`





