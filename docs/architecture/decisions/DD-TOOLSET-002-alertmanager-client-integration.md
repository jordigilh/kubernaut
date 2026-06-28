# DD-TOOLSET-002: Alertmanager Client Integration for KA Investigation Tools

**Date**: 2026-06-27
**Status**: Proposed
**Confidence**: 90%
**Decision Type**: Architecture — External Service Integration
**Issue**: [#1507](https://github.com/jordigilh/kubernaut/issues/1507)

---

## Context

Issue #1507 introduces `get_alerts` and `get_silences` tools to the Kubernaut Agent investigator toolset. These tools query the Alertmanager HTTP API (`/api/v2/alerts`, `/api/v2/silences`) and return results for LLM consumption during the RCA phase.

KA already has a Prometheus client (`pkg/kubernautagent/tools/prometheus/client.go`) that follows a raw-JSON pass-through pattern: the client performs HTTP GET requests and returns the response body as a string, truncated at a configurable size limit. This design is intentional — the LLM consumes JSON directly.

A separate Alertmanager client already exists in the Effectiveness Monitor service (`pkg/effectivenessmonitor/client/alertmanager_http.go`), but it returns typed Go structs for scoring logic, not raw JSON for LLM consumption.

### Problem Statement

Design the Alertmanager client integration for KA investigation tools that:
- Returns raw JSON responses for direct LLM consumption
- Supports the full Alertmanager v2 API query parameters (`active`, `silenced`, `inhibited`, `filter`, `receiver`)
- Follows established tool registration patterns (DD-HAPI-019-002)
- Maintains local/remote tool symmetry with the K8s MCP server

---

## Decision Drivers

1. **LLM consumption pattern**: KA tools return `string` (raw JSON), not typed structs
2. **Existing Prometheus pattern**: `Client.doGet()` → raw response → truncation → string output
3. **Independence from EM**: KA and EM have different lifecycles; coupling would complicate both
4. **Query parameter completeness**: K8s MCP server `get_alerts` supports `active`, `silenced`, `inhibited`, `filter`, `receiver`

---

## Alternatives Considered

### Alternative A: New Dedicated Package (RECOMMENDED)

**Approach**: Create `pkg/kubernautagent/tools/alertmanager/` with its own `Client` following the Prometheus raw-JSON pattern.

```go
// pkg/kubernautagent/tools/alertmanager/client.go
type Client struct {
    config     ClientConfig
    httpClient *http.Client
}

func (c *Client) doGet(ctx context.Context, apiPath string, params url.Values) (string, error) {
    // Same pattern as prometheus.Client.doGet() — raw JSON pass-through
}
```

**Pros**:
- Exact pattern match with existing Prometheus tools (team familiarity)
- Independent lifecycle — no cross-service coupling
- Supports all query parameters naturally
- Clean testability (mock HTTP, not typed interface)
- Consistent with DD-HAPI-019-002 tool design

**Cons**:
- New package (~150 lines of client + tools code)
- Slight duplication of HTTP client setup with Prometheus package

**Confidence**: 92%

---

### Alternative B: Reuse EM AlertManagerClient Directly

**Approach**: Import `pkg/effectivenessmonitor/client` in KA and wrap `AlertManagerClient.GetAlerts()` to serialize results as JSON.

**Pros**:
- No new client code

**Cons**:
- EM client returns `[]Alert` (typed structs) — requires serialization adapter for LLM consumption
- EM interface lacks `GetSilences()` — would need interface expansion (breaks EM)
- EM client only supports `filter` matchers — missing `active`, `silenced`, `inhibited`, `receiver` params
- Cross-service import coupling (KA depends on EM package)
- Different responsibility: EM client designed for scoring, not investigation

**Confidence**: 30% (rejected)

---

### Alternative C: Extract Shared Client to `pkg/shared/alertmanager/`

**Approach**: Create a shared Alertmanager client package used by both KA and EM.

**Pros**:
- DRY — single HTTP client for Alertmanager access
- Both services share configuration patterns

**Cons**:
- Requires refactoring EM client (out of scope, different concern)
- Shared client must serve two incompatible consumption patterns (typed structs for EM, raw JSON for KA)
- Increases blast radius — touching EM for a KA feature
- Over-engineering for 2 consumers with different needs

**Confidence**: 25% (rejected)

---

## Decision

**APPROVED: Alternative A** — New dedicated `pkg/kubernautagent/tools/alertmanager/` package.

### Rationale

1. **Pattern consistency**: Matches `pkg/kubernautagent/tools/prometheus/` exactly — same `ClientConfig`, same `doGet()` pass-through, same TLS/auth transport composition
2. **Minimal blast radius**: Only new files, no existing code modified
3. **Complete API coverage**: Can support all Alertmanager v2 query parameters without interface constraints
4. **Clean test boundary**: `httptest.NewServer` mocks the Alertmanager API directly

### Key Insight

KA tools and EM serve fundamentally different purposes for Alertmanager data. KA passes raw JSON to an LLM for reasoning. EM deserializes into typed structs for scoring algorithms. These are two distinct consumption patterns that should not be forced into a shared abstraction.

---

## Implementation

**Primary files**:
- `pkg/kubernautagent/tools/alertmanager/client.go` — HTTP client (ClientConfig, doGet)
- `pkg/kubernautagent/tools/alertmanager/tools.go` — `get_alerts`, `get_silences` tool implementations
- `pkg/kubernautagent/tools/alertmanager/suite_test.go` — Ginkgo test suite

**Configuration** (in `internal/kubernautagent/config/config.go`):
```go
type ToolsConfig struct {
    Prometheus   PrometheusToolConfig   `yaml:"prometheus"`
    Alertmanager AlertmanagerToolConfig `yaml:"alertmanager"`
}

type AlertmanagerToolConfig struct {
    URL       string        `yaml:"url"`
    Timeout   time.Duration `yaml:"timeout"`
    SizeLimit int           `yaml:"sizeLimit"`
    TLSCaFile string        `yaml:"tlsCaFile"`
}
```

**Wiring** (in `cmd/kubernautagent/main.go` `buildToolRegistry()`):
```go
if cfg.Integrations.Tools.Alertmanager.URL != "" {
    amClient, err := alertmanager.NewClient(amCfg)
    // ... register tools
}
```

---

## Consequences

### Positive
- Follows established patterns — minimal learning curve
- No cross-service coupling
- Full Alertmanager v2 API support
- Clean test isolation

### Negative
- ~150 lines of client code that partially overlaps with EM client HTTP logic
  - **Mitigation**: The overlap is minimal (basic HTTP GET with auth). The EM client will eventually diverge further as it adds scoring-specific features.

### Neutral
- New config section (`tools.alertmanager`) — operator must render it in KA ConfigMap

---

## Related Decisions

- **Builds on**: DD-HAPI-019-002 (Toolset Implementation Design)
- **Supports**: BR-KA-TOOLSET-001 (Node + Alertmanager toolset expansion)
- **Pattern reference**: `pkg/kubernautagent/tools/prometheus/` (Client + Tools structure)

---

## Review & Evolution

**When to revisit**:
- If a third service needs Alertmanager access → consider extracting shared client then
- If Alertmanager v3 API is released → update client endpoints

**Success Metrics**:
- `get_alerts` and `get_silences` work identically via KA native tools and K8s MCP server
- No EM test regressions from this change (zero coupling confirmed)
