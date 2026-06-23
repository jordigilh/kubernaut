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

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ========================================
// WORKFLOW LABEL TYPES (V1.0 - STRUCTURED)
// ========================================
// Authority: DD-WORKFLOW-001 v2.3 (Mandatory Label Schema)
// Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
// V1.0: Eliminates unstructured map[string]interface{} for type safety
// ========================================

// MandatoryLabels represents the workflow labels stored in the catalog JSONB column.
// 4 required fields: severity, component, environment, priority.
// Authority: DD-WORKFLOW-001 v1.4, DD-WORKFLOW-016
// Issue #274: signalName removed — LLM selects by actionType, not signalName.
type MandatoryLabels struct {
	// Severity is the severity level(s) this workflow is designed for (REQUIRED)
	// Values: "critical", "high", "warning", "info", "*" (wildcard for all)
	// Source: Alert/Event (auto-populated by Signal Processing)
	// DD-WORKFLOW-001 v2.8: Always stored as JSONB array. Supports "*" wildcard (like environment).
	Severity []string `json:"severity" validate:"required,min=1"`

	// Component is the Kubernetes resource GVK(s) this workflow remediates (REQUIRED)
	// Format: "apiVersion/Kind" — e.g. "apps/v1/Deployment", "v1/Pod" (core group omits group prefix)
	// Examples: ["apps/v1/Deployment"], ["v1/Pod", "apps/v1/StatefulSet"], ["*"] (wildcard for all)
	// Source: K8s Resource (auto-populated by Signal Processing)
	// Issue #790: Changed from string to []string to match severity/environment pattern
	// Issue #1051: Changed from plain lowercase kind to fully-qualified GVK format
	Component []string `json:"component" validate:"required,min=1"`

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

// DetectedLabels is the DB-oriented label detection type (DD-WORKFLOW-001 v2.3).
// Derived from sharedtypes.DetectedLabels with sparse JSON serialization for JSONB storage.
// The canonical CRD type (sharedtypes.DetectedLabels) uses full JSON with all fields present;
// this type omits false booleans and empty strings for efficient JSONB storage.
type DetectedLabels sharedtypes.DetectedLabels

// NewDetectedLabels creates a DetectedLabels with an initialized (non-nil) FailedDetections slice.
func NewDetectedLabels() *DetectedLabels {
	return &DetectedLabels{
		FailedDetections: make([]string, 0),
	}
}

// SerializeLabels produces sparse JSON, omitting false boolean fields.
func (d DetectedLabels) SerializeLabels() ([]byte, error) {
	m := make(map[string]interface{}, 14)
	if len(d.FailedDetections) > 0 {
		m["failedDetections"] = d.FailedDetections
	}
	if d.GitOpsManaged {
		m["gitOpsManaged"] = true
	}
	if d.GitOpsTool != "" {
		m["gitOpsTool"] = d.GitOpsTool
	}
	if d.PDBProtected {
		m["pdbProtected"] = true
	}
	if d.HPAEnabled {
		m["hpaEnabled"] = true
	}
	if d.Stateful {
		m["stateful"] = true
	}
	if d.HelmManaged {
		m["helmManaged"] = true
	}
	if d.NetworkIsolated {
		m["networkIsolated"] = true
	}
	if d.ServiceMesh != "" {
		m["serviceMesh"] = d.ServiceMesh
	}
	if d.ResourceQuotaConstrained {
		m["resourceQuotaConstrained"] = true
	}
	if d.VirtualMachine {
		m["virtualMachine"] = true
	}
	if d.LiveMigratable {
		m["liveMigratable"] = true
	}
	if d.CDIManaged {
		m["cdiManaged"] = true
	}
	if d.StorageBackend != "" {
		m["storageBackend"] = d.StorageBackend
	}
	return json.Marshal(m)
}

// MarshalJSON implements json.Marshaler for sparse DB-oriented output.
// Value receiver ensures the interface is satisfied when boxed in interface{}.
func (d DetectedLabels) MarshalJSON() ([]byte, error) {
	return d.SerializeLabels()
}

// Value implements driver.Valuer for PostgreSQL JSONB column writing.
// Value receiver ensures the interface is satisfied when boxed in interface{}.
func (d DetectedLabels) Value() (driver.Value, error) {
	return d.SerializeLabels()
}

// Scan implements sql.Scanner for PostgreSQL JSONB column scanning.
func (d *DetectedLabels) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("DetectedLabels.Scan: expected []byte, got %T", value)
	}
	type Alias DetectedLabels
	return json.Unmarshal(bytes, (*Alias)(d))
}

// IsEmpty returns true when no label detection produced a positive result.
func (d *DetectedLabels) IsEmpty() bool {
	if d == nil {
		return true
	}
	return !d.GitOpsManaged &&
		d.GitOpsTool == "" &&
		!d.PDBProtected &&
		!d.HPAEnabled &&
		!d.Stateful &&
		!d.HelmManaged &&
		!d.NetworkIsolated &&
		d.ServiceMesh == "" &&
		!d.VirtualMachine &&
		!d.LiveMigratable &&
		!d.CDIManaged &&
		d.StorageBackend == ""
}

// ========================================
// STRUCTURED DESCRIPTION (BR-WORKFLOW-004)
// ========================================
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// Stored as JSONB in the description column (migration 026)
// Matches action_type_taxonomy.description format (DD-WORKFLOW-016)

// StructuredDescription provides structured workflow information for LLM and operators.
// This is stored as JSONB in the description column of remediation_workflow_catalog.
//
// This is a DB-scannable variant of sharedtypes.StructuredDescription that adds
// custom UnmarshalJSON (backward-compat with plain strings), sql.Scanner, and
// driver.Valuer implementations. Use ToShared() / FromSharedDescription() to convert.
type StructuredDescription struct {
	What          string `json:"what" validate:"required"`
	WhenToUse     string `json:"whenToUse" validate:"required"`
	WhenNotToUse  string `json:"whenNotToUse,omitempty"`
	Preconditions string `json:"preconditions,omitempty"`
}

// ToShared converts to the canonical shared StructuredDescription type.
func (d StructuredDescription) ToShared() sharedtypes.StructuredDescription {
	return sharedtypes.StructuredDescription{
		What:          d.What,
		WhenToUse:     d.WhenToUse,
		WhenNotToUse:  d.WhenNotToUse,
		Preconditions: d.Preconditions,
	}
}

// FromSharedDescription creates a DB-scannable StructuredDescription from the shared type.
func FromSharedDescription(d sharedtypes.StructuredDescription) StructuredDescription {
	return StructuredDescription{
		What:          d.What,
		WhenToUse:     d.WhenToUse,
		WhenNotToUse:  d.WhenNotToUse,
		Preconditions: d.Preconditions,
	}
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
// DD-WORKFLOW-001 v2.8: severity is []string (always array, supports "*" wildcard like environment)
// Issue #274: signalName parameter removed — LLM selects by actionType.
// Issue #790: component is now []string (matches severity/environment pattern)
func NewMandatoryLabels(severity []string, component []string, environment []string, priority string) *MandatoryLabels {
	return &MandatoryLabels{
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

// IsEmpty checks if CustomLabels has no subdomains
func (c CustomLabels) IsEmpty() bool {
	return len(c) == 0
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
	"resourceQuotaConstrained",
	"virtualMachine",
	"liveMigratable",
	"cdiManaged",
	"storageBackend",
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

