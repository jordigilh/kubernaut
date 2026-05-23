# Spike: Run State Persistence Strategy for Ephemeral Pods

**Issue**: [#1206](https://github.com/jordigilh/kubernaut/issues/1206)
**Status**: COMPLETED
**Date**: 2026-05-20
**Recommendation**: **Option A — Accept Ephemeral (no persistence needed)**

## Objective

Evaluate whether the OAS Runtime's in-memory run state is sufficient for the
ephemeral pod model, or whether durable persistence is needed. The spike tests
KA's ability to detect runtime crashes mid-investigation and recover gracefully.

## Options Evaluated

| Option | Description | Complexity | Tested |
|--------|-------------|-----------|--------|
| **A** | Ephemeral in-memory (pod crash = run failure, KA retries) | None | YES |
| **B** | Persistent state (emptyDir / ConfigMap / annotation) | Medium | NO |
| **C** | Fully stateless runtime (KA owns all state via CRD) | Low | YES |

## Test Results

### Test 1: SSE Stream Drop Detection
KA consumes an SSE stream from the OAS Runtime. The runtime is killed mid-stream
after 250ms. KA receives `unexpected EOF` immediately.

| Metric | Value |
|--------|-------|
| Events received before crash | 5 (2 lifecycle + 3 message parts) |
| Detection mechanism | `unexpected EOF` from `bufio.Scanner` |
| Detection latency | ~251ms (= time to crash, not detection overhead) |
| Detection overhead | <1ms (EOF is synchronous with TCP RST) |

**Finding**: SSE stream EOF is an immediate, reliable crash signal. No polling needed.

### Test 2: GET /runs/{id} After Restart
A run is created, verified accessible, then the runtime is killed. A new runtime
instance starts. GET for the original run_id returns 404.

| Assertion | Result |
|-----------|--------|
| Run accessible before crash | PASS |
| GET /runs/{id} → 404 after restart | PASS |
| Lookup latency | ~323µs |

**Finding**: Confirms state is lost on restart. KA cannot resume — must retry.

### Test 3: Health Check Failure
After killing the runtime, `/healthz` becomes unreachable (`connection refused`).

| Metric | Value |
|--------|-------|
| Detection latency | ~234µs |
| Error | `dial tcp: connect: connection refused` |

**Finding**: Health check failure is even faster than SSE EOF for detecting crashes
when KA is not actively streaming.

### Test 4: Stateless Retry
After detecting a crash (via SSE EOF), KA creates a new run on a fresh runtime
instance with the same input. The retry succeeds independently.

| Metric | Value |
|--------|-------|
| Retry run creation latency | ~353µs |
| Retry status | completed |
| New run_id | YES (independent from failed run) |

**Finding**: Stateless retry is trivial. KA replays the same prompt; the agent
re-executes from scratch. No checkpoint/resume complexity.

### Test 5: Partial Output Preservation
KA preserves SSE events received before the crash for audit trail purposes.

| Metric | Value |
|--------|-------|
| Partial events preserved | 3 |
| Events content | "Analyzing pod status...", "Checking container logs...", "Identifying root cause..." |

KA constructs an audit record mapping to the existing error taxonomy:

```json
{
  "type": "infrastructure_failure",
  "error_code": "ERR_UPSTREAM_RUNTIME_CRASH",
  "retry_possible": true,
  "partial_events": 3,
  "partial_trajectory": [
    "Analyzing pod status...",
    "Checking container logs...",
    "Identifying root cause..."
  ],
  "action": "stateless_retry"
}
```

**Finding**: Partial trajectory is preserved for SOC2 audit compliance without any
persistence mechanism in the runtime itself.

### Test 6: Option A vs Option C Comparison

| Behavior | Option A (Ephemeral) | Option C (Stateless) |
|----------|---------------------|---------------------|
| GET /runs/{id} while alive | 200 OK (run state) | 404 (by design) |
| GET /runs/{id} after crash | connection refused | N/A (same) |
| In-flight status checks | YES | NO |
| KA complexity | Low | Low |
| Runtime complexity | Low (map + mutex) | Minimal (no storage) |

**Finding**: Option A provides useful in-flight status checks (e.g., KA polling run
status during long investigations) at negligible cost. Option C eliminates even that
capability. Option B was not tested because the added complexity (PVC lifecycle, mount
coordination, checkpoint serialization, recovery deserialization) provides no meaningful
benefit in the ephemeral pod model.

## Summary

| Metric | Value |
|--------|-------|
| Total tests | 15 |
| Passed | 15 |
| Failed | 0 |
| SSE drop detection | ~251ms (= crash time, ~0ms overhead) |
| Health check detection | ~234µs |
| GET after restart | 404 confirmed (~323µs) |
| Stateless retry | ~353µs |

## Recommendation: Option A (Ephemeral In-Memory)

### Rationale

1. **KA already owns durable state**: The `AgenticWorkflow` CRD status conditions
   track investigation progress. The runtime's in-memory state is a transient cache,
   not the source of truth.

2. **SSE EOF is a reliable crash signal**: KA detects runtime crashes within
   milliseconds via TCP connection reset. No additional heartbeat or watchdog needed.

3. **Stateless retry is the right recovery model**: Investigations are idempotent
   (re-running the same prompt produces equivalent results). Checkpoint/resume adds
   complexity without proportional benefit.

4. **Partial trajectory is preserved by KA**: Events received before the crash are
   buffered by KA for audit purposes. The runtime doesn't need to persist them.

5. **Same model as Goose**: The existing goose-server uses ephemeral state. This
   decision maintains behavioral parity, simplifying the migration.

6. **No infrastructure overhead**: No PVCs, ConfigMaps, or shared volumes needed.
   Pods remain truly ephemeral, which aligns with OpenShell's BYOC sandbox model.

### What KA Needs (v1.6 Implementation)

1. **Error code**: Add `ERR_UPSTREAM_RUNTIME_CRASH` to the error taxonomy with
   `retry_possible=true`.

2. **SSE consumer resilience**: The ACP client in KA must handle `unexpected EOF`
   from the SSE stream and translate it to a phase failure with retry.

3. **Retry budget**: Configurable max retries per investigation phase (e.g., 2
   retries with exponential backoff). Existing `maxConsecutiveFailures` in RO can
   be reused.

4. **Partial trajectory audit**: Buffer SSE events in KA's memory during streaming.
   On crash, emit an audit event with the partial trajectory before retrying.

### Why NOT Option B (Persistent State)

- **Pod restarts vs pod replacements**: `emptyDir` survives container restarts but
  not pod replacements (the common failure mode in K8s). To survive pod replacement,
  you'd need a PVC — which defeats the ephemeral pod model.
- **Checkpoint complexity**: Serializing agent state (conversation history, tool call
  context, SDK internal state) mid-loop is non-trivial and error-prone.
- **Recovery complexity**: Deserializing and resuming from a checkpoint requires the
  SDK to support deterministic replay, which it does not.
- **Marginal benefit**: For investigations taking 30-120 seconds, losing at most 2
  minutes of work and retrying is acceptable. Persistence would save 1-2 minutes per
  crash at the cost of significant engineering complexity.

### Why NOT Option C (Fully Stateless)

Option C is viable but loses `GET /runs/{id}` for in-flight monitoring. Since the
runtime already maintains a `map[string]*Run` for the ACP API, keeping it costs
nothing and provides useful observability.

## Files

- `run-state-persistence/main.go` — Test harness with 6 test scenarios
- `run-state-persistence/go.mod` — Isolated Go module
- `docs/architecture/spikes/SPIKE-RUN-STATE-PERSISTENCE.md` — This document
