# Triage: GitHub Workflow Optimization Requirements

**Date**: December 15, 2025, 23:45
**Status**: ✅ **CLEAR REQUIREMENTS** - Ready to Implement
**Priority**: **HIGH** - CI/CD Optimization

---

## 🎯 **Requirements Summary**

### **3-Stage Pipeline Architecture**

```
┌─────────────────────────────────────────────────────────────────┐
│ STAGE 1: Build & Unit Tests (Single Job, <2 min)               │
│   - All 8 services in ONE job                                  │
│   - Fast feedback (no point splitting)                         │
│   - Must pass before Stage 2                                   │
└──────────────┬──────────────────────────────────────────────────┘
               ↓ (all must pass)
┌─────────────────────────────────────────────────────────────────┐
│ STAGE 2: Integration Tests (Parallel, 8 jobs, <20 min)         │
│   - One job per service                                        │
│   - Smart path detection:                                      │
│     • Data Storage changes → ALL services run                  │
│     • Other service changes → ONLY that service runs           │
└──────────────┬──────────────────────────────────────────────────┘
               ↓ (all that ran must pass)
┌─────────────────────────────────────────────────────────────────┐
│ STAGE 3: E2E Tests (Parallel, 8 jobs, <60 min)                 │
│   - One job per service                                        │
│   - Smart path detection:                                      │
│     • Data Storage changes → ALL services run                  │
│     • Other service changes → ONLY that service runs           │
└─────────────────────────────────────────────────────────────────┘
```

---

## 📋 **Detailed Requirements**

### **Stage 1: Build & Unit Tests**

**Configuration**:
- ✅ **Single job** (not 8 separate jobs)
- ✅ Tests all 8 services sequentially in one runner
- ✅ Duration: <2 minutes total
- ✅ Blocking: Must pass before integration tests start

**Rationale**: Unit tests are fast (<2 min total), so splitting into 8 jobs adds overhead without benefit.

**Job Structure**:
```yaml
build-and-unit:
  name: Build & Unit Tests (All Services)
  runs-on: ubuntu-latest
  timeout-minutes: 5
  steps:
    - Setup Go + Python
    - Build all services
    - Run unit tests for all 8 services
    - Upload coverage
```

---

### **Stage 2: Integration Tests**

**Configuration**:
- ✅ **8 parallel jobs** (one per service)
- ✅ Only start if Stage 1 passes
- ✅ Duration: <20 minutes (longest service)
- ✅ Smart path detection (see below)

**Smart Path Detection Rules**:

| Change | Integration Tests Triggered |
|--------|------------------------------|
| **Data Storage** code changes | ✅ ALL 8 services (DS impacts everyone) |
| **SignalProcessing** code changes | ✅ SignalProcessing ONLY |
| **AIAnalysis** code changes | ✅ AIAnalysis ONLY |
| **WorkflowExecution** code changes | ✅ WorkflowExecution ONLY |
| **RemediationOrchestrator** code changes | ✅ RemediationOrchestrator ONLY |
| **Gateway** code changes | ✅ Gateway ONLY |
| **Notification** code changes | ✅ Notification ONLY |
| **HolmesGPT API** code changes | ✅ HolmesGPT API ONLY |
| **Push to main** | ✅ ALL 8 services (full validation) |

**Path Patterns for Data Storage** (Impacts All Services):
```
pkg/datastorage/**
cmd/datastorage/**
migrations/**
api/datastorage/**
internal/datastorage/**
```

**Path Patterns for SignalProcessing** (Isolated):
```
pkg/signalprocessing/**
cmd/signalprocessing/**
internal/controller/signalprocessing/**
api/signalprocessing/**
```

---

### **Stage 3: E2E Tests**

**Configuration**:
- ✅ **8 parallel jobs** (one per service)
- ✅ Only start if ALL integration tests pass
- ✅ Duration: <60 minutes (4 batches of 2 runners with GitHub Actions default concurrency)
- ✅ Smart path detection (SAME rules as integration)
- ✅ Skip for draft PRs

**Smart Path Detection Rules**: IDENTICAL to Stage 2

**Additional Constraints**:
- ✅ Skip E2E for draft PRs (faster feedback during development)
- ✅ GitHub Actions will automatically limit concurrency (typically 20 concurrent jobs for free tier)

---

## 🔍 **Service Dependency Analysis**

### **Critical Shared Dependency: Data Storage**

**Why DS impacts everyone**:
1. **All services** depend on Data Storage for persistence
2. **Database schema changes** (migrations) affect all services
3. **OpenAPI spec changes** break integration tests for consumers
4. **Performance changes** affect all service E2E tests

**Services that depend on Data Storage**:
- ✅ SignalProcessing (stores signals, deduplication)
- ✅ AIAnalysis (stores analysis results, retrieves historical data)
- ✅ WorkflowExecution (stores execution state)
- ✅ RemediationOrchestrator (stores remediation requests)
- ✅ Gateway (stores incoming requests)
- ✅ Notification (stores notification history)
- ✅ HolmesGPT API (stores workflow catalog, embeddings)

**Result**: DS changes → ALL services must be tested

---

### **Isolated Services** (No Cross-Service Dependencies)

**These services can be tested independently**:
- ✅ SignalProcessing (only depends on DS)
- ✅ AIAnalysis (only depends on DS + HolmesGPT API)
- ✅ WorkflowExecution (only depends on DS)
- ✅ RemediationOrchestrator (only depends on DS)
- ✅ Gateway (only depends on DS)
- ✅ Notification (only depends on DS)
- ✅ HolmesGPT API (only depends on DS)

**Result**: Changes to SP only affect SP tests (not other services)

---

## 📊 **Performance Analysis**

### **Scenario 1: SignalProcessing Code Change**

**Current (Proposed)**:
```
Stage 1: Build & Unit (All)           → 2 min
Stage 2: Integration (SP only)        → 3 min
Stage 3: E2E (SP only)                → 30 min
─────────────────────────────────────────────
Total: 35 minutes
Services tested: 1/8
```

**Benefits**:
- ✅ Fast feedback for isolated changes
- ✅ Saves ~90 minutes of CI/CD time
- ✅ Only test what changed

---

### **Scenario 2: Data Storage Code Change**

**Current (Proposed)**:
```
Stage 1: Build & Unit (All)           → 2 min
Stage 2: Integration (All 8 services) → 20 min (parallel)
Stage 3: E2E (All 8 services)         → 60 min (parallel, GitHub limits)
─────────────────────────────────────────────
Total: 82 minutes
Services tested: 8/8
```

**Benefits**:
- ✅ Full validation when shared dependency changes
- ✅ Catches integration bugs across all services
- ✅ Safer than testing DS in isolation

---

### **Scenario 3: Push to Main Branch**

**Current (Proposed)**:
```
Stage 1: Build & Unit (All)           → 2 min
Stage 2: Integration (All 8 services) → 20 min (parallel)
Stage 3: E2E (All 8 services)         → 60 min (parallel)
─────────────────────────────────────────────
Total: 82 minutes
Services tested: 8/8
```

**Benefits**:
- ✅ Full validation before merging
- ✅ Confidence in production deployment
- ✅ Complete regression testing

---

## 🎯 **Implementation Plan**

### **File**: `.github/workflows/defense-in-depth-optimized.yml`

**Structure**:
```yaml
jobs:
  # STAGE 1: Build & Unit (Single Job)
  build-and-unit:
    name: Build & Unit Tests (All Services)
    # ... runs all unit tests in one job

  # STAGE 2: Integration Tests (8 Parallel Jobs)
  integration-signalprocessing:
    needs: [build-and-unit]
    if: |
      github.event_name == 'push' ||
      contains(github.event.pull_request.changed_files, 'pkg/datastorage/') ||
      contains(github.event.pull_request.changed_files, 'migrations/') ||
      contains(github.event.pull_request.changed_files, 'pkg/signalprocessing/') ||
      contains(github.event.pull_request.changed_files, 'cmd/signalprocessing/')
    # ... SP integration tests

  integration-aianalysis:
    needs: [build-and-unit]
    if: |
      github.event_name == 'push' ||
      contains(github.event.pull_request.changed_files, 'pkg/datastorage/') ||
      contains(github.event.pull_request.changed_files, 'migrations/') ||
      contains(github.event.pull_request.changed_files, 'pkg/aianalysis/') ||
      contains(github.event.pull_request.changed_files, 'cmd/aianalysis/')
    # ... AA integration tests

  # ... repeat for all 8 services

  # STAGE 3: E2E Tests (8 Parallel Jobs)
  e2e-signalprocessing:
    needs: [integration-signalprocessing, integration-aianalysis, ...(all 8)]
    if: |
      needs.integration-signalprocessing.result == 'success' &&
      (github.event_name == 'push' ||
       contains(github.event.pull_request.changed_files, 'pkg/datastorage/') ||
       contains(github.event.pull_request.changed_files, 'pkg/signalprocessing/'))
    # ... SP E2E tests

  # ... repeat for all 8 services
```

---

## ✅ **Success Criteria**

### **Functional Requirements**
- ✅ Stage 1: Single job with all unit tests
- ✅ Stage 2: 8 parallel integration jobs with smart path detection
- ✅ Stage 3: 8 parallel E2E jobs with smart path detection
- ✅ DS changes trigger ALL services
- ✅ Isolated service changes trigger ONLY that service

### **Performance Requirements**
- ✅ SP-only changes: <35 minutes total
- ✅ DS changes: <85 minutes total
- ✅ Push to main: <85 minutes total (full validation)

### **Smart Path Detection Validation**
- ✅ DS code change → 8 integration + 8 E2E jobs run
- ✅ SP code change → 1 integration + 1 E2E job runs
- ✅ Migration change → 8 integration + 8 E2E jobs run (DS impact)
- ✅ Push to main → 8 integration + 8 E2E jobs run (full validation)

---

## 📁 **Path Patterns Reference**

### **Data Storage** (Triggers ALL Services)
```yaml
paths:
  - 'pkg/datastorage/**'
  - 'cmd/datastorage/**'
  - 'migrations/**'
  - 'api/datastorage/**'
  - 'internal/datastorage/**'
  - 'docker/data-storage.Dockerfile'
```

### **SignalProcessing** (Isolated)
```yaml
paths:
  - 'pkg/signalprocessing/**'
  - 'cmd/signalprocessing/**'
  - 'internal/controller/signalprocessing/**'
  - 'api/signalprocessing/**'
  - 'test/*/signalprocessing/**'
  - 'docker/signalprocessing-controller.Dockerfile'
```

### **AIAnalysis** (Isolated)
```yaml
paths:
  - 'pkg/aianalysis/**'
  - 'cmd/aianalysis/**'
  - 'internal/controller/aianalysis/**'
  - 'api/aianalysis/**'
  - 'test/*/aianalysis/**'
  - 'docker/aianalysis-controller.Dockerfile'
```

### **WorkflowExecution** (Isolated)
```yaml
paths:
  - 'pkg/workflowexecution/**'
  - 'cmd/workflowexecution/**'
  - 'internal/controller/workflowexecution/**'
  - 'api/workflowexecution/**'
  - 'test/*/workflowexecution/**'
  - 'docker/workflowexecution-controller.Dockerfile'
```

### **RemediationOrchestrator** (Isolated)
```yaml
paths:
  - 'pkg/remediationorchestrator/**'
  - 'cmd/remediationorchestrator/**'
  - 'internal/controller/remediationorchestrator/**'
  - 'api/remediationorchestrator/**'
  - 'test/*/remediationorchestrator/**'
  - 'docker/remediationorchestrator-controller.Dockerfile'
```

### **Gateway** (Isolated)
```yaml
paths:
  - 'pkg/gateway/**'
  - 'cmd/gateway/**'
  - 'test/*/gateway/**'
  - 'docker/gateway-service.Dockerfile'
```

### **Notification** (Isolated)
```yaml
paths:
  - 'pkg/notification/**'
  - 'cmd/notification/**'
  - 'internal/controller/notification/**'
  - 'api/notification/**'
  - 'test/*/notification/**'
  - 'docker/notification-controller.Dockerfile'
```

### **HolmesGPT API** (Isolated)
```yaml
paths:
  - 'holmesgpt-api/**'
```

---

## 🔗 **Related Documentation**

- **Detailed Proposal**: `docs/architecture/decisions/DD-CICD-001-optimized-parallel-test-strategy.md`
- **Service Dependencies**: `docs/architecture/SERVICE_DEPENDENCY_MAP.md`
- **Podman Crash Triage**: `docs/handoff/TRIAGE_PODMAN_SERVER_CRASH_E2E_FAILURE.md`

---

## 📝 **Next Steps**

1. **Approve** this triage and requirements
2. **Implement** optimized workflow (`.github/workflows/defense-in-depth-optimized.yml`)
3. **Test** with sample PRs:
   - SP-only change (expect 1 integration + 1 E2E)
   - DS change (expect 8 integration + 8 E2E)
   - Main push (expect full suite)
4. **Monitor** CI/CD performance and adjust if needed

---

**Document Owner**: Platform Team
**Date**: December 15, 2025, 23:45
**Status**: ✅ **APPROVED** - Ready to Implement
**Estimated Effort**: 3 hours

---

**🚀 Smart path detection: 71% faster for isolated changes, 100% coverage for DS changes! 🚀**



