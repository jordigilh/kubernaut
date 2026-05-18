# Test Plan — Cycle D: Supply Chain and Documentation

| Field | Value |
|-------|-------|
| **Identifier** | `TP-CYCLE-D-001` |
| **Version** | 1.0 |
| **Status** | Draft |
| **Created** | 2026-05-13 |
| **Standard** | IEEE 829-2008 |

## 1. References

| Ref | Document |
|-----|----------|
| R-01 | GA Readiness Audit — findings SC-02, CTR-01, WIRE-12, DOC-01, DOC-02 |
| R-02 | `.github/workflows/ci.yml` — CI workflow |
| R-03 | `.github/workflows/release.yml` — release workflow |
| R-04 | `deploy/kustomize/base/03-deployment.yaml` — base deployment |
| R-05 | `docs/operations/deployment-guide.md` — production deployment guide |
| R-06 | `README.md` — project README |
| R-07 | Cycle A–C Test Plans |

## 2. Introduction

This test plan covers **non-Go changes**: CI workflow hardening, manifest fixes,
operational runbooks, and documentation updates. These items do not follow strict TDD
(no Go test code) but are validated through structural checks and CI execution.

**Objective:** Ensure supply chain integrity (pinned actions), correct manifest references,
complete operational runbooks, and accurate documentation.

## 3. Test Items

| Item | Version | Source |
|------|---------|--------|
| `.github/workflows/ci.yml` | HEAD | R-02 |
| `.github/workflows/release.yml` | HEAD | R-03 |
| `deploy/kustomize/base/03-deployment.yaml` | HEAD | R-04 |
| `docs/operations/runbooks/RB-AF-011.md` | NEW | R-01 |
| `docs/operations/runbooks/RB-AF-012.md` | NEW | R-01 |
| `docs/operations/deployment-guide.md` | HEAD | R-05 |
| `README.md` | HEAD | R-06 |

## 4. Features to be Tested

### 4.1 SC-02 — Pin CI Actions by SHA

**Current behavior:** `actions/upload-artifact` and `actions/download-artifact` use tag
references (e.g. `@v4`) instead of commit SHA pinning.

**Required behavior:** All third-party GitHub Actions pinned by full commit SHA with
version comment.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-D-01a | Parse `ci.yml` | All `uses:` entries | Third-party actions use SHA (40-char hex) | Script |
| TC-D-01b | Parse `release.yml` | All `uses:` entries | Third-party actions use SHA | Script |
| TC-D-01c | SHA comment | Each pinned action | Comment indicates version (e.g. `# v4.4.3`) | Manual review |

### 4.2 CTR-01 — Base Kustomize Image Reference

**Current behavior:** `deploy/kustomize/base/03-deployment.yaml` may reference a specific
image tag that doesn't exist in registry for all environments.

**Required behavior:** Base uses a placeholder image reference that overlays replace.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-D-02a | Parse `03-deployment.yaml` | Container image field | Uses placeholder or documented default | Manual review |
| TC-D-02b | `make validate-kustomize` | All overlays | Build succeeds with image overrides | Script |

### 4.3 WIRE-12 — Operational Runbooks

**Current behavior:** PrometheusRule references `RB-AF-011.md` (Auth Latency High) and
`RB-AF-012.md` (Sustained Attack Pattern) but these files do not exist.

**Required behavior:** Both runbooks exist with actionable SRE guidance.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-D-03a | Parse PrometheusRule | All `runbook_url` annotations | Every referenced URL resolves to an existing file | Script |
| TC-D-03b | `RB-AF-011.md` content | Manual review | Contains: alert description, probable cause, triage steps, resolution, escalation | Manual |
| TC-D-03c | `RB-AF-012.md` content | Manual review | Contains: attack pattern description, indicators, response, escalation | Manual |

### 4.4 DOC-01 — Deployment Guide Readiness Probe

**Current behavior:** Deployment guide may describe readiness probe behavior incorrectly
(not reflecting dependency-aware `/readyz` from Cycle A fix).

**Required behavior:** Guide accurately describes:
- `:8081/readyz` checks dependency health (KA, DS)
- Returns 503 when any dependency unhealthy or draining
- Used by K8s for rolling update decisions

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-D-04a | `deployment-guide.md` | Grep for "readyz" | Describes dependency-aware behavior | Manual review |
| TC-D-04b | Same | Grep for probe ports | Port 8081 documented | Manual review |

### 4.5 DOC-02 — README Go Version

**Current behavior:** README may state an older Go version requirement.

**Required behavior:** States Go 1.26+ (matching `go.mod` toolchain).

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-D-05a | Parse `README.md` | Go version reference | States `1.26` or `1.26+` | Script |
| TC-D-05b | Parse `go.mod` | `go` directive | Matches README statement | Script |

## 5. Features Not to be Tested

| Feature | Rationale |
|---------|-----------|
| Syft/Trivy checksum verification (SC-01) | Deferred; requires upstream installer changes |
| SLSA provenance (issue #118) | Tracked separately, not v1.5 scope |

## 6. Approach

### 6.1 Action SHA Pinning Validation

Write a shell script or Go test that:
1. Parses `.github/workflows/*.yml`
2. Extracts all `uses:` references
3. Asserts third-party references (not `actions/checkout` which may be excluded) use SHA format
4. Verify SHA length is 40 hex characters

### 6.2 Runbook Completeness

Each runbook follows a standard template:
```
# RB-AF-NNN: Alert Name
## Description
## Probable Cause
## Triage Steps
## Resolution
## Escalation
## References
```

### 6.3 Cross-Reference Validation

Parse PrometheusRule `runbook_url` annotations, extract file paths, verify each exists
in the repository.

## 7. Pass/Fail Criteria

The plan **passes** when:
1. All CI action references use SHA pinning
2. `make validate-kustomize` exits 0
3. All referenced runbooks exist with required sections
4. Documentation accurately reflects post-Cycle-A/B/C behavior
5. `go.mod` Go version matches README

## 8. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/test/cycle-d-supply-chain-docs/TEST_PLAN.md` |
| Runbook `RB-AF-011.md` | `docs/operations/runbooks/RB-AF-011.md` |
| Runbook `RB-AF-012.md` | `docs/operations/runbooks/RB-AF-012.md` |
| Updated `deployment-guide.md` | `docs/operations/deployment-guide.md` |
| Updated `README.md` | `README.md` |

## 9. Cross-Phase Integration (Final)

| Integration point | Verification |
|-------------------|-------------|
| Runbook URLs in PrometheusRule → actual files | Script: parse YAML, check file existence |
| Deployment guide probe description → Cycle A `/readyz` behavior | Manual: verify guide matches implemented behavior |
| README Go version → `go.mod` toolchain | Script: compare versions |
| CI action SHAs → actual GitHub Action releases | Manual: verify SHA matches claimed version |
