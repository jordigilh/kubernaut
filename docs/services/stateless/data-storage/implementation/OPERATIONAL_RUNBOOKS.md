# Data Storage Service - Operational Runbooks

**Service**: Data Storage Service (API Gateway)
**Purpose**: Production deployment, troubleshooting, and maintenance procedures
**Authority**: Gateway v2.23 Operational Runbooks + Kubernetes Best Practices
**Date**: November 2, 2025

---

## 📖 **TABLE OF CONTENTS**

1. [Runbook 1: Deployment](#runbook-1-deployment)
2. [Runbook 2: Troubleshooting](#runbook-2-troubleshooting)
3. [Runbook 3: Rollback](#runbook-3-rollback)
4. [Runbook 4: Performance Tuning](#runbook-4-performance-tuning)
5. [Runbook 5: Maintenance](#runbook-5-maintenance)
6. [Runbook 6: On-Call Procedures](#runbook-6-on-call-procedures)

---

## **Runbook 1: Deployment** ⏱️ (30 minutes)

### **Purpose**
Deploy Data Storage Service to Kubernetes cluster with zero downtime

### **Prerequisites**
- [ ] Kubernetes cluster access (`kubectl get nodes` works)
- [ ] PostgreSQL 15+ running and accessible
- [ ] Database schema deployed (run `scripts/schema.sql`)
- [ ] Docker image built and pushed to registry
- [ ] ConfigMap and Secret manifests ready

### **Steps**

#### **Step 1.1: Verify Prerequisites** (2 min)
```bash
# Check cluster access
kubectl cluster-info

# Check PostgreSQL
pg_isready -h <db-host> -p 5432

# Check schema
psql -h <db-host> -U postgres -d action_history -c "\dt"

# Verify docker image exists
docker pull quay.io/jordigilh/data-storage:v1.0.0
```

#### **Step 1.2: Create Namespace** (1 min)
```bash
kubectl create namespace datastorage
kubectl label namespace datastorage app=kubernaut
```

#### **Step 1.3: Deploy ConfigMap** (2 min)
```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: datastorage
data:
  DB_HOST: "postgres.datastorage.svc.cluster.local"
  DB_PORT: "5432"
  DB_NAME: "action_history"
  DB_USER: "datastorage_user"
  LOG_LEVEL: "info"
  HTTP_PORT: "8080"
  METRICS_PORT: "9090"
EOF
```

#### **Step 1.4: Deploy Secret** (2 min)
```bash
kubectl create secret generic datastorage-secret \
  --from-literal=DB_PASSWORD='<password>' \
  --namespace=datastorage

# Verify secret
kubectl get secret datastorage-secret -n datastorage -o yaml
```

#### **Step 1.5: Deploy Service** (5 min)
```bash
kubectl apply -f deploy/datastorage-deployment.yaml -n datastorage
kubectl apply -f deploy/datastorage-service.yaml -n datastorage
```

**Deployment YAML** (`deploy/datastorage-deployment.yaml`):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: datastorage
spec:
  replicas: 2
  selector:
    matchLabels:
      app: datastorage
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0  # Zero downtime
  template:
    metadata:
      labels:
        app: datastorage
    spec:
      containers:
      - name: datastorage
        image: quay.io/jordigilh/data-storage:v1.0.0
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        envFrom:
        - configMapRef:
            name: datastorage-config
        env:
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: datastorage-secret
              key: DB_PASSWORD
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          failureThreshold: 1  # Fast endpoint removal for DD-007
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        securityContext:
          runAsNonRoot: true
          runAsUser: 1001
          allowPrivilegeEscalation: false
      terminationGracePeriodSeconds: 40  # DD-007: 30s shutdown + 10s buffer
```

#### **Step 1.6: Verify Deployment** (10 min)
```bash
# Check pods
kubectl get pods -n datastorage
# Expected: 2/2 Running

# Check pod logs
kubectl logs -f deployment/datastorage -n datastorage
# Expected: "Server started on :8080"

# Check events
kubectl get events -n datastorage --sort-by='.lastTimestamp'
# Expected: No errors

# Wait for ready
kubectl wait --for=condition=Ready pod -l app=datastorage -n datastorage --timeout=120s
```

#### **Step 1.7: Test Health Endpoints** (3 min)
```bash
# Port-forward
kubectl port-forward svc/datastorage 8080:8080 -n datastorage &

# Test liveness
curl http://localhost:8080/health/live
# Expected: 200 OK

# Test readiness
curl http://localhost:8080/health/ready
# Expected: 200 OK

# Test metrics
curl http://localhost:8080/metrics
# Expected: Prometheus metrics
```

#### **Step 1.8: Test REST API** (5 min)
```bash
# Test list incidents endpoint
curl -X GET "http://localhost:8080/api/v1/incidents?namespace=default&limit=10"
# Expected: JSON response with incidents array

# Test with filters
curl -X GET "http://localhost:8080/api/v1/incidents?severity=high&limit=5"
# Expected: Filtered results
```

### **Rollback Procedure**
If deployment fails, rollback immediately:
```bash
kubectl rollout undo deployment/datastorage -n datastorage
kubectl rollout status deployment/datastorage -n datastorage
```

### **Success Criteria**
- [ ] All pods running (`2/2 Running`)
- [ ] Health checks passing (200 OK)
- [ ] Logs show no errors
- [ ] /metrics endpoint accessible
- [ ] REST API responds correctly
- [ ] Zero downtime during deployment (DD-007)

---

## **Runbook 2: Troubleshooting** ⏱️ (Variable)

### **Common Issues**

#### **Issue 2.1: Pod Crashes (CrashLoopBackOff)**

**Symptoms**:
```bash
$ kubectl get pods -n datastorage
NAME                           READY   STATUS             RESTARTS   AGE
datastorage-7d9c8f4b5d-abcde  0/1     CrashLoopBackOff   5          3m
```

**Diagnosis**:
```bash
# Check logs
kubectl logs datastorage-7d9c8f4b5d-abcde -n datastorage

# Check previous logs (if crashed)
kubectl logs datastorage-7d9c8f4b5d-abcde -n datastorage --previous

# Check events
kubectl describe pod datastorage-7d9c8f4b5d-abcde -n datastorage
```

**Common Causes**:
1. **Database connection failure**
   - Symptom: `Failed to connect to PostgreSQL`
   - Fix: Verify `DB_HOST`, `DB_PORT`, `DB_PASSWORD` in ConfigMap/Secret
   - Test: `pg_isready -h <DB_HOST> -p <DB_PORT>`

2. **Missing environment variables**
   - Symptom: `Required environment variable not set`
   - Fix: Check ConfigMap and Secret are applied
   - Verify: `kubectl get configmap datastorage-config -n datastorage -o yaml`

3. **Image pull failure**
   - Symptom: `ErrImagePull` or `ImagePullBackOff`
   - Fix: Check image name and registry credentials
   - Verify: `docker pull quay.io/jordigilh/data-storage:v1.0.0`

#### **Issue 2.2: Readiness Probe Failures**

**Symptoms**:
```bash
$ kubectl get pods -n datastorage
NAME                           READY   STATUS    RESTARTS   AGE
datastorage-7d9c8f4b5d-abcde  0/1     Running   0          5m
```

**Diagnosis**:
```bash
# Check readiness endpoint
kubectl exec -it datastorage-7d9c8f4b5d-abcde -n datastorage -- curl localhost:8080/health/ready

# Check logs
kubectl logs datastorage-7d9c8f4b5d-abcde -n datastorage | grep "readiness"
```

**Common Causes**:
1. **Database unhealthy**
   - Symptom: `Database ping failed`
   - Fix: Check PostgreSQL is running and accessible
   - Test: `psql -h <DB_HOST> -U <DB_USER> -d <DB_NAME> -c "SELECT 1"`

2. **Slow startup**
   - Symptom: Pod takes >5s to become ready
   - Fix: Increase `initialDelaySeconds` in readiness probe
   - Update: `kubectl patch deployment datastorage -n datastorage --patch '{"spec":{"template":{"spec":{"containers":[{"name":"datastorage","readinessProbe":{"initialDelaySeconds":10}}]}}}}'`

#### **Issue 2.3: High Response Latency**

**Symptoms**:
- API requests taking >1 second
- Users reporting slow performance

**Diagnosis**:
```bash
# Check metrics
kubectl port-forward svc/datastorage 9090:9090 -n datastorage &
curl http://localhost:9090/metrics | grep http_request_duration

# Check database connections
kubectl exec -it datastorage-7d9c8f4b5d-abcde -n datastorage -- \
  psql -h <DB_HOST> -U <DB_USER> -d <DB_NAME> -c "SELECT count(*) FROM pg_stat_activity WHERE datname='action_history'"

# Check logs for slow queries
kubectl logs datastorage-7d9c8f4b5d-abcde -n datastorage | grep "slow query"
```

**Common Causes**:
1. **Missing database indexes**
   - Fix: Run `scripts/add-indexes.sql`
   - Verify: `psql -h <DB_HOST> -c "\di"`

2. **Database connection pool exhausted**
   - Fix: Increase `DB_MAX_OPEN_CONNS` in ConfigMap
   - Update: See Runbook 4 (Performance Tuning)

3. **Large result sets without pagination**
   - Fix: Enforce `limit` parameter (max 1000)
   - Code: Already enforced in BR-STORAGE-023

#### **Issue 2.4: DD-007 Graceful Shutdown Not Working**

**Symptoms**:
- Request failures during rolling updates
- Errors like "connection refused" during deployments

**Diagnosis**:
```bash
# Check if readiness returns 503 during shutdown
kubectl exec -it datastorage-7d9c8f4b5d-abcde -n datastorage -- \
  bash -c 'kill -TERM 1; sleep 1; curl localhost:8080/health/ready'
# Expected: 503 Service Unavailable

# Check logs during shutdown
kubectl logs datastorage-7d9c8f4b5d-abcde -n datastorage | grep "shutdown"
# Expected: "Shutdown flag set", "Endpoint removal propagation", "Graceful shutdown complete"
```

**Fix**:
- Verify `isShuttingDown` flag implemented in code
- Verify `terminationGracePeriodSeconds: 40` in deployment
- Verify readiness probe checks shutdown flag FIRST

---

## **Runbook 3: Rollback** ⏱️ (5 minutes)

### **Purpose**
Quickly rollback to previous working version if new deployment fails

### **When to Rollback**
- [ ] Deployment stuck (pods not becoming ready after 5 minutes)
- [ ] Critical bug discovered in production
- [ ] Performance degradation (p99 latency >2x normal)
- [ ] Data corruption detected
- [ ] Any P0 incident

### **Rollback Steps**

#### **Step 3.1: Immediate Rollback** (1 min)
```bash
# Rollback to previous revision
kubectl rollout undo deployment/datastorage -n datastorage

# Monitor rollback progress
kubectl rollout status deployment/datastorage -n datastorage
```

#### **Step 3.2: Rollback to Specific Revision** (2 min)
```bash
# List deployment history
kubectl rollout history deployment/datastorage -n datastorage

# Rollback to specific revision
kubectl rollout undo deployment/datastorage -n datastorage --to-revision=3
```

#### **Step 3.3: Verify Rollback** (2 min)
```bash
# Check pods
kubectl get pods -n datastorage
# Expected: 2/2 Running (old version)

# Check version
kubectl describe deployment datastorage -n datastorage | grep Image
# Expected: Previous image version

# Test health
curl http://localhost:8080/health/ready
# Expected: 200 OK

# Test API
curl http://localhost:8080/api/v1/incidents?limit=1
# Expected: JSON response
```

### **Post-Rollback Actions**
1. **Notify team**: Alert #incidents channel
2. **Create incident**: Document what went wrong
3. **Block deployment**: Prevent re-deploy of bad version
4. **Root cause analysis**: Investigate failure
5. **Fix forward**: Prepare hotfix if needed

---

## **Runbook 4: Performance Tuning** ⏱️ (Variable)

### **PostgreSQL Optimization**

#### **Tune Connection Pool** (10 min)
```bash
# Update ConfigMap
kubectl edit configmap datastorage-config -n datastorage

# Add/update:
# DB_MAX_OPEN_CONNS: "100"      # Max connections
# DB_MAX_IDLE_CONNS: "25"       # Idle connections
# DB_CONN_MAX_LIFETIME: "1h"    # Connection lifetime

# Restart deployment
kubectl rollout restart deployment/datastorage -n datastorage
```

#### **Add Database Indexes** (30 min)
```sql
-- Run on PostgreSQL
psql -h <DB_HOST> -U postgres -d action_history

-- Indexes for common queries
CREATE INDEX CONCURRENTLY idx_namespace ON resource_action_traces(namespace);
CREATE INDEX CONCURRENTLY idx_severity ON resource_action_traces(severity);
CREATE INDEX CONCURRENTLY idx_timestamp ON resource_action_traces(action_timestamp);
CREATE INDEX CONCURRENTLY idx_composite ON resource_action_traces(namespace, severity, action_timestamp DESC);

-- Verify indexes
\di

-- Monitor index usage
SELECT schemaname, tablename, indexname, idx_scan, idx_tup_read, idx_tup_fetch
FROM pg_stat_user_indexes
WHERE schemaname = 'public';
```

#### **Enable Query Performance Monitoring** (15 min)
```sql
-- Enable pg_stat_statements
ALTER SYSTEM SET shared_preload_libraries = 'pg_stat_statements';
ALTER SYSTEM SET pg_stat_statements.track = 'all';

-- Restart PostgreSQL
-- (deployment-specific)

-- View slow queries
SELECT
    queryid,
    query,
    calls,
    mean_exec_time,
    max_exec_time
FROM pg_stat_statements
WHERE mean_exec_time > 100  -- > 100ms
ORDER BY mean_exec_time DESC
LIMIT 20;
```

### **Application Performance Tuning**

#### **Increase Resource Limits** (5 min)
```bash
# Edit deployment
kubectl edit deployment datastorage -n datastorage

# Update resources:
spec:
  template:
    spec:
      containers:
      - name: datastorage
        resources:
          requests:
            memory: "512Mi"   # Increased from 256Mi
            cpu: "500m"       # Increased from 250m
          limits:
            memory: "1Gi"     # Increased from 512Mi
            cpu: "1000m"      # Increased from 500m
```

#### **Horizontal Pod Autoscaling** (10 min)
```bash
# Create HPA
kubectl autoscale deployment datastorage -n datastorage \
  --cpu-percent=70 \
  --min=2 \
  --max=10

# Verify HPA
kubectl get hpa -n datastorage
```

---

## **Runbook 5: Maintenance** ⏱️ (Variable)

### **Database Maintenance**

#### **Vacuum and Analyze** (30 min)
```sql
-- Run weekly during maintenance window
psql -h <DB_HOST> -U postgres -d action_history

-- Vacuum (reclaim space)
VACUUM ANALYZE resource_action_traces;

-- Check bloat
SELECT
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

#### **Partition Old Data** (2 hours)
```sql
-- Create partition for old data
CREATE TABLE resource_action_traces_2024_11 PARTITION OF resource_action_traces
FOR VALUES FROM ('2024-11-01') TO ('2024-12-01');

-- Verify partitions
SELECT
    parent.relname AS parent,
    child.relname AS child
FROM pg_inherits
JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
JOIN pg_class child ON pg_inherits.inhrelid = child.oid
WHERE parent.relname = 'resource_action_traces';
```

#### **Backup and Recovery** (1 hour)
```bash
# Backup database
pg_dump -h <DB_HOST> -U postgres -d action_history -F c -f action_history_backup_$(date +%Y%m%d).dump

# Verify backup
pg_restore -l action_history_backup_20251102.dump | head -20

# Upload to S3 (or other storage)
aws s3 cp action_history_backup_20251102.dump s3://kubernaut-backups/datastorage/

# Test restore (to test database)
pg_restore -h <TEST_DB_HOST> -U postgres -d action_history_test -c action_history_backup_20251102.dump
```

### **Log Rotation** (15 min)
```bash
# Check log size
kubectl exec -it datastorage-7d9c8f4b5d-abcde -n datastorage -- du -sh /var/log/

# Rotate logs (if needed)
kubectl exec -it datastorage-7d9c8f4b5d-abcde -n datastorage -- \
  logrotate /etc/logrotate.d/datastorage
```

---

## **Runbook 6: On-Call Procedures** ⏱️ (Variable)

### **Incident Response**

#### **P0: Service Down**
**SLA**: 15 minutes

**Actions**:
1. **Check service health** (1 min)
   ```bash
   kubectl get pods -n datastorage
   curl http://<service-url>/health/ready
   ```

2. **Check recent changes** (2 min)
   ```bash
   kubectl rollout history deployment/datastorage -n datastorage
   ```

3. **Rollback if bad deployment** (5 min)
   ```bash
   kubectl rollout undo deployment/datastorage -n datastorage
   ```

4. **Check dependencies** (5 min)
   ```bash
   # PostgreSQL
   pg_isready -h <DB_HOST> -p 5432

   # Network
   kubectl get networkpolicies -n datastorage
   ```

5. **Escalate** (2 min)
   - If not resolved in 15 min, page senior engineer
   - Update incident channel

#### **P1: High Error Rate**
**SLA**: 30 minutes

**Actions**:
1. **Check error metrics** (5 min)
   ```bash
   curl http://<metrics-url>:9090/metrics | grep http_requests_errors_total
   ```

2. **Check logs for patterns** (10 min)
   ```bash
   kubectl logs -f deployment/datastorage -n datastorage | grep ERROR
   ```

3. **Identify root cause** (10 min)
   - Database timeouts?
   - SQL injection attempts?
   - Invalid parameters?

4. **Mitigate** (5 min)
   - Rate limiting
   - Block malicious IPs
   - Increase resources

#### **P2: Performance Degradation**
**SLA**: 1 hour

**Actions**:
1. **Check latency metrics** (10 min)
   ```bash
   curl http://<metrics-url>:9090/metrics | grep http_request_duration
   ```

2. **Check database performance** (20 min)
   ```sql
   -- Slow queries
   SELECT * FROM pg_stat_statements WHERE mean_exec_time > 1000 LIMIT 10;

   -- Active connections
   SELECT count(*) FROM pg_stat_activity WHERE state = 'active';
   ```

3. **Apply tuning** (20 min)
   - Add indexes
   - Increase connection pool
   - Scale pods

4. **Monitor improvement** (10 min)
   - Watch metrics dashboard
   - Confirm p99 latency improved

### **Contact Information**
- **Primary On-Call**: Check PagerDuty schedule
- **Backup On-Call**: Check PagerDuty schedule
- **Database Team**: #database-team
- **Infrastructure Team**: #infrastructure
- **Incident Commander**: VP Engineering

---

**Last Updated**: November 2, 2025
**Maintained By**: Data Storage Service Team
**Review Frequency**: Quarterly


