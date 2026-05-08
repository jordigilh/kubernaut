# RB-AF-005: ApifrontendCircuitBreakerOpen

## Alert

`ApifrontendCircuitBreakerOpen` — Circuit breaker for a dependency is open for > 2 minutes.

## Symptoms

- Requests to the affected dependency fail-fast with 503
- Dependent features (tool calls, session creation, audit flush) are degraded
- `af_circuit_breaker_state{dependency="<name>"} == 2`

## Diagnosis

1. Identify which dependency is affected:
   ```promql
   af_circuit_breaker_state{job="apifrontend"} == 2
   ```

2. Check dependency health:
   - **KA** (Kubernaut Agent): `curl http://kubernaut-agent:8080/healthz`
   - **DS** (DataStorage): `curl http://data-storage:9090/healthz`
   - **K8s** API: `kubectl get --raw /healthz`

3. Check retry metrics to see failure pattern:
   ```promql
   rate(af_downstream_retry_total{dependency="<name>"}[5m])
   ```

4. Check downstream pod status:
   ```bash
   kubectl get pods -l app.kubernetes.io/name=<dependency> -n kubernaut
   ```

## Resolution

1. If dependency is recovering → CB will auto-transition to half-open after configured timeout (15-30s)
2. If dependency is permanently down → escalate to that service's on-call
3. If network partition → check NetworkPolicies and DNS resolution
4. If AF-side misconfiguration (wrong URL) → fix ConfigMap and restart AF

## Prevention

- Configure CB thresholds conservatively (allow for transient failures)
- Ensure dependencies have their own health monitoring
- Deploy multi-replica dependencies for resilience
