# DD-TEST-002 Deprecation Triage

**Date**: December 27, 2025
**Status**: ✅ **TRIAGE COMPLETE**
**Action**: Consolidate valid parts into DD-INTEGRATION-001 v2.0, then fully deprecate DD-TEST-002

---

## 📊 **Triage Results**

### **Valid Content to Preserve** (1 section)

| Section | Content | Status | Action |
|---------|---------|--------|--------|
| **Python pytest Fixtures (HAPI)** | Lines 217-269 | ✅ **VALID** | Consolidate into DD-INTEGRATION-001 v2.0 |

### **Invalid/Deprecated Content** (Everything Else)

| Section | Content | Status | Reason |
|---------|---------|--------|--------|
| **Shell Script Pattern** | Lines 103-181 | ❌ **DEPRECATED** | Conflicts with DD-INTEGRATION-001 v2.0 (programmatic Go) |
| **BeforeSuite with Shell Scripts** | Lines 189-213 | ❌ **DEPRECATED** | Uses `./setup-infrastructure.sh` (forbidden) |
| **Image Tags** | Line 155, 161, 171 | ❌ **WRONG** | Uses `datastorage:latest` instead of `{service}-{uuid}` |
| **Decision Matrix** | Lines 89-97 | ❌ **OUTDATED** | Recommends `podman-compose` for some scenarios |
| **Service Migration Status** | Lines 372-381 | ❌ **OUTDATED** | All services already migrated per DD-INTEGRATION-001 v2.0 |

---

## ✅ **Valid Content: Python pytest Fixtures**

**Location**: DD-TEST-002 lines 217-269

**Why Valid**:
- ✅ Real implementation exists (`holmesgpt-api/tests/integration/conftest.py`)
- ✅ Follows correct pattern (framework manages infrastructure, no shell scripts)
- ✅ Well-documented and actively used
- ✅ Consistent with Go service pattern (BeforeSuite manages infrastructure)

**Content to Consolidate**:

```python
# tests/integration/conftest.py

def start_infrastructure() -> bool:
    """
    Start integration infrastructure using Python (no shell scripts).

    This provides:
    - Consistency with Go service patterns (framework manages infrastructure)
    - Better error handling (Python exceptions propagate to pytest)
    - Simpler maintenance (single source of truth)
    """
    script_dir = os.path.dirname(os.path.abspath(__file__))
    compose_file = os.path.join(script_dir, "docker-compose.workflow-catalog.yml")

    # Determine compose command
    compose_cmd = "podman-compose" if shutil.which("podman-compose") else "docker-compose"

    # Start services sequentially via compose
    result = subprocess.run(
        [compose_cmd, "-f", "docker-compose.yml", "-p", "project-name", "up", "-d"],
        cwd=script_dir,
        capture_output=True,
        timeout=180
    )

    if result.returncode != 0:
        return False

    # Wait for services to be healthy (60s timeout)
    return wait_for_infrastructure(timeout=60.0)

@pytest.fixture(scope="session")
def integration_infrastructure():
    """Session-scoped fixture for infrastructure management."""
    if not is_integration_infra_available():
        pytest.fail("REQUIRED: Infrastructure not running")

    yield

    # Automatic cleanup handled by pytest_sessionfinish hook
```

**Benefits**:
- ✅ No external shell scripts needed
- ✅ Python debugging works natively
- ✅ Errors propagate cleanly to pytest
- ✅ Consistent with Go service pattern (framework manages infrastructure)

---

## ❌ **Invalid Content: Shell Script Pattern**

**Location**: DD-TEST-002 lines 103-181

**Why Invalid**:
- ❌ **Conflicts with DD-INTEGRATION-001 v2.0**: Requires programmatic Go, not shell scripts
- ❌ **Wrong image tags**: Uses `datastorage:latest` instead of `{service}-{uuid}`
- ❌ **Deprecated approach**: Shell scripts forbidden per DD-INTEGRATION-001 v2.0

**Example of Wrong Pattern**:

```bash
# ❌ WRONG: Shell script with wrong image tag
podman run -d \
  --name {service}_datastorage_1 \
  --network {service}_test-network \
  -p {DS_HTTP_PORT}:8080 \
  datastorage:latest  # ❌ WRONG: Should be datastorage-{uuid}
```

**Correct Pattern** (DD-INTEGRATION-001 v2.0):

```go
// ✅ CORRECT: Programmatic Go with composite tag
dsImage := fmt.Sprintf("datastorage-%s", uuid.New().String())
buildCmd := exec.Command("podman", "build", "-t", dsImage, ...)
// ... start container with dsImage
```

---

## ❌ **Invalid Content: BeforeSuite with Shell Scripts**

**Location**: DD-TEST-002 lines 189-213

**Why Invalid**:
- ❌ **Uses shell scripts**: `cmd := exec.Command("./setup-infrastructure.sh")`
- ❌ **Forbidden per DD-INTEGRATION-001 v2.0**: Must use programmatic Go

**Example of Wrong Pattern**:

```go
// ❌ WRONG: Calling shell script from BeforeSuite
var _ = BeforeSuite(func() {
    cmd := exec.Command("./setup-infrastructure.sh")  // ❌ FORBIDDEN
    cmd.Dir = "test/integration/{service}"
    output, err := cmd.CombinedOutput()
    // ...
})
```

**Correct Pattern** (DD-INTEGRATION-001 v2.0):

```go
// ✅ CORRECT: Programmatic Go infrastructure management
var _ = SynchronizedBeforeSuite(func() []byte {
    // Call programmatic Go function (not shell script)
    err := infrastructure.Start{Service}IntegrationInfrastructure(GinkgoWriter)
    if err != nil {
        Fail(fmt.Sprintf("Failed to start infrastructure: %v", err))
    }
    return nil
}, func(data []byte) {
    // All processes wait for infrastructure
})
```

---

## 📋 **Consolidation Plan**

### **Step 1: Add Python Pattern to DD-INTEGRATION-001 v2.0**

Add new section: **"Python Services (pytest Fixtures)"**

**Content**:
- Python pytest fixtures pattern (from DD-TEST-002 lines 217-269)
- Reference to actual implementation (`holmesgpt-api/tests/integration/conftest.py`)
- Benefits of Python-only approach
- Comparison with Go BeforeSuite pattern

### **Step 2: Update DD-TEST-002 Status**

- Change status from "✅ Accepted" to "❌ **FULLY DEPRECATED**"
- Add prominent deprecation notice at top
- Add redirect to DD-INTEGRATION-001 v2.0
- Keep document for historical reference only

### **Step 3: Update Cross-References**

Files that reference DD-TEST-002:
- `holmesgpt-api/tests/integration/conftest.py` (line 5)
- Any other documents that link to DD-TEST-002

Update to reference DD-INTEGRATION-001 v2.0 instead.

---

## 🎯 **Recommended Actions**

### **Immediate Actions**

1. ✅ Add Python pytest fixtures section to DD-INTEGRATION-001 v2.0
2. ✅ Mark DD-TEST-002 as "❌ FULLY DEPRECATED"
3. ✅ Add redirect notice to DD-INTEGRATION-001 v2.0
4. ✅ Update cross-references in codebase

### **Future Actions**

1. **After 3 months** (March 27, 2026):
   - Archive DD-TEST-002 to `docs/architecture/decisions/archive/`
   - Update any remaining references

2. **Ongoing**:
   - Monitor for any new references to DD-TEST-002
   - Ensure all new services follow DD-INTEGRATION-001 v2.0

---

## 📚 **Key Takeaways**

### **What to Keep**
- ✅ Python pytest fixtures pattern (HAPI-specific, well-implemented)
- ✅ Concept of "framework manages infrastructure" (applies to both Go and Python)

### **What to Discard**
- ❌ Shell script patterns (conflicts with DD-INTEGRATION-001 v2.0)
- ❌ `podman-compose` recommendations (deprecated)
- ❌ Wrong image tag patterns (`datastorage:latest`)
- ❌ BeforeSuite with shell scripts (forbidden)

### **Single Source of Truth**
- **DD-INTEGRATION-001 v2.0**: Authoritative document for ALL integration test infrastructure
- **DD-TEST-002**: Historical reference only (fully deprecated)

---

## ✅ **Success Criteria**

This triage is successful when:
- ✅ Valid Python pattern consolidated into DD-INTEGRATION-001 v2.0
- ✅ DD-TEST-002 marked as fully deprecated
- ✅ All cross-references updated
- ✅ No conflicting guidance exists across documents

---

**Document Status**: ✅ Complete
**Next Steps**: Consolidate Python pattern, deprecate DD-TEST-002
**Timeline**: Complete by end of December 27, 2025


