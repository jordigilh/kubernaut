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
  recommendations:
  - action: "increase-memory-limit"
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
```

---

