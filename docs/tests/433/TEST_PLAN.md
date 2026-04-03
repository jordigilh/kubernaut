# Test Plan: Kubernaut Agent Go Rewrite (#433)

> **Template**: IEEE 829-2008 + Kubernaut Hybrid v2.0

**Test Plan Identifier**: TP-433-v1.0
**Feature**: Kubernaut Agent — Go reimplementation of HolmesGPT-API with Kubernaut-owned interface architecture. Replaces the Python HolmesGPT SDK dependency with a purpose-built Go investigation engine using LangChainGo, client-go, and a layered security architecture.
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the Go reimplementation of HolmesGPT-API as the Kubernaut Agent service (#433). The rewrite replaces the Python HolmesGPT SDK with a Kubernaut-owned interface architecture that gives the project full control over the agentic investigation loop, tool execution, LLM providers, and security layers.

The test plan covers six functional areas:

1. **Core Engine**: Configuration, session management, audit event emission, MCP skeleton
2. **Investigation Loop**: Two-invocation architecture (RCA then Workflow Selection), prompt rendering, result parsing/validation, enrichment, LangChainGo adapter
3. **Kubernetes Toolset**: 11 K8s tools via client-go replacing kubectl subprocess calls
4. **Prometheus + Custom Tools**: 6 Prometheus tools, 3 workflow discovery tools, resource context, sanitization pipeline, llm_summarize transformer
5. **Security Hardening**: I1 injection stripping, G4 credential scrubbing, I7 anomaly detection, I4 phase-based tool scoping, I5 output validation
6. **E2E Parity + Containerization**: Full investigation flow against mock-llm, API contract, NFRs (image size, CVE scan)

### 1.2 Objectives

1. **Behavioral parity**: Go Kubernaut Agent produces equivalent workflow selections to the Python HAPI for identical investigation scenarios
2. **Zero shell execution**: All K8s tools use client-go, all Prometheus tools use net/http — no subprocess calls
3. **Layered security**: 6 independent defense layers (I1, G4, I3, I4, I5, I7) validated independently and as an integrated system
4. **Fire-and-forget audit**: 8 audit event types emitted at correct investigation points, non-blocking (ADR-038)
5. **Framework isolation**: LangChainGo adapter is ~60 LOC; business logic never imports LangChainGo directly
6. **Image size**: Container image <=80MB (down from ~2.5GB Python image)
7. **Per-tier coverage**: >=80% on unit-testable code, >=80% on integration-testable code

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| E2E test pass rate | 100% | `go test ./test/e2e/kubernautagent/...` (Kind cluster + mock-llm) |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable packages |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable packages |
| Container image size | <=80MB | `docker images kubernaut-agent` |
| Python-inherited CVEs | 0 | `trivy image kubernaut-agent` |
| Data races | 0 | `go test -race ./...` |
| Existing test regressions | 0 | Full Kubernaut test suite passes with Kubernaut Agent merged |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-HAPI-433: Go Language Migration (parent BR)
- BR-HAPI-433-001: Framework Evaluation (LangChainGo selection)
- BR-HAPI-433-002: Kubernetes Toolset (11 tools, client-go)
- BR-HAPI-433-003: Prometheus Toolset (6 tools, net/http)
- BR-HAPI-433-004: Security Requirements (I1, G4, I3, I4, I5, I7)
- BR-HAPI-197: Human Review Flag (needs_human_review on max-turn exhaustion)
- BR-HAPI-211: Credential Scrubbing (17 DD-005 pattern categories)
- DD-HAPI-019: Go Rewrite Design (v1.1)
- DD-HAPI-019-001: Framework Selection (LangChainGo)
- DD-HAPI-019-002: Toolset Implementation (v1.1, includes MCP skeleton)
- DD-HAPI-019-003: Security Architecture (v1.1, layered defense)
- ADR-038: Fire-and-Forget Audit (BufferedAuditStore)
- ADR-041 v3.3: LLM Prompt and Response Contract

### 2.2 Cross-References

- [TP-433-WIR-v1.0](./TP-433-WIR-v1.0.md) — Wiring and adapter integration test plan (33 scenarios)
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- Issue #433: Kubernaut Agent Go rewrite
- Issue #531: Mock LLM Go rewrite (provides mock-llm test infrastructure)
- PoC: `kubernaut-poc-langchaingo/` (validated LangChainGo integration)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | **LangChainGo adapter breaks on framework update** — API change in LangChainGo breaks the ~60 LOC adapter | Investigation flow fails entirely | Medium | IT-KA-433-005..009 | Pin version in go.mod. Adapter is ~60 LOC so updates are trivial. UT-KA-433-004 validates type round-trip. |
| R2 | **Two-invocation architecture loses conversation context** — Phase 3 (Workflow Selection) cannot reference Phase 1 (RCA) findings | Workflow selection ignores RCA, producing wrong remediation | High | IT-KA-433-005..007 | IT-KA-433-007 explicitly verifies conversation history across invocations. CHECKPOINT 2 traces full message flow. |
| R3 | **K8s structured output insufficient for LLM reasoning** — JSON output lacks fields that kubectl text provides | LLM cannot reason about resource state, producing incorrect RCA | Medium | IT-KA-433-014..024 | CHECKPOINT 3 spot-checks Go structured JSON vs Python kubectl text output for same fixtures. |
| R4 | **Prompt injection via tool output bypasses sanitization** — Attacker-controlled content in kubectl logs or Prometheus labels reaches LLM unsanitized | LLM executes attacker instructions, selects wrong workflow | High | UT-KA-433-039..047 | 6-layer defense (I1+G4+I3+I4+I5+I7). CHECKPOINT 4 integration test with known payloads. |
| R5 | **Credential leakage in investigation prompts or audit events** — Tool output containing credentials reaches LLM context or audit store without scrubbing | Security incident — secrets exposed in prompts or audit trail | High | UT-KA-433-048..053 | G4 sanitization in pipeline. CHECKPOINT 4 credential leakage audit verifies no bypass paths. |
| R6 | **Anomaly detection thresholds too aggressive** — Legitimate multi-tool investigations aborted prematurely | Investigation fails for complex incidents that require many tool calls | Medium | UT-KA-433-054..059 | UT-KA-433-058 validates below-threshold calls proceed. CHECKPOINT 4 simulates 15-tool legitimate investigation. Thresholds configurable. |
| R7 | **Mock-llm parity divergence** — Go produces different workflow selections than Python for same scenario | Cannot validate behavioral equivalence, regressions undetected | High | E2E-KA-433-001..002 | E2E parity tests compare Go vs Python results for OOMKilled and CrashLoopBackOff scenarios. |
| R8 | **Audit events not emitted during investigation** — Business operations complete but audit trail is incomplete | Compliance gap — investigation decisions not auditable | Medium | IT-KA-433-009, IT-KA-433-013 | IT-KA-433-009 captures all 8 audit event types as side effects. CHECKPOINT 2 validates completeness. |
| R9 | **Session TTL cleanup races with active investigation goroutine** — TTL timer fires while investigation is still running, causing data race | Panic or corrupted session state | Medium | UT-KA-433-009 | UT-KA-433-009 tests concurrent access with -race. CHECKPOINT 1 leak check verifies goroutine cleanup. |
| R10 | **MCP skeleton prevents clean v1.4 transport integration** — Interface design too rigid for real MCP transport | v1.4 requires breaking changes to MCP interface | Low | UT-KA-433-010..012 | CHECKPOINT 1 reviews MCPToolProvider interface for v1.4 compatibility (lazy discovery, graceful shutdown). |

### 3.1 Risk-to-Test Traceability

| Risk | Primary Tests | Secondary Tests |
|------|--------------|-----------------|
| R1 (LangChainGo adapter) | UT-KA-433-004, IT-KA-433-005 | CHECKPOINT 2 action 7 |
| R2 (context loss) | IT-KA-433-007 | CHECKPOINT 2 actions 1-2 |
| R3 (structured output) | IT-KA-433-014..024 | CHECKPOINT 3 action 3 |
| R4 (injection bypass) | UT-KA-433-039..047 | CHECKPOINT 4 action 1 |
| R5 (credential leakage) | UT-KA-433-048..053 | CHECKPOINT 4 action 7 |
| R6 (aggressive thresholds) | UT-KA-433-054..059 | CHECKPOINT 4 action 4 |
| R7 (parity divergence) | E2E-KA-433-001..002 | CHECKPOINT 2 action 1 |
| R8 (missing audit events) | IT-KA-433-009 | CHECKPOINT 2 action 3 |
| R9 (session TTL race) | UT-KA-433-009 | CHECKPOINT 1 action 5 |
| R10 (MCP interface) | UT-KA-433-010..012 | CHECKPOINT 1 action 4 |

---

## 4. Scope

### 4.1 Features to be Tested

- **Core Engine**: Config parsing, session store/manager, audit event factories, MCP skeleton, `cmd/kubernautagent/main.go` wiring
- **Investigation Loop**: Two-invocation investigator, phase definitions, prompt builder (Go text/template), result parser/validator, self-correction loop, enrichment (owner chain + remediation history)
- **LangChainGo Adapter**: `llm.Client` interface implementation (~60 LOC)
- **Kubernetes Toolset**: 11 tools via client-go (describe, get, events, logs variants, logs_grep)
- **Prometheus Toolset**: 6 tools (instant query, range query, metric names, label values, all labels, metric metadata) + pluggable auth providers
- **Workflow Discovery Tools**: list_available_actions, list_workflows, get_workflow (DD-HAPI-017 three-step protocol)
- **Resource Context Tool**: Owner chain resolution + DataStorage remediation history
- **Sanitization Pipeline**: G4 credential scrubbing, I1 injection stripping, llm_summarize transformer
- **Security Layers**: I4 per-phase tool scoping, I5 output validation, I7 behavioral anomaly detection
- **API Contract**: POST /analyze, GET /session/{id}, GET /result, GET /health, GET /metrics
- **NFRs**: Container image <=80MB, 0 Python-inherited CVEs

### 4.2 Features Not to be Tested

- **LLM provider APIs**: LLM providers (OpenAI, Azure, Ollama) are external dependencies — mocked via `llm.Client` interface
- **DataStorage service internals**: Workflow/remediation-history API behavior is tested by TP-DataStorage, not here
- **Helm chart rendering**: Chart templating is tested by Helm's own framework
- **TLS transport** (#493): Orthogonal; encrypts transport, does not affect investigation logic
- **MCP transport** (v1.4): v1.3 includes only the skeleton; real SSE/stdio transport deferred
- **CaMeL dual-LLM** (v1.4): Architectural defense layer deferred to next release
- **AWS SigV4 Prometheus auth** (v1.4): AWS AMP SigV4 signing deferred; Azure/Vertex providers covered in Phase 7

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Two-invocation architecture | Phase 1 (RCA) and Phase 3 (Workflow Selection) run as separate LLM sessions. Prevents context confusion and enables independent tool scoping per phase. |
| Kubernaut-owned `llm.Client` interface | Business logic never imports LangChainGo directly. Framework can be swapped by changing one ~60 LOC adapter file. |
| Mock `llm.Client` for integration tests | LLM providers are external dependencies. Integration tests use a mock implementing the interface, not real API calls. |
| `fake.NewClientBuilder()` for K8s tools | client-go provides a fake client for testing. Tools are tested against the fake with realistic fixtures. |
| `httptest.Server` for Prometheus tools | Simulates external Prometheus API. Not testing our own HTTP server (that's E2E). |
| Fire-and-forget audit via `BufferedAuditStore` | Investigation never blocks on audit delivery. Errors are logged, not propagated. Consistent with all other Go services (ADR-038). |
| MCP Option C: Wire skeleton, defer transport | Config parsing, interface, and registry integration tested now. Real SSE transport in v1.4. |

---

## 5. Test Approach

### 5.1 Test Framework

- **Framework**: Ginkgo v2 / Gomega (BDD)
- **Test ID format**: `{TIER}-KA-433-{SEQUENCE}` (e.g., `UT-KA-433-001`, `IT-KA-433-014`, `E2E-KA-433-001`)
- **Runner**: `go test` with Ginkgo CLI for parallel execution

### 5.2 Test Tiers

| Tier | What | Mock Strategy | Location |
|------|------|---------------|----------|
| Unit (UT) | Pure logic: config, types, parsers, validators, sanitizers, phase definitions, anomaly detection | No mocks needed (pure functions) or interface mocks for dependencies | `test/unit/kubernautagent/` |
| Integration (IT) | I/O-dependent: investigator loop, session manager, tool execution, enrichment, LangChainGo adapter | Mock `llm.Client`, `fake.NewClientBuilder()` for K8s, `httptest.Server` for Prometheus, mock DataStorage client | `test/integration/kubernautagent/` |
| E2E | Full service in Kind cluster with mock-llm | Real containers, real K8s API, mock-llm service | `test/e2e/kubernautagent/` |

### 5.3 Anti-Pattern Compliance

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md):

| Anti-Pattern | Enforcement |
|--------------|-------------|
| No `time.Sleep()` | All async waits use `Eventually()` / `Consistently()` with configurable timeouts |
| No `Skip()` | All test scenarios implemented or removed; no pending tests |
| No HTTP in integration tests | Integration tests call business logic directly (`investigator.Investigate()`, `tool.Execute()`, `manager.Start()`). HTTP endpoint testing is E2E only. |
| No direct audit testing | Integration tests trigger business operations and verify audit events as side effects |
| No direct metrics testing | Metrics verified as side effects of business operations |
| Mock ONLY external deps | Mock `llm.Client`; use `fake.NewClientBuilder()` for K8s; use `httptest.Server` for Prometheus |

---

## 6. Code Partitioning

### 6.1 Unit-Testable Code (pure logic, no I/O) — target >=80%

| Package | Key Files | Approx Lines |
|---------|-----------|-------------|
| `internal/kubernautagent/config/` | `config.go` | ~150 |
| `internal/kubernautagent/session/store.go` | Session store + TTL | ~120 |
| `internal/kubernautagent/investigator/phases.go` | Phase definitions, tool maps | ~80 |
| `internal/kubernautagent/investigator/anomaly.go` | I7 anomaly detection | ~150 |
| `internal/kubernautagent/prompt/builder.go` | Template rendering | ~100 |
| `internal/kubernautagent/result/parser.go` | JSON parsing | ~80 |
| `internal/kubernautagent/result/validator.go` | Allowlist, bounds, self-correction | ~200 |
| `internal/kubernautagent/audit/emitter.go` | Event factories | ~120 |
| `pkg/kubernautagent/llm/types.go` | ChatRequest/Response types | ~80 |
| `pkg/kubernautagent/tools/registry.go` | Registry, phase scoping | ~150 |
| `pkg/kubernautagent/sanitization/injection.go` | I1 patterns | ~200 |
| `pkg/kubernautagent/sanitization/credential.go` | G4 patterns | ~250 |
| `pkg/kubernautagent/tools/summarizer.go` | Threshold logic | ~60 |
| `pkg/kubernautagent/tools/mcp/config.go` | MCP config parsing | ~40 |
| `pkg/kubernautagent/tools/mcp/stub.go` | Stub provider | ~30 |

### 6.2 Integration-Testable Code (I/O-dependent) — target >=80%

| Package | Key Files | Approx Lines |
|---------|-----------|-------------|
| `internal/kubernautagent/session/manager.go` | Goroutine lifecycle | ~200 |
| `internal/kubernautagent/investigator/investigator.go` | Two-invocation loop | ~300 |
| `internal/kubernautagent/enrichment/enricher.go` | K8s + DS enrichment | ~150 |
| `pkg/kubernautagent/llm/langchaingo.go` | LangChainGo adapter | ~60 |
| `pkg/kubernautagent/tools/k8s/*.go` | 11 K8s tools | ~800 |
| `pkg/kubernautagent/tools/prometheus/*.go` | 6 Prom tools + client | ~500 |
| `pkg/kubernautagent/tools/workflow/*.go` | 3 workflow tools | ~200 |
| `pkg/kubernautagent/tools/resource/context.go` | Resource context | ~100 |
| `pkg/kubernautagent/tools/mcp/registry_integration.go` | MCP wiring | ~60 |

### 6.3 E2E-Testable Code

Full service (`cmd/kubernautagent/main.go` + all packages) deployed in Kind cluster with mock-llm.

---

## 7. BR Coverage Matrix

| Business Requirement | Unit Tests | Integration Tests | E2E Tests |
|---------------------|-----------|------------------|-----------|
| BR-HAPI-433 (parent) | UT-KA-433-001..003, 006..009 | IT-KA-433-001..004, 010..012 | E2E-KA-433-001..009 |
| BR-HAPI-433-001 (Framework) | UT-KA-433-004, 200..204 | IT-KA-433-005..009, 050 | E2E-KA-433-001..002 |
| BR-HAPI-433-002 (K8s Toolset) | UT-KA-433-029..032 | IT-KA-433-014..024 | E2E-KA-433-001..002 |
| BR-HAPI-433-003 (Prometheus) | UT-KA-433-033..034, 194..196 | IT-KA-433-025..032, 040..042 | E2E-KA-433-001..002 |
| BR-HAPI-433-004 (Security) | UT-KA-433-014, 023..027, 035..059 | IT-KA-433-006, 038..039 | E2E-KA-433-001..002 |
| BR-HAPI-197 (Human Review) | UT-KA-433-016, 027 | IT-KA-433-008 | — |
| BR-HAPI-211 (Credential Scrub) | UT-KA-433-048..053 | — | — |
| ADR-038 (Audit) | UT-KA-433-005, 013 | IT-KA-433-009, 013 | — |
| MCP Option C | UT-KA-433-010..012 | — | — |

---

## 8. Test Scenarios

### 8.1 Tier 1: Unit Tests (64 scenarios)

#### Phase 1 — Core Engine (13 UT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| UT-KA-433-001 | Kubernaut Agent loads valid YAML configuration with all required fields | BR-HAPI-433 |
| UT-KA-433-002 | Kubernaut Agent applies correct defaults when optional config fields omitted | BR-HAPI-433 |
| UT-KA-433-003 | Kubernaut Agent rejects invalid config (missing LLM endpoint, invalid max-turns) at startup | BR-HAPI-433 |
| UT-KA-433-004 | ChatRequest/ChatResponse round-trip serialization preserves all fields | BR-HAPI-433-001 |
| UT-KA-433-005 | Audit event factory produces correct event_type and event_category="aiagent" for all 8 event types | ADR-038 |
| UT-KA-433-006 | Session store creates session and retrieves it by ID | BR-HAPI-433 (FR-08) |
| UT-KA-433-007 | Session store returns not-found error for unknown session ID | BR-HAPI-433 (FR-07) |
| UT-KA-433-008 | Session store TTL cleanup removes sessions expired beyond configured duration | BR-HAPI-433 (FR-08) |
| UT-KA-433-009 | Session store concurrent read/write access is data-race-free | BR-HAPI-433 (FR-08) |
| UT-KA-433-010 | MCP config parsing handles multiple server entries with different transports | MCP Option C |
| UT-KA-433-011 | MCP stub provider returns empty tool list and logs warning | MCP Option C |
| UT-KA-433-012 | MCP registry integration registers all tools from provider into Registry | MCP Option C |
| UT-KA-433-013 | Audit best-effort helper does not propagate StoreAudit errors to caller | ADR-038 |

#### Phase 2 — Investigation Loop (15 UT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| UT-KA-433-014 | Phase definitions map K8s+Prom tools to RCA, workflow tools to Discovery, no tools to Validation | BR-HAPI-433-004 (I4) |
| UT-KA-433-015 | Phase transition from RCA to WorkflowDiscovery triggered by investigator | BR-HAPI-433 (FR-01) |
| UT-KA-433-016 | Max-turn exhaustion produces human-review flag in result | BR-HAPI-197 |
| UT-KA-433-017 | Prompt template renders signal context (name, namespace, severity, message) | BR-HAPI-433 (FR-09) |
| UT-KA-433-018 | Prompt template includes enrichment data (owner chain, remediation history) when present | BR-HAPI-433 (FR-09) |
| UT-KA-433-019 | Prompt template handles missing optional enrichment fields without error | BR-HAPI-433 (FR-09) |
| UT-KA-433-020 | Prompt template sanitizes input fields before rendering | BR-HAPI-433-004 (G4) |
| UT-KA-433-021 | Parser extracts InvestigationResult from valid LLM JSON response | BR-HAPI-433 (FR-01) |
| UT-KA-433-022 | Parser returns structured error for malformed JSON | BR-HAPI-433 (FR-01) |
| UT-KA-433-023 | Validator accepts workflow_id present in session allowlist | BR-HAPI-433-004 (I5) |
| UT-KA-433-024 | Validator rejects workflow_id absent from session allowlist | BR-HAPI-433-004 (I5) |
| UT-KA-433-025 | Validator enforces parameter bounds (numeric ranges, string lengths) | BR-HAPI-433-004 (I5) |
| UT-KA-433-026 | Self-correction loop retries up to 3 times on validation failure | BR-HAPI-433-004 (I5) |
| UT-KA-433-027 | Self-correction exhaustion produces human-review flag (BR-HAPI-197) | BR-HAPI-197 |
| UT-KA-433-028 | Enrichment result struct serializes owner chain + labels + remediation history | BR-HAPI-433 |

#### Phase 3 — K8s Toolset (4 UT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| UT-KA-433-029 | All 11 K8s tools satisfy Tool interface (Name, Description, Parameters, Execute) | BR-HAPI-433-002 |
| UT-KA-433-030 | Tool registry registers all tools and resolves by name | BR-HAPI-433-002 |
| UT-KA-433-031 | Registry ToolsForPhase returns correct tool subset per phase | BR-HAPI-433-004 (I4) |
| UT-KA-433-032 | Registry rejects execution of unregistered tool name | BR-HAPI-433-004 (I4) |

#### Phase 4 — Prometheus + Custom Tools (9 UT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| UT-KA-433-033 | Prometheus client config parses URL, headers, timeout, size limit | BR-HAPI-433-003 |
| UT-KA-433-034 | Response exceeding size limit (30000 chars) is truncated with topk() hint | BR-HAPI-433-003 |
| UT-KA-433-194 | `get_series` rejects empty `match` with descriptive error | BR-HAPI-433-003 |
| UT-KA-433-195 | `ClientConfig` defaults `MetadataLimit=100` and `MetadataTimeWindowHrs=1` when zero | BR-HAPI-433-003 |
| UT-KA-433-196 | `get_series` schema declares `start` and `end` as optional properties with `match` required | BR-HAPI-433-003 |
| UT-KA-433-035 | Sanitization pipeline executes G4 before I1 in correct order | BR-HAPI-433-004 |
| UT-KA-433-036 | Pipeline triggers llm_summarize when output exceeds threshold | BR-HAPI-433-002 |
| UT-KA-433-037 | Below-threshold tool output passes through summarizer unchanged | BR-HAPI-433-002 |
| UT-KA-433-038 | Above-threshold tool output triggers secondary LLM summarization call | BR-HAPI-433-002 |

#### Phase 5 — Security (21 UT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| UT-KA-433-039 | I1: Strips imperative instruction patterns ("ignore all previous") | BR-HAPI-433-004 (I1) |
| UT-KA-433-040 | I1: Strips role impersonation ("system:", "assistant:") | BR-HAPI-433-004 (I1) |
| UT-KA-433-041 | I1: Strips workflow selection injection ("select workflow X") | BR-HAPI-433-004 (I1) |
| UT-KA-433-042 | I1: Strips JSON response mimicry blocks | BR-HAPI-433-004 (I1) |
| UT-KA-433-043 | I1: Strips closing tag injection (`</tool_result>`) | BR-HAPI-433-004 (I1) |
| UT-KA-433-044 | I1: Strips prompt escape sequences (boundary markers) | BR-HAPI-433-004 (I1) |
| UT-KA-433-045 | I1: Preserves legitimate tool output not matching injection patterns | BR-HAPI-433-004 (I1) |
| UT-KA-433-046 | I1: Configurable patterns loaded from config (extensible without code changes) | BR-HAPI-433-004 (I1) |
| UT-KA-433-047 | G4+I1: Combined sanitization latency < 10ms per call | BR-HAPI-433-004 |
| UT-KA-433-048 | G4: Scrubs database URL patterns (postgres://, mysql://) | BR-HAPI-211 |
| UT-KA-433-049 | G4: Scrubs API key patterns (Bearer, x-api-key) | BR-HAPI-211 |
| UT-KA-433-050 | G4: Scrubs bearer token patterns | BR-HAPI-211 |
| UT-KA-433-051 | G4: Covers all 17 BR-HAPI-211/DD-005 credential pattern categories | BR-HAPI-211 |
| UT-KA-433-052 | G4: Preserves non-credential content unchanged | BR-HAPI-211 |
| UT-KA-433-053 | G4: Single-call scrubbing latency < 10ms | BR-HAPI-211 |
| UT-KA-433-054 | I7: Per-tool call limit triggers abort at configurable threshold | BR-HAPI-433-004 (I7) |
| UT-KA-433-055 | I7: Total tool call limit triggers abort at threshold | BR-HAPI-433-004 (I7) |
| UT-KA-433-056 | I7: Repeated identical failures (same tool+args) trigger abort | BR-HAPI-433-004 (I7) |
| UT-KA-433-057 | I7: Suspicious argument patterns logged as anomaly | BR-HAPI-433-004 (I7) |
| UT-KA-433-058 | I7: Below-threshold calls proceed normally (no false positives) | BR-HAPI-433-004 (I7) |
| UT-KA-433-059 | I7: Configurable thresholds from config | BR-HAPI-433-004 (I7) |

#### Phase 7 — LLM Provider Extension: Azure/Vertex (5 UT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| UT-KA-433-200 | `New("azure", ...)` creates LangChainGo adapter without error using `openai.WithAPIType(APITypeAzure)` | BR-HAPI-433-001 |
| UT-KA-433-201 | `New("vertex", ...)` creates LangChainGo adapter without error using Vertex AI client | BR-HAPI-433-001 |
| UT-KA-433-202 | Azure adapter uses configured `APIVersion` in LangChainGo client options | BR-HAPI-433-001 |
| UT-KA-433-203 | Vertex adapter uses configured `GCPProjectID` and `GCPRegion` | BR-HAPI-433-001 |
| UT-KA-433-204 | `New("unknown_provider", ...)` returns descriptive error matching "unsupported LLM provider" (regression guard) | BR-HAPI-433-001 |

### 8.2 Tier 2: Integration Tests (40 scenarios)

#### Phase 1 — Core Engine (4 IT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| IT-KA-433-001 | Session manager starts background investigation goroutine and returns session ID | BR-HAPI-433 (FR-08) |
| IT-KA-433-002 | Session manager reports in-progress status via Get during investigation | BR-HAPI-433 (FR-07) |
| IT-KA-433-003 | Session manager delivers completed result after investigation finishes | BR-HAPI-433 (FR-07) |
| IT-KA-433-004 | Session manager captures investigation failure and exposes via Get | BR-HAPI-433 (FR-07) |

#### Phase 2 — Investigation Loop (9 IT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| IT-KA-433-005 | Two-invocation investigation produces RCA summary then workflow selection | BR-HAPI-433 (FR-01) |
| IT-KA-433-006 | Investigation uses K8s+Prom tools in RCA phase, workflow tools in discovery phase | BR-HAPI-433-004 (I4) |
| IT-KA-433-007 | Investigation preserves conversation history across RCA and discovery invocations | BR-HAPI-433 (FR-01) |
| IT-KA-433-008 | Investigation stops at max turns and returns human-review result | BR-HAPI-197 |
| IT-KA-433-009 | Investigation emits all 8 audit event types at correct points (side effect of flow) | ADR-038 |
| IT-KA-433-010 | Enricher resolves owner chain via K8s client (fake) | BR-HAPI-433 |
| IT-KA-433-011 | Enricher fetches remediation history via DataStorage client | BR-HAPI-433 |
| IT-KA-433-012 | Enricher handles partial failure (owner chain fails, history succeeds) | BR-HAPI-433 |
| IT-KA-433-013 | Enricher emits enrichment.completed/failed audit events (side effect) | ADR-038 |

#### Phase 3 — K8s Toolset (11 IT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| IT-KA-433-014 | kubectl_describe produces structured JSON summary of Deployment | BR-HAPI-433-002 |
| IT-KA-433-015 | kubectl_get_by_name returns serialized Pod object | BR-HAPI-433-002 |
| IT-KA-433-016 | kubectl_get_by_kind_in_namespace lists matching objects | BR-HAPI-433-002 |
| IT-KA-433-017 | kubectl_events returns events for target resource | BR-HAPI-433-002 |
| IT-KA-433-018 | kubectl_logs respects TailLines (500) and LimitBytes (256KB) | BR-HAPI-433-002 |
| IT-KA-433-019 | kubectl_previous_logs retrieves previous container logs | BR-HAPI-433-002 |
| IT-KA-433-020 | kubectl_logs_all_containers aggregates logs from all Pod containers | BR-HAPI-433-002 |
| IT-KA-433-021 | kubectl_container_logs retrieves named container logs | BR-HAPI-433-002 |
| IT-KA-433-022 | kubectl_container_previous_logs retrieves named container previous logs | BR-HAPI-433-002 |
| IT-KA-433-023 | kubectl_previous_logs_all_containers retrieves previous from all containers | BR-HAPI-433-002 |
| IT-KA-433-024 | kubectl_logs_grep filters log lines matching pattern | BR-HAPI-433-002 |

#### Phase 4 — Prometheus + Custom Tools (16 IT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| IT-KA-433-025 | execute_prometheus_instant_query returns PromQL query result | BR-HAPI-433-003 |
| IT-KA-433-026 | execute_prometheus_range_query returns time-series data | BR-HAPI-433-003 |
| IT-KA-433-027 | get_metric_names returns available metric names | BR-HAPI-433-003 |
| IT-KA-433-028 | get_label_values returns label values for metric | BR-HAPI-433-003 |
| IT-KA-433-029 | get_all_labels returns all label names | BR-HAPI-433-003 |
| IT-KA-433-030 | get_metric_metadata returns metric help/type info | BR-HAPI-433-003 |
| IT-KA-433-031 | Prometheus client respects timeout configuration | BR-HAPI-433-003 |
| IT-KA-433-032 | Prometheus client sends provider-specific auth headers | BR-HAPI-433-003 |
| IT-KA-433-040 | `get_series` sends `match[]`, `limit`, `start`, `end` to `/api/v1/series` | BR-HAPI-433-003 |
| IT-KA-433-041 | `get_series` defaults `start` to 1h ago and `end` to now when not provided | BR-HAPI-433-003 |
| IT-KA-433-042 | `get_series` injects `_truncated` hint when response contains exactly `MetadataLimit` series | BR-HAPI-433-003 |
| IT-KA-433-033 | list_available_actions queries DataStorage API | BR-HAPI-433 (DD-HAPI-017) |
| IT-KA-433-034 | list_workflows searches DataStorage with criteria | BR-HAPI-433 (DD-HAPI-017) |
| IT-KA-433-035 | get_workflow retrieves specific workflow definition | BR-HAPI-433 (DD-HAPI-017) |
| IT-KA-433-036 | get_resource_context combines K8s owner chain + DS remediation history | BR-HAPI-433 (DD-HAPI-017) |
| IT-KA-433-037 | Summarizer produces shortened output via secondary llm.Client | BR-HAPI-433-002 |

#### Phase 5 — Security (2 IT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| IT-KA-433-038 | Off-phase tool call rejected with error returned as tool message to LLM | BR-HAPI-433-004 (I4) |
| IT-KA-433-039 | Phase transition correctly updates available tool set mid-investigation | BR-HAPI-433-004 (I4) |

#### Phase 7 — LLM Provider Extension: Azure (1 IT)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| IT-KA-433-050 | Azure adapter `Chat` sends request to configured endpoint via `httptest.Server` (verifies API routing with APIType=Azure) | BR-HAPI-433-001 |

### 8.3 Tier 3: E2E Tests (8 scenarios)

| ID | Business Outcome Under Test | BR |
|----|----------------------------|-----|
| E2E-KA-433-001 | Full OOMKilled investigation against mock-llm produces correct workflow selection (parity with Python) | BR-HAPI-433 |
| E2E-KA-433-002 | Full CrashLoopBackOff investigation produces correct workflow selection (parity with Python) | BR-HAPI-433 |
| E2E-KA-433-003 | POST /analyze returns 202 with session ID in response body | BR-HAPI-433 (FR-07) |
| E2E-KA-433-004 | GET /session/{id} returns investigation progress/status | BR-HAPI-433 (FR-07) |
| E2E-KA-433-005 | GET /result returns completed investigation JSON matching API contract | BR-HAPI-433 (FR-07) |
| E2E-KA-433-006 | GET /health returns 200 within 5s of container start | BR-HAPI-433 (NFR) |
| E2E-KA-433-007 | GET /metrics exposes Prometheus metrics (go runtime + request counters) | BR-HAPI-433 (NFR) |
| E2E-KA-433-009 | Trivy scan reports 0 Python-inherited CVEs | BR-HAPI-433 (NFR) |

> **Removed**: E2E-KA-433-008 (container image size <= 80MB) — NFR validated in CI build pipeline, not a functional E2E test.

---

## 9. Test Execution

### 9.1 Commands

```bash
# Unit tests
go test -v -race ./test/unit/kubernautagent/...

# Integration tests
go test -v -race ./test/integration/kubernautagent/...

# E2E tests (requires Kind cluster + mock-llm deployed)
go test -v ./test/e2e/kubernautagent/... -timeout 10m

# Coverage (unit-testable)
go test -coverprofile=coverage-ut.out ./test/unit/kubernautagent/...

# Coverage (integration-testable)
go test -coverprofile=coverage-it.out ./test/integration/kubernautagent/...
```

### 9.2 Dependencies

| Dependency | Version | Purpose |
|-----------|---------|---------|
| LangChainGo | v0.1.14 | LLM framework (pinned) |
| chi v5 | latest | HTTP router |
| client-go | matches cluster version | K8s API client |
| prometheus/client_golang | latest | Metrics exposition |
| Ginkgo v2 | latest | BDD test framework |
| Gomega | latest | BDD assertion library |

---

## 10. Quality Checkpoints

Five checkpoints are integrated into the TDD implementation plan. Each is a hard quality gate — no code proceeds past the checkpoint until all actions pass.

| Checkpoint | Trigger | Key Validations |
|-----------|---------|-----------------|
| CHECKPOINT 0 | After Phase 0, before any code | Doc consistency, LangChainGo version alignment, prompt equivalence, BR coverage, config forward-check |
| CHECKPOINT 1 | After Phase 1-REFACTOR | Build+race, config forward-compat, audit OpenAPI compliance, MCP interface review, goroutine leak check |
| CHECKPOINT 2 (critical) | After Phase 2-REFACTOR | Two-invocation behavioral trace, context preservation, 8 audit events, tool name alignment, self-correction termination |
| CHECKPOINT 3 | After Phase 4-REFACTOR | Registry completeness (21 tools), tool name vs phase alignment, K8s output spot-check, Prometheus size handling, sanitization end-to-end |
| CHECKPOINT 4 | After Phase 5-REFACTOR | Layered security integration, false positive validation, sanitization latency, anomaly threshold validation, full build+lint+race, per-tier coverage |

---

## 11. Approval

| Role | Name | Date | Status |
|------|------|------|--------|
| Test Plan Author | AI Assistant | 2026-03-04 | Draft |
| Technical Lead | — | — | Pending |
| QA Lead | — | — | Pending |

---

**Document Version**: 1.1
**Last Updated**: 2026-03-04
**Change Log**:
- v1.1: Added Phase 7 (LLM Provider Extension: Azure/Vertex) — 5 UT + 1 IT. Added TP-433-WIR cross-reference. Documented SigV4 deferral to v1.4. Updated scenario counts (64 UT, 40 IT, 8 E2E = 112 total).
