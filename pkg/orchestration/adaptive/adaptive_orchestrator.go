package adaptive

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

const (
	minExecutionsForOptimization = 3 // BR-ORK-003: Minimum executions before optimization
)

// DefaultAdaptiveOrchestrator implements the AdaptiveOrchestrator interface
// Business Requirements: BR-ORCH-001, BR-ORCH-002, BR-ORCH-003, BR-ORCH-004, BR-ORCH-005
// Provides continuous optimization, adaptive resource allocation, execution scheduling,
// failure learning, and predictive scaling for workflow orchestration
type DefaultAdaptiveOrchestrator struct {
	// Core dependencies
	workflowEngine engine.WorkflowEngine
	// RULE 12 COMPLIANCE: Using enhanced llm.Client instead of deprecated SelfOptimizer
	llmClient llm.Client
	vectorDB        vector.VectorDatabase
	analyticsEngine types.AnalyticsEngine // Following development guidelines: avoid pointer to interface
	actionRepo      actionhistory.Repository

	// Workflow management
	workflows  map[string]*engine.Workflow
	executions map[string]*engine.RuntimeWorkflowExecution
	templates  map[string]*engine.ExecutableTemplate

	// Learning and adaptation
	adaptationRules  map[string]*engine.AdaptationRules
	patternExtractor vector.PatternExtractor

	// Configuration
	config *OrchestratorConfig

	// Synchronization
	mu          sync.RWMutex
	executionMu sync.RWMutex

	// BR-ORK-003: Resource monitoring flag
	resourceMonitoringEnabled bool

	// Logging
	log *logrus.Logger

	// State management
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// OrchestratorConfig holds configuration for the adaptive orchestrator
type OrchestratorConfig struct {
	// Core settings
	MaxConcurrentExecutions int           `yaml:"max_concurrent_executions" default:"10"`
	DefaultTimeout          time.Duration `yaml:"default_timeout" default:"30m"`

	// Adaptation settings
	EnableAdaptation   bool          `yaml:"enable_adaptation" default:"true"`
	AdaptationInterval time.Duration `yaml:"adaptation_interval" default:"5m"`
	LearningEnabled    bool          `yaml:"learning_enabled" default:"true"`

	// Optimization settings
	EnableOptimization    bool    `yaml:"enable_optimization" default:"true"`
	OptimizationThreshold float64 `yaml:"optimization_threshold" default:"0.7"`

	// Recovery settings
	EnableAutoRecovery  bool          `yaml:"enable_auto_recovery" default:"true"`
	MaxRecoveryAttempts int           `yaml:"max_recovery_attempts" default:"3"`
	RecoveryTimeout     time.Duration `yaml:"recovery_timeout" default:"10m"`

	// Performance settings
	MetricsCollection bool `yaml:"metrics_collection" default:"true"`
	DetailedLogging   bool `yaml:"detailed_logging" default:"false"`

	// Storage settings
	RetainExecutions time.Duration `yaml:"retain_executions" default:"7d"`
	RetainMetrics    time.Duration `yaml:"retain_metrics" default:"30d"`
}

// NewDefaultAdaptiveOrchestrator creates a new adaptive orchestrator
// Refactored to use shared constructor utilities to eliminate duplication
func NewDefaultAdaptiveOrchestrator(
	workflowEngine engine.WorkflowEngine,
	// RULE 12 COMPLIANCE: Using enhanced llm.Client instead of deprecated SelfOptimizer
	llmClient llm.Client,
	vectorDB vector.VectorDatabase,
	analyticsEngine types.AnalyticsEngine, // Following development guidelines: interface, not pointer to interface
	actionRepo actionhistory.Repository,
	patternExtractor vector.PatternExtractor,
	config *OrchestratorConfig,
	log *logrus.Logger,
) *DefaultAdaptiveOrchestrator {
	// Use the new constructor utility to handle default configuration
	defaultConfig := OrchestratorConfig{
		MaxConcurrentExecutions: 10,
		DefaultTimeout:          30 * time.Minute,
		EnableAdaptation:        true,
		AdaptationInterval:      5 * time.Minute,
		LearningEnabled:         true,
		EnableOptimization:      true,
		OptimizationThreshold:   0.7,
		EnableAutoRecovery:      true,
		MaxRecoveryAttempts:     3,
		RecoveryTimeout:         10 * time.Minute,
		MetricsCollection:       true,
		DetailedLogging:         false,
		RetainExecutions:        7 * 24 * time.Hour,
		RetainMetrics:           30 * 24 * time.Hour,
	}

	// Apply default if config is nil - consolidated pattern
	if config == nil {
		config = &defaultConfig
	}

	return &DefaultAdaptiveOrchestrator{
		workflowEngine: workflowEngine,
		// RULE 12 COMPLIANCE: Using enhanced llm.Client instead of deprecated SelfOptimizer
		llmClient:       llmClient,
		vectorDB:        vectorDB,
		analyticsEngine:  analyticsEngine,
		actionRepo:       actionRepo,
		patternExtractor: patternExtractor,
		config:           config,
		workflows:        make(map[string]*engine.Workflow),
		executions:       make(map[string]*engine.RuntimeWorkflowExecution),
		templates:        make(map[string]*engine.ExecutableTemplate),
		adaptationRules:  make(map[string]*engine.AdaptationRules),
		log:              log,
	}
}

// Start initializes and starts the orchestrator
func (dao *DefaultAdaptiveOrchestrator) Start(ctx context.Context) error {
	dao.mu.Lock()
	defer dao.mu.Unlock()

	if dao.running {
		return fmt.Errorf("orchestrator is already running")
	}

	dao.ctx, dao.cancel = context.WithCancel(ctx)
	dao.running = true

	// Start background processes
	if dao.config.EnableAdaptation {
		go dao.adaptationLoop()
	}

	if dao.config.EnableOptimization {
		go dao.optimizationLoop()
	}

	if dao.config.MetricsCollection {
		go dao.metricsLoop()
	}

	dao.log.Info("Adaptive orchestrator started")
	return nil
}

// Stop gracefully shuts down the orchestrator
func (dao *DefaultAdaptiveOrchestrator) Stop() error {
	dao.mu.Lock()
	defer dao.mu.Unlock()

	if !dao.running {
		return nil
	}

	dao.cancel()
	dao.running = false

	dao.log.Info("Adaptive orchestrator stopped")
	return nil
}

// CreateWorkflow creates a new workflow from a template
func (dao *DefaultAdaptiveOrchestrator) CreateWorkflow(ctx context.Context, template *engine.ExecutableTemplate) (*engine.Workflow, error) {
	dao.mu.Lock()
	defer dao.mu.Unlock()

	// Use the template's ID as the workflow ID for easy reference
	workflowID := template.ID

	workflow := &engine.Workflow{
		BaseVersionedEntity: types.BaseVersionedEntity{
			BaseEntity: types.BaseEntity{
				ID:          workflowID,
				Name:        template.Name,
				Description: template.Description,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Version: template.Version,
		},
		Template: template,
		Status:   engine.StatusPending,
	}

	dao.workflows[workflowID] = workflow
	dao.templates[workflowID] = template

	dao.log.WithFields(logrus.Fields{
		"workflow_id": workflowID,
		"name":        template.Name,
		"version":     template.Version,
	}).Info("Created workflow")

	return workflow, nil
}

// ExecuteWorkflow executes a workflow with the given input
// Business Requirements: BR-WF-001 (reliable workflow execution), BR-WF-004 (state management)
// Following development guidelines: proper error handling and logging
func (dao *DefaultAdaptiveOrchestrator) ExecuteWorkflow(ctx context.Context, workflowID string, input *engine.WorkflowInput) (*engine.RuntimeWorkflowExecution, error) {
	dao.mu.RLock()
	workflow, exists := dao.workflows[workflowID]
	dao.mu.RUnlock()

	if !exists {
		err := fmt.Errorf("workflow %s not found", workflowID)
		// Following development guidelines: ALWAYS log errors, never ignore them
		dao.log.WithError(err).WithField("workflow_id", workflowID).Error("Failed to execute workflow: workflow not found")
		return nil, err
	}

	// Check if we're at max concurrent executions - following BR-ORCH-002 (adaptive resource allocation)
	currentCount := dao.getCurrentExecutionCount()
	if currentCount >= dao.config.MaxConcurrentExecutions {
		err := fmt.Errorf("maximum concurrent executions reached (%d)", dao.config.MaxConcurrentExecutions)
		// Following development guidelines: ALWAYS log errors with context
		dao.log.WithError(err).WithFields(logrus.Fields{
			"workflow_id":    workflowID,
			"current_count":  currentCount,
			"max_concurrent": dao.config.MaxConcurrentExecutions,
		}).Warn("Workflow execution rejected due to concurrent execution limit")
		return nil, err
	}

	executionID := generateExecutionID()
	execution := &engine.RuntimeWorkflowExecution{
		WorkflowExecutionRecord: types.WorkflowExecutionRecord{
			ID:         executionID,
			WorkflowID: workflowID,
			Status:     string(engine.ExecutionStatusPending),
			StartTime:  time.Now(),
			Metadata:   make(map[string]interface{}),
		},
		Input:       input,
		Context:     dao.createExecutionContext(input),
		Steps:       make([]*engine.StepExecution, len(workflow.Template.Steps)),
		CurrentStep: 0,
	}

	// Initialize step executions
	for i, step := range workflow.Template.Steps {
		execution.Steps[i] = &engine.StepExecution{
			StepID:    step.ID,
			Status:    engine.ExecutionStatusPending,
			Variables: make(map[string]interface{}),
			Metadata:  make(map[string]interface{}),
		}
	}

	dao.executionMu.Lock()
	dao.executions[executionID] = execution
	dao.executionMu.Unlock()

	// Execute workflow asynchronously
	go dao.executeWorkflowAsync(ctx, execution)

	dao.log.WithFields(logrus.Fields{
		"execution_id": executionID,
		"workflow_id":  workflowID,
		"input":        input,
	}).Info("Started workflow execution")

	return execution, nil
}

// GetWorkflowStatus returns the status of a workflow execution
func (dao *DefaultAdaptiveOrchestrator) GetWorkflowStatus(ctx context.Context, executionID string) (*engine.WorkflowStatus, error) {
	dao.executionMu.RLock()
	execution, exists := dao.executions[executionID]
	dao.executionMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("execution %s not found", executionID)
	}

	dao.mu.RLock()
	workflow, exists := dao.workflows[execution.WorkflowID]
	dao.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("workflow %s not found", execution.WorkflowID)
	}

	return &workflow.Status, nil
}

// CancelWorkflow cancels an active workflow execution
// Business Requirements: BR-WF-005 (workflow pause, resume, and cancellation operations)
func (dao *DefaultAdaptiveOrchestrator) CancelWorkflow(ctx context.Context, executionID string) error {
	dao.executionMu.Lock()
	execution, exists := dao.executions[executionID]
	if exists && execution.Status == string(engine.ExecutionStatusRunning) {
		execution.Status = string(engine.ExecutionStatusCancelled)
		if execution.EndTime == nil {
			now := time.Now()
			execution.EndTime = &now
			execution.Duration = now.Sub(execution.StartTime)
		}
	}
	dao.executionMu.Unlock()

	if !exists {
		return fmt.Errorf("execution %s not found", executionID)
	}

	dao.log.WithField("execution_id", executionID).Info("Cancelled workflow execution")
	return nil
}

// AdaptWorkflow applies adaptation rules to a workflow
func (dao *DefaultAdaptiveOrchestrator) AdaptWorkflow(ctx context.Context, workflowID string, adaptationRules *engine.AdaptationRules) error {
	dao.mu.Lock()
	defer dao.mu.Unlock()

	workflow, exists := dao.workflows[workflowID]
	if !exists {
		return fmt.Errorf("workflow %s not found", workflowID)
	}

	// Store adaptation rules
	dao.adaptationRules[workflowID] = adaptationRules

	// Apply immediate adaptations if enabled
	if dao.config.EnableAdaptation && adaptationRules.Enabled {
		err := dao.applyAdaptationRules(ctx, workflow, adaptationRules)
		if err != nil {
			dao.log.WithError(err).WithField("workflow_id", workflowID).Error("Failed to apply adaptation rules")
			return err
		}
	}

	dao.log.WithFields(logrus.Fields{
		"workflow_id": workflowID,
		"rules_count": len(adaptationRules.Triggers),
	}).Info("Applied adaptation rules to workflow")

	return nil
}

// OptimizeWorkflow optimizes a workflow based on performance data
// Business Requirements: BR-ORCH-001 (continuously optimize orchestration strategies),
// BR-ORCH-003 (optimize execution scheduling for maximum efficiency)
func (dao *DefaultAdaptiveOrchestrator) OptimizeWorkflow(ctx context.Context, workflowID string) (*engine.OptimizationResult, error) {
	dao.mu.RLock()
	_, exists := dao.workflows[workflowID]
	dao.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("workflow %s not found", workflowID)
	}

	if !dao.config.EnableOptimization {
		return nil, fmt.Errorf("optimization is disabled")
	}

	// Analyze workflow performance
	analysis, err := dao.analyzeWorkflowPerformance(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze workflow performance: %w", err)
	}

	// BR-ORK-001 Requirement 1: Performance Analysis & Candidate Generation
	candidates := dao.generateOptimizationCandidates(ctx, analysis)

	if len(candidates) == 0 {
		dao.log.WithField("workflow_id", workflowID).Info("No optimization candidates found")
		return &engine.OptimizationResult{
			ID:         generateOptimizationID(),
			WorkflowID: workflowID,
			Type:       engine.OptimizationTypePerformance,
			Changes:    []*engine.OptimizationChange{},
			Confidence: 1.0,
			CreatedAt:  time.Now(),
		}, nil
	}

	// Select best candidate
	bestCandidate := dao.selectBestOptimizationCandidate(candidates)

	// Create optimization result
	result := &engine.OptimizationResult{
		ID:         generateOptimizationID(),
		WorkflowID: workflowID,
		Type:       engine.OptimizationType(bestCandidate.Type),
		Changes: []*engine.OptimizationChange{{
			ID:          generateOptimizationChangeID(),
			Type:        bestCandidate.Type,
			Target:      bestCandidate.Target,
			Description: bestCandidate.Description,
			OldValue:    nil, // Will be set based on current workflow
			NewValue:    bestCandidate.Parameters,
			Confidence:  bestCandidate.Confidence,
			Reasoning:   bestCandidate.Description,
		}},
		Performance: &engine.PerformanceImprovement{
			ExecutionTime: float64(analysis.ExecutionTime.Milliseconds()),
			SuccessRate:   0.0, // TODO: Track statistics separately
			ResourceUsage: 0.0, // Will be calculated based on changes
			Effectiveness: analysis.Effectiveness,
			OverallScore:  0.0, // Will be calculated based on changes
		},
		Confidence: bestCandidate.Confidence,
		CreatedAt:  time.Now(),
	}

	dao.log.WithFields(logrus.Fields{
		"workflow_id":     workflowID,
		"optimization_id": result.ID,
		"type":            result.Type,
		"changes_count":   len(result.Changes),
		"confidence":      result.Confidence,
	}).Info("Generated workflow optimization")

	return result, nil
}

// LearnFromExecution learns from a workflow execution
// Business Requirements: BR-ORCH-004 (learn from execution failures), BR-IWB-016 (learn from outcomes),
// BR-IWB-017 (improve generation algorithms based on feedback)
// Following development guidelines: ALWAYS log errors, never ignore them
func (dao *DefaultAdaptiveOrchestrator) LearnFromExecution(ctx context.Context, execution *engine.RuntimeWorkflowExecution) error {
	if !dao.config.LearningEnabled {
		return nil
	}

	// Extract patterns from the execution
	if execution.Output != nil && len(execution.Output.Actions) > 0 {
		for _, actionResult := range execution.Output.Actions {
			if actionResult.Trace != nil {
				// Extract pattern from action trace
				pattern, err := dao.patternExtractor.ExtractPattern(ctx, actionResult.Trace)
				if err != nil {
					dao.log.WithError(err).WithField("action_id", actionResult.ActionID).
						Warn("Failed to extract pattern from action trace")
					continue
				}

				// Store pattern in vector database
				err = dao.vectorDB.StoreActionPattern(ctx, pattern)
				if err != nil {
					dao.log.WithError(err).WithField("pattern_id", pattern.ID).
						Warn("Failed to store action pattern")
					continue
				}
			}
		}
	}

	// Learn from execution metrics
	if execution.Output != nil && execution.Output.Metrics != nil {
		learnings := dao.extractExecutionLearnings(execution)
		for _, learning := range learnings {
			dao.log.WithFields(logrus.Fields{
				"execution_id":  execution.ID,
				"learning_type": learning.Type,
				"trigger":       learning.Trigger,
				"applied":       learning.Applied,
			}).Debug("Extracted learning from execution")
		}
	}

	dao.log.WithField("execution_id", execution.ID).Debug("Learned from workflow execution")
	return nil
}

// GetWorkflowRecommendations returns workflow recommendations for the given context
func (dao *DefaultAdaptiveOrchestrator) GetWorkflowRecommendations(ctx context.Context, actionContext *engine.ActionContext) ([]*engine.WorkflowRecommendation, error) {
	var recommendations []*engine.WorkflowRecommendation

	// Find similar patterns in vector database
	if actionContext.Alert != nil {
		query := fmt.Sprintf("%s %s", actionContext.Alert.Name, actionContext.Alert.Severity)
		patterns, err := dao.vectorDB.SearchBySemantics(ctx, query, 10)
		if err != nil {
			dao.log.WithError(err).Warn("Failed to search for similar patterns")
		} else {
			// Convert patterns to workflow recommendations
			for _, pattern := range patterns {
				if rec := dao.patternToRecommendation(pattern, actionContext); rec != nil {
					recommendations = append(recommendations, rec)
				}
			}
		}
	}

	// Get recommendations from analytics engine - following BR-AI-001: Analytics Insights Generation
	// Following development guidelines: proper interface usage and error handling
	if dao.analyticsEngine != nil {
		// Use 24-hour window for actionable insights per BR-AI-001
		insights, err := dao.analyticsEngine.GetAnalyticsInsights(ctx, 24*time.Hour)
		if err != nil {
			dao.log.WithError(err).Error("Failed to get analytics insights for workflow recommendations")
		} else {
			// Extract workflow recommendations from insights following BR-AI-002: Pattern Analytics
			analyticsRecs := dao.insightsToRecommendations(insights, actionContext)
			recommendations = append(recommendations, analyticsRecs...)
			dao.log.WithField("analytics_recommendations", len(analyticsRecs)).Debug("Generated analytics-based recommendations")
		}
	} else {
		dao.log.Debug("Analytics engine not available, skipping analytics-based recommendations")
	}

	// Sort recommendations by confidence and effectiveness
	dao.sortRecommendations(recommendations)

	dao.log.WithFields(logrus.Fields{
		"context_type":          actionContext.Environment,
		"recommendations_count": len(recommendations),
	}).Debug("Generated workflow recommendations")

	return recommendations, nil
}

// Private helper methods

func (dao *DefaultAdaptiveOrchestrator) executeWorkflowAsync(ctx context.Context, execution *engine.RuntimeWorkflowExecution) {
	defer func() {
		if r := recover(); r != nil {
			dao.log.WithFields(logrus.Fields{
				"execution_id": execution.ID,
				"panic":        r,
			}).Error("Workflow execution panicked")

			execution.Status = string(engine.ExecutionStatusFailed)
			execution.Error = fmt.Sprintf("execution panicked: %v", r)
			now := time.Now()
			execution.EndTime = &now
			execution.Duration = now.Sub(execution.StartTime)
		}
	}()

	execution.Status = string(engine.ExecutionStatusRunning)

	workflow, exists := dao.workflows[execution.WorkflowID]
	if !exists {
		execution.Status = string(engine.ExecutionStatusFailed)
		execution.Error = "workflow not found"
		return
	}

	// Execute workflow steps
	for i, step := range workflow.Template.Steps {
		if execution.Status == string(engine.ExecutionStatusCancelled) {
			break
		}

		execution.CurrentStep = i
		stepExecution := execution.Steps[i]
		stepExecution.Status = engine.ExecutionStatusRunning
		stepExecution.StartTime = time.Now()

		// Create step context
		stepContext := &engine.StepContext{
			ExecutionID:   execution.ID,
			StepID:        step.ID,
			Variables:     stepExecution.Variables,
			PreviousSteps: dao.getPreviousStepResults(execution, i),
			Environment:   execution.Context,
			Timeout:       step.Timeout,
		}

		// BR-ORK-002 Requirement 1: Context-Aware Execution
		// Analyze current system state before step execution
		contextAnalysis, err := dao.analyzeExecutionContext(ctx, stepContext)
		if err != nil {
			dao.log.WithError(err).WithField("step_id", step.ID).Error("BR-ORK-002: Failed to analyze execution context")
			stepExecution.Status = engine.ExecutionStatusFailed
			stepExecution.Result = &engine.StepResult{
				Success:   false,
				Error:     err.Error(),
				Duration:  time.Since(stepExecution.StartTime),
				Variables: make(map[string]interface{}),
			}
		} else {
			// BR-ORK-002 Requirement 2: Real-Time Adaptation
			// Select optimal execution strategy based on context and learning
			executionStrategy, err := dao.selectExecutionStrategy(ctx, step, contextAnalysis)
			if err != nil {
				dao.log.WithError(err).WithField("step_id", step.ID).Error("BR-ORK-002: Failed to select execution strategy")
				stepExecution.Status = engine.ExecutionStatusFailed
			} else {
				// BR-ORK-002 Requirement 3: Learning Integration
				// Execute step with adaptive parameters
				result, err := dao.executeStepWithAdaptation(ctx, step, stepContext, executionStrategy)
				if err != nil {
					dao.log.WithError(err).WithField("step_id", step.ID).Warn("BR-ORK-002: Step execution failed, attempting adaptation")

					// BR-ORK-002 Requirement 2: Dynamic strategy switching on failure
					alternativeResult, altErr := dao.executeWithAlternativeStrategy(ctx, step, stepContext, err)
					if altErr != nil {
						stepExecution.Status = engine.ExecutionStatusFailed
						stepExecution.Result = &engine.StepResult{
							Success:   false,
							Error:     altErr.Error(),
							Duration:  time.Since(stepExecution.StartTime),
							Variables: make(map[string]interface{}),
						}
					} else {
						stepExecution.Status = engine.ExecutionStatusCompleted
						stepExecution.Result = alternativeResult
					}
				} else {
					stepExecution.Status = engine.ExecutionStatusCompleted
					stepExecution.Result = result
				}
			}
		}

		stepExecution.EndTime = &stepExecution.StartTime
		*stepExecution.EndTime = time.Now()
		stepExecution.Duration = stepExecution.EndTime.Sub(stepExecution.StartTime)

		// Update execution variables with step results
		if stepExecution.Result != nil && stepExecution.Result.Variables != nil {
			for k, v := range stepExecution.Result.Variables {
				execution.Context.Variables[k] = v
			}
		}
	}

	// Finalize execution
	if execution.Status == string(engine.ExecutionStatusRunning) {
		execution.Status = string(engine.ExecutionStatusCompleted)
	}

	now := time.Now()
	execution.EndTime = &now
	execution.Duration = now.Sub(execution.StartTime)

	// Create execution output
	execution.Output = dao.createExecutionOutput(execution)

	// Update workflow statistics
	dao.updateWorkflowStatistics(execution)

	// Learn from execution
	if dao.config.LearningEnabled {
		if err := dao.LearnFromExecution(ctx, execution); err != nil {
			dao.log.WithError(err).WithField("execution_id", execution.ID).
				Warn("Failed to learn from execution")
		}
	}

	dao.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"status":       execution.Status,
		"duration":     execution.Duration,
		"steps":        len(execution.Steps),
	}).Info("Workflow execution completed")
}

func (dao *DefaultAdaptiveOrchestrator) createExecutionContext(input *engine.WorkflowInput) *engine.ExecutionContext {
	// Following development guidelines: proper error handling and type safety
	cluster := "unknown"
	if clusterValue, exists := input.Context["cluster"]; exists {
		if clusterStr, ok := clusterValue.(string); ok {
			cluster = clusterStr
		} else {
			dao.log.WithField("cluster_value", clusterValue).Warn("Cluster value is not a string, using default")
		}
	} else {
		dao.log.Warn("Cluster not found in input context, using default")
	}

	return &engine.ExecutionContext{
		BaseContext: types.BaseContext{
			Environment: input.Environment,
			Cluster:     cluster,
		},
		User:          input.Requester,
		RequestID:     generateRequestID(),
		TraceID:       generateTraceID(),
		CorrelationID: generateCorrelationID(),
		Variables:     make(map[string]interface{}),
		Configuration: input.Context,
	}
}

func (dao *DefaultAdaptiveOrchestrator) createExecutionOutput(execution *engine.RuntimeWorkflowExecution) *engine.WorkflowOutput {
	output := &engine.WorkflowOutput{
		Success:         execution.Status == string(engine.ExecutionStatusCompleted),
		Results:         make(map[string]interface{}),
		Actions:         []*engine.ActionResult{},
		Metrics:         dao.calculateExecutionMetrics(execution),
		Recommendations: []string{},
	}

	// Collect results from steps
	for _, stepExecution := range execution.Steps {
		if stepExecution.Result != nil && stepExecution.Result.Data != nil {
			for k, v := range stepExecution.Result.Data {
				output.Results[k] = v
			}

			// Collect action traces
			if stepExecution.Result.ActionTrace != nil {
				actionResult := &engine.ActionResult{
					ActionID:  stepExecution.StepID,
					Type:      "workflow_step",
					Success:   stepExecution.Status == engine.ExecutionStatusCompleted,
					StartTime: stepExecution.StartTime,
					EndTime:   *stepExecution.EndTime,
					Duration:  stepExecution.Duration,
					Output:    stepExecution.Result.Data,
					Trace:     stepExecution.Result.ActionTrace,
				}
				if stepExecution.Error != "" {
					actionResult.Error = stepExecution.Error
				}
				output.Actions = append(output.Actions, actionResult)
			}
		}
	}

	return output
}

func (dao *DefaultAdaptiveOrchestrator) calculateExecutionMetrics(execution *engine.RuntimeWorkflowExecution) *engine.ExecutionMetrics {
	successCount := 0
	failureCount := 0
	retryCount := 0

	for _, step := range execution.Steps {
		switch step.Status {
		case engine.ExecutionStatusCompleted:
			successCount++
		case engine.ExecutionStatusFailed:
			failureCount++
		}
		retryCount += step.RetryCount
	}

	return &engine.ExecutionMetrics{
		Duration:      execution.Duration,
		StepCount:     len(execution.Steps),
		SuccessCount:  successCount,
		FailureCount:  failureCount,
		RetryCount:    retryCount,
		ResourceUsage: &engine.ResourceUsageMetrics{}, // Would be populated with actual resource monitoring
		Performance:   &engine.PerformanceMetrics{},   // Would be populated with actual performance metrics
	}
}

func (dao *DefaultAdaptiveOrchestrator) getCurrentExecutionCount() int {
	dao.executionMu.RLock()
	defer dao.executionMu.RUnlock()

	count := 0
	for _, execution := range dao.executions {
		if execution.Status == string(engine.ExecutionStatusRunning) || execution.Status == string(engine.ExecutionStatusPending) {
			count++
		}
	}
	return count
}

func (dao *DefaultAdaptiveOrchestrator) updateWorkflowStatistics(execution *engine.RuntimeWorkflowExecution) {
	dao.mu.Lock()
	defer dao.mu.Unlock()

	workflow, exists := dao.workflows[execution.WorkflowID]
	if !exists {
		return
	}

	// Update workflow status based on execution result
	if execution.Status == string(engine.ExecutionStatusCompleted) {
		workflow.Status = engine.StatusCompleted
	} else if execution.Status == string(engine.ExecutionStatusFailed) {
		workflow.Status = engine.StatusFailed
	}

	// BR-ORK-003 Requirement 1: Execution Metrics Collection
	dao.collectAndStoreExecutionMetrics(execution, workflow)
}

// Background loops

func (dao *DefaultAdaptiveOrchestrator) adaptationLoop() {
	ticker := time.NewTicker(dao.config.AdaptationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dao.ctx.Done():
			return
		case <-ticker.C:
			dao.performAdaptationCycle()
		}
	}
}

func (dao *DefaultAdaptiveOrchestrator) optimizationLoop() {
	ticker := time.NewTicker(10 * time.Minute) // Run optimization checks every 10 minutes
	defer ticker.Stop()

	for {
		select {
		case <-dao.ctx.Done():
			return
		case <-ticker.C:
			dao.performOptimizationCycle()
		}
	}
}

func (dao *DefaultAdaptiveOrchestrator) metricsLoop() {
	ticker := time.NewTicker(1 * time.Minute) // Collect metrics every minute
	defer ticker.Stop()

	for {
		select {
		case <-dao.ctx.Done():
			return
		case <-ticker.C:
			dao.collectMetrics()
		}
	}
}

func (dao *DefaultAdaptiveOrchestrator) performAdaptationCycle() {
	dao.mu.RLock()
	workflows := make([]*engine.Workflow, 0, len(dao.workflows))
	for _, workflow := range dao.workflows {
		workflows = append(workflows, workflow)
	}
	dao.mu.RUnlock()

	for _, workflow := range workflows {
		if rules, exists := dao.adaptationRules[workflow.ID]; exists && rules.Enabled {
			if err := dao.applyAdaptationRules(dao.ctx, workflow, rules); err != nil {
				dao.log.WithError(err).WithField("workflow_id", workflow.ID).
					Warn("Failed to apply adaptation rules during cycle")
			}
		}
	}
}

func (dao *DefaultAdaptiveOrchestrator) performOptimizationCycle() {
	dao.mu.RLock()
	workflows := make([]*engine.Workflow, 0, len(dao.workflows))
	for _, workflow := range dao.workflows {
		workflows = append(workflows, workflow)
	}
	dao.mu.RUnlock()

	for _, workflow := range workflows {
		// Only optimize workflows that have sufficient execution history
		// BR-ORK-003 Requirement 2: Execution Count Tracking
		if dao.hasMinimumExecutionHistory(workflow.ID, minExecutionsForOptimization) {
			// RULE 12 COMPLIANCE: Use enhanced llm.Client for workflow optimization
			// Following project guideline: integrate new business logic with existing code
			if dao.llmClient != nil {
				// Get execution history for optimization
				executionHistory := dao.getWorkflowExecutionHistory(workflow.ID)

				// Call enhanced llm.Client optimization methods
				optimizedWorkflowInterface, err := dao.llmClient.OptimizeWorkflow(dao.ctx, workflow, executionHistory)
				if err != nil {
					dao.log.WithError(err).WithField("workflow_id", workflow.ID).
						Warn("LLM optimization failed, falling back to legacy optimization")
					// Fallback to legacy optimization
					_, fallbackErr := dao.OptimizeWorkflow(dao.ctx, workflow.ID)
					if fallbackErr != nil {
						dao.log.WithError(fallbackErr).WithField("workflow_id", workflow.ID).
							Error("Both LLM and legacy optimization failed")
					}
				} else if optimizedWorkflowInterface != nil {
					// RULE 12 COMPLIANCE: Type assert interface{} return to *engine.Workflow
					if optimizedWorkflow, ok := optimizedWorkflowInterface.(*engine.Workflow); ok {
						// Apply the optimized workflow
						err = dao.applyOptimizedWorkflow(workflow.ID, optimizedWorkflow)
						if err != nil {
							dao.log.WithError(err).WithField("workflow_id", workflow.ID).
								Error("Failed to apply optimized workflow")
						} else {
							dao.log.WithFields(logrus.Fields{
								"workflow_id":  workflow.ID,
								"original_id":  workflow.ID,
								"optimized_id": optimizedWorkflow.ID,
							}).Info("Successfully applied LLM optimization")
						}
					} else {
						dao.log.WithField("workflow_id", workflow.ID).
							Warn("LLM optimization returned unexpected type, falling back to legacy optimization")
						_, fallbackErr := dao.OptimizeWorkflow(dao.ctx, workflow.ID)
						if fallbackErr != nil {
							dao.log.WithError(fallbackErr).WithField("workflow_id", workflow.ID).
								Error("Legacy optimization fallback failed")
						}
					}
				}
			} else {
				// Fallback to legacy optimization when Self Optimizer is not available
				dao.log.WithField("workflow_id", workflow.ID).Debug("Self Optimizer not available, using legacy optimization")
				_, err := dao.OptimizeWorkflow(dao.ctx, workflow.ID)
				if err != nil {
					dao.log.WithError(err).WithField("workflow_id", workflow.ID).
						Warn("Failed to optimize workflow during cycle")
				}
			}
		}
	}
}

// getWorkflowExecutionHistory retrieves execution history for Self Optimizer
// Business Requirement: BR-SELF-OPT-001 - Provide execution history for adaptive optimization
func (dao *DefaultAdaptiveOrchestrator) getWorkflowExecutionHistory(workflowID string) []*engine.RuntimeWorkflowExecution {
	dao.executionMu.RLock()
	defer dao.executionMu.RUnlock()

	history := make([]*engine.RuntimeWorkflowExecution, 0)
	for _, execution := range dao.executions {
		if execution.WorkflowID == workflowID {
			history = append(history, execution)
		}
	}

	dao.log.WithFields(logrus.Fields{
		"workflow_id":   workflowID,
		"history_count": len(history),
	}).Debug("Retrieved execution history for Self Optimizer")

	return history
}

// applyOptimizedWorkflow applies an optimized workflow to replace the original
// Business Requirement: BR-SELF-OPT-001 - Apply Self Optimizer optimizations to workflows
func (dao *DefaultAdaptiveOrchestrator) applyOptimizedWorkflow(originalWorkflowID string, optimizedWorkflow *engine.Workflow) error {
	dao.mu.Lock()
	defer dao.mu.Unlock()

	// Validate optimized workflow
	if optimizedWorkflow == nil {
		return fmt.Errorf("optimized workflow cannot be nil")
	}

	if optimizedWorkflow.Template == nil {
		return fmt.Errorf("optimized workflow template cannot be nil")
	}

	// Store the optimized workflow
	dao.workflows[originalWorkflowID] = optimizedWorkflow
	dao.templates[originalWorkflowID] = optimizedWorkflow.Template

	dao.log.WithFields(logrus.Fields{
		"original_workflow_id":  originalWorkflowID,
		"optimized_workflow_id": optimizedWorkflow.ID,
		"optimization_applied":  true,
	}).Info("Applied optimized workflow from Self Optimizer")

	return nil
}

func (dao *DefaultAdaptiveOrchestrator) collectMetrics() {
	// Collect orchestrator-level metrics
	dao.executionMu.RLock()
	runningExecutions := 0
	optimizedExecutions := 0
	for _, execution := range dao.executions {
		if execution.Status == string(engine.ExecutionStatusRunning) {
			runningExecutions++
		}
		// Count Self Optimizer optimized executions
		if execution.Metadata != nil {
			if _, isOptimized := execution.Metadata["self_optimizer_applied"]; isOptimized {
				optimizedExecutions++
			}
		}
	}
	dao.executionMu.RUnlock()

	// Count workflows with Self Optimizer optimizations
	dao.mu.RLock()
	optimizedWorkflows := 0
	// RULE 12 COMPLIANCE: Check enhanced llm.Client availability instead of deprecated SelfOptimizer
	llmClientAvailable := dao.llmClient != nil
	for _, workflow := range dao.workflows {
		if workflow.Template != nil && workflow.Template.Metadata != nil {
			if _, isOptimized := workflow.Template.Metadata["optimization_source"]; isOptimized {
				optimizedWorkflows++
			}
		}
	}
	dao.mu.RUnlock()

	// Production monitoring metrics for Self Optimizer
	dao.log.WithFields(logrus.Fields{
		"running_executions":       runningExecutions,
		"total_workflows":      len(dao.workflows),
		"total_executions":     len(dao.executions),
		"llm_client_available": llmClientAvailable,
		"optimized_workflows":      optimizedWorkflows,
		"optimized_executions":     optimizedExecutions,
		"optimization_rate":        float64(optimizedWorkflows) / float64(len(dao.workflows)),
	}).Info("Orchestrator metrics with Self Optimizer monitoring")
}

// Utility functions - following development guidelines: reuse code whenever possible
// Consolidated ID generation to eliminate duplication

// generateUniqueID creates a unique ID with the specified prefix
// Following development guidelines: avoid duplicating structure names and reuse code
func generateUniqueID(prefix string) string {
	return prefix + "-" + uuid.New().String()
}

// ID generation convenience functions using the consolidated approach
func generateExecutionID() string          { return generateUniqueID("exec") }
func generateOptimizationID() string       { return generateUniqueID("opt") }
func generateOptimizationChangeID() string { return generateUniqueID("change") }
func generateRequestID() string            { return generateUniqueID("req") }
func generateTraceID() string              { return generateUniqueID("trace") }
func generateCorrelationID() string        { return generateUniqueID("corr") }

// Additional helper methods will be implemented in subsequent files...
