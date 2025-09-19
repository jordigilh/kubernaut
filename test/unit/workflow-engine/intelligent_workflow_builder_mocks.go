package workflowengine

import (
	"context"
	"fmt"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

// MockIntelligentWorkflowBuilder implements workflow builder for testing
type MockIntelligentWorkflowBuilder struct {
	generatedWorkflow *engine.Workflow
	generatedTemplate *engine.ExecutableTemplate
	buildError        error
}

// NewMockIntelligentWorkflowBuilder creates a new mock workflow builder
func NewMockIntelligentWorkflowBuilder() *MockIntelligentWorkflowBuilder {
	return &MockIntelligentWorkflowBuilder{}
}

// SetGeneratedWorkflow sets the workflow to return from generation calls
func (m *MockIntelligentWorkflowBuilder) SetGeneratedWorkflow(workflow *engine.Workflow) {
	m.generatedWorkflow = workflow
}

// SetGeneratedTemplate sets the template to return from generation calls
func (m *MockIntelligentWorkflowBuilder) SetGeneratedTemplate(template *engine.ExecutableTemplate) {
	m.generatedTemplate = template
}

// SetBuildError sets the error to return from generation calls
func (m *MockIntelligentWorkflowBuilder) SetBuildError(err error) {
	m.buildError = err
}

// GenerateWorkflow implements intelligent workflow generation following IntelligentWorkflowBuilder interface
func (m *MockIntelligentWorkflowBuilder) GenerateWorkflow(ctx context.Context, objective *engine.WorkflowObjective) (*engine.ExecutableTemplate, error) {
	// Check for context cancellation in test mock
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if m.buildError != nil {
		return nil, m.buildError
	}

	// If no specific template set, create a default one based on objective
	if m.generatedWorkflow == nil {
		// Create a basic template using constructor function
		template := engine.NewWorkflowTemplate("mock-template-"+objective.ID, "Mock Generated Template")
		template.Description = "Default AI-generated template for " + objective.Description

		// Add mock steps using proper ExecutableWorkflowStep
		assessStep := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:          "assess-step",
				Name:        "assess_situation",
				Description: "Assess the current situation",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Type:      engine.StepTypeCondition,
			Timeout:   5 * time.Minute,
			Variables: make(map[string]interface{}),
			Metadata:  make(map[string]interface{}),
		}

		remediationStep := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:          "remediation-step",
				Name:        "apply_remediation",
				Description: "Apply remediation actions",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Type:      engine.StepTypeAction,
			Timeout:   10 * time.Minute,
			Variables: make(map[string]interface{}),
			Metadata:  make(map[string]interface{}),
		}

		template.Steps = []*engine.ExecutableWorkflowStep{assessStep, remediationStep}
		template.Metadata["generation_confidence"] = 0.8
		template.Metadata["generated_by"] = "mock_builder"

		return template, nil
	}

	// Return the template from the generated workflow (need to extract template)
	if m.generatedWorkflow.Template != nil {
		return m.generatedWorkflow.Template, nil
	}

	// Fallback to basic template
	return engine.NewWorkflowTemplate("fallback-template", "Fallback Template"), nil
}

// MockTemplateManager implements template management for testing
type MockTemplateManager struct {
	templateLibrary map[string]*engine.ExecutableTemplate
	baseTemplate    *engine.ExecutableTemplate
	templateError   error
}

// NewMockTemplateManager creates a new mock template manager
func NewMockTemplateManager() *MockTemplateManager {
	return &MockTemplateManager{
		templateLibrary: make(map[string]*engine.ExecutableTemplate),
	}
}

// SetTemplateLibrary sets the template library
func (m *MockTemplateManager) SetTemplateLibrary(library map[string]*engine.ExecutableTemplate) {
	m.templateLibrary = library
}

// SetBaseTemplate sets the base template for customization
func (m *MockTemplateManager) SetBaseTemplate(template *engine.ExecutableTemplate) {
	m.baseTemplate = template
}

// SetTemplateError sets the error to return from template operations
func (m *MockTemplateManager) SetTemplateError(err error) {
	m.templateError = err
}

// GetTemplateByScenario retrieves template by scenario type
func (m *MockTemplateManager) GetTemplateByScenario(scenario string) (*engine.ExecutableTemplate, error) {
	if m.templateError != nil {
		return nil, m.templateError
	}

	template, exists := m.templateLibrary[scenario]
	if !exists {
		return nil, ErrTemplateNotFound
	}

	return template, nil
}

// CustomizeTemplate customizes a template with parameters
func (m *MockTemplateManager) CustomizeTemplate(templateID string, params map[string]interface{}) (*engine.ExecutableTemplate, error) {
	if m.templateError != nil {
		return nil, m.templateError
	}

	if m.baseTemplate == nil {
		return nil, ErrTemplateNotFound
	}

	// Create customized copy using constructor
	customized := engine.NewWorkflowTemplate(m.baseTemplate.ID+"-customized", m.baseTemplate.Name+" (Customized)")
	customized.Description = m.baseTemplate.Description
	customized.Variables = params

	// Copy and customize steps
	for _, step := range m.baseTemplate.Steps {
		// Create a new step based on the original
		newStep := &engine.ExecutableWorkflowStep{
			BaseEntity: types.BaseEntity{
				ID:          step.ID + "-customized",
				Name:        step.Name,
				Description: step.Description,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				Metadata:    make(map[string]interface{}),
			},
			Type:         step.Type,
			Action:       step.Action,
			Condition:    step.Condition,
			Dependencies: step.Dependencies,
			Timeout:      step.Timeout,
			RetryPolicy:  step.RetryPolicy,
			OnSuccess:    step.OnSuccess,
			OnFailure:    step.OnFailure,
			Variables:    make(map[string]interface{}),
			Metadata:     make(map[string]interface{}),
		}

		// Apply parameter customization
		for key, value := range params {
			newStep.Variables[key] = value
		}

		customized.Steps = append(customized.Steps, newStep)
	}

	return customized, nil
}

// MockValidationEngine implements workflow validation for testing
type MockValidationEngine struct {
	validationResult *engine.WorkflowValidationResult
	validationError  error
}

// NewMockValidationEngine creates a new mock validation engine
func NewMockValidationEngine() *MockValidationEngine {
	return &MockValidationEngine{}
}

// SetValidationResult sets the result to return from validation calls
func (m *MockValidationEngine) SetValidationResult(result *engine.WorkflowValidationResult) {
	m.validationResult = result
}

// SetValidationError sets the error to return from validation calls
func (m *MockValidationEngine) SetValidationError(err error) {
	m.validationError = err
}

// ValidateWorkflow validates a workflow for correctness and safety
func (m *MockValidationEngine) ValidateWorkflow(workflow *engine.Workflow) (*engine.WorkflowValidationResult, error) {
	if m.validationError != nil {
		return nil, m.validationError
	}

	if m.validationResult != nil {
		return m.validationResult, nil
	}

	// Default validation result
	return &engine.WorkflowValidationResult{
		Valid:            true,
		SafetyScore:      0.9,
		CorrectnessScore: 0.9,
		SecurityScore:    0.9,
		OverallScore:     0.9,
		ValidationChecks: map[string]interface{}{
			"basic_structure": true,
			"step_validation": true,
		},
		Warnings: []string{},
	}, nil
}

// MockLearningIntegrator implements learning integration for testing
type MockLearningIntegrator struct {
	learningResult *engine.LearningResult
	learningError  error
}

// NewMockLearningIntegrator creates a new mock learning integrator
func NewMockLearningIntegrator() *MockLearningIntegrator {
	return &MockLearningIntegrator{}
}

// SetLearningResult sets the result to return from learning calls
func (m *MockLearningIntegrator) SetLearningResult(result *engine.LearningResult) {
	m.learningResult = result
}

// SetLearningError sets the error to return from learning calls
func (m *MockLearningIntegrator) SetLearningError(err error) {
	m.learningError = err
}

// LearnFromExecutions learns from workflow execution outcomes
func (m *MockLearningIntegrator) LearnFromExecutions(executions []engine.ExecutionOutcome) (*engine.LearningResult, error) {
	if m.learningError != nil {
		return nil, m.learningError
	}

	if m.learningResult != nil {
		return m.learningResult, nil
	}

	// Default learning result - using new LearningResult structure
	return &engine.LearningResult{
		PatternConfidence: 0.85,
		LearningImpact:    "medium",
		UpdatedRules:      []string{"default_algorithm_rule"},
	}, nil
}

// Additional error definitions needed for the tests
var ErrTemplateNotFound = fmt.Errorf("template not found")

// Mock-specific types for testing (avoiding duplicates with engine package)
// Note: Using engine types directly for ExecutionOutcome and LearningResult
