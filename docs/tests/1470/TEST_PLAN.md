# Test Plan — #1470: Per-Phase LLM Model Routing

**IEEE 829 Compliant** | **Issue**: [#1470](https://github.com/jordigilh/kubernaut/issues/1470) | **Milestone**: v1.5

## 1. Test Plan Identifier

TP-1470-PER-PHASE-LLM-ROUTING

## 2. Introduction

### 2.1 Purpose

Workflow discovery takes ~60s because it uses the same heavy reasoning model as RCA. This feature adds per-phase LLM model routing so each investigation phase (RCA, workflow discovery, validation) can use a different model. A fast model (e.g. Haiku) for workflow discovery reduces total investigation time by ~74%.

### 2.2 Objectives

1. **Configuration**: `phaseModels` map in `llm-runtime.yaml` specifies per-phase LLM overrides (model, endpoint, provider, etc.)
2. **Input validation (FedRAMP SI-10)**: Invalid phase names and empty overrides are rejected at config load time
3. **Phase resolution**: `PhaseClientResolver` returns the correct LLM client per investigation phase
4. **Audit attribution (FedRAMP AU-2/AU-12)**: Audit events record the actual model name used per phase
5. **Hot-reload (FedRAMP CM-3)**: Phase model changes via ConfigMap are applied at runtime with audit trail
6. **Backward compatibility**: Absent `phaseModels` preserves existing single-model behavior

### 2.3 Business Requirements

- BR-AI-1470: Per-phase model routing for workflow discovery performance optimization

## 3. Features to be Tested

- F-1: `PhaseModels` YAML parsing into `map[string]*LLMOverrideConfig`
- F-2: `Validate()` rejects unknown phase names and empty overrides
- F-3: `EffectivePhaseConfig()` merges phase overrides onto base config
- F-4: `DefaultPhaseResolver.ResolvePhase()` returns phase-specific or default client
- F-5: `PinDecorator` applied to phase-resolved clients (shadow agent chain preserved)
- F-6: `Investigate()` uses `PhaseRCA` for RCA and `PhaseWorkflowDiscovery` for workflow selection
- F-7: `RunInteractiveTurn()`, `RunRCAExtractionFromConversation()` use `PhaseRCA`
- F-8: `RunWorkflowDiscoveryFromRCA()` uses `PhaseWorkflowDiscovery`
- F-9: Audit events contain correct per-phase model name
- F-10: Hot-reload rebuilds, adds, and removes phase-specific clients
- F-11: Nil `PhaseResolver` falls back to legacy single-pin behavior

## 4. Features Not to be Tested

- LLM response quality differences between models
- E2E with real LLM providers (covered by manual acceptance testing)
- Alignment checker correctness (covered by existing shadow agent tests)

## 5. Approach

### Test Pyramid

| Tier | Scope | Count |
|---|---|---|
| Unit | Config parsing, validation, merge logic, resolver edge cases | 11 |
| Integration | Resolver wiring, investigator dispatch, audit attribution, hot-reload chain | 14 |
| E2E | Deferred to E2E suite (requires Kind cluster + real config) | 0 |

### FedRAMP Control Mapping

| Control | Objective | Behavioral Assurance | Test IDs |
|---|---|---|---|
| CM-6 | Configuration settings validated | `PhaseModels` rejects unknown phase names and empty overrides. Only `rca`, `workflow_discovery`, `validation` accepted | UT-AI-1470-001b/c/d |
| SI-10 | Information input validation | Invalid phase config rejected at load time, preventing misconfigured routing from reaching production dispatch | UT-AI-1470-001b/c |
| AU-2 | Auditable events defined | Audit events from `runRCA` and `runWorkflowSelection` contain per-phase `model` name | IT-AI-1470-003f/g |
| AU-12 | Audit generation | Every LLM call emits audit record with `model` field reflecting per-phase routing | IT-AI-1470-003f/g |
| CM-3 | Configuration change control | Hot-reload of `phaseModels` logged with structured fields (phase, old model, new model) | IT-AI-1470-004a |

## 6. Test Cases

### 6.1 Config Parsing and Validation (CM-6 / SI-10)

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| UT-AI-1470-001a | LoadLLMRuntime parses phaseModels from YAML | Parsed map contains `workflow_discovery` key with correct model | CM-6 |
| UT-AI-1470-001b | Validate rejects unknown phase name | Error returned for phase `"foo"` | SI-10 |
| UT-AI-1470-001c | Validate rejects empty override | Error returned when no model/endpoint/provider set | SI-10 |
| UT-AI-1470-001d | Validate accepts valid phase model | No error for `workflow_discovery` with model set | CM-6 |

### 6.2 EffectivePhaseConfig Merge

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| UT-AI-1470-002a | No override returns base unchanged | Output equals input base config | -- |
| UT-AI-1470-002b | Model-only override merges | Runtime model overridden, all other fields preserved | -- |
| UT-AI-1470-002c | Provider + endpoint override merges | Both static and runtime fields overridden | -- |
| UT-AI-1470-002d | Merge does not mutate arguments | Original base/runtime unchanged after call | -- |

### 6.3 Phase Resolver Logic

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| UT-AI-1470-005a | Nil pinDecorator falls back to InstrumentedClient | No panic, client returned | -- |
| UT-AI-1470-005b | Empty phase map returns default | Model name matches default SwappableClient | -- |
| UT-AI-1470-005c | Unknown phase returns default | Phase not in map falls back to default client | -- |

### 6.4 Phase Resolver Wiring

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| IT-AI-1470-001 | Interface satisfied by DefaultPhaseResolver | Compile-time assertion | -- |
| IT-AI-1470-002a | Default client used when no override | Model name matches default SwappableClient | -- |
| IT-AI-1470-002b | Phase-specific client used when override exists | Model name matches phase SwappableClient | -- |
| IT-AI-1470-002c | PinDecorator applied to resolved client | Spy records decorator invocation | -- |
| IT-AI-1470-002d | Set/Remove phase swappable reflected | ResolvePhase returns updated client after mutation | -- |

### 6.5 Investigator Dispatch

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| IT-AI-1470-003a | Investigate resolves PhaseRCA then PhaseWorkflowDiscovery | Spy records both phases in order | -- |
| IT-AI-1470-003b | RunInteractiveTurn resolves PhaseRCA | Spy records PhaseRCA | -- |
| IT-AI-1470-003c | RunWorkflowDiscoveryFromRCA resolves PhaseWorkflowDiscovery | Spy records PhaseWorkflowDiscovery | -- |
| IT-AI-1470-003d | RunRCAExtractionFromConversation resolves PhaseRCA | Spy records PhaseRCA | -- |
| IT-AI-1470-003e | Nil PhaseResolver falls back to legacy behavior | No panic, uses Swappable directly | -- |

### 6.6 Audit Model Attribution (AU-2 / AU-12)

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| IT-AI-1470-003f | RCA audit events contain reasoning model name | `Data["model"]` == "sonnet" in RCA audit events | AU-2/AU-12 |
| IT-AI-1470-003g | Workflow discovery audit events contain fast model name | `Data["model"]` == "haiku" in WD audit events | AU-2/AU-12 |

### 6.7 Hot-Reload Wiring (CM-3)

| ID | Test Case | Success Criteria | Control |
|---|---|---|---|
| IT-AI-1470-004a | Hot-reload swaps phase client with audit trail | ResolvePhase returns new model; log contains phase + old/new model | CM-3 |
| IT-AI-1470-004b | Hot-reload adds new phase override | Phase not previously in resolver now resolves to override | -- |
| IT-AI-1470-004c | Hot-reload removes phase override | Phase reverts to default client | -- |

## 7. Pass/Fail Criteria

- All unit tests pass: `go test ./internal/kubernautagent/config/ --ginkgo.focus="1470"`
- All integration tests pass: `go test ./internal/kubernautagent/investigator/ --ginkgo.focus="1470"`
- All hot-reload tests pass: `go test ./cmd/kubernautagent/ -run AI.1470`
- Zero regressions in existing tests
- Code coverage >= 80% for new code paths
- `go build ./...` succeeds
- Pyramid Invariant satisfied: every component has both UT and IT coverage

## 8. Pyramid Invariant Compliance

| Component | UT (proves logic) | IT (proves wiring) | Status |
|---|---|---|---|
| PhaseModels config parsing + validation | UT-AI-1470-001a/b/c/d | IT-AI-1470-004a | Compliant |
| EffectivePhaseConfig merge | UT-AI-1470-002a/b/c/d | IT-AI-1470-004a | Compliant |
| DefaultPhaseResolver | UT-AI-1470-005a/b/c | IT-AI-1470-002a/b/c/d | Compliant |
| Phase-aware Investigate dispatch | -- | IT-AI-1470-003a/b/c/d/e | Compliant |
| Audit model attribution | -- | IT-AI-1470-003f/g | Compliant |
| Hot-reload phase swap | -- | IT-AI-1470-004a/b/c | Compliant |

## 9. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | UT Test ID | IT Test ID |
|---|---|---|---|---|
| PhaseModels config field | LLMRuntimeConfig parse/validate | config/config.go | UT-AI-1470-001a/b/c/d | IT-AI-1470-004a |
| EffectivePhaseConfig | llmRuntimeReloadCallback + main.go | config/config.go | UT-AI-1470-002a/b/c/d | IT-AI-1470-004a |
| PhaseClientResolver interface | investigator.Config | investigator/phase_resolver.go | -- | IT-AI-1470-001 |
| DefaultPhaseResolver | main.go (wiring) | investigator/phase_resolver.go | UT-AI-1470-005a/b/c | IT-AI-1470-002a/b/c/d |
| Phase-aware Investigate() | Investigate(), RunWorkflowDiscoveryFromRCA() | investigator/investigator.go | -- | IT-AI-1470-003a/b/c/d/e |
| Audit model attribution | runRCA/runWorkflowSelection audit events | investigator/investigator.go | -- | IT-AI-1470-003f/g |
| Hot-reload phase swap | llmRuntimeReloadCallback | llm_builder.go | -- | IT-AI-1470-004a/b/c |

## 10. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-06-20 | Initial test plan |
