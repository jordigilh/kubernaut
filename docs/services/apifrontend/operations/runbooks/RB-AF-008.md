# RB-AF-008: ApifrontendRateLimitStorm / SSEConnectionsHigh

## Alert

- `ApifrontendRateLimitStorm` — Rate limit rejections > 10/s for > 2 minutes
- `ApifrontendSSEConnectionsHigh` — Active SSE connections > 100 for > 5 minutes

## Symptoms

- Legitimate users receiving 429 responses
- Connection pool exhaustion for SSE streams
- Possible DoS or misconfigured client

## Diagnosis

1. Identify rate limit tier and source:
   ```promql
   sum by (tier, reason) (rate(af_rate_limit_rejections_total[5m]))
   ```

2. For IP-tier storms, check top offending IPs (requires audit log query):
   ```bash
   kubectl logs -l app.kubernetes.io/name=kubernaut-apifrontend -n kubernaut | grep "rate_limit_denied" | jq .source_ip | sort | uniq -c | sort -rn | head -10
   ```

3. For SSE connection accumulation:
   ```promql
   af_sse_active_connections
   ```

4. Check if connections are being properly closed:
   ```bash
   kubectl exec <af-pod> -n kubernaut -- ss -s
   ```

## Resolution

1. If single-IP DDoS → block at ingress/WAF level
2. If misconfigured client (retry storm) → identify client and fix retry logic
3. If legitimate traffic spike → increase rate limits via hot-reload:
   ```yaml
   rateLimit:
     ipRequestsPerSec: 200
     userRequestsPerSec: 100
   ```
4. If SSE leak → restart AF pods (graceful drain handles in-flight)

## Prevention

- Implement exponential backoff guidance in API docs
- Set connection limits at the ingress controller level
- Monitor rate limit metrics as leading indicator of traffic anomalies
