# DD-HAPI-019: Go Rewrite Design

**Status**: ✅ Approved
**Decision Date**: 2026-03-04
**Version**: 1.0
**Confidence**: 88%
**Deciders**: Architecture Team, HAPI Team
**Applies To**: HolmesGPT-API (HAPI)

**Related Business Requirements**:
- [BR-HAPI-433: Go Language Migration](../../../requirements/BR-HAPI-433-go-language-migration/BR-HAPI-433-go-language-migration.md)

**Related Design Decisions**:
- [DD-HAPI-019-001: Framework Selection](DD-HAPI-019-001-framework-selection.md)
- [DD-HAPI-019-002: Toolset Implementation](DD-HAPI-019-002-toolset-implementation.md)
- [DD-HAPI-019-003: Security Architecture](DD-HAPI-019-003-security-architecture.md)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-03-04 | Architecture Team | Initial design: Kubernaut-owned interface architecture, component layout, framework isolation pattern |

---

## Context & Problem

### Current State

HAPI is a Python service wrapping the HolmesGPT SDK. The architecture couples HAPI to HolmesGPT's implementation decisions:

```
┌─────────────────────────────────────────────────┐
│                 HAPI (Python)                    │
│                                                  │
│  ┌──────────────────┐  ┌─────────────────────┐  │
│  │ FastAPI endpoints │  │ HolmesGPT SDK       │  │
│  │ (incident.py)     │  │ (agentic loop,      │  │
│  │ (recovery.py)     │  │  tool execution,    │  │
│  └────────┬─────────┘  │  LLM providers)     │  │
│           │             └─────────┬───────────┘  │
│           │                       │               │
│           │          ┌────────────┴────────────┐  │
│           │          │ subprocess.run(shell=T)  │  │
│           │          │ kubectl, helm, jq, etc.  │  │
│           │          └─────────────────────────┘  │
│           │                                       │
│  ┌────────┴─────────┐                            │
│  │ HAPI-custom       │                            │
│  │ toolsets           │                            │
│  │ (workflow_discovery│                            │
│  │  resource_context) │                            │
│  └──────────────────┘                            │
└─────────────────────────────────────────────────┘
```

**Problems**:
1. The agentic loop, tool execution, and LLM providers are all controlled by HolmesGPT SDK — we cannot modify them independently
2. All toolsets execute via `subprocess.run(cmd, shell=True)` — shell injection vector
3. CLI binaries (kubectl, helm, jq) are bundled in the image — ~50MB+ of bloat
4. Python runtime + pip dependencies create the 2.5GB image

### Problem Statement

Design a Go architecture for HAPI that:
- Gives Kubernaut full control over the agentic loop, tool execution, and LLM providers
- Isolates framework-specific code so the LLM framework can be swapped
- Eliminates shell execution entirely
- Preserves the REST API contract

---

## Decision Drivers

1. **Framework isolation**: LLM frameworks evolve rapidly. The architecture must isolate framework code so swapping (e.g., LangChainGo → Eino) requires minimal changes.
2. **Tool scoping control**: Per-turn and per-phase tool scoping requires the investigator loop to be Kubernaut-owned, not delegated to a framework.
3. **Security by design**: Tool output sanitization and behavioral anomaly detection must be embedded in the execution pipeline, not bolted on.
4. **Existing patterns**: Follow Kubernaut's established Go patterns (interfaces, dependency injection, structured logging, ConfigManager).

---

## Decision

### Kubernaut-Owned Interface Architecture

All business logic is Kubernaut-owned. The LLM framework is isolated behind a single interface with a ~60 LOC adapter.

```
┌─────────────────────────────────────────────────────────────────┐
│                       HAPI (Go)                                  │
│                                                                  │
│  ┌─────────────────┐  ┌──────────────────────────────────────┐  │
│  │ HTTP Server      │  │ Kubernaut-Owned Core                 │  │
│  │ (net/http / chi) │  │                                      │  │
│  │                  │  │  ┌──────────────┐                    │  │
│  │ POST /analyze    │──▶  │ Investigator │ (multi-turn loop)  │  │
│  │ GET  /session/id │  │  │              │                    │  │
│  │ GET  /result     │  │  │  per-turn    │                    │  │
│  │ GET  /health     │  │  │  tool scoping│                    │  │
│  │ GET  /metrics    │  │  └──────┬───────┘                    │  │
│  └─────────────────┘  │         │                             │  │
│                        │    ┌────┴────┐                        │  │
│                        │    │         │                        │  │
│                 ┌──────┴────┴──┐  ┌───┴──────────────┐        │  │
│                 │ llm.Client   │  │ tools.Registry    │        │  │
│                 │ (interface)  │  │ (tool dispatch)   │        │  │
│                 └──────┬──────┘  └───┬──────────────┘        │  │
│                        │             │                        │  │
│              ┌─────────┴─────┐   ┌───┴────────────────────┐  │  │
│              │ LangChainGo   │   │ Tool Implementations    │  │  │
│              │ Adapter        │   │                         │  │  │
│              │ (~60 LOC)     │   │ ┌─────────────────────┐ │  │  │
│              └───────────────┘   │ │ K8s (client-go)     │ │  │  │
│                                  │ ├─────────────────────┤ │  │  │
│                                  │ │ Prometheus (net/http)│ │  │  │
│                                  │ ├─────────────────────┤ │  │  │
│                                  │ │ Workflow Discovery   │ │  │  │
│                                  │ │ (DataStorage client) │ │  │  │
│                                  │ ├─────────────────────┤ │  │  │
│                                  │ │ Resource Context     │ │  │  │
│                                  │ │ (client-go + DS)    │ │  │  │
│                                  │ └─────────────────────┘ │  │  │
│                                  └─────────────────────────┘  │  │
│                                                               │  │
│  ┌────────────────────────────────────────────────────────┐   │  │
│  │ Cross-Cutting Concerns                                 │   │  │
│  │ ┌──────────────┐ ┌──────────┐ ┌─────────────────────┐ │   │  │
│  │ │ Sanitizer    │ │ Audit    │ │ Config Manager      │ │   │  │
│  │ │ (I1 + G4)   │ │ Emitter  │ │ (hot-reload)        │ │   │  │
│  │ └──────────────┘ └──────────┘ └─────────────────────┘ │   │  │
│  └────────────────────────────────────────────────────────┘   │  │
└───────────────────────────────────────────────────────────────┘
```

### Package Layout

```
cmd/hapi/
├── main.go                    # Wiring: HTTP server, config, DI

internal/hapi/
├── server/
│   ├── server.go              # HTTP server setup (chi router)
│   ├── handlers.go            # /analyze, /session/{id}, /result, /health, /metrics
│   └── middleware.go          # Auth (TokenReview/SAR), request ID, logging
├── session/
│   ├── manager.go             # Session lifecycle (goroutine-based async)
│   └── store.go               # In-memory session store with TTL cleanup
├── investigator/
│   ├── investigator.go        # Multi-turn agentic loop (framework-agnostic)
│   ├── phases.go              # Phase definitions (RCA, workflow discovery, validation)
│   └── anomaly.go             # Behavioral anomaly detection (I7)
├── prompt/
│   ├── builder.go             # Investigation prompt construction (Go text/template)
│   └── templates/             # .tmpl files for prompts
├── result/
│   ├── parser.go              # LLM result parsing and validation (I5)
│   └── validator.go           # Workflow ID allowlist, parameter bounds, self-correction
└── config/
    └── config.go              # HAPI-specific config (extends shared ConfigManager)

pkg/hapi/
├── llm/
│   ├── client.go              # llm.Client interface (Kubernaut-owned)
│   ├── types.go               # ChatRequest, ChatResponse, Message, ToolCall
│   └── langchaingo.go         # LangChainGo adapter (~60 LOC)
├── tools/
│   ├── registry.go            # Tool registry, per-turn tool scoping
│   ├── tool.go                # tools.Tool interface
│   ├── sanitizer.go           # Tool-output sanitization pipeline (I1)
│   ├── summarizer.go          # llm_summarize transformer
│   ├── k8s/                   # Kubernetes tool implementations (client-go)
│   │   ├── describe.go
│   │   ├── get.go
│   │   ├── events.go
│   │   ├── logs.go
│   │   └── logs_grep.go
│   ├── prometheus/            # Prometheus tool implementations (net/http)
│   │   ├── client.go          # PrometheusClient with auth, timeout, size limits
│   │   ├── query.go           # Instant + range queries
│   │   ├── discovery.go       # Metric names, labels, metadata
│   │   └── provider.go        # AWS AMP SigV4, OpenShift token, auto-discovery
│   ├── workflow/              # Workflow discovery tools (DataStorage client)
│   │   ├── list_actions.go
│   │   ├── list_workflows.go
│   │   └── get_workflow.go
│   └── resource/              # Resource context tool (client-go + DataStorage)
│       └── context.go
└── sanitization/
    ├── credential.go          # G4: BR-HAPI-211 patterns in Go
    └── injection.go           # I1: Prompt injection pattern stripping
```

### Core Interfaces

```go
// pkg/hapi/llm/client.go
type Client interface {
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type ChatRequest struct {
    Messages []Message
    Tools    []ToolDefinition
    Options  ChatOptions
}

type ChatResponse struct {
    Message   Message
    ToolCalls []ToolCall
    Usage     TokenUsage
}
```

```go
// pkg/hapi/tools/tool.go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage  // JSON Schema
    Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

type ToolResult struct {
    Content string
    Error   string
}
```

```go
// internal/hapi/investigator/investigator.go
type Investigator struct {
    llmClient    llm.Client
    toolRegistry *tools.Registry
    sanitizer    *sanitization.Pipeline
    anomaly      *AnomalyDetector
    config       *config.InvestigatorConfig
}

func (inv *Investigator) Investigate(ctx context.Context, req IncidentRequest) (InvestigationResult, error) {
    // Multi-turn loop:
    // 1. Build initial prompt from signal context
    // 2. For each turn:
    //    a. Select tools for current phase (per-phase scoping)
    //    b. Call LLM with messages + phase-appropriate tools
    //    c. If tool calls: execute, sanitize results, append to messages
    //    d. If structured result: validate, return
    //    e. Check anomaly detector (excessive calls, phase violations)
    // 3. On max-turn exhaustion: flag for human review
}
```

---

## Consequences

### Positive Consequences

1. **Framework isolation**: LangChainGo adapter is ~60 LOC. Swapping to Eino or raw openai-go requires changing one file.
2. **Full control**: Kubernaut owns the agentic loop, tool dispatch, sanitization, and anomaly detection. No framework black boxes.
3. **Security embedded**: Sanitization and anomaly detection are in the tool execution pipeline, not optional middleware.
4. **Testability**: Each component is independently testable. `llm.Client` can be mocked for unit tests. Tools can be tested against real client-go fakes.
5. **Consistent patterns**: Follows Kubernaut's existing Go patterns (interfaces, DI, structured logging).

### Negative Consequences

1. **More code to maintain**: Kubernaut owns the investigator loop, tool dispatch, and sanitization — previously delegated to HolmesGPT SDK.
   - **Mitigation**: The code is simpler than HolmesGPT's Python implementation because it's purpose-built for Kubernaut's use case, not a general-purpose framework.

2. **LangChainGo breaking changes**: Framework updates could break the adapter.
   - **Mitigation**: Pin version in `go.mod`. The adapter is ~60 LOC so updates are trivial.

---

## Compliance

| Requirement | Status | Notes |
|---|---|---|
| BR-HAPI-433 | ✅ | Core architecture for Go rewrite |
| BR-HAPI-211 | ✅ | Credential scrubbing in `pkg/hapi/sanitization/credential.go` |
| BR-HAPI-197 | ✅ | Human review flag preserved in `internal/hapi/result/validator.go` |
| DD-HAPI-017 | ✅ | Three-step workflow discovery preserved in `pkg/hapi/tools/workflow/` |

---

## Validation Strategy

1. **PoC validation**: LangChainGo PoC (`kubernaut-poc-langchaingo/`) validated the full investigation flow against mock-llm
2. **Unit tests**: Each package independently testable with mocked dependencies
3. **Integration tests**: Full investigation flow against mock-llm in Kind cluster
4. **E2E tests**: Same mock-llm scenarios as current Python HAPI — results must match
5. **Image size**: Verify ≤80MB with `docker images`
6. **CVE scan**: Verify 0 Python-inherited CVEs with Trivy

---

## References

- [BR-HAPI-433: Go Language Migration](../../../requirements/BR-HAPI-433-go-language-migration/)
- [DD-HAPI-017: Three-Step Workflow Discovery](../DD-HAPI-017-three-step-workflow-discovery-integration.md)
- [DD-HAPI-005: LLM Input Sanitization](../DD-HAPI-005-llm-input-sanitization.md)
- [ADR-061: DD Template Standard](../ADR-061-design-decision-template-standard.md)

---

**Document Version**: 1.0
**Last Updated**: 2026-03-04
