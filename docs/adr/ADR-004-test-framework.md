# ADR-004: Test Framework Selection

**Status:** Accepted
**Date:** 2026-05-03
**Deciders:** AF team
**Source:** #42, QA-1 (#63)

## Context

The project needs a test framework for unit, integration, and conformance tests. The issue body (#42) initially mandated Ginkgo/Gomega BDD for all test tiers. However, protocol conformance tests are assertion-heavy (schema validation), not behavior-heavy (scenario orchestration).

## Decision

- **Unit and Integration tests:** Go standard `testing` package + `httptest` + `testify/assert`
- **Protocol conformance:** Go standard `testing` + table-driven tests + custom assertion helpers
- **E2E (Tier 3, deferred):** Ginkgo/Gomega (if needed for complex scenario orchestration)

## Consequences

- Lower dependency count (no Ginkgo for 90% of tests)
- Table-driven tests provide clear, reviewable test matrices for protocol conformance
- Custom assertion helpers (`assertJSONRPCError`, `assertMCPToolResult`) give readable failure messages
- IDE support (Go test runner) works out of the box without Ginkgo CLI
- Consistent with controller-runtime test patterns (envtest uses standard `testing`)
- E2E tier (deferred to Phase 2) may adopt Ginkgo for multi-step scenario orchestration

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| Ginkgo/Gomega for everything | Over-engineered for assertion-heavy conformance tests; adds dependency for no benefit at unit/integration level |
| testify/suite | Adds complexity without clear benefit over table-driven tests |
| Custom BDD framework | Maintenance burden; Ginkgo already exists for when BDD is genuinely needed |
