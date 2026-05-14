# Data Storage Service — Test Maturity Plan

**Milestone**: v1.5 GA (current)  
**Related**: [testing-strategy.md](testing-strategy.md), [integration-points.md](integration-points.md), `.cursor/rules/03-testing-strategy.mdc`

---

## Current state

- **Unit**: Broad coverage of `pkg/datastorage/` (validation, query builders, handlers, server wiring) with Ginkgo/Gomega; aim **≥80% of the unit-testable subset** defined in `scripts/coverage/coverage_report.py`.
- **Integration**: PostgreSQL / Redis (and related I/O) exercised via shared integration infrastructure; HTTP and repository paths covered for read/write flows.
- **E2E**: Full-stack scenarios run in **Kind** where enabled, validating the service with real dependencies alongside the rest of the platform.

---

## Target state

- **Per-tier code coverage**: **≥80%** of each tier’s testable subset (unit, integration, E2E), with **≥80% merged** across tiers after line deduplication — same model as the repo-wide testing strategy.
- **Traceability**: Each BR touching Data Storage maps to explicit test scenarios (preferred: test plan IDs; fallback: `BR-*` tags in `Describe`/`It` names).
- **Operational confidence**: Metrics and health remain observable during tests and in cluster (default Prometheus path: **`http://<pod-or-localhost>:9090/metrics`**, configurable via `metricsPort`).

---

## Gap analysis

| Area | Status | Notes |
|------|--------|--------|
| **Verify-chain** | Closing | Handler-level tests added; integration coverage for chain verification exists — keep aligned with API contract changes. |
| **Export** | Strong | Handler-level tests present; integration and E2E paths exist — maintain as export surface evolves. |
| **Lifecycle E2E** | Planned | End-to-end **startup → traffic → shutdown → DLQ drain** to validate DD-007 / graceful shutdown behavior under load (documented in architecture decisions). |
| **Reconstruction** | Strong | Unit + integration coverage; E2E scenario exists — extend when new reconstruction modes or storage layouts ship. |

---

## Timeline

| When | Focus |
|------|--------|
| **v1.5 GA** | Meet per-tier coverage targets, keep BR↔test mapping current, stabilize Kind E2E for DS-critical paths. |
| **Next** | Implement lifecycle E2E (startup → traffic → shutdown → DLQ drain) and fold results into CI gates. |

---

## Metrics

- **Coverage**: `make coverage-report` → `scripts/coverage/coverage_report.py` (service-specific `unit_exclude` / `int_include` patterns).
- **CI**: Unit, integration, and E2E targets as defined in Makefile and GitHub workflows; DS follows the same defense-in-depth pyramid as other services.
