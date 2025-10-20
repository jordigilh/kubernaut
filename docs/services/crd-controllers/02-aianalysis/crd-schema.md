## CRD Schema

**Full Schema**: See [docs/design/CRD/03_AI_ANALYSIS_CRD.md](../../design/CRD/03_AI_ANALYSIS_CRD.md)

**Note**: The examples below show the conceptual structure. The authoritative OpenAPI v3 schema is defined in `03_AI_ANALYSIS_CRD.md`.

### Spec Fields

```yaml
spec:
  # Parent RemediationRequest reference (for audit/lineage only)
  alertRemediationRef:
    name: remediation-abc12345
    namespace: kubernaut-system

  # SELF-CONTAINED analysis request (complete data snapshot from RemediationProcessing)
  # No need to read RemediationProcessing - all enriched data copied here at creation
  analysisRequest:
    alertContext:
      # Basic alert identifiers
      fingerprint: "abc123def456"
      severity: critical
      environment: production
      businessPriority: p0

      # COMPLETE enriched payload (snapshot from RemediationProcessing.status)
      enrichedPayload:
        originalAlert:
          labels:
            alertname: PodOOMKilled
            namespace: production
            pod: web-app-789
          annotations:
            summary: "Pod killed due to OOM"
            description: "Memory limit exceeded"

        kubernetesContext:
          podDetails:
            name: web-app-789
            namespace: production
            containers:
            - name: app
              memoryLimit: "512Mi"
              memoryUsage: "498Mi"
          deploymentDetails:
            name: web-app
            replicas: 3
          nodeDetails:
            name: node-1
            capacity: {...}

        monitoringContext:
          relatedAlerts: [...]
          metrics: [...]
          logs: [...]

        businessContext:
          serviceOwner: "platform-team"
          criticality: "high"
          sla: "99.9%"

    analysisTypes:
    - investigation
    - root-cause
    - recovery-analysis
    - recommendation-generation

    investigationScope:
      timeWindow: "24h"
      resourceScope:
      - kind: Pod
        namespace: production
      correlationDepth: detailed
      includeHistoricalPatterns: true

  # Note: HolmesGPT toolset configuration is managed by Dynamic Toolset Service
  # (standalone service, see DYNAMIC_TOOLSET_CONFIGURATION_ARCHITECTURE.md)
  # Toolsets are system-wide configuration, not per-investigation
  # HolmesGPT-API queries Dynamic Toolset Service for available toolsets
```

### Status Fields

```yaml
status:
  phase: recommending

  # Phase transition tracking
  phaseTransitions:
    investigating: "2025-01-15T10:00:00Z"
    analyzing: "2025-01-15T10:15:00Z"
    recommending: "2025-01-15T10:30:00Z"

  # Investigation results (Phase 1)
  investigationResult:
    rootCauseHypotheses:
    - hypothesis: "Pod memory limit too low"
      confidence: 0.85
      evidence:
      - "OOMKilled events in pod history"
      - "Memory usage consistently near 95%"
    correlatedAlerts:
    - fingerprint: "abc123def456"
      timestamp: "2025-01-15T10:30:00Z"
    investigationReport: "..."
    contextualAnalysis: "..."

  # Analysis results (Phase 2)
  analysisResult:
    analysisTypes:
    - type: diagnostic
      result: "Container memory limit insufficient"
      confidence: 0.9
    - type: prescriptive
      result: "Increase memory limit to 1Gi"
      confidence: 0.88
    validationStatus:
      completeness: true
      hallucinationDetected: false
      confidenceThresholdMet: true

  # Recommendations (Phase 3)
  # Business Requirement: BR-HOLMES-031, BR-HOLMES-032, BR-HOLMES-033
  recommendations:
  - id: "rec-001"  # ✅ NEW: Unique identifier for dependency references
    action: "increase-memory-limit"
    targetResource:
      kind: Deployment
      name: web-app
      namespace: production
    parameters:
      newMemoryLimit: "1Gi"
    effectivenessProbability: 0.92
    historicalSuccessRate: 0.88
    riskLevel: low
    explanation: "Historical data shows 88% success rate"
    supportingEvidence:
    - "15 similar cases resolved by memory increase"
    constraints:
      environmentAllowed: [production, staging]
      rbacRequired: ["apps/deployments:update"]
    dependencies: []  # ✅ NEW: Empty array = no dependencies, can execute immediately

  # Workflow reference (Phase 4)
  workflowExecutionRef:
    name: aianalysis-abc123-workflow-1
    namespace: kubernaut-system

  # Observability
  conditions:
  - type: InvestigationComplete
    status: "True"
    reason: RootCauseIdentified
  - type: AnalysisValidated
    status: "True"
    reason: ConfidenceThresholdMet
  - type: RecommendationsGenerated
    status: "True"
    reason: TopRecommendationSelected

  # V1.0 Approval Notification Integration (ADR-018)
  # ================================================

  # Approval Context (BR-AI-059) - Rich context for RemediationOrchestrator notifications
  approvalContext:
    reason: string                           # "Medium confidence (72.5%) - requires human review"
    confidenceScore: float64                 # 72.5
    confidenceLevel: string                  # "medium" (low/medium/high)
    investigationSummary: string             # "Memory leak in payment processing..."
    evidenceCollected: []string              # ["Linear memory growth 50MB/hour per pod", ...]
    recommendedActions: []RecommendedAction  # [{action: "collect_diagnostics", rationale: "..."}, ...]
    alternativesConsidered: []Alternative    # [{approach: "Wait and monitor", prosCons: "..."}, ...]
    whyApprovalRequired: string              # "Historical pattern requires validation..."

  # Approval Decision Tracking (BR-AI-060) - Audit trail for operator decisions
  approvalStatus: string                     # "approved" | "rejected" | "pending"
  approvedBy: string                         # "ops-engineer@company.com"
  approvalTime: *metav1.Time                 # When decision was made
  approvalDuration: string                   # "2m15s" (time from request to decision)
  approvalMethod: string                     # "console" | "slack" | "api"
  approvalJustification: string              # "Approved - low risk change in staging"
  rejectedBy: string                         # (populated if rejected)
  rejectionReason: string                    # (populated if rejected)
```

### Multi-Step Workflow Example with Dependencies

**Business Requirements**: BR-HOLMES-031, BR-HOLMES-032, BR-HOLMES-033

This example demonstrates how HolmesGPT specifies step dependencies to enable parallel execution optimization:

```yaml
status:
  # Multi-step recommendations with dependency graph
  recommendations:
  - id: "rec-001"
    action: "scale-deployment"
    targetResource:
      kind: Deployment
      name: payment-api
      namespace: production
    parameters:
      replicas: 5
    effectivenessProbability: 0.92
    dependencies: []  # No dependencies - executes first

  - id: "rec-002"
    action: "restart-pods"
    targetResource:
      kind: Pod
      namespace: production
      labelSelector: "app=payment-api"
    parameters:
      gracePeriodSeconds: 30
    effectivenessProbability: 0.88
    dependencies: ["rec-001"]  # Depends on scale completion

  - id: "rec-003"
    action: "increase-memory-limit"
    targetResource:
      kind: Deployment
      name: payment-api
      namespace: production
    parameters:
      newMemoryLimit: "2Gi"
    effectivenessProbability: 0.85
    dependencies: ["rec-001"]  # Also depends on scale completion

  # rec-002 and rec-003 can execute IN PARALLEL after rec-001 completes
  # Both depend only on rec-001, no dependency between them

  - id: "rec-004"
    action: "verify-deployment"
    targetResource:
      kind: Deployment
      name: payment-api
      namespace: production
    parameters:
      healthCheckEndpoint: "/health"
    effectivenessProbability: 0.95
    dependencies: ["rec-002", "rec-003"]  # Waits for BOTH to complete

  # Execution order (determined by WorkflowExecution Controller):
  # Batch 1: rec-001 (sequential)
  # Batch 2: rec-002, rec-003 (parallel - both depend only on rec-001)
  # Batch 3: rec-004 (sequential - waits for rec-002 AND rec-003)
```

**Dependency Validation** (BR-AI-051, BR-AI-052, BR-AI-053):
- All dependency IDs reference valid recommendations in the list ✅
- No circular dependencies (acyclic graph) ✅
- rec-002 and rec-003 identified as parallelizable ✅

---

