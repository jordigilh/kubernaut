# ⛔ DEPRECATED - DO NOT USE FOR IMPLEMENTATION

**Status**: 🚨 **HISTORICAL REFERENCE ONLY**
**Deprecated**: January 2025
**Confidence**: 100% - This document is ~60% incorrect

---

## 🎯 **FOR CURRENT V1 INFORMATION, SEE:**

### **1. Implementation (Source of Truth)**
- **[`api/workflowexecution/v1alpha1/workflowexecution_types.go`](../../../api/workflowexecution/v1alpha1/workflowexecution_types.go)** - Actual Go implementation

### **2. Schema Documentation**
- **[`docs/architecture/CRD_SCHEMAS.md`](../../architecture/CRD_SCHEMAS.md)** - Authoritative schema documentation

### **3. Service Specifications (~2,000 lines)**
- **[`docs/services/crd-controllers/03-workflowexecution/`](../../services/crd-controllers/03-workflowexecution/)** - Complete service specs

---

## ⚠️ **CRITICAL ISSUES IN THIS DOCUMENT**

**This document is ~60% incorrect for V1. Missing critical validation and dependency patterns!**

| Issue | Severity | What's Wrong |
|-------|----------|--------------|
| **API Group** | 🟡 MEDIUM | `workflow.kubernaut.io` → Should be `workflowexecution.kubernaut.io` |
| **API Version** | 🟡 MEDIUM | `v1` → Should be `v1alpha1` |
| **Parent Reference** | 🔴 HIGH | Likely `aiAnalysisRef` → Should be `remediationRequestRef` |
| **Validation Pattern** | 🔴 HIGH | Missing ADR-016 validation responsibility chain (relies on step status, no direct K8s validation) |
| **Dependency Analysis** | 🔴 HIGH | Missing dynamic execution mode determination (sequential vs parallel based on DAG analysis) |
| **Context API Usage** | 🔴 HIGH | Document likely shows Context API queries - V1 uses AI recommendations as authoritative |

**Schema Completeness**: ~40% of V1 patterns documented

**Missing V1 Patterns**:
- Validation Responsibility Chain (ADR-016)
- Dependency graph analysis for execution ordering
- Topological sort for DAG linearization
- Dynamic sequential/parallel execution determination

**See**: `docs/architecture/decisions/ADR-016-validation-responsibility-chain.md`

---

## 📜 **ORIGINAL DOCUMENT (OUTDATED) BELOW**

**Warning**: Everything below this line is outdated. See links above for current information.

---

# WorkflowExecution CRD Design Document

**Document Version**: 1.0
**Date**: January 2025
**Status**: **DEPRECATED** - See banner above (was: "APPROVED")
**CRD Version**: V1alpha1
**Module**: Workflow Service (`workflowexecution.kubernaut.io`)

---

## 🎯 **Purpose & Scope**

### **Business Purpose**
The `WorkflowExecution CRD` manages the execution of AI-generated remediation workflows, providing sophisticated orchestration, dependency management, and execution control for complex multi-step remediation scenarios. This CRD serves as the orchestration engine that coordinates between multiple executor services to achieve comprehensive alert remediation.

### **V1 Scope**
- **Multi-Step Workflow Execution**: Execute complex remediation workflows with conditional logic
- **Dependency Management**: Automatic dependency resolution and execution ordering
- **Progress Tracking**: Real-time workflow progress monitoring and milestone tracking
- **Timeout Management**: Configurable workflow and step-level timeout enforcement
- **Rollback Coordination**: Comprehensive rollback and compensation mechanisms
- **State Persistence**: Workflow state management for pause/resume operations
- **Executor Integration**: Seamless integration with Kubernetes and monitoring executors

---

## 📋 **Business Requirements Addressed**

### **Core Workflow Execution Requirements**
- **BR-WF-001**: MUST execute complex multi-step remediation workflows reliably
- **BR-WF-002**: MUST support conditional logic and branching within workflows
- **BR-WF-003**: MUST implement parallel and sequential execution patterns
- **BR-WF-004**: MUST provide workflow state management and persistence
- **BR-WF-005**: MUST support workflow pause, resume, and cancellation operations

### **Timeout & Lifecycle Management Requirements**
- **BR-WF-TIMEOUT-001**: MUST implement configurable workflow execution timeouts based on complexity
  - Simple workflows (1-3 steps): 10 minutes maximum
  - Medium workflows (4-10 steps): 20 minutes maximum
  - Complex workflows (11+ steps): 45 minutes maximum
- **BR-WF-TIMEOUT-002**: MUST provide step-level timeout configuration and enforcement
- **BR-WF-TIMEOUT-003**: MUST implement workflow deadline propagation to all sub-steps
- **BR-WF-TIMEOUT-004**: MUST support dynamic timeout adjustment based on execution progress
- **BR-WF-TIMEOUT-005**: MUST implement graceful workflow termination on timeout expiration

### **Lifecycle Management Requirements**
- **BR-WF-LIFECYCLE-001**: MUST track workflow execution phases (queued, running, paused, completed, failed, cancelled)
- **BR-WF-LIFECYCLE-002**: MUST provide workflow progress reporting with milestone tracking
- **BR-WF-LIFECYCLE-003**: MUST implement workflow heartbeat monitoring to detect stuck executions
- **BR-WF-LIFECYCLE-004**: MUST support workflow execution prioritization and scheduling
- **BR-WF-LIFECYCLE-005**: MUST provide workflow dependency chain visualization and management

### **Multi-Stage Processing Requirements**
- **BR-WF-017**: MUST process AI-generated JSON workflow responses with primary and secondary actions
- **BR-WF-018**: MUST execute conditional action sequences based on primary action outcomes
- **BR-WF-019**: MUST preserve context across multiple remediation stages
- **BR-WF-020**: MUST support execution conditions (if_primary_fails, after_primary, parallel_with_primary)
- **BR-WF-021**: MUST implement dynamic monitoring based on AI-defined success criteria
- **BR-WF-022**: MUST execute rollback actions when AI-defined triggers are met

### **Dependency Management Requirements**
- **BR-DEP-001**: MUST identify and resolve workflow step dependencies automatically
- **BR-DEP-002**: MUST handle circular dependency detection and prevention
- **BR-DEP-011**: MUST coordinate execution order based on dependency graphs
- **BR-DEP-012**: MUST optimize parallel execution while respecting dependencies
- **BR-DEP-013**: MUST handle dependency failures and provide alternatives
- **BR-DEP-014**: MUST implement dependency-aware rollback strategies

### **Post-Condition Validation Requirements**
- **BR-WF-050**: MUST implement post-condition checks for each workflow step
- **BR-WF-051**: MUST support custom validation rules and business logic
- **BR-WF-052**: MUST provide condition evaluation metrics and reporting

### **Action Execution Framework Requirements**
- **BR-WF-011**: MUST support custom action executors for specialized operations
- **BR-WF-012**: MUST integrate Kubernetes action executors seamlessly
- **BR-WF-013**: MUST provide monitoring action executors for health checks
- **BR-WF-014**: MUST implement action retry mechanisms with configurable strategies
- **BR-WF-015**: MUST support action rollback and compensation patterns

---

## 🏗️ **CRD Specification**

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: workflowexecutions.workflow.kubernaut.io
  annotations:
    controller-gen.kubebuilder.io/version: v0.13.0
    kubernaut.io/description: "Workflow execution CRD for AI-generated remediation workflows"
    kubernaut.io/business-requirements: "BR-WF-001,BR-WF-002,BR-WF-003,BR-WF-004,BR-WF-005,BR-WF-TIMEOUT-001,BR-WF-LIFECYCLE-001"
spec:
  group: workflow.kubernaut.io
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
            - workflowDefinition
            - executionConfig
            properties:
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

              aiAnalysisRef:
                type: object
                required:
                - name
                - namespace
                properties:
                  name:
                    type: string
                    description: "Name of the AIAnalysis resource that generated this workflow"
                  namespace:
                    type: string
                    description: "Namespace of the AIAnalysis resource"

              workflowDefinition:
                type: object
                required:
                - steps
                - metadata
                properties:
                  metadata:
                    type: object
                    properties:
                      workflowId:
                        type: string
                        description: "Unique workflow identifier"
                      workflowType:
                        type: string
                        enum: [simple, medium, complex]
                        description: "Workflow complexity classification (BR-WF-TIMEOUT-001)"
                      generatedBy:
                        type: string
                        enum: [ai-analysis, template, manual]
                        description: "Source of workflow generation"
                      confidence:
                        type: number
                        minimum: 0.0
                        maximum: 1.0
                        description: "AI confidence in workflow effectiveness"
                      estimatedDuration:
                        type: string
                        pattern: "^[0-9]+(s|m|h)$"
                        description: "Estimated execution duration"
                      tags:
                        type: array
                        items:
                          type: string
                        description: "Workflow classification tags"

                  steps:
                    type: array
                    minItems: 1
                    items:
                      type: object
                      required:
                      - id
                      - name
                      - actionType
                      properties:
                        id:
                          type: string
                          description: "Unique step identifier within workflow"
                        name:
                          type: string
                          description: "Human-readable step name"
                        description:
                          type: string
                          description: "Step description and purpose"

                        actionType:
                          type: string
                          enum: [kubernetes, monitoring, notification, validation, custom]
                          description: "Type of action to execute"

                        actionConfig:
                          type: object
                          description: "Action-specific configuration"
                          properties:
                            executor:
                              type: string
                              description: "Executor service to handle this action"
                            parameters:
                              type: object
                              additionalProperties: true
                              description: "Parameters for action execution"
                            timeout:
                              type: string
                              pattern: "^[0-9]+(s|m|h)$"
                              default: "5m"
                              description: "Step-level timeout (BR-WF-TIMEOUT-002)"
                            retryPolicy:
                              type: object
                              properties:
                                maxRetries:
                                  type: integer
                                  minimum: 0
                                  maximum: 5
                                  default: 2
                                backoffMultiplier:
                                  type: number
                                  minimum: 1.0
                                  default: 2.0
                                retryableErrors:
                                  type: array
                                  items:
                                    type: string
                                  description: "Error patterns that trigger retry"

                        executionConditions:
                          type: object
                          properties:
                            runIf:
                              type: string
                              description: "Condition expression for step execution (BR-WF-002)"
                            skipIf:
                              type: string
                              description: "Condition expression to skip step"
                            dependsOn:
                              type: array
                              items:
                                type: string
                              description: "Step IDs this step depends on (BR-DEP-001)"
                            executionMode:
                              type: string
                              enum: [sequential, parallel, conditional]
                              default: "sequential"
                              description: "Execution mode relative to dependencies (BR-WF-003)"

                        validationRules:
                          type: object
                          properties:
                            preConditions:
                              type: array
                              items:
                                type: object
                                properties:
                                  condition:
                                    type: string
                                    description: "Pre-execution condition"
                                  errorMessage:
                                    type: string
                                    description: "Error message if condition fails"
                            postConditions:
                              type: array
                              items:
                                type: object
                                properties:
                                  condition:
                                    type: string
                                    description: "Post-execution validation (BR-WF-050)"
                                  successCriteria:
                                    type: string
                                    description: "Success criteria for step completion"
                                  failureAction:
                                    type: string
                                    enum: [retry, skip, fail-workflow, rollback]
                                    description: "Action to take if validation fails"

                        rollbackConfig:
                          type: object
                          properties:
                            rollbackAction:
                              type: object
                              description: "Action to execute for rollback (BR-WF-015)"
                            rollbackConditions:
                              type: array
                              items:
                                type: string
                              description: "Conditions that trigger rollback"
                            compensationSteps:
                              type: array
                              items:
                                type: string
                              description: "Additional steps for compensation"

                  globalConfig:
                    type: object
                    properties:
                      variables:
                        type: object
                        additionalProperties: true
                        description: "Global workflow variables (BR-WF-007)"

                      timeoutConfig:
                        type: object
                        properties:
                          workflowTimeout:
                            type: string
                            pattern: "^[0-9]+(s|m|h)$"
                            description: "Overall workflow timeout (BR-WF-TIMEOUT-001)"
                          deadlinePropagate:
                            type: boolean
                            default: true
                            description: "Propagate deadline to all steps (BR-WF-TIMEOUT-003)"
                          dynamicAdjustment:
                            type: boolean
                            default: true
                            description: "Enable dynamic timeout adjustment (BR-WF-TIMEOUT-004)"

                      executionPolicy:
                        type: object
                        properties:
                          failureStrategy:
                            type: string
                            enum: [fail-fast, continue-on-error, retry-failed]
                            default: "fail-fast"
                            description: "Strategy for handling step failures"
                          parallelismLimit:
                            type: integer
                            minimum: 1
                            maximum: 10
                            default: 3
                            description: "Maximum parallel step executions"
                          resourceLimits:
                            type: object
                            properties:
                              maxCpuCores:
                                type: string
                                default: "2"
                              maxMemoryGb:
                                type: string
                                default: "4"

              executionConfig:
                type: object
                properties:
                  priority:
                    type: string
                    enum: [low, normal, high, critical]
                    default: "normal"
                    description: "Execution priority (BR-WF-LIFECYCLE-004)"

                  scheduling:
                    type: object
                    properties:
                      startTime:
                        type: string
                        format: date-time
                        description: "Scheduled start time (optional)"
                      maxDelay:
                        type: string
                        pattern: "^[0-9]+(s|m|h)$"
                        default: "5m"
                        description: "Maximum delay before execution starts"

                  monitoring:
                    type: object
                    properties:
                      enableHeartbeat:
                        type: boolean
                        default: true
                        description: "Enable heartbeat monitoring (BR-WF-LIFECYCLE-003)"
                      heartbeatInterval:
                        type: string
                        pattern: "^[0-9]+(s|m)$"
                        default: "30s"
                        description: "Heartbeat check interval"
                      progressReporting:
                        type: boolean
                        default: true
                        description: "Enable progress reporting (BR-WF-LIFECYCLE-002)"
                      milestoneTracking:
                        type: boolean
                        default: true
                        description: "Enable milestone tracking"

                  notifications:
                    type: object
                    properties:
                      onStart:
                        type: array
                        items:
                          type: string
                        description: "Notification channels for workflow start"
                      onCompletion:
                        type: array
                        items:
                          type: string
                        description: "Notification channels for workflow completion"
                      onFailure:
                        type: array
                        items:
                          type: string
                        description: "Notification channels for workflow failure"
                      onTimeout:
                        type: array
                        items:
                          type: string
                        description: "Notification channels for workflow timeout"

          status:
            type: object
            properties:
              phase:
                type: string
                enum: [queued, running, paused, completed, failed, cancelled, timeout]
                description: "Current workflow execution phase (BR-WF-LIFECYCLE-001)"

              executionSummary:
                type: object
                properties:
                  totalSteps:
                    type: integer
                    minimum: 0
                    description: "Total number of steps in workflow"
                  completedSteps:
                    type: integer
                    minimum: 0
                    description: "Number of completed steps"
                  failedSteps:
                    type: integer
                    minimum: 0
                    description: "Number of failed steps"
                  skippedSteps:
                    type: integer
                    minimum: 0
                    description: "Number of skipped steps"
                  currentStep:
                    type: string
                    description: "ID of currently executing step"
                  overallProgress:
                    type: integer
                    minimum: 0
                    maximum: 100
                    description: "Overall progress percentage"
                  estimatedTimeRemaining:
                    type: string
                    description: "Estimated time remaining (e.g., '5m30s')"

              stepStatuses:
                type: array
                items:
                  type: object
                  properties:
                    stepId:
                      type: string
                      description: "Step identifier"
                    stepName:
                      type: string
                      description: "Step name"
                    phase:
                      type: string
                      enum: [pending, running, completed, failed, skipped, cancelled]
                      description: "Step execution phase"
                    startTime:
                      type: string
                      format: date-time
                    completionTime:
                      type: string
                      format: date-time
                    duration:
                      type: string
                      description: "Step execution duration"

                    executionResults:
                      type: object
                      properties:
                        exitCode:
                          type: integer
                          description: "Step exit code"
                        output:
                          type: string
                          description: "Step execution output"
                        error:
                          type: string
                          description: "Error message if step failed"
                        metrics:
                          type: object
                          additionalProperties: true
                          description: "Step-specific metrics"

                    validationResults:
                      type: object
                      properties:
                        preConditionsPassed:
                          type: boolean
                          description: "Whether pre-conditions passed"
                        postConditionsPassed:
                          type: boolean
                          description: "Whether post-conditions passed (BR-WF-050)"
                        validationErrors:
                          type: array
                          items:
                            type: string
                          description: "Validation error messages"

                    retryInfo:
                      type: object
                      properties:
                        attemptCount:
                          type: integer
                          minimum: 0
                          description: "Number of retry attempts"
                        lastRetryTime:
                          type: string
                          format: date-time
                        nextRetryTime:
                          type: string
                          format: date-time
                        retryReason:
                          type: string
                          description: "Reason for retry"

              dependencyGraph:
                type: object
                properties:
                  resolved:
                    type: boolean
                    description: "Whether dependency graph is resolved (BR-DEP-001)"
                  dependencies:
                    type: array
                    items:
                      type: object
                      properties:
                        stepId:
                          type: string
                        dependsOn:
                          type: array
                          items:
                            type: string
                        dependencyType:
                          type: string
                          enum: [hard, soft, conditional]
                        satisfied:
                          type: boolean
                  circularDependencies:
                    type: array
                    items:
                      type: string
                    description: "Detected circular dependencies (BR-DEP-002)"

              executionMetrics:
                type: object
                properties:
                  startTime:
                    type: string
                    format: date-time
                  completionTime:
                    type: string
                    format: date-time
                  totalDuration:
                    type: string
                    description: "Total execution duration"

                  resourceUsage:
                    type: object
                    properties:
                      peakCpuUsage:
                        type: string
                        description: "Peak CPU usage during execution"
                      peakMemoryUsage:
                        type: string
                        description: "Peak memory usage during execution"
                      totalCpuTime:
                        type: string
                        description: "Total CPU time consumed"

                  performanceMetrics:
                    type: object
                    properties:
                      averageStepDuration:
                        type: string
                        description: "Average step execution duration"
                      longestStepDuration:
                        type: string
                        description: "Duration of longest-running step"
                      parallelEfficiency:
                        type: number
                        minimum: 0.0
                        maximum: 1.0
                        description: "Parallel execution efficiency score"
                      timeoutOccurred:
                        type: boolean
                        description: "Whether any timeouts occurred"

              heartbeatStatus:
                type: object
                properties:
                  lastHeartbeat:
                    type: string
                    format: date-time
                    description: "Last heartbeat timestamp (BR-WF-LIFECYCLE-003)"
                  heartbeatInterval:
                    type: string
                    description: "Current heartbeat interval"
                  missedHeartbeats:
                    type: integer
                    minimum: 0
                    description: "Number of consecutive missed heartbeats"
                  healthStatus:
                    type: string
                    enum: [healthy, degraded, unhealthy]
                    description: "Overall workflow health status"

              rollbackStatus:
                type: object
                properties:
                  rollbackTriggered:
                    type: boolean
                    description: "Whether rollback was triggered"
                  rollbackReason:
                    type: string
                    description: "Reason for rollback initiation"
                  rollbackSteps:
                    type: array
                    items:
                      type: object
                      properties:
                        stepId:
                          type: string
                        rollbackAction:
                          type: string
                        rollbackStatus:
                          type: string
                          enum: [pending, running, completed, failed]
                        rollbackTime:
                          type: string
                          format: date-time

              lastReconciled:
                type: string
                format: date-time
              error:
                type: string
                description: "Error message if workflow failed"

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
                      description: "Condition type (e.g., 'Ready', 'DependenciesResolved', 'Executing', 'Completed')"
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
      description: Current execution phase
      jsonPath: .status.phase
    - name: Progress
      type: string
      description: Overall progress percentage
      jsonPath: .status.executionSummary.overallProgress
    - name: Current-Step
      type: string
      description: Currently executing step
      jsonPath: .status.executionSummary.currentStep
    - name: Steps
      type: string
      description: Completed/Total steps
      jsonPath: .status.executionSummary.completedSteps/.status.executionSummary.totalSteps
    - name: Duration
      type: string
      description: Total execution duration
      jsonPath: .status.executionMetrics.totalDuration
    - name: Priority
      type: string
      description: Execution priority
      jsonPath: .spec.executionConfig.priority
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp

    subresources:
      status: {}

  scope: Namespaced
  names:
    plural: workflowexecutions
    singular: workflowexecution
    kind: WorkflowExecution
    shortNames:
    - wfe
    - workflow
    categories:
    - kubernaut
    - workflow
    - execution
```

---

## 📝 **Example Custom Resource**

```yaml
apiVersion: workflow.kubernaut.io/v1
kind: WorkflowExecution
metadata:
  name: workflow-execution-high-cpu-alert-abc123
  namespace: kubernaut-system
  labels:
    kubernaut.io/remediation: "high-cpu-alert-abc123"
    kubernaut.io/environment: "production"
    kubernaut.io/priority: "p1"
spec:
  alertRemediationRef:
    name: "high-cpu-alert-abc123"
    namespace: "kubernaut-system"

  aiAnalysisRef:
    name: "ai-analysis-high-cpu-alert-abc123"
    namespace: "kubernaut-system"

  workflowDefinition:
    metadata:
      workflowId: "wf-cpu-remediation-v1.2"
      workflowType: "medium"
      generatedBy: "ai-analysis"
      confidence: 0.88
      estimatedDuration: "8m"
      tags: ["cpu-remediation", "pod-restart", "production"]

    steps:
    - id: "validate-environment"
      name: "Validate Production Environment"
      description: "Ensure production environment is stable for remediation"
      actionType: "validation"
      actionConfig:
        executor: "validation-service"
        parameters:
          environment: "production"
          namespace: "production-web"
          checkLoadBalancer: true
        timeout: "2m"
        retryPolicy:
          maxRetries: 1
          backoffMultiplier: 1.5
      executionConditions:
        executionMode: "sequential"
      validationRules:
        preConditions:
        - condition: "environment == 'production'"
          errorMessage: "Environment validation failed"
        postConditions:
        - condition: "loadBalancerHealthy == true"
          successCriteria: "Load balancer is healthy"
          failureAction: "fail-workflow"

    - id: "backup-current-state"
      name: "Backup Current Pod State"
      description: "Create backup of current pod configuration"
      actionType: "kubernetes"
      actionConfig:
        executor: "kubernetes-executor"  # DEPRECATED - ADR-025
        parameters:
          action: "backup-pod"
          namespace: "production-web"
          podName: "web-server-1-7d8f9c-xyz"
        timeout: "3m"
      executionConditions:
        dependsOn: ["validate-environment"]
        executionMode: "sequential"
      rollbackConfig:
        rollbackAction:
          action: "restore-pod"
          parameters:
            backupId: "${backup.id}"

    - id: "restart-pod"
      name: "Restart High CPU Pod"
      description: "Restart the pod experiencing high CPU usage"
      actionType: "kubernetes"
      actionConfig:
        executor: "kubernetes-executor"  # DEPRECATED - ADR-025
        parameters:
          action: "restart-pod"
          namespace: "production-web"
          podName: "web-server-1-7d8f9c-xyz"
          gracefulShutdown: true
        timeout: "5m"
        retryPolicy:
          maxRetries: 2
          backoffMultiplier: 2.0
      executionConditions:
        dependsOn: ["backup-current-state"]
        executionMode: "sequential"
        runIf: "backup.success == true"
      validationRules:
        postConditions:
        - condition: "pod.status == 'Running'"
          successCriteria: "Pod is running and healthy"
          failureAction: "rollback"
      rollbackConfig:
        rollbackConditions: ["pod.status != 'Running'", "timeout"]
        compensationSteps: ["restore-pod", "notify-team"]

    - id: "monitor-recovery"
      name: "Monitor CPU Recovery"
      description: "Monitor CPU usage to confirm remediation success"
      actionType: "monitoring"
      actionConfig:
        executor: "monitoring-service"
        parameters:
          metric: "cpu_usage_percent"
          namespace: "production-web"
          podName: "web-server-1-7d8f9c-xyz"
          threshold: 70
          duration: "3m"
        timeout: "5m"
      executionConditions:
        dependsOn: ["restart-pod"]
        executionMode: "sequential"
      validationRules:
        postConditions:
        - condition: "cpu_usage < 70"
          successCriteria: "CPU usage below 70%"
          failureAction: "retry"

    - id: "notify-completion"
      name: "Notify Remediation Completion"
      description: "Send notification about successful remediation"
      actionType: "notification"
      actionConfig:
        executor: "notification-service"
        parameters:
          channels: ["slack", "email"]
          message: "High CPU alert remediated successfully"
          severity: "info"
        timeout: "1m"
      executionConditions:
        dependsOn: ["monitor-recovery"]
        executionMode: "sequential"

    globalConfig:
      variables:
        environment: "production"
        namespace: "production-web"
        alertId: "high-cpu-alert-abc123"

      timeoutConfig:
        workflowTimeout: "15m"
        deadlinePropagate: true
        dynamicAdjustment: true

      executionPolicy:
        failureStrategy: "fail-fast"
        parallelismLimit: 2
        resourceLimits:
          maxCpuCores: "1"
          maxMemoryGb: "2"

  executionConfig:
    priority: "high"

    monitoring:
      enableHeartbeat: true
      heartbeatInterval: "30s"
      progressReporting: true
      milestoneTracking: true

    notifications:
      onStart: ["slack://ops-channel"]
      onCompletion: ["slack://ops-channel", "email://team@company.com"]
      onFailure: ["slack://alerts-channel", "email://oncall@company.com"]
      onTimeout: ["slack://alerts-channel", "email://oncall@company.com"]

status:
  phase: "completed"

  executionSummary:
    totalSteps: 5
    completedSteps: 5
    failedSteps: 0
    skippedSteps: 0
    currentStep: ""
    overallProgress: 100
    estimatedTimeRemaining: "0s"

  stepStatuses:
  - stepId: "validate-environment"
    stepName: "Validate Production Environment"
    phase: "completed"
    startTime: "2025-01-15T10:35:00Z"
    completionTime: "2025-01-15T10:36:30Z"
    duration: "1m30s"
    executionResults:
      exitCode: 0
      output: "Environment validation successful"
      metrics:
        loadBalancerHealth: "healthy"
    validationResults:
      preConditionsPassed: true
      postConditionsPassed: true

  - stepId: "backup-current-state"
    stepName: "Backup Current Pod State"
    phase: "completed"
    startTime: "2025-01-15T10:36:30Z"
    completionTime: "2025-01-15T10:38:00Z"
    duration: "1m30s"
    executionResults:
      exitCode: 0
      output: "Pod backup created successfully"
      metrics:
        backupId: "backup-20250115-103800"

  - stepId: "restart-pod"
    stepName: "Restart High CPU Pod"
    phase: "completed"
    startTime: "2025-01-15T10:38:00Z"
    completionTime: "2025-01-15T10:41:00Z"
    duration: "3m"
    executionResults:
      exitCode: 0
      output: "Pod restarted successfully"
      metrics:
        podRestartTime: "45s"
    validationResults:
      postConditionsPassed: true

  - stepId: "monitor-recovery"
    stepName: "Monitor CPU Recovery"
    phase: "completed"
    startTime: "2025-01-15T10:41:00Z"
    completionTime: "2025-01-15T10:44:00Z"
    duration: "3m"
    executionResults:
      exitCode: 0
      output: "CPU usage normalized to 35%"
      metrics:
        finalCpuUsage: 35.2
    validationResults:
      postConditionsPassed: true

  - stepId: "notify-completion"
    stepName: "Notify Remediation Completion"
    phase: "completed"
    startTime: "2025-01-15T10:44:00Z"
    completionTime: "2025-01-15T10:44:30Z"
    duration: "30s"
    executionResults:
      exitCode: 0
      output: "Notifications sent successfully"

  dependencyGraph:
    resolved: true
    dependencies:
    - stepId: "backup-current-state"
      dependsOn: ["validate-environment"]
      dependencyType: "hard"
      satisfied: true
    - stepId: "restart-pod"
      dependsOn: ["backup-current-state"]
      dependencyType: "hard"
      satisfied: true
    - stepId: "monitor-recovery"
      dependsOn: ["restart-pod"]
      dependencyType: "hard"
      satisfied: true
    - stepId: "notify-completion"
      dependsOn: ["monitor-recovery"]
      dependencyType: "hard"
      satisfied: true
    circularDependencies: []

  executionMetrics:
    startTime: "2025-01-15T10:35:00Z"
    completionTime: "2025-01-15T10:44:30Z"
    totalDuration: "9m30s"
    resourceUsage:
      peakCpuUsage: "0.8"
      peakMemoryUsage: "1.2Gi"
      totalCpuTime: "2m15s"
    performanceMetrics:
      averageStepDuration: "1m54s"
      longestStepDuration: "3m"
      parallelEfficiency: 0.95
      timeoutOccurred: false

  heartbeatStatus:
    lastHeartbeat: "2025-01-15T10:44:30Z"
    heartbeatInterval: "30s"
    missedHeartbeats: 0
    healthStatus: "healthy"

  rollbackStatus:
    rollbackTriggered: false

  lastReconciled: "2025-01-15T10:44:30Z"

  conditions:
  - type: "Ready"
    status: "True"
    reason: "WorkflowCompleted"
    message: "Workflow executed successfully with all steps completed"
    lastTransitionTime: "2025-01-15T10:44:30Z"
  - type: "DependenciesResolved"
    status: "True"
    reason: "AllDependenciesResolved"
    message: "All step dependencies resolved successfully"
    lastTransitionTime: "2025-01-15T10:35:00Z"
  - type: "Executing"
    status: "False"
    reason: "ExecutionCompleted"
    message: "Workflow execution completed"
    lastTransitionTime: "2025-01-15T10:44:30Z"
```

---

## 🎛️ **Controller Responsibilities**

### **Primary Functions**
1. **Workflow Orchestration**: Execute multi-step workflows with sophisticated dependency management
2. **Step Execution Coordination**: Coordinate individual step execution with appropriate executor services
3. **Progress Tracking**: Monitor and report real-time workflow progress and milestone achievement
4. **Timeout Management**: Enforce workflow and step-level timeouts with dynamic adjustment
5. **Rollback Coordination**: Handle rollback scenarios and execute compensation actions
6. **State Persistence**: Maintain workflow state for pause/resume operations and recovery
7. **Dependency Resolution**: Automatically resolve step dependencies and optimize execution order

### **Reconciliation Logic**
```go
// WorkflowExecutionController reconciliation phases
func (r *WorkflowExecutionController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Phase 1: Dependency Resolution (10-30 seconds)
    // - Analyze workflow step dependencies
    // - Detect and prevent circular dependencies
    // - Build execution graph with parallel optimization

    // Phase 2: Execution Planning (5-15 seconds)
    // - Calculate execution timeline and resource requirements
    // - Apply timeout configurations and deadline propagation
    // - Initialize heartbeat monitoring and progress tracking

    // Phase 3: Step Execution (Variable: 2-45 minutes)
    // - Execute steps according to dependency graph
    // - Coordinate with executor services (Kubernetes, Monitoring, etc.)
    // - Monitor step progress and handle retries
    // - Validate post-conditions and handle failures

    // Phase 4: Progress Management (Continuous)
    // - Update execution progress and milestone tracking
    // - Handle heartbeat monitoring and health checks
    // - Manage timeout enforcement and dynamic adjustment

    // Phase 5: Completion/Rollback (30-120 seconds)
    // - Handle workflow completion or failure scenarios
    // - Execute rollback actions if required
    // - Update AlertRemediation status with execution results
    // - Trigger cleanup and audit data persistence
}
```

### **Integration Points**
- **Input**: Receives workflow definition from `AIAnalysis CRD`
- **Executor Services**: Coordinates with Kubernetes Executor, Monitoring Service, Notification Service
- **Output**: Creates `~~KubernetesExecution~~ (DEPRECATED - ADR-025) CRDs` for Kubernetes-specific actions
- **Parent Update**: Updates `AlertRemediation CRD` with workflow execution results
- **Audit**: Persists execution data for historical analysis and learning

---

## 📊 **Operational Characteristics**

### **Performance Metrics**
- **Processing Time**: 2-45 minutes depending on workflow complexity and step count
- **Startup Time**: <5 seconds from trigger to execution start (BR-PERF-001)
- **Progress Reporting**: Real-time updates every 30 seconds
- **Resource Usage**: Configurable CPU/memory limits per workflow
- **Concurrency**: Supports 100+ concurrent workflow executions (BR-PERF-006)

### **Reliability Features**
- **Dependency Management**: Automatic dependency resolution with circular dependency detection
- **Timeout Enforcement**: Multi-level timeout management with dynamic adjustment
- **Retry Logic**: Configurable retry policies with exponential backoff
- **Rollback Mechanisms**: Comprehensive rollback and compensation strategies
- **State Persistence**: Complete workflow state preservation for recovery
- **Heartbeat Monitoring**: Continuous health monitoring to detect stuck executions

### **Dependencies**
- **Required**: Kubernetes Executor Service, Monitoring Service, Notification Service
- **Input**: AI-generated workflow definition from AIAnalysis CRD
- **Configuration**: Executor service endpoints and timeout configurations
- **Storage**: Workflow state persistence and execution history

### **Scalability Considerations**
- **Horizontal Scaling**: Controller supports multiple replicas with leader election
- **Resource Management**: Per-workflow resource limits and global resource quotas
- **Queue Management**: Priority-based workflow scheduling and execution queuing
- **Load Distribution**: Intelligent load balancing across executor services

---

## 🔒 **Security & Compliance**

### **Data Protection**
- **Sensitive Data**: Workflow parameters may contain sensitive system information
- **Encryption**: All executor service communication over HTTPS/TLS
- **Access Control**: RBAC-based access to WorkflowExecution resources
- **Audit Logging**: Complete audit trail of all workflow execution activities

### **Execution Security**
- **Privilege Management**: Least-privilege execution for all workflow steps
- **Resource Isolation**: Isolated execution environments for workflow steps
- **Validation**: Comprehensive input validation and sanitization
- **Safety Checks**: Pre-execution safety validation for destructive actions

---

## 🚀 **Implementation Priority**

### **V1 Implementation (Current)**
- ✅ **Multi-Step Execution**: Complex workflow orchestration with dependency management
- ✅ **Timeout Management**: Configurable timeouts with dynamic adjustment
- ✅ **Progress Tracking**: Real-time progress monitoring and milestone tracking
- ✅ **Rollback Coordination**: Comprehensive rollback and compensation mechanisms
- ✅ **Executor Integration**: Seamless integration with Kubernetes and monitoring executors
- ✅ **State Persistence**: Complete workflow state management for pause/resume

### **V2 Future Enhancements** (Not Implemented)
- 🔄 **Advanced Scheduling**: Sophisticated workflow scheduling with resource optimization
- 🔄 **Multi-Cluster Coordination**: Cross-cluster workflow execution and coordination
- 🔄 **Workflow Templates**: Reusable workflow templates and composition
- 🔄 **Performance Optimization**: Advanced performance tuning and resource optimization
- 🔄 **Visual Workflow Builder**: Graphical workflow design and editing interface

---

## 📈 **Success Metrics**

### **Execution Metrics**
- **Workflow Success Rate**: >95% successful completion rate for generated workflows
- **Execution Time**: Meet complexity-based timeout requirements (10m/20m/45m)
- **Dependency Resolution**: 100% accuracy in dependency graph resolution
- **Rollback Effectiveness**: >90% successful rollback completion when triggered

### **Performance Metrics**
- **Startup Time**: <5 seconds from trigger to execution start
- **Progress Accuracy**: Real-time progress reporting with <30 second latency
- **Resource Efficiency**: Optimal resource utilization within configured limits
- **Concurrent Capacity**: Support 100+ concurrent workflow executions

### **Reliability Metrics**
- **Heartbeat Monitoring**: <1% false positive rate in stuck execution detection
- **Timeout Accuracy**: <5% variance in timeout enforcement accuracy
- **State Recovery**: 100% successful workflow recovery after controller restart
- **Error Handling**: Comprehensive error recovery with detailed status reporting

---

## 🔗 **Related Documentation**

- **Architecture**: [Kubernaut CRD Architecture](../../architecture/KUBERNAUT_CRD_ARCHITECTURE.md) (Authoritative)
- ~~[Multi-CRD Reconciliation Architecture](../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)~~ (DEPRECATED)
- **Requirements**: [Workflow Engine & Orchestration Requirements](../requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md)
- **Parent CRD**: [AlertRemediation CRD](01_ALERT_REMEDIATION_CRD.md)
- **Input CRD**: [AIAnalysis CRD](03_AI_ANALYSIS_CRD.md)
- **Next CRD**: [KubernetesExecution CRD](05_KUBERNETES_EXECUTION_CRD.md) (DEPRECATED - ADR-025)

---

**Status**: ✅ **APPROVED** - Ready for V1 Implementation
**Next Step**: ~~Proceed with KubernetesExecution CRD design~~ (DEPRECATED - ADR-025)

