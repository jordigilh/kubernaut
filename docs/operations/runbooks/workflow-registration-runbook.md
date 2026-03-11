# Workflow Registration - Production Runbooks

**Version**: v1.0
**Last Updated**: 2026-03-08
**Status**: Production Ready
**Related**: [ADR-058](../../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md), [BR-WORKFLOW-006](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md), [DD-WORKFLOW-017](../../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md)

---

## Runbook Index

| ID | Runbook | Triggers On | Automation |
|----|---------|-------------|------------|
| RB-WR-001 | [Apply Rejected: DS Unavailable](#rb-wr-001-apply-rejected-ds-unavailable) | `kubectl apply` returns admission error | Manual |
| RB-WR-002 | [CRD Status Not Populated](#rb-wr-002-crd-status-not-populated) | `.status` fields empty after successful apply | Manual |
| RB-WR-003 | [Orphaned CRD](#rb-wr-003-orphaned-crd-no-ds-entry) | CRD exists but DS catalog has no matching entry | Manual |
| RB-WR-004 | [DS Shows Active After CRD Deleted](#rb-wr-004-ds-shows-active-after-crd-deleted) | Expected behavior documentation | N/A |
| RB-WR-005 | [AuthWebhook Not Ready](#rb-wr-005-authwebhook-not-ready) | Webhook pod not ready or certificate issues | Manual |

---

## Architecture Overview

Workflow registration uses a ValidatingWebhook pattern:

```
kubectl apply -f workflow.yaml
    -> K8s API Server
    -> ValidatingWebhookConfiguration (failurePolicy: Fail)
    -> AuthWebhook RemediationWorkflowHandler
    -> DS POST /api/v1/workflows (internal API)
    -> Response: Allowed/Denied
    -> (async) AuthWebhook patches CRD .status
```

Key components:
- **AuthWebhook** (`kubernaut-auth-webhook`): Intercepts CREATE/DELETE of `RemediationWorkflow` CRDs
- **Data Storage** (`data-storage-service`): Stores workflow in catalog, returns UUID
- **CRD Status**: Updated asynchronously after admission response

---

## RB-WR-001: Apply Rejected: DS Unavailable

### Symptoms

- `kubectl apply -f workflow.yaml` returns an error similar to:

```
Error from server (InternalError): error when creating "workflow.yaml":
Internal error occurred: failed calling webhook "validate.remediationworkflow.kubernaut.ai":
failed to call webhook: Post "https://auth-webhook-service.kubernaut-system.svc:443/validate-remediationworkflow":
dial tcp: lookup auth-webhook-service.kubernaut-system.svc: connection refused
```

- Or an explicit denial message containing "data storage":

```
Error from server: admission webhook "validate.remediationworkflow.kubernaut.ai" denied the request:
workflow registration failed: data storage service unavailable
```

### Diagnosis Steps

```bash
# Step 1: Check DS pod health
kubectl get pods -n kubernaut-system -l app=data-storage
kubectl logs -n kubernaut-system -l app=data-storage --tail=20

# Step 2: Check DS service endpoint
kubectl get endpoints data-storage-service -n kubernaut-system

# Step 3: Check AuthWebhook pod health
kubectl get pods -n kubernaut-system -l app=auth-webhook
kubectl logs -n kubernaut-system -l app=auth-webhook --tail=20

# Step 4: Check AuthWebhook can reach DS
kubectl exec -n kubernaut-system deploy/auth-webhook -- \
  wget -q -O- http://data-storage-service:8080/health 2>&1 || echo "DS unreachable from AW"

# Step 5: Check webhook configuration
kubectl get validatingwebhookconfiguration kubernaut-auth-webhook -o yaml | \
  grep -A5 "remediationworkflow"
```

### Resolution

1. **DS pod not running**: Check DS deployment, PVC mounts, database connectivity
2. **DS service has no endpoints**: DS pods may be in CrashLoopBackOff -- check logs
3. **Network policy blocking**: Verify that AuthWebhook namespace can reach DS service
4. **Wait and retry**: If DS is recovering, simply retry `kubectl apply` once DS is healthy

### Prevention

- Monitor DS health with readiness probes
- Set up alerting on `data-storage-service` endpoint count dropping to 0
- The `failurePolicy: Fail` ensures no orphaned CRDs are created when DS is down

---

## RB-WR-002: CRD Status Not Populated

### Symptoms

- `kubectl apply` succeeded (no error)
- `kubectl get remediationworkflow <name> -o yaml` shows empty `.status` fields
- Workflow IS registered in DS (verify via DS API)

### Diagnosis Steps

```bash
# Step 1: Check CRD exists and inspect status
kubectl get rw -n kubernaut-system
kubectl get rw <name> -n kubernaut-system -o jsonpath='{.status}'

# Step 2: Check AW logs for status patch errors
kubectl logs -n kubernaut-system -l app=auth-webhook --tail=50 | grep -i "status\|patch\|error"

# Step 3: Verify AW has RBAC to patch status subresource
kubectl auth can-i patch remediationworkflows/status \
  --as=system:serviceaccount:kubernaut-system:kubernaut-auth-webhook \
  -n kubernaut-system

# Step 4: Verify DS actually registered the workflow
# (replace <workflow-id> with the metadata.name from the CRD)
kubectl exec -n kubernaut-system deploy/data-storage -- \
  wget -q -O- "http://localhost:8080/api/v1/workflows?name=<workflow-id>" 2>&1
```

### Resolution

1. **RBAC missing**: The AW ServiceAccount needs `patch` on `remediationworkflows/status`. Check ClusterRole:

```bash
kubectl get clusterrole kubernaut-auth-webhook-role -o yaml | grep -A3 remediationworkflow
```

2. **Async goroutine failed**: The status update runs in a goroutine after the admission response. If the AW pod restarts before the goroutine completes, the status is lost. Delete and re-create the CRD to trigger re-registration.

3. **CRD status subresource not enabled**: Verify the CRD has `subresources.status: {}` in its definition:

```bash
kubectl get crd remediationworkflows.kubernaut.ai -o jsonpath='{.spec.versions[0].subresources}'
```

### Prevention

- Status population is best-effort (async). The workflow IS registered in DS even if `.status` is empty.
- Monitor AW pod restarts

---

## RB-WR-003: Orphaned CRD (No DS Entry)

### Symptoms

- CRD exists: `kubectl get rw <name> -n kubernaut-system` returns a result
- DS has no matching workflow entry

This should NOT happen under normal operation because `failurePolicy: Fail` rejects the CRD creation if DS registration fails. Possible causes:
- DS database was restored from a backup that predates the CRD creation
- Manual database manipulation

### Diagnosis Steps

```bash
# Step 1: List CRDs
kubectl get rw -n kubernaut-system -o wide

# Step 2: List DS catalog
kubectl exec -n kubernaut-system deploy/data-storage -- \
  wget -q -O- http://localhost:8080/api/v1/workflows 2>&1 | python3 -m json.tool

# Step 3: Compare -- find CRDs not in DS
# (manual comparison of metadata.name vs DS workflow_name)
```

### Resolution

Delete and re-create the orphaned CRD:

```bash
kubectl delete rw <name> -n kubernaut-system
kubectl apply -f <workflow-manifest>.yaml -n kubernaut-system
```

The DELETE will attempt to disable a non-existent DS entry (no-op or logged warning). The subsequent CREATE will register it fresh.

---

## RB-WR-004: DS Shows Active After CRD Deleted

### Symptoms

- `kubectl delete rw <name>` succeeded
- DS catalog still shows the workflow as `active`

**This is NOT a bug.** This is expected behavior per ADR-058:

- CRD DELETE triggers the AW to call `PATCH /api/v1/workflows/{id}/disable` on DS
- DS sets `catalog_status = 'disabled'`, NOT deletes the row
- If the disable call failed (e.g., AW pod restarted during DELETE), the DS entry remains `active`

### Resolution

If the workflow should be disabled in DS but the AW disable call was missed:

```bash
# Option 1: Re-create and delete the CRD
kubectl apply -f <workflow-manifest>.yaml -n kubernaut-system
kubectl delete rw <name> -n kubernaut-system

# Option 2: Directly disable via DS API (admin operation)
kubectl exec -n kubernaut-system deploy/data-storage -- \
  wget -q -O- --method=PATCH \
  "http://localhost:8080/api/v1/workflows/<uuid>/disable" 2>&1
```

### Note on `deprecated` and `archived` States

The `deprecated` and `archived` catalog states are admin-only operations managed via the DS REST API. They are NOT exposed through the CRD lifecycle. The CRD `.status.catalogStatus` may be stale for these rare admin operations. This is a documented and accepted trade-off (ADR-058).

---

## RB-WR-005: AuthWebhook Not Ready

### Symptoms

- All `kubectl apply -f workflow.yaml` commands fail
- Other CRD operations (NotificationRule, etc.) may also fail
- AW pod is not running or not ready

### Diagnosis Steps

```bash
# Step 1: Check AW pod status
kubectl get pods -n kubernaut-system -l app=auth-webhook
kubectl describe pod -n kubernaut-system -l app=auth-webhook

# Step 2: Check AW logs
kubectl logs -n kubernaut-system -l app=auth-webhook --tail=50

# Step 3: Check TLS certificate
kubectl get secret kubernaut-auth-webhook-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | \
  base64 -d | openssl x509 -noout -dates 2>/dev/null || echo "Certificate not found or invalid"

# Step 4: Check webhook configuration CA bundle
kubectl get validatingwebhookconfiguration kubernaut-auth-webhook \
  -o jsonpath='{.webhooks[0].clientConfig.caBundle}' | base64 -d | \
  openssl x509 -noout -dates 2>/dev/null || echo "CA bundle invalid"

# Step 5: Check if webhook is registered
kubectl get validatingwebhookconfiguration kubernaut-auth-webhook -o yaml
```

### Resolution

1. **Pod not running**: Check deployment, image pull errors, resource limits
2. **Certificate expired**: Re-run the TLS cert generation hook:

```bash
helm upgrade kubernaut charts/kubernaut -n kubernaut-system
```

3. **CA bundle mismatch**: The TLS cert hook patches the webhook configuration with the CA bundle. If the CA bundle doesn't match the serving certificate, the API server cannot verify the webhook.

4. **Pod CrashLoopBackOff**: Check logs for startup errors (missing config, DS URL not set, etc.)

### Prevention

- Monitor AW pod restarts and readiness
- Set up alerting on webhook failure rates
- Certificate rotation should be automated via the Helm hook
