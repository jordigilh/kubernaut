# RB-AF-004: ApifrontendAuthFailureSpike

## Alert

`ApifrontendAuthFailureSpike` — 401 rate > 10% of total requests for > 2 minutes.

## Symptoms

- Legitimate users unable to authenticate
- Burst of 401 responses in metrics
- Possible IdP outage or credential rotation issues

## Diagnosis

1. Check JWT validator readiness:
   ```bash
   curl -s http://<af-pod>:8443/readyz | jq .
   ```

2. Check if OIDC issuer is reachable from the cluster:
   ```bash
   kubectl exec -it <af-pod> -n kubernaut -- wget -q -O- <issuer-url>/.well-known/openid-configuration
   ```

3. Check if JWKS cache is stale (provider limiter may be throttling fetches):
   ```promql
   af_auth_duration_seconds{result="failure"}
   ```

4. Check audit trail for patterns (single IP, single user, or distributed):
   ```bash
   kubectl logs -l app.kubernetes.io/name=kubernaut-apifrontend -n kubernaut | grep "auth_denied"
   ```

## Resolution

1. If IdP is down → wait for recovery; circuit breaker on JWKS fetches prevents retry storms
2. If token audience mismatch (post-IdP config change) → update `auth.audience` in ConfigMap and restart
3. If brute-force attack → IP rate limiter should handle; escalate to security team if sustained
4. If JWKS rotation → trigger manual JWKS refresh by restarting AF pods

## Prevention

- Configure multiple JWT providers for resilience
- Monitor IdP SLOs independently
- Set up IP-based blocking at the ingress layer for known bad actors

## Escalation (FedRAMP IR-4)

If pattern indicates credential stuffing or brute-force attack:
1. Notify security team immediately
2. Preserve audit logs as evidence
3. Consider temporary IP block at ingress/WAF level
