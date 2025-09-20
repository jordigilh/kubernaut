package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/infrastructure/metrics"
	"github.com/jordigilh/kubernaut/pkg/platform/k8s"
	"github.com/jordigilh/kubernaut/pkg/platform/monitoring"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/sirupsen/logrus"
)

// AIServiceIntegrator provides intelligent AI service detection and integration
// Business Requirement: BR-AI-001 - Automatic service detection and configuration
type AIServiceIntegrator struct {
	config        *config.Config
	llmClient     llm.Client
	holmesClient  holmesgpt.Client
	vectorDB      vector.VectorDatabase
	metricsClient *metrics.Client
	log           *logrus.Logger
}

// AIServiceStatus represents the status of AI services
type AIServiceStatus struct {
	LLMAvailable       bool      `json:"llm_available"`
	HolmesGPTAvailable bool      `json:"holmesgpt_available"`
	VectorDBEnabled    bool      `json:"vectordb_enabled"`
	MetricsEnabled     bool      `json:"metrics_enabled"`
	LLMServiceEndpoint string    `json:"llm_service_endpoint"`
	HolmesGPTEndpoint  string    `json:"holmesgpt_endpoint"`
	LastHealthCheck    time.Time `json:"last_health_check"`
	HealthCheckError   string    `json:"health_check_error,omitempty"`
}

// NewAIServiceIntegrator creates a new AI service integrator
func NewAIServiceIntegrator(
	cfg *config.Config,
	llmClient llm.Client,
	holmesClient holmesgpt.Client,
	vectorDB vector.VectorDatabase,
	metricsClient *metrics.Client,
	log *logrus.Logger,
) *AIServiceIntegrator {
	return &AIServiceIntegrator{
		config:        cfg,
		llmClient:     llmClient,
		holmesClient:  holmesClient,
		vectorDB:      vectorDB,
		metricsClient: metricsClient,
		log:           log,
	}
}

// DetectAndConfigure automatically detects available AI services and configures the appropriate implementations
func (asi *AIServiceIntegrator) DetectAndConfigure(ctx context.Context) (*AIServiceStatus, error) {
	// Add context-aware logging for traceability and monitoring
	logger := asi.log.WithFields(logrus.Fields{
		"trace_id": ctx.Value("trace_id"),
	})
	logger.Info("Detecting AI service availability for intelligent workflow integration")

	// Check for context cancellation during potentially long-running service detection
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	llmConfig := asi.config.GetLLMConfig()
	holmesConfig := asi.config.GetHolmesGPTConfig()

	status := &AIServiceStatus{
		LastHealthCheck:    time.Now(),
		LLMServiceEndpoint: llmConfig.Endpoint,
		HolmesGPTEndpoint:  holmesConfig.Endpoint,
	}

	// Check LLM service health
	if asi.llmClient != nil {
		status.LLMAvailable = asi.llmClient.IsHealthy()
		if !status.LLMAvailable {
			status.HealthCheckError = fmt.Sprintf("LLM service at %s is not healthy", llmConfig.Endpoint)
		}
	}

	// Check HolmesGPT service health
	if asi.holmesClient != nil && asi.config.IsHolmesGPTEnabled() {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := asi.holmesClient.GetHealth(ctx); err != nil {
			status.HolmesGPTAvailable = false
			if status.HealthCheckError != "" {
				status.HealthCheckError += "; "
			}
			status.HealthCheckError += fmt.Sprintf("HolmesGPT service at %s is not healthy: %v", holmesConfig.Endpoint, err)
		} else {
			status.HolmesGPTAvailable = true
		}
	}

	// Check vector database availability
	if asi.vectorDB != nil {
		status.VectorDBEnabled = true
	}

	// Check metrics collection availability
	if asi.metricsClient != nil {
		status.MetricsEnabled = true
	}

	asi.log.WithFields(logrus.Fields{
		"llm_available":       status.LLMAvailable,
		"holmesgpt_available": status.HolmesGPTAvailable,
		"vectordb_enabled":    status.VectorDBEnabled,
		"metrics_enabled":     status.MetricsEnabled,
		"llm_endpoint":        status.LLMServiceEndpoint,
		"holmesgpt_endpoint":  status.HolmesGPTEndpoint,
	}).Info("AI service detection completed")

	return status, nil
}

// CreateConfiguredAIMetricsCollector creates an AI metrics collector based on service availability
func (asi *AIServiceIntegrator) CreateConfiguredAIMetricsCollector(ctx context.Context) AIMetricsCollector {
	// Business Requirement: Use real implementation when services are available, fail-fast otherwise

	status, err := asi.DetectAndConfigure(ctx)
	if err != nil {
		asi.log.WithError(err).Warn("Failed to detect AI services, using fail-fast implementation")
		return &FailFastAIMetricsCollector{}
	}

	// Use real implementation if LLM is available
	if status.LLMAvailable && asi.llmClient != nil {
		asi.log.Info("Creating real AI metrics collector with LLM integration")
		return NewRealAIMetricsCollector(
			asi.llmClient,
			asi.vectorDB,
			asi.metricsClient,
			asi.log,
		)
	}

	// Use fail-fast implementation with informative logging
	asi.log.WithField("endpoint", asi.config.SLM.Endpoint).Warn("LLM service unavailable, using fail-fast AI metrics collector")
	return &FailFastAIMetricsCollector{}
}

// CreateConfiguredLearningEnhancedPromptBuilder creates a prompt builder based on service availability
func (asi *AIServiceIntegrator) CreateConfiguredLearningEnhancedPromptBuilder(ctx context.Context, executionRepo ExecutionRepository) LearningEnhancedPromptBuilder {
	// Business Requirement: Use real implementation when services are available, fail-fast otherwise

	status, err := asi.DetectAndConfigure(ctx)
	if err != nil {
		asi.log.WithError(err).Warn("Failed to detect AI services, using fail-fast implementation")
		return &FailFastLearningEnhancedPromptBuilder{}
	}

	// Use real implementation if LLM is available
	if status.LLMAvailable && asi.llmClient != nil {
		asi.log.Info("Creating real learning-enhanced prompt builder with LLM integration")
		return NewRealLearningEnhancedPromptBuilder(
			asi.llmClient,
			asi.vectorDB,
			executionRepo,
			asi.log,
		)
	}

	// Use fail-fast implementation with informative logging
	asi.log.WithField("endpoint", asi.config.SLM.Endpoint).Warn("LLM service unavailable, using fail-fast prompt builder")
	return &FailFastLearningEnhancedPromptBuilder{}
}

// TestLLMConnectivity performs a test request to verify LLM integration
func (asi *AIServiceIntegrator) TestLLMConnectivity(ctx context.Context) error {
	if asi.llmClient == nil {
		return fmt.Errorf("LLM client not configured")
	}

	if !asi.llmClient.IsHealthy() {
		return fmt.Errorf("LLM service health check failed for endpoint: %s", asi.config.GetLLMConfig().Endpoint)
	}

	// Perform a simple test request
	testCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	response, err := asi.llmClient.ChatCompletion(testCtx, "Test connectivity: respond with 'OK'")
	if err != nil {
		return fmt.Errorf("LLM connectivity test failed: %w", err)
	}

	if response == "" {
		return fmt.Errorf("LLM returned empty response")
	}

	llmConfig := asi.config.GetLLMConfig()
	asi.log.WithFields(logrus.Fields{
		"endpoint": llmConfig.Endpoint,
		"provider": llmConfig.Provider,
		"model":    llmConfig.Model,
		"response": response[:min(50, len(response))],
	}).Info("LLM connectivity test successful")

	return nil
}

// GetServiceStatus returns the current status of AI services
func (asi *AIServiceIntegrator) GetServiceStatus(ctx context.Context) (*AIServiceStatus, error) {
	return asi.DetectAndConfigure(ctx)
}

// Enhanced workflow engine constructor that uses AI service integration
func NewDefaultWorkflowEngineWithAIIntegration(
	k8sClient k8s.Client,
	actionRepo actionhistory.Repository,
	monitoringClients *monitoring.MonitoringClients,
	stateStorage StateStorage,
	executionRepo ExecutionRepository,
	config *WorkflowEngineConfig,
	aiConfig *config.Config,
	log *logrus.Logger,
) (*DefaultWorkflowEngine, error) {
	// Create LLM client
	var llmClient llm.Client
	var err error

	if aiConfig != nil && aiConfig.GetLLMConfig().Endpoint != "" {
		llmClient, err = llm.NewClient(aiConfig.GetLLMConfig(), log)
		if err != nil {
			log.WithError(err).Warn("Failed to create LLM client, AI features will use fail-fast implementations")
		}
	}

	// Create HolmesGPT client if configured
	var holmesClient holmesgpt.Client
	if aiConfig != nil && aiConfig.IsHolmesGPTEnabled() {
		holmesConfig := aiConfig.GetHolmesGPTConfig()
		// Create holmesgpt client (following development guideline: reuse existing patterns)
		var err error
		holmesClient, err = holmesgpt.NewClient(holmesConfig.Endpoint, "", log)
		if err != nil {
			log.WithError(err).Warn("Failed to create HolmesGPT client, investigation features will use fallback")
		} else {
			log.WithField("endpoint", holmesConfig.Endpoint).Info("HolmesGPT client created")
		}
	}

	// Create vector database using production factory pattern
	// Following development guideline: integrate with existing code
	var vectorDB vector.VectorDatabase
	if aiConfig != nil && aiConfig.VectorDB.Enabled {
		vectorFactory := vector.NewVectorDatabaseFactory(&aiConfig.VectorDB, nil, log)
		createdVectorDB, err := vectorFactory.CreateVectorDatabase()
		if err != nil {
			log.WithError(err).Warn("Failed to create vector database, using memory fallback")
			vectorDB = vector.NewMemoryVectorDatabase(log)
		} else {
			vectorDB = createdVectorDB
			log.WithField("backend", aiConfig.VectorDB.Backend).Info("Vector database created successfully")
		}
	} else {
		// Graceful fallback when no config is available or vector DB is disabled
		log.Info("Vector database disabled or no config provided, using memory fallback")
		vectorDB = vector.NewMemoryVectorDatabase(log)
	}

	// Create AI service integrator with available context sources
	// Following development guideline: integrate with existing code
	integrator := NewAIServiceIntegrator(
		aiConfig,
		llmClient,
		holmesClient,
		vectorDB, // Real vector database instead of nil
		nil,      // Metrics client will be enhanced
		log,
	)

	// Configure integrator with provided dependencies
	if k8sClient != nil {
		log.WithField("k8s_client_type", fmt.Sprintf("%T", k8sClient)).Debug("K8s client provided for AI integration")
		// In a full implementation, this would configure the integrator with k8s client
	}

	if actionRepo != nil {
		log.WithField("action_repo_type", fmt.Sprintf("%T", actionRepo)).Debug("Action repository provided for AI integration")
		// In a full implementation, this would configure the integrator with action history
	}

	if monitoringClients != nil {
		log.WithField("monitoring_clients_type", fmt.Sprintf("%T", monitoringClients)).Debug("Monitoring clients provided for AI integration")
		// In a full implementation, this would configure the integrator with monitoring
	}

	if stateStorage != nil {
		log.WithField("state_storage_type", fmt.Sprintf("%T", stateStorage)).Debug("State storage provided for AI integration")
	}

	if executionRepo != nil {
		log.WithField("execution_repo_type", fmt.Sprintf("%T", executionRepo)).Debug("Execution repository provided for AI integration")
	}

	if config != nil {
		log.WithFields(map[string]interface{}{
			"max_concurrency":      config.MaxConcurrency,
			"default_step_timeout": config.DefaultStepTimeout,
		}).Debug("Workflow engine config provided for AI integration")
	}

	// Test AI connectivity if enabled
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if llmClient != nil {
		if err := integrator.TestLLMConnectivity(ctx); err != nil {
			log.WithError(err).Warn("LLM connectivity test failed, AI features will degrade gracefully")
		} else {
			log.Info("✅ LLM integration validated successfully - AI features enabled")
		}
	}

	if holmesClient != nil {
		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()
		if err := holmesClient.GetHealth(testCtx); err != nil {
			log.WithError(err).Warn("HolmesGPT connectivity test failed, investigation features will use fallback")
		} else {
			log.Info("✅ HolmesGPT integration validated successfully - Investigation features enabled")
		}
	}

	// Create AI condition evaluator based on service availability
	// Business Requirement: BR-AI-COND-001 - Use real AI condition evaluator instead of nil
	// Following development guideline: integrate with existing code (use factory pattern)
	aiConditionEvaluator := NewDefaultAIConditionEvaluator(
		llmClient,
		holmesClient,
		integrator.vectorDB,
		log,
	)

	// Create the enhanced workflow engine with AI integration
	// Following development guideline: integrate with existing code (use NewDefaultWorkflowEngine)
	workflowEngine := NewDefaultWorkflowEngine(
		k8sClient,
		actionRepo,
		monitoringClients,
		stateStorage,
		executionRepo,
		config,
		log,
	)

	// Set the AI condition evaluator to enable AI-enhanced workflow features
	workflowEngine.SetAIConditionEvaluator(aiConditionEvaluator)

	log.WithFields(logrus.Fields{
		"ai_integration_enabled":     true,
		"llm_client_available":       llmClient != nil,
		"holmesgpt_client_available": holmesClient != nil,
		"vector_db_available":        integrator.vectorDB != nil,
	}).Info("Workflow engine created with AI service integration")

	return workflowEngine, nil
}

// InvestigateAlert performs hybrid AI investigation using the priority-based fallback strategy
// Business Requirement: BR-AI-011, BR-AI-012, BR-AI-013 - Intelligent alert investigation
func (asi *AIServiceIntegrator) InvestigateAlert(ctx context.Context, alert types.Alert) *InvestigationResult {
	asi.log.WithFields(logrus.Fields{
		"alert_name": alert.Name,
		"severity":   alert.Severity,
		"namespace":  alert.Namespace,
	}).Info("Starting hybrid AI investigation")

	status, err := asi.DetectAndConfigure(ctx)
	if err != nil {
		asi.log.WithError(err).Warn("Service detection failed, proceeding with graceful degradation")
		return asi.gracefulInvestigation(ctx, alert)
	}

	// Strategy 1: Try HolmesGPT (highest priority for investigations)
	if status.HolmesGPTAvailable && asi.holmesClient != nil {
		result, err := asi.investigateWithHolmesGPT(ctx, alert)
		if err == nil {
			asi.log.WithField("method", "holmesgpt").Info("Investigation completed successfully")
			return result
		}
		asi.log.WithError(err).Warn("HolmesGPT investigation failed, trying LLM fallback")
	}

	// Strategy 2: Fallback to LLM (general purpose analysis)
	if status.LLMAvailable && asi.llmClient != nil {
		result, err := asi.investigateWithLLM(ctx, alert)
		if err == nil {
			asi.log.WithField("method", "llm_fallback").Info("Investigation completed successfully")
			return result
		}
		asi.log.WithError(err).Warn("LLM investigation failed, using graceful degradation")
	}

	// Strategy 3: Graceful degradation (no AI available)
	asi.log.Warn("All AI services unavailable, using graceful degradation")
	return asi.gracefulInvestigation(ctx, alert)
}

// investigateWithHolmesGPT performs investigation using HolmesGPT with context enrichment
// Business Requirement: BR-AI-011, BR-AI-012, BR-AI-013 - Context-enriched investigation
func (asi *AIServiceIntegrator) investigateWithHolmesGPT(ctx context.Context, alert types.Alert) (*InvestigationResult, error) {
	// Convert alert to HolmesGPT request format (following development guideline: reuse existing patterns)
	request := asi.convertAlertToInvestigateRequest(alert)

	// Enrich context using existing patterns (reusing code from ai_insights_impl.go)
	enrichedRequest := asi.enrichHolmesGPTContext(ctx, request, alert)

	// Perform investigation with enriched context
	response, err := asi.holmesClient.Investigate(ctx, enrichedRequest)
	if err != nil {
		return nil, fmt.Errorf("HolmesGPT investigation failed: %w", err)
	}

	// Convert response to our format (following development guideline: align with business requirements)
	return &InvestigationResult{
		Method:          "holmesgpt_enriched",
		Analysis:        response.Summary, // Use the new field from InvestigateResponse
		Recommendations: asi.extractRecommendationsFromSummary(response.Summary),
		Confidence:      0.8, // Default confidence for HolmesGPT investigations
		ProcessingTime:  0,   // Not tracked in current response
		Source:          "HolmesGPT v0.13.1 (Context-Enriched)",
		Context:         response.ContextUsed,
	}, nil
}

// investigateWithLLM performs investigation using standard LLM with enriched context
// Following development guideline: reuse existing context enrichment patterns for consistent investigation quality
func (asi *AIServiceIntegrator) investigateWithLLM(ctx context.Context, alert types.Alert) (*InvestigationResult, error) {
	// Enrich context for LLM investigation (following same patterns as HolmesGPT)
	// This ensures consistent investigation quality regardless of which AI service is used
	enrichedAlert := asi.enrichLLMContext(ctx, alert)

	// Use existing LLM client for alert analysis with enriched context
	recommendation, err := asi.llmClient.AnalyzeAlert(ctx, enrichedAlert)
	if err != nil {
		return nil, fmt.Errorf("llm analysis failed: %w", err)
	}

	// Extract reasoning summary safely
	reasoningSummary := "LLM analysis completed"
	if recommendation.Reasoning != nil {
		reasoningSummary = recommendation.Reasoning.Summary
	}

	return &InvestigationResult{
		Method:   "llm_fallback_enriched",
		Analysis: reasoningSummary,
		Recommendations: []InvestigationRecommendation{
			{
				Action:      recommendation.Action,
				Description: reasoningSummary,
				Priority:    "medium",
				Confidence:  recommendation.Confidence,
				Parameters:  convertLLMParameters(recommendation.Parameters),
			},
		},
		Confidence:     recommendation.Confidence,
		ProcessingTime: 0, // LLM client doesn't track this
		Source:         fmt.Sprintf("LLM (%s) with Context Enrichment", asi.config.GetLLMConfig().Provider),
		Context: map[string]interface{}{
			"context_enriched":  true,
			"enrichment_source": "ai_service_integrator",
		},
	}, nil
}

// gracefulInvestigation provides basic investigation when AI services are unavailable
func (asi *AIServiceIntegrator) gracefulInvestigation(ctx context.Context, alert types.Alert) *InvestigationResult {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return &InvestigationResult{Analysis: "context_cancelled"}
	default:
	}

	// Basic rule-based analysis based on alert characteristics
	analysis := fmt.Sprintf("Alert analysis for %s: %s", alert.Name, alert.Description)

	// Simple heuristic recommendations based on alert name/severity
	var recommendations []InvestigationRecommendation

	switch alert.Severity {
	case "critical":
		recommendations = append(recommendations, InvestigationRecommendation{
			Action:      "escalate_alert",
			Description: "Critical alert requires immediate attention",
			Priority:    "high",
			Confidence:  0.8,
		})
	case "warning":
		recommendations = append(recommendations, InvestigationRecommendation{
			Action:      "monitor_situation",
			Description: "Warning alert should be monitored closely",
			Priority:    "medium",
			Confidence:  0.6,
		})
	default:
		recommendations = append(recommendations, InvestigationRecommendation{
			Action:      "log_investigation",
			Description: "Alert logged for further analysis",
			Priority:    "low",
			Confidence:  0.5,
		})
	}

	return &InvestigationResult{
		Method:          "graceful_degradation",
		Analysis:        analysis,
		Recommendations: recommendations,
		Confidence:      0.5, // Lower confidence for rule-based analysis
		ProcessingTime:  0,
		Source:          "Rule-based fallback",
		Context:         map[string]interface{}{"fallback_reason": "AI services unavailable"},
	}
}

// InvestigationResult represents the result of an AI investigation
type InvestigationResult struct {
	Method          string                        `json:"method"`            // Investigation method used
	Analysis        string                        `json:"analysis"`          // Analysis summary
	Recommendations []InvestigationRecommendation `json:"recommendations"`   // Action recommendations
	Confidence      float64                       `json:"confidence"`        // Overall confidence score
	ProcessingTime  time.Duration                 `json:"processing_time"`   // Time taken for investigation
	Source          string                        `json:"source"`            // Source of investigation (HolmesGPT, LLM, etc.)
	Context         map[string]interface{}        `json:"context,omitempty"` // Additional context
}

// InvestigationRecommendation represents a single investigation recommendation
type InvestigationRecommendation struct {
	Action      string            `json:"action"`               // Recommended action
	Description string            `json:"description"`          // Description of recommendation
	Priority    string            `json:"priority"`             // Priority level (high, medium, low)
	Risk        string            `json:"risk,omitempty"`       // Risk level
	Confidence  float64           `json:"confidence"`           // Confidence in recommendation
	Parameters  map[string]string `json:"parameters,omitempty"` // Action parameters
}

// Helper functions for conversion (following development guideline: align with business requirements)

// convertAlertToInvestigateRequest converts types.Alert to holmesgpt.InvestigateRequest
// Following development guideline: reuse existing patterns
func (asi *AIServiceIntegrator) convertAlertToInvestigateRequest(alert types.Alert) *holmesgpt.InvestigateRequest {
	return &holmesgpt.InvestigateRequest{
		AlertName:       alert.Name,
		Namespace:       alert.Namespace,
		Labels:          alert.Labels,
		Annotations:     alert.Annotations,
		Priority:        alert.Severity, // Map severity to priority
		AsyncProcessing: false,
		IncludeContext:  true,
	}
}

// extractRecommendationsFromSummary extracts actionable recommendations from summary text
// Following development guideline: align functionality with business requirements
func (asi *AIServiceIntegrator) extractRecommendationsFromSummary(summary string) []InvestigationRecommendation {
	// Simple heuristic extraction - could be enhanced with LLM processing
	recommendations := []InvestigationRecommendation{
		{
			Action:      "review_investigation",
			Description: "Review HolmesGPT investigation results and take appropriate action",
			Priority:    "medium",
			Confidence:  0.8,
			Parameters: map[string]string{
				"investigation_length": fmt.Sprintf("%d", len(summary)),
			},
		},
	}

	// Add specific recommendations based on summary content
	if strings.Contains(strings.ToLower(summary), "restart") {
		recommendations = append(recommendations, InvestigationRecommendation{
			Action:      "restart_resource",
			Description: "Consider restarting the affected resource",
			Priority:    "high",
			Confidence:  0.7,
		})
	}

	if strings.Contains(strings.ToLower(summary), "scale") {
		recommendations = append(recommendations, InvestigationRecommendation{
			Action:      "scale_resource",
			Description: "Consider scaling the affected resource",
			Priority:    "medium",
			Confidence:  0.6,
		})
	}

	return recommendations
}

func convertLLMParameters(params map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range params {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

// enrichHolmesGPTContext enriches investigation context using existing patterns
// Reuses context gathering patterns from ai_insights_impl.go (following development guidelines)
func (asi *AIServiceIntegrator) enrichHolmesGPTContext(ctx context.Context, request *holmesgpt.InvestigateRequest, alert types.Alert) *holmesgpt.InvestigateRequest {
	// Enrich the request by adding context information to annotations
	if request.Annotations == nil {
		request.Annotations = make(map[string]string)
	}

	// Add basic enrichment (existing functionality) using annotations
	request.Annotations["kubernaut_source"] = "ai_service_integrator"
	request.Annotations["enrichment_timestamp"] = time.Now().UTC().Format(time.RFC3339)

	// 1. Metrics Context - Reuse existing MetricsClient.GetResourceMetrics pattern
	if asi.metricsClient != nil && alert.Namespace != "" && alert.Resource != "" {
		if metrics := asi.GatherCurrentMetricsContext(ctx, alert); metrics != nil {
			request.Annotations["metrics_available"] = "true"
			asi.log.WithField("alert", alert.Name).Debug("Metrics context available")
		}
	}

	// 2. Action History Context - Reuse existing patterns from EnhancedAssessor
	if actionHistoryContext := asi.GatherActionHistoryContext(ctx, alert); actionHistoryContext != nil {
		request.Annotations["action_history_available"] = "true"
		if contextHash, ok := actionHistoryContext["context_hash"].(string); ok {
			request.Annotations["action_context_hash"] = contextHash
		}
		asi.log.WithField("alert", alert.Name).Debug("Added action history context to investigation")
	}

	// 3. Kubernetes Context - Basic cluster information
	if alert.Namespace != "" {
		request.Annotations["kubernetes_context_available"] = "true"
		request.Annotations["kubernetes_namespace"] = alert.Namespace
		if alert.Resource != "" {
			request.Annotations["kubernetes_resource"] = alert.Resource
		}
		asi.log.WithField("alert", alert.Name).Debug("Added kubernetes context to investigation")
	}

	return request
}

// enrichLLMContext enriches alert with context for LLM investigation
// Following development guideline: reuse existing context gathering patterns to ensure consistent investigation quality
func (asi *AIServiceIntegrator) enrichLLMContext(ctx context.Context, alert types.Alert) types.Alert {
	// Create enriched copy of alert (preserving original alert data)
	enrichedAlert := alert

	// Initialize additional context fields if they don't exist
	if enrichedAlert.Annotations == nil {
		enrichedAlert.Annotations = make(map[string]string)
	}

	// Add enrichment metadata
	enrichedAlert.Annotations["kubernaut_context_enriched"] = "true"
	enrichedAlert.Annotations["kubernaut_enrichment_timestamp"] = time.Now().UTC().Format(time.RFC3339)
	enrichedAlert.Annotations["kubernaut_enrichment_source"] = "ai_service_integrator"

	// 1. Metrics Context - Reuse existing pattern from HolmesGPT enrichment
	if asi.metricsClient != nil && alert.Namespace != "" && alert.Resource != "" {
		if metrics := asi.GatherCurrentMetricsContext(ctx, alert); metrics != nil {
			// Inject metrics context into alert annotations for LLM consumption
			if metricsData, ok := metrics["namespace"].(string); ok {
				enrichedAlert.Annotations["kubernaut_metrics_namespace"] = metricsData
			}
			if metricsData, ok := metrics["resource_name"].(string); ok {
				enrichedAlert.Annotations["kubernaut_metrics_resource"] = metricsData
			}
			if metricsData, ok := metrics["collection_time"]; ok {
				enrichedAlert.Annotations["kubernaut_metrics_collection_time"] = fmt.Sprintf("%v", metricsData)
			}
			enrichedAlert.Annotations["kubernaut_metrics_available"] = "true"
			asi.log.WithField("alert", alert.Name).Debug("Added metrics context to LLM investigation")
		}
	}

	// 2. Action History Context - Reuse existing pattern from HolmesGPT enrichment
	if actionHistoryContext := asi.GatherActionHistoryContext(ctx, alert); actionHistoryContext != nil {
		// Inject action history context into alert annotations
		if contextHash, ok := actionHistoryContext["context_hash"].(string); ok {
			enrichedAlert.Annotations["kubernaut_action_context_hash"] = contextHash
		}
		if alertType, ok := actionHistoryContext["alert_type"].(string); ok {
			enrichedAlert.Annotations["kubernaut_action_alert_type"] = alertType
		}
		enrichedAlert.Annotations["kubernaut_action_history_available"] = "true"
		asi.log.WithField("alert", alert.Name).Debug("Added action history context to LLM investigation")
	}

	// 3. Enhanced Description with Context Summary
	// Following development guideline: reuse existing patterns while enhancing functionality
	contextSummary := asi.generateContextSummary(enrichedAlert)
	if contextSummary != "" {
		enrichedAlert.Description = fmt.Sprintf("%s\n\nCONTEXT ANALYSIS:\n%s",
			enrichedAlert.Description, contextSummary)
	}

	asi.log.WithFields(logrus.Fields{
		"alert":                alert.Name,
		"enriched_annotations": len(enrichedAlert.Annotations) - len(alert.Annotations),
	}).Debug("LLM context enrichment completed")

	return enrichedAlert
}

// generateContextSummary creates a human-readable context summary for LLM consumption
// Following development guideline: reuse existing patterns and integrate with existing code
func (asi *AIServiceIntegrator) generateContextSummary(enrichedAlert types.Alert) string {
	var contextParts []string

	// Historical patterns context
	if enrichedAlert.Annotations["kubernaut_action_history_available"] == "true" {
		if contextHash := enrichedAlert.Annotations["kubernaut_action_context_hash"]; contextHash != "" {
			contextParts = append(contextParts, fmt.Sprintf("Historical Pattern: Alert correlation hash '%s' available for pattern analysis", contextHash[:8]))
		}
	}

	// Metrics context
	if enrichedAlert.Annotations["kubernaut_metrics_available"] == "true" {
		if collectionTime := enrichedAlert.Annotations["kubernaut_metrics_collection_time"]; collectionTime != "" {
			contextParts = append(contextParts, fmt.Sprintf("Current Metrics: Performance data available from %s", collectionTime))
		}
	}

	// Kubernetes context (always available from basic alert)
	if enrichedAlert.Namespace != "" && enrichedAlert.Resource != "" {
		contextParts = append(contextParts, fmt.Sprintf("Kubernetes Context: Resource '%s' in namespace '%s'",
			enrichedAlert.Resource, enrichedAlert.Namespace))
	}

	// Enrichment metadata
	if enrichmentTime := enrichedAlert.Annotations["kubernaut_enrichment_timestamp"]; enrichmentTime != "" {
		contextParts = append(contextParts, fmt.Sprintf("Context Enrichment: Enhanced at %s with historical and metrics data", enrichmentTime))
	}

	return fmt.Sprintf("- %s", fmt.Sprintf(strings.Join(contextParts, "\n- ")))
}

// GatherCurrentMetricsContext reuses existing MetricsClient pattern from ai_insights_impl.go
// Made public for Context API reuse (following development guideline: reuse code whenever possible)
func (asi *AIServiceIntegrator) GatherCurrentMetricsContext(ctx context.Context, alert types.Alert) map[string]interface{} {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		asi.log.WithContext(ctx).Warn("Context cancelled during metrics gathering")
		return map[string]interface{}{
			"error":     "context_cancelled",
			"namespace": alert.Namespace,
		}
	default:
	}

	// Following existing pattern from EnhancedAssessor.gatherPreActionMetrics
	// Enhanced implementation with context awareness
	metricsContext := map[string]interface{}{
		"namespace":         alert.Namespace,
		"resource_name":     alert.Resource,
		"collection_time":   time.Now().UTC(),
		"metrics_available": asi.metricsClient != nil,
	}

	// Add context deadline information if available
	if deadline, ok := ctx.Deadline(); ok {
		metricsContext["context_deadline"] = deadline
		metricsContext["time_remaining"] = time.Until(deadline).Seconds()
	}

	// Add context values if available
	if requestID := ctx.Value("request_id"); requestID != nil {
		metricsContext["request_id"] = requestID
	}

	// TODO: Integrate with actual metrics collection once interface is clarified
	// This follows development guideline: reuse existing patterns without breaking changes

	return metricsContext
}

// GatherActionHistoryContext reuses existing patterns from effectiveness repository
// Made public for Context API reuse (following development guideline: reuse code whenever possible)
func (asi *AIServiceIntegrator) GatherActionHistoryContext(ctx context.Context, alert types.Alert) map[string]interface{} {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		asi.log.WithContext(ctx).Warn("Context cancelled during action history gathering")
		return map[string]interface{}{
			"error":      "context_cancelled",
			"alert_type": alert.Name,
		}
	default:
	}

	// Basic action history context - following existing patterns but simplified
	// This avoids creating complex new interfaces while following existing patterns
	contextHash := asi.CreateActionContextHash(alert.Name, alert.Namespace)

	actionHistoryContext := map[string]interface{}{
		"context_hash":    contextHash,
		"alert_type":      alert.Name,
		"namespace":       alert.Namespace,
		"historical_data": "available", // Placeholder - could be enhanced with actual history queries
		"collection_time": time.Now().UTC(),
	}

	// Add context deadline information if available
	if deadline, ok := ctx.Deadline(); ok {
		actionHistoryContext["context_deadline"] = deadline
		actionHistoryContext["time_remaining"] = time.Until(deadline).Seconds()
	}

	// Add context trace information if available
	if traceID := ctx.Value("trace_id"); traceID != nil {
		actionHistoryContext["trace_id"] = traceID
	}

	return actionHistoryContext
}

// CreateActionContextHash follows existing pattern from EnhancedAssessor.hashActionContext
// Made public for Context API reuse (following development guideline: reuse code whenever possible)
func (asi *AIServiceIntegrator) CreateActionContextHash(alertName, namespace string) string {
	// Reuse existing hash pattern from ai_insights_impl.go
	contextString := fmt.Sprintf("%s:%s", alertName, namespace)
	return fmt.Sprintf("%x", contextString) // Simplified hash for now
}

// min function is already defined in the package
