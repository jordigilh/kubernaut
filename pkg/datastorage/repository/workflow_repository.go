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

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"

	"github.com/jordigilh/kubernaut/pkg/datastorage/embedding"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW REPOSITORY
// ========================================
// Authority: DD-STORAGE-008 v2.0 (Workflow Catalog Schema)
// Business Requirement: BR-STORAGE-012 (Workflow Semantic Search)
// Design Decision: DD-WORKFLOW-002 (MCP Workflow Catalog Architecture)
// ========================================

// WorkflowRepository handles workflow catalog operations
type WorkflowRepository struct {
	db              *sqlx.DB
	logger          *zap.Logger
	embeddingClient *embedding.Client
}

// NewWorkflowRepository creates a new workflow repository
// BR-STORAGE-014: Embedding client is optional (nil = no automatic embedding generation)
func NewWorkflowRepository(db *sqlx.DB, logger *zap.Logger, embeddingClient *embedding.Client) *WorkflowRepository {
	return &WorkflowRepository{
		db:              db,
		logger:          logger,
		embeddingClient: embeddingClient,
	}
}

// ========================================
// CREATE OPERATIONS
// ========================================

// Create inserts a new workflow into the catalog
// BR-STORAGE-012: Workflow catalog persistence
// BR-STORAGE-014: Automatic embedding generation for semantic search
func (r *WorkflowRepository) Create(ctx context.Context, workflow *models.RemediationWorkflow) error {
	// Generate embedding if not provided and embedding client is available
	if workflow.Embedding == nil && r.embeddingClient != nil {
		// Construct searchable text from workflow metadata
		searchText := r.buildSearchText(workflow)

		// Generate embedding vector
		embeddingVec, err := r.embeddingClient.Embed(ctx, searchText)
		if err != nil {
			r.logger.Warn("failed to generate embedding, proceeding without it",
				zap.String("workflow_id", workflow.WorkflowID),
				zap.String("version", workflow.Version),
				zap.Error(err))
			// Continue without embedding (graceful degradation)
		} else {
			// Convert []float32 to *pgvector.Vector
			vec := pgvector.NewVector(embeddingVec)
			workflow.Embedding = &vec
			r.logger.Debug("generated embedding for workflow",
				zap.String("workflow_id", workflow.WorkflowID),
				zap.String("version", workflow.Version),
				zap.Int("dimensions", len(embeddingVec)))
		}
	}

	query := `
		INSERT INTO remediation_workflow_catalog (
			workflow_id, version, name, description, owner, maintainer,
			content, content_hash, labels, embedding, status,
			is_latest_version, previous_version, version_notes, change_summary,
			approved_by, approved_at, expected_success_rate, expected_duration_seconds,
			created_by
		) VALUES (
			:workflow_id, :version, :name, :description, :owner, :maintainer,
			:content, :content_hash, :labels, :embedding, :status,
			:is_latest_version, :previous_version, :version_notes, :change_summary,
			:approved_by, :approved_at, :expected_success_rate, :expected_duration_seconds,
			:created_by
		)
	`

	_, err := r.db.NamedExecContext(ctx, query, workflow)
	if err != nil {
		r.logger.Error("failed to create workflow",
			zap.String("workflow_id", workflow.WorkflowID),
			zap.String("version", workflow.Version),
			zap.Error(err))
		return fmt.Errorf("failed to create workflow: %w", err)
	}

	r.logger.Info("workflow created",
		zap.String("workflow_id", workflow.WorkflowID),
		zap.String("version", workflow.Version),
		zap.Bool("has_embedding", workflow.Embedding != nil))

	return nil
}

// ========================================
// READ OPERATIONS
// ========================================

// GetByID retrieves a specific workflow version
func (r *WorkflowRepository) GetByID(ctx context.Context, workflowID, version string) (*models.RemediationWorkflow, error) {
	query := `
		SELECT * FROM remediation_workflow_catalog
		WHERE workflow_id = $1 AND version = $2
	`

	var workflow models.RemediationWorkflow
	err := r.db.GetContext(ctx, &workflow, query, workflowID, version)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workflow not found: %s@%s", workflowID, version)
		}
		r.logger.Error("failed to get workflow",
			zap.String("workflow_id", workflowID),
			zap.String("version", version),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	return &workflow, nil
}

// GetLatestVersion retrieves the latest version of a workflow
func (r *WorkflowRepository) GetLatestVersion(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error) {
	query := `
		SELECT * FROM remediation_workflow_catalog
		WHERE workflow_id = $1 AND is_latest_version = true
		LIMIT 1
	`

	var workflow models.RemediationWorkflow
	err := r.db.GetContext(ctx, &workflow, query, workflowID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workflow not found: %s", workflowID)
		}
		r.logger.Error("failed to get latest workflow version",
			zap.String("workflow_id", workflowID),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get latest workflow version: %w", err)
	}

	return &workflow, nil
}

// List retrieves workflows with optional filtering
func (r *WorkflowRepository) List(ctx context.Context, filters *models.WorkflowSearchFilters, limit, offset int) ([]models.RemediationWorkflow, int, error) {
	// Build WHERE clause
	whereClauses := []string{}
	args := []interface{}{}
	argIndex := 1

	// Default: only active workflows
	if filters == nil || len(filters.Status) == 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, "active")
		argIndex++
	} else {
		// Filter by specified statuses
		placeholders := []string{}
		for _, status := range filters.Status {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
			args = append(args, status)
			argIndex++
		}
		whereClauses = append(whereClauses, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ", ")))
	}

	// Only latest versions by default
	whereClauses = append(whereClauses, "is_latest_version = true")

	// Apply label filters (updated for DD-LLM-001 taxonomy)
	if filters != nil {
		// Mandatory: signal-type (single value)
		if filters.SignalType != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'signal-type' = $%d", argIndex))
			args = append(args, filters.SignalType)
			argIndex++
		}

		// Mandatory: severity
		if filters.Severity != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'severity' = $%d", argIndex))
			args = append(args, filters.Severity)
			argIndex++
		}

		// Optional filters
		if filters.BusinessCategory != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'business-category' = $%d", argIndex))
			args = append(args, *filters.BusinessCategory)
			argIndex++
		}

		if filters.RiskTolerance != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'risk-tolerance' = $%d", argIndex))
			args = append(args, *filters.RiskTolerance)
			argIndex++
		}

		if filters.Environment != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'environment' = $%d", argIndex))
			args = append(args, *filters.Environment)
			argIndex++
		}
	}

	whereClause := strings.Join(whereClauses, " AND ")

	// Count total results
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM remediation_workflow_catalog WHERE %s", whereClause)
	var total int
	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		r.logger.Error("failed to count workflows", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to count workflows: %w", err)
	}

	// Get paginated results
	query := fmt.Sprintf(`
		SELECT * FROM remediation_workflow_catalog
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIndex, argIndex+1)

	args = append(args, limit, offset)

	var workflows []models.RemediationWorkflow
	err = r.db.SelectContext(ctx, &workflows, query, args...)
	if err != nil {
		r.logger.Error("failed to list workflows", zap.Error(err))
		return nil, 0, fmt.Errorf("failed to list workflows: %w", err)
	}

	return workflows, total, nil
}

// ========================================
// SEMANTIC SEARCH (pgvector)
// ========================================
// BR-STORAGE-013: Semantic Search API
// DD-WORKFLOW-002: MCP Workflow Catalog Architecture

// SearchByEmbedding performs semantic search using pgvector
// TDD CYCLE 1: Mandatory Label Filtering (GREEN Phase)
// BR-STORAGE-013: Hybrid Weighted Scoring
// DD-WORKFLOW-004 v1.1: Hybrid Weighted Label Scoring
func (r *WorkflowRepository) SearchByEmbedding(ctx context.Context, request *models.WorkflowSearchRequest) (*models.WorkflowSearchResponse, error) {
	if request.Embedding == nil {
		return nil, fmt.Errorf("embedding is required for semantic search")
	}

	// Build WHERE clause for filters
	whereClauses := []string{}
	args := []interface{}{request.Embedding} // $1 is the embedding vector
	argIndex := 2

	// Default: only active workflows unless IncludeDisabled is true
	if !request.IncludeDisabled {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, "active")
		argIndex++
	}

	// Only latest versions
	whereClauses = append(whereClauses, "is_latest_version = true")

	// ========================================
	// TDD CYCLE 1: MANDATORY LABEL FILTERING (REFACTOR)
	// ========================================
	// Authority: DD-LLM-001 v1.0 (MCP Search Taxonomy)
	// DD-WORKFLOW-004 v1.1: Hybrid Weighted Label Scoring
	//
	// Mandatory labels provide strict filtering (Phase 1):
	// - signal-type: Exact match required (e.g., "OOMKilled", "MemoryLeak")
	// - severity: Exact match required (e.g., "critical", "high", "medium", "low")
	//
	// These filters reduce the candidate set before semantic ranking.
	// Only workflows matching BOTH mandatory labels proceed to scoring.

	// Apply mandatory label filters
	if request.Filters != nil {
		// Mandatory Filter 1: signal-type (exact match)
		// Example: labels->>'signal-type' = 'OOMKilled'
		if request.Filters.SignalType != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'signal-type' = $%d", argIndex))
			args = append(args, request.Filters.SignalType)
			argIndex++
		}

		// Mandatory Filter 2: severity (exact match)
		// Example: labels->>'severity' = 'critical'
		if request.Filters.Severity != "" {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'severity' = $%d", argIndex))
			args = append(args, request.Filters.Severity)
			argIndex++
		}

		// Optional label filters (for backward compatibility, will be enhanced in later cycles)
		if request.Filters.BusinessCategory != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'business-category' = $%d", argIndex))
			args = append(args, *request.Filters.BusinessCategory)
			argIndex++
		}

		if request.Filters.RiskTolerance != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'risk-tolerance' = $%d", argIndex))
			args = append(args, *request.Filters.RiskTolerance)
			argIndex++
		}

		if request.Filters.Environment != nil {
			whereClauses = append(whereClauses, fmt.Sprintf("labels->>'environment' = $%d", argIndex))
			args = append(args, *request.Filters.Environment)
			argIndex++
		}
	}

	// Apply minimum similarity threshold
	minSimilarity := 0.7 // Default: 70% similarity
	if request.MinSimilarity != nil {
		minSimilarity = *request.MinSimilarity
	}

	// pgvector cosine similarity: 1 - (embedding <=> $1)
	// Higher similarity = closer to 1.0
	whereClauses = append(whereClauses, fmt.Sprintf("(1 - (embedding <=> $1)) >= $%d", argIndex))
	args = append(args, minSimilarity)
	argIndex++

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Set default TopK
	topK := 10
	if request.TopK > 0 {
		topK = request.TopK
	}

	// ========================================
	// TDD CYCLE 2: HYBRID SCORING SQL QUERY WITH LABEL BOOSTING (GREEN)
	// ========================================
	// Semantic search query with pgvector + hybrid scoring
	// ORDER BY: final_score DESC (highest score first)
	// Score components:
	//   - base_similarity: cosine similarity from pgvector (0.0-1.0)
	//   - label_boost: boost from matching optional labels (0.0-0.46)
	//   - label_penalty: penalty from conflicting optional labels (0.0-0.20) [TDD Cycle 3]
	//   - final_score: LEAST(base_similarity + label_boost - label_penalty, 1.0) [TDD Cycle 4]
	//
	// Boost Weights (DD-WORKFLOW-004 v1.1):
	//   - resource-management: +0.10
	//   - gitops-tool: +0.10
	//   - environment: +0.08
	//   - business-category: +0.08
	//   - priority: +0.05
	//   - risk-tolerance: +0.05
	//   - Max total boost: 0.46

	// ========================================
	// TDD CYCLE 2: LABEL BOOST CALCULATION (REFACTOR)
	// ========================================
	// Build label boost calculation using extracted constants
	// Each matching optional label adds its weight to the total boost
	boostCases := []string{}

	// Optional label boosts (only if filter provided)
	if request.Filters != nil {
		// Boost 1: resource-management (+0.10)
		// Example: gitops workflow gets boost when searching for gitops
		if request.Filters.ResourceManagement != nil {
			boostCases = append(boostCases, fmt.Sprintf("WHEN labels->>'resource-management' = $%d THEN %.2f", argIndex, boostWeightResourceManagement))
			args = append(args, *request.Filters.ResourceManagement)
			argIndex++
		}

		// Boost 2: gitops-tool (+0.10)
		// Example: argocd workflow gets boost when searching for argocd
		if request.Filters.GitOpsTool != nil {
			boostCases = append(boostCases, fmt.Sprintf("WHEN labels->>'gitops-tool' = $%d THEN %.2f", argIndex, boostWeightGitOpsTool))
			args = append(args, *request.Filters.GitOpsTool)
			argIndex++
		}

		// Boost 3: environment (+0.08)
		// Example: production workflow gets boost when searching for production
		if request.Filters.Environment != nil {
			boostCases = append(boostCases, fmt.Sprintf("WHEN labels->>'environment' = $%d THEN %.2f", argIndex, boostWeightEnvironment))
			args = append(args, *request.Filters.Environment)
			argIndex++
		}

		// Boost 4: business-category (+0.08)
		// Example: payments workflow gets boost when searching for payments
		if request.Filters.BusinessCategory != nil {
			boostCases = append(boostCases, fmt.Sprintf("WHEN labels->>'business-category' = $%d THEN %.2f", argIndex, boostWeightBusinessCategory))
			args = append(args, *request.Filters.BusinessCategory)
			argIndex++
		}

		// Boost 5: priority (+0.05)
		// Example: P0 workflow gets boost when searching for P0
		if request.Filters.Priority != nil {
			boostCases = append(boostCases, fmt.Sprintf("WHEN labels->>'priority' = $%d THEN %.2f", argIndex, boostWeightPriority))
			args = append(args, *request.Filters.Priority)
			argIndex++
		}

		// Boost 6: risk-tolerance (+0.05)
		// Example: low-risk workflow gets boost when searching for low-risk
		if request.Filters.RiskTolerance != nil {
			boostCases = append(boostCases, fmt.Sprintf("WHEN labels->>'risk-tolerance' = $%d THEN %.2f", argIndex, boostWeightRiskTolerance))
			args = append(args, *request.Filters.RiskTolerance)
			argIndex++
		}
	}

	// Build label boost SQL expression
	labelBoostSQL := "0.0"
	if len(boostCases) > 0 {
		// Sum all matching label boosts
		caseExpressions := []string{}
		for _, boostCase := range boostCases {
			caseExpressions = append(caseExpressions, fmt.Sprintf("(CASE %s ELSE 0.0 END)", boostCase))
		}
		labelBoostSQL = strings.Join(caseExpressions, " + ")
	}

	// ========================================
	// TDD CYCLE 3: LABEL PENALTY CALCULATION (REFACTOR)
	// ========================================
	// Build label penalty calculation
	//
	// Penalty Logic:
	// - Applies when workflow label EXISTS but CONFLICTS with search filter
	// - Only resource-management and gitops-tool have penalties (environment, business-category, etc. don't)
	// - Rationale: Resource management is a critical decision (gitops vs manual)
	//              Other labels are descriptive, not prescriptive
	//
	// Examples:
	// - Searching for gitops, find manual workflow → -0.10 penalty
	// - Searching for argocd, find flux workflow → -0.10 penalty
	// - Searching for production, find staging workflow → 0.0 penalty (no penalty for environment)
	penaltyCases := []string{}

	if request.Filters != nil {
		// Penalty 1: resource-management conflict (-0.10)
		// Critical label: GitOps vs Manual is a fundamental workflow difference
		if request.Filters.ResourceManagement != nil {
			penaltyCases = append(penaltyCases, fmt.Sprintf(
				"WHEN labels->>'resource-management' IS NOT NULL AND labels->>'resource-management' != $%d THEN %.2f",
				argIndex-len(boostCases), // Reuse the same parameter
				penaltyWeightResourceManagement,
			))
		}

		// Penalty 2: gitops-tool conflict (-0.10)
		// Critical label: ArgoCD vs Flux are incompatible tools
		if request.Filters.GitOpsTool != nil {
			// Find the correct parameter index for gitops-tool
			gitOpsParamOffset := 0
			if request.Filters.ResourceManagement != nil {
				gitOpsParamOffset = 1
			}
			penaltyCases = append(penaltyCases, fmt.Sprintf(
				"WHEN labels->>'gitops-tool' IS NOT NULL AND labels->>'gitops-tool' != $%d THEN %.2f",
				argIndex-len(boostCases)+gitOpsParamOffset,
				penaltyWeightGitOpsTool,
			))
		}
	}

	// Build label penalty SQL expression
	labelPenaltySQL := "0.0"
	if len(penaltyCases) > 0 {
		// Sum all conflicting label penalties
		caseExpressions := []string{}
		for _, penaltyCase := range penaltyCases {
			caseExpressions = append(caseExpressions, fmt.Sprintf("(CASE %s ELSE 0.0 END)", penaltyCase))
		}
		labelPenaltySQL = strings.Join(caseExpressions, " + ")
	}

	// ========================================
	// TDD CYCLE 4: FINAL SCORE CALCULATION WITH CAPPING (GREEN)
	// ========================================
	// Final Score Formula:
	//   final_score = LEAST(base_similarity + label_boost - label_penalty, 1.0)
	//
	// Capping Rationale:
	//   - Scores must remain in [0.0, 1.0] range for consistency
	//   - High base similarity (e.g., 0.95) + high boost (e.g., 0.18) could exceed 1.0
	//   - LEAST() function caps the score at 1.0
	//
	// Example:
	//   base=0.95, boost=0.18, penalty=0.0 → uncapped=1.13 → final=1.0 (capped)
	//   base=0.80, boost=0.10, penalty=0.0 → uncapped=0.90 → final=0.90 (not capped)

	query := fmt.Sprintf(`
		SELECT
			*,
			(1 - (embedding <=> $1)) AS base_similarity,
			(%s) AS label_boost,
			(%s) AS label_penalty,
			LEAST((1 - (embedding <=> $1)) + (%s) - (%s), 1.0) AS final_score,
			(1 - (embedding <=> $1)) AS similarity_score
		FROM remediation_workflow_catalog
		%s
		ORDER BY final_score DESC
		LIMIT $%d
	`, labelBoostSQL, labelPenaltySQL, labelBoostSQL, labelPenaltySQL, whereClause, argIndex)

	args = append(args, topK)

	// Execute query
	type workflowWithScore struct {
		models.RemediationWorkflow
		BaseSimilarity  float64 `db:"base_similarity"`
		LabelBoost      float64 `db:"label_boost"`
		LabelPenalty    float64 `db:"label_penalty"`
		FinalScore      float64 `db:"final_score"`
		SimilarityScore float64 `db:"similarity_score"`
	}

	var results []workflowWithScore
	err := r.db.SelectContext(ctx, &results, query, args...)
	if err != nil {
		r.logger.Error("failed to search workflows",
			zap.String("query", request.Query),
			zap.Error(err))
		return nil, fmt.Errorf("failed to search workflows: %w", err)
	}

	// Build response
	searchResults := make([]models.WorkflowSearchResult, len(results))
	for i, result := range results {
		searchResults[i] = models.WorkflowSearchResult{
			Workflow:        result.RemediationWorkflow,
			BaseSimilarity:  result.BaseSimilarity,
			LabelBoost:      result.LabelBoost,
			LabelPenalty:    result.LabelPenalty,
			FinalScore:      result.FinalScore,
			SimilarityScore: result.SimilarityScore,
			Rank:            i + 1,
		}
	}

	response := &models.WorkflowSearchResponse{
		Workflows:    searchResults,
		TotalResults: len(searchResults),
		Query:        request.Query,
		Filters:      request.Filters,
	}

	r.logger.Info("semantic search completed",
		zap.String("query", request.Query),
		zap.Int("results", len(searchResults)))

	return response, nil
}

// ========================================
// UPDATE OPERATIONS
// ========================================

// UpdateSuccessMetrics updates the success metrics for a workflow
func (r *WorkflowRepository) UpdateSuccessMetrics(ctx context.Context, workflowID, version string, totalExecutions, successfulExecutions int) error {
	query := `
		UPDATE remediation_workflow_catalog
		SET
			total_executions = $1,
			successful_executions = $2,
			actual_success_rate = CASE
				WHEN $1 > 0 THEN $2::DECIMAL / $1::DECIMAL
				ELSE NULL
			END,
			updated_at = NOW()
		WHERE workflow_id = $3 AND version = $4
	`

	result, err := r.db.ExecContext(ctx, query, totalExecutions, successfulExecutions, workflowID, version)
	if err != nil {
		r.logger.Error("failed to update workflow success metrics",
			zap.String("workflow_id", workflowID),
			zap.String("version", version),
			zap.Error(err))
		return fmt.Errorf("failed to update workflow success metrics: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("workflow not found: %s@%s", workflowID, version)
	}

	r.logger.Info("workflow success metrics updated",
		zap.String("workflow_id", workflowID),
		zap.String("version", version),
		zap.Int("total_executions", totalExecutions),
		zap.Int("successful_executions", successfulExecutions))

	return nil
}

// UpdateStatus updates the workflow status
func (r *WorkflowRepository) UpdateStatus(ctx context.Context, workflowID, version, status, reason, updatedBy string) error {
	query := `
		UPDATE remediation_workflow_catalog
		SET
			status = $1,
			disabled_at = CASE WHEN $1 = 'disabled' THEN NOW() ELSE disabled_at END,
			disabled_by = CASE WHEN $1 = 'disabled' THEN $2 ELSE disabled_by END,
			disabled_reason = CASE WHEN $1 = 'disabled' THEN $3 ELSE disabled_reason END,
			updated_at = NOW(),
			updated_by = $2
		WHERE workflow_id = $4 AND version = $5
	`

	result, err := r.db.ExecContext(ctx, query, status, updatedBy, reason, workflowID, version)
	if err != nil {
		r.logger.Error("failed to update workflow status",
			zap.String("workflow_id", workflowID),
			zap.String("version", version),
			zap.String("status", status),
			zap.Error(err))
		return fmt.Errorf("failed to update workflow status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("workflow not found: %s@%s", workflowID, version)
	}

	r.logger.Info("workflow status updated",
		zap.String("workflow_id", workflowID),
		zap.String("version", version),
		zap.String("status", status))

	return nil
}

// ========================================
// HELPER FUNCTIONS
// ========================================

// buildSearchText constructs searchable text from workflow metadata for embedding generation
// BR-STORAGE-014: Semantic search text construction
//
// Strategy: Combine name, description, and content for comprehensive semantic representation
// Weight: Name (high), Description (medium), Content preview (low)
func (r *WorkflowRepository) buildSearchText(workflow *models.RemediationWorkflow) string {
	var parts []string

	// Name (highest weight - include 3 times for emphasis)
	if workflow.Name != "" {
		parts = append(parts, workflow.Name, workflow.Name, workflow.Name)
	}

	// Description (medium weight - include 2 times)
	if workflow.Description != "" {
		parts = append(parts, workflow.Description, workflow.Description)
	}

	// Content preview (low weight - first 500 chars only)
	if workflow.Content != "" {
		contentPreview := workflow.Content
		if len(contentPreview) > 500 {
			contentPreview = contentPreview[:500]
		}
		parts = append(parts, contentPreview)
	}

	return strings.Join(parts, " ")
}

// toJSONArray converts a string slice to a JSON array string
func toJSONArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}

	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf(`"%s"`, item)
	}

	return fmt.Sprintf("[%s]", strings.Join(quoted, ", "))
}

// ========================================
// HYBRID SCORING CONSTANTS
// ========================================
// Authority: DD-WORKFLOW-004 v1.1 (Hybrid Weighted Label Scoring)
// TDD Cycle 2: Optional Label Boost Weights
// TDD Cycle 3: Optional Label Penalty Weights

const (
	// Boost weights for optional labels (when they match)
	boostWeightResourceManagement = 0.10 // GitOps vs Manual vs Automated
	boostWeightGitOpsTool         = 0.10 // ArgoCD vs Flux vs None
	boostWeightEnvironment        = 0.08 // Production vs Staging vs Development
	boostWeightBusinessCategory   = 0.08 // Payments, Auth, Data-Processing, etc.
	boostWeightPriority           = 0.05 // P0, P1, P2, P3, P4
	boostWeightRiskTolerance      = 0.05 // Low, Medium, High

	// Penalty weights for optional labels (when they conflict)
	// Only resource-management and gitops-tool have penalties
	// Other labels (environment, business-category, priority, risk-tolerance) don't penalize
	penaltyWeightResourceManagement = 0.10 // Penalty for conflicting resource management
	penaltyWeightGitOpsTool         = 0.10 // Penalty for conflicting GitOps tool

	// Maximum possible boost (sum of all boost weights)
	maxLabelBoost = 0.46

	// Maximum possible penalty (sum of penalty weights)
	maxLabelPenalty = 0.20
)

