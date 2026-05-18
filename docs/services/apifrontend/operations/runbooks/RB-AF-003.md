# RB-AF-003: ApifrontendHighErrorRate

## Alert

- `ApifrontendHighErrorRateWarning` — 5xx rate > 0.5% for > 2 minutes
- `ApifrontendHighErrorRate` — 5xx rate > 1% for > 2 minutes (SLO-6 breach)

## Symptoms

- Clients receive 500/502/503 responses
- Error budget for SLO-6 (< 0.1% error rate) being consumed rapidly

## Diagnosis

1. Identify error source by path:
   ```promql
   sum by (path, status) (rate(af_http_requests_total{status=~"5.."}[5m]))
   ```

2. Check AF pod logs for panics or unhandled errors:
   ```bash
   kubectl logs -l app.kubernetes.io/name=kubernaut-apifrontend --tail=200 -n kubernaut | grep -i "error\|panic"
   ```

3. Check if errors correlate with circuit breaker state:
   ```promql
   af_circuit_breaker_state == 2
   ```

4. Check rate limit rejections (429s are not 5xx but may indicate load issues):
   ```promql
   rate(af_rate_limit_rejections_total[5m])
   ```

## Resolution

1. If panics → identify stack trace, hotfix or rollback
2. If downstream failure → circuit breaker should auto-recover; monitor half-open transitions
3. If ConfigMap misconfiguration → fix and let hot-reload apply (for log-level/rate-limit) or restart (for other fields)
4. If deployment regression → rollback:
   ```bash
   kubectl rollout undo deployment/kubernaut-apifrontend -n kubernaut
   ```

## Prevention

- Comprehensive unit/integration test coverage (>80%)
- Canary deployments for new releases
- Circuit breakers prevent cascading failures from downstream outages
