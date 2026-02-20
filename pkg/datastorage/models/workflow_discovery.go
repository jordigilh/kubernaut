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
	"time"
)

// ========================================
// THREE-STEP WORKFLOW DISCOVERY MODELS
// ========================================
// Authority: DD-WORKFLOW-016 (Action-Type Workflow Catalog Indexing)
// Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration)
// Business Requirement: BR-HAPI-017-001 (Three-Step Tool Implementation)
// ========================================

// ActionTypeTaxonomy represents an entry in the action_type_taxonomy table
// Migration 025: action_type_taxonomy table
type ActionTypeTaxonomy struct {
	ActionType  string          `json:"actionType" db:"action_type"`
	Description json.RawMessage `json:"description" db:"description"`
	CreatedAt   time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time       `json:"updatedAt" db:"updated_at"`
}

// ActionTypeDescription represents the JSONB description structure for an action type
// Stored in action_type_taxonomy.description column
// BR-WORKFLOW-004: camelCase JSON keys (migration 026 updates existing data)
type ActionTypeDescription struct {
	What          string `json:"what"`
	WhenToUse     string `json:"whenToUse"`
	WhenNotToUse  string `json:"whenNotToUse,omitempty"`
	Preconditions string `json:"preconditions,omitempty"`
}

// ActionTypeEntry represents a single action type in the discovery response (Step 1)
// Includes the action type metadata and count of matching workflows
type ActionTypeEntry struct {
	ActionType    string                `json:"actionType"`
	Description   ActionTypeDescription `json:"description"`
	WorkflowCount int                   `json:"workflowCount"`
}

// WorkflowDiscoveryEntry represents a workflow summary in the discovery response (Step 2)
// Contains enough information for the LLM to compare workflows without the full parameter schema.
//
// DD-HAPI-017 v1.1: ActualSuccessRate and TotalExecutions REMOVED from this LLM-facing struct.
// Global aggregate metrics are misleading for per-incident workflow selection — the conditions
// under which they were collected (different signals, targets, environments) are not applicable
// to the current case. The LLM should rely on contextual remediation history via spec-hash
// matching (DD-HAPI-016) and the StructuredDescription for workflow comparison.
// These fields remain on the full RemediationWorkflow model for operator dashboards.
type WorkflowDiscoveryEntry struct {
	WorkflowID      string                `json:"workflowId"`
	WorkflowName    string                `json:"workflowName"`
	Name            string                `json:"name"`
	Description     StructuredDescription `json:"description"`
	Version         string                `json:"version"`
	SchemaImage     string                `json:"schemaImage,omitempty"`
	ExecutionBundle string                `json:"executionBundle,omitempty"`
	ExecutionEngine string                `json:"executionEngine,omitempty"`
}

// PaginationMetadata represents pagination information for discovery responses
// DD-WORKFLOW-016: Default page size 10, offset-based pagination
type PaginationMetadata struct {
	TotalCount int  `json:"totalCount"`
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
	HasMore    bool `json:"hasMore"`
}

// ActionTypeListResponse is the response for Step 1: list available action types
// GET /api/v1/workflows/actions
type ActionTypeListResponse struct {
	ActionTypes []ActionTypeEntry  `json:"actionTypes"`
	Pagination  PaginationMetadata `json:"pagination"`
}

// WorkflowDiscoveryResponse is the response for Step 2: list workflows for action type
// GET /api/v1/workflows/actions/{action_type}
type WorkflowDiscoveryResponse struct {
	ActionType string                   `json:"actionType"`
	Workflows  []WorkflowDiscoveryEntry `json:"workflows"`
	Pagination PaginationMetadata       `json:"pagination"`
}

// WorkflowDiscoveryFilters represents the signal context filters used across
// all three steps of the discovery protocol.
// These filters are passed as query parameters on GET endpoints.
// Authority: DD-WORKFLOW-016, DD-HAPI-017
type WorkflowDiscoveryFilters struct {
	// Mandatory context filters (required for all discovery calls)
	Severity    string `json:"severity"`
	Component   string `json:"component"`
	Environment string `json:"environment"`
	Priority    string `json:"priority"`

	// Optional context filters
	CustomLabels   map[string][]string `json:"customLabels,omitempty"`
	DetectedLabels *DetectedLabels     `json:"detectedLabels,omitempty"`

	// Audit correlation
	RemediationID string `json:"remediationId,omitempty"`
}

// HasContextFilters returns true if any of the mandatory context filters are set.
// Used to determine whether the security gate should be applied for GET /workflows/{id}.
func (f *WorkflowDiscoveryFilters) HasContextFilters() bool {
	return f.Severity != "" || f.Component != "" || f.Environment != "" || f.Priority != ""
}

// DefaultPaginationLimit is the default page size for discovery endpoints
const DefaultPaginationLimit = 10

// MaxPaginationLimit is the maximum page size for discovery endpoints
const MaxPaginationLimit = 100
