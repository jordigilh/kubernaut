# Test Plan: Context-Aware Agent Prompt (Issue #1275)

## 1. Test Plan Identifier

TP-AF-1275-v1.0

## 2. References

- Issue: https://github.com/jordigilh/kubernaut/issues/1275
- BR: BR-API-1275 — AF A2A prompt should be context-aware of deployment namespace and kubernaut.ai CRDs
- FedRAMP Controls: CM-6 (Configuration Management), SC-7 (Boundary Protection), SI-10 (Information Input Validation)
- ADR: ADR-021 (SAR-based tool authorization)
- Related: #1276 (per-request personalization), #1277 (config-driven prompt)

## 3. Introduction

This test plan validates that the AF agent prompt is enriched with deployment-specific context (namespace, CRD types) and that the GVK static table resolves kubernaut.ai CRD kinds without REST mapper dependency.

## 4. Test Items

| Component | Version | Description |
|---|---|---|
| `pkg/shared/k8s/gvk.go` | v1.5.0 | GVK static resolution table |
| `pkg/apifrontend/agent/prompt.go` | v1.5.0 | BuildInstruction() with namespace/CRD context |
| `pkg/apifrontend/agent/prompt.txt` | v1.5.0 | Intent-grouped tool documentation |
| `cmd/apifrontend/main.go` | v1.5.0 | Wiring BuildInstruction into agent config |

## 5. Software Risk Issues

| Risk | Impact | Mitigation |
|---|---|---|
| Prompt text change breaks existing test UT-AF-131-004 (no internal names) | Medium | Update test to skip "Deployment Context" section |
| Empty namespace in minimal configs causes empty context section | Low | Fallback to "default" namespace |
| GVK table stale when new CRDs added | Low | REST mapper fallback still works |

## 6. Features to be Tested

### 6.1 GVK Static Resolution (CM-6)

- F-1: All 9 kubernaut.ai CRD kinds resolve to `kubernaut.ai/v1alpha1` without REST mapper
- F-2: Resolution does not require a non-nil REST mapper
- F-3: Existing core kinds (Deployment, Pod, etc.) are not regressed

### 6.2 BuildInstruction (SC-7, CM-6)

- F-4: Output contains embedded prompt (core prompt immutable)
- F-5: Output contains deployment namespace in "Deployment Context" section
- F-6: Output contains kubernaut.ai CRD types for agent awareness
- F-7: Empty namespace falls back gracefully (no panic, uses "default")

### 6.3 Prompt Restructure (CM-6)

- F-8: Tool inventory grouped by user intent
- F-9: Natural language alias mappings present for each intent group
- F-10: All 28 ADK tools covered in at least one intent group

### 6.4 Main Wiring (SC-7)

- F-11: `NewRootAgent` receives instruction from `BuildInstruction(namespace)` not `DefaultTestConfig()`
- F-12: Production config uses `session.namespace` value

## 7. Features Not to be Tested

- LLM response quality (subjective, not deterministic)
- E2E prompt effectiveness (covered by manual acceptance testing)
- Config-driven prompt augmentation (deferred to #1277)

## 8. Approach

### Test Pyramid

| Tier | Scope | Count |
|---|---|---|
| Unit | GVK resolution, BuildInstruction, prompt content assertions | 20+ |
| Integration | Full agent construction with BuildInstruction in httptest server | 2 |
| E2E | Agent responds with namespace-aware context in Kind cluster | 1 |

### FedRAMP Control Mapping

| Test ID | Control | Behavior |
|---|---|---|
| UT-K8S-1275-001..009 | CM-6 | CRD kinds resolve to correct GVK |
| UT-AF-1275-010 | SC-7 | Core prompt immutable in BuildInstruction output |
| UT-AF-1275-011 | CM-6 | Namespace injected from config |
| UT-AF-1275-012 | SI-10 | Empty namespace handled safely |
| UT-AF-1275-013 | CM-6 | CRD types listed in deployment context |
| UT-AF-1275-014..019 | CM-6 | Intent groups contain correct tool mappings |
| IT-AF-1275-001 | SC-7 | Agent constructed with BuildInstruction uses namespace |
| IT-AF-1275-002 | CM-6 | Agent card skills match tools in prompt |

## 9. Item Pass/Fail Criteria

- All unit tests pass with `go test ./pkg/shared/k8s/... ./pkg/apifrontend/agent/...`
- Zero regressions in existing GVK and prompt tests
- Code coverage >= 80% for new code paths
- `go build ./...` succeeds
- `golangci-lint run` produces no new findings

## 10. Test Deliverables

- `pkg/shared/k8s/gvk_test.go` — 9 new DescribeTable entries
- `pkg/apifrontend/agent/prompt_test.go` — BuildInstruction specs + updated existing tests
- `test/integration/apifrontend/prompt_context_test.go` — IT specs
- `test/e2e/apifrontend/prompt_context_test.go` — E2E spec (if applicable)

## 11. Environmental Needs

- Unit: `go test` (no external deps)
- Integration: `httptest` server with mock LLM
- E2E: Kind cluster with AF deployed, mock-LLM ConfigMap

## 12. Schedule

| Phase | Duration |
|---|---|
| RED (failing tests) | 15 min |
| GREEN (implementation) | 20 min |
| REFACTOR (quality) | 10 min |
| Checkpoint audit | 5 min |
