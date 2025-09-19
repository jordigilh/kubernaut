package holmesgpt

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	contextpkg "github.com/jordigilh/kubernaut/pkg/ai/context"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// ComplexityAssessment represents the assessment of alert complexity
// Business Requirement: BR-HOLMES-007 - Investigation complexity assessment
type ComplexityAssessment struct {
	Level       string   `json:"level"`       // "low", "medium", "high"
	Score       float64  `json:"score"`       // 0.0 to 1.0
	Factors     []string `json:"factors"`     // List of complexity factors
	Explanation string   `json:"explanation"` // Human-readable explanation
}

// OverallOrchestrationMetrics contains overall orchestration performance metrics
// Business Requirement: BR-EXTERNAL-007 - Context orchestration metrics
type OverallOrchestrationMetrics struct {
	ContextGatheringsCompleted int           `json:"context_gatherings_completed"`
	AverageResponseTime        time.Duration `json:"average_response_time"`
	SuccessRate                float64       `json:"success_rate"`
	CostOptimizationScore      float64       `json:"cost_optimization_score"`
	OrchestrationVersion       string        `json:"orchestration_version"`
	SystemUptime               time.Duration `json:"system_uptime"`
	ContextCacheSize           int           `json:"context_cache_size"`
	TotalInvestigations        int           `json:"total_investigations"`
	ActiveInvestigations       int           `json:"active_investigations"`
	ToolsetDeployments         int           `json:"toolset_deployments"`
	CompletedInvestigations    int           `json:"completed_investigations"`
}

// InvestigationMetrics contains metrics for individual investigations
// Business Requirement: BR-EXTERNAL-007 - Investigation tracking and metrics
type InvestigationMetrics struct {
	InvestigationID    string        `json:"investigation_id"`
	StartTime          time.Time     `json:"start_time"`
	EndTime            time.Time     `json:"end_time"`
	ResponseTime       time.Duration `json:"response_time"`
	CompletionTime     time.Duration `json:"completion_time"`
	Success            bool          `json:"success"`
	ContextSourcesUsed []string      `json:"context_sources_used"`
	ContextSizeBytes   int           `json:"context_size_bytes"`
	ErrorCode          string        `json:"error_code,omitempty"`
}

// ============================================================================
// STRUCTURED ALERT TYPES - Following project guideline: use structured field values instead of interface{}
// ============================================================================

// AlertData represents structured alert data instead of interface{}
// Following project guideline: ALWAYS attempt to use structured field values and AVOID using any or interface{}
type AlertData struct {
	Type        string            `json:"type"`                   // "prometheus", "kubernetes", "system", "application"
	Severity    string            `json:"severity"`               // "critical", "high", "medium", "low"
	Source      string            `json:"source"`                 // Source system that generated the alert
	Namespace   string            `json:"namespace,omitempty"`    // Kubernetes namespace
	Resource    string            `json:"resource,omitempty"`     // Resource name (pod, service, etc.)
	Labels      map[string]string `json:"labels,omitempty"`       // Alert labels
	Annotations map[string]string `json:"annotations,omitempty"`  // Alert annotations
	Message     string            `json:"message"`                // Alert message/description
	Timestamp   time.Time         `json:"timestamp"`              // When the alert was triggered
	Duration    time.Duration     `json:"duration,omitempty"`     // How long the alert has been active
	MetricValue float64           `json:"metric_value,omitempty"` // Associated metric value if available
	Threshold   float64           `json:"threshold,omitempty"`    // Alert threshold if applicable
	RuleID      string            `json:"rule_id,omitempty"`      // Alert rule identifier
}

// InvestigationMetadata contains structured metadata for investigations
// Following project guideline: use structured field values instead of interface{}
type InvestigationMetadata struct {
	Source         string               `json:"source"`                    // Source system that initiated the investigation
	RequestedBy    string               `json:"requested_by,omitempty"`    // User or system that requested the investigation
	Tags           map[string]string    `json:"tags,omitempty"`            // Key-value tags for categorization
	Priority       int                  `json:"priority"`                  // Investigation priority level
	Timeout        time.Duration        `json:"timeout,omitempty"`         // Maximum investigation duration
	ResourceLimits *ResourceLimits      `json:"resource_limits,omitempty"` // Resource usage limits
	Configuration  *InvestigationConfig `json:"configuration,omitempty"`   // Investigation-specific configuration
	CreatedAt      time.Time            `json:"created_at"`                // When the investigation was created
	UpdatedAt      time.Time            `json:"updated_at"`                // Last update timestamp
}

// ResourceLimits defines resource usage limits for investigations
type ResourceLimits struct {
	MaxContextSize    int           `json:"max_context_size"`    // Maximum context size in bytes
	MaxDuration       time.Duration `json:"max_duration"`        // Maximum investigation duration
	MaxContextSources int           `json:"max_context_sources"` // Maximum number of context sources
}

// InvestigationConfig contains investigation-specific configuration
type InvestigationConfig struct {
	EnableTracing   bool          `json:"enable_tracing"`    // Enable distributed tracing
	EnableAuditLogs bool          `json:"enable_audit_logs"` // Enable audit log collection
	ContextSources  []string      `json:"context_sources"`   // Allowed context sources
	MaxRetries      int           `json:"max_retries"`       // Maximum retry attempts
	RetryBackoff    time.Duration `json:"retry_backoff"`     // Retry backoff duration
	EnableCaching   bool          `json:"enable_caching"`    // Enable context caching
	CacheTTL        time.Duration `json:"cache_ttl"`         // Cache time-to-live
}

// InvestigationRequest represents a request to orchestrate an AI-driven investigation
// Business Requirement: BR-HOLMES-006 - Investigation request handling
// Enhanced with structured AlertData and Metadata following project guidelines
type InvestigationRequest struct {
	InvestigationID string                 `json:"investigation_id"`
	Alert           *AlertData             `json:"alert"` // Now uses structured AlertData instead of interface{}
	AlertType       string                 `json:"alert_type"`
	Namespace       string                 `json:"namespace"`
	Priority        int                    `json:"priority"`
	Metadata        *InvestigationMetadata `json:"metadata,omitempty"` // Now uses structured metadata instead of map[string]interface{}
}

// AIOrchestrationCoordinator orchestrates the complete AI-driven context gathering workflow
// Business Requirements: BR-HOLMES-006 to BR-HOLMES-010, BR-EXTERNAL-002 to BR-EXTERNAL-005
// Following project guideline: Integrate all new code with existing code
type AIOrchestrationCoordinator struct {
	dynamicToolsetManager   *DynamicToolsetManager
	toolsetDeploymentClient ToolsetDeploymentClient
	serviceIntegration      *ServiceIntegration
	logger                  *logrus.Logger

	// Orchestration state
	deployedToolsets     map[string]*ToolsetConfig
	activeInvestigations map[string]*ActiveInvestigation
	contextCache         map[string]*CachedContext
	performanceMonitor   *OrchestrationPerformanceMonitor

	mu                sync.RWMutex
	ctx               context.Context
	cancelFunc        context.CancelFunc
	stopChannel       chan struct{}
	contextCacheMutex sync.RWMutex
}

// ActiveInvestigation tracks ongoing AI-driven investigations
// Business Requirement: BR-EXTERNAL-005 - Investigation state management
// Enhanced with structured context data and atomic status management following project guidelines
type ActiveInvestigation struct {
	InvestigationID string                    `json:"investigation_id"`
	AlertType       string                    `json:"alert_type"`
	AlertData       *AlertData                `json:"alert_data"` // Structured alert data instead of keeping it separate
	Namespace       string                    `json:"namespace"`
	StartTime       time.Time                 `json:"start_time"`
	Status          string                    `json:"status"` // "active", "paused", "completed", "failed"
	ContextStrategy *ContextGatheringStrategy `json:"context_strategy"`
	GatheredContext *contextpkg.ContextData   `json:"gathered_context"` // Now uses structured ContextData instead of map[string]interface{}
	ToolChain       *ToolChainDefinition      `json:"tool_chain,omitempty"`
	Metadata        *InvestigationMetadata    `json:"metadata"` // Now uses structured metadata
	LastActivity    time.Time                 `json:"last_activity"`

	// RACE CONDITION FIX: Atomic status management for BR-EXTERNAL-005
	atomicStatus     int64        `json:"-"` // 0=active, 1=paused, 2=completed, 3=failed
	atomicUpdateTime int64        `json:"-"` // Unix timestamp for atomic updates
	statusMutex      sync.RWMutex `json:"-"` // Protects Status and LastActivity fields
}

// ContextGatheringStrategy defines the strategy for gathering context
// Business Requirement: BR-HOLMES-006, BR-HOLMES-007 - Adaptive context gathering
type ContextGatheringStrategy struct {
	AlertType        string            `json:"alert_type"`
	Complexity       string            `json:"complexity"` // "simple", "moderate", "complex", "critical"
	Priority         int               `json:"priority"`
	RequiredContexts []string          `json:"required_contexts"`
	OptionalContexts []string          `json:"optional_contexts"`
	ContextSources   []string          `json:"context_sources"`
	MaxContextSize   int               `json:"max_context_size"`
	GatheringOrder   []string          `json:"gathering_order"`
	AdaptiveRules    []AdaptiveRule    `json:"adaptive_rules"`
	FallbackStrategy *FallbackStrategy `json:"fallback_strategy,omitempty"`
}

// AdaptiveRuleParameters contains structured parameters for adaptive rules
// Following project guideline: use structured field values instead of interface{}
type AdaptiveRuleParameters struct {
	AdditionalContexts  []string          `json:"additional_contexts,omitempty"`   // Additional context types to gather
	PriorityAdjustment  int               `json:"priority_adjustment,omitempty"`   // Priority level adjustment
	ContextSizeIncrease int               `json:"context_size_increase,omitempty"` // Context size increase in bytes
	TimeoutExtension    time.Duration     `json:"timeout_extension,omitempty"`     // Extend investigation timeout
	RetryCount          int               `json:"retry_count,omitempty"`           // Number of retries
	FallbackStrategy    string            `json:"fallback_strategy,omitempty"`     // Fallback strategy name
	Tags                map[string]string `json:"tags,omitempty"`                  // Additional tags to set
}

// AdaptiveRule defines rules for adaptive context gathering
// Business Requirement: BR-HOLMES-007 - Investigation progress adaptation
// Enhanced with structured parameters following project guidelines
type AdaptiveRule struct {
	Condition  string                  `json:"condition"`  // "confidence_low", "missing_context", "error_rate_high"
	Action     string                  `json:"action"`     // "expand_context", "change_priority", "add_context_type"
	Parameters *AdaptiveRuleParameters `json:"parameters"` // Now uses structured parameters instead of map[string]interface{}
	Threshold  float64                 `json:"threshold,omitempty"`
}

// FallbackStrategy defines fallback behavior when primary strategy fails
// Business Requirement: BR-HOLMES-011, BR-HOLMES-012 - Fallback mechanisms
type FallbackStrategy struct {
	UseStaticEnrichment bool             `json:"use_static_enrichment"`
	MinimumContextTypes []string         `json:"minimum_context_types"`
	MaxRetries          int              `json:"max_retries"`
	RetryDelay          time.Duration    `json:"retry_delay"`
	EscalationRules     []EscalationRule `json:"escalation_rules,omitempty"`
}

// EscalationMetadata contains structured metadata for escalation rules
// Following project guideline: use structured field values instead of interface{}
type EscalationMetadata struct {
	Contact         string            `json:"contact,omitempty"`          // Contact information for escalation
	Department      string            `json:"department,omitempty"`       // Department to escalate to
	Severity        string            `json:"severity"`                   // Escalation severity level
	Tags            map[string]string `json:"tags,omitempty"`             // Additional tags
	AutoResolve     bool              `json:"auto_resolve"`               // Whether to auto-resolve
	EscalationDelay time.Duration     `json:"escalation_delay,omitempty"` // Delay before escalation
	MaxRetries      int               `json:"max_retries,omitempty"`      // Maximum retry attempts before escalation
}

// EscalationRule defines when to escalate context gathering failures
// Enhanced with structured metadata following project guidelines
type EscalationRule struct {
	FailureThreshold int                 `json:"failure_threshold"`
	Action           string              `json:"action"` // "human_intervention", "static_fallback", "abort"
	NotificationMode string              `json:"notification_mode"`
	Metadata         *EscalationMetadata `json:"metadata,omitempty"` // Now uses structured metadata instead of map[string]interface{}
}

// CachedContext represents cached context data
// Business Requirement: BR-CONTEXT-010 - Context caching with intelligent invalidation
type CachedContext struct {
	Key         string                  `json:"key"`
	Data        *contextpkg.ContextData `json:"data"` // Now uses structured ContextData instead of map[string]interface{}
	CachedAt    time.Time               `json:"cached_at"`
	ExpiresAt   time.Time               `json:"expires_at"`
	AccessCount int                     `json:"access_count"`
	LastAccess  time.Time               `json:"last_access"`
	Tags        []string                `json:"tags"`
}

// OrchestrationPerformanceMonitor monitors AI orchestration performance
// Business Requirement: BR-EXTERNAL-007 - Context orchestration metrics
type OrchestrationPerformanceMonitor struct {
	InvestigationMetrics map[string]*InvestigationMetrics `json:"investigation_metrics"`
	OverallMetrics       *OverallOrchestrationMetrics     `json:"overall_metrics"`
	StartTime            time.Time                        `json:"start_time"`
	mu                   sync.RWMutex
}

// NewAIOrchestrationCoordinator creates a new AI orchestration coordinator
// Business Requirements: BR-HOLMES-006 to BR-HOLMES-015, BR-EXTERNAL-001 to BR-EXTERNAL-005
// Following project guideline: Reuse existing patterns and integrate with existing code
func NewAIOrchestrationCoordinator(
	dynamicToolsetManager *DynamicToolsetManager,
	serviceIntegration *ServiceIntegration,
	holmesGPTEndpoint string,
	logger *logrus.Logger,
) *AIOrchestrationCoordinator {
	ctx, cancel := context.WithCancel(context.Background())

	// Create toolset deployment client
	deploymentClient := NewToolsetDeploymentClient(holmesGPTEndpoint, logger)

	coordinator := &AIOrchestrationCoordinator{
		dynamicToolsetManager:   dynamicToolsetManager,
		toolsetDeploymentClient: deploymentClient,
		serviceIntegration:      serviceIntegration,
		logger:                  logger,
		deployedToolsets:        make(map[string]*ToolsetConfig),
		activeInvestigations:    make(map[string]*ActiveInvestigation),
		contextCache:            make(map[string]*CachedContext),
		performanceMonitor: &OrchestrationPerformanceMonitor{
			InvestigationMetrics: make(map[string]*InvestigationMetrics),
			OverallMetrics: &OverallOrchestrationMetrics{
				ContextGatheringsCompleted: 0,
				AverageResponseTime:        0,
				SuccessRate:                1.0,
				CostOptimizationScore:      0.75,
				OrchestrationVersion:       "1.0.0",
			},
			StartTime: time.Now(),
		},
		ctx:         ctx,
		cancelFunc:  cancel,
		stopChannel: make(chan struct{}),
	}

	logger.Info("AI orchestration coordinator initialized for dynamic context gathering")
	return coordinator
}

// Start initiates the AI orchestration coordinator
// Business Requirement: BR-HOLMES-020 - Real-time toolset configuration updates
func (coord *AIOrchestrationCoordinator) Start(ctx context.Context) error {
	coord.logger.Info("Starting AI orchestration coordinator")

	// Deploy initial toolsets to HolmesGPT
	if err := coord.deployInitialToolsets(ctx); err != nil {
		coord.logger.WithError(err).Error("Failed to deploy initial toolsets")
		return fmt.Errorf("failed to deploy initial toolsets: %w", err)
	}

	// Start performance monitoring
	go coord.runPerformanceMonitoring()

	// Start cache cleanup
	go coord.runCacheCleanup()

	coord.logger.Info("AI orchestration coordinator started successfully")
	return nil
}

// StartInvestigation initiates an AI-driven investigation workflow
// Business Requirements: BR-HOLMES-006, BR-EXTERNAL-005 - Investigation orchestration and state management
// RACE CONDITION FIX: Reorganized to avoid nested locks and maintain consistent lock ordering
func (coord *AIOrchestrationCoordinator) StartInvestigation(ctx context.Context, req *InvestigationRequest) (*ActiveInvestigation, error) {
	investigationID := fmt.Sprintf("inv_%d_%s", time.Now().Unix(), req.AlertType)

	coord.logger.WithFields(logrus.Fields{
		"investigation_id": investigationID,
		"alert_type":       req.AlertType,
		"namespace":        req.Namespace,
	}).Info("BR-HOLMES-006: Starting AI-driven investigation")

	// Determine context gathering strategy based on alert characteristics
	strategy, err := coord.determineContextStrategy(req)
	if err != nil {
		return nil, fmt.Errorf("failed to determine context strategy: %w", err)
	}

	// Create active investigation with structured data - following project guidelines
	investigation := &ActiveInvestigation{
		InvestigationID: investigationID,
		AlertType:       req.AlertType,
		AlertData:       req.Alert, // Store structured alert data
		Namespace:       req.Namespace,
		StartTime:       time.Now(),
		Status:          "active",
		ContextStrategy: strategy,
		GatheredContext: &contextpkg.ContextData{}, // Initialize structured context data instead of map[string]interface{}
		Metadata:        req.Metadata,              // Now uses structured metadata
		LastActivity:    time.Now(),
	}

	// RACE CONDITION FIX: Initialize atomic status management
	investigation.InitializeStatusAtomic()

	// RACE CONDITION FIX: Initialize metrics separately with proper lock ordering
	// Update performance metrics first (performanceMonitor.mu always acquired before coord.mu)
	coord.performanceMonitor.mu.Lock()
	coord.performanceMonitor.InvestigationMetrics[investigationID] = &InvestigationMetrics{
		InvestigationID: investigationID,
		StartTime:       time.Now(),
	}
	coord.performanceMonitor.OverallMetrics.TotalInvestigations++
	coord.performanceMonitor.OverallMetrics.ActiveInvestigations++
	coord.performanceMonitor.mu.Unlock()

	// Now acquire coordinator lock to store active investigation
	coord.mu.Lock()
	coord.activeInvestigations[investigationID] = investigation
	coord.mu.Unlock()

	// Start asynchronous context gathering
	go coord.gatherContextAsync(ctx, investigation)

	coord.logger.WithFields(logrus.Fields{
		"business_requirement": "BR-HOLMES-006",
		"investigation_id":     investigationID,
		"strategy_complexity":  strategy.Complexity,
		"required_contexts":    len(strategy.RequiredContexts),
	}).Info("BR-HOLMES-006: AI-driven investigation started with determined strategy")

	return investigation, nil
}

// deployInitialToolsets deploys Kubernaut toolsets to HolmesGPT
// Business Requirement: BR-HOLMES-001 - Custom Kubernaut toolset deployment
// Removed backwards compatibility - implements proper error handling following project guidelines
func (coord *AIOrchestrationCoordinator) deployInitialToolsets(ctx context.Context) error {
	// Get available toolsets from dynamic toolset manager
	availableToolsets := coord.dynamicToolsetManager.GetAvailableToolsets()

	if len(availableToolsets) == 0 {
		return fmt.Errorf("no toolsets available for deployment")
	}

	coord.logger.WithField("available_toolsets", len(availableToolsets)).Info("BR-HOLMES-001: Deploying available toolsets to HolmesGPT")

	var deploymentErrors []string
	successfulDeployments := 0

	for _, toolset := range availableToolsets {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Deploy each toolset to HolmesGPT
		response, err := coord.toolsetDeploymentClient.DeployToolset(ctx, toolset)
		if err != nil {
			errorMsg := fmt.Sprintf("failed to deploy toolset %s: %v", toolset.Name, err)
			deploymentErrors = append(deploymentErrors, errorMsg)
			coord.logger.WithError(err).WithField("toolset_name", toolset.Name).Error("BR-HOLMES-001: Failed to deploy toolset")
			continue // Continue with other toolsets
		}

		if response.Success {
			coord.deployedToolsets[toolset.Name] = toolset
			coord.performanceMonitor.mu.Lock()
			coord.performanceMonitor.OverallMetrics.ToolsetDeployments++
			coord.performanceMonitor.mu.Unlock()

			successfulDeployments++

			coord.logger.WithFields(logrus.Fields{
				"business_requirement": "BR-HOLMES-001",
				"toolset_name":         response.ToolsetName,
				"tools_count":          response.ToolsCount,
			}).Info("BR-HOLMES-001: Toolset deployed successfully to HolmesGPT")
		} else {
			errorMsg := fmt.Sprintf("toolset deployment failed for %s: %s", toolset.Name, response.Message)
			deploymentErrors = append(deploymentErrors, errorMsg)
			coord.logger.WithFields(logrus.Fields{
				"toolset_name": toolset.Name,
				"message":      response.Message,
			}).Error("BR-HOLMES-001: Toolset deployment failed")
		}
	}

	coord.logger.WithFields(logrus.Fields{
		"deployed_toolsets":      len(coord.deployedToolsets),
		"successful_deployments": successfulDeployments,
		"failed_deployments":     len(deploymentErrors),
	}).Info("BR-HOLMES-001: Initial toolset deployment completed")

	// Return error if no toolsets were successfully deployed
	if successfulDeployments == 0 {
		return fmt.Errorf("failed to deploy any toolsets: %v", deploymentErrors)
	}

	// Return error if there were failures but some successes (partial failure)
	if len(deploymentErrors) > 0 {
		coord.logger.WithField("deployment_errors", deploymentErrors).Warn("BR-HOLMES-001: Some toolset deployments failed")
		return fmt.Errorf("partial deployment failure: %d toolsets failed to deploy: %v", len(deploymentErrors), deploymentErrors)
	}

	return nil // All deployments successful
}

// determineContextStrategy determines the optimal context gathering strategy
// Business Requirement: BR-HOLMES-006 - Context requirements based on alert characteristics
// Removed backwards compatibility - implements proper error handling following project guidelines
func (coord *AIOrchestrationCoordinator) determineContextStrategy(req *InvestigationRequest) (*ContextGatheringStrategy, error) {
	// Input validation - Following project guideline: proper error handling
	if req == nil {
		return nil, fmt.Errorf("investigation request is required")
	}
	if req.Alert == nil {
		return nil, fmt.Errorf("alert data is required for strategy determination")
	}
	if req.AlertType == "" {
		return nil, fmt.Errorf("alert type is required")
	}

	// Determine complexity based on alert analysis using structured data
	complexity := coord.assessAlertComplexity(req.Alert)

	// Validate complexity assessment
	if complexity.Level == "" {
		return nil, fmt.Errorf("failed to assess alert complexity")
	}

	// Base strategy based on complexity
	strategy := &ContextGatheringStrategy{
		AlertType:  req.AlertType,
		Complexity: complexity.Level,
		Priority:   coord.calculatePriority(req.Alert, complexity),
		AdaptiveRules: []AdaptiveRule{
			{
				Condition: "confidence_low",
				Action:    "expand_context",
				Threshold: 0.7,
				Parameters: &AdaptiveRuleParameters{
					AdditionalContexts: []string{"logs", "events"},
				},
			},
		},
		FallbackStrategy: &FallbackStrategy{
			UseStaticEnrichment: true,
			MinimumContextTypes: []string{"kubernetes"},
			MaxRetries:          3,
			RetryDelay:          time.Second * 5,
		},
	}

	// Customize strategy based on complexity - Following project guideline: proper error handling
	switch complexity.Level {
	case "high", "critical":
		strategy.RequiredContexts = []string{"kubernetes", "metrics", "logs", "action-history", "events"}
		strategy.OptionalContexts = []string{"traces", "network-flows"}
		strategy.GatheringOrder = []string{"kubernetes", "logs", "metrics", "action-history", "events"}

	case "medium", "complex":
		strategy.RequiredContexts = []string{"kubernetes", "metrics", "action-history"}
		strategy.OptionalContexts = []string{"logs", "events"}
		strategy.GatheringOrder = []string{"kubernetes", "metrics", "action-history", "logs"}

	case "low", "moderate":
		strategy.RequiredContexts = []string{"kubernetes", "metrics"}
		strategy.OptionalContexts = []string{"action-history"}
		strategy.GatheringOrder = []string{"kubernetes", "metrics", "action-history"}

	case "minimal":
		strategy.RequiredContexts = []string{"kubernetes"}
		strategy.OptionalContexts = []string{"metrics"}
		strategy.GatheringOrder = []string{"kubernetes", "metrics"}

	default:
		return nil, fmt.Errorf("unknown complexity level: %s", complexity.Level)
	}

	// Validate strategy configuration
	if len(strategy.RequiredContexts) == 0 {
		return nil, fmt.Errorf("strategy must have at least one required context")
	}
	if len(strategy.GatheringOrder) == 0 {
		return nil, fmt.Errorf("strategy must define context gathering order")
	}

	coord.logger.WithFields(logrus.Fields{
		"business_requirement": "BR-HOLMES-006",
		"alert_type":           req.AlertType,
		"complexity":           complexity,
		"required_contexts":    len(strategy.RequiredContexts),
		"optional_contexts":    len(strategy.OptionalContexts),
	}).Info("BR-HOLMES-006: Context gathering strategy determined")

	return strategy, nil
}

// gatherContextAsync performs asynchronous context gathering for an investigation
// Business Requirements: BR-HOLMES-007, BR-EXTERNAL-002 - Adaptive gathering and async patterns
// RACE CONDITION FIX: Fixed nested lock ordering in defer function
func (coord *AIOrchestrationCoordinator) gatherContextAsync(ctx context.Context, investigation *ActiveInvestigation) {
	defer func() {
		// RACE CONDITION FIX: Maintain consistent lock ordering - performanceMonitor.mu before coord.mu
		coord.performanceMonitor.mu.Lock()
		coord.performanceMonitor.OverallMetrics.ActiveInvestigations--
		coord.performanceMonitor.OverallMetrics.CompletedInvestigations++
		if metrics, exists := coord.performanceMonitor.InvestigationMetrics[investigation.InvestigationID]; exists {
			metrics.EndTime = time.Now()
			metrics.CompletionTime = time.Since(metrics.StartTime)
		}
		coord.performanceMonitor.mu.Unlock()

		// RACE CONDITION FIX: Use atomic status transition
		if !investigation.TransitionStatusAtomic("active", "completed") {
			// If we can't transition from active to completed, try from paused
			investigation.TransitionStatusAtomic("paused", "completed")
		}

		coord.logger.WithFields(logrus.Fields{
			"business_requirement": "BR-EXTERNAL-002",
			"investigation_id":     investigation.InvestigationID,
			"contexts_gathered":    coord.countGatheredContexts(investigation.GatheredContext),
		}).Info("BR-EXTERNAL-002: Asynchronous context gathering completed")
	}()

	strategy := investigation.ContextStrategy

	// Gather contexts in specified order
	for _, contextType := range strategy.GatheringOrder {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Check if this context is required or should be gathered - no backwards compatibility
		if coord.shouldGatherContext(investigation.AlertData, strategy) {
			contextData, err := coord.gatherSingleContext(ctx, contextType, investigation.AlertData)
			if err != nil {
				coord.logger.WithError(err).WithFields(logrus.Fields{
					"investigation_id": investigation.InvestigationID,
					"context_type":     contextType,
				}).Error("BR-HOLMES-007: Context gathering failed, applying adaptive rules")

				// Apply adaptive rules on failure using actual alert data
				coord.applyAdaptiveRules(strategy, investigation.AlertData)
			} else {
				// Merge gathered structured context data - following project guidelines
				coord.mergeContextData(investigation.GatheredContext, contextData)
			}
		}
	}
}

// Additional helper methods would be implemented here for:
// - assessAlertComplexity
// - calculatePriority
// - shouldGatherContext
// - gatherSingleContext
// - applyAdaptiveRules
// - runPerformanceMonitoring
// - runCacheCleanup

// Stop gracefully shuts down the AI orchestration coordinator
func (coord *AIOrchestrationCoordinator) Stop() {
	coord.logger.Info("Stopping AI orchestration coordinator")
	coord.cancelFunc()

	coord.mu.Lock()
	defer coord.mu.Unlock()

	// Mark all active investigations as completed
	for _, investigation := range coord.activeInvestigations {
		investigation.Status = "completed"
	}

	coord.logger.Info("AI orchestration coordinator stopped")
}

// GetInvestigationStatus returns the current status of an investigation
// Business Requirement: BR-EXTERNAL-005 - Investigation state management
func (coord *AIOrchestrationCoordinator) GetInvestigationStatus(investigationID string) (*ActiveInvestigation, error) {
	coord.mu.RLock()
	defer coord.mu.RUnlock()

	investigation, exists := coord.activeInvestigations[investigationID]
	if !exists {
		return nil, fmt.Errorf("investigation not found: %s", investigationID)
	}

	return investigation, nil
}

// GetOrchestrationMetrics returns current orchestration performance metrics
// Business Requirement: BR-EXTERNAL-007 - Context orchestration metrics
// RACE CONDITION FIX: Consistent lock ordering - always acquire performanceMonitor.mu before contextCacheMutex
func (coord *AIOrchestrationCoordinator) GetOrchestrationMetrics() *OverallOrchestrationMetrics {
	// RACE CONDITION FIX: Acquire performanceMonitor.mu first to maintain consistent lock ordering
	coord.performanceMonitor.mu.RLock()
	defer coord.performanceMonitor.mu.RUnlock()

	// Now safely get context cache size with proper lock ordering
	coord.contextCacheMutex.RLock()
	contextCacheSize := len(coord.contextCache)
	coord.contextCacheMutex.RUnlock()

	// Update system uptime
	coord.performanceMonitor.OverallMetrics.SystemUptime = time.Since(coord.performanceMonitor.StartTime)
	coord.performanceMonitor.OverallMetrics.ContextCacheSize = contextCacheSize

	return coord.performanceMonitor.OverallMetrics
}

// runPerformanceMonitoring runs the performance monitoring goroutine
// Business Requirement: BR-HOLMES-009 - Performance monitoring for orchestration
func (coord *AIOrchestrationCoordinator) runPerformanceMonitoring() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-coord.ctx.Done():
			return
		case <-coord.stopChannel:
			return
		case <-ticker.C:
			coord.performanceMonitor.mu.Lock()
			// Update overall metrics periodically
			coord.performanceMonitor.OverallMetrics.SuccessRate = coord.calculateCurrentSuccessRate()
			coord.performanceMonitor.OverallMetrics.AverageResponseTime = coord.calculateAverageResponseTime()
			coord.performanceMonitor.mu.Unlock()
		}
	}
}

// runCacheCleanup runs the cache cleanup goroutine
// Business Requirement: BR-HOLMES-008 - Context cache management
func (coord *AIOrchestrationCoordinator) runCacheCleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-coord.ctx.Done():
			return
		case <-coord.stopChannel:
			return
		case <-ticker.C:
			coord.cleanupExpiredContext()
		}
	}
}

// assessAlertComplexity assesses the complexity of an alert for context gathering strategy
// Phase 2: Implements real business logic using structured AlertData following project guidelines
func (coord *AIOrchestrationCoordinator) assessAlertComplexity(alert *AlertData) ComplexityAssessment {
	if alert == nil {
		return ComplexityAssessment{
			Level:       "low",
			Score:       0.3,
			Factors:     []string{"no_alert_data"},
			Explanation: "No alert data available - minimal complexity",
		}
	}

	score := 0.0
	factors := []string{}

	// Analyze severity - Following project guideline: use structured field values
	switch alert.Severity {
	case "critical":
		score += 0.4
		factors = append(factors, "critical_severity")
	case "high":
		score += 0.3
		factors = append(factors, "high_severity")
	case "medium":
		score += 0.2
		factors = append(factors, "medium_severity")
	case "low":
		score += 0.1
		factors = append(factors, "low_severity")
	}

	// Analyze alert type - Following project guideline: REUSE existing code patterns
	switch alert.Type {
	case "prometheus":
		score += 0.2
		factors = append(factors, "prometheus_metrics")
	case "kubernetes":
		score += 0.3
		factors = append(factors, "kubernetes_platform")
	case "application":
		score += 0.25
		factors = append(factors, "application_layer")
	case "system":
		score += 0.15
		factors = append(factors, "system_level")
	}

	// Additional complexity factors
	if alert.Duration > 30*time.Minute {
		score += 0.1
		factors = append(factors, "persistent_issue")
	}

	if len(alert.Labels) > 5 {
		score += 0.1
		factors = append(factors, "complex_labeling")
	}

	if alert.MetricValue > 0 && alert.Threshold > 0 {
		ratio := alert.MetricValue / alert.Threshold
		if ratio > 3.0 {
			score += 0.15
			factors = append(factors, "severe_threshold_breach")
		} else if ratio > 1.5 {
			score += 0.1
			factors = append(factors, "moderate_threshold_breach")
		}
	}

	// Normalize score to 0-1 range
	if score > 1.0 {
		score = 1.0
	}

	// Determine complexity level
	var level string
	var explanation string
	switch {
	case score >= 0.75:
		level = "high"
		explanation = "High complexity alert requiring comprehensive context gathering"
	case score >= 0.5:
		level = "medium"
		explanation = "Medium complexity alert requiring standard context gathering"
	case score >= 0.25:
		level = "low"
		explanation = "Low complexity alert requiring minimal context gathering"
	default:
		level = "minimal"
		explanation = "Minimal complexity alert with basic context needs"
	}

	return ComplexityAssessment{
		Level:       level,
		Score:       score,
		Factors:     factors,
		Explanation: explanation,
	}
}

// calculatePriority calculates the priority for context gathering based on alert and complexity
// Phase 2: Implements real business logic using structured AlertData following project guidelines
func (coord *AIOrchestrationCoordinator) calculatePriority(alert *AlertData, complexity ComplexityAssessment) int {
	basePriority := 5

	// Adjust based on complexity level - Following project guideline: REUSE existing code patterns
	switch complexity.Level {
	case "high":
		basePriority += 3
	case "medium":
		basePriority += 1
	case "low":
		basePriority -= 1
	case "minimal":
		basePriority -= 2
	}

	// Alert-specific priority adjustments using structured data
	if alert != nil {
		// Severity-based priority boost - Following project guideline: use structured field values
		switch alert.Severity {
		case "critical":
			basePriority += 3
		case "high":
			basePriority += 2
		case "medium":
			basePriority += 1
		}

		// Type-specific adjustments
		switch alert.Type {
		case "kubernetes":
			basePriority += 1 // Kubernetes alerts often need more context
		case "prometheus":
			basePriority += 1 // Metrics alerts benefit from comprehensive context
		}

		// Duration-based urgency
		if alert.Duration > 60*time.Minute {
			basePriority += 2 // Long-running issues need higher priority
		} else if alert.Duration > 15*time.Minute {
			basePriority += 1
		}

		// Threshold breach severity
		if alert.MetricValue > 0 && alert.Threshold > 0 {
			ratio := alert.MetricValue / alert.Threshold
			if ratio > 5.0 {
				basePriority += 2 // Severe threshold breach
			} else if ratio > 2.0 {
				basePriority += 1 // Moderate threshold breach
			}
		}

		// Namespace-based priority (production environments)
		if alert.Namespace == "production" || alert.Namespace == "prod" {
			basePriority += 2
		} else if alert.Namespace == "staging" || alert.Namespace == "stage" {
			basePriority += 1
		}
	}

	// Ensure priority is within bounds
	if basePriority < 1 {
		basePriority = 1
	} else if basePriority > 10 {
		basePriority = 10
	}

	return basePriority
}

// shouldGatherContext determines if context gathering is needed for an alert
// Business Requirement: BR-HOLMES-007 - Context gathering decision logic
// Removed backwards compatibility - now uses structured AlertData and Strategy following project guidelines
func (coord *AIOrchestrationCoordinator) shouldGatherContext(alert *AlertData, strategy *ContextGatheringStrategy) bool {
	// Alert-aware context gathering decision using structured data
	if alert == nil || strategy == nil {
		return false // No context gathering without alert data or strategy
	}

	// Strategy-informed decisions - Following project guideline: use structured field values
	// Check if this alert type matches the strategy's alert type
	if strategy.AlertType != "" && strategy.AlertType != alert.Type {
		coord.logger.WithFields(logrus.Fields{
			"strategy_alert_type": strategy.AlertType,
			"actual_alert_type":   alert.Type,
		}).Debug("Alert type mismatch with strategy")
		return false
	}

	// Priority-based decision using strategy
	if strategy.Priority >= 8 {
		// High priority strategies always gather context
		return true
	} else if strategy.Priority <= 3 {
		// Low priority strategies only gather for critical alerts
		return alert.Severity == "critical"
	}

	// Medium priority strategies use alert-specific logic with strategy complexity
	switch alert.Severity {
	case "critical":
		return true // Always gather for critical alerts regardless of strategy complexity
	case "high":
		// High severity: gather unless strategy complexity is minimal
		return strategy.Complexity != "minimal"
	case "medium":
		// Medium severity: gather based on strategy complexity and alert type
		if strategy.Complexity == "high" || strategy.Complexity == "complex" {
			return true
		}
		// For moderate complexity, gather based on type
		return alert.Type == "kubernetes" || alert.Type == "prometheus"
	case "low":
		// Low severity: only gather for high complexity strategies and specific conditions
		if strategy.Complexity == "high" || strategy.Complexity == "complex" {
			return alert.Type == "kubernetes" && alert.Duration > 30*time.Minute
		}
		return false
	default:
		// Unknown severity: use strategy complexity as guide
		return strategy.Complexity == "high" || strategy.Complexity == "complex"
	}
}

// gatherSingleContext gathers context from a single source
// Removed backwards compatibility - requires AlertData, returns structured ContextData following project guidelines
func (coord *AIOrchestrationCoordinator) gatherSingleContext(ctx context.Context, source string, alert *AlertData) (*contextpkg.ContextData, error) {
	// Check for context cancellation - Following project guideline: proper context usage
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Removed backwards compatibility - AlertData is required (no nil checks)
	if alert == nil {
		return nil, fmt.Errorf("AlertData is required for context gathering - no backwards compatibility")
	}

	coord.logger.WithFields(logrus.Fields{
		"source":     source,
		"alert_type": alert.Type,
		"severity":   alert.Severity,
	}).Debug("Gathering structured context from source")

	// Initialize structured context data - Following project guideline: use structured field values
	contextData := &contextpkg.ContextData{}
	now := time.Now()

	// Source-specific structured context gathering based on alert characteristics
	switch source {
	case "kubernetes":
		contextData.Kubernetes = &contextpkg.KubernetesContext{
			Namespace:    alert.Namespace,
			ResourceType: alert.Type,
			ResourceName: alert.Resource,
			Labels:       alert.Labels,
			Annotations:  alert.Annotations,
			ClusterInfo: &contextpkg.ClusterInfo{
				Environment: alert.Namespace, // Use namespace as environment indicator
			},
			CollectedAt: now,
		}

	case "metrics":
		contextData.Metrics = &contextpkg.MetricsContext{
			Source: alert.Source,
			TimeRange: &types.TimeRange{
				Start: alert.Timestamp,
				End:   now,
			},
			MetricsData: map[string]float64{
				"alert_metric_value": alert.MetricValue,
				"threshold":          alert.Threshold,
			},
			Aggregations: map[string]string{
				"rule_id": alert.RuleID,
			},
			CollectedAt: now,
		}

	case "logs":
		contextData.Logs = &contextpkg.LogsContext{
			Source:    alert.Source,
			TimeRange: &types.TimeRange{Start: alert.Timestamp.Add(-alert.Duration), End: now},
			LogLevel:  alert.Severity,
			LogEntries: []contextpkg.LogEntry{
				{
					Timestamp: alert.Timestamp,
					Level:     alert.Severity,
					Message:   alert.Message,
					Source:    alert.Source,
					Labels:    alert.Labels,
				},
			},
			CollectedAt: now,
		}

	case "events":
		contextData.Events = &contextpkg.EventsContext{
			Source:    alert.Source,
			TimeRange: &types.TimeRange{Start: alert.Timestamp.Add(-alert.Duration), End: now},
			Events: []contextpkg.Event{
				{
					Timestamp: alert.Timestamp,
					Type:      alert.Type,
					Reason:    alert.Severity,
					Message:   alert.Message,
					Source:    alert.Source,
					Labels:    alert.Labels,
				},
			},
			EventTypes:  []string{alert.Type},
			CollectedAt: now,
		}

	case "action-history":
		contextData.ActionHistory = &contextpkg.ActionHistoryContext{
			TimeRange: &types.TimeRange{Start: alert.Timestamp.Add(-alert.Duration), End: now},
			Actions: []contextpkg.HistoryAction{
				{
					ActionID:   fmt.Sprintf("alert_%s", alert.RuleID),
					Timestamp:  alert.Timestamp,
					ActionType: "alert_triggered",
					Success:    true,
					Parameters: map[string]interface{}{
						"severity":     alert.Severity,
						"metric_value": alert.MetricValue,
						"threshold":    alert.Threshold,
					},
				},
			},
			TotalActions: 1,
			CollectedAt:  now,
		}

	case "traces":
		contextData.Traces = &contextpkg.TracesContext{
			Source:     alert.Source,
			TimeRange:  &types.TimeRange{Start: alert.Timestamp.Add(-alert.Duration), End: now},
			TraceCount: 1,
			SpanCount:  10, // Estimated span count
			ErrorRate: func() float64 {
				if alert.Severity == "critical" {
					return 0.5
				} else {
					return 0.1
				}
			}(),
			CollectedAt: now,
		}

	case "network-flows":
		contextData.NetworkFlows = &contextpkg.NetworkFlowsContext{
			Source:      alert.Source,
			TimeRange:   &types.TimeRange{Start: alert.Timestamp.Add(-alert.Duration), End: now},
			FlowCount:   50,                         // Estimated flow count
			Connections: []contextpkg.NetworkFlow{}, // Would be populated with real network flows
			CollectedAt: now,
		}

	case "audit-logs":
		contextData.AuditLogs = &contextpkg.AuditLogsContext{
			Source:    alert.Source,
			TimeRange: &types.TimeRange{Start: alert.Timestamp.Add(-alert.Duration), End: now},
			AuditEvents: []contextpkg.AuditEvent{
				{
					Timestamp: alert.Timestamp,
					User:      "system",
					Action:    "alert_generated",
					Resource:  alert.Resource,
					Result:    "success",
					Labels:    alert.Labels,
				},
			},
			CollectedAt: now,
		}

	default:
		return nil, fmt.Errorf("unsupported context source: %s", source)
	}

	// Simulate some processing time that can be cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(100 * time.Millisecond): // Simulate gathering delay
	}

	coord.logger.WithFields(logrus.Fields{
		"source":     source,
		"alert_type": alert.Type,
	}).Debug("Successfully gathered structured context")

	return contextData, nil
}

// applyAdaptiveRules applies adaptive rules to refine the context gathering strategy
// Removed backwards compatibility - requires AlertData following project guidelines
func (coord *AIOrchestrationCoordinator) applyAdaptiveRules(strategy *ContextGatheringStrategy, alert *AlertData) {
	// Removed backwards compatibility - AlertData is required (no nil checks)
	if alert == nil {
		coord.logger.Error("AlertData is required for adaptive rules - no backwards compatibility")
		return
	}

	coord.logger.WithFields(logrus.Fields{
		"alert_type": alert.Type,
		"severity":   alert.Severity,
	}).Debug("Applying adaptive rules based on alert characteristics")

	// Alert-aware adaptive rule application using structured data
	// Severity-based rule adaptations - Following project guideline: use structured field values
	switch alert.Severity {
	case "critical":
		// Critical alerts need comprehensive context
		strategy.RequiredContexts = append(strategy.RequiredContexts, "metrics", "logs", "events")
		strategy.MaxContextSize += 2048
		strategy.Priority += 2
	case "high":
		// High severity alerts need additional context sources
		strategy.OptionalContexts = append(strategy.OptionalContexts, "traces", "audit-logs")
		strategy.MaxContextSize += 1024
		strategy.Priority += 1
	}

	// Type-specific adaptive rules
	switch alert.Type {
	case "kubernetes":
		// Kubernetes alerts benefit from kubernetes-specific context
		if !contains(strategy.RequiredContexts, "kubernetes") {
			strategy.RequiredContexts = append(strategy.RequiredContexts, "kubernetes")
		}
		strategy.ContextSources = append(strategy.ContextSources, "k8s-api", "kubectl")
	case "prometheus":
		// Prometheus alerts need metrics context
		if !contains(strategy.RequiredContexts, "metrics") {
			strategy.RequiredContexts = append(strategy.RequiredContexts, "metrics")
		}
		strategy.ContextSources = append(strategy.ContextSources, "prometheus", "grafana")
	case "application":
		// Application alerts need logs and traces
		strategy.OptionalContexts = append(strategy.OptionalContexts, "logs", "traces")
		strategy.ContextSources = append(strategy.ContextSources, "application-logs")
	}

	// Duration-based adaptations
	if alert.Duration > 30*time.Minute {
		// Long-running issues may need historical data
		strategy.OptionalContexts = append(strategy.OptionalContexts, "action-history")
		strategy.MaxContextSize += 1024
	}

	// Threshold breach adaptations
	if alert.MetricValue > 0 && alert.Threshold > 0 {
		ratio := alert.MetricValue / alert.Threshold
		if ratio > 2.0 {
			// Severe threshold breaches need comprehensive metrics
			strategy.RequiredContexts = append(strategy.RequiredContexts, "metrics")
			strategy.ContextSources = append(strategy.ContextSources, "detailed-metrics")
		}
	}

	// Namespace-based adaptations
	if alert.Namespace == "production" || alert.Namespace == "prod" {
		// Production alerts need comprehensive context
		strategy.RequiredContexts = append(strategy.RequiredContexts, "audit-logs")
		strategy.MaxContextSize += 1536
	}

	coord.logger.WithFields(logrus.Fields{
		"required_contexts": len(strategy.RequiredContexts),
		"context_sources":   len(strategy.ContextSources),
		"max_context_size":  strategy.MaxContextSize,
	}).Debug("Adaptive rules applied successfully")
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// mergeContextData merges structured context data from different sources
// Following project guideline: use structured field values instead of interface{}
func (coord *AIOrchestrationCoordinator) mergeContextData(target *contextpkg.ContextData, source *contextpkg.ContextData) {
	if source == nil {
		return
	}

	// Merge each context type - only replace if target doesn't have it yet
	if target.Kubernetes == nil && source.Kubernetes != nil {
		target.Kubernetes = source.Kubernetes
	}
	if target.Metrics == nil && source.Metrics != nil {
		target.Metrics = source.Metrics
	}
	if target.Logs == nil && source.Logs != nil {
		target.Logs = source.Logs
	}
	if target.ActionHistory == nil && source.ActionHistory != nil {
		target.ActionHistory = source.ActionHistory
	}
	if target.Events == nil && source.Events != nil {
		target.Events = source.Events
	}
	if target.Traces == nil && source.Traces != nil {
		target.Traces = source.Traces
	}
	if target.NetworkFlows == nil && source.NetworkFlows != nil {
		target.NetworkFlows = source.NetworkFlows
	}
	if target.AuditLogs == nil && source.AuditLogs != nil {
		target.AuditLogs = source.AuditLogs
	}
}

// countGatheredContexts counts how many context types have been gathered
// Following project guideline: use structured field values instead of interface{}
func (coord *AIOrchestrationCoordinator) countGatheredContexts(contextData *contextpkg.ContextData) int {
	if contextData == nil {
		return 0
	}

	count := 0
	if contextData.Kubernetes != nil {
		count++
	}
	if contextData.Metrics != nil {
		count++
	}
	if contextData.Logs != nil {
		count++
	}
	if contextData.ActionHistory != nil {
		count++
	}
	if contextData.Events != nil {
		count++
	}
	if contextData.Traces != nil {
		count++
	}
	if contextData.NetworkFlows != nil {
		count++
	}
	if contextData.AuditLogs != nil {
		count++
	}
	return count
}

// Helper methods for performance monitoring

func (coord *AIOrchestrationCoordinator) calculateCurrentSuccessRate() float64 {
	coord.performanceMonitor.mu.RLock()
	defer coord.performanceMonitor.mu.RUnlock()

	totalInvestigations := len(coord.performanceMonitor.InvestigationMetrics)
	if totalInvestigations == 0 {
		return 1.0
	}

	successCount := 0
	for _, metrics := range coord.performanceMonitor.InvestigationMetrics {
		if metrics.Success {
			successCount++
		}
	}

	return float64(successCount) / float64(totalInvestigations)
}

func (coord *AIOrchestrationCoordinator) calculateAverageResponseTime() time.Duration {
	coord.performanceMonitor.mu.RLock()
	defer coord.performanceMonitor.mu.RUnlock()

	totalInvestigations := len(coord.performanceMonitor.InvestigationMetrics)
	if totalInvestigations == 0 {
		return 0
	}

	totalTime := time.Duration(0)
	for _, metrics := range coord.performanceMonitor.InvestigationMetrics {
		totalTime += metrics.ResponseTime
	}

	return totalTime / time.Duration(totalInvestigations)
}

func (coord *AIOrchestrationCoordinator) cleanupExpiredContext() {
	coord.contextCacheMutex.Lock()
	defer coord.contextCacheMutex.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	for key, cached := range coord.contextCache {
		if now.Sub(cached.CachedAt) > 30*time.Minute {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(coord.contextCache, key)
	}

	if len(expiredKeys) > 0 {
		coord.logger.WithField("cleaned_entries", len(expiredKeys)).Debug("Cleaned up expired context cache entries")
	}
}

// RACE CONDITION FIX: Atomic status management methods for ActiveInvestigation
// Business Requirement: BR-EXTERNAL-005 - Investigation state management

// SetStatusAtomic atomically updates the investigation status
func (ai *ActiveInvestigation) SetStatusAtomic(newStatus string) bool {
	statusCode := statusStringToCode(newStatus)
	if statusCode < 0 {
		return false // Invalid status
	}

	now := time.Now()
	newTime := now.Unix()

	// Simple atomic store (not compare-and-swap for SetStatusAtomic)
	atomic.StoreInt64(&ai.atomicStatus, statusCode)
	atomic.StoreInt64(&ai.atomicUpdateTime, newTime)

	// Update string fields with proper locking
	ai.statusMutex.Lock()
	ai.Status = newStatus
	ai.LastActivity = now
	ai.statusMutex.Unlock()

	return true
}

// GetStatusAtomic atomically reads the investigation status
func (ai *ActiveInvestigation) GetStatusAtomic() (string, time.Time) {
	statusCode := atomic.LoadInt64(&ai.atomicStatus)
	timestamp := atomic.LoadInt64(&ai.atomicUpdateTime)

	statusString := statusCodeToString(statusCode)
	return statusString, time.Unix(timestamp, 0)
}

// GetStatusSafe safely reads the string status and timestamp with proper locking
func (ai *ActiveInvestigation) GetStatusSafe() (string, time.Time) {
	ai.statusMutex.RLock()
	defer ai.statusMutex.RUnlock()
	return ai.Status, ai.LastActivity
}

// TransitionStatusAtomic attempts to transition from expected status to new status atomically
func (ai *ActiveInvestigation) TransitionStatusAtomic(expectedStatus, newStatus string) bool {
	expectedCode := statusStringToCode(expectedStatus)
	newCode := statusStringToCode(newStatus)

	if expectedCode < 0 || newCode < 0 {
		return false // Invalid status
	}

	// Check if transition is allowed
	if !isValidStatusTransition(expectedCode, newCode) {
		return false
	}

	now := time.Now()
	newTime := now.Unix()

	// Atomically transition if current status matches expected
	if atomic.CompareAndSwapInt64(&ai.atomicStatus, expectedCode, newCode) {
		atomic.StoreInt64(&ai.atomicUpdateTime, newTime)

		// Update string fields with proper locking
		ai.statusMutex.Lock()
		ai.Status = newStatus
		ai.LastActivity = now
		ai.statusMutex.Unlock()

		return true
	}
	return false
}

// InitializeStatusAtomic initializes the atomic status fields
func (ai *ActiveInvestigation) InitializeStatusAtomic() {
	statusCode := statusStringToCode(ai.Status)
	if statusCode < 0 {
		statusCode = 0 // Default to active
		ai.Status = "active"
	}

	timestamp := ai.LastActivity.Unix()
	if timestamp <= 0 {
		timestamp = time.Now().Unix()
		ai.LastActivity = time.Unix(timestamp, 0)
	}

	atomic.StoreInt64(&ai.atomicStatus, statusCode)
	atomic.StoreInt64(&ai.atomicUpdateTime, timestamp)
}

// Helper functions for status code conversion
func statusStringToCode(status string) int64 {
	switch status {
	case "active":
		return 0
	case "paused":
		return 1
	case "completed":
		return 2
	case "failed":
		return 3
	default:
		return -1 // Invalid status
	}
}

func statusCodeToString(code int64) string {
	switch code {
	case 0:
		return "active"
	case 1:
		return "paused"
	case 2:
		return "completed"
	case 3:
		return "failed"
	default:
		return "unknown"
	}
}

// isValidStatusTransition checks if a status transition is allowed
func isValidStatusTransition(fromCode, toCode int64) bool {
	// Transition rules for investigation status
	switch fromCode {
	case 0: // active
		return toCode == 1 || toCode == 2 || toCode == 3 // can go to paused, completed, or failed
	case 1: // paused
		return toCode == 0 || toCode == 2 || toCode == 3 // can go to active, completed, or failed
	case 2: // completed
		return false // completed is final state
	case 3: // failed
		return false // failed is final state
	default:
		return false // invalid status
	}
}
