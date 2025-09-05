package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// WorkflowSimulator provides safe workflow testing without real execution
type WorkflowSimulator struct {
	config          *SimulationConfig
	mockK8sClient   *MockKubernetesClient
	mockActionRepo  *MockActionRepository
	mockMetrics     *MockMetricsClient
	resourceModeler *ResourceStateModeler
	timeAccelerator *TimeAccelerator
	failureInjector *FailureInjector
	log             *logrus.Logger
	mu              sync.RWMutex
	simulations     map[string]*ActiveSimulation
}

// SimulationConfig configures simulation behavior
type SimulationConfig struct {
	TimeAcceleration       float64        `yaml:"time_acceleration" default:"10.0"` // 10x faster than real time
	EnableFailureInjection bool           `yaml:"enable_failure_injection" default:"true"`
	ResourceLimits         ResourceLimits `yaml:"resource_limits"`
	SafetyChecks           bool           `yaml:"safety_checks" default:"true"`
	MaxConcurrentSims      int            `yaml:"max_concurrent_sims" default:"50"`
	SimulationTimeout      time.Duration  `yaml:"simulation_timeout" default:"30m"`
}

// ResourceLimits defines limits for simulated resources
type ResourceLimits struct {
	MaxPods        int `yaml:"max_pods" default:"1000"`
	MaxNodes       int `yaml:"max_nodes" default:"100"`
	MaxDeployments int `yaml:"max_deployments" default:"500"`
	MaxServices    int `yaml:"max_services" default:"200"`
}

// SimulationScenario defines a testing scenario
type WorkflowSimulationScenario struct {
	ID               string                 `json:"id"`
	Type             string                 `json:"type"`
	Environment      string                 `json:"environment"`
	InitialState     *ClusterState          `json:"initial_state"`
	FailureScenarios []*FailureScenario     `json:"failure_scenarios"`
	LoadProfile      *LoadProfile           `json:"load_profile"`
	Duration         time.Duration          `json:"duration"`
	Variables        map[string]interface{} `json:"variables"`
}

// ClusterState represents the simulated cluster state
type ClusterState struct {
	Nodes       []*SimulatedNode       `json:"nodes"`
	Namespaces  []*SimulatedNamespace  `json:"namespaces"`
	Deployments []*SimulatedDeployment `json:"deployments"`
	Pods        []*SimulatedPod        `json:"pods"`
	Services    []*SimulatedService    `json:"services"`
	Metrics     map[string]float64     `json:"metrics"`
	Timestamp   time.Time              `json:"timestamp"`
}

// SimulationResult contains comprehensive simulation results
type WorkflowSimulationResult struct {
	ID                string                      `json:"id"`
	Success           bool                        `json:"success"`
	Duration          time.Duration               `json:"duration"`
	StepsExecuted     int                         `json:"steps_executed"`
	StepResults       []*StepSimulationResult     `json:"step_results"`
	ResourceChanges   []*ResourceChange           `json:"resource_changes"`
	FailuresTriggered []*TriggeredFailure         `json:"failures_triggered"`
	Performance       *PerformanceMetrics         `json:"performance"`
	SafetyViolations  []*SafetyViolation          `json:"safety_violations"`
	FinalState        *ClusterState               `json:"final_state"`
	Recommendations   []*SimulationRecommendation `json:"recommendations"`
	StartedAt         time.Time                   `json:"started_at"`
	CompletedAt       time.Time                   `json:"completed_at"`
}

// StepSimulationResult contains results for individual workflow steps
type StepSimulationResult struct {
	StepID            string                 `json:"step_id"`
	StepName          string                 `json:"step_name"`
	Success           bool                   `json:"success"`
	Duration          time.Duration          `json:"duration"`
	ResourcesAffected []string               `json:"resources_affected"`
	StateChanges      map[string]interface{} `json:"state_changes"`
	Outputs           map[string]interface{} `json:"outputs,omitempty"`
	Errors            []string               `json:"errors,omitempty"`
	Warnings          []string               `json:"warnings,omitempty"`
}

// NewWorkflowSimulator creates a new workflow simulator instance
func NewWorkflowSimulator(config *SimulationConfig, log *logrus.Logger) *WorkflowSimulator {
	if config == nil {
		config = &SimulationConfig{
			TimeAcceleration:       10.0,
			EnableFailureInjection: true,
			SafetyChecks:           true,
			MaxConcurrentSims:      50,
			SimulationTimeout:      30 * time.Minute,
			ResourceLimits: ResourceLimits{
				MaxPods:        1000,
				MaxNodes:       100,
				MaxDeployments: 500,
				MaxServices:    200,
			},
		}
	}

	return &WorkflowSimulator{
		config:          config,
		mockK8sClient:   NewMockKubernetesClient(),
		mockActionRepo:  NewMockActionRepository(),
		mockMetrics:     NewMockMetricsClient(),
		resourceModeler: NewResourceStateModeler(config.ResourceLimits),
		timeAccelerator: NewTimeAccelerator(config.TimeAcceleration),
		failureInjector: NewFailureInjector(config.EnableFailureInjection),
		log:             log,
		simulations:     make(map[string]*ActiveSimulation),
	}
}

// SimulateExecution runs workflow in simulated environment
func (ws *WorkflowSimulator) SimulateExecution(ctx context.Context, template *WorkflowTemplate, scenario *WorkflowSimulationScenario) (*WorkflowSimulationResult, error) {
	// Input validation
	if template == nil {
		return nil, fmt.Errorf("workflow template cannot be nil")
	}
	if scenario == nil {
		return nil, fmt.Errorf("simulation scenario cannot be nil")
	}
	if len(template.Steps) == 0 {
		return nil, fmt.Errorf("workflow template must have at least one step")
	}
	if template.ID == "" {
		return nil, fmt.Errorf("workflow template must have a valid ID")
	}
	if scenario.ID == "" {
		return nil, fmt.Errorf("simulation scenario must have a valid ID")
	}

	// Context validation
	if ctx == nil {
		ws.log.Warn("Context is nil, creating background context")
		ctx = context.Background()
	}

	ws.log.WithFields(logrus.Fields{
		"template_id": template.ID,
		"scenario_id": scenario.ID,
		"environment": scenario.Environment,
		"step_count":  len(template.Steps),
	}).Info("Starting workflow simulation")

	// Create simulation context
	simCtx, err := ws.createSimulationContext(ctx, template, scenario)
	if err != nil {
		return nil, fmt.Errorf("failed to create simulation context: %w", err)
	}

	// Initialize simulated environment
	if err := ws.initializeSimulatedEnvironment(simCtx, scenario.InitialState); err != nil {
		return nil, fmt.Errorf("failed to initialize simulated environment: %w", err)
	}

	// Execute workflow steps in simulation mode
	result, err := ws.executeWorkflowSimulation(simCtx, template)
	if err != nil {
		return nil, fmt.Errorf("workflow simulation failed: %w", err)
	}

	// Generate comprehensive results
	finalResult := ws.generateSimulationResults(ctx, result)

	ws.log.WithFields(logrus.Fields{
		"simulation_id":     finalResult.ID,
		"success":           finalResult.Success,
		"duration":          finalResult.Duration,
		"steps_executed":    finalResult.StepsExecuted,
		"safety_violations": len(finalResult.SafetyViolations),
	}).Info("Workflow simulation completed")

	return finalResult, nil
}

// StressTest validates workflow under various load conditions
func (ws *WorkflowSimulator) StressTest(ctx context.Context, template *WorkflowTemplate, loadScenarios []*LoadScenario) (*StressTestResult, error) {
	// Input validation
	if template == nil {
		return nil, fmt.Errorf("workflow template cannot be nil")
	}
	if template.ID == "" {
		return nil, fmt.Errorf("workflow template must have a valid ID")
	}
	if len(template.Steps) == 0 {
		return nil, fmt.Errorf("workflow template must have at least one step")
	}
	if len(loadScenarios) == 0 {
		return nil, fmt.Errorf("at least one load scenario is required for stress testing")
	}

	// Validate load scenarios
	for i, scenario := range loadScenarios {
		if scenario == nil {
			return nil, fmt.Errorf("load scenario %d cannot be nil", i)
		}
		if scenario.ConcurrentUsers <= 0 {
			return nil, fmt.Errorf("load scenario %d must have at least 1 concurrent user", i)
		}
		if scenario.Duration <= 0 {
			return nil, fmt.Errorf("load scenario %d must have a positive duration", i)
		}
	}

	// Context validation
	if ctx == nil {
		ws.log.Warn("Context is nil for stress test, creating background context")
		ctx = context.Background()
	}

	ws.log.WithFields(logrus.Fields{
		"template_id":    template.ID,
		"scenario_count": len(loadScenarios),
		"step_count":     len(template.Steps),
		"load_scenarios": len(loadScenarios),
	}).Info("Starting workflow stress test")

	results := make([]*StressTestScenarioResult, 0, len(loadScenarios))

	for _, loadScenario := range loadScenarios {
		scenarioResult, err := ws.executeStressScenario(ctx, template, loadScenario)
		if err != nil {
			ws.log.WithError(err).WithField("scenario", loadScenario.Name).Error("Stress scenario failed")
			continue
		}
		results = append(results, scenarioResult)
	}

	// Analyze results and identify breaking points
	stressResult := &StressTestResult{
		TemplateID:      template.ID,
		ScenarioResults: results,
		BreakingPoints:  ws.identifyBreakingPoints(results),
		Performance:     ws.analyzeStressPerformance(results),
		Recommendations: ws.generateStressRecommendations(results),
	}

	return stressResult, nil
}

// FailureInjection tests workflow resilience
func (ws *WorkflowSimulator) FailureInjection(ctx context.Context, template *WorkflowTemplate, failures []*FailureScenario) (*ResilienceTestResult, error) {
	ws.log.WithFields(logrus.Fields{
		"template_id":       template.ID,
		"failure_scenarios": len(failures),
	}).Info("Starting failure injection testing")

	results := make([]*FailureInjectionResult, 0, len(failures))

	for _, failure := range failures {
		// Create base scenario
		scenario := &WorkflowSimulationScenario{
			ID:               fmt.Sprintf("failure-%s", failure.ID),
			Type:             "failure_injection",
			FailureScenarios: []*FailureScenario{failure},
			Duration:         5 * time.Minute,
		}

		// Run simulation with failure injection
		simResult, err := ws.SimulateExecution(ctx, template, scenario)
		if err != nil {
			ws.log.WithError(err).WithField("failure", failure.Type).Error("Failure injection test failed")
			continue
		}

		// Analyze failure recovery
		failureResult := &FailureInjectionResult{
			FailureID:     failure.ID,
			Success:       simResult.Success,
			InjectionTime: failure.TriggerTime,
			RecoveryTime:  ws.calculateRecoveryTime(simResult),
			Impact:        ws.calculateImpact(simResult, failure),
			Errors:        []string{},
		}

		results = append(results, failureResult)
	}

	// Convert FailureInjectionResult to FailureTestResult
	failureTestResults := make([]FailureTestResult, len(results))
	for i, result := range results {
		failureTestResults[i] = FailureTestResult{
			FailureName:   result.FailureID,
			InjectionTime: result.InjectionTime,
			RecoveryTime:  result.RecoveryTime,
			Success:       result.Success,
			Impact:        result.Impact,
		}
	}

	// Generate resilience analysis
	resilienceResult := &ResilienceTestResult{
		TemplateID:      template.ID,
		FailureResults:  failureTestResults,
		ResilienceScore: ws.calculateResilienceScore(results),
		RecoveryMetrics: ws.calculateRecoveryMetrics(results),
		Recommendations: ws.generateResilienceRecommendations(results),
	}

	return resilienceResult, nil
}

// ResourceImpactAnalysis models resource usage patterns
func (ws *WorkflowSimulator) ResourceImpactAnalysis(ctx context.Context, template *WorkflowTemplate) (*ResourceImpactReport, error) {
	ws.log.WithFields(logrus.Fields{
		"template_id": template.ID,
	}).Info("Starting resource impact analysis")

	// Create baseline scenario
	scenario := &WorkflowSimulationScenario{
		ID:   "resource-impact-analysis",
		Type: "resource_analysis",
		InitialState: &ClusterState{
			Nodes:       []*SimulatedNode{},
			Namespaces:  []*SimulatedNamespace{},
			Deployments: []*SimulatedDeployment{},
			Metrics:     map[string]float64{},
		},
		Duration: 10 * time.Minute,
	}

	// Run simulation with resource tracking
	result, err := ws.SimulateExecution(ctx, template, scenario)
	if err != nil {
		return nil, fmt.Errorf("resource impact simulation failed: %w", err)
	}

	// Analyze resource consumption patterns
	consumption := ws.analyzeResourceConsumption(result)
	peakUsage := ws.calculatePeakUsage(result)

	report := &ResourceImpactReport{
		TemplateID:            template.ID,
		ResourceConsumption:   consumption,
		PeakUsage:             peakUsage,
		ResourceConflicts:     ws.identifyResourceConflicts(result),
		ScalingRequirements:   ws.calculateScalingRequirements(template, nil), // Pass nil for report initially
		CostEstimation:        ws.estimateResourceCosts(template, consumption),
		Recommendations:       ws.generateResourceRecommendations(result),
		UtilizationEfficiency: ws.calculateUtilizationEfficiency(template, consumption, peakUsage),
	}

	return report, nil
}

// Helper methods for simulation execution

func (ws *WorkflowSimulator) createSimulationContext(ctx context.Context, template *WorkflowTemplate, scenario *WorkflowSimulationScenario) (*SimulationContext, error) {
	simCtx := &SimulationContext{
		ID:              fmt.Sprintf("sim-%s-%d", template.ID, time.Now().Unix()),
		Template:        template,
		Scenario:        scenario,
		StartTime:       time.Now(),
		State:           scenario.InitialState,
		StepResults:     make([]*StepSimulationResult, 0),
		ResourceChanges: make([]*ResourceChange, 0),
		Metrics:         make(map[string]float64),
	}

	return simCtx, nil
}

func (ws *WorkflowSimulator) initializeSimulatedEnvironment(simCtx *SimulationContext, initialState *ClusterState) error {
	// Store reference to initial state or create default
	if initialState != nil {
		simCtx.State = initialState
	} else {
		// Create default initial state
		simCtx.State = &ClusterState{
			Nodes:       make([]*SimulatedNode, 0),
			Namespaces:  make([]*SimulatedNamespace, 0),
			Deployments: make([]*SimulatedDeployment, 0),
			Pods:        make([]*SimulatedPod, 0),
			Services:    make([]*SimulatedService, 0),
			Metrics:     make(map[string]float64),
			Timestamp:   ws.simulationNow(),
		}

		// Initialize with baseline resources
		ws.initializeDefaultState(simCtx.State)
	}

	// Initialize mock Kubernetes client with initial state
	if err := ws.loadInitialState(simCtx.State); err != nil {
		return fmt.Errorf("failed to load initial state: %w", err)
	}

	// Set up resource modeling with configuration awareness
	ws.initializeResourceState(simCtx.State)

	// Configure failure injection if enabled
	if len(simCtx.Scenario.FailureScenarios) > 0 {
		for _, failure := range simCtx.Scenario.FailureScenarios {
			ws.scheduleFailure(failure)
		}

		// Apply scenario-specific configuration
		ws.applyScenarioConfiguration(simCtx)
	}

	ws.log.WithFields(logrus.Fields{
		"simulation_id":    simCtx.ID,
		"node_count":       len(simCtx.State.Nodes),
		"deployment_count": len(simCtx.State.Deployments),
		"namespace_count":  len(simCtx.State.Namespaces),
	}).Info("Initialized simulated environment")

	return nil
}

// initializeDefaultState creates a realistic default cluster state
func (ws *WorkflowSimulator) initializeDefaultState(state *ClusterState) {
	// Create default namespaces
	defaultNamespaces := []string{"default", "kube-system", "kube-public"}
	for range defaultNamespaces {
		state.Namespaces = append(state.Namespaces, &SimulatedNamespace{})
	}

	// Create baseline nodes based on config
	nodeCount := 3
	if ws.config != nil && ws.config.ResourceLimits.MaxNodes > 0 {
		calculated := ws.config.ResourceLimits.MaxNodes / 3
		if calculated > 5 {
			nodeCount = 5 // Conservative baseline cap
		} else if calculated > 0 {
			nodeCount = calculated
		}
	}
	for i := 0; i < nodeCount; i++ {
		state.Nodes = append(state.Nodes, &SimulatedNode{})
	}

	// Create some baseline deployments
	baseDeployments := []struct{ name, namespace string }{
		{"coredns", "kube-system"},
		{"metrics-server", "kube-system"},
	}
	for _, dep := range baseDeployments {
		state.Deployments = append(state.Deployments, &SimulatedDeployment{
			Name:      dep.name,
			Namespace: dep.namespace,
			Replicas:  2,
		})
	}

	// Initialize baseline metrics
	state.Metrics["cpu_usage"] = 0.3
	state.Metrics["memory_usage"] = 0.4
	state.Metrics["disk_usage"] = 0.2
}

// applyScenarioConfiguration applies scenario-specific configuration
func (ws *WorkflowSimulator) applyScenarioConfiguration(simCtx *SimulationContext) {
	scenario := simCtx.Scenario
	if scenario == nil {
		return
	}

	// Apply load profile effects
	if scenario.LoadProfile != nil {
		// Adjust baseline metrics based on expected load
		simCtx.State.Metrics["expected_load"] = 1.0 // Will be adjusted during execution
	}

	// Apply environment-specific settings
	switch scenario.Environment {
	case "production":
		simCtx.State.Metrics["safety_level"] = 0.9
	case "staging":
		simCtx.State.Metrics["safety_level"] = 0.7
	case "development":
		simCtx.State.Metrics["safety_level"] = 0.5
	default:
		simCtx.State.Metrics["safety_level"] = 0.8
	}

	ws.log.WithFields(logrus.Fields{
		"scenario_type": scenario.Type,
		"environment":   scenario.Environment,
		"safety_level":  simCtx.State.Metrics["safety_level"],
	}).Debug("Applied scenario configuration")
}

func (ws *WorkflowSimulator) executeWorkflowSimulation(simCtx *SimulationContext, template *WorkflowTemplate) (*WorkflowSimulationResult, error) {
	result := &WorkflowSimulationResult{
		ID:          simCtx.ID,
		StepResults: make([]*StepSimulationResult, 0),
		Performance: &PerformanceMetrics{},
	}

	// Execute each workflow step in simulation
	for i, step := range template.Steps {
		stepStart := ws.simulationNow()

		stepResult, err := ws.simulateWorkflowStep(simCtx, step)
		if err != nil {
			ws.log.WithError(err).WithField("step_id", step.ID).Error("Step simulation failed")
			stepResult.Success = false
			stepResult.Errors = append(stepResult.Errors, err.Error())
		}

		stepResult.Duration = ws.simulationSince(stepStart)
		result.StepResults = append(result.StepResults, stepResult)

		// Check for safety violations
		if violations := ws.checkSafetyViolations(simCtx); len(violations) > 0 {
			result.SafetyViolations = append(result.SafetyViolations, violations...)
		}

		// Update simulation state
		ws.updateSimulationState(simCtx, step)

		// Check if we should continue
		if !stepResult.Success && step.OnFailure == nil {
			break
		}

		result.StepsExecuted = i + 1
	}

	result.Success = result.StepsExecuted == len(template.Steps)
	result.Duration = ws.simulationSince(simCtx.StartTime)

	return result, nil
}

func (ws *WorkflowSimulator) simulateWorkflowStep(simCtx *SimulationContext, step *WorkflowStep) (*StepSimulationResult, error) {
	stepResult := &StepSimulationResult{
		StepID:            step.ID,
		StepName:          step.Name,
		Success:           true,
		ResourcesAffected: make([]string, 0),
		StateChanges:      make(map[string]interface{}),
	}

	// Simulate different step types
	switch step.Type {
	case StepTypeAction:
		return ws.simulateActionStep(simCtx, step)
	case StepTypeCondition:
		return ws.simulateConditionStep(simCtx, step)
	case StepTypeWait:
		return ws.simulateWaitStep(simCtx, step)
	default:
		return stepResult, nil
	}
}

func (ws *WorkflowSimulator) simulateActionStep(simCtx *SimulationContext, step *WorkflowStep) (*StepSimulationResult, error) {
	if step.Action == nil {
		return nil, fmt.Errorf("action step %s has no action defined", step.ID)
	}

	stepResult := &StepSimulationResult{
		StepID:   step.ID,
		StepName: step.Name,
		Success:  true,
	}

	// Check for injected failures
	if failure := ws.checkForFailure(step.ID, ws.simulationNow()); failure != nil {
		stepResult.Success = false
		stepResult.Errors = append(stepResult.Errors, fmt.Sprintf("Injected failure: %s", failure.Description))
		return stepResult, nil
	}

	// Simulate action execution based on type
	switch step.Action.Type {
	case "kubernetes":
		return ws.simulateKubernetesAction(simCtx, step, stepResult)
	case "notification":
		ws.simulateNotificationAction(stepResult, step.Action)
		return stepResult, nil
	default:
		stepResult.Warnings = append(stepResult.Warnings, fmt.Sprintf("Unknown action type: %s", step.Action.Type))
	}

	return stepResult, nil
}

func (ws *WorkflowSimulator) simulateKubernetesAction(simCtx *SimulationContext, step *WorkflowStep, stepResult *StepSimulationResult) (*StepSimulationResult, error) {
	action := step.Action.Parameters["action"].(string)

	switch action {
	case "scale_deployment":
		return ws.simulateScaleDeployment(simCtx, step, stepResult)
	case "restart_pod":
		ws.simulateRestartPod(stepResult, step.Action)
	case "drain_node":
		ws.simulateDrainNode(stepResult, step.Action)
	case "expand_pvc":
		ws.simulateExpandPVC(stepResult, step.Action)
	default:
		stepResult.Warnings = append(stepResult.Warnings, fmt.Sprintf("Simulated generic kubernetes action: %s", action))
		// Add simulated delay
		ws.simulationSleep(2 * time.Second)
	}

	return stepResult, nil
}

func (ws *WorkflowSimulator) simulateScaleDeployment(simCtx *SimulationContext, step *WorkflowStep, stepResult *StepSimulationResult) (*StepSimulationResult, error) {
	target := step.Action.Target
	replicas := step.Action.Parameters["replicas"].(int)

	// Find deployment in simulated state
	deployment := ws.findSimulatedDeployment(target.Namespace, target.Name)
	if deployment == nil {
		stepResult.Success = false
		stepResult.Errors = append(stepResult.Errors, fmt.Sprintf("Deployment %s/%s not found", target.Namespace, target.Name))
		return stepResult, nil
	}

	// Simulate scaling
	oldReplicas := deployment.Replicas
	deployment.Replicas = replicas

	// Update pods accordingly
	ws.updatePodsForDeployment(deployment, replicas)

	// Record changes
	stepResult.ResourcesAffected = append(stepResult.ResourcesAffected, fmt.Sprintf("deployment/%s", target.Name))
	stepResult.StateChanges["old_replicas"] = oldReplicas
	stepResult.StateChanges["new_replicas"] = replicas

	// Simulate scaling time
	scalingTime := time.Duration(absInt(replicas-oldReplicas)) * 2 * time.Second
	ws.simulationSleep(scalingTime)

	ws.log.WithFields(logrus.Fields{
		"deployment":   target.Name,
		"old_replicas": oldReplicas,
		"new_replicas": replicas,
		"scaling_time": scalingTime,
	}).Info("Simulated deployment scaling")

	return stepResult, nil
}

// Additional simulation methods would be implemented here...

// Helper types and functions

type SimulationContext struct {
	ID              string
	Template        *WorkflowTemplate
	Scenario        *WorkflowSimulationScenario
	StartTime       time.Time
	State           *ClusterState
	StepResults     []*StepSimulationResult
	ResourceChanges []*ResourceChange
	Metrics         map[string]float64
}

type ActiveSimulation struct {
	ID        string
	Context   *SimulationContext
	StartTime time.Time
	Status    string
}

// Helper function
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Stub implementations for undefined mock types
type MockKubernetesClient struct{}
type MockActionRepository struct{}
type MockMetricsClient struct{}
type ResourceStateModeler struct{}
type TimeAccelerator struct{}
type FailureInjector struct{}

func NewMockKubernetesClient() *MockKubernetesClient { return &MockKubernetesClient{} }
func NewMockActionRepository() *MockActionRepository { return &MockActionRepository{} }
func NewMockMetricsClient() *MockMetricsClient       { return &MockMetricsClient{} }
func NewResourceStateModeler(limits interface{}) *ResourceStateModeler {
	return &ResourceStateModeler{}
}
func NewTimeAccelerator(config interface{}) *TimeAccelerator  { return &TimeAccelerator{} }
func NewFailureInjector(enabled interface{}) *FailureInjector { return &FailureInjector{} }

// Stub implementations for simulated Kubernetes resources
type SimulatedNode struct{}
type SimulatedNamespace struct{}
type SimulatedDeployment struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int    `json:"replicas"`
}
type SimulatedPod struct{}
type SimulatedService struct{}

// Additional stub types for workflow simulation
type FailureScenario struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Duration    time.Duration          `json:"duration"`
	TriggerTime time.Time              `json:"trigger_time"`
	Parameters  map[string]interface{} `json:"parameters"`
}
type LoadProfile struct{}
type ResourceChange struct{}
type TriggeredFailure struct{}
type SafetyViolation struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
}
type SimulationRecommendation struct{}
type LoadScenario struct {
	Name               string        `json:"name"`
	ConcurrentUsers    int           `json:"concurrent_users"`
	RequestsPerSecond  int           `json:"requests_per_second"`
	Duration           time.Duration `json:"duration"`
	ResourceMultiplier float64       `json:"resource_multiplier"`
}

type StressTestScenarioResult struct {
	ScenarioName    string                 `json:"scenario_name"`
	Success         bool                   `json:"success"`
	Duration        time.Duration          `json:"duration"`
	RequestsHandled int                    `json:"requests_handled"`
	Errors          []string               `json:"errors"`
	Performance     map[string]interface{} `json:"performance"`
}

type StressTestResult struct {
	TemplateID      string                      `json:"template_id"`
	ScenarioResults []*StressTestScenarioResult `json:"scenario_results"`
	BreakingPoints  []string                    `json:"breaking_points"`
	Performance     map[string]interface{}      `json:"performance"`
	Recommendations []string                    `json:"recommendations"`
}

type ResilienceTestResult struct {
	TemplateID      string                 `json:"template_id"`
	FailureResults  []FailureTestResult    `json:"failure_results"`
	RecoveryTime    time.Duration          `json:"recovery_time"`
	Success         bool                   `json:"success"`
	ResilienceScore float64                `json:"resilience_score"`
	RecoveryMetrics map[string]interface{} `json:"recovery_metrics"`
	Recommendations []string               `json:"recommendations"`
}

type FailureTestResult struct {
	FailureName   string        `json:"failure_name"`
	InjectionTime time.Time     `json:"injection_time"`
	RecoveryTime  time.Duration `json:"recovery_time"`
	Success       bool          `json:"success"`
	Impact        string        `json:"impact"`
}

type ResourceImpactReport struct {
	TemplateID            string                 `json:"template_id"`
	ResourceType          string                 `json:"resource_type"`
	Baseline              map[string]interface{} `json:"baseline"`
	Impact                map[string]interface{} `json:"impact"`
	Recovery              map[string]interface{} `json:"recovery"`
	ResourceConsumption   map[string]interface{} `json:"resource_consumption"`
	PeakUsage             map[string]interface{} `json:"peak_usage"`
	ResourceConflicts     []string               `json:"resource_conflicts"`
	ScalingRequirements   map[string]interface{} `json:"scaling_requirements"`
	CostEstimation        map[string]interface{} `json:"cost_estimation"`
	Recommendations       []string               `json:"recommendations"`
	UtilizationEfficiency float64                `json:"utilization_efficiency"`
}

// FailureInjectionResult represents the result of a failure injection test
type FailureInjectionResult struct {
	FailureID     string        `json:"failure_id"`
	Success       bool          `json:"success"`
	InjectionTime time.Time     `json:"injection_time"`
	RecoveryTime  time.Duration `json:"recovery_time"`
	Impact        string        `json:"impact"`
	Errors        []string      `json:"errors"`
}

// Missing method implementations

// generateSimulationResults creates comprehensive simulation results
func (ws *WorkflowSimulator) generateSimulationResults(ctx context.Context, result *WorkflowSimulationResult) *WorkflowSimulationResult {
	// This method enhances the basic simulation result with additional analysis
	if result == nil {
		return &WorkflowSimulationResult{
			ID:      fmt.Sprintf("sim-%d", time.Now().Unix()),
			Success: false,
		}
	}

	// Add any additional analysis, recommendations, or post-processing
	if result.CompletedAt.IsZero() {
		result.CompletedAt = time.Now()
	}

	return result
}

// executeStressScenario executes a single stress test scenario
func (ws *WorkflowSimulator) executeStressScenario(ctx context.Context, template *WorkflowTemplate, scenario *LoadScenario) (*StressTestScenarioResult, error) {
	startTime := time.Now()

	result := &StressTestScenarioResult{
		ScenarioName: scenario.Name,
		Success:      true,
		Performance:  make(map[string]interface{}),
		Errors:       make([]string, 0),
	}

	// Simulate stress testing
	requestsHandled := 0
	for i := 0; i < scenario.ConcurrentUsers; i++ {
		// Simulate concurrent requests
		for j := 0; j < scenario.RequestsPerSecond; j++ {
			requestsHandled++

			// Simulate some failures under stress
			if requestsHandled > 1000 && requestsHandled%100 == 0 {
				result.Errors = append(result.Errors, fmt.Sprintf("Simulated error at request %d", requestsHandled))
				result.Success = false
			}
		}
	}

	result.Duration = time.Since(startTime)
	result.RequestsHandled = requestsHandled
	result.Performance["requests_per_second"] = float64(requestsHandled) / result.Duration.Seconds()
	result.Performance["error_rate"] = float64(len(result.Errors)) / float64(requestsHandled)

	return result, nil
}

// identifyBreakingPoints analyzes scenario results to find breaking points
func (ws *WorkflowSimulator) identifyBreakingPoints(results []*StressTestScenarioResult) []string {
	breakingPoints := make([]string, 0)

	for _, result := range results {
		if !result.Success {
			breakingPoints = append(breakingPoints, fmt.Sprintf("Breaking point detected in scenario '%s' after handling %d requests",
				result.ScenarioName, result.RequestsHandled))
		}

		if errorRate, ok := result.Performance["error_rate"].(float64); ok && errorRate > 0.1 {
			breakingPoints = append(breakingPoints, fmt.Sprintf("High error rate (%.2f%%) in scenario '%s'",
				errorRate*100, result.ScenarioName))
		}
	}

	return breakingPoints
}

// analyzeStressPerformance analyzes overall performance across scenarios
func (ws *WorkflowSimulator) analyzeStressPerformance(results []*StressTestScenarioResult) map[string]interface{} {
	performance := make(map[string]interface{})

	totalRequests := 0
	totalErrors := 0
	var totalDuration time.Duration

	for _, result := range results {
		totalRequests += result.RequestsHandled
		totalErrors += len(result.Errors)
		totalDuration += result.Duration
	}

	performance["total_requests"] = totalRequests
	performance["total_errors"] = totalErrors
	performance["overall_error_rate"] = float64(totalErrors) / float64(totalRequests)
	performance["average_duration"] = totalDuration / time.Duration(len(results))
	performance["scenarios_passed"] = len(results) - len(ws.getFailedScenarios(results))

	return performance
}

// generateStressRecommendations generates recommendations based on stress test results
func (ws *WorkflowSimulator) generateStressRecommendations(results []*StressTestScenarioResult) []string {
	recommendations := make([]string, 0)

	failedScenarios := ws.getFailedScenarios(results)
	if len(failedScenarios) > 0 {
		recommendations = append(recommendations, "Consider increasing resource limits for better performance under stress")
		recommendations = append(recommendations, "Implement circuit breaker patterns to handle high load gracefully")
	}

	// Check for high error rates
	for _, result := range results {
		if errorRate, ok := result.Performance["error_rate"].(float64); ok && errorRate > 0.05 {
			recommendations = append(recommendations, fmt.Sprintf("Review error handling in scenario '%s' - error rate: %.2f%%",
				result.ScenarioName, errorRate*100))
		}
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Workflow performed well under all stress scenarios")
	}

	return recommendations
}

// getFailedScenarios helper method to get failed scenarios
func (ws *WorkflowSimulator) getFailedScenarios(results []*StressTestScenarioResult) []*StressTestScenarioResult {
	failed := make([]*StressTestScenarioResult, 0)
	for _, result := range results {
		if !result.Success {
			failed = append(failed, result)
		}
	}
	return failed
}

// calculateRecoveryTime calculates recovery time for a scenario
func (ws *WorkflowSimulator) calculateRecoveryTime(scenario interface{}) time.Duration {
	// Type assertion and scenario-specific logic
	switch s := scenario.(type) {
	case *FailureScenario:
		switch s.Type {
		case "network_failure":
			return time.Duration(45) * time.Second
		case "database_failure":
			return time.Duration(120) * time.Second
		case "pod_failure":
			return time.Duration(15) * time.Second
		default:
			return time.Duration(30) * time.Second
		}
	case *LoadScenario:
		// Scale based on concurrent users
		return time.Duration(10+s.ConcurrentUsers/10) * time.Second
	default:
		ws.log.Warn("Unknown scenario type for recovery time calculation")
		return time.Duration(30) * time.Second
	}
}

// calculateImpact calculates the impact of a failure
func (ws *WorkflowSimulator) calculateImpact(scenario interface{}, failure *FailureScenario) string {
	// Simulate impact calculation based on failure type
	switch failure.Type {
	case "network_failure":
		return "medium"
	case "database_failure":
		return "high"
	case "pod_failure":
		return "low"
	default:
		return "unknown"
	}
}

// countAffectedSteps counts the number of steps affected by a failure
func (ws *WorkflowSimulator) countAffectedSteps(scenario interface{}, failure *FailureScenario) int {
	// Base affected steps on failure severity and scope
	var baseSteps int

	// Adjust based on failure type severity
	switch failure.Type {
	case "database_failure":
		baseSteps = 3 // Database failures affect multiple dependent steps
	case "network_failure":
		baseSteps = 2 // Network failures affect communication steps
	case "node_failure":
		baseSteps = 4 // Node failures can affect many pods/services
	case "pod_failure":
		baseSteps = 1 // Pod failures are usually localized
	default:
		baseSteps = 1
	}

	// Adjust based on scenario type and scope
	switch s := scenario.(type) {
	case *LoadScenario:
		// Higher load scenarios may amplify failure impact
		if s.ConcurrentUsers > 50 {
			baseSteps += 1
		}
	case *FailureScenario:
		// Multiple failure scenarios compound the impact
		if s.Duration > time.Minute*5 {
			baseSteps += 1
		}
	}

	return baseSteps
}

// wasRollbackTriggered checks if rollback was triggered
func (ws *WorkflowSimulator) wasRollbackTriggered(scenario interface{}) bool {
	// Determine rollback trigger based on scenario severity and type
	switch s := scenario.(type) {
	case *FailureScenario:
		// Rollback more likely for severe, long-duration failures
		switch s.Type {
		case "database_failure":
			return s.Duration > time.Minute*2 // Critical systems trigger rollback faster
		case "node_failure":
			return s.Duration > time.Minute*5 // Node failures allow more recovery time
		case "network_failure":
			return s.Duration > time.Minute*3 // Network issues have medium tolerance
		default:
			return s.Duration > time.Minute*10 // Other failures have higher tolerance
		}
	case *LoadScenario:
		// High load scenarios rarely trigger automatic rollback
		return s.ConcurrentUsers > 200 // Only extreme load triggers rollback
	case *StressTestScenarioResult:
		// Rollback if stress test shows critical resource exhaustion
		return !s.Success && s.RequestsHandled < 10
	default:
		// Unknown scenarios default to no rollback
		ws.log.Debug("Unknown scenario type for rollback detection")
		return false
	}
}

// calculateResilienceScore calculates overall resilience score
func (ws *WorkflowSimulator) calculateResilienceScore(results []*FailureInjectionResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	successCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		}
	}

	return float64(successCount) / float64(len(results))
}

// calculateRecoveryMetrics calculates recovery metrics
func (ws *WorkflowSimulator) calculateRecoveryMetrics(results []*FailureInjectionResult) map[string]interface{} {
	metrics := make(map[string]interface{})

	if len(results) == 0 {
		return metrics
	}

	var totalRecoveryTime time.Duration
	for _, result := range results {
		totalRecoveryTime += result.RecoveryTime
	}

	metrics["average_recovery_time"] = totalRecoveryTime / time.Duration(len(results))
	metrics["total_failures"] = len(results)

	return metrics
}

// generateResilienceRecommendations generates recommendations for improving resilience
func (ws *WorkflowSimulator) generateResilienceRecommendations(results []*FailureInjectionResult) []string {
	recommendations := make([]string, 0)

	failedResults := 0
	for _, result := range results {
		if !result.Success {
			failedResults++
		}
	}

	if failedResults > 0 {
		recommendations = append(recommendations, "Consider implementing better error handling and recovery mechanisms")
		recommendations = append(recommendations, "Add circuit breaker patterns for external dependencies")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Workflow shows good resilience to failure scenarios")
	}

	return recommendations
}

// Missing baseline creation methods for ResourceImpactAnalysis

// createBaselineNodes creates baseline node information
func (ws *WorkflowSimulator) createBaselineNodes() map[string]interface{} {
	// Create realistic baseline based on typical cluster configurations
	nodeCount := 3 // Default small cluster

	// Vary node count based on configuration
	if ws.config != nil && ws.config.ResourceLimits.MaxNodes > 0 {
		// Scale baseline to be 30% of max capacity for realistic simulation
		nodeCount = int(float64(ws.config.ResourceLimits.MaxNodes) * 0.3)
		if nodeCount < 1 {
			nodeCount = 1
		}
		if nodeCount > 10 {
			nodeCount = 10 // Cap at reasonable baseline
		}
	}

	// Generate realistic node capacities
	var cpuPerNode string
	var memoryPerNode string

	// Vary capacity based on cluster size
	switch {
	case nodeCount <= 3:
		cpuPerNode = "4"
		memoryPerNode = "8Gi"
	case nodeCount <= 6:
		cpuPerNode = "8"
		memoryPerNode = "16Gi"
	default:
		cpuPerNode = "16"
		memoryPerNode = "32Gi"
	}

	availableNodes := nodeCount // Assume all nodes available initially

	return map[string]interface{}{
		"total_nodes":     nodeCount,
		"available_nodes": availableNodes,
		"node_capacity":   map[string]string{"cpu": cpuPerNode, "memory": memoryPerNode},
		"cluster_size":    "small", // Could be "small", "medium", "large"
	}
}

// createBaselineNamespaces creates baseline namespace information
func (ws *WorkflowSimulator) createBaselineNamespaces() map[string]interface{} {
	// Base namespaces that always exist
	baseNamespaces := []string{"default", "kube-system"}
	activeNamespaces := make([]string, len(baseNamespaces))
	copy(activeNamespaces, baseNamespaces)

	// Add common operational namespaces
	commonNamespaces := []string{"monitoring", "logging", "ingress-nginx", "cert-manager"}

	// Determine how many additional namespaces based on cluster configuration
	additionalCount := 3 // Default
	if ws.config != nil {
		// Scale namespaces with cluster size and concurrent simulation capacity
		if ws.config.MaxConcurrentSims > 0 {
			additionalCount = ws.config.MaxConcurrentSims / 10 // 1 additional namespace per 10 sims
			if additionalCount < 2 {
				additionalCount = 2
			}
			if additionalCount > 10 {
				additionalCount = 10
			}
		}
	}

	// Add additional namespaces up to the calculated count
	for i := 0; i < additionalCount && i < len(commonNamespaces); i++ {
		activeNamespaces = append(activeNamespaces, commonNamespaces[i])
	}

	// Add application namespaces if we have room
	if len(activeNamespaces) < additionalCount+len(baseNamespaces) {
		appNamespaces := []string{"production", "staging", "development"}
		for _, ns := range appNamespaces {
			if len(activeNamespaces) < additionalCount+len(baseNamespaces) {
				activeNamespaces = append(activeNamespaces, ns)
			}
		}
	}

	return map[string]interface{}{
		"total_namespaces":  len(activeNamespaces),
		"active_namespaces": activeNamespaces,
		"system_namespaces": baseNamespaces,
		"app_namespaces":    len(activeNamespaces) - len(baseNamespaces),
	}
}

// createBaselineDeployments creates baseline deployment information
func (ws *WorkflowSimulator) createBaselineDeployments() map[string]interface{} {
	// Calculate realistic deployment counts based on cluster configuration
	totalDeployments := 10 // Default for small cluster

	if ws.config != nil && ws.config.ResourceLimits.MaxDeployments > 0 {
		// Use 20% of max capacity as baseline
		totalDeployments = int(float64(ws.config.ResourceLimits.MaxDeployments) * 0.2)
		if totalDeployments < 5 {
			totalDeployments = 5 // Minimum for realistic cluster
		}
		if totalDeployments > 50 {
			totalDeployments = 50 // Cap for baseline
		}
	}

	// Simulate realistic running rate (80-95%)
	runningRate := 0.8 + (float64(totalDeployments%10) * 0.015) // Vary between 80-95%
	runningDeployments := int(float64(totalDeployments) * runningRate)

	// Calculate average replicas based on deployment count
	var averageReplicas int
	switch {
	case totalDeployments <= 10:
		averageReplicas = 2
	case totalDeployments <= 30:
		averageReplicas = 3
	default:
		averageReplicas = 4
	}

	// Calculate failed and pending deployments
	failedDeployments := totalDeployments - runningDeployments
	pendingDeployments := failedDeployments / 2 // Half of non-running are pending
	actualFailedDeployments := failedDeployments - pendingDeployments

	return map[string]interface{}{
		"total_deployments":   totalDeployments,
		"running_deployments": runningDeployments,
		"pending_deployments": pendingDeployments,
		"failed_deployments":  actualFailedDeployments,
		"average_replicas":    averageReplicas,
		"total_pods":          totalDeployments * averageReplicas,
		"healthy_rate":        runningRate,
	}
}

// createBaselineMetrics creates baseline metrics
func (ws *WorkflowSimulator) createBaselineMetrics() map[string]interface{} {
	// Generate realistic baseline metrics that vary based on "time of day"
	currentHour := ws.simulationNow().Hour()

	// Simulate daily usage patterns
	var cpuUsage, memoryUsage float64
	var networkThroughput string

	// Business hours (9-17) have higher resource usage
	if currentHour >= 9 && currentHour <= 17 {
		// Peak hours - higher usage
		cpuUsage = 0.55 + float64(currentHour-12)*0.02 // Peak around noon
		memoryUsage = 0.65 + float64(currentHour-12)*0.015
		networkThroughput = "150Mbps"
	} else if currentHour >= 7 && currentHour <= 21 {
		// Extended business hours - moderate usage
		cpuUsage = 0.35 + float64(currentHour%12)*0.01
		memoryUsage = 0.45 + float64(currentHour%12)*0.01
		networkThroughput = "100Mbps"
	} else {
		// Off hours - lower usage
		cpuUsage = 0.20 + float64(currentHour%6)*0.005
		memoryUsage = 0.30 + float64(currentHour%6)*0.005
		networkThroughput = "50Mbps"
	}

	// Ensure values stay within reasonable bounds
	if cpuUsage > 0.85 {
		cpuUsage = 0.85
	}
	if cpuUsage < 0.15 {
		cpuUsage = 0.15
	}
	if memoryUsage > 0.90 {
		memoryUsage = 0.90
	}
	if memoryUsage < 0.25 {
		memoryUsage = 0.25
	}

	// Add some cluster configuration influence
	if ws.config != nil {
		// Higher concurrent simulations indicate more active cluster
		if ws.config.MaxConcurrentSims > 30 {
			cpuUsage += 0.10
			memoryUsage += 0.05
			networkThroughput = "200Mbps"
		}
	}

	return map[string]interface{}{
		"cpu_usage_avg":      cpuUsage,
		"memory_usage_avg":   memoryUsage,
		"network_throughput": networkThroughput,
		"disk_io_avg":        cpuUsage * 0.6, // Correlate with CPU
		"current_hour":       currentHour,
		"load_profile":       ws.determineLoadProfile(currentHour),
	}
}

// analyzeResourceConsumption analyzes resource consumption patterns
func (ws *WorkflowSimulator) analyzeResourceConsumption(result *WorkflowSimulationResult) map[string]interface{} {
	// Base consumption on actual simulation results
	cpuIncrease := 0.10 // Default base increase
	memoryIncrease := 0.15
	storageUsed := "1Gi"

	if result != nil {
		// Scale based on execution duration - longer executions use more resources
		durationMinutes := result.Duration.Minutes()
		cpuIncrease += durationMinutes * 0.02    // 2% per minute
		memoryIncrease += durationMinutes * 0.01 // 1% per minute

		// Scale based on number of steps executed
		if len(result.StepResults) > 0 {
			stepMultiplier := float64(len(result.StepResults)) * 0.05
			cpuIncrease += stepMultiplier
			memoryIncrease += stepMultiplier

			// Storage scales with step count
			storageGB := 1 + len(result.StepResults)/5 // 1GB base + 1GB per 5 steps
			storageUsed = fmt.Sprintf("%dGi", storageGB)
		}

		// Account for failure overhead
		if !result.Success {
			cpuIncrease += 0.05 // Failed executions use more CPU for retries/cleanup
			memoryIncrease += 0.03
		}
	}

	return map[string]interface{}{
		"cpu_increase":    cpuIncrease,
		"memory_increase": memoryIncrease,
		"storage_used":    storageUsed,
	}
}

// calculatePeakUsage calculates peak resource usage
func (ws *WorkflowSimulator) calculatePeakUsage(result *WorkflowSimulationResult) map[string]interface{} {
	// Calculate realistic peak usage based on simulation data
	peakCPU := 0.60     // Base peak CPU
	peakMemory := 0.55  // Base peak memory
	peakTime := "12:00" // Default peak time

	if result != nil {
		// Peak usage correlates with workflow complexity and duration
		complexityFactor := float64(len(result.StepResults)) * 0.05
		peakCPU += complexityFactor
		peakMemory += complexityFactor * 0.8

		// Failed workflows often have higher peak usage due to retries
		if !result.Success {
			peakCPU += 0.15
			peakMemory += 0.10
		}

		// Ensure we don't exceed realistic limits
		if peakCPU > 0.95 {
			peakCPU = 0.95
		}
		if peakMemory > 0.90 {
			peakMemory = 0.90
		}

		// Calculate peak time based on when execution started (simulated)
		if !result.StartedAt.IsZero() {
			// Peak usually occurs midway through execution
			midpoint := result.StartedAt.Add(result.Duration / 2)
			peakTime = midpoint.Format("15:04")
		}
	}

	return map[string]interface{}{
		"peak_cpu":    peakCPU,
		"peak_memory": peakMemory,
		"peak_time":   peakTime,
	}
}

// identifyResourceConflicts identifies potential resource conflicts
func (ws *WorkflowSimulator) identifyResourceConflicts(result *WorkflowSimulationResult) []string {
	conflicts := make([]string, 0)

	if result == nil {
		return conflicts
	}

	// Analyze execution patterns to identify realistic conflicts
	if result.Duration > time.Minute*10 {
		conflicts = append(conflicts, "Long execution time may indicate resource bottlenecks")
	}

	if !result.Success {
		conflicts = append(conflicts, "Execution failure may be due to resource exhaustion")
	}

	// Check for high step count which may cause resource contention
	if len(result.StepResults) > 10 {
		conflicts = append(conflicts, "High step count may cause memory contention between pods")
	}

	// Analyze step execution patterns
	failedSteps := 0
	longRunningSteps := 0

	for _, step := range result.StepResults {
		if step != nil {
			if step.Duration > time.Minute*2 {
				longRunningSteps++
			}
			if !step.Success {
				failedSteps++
			}
		}
	}

	if longRunningSteps > 3 {
		conflicts = append(conflicts, "Multiple long-running steps detected - CPU throttling likely")
	}

	if failedSteps > 2 {
		conflicts = append(conflicts, "Multiple step failures suggest resource allocation issues")
	}

	// If no specific conflicts found but execution was problematic
	if len(conflicts) == 0 && (!result.Success || result.Duration > time.Minute*15) {
		conflicts = append(conflicts, "Potential resource conflicts detected based on execution patterns")
	}

	return conflicts
}

// calculateScalingRequirements calculates scaling requirements
func (ws *WorkflowSimulator) calculateScalingRequirements(template *WorkflowTemplate, report *ResourceImpactReport) map[string]interface{} {
	// Calculate realistic scaling requirements based on template complexity and impact analysis
	cpuIncrease := "25%"    // Default conservative increase
	memoryIncrease := "20%" // Default conservative increase
	suggestedReplicas := 2  // Default
	horizontalScaling := true

	if template != nil {
		// Scale recommendations based on workflow complexity
		stepCount := len(template.Steps)

		// More complex workflows need more resources
		switch {
		case stepCount <= 5:
			cpuIncrease = "25%"
			memoryIncrease = "20%"
			suggestedReplicas = 2
		case stepCount <= 15:
			cpuIncrease = "40%"
			memoryIncrease = "30%"
			suggestedReplicas = 3
		default:
			cpuIncrease = "60%"
			memoryIncrease = "45%"
			suggestedReplicas = 5
		}

		// Check for resource-intensive actions
		resourceIntensiveCount := 0
		for _, step := range template.Steps {
			if step.Action != nil {
				switch step.Action.Type {
				case "scale_deployment", "create_resource", "collect_diagnostics":
					resourceIntensiveCount++
				}
			}
		}

		// Adjust for resource-intensive workflows
		if resourceIntensiveCount > stepCount/3 {
			cpuIncrease = ws.increasePercentage(cpuIncrease, 20)
			memoryIncrease = ws.increasePercentage(memoryIncrease, 15)
			suggestedReplicas++
		}
	}

	if report != nil {
		// Factor in actual impact analysis
		if consumption, ok := report.ResourceConsumption["cpu_increase"].(float64); ok {
			if consumption > 0.5 {
				cpuIncrease = ws.increasePercentage(cpuIncrease, 25)
			}
		}
		if consumption, ok := report.ResourceConsumption["memory_increase"].(float64); ok {
			if consumption > 0.4 {
				memoryIncrease = ws.increasePercentage(memoryIncrease, 20)
			}
		}

		// Consider conflicts for scaling strategy
		if len(report.ResourceConflicts) > 3 {
			horizontalScaling = true // Prefer horizontal scaling when conflicts exist
			suggestedReplicas += len(report.ResourceConflicts) / 2
		} else if len(report.ResourceConflicts) == 0 {
			horizontalScaling = false // Vertical scaling may be sufficient
		}
	}

	// Cap suggestions at reasonable limits
	if suggestedReplicas > 10 {
		suggestedReplicas = 10
	}

	return map[string]interface{}{
		"recommended_cpu_increase":    cpuIncrease,
		"recommended_memory_increase": memoryIncrease,
		"suggested_replicas":          suggestedReplicas,
		"horizontal_scaling":          horizontalScaling,
		"scaling_rationale":           ws.generateScalingRationale(template, report),
	}
}

// estimateResourceCosts estimates resource costs
func (ws *WorkflowSimulator) estimateResourceCosts(template *WorkflowTemplate, consumption map[string]interface{}) map[string]interface{} {
	// Base cost rates (simulated cloud pricing)
	cpuCostPerCoreHour := 0.05    // $0.05 per core hour
	memoryCostPerGBHour := 0.01   // $0.01 per GB hour
	storageCostPerGBMonth := 0.10 // $0.10 per GB month

	// Default resource usage if no consumption data
	cpuHours := 100.0 // Default baseline
	memoryGBHours := 200.0
	storageGBMonths := 50.0

	// Extract actual consumption if available
	if consumption != nil {
		if cpuInc, ok := consumption["cpu_increase"].(float64); ok {
			// Convert percentage increase to actual hours
			cpuHours = 100.0 * (1.0 + cpuInc) // Base 100 hours + increase
		}
		if memInc, ok := consumption["memory_increase"].(float64); ok {
			memoryGBHours = 200.0 * (1.0 + memInc) // Base 200 GB-hours + increase
		}
		if storage, ok := consumption["storage_used"].(string); ok {
			// Parse storage string like "5Gi"
			if strings.HasSuffix(storage, "Gi") {
				if storageVal := ws.parseStorageValue(storage); storageVal > 0 {
					storageGBMonths = storageVal
				}
			}
		}
	}

	// Factor in template complexity
	if template != nil {
		complexityMultiplier := 1.0 + float64(len(template.Steps))*0.05 // 5% per step
		if complexityMultiplier > 2.0 {
			complexityMultiplier = 2.0 // Cap at 2x
		}
		cpuHours *= complexityMultiplier
		memoryGBHours *= complexityMultiplier
	}

	// Calculate costs
	cpuCost := cpuHours * cpuCostPerCoreHour
	memCost := memoryGBHours * memoryCostPerGBHour
	storageCost := storageGBMonths * storageCostPerGBMonth
	totalCost := cpuCost + memCost + storageCost

	return map[string]interface{}{
		"estimated_monthly_cost": fmt.Sprintf("$%.2f", totalCost),
		"cpu_cost":               fmt.Sprintf("$%.2f", cpuCost),
		"memory_cost":            fmt.Sprintf("$%.2f", memCost),
		"storage_cost":           fmt.Sprintf("$%.2f", storageCost),
		"cpu_hours":              cpuHours,
		"memory_gb_hours":        memoryGBHours,
		"storage_gb_months":      storageGBMonths,
	}
}

// calculateUtilizationEfficiency calculates utilization efficiency
func (ws *WorkflowSimulator) calculateUtilizationEfficiency(template *WorkflowTemplate, consumption, peak map[string]interface{}) float64 {
	// Calculate efficiency based on resource utilization patterns
	baseEfficiency := 0.70 // Start with 70% base efficiency

	if consumption == nil || peak == nil {
		return baseEfficiency
	}

	// Get CPU efficiency
	cpuEfficiency := 0.70
	if cpuInc, ok := consumption["cpu_increase"].(float64); ok {
		if peakCPU, ok := peak["peak_cpu"].(float64); ok {
			// Good efficiency when consumption is close to peak (not over-provisioned)
			utilization := cpuInc / peakCPU
			if utilization > 0.8 && utilization < 1.1 {
				cpuEfficiency = 0.90 // High efficiency
			} else if utilization > 0.5 && utilization <= 0.8 {
				cpuEfficiency = 0.75 // Good efficiency
			} else {
				cpuEfficiency = 0.60 // Lower efficiency
			}
		}
	}

	// Get Memory efficiency
	memEfficiency := 0.70
	if memInc, ok := consumption["memory_increase"].(float64); ok {
		if peakMem, ok := peak["peak_memory"].(float64); ok {
			utilization := memInc / peakMem
			if utilization > 0.8 && utilization < 1.1 {
				memEfficiency = 0.85
			} else if utilization > 0.5 && utilization <= 0.8 {
				memEfficiency = 0.75
			} else {
				memEfficiency = 0.55
			}
		}
	}

	// Factor in template characteristics
	workflowEfficiency := 1.0
	if template != nil {
		// Well-structured workflows are more efficient
		stepCount := len(template.Steps)
		if stepCount > 0 {
			// Optimal step count is 5-15 steps
			if stepCount >= 5 && stepCount <= 15 {
				workflowEfficiency = 1.1
			} else if stepCount > 20 {
				workflowEfficiency = 0.9 // Too complex
			} else if stepCount < 3 {
				workflowEfficiency = 0.95 // Too simple, overhead dominant
			}
		}

		// Proper recovery configuration improves efficiency
		if template.Recovery != nil && template.Recovery.Enabled {
			workflowEfficiency += 0.05
		}
	}

	// Calculate weighted average efficiency
	overallEfficiency := (cpuEfficiency*0.4 + memEfficiency*0.4 + baseEfficiency*0.2) * workflowEfficiency

	// Ensure efficiency stays within bounds
	if overallEfficiency > 1.0 {
		overallEfficiency = 1.0
	}
	if overallEfficiency < 0.1 {
		overallEfficiency = 0.1
	}

	return overallEfficiency
}

// generateResourceRecommendations generates resource recommendations
func (ws *WorkflowSimulator) generateResourceRecommendations(result *WorkflowSimulationResult) []string {
	recommendations := make([]string, 0)

	recommendations = append(recommendations, "Consider implementing resource quotas for better resource management")
	recommendations = append(recommendations, "Monitor resource usage patterns for optimization opportunities")

	if !result.Success {
		recommendations = append(recommendations, "Review resource allocation - simulation failed due to resource constraints")
	}

	return recommendations
}

// Missing helper methods for simulation environment initialization

// loadInitialState simulates loading initial state into mock Kubernetes client
func (ws *WorkflowSimulator) loadInitialState(initialState *ClusterState) error {
	if initialState == nil {
		return fmt.Errorf("initial state cannot be nil")
	}

	// Configure mock Kubernetes client with state
	if ws.mockK8sClient != nil {
		// In a real implementation, this would populate the mock client
		// with the deployments, pods, services, etc. from initialState
		ws.log.WithFields(logrus.Fields{
			"deployments": len(initialState.Deployments),
			"pods":        len(initialState.Pods),
			"services":    len(initialState.Services),
		}).Debug("Loading initial cluster state into mock client")
	}

	// Configure mock action repository with historical data
	if ws.mockActionRepo != nil {
		// In a real implementation, this would load action history
		ws.log.Debug("Loading action history into mock repository")
	}

	// Configure mock metrics client with baseline metrics
	if ws.mockMetrics != nil && initialState.Metrics != nil {
		// In a real implementation, this would set up baseline metrics
		for metric, value := range initialState.Metrics {
			ws.log.WithFields(logrus.Fields{
				"metric": metric,
				"value":  value,
			}).Debug("Setting baseline metric")
		}
	}

	ws.log.WithField("timestamp", initialState.Timestamp).Info("Successfully loaded initial cluster state")
	return nil
}

// initializeResourceState simulates resource state initialization
func (ws *WorkflowSimulator) initializeResourceState(initialState *ClusterState) {
	if initialState == nil {
		ws.log.Warn("Cannot initialize resource state: initial state is nil")
		return
	}

	// Initialize resource modeler with current state
	if ws.resourceModeler != nil {
		ws.log.WithFields(logrus.Fields{
			"nodes":       len(initialState.Nodes),
			"deployments": len(initialState.Deployments),
		}).Debug("Configuring resource modeler with initial state")

		// In a real implementation, this would configure the resource modeler
		// to track resource usage and capacity based on the initial state
	}

	// Calculate initial resource utilization
	if initialState.Metrics == nil {
		initialState.Metrics = make(map[string]float64)
	}

	// Set initial resource utilization based on deployments and pods
	totalCPURequests := 0.0
	totalMemoryRequests := 0.0

	for _, deployment := range initialState.Deployments {
		// Estimate resource usage per deployment
		cpuPerReplica := 0.1      // 100m CPU per replica
		memoryPerReplica := 128.0 // 128Mi memory per replica

		totalCPURequests += float64(deployment.Replicas) * cpuPerReplica
		totalMemoryRequests += float64(deployment.Replicas) * memoryPerReplica
	}

	// Assume baseline cluster capacity
	totalCPUCapacity := float64(len(initialState.Nodes)) * 4.0       // 4 CPU cores per node
	totalMemoryCapacity := float64(len(initialState.Nodes)) * 8192.0 // 8Gi memory per node

	if totalCPUCapacity > 0 {
		initialState.Metrics["cpu_utilization"] = totalCPURequests / totalCPUCapacity
	}
	if totalMemoryCapacity > 0 {
		initialState.Metrics["memory_utilization"] = totalMemoryRequests / totalMemoryCapacity
	}

	ws.log.WithFields(logrus.Fields{
		"cpu_utilization":    initialState.Metrics["cpu_utilization"],
		"memory_utilization": initialState.Metrics["memory_utilization"],
		"total_deployments":  len(initialState.Deployments),
	}).Info("Initialized resource state tracking")
}

// scheduleFailure simulates failure injection scheduling
func (ws *WorkflowSimulator) scheduleFailure(failure *FailureScenario) {
	if failure == nil {
		ws.log.Warn("Cannot schedule nil failure scenario")
		return
	}

	if ws.failureInjector == nil {
		ws.log.Warn("Failure injector not available, skipping failure scheduling")
		return
	}

	// Calculate when to trigger the failure
	triggerTime := failure.TriggerTime
	if triggerTime.IsZero() {
		// If no trigger time specified, schedule it randomly within the scenario duration
		// For simulation purposes, trigger early in the execution
		triggerTime = time.Now().Add(time.Second * 30) // 30 seconds from now
	}

	// Validate failure parameters
	if err := ws.validateFailureScenario(failure); err != nil {
		ws.log.WithError(err).WithField("failure_id", failure.ID).Error("Invalid failure scenario")
		return
	}

	// In a real implementation, this would use a scheduler/timer to trigger the failure
	// For simulation, we'll log the scheduling and store for later injection
	ws.log.WithFields(logrus.Fields{
		"failure_id":   failure.ID,
		"failure_type": failure.Type,
		"trigger_time": triggerTime,
		"duration":     failure.Duration,
		"description":  failure.Description,
	}).Info("Scheduled failure injection")

	// Store failure for later injection during step execution
	// In a real implementation, this would be stored in the failure injector
}

// validateFailureScenario validates a failure scenario configuration
func (ws *WorkflowSimulator) validateFailureScenario(failure *FailureScenario) error {
	if failure.ID == "" {
		return fmt.Errorf("failure scenario must have an ID")
	}

	if failure.Type == "" {
		return fmt.Errorf("failure scenario must have a type")
	}

	// Validate known failure types
	validTypes := []string{"network_failure", "database_failure", "node_failure", "pod_failure", "disk_failure", "memory_pressure"}
	validType := false
	for _, vt := range validTypes {
		if failure.Type == vt {
			validType = true
			break
		}
	}
	if !validType {
		return fmt.Errorf("unknown failure type: %s", failure.Type)
	}

	if failure.Duration <= 0 {
		return fmt.Errorf("failure duration must be positive")
	}

	// Validate duration is reasonable (not too long for simulation)
	maxDuration := time.Hour * 2
	if failure.Duration > maxDuration {
		return fmt.Errorf("failure duration %v exceeds maximum allowed %v", failure.Duration, maxDuration)
	}

	return nil
}

// checkSafetyViolations checks for safety violations during simulation
func (ws *WorkflowSimulator) checkSafetyViolations(simCtx *SimulationContext) []*SafetyViolation {
	violations := make([]*SafetyViolation, 0)

	// Check resource limits violations
	if simCtx.State != nil {
		// Check pod count limits
		podCount := len(simCtx.State.Pods)
		if ws.config != nil && podCount > ws.config.ResourceLimits.MaxPods {
			violations = append(violations, &SafetyViolation{
				Type:        "resource_limit",
				Severity:    "high",
				Description: fmt.Sprintf("Pod count (%d) exceeds limit (%d)", podCount, ws.config.ResourceLimits.MaxPods),
				Timestamp:   ws.simulationNow(),
			})
		}

		// Check deployment count limits
		deploymentCount := len(simCtx.State.Deployments)
		if ws.config != nil && deploymentCount > ws.config.ResourceLimits.MaxDeployments {
			violations = append(violations, &SafetyViolation{
				Type:        "resource_limit",
				Severity:    "medium",
				Description: fmt.Sprintf("Deployment count (%d) exceeds limit (%d)", deploymentCount, ws.config.ResourceLimits.MaxDeployments),
				Timestamp:   ws.simulationNow(),
			})
		}
	}

	// Check for concurrent operation violations
	activeSteps := 0
	for _, stepResult := range simCtx.StepResults {
		if stepResult.Success && stepResult.Duration > time.Minute*5 {
			activeSteps++
		}
	}
	if activeSteps > 10 { // Too many long-running operations
		violations = append(violations, &SafetyViolation{
			Type:        "concurrency_limit",
			Severity:    "medium",
			Description: fmt.Sprintf("Too many concurrent long-running operations (%d)", activeSteps),
			Timestamp:   ws.simulationNow(),
		})
	}

	// Check for destructive operations without safeguards
	if simCtx.Template != nil {
		for _, step := range simCtx.Template.Steps {
			if step.Action != nil && ws.isDestructiveAction(step.Action.Type) {
				if step.Action.Rollback == nil {
					violations = append(violations, &SafetyViolation{
						Type:        "safety_policy",
						Severity:    "high",
						Description: fmt.Sprintf("Destructive operation '%s' in step '%s' lacks rollback policy", step.Action.Type, step.Name),
						Timestamp:   ws.simulationNow(),
					})
				}
			}
		}
	}

	return violations
}

// isDestructiveAction checks if an action type is destructive
func (ws *WorkflowSimulator) isDestructiveAction(actionType string) bool {
	destructiveActions := []string{"delete", "remove", "drain", "scale_down", "terminate"}
	actionLower := strings.ToLower(actionType)
	for _, destructive := range destructiveActions {
		if strings.Contains(actionLower, destructive) {
			return true
		}
	}
	return false
}

// updateSimulationState updates the simulation state
func (ws *WorkflowSimulator) updateSimulationState(simCtx *SimulationContext, step *WorkflowStep) {
	ws.log.WithFields(logrus.Fields{
		"step_id":       step.ID,
		"simulation_id": simCtx.ID,
	}).Debug("Updating simulation state")

	// Update metrics based on step execution
	if simCtx.Metrics == nil {
		simCtx.Metrics = make(map[string]float64)
	}

	// Track resource usage based on step type
	if step.Action != nil {
		switch step.Action.Type {
		case "scale_deployment":
			simCtx.Metrics["scale_operations"]++
			if replicas, ok := step.Action.Parameters["replicas"].(int); ok && replicas > 0 {
				simCtx.Metrics["total_replicas"] += float64(replicas)
			}
		case "restart_pod":
			simCtx.Metrics["restart_operations"]++
		case "create_resource":
			simCtx.Metrics["create_operations"]++
		case "delete_resource":
			simCtx.Metrics["delete_operations"]++
		}
	}

	// Update state resources based on actions
	if step.Action != nil && step.Action.Target != nil && simCtx.State != nil {
		switch step.Action.Type {
		case "scale_deployment":
			ws.updateDeploymentInState(simCtx.State, step.Action.Target.Namespace, step.Action.Target.Name, step.Action.Parameters)
		case "create_resource":
			ws.addResourceToState(simCtx.State, step.Action.Target, step.Action.Parameters)
		case "delete_resource":
			ws.removeResourceFromState(simCtx.State, step.Action.Target)
		}
	}

	// Update timestamp
	if simCtx.State != nil {
		simCtx.State.Timestamp = ws.simulationNow()
	}
}

// updateDeploymentInState updates deployment in simulated state
func (ws *WorkflowSimulator) updateDeploymentInState(state *ClusterState, namespace, name string, params map[string]interface{}) {
	for _, deployment := range state.Deployments {
		if deployment.Namespace == namespace && deployment.Name == name {
			if replicas, ok := params["replicas"].(int); ok {
				deployment.Replicas = replicas
				ws.log.WithFields(logrus.Fields{
					"namespace":  namespace,
					"deployment": name,
					"replicas":   replicas,
				}).Debug("Updated deployment in simulation state")
			}
			return
		}
	}
}

// addResourceToState adds new resource to simulated state
func (ws *WorkflowSimulator) addResourceToState(state *ClusterState, target *ActionTarget, params map[string]interface{}) {
	switch target.Resource {
	case "deployment":
		replicas := 1
		if r, ok := params["replicas"].(int); ok {
			replicas = r
		}
		state.Deployments = append(state.Deployments, &SimulatedDeployment{
			Name:      target.Name,
			Namespace: target.Namespace,
			Replicas:  replicas,
		})
		ws.log.WithFields(logrus.Fields{
			"namespace":  target.Namespace,
			"deployment": target.Name,
		}).Debug("Added deployment to simulation state")
	}
}

// removeResourceFromState removes resource from simulated state
func (ws *WorkflowSimulator) removeResourceFromState(state *ClusterState, target *ActionTarget) {
	switch target.Resource {
	case "deployment":
		for i, deployment := range state.Deployments {
			if deployment.Namespace == target.Namespace && deployment.Name == target.Name {
				state.Deployments = append(state.Deployments[:i], state.Deployments[i+1:]...)
				ws.log.WithFields(logrus.Fields{
					"namespace":  target.Namespace,
					"deployment": target.Name,
				}).Debug("Removed deployment from simulation state")
				return
			}
		}
	}
}

// simulationNow provides simulated current time for time acceleration
func (ws *WorkflowSimulator) simulationNow() time.Time {
	if ws.timeAccelerator != nil && ws.config != nil && ws.config.TimeAcceleration > 1.0 {
		// Calculate accelerated time based on simulation start
		// In real implementation, TimeAccelerator would track simulation start time
		return time.Now() // For now, return real time but with acceleration awareness
	}
	return time.Now()
}

// simulationSince calculates time elapsed since start in simulation time
func (ws *WorkflowSimulator) simulationSince(start time.Time) time.Duration {
	duration := time.Since(start)
	if ws.timeAccelerator != nil && ws.config != nil && ws.config.TimeAcceleration > 1.0 {
		// Accelerate the perceived duration
		acceleratedDuration := time.Duration(float64(duration) / ws.config.TimeAcceleration)
		ws.log.WithFields(logrus.Fields{
			"real_duration":        duration,
			"accelerated_duration": acceleratedDuration,
			"acceleration_factor":  ws.config.TimeAcceleration,
		}).Debug("Applied time acceleration")
		return acceleratedDuration
	}
	return duration
}

// Additional missing simulation methods

// simulateConditionStep simulates a condition step
func (ws *WorkflowSimulator) simulateConditionStep(simCtx *SimulationContext, step *WorkflowStep) (*StepSimulationResult, error) {
	ws.log.WithFields(logrus.Fields{
		"simulation_id": simCtx.ID,
		"step_id":       step.ID,
	}).Debug("Simulating condition step")

	return &StepSimulationResult{
		StepID:   step.ID,
		Success:  true,
		Duration: time.Millisecond * 100,
		Errors:   []string{},
		Outputs:  map[string]interface{}{"condition_result": true},
	}, nil
}

// simulateWaitStep simulates a wait step
func (ws *WorkflowSimulator) simulateWaitStep(simCtx *SimulationContext, step *WorkflowStep) (*StepSimulationResult, error) {
	ws.log.WithFields(logrus.Fields{
		"simulation_id": simCtx.ID,
		"step_id":       step.ID,
	}).Debug("Simulating wait step")

	// Simulate wait duration
	waitDuration := time.Second * 5 // Default wait
	if step.Action != nil && step.Action.Parameters != nil {
		if duration, ok := step.Action.Parameters["duration"].(time.Duration); ok {
			waitDuration = duration
		}
	}

	return &StepSimulationResult{
		StepID:   step.ID,
		Success:  true,
		Duration: waitDuration,
		Errors:   []string{},
		Outputs:  map[string]interface{}{"waited_duration": waitDuration.String()},
	}, nil
}

// checkForFailure simulates failure injection checking
func (ws *WorkflowSimulator) checkForFailure(stepID string, currentTime time.Time) *FailureScenario {
	// Simulate time-based and step-specific failure injection
	// This provides more realistic failure patterns for testing

	// Time-based failure simulation (failures more likely during "peak hours")
	hour := currentTime.Hour()
	isHighTrafficHour := hour >= 9 && hour <= 17 // Business hours

	// Step-specific failure probabilities
	failureProbability := 0.0
	switch {
	case stepID == "" || len(stepID) == 0:
		return nil // Invalid step ID
	case strings.Contains(stepID, "network"):
		failureProbability = 0.15 // Network steps have higher failure rate
	case strings.Contains(stepID, "database"):
		failureProbability = 0.10 // Database operations can fail
	case strings.Contains(stepID, "scale"):
		failureProbability = 0.08 // Scaling operations have some risk
	default:
		failureProbability = 0.05 // Base failure rate for other steps
	}

	// Increase failure probability during high traffic
	if isHighTrafficHour {
		failureProbability *= 1.5
	}

	// Simple pseudo-random check (in real implementation, use proper random seeding)
	// Using time and stepID for deterministic but varied results
	hash := len(stepID) + int(currentTime.Unix()%100)
	if float64(hash%100)/100.0 < failureProbability {
		// Return appropriate failure type based on step characteristics
		if strings.Contains(stepID, "network") {
			return &FailureScenario{
				Type:     "network_failure",
				Duration: time.Minute*2 + time.Duration(hash%60)*time.Second,
			}
		} else if strings.Contains(stepID, "database") {
			return &FailureScenario{
				Type:     "database_failure",
				Duration: time.Minute*3 + time.Duration(hash%120)*time.Second,
			}
		} else {
			return &FailureScenario{
				Type:     "pod_failure",
				Duration: time.Second*30 + time.Duration(hash%90)*time.Second,
			}
		}
	}

	return nil // No failure injected
}

// simulateNotificationAction simulates a notification action
func (ws *WorkflowSimulator) simulateNotificationAction(result *StepSimulationResult, action *StepAction) {
	// Simulate notification sending
	if result.Outputs == nil {
		result.Outputs = make(map[string]interface{})
	}

	// Extract notification details from action
	notificationType := "email" // Default
	message := "Workflow notification"
	recipient := "admin@example.com"

	if action.Parameters != nil {
		if nType, ok := action.Parameters["type"].(string); ok {
			notificationType = nType
		}
		if msg, ok := action.Parameters["message"].(string); ok {
			message = msg
		}
		if rec, ok := action.Parameters["recipient"].(string); ok {
			recipient = rec
		}
	}

	result.Outputs["notification_sent"] = true
	result.Outputs["notification_type"] = notificationType
	result.Outputs["message"] = message
	result.Outputs["recipient"] = recipient

	// Add small delay to simulate notification processing
	result.Duration += time.Millisecond * 50
}

// simulateRestartPod simulates restarting a pod
func (ws *WorkflowSimulator) simulateRestartPod(result *StepSimulationResult, action *StepAction) {
	if result.Outputs == nil {
		result.Outputs = make(map[string]interface{})
	}

	// Extract pod details from action
	podName := "default-pod"
	namespace := "default"
	gracePeriod := 30 // seconds

	if action.Target != nil {
		if action.Target.Name != "" {
			podName = action.Target.Name
		}
		if action.Target.Namespace != "" {
			namespace = action.Target.Namespace
		}
	}

	if action.Parameters != nil {
		if period, ok := action.Parameters["grace_period"].(int); ok {
			gracePeriod = period
		}
	}

	result.Outputs["pod_restarted"] = true
	result.Outputs["pod_name"] = podName
	result.Outputs["namespace"] = namespace
	result.Outputs["grace_period"] = gracePeriod
	result.Outputs["restart_time"] = ws.simulationNow().Format(time.RFC3339)
	result.Duration += time.Second * 2
}

// simulateDrainNode simulates draining a node
func (ws *WorkflowSimulator) simulateDrainNode(result *StepSimulationResult, action *StepAction) {
	if result.Outputs == nil {
		result.Outputs = make(map[string]interface{})
	}

	// Extract node details from action
	nodeName := "default-node"
	podsEvicted := 5 // Default
	forceDrain := false

	if action.Target != nil && action.Target.Name != "" {
		nodeName = action.Target.Name
	}

	if action.Parameters != nil {
		if force, ok := action.Parameters["force"].(bool); ok {
			forceDrain = force
		}
		if pods, ok := action.Parameters["expected_pods"].(int); ok {
			podsEvicted = pods
		}
	}

	result.Outputs["node_drained"] = true
	result.Outputs["node_name"] = nodeName
	result.Outputs["pods_evicted"] = podsEvicted
	result.Outputs["force_drain"] = forceDrain
	result.Duration += time.Second * 30 // Longer operation
}

// simulateExpandPVC simulates expanding a PVC
func (ws *WorkflowSimulator) simulateExpandPVC(result *StepSimulationResult, action *StepAction) {
	if result.Outputs == nil {
		result.Outputs = make(map[string]interface{})
	}

	// Extract PVC details from action parameters
	newSize := "20Gi" // Default
	if action.Parameters != nil {
		if size, ok := action.Parameters["new_size"].(string); ok {
			newSize = size
		}
	}

	result.Outputs["pvc_expanded"] = true
	result.Outputs["new_size"] = newSize
	result.Duration += time.Second * 10
}

// simulationSleep simulates sleep in accelerated time
func (ws *WorkflowSimulator) simulationSleep(duration time.Duration) {
	// In real implementation, this would use TimeAccelerator
	// For simulation, we just track the time conceptually
	ws.log.WithField("sleep_duration", duration).Debug("Simulating sleep")
}

// findSimulatedDeployment simulates finding a deployment in the cluster state
func (ws *WorkflowSimulator) findSimulatedDeployment(namespace, name string) *SimulatedDeployment {
	// Simulate finding a deployment - in real implementation this would query the ResourceStateModeler
	// For now, return a simulated deployment
	return &SimulatedDeployment{
		Name:      name,
		Namespace: namespace,
		Replicas:  3, // Default simulated replicas
	}
}

// updatePodsForDeployment simulates updating pods for a deployment
func (ws *WorkflowSimulator) updatePodsForDeployment(deployment *SimulatedDeployment, replicas int) {
	// Simulate updating pods - in real implementation this would use ResourceStateModeler
	deployment.Replicas = replicas
	ws.log.WithFields(map[string]interface{}{
		"deployment": deployment.Name,
		"namespace":  deployment.Namespace,
		"replicas":   replicas,
	}).Debug("Updated simulated deployment replicas")
}

// absInt returns the absolute value of an integer
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// determineLoadProfile determines the current load profile based on time
func (ws *WorkflowSimulator) determineLoadProfile(hour int) string {
	switch {
	case hour >= 9 && hour <= 12:
		return "high_morning"
	case hour >= 13 && hour <= 17:
		return "high_afternoon"
	case hour >= 18 && hour <= 21:
		return "moderate_evening"
	case hour >= 22 || hour <= 6:
		return "low_overnight"
	default:
		return "moderate_transition"
	}
}

// increasePercentage increases a percentage string by a given amount
func (ws *WorkflowSimulator) increasePercentage(original string, increaseBy int) string {
	// Parse percentage like "25%"
	if strings.HasSuffix(original, "%") {
		originalStr := strings.TrimSuffix(original, "%")
		if originalVal := ws.parseIntValue(originalStr); originalVal > 0 {
			newVal := originalVal + increaseBy
			if newVal > 200 { // Cap at 200%
				newVal = 200
			}
			return fmt.Sprintf("%d%%", newVal)
		}
	}
	return original
}

// generateScalingRationale generates rationale for scaling recommendations
func (ws *WorkflowSimulator) generateScalingRationale(template *WorkflowTemplate, report *ResourceImpactReport) string {
	rationale := "Scaling recommendation based on: "
	reasons := make([]string, 0)

	if template != nil {
		if len(template.Steps) > 10 {
			reasons = append(reasons, "high workflow complexity")
		}
	}

	if report != nil {
		if len(report.ResourceConflicts) > 2 {
			reasons = append(reasons, "resource conflicts detected")
		}
		if consumption, ok := report.ResourceConsumption["cpu_increase"].(float64); ok && consumption > 0.4 {
			reasons = append(reasons, "high CPU utilization")
		}
	}

	if len(reasons) == 0 {
		return "Standard scaling recommendation for workflow execution"
	}

	return rationale + strings.Join(reasons, ", ")
}

// parseStorageValue parses storage values like "5Gi" to GB
func (ws *WorkflowSimulator) parseStorageValue(storage string) float64 {
	if strings.HasSuffix(storage, "Gi") {
		valueStr := strings.TrimSuffix(storage, "Gi")
		if val := ws.parseIntValue(valueStr); val > 0 {
			return float64(val) // Gi to GB is approximately 1:1 for estimation
		}
	}
	return 0.0
}

// parseIntValue parses string to int, returns 0 if invalid
func (ws *WorkflowSimulator) parseIntValue(s string) int {
	// Simple integer parsing without importing strconv
	val := 0
	for _, char := range s {
		if char >= '0' && char <= '9' {
			val = val*10 + int(char-'0')
		} else {
			return 0 // Invalid character
		}
	}
	return val
}
