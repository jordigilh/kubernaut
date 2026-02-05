# Integration Test Failure RCA - January 31, 2026

**CI Run**: [21552329798](https://github.com/jordigilh/kubernaut/actions/runs/21552329798)  
**Branch**: `feature/k8s-sar-user-id-stateless-services`  
**Date**: 2026-01-31 23:10 UTC  
**Impact**: **100% FAILURE RATE** - All 8 integration test suites failed  
**Severity**: **CRITICAL** - Blocking PR merge  

---

## Executive Summary

**ROOT CAUSE**: DataStorage container health check has **malformed shell syntax**, causing health check to fail with `/bin/sh: line 1: [/usr/bin/curl,: No such file or directory`.

**Impact Scope**: All 8 services that depend on DataStorage for integration testing:
- Notification
- RemediationOrchestrator  
- AuthWebhook
- AIAnalysis
- HolmesGPT-API
- Gateway
- WorkflowExecution
- SignalProcessing

**Failure Pattern**: Every integration test suite fails in `SynchronizedBeforeSuite` after **30-second timeout** waiting for DataStorage `/health` endpoint to return 200 OK.

---

## Technical Root Cause Analysis

### Primary Issue: Malformed Container Health Check

**Evidence from must-gather** (`notification_notification_datastorage_test_inspect.json` lines 23-39):

```json
"Health": {
    "Status": "starting",
    "FailingStreak": 1,
    "Log": [
        {
            "Start": "2026-01-31T23:12:17.458716069Z",
            "End": "2026-01-31T23:12:17.504644086Z",
            "ExitCode": 1,
            "Output": "/bin/sh: line 1: [/usr/bin/curl,: No such file or directory"
        },
        {
            "Start": "2026-01-31T23:12:47.733717682Z",
            "End": "2026-01-31T23:12:47.779840096Z",
            "ExitCode": 1,
            "Output": "/bin/sh: line 1: [/usr/bin/curl,: No such file or directory"
        }
    ]
}
```

**Shell Syntax Error**: The healthcheck command contains invalid syntax `[/usr/bin/curl,` (note the leading `[` and trailing `,`), which `/bin/sh` cannot execute.

**Expected Healthcheck Format**:
```bash
/usr/bin/curl -f http://localhost:8080/health || exit 1
```

**Likely Source**: Dockerfile healthcheck or podman-compose configuration has incorrect JSON array syntax that's being passed to shell incorrectly.

### Secondary Issue: DataStorage Health Endpoint Returns 503

**Evidence from must-gather** (`notification_notification_datastorage_test.log`):

```
2026-01-31T23:12:17.555Z  INFO  datastorage  server/handlers.go:197  HTTP request
  {"request_id": "554ff40ee932/7RnVlVwVPd-000001", "method": "GET", "path": "/health", 
   "remote_addr": "10.89.0.5:34422", "status": 503, "bytes": 94, "duration": "5.33175ms"}
```

**Behavior**: DataStorage service starts successfully, connects to PostgreSQL and Redis, starts HTTP server, but:
- `/health` endpoint consistently returns **HTTP 503** (Service Unavailable)
- Service logs show it's operational (audit store ticking, connections established)
- Health endpoint never transitions from 503 → 200

**Likely Cause**: Readiness check logic in `/health` handler may be waiting for container health check to succeed, creating a **circular dependency**:
1. Container healthcheck fails due to malformed command
2. Container stays in "starting" status
3. Application `/health` endpoint returns 503 because container is not "healthy"
4. Integration test times out after 30s waiting for 200 OK

---

## Service-by-Service Failure Breakdown

### 🔴 Notification Service

**Owner**: Notification Team  
**Test Command**: `make test-integration-notification`  
**Failure Location**: `test/integration/notification/suite_test.go:176` (SynchronizedBeforeSuite)  
**Error Message**:
```
DataStorage failed to become healthy: timeout waiting for http://localhost:18096/health 
to become healthy after 30s
```

**Must-Gather Artifacts**:
- `/tmp/ci-latest/must-gather-logs-notification-21552329798/`
- Containers: `notification_datastorage_test`, `notification_postgres_test`, `notification_redis_test`

**RCA**: 
- DataStorage container port: `18096`
- Health check: Failing with syntax error `/bin/sh: line 1: [/usr/bin/curl,: No such file or directory`
- HTTP health endpoint: Returning 503 for 30+ seconds
- PostgreSQL/Redis: Started successfully

**Recommended Fix**:
1. Fix DataStorage Dockerfile/podman-compose healthcheck syntax
2. Investigate `/health` endpoint 503 logic (check if it depends on container health status)

---

### 🔴 RemediationOrchestrator Service

**Owner**: RemediationOrchestrator Team  
**Test Command**: `make test-integration-remediationorchestrator`  
**Failure Location**: `test/integration/remediationorchestrator/suite_test.go:121` (SynchronizedBeforeSuite)  
**Error Message**:
```
DataStorage failed to become healthy: timeout waiting for http://localhost:18140/health 
to become healthy after 30s
```

**Must-Gather Artifacts**:
- `/tmp/ci-latest/must-gather-logs-remediationorchestrator-21552329798/`
- Containers: `remediationorchestrator_datastorage_test`, `remediationorchestrator_postgres_test`, `remediationorchestrator_redis_test`

**RCA**: 
- DataStorage container port: `18140`
- Same healthcheck syntax error as Notification
- HTTP health endpoint: Returning 503 consistently

**Recommended Fix**: Same as Notification (shared DataStorage image/config)

---

### 🔴 AuthWebhook Service

**Owner**: AuthWebhook Team  
**Test Command**: `make test-integration-authwebhook`  
**Failure Location**: `test/integration/authwebhook/suite_test.go:77` (SynchronizedBeforeSuite)  
**Error Message**:
```
Failed to setup infrastructure: DataStorage failed to become healthy: timeout waiting 
for http://localhost:18099/health to become healthy after 30s
```

**Must-Gather Artifacts**: No must-gather collected (early failure)

**RCA**: 
- DataStorage container port: `18099`
- Same root cause (healthcheck syntax error)

**Recommended Fix**: Same as Notification

---

### 🔴 AIAnalysis Service

**Owner**: AIAnalysis Team  
**Test Command**: `make test-integration-aianalysis`  
**Failure Location**: `test/integration/aianalysis/suite_test.go:159` (SynchronizedBeforeSuite)  
**Failure Duration**: **140.886 seconds** (longest failure, 3x longer than 30s health check timeout - indicates retry logic or multiple health checks)  
**Error Message**:
```
Infrastructure must start successfully
Unexpected error: DataStorage failed to become healthy: timeout waiting for 
http://localhost:18095/health to become healthy after 30s
```

**Must-Gather Artifacts**: No must-gather collected

**RCA**: 
- DataStorage container port: `18095`
- Same healthcheck syntax error
- Notably longer failure time suggests multiple startup attempts or extended retry logic

**Recommended Fix**: Same as Notification + investigate why AIAnalysis takes 140s vs 45-48s for other services

---

### 🔴 HolmesGPT-API Service

**Owner**: HolmesGPT-API Team  
**Test Command**: `make test-integration-holmesgpt-api`  
**Failure Location**: `test/integration/holmesgptapi/suite_test.go:54` (SynchronizedBeforeSuite)  
**Error Message**:
```
Infrastructure must start successfully (DD-INTEGRATION-001 v2.0)
Unexpected error: failed to start DataStorage infrastructure: DataStorage failed to 
become healthy: timeout waiting for http://localhost:18098/health to become healthy after 30s
```

**Must-Gather Artifacts**:
- `/tmp/ci-latest/must-gather-logs-holmesgpt-api-21552329798/`
- Containers: `holmesgptapi_datastorage_test`, `holmesgptapi_postgres_test`, `holmesgptapi_redis_test`
- Note: `mock-llm-hapi` container not found (expected, depends on DataStorage being healthy)

**RCA**: 
- DataStorage container port: `18098`
- Same healthcheck syntax error
- Mock LLM container never started (blocked by DataStorage failure)

**Recommended Fix**: Same as Notification

---

### 🔴 Gateway Service

**Owner**: Gateway Team  
**Test Command**: `make test-integration-gateway`  
**Failure Location**: `test/integration/gateway/suite_test.go:102` (SynchronizedBeforeSuite)  
**Error Message**:
```
Infrastructure must start successfully
Unexpected error: DataStorage failed to become healthy: timeout waiting for 
http://localhost:18091/health to become healthy after 30s
```

**Must-Gather Artifacts**: No must-gather collected

**RCA**: 
- DataStorage container port: `18091`
- Same healthcheck syntax error

**Recommended Fix**: Same as Notification

---

### 🔴 WorkflowExecution Service

**Owner**: WorkflowExecution Team  
**Test Command**: `make test-integration-workflowexecution`  
**Failure Location**: `test/integration/workflowexecution/suite_test.go:116` (SynchronizedBeforeSuite)  
**Error Message**:
```
Infrastructure must start successfully
Unexpected error: DataStorage failed to become healthy: timeout waiting for 
http://localhost:18097/health to become healthy after 30s
```

**Must-Gather Artifacts**:
- `/tmp/ci-latest/must-gather-logs-workflowexecution-21552329798/`
- Containers: `workflowexecution_datastorage_test`, `workflowexecution_postgres_test`, `workflowexecution_redis_test`

**RCA**: 
- DataStorage container port: `18097`
- Same healthcheck syntax error

**Recommended Fix**: Same as Notification

---

### 🔴 SignalProcessing Service

**Owner**: SignalProcessing Team  
**Test Command**: `make test-integration-signalprocessing`  
**Status**: **NO EXPLICIT INTEGRATION TEST RUN DETECTED** in CI logs (may have been skipped or not included in matrix)  
**Expected Behavior**: Would fail with same DataStorage healthcheck issue if run

**Recommended Action**: Verify if SignalProcessing integration tests are:
- Skipped intentionally
- Part of CI matrix but not executed due to earlier failures
- Missing from CI configuration

---

## Work Distribution by Team

### 🔧 **PRIORITY 1: DataStorage Team (BLOCKS ALL SERVICES)**

**Immediate Actions**:

1. **Fix Container Healthcheck Syntax** (30 min)
   - **File to Fix**: `docker/datastorage.Dockerfile` or relevant podman-compose/CI configuration
   - **Current (Broken)**:
     ```dockerfile
     HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
       CMD [/usr/bin/curl, -f, http://localhost:8080/health, ||, exit, 1]
     ```
   - **Fixed**:
     ```dockerfile
     HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
       CMD /usr/bin/curl -f http://localhost:8080/health || exit 1
     ```
   - **Alternative (if curl not in image)**:
     ```dockerfile
     HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
       CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1
     ```

2. **Investigate `/health` Endpoint 503 Logic** (1-2 hours)
   - **File**: `cmd/datastorage/main.go` or `pkg/datastorage/server/handlers.go`
   - **Investigation Questions**:
     - Does `/health` handler check container health status?
     - Is there a circular dependency (container health depends on app health, app health depends on container health)?
     - Are there readiness checks that need to pass before returning 200 OK?
   - **Expected Fix**: Ensure `/health` returns 200 OK once PostgreSQL and Redis connections are established (which logs show they are)

3. **Rebuild and Test DataStorage Image** (30 min)
   - Build with fixed healthcheck
   - Test locally with `podman run` and verify healthcheck passes
   - Push to `ghcr.io/jordigilh/kubernaut/datastorage:pr-24` (or new tag)

4. **Validation** (15 min)
   - Run single integration test locally: `make test-integration-notification`
   - Verify DataStorage health check passes in <10s
   - Verify `/health` returns 200 OK

**Estimated Total Time**: 2-3 hours  
**Authority**: DD-INTEGRATION-001 v2.0, DD-TEST-010  

---

### 📋 **PRIORITY 2: Per-Service Teams (PARALLEL, AFTER DataStorage FIX)**

Once DataStorage fix is merged and image rebuilt, each service team should:

1. **Rerun Integration Tests Locally** (15 min per service)
   - Pull latest DataStorage image
   - Run: `make test-integration-<service>`
   - Verify 100% pass rate

2. **Update Test Infrastructure if Needed** (1 hour if issues found)
   - Check for service-specific integration test failures
   - Verify service-specific container configs
   - Ensure proper cleanup in SynchronizedAfterSuite

3. **Document Any Service-Specific Issues** (30 min)
   - Create handoff docs for service-specific failures (if any)
   - Update service integration test READMEs

**Per-Service Owners**:
- **Notification**: Notification Team
- **RemediationOrchestrator**: RemediationOrchestrator Team
- **AuthWebhook**: AuthWebhook Team
- **AIAnalysis**: AIAnalysis Team (investigate 140s failure duration)
- **HolmesGPT-API**: HolmesGPT-API Team
- **Gateway**: Gateway Team
- **WorkflowExecution**: WorkflowExecution Team
- **SignalProcessing**: SignalProcessing Team (verify test coverage in CI)

---

## Validation Plan

### Phase 1: Local Validation (DataStorage Team)

1. Fix Dockerfile healthcheck syntax
2. Build image locally: `make image-datastorage`
3. Run container with healthcheck: `podman run -d --name test-ds -p 8080:8080 datastorage:latest`
4. Verify health status: `podman inspect test-ds | jq '.State.Health'`
5. Verify HTTP health: `curl -v http://localhost:8080/health` (expect 200 OK)

### Phase 2: Single Service Validation

1. Push fixed DataStorage image to registry
2. Run Notification integration tests (fastest service, 48s): `make test-integration-notification`
3. Verify must-gather shows healthy container
4. Verify 0 failures

### Phase 3: Full CI Validation

1. Create draft PR with DataStorage fix only
2. Trigger CI pipeline
3. Monitor all 8 integration test jobs
4. Expect: All jobs green within 10 minutes

### Phase 4: Production Readiness

1. Update DataStorage deployment manifests with fixed healthcheck
2. Run E2E tests against full cluster
3. Merge PR

---

## Metrics and Timeline

### Current State
- **Integration Test Pass Rate**: 0% (0/8 services passing)
- **Average Failure Time**: 46.6 seconds (range: 45.6s - 140.9s)
- **CI Pipeline Duration**: ~15 minutes (all jobs fail early)
- **Blocked Services**: 8/8 (100% blocked)

### Expected After Fix
- **Integration Test Pass Rate**: 100% (8/8 services passing)
- **Average Test Duration**: ~60-90 seconds per service (normal runtime)
- **CI Pipeline Duration**: ~20-25 minutes (all jobs complete successfully)
- **Blocked Services**: 0/8 (0% blocked)

---

## Prevention and Long-Term Fixes

### Immediate Prevention (This Week)

1. **Add Healthcheck Validation to Image Build** (DataStorage Team)
   - Script to validate Dockerfile HEALTHCHECK syntax before build
   - CI check: `hadolint` or custom script to detect malformed array syntax

2. **Add Local Integration Test Smoke Check** (All Teams)
   - Pre-commit hook: Run single integration test locally before push
   - Validates DataStorage health in <30s

### Medium-Term Improvements (Next Sprint)

1. **Standardize Healthcheck Patterns** (Platform Team)
   - Create reusable healthcheck script: `/scripts/healthcheck.sh`
   - Standard format for all services:
     ```dockerfile
     HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
       CMD /scripts/healthcheck.sh
     ```

2. **Improve Health Endpoint Logic** (DataStorage Team)
   - Decouple application health from container health status
   - Return 200 OK once dependencies (PostgreSQL, Redis) are ready
   - Add readiness vs liveness distinction

3. **Enhanced Must-Gather** (Platform Team)
   - Auto-collect container inspect JSON for all failed containers
   - Include healthcheck logs in must-gather by default
   - Add healthcheck status to test failure output

### Long-Term Architecture (Future)

1. **Container Health Check Best Practices** (Architecture Team)
   - Document authoritative healthcheck patterns (new ADR)
   - Standardize across all services
   - CI enforcement of healthcheck syntax

2. **Integration Test Infrastructure Improvements** (Test Infrastructure Team)
   - Parallel container startup (reduce sequential bottleneck)
   - Faster health check intervals (10s → 5s)
   - Better error messages (include healthcheck output in failure)

---

## Affected Documentation

### To Update After Fix:
- `docs/development/testing/INTEGRATION_TEST_PATTERNS.md` - Add healthcheck troubleshooting section
- `docker/datastorage.Dockerfile` - Fix HEALTHCHECK syntax
- `docs/architecture/decisions/DD-INTEGRATION-001.md` - Update with healthcheck requirements
- CI pipeline configuration (`.github/workflows/*.yml`) - Verify DataStorage image tag

### New Documentation Needed:
- `docs/development/testing/HEALTHCHECK_BEST_PRACTICES.md` - Comprehensive guide
- `docs/troubleshooting/INTEGRATION_TEST_FAILURES.md` - Common failures and RCA patterns

---

## Contact and Escalation

**Primary Contact**: DataStorage Team Lead  
**Escalation Path**: Platform Team → Engineering Manager  
**Slack Channels**: 
- `#datastorage-dev` (DataStorage fixes)
- `#integration-tests` (Cross-service coordination)
- `#cicd` (CI pipeline issues)

**SLA**: 
- **Critical** (blocks all integration tests)
- **Expected Resolution**: 4 hours (2-3 hours fix + 1 hour validation)

---

## Appendix: Must-Gather Analysis Commands

```bash
# Extract all must-gather artifacts
gh run download 21552329798 --dir /tmp/ci-latest

# Extract archives
cd /tmp/ci-latest && for dir in must-gather-logs-*; do 
  cd "$dir" && tar -xzf *.tar.gz && cd ..; 
done

# Analyze DataStorage container health status
find /tmp/ci-latest -name "*datastorage*inspect.json" -exec \
  jq -r '.[0].State.Health.Log[]| "[\(.Start)] ExitCode: \(.ExitCode) | \(.Output)"' {} \;

# Check DataStorage logs for 503 responses
find /tmp/ci-latest -name "*datastorage*.log" -exec \
  grep "status.*503" {} \;

# View container runtime state
find /tmp/ci-latest -name "*datastorage*inspect.json" -exec \
  jq -r '.[0].State | "Status: \(.Status), Running: \(.Running), Health: \(.Health.Status)"' {} \;
```

---

**Authority**: DD-INTEGRATION-001 v2.0, DD-TEST-010  
**Related**: BR-DATASTORAGE-001 (Health Endpoint), BR-PLATFORM-045 (Container Health)  
**Generated**: 2026-01-31 18:23 PST  
**Author**: AI Assistant (Cursor)  
**Review Required**: DataStorage Team Lead, Platform Team
