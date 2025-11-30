## Reconciliation Architecture

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.4 | 2025-11-30 | Updated to DD-WORKFLOW-001 v1.4 (5 mandatory labels) | [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) v3.0, [DD-WORKFLOW-001 v1.4](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) |
> | v1.3 | 2025-11-30 | Added label detection (DetectedLabels V1.0, CustomLabels V1.0) to enriching phase | [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) v2.0 |
> | v1.2 | 2025-11-28 | Performance target <5s, graceful shutdown, retry strategy | [DD-007](../../../architecture/decisions/DD-007-kubernetes-aware-graceful-shutdown.md), [ADR-019](../../../architecture/decisions/ADR-019-holmesgpt-circuit-breaker-retry-strategy.md) |
> | v1.1 | 2025-11-27 | Service rename: RemediationProcessing → SignalProcessing | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
> | v1.1 | 2025-11-27 | Context API deprecated: Recovery context from spec.failureData | [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md) |
> | v1.1 | 2025-11-27 | Added categorizing phase | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
> | v1.0 | 2025-01-15 | Initial reconciliation phases | - |

> **📋 Design Decision: DD-001 - Recovery Context Enrichment**
> **UPDATE (2025-11-11)**: Signal Processing no longer queries Context API for historical recovery data.
> Remediation Orchestrator embeds current failure data from WorkflowExecution CRD in `spec.failureData`.
> **Status**: ✅ Approved Design | **Confidence**: 95%
> **See**: [DD-001](../../../architecture/decisions/DD-001-recovery-context-enrichment.md), [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md)

### Phase Transitions

```
pending → enriching → classifying → categorizing → completed
   ↓          ↓            ↓             ↓            ↓
(initial)  (3-5 sec)    (1-2 sec)     (1 sec)     (final)
```

**Special Case - Recovery Flow**:
```
Workflow Fails
   ↓
Remediation Orchestrator creates SignalProcessing #2 (recovery)
   ↓ (embeds failureData from WorkflowExecution CRD)
pending → enriching → classifying → categorizing → completed
             ↓
   K8s Context (FRESH!)
   Recovery Context (from spec.failureData)
```

---

### Phase Breakdown

#### 1. **pending** Phase

**Purpose**: Initial state when CRD is created, before enrichment begins

**Actions**:
- CRD created by Remediation Orchestrator or Gateway Service
- Validate signal data completeness
- Prepare for enrichment
- Check if recovery attempt (`spec.isRecoveryAttempt`)

**Duration**: < 1 second

**Transition Criteria**:
```go
if signalDataValid {
    phase = "enriching"
} else {
    phase = "failed"
    reason = "invalid_signal_data"
}
```

---

#### 2. **enriching** Phase (BR-SP-060)

**Purpose**: Enrich signal with Kubernetes context, detect labels, and (if recovery) read embedded failure context

**Actions**:

**A. ALWAYS (Initial & Recovery)**:
- Query enrichment service for Kubernetes context (BR-SP-060)
  - Pod states, resource usage, recent events
  - Current cluster metrics
- Query for business context
  - Owner team, runbook version, SLA level
  - Contact information
- Update `status.enrichmentResults`

**B. LABEL DETECTION (V1.3)**:
- **DetectedLabels (V1.0)**: Auto-detect cluster characteristics from K8s resources
  - GitOps management (ArgoCD/Flux annotations)
  - Workload protection (PDB, HPA queries)
  - Workload characteristics (StatefulSet, Helm)
  - Security posture (NetworkPolicy, PSS, ServiceMesh)
- **CustomLabels (V1.1)**: Extract user-defined labels via Rego policies
  - Query ConfigMap `signal-processing-policies` in `kubernaut-system`
  - Evaluate Rego policy with K8s context as input
  - Output: user-defined labels (e.g., `business_category`, `team`, `region`)

**C. IF RECOVERY ATTEMPT**:
- Detect `spec.isRecoveryAttempt = true`
- Read embedded failure data from `spec.failureData` (provided by Remediation Orchestrator)
- Build `status.enrichmentResults.recoveryContext` from embedded data
- Set `contextQuality = "complete"` or `"degraded"` based on data availability

**Note**: Context API is DEPRECATED per [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md). Recovery context is now embedded by Remediation Orchestrator.

**Note**: Label detection follows [HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md](HANDOFF_REQUEST_REGO_LABEL_EXTRACTION.md) v2.0. DetectedLabels are V1.0 priority (no config), CustomLabels are V1.1 (requires Rego ConfigMap).

**Timeout**: 5 seconds

**Transition Criteria**:
```go
// Normal enrichment complete
if kubernetesContextRetrieved && businessContextRetrieved {
    // If recovery, also check embedded failure data
    if spec.IsRecoveryAttempt {
        if spec.FailureData != nil {
            // Build recovery context from embedded data
            status.enrichmentResults.recoveryContext = buildFromFailureData(spec.FailureData)
        } else {
            // Use degraded mode - minimal context from spec fields
            status.enrichmentResults.recoveryContext = buildDegradedContext(spec)
        }
    }
    phase = "classifying"
} else if timeout {
    phase = "failed"
    reason = "enrichment_timeout"
}
```

**Example CRD Update (Normal Enrichment with Label Detection)**:
```yaml
status:
  phase: enriching
  enrichmentResults:
    kubernetesContext:
      namespace: "production"
      podDetails:
        name: "web-app-789"
        phase: "Running"
        restartCount: 3
      deploymentDetails:
        name: "web-app"
        replicas: 3
        readyReplicas: 2
    historicalContext:
      previousSignals: 5
      signalFrequency: 2.5
    # DetectedLabels (V1.0) - Auto-detected from K8s
    detectedLabels:
      gitOpsManaged: true
      gitOpsTool: "argocd"
      pdbProtected: true
      hpaEnabled: true
      stateful: false
      helmManaged: true
      networkIsolated: true
      podSecurityLevel: "restricted"
      serviceMesh: "istio"
    # CustomLabels (V1.1) - Extracted via Rego policies
    customLabels:
      team: "platform"
      region: "us-east-1"
      business_category: "payment-service"
    enrichmentQuality: 0.95
    enrichedAt: "2025-01-15T10:00:00Z"
```

**Example CRD Update (Recovery Enrichment)**:
```yaml
# spec.failureData embedded by Remediation Orchestrator
spec:
  isRecoveryAttempt: true
  recoveryAttemptNumber: 2
  failureData:
    workflowRef: "workflow-001"
    attemptNumber: 1
    failedStep: 3
    action: "scale-deployment"
    errorType: "timeout"
    failureReason: "Operation timed out after 5m"
    duration: "5m3s"
    failedAt: "2025-01-15T09:50:00Z"
    resourceState:
      replicas: "3"
      readyReplicas: "1"

status:
  phase: enriching
  enrichmentResults:
    # FRESH Kubernetes context (current cluster state!)
    kubernetesContext:
      namespace: "production"
      podDetails:
        name: "web-app-789"
        phase: "CrashLoopBackOff"
        restartCount: 7  # Increased from 3!

    # Recovery context from embedded failureData
    recoveryContext:
      contextQuality: "complete"
      previousFailure:
        workflowRef: "workflow-001"
        attemptNumber: 1
        failedStep: 3
        action: "scale-deployment"
        errorType: "timeout"
        failureReason: "Operation timed out after 5m"
        duration: "5m3s"
        timestamp: "2025-01-15T09:50:00Z"
      processedAt: "2025-01-15T10:00:00Z"

    enrichmentQuality: 0.95
    enrichedAt: "2025-01-15T10:00:00Z"
```

**Degraded Recovery Context Example (no failureData)**:
```yaml
status:
  phase: enriching
  enrichmentResults:
    # Kubernetes context still FRESH (success!)
    kubernetesContext: {...}

    # Recovery context degraded (no embedded failureData)
    recoveryContext:
      contextQuality: "degraded"
      previousFailure:
        workflowRef: "workflow-001"  # From spec.failedWorkflowRef
        failedStep: 3                 # From spec.failedStep
        failureReason: "timeout"      # From spec.failureReason
      processedAt: "2025-01-15T10:00:00Z"

    enrichmentQuality: 0.8
```

---

#### 3. **classifying** Phase (BR-SP-051, BR-SP-052, BR-SP-053)

**Purpose**: Classify environment tier

**Actions**:
- Detect environment tier from namespace labels (BR-SP-051)
- Validate environment classification (BR-SP-052)
- Load environment-specific configuration (BR-SP-053)
- Determine business criticality level

**Duration**: 1-2 seconds

**Transition Criteria**:
```go
if environmentClassified && configurationLoaded {
    phase = "categorizing"
} else {
    phase = "failed"
    reason = "classification_failed"
}
```

**Example CRD Update**:
```yaml
status:
  phase: classifying
  environmentClassification:
    environment: "production"
    confidence: 0.95
    businessCriticality: "critical"
    slaRequirement: "5m"
  enrichmentResults:
    # ... (enrichment data from previous phase)
```

---

#### 4. **categorizing** Phase (BR-SP-070 to BR-SP-075, DD-CATEGORIZATION-001)

**Purpose**: Assign priority based on enriched Kubernetes context

**Actions**:
- Calculate priority score based on environment classification
- Consider namespace labels and annotations
- Factor in workload type and business criticality
- Set final priority (P0-P3)
- Prepare routing decision for downstream services

**Duration**: 1 second

**Added per [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)**: Gateway sets placeholder priority; Signal Processing has richer K8s context for accurate categorization.

**Transition Criteria**:
```go
if priorityAssigned {
    phase = "completed"
} else {
    phase = "failed"
    reason = "categorization_failed"
}
```

**Example CRD Update**:
```yaml
status:
  phase: categorizing
  categorization:
    priority: "P0"
    priorityScore: 95
    categorizationFactors:
    - factor: "environment"
      value: "production"
      weight: 0.4
      contribution: 40
    - factor: "namespace_labels"
      value: "tier=critical"
      weight: 0.3
      contribution: 30
    - factor: "workload_type"
      value: "deployment"
      weight: 0.2
      contribution: 15
    - factor: "signal_severity"
      value: "critical"
      weight: 0.1
      contribution: 10
    categorizationSource: "enriched_context"
    categorizationTime: "2025-01-15T10:00:03Z"
  routingDecision:
    nextService: "ai-analysis"
    routingKey: "signal-fingerprint-123"
    priority: 9
```

---

#### 5. **completed** Phase

**Purpose**: Terminal success state - enrichment complete, ready for AIAnalysis

**Actions**:
- Set `status.phase = "completed"`
- Set `status.processingTime`
- Emit event for RemediationRequest controller
- RemediationRequest controller watches this status change
- RemediationRequest creates AIAnalysis CRD with enrichment data

**Post-Completion Flow**:
```
SignalProcessing.status.phase = "completed"
   ↓ (watch event)
Remediation Orchestrator detects completion
   ↓
Remediation Orchestrator creates AIAnalysis CRD
   ↓
Copies status.enrichmentResults → AIAnalysis.spec.enrichmentData
   ↓
AIAnalysis has ALL contexts (K8s + recovery if applicable)
```

**Example Final Status**:
```yaml
status:
  phase: completed
  processingTime: "4.2s"
  enrichmentResults:
    kubernetesContext: {...}
    historicalContext: {...}
    recoveryContext: {...}  # Only present if isRecoveryAttempt = true
    enrichmentQuality: 0.95
    enrichedAt: "2025-01-15T10:00:00Z"
  environmentClassification:
    environment: "production"
    confidence: 0.95
    businessCriticality: "critical"
    slaRequirement: "5m"
  categorization:
    priority: "P0"
    priorityScore: 95
    categorizationSource: "enriched_context"
  routingDecision:
    nextService: "ai-analysis"
    routingKey: "signal-fingerprint-123"
    priority: 9
```

---

### Error Handling

#### **failed** Phase

**Purpose**: Terminal failure state when processing cannot complete

**Causes**:
- Invalid signal data (missing required fields)
- Enrichment timeout (> 5 seconds)
- Classification failure
- Categorization failure
- Unrecoverable enrichment service error

**Actions**:
- Set `status.phase = "failed"`
- Set `status.failureReason`
- Emit warning event
- Remediation Orchestrator escalates to manual review

**Example**:
```yaml
status:
  phase: failed
  failureReason: "enrichment_service_unavailable"
  processingTime: "30s"
```

---

### Recovery Enrichment Flow

#### Comparison: Initial vs Recovery Enrichment

| Aspect | Initial Enrichment | Recovery Enrichment |
|--------|-------------------|---------------------|
| **K8s Context** | Current cluster state | **FRESH** current cluster state |
| **Business Context** | Current ownership/runbooks | **FRESH** current ownership/runbooks |
| **Recovery Context** | N/A | **From spec.failureData** (embedded by Remediation Orchestrator) |
| **Context API Call** | No | **No** (deprecated per DD-CONTEXT-006) |
| **Graceful Degradation** | N/A | Fallback from `failedWorkflowRef` fields |
| **ContextQuality** | "complete" or "partial" | "complete" or "degraded" |

#### Recovery Enrichment Benefits

1. ✅ **Fresh K8s Data**: Recovery sees CURRENT cluster state (not stale from initial attempt)
2. ✅ **Fresh Business Data**: Runbooks/ownership may have changed
3. ✅ **Historical Context**: AI knows what already failed (from embedded data)
4. ✅ **No External Dependency**: No Context API call needed (simplified architecture)
5. ✅ **Immutable Audit Trail**: Each SignalProcessing CRD contains complete snapshot
6. ✅ **Pattern Reuse**: Recovery uses same enrichment flow as initial

---

### Performance Characteristics

**Target Timing** (Normal Enrichment):
- `pending` → `enriching`: < 100ms
- `enriching` (K8s context): 3 seconds
- `classifying`: 1-2 seconds
- `categorizing`: 1 second
- **Total**: 4-6 seconds

**Target Timing** (Recovery Enrichment):
- Same as initial (no additional API calls needed)
- Recovery context read from embedded data (< 100ms)
- **Total**: 4-7 seconds

**Timeout Configuration**:
```yaml
apiVersion: signalprocessing.kubernaut.io/v1
kind: SignalProcessing
metadata:
  annotations:
    kubernaut.io/enrichment-timeout: "5s"
    kubernaut.io/classification-timeout: "2s"
    kubernaut.io/categorization-timeout: "1s"
```

---

### Observability

**Metrics**:
```go
// Enrichment duration by type
kubernaut_signal_processing_enrichment_duration_seconds{type="kubernetes|historical|recovery"}

// Context quality distribution
kubernaut_signal_processing_context_quality_total{quality="complete|partial|degraded"}

// Phase duration
kubernaut_signal_processing_phase_duration_seconds{phase="enriching|classifying|categorizing"}

// Categorization source
kubernaut_signal_processing_categorization_source_total{source="enriched_context|fallback_labels|default"}

// Priority distribution
kubernaut_signal_processing_priority_total{priority="P0|P1|P2|P3"}
```

**Log Patterns**:
```
# Normal enrichment
INFO  Enriching signal processing  isRecovery=false  fingerprint=signal-123
INFO  Kubernetes context retrieved  duration=1.2s  quality=complete
INFO  Business context retrieved  duration=0.8s
INFO  Environment classified  environment=production  confidence=0.95
INFO  Priority categorized  priority=P0  score=95  source=enriched_context
INFO  Signal processing complete  totalDuration=4.2s  phase=completed

# Recovery enrichment
INFO  Enriching signal processing (RECOVERY)  isRecovery=true  attemptNumber=2
INFO  Kubernetes context retrieved (FRESH!)  duration=1.3s  restartCountChange=+4
INFO  Reading embedded failure data from spec.failureData
INFO  Recovery context built  contextQuality=complete  failedStep=3  errorType=timeout
INFO  Environment classified  environment=production  confidence=0.95
INFO  Priority categorized  priority=P0  score=95  source=enriched_context
INFO  Signal processing complete (RECOVERY)  totalDuration=4.5s  phase=completed
```

---

### Testing Scenarios

**Unit Tests**:
- Normal enrichment (K8s + business context)
- Recovery enrichment (K8s + embedded failure data)
- Recovery enrichment with missing failureData → degraded context
- Enrichment timeout handling
- Classification logic
- Categorization logic with various factors

**Integration Tests**:
- End-to-end enrichment with real enrichment service
- End-to-end recovery enrichment with embedded failure data
- Fresh context validation (K8s state updated between attempts)
- Categorization accuracy testing

---

### Related Documentation

- **Controller Implementation**: [`controller-implementation.md`](./controller-implementation.md)
- **CRD Schema**: [`crd-schema.md`](./crd-schema.md)
- **Context API Deprecation**: [`DD-CONTEXT-006`](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md)
- **Categorization Consolidation**: [`DD-CATEGORIZATION-001`](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md)
- **Business Requirements**: [`docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`](../../../requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md)
