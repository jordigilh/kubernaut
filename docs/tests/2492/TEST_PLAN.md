# Test Plan: Operator-built FullPipeline, Fleet, and demo deployment

**Test Plan Identifier**: TP-2492-v1.0  
**Feature**: Replace live application-chart deployment in FP/Fleet/demo paths with the pinned Kubernaut Operator bootstrap chart plus a user-applied `Kubernaut` CR.  
**Version**: 1.0  
**Created**: 2026-10-19  
**Author**: Kubernaut development team  
**Status**: Active  
**Branch**: `issue-2492-operator-helm`

---

## 1. Introduction

### 1.1 Purpose

This plan proves that issue #2492 uses an exact operator source revision and immutable
application/dependency inputs, installs only the operator bootstrap and independently
owned development dependencies, applies a user/GitOps-style `kubernaut.ai/v1alpha2`
`Kubernaut` resource, and preserves the real FullPipeline and Fleet journeys without
installing `charts/kubernaut`.

### 1.2 Objectives

1. Resolve and record operator source, image, chart, application, dependency, and
   qualification provenance before a live lane runs.
2. Prove the bootstrap chart creates no `Kubernaut` CR or application workload and that
   PostgreSQL/Valkey are owned by the disposable dependency chart.
3. Prove CR reconciliation gates on `Running`, positive conditions, service readiness,
   and EndpointSlices, then verifies every effective Deployment image against the
   complete 16-key operand map.
4. Prove FP, Fleet, and local/fleet demo paths have no live application Helm owner and
   retain explicit cleanup and diagnostics.
5. Prove the manual RC handoff model links per-artifact SBOM/signature/provenance data
   to both qualification lanes without adding cross-repository automation.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Requirements-based unit scenarios | >=19/20 passing | Ginkgo/Gomega scenarios below |
| Critical unit scenarios | 100% passing | Provenance, image integrity, ownership, TLS, and forbidden deployment tests |
| Wiring points | 100% | Wiring manifest in the approved implementation plan |
| FP/Fleet/demo live application Helm installs | 0 | Boundary scan and workflow review |
| Live operator qualification | FP and Fleet pass | Helios08 evidence plus CI artifacts |

---

## 2. References

### 2.1 Authority

- `BR-E2E-001`: preserve the real FullPipeline remediation journey.
- `BR-PLATFORM-014`: preserve the stable Fleet demo/QE environment and lifecycle.
- `DD-TEST-015`: provision Fleet infrastructure before the application configuration is reconciled.
- Issue #2492: run FP and Fleet against an operator-built deployment.
- Operator revision `c226705e24bef67fa947c535d59ea87d4251b483`.
- `docs/testing/TEST_PLAN_TEMPLATE.md`.

### 2.2 Cross-References

- `AGENTS.md` TDD, wiring, coverage, and completion requirements.
- `docs/architecture/decisions/ADR-068-fleet-external-infrastructure.md`.
- `docs/architecture/decisions/DD-AUDIT-003-service-audit-trace-requirements.md`.

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | A mutable or stale operator image is used | Qualification is not reproducible | High | UT-2492-001, UT-2492-004, IT-2492-CI-001 | Require exact SHA, immutable image evidence, and fail closed on mismatch |
| R2 | Bootstrap or dependency charts become application owners | Competing workloads and upgrade drift | High | UT-2492-005, UT-2492-006, IT-2492-BOOTSTRAP-001, IT-2492-DEPENDENCY-001 | Render/inspect ownership before CR apply |
| R3 | CR is treated as ready before all services and EndpointSlices are usable | Flaky or misleading FP/Fleet results | High | UT-2492-008, IT-2492-CR-001, IT-2492-EXPOSURE-001 | Gate on Running, positive conditions, every active service, and EndpointSlices |
| R4 | Operand image fallback silently selects a different artifact | Tests qualify the wrong application bytes | Critical | UT-2492-002, UT-2492-003, IT-2492-IMAGE-001 | Require all 16 related image entries and compare reconciled Deployments |
| R5 | NodePort adapter mutates operator ownership or disables TLS verification | Reconciliation/security regression | Critical | UT-2492-010, UT-2492-011, UT-2492-012, IT-2492-EXPOSURE-001 | Test-owned narrow selectors, preallocated mappings, CA verification, explicit deletion |
| R6 | Failure output omits the input contract | Root cause cannot be reconstructed | High | UT-2492-015, IT-2492-PROVENANCE-001 | Persist CR, provenance, conditions/events, dependencies, and exposure evidence |
| R7 | A local demo silently builds source or falls back to operator main | Non-deterministic user experience | High | UT-2492-016, UT-2492-017, IT-2492-DEMO-001 | Prebuilt-only default, explicit override escape hatch, dirty-tree rejection |
| R8 | Release automation creates a cross-repository coupling | RC promotion deadlock | Medium | UT-2492-018, UT-2492-019, IT-2492-RELEASE-001 | Keep handoff manual and link immutable artifacts in the manifest |

---

## 4. Scope

### 4.1 Features to be Tested

- `test/infrastructure/operator_deployment.go`: provenance, chart arguments, minimal CR
  rendering, dependency bootstrap, readiness, image verification, exposure, and diagnostics.
- `test/infrastructure/kind_cluster_helpers.go`: pre-creation port-mapping generation.
- `test/infrastructure/fullpipeline_e2e.go`, `fleet_e2e.go`, and `demo_helm.go`:
  operator wiring and cleanup ownership.
- `.github/workflows/ci-pipeline.yml` and release qualification inputs: exact-SHA artifact handoff.
- `hack/setup-demo-infra` and Make targets: prebuilt operator/application demo flow.
- `docs/tests/2492/TEST_PLAN.md` and qualification manifest format.

### 4.2 Features Not to be Tested

- Operator repository unit/envtest/provider-specific tests; those remain owned by
  `kubernaut-operator`.
- CNI dataplane enforcement. This qualification checks rendered NetworkPolicy objects
  and successful traffic through the supported Kind host/node path only.
- Cross-repository bot PR, repository-dispatch, or automatic operator release triggers.
- Offline deprecated application-chart lint/template/helm-unittest behavior except for
  confirming it is not used as a live qualification owner.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Operator bootstrap chart plus user-applied CR | The operator is the sole application owner and preserves the user/GitOps contract. |
| Dependency bootstrap before CR | PostgreSQL/Valkey are BYO inputs and must be ready before reconciliation. |
| Test-owned NodePort adapters | The CR has no NodePort API; adapters cross the Kind host boundary without mutating operator Services. |
| Exact 16-key related image map | `RELATED_IMAGE_*` is the operator's normal artifact contract; CR overrides remain optional. |
| Manual release handoff | Keeps source/build ownership separate without introducing a release deadlock. |

---

## 5. Approach

### 5.1 Coverage Policy

The frozen requirements-based unit denominator is 20 scenarios. At least 19/20 must
pass (>90%), and all critical provenance, ownership, image, TLS, and forbidden-deployment
scenarios must pass. Structural line coverage is diagnostic only.

### 5.2 Two-Tier Minimum

Every in-scope business requirement has unit contracts for pure builders/validators and
integration/live contracts for workflow wiring. FP/Fleet E2E remains the journey proof.

### 5.3 Pass/Fail Criteria

**PASS** requires all P0 scenarios, at least 19/20 unit scenarios, all wiring rows,
the scoped build/lint/tests, and successful FP/Fleet qualification evidence. Any live
application-chart installation in FP/Fleet/demo is a failure.

**FAIL** includes missing provenance, image mismatch, bootstrap-owned application
resources, readiness without EndpointSlices, insecure TLS bypass, incomplete diagnostics,
or a legacy live application-chart install.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods |
|------|-------------------|
| `test/infrastructure/operator_deployment.go` | operand-map validation, provenance/candidate parsing, chart/CR/adapter/qualification builders, readiness predicate, diagnostics |
| `test/infrastructure/kind_cluster_helpers.go` | Kind mapping append/validation |
| `test/infrastructure/demo_dependencies.go` | strict demo artifact override resolution |

### 6.2 Integration-Testable Code

| File | Functions/Methods |
|------|-------------------|
| `test/infrastructure/operator_deployment.go` | bootstrap install, CR apply, readiness, Deployment image verification, EndpointSlice exposure, cleanup |
| `test/infrastructure/fullpipeline_e2e.go` | operator deployment entry point |
| `test/infrastructure/fleet_e2e.go` | Fleet-first infrastructure and operator deployment |
| `test/infrastructure/demo_helm.go` | local/fleet demo operator path |
| `.github/workflows/ci-pipeline.yml` | exact-SHA build/artifact lane and FP/Fleet handoff |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Upstream operator | `c226705e24bef67fa947c535d59ea87d4251b483` | Exact qualification source |
| Kind node | `kindest/node:v1.37.0@sha256:a1ed56cf...` | Existing pinned FP profile |
| PostgreSQL | `docker.io/library/postgres:16.4@sha256:e62fbf9d...` | Operator harness Kind profile; record resolved digest per run |
| Valkey | `docker.io/valkey/valkey:8.0.2@sha256:57bcc49c...` | Operator dev-bootstrap profile; record resolved digest per run |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-E2E-001 | Preserve FP remediation | P0 | E2E | E2E-2492-FP-001 | Pending |
| BR-PLATFORM-014 | Preserve Fleet/demo lifecycle | P0 | E2E | E2E-2492-FLEET-001 | Pending |
| DD-TEST-015 | Fleet infrastructure before CR reconciliation | P0 | IT | IT-2492-FLEET-002 | Pending |
| BR-PLATFORM-014 | Exact operator/application provenance | P0 | Unit/IT | UT-2492-001, IT-2492-PROVENANCE-001 | RED |
| BR-E2E-001 | Complete related image contract | P0 | Unit/IT | UT-2492-002, IT-2492-003, IT-2492-IMAGE-001 | RED |
| BR-E2E-001 | Operator-only bootstrap ownership | P0 | Unit/IT | UT-2492-005, UT-2492-006, IT-2492-BOOTSTRAP-001 | RED |
| BR-E2E-001 | CR readiness and dependencies | P0 | Unit/IT | UT-2492-008, IT-2492-CR-001, IT-2492-DEPENDENCY-001 | RED |
| BR-E2E-001 | TLS-safe host boundary | P0 | Unit/IT | UT-2492-010, UT-2492-011, UT-2492-012, IT-2492-EXPOSURE-001 | RED |
| BR-PLATFORM-014 | Demo prebuilt artifact resolution | P1 | Unit/IT | UT-2492-016, UT-2492-017, IT-2492-DEMO-001 | RED |
| BR-PLATFORM-014 | Manual release handoff | P1 | Unit/IT | UT-2492-018, UT-2492-019, IT-2492-RELEASE-001 | RED |

---

## 8. Test Scenarios

### Tier 1: Unit Tests — frozen denominator 20

| ID | Business Outcome Under Test | Priority | Critical | Phase |
|----|----------------------------|----------|----------|-------|
| `UT-2492-001` | Provenance records repository, requested ref, resolved SHA, image/chart inputs, upstream SHA, workflow, and profile. | P0 | Yes | RED |
| `UT-2492-002` | A complete 16-key operand map is accepted in stable order. | P0 | Yes | RED |
| `UT-2492-003` | Missing or empty operand images fail closed; no fallback image is selected. | P0 | Yes | RED |
| `UT-2492-004` | Operator chart arguments use the exact immutable image/ref and never `latest`. | P0 | Yes | RED |
| `UT-2492-005` | Bootstrap chart arguments render operator-only resources and no application release. | P0 | Yes | RED |
| `UT-2492-006` | Dependency chart arguments own PostgreSQL/Valkey and do not render an application chart. | P0 | Yes | RED |
| `UT-2492-007` | CR rendering contains the small required BYO/policy/LLM contract and only explicit profile-specific settings (including the disposable-Kind PostgreSQL SSL mode); operator defaults are not copied into the CR. | P0 | No | RED |
| `UT-2492-008` | Readiness requires `Running`, positive conditions, all active services, and EndpointSlices; `Ready` alone is insufficient. | P0 | Yes | RED |
| `UT-2492-009` | Optional `spec.image.overrides` is absent by default and appears only for explicit custom/mirror runs. | P1 | No | RED |
| `UT-2492-010` | Initial TLS configuration includes the host/DNS SAN before certificate generation. | P0 | Yes | RED |
| `UT-2492-011` | Adapter Services have narrow operator-Pod selectors, named target ports, test ownership labels, and no operator owner reference. | P0 | Yes | RED |
| `UT-2492-012` | Adapter configuration preserves CA-based HTTPS verification and records node/port evidence. | P0 | Yes | RED |
| `UT-2492-013` | Kind extra port mappings are generated before cluster creation without collisions or fixed global assumptions. | P1 | No | RED |
| `UT-2492-014` | Fleet CR fields preserve separate read and write OAuth2 Secret references. | P0 | Yes | RED |
| `UT-2492-015` | Failure diagnostics include operator/artifact identity, CR, status/conditions/events, dependencies, and scenario. | P0 | Yes | RED |
| `UT-2492-016` | Explicit demo artifact/YAML overrides win over release defaults. | P1 | No | RED |
| `UT-2492-017` | Dirty explicit operator source checkout is rejected; normal demo setup does not build source. | P0 | Yes | RED |
| `UT-2492-018` | Qualification manifest requires operator digest and links per-artifact SBOM/signature/provenance. | P0 | Yes | RED |
| `UT-2492-019` | Legacy live application-chart deployment is detected while offline deprecated checks remain allowed. | P0 | Yes | RED |
| `UT-2492-020` | Operator adapters and dependency releases have explicit teardown actions. | P1 | No | RED |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-2492-CI-001` | CI builds and records the operator from the exact resolved SHA, then passes the artifact to FP/Fleet. | RED |
| `IT-2492-BOOTSTRAP-001` | Both operator charts lint/render and bootstrap creates no CR or application workload. | RED |
| `IT-2492-CR-001` | The production deployment entry point applies the CR and waits on the real operator lifecycle contract. | RED |
| `IT-2492-IMAGE-001` | Every reconciled Deployment image matches the recorded 16-key map. | RED |
| `IT-2492-DEPENDENCY-001` | PostgreSQL/Valkey are ready before CR reconciliation and remain independently owned. | RED |
| `IT-2492-FLEET-002` | Keycloak/MCP Gateway/remote cluster exist before the Fleet CR is first reconciled. | RED |
| `IT-2492-EXPOSURE-001` | Host access works through test-owned NodePort adapters with EndpointSlices and CA/SAN verification. | RED |
| `IT-2492-PROVENANCE-001` | Qualification artifacts contain all identity, diagnostic, and dependency evidence. | RED |
| `IT-2492-DEMO-001` | Local and `-mode fleet` demo paths use the same operator bootstrap/CR path with prebuilt inputs. | RED |
| `IT-2492-RELEASE-001` | Release qualification consumes a pinned operator candidate manifest without bot PR/dispatch automation. | RED |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-2492-FP-001` | Existing deterministic signal-to-remediation FullPipeline journey passes through operator-managed workloads. | VERIFY |
| `E2E-2492-FLEET-001` | Existing remote OAuth2/MCP/scope/remediation Fleet journey passes through operator-managed workloads. | VERIFY |
| `E2E-2492-DEMO-001` | Local and Fleet demo environments expose the Console and scenario tooling without application Helm ownership. | VERIFY |

---

## 9. Environmental Needs

### 9.1 Unit Tests

- Ginkgo/Gomega BDD; no external dependency mocks.
- Local filesystem reads are limited to boundary/provenance fixtures.

### 9.2 Integration/E2E Tests

- Disposable Kind/Podman cluster with Helm and `kubectl`.
- `ssh helios08` for live qualification only, using dedicated cluster names.
- Exact operator revision, application image artifacts, and immutable dependency profile.
- No unrelated cluster or dirty sibling checkout access.

---

## 10. Execution Order

1. **RED**: add this plan and the 20 failing contracts.
2. **GREEN**: implement shared operator deployment/configuration and wire FP/Fleet/demo.
3. **REFACTOR**: consolidate helpers, remove chart-specific ownership, improve diagnostics.
4. **WIRING VERIFICATION**: prove every plan wiring row through production entry points.
5. **VERIFY**: run scoped unit/integration/build/lint/boundary checks, then Helios08 FP/Fleet/demo qualification.

---

## 11. Deliverables

| Deliverable | Location |
|-------------|----------|
| Test plan | `docs/tests/2492/TEST_PLAN.md` |
| Unit contracts | `test/infrastructure/operator_deployment_test.go` |
| Operator deployment helper | `test/infrastructure/operator_deployment.go` |
| Qualification manifest format | `docs/tests/2492/` and CI artifacts |
| CI/release artifact lane | `.github/workflows/ci-pipeline.yml`, release workflow |

---

## 12. Wiring Verification

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| Exact-SHA operator artifact | CI `build-operator` | FP/Fleet E2E job | `IT-2492-CI-001` | Pending |
| Bootstrap/dependency charts | FP/Fleet/demo setup | Ready operator/DB/Valkey | `IT-2492-BOOTSTRAP-001`, `IT-2492-DEPENDENCY-001` | Pending |
| `Kubernaut` CR builder/applier | FP/Fleet/demo setup | Running CR and services | `IT-2492-CR-001` | Pending |
| Related image verification | CR reconciliation | Deployment image inventory | `IT-2492-IMAGE-001` | Pending |
| Fleet-first provisioning | Fleet setup | Remote OAuth2/MCP journey | `IT-2492-FLEET-002` | Pending |
| Host-access adapters | FP/Fleet/demo clients | CA-verified HTTPS | `IT-2492-EXPOSURE-001` | Pending |
| Provenance/diagnostics | CI/evidence collector | Qualification manifest | `IT-2492-PROVENANCE-001` | Pending |

---

## 13. Existing Tests Requiring Updates

| Location | Current Assertion | Required Change | Reason |
|----------|-------------------|-----------------|--------|
| `test/e2e/fullpipeline/suite_test.go` | Uses chart-pinned NodePorts and fixed URLs | Consume operator exposure evidence and CA-aware URLs | Operator Services remain ClusterIP |
| `test/e2e/fleet/suite_test.go` | Assumes the shared Helm install owns app workloads | Consume operator CR/exposure evidence | Removes competing application owner |
| `hack/setup-demo-infra/main.go` | Calls `InstallDemoHelmChart` | Calls shared operator bootstrap/CR flow | Demo is a supported operator path |
| `Makefile` demo help | Says “helm install” | Says “operator bootstrap + CR” | User-facing contract update |

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-10-19 | Initial RED-phase plan for issue #2492 |
