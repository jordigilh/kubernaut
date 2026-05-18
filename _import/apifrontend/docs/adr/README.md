# Architecture Decision Records

This directory contains the Architecture Decision Records (ADRs) for kubernaut-apifrontend.

## Format

Each ADR follows the standard structure:
- **Title** — short noun phrase
- **Status** — Proposed, Accepted, Deprecated, Superseded
- **Context** — what motivated the decision
- **Decision** — what we chose
- **Consequences** — what results from the decision
- **Alternatives Considered** — what we rejected and why

## Index

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [ADR-001](ADR-001-crd-api-group.md) | CRD API group: `apifrontend.kubernaut.ai/v1alpha1` | Accepted | 2026-05-01 |
| [ADR-002](ADR-002-llm-provider.md) | LLM provider: Claude Sonnet 4.6 via Vertex AI | Accepted | 2026-04-30 |
| [ADR-003](ADR-003-metric-prefix.md) | Prometheus metric prefix: `af_` | Accepted | 2026-05-03 |
| [ADR-004](ADR-004-test-framework.md) | Test framework: Go stdlib + httptest (no Ginkgo for unit/integration) | Superseded by ADR-015 | 2026-05-03 |
| [ADR-005](ADR-005-session-persistence.md) | Session persistence: K8s CRD (not in-memory) | Accepted | 2026-05-01 |
| [ADR-006](ADR-006-ka-communication.md) | AF-to-KA communication: REST polling + CRD watch | Superseded by ADR-014 | 2026-05-02 |
| [ADR-007](ADR-007-spec-immutability.md) | Spec immutability: ValidatingAdmissionPolicy in prod, convention in dev | Accepted | 2026-05-03 |
| [ADR-008](ADR-008-rate-limiting.md) | Rate limiting: 3-tier (request rate, concurrency, token budget) | Accepted | 2026-05-02 |
| [ADR-009](ADR-009-go-module-dependency.md) | Go module dependency: direct import from kubernaut monorepo | Accepted | 2026-05-02 |
| [ADR-010](ADR-010-load-testing-tool.md) | Load testing tool: k6 (Grafana) | Accepted (speculative) | 2026-05-03 |
| [ADR-011](ADR-011-deployment-model.md) | Deployment model: operator for OCP prod, Helm for dev/test | Accepted | 2026-05-02 |
| [ADR-012](ADR-012-signal-mode-manual.md) | Signal mode: `manual` for user-initiated investigations | Accepted (speculative) | 2026-04-30 |
| [ADR-013](ADR-013-jwt-forwarding.md) | JWT forwarding for AF-to-KA identity delegation | Accepted | 2026-05-03 |
| [ADR-014](ADR-014-hybrid-ka-communication.md) | Hybrid REST + MCP client for AF-to-KA communication | Accepted | 2026-05-03 |
| [ADR-015](ADR-015-ginkgo-mandate.md) | Ginkgo/Gomega for all test tiers (supersedes ADR-004) | Accepted | 2026-05-04 |
| [ADR-016](ADR-016-jwks-failopen-rationale.md) | JWKS fail-open rationale and compensating controls | Accepted | 2026-05-05 |
| [ADR-017](ADR-017-crd-pii-classification.md) | CRD PII data classification and retention policy | Accepted | 2026-05-06 |
| [ADR-018](ADR-018-impersonation-risk-acceptance.md) | Impersonation risk acceptance and boundary enforcement | Accepted | 2026-05-07 |
| [ADR-019](ADR-019-audit-buffer-volatility.md) | Audit buffer volatility and overflow handling | Accepted | 2026-05-08 |
| [ADR-020](ADR-020-mcp-bridge-rbac-runtime.md) | MCP bridge RBAC: runtime enforcement only | Accepted | 2026-05-09 |
| [ADR-021](ADR-021-severity-triage.md) | Severity triage: mandatory LLM, no silent defaults | Accepted | 2026-05-10 |

## Adding New ADRs

1. Create a new file: `ADR-NNN-short-title.md`
2. Use the template above
3. Add an entry to this index
4. Submit as part of a PR for review

ADRs are immutable once accepted. To change a decision, create a new ADR that supersedes the old one (update the old ADR's status to "Superseded by ADR-NNN").
