# RB-AF-012: Sustained Attack Pattern Detected

## Description

The `ApifrontendSustainedAttackPattern` alert fires when both rate-limit
rejections and HTTP 401 responses remain elevated simultaneously for 5+ minutes.
This pattern is a strong indicator of credential stuffing, brute force, or
automated attack tooling targeting the API Frontend.

## Indicators

- `af_rate_limit_rejections_total` rate > 50/min
- `af_http_requests_total{status="401"}` rate > 100/min
- Correlation: same source IPs appear in both rate-limited and 401 responses

## Probable Cause

1. **Credential stuffing** — automated tool trying leaked credentials.
2. **Brute force attack** — repeated auth attempts with variations.
3. **Misconfigured client** — legitimate client with expired credentials in a
   retry loop.
4. **Token replay attempt** — attacker reusing captured tokens (mitigated by
   JTI replay protection).

## Triage Steps

1. Identify top source IPs:
   ```
   topk(10, sum by (source_ip) (rate(af_http_requests_total{status="401"}[5m])))
   ```

2. Check if replay protection is triggering:
   ```
   rate(af_http_requests_total{status="401"}[5m])
   ```
   Cross-reference with audit logs for `reason: "token_replayed"`.

3. Review rate-limit tier distribution:
   ```
   sum by (tier) (rate(af_rate_limit_rejections_total[5m]))
   ```

4. Check audit trail for attack pattern:
   ```
   kubectl logs -l app=apifrontend | jq 'select(.type=="auth_failure")'
   ```

## Response

### Immediate (P1 if sustained > 15min)

1. **Do NOT block IPs at the application level** — use the upstream WAF or
   cloud provider firewall for IP blocking.
2. Verify rate limits are functioning correctly (the AF is already protecting
   itself via the IP tier limiter).
3. If attack volume threatens service stability, enable stricter rate limits
   via config hot-reload:
   ```yaml
   rateLimit:
     perIP:
       requestsPerSecond: 2  # reduce from default 10
       burst: 5
   ```

### Post-Incident

1. Collect affected source IPs for threat intelligence.
2. Verify no successful authentications from attacking IPs.
3. If token replay was detected, confirm JTI cache is functioning and
   consider reducing token lifetime.

## Escalation

- If attack sustains > 30 minutes at high volume: escalate to Security team (P1).
- If any successful auth from attacking IPs: immediate incident — escalate to
  Security + Identity teams.

## References

- [Cycle B: SEC-05 — ErrTokenReplayed sentinel](../../../docs/test/cycle-b-security-hardening/TEST_PLAN.md)
- [Rate Limiting Configuration](../../ARCHITECTURE.md)
