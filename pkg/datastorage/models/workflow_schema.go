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
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"gopkg.in/yaml.v3"
)

// ErrNoEngineConfig indicates ParseEngineConfig has no typed engineConfig to
// return: either raw was empty, or the engine (tekton/job) has no typed
// engineConfig struct at all. Issue #1674: typed sentinel replacing the
// previous ambiguous (nil, nil) returns.
var ErrNoEngineConfig = errors.New("no engineConfig to parse")

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
// metadata.name provides the workflow name (issue #329).
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
// Issue #329: WorkflowName is derived from CRD metadata.name (not duplicated in spec).
type WorkflowSchema struct {
	// SchemaVersion is derived from apiVersion by the parser (not present in YAML).
	// Stored in DB column schema_version for format compatibility tracking.
	SchemaVersion string `yaml:"-" json:"schemaVersion"`

	// WorkflowName is derived from the CRD metadata.name by the parser.
	// Not present in the spec YAML; populated by Parse() from crd.Metadata.Name.
	WorkflowName string `yaml:"-" json:"workflowName" validate:"required,max=255"`

	// Version is the semantic version (e.g., "1.0.0", "2.1.3")
	Version string `yaml:"version" json:"version" validate:"required,max=50"`

	// Description is a structured description for LLM and operator consumption
	// BR-WORKFLOW-004: Uses same format as action_type_taxonomy.description (DD-WORKFLOW-016)
	Description WorkflowDescription `yaml:"description" json:"description" validate:"required"`

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
	// ADR-043 v1.3: Matched against incident DetectedLabels from KA LabelDetector
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

	// Maintainers is optional maintainer information
	Maintainers []WorkflowMaintainer `yaml:"maintainers,omitempty" json:"maintainers,omitempty" validate:"omitempty,dive"`

	// Parameters defines the workflow input parameters (at least one required)
	// These are returned to the LLM for parameter population
	Parameters []WorkflowParameter `yaml:"parameters" json:"parameters" validate:"required,min=1,dive"`

	// RollbackParameters defines parameters needed for rollback (optional)
	RollbackParameters []WorkflowParameter `yaml:"rollbackParameters,omitempty" json:"rollbackParameters,omitempty" validate:"omitempty,dive"`
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
// These fields are used by the three-step discovery protocol (DD-KA-017) to filter
// workflows for a given incident context. Stored in the labels JSONB column.
//
// BR-WORKFLOW-004: severity, environment, component, priority are required.
// Issue #274: signalName removed — LLM selects by actionType + structured descriptions.
type WorkflowSchemaLabels struct {
	// Severity is the severity level(s) this workflow is designed for (REQUIRED)
	// Values: "critical", "high", "warning", "info", "*" (wildcard for all)
	// DD-WORKFLOW-001 v2.8: Always an array in workflow-schema.yaml. Supports "*" wildcard.
	// ADR-066: "low" replaced by "info". Examples: severity: [critical] or severity: [info, warning, high] or severity: ["*"]
	Severity []string `yaml:"severity" json:"severity" validate:"required,min=1"`

	// Environment is the target environment(s) (REQUIRED)
	// DD-WORKFLOW-016: Stored as JSONB array in remediation_workflow_catalog
	// Examples: ["production"], ["staging", "production"], ["*"] (wildcard for all)
	Environment []string `yaml:"environment" json:"environment" validate:"required,min=1"`

	// Component is the Kubernetes resource GVK(s) this workflow remediates (REQUIRED)
	// Format: "apiVersion/Kind" — e.g. "apps/v1/Deployment", "v1/Pod" (core group omits group prefix)
	// Examples: ["apps/v1/Deployment"], ["v1/Pod", "apps/v1/StatefulSet"], ["*"] (wildcard for all)
	// Issue #790: Changed from string to []string to match severity/environment pattern
	// Issue #1051: Changed from plain lowercase kind to fully-qualified GVK format
	Component []string `yaml:"component" json:"component" validate:"required,min=1"`

	// Priority is the business priority level (REQUIRED)
	// Values: "P0", "P1", "P2", "P3", "*" (wildcard for all)
	// Note: ExtractLabels normalizes to uppercase per OpenAPI enum [P0, P1, P2, P3, "*"]
	Priority string `yaml:"priority" json:"priority" validate:"required"`

	// Cluster is the cluster classification(s) this workflow is eligible for (OPTIONAL)
	// Values: operator-defined by SP Rego policy (e.g. "production", "staging-eu"), "*" (wildcard)
	// Source: SignalProcessing.Status.ClusterClassification, propagated via RO/AA/KA
	// BR-FLEET-003, Issue #1511, DD-FLEET-002: unlike severity/environment/component/priority,
	// this field is optional at the schema level -- non-fleet deployments never set it, and
	// it is never evaluated unless fleet mode supplies a concrete filter value.
	Cluster []string `yaml:"cluster,omitempty" json:"cluster,omitempty" validate:"omitempty"`
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

	// ServiceAccountName is the pre-existing ServiceAccount for the Job/PipelineRun.
	// DD-WE-005 v2.0: Operators pre-create SAs with appropriate RBAC in the
	// execution namespace. If absent, K8s assigns the namespace's default SA.
	// Ignored for ansible engine (AWX uses its own credential system).
	// +optional
	ServiceAccountName string `yaml:"serviceAccountName,omitempty" json:"serviceAccountName,omitempty"`

	// EngineConfig holds engine-specific configuration as parsed YAML/JSON.
	// BR-WE-016: Discriminator pattern — the Engine field determines the shape.
	// Stored as interface{} for YAML compatibility; converted to json.RawMessage by the parser.
	EngineConfig interface{} `yaml:"engineConfig,omitempty" json:"engineConfig,omitempty"`

	// Resources declares CPU/memory requests and limits for the "workflow"
	// container (Job engine only). BR-WE-019 / DD-WE-008.
	// +optional
	Resources *ResourcesSchema `yaml:"resources,omitempty" json:"resources,omitempty"`
}

// ResourcesSchema declares CPU/memory requests and limits for the "workflow"
// container (Job engine only). Plain string-keyed maps (yaml.v3-native)
// rather than corev1.ResourceRequirements directly -- resource.Quantity only
// implements the JSON codec (DD-WE-008 Finding 1); yaml.v3 would silently
// mis-parse it via struct-reflection over its unexported fields. Quantity
// strings are parsed explicitly in ExtractResources via resource.ParseQuantity.
// validate tags follow the sibling-struct convention in this file (see
// ADR-046) but are not currently enforced -- WorkflowSchema validation does
// not yet call validator.Struct() anywhere (tracked separately, issue #1591,
// not addressed by DD-WE-008).
type ResourcesSchema struct {
	// +optional
	Requests map[string]string `yaml:"requests,omitempty" json:"requests,omitempty" validate:"omitempty,dive,keys,required,endkeys,required"`
	// +optional
	Limits map[string]string `yaml:"limits,omitempty" json:"limits,omitempty" validate:"omitempty,dive,keys,required,endkeys,required"`
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
// Returns ErrNoEngineConfig when raw is empty, regardless of engine.
// Returns error for unknown engines or invalid/incomplete ansible config.
func ParseEngineConfig(engine string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, ErrNoEngineConfig
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
		// tekton/job engines don't have a typed engineConfig struct at all.
		return nil, ErrNoEngineConfig
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
	"virtualMachine":  {isBoolean: true},
	"liveMigratable":  {isBoolean: true},
	"cdiManaged":      {isBoolean: true},
	"storageBackend":  {validValues: map[string]bool{"odf-ceph": true, "lvms": true, "local": true, "*": true}},
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
	VirtualMachine  string `yaml:"-" json:"virtualMachine,omitempty"`
	LiveMigratable  string `yaml:"-" json:"liveMigratable,omitempty"`
	CDIManaged      string `yaml:"-" json:"cdiManaged,omitempty"`
	StorageBackend  string `yaml:"-" json:"storageBackend,omitempty"`

	// PopulatedFields records which fields the author explicitly declared.
	// Mirrors the Signal Processing pattern for distinguishing "empty" from
	// "not defined" at the field level.
	PopulatedFields []string `yaml:"-" json:"-"`
}

// UnmarshalYAML implements custom YAML unmarshaling for DetectedLabelsSchema.
// Iterates over the raw YAML map to track which fields were explicitly declared
// (PopulatedFields) while assigning values to typed struct fields.
// detectedLabelsFieldSetters maps each known detectedLabels field name to a
// setter that assigns the raw YAML scalar value to the corresponding struct
// field. Registry pattern used by UnmarshalYAML in place of a large switch
// statement (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 3) — pure code motion, no
// behavior change.
var detectedLabelsFieldSetters = map[string]func(*DetectedLabelsSchema, string){
	"gitOpsManaged":   func(d *DetectedLabelsSchema, v string) { d.GitOpsManaged = v },
	"gitOpsTool":      func(d *DetectedLabelsSchema, v string) { d.GitOpsTool = v },
	"pdbProtected":    func(d *DetectedLabelsSchema, v string) { d.PDBProtected = v },
	"hpaEnabled":      func(d *DetectedLabelsSchema, v string) { d.HPAEnabled = v },
	"stateful":        func(d *DetectedLabelsSchema, v string) { d.Stateful = v },
	"helmManaged":     func(d *DetectedLabelsSchema, v string) { d.HelmManaged = v },
	"networkIsolated": func(d *DetectedLabelsSchema, v string) { d.NetworkIsolated = v },
	"serviceMesh":     func(d *DetectedLabelsSchema, v string) { d.ServiceMesh = v },
	"virtualMachine":  func(d *DetectedLabelsSchema, v string) { d.VirtualMachine = v },
	"liveMigratable":  func(d *DetectedLabelsSchema, v string) { d.LiveMigratable = v },
	"cdiManaged":      func(d *DetectedLabelsSchema, v string) { d.CDIManaged = v },
	"storageBackend":  func(d *DetectedLabelsSchema, v string) { d.StorageBackend = v },
}

func (d *DetectedLabelsSchema) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.MappingNode {
		return fmt.Errorf("detectedLabels must be a mapping, got %v", value.Kind)
	}

	d.PopulatedFields = make([]string, 0, len(value.Content)/2)

	for i := 0; i < len(value.Content)-1; i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]
		key := keyNode.Value

		setter, known := detectedLabelsFieldSetters[key]
		if !known {
			return NewSchemaValidationError("detectedLabels."+key,
				fmt.Sprintf("unknown field %q in detectedLabels", key))
		}

		d.PopulatedFields = append(d.PopulatedFields, key)
		setter(d, valNode.Value)
	}

	return nil
}

// MarshalYAML implements custom YAML marshaling for DetectedLabelsSchema.
// Symmetric with UnmarshalYAML: emits only the fields listed in PopulatedFields,
// preserving the distinction between "field absent" and "field present with value".
// Issue #330: Enables round-trip marshal/unmarshal for test fixture builders.
func (d DetectedLabelsSchema) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, field := range d.PopulatedFields {
		val := d.fieldValue(field)
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: field},
			&yaml.Node{Kind: yaml.ScalarNode, Value: val},
		)
	}
	return node, nil
}

// fieldValue returns the raw string value for a given field name.
// Centralizes the field-name-to-struct-field mapping used by
// UnmarshalYAML (write), MarshalYAML (read), and ValidateDetectedLabels (read).
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
	case "virtualMachine":
		return d.VirtualMachine
	case "liveMigratable":
		return d.LiveMigratable
	case "cdiManaged":
		return d.CDIManaged
	case "storageBackend":
		return d.StorageBackend
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
	// DD-WORKFLOW-001 v2.8: "*" wildcard restored for severity (matches any level)
	// ADR-066: "medium" replaced by "warning"
	allowedSeverities := map[string]bool{"critical": true, "high": true, "warning": true, "info": true, "*": true}
	for _, sev := range l.Severity {
		if !allowedSeverities[sev] {
			return NewSchemaValidationError("labels.severity",
				fmt.Sprintf("invalid severity %q: must be one of critical, high, warning, info, * (wildcard)", sev))
		}
	}
	if len(l.Environment) == 0 {
		return NewSchemaValidationError("labels.environment", "environment is required (at least one value)")
	}
	if len(l.Component) == 0 {
		return NewSchemaValidationError("labels.component", "component is required (at least one value)")
	}
	// #1051: validate GVK format for component values. Must be "apiVersion/Kind"
	// (e.g. "apps/v1/Deployment", "v1/Pod") or "*" wildcard.
	for _, comp := range l.Component {
		if comp == "*" {
			continue
		}
		if !isValidComponentGVK(comp) {
			return NewSchemaValidationError("labels.component",
				fmt.Sprintf("invalid component %q: must be in GVK format (e.g. \"apps/v1/Deployment\", \"v1/Pod\") or \"*\" wildcard", comp))
		}
	}
	if l.Priority == "" {
		return NewSchemaValidationError("labels.priority", "priority is required")
	}
	return nil
}

// isValidComponentGVK checks whether a component value follows the GVK format:
// "version/Kind" for core group (e.g. "v1/Pod") or "group/version/Kind" for
// named groups (e.g. "apps/v1/Deployment"). The Kind part must start with an
// uppercase letter. Rejects control characters and NUL bytes (FedRAMP SI-10).
// Issue #1051.
func isValidComponentGVK(s string) bool {
	parts := strings.Split(s, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < 0x20 || r == 0x7f {
				return false
			}
		}
	}
	kind := parts[len(parts)-1]
	if kind[0] < 'A' || kind[0] > 'Z' {
		return false
	}
	return true
}

// ValidateDescription checks if the structured description has required fields.
// BR-WORKFLOW-004: what and whenToUse are required.
func ValidateDescription(d *WorkflowDescription) error {
	if d.What == "" {
		return NewSchemaValidationError("description.what", "what is required")
	}
	if d.WhenToUse == "" {
		return NewSchemaValidationError("description.whenToUse", "whenToUse is required")
	}
	return nil
}

const (
	parameterTypeInteger = "integer"
	parameterTypeFloat   = "float"
	parameterTypeString  = "string"
	parameterTypeBoolean = "boolean"
	parameterTypeArray   = "array"
)

// ValidateParameterValue checks a string parameter value against the schema's type and bounds.
// Returns nil if the value is valid; error with descriptive message otherwise.
func ValidateParameterValue(param WorkflowParameter, value string) error {
	switch param.Type {
	case parameterTypeInteger:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil || !json.Valid([]byte(value)) {
			return fmt.Errorf("parameter %q: invalid integer value %q", param.Name, value)
		}
		return checkNumericBounds(param, float64(v), &v)
	case parameterTypeFloat:
		return validateFloatParameterValue(param, value)
	case parameterTypeString:
		return nil
	case parameterTypeBoolean:
		if value != "true" && value != "false" {
			return fmt.Errorf("parameter %q: invalid boolean value %q", param.Name, value)
		}
		return nil
	case parameterTypeArray:
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(value), &items); err != nil || items == nil {
			return fmt.Errorf("parameter %q: invalid array value %q", param.Name, value)
		}
		return nil
	default:
		return fmt.Errorf("parameter %q: unsupported type %q", param.Name, param.Type)
	}
}

// ValidateParameterSet validates the complete effective string-valued parameter
// map without modifying it or inserting schema defaults (BR-KA-191).
// TypedParameterValue adapts a KA/AF typed value to the schema-directed string
// validator without mutating the value used for the execution handoff.
func TypedParameterValue(parameter WorkflowParameter, value interface{}) (string, error) {
	switch parameter.Type {
	case parameterTypeString:
		if v, ok := value.(string); ok {
			return v, nil
		}
	case parameterTypeBoolean:
		if v, ok := value.(bool); ok {
			return strconv.FormatBool(v), nil
		}
	case parameterTypeArray:
		if v, ok := value.([]interface{}); ok && v != nil {
			raw, err := json.Marshal(v)
			if err != nil {
				return "", fmt.Errorf("parameter %q: cannot encode array: %w", parameter.Name, err)
			}
			return string(raw), nil
		}
	case parameterTypeInteger, parameterTypeFloat:
		return typedNumericParameterValue(parameter, value)
	}
	return "", fmt.Errorf("%s: expected type %s, got %T", parameter.Name, parameter.Type, value)
}

func typedNumericParameterValue(parameter WorkflowParameter, value interface{}) (string, error) {
	var raw string
	switch v := value.(type) {
	case json.Number:
		raw = string(v)
		if parameter.Type == parameterTypeInteger && strings.ContainsAny(raw, ".eE") {
			return "", fmt.Errorf("parameter %q: integer requires integer JSON text", parameter.Name)
		}
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || (parameter.Type == parameterTypeInteger && math.Abs(v) > 9007199254740992) {
			return "", fmt.Errorf("parameter %q: invalid or imprecise numeric value", parameter.Name)
		}
		raw = strconv.FormatFloat(v, 'g', -1, 64)
	case int:
		raw = strconv.FormatInt(int64(v), 10)
	case int64:
		raw = strconv.FormatInt(v, 10)
	default:
		return "", fmt.Errorf("%s: expected type %s, got %T", parameter.Name, parameter.Type, value)
	}
	if parameter.Type == parameterTypeInteger {
		integer, err := typedIntegerValue(raw)
		if err != nil {
			return "", fmt.Errorf("parameter %q: invalid integer: %w", parameter.Name, err)
		}
		return integer, nil
	}
	if err := ValidateParameterValue(WorkflowParameter{Name: parameter.Name, Type: parameterTypeFloat}, raw); err != nil {
		return "", err
	}
	return raw, nil
}

// typedIntegerValue accepts integral JSON numeric values, including decimal and
// exponent notation, and checks int64 range without converting through float64.
func typedIntegerValue(raw string) (string, error) {
	if len(raw) == 0 || !json.Valid([]byte(raw)) || (raw[0] != '-' && (raw[0] < '0' || raw[0] > '9')) {
		return "", fmt.Errorf("invalid JSON number")
	}
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(raw), "e")
	negative := strings.HasPrefix(mantissa, "-")
	mantissa = strings.TrimPrefix(mantissa, "-")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return "0", nil
	}
	var exponent int64
	if hasExponent {
		var err error
		exponent, err = strconv.ParseInt(exponentText, 10, 64)
		if err != nil {
			return "", fmt.Errorf("integer exponent out of range")
		}
	}
	if exponent < math.MinInt64+int64(len(fraction)) {
		return "", fmt.Errorf("integer is fractional")
	}
	scale := exponent - int64(len(fraction))
	digits, err := scaleIntegerDigits(digits, scale)
	if err != nil {
		return "", err
	}
	if negative {
		digits = "-" + digits
	}
	v, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return "", fmt.Errorf("integer out of range: %w", err)
	}
	return strconv.FormatInt(v, 10), nil
}

func scaleIntegerDigits(digits string, scale int64) (string, error) {
	if scale < 0 {
		if scale < -int64(len(digits)) {
			return "", fmt.Errorf("integer is fractional")
		}
		cut := len(digits) + int(scale)
		if strings.Trim(digits[cut:], "0") != "" {
			return "", fmt.Errorf("integer is fractional")
		}
		return digits[:cut], nil
	}
	if scale > 19 || int64(len(digits))+scale > 19 {
		return "", fmt.Errorf("integer out of range")
	}
	return digits + strings.Repeat("0", int(scale)), nil
}

func ValidateParameterSet(schema []WorkflowParameter, values map[string]string) error {
	validationErrors := ValidateParameterSetErrors(schema, values)
	if len(validationErrors) > 0 {
		return validationErrors[0]
	}
	return nil
}

// ValidateParameterSetErrors returns all constraint failures without mutating inputs.
func ValidateParameterSetErrors(schema []WorkflowParameter, values map[string]string) []error {
	if schema == nil || values == nil {
		return []error{fmt.Errorf("parameter schema or values are missing")}
	}
	var errs []error
	declared := make(map[string]WorkflowParameter, len(schema))
	patterns := make(map[string]*regexp.Regexp, len(schema))
	for _, parameter := range schema {
		if parameter.Name == "" {
			errs = append(errs, fmt.Errorf("parameter schema contains an empty name"))
			continue
		}
		if _, exists := declared[parameter.Name]; exists {
			errs = append(errs, fmt.Errorf("parameter %q is declared more than once", parameter.Name))
			continue
		}
		declared[parameter.Name] = parameter
		pattern, schemaErrors := validateParameterSchema(parameter)
		errs = append(errs, schemaErrors...)
		if pattern != nil {
			patterns[parameter.Name] = pattern
		}
	}
	if len(errs) > 0 {
		return errs
	}
	for key := range values {
		if _, exists := declared[key]; !exists {
			errs = append(errs, fmt.Errorf("parameter %q is not declared", key))
		}
	}
	for _, parameter := range schema {
		errs = append(errs, validateParameterConstraints(parameter, values, patterns[parameter.Name])...)
	}
	return errs
}

func validateParameterSchema(parameter WorkflowParameter) (*regexp.Regexp, []error) {
	var errs []error
	switch parameter.Type {
	case parameterTypeString, parameterTypeInteger, parameterTypeFloat, parameterTypeBoolean, parameterTypeArray:
	default:
		errs = append(errs, fmt.Errorf("parameter %q: unsupported type %q", parameter.Name, parameter.Type))
	}
	errs = append(errs, validateSchemaBounds(parameter)...)
	if parameter.Pattern == "" {
		return nil, errs
	}
	if parameter.Type != parameterTypeString {
		return nil, append(errs, fmt.Errorf("parameter %q: pattern requires a string type", parameter.Name))
	}
	if len(parameter.Pattern) > 1024 {
		return nil, append(errs, fmt.Errorf("parameter %q: pattern exceeds 1024 characters", parameter.Name))
	}
	pattern, err := regexp.Compile(parameter.Pattern)
	if err != nil {
		return nil, append(errs, fmt.Errorf("parameter %q: invalid pattern: %w", parameter.Name, err))
	}
	return pattern, errs
}

func validateSchemaBounds(parameter WorkflowParameter) []error {
	var errs []error
	if (parameter.Minimum != nil || parameter.Maximum != nil) && parameter.Type != parameterTypeInteger && parameter.Type != parameterTypeFloat {
		errs = append(errs, fmt.Errorf("parameter %q: numeric bounds require a numeric type", parameter.Name))
	}
	if (parameter.Minimum != nil && (math.IsNaN(*parameter.Minimum) || math.IsInf(*parameter.Minimum, 0))) ||
		(parameter.Maximum != nil && (math.IsNaN(*parameter.Maximum) || math.IsInf(*parameter.Maximum, 0))) {
		errs = append(errs, fmt.Errorf("parameter %q: invalid numeric bounds", parameter.Name))
	} else if parameter.Minimum != nil && parameter.Maximum != nil && *parameter.Minimum > *parameter.Maximum {
		errs = append(errs, fmt.Errorf("parameter %q: minimum exceeds maximum", parameter.Name))
	}
	return errs
}

func validateParameterConstraints(parameter WorkflowParameter, values map[string]string, pattern *regexp.Regexp) []error {
	value, present := values[parameter.Name]
	if !present {
		if parameter.Required {
			return []error{fmt.Errorf("parameter %q: required parameter is missing", parameter.Name)}
		}
		return nil
	}
	if err := ValidateParameterValue(parameter, value); err != nil {
		return []error{err}
	}
	var errs []error
	if len(parameter.Enum) > 0 && !slices.Contains(parameter.Enum, value) {
		errs = append(errs, fmt.Errorf("parameter %q: value is not in enum", parameter.Name))
	}
	if pattern != nil && !pattern.MatchString(value) {
		errs = append(errs, fmt.Errorf("parameter %q: value does not match pattern", parameter.Name))
	}
	for _, dependency := range parameter.DependsOn {
		if _, exists := values[dependency]; !exists {
			errs = append(errs, fmt.Errorf("parameter %q: depends on %q which is not present", parameter.Name, dependency))
		}
	}
	return errs
}

func validateFloatParameterValue(param WorkflowParameter, value string) error {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil || !json.Valid([]byte(value)) || math.IsInf(v, 0) || math.IsNaN(v) {
		return fmt.Errorf("parameter %q: invalid float value %q", param.Name, value)
	}
	if v == 0 {
		mantissa, _, _ := strings.Cut(strings.ToLower(value), "e")
		if strings.ContainsAny(mantissa, "123456789") {
			return fmt.Errorf("parameter %q: float value %q underflows", param.Name, value)
		}
	}
	return checkNumericBounds(param, v, nil)
}

func checkNumericBounds(param WorkflowParameter, value float64, integer *int64) error {
	if (param.Minimum != nil && (math.IsNaN(*param.Minimum) || math.IsInf(*param.Minimum, 0))) ||
		(param.Maximum != nil && (math.IsNaN(*param.Maximum) || math.IsInf(*param.Maximum, 0))) {
		return fmt.Errorf("parameter %q: invalid numeric bounds", param.Name)
	}
	if integer != nil {
		return checkIntegerBounds(param, *integer)
	}
	if param.Minimum != nil && value < *param.Minimum {
		return fmt.Errorf("parameter %q: value %v is below minimum %v", param.Name, value, *param.Minimum)
	}
	if param.Maximum != nil && value > *param.Maximum {
		return fmt.Errorf("parameter %q: value %v exceeds maximum %v", param.Name, value, *param.Maximum)
	}
	return nil
}

func checkIntegerBounds(param WorkflowParameter, value int64) error {
	exact := new(big.Rat).SetInt64(value)
	if param.Minimum != nil && exact.Cmp(new(big.Rat).SetFloat64(*param.Minimum)) < 0 {
		return fmt.Errorf("parameter %q: value %d is below minimum %v", param.Name, value, *param.Minimum)
	}
	if param.Maximum != nil && exact.Cmp(new(big.Rat).SetFloat64(*param.Maximum)) > 0 {
		return fmt.Errorf("parameter %q: value %d exceeds maximum %v", param.Name, value, *param.Maximum)
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
