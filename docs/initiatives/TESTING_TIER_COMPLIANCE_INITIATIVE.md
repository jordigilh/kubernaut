# Cross-Service Testing Tier Compliance Initiative

**Initiative ID**: INIT-TEST-001
**Status**: 📋 DRAFT - PENDING APPROVAL
**Priority**: 🔴 P0 (BLOCKING V1.0 Sign-off)
**Start Date**: December 8, 2025
**Target Completion**: December 20, 2025 (12 days)
**Coordinator**: Architecture Team

---

## 🚨 **Executive Summary**

A comprehensive audit (December 8, 2025) revealed **systemic violations** of `TESTING_GUIDELINES.md` across multiple services. These violations cause:

1. **False confidence** in test coverage (tests pass but don't validate real behavior)
2. **API contract mismatches** going undetected (Data Storage batch endpoint issue)
3. **Production failures** that would be caught by proper testing

This initiative coordinates all services to achieve **TESTING_GUIDELINES.md compliance** before V1.0 sign-off.

---

## 📊 **Current State Assessment**

### **Violations by Service**

| Service | Integration Test Violation | E2E Test Violation | Audit Integration | Status |
|---------|---------------------------|-------------------|-------------------|--------|
| **Notification** | 🔴 Uses httptest mocks | 🔴 Uses envtest | ⚠️ Workaround | Partial |
| **Remediation Orchestrator** | 🔴 Uses httptest mocks | 🔴 Empty (suite only) | 🔴 Not integrated | ⏳ Pending |
| **AIAnalysis** | 🔴 Uses httptest mocks | 🟡 Unknown | 🔴 Not integrated | ⏳ Pending |
| **Gateway** | 🟡 Unknown | 🟡 Unknown | 🔴 Not integrated | ⏳ Assessment |
| **SignalProcessing** | 🟡 Unknown | 🟡 Unknown | 🔴 Not integrated | ⏳ Assessment |
| **WorkflowExecution** | 🟡 Unknown | 🟡 Unknown | 🔴 Not integrated | ⏳ Assessment |
| **Data Storage** | 🟡 Unknown | 🟡 Unknown | N/A (is the audit store) | ⏳ Assessment |

### **Root Cause**

`TESTING_GUIDELINES.md` (Lines 83-88) mandates:

```markdown
| Test Type | Kubernetes API | Services | Infrastructure (DB, APIs) | LLM |
|-----------|---------------|----------|---------------------------|-----|
| **Unit** | Mock | Mock | Mock | Mock |
| **Integration** | envtest | **Real** (via podman-compose) | **Real** | Mock ✅ |
| **E2E** | Real (Kind) | **Real** (deployed) | **Real** | Mock ✅ |
```

**Actual Implementation** (across services):
- Integration tests use `httptest.NewServer()` mocks instead of real services
- E2E tests use `envtest` instead of Kind clusters
- No database verification in any test tier

---

## 🎯 **Initiative Objectives**

### **Primary Objectives**

| # | Objective | Success Metric | Priority |
|---|-----------|---------------|----------|
| 1 | All integration tests use real infrastructure (podman-compose) | 100% services compliant | P0 |
| 2 | All E2E tests use Kind clusters | 100% services compliant | P0 |
| 3 | All services integrate audit client | 100% (blocked on Data Storage) | P1 |
| 4 | Database verification in integration/E2E tests | All critical paths verified | P0 |

### **Deliverables**

| # | Deliverable | Owner | Due Date |
|---|-------------|-------|----------|
| 1 | Shared podman-compose.test.yml infrastructure | Architecture Team | Day 3 |
| 2 | Shared Kind cluster setup scripts | Architecture Team | Day 5 |
| 3 | Service-specific integration test updates | Each Service Team | Day 8 |
| 4 | Service-specific E2E test updates | Each Service Team | Day 10 |
| 5 | Compliance verification report | Architecture Team | Day 12 |

---

## 📅 **Timeline**

### **Phase 1: Infrastructure (Days 1-5)**

| Day | Task | Owner | Deliverable |
|-----|------|-------|-------------|
| 1 | Create shared podman-compose.test.yml | Architecture | Base infrastructure file |
| 2 | Document integration test patterns | Architecture | INTEGRATION_TEST_PATTERNS.md |
| 3 | Create shared Kind setup scripts | Architecture | scripts/e2e-kind-setup.sh |
| 4 | Document E2E test patterns | Architecture | E2E_TEST_PATTERNS.md |
| 5 | Distribute infrastructure to all teams | Architecture | PR to each service |

### **Phase 2: Service Remediation (Days 6-10)**

| Day | Task | Owner | Deliverable |
|-----|------|-------|-------------|
| 6-7 | RO: Integration + E2E test fixes | RO Team | PR with real infrastructure |
| 6-7 | Notification: E2E test fixes | Notification Team | PR with Kind cluster |
| 8-9 | AIAnalysis: Integration + E2E fixes | HAPI Team | PR with real infrastructure |
| 8-9 | Gateway: Assessment + fixes | Gateway Team | PR with real infrastructure |
| 10 | All remaining services | All Teams | PRs submitted |

### **Phase 3: Verification (Days 11-12)**

| Day | Task | Owner | Deliverable |
|-----|------|-------|-------------|
| 11 | Cross-service integration test run | Architecture | Test report |
| 12 | Compliance verification + sign-off | Architecture | Final report |

---

## 🏗️ **Shared Infrastructure**

### **podman-compose.test.yml (Shared)**

Location: `test/infrastructure/podman-compose.test.yml`

```yaml
# Shared test infrastructure for integration tests
# All services should use this file for consistent testing

version: "3.8"

services:
  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: kubernaut_test
      POSTGRES_USER: test_user
      POSTGRES_PASSWORD: test_pass
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U test_user -d kubernaut_test"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  datastorage:
    build:
      context: ../../
      dockerfile: cmd/datastorage/Dockerfile
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    ports:
      - "8082:8082"
    environment:
      DATABASE_URL: postgres://test_user:test_pass@postgres:5432/kubernaut_test
      REDIS_URL: redis://redis:6379
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8082/health"]
      interval: 10s
      timeout: 5s
      retries: 5

networks:
  default:
    name: kubernaut-test
```

### **Kind Cluster Setup Script**

Location: `scripts/e2e-kind-setup.sh`

```bash
#!/bin/bash
# E2E Kind cluster setup with kubeconfig isolation
# Per TESTING_GUIDELINES.md - NEVER overwrites ~/.kube/config

set -euo pipefail

SERVICE_NAME="${1:-default}"
CLUSTER_NAME="${SERVICE_NAME}-e2e-test"
KUBECONFIG_PATH="$HOME/.kube/${SERVICE_NAME}-e2e-config"

echo "Creating Kind cluster: $CLUSTER_NAME"
echo "Kubeconfig: $KUBECONFIG_PATH"

# Create cluster with isolated kubeconfig
kind create cluster \
    --name "$CLUSTER_NAME" \
    --kubeconfig "$KUBECONFIG_PATH" \
    --wait 5m

# Apply CRDs
kubectl apply -f config/crd/bases/ --kubeconfig "$KUBECONFIG_PATH"

echo "Kind cluster ready: $CLUSTER_NAME"
echo "Use: export KUBECONFIG=$KUBECONFIG_PATH"
```

---

## 📋 **Service Checklist**

### **Each Service Must Complete**

- [ ] **Integration Tests**
  - [ ] Replace `httptest.NewServer()` with real service connection
  - [ ] Use shared `podman-compose.test.yml`
  - [ ] Add database verification (`SELECT FROM audit_events`)
  - [ ] Verify ≥50% coverage of integration points

- [ ] **E2E Tests**
  - [ ] Replace `envtest` with Kind cluster
  - [ ] Use shared `scripts/e2e-kind-setup.sh`
  - [ ] Deploy real services to Kind
  - [ ] Verify end-to-end workflows

- [ ] **Audit Integration** (when Data Storage unblocked)
  - [ ] Add audit client to service
  - [ ] Emit audit events at key points
  - [ ] Verify events persisted to database

---

## 📊 **Compliance Tracking**

### **Service Compliance Status**

| Service | Integration | E2E | Audit | Overall | Last Updated |
|---------|-------------|-----|-------|---------|--------------|
| **Notification** | ⏳ | ⏳ | ⚠️ | 30% | Dec 8 |
| **RO** | ⏳ | ⏳ | 🚫 | 0% | Dec 8 |
| **AIAnalysis** | ⏳ | ⏳ | 🚫 | 0% | Dec 8 |
| **Gateway** | ⏳ | ⏳ | 🚫 | 0% | - |
| **SignalProcessing** | ⏳ | ⏳ | 🚫 | 0% | - |
| **WorkflowExecution** | ⏳ | ⏳ | 🚫 | 0% | - |
| **Data Storage** | ⏳ | ⏳ | N/A | 0% | - |

**Legend**:
- ✅ Compliant
- ⏳ Pending
- 🚫 Blocked
- ⚠️ Workaround in place

---

## 🚧 **Blockers**

### **Data Storage Batch Endpoint**

**Status**: 🔴 BLOCKING audit integration for all services

**Tracking**: `docs/handoff/NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md`

**Impact**: All services waiting on `POST /api/v1/audit/events/batch`

**ETA**: ~4 days (per Data Storage Team estimate)

---

## 📞 **Team Contacts**

| Team | Primary Contact | Service |
|------|-----------------|---------|
| **Architecture** | (Coordinator) | Initiative coordination |
| **RO Team** | - | Remediation Orchestrator |
| **Notification Team** | - | Notification |
| **HAPI Team** | - | AIAnalysis |
| **Gateway Team** | - | Gateway |
| **SP Team** | - | SignalProcessing |
| **WE Team** | - | WorkflowExecution |
| **DS Team** | - | Data Storage |

---

## ✅ **Acceptance Criteria**

The initiative is complete when:

1. [ ] All services pass integration tests with real infrastructure
2. [ ] All services pass E2E tests with Kind clusters
3. [ ] No `httptest.NewServer()` mocks in integration tests
4. [ ] No `envtest` usage in E2E tests
5. [ ] Database verification in all integration tests
6. [ ] Audit integration ready (pending Data Storage unblock)
7. [ ] Compliance report signed off by Architecture Team

---

## 🔗 **Related Documents**

- **Authoritative**: `docs/development/business-requirements/TESTING_GUIDELINES.md`
- **RO Implementation Plan**: `docs/services/crd-controllers/05-remediationorchestrator/implementation/RO_GAP_REMEDIATION_IMPLEMENTATION_PLAN_V1.0.md`
- **Notification Triage**: `docs/services/crd-controllers/06-notification/COMPREHENSIVE-AUDIT-TRAIL-TRIAGE-v2.md`
- **Data Storage Blocker**: `docs/handoff/NOTICE_DATASTORAGE_AUDIT_BATCH_ENDPOINT_MISSING.md`

---

**Document Status**: 📋 **DRAFT - PENDING APPROVAL**
**Last Updated**: December 8, 2025
**Maintained By**: Architecture Team / AI Assistant
**Next Action**: Await stakeholder approval to begin Phase 1


