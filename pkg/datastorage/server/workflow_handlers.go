package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// ========================================
// WORKFLOW CATALOG HANDLERS
// ========================================
// Business Requirements:
// - BR-STORAGE-013: Semantic search for remediation workflows
// - BR-STORAGE-014: Workflow catalog management
//
// API Endpoints:
// - POST /api/v1/workflows/search - Semantic search for workflows
// - GET /api/v1/workflows - List workflows with filters
// - GET /api/v1/workflows/{id}/{version} - Get specific workflow version
// - GET /api/v1/workflows/{id}/latest - Get latest workflow version

// HandleWorkflowSearch handles POST /api/v1/workflows/search
// BR-STORAGE-013: Semantic search for remediation workflows
func (h *Handler) HandleWorkflowSearch(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var searchReq models.WorkflowSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&searchReq); err != nil {
		h.logger.Error("Failed to decode workflow search request",
			"error", err,
		)
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			fmt.Sprintf("Invalid request body: %v", err),
		)
		return
	}

	// Validate request
	if err := h.validateWorkflowSearchRequest(&searchReq); err != nil {
		h.logger.Error("Invalid workflow search request",
			"error", err,
			"query", searchReq.Query,
		)
		h.writeRFC7807Error(w, http.StatusBadRequest,
			"https://kubernaut.dev/problems/bad-request",
			"Bad Request",
			err.Error(),
		)
		return
	}

	// Generate embedding from query text if not provided
	if searchReq.Embedding == nil {
		if h.embeddingService == nil {
			h.logger.Error("Embedding service not configured",
				"query", searchReq.Query,
			)
			h.writeRFC7807Error(w, http.StatusInternalServerError,
				"https://kubernaut.dev/problems/internal-error",
				"Internal Server Error",
				"Embedding service not configured",
			)
			return
		}

		embedding, err := h.embeddingService.GenerateEmbedding(r.Context(), searchReq.Query)
		if err != nil {
			h.logger.Error("Failed to generate embedding",
				"error", err,
				"query", searchReq.Query,
			)
			h.writeRFC7807Error(w, http.StatusInternalServerError,
				"https://kubernaut.dev/problems/internal-error",
				"Internal Server Error",
				fmt.Sprintf("Failed to generate embedding: %v", err),
			)
			return
		}
		searchReq.Embedding = embedding
	}

	// Execute semantic search
	response, err := h.workflowRepo.SearchByEmbedding(r.Context(), &searchReq)
	if err != nil {
		h.logger.Error("Failed to search workflows",
			"error", err,
			"query", searchReq.Query,
			"top_k", searchReq.TopK,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.dev/problems/internal-error",
			"Internal Server Error",
			"Failed to search workflows",
		)
		return
	}

	// Log success
	h.logger.Info("Workflow search completed",
		"query", searchReq.Query,
		"results_count", len(response.Workflows),
		"top_k", searchReq.TopK,
	)

	// Return results
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error(err, "Failed to encode workflow search response")
	}
}

// HandleListWorkflows handles GET /api/v1/workflows
// BR-STORAGE-014: Workflow catalog management
func (h *Handler) HandleListWorkflows(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	filters := &models.WorkflowSearchFilters{}

	// Status filter
	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = []string{status}
	}

	// Business category filter
	if category := r.URL.Query().Get("business_category"); category != "" {
		filters.BusinessCategory = &category
	}

	// Environment filter
	if env := r.URL.Query().Get("environment"); env != "" {
		filters.Environment = &env
	}

	// Risk tolerance filter
	if risk := r.URL.Query().Get("risk_tolerance"); risk != "" {
		filters.RiskTolerance = &risk
	}

	// Pagination
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 100 {
		limit = 100 // Max limit
	}

	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	// Execute list query
	workflows, total, err := h.workflowRepo.List(r.Context(), filters, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list workflows",
			"error", err,
			"filters", filters,
			"limit", limit,
			"offset", offset,
		)
		h.writeRFC7807Error(w, http.StatusInternalServerError,
			"https://kubernaut.dev/problems/internal-error",
			"Internal Server Error",
			"Failed to list workflows",
		)
		return
	}

	// Log success
	h.logger.Info("Workflows listed",
		"count", len(workflows),
		"filters", filters,
		"limit", limit,
		"offset", offset,
	)

	// Convert to pointer slice for response
	workflowPtrs := make([]*models.RemediationWorkflow, len(workflows))
	for i := range workflows {
		workflowPtrs[i] = &workflows[i]
	}

	// Return results
	response := models.WorkflowListResponse{
		Workflows: workflowPtrs,
		Limit:     limit,
		Offset:    offset,
		Total:     total,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error(err, "Failed to encode workflow list response")
	}
}

// validateWorkflowSearchRequest validates the workflow search request
func (h *Handler) validateWorkflowSearchRequest(req *models.WorkflowSearchRequest) error {
	if req.Query == "" {
		return fmt.Errorf("query is required")
	}

	if req.TopK <= 0 {
		req.TopK = 10 // Default to 10 results
	}
	if req.TopK > 100 {
		req.TopK = 100 // Max 100 results
	}

	if req.MinSimilarity != nil {
		if *req.MinSimilarity < 0 || *req.MinSimilarity > 1 {
			return fmt.Errorf("min_similarity must be between 0 and 1")
		}
	}

	return nil
}
