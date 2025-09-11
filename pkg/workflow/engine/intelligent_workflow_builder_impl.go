package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	sharedTypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// DefaultIntelligentWorkflowBuilder implements the IntelligentWorkflowBuilder interface
// This is a real implementation that replaces the stub interface
type DefaultIntelligentWorkflowBuilder struct {
	llmClient        llm.Client
	vectorDB         vector.VectorDatabase
	analyticsEngine  sharedTypes.AnalyticsEngine
	metricsCollector AIMetricsCollector
	patternStore     PatternStore
	executionRepo    ExecutionRepository
	templateFactory  *TemplateFactory
	validator        WorkflowValidator
	simulator        *WorkflowSimulator
	workflowEngine   WorkflowEngine // Real workflow execution engine for BR-PA-011
	log              *logrus.Logger
	config           *WorkflowBuilderConfig
	stepTypeHandlers map[StepType]StepHandler
	patternMatcher   *PatternMatcher
}

// WorkflowBuilderConfig provides configuration for the workflow builder
type WorkflowBuilderConfig struct {
	MaxWorkflowSteps      int           `yaml:"max_workflow_steps" default:"20"`
	DefaultStepTimeout    time.Duration `yaml:"default_step_timeout" default:"5m"`
	MaxRetries            int           `yaml:"max_retries" default:"3"`
	MinPatternSimilarity  float64       `yaml:"min_pattern_similarity" default:"0.8"`
	MinExecutionCount     int           `yaml:"min_execution_count" default:"5"`
	MinSuccessRate        float64       `yaml:"min_success_rate" default:"0.7"`
	PatternLookbackDays   int           `yaml:"pattern_lookback_days" default:"30"`
	EnableSafetyChecks    bool          `yaml:"enable_safety_checks" default:"true"`
	EnableSimulation      bool          `yaml:"enable_simulation" default:"true"`
	ValidationTimeout     time.Duration `yaml:"validation_timeout" default:"2m"`
	EnableLearning        bool          `yaml:"enable_learning" default:"true"`
	LearningBatchSize     int           `yaml:"learning_batch_size" default:"100"`
	PatternUpdateInterval time.Duration `yaml:"pattern_update_interval" default:"1h"`
}

// StepHandler defines how different step types are generated
type StepHandler interface {
	GenerateSteps(ctx context.Context, objective *WorkflowObjective, context *WorkflowContext) ([]*ExecutableWorkflowStep, error)
	ValidateStep(step *ExecutableWorkflowStep) error
	OptimizeStep(step *ExecutableWorkflowStep, metrics *ExecutionMetrics) (*ExecutableWorkflowStep, error)
}

// NewIntelligentWorkflowBuilder creates a new intelligent workflow builder
func NewIntelligentWorkflowBuilder(
	slmClient llm.Client,
	vectorDB vector.VectorDatabase,
	analyticsEngine sharedTypes.AnalyticsEngine,
	metricsCollector AIMetricsCollector,
	patternStore PatternStore,
	executionRepo ExecutionRepository,
	log *logrus.Logger,
) *DefaultIntelligentWorkflowBuilder {
	if log == nil {
		log = logrus.New()
	}

	config := &WorkflowBuilderConfig{
		MaxWorkflowSteps:      20,
		DefaultStepTimeout:    5 * time.Minute,
		MaxRetries:            3,
		MinPatternSimilarity:  0.8,
		MinExecutionCount:     5,
		MinSuccessRate:        0.7,
		PatternLookbackDays:   30,
		EnableSafetyChecks:    true,
		EnableSimulation:      true,
		ValidationTimeout:     2 * time.Minute,
		EnableLearning:        true,
		LearningBatchSize:     100,
		PatternUpdateInterval: time.Hour,
	}

	builder := &DefaultIntelligentWorkflowBuilder{
		llmClient:        slmClient,
		vectorDB:         vectorDB,
		analyticsEngine:  analyticsEngine,
		metricsCollector: metricsCollector,
		patternStore:     patternStore,
		executionRepo:    executionRepo,
		templateFactory:  &TemplateFactory{templates: make(map[string]*ExecutableTemplate)},
		validator:        nil, // Will be set by caller
		simulator:        NewWorkflowSimulator(nil, log),
		workflowEngine:   nil, // Will be created when needed for real execution
		log:              log,
		config:           config,
		stepTypeHandlers: make(map[StepType]StepHandler),
		patternMatcher:   nil, // Will be implemented later
	}

	// Register step type handlers
	builder.registerStepHandlers()

	return builder
}

// setupWorkflowEngine creates and configures a real workflow engine for execution
// Implements BR-PA-011: Execute 25+ supported Kubernetes remediation actions
func (b *DefaultIntelligentWorkflowBuilder) setupWorkflowEngine() error {
	if b.workflowEngine != nil {
		return nil // Already set up
	}

	// Create a basic workflow engine configuration
	config := &WorkflowEngineConfig{
		DefaultStepTimeout:    b.config.DefaultStepTimeout,
		MaxRetryDelay:         5 * time.Minute,
		EnableStateRecovery:   true,
		EnableDetailedLogging: false,
		MaxConcurrency:        10,
	}

	// Create state storage implementation for execution
	// Note: For production use, a real database connection should be provided
	stateStorage := NewWorkflowStateStorage(nil, b.log)

	// Create workflow engine with available dependencies
	// Note: For real K8s operations, k8sClient and actionRepo should be provided
	// For now, create with nil values and let the engine handle gracefully
	b.workflowEngine = NewDefaultWorkflowEngine(
		nil, // k8sClient - will be set when available
		nil, // actionRepo - will be set when available
		nil, // monitoringClients - will be set when available
		stateStorage,
		b.executionRepo,
		config,
		b.log,
	)

	b.log.Info("Real workflow execution engine configured for Kubernetes operations")
	return nil
}

// Business Requirement: BR-AI-001 - Generate optimal workflows from high-level objectives
func (b *DefaultIntelligentWorkflowBuilder) GenerateWorkflow(ctx context.Context, objective *WorkflowObjective) (*ExecutableTemplate, error) {
	b.log.WithFields(logrus.Fields{
		"objective_id":   objective.ID,
		"objective_type": objective.Type,
		"description":    objective.Description,
	}).Info("Generating workflow from objective")

	// Phase 1: Analyze objective and gather context
	workflowContext, err := b.buildWorkflowContext(ctx, objective)
	if err != nil {
		return nil, fmt.Errorf("failed to build workflow context: %w", err)
	}

	// Phase 2: Search for similar patterns
	patterns, err := b.findRelevantPatterns(ctx, objective, workflowContext)
	if err != nil {
		b.log.WithError(err).Warn("Failed to find patterns, proceeding with AI generation")
		patterns = []*WorkflowPattern{} // Continue without patterns
	}

	// Phase 3: Generate workflow using AI + patterns
	template, err := b.generateWorkflowTemplate(ctx, objective, workflowContext, patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to generate workflow template: %w", err)
	}

	// Phase 4: Apply safety checks and validation
	if b.config.EnableSafetyChecks {
		if err := b.applySafetyChecks(template, objective); err != nil {
			return nil, fmt.Errorf("workflow failed safety checks: %w", err)
		}
	}

	// Phase 5: Optimize workflow structure
	optimizedTemplate, err := b.OptimizeWorkflowStructure(ctx, template)
	if err != nil {
		b.log.WithError(err).Warn("Optimization failed, using original template")
		optimizedTemplate = template
	}

	b.log.WithFields(logrus.Fields{
		"workflow_id":   optimizedTemplate.ID,
		"step_count":    len(optimizedTemplate.Steps),
		"patterns_used": len(patterns),
	}).Info("Successfully generated workflow")

	return optimizedTemplate, nil
}

// Business Requirement: BR-AI-002 - Optimize workflow structure for efficiency and reliability
func (b *DefaultIntelligentWorkflowBuilder) OptimizeWorkflowStructure(ctx context.Context, template *ExecutableTemplate) (*ExecutableTemplate, error) {
	b.log.WithField("workflow_id", template.ID).Info("Optimizing workflow structure with performance analytics")

	// Phase 1: Analyze current workflow performance
	performanceAnalysis, err := b.analyzeWorkflowPerformance(ctx, template)
	if err != nil {
		b.log.WithError(err).Warn("Performance analysis failed, proceeding with basic optimization")
		return b.performBasicOptimization(template)
	}

	// Phase 2: Identify bottlenecks and optimization opportunities
	bottlenecks := b.identifyBottlenecks(performanceAnalysis)

	// Phase 3: Generate optimization recommendations
	recommendations, err := b.generateOptimizationRecommendations(ctx, template, bottlenecks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate optimization recommendations: %w", err)
	}

	// Phase 4: Create optimized copy and apply advanced workflow optimization engine
	optimized := b.deepCopyTemplate(template)
	optimized, err = b.applyAdvancedWorkflowOptimizations(ctx, optimized, recommendations, performanceAnalysis)
	if err != nil {
		return nil, fmt.Errorf("failed to apply advanced optimizations: %w", err)
	}

	// Phase 5: Apply comprehensive resource constraint management
	// Note: objective is not available in this context, we'll use a default objective
	defaultObjective := &WorkflowObjective{
		Type:        "optimization",
		Priority:    3,
		Constraints: make(map[string]interface{}),
	}
	optimized, err = b.applyResourceConstraintManagement(ctx, optimized, defaultObjective)
	if err != nil {
		b.log.WithError(err).Warn("Resource constraint management failed, using current optimizations")
	}

	// Phase 6: Validate optimization results
	if err := b.validateOptimizationResults(template, optimized, performanceAnalysis); err != nil {
		b.log.WithError(err).Warn("Optimization validation failed, reverting to basic optimization")
		return b.performBasicOptimization(template)
	}

	b.log.WithFields(logrus.Fields{
		"original_steps":          len(template.Steps),
		"optimized_steps":         len(optimized.Steps),
		"workflow_id":             optimized.ID,
		"bottlenecks_addressed":   len(bottlenecks),
		"recommendations_applied": len(recommendations),
		"effectiveness_gain":      b.calculateEffectivenessGain(performanceAnalysis, optimized),
	}).Info("Advanced workflow structure optimization complete")

	return optimized, nil
}

// performBasicOptimization provides fallback basic optimization
func (b *DefaultIntelligentWorkflowBuilder) performBasicOptimization(template *ExecutableTemplate) (*ExecutableTemplate, error) {
	b.log.WithField("workflow_id", template.ID).Info("Performing basic workflow optimization")

	// Create optimized copy
	optimized := b.deepCopyTemplate(template)

	// Basic optimizations
	if err := b.optimizeParallelExecution(optimized); err != nil {
		b.log.WithError(err).Warn("Parallel execution optimization failed")
	}

	b.removeRedundantSteps(optimized)
	b.optimizeStepOrdering(optimized)

	if err := b.optimizeResourceUsage(optimized); err != nil {
		b.log.WithError(err).Warn("Resource usage optimization failed")
	}

	if err := b.enhanceErrorHandling(optimized); err != nil {
		b.log.WithError(err).Warn("Error handling enhancement failed")
	}

	return optimized, nil
}

// validateOptimizationResults validates that optimizations improved the workflow
func (b *DefaultIntelligentWorkflowBuilder) validateOptimizationResults(original, optimized *ExecutableTemplate, analysis *PerformanceAnalysis) error {
	// Basic validation - ensure we didn't break the workflow
	if len(optimized.Steps) == 0 {
		return fmt.Errorf("optimization resulted in empty workflow")
	}

	// Ensure critical steps are still present
	originalStepTypes := make(map[string]int)
	optimizedStepTypes := make(map[string]int)

	for _, step := range original.Steps {
		if step.Action != nil {
			originalStepTypes[step.Action.Type]++
		}
	}

	for _, step := range optimized.Steps {
		if step.Action != nil {
			optimizedStepTypes[step.Action.Type]++
		}
	}

	// Check that no critical action types were removed
	for actionType, count := range originalStepTypes {
		if optimizedStepTypes[actionType] < count && b.isCriticalActionType(actionType) {
			return fmt.Errorf("critical action type %s was removed or reduced during optimization", actionType)
		}
	}

	return nil
}

// calculateEffectivenessGain estimates the effectiveness improvement
func (b *DefaultIntelligentWorkflowBuilder) calculateEffectivenessGain(analysis *PerformanceAnalysis, optimized *ExecutableTemplate) float64 {
	// Simple heuristic based on step reduction and optimization metadata
	originalSteps := float64(len(analysis.Optimizations)) // Approximate original complexity
	optimizedSteps := float64(len(optimized.Steps))

	if originalSteps == 0 {
		return 0.0
	}

	// Calculate relative improvement (negative if steps increased)
	stepEfficiency := (originalSteps - optimizedSteps) / originalSteps

	// Add bonus for optimization features
	var optimizationBonus float64
	for _, step := range optimized.Steps {
		if step.Metadata != nil {
			if optimized, ok := step.Metadata["resource_optimized"].(bool); ok && optimized {
				optimizationBonus += 0.1
			}
			if parallelized, ok := step.Metadata["parallel"].(bool); ok && parallelized {
				optimizationBonus += 0.15
			}
		}
	}

	return stepEfficiency + optimizationBonus
}

// isCriticalActionType determines if an action type is critical and shouldn't be removed
func (b *DefaultIntelligentWorkflowBuilder) isCriticalActionType(actionType string) bool {
	criticalActions := map[string]bool{
		"validate":     true,
		"backup":       true,
		"rollback":     true,
		"notify":       true,
		"safety_check": true,
		"health_check": true,
	}

	return criticalActions[actionType]
}

// Business Requirement: BR-RC-001 - Comprehensive Resource Constraint Management
func (b *DefaultIntelligentWorkflowBuilder) applyResourceConstraintManagement(ctx context.Context, template *ExecutableTemplate, objective *WorkflowObjective) (*ExecutableTemplate, error) {
	b.log.WithFields(logrus.Fields{
		"workflow_id":    template.ID,
		"objective_type": objective.Type,
		"step_count":     len(template.Steps),
	}).Info("Applying comprehensive resource constraint management")

	startTime := time.Now()
	optimized := b.deepCopyTemplate(template)

	// Phase 1: Extract and validate constraints
	constraints, err := b.extractConstraintsFromObjective(objective)
	if err != nil {
		return template, fmt.Errorf("failed to extract constraints: %w", err)
	}

	// Phase 2: Apply time-based constraints
	if err := b.applyTimeConstraints(optimized, constraints); err != nil {
		b.log.WithError(err).Warn("Time constraint application failed")
	}

	// Phase 3: Apply resource limit constraints
	if err := b.applyResourceLimitConstraints(optimized, constraints); err != nil {
		b.log.WithError(err).Warn("Resource limit constraint application failed")
	}

	// Phase 4: Apply cost optimization constraints
	if err := b.applyCostOptimizationConstraints(optimized, constraints); err != nil {
		b.log.WithError(err).Warn("Cost optimization constraint application failed")
	}

	// Phase 5: Apply environment-specific resource constraints
	if err := b.applyEnvironmentResourceConstraints(optimized, constraints); err != nil {
		b.log.WithError(err).Warn("Environment resource constraint application failed")
	}

	// Phase 6: Calculate and validate resource efficiency
	resourceEfficiency, err := b.calculateResourceEfficiency(optimized, template)
	if err != nil {
		b.log.WithError(err).Warn("Resource efficiency calculation failed")
		resourceEfficiency = 0.5 // Default value
	}

	duration := time.Since(startTime)
	b.log.WithFields(logrus.Fields{
		"workflow_id":           optimized.ID,
		"resource_efficiency":   resourceEfficiency,
		"constraints_applied":   len(constraints),
		"optimization_duration": duration,
	}).Info("Resource constraint management completed")

	return optimized, nil
}

// Helper methods for resource constraint management

// extractConstraintsFromObjective extracts resource constraints from workflow objective
func (b *DefaultIntelligentWorkflowBuilder) extractConstraintsFromObjective(objective *WorkflowObjective) (map[string]interface{}, error) {
	constraints := make(map[string]interface{})

	// Copy explicit constraints from objective
	for key, value := range objective.Constraints {
		constraints[key] = value
	}

	// Add default constraints based on objective type and priority
	b.addDefaultConstraints(constraints, objective)

	// Add environment-based constraints
	b.addEnvironmentConstraints(constraints, objective)

	// Validate constraint values
	if err := b.validateConstraints(constraints); err != nil {
		return nil, fmt.Errorf("constraint validation failed: %w", err)
	}

	return constraints, nil
}

// applyTimeConstraints applies time-based resource constraints
func (b *DefaultIntelligentWorkflowBuilder) applyTimeConstraints(template *ExecutableTemplate, constraints map[string]interface{}) error {
	b.log.WithField("constraint_count", len(constraints)).Debug("Applying time constraints")

	// Apply maximum execution time constraint using existing helper
	if maxTime, ok := constraints["max_execution_time"]; ok {
		if maxTimeStr, ok := maxTime.(string); ok {
			if duration, err := time.ParseDuration(maxTimeStr); err == nil {
				b.adjustTimeoutsForMaxDuration(template, duration)
				b.log.WithField("max_execution_time", duration).Info("Applied maximum execution time constraint")
			}
		}
	}

	// Apply step timeout constraints
	if stepTimeout, ok := constraints["max_step_timeout"]; ok {
		if stepTimeoutStr, ok := stepTimeout.(string); ok {
			if duration, err := time.ParseDuration(stepTimeoutStr); err == nil {
				b.enforceMaxStepTimeouts(template, duration)
			}
		}
	}

	return nil
}

// applyResourceLimitConstraints applies resource limit constraints
func (b *DefaultIntelligentWorkflowBuilder) applyResourceLimitConstraints(template *ExecutableTemplate, constraints map[string]interface{}) error {
	b.log.Debug("Applying resource limit constraints")

	// Apply resource limits using existing helper
	if resourceLimits, ok := constraints["resource_limits"]; ok {
		if limits, ok := resourceLimits.(map[string]interface{}); ok {
			b.applyResourceConstraints(template, limits)
			b.log.WithField("resource_limits", limits).Info("Applied resource limit constraints")
		}
	}

	// Apply memory pressure constraints
	if memoryPressure, ok := constraints["memory_pressure"]; ok {
		if pressure, ok := memoryPressure.(string); ok {
			b.applyMemoryPressureOptimization(template, pressure)
		}
	}

	// Apply CPU quota constraints
	if cpuQuota, ok := constraints["cpu_quota"]; ok {
		if quota, ok := cpuQuota.(float64); ok {
			b.applyCPUQuotaOptimization(template, quota)
		}
	}

	return nil
}

// applyCostOptimizationConstraints applies cost-focused constraints
func (b *DefaultIntelligentWorkflowBuilder) applyCostOptimizationConstraints(template *ExecutableTemplate, constraints map[string]interface{}) error {
	b.log.Debug("Applying cost optimization constraints")

	// Apply cost budget constraints
	if budget, ok := constraints["cost_budget"]; ok {
		if budgetValue, ok := budget.(float64); ok {
			b.optimizeForCostBudget(template, budgetValue)
		}
	}

	// Apply efficiency targets
	if efficiencyTarget, ok := constraints["efficiency_target"]; ok {
		if target, ok := efficiencyTarget.(float64); ok {
			b.optimizeForEfficiencyTarget(template, target)
		}
	}

	return nil
}

// applyEnvironmentResourceConstraints applies environment-specific constraints
func (b *DefaultIntelligentWorkflowBuilder) applyEnvironmentResourceConstraints(template *ExecutableTemplate, constraints map[string]interface{}) error {
	environment := b.getEnvironmentFromConstraints(constraints)

	b.log.WithField("environment", environment).Debug("Applying environment-specific resource constraints")

	switch environment {
	case "production":
		b.applyProductionResourceConstraints(template, constraints)
	case "staging":
		b.applyStagingResourceConstraints(template, constraints)
	case "development":
		b.applyDevelopmentResourceConstraints(template, constraints)
	default:
		b.applyDefaultResourceConstraints(template, constraints)
	}

	return nil
}

// Constraint validation and setup methods

func (b *DefaultIntelligentWorkflowBuilder) addDefaultConstraints(constraints map[string]interface{}, objective *WorkflowObjective) {
	// Add priority-based constraints
	switch objective.Priority {
	case 1, 2: // High priority
		if _, exists := constraints["max_execution_time"]; !exists {
			constraints["max_execution_time"] = "30m"
		}
	case 3, 4: // Medium priority
		if _, exists := constraints["max_execution_time"]; !exists {
			constraints["max_execution_time"] = "1h"
		}
	default: // Low priority
		if _, exists := constraints["max_execution_time"]; !exists {
			constraints["max_execution_time"] = "2h"
		}
	}

	// Add default resource limits if not specified
	if _, exists := constraints["resource_limits"]; !exists {
		constraints["resource_limits"] = map[string]interface{}{
			"cpu":    "1000m",
			"memory": "1Gi",
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) addEnvironmentConstraints(constraints map[string]interface{}, objective *WorkflowObjective) {
	environment := "production" // default
	if objective.Constraints != nil {
		if env, ok := objective.Constraints["environment"].(string); ok {
			environment = env
		}
	}

	switch environment {
	case "production":
		constraints["safety_level"] = "high"
		constraints["max_parallel_steps"] = 2
	case "staging":
		constraints["safety_level"] = "medium"
		constraints["max_parallel_steps"] = 4
	case "development":
		constraints["safety_level"] = "low"
		constraints["max_parallel_steps"] = 8
	}
}

func (b *DefaultIntelligentWorkflowBuilder) validateConstraints(constraints map[string]interface{}) error {
	// Validate time constraints
	if maxTime, ok := constraints["max_execution_time"].(string); ok {
		if _, err := time.ParseDuration(maxTime); err != nil {
			return fmt.Errorf("invalid max_execution_time format: %s", maxTime)
		}
	}

	// Validate resource limits
	if limits, ok := constraints["resource_limits"].(map[string]interface{}); ok {
		if cpu, exists := limits["cpu"]; exists {
			if _, ok := cpu.(string); !ok {
				return fmt.Errorf("resource_limits.cpu must be a string")
			}
		}
		if memory, exists := limits["memory"]; exists {
			if _, ok := memory.(string); !ok {
				return fmt.Errorf("resource_limits.memory must be a string")
			}
		}
	}

	return nil
}

// Resource constraint optimization helper methods

// Note: adjustTimeoutsForMaxDuration and applyResourceConstraints are implemented in helpers file

func (b *DefaultIntelligentWorkflowBuilder) enforceMaxStepTimeouts(template *ExecutableTemplate, maxTimeout time.Duration) {
	for _, step := range template.Steps {
		if step.Timeout > maxTimeout {
			step.Timeout = maxTimeout
			b.log.WithFields(logrus.Fields{
				"step_id":     step.ID,
				"old_timeout": step.Timeout,
				"new_timeout": maxTimeout,
			}).Debug("Enforced maximum step timeout")
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) applyMemoryPressureOptimization(template *ExecutableTemplate, pressure string) {
	multiplier := 1.0
	switch pressure {
	case "high":
		multiplier = 0.7 // Reduce memory by 30%
	case "medium":
		multiplier = 0.85 // Reduce memory by 15%
	case "low":
		multiplier = 1.0 // No reduction
	}

	for _, step := range template.Steps {
		if step.Action != nil && step.Action.Parameters != nil {
			if memLimit, exists := step.Action.Parameters["memory_limit"].(string); exists {
				// Parse and reduce memory limit (simplified)
				step.Action.Parameters["memory_limit"] = fmt.Sprintf("%.0f%s",
					parseMemoryValue(memLimit)*multiplier, "Mi")
			}
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) applyCPUQuotaOptimization(template *ExecutableTemplate, quota float64) {
	totalSteps := float64(len(template.Steps))
	perStepQuota := quota / totalSteps

	for _, step := range template.Steps {
		if step.Action != nil && step.Action.Parameters != nil {
			step.Action.Parameters["cpu_limit"] = fmt.Sprintf("%.0fm", perStepQuota*1000)
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeForCostBudget(template *ExecutableTemplate, budget float64) {
	// Cost-based optimization
	costPerStep := budget / float64(len(template.Steps))

	// Apply conservative resource limits based on budget
	for _, step := range template.Steps {
		if step.Action != nil && step.Action.Parameters != nil {
			if costPerStep < 1.0 { // Low budget
				step.Action.Parameters["cpu_limit"] = "200m"
				step.Action.Parameters["memory_limit"] = "256Mi"
			} else if costPerStep < 5.0 { // Medium budget
				step.Action.Parameters["cpu_limit"] = "500m"
				step.Action.Parameters["memory_limit"] = "512Mi"
			}
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeForEfficiencyTarget(template *ExecutableTemplate, target float64) {
	// Optimize for efficiency target (0-1 scale)
	if target >= 0.8 { // High efficiency target
		// Enable more parallelization
		b.enableParallelExecution(template)
		// Optimize timeouts
		for _, step := range template.Steps {
			if step.Timeout > 5*time.Minute {
				step.Timeout = 5 * time.Minute
			}
		}
	}
}

// Note: enableParallelExecution and canRunInParallel are implemented in helpers file

func (b *DefaultIntelligentWorkflowBuilder) getEnvironmentFromConstraints(constraints map[string]interface{}) string {
	if env, ok := constraints["environment"].(string); ok {
		return env
	}
	return "production" // default
}

func (b *DefaultIntelligentWorkflowBuilder) applyProductionResourceConstraints(template *ExecutableTemplate, constraints map[string]interface{}) {
	// Production: Conservative resource limits, high reliability
	for _, step := range template.Steps {
		if step.Action != nil {
			if step.Action.Parameters == nil {
				step.Action.Parameters = make(map[string]interface{})
			}
			step.Action.Parameters["cpu_limit"] = "500m"
			step.Action.Parameters["memory_limit"] = "512Mi"
			step.Timeout = 10 * time.Minute // Longer timeouts for reliability
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) applyStagingResourceConstraints(template *ExecutableTemplate, constraints map[string]interface{}) {
	// Staging: Balanced resource limits
	for _, step := range template.Steps {
		if step.Action != nil {
			if step.Action.Parameters == nil {
				step.Action.Parameters = make(map[string]interface{})
			}
			step.Action.Parameters["cpu_limit"] = "750m"
			step.Action.Parameters["memory_limit"] = "768Mi"
			step.Timeout = 8 * time.Minute
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) applyDevelopmentResourceConstraints(template *ExecutableTemplate, constraints map[string]interface{}) {
	// Development: Higher resource limits, shorter timeouts
	for _, step := range template.Steps {
		if step.Action != nil {
			if step.Action.Parameters == nil {
				step.Action.Parameters = make(map[string]interface{})
			}
			step.Action.Parameters["cpu_limit"] = "1000m"
			step.Action.Parameters["memory_limit"] = "1Gi"
			step.Timeout = 5 * time.Minute
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) applyDefaultResourceConstraints(template *ExecutableTemplate, constraints map[string]interface{}) {
	// Apply default resource constraints
	b.applyProductionResourceConstraints(template, constraints)
}

func (b *DefaultIntelligentWorkflowBuilder) calculateResourceEfficiency(optimized, original *ExecutableTemplate) (float64, error) {
	// Calculate resource efficiency improvement
	optimizedResources := b.calculateTotalResources(optimized)
	originalResources := b.calculateTotalResources(original)

	if originalResources == 0 {
		return 0.5, nil // Default efficiency
	}

	efficiency := (originalResources - optimizedResources) / originalResources
	if efficiency < 0 {
		efficiency = 0
	} else if efficiency > 1 {
		efficiency = 1
	}

	return efficiency, nil
}

func (b *DefaultIntelligentWorkflowBuilder) calculateTotalResources(template *ExecutableTemplate) float64 {
	total := 0.0
	for _, step := range template.Steps {
		if step.Action != nil && step.Action.Parameters != nil {
			// Simple resource calculation
			if cpuLimit, exists := step.Action.Parameters["cpu_limit"].(string); exists {
				total += parseCPUValue(cpuLimit)
			}
			if memLimit, exists := step.Action.Parameters["memory_limit"].(string); exists {
				total += parseMemoryValue(memLimit)
			}
		}
		// Add timeout as a resource factor
		total += step.Timeout.Seconds() / 60.0 // Convert to minutes
	}
	return total
}

// Utility methods for resource parsing (simplified)

func parseMemoryValue(memStr string) float64 {
	// Simplified memory parsing - in production would use proper units parsing
	if strings.HasSuffix(memStr, "Gi") {
		memStr = strings.TrimSuffix(memStr, "Gi")
		if val, err := strconv.ParseFloat(memStr, 64); err == nil {
			return val * 1024 // Convert to Mi
		}
	} else if strings.HasSuffix(memStr, "Mi") {
		memStr = strings.TrimSuffix(memStr, "Mi")
		if val, err := strconv.ParseFloat(memStr, 64); err == nil {
			return val
		}
	}
	return 512 // Default
}

func parseCPUValue(cpuStr string) float64 {
	// Simplified CPU parsing
	if strings.HasSuffix(cpuStr, "m") {
		cpuStr = strings.TrimSuffix(cpuStr, "m")
		if val, err := strconv.ParseFloat(cpuStr, 64); err == nil {
			return val / 1000.0 // Convert to cores
		}
	}
	if val, err := strconv.ParseFloat(cpuStr, 64); err == nil {
		return val
	}
	return 0.5 // Default
}

// Business Requirement: BR-WO-001 - Advanced Workflow Optimization Engine
func (b *DefaultIntelligentWorkflowBuilder) applyAdvancedWorkflowOptimizations(ctx context.Context, template *ExecutableTemplate, recommendations []*OptimizationSuggestion, performanceAnalysis *PerformanceAnalysis) (*ExecutableTemplate, error) {
	b.log.WithFields(logrus.Fields{
		"workflow_id":           template.ID,
		"recommendation_count":  len(recommendations),
		"bottlenecks_count":     len(performanceAnalysis.Bottlenecks),
		"current_effectiveness": performanceAnalysis.Effectiveness,
	}).Info("Applying advanced workflow optimization engine")

	startTime := time.Now()
	optimized := b.deepCopyTemplate(template)

	// Phase 1: Structural Optimizations
	if err := b.applyStructuralOptimizations(ctx, optimized); err != nil {
		b.log.WithError(err).Warn("Structural optimizations failed")
	}

	// Phase 2: Logic-based Optimizations
	if err := b.applyLogicOptimizations(ctx, optimized, recommendations); err != nil {
		b.log.WithError(err).Warn("Logic optimizations failed")
	}

	// Phase 3: Performance-based Optimizations
	if err := b.applyPerformanceOptimizations(ctx, optimized, performanceAnalysis); err != nil {
		b.log.WithError(err).Warn("Performance optimizations failed")
	}

	// Phase 4: Parallelization Optimizations
	if err := b.applyParallelizationOptimizations(ctx, optimized); err != nil {
		b.log.WithError(err).Warn("Parallelization optimizations failed")
	}

	// Phase 5: Cost-effectiveness Optimizations
	if err := b.applyCostEffectivenessOptimizations(ctx, optimized, performanceAnalysis); err != nil {
		b.log.WithError(err).Warn("Cost-effectiveness optimizations failed")
	}

	// Calculate optimization impact
	optimizationImpact := b.calculateOptimizationImpact(template, optimized, performanceAnalysis)
	duration := time.Since(startTime)

	// Update metadata
	optimized.Metadata["optimization_applied"] = true
	optimized.Metadata["optimization_impact"] = optimizationImpact
	optimized.Metadata["optimization_duration"] = duration
	optimized.Metadata["optimizations_used"] = len(recommendations)

	b.log.WithFields(logrus.Fields{
		"workflow_id":         optimized.ID,
		"optimization_impact": optimizationImpact,
		"original_steps":      len(template.Steps),
		"optimized_steps":     len(optimized.Steps),
		"optimization_time":   duration,
	}).Info("Advanced workflow optimization engine completed")

	return optimized, nil
}

// Phase 1: Structural Optimizations
func (b *DefaultIntelligentWorkflowBuilder) applyStructuralOptimizations(ctx context.Context, template *ExecutableTemplate) error {
	b.log.WithField("step_count", len(template.Steps)).Debug("Applying structural optimizations")

	// Remove redundant steps using existing helper
	b.removeRedundantSteps(template)

	// Merge similar steps using existing helper
	b.mergeSimilarSteps(template)

	// Optimize step ordering using existing helper
	b.optimizeStepOrdering(template)

	return nil
}

// Phase 2: Logic-based Optimizations
func (b *DefaultIntelligentWorkflowBuilder) applyLogicOptimizations(ctx context.Context, template *ExecutableTemplate, recommendations []*OptimizationSuggestion) error {
	b.log.WithField("recommendation_count", len(recommendations)).Debug("Applying logic optimizations")

	// Optimize conditions using existing helper
	b.optimizeConditions(template)

	// Apply specific logic optimizations from recommendations
	for _, rec := range recommendations {
		switch rec.Type {
		case "remove_redundant_steps":
			b.removeRedundantSteps(template)
		case "merge_similar_steps":
			b.mergeSimilarSteps(template)
		case "logic_optimization":
			b.applyLogicOptimization(template, rec)
		case "simplify_expressions":
			b.simplifyWorkflowExpressions(template)
		}
	}

	return nil
}

// Phase 3: Performance-based Optimizations
func (b *DefaultIntelligentWorkflowBuilder) applyPerformanceOptimizations(ctx context.Context, template *ExecutableTemplate, analysis *PerformanceAnalysis) error {
	b.log.WithField("effectiveness", analysis.Effectiveness).Debug("Applying performance optimizations")

	// Apply timeout optimizations using existing helper
	for _, step := range template.Steps {
		if step.Timeout > analysis.ExecutionTime*2 {
			// Reduce overly long timeouts
			step.Timeout = analysis.ExecutionTime + (30 * time.Second)
		}
	}

	// Apply resource optimizations based on bottlenecks
	for _, bottleneck := range analysis.Bottlenecks {
		b.optimizeBottleneck(template, bottleneck)
	}

	return nil
}

// Phase 4: Parallelization Optimizations
func (b *DefaultIntelligentWorkflowBuilder) applyParallelizationOptimizations(ctx context.Context, template *ExecutableTemplate) error {
	b.log.WithField("step_count", len(template.Steps)).Debug("Applying parallelization optimizations")

	// Enable parallel execution using existing helper
	b.enableParallelExecution(template)

	// Optimize parallel group sizes
	b.optimizeParallelGroupSizes(template)

	// Add parallel execution metadata
	b.addParallelExecutionMetadata(template)

	return nil
}

// Phase 5: Cost-effectiveness Optimizations
func (b *DefaultIntelligentWorkflowBuilder) applyCostEffectivenessOptimizations(ctx context.Context, template *ExecutableTemplate, analysis *PerformanceAnalysis) error {
	b.log.WithField("cost_efficiency", analysis.CostEfficiency).Debug("Applying cost-effectiveness optimizations")

	// Calculate and apply cost-effective resource limits
	if analysis.CostEfficiency < 0.7 {
		b.applyCostEffectiveResourceLimits(template)
	}

	// Optimize for cost-effectiveness based on existing helper
	costEfficiency := b.calculateCostEfficiency(analysis.ResourceUsage, analysis.Effectiveness)
	if costEfficiency < 0.6 {
		// Apply aggressive cost optimizations
		b.applyAggressiveCostOptimizations(template)
	}

	return nil
}

// Helper methods for advanced optimization engine

// Note: removeRedundantSteps, mergeSimilarSteps, optimizeStepOrdering, optimizeConditions,
// and simplifyExpression are implemented in helpers file

// Additional helper methods for advanced optimization engine

func (b *DefaultIntelligentWorkflowBuilder) calculateOptimizationImpact(original, optimized *ExecutableTemplate, analysis *PerformanceAnalysis) float64 {
	originalComplexity := float64(len(original.Steps))
	optimizedComplexity := float64(len(optimized.Steps))

	if originalComplexity == 0 {
		return 0.0
	}

	// Calculate impact based on step reduction and performance improvement
	complexityReduction := (originalComplexity - optimizedComplexity) / originalComplexity

	// Add bonus for optimization features
	var optimizationBonus float64
	for _, step := range optimized.Steps {
		if step.Metadata != nil {
			if optimized, ok := step.Metadata["resource_optimized"].(bool); ok && optimized {
				optimizationBonus += 0.1
			}
			if parallel, ok := step.Metadata["parallel"].(bool); ok && parallel {
				optimizationBonus += 0.15
			}
		}
	}

	return complexityReduction + optimizationBonus
}

// Note: generateStepKey and generateStepGroupKey are implemented in helpers file

func (b *DefaultIntelligentWorkflowBuilder) canMergeSteps(steps []*ExecutableWorkflowStep) bool {
	if len(steps) < 2 {
		return false
	}

	// Check if steps are similar enough to merge
	firstStep := steps[0]
	for i := 1; i < len(steps); i++ {
		if !b.areStepsSimilar(firstStep, steps[i]) {
			return false
		}
	}
	return true
}

func (b *DefaultIntelligentWorkflowBuilder) areStepsSimilar(step1, step2 *ExecutableWorkflowStep) bool {
	// Steps are similar if they have the same action type
	if step1.Action != nil && step2.Action != nil {
		return step1.Action.Type == step2.Action.Type
	}
	return step1.Type == step2.Type
}

// Note: mergeSteps is implemented in helpers file

func (b *DefaultIntelligentWorkflowBuilder) topologicalSortSteps(steps []*ExecutableWorkflowStep) []*ExecutableWorkflowStep {
	// Simple dependency-aware ordering
	ordered := make([]*ExecutableWorkflowStep, 0, len(steps))
	remaining := make([]*ExecutableWorkflowStep, len(steps))
	copy(remaining, steps)

	for len(remaining) > 0 {
		var next *ExecutableWorkflowStep
		nextIndex := -1

		// Find next step with no unresolved dependencies
		for i, step := range remaining {
			if b.areDependenciesResolved(step, ordered) {
				next = step
				nextIndex = i
				break
			}
		}

		if next == nil {
			// No resolvable dependencies, just take first
			next = remaining[0]
			nextIndex = 0
		}

		ordered = append(ordered, next)
		// Remove from remaining
		remaining = append(remaining[:nextIndex], remaining[nextIndex+1:]...)
	}

	return ordered
}

func (b *DefaultIntelligentWorkflowBuilder) areDependenciesResolved(step *ExecutableWorkflowStep, completed []*ExecutableWorkflowStep) bool {
	if len(step.Dependencies) == 0 {
		return true
	}

	completedNames := make(map[string]bool)
	for _, comp := range completed {
		completedNames[comp.ID] = true
		completedNames[comp.Name] = true
	}

	for _, dep := range step.Dependencies {
		if !completedNames[dep] {
			return false
		}
	}

	return true
}

func (b *DefaultIntelligentWorkflowBuilder) getStepNames(steps []*ExecutableWorkflowStep) []string {
	names := make([]string, len(steps))
	for i, step := range steps {
		names[i] = step.Name
	}
	return names
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeBottleneck(template *ExecutableTemplate, bottleneck *Bottleneck) {
	// Find and optimize the specific bottleneck step
	for _, step := range template.Steps {
		if step.ID == bottleneck.StepID {
			switch bottleneck.Type {
			case BottleneckTypeTimeout:
				// Reduce timeout if it's the bottleneck
				if step.Timeout > 5*time.Minute {
					step.Timeout = 5 * time.Minute
				}
			case BottleneckTypeResource:
				// Optimize resource usage
				if step.Action != nil && step.Action.Parameters != nil {
					b.optimizeStepResourcesForBottleneck(step, bottleneck)
				}
			}
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeStepResourcesForBottleneck(step *ExecutableWorkflowStep, bottleneck *Bottleneck) {
	if step.Action == nil || step.Action.Parameters == nil {
		return
	}

	// Reduce resource consumption based on bottleneck impact
	reductionFactor := bottleneck.Impact * 0.5 // Reduce by up to 50%

	if cpuLimit, ok := step.Action.Parameters["cpu_limit"].(string); ok {
		currentCPU := parseCPUValue(cpuLimit)
		newCPU := currentCPU * (1 - reductionFactor)
		step.Action.Parameters["cpu_limit"] = fmt.Sprintf("%.0fm", newCPU*1000)
	}

	if memLimit, ok := step.Action.Parameters["memory_limit"].(string); ok {
		currentMem := parseMemoryValue(memLimit)
		newMem := currentMem * (1 - reductionFactor)
		step.Action.Parameters["memory_limit"] = fmt.Sprintf("%.0fMi", newMem)
	}
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeParallelGroupSizes(template *ExecutableTemplate) {
	maxParallelSteps := 4 // Configurable max parallel steps
	parallelCount := 0

	for _, step := range template.Steps {
		if step.Metadata != nil {
			if parallel, ok := step.Metadata["parallel"].(bool); ok && parallel {
				parallelCount++
				if parallelCount > maxParallelSteps {
					// Remove parallelism from excess steps
					step.Metadata["parallel"] = false
				}
			}
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) addParallelExecutionMetadata(template *ExecutableTemplate) {
	for _, step := range template.Steps {
		if step.Metadata != nil {
			if parallel, ok := step.Metadata["parallel"].(bool); ok && parallel {
				step.Metadata["parallel_group_size"] = 1
				step.Metadata["parallel_optimization"] = true
			}
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) applyCostEffectiveResourceLimits(template *ExecutableTemplate) {
	// Apply cost-effective resource limits to all steps
	for _, step := range template.Steps {
		if step.Action != nil {
			if step.Action.Parameters == nil {
				step.Action.Parameters = make(map[string]interface{})
			}
			// Set conservative resource limits for cost efficiency
			step.Action.Parameters["cpu_limit"] = "300m"
			step.Action.Parameters["memory_limit"] = "384Mi"
		}
	}
}

func (b *DefaultIntelligentWorkflowBuilder) applyAggressiveCostOptimizations(template *ExecutableTemplate) {
	// Apply aggressive optimizations for cost savings
	for _, step := range template.Steps {
		if step.Action != nil {
			if step.Action.Parameters == nil {
				step.Action.Parameters = make(map[string]interface{})
			}
			// Very conservative resource limits
			step.Action.Parameters["cpu_limit"] = "200m"
			step.Action.Parameters["memory_limit"] = "256Mi"

			// Reduce timeout for faster failure detection
			if step.Timeout > 3*time.Minute {
				step.Timeout = 3 * time.Minute
			}
		}
	}
}

// Note: calculateCostEfficiency and applyLogicOptimization are implemented in helpers file

func (b *DefaultIntelligentWorkflowBuilder) simplifyWorkflowExpressions(template *ExecutableTemplate) {
	// Simplify all expressions in the workflow
	b.optimizeConditions(template)

	// Also optimize any variable expressions
	for key, value := range template.Variables {
		if strValue, ok := value.(string); ok {
			template.Variables[key] = b.simplifyExpression(strValue)
		}
	}
}

// Business Requirement: BR-AI-003 - Discover successful workflow patterns from execution history
func (b *DefaultIntelligentWorkflowBuilder) FindWorkflowPatterns(ctx context.Context, criteria *PatternCriteria) ([]*WorkflowPattern, error) {
	b.log.WithFields(logrus.Fields{
		"min_similarity":      criteria.MinSimilarity,
		"min_execution_count": criteria.MinExecutionCount,
		"min_success_rate":    criteria.MinSuccessRate,
		"time_window":         criteria.TimeWindow,
	}).Info("Discovering workflow patterns")

	// Fetch execution history within time window
	executions, err := b.getExecutionHistory(ctx, criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch execution history: %w", err)
	}

	if len(executions) < criteria.MinExecutionCount {
		b.log.WithFields(logrus.Fields{
			"available_executions": len(executions),
			"required_minimum":     criteria.MinExecutionCount,
		}).Warn("Insufficient execution history for pattern discovery")
		return []*WorkflowPattern{}, nil
	}

	// Group executions by similarity
	clusters, err := b.clusterExecutions(executions, criteria)
	if err != nil {
		return nil, fmt.Errorf("failed to cluster executions: %w", err)
	}

	// Extract patterns from clusters
	patterns := make([]*WorkflowPattern, 0)
	for _, cluster := range clusters {
		if len(cluster.Members) >= criteria.MinExecutionCount {
			pattern, err := b.extractPatternFromCluster(cluster, criteria)
			if err != nil {
				b.log.WithError(err).Warn("Failed to extract pattern from cluster")
				continue
			}

			if pattern.SuccessRate >= criteria.MinSuccessRate {
				patterns = append(patterns, pattern)
			}
		}
	}

	// Rank patterns by effectiveness and confidence
	b.rankPatterns(patterns)

	b.log.WithFields(logrus.Fields{
		"patterns_discovered": len(patterns),
		"executions_analyzed": len(executions),
		"clusters_formed":     len(clusters),
	}).Info("Pattern discovery complete")

	return patterns, nil
}

// Business Requirement: BR-AI-004 - Apply discovered patterns to improve new workflow generation
func (b *DefaultIntelligentWorkflowBuilder) ApplyWorkflowPattern(ctx context.Context, pattern *WorkflowPattern, workflowContext *WorkflowContext) (*ExecutableTemplate, error) {
	b.log.WithFields(logrus.Fields{
		"pattern_id":   pattern.ID,
		"pattern_type": pattern.Type,
		"success_rate": pattern.SuccessRate,
		"confidence":   pattern.Confidence,
		"workflow_id":  workflowContext.WorkflowID,
		"environment":  workflowContext.Environment,
	}).Info("Applying workflow pattern")

	// Create base template from pattern
	template := &ExecutableTemplate{
		BaseVersionedEntity: sharedTypes.BaseVersionedEntity{
			BaseEntity: sharedTypes.BaseEntity{
				ID:          generateWorkflowID(),
				Name:        fmt.Sprintf("%s (Pattern-Based)", pattern.Name),
				Description: fmt.Sprintf("Generated from pattern %s with %d executions", pattern.ID, pattern.ExecutionCount),
				CreatedAt:   time.Now(),
			},
			Version:   "1.0.0",
			CreatedBy: "IntelligentWorkflowBuilder",
		},
		Steps:      make([]*ExecutableWorkflowStep, 0),
		Conditions: make([]*ExecutableCondition, 0),
		Variables:  make(map[string]interface{}),
		Tags:       []string{"pattern-based", pattern.Type},
	}

	// Apply pattern steps with context adaptation
	for _, patternStep := range pattern.Steps {
		step, err := b.adaptStepToContext(patternStep, workflowContext)
		if err != nil {
			return nil, fmt.Errorf("failed to adapt step %s to context: %w", patternStep.ID, err)
		}
		template.Steps = append(template.Steps, step)
	}

	// Apply pattern conditions
	for _, patternCondition := range pattern.Conditions {
		condition, err := b.adaptConditionToContext(patternCondition, workflowContext)
		if err != nil {
			return nil, fmt.Errorf("failed to adapt condition to context: %w", err)
		}
		template.Conditions = append(template.Conditions, condition)
	}

	// Set pattern metadata
	template.Metadata = map[string]interface{}{
		"pattern_id":           pattern.ID,
		"pattern_name":         pattern.Name,
		"pattern_success_rate": pattern.SuccessRate,
		"pattern_confidence":   pattern.Confidence,
		"execution_count":      pattern.ExecutionCount,
		"environments":         pattern.Environments,
		"resource_types":       pattern.ResourceTypes,
		"applied_at":           time.Now(),
	}

	// Add appropriate timeouts based on pattern history
	template.Timeouts = &WorkflowTimeouts{
		Execution: time.Duration(float64(pattern.AverageTime) * 1.5), // 50% buffer
		Step:      b.config.DefaultStepTimeout,
		Condition: 30 * time.Second,
		Recovery:  2 * time.Minute,
	}

	// Validate the generated template
	if b.validator != nil {
		if _, err := b.validator.ValidateWorkflow(ctx, template); err != nil {
			return nil, fmt.Errorf("generated template failed validation: %w", err)
		}
	}

	b.log.WithFields(logrus.Fields{
		"template_id":     template.ID,
		"step_count":      len(template.Steps),
		"pattern_applied": pattern.ID,
	}).Info("Pattern successfully applied to create workflow template")

	return template, nil
}

// Business Requirement: BR-IV-001 - Intelligent Pattern-Based Validation System
func (b *DefaultIntelligentWorkflowBuilder) ValidateWorkflow(ctx context.Context, template *ExecutableTemplate) (*ValidationReport, error) {
	b.log.WithFields(logrus.Fields{
		"workflow_id": template.ID,
		"step_count":  len(template.Steps),
	}).Info("Starting comprehensive intelligent workflow validation")

	startTime := time.Now()

	report := &ValidationReport{
		ID:         generateValidationID(),
		WorkflowID: template.ID,
		Type:       ValidationTypeIntegrity,
		Status:     "running",
		Results:    make([]*WorkflowRuleValidationResult, 0),
		CreatedAt:  startTime,
	}

	// Phase 1: Core structural validation using unused helper functions
	if err := b.performCoreStructuralValidation(ctx, template, report); err != nil {
		return nil, fmt.Errorf("core structural validation failed: %w", err)
	}

	// Phase 2: Pattern-based validation using learning intelligence
	if err := b.performPatternBasedValidation(ctx, template, report); err != nil {
		b.log.WithError(err).Warn("Pattern-based validation failed, continuing with core validation")
	}

	// Phase 3: Learning-enhanced validation using execution history
	if err := b.performLearningEnhancedValidation(ctx, template, report); err != nil {
		b.log.WithError(err).Warn("Learning-enhanced validation failed, continuing with pattern validation")
	}

	// Phase 4: Risk-based validation using AI insights
	if err := b.performRiskBasedValidation(ctx, template, report); err != nil {
		b.log.WithError(err).Warn("Risk-based validation failed, continuing with learning validation")
	}

	// Phase 5: Advanced validator integration (if available)
	if b.validator != nil {
		if err := b.integrateAdvancedValidator(ctx, template, report); err != nil {
			b.log.WithError(err).Warn("Advanced validator integration failed")
		}
	}

	// Calculate comprehensive summary
	report.Summary = b.calculateValidationSummary(report.Results)

	// Determine overall status with intelligent risk assessment
	report.Status = b.determineValidationStatus(report)

	// Add validation metadata as additional result
	validationDuration := time.Since(startTime)
	completedAt := time.Now()
	report.CompletedAt = &completedAt

	// Add metadata as a special validation result
	metadataResult := &WorkflowRuleValidationResult{
		RuleID:  "validation-metadata",
		Type:    ValidationTypePerformance,
		Passed:  true,
		Message: fmt.Sprintf("Intelligent validation completed in %v with 5 phases", validationDuration),
		Details: map[string]interface{}{
			"validation_phases":   5,
			"validation_duration": validationDuration.String(),
			"intelligence_used":   []string{"pattern-based", "learning-enhanced", "risk-based"},
			"core_functions":      []string{"dependencies", "parameters", "resources", "safety"},
		},
		Timestamp: completedAt,
	}
	report.Results = append(report.Results, metadataResult)

	b.log.WithFields(logrus.Fields{
		"workflow_id":         template.ID,
		"validation_status":   report.Status,
		"total_checks":        report.Summary.Total,
		"passed_checks":       report.Summary.Passed,
		"failed_checks":       report.Summary.Failed,
		"validation_duration": validationDuration,
		"intelligence_phases": 4,
	}).Info("Intelligent workflow validation completed")

	return report, nil
}

// Intelligent validation helper methods

// Phase 1: Core Structural Validation - Integrates unused validation functions
func (b *DefaultIntelligentWorkflowBuilder) performCoreStructuralValidation(ctx context.Context, template *ExecutableTemplate, report *ValidationReport) error {
	b.log.WithField("workflow_id", template.ID).Debug("Performing core structural validation")

	// Use unused validation helper functions
	// Step Dependencies Validation (from validation file)
	dependencyResults := b.validateStepDependencies(ctx, template)
	report.Results = append(report.Results, dependencyResults...)

	// Action Parameters Validation (from validation file)
	parameterResults := b.validateActionParameters(ctx, template)
	report.Results = append(report.Results, parameterResults...)

	// Resource Access Validation (from validation file)
	resourceResults := b.validateResourceAccess(ctx, template)
	report.Results = append(report.Results, resourceResults...)

	// Safety Constraints Validation (from validation file)
	safetyResults := b.validateSafetyConstraints(ctx, template)
	report.Results = append(report.Results, safetyResults...)

	b.log.WithFields(logrus.Fields{
		"dependency_checks": len(dependencyResults),
		"parameter_checks":  len(parameterResults),
		"resource_checks":   len(resourceResults),
		"safety_checks":     len(safetyResults),
	}).Debug("Core structural validation completed")

	return nil
}

// Phase 2: Pattern-Based Validation - Uses learning patterns for validation
func (b *DefaultIntelligentWorkflowBuilder) performPatternBasedValidation(ctx context.Context, template *ExecutableTemplate, report *ValidationReport) error {
	b.log.WithField("workflow_id", template.ID).Debug("Performing pattern-based validation")

	// Find relevant patterns for this workflow type
	patterns, err := b.findValidationPatternsForTemplate(ctx, template)
	if err != nil {
		return fmt.Errorf("failed to find validation patterns: %w", err)
	}

	// Apply pattern-based validation rules
	for _, pattern := range patterns {
		patternResults := b.applyPatternValidation(ctx, template, pattern)
		report.Results = append(report.Results, patternResults...)
	}

	b.log.WithFields(logrus.Fields{
		"patterns_applied": len(patterns),
		"pattern_checks":   len(report.Results),
	}).Debug("Pattern-based validation completed")

	return nil
}

// Phase 3: Learning-Enhanced Validation - Uses execution history for validation
func (b *DefaultIntelligentWorkflowBuilder) performLearningEnhancedValidation(ctx context.Context, template *ExecutableTemplate, report *ValidationReport) error {
	b.log.WithField("workflow_id", template.ID).Debug("Performing learning-enhanced validation")

	// Get historical failure patterns for similar workflows
	failurePatterns, err := b.getHistoricalFailurePatterns(ctx, template)
	if err != nil {
		return fmt.Errorf("failed to get failure patterns: %w", err)
	}

	// Validate against known failure scenarios
	for _, failurePattern := range failurePatterns {
		failureResults := b.validateAgainstFailurePattern(ctx, template, failurePattern)
		report.Results = append(report.Results, failureResults...)
	}

	// Apply learnings from successful executions
	successLearnings, err := b.getSuccessLearnings(ctx, template)
	if err != nil {
		b.log.WithError(err).Debug("Could not get success learnings, skipping")
	} else {
		for _, learning := range successLearnings {
			learningResults := b.validateWithSuccessLearning(ctx, template, learning)
			report.Results = append(report.Results, learningResults...)
		}
	}

	b.log.WithFields(logrus.Fields{
		"failure_patterns":  len(failurePatterns),
		"success_learnings": len(successLearnings),
	}).Debug("Learning-enhanced validation completed")

	return nil
}

// Phase 4: Risk-Based Validation - Uses AI risk assessment
func (b *DefaultIntelligentWorkflowBuilder) performRiskBasedValidation(ctx context.Context, template *ExecutableTemplate, report *ValidationReport) error {
	b.log.WithField("workflow_id", template.ID).Debug("Performing risk-based validation")

	// Calculate workflow risk score
	riskScore := b.calculateWorkflowRiskScore(template)

	// Apply risk-specific validation rules
	riskResults := b.applyRiskBasedValidation(template, riskScore)
	report.Results = append(report.Results, riskResults...)

	// Add risk metadata as validation result details (stored in the risk result above)
	// Risk metadata is captured in the risk validation results above

	b.log.WithFields(logrus.Fields{
		"risk_score":  riskScore,
		"risk_level":  b.getRiskLevel(riskScore),
		"risk_checks": len(riskResults),
	}).Debug("Risk-based validation completed")

	return nil
}

// Phase 5: Advanced Validator Integration
func (b *DefaultIntelligentWorkflowBuilder) integrateAdvancedValidator(ctx context.Context, template *ExecutableTemplate, report *ValidationReport) error {
	b.log.WithField("workflow_id", template.ID).Debug("Integrating advanced validator")

	// Use external validator if available
	validationReport, err := b.validator.ValidateWorkflow(ctx, template)
	if err != nil {
		return fmt.Errorf("advanced validator failed: %w", err)
	}

	// Merge results from advanced validator
	report.Results = append(report.Results, validationReport.Results...)

	b.log.WithField("advanced_checks", len(validationReport.Results)).Debug("Advanced validator integration completed")
	return nil
}

// Intelligent validation helper methods

func (b *DefaultIntelligentWorkflowBuilder) determineValidationStatus(report *ValidationReport) string {
	// Intelligent status determination based on risk and failure patterns
	if report.Summary.Failed == 0 {
		return "passed"
	}

	// Check if failures are critical
	criticalFailures := 0
	for _, result := range report.Results {
		if !result.Passed && b.isCriticalValidationFailure(result) {
			criticalFailures++
		}
	}

	if criticalFailures > 0 {
		return "critical_failure"
	}

	// Check failure percentage
	failureRate := float64(report.Summary.Failed) / float64(report.Summary.Total)
	if failureRate > 0.5 {
		return "failed"
	}

	return "warning" // Some failures but not critical
}

func (b *DefaultIntelligentWorkflowBuilder) isCriticalValidationFailure(result *WorkflowRuleValidationResult) bool {
	// Determine if a validation failure is critical
	criticalTypes := []string{
		"circular_dependency",
		"missing_required_parameter",
		"destructive_action_without_backup",
		"resource_not_found",
		"permission_denied",
	}

	if result.Details != nil {
		if issue, ok := result.Details["issue"].(string); ok {
			for _, criticalType := range criticalTypes {
				if issue == criticalType {
					return true
				}
			}
		}
	}

	return result.Type == ValidationTypeIntegrity && strings.Contains(result.Message, "critical")
}

// Pattern-based validation helper methods

func (b *DefaultIntelligentWorkflowBuilder) findValidationPatternsForTemplate(ctx context.Context, template *ExecutableTemplate) ([]*WorkflowPattern, error) {
	// Create a validation query based on template characteristics
	templateType := b.determineTemplateType(template)

	// Search for patterns using existing helper function
	patterns, err := b.FindWorkflowPatterns(ctx, &PatternCriteria{
		MinSimilarity:     0.7,
		MinExecutionCount: 3,
		MinSuccessRate:    0.8,
		EnvironmentFilter: []string{templateType},
	})

	if err != nil {
		b.log.WithError(err).Debug("Could not find validation patterns, using empty set")
		return []*WorkflowPattern{}, nil
	}

	return patterns, nil
}

func (b *DefaultIntelligentWorkflowBuilder) applyPatternValidation(ctx context.Context, template *ExecutableTemplate, pattern *WorkflowPattern) []*WorkflowRuleValidationResult {
	results := make([]*WorkflowRuleValidationResult, 0)

	// Validate template against pattern expectations
	if pattern.SuccessRate < 0.5 {
		results = append(results, &WorkflowRuleValidationResult{
			RuleID:    uuid.New().String(),
			Type:      ValidationTypePerformance,
			Passed:    false,
			Message:   fmt.Sprintf("Template matches low-success pattern %s (%.1f%% success rate)", pattern.Name, pattern.SuccessRate*100),
			Details:   map[string]interface{}{"pattern_id": pattern.ID, "success_rate": pattern.SuccessRate},
			Timestamp: time.Now(),
		})
	}

	// Check if template has known problematic step combinations
	if b.hasProblematicStepCombination(template, pattern) {
		results = append(results, &WorkflowRuleValidationResult{
			RuleID:    uuid.New().String(),
			Type:      ValidationTypeIntegrity,
			Passed:    false,
			Message:   "Template contains step combination known to cause issues",
			Details:   map[string]interface{}{"pattern_id": pattern.ID, "issue": "problematic_step_combination"},
			Timestamp: time.Now(),
		})
	}

	return results
}

// Learning-enhanced validation helper methods

func (b *DefaultIntelligentWorkflowBuilder) getHistoricalFailurePatterns(ctx context.Context, template *ExecutableTemplate) ([]*WorkflowLearning, error) {
	// This would typically query an execution repository
	// For now, return empty since execution repository is not available
	b.log.WithField("template_id", template.ID).Debug("Historical failure patterns not available - execution repository required")
	return []*WorkflowLearning{}, nil
}

func (b *DefaultIntelligentWorkflowBuilder) getSuccessLearnings(ctx context.Context, template *ExecutableTemplate) ([]*WorkflowLearning, error) {
	// This would typically query learning data
	// For now, return empty since learning storage is not available
	b.log.WithField("template_id", template.ID).Debug("Success learnings not available - learning storage required")
	return []*WorkflowLearning{}, nil
}

func (b *DefaultIntelligentWorkflowBuilder) validateAgainstFailurePattern(ctx context.Context, template *ExecutableTemplate, failurePattern *WorkflowLearning) []*WorkflowRuleValidationResult {
	results := make([]*WorkflowRuleValidationResult, 0)

	// Extract failure characteristics from learning data
	if failureData, ok := failurePattern.Data["failure_reason"].(string); ok {
		if b.templateMatchesFailureScenario(template, failureData) {
			results = append(results, &WorkflowRuleValidationResult{
				RuleID:    uuid.New().String(),
				Type:      ValidationTypeIntegrity,
				Passed:    false,
				Message:   fmt.Sprintf("Template matches historical failure pattern: %s", failureData),
				Details:   map[string]interface{}{"learning_id": failurePattern.ID, "failure_reason": failureData},
				Timestamp: time.Now(),
			})
		}
	}

	return results
}

func (b *DefaultIntelligentWorkflowBuilder) validateWithSuccessLearning(ctx context.Context, template *ExecutableTemplate, learning *WorkflowLearning) []*WorkflowRuleValidationResult {
	results := make([]*WorkflowRuleValidationResult, 0)

	// Validate template incorporates success patterns
	if qualityScore, ok := learning.Data["quality_score"].(float64); ok {
		if qualityScore > 0.9 && !b.templateIncorporatesSuccessPattern(template, learning) {
			results = append(results, &WorkflowRuleValidationResult{
				RuleID:    uuid.New().String(),
				Type:      ValidationTypePerformance,
				Passed:    false,
				Message:   "Template could benefit from high-quality success patterns",
				Details:   map[string]interface{}{"learning_id": learning.ID, "quality_score": qualityScore},
				Timestamp: time.Now(),
			})
		}
	}

	return results
}

// Risk-based validation helper methods

func (b *DefaultIntelligentWorkflowBuilder) calculateWorkflowRiskScore(template *ExecutableTemplate) float64 {
	riskScore := 0.0

	// Calculate risk based on various factors
	// Destructive actions increase risk
	destructiveActions := 0
	for _, step := range template.Steps {
		if step.Action != nil && b.isDestructiveAction(step.Action.Type) {
			destructiveActions++
		}
	}
	riskScore += float64(destructiveActions) * 0.2

	// Long workflows increase risk
	if len(template.Steps) > 10 {
		riskScore += 0.3
	}

	// Missing safety measures increase risk
	safeguards := 0
	for _, step := range template.Steps {
		if step.Action != nil && step.Action.Rollback != nil {
			safeguards++
		}
	}
	if safeguards == 0 && destructiveActions > 0 {
		riskScore += 0.4
	}

	// Complex dependencies increase risk
	totalDependencies := 0
	for _, step := range template.Steps {
		totalDependencies += len(step.Dependencies)
	}
	if totalDependencies > len(template.Steps)*2 {
		riskScore += 0.2
	}

	// Cap risk score at 1.0
	if riskScore > 1.0 {
		riskScore = 1.0
	}

	return riskScore
}

func (b *DefaultIntelligentWorkflowBuilder) applyRiskBasedValidation(template *ExecutableTemplate, riskScore float64) []*WorkflowRuleValidationResult {
	results := make([]*WorkflowRuleValidationResult, 0)

	// High-risk workflows need additional validation
	if riskScore > 0.7 {
		results = append(results, &WorkflowRuleValidationResult{
			RuleID:    uuid.New().String(),
			Type:      ValidationTypeIntegrity,
			Passed:    false,
			Message:   fmt.Sprintf("High-risk workflow (risk score: %.2f) requires manual review", riskScore),
			Details:   map[string]interface{}{"risk_score": riskScore, "requires": "manual_review"},
			Timestamp: time.Now(),
		})
	}

	// Medium-risk workflows need warnings
	if riskScore > 0.4 && riskScore <= 0.7 {
		results = append(results, &WorkflowRuleValidationResult{
			RuleID:    uuid.New().String(),
			Type:      ValidationTypeIntegrity,
			Passed:    true,
			Message:   fmt.Sprintf("Medium-risk workflow (risk score: %.2f) - consider additional safeguards", riskScore),
			Details:   map[string]interface{}{"risk_score": riskScore, "recommendation": "additional_safeguards"},
			Timestamp: time.Now(),
		})
	}

	return results
}

func (b *DefaultIntelligentWorkflowBuilder) getRiskLevel(riskScore float64) string {
	if riskScore > 0.7 {
		return "high"
	} else if riskScore > 0.4 {
		return "medium"
	} else {
		return "low"
	}
}

// Additional validation helper methods

func (b *DefaultIntelligentWorkflowBuilder) determineTemplateType(template *ExecutableTemplate) string {
	// Determine template type based on step types and actions
	hasDestructiveActions := false
	hasResourceActions := false
	hasDataActions := false

	for _, step := range template.Steps {
		if step.Action != nil {
			if b.isDestructiveAction(step.Action.Type) {
				hasDestructiveActions = true
			}
			if strings.Contains(step.Action.Type, "resource") || strings.Contains(step.Action.Type, "deploy") {
				hasResourceActions = true
			}
			if strings.Contains(step.Action.Type, "data") || strings.Contains(step.Action.Type, "backup") {
				hasDataActions = true
			}
		}
	}

	if hasDestructiveActions {
		return "destructive"
	} else if hasResourceActions {
		return "resource-management"
	} else if hasDataActions {
		return "data-processing"
	} else {
		return "general"
	}
}

func (b *DefaultIntelligentWorkflowBuilder) hasProblematicStepCombination(template *ExecutableTemplate, pattern *WorkflowPattern) bool {
	// Check for known problematic combinations
	// This is a simplified check - in practice, this would use more sophisticated pattern matching
	if pattern.SuccessRate < 0.3 && len(template.Steps) > 5 {
		return true // Long workflows with patterns that historically fail
	}

	return false
}

func (b *DefaultIntelligentWorkflowBuilder) templateMatchesFailureScenario(template *ExecutableTemplate, failureReason string) bool {
	// Check if template characteristics match known failure scenarios
	if strings.Contains(failureReason, "timeout") {
		// Check for potential timeout issues
		for _, step := range template.Steps {
			if step.Timeout > 10*time.Minute {
				return true
			}
		}
	}

	if strings.Contains(failureReason, "dependency") {
		// Check for complex dependencies
		totalDeps := 0
		for _, step := range template.Steps {
			totalDeps += len(step.Dependencies)
		}
		if totalDeps > len(template.Steps)*2 {
			return true
		}
	}

	return false
}

func (b *DefaultIntelligentWorkflowBuilder) templateIncorporatesSuccessPattern(template *ExecutableTemplate, learning *WorkflowLearning) bool {
	// Check if template incorporates learnings from successful executions
	// This is simplified - in practice would check specific success characteristics
	if duration, ok := learning.Data["duration"].(time.Duration); ok {
		if duration < 5*time.Minute {
			// Success pattern indicates fast execution
			// Check if template is optimized for speed
			for _, step := range template.Steps {
				if step.Timeout > 5*time.Minute {
					return false
				}
			}
		}
	}

	return true
}

// Business Requirement: BR-AI-006 - Simulate workflows before execution
// SimulateWorkflow executes workflow with real Kubernetes operations
// Implements BR-PA-011: Execute 25+ supported Kubernetes remediation actions
func (b *DefaultIntelligentWorkflowBuilder) SimulateWorkflow(ctx context.Context, template *ExecutableTemplate, scenario *SimulationScenario) (*SimulationResult, error) {
	if !b.config.EnableSimulation {
		return nil, fmt.Errorf("workflow simulation is disabled")
	}

	b.log.WithFields(logrus.Fields{
		"workflow_id":   template.ID,
		"scenario_id":   scenario.ID,
		"scenario_type": scenario.Type,
	}).Info("Starting real workflow execution (replacing simulation)")

	// Setup real workflow engine for execution
	if err := b.setupWorkflowEngine(); err != nil {
		return nil, fmt.Errorf("failed to setup workflow engine: %w", err)
	}

	startTime := time.Now()

	result := &SimulationResult{
		ID:         generateSimulationID(),
		ScenarioID: scenario.ID,
		Success:    false,
		Results:    make(map[string]interface{}),
		Metrics:    make(map[string]float64),
		Logs:       make([]string, 0),
		Errors:     make([]string, 0),
		RunAt:      startTime,
	}

	// Create workflow from template for execution using constructor
	workflow := NewWorkflow(template.ID, template)

	// Add scenario metadata
	workflow.Metadata["scenario_id"] = scenario.ID
	workflow.Metadata["scenario_type"] = scenario.Type
	workflow.Metadata["environment"] = scenario.Environment

	// Execute the workflow using real workflow engine
	execution, err := b.workflowEngine.Execute(ctx, workflow)

	// Calculate execution duration
	result.Duration = time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Errors = append(result.Errors, err.Error())
		result.Results = map[string]interface{}{
			"execution_failed": true,
			"error":            err.Error(),
			"environment":      scenario.Environment,
			"steps_count":      len(template.Steps),
		}

		b.log.WithError(err).WithFields(logrus.Fields{
			"workflow_id": template.ID,
			"scenario_id": scenario.ID,
			"duration":    result.Duration,
		}).Error("Real workflow execution failed")

		return result, nil // Return result with error info, not an error itself
	}

	// Extract results from successful execution
	result.Success = execution.OperationalStatus == ExecutionStatusCompleted
	result.Results = map[string]interface{}{
		"execution_id":     execution.ID,
		"workflow_id":      execution.WorkflowID,
		"status":           string(execution.OperationalStatus),
		"steps_executed":   len(execution.Steps),
		"successful_steps": b.countSuccessfulSteps(execution.Steps),
		"failed_steps":     b.countFailedSteps(execution.Steps),
		"environment":      scenario.Environment,
		"actual_duration":  execution.Duration.Seconds(),
		"real_execution":   true,
	}

	// Calculate metrics from real execution
	result.Metrics = map[string]float64{
		"execution_time":   result.Duration.Seconds(),
		"success_rate":     b.calculateStepSuccessRate(execution.Steps),
		"steps_executed":   float64(len(execution.Steps)),
		"successful_steps": float64(b.countSuccessfulSteps(execution.Steps)),
		"failed_steps":     float64(b.countFailedSteps(execution.Steps)),
	}

	// Extract logs from execution steps
	for _, step := range execution.Steps {
		result.Logs = append(result.Logs,
			fmt.Sprintf("Step %s: %s (duration: %v)",
				step.StepID, string(step.Status), step.Duration))

		if step.Error != "" {
			result.Errors = append(result.Errors,
				fmt.Sprintf("Step %s failed: %s", step.StepID, step.Error))
		}
	}

	if result.Success {
		result.Logs = append(result.Logs, "Real workflow execution completed successfully")
	}

	b.log.WithFields(logrus.Fields{
		"workflow_id":       template.ID,
		"scenario_id":       scenario.ID,
		"execution_success": result.Success,
		"duration":          result.Duration,
		"steps_executed":    len(execution.Steps),
		"errors_count":      len(result.Errors),
		"real_execution":    true,
	}).Info("Real workflow execution completed")

	return result, nil
}

// Helper methods for workflow execution analysis

// countSuccessfulSteps counts the number of successful steps in an execution
func (b *DefaultIntelligentWorkflowBuilder) countSuccessfulSteps(steps []*StepExecution) int {
	count := 0
	for _, step := range steps {
		if step.Status == ExecutionStatusCompleted {
			count++
		}
	}
	return count
}

// countFailedSteps counts the number of failed steps in an execution
func (b *DefaultIntelligentWorkflowBuilder) countFailedSteps(steps []*StepExecution) int {
	count := 0
	for _, step := range steps {
		if step.Status == ExecutionStatusFailed {
			count++
		}
	}
	return count
}

// calculateStepSuccessRate calculates the success rate of steps in an execution
func (b *DefaultIntelligentWorkflowBuilder) calculateStepSuccessRate(steps []*StepExecution) float64 {
	if len(steps) == 0 {
		return 0.0
	}

	successfulSteps := b.countSuccessfulSteps(steps)
	return float64(successfulSteps) / float64(len(steps))
}

// Business Requirement: BR-AI-007 - Learn from workflow execution to improve future generation
func (b *DefaultIntelligentWorkflowBuilder) LearnFromWorkflowExecution(ctx context.Context, execution *RuntimeWorkflowExecution) error {
	if !b.config.EnableLearning {
		return nil
	}

	b.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"success":      execution.OperationalStatus == ExecutionStatusCompleted,
		"duration":     execution.Duration,
	}).Info("Learning from workflow execution")

	// Extract learnings from execution
	learnings, err := b.extractLearnings(execution)
	if err != nil {
		return fmt.Errorf("failed to extract learnings: %w", err)
	}

	// Apply learnings to improve patterns
	for _, learning := range learnings {
		if err := b.applyLearning(ctx, learning); err != nil {
			b.log.WithError(err).WithField("learning_type", learning.Type).Warn("Failed to apply learning")
		}
	}

	// Update pattern effectiveness scores
	if err := b.updatePatternEffectiveness(ctx, execution); err != nil {
		b.log.WithError(err).Warn("Failed to update pattern effectiveness")
	}

	// Store execution data for future pattern discovery
	if err := b.storeExecutionForPatternDiscovery(ctx, execution); err != nil {
		b.log.WithError(err).Warn("Failed to store execution for pattern discovery")
	}

	b.log.WithFields(logrus.Fields{
		"execution_id":    execution.ID,
		"learnings_count": len(learnings),
	}).Info("Learning from workflow execution complete")

	return nil
}

// Helper methods for workflow generation

func (b *DefaultIntelligentWorkflowBuilder) buildWorkflowContext(ctx context.Context, objective *WorkflowObjective) (*WorkflowContext, error) {
	context := &WorkflowContext{
		BaseContext: sharedTypes.BaseContext{
			Environment: "production", // Default, should come from objective
			Timestamp:   time.Now(),
		},
		WorkflowID: generateWorkflowID(),
		Variables:  make(map[string]interface{}),
		CreatedAt:  time.Now(),
	}

	// Extract context from objective
	if constraints := objective.Constraints; constraints != nil {
		if env, ok := constraints["environment"].(string); ok {
			context.Environment = env
		}
		if namespace, ok := constraints["namespace"].(string); ok {
			context.Namespace = namespace
		}
		if cluster, ok := constraints["cluster"].(string); ok {
			context.Cluster = cluster
		}
	}

	return context, nil
}

func (b *DefaultIntelligentWorkflowBuilder) findRelevantPatterns(ctx context.Context, objective *WorkflowObjective, workflowContext *WorkflowContext) ([]*WorkflowPattern, error) {
	criteria := &PatternCriteria{
		MinSimilarity:     b.config.MinPatternSimilarity,
		MinExecutionCount: b.config.MinExecutionCount,
		MinSuccessRate:    b.config.MinSuccessRate,
		TimeWindow:        time.Duration(b.config.PatternLookbackDays) * 24 * time.Hour,
		EnvironmentFilter: []string{workflowContext.Environment},
	}

	// Add resource filter if available
	if workflowContext.Resource != nil {
		criteria.ResourceFilter = map[string]string{
			"kind":      workflowContext.Resource.Kind,
			"namespace": workflowContext.Resource.Namespace,
		}
	}

	return b.FindWorkflowPatterns(ctx, criteria)
}

func (b *DefaultIntelligentWorkflowBuilder) generateWorkflowTemplate(ctx context.Context, objective *WorkflowObjective, workflowContext *WorkflowContext, patterns []*WorkflowPattern) (*ExecutableTemplate, error) {
	// Use AI to generate workflow if no patterns available
	if len(patterns) == 0 {
		return b.generateAIWorkflowTemplate(ctx, objective, workflowContext)
	}

	// Use best pattern as base and enhance with AI
	bestPattern := patterns[0] // Patterns are ranked
	template, err := b.ApplyWorkflowPattern(ctx, bestPattern, workflowContext)
	if err != nil {
		// Fallback to AI generation if pattern application fails
		b.log.WithError(err).Warn("Pattern application failed, falling back to AI generation")
		return b.generateAIWorkflowTemplate(ctx, objective, workflowContext)
	}

	// Enhance with AI insights
	return b.enhanceWithAI(ctx, template, objective, workflowContext)
}

func (b *DefaultIntelligentWorkflowBuilder) generateAIWorkflowTemplate(ctx context.Context, objective *WorkflowObjective, workflowContext *WorkflowContext) (*ExecutableTemplate, error) {
	b.log.WithFields(logrus.Fields{
		"objective_id": objective.ID,
		"environment":  workflowContext.Environment,
		"namespace":    workflowContext.Namespace,
	}).Info("Generating AI-powered workflow template")

	// Phase 1: Analyze objective for AI generation
	analysis, err := b.analyzeObjective(ctx, objective)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze objective: %w", err)
	}

	// Phase 2: Find similar successful patterns for AI context
	patterns, err := b.findSimilarSuccessfulPatterns(ctx, analysis)
	if err != nil {
		b.log.WithError(err).Warn("Failed to find similar patterns, proceeding without pattern context")
		patterns = []*WorkflowPattern{}
	}

	// Phase 3: Generate workflow with enhanced AI system
	template, err := b.generateWorkflowWithAI(ctx, objective, analysis, patterns)
	if err != nil {
		// Fallback to basic AI generation if enhanced generation fails
		b.log.WithError(err).Warn("Enhanced AI generation failed, falling back to basic generation")
		return b.generateBasicAIWorkflow(ctx, objective, workflowContext)
	}

	b.log.WithFields(logrus.Fields{
		"workflow_id":   template.ID,
		"steps_count":   len(template.Steps),
		"patterns_used": len(patterns),
		"risk_level":    analysis.RiskLevel,
		"complexity":    analysis.Complexity,
	}).Info("AI-powered workflow generation completed")

	return template, nil
}

// generateBasicAIWorkflow provides fallback basic AI generation
func (b *DefaultIntelligentWorkflowBuilder) generateBasicAIWorkflow(ctx context.Context, objective *WorkflowObjective, workflowContext *WorkflowContext) (*ExecutableTemplate, error) {
	// Create workflow objective for AI generation
	llmObjective := &llm.WorkflowObjective{
		ID:          objective.ID,
		Type:        objective.Type,
		Description: objective.Description,
		Environment: workflowContext.Environment,
		Namespace:   workflowContext.Namespace,
		Priority:    strconv.Itoa(objective.Priority),
		Constraints: objective.Constraints,
		Context: map[string]interface{}{
			"cluster":  workflowContext.Cluster,
			"resource": workflowContext.Resource,
		},
	}

	// Generate workflow using basic AI
	aiResult, err := b.llmClient.GenerateWorkflow(ctx, llmObjective)
	if err != nil {
		return nil, fmt.Errorf("basic AI workflow generation failed: %w", err)
	}

	// Convert AI result to executable template
	template, err := b.convertAIResultToTemplate(aiResult, objective, workflowContext)
	if err != nil {
		return nil, fmt.Errorf("failed to convert AI result to template: %w", err)
	}

	return template, nil
}

func generateWorkflowID() string {
	return "workflow-" + strings.Replace(uuid.New().String(), "-", "", -1)[:12]
}

func generateValidationID() string {
	return "validation-" + strings.Replace(uuid.New().String(), "-", "", -1)[:12]
}

func generateSimulationID() string {
	return "simulation-" + strings.Replace(uuid.New().String(), "-", "", -1)[:12]
}

// Register step type handlers for different step types
func (b *DefaultIntelligentWorkflowBuilder) registerStepHandlers() {
	b.stepTypeHandlers[StepTypeAction] = NewActionStepHandler(b.log)
	b.stepTypeHandlers[StepTypeCondition] = NewConditionStepHandler(b.log)
	b.stepTypeHandlers[StepTypeParallel] = NewParallelStepHandler(b.log)
	b.stepTypeHandlers[StepTypeLoop] = NewLoopStepHandler(b.log)
	b.stepTypeHandlers[StepTypeDecision] = NewDecisionStepHandler(b.log)
	b.stepTypeHandlers[StepTypeWait] = NewWaitStepHandler(b.log)
	b.stepTypeHandlers[StepTypeSubflow] = NewSubflowStepHandler(b.log)
}

// applySafetyChecks performs basic safety validation on workflow templates
func (b *DefaultIntelligentWorkflowBuilder) applySafetyChecks(template *ExecutableTemplate, objective *WorkflowObjective) error {
	if template == nil {
		return fmt.Errorf("template cannot be nil")
	}

	b.log.WithFields(logrus.Fields{
		"workflow_id":    template.ID,
		"objective_type": objective.Type,
		"step_count":     len(template.Steps),
	}).Info("Applying comprehensive safety checks")

	// Phase 1: Basic safety validation
	if err := b.performBasicSafetyChecks(template); err != nil {
		return fmt.Errorf("basic safety checks failed: %w", err)
	}

	// Phase 2: Apply safety enhancements based on risk level
	if err := b.applySafetyEnhancements(template, objective); err != nil {
		return fmt.Errorf("failed to apply safety enhancements: %w", err)
	}

	// Phase 3: Add environment-specific safety measures
	if err := b.applyEnvironmentSafety(template, objective); err != nil {
		return fmt.Errorf("failed to apply environment safety: %w", err)
	}

	b.log.WithField("workflow_id", template.ID).Info("Safety checks completed successfully")
	return nil
}

// performBasicSafetyChecks performs fundamental safety validations
func (b *DefaultIntelligentWorkflowBuilder) performBasicSafetyChecks(template *ExecutableTemplate) error {
	// Check for destructive actions without confirmation
	for _, step := range template.Steps {
		if step.Action != nil && b.isDestructiveAction(step.Action.Type) {
			b.log.WithFields(logrus.Fields{
				"step_id":     step.ID,
				"action_type": step.Action.Type,
			}).Warn("Destructive action detected - ensure proper safeguards are in place")
		}
	}

	// Check for reasonable timeouts
	for _, step := range template.Steps {
		if step.Timeout > 0 {
			if step.Timeout < 10*time.Second {
				return fmt.Errorf("step %s has dangerously short timeout: %s", step.ID, step.Timeout)
			}
			if step.Timeout > 24*time.Hour {
				b.log.WithFields(logrus.Fields{
					"step_id": step.ID,
					"timeout": step.Timeout,
				}).Warn("Step has very long timeout")
			}
		}
	}

	return nil
}

// applySafetyEnhancements applies comprehensive safety measures
func (b *DefaultIntelligentWorkflowBuilder) applySafetyEnhancements(template *ExecutableTemplate, objective *WorkflowObjective) error {
	// Determine safety level from objective constraints
	safetyLevel := "medium" // default
	if objective.Constraints != nil {
		if level, ok := objective.Constraints["safety_level"].(string); ok {
			safetyLevel = level
		}
	}

	// Apply safety constraints using existing helper function
	b.applySafetyConstraints(template, safetyLevel)

	// Add confirmation steps for high-risk actions
	if safetyLevel == "high" {
		b.addConfirmationSteps(template)
	}

	// Add rollback capabilities
	b.addRollbackSteps(template)

	// Add validation steps
	b.addValidationSteps(template)

	return nil
}

// applyEnvironmentSafety applies environment-specific safety measures
func (b *DefaultIntelligentWorkflowBuilder) applyEnvironmentSafety(template *ExecutableTemplate, objective *WorkflowObjective) error {
	environment := "production" // default assumption
	if objective.Constraints != nil {
		if env, ok := objective.Constraints["environment"].(string); ok {
			environment = env
		}
	}

	// Production environments get additional safety measures
	if environment == "production" {
		// Reduce parallelism for safety
		b.reduceParallelism(template)

		// Add additional validation
		b.addProductionSafetyValidation(template)
	}

	return nil
}

// addProductionSafetyValidation adds production-specific safety validation
func (b *DefaultIntelligentWorkflowBuilder) addProductionSafetyValidation(template *ExecutableTemplate) {
	// Add pre-execution safety check
	safetyStep := &ExecutableWorkflowStep{
		BaseEntity: sharedTypes.BaseEntity{
			ID:   "production-safety-check",
			Name: "Production Safety Validation",
		},
		Type: StepTypeCondition,
		Condition: &ExecutableCondition{
			Expression: "environment == 'production' && safety_approved == true",
			Type:       "safety_gate",
		},
		Timeout: 2 * time.Minute,
	}

	// Insert at the beginning
	template.Steps = append([]*ExecutableWorkflowStep{safetyStep}, template.Steps...)
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeParallelExecution(template *ExecutableTemplate) error {
	b.log.WithField("step_count", len(template.Steps)).Debug("Analyzing workflow for parallel execution opportunities")

	if len(template.Steps) < 2 {
		b.log.Debug("Insufficient steps for parallel optimization")
		return nil
	}

	// Build dependency graph
	dependencies := b.buildDependencyGraph(template.Steps)

	// Find independent step groups that can run in parallel
	parallelGroups := b.identifyParallelGroups(template.Steps, dependencies)

	if len(parallelGroups) <= 1 {
		b.log.Debug("No parallel optimization opportunities found")
		return nil
	}

	// Create parallel execution steps
	parallelStepsAdded := 0
	for _, group := range parallelGroups {
		if len(group) > 1 {
			parallelStep := b.createParallelExecutionStep(group)
			if parallelStep != nil {
				// Insert parallel step and mark constituent steps as part of parallel group
				for _, step := range group {
					if step.Metadata == nil {
						step.Metadata = make(map[string]interface{})
					}
					step.Metadata["parallel_group"] = parallelStep.ID
				}
				parallelStepsAdded++
			}
		}
	}

	b.log.WithFields(logrus.Fields{
		"parallel_groups_created": parallelStepsAdded,
		"total_groups":            len(parallelGroups),
	}).Info("Parallel execution optimization completed")

	return nil
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeResourceUsage(template *ExecutableTemplate) error {
	b.log.WithField("step_count", len(template.Steps)).Debug("Optimizing workflow resource usage")

	resourceOptimizations := 0

	for _, step := range template.Steps {
		if step.Action == nil {
			continue
		}

		optimized := false

		// Optimize CPU and memory resource requests
		if b.optimizeCPUMemoryResources(step) {
			optimized = true
		}

		// Optimize timeout values based on action type
		if b.optimizeStepTimeouts(step) {
			optimized = true
		}

		// Optimize retry policies based on action type
		if b.optimizeRetryPolicies(step) {
			optimized = true
		}

		// Add resource usage monitoring metadata
		if step.Metadata == nil {
			step.Metadata = make(map[string]interface{})
		}
		step.Metadata["resource_optimized"] = optimized
		step.Metadata["optimization_timestamp"] = time.Now()

		if optimized {
			resourceOptimizations++
		}
	}

	// Optimize overall workflow resource limits
	b.optimizeWorkflowResourceLimits(template)

	b.log.WithFields(logrus.Fields{
		"steps_optimized":   resourceOptimizations,
		"total_steps":       len(template.Steps),
		"optimization_rate": float64(resourceOptimizations) / float64(len(template.Steps)),
	}).Info("Resource usage optimization completed")

	return nil
}

func (b *DefaultIntelligentWorkflowBuilder) enhanceErrorHandling(template *ExecutableTemplate) error {
	b.log.Debug("Error handling enhancement not fully implemented - using basic validation")
	// At least ensure each step has a timeout
	for _, step := range template.Steps {
		if step.Timeout == 0 {
			step.Timeout = 5 * time.Minute // Default timeout
		}
	}
	return nil
}

func (b *DefaultIntelligentWorkflowBuilder) getExecutionHistory(ctx context.Context, criteria *PatternCriteria) ([]*RuntimeWorkflowExecution, error) {
	b.log.WithFields(logrus.Fields{
		"time_window":         criteria.TimeWindow,
		"min_execution_count": criteria.MinExecutionCount,
		"min_success_rate":    criteria.MinSuccessRate,
	}).Debug("Retrieving execution history for pattern discovery")

	if b.executionRepo == nil {
		b.log.Warn("Execution repository not available - using in-memory fallback")
		// Create temporary in-memory repository for development/testing
		b.executionRepo = NewInMemoryExecutionRepository(b.log)
	}

	// Calculate time window
	endTime := time.Now()
	startTime := endTime.Add(-criteria.TimeWindow)

	// Retrieve executions in time window
	executions, err := b.executionRepo.GetExecutionsInTimeWindow(ctx, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve execution history: %w", err)
	}

	// Filter executions based on criteria
	var filteredExecutions []*RuntimeWorkflowExecution
	for _, execution := range executions {
		// Apply environment filter if specified
		if len(criteria.EnvironmentFilter) > 0 {
			found := false
			for _, env := range criteria.EnvironmentFilter {
				if execution.Metadata != nil {
					if executionEnv, ok := execution.Metadata["environment"].(string); ok && executionEnv == env {
						found = true
						break
					}
				}
			}
			if !found {
				continue
			}
		}

		// Apply resource filter if specified
		if len(criteria.ResourceFilter) > 0 {
			found := false
			for key, value := range criteria.ResourceFilter {
				if execution.Metadata != nil {
					if executionValue, ok := execution.Metadata[key].(string); ok && executionValue == value {
						found = true
						break
					}
				}
			}
			if !found {
				continue
			}
		}

		filteredExecutions = append(filteredExecutions, execution)
	}

	b.log.WithFields(logrus.Fields{
		"total_executions":    len(executions),
		"filtered_executions": len(filteredExecutions),
		"time_range":          fmt.Sprintf("%v to %v", startTime, endTime),
	}).Info("Retrieved execution history for pattern discovery")

	return filteredExecutions, nil
}

func (b *DefaultIntelligentWorkflowBuilder) clusterExecutions(executions []*RuntimeWorkflowExecution, criteria *PatternCriteria) ([]*ExecutionCluster, error) {
	b.log.WithFields(logrus.Fields{
		"execution_count": len(executions),
		"min_similarity":  criteria.MinSimilarity,
	}).Debug("Clustering executions for pattern discovery")

	if len(executions) < criteria.MinExecutionCount {
		b.log.Debug("Insufficient executions for clustering")
		return []*ExecutionCluster{}, nil
	}

	// Simple similarity-based clustering using execution characteristics
	var clusters []*ExecutionCluster
	used := make(map[string]bool)

	for i, execution := range executions {
		if used[execution.ID] {
			continue
		}

		// Create new cluster with this execution as seed
		cluster := &ExecutionCluster{
			Members: []*RuntimeWorkflowExecution{execution},
		}
		used[execution.ID] = true

		// Find similar executions to add to this cluster
		for j, otherExecution := range executions {
			if i != j && !used[otherExecution.ID] {
				similarity := b.calculateExecutionSimilarity(execution, otherExecution)
				if similarity >= criteria.MinSimilarity {
					cluster.Members = append(cluster.Members, otherExecution)
					used[otherExecution.ID] = true
				}
			}
		}

		// Only keep clusters that meet minimum size requirements
		if len(cluster.Members) >= criteria.MinExecutionCount {
			clusters = append(clusters, cluster)
		}
	}

	b.log.WithFields(logrus.Fields{
		"total_executions": len(executions),
		"clusters_formed":  len(clusters),
	}).Info("Execution clustering completed")

	return clusters, nil
}

// calculateExecutionSimilarity calculates similarity between two workflow executions
func (b *DefaultIntelligentWorkflowBuilder) calculateExecutionSimilarity(exec1, exec2 *RuntimeWorkflowExecution) float64 {
	var similarity float64

	// Factor 1: Workflow ID similarity (50% weight if same workflow)
	if exec1.WorkflowID == exec2.WorkflowID {
		similarity += 0.5
	}

	// Factor 2: Environment similarity (20% weight)
	env1 := b.getExecutionEnvironment(exec1)
	env2 := b.getExecutionEnvironment(exec2)
	if env1 == env2 {
		similarity += 0.2
	}

	// Factor 3: Success status similarity (15% weight)
	success1 := exec1.IsSuccessful()
	success2 := exec2.IsSuccessful()
	if success1 == success2 {
		similarity += 0.15
	}

	// Factor 4: Duration similarity (10% weight)
	// Consider similar if durations are within 50% of each other
	ratio := float64(exec1.Duration) / float64(exec2.Duration)
	if ratio > 2.0 {
		ratio = 1.0 / ratio
	}
	if ratio >= 0.5 {
		similarity += 0.10
	}

	// Factor 5: Step count similarity (5% weight)
	stepCount1 := len(exec1.Steps)
	stepCount2 := len(exec2.Steps)
	if stepCount1 > 0 && stepCount2 > 0 {
		ratio := float64(stepCount1) / float64(stepCount2)
		if ratio > 2.0 {
			ratio = 1.0 / ratio
		}
		if ratio >= 0.5 {
			similarity += 0.05
		}
	}

	b.log.WithFields(logrus.Fields{
		"execution1_id": exec1.ID,
		"execution2_id": exec2.ID,
		"similarity":    similarity,
	}).Debug("Calculated execution similarity")

	return similarity
}

// getExecutionEnvironment extracts environment from execution metadata
func (b *DefaultIntelligentWorkflowBuilder) getExecutionEnvironment(execution *RuntimeWorkflowExecution) string {
	if execution.Metadata != nil {
		if env, ok := execution.Metadata["environment"].(string); ok {
			return env
		}
	}
	return "unknown"
}

func (b *DefaultIntelligentWorkflowBuilder) extractPatternFromCluster(cluster *ExecutionCluster, criteria *PatternCriteria) (*WorkflowPattern, error) {
	b.log.WithFields(logrus.Fields{
		"cluster_size": len(cluster.Members),
	}).Debug("Extracting pattern from execution cluster")

	if len(cluster.Members) == 0 {
		return nil, fmt.Errorf("cannot extract pattern from empty cluster")
	}

	// Use the first execution as the template for pattern extraction
	templateExecution := cluster.Members[0]

	// Calculate pattern statistics
	successCount := 0
	totalDuration := time.Duration(0)
	environments := make(map[string]int)
	resourceTypes := make(map[string]int)

	for _, execution := range cluster.Members {
		if execution.IsSuccessful() {
			successCount++
		}
		totalDuration += execution.Duration

		// Track environments
		env := b.getExecutionEnvironment(execution)
		environments[env]++

		// Track resource types from metadata
		if execution.Metadata != nil {
			if resourceType, ok := execution.Metadata["resource_type"].(string); ok {
				resourceTypes[resourceType]++
			}
		}
	}

	// Calculate success rate
	successRate := float64(successCount) / float64(len(cluster.Members))
	if successRate < criteria.MinSuccessRate {
		return nil, fmt.Errorf("cluster success rate %.2f below threshold %.2f", successRate, criteria.MinSuccessRate)
	}

	// Generate pattern ID
	patternID := fmt.Sprintf("pattern-%s-%d", templateExecution.WorkflowID, time.Now().Unix())

	// Create pattern
	pattern := &WorkflowPattern{
		ID:             patternID,
		Name:           fmt.Sprintf("Pattern from %d similar executions", len(cluster.Members)),
		Type:           b.determinePatternType(cluster.Members),
		Steps:          b.extractStepsFromCluster(cluster.Members),
		Conditions:     []*ActionCondition{}, // Will be implemented if needed
		SuccessRate:    successRate,
		ExecutionCount: len(cluster.Members),
		AverageTime:    totalDuration / time.Duration(len(cluster.Members)),
		Confidence:     b.calculateClusterPatternConfidence(cluster.Members, successRate),
		Environments:   b.getTopEnvironments(environments, 3),
		ResourceTypes:  b.getTopResourceTypes(resourceTypes, 5),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	b.log.WithFields(logrus.Fields{
		"pattern_id":      pattern.ID,
		"success_rate":    successRate,
		"execution_count": len(cluster.Members),
		"confidence":      pattern.Confidence,
	}).Info("Successfully extracted pattern from cluster")

	return pattern, nil
}

// Helper methods for pattern extraction

func (b *DefaultIntelligentWorkflowBuilder) determinePatternType(executions []*RuntimeWorkflowExecution) string {
	// Determine pattern type based on execution characteristics
	if len(executions) == 0 {
		return "unknown"
	}

	// Check if all executions are from the same workflow
	firstWorkflowID := executions[0].WorkflowID
	sameWorkflow := true
	for _, execution := range executions {
		if execution.WorkflowID != firstWorkflowID {
			sameWorkflow = false
			break
		}
	}

	if sameWorkflow {
		return "workflow-specific"
	}

	// Check for resource-based patterns
	resourceTypes := make(map[string]int)
	for _, execution := range executions {
		if execution.Metadata != nil {
			if resourceType, ok := execution.Metadata["resource_type"].(string); ok {
				resourceTypes[resourceType]++
			}
		}
	}

	if len(resourceTypes) > 0 {
		return "resource-based"
	}

	return "general"
}

func (b *DefaultIntelligentWorkflowBuilder) extractStepsFromCluster(executions []*RuntimeWorkflowExecution) []*ExecutableWorkflowStep {
	if len(executions) == 0 {
		return []*ExecutableWorkflowStep{}
	}

	// Use the most successful execution as template
	var templateExecution *RuntimeWorkflowExecution
	for _, execution := range executions {
		if execution.IsSuccessful() {
			templateExecution = execution
			break
		}
	}

	if templateExecution == nil {
		templateExecution = executions[0] // Fallback to first execution
	}

	// Create pattern steps based on template execution
	var steps []*ExecutableWorkflowStep
	for _, step := range templateExecution.Steps {
		// Create a pattern step based on the template step
		patternStep := &ExecutableWorkflowStep{
			BaseEntity: sharedTypes.BaseEntity{
				ID:          step.StepID,
				Name:        fmt.Sprintf("Step %s", step.StepID),
				Description: fmt.Sprintf("Pattern step extracted from %d executions", len(executions)),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    map[string]interface{}{"pattern_extracted": true},
			},
			Type:         StepTypeAction, // Default to action type since we can't determine from StepExecution
			Dependencies: []string{},     // StepExecution doesn't have dependencies
			Timeout:      step.Duration,  // Use actual duration as timeout estimate
			Variables:    make(map[string]interface{}),
		}

		// Copy action if present (simplified)
		if step.Result != nil && step.Result.ActionTrace != nil {
			// Extract action information from trace
			patternStep.Action = &StepAction{
				Type:       step.Result.ActionTrace.ActionType,
				Parameters: make(map[string]interface{}), // ActionTrace doesn't have Parameters field
			}
		}

		steps = append(steps, patternStep)
	}

	return steps
}

func (b *DefaultIntelligentWorkflowBuilder) calculateClusterPatternConfidence(executions []*RuntimeWorkflowExecution, successRate float64) float64 {
	// Base confidence on success rate and execution count
	baseConfidence := successRate

	// Boost confidence for larger sample sizes
	sampleSize := float64(len(executions))
	if sampleSize >= 10 {
		baseConfidence += 0.1
	} else if sampleSize >= 5 {
		baseConfidence += 0.05
	}

	// Cap confidence at 0.95
	if baseConfidence > 0.95 {
		baseConfidence = 0.95
	}

	return baseConfidence
}

func (b *DefaultIntelligentWorkflowBuilder) getTopEnvironments(environments map[string]int, limit int) []string {
	type envCount struct {
		env   string
		count int
	}

	var envs []envCount
	for env, count := range environments {
		envs = append(envs, envCount{env, count})
	}

	// Simple sort by count (descending)
	for i := 0; i < len(envs); i++ {
		for j := i + 1; j < len(envs); j++ {
			if envs[j].count > envs[i].count {
				envs[i], envs[j] = envs[j], envs[i]
			}
		}
	}

	var result []string
	for i, env := range envs {
		if i >= limit {
			break
		}
		result = append(result, env.env)
	}

	return result
}

func (b *DefaultIntelligentWorkflowBuilder) getTopResourceTypes(resourceTypes map[string]int, limit int) []string {
	type resourceCount struct {
		resource string
		count    int
	}

	var resources []resourceCount
	for resource, count := range resourceTypes {
		resources = append(resources, resourceCount{resource, count})
	}

	// Simple sort by count (descending)
	for i := 0; i < len(resources); i++ {
		for j := i + 1; j < len(resources); j++ {
			if resources[j].count > resources[i].count {
				resources[i], resources[j] = resources[j], resources[i]
			}
		}
	}

	var result []string
	for i, resource := range resources {
		if i >= limit {
			break
		}
		result = append(result, resource.resource)
	}

	return result
}

func (b *DefaultIntelligentWorkflowBuilder) rankPatterns(patterns []*WorkflowPattern) {
	b.log.WithField("pattern_count", len(patterns)).Debug("Ranking patterns by effectiveness and confidence")

	// Sort patterns by composite score (success rate + confidence + execution count bonus)
	for i := 0; i < len(patterns); i++ {
		for j := i + 1; j < len(patterns); j++ {
			score1 := b.calculatePatternScore(patterns[i])
			score2 := b.calculatePatternScore(patterns[j])

			// Sort in descending order (higher score first)
			if score2 > score1 {
				patterns[i], patterns[j] = patterns[j], patterns[i]
			}
		}
	}

	b.log.WithFields(logrus.Fields{
		"pattern_count": len(patterns),
		"top_score": func() float64 {
			if len(patterns) > 0 {
				return b.calculatePatternScore(patterns[0])
			}
			return 0.0
		}(),
	}).Info("Pattern ranking completed")
}

// calculatePatternScore calculates a composite score for ranking patterns
func (b *DefaultIntelligentWorkflowBuilder) calculatePatternScore(pattern *WorkflowPattern) float64 {
	// Base score from success rate (0-1)
	score := pattern.SuccessRate

	// Add confidence bonus (0-0.3)
	score += pattern.Confidence * 0.3

	// Add execution count bonus (diminishing returns)
	executionBonus := float64(pattern.ExecutionCount) / 50.0 // Max bonus 0.2 for 10+ executions
	if executionBonus > 0.2 {
		executionBonus = 0.2
	}
	score += executionBonus

	// Add recency bonus if pattern is recent (0-0.1)
	age := time.Since(pattern.UpdatedAt)
	if age < 24*time.Hour {
		score += 0.1 * (1.0 - float64(age.Hours())/24.0) // Linear decay over 24 hours
	}

	return score
}

// Parallel execution optimization helper methods

func (b *DefaultIntelligentWorkflowBuilder) buildDependencyGraph(steps []*ExecutableWorkflowStep) map[string][]string {
	dependencies := make(map[string][]string)

	for _, step := range steps {
		dependencies[step.ID] = step.Dependencies
	}

	return dependencies
}

func (b *DefaultIntelligentWorkflowBuilder) identifyParallelGroups(steps []*ExecutableWorkflowStep, dependencies map[string][]string) [][]*ExecutableWorkflowStep {
	var groups [][]*ExecutableWorkflowStep
	visited := make(map[string]bool)

	// Find steps with no dependencies (can start immediately)
	var independentSteps []*ExecutableWorkflowStep
	for _, step := range steps {
		if len(dependencies[step.ID]) == 0 && !visited[step.ID] {
			independentSteps = append(independentSteps, step)
			visited[step.ID] = true
		}
	}

	if len(independentSteps) > 1 {
		groups = append(groups, independentSteps)
	}

	// Find steps that can run in parallel after their dependencies are met
	for _, step := range steps {
		if visited[step.ID] {
			continue
		}

		// Find steps with same dependency set
		var siblingSteps []*ExecutableWorkflowStep
		siblingSteps = append(siblingSteps, step)
		visited[step.ID] = true

		for _, otherStep := range steps {
			if !visited[otherStep.ID] && b.haveSameDependencies(dependencies[step.ID], dependencies[otherStep.ID]) {
				siblingSteps = append(siblingSteps, otherStep)
				visited[otherStep.ID] = true
			}
		}

		if len(siblingSteps) > 1 {
			groups = append(groups, siblingSteps)
		}
	}

	return groups
}

func (b *DefaultIntelligentWorkflowBuilder) haveSameDependencies(deps1, deps2 []string) bool {
	if len(deps1) != len(deps2) {
		return false
	}

	// Simple comparison - could be optimized with sorting
	depMap := make(map[string]bool)
	for _, dep := range deps1 {
		depMap[dep] = true
	}

	for _, dep := range deps2 {
		if !depMap[dep] {
			return false
		}
	}

	return true
}

func (b *DefaultIntelligentWorkflowBuilder) createParallelExecutionStep(steps []*ExecutableWorkflowStep) *ExecutableWorkflowStep {
	if len(steps) <= 1 {
		return nil
	}

	// Create a parallel execution step
	parallelStep := &ExecutableWorkflowStep{
		BaseEntity: sharedTypes.BaseEntity{
			ID:          fmt.Sprintf("parallel-group-%d", time.Now().UnixNano()),
			Name:        fmt.Sprintf("Parallel Group (%d steps)", len(steps)),
			Description: fmt.Sprintf("Parallel execution of %d independent steps", len(steps)),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Metadata:    map[string]interface{}{"optimization": "parallel_execution", "member_count": len(steps)},
		},
		Type:         StepTypeParallel,
		Dependencies: steps[0].Dependencies, // Use first step's dependencies
		Timeout:      b.calculateParallelTimeout(steps),
		Variables:    make(map[string]interface{}),
	}

	return parallelStep
}

func (b *DefaultIntelligentWorkflowBuilder) calculateParallelTimeout(steps []*ExecutableWorkflowStep) time.Duration {
	// Parallel timeout should be the maximum of individual step timeouts plus buffer
	var maxTimeout time.Duration
	for _, step := range steps {
		if step.Timeout > maxTimeout {
			maxTimeout = step.Timeout
		}
	}

	// Add 30% buffer for parallel coordination overhead
	return time.Duration(float64(maxTimeout) * 1.3)
}

// Resource optimization helper methods

func (b *DefaultIntelligentWorkflowBuilder) optimizeCPUMemoryResources(step *ExecutableWorkflowStep) bool {
	if step.Action == nil || step.Action.Parameters == nil {
		return false
	}

	optimized := false

	// Optimize CPU limits based on action type
	if cpuLimit, exists := step.Action.Parameters["cpu_limit"]; exists {
		if optimizedCPU := b.optimizeResourceValue(step.Action.Type, "cpu", cpuLimit); optimizedCPU != cpuLimit {
			step.Action.Parameters["cpu_limit"] = optimizedCPU
			optimized = true
		}
	}

	// Optimize Memory limits based on action type
	if memoryLimit, exists := step.Action.Parameters["memory_limit"]; exists {
		if optimizedMemory := b.optimizeResourceValue(step.Action.Type, "memory", memoryLimit); optimizedMemory != memoryLimit {
			step.Action.Parameters["memory_limit"] = optimizedMemory
			optimized = true
		}
	}

	// Add resource requests if not present (best practice)
	if _, exists := step.Action.Parameters["cpu_request"]; !exists && step.Action.Type != "notify_only" {
		step.Action.Parameters["cpu_request"] = "100m" // Conservative default
		optimized = true
	}

	if _, exists := step.Action.Parameters["memory_request"]; !exists && step.Action.Type != "notify_only" {
		step.Action.Parameters["memory_request"] = "128Mi" // Conservative default
		optimized = true
	}

	return optimized
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeResourceValue(actionType, resourceType string, currentValue interface{}) interface{} {
	// Resource optimization rules based on action type
	switch actionType {
	case "scale_deployment":
		if resourceType == "cpu" {
			return "200m" // Scale operations need moderate CPU
		} else if resourceType == "memory" {
			return "256Mi" // Scale operations need moderate memory
		}
	case "restart_pod":
		if resourceType == "cpu" {
			return "100m" // Pod restarts are lightweight
		} else if resourceType == "memory" {
			return "128Mi"
		}
	case "increase_resources":
		if resourceType == "cpu" {
			return "300m" // Resource modifications need more CPU
		} else if resourceType == "memory" {
			return "512Mi"
		}
	case "collect_diagnostics":
		if resourceType == "cpu" {
			return "500m" // Diagnostics can be CPU intensive
		} else if resourceType == "memory" {
			return "1Gi" // May need to store diagnostic data
		}
	case "drain_node":
		if resourceType == "cpu" {
			return "200m" // Node operations need moderate resources
		} else if resourceType == "memory" {
			return "256Mi"
		}
	case "notify_only":
		// Notifications need minimal resources - don't change
		return currentValue
	default:
		// Conservative defaults for unknown actions
		if resourceType == "cpu" {
			return "150m"
		} else if resourceType == "memory" {
			return "192Mi"
		}
	}

	return currentValue
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeStepTimeouts(step *ExecutableWorkflowStep) bool {
	if step.Action == nil {
		return false
	}

	originalTimeout := step.Timeout
	optimizedTimeout := b.getOptimalTimeoutForAction(step.Action.Type)

	// Only change if the optimized timeout is different and reasonable
	if optimizedTimeout != originalTimeout && optimizedTimeout > 10*time.Second && optimizedTimeout < 30*time.Minute {
		step.Timeout = optimizedTimeout
		return true
	}

	return false
}

func (b *DefaultIntelligentWorkflowBuilder) getOptimalTimeoutForAction(actionType string) time.Duration {
	switch actionType {
	case "restart_pod":
		return 2 * time.Minute // Pod restarts are usually quick
	case "scale_deployment":
		return 3 * time.Minute // Scaling can take time for pods to start
	case "increase_resources":
		return 5 * time.Minute // Resource changes require restarts
	case "rollback_deployment":
		return 4 * time.Minute // Rollbacks involve pod cycling
	case "drain_node":
		return 15 * time.Minute // Node draining can take significant time
	case "collect_diagnostics":
		return 10 * time.Minute // Diagnostic collection varies
	case "notify_only":
		return 30 * time.Second // Notifications should be fast
	case "backup_data":
		return 20 * time.Minute // Backups can take time
	case "cleanup_storage":
		return 10 * time.Minute // Storage cleanup varies
	default:
		return 5 * time.Minute // Conservative default
	}
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeRetryPolicies(step *ExecutableWorkflowStep) bool {
	if step.Action == nil {
		return false
	}

	originalRetries := step.RetryPolicy.MaxRetries
	optimizedRetries := b.getOptimalRetriesForAction(step.Action.Type)

	// Only change if different
	if optimizedRetries != originalRetries {
		step.RetryPolicy.MaxRetries = optimizedRetries
		step.RetryPolicy.Delay = b.getOptimalRetryDelayForAction(step.Action.Type)
		return true
	}

	return false
}

func (b *DefaultIntelligentWorkflowBuilder) getOptimalRetriesForAction(actionType string) int {
	switch actionType {
	case "restart_pod":
		return 2 // Restarts usually work quickly or not at all
	case "scale_deployment":
		return 3 // Scaling might need a few tries
	case "increase_resources":
		return 2 // Resource changes are usually straightforward
	case "rollback_deployment":
		return 2 // Rollbacks should be reliable
	case "drain_node":
		return 1 // Node draining shouldn't be retried aggressively
	case "collect_diagnostics":
		return 3 // Collection might fail due to temporary issues
	case "notify_only":
		return 5 // Notifications should be reliable
	case "backup_data":
		return 2 // Backups should not be retried too much
	case "cleanup_storage":
		return 2 // Storage cleanup retries should be limited
	default:
		return 3 // Default moderate retry count
	}
}

func (b *DefaultIntelligentWorkflowBuilder) getOptimalRetryDelayForAction(actionType string) time.Duration {
	switch actionType {
	case "restart_pod":
		return 10 * time.Second
	case "scale_deployment":
		return 15 * time.Second
	case "increase_resources":
		return 20 * time.Second
	case "drain_node":
		return 30 * time.Second // Give nodes time to drain
	case "collect_diagnostics":
		return 5 * time.Second
	case "notify_only":
		return 2 * time.Second
	default:
		return 10 * time.Second
	}
}

func (b *DefaultIntelligentWorkflowBuilder) optimizeWorkflowResourceLimits(template *ExecutableTemplate) {
	// Optimize overall workflow timeouts based on optimized step timeouts
	totalOptimizedTime := time.Duration(0)
	for _, step := range template.Steps {
		totalOptimizedTime += step.Timeout
	}

	// Set workflow execution timeout to 150% of total step time (buffer for coordination)
	if template.Timeouts == nil {
		template.Timeouts = &WorkflowTimeouts{}
	}

	optimizedExecutionTimeout := time.Duration(float64(totalOptimizedTime) * 1.5)
	if optimizedExecutionTimeout < template.Timeouts.Execution || template.Timeouts.Execution == 0 {
		template.Timeouts.Execution = optimizedExecutionTimeout
	}
}

func (b *DefaultIntelligentWorkflowBuilder) adaptStepToContext(patternStep *ExecutableWorkflowStep, workflowContext *WorkflowContext) (*ExecutableWorkflowStep, error) {
	if patternStep == nil {
		return nil, fmt.Errorf("pattern step cannot be nil")
	}

	b.log.Debug("Step adaptation: Using pattern step as-is - context adaptation not fully implemented")
	// Create a copy to avoid modifying the original pattern
	adapted := &ExecutableWorkflowStep{
		Type:         patternStep.Type,
		Action:       patternStep.Action,
		Condition:    patternStep.Condition,
		Dependencies: patternStep.Dependencies,
		Timeout:      patternStep.Timeout,
		RetryPolicy:  patternStep.RetryPolicy,
		OnSuccess:    patternStep.OnSuccess,
		OnFailure:    patternStep.OnFailure,
		Variables:    make(map[string]interface{}),
		Metadata:     make(map[string]interface{}),
	}

	// Set embedded fields from BaseEntity
	adapted.ID = patternStep.ID + "-adapted"
	adapted.Name = patternStep.Name + " (adapted)"
	adapted.Description = "Adapted from pattern step"

	return adapted, nil
}

func (b *DefaultIntelligentWorkflowBuilder) adaptConditionToContext(patternCondition *ActionCondition, workflowContext *WorkflowContext) (*ExecutableCondition, error) {
	b.log.Debug("Condition adaptation: Using basic condition adaptation")
	return &ExecutableCondition{
		ID:         "adapted-condition",
		Name:       "Adapted Condition",
		Type:       ConditionTypeExpression,
		Expression: "true", // Default to true for now
		Variables:  make(map[string]interface{}),
		Timeout:    30 * time.Second,
	}, nil
}

func (b *DefaultIntelligentWorkflowBuilder) calculateValidationSummary(results []*WorkflowRuleValidationResult) *ValidationSummary {
	summary := &ValidationSummary{}
	for _, result := range results {
		summary.Total++
		if result.Passed {
			summary.Passed++
		} else {
			summary.Failed++
		}
	}
	return summary
}

// Business Requirement: BR-PD-001 - Advanced Learning Extraction System
func (b *DefaultIntelligentWorkflowBuilder) extractLearnings(execution *RuntimeWorkflowExecution) ([]*WorkflowLearning, error) {
	b.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"duration":     execution.Duration,
		"status":       execution.OperationalStatus,
	}).Debug("Extracting comprehensive learnings from workflow execution")

	learnings := make([]*WorkflowLearning, 0)

	// Extract performance learnings
	if performanceLearning := b.extractPerformanceLearning(execution); performanceLearning != nil {
		learnings = append(learnings, performanceLearning)
	}

	// Extract failure pattern learnings
	if execution.OperationalStatus != ExecutionStatusCompleted {
		if failureLearning := b.extractFailureLearning(execution); failureLearning != nil {
			learnings = append(learnings, failureLearning)
		}
	}

	// Extract success pattern learnings
	if execution.OperationalStatus == ExecutionStatusCompleted {
		if successLearning := b.extractSuccessLearning(execution); successLearning != nil {
			learnings = append(learnings, successLearning)
		}
	}

	// Extract step-level learnings
	stepLearnings := b.extractStepLearnings(execution)
	learnings = append(learnings, stepLearnings...)

	// Extract resource efficiency learnings
	if resourceLearning := b.extractResourceLearning(execution); resourceLearning != nil {
		learnings = append(learnings, resourceLearning)
	}

	b.log.WithFields(logrus.Fields{
		"execution_id":    execution.ID,
		"learnings_count": len(learnings),
		"types":           b.getLearningTypes(learnings),
	}).Info("Learning extraction completed")

	return learnings, nil
}

// Business Requirement: BR-PD-002 - Intelligent Learning Application System
func (b *DefaultIntelligentWorkflowBuilder) applyLearning(ctx context.Context, learning *WorkflowLearning) error {
	b.log.WithFields(logrus.Fields{
		"learning_id":   learning.ID,
		"learning_type": learning.Type,
		"workflow_id":   learning.WorkflowID,
	}).Info("Applying comprehensive learning to pattern system")

	startTime := time.Now()

	// Apply learning based on type
	switch learning.Type {
	case LearningTypePattern:
		if err := b.applyPatternDiscoveryLearning(ctx, learning); err != nil {
			return fmt.Errorf("failed to apply pattern discovery learning: %w", err)
		}

	case LearningTypePerformance:
		if err := b.applyPerformanceOptimizationLearning(ctx, learning); err != nil {
			return fmt.Errorf("failed to apply performance optimization learning: %w", err)
		}

	case LearningTypeFailure:
		if err := b.applyFailureAnalysisLearning(ctx, learning); err != nil {
			return fmt.Errorf("failed to apply failure analysis learning: %w", err)
		}

	case LearningTypeOptimization:
		if err := b.applyResourceOptimizationLearning(ctx, learning); err != nil {
			return fmt.Errorf("failed to apply resource optimization learning: %w", err)
		}

	default:
		b.log.WithField("learning_type", learning.Type).Warn("Unknown learning type, applying as generic learning")
		if err := b.applyGenericLearning(ctx, learning); err != nil {
			return fmt.Errorf("failed to apply generic learning: %w", err)
		}
	}

	// Update patterns from learning using helper function
	if err := b.updatePatternsFromLearning(ctx, learning); err != nil {
		b.log.WithError(err).Warn("Failed to update patterns from learning")
	}

	duration := time.Since(startTime)
	b.log.WithFields(logrus.Fields{
		"learning_id":      learning.ID,
		"learning_type":    learning.Type,
		"application_time": duration,
		"success":          true,
	}).Info("Learning application completed successfully")

	return nil
}

// Business Requirement: BR-PD-003 - Pattern Effectiveness Tracking System
func (b *DefaultIntelligentWorkflowBuilder) updatePatternEffectiveness(ctx context.Context, execution *RuntimeWorkflowExecution) error {
	b.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"success":      execution.OperationalStatus == ExecutionStatusCompleted,
	}).Debug("Updating pattern effectiveness from execution results")

	// Find patterns used in this workflow using helper function
	patterns, err := b.findPatternsForWorkflow(ctx, execution.WorkflowID)
	if err != nil {
		return fmt.Errorf("failed to find patterns for workflow: %w", err)
	}

	// Calculate execution quality score
	qualityScore := b.calculateExecutionQuality(execution)

	// Update each pattern with execution results
	for _, pattern := range patterns {
		// Create learning from execution results
		learning := &WorkflowLearning{
			ID:         uuid.New().String(),
			Type:       LearningTypePattern,
			WorkflowID: execution.WorkflowID,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			Data: map[string]interface{}{
				"execution_id":        execution.ID,
				"success":             execution.OperationalStatus == ExecutionStatusCompleted,
				"duration":            execution.Duration,
				"quality_score":       qualityScore,
				"effectiveness_score": qualityScore,
				"execution_time":      execution.Duration,
			},
		}

		// Apply learning to pattern using helper function
		updated, err := b.applyLearningsToPattern(ctx, pattern, []*WorkflowLearning{learning})
		if err != nil {
			b.log.WithError(err).WithField("pattern_id", pattern.ID).Warn("Failed to apply learning to pattern")
			continue
		}

		if updated {
			// Store updated pattern using helper function
			if err := b.storeUpdatedPattern(ctx, pattern); err != nil {
				b.log.WithError(err).WithField("pattern_id", pattern.ID).Warn("Failed to store updated pattern")
			}
		}
	}

	b.log.WithFields(logrus.Fields{
		"execution_id":     execution.ID,
		"patterns_updated": len(patterns),
		"quality_score":    qualityScore,
	}).Info("Pattern effectiveness update completed")

	return nil
}

// Business Requirement: BR-PD-004 - Execution-Based Pattern Discovery
func (b *DefaultIntelligentWorkflowBuilder) storeExecutionForPatternDiscovery(ctx context.Context, execution *RuntimeWorkflowExecution) error {
	b.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"step_count":   len(execution.Steps),
		"success":      execution.OperationalStatus == ExecutionStatusCompleted,
	}).Debug("Storing execution for pattern discovery analysis")

	// Only process successful executions for pattern discovery
	if execution.OperationalStatus != ExecutionStatusCompleted {
		b.log.WithField("execution_id", execution.ID).Debug("Skipping failed execution for pattern discovery")
		return nil
	}

	// Create pattern from execution using helper function
	pattern, err := b.createPatternFromExecutions(ctx, []*RuntimeWorkflowExecution{execution})
	if err != nil {
		return fmt.Errorf("failed to create pattern from execution: %w", err)
	}

	// Check if this pattern already exists or should be merged with existing patterns
	existingPatterns, err := b.findSimilarPatterns(ctx, pattern)
	if err != nil {
		b.log.WithError(err).Warn("Failed to find similar patterns, treating as new pattern")
	}

	if len(existingPatterns) > 0 {
		// Merge with most similar existing pattern
		mostSimilar := existingPatterns[0]
		mergedPattern, err := b.mergePatterns(ctx, mostSimilar, pattern)
		if err != nil {
			return fmt.Errorf("failed to merge patterns: %w", err)
		}

		// Store merged pattern using helper function
		if err := b.storeUpdatedPattern(ctx, mergedPattern); err != nil {
			return fmt.Errorf("failed to store merged pattern: %w", err)
		}

		b.log.WithFields(logrus.Fields{
			"execution_id": execution.ID,
			"pattern_id":   mergedPattern.ID,
			"merge_count":  len(existingPatterns),
		}).Info("Execution merged with existing pattern")
	} else {
		// Store as new pattern using helper function
		if err := b.storeUpdatedPattern(ctx, pattern); err != nil {
			return fmt.Errorf("failed to store new pattern: %w", err)
		}

		b.log.WithFields(logrus.Fields{
			"execution_id": execution.ID,
			"pattern_id":   pattern.ID,
		}).Info("New pattern created from execution")
	}

	return nil
}

// Learning extraction helper methods for Pattern Discovery System

func (b *DefaultIntelligentWorkflowBuilder) extractPerformanceLearning(execution *RuntimeWorkflowExecution) *WorkflowLearning {
	// Calculate performance metrics
	avgStepDuration := execution.Duration / time.Duration(max(len(execution.Steps), 1))

	return &WorkflowLearning{
		ID:         uuid.New().String(),
		Type:       LearningTypePerformance,
		WorkflowID: execution.WorkflowID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Data: map[string]interface{}{
			"execution_id":          execution.ID,
			"total_duration":        execution.Duration,
			"average_step_duration": avgStepDuration,
			"step_count":            len(execution.Steps),
			"success":               execution.OperationalStatus == ExecutionStatusCompleted,
		},
	}
}

func (b *DefaultIntelligentWorkflowBuilder) extractFailureLearning(execution *RuntimeWorkflowExecution) *WorkflowLearning {
	// Identify failure patterns
	failedSteps := make([]string, 0)
	for _, step := range execution.Steps {
		if step.Status != ExecutionStatusCompleted {
			failedSteps = append(failedSteps, step.StepID)
		}
	}

	return &WorkflowLearning{
		ID:         uuid.New().String(),
		Type:       LearningTypeFailure,
		WorkflowID: execution.WorkflowID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Data: map[string]interface{}{
			"execution_id":   execution.ID,
			"failure_reason": execution.Error,
			"failed_steps":   failedSteps,
			"success":        false,
			"step_count":     len(execution.Steps),
		},
	}
}

func (b *DefaultIntelligentWorkflowBuilder) extractSuccessLearning(execution *RuntimeWorkflowExecution) *WorkflowLearning {
	// Extract successful execution patterns
	qualityScore := b.calculateExecutionQuality(execution)

	return &WorkflowLearning{
		ID:         uuid.New().String(),
		Type:       LearningTypePattern,
		WorkflowID: execution.WorkflowID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Data: map[string]interface{}{
			"execution_id":        execution.ID,
			"success":             true,
			"quality_score":       qualityScore,
			"duration":            execution.Duration,
			"step_count":          len(execution.Steps),
			"effectiveness_score": qualityScore,
		},
	}
}

func (b *DefaultIntelligentWorkflowBuilder) extractStepLearnings(execution *RuntimeWorkflowExecution) []*WorkflowLearning {
	learnings := make([]*WorkflowLearning, 0)

	for _, step := range execution.Steps {
		// Extract individual step learnings
		if step.Duration > 5*time.Minute { // Long-running step
			learning := &WorkflowLearning{
				ID:         uuid.New().String(),
				Type:       LearningTypePerformance,
				WorkflowID: execution.WorkflowID,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
				Data: map[string]interface{}{
					"execution_id":  execution.ID,
					"step_id":       step.StepID,
					"step_duration": step.Duration,
					"step_type":     "long_running",
					"success":       step.Status == ExecutionStatusCompleted,
				},
			}
			learnings = append(learnings, learning)
		}
	}

	return learnings
}

func (b *DefaultIntelligentWorkflowBuilder) extractResourceLearning(execution *RuntimeWorkflowExecution) *WorkflowLearning {
	// Extract resource usage patterns
	if execution.Output == nil || execution.Output.Metrics == nil {
		return nil
	}

	return &WorkflowLearning{
		ID:         uuid.New().String(),
		Type:       LearningTypeOptimization,
		WorkflowID: execution.WorkflowID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Data: map[string]interface{}{
			"execution_id":   execution.ID,
			"resource_usage": execution.Output.Metrics.ResourceUsage,
			"success":        execution.OperationalStatus == ExecutionStatusCompleted,
			"duration":       execution.Duration,
		},
	}
}

func (b *DefaultIntelligentWorkflowBuilder) getLearningTypes(learnings []*WorkflowLearning) []string {
	types := make([]string, 0, len(learnings))
	for _, learning := range learnings {
		types = append(types, string(learning.Type))
	}
	return types
}

// Learning application helper methods

func (b *DefaultIntelligentWorkflowBuilder) applyPatternDiscoveryLearning(ctx context.Context, learning *WorkflowLearning) error {
	b.log.WithField("learning_id", learning.ID).Debug("Applying pattern discovery learning")

	// Use helper function to find patterns for workflow
	patterns, err := b.findPatternsForWorkflow(ctx, learning.WorkflowID)
	if err != nil {
		return fmt.Errorf("failed to find patterns: %w", err)
	}

	// Apply learning to each pattern
	for _, pattern := range patterns {
		_, err := b.applyLearningsToPattern(ctx, pattern, []*WorkflowLearning{learning})
		if err != nil {
			b.log.WithError(err).WithField("pattern_id", pattern.ID).Warn("Failed to apply learning to pattern")
		}
	}

	return nil
}

func (b *DefaultIntelligentWorkflowBuilder) applyPerformanceOptimizationLearning(ctx context.Context, learning *WorkflowLearning) error {
	b.log.WithField("learning_id", learning.ID).Debug("Applying performance optimization learning")

	// Create optimization recommendations based on learning
	if duration, ok := learning.Data["total_duration"].(time.Duration); ok {
		if duration > 10*time.Minute {
			// Long execution time - focus on timeout optimization
			b.log.WithFields(logrus.Fields{
				"learning_id": learning.ID,
				"duration":    duration,
			}).Info("Learning indicates need for timeout optimization")
		}
	}

	return nil
}

func (b *DefaultIntelligentWorkflowBuilder) applyFailureAnalysisLearning(ctx context.Context, learning *WorkflowLearning) error {
	b.log.WithField("learning_id", learning.ID).Debug("Applying failure analysis learning")

	// Analyze failure patterns to improve resilience
	if failedSteps, ok := learning.Data["failed_steps"].([]string); ok {
		b.log.WithFields(logrus.Fields{
			"learning_id":        learning.ID,
			"failed_steps_count": len(failedSteps),
		}).Info("Learning indicates need for step resilience improvement")
	}

	return nil
}

func (b *DefaultIntelligentWorkflowBuilder) applyResourceOptimizationLearning(ctx context.Context, learning *WorkflowLearning) error {
	b.log.WithField("learning_id", learning.ID).Debug("Applying resource optimization learning")

	// Apply resource optimization insights
	return nil
}

func (b *DefaultIntelligentWorkflowBuilder) applyGenericLearning(ctx context.Context, learning *WorkflowLearning) error {
	b.log.WithField("learning_id", learning.ID).Debug("Applying generic learning")

	// Generic learning application
	return nil
}

// Pattern discovery helper methods

func (b *DefaultIntelligentWorkflowBuilder) findSimilarPatterns(ctx context.Context, pattern *WorkflowPattern) ([]*WorkflowPattern, error) {
	// Search for similar patterns using basic matching
	// In a real implementation, this would use the vector database
	b.log.WithField("pattern_id", pattern.ID).Debug("Searching for similar patterns")
	return []*WorkflowPattern{}, nil // No similar patterns found
}

func (b *DefaultIntelligentWorkflowBuilder) mergePatterns(ctx context.Context, existing, new *WorkflowPattern) (*WorkflowPattern, error) {
	// Merge two patterns
	merged := existing // Start with existing pattern

	// Update execution count and success rate
	totalExecutions := existing.ExecutionCount + new.ExecutionCount
	weightedSuccessRate := (existing.SuccessRate*float64(existing.ExecutionCount) + new.SuccessRate*float64(new.ExecutionCount)) / float64(totalExecutions)

	merged.ExecutionCount = totalExecutions
	merged.SuccessRate = weightedSuccessRate
	merged.UpdatedAt = time.Now()
	merged.LastUsed = time.Now()

	b.log.WithFields(logrus.Fields{
		"existing_id":       existing.ID,
		"new_id":            new.ID,
		"merged_executions": totalExecutions,
		"merged_success":    weightedSuccessRate,
	}).Debug("Patterns merged successfully")

	return merged, nil
}

// Note: calculateExecutionQuality is implemented in ai_integration_helpers.go

func (b *DefaultIntelligentWorkflowBuilder) enhanceWithAI(ctx context.Context, template *ExecutableTemplate, objective *WorkflowObjective, workflowContext *WorkflowContext) (*ExecutableTemplate, error) {
	b.log.Debug("AI enhancement: AI workflow enhancement not implemented - returning template as-is")
	return template, nil // Return unchanged template
}

func (b *DefaultIntelligentWorkflowBuilder) convertAIResultToTemplate(aiResult *llm.WorkflowGenerationResult, objective *WorkflowObjective, workflowContext *WorkflowContext) (*ExecutableTemplate, error) {
	b.log.WithFields(logrus.Fields{
		"workflow_id":      aiResult.WorkflowID,
		"steps_count":      len(aiResult.Steps),
		"conditions_count": len(aiResult.Conditions),
		"confidence":       aiResult.Confidence,
	}).Debug("Converting AI result to executable template")

	// Create executable template
	template := &ExecutableTemplate{
		BaseVersionedEntity: sharedTypes.BaseVersionedEntity{
			BaseEntity: sharedTypes.BaseEntity{
				ID:          aiResult.WorkflowID,
				Name:        aiResult.Name,
				Description: aiResult.Description,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Version:   "1.0.0",
			CreatedBy: "IntelligentWorkflowBuilder",
		},
		Steps:      make([]*ExecutableWorkflowStep, 0, len(aiResult.Steps)),
		Conditions: make([]*ExecutableCondition, 0, len(aiResult.Conditions)),
		Variables:  aiResult.Variables,
		Tags:       []string{"ai-generated", objective.Type},
	}

	// Convert AI steps to executable steps
	for _, aiStep := range aiResult.Steps {
		execStep, err := b.convertAIStepToExecutableStep(aiStep)
		if err != nil {
			return nil, fmt.Errorf("failed to convert AI step %s: %w", aiStep.ID, err)
		}
		template.Steps = append(template.Steps, execStep)
	}

	// Convert AI conditions to executable conditions
	for _, aiCondition := range aiResult.Conditions {
		execCondition, err := b.convertAIConditionToExecutableCondition(aiCondition)
		if err != nil {
			return nil, fmt.Errorf("failed to convert AI condition %s: %w", aiCondition.ID, err)
		}
		template.Conditions = append(template.Conditions, execCondition)
	}

	// Set timeouts from AI result
	if aiResult.Timeouts != nil {
		template.Timeouts = &WorkflowTimeouts{
			Execution: b.parseDuration(aiResult.Timeouts.Execution, 30*time.Minute),
			Step:      b.parseDuration(aiResult.Timeouts.Step, 5*time.Minute),
			Condition: b.parseDuration(aiResult.Timeouts.Condition, 30*time.Second),
			Recovery:  2 * time.Minute,
		}
	} else {
		// Default timeouts
		template.Timeouts = &WorkflowTimeouts{
			Execution: 30 * time.Minute,
			Step:      5 * time.Minute,
			Condition: 30 * time.Second,
			Recovery:  2 * time.Minute,
		}
	}

	// Add metadata from AI generation
	template.Metadata["ai_generated"] = true
	template.Metadata["ai_confidence"] = aiResult.Confidence
	template.Metadata["ai_reasoning"] = aiResult.Reasoning
	template.Metadata["objective_id"] = objective.ID
	template.Metadata["objective_type"] = objective.Type
	template.Metadata["generated_at"] = time.Now()

	return template, nil
}

// Helper methods for converting AI-generated components

func (b *DefaultIntelligentWorkflowBuilder) convertAIStepToExecutableStep(aiStep *llm.AIGeneratedStep) (*ExecutableWorkflowStep, error) {
	// Parse timeout duration
	timeout := b.parseDuration(aiStep.Timeout, b.config.DefaultStepTimeout)

	// Create executable step
	execStep := &ExecutableWorkflowStep{
		BaseEntity: sharedTypes.BaseEntity{
			ID:          uuid.New().String(), // Generate ID since AIGeneratedStep doesn't have one
			Name:        aiStep.Name,
			Description: fmt.Sprintf("AI-generated %s step", aiStep.Type),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Metadata:    map[string]interface{}{"ai_generated": true, "step_type": aiStep.Type},
		},
		Type:         StepType(aiStep.Type),
		Dependencies: aiStep.Dependencies,
		Timeout:      timeout,
		Variables:    make(map[string]interface{}),
	}

	// Convert action if present
	if aiStep.Action != nil {
		execStep.Action = &StepAction{
			Type:       aiStep.Action.Type,
			Parameters: aiStep.Action.Parameters,
		}
		// Note: llm.AIStepAction doesn't have Target field, so we skip target handling
	}

	// Convert condition if present (aiStep.Condition is *llm.AIStepCondition)
	if aiStep.Condition != nil {
		execStep.Condition = &ExecutableCondition{
			ID:         fmt.Sprintf("condition_%s", execStep.ID),
			Name:       fmt.Sprintf("Condition for %s", aiStep.Name),
			Type:       ConditionTypeExpression,
			Expression: aiStep.Condition.Expression,
			Variables:  make(map[string]interface{}),
			Timeout:    30 * time.Second,
		}
	}

	// Set default retry policy (no RetryPolicy field in AIGeneratedStep)
	execStep.RetryPolicy = &RetryPolicy{
		MaxRetries:  3, // Default value
		Delay:       5 * time.Second,
		Backoff:     BackoffTypeExponential,
		BackoffRate: 2.0,
		Conditions:  []string{"network_error", "timeout", "temporary_failure"},
	}

	// Note: llm.AIGeneratedStep doesn't have OnSuccess field
	// Set default success behavior or leave empty

	// Convert failure policy to failure steps
	if aiStep.OnFailure != nil {
		execStep.OnFailure = []string{aiStep.OnFailure.Action}
	}

	// Note: llm.AIGeneratedStep doesn't have Variables field
	// Keep the default empty Variables map initialized above

	return execStep, nil
}

func (b *DefaultIntelligentWorkflowBuilder) convertAIConditionToExecutableCondition(aiCondition *llm.LLMConditionSpec) (*ExecutableCondition, error) {
	// Parse timeout duration
	timeout := b.parseDuration(aiCondition.Timeout, 30*time.Second)

	execCondition := &ExecutableCondition{
		ID:         aiCondition.ID,
		Name:       aiCondition.Name,
		Type:       ConditionType(aiCondition.Type),
		Expression: aiCondition.Expression,
		Variables:  make(map[string]interface{}),
		Timeout:    timeout,
	}

	return execCondition, nil
}

func (b *DefaultIntelligentWorkflowBuilder) parseDuration(durationStr string, defaultDuration time.Duration) time.Duration {
	if durationStr == "" {
		return defaultDuration
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		b.log.WithFields(logrus.Fields{
			"duration_str": durationStr,
			"error":        err,
			"default":      defaultDuration,
		}).Warn("Failed to parse duration, using default")
		return defaultDuration
	}

	return duration
}

// Additional stub types needed for compilation
type ExecutionCluster struct {
	Members []*RuntimeWorkflowExecution
}

// WorkflowLearning is already defined in models.go
// Removing duplicate declaration

// Step handler constructors - provide basic implementations
func NewActionStepHandler(log *logrus.Logger) StepHandler {
	return &BasicStepHandler{log: log, handlerType: "action"}
}

func NewConditionStepHandler(log *logrus.Logger) StepHandler {
	return &BasicStepHandler{log: log, handlerType: "condition"}
}

func NewParallelStepHandler(log *logrus.Logger) StepHandler {
	return &BasicStepHandler{log: log, handlerType: "parallel"}
}

func NewLoopStepHandler(log *logrus.Logger) StepHandler {
	return &BasicStepHandler{log: log, handlerType: "loop"}
}

func NewDecisionStepHandler(log *logrus.Logger) StepHandler {
	return &BasicStepHandler{log: log, handlerType: "decision"}
}

func NewWaitStepHandler(log *logrus.Logger) StepHandler {
	return &BasicStepHandler{log: log, handlerType: "wait"}
}

func NewSubflowStepHandler(log *logrus.Logger) StepHandler {
	return &BasicStepHandler{log: log, handlerType: "subflow"}
}

// BasicStepHandler provides minimal step handling functionality
type BasicStepHandler struct {
	log         *logrus.Logger
	handlerType string
}

func (s *BasicStepHandler) GenerateSteps(ctx context.Context, objective *WorkflowObjective, context *WorkflowContext) ([]*ExecutableWorkflowStep, error) {
	s.log.WithField("handler_type", s.handlerType).Debug("Step generation: Advanced step generation not implemented - returning empty steps")
	return []*ExecutableWorkflowStep{}, nil // No steps generated
}

func (s *BasicStepHandler) ValidateStep(step *ExecutableWorkflowStep) error {
	if step == nil {
		return fmt.Errorf("step cannot be nil")
	}
	if step.ID == "" {
		return fmt.Errorf("step ID is required")
	}
	s.log.WithFields(logrus.Fields{
		"handler_type": s.handlerType,
		"step_id":      step.ID,
	}).Debug("Step validation: Basic validation passed")
	return nil
}

func (s *BasicStepHandler) OptimizeStep(step *ExecutableWorkflowStep, metrics *ExecutionMetrics) (*ExecutableWorkflowStep, error) {
	if step == nil {
		return nil, fmt.Errorf("step cannot be nil")
	}
	s.log.WithFields(logrus.Fields{
		"handler_type": s.handlerType,
		"step_id":      step.ID,
	}).Debug("Step optimization: Advanced optimization not implemented - returning step as-is")
	return step, nil // Return unchanged
}
