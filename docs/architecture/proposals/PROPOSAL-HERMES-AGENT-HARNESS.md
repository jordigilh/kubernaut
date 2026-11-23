# PROPOSAL: Replace Custom Investigation Loop with NousResearch Hermes Agent Harness

**Author:** Raghuram Banda  
**Date:** 2026-07-26  
**Status:** Draft (Pending Maintainer Review)  
**Target:** Kubernaut Agent (`cmd/kubernautagent`, `internal/kubernautagent/investigator/`)  
**Related:** DD-HAPI-019-001 (Framework Isolation), PROPOSAL-EXT-005/006 (superseded agent runtimes)

---

## 1. Problem Statement

The Kubernaut Agent maintains a bespoke agentic investigation loop in Go (`internal/kubernautagent/investigator/`, ~2400 lines production code across 14 files, ~30 test files). This custom framework implements:

- ReAct-pattern conversation loop (`investigator_loop.go`)
- Multi-phase orchestration: RCA → Workflow Discovery (`investigator_phases.go`, `investigator_rca.go`, `investigator_workflow_selection.go`)
- Tool dispatch with sanitization and summarization (`investigator_tools.go`)
- Validation gates with retry/self-correction (`investigator_gates.go`)
- Anomaly detection (`anomaly.go`)
- Token accumulation and context management (`token_accumulator.go`)
- Per-phase LLM client resolution (`phase_resolver.go`)

**The maintenance burden of this custom framework is significant:**

1. **LLM response parsing fragility** — The `"no JSON found in response"` failures observed in production (see agent logs 2026-07-23) are a direct consequence of hand-rolled response parsing. Each new LLM provider or model revision risks introducing format incompatibilities that require custom mitigation code.

2. **Sequential tool execution** — The current loop executes tools one-at-a-time. Parallel tool execution (which LLMs increasingly request via parallel function calls) is not supported, limiting investigation speed.

3. **No subagent delegation** — Complex incidents requiring multi-hypothesis exploration cannot spawn specialist sub-investigations. The current architecture is single-threaded-single-agent.

4. **Duplicated concerns** — Context window management, iteration budgeting, error classification, model failover, streaming, and interrupt handling are all implemented bespoke when open-source agent runtimes have solved these problems with significantly more testing surface area.

5. **Framework lock-in** — DD-HAPI-019-001 explicitly chose "framework isolation" to avoid depending on langchaingo or similar Go agent libraries. The result is a from-scratch implementation that must be maintained indefinitely as LLM APIs evolve.

---

## 2. Proposal

Replace the custom investigation loop (`internal/kubernautagent/investigator/`) with [NousResearch Hermes Agent](https://github.com/NousResearch/hermes-agent) as the agentic runtime harness, while retaining all Kubernaut-specific business logic (K8s tools, enrichment, audit, prompts, session management, safety mechanisms) as Hermes tool registrations and hooks.

**Hermes Agent** (218K GitHub stars, Apache-2.0 license) is a production-grade agent runtime that provides:

- Battle-tested synchronous conversation loop (`agent/conversation_loop.py`, ~4900 lines)
- OpenAI function-calling compatible tool registry with auto-discovery
- `IterationBudget` with refunds for cheap operations (smarter than flat `maxTurns`)
- Concurrent tool execution (up to 8 workers)
- Subagent delegation (`delegate_task`) with independent budgets and timeouts
- Progressive tool loading via BM25-based tool search (scales to 100+ tools)
- Error classification with model failover (`classify_api_error` → `FailoverReason`)
- Non-blocking interrupt system for cancellation
- Docker/SSH/serverless isolated execution environments
- Session persistence (SQLite, adaptable to external stores)
- Native multi-model support (OpenAI, Anthropic, Ollama, any OpenAI-compatible endpoint)

---

## 3. Architecture

### 3.1 Current Architecture (What Changes)

```
cmd/kubernautagent/main.go
  → buildLLMClients()                    # Custom LLM client construction
  → buildCoreServices()                  # Tools, enrichment, audit
  → buildInvestigationRunner()           # The custom Investigator
      → investigator.New(Config{...})    # ← THIS IS WHAT HERMES REPLACES
  → server.NewHandler(mgr, runner, ...)  # HTTP API (stays)
```

### 3.2 Proposed Architecture

```
cmd/kubernautagent/main.go
  → buildCoreServices()                  # Tools, enrichment, audit (unchanged)
  → buildHermesAgent()                   # NEW: Configure Hermes AIAgent
      → Register K8s tools with Hermes registry
      → Register workflow catalog tools
      → Configure IterationBudget, model, prompts
      → Wire audit hooks
  → server.NewHandler(mgr, hermesAdapter, ...)  # HTTP API (adapter pattern)
```

### 3.3 Component Mapping

| Current (Go, custom) | Proposed (Hermes) | Change Type |
|---|---|---|
| `investigator_loop.go` — ReAct loop | `agent/conversation_loop.py` — Hermes core loop | **Replaced** |
| `investigator_tools.go` — tool dispatch | `model_tools.handle_function_call()` | **Replaced** |
| `investigator_phases.go` — Phase 1→2 routing | Hermes session config + sentinel tools | **Replaced** |
| `phase_resolver.go` — per-phase model | Hermes model config per session | **Replaced** |
| `investigator_gates.go` — validation gates | Hermes post-hooks on `submit_result` tool | **Adapted** |
| `anomaly.go` — repeated failure detection | `IterationBudget` + custom `check_fn` | **Adapted** |
| `token_accumulator.go` — token counting | Built into Hermes (tracked natively) | **Removed** |
| `investigation_metrics.go` — Prometheus metrics | Hermes metrics hook → existing metrics registry | **Adapted** |
| `pkg/kubernautagent/llm/` — LLM client abstraction | Hermes native model support (OpenAI, Anthropic, Ollama) | **Replaced** |
| `pkg/kubernautagent/tools/k8s/` — K8s investigation tools | **Same logic**, re-registered with `registry.register()` | **Ported** (thin wrapper) |
| `pkg/kubernautagent/tools/prometheus/` — Prometheus tools | **Same logic**, re-registered | **Ported** |
| `internal/kubernautagent/enrichment/` — pre-investigation context | Called before Hermes session start (unchanged) | **Unchanged** |
| `internal/kubernautagent/prompt/` — system prompts | Fed to Hermes as system message config | **Unchanged** |
| `internal/kubernautagent/audit/` — DataStorage audit trail | Hermes tool execution hook → existing audit emitter | **Adapted** |
| `internal/kubernautagent/session/` — HTTP session management | Adapter: Hermes session ↔ Kubernaut session store | **Adapted** |
| `internal/kubernautagent/mcp/` — interactive mode | Maps to Hermes gateway/interrupt system | **Adapted** |
| `internal/kubernautagent/alignment/` — shadow agent | Options below (Section 5) | **TBD** |

### 3.4 Interface Contract (Unchanged)

The AI Analysis controller's interface to the agent does NOT change:

```
POST /api/v1/incident/analyze
  Request:  IncidentRequest (signal context, remediation_id, interactive flag)
  Response: 202 Accepted {session_id}

GET /api/v1/incident/session/{id}/result
  Response: InvestigationResult (RCA + selected workflow + parameters)
```

A thin Go HTTP server remains as the adapter between the K8s-native HTTP API and the Hermes Python runtime. This adapter:
1. Receives the HTTP request (Go)
2. Launches a Hermes session with the appropriate tools and prompt (Go → Python bridge)
3. Polls for completion
4. Maps Hermes output to `InvestigationResult` schema
5. Returns to AI Analysis controller

---

## 4. Tool Migration

Each existing Kubernaut tool implements a 4-method interface:

```go
// Current (Go)
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage  // OpenAI function-calling JSON schema
    Execute(ctx context.Context, args json.RawMessage) (string, error)
}
```

Hermes uses an equivalent registration:

```python
# Proposed (Python)
from tools.registry import registry

registry.register(
    name="get_pod_status",
    toolset="kubernetes",
    schema={
        "name": "get_pod_status",
        "description": "Get status of pods matching a label selector or name in a namespace",
        "parameters": {
            "type": "object",
            "properties": {
                "namespace": {"type": "string", "description": "Kubernetes namespace"},
                "name": {"type": "string", "description": "Pod name or prefix"},
            },
            "required": ["namespace"]
        }
    },
    handler=handle_get_pod_status,
    check_fn=lambda: True,  # Always available
)

def handle_get_pod_status(args, **kwargs):
    """Uses Python kubernetes client — same logic as current Go implementation."""
    from kubernetes import client, config
    config.load_incluster_config()
    v1 = client.CoreV1Api()
    pods = v1.list_namespaced_pod(namespace=args["namespace"], ...)
    return format_pod_status(pods)
```

**Tools to port (8 production tools + 3 catalog tools):**

| Tool | Current location | Complexity |
|------|-----------------|------------|
| `get_pod_status` | `pkg/kubernautagent/tools/k8s/pod_status.go` | Low |
| `describe_resource` | `pkg/kubernautagent/tools/k8s/describe.go` | Medium |
| `get_events` | `pkg/kubernautagent/tools/k8s/events.go` | Low |
| `get_pod_logs` | `pkg/kubernautagent/tools/k8s/logs.go` | Low |
| `get_node_metrics` | `pkg/kubernautagent/tools/k8s/metrics.go` | Low |
| `prometheus_query` | `pkg/kubernautagent/tools/prometheus/query.go` | Low |
| `alertmanager_query` | `pkg/kubernautagent/tools/alertmanager/query.go` | Low |
| `get_resource_context` | `internal/kubernautagent/tools/custom/resource_context.go` | Medium |
| `list_available_actions` | `internal/kubernautagent/tools/custom/catalog.go` | Medium (DataStorage HTTP client) |
| `list_workflows` | `internal/kubernautagent/tools/custom/catalog.go` | Medium |
| `get_workflow` | `internal/kubernautagent/tools/custom/catalog.go` | Medium |

Estimated porting effort: ~500-700 lines of Python total. The K8s API calls are simple get/list operations with response formatting.

---

## 5. Shadow Agent / Alignment

The current shadow agent (`internal/kubernautagent/alignment/`) runs a parallel LLM evaluator on every investigation step. Three options for Hermes integration:

**Option A: Hermes post-hook (simplest)**
Register a post-execution hook in Hermes that sends each tool call + LLM response to the existing alignment `Evaluator` (running as a Go sidecar or gRPC service). Hermes's hook system (`pre_hooks`/`post_hooks` in tool dispatch) natively supports this.

**Option B: AuthBridge sidecar (v1.7 pattern)**
Run the shadow agent as the AuthBridge sidecar (already designed for v1.7 AgenticWorkflow). Intercepts all outbound LLM/tool traffic at the network level. No modification to Hermes required.

**Option C: Defer (accept IterationBudget as sufficient initially)**
Launch without shadow agent. Hermes's `IterationBudget` prevents runaway investigations. Add alignment as a follow-up once the core migration is proven. This is the fastest path to validating the approach.

**Recommendation:** Option C for initial migration, Option B for production hardening.

---

## 6. Go → Python Bridge

The kubernaut-agent pod currently runs a single Go binary. This proposal introduces Python. Three deployment options:

**Option 1: Python-only pod (recommended)**
Replace the Go binary entirely with a Python service (FastAPI/uvicorn) that:
- Exposes the same HTTP API (`/api/v1/incident/analyze`, `/session/{id}/result`)
- Runs Hermes internally
- Uses Python `kubernetes` client for cluster access
- Emits audit events to DataStorage via HTTP

**Option 2: Go wrapper + Python subprocess**
Keep the Go HTTP server. On each investigation request, spawn Hermes as a subprocess, pass signal context via stdin/env, collect result from stdout. Avoids rewriting the HTTP layer.

**Option 3: Sidecar pattern**
Go container handles HTTP API + session management. Python container runs Hermes. Communicate via localhost gRPC or Unix socket.

**Recommendation:** Option 1. Clean separation. The Go HTTP server adds no value over FastAPI for this use case, and eliminates the multi-process coordination complexity of Options 2/3.

---

## 7. What Gets Deleted

Upon completion, the following packages are removed from the codebase:

```
internal/kubernautagent/investigator/   # 14 production files, 30 test files — the entire custom loop
pkg/kubernautagent/llm/                 # LLM client abstraction (Hermes handles this)
pkg/kubernautagent/llm/anthropicfamily/ # Anthropic SDK wrapper
pkg/kubernautagent/llm/openai/          # OpenAI-compat wrapper
pkg/kubernautagent/llm/transport/       # OAuth2/scrubbing transport
```

**Lines removed:** ~4000+ (production) + ~6000+ (tests)  
**Lines added:** ~1500 (Python tools + HTTP adapter + Hermes config)  
**Net reduction:** ~8000+ lines of custom framework code

---

## 8. Migration Path

| Phase | Deliverable | Duration | Risk |
|-------|-------------|----------|------|
| **Phase 0** | Spike: Run Hermes with 3 K8s tools, validate tool-call format compatibility with same system prompt | 3 days | None (proof of concept) |
| **Phase 1** | All 11 tools ported. Python HTTP wrapper. End-to-end investigation via AI Analysis controller. No audit, no interactive, no shadow agent. | 1 week | Low |
| **Phase 2** | Audit hook. Session management adapter. `InvestigationResult` schema compliance. Integration tests passing. | 1 week | Medium |
| **Phase 3** | MCP interactive mode bridge (Hermes interrupt → MCP events). Shadow agent as sidecar (Option B). | 1 week | Medium |
| **Phase 4** | Helm chart updated. CI pipeline builds Python image. Old Go investigator code deleted. Release. | 1 week | Low |

**Total: 4 weeks to production parity.**

---

## 9. Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Python performance vs Go for K8s API calls | LLM latency (seconds) dominates tool execution (milliseconds). Benchmark in Phase 0 spike to confirm. |
| Hermes project direction diverges | Apache-2.0 license. Fork if necessary. The integration is via tool registry (stable API surface), not deep framework coupling. |
| Two languages in codebase (Go platform + Python agent) | Clean boundary: Go for CRD controllers/operator, Python for agent only. Same pattern as ML-based systems (Go API + Python model serving). |
| Audit trail compliance | Phase 2 validates every audit event type is emitted identically. Existing integration tests are the acceptance criteria. |
| Shadow agent gap during migration | IterationBudget + approval gates provide safety. Shadow agent added in Phase 3 before production release. |

---

## 10. Success Criteria

1. AI Analysis controller integration tests pass unchanged (same HTTP API contract)
2. Investigation produces identical `InvestigationResult` schema
3. All 11 tools callable with same parameters/responses as current
4. Audit events emitted to DataStorage (same event types, same correlation_id linkage)
5. MCP interactive mode functional (human takeover mid-investigation)
6. Investigation time within 1.5x of current (Hermes overhead acceptable if <50% regression)
7. `internal/kubernautagent/investigator/` and `pkg/kubernautagent/llm/` deleted from tree

---

## 11. Alternatives Considered

| Alternative | Why not |
|---|---|
| **Keep custom loop, fix bugs** | Ongoing maintenance of ~4000 lines of bespoke agent framework. Every new model/provider requires custom integration code. The "no JSON found" parse failures are symptomatic of hand-rolling what agent frameworks already solve. |
| **LangChain/LangGraph (Python)** | Heavier framework with more opinions about state management. Hermes is lighter, more composable, and explicitly designed for tool-heavy agentic loops rather than chain-based orchestration. |
| **CrewAI / AutoGen** | Multi-agent orchestration frameworks. Kubernaut needs a single-agent investigation loop, not multi-agent coordination (that's the Orchestrator's job via CRDs). Over-engineered for this use case. |
| **langchaingo (Go)** | Already evaluated and rejected in DD-HAPI-019-001. Limited tool ecosystem, less mature than Python equivalents, smaller community. |
| **Google ADK (Go)** | Used by API Frontend but tightly coupled to Google's agent protocol. Not suitable for framework-isolated investigation agent (DD-HAPI-019-001 reasoning still applies to ADK coupling). |
| **Build on Kubernaut's v1.7 AgenticWorkflow CRD** | This is for CUSTOMER-authored agents, not for replacing the platform's own investigation engine. Using the same mechanism would create a circular dependency. |

---

## 12. References

- [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent) — 218K stars, Apache-2.0
- [Hermes Agent Architecture](https://nousresearch-hermes-agent.mintlify.app/developer-guide/architecture)
- [Hermes Tool Registry API](https://hermes-agent.nousresearch.com/docs/developer-guide/adding-tools)
- [DD-HAPI-019-001](docs/architecture/decisions/DD-HAPI-019-go-rewrite-design/DD-HAPI-019-001-framework-selection.md) — Framework isolation decision (context for why a bespoke loop was originally chosen)
- [ADR-KA-002](docs/architecture/decisions/ADR-KA-002-agent-security-defense-in-depth.md) — Agent security (AuthBridge/OpenShell design applicable to shadow agent sidecar)
- Current investigator code: `internal/kubernautagent/investigator/` (14 production files)
- Current tool interface: `pkg/kubernautagent/tools/tool.go`
- Current LLM clients: `pkg/kubernautagent/llm/`, `pkg/kubernautagent/llm/anthropicfamily/`, `pkg/kubernautagent/llm/openai/`
