# RB-AF-007: ApifrontendToolLatencyHigh / DependencyLatencyHigh

## Alert

- `ApifrontendToolLatencyHigh` — Tool call P99 > 5s for > 5 minutes
- `ApifrontendDependencyLatencyHigh` — Downstream P95 > 2s for > 5 minutes

## Symptoms

- User-facing operations (triage, remediation) are slow
- SSE streams may timeout waiting for tool results

## Diagnosis

1. Identify slow tools:
   ```promql
   histogram_quantile(0.99, sum by (le, tool) (rate(af_tool_call_duration_seconds_bucket[5m])))
   ```

2. Determine if it's a specific dependency:
   ```promql
   histogram_quantile(0.95, sum by (le, dependency) (rate(af_downstream_request_duration_seconds_bucket[5m])))
   ```

3. Check if LLM inference is the bottleneck (for proxy-type tools):
   ```bash
   kubectl logs -l app.kubernetes.io/name=kubernaut-apifrontend -n kubernaut | grep "tool_call_duration"
   ```

4. Check K8s API latency for CRD tools:
   ```promql
   apiserver_request_duration_seconds_bucket{resource="remediationrequests"}
   ```

## Resolution

1. If KA/DS dependency → check that service's performance
2. If K8s API → check etcd performance, API server resource utilization
3. If LLM inference → nothing AF can do; document and inform users
4. If specific tool consistently slow → consider timeout adjustment or caching

## Prevention

- Set per-tool SLO targets in SLO_DEFINITIONS.md
- Configure request timeouts per dependency in config.yaml
- Monitor downstream SLOs proactively
