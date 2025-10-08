## Reconciliation Architecture

> **📋 Design Decision: DD-001 - Alternative 2**
> **Pattern**: RemediationProcessing enriches ALL contexts (monitoring + business + recovery)
> **Status**: ✅ Approved Design | **Confidence**: 95%
> **See**: [DD-001](../../../architecture/DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2)

**Reference**: Alternative 2 - RemediationProcessing Enrichment Pattern
**See**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) (Version 1.2)

### Phase Transitions

```
pending → enriching → classifying → completed
   ↓          ↓            ↓            ↓
(initial)  (3-5 sec)    (1-2 sec)   (final)
```

**Special Case - Recovery Flow (Alternative 2)**:
```
Workflow Fails
   ↓
Remediation Orchestrator creates RemediationProcessing #2 (recovery)
   ↓
pending → enriching (ALL contexts!) → classifying → completed
             ↓
   Monitoring (FRESH!)
   Business (FRESH!)
   Recovery (Context API - FRESH!)
```

---

### Phase Breakdown

#### 1. **pending** Phase

**Purpose**: Initial state when CRD is created, before enrichment begins

**Actions**:
- CRD created by Remediation Orchestrator or Gateway Service
- Validate alert data completeness
- Prepare for enrichment
- Check if recovery attempt (`spec.isRecoveryAttempt`)

**Duration**: < 1 second

**Transition Criteria**:
```go
if alertDataValid {
    phase = "enriching"
} else {
    phase = "failed"
    reason = "invalid_alert_data"
}
```

---

#### 2. **enriching** Phase (BR-AP-060, BR-WF-RECOVERY-011)

**Purpose**: Enrich alert with Kubernetes context, business metadata, and (if recovery) historical failure context

**Actions**:

**A. ALWAYS (Initial & Recovery)**:
- Query Context Service for monitoring context (BR-AP-060)
  - Pod states, resource usage, recent events
  - Current cluster metrics
- Query Context Service for business context
  - Owner team, runbook version, SLA level
  - Contact information
- Update `status.enrichmentResults`

**B. IF RECOVERY ATTEMPT (Alternative 2)**:
- Detect `spec.isRecoveryAttempt = true`
- Query Context API for recovery context (BR-WF-RECOVERY-011)
  - Previous workflow failures
  - Related alerts and correlations
  - Historical patterns
  - Successful strategies
- Graceful degradation if Context API unavailable
  - Build fallback context from `spec.failedWorkflowRef`
  - Set `contextQuality = "degraded"`
- Update `status.enrichmentResults.recoveryContext`

**Timeout**: 5 seconds (3s for monitoring/business, 2s for recovery context)

**Transition Criteria**:
```go
// Normal enrichment complete
if monitoringContextRetrieved && businessContextRetrieved {
    // If recovery, also check recovery context
    if spec.IsRecoveryAttempt {
        if recoveryContextRetrieved || fallbackContextCreated {
            phase = "classifying"
        } else {
            // Context API totally failed, use degraded mode
            status.enrichmentResults.contextQuality = "degraded"
            phase = "classifying"  // Continue anyway (graceful degradation)
        }
    } else {
        phase = "classifying"
    }
} else if timeout {
    phase = "failed"
    reason = "enrichment_timeout"
}
```

**Example CRD Update (Normal Enrichment)**:
```yaml
status:
  phase: enriching
  enrichmentResults:
    monitoringContext:
      clusterMetrics:
        cpu: "75%"
        memory: "82%"
      podStates:
      - name: "web-app-789"
        phase: "Running"
        restartCount: 3
    businessContext:
      ownerTeam: "platform-team"
      runbookVersion: "v2.1"
      slaLevel: "P1"
    contextQuality: "complete"
    enrichedAt: "2025-01-15T10:00:00Z"
```

**Example CRD Update (Recovery Enrichment - Alternative 2)**:
```yaml
status:
  phase: enriching
  enrichmentResults:
    # FRESH monitoring context (current cluster state!)
    monitoringContext:
      clusterMetrics:
        cpu: "82%"  # May have changed since initial attempt!
        memory: "95%"
      podStates:
      - name: "web-app-789"
        phase: "CrashLoopBackOff"
        restartCount: 7  # Increased from 3!

    # FRESH business context (may have been updated!)
    businessContext:
      ownerTeam: "platform-team"
      runbookVersion: "v2.2"  # Runbook updated between attempts!
      slaLevel: "P0"  # Escalated to P0!

    # Recovery context from Context API (Alternative 2)
    recoveryContext:
      contextQuality: "complete"
      previousFailures:
      - workflowRef: "workflow-001"
        attemptNumber: 1
        failedStep: 3
        action: "scale-deployment"
        errorType: "timeout"
        failureReason: "Operation timed out after 5m"
        timestamp: "2025-01-15T09:50:00Z"
      relatedAlerts:
      - alertFingerprint: "related-alert-123"
        alertName: "HighMemoryUsage"
        correlation: 0.85
      historicalPatterns:
      - pattern: "scale_timeout_high_memory"
        occurrences: 12
        successRate: 0.73
      successfulStrategies:
      - strategy: "force-delete-pods-then-scale"
        confidence: 0.88
        successCount: 8
      retrievedAt: "2025-01-15T10:00:00Z"

    contextQuality: "complete"
    enrichedAt: "2025-01-15T10:00:00Z"  # All contexts same timestamp!
```

**Graceful Degradation Example (Context API Failed)**:
```yaml
status:
  phase: enriching
  enrichmentResults:
    # Monitoring/business still FRESH (success!)
    monitoringContext: {...}
    businessContext: {...}

    # Recovery context degraded (Context API failed)
    recoveryContext:
      contextQuality: "degraded"
      previousFailures:
      - workflowRef: "workflow-001"  # Extracted from spec.failedWorkflowRef
        failedStep: 3                 # Extracted from spec.failedStep
        failureReason: "timeout"      # Extracted from spec.failureReason
      retrievedAt: "2025-01-15T10:00:00Z"

    contextQuality: "degraded"
```

---

#### 3. **classifying** Phase (BR-AP-051, BR-AP-052, BR-AP-053)

**Purpose**: Classify environment tier and finalize enrichment

**Actions**:
- Detect environment tier from namespace labels (BR-AP-051)
- Validate environment classification (BR-AP-052)
- Load environment-specific configuration (BR-AP-053)
- Finalize enrichment results
- Prepare for AIAnalysis creation by Remediation Orchestrator

**Duration**: 1-2 seconds

**Transition Criteria**:
```go
if environmentClassified && configurationLoaded {
    phase = "completed"
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
    tier: "production"
    confidence: 0.95
    source: "namespace-labels"
  enrichmentResults:
    # ... (enrichment data from previous phase)
```

---

#### 4. **completed** Phase

**Purpose**: Terminal success state - enrichment complete, ready for AIAnalysis

**Actions**:
- Set `status.phase = "completed"`
- Set `status.completionTime`
- Emit event for RemediationRequest controller
- RemediationRequest controller watches this status change
- RemediationRequest creates AIAnalysis CRD with enrichment data

**Post-Completion Flow (Alternative 2)**:
```
RemediationProcessing.status.phase = "completed"
   ↓ (watch event)
Remediation Orchestrator detects completion
   ↓
Remediation Orchestrator creates AIAnalysis CRD
   ↓
Copies status.enrichmentResults → AIAnalysis.spec.enrichmentData
   ↓
AIAnalysis has ALL contexts (monitoring + business + recovery)
```

**Example Final Status**:
```yaml
status:
  phase: completed
  completionTime: "2025-01-15T10:00:05Z"
  enrichmentResults:
    monitoringContext: {...}
    businessContext: {...}
    recoveryContext: {...}  # Only present if isRecoveryAttempt = true
    contextQuality: "complete"
    enrichedAt: "2025-01-15T10:00:00Z"
  environmentClassification:
    tier: "production"
```

---

### Error Handling

#### **failed** Phase

**Purpose**: Terminal failure state when enrichment cannot complete

**Causes**:
- Invalid alert data (missing required fields)
- Enrichment timeout (> 5 seconds)
- Classification failure
- Unrecoverable Context Service error

**Actions**:
- Set `status.phase = "failed"`
- Set `status.failureReason`
- Emit warning event
- Remediation Orchestrator escalates to manual review

**Example**:
```yaml
status:
  phase: failed
  failureReason: "context_service_unavailable"
  completionTime: "2025-01-15T10:00:30Z"
```

---

### Recovery Enrichment Flow (Alternative 2)

#### Comparison: Initial vs Recovery Enrichment

| Aspect | Initial Enrichment | Recovery Enrichment (Alternative 2) |
|--------|-------------------|-------------------------------------|
| **Monitoring Context** | Current cluster state | **FRESH** current cluster state |
| **Business Context** | Current ownership/runbooks | **FRESH** current ownership/runbooks |
| **Recovery Context** | N/A | **NEW** - Historical failures from Context API |
| **Context API Call** | No | **Yes** - `/context/remediation/{id}` |
| **Graceful Degradation** | N/A | Fallback from `failedWorkflowRef` |
| **ContextQuality** | "complete" or "partial" | "complete", "partial", or "degraded" |
| **Temporal Consistency** | Single timestamp | **All contexts same timestamp** ✅ |

#### Recovery Enrichment Benefits

1. ✅ **Fresh Monitoring Data**: Recovery sees CURRENT cluster state (not 10min old)
2. ✅ **Fresh Business Data**: Runbooks/ownership may have changed
3. ✅ **Historical Context**: AI knows what already failed
4. ✅ **Temporal Consistency**: All contexts captured at same moment
5. ✅ **Immutable Audit Trail**: Each RemediationProcessing CRD is separate
6. ✅ **Pattern Reuse**: Recovery uses same enrichment flow as initial

---

### Performance Characteristics

**Target Timing** (Normal Enrichment):
- `pending` → `enriching`: < 100ms
- `enriching` (monitoring + business): 3 seconds
- `enriching` (+ recovery context): +2 seconds = 5 seconds total
- `classifying`: 1-2 seconds
- **Total**: 4-7 seconds (6-9 seconds for recovery)

**Timeout Configuration**:
```yaml
apiVersion: remediationprocessing.kubernaut.io/v1
kind: RemediationProcessing
metadata:
  annotations:
    kubernaut.io/enrichment-timeout: "5s"
    kubernaut.io/classification-timeout: "2s"
```

---

### Observability

**Metrics**:
```go
// Enrichment duration by type
kubernaut_remediation_processing_enrichment_duration_seconds{type="monitoring|business|recovery"}

// Context quality distribution
kubernaut_remediation_processing_context_quality_total{quality="complete|partial|degraded"}

// Recovery enrichment success rate
kubernaut_remediation_processing_recovery_enrichment_success_total{outcome="success|degraded|failed"}

// Context API latency (recovery only)
kubernaut_remediation_processing_context_api_duration_seconds

// Fresh context benefit (recovery only)
kubernaut_remediation_processing_fresh_context_age_seconds{type="monitoring|business"}
```

**Log Patterns**:
```
# Normal enrichment
INFO  Enriching alert processing  isRecovery=false  attemptNumber=0
INFO  Monitoring context retrieved  duration=1.2s  quality=complete
INFO  Business context retrieved  duration=0.8s
INFO  Enrichment complete  totalDuration=2.1s  contextQuality=complete

# Recovery enrichment (Alternative 2)
INFO  Enriching alert processing (RECOVERY)  isRecovery=true  attemptNumber=2
INFO  Monitoring context retrieved (FRESH!)  duration=1.3s  cpuChange=+7%  memoryChange=+13%
INFO  Business context retrieved (FRESH!)  duration=0.9s  runbookUpdated=true
INFO  Querying Context API for recovery context  remediationRequestID=rr-001
INFO  Recovery context retrieved  duration=1.8s  previousFailures=1  patterns=5  quality=complete
INFO  Enrichment complete (ALL CONTEXTS)  totalDuration=4.2s  contextQuality=complete
```

---

### Testing Scenarios

**Unit Tests**:
- Normal enrichment (monitoring + business)
- Recovery enrichment (monitoring + business + recovery)
- Context API success (recovery)
- Context API failure → graceful degradation (recovery)
- Enrichment timeout handling
- Classification logic

**Integration Tests**:
- End-to-end enrichment with real Context Service
- End-to-end recovery enrichment with real Context API
- Fresh context validation (monitoring/business updated between attempts)
- Temporal consistency verification (all contexts same timestamp)

---

### Related Documentation

- **Controller Implementation**: [`controller-implementation.md`](./controller-implementation.md)
- **CRD Schema**: [`crd-schema.md`](./crd-schema.md)
- **Alternative 2 Architecture**: [`docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)
- **Business Requirements**: [`docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`](../../../requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md) (BR-WF-RECOVERY-011)
- **Context API Specification**: [`docs/services/stateless/context-api/api-specification.md`](../../stateless/context-api/api-specification.md)

