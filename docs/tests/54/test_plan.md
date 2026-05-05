# Test Plan: A2A Interaction Lifecycle — HTTP Mux Wiring (Issue #54)

**Test Plan Identifier:** TP-AF-054
**Version:** 1.0
**Date:** 2026-05-05
**Author:** AI Assistant

---

## 1. Introduction

This test plan covers PR5: HTTP mux wiring that connects the ADK root agent (PR3) and CRD session service (PR4) into the HTTP server. It validates A2A JSON-RPC endpoints, MCP Streamable HTTP endpoints, Agent Card discovery, HTTP observability middleware, model wiring, and per-request JWT delegation.

## 2. Test Items

- `internal/handler/router.go` -- HTTP mux with route registration and auth boundaries
- `internal/handler/middleware.go` -- HTTP request metrics (duration, status code)
- `internal/handler/health.go` -- `/healthz`, `/readyz` probes
- `internal/handler/a2a.go` -- A2A JSON-RPC handler wrapping ADK executor
- `internal/handler/mcp.go` -- MCP Streamable HTTP handler with tool bridge
- `internal/handler/mcptools.go` -- ADK tool to MCP tool adapter
- `internal/handler/agentcard.go` -- `/.well-known/agent-card.json` static handler
- `internal/launcher/launcher.go` -- ADK executor + model + session service assembly
- `internal/launcher/model.go` -- Claude Sonnet 4.6 via Vertex AI model creation
- `cmd/apifrontend/main.go` -- Final wiring of all components

## 3. Features to Be Tested

| Feature | Source | Priority |
|---------|--------|----------|
| A2A `message/send` creates task and returns task ID | Issue #54, A2A spec v0.3 | P0 |
| A2A `tasks/get` returns current task state | Issue #54, A2A spec | P0 |
| A2A `tasks/cancel` stops active task | Issue #54, A2A spec | P0 |
| A2A `message/stream` streams SSE events | Issue #54, A2A spec | P0 |
| `present_decision` triggers `input-required` state | Issue #54, ADK Findings | P0 |
| MCP `initialize` returns server info | MCP spec 2025-03-26 | P0 |
| MCP `tools/list` returns all 14 tools with schemas | Issue #42, MCP spec | P0 |
| MCP `tools/call` dispatches to correct handler | MCP spec | P0 |
| MCP RBAC filtering per user role | Issue #3, RBAC config | P0 |
| Agent Card at `/.well-known/agent-card.json` | Issue #28, A2A spec | P0 |
| Agent Card includes all 14 skills | Issue #28 | P1 |
| Agent Card declares auth requirements | Issue #28 | P1 |
| HTTP metrics `af_http_requests_total` | ARCHITECTURE.md §7 | P0 |
| HTTP metrics `af_http_request_duration_seconds` | ARCHITECTURE.md §7 | P0 |
| `/healthz` returns 200 without auth | K8s probe convention | P0 |
| `/readyz` returns 503 when JWKS breaker open | K8s probe convention | P0 |
| Auth boundary: `/a2a/invoke` requires bearer token | Security model | P0 |
| Auth boundary: `/mcp` requires bearer token | Security model | P0 |
| Model wiring: Claude Sonnet 4.6 via Vertex AI | Issue #1, ADK plan | P0 |
| Per-request JWT delegation for KA calls | ARCHITECTURE.md §9 | P0 |
| Graceful shutdown cancels active A2A tasks | Ops requirement | P1 |
| CRDSessionService wired as ADK session service | PR4, Issue #56 | P0 |

## 4. Approach

- **Framework:** Ginkgo v2 / Gomega (per ADR-015)
- **Naming:** `UT-AF-2XX-NNN` where 2XX is the component (200=handler, 210=A2A, 220=MCP, 230=card)
- **Isolation:** A2A tested with mock LLM agent (no real API calls). MCP tested with `mcp.StreamableClientTransport`. Agent Card tested with JSON schema validation.
- **TDD:** Red/Green/Refactor with 9-category checkpoint audits per component
- **Infrastructure:** `httptest.NewServer` for HTTP round-trips, `adksession.InMemoryService()` as delegate in tests

## 5. Pass/Fail Criteria

- All ~46 specs pass with `ginkgo -race`
- Coverage >= 80% per new package (`internal/handler`, `internal/launcher`)
- `golangci-lint run ./...` produces 0 errors
- `gosec -severity medium -confidence medium ./...` produces 0 issues
- No `panic()` in production code
- `go mod tidy` clean
- A2A round-trip: `message/send` -> working -> completed (mock LLM)
- MCP round-trip: `initialize` -> `tools/list` -> `tools/call` (mock tools)
- Agent Card: validates against A2A JSON Schema

## 6. Test Deliverables

| File | Specs | Component |
|------|-------|-----------|
| `internal/handler/handler_test.go` | 10 | Router, health, auth boundary, HTTP metrics |
| `internal/handler/a2a_test.go` | 8 | A2A JSON-RPC handler, callbacks |
| `internal/launcher/launcher_test.go` | 6 | Model wiring, executor creation, JWT delegation |
| `internal/handler/mcp_test.go` | 12 | MCP tools/list, tools/call, streaming, RBAC |
| `internal/handler/agentcard_test.go` | 7 | Agent Card schema, skills, auth, provider |
| `cmd/apifrontend/main_test.go` | 3 | Main wiring, graceful shutdown |

## 7. Environmental Needs

- Go 1.25+
- `google.golang.org/adk` v1.2.0
- `github.com/modelcontextprotocol/go-sdk` v1.6.0
- `github.com/a2aproject/a2a-go` v0.3.13+ (transitive via ADK)
- `github.com/Alcova-AI/adk-anthropic-go` v0.1.15
- No external services required (all mocked in unit tests)

## 8. Schedule

| Phase | Scope | Specs | Gate |
|-------|-------|-------|------|
| Phase 1-3 (Handler) | Router, middleware, health | 10 | Checkpoint A-B |
| Phase 4-6 (A2A) | Executor, model, callbacks | 14 | Checkpoint C-D |
| Phase 7-9 (MCP) | Server, tool bridge, RBAC | 12 | Checkpoint E-F |
| Phase 10-12 (Card+Wiring) | Agent Card, main.go | 10 | Checkpoint G-H |

## 9. Risks and Mitigations

| Risk | Severity | Mitigation |
|------|----------|------------|
| `runner.Runner` doesn't implement `adka2a.Runner` | High | Use `ExecutorConfig.RunnerConfig` (executor wraps internally) |
| MCP SDK doesn't propagate request context to tools | High | Validated via spike: ctx propagates through `getServer(r)` |
| Per-request tool rebuild is expensive | Medium | Share agent+tools; use `SchemaCache` for MCP |
| `AutoCreateSession` required for A2A executor | Medium | Validated: `runner.Config{AutoCreateSession: true}` works |
| A2A `a2a-go` not in go.sum | Low | `go get google.golang.org/adk/server/adka2a` pulls transitively |
