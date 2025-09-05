package engine

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/insights"
	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
)

// DefaultIntelligentWorkflowBuilder provides AI-driven workflow generation
type DefaultIntelligentWorkflowBuilder struct {
	llmClient        llm.Client
	vectorDB         vector.VectorDatabase
	analyticsEngine  *insights.AnalyticsEngine
	patternExtractor vector.PatternExtractor
	executionRepo    ExecutionRepository
	log              *logrus.Logger

	// AI Enhancement Components
	promptOptimizer  PromptOptimizer
	metricsCollector AIMetricsCollector
	learningBuilder  LearningEnhancedPromptBuilder

	// Configuration
	config *WorkflowBuilderConfig
}

// WorkflowBuilderConfig holds configuration for the workflow builder
type WorkflowBuilderConfig struct {
	// AI generation settings
	MaxWorkflowSteps   int           `yaml:"max_workflow_steps" default:"20"`
	DefaultStepTimeout time.Duration `yaml:"default_step_timeout" default:"5m"`
	MaxRetries         int           `yaml:"max_retries" default:"3"`

	// Pattern discovery settings
	MinPatternSimilarity float64 `yaml:"min_pattern_similarity" default:"0.8"`
	MinExecutionCount    int     `yaml:"min_execution_count" default:"5"`
	MinSuccessRate       float64 `yaml:"min_success_rate" default:"0.7"`
	PatternLookbackDays  int     `yaml:"pattern_lookback_days" default:"30"`

	// Validation settings
	EnableSafetyChecks bool          `yaml:"enable_safety_checks" default:"true"`
	EnableSimulation   bool          `yaml:"enable_simulation" default:"true"`
	ValidationTimeout  time.Duration `yaml:"validation_timeout" default:"2m"`

	// Learning settings
	EnableLearning        bool          `yaml:"enable_learning" default:"true"`
	LearningBatchSize     int           `yaml:"learning_batch_size" default:"100"`
	PatternUpdateInterval time.Duration `yaml:"pattern_update_interval" default:"1h"`
}

// NewDefaultIntelligentWorkflowBuilder creates a new workflow builder instance
func NewDefaultIntelligentWorkflowBuilder(
	llmClient llm.Client,
	vectorDB vector.VectorDatabase,
	analyticsEngine *insights.AnalyticsEngine,
	patternExtractor vector.PatternExtractor,
	executionRepo ExecutionRepository,
	log *logrus.Logger,
) *DefaultIntelligentWorkflowBuilder {
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

	// Initialize AI enhancement components
	promptOptimizer := NewPromptOptimizer()
	metricsCollector := NewAIMetricsCollector()
	learningBuilder := NewLearningEnhancedPromptBuilder()

	// Register default prompt versions
	iwb := &DefaultIntelligentWorkflowBuilder{
		llmClient:        llmClient,
		vectorDB:         vectorDB,
		analyticsEngine:  analyticsEngine,
		patternExtractor: patternExtractor,
		executionRepo:    executionRepo,
		promptOptimizer:  promptOptimizer,
		metricsCollector: metricsCollector,
		learningBuilder:  learningBuilder,
		log:              log,
		config:           config,
	}

	// Register initial prompt versions
	iwb.registerInitialPrompts()

	return iwb
}

// GenerateWorkflow creates workflow template from high-level objective
func (iwb *DefaultIntelligentWorkflowBuilder) GenerateWorkflow(ctx context.Context, objective *WorkflowObjective) (*WorkflowTemplate, error) {
	iwb.log.WithFields(logrus.Fields{
		"objective_id":   objective.ID,
		"objective_type": objective.Type,
		"description":    objective.Description,
	}).Info("Generating workflow from objective")

	// Step 1: Analyze objective and extract context
	analysisResult, err := iwb.analyzeObjective(ctx, objective)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze objective: %w", err)
	}

	// Step 2: Query vector database for similar successful patterns
	similarPatterns, err := iwb.findSimilarSuccessfulPatterns(ctx, analysisResult)
	if err != nil {
		iwb.log.WithError(err).Warn("Failed to find similar patterns, proceeding with AI generation")
	}

	// Step 3: Use AI to generate step sequence
	template, err := iwb.generateWorkflowWithAI(ctx, objective, analysisResult, similarPatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to generate workflow with AI: %w", err)
	}

	// Step 4: Validate generated workflow structure
	if iwb.config.EnableSafetyChecks {
		validationReport, err := iwb.ValidateWorkflow(ctx, template)
		if err != nil {
			return nil, fmt.Errorf("workflow validation failed: %w", err)
		}

		if validationReport.Summary.Failed > 0 {
			return nil, fmt.Errorf("generated workflow failed validation: %d failures", validationReport.Summary.Failed)
		}
	}

	// Step 5: Optimize workflow based on constraints
	optimizedTemplate, err := iwb.optimizeWorkflowForConstraints(ctx, template, objective.Constraints)
	if err != nil {
		iwb.log.WithError(err).Warn("Failed to optimize workflow, using original template")
		optimizedTemplate = template
	}

	iwb.log.WithFields(logrus.Fields{
		"template_id":  optimizedTemplate.ID,
		"step_count":   len(optimizedTemplate.Steps),
		"has_recovery": optimizedTemplate.Recovery != nil,
	}).Info("Successfully generated workflow template")

	return optimizedTemplate, nil
}

// OptimizeWorkflowStructure improves existing workflow performance
func (iwb *DefaultIntelligentWorkflowBuilder) OptimizeWorkflowStructure(ctx context.Context, template *WorkflowTemplate) (*WorkflowTemplate, error) {
	iwb.log.WithFields(logrus.Fields{
		"template_id": template.ID,
		"version":     template.Version,
	}).Info("Optimizing workflow structure")

	// Step 1: Analyze current workflow performance metrics
	performanceAnalysis, err := iwb.analyzeWorkflowPerformance(ctx, template)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze workflow performance: %w", err)
	}

	// Step 2: Identify bottlenecks and inefficiencies
	bottlenecks := iwb.identifyBottlenecks(performanceAnalysis)

	// Step 3: Generate optimization recommendations
	recommendations, err := iwb.generateOptimizationRecommendations(ctx, template, bottlenecks)
	if err != nil {
		return nil, fmt.Errorf("failed to generate optimization recommendations: %w", err)
	}

	// Step 4: Apply AI-driven structural improvements
	optimizedTemplate, err := iwb.applyOptimizations(ctx, template, recommendations)
	if err != nil {
		return nil, fmt.Errorf("failed to apply optimizations: %w", err)
	}

	// Step 5: Validate optimized workflow
	if iwb.config.EnableSafetyChecks {
		validationReport, err := iwb.ValidateWorkflow(ctx, optimizedTemplate)
		if err != nil {
			return nil, fmt.Errorf("optimized workflow validation failed: %w", err)
		}

		if validationReport.Summary.Failed > 0 {
			iwb.log.Warn("Optimized workflow failed validation, returning original")
			return template, nil
		}
	}

	iwb.log.WithFields(logrus.Fields{
		"original_steps":  len(template.Steps),
		"optimized_steps": len(optimizedTemplate.Steps),
		"optimizations":   len(recommendations),
	}).Info("Successfully optimized workflow structure")

	return optimizedTemplate, nil
}

// FindWorkflowPatterns discovers reusable workflow patterns
func (iwb *DefaultIntelligentWorkflowBuilder) FindWorkflowPatterns(ctx context.Context, criteria *PatternCriteria) ([]*WorkflowPattern, error) {
	iwb.log.WithFields(logrus.Fields{
		"min_similarity":      criteria.MinSimilarity,
		"min_execution_count": criteria.MinExecutionCount,
		"min_success_rate":    criteria.MinSuccessRate,
	}).Info("Discovering workflow patterns")

	// Step 1: Query execution history based on criteria
	timeWindow := time.Duration(iwb.config.PatternLookbackDays) * 24 * time.Hour
	if criteria.TimeWindow > 0 {
		timeWindow = criteria.TimeWindow
	}

	executions, err := iwb.executionRepo.GetExecutionsInTimeWindow(ctx, time.Now().Add(-timeWindow), time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to get execution history: %w", err)
	}

	// Filter executions based on criteria
	filteredExecutions := iwb.filterExecutionsByCriteria(executions, criteria)

	// Step 2: Group executions by similarity
	executionGroups := iwb.groupExecutionsBySimilarity(ctx, filteredExecutions, criteria.MinSimilarity)

	// Step 3: Extract common step sequences and conditions
	patterns := make([]*WorkflowPattern, 0)
	for groupID, group := range executionGroups {
		if len(group) < criteria.MinExecutionCount {
			continue
		}

		pattern, err := iwb.extractPatternFromExecutions(ctx, groupID, group)
		if err != nil {
			iwb.log.WithError(err).Warn("Failed to extract pattern from execution group")
			continue
		}

		// Step 4: Calculate pattern effectiveness and confidence
		pattern.SuccessRate = iwb.calculateSuccessRate(group)
		pattern.Confidence = iwb.calculatePatternConfidence(pattern, group)
		pattern.ExecutionCount = len(group)
		pattern.AverageTime = iwb.calculateAverageExecutionTime(group)

		// Filter by minimum success rate
		if pattern.SuccessRate >= criteria.MinSuccessRate {
			patterns = append(patterns, pattern)
		}
	}

	// Step 5: Rank patterns by success rate and applicability
	sort.Slice(patterns, func(i, j int) bool {
		// Sort by success rate first, then confidence, then execution count
		if patterns[i].SuccessRate != patterns[j].SuccessRate {
			return patterns[i].SuccessRate > patterns[j].SuccessRate
		}
		if patterns[i].Confidence != patterns[j].Confidence {
			return patterns[i].Confidence > patterns[j].Confidence
		}
		return patterns[i].ExecutionCount > patterns[j].ExecutionCount
	})

	iwb.log.WithFields(logrus.Fields{
		"patterns_found":      len(patterns),
		"executions_analyzed": len(filteredExecutions),
	}).Info("Successfully discovered workflow patterns")

	return patterns, nil
}

// ApplyWorkflowPattern creates template from discovered pattern
func (iwb *DefaultIntelligentWorkflowBuilder) ApplyWorkflowPattern(ctx context.Context, pattern *WorkflowPattern, workflowContext *WorkflowContext) (*WorkflowTemplate, error) {
	iwb.log.WithFields(logrus.Fields{
		"pattern_id":   pattern.ID,
		"pattern_name": pattern.Name,
		"context_env":  workflowContext.Environment,
	}).Info("Applying workflow pattern")

	// Step 1: Adapt pattern to specific context
	adaptedSteps, err := iwb.adaptPatternStepsToContext(ctx, pattern.Steps, workflowContext)
	if err != nil {
		return nil, fmt.Errorf("failed to adapt pattern steps to context: %w", err)
	}

	// Step 2: Customize parameters based on environment
	customizedSteps, err := iwb.customizeStepsForEnvironment(ctx, adaptedSteps, workflowContext.Environment)
	if err != nil {
		return nil, fmt.Errorf("failed to customize steps for environment: %w", err)
	}

	// Step 3: Add context-specific conditions
	enhancedSteps, err := iwb.addContextSpecificConditions(ctx, customizedSteps, workflowContext)
	if err != nil {
		return nil, fmt.Errorf("failed to add context-specific conditions: %w", err)
	}

	// Step 4: Create workflow template
	template := &WorkflowTemplate{
		ID:          uuid.New().String(),
		Name:        fmt.Sprintf("Pattern-%s-Applied", pattern.Name),
		Description: fmt.Sprintf("Workflow generated from pattern %s for %s environment", pattern.Name, workflowContext.Environment),
		Version:     "1.0.0",
		Steps:       enhancedSteps,
		Variables:   iwb.extractVariablesFromContext(workflowContext),
		Timeouts: &WorkflowTimeouts{
			Execution: time.Duration(len(enhancedSteps)) * iwb.config.DefaultStepTimeout,
			Step:      iwb.config.DefaultStepTimeout,
			Condition: iwb.config.DefaultStepTimeout / 2,
			Recovery:  iwb.config.DefaultStepTimeout,
		},
		Recovery:  iwb.createDefaultRecoveryPolicy(),
		Tags:      []string{"pattern-generated", pattern.Type, workflowContext.Environment},
		CreatedBy: "intelligent-workflow-builder",
		CreatedAt: time.Now(),
	}

	// Step 5: Validate pattern applicability
	if iwb.config.EnableSafetyChecks {
		validationReport, err := iwb.ValidateWorkflow(ctx, template)
		if err != nil {
			return nil, fmt.Errorf("pattern application validation failed: %w", err)
		}

		if validationReport.Summary.Failed > 0 {
			return nil, fmt.Errorf("applied pattern failed validation: %d failures", validationReport.Summary.Failed)
		}
	}

	iwb.log.WithFields(logrus.Fields{
		"template_id":  template.ID,
		"step_count":   len(template.Steps),
		"pattern_type": pattern.Type,
	}).Info("Successfully applied workflow pattern")

	return template, nil
}

// ValidateWorkflow ensures workflow correctness and safety
func (iwb *DefaultIntelligentWorkflowBuilder) ValidateWorkflow(ctx context.Context, template *WorkflowTemplate) (*ValidationReport, error) {
	iwb.log.WithFields(logrus.Fields{
		"template_id": template.ID,
		"step_count":  len(template.Steps),
	}).Info("Validating workflow template")

	validationCtx, cancel := context.WithTimeout(ctx, iwb.config.ValidationTimeout)
	defer cancel()

	report := &ValidationReport{
		ID:         uuid.New().String(),
		WorkflowID: template.ID,
		Type:       ValidationTypeIntegrity,
		Status:     "running",
		Results:    make([]*ValidationResult, 0),
		CreatedAt:  time.Now(),
	}

	// Step 1: Check step dependencies and cycles
	dependencyResults := iwb.validateStepDependencies(validationCtx, template)
	report.Results = append(report.Results, dependencyResults...)

	// Step 2: Validate action parameters and targets
	actionResults := iwb.validateActionParameters(validationCtx, template)
	report.Results = append(report.Results, actionResults...)

	// Step 3: Verify resource availability and permissions
	resourceResults := iwb.validateResourceAccess(validationCtx, template)
	report.Results = append(report.Results, resourceResults...)

	// Step 4: Assess risk and safety constraints
	safetyResults := iwb.validateSafetyConstraints(validationCtx, template)
	report.Results = append(report.Results, safetyResults...)

	// Step 5: Generate validation summary
	report.Summary = iwb.generateValidationSummary(report.Results)
	report.Status = "completed"
	completedAt := time.Now()
	report.CompletedAt = &completedAt

	iwb.log.WithFields(logrus.Fields{
		"validation_id": report.ID,
		"total_checks":  report.Summary.Total,
		"passed":        report.Summary.Passed,
		"failed":        report.Summary.Failed,
		"is_valid":      report.Summary.Failed == 0,
	}).Info("Workflow validation completed")

	return report, nil
}

// SimulateWorkflow tests workflow behavior without execution
func (iwb *DefaultIntelligentWorkflowBuilder) SimulateWorkflow(ctx context.Context, template *WorkflowTemplate, scenario *SimulationScenario) (*SimulationResult, error) {
	iwb.log.WithFields(logrus.Fields{
		"template_id":   template.ID,
		"scenario_id":   scenario.ID,
		"scenario_type": scenario.Type,
	}).Info("Simulating workflow execution")

	startTime := time.Now()

	// Step 1: Create simulated environment based on scenario
	simEnv, err := iwb.createSimulatedEnvironment(ctx, scenario)
	if err != nil {
		return nil, fmt.Errorf("failed to create simulated environment: %w", err)
	}

	// Step 2: Execute workflow steps in simulation mode
	simulationLogs := make([]string, 0)
	simulationErrors := make([]string, 0)
	stepResults := make(map[string]interface{})
	simulationMetrics := make(map[string]float64)

	for i, step := range template.Steps {
		stepStartTime := time.Now()

		stepResult, err := iwb.simulateStep(ctx, step, simEnv, stepResults)
		if err != nil {
			simulationErrors = append(simulationErrors, fmt.Sprintf("Step %d (%s): %v", i+1, step.Name, err))
			continue
		}

		stepDuration := time.Since(stepStartTime)
		simulationLogs = append(simulationLogs, fmt.Sprintf("Step %d (%s) completed in %v", i+1, step.Name, stepDuration))
		stepResults[step.ID] = stepResult
		simulationMetrics[fmt.Sprintf("step_%d_duration_ms", i+1)] = float64(stepDuration.Milliseconds())

		// Step 3: Track resource usage and timing
		iwb.trackSimulatedResourceUsage(step, stepResult, simulationMetrics)
	}

	// Step 4: Identify potential failure points
	failurePoints := iwb.identifyPotentialFailurePoints(template, stepResults, simulationErrors)

	// Step 5: Generate detailed simulation report
	duration := time.Since(startTime)
	result := &SimulationResult{
		ID:         uuid.New().String(),
		ScenarioID: scenario.ID,
		Success:    len(simulationErrors) == 0,
		Duration:   duration,
		Results: map[string]interface{}{
			"step_results":    stepResults,
			"failure_points":  failurePoints,
			"environment":     scenario.Environment,
			"simulation_type": scenario.Type,
		},
		Metrics: simulationMetrics,
		Logs:    simulationLogs,
		Errors:  simulationErrors,
		RunAt:   startTime,
	}

	iwb.log.WithFields(logrus.Fields{
		"simulation_id": result.ID,
		"success":       result.Success,
		"duration":      result.Duration,
		"errors":        len(result.Errors),
		"steps":         len(template.Steps),
	}).Info("Workflow simulation completed")

	return result, nil
}
