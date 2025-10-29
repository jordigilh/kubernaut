# {{CONTROLLER_NAME}} Controller - Operations Guide

**Controller**: {{CONTROLLER_NAME}}
**Version**: 1.0.0
**Last Updated**: 2025-10-22

---

## 📋 Table of Contents

1. [Health Checks](#health-checks)
2. [Metrics and Monitoring](#metrics-and-monitoring)
3. [Logging](#logging)
4. [Troubleshooting](#troubleshooting)
5. [Incident Response](#incident-response)
6. [Performance Tuning](#performance-tuning)
7. [Maintenance Procedures](#maintenance-procedures)

---

## 🏥 Health Checks

### Health Endpoints

| Endpoint | Purpose | Expected Response |
|---|---|---|
| `GET /healthz` | Liveness probe | HTTP 200 |
| `GET /readyz` | Readiness probe | HTTP 200 |
| `GET /metrics` | Prometheus metrics | HTTP 200 with metrics |

### Liveness Probe

```bash
# Check liveness (pod is alive)
kubectl exec -it deployment/{{CONTROLLER_NAME}} -n kubernaut-system -- \
  curl -f http://localhost:8081/healthz

# Expected output
ok
```

### Readiness Probe

```bash
# Check readiness (pod is ready to serve traffic)
kubectl exec -it deployment/{{CONTROLLER_NAME}} -n kubernaut-system -- \
  curl -f http://localhost:8081/readyz

# Expected output
ok
```

### Health Check Configuration

```yaml
# Kubernetes deployment health checks
livenessProbe:
  httpGet:
    path: /healthz
    port: 8081
  initialDelaySeconds: 15
  periodSeconds: 20
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /readyz
    port: 8081
  initialDelaySeconds: 5
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

---

## 📊 Metrics and Monitoring

### Prometheus Metrics

#### Controller-Specific Metrics

```prometheus
# Reconciliation metrics
{{CONTROLLER_NAME}}_reconcile_total{result="success|error"}
{{CONTROLLER_NAME}}_reconcile_duration_seconds{result="success|error"}
{{CONTROLLER_NAME}}_reconcile_queue_length

# Resource metrics
{{CONTROLLER_NAME}}_resources_total{namespace="*",status="active|failed|pending"}

# Error metrics
{{CONTROLLER_NAME}}_errors_total{type="*",namespace="*"}

# TODO: Add controller-specific metrics here
# Example for RemediationProcessor:
# {{CONTROLLER_NAME}}_classification_duration_seconds
# {{CONTROLLER_NAME}}_enrichment_cache_hits_total
# {{CONTROLLER_NAME}}_vectordb_queries_total
```

#### Standard Controller Runtime Metrics

```prometheus
# Work queue metrics
workqueue_adds_total{name="{{CONTROLLER_NAME}}"}
workqueue_depth{name="{{CONTROLLER_NAME}}"}
workqueue_queue_duration_seconds{name="{{CONTROLLER_NAME}}"}

# Client metrics
rest_client_requests_total{code="*",method="*"}
rest_client_request_duration_seconds{verb="*"}
```

### Accessing Metrics

```bash
# Port-forward metrics endpoint
kubectl port-forward -n kubernaut-system \
  deployment/{{CONTROLLER_NAME}} 8080:8080

# Fetch metrics
curl http://localhost:8080/metrics

# Filter specific metrics
curl -s http://localhost:8080/metrics | grep {{CONTROLLER_NAME}}
```

### Prometheus Scrape Configuration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: '{{CONTROLLER_NAME}}'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
            - kubernaut-system
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        action: keep
        regex: {{CONTROLLER_NAME}}
      - source_labels: [__meta_kubernetes_pod_container_port_number]
        action: keep
        regex: 8080
```

### Grafana Dashboard

```json
{
  "dashboard": {
    "title": "{{CONTROLLER_NAME}} Controller",
    "panels": [
      {
        "title": "Reconciliation Rate",
        "targets": [
          {
            "expr": "rate({{CONTROLLER_NAME}}_reconcile_total[5m])"
          }
        ]
      },
      {
        "title": "Reconciliation Duration",
        "targets": [
          {
            "expr": "histogram_quantile(0.99, rate({{CONTROLLER_NAME}}_reconcile_duration_seconds_bucket[5m]))"
          }
        ]
      },
      {
        "title": "Error Rate",
        "targets": [
          {
            "expr": "rate({{CONTROLLER_NAME}}_errors_total[5m])"
          }
        ]
      },
      {
        "title": "Queue Depth",
        "targets": [
          {
            "expr": "workqueue_depth{name=\"{{CONTROLLER_NAME}}\"}"
          }
        ]
      }
    ]
  }
}
```

---

## 📝 Logging

### Log Levels

| Level | Purpose | Example |
|---|---|---|
| `debug` | Detailed troubleshooting | Internal state changes, detailed reconciliation steps |
| `info` | Normal operations | Reconciliation started/completed, resource creation |
| `warn` | Non-critical issues | Retries, temporary failures, deprecated config |
| `error` | Critical failures | Reconciliation failures, API errors, invalid config |

### Viewing Logs

```bash
# Tail live logs
kubectl logs -f deployment/{{CONTROLLER_NAME}} -n kubernaut-system

# Get recent logs
kubectl logs --tail=100 deployment/{{CONTROLLER_NAME}} -n kubernaut-system

# Get logs from specific time
kubectl logs --since=1h deployment/{{CONTROLLER_NAME}} -n kubernaut-system

# Get logs from all replicas
kubectl logs -l app={{CONTROLLER_NAME}} -n kubernaut-system --all-containers

# Get previous container logs (after restart)
kubectl logs deployment/{{CONTROLLER_NAME}} -n kubernaut-system --previous
```

### Log Format

```json
{
  "level": "info",
  "ts": "2025-10-22T14:30:45.123Z",
  "logger": "controllers.{{CRD_KIND}}",
  "msg": "Reconciliation completed",
  "controller": "{{CONTROLLER_NAME}}",
  "controllerGroup": "{{CRD_GROUP}}",
  "controllerKind": "{{CRD_KIND}}",
  "namespace": "default",
  "name": "example-resource",
  "reconcileID": "abc123"
}
```

### Changing Log Level Dynamically

```bash
# Update log level via ConfigMap
kubectl edit configmap {{CONTROLLER_NAME}}-config -n kubernaut-system

# Change log_level: debug
# Save and exit

# Restart controller to apply
kubectl rollout restart deployment/{{CONTROLLER_NAME}} -n kubernaut-system
```

---

## 🔧 Troubleshooting

### Common Issues

#### 1. Controller Not Starting

**Symptoms**:
- Pod in `CrashLoopBackOff` state
- Health checks failing immediately

**Diagnosis**:
```bash
# Check pod status
kubectl get pods -l app={{CONTROLLER_NAME}} -n kubernaut-system

# Check pod events
kubectl describe pod -l app={{CONTROLLER_NAME}} -n kubernaut-system

# Check logs
kubectl logs -l app={{CONTROLLER_NAME}} -n kubernaut-system --tail=50
```

**Common Causes**:
- Invalid configuration file
- Missing RBAC permissions
- Unable to connect to Kubernetes API
- Missing required dependencies (e.g., PostgreSQL, Context API)

**Resolution**:
```bash
# Validate configuration
kubectl get configmap {{CONTROLLER_NAME}}-config -n kubernaut-system -o yaml

# Check RBAC
kubectl auth can-i list {{CRD_KIND}} --as=system:serviceaccount:kubernaut-system:{{CONTROLLER_NAME}}

# Check dependencies
# TODO: Add controller-specific dependency checks
# Example: kubectl get svc postgres-service -n kubernaut-system
```

#### 2. Reconciliation Failing

**Symptoms**:
- Resources stuck in pending state
- Increasing error metrics
- Queue depth growing

**Diagnosis**:
```bash
# Check reconciliation metrics
kubectl port-forward deployment/{{CONTROLLER_NAME}} 8080:8080 -n kubernaut-system
curl -s http://localhost:8080/metrics | grep reconcile

# Check error logs
kubectl logs -l app={{CONTROLLER_NAME}} -n kubernaut-system | grep ERROR

# Check resource status
kubectl get {{CRD_KIND}} -A -o wide
```

**Common Causes**:
- Invalid resource specifications
- External service unavailable
- API rate limiting
- Resource conflicts

**Resolution**:
```bash
# Check specific resource
kubectl describe {{CRD_KIND}} <name> -n <namespace>

# Check controller events
kubectl get events -n <namespace> --field-selector involvedObject.kind={{CRD_KIND}}

# Increase log level for debugging
kubectl set env deployment/{{CONTROLLER_NAME}} LOG_LEVEL=debug -n kubernaut-system
```

#### 3. High Memory/CPU Usage

**Symptoms**:
- Pod being OOMKilled
- CPU throttling
- Slow reconciliation

**Diagnosis**:
```bash
# Check resource usage
kubectl top pod -l app={{CONTROLLER_NAME}} -n kubernaut-system

# Check resource limits
kubectl describe pod -l app={{CONTROLLER_NAME}} -n kubernaut-system | grep -A 5 Limits

# Check metrics
curl -s http://localhost:8080/metrics | grep go_memstats
```

**Common Causes**:
- Memory leaks
- Too many concurrent reconciliations
- Large resource caches
- Inefficient algorithms

**Resolution**:
```bash
# Reduce concurrency
kubectl set env deployment/{{CONTROLLER_NAME}} MAX_CONCURRENCY=5 -n kubernaut-system

# Increase resource limits
kubectl edit deployment {{CONTROLLER_NAME}} -n kubernaut-system
# Update resources.limits.memory and resources.limits.cpu
```

---

## 🚨 Incident Response

### Severity Levels

| Severity | Response Time | Escalation |
|---|---|---|
| **P1 - Critical** | 15 minutes | Immediate escalation to on-call engineer |
| **P2 - High** | 1 hour | Escalate if not resolved in 2 hours |
| **P3 - Medium** | 4 hours | Escalate if not resolved in 8 hours |
| **P4 - Low** | 1 business day | Handle during normal hours |

### P1: Controller Completely Down

**Symptoms**:
- All pods in `CrashLoopBackOff`
- No resources being reconciled
- All health checks failing

**Immediate Actions**:
```bash
# 1. Check if this is a cluster-wide issue
kubectl get nodes
kubectl get pods -A | grep -v Running

# 2. Roll back to previous version
kubectl rollout undo deployment/{{CONTROLLER_NAME}} -n kubernaut-system

# 3. Check for breaking changes in dependencies
kubectl get events -A --sort-by='.lastTimestamp' | tail -20

# 4. If rollback fails, scale to 0 and investigate
kubectl scale deployment/{{CONTROLLER_NAME}} --replicas=0 -n kubernaut-system
```

### P2: Partial Outage

**Symptoms**:
- Some reconciliations failing
- Increasing error rate
- Degraded performance

**Actions**:
```bash
# 1. Identify failing pattern
kubectl logs -l app={{CONTROLLER_NAME}} -n kubernaut-system | grep ERROR | tail -100

# 2. Check external dependencies
# TODO: Add controller-specific dependency checks

# 3. Increase replicas if load-related
kubectl scale deployment/{{CONTROLLER_NAME}} --replicas=3 -n kubernaut-system

# 4. Enable debug logging
kubectl set env deployment/{{CONTROLLER_NAME}} LOG_LEVEL=debug -n kubernaut-system
```

---

## ⚡ Performance Tuning

### Concurrency Tuning

```yaml
# Adjust max concurrent reconciliations
env:
  - name: MAX_CONCURRENCY
    value: "20"  # Default: 10, increase for high throughput
```

### Kubernetes API Tuning

```yaml
# Adjust API client QPS and burst
kubernetes:
  qps: 50.0    # Default: 20.0, increase for high API usage
  burst: 75    # Default: 30, increase with QPS
```

### Cache Tuning

```bash
# Increase cache size for large clusters
# TODO: Add controller-specific cache tuning
```

### Resource Limits

```yaml
resources:
  requests:
    memory: "256Mi"
    cpu: "100m"
  limits:
    memory: "512Mi"
    cpu: "500m"
```

---

## 🔧 Maintenance Procedures

### Upgrading Controller

```bash
# 1. Check release notes for breaking changes
# 2. Update image version in deployment
kubectl set image deployment/{{CONTROLLER_NAME}} \
  {{CONTROLLER_NAME}}=quay.io/jordigilh/{{IMAGE_NAME}}:v0.2.0 \
  -n kubernaut-system

# 3. Monitor rollout
kubectl rollout status deployment/{{CONTROLLER_NAME}} -n kubernaut-system

# 4. Verify health
kubectl get pods -l app={{CONTROLLER_NAME}} -n kubernaut-system
kubectl logs -l app={{CONTROLLER_NAME}} -n kubernaut-system --tail=50
```

### Backup and Restore

```bash
# Backup CRD resources
kubectl get {{CRD_KIND}} -A -o yaml > {{CONTROLLER_NAME}}-backup.yaml

# Backup configuration
kubectl get configmap {{CONTROLLER_NAME}}-config -n kubernaut-system -o yaml > {{CONTROLLER_NAME}}-config-backup.yaml

# Restore from backup
kubectl apply -f {{CONTROLLER_NAME}}-backup.yaml
```

### Certificate Rotation

```bash
# TODO: Add certificate rotation procedures if controller uses TLS
```

---

## 📚 Additional Resources

- [Kubernetes Debugging Guide](https://kubernetes.io/docs/tasks/debug/)
- [Prometheus Best Practices](https://prometheus.io/docs/practices/naming/)
- [Controller Runtime Documentation](https://github.com/kubernetes-sigs/controller-runtime)

---

**Document Status**: ✅ **PRODUCTION-READY**
**Maintained By**: Kubernaut Operations Team
