# ⛔ DEPRECATED - DO NOT USE FOR IMPLEMENTATION

**Status**: 🚨 **HISTORICAL REFERENCE ONLY**
**Deprecated**: January 2025
**Confidence**: 100% - This document is ~70% incorrect

---

## 🎯 **FOR CURRENT V1 INFORMATION, SEE:**

### **1. Implementation (Source of Truth)**
- **[`api/remediationprocessing/v1alpha1/remediationprocessing_types.go`](../../../api/remediationprocessing/v1alpha1/remediationprocessing_types.go)** - Actual Go implementation

### **2. Schema Documentation**
- **[`docs/architecture/CRD_SCHEMAS.md`](../../architecture/CRD_SCHEMAS.md)** - Authoritative schema documentation

### **3. Service Specifications (~2,000 lines)**
- **[`docs/services/crd-controllers/01-signalprocessing/`](../../services/crd-controllers/01-signalprocessing/)** - Complete service specs

---

## ⚠️ **CRITICAL ISSUES IN THIS DOCUMENT**

**This document is ~70% incorrect for V1. Missing 18 Phase 1 fields!**

| Issue | Severity | What's Wrong |
|-------|----------|--------------|
| **CRD Name** | ⛔ BLOCKER | `alertprocessings.alertprocessor.kubernaut.io` → Should be `remediationprocessings.signalprocessing.kubernaut.io` |
| **API Version** | 🟡 MEDIUM | `v1` → Should be `v1alpha1` |
| **Parent Reference** | 🔴 HIGH | `alertRemediationRef` → Should be `remediationRequestRef` |
| **Missing Fields** | 🔴 HIGH | Missing 18 Phase 1 self-contained fields (signalLabels, signalAnnotations, targetResource, etc.) |
| **Field Naming** | ⛔ BLOCKER | Uses deprecated "Alert" prefix (`AlertProcessing`, `alertRemediationRef`) |
| **Business Reqs** | 🟡 MEDIUM | Wrong BR references (BR-SP-*, BR-ENV-* → Should be BR-PROC-*) |
| **Self-Containment** | 🔴 HIGH | Missing core V1 pattern - RemediationProcessing must be self-contained |

**Schema Completeness**: ~30% of V1 fields present

**Phase 1 Priority**: This CRD needs 18 new fields for self-containment (Task 2)

---

## 📜 **ORIGINAL DOCUMENT (OUTDATED) BELOW**

**Warning**: Everything below this line is outdated. See links above for current information.

---

# SignalProcessing CRD Design Document

**Document Version**: 1.0
**Date**: January 2025
**Status**: **DEPRECATED** - See banner above
**CRD Type**: Signal Processor Service Controller
**Priority**: **CRITICAL** - Core signal processing component

---

## ⚠️ ORIGINAL DEPRECATION NOTICES (ALSO OUTDATED)

### **1. Authoritative Source**

This document is **REFERENCE ONLY**. The authoritative CRD definitions are in:
- **[CRD_SCHEMAS.md](../../architecture/CRD_SCHEMAS.md)** - Authoritative OpenAPI v3 schemas
- **[V1 Source of Truth Hierarchy](../../V1_SOURCE_OF_TRUTH_HIERARCHY.md)** - Documentation authority

### **2. Naming Convention**

This document uses **deprecated "Alert" prefix naming**:
- `AlertProcessing` CRD → **`RemediationProcessing`** (current name)
- Alert-specific terminology → Signal-agnostic terminology

**Why Deprecated**: Kubernaut processes multiple signal types (Prometheus alerts, Kubernetes events, AWS CloudWatch alarms), not just alerts.

**Migration**: [ADR-015: Alert to Signal Naming Migration](../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md)

---

## 🎯 **PURPOSE AND SCOPE**

### **Business Purpose**
The AlertProcessing CRD manages the first stage of alert processing where raw alerts are enriched with context, classified by environment using fully flexible business rules, and assigned dynamic business priorities. This CRD enables sophisticated, enterprise-ready alert processing that adapts to any organizational structure and naming conventions.

### **Scope**
- **Alert Enrichment**: Multi-source context enrichment (Kubernetes, monitoring, historical, business, external)
- **Flexible Environment Classification**: Completely configurable environment classification without hardcoded assumptions
- **Dynamic Business Priority Assignment**: Priority calculation based on business criticality, not predetermined environments
- **Hierarchical Environment Support**: Support for complex organizational naming patterns
- **ConfigMap-Based Rules**: External rule configuration for dynamic updates
- **Business Hours Awareness**: Timezone and business hours consideration for priority adjustment

---

## 📋 **BUSINESS REQUIREMENTS ADDRESSED**

### **Primary Business Requirements**
- **BR-SP-001 to BR-SP-050**: Complete alert processing capabilities
- **BR-ENV-001 to BR-ENV-050**: Environment classification and namespace management
- **BR-ENV-004**: ConfigMap-based classification rules for dynamic configuration
- **BR-ENV-007**: Hierarchical environment classification support
- **BR-ENV-019**: Dynamic configuration updates without service restart
- **BR-CLOUD-002**: Custom organizational labels for business context
- **BR-SP-031 to BR-SP-033**: Dynamic business priority assignment

### **Specific Compliance Requirements**
- **BR-ENV-005**: Fallback classification using namespace pattern matching
- **BR-ENV-009**: Business criticality levels based on environment classification
- **BR-ENV-014**: Alert filtering based on environment classification and business priority
- **BR-SP-026**: Environment-specific alert routing based on business criticality
- **BR-QUAL-ENV-011**: Reasonable fallback defaults for unknown namespaces

---

## 🏗️ **CRD SPECIFICATION**

### **Custom Resource Definition**

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: alertprocessings.alertprocessor.kubernaut.io
  annotations:
    controller-gen.kubebuilder.io/version: v0.13.0
    kubernaut.io/description: "Alert processing CRD for enrichment and fully flexible environment classification"
    kubernaut.io/business-requirements: "BR-SP-001,BR-ENV-004,BR-ENV-007,BR-ENV-019,BR-CLOUD-002,BR-SP-031"
spec:
  group: alertprocessor.kubernaut.io
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
            - alertRemediationRef
            - alert
            - processingConfig
            properties:
              # Reference to parent AlertRemediation
              alertRemediationRef:
                type: object
                required:
                - name
                - namespace
                properties:
                  name:
                    type: string
                    description: "Name of the parent AlertRemediation resource"
                  namespace:
                    type: string
                    description: "Namespace of the parent AlertRemediation resource"

              # Alert Data
              alert:
                type: object
                required:
                - fingerprint
                - payload
                - severity
                - targetNamespace
                properties:
                  fingerprint:
                    type: string
                    pattern: "^[a-f0-9]{12,64}$"
                    description: "Alert fingerprint for correlation"
                  payload:
                    type: object
                    description: "Raw alert payload from Prometheus"
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
                  severity:
                    type: string
                    enum: [critical, warning, info]
                    description: "Original alert severity"
                  targetNamespace:
                    type: string
                    description: "Kubernetes namespace where the alert originated"
                  receivedAt:
                    type: string
                    format: date-time
                    description: "When the alert was received for processing"

              # Processing Configuration
              processingConfig:
                type: object
                properties:
                  # Context Enrichment Configuration (BR-SP-011 to BR-SP-015)
                  enrichmentConfig:
                    type: object
                    properties:
                      contextSources:
                        type: array
                        items:
                          type: string
                          enum: [kubernetes, monitoring, historical, business, external]
                        description: "Sources for context enrichment"
                      contextDepth:
                        type: string
                        enum: [basic, detailed, comprehensive]
                        default: detailed
                        description: "Depth of context enrichment"
                      historicalLookback:
                        type: string
                        pattern: "^[0-9]+(s|m|h|d)$"
                        default: "24h"
                        description: "Historical data lookback period"
                      includeMetrics:
                        type: boolean
                        default: true
                        description: "Include monitoring metrics in enrichment"
                      includeEvents:
                        type: boolean
                        default: true
                        description: "Include Kubernetes events in enrichment"

                  # Fully Flexible Environment Classification Configuration
                  environmentClassification:
                    type: object
                    properties:
                      classificationSources:
                        type: array
                        items:
                          type: string
                          enum: [labels, annotations, patterns, configmap, external]
                        default: [labels, annotations, patterns]
                        description: "Sources for environment classification"
                      confidenceThreshold:
                        type: number
                        minimum: 0.0
                        maximum: 1.0
                        default: 0.8
                        description: "Minimum confidence for classification"

                      # Fully Flexible Business Rules (BR-ENV-004, BR-ENV-007, BR-CLOUD-002)
                      businessRules:
                        type: object
                        properties:
                          # Dynamic environment mapping (completely flexible)
                          environmentMappings:
                            type: array
                            items:
                              type: object
                              required:
                              - environment
                              - patterns
                              - priority
                              properties:
                                environment:
                                  type: string
                                  description: "Target environment name (completely flexible, supports any custom environment)"
                                patterns:
                                  type: array
                                  items:
                                    type: string
                                  description: "Regex patterns for this environment"
                                priority:
                                  type: integer
                                  minimum: 1
                                  description: "Priority for conflict resolution (1=highest)"
                                businessCriticality:
                                  type: string
                                  enum: [critical, high, medium, low]
                                  description: "Business criticality for this environment"
                                slaRequirements:
                                  type: object
                                  properties:
                                    responseTime:
                                      type: string
                                      description: "Required response time (e.g., '5m')"
                                    resolutionTime:
                                      type: string
                                      description: "Required resolution time (e.g., '1h')"
                                    availability:
                                      type: string
                                      description: "Availability requirement (e.g., '99.9%')"
                                metadata:
                                  type: object
                                  additionalProperties:
                                    type: string
                                  description: "Additional metadata for this environment"

                          # ConfigMap reference for external rules (BR-ENV-004, BR-ENV-019)
                          configMapRef:
                            type: object
                            properties:
                              name:
                                type: string
                                description: "ConfigMap containing classification rules"
                              namespace:
                                type: string
                                description: "Namespace of the ConfigMap"
                              key:
                                type: string
                                default: "classification-rules.yaml"
                                description: "Key in ConfigMap containing rules"

                          # Hierarchical classification support (BR-ENV-007)
                          hierarchicalRules:
                            type: object
                            properties:
                              enabled:
                                type: boolean
                                default: true
                                description: "Enable hierarchical environment classification"
                              separator:
                                type: string
                                default: "-"
                                description: "Separator for hierarchical names (e.g., 'prod-web-frontend')"
                              maxDepth:
                                type: integer
                                minimum: 1
                                maximum: 5
                                default: 3
                                description: "Maximum hierarchy depth to consider"
                              extractionOrder:
                                type: array
                                items:
                                  type: string
                                  enum: [prefix, suffix, middle]
                                default: [prefix, suffix]
                                description: "Order to extract environment from hierarchical names"

                      fallbackEnvironment:
                        type: string
                        default: "development"
                        description: "Fallback environment when classification fails (BR-ENV-005, BR-QUAL-ENV-011)"

                  # Fully Dynamic Business Priority Configuration (BR-SP-031 to BR-SP-033)
                  businessPriorityConfig:
                    type: object
                    properties:
                      enableBusinessHours:
                        type: boolean
                        default: true
                        description: "Apply business hours priority adjustment"
                      timezone:
                        type: string
                        default: "UTC"
                        description: "Timezone for business hours calculation"
                      businessHours:
                        type: object
                        properties:
                          start:
                            type: string
                            pattern: "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$"
                            default: "09:00"
                          end:
                            type: string
                            pattern: "^([0-1]?[0-9]|2[0-3]):[0-5][0-9]$"
                            default: "17:00"
                          weekdays:
                            type: array
                            items:
                              type: string
                              enum: [monday, tuesday, wednesday, thursday, friday, saturday, sunday]
                            default: [monday, tuesday, wednesday, thursday, friday]

                      # Fully Dynamic Priority Configuration (No Hardcoded Environments)
                      priorityConfiguration:
                        type: object
                        properties:
                          # Base priority multipliers based on business criticality
                          baseMultipliers:
                            type: object
                            properties:
                              critical:
                                type: number
                                minimum: 1.0
                                default: 4.0
                                description: "Multiplier for critical business criticality"
                              high:
                                type: number
                                minimum: 1.0
                                default: 3.0
                                description: "Multiplier for high business criticality"
                              medium:
                                type: number
                                minimum: 1.0
                                default: 2.0
                                description: "Multiplier for medium business criticality"
                              low:
                                type: number
                                minimum: 1.0
                                default: 1.0
                                description: "Multiplier for low business criticality"

                          # Severity multipliers
                          severityMultipliers:
                            type: object
                            properties:
                              critical:
                                type: number
                                minimum: 1.0
                                default: 3.0
                              warning:
                                type: number
                                minimum: 1.0
                                default: 2.0
                              info:
                                type: number
                                minimum: 1.0
                                default: 1.0

                          # Business hours multipliers
                          businessHoursMultipliers:
                            type: object
                            properties:
                              duringBusinessHours:
                                type: number
                                minimum: 1.0
                                default: 1.5
                                description: "Multiplier during business hours"
                              outsideBusinessHours:
                                type: number
                                minimum: 1.0
                                default: 1.0
                                description: "Multiplier outside business hours"

                          # Custom environment-specific overrides (optional)
                          environmentOverrides:
                            type: array
                            items:
                              type: object
                              required:
                              - environment
                              - multiplier
                              properties:
                                environment:
                                  type: string
                                  description: "Environment name (matches environmentMappings)"
                                multiplier:
                                  type: number
                                  minimum: 1.0
                                  description: "Custom multiplier for this specific environment"
                                reason:
                                  type: string
                                  description: "Business justification for custom multiplier"
                            description: "Optional environment-specific multiplier overrides"

                          # Priority calculation method
                          calculationMethod:
                            type: string
                            enum: [multiplicative, additive, weighted]
                            default: multiplicative
                            description: "Method for combining priority factors"

                          # Priority thresholds for P0-P4 assignment
                          priorityThresholds:
                            type: object
                            properties:
                              p0:
                                type: number
                                minimum: 0.0
                                default: 20.0
                                description: "Minimum score for P0 priority"
                              p1:
                                type: number
                                minimum: 0.0
                                default: 15.0
                                description: "Minimum score for P1 priority"
                              p2:
                                type: number
                                minimum: 0.0
                                default: 10.0
                                description: "Minimum score for P2 priority"
                              p3:
                                type: number
                                minimum: 0.0
                                default: 5.0
                                description: "Minimum score for P3 priority"
                              # P4 is anything below P3 threshold

          status:
            type: object
            properties:
              # Processing State
              phase:
                type: string
                enum: [enriching, classifying, prioritizing, routing, completed, failed]
                description: "Current processing phase"

              # Processing Results
              enrichmentResults:
                type: object
                properties:
                  kubernetesContext:
                    type: object
                    properties:
                      clusterName:
                        type: string
                      nodeInfo:
                        type: object
                      podInfo:
                        type: object
                      serviceInfo:
                        type: object
                      resourceQuotas:
                        type: object
                  monitoringContext:
                    type: object
                    properties:
                      relatedMetrics:
                        type: array
                        items:
                          type: object
                      historicalData:
                        type: object
                      trendAnalysis:
                        type: object
                  businessContext:
                    type: object
                    properties:
                      costCenter:
                        type: string
                      businessUnit:
                        type: string
                      applicationOwner:
                        type: string
                      serviceLevel:
                        type: string
                  enrichmentTime:
                    type: string
                    format: date-time
                  enrichmentDuration:
                    type: string
                    description: "Time taken for enrichment (e.g., '1.5s')"

              # Flexible Environment Classification Results
              environmentClassification:
                type: object
                properties:
                  environment:
                    type: string
                    description: "Classified environment type (completely flexible, not enum)"
                  confidence:
                    type: number
                    minimum: 0.0
                    maximum: 1.0
                    description: "Classification confidence score"
                  classificationMethod:
                    type: string
                    enum: [labels, annotations, patterns, configmap, external, hierarchical, fallback]
                    description: "Method used for classification"
                  matchedPattern:
                    type: string
                    description: "Specific pattern that matched (for debugging)"
                  hierarchicalMatch:
                    type: object
                    properties:
                      originalName:
                        type: string
                        description: "Original namespace name"
                      extractedEnvironment:
                        type: string
                        description: "Environment extracted from hierarchy"
                      hierarchyLevel:
                        type: integer
                        description: "Level in hierarchy where match was found"
                  businessCriticality:
                    type: string
                    enum: [critical, high, medium, low]
                    description: "Business criticality level (BR-ENV-009)"
                  slaRequirements:
                    type: object
                    properties:
                      responseTime:
                        type: string
                        description: "Required response time (e.g., '5m')"
                      resolutionTime:
                        type: string
                        description: "Required resolution time (e.g., '1h')"
                      availability:
                        type: string
                        description: "Availability requirement (e.g., '99.9%')"
                  environmentMetadata:
                    type: object
                    additionalProperties:
                      type: string
                    description: "Additional environment-specific metadata"
                  classificationTime:
                    type: string
                    format: date-time
                  classificationDuration:
                    type: string
                    description: "Time taken for classification (e.g., '0.1s')"

              # Dynamic Business Priority Results
              businessPriority:
                type: object
                properties:
                  priority:
                    type: string
                    enum: [p0, p1, p2, p3, p4]
                    description: "Assigned business priority"
                  priorityScore:
                    type: number
                    minimum: 0.0
                    description: "Calculated priority score"
                  priorityFactors:
                    type: object
                    properties:
                      baseMultiplier:
                        type: number
                        description: "Multiplier based on business criticality"
                      severityMultiplier:
                        type: number
                        description: "Multiplier based on alert severity"
                      businessHoursMultiplier:
                        type: number
                        description: "Multiplier based on business hours"
                      environmentOverride:
                        type: number
                        description: "Environment-specific override multiplier"
                      calculationMethod:
                        type: string
                        description: "Method used for calculation"
                  businessCriticalityUsed:
                    type: string
                    enum: [critical, high, medium, low]
                    description: "Business criticality level used from environment mapping"
                  environmentOverrideApplied:
                    type: string
                    description: "Environment override rule applied (if any)"
                  businessHoursActive:
                    type: boolean
                    description: "Whether business hours are currently active"
                  priorityTime:
                    type: string
                    format: date-time

              # Routing Decision (BR-SP-026, BR-ENV-014)
              routingDecision:
                type: object
                properties:
                  targetService:
                    type: string
                    enum: [ai-analysis, workflow-execution, notification]
                    description: "Next service in the processing chain"
                  routingReason:
                    type: string
                    description: "Reason for routing decision"
                  escalationChannels:
                    type: array
                    items:
                      type: string
                    description: "Notification channels for escalation"
                  approvalRequired:
                    type: boolean
                    description: "Whether manual approval is required"
                  routingTime:
                    type: string
                    format: date-time

              # Processing Metrics
              processingMetrics:
                type: object
                properties:
                  totalProcessingTime:
                    type: string
                    description: "Total processing time (e.g., '2.3s')"
                  enrichmentTime:
                    type: string
                    description: "Time spent on enrichment"
                  classificationTime:
                    type: string
                    description: "Time spent on classification"
                  prioritizationTime:
                    type: string
                    description: "Time spent on prioritization"
                  routingTime:
                    type: string
                    description: "Time spent on routing"

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

              # Error Information
              error:
                type: string
                description: "Error message if processing failed"

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
                      description: "Condition type (e.g., 'Ready', 'Enriched', 'Classified')"
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

    additionalPrinterColumns:
    - name: Phase
      type: string
      description: Current processing phase
      jsonPath: .status.phase
    - name: Environment
      type: string
      description: Classified environment
      jsonPath: .status.environmentClassification.environment
    - name: Priority
      type: string
      description: Business priority
      jsonPath: .status.businessPriority.priority
    - name: Confidence
      type: string
      description: Classification confidence
      jsonPath: .status.environmentClassification.confidence
    - name: Method
      type: string
      description: Classification method used
      jsonPath: .status.environmentClassification.classificationMethod
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp

    subresources:
      status: {}

  scope: Namespaced
  names:
    plural: alertprocessings
    singular: alertprocessing
    kind: AlertProcessing
    shortNames:
    - ap
    - alertproc
    categories:
    - kubernaut
    - alerts
```

---

## 📝 **CUSTOM RESOURCE EXAMPLES**

### **Enterprise Production Alert Example**

```yaml
apiVersion: alertprocessor.kubernaut.io/v1
kind: AlertProcessing
metadata:
  name: alert-processing-prod-memory-high-001
  namespace: kubernaut-system
  labels:
    kubernaut.io/alert-fingerprint: "a1b2c3d4e5f6789a"
    kubernaut.io/environment: "production"
    kubernaut.io/severity: "critical"
  annotations:
    kubernaut.io/created-by: "gateway-service"
    kubernaut.io/business-requirement: "BR-SP-001,BR-ENV-004"
spec:
  # Reference to parent AlertRemediation
  alertRemediationRef:
    name: "alert-remediation-prod-memory-high-001"
    namespace: "kubernaut-system"

  # Alert Data
  alert:
    fingerprint: "a1b2c3d4e5f6789a"
    payload:
      alertname: "HighMemoryUsage"
      labels:
        alertname: "HighMemoryUsage"
        instance: "web-server-01:9100"
        job: "node-exporter"
        severity: "critical"
        namespace: "prod-finance-web"
        pod: "web-server-deployment-7d4b8c9f5-xyz12"
        container: "web-server"
      annotations:
        description: "Memory usage is above 90% for more than 5 minutes"
        summary: "High memory usage detected on web-server-01"
        runbook_url: "https://runbooks.company.com/memory-high"
      startsAt: "2025-01-15T10:30:00Z"
      generatorURL: "http://prometheus:9090/graph?g0.expr=..."
    severity: critical
    targetNamespace: "prod-finance-web"
    receivedAt: "2025-01-15T10:30:00Z"

  # Processing Configuration
  processingConfig:
    # Context Enrichment
    enrichmentConfig:
      contextSources: [kubernetes, monitoring, historical, business]
      contextDepth: comprehensive
      historicalLookback: "24h"
      includeMetrics: true
      includeEvents: true

    # Flexible Environment Classification
    environmentClassification:
      classificationSources: [labels, annotations, patterns, configmap]
      confidenceThreshold: 0.8
      businessRules:
        environmentMappings:
        # Production Finance Environment
        - environment: "production"
          patterns:
          - "^prod-.*"
          - "^production-.*"
          - ".*-prod$"
          priority: 1
          businessCriticality: critical
          slaRequirements:
            responseTime: "5m"
            resolutionTime: "1h"
            availability: "99.9%"
          metadata:
            compliance: "sox,gdpr"
            costCenter: "finance"

        # Pre-production Environment
        - environment: "pre-production"
          patterns:
          - "^pre-prod-.*"
          - "^preprod-.*"
          priority: 2
          businessCriticality: high
          slaRequirements:
            responseTime: "15m"
            resolutionTime: "4h"
            availability: "99.5%"

        # ConfigMap reference for additional rules
        configMapRef:
          name: "acme-corp-classification-rules"
          namespace: "kubernaut-system"
          key: "business-rules.yaml"

        # Hierarchical support
        hierarchicalRules:
          enabled: true
          separator: "-"
          maxDepth: 3
          extractionOrder: [prefix, suffix]

      fallbackEnvironment: "development"

    # Dynamic Business Priority Configuration
    businessPriorityConfig:
      enableBusinessHours: true
      timezone: "America/New_York"
      businessHours:
        start: "09:00"
        end: "17:00"
        weekdays: [monday, tuesday, wednesday, thursday, friday]

      priorityConfiguration:
        baseMultipliers:
          critical: 4.0
          high: 3.0
          medium: 2.0
          low: 1.0
        severityMultipliers:
          critical: 3.0
          warning: 2.0
          info: 1.0
        businessHoursMultipliers:
          duringBusinessHours: 1.5
          outsideBusinessHours: 1.0
        environmentOverrides:
        - environment: "production"
          multiplier: 5.0
          reason: "SOX compliance requires immediate response for production finance systems"
        calculationMethod: multiplicative
        priorityThresholds:
          p0: 20.0
          p1: 15.0
          p2: 10.0
          p3: 5.0

status:
  phase: completed

  # Enrichment Results
  enrichmentResults:
    kubernetesContext:
      clusterName: "prod-finance-cluster"
      nodeInfo:
        nodeName: "web-server-01"
        nodeCapacity:
          memory: "32Gi"
          cpu: "8"
        nodeAllocatable:
          memory: "30Gi"
          cpu: "7800m"
      podInfo:
        podName: "web-server-deployment-7d4b8c9f5-xyz12"
        podPhase: "Running"
        containers:
        - name: "web-server"
          memoryRequest: "2Gi"
          memoryLimit: "4Gi"
          cpuRequest: "500m"
          cpuLimit: "1000m"
      serviceInfo:
        serviceName: "web-server-service"
        serviceType: "ClusterIP"
        endpoints: 3
    monitoringContext:
      relatedMetrics:
      - name: "node_memory_usage_percent"
        value: 92.5
        timestamp: "2025-01-15T10:29:00Z"
      - name: "container_memory_usage_bytes"
        value: 3865470976
        timestamp: "2025-01-15T10:29:00Z"
      historicalData:
        averageMemoryUsage: 78.2
        peakMemoryUsage: 95.1
        trendDirection: "increasing"
    businessContext:
      costCenter: "finance-operations"
      businessUnit: "finance"
      applicationOwner: "finance-team@company.com"
      serviceLevel: "tier-1"
    enrichmentTime: "2025-01-15T10:30:02Z"
    enrichmentDuration: "1.8s"

  # Environment Classification Results
  environmentClassification:
    environment: "production"
    confidence: 0.95
    classificationMethod: patterns
    matchedPattern: "^prod-.*"
    businessCriticality: critical
    slaRequirements:
      responseTime: "5m"
      resolutionTime: "1h"
      availability: "99.9%"
    environmentMetadata:
      compliance: "sox,gdpr"
      costCenter: "finance"
    classificationTime: "2025-01-15T10:30:03Z"
    classificationDuration: "0.2s"

  # Business Priority Results
  businessPriority:
    priority: p0
    priorityScore: 60.0  # 4.0 * 3.0 * 1.0 * 5.0 = 60.0
    priorityFactors:
      baseMultiplier: 4.0      # critical business criticality
      severityMultiplier: 3.0  # critical alert severity
      businessHoursMultiplier: 1.0  # outside business hours
      environmentOverride: 5.0     # production finance override
      calculationMethod: multiplicative
    businessCriticalityUsed: critical
    environmentOverrideApplied: "production finance SOX compliance"
    businessHoursActive: false
    priorityTime: "2025-01-15T10:30:04Z"

  # Routing Decision
  routingDecision:
    targetService: ai-analysis
    routingReason: "P0 critical production alert requires AI analysis"
    escalationChannels:
    - "slack-production-alerts"
    - "pagerduty-sre-team"
    - "email-finance-team"
    approvalRequired: false
    routingTime: "2025-01-15T10:30:05Z"

  # Processing Metrics
  processingMetrics:
    totalProcessingTime: "3.2s"
    enrichmentTime: "1.8s"
    classificationTime: "0.2s"
    prioritizationTime: "0.1s"
    routingTime: "0.1s"

  # Timing
  startTime: "2025-01-15T10:30:01Z"
  completionTime: "2025-01-15T10:30:05Z"
  lastReconciled: "2025-01-15T10:30:05Z"

  # Conditions
  conditions:
  - type: "Ready"
    status: "True"
    reason: "ProcessingCompleted"
    message: "Alert processing completed successfully"
    lastTransitionTime: "2025-01-15T10:30:05Z"
  - type: "Enriched"
    status: "True"
    reason: "ContextEnrichmentCompleted"
    message: "Alert enriched with comprehensive context"
    lastTransitionTime: "2025-01-15T10:30:02Z"
  - type: "Classified"
    status: "True"
    reason: "EnvironmentClassified"
    message: "Environment classified as production with 95% confidence"
    lastTransitionTime: "2025-01-15T10:30:03Z"
  - type: "Prioritized"
    status: "True"
    reason: "BusinessPriorityAssigned"
    message: "Assigned P0 priority based on critical production finance alert"
    lastTransitionTime: "2025-01-15T10:30:04Z"
```

### **Custom Environment Example (UAT)**

```yaml
apiVersion: alertprocessor.kubernaut.io/v1
kind: AlertProcessing
metadata:
  name: alert-processing-uat-disk-space-002
  namespace: kubernaut-system
  labels:
    kubernaut.io/alert-fingerprint: "b2c3d4e5f6789abc"
    kubernaut.io/environment: "uat"
    kubernaut.io/severity: "warning"
spec:
  alertRemediationRef:
    name: "alert-remediation-uat-disk-space-002"
    namespace: "kubernaut-system"

  alert:
    fingerprint: "b2c3d4e5f6789abc"
    payload:
      alertname: "DiskSpaceLow"
      labels:
        alertname: "DiskSpaceLow"
        instance: "uat-api-server-01:9100"
        job: "node-exporter"
        severity: "warning"
        namespace: "uat-mobile-api"
        device: "/dev/sda1"
      annotations:
        description: "Disk usage is above 80% on /dev/sda1"
        summary: "Low disk space on uat-api-server-01"
      startsAt: "2025-01-15T14:15:00Z"
    severity: warning
    targetNamespace: "uat-mobile-api"
    receivedAt: "2025-01-15T14:15:00Z"

  processingConfig:
    enrichmentConfig:
      contextSources: [kubernetes, monitoring, historical]
      contextDepth: detailed
      historicalLookback: "12h"
      includeMetrics: true
      includeEvents: false

    environmentClassification:
      businessRules:
        environmentMappings:
        # Custom UAT Environment
        - environment: "uat"
          patterns:
          - "^uat-.*"
          - "^user-acceptance-.*"
          priority: 1
          businessCriticality: medium
          slaRequirements:
            responseTime: "30m"
            resolutionTime: "8h"
            availability: "99.0%"
          metadata:
            testingPhase: "user-acceptance"
            businessUnit: "mobile-team"

        hierarchicalRules:
          enabled: true
          separator: "-"
          maxDepth: 3

      fallbackEnvironment: "development"

    businessPriorityConfig:
      enableBusinessHours: true
      timezone: "UTC"
      priorityConfiguration:
        baseMultipliers:
          critical: 4.0
          high: 3.0
          medium: 2.0
          low: 1.0
        severityMultipliers:
          critical: 3.0
          warning: 2.0
          info: 1.0
        businessHoursMultipliers:
          duringBusinessHours: 1.5
          outsideBusinessHours: 1.0
        calculationMethod: multiplicative
        priorityThresholds:
          p0: 20.0
          p1: 15.0
          p2: 10.0
          p3: 5.0

status:
  phase: completed

  environmentClassification:
    environment: "uat"
    confidence: 0.88
    classificationMethod: patterns
    matchedPattern: "^uat-.*"
    businessCriticality: medium
    slaRequirements:
      responseTime: "30m"
      resolutionTime: "8h"
      availability: "99.0%"
    environmentMetadata:
      testingPhase: "user-acceptance"
      businessUnit: "mobile-team"
    classificationTime: "2025-01-15T14:15:02Z"
    classificationDuration: "0.1s"

  businessPriority:
    priority: p2
    priorityScore: 6.0  # 2.0 * 2.0 * 1.5 = 6.0
    priorityFactors:
      baseMultiplier: 2.0      # medium business criticality
      severityMultiplier: 2.0  # warning alert severity
      businessHoursMultiplier: 1.5  # during business hours
      environmentOverride: 1.0     # no override
      calculationMethod: multiplicative
    businessCriticalityUsed: medium
    businessHoursActive: true
    priorityTime: "2025-01-15T14:15:03Z"

  routingDecision:
    targetService: ai-analysis
    routingReason: "P2 UAT alert requires standard AI analysis"
    escalationChannels:
    - "slack-mobile-team"
    approvalRequired: true  # UAT requires approval
    routingTime: "2025-01-15T14:15:04Z"

  processingMetrics:
    totalProcessingTime: "1.8s"
    enrichmentTime: "1.2s"
    classificationTime: "0.1s"
    prioritizationTime: "0.1s"
    routingTime: "0.1s"

  startTime: "2025-01-15T14:15:01Z"
  completionTime: "2025-01-15T14:15:04Z"
  lastReconciled: "2025-01-15T14:15:04Z"

  conditions:
  - type: "Ready"
    status: "True"
    reason: "ProcessingCompleted"
    message: "Alert processing completed successfully"
    lastTransitionTime: "2025-01-15T14:15:04Z"
  - type: "ApprovalRequired"
    status: "True"
    reason: "UATEnvironment"
    message: "Manual approval required for UAT environment"
    lastTransitionTime: "2025-01-15T14:15:04Z"
```

---

## 🔄 **CONTROLLER RESPONSIBILITIES**

### **Primary Functions**
1. **Alert Enrichment**: Gather context from multiple sources (Kubernetes, monitoring, historical, business, external)
2. **Environment Classification**: Classify environment using flexible business rules and multiple sources
3. **Business Priority Assignment**: Calculate dynamic priority based on business criticality and configurable factors
4. **Routing Decision**: Determine next processing stage based on classification and priority
5. **Status Management**: Update parent AlertRemediation CRD with processing progress
6. **Configuration Management**: Support dynamic rule updates via ConfigMap

### **Reconciliation Logic**
```go
func (r *AlertProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Get AlertProcessing resource
    // 2. Phase: Enriching - Gather context from configured sources
    // 3. Phase: Classifying - Apply flexible business rules for environment classification
    // 4. Phase: Prioritizing - Calculate dynamic business priority
    // 5. Phase: Routing - Make intelligent routing decision
    // 6. Update parent AlertRemediation status
    // 7. Create next CRD in processing chain (AIAnalysis)
    // 8. Return appropriate requeue strategy
}
```

### **Configuration Management**
- **ConfigMap Watching**: Monitor ConfigMap changes for dynamic rule updates
- **Rule Validation**: Validate business rules before applying
- **Fallback Handling**: Graceful degradation when external sources are unavailable
- **Cache Management**: Efficient caching of classification rules and context data

---

## 🎯 **KEY DESIGN FEATURES**

### **1. Completely Flexible Environment Classification**
- **No Hardcoded Assumptions**: Supports unlimited custom environments
- **Multiple Classification Sources**: Labels, annotations, patterns, ConfigMap, external APIs
- **Hierarchical Support**: Complex organizational naming patterns (e.g., "prod-finance-web-frontend")
- **Priority-Based Conflict Resolution**: Configurable priority for rule precedence
- **Confidence Scoring**: Classification confidence for debugging and validation

### **2. Dynamic Business Priority Calculation**
- **Business Criticality Based**: Priority calculation based on business impact, not environment names
- **Configurable Multipliers**: All priority factors are configurable
- **Environment-Specific Overrides**: Custom multipliers with business justification
- **Multiple Calculation Methods**: Multiplicative, additive, weighted approaches
- **Business Hours Awareness**: Timezone and business hours consideration

### **3. Enterprise-Ready Configuration**
- **ConfigMap Integration**: External rule configuration for dynamic updates
- **Multi-Source Validation**: Combine multiple classification sources
- **SLA Requirements Mapping**: Environment-specific SLA requirements
- **Metadata Support**: Additional environment-specific metadata
- **Audit Trail**: Complete processing history with detailed reasoning

### **4. Cloud-Native Integration**
- **Kubernetes Standards**: Proper conditions, status, printer columns
- **Watch-Based Updates**: Efficient ConfigMap watching for rule updates
- **RBAC Support**: Proper security model with minimal required permissions
- **Observability**: Comprehensive metrics and logging

### **5. Performance Optimization**
- **Efficient Classification**: <100ms classification time target
- **Context Caching**: Intelligent caching of enrichment data
- **Parallel Processing**: Concurrent enrichment from multiple sources
- **Resource Management**: Configurable resource limits and timeouts

---

## 📊 **OPERATIONAL CHARACTERISTICS**

### **Performance Targets**
- **Total Processing Time**: <3 seconds for comprehensive enrichment
- **Classification Time**: <100ms for environment classification
- **Priority Calculation**: <50ms for business priority assignment
- **Context Enrichment**: <2 seconds for multi-source enrichment
- **ConfigMap Updates**: <5 seconds for rule reload

### **Scalability**
- **Concurrent Processing**: Support 500+ concurrent alert processing
- **Rule Complexity**: Support 100+ environment mappings
- **Context Sources**: Support 10+ concurrent enrichment sources
- **ConfigMap Size**: Support up to 1MB classification rules

### **Reliability**
- **Fallback Classification**: Always provide environment classification
- **Graceful Degradation**: Continue processing with partial context
- **Error Recovery**: Automatic retry with exponential backoff
- **Configuration Validation**: Validate rules before applying

---

## 🔒 **SECURITY CONSIDERATIONS**

### **RBAC Requirements**
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alertprocessing-controller
rules:
- apiGroups: ["kubernaut.io"]
  resources: ["alertremediations"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["alertprocessor.kubernaut.io"]
  resources: ["alertprocessings"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["alertprocessor.kubernaut.io"]
  resources: ["alertprocessings/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["ai.kubernaut.io"]
  resources: ["aianalyses"]
  verbs: ["create"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods", "services", "nodes"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

### **Data Protection**
- **Sensitive Context**: Alert payloads and context may contain sensitive information
- **Access Control**: Proper RBAC for resource and ConfigMap access
- **Audit Logging**: Complete audit trail for compliance
- **Configuration Security**: Secure handling of external API credentials

---

## 📋 **IMPLEMENTATION CHECKLIST**

### **Phase 1: Core Processing (Week 3-4)**
- [ ] CRD definition and installation
- [ ] Basic controller scaffolding with phase management
- [ ] Alert enrichment from Kubernetes sources
- [ ] Basic environment classification using patterns

### **Phase 2: Flexible Classification (Week 4-5)**
- [ ] ConfigMap-based rule configuration
- [ ] Multiple classification source support
- [ ] Hierarchical environment classification
- [ ] Dynamic business priority calculation

### **Phase 3: Advanced Features (Week 5-6)**
- [ ] Multi-source context enrichment
- [ ] Business hours and timezone support
- [ ] Environment-specific overrides
- [ ] Performance optimization and caching

### **Phase 4: Production Readiness (Week 6-7)**
- [ ] Comprehensive error handling
- [ ] Monitoring and metrics
- [ ] Security hardening
- [ ] Documentation and runbooks

---

## 🔗 **RELATED DOCUMENTS**

- [AlertRemediation CRD Design](./01_ALERT_REMEDIATION_CRD.md) *(Parent CRD)*
- [Kubernaut CRD Architecture](../../architecture/KUBERNAUT_CRD_ARCHITECTURE.md) (Authoritative)
- ~~[Multi-CRD Reconciliation Architecture](../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)~~ (DEPRECATED)
- [Environment Classification Requirements](../requirements/16_ENVIRONMENT_CLASSIFICATION_NAMESPACE_MANAGEMENT.md)
- [AIAnalysis CRD Design](./03_AI_ANALYSIS_CRD.md) *(Next)*
- [WorkflowExecution CRD Design](./04_WORKFLOW_EXECUTION_CRD.md) *(Planned)*
- [KubernetesExecution CRD Design](./05_KUBERNETES_EXECUTION_CRD.md) *(DEPRECATED - ADR-025)*

---

**Document Status**: ✅ **APPROVED**
**Implementation Priority**: **P0 - CRITICAL**
**Next Steps**: Proceed with AIAnalysis CRD design


