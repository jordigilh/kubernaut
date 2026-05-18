# RB-AF-001: ApifrontendDown

## Alert

`ApifrontendDown` — API Frontend service has been unreachable for > 5 minutes.

## Symptoms

- `up{job="apifrontend"} == 0` persists beyond the 5-minute window
- Upstream clients (UI, A2A callers) receive connection refused or timeout errors
- No new entries in the DS audit log from AF

## Diagnosis

1. Check pod status:
   ```bash
   kubectl get pods -l app.kubernetes.io/name=kubernaut-apifrontend -n kubernaut
   ```

2. Check pod logs for crash loop or OOM:
   ```bash
   kubectl logs -l app.kubernetes.io/name=kubernaut-apifrontend --tail=100 -n kubernaut
   ```

3. Check node resource pressure:
   ```bash
   kubectl top nodes
   kubectl describe pod <pod-name> -n kubernaut | grep -A5 "Conditions"
   ```

4. Verify the ServiceMonitor is scraping correctly:
   ```bash
   kubectl get servicemonitors -n kubernaut
   ```

## Resolution

1. If CrashLoopBackOff due to config error → fix ConfigMap and restart:
   ```bash
   kubectl rollout restart deployment/kubernaut-apifrontend -n kubernaut
   ```

2. If OOMKilled → increase memory limits in Helm values and redeploy

3. If node pressure → cordon node and allow rescheduling

4. If readyz failing but pod running → check downstream dependencies (KA, DS, K8s API) via circuit breaker state metrics

## Prevention

- Ensure resource requests/limits are set appropriately for expected load
- Configure PodDisruptionBudget with `minAvailable: 1`
- Set up multi-AZ pod anti-affinity for HA deployments

## Escalation (FedRAMP IR-4)

If resolution exceeds 15 minutes:
1. Page on-call SRE via PagerDuty
2. Open incident in the incident management system
3. Preserve pod logs and events as evidence (`kubectl get events --sort-by=.metadata.creationTimestamp`)
