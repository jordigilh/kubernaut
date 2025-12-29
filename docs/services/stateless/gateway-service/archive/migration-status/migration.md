# Gateway Service - Migration Guide

**Version**: v1.0  
**Applies To**: Upgrading to Design B (Adapter-Specific Endpoints)  
**Breaking Changes**: YES  
**Downtime Required**: NO (rolling update)

---

## Overview

This guide helps you migrate from generic endpoint design to Design B's adapter-specific endpoints.

**Key Change**: `/api/v1/signals` → `/api/v1/signals/prometheus` or `/api/v1/signals/kubernetes-event`

---

## Breaking Changes

### 1. API Endpoints Changed

#### Before (Generic Endpoint)
```http
POST /api/v1/signals
Content-Type: application/json
X-Signal-Source: prometheus

{"alerts": [...]}
```

#### After (Adapter-Specific Endpoint)
```http
POST /api/v1/signals/prometheus
Content-Type: application/json

{"alerts": [...]}
```

**Note**: `X-Signal-Source` header is no longer needed (route defines source)

---

### 2. Configuration Changes

#### Alertmanager Configuration

**Before**:
```yaml
# alertmanager.yaml
receivers:
  - name: kubernaut
    webhook_configs:
      - url: http://gateway-service:8080/api/v1/signals
        http_config:
          headers:
            X-Signal-Source: prometheus
```

**After**:
```yaml
# alertmanager.yaml
receivers:
  - name: kubernaut
    webhook_configs:
      - url: http://gateway-service:8080/api/v1/signals/prometheus
        # No X-Signal-Source header needed
```

---

### 3. Gateway Service Configuration

#### Adapter Configuration Simplified

**Before** (if you had priority/required fields):
```yaml
gateway:
  adapters:
    prometheus:
      enabled: true
      required: true
      priority: 100
```

**After**:
```yaml
gateway:
  adapters:
    prometheus:
      enabled: true  # If enabled, it's required (fail-fast)
      config:
        validate_fingerprint: true
```

---

## Migration Steps

### Step 1: Update Gateway Service Configuration

```bash
# 1. Update config/gateway.yaml
# Remove 'priority' and 'required' fields
# Keep only 'enabled' and 'config'

# 2. Apply updated configuration
kubectl apply -f config/gateway.yaml
```

### Step 2: Deploy Updated Gateway Service

```bash
# Deploy new Gateway version with Design B
kubectl apply -f deploy/gateway-service.yaml

# Verify deployment (rolling update - no downtime)
kubectl rollout status deployment/gateway-service -n kubernaut-system

# Check pods are ready
kubectl get pods -n kubernaut-system -l app=gateway-service
```

### Step 3: Update Alertmanager Configuration

**Option A: Zero-downtime (Recommended)**

```bash
# 1. Gateway now supports BOTH endpoints during transition:
#    - New: /api/v1/signals/prometheus (use this)
#    - Old: Falls back to 404 (update Alertmanager first)

# 2. Update Alertmanager webhook URL
kubectl edit configmap alertmanager-config -n monitoring

# 3. Change webhook URL:
#    url: http://gateway-service:8080/api/v1/signals/prometheus

# 4. Reload Alertmanager
kubectl exec -n monitoring alertmanager-0 -- kill -HUP 1
```

**Option B: Maintenance window**

```bash
# 1. Schedule brief maintenance window
# 2. Update both Gateway and Alertmanager simultaneously
# 3. Verify alerts flowing
```

### Step 4: Update Monitoring/Dashboards

```bash
# Update any references to old endpoints:
# - Grafana dashboards
# - Alert rules
# - Documentation
# - curl examples in runbooks
```

### Step 5: Verify Migration

```bash
# 1. Send test alert to new endpoint
curl -X POST http://gateway-service:8080/api/v1/signals/prometheus \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "alerts": [{
      "labels": {"alertname": "TestAlert", "severity": "info"},
      "annotations": {"description": "Migration test"}
    }]
  }'

# 2. Check Gateway logs
kubectl logs -f deployment/gateway-service -n kubernaut-system

# 3. Verify RemediationRequest CRD created
kubectl get remediationrequests -n kubernaut-system

# 4. Check metrics
kubectl port-forward svc/gateway-service 9090:9090 -n kubernaut-system
curl http://localhost:9090/metrics | grep gateway_alerts_received_total
```

---

## Rollback Plan

If issues occur, rollback is straightforward:

### Option 1: Rollback Gateway Deployment

```bash
# Rollback to previous Gateway version
kubectl rollout undo deployment/gateway-service -n kubernaut-system

# Verify rollback
kubectl rollout status deployment/gateway-service -n kubernaut-system
```

### Option 2: Revert Alertmanager Configuration

```bash
# Restore old webhook URL
kubectl edit configmap alertmanager-config -n monitoring

# Change back to:
#   url: http://gateway-service:8080/api/v1/signals
#   X-Signal-Source: prometheus

# Reload Alertmanager
kubectl exec -n monitoring alertmanager-0 -- kill -HUP 1
```

---

## Troubleshooting

### Issue: 404 Not Found

**Symptom**: Alertmanager getting 404 errors

**Cause**: Using old generic endpoint `/api/v1/signals`

**Solution**:
```bash
# Update Alertmanager to use adapter-specific endpoint
url: http://gateway-service:8080/api/v1/signals/prometheus
```

### Issue: No Alerts Received

**Symptom**: Gateway not receiving alerts

**Check**:
```bash
# 1. Verify Gateway pods running
kubectl get pods -n kubernaut-system -l app=gateway-service

# 2. Check Gateway logs
kubectl logs deployment/gateway-service -n kubernaut-system

# 3. Test endpoint directly
kubectl port-forward svc/gateway-service 8080:8080 -n kubernaut-system
curl -v http://localhost:8080/api/v1/signals/prometheus
```

### Issue: Authentication Errors

**Symptom**: 401 Unauthorized

**Solution**:
```bash
# Verify ServiceAccount token is valid
kubectl get secret -n monitoring alertmanager-token -o yaml

# Test with token
TOKEN=$(kubectl get secret -n monitoring alertmanager-token -o jsonpath='{.data.token}' | base64 -d)
curl -H "Authorization: Bearer $TOKEN" http://gateway-service:8080/api/v1/signals/prometheus
```

---

## Estimated Effort

| Task | Effort | Downtime |
|------|--------|----------|
| Update Gateway config | 5 min | None |
| Deploy Gateway (rolling) | 10 min | None |
| Update Alertmanager | 5 min | None |
| Update dashboards | 15 min | None |
| Verification | 10 min | None |
| **Total** | **45 min** | **None** |

---

## Verification Checklist

- [ ] Gateway deployment successful (all pods running)
- [ ] Alertmanager configuration updated
- [ ] Test alert successfully received
- [ ] RemediationRequest CRD created
- [ ] Gateway metrics showing incoming alerts
- [ ] No errors in Gateway logs
- [ ] Dashboards updated with new endpoint references
- [ ] Documentation updated

---

## Support

If you encounter issues during migration:

1. Check Gateway logs: `kubectl logs -f deployment/gateway-service -n kubernaut-system`
2. Check Alertmanager logs: `kubectl logs -f alertmanager-0 -n monitoring`
3. Review [DESIGN_B_IMPLEMENTATION_SUMMARY.md](./DESIGN_B_IMPLEMENTATION_SUMMARY.md)
4. See [Troubleshooting](#troubleshooting) section above

---

## Post-Migration

After successful migration:

- [ ] Remove old documentation referencing generic endpoints
- [ ] Update team runbooks
- [ ] Update training materials
- [ ] Monitor for 24 hours to ensure stability
- [ ] Remove rollback configurations after 1 week of stable operation

---

**Document Status**: ✅ Complete  
**Migration Type**: Rolling update (zero downtime)  
**Risk Level**: LOW (simple endpoint change)  
**Rollback**: Easy (revert configuration)
