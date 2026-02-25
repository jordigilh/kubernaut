# 🤝 RO E2E Testing - Team Coordination & Contribution

> **Note (Issue #91):** This document references `kubernaut.ai/*` CRD labels that have since been migrated to immutable spec fields. See [DD-CRD-003](../architecture/DD-CRD-003-field-selectors-operational-queries.md) for the current field-selector-based approach.

**Date**: December 13, 2025
**From**: Remediation Orchestrator (RO) Team
**To**: Gateway, SignalProcessing, AIAnalysis, WorkflowExecution, Notification Teams
**Purpose**: Collaborative E2E test planning for RO segmented E2E tests
**Status**: 🟡 **AWAITING TEAM INPUT**

---

## 📋 **Quick Summary**

RO is implementing **segmented E2E tests** to validate orchestration contracts with each service. We need your help to:

1. ✅ **Confirm E2E readiness** - Is your service ready for E2E testing?
2. ✅ **Provide configuration** - How should we deploy/configure your service?
3. ✅ **Contribute test scenarios** - What edge cases should we test?

**Timeline**: Please respond by **December 20, 2025** for V1.0 inclusion

---

## 🎯 **RO's Segmented E2E Strategy**

### **Why Segmented?**

RO is implementing **5 focused E2E segments** instead of one large full-stack test:

```
Segment 1: Signal → Gateway → RO         (Entry point validation)
Segment 2: RO → SP → RO                  (SignalProcessing orchestration)
Segment 3: RO → AA → HAPI → AA → RO     (AI analysis orchestration)
Segment 4: RO → WE → RO                  (Workflow execution orchestration)
Segment 5: RO → Notification → RO        (Notification orchestration)
```

**Benefits for Your Team**:
- ✅ **Fast feedback** - Each segment runs in < 2 minutes
- ✅ **Easy debugging** - Only 2 services involved per segment
- ✅ **Independent testing** - RO can test contracts without full platform deployment
- ✅ **Clear ownership** - Each segment validates ONE contract

**Full E2E** (all services together) will be a **separate Platform-level test suite** owned by the Platform Team, NOT part of RO's regular CI/CD.

---

## 📊 **Team Input Needed**

### **For Each Service, Please Provide**:

1. **E2E Readiness Status** - Are you ready for E2E testing?
2. **Deployment Configuration** - How should we deploy your service in Kind cluster?
3. **Environment Variables** - What env vars are required?
4. **Dependencies** - What external services do you need?
5. **Health Check** - How can we verify your service is ready?
6. **Test Scenarios** - What edge cases should we test in RO→YourService→RO?

**Template provided below for each team** ⬇️

---

## 🟢 **Gateway Team**

**Segment**: Segment 1 - Signal → Gateway → RO

**Priority**: P2 (V1.2) - Gateway is production-ready, this segment validates entry point

### **1. E2E Readiness Status**

**Status**: 🟡 **In Progress** (95% complete - infrastructure fixes in progress)

**Blockers** (if any):
- [x] E2E infrastructure issues being resolved (9/10 fixes complete)
- [ ] Need clean Podman environment for baseline run
- [ ] DataStorage deployment now working (fixed Dec 13, 2025)

**Estimated Ready Date** (if blocked): **December 16, 2025** (2 days for final infrastructure validation)

---

### **2. Deployment Configuration**

**Deployment Method**: ⬜ Helm / ⬜ Kustomize / ⬜ Raw YAML / ⬜ Other: _______

**Manifest Location**:
```
Path:
```

**Container Image**:
```
Repository:
Tag:
```

**Namespace**:
```
Namespace: kubernaut-system (or specify)
```

---

### **3. Environment Variables**

```yaml
env:
  # Gateway uses CONFIG FILE (not env vars for most settings)
  # Config file path specified via args: --config=/etc/gateway/config.yaml

  # KUBECONFIG (optional - uses in-cluster config by default)
  - name: KUBECONFIG
    value: ""  # Empty = use in-cluster ServiceAccount

# Configuration via ConfigMap (mounted at /etc/gateway/config.yaml):
configMap:
  name: gateway-config
  data:
    config.yaml: |
      server:
        port: 8080
        host: "0.0.0.0"
      processing:
        signal_timeout: 30s
        max_concurrent: 100
      datastorage:
        url: "http://datastorage.kubernaut-system.svc.cluster.local:8080"
      deduplication:
        enabled: true
        ttl: 3600s
      audit:
        enabled: true
      logging:
        level: info
        format: json
```

**Note**: Redis configuration is in the config file, not env vars. See `test/e2e/gateway/gateway-deployment.yaml` for complete ConfigMap.

---

### **4. Dependencies**

**Required Services**:
- [x] **PostgreSQL** (via Data Storage for audit) - Deployed in Gateway E2E infrastructure
- [x] **Redis** (deduplication state - deprecated, moving to CRD Status per DD-GATEWAY-011)
- [x] **Data Storage** (audit API) - Built and deployed via shared `deployDataStorage` function
- [x] **Kubernetes API** (CRD creation) - Available in Kind cluster
- [x] **audit_events table** - Applied via shared migration library

**External Services** (if any):
```
Service: None for E2E - Gateway is entry point
Endpoint: N/A
Mock available: N/A

Note: For RO→Gateway testing, RO will POST signals to Gateway's webhook endpoint
```

**Critical Note**: Gateway's Redis dependency is being **deprecated** (DD-GATEWAY-011). Deduplication state now stored in `RemediationRequest` CRD Status. For E2E tests, Redis can be omitted or mocked.

---

### **5. Health Check**

**Health Endpoint**:
```
URL: http://gateway.kubernaut-system.svc.cluster.local:8080/health
Expected Status: 200
Expected Response: {"status":"ok"}
```

**Readiness Endpoint**:
```
URL: http://gateway.kubernaut-system.svc.cluster.local:8080/ready
Expected Status: 200
Expected Response: {"status":"ready"}
```

**Metrics Endpoint**:
```
URL: http://gateway.kubernaut-system.svc.cluster.local:9090/metrics
Expected Status: 200 (Prometheus format)
```

**Readiness Check**:
```bash
# 1. Wait for Gateway deployment to be available
kubectl wait --for=condition=available \
  deployment/gateway \
  -n kubernaut-system \
  --timeout=120s

# 2. Wait for Gateway pod to be ready
kubectl wait --for=condition=ready \
  pod -l app=gateway \
  -n kubernaut-system \
  --timeout=120s

# 3. Verify health endpoint is accessible
kubectl exec -n kubernaut-system deploy/gateway -- \
  curl -f http://localhost:8080/health
```

---

### **6. Test Scenarios for RO→Gateway Contract**

**Scenario 1**: Valid Prometheus AlertManager signal → RemediationRequest created
```yaml
Description: Gateway receives valid AlertManager webhook, creates RemediationRequest CRD
Expected Gateway Output:
  - RemediationRequest CRD created in target namespace
  - status.phase=Pending
  - spec.signalFingerprint set (SHA256 hash)
  - spec.targetType="kubernetes"
  - spec.firingTime and spec.receivedTime populated
Expected RO Behavior: RO picks up RR in Pending phase, transitions to Processing
Test Data:
  POST http://gateway:8080/webhook/prometheus
  Content-Type: application/json
  {
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "PodCrashLooping",
        "severity": "critical",
        "namespace": "production",
        "pod": "api-server-123",
        "container": "app"
      },
      "annotations": {
        "summary": "Pod is crash looping",
        "description": "Pod api-server-123 has restarted 5 times"
      },
      "startsAt": "2025-12-13T10:00:00Z"
    }]
  }
Expected RemediationRequest:
  metadata:
    name: <generated>
    namespace: production
  spec:
    signalFingerprint: <SHA256 of alertname:namespace:kind:name>
    targetType: kubernetes
    signalType: prometheus_alert
    signalLabels:
      alertname: PodCrashLooping
      severity: critical
      namespace: production
      pod: api-server-123
```

**Scenario 2**: Duplicate signal → Gateway updates deduplication status
```yaml
Description: Gateway receives identical signal within TTL, updates status.deduplication.occurrenceCount
Expected Gateway Output:
  - NO new RemediationRequest created
  - Existing RR's status.deduplication.occurrenceCount incremented
  - status.deduplication.lastOccurrence updated
Expected RO Behavior: RO detects duplicate via deduplication status, may skip or aggregate
Test Data: Same payload as Scenario 1, sent twice within 60 seconds
Expected Deduplication Status:
  status:
    deduplication:
      occurrenceCount: 2
      firstOccurrence: "2025-12-13T10:00:00Z"
      lastOccurrence: "2025-12-13T10:00:30Z"
Note: Fingerprint calculation: SHA256("PodCrashLooping:production:Pod:api-server-123")
```

**Scenario 3**: Kubernetes Event signal → RemediationRequest created with different adapter
```yaml
Description: Gateway receives K8s Event (not Prometheus), creates RR with correct signal type
Expected Gateway Output:
  - RemediationRequest CRD created
  - spec.signalType="kubernetes_event"
  - Different fingerprint calculation (SHA256 of reason:namespace:kind:name)
Expected RO Behavior: RO processes K8s Event signals same as Prometheus alerts
Test Data:
  POST http://gateway:8080/webhook/kubernetes
  Content-Type: application/json
  {
    "type": "Warning",
    "reason": "BackOff",
    "message": "Back-off restarting failed container",
    "involvedObject": {
      "kind": "Pod",
      "namespace": "production",
      "name": "api-server-123"
    },
    "firstTimestamp": "2025-12-13T10:00:00Z",
    "lastTimestamp": "2025-12-13T10:05:00Z",
    "count": 5
  }
```

**Scenario 4**: Gateway audit trail → DataStorage receives audit event
```yaml
Description: Gateway writes audit event for every signal ingestion
Expected Gateway Output:
  - Audit event written to DataStorage
  - Event type: "gateway.signal.ingested"
  - Contains signal metadata and fingerprint
Expected RO Behavior: N/A (audit is informational)
Verification: Query DataStorage for audit events with type="gateway.signal.ingested"
```

**Additional Scenarios**:
```yaml
# Scenario 5: Invalid signal → Gateway rejects with 400
Description: Gateway validates signal format, rejects malformed JSON
Expected Gateway Output: HTTP 400 Bad Request
Expected RO Behavior: N/A (no RR created)

# Scenario 6: Gateway namespace not found → Fallback to default
Description: Gateway handles missing target namespace gracefully
Expected Gateway Output: RR created in default namespace (per BR-GATEWAY-006)
Expected RO Behavior: RO processes RR from default namespace
```

---

### **7. Contact & Availability**

**Team Contact**: Gateway Team (via handoff documents)
**Slack Channel**: #gateway-service
**Available for E2E Test Review**: ✅ **Yes**
**Preferred Review Time**: Weekdays 9am-5pm EST
**Documentation**:
  - `docs/handoff/GATEWAY_E2E_FINAL_STATUS.md` - Current E2E infrastructure status
  - `docs/services/stateless/gateway-service/README.md` - Authoritative Gateway docs
  - `docs/handoff/DS_TEAM_GATEWAY_E2E_DATASTORAGE_ISSUE.md` - DataStorage deployment guide

**Note**: Gateway E2E infrastructure is 95% complete (as of Dec 13, 2025). Estimated ready for RO integration testing by **December 16, 2025**.

---

## 🟢 **SignalProcessing Team**

**Segment**: Segment 2 - RO → SP → RO

**Priority**: P0 (V1.0) - **READY TO START NOW**

### **1. E2E Readiness Status**

**Status**: ⬜ Ready / ⬜ Blocked / ⬜ In Progress

**Blockers** (if any):
- [ ]

**Estimated Ready Date** (if blocked):

---

### **2. Deployment Configuration**

**Deployment Method**: ⬜ Helm / ⬜ Kustomize / ⬜ Raw YAML / ⬜ Other: _______

**Manifest Location**:
```
Path:
```

**Container Image**:
```
Repository:
Tag:
```

**Namespace**:
```
Namespace: kubernaut-system (or specify)
```

---

### **3. Environment Variables**

```yaml
env:
  # Data Storage URL (for audit events - BR-SP-090)
  - name: DATASTORAGE_URL
    value: "http://datastorage.kubernaut-system.svc.cluster.local:8080"

  # Rego Policy Paths (BR-SP-051, BR-SP-070, BR-SP-102)
  - name: ENVIRONMENT_POLICY_PATH
    value: "/etc/signalprocessing/policies/environment.rego"
  - name: PRIORITY_POLICY_PATH
    value: "/etc/signalprocessing/policies/priority.rego"
  - name: CUSTOMLABELS_POLICY_PATH
    value: "/etc/signalprocessing/policies/customlabels.rego"

  # Health and metrics ports
  - name: HEALTH_PROBE_BIND_ADDRESS
    value: ":8081"
  - name: METRICS_BIND_ADDRESS
    value: ":9090"

  # Leader election (for HA deployments)
  - name: ENABLE_LEADER_ELECTION
    value: "true"
```

**Note**: SP controller uses ConfigMap-mounted Rego policies (not env vars for policy content). See ConfigMaps above.

---

### **4. Dependencies**

**Required Services**:
- [x] **PostgreSQL** (audit storage via Data Storage) - Deployed in SP E2E/integration infrastructure
- [x] **Redis** (DLQ for failed audit writes) - Deployed in SP E2E/integration infrastructure
- [x] **Data Storage** (audit API - BR-SP-090) - Built and deployed via shared infrastructure
- [x] **Kubernetes API** (CRD watching and K8s enrichment) - Available in Kind cluster
- [x] **audit_events table** - Applied via shared migration library
- [x] **Rego Policy ConfigMaps** - Required for environment, priority, and CustomLabels evaluation

**External Services** (if any):
```
Service: None - SP is fully self-contained
Endpoint: N/A
Mock available: N/A

Note: SP watches SignalProcessing CRDs created by RO and enriches them with:
  - Kubernetes context (namespace, pod, deployment, node)
  - Environment classification (production, staging, development)
  - Priority assignment (critical, high, medium, low)
  - Business classification (criticality, SLA requirements)
  - CustomLabels (customer-defined Rego policies)
```

---

### **5. Health Check**

**Health Endpoint**:
```
URL: http://signalprocessing-controller.kubernaut-system.svc.cluster.local:8081/healthz
Expected Status: 200
Note: Port 8081 (not 8080) per controller-runtime standards
```

**Metrics Endpoint**:
```
URL: http://signalprocessing-controller.kubernaut-system.svc.cluster.local:9090/metrics
Expected Status: 200 (Prometheus format)
Auth: Requires Kubernetes ServiceAccount token
Metrics Include:
  - signalprocessing_phase_transitions_total (phase transition counts)
  - signalprocessing_enrichment_duration_seconds (K8s enrichment performance)
  - signalprocessing_classification_confidence (classification quality)
```

**Readiness Check**:
```bash
# 1. Wait for SP controller deployment to be available
kubectl wait --for=condition=available \
  deployment/signalprocessing-controller \
  -n kubernaut-system \
  --timeout=120s

# 2. Wait for SP controller pod to be ready
kubectl wait --for=condition=ready \
  pod -l app=signalprocessing-controller \
  -n kubernaut-system \
  --timeout=120s

# 3. Verify Data Storage is ready (audit dependency)
kubectl wait --for=condition=ready \
  pod -l app=datastorage \
  -n kubernaut-system \
  --timeout=120s

# 4. Verify Rego policy ConfigMaps exist
kubectl get configmap signalprocessing-policies -n kubernaut-system
```

---

### **6. Test Scenarios for RO→SP Contract**

**Scenario 1**: RO creates SP CRD → SP completes 4-phase enrichment → RO receives full context
```yaml
Description: SP enriches production signal with full K8s context, environment, priority, and CustomLabels
Expected RO Behavior:
  - RO monitors SP status.phase transitions: Pending → Enriching → Classifying → Completed
  - RO reads status.kubernetesContext for K8s enrichment data
  - RO reads status.environmentClassification for environment determination
  - RO reads status.priorityAssignment for priority level
  - RO reads status.businessClassification for criticality assessment
  - RO reads status.customLabels for customer-defined labels
  - RO transitions to Analyzing phase with AIAnalysis CRD

Expected SP Output:
  status:
    phase: "Completed"
    kubernetesContext:
      namespace:
        name: "production"
        labels:
          environment: "prod"
          team: "platform"
      pod:
        name: "api-server-abc123"
        phase: "Running"
        labels:
          app: "api-server"
        containerStatuses:
          - name: "app"
            ready: true
            restartCount: 5
      deployment:
        name: "api-server"
        replicas: 3
        readyReplicas: 2
      node:
        name: "node-worker-01"
        labels:
          node.kubernetes.io/instance-type: "m5.large"
    environmentClassification:
      environment: "production"
      confidence: 0.95
      detectedLabels:
        isProduction: true
        hasPDB: true
    priorityAssignment:
      priority: "critical"
      confidence: 0.90
      reason: "High severity alert in production with multiple restarts"
    businessClassification:
      criticality: "business-critical"
      slaRequirement: "99.9"
      costTier: "high"
    customLabels:
      team: ["platform"]
      cost-center: ["eng-infra"]

Test Data (SignalProcessing CRD created by RO):
---
apiVersion: kubernaut.ai/v1alpha1
kind: SignalProcessing
metadata:
  name: sp-ro-e2e-test-001
  namespace: production
spec:
  remediationRequestRef:
    name: rr-e2e-test-001
    namespace: production
  signal:
    type: "prometheus_alert"
    severity: "critical"
    source: "prometheus"
    labels:
      alertname: "PodCrashLooping"
      namespace: "production"
      pod: "api-server-abc123"
      container: "app"
    annotations:
      summary: "Pod is crash looping"
      description: "Pod api-server-abc123 has restarted 5 times"
    firingTime: "2025-12-18T10:00:00Z"
```

**Scenario 2**: SP handles missing target pod gracefully (degraded mode - BR-SP-062)
```yaml
Description: SP enriches signal when target pod doesn't exist (e.g., already deleted)
Expected RO Behavior:
  - RO receives partial enrichment with namespace context only
  - RO proceeds to Analyzing phase with available data
Expected SP Output:
  status:
    phase: "Completed"
    kubernetesContext:
      namespace:
        name: "production"
        labels:
          environment: "prod"
      # pod: null (pod not found)
      # deployment: null (deployment not found)
    environmentClassification:
      environment: "production"  # Still classified from namespace labels
      confidence: 0.80
    priorityAssignment:
      priority: "high"  # Degraded confidence
      confidence: 0.70
    customLabels:
      stage: ["prod"]  # Fallback labels from namespace

Test Data:
---
apiVersion: kubernaut.ai/v1alpha1
kind: SignalProcessing
metadata:
  name: sp-ro-e2e-test-missing-pod
  namespace: production
spec:
  remediationRequestRef:
    name: rr-e2e-test-002
    namespace: production
  signal:
    type: "prometheus_alert"
    severity: "warning"
    source: "prometheus"
    labels:
      alertname: "PodNotFound"
      namespace: "production"
      pod: "deleted-pod-xyz"  # Pod doesn't exist
```

**Scenario 3**: SP emits audit events → RO can correlate SP processing (BR-SP-090)
```yaml
Description: Verify audit events are written to Data Storage for SP lifecycle
Expected RO Behavior:
  - RO can query audit_events table to verify SP processing history
  - RO correlates SP events with RemediationRequest via correlation_id

Expected Audit Events (5 total):
  Event 1 (signal processed):
    event_type: "signalprocessing.signal.processed"
    event_outcome: "success"
    correlation_id: "rr-e2e-test-001"
    metadata:
      signalType: "prometheus_alert"
      namespace: "production"

  Event 2 (phase transition):
    event_type: "signalprocessing.phase.transition"
    event_outcome: "success"
    correlation_id: "rr-e2e-test-001"
    metadata:
      fromPhase: "Pending"
      toPhase: "Enriching"

  Event 3 (classification decision):
    event_type: "signalprocessing.classification.decision"
    event_outcome: "success"
    correlation_id: "rr-e2e-test-001"
    metadata:
      environment: "production"
      priority: "critical"

  Event 4 (enrichment completed):
    event_type: "signalprocessing.enrichment.completed"
    event_outcome: "success"
    correlation_id: "rr-e2e-test-001"
    metadata:
      hasDeployment: true
      hasPod: true
      detectedLabelsCount: 5

  Event 5 (error - if any):
    event_type: "signalprocessing.error.occurred"
    event_outcome: "failure"
    correlation_id: "rr-e2e-test-001"
    metadata:
      errorType: "KubernetesAPIError"
      errorMessage: "Failed to fetch pod"

Validation Query (PostgreSQL):
  SELECT event_type, event_outcome, metadata, timestamp
  FROM audit_events
  WHERE correlation_id = 'rr-e2e-test-001'
    AND service_name = 'signalprocessing'
  ORDER BY timestamp ASC;
```

**Scenario 4**: SP hot-reloads Rego policy → Classification changes (BR-SP-072)
```yaml
Description: SP detects ConfigMap policy update and reloads without pod restart
Expected RO Behavior:
  - No action needed - transparent to RO
  - Future SP CRDs use new policy automatically
Expected SP Output:
  - New SP CRDs reflect updated policy decisions
  - Controller logs show "policy hot-reloaded successfully"
Test Data:
  1. Create initial SP CRD → receives priority "high"
  2. Update priority.rego ConfigMap → change "critical" threshold
  3. Create second SP CRD (same signal) → receives priority "critical"
  4. No pod restart occurred

Validation:
  kubectl logs -n kubernaut-system deploy/signalprocessing-controller | grep "hot-reload"
  # Should show: "Rego policy hot-reloaded successfully"
```

**Scenario 5**: SP handles invalid signal gracefully → RO receives validation error
```yaml
Description: SP validates signal spec and fails fast on invalid data
Expected RO Behavior:
  - RO transitions to Failed phase
  - RO captures error message for operator notification
Expected SP Output:
  status:
    phase: "Failed"
    error:
      type: "ValidationError"
      message: "Signal type 'invalid_type' not supported"
      timestamp: "2025-12-18T10:05:00Z"

Test Data (Invalid SignalProcessing CRD):
---
apiVersion: kubernaut.ai/v1alpha1
kind: SignalProcessing
metadata:
  name: sp-ro-e2e-test-invalid
  namespace: production
spec:
  remediationRequestRef:
    name: rr-e2e-test-003
    namespace: production
  signal:
    type: "invalid_type"  # Invalid signal type
    severity: "unknown"   # Invalid severity
    # Missing required fields
```

**Additional Scenarios**:
```yaml
# Scenario 6: SP completes under 5 seconds (BR-SP-060 performance target)
Description: Verify SP enrichment latency for production readiness
Expected: status.duration < 5s for 95th percentile
Validation: Check status.processingDuration field

# Scenario 7: SP handles concurrent signals (BR-SP-061)
Description: RO creates 10 SP CRDs simultaneously
Expected: All 10 complete successfully without resource contention
Validation: All status.phase="Completed" within 10 seconds

# Scenario 8: SP survives pod restart (graceful shutdown - BR-SP-063)
Description: Kubernetes restarts SP controller pod mid-reconciliation
Expected: In-flight SP CRDs are reconciled after restart (no data loss)
Validation: status.phase transitions continue from last checkpoint
```

---

### **7. Contact & Availability**

**Team Contact**: SignalProcessing Team (via handoff documents)
**Slack Channel**: #signalprocessing-service
**Available for E2E Test Review**: ✅ **Yes**
**Preferred Review Time**: Flexible - Any time week of Dec 16-20, 2025

**Reference Documentation**:
- **Service Docs**: `docs/services/crd-controllers/01-signalprocessing/`
- **E2E Tests**: `test/e2e/signalprocessing/` (11 tests, 100% passing)
- **Integration Tests**: `test/integration/signalprocessing/` (59/62 passing)
- **Business Requirements**: `docs/services/crd-controllers/01-signalprocessing/BUSINESS_REQUIREMENTS.md`
- **Handoff Document**: `docs/handoff/SP_SERVICE_HANDOFF.md`
- **Hot-Reload Deployment**: `docs/services/crd-controllers/01-signalprocessing/CONFIGMAP_HOTRELOAD_DEPLOYMENT.md`

**Integration Notes for RO Team**:
1. **CRD Lifecycle**: SP CRDs created in **same namespace as RemediationRequest** (not `kubernaut-system`)
2. **Phase Watching**: RO should watch `status.phase` for transitions: Pending → Enriching → Classifying → Completed
3. **Degraded Mode**: SP completes successfully even if target pod/deployment not found (partial enrichment)
4. **Audit Trail**: All SP events use `correlation_id = RemediationRequest.metadata.name` for linking
5. **Rego Policies**: SP requires ConfigMap `signalprocessing-policies` with 3 Rego files
6. **Performance**: SP enrichment typically completes in < 3 seconds (95th percentile)

**Known Issues**:
- 3 component integration tests need infrastructure fixes (K8sEnricher in ENVTEST)
- Core business logic 100% functional and production-ready

**Estimated Integration Effort**: 1-2 days for RO team to integrate with SP E2E patterns

---

**Updated by SignalProcessing Team**: December 13, 2025

---

## 🟢 **AIAnalysis Team**

**Segment**: Segment 3 - RO → AA → HAPI → AA → RO

**Priority**: P1 (V1.1) - **READY FOR INTEGRATION** (Controller production-ready, E2E infrastructure established)

### **1. E2E Readiness Status**

**Status**: ✅ **Ready** (with minor caveats - see below)

**Blockers** (if any):
- [x] AA controller implementation complete (as of Dec 13, 2025)
- [x] E2E infrastructure functional (25 tests, 18 passing in last successful run)
- [ ] Podman infrastructure stability (transient issues on macOS)
- [ ] 7 E2E tests need minor fixes (policy evaluation, health checks, metrics)

**Estimated Ready Date** (if blocked): **December 18, 2025** (3 days for final E2E stabilization)

**Current Status**: AIAnalysis controller is production-ready with:
- ✅ Full 4-phase reconciliation (Pending → Investigating → Analyzing → Completed)
- ✅ HAPI integration with mock mode support
- [Deprecated - Issue #180] Recovery flow implementation
- ✅ Rego policy-based approval decisions
- ✅ E2E test infrastructure deployed and functional

---

### **2. Deployment Configuration**

**Deployment Method**: ⬜ Helm / ⬜ Kustomize / ⬜ Raw YAML / ⬜ Other: _______

**Manifest Location**:
```
Path:
```

**Container Image**:
```
Repository:
Tag:
```

**Namespace**:
```
Namespace: kubernaut-system (or specify)
```

---

### **3. Environment Variables**

```yaml
env:
  # HAPI Configuration (BR-AI-001: AI analysis integration)
  - name: HOLMESGPT_API_URL
    value: "http://holmesgpt-api.kubernaut-system.svc.cluster.local:8088"
  - name: HOLMESGPT_API_TIMEOUT
    value: "120s"

  # Data Storage URL (for audit events + workflow catalog - BR-AI-017)
  - name: DATASTORAGE_URL
    value: "http://datastorage.kubernaut-system.svc.cluster.local:8080"

  # Rego Policy Path (BR-AI-011: Policy evaluation)
  - name: REGO_POLICY_PATH
    value: "/etc/aianalysis/policies/approval.rego"

  # Health and metrics ports
  - name: HEALTH_PROBE_BIND_ADDRESS
    value: ":8081"
  - name: METRICS_BIND_ADDRESS
    value: ":9090"

  # Leader election (for HA deployments)
  - name: ENABLE_LEADER_ELECTION
    value: "true"

# Configuration via ConfigMap (Rego policy mounted at /etc/aianalysis/policies/):
configMaps:
  - name: aianalysis-policies
    mountPath: /etc/aianalysis/policies/
    files:
      - approval.rego  # Rego policy for approval decisions
```

**Note**: HAPI configuration uses `HOLMESGPT_API_URL` (not `HAPI_URL`) per implementation

---

### **4. Dependencies**

**Required Services**:
- [x] **PostgreSQL** (audit storage via Data Storage) - Deployed in AA E2E infrastructure
- [x] **Redis** (DLQ for failed audit writes) - Deployed in AA E2E infrastructure
- [x] **Data Storage** (audit API + workflow catalog) - Built and deployed in AA E2E setup
- [x] **HolmesGPT-API** (AI analysis) - **MOCK_LLM_MODE=true available** ✅
- [x] **audit_events table** - Applied via shared migration library
- [x] **Rego Policy ConfigMap** - Applied for approval decisions (BR-AI-011)

**External Services** (if any):
```
Service: HolmesGPT-API (HAPI)
Endpoint: http://holmesgpt-api.kubernaut-system.svc.cluster.local:8088
Mock Mode: YES - Use MOCK_LLM_MODE=true (no LLM API calls, deterministic responses)
Mock Behavior:
  - Returns deterministic RCA for known alert patterns
  - Returns workflow recommendations with confidence scores
  - Supports recovery endpoint for failed workflow analysis
  - No external OpenAI/LLM API calls required
```

**Critical Note**: HAPI must be deployed with `MOCK_LLM_MODE=true` for E2E tests to work reliably without external LLM dependencies.

**HAPI Health Check Required Before AA Start**:
```bash
# Verify HAPI is ready before starting AA controller
kubectl wait --for=condition=ready \
  pod -l app=holmesgpt-api \
  -n kubernaut-system \
  --timeout=120s

# Verify HAPI health endpoint
curl http://holmesgpt-api.kubernaut-system.svc.cluster.local:8088/health
```

---

### **5. Health Check**

**Health Endpoint**:
```
URL: http://aianalysis-controller.kubernaut-system.svc.cluster.local:8081/healthz
Expected Status: 200
Note: Port 8081 (not 8080) per controller-runtime standards
```

**Metrics Endpoint**:
```
URL: http://aianalysis-controller.kubernaut-system.svc.cluster.local:9090/metrics
Expected Status: 200 (Prometheus format)
Auth: Requires Kubernetes ServiceAccount token
Metrics Include:
  - aianalysis_phase_transitions_total (BR-AI-022)
  - aianalysis_confidence_score_distribution (BR-AI-022)
  - aianalysis_rego_evaluation_duration_seconds (BR-AI-022)
  - aianalysis_approval_decisions_total (BR-AI-022)
```

**Readiness Check**:
```bash
# 1. Wait for AA controller deployment to be available
kubectl wait --for=condition=available \
  deployment/aianalysis-controller \
  -n kubernaut-system \
  --timeout=120s

# 2. Wait for AA controller pod to be ready
kubectl wait --for=condition=ready \
  pod -l app=aianalysis-controller \
  -n kubernaut-system \
  --timeout=120s

# 3. Verify HAPI is ready (critical dependency)
kubectl wait --for=condition=ready \
  pod -l app=holmesgpt-api \
  -n kubernaut-system \
  --timeout=120s

# 4. Verify Data Storage is ready
kubectl wait --for=condition=ready \
  pod -l app=datastorage \
  -n kubernaut-system \
  --timeout=120s

# 5. Verify Rego policy ConfigMap exists
kubectl get configmap aianalysis-policies -n kubernaut-system
```

---

### **6. Test Scenarios for RO→AA→HAPI Contract**

**Scenario 1**: RO creates AA CRD → AA completes 4-phase cycle → RO receives workflow recommendation
```yaml
Description: AA analyzes production incident, HAPI recommends workflow, policy requires approval
Expected RO Behavior:
  - RO monitors AA status.phase transitions: Pending → Investigating → Analyzing → Completed
  - RO reads status.selectedWorkflow for workflow execution
  - RO checks status.approvalSignaling.requiresApproval for approval decision
Expected AA Output:
  status:
    phase: Completed
    selectedWorkflow:
      workflowID: "wf-restart-pod-v1"
      containerImage: "kubernaut.io/workflows/restart:v1.0.0"
      confidence: 0.85
    approvalSignaling:
      requiresApproval: true
      reason: "Production environment requires manual approval"
Test Data:
  apiVersion: kubernaut.ai/v1alpha1
  kind: AIAnalysis
  metadata:
    name: test-production-analysis
    namespace: production
  spec:
    analysisRequest:
      signalContext:
        signalType: "prometheus_alert"
        severity: "critical"
        environment: "production"
        targetResource:
          kind: "Pod"
          name: "api-server-123"
          namespace: "production"
```

**Scenario 2**: AA returns "WorkflowNotNeeded" (BR-HAPI-200) → RO completes without workflow execution
```yaml
Description: HAPI determines problem self-resolved (e.g., pod already recovered)
Expected RO Behavior:
  - RO marks remediation as Completed
  - RO does NOT create WorkflowExecution CRD
  - RO captures RCA for learning/audit
Expected AA Output:
  status:
    phase: Completed
    reason: "WorkflowNotNeeded"
    subReason: "ProblemResolved"
    rootCauseAnalysis:
      summary: "Pod recovered automatically before workflow execution"
      severity: "info"
Test Data:
  # Similar to Scenario 1, but HAPI mock returns high confidence + no workflow
  spec:
    analysisRequest:
      signalContext:
        confidence: 0.95  # High confidence
```

**Scenario 3**: AA encounters recovery scenario → Uses /recovery/analyze endpoint
```yaml
Description: RO creates AA for failed workflow recovery analysis (BR-AI-083)
Expected RO Behavior:
  - RO sets spec.isRecoveryAttempt=true
  - RO provides spec.previousExecutions with failed workflow details
  - RO monitors recovery-specific status.recoveryStatus
Expected AA Output:
  status:
    phase: Completed
    recoveryStatus:
      previousAttemptAssessment:
        failureUnderstood: true
        failureCategory: "resource_exhaustion"
      stateChanged: true
      recommendedAction: "scale_up"
    selectedWorkflow:
      workflowID: "wf-scale-deployment-v1"
      confidence: 0.75
Test Data:
  spec:
    isRecoveryAttempt: true
    recoveryAttemptNumber: 1
    previousExecutions:
      - workflowID: "wf-restart-pod-v1"
        executionID: "exec-123"
        outcome: "Failed"
        failureDetails: "Pod crashed again after restart"
```

**Scenario 4**: AA policy evaluation requires approval (BR-AI-011, BR-AI-013)
```yaml
Description: Rego policy determines manual approval required based on environment/data quality
Expected RO Behavior:
  - RO reads status.approvalSignaling.requiresApproval=true
  - RO creates RemediationApprovalRequest CRD
  - RO transitions to AwaitingApproval phase
Expected AA Output:
  status:
    phase: Completed
    approvalSignaling:
      requiresApproval: true
      reason: "Production environment requires manual approval"
      policyEvaluation:
        policyName: "aianalysis.approval"
        decision: "manual_review_required"
Test Data:
  spec:
    analysisRequest:
      signalContext:
        environment: "production"  # Always requires approval per policy
```

**Scenario 5**: HAPI returns needs_human_review=true (BR-HAPI-197)
```yaml
Description: HAPI workflow resolution failed, requires human intervention
Expected RO Behavior:
  - RO creates manual review notification
  - RO transitions to ManualReview phase (if supported)
  - RO captures validation attempts history for debugging
Expected AA Output:
  status:
    phase: Completed
    reason: "WorkflowResolutionFailed"
    subReason: "NeedsHumanReview"
    humanReview:
      required: true
      reason: "target_validation_failed"
      validationAttemptsHistory: [...]
Test Data:
  # HAPI mock configured to return needs_human_review=true
```

**Additional Scenarios**:
```yaml
# Scenario 6: AA handles HAPI timeout gracefully
Description: HAPI investigation exceeds timeout (BR-AI-007)
Expected AA Output: status.phase=Failed, status.message="HAPI timeout"
Expected RO Behavior: RO creates timeout notification

# Scenario 7: AA transitions through all 4 phases with metrics recording (BR-AI-022)
Description: Verify all phase transitions record Prometheus metrics
Expected: aianalysis_phase_transitions_total{phase="Investigating"} > 0

# Scenario 8: AA handles data quality warnings (BR-AI-008)
Description: HAPI returns warnings about data quality
Expected AA Output: status.warnings=["High memory pressure", "..."]
Expected RO Behavior: RO logs warnings for operator visibility
```

---

### **7. Contact & Availability**

**Team Contact**: AIAnalysis Team (via handoff documents)
**Slack Channel**: #aianalysis-service
**Available for E2E Test Review**: ✅ **Yes**
**Preferred Review Time**: Weekdays 9am-5pm EST, Flexible during week of Dec 16-20, 2025

**Reference Documentation**:
- **Service Docs**: `docs/services/crd-controllers/02-aianalysis/`
- **E2E Infrastructure**: `test/infrastructure/aianalysis.go`
- **E2E Tests**: `test/e2e/aianalysis/` (25 tests, 18 passing in last successful run)
- **Business Requirements**: `docs/services/crd-controllers/02-aianalysis/BUSINESS_REQUIREMENTS.md`
- **Handoff Document**: `docs/handoff/HANDOFF_AIANALYSIS_SERVICE_COMPLETE_2025-12-13.md`
- **E2E Test Patterns**: `docs/services/crd-controllers/02-aianalysis/E2E_TEST_PATTERNS_AND_TROUBLESHOOTING.md`
- **HAPI Contract**: `docs/handoff/NOTICE_AIANALYSIS_HAPI_CONTRACT_MISMATCH.md`

**Current E2E Status** (as of Dec 13, 2025):
- ✅ Infrastructure functional (Kind cluster, HAPI mock mode, Data Storage)
- ✅ 18/25 tests passing (72% pass rate validates core orchestration)
- ⚠️ 7 tests need minor fixes (policy evaluation timing, health checks, metrics)
- ⚠️ Podman infrastructure stability issues on macOS (transient)

**Integration Notes for RO Team**:
1. **HAPI Mock Mode**: Ensure `MOCK_LLM_MODE=true` is set in HAPI deployment
2. **Policy ConfigMap**: AA requires `aianalysis-policies` ConfigMap with Rego policy
3. **Namespace**: AA CRDs created in same namespace as incident (not `kubernaut-system`)
4. **Phase Watching**: RO should watch `status.phase` for transitions, not rely on events
5. **Approval Signaling**: Check `status.approvalSignaling.requiresApproval` for approval decisions
6. **Recovery Flow**: Set `spec.isRecoveryAttempt=true` and provide `spec.previousExecutions`

**Known Issues**:
- Podman infrastructure instability (affects cluster setup, not AA controller)
- 7 E2E tests need fixes (policy timing, health endpoints, metrics exposure)
- HAPI mock responses fully functional for primary scenarios

**Estimated Integration Effort**: 2-3 days for RO team to integrate with AA E2E patterns

---

## 🟢 **WorkflowExecution Team**

**Segment**: Segment 4 - RO → WE → RO

**Priority**: P0 (V1.0) - **READY TO START NOW** (WE is production-ready)

### **1. E2E Readiness Status**

**Status**: ✅ **Ready**

**Blockers** (if any):
- None - WE controller is production-ready with full E2E test infrastructure

**Estimated Ready Date** (if blocked): N/A - Ready now

---

### **2. Deployment Configuration**

**Deployment Method**: ✅ **Raw YAML** (Generated via controller-gen)

**Manifest Location**:
```
Path: test/infrastructure/workflowexecution.go (DeployWorkflowExecutionController function)
Build Command: make manifests (generates CRD YAML)
```

**Container Image**:
```
Repository: Built locally from cmd/workflowexecution
Tag: Latest (test builds use local images loaded into Kind)
Build: podman build -f docker/workflowexecution.Dockerfile -t localhost/kubernaut-workflowexecution:latest
```

**Namespaces**:
```
Controller Namespace: kubernaut-system
Execution Namespace: kubernaut-workflows (where Tekton PipelineRuns execute)
```

**Tekton Pipelines Required**: ✅ **Yes** (Critical dependency)

**Tekton Installation**:
```
Version: v1.7.0+ (uses ghcr.io, no auth required)
Installation: Automated in CreateWorkflowExecutionCluster()
Command: kubectl apply -f https://storage.googleapis.com/tekton-releases/pipeline/previous/v1.7.0/release.yaml
```

---

### **3. Environment Variables**

```yaml
env:
  # Data Storage URL (for audit events - BR-WE-005)
  - name: DATASTORAGE_URL
    value: "http://datastorage.kubernaut-system.svc.cluster.local:8080"

  # Health and metrics ports
  - name: HEALTH_PROBE_BIND_ADDRESS
    value: ":8081"
  - name: METRICS_BIND_ADDRESS
    value: ":9090"

  # Leader election (for HA deployments)
  - name: ENABLE_LEADER_ELECTION
    value: "true"
```

---

### **4. Dependencies**

**Required Services**:
- [x] **PostgreSQL** (audit storage via Data Storage) - Deployed in WE E2E infrastructure
- [x] **Redis** (DLQ for failed audit writes) - Deployed in WE E2E infrastructure
- [x] **Data Storage** (audit API) - Built and deployed in WE E2E setup
- [x] **Tekton Pipelines** (workflow execution engine) - Installed automatically in Kind cluster
- [x] **audit_events table** - Applied via shared migration library (`test/infrastructure/migrations.go`)

**External Services** (if any):
```
Service: None - All dependencies are containerized in Kind cluster
Endpoint: N/A
Mock available: N/A (uses real Tekton PipelineRuns in Kind)
```

**Note**: WE uses **real Tekton execution** (not mocks) to validate actual workflow behavior

---

### **5. Health Check**

**Health Endpoint**:
```
URL: http://workflowexecution-controller.kubernaut-system.svc.cluster.local:8081/healthz
Expected Status: 200
Note: Port 8081 (not 8080) per DD-TEST-001
```

**Metrics Endpoint**:
```
URL: http://workflowexecution-controller.kubernaut-system.svc.cluster.local:9090/metrics
Expected Status: 200
Auth: Requires Kubernetes ServiceAccount token
```

**Readiness Check**:
```bash
# 1. Wait for WE controller deployment to be available
kubectl wait --for=condition=available \
  deployment/workflowexecution-controller \
  -n kubernaut-system \
  --timeout=120s

# 2. Wait for WE controller pod to be ready
kubectl wait --for=condition=ready \
  pod -l app=workflowexecution-controller \
  -n kubernaut-system \
  --timeout=120s

# 3. Verify Tekton Pipelines is ready (critical dependency)
kubectl wait --for=condition=ready \
  pod -l app=tekton-pipelines-controller \
  -n tekton-pipelines \
  --timeout=120s

# 4. Verify execution namespace exists
kubectl get namespace kubernaut-workflows
```

---

### **6. Test Scenarios for RO→WE Contract**

**Scenario 1**: RO creates WE CRD → WE executes Tekton pipeline → RO receives completion
```yaml
Description: WE successfully executes workflow via Tekton PipelineRun
Expected RO Behavior:
  - RO transitions to Executing phase
  - RO watches WE status.phase for completion
  - RO transitions to Completed when WE completes
Expected WE Output:
  status:
    phase: "Completed"
    outcome: "Success"
    pipelineRunRef:
      name: "workflow-exec-abc123"  # Created in kubernaut-workflows namespace
    completionTime: "2025-12-18T10:15:30Z"
    duration: "45s"

Test Data (WorkflowExecution CRD):
---
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: ro-e2e-test-success
  namespace: kubernaut-system
spec:
  remediationRequestRef:
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: rr-e2e-test-001
    namespace: kubernaut-system
  workflowRef:
    workflowID: test-hello-world
    version: v1.0.0
    containerImage: quay.io/kubernaut/workflows/test-hello-world:v1.0.0
  targetResource: default/deployment/payment-api
  parameters:
    MESSAGE: "RO E2E test - scaling payment-api"
    TARGET_REPLICAS: "3"
```

**Scenario 2**: WE emits audit events → RO verifies audit trail (BR-WE-005)
```yaml
Description: Verify audit events are written to Data Storage for lifecycle transitions
Expected RO Behavior:
  - RO can query audit_events table to verify WE execution history
  - RO correlates WE events with RemediationRequest events

Expected Audit Events:
  Event 1 (workflow started):
    event_type: "workflowexecution.workflow.started"
    event_outcome: "success"
    service_name: "workflowexecution"
    correlation_id: "rr-e2e-test-001"  # Links to RemediationRequest
    metadata:
      workflowId: "test-hello-world"
      targetResource: "default/deployment/payment-api"
    timestamp: "2025-12-18T10:14:45Z"

  Event 2 (workflow completed):
    event_type: "workflowexecution.workflow.completed"
    event_outcome: "success"
    service_name: "workflowexecution"
    correlation_id: "rr-e2e-test-001"
    metadata:
      workflowId: "test-hello-world"
      targetResource: "default/deployment/payment-api"
      duration: "45s"
      pipelineRunName: "workflow-exec-abc123"
    timestamp: "2025-12-18T10:15:30Z"

Validation Query (PostgreSQL):
  SELECT event_type, event_outcome, metadata->>'workflowId', timestamp
  FROM audit_events
  WHERE service_name = 'workflowexecution'
    AND correlation_id = 'rr-e2e-test-001'
  ORDER BY timestamp ASC;

Test Data: Use Scenario 1 WorkflowExecution, then query audit_events table
```

**Scenario 3**: WE fails execution → RO transitions to Failed with error details
```yaml
Description: Tekton PipelineRun fails with TaskRun error
Expected RO Behavior:
  - RO transitions to Failed phase
  - RO captures failureDetails for potential recovery flow
  - RO creates failure notification with error context

Expected WE Output:
  status:
    phase: "Failed"
    outcome: "Failed"
    failureDetails:
      wasExecutionFailure: true  # Workflow DID execute (may have side effects)
      failureReason: "TaskFailed"
      failureMessage: "Task apply-memory-increase failed: kubectl scale failed with exit code 1"
      failedTaskName: "apply-memory-increase"
      pipelineRunName: "workflow-exec-xyz789"
      timestamp: "2025-12-18T10:20:15Z"
    completionTime: "2025-12-18T10:20:15Z"

Expected Audit Event:
  event_type: "workflowexecution.workflow.failed"
  event_outcome: "failure"
  metadata:
    failureReason: "TaskFailed"
    wasExecutionFailure: true

Test Data (WorkflowExecution CRD):
---
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: ro-e2e-test-failure
  namespace: kubernaut-system
spec:
  remediationRequestRef:
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: rr-e2e-test-002
    namespace: kubernaut-system
  workflowRef:
    workflowID: test-intentional-failure
    version: v1.0.0
    containerImage: quay.io/kubernaut/workflows/test-intentional-failure:v1.0.0
  targetResource: default/deployment/payment-api
  parameters:
    FAILURE_REASON: "RO E2E test - simulated task failure"
```

**Scenario 4**: WE skips due to resource lock → RO handles skip reason correctly
```yaml
Description: WE skips execution due to resource busy (BR-WE-009, DD-WE-001)
Expected RO Behavior:
  - RO marks second RemediationRequest as Skipped
  - RO tracks skip reason for reporting
  - RO does NOT create failure notification (expected behavior)

Expected WE Output (Second WFE):
  status:
    phase: "Skipped"
    skipDetails:
      reason: "ResourceBusy"
      conflictingWorkflow:
        name: "ro-e2e-test-wfe1"
        namespace: "kubernaut-system"
        startTime: "2025-12-18T10:14:00Z"
      message: "Another workflow is executing on target default/deployment/payment-api"
    # NO pipelineRunRef (no PipelineRun created for skipped executions)

Expected Audit Event:
  event_type: "workflowexecution.workflow.skipped"
  event_outcome: "skipped"
  metadata:
    skipReason: "ResourceBusy"
    conflictingWorkflow: "ro-e2e-test-wfe1"

Test Data (Create TWO WorkflowExecutions targeting same resource):
---
# WFE #1 - Creates first, starts executing
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: ro-e2e-test-wfe1
  namespace: kubernaut-system
spec:
  remediationRequestRef:
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: rr-e2e-test-003
    namespace: kubernaut-system
  workflowRef:
    workflowID: test-hello-world
    version: v1.0.0
    containerImage: quay.io/kubernaut/workflows/test-hello-world:v1.0.0
  targetResource: default/deployment/payment-api  # Same target!
  parameters:
    MESSAGE: "First workflow executing"

---
# WFE #2 - Created while WFE #1 is Running (should be Skipped)
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: ro-e2e-test-wfe2
  namespace: kubernaut-system
spec:
  remediationRequestRef:
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: rr-e2e-test-004
    namespace: kubernaut-system
  workflowRef:
    workflowID: test-hello-world
    version: v1.0.0
    containerImage: quay.io/kubernaut/workflows/test-hello-world:v1.0.0
  targetResource: default/deployment/payment-api  # Same target as WFE #1!
  parameters:
    MESSAGE: "Should be skipped - resource busy"

# Expected: WFE #1 → status.phase=Running, WFE #2 → status.phase=Skipped (ResourceBusy)
```

**Scenario 5**: Cooldown enforcement → RO respects recently remediated (BR-WE-010)
```yaml
Description: Second WorkflowExecution with same workflow+target within cooldown period
Expected RO Behavior:
  - RO creates second WFE after first completes
  - RO marks second RR as Skipped (RecentlyRemediated)
  - RO does NOT trigger duplicate notification

Expected WE Output (Second WFE):
  status:
    phase: "Skipped"
    skipDetails:
      reason: "RecentlyRemediated"
      recentRemediation:
        name: "ro-e2e-cooldown-first"
        namespace: "kubernaut-system"
        completionTime: "2025-12-18T10:15:30Z"
        cooldownPeriod: "5m"
      cooldownRemaining: "4m30s"
      message: "Workflow test-hello-world executed on target 30s ago (cooldown: 5m)"

Test Data:
---
# WFE #1 - Execute and wait for completion
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: ro-e2e-cooldown-first
  namespace: kubernaut-system
spec:
  remediationRequestRef:
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: rr-e2e-test-005
    namespace: kubernaut-system
  workflowRef:
    workflowID: test-hello-world  # SAME workflow ID
    version: v1.0.0
    containerImage: quay.io/kubernaut/workflows/test-hello-world:v1.0.0
  targetResource: default/deployment/payment-api  # SAME target
  parameters:
    MESSAGE: "First execution"

---
# WFE #2 - Create < 5 minutes after WFE #1 completes (should be Skipped)
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: ro-e2e-cooldown-second
  namespace: kubernaut-system
spec:
  remediationRequestRef:
    apiVersion: kubernaut.ai/v1alpha1
    kind: RemediationRequest
    name: rr-e2e-test-006
    namespace: kubernaut-system
  workflowRef:
    workflowID: test-hello-world  # SAME workflow ID as WFE #1
    version: v1.0.0
    containerImage: quay.io/kubernaut/workflows/test-hello-world:v1.0.0
  targetResource: default/deployment/payment-api  # SAME target as WFE #1
  parameters:
    MESSAGE: "Should be skipped - within cooldown"

# Expected: WFE #1 → Completed, WFE #2 → Skipped (RecentlyRemediated)
# Note: DIFFERENT workflow on SAME target IS allowed (only same workflow+target blocked)
```

**Additional Scenarios** (Lower Priority):
```yaml
# Scenario 6: PipelineRun parameters passed correctly (BR-WE-002)
Description: Verify workflow parameters from spec.parameters passed to Tekton PipelineRun
Validation:
  1. Create WorkflowExecution with parameters (see Scenario 1)
  2. Get created PipelineRun: kubectl get pipelinerun -n kubernaut-workflows \
       -l kubernaut.ai/name=ro-e2e-test-success -o yaml
  3. Verify params match exactly:
       - name: MESSAGE
         value: "RO E2E test - scaling payment-api"
       - name: TARGET_REPLICAS
         value: "3"
Expected: PipelineRun params match WorkflowExecution spec.parameters (UPPER_SNAKE_CASE preserved)

# Scenario 7: Exponential backoff for pre-execution failures (BR-WE-012, DD-WE-004)
Description: Pre-execution failures trigger exponential backoff cooldown
Test: Simulate quota exceeded error (requires cluster ResourceQuota)
Expected: status.consecutiveFailures increments, nextAllowedExecution timestamp increases exponentially
Note: Requires E2E cluster with ResourceQuota configured (complex setup)
Priority: P2 (defer to later testing phase)
```

---

### **7. Contact & Availability**

**Team Contact**: WorkflowExecution Team Lead
**Slack Channel**: #workflowexecution
**Available for E2E Test Review**: ✅ **Yes**
**Preferred Review Time**: Flexible - Any time week of Dec 16-20, 2025

**Reference Documentation**:
- Service Docs: `docs/services/crd-controllers/03-workflowexecution/`
- E2E Infrastructure: `test/infrastructure/workflowexecution.go`
- E2E Tests: `test/e2e/workflowexecution/`
- Business Requirements: `docs/services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md`
- Handoff Document: `docs/handoff/HANDOFF_WORKFLOWEXECUTION_SERVICE_OWNERSHIP.md`

---

## 🟢 **Notification Team**

**Segment**: Segment 5 - RO → Notification → RO

**Priority**: P0 (V1.0) - **READY TO START NOW** (Notification is production-ready)

### **1. E2E Readiness Status**

**Status**: ✅ **Ready**

**Blockers** (if any):
- None - Notification controller is production-ready with full E2E test infrastructure

**Estimated Ready Date** (if blocked): N/A - Ready now

**Current Status**:
- ✅ 349 tests passing (225 unit, 112 integration, 12 E2E)
- ✅ E2E infrastructure functional (Kind cluster, file delivery, audit integration)
- ✅ 17/18 Business Requirements implemented (BR-NOT-069 in progress, approved for V1.0)
- ✅ Production-ready CRD controller with 5-phase lifecycle

---

### **2. Deployment Configuration**

**Deployment Method**: ✅ **Raw YAML** (Generated via controller-gen + custom manifests)

**Manifest Location**:
```
Path: test/infrastructure/notification.go (DeployNotificationController function)
Manifests: test/e2e/notification/manifests/
  - notification-deployment.yaml
  - notification-rbac.yaml
  - notification-service.yaml
  - notification-configmap.yaml (routing rules)
Build Command: make manifests (generates CRD YAML)
```

**Container Image**:
```
Repository: Built locally from cmd/notification
Tag: localhost/kubernaut-notification:e2e-test
Build: podman build -f docker/notification.Dockerfile -t localhost/kubernaut-notification:e2e-test
```

**Namespace**:
```
Namespace: kubernaut-system (controller deployment)
Note: NotificationRequest CRDs created in same namespace as source RemediationRequest
```

**File Adapter Configuration**:
```yaml
# File notification adapter (for E2E tests - DD-NOT-002)
# Console and File channels write to /tmp/notifications (HostPath volume)
env:
  - name: E2E_FILE_OUTPUT
    value: "/tmp/notifications"  # Mounted from host via Kind extraMounts
  - name: NOTIFICATION_CONSOLE_ENABLED
    value: "true"

# File delivery directory (platform-specific):
# - Linux/CI: /tmp/kubernaut-e2e-notifications
# - macOS: $HOME/.kubernaut/e2e-notifications (Podman VM mount limitation)
```

---

### **3. Environment Variables**

```yaml
env:
  # Health and metrics ports (DD-TEST-001 compliant)
  - name: HEALTH_PROBE_BIND_ADDRESS
    value: ":8081"
  - name: METRICS_BIND_ADDRESS
    value: ":9090"

  # Leader election (for HA deployments)
  - name: ENABLE_LEADER_ELECTION
    value: "false"  # Disabled for E2E (single replica)

  # Console channel (always enabled for E2E)
  - name: NOTIFICATION_CONSOLE_ENABLED
    value: "true"

  # File delivery output (E2E tests - DD-NOT-002)
  - name: E2E_FILE_OUTPUT
    value: "/tmp/notifications"  # Mounted from host

  # Slack webhook (mock endpoint for E2E)
  - name: NOTIFICATION_SLACK_WEBHOOK_URL
    value: "http://mock-slack:8080/webhook"  # Optional: Mock Slack service

  # Logging configuration
  - name: ZAP_LOG_LEVEL
    value: "info"

# Note: Data Storage URL for audit is NOT required for Notification E2E
# Notification writes audit events, but E2E tests validate CRD status and file delivery
# If RO wants to test audit integration, Data Storage should be deployed separately
```

---

### **4. Dependencies**

**Required Services**:
- [x] **NotificationRequest CRD** (cluster-wide) - Installed automatically in E2E setup
- [x] **Kubernetes API** (CRD watching) - Available in Kind cluster
- [ ] **PostgreSQL** - **NOT REQUIRED** for Notification E2E (audit optional)
- [ ] **Redis** - **NOT REQUIRED** for Notification E2E
- [ ] **Data Storage** - **NOT REQUIRED** for Notification E2E (audit optional)

**External Services** (if any):
```
Service: File delivery (DD-NOT-002)
Endpoint: Local filesystem (/tmp/notifications)
Mock available: N/A (file-based, no mocking needed)
Description: Console and File channels write notifications to /tmp/notifications
  E2E tests validate file contents for delivery confirmation

Service: Mock Slack (Optional)
Endpoint: http://mock-slack:8080/webhook
Mock available: YES (simple HTTP server that logs webhook POSTs)
Description: If RO wants to test Slack channel, deploy mock-slack service
  Not required for basic RO→Notification contract validation
```

**Critical Note**: Notification E2E tests use **file delivery** (BR-NOT-050) for validation:
- Console channel writes to stdout (logged)
- File channel writes to /tmp/notifications/*.json (validated by E2E tests)
- Slack channel is OPTIONAL (mock-slack service available if needed)
- **No PostgreSQL/Redis required** for core Notification E2E tests

---

### **5. Health Check**

**Health Endpoint**:
```
URL: http://notification:8080/health (or specify)
Expected Status: 200
```

**Readiness Check**:
```bash
# Command to verify Notification is ready
kubectl wait --for=condition=available deployment/notification-controller -n kubernaut-system --timeout=120s
```

---

### **6. Test Scenarios for RO→Notification Contract**

**Scenario 1**: RO creates escalation notification → Notification delivers via file channel
```yaml
Description: RO creates escalation notification for timeout/failure, Notification delivers successfully
Expected RO Behavior:
  - RO creates NotificationRequest CRD with proper labels
  - RO watches status.phase for completion
  - RO transitions to NotificationSent phase when status.phase=Sent

Expected Notification Output:
  status:
    phase: "Sent"
    deliveryAttempts:
      - channel: "console"
        timestamp: "2025-12-18T10:15:00Z"
        status: "success"
      - channel: "file"
        timestamp: "2025-12-18T10:15:01Z"
        status: "success"
        outputPath: "/tmp/notifications/notif-abc123.json"
    completedAt: "2025-12-18T10:15:01Z"

Test Data (NotificationRequest CRD):
---
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: ro-e2e-escalation-001
  namespace: kubernaut-system
  labels:
    kubernaut.ai/notification-type: escalation
    kubernaut.ai/severity: high
    kubernaut.ai/environment: production
    kubernaut.ai/remediation-request: rr-timeout-001
    kubernaut.ai/component: remediation-orchestrator
spec:
  type: escalation
  priority: high
  subject: "Remediation Timeout: rr-timeout-001"
  body: "RemediationRequest rr-timeout-001 exceeded timeout threshold (5m)"
  metadata:
    remediationRequestName: "rr-timeout-001"
    remediationRequestNamespace: "production"
    timeoutDuration: "5m"
    failurePhase: "Analyzing"

Validation:
  1. CRD created with proper labels (MANDATORY per BR-NOT-065)
  2. status.phase transitions: Pending → Sending → Sent
  3. File created: /tmp/notifications/ro-e2e-escalation-001.json
  4. File contents include subject, body, metadata
```

**Scenario 2**: Manual review notification (BR-NOT-068, BR-ORCH-036)
```yaml
Description: RO creates manual-review notification with skip-reason label
Expected RO Behavior:
  - RO creates NotificationRequest with skip-reason=PreviousExecutionFailed
  - RO includes actionLinks for manual intervention
  - RO transitions to AwaitingManualReview phase

Expected Notification Output:
  status:
    phase: "Sent"
    deliveryAttempts:
      - channel: "console"
        status: "success"
      - channel: "file"
        status: "success"
        outputPath: "/tmp/notifications/notif-manual-review-001.json"

Test Data (NotificationRequest CRD):
---
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: ro-e2e-manual-review-001
  namespace: kubernaut-system
  labels:
    kubernaut.ai/notification-type: manual-review
    kubernaut.ai/severity: critical
    kubernaut.ai/environment: production
    kubernaut.ai/skip-reason: PreviousExecutionFailed  # DD-WE-004 skip-reason routing
    kubernaut.ai/remediation-request: rr-failed-exec-001
    kubernaut.ai/component: remediation-orchestrator
spec:
  type: manual-review
  priority: critical
  subject: "Manual Review Required: Workflow Execution Failed"
  body: "Previous workflow execution failed. Manual intervention required."
  actionLinks:
    - service: "kubernaut-approval"
      url: "https://kubernaut.example.com/review/rr-failed-exec-001"
      label: "Review Failure"
  metadata:
    remediationRequestName: "rr-failed-exec-001"
    skipReason: "PreviousExecutionFailed"
    previousExecutionID: "wf-exec-123"

Validation:
  1. skip-reason label present (CONDITIONAL per BR-NOT-065)
  2. actionLinks rendered in file output
  3. Severity=critical correctly propagated
```

**Scenario 3**: Approval notification (BR-NOT-068, BR-ORCH-001)
```yaml
Description: RO creates approval notification with action buttons
Expected RO Behavior:
  - RO creates NotificationRequest with type=approval
  - RO includes actionLinks for approve/reject
  - RO transitions to AwaitingApproval phase

Expected Notification Output:
  status:
    phase: "Sent"
    deliveryAttempts:
      - channel: "console"
        status: "success"
      - channel: "file"
        status: "success"
        outputPath: "/tmp/notifications/notif-approval-001.json"

Test Data (NotificationRequest CRD):
---
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: ro-e2e-approval-001
  namespace: kubernaut-system
  labels:
    kubernaut.ai/notification-type: approval
    kubernaut.ai/severity: medium
    kubernaut.ai/environment: production
    kubernaut.ai/remediation-request: rr-approval-001
    kubernaut.ai/component: remediation-orchestrator
spec:
  type: approval
  priority: medium
  subject: "Approval Required: Scale Production Deployment"
  body: "AIAnalysis recommends scaling payment-api deployment. Manual approval required."
  actionLinks:
    - service: "kubernaut-approval"
      url: "https://kubernaut.example.com/approve/rr-approval-001"
      label: "✅ Approve Workflow"
    - service: "kubernaut-rejection"
      url: "https://kubernaut.example.com/reject/rr-approval-001"
      label: "❌ Reject Workflow"
  metadata:
    remediationRequestName: "rr-approval-001"
    workflowID: "wf-scale-deployment-v1"
    confidence: "0.85"

Validation:
  1. notification-type=approval label present
  2. actionLinks included in file output (2 buttons)
  3. status.phase=Sent confirms delivery
```

**Scenario 4**: Notification sanitization (BR-NOT-058)
```yaml
Description: RO includes sensitive data in notification, Notification sanitizes before delivery
Expected RO Behavior:
  - RO creates NotificationRequest with sensitive data in body
  - RO does NOT pre-sanitize (Notification handles this)

Expected Notification Output:
  status:
    phase: "Sent"
    sanitizationApplied: true  # Indicates redaction occurred
    deliveryAttempts:
      - channel: "console"
        status: "success"
        sanitized: true

Test Data (NotificationRequest CRD):
---
apiVersion: kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: ro-e2e-sanitization-001
  namespace: kubernaut-system
  labels:
    kubernaut.ai/notification-type: escalation
    kubernaut.ai/severity: high
    kubernaut.ai/remediation-request: rr-sanitize-001
    kubernaut.ai/component: remediation-orchestrator
spec:
  type: escalation
  priority: high
  subject: "Workflow Failure: API Key Exposed"
  body: |
    Workflow execution failed with error:
    kubectl create secret generic api-key --from-literal=key=sk-1234567890abcdef
    Environment variable: AWS_SECRET_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE

Validation:
  1. File output DOES NOT contain "sk-1234567890abcdef" (redacted)
  2. File output DOES NOT contain "AKIAIOSFODNN7EXAMPLE" (redacted)
  3. File output shows "[REDACTED-SECRET-TOKEN]" or similar
  4. status.sanitizationApplied=true
```

**Scenario 5**: Notification retry on transient failure (BR-NOT-052, BR-NOT-053)
```yaml
Description: Notification delivery fails transiently, Notification retries automatically
Expected RO Behavior:
  - RO creates NotificationRequest
  - RO watches status.deliveryAttempts array for retry behavior
  - RO does NOT manually retry (Notification handles this)

Expected Notification Output:
  status:
    phase: "Sent"  # Eventually succeeds after retries
    deliveryAttempts:
      - channel: "file"
        timestamp: "2025-12-18T10:15:00Z"
        status: "failed"
        error: "filesystem temporarily unavailable"
        retryAttempt: 1
      - channel: "file"
        timestamp: "2025-12-18T10:15:02Z"  # 2s delay (exponential backoff)
        status: "failed"
        error: "filesystem temporarily unavailable"
        retryAttempt: 2
      - channel: "file"
        timestamp: "2025-12-18T10:15:06Z"  # 4s delay (exponential backoff)
        status: "success"
        retryAttempt: 3

Validation:
  1. deliveryAttempts array shows 3 attempts (2 failures + 1 success)
  2. Retry delays follow exponential backoff (2s, 4s, 8s, ...)
  3. Final status.phase=Sent (at-least-once delivery guarantee)
  4. RO does NOT need to manually retry

Note: This scenario requires simulated filesystem failure (complex E2E setup)
Priority: P2 (can be deferred to integration testing)
```

**Scenario 6**: Multiple notification priorities (BR-NOT-057)
```yaml
Description: RO creates notifications with different priorities, Notification processes all
Expected RO Behavior:
  - RO creates 3 NotificationRequests (critical, high, low)
  - RO observes all notifications delivered (no priority dropping)

Expected Notification Output:
  All 3 NotificationRequests → status.phase=Sent

Test Data:
  - NotificationRequest 1: priority=critical
  - NotificationRequest 2: priority=high
  - NotificationRequest 3: priority=low

Validation:
  1. All 3 notifications delivered successfully
  2. No priority-based dropping (V1.0 does not implement priority ordering)
  3. All 3 files created in /tmp/notifications/

Note: Priority-based ordering is NOT implemented in V1.0 (BR-NOT-057)
V1.0 delivers all notifications regardless of priority
```

**Additional Scenarios**:
```yaml
# Scenario 7: Notification lifecycle audit trail (BR-NOT-051)
Description: Every notification records complete delivery history
Validation:
  1. status.deliveryAttempts array populated
  2. Timestamps for each attempt
  3. Success/failure status per channel

# Scenario 8: Notification CRD persistence (BR-NOT-050)
Description: Notification survives controller restart (zero data loss)
Test: Create NotificationRequest → Scale controller to 0 → Scale to 1
Validation: NotificationRequest still exists with correct status

# Scenario 9: Missing mandatory labels (BR-NOT-065)
Description: RO creates NotificationRequest without mandatory labels
Expected: Notification logs warning, uses console fallback
Validation: status.phase=Sent, deliveryAttempts shows console only

# Scenario 10: Explicit channels override routing (BR-NOT-065)
Description: RO specifies spec.channels explicitly (bypass label routing)
Expected: Notification uses spec.channels, ignores routing rules
Validation: status.deliveryAttempts matches spec.channels exactly
```

---

### **7. Contact & Availability**

**Team Contact**: Notification Team Lead (via handoff documents)
**Slack Channel**: #notification-service
**Available for E2E Test Review**: ✅ **Yes**
**Preferred Review Time**: Flexible - Any time week of Dec 16-20, 2025

**Reference Documentation**:
- Service Docs: `docs/services/crd-controllers/06-notification/`
- E2E Infrastructure: `test/infrastructure/notification.go`
- E2E Tests: `test/e2e/notification/` (12 tests, 100% passing)
- Business Requirements: `docs/services/crd-controllers/06-notification/BUSINESS_REQUIREMENTS.md`
- Handoff Document: `docs/handoff/HANDOFF_NOTIFICATION_SERVICE_OWNERSHIP_TRANSFER.md`
- API Specification: `docs/services/crd-controllers/06-notification/api-specification.md`

**Integration Notes for RO Team**:
1. **Mandatory Labels**: RO MUST set routing labels (BR-NOT-065):
   - `kubernaut.ai/notification-type` (MANDATORY)
   - `kubernaut.ai/severity` (MANDATORY)
   - `kubernaut.ai/environment` (MANDATORY)
   - `kubernaut.ai/remediation-request` (MANDATORY - correlation ID)
   - `kubernaut.ai/component` (MANDATORY - "remediation-orchestrator")
   - `kubernaut.ai/skip-reason` (CONDITIONAL - when WE skips)

2. **Phase Watching**: RO should watch `status.phase` for transitions:
   - `Pending` → Notification created, queued
   - `Sending` → Delivery in progress
   - `Sent` → All channels delivered successfully
   - `PartiallySent` → Some channels succeeded, some failed
   - `Failed` → All channels failed (rare - console fallback always works)

3. **File Delivery**: E2E tests use file channel (DD-NOT-002):
   - Files written to: `/tmp/notifications/*.json`
   - Platform-specific paths (see deployment config)
   - No Slack/Email required for core E2E validation

4. **Sanitization**: Notification automatically redacts secrets (BR-NOT-058):
   - RO does NOT need to pre-sanitize notification body
   - 22 secret patterns detected and redacted
   - status.sanitizationApplied indicates redaction occurred

5. **At-Least-Once Delivery**: Notification guarantees delivery (BR-NOT-053):
   - Automatic retry with exponential backoff (BR-NOT-052)
   - RO does NOT need to manually retry failed notifications
   - Check `status.deliveryAttempts` array for retry history

**Known Limitations (V1.0)**:
- Priority-based ordering NOT implemented (BR-NOT-057 deferred)
- Email/Teams/SMS channels NOT implemented (V2.0)
- PagerDuty integration available via routing rules only
- Bulk notification (BR-ORCH-034) pending RO Day 12 implementation

**Estimated Integration Effort**: 1-2 days for RO team to integrate with Notification E2E patterns

---

## 🛠️ **Shared Infrastructure Requirements**

### **All Segments Require**:

```yaml
# PostgreSQL for audit storage
postgresql:
  image: postgres:16-alpine
  env:
    - name: POSTGRES_USER
      value: slm_user
    - name: POSTGRES_PASSWORD
      value: slm_password
    - name: POSTGRES_DB
      value: action_history
  port: 5432

# Redis for caching/deduplication
redis:
  image: redis:7-alpine
  port: 6379

# Data Storage for audit API + workflow catalog
datastorage:
  image: ghcr.io/kubernaut/datastorage:latest
  env:
    - name: POSTGRES_HOST
      value: postgresql
    - name: POSTGRES_PORT
      value: "5432"
    - name: POSTGRES_USER
      value: slm_user
    - name: POSTGRES_PASSWORD
      value: slm_password
    - name: POSTGRES_DB
      value: action_history
    - name: REDIS_HOST
      value: redis
    - name: REDIS_PORT
      value: "6379"
  ports:
    - 8080  # API
    - 9090  # Metrics
```

---

## 📅 **Timeline & Milestones**

### **Phase 1: V1.0 (December 2025)**

| Segment | Teams | Status | Target Date |
|---------|-------|--------|-------------|
| **Segment 2: RO→SP→RO** | RO + SP | 🟢 Ready | Dec 16, 2025 |
| **Segment 4: RO→WE→RO** | RO + WE | 🟢 Ready | Dec 18, 2025 |
| **Segment 5: RO→Notification→RO** | RO + Notification | 🟢 Ready | Dec 20, 2025 |

### **Phase 2: V1.1 (January 2026)**

| Segment | Teams | Status | Target Date |
|---------|-------|--------|-------------|
| **Segment 3: RO→AA→HAPI→AA→RO** | RO + AA | 🟡 Waiting for AA | Jan 10, 2026 |

### **Phase 3: V1.2 (February 2026)**

| Segment | Teams | Status | Target Date |
|---------|-------|--------|-------------|
| **Segment 1: Signal→Gateway→RO** | RO + Gateway | 🟢 Ready | Feb 1, 2026 |

---

## 🤝 **How Teams Can Contribute**

### **Option 1: Fill in Your Section** (Recommended)

1. Find your team's section above
2. Fill in the 7 required sections
3. Commit your changes
4. Notify RO team via Slack

### **Option 2: Provide Documentation Links**

If you already have E2E documentation, provide links:
```
Service: [Your Service]
E2E Documentation: [Link to your docs]
Deployment Guide: [Link to deployment instructions]
Test Scenarios: [Link to test scenarios]
```

### **Option 3: Schedule Pairing Session**

If you prefer to collaborate in real-time:
```
Service: [Your Service]
Preferred Pairing Time: [Date/Time]
Duration Needed: [30 min / 1 hour / 2 hours]
Topics to Cover: [Configuration / Test scenarios / Both]
```

---

## ❓ **FAQ**

### **Q: What if my service isn't ready yet?**

**A**: No problem! Mark your status as "Blocked" or "In Progress" and provide an estimated ready date. RO will start with ready services first.

### **Q: Can we contribute test scenarios incrementally?**

**A**: Yes! Start with 1-2 core scenarios, we can add edge cases later.

### **Q: Do we need to write the E2E tests ourselves?**

**A**: No! RO team will write the tests. We just need your configuration and scenario guidance.

### **Q: What if we have existing E2E tests?**

**A**: Great! Share links to your E2E test documentation, and we'll align our approach.

### **Q: How long will this take?**

**A**: Filling in your section: ~30-60 minutes. Reviewing RO's E2E tests: ~1-2 hours per segment.

---

## 📞 **Contact & Questions**

**RO Team Contact**: [Your Contact Info]
**Slack Channel**: #remediation-orchestrator
**Document Questions**: Comment directly in this document or reach out on Slack

---

## ✅ **Response Checklist**

Please complete by **December 20, 2025**:

- [x] **Gateway Team**: Section completed (95% ready - Dec 16, 2025)
- [x] **SignalProcessing Team**: Section completed (✅ Ready now - Dec 13, 2025)
- [x] **AIAnalysis Team**: Section completed (Ready Dec 18, 2025)
- [x] **WorkflowExecution Team**: Section completed (Ready now)
- [x] **Notification Team**: Section completed (✅ Ready now - Dec 13, 2025)

---

## 🎯 **Success Criteria**

**For RO E2E to Succeed, We Need**:
1. ✅ Deployment configuration for each service (manifests + env vars)
2. ✅ Health check commands to verify service readiness
3. ✅ 2-3 core test scenarios per segment
4. ✅ Team availability for E2E test review (1-2 hour sessions)

**What RO Team Will Deliver**:
1. ✅ Segmented E2E test implementation (5 segments)
2. ✅ Shared infrastructure setup (PostgreSQL, Redis, Data Storage)
3. ✅ CI/CD integration for fast feedback (< 15 min total)
4. ✅ Documentation for running E2E tests locally

---

**Document Status**: 🟡 **AWAITING TEAM INPUT**
**Last Updated**: December 13, 2025
**Next Review**: After team responses (December 20, 2025)

