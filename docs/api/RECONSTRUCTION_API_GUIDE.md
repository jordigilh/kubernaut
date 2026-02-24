# RemediationRequest Reconstruction API - User Guide

**Version**: 1.0
**Last Updated**: 2026-01-12
**Business Requirement**: BR-AUDIT-006

## Overview

The RemediationRequest Reconstruction API allows you to reconstruct complete Kubernetes `RemediationRequest` CRDs from audit trail events. This is useful for:

- **Disaster Recovery**: Recreate lost RRs from the audit trail
- **Compliance Audits**: Prove RR state at any point in time
- **Debugging**: Understand RR evolution from audit events

## Endpoint

```
POST /api/v1/audit/remediation-requests/{correlation_id}/reconstruct
```

### Authentication

- **Production/E2E**: Protected by OAuth-proxy sidecar
- **Integration Tests**: Use mock `X-Auth-Request-User` header

## Request

### Path Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `correlation_id` | string | Yes | Unique correlation ID for the remediation lifecycle |

### Example Request

```bash
curl -X POST \
  "http://data-storage-service:8080/api/v1/audit/remediation-requests/rr-prometheus-alert-highcpu-abc123/reconstruct" \
  -H "Authorization: Bearer ${SA_TOKEN}"
```

## Response

### Success Response (200 OK)

```json
{
  "remediation_request_yaml": "apiVersion: remediation.kubernaut.ai/v1alpha1\nkind: RemediationRequest\nmetadata:\n  name: rr-reconstructed-...\n  namespace: kubernaut-system\n  labels:\n    app.kubernetes.io/managed-by: kubernaut-datastorage\n    kubernaut.ai/reconstructed: \"true\"\n    kubernaut.ai/correlation-id: rr-prometheus-alert-highcpu-abc123\n  annotations:\n    kubernaut.ai/reconstructed-at: \"2026-01-12T19:53:00Z\"\n    kubernaut.ai/reconstruction-source: audit-trail\n  finalizers:\n    - kubernaut.ai/audit-retention\nspec:\n  signalName: HighCPU\n  signalType: alert\n  signalLabels:\n    alertname: HighCPU\n    severity: critical\n  signalAnnotations:\n    summary: CPU usage is high\n  originalPayload: '{\"alert\":\"data\"}'\nstatus:\n  timeoutConfig:\n    global: 1h0m0s\n    processing: 10m0s\n    analyzing: 15m0s\n",
  "validation": {
    "is_valid": true,
    "completeness": 83,
    "errors": [],
    "warnings": [
      "SignalAnnotations are missing (Gap #3) - annotations provide additional metadata"
    ]
  },
  "reconstructed_at": "2026-01-12T19:53:00Z",
  "correlation_id": "rr-prometheus-alert-highcpu-abc123"
}
```

### Response Fields

#### `remediation_request_yaml` (string)

Complete `RemediationRequest` CRD in YAML format. Can be applied directly to Kubernetes with `kubectl apply`.

**Key Fields**:
- `apiVersion`: `remediation.kubernaut.ai/v1alpha1`
- `kind`: `RemediationRequest`
- `metadata`: ObjectMeta with labels and annotations
- `spec`: RemediationRequestSpec with signal information
- `status`: RemediationRequestStatus with timeout configuration

#### `validation` (object)

Validation results for the reconstructed RR:

| Field | Type | Description |
|-------|------|-------------|
| `is_valid` | boolean | Whether RR passes validation (no blocking errors) |
| `completeness` | integer | Completeness percentage (0-100) |
| `errors` | array[string] | Blocking validation errors (empty if valid) |
| `warnings` | array[string] | Non-blocking warnings for missing optional fields |

**Completeness Calculation**:
- **100%**: All required and optional fields present (Gaps #1-8 complete)
- **50-99%**: Required fields + some optional fields
- **0-49%**: Only required fields or incomplete (reconstruction rejected)

#### `reconstructed_at` (string)

ISO 8601 timestamp when reconstruction was performed (UTC).

#### `correlation_id` (string)

Correlation ID used for reconstruction (matches request path parameter).

## Applying Reconstructed RR to Kubernetes

### Step 1: Save YAML to File

```bash
curl -X POST \
  "http://data-storage-service:8080/api/v1/audit/remediation-requests/rr-prometheus-alert-highcpu-abc123/reconstruct" \
  -H "Authorization: Bearer ${SA_TOKEN}" \
  | jq -r '.remediation_request_yaml' > reconstructed-rr.yaml
```

### Step 2: Apply to Cluster

```bash
kubectl apply -f reconstructed-rr.yaml
```

### Step 3: Verify

```bash
kubectl get remediationrequest -n kubernaut-system
kubectl describe remediationrequest <name> -n kubernaut-system
```

## Error Responses

### 404 Not Found

**Cause**: No audit events found for the given `correlation_id`.

```json
{
  "type": "https://kubernaut.ai/problems/audit/correlation-not-found",
  "title": "Audit Events Not Found",
  "status": 404,
  "detail": "No audit events found for correlation_id: nonexistent-correlation"
}
```

**Resolution**: Verify the correlation ID is correct and that audit events exist in the database.

### 400 Bad Request (Missing Gateway Event)

**Cause**: Required `gateway.signal.received` event is missing from audit trail.

```json
{
  "type": "https://kubernaut.ai/problems/reconstruction/missing-gateway-event",
  "title": "Reconstruction Failed",
  "status": 400,
  "detail": "gateway.signal.received event is required for reconstruction"
}
```

**Resolution**: Ensure the Gateway service emitted the `gateway.signal.received` event for this correlation ID.

### 400 Bad Request (Incomplete Reconstruction)

**Cause**: Reconstructed RR is less than 50% complete.

```json
{
  "type": "https://kubernaut.ai/problems/reconstruction/incomplete-data",
  "title": "Incomplete Reconstruction",
  "status": 400,
  "detail": "Reconstructed RR is only 33% complete (SignalName and SignalType only)"
}
```

**Resolution**: Check audit trail for missing events (orchestrator, gateway). Partial reconstruction is rejected to prevent applying incomplete CRDs to Kubernetes.

### 500 Internal Server Error

**Cause**: Database query failure, YAML marshaling error, or internal reconstruction logic failure.

```json
{
  "type": "https://kubernaut.ai/problems/reconstruction/query-failed",
  "title": "Reconstruction Query Failed",
  "status": 500,
  "detail": "Failed to query audit events: database connection timeout"
}
```

**Resolution**: Check DataStorage service logs for detailed error messages. Verify database connectivity.

## Reconstruction Workflow

The API performs an 8-step reconstruction workflow:

1. **Query** audit events from database by `correlation_id`
2. **Parse** events to extract structured data
3. **Map** parsed data to RR Spec/Status fields
4. **Build** complete `RemediationRequest` CRD
5. **Validate** reconstructed RR (completeness check)
6. **Convert** RR to YAML format
7. **Build** ReconstructionResponse with validation results
8. **Return** JSON response with YAML + validation

## Gap Coverage

The reconstruction API reconstructs the following fields from audit events:

### Gap #1: Spec Fields (Gateway)
- `spec.signalName`
- `spec.signalType`
- `spec.signalLabels`

### Gap #2: OriginalPayload (Gateway)
- `spec.originalPayload`

### Gap #3: SignalAnnotations (Gateway)
- `spec.signalAnnotations`

### Gap #8: TimeoutConfig (Orchestrator)
- `status.timeoutConfig.global`
- `status.timeoutConfig.processing`
- `status.timeoutConfig.analyzing`

## Best Practices

### 1. Verify Completeness Before Applying

Always check the `validation.completeness` field before applying reconstructed RRs to Kubernetes:

```bash
COMPLETENESS=$(curl -X POST \
  "http://data-storage-service:8080/api/v1/audit/remediation-requests/${CORRELATION_ID}/reconstruct" \
  -H "Authorization: Bearer ${SA_TOKEN}" \
  | jq '.validation.completeness')

if [ "$COMPLETENESS" -lt 80 ]; then
  echo "⚠️  Warning: Reconstruction is only ${COMPLETENESS}% complete"
  echo "Consider investigating missing audit events before applying"
fi
```

### 2. Review Warnings

Check `validation.warnings` for missing optional fields:

```bash
curl -X POST \
  "http://data-storage-service:8080/api/v1/audit/remediation-requests/${CORRELATION_ID}/reconstruct" \
  -H "Authorization: Bearer ${SA_TOKEN}" \
  | jq '.validation.warnings[]'
```

### 3. Use Reconstructed Metadata

Reconstructed RRs include special labels and annotations for tracking:

```yaml
metadata:
  labels:
    kubernaut.ai/reconstructed: "true"
    kubernaut.ai/correlation-id: "rr-prometheus-alert-highcpu-abc123"
  annotations:
    kubernaut.ai/reconstructed-at: "2026-01-12T19:53:00Z"
    kubernaut.ai/reconstruction-source: "audit-trail"
```

Use these to identify and track reconstructed RRs in your cluster.

### 4. Audit Trail Retention

Ensure audit events are retained for the duration of your compliance requirements (typically 7+ years for SOC2/HIPAA).

## Troubleshooting

### Problem: Reconstruction returns < 50% completeness

**Possible Causes**:
1. Gateway didn't emit `gateway.signal.received` event
2. Orchestrator didn't emit `orchestrator.lifecycle.created` event
3. Audit events were pruned/deleted before reconstruction

**Solution**:
```bash
# Check audit events for correlation ID
curl -X GET \
  "http://data-storage-service:8080/api/v1/audit/events?correlation_id=${CORRELATION_ID}" \
  -H "Authorization: Bearer ${SA_TOKEN}"

# Look for missing event types
```

### Problem: YAML validation fails when applying to Kubernetes

**Possible Causes**:
1. Reconstructed RR has invalid field values
2. K8s API version mismatch

**Solution**:
```bash
# Validate YAML locally before applying
kubectl apply --dry-run=client -f reconstructed-rr.yaml

# Check K8s API versions
kubectl api-resources | grep remediationrequest
```

### Problem: 500 Internal Server Error

**Possible Causes**:
1. Database connection timeout
2. Malformed audit event data

**Solution**:
```bash
# Check DataStorage service logs
kubectl logs -n kubernaut-system deployment/data-storage-service -f

# Check database connectivity
kubectl exec -it -n kubernaut-system deployment/data-storage-service -- \
  psql -h postgres -U datastorage -c "SELECT COUNT(*) FROM audit_events;"
```

## Security Considerations

### Authentication

All reconstruction requests MUST be authenticated via OAuth-proxy in production:

1. Client obtains ServiceAccount token
2. OAuth-proxy validates token and performs Subject Access Review (SAR)
3. DataStorage handler receives `X-Auth-Request-User` header

### Authorization

Reconstruction operations require `read` permission on audit events:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: audit-reader
rules:
- apiGroups: ["datastorage.kubernaut.ai"]
  resources: ["audit-events"]
  verbs: ["get", "list"]
```

### Audit Trail

Reconstruction operations are NOT audited (read-only operation). However, applying reconstructed RRs to Kubernetes will generate new audit events.

## Performance

### Expected Response Times

| Audit Events | Response Time |
|--------------|---------------|
| 1-10 events  | < 100ms       |
| 10-50 events | < 500ms       |
| 50+ events   | < 1s          |

### Rate Limiting

No rate limiting currently enforced. Consider implementing if reconstruction is used frequently (> 100 req/s).

## Related Documentation

- [SOC2 Audit RR Reconstruction Test Plan](../development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md)
- [OpenAPI Schema](../../api/openapi/data-storage-v1.yaml)
- [Audit Events API](./AUDIT_EVENTS_API_GUIDE.md)

## Support

For questions or issues with the Reconstruction API:

1. Check DataStorage service logs
2. Verify audit events exist for correlation ID
3. Review RFC 7807 error response details
4. Contact Kubernaut Platform Team

---

**Version History**:
- **1.0** (2026-01-12): Initial release with Gaps #1-3, #8 support
