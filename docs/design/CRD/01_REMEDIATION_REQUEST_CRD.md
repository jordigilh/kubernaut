# RemediationRequest CRD Design Document

**Document Version**: 1.0
**Date**: January 2025
**Status**: **REFERENCE ONLY** - Superseded by CRD_SCHEMAS.md
**CRD Type**: Central Coordination Controller
**Priority**: **CRITICAL** - Core orchestration component

---

## ⚠️ DEPRECATION NOTICES

### **1. Authoritative Source**

This document is **REFERENCE ONLY**. The authoritative CRD definitions are in:
- **[CRD_SCHEMAS.md](../../architecture/CRD_SCHEMAS.md)** - Authoritative OpenAPI v3 schemas
- **[V1 Source of Truth Hierarchy](../../V1_SOURCE_OF_TRUTH_HIERARCHY.md)** - Documentation authority

### **2. Naming Convention**

This document uses **deprecated "Alert" prefix naming**:
- `AlertRemediation` CRD → **`RemediationOrchestration`** (current name)
- `AlertProcessing` → **`RemediationProcessing`** (current name)

**Why Deprecated**: Kubernaut processes multiple signal types (alerts, events, alarms), not just alerts.

**Migration**: [ADR-015: Alert to Signal Naming Migration](../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)

---

## 🎯 **PURPOSE AND SCOPE**

### **Business Purpose**
The AlertRemediation CRD serves as the central coordination point for all alert processing activities in Kubernaut. It orchestrates the entire remediation lifecycle from alert reception through final execution, providing state persistence, progress tracking, and cross-service coordination.

### **Scope**
- **Central State Management**: Master record for all alert remediation activities
- **Cross-Service Coordination**: Orchestrates AlertProcessing, AIAnalysis, WorkflowExecution, and ~~KubernetesExecution~~ (DEPRECATED - ADR-025) CRDs
- **Duplicate Detection**: Implements BR-WH-008 request deduplication for identical alerts
- **Progress Tracking**: Real-time visibility into remediation progress across all services
- **Timeout Management**: Configurable timeouts with automatic escalation
- **Audit Trail**: Complete lifecycle tracking for compliance and debugging

---

## 📋 **BUSINESS REQUIREMENTS ADDRESSED**

### **Primary Business Requirements**
- **BR-PA-001**: Alert reception and processing coordination with 99.9% availability
- **BR-PA-003**: Process alerts within 5 seconds (coordination and tracking)
- **BR-PA-010**: Support dry-run mode for testing decisions without executing actions
- **BR-WH-008**: Request deduplication for identical alerts
- **BR-SP-021**: Alert lifecycle state tracking throughout processing
- **BR-ALERT-003**: Alert suppression to reduce noise through duplicate handling
- **BR-ALERT-005**: Alert correlation and grouping under single remediation context

### **Cross-Service Requirements**
- **Cross-service coordination**: Audit trails and compliance tracking
- **State persistence**: Survive pod restarts and service failures
- **Network resilience**: Automatic reconciliation when services recover
- **Cloud-native integration**: Leverage Kubernetes-native patterns

---

## 🏗️ **CRD SPECIFICATION**

### **Custom Resource Definition**

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: alertremediations.kubernaut.io
  annotations:
    controller-gen.kubebuilder.io/version: v0.13.0
    kubernaut.io/description: "Central coordination CRD for alert remediation lifecycle"
    kubernaut.io/business-requirements: "BR-PA-001,BR-PA-003,BR-WH-008,BR-SP-021"
spec:
  group: kubernaut.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            required:
            - alertFingerprint
            - severity
            - environment
            - alertPayload
            properties:
              # Core Alert Identity
              alertFingerprint:
                type: string
                description: "Unique fingerprint for alert deduplication (BR-WH-008)"
                pattern: "^[a-f0-9]{12,64}$"
              severity:
                type: string
                enum: [critical, warning, info]
                description: "Alert severity level for priority handling"
              environment:
                type: string
                enum: [production, staging, development, testing]
                description: "Target environment for environment-specific processing"

              # Alert Data
              alertPayload:
                type: object
                description: "Complete alert payload from Prometheus"
                properties:
                  alertname:
                    type: string
                  labels:
                    type: object
                    additionalProperties:
                      type: string
                  annotations:
                    type: object
                    additionalProperties:
                      type: string
                  startsAt:
                    type: string
                    format: date-time
                  endsAt:
                    type: string
                    format: date-time
                  generatorURL:
                    type: string
                required:
                - alertname
                - labels

              # Timing and Lifecycle
              createdAt:
                type: string
                format: date-time
                description: "When the remediation was initiated"
              timeoutAt:
                type: string
                format: date-time
                description: "When the remediation should timeout"
              maxDuration:
                type: string
                pattern: "^[0-9]+(s|m|h)$"
                default: "1h"
                description: "Maximum allowed remediation time (e.g., '1h', '30m')"

              # Configuration
              remediationConfig:
                type: object
                properties:
                  dryRun:
                    type: boolean
                    default: false
                    description: "Execute in dry-run mode (BR-PA-010)"
                  requireApproval:
                    type: boolean
                    default: false
                    description: "Require manual approval before execution"
                  escalationChannels:
                    type: array
                    items:
                      type: string
                    description: "Notification channels for escalation"
                  retryPolicy:
                    type: object
                    properties:
                      maxRetries:
                        type: integer
                        minimum: 0
                        maximum: 5
                        default: 3
                      backoffMultiplier:
                        type: number
                        minimum: 1.0
                        default: 2.0

          status:
            type: object
            properties:
              # Overall State
              phase:
                type: string
                enum: [processing, completed, failed, timeout, cancelled]
                description: "Current remediation phase"
              overallProgress:
                type: integer
                minimum: 0
                maximum: 100
                description: "Overall progress percentage"

              # Service Status Tracking
              serviceStatuses:
                type: object
                properties:
                  alertprocessor:
                    type: object
                    properties:
                      phase:
                        type: string
                        enum: [pending, processing, completed, failed]
                      startTime:
                        type: string
                        format: date-time
                      completionTime:
                        type: string
                        format: date-time
                      environment:
                        type: string
                      businessPriority:
                        type: string
                      error:
                        type: string
                  aianalysis:
                    type: object
                    properties:
                      phase:
                        type: string
                        enum: [pending, processing, completed, failed]
                      startTime:
                        type: string
                        format: date-time
                      completionTime:
                        type: string
                        format: date-time
                      confidence:
                        type: number
                        minimum: 0.0
                        maximum: 1.0
                      recommendations:
                        type: integer
                        minimum: 0
                      error:
                        type: string
                  workflow:
                    type: object
                    properties:
                      phase:
                        type: string
                        enum: [pending, processing, completed, failed]
                      startTime:
                        type: string
                        format: date-time
                      completionTime:
                        type: string
                        format: date-time
                      currentStep:
                        type: integer
                        minimum: 0
                      totalSteps:
                        type: integer
                        minimum: 0
                      error:
                        type: string
                  executor:
                    type: object
                    properties:
                      phase:
                        type: string
                        enum: [pending, processing, completed, failed]
                      startTime:
                        type: string
                        format: date-time
                      completionTime:
                        type: string
                        format: date-time
                      actionsExecuted:
                        type: integer
                        minimum: 0
                      actionsTotal:
                        type: integer
                        minimum: 0
                      error:
                        type: string

              # Timing
              startTime:
                type: string
                format: date-time
              completionTime:
                type: string
                format: date-time
              lastReconciled:
                type: string
                format: date-time

              # Duplicate Alert Handling (BR-WH-008)
              duplicateAlerts:
                type: object
                properties:
                  count:
                    type: integer
                    minimum: 0
                    description: "Number of duplicate alerts received"
                  lastSeenAt:
                    type: string
                    format: date-time
                    description: "When the last duplicate was received"
                  lastPayloadHash:
                    type: string
                    description: "Hash of the last duplicate payload"

              # Conditions (Kubernetes standard)
              conditions:
                type: array
                items:
                  type: object
                  required:
                  - type
                  - status
                  - lastTransitionTime
                  properties:
                    type:
                      type: string
                      description: "Condition type (e.g., 'Ready', 'DuplicateAlert', 'Timeout')"
                    status:
                      type: string
                      enum: ["True", "False", "Unknown"]
                    reason:
                      type: string
                      description: "Machine-readable reason for the condition"
                    message:
                      type: string
                      description: "Human-readable message"
                    lastTransitionTime:
                      type: string
                      format: date-time
                    observedGeneration:
                      type: integer
                      format: int64

    # Additional versions for future compatibility
    additionalPrinterColumns:
    - name: Phase
      type: string
      description: Current remediation phase
      jsonPath: .status.phase
    - name: Progress
      type: string
      description: Overall progress percentage
      jsonPath: .status.overallProgress
    - name: Environment
      type: string
      description: Target environment
      jsonPath: .spec.environment
    - name: Severity
      type: string
      description: Alert severity
      jsonPath: .spec.severity
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp

    subresources:
      status: {}

  scope: Namespaced
  names:
    plural: alertremediations
    singular: alertremediation
    kind: AlertRemediation
    shortNames:
    - ar
    - remediation
    categories:
    - kubernaut
    - alerts
```

---

## 📝 **CUSTOM RESOURCE EXAMPLE**

### **Production Critical Alert Example**

```yaml
apiVersion: kubernaut.io/v1
kind: AlertRemediation
metadata:
  name: alert-remediation-prod-memory-high-001
  namespace: kubernaut-system
  labels:
    kubernaut.io/alert-fingerprint: "a1b2c3d4e5f6789a"
    kubernaut.io/environment: "production"
    kubernaut.io/severity: "critical"
    kubernaut.io/alert-type: "memory-usage"
  annotations:
    kubernaut.io/created-by: "gateway-service"
    kubernaut.io/business-requirement: "BR-PA-001,BR-WH-008"
spec:
  # Core Alert Identity
  alertFingerprint: "a1b2c3d4e5f6789a"
  severity: critical
  environment: production

  # Complete Alert Payload
  alertPayload:
    alertname: "HighMemoryUsage"
    labels:
      alertname: "HighMemoryUsage"
      instance: "web-server-01:9100"
      job: "node-exporter"
      severity: "critical"
      namespace: "production-web"
      pod: "web-server-deployment-7d4b8c9f5-xyz12"
      container: "web-server"
    annotations:
      description: "Memory usage is above 90% for more than 5 minutes"
      summary: "High memory usage detected on web-server-01"
      runbook_url: "https://runbooks.company.com/memory-high"
    startsAt: "2025-01-15T10:30:00Z"
    generatorURL: "http://prometheus:9090/graph?g0.expr=..."

  # Timing Configuration
  createdAt: "2025-01-15T10:30:00Z"
  timeoutAt: "2025-01-15T11:30:00Z"
  maxDuration: "1h"

  # Remediation Configuration
  remediationConfig:
    dryRun: false
    requireApproval: false  # Auto-execute for critical production alerts
    escalationChannels:
    - "slack-production-alerts"
    - "pagerduty-sre-team"
    - "email-platform-team"
    retryPolicy:
      maxRetries: 3
      backoffMultiplier: 2.0

status:
  # Overall State
  phase: processing
  overallProgress: 60

  # Service Status Tracking
  serviceStatuses:
    alertprocessor:
      phase: completed
      startTime: "2025-01-15T10:30:01Z"
      completionTime: "2025-01-15T10:30:15Z"
      environment: production
      businessPriority: critical
    aianalysis:
      phase: completed
      startTime: "2025-01-15T10:30:15Z"
      completionTime: "2025-01-15T10:31:45Z"
      confidence: 0.92
      recommendations: 3
    workflow:
      phase: processing
      startTime: "2025-01-15T10:31:45Z"
      currentStep: 2
      totalSteps: 4
    executor:
      phase: pending

  # Timing
  startTime: "2025-01-15T10:30:01Z"
  lastReconciled: "2025-01-15T10:32:30Z"

  # No duplicate alerts yet
  duplicateAlerts:
    count: 0

  # Conditions
  conditions:
  - type: "Ready"
    status: "True"
    reason: "RemediationInProgress"
    message: "Alert remediation is progressing normally"
    lastTransitionTime: "2025-01-15T10:30:01Z"
  - type: "DuplicateAlert"
    status: "False"
    reason: "NoDuplicates"
    message: "No duplicate alerts detected"
    lastTransitionTime: "2025-01-15T10:30:01Z"
```

### **Development Environment Alert Example**

```yaml
apiVersion: kubernaut.io/v1
kind: AlertRemediation
metadata:
  name: alert-remediation-dev-disk-space-002
  namespace: kubernaut-system
  labels:
    kubernaut.io/alert-fingerprint: "b2c3d4e5f6789abc"
    kubernaut.io/environment: "development"
    kubernaut.io/severity: "warning"
    kubernaut.io/alert-type: "disk-usage"
spec:
  alertFingerprint: "b2c3d4e5f6789abc"
  severity: warning
  environment: development

  alertPayload:
    alertname: "DiskSpaceLow"
    labels:
      alertname: "DiskSpaceLow"
      instance: "dev-worker-02:9100"
      job: "node-exporter"
      severity: "warning"
      namespace: "development"
      device: "/dev/sda1"
    annotations:
      description: "Disk usage is above 80% on /dev/sda1"
      summary: "Low disk space on dev-worker-02"
    startsAt: "2025-01-15T14:15:00Z"

  createdAt: "2025-01-15T14:15:00Z"
  timeoutAt: "2025-01-15T14:45:00Z"  # Shorter timeout for dev
  maxDuration: "30m"

  remediationConfig:
    dryRun: false
    requireApproval: true  # Require approval for dev environment
    escalationChannels:
    - "slack-dev-team"
    retryPolicy:
      maxRetries: 2
      backoffMultiplier: 1.5

status:
  phase: processing
  overallProgress: 25

  serviceStatuses:
    alertprocessor:
      phase: completed
      startTime: "2025-01-15T14:15:01Z"
      completionTime: "2025-01-15T14:15:10Z"
      environment: development
      businessPriority: low
    aianalysis:
      phase: processing
      startTime: "2025-01-15T14:15:10Z"
    workflow:
      phase: pending
    executor:
      phase: pending

  startTime: "2025-01-15T14:15:01Z"
  lastReconciled: "2025-01-15T14:15:45Z"

  duplicateAlerts:
    count: 0

  conditions:
  - type: "Ready"
    status: "True"
    reason: "RemediationInProgress"
    message: "Alert remediation is progressing normally"
    lastTransitionTime: "2025-01-15T14:15:01Z"
  - type: "ApprovalRequired"
    status: "True"
    reason: "DevelopmentEnvironment"
    message: "Manual approval required for development environment"
    lastTransitionTime: "2025-01-15T14:15:01Z"
```

---

## 🔄 **CONTROLLER RESPONSIBILITIES**

### **Primary Functions**
1. **Watch Service CRDs**: Monitor AlertProcessing, AIAnalysis, WorkflowExecution, and ~~KubernetesExecution~~ (DEPRECATED - ADR-025) CRDs
2. **Status Aggregation**: Collect and aggregate status from all service controllers
3. **Progress Calculation**: Calculate overall progress based on service completion
4. **Timeout Management**: Monitor remediation timeouts and trigger escalation
5. **Duplicate Detection**: Detect and handle duplicate alerts using fingerprint matching
6. **Lifecycle Management**: Manage complete remediation lifecycle from creation to cleanup

### **Reconciliation Logic**
```go
func (r *AlertRemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Get AlertRemediation resource
    // 2. Check for timeout and handle escalation
    // 3. Aggregate status from service-specific CRDs
    // 4. Calculate overall progress and phase
    // 5. Update status if changes detected
    // 6. Handle duplicate alert detection
    // 7. Manage cleanup for completed remediations
    // 8. Return appropriate requeue strategy
}
```

### **Watch Configuration**
The controller watches:
- **AlertRemediation CRDs**: Primary resource
- **AlertProcessing CRDs**: For alert processor status updates
- **AIAnalysis CRDs**: For AI analysis status updates
- **WorkflowExecution CRDs**: For workflow status updates
- **~~KubernetesExecution~~ (DEPRECATED - ADR-025) CRDs**: For executor status updates

---

## 🎯 **KEY DESIGN FEATURES**

### **1. Deduplication Support (BR-WH-008)**
- **Fingerprint-based**: Uses `alertFingerprint` for unique identification
- **Duplicate Tracking**: Counts and timestamps duplicate occurrences
- **Payload Hashing**: Tracks payload changes in duplicates

### **2. Environment-Aware Processing**
- **Environment Classification**: Explicit environment field for routing
- **Environment-Specific Timeouts**: Different timeout policies per environment
- **Approval Requirements**: Configurable approval workflows per environment

### **3. Progress Tracking**
- **Service-Level Status**: Detailed status for each service controller
- **Overall Progress**: Aggregated progress percentage
- **Real-Time Updates**: Watch-based status updates

### **4. Timeout Management**
- **Configurable Timeouts**: Per-alert timeout configuration
- **Automatic Escalation**: Escalation when timeouts are exceeded
- **Escalation Channels**: Multi-channel notification support

### **5. Kubernetes Standards Compliance**
- **Conditions**: Standard Kubernetes condition patterns
- **Printer Columns**: kubectl output customization
- **Subresources**: Status subresource for proper RBAC
- **Short Names**: Convenient CLI aliases

### **6. Extensibility**
- **Additional Properties**: Room for future enhancements
- **Versioning Support**: Multiple API versions
- **Categories**: Logical grouping for discovery

---

## 📊 **OPERATIONAL CHARACTERISTICS**

### **Performance Targets**
- **Reconciliation Latency**: <2 seconds for status updates
- **Duplicate Detection**: <100ms for fingerprint lookup
- **Timeout Detection**: <30 seconds from timeout occurrence
- **Status Aggregation**: <1 second for multi-service status collection

### **Scalability**
- **Concurrent Remediations**: Support 1000+ active remediations
- **Watch Efficiency**: Optimized watch patterns for minimal API load
- **Resource Usage**: <100MB memory per 1000 active remediations

### **Reliability**
- **Leader Election**: High availability through controller leader election
- **Graceful Degradation**: Continue operation with partial service availability
- **Error Recovery**: Automatic retry with exponential backoff

---

## 🔒 **SECURITY CONSIDERATIONS**

### **RBAC Requirements**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alertremediation-controller
rules:
- apiGroups: ["kubernaut.io"]
  resources: ["alertremediations"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["kubernaut.io"]
  resources: ["alertremediations/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["kubernaut.io"]
  resources: ["alertprocessings", "aianalyses", "workflowexecutions", "kubernetesexecutions"]
  verbs: ["get", "list", "watch"]
```

### **Data Protection**
- **Sensitive Data**: Alert payloads may contain sensitive information
- **Access Control**: Proper RBAC for resource access
- **Audit Logging**: Complete audit trail for compliance

---

## 📋 **IMPLEMENTATION CHECKLIST**

### **Phase 1: Core Infrastructure (Week 1-2)**
- [ ] CRD definition and installation
- [ ] Basic controller scaffolding
- [ ] Status aggregation logic
- [ ] Watch configuration for service CRDs

### **Phase 2: Advanced Features (Week 3-4)**
- [ ] Duplicate detection and handling
- [ ] Timeout management and escalation
- [ ] Progress calculation algorithms
- [ ] Cleanup and lifecycle management

### **Phase 3: Production Readiness (Week 5-6)**
- [ ] Performance optimization
- [ ] Error handling and recovery
- [ ] Monitoring and metrics
- [ ] Documentation and runbooks

---

## 🔗 **RELATED DOCUMENTS**

- [Kubernaut CRD Architecture](../../architecture/KUBERNAUT_CRD_ARCHITECTURE.md) (Authoritative)
- ~~[Multi-CRD Reconciliation Architecture](../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)~~ (DEPRECATED)
- [Business Requirements Overview](../requirements/00_REQUIREMENTS_OVERVIEW.md)
- [AlertProcessing CRD Design](./02_ALERT_PROCESSING_CRD.md) *(Next)*
- [AIAnalysis CRD Design](./03_AI_ANALYSIS_CRD.md) *(Planned)*
- [WorkflowExecution CRD Design](./04_WORKFLOW_EXECUTION_CRD.md) *(Planned)*
- [KubernetesExecution CRD Design](./05_KUBERNETES_EXECUTION_CRD.md) *(DEPRECATED - ADR-025)*

---

**Document Status**: ✅ **APPROVED**
**Implementation Priority**: **P0 - CRITICAL**
**Next Steps**: Proceed with AlertProcessing CRD design
