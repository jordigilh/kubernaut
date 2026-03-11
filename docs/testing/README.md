# Kubernaut Testing Documentation

This directory contains testing strategy, test plans, and implementation guidance for kubernaut.

## Testing Strategy

Kubernaut uses a **defense-in-depth** testing approach. Each tier targets **>=80% coverage of the code testable at that tier**, rather than a fixed split of total test count across tiers.

### Per-Tier Code Coverage Targets (>=80%)

| Tier | Scope | Coverage Target | What Counts as Testable |
|------|-------|-----------------|------------------------|
| **Unit** | Pure logic | >=80% of unit-testable code | Config, validators, scoring, builders, types |
| **Integration** | I/O boundaries | >=80% of integration-testable code | Reconcilers, K8s clients, HTTP handlers, DB adapters |
| **E2E** | Full stack | >=80% of full service code | Complete service execution in KIND |
| **All Tiers** | Merged | >=80% (line-by-line dedup) | Union of all tiers |

Each service defines `unit_exclude`/`int_include` patterns in `scripts/coverage/coverage_report.py` that partition code into tier-specific subsets. Run `make coverage-report` to measure.

### Key Principles

1. **TDD mandate** -- every business requirement (BR) must have a corresponding test
2. **Zero mocks in integration/E2E** -- see [INTEGRATION_E2E_NO_MOCKS_POLICY.md](INTEGRATION_E2E_NO_MOCKS_POLICY.md)
3. **Mock only external dependencies in unit tests** -- databases, LLM, K8s API, network services
4. **Real business logic everywhere** -- all `pkg/` components used as-is in every tier
5. **Ginkgo/Gomega BDD framework** -- mandatory for all tiers, no standard Go `testing`

### Mock Strategy

| Dependency Type | Unit Tests | Integration Tests | E2E Tests |
|-----------------|-----------|-------------------|-----------|
| External APIs (LLM, HolmesGPT) | Mock | Mock (`httptest` / mock container) | Mock (or real) |
| Databases (PostgreSQL, Redis) | Mock | Real (Podman containers) | Real |
| Kubernetes API | Fake client (`fake.NewClientBuilder()`) | envtest | KIND/OCP |
| Internal business logic (`pkg/`) | **Real** | **Real** | **Real** |

Integration test infrastructure is fully programmatic Go -- `test/infrastructure/` provides `StartDSBootstrap()` which orchestrates Podman containers for PostgreSQL, Redis, migrations, and the DataStorage API. Each service uses dedicated ports for parallel execution. See [INTEGRATION_TEST_INFRASTRUCTURE.md](INTEGRATION_TEST_INFRASTRUCTURE.md) for details.

## Documentation Index

### Mandatory Reading

| Document | Description |
|----------|-------------|
| [INTEGRATION_E2E_NO_MOCKS_POLICY.md](INTEGRATION_E2E_NO_MOCKS_POLICY.md) | Zero mocks policy for integration and E2E tests |
| [TESTING_PATTERNS_QUICK_REFERENCE.md](TESTING_PATTERNS_QUICK_REFERENCE.md) | Daily reference for testing patterns |
| [ANTI_PATTERN_DETECTION.md](ANTI_PATTERN_DETECTION.md) | Detecting and fixing NULL-TESTING, mock overuse, library testing |

### Guides and References

| Document | Description |
|----------|-------------|
| [TESTING_GUIDELINES_TRANSFORMATION_GUIDE.md](TESTING_GUIDELINES_TRANSFORMATION_GUIDE.md) | Testing transformation best practices and anti-patterns |
| [BEFORE_AFTER_EXAMPLES.md](BEFORE_AFTER_EXAMPLES.md) | Real transformation examples |
| [TESTING_COVERAGE_METHODOLOGY.md](TESTING_COVERAGE_METHODOLOGY.md) | Coverage measurement methodology |
| [EDGE_CASE_TESTING_GUIDE.md](EDGE_CASE_TESTING_GUIDE.md) | Edge case testing approach |

### Templates

| Document | Description |
|----------|-------------|
| [TEST_PLAN_TEMPLATE.md](TEST_PLAN_TEMPLATE.md) | Reusable test plan template -- use when creating a test plan for any new feature |
| [TEST_CASE_SPECIFICATION_TEMPLATE.md](TEST_CASE_SPECIFICATION_TEMPLATE.md) | IEEE 829 individual test case format |

### Infrastructure

| Document | Description |
|----------|-------------|
| [ENVTEST_SETUP_REQUIREMENTS.md](ENVTEST_SETUP_REQUIREMENTS.md) | envtest environment setup |
| [KIND_CLUSTER_TEST_TEMPLATE.md](KIND_CLUSTER_TEST_TEMPLATE.md) | KIND cluster test configuration |
| [REUSABLE_KIND_INFRASTRUCTURE.md](REUSABLE_KIND_INFRASTRUCTURE.md) | Shared KIND infrastructure for E2E |
| [INTEGRATION_TEST_INFRASTRUCTURE.md](INTEGRATION_TEST_INFRASTRUCTURE.md) | Integration test infrastructure setup |

### Test Plans

Test plans for specific features and issues live in subdirectories named by issue number or BR/DD identifier (e.g., `docs/testing/291/TEST_PLAN.md`, `docs/testing/BR-HAPI-197/`).

## Validation Commands

```bash
make test                          # Unit tests
make test-integration-[service]    # Integration tests per service
make test-e2e-[service]            # E2E tests per service
make coverage-report               # Per-tier coverage measurement
make lint-test-patterns            # Detect anti-patterns (NULL-TESTING, mock overuse)
make lint-tdd-compliance           # Verify BDD framework and BR references
```

## Quick Navigation

- [Main Documentation Index](../DOCUMENTATION_INDEX.md)
- [Architecture Decisions](../architecture/decisions/)
- [Business Requirements](../requirements/)
- [Development Guides](../development/)
