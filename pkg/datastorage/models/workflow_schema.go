/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package models

// ========================================
// WORKFLOW SCHEMA MODELS (ADR-043)
// ========================================
// Authority: ADR-043 (Workflow Schema Definition Standard)
// Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
// Design Decision: DD-WORKFLOW-005 (Automated Schema Extraction)
// ========================================
//
// These types represent the structure of /workflow-schema.yaml
// as defined in ADR-043. The schema is extracted from workflow
// container images and stored in the workflow catalog.
//
// V1.0: Operators manually extract and POST the schema
// V1.1: WorkflowRegistration CRD controller extracts automatically
//
// ========================================

// WorkflowSchema represents the complete workflow-schema.yaml structure
// per ADR-043. This is the authoritative schema format for all Kubernaut
// remediation workflows.
type WorkflowSchema struct {
	// APIVersion is the schema version (e.g., "kubernaut.io/v1alpha1")
	APIVersion string `yaml:"apiVersion" json:"apiVersion" validate:"required"`

	// Kind must be "WorkflowSchema"
	Kind string `yaml:"kind" json:"kind" validate:"required,eq=WorkflowSchema"`

	// Metadata contains workflow identification information
	Metadata WorkflowSchemaMetadata `yaml:"metadata" json:"metadata" validate:"required"`

	// Labels contains discovery labels for MCP search
	// DD-WORKFLOW-001 v1.3: 6 mandatory labels + optional custom labels
	Labels WorkflowSchemaLabels `yaml:"labels" json:"labels" validate:"required"`

	// Execution contains execution engine information (optional in V1.0)
	Execution *WorkflowExecution `yaml:"execution,omitempty" json:"execution,omitempty"`

	// Parameters defines the workflow input parameters
	// These are returned to the LLM for parameter population
	Parameters []WorkflowParameter `yaml:"parameters" json:"parameters" validate:"required,min=1,dive"`

	// RollbackParameters defines parameters needed for rollback (optional)
	RollbackParameters []WorkflowParameter `yaml:"rollback_parameters,omitempty" json:"rollback_parameters,omitempty" validate:"omitempty,dive"`
}

// WorkflowSchemaMetadata contains workflow identification information
type WorkflowSchemaMetadata struct {
	// WorkflowID is the unique workflow identifier
	// Format: lowercase alphanumeric with hyphens (e.g., "oomkill-scale-down")
	WorkflowID string `yaml:"workflow_id" json:"workflow_id" validate:"required,max=255"`

	// Version is the semantic version (e.g., "1.0.0", "2.1.3")
	Version string `yaml:"version" json:"version" validate:"required,max=50"`

	// Description is a human-readable description (shown to LLM and operators)
	Description string `yaml:"description" json:"description" validate:"required,max=500"`

	// Maintainers is optional maintainer information
	Maintainers []WorkflowMaintainer `yaml:"maintainers,omitempty" json:"maintainers,omitempty" validate:"omitempty,dive"`
}

// WorkflowMaintainer contains maintainer contact information
type WorkflowMaintainer struct {
	// Name is the maintainer's name
	Name string `yaml:"name" json:"name" validate:"required"`

	// Email is the maintainer's email address
	Email string `yaml:"email" json:"email" validate:"required,email"`
}

// WorkflowSchemaLabels contains discovery labels for MCP search
// DD-WORKFLOW-001: Mandatory label schema
type WorkflowSchemaLabels struct {
	// SignalType is the signal type this workflow handles (REQUIRED)
	// Examples: "OOMKilled", "CrashLoopBackOff", "HighMemoryUsage"
	SignalType string `yaml:"signal_type" json:"signal_type" validate:"required"`

	// Severity is the severity level this workflow is designed for (REQUIRED)
	// Values: "critical", "high", "medium", "low"
	Severity string `yaml:"severity" json:"severity" validate:"required,oneof=critical high medium low"`

	// RiskTolerance is the risk tolerance required for this workflow (REQUIRED)
	// Values: "low", "medium", "high"
	RiskTolerance string `yaml:"risk_tolerance" json:"risk_tolerance" validate:"required,oneof=low medium high"`

	// BusinessCategory is an optional custom label for business domain filtering
	// Per DD-WORKFLOW-001 v1.3: Moved from mandatory to optional custom label
	// Values: user-defined (e.g., "payment-service", "analytics", "infrastructure")
	BusinessCategory string `yaml:"business_category,omitempty" json:"business_category,omitempty" validate:"omitempty"`

	// Environment is the target environment (OPTIONAL)
	// Examples: "production", "staging", "development"
	Environment string `yaml:"environment,omitempty" json:"environment,omitempty" validate:"omitempty"`

	// Priority is the priority level (OPTIONAL)
	// Values: "p0", "p1", "p2", "p3", "p4"
	Priority string `yaml:"priority,omitempty" json:"priority,omitempty" validate:"omitempty,oneof=p0 p1 p2 p3 p4"`

	// Component is the component type (OPTIONAL)
	// Examples: "pod", "deployment", "node", "service"
	Component string `yaml:"component,omitempty" json:"component,omitempty" validate:"omitempty"`

	// ActionType is the action type for workflow discovery indexing (REQUIRED)
	// DD-WORKFLOW-016: Used as FK into action_type_taxonomy for three-step discovery
	// DD-WORKFLOW-017: Extracted from workflow-schema.yaml during OCI-based registration
	// Examples: "restart_pod", "scale_deployment", "drain_node"
	ActionType string `yaml:"action_type" json:"action_type" validate:"required"`

	// CustomLabels contains additional operator-defined labels
	// No character limits (unlike K8s labels)
	CustomLabels map[string]string `yaml:"-" json:"-"` // Populated from remaining YAML keys
}

// WorkflowExecution contains execution engine information
type WorkflowExecution struct {
	// Engine is the execution engine type
	// V1 values: "tekton"
	// V2 values: "tekton", "ansible", "lambda", "shell"
	Engine string `yaml:"engine,omitempty" json:"engine,omitempty" validate:"omitempty,oneof=tekton ansible lambda shell"`

	// Bundle is the container image or bundle reference
	// For Tekton: OCI bundle URL
	// For Ansible: Git repo or container with workflow
	Bundle string `yaml:"bundle,omitempty" json:"bundle,omitempty" validate:"omitempty"`
}

// WorkflowParameter defines a workflow input parameter
// Format: JSON Schema compatible subset per ADR-043
type WorkflowParameter struct {
	// Name is the parameter name (REQUIRED)
	// Format: UPPER_SNAKE_CASE per DD-WORKFLOW-003
	Name string `yaml:"name" json:"name" validate:"required"`

	// Type is the parameter type (REQUIRED)
	// Values: "string", "integer", "boolean", "array"
	Type string `yaml:"type" json:"type" validate:"required,oneof=string integer boolean array"`

	// Required indicates whether the parameter must be provided (REQUIRED)
	Required bool `yaml:"required" json:"required"`

	// Description is a human-readable description for LLM (REQUIRED)
	Description string `yaml:"description" json:"description" validate:"required"`

	// Enum contains allowed values for string type (OPTIONAL)
	Enum []string `yaml:"enum,omitempty" json:"enum,omitempty" validate:"omitempty"`

	// Pattern is a regex pattern for string validation (OPTIONAL)
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty" validate:"omitempty"`

	// Minimum is the minimum value for integer type (OPTIONAL)
	Minimum *int `yaml:"minimum,omitempty" json:"minimum,omitempty" validate:"omitempty"`

	// Maximum is the maximum value for integer type (OPTIONAL)
	Maximum *int `yaml:"maximum,omitempty" json:"maximum,omitempty" validate:"omitempty"`

	// Default is the default value if not provided (OPTIONAL)
	Default interface{} `yaml:"default,omitempty" json:"default,omitempty" validate:"omitempty"`

	// DependsOn references other parameter names that must be set first (OPTIONAL)
	DependsOn []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty" validate:"omitempty"`
}

// ========================================
// VALIDATION HELPERS
// ========================================

// ValidateMandatoryLabels checks if all mandatory labels are present
// DD-WORKFLOW-001: signal_type, severity, risk_tolerance are required
func (l *WorkflowSchemaLabels) ValidateMandatoryLabels() error {
	if l.SignalType == "" {
		return NewSchemaValidationError("labels.signal_type", "signal_type is required")
	}
	if l.Severity == "" {
		return NewSchemaValidationError("labels.severity", "severity is required")
	}
	if l.RiskTolerance == "" {
		return NewSchemaValidationError("labels.risk_tolerance", "risk_tolerance is required")
	}
	return nil
}

// SchemaValidationError represents a workflow schema validation error
type SchemaValidationError struct {
	Field   string
	Message string
}

func (e *SchemaValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// NewSchemaValidationError creates a new schema validation error
func NewSchemaValidationError(field, message string) *SchemaValidationError {
	return &SchemaValidationError{
		Field:   field,
		Message: message,
	}
}
