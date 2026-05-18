# Release Readiness Audit Template

**Service:** kubernaut-apifrontend
**Version:** _v1.x.x_
**Date:** _YYYY-MM-DD_
**Auditor:** _Name_

## Feature Inventory

| Feature | Issue | Status | Coverage | Notes |
|---------|-------|--------|----------|-------|
| _feature name_ | #XX | Complete/Partial/Missing | XX% | |

## Test Coverage

| Package | Line Coverage | Branch Coverage | Minimum |
|---------|-------------|-----------------|---------|
| internal/auth | | | 80% |
| internal/ratelimit | | | 80% |
| internal/handler | | | 80% |
| internal/session | | | 80% |
| internal/agent | | | 80% |
| internal/launcher | | | 80% |
| **Total** | | | **80%** |

## Quality Gates

- [ ] All unit tests pass (`make test-unit`)
- [ ] Coverage >= 80% (`go tool cover -func=cover.out`)
- [ ] No critical/high linter findings (`make lint`)
- [ ] No critical/high vulnerabilities (`make image-scan`)
- [ ] SBOM generated (`make sbom`)
- [ ] Service maturity checks pass (`make validate-maturity-ci`)
- [ ] OpenAPI spec valid (`make validate-openapi`)
- [ ] Generated code up to date (`make verify-generate`)

## Security Checklist

- [ ] No new dependencies with known CVEs
- [ ] No hardcoded secrets or credentials in code
- [ ] RBAC scoped to minimum required permissions
- [ ] Audit events emitted for all security-relevant actions
- [ ] Rate limiting covers all public endpoints
- [ ] JWT validation configured for all protected routes

## Operational Readiness

- [ ] Prometheus alerting rules deployed and tested
- [ ] Runbooks exist for all alerts
- [ ] Hot-reload tested for log-level changes
- [ ] Graceful shutdown tested (SSE drain, audit flush)
- [ ] Health/readiness probes configured in Helm chart
- [ ] Resource limits set appropriately

## Known Issues / Technical Debt

| Issue | Severity | Mitigation | Target |
|-------|----------|------------|--------|
| | | | |

## Verdict

- [ ] **GO** — All gates pass, no blocking issues
- [ ] **CONDITIONAL GO** — Minor issues documented above, fix within 1 sprint
- [ ] **NO GO** — Blocking issues must be resolved before release

**Sign-off:**
- QE: ___________ Date: ___
- SRE: ___________ Date: ___
- Security: ___________ Date: ___
- Product: ___________ Date: ___
