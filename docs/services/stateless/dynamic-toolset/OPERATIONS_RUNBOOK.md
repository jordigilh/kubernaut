# Dynamic Toolset Service - Operations Runbook

**Service**: Dynamic Toolset Service
**Version**: 1.0
**Last Updated**: November 10, 2025
**On-Call Contact**: SRE Team
**Escalation**: Platform Team

---

## 📋 Quick Reference

### Service Overview
- **Purpose**: Automatically discovers observability services and generates HolmesGPT-compatible toolset configurations
- **Namespace**: `kubernaut-system`
- **Deployment**: `kubernaut-dynamic-toolsets`
- **Port**: 8080 (HTTP), 9090 (Metrics)
- **Discovery Interval**: 5 minutes (configurable)

### Critical Endpoints
- **Health Check**: `http://kubernaut-dynamic-toolsets:8080/health`
- **Readiness**: `http://kubernaut-dynamic-toolsets:8080/ready`
- **Metrics**: `http://kubernaut-dynamic-toolsets:9090/metrics`

### Key ConfigMaps
- **Service Config**: `kubernaut-dynamic-toolset-config` (namespace: `kubernaut-system`)
- **Generated Toolsets**: `kubernaut-toolset-config` (per-namespace where services are discovered)

---

## 🚨 Common Failure Scenarios

### Scenario 1: Service Not Discovering Services

**Symptoms**:
- ConfigMap `kubernaut-toolset-config` is empty or missing
- Metrics show `toolset_services_discovered_total{status="success"}` = 0
- Logs show: "No services discovered" or "Service discovery failed"

**Diagnosis**:
```bash
# 1. Check if service is running
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=dynamic-toolsets

# 2. Check service logs
kubectl logs -n kubernaut-system deployment/kubernaut-dynamic-toolsets --tail=100

# 3. Verify RBAC permissions
kubectl auth can-i list services --as=system:serviceaccount:kubernaut-system:kubernaut-service-discovery

# 4. Check ConfigMap exists
kubectl get configmap -n kubernaut-system kubernaut-dynamic-toolset-config

# 5. Verify target services exist
kubectl get services -n monitoring -l app.kubernetes.io/name=prometheus
```

**Root Causes & Remediation**:

| Cause | Solution | Time |
|-------|----------|------|
| **RBAC permissions missing** | Apply RBAC from `/deploy/dynamic-toolset-deployment.yaml` | 2 min |
| **Target namespaces not configured** | Update ConfigMap `service_discovery.namespaces` | 5 min |
| **Services lack required labels** | Add labels to target services (e.g., `app.kubernetes.io/name=prometheus`) | 10 min |
| **Health checks failing** | Check service endpoints are accessible, adjust health check config | 5 min |
| **Discovery interval too long** | Reduce `discovery_interval` in ConfigMap (default: 5m) | 2 min |

**Quick Fix**:
```bash
# Restart the service to trigger immediate discovery
kubectl rollout restart deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# Wait for rollout
kubectl rollout status deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# Verify discovery
kubectl logs -n kubernaut-system deployment/kubernaut-dynamic-toolsets | grep "discovered"
```

---

### Scenario 2: ConfigMap Not Being Created/Updated

**Symptoms**:
- Services are discovered (logs show "discovered N services")
- ConfigMap `kubernaut-toolset-config` doesn't exist or is stale
- Metrics show `toolset_configmap_updates_total{status="failed"}` increasing

**Diagnosis**:
```bash
# 1. Check ConfigMap creation permissions
kubectl auth can-i create configmaps -n <target-namespace> \
  --as=system:serviceaccount:kubernaut-system:kubernaut-service-discovery

# 2. Check for ConfigMap creation errors
kubectl logs -n kubernaut-system deployment/kubernaut-dynamic-toolsets | grep -i "configmap"

# 3. Verify namespace exists
kubectl get namespace <target-namespace>

# 4. Check for resource quotas
kubectl describe namespace <target-namespace> | grep -A 5 "Resource Quotas"
```

**Root Causes & Remediation**:

| Cause | Solution | Time |
|-------|----------|------|
| **Missing ConfigMap write permissions** | Add ClusterRole rule for `configmaps` with `create,update,patch` | 2 min |
| **Namespace doesn't exist** | Create namespace or remove from discovery config | 1 min |
| **Resource quota exceeded** | Increase quota or remove old ConfigMaps | 5 min |
| **ConfigMap name conflict** | Check for existing ConfigMap with same name, delete if orphaned | 2 min |

**Quick Fix**:
```bash
# Grant ConfigMap permissions (if missing)
kubectl patch clusterrole kubernaut-service-discovery --type='json' -p='[
  {"op": "add", "path": "/rules/-", "value": {
    "apiGroups": [""],
    "resources": ["configmaps"],
    "verbs": ["get", "list", "watch", "create", "update", "patch"]
  }}
]'

# Trigger immediate reconciliation
kubectl annotate deployment kubernaut-dynamic-toolsets -n kubernaut-system \
  kubectl.kubernetes.io/restartedAt="$(date +%Y-%m-%dT%H:%M:%S%z)"
```

---

### Scenario 3: Service Crash Loop / Pod Not Starting

**Symptoms**:
- Pod status: `CrashLoopBackOff` or `Error`
- Readiness probe failing
- Service unavailable

**Diagnosis**:
```bash
# 1. Check pod status
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=dynamic-toolsets

# 2. Check recent events
kubectl get events -n kubernaut-system --sort-by='.lastTimestamp' | grep dynamic-toolsets

# 3. Check logs from crashed container
kubectl logs -n kubernaut-system deployment/kubernaut-dynamic-toolsets --previous

# 4. Describe pod for detailed status
kubectl describe pod -n kubernaut-system -l app.kubernetes.io/component=dynamic-toolsets
```

**Root Causes & Remediation**:

| Cause | Solution | Time |
|-------|----------|------|
| **Invalid ConfigMap configuration** | Validate YAML syntax, fix and update ConfigMap | 5 min |
| **Missing required environment variables** | Check Deployment env vars match expected config | 2 min |
| **Insufficient resources (OOMKilled)** | Increase memory limits in Deployment | 5 min |
| **Image pull failure** | Verify image exists and pull secrets configured | 10 min |
| **Kubernetes API unreachable** | Check network policies, service account token | 10 min |

**Quick Fix**:
```bash
# Check for OOMKilled
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=dynamic-toolsets \
  -o jsonpath='{.items[*].status.containerStatuses[*].lastState.terminated.reason}'

# If OOMKilled, increase memory
kubectl patch deployment kubernaut-dynamic-toolsets -n kubernaut-system --type='json' -p='[
  {"op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/memory", "value": "2Gi"}
]'

# Validate ConfigMap syntax
kubectl get configmap kubernaut-dynamic-toolset-config -n kubernaut-system -o yaml | yq eval '.data."config.yaml"'
```

---

### Scenario 4: High Memory/CPU Usage

**Symptoms**:
- Pod using >80% of memory/CPU limits
- Slow response times
- Metrics show high resource utilization

**Diagnosis**:
```bash
# 1. Check current resource usage
kubectl top pods -n kubernaut-system -l app.kubernetes.io/component=dynamic-toolsets

# 2. Check resource limits
kubectl get deployment kubernaut-dynamic-toolsets -n kubernaut-system \
  -o jsonpath='{.spec.template.spec.containers[0].resources}'

# 3. Check number of discovered services
kubectl get configmap kubernaut-toolset-config -n <namespace> -o yaml | grep -c "endpoint:"

# 4. Check discovery interval
kubectl get configmap kubernaut-dynamic-toolset-config -n kubernaut-system \
  -o jsonpath='{.data.config\.yaml}' | grep discovery_interval
```

**Root Causes & Remediation**:

| Cause | Solution | Time |
|-------|----------|------|
| **Too many services being discovered** | Reduce namespaces in config or add service filters | 5 min |
| **Discovery interval too short** | Increase from 5m to 10m or 15m | 2 min |
| **Memory leak** | Restart deployment, monitor, escalate if persists | 5 min |
| **Insufficient resources allocated** | Increase CPU/memory limits | 5 min |

**Quick Fix**:
```bash
# Increase resources
kubectl patch deployment kubernaut-dynamic-toolsets -n kubernaut-system --type='json' -p='[
  {"op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/memory", "value": "2Gi"},
  {"op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/cpu", "value": "1000m"}
]'

# Increase discovery interval to reduce load
kubectl patch configmap kubernaut-dynamic-toolset-config -n kubernaut-system --type='json' -p='[
  {"op": "replace", "path": "/data/config.yaml", "value": "service_discovery:\n  discovery_interval: \"10m\"\n"}
]'

# Restart to apply changes
kubectl rollout restart deployment/kubernaut-dynamic-toolsets -n kubernaut-system
```

---

### Scenario 5: Stale/Incorrect Toolset Configuration

**Symptoms**:
- ConfigMap contains services that no longer exist
- Recently deployed services not appearing in ConfigMap
- HolmesGPT using outdated toolset information

**Diagnosis**:
```bash
# 1. Check ConfigMap age
kubectl get configmap kubernaut-toolset-config -n <namespace> \
  -o jsonpath='{.metadata.creationTimestamp}'

# 2. Compare with actual services
kubectl get services -n monitoring -l app.kubernetes.io/name=prometheus

# 3. Check last successful discovery
kubectl logs -n kubernaut-system deployment/kubernaut-dynamic-toolsets | grep "discovery completed"

# 4. Check reconciliation metrics
curl http://kubernaut-dynamic-toolsets.kubernaut-system:9090/metrics | grep toolset_configmap_updates
```

**Root Causes & Remediation**:

| Cause | Solution | Time |
|-------|----------|------|
| **Discovery loop not running** | Check logs for errors, restart deployment | 5 min |
| **Service labels changed** | Update service labels or adjust discovery selectors | 10 min |
| **Discovery interval too long** | Reduce interval or trigger manual discovery | 2 min |
| **ConfigMap reconciliation disabled** | Verify reconciliation is enabled in config | 5 min |

**Quick Fix**:
```bash
# Force immediate discovery by restarting
kubectl rollout restart deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# Manually delete stale ConfigMap (will be recreated)
kubectl delete configmap kubernaut-toolset-config -n <namespace>

# Wait for recreation (up to discovery_interval)
kubectl wait --for=condition=available --timeout=300s \
  deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# Verify new ConfigMap
kubectl get configmap kubernaut-toolset-config -n <namespace> -o yaml
```

---

## 🔧 Troubleshooting Decision Tree

```
Service Issue Detected
│
├─ Is pod running?
│  ├─ NO → Check pod status
│  │       ├─ CrashLoopBackOff → Check logs (Scenario 3)
│  │       ├─ ImagePullBackOff → Verify image/credentials
│  │       └─ Pending → Check resources/scheduling
│  │
│  └─ YES → Is service discovering?
│           ├─ NO → Check RBAC/Config (Scenario 1)
│           │
│           └─ YES → Is ConfigMap being created?
│                    ├─ NO → Check permissions (Scenario 2)
│                    │
│                    └─ YES → Is data correct?
│                             ├─ NO → Check reconciliation (Scenario 5)
│                             └─ YES → Check resource usage (Scenario 4)
```

---

## 📊 Health Check Commands

### Quick Health Check (30 seconds)
```bash
#!/bin/bash
# Quick health check script

echo "=== Dynamic Toolset Health Check ==="

# 1. Pod Status
echo -e "\n1. Pod Status:"
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=dynamic-toolsets

# 2. Service Endpoint
echo -e "\n2. Health Endpoint:"
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -s http://kubernaut-dynamic-toolsets.kubernaut-system:8080/health

# 3. Recent Logs
echo -e "\n3. Recent Activity:"
kubectl logs -n kubernaut-system deployment/kubernaut-dynamic-toolsets --tail=10

# 4. Discovery Status
echo -e "\n4. Services Discovered:"
kubectl get configmap -n monitoring kubernaut-toolset-config -o jsonpath='{.data.toolset\.json}' 2>/dev/null | \
  jq -r '.tools | length' || echo "ConfigMap not found"

echo -e "\n=== Health Check Complete ==="
```

### Detailed Diagnostic (5 minutes)
```bash
#!/bin/bash
# Detailed diagnostic script

echo "=== Dynamic Toolset Detailed Diagnostics ==="

# 1. Deployment Status
echo -e "\n1. Deployment Status:"
kubectl describe deployment kubernaut-dynamic-toolsets -n kubernaut-system | grep -A 10 "Conditions:"

# 2. Resource Usage
echo -e "\n2. Resource Usage:"
kubectl top pods -n kubernaut-system -l app.kubernetes.io/component=dynamic-toolsets

# 3. RBAC Verification
echo -e "\n3. RBAC Permissions:"
for resource in services endpoints namespaces configmaps; do
  echo -n "  $resource: "
  kubectl auth can-i list $resource \
    --as=system:serviceaccount:kubernaut-system:kubernaut-service-discovery && echo "✓" || echo "✗"
done

# 4. Configuration
echo -e "\n4. Configuration:"
kubectl get configmap kubernaut-dynamic-toolset-config -n kubernaut-system \
  -o jsonpath='{.data.config\.yaml}' | grep -E "discovery_interval|namespaces"

# 5. Metrics Summary
echo -e "\n5. Metrics Summary:"
kubectl run -it --rm metrics-check --image=curlimages/curl --restart=Never -- \
  curl -s http://kubernaut-dynamic-toolsets.kubernaut-system:9090/metrics | \
  grep -E "toolset_services_discovered|toolset_configmap_updates"

# 6. Recent Events
echo -e "\n6. Recent Events:"
kubectl get events -n kubernaut-system --sort-by='.lastTimestamp' | \
  grep dynamic-toolsets | tail -5

echo -e "\n=== Diagnostics Complete ==="
```

---

## 🔄 Rollback Procedures

### Rollback to Previous Version

**When to Rollback**:
- New version causing crashes
- Significant performance degradation
- Data corruption in ConfigMaps

**Procedure**:
```bash
# 1. Check rollout history
kubectl rollout history deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# 2. Rollback to previous version
kubectl rollout undo deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# 3. Monitor rollback
kubectl rollout status deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# 4. Verify health
kubectl logs -n kubernaut-system deployment/kubernaut-dynamic-toolsets --tail=50

# 5. Check discovery is working
kubectl get configmap kubernaut-toolset-config -n monitoring -o yaml
```

**Rollback to Specific Revision**:
```bash
# List revisions
kubectl rollout history deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# Rollback to specific revision
kubectl rollout undo deployment/kubernaut-dynamic-toolsets -n kubernaut-system --to-revision=<number>
```

### Emergency Stop (Circuit Breaker)

**When to Stop**:
- Service causing cluster instability
- Excessive resource consumption
- Data corruption risk

**Procedure**:
```bash
# 1. Scale to zero (immediate stop)
kubectl scale deployment kubernaut-dynamic-toolsets -n kubernaut-system --replicas=0

# 2. Verify stopped
kubectl get pods -n kubernaut-system -l app.kubernetes.io/component=dynamic-toolsets

# 3. Investigate root cause (check logs, metrics, events)

# 4. When ready to restart
kubectl scale deployment kubernaut-dynamic-toolsets -n kubernaut-system --replicas=2
```

---

## 📈 Performance Tuning

### Optimize for Large Clusters (>100 services)

```yaml
# Update ConfigMap for better performance
service_discovery:
  discovery_interval: "10m"        # Increase from 5m
  cache_ttl: "15m"                 # Increase cache duration
  health_check_interval: "60s"     # Reduce health check frequency
  namespaces:                      # Limit to specific namespaces
    - "monitoring"
    - "observability"
```

### Optimize for Fast Updates (<1 minute)

```yaml
# Update ConfigMap for faster discovery
service_discovery:
  discovery_interval: "30s"        # Decrease from 5m
  cache_ttl: "1m"                  # Shorter cache
  health_check_interval: "15s"     # More frequent health checks
```

### Resource Recommendations

| Cluster Size | CPU Request | CPU Limit | Memory Request | Memory Limit | Replicas |
|--------------|-------------|-----------|----------------|--------------|----------|
| Small (<20 services) | 100m | 250m | 256Mi | 512Mi | 1 |
| Medium (20-50 services) | 250m | 500m | 512Mi | 1Gi | 2 |
| Large (50-100 services) | 500m | 1000m | 1Gi | 2Gi | 2 |
| XLarge (>100 services) | 1000m | 2000m | 2Gi | 4Gi | 3 |

---

## 🔐 Security Incidents

### Scenario: Unauthorized ConfigMap Modification

**Detection**:
```bash
# Check ConfigMap audit logs
kubectl get events -n <namespace> | grep configmap

# Check who modified ConfigMap
kubectl get configmap kubernaut-toolset-config -n <namespace> \
  -o jsonpath='{.metadata.managedFields[*].manager}'
```

**Response**:
1. Identify unauthorized changes
2. Restore from backup or delete (will be recreated)
3. Review RBAC permissions
4. Investigate access logs

### Scenario: Service Account Token Compromised

**Response**:
```bash
# 1. Delete compromised service account
kubectl delete serviceaccount kubernaut-service-discovery -n kubernaut-system

# 2. Recreate from manifest
kubectl apply -f /deploy/dynamic-toolset-deployment.yaml

# 3. Restart deployment to get new token
kubectl rollout restart deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# 4. Audit recent API calls
kubectl get events --all-namespaces | grep kubernaut-service-discovery
```

---

## 📞 Escalation Procedures

### Level 1: On-Call SRE (0-30 minutes)
- **Scope**: Standard operational issues
- **Actions**: Follow runbook procedures
- **Escalate if**: Issue not resolved in 30 minutes

### Level 2: Platform Team (30-60 minutes)
- **Scope**: Complex configuration or RBAC issues
- **Contact**: Platform team on-call
- **Escalate if**: Requires code changes or architecture decisions

### Level 3: Development Team (>60 minutes)
- **Scope**: Bugs, performance issues, or design flaws
- **Contact**: Dynamic Toolset service owners
- **Actions**: Bug fix, hotfix deployment, or architectural changes

### Emergency Contacts
- **SRE On-Call**: [PagerDuty/Slack Channel]
- **Platform Team**: [Slack Channel]
- **Service Owners**: [Email/Slack]

---

## 📝 Maintenance Procedures

### Planned Maintenance Window

**Pre-Maintenance**:
```bash
# 1. Announce maintenance window
# 2. Backup current ConfigMaps
kubectl get configmap -A -l app.kubernetes.io/name=kubernaut -o yaml > backup-configmaps.yaml

# 3. Scale down to single replica
kubectl scale deployment kubernaut-dynamic-toolsets -n kubernaut-system --replicas=1
```

**During Maintenance**:
```bash
# Perform updates (configuration, deployment, etc.)
kubectl apply -f /deploy/dynamic-toolset-deployment.yaml
```

**Post-Maintenance**:
```bash
# 1. Scale back to normal
kubectl scale deployment kubernaut-dynamic-toolsets -n kubernaut-system --replicas=2

# 2. Verify health
kubectl rollout status deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# 3. Check discovery
kubectl logs -n kubernaut-system deployment/kubernaut-dynamic-toolsets | grep "discovery completed"

# 4. Announce completion
```

### Configuration Updates

**Safe Configuration Update Procedure**:
```bash
# 1. Backup current config
kubectl get configmap kubernaut-dynamic-toolset-config -n kubernaut-system -o yaml > config-backup.yaml

# 2. Update ConfigMap
kubectl edit configmap kubernaut-dynamic-toolset-config -n kubernaut-system

# 3. Validate YAML syntax
kubectl get configmap kubernaut-dynamic-toolset-config -n kubernaut-system -o yaml | yq eval

# 4. Restart to apply changes
kubectl rollout restart deployment/kubernaut-dynamic-toolsets -n kubernaut-system

# 5. Monitor for errors
kubectl logs -n kubernaut-system deployment/kubernaut-dynamic-toolsets -f

# 6. If errors, rollback
kubectl apply -f config-backup.yaml
kubectl rollout restart deployment/kubernaut-dynamic-toolsets -n kubernaut-system
```

---

## 📚 Additional Resources

- **Architecture Documentation**: `/docs/services/stateless/dynamic-toolset/README.md`
- **API Documentation**: `/docs/services/stateless/dynamic-toolset/overview.md`
- **Deployment Manifests**: `/deploy/dynamic-toolset-deployment.yaml`
- **Metrics Documentation**: `/docs/services/stateless/dynamic-toolset/metrics-slos.md`
- **E2E Test Results**: `/test/e2e/toolset/`

---

## 📊 Appendix: Key Metrics

### Service Health Metrics
- `toolset_services_discovered_total{status="success"}` - Total services discovered
- `toolset_services_discovered_total{status="failed"}` - Failed discoveries
- `toolset_configmap_updates_total{status="success"}` - Successful ConfigMap updates
- `toolset_configmap_updates_total{status="failed"}` - Failed ConfigMap updates
- `toolset_discovery_duration_seconds` - Time taken for discovery
- `toolset_health_check_duration_seconds` - Health check latency

### Alert Thresholds
- **Critical**: No services discovered for >15 minutes
- **Warning**: ConfigMap update failures >5 in 10 minutes
- **Warning**: Discovery duration >60 seconds
- **Critical**: Pod crash loop for >5 minutes

---

**Document Version**: 1.0
**Last Reviewed**: November 10, 2025
**Next Review**: December 10, 2025

