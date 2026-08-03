# Test Plan: LLM Profile Consolidation (Helm Chart)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1735-v1.0
**Feature**: Replace literal per-service LLM config blocks in the Helm chart with a shared `global.llmProfiles` map referenced by name, mirroring the Kubernaut Operator's `llmProfiles`/`llmProfileRef` pattern.
**Version**: 1.0
**Created**: 2026-07-26
**Author**: Platform (AI-assisted, DD-PLATFORM-007 implementation)
**Status**: Draft
**Branch**: `feat/1735-llm-profile-consolidation`

---

## 1. Introduction

### 1.1 Purpose

This test plan defines the test design for DD-PLATFORM-007's implementation: it proves that
`global.llmProfiles` resolution, per-consumer reference fields, `phaseModels` overrides, and
multi-secret-volume mounting render correctly for every consumer (Kubernaut Agent static config,
KA runtime ConfigMap, KA `phaseModels`, KA `alignmentCheck`, API Frontend `agent.llm`, AF
`severityTriage.llm`), and that the two `fail()` guards (undefined profile reference, #1731
vertex_ai shared-ambient-credentials violation) fire only in their correct, verified scope.

### 1.2 Objectives

1. **Profile resolution correctness**: every reference field resolves to the correct
   `global.llmProfiles` entry, with override precedence (non-empty override field wins) and
   inherit-when-empty default semantics matching the Operator's `EffectiveLLM`/`EffectivePhaseConfig`.
2. **Zero silent misconfiguration**: an undefined profile name and a #1731 vertex_ai violation
   both `fail()` at `helm template`/`helm upgrade` time — never render successfully with wrong
   or ambiguous credentials.
3. **Multi-secret-mount correctness**: a consumer sharing its base profile's `credentialsSecretName`
   gets no duplicate mount; a consumer with a distinct `credentialsSecretName` gets its own mount
   with the correct provider-specific credential filename (`api_key` vs. `credentials.json`).
4. **Field-name fidelity**: every field the chart renders matches the consuming Go struct's YAML
   tag exactly — the specific class of bug (`apiKey` vs. `apiKeyFile`) this DD fixes.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| helm-unittest pass rate | 100% | `helm unittest charts/kubernaut` |
| Wiring Manifest row coverage | 8/8 rows have a passing IT case | Section 7 BR Coverage Matrix |
| `helm lint` / `helm template` render validity | 0 errors across representative value sets | Section 13 Execution |
| Regression in existing 9 helm-unittest suites | 0 | `helm unittest charts/kubernaut` (full suite) |
| Field-name audit | 100% of resolved profile fields cross-checked against Go YAML tags | Manual review, documented in PR description |

---

## 2. References

### 2.1 Authority (governing documents)

- [BR-PLATFORM-008](../../requirements/BR-PLATFORM-008-helm-chart-llm-configuration-parity.md): Helm Chart LLM Configuration Parity
- [DD-PLATFORM-007](../../architecture/decisions/DD-PLATFORM-007-llm-profile-consolidation.md) v1.2: LLM Profile Consolidation (Helm Chart) — confidence 92%, spike-validated
- [DD-PLATFORM-005](../../architecture/decisions/DD-PLATFORM-005-helm-unittest-ci-integration.md): helm-unittest as the dedicated fast-fail CI gate for this chart (establishes helm-unittest, not Ginkgo, as this project's approved BDD-equivalent for Helm template assertions — Go's Ginkgo/Gomega mandate governs Go business-logic tests; this DD makes zero Go code changes)
- Issue #1735: implementation tracking issue (embeds this plan's Wiring Manifest as a checklist)
- Issue #1589: Helm/Operator parity triage (same family)
- Issue #1731: vertex_ai ambient-ADC constraint (this plan's IT-PLATFORM-LLM-008 proves the guard)

### 2.2 Cross-References

- `kubernaut-operator/internal/resources/validation_test.go` (VL-001..027) — the Operator's equivalent coverage this plan targets parity with.
- `charts/kubernaut/tests/` — existing 9 helm-unittest suites (81+ cases); none currently reference `kubernautAgent.llm`/`alignmentCheck` (confirmed by content grep, not just filename, during DD-PLATFORM-007 preflight).

---

## 3. Risks & Mitigations

> Carried forward from DD-PLATFORM-007's Risks table — reproduced here per IEEE 829 §5 so test
> design is driven directly by these, not derived separately.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|-----------------|------------|
| R1 | Helm merge helper diverges from Go's `EffectiveLLM`/`EffectivePhaseConfig` field precedence for some field | High (silently wrong LLM identity/credentials in production) | Low (verified field-for-field against the Operator's `configmaps.go`) | IT-PLATFORM-LLM-002..005 | Explicit override-wins/inherit-when-empty assertion per field, not just per profile |
| R2 | Multi-secret-volume mounting (first-of-its-kind Sprig logic for this chart) has an implementation edge case | Medium (extra/missed mount) | Medium | IT-PLATFORM-LLM-008 | Dedicated cases: same-secret-two-consumers -> 1 mount; distinct-secrets -> N mounts with correct filenames (spike already proved the mechanism against `helm template`) |
| R3 | (Historical, resolved by #1861) #1731 guard applied to the wrong scope (e.g. accidentally blocking KA's `phaseModels`) | Medium (false-positive rejects valid configs) | Low (scope verified against the Operator's `validation.go`) | IT-PLATFORM-LLM-008 (negative case) | KA `phaseModels` with two different `vertex_ai` profiles renders successfully — explicit negative test, not just the positive AF-fails case. #1861 removed the guard entirely, so this scope question is now moot; the negative test remains as a regression guard. |
| R4 | Repeat of an `apiKey`/`apiKeyFile`-style field-name mismatch on a different field | High if it recurs | Low (this DD's entire purpose is closing that class of bug) | All IT-PLATFORM-LLM-* | Manual field-name audit against Go struct YAML tags, documented in PR description (Section 1.3 metric) |

### 3.1 Risk-to-Test Traceability

All four risks have at least one directly mitigating test ID (see table above) — no coverage gaps.

---

## 4. Scope

### 4.1 Features to be Tested

- **`kubernaut.llm.resolveProfile` helper** (`charts/kubernaut/templates/_helpers.tpl`): profile lookup, `fail()` on unknown name, override-merge semantics.
- **KA static SDK config + `llm-runtime.yaml`** (`templates/kubernaut-agent/kubernaut-agent.yaml`): `llmProfileRef` resolution into both the static and hot-reloadable ConfigMaps.
- **KA `phaseModels`** (new): per-phase override resolution (`rca`/`workflow_discovery`/`validation`), unknown phase -> `fail()`.
- **KA `alignmentCheck.llm`**: bug fix (`apiKeyFile` not `apiKey`), profile-ref with inherit-when-empty.
- **AF `agent.llm`** (new) and **AF `severityTriage.llm`** (new) (`templates/apifrontend/apifrontend.yaml`): inherit-when-empty default chains, disabled/absent renders no `llm:` block.
- **Multi-secret-volume mounting**: dedup logic, provider-specific credential filenames.
- **#1731 `fail()` guard**: AF-scoped only.

### 4.2 Features Not to be Tested

- **Go-side LLM dispatch logic** (`cmd/apifrontend/backend_deps.go`'s `newLLMTriagerFromConfig`, KA's client construction): already covered by that code's own existing Go test suites; this DD makes zero Go changes, so no new Go-side test is warranted.
- **DD-LLM-008's restart-required identity-lock runtime behavior**: unaffected by this DD (only changes how `llm-runtime.yaml` is *assembled*, not runtime hot-reload semantics) — covered by DD-LLM-008's own existing test coverage, out of scope here.
- **`values-fleet.yaml`/`values-airgap.yaml` overlays**: confirmed (grep) to contain zero references to the fields being restructured — no test needed.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| helm-unittest (not Ginkgo) as the test framework | This is a Helm-template-only change (zero Go code); helm-unittest is this project's established, DD-PLATFORM-005-approved mechanism for asserting rendered chart output. Ginkgo/Gomega's BDD mandate governs Go business-logic tests, which this DD does not touch. |
| Single practical tier (Integration) rather than forcing a Unit tier | There is no new pure-logic Go function to unit test — the "logic" here is entirely Helm/Sprig template resolution, which only exists as rendered output (i.e., it is inherently integration/wiring-tier, per the Pyramid Invariant's own definition of what IT proves). Documented as N/A, not silently skipped (Section 8). |
| E2E deferred | See Section 8 Tier Skip Rationale. |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc`, adapted for a Helm-only change per Section 4.3 above.

- **Unit**: N/A — no new Go logic (see Section 4.3).
- **Integration**: 100% of the 8 Wiring Manifest rows (Section 7) have at least one passing helm-unittest case — the applicable analog to "80% of integration-testable code" when the entire testable surface is template wiring, not line-counted Go.
- **E2E**: deferred (Section 8).

### 5.2 Two-Tier Minimum — Exception Documented

This plan intentionally uses a **single practical tier** (Integration via helm-unittest) rather
than the standard UT+IT minimum, because the Unit tier has no applicable target (Section 4.3).
This is the tier-skip-rationale case AGENTS.md requires to be documented, not silently applied —
see Section 8.

### 5.3 Business Outcome Quality Bar

Every test scenario below asserts a business outcome ("the correct credentials/profile end up
mounted/rendered for this consumer") via `helm template` output — never merely "the helper was
called."

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:

1. All 8 Wiring Manifest rows (Section 7) have a passing `IT-PLATFORM-LLM-*` case.
2. `helm lint` + `helm template` clean across all Section 13 representative value combinations.
3. Zero regressions in the existing 9 helm-unittest suites (81+ pre-existing cases).
4. Field-name audit (Section 1.3) complete with zero mismatches found.

**FAIL** — any of:

1. Any `IT-PLATFORM-LLM-*` case fails.
2. Any pre-existing helm-unittest case that was passing now fails (regression).
3. `helm lint`/`helm template` errors on any representative value combination.

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**: the chart fails to render at all (`helm template` errors before
reaching any assertion) — fix the render error before continuing test authoring.

**Resume testing when**: `helm template` succeeds (even if assertions still fail) — assertion
failures are expected and normal during RED/GREEN, not a suspension condition.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

N/A — no new Go code (see Section 4.3).

### 6.2 Integration-Testable Code (Helm template wiring)

| File | Templates/Helpers | Lines (approx, post-implementation) |
|------|--------------------|--------------------------------------|
| `charts/kubernaut/templates/_helpers.tpl` | `kubernaut.llm.resolveProfile`, phase-override merge, secret-volume dedup helpers | ~120 (new) |
| `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml` | Static SDK config block, `llm-runtime.yaml` ConfigMap, `phaseModels`, `alignmentCheck.llm`, volume/volumeMount list | ~60 (modified from literal to helper-resolved) |
| `charts/kubernaut/templates/apifrontend/apifrontend.yaml` | New `agent.llm`, `severityTriage.llm` blocks, volume/volumeMount list | ~50 (net-new) |
| `charts/kubernaut/values.schema.json` | `global.llmProfiles`, 5 reference fields | ~90 (schema only, no logic) |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|-----------------|-------|
| Code under test | `feat/1735-llm-profile-consolidation` HEAD | Branched from `origin/main` at `c6d06ca5d` |
| Dependency: helm-unittest plugin | per `charts/kubernaut/tests/` existing usage | No version change required |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-PLATFORM-008 FR-1 | `global.llmProfiles` map | P0 | Integration | IT-PLATFORM-LLM-001 | Pending |
| BR-PLATFORM-008 FR-2 | `kubernautAgent.llmProfileRef` (required) replaces literal block | P0 | Integration | IT-PLATFORM-LLM-002, IT-PLATFORM-LLM-003 | Pending |
| BR-PLATFORM-008 FR-3 | `kubernautAgent.phaseModels` (new) | P0 | Integration | IT-PLATFORM-LLM-004 | Pending |
| BR-PLATFORM-008 FR-4 | `kubernautAgent.alignmentCheck.llmProfileRef` (bug fix) | P0 | Integration | IT-PLATFORM-LLM-005 | Pending |
| BR-PLATFORM-008 FR-5 | `apifrontend.llmProfileRef` (new) | P0 | Integration | IT-PLATFORM-LLM-006 | Pending |
| BR-PLATFORM-008 FR-6 | `apifrontend.severityTriage.llmProfileRef` (new) | P0 | Integration | IT-PLATFORM-LLM-007 | Pending |
| BR-PLATFORM-008 Success Criterion 3 | Multi-secret-volume mounting (shared vs. dedicated) | P0 | Integration | IT-PLATFORM-LLM-008 | Pending |
| BR-PLATFORM-008 Success Criterion 4 | #1731 vertex_ai constraint, AF-scoped (originally a `fail()` guard; lifted by #1861 once both severityTriage Vertex constructors resolve credentials independently) | P0 | Integration | IT-PLATFORM-LLM-008 | Pass |

### Status Legend

Pending / RED / GREEN / REFACTORED / Pass (per template convention).

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `IT-PLATFORM-LLM-{SEQUENCE}` — reserved directly in DD-PLATFORM-007's Wiring Manifest;
reused here rather than renumbering under the general `{TIER}-{SERVICE}-{BR}-{SEQ}` convention,
since the DD already fixed these IDs as the checklist basis for issue #1735.

### Tier 1: Unit Tests

**N/A** — see Section 4.3 and Tier Skip Rationale below.

### Tier 2: Integration Tests

**Testable code scope**: all files in Section 6.2; target 8/8 Wiring Manifest rows covered.

| ID | Business Outcome Under Test | Phase |
|----|------------------------------|-------|
| `IT-PLATFORM-LLM-001` | An undefined `llmProfileRef`/`phaseModels` entry fails the render with a clear error, never silently falls through | Pending |
| `IT-PLATFORM-LLM-002` | KA's static SDK config (env/ConfigMap) resolves `llmProfileRef` into the correct provider/model/credentials fields | Pending |
| `IT-PLATFORM-LLM-003` | KA's `llm-runtime.yaml` ConfigMap resolves the same profile's hot-reloadable fields correctly | Pending |
| `IT-PLATFORM-LLM-004` | `phaseModels` override resolves the documented field subset only (no `azureApiVersion`/`bedrockRegion`); unknown phase name fails the render | Pending |
| `IT-PLATFORM-LLM-005` | `alignmentCheck.llmProfileRef` empty inherits KA's resolved profile; set explicitly resolves to a different profile; renders `apiKeyFile` (not the old broken `apiKey`) | Pending |
| `IT-PLATFORM-LLM-006` | AF `llmProfileRef` empty defaults to KA's profile; set explicitly overrides | Pending |
| `IT-PLATFORM-LLM-007` | AF `severityTriage.llmProfileRef` empty inherits AF's resolved profile; disabled/absent renders no `llm:` block (rule-based-only triage preserved) | Pending |
| `IT-PLATFORM-LLM-008` | Same secret across multiple consumers mounts once (no duplicate); distinct secrets each get their own mount with the correct `api_key`/`credentials.json` filename; #1731 violation (AF main + severityTriage both vertex_ai, different secrets) fails the render; the equivalent KA `phaseModels` scenario does NOT fail (negative test proving guard scope) | Pending |

### Tier 3: E2E Tests

Deferred — see Tier Skip Rationale below.

### Tier Skip Rationale

- **Unit**: not applicable, not skipped — this DD makes zero new Go code; there is no pure-logic
  function for a Unit tier to target. The "logic" under test is Helm/Sprig template resolution,
  which is inherently only observable as rendered output (Integration tier), consistent with the
  Pyramid Invariant's own definition ("IT proves wiring").
- **E2E**: deferred. No existing E2E harness for this chart exercises live LLM credentials/provider
  dispatch (would require real provider credentials or a mock LLM endpoint — out of both this DD's
  and BR-PLATFORM-008's scope). `helm template` render-validity (Section 13) plus the Operator's
  own already-passing E2E coverage of the *identical* Go dispatch code (`cmd/apifrontend`,
  `cmd/kubernautagent` — unchanged by this DD) is judged sufficient control-objective coverage for
  a Helm-authoring-only change. If a concrete need for chart-level E2E LLM coverage arises later,
  it should be scoped as its own follow-up, not retrofitted here speculatively.

---

## 9. Test Cases

### IT-PLATFORM-LLM-001: Undefined profile reference fails the render

**BR**: BR-PLATFORM-008 FR-1
**Priority**: P0
**Type**: Integration (helm-unittest)
**File**: `charts/kubernaut/tests/llm_profiles_test.yaml`

**Preconditions**: chart values set `kubernautAgent.llmProfileRef: does-not-exist` with no matching entry under `global.llmProfiles`.

**Test Steps**:
1. **Given**: `global.llmProfiles` does not contain a `does-not-exist` key.
2. **When**: `helm template` renders the `kubernaut-agent` Deployment.
3. **Then**: the render fails with a `fail()` error identifying the missing profile name — not a silent empty-value render.

**Expected Results**:
1. `helm unittest` reports the test's `failedTemplate` assertion matches.
2. Error message names the specific undefined profile reference (not a generic Helm error).

**Acceptance Criteria**:
- **Behavior**: render fails, does not silently proceed.
- **Correctness**: failure occurs during the profile-lookup helper call, not a downstream symptom.
- **Accuracy**: error message is specific enough to self-diagnose (states the missing name).

**Dependencies**: none (first test in the suite; GREEN implementation order starts here per the Plan's TDD phase mapping).

### IT-PLATFORM-LLM-008: Multi-secret-mount dedup (#1731 guard lifted by #1861)

**BR**: BR-PLATFORM-008 Success Criteria 3 & 4
**Priority**: P0
**Type**: Integration (helm-unittest)
**File**: `charts/kubernaut/tests/llm_profiles_test.yaml`

**Amendment (kubernaut#1861, 2026-08-02)**: the #1731 `fail()` guard
described below was removed once `cmd/apifrontend`'s two severityTriage
Vertex constructors stopped depending on the shared ambient
`GOOGLE_APPLICATION_CREDENTIALS` env var (see DD-PLATFORM-007's Addendum).
The AF-both-vertex_ai-different-secrets case now asserts a **successful**
render with two independent mounts/`apiKeyFile`s instead of a
`failedTemplate`. Original (historical) preconditions/steps below still
apply for the KA `phaseModels`/shared-secret dedup mechanics, which are
unaffected by this amendment.

**Preconditions**: base profile `primary` (`credentialsSecretName: llm-credentials-primary`); `phaseModels.rca` -> `primary` (same secret); `phaseModels.validation` -> `gcp-profile` (`vertex_ai`, different secret); AF `severityTriage.llmProfileRef` -> a second `vertex_ai` profile with a *different* `credentialsSecretName` than AF's main resolved profile.

**Test Steps**:
1. **Given**: the value set above.
2. **When**: `helm template` renders the `kubernaut-agent` and `apifrontend` Deployments.
3. **Then**: `rca` inherits the base mount (no dedicated volume); `validation` gets its own mount with `credentials.json` (vertex_ai convention); the AF render succeeds with both profiles' own dedicated mounts and `apiKeyFile` values, and no static `GOOGLE_APPLICATION_CREDENTIALS` env var (kubernaut#1861 — previously this render failed with the #1731 guard naming both conflicting profile refs).

**Expected Results**:
1. KA Deployment has exactly 2 volumes (base + `validation`'s), not 3.
2. `validation`'s mount path uses `credentials.json`, not `api_key`.
3. AF's render fails (`fail()`), not a silently-inconsistent successful render.
4. A control case with the identical dual-vertex_ai shape on KA's `phaseModels` (not AF) renders successfully — proving the guard's AF-only scope (R3 mitigation).

**Acceptance Criteria**:
- **Behavior**: dedup and guard both fire in the documented scope, and only that scope.
- **Correctness**: filenames match the Operator's `api_key`/`credentials.json` provider convention exactly.
- **Accuracy**: volume count is exact, not "at least."

**Dependencies**: `IT-PLATFORM-LLM-004` (phaseModels resolution) and `IT-PLATFORM-LLM-006`/`007` (AF profile resolution) must pass first, since this case composes both.

> Remaining P0 cases (`IT-PLATFORM-LLM-002` through `007`) follow the same Given/When/Then structure
> against the scenarios already itemized in Section 8's table; summarized there per the template's
> guidance to detail P0s and summarize the rest once the pattern is established by the two cases above.

---

## 10. Environmental Needs

### 10.1 Unit Tests

N/A.

### 10.2 Integration Tests

- **Framework**: `helm-unittest` (Helm plugin) — this project's approved exception to the
  Ginkgo/Gomega mandate for Helm-template-only assertions, per DD-PLATFORM-005.
- **Mocks**: none — `helm template` renders real chart output; no external dependency to mock.
- **Location**: `charts/kubernaut/tests/llm_profiles_test.yaml` (new file, naming consistent with the existing 9 suites in that directory).
- **Resources**: none beyond the `helm` binary + `helm-unittest` plugin already used by the existing suite and wired into CI per DD-PLATFORM-005.

### 10.3 E2E Tests

N/A — deferred (Section 8).

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-------------------|---------|
| Helm | matches `charts/kubernaut/Chart.yaml` `kubeVersion`/existing CI pin | Render chart templates |
| helm-unittest plugin | matches existing `charts/kubernaut/tests/` usage (DD-PLATFORM-005 CI pin) | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — all consumption contracts already exist and are already validated in the Go binaries (confirmed during DD-PLATFORM-007 preflight); this is a Helm-chart-only change with zero Go code changes.

### 11.2 Execution Order

1. **Phase 1 (RED)**: write all 8 `IT-PLATFORM-LLM-*` cases against the unmodified chart; confirm each fails for the expected reason (schema rejection or absent behavior).
2. **Phase 2 (GREEN)**, in dependency order: schema fields -> `resolveProfile` helper -> KA wiring -> AF wiring -> secret-mount + #1731 guard — re-running the suite after each sub-step.
3. **Phase 3 (CHECKPOINT W)**: confirm all 8 rows pass through the real `helm template` dispatch path (Section 14).
4. **Phase 4 (REFACTOR)**: consolidate duplicated resolution logic, tidy `fail()` messages — RED-phase tests stay green throughout.
5. **Phase 5 (E2E)**: N/A (deferred, Section 8).

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/testing/DD-PLATFORM-007/TEST_PLAN.md` | Strategy and test design |
| Integration test suite | `charts/kubernaut/tests/llm_profiles_test.yaml` | helm-unittest cases, one per Wiring Manifest row |
| Field-name audit | PR description | Manual cross-check, Section 1.3 |

---

## 13. Execution

```bash
# Full helm-unittest suite (includes the 9 pre-existing suites + new llm_profiles_test.yaml)
helm unittest charts/kubernaut

# This plan's suite only
helm unittest charts/kubernaut -f 'tests/llm_profiles_test.yaml'

# Render-validity gate across representative value sets
helm lint charts/kubernaut
helm template charts/kubernaut --set kubernautAgent.llmProfileRef=primary --set global.llmProfiles.primary.provider=openai ...
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|--------------|------------|-----------|--------|
| `kubernaut.llm.resolveProfile` helper | Every LLM-consuming template | Merged profile fields in rendered YAML | IT-PLATFORM-LLM-001 | Pending |
| KA static SDK config | `helm template`/`helm upgrade` | KA Deployment env/ConfigMap | IT-PLATFORM-LLM-002 | Pending |
| KA `llm-runtime.yaml` | same | KA hot-reload ConfigMap | IT-PLATFORM-LLM-003 | Pending |
| KA `phaseModels` | same | KA hot-reload ConfigMap `phaseModels` key | IT-PLATFORM-LLM-004 | Pending |
| KA `alignmentCheck.llm` | same | KA static ConfigMap `alignmentCheck` block | IT-PLATFORM-LLM-005 | Pending |
| AF `agent.llm` | same | AF ConfigMap `agent.llm` block | IT-PLATFORM-LLM-006 | Pending |
| AF `severityTriage.llm` | same | AF ConfigMap `severityTriage.llm` block | IT-PLATFORM-LLM-007 | Pending |
| Multi-secret mounting + #1731 guard | same | Deployment volumes/volumeMounts | IT-PLATFORM-LLM-008 | Pending |

**Unit tests do NOT count as wiring proof** — N/A here regardless, since there are none (Section 4.3); every row above is proven exclusively by an integration-tier `helm template` assertion.

---

## 15. Existing Tests Requiring Updates

None. Confirmed during DD-PLATFORM-007 preflight (content-level grep, not just filename pattern) that none of the existing 9 `charts/kubernaut/tests/*.yaml` suites reference `kubernautAgent.llm.*` or `alignmentCheck` — no existing assertions depend on the fields being removed.

---

## 16. Pre-Implementation Readiness Audit (AGENTS.md Step 5)

> Performed before RED begins, per the Pre-Implementation Workflow. Only dimensions assessable
> pre-code are scored here; Build/Lint/Regression are re-verified for real in Section 5.4's PASS
> criteria and this plan's `validate` step once GREEN exists — this section is a readiness gate,
> not a substitute for actually running those checks later.

| # | Dimension | Pre-implementation status | Basis |
|---|-----------|---------------------------|-------|
| 1 | Build | N/A pre-code — re-checked in `validate` step | Zero Go changes; `helm template` render-validity is this DD's equivalent, deferred to GREEN |
| 2 | Lint | N/A pre-code — re-checked in `validate` step | `helm lint` deferred to GREEN/Validate; no `golangci-lint` surface (no Go changes) |
| 3 | Unit Tests | N/A, documented not skipped | Section 4.3 / Section 8 Tier Skip Rationale |
| 4 | Integration Tests | **Ready** | 8/8 Wiring Manifest rows have a reserved `IT-PLATFORM-LLM-*` ID (Section 7) |
| 5 | Wiring Verification | **Ready** | CHECKPOINT W is its own gate in the implementation plan, not folded into REFACTOR |
| 6 | BDD Framework | **Ready** | helm-unittest is the DD-PLATFORM-005-approved equivalent for Helm-only changes (Section 4.3) |
| 7 | Test ID Assignment | **Ready** | All 8 IDs assigned in the DD and carried into this plan (Section 7, Section 8) |
| 8 | SOC2/FedRAMP Compliance | **N/A, with a note** | This DD changes deploy-time config assembly only, not runtime audit event emission — no new audit event types or schema changes. The multi-secret-mount design is incidentally AC-6-favorable (each consumer mounts only its own resolved secret; dedup only merges when `credentialsSecretName` is identical, never across distinct secrets) but this DD does not claim a formal control mapping since it emits no audit events. |
| 9 | 100 Go Mistakes | N/A | Zero Go code changes |
| 10 | Business Requirement Satisfaction | **Ready** | BR-PLATFORM-008 FR-1..FR-6 + 2 success criteria all mapped (Section 7) |
| 11 | Regression | **Ready** | Confirmed zero existing helm-unittest cases reference the fields being restructured (content-level grep, Section 15) |
| 12 | Fail-Open Safety | **Ready** | Both `fail()` guards (undefined profile ref, #1731 violation) are designed up front, not deferred. Distinguished explicitly from a fail-open concern: AF `severityTriage.llmProfileRef` absent/disabled falling back to rule-based-only triage is an intentional, documented default (opt-in feature absent), not a masked error — the two are not the same failure mode and are tested separately (IT-PLATFORM-LLM-007's "disabled" case vs. IT-PLATFORM-LLM-001/008's `fail()` cases). |
| 13 | Domain-Specific (K8s safety) | **Ready** | Multi-secret-mount design has no cross-consumer credential leakage path by construction — each consumer's volume is derived solely from its own resolved `credentialsSecretName`; dedup activates only on exact-match, never partial/fuzzy matching |

**Outcome**: cleared to begin RED. Dimensions 1/2 are structurally deferred (nothing to build/lint
yet) and will be re-verified for real once GREEN produces renderable templates — not skipped, just
sequenced correctly.

---

## 17. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-26 | Initial test plan, written before RED phase per AGENTS.md Testing Requirements |
