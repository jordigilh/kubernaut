# Mock LLM Migration - Phase 6 Validation Execution Plan

**Date**: 2026-01-11
**Status**: ✅ **INFRASTRUCTURE READY** - All code changes complete
**Related**: MOCK_LLM_MIGRATION_PLAN.md v1.6.0

---

## 📊 **Current Status**

### **✅ COMPLETED**
- Phase 1-5: Mock LLM extracted, containerized, and integrated
- HAPI unit tests: 557/557 passing ✅
- Mock LLM Docker image: Built and tested ✅
- DD-TEST-004 compliance: Infrastructure updated with unique image tags ✅
- HAPI E2E fixtures: Updated to use standalone Mock LLM ✅
- AIAnalysis integration suite: Updated with Mock LLM lifecycle ✅
- 3 HAPI E2E tests: Enabled (previously skipped) ✅

### **⏳ REMAINING**
- HAPI integration tests (65 tests)
- HAPI E2E tests (61 tests including 3 newly enabled)
- AIAnalysis integration tests
- AIAnalysis E2E tests

---

## 🚀 **Execution Plan**

### **Phase 6.4: HAPI Integration Tests**

**Command**:
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make test-integration-holmesgpt-api
```

**What This Does**:
1. Builds Mock LLM image with unique tag: `localhost/mock-llm:hapi-{uuid}`
2. Starts Mock LLM container on `localhost:18140`
3. Starts DataStorage, PostgreSQL, Redis containers
4. Runs 65 HAPI integration tests
5. Stops all containers (cleanup)

**Expected Result**: 65/65 tests passing

**Infrastructure Required**:
- ✅ Mock LLM infrastructure (`test/infrastructure/mock_llm.go`) - READY
- ✅ Unique image tag generation (DD-TEST-004) - READY
- ✅ Port 18140 allocated (DD-TEST-001 v2.5) - READY

**Estimated Time**: 3-5 minutes

---

### **Phase 6.5: HAPI E2E Tests**

**Prerequisites**:
1. Kind cluster must be running
2. Mock LLM must be deployed to Kind

**Setup Commands**:
```bash
# 1. Create Kind cluster (if not exists)
kind create cluster --name kubernaut-test

# 2. Build Mock LLM image for E2E
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
podman build -t localhost/mock-llm:e2e -f test/services/mock-llm/Dockerfile .

# 3. Load Mock LLM image to Kind
kind load docker-image localhost/mock-llm:e2e --name kubernaut-test

# 4. Deploy Mock LLM to Kind (ClusterIP in kubernaut-system)
kubectl apply -k deploy/mock-llm/

# 5. Wait for Mock LLM ready
kubectl wait --for=condition=ready pod -l app=mock-llm -n kubernaut-system --timeout=60s

# 6. Verify Mock LLM accessible
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -n kubernaut-system -- \
  curl http://mock-llm:8080/health
```

**Test Command**:
```bash
make test-e2e-holmesgpt-api
```

**Expected Result**: 61/61 tests passing (58 existing + 3 newly enabled)

**Critical Tests** (Newly Enabled):
1. `test_incident_analysis_calls_workflow_search_tool`
2. `test_incident_with_detected_labels_passes_to_tool`
3. `test_recovery_analysis_calls_workflow_search_tool`

**Estimated Time**: 5-10 minutes

---

### **Phase 6.6: AIAnalysis Integration Tests**

**Command**:
```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make test-integration-aianalysis
```

**What This Does**:
1. Builds Mock LLM image with unique tag: `localhost/mock-llm:aianalysis-{uuid}`
2. Starts Mock LLM container on `localhost:18141` (different port than HAPI)
3. Starts HAPI, DataStorage, PostgreSQL, Redis containers
4. Runs AIAnalysis integration tests
5. Stops all containers (cleanup)

**Expected Result**: All tests passing

**Infrastructure Required**:
- ✅ Mock LLM infrastructure (`test/infrastructure/mock_llm.go`) - READY
- ✅ AIAnalysis integration suite updated - READY
- ✅ Port 18141 allocated (DD-TEST-001 v2.5) - READY

**Estimated Time**: 3-5 minutes

---

### **Phase 6.7: AIAnalysis E2E Tests**

**Prerequisites**:
- Kind cluster running (same as HAPI E2E)
- Mock LLM deployed to Kind (shared with HAPI E2E)

**Test Command**:
```bash
make test-e2e-aianalysis
```

**Expected Result**: All tests passing

**Estimated Time**: 5-10 minutes

---

## 🎯 **Validation Checklist**

### **Before Running Tests**

- [ ] All code changes committed
- [ ] podman is running
- [ ] Ports 18140, 18141 are available
- [ ] No stale Mock LLM containers running

**Check Commands**:
```bash
# Check podman status
podman ps

# Check port availability
lsof -i :18140
lsof -i :18141

# Clean up stale containers
podman ps -a | grep mock-llm
podman rm -f mock-llm-hapi mock-llm-aianalysis
```

### **Success Criteria**

Phase 6 is **COMPLETE** when:

- [x] HAPI Unit: 557/557 passing ✅
- [ ] HAPI Integration: 65/65 passing
- [ ] HAPI E2E: 61/61 passing (including 3 newly enabled)
- [ ] AIAnalysis Integration: 100% passing
- [ ] AIAnalysis E2E: 100% passing
- [ ] Zero test regressions
- [ ] All newly enabled tests passing

---

## 🚨 **Troubleshooting**

### **Mock LLM Container Won't Start**

**Symptom**: `failed to start Mock LLM container`

**Solution**:
```bash
# Check if port is already in use
lsof -i :18140  # or :18141

# Kill process using the port
kill -9 <PID>

# Or remove stale container
podman rm -f mock-llm-hapi
```

### **Mock LLM Health Check Fails**

**Symptom**: `Mock LLM did not become healthy after 30 seconds`

**Solution**:
```bash
# Check container logs
podman logs mock-llm-hapi

# Verify Mock LLM is running
podman ps | grep mock-llm

# Test health endpoint manually
curl http://127.0.0.1:18140/health
```

### **Image Tag Mismatch**

**Symptom**: `image not found: localhost/mock-llm:hapi-{uuid}`

**Solution**:
The image tag is generated dynamically by `GenerateInfraImageName()`. The test infrastructure will build the image automatically with the correct tag.

### **E2E Tests Can't Find Mock LLM**

**Symptom**: `Mock LLM service not ready at http://mock-llm:8080`

**Solution**:
```bash
# Verify Mock LLM pod is running
kubectl get pods -n kubernaut-system -l app=mock-llm

# Check service
kubectl get svc -n kubernaut-system mock-llm

# Test from inside cluster
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -n kubernaut-system -- \
  curl http://mock-llm:8080/health

# Check logs
kubectl logs -n kubernaut-system -l app=mock-llm
```

---

## 📝 **Test Results Tracking**

### **Create Results Directory**:
```bash
mkdir -p /tmp/mock-llm-validation-results
cd /tmp/mock-llm-validation-results
```

### **Capture Test Results**:
```bash
# HAPI Integration
make test-integration-holmesgpt-api 2>&1 | tee hapi-integration-results.log

# HAPI E2E
make test-e2e-holmesgpt-api 2>&1 | tee hapi-e2e-results.log

# AIAnalysis Integration
make test-integration-aianalysis 2>&1 | tee aianalysis-integration-results.log

# AIAnalysis E2E
make test-e2e-aianalysis 2>&1 | tee aianalysis-e2e-results.log
```

### **Extract Test Counts**:
```bash
# HAPI Integration
grep "passed" hapi-integration-results.log | tail -1

# HAPI E2E
grep "passed\|failed" hapi-e2e-results.log | tail -1

# AIAnalysis Integration
grep "passed" aianalysis-integration-results.log | tail -1

# AIAnalysis E2E
grep "passed\|failed" aianalysis-e2e-results.log | tail -1
```

---

## ✅ **Phase 7 Blocker Status**

**Phase 7 (Cleanup)** is **BLOCKED** until Phase 6 validation passes 100%.

**Rationale**: Cannot remove business code (`holmesgpt-api/src/mock_responses.py` - 900 lines) until all tests validate the standalone Mock LLM works correctly.

**Estimated Cleanup Time**: 1-2 hours (after Phase 6 passes)

---

## 🎯 **Quick Execution (All Tiers)**

**Run all validation tests in sequence**:
```bash
#!/bin/bash
set -e  # Exit on first failure

echo "🚀 Starting Phase 6 Validation..."
echo ""

# Phase 6.4: HAPI Integration
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 6.4: HAPI Integration Tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
make test-integration-holmesgpt-api

# Phase 6.5: HAPI E2E
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 6.5: HAPI E2E Tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
make test-e2e-holmesgpt-api

# Phase 6.6: AIAnalysis Integration
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 6.6: AIAnalysis Integration Tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
make test-integration-aianalysis

# Phase 6.7: AIAnalysis E2E
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Phase 6.7: AIAnalysis E2E Tests"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
make test-e2e-aianalysis

echo ""
echo "✅ Phase 6 Validation COMPLETE!"
echo "Ready to proceed with Phase 7 (Cleanup)"
```

**Save as**: `scripts/validate-mock-llm-migration.sh`

---

## 📊 **Expected Timeline**

| Phase | Task | Time | Total |
|-------|------|------|-------|
| ✅ 6.3 | HAPI Unit | 19s | 19s |
| ⏳ 6.4 | HAPI Integration | 3-5 min | 3-5 min |
| ⏳ 6.5 | HAPI E2E | 5-10 min | 8-15 min |
| ⏳ 6.6 | AIAnalysis Integration | 3-5 min | 11-20 min |
| ⏳ 6.7 | AIAnalysis E2E | 5-10 min | 16-30 min |

**Total Estimated Time**: **16-30 minutes**

---

## 🎉 **Success Criteria Met = Ready for Phase 7**

Once all Phase 6 validation passes:
1. Commit all test results
2. Update validation document with final counts
3. Proceed to Phase 7: Cleanup Business Code

**Phase 7 Tasks**:
- Remove `holmesgpt-api/src/mock_responses.py` (900 lines)
- Remove mock mode checks from `incident/llm_integration.py`
- Remove mock mode checks from `recovery/llm_integration.py`
- Final validation run

**Phase 7 Estimated Time**: 1-2 hours

---

## 📚 **References**

- **MOCK_LLM_MIGRATION_PLAN.md v1.6.0**: Migration plan
- **DD-TEST-004**: Unique resource naming strategy
- **DD-TEST-001 v2.5**: Port allocation strategy
- **MOCK_LLM_DD_TEST_004_COMPLIANCE.md**: Image tag compliance

---

**Document Version**: 1.0
**Status**: ✅ **READY FOR EXECUTION**
**Next Action**: Run Phase 6.4 (HAPI Integration Tests)
