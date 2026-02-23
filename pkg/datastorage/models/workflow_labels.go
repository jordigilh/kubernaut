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
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// ========================================
// WORKFLOW LABEL TYPES (V1.0 - STRUCTURED)
// ========================================
// Authority: DD-WORKFLOW-001 v2.3 (Mandatory Label Schema)
// Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
// V1.0: Eliminates unstructured map[string]interface{} for type safety
// ========================================

// MandatoryLabels represents the workflow labels stored in the catalog JSONB column.
// 4 required fields (severity, component, environment, priority) + 1 optional (signalName).
// Authority: DD-WORKFLOW-001 v1.4, DD-WORKFLOW-016 (signalName made optional)
type MandatoryLabels struct {
	// SignalName is the signal type this workflow handles (REQUIRED)
	// Examples: "OOMKilled", "CrashLoopBackOff", "NodeNotReady"
	// Source: K8s Event Reason (auto-populated by Signal Processing)
	SignalName string `json:"signalType" validate:"required"`

	// Severity is the severity level(s) this workflow is designed for (REQUIRED)
	// Values: "critical", "high", "medium", "low"
	// Source: Alert/Event (auto-populated by Signal Processing)
	// DD-WORKFLOW-001 v2.7: Always stored as JSONB array. No wildcard.
	Severity []string `json:"severity" validate:"required,min=1"`

	// Component is the Kubernetes resource type this workflow remediates (REQUIRED)
	// Examples: "pod", "deployment", "node", "service", "pvc"
	// Source: K8s Resource (auto-populated by Signal Processing)
	Component string `json:"component" validate:"required"`

	// Environment is the deployment environment(s) this workflow targets (REQUIRED)
	// Values: ["production"], ["staging", "production"], ["*"] (wildcard for all)
	// Source: Workflow author declares target environments
	// DD-WORKFLOW-001 v2.5: Array allows workflows to work in multiple environments
	Environment []string `json:"environment" validate:"required,min=1"`

	// Priority is the business priority level (REQUIRED)
	// Values: "P0", "P1", "P2", "P3", "*" (wildcard)
	// Source: Derived from severity + environment (Signal Processing via Rego)
	Priority string `json:"priority" validate:"required"`
}

// CustomLabels represents customer-defined labels via Rego policies (DD-WORKFLOW-001 v1.5)
// Format: map[subdomain][]values
// Example: {"constraint": ["cost-constrained"], "team": ["name=payments"]}
//
// Validation Limits (DD-WORKFLOW-001 v1.9):
// - Max 10 subdomains (keys)
// - Max 5 values per subdomain
// - Max 63 characters per subdomain key
// - Max 100 characters per value
//
// Common Subdomains:
// - "constraint": Workflow constraints (e.g., "cost-constrained", "stateful-safe")
// - "team": Team ownership (e.g., "name=payments", "name=platform")
// - "risk_tolerance": Risk tolerance level (e.g., "low", "medium", "high")
// - "business_category": Business domain (e.g., "payment-service", "analytics")
// - "region": Geographic region (e.g., "us-west-2", "eu-central-1")
type CustomLabels map[string][]string

// Value implements the driver.Valuer interface for database storage
// Returns '{}' for empty map instead of NULL to satisfy NOT NULL constraint
func (c CustomLabels) Value() (driver.Value, error) {
	if len(c) == 0 {
		return []byte("{}"), nil // ✅ Empty JSON object, not NULL
	}
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for database retrieval
func (c *CustomLabels) Scan(value interface{}) error {
	if value == nil {
		*c = CustomLabels{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal CustomLabels value: %v", value)
	}
	return json.Unmarshal(bytes, c)
}

// DetectedLabels represents auto-detected labels from Kubernetes resources (DD-WORKFLOW-001 v1.6)
// V2.3: 8 auto-detected fields with detection failure handling
// Detection is performed by SignalProcessing at incident time (NOT by Data Storage)
//
// Boolean Normalization Rule (DD-WORKFLOW-001 v1.5):
// - Booleans only included when true
// - False values are omitted from JSON
//
// Wildcard Support (DD-WORKFLOW-001 v1.6):
// - String fields (GitOpsTool, ServiceMesh) support "*" wildcard
// - "*" means "requires SOME value" (not specific)
// - Absent field means "no requirement"
type DetectedLabels struct {
	// FailedDetections lists fields where detection failed (RBAC, timeout, etc.)
	// If a field name is in this array, its value should be ignored
	// If empty/nil, all detections succeeded
	// Validated: only accepts values from ValidDetectedLabelFields
	// Authority: DD-WORKFLOW-001 v2.1 (Detection Failure Handling)
	FailedDetections []string `json:"failedDetections,omitempty" validate:"omitempty,dive,oneof=gitOpsManaged gitOpsTool pdbProtected hpaEnabled stateful helmManaged networkIsolated serviceMesh"`

	// ========================================
	// GITOPS MANAGEMENT (DD-WORKFLOW-001 v2.3)
	// ========================================

	// GitOpsManaged indicates if resource is managed by GitOps (ArgoCD/Flux)
	// Detection: Check annotations:
	//   - ArgoCD: "argocd.argoproj.io/instance" exists
	//   - Flux: "kustomize.toolkit.fluxcd.io/name" exists
	// API Call: kubectl get <resource> -o jsonpath='{.metadata.annotations}'
	GitOpsManaged bool `json:"gitOpsManaged,omitempty"`

	// GitOpsTool is the specific GitOps tool if detected
	// Values: "argocd", "flux", "*" (wildcard = any tool)
	// Detection: Based on annotation prefix:
	//   - ArgoCD: "argocd.argoproj.io/" prefix
	//   - Flux: "kustomize.toolkit.fluxcd.io/" or "helm.toolkit.fluxcd.io/" prefix
	// API Call: Same as GitOpsManaged (annotation-based)
	GitOpsTool string `json:"gitOpsTool,omitempty" validate:"omitempty,oneof=argocd flux *"`

	// ========================================
	// WORKLOAD PROTECTION (DD-WORKFLOW-001 v2.3)
	// ========================================

	// PDBProtected indicates if a PodDisruptionBudget protects this workload
	// Detection: Check for PDB in namespace matching workload selector
	// API Call: kubectl get pdb -n <namespace> -o json
	// Match: PDB .spec.selector matches workload .spec.selector
	PDBProtected bool `json:"pdbProtected,omitempty"`

	// HPAEnabled indicates if a HorizontalPodAutoscaler is configured
	// Detection: Check for HPA in namespace targeting this workload
	// API Call: kubectl get hpa -n <namespace> -o json
	// Match: HPA .spec.scaleTargetRef matches workload name/kind
	HPAEnabled bool `json:"hpaEnabled,omitempty"`

	// ========================================
	// WORKLOAD CHARACTERISTICS (DD-WORKFLOW-001 v2.3)
	// ========================================

	// Stateful indicates if workload uses persistent storage or is StatefulSet
	// Detection: Check owner chain for StatefulSet OR check for PVC mounts
	// Method: Owner chain traversal (NO K8s API call)
	//   - If owner_chain contains StatefulSet → stateful = true
	//   - Else if Pod has volumeMounts referencing PVCs → stateful = true
	// Clarification: Uses owner chain, not direct StatefulSet lookup
	Stateful bool `json:"stateful,omitempty"`

	// HelmManaged indicates if resource is managed by Helm
	// Detection: Check annotations:
	//   - "meta.helm.sh/release-name" exists
	//   - "meta.helm.sh/release-namespace" exists
	// API Call: kubectl get <resource> -o jsonpath='{.metadata.annotations}'
	HelmManaged bool `json:"helmManaged,omitempty"`

	// ========================================
	// SECURITY POSTURE (DD-WORKFLOW-001 v2.3)
	// ========================================

	// NetworkIsolated indicates if NetworkPolicy restricts traffic
	// Detection: Check for NetworkPolicy in namespace selecting this Pod
	// API Call: kubectl get networkpolicy -n <namespace> -o json
	// Match: NetworkPolicy .spec.podSelector matches Pod labels
	NetworkIsolated bool `json:"networkIsolated,omitempty"`

	// ServiceMesh is the service mesh type if detected
	// Values: "istio", "linkerd", "*" (wildcard = any mesh)
	// Detection: Check pod annotations for sidecar injection:
	//   - Istio: "sidecar.istio.io/status" exists (present after injection)
	//   - Linkerd: "linkerd.io/proxy-version" exists (present after injection)
	// API Call: kubectl get pod -o jsonpath='{.metadata.annotations}'
	// Clarification: Uses annotations, not direct mesh API checks
	ServiceMesh string `json:"serviceMesh,omitempty" validate:"omitempty,oneof=istio linkerd *"`
}

// ========================================
// STRUCTURED DESCRIPTION (BR-WORKFLOW-004)
// ========================================
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// Stored as JSONB in the description column (migration 026)
// Matches action_type_taxonomy.description format (DD-WORKFLOW-016)

// StructuredDescription provides structured workflow information for LLM and operators.
// This is stored as JSONB in the description column of remediation_workflow_catalog.
type StructuredDescription struct {
	// What describes what this workflow concretely does. One sentence. (REQUIRED)
	What string `json:"what" validate:"required"`

	// WhenToUse describes root cause conditions under which this workflow is appropriate. (REQUIRED)
	WhenToUse string `json:"whenToUse" validate:"required"`

	// WhenNotToUse describes specific exclusion conditions. (OPTIONAL)
	WhenNotToUse string `json:"whenNotToUse,omitempty"`

	// Preconditions describes conditions that must be verified through investigation. (OPTIONAL)
	Preconditions string `json:"preconditions,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling for StructuredDescription
// Handles both formats:
// - string: {"description": "some text"} -> StructuredDescription{What: "some text"}
// - object: {"description": {"what": "...", "whenToUse": "..."}} -> StructuredDescription{...}
// This enables backward compatibility with API clients that send plain string descriptions
func (d *StructuredDescription) UnmarshalJSON(data []byte) error {
	// Try as string first (backward compatibility)
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		d.What = s
		return nil
	}

	// Try as structured object
	type Alias StructuredDescription // Prevent infinite recursion
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return fmt.Errorf("description must be a string or structured object: %w", err)
	}
	*d = StructuredDescription(a)
	return nil
}

// Scan implements sql.Scanner for StructuredDescription
// Allows scanning JSONB column data into structured StructuredDescription type
func (d *StructuredDescription) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan StructuredDescription: expected []byte, got %T", value)
	}

	return json.Unmarshal(bytes, d)
}

// Value implements driver.Valuer for StructuredDescription
// Allows writing structured StructuredDescription type to JSONB column
func (d StructuredDescription) Value() (driver.Value, error) {
	return json.Marshal(d)
}

// String returns a human-readable flat text representation of the description
func (d StructuredDescription) String() string {
	return d.What
}

// ========================================
// HELPER FUNCTIONS
// ========================================

// NewMandatoryLabels creates a new MandatoryLabels instance
// DD-WORKFLOW-001 v2.5: environment is []string (workflow declares target environments)
// DD-WORKFLOW-001 v2.7: severity is []string (always array, no wildcard)
func NewMandatoryLabels(signalName string, severity []string, component string, environment []string, priority string) *MandatoryLabels {
	return &MandatoryLabels{
		SignalName:  signalType,
		Severity:    severity,
		Component:   component,
		Environment: environment,
		Priority:    priority,
	}
}

// NewCustomLabels creates a new CustomLabels instance
func NewCustomLabels() CustomLabels {
	return make(CustomLabels)
}

// NewDetectedLabels creates a new DetectedLabels instance
func NewDetectedLabels() *DetectedLabels {
	return &DetectedLabels{
		FailedDetections: make([]string, 0),
	}
}

// IsEmpty checks if CustomLabels has no subdomains
func (c CustomLabels) IsEmpty() bool {
	return len(c) == 0
}

// IsEmpty checks if DetectedLabels has no detected fields
func (d *DetectedLabels) IsEmpty() bool {
	return !d.GitOpsManaged &&
		d.GitOpsTool == "" &&
		!d.PDBProtected &&
		!d.HPAEnabled &&
		!d.Stateful &&
		!d.HelmManaged &&
		!d.NetworkIsolated &&
		d.ServiceMesh == ""
}

// ValidDetectedLabelFields is the authoritative list of valid detected label field names
// Used for validation of FailedDetections array (DD-WORKFLOW-001 v2.1)
var ValidDetectedLabelFields = []string{
	"gitOpsManaged",
	"gitOpsTool",
	"pdbProtected",
	"hpaEnabled",
	"stateful",
	"helmManaged",
	"networkIsolated",
	"serviceMesh",
}

// ========================================
// DATABASE SCANNING SUPPORT (sql.Scanner / driver.Valuer)
// ========================================
// V1.0: Enable JSONB column scanning into structured types
// These implementations allow sqlx to automatically convert
// between PostgreSQL JSONB and Go structured types
// ========================================

// Scan implements sql.Scanner for MandatoryLabels
// Allows scanning JSONB column data into structured MandatoryLabels type
func (m *MandatoryLabels) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan MandatoryLabels: expected []byte, got %T", value)
	}

	return json.Unmarshal(bytes, m)
}

// Value implements driver.Valuer for MandatoryLabels
// Allows writing structured MandatoryLabels type to JSONB column
func (m MandatoryLabels) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan implements sql.Scanner for DetectedLabels
// Allows scanning JSONB column data into structured DetectedLabels type
func (d *DetectedLabels) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan DetectedLabels: expected []byte, got %T", value)
	}

	return json.Unmarshal(bytes, d)
}

// Value implements driver.Valuer for DetectedLabels
// Allows writing structured DetectedLabels type to JSONB column
func (d *DetectedLabels) Value() (driver.Value, error) {
	if d == nil {
		return nil, nil
	}
	return json.Marshal(d)
}
