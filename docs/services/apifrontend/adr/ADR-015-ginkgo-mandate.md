# ADR-015: Ginkgo/Gomega for All Test Tiers

**Status:** Accepted
**Date:** 2026-05-04
**Deciders:** AF team
**Supersedes:** [ADR-004](ADR-004-test-framework.md)
**Source:** kubernaut project-wide testing strategy (`03-testing-strategy.mdc`), PR #74 review

## Context

ADR-004 chose Go stdlib `testing` + `testify` for unit/integration tests, reserving Ginkgo
only for E2E scenarios. During PR #74, the kubernaut project-wide testing mandate
(`03-testing-strategy.mdc`) was applied, requiring Ginkgo/Gomega BDD across all
test tiers for consistency with the kubernaut monorepo and other satellite projects.

All 47 existing tests were migrated to Ginkgo/Gomega in commit `5b96f8d`, and 26
new specs were added for infrastructure packages. The codebase now has 73 Ginkgo
specs across 7 suites.

## Decision

Use Ginkgo/Gomega BDD for **all** test tiers: unit, integration, and E2E.

- Test IDs follow the `UT-AF-XXX-YYY` naming convention
- `ginkgo` CLI is the test runner (with `--race`, `--coverpkg`)
- `testify` is no longer a direct dependency

## Consequences

- Consistent with kubernaut ecosystem testing patterns
- BDD-style `Describe`/`It` blocks provide clear test organization
- `DeferCleanup` replaces `t.Cleanup` for resource management
- Ginkgo CLI required for test execution (installed via `go install`)
- IDE integration requires Ginkgo plugin for best experience

## Alternatives Considered

| Alternative | Why rejected |
|-------------|-------------|
| Keep ADR-004 (stdlib + testify) | Contradicts kubernaut project mandate; inconsistent with monorepo patterns |
| Hybrid (Ginkgo for integration, stdlib for unit) | Split tooling adds cognitive load; mandate requires uniformity |
