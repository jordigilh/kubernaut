# Test Plan: Alert Severity Prioritization

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1412-v1
**Feature**: Deterministic alert severity prioritization in list_alerts tool
**Version**: 1.0
**Created**: 2026-06-13
**Author**: AI Agent
**Status**: Implemented
**Branch**: `feat/structured-decision-payload`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the deterministic alert severity prioritization feature added to the `list_alerts` / `kubernaut_list_alerts` tool. When multiple Prometheus alerts fire simultaneously, the tool must select the highest-severity, longest-firing alert and structure the response for downstream consumers (LLM agent, Console, autonomous flow).

### 1.2 Objectives

1. **Deterministic ranking**: Alerts sorted by severity (descending) then ActiveAt (ascending, FIFO)
2. **Response contract**: `prioritized` field populated with `selected`, `tied`, `also_active`
3. **MCP wiring**: `kubernaut_list_alerts` registered on MCP bridge when PromClient is configured
4. **RBAC enforcement**: SAR authorization required for MCP tool invocation
5. **Prompt guidance**: LLM instructed to investigate `selected` alert
6. **Edge cases**: Empty results, single alert, all-same-severity, unknown severity labels

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "UT-AF-1412"` |
| Integration test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -run "IT-AF-1412"` |
| E2E test pass rate | 100% | `make test-e2e-apifrontend` (alert-prioritization label) |
| Coverage on PrioritizeAlerts | >=80% | `go test -coverprofile` |

---

## 2. References

### 2.1 Authority

- Issue #1412: Alert severity prioritization
- BR-ALERT-001: Intelligent alert routing and prioritization
- ADR-066: 4-level severity model migration
- DD-AF-005: Alert prioritization algorithm

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| SI-4(5) | Automated alert severity classification | `PrioritizeAlerts` deterministically ranks by severity | UT-AF-1412-010, 020, 030 |
| IR-4(1) | Automated incident response mechanisms | Selected alert drives RR creation in autonomous flow | E2E-AF-1412-001 |
| AU-3 | Audit content (what, when, where, who) | MCP tool call logged with user, tool, result | IT-AF-1412-001 |

### 2.3 Cross-References

- [DD-AF-005](../../architecture/decisions/DD-AF-005-alert-prioritization-algorithm.md)
- [ADR-066](../../architecture/decisions/ADR-066-4-level-severity-model.md)
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)

---

## 3. Scope

### 3.1 In Scope

- `PrioritizeAlerts` ranking logic
- `HandleListAlerts` pipeline (filter → truncate → prioritize)
- MCP bridge registration of `kubernaut_list_alerts`
- RBAC SAR authorization
- LLM prompt guidance for prioritized results
- E2E validation via MCP `tools/call`

### 3.2 Out of Scope

- Prometheus alert rule creation (infrastructure fixture)
- Console rendering of prioritized alerts (#1416)
- ADR-066 Phase 2+ migration (future work)

---

## 4. Test Scenarios

### 4.1 Unit Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| UT-AF-1412-010 | Single critical among warning and info | `selected.severity == "critical"` | Implemented |
| UT-AF-1412-020 | Two critical alerts, different ActiveAt | Older (FIFO) is selected, newer in tied | Implemented |
| UT-AF-1412-030 | All same severity | First by FIFO is selected, rest in tied | Implemented |
| UT-AF-1412-040 | Empty alert list | `prioritized == nil` | Implemented |
| UT-AF-1412-050 | Unknown severity label | Ranks at 0 (below info) | Implemented |
| UT-AF-1412-060 | Warning and medium coexist (rank 3 tie) | Both rank equally, FIFO breaks tie | Implemented |
| UT-AF-1412-001 | `severityRank` includes "warning" | `warning` maps to rank 3 | Implemented |

### 4.2 Integration Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| IT-AF-1412-001 | MCP tools/call `kubernaut_list_alerts` returns JSON with `prioritized` field | HTTP 200, valid ListAlertsResult JSON | Implemented |
| IT-AF-1412-002 | MCP tools/call without RBAC grant denied | SAR denial, isError=true | Implemented |

### 4.3 E2E Tests

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| E2E-AF-1412-001 | MCP tools/call in Kind cluster with Prometheus firing HighCPU (critical) | `prioritized.selected.labels.severity == "critical"` | Implemented |
| E2E-AF-1412-002 | Structural validation — tool returns valid response shape | No JSON-RPC error, valid result | Implemented |

---

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID |
|-----------|----------------------|---------------------|----------------|
| PrioritizeAlerts | HandleListAlerts() | pkg/apifrontend/tools/af_alerts.go:179 | UT-AF-1412-010 |
| kubernaut_list_alerts (MCP) | RegisterTools() | pkg/apifrontend/handler/mcp_bridge.go | IT-AF-1412-001 |
| list_alerts (A2A) | buildToolList() | pkg/apifrontend/agent/root.go:147 | UT-AF-1412-001 |
| PromClient → MCPBridgeConfig | main.go bridgeCfg | cmd/apifrontend/main.go:845 | E2E-AF-1412-001 |
| RBAC grant | e2e-user-rbac.yaml | deploy/apifrontend/overlays/e2e/ | IT-AF-1412-002 |

---

## 6. Infrastructure

- **Prometheus**: Deployed in E2E Kind cluster (`test/infrastructure/apifrontend_prometheus_e2e.go`)
- **Alert fixture**: `SeverityTriageAlertRulesYAML` fires HighCPU (critical) in `default` namespace
- **Mock-LLM**: `af_list_alerts_prioritized` scenario for A2A path (keyword: "list alerts")

---

## 7. Execution

```bash
# Unit tests
go test ./pkg/apifrontend/tools/... -run "UT-AF-1412" -v -count=1
go test ./pkg/apifrontend/severity/... -run "UT-AF-1412" -v -count=1

# Integration tests
go test ./pkg/apifrontend/tools/... -run "IT-AF-1412" -v -count=1

# E2E tests
make test-e2e-apifrontend GINKGO_FOCUS="1412"
```
