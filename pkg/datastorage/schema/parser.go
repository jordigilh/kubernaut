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
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW SCHEMA PARSER (ADR-043)
// ========================================
// Authority: ADR-043 (Workflow Schema Definition Standard)
// Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
// Design Decision: DD-WORKFLOW-005 (Automated Schema Extraction)
// ========================================
//
// This package provides parsing and validation for workflow-schema.yaml
// files as defined in ADR-043.
//
// V1.0: Operators manually extract and POST the schema content
// V1.1: WorkflowRegistration CRD controller extracts automatically
//
// ========================================

// Parser handles parsing and validation of workflow-schema.yaml content
type Parser struct{}

// NewParser creates a new workflow schema parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses workflow-schema.yaml content and returns a WorkflowSchema
// Returns error if content is invalid YAML or doesn't match ADR-043 structure
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
// - Required fields (apiVersion, kind, metadata, labels, parameters)
// - Mandatory labels (signal_type, severity, risk_tolerance)
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

// Validate validates a WorkflowSchema against ADR-043 requirements
// Level 2 validation: fields + mandatory labels (per user decision)
func (p *Parser) Validate(schema *models.WorkflowSchema) error {
	// Validate APIVersion
	if schema.APIVersion == "" {
		return models.NewSchemaValidationError("apiVersion", "apiVersion is required")
	}

	// Validate Kind
	if schema.Kind == "" {
		return models.NewSchemaValidationError("kind", "kind is required")
	}
	if schema.Kind != "WorkflowSchema" {
		return models.NewSchemaValidationError("kind", "kind must be 'WorkflowSchema'")
	}

	// Validate Metadata
	if schema.Metadata.WorkflowID == "" {
		return models.NewSchemaValidationError("metadata.workflow_id", "workflow_id is required")
	}
	if schema.Metadata.Version == "" {
		return models.NewSchemaValidationError("metadata.version", "version is required")
	}
	if schema.Metadata.Description == "" {
		return models.NewSchemaValidationError("metadata.description", "description is required")
	}

	// Validate Labels (DD-WORKFLOW-001 v1.3: 6 mandatory labels)
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
// Returns JSON bytes suitable for storing in the database
func (p *Parser) ExtractLabels(schema *models.WorkflowSchema) (json.RawMessage, error) {
	if schema == nil {
		return nil, fmt.Errorf("schema is nil")
	}

	// Build labels map from struct fields
	labels := map[string]string{
		"signal_type":    schema.Labels.SignalType,
		"severity":       schema.Labels.Severity,
		"risk_tolerance": schema.Labels.RiskTolerance,
	}

	// DD-WORKFLOW-016/017: Include action_type for discovery indexing
	if schema.Labels.ActionType != "" {
		labels["action_type"] = schema.Labels.ActionType
	}

	// Add optional labels if present
	if schema.Labels.BusinessCategory != "" {
		labels["business_category"] = schema.Labels.BusinessCategory
	}
	if schema.Labels.Environment != "" {
		labels["environment"] = schema.Labels.Environment
	}
	if schema.Labels.Priority != "" {
		labels["priority"] = schema.Labels.Priority
	}
	if schema.Labels.Component != "" {
		labels["component"] = schema.Labels.Component
	}

	// Add custom labels
	for k, v := range schema.Labels.CustomLabels {
		labels[k] = v
	}

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal labels: %w", err)
	}

	return labelsJSON, nil
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
// Returns nil if not specified (V1.0 behavior)
func (p *Parser) ExtractExecutionBundle(schema *models.WorkflowSchema) *string {
	if schema.Execution != nil && schema.Execution.Bundle != "" {
		return &schema.Execution.Bundle
	}
	return nil
}
