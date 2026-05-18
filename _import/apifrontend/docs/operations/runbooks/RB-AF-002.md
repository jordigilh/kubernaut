# RB-AF-002: ApifrontendHighLatencyP95 / P99

## Alert

- `ApifrontendHighLatencyP95` — P95 latency > 500ms for > 2 minutes
- `ApifrontendHighLatencyP99` — P99 latency > 1s for > 2 minutes

## Symptoms

- Users observe slow responses on A2A/MCP operations
- SLO-1 / SLO-2 budget being consumed

## Diagnosis

1. Identify slow endpoints:
   ```promql
   histogram_quantile(0.95, sum by (le, path) (rate(af_http_request_duration_seconds_bucket[5m])))
   ```

2. Check downstream dependency latency:
   ```promql
   histogram_quantile(0.95, sum by (le, dependency) (rate(af_downstream_request_duration_seconds_bucket[5m])))
   ```

3. Check if circuit breakers are tripping:
   ```promql
   af_circuit_breaker_state{job="apifrontend"}
   ```
   (0=closed, 1=half-open, 2=open)

4. Check goroutine count for resource exhaustion:
   ```promql
   go_goroutines{job="apifrontend"}
   ```

## Resolution

1. If downstream KA latency is the cause → investigate KA service health
2. If DS latency is the cause → check DataStorage pod resources and DB connections
3. If K8s API latency → check API server audit logs and etcd performance
4. If all downstreams healthy → check AF CPU throttling or memory pressure

## Prevention

- Ensure circuit breaker timeouts are tuned to prevent cascading failures
- Monitor SLO burn rate dashboards proactively
- Scale AF horizontally if sustained traffic exceeds single-pod capacity
