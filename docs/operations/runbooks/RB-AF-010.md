# RB-AF-010: Severity Triage Troubleshooting

**Alert:** `ApifrontendSeverityTriageErrorRate`
**Severity:** warning
**Service:** kubernaut-apifrontend
**Packages:** `internal/severity/`, `internal/prometheus/`

---

## Symptoms

- `af_severity_triage_errors_total` counter rising
- `af_create_rr` or `kubernaut_submit_signal` returning errors when severity is not user-supplied
- Audit events `severity.triage.failed` appearing in the audit trail
- Users reporting "triage failed" errors when creating remediations without explicit severity

## Triage Pipeline Overview

The severity triage pipeline runs five tiers in order:

```
Tier 1: Prometheus /api/v1/alerts (firing alerts)
  ↓ miss
Tier 1.5: Prometheus /api/v1/rules (pending rules, cached)
  ↓ miss
Tier 2: Prometheus /api/v1/query (instant query per matching rule)
  ↓ miss
Tier 2.5: LLM with rule context
  ↓ miss
Tier 3: Pure LLM fallback
  ↓ error → propagated to caller
```

## Diagnostic Steps

### 1. Check which tier is failing

```promql
# Error breakdown by tier
rate(af_severity_triage_errors_total[5m])
```

| Tier | Failure Meaning |
|------|----------------|
| 1 | Prometheus `/api/v1/alerts` unreachable or returning errors |
| 1.5 | Prometheus `/api/v1/rules` unreachable (or cache miss + fetch failed) |
| 2 | Prometheus `/api/v1/query` failing for instant queries |
| 2.5 | LLM call with rule context failed |
| 3 | LLM pure fallback failed — **this is the terminal error** |

### 2. Check Prometheus connectivity

```bash
# From AF pod
curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
  http://prometheus:9090/api/v1/alerts
```

Expected: HTTP 200. If not:
- Check network connectivity (NetworkPolicy, DNS)
- Check bearer token validity
- Check Prometheus health

### 3. Check LLM connectivity

If Tier 3 is failing, the LLM provider is unreachable:
- Check Vertex AI credentials (Workload Identity, ADC)
- Check GCP project/region configuration
- Check LLM circuit breaker state: `af_circuit_breaker_state{dependency="llm"}`

### 4. Check AF logs

```bash
kubectl logs -l app.kubernetes.io/name=kubernaut-apifrontend -c apifrontend | \
  grep -E "Tier [0-9].*failed|LLM.*failed|triage"
```

Key log messages:
- `"Tier 1 failed, continuing"` — Prometheus alerts API error (non-fatal)
- `"skipping Tier 1.5: rules fetch failed"` — Rules fetch failed (non-fatal)
- `"skipping Tier 2: rules fetch failed"` — Same as above
- `"Tier 2 query failed"` — Individual instant query failed (non-fatal)
- `"Tier 2.5 LLM failed"` — LLM with rule context failed (falls to Tier 3)
- `"tier 3 LLM triage failed"` — Terminal error (propagated to caller)

### 5. Check configuration

```bash
kubectl get configmap apifrontend-config -o yaml | grep -A 10 severityTriage
```

Verify:
- `enabled: true`
- `prometheusURL` is correct and reachable
- `cacheTTLSeconds` is reasonable (default: 30)
- `maxQueriesPerCall` is not set too low

## Resolution

| Root Cause | Fix |
|-----------|-----|
| Prometheus unreachable | Check NetworkPolicy, DNS, Service endpoints |
| Prometheus returning 5xx | Check Prometheus health, disk space, memory |
| Bearer token expired | Check projected volume mount, kubelet token rotation |
| TLS certificate mismatch | Verify `prometheus.tlsCaFile` matches Prometheus server cert |
| LLM provider down | Check Vertex AI status, credentials, circuit breaker state |
| LLM rate limited | Check `MaxLLMConcurrency` configuration |
| Config missing `prometheusURL` | AF won't start if triage is enabled without URL |

## Escalation

If both Prometheus and LLM are healthy but triage still fails:
1. Check `af_severity_triage_duration_seconds` for timeouts
2. Check for PromQL parsing errors in Tier 2 (may indicate rule format changes)
3. Escalate to the kubernaut team with AF logs and `af_severity_triage_*` metric snapshots

---

*Related: `docs/design/SEVERITY_TRIAGE.md`, `docs/adr/ADR-021-severity-triage.md`*
