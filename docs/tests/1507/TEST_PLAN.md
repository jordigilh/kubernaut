# IEEE 829 Test Plan: K8s MCP Tool Parity — Node & Alertmanager Tools

**Test Plan Identifier**: TP-KA-1507
**Version**: 1.0.0
**Created**: 2026-06-27
**Status**: Draft
**Issue**: [#1507](https://github.com/jordigilh/kubernaut/issues/1507)
**Business Requirement**: BR-KA-TOOLSET-001
**Design Decision**: DD-TOOLSET-002

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the implementation of 4 new investigation tools for the Kubernaut Agent (KA):
- `nodes_log` — retrieve node-level logs via kubelet proxy API
- `nodes_stats_summary` — retrieve node resource stats via kubelet Summary API
- `get_alerts` — query Alertmanager for active/silenced/inhibited alerts
- `get_silences` — query Alertmanager for configured silences

### 1.2 Scope

| In Scope | Out of Scope |
|----------|--------------|
| Tool logic (parameter parsing, response formatting) | Kubernaut Operator RBAC changes (separate issue) |
| HTTP client integration (Alertmanager, kubelet proxy) | K8s MCP server modifications |
| Configuration loading and validation | LLM prompt tuning for new tools |
| Tool registration and phase mapping | E2E Kind cluster tests (future phase) |
| Error handling and edge cases | TLS certificate provisioning |

### 1.3 References

| Document | Path |
|----------|------|
| Business Requirement | `docs/requirements/BR-KA-TOOLSET-001.md` |
| Design Decision | `docs/architecture/decisions/DD-TOOLSET-002-alertmanager-client-integration.md` |
| Tool Coverage Matrix | `docs/spikes/multi-cluster-mcp-gateway/spike-1-tool-coverage-matrix.md` |
| Prometheus Tool Pattern | `pkg/kubernautagent/tools/prometheus/tools.go` |
| K8s Metrics Tool Pattern | `pkg/kubernautagent/tools/k8s/metrics.go` |
| Test Plan Template | `docs/development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md` |

### 1.4 Test Scenario Naming Convention

**Format**: `{TIER}-KA-1507-{SEQUENCE}`

- `UT-KA-1507-NNN` — Unit Tests (100% coverage target)
- `IT-KA-1507-NNN` — Integration Tests (>=80% coverage target)

---

## 2. Test Items

### 2.1 Components Under Test

| Component | Package | Type | Production Entry Point |
|-----------|---------|------|----------------------|
| `nodes_log` tool | `pkg/kubernautagent/tools/k8s/` | Tool (struct) | `cmd/kubernautagent/main.go` `registerK8sTools()` |
| `nodes_stats_summary` tool | `pkg/kubernautagent/tools/k8s/` | Tool (struct) | `cmd/kubernautagent/main.go` `registerK8sTools()` |
| `get_alerts` tool | `pkg/kubernautagent/tools/alertmanager/` | Tool (struct) | `cmd/kubernautagent/main.go` `buildToolRegistry()` |
| `get_silences` tool | `pkg/kubernautagent/tools/alertmanager/` | Tool (struct) | `cmd/kubernautagent/main.go` `buildToolRegistry()` |
| Alertmanager Client | `pkg/kubernautagent/tools/alertmanager/` | HTTP Client | `cmd/kubernautagent/main.go` `buildToolRegistry()` |
| Node Proxy Client | `pkg/kubernautagent/tools/k8s/` | Interface | `cmd/kubernautagent/main.go` `registerK8sTools()` |
| Configuration | `internal/kubernautagent/config/` | Struct | Config file parsing |
| Phase Mapping | `internal/kubernautagent/investigator/` | Map entry | `DefaultPhaseToolMap()` |

### 2.2 Interfaces and Dependencies

```
                     ┌──────────────────────────────┐
                     │   cmd/kubernautagent/main.go  │
                     │       (Production Wiring)     │
                     └───────┬──────────┬───────────┘
                             │          │
              ┌──────────────▼──┐   ┌───▼────────────────┐
              │  k8s/node_tools │   │ alertmanager/tools  │
              │  nodes_log      │   │ get_alerts          │
              │  nodes_stats    │   │ get_silences        │
              └───────┬─────────┘   └─────────┬──────────┘
                      │                       │
         ┌────────────▼────────┐    ┌─────────▼──────────┐
         │  NodeProxyClient    │    │  alertmanager.Client │
         │  (interface)        │    │  (HTTP doGet)        │
         └────────────┬────────┘    └─────────┬──────────┘
                      │                       │
         ┌────────────▼────────┐    ┌─────────▼──────────┐
         │  K8s API Server     │    │  Alertmanager API   │
         │  /nodes/{n}/proxy/  │    │  /api/v2/alerts     │
         │                     │    │  /api/v2/silences   │
         └─────────────────────┘    └────────────────────┘
```

---

## 3. Features to be Tested

### 3.1 Functional Requirements

| ID | Feature | Tool | Acceptance Criteria |
|----|---------|------|---------------------|
| F-01 | Retrieve kubelet logs by node name and log path | `nodes_log` | Returns raw log text from `/api/v1/nodes/{name}/proxy/logs/{path}` |
| F-02 | Retrieve kubelet stats summary by node name | `nodes_stats_summary` | Returns raw JSON from `/api/v1/nodes/{name}/proxy/stats/summary` |
| F-03 | Query Alertmanager alerts with filters | `get_alerts` | Returns raw JSON from `/api/v2/alerts` with query params |
| F-04 | Query Alertmanager silences | `get_silences` | Returns raw JSON from `/api/v2/silences` |
| F-05 | Parameter validation | All | Invalid/missing params return structured error |
| F-06 | Response truncation | All | Output exceeding SizeLimit is truncated with hint |
| F-07 | HTTP error handling | All | Non-2xx responses return contextual error messages |
| F-08 | Timeout handling | All | Context cancellation or timeout produces meaningful error |
| F-09 | Tool registration | All | All 4 tools are registered in registry and accessible by name |
| F-10 | Phase mapping | All | All 4 tools are available in RCA phase |

### 3.2 Non-Functional Requirements

| ID | Feature | Acceptance Criteria |
|----|---------|---------------------|
| NF-01 | Response size control | All responses capped at configurable `SizeLimit` |
| NF-02 | Testability | All external I/O abstracted behind interfaces/mocks |
| NF-03 | Configuration | Tools disabled when URL is empty string |

---

## 4. Features Not Tested

| Feature | Reason |
|---------|--------|
| TLS mutual auth | Covered by shared TLS transport tests in `pkg/shared/tls/` |
| Bearer token injection | Covered by `pkg/shared/auth/` tests |
| LLM interpretation of tool output | Functional validation, not code test |
| Operator RBAC provisioning | Separate repo, separate issue |
| Real kubelet or Alertmanager integration | Requires cluster; deferred to E2E phase |

---

## 5. Approach

### 5.1 Test Strategy

**Pyramid**:
- **UT (100% code coverage)**: All tool logic, parameter parsing, error paths, truncation, client methods
- **IT (>=80%)**: HTTP integration via `httptest.NewServer`, tool registration dispatch, config loading

**TDD Sequence** (per tool):
1. RED: Write UT for parameter parsing → fails (type does not exist)
2. RED: Write UT for execute happy path → fails (method not implemented)
3. RED: Write UT for error paths → fails
4. GREEN: Implement minimal tool struct + Execute method → UT pass
5. RED: Write IT for HTTP dispatch via tool registry → fails (not wired)
6. GREEN: Wire tool in `main.go` → IT pass
7. REFACTOR: Edge cases, documentation, code quality

### 5.2 Mock Strategy

| Dependency | Mock Approach | Justification |
|------------|--------------|---------------|
| Alertmanager API | `httptest.NewServer` returning fixture JSON | Tool must handle real HTTP responses |
| Kubelet Proxy API | `NodeProxyClientInterface` (interface mock) | K8s client-go RESTClient is complex to mock at HTTP level |
| K8s API (for node proxy) | `fake.NewClientBuilder()` or interface mock | Consistent with existing `MetricsClientInterface` pattern |
| Tool Registry | Real `registry.Registry` instance | Proves wiring, not external |

### 5.3 FedRAMP/SOC2 Control Mappings

| Control | Objective | Mapped Tests |
|---------|-----------|--------------|
| SI-4 (Information System Monitoring) | System monitors for unauthorized access and anomalies | UT-KA-1507-030..040 (get_alerts filters active/silenced/inhibited) |
| AU-6 (Audit Review, Analysis, Reporting) | Review and analyze audit records for inappropriate activity | UT-KA-1507-001..020 (nodes_log retrieval and parsing) |
| CM-6 (Configuration Settings) | Enforce security configuration settings | UT-KA-1507-100..120 (config validation, disabled when URL empty) |
| SC-7 (Boundary Protection) | Monitor communications at system boundary | IT-KA-1507-001..010 (HTTP client integration, error handling) |
| SI-5 (Security Alerts, Advisories) | Receive and disseminate security alerts | UT-KA-1507-050..060 (get_silences validates silence correlation) |

---

## 6. Test Cases — Unit Tests (100% Coverage Target)

### 6.1 nodes_log Tool

| ID | Description | Input | Expected Output | Status |
|----|-------------|-------|-----------------|--------|
| UT-KA-1507-001 | Happy path: valid node name + log path | `{"node":"worker-1","path":"kubelet.log"}` | Raw log text from proxy client | Planned |
| UT-KA-1507-002 | Optional tail_lines parameter | `{"node":"worker-1","path":"kubelet.log","tail_lines":100}` | Appends `?tail=100` to proxy request | Planned |
| UT-KA-1507-003 | Missing required param: node | `{"path":"kubelet.log"}` | Error: "node is required" | Planned |
| UT-KA-1507-004 | Missing required param: path | `{"node":"worker-1"}` | Error: "path is required" | Planned |
| UT-KA-1507-005 | Empty node name (empty string) | `{"node":"","path":"kubelet.log"}` | Error: "node is required" | Planned |
| UT-KA-1507-006 | Proxy client returns error (node not found) | `{"node":"nonexistent","path":"kubelet.log"}` | Error wrapping proxy error | Planned |
| UT-KA-1507-007 | Response exceeds size limit | Large response body | Truncated output with hint | Planned |
| UT-KA-1507-008 | Context cancelled (timeout) | Cancelled context | Context error propagated | Planned |
| UT-KA-1507-009 | Name() returns "nodes_log" | N/A | `"nodes_log"` | Planned |
| UT-KA-1507-010 | Description() returns meaningful text | N/A | Non-empty string | Planned |
| UT-KA-1507-011 | Parameters() returns valid JSON schema | N/A | Valid JSON with required fields | Planned |
| UT-KA-1507-012 | Path traversal sanitization | `{"node":"worker-1","path":"../../etc/passwd"}` | Error: invalid path | Planned |
| UT-KA-1507-013 | Nil args passed to Execute | `nil` | Error: "node is required" | Planned |
| UT-KA-1507-014 | Malformed JSON args | `{invalid` | Error: "parsing args" | Planned |

### 6.2 nodes_stats_summary Tool

| ID | Description | Input | Expected Output | Status |
|----|-------------|-------|-----------------|--------|
| UT-KA-1507-020 | Happy path: valid node name | `{"node":"worker-1"}` | Raw JSON from kubelet stats/summary | Planned |
| UT-KA-1507-021 | Missing required param: node | `{}` | Error: "node is required" | Planned |
| UT-KA-1507-022 | Empty node name | `{"node":""}` | Error: "node is required" | Planned |
| UT-KA-1507-023 | Proxy client returns error | `{"node":"nonexistent"}` | Error wrapping proxy error | Planned |
| UT-KA-1507-024 | Response exceeds size limit | Large stats JSON | Truncated with hint | Planned |
| UT-KA-1507-025 | Context cancelled | Cancelled context | Context error propagated | Planned |
| UT-KA-1507-026 | Name() returns "nodes_stats_summary" | N/A | `"nodes_stats_summary"` | Planned |
| UT-KA-1507-027 | Description() returns meaningful text | N/A | Non-empty string | Planned |
| UT-KA-1507-028 | Parameters() returns valid JSON schema | N/A | Valid JSON with "node" required | Planned |
| UT-KA-1507-029 | Nil args passed to Execute | `nil` | Error: "node is required" | Planned |
| UT-KA-1507-030-a | Malformed JSON args | `{invalid` | Error: "parsing args" | Planned |

### 6.3 get_alerts Tool

| ID | Description | Input | Expected Output | Status |
|----|-------------|-------|-----------------|--------|
| UT-KA-1507-030 | Happy path: no filters (all alerts) | `{}` | Raw JSON from `/api/v2/alerts` | Planned |
| UT-KA-1507-031 | Filter: active=true | `{"active":true}` | Query param `active=true` sent | Planned |
| UT-KA-1507-032 | Filter: silenced=true | `{"silenced":true}` | Query param `silenced=true` sent | Planned |
| UT-KA-1507-033 | Filter: inhibited=true | `{"inhibited":true}` | Query param `inhibited=true` sent | Planned |
| UT-KA-1507-034 | Filter: receiver name | `{"receiver":"slack-alerts"}` | Query param `receiver=slack-alerts` sent | Planned |
| UT-KA-1507-035 | Filter: label matchers | `{"filter":["alertname=~KubePod.*"]}` | Query param `filter=alertname=~KubePod.*` sent | Planned |
| UT-KA-1507-036 | Combined filters | `{"active":true,"silenced":false,"filter":["severity=critical"]}` | All query params composed correctly | Planned |
| UT-KA-1507-037 | HTTP error (Alertmanager down) | Client returns non-2xx | Error with status code context | Planned |
| UT-KA-1507-038 | Response exceeds size limit | Large alerts JSON | Truncated with hint | Planned |
| UT-KA-1507-039 | Context cancelled | Cancelled context | Context error propagated | Planned |
| UT-KA-1507-040 | Name() returns "get_alerts" | N/A | `"get_alerts"` | Planned |
| UT-KA-1507-041 | Description() returns meaningful text | N/A | Non-empty string | Planned |
| UT-KA-1507-042 | Parameters() returns valid JSON schema | N/A | Valid JSON with optional properties | Planned |
| UT-KA-1507-043 | Nil args (treated as no filters) | `nil` | Same as empty object — returns all alerts | Planned |
| UT-KA-1507-044 | Malformed JSON args | `{invalid` | Error: "parsing args" | Planned |
| UT-KA-1507-045 | Empty filter array | `{"filter":[]}` | No filter query param sent | Planned |

### 6.4 get_silences Tool

| ID | Description | Input | Expected Output | Status |
|----|-------------|-------|-----------------|--------|
| UT-KA-1507-050 | Happy path: no filters | `{}` | Raw JSON from `/api/v2/silences` | Planned |
| UT-KA-1507-051 | Filter: label matchers | `{"filter":["alertname=KubePodCrashLooping"]}` | Query param `filter=alertname=KubePodCrashLooping` | Planned |
| UT-KA-1507-052 | HTTP error (Alertmanager unreachable) | Client returns error | Error with context | Planned |
| UT-KA-1507-053 | Response exceeds size limit | Large silences JSON | Truncated with hint | Planned |
| UT-KA-1507-054 | Context cancelled | Cancelled context | Context error propagated | Planned |
| UT-KA-1507-055 | Name() returns "get_silences" | N/A | `"get_silences"` | Planned |
| UT-KA-1507-056 | Description() returns meaningful text | N/A | Non-empty string | Planned |
| UT-KA-1507-057 | Parameters() returns valid JSON schema | N/A | Valid JSON | Planned |
| UT-KA-1507-058 | Nil args (no filters) | `nil` | Same as empty object | Planned |
| UT-KA-1507-059 | Malformed JSON args | `{invalid` | Error: "parsing args" | Planned |

### 6.5 Alertmanager Client

| ID | Description | Input | Expected Output | Status |
|----|-------------|-------|-----------------|--------|
| UT-KA-1507-100 | NewClient with valid config | Valid ClientConfig | Non-nil Client, no error | Planned |
| UT-KA-1507-101 | NewClient defaults: SizeLimit | Config with SizeLimit=0 | Defaults to 30000 | Planned |
| UT-KA-1507-102 | NewClient defaults: Timeout | Config with Timeout=0 | Defaults to 30s | Planned |
| UT-KA-1507-103 | doGet: successful GET with params | URL + params | Correct URL + query string composed | Planned |
| UT-KA-1507-104 | doGet: response truncation at SizeLimit | Response > SizeLimit | Truncated + hint appended | Planned |
| UT-KA-1507-105 | doGet: HTTP error response (4xx) | Server returns 404 | Error with HTTP status code | Planned |
| UT-KA-1507-106 | doGet: HTTP error response (5xx) | Server returns 500 | Error with HTTP status code | Planned |
| UT-KA-1507-107 | doGet: network error | Unreachable server | Connection error | Planned |
| UT-KA-1507-108 | doGet: context cancellation | Cancelled context | Context error | Planned |
| UT-KA-1507-109 | doGet: headers set from config | Config with custom headers | Headers present in request | Planned |
| UT-KA-1507-110 | doGet: custom Transport used | Config with Transport | Transport used by HTTP client | Planned |
| UT-KA-1507-111 | Config() returns stored config | N/A | Returns exact config passed to NewClient | Planned |

### 6.6 Node Proxy Client Interface

| ID | Description | Input | Expected Output | Status |
|----|-------------|-------|-----------------|--------|
| UT-KA-1507-120 | GetNodeLogs: successful call | nodeName, logPath, tailLines | Raw bytes from proxy endpoint | Planned |
| UT-KA-1507-121 | GetNodeLogs: node not found | nonexistent node | Error: not found | Planned |
| UT-KA-1507-122 | GetNodeLogs: forbidden (RBAC) | node without proxy permission | Error: forbidden | Planned |
| UT-KA-1507-123 | GetNodeStats: successful call | nodeName | Raw bytes from proxy endpoint | Planned |
| UT-KA-1507-124 | GetNodeStats: node not found | nonexistent node | Error: not found | Planned |
| UT-KA-1507-125 | GetNodeStats: forbidden (RBAC) | No nodes/proxy permission | Error: forbidden | Planned |

### 6.7 Configuration

| ID | Description | Input | Expected Output | Status |
|----|-------------|-------|-----------------|--------|
| UT-KA-1507-130 | AlertmanagerToolConfig parsed from YAML | Valid YAML | Config struct populated | Planned |
| UT-KA-1507-131 | Empty Alertmanager URL disables tools | URL="" | Tools not registered | Planned |
| UT-KA-1507-132 | TLSCaFile path stored in config | `tlsCaFile: "/path/to/ca.crt"` | Config.TLSCaFile == "/path/to/ca.crt" | Planned |

---

## 7. Test Cases — Integration Tests (>=80% Coverage Target)

### 7.1 Alertmanager HTTP Integration

| ID | Description | Test Approach | Status |
|----|-------------|---------------|--------|
| IT-KA-1507-001 | get_alerts via httptest server (happy path) | `httptest.NewServer` returns fixture alerts JSON; tool invoked through registry | Planned |
| IT-KA-1507-002 | get_alerts with query params forwarded | Assert request received has correct query string | Planned |
| IT-KA-1507-003 | get_silences via httptest server (happy path) | `httptest.NewServer` returns fixture silences JSON | Planned |
| IT-KA-1507-004 | get_alerts: server returns 500 | httptest returns 500; assert tool returns error | Planned |
| IT-KA-1507-005 | get_silences: server timeout | httptest delays > client timeout | Planned |
| IT-KA-1507-006 | Client respects custom headers | Assert request has expected headers | Planned |
| IT-KA-1507-007 | Client respects custom Transport | Verify Transport.RoundTrip is invoked | Planned |

### 7.2 Node Proxy HTTP Integration

| ID | Description | Test Approach | Status |
|----|-------------|---------------|--------|
| IT-KA-1507-010 | nodes_log via mock NodeProxyClient (happy path) | Interface mock returns fixture log bytes; tool returns string | Planned |
| IT-KA-1507-011 | nodes_stats_summary via mock NodeProxyClient | Interface mock returns fixture stats JSON | Planned |
| IT-KA-1507-012 | nodes_log: proxy returns error | Mock returns error; tool propagates | Planned |
| IT-KA-1507-013 | nodes_stats_summary: proxy returns error | Mock returns error; tool propagates | Planned |

### 7.3 Tool Registration & Wiring

| ID | Description | Test Approach | Status |
|----|-------------|---------------|--------|
| IT-KA-1507-020 | get_alerts registered in tool registry | Build registry with Alertmanager config; assert `registry.Get("get_alerts")` != nil | Planned |
| IT-KA-1507-021 | get_silences registered in tool registry | Same as above for "get_silences" | Planned |
| IT-KA-1507-022 | nodes_log registered in tool registry | Build registry with K8s config; assert `registry.Get("nodes_log")` != nil | Planned |
| IT-KA-1507-023 | nodes_stats_summary registered in tool registry | Same as above | Planned |
| IT-KA-1507-024 | Tools absent when Alertmanager URL is empty | Empty URL config; assert get_alerts not in registry | Planned |
| IT-KA-1507-025 | All 4 tools in RCA phase tool map | Assert DefaultPhaseToolMap()[RCA] contains all 4 names | Planned |

### 7.4 End-to-End Tool Dispatch (IT-level)

| ID | Description | Test Approach | Status |
|----|-------------|---------------|--------|
| IT-KA-1507-030 | get_alerts dispatched via registry.Execute() | Call registry.Execute("get_alerts", args); assert response matches fixture | Planned |
| IT-KA-1507-031 | nodes_log dispatched via registry.Execute() | Call registry.Execute("nodes_log", args); assert log text returned | Planned |

---

## 8. Test Environment

### 8.1 Unit Test Environment

- Go 1.23+
- Ginkgo v2 / Gomega BDD framework
- No external services required
- Mock interfaces for all external I/O

### 8.2 Integration Test Environment

- Go 1.23+
- `httptest.NewServer` for Alertmanager mock
- Interface mocks for NodeProxyClient
- Real `registry.Registry` and `config.Config` instances
- No cluster access required

### 8.3 Test Files

| File | Scope |
|------|-------|
| `pkg/kubernautagent/tools/alertmanager/client_test.go` | UT: Client doGet, config defaults, error paths |
| `pkg/kubernautagent/tools/alertmanager/tools_test.go` | UT: get_alerts, get_silences tool logic |
| `pkg/kubernautagent/tools/alertmanager/suite_test.go` | Ginkgo suite bootstrap |
| `pkg/kubernautagent/tools/k8s/node_tools_test.go` | UT: nodes_log, nodes_stats_summary tool logic |
| `test/integration/kubernautagent/alertmanager_tools_test.go` | IT: HTTP integration, registry dispatch |
| `test/integration/kubernautagent/node_tools_test.go` | IT: Node proxy integration, registry dispatch |

---

## 9. Pass/Fail Criteria

### 9.1 Unit Tests

- **PASS**: 100% line coverage of new code in `pkg/kubernautagent/tools/alertmanager/` and new node tool code in `pkg/kubernautagent/tools/k8s/`
- **FAIL**: Any uncovered line, any test using `Skip()` or `XIt`

### 9.2 Integration Tests

- **PASS**: >=80% of integration-testable code exercised through production wiring paths
- **FAIL**: Any wiring point in the Wiring Manifest without a passing IT

### 9.3 Overall

- **PASS**: All tests green, `go build ./...` succeeds, `golangci-lint run` passes
- **FAIL**: Any test red, build failure, or lint violation

---

## 10. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `nodes_log` tool | `registerK8sTools()` | `cmd/kubernautagent/main.go` | IT-KA-1507-022 |
| `nodes_stats_summary` tool | `registerK8sTools()` | `cmd/kubernautagent/main.go` | IT-KA-1507-023 |
| `get_alerts` tool | `buildToolRegistry()` | `cmd/kubernautagent/main.go` | IT-KA-1507-020 |
| `get_silences` tool | `buildToolRegistry()` | `cmd/kubernautagent/main.go` | IT-KA-1507-021 |
| `alertmanager.Client` | `buildToolRegistry()` | `cmd/kubernautagent/main.go` | IT-KA-1507-001 |
| `NodeProxyClient` | `registerK8sTools()` | `cmd/kubernautagent/main.go` | IT-KA-1507-010 |
| RCA phase mapping | `DefaultPhaseToolMap()` | `internal/kubernautagent/investigator/types.go` | IT-KA-1507-025 |
| `AlertmanagerToolConfig` | Config parsing | `internal/kubernautagent/config/config.go` | UT-KA-1507-130 |

---

## 11. Suspension Criteria and Resumption

### 11.1 Suspension

- RBAC `nodes/proxy` permission not available in test cluster → suspend node tool IT tests
- Alertmanager API contract changes incompatibly → suspend alertmanager tool tests pending spec update

### 11.2 Resumption

- Operator issue resolved → re-enable node tool IT
- API spec confirmed → update fixtures and re-enable

---

## 12. Risks and Mitigations

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Kubelet proxy API rate limiting | Low | Medium | UT mocks bypass; IT uses interface mock |
| Alertmanager v2 API deprecation | Very Low | High | Pin to `/api/v2/`; monitor upstream |
| Large alert volumes causing OOM | Medium | Medium | `SizeLimit` truncation enforced at client level |
| `nodes/proxy` RBAC missing in prod | High (known) | Blocks node tools | Separate operator issue; tools return clear RBAC error |
| Path traversal in `nodes_log` path param | Medium | High | Sanitize path: reject `..`, absolute paths |

---

## 13. Approvals

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Author | AI Assistant | 2026-06-27 | — |
| Reviewer | | | |
| Approver | | | |
