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
	"strconv"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"gopkg.in/yaml.v3"
)

// ========================================
// WORKFLOW SCHEMA MODELS
// ========================================
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// Authority: BR-WORKFLOW-006 (RemediationWorkflow CRD Definition)
// Design Decision: DD-WORKFLOW-017 (OCI-based Workflow Registration)
// ========================================
//
// These types represent the structure of /workflow-schema.yaml files which use
// the Kubernetes CRD envelope format: apiVersion/kind/metadata/spec.
// The apiVersion determines the schema version (e.g., kubernaut.ai/v1alpha1 -> "1.0").
//
// Naming: camelCase for all YAML/JSON field names per kubernaut convention.
//
// ========================================

// WorkflowSchemaCRD is the top-level CRD envelope for workflow-schema.yaml.
// The parser unmarshals into this structure and extracts the Spec for downstream use.
type WorkflowSchemaCRD struct {
	APIVersion string              `yaml:"apiVersion" json:"apiVersion"`
	Kind       string              `yaml:"kind" json:"kind"`
	Metadata   WorkflowCRDMetadata `yaml:"metadata" json:"metadata"`
	Spec       WorkflowSchema      `yaml:"spec" json:"spec"`
}

// WorkflowCRDMetadata is the CRD-level metadata (name, namespace).
// Distinct from WorkflowSchemaMetadata which holds workflowId, version, description.
type WorkflowCRDMetadata struct {
	Name      string `yaml:"name" json:"name"`
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
}

// apiVersionToSchemaVersion maps CRD apiVersion to the DB schema_version value.
// kubernaut.ai/v1alpha1 is the pre-GA version established by #292.
var APIVersionToSchemaVersion = map[string]string{
	"kubernaut.ai/v1alpha1": "1.0",
}

// WorkflowSchema represents the spec content of a RemediationWorkflow CRD.
// This is the authoritative schema format for all Kubernaut remediation workflows.
type WorkflowSchema struct {
	// SchemaVersion is derived from apiVersion by the parser (not present in YAML).
	// Stored in DB column schema_version for format compatibility tracking.
	SchemaVersion string `yaml:"-" json:"schemaVersion"`

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

	// DetectedLabels contains author-declared infrastructure requirements (OPTIONAL)
	// ADR-043 v1.3: Matched against incident DetectedLabels from HAPI LabelDetector
	// DD-WORKFLOW-001 v2.0: Boolean fields accept only "true"; string fields accept
	// specific values or "*" wildcard.
	// nil = section absent from YAML; non-nil empty = explicit empty section
	DetectedLabels *DetectedLabelsSchema `yaml:"detectedLabels,omitempty" json:"detectedLabels,omitempty"`

	// Execution contains execution engine configuration (optional)
	Execution *WorkflowExecution `yaml:"execution,omitempty" json:"execution,omitempty"`

	// Dependencies declares infrastructure resources (Secrets, ConfigMaps) required
	// by the workflow, provisioned by operators in the execution namespace.
	// DD-WE-006: Validated at registration (DS) and execution (WFE) time.
	Dependencies *WorkflowDependencies `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`

	// Parameters defines the workflow input parameters (at least one required)
	// These are returned to the LLM for parameter population
	Parameters []WorkflowParameter `yaml:"parameters" json:"parameters" validate:"required,min=1,dive"`

	// RollbackParameters defines parameters needed for rollback (optional)
	RollbackParameters []WorkflowParameter `yaml:"rollbackParameters,omitempty" json:"rollbackParameters,omitempty" validate:"omitempty,dive"`
}

// WorkflowSchemaMetadata contains workflow identification and description
type WorkflowSchemaMetadata struct {
	// WorkflowName is the human-readable workflow name (maps to DS workflow_name)
	// Format: lowercase alphanumeric with hyphens (e.g., "oomkill-restart-pod")
	WorkflowName string `yaml:"workflowName" json:"workflowName" validate:"required,max=255"`

	// Version is the semantic version (e.g., "1.0.0", "2.1.3")
	Version string `yaml:"version" json:"version" validate:"required,max=50"`

	// Description is a structured description for LLM and operator consumption
	// BR-WORKFLOW-004: Uses same format as action_type_taxonomy.description (DD-WORKFLOW-016)
	Description WorkflowDescription `yaml:"description" json:"description" validate:"required"`

	// Maintainers is optional maintainer information
	Maintainers []WorkflowMaintainer `yaml:"maintainers,omitempty" json:"maintainers,omitempty" validate:"omitempty,dive"`
}

// WorkflowDescription is an alias for the shared StructuredDescription type.
// DD-WORKFLOW-016: Same format shared between RemediationWorkflow and ActionType.
type WorkflowDescription = sharedtypes.StructuredDescription

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
// Issue #274: signalName removed — LLM selects by actionType + structured descriptions.
type WorkflowSchemaLabels struct {
	// Severity is the severity level(s) this workflow is designed for (REQUIRED)
	// Values: "critical", "high", "medium", "low"
	// DD-WORKFLOW-001 v2.7: Always an array in workflow-schema.yaml.
	// Examples: severity: [critical] or severity: [low, medium, high]
	Severity []string `yaml:"severity" json:"severity" validate:"required,min=1"`

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

// WorkflowDependencies declares infrastructure resources (Secrets, ConfigMaps)
// that must exist in the execution namespace for the workflow to function.
// DD-WE-006: These are operator-provisioned resources, NOT LLM-provided parameters.
type WorkflowDependencies struct {
	Secrets    []ResourceDependency `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	ConfigMaps []ResourceDependency `yaml:"configMaps,omitempty" json:"configMaps,omitempty"`
}

// ResourceDependency identifies a Kubernetes resource (Secret or ConfigMap)
// by name in the execution namespace (kubernaut-workflows).
type ResourceDependency struct {
	Name string `yaml:"name" json:"name" validate:"required"`
}

// ValidateDependencies checks the dependencies section for structural correctness:
// non-empty names and unique names within each category.
func (d *WorkflowDependencies) ValidateDependencies() error {
	if d == nil {
		return nil
	}

	seen := make(map[string]bool)
	for i, s := range d.Secrets {
		if s.Name == "" {
			return NewSchemaValidationError(
				fmt.Sprintf("dependencies.secrets[%d].name", i),
				"name is required for each secret dependency",
			)
		}
		if seen[s.Name] {
			return NewSchemaValidationError(
				"dependencies.secrets",
				fmt.Sprintf("duplicate secret name %q", s.Name),
			)
		}
		seen[s.Name] = true
	}

	seen = make(map[string]bool)
	for i, cm := range d.ConfigMaps {
		if cm.Name == "" {
			return NewSchemaValidationError(
				fmt.Sprintf("dependencies.configMaps[%d].name", i),
				"name is required for each configMap dependency",
			)
		}
		if seen[cm.Name] {
			return NewSchemaValidationError(
				"dependencies.configMaps",
				fmt.Sprintf("duplicate configMap name %q", cm.Name),
			)
		}
		seen[cm.Name] = true
	}

	return nil
}

// WorkflowExecution contains execution engine configuration
type WorkflowExecution struct {
	// Engine is the execution engine type
	// Values: "tekton", "job", "ansible"
	// Defaults to "tekton" if not specified.
	Engine string `yaml:"engine,omitempty" json:"engine,omitempty" validate:"omitempty,oneof=tekton job ansible"`

	// Bundle is the execution bundle or container image reference
	Bundle string `yaml:"bundle,omitempty" json:"bundle,omitempty" validate:"omitempty"`

	// BundleDigest is the digest of the execution bundle (OPTIONAL).
	// For tekton/job: OCI image SHA. For ansible: Git commit SHA.
	BundleDigest string `yaml:"bundleDigest,omitempty" json:"bundleDigest,omitempty" validate:"omitempty"`

	// EngineConfig holds engine-specific configuration as parsed YAML/JSON.
	// BR-WE-016: Discriminator pattern — the Engine field determines the shape.
	// Stored as interface{} for YAML compatibility; converted to json.RawMessage by the parser.
	EngineConfig interface{} `yaml:"engineConfig,omitempty" json:"engineConfig,omitempty"`
}

// AnsibleEngineConfig holds Ansible/AWX/AAP-specific execution configuration.
// BR-WE-015, BR-WE-016: Deserialized from WorkflowExecution.EngineConfig
// when Engine="ansible" via two-phase unmarshal (ParseEngineConfig).
type AnsibleEngineConfig struct {
	PlaybookPath    string `yaml:"playbookPath" json:"playbookPath"`
	JobTemplateName string `yaml:"jobTemplateName,omitempty" json:"jobTemplateName,omitempty"`
	InventoryName   string `yaml:"inventoryName,omitempty" json:"inventoryName,omitempty"`
}

// ParseEngineConfig deserializes raw engineConfig JSON based on the engine discriminator.
// BR-WE-016: Two-phase unmarshal — read engine first, then unmarshal config.
// Returns (nil, nil) when raw is empty, regardless of engine.
// Returns error for unknown engines or invalid/incomplete ansible config.
func ParseEngineConfig(engine string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	switch engine {
	case "ansible":
		var cfg AnsibleEngineConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("invalid ansible engineConfig: %w", err)
		}
		if cfg.PlaybookPath == "" {
			return nil, fmt.Errorf("playbookPath is required in ansible engineConfig")
		}
		return &cfg, nil
	case "tekton", "job":
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown engine %q: cannot parse engineConfig", engine)
	}
}

// WorkflowParameter defines a workflow input parameter
// Format: JSON Schema compatible subset per BR-WORKFLOW-004
type WorkflowParameter struct {
	// Name is the parameter name (REQUIRED)
	// Format: UPPER_SNAKE_CASE per DD-WORKFLOW-003
	Name string `yaml:"name" json:"name" validate:"required"`

	// Type is the parameter type (REQUIRED)
	// Values: "string", "integer", "boolean", "array", "float"
	// BR-WORKFLOW-005: "float" added for AWX survey compatibility.
	Type string `yaml:"type" json:"type" validate:"required,oneof=string integer boolean array float"`

	// Required indicates whether the parameter must be provided
	Required bool `yaml:"required" json:"required"`

	// Description is a human-readable description for LLM (REQUIRED)
	Description string `yaml:"description" json:"description" validate:"required"`

	// Enum contains allowed values for string type (OPTIONAL)
	Enum []string `yaml:"enum,omitempty" json:"enum,omitempty" validate:"omitempty"`

	// Pattern is a regex pattern for string validation (OPTIONAL)
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty" validate:"omitempty"`

	// Minimum is the minimum value for integer/float types (OPTIONAL)
	// BR-WORKFLOW-005: Changed from *int to *float64 for float support. Backward compatible.
	Minimum *float64 `yaml:"minimum,omitempty" json:"minimum,omitempty" validate:"omitempty"`

	// Maximum is the maximum value for integer/float types (OPTIONAL)
	// BR-WORKFLOW-005: Changed from *int to *float64 for float support. Backward compatible.
	Maximum *float64 `yaml:"maximum,omitempty" json:"maximum,omitempty" validate:"omitempty"`

	// Default is the default value if not provided (OPTIONAL)
	Default interface{} `yaml:"default,omitempty" json:"default,omitempty" validate:"omitempty"`

	// DependsOn references other parameter names that must be set first (OPTIONAL)
	DependsOn []string `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty" validate:"omitempty"`
}

// ========================================
// DETECTED LABELS SCHEMA (ADR-043 v1.3)
// ========================================
// Authority: ADR-043 v1.3 (detectedLabels schema field)
// Authority: DD-WORKFLOW-001 v2.0 (DetectedLabels architecture)
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
//
// Raw YAML representation of the detectedLabels section. All fields are
// strings matching the YAML values. Converted to models.DetectedLabels
// (bool/string) via Parser.ExtractDetectedLabels().
//
// PopulatedFields tracks which fields the author explicitly declared,
// mirroring the Signal Processing pattern for distinguishing "empty"
// from "not defined" at the field level.
// ========================================

// detectedLabelFieldSpec describes one detectedLabels field's type and valid values.
// Single source of truth for field classification, used by UnmarshalYAML and Validate.
type detectedLabelFieldSpec struct {
	isBoolean   bool
	validValues map[string]bool // nil for boolean fields
}

// detectedLabelsFieldSpecs is the authoritative registry of all valid detectedLabels
// fields, their type (boolean vs string-enum), and accepted values.
// Adding a new field requires a single entry here plus the struct field + accessor.
var detectedLabelsFieldSpecs = map[string]detectedLabelFieldSpec{
	"gitOpsManaged":   {isBoolean: true},
	"gitOpsTool":      {validValues: map[string]bool{"argocd": true, "flux": true, "*": true}},
	"pdbProtected":    {isBoolean: true},
	"hpaEnabled":      {isBoolean: true},
	"stateful":        {isBoolean: true},
	"helmManaged":     {isBoolean: true},
	"networkIsolated": {isBoolean: true},
	"serviceMesh":     {validValues: map[string]bool{"istio": true, "linkerd": true, "*": true}},
}

// DetectedLabelsSchema represents the raw YAML detectedLabels section.
// Fields are strings matching YAML values; converted to models.DetectedLabels
// via Parser.ExtractDetectedLabels().
type DetectedLabelsSchema struct {
	GitOpsManaged   string `yaml:"-" json:"gitOpsManaged,omitempty"`
	GitOpsTool      string `yaml:"-" json:"gitOpsTool,omitempty"`
	PDBProtected    string `yaml:"-" json:"pdbProtected,omitempty"`
	HPAEnabled      string `yaml:"-" json:"hpaEnabled,omitempty"`
	Stateful        string `yaml:"-" json:"stateful,omitempty"`
	HelmManaged     string `yaml:"-" json:"helmManaged,omitempty"`
	NetworkIsolated string `yaml:"-" json:"networkIsolated,omitempty"`
	ServiceMesh     string `yaml:"-" json:"serviceMesh,omitempty"`

	// PopulatedFields records which fields the author explicitly declared.
	// Mirrors the Signal Processing pattern for distinguishing "empty" from
	// "not defined" at the field level.
	PopulatedFields []string `yaml:"-" json:"-"`
}

// UnmarshalYAML implements custom YAML unmarshaling for DetectedLabelsSchema.
// Iterates over the raw YAML map to track which fields were explicitly declared
// (PopulatedFields) while assigning values to typed struct fields.
func (d *DetectedLabelsSchema) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("detectedLabels must be a mapping, got %v", value.Kind)
	}

	d.PopulatedFields = make([]string, 0, len(value.Content)/2)

	for i := 0; i < len(value.Content)-1; i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]
		key := keyNode.Value

		if _, known := detectedLabelsFieldSpecs[key]; !known {
			return NewSchemaValidationError("detectedLabels."+key,
				fmt.Sprintf("unknown field %q in detectedLabels", key))
		}

		d.PopulatedFields = append(d.PopulatedFields, key)

		val := valNode.Value
		switch key {
		case "gitOpsManaged":
			d.GitOpsManaged = val
		case "gitOpsTool":
			d.GitOpsTool = val
		case "pdbProtected":
			d.PDBProtected = val
		case "hpaEnabled":
			d.HPAEnabled = val
		case "stateful":
			d.Stateful = val
		case "helmManaged":
			d.HelmManaged = val
		case "networkIsolated":
			d.NetworkIsolated = val
		case "serviceMesh":
			d.ServiceMesh = val
		}
	}

	return nil
}

// fieldValue returns the raw string value for a given field name.
// Centralizes the field-name-to-struct-field mapping used by both
// UnmarshalYAML (write) and ValidateDetectedLabels (read).
func (d *DetectedLabelsSchema) fieldValue(name string) string {
	switch name {
	case "gitOpsManaged":
		return d.GitOpsManaged
	case "gitOpsTool":
		return d.GitOpsTool
	case "pdbProtected":
		return d.PDBProtected
	case "hpaEnabled":
		return d.HPAEnabled
	case "stateful":
		return d.Stateful
	case "helmManaged":
		return d.HelmManaged
	case "networkIsolated":
		return d.NetworkIsolated
	case "serviceMesh":
		return d.ServiceMesh
	default:
		return ""
	}
}

// validValuesSlice returns sorted valid values for error messages.
func validValuesSlice(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	return out
}

// ValidateDetectedLabels validates the detectedLabels section per BR-WORKFLOW-004.
// Boolean fields accept only "true"; string fields accept specific values or "*".
// Uses detectedLabelsFieldSpecs as the single source of truth for field classification.
func (d *DetectedLabelsSchema) ValidateDetectedLabels() error {
	if d == nil {
		return nil
	}

	for _, field := range d.PopulatedFields {
		val := d.fieldValue(field)
		spec := detectedLabelsFieldSpecs[field]

		if spec.isBoolean {
			if val != "true" {
				return NewSchemaValidationError("detectedLabels."+field,
					fmt.Sprintf("%s only accepts \"true\", got %q", field, val))
			}
			continue
		}

		if spec.validValues != nil && !spec.validValues[val] {
			return NewSchemaValidationError("detectedLabels."+field,
				fmt.Sprintf("%s must be one of %v, got %q", field, validValuesSlice(spec.validValues), val))
		}
	}

	return nil
}

// ========================================
// VALIDATION HELPERS
// ========================================

// ValidateMandatoryLabels checks if all mandatory labels are present
// BR-WORKFLOW-004: severity, environment, component, priority are required.
// Issue #274: signalName removed from schema — not validated or stored.
func (l *WorkflowSchemaLabels) ValidateMandatoryLabels() error {
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

// ValidateDescription checks if the structured description has required fields.
// BR-WORKFLOW-004: what and whenToUse are required.
func ValidateDescription(d *WorkflowDescription) error {
	if d.What == "" {
		return NewSchemaValidationError("metadata.description.what", "what is required")
	}
	if d.WhenToUse == "" {
		return NewSchemaValidationError("metadata.description.whenToUse", "whenToUse is required")
	}
	return nil
}

// ValidateParameterValue checks a string parameter value against the schema's type and bounds.
// Returns nil if the value is valid; error with descriptive message otherwise.
func ValidateParameterValue(param WorkflowParameter, value string) error {
	switch param.Type {
	case "integer":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("parameter %q: invalid integer value %q", param.Name, value)
		}
		return checkNumericBounds(param, v)
	case "float":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("parameter %q: invalid float value %q", param.Name, value)
		}
		return checkNumericBounds(param, v)
	default:
		return nil
	}
}

func checkNumericBounds(param WorkflowParameter, value float64) error {
	if param.Minimum != nil && value < *param.Minimum {
		return fmt.Errorf("parameter %q: value %v is below minimum %v", param.Name, value, *param.Minimum)
	}
	if param.Maximum != nil && value > *param.Maximum {
		return fmt.Errorf("parameter %q: value %v exceeds maximum %v", param.Name, value, *param.Maximum)
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
