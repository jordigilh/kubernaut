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

	// DD-WORKFLOW-016: signalType is optional metadata -- only include when non-empty
	if schema.Labels.SignalType != "" {
		labels["signalType"] = schema.Labels.SignalType
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

	// Add custom labels
	for k, v := range schema.CustomLabels {
		labels[k] = v
	}

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal labels: %w", err)
	}

	return labelsJSON, nil
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
