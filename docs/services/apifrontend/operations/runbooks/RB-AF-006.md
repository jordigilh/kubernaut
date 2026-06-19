# RB-AF-006: ApifrontendAuditBufferOverflow

## Alert

`ApifrontendAuditBufferOverflow` — Audit events being dropped (> 0.1/s overflow rate for > 1 minute).

## Symptoms

- `audit_events_dropped_total{service="apifrontend"}` counter incrementing (from `pkg/audit.BufferedAuditStore`)
- Audit events may be lost — FedRAMP AU-11 compliance at risk
- Likely indicates DS connectivity issues

## Diagnosis

1. Check DS circuit breaker state:
   ```promql
   af_circuit_breaker_state{dependency="ds"}
   ```

2. Check DS latency:
   ```promql
   histogram_quantile(0.99, rate(af_downstream_request_duration_seconds_bucket{dependency="ds"}[5m]))
   ```

3. Check DS pod health:
   ```bash
   kubectl get pods -l app.kubernetes.io/name=data-storage -n kubernaut
   kubectl logs -l app.kubernetes.io/name=data-storage --tail=50 -n kubernaut
   ```

4. Check AF buffer metrics:
   ```promql
   rate(audit_events_dropped_total{service="apifrontend"}[5m])
   ```

## Resolution

1. If DS is slow → scale DS or increase its resource limits
2. If DS is down → restore DS service; buffered events will flush once reconnected (within buffer capacity)
3. If sustained high load → increase AF audit buffer size in code (default: 10000; see `audit.DefaultConfig()`)
4. Emergency: if audit loss is unacceptable, consider draining AF traffic until DS recovers

## Prevention

- Size DS to handle peak audit event throughput (100 events/batch × 5s interval = 20 batches/min)
- Set up proactive DS latency alerting at a lower threshold
- Consider persistent queue (future enhancement) for guaranteed delivery

## Escalation (FedRAMP AU-11)

Audit event loss in a FedRAMP environment is a compliance finding:
1. Document the time window of potential event loss
2. Notify compliance team within 24 hours
3. Preserve AF and DS logs for the incident period
