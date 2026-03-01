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

package schema

import (
	"encoding/json"
	"encoding/hex"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW SCHEMA PARSER
// ========================================
// Authority: BR-WORKFLOW-004 (Workflow Schema Format Specification)
// Design Decision: DD-WORKFLOW-017 (OCI-based Workflow Registration)
// ========================================
//
// This package provides parsing and validation for /workflow-schema.yaml
// files as defined in BR-WORKFLOW-004.
//
// The schema is a plain configuration file (not a Kubernetes resource)
// using camelCase field names per kubernaut convention.
//
// ========================================

// Parser handles parsing and validation of workflow-schema.yaml content
type Parser struct{}

// NewParser creates a new workflow schema parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses workflow-schema.yaml content and returns a WorkflowSchema
// Returns error if content is invalid YAML or doesn't match the schema structure
func (p *Parser) Parse(content string) (*models.WorkflowSchema, error) {
	if content == "" {
		return nil, fmt.Errorf("content is empty")
	}

	var schema models.WorkflowSchema
	if err := yaml.Unmarshal([]byte(content), &schema); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	return &schema, nil
}

// ParseAndValidate parses and validates workflow-schema.yaml content
// Validates:
// - Valid YAML structure
// - Required fields (metadata, actionType, labels, parameters)
// - Mandatory labels (signalType, severity, environment, component, priority)
// - Structured description (what, whenToUse required)
// Returns error if validation fails
func (p *Parser) ParseAndValidate(content string) (*models.WorkflowSchema, error) {
	schema, err := p.Parse(content)
	if err != nil {
		return nil, err
	}

	if err := p.Validate(schema); err != nil {
		return nil, err
	}

	return schema, nil
}

// Validate validates a WorkflowSchema against BR-WORKFLOW-004 requirements
func (p *Parser) Validate(schema *models.WorkflowSchema) error {
	// Validate Metadata
	if schema.Metadata.WorkflowID == "" {
		return models.NewSchemaValidationError("metadata.workflowId", "workflowId is required")
	}
	if schema.Metadata.Version == "" {
		return models.NewSchemaValidationError("metadata.version", "version is required")
	}

	// Validate structured description
	if err := schema.Metadata.Description.ValidateDescription(); err != nil {
		return err
	}

	// Validate ActionType (top-level, required)
	if schema.ActionType == "" {
		return models.NewSchemaValidationError("actionType", "actionType is required")
	}

	// Validate Labels (mandatory matching criteria)
	if err := schema.Labels.ValidateMandatoryLabels(); err != nil {
		return err
	}

	// Validate Parameters (at least one required)
	if len(schema.Parameters) == 0 {
		return models.NewSchemaValidationError("parameters", "at least one parameter is required")
	}

	// Validate each parameter has required fields
	for i, param := range schema.Parameters {
		if param.Name == "" {
			return models.NewSchemaValidationError(
				fmt.Sprintf("parameters[%d].name", i),
				"parameter name is required",
			)
		}
		if param.Type == "" {
			return models.NewSchemaValidationError(
				fmt.Sprintf("parameters[%d].type", i),
				"parameter type is required",
			)
		}
		if param.Description == "" {
			return models.NewSchemaValidationError(
				fmt.Sprintf("parameters[%d].description", i),
				"parameter description is required",
			)
		}
	}

	// Validate dependencies (DD-WE-006)
	if schema.Dependencies != nil {
		if err := schema.Dependencies.ValidateDependencies(); err != nil {
			return err
		}
	}

	// Validate detectedLabels (ADR-043 v1.3)
	if schema.DetectedLabels != nil {
		if err := schema.DetectedLabels.ValidateDetectedLabels(); err != nil {
			return err
		}
	}

	// Validate execution section (Issue #89: execution.bundle is mandatory with sha256 digest)
	if schema.Execution == nil {
		return models.NewSchemaValidationError("execution", "execution section is required")
	}
	if schema.Execution.Bundle == "" {
		return models.NewSchemaValidationError("execution.bundle", "execution.bundle is required")
	}
	if err := validateBundleDigest(schema.Execution.Bundle); err != nil {
		return err
	}

	return nil
}

// ParseBundleDigest parses an OCI bundle reference and extracts its sha256 digest.
// Returns the full reference and the hex-only digest (without "sha256:" prefix).
//
// Accepted formats:
//   - registry/repo@sha256:<64 hex>           (digest-only)
//   - registry/repo:tag@sha256:<64 hex>       (tag+digest)
//
// Returns error for:
//   - Missing @ separator (tag-only references)
//   - Non-sha256 algorithm
//   - Truncated digest (not exactly 64 hex chars)
//   - Invalid hex characters
func ParseBundleDigest(bundle string) (fullRef string, digest string, err error) {
	atIdx := strings.LastIndex(bundle, "@")
	if atIdx < 0 {
		return "", "", fmt.Errorf("must contain a sha256 digest (e.g., @sha256:<64 hex chars>)")
	}

	digestPart := bundle[atIdx+1:]

	if !strings.HasPrefix(digestPart, "sha256:") {
		algo := strings.SplitN(digestPart, ":", 2)[0]
		return "", "", fmt.Errorf("digest algorithm must be sha256, got %q", algo)
	}

	hexPart := strings.TrimPrefix(digestPart, "sha256:")
	if len(hexPart) != 64 {
		return "", "", fmt.Errorf("sha256 digest must be exactly 64 hex characters, got %d", len(hexPart))
	}

	if _, decodeErr := hex.DecodeString(hexPart); decodeErr != nil {
		return "", "", fmt.Errorf("sha256 digest contains invalid hex characters: %v", decodeErr)
	}

	return bundle, hexPart, nil
}

// validateBundleDigest wraps ParseBundleDigest as a schema validation error.
func validateBundleDigest(bundle string) *models.SchemaValidationError {
	_, _, err := ParseBundleDigest(bundle)
	if err != nil {
		return models.NewSchemaValidationError("execution.bundle", err.Error())
	}
	return nil
}

// ExtractParameters extracts parameters from a WorkflowSchema as JSON
// Returns JSON bytes suitable for storing in the database
func (p *Parser) ExtractParameters(schema *models.WorkflowSchema) (json.RawMessage, error) {
	if schema == nil {
		return nil, fmt.Errorf("schema is nil")
	}

	params, err := json.Marshal(schema.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	return params, nil
}

// ExtractLabels extracts labels from a WorkflowSchema as JSON
// Returns JSON bytes with camelCase keys, suitable for storing in the labels JSONB column
// BR-WORKFLOW-004: camelCase keys match the schema field names
func (p *Parser) ExtractLabels(schema *models.WorkflowSchema) (json.RawMessage, error) {
	if schema == nil {
		return nil, fmt.Errorf("schema is nil")
	}

	// Build labels map from label fields (camelCase keys)
	// DD-WORKFLOW-001 v2.7: severity always stored as JSONB array. No wildcard.
	// DD-WORKFLOW-016: signalType is optional, environment/severity are []string for JSONB array storage
	labels := map[string]interface{}{
		"severity": []string(schema.Labels.Severity),
	}

	// DD-WORKFLOW-016: signalName is optional metadata -- only include when non-empty
	if schema.Labels.SignalName != "" {
		labels["signalName"] = schema.Labels.SignalName
	}

	// Add required labels
	if len(schema.Labels.Environment) > 0 {
		labels["environment"] = schema.Labels.Environment
	}
	if schema.Labels.Component != "" {
		labels["component"] = schema.Labels.Component
	}
	if schema.Labels.Priority != "" {
		// Normalize priority to uppercase to comply with OpenAPI enum [P0, P1, P2, P3, "*"]
		// OCI images may contain lowercase values (e.g., "p1") in workflow-schema.yaml
		// Authority: MandatoryLabels.priority enum in data-storage-v1.yaml
		labels["priority"] = strings.ToUpper(schema.Labels.Priority)
	}

	// #212: Custom labels are NOT merged here -- they go into the custom_labels column
	// via ExtractCustomLabels

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal labels: %w", err)
	}

	return labelsJSON, nil
}

// ExtractCustomLabels extracts custom labels from a WorkflowSchema
// and converts them to the DB model format (map[string][]string).
// #212: Custom labels must be stored separately in the custom_labels column,
// not merged into the mandatory labels map.
func (p *Parser) ExtractCustomLabels(schema *models.WorkflowSchema) models.CustomLabels {
	if schema == nil || len(schema.CustomLabels) == 0 {
		return models.CustomLabels{}
	}

	result := make(models.CustomLabels, len(schema.CustomLabels))
	for k, v := range schema.CustomLabels {
		result[k] = []string{v}
	}
	return result
}

// ExtractDescription extracts the structured description as JSON
// Returns JSON bytes suitable for storing in the description JSONB column
func (p *Parser) ExtractDescription(schema *models.WorkflowSchema) (json.RawMessage, error) {
	if schema == nil {
		return nil, fmt.Errorf("schema is nil")
	}

	descJSON, err := json.Marshal(schema.Metadata.Description)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal description: %w", err)
	}

	return descJSON, nil
}

// ExtractDetectedLabels converts the YAML-parsed DetectedLabelsSchema (string fields)
// to the business model DetectedLabels (bool/string fields).
// ADR-043 v1.3: Boolean fields convert "true" -> true; string fields pass through.
// Returns a zero-value DetectedLabels if the schema has no detectedLabels section.
func (p *Parser) ExtractDetectedLabels(schema *models.WorkflowSchema) (*models.DetectedLabels, error) {
	if schema == nil {
		return nil, fmt.Errorf("schema is nil")
	}

	dl := &models.DetectedLabels{}

	if schema.DetectedLabels == nil {
		return dl, nil
	}

	src := schema.DetectedLabels
	dl.GitOpsManaged = src.GitOpsManaged == "true"
	dl.GitOpsTool = src.GitOpsTool
	dl.PDBProtected = src.PDBProtected == "true"
	dl.HPAEnabled = src.HPAEnabled == "true"
	dl.Stateful = src.Stateful == "true"
	dl.HelmManaged = src.HelmManaged == "true"
	dl.NetworkIsolated = src.NetworkIsolated == "true"
	dl.ServiceMesh = src.ServiceMesh

	return dl, nil
}

// ExtractDependencies returns the workflow's declared dependencies.
// Returns nil if the schema has no dependencies section.
func (p *Parser) ExtractDependencies(schema *models.WorkflowSchema) *models.WorkflowDependencies {
	if schema == nil {
		return nil
	}
	return schema.Dependencies
}

// ExtractExecutionEngine extracts the execution engine from a WorkflowSchema
// Returns "tekton" as default if not specified
func (p *Parser) ExtractExecutionEngine(schema *models.WorkflowSchema) string {
	if schema.Execution != nil && schema.Execution.Engine != "" {
		return schema.Execution.Engine
	}
	return "tekton" // Default for V1.0
}

// ExtractExecutionBundle extracts the execution bundle from a WorkflowSchema
// Returns nil if not specified
func (p *Parser) ExtractExecutionBundle(schema *models.WorkflowSchema) *string {
	if schema.Execution != nil && schema.Execution.Bundle != "" {
		return &schema.Execution.Bundle
	}
	return nil
}
