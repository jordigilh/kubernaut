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

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ========================================
// FLEXIBLE LABEL TYPES
// ========================================
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// DD-WORKFLOW-016: Label fields like severity and environment support both
// single-value and multi-value declarations in workflow-schema.yaml.
//
// StringOrSlice handles both formats transparently:
//   YAML: severity: "critical"         → ["critical"]
//   YAML: severity: [low, medium, high] → ["low", "medium", "high"]
//   JSON: "severity": "critical"        → ["critical"]
//   JSON: "severity": ["low","medium"]  → ["low", "medium"]
//
// MarshalJSON preserves backward compatibility:
//   len==1 → marshals as string (e.g., "critical")
//   len>1  → marshals as array  (e.g., ["low","medium","high"])
// ========================================

// StringOrSlice is a []string that unmarshals from both a single string and an
// array of strings in YAML and JSON. This enables workflow authors to declare
// label fields as either scalar ("critical") or array ([low, medium, high]).
type StringOrSlice []string

// UnmarshalYAML implements yaml.Unmarshaler for gopkg.in/yaml.v3.
// Accepts both scalar string nodes and sequence nodes.
func (s *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		// Single string: severity: "critical"
		*s = StringOrSlice{value.Value}
		return nil
	case yaml.SequenceNode:
		// Array: severity: [low, medium, high]
		var slice []string
		if err := value.Decode(&slice); err != nil {
			return fmt.Errorf("failed to decode string array: %w", err)
		}
		*s = StringOrSlice(slice)
		return nil
	default:
		return fmt.Errorf("expected string or array, got YAML node kind %d", value.Kind)
	}
}

// UnmarshalJSON implements json.Unmarshaler.
// Accepts both a JSON string and a JSON array of strings.
// Needed for reading JSONB columns that may contain either format.
func (s *StringOrSlice) UnmarshalJSON(data []byte) error {
	// Try single string first (most common for existing data)
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = StringOrSlice{single}
		return nil
	}
	// Try array of strings
	var slice []string
	if err := json.Unmarshal(data, &slice); err != nil {
		return fmt.Errorf("severity must be a string or array of strings: %w", err)
	}
	*s = StringOrSlice(slice)
	return nil
}

// MarshalJSON implements json.Marshaler.
// Preserves backward compatibility: single-element slices marshal as a plain
// string, multi-element slices marshal as a JSON array. This ensures existing
// API consumers and ogen client decoders continue to work for single-severity
// workflows while correctly representing multi-severity workflows.
func (s StringOrSlice) MarshalJSON() ([]byte, error) {
	if len(s) == 1 {
		return json.Marshal(s[0])
	}
	return json.Marshal([]string(s))
}

// ========================================
// WORKFLOW SCHEMA MODELS
// ========================================
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// Design Decision: DD-WORKFLOW-017 (OCI-based Workflow Registration)
// ========================================
//
// These types represent the structure of /workflow-schema.yaml
// as defined in BR-WORKFLOW-004. The schema is a plain configuration
// file (not a Kubernetes resource) extracted from workflow OCI images
// and stored in the workflow catalog.
//
// Naming: camelCase for all YAML/JSON field names per kubernaut convention.
//
// ========================================

// WorkflowSchema represents the complete /workflow-schema.yaml structure
// per BR-WORKFLOW-004. This is the authoritative schema format for all
// Kubernaut remediation workflows.
type WorkflowSchema struct {
	// Metadata contains workflow identification and description
	Metadata WorkflowSchemaMetadata `yaml:"metadata" json:"metadata" validate:"required"`

	// ActionType is the action type from the taxonomy (PascalCase).
	// Must match a valid entry in action_type_taxonomy.
	// DD-WORKFLOW-016: Used as FK for three-step discovery indexing.
	// Examples: "RestartPod", "ScaleReplicas", "DrainNode"
	ActionType string `yaml:"actionType" json:"actionType" validate:"required"`

	// Labels contains mandatory matching/filtering criteria for discovery
	Labels WorkflowSchemaLabels `yaml:"labels" json:"labels" validate:"required"`

	// CustomLabels contains operator-defined key-value labels for additional filtering
	CustomLabels map[string]string `yaml:"customLabels,omitempty" json:"customLabels,omitempty"`

	// Execution contains execution engine configuration (optional)
	Execution *WorkflowExecution `yaml:"execution,omitempty" json:"execution,omitempty"`

	// Parameters defines the workflow input parameters (at least one required)
	// These are returned to the LLM for parameter population
	Parameters []WorkflowParameter `yaml:"parameters" json:"parameters" validate:"required,min=1,dive"`

	// RollbackParameters defines parameters needed for rollback (optional)
	RollbackParameters []WorkflowParameter `yaml:"rollbackParameters,omitempty" json:"rollbackParameters,omitempty" validate:"omitempty,dive"`
}

// WorkflowSchemaMetadata contains workflow identification and description
type WorkflowSchemaMetadata struct {
	// WorkflowID is the unique workflow identifier
	// Format: lowercase alphanumeric with hyphens (e.g., "oomkill-restart-pod")
	WorkflowID string `yaml:"workflowId" json:"workflowId" validate:"required,max=255"`

	// Version is the semantic version (e.g., "1.0.0", "2.1.3")
	Version string `yaml:"version" json:"version" validate:"required,max=50"`

	// Description is a structured description for LLM and operator consumption
	// BR-WORKFLOW-004: Uses same format as action_type_taxonomy.description (DD-WORKFLOW-016)
	Description WorkflowDescription `yaml:"description" json:"description" validate:"required"`

	// Maintainers is optional maintainer information
	Maintainers []WorkflowMaintainer `yaml:"maintainers,omitempty" json:"maintainers,omitempty" validate:"omitempty,dive"`
}

// WorkflowDescription provides structured information about a workflow.
// This is shown to the LLM during workflow selection and to operators in the catalog.
// Format matches action_type_taxonomy.description (DD-WORKFLOW-016).
type WorkflowDescription struct {
	// What describes what this workflow concretely does. One sentence. (REQUIRED)
	What string `yaml:"what" json:"what" validate:"required"`

	// WhenToUse describes root cause conditions under which this workflow is appropriate. (REQUIRED)
	WhenToUse string `yaml:"whenToUse" json:"whenToUse" validate:"required"`

	// WhenNotToUse describes specific exclusion conditions. (OPTIONAL)
	// Only include genuinely useful exclusions. Do not include failure-based exclusions
	// (handled by remediation history, DD-HAPI-016).
	WhenNotToUse string `yaml:"whenNotToUse,omitempty" json:"whenNotToUse,omitempty"`

	// Preconditions describes conditions that must be verified through investigation
	// that cannot be determined by catalog label filtering. (OPTIONAL)
	Preconditions string `yaml:"preconditions,omitempty" json:"preconditions,omitempty"`
}

// WorkflowMaintainer contains maintainer contact information
type WorkflowMaintainer struct {
	// Name is the maintainer's name
	Name string `yaml:"name" json:"name" validate:"required"`

	// Email is the maintainer's email address
	Email string `yaml:"email" json:"email" validate:"required,email"`
}

// WorkflowSchemaLabels contains mandatory matching/filtering criteria for discovery.
// These fields are used by the three-step discovery protocol (DD-HAPI-017) to filter
// workflows for a given incident context. Stored in the labels JSONB column.
//
// BR-WORKFLOW-004: severity, environment, component, priority are required.
// DD-WORKFLOW-016: signalType changed to optional metadata (not used for matching in V1.0).
type WorkflowSchemaLabels struct {
	// SignalType is the signal type this workflow handles (OPTIONAL per DD-WORKFLOW-016)
	// Was required prior to DD-WORKFLOW-016; now optional metadata for workflow authors.
	// Examples: "OOMKilled", "CrashLoopBackOff", "NodeNotReady"
	SignalType string `yaml:"signalType,omitempty" json:"signalType,omitempty" validate:"omitempty"`

	// Severity is the severity level(s) this workflow is designed for (REQUIRED)
	// Values: "critical", "high", "medium", "low"
	// Accepts both single string and array in workflow-schema.yaml:
	//   severity: "critical"           → StringOrSlice{"critical"}
	//   severity: [low, medium, high]  → StringOrSlice{"low", "medium", "high"}
	Severity StringOrSlice `yaml:"severity" json:"severity" validate:"required,min=1"`

	// Environment is the target environment(s) (REQUIRED)
	// DD-WORKFLOW-016: Stored as JSONB array in remediation_workflow_catalog
	// Examples: ["production"], ["staging", "production"], ["*"] (wildcard for all)
	Environment []string `yaml:"environment" json:"environment" validate:"required,min=1"`

	// Component is the Kubernetes resource type this workflow remediates (REQUIRED)
	// Examples: "pod", "deployment", "node", "service"
	Component string `yaml:"component" json:"component" validate:"required"`

	// Priority is the business priority level (REQUIRED)
	// Values: "P0", "P1", "P2", "P3", "*" (wildcard for all)
	// Note: ExtractLabels normalizes to uppercase per OpenAPI enum [P0, P1, P2, P3, "*"]
	Priority string `yaml:"priority" json:"priority" validate:"required"`
}

// WorkflowExecution contains execution engine configuration
type WorkflowExecution struct {
	// Engine is the execution engine type
	// Values: "tekton", "ansible", "lambda", "shell"
	// Defaults to "tekton" if not specified.
	Engine string `yaml:"engine,omitempty" json:"engine,omitempty" validate:"omitempty,oneof=tekton ansible lambda shell"`

	// Bundle is the execution bundle or container image reference
	Bundle string `yaml:"bundle,omitempty" json:"bundle,omitempty" validate:"omitempty"`
}

// WorkflowParameter defines a workflow input parameter
// Format: JSON Schema compatible subset per BR-WORKFLOW-004
type WorkflowParameter struct {
	// Name is the parameter name (REQUIRED)
	// Format: UPPER_SNAKE_CASE per DD-WORKFLOW-003
	Name string `yaml:"name" json:"name" validate:"required"`

	// Type is the parameter type (REQUIRED)
	// Values: "string", "integer", "boolean", "array"
	Type string `yaml:"type" json:"type" validate:"required,oneof=string integer boolean array"`

	// Required indicates whether the parameter must be provided
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
	DependsOn []string `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty" validate:"omitempty"`
}

// ========================================
// VALIDATION HELPERS
// ========================================

// ValidateMandatoryLabels checks if all mandatory labels are present
// BR-WORKFLOW-004 + DD-WORKFLOW-016: severity, environment, component, priority are required.
// signalType is optional metadata (DD-WORKFLOW-016).
func (l *WorkflowSchemaLabels) ValidateMandatoryLabels() error {
	// Note: signalType intentionally NOT validated -- optional per DD-WORKFLOW-016
	if len(l.Severity) == 0 {
		return NewSchemaValidationError("labels.severity", "severity is required (at least one value)")
	}
	// Validate each severity value is in the allowed set
	allowedSeverities := map[string]bool{"critical": true, "high": true, "medium": true, "low": true}
	for _, sev := range l.Severity {
		if !allowedSeverities[sev] {
			return NewSchemaValidationError("labels.severity",
				fmt.Sprintf("invalid severity %q: must be one of critical, high, medium, low", sev))
		}
	}
	if len(l.Environment) == 0 {
		return NewSchemaValidationError("labels.environment", "environment is required (at least one value)")
	}
	if l.Component == "" {
		return NewSchemaValidationError("labels.component", "component is required")
	}
	if l.Priority == "" {
		return NewSchemaValidationError("labels.priority", "priority is required")
	}
	return nil
}

// ValidateDescription checks if the structured description has required fields
// BR-WORKFLOW-004: what and whenToUse are required
func (d *WorkflowDescription) ValidateDescription() error {
	if d.What == "" {
		return NewSchemaValidationError("metadata.description.what", "what is required")
	}
	if d.WhenToUse == "" {
		return NewSchemaValidationError("metadata.description.whenToUse", "whenToUse is required")
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
