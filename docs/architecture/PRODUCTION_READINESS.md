# Production Readiness - Backup/Restore & Multi-Region

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ **COMPREHENSIVE**
**Scope**: All Services + Infrastructure

---

## 📋 **Table of Contents**

1. [Backup & Restore](#backup--restore)
2. [Multi-Region Deployment](#multi-region-deployment)
3. [Disaster Recovery](#disaster-recovery)
4. [Production Checklist](#production-checklist)

---

## 💾 **1. Backup & Restore**

### **1.1. What Needs Backup**

| Component | Data | Backup Frequency | Retention | Priority |
|-----------|------|------------------|-----------|----------|
| **PostgreSQL** | Audit trail, embeddings | Hourly | 30 days | P0 - CRITICAL |
| **ConfigMaps** | Service config, policies | Daily | 90 days | P1 - HIGH |
| **Secrets** | API keys, credentials | Daily | 90 days | P0 - CRITICAL |
| **CRDs** | Remediation requests | Real-time (via PostgreSQL) | 30 days | P1 - HIGH |

**NOT Backed Up**:
- ❌ Redis (ephemeral cache)
- ❌ Pod logs (shipped to logging system)
- ❌ Metrics (Prometheus retention policy)

---

### **1.2. PostgreSQL Backup**

#### **Automated Backup** (Recommended)

```bash
# Using pgBackRest or Velero
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgresql-backup
  namespace: kubernaut-system
spec:
  schedule: "0 * * * *"  # Hourly
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: postgres:14
            env:
            - name: PGHOST
              value: "postgresql.kubernaut-system"
            - name: PGUSER
              value: "kubernaut"
            - name: PGPASSWORD
              valueFrom:
                secretKeyRef:
                  name: postgresql-credentials
                  key: password
            command:
            - /bin/sh
            - -c
            - |
              BACKUP_FILE="/backups/kubernaut-$(date +%Y%m%d%H%M%S).sql"
              pg_dump -Fc -f "$BACKUP_FILE"
              echo "Backup complete: $BACKUP_FILE"

              # Upload to S3/GCS/Azure Blob
              aws s3 cp "$BACKUP_FILE" s3://kubernaut-backups/postgresql/

              # Cleanup local backups older than 7 days
              find /backups -name "kubernaut-*.sql" -mtime +7 -delete
            volumeMounts:
            - name: backups
              mountPath: /backups
          volumes:
          - name: backups
            persistentVolumeClaim:
              claimName: postgresql-backups
          restartPolicy: OnFailure
```

---

#### **Manual Backup**

```bash
# Full database backup
kubectl exec -n kubernaut-system postgresql-0 -- pg_dump -U kubernaut -Fc > kubernaut-backup-$(date +%Y%m%d).dump

# Specific table backup
kubectl exec -n kubernaut-system postgresql-0 -- pg_dump -U kubernaut -t incident_embeddings -Fc > embeddings-backup.dump

# Schema-only backup
kubectl exec -n kubernaut-system postgresql-0 -- pg_dump -U kubernaut --schema-only > schema-backup.sql
```

---

#### **Restore PostgreSQL**

```bash
# Stop services writing to database
kubectl scale deployment/data-storage --replicas=0 -n kubernaut-system
kubectl scale deployment/context-api --replicas=0 -n kubernaut-system

# Restore from backup
kubectl exec -i -n kubernaut-system postgresql-0 -- pg_restore -U kubernaut -d kubernaut -c < kubernaut-backup-20251006.dump

# Verify restore
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "SELECT count(*) FROM incident_embeddings;"

# Restart services
kubectl scale deployment/data-storage --replicas=2 -n kubernaut-system
kubectl scale deployment/context-api --replicas=2 -n kubernaut-system
```

---

### **1.3. ConfigMap & Secret Backup**

#### **Automated Backup** (Velero)

```bash
# Install Velero
kubectl apply -f https://github.com/vmware-tanzu/velero/releases/download/v1.12.0/velero-v1.12.0-linux-amd64.tar.gz

# Configure backup schedule
velero schedule create kubernaut-daily \
  --schedule="0 2 * * *" \
  --include-namespaces kubernaut-system \
  --include-resources configmaps,secrets

# List backups
velero backup get

# Restore from backup
velero restore create --from-backup kubernaut-daily-20251006020000
```

---

#### **Manual Backup**

```bash
# Backup all ConfigMaps
kubectl get configmaps -n kubernaut-system -o yaml > configmaps-backup-$(date +%Y%m%d).yaml

# Backup all Secrets
kubectl get secrets -n kubernaut-system -o yaml > secrets-backup-$(date +%Y%m%d).yaml

# Backup specific policy ConfigMap
kubectl get configmap rego-policies -n kubernaut-system -o yaml > rego-policies-backup.yaml

# Backup toolset ConfigMap
kubectl get configmap kubernaut-toolset-config -n kubernaut-system -o yaml > toolset-config-backup.yaml
```

---

#### **Restore ConfigMap/Secret**

```bash
# Restore ConfigMaps
kubectl apply -f configmaps-backup-20251006.yaml

# Restore Secrets
kubectl apply -f secrets-backup-20251006.yaml

# Verify restoration
kubectl get configmaps -n kubernaut-system
kubectl get secrets -n kubernaut-system
```

---

### **1.4. CRD Backup**

**Backup Strategy**: CRDs are backed up **indirectly** via PostgreSQL audit trail

**CRDs**:
- `RemediationRequest`
- `AIAnalysis`
- `WorkflowExecution`
- `KubernetesAction`

**Rationale**: CRD data is written to PostgreSQL by Data Storage Service, providing automatic backup

---

### **1.5. Backup Testing**

**Monthly Restore Test**:

```bash
#!/bin/bash
# test-restore.sh - Monthly disaster recovery drill

# 1. Create test namespace
kubectl create namespace kubernaut-test

# 2. Restore PostgreSQL to test instance
kubectl exec -i -n kubernaut-test postgresql-test-0 -- pg_restore -U kubernaut -d kubernaut_test < latest-backup.dump

# 3. Verify data integrity
kubectl exec -n kubernaut-test postgresql-test-0 -- psql -U kubernaut -d kubernaut_test -c "SELECT count(*) FROM incident_embeddings;"

# 4. Cleanup
kubectl delete namespace kubernaut-test

echo "Restore test complete!"
```

---

## 🌍 **2. Multi-Region Deployment**

### **2.1. Multi-Region Architecture**

**Pattern**: **Active-Active** (multiple regions serving traffic)

```
┌──────────────────────────────────────────────────────────────┐
│                      Global Load Balancer                    │
│                   (AWS Route 53 / Cloudflare)                │
└────────────┬──────────────────────────┬─────────────────────┘
             │                          │
             │                          │
    ┌────────▼────────┐        ┌────────▼────────┐
    │  Region: US-EAST │        │  Region: EU-WEST │
    │  (Primary)       │        │  (Secondary)     │
    │                  │        │                  │
    │  Gateway (3)     │        │  Gateway (3)     │
    │  Context API (2) │        │  Context API (2) │
    │  Data Storage (2)│        │  Data Storage (2)│
    │  ...             │        │  ...             │
    │                  │        │                  │
    │  PostgreSQL (HA) │◄──────►│  PostgreSQL (HA) │
    │  Redis (Cluster) │        │  Redis (Cluster) │
    └──────────────────┘        └──────────────────┘
             │                          │
             │                          │
             └──────────┬───────────────┘
                        │
                ┌───────▼────────┐
                │  Shared Object │
                │  Storage (S3)  │
                │  - Backups     │
                │  - Embeddings  │
                └────────────────┘
```

---

### **2.2. Data Replication Strategy**

#### **PostgreSQL Replication** (Streaming Replication)

```yaml
# Primary region: US-EAST
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: postgresql-primary
  namespace: kubernaut-system
spec:
  instances: 3
  primaryUpdateStrategy: unsupervised
  postgresql:
    parameters:
      max_connections: "200"
      shared_buffers: "2GB"
  storage:
    size: 100Gi
  backup:
    barmanObjectStore:
      destinationPath: s3://kubernaut-backups/postgresql-primary
      s3Credentials:
        accessKeyId:
          name: aws-credentials
          key: access-key-id
        secretAccessKey:
          name: aws-credentials
          key: secret-access-key
  # Replication to EU-WEST
  replica:
    enabled: true
    source: postgresql-primary
```

---

```yaml
# Secondary region: EU-WEST (read replica)
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: postgresql-secondary
  namespace: kubernaut-system
spec:
  instances: 3
  replica:
    enabled: true
    source: postgresql-primary.us-east.kubernaut.io
  storage:
    size: 100Gi
```

---

#### **Redis Replication** (Sentinel or Cluster Mode)

```yaml
# Redis Cluster (spans regions)
apiVersion: redis.redis.opstreelabs.in/v1beta1
kind: RedisCluster
metadata:
  name: redis-cluster
  namespace: kubernaut-system
spec:
  clusterSize: 6
  kubernetesConfig:
    image: redis:7.0
  redisExporter:
    enabled: true
  storage:
    volumeClaimTemplate:
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 10Gi
  # Multi-region node affinity
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchLabels:
            app: redis-cluster
        topologyKey: topology.kubernetes.io/zone
```

---

### **2.3. Regional Failover**

**Automatic Failover** (DNS-based):

```yaml
# Route 53 Health Checks
apiVersion: route53.aws.amazon.com/v1alpha1
kind: HealthCheck
metadata:
  name: kubernaut-gateway-us-east
spec:
  type: HTTPS
  resourcePath: /healthz
  fullyQualifiedDomainName: gateway.us-east.kubernaut.io
  port: 443
  requestInterval: 30
  failureThreshold: 3
---
apiVersion: route53.aws.amazon.com/v1alpha1
kind: RecordSet
metadata:
  name: gateway-kubernaut-io
spec:
  name: gateway.kubernaut.io
  type: A
  setIdentifier: us-east
  weight: 100
  healthCheckId: kubernaut-gateway-us-east
  aliasTarget:
    dnsName: gateway.us-east.kubernaut.io
    evaluateTargetHealth: true
---
apiVersion: route53.aws.amazon.com/v1alpha1
kind: RecordSet
metadata:
  name: gateway-kubernaut-io-eu
spec:
  name: gateway.kubernaut.io
  type: A
  setIdentifier: eu-west
  weight: 100
  healthCheckId: kubernaut-gateway-eu-west
  aliasTarget:
    dnsName: gateway.eu-west.kubernaut.io
    evaluateTargetHealth: true
```

---

### **2.4. Multi-Region Considerations**

#### **Latency**
- **Cross-region latency**: 50-150ms (US-EAST ↔ EU-WEST)
- **Database replication lag**: 1-5 seconds
- **Redis replication lag**: < 1 second

#### **Data Consistency**
- **Eventual consistency**: PostgreSQL replication lag
- **Conflict resolution**: Last-write-wins (timestamp-based)
- **Split-brain prevention**: PostgreSQL Patroni + etcd

#### **Cost**
- **Data transfer**: $0.02-0.09/GB between regions
- **Additional infrastructure**: 2x compute, storage
- **Backup storage**: Shared S3/GCS (no duplication)

---

### **2.5. Multi-Region Deployment Steps**

```bash
# Step 1: Deploy primary region (US-EAST)
kubectl apply -f deploy/us-east/ --context=us-east

# Step 2: Wait for primary to be healthy
kubectl wait --for=condition=ready pod -l app.kubernetes.io/part-of=kubernaut -n kubernaut-system --context=us-east --timeout=300s

# Step 3: Deploy secondary region (EU-WEST)
kubectl apply -f deploy/eu-west/ --context=eu-west

# Step 4: Configure PostgreSQL replication
kubectl exec -n kubernaut-system postgresql-0 --context=us-east -- psql -U postgres -c "SELECT * FROM pg_stat_replication;"

# Step 5: Configure global load balancer
aws route53 create-health-check --health-check-config file://health-checks.json
aws route53 change-resource-record-sets --hosted-zone-id Z123 --change-batch file://record-sets.json

# Step 6: Verify multi-region setup
curl https://gateway.kubernaut.io/healthz  # Should route to nearest region
```

---

## 🚨 **3. Disaster Recovery**

### **3.1. Recovery Time Objective (RTO)**

| Scenario | RTO | RPO | Priority |
|----------|-----|-----|----------|
| **Single pod failure** | < 1 min | 0 (no data loss) | P0 |
| **Service failure** | < 5 min | 0 (no data loss) | P0 |
| **Database corruption** | < 30 min | < 1 hour | P1 |
| **Region failure** | < 15 min | < 5 min | P0 |
| **Complete outage** | < 2 hours | < 1 hour | P1 |

---

### **3.2. Disaster Recovery Plan**

#### **Scenario 1: Database Corruption**

```bash
# 1. Detect corruption
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "SELECT * FROM pg_database;"

# 2. Stop all services
kubectl scale deployment --all --replicas=0 -n kubernaut-system

# 3. Restore from latest backup
kubectl exec -i -n kubernaut-system postgresql-0 -- pg_restore -U kubernaut -d kubernaut -c < latest-backup.dump

# 4. Verify restore
kubectl exec -n kubernaut-system postgresql-0 -- psql -U kubernaut -c "SELECT count(*) FROM incident_embeddings;"

# 5. Restart services
kubectl scale deployment --all --replicas=2 -n kubernaut-system

# 6. Verify system health
kubectl get pods -n kubernaut-system
```

**RTO**: 30 minutes
**RPO**: 1 hour (hourly backups)

---

#### **Scenario 2: Region Failure**

```bash
# 1. Detect region failure
# Route 53 health checks automatically failover to secondary region

# 2. Verify traffic routing to EU-WEST
dig gateway.kubernaut.io +short
# Should return EU-WEST IP

# 3. Promote secondary PostgreSQL to primary
kubectl exec -n kubernaut-system postgresql-0 --context=eu-west -- pg_ctl promote

# 4. Verify all services healthy in EU-WEST
kubectl get pods -n kubernaut-system --context=eu-west

# 5. Monitor for US-EAST recovery
# When US-EAST recovers, reconfigure as replica

# 6. Restore active-active configuration
# Update Route 53 weights to split traffic
```

**RTO**: 15 minutes (automatic failover)
**RPO**: 5 minutes (replication lag)

---

## ✅ **4. Production Checklist**

### **Pre-Production**

- ✅ All services deployed and healthy
- ✅ Automated backups configured (PostgreSQL, ConfigMaps, Secrets)
- ✅ Backup restoration tested successfully
- ✅ Multi-region replication configured (if applicable)
- ✅ Global load balancer configured with health checks
- ✅ Disaster recovery plan documented and tested
- ✅ Monitoring and alerting configured (Prometheus, Grafana)
- ✅ On-call rotation and escalation procedures defined
- ✅ Runbooks created for common incidents
- ✅ Security audit completed (RBAC, secrets, network policies)

---

### **Post-Production**

- ✅ Monthly backup restoration drills
- ✅ Quarterly disaster recovery exercises
- ✅ Annual multi-region failover tests
- ✅ Continuous monitoring of backup success/failure
- ✅ Regular review of RTO/RPO targets
- ✅ Capacity planning and scaling reviews

---

## 📚 **Related Documentation**

- [TROUBLESHOOTING_GUIDE.md](./TROUBLESHOOTING_GUIDE.md) - Incident response
- [OPERATIONAL_STANDARDS.md](./OPERATIONAL_STANDARDS.md) - Operational best practices
- [DEPLOYMENT_YAML_TEMPLATE.md](./DEPLOYMENT_YAML_TEMPLATE.md) - Deployment configuration

---

**Document Status**: ✅ Complete
**Last Updated**: October 6, 2025
**Version**: 1.0
