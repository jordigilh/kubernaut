# Test Plan: IT Tier Coverage Expansion — Real Containers + envtest

**Test Plan Identifier:** TP-AF-IT-EXP-01
**Issue:** #87
**Version:** 1.0
**Date:** 2026-05-14
**Predecessor:** TP-AF-GA-P2 (Phase 2–3 Test Plan)

---

## 1. Introduction

This test plan validates the integration tier (IT) expansion for the kubernaut-apifrontend v1.5.0-rc1 release. The IT tier exercises the full MCP tool surface (20 tools) through real containers (KA, DS, PostgreSQL, Redis, Mock LLM) and a real Kubernetes API (envtest), per the `INTEGRATION_E2E_NO_MOCKS_POLICY`.

### 1.1 Scope

- Replace `dynamicfake.FakeDynamicClient` with a real `dynamic.NewForConfig` backed by envtest
- Exercise all 20 MCP tools against real containers and envtest K8s API
- Verify resilience contracts (circuit breaker, retry, timeout) using TCP proxy pattern
- Validate RBAC enforcement per tool
- Assert metrics emission and audit event fidelity

### 1.2 Out of Scope

- LLM orchestration (severity triage with real LLM) — mocked per LLM policy
- HTTP middleware stack (JWT, CORS, rate limit) — covered by UT and E2E
- Server bootstrap (router, health, agentcard) — covered by E2E
- controller-runtime K8s circuit breakers — covered by UT

### 1.3 References

- TP-AF-GA-P1 (`docs/test/87/TEST_PLAN.md`)
- TP-AF-GA-P2 (`docs/test/87/TEST_PLAN_PHASE2.md`)
- kubernaut `TESTING_GUIDELINES` v2.7.0
- kubernaut `INTEGRATION_E2E_NO_MOCKS_POLICY.md`
- kubernaut PR #828 (`tcpProxy` pattern, commit `26d7f17b`)
- Architecture Design Document (`docs/design/ARCHITECTURE.md` §5: API Surface)
- 100 Go Mistakes and How to Avoid Them
- IEEE 829-2008 Standard for Software and System Test Documentation

### 1.4 Business Requirements and Feature Traceability

| BR ID | Description | Source Issue | v1.5 Feature |
|-------|-------------|-------------|--------------|
| BR-MCP-001 | MCP tool surface proxies to KA and native CRD operations | #3 | MCP Protocol Handler |
| BR-CRD-001 | Native K8s CRD tools: submit_signal, list/get remediations, approve, cancel | #19 | CRD Tool Surface |
| BR-DS-001 | Native DS tools: list_workflows, remediation_history, effectiveness, audit_trail | #20 | DataStorage Query Tools |
| BR-AUTH-001 | User impersonation and JWT delegation for downstream calls | #55 | End-to-End AuthZ |
| BR-RESIL-001 | Circuit breaker + retry + timeout on all downstream dependencies | #38 | Resilience Layer |
| BR-OBS-001 | Prometheus metrics emission for all tool calls (count, duration, result) | #11 | Observability |
| BR-AUDIT-001 | Audit trail emission for security-relevant tool calls | #34 | Audit Trail |
| BR-RBAC-001 | Per-tool RBAC enforcement based on user group membership | #55, #83 | Tool-level RBAC |
| BR-TRIAGE-001 | AF K8s triage tools provide cluster context to LLM orchestrator | #52, #53 | NL Investigation |
| BR-DEDUP-001 | af_check_existing_rr prevents duplicate RR creation | #19 | RR Dedup |
| BR-DECISION-001 | kubernaut_present_decision surfaces user decision points from KA | #3 | User Decision Flow |
| BR-E2E-001 | E2E CI pipeline validates full integration stack before merge | #87 | CI Pipeline |

---

## 2. Test Items

| Item | Package(s) | Source Files | BR |
|------|-----------|-------------|-----|
| envtest K8s client (no fakes) | `test/integration` | `suite_test.go` | BR-AUTH-001 |
| KA REST dispatch tools | `internal/tools` | `ka_tools.go` | BR-MCP-001 |
| DS query tools | `internal/tools` | `ds_tools.go` | BR-DS-001 |
| K8s CRD tools | `internal/tools` | `crd_tools.go` | BR-CRD-001 |
| AF triage tools | `internal/tools` | `af_*.go` | BR-TRIAGE-001, BR-DEDUP-001 |
| MCP bridge dispatch | `internal/handler` | `mcp_bridge.go` | BR-MCP-001 |
| RBAC gate | `internal/handler` | `mcp_bridge.go` | BR-RBAC-001 |
| Circuit breaker (KA) | `internal/resilience` | `circuitbreaker.go` | BR-RESIL-001 |
| Retry transport (KA) | `internal/resilience` | `retry.go` | BR-RESIL-001 |
| Metrics collection | `internal/handler` | `mcp_bridge.go` | BR-OBS-001 |
| Audit emission | `internal/audit` | `audit.go` | BR-AUDIT-001 |

---

## 3. Approach

### 3.1 TDD Methodology

RED → GREEN → REFACTOR for each spec category.

### 3.2 Policy Compliance

- **NO fake K8s clients** in IT — real envtest with CRDs loaded
- **NO mock HTTP servers for KA/DS** — real containers via Podman
- Only LLM is mocked (per `INTEGRATION_E2E_NO_MOCKS_POLICY` exemption)
- TCP proxy for deterministic network failure simulation (resilience)

### 3.3 Infrastructure Stack

```
envtest (K8s API: etcd + kube-apiserver)
  ├── kubernaut CRDs (9) + apifrontend CRD (1)
  ├── Fixtures: Pods, Events, Deployments, RRs, SPs
  └── ServiceAccount with DataStorage access

Podman containers:
  ├── PostgreSQL (port 14400)
  ├── Redis (port 14401)
  ├── DataStorage (port 14402)
  ├── Mock LLM (port 14406)
  └── Kubernaut Agent (port 14404)

TCP Proxy (resilience tests only):
  └── ka.Client -> proxy(ephemeral) -> KA(14404)
```

### 3.4 Coverage Target

Per `TESTING_GUIDELINES` v2.7.0: **>=80% of the IT-testable code subset** (104 funcs, ~3,500 LOC).

---

## 4. Pass/Fail Criteria

- All 50 specs pass with `go test -race -count=1`
- `go vet ./...` reports zero errors
- `golangci-lint run` reports zero issues
- >=80% line coverage of IT-testable subset (measured with `--coverpkg=./internal/handler/...,./internal/ka/...,./internal/ds/...,./internal/resilience/...,./internal/tools/...,./internal/audit/...,./internal/auth/...`)
- No regressions in existing test suites

---

## 5. Test Cases

### 5.0 Phase 0: envtest Policy Compliance (BR-AUTH-001)

| TC ID | BR | Description | Input | Expected Result | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-ENV-001 | BR-AUTH-001 | envtest loads kubernaut CRDs from module cache | CRD YAML paths from GOMODCACHE | All 9 kubernaut + 1 apifrontend CRD registered | Setup |
| IT-ENV-002 | BR-AUTH-001 | Real dynamic client connects to envtest | `dynamic.NewForConfig(envtestCfg)` | List operations succeed on CRD GVRs | Setup |
| IT-ENV-003 | BR-CRD-001 | K8s fixtures seeded successfully | Create RR, SP, Pod, Event, Deploy, RS | All resources exist in envtest | Setup |

---

### 5.1 Category A: MCP Protocol and Session (BR-MCP-001)

| TC ID | BR | Description | Input | Expected Result | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-PROTO-001 | BR-MCP-001 | tools/list returns all 20 registered tools with schemas | `tools/list` JSON-RPC call | Response lists 20 tools, each with name + inputSchema | IT |
| IT-PROTO-002 | BR-AUDIT-001 | Multiple sessions produce independent audit trails | 2 sessions, 1 tool call each | >=2 tool-specific audit events; per-session UserID preserved | IT |
| IT-PROTO-003 | BR-MCP-001 | Invalid JSON-RPC returns protocol-level error | Malformed JSON body | JSON-RPC error response (code -32700) | IT |
| IT-PROTO-004 | BR-MCP-001 | Uninitialized session cannot call tools | `tools/call` without prior `initialize` | JSON-RPC error (session not found) | IT |

---

### 5.2 Category B: K8s CRD Tool Dispatch (BR-CRD-001)

All specs execute against the **real envtest API server** with seeded fixtures.

| TC ID | BR | Description | Input | Expected Result | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-CRD-001 | BR-CRD-001 | kubernaut_list_remediations returns seeded RRs | `namespace: "it-apifrontend"` | Response contains seeded RR names | IT |
| IT-CRD-002 | BR-CRD-001 | kubernaut_get_remediation with valid RR name | Existing RR name | Spec and status fields returned | IT |
| IT-CRD-003 | BR-CRD-001 | kubernaut_get_remediation with invalid name returns error | Non-existent name | Error containing "not found" | IT |
| IT-CRD-004 | BR-CRD-001 | kubernaut_submit_signal creates SignalProcessing in envtest | `namespace, kind, name, description, severity` (SubmitSignalArgs schema) | New SP resource exists in envtest after call | IT |
| IT-CRD-005 | BR-CRD-001 | kubernaut_approve transitions RAR phase | Seeded RAR resource | RAR status.phase updated via status patch; read-after-write confirms "approved" | IT |
| IT-CRD-006 | BR-CRD-001 | kubernaut_cancel_remediation marks RR cancelled | Active RR | RR status reflects cancellation; read-after-write confirms phase | IT |
| IT-CRD-007 | BR-CRD-001 | kubernaut_watch returns events or graceful timeout for seeded RR | `namespace, name` of seeded RR | WatchResult with events or status "completed"/"cancelled" | IT |

---

### 5.3 Category C: AF K8s Triage Tools (BR-TRIAGE-001, BR-DEDUP-001)

| TC ID | BR | Description | Input | Expected Result | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-TRIAGE-001 | BR-TRIAGE-001 | af_list_events returns seeded events | `namespace: "it-apifrontend"` | Event objects with reason, message, timestamp | IT |
| IT-TRIAGE-002 | BR-TRIAGE-001 | af_get_pods returns real pods | `namespace: "it-apifrontend"` | Pod names, phases, container statuses | IT |
| IT-TRIAGE-003 | BR-TRIAGE-001 | af_get_workloads returns deployments and statefulsets | `namespace: "it-apifrontend"` | Deployment + StatefulSet with replica counts | IT |
| IT-TRIAGE-004 | BR-TRIAGE-001 | af_resolve_owner resolves Pod -> RS -> Deployment chain | Pod owned by RS owned by Deployment | Returns Deployment as root owner | IT |
| IT-TRIAGE-005 | BR-DEDUP-001 | af_check_existing_rr finds seeded RR by label | Label selector matching seeded RR | `exists: true`, RR name returned | IT |
| IT-TRIAGE-006 | BR-DEDUP-001 | af_create_rr creates real RR in envtest | Valid RR spec | RR persists in envtest; read-after-write verifies labels, severity, fingerprint (64-char SHA256) | IT |

---

### 5.4 Category D: KA REST Tool Dispatch (BR-MCP-001)

These specs exercise the real KA container (with mock LLM backend).

| TC ID | BR | Description | Input | Expected Result | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-KA-001 | BR-MCP-001 | kubernaut_start_investigation dispatches to real KA | `namespace: "default", name: "test-deploy", kind: "Deployment"` | Response contains session_id or "accepted" | IT |
| IT-KA-002 | BR-MCP-001 | kubernaut_poll_investigation against real KA | Valid session_id from start | Non-empty response (status or findings) | IT |
| IT-KA-003 | BR-DS-001 | kubernaut_list_workflows routes to DS (not KA) | No args | Returns workflow list from DataStorage | IT |
| IT-KA-004 | BR-DECISION-001 | kubernaut_present_decision proxies to KA MCP | Decision payload | KA acknowledges or returns decision state | IT |
| IT-KA-005 | BR-MCP-001 | Poll with nonexistent session_id returns "not found" | Random UUID | Error containing "not found" or "404" | IT |
| IT-KA-006 | BR-MCP-001 | kubernaut_select_workflow dispatches to KA MCP | `rr_id, workflow_id` | Response contains "selected" or KA error | IT |

---

### 5.5 Category E: DS Query Tool Dispatch (BR-DS-001)

These specs exercise the real DataStorage container (with PostgreSQL + Redis).

| TC ID | BR | Description | Input | Expected Result | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-DS-001 | BR-DS-001 | kubernaut_list_workflows returns workflow list from DS | No filter | List (possibly empty) with count field | IT |
| IT-DS-002 | BR-DS-001 | kubernaut_get_remediation_history queries DS | `namespace: "default"` | Empty history or error (no data seeded) | IT |
| IT-DS-003 | BR-DS-001 | kubernaut_get_effectiveness returns metrics from DS | `namespace: "default"` | Response with effectiveness data or empty | IT |
| IT-DS-004 | BR-DS-001 | kubernaut_get_audit_trail returns audit records from DS | `rr_name: "test-rr"` | Response with audit entries or empty | IT |

---

### 5.6 Category F: RBAC Enforcement (BR-RBAC-001)

| TC ID | BR | Description | Input | Expected Result | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-RBAC-001 | BR-RBAC-001 | User with wildcard `*` role can call any tool | `sre` group -> `*` tools | Tool executes successfully | IT |
| IT-RBAC-002 | BR-RBAC-001 | User with `viewer` role denied write tool | `viewer` group denied `kubernaut_start_investigation` | Error "permission denied" | IT |
| IT-RBAC-003 | BR-RBAC-001 | Nil user (unauthenticated) rejected | No UserIdentity in context | Error "authentication required" | IT |
| IT-RBAC-004 | BR-OBS-001 | RBAC denial increments RBACDeniedTotal metric | Denied call | `RBACDeniedTotal{tool="..."}` == 1 | IT |
| IT-RBAC-005 | BR-RBAC-001 | Partial-role grants read-only tools, denies write tools | `readonly` role with limited tool list | Read tools succeed; write tools return "permission denied" | IT |

---

### 5.7 Category G: Resilience — Circuit Breaker + Timeout (BR-RESIL-001)

Uses TCP proxy pattern (kubernaut PR #828, commit `26d7f17b`):
```
ka.Client -> tcpProxy(127.0.0.1:ephemeral) -> KA container(14404)
```

Dedicated MCP handler with `CBFailureThreshold=3, CBTimeout=200ms`.

| TC ID | BR | Description | Input | Expected Result | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-RESIL-001 | BR-RESIL-001 | Baseline call through TCP proxy succeeds | Normal proxy | Tool call succeeds (proxy transparent) | IT |
| IT-RESIL-002 | BR-RESIL-001 | proxy.Disconnect() -> N failures trip CB | 3+ failures | Error contains "unavailable"; CB transitions to Open | IT |
| IT-RESIL-003 | BR-RESIL-001 | CB open -> fast-fail without hitting KA | Call after trip | Error returned immediately (no network attempt) | IT |
| IT-RESIL-004 | BR-RESIL-001 | Reconnect proxy -> half-open probe succeeds | Wait CBTimeout, reconnect proxy | Next call succeeds; CB transitions to Closed | IT |
| IT-RESIL-005 | BR-RESIL-001 | DS CB independent from KA CB | KA CB open | DS tool still succeeds normally | IT |
| IT-RESIL-006 | BR-RESIL-001 | Tool timeout interrupts long operation | ToolTimeout=100ms, slow KA response | Error "context deadline exceeded" or "timeout" | IT |
| IT-RESIL-007 | BR-RESIL-001 | Semaphore exhaustion returns "server busy" | MaxConcurrentTools=1, 2 concurrent calls | Second call gets "server busy" or "throttled" | IT |

---

### 5.8 Category H: Metrics and Audit Fidelity (BR-OBS-001, BR-AUDIT-001)

| TC ID | BR | Description | Input | Expected Result | Type |
|-------|-----|-------------|-------|-----------------|------|
| IT-METRICS-001 | BR-OBS-001 | Successful tool call increments ToolCallsTotal{result=success} | Any passing tool call | Counter value == 1 | IT |
| IT-METRICS-002 | BR-OBS-001 | Failed tool call increments ToolCallsTotal{result=error} | Invalid tool input causing error | Counter value == 1 | IT |
| IT-METRICS-003 | BR-OBS-001 | Duration histogram has observations after call | Any tool call | Histogram count > 0 | IT |
| IT-AUDIT-001 | BR-AUDIT-001 | Audit event contains correct fields | Any tool call | Event has: Type == `mcp.tool_invoked`, UserID, Detail["tool"], Timestamp (recent, not future) | IT |

---

## 6. Test Environment

- Go 1.26+, Ginkgo v2.28/Gomega
- envtest (etcd + kube-apiserver from `setup-envtest`)
- Podman 5.x for containers
- No external cluster required
- `go test -race -count=1 -timeout=5m`
- `KUBEBUILDER_ASSETS` set from `setup-envtest use -p path`

---

## 7. Schedule

| Phase | Description | Specs |
|-------|-------------|-------|
| Phase 0 | envtest CRD loading + real dynamic client + fixture seeding | IT-ENV-001..003 |
| Phase 1 | Protocol/session specs | IT-PROTO-001..004 |
| Phase 2 | K8s CRD tool specs | IT-CRD-001..006 |
| Phase 3 | AF triage tool specs | IT-TRIAGE-001..006 |
| Phase 4 | KA error mode + DS dispatch specs | IT-KA-001..005, IT-DS-001..004 |
| Phase 5 | RBAC enforcement specs | IT-RBAC-001..004 |
| Phase 6 | Resilience specs (TCP proxy) | IT-RESIL-001..007 |
| Phase 7 | Metrics + audit fidelity specs | IT-METRICS-001..003, IT-AUDIT-001 |
| REFACTOR | 100-go-mistakes audit + lint | — |

---

## 8. Risks

| Risk | Impact | Mitigation |
|------|--------|-----------|
| CRD YAML read from module cache (read-only) | envtest may fail if trying to write to CRD dir | envtest only reads CRDs; verified files are `r--r--r--` |
| KA container may not serve all REST endpoints with mock LLM | Some tool responses may be partial | Accept any non-error response as success (behavioral, not content) |
| TCP proxy reconnect timing in CI | Flaky resilience specs | Use generous CBTimeout (200ms) + Eventually with 5s timeout |
| envtest watch may buffer events differently | IT-CRD-006 (watch) may need Eventually | Use Gomega Eventually with 3s timeout for watch assertions |
| `kubernaut_select_workflow` uses KAMCPClient (mock interface) | Not fully integrated | Explicitly documented as mock; KA MCP SDK endpoint not containerized |

---

## 9. Traceability Matrix

### 9.1 BR → TC Mapping

| BR ID | TCs Covering | Count |
|-------|-------------|-------|
| BR-MCP-001 | IT-PROTO-001..004, IT-KA-001..005 | 9 |
| BR-CRD-001 | IT-ENV-003, IT-CRD-001..006 | 7 |
| BR-DS-001 | IT-DS-001..004 | 4 |
| BR-AUTH-001 | IT-ENV-001..002 | 2 |
| BR-RESIL-001 | IT-RESIL-001..007 | 7 |
| BR-OBS-001 | IT-METRICS-001..003, IT-RBAC-004 | 4 |
| BR-AUDIT-001 | IT-PROTO-002, IT-AUDIT-001 | 2 |
| BR-RBAC-001 | IT-RBAC-001..004 | 4 |
| BR-TRIAGE-001 | IT-TRIAGE-001..004 | 4 |
| BR-DEDUP-001 | IT-TRIAGE-005..006 | 2 |
| BR-DECISION-001 | IT-KA-004 | 1 |
| BR-E2E-001 | (All — IT validates components before E2E) | 47 |

### 9.2 TC → Implementation File Mapping

| TC ID | Test File | Spec Description |
|-------|-----------|------------------|
| IT-ENV-001 | `test/integration/suite_test.go` | SynchronizedBeforeSuite: CRD loading |
| IT-ENV-002 | `test/integration/suite_test.go` | SynchronizedBeforeSuite: dynamic client |
| IT-ENV-003 | `test/integration/fixtures_test.go` | seedK8sFixtures() |
| IT-PROTO-001 | `test/integration/bridge_test.go` | "tools/list returns all 20 tools" |
| IT-PROTO-002 | `test/integration/bridge_test.go` | "session isolation produces independent audit" |
| IT-PROTO-003 | `test/integration/bridge_test.go` | "invalid JSON-RPC returns parse error" |
| IT-PROTO-004 | `test/integration/bridge_test.go` | "uninitialized session rejected" |
| IT-CRD-001 | `test/integration/bridge_test.go` | "list_remediations returns seeded RRs" |
| IT-CRD-002 | `test/integration/bridge_test.go` | "get_remediation with valid name" |
| IT-CRD-003 | `test/integration/bridge_test.go` | "get_remediation with invalid name" |
| IT-CRD-004 | `test/integration/bridge_test.go` | "submit_signal creates SP" |
| IT-CRD-005 | `test/integration/bridge_test.go` | "approve transitions RAR phase" |
| IT-CRD-006 | `test/integration/bridge_test.go` | "cancel marks RR cancelled" |
| IT-TRIAGE-001 | `test/integration/bridge_test.go` | "af_list_events returns seeded events" |
| IT-TRIAGE-002 | `test/integration/bridge_test.go` | "af_get_pods returns pods" |
| IT-TRIAGE-003 | `test/integration/bridge_test.go` | "af_get_workloads returns deployments" |
| IT-TRIAGE-004 | `test/integration/bridge_test.go` | "af_resolve_owner resolves chain" |
| IT-TRIAGE-005 | `test/integration/bridge_test.go` | "af_check_existing_rr finds RR" |
| IT-TRIAGE-006 | `test/integration/bridge_test.go` | "af_create_rr creates RR" |
| IT-KA-001 | `test/integration/bridge_test.go` | "start_investigation dispatches to KA" |
| IT-KA-002 | `test/integration/bridge_test.go` | "poll_investigation against real KA" |
| IT-KA-003 | `test/integration/bridge_test.go` | "list_workflows queries KA" |
| IT-KA-004 | `test/integration/bridge_test.go` | "present_decision proxies to KA" |
| IT-KA-005 | `test/integration/bridge_test.go` | "poll nonexistent session" |
| IT-DS-001 | `test/integration/bridge_test.go` | "list_workflows from DS" |
| IT-DS-002 | `test/integration/bridge_test.go` | "get_remediation_history from DS" |
| IT-DS-003 | `test/integration/bridge_test.go` | "get_effectiveness from DS" |
| IT-DS-004 | `test/integration/bridge_test.go` | "get_audit_trail from DS" |
| IT-RBAC-001 | `test/integration/bridge_test.go` | "wildcard role allows all" |
| IT-RBAC-002 | `test/integration/bridge_test.go` | "viewer role denied write tool" |
| IT-RBAC-003 | `test/integration/bridge_test.go` | "nil user rejected" |
| IT-RBAC-004 | `test/integration/bridge_test.go` | "denial increments metric" |
| IT-RESIL-001 | `test/integration/resilience_test.go` | "baseline through proxy" |
| IT-RESIL-002 | `test/integration/resilience_test.go` | "disconnect trips CB" |
| IT-RESIL-003 | `test/integration/resilience_test.go` | "CB open fast-fails" |
| IT-RESIL-004 | `test/integration/resilience_test.go` | "reconnect recovers CB" |
| IT-RESIL-005 | `test/integration/resilience_test.go` | "DS independent from KA CB" |
| IT-RESIL-006 | `test/integration/resilience_test.go` | "tool timeout interrupts" |
| IT-RESIL-007 | `test/integration/resilience_test.go` | "semaphore exhaustion" |
| IT-METRICS-001 | `test/integration/bridge_test.go` | "success increments counter" |
| IT-METRICS-002 | `test/integration/bridge_test.go` | "error increments counter" |
| IT-METRICS-003 | `test/integration/bridge_test.go` | "duration histogram populated" |
| IT-AUDIT-001 | `test/integration/bridge_test.go` | "audit event fields correct" |

---

## 10. TDD Refactor: 100 Go Mistakes Audit

*To be populated during REFACTOR phase.*

---

## 11. Execution Results

*To be populated after implementation.*

| Phase | Specs | Status | Duration | Coverage |
|-------|-------|--------|----------|----------|
| 0 (envtest) | 3 | | | |
| 1 (protocol) | 4 | | | |
| 2 (CRD) | 6 | | | |
| 3 (triage) | 6 | | | |
| 4 (KA+DS) | 9 | | | |
| 5 (RBAC) | 4 | | | |
| 6 (resilience) | 7 | | | |
| 7 (metrics/audit) | 4 | | | |
| **Total** | **43 new + 4 existing = 47** | | | **>=80%** |

---

## 12. Checkpoint Audit

*GA readiness audit to be performed after all phases complete.*

| # | Dimension | Score | Notes |
|---|-----------|-------|-------|
| 1 | Correctness | | |
| 2 | Coverage | | |
| 3 | Lint Compliance | | |
| 4 | Security (AppSec) | | |
| 5 | API Contract | | |
| 6 | Observability | | |
| 7 | Resilience | | |
| 8 | FedRAMP Alignment | | |
| 9 | Test Quality (QE) | | |
| 10 | Test Documentation | | |
| 11 | Regression Safety | | |
| 12 | Product Acceptance | | |
| 13 | UX / DX | | |
| 14 | Product Security | | |
| **Average** | | | |
| **Confidence** | | | |
