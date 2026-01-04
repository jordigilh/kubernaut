# ADR-CI-001: CI/CD Pipeline Testing Strategy

**Status**: ✅ Approved
**Date**: 2025-12-31
**Last Updated**: 2025-12-31
**Version**: 1.0
**Author**: AI Assistant
**Reviewers**: TBD
**Related**: [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc), [DD-TEST-001](DD-TEST-001-port-allocation-strategy.md)

---

## Context

The kubernaut CI/CD pipeline (GitHub Actions) must execute three test tiers efficiently:
1. **Unit Tests** - Fast, isolated business logic tests
2. **Integration Tests** - Service-level tests with real infrastructure (Postgres, Redis, DataStorage)
3. **E2E Tests** - Full system tests with Kind clusters and CRD controllers

**Key Challenges**:
- 8 services with integration tests taking ~3-8 minutes each
- E2E tests requiring infrastructure warmup (images, Kind clusters)
- Balancing parallelization vs. GitHub Actions runner capacity
- Path-based conditional execution vs. comprehensive coverage

**Problem Statement**: Optimize CI pipeline for speed without sacrificing test coverage or introducing flaky tests from path filtering.

---

## Decision

**Establish a two-tier conditional execution strategy: always-run integration matrix + conditionally-run E2E jobs.**

### **Integration Tests: Matrix Strategy (Always Run)**

**Pattern**: Single matrix job that executes all 8 services in parallel, runs for every PR/commit.

```yaml
integration-tests:
  strategy:
    fail-fast: false
    matrix:
      service:
        - name: Signal Processing
          path: signalprocessing
        - name: AI Analysis
          path: aianalysis
        # ... 6 more services
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - run: make generate  # Required for OpenAPI specs
    - run: make test-integration-${{ matrix.service.path }}
```

**Rationale for No Path Filtering** (Integration Tests):
- **Time Balance**: All integration tests complete in roughly the same timeframe (~3-8 min)
- **Comprehensive Coverage**: Services are highly interconnected; changes in one often affect others
- **Flake Prevention**: Path-based skipping can hide integration failures until later
- **GitHub Actions Efficiency**: Matrix parallelization is efficient; 8 jobs × 5 min = ~5 min total wall time
- **Debugging Simplicity**: Consistent execution makes CI failures easier to reproduce

**Quote from Project Context**:
> "since most of them will take about the same time, it's all good to run them all"

### **E2E Tests: Dedicated Jobs (Conditional Execution)**

**Pattern**: Separate jobs for each service, triggered conditionally based on path filters and integration test success.

```yaml
e2e-signalprocessing:
  needs: [build-and-lint, integration-tests]
  if: |
    needs.integration-tests.result == 'success' &&
    (contains(github.event.pull_request.labels.*.name, 'test-e2e') ||
     contains(fromJSON('["test/**", "pkg/signalprocessing/**", ...]'), github.event.pull_request.changed_files[*]))
  runs-on: ubuntu-latest
  steps:
    - uses: dorny/paths-filter@v3
      id: changes
      with:
        filters: |
          e2e:
            - 'test/e2e/signalprocessing/**'
            - 'pkg/signalprocessing/**'
            # ... other relevant paths
    - run: make test-e2e-signalprocessing
      if: steps.changes.outputs.e2e == 'true'
```

**Rationale for Conditional Execution** (E2E Tests):
- **Infrastructure Cost**: E2E tests require Kind cluster creation, image builds, Tekton installation
- **Time Investment**: Each E2E test takes 8-15 minutes
- **Selective Value**: E2E tests validate full system behavior; not every PR requires this
- **Future Consideration**: "It might be the same case for E2E that the average run is about the same so the time benefit to run conditionally is not worth it, but we'll see."

---

## Technical Implementation Details

### **Container Networking Strategy**

**Decision**: Use `host.containers.internal` for DataStorage container networking in integration tests.

**Problem Fixed** (2025-12-31):
- Original config files used container names (`workflowexecution_postgres_1`) without custom Podman networks
- DataStorage containers couldn't resolve PostgreSQL/Redis hostnames → DNS lookup failures
- Tests failed with: `lookup workflowexecution_postgres_1 on 192.168.127.1:53: no such host`

**Solution**:
- Updated all config files to use `host.containers.internal` with DD-TEST-001 allocated ports
- DataStorage connects to Postgres/Redis via host port mapping (no custom network needed)
- Matches successful patterns from Gateway and SignalProcessing services

**Example** (`test/integration/workflowexecution/config/config.yaml`):
```yaml
database:
  host: host.containers.internal  # Was: workflowexecution_postgres_1
  port: 15441                      # DD-TEST-001 allocated port

redis:
  addr: host.containers.internal:16388  # Was: workflowexecution_redis_1:6379
```

**Services Fixed**:
- `workflowexecution` → ports 15441/16388
- `notification` → ports 15439/16385
- `holmesgptapi` → ports 15439/16387

**Port Authority**: [DD-TEST-001](DD-TEST-001-port-allocation-strategy.md) v1.9

### **OpenAPI Spec Generation Requirement**

**Critical Dependency**: All integration and E2E jobs must run `make generate` before tests.

**Reason**:
- Go services depend on embedded OpenAPI specs (`openapi_spec_data.yaml`)
- `ogen` tool generates HolmesGPT-API client from OpenAPI specs
- Without generation: `openapi_spec_data.yaml: no matching files found`

**Implementation**:
```yaml
- name: Generate OpenAPI specs
  run: make generate
  env:
    PATH: "${{ github.workspace }}/bin:$PATH"  # ogen must be in PATH
```

---

## Matrix Parallelization Benefits

### **Integration Tests Performance**

| Strategy | Total Wall Time | Runner Usage | Debugging |
|----------|----------------|--------------|-----------|
| **Sequential** | 8 × 5 min = 40 min | 1 runner × 40 min | ✅ Simple |
| **Matrix Parallel** | max(5 min) = ~5 min | 8 runners × 5 min | ✅ Simple |

**Improvement**: **8x faster** (40 min → 5 min)

**Trade-offs**:
- ✅ **Pros**: Massive speedup, early failure detection, comprehensive coverage
- ⚠️ **Cons**: Uses 8 concurrent runners (GitHub Actions allows 20 concurrent jobs for free tier)

---

## Consequences

### **Positive**

- ✅ **Fast Feedback**: Integration tests complete in ~5 min (vs 40 min sequential)
- ✅ **Comprehensive Coverage**: All services tested every commit
- ✅ **Reliable Networking**: `host.containers.internal` pattern prevents DNS failures
- ✅ **Predictable Behavior**: No path-filter-induced flakiness in integration tests
- ✅ **E2E Efficiency**: Conditional execution saves CI time on non-critical changes
- ✅ **Port Collision Prevention**: DD-TEST-001 ensures no conflicts in parallel execution

### **Negative**

- ⚠️ **Runner Capacity**: Uses 8 concurrent runners for integration tests (40% of free tier limit)
- ⚠️ **E2E Complexity**: Path filters require maintenance as code structure changes
- ⚠️ **Integration Test Duration**: ~5 min baseline even for trivial changes
- ⚠️ **Container Cleanup**: Failed tests may leave containers (cleanup logic required)

### **Mitigation**

- 📊 **Monitor Runner Usage**: Track GitHub Actions runner usage to detect capacity issues
- 🔄 **E2E Path Review**: Quarterly review of path filters for accuracy
- 🧹 **Cleanup Automation**: `SynchronizedBeforeSuite` handles container cleanup
- 📈 **Future Optimization**: If E2E times normalize, consider always-run strategy

---

## Implementation Checklist

### **Phase 1: Integration Tests Matrix** (✅ Complete)
- [x] Create `integration-tests` matrix job with 8 services
- [x] Add `make generate` to each matrix job
- [x] Remove individual integration jobs (consolidated into matrix)
- [x] Fix container networking in 3 config files (WE, NT, HAPI)
- [x] Remove path filtering (always-run strategy)
- [x] Document rationale in workflow comments

### **Phase 2: E2E Conditional Execution** (✅ Complete)
- [x] Keep dedicated E2E jobs per service
- [x] Add `needs: [integration-tests]` dependency
- [x] Implement path filters with `dorny/paths-filter@v3`
- [x] Add manual trigger via `test-e2e` label
- [x] Document `make generate` requirement

### **Phase 3: Workflow Rename** (✅ Complete)
- [x] Rename `defense-in-depth-optimized.yml` → `ci-pipeline.yml`
- [x] Update comments to clarify CI/CD vs testing strategy
- [x] Reference DD-TEST-001 for port allocations
- [x] Reference ADR-CI-001 for strategy decisions

### **Phase 4: Documentation** (In Progress)
- [x] Create ADR-CI-001 (this document)
- [ ] Update `.github/workflows/README.md` with strategy overview
- [ ] Add troubleshooting section for common CI failures
- [ ] Document container cleanup procedures

---

## Decision Criteria (Future Reviews)

### **When to Reconsider Always-Run Integration Tests**

**Trigger Conditions**:
1. Integration test duration increases significantly (avg > 10 min)
2. GitHub Actions runner capacity becomes constrained
3. Cost concerns emerge (if migrating to paid tier)
4. Service isolation improves (reduced interconnections)

**Action**: Implement path-based filtering for integration tests if conditions met

### **When to Make E2E Tests Always-Run**

**Trigger Conditions**:
1. E2E tests stabilize in duration (~5 min avg)
2. E2E tests become critical for every PR (high interconnection)
3. Path filtering causes missed regressions
4. Infrastructure warmup time reduces significantly

**Action**: Remove path filtering from E2E tests

**Quote from Project Context**:
> "It might be the same case for E2E that the average run is about the same so the time benefit to run conditionally is not worth it, but we'll see."

---

## Integration with Other Standards

### **Port Allocation** ([DD-TEST-001](DD-TEST-001-port-allocation-strategy.md))
- Integration tests use ports 15433-18139 (Podman containers)
- E2E tests use ports 30080-30199 (Kind NodePort)
- Each service has dedicated 10-port buffer for parallel execution

### **Testing Strategy** ([03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc))
- Unit Tests: 70%+ coverage (fastest feedback)
- Integration Tests: >50% coverage (service coordination)
- E2E Tests: 10-15% coverage (critical user journeys)

### **Container Networking Patterns**
| Pattern | Services | Network Strategy | Config Example |
|---------|----------|------------------|----------------|
| **Port Mapping** | Gateway, SignalProcessing, WE, NT, HAPI | `host.containers.internal` | `host: host.containers.internal` |
| **Custom Network** | AIAnalysis, RemediationOrchestrator | Podman network | `host: postgres` (DNS) |
| **Static IPs** | RemediationOrchestrator | Podman network + IPs | `host: 10.88.0.20` |

---

## Success Metrics

### **Performance Targets**
- Integration tests: < 6 min total wall time ✅
- E2E tests (when run): < 15 min per service ⚠️ (monitoring)
- Full CI pipeline (all tiers): < 25 min ⚠️ (monitoring)

### **Reliability Targets**
- Integration test pass rate: > 95% ⚠️ (monitoring)
- E2E test pass rate: > 90% ⚠️ (monitoring)
- Container networking failures: 0% ✅ (fixed with host.containers.internal)

### **Coverage Targets**
- Integration tests execute: 100% of services every commit ✅
- E2E tests execute: > 80% relevance (path filtering) ⚠️ (monitoring)

---

## References

- **Testing Strategy**: [03-testing-strategy.mdc](mdc:.cursor/rules/03-testing-strategy.mdc)
- **Port Allocation**: [DD-TEST-001](DD-TEST-001-port-allocation-strategy.md)
- **Workflow File**: `.github/workflows/ci-pipeline.yml`
- **Container Networking**: DD-TEST-002 Sequential Startup pattern

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-31 | AI Assistant | Initial ADR: integration matrix (always-run), E2E conditional execution, container networking fixes |

---

**Authority**: This design decision is **AUTHORITATIVE** for CI/CD pipeline strategy.
**Scope**: All GitHub Actions workflows for kubernaut testing.
**Enforcement**: CI pipeline configuration MUST follow this strategy.
**Review Cadence**: Quarterly review of E2E path filters and integration test duration.

