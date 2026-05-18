# RB-AF-011: Auth Middleware Latency High

## Description

The `ApifrontendAuthLatencyHigh` alert fires when the P99 authentication
middleware latency exceeds 1 second for 5 minutes. This indicates JWT validation
is slower than expected — typically caused by JWKS fetch issues or network
degradation to the identity provider.

## Probable Cause

1. **JWKS endpoint slow or unreachable** — the JWKS circuit breaker may be
   half-open, causing retries on each request.
2. **Certificate validation issues** — TLS handshake to JWKS endpoint is slow
   due to OCSP stapling or CRL downloads.
3. **CPU saturation** — RSA signature verification under high request rate.
4. **Network policy blocking egress** — partial connectivity to the OIDC issuer.

## Triage Steps

1. Check the JWKS circuit breaker state:
   ```
   af_circuit_breaker_state{dependency=~"jwks_.*"}
   ```
   Value `2` (open) means fetches are failing. Value `1` (half-open) means
   recovery is being attempted.

2. Check auth duration distribution:
   ```
   histogram_quantile(0.99, rate(af_auth_duration_seconds_bucket[5m]))
   ```

3. Review pod logs for JWKS fetch errors:
   ```
   kubectl logs -l app=apifrontend -c apifrontend | grep "JWKS"
   ```

4. Verify network connectivity to the OIDC issuer:
   ```
   kubectl exec -it deploy/apifrontend -- curl -v <issuer-url>/.well-known/openid-configuration
   ```

## Resolution

- If JWKS endpoint is down: wait for circuit breaker recovery (30s timeout).
  Cached keys will serve existing sessions (fail-open).
- If network policy blocks egress: verify the `allow-egress-to-dex` NetworkPolicy
  is applied and the issuer IP hasn't changed.
- If CPU saturation: scale horizontally via HPA.

## Escalation

- If unresolved after 15 minutes: escalate to the Identity team for OIDC
  issuer investigation.
- If circuit breaker remains open: page the on-call SRE (P2).

## References

- [ARCHITECTURE.md §5 — Authentication Flow](../../ARCHITECTURE.md)
- [Cycle A: WIRE-08 — JWKS CB metrics wiring](../../../docs/test/cycle-a-operational-wiring/TEST_PLAN.md)
