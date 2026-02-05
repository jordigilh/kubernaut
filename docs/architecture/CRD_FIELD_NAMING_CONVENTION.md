# Kubernaut YAML Naming Convention - Universal Standard

**Version**: 1.1
**Last Updated**: January 30, 2026
**Status**: ✅ Authoritative Standard
**Scope**: All YAML configurations (CRDs, service configs, Kubernetes manifests)

---

## 📋 Changelog

### Version 1.1 - January 30, 2026

**Scope Expansion**: Extended from CRD-only to **ALL YAML configurations**

**Changes**:
- ✅ **Universal Application**: camelCase now MANDATORY for ALL YAML files:
  - CRD specs (existing scope)
  - Service configuration files (ADR-030 configs)
  - Kubernetes manifests
  - Test configurations
  - Production configs
- ✅ **ADR-030 Integration**: Service configuration management now references this standard
- ✅ **Consistency**: Single naming convention across entire platform

**Rationale**: 
Inconsistent naming (snake_case in service configs, camelCase in CRDs) caused confusion and maintenance burden. Standardizing to camelCase provides:
- Clean serialization for both JSON and YAML
- Consistency with Kubernetes ecosystem
- Alignment with Go struct field naming (PascalCase → camelCase in YAML)
- Single authoritative standard for all YAML files

**Authority**: This document is now the **SOLE** naming authority for all YAML configurations in Kubernaut.

### Version 1.0 - October 6, 2025

**Initial Release**: CRD field naming conventions established

---

## 📋 Table of Contents

1. [Overview](#overview)
2. [Naming Principles](#naming-principles)
3. [Field Naming Patterns](#field-naming-patterns)
4. [CRD-Specific Conventions](#crd-specific-conventions)
5. [Common Field Patterns](#common-field-patterns)
6. [Anti-Patterns](#anti-patterns)
7. [Migration Guide](#migration-guide)

---

## Overview

### Purpose

This document establishes **universal** naming conventions for ALL YAML configurations in Kubernaut, ensuring:
- ✅ **Consistency**: Uniform naming across entire platform
- ✅ **Clarity**: Self-documenting field names
- ✅ **Go Conventions**: Alignment with Go and Kubernetes standards
- ✅ **JSON/YAML Compatibility**: Clean serialization for both formats

**MANDATE**: ALL YAML files MUST use camelCase for field names (no exceptions).

---

### Scope (V1.1 - Expanded)

**ALL YAML Configurations**:
1. **Custom Resource Definitions (CRDs)** - All 5 Kubernaut CRDs:
   - RemediationRequest (Gateway Service)
   - RemediationProcessing (Remediation Processor)
   - AIAnalysis (AI Analysis Controller)
   - WorkflowExecution (Workflow Execution Controller)
   - KubernetesExecution (Kubernetes Executor)

2. **Service Configuration Files** (ADR-030):
   - Gateway config (`pkg/gateway/config/`)
   - DataStorage config (`pkg/datastorage/server/`)
   - RemediationOrchestrator config (`internal/config/remediationorchestrator.go`)
   - All other service configs

3. **Kubernetes Manifests**:
   - Deployments, Services, ConfigMaps
   - E2E test manifests (`test/infrastructure/`, `test/e2e/`)
   - Production manifests (`deploy/`)

4. **Test Configurations**:
   - Integration test configs
   - E2E test configs
   - Mock service configs

---

## Naming Principles

### Core Rules

#### 1. **Use PascalCase for Go Struct Fields**
```go
// ✅ CORRECT
type RemediationRequestSpec struct {
    SignalType    string `json:"signalType"`
    TargetType    string `json:"targetType"`
    ProviderData  json.RawMessage `json:"providerData"`
}

// ❌ WRONG
type RemediationRequestSpec struct {
    signal_type   string `json:"signal_type"`   // Wrong: snake_case
    targettype    string `json:"targettype"`    // Wrong: no separation
    providerdata  json.RawMessage `json:"providerdata"` // Wrong: no separation
}
```

---

#### 2. **Use camelCase for JSON/YAML Fields**
```go
// ✅ CORRECT
type RemediationRequestSpec struct {
    SignalType    string `json:"signalType"`    // camelCase in JSON
    CreatedAt     metav1.Time `json:"createdAt"` // camelCase in JSON
}

// ❌ WRONG
type RemediationRequestSpec struct {
    SignalType    string `json:"SignalType"`    // Wrong: PascalCase
    CreatedAt     metav1.Time `json:"created_at"` // Wrong: snake_case
}
```

---

#### 3. **Avoid Redundant Prefixes**
```go
// ✅ CORRECT
type RemediationRequestSpec struct {
    TargetName      string `json:"targetName"`
    TargetNamespace string `json:"targetNamespace"`
}

// ❌ WRONG
type RemediationRequestSpec struct {
    RemediationTargetName      string `json:"remediationTargetName"` // Redundant prefix
    RemediationTargetNamespace string `json:"remediationTargetNamespace"`
}
```

---

#### 4. **Use Clear, Descriptive Names**
```go
// ✅ CORRECT
type RemediationRequestSpec struct {
    OriginalPayload []byte `json:"originalPayload"` // Clear purpose
    Priority        string `json:"priority"`        // Clear meaning
}

// ❌ WRONG
type RemediationRequestSpec struct {
    Data []byte `json:"data"` // Too vague
    Pri  string `json:"pri"`  // Abbreviated
}
```

---

#### 5. **Use Kubernetes Naming Conventions for References**
```go
// ✅ CORRECT - Kubernetes pattern
type AIAnalysisSpec struct {
    RemediationRequestRef string `json:"remediationRequestRef"` // Ref suffix
}

// ❌ WRONG
type AIAnalysisSpec struct {
    RemediationRequestName string `json:"remediationRequestName"` // Inconsistent
    RemediationRequest     string `json:"remediationRequest"`     // Ambiguous
}
```

---

## Field Naming Patterns

### Temporal Fields

**Pattern**: Use past tense for timestamps

```go
// ✅ CORRECT
type RemediationRequestStatus struct {
    CreatedAt   metav1.Time `json:"createdAt"`   // When created
    StartedAt   metav1.Time `json:"startedAt"`   // When started
    CompletedAt metav1.Time `json:"completedAt"` // When completed
    UpdatedAt   metav1.Time `json:"updatedAt"`   // Last update
}

// ❌ WRONG
type RemediationRequestStatus struct {
    CreateTime metav1.Time `json:"createTime"` // Inconsistent suffix
    Start      metav1.Time `json:"start"`      // Ambiguous
    EndTime    metav1.Time `json:"endTime"`    // Inconsistent with others
}
```

**Note**: `CreatedAt` should **NOT** be in CRD spec (metadata only). See [CRD_SCHEMAS.md](./CRD_SCHEMAS.md).

---

### Reference Fields

**Pattern**: Use `Ref` suffix for object references

```go
// ✅ CORRECT
type AIAnalysisSpec struct {
    RemediationRequestRef string `json:"remediationRequestRef"` // References RemediationRequest
    WorkflowTemplateRef   string `json:"workflowTemplateRef"`   // References WorkflowTemplate
}

// ❌ WRONG
type AIAnalysisSpec struct {
    RemediationRequest     string `json:"remediationRequest"`     // Ambiguous
    WorkflowTemplateName   string `json:"workflowTemplateName"`   // Inconsistent
}
```

---

### Boolean Fields

**Pattern**: Use `is`, `has`, `should`, or `enable` prefixes

```go
// ✅ CORRECT
type WorkflowExecutionSpec struct {
    IsAutoApproved bool `json:"isAutoApproved"` // Clear boolean
    HasManualSteps bool `json:"hasManualSteps"` // Clear boolean
    ShouldNotify   bool `json:"shouldNotify"`   // Clear boolean
    EnableRetry    bool `json:"enableRetry"`    // Clear boolean
}

// ❌ WRONG
type WorkflowExecutionSpec struct {
    AutoApproved bool `json:"autoApproved"` // Ambiguous (noun or adjective?)
    ManualSteps  bool `json:"manualSteps"`  // Confusing (sounds like array)
    Notify       bool `json:"notify"`       // Could be verb or noun
}
```

---

### Count Fields

**Pattern**: Use `Count` suffix

```go
// ✅ CORRECT
type WorkflowExecutionStatus struct {
    StepCount         int `json:"stepCount"`         // Total steps
    CompletedCount    int `json:"completedCount"`    // Completed steps
    FailedCount       int `json:"failedCount"`       // Failed steps
    RetryCount        int `json:"retryCount"`        // Retry attempts
}

// ❌ WRONG
type WorkflowExecutionStatus struct {
    Steps          int `json:"steps"`          // Ambiguous (could be array)
    Completed      int `json:"completed"`      // Ambiguous type
    NumberOfFailed int `json:"numberOfFailed"` // Verbose
}
```

---

### List/Array Fields

**Pattern**: Use plural nouns

```go
// ✅ CORRECT
type WorkflowExecutionSpec struct {
    Steps       []WorkflowStep `json:"steps"`       // Plural
    Conditions  []Condition    `json:"conditions"`  // Plural
    Labels      map[string]string `json:"labels"`   // Plural
}

// ❌ WRONG
type WorkflowExecutionSpec struct {
    StepList      []WorkflowStep `json:"stepList"`      // Redundant suffix
    ConditionList []Condition    `json:"conditionList"` // Redundant suffix
    LabelMap      map[string]string `json:"labelMap"`   // Redundant suffix
}
```

---

### Nested Object Fields

**Pattern**: Use singular noun for nested objects

```go
// ✅ CORRECT
type RemediationRequestSpec struct {
    Target        TargetResource `json:"target"`        // Singular
    ProviderData  json.RawMessage `json:"providerData"` // Singular
    Configuration Config `json:"configuration"`        // Singular
}

// ❌ WRONG
type RemediationRequestSpec struct {
    Targets       TargetResource `json:"targets"`       // Plural for single object
    ProviderDatas json.RawMessage `json:"providerDatas"` // Incorrect plural
}
```

---

### Enum/Status Fields

**Pattern**: Use clear state names without redundant suffixes

```go
// ✅ CORRECT
type RemediationRequestStatus struct {
    Phase      string `json:"phase"`      // "Pending", "Processing", "Completed"
    State      string `json:"state"`      // "Active", "Failed", "Succeeded"
    Condition  string `json:"condition"`  // Kubernetes pattern
}

// ❌ WRONG
type RemediationRequestStatus struct {
    PhaseStatus    string `json:"phaseStatus"`    // Redundant suffix
    CurrentState   string `json:"currentState"`   // Redundant prefix
    StatusValue    string `json:"statusValue"`    // Confusing
}
```

---

## CRD-Specific Conventions

### RemediationRequest CRD

**Location**: Created by Gateway Service
**Purpose**: Entry point for remediation workflow

```go
// pkg/apis/remediation/v1/remediationrequest_types.go
package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemediationRequestSpec defines the desired state of RemediationRequest
type RemediationRequestSpec struct {
    // Signal identification
    SignalType      string `json:"signalType"`      // "prometheus", "kubernetes-event"
    SignalName      string `json:"signalName"`      // Alert/event name
    SignalNamespace string `json:"signalNamespace"` // Source namespace

    // Target resource
    TargetType      string `json:"targetType"`      // "deployment", "statefulset", etc.
    TargetName      string `json:"targetName"`      // Resource name
    TargetNamespace string `json:"targetNamespace"` // Resource namespace

    // Context and metadata
    Environment     string `json:"environment"`     // "production", "staging", "dev"
    Priority        string `json:"priority"`        // "P0", "P1", "P2"
    Fingerprint     string `json:"fingerprint"`     // SHA256 hash for deduplication

    // Provider-specific data (raw JSON)
    ProviderData    json.RawMessage `json:"providerData"`    // Kubernetes/AWS/etc. specific data
    OriginalPayload []byte          `json:"originalPayload"` // Raw webhook payload
}

// RemediationRequestStatus defines the observed state of RemediationRequest
type RemediationRequestStatus struct {
    // Phase tracking
    Phase      string      `json:"phase"`      // "Pending", "Processing", "Completed", "Failed"
    Message    string      `json:"message"`    // Human-readable message
    Reason     string      `json:"reason"`     // Machine-readable reason

    // Timestamps
    StartedAt   *metav1.Time `json:"startedAt,omitempty"`   // When processing started
    CompletedAt *metav1.Time `json:"completedAt,omitempty"` // When processing completed

    // Child CRD references
    RemediationProcessingRef string `json:"remediationProcessingRef,omitempty"` // Child CRD
    AIAnalysisRef            string `json:"aiAnalysisRef,omitempty"`            // Child CRD
    WorkflowExecutionRef     string `json:"workflowExecutionRef,omitempty"`     // Child CRD

    // Conditions (Kubernetes pattern)
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=rr
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.spec.priority`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RemediationRequest is the Schema for the remediationrequests API
type RemediationRequest struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   RemediationRequestSpec   `json:"spec,omitempty"`
    Status RemediationRequestStatus `json:"status,omitempty"`
}
```

---

### SignalProcessing CRD

**Location**: Created by Remediation Orchestrator
**Purpose**: Signal enrichment and context gathering

```go
// pkg/apis/remediationprocessing/v1/remediationprocessing_types.go
package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemediationProcessingSpec defines the desired state of RemediationProcessing
type RemediationProcessingSpec struct {
    // Parent reference
    RemediationRequestRef string `json:"remediationRequestRef"` // Parent CRD name

    // Signal details (copied from parent for convenience)
    SignalType      string `json:"signalType"`
    SignalName      string `json:"signalName"`
    SignalNamespace string `json:"signalNamespace"`

    // Target details
    TargetType      string `json:"targetType"`
    TargetName      string `json:"targetName"`
    TargetNamespace string `json:"targetNamespace"`

    // Processing configuration
    EnrichmentSources []string `json:"enrichmentSources"` // "context-api", "prometheus", etc.
}

// RemediationProcessingStatus defines the observed state of RemediationProcessing
type RemediationProcessingStatus struct {
    // Phase tracking
    Phase   string `json:"phase"`   // "Pending", "Enriching", "Completed", "Failed"
    Message string `json:"message"`
    Reason  string `json:"reason"`

    // Timestamps
    StartedAt   *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`

    // Enrichment results
    EnrichedContext   map[string]string `json:"enrichedContext,omitempty"`   // Key-value pairs
    ContextQuality    float64           `json:"contextQuality,omitempty"`    // 0.0-1.0
    MissingDataFields []string          `json:"missingDataFields,omitempty"` // Fields that couldn't be enriched

    // Conditions
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

---

### AIAnalysis CRD

**Location**: Created by Remediation Orchestrator
**Purpose**: AI-powered root cause analysis

```go
// pkg/apis/aianalysis/v1/aianalysis_types.go
package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AIAnalysisSpec defines the desired state of AIAnalysis
type AIAnalysisSpec struct {
    // Parent reference
    RemediationRequestRef string `json:"remediationRequestRef"`

    // Analysis input
    SignalType    string            `json:"signalType"`
    SignalContext map[string]string `json:"signalContext"` // Enriched context from RemediationProcessing

    // Analysis configuration
    LLMProvider    string  `json:"llmProvider"`    // "openai", "anthropic", "local"
    LLMModel       string  `json:"llmModel"`       // "gpt-4", "claude-3", etc.
    MaxTokens      int     `json:"maxTokens"`      // Token limit
    Temperature    float64 `json:"temperature"`    // 0.0-1.0
    IncludeHistory bool    `json:"includeHistory"` // Include historical patterns
}

// AIAnalysisStatus defines the observed state of AIAnalysis
type AIAnalysisStatus struct {
    // Phase tracking
    Phase   string `json:"phase"`   // "Pending", "Investigating", "Completed", "Failed"
    Message string `json:"message"`
    Reason  string `json:"reason"`

    // Timestamps
    StartedAt   *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`

    // Analysis results
    RootCause         string   `json:"rootCause,omitempty"`         // Identified root cause
    Confidence        float64  `json:"confidence,omitempty"`        // 0.0-1.0
    RecommendedAction string   `json:"recommendedAction,omitempty"` // Suggested remediation
    RequiresApproval  bool     `json:"requiresApproval"`            // Manual approval needed

    // Investigation details
    InvestigationID   string `json:"investigationId,omitempty"`   // HolmesGPT investigation ID
    // NOTE: TokensUsed REMOVED - HAPI owns LLM cost observability (use InvestigationID to correlate)
    InvestigationTime int64  `json:"investigationTime,omitempty"` // Duration in seconds

    // Conditions
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

---

### WorkflowExecution CRD

**Location**: Created by Remediation Orchestrator
**Purpose**: Multi-step remediation workflow orchestration

```go
// pkg/apis/workflowexecution/v1/workflowexecution_types.go
package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkflowExecutionSpec defines the desired state of WorkflowExecution
type WorkflowExecutionSpec struct {
    // Parent reference
    RemediationRequestRef string `json:"remediationRequestRef"`
    AIAnalysisRef         string `json:"aiAnalysisRef,omitempty"` // If AI analysis was performed

    // Workflow definition
    WorkflowName    string         `json:"workflowName"`    // Workflow template name
    Steps           []WorkflowStep `json:"steps"`           // Ordered list of steps
    IsAutoApproved  bool           `json:"isAutoApproved"`  // Auto-approved by Rego policy
    RequiresApproval bool          `json:"requiresApproval"` // Manual approval needed

    // Execution configuration
    TimeoutSeconds int  `json:"timeoutSeconds"` // Total workflow timeout
    EnableRetry    bool `json:"enableRetry"`    // Enable step retries
    MaxRetries     int  `json:"maxRetries"`     // Max retry attempts per step
}

// WorkflowStep defines a single step in the workflow
type WorkflowStep struct {
    Name           string            `json:"name"`           // Step name
    ActionType     string            `json:"actionType"`     // "kubectl-apply", "scale", etc.
    ActionData     map[string]string `json:"actionData"`     // Step-specific data
    TimeoutSeconds int               `json:"timeoutSeconds"` // Step timeout
    IsOptional     bool              `json:"isOptional"`     // Skip on failure
}

// WorkflowExecutionStatus defines the observed state of WorkflowExecution
type WorkflowExecutionStatus struct {
    // Phase tracking
    Phase   string `json:"phase"`   // "Pending", "Executing", "Completed", "Failed", "Paused"
    Message string `json:"message"`
    Reason  string `json:"reason"`

    // Timestamps
    StartedAt   *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`
    PausedAt    *metav1.Time `json:"pausedAt,omitempty"`

    // Execution progress
    CurrentStep     int `json:"currentStep"`     // Current step index (0-based)
    StepCount       int `json:"stepCount"`       // Total steps
    CompletedCount  int `json:"completedCount"`  // Completed steps
    FailedCount     int `json:"failedCount"`     // Failed steps
    SkippedCount    int `json:"skippedCount"`    // Skipped steps

    // Step statuses
    StepStatuses []WorkflowStepStatus `json:"stepStatuses,omitempty"`

    // Child CRD reference
    KubernetesExecutionRef string `json:"kubernetesExecutionRef,omitempty"`

    // Conditions
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// WorkflowStepStatus defines the status of a single workflow step
type WorkflowStepStatus struct {
    Name        string       `json:"name"`
    Phase       string       `json:"phase"`       // "Pending", "Running", "Completed", "Failed", "Skipped"
    StartedAt   *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`
    RetryCount  int          `json:"retryCount"`  // Number of retries
    Message     string       `json:"message,omitempty"`
}
```

---

### KubernetesExecution CRD

**Location**: Created by Workflow Execution Controller
**Purpose**: Kubernetes action execution with safety validation

```go
// pkg/apis/kubernetesexecution/v1/kubernetesexecution_types.go
package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubernetesExecutionSpec defines the desired state of KubernetesExecution
type KubernetesExecutionSpec struct {
    // Parent reference
    WorkflowExecutionRef string `json:"workflowExecutionRef"`
    StepName             string `json:"stepName"` // Workflow step name

    // Action details
    ActionType string            `json:"actionType"` // "apply", "patch", "scale", "delete"
    ActionData map[string]string `json:"actionData"` // Action-specific data

    // Target resource
    TargetType      string `json:"targetType"`      // "deployment", "statefulset", etc.
    TargetName      string `json:"targetName"`
    TargetNamespace string `json:"targetNamespace"`

    // Safety configuration
    EnableDryRun       bool `json:"enableDryRun"`       // Test before applying
    EnableValidation   bool `json:"enableValidation"`   // Validate safety rules
    EnableRollback     bool `json:"enableRollback"`     // Rollback on failure
    TimeoutSeconds     int  `json:"timeoutSeconds"`     // Action timeout
}

// KubernetesExecutionStatus defines the observed state of KubernetesExecution
type KubernetesExecutionStatus struct {
    // Phase tracking
    Phase   string `json:"phase"`   // "Pending", "Validating", "Executing", "Completed", "Failed", "RolledBack"
    Message string `json:"message"`
    Reason  string `json:"reason"`

    // Timestamps
    StartedAt      *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt    *metav1.Time `json:"completedAt,omitempty"`
    RolledBackAt   *metav1.Time `json:"rolledBackAt,omitempty"`

    // Execution results
    ActionResult       string `json:"actionResult,omitempty"`       // "success", "failure"
    ResourceVersion    string `json:"resourceVersion,omitempty"`    // Applied resource version
    PreviousVersion    string `json:"previousVersion,omitempty"`    // Version before action

    // Safety validation
    DryRunResult       string `json:"dryRunResult,omitempty"`       // Dry-run output
    ValidationResult   string `json:"validationResult,omitempty"`   // Safety validation result
    ValidationWarnings []string `json:"validationWarnings,omitempty"` // Safety warnings

    // Rollback details
    IsRolledBack       bool   `json:"isRolledBack"`                // Was rolled back
    RollbackReason     string `json:"rollbackReason,omitempty"`    // Why rolled back

    // Conditions
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

---

## Common Field Patterns

### Standard Status Fields (All CRDs)

```go
// All CRDs should include these standard status fields
type <CRD>Status struct {
    // Phase tracking (REQUIRED)
    Phase   string `json:"phase"`   // Current phase
    Message string `json:"message"` // Human-readable message
    Reason  string `json:"reason"`  // Machine-readable reason

    // Timestamps (OPTIONAL but RECOMMENDED)
    StartedAt   *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`

    // Conditions (REQUIRED for Kubernetes compliance)
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

---

### Standard Labels (All CRDs)

```go
// All CRDs should include these standard labels
metadata:
  labels:
    kubernaut.io/correlation-id: "req-2025-10-06-abc123"  // Correlation ID
    kubernaut.io/signal-type: "prometheus"                // Signal type
    kubernaut.io/priority: "P0"                           // Priority
    kubernaut.io/environment: "production"                // Environment
    kubernaut.io/parent-name: "remediation-abc123"        // Parent CRD (child CRDs only)
```

---

### Reference Field Pattern

**Convention**: Use `Ref` suffix for all CRD references

```go
// ✅ CORRECT - Consistent Ref suffix
type AIAnalysisSpec struct {
    RemediationRequestRef string `json:"remediationRequestRef"` // Parent
}

type WorkflowExecutionSpec struct {
    RemediationRequestRef string `json:"remediationRequestRef"` // Parent
    AIAnalysisRef         string `json:"aiAnalysisRef"`         // Sibling
}

type KubernetesExecutionSpec struct {
    WorkflowExecutionRef string `json:"workflowExecutionRef"` // Parent
}
```

---

## Anti-Patterns

### ❌ **1. Inconsistent Reference Naming**

```go
// ❌ WRONG - Mixed naming patterns
type AIAnalysisSpec struct {
    RemediationRequestName string `json:"remediationRequestName"` // Uses "Name"
}

type WorkflowExecutionSpec struct {
    RemediationRequestRef string `json:"remediationRequestRef"` // Uses "Ref"
}

// ✅ CORRECT - Consistent "Ref" suffix
type AIAnalysisSpec struct {
    RemediationRequestRef string `json:"remediationRequestRef"`
}

type WorkflowExecutionSpec struct {
    RemediationRequestRef string `json:"remediationRequestRef"`
}
```

---

### ❌ **2. Redundant CRD Name in Fields**

```go
// ❌ WRONG - Redundant "Remediation" prefix
type RemediationRequestSpec struct {
    RemediationSignalType      string `json:"remediationSignalType"`
    RemediationTargetNamespace string `json:"remediationTargetNamespace"`
}

// ✅ CORRECT - No redundant prefix
type RemediationRequestSpec struct {
    SignalType      string `json:"signalType"`
    TargetNamespace string `json:"targetNamespace"`
}
```

---

### ❌ **3. Ambiguous Boolean Names**

```go
// ❌ WRONG - Unclear boolean meaning
type WorkflowExecutionSpec struct {
    Approved bool `json:"approved"` // Approved by who? When?
    Manual   bool `json:"manual"`   // Manual what?
}

// ✅ CORRECT - Clear boolean intent
type WorkflowExecutionSpec struct {
    IsAutoApproved   bool `json:"isAutoApproved"`   // Clear state
    RequiresApproval bool `json:"requiresApproval"` // Clear requirement
}
```

---

### ❌ **4. Inconsistent Timestamp Fields**

```go
// ❌ WRONG - Inconsistent naming
type RemediationRequestStatus struct {
    CreateTime   metav1.Time `json:"createTime"`   // "Time" suffix
    StartedAt    metav1.Time `json:"startedAt"`    // "At" suffix
    EndTimestamp metav1.Time `json:"endTimestamp"` // "Timestamp" suffix
}

// ✅ CORRECT - Consistent "At" suffix
type RemediationRequestStatus struct {
    StartedAt   metav1.Time `json:"startedAt"`
    CompletedAt metav1.Time `json:"completedAt"`
}
```

---

### ❌ **5. List Fields with Redundant Suffixes**

```go
// ❌ WRONG - Redundant "List" suffix
type WorkflowExecutionSpec struct {
    StepList      []WorkflowStep `json:"stepList"`
    ConditionList []Condition    `json:"conditionList"`
}

// ✅ CORRECT - Plural is sufficient
type WorkflowExecutionSpec struct {
    Steps      []WorkflowStep `json:"steps"`
    Conditions []Condition    `json:"conditions"`
}
```

---

## Migration Guide

### Identifying Inconsistencies

```bash
# Find all CRD type definitions
find pkg/apis -name "*_types.go" -exec grep -H "type.*Spec struct" {} \;

# Check for inconsistent reference naming
grep -r "RequestName\|RequestRef" pkg/apis --include="*_types.go"

# Check for redundant prefixes
grep -r "RemediationSignal\|RemediationTarget" pkg/apis --include="*_types.go"

# Check for timestamp inconsistencies
grep -r "Time\|Timestamp\|At" pkg/apis --include="*_types.go"
```

---

### Migration Steps

#### Step 1: Update Type Definitions

```go
// Before (inconsistent)
type AIAnalysisSpec struct {
    RemediationRequestName string `json:"remediationRequestName"`
}

// After (consistent)
type AIAnalysisSpec struct {
    RemediationRequestRef string `json:"remediationRequestRef"`
}
```

---

#### Step 2: Update CRD Generation

```bash
# Regenerate CRDs with updated types
make generate
make manifests

# Verify changes
git diff config/crd/bases/
```

---

#### Step 3: Update Conversion Webhooks (if needed)

```go
// pkg/apis/aianalysis/v1/conversion.go
func (src *AIAnalysisSpec) ConvertTo(dst *v2.AIAnalysisSpec) error {
    // Map old field to new field
    dst.RemediationRequestRef = src.RemediationRequestName // Old → New
    return nil
}
```

---

#### Step 4: Update Controller Code

```go
// Before
remediationReqName := aiAnalysis.Spec.RemediationRequestName

// After
remediationReqRef := aiAnalysis.Spec.RemediationRequestRef
```

---

#### Step 5: Update Tests

```go
// Before
aiAnalysis := &aianalysisv1.AIAnalysis{
    Spec: aianalysisv1.AIAnalysisSpec{
        RemediationRequestName: "remediation-abc123",
    },
}

// After
aiAnalysis := &aianalysisv1.AIAnalysis{
    Spec: aianalysisv1.AIAnalysisSpec{
        RemediationRequestRef: "remediation-abc123",
    },
}
```

---

#### Step 6: Update Documentation

```bash
# Update all references in documentation
find docs/ -name "*.md" -exec sed -i '' 's/RemediationRequestName/RemediationRequestRef/g' {} \;
```

---

### Backward Compatibility (V1 Only)

**Note**: Kubernaut V1 has **no backward compatibility requirement** (pre-release).

For **future V2 migrations**:
- Use CRD conversion webhooks
- Maintain old field names with deprecation markers
- Provide migration guides for users

---

## Summary

### Naming Conventions Quick Reference

| Pattern | Rule | Example |
|---------|------|---------|
| **Go Struct Fields** | PascalCase | `SignalType`, `TargetNamespace` |
| **JSON/YAML Fields** | camelCase | `signalType`, `targetNamespace` |
| **Reference Fields** | `Ref` suffix | `remediationRequestRef` |
| **Timestamp Fields** | `At` suffix | `startedAt`, `completedAt` |
| **Boolean Fields** | `is`, `has`, `should`, `enable` prefix | `isAutoApproved`, `hasManualSteps` |
| **Count Fields** | `Count` suffix | `stepCount`, `failedCount` |
| **List Fields** | Plural nouns | `steps`, `conditions` |
| **Nested Objects** | Singular nouns | `target`, `configuration` |

---

### Standard Status Fields

All CRDs **MUST** include:
```go
Phase      string             `json:"phase"`
Message    string             `json:"message"`
Reason     string             `json:"reason"`
Conditions []metav1.Condition `json:"conditions,omitempty"`
```

All CRDs **SHOULD** include:
```go
StartedAt   *metav1.Time `json:"startedAt,omitempty"`
CompletedAt *metav1.Time `json:"completedAt,omitempty"`
```

---

### Key Takeaways

1. ✅ **Consistency**: Use same patterns across all CRDs
2. ✅ **Clarity**: Self-documenting field names
3. ✅ **Kubernetes Alignment**: Follow K8s conventions
4. ✅ **Go Conventions**: PascalCase for structs, camelCase for JSON
5. ✅ **No Redundancy**: Avoid prefixes that duplicate CRD name
6. ✅ **Clear References**: Always use `Ref` suffix
7. ✅ **Standard Status**: Include phase, message, reason, conditions

---

## References

### Related Documentation
- [CRD Schemas](./CRD_SCHEMAS.md) - Authoritative CRD schema definitions
- [Multi-Provider CRD Alternatives](./MULTI_PROVIDER_CRD_ALTERNATIVES.md) - V2 design
- [Service Dependency Map](./SERVICE_DEPENDENCY_MAP.md) - CRD interaction patterns

### Kubernetes Documentation
- [API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [CRD Best Practices](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/#best-practices)

### Go Documentation
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

---

**Document Status**: ✅ Complete
**Last Updated**: October 6, 2025
**Maintainer**: Kubernaut Architecture Team
**Version**: 1.0
