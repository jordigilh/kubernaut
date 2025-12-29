# DD-TEST-002 v1.1 Update: Python Service Implementation Guidance

**Date**: December 23, 2025
**Status**: ✅ **COMPLETE**
**Authority**: DD-TEST-002 (Integration Test Container Orchestration Pattern)
**Impact**: Python services can now implement DD-TEST-002 in their native language

---

## 🎯 **Executive Summary**

Updated **DD-TEST-002** (Integration Test Container Orchestration Pattern) to **version 1.1** with comprehensive guidance for **Python service implementations**.

### **Key Changes**

1. ✅ Added **language-agnostic principle** clarification
2. ✅ Added **Python implementation pattern** with complete code examples
3. ✅ Updated **service migration status** to include HAPI (Python)
4. ✅ Added **reference implementations by language** (Go, Python, Shell)
5. ✅ Updated **working implementations** with Python examples

---

## 📋 **What Changed in DD-TEST-002 v1.1**

### **New Section: "Python Service Implementation"**

Added comprehensive section (lines 200-330) covering:

1. **Language-Agnostic Principle**
   - DD-TEST-002 mandates **sequential startup**, not specific tooling
   - Go, Python, and Shell implementations are all DD-TEST-002 compliant

2. **Python Implementation Pattern**
   - Complete `ContainerOrchestrator` class using `subprocess.run()`
   - Sequential startup: Cleanup → Network → PostgreSQL (wait) → Redis (wait) → DataStorage (wait)
   - Explicit health checks using `pg_isready`, `redis-cli ping`, HTTP endpoints

3. **Pytest Integration**
   - Session-scoped fixture for automatic infrastructure startup
   - Clean integration with pytest lifecycle

4. **Why Python Implementation is Preferred**
   - Comparison table: Cross-language (Go CLI) vs. Pure Python
   - Python-native workflow, single-language debugging, self-contained

5. **Reference Implementations by Language**
   - Go: `exec.Command()` pattern
   - Python: `subprocess.run()` pattern
   - Shell: bash script pattern

### **Updated: Service Migration Status**

**Before (v1.0)**:
```
| Service | Status | Date | Notes |
| AIAnalysis | ⚠️ VIOLATION | - | Must migrate |
```

**After (v1.1)**:
```
| Service | Language | Status | Date | Implementation Pattern | Notes |
| HolmesGPT-API (HAPI) | 🐍 Python | 🔄 Planned | 2025-12-23 | Sequential Python (subprocess.run) | Reference Python implementation |
| AIAnalysis | Go | ⚠️ VIOLATION | - | ❌ podman-compose | Must migrate |
```

### **Updated: Affected Services**

**Before (v1.0)**:
- Listed only Go services
- No Python service guidance

**After (v1.1)**:
- 🐍 **HolmesGPT-API (HAPI)**: Python implementation (planned Dec 23, 2025)
- All 6 Go services listed as migrated
- AIAnalysis marked as violation

### **Updated: Working Implementations**

**Added**:
```markdown
#### Python Services
- **HolmesGPT-API (HAPI)**: `holmesgpt-api/tests/integration/infrastructure.py` (Python subprocess.run pattern)
- **Reference**: `docs/handoff/HAPI_DD_TEST_002_PURE_PYTHON_SOLUTION_DEC_23_2025.md` (full implementation guide)
```

### **Updated: References**

**Added**:
- **Python Implementation Guide**: `docs/handoff/HAPI_DD_TEST_002_PURE_PYTHON_SOLUTION_DEC_23_2025.md` (HAPI reference)

### **Updated: Document Metadata**

**Version**: 1.0 → **1.1**
**Deciders**: DataStorage Team, Infrastructure Team → **+ HAPI Team**
**Last Reviewed**: 2025-12-21 → **2025-12-23**
**Document Status**: ✅ Authoritative (Version 1.1)

### **Added: Changelog**

```markdown
| Date | Version | Changes | Author |
| 2025-12-23 | 1.1 | Added Python service implementation guidance | HAPI Team |
| 2025-12-21 | 1.0 | Initial DD-TEST-002 creation | DataStorage Team |
```

---

## 🐍 **Python Implementation Highlights**

### **Core Principle**

```python
# DD-TEST-002 in Python (same pattern as Go, different syntax)

# Go equivalent: exec.Command("podman", "run", ...)
subprocess.run(["podman", "run", ...], check=True)

# Go equivalent: exec.Command("podman", "exec", ...).Run()
subprocess.run(["podman", "exec", ...], capture_output=True)

# Go equivalent: http.Get(url)
urllib.request.urlopen(health_url, timeout=1)
```

### **Sequential Startup Pattern (Python)**

```python
class ContainerOrchestrator:
    def start_all(self):
        # Same sequence as Go services
        self.cleanup_containers()
        self.create_network()

        # PostgreSQL → WAIT (critical!)
        self.start_postgres()
        self.wait_for_postgres(timeout=30)

        # Redis → WAIT
        self.start_redis()
        self.wait_for_redis(timeout=10)

        # DataStorage → WAIT
        self.start_datastorage()
        self.wait_for_datastorage(timeout=30)
```

### **Why This Matters**

| Before v1.1 | After v1.1 |
|-------------|------------|
| ❌ Python services unclear how to comply | ✅ Python services have clear implementation path |
| ❌ Cross-language complexity (Go CLI) | ✅ Python-native solution documented |
| ⚠️ Exception documentation considered | ✅ No exception needed - full compliance possible |
| ❌ HAPI seen as special case | ✅ HAPI is reference Python implementation |

---

## 📊 **Impact on Services**

### **Go Services** (No Change)

DD-TEST-002 v1.1 does not change Go service requirements:
- ✅ DataStorage, Gateway, WorkflowExecution, Notification, RemediationOrchestrator, SignalProcessing
- Continue using `exec.Command()` pattern
- All existing implementations remain valid

### **Python Services** (NEW GUIDANCE)

DD-TEST-002 v1.1 provides **first-class Python support**:
- ✅ **HolmesGPT-API (HAPI)**: Reference implementation
- ✅ Use `subprocess.run()` instead of `exec.Command()`
- ✅ Pytest fixtures instead of Ginkgo BeforeSuite
- ✅ Same sequential pattern, Python syntax

### **Future Python Services**

Any future Python service with multi-service dependencies:
- ✅ Follow HAPI reference implementation
- ✅ Use `subprocess.run(["podman", "run", ...])` pattern
- ✅ Implement session-scoped pytest fixtures
- ✅ 100% DD-TEST-002 compliant without cross-language tools

---

## ✅ **Compliance Checklist**

### **For Go Services** (Existing Pattern)

- [x] Use `exec.Command("podman", "run", ...)`
- [x] Sequential startup with explicit waits
- [x] Health checks before next service
- [x] BeforeSuite integration with Ginkgo

### **For Python Services** (NEW Pattern)

- [x] Use `subprocess.run(["podman", "run", ...], check=True)`
- [x] Sequential startup with explicit waits
- [x] Health checks before next service (`pg_isready`, HTTP endpoints)
- [x] Session-scoped pytest fixture integration

**Both patterns are DD-TEST-002 compliant** - different syntax, same sequential principle.

---

## 📚 **Key References**

### **Authoritative Document**

- **DD-TEST-002 v1.1**: `docs/architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md`

### **Implementation Guides**

- **Python**: `docs/handoff/HAPI_DD_TEST_002_PURE_PYTHON_SOLUTION_DEC_23_2025.md` (300 lines Python code)
- **Go**: `test/infrastructure/datastorage_bootstrap.go` (Go reference)
- **Shell**: `test/integration/workflowexecution/setup-infrastructure.sh` (bash reference)

### **Service Implementations**

- **Python**: `holmesgpt-api/tests/integration/infrastructure.py` (HAPI reference, planned)
- **Go**: `test/infrastructure/datastorage_bootstrap.go` (DataStorage reference)
- **Go**: `test/infrastructure/gateway.go` (Gateway reference)

---

## 🎯 **Success Criteria**

### **Documentation**

- [x] DD-TEST-002 updated to version 1.1
- [x] Python implementation pattern documented
- [x] HAPI listed as reference Python implementation
- [x] Language-agnostic principle clarified
- [x] Changelog added

### **Guidance Clarity**

- [x] Python services understand how to implement DD-TEST-002
- [x] No cross-language complexity required
- [x] Clear comparison: Go vs. Python implementations
- [x] Complete code examples provided

### **Service Readiness**

- [ ] HAPI implements Python pattern (pending 1 day implementation)
- [x] Go services continue with existing pattern (no changes)
- [ ] AIAnalysis migrates from podman-compose (pending)

---

## 📝 **Files Modified**

### **Authoritative Document**

| File | Changes | Status |
|------|---------|--------|
| `docs/architecture/decisions/DD-TEST-002-integration-test-container-orchestration.md` | Updated to v1.1 with Python guidance | ✅ Complete |

### **Supporting Documents**

| File | Purpose | Status |
|------|---------|--------|
| `docs/handoff/HAPI_DD_TEST_002_PURE_PYTHON_SOLUTION_DEC_23_2025.md` | Python implementation guide (300 lines) | ✅ Complete |
| `docs/handoff/HAPI_RESPONSE_DD_TEST_002_TRIAGE_DEC_23_2025.md` | HAPI team response with Option E | ✅ Complete |
| `docs/handoff/HAPI_INTEGRATION_TEST_TRIAGE_DEC_23_2025.md` | Updated with Option E recommendation | ✅ Complete |
| `docs/handoff/DD_TEST_002_V1_1_PYTHON_SERVICE_UPDATE_DEC_23_2025.md` | This summary document | ✅ Complete |

---

## 🔗 **Cross-References**

### **Related Design Decisions**

- **DD-TEST-001**: Port allocation strategy (HAPI ports: 15435, 16381, 18094, 19095, 18001)
- **DD-TEST-007**: E2E coverage capture standard
- **DD-TEST-008**: Reusable E2E coverage infrastructure

### **Related Handoff Documents**

- **HAPI Integration Test Triage**: Gateway team's original notification
- **HAPI DD-TEST-002 Pure Python Solution**: Complete implementation (Option E)
- **HAPI Response**: Decision to use Option E (Pure Python)

---

## 🎉 **Summary**

DD-TEST-002 v1.1 establishes **Python as a first-class implementation language** for the sequential container orchestration pattern. Python services like HAPI can now achieve 100% DD-TEST-002 compliance using native Python tools (`subprocess.run`), without requiring cross-language Go infrastructure or exception documentation.

**Result**: **Language-agnostic DD-TEST-002 compliance** - Go, Python, and Shell implementations all supported equally.

---

**Created**: December 23, 2025
**Author**: HAPI Team
**Status**: ✅ **COMPLETE - DD-TEST-002 v1.1 Published**





