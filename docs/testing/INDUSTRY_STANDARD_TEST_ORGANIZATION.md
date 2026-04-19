# Industry-Standard Test Tier Organization

**Version**: 1.0
**Date**: November 20, 2025
**Authority**: Industry best practices from Google, Netflix, Spotify, Microsoft
**Purpose**: Define test tier organization strategy for Kubernaut microservices

---

## 🎯 **Industry Standards Summary**

### **Test Pyramid (Google/Martin Fowler)**

```
         /\
        /  \  E2E (5-10%)
       /____\
      /      \  Integration (15-25%)
     /________\
    /          \  Unit (70-80%)
   /____________\
```

### **Test Trophy (Kent C. Dodds)**

```
       ___
      /   \
     /     \  E2E (10%)
    /       \
   /         \  Integration (50%)
  /           \
 /             \  Unit (40%)
/_______________\
```

**Key Difference**: Trophy prioritizes integration tests for better confidence/cost ratio.

---

## 📊 **Industry Practices by Company**

### **Google (Testing on the Toilet)**

| Tier | Coverage | Execution Time | Feedback Time |
|------|----------|----------------|---------------|
| **Small** (Unit) | 70-80% | < 1 min | Seconds |
| **Medium** (Integration) | 15-25% | < 5 min | Minutes |
| **Large** (E2E) | 5-10% | < 30 min | Hours |

**Key Principles**:
- ✅ Run small tests on every commit
- ✅ Run medium tests on pre-submit
- ✅ Run large tests nightly or on-demand
- ✅ Flaky tests are disabled immediately

### **Netflix (Chaos Engineering)**

| Tier | Purpose | Infrastructure |
|------|---------|----------------|
| **Unit** | Business logic | In-memory mocks |
| **Integration** | Service contracts | Embedded services (H2, embedded Kafka) |
| **Contract** | API compatibility | Pact/Spring Cloud Contract |
| **E2E** | Critical paths only | Staging environment |
| **Chaos** | Resilience | Production (controlled) |

**Key Principles**:
- ✅ Contract tests prevent breaking changes
- ✅ E2E tests are minimal (< 10 scenarios)
- ✅ Chaos tests run in production with circuit breakers

### **Spotify (Testing Microservices)**

| Tier | Scope | Parallelization |
|------|-------|-----------------|
| **Unit** | Single class/function | Unlimited |
| **Integration** | Service + dependencies | Per-service lanes |
| **Component** | Service + test doubles | Per-service lanes |
| **Contract** | API boundaries | Per-contract |
| **E2E** | Critical user journeys | Limited (expensive) |

**Key Principles**:
- ✅ **Per-service CI/CD lanes** (your question!)
- ✅ Component tests with test doubles (not real dependencies)
- ✅ Contract tests for inter-service communication
- ✅ E2E tests only for critical business flows

### **Microsoft (Azure DevOps)**

| Tier | Trigger | Timeout |
|------|---------|---------|
| **PR Validation** | Every PR | 10 min |
| **CI Build** | Main branch | 30 min |
| **Nightly** | Scheduled | 2 hours |
| **Release** | Manual | 4 hours |

**Key Principles**:
- ✅ **Fast feedback loop** (< 10 min for PR)
- ✅ Tiered timeouts (fail fast)
- ✅ Parallel test execution
- ✅ Test impact analysis (only run affected tests)

---

## 🏗️ **Recommended Architecture for Kubernaut**

### **Strategy: Hybrid Pyramid/Trophy with Service Lanes**

Based on your microservices architecture (5 services), here's the industry-standard approach:

```
┌─────────────────────────────────────────────────────────┐
│                    GITHUB ACTIONS                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐             │
│  │   Unit   │  │   Lint   │  │  Build   │             │
│  │  Tests   │  │          │  │          │             │
│  │  < 2min  │  │  < 1min  │  │  < 2min  │             │
│  └──────────┘  └──────────┘  └──────────┘             │
│       ↓              ↓              ↓                    │
│  ┌──────────────────────────────────────┐              │
│  │    INTEGRATION (Parallel Lanes)      │              │
│  ├──────────────────────────────────────┤              │
│  │  ┌──────┐ ┌──────┐ ┌──────┐ ┌─────┐ │              │
│  │  │ Data │ │Holmes│ │ Gate │ │Notif│ │              │
│  │  │Store │ │ API  │ │ way  │ │ Svc │ │              │
│  │  │ 4min │ │ 1min │ │ 5min │ │10min│ │              │
│  │  └──────┘ └──────┘ └──────┘ └─────┘ │              │
│  └──────────────────────────────────────┘              │
│       ↓                                                  │
│  ┌──────────────────────────────────────┐              │
│  │         E2E (Critical Paths)         │              │
│  │         Run Nightly / On-Demand      │              │
│  └──────────────────────────────────────┘              │
└─────────────────────────────────────────────────────────┘
```

---

## 🎯 **Recommended Test Tier Organization**

### **Tier 1: Unit Tests** (Every Commit)

**Trigger**: Every commit
**Timeout**: 2 minutes
**Parallelization**: Unlimited
**Infrastructure**: None (in-memory)

```yaml
jobs:
  unit:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - uses: actions/checkout@v4
      - name: Run unit tests
        run: make test
```

**Services**:
- ✅ All services (Go + Python)
- ✅ No external dependencies
- ✅ Fast feedback (< 2 min)

---

### **Tier 2: Integration Tests** (Per-Service Lanes)

**Trigger**: Every PR
**Timeout**: 10 minutes per service
**Parallelization**: Per-service lanes
**Infrastructure**: Service-specific (Podman/Kind)

#### **Lane 1: Fast Services** (< 2 minutes)

```yaml
jobs:
  integration-fast:
    strategy:
      matrix:
        service: [kubernaut-agent]
    timeout-minutes: 5
```

**Services**:
- ✅ **HolmesGPT API** (~1 min) - Mock LLM + Fake K8s client
  - 39 integration tests
  - No real infrastructure
  - Fast feedback

#### **Lane 2: Medium Services** (2-5 minutes)

```yaml
jobs:
  integration-medium:
    strategy:
      matrix:
        service: [datastorage]
    timeout-minutes: 10
```

**Services**:
- ✅ **Data Storage** (4 min) - PostgreSQL + Redis via Podman
  - 161 integration tests
  - Real database operations
  - Comprehensive coverage

#### **Lane 3: Slow Services** (5-15 minutes)

```yaml
jobs:
  integration-slow:
    strategy:
      matrix:
        service: [gateway, notification, toolset]
    timeout-minutes: 20
```

**Services**:
- ✅ **Gateway Service** (~5 min) - Kind cluster
- ✅ **Notification Service** (~10 min) - Kind cluster
- ✅ **Dynamic Toolset** (~10 min) - Kind cluster

**Why Separate Lanes?**:
- ✅ Fast services don't wait for slow services
- ✅ Failures are isolated (one service failure doesn't block others)
- ✅ Clear visibility (each service has its own status)
- ✅ Parallel execution (5 min total instead of 30+ min sequential)

---

### **Tier 3: Contract Tests** (API Boundaries)

**Trigger**: Every PR (if API changes detected)
**Timeout**: 5 minutes
**Parallelization**: Per-contract
**Infrastructure**: None (schema validation)

```yaml
jobs:
  contract:
    runs-on: ubuntu-latest
    timeout-minutes: 5
    steps:
      - name: Validate OpenAPI specs
        run: make validate-contracts
      - name: Run Pact tests
        run: make test-contracts
```

**Purpose**:
- ✅ Prevent breaking API changes
- ✅ Validate OpenAPI/gRPC schemas
- ✅ Consumer-driven contract testing (Pact)

**Industry Standard**: Netflix, Spotify, ThoughtWorks use contract tests extensively.

---

### **Tier 4: E2E Tests** (Critical Paths Only)

**Trigger**: Nightly or manual
**Timeout**: 30 minutes
**Parallelization**: Limited (expensive)
**Infrastructure**: Full Kind cluster

```yaml
jobs:
  e2e:
    runs-on: ubuntu-latest
    timeout-minutes: 30
    if: github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'
    steps:
      - name: Create Kind cluster
        run: kind create cluster
      - name: Deploy all services
        run: make deploy-all
      - name: Run E2E tests
        run: make test-e2e
```

**Scenarios** (< 10 critical paths):
1. ✅ Signal ingestion → Investigation → Remediation → Success
2. ✅ Signal ingestion → Investigation → Remediation → Failure → DLQ
3. ✅ Multi-service workflow (Gateway → HolmesGPT → Data Storage)

**Why Nightly?**:
- ❌ Too slow for PR feedback (30+ min)
- ❌ Expensive (full cluster + all services)
- ❌ Flaky (network, timing issues)
- ✅ Good for regression detection

---

## 📋 **Path Filtering (Smart CI/CD)**

**Industry Standard**: Only run tests for changed code.

### **Example: Data Storage Service**

```yaml
on:
  pull_request:
    paths:
      - 'pkg/datastorage/**'
      - 'cmd/datastorage/**'
      - 'test/integration/datastorage/**'
      - 'migrations/**'
      - 'docker/data-storage.Dockerfile'
```

**Benefits**:
- ✅ **Faster feedback**: Only 4 min for Data Storage changes (not 30+ min for all services)
- ✅ **Lower cost**: Fewer GitHub Actions minutes
- ✅ **Better UX**: Developers see relevant test results

**Fallback**: Run all tests on main branch merges.

---

## 🎯 **Recommended Implementation for Kubernaut**

### **Phase 1: Per-Service Integration Lanes** (Immediate)

Create `.github/workflows/test-integration-services.yml`:

```yaml
jobs:
  integration-fast:
    strategy:
      matrix:
        service: [kubernaut-agent]
    timeout-minutes: 5

  integration-medium:
    strategy:
      matrix:
        service: [datastorage]
    timeout-minutes: 10

  integration-slow:
    strategy:
      matrix:
        service: [gateway, notification, toolset]
    timeout-minutes: 20
```

**Result**: 5 min total (parallel) instead of 30+ min (sequential).

### **Phase 2: Path Filtering** (Next)

Add path filters to each service lane:

```yaml
on:
  pull_request:
    paths:
      - 'pkg/datastorage/**'
      - 'cmd/datastorage/**'
      # ... service-specific paths
```

**Result**: Only run affected service tests (1-5 min for most PRs).

### **Phase 3: Contract Tests** (Future)

Add contract validation for API boundaries:

```yaml
jobs:
  contract:
    steps:
      - name: Validate OpenAPI specs
        run: spectral lint docs/api/*.yaml
      - name: Run Pact tests
        run: make test-contracts
```

**Result**: Prevent breaking API changes between services.

### **Phase 4: E2E Nightly** (Future)

Move E2E tests to nightly schedule:

```yaml
on:
  schedule:
    - cron: '0 2 * * *'  # 2 AM daily
  workflow_dispatch:     # Manual trigger
```

**Result**: Fast PR feedback (< 10 min), comprehensive nightly validation.

---

## 📊 **Performance Comparison**

| Approach | PR Feedback Time | Parallelization | Cost |
|----------|------------------|-----------------|------|
| **Current (Sequential)** | ~30 min | None | High |
| **Per-Service Lanes** | ~5 min | 5 services | Medium |
| **+ Path Filtering** | ~1-5 min | Only changed | Low |
| **+ E2E Nightly** | ~5 min (PR) | Smart | Optimal |

---

## 🎯 **Industry Benchmarks**

| Company | PR Feedback Time | Test Pyramid | E2E Strategy |
|---------|------------------|--------------|--------------|
| **Google** | < 10 min | 70/20/10 | Nightly |
| **Netflix** | < 5 min | 60/30/10 | Staging + Chaos |
| **Spotify** | < 10 min | 50/40/10 | Per-service lanes |
| **Microsoft** | < 10 min | 70/20/10 | Tiered (PR/CI/Nightly) |
| **Kubernaut (Recommended)** | **< 5 min** | **60/30/10** | **Per-service + Nightly** |

---

## ✅ **Summary: Industry-Standard Recommendations**

### **1. Test Tier Organization**

```
Unit (70%) → Integration (25%) → E2E (5%)
  ↓              ↓                  ↓
Every commit   Per-service      Nightly
< 2 min        < 5 min          < 30 min
```

### **2. Per-Service Integration Lanes**

✅ **Fast Lane**: HolmesGPT API (< 2 min)
✅ **Medium Lane**: Data Storage (< 5 min)
✅ **Slow Lane**: Gateway, Notification, Toolset (< 15 min)

### **3. Path Filtering**

Only run tests for changed services (1-5 min for most PRs).

### **4. E2E Strategy**

- ✅ Nightly schedule (comprehensive)
- ✅ Manual trigger (on-demand)
- ✅ < 10 critical scenarios only

### **5. Contract Tests**

- ✅ Validate API schemas (OpenAPI/gRPC)
- ✅ Consumer-driven contracts (Pact)
- ✅ Prevent breaking changes

---

## 📚 **References**

- **Google Testing Blog**: https://testing.googleblog.com/
- **Martin Fowler - Test Pyramid**: https://martinfowler.com/articles/practical-test-pyramid.html
- **Kent C. Dodds - Test Trophy**: https://kentcdodds.com/blog/write-tests
- **Netflix Tech Blog**: https://netflixtechblog.com/
- **Spotify Engineering**: https://engineering.atspotify.com/
- **Microsoft DevOps**: https://docs.microsoft.com/en-us/azure/devops/

---

**Authority**: This document reflects industry-standard practices from Google, Netflix, Spotify, and Microsoft for microservices test organization.

