# Performance Test Plan — kubernaut API Frontend

**Version:** 1.0
**Date:** 2026-05-06
**Status:** Investigation complete — execution deferred until proper hardware

---

## 1. Load Profiles

| Profile | Concurrent Users | Tool Calls/sec | SSE Streams | Duration | Purpose |
|---------|-----------------|----------------|-------------|----------|---------|
| Baseline | 10 | 5 | 10 | 5m | Establish idle resource usage |
| Normal | 50 | 25 | 50 | 15m | Typical production load |
| Peak | 200 | 100 | 200 | 15m | Expected traffic spikes |
| Stress | 500 | 250 | 500 | 30m | Find breaking point |
| Soak | 50 | 25 | 50 | 4h | Detect memory leaks, resource exhaustion |

---

## 2. Tool Recommendation

**Selected: k6 (Grafana Labs)**

### Rationale

| Criterion | k6 | vegeta | locust | Custom Go |
|-----------|----|---------|---------|-----------
| HTTP/SSE support | Native | HTTP only | Plugin | Full control |
| Protocol-aware (JSON-RPC) | Scriptable | No | Scriptable | Full |
| Cloud execution | k6 Cloud | N/A | Distributed | Custom infra |
| CI integration | CLI, exit codes | CLI | Complex | Native |
| Learning curve | Low (JS) | Low | Medium (Py) | High |
| Grafana integration | Native | Manual | Manual | Manual |
| Threshold assertions | Built-in | External | External | Custom |

k6 is selected per ADR-010 for:
- JavaScript scripting with full HTTP/WebSocket/SSE support
- Built-in threshold assertions that map to SLO targets
- Native Grafana Cloud integration for result visualization
- Single binary, easy CI integration
- Active community with MCP/JSON-RPC examples

---

## 3. Metrics to Capture

### Linked to SLO Definitions (docs/slo/SLO_DEFINITIONS.md)

| Metric | SLO | k6 Metric Name | Threshold |
|--------|-----|----------------|-----------|
| HTTP request latency (p95) | SLO-1 | `http_req_duration{p(95)}` | < 500ms |
| HTTP request latency (p99) | SLO-2 | `http_req_duration{p(99)}` | < 1000ms |
| Tool call latency (p99) | SLO-3/4 | tagged by tool type | < 500ms (CRD), < 2s (proxy) |
| Auth latency (p99) | SLO-5 | tagged by endpoint | < 200ms |
| Error rate | SLO-6 | `http_req_failed` | < 0.1% |
| Agent Card latency (p99) | SLO-7 | tagged by path | < 50ms |

### System Metrics (collected via Prometheus during test)

- `af_sessions_active` — concurrent session count
- `af_circuit_breaker_state` — trip events under load
- `process_resident_memory_bytes` — memory growth during soak
- `go_goroutines` — goroutine count stability
- Container CPU usage (cAdvisor)
- Container memory RSS (cAdvisor)

---

## 4. Hardware Requirements

### Minimum for Meaningful Results

| Resource | Specification | Justification |
|----------|--------------|---------------|
| CPU | 4 cores (dedicated, no throttling) | Avoid CPU contention skewing latency |
| RAM | 8 GB | Service + k6 + Prometheus + system |
| Disk | SSD, 100 IOPS sustained | Log/metric writes during soak |
| Network | 1 Gbps between k6 and target | SSE streaming saturation test |

### Deployment Options

| Option | Sufficient? | Notes |
|--------|-------------|-------|
| Kind cluster (local) | Baseline/Normal only | Resource contention above 50 VUs |
| Dedicated OCP cluster | All profiles | Required for Peak/Stress/Soak |
| k6 Cloud + remote target | All profiles | Recommended for reproducibility |

### Estimated Availability

- Kind cluster: Available now (developer laptop)
- OCP cluster: Requires provisioning (1-2 week lead time)
- k6 Cloud: SaaS, available immediately with account

---

## 5. Test Scenarios

### 5.1 Health Endpoint Baseline

Establish absolute minimum latency floor using `/healthz` (no auth, no business logic).

### 5.2 MCP tools/call Under Load

Authenticated `POST /mcp` with `tools/call` payloads. Measures:
- JWT validation throughput
- Rate limiting behavior
- Tool dispatch latency

### 5.3 SSE Stream Stability

Open N SSE connections via `POST /mcp` with streaming Accept header. Hold for test duration. Measure:
- Stream establishment latency
- Event delivery latency
- Memory growth per connection
- Graceful handling of connection drops

### 5.4 Mixed Workload

Combined scenario simulating real production:
- 40% tool calls
- 30% SSE streams
- 20% Agent Card requests
- 10% health/ready probes

---

## 6. SLO Validation Approach

1. Run Normal profile for 15 minutes
2. Calculate P50/P95/P99 from k6 summary
3. Compare against SLO targets in SLO_DEFINITIONS.md
4. Record the load level at which each SLO is first breached
5. Document capacity ceiling (max VUs before SLO breach)

---

## 7. Execution Plan (Deferred)

| Phase | When | Action |
|-------|------|--------|
| 1. Script validation | Now | Verify k6 scripts run against local Kind |
| 2. Baseline measurement | Post-deploy to OCP | Baseline + Normal profiles |
| 3. SLO validation | Post-baseline | Compare measured vs aspirational SLOs |
| 4. Stress testing | Post-SLO-validation | Stress + Soak profiles |
| 5. SLO adjustment | Post-measurement | Update SLO_DEFINITIONS.md with measured values |

---

## Dependencies

- Issue #41 — SLO definitions (targets to validate)
- Issue #11 — Observability (metrics to capture)
- Issue #3 — MCP tool surface (tools to load test)
- OPS-2 — Hardware provisioning for stress/soak
