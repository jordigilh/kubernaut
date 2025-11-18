# LLM RCA Validation Test Scenarios - Production-Focused

**Purpose**: Validate that the LLM investigates Kubernetes events to identify the actual root cause signal type rather than just treating the initial symptom.

**Test Objective**: Demonstrate that the LLM:
1. Reads Kubernetes events from `kubectl describe pod` / `kubectl get events`
2. Identifies the **actual K8s event reason** (e.g., `Evicted`, `FailedScheduling`)
3. Uses the **RCA signal type** for workflow search, not the initial symptom
4. Selects remediation workflow that addresses the root cause

**Critical Design Principles**:
- ✅ **Use ONLY real Kubernetes event reasons** (OOMKilled, Evicted, FailedScheduling, FailedMount, etc.)
- ✅ **Focus on common production scenarios** (high frequency, high impact)
- ✅ **Test safe remediations only** (no security policy deletions, no data loss)
- ✅ **Chaos engineering approach** (inject fault → observe → validate LLM RCA)

**Environment**: Local KIND cluster with limited resources (2-4GB RAM, 2 CPUs)

**Test Duration**: ~90 minutes (setup + 2 scenarios + validation)

---

## Scenario 1: Node Disk Pressure - `Evicted` → `Evicted` (DiskPressure) 💾

### Production Context

**Frequency**: Monthly in production
**Confidence**: 90%

**Real-World Scenario**:
- Node disk fills up with logs, container images, or ephemeral storage
- Kubelet detects disk pressure and starts evicting pods
- Pods show `Evicted` status
- Root cause is **node-level disk exhaustion**, not individual pod issue
- Logs accumulate over time (application logs, system logs, container layers)

**Why This Tests RCA**:
- ❌ **Wrong diagnosis**: "This pod is misbehaving" → Delete/restart pod (doesn't help)
- ✅ **Correct diagnosis**: "Node has DiskPressure" → Clean up disk space, address root cause

### Chaos Engineering Setup

**Fault Injection**: Fill node disk to trigger DiskPressure condition

```bash
# 1. Create test namespace
kubectl create namespace test-rca-disk-pressure

# 2. Check current node disk usage
docker exec kind-control-plane df -h /

# Typical KIND node: 50-100GB disk, we'll fill it to trigger DiskPressure
# Kubelet typically triggers DiskPressure at 85-90% usage

# 3. Fill node disk (chaos injection)
# Create a large file to consume disk space
docker exec kind-control-plane bash -c "dd if=/dev/zero of=/tmp/disk-filler bs=1M count=10000"
# This creates a 10GB file - adjust count based on available disk

# Alternative: Fill with many smaller files to simulate log accumulation
docker exec kind-control-plane bash -c "for i in {1..1000}; do dd if=/dev/zero of=/tmp/log-file-\$i bs=1M count=10; done"

# 4. Check node condition (should show DiskPressure)
kubectl describe node kind-control-plane | grep -A 10 "Conditions:"
# Look for: DiskPressure   True

# 5. Deploy application pod (will be evicted due to disk pressure)
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-on-pressured-node
  namespace: test-rca-disk-pressure
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: app
        image: nginx:alpine
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "200m"
EOF

# 6. Wait for kubelet to detect disk pressure and evict pod
sleep 60

# 7. Check for eviction due to disk pressure
kubectl get pods -n test-rca-disk-pressure
kubectl describe node kind-control-plane | grep -A 5 "DiskPressure"
kubectl get events -n test-rca-disk-pressure --field-selector reason=Evicted
kubectl describe pod -l app=test-app -n test-rca-disk-pressure | grep -A 15 "Events:"
```

### Expected Kubernetes Events

**On the evicted pod**, you'll see events like:

```
Type     Reason    Message
----     ------    -------
Warning  Evicted   The node had condition: [DiskPressure].
Normal   Killing   Stopping container app
Warning  BackOff   Back-off restarting failed container app (if trying to reschedule)
```

**Node condition**:
```bash
kubectl describe node kind-control-plane
# Should show: DiskPressure: True
# Message: kubelet has disk pressure
```

**Key Differentiator**: Event message explicitly states "The node had condition: [DiskPressure]" (not memory-related)

### LLM Investigation Flow

#### Initial Signal Sent to API
```json
{
  "signal_type": "Evicted",
  "severity": "high",
  "resource_namespace": "test-rca-disk-pressure",
  "resource_name": "app-on-pressured-node-xxx",
  "resource_kind": "pod"
}
```

#### Expected LLM Investigation Steps

**Phase 1: Investigation**
1. Check pod status: `kubectl get pod app-on-pressured-node-xxx`
   - Status: `Failed` or `Evicted`
   - Reason: `Evicted`
2. **CRITICAL**: Check pod events: `kubectl describe pod app-on-pressured-node-xxx`
   - Event Reason: **`Evicted`**
   - Event Message: **"The node had condition: [DiskPressure]"**
3. Check node conditions: `kubectl describe node kind-control-plane`
   - Condition: **`DiskPressure: True`**
   - Message: "kubelet has disk pressure"
4. Check node disk usage: `kubectl top node` or node metrics
   - Sees high disk utilization (>85%)
5. **Key insight**: Node-level disk issue, not pod-specific problem

**Phase 2: Root Cause Analysis**
- Root Cause: Node disk exhaustion causing kubelet to evict pods
- **RCA Signal Type**: `Evicted` (from K8s event reason)
- **RCA Context**: DiskPressure node condition (from event message)
- Severity: `high` or `critical` (affects node stability, multiple workloads at risk)
- Contributing Factors:
  - Node has DiskPressure condition
  - Disk usage exceeded kubelet threshold (typically 85-90%)
  - Likely causes: log accumulation, container images, ephemeral storage

**Phase 3: Signal Type Identification**
- **Output**: `Evicted` (signal type matches)
- **Context**: DiskPressure (distinguishes from MemoryPressure or other eviction causes)
- Justification: Pod was evicted by kubelet due to node-level disk exhaustion

**Phase 4: Workflow Search**
- Query: `"Evicted high node disk pressure"`
- Label Filters: Include node context + disk-related filters
- Expected Workflows:
  1. `cleanup-node-disk-space` (remove logs, unused images, temp files)
  2. `expand-node-disk-capacity` (if cloud provider supports)
  3. `cordon-node-disk-pressure` (prevent new scheduling until fixed)

**Phase 5: Workflow Selection**
- Expected: `cleanup-node-disk-space`
- Parameters:
  - `node_name=kind-control-plane`
  - `namespace=test-rca-disk-pressure`
  - `cleanup_targets=["logs", "unused-images", "tmp-files"]`
- **NOT**: `delete-pod` or `restart-pod` (doesn't address node-level disk issue)

### Validation Criteria

✅ **PRIMARY SUCCESS**: LLM identifies `Evicted` as RCA signal type
✅ **SECONDARY SUCCESS**: LLM mentions **DiskPressure** node condition in RCA summary
✅ **CONTEXT SUCCESS**: LLM distinguishes DiskPressure from MemoryPressure/other eviction causes
✅ **WORKFLOW SUCCESS**: Workflow search query includes "Evicted" + "disk" context
✅ **REMEDIATION SUCCESS**: Selected workflow addresses disk cleanup (not pod deletion/restart)
✅ **NODE-LEVEL SUCCESS**: LLM recognizes this as node issue, not pod-specific problem
❌ **FAILURE**: LLM selects pod-level workflow (delete/restart) without addressing disk pressure

### Test API Request

```bash
# Get the evicted pod name
POD_NAME=$(kubectl get pods -n test-rca-disk-pressure -l app=test-app -o jsonpath='{.items[0].metadata.name}')

# Send incident to HolmesGPT API
curl -X POST http://localhost:8080/api/v1/incident/analyze \
  -H "Content-Type: application/json" \
  -d "{
    \"incident_id\": \"incident-disk-pressure-001\",
    \"signal_type\": \"Evicted\",
    \"severity\": \"high\",
    \"resource_namespace\": \"test-rca-disk-pressure\",
    \"resource_name\": \"$POD_NAME\",
    \"resource_kind\": \"pod\",
    \"alert_name\": \"PodEvicted\",
    \"error_message\": \"Pod was evicted from node\",
    \"description\": \"Application pod evicted due to node conditions\",
    \"business_context\": {
      \"environment\": \"production\",
      \"priority\": \"P1\",
      \"risk_tolerance\": \"medium\",
      \"business_category\": \"general\"
    }
  }" | jq '.'
```

### Expected LLM Response (Key Fields)

```json
{
  "root_cause_analysis": {
    "summary": "Node is under disk pressure causing kubelet to evict pods. The pod was evicted due to node-level disk exhaustion (DiskPressure condition), not due to the pod's own behavior or resource usage.",
    "severity": "high",
    "contributing_factors": [
      "Node DiskPressure condition is True",
      "Disk usage exceeded kubelet threshold (85-90%)",
      "Kubelet evicted pod to protect node stability",
      "Likely causes: log accumulation, unused container images, ephemeral storage growth"
    ]
  },
  "selected_workflow": {
    "workflow_id": "cleanup-node-disk-space",
    "version": "1.0.0",
    "confidence": 0.90,
    "rationale": "Investigation confirms the root cause is node-level disk pressure (Evicted event with DiskPressure condition). The 'cleanup-node-disk-space' workflow will remove logs, unused images, and temporary files to free up disk space.",
    "parameters": {
      "node_name": "kind-control-plane",
      "namespace": "test-rca-disk-pressure",
      "cleanup_targets": ["logs", "unused-images", "tmp-files"],
      "environment": "production",
      "priority": "P1",
      "risk_tolerance": "medium",
      "business_category": "general"
    }
  }
}
```

### Cleanup

```bash
# Remove test namespace
kubectl delete namespace test-rca-disk-pressure

# Clean up the disk filler files
docker exec kind-control-plane bash -c "rm -f /tmp/disk-filler /tmp/log-file-*"

# Verify node condition returns to normal
kubectl describe node kind-control-plane | grep -A 5 "DiskPressure"
# Should show: DiskPressure: False
```

---

## Scenario 2: Over-Provisioned Resources - `Pending` → `FailedScheduling` 📊

### Production Context

**Frequency**: Daily/Weekly in production
**Confidence**: 95%

**Real-World Scenario**:
- Developer sets `memory: 64Gi` request "just to be safe"
- Team copies resource specs from large production app to smaller staging cluster
- New deployment with unrealistic resource requests
- ML/batch job requesting more resources than cluster capacity
- Most common K8s scheduling issue

**Why This Tests RCA**:
- ❌ **Wrong diagnosis**: "Pod is stuck pending, restart it" → Delete/recreate pod (doesn't help)
- ✅ **Correct diagnosis**: "Resource requests exceed capacity" → Adjust resource requests or scale cluster

### Chaos Engineering Setup

**Fault Injection**: Deploy pod with resource requests exceeding cluster capacity

```bash
# 1. Create test namespace
kubectl create namespace test-rca-scheduling

# 2. Check node capacity (for reference)
kubectl describe node kind-control-plane | grep -A 5 "Allocatable"
# Typical KIND node: ~3-4GB memory, 2-4 CPUs allocatable

# 3. Deploy pod requesting MORE resources than cluster has
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: overprovisioned-app
  namespace: test-rca-scheduling
spec:
  replicas: 1
  selector:
    matchLabels:
      app: overprovisioned
  template:
    metadata:
      labels:
        app: overprovisioned
    spec:
      containers:
      - name: app
        image: nginx:alpine
        resources:
          requests:
            memory: "64Gi"  # WAY more than KIND node has (3-4GB)
            cpu: "16"       # WAY more than KIND node has (2-4 CPUs)
          limits:
            memory: "64Gi"
            cpu: "16"
EOF

# 4. Check pod status (should be Pending immediately)
sleep 5
kubectl get pods -n test-rca-scheduling
kubectl describe pod -l app=overprovisioned -n test-rca-scheduling | grep -A 10 "Events:"
```

### Expected Kubernetes Events

```
Type     Reason            Message
----     ------            -------
Warning  FailedScheduling  0/1 nodes are available: 1 Insufficient cpu, 1 Insufficient memory.
Warning  FailedScheduling  0/1 nodes are available: 1 Insufficient cpu, 1 Insufficient memory.
```

**Key Details**:
- Event Reason: `FailedScheduling`
- Clear message showing exactly why (Insufficient cpu, Insufficient memory)
- Deterministic - scheduler will always emit this event

### LLM Investigation Flow

#### Initial Signal Sent to API
```json
{
  "signal_type": "Pending",
  "severity": "high",
  "resource_namespace": "test-rca-scheduling",
  "resource_name": "overprovisioned-app-xxx",
  "resource_kind": "pod"
}
```

#### Expected LLM Investigation Steps

**Phase 1: Investigation**
1. Check pod status: `kubectl get pod overprovisioned-app-xxx`
   - Status: `Pending`
   - Phase: `Pending`
2. **CRITICAL**: Check pod events: `kubectl describe pod overprovisioned-app-xxx`
   - Event Reason: **`FailedScheduling`**
   - Event Message: "0/1 nodes are available: 1 Insufficient cpu, 1 Insufficient memory"
3. Check pod resource requests:
   - Requests: 64Gi memory, 16 CPUs
4. Check node capacity: `kubectl describe node`
   - Allocatable: ~3-4Gi memory, 2-4 CPUs
5. **Key insight**: Resource requests (64Gi, 16 CPU) far exceed node capacity (3Gi, 2 CPU)

**Phase 2: Root Cause Analysis**
- Root Cause: Pod resource requests exceed cluster capacity
- **RCA Signal Type**: `FailedScheduling` (from K8s event reason)
- Severity: `high` (application cannot start)
- Contributing Factors:
  - Memory request (64Gi) exceeds node capacity (3-4Gi)
  - CPU request (16) exceeds node capacity (2-4)
  - No nodes available that can satisfy resource requirements

**Phase 3: Signal Type Identification**
- **Output**: `FailedScheduling` (NOT `Pending`)
- Justification: Scheduler cannot place pod due to insufficient cluster resources

**Phase 4: Workflow Search**
- Query: `"FailedScheduling high insufficient resources"`
- Label Filters: business context
- Expected Workflows:
  1. `reduce-resource-requests` (adjust deployment resource requests)
  2. `scale-cluster-capacity` (add nodes to cluster)

**Phase 5: Workflow Selection**
- Expected: `reduce-resource-requests`
- Parameters:
  - `deployment_name=overprovisioned-app`
  - `namespace=test-rca-scheduling`
  - `requested_memory=64Gi` (current)
  - `requested_cpu=16` (current)
  - `node_capacity_memory=3Gi` (available)
  - `node_capacity_cpu=2` (available)

### Validation Criteria

✅ **PRIMARY SUCCESS**: LLM identifies `FailedScheduling` (not `Pending`) as RCA signal type
✅ **SECONDARY SUCCESS**: LLM mentions resource request exceeds capacity in RCA summary
✅ **WORKFLOW SUCCESS**: Workflow search query includes "FailedScheduling"
✅ **REMEDIATION SUCCESS**: Selected workflow addresses resource requests or cluster scaling
✅ **RATIONALE QUALITY**: LLM compares requested vs available resources with specific numbers
❌ **FAILURE**: LLM uses `Pending` as signal type for workflow search
❌ **FAILURE**: LLM suggests restarting pod or deleting/recreating (doesn't address root cause)

### Test API Request

```bash
# Get the pod name
POD_NAME=$(kubectl get pods -n test-rca-scheduling -l app=overprovisioned -o jsonpath='{.items[0].metadata.name}')

# Send incident to HolmesGPT API
curl -X POST http://localhost:8080/api/v1/incident/analyze \
  -H "Content-Type: application/json" \
  -d "{
    \"incident_id\": \"incident-scheduling-001\",
    \"signal_type\": \"Pending\",
    \"severity\": \"high\",
    \"resource_namespace\": \"test-rca-scheduling\",
    \"resource_name\": \"$POD_NAME\",
    \"resource_kind\": \"pod\",
    \"alert_name\": \"PodStuckPending\",
    \"error_message\": \"Pod stuck in Pending state for >5 minutes\",
    \"description\": \"Application pod cannot be scheduled\",
    \"business_context\": {
      \"environment\": \"production\",
      \"priority\": \"P1\",
      \"risk_tolerance\": \"medium\",
      \"business_category\": \"general\"
    }
  }" | jq '.'
```

### Expected LLM Response (Key Fields)

```json
{
  "root_cause_analysis": {
    "summary": "Pod cannot be scheduled because resource requests (64Gi memory, 16 CPU) far exceed available node capacity (3Gi memory, 2 CPU). No nodes in the cluster can satisfy these requirements.",
    "severity": "high",
    "contributing_factors": [
      "Memory request (64Gi) exceeds node allocatable capacity (3Gi) by 21x",
      "CPU request (16) exceeds node allocatable capacity (2) by 8x",
      "Scheduler cannot find any suitable node for placement"
    ]
  },
  "selected_workflow": {
    "workflow_id": "reduce-resource-requests",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Investigation confirms the root cause is FailedScheduling due to resource requests exceeding cluster capacity. The 'reduce-resource-requests' workflow will analyze actual resource usage and adjust deployment spec to realistic values based on node capacity.",
    "parameters": {
      "deployment_name": "overprovisioned-app",
      "namespace": "test-rca-scheduling",
      "current_memory_request": "64Gi",
      "current_cpu_request": "16",
      "node_capacity_memory": "3Gi",
      "node_capacity_cpu": "2",
      "environment": "production",
      "priority": "P1",
      "risk_tolerance": "medium",
      "business_category": "general"
    }
  }
}
```

### Cleanup

```bash
kubectl delete namespace test-rca-scheduling
```

---

## Comparison Matrix

| Scenario | Initial Signal | RCA Signal (K8s Event) | Prod Frequency | Confidence | Setup Time | Investigation Time |
|----------|---------------|----------------------|----------------|------------|------------|-------------------|
| **1: Node Disk Pressure** | Evicted | **Evicted (DiskPressure)** | Monthly | 90% | 1 min | 60s |
| **2: Over-Provisioned** | Pending | **FailedScheduling** | Daily/Weekly | 95% | 30 sec | 30s |

---

## Success Metrics

### Primary Metric: RCA Signal Type Accuracy
- **Target**: LLM identifies correct K8s event reason in BOTH scenarios (100%)
- **Measurement**: Check if `selected_workflow` search uses RCA signal type (Evicted, FailedScheduling) instead of initial symptom

### Secondary Metric: Event Investigation
- **Target**: LLM explicitly mentions checking pod events in investigation phase
- **Measurement**: `root_cause_analysis.summary` references K8s events

### Tertiary Metric: Workflow Appropriateness
- **Target**: Selected workflow matches RCA signal type, not symptom
- **Measurement**:
  - Scenario 1: Workflow ID contains "cleanup", "disk", "node" (NOT "delete-pod", "restart")
  - Scenario 2: Workflow ID contains "reduce", "scale", "resource" (NOT "restart", "delete")

---

## Required Workflow Catalog Entries

To support these scenarios, populate the workflow catalog with:

### For Scenario 1 (Node Disk Pressure):
```yaml
workflow_id: cleanup-node-disk-space
version: 1.0.0
signal_types: [Evicted]
severity: [high, critical]
description: Clean up disk space on node experiencing DiskPressure condition
parameters:
  - name: node_name
    required: true
  - name: namespace
    required: true
  - name: cleanup_targets
    required: true
    type: array
    default: ["logs", "unused-images", "tmp-files"]
  - name: disk_threshold
    required: false
    default: "85%"
```

### For Scenario 2 (Scheduling):
```yaml
workflow_id: reduce-resource-requests
version: 1.0.0
signal_types: [FailedScheduling]
severity: [high, critical]
description: Adjust deployment resource requests to match cluster capacity
parameters:
  - name: deployment_name
    required: true
  - name: namespace
    required: true
  - name: current_memory_request
    required: true
  - name: current_cpu_request
    required: true
  - name: node_capacity_memory
    required: true
  - name: node_capacity_cpu
    required: true
```

### Contrasting Workflows (Symptom-Focused - Wrong Choice):
```yaml
# LLM should NOT select these
workflow_id: increase-pod-memory-limit
signal_types: [OOMKilled]  # Wrong for node pressure

workflow_id: restart-pending-pod
signal_types: [Pending]  # Wrong for scheduling failure
```

---

## Implementation Checklist

- [ ] Create local KIND cluster with 2-4GB RAM
- [ ] Deploy workflow catalog with RCA and symptom workflows
- [ ] Configure holmesgpt-api with Claude locally (from previous work)
- [ ] Run Scenario 1 (Node Disk Pressure)
  - [ ] Deploy chaos (fill node disk)
  - [ ] Trigger pod eviction due to DiskPressure
  - [ ] Send incident to API
  - [ ] Validate RCA signal type = `Evicted` with DiskPressure context
  - [ ] Validate workflow selection addresses disk cleanup (not pod deletion)
- [ ] Run Scenario 2 (Over-Provisioned Resources)
  - [ ] Deploy overprovisioned pod
  - [ ] Verify FailedScheduling event
  - [ ] Send incident to API
  - [ ] Validate RCA signal type = `FailedScheduling`
  - [ ] Validate workflow selection addresses resource requests
- [ ] Document results with confidence scores
- [ ] Update ADR-041 with validation results

---

## Expected Timeline

- **KIND Cluster Setup**: 10 minutes
- **Workflow Catalog Population**: 10 minutes
- **Scenario 1 Execution**: 20 minutes (disk fill + eviction + test + validation)
- **Scenario 2 Execution**: 15 minutes (setup + test + validation)
- **Documentation**: 25 minutes (results + analysis + screenshots)

**Total**: ~80 minutes for complete validation suite

---

## Key Testing Principles

### ✅ What Makes These Scenarios Valid

1. **Real K8s Event Reasons Only**:
   - `Evicted`, `FailedScheduling` are actual Kubernetes event reasons
   - Deterministic - K8s will always emit these events in these scenarios
   - LLM can find them in `kubectl describe pod` events

2. **Common Production Issues**:
   - Scenario 1: Monthly occurrence (disk pressure from log/image accumulation)
   - Scenario 2: Daily/Weekly occurrence (resource misconfig)
   - Both have significant production impact

3. **Safe Remediations**:
   - Scenario 1: Clean up disk space (safe, no pod deletion ambiguity)
   - Scenario 2: Adjust resource requests (safe, no data loss)
   - No security policy changes, no arbitrary pod deletion

4. **Clear Investigation Path**:
   - Initial symptom (OOMKilled, Pending) → Check events → Find K8s event reason → Use for workflow search

### ❌ What We Avoid

1. **Invented Signal Types**:
   - ❌ No "MemoryLeak", "ConfigMapNotFound", "RegistryCredentialExpired"
   - ✅ Only real K8s event reasons

2. **Unsafe Remediations**:
   - ❌ No NetworkPolicy deletion
   - ❌ No PersistentVolume deletion with data
   - ❌ No RBAC/SecurityContext changes

3. **Unrealistic Scenarios**:
   - ❌ No rare edge cases (storage provisioner failure)
   - ✅ Focus on common, high-impact production issues

---

## LLM Investigation Pattern (Expected Behavior)

For both scenarios, the LLM should follow this pattern:

```
1. Receive initial signal (symptom: OOMKilled, Pending)
2. Check pod status: kubectl get pod <name>
3. **CRITICAL STEP**: Check pod events: kubectl describe pod <name>
4. Extract event Reason field (Evicted, FailedScheduling)
5. Investigate context (node conditions, resource capacity)
6. Determine RCA signal type = K8s event reason
7. Search workflow catalog using RCA signal type
8. Select workflow that addresses root cause
9. Populate parameters from investigation findings
10. Provide rationale explaining symptom vs root cause
```

**Success Indicator**: LLM uses K8s event reason (step 4) for workflow search (step 7), not the initial symptom (step 1).

---

## Next Steps After Validation

1. **Document Results**: Create test report with actual LLM responses
2. **Update ADR-041**: Add validation results to appendix
3. **Refine Prompt** (if needed): If LLM doesn't check events, enhance prompt directive
4. **Expand Catalog**: Add more RCA-focused workflows based on learnings
5. **Production Deployment**: Deploy validated system to staging cluster
