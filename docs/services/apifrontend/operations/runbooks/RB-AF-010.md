# RB-AF-010: Severity Triage Troubleshooting

**Alert:** `ApifrontendSeverityTriageErrorRate`
**Severity:** warning
**Service:** kubernaut-apifrontend
**Packages:** `pkg/apifrontend/severity/`, `pkg/apifrontend/tools/`, Prometheus client

---

## Symptoms

- `af_severity_triage_errors_total` counter rising
- `af_create_rr` returning errors when no explicit, unambiguous alert/rule severity is available
- Audit events `severity.triage.failed` appearing in the audit trail
- Users reporting "triage failed" errors when creating remediations without explicit severity

## Triage Pipeline Overview

The severity triage pipeline accepts only explicit Prometheus severity labels:

```
Tier 1: Prometheus /api/v1/alerts (firing alerts; selected matches must agree on one exact label)
  ↓ miss
Tier 1.5: Prometheus /api/v1/rules (pending rules; matching candidates must agree on one exact label)
  ↓ miss
Tier 2: Correlated rule-only candidates for potential-issue investigation
  (all relevant rules must carry the same non-empty raw severity)
  ↓ no match / missing or conflicting severity → ErrSeverityUndetermined; no RR
```

> **DD-AF-010 / DD-AF-016:** Tier 3's ungrounded LLM fallback and Tier 2.5's
> rule-context LLM inference are both removed. AF passes an explicit raw
> alert/rule severity unchanged; it does not infer severity or map unknown
> source labels to `warning`. SP Rego owns canonical normalization. If no
> correlated alert/rule provides usable, unambiguous source evidence,
> `Triage()` returns `severity.ErrSeverityUndetermined` and AF creates no RR.
> This is expected fail-closed behavior, not an incident.

## Diagnostic Steps

### 1. Check which tier is failing

```promql
# Error breakdown by tier
rate(af_severity_triage_errors_total[5m])
```

| Tier | Failure Meaning |
|------|----------------|
| 1 | Prometheus `/api/v1/alerts` unreachable or returning errors |
| 1.5/2 | Prometheus `/api/v1/rules` unreachable, or candidate rule severity is missing/conflicting |
| — | `ErrSeverityUndetermined`: no correlated alert/rule supplies one explicit source severity — **expected fail-closed result, not a failure to diagnose** (DD-AF-016) |

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

### 3. Check rule severity labels

For a potential issue without a firing alert, inspect all Prometheus rules
correlated to the target. Every relevant candidate must carry the same,
non-empty `severity` label. Correct missing/conflicting labels, or retry once
the alert is pending/firing so the specific source is identified. Do not add an
AF-side default; the raw value is passed to SP Rego for canonical mapping.

### 4. Check AF logs

```bash
kubectl logs -l app.kubernetes.io/name=kubernaut-apifrontend -c apifrontend | \
  grep -E "Tier [0-9].*failed|triage"
```

Key log messages:
- `"Tier 1 failed, continuing"` — Prometheus alerts API error (non-fatal)
- `"skipping Tier 1.5: rules fetch failed"` — Rules fetch failed (non-fatal)
- `"skipping Tier 2: rules fetch failed"` — Same as above; no rule-only severity can be established
- `cannot determine severity: no correlated alert or rule supplies one explicit, unambiguous severity` (`ErrSeverityUndetermined`) — **expected fail-closed result, not an error to fix**; configure rule labels or retry when a specific alert becomes pending/firing (DD-AF-016)

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
| Config missing `prometheusURL` | AF won't start if triage is enabled without URL |
| `ErrSeverityUndetermined` (no explicit, unambiguous severity) | **Expected fail-closed behavior** — no correlated alert/rule supplies a usable raw severity, or rule-only candidates are missing/conflicting. Correct rule labels or retry when the specific alert is pending/firing. Do not add an AF fallback; see DD-AF-016. |

## Escalation

If Prometheus is healthy but triage still fails (excluding the expected `ErrSeverityUndetermined` fail-closed case above):
1. Check `af_severity_triage_duration_seconds` for timeouts
2. Check for PromQL parsing errors in Tier 2 (may indicate rule format changes)
3. Escalate to the kubernaut team with AF logs and `af_severity_triage_*` metric snapshots

---

*Related: `docs/tests/1282/TEST_PLAN.md` (signal grounding), [DD-AF-010](../../../../architecture/decisions/DD-AF-010-remove-ungrounded-severity-inference.md) (Tier 3 removal), [DD-AF-016](../../../../architecture/decisions/DD-AF-016-explicit-alert-severity-source.md) (explicit severity source)*
