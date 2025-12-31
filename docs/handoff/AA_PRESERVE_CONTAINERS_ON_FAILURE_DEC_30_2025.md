# AIAnalysis Integration Tests - Preserve Containers for Debugging

**Date**: December 30, 2025
**Status**: ✅ **COMPLETE**
**Feature**: Preserve integration test containers when failures occur

---

## 🎯 **Objective**

Update AIAnalysis integration test infrastructure to **preserve containers** when tests fail, enabling log inspection and debugging without containers being automatically removed.

---

## ✅ **Implementation**

### **Changes Made**

#### **File**: `test/integration/aianalysis/suite_test.go`

**Updated `SynchronizedAfterSuite`** to check for `PRESERVE_CONTAINERS` environment variable:

```go
// Check if containers should be preserved for debugging
// Set PRESERVE_CONTAINERS=true to keep containers running after tests
// This is useful for inspecting logs when tests fail
preserveContainers := os.Getenv("PRESERVE_CONTAINERS") == "true"

if preserveContainers {
    GinkgoWriter.Println("⚠️  Tests may have failed - preserving containers for debugging")
    GinkgoWriter.Println("📋 To inspect container logs:")
    GinkgoWriter.Println("   podman logs aianalysis_hapi_1")
    GinkgoWriter.Println("   podman logs aianalysis_datastorage_1")
    GinkgoWriter.Println("   podman logs aianalysis_postgres_1")
    GinkgoWriter.Println("   podman logs aianalysis_redis_1")
    GinkgoWriter.Println("📋 To manually clean up:")
    GinkgoWriter.Println("   podman stop aianalysis_hapi_1 aianalysis_datastorage_1 aianalysis_redis_1 aianalysis_postgres_1")
    GinkgoWriter.Println("   podman rm aianalysis_hapi_1 aianalysis_datastorage_1 aianalysis_redis_1 aianalysis_postgres_1")
    GinkgoWriter.Println("   podman network rm aianalysis_test-network")
} else {
    // Normal cleanup - stop and remove containers
    infrastructure.StopAIAnalysisIntegrationInfrastructure(GinkgoWriter)
    // ... prune images ...
}
```

---

## 📖 **Usage**

### **Normal Test Run** (containers are cleaned up)
```bash
make test-integration-aianalysis
```

### **Preserve Containers for Debugging**
```bash
PRESERVE_CONTAINERS=true make test-integration-aianalysis
```

Or for specific tests:
```bash
PRESERVE_CONTAINERS=true make test-integration-aianalysis FOCUS="BR-HAPI-197"
```

### **Inspect Container Logs After Failure**

**HAPI Service Logs**:
```bash
podman logs aianalysis_hapi_1

# Follow logs in real-time
podman logs -f aianalysis_hapi_1

# Show last 100 lines
podman logs --tail 100 aianalysis_hapi_1

# Grep for errors
podman logs aianalysis_hapi_1 2>&1 | grep -E "(500|ERROR|Exception)"
```

**Data Storage Logs**:
```bash
podman logs aianalysis_datastorage_1
```

**PostgreSQL Logs**:
```bash
podman logs aianalysis_postgres_1
```

**Redis Logs**:
```bash
podman logs aianalysis_redis_1
```

### **Manual Cleanup After Debugging**

```bash
# Stop all containers
podman stop aianalysis_hapi_1 aianalysis_datastorage_1 \
  aianalysis_redis_1 aianalysis_postgres_1 aianalysis_migrations

# Remove all containers
podman rm aianalysis_hapi_1 aianalysis_datastorage_1 \
  aianalysis_redis_1 aianalysis_postgres_1 aianalysis_migrations

# Remove network
podman network rm aianalysis_test-network

# Prune images (optional)
podman image prune -f --filter "label=io.podman.compose.project=aianalysis-integration"
```

---

## 🔍 **Debugging Workflow**

### **Step 1: Run Tests with Preservation**
```bash
PRESERVE_CONTAINERS=true make test-integration-aianalysis FOCUS="BR-HAPI-197"
```

### **Step 2: Check Test Output**
The test output will show:
```
⚠️  Tests may have failed - preserving containers for debugging
📋 To inspect container logs:
   podman logs aianalysis_hapi_1
   ...
```

### **Step 3: Inspect HAPI Logs**
```bash
# Check for HTTP 500 errors
podman logs aianalysis_hapi_1 2>&1 | grep -E "(500|ERROR)"

# Check for Python exceptions
podman logs aianalysis_hapi_1 2>&1 | grep -A10 "Exception"

# Check recovery endpoint logs
podman logs aianalysis_hapi_1 2>&1 | grep "recovery"
```

### **Step 4: Test HAPI Directly** (while container is still running)
```bash
# Health check
curl http://localhost:18120/health

# Test recovery endpoint
curl -X POST http://localhost:18120/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "incident_id": "test",
    "remediation_id": "test",
    "signal_type": "MOCK_NO_WORKFLOW_FOUND",
    "is_recovery_attempt": true,
    "recovery_attempt_number": 1
  }' | jq '.'
```

### **Step 5: Clean Up**
```bash
podman stop aianalysis_hapi_1 aianalysis_datastorage_1 \
  aianalysis_redis_1 aianalysis_postgres_1
podman rm aianalysis_hapi_1 aianalysis_datastorage_1 \
  aianalysis_redis_1 aianalysis_postgres_1
podman network rm aianalysis_test-network
```

---

## 🎓 **Benefits**

### **Before** (containers auto-removed)
❌ Tests fail → containers removed → logs lost → hard to debug

### **After** (containers preserved)
✅ Tests fail → containers preserved → inspect logs → identify root cause → fix issue

### **Specific Use Cases**
1. **HAPI HTTP 500 Errors**: Inspect Python exceptions in HAPI logs
2. **Database Issues**: Check PostgreSQL logs for connection/query errors
3. **Redis Problems**: Verify Redis connectivity and commands
4. **Network Issues**: Test direct HTTP calls to services while containers run

---

## 📋 **Example: Debugging HAPI HTTP 500**

```bash
# Run tests with preservation
PRESERVE_CONTAINERS=true make test-integration-aianalysis FOCUS="BR-HAPI-197"

# Tests fail - containers are preserved

# Check HAPI logs for the error
podman logs aianalysis_hapi_1 2>&1 | grep -B5 -A10 "500"

# Example output might show:
# ERROR: AttributeError: 'NoneType' object has no attribute 'get'
# File "/app/src/extensions/recovery/endpoint.py", line 123
# This helps identify the exact Python error causing HTTP 500

# Test the endpoint directly to reproduce
curl -v -X POST http://localhost:18120/api/v1/recovery/analyze \
  -H "Content-Type: application/json" \
  -d '{"incident_id":"test","remediation_id":"test","signal_type":"MOCK_NO_WORKFLOW_FOUND"}'

# Clean up when done
podman stop aianalysis_hapi_1 aianalysis_datastorage_1 aianalysis_redis_1 aianalysis_postgres_1
podman rm aianalysis_hapi_1 aianalysis_datastorage_1 aianalysis_redis_1 aianalysis_postgres_1
podman network rm aianalysis_test-network
```

---

## 🔗 **Related Documents**

- **HAPI HTTP 500 Investigation**: `docs/handoff/AA_INTEGRATION_TEST_HAPI_500_ERROR_DEC_30_2025.md`
- **Integration Test Migration**: `docs/handoff/AA_INTEGRATION_TESTS_REAL_HAPI_DEC_30_2025.md`
- **Testing Strategy**: `.cursor/rules/03-testing-strategy.mdc`

---

**Status**: ✅ Complete and ready to use
**Next Action**: Use `PRESERVE_CONTAINERS=true` to debug HAPI HTTP 500 errors

