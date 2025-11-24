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

	"github.com/pgvector/pgvector-go"
)

// ========================================
// REMEDIATION WORKFLOW CATALOG MODELS
// ========================================
// Authority: DD-STORAGE-008 v2.0 (Workflow Catalog Schema)
// Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
// Design Decision: DD-NAMING-001 (Remediation Workflow Terminology)
// ========================================

// RemediationWorkflow represents a workflow in the catalog
// Maps to remediation_workflow_catalog table (migration 015)
type RemediationWorkflow struct {
	// ========================================
	// IDENTITY (Composite Primary Key)
	// ========================================
	WorkflowID string `json:"workflow_id" db:"workflow_id" validate:"required,max=255"`
	Version    string `json:"version" db:"version" validate:"required,max=50"`

	// ========================================
	// METADATA
	// ========================================
	Name        string  `json:"name" db:"name" validate:"required,max=255"`
	Description string  `json:"description" db:"description" validate:"required"`
	Owner       *string `json:"owner,omitempty" db:"owner" validate:"omitempty,max=255"`
	Maintainer  *string `json:"maintainer,omitempty" db:"maintainer" validate:"omitempty,max=255,email"`

	// ========================================
	// CONTENT
	// ========================================
	Content     string `json:"content" db:"content" validate:"required"`
	ContentHash string `json:"content_hash" db:"content_hash" validate:"required,len=64"`

	// ========================================
	// LABELS (JSONB for flexible filtering)
	// ========================================
	// DD-CONTEXT-005: Filter Before LLM pattern
	// Examples:
	// {
	//   "signal_types": ["MemoryLeak", "OOMKilled"],
	//   "business_category": "payments",
	//   "risk_tolerance": "low",
	//   "environment": "production"
	// }
	Labels json.RawMessage `json:"labels" db:"labels" validate:"required"`

	// ========================================
	// SEMANTIC SEARCH (pgvector)
	// ========================================
	// BR-STORAGE-012: Vector embeddings for semantic search
	// Model: sentence-transformers/all-MiniLM-L6-v2 (384 dimensions)
	Embedding *pgvector.Vector `json:"embedding,omitempty" db:"embedding"`

	// ========================================
	// LIFECYCLE MANAGEMENT
	// ========================================
	Status         string     `json:"status" db:"status" validate:"required,oneof=active disabled deprecated archived"`
	DisabledAt     *time.Time `json:"disabled_at,omitempty" db:"disabled_at"`
	DisabledBy     *string    `json:"disabled_by,omitempty" db:"disabled_by" validate:"omitempty,max=255"`
	DisabledReason *string    `json:"disabled_reason,omitempty" db:"disabled_reason"`

	// ========================================
	// VERSION MANAGEMENT
	// ========================================
	IsLatestVersion   bool    `json:"is_latest_version" db:"is_latest_version"`
	PreviousVersion   *string `json:"previous_version,omitempty" db:"previous_version" validate:"omitempty,max=50"`
	DeprecationNotice *string `json:"deprecation_notice,omitempty" db:"deprecation_notice"`

	// ========================================
	// VERSION CHANGE METADATA
	// ========================================
	VersionNotes  *string    `json:"version_notes,omitempty" db:"version_notes"`
	ChangeSummary *string    `json:"change_summary,omitempty" db:"change_summary"`
	ApprovedBy    *string    `json:"approved_by,omitempty" db:"approved_by" validate:"omitempty,max=255"`
	ApprovedAt    *time.Time `json:"approved_at,omitempty" db:"approved_at"`

	// ========================================
	// SUCCESS METRICS (ADR-033)
	// ========================================
	ExpectedSuccessRate    *float64 `json:"expected_success_rate,omitempty" db:"expected_success_rate" validate:"omitempty,min=0,max=1"`
	ExpectedDurationSeconds *int     `json:"expected_duration_seconds,omitempty" db:"expected_duration_seconds" validate:"omitempty,min=0"`
	ActualSuccessRate      *float64 `json:"actual_success_rate,omitempty" db:"actual_success_rate" validate:"omitempty,min=0,max=1"`
	TotalExecutions        int      `json:"total_executions" db:"total_executions" validate:"min=0"`
	SuccessfulExecutions   int      `json:"successful_executions" db:"successful_executions" validate:"min=0"`

	// ========================================
	// AUDIT TRAIL
	// ========================================
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	CreatedBy *string    `json:"created_by,omitempty" db:"created_by" validate:"omitempty,max=255"`
	UpdatedBy *string    `json:"updated_by,omitempty" db:"updated_by" validate:"omitempty,max=255"`
}

// ========================================
// WORKFLOW SEARCH REQUEST
// ========================================
// Business Requirement: BR-STORAGE-013 (Semantic Search API)
// Design Decision: DD-WORKFLOW-002 (MCP Workflow Catalog Architecture)

// WorkflowSearchRequest represents a semantic search request
type WorkflowSearchRequest struct {
	// Query is the natural language search query
	// Example: "Memory leak from unclosed database connections causing OOMKilled"
	Query string `json:"query" validate:"required,min=1,max=1000"`

	// Embedding is the vector representation of the query (384 dimensions)
	// Generated by embedding service from the query text
	Embedding *pgvector.Vector `json:"embedding,omitempty"`

	// Filters for label-based filtering (DD-CONTEXT-005)
	Filters *WorkflowSearchFilters `json:"filters,omitempty"`

	// TopK is the number of results to return (default: 10)
	TopK int `json:"top_k" validate:"omitempty,min=1,max=100"`

	// MinSimilarity is the minimum cosine similarity threshold (0.0-1.0)
	// Default: 0.7 (70% similarity)
	MinSimilarity *float64 `json:"min_similarity,omitempty" validate:"omitempty,min=0,max=1"`

	// IncludeDisabled includes disabled workflows in results (default: false)
	IncludeDisabled bool `json:"include_disabled,omitempty"`
}

// WorkflowSearchFilters represents label-based filters
// DD-CONTEXT-005: Filter Before LLM pattern
// DD-LLM-001: MCP Workflow Search Parameter Taxonomy
// DD-WORKFLOW-004: Hybrid Weighted Label Scoring
type WorkflowSearchFilters struct {
	// ========================================
	// MANDATORY LABELS (Strict Filtering)
	// ========================================
	// Authority: DD-LLM-001 v1.0 (MCP Search Taxonomy)

	// SignalType filters by single alert/signal type (MANDATORY)
	// Example: "OOMKilled", "MemoryLeak", "DatabaseConnectionLeak"
	// Changed from SignalTypes []string to SignalType string per DD-LLM-001
	SignalType string `json:"signal-type" validate:"required"`

	// Severity filters by severity level (MANDATORY)
	// Values: "critical", "high", "medium", "low"
	// Authority: DD-LLM-001 (Canonical Severity Values)
	Severity string `json:"severity" validate:"required,oneof=critical high medium low"`

	// ========================================
	// OPTIONAL LABELS (Weighted Scoring)
	// ========================================
	// Authority: DD-WORKFLOW-004 v1.1 (Hybrid Weighted Scoring)

	// ResourceManagement filters by resource management approach (OPTIONAL)
	// Values: "gitops", "manual", "automated"
	// Boost: +0.10 for match, Penalty: -0.10 for conflict
	ResourceManagement *string `json:"resource-management,omitempty" validate:"omitempty,oneof=gitops manual automated"`

	// GitOpsTool filters by GitOps tool (OPTIONAL)
	// Values: "argocd", "flux", "none"
	// Boost: +0.10 for match, Penalty: -0.10 for conflict
	GitOpsTool *string `json:"gitops-tool,omitempty" validate:"omitempty,oneof=argocd flux none"`

	// Environment filters by deployment environment (OPTIONAL)
	// Example: "production", "staging", "development"
	// Boost: +0.08 for match
	Environment *string `json:"environment,omitempty"`

	// BusinessCategory filters by business domain (OPTIONAL)
	// Example: "payments", "authentication", "data-processing"
	// Boost: +0.08 for match
	BusinessCategory *string `json:"business-category,omitempty"`

	// Priority filters by priority level (OPTIONAL)
	// Values: "p0", "p1", "p2", "p3", "p4"
	// Boost: +0.05 for match
	Priority *string `json:"priority,omitempty" validate:"omitempty,oneof=p0 p1 p2 p3 p4"`

	// RiskTolerance filters by risk level (OPTIONAL)
	// Values: "low", "medium", "high"
	// Boost: +0.05 for match
	RiskTolerance *string `json:"risk-tolerance,omitempty" validate:"omitempty,oneof=low medium high"`

	// Status filters by workflow lifecycle status
	// Default: ["active"] if not specified
	Status []string `json:"status,omitempty"`
}

// ========================================
// WORKFLOW SEARCH RESPONSE
// ========================================

// WorkflowSearchResponse represents semantic search results
type WorkflowSearchResponse struct {
	// Workflows is the list of matching workflows, ranked by similarity
	Workflows []WorkflowSearchResult `json:"workflows"`

	// TotalResults is the total number of matching workflows
	TotalResults int `json:"total_results"`

	// Query is the original search query
	Query string `json:"query"`

	// Filters are the applied filters
	Filters *WorkflowSearchFilters `json:"filters,omitempty"`
}

// WorkflowSearchResult represents a single search result with similarity score
// DD-WORKFLOW-004: Hybrid Weighted Label Scoring
type WorkflowSearchResult struct {
	// Workflow is the matching workflow
	Workflow RemediationWorkflow `json:"workflow"`

	// ========================================
	// HYBRID SCORING COMPONENTS
	// ========================================
	// Authority: DD-WORKFLOW-004 v1.1 (Hybrid Weighted Scoring)

	// BaseSimilarity is the cosine similarity from pgvector (0.0-1.0)
	// This is the semantic similarity between query and workflow embeddings
	BaseSimilarity float64 `json:"base_similarity" validate:"min=0,max=1"`

	// LabelBoost is the boost from matching optional labels (0.0-0.46)
	// Sum of all matching optional label weights
	// Max: 0.10 (resource-management) + 0.10 (gitops-tool) + 0.08 (environment) + 0.08 (business-category) + 0.05 (priority) + 0.05 (risk-tolerance) = 0.46
	LabelBoost float64 `json:"label_boost" validate:"min=0,max=0.46"`

	// LabelPenalty is the penalty from conflicting optional labels (0.0-0.20)
	// Sum of all conflicting optional label weights
	// Max: 0.10 (resource-management) + 0.10 (gitops-tool) = 0.20
	LabelPenalty float64 `json:"label_penalty" validate:"min=0,max=0.20"`

	// FinalScore is the final weighted score (0.0-1.0)
	// Formula: LEAST(base_similarity + label_boost - label_penalty, 1.0)
	// Capped at 1.0 to maintain score range
	FinalScore float64 `json:"final_score" validate:"min=0,max=1"`

	// SimilarityScore is the cosine similarity (0.0-1.0)
	// DEPRECATED: Use FinalScore instead (kept for backward compatibility)
	// Higher is better (1.0 = identical)
	SimilarityScore float64 `json:"similarity_score" validate:"min=0,max=1"`

	// Rank is the position in the result set (1-based)
	Rank int `json:"rank" validate:"min=1"`
}

// ========================================
// HELPER METHODS
// ========================================

// IsActive returns true if the workflow is active
func (w *RemediationWorkflow) IsActive() bool {
	return w.Status == "active"
}

// IsDisabled returns true if the workflow is disabled
func (w *RemediationWorkflow) IsDisabled() bool {
	return w.Status == "disabled"
}

// IsDeprecated returns true if the workflow is deprecated
func (w *RemediationWorkflow) IsDeprecated() bool {
	return w.Status == "deprecated"
}

// IsArchived returns true if the workflow is archived
func (w *RemediationWorkflow) IsArchived() bool {
	return w.Status == "archived"
}

// GetLabelsMap returns labels as a map for easier access
func (w *RemediationWorkflow) GetLabelsMap() (map[string]interface{}, error) {
	var labels map[string]interface{}
	if err := json.Unmarshal(w.Labels, &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

// SetLabelsFromMap sets labels from a map
func (w *RemediationWorkflow) SetLabelsFromMap(labels map[string]interface{}) error {
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return err
	}
	w.Labels = labelsJSON
	return nil
}

// ========================================
// WORKFLOW LIST RESPONSE
// ========================================

// WorkflowListResponse represents paginated workflow list results
type WorkflowListResponse struct {
	Workflows []*RemediationWorkflow `json:"workflows"`
	Limit     int                    `json:"limit"`
	Offset    int                    `json:"offset"`
	Total     int                    `json:"total"`
}

