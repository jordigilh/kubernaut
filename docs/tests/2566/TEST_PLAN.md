# Test Plan: Local Kind Demo OIDC Trust Root Alignment

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2566-v1  
**Feature**: Ensure local Kind demo Keycloak TLS uses the CA trusted by the operator-managed Console  
**Version**: 1.0  
**Created**: 2026-10-27  
**Author**: Kubernaut Platform  
**Status**: Active  
**Branch**: `fix/2566-oidc-ca`

---

## 1. Introduction

### 1.1 Purpose

Issue #2566 fixes the local Kind demo's split TLS trust roots. The test plan proves that
the operator internal CA exists before Keycloak TLS provisioning, that Keycloak's leaf
certificate validates against the Console's configured CA, and that fresh and retained
demo runs reach a healthy Console without disabling OIDC certificate verification.

### 1.2 Objectives

1. Prove the local demo seeds `kubernaut-internal-ca` before `SetupOIDCInfrastructure`.
2. Prove Keycloak OIDC discovery succeeds using the operator-managed CA.
3. Prove retained-cluster reruns reconcile stale `keycloak-tls` material without manual deletion.
4. Prove Fleet and shared Ginkgo OIDC paths retain their existing certificate lifecycle.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit/static contract pass rate | 100% | `go test ./test/infrastructure -run TestInfrastructure -ginkgo.focus=OIDC` |
| Wiring coverage | 100% of manifest rows | Local demo source contract and live Kind run |
| Console readiness | `2/2 Ready` | `kubectl -n kubernaut-system wait` / Deployment status |
| TLS verification | No insecure OIDC bypass | Console oauth2-proxy args and OIDC discovery logs |
| Regression | 0 unrelated failures | Focused infrastructure suite and full build/lint |

---

## 2. References

### 2.1 Authority

- [BR-PLATFORM-014](../../requirements/BR-PLATFORM-014-demo-environment-cert-lifecycle.md): Demo Environment Certificate and State Lifecycle
- Issue #2566: Kind demo Console oauth2-proxy trusts wrong CA for Keycloak OIDC discovery
- Issue #2492 / `docs/tests/2492/TEST_PLAN.md`: operator-backed demo deployment contract
- `test/infrastructure/oidc_helm.go`: OIDC provisioning order
- `test/infrastructure/operator_fleet_tls.go`: operator CA lifecycle

### 2.2 Control note

This change emits no production audit events. No SOC2/FedRAMP audit mapping is required.
TLS certificate verification remains enabled as the applicable security behavior.

---

## 3. Risks and Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Keycloak is provisioned from a CA different from the Console trust Secret | oauth2-proxy CrashLoopBackOff; local demo unusable | High | UT-INFRA-OIDC-2566-001, IT-INFRA-OIDC-2566-001 | Seed the operator CA before shared OIDC setup |
| R2 | A retained cluster contains a stale `keycloak-tls` Secret | Rerun continues to fail or uses an obsolete signer | Medium | IT-INFRA-OIDC-2566-002 | Reuse the operator CA and retain stale-secret detection/reissuance |
| R3 | Shared Ginkgo/Fleet paths receive local-only ordering changes | Existing short-lived E2E behavior regresses | Low | IT-INFRA-OIDC-2566-003 | Wire the preflight only in `setupLocalDemoInfrastructure` |

---

## 4. Scope

### 4.1 Features to be tested

- `hack/setup-demo-infra/main.go`: local demo CA preflight ordering.
- `test/infrastructure/operator_fleet_tls.go`: idempotent operator CA preparation.
- `test/infrastructure/oidc_helm.go` and `keycloak_certmanager.go`: Keycloak Issuer/Certificate trust root.
- Operator-backed Console readiness and oauth2-proxy OIDC discovery in local Kind.

### 4.2 Features not to be tested

- Production operator or Console implementation in the sibling repositories.
- Combined trust-bundle alternatives or insecure OIDC configuration.
- New certificate rotation behavior for non-demo Ginkgo suites.

### 4.3 Design decision

The local demo creates the operator CA before OIDC provisioning. This preserves one trust
root, avoids a second Keycloak rotation phase, and leaves shared non-demo paths unchanged.

---

## 5. Approach

### 5.1 TDD sequence

1. **RED**: add the failing ordering contract and this test plan.
2. **GREEN**: add the narrow CA preflight wrapper and local-demo wiring.
3. **REFACTOR**: document the trust-root invariant and remove avoidable duplication only if discovered.
4. **VERIFY**: run focused tests, build, lint, and fresh/retained Kind validation.

### 5.2 Pass/fail criteria

**PASS** — all of the following are true:

1. `UT-INFRA-OIDC-2566-001` passes.
2. `IT-INFRA-OIDC-2566-001` proves `keycloak-tls` chains to `kubernaut-internal-ca`.
3. `IT-INFRA-OIDC-2566-002` proves a retained stale leaf is reissued from the current CA.
4. `IT-INFRA-OIDC-2566-003` proves Fleet/Ginkgo paths remain unchanged.
5. The local demo reaches `kubernaut-console 2/2 Ready` without `--insecure-oidc-skip-issuer-verification`.
6. `go build ./...` and `golangci-lint run --timeout=5m` pass with no new failures.

**FAIL** — any P0 acceptance scenario fails, certificate verification is bypassed, or an
existing shared OIDC path regresses.

---

## 6. Test Items and Wiring Manifest

| Component | Production/qualification entry point | Wiring location | Proving scenario |
|---|---|---|---|
| Operator internal CA preflight | Local Kind demo bootstrap | `setupLocalDemoInfrastructure` | `IT-INFRA-OIDC-2566-001` |
| Keycloak CA Issuer/Certificate | Local OIDC infrastructure | `SetupOIDCInfrastructure` → `ensureKeycloakCertManagerIssuer` | `IT-INFRA-OIDC-2566-002` |
| Console OIDC readiness | Operator-backed local demo | `main` → `InstallDemoOperatorApplication` | `IT-INFRA-OIDC-2566-003` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-PLATFORM-014 | Demo TLS material uses one stable trust root | P0 | Unit/static | UT-INFRA-OIDC-2566-001 | Pass |
| BR-PLATFORM-014 | Operator CA preflight creates the expected Secret | P0 | Integration | IT-INFRA-OIDC-2566-001 | Pass |
| BR-PLATFORM-014 | Retained rerun repairs stale Keycloak TLS | P0 | Integration | IT-INFRA-OIDC-2566-002 | Pass |
| BR-PLATFORM-014 | Console OIDC discovery and readiness succeed | P0 | E2E | IT-INFRA-OIDC-2566-003 | Pass |
| BR-PLATFORM-014 | Non-demo paths remain unchanged | P1 | Regression | IT-INFRA-OIDC-2566-004 | Pass with unrelated baseline failure |

---

## 8. Test Scenarios

### UT-INFRA-OIDC-2566-001 — Local CA ordering

**Given** the local demo bootstrap source  
**When** its setup sequence is inspected  
**Then** operator CA preparation appears before `SetupOIDCInfrastructure`.

### IT-INFRA-OIDC-2566-001 — Operator CA preflight

**Given** a missing `kubernaut-internal-ca` Secret and a Kubernetes API  
**When** the local demo's operator CA preflight runs  
**Then** the preflight creates the expected administrator-owned CA Secret, and the local ordering contract places it before OIDC setup.

### IT-INFRA-OIDC-2566-002 — Retained stale Secret

**Given** a retained cluster with `keycloak-tls` signed by an obsolete CA  
**When** local demo setup is rerun  
**Then** stale material is removed/reissued and the resulting leaf validates against the current operator CA.

Live evidence: `kubernaut-internal-ca` and the Keycloak issuer CA both had SHA-256
fingerprint `5D:FD:86:56:EF:68:9D:67:1D:B1:73:AB:F9:06:4C:6F:3B:4C:D8:3C:42:69:4A:81:17:AB:1C:AF:09:B2:F5:02` after the rerun; no manual Secret deletion was performed.

### IT-INFRA-OIDC-2566-003 — Console readiness

**Given** the operator CR is reconciled after OIDC setup  
**When** the Console Deployment starts  
**Then** both Console containers are Ready and oauth2-proxy has no insecure issuer-verification bypass.

Live evidence: the Console pod reached `2/2` Ready, and oauth2-proxy logged successful OIDC
discovery without `--insecure-oidc-skip-issuer-verification`.

### IT-INFRA-OIDC-2566-004 — Regression boundary

**Given** Fleet demo and shared Ginkgo setup paths  
**When** their existing infrastructure tests run  
**Then** they retain their existing CA provisioning and certificate lifecycle.

---

## 9. Execution and Evidence

```bash
go test ./test/infrastructure -run TestInfrastructure -ginkgo.focus='OIDC'
go build ./...
golangci-lint run --timeout=5m
```

Live evidence must include the CA fingerprints for `kubernaut-internal-ca` and
`keycloak-tls`, the oauth2-proxy startup/discovery result, Console Deployment readiness,
and the retained-cluster rerun result. The full infrastructure suite had one unrelated
baseline failure in `fleet_api_diagnostics_test.go` (deadline text differed from the
expected `Connection refused`); 363/364 specs passed.

---

## 10. Exit Criteria

- All P0 scenarios pass.
- The wiring manifest is 100% covered.
- Fresh and retained local Kind runs meet the issue acceptance criteria.
- No unrelated baseline failure is misattributed to this change.
