# Kubernaut Coverage Methodology

Kubernaut tracks code coverage across three test tiers using a defense-in-depth testing pyramid. Coverage is reported automatically on every pull request via CI.

---

## Test Tiers

### Unit-Testable

Pure business logic that can be tested in isolation with no external dependencies.

**Includes**: config, validators, builders, formatters, classifiers, scoring, routing, retry, conditions, types, Rego policies.

**Excludes**: handlers, servers, DB adapters, K8s clients, workers, delivery channels.

### Integration-Testable

Code that requires real or simulated external dependencies (in-process controllers, test databases, HTTP servers).

**Includes**: handlers, servers, DB adapters, K8s clients, cache, enrichers, status updaters, audit managers, phase handlers, aggregators.

### E2E

Full end-to-end user journeys running against a Kind cluster with all services deployed.

### All Tiers (Merged)

Line-by-line deduplication across all three tiers. A statement covered by **any** tier counts once. This is the most accurate measure of overall test coverage.

---

## Quality Targets

| Tier | Target |
|------|--------|
| Unit-Testable | >= 80% |
| Integration-Testable | >= 80% |
| All Tiers (Merged) | >= 80% |

---

## How Coverage Is Reported

### On Every Pull Request

The CI pipeline runs `make coverage-report-markdown` and posts a coverage table as a PR comment. This is the **source of truth** for current coverage numbers -- it is always up to date and requires no manual maintenance.

### Locally

```bash
# Generate coverage report (markdown table)
make coverage-report-markdown

# Generate coverage report (plain text table)
make coverage-report

# Generate coverage report (JSON, for tooling)
make coverage-report-json
```

### Per-Service

```bash
# Run unit tests for a specific service
make test-unit-gateway

# Run all test tiers for a single service
make test-all-gateway

# Inspect coverage for a specific service
go tool cover -func=coverage_unit_gateway.out

# Generate HTML coverage report
go tool cover -html=coverage_unit_gateway.out -o coverage.html
```

---

## How Merging Works

The "All Tiers" metric is computed by `scripts/coverage/coverage_report.py`, which:

1. Collects coverage profiles from each tier (unit, integration, E2E)
2. Normalizes them to a common line-level format
3. Merges using logical OR -- if a line is covered by any tier, it counts as covered
4. Reports the merged percentage per service

This avoids double-counting lines covered by multiple tiers and gives the most accurate picture of total test coverage.

---

## HolmesGPT API (Hybrid Service)

HolmesGPT API is a Python service with a Go client wrapper, requiring special handling:

- **Python tests** (unit + integration): Run via `pytest` with `coverage.py`, producing `.coverage` files
- **Go tests** (E2E): Run via Ginkgo against the Go client in `pkg/holmesgpt/`, producing Go coverage profiles
- **Merging**: The coverage report script handles both Python and Go profiles when computing All Tiers
