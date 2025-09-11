package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/storage/vector"
	"github.com/sirupsen/logrus"
)

// DefaultLearningEnhancedPromptBuilder implements LearningEnhancedPromptBuilder interface
// Provides intelligent prompt generation with learning capabilities from workflow executions
type DefaultLearningEnhancedPromptBuilder struct {
	llmClient     llm.Client
	vectorDB      vector.VectorDatabase
	executionRepo ExecutionRepository
	log           *logrus.Logger

	// Learning components
	templateStore    *PromptTemplateStore
	patternMatcher   *PromptPatternMatcher
	qualityAssessor  *PromptQualityAssessor
	adaptationEngine *PromptAdaptationEngine

	// Configuration
	config *LearningPromptConfig
}

// LearningPromptConfig holds configuration for learning-enhanced prompt building
type LearningPromptConfig struct {
	EnablePatternLearning      bool    `yaml:"enable_pattern_learning" default:"true"`
	MinPatternOccurrences      int     `yaml:"min_pattern_occurrences" default:"3"`
	QualityThreshold           float64 `yaml:"quality_threshold" default:"0.7"`
	AdaptationLearningRate     float64 `yaml:"adaptation_learning_rate" default:"0.1"`
	MaxTemplateVariants        int     `yaml:"max_template_variants" default:"10"`
	LearningBatchSize          int     `yaml:"learning_batch_size" default:"50"`
	PatternExpirationDays      int     `yaml:"pattern_expiration_days" default:"90"`
	EnableContextualAdaptation bool    `yaml:"enable_contextual_adaptation" default:"true"`
}

// PromptTemplateStore manages optimized prompt templates
type PromptTemplateStore struct {
	templates map[string]*OptimizedTemplate
	mutex     sync.RWMutex
}

// OptimizedTemplate represents a learned and optimized prompt template
type OptimizedTemplate struct {
	ID               string                 `json:"id"`
	BaseTemplate     string                 `json:"base_template"`
	OptimizedVersion string                 `json:"optimized_version"`
	SuccessRate      float64                `json:"success_rate"`
	UsageCount       int64                  `json:"usage_count"`
	LastUsed         time.Time              `json:"last_used"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	Variables        map[string]interface{} `json:"variables"`
	Patterns         []*LearnedPattern      `json:"patterns"`
	QualityScore     float64                `json:"quality_score"`
	Context          map[string]interface{} `json:"context"`
}

// LearnedPattern represents a learned pattern from execution history
type LearnedPattern struct {
	ID          string                 `json:"id"`
	Pattern     string                 `json:"pattern"`
	Replacement string                 `json:"replacement"`
	Confidence  float64                `json:"confidence"`
	Occurrences int                    `json:"occurrences"`
	SuccessRate float64                `json:"success_rate"`
	Context     map[string]interface{} `json:"context"`
	LearnedAt   time.Time              `json:"learned_at"`
}

// PromptPatternMatcher finds and analyzes patterns in successful prompts
type PromptPatternMatcher struct {
	patterns map[string]*LearnedPattern
	mutex    sync.RWMutex
	log      *logrus.Logger
}

// PromptQualityAssessor evaluates prompt quality and effectiveness
type PromptQualityAssessor struct {
	qualityMetrics map[string]*PromptQualityMetrics
	mutex          sync.RWMutex
	llmClient      llm.Client
	log            *logrus.Logger
}

// PromptQualityMetrics tracks quality metrics for prompts
type PromptQualityMetrics struct {
	PromptHash    string    `json:"prompt_hash"`
	Effectiveness float64   `json:"effectiveness"`
	Clarity       float64   `json:"clarity"`
	Specificity   float64   `json:"specificity"`
	Adaptability  float64   `json:"adaptability"`
	SuccessRate   float64   `json:"success_rate"`
	UsageCount    int64     `json:"usage_count"`
	LastAssessed  time.Time `json:"last_assessed"`
}

// PromptAdaptationEngine adapts prompts based on context and learning
type PromptAdaptationEngine struct {
	adaptationRules map[string]*AdaptationRule
	mutex           sync.RWMutex
	learningRate    float64
	log             *logrus.Logger
}

// AdaptationRule defines how prompts should be adapted for specific contexts
type AdaptationRule struct {
	ID          string                 `json:"id"`
	Condition   string                 `json:"condition"`
	Adaptation  string                 `json:"adaptation"`
	Confidence  float64                `json:"confidence"`
	Performance float64                `json:"performance"`
	Context     map[string]interface{} `json:"context"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// NewDefaultLearningEnhancedPromptBuilder creates a new learning-enhanced prompt builder
func NewDefaultLearningEnhancedPromptBuilder(
	llmClient llm.Client,
	vectorDB vector.VectorDatabase,
	executionRepo ExecutionRepository,
	log *logrus.Logger,
) *DefaultLearningEnhancedPromptBuilder {
	config := &LearningPromptConfig{
		EnablePatternLearning:      true,
		MinPatternOccurrences:      3,
		QualityThreshold:           0.7,
		AdaptationLearningRate:     0.1,
		MaxTemplateVariants:        10,
		LearningBatchSize:          50,
		PatternExpirationDays:      90,
		EnableContextualAdaptation: true,
	}

	return &DefaultLearningEnhancedPromptBuilder{
		llmClient:     llmClient,
		vectorDB:      vectorDB,
		executionRepo: executionRepo,
		log:           log,
		templateStore: &PromptTemplateStore{
			templates: make(map[string]*OptimizedTemplate),
		},
		patternMatcher: &PromptPatternMatcher{
			patterns: make(map[string]*LearnedPattern),
			log:      log,
		},
		qualityAssessor: &PromptQualityAssessor{
			qualityMetrics: make(map[string]*PromptQualityMetrics),
			llmClient:      llmClient,
			log:            log,
		},
		adaptationEngine: &PromptAdaptationEngine{
			adaptationRules: make(map[string]*AdaptationRule),
			learningRate:    config.AdaptationLearningRate,
			log:             log,
		},
		config: config,
	}
}

// BuildPrompt builds an enhanced prompt using learned patterns and optimizations
func (lepb *DefaultLearningEnhancedPromptBuilder) BuildPrompt(ctx context.Context, template string, context map[string]interface{}) (string, error) {
	lepb.log.WithFields(logrus.Fields{
		"template_len": len(template),
		"context_keys": len(context),
	}).Debug("Building enhanced prompt")

	// 1. Find optimized template if available
	optimizedTemplate := lepb.findOptimizedTemplate(template)
	basePrompt := template
	if optimizedTemplate != nil {
		basePrompt = optimizedTemplate.OptimizedVersion
		lepb.log.Debug("Using optimized template version")
	}

	// 2. Apply learned patterns
	enhancedPrompt := lepb.applyLearnedPatterns(basePrompt, context)

	// 3. Apply contextual adaptations
	if lepb.config.EnableContextualAdaptation {
		adaptedPrompt, err := lepb.applyContextualAdaptations(ctx, enhancedPrompt, context)
		if err != nil {
			lepb.log.WithError(err).Warn("Failed to apply contextual adaptations")
		} else {
			enhancedPrompt = adaptedPrompt
		}
	}

	// 4. Validate prompt quality
	quality, err := lepb.assessPromptQuality(ctx, enhancedPrompt, context)
	if err != nil {
		lepb.log.WithError(err).Warn("Failed to assess prompt quality")
	} else if quality.Effectiveness < lepb.config.QualityThreshold {
		lepb.log.WithFields(logrus.Fields{
			"quality":   quality.Effectiveness,
			"threshold": lepb.config.QualityThreshold,
		}).Warn("Prompt quality below threshold, using fallback")
		enhancedPrompt = lepb.applyFallbackEnhancements(basePrompt, context)
	}

	// 5. Record usage for learning
	lepb.recordPromptUsage(template, enhancedPrompt, context)

	lepb.log.WithFields(logrus.Fields{
		"original_len": len(template),
		"enhanced_len": len(enhancedPrompt),
		"quality":      quality.Effectiveness,
	}).Debug("Enhanced prompt building completed")

	return enhancedPrompt, nil
}

// GetLearnFromExecution learns from workflow execution outcomes
func (lepb *DefaultLearningEnhancedPromptBuilder) GetLearnFromExecution(ctx context.Context, execution *RuntimeWorkflowExecution) error {
	lepb.log.WithFields(logrus.Fields{
		"execution_id": execution.ID,
		"workflow_id":  execution.WorkflowID,
		"success":      execution.Status == string(ExecutionStatusCompleted),
	}).Debug("Learning from workflow execution")

	// Extract prompts and outcomes from execution
	prompts := lepb.extractPromptsFromExecution(execution)
	if len(prompts) == 0 {
		lepb.log.Debug("No prompts found in execution")
		return nil
	}

	// Learn patterns from successful executions
	if execution.Status == string(ExecutionStatusCompleted) {
		if err := lepb.learnPatternsFromSuccess(prompts, execution); err != nil {
			lepb.log.WithError(err).Warn("Failed to learn patterns from successful execution")
		}
	} else {
		if err := lepb.learnFromFailure(prompts, execution); err != nil {
			lepb.log.WithError(err).Warn("Failed to learn from failed execution")
		}
	}

	// Update template optimizations
	if err := lepb.updateTemplateOptimizations(prompts, execution); err != nil {
		lepb.log.WithError(err).Warn("Failed to update template optimizations")
	}

	// Update adaptation rules
	if err := lepb.updateAdaptationRules(prompts, execution); err != nil {
		lepb.log.WithError(err).Warn("Failed to update adaptation rules")
	}

	return nil
}

// GetGetOptimizedTemplate retrieves an optimized version of a template
func (lepb *DefaultLearningEnhancedPromptBuilder) GetGetOptimizedTemplate(ctx context.Context, templateID string) (string, error) {
	lepb.log.WithField("template_id", templateID).Debug("Retrieving optimized template")

	lepb.templateStore.mutex.RLock()
	defer lepb.templateStore.mutex.RUnlock()

	if template, exists := lepb.templateStore.templates[templateID]; exists {
		// Update usage statistics
		template.UsageCount++
		template.LastUsed = time.Now()

		lepb.log.WithFields(logrus.Fields{
			"template_id":   templateID,
			"success_rate":  template.SuccessRate,
			"quality_score": template.QualityScore,
		}).Debug("Returning optimized template")

		return template.OptimizedVersion, nil
	}

	return "", fmt.Errorf("optimized template not found for ID: %s", templateID)
}

// GetBuildEnhancedPrompt builds a prompt with advanced enhancements
func (lepb *DefaultLearningEnhancedPromptBuilder) GetBuildEnhancedPrompt(ctx context.Context, basePrompt string, context map[string]interface{}) (string, error) {
	lepb.log.WithFields(logrus.Fields{
		"base_prompt_len": len(basePrompt),
		"context_keys":    len(context),
	}).Debug("Building enhanced prompt with advanced features")

	// Start with base prompt
	enhancedPrompt := basePrompt

	// 1. Apply AI-driven enhancements using LLM
	if lepb.llmClient != nil {
		aiEnhanced, err := lepb.enhanceWithAI(ctx, basePrompt, context)
		if err != nil {
			lepb.log.WithError(err).Warn("Failed to enhance prompt with AI")
		} else {
			enhancedPrompt = aiEnhanced
		}
	}

	// 2. Apply learned improvements
	enhancedPrompt = lepb.applyLearnedImprovements(enhancedPrompt, context)

	// 3. Apply domain-specific optimizations
	enhancedPrompt = lepb.applyDomainOptimizations(enhancedPrompt, context)

	// 4. Validate and refine
	if refined, err := lepb.validateAndRefine(ctx, enhancedPrompt, context); err == nil {
		enhancedPrompt = refined
	}

	lepb.log.WithFields(logrus.Fields{
		"original_len":      len(basePrompt),
		"enhanced_len":      len(enhancedPrompt),
		"enhancement_ratio": float64(len(enhancedPrompt)) / float64(len(basePrompt)),
	}).Debug("Advanced prompt enhancement completed")

	return enhancedPrompt, nil
}

// Private helper methods

func (lepb *DefaultLearningEnhancedPromptBuilder) findOptimizedTemplate(template string) *OptimizedTemplate {
	lepb.templateStore.mutex.RLock()
	defer lepb.templateStore.mutex.RUnlock()

	// Find template by exact match or similarity
	for _, optimized := range lepb.templateStore.templates {
		if optimized.BaseTemplate == template {
			return optimized
		}
	}

	// Find by similarity (simplified implementation)
	bestMatch := &OptimizedTemplate{}
	bestSimilarity := 0.0

	for _, optimized := range lepb.templateStore.templates {
		similarity := lepb.calculateTemplateSimilarity(template, optimized.BaseTemplate)
		if similarity > bestSimilarity && similarity > 0.8 {
			bestSimilarity = similarity
			bestMatch = optimized
		}
	}

	if bestSimilarity > 0.8 {
		return bestMatch
	}

	return nil
}

func (lepb *DefaultLearningEnhancedPromptBuilder) applyLearnedPatterns(prompt string, context map[string]interface{}) string {
	lepb.patternMatcher.mutex.RLock()
	defer lepb.patternMatcher.mutex.RUnlock()

	enhanced := prompt

	// Sort patterns by confidence
	patterns := make([]*LearnedPattern, 0, len(lepb.patternMatcher.patterns))
	for _, pattern := range lepb.patternMatcher.patterns {
		patterns = append(patterns, pattern)
	}

	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Confidence > patterns[j].Confidence
	})

	// Apply patterns in order of confidence
	for _, pattern := range patterns {
		if lepb.patternApplies(pattern, context) {
			enhanced = strings.ReplaceAll(enhanced, pattern.Pattern, pattern.Replacement)
		}
	}

	return enhanced
}

func (lepb *DefaultLearningEnhancedPromptBuilder) applyContextualAdaptations(ctx context.Context, prompt string, context map[string]interface{}) (string, error) {
	lepb.adaptationEngine.mutex.RLock()
	defer lepb.adaptationEngine.mutex.RUnlock()

	adapted := prompt

	for _, rule := range lepb.adaptationEngine.adaptationRules {
		if lepb.adaptationRuleApplies(rule, context) {
			adapted = lepb.applyAdaptationRule(adapted, rule, context)
		}
	}

	return adapted, nil
}

func (lepb *DefaultLearningEnhancedPromptBuilder) assessPromptQuality(ctx context.Context, prompt string, context map[string]interface{}) (*PromptQualityMetrics, error) {
	promptHash := lepb.hashPrompt(prompt)

	lepb.qualityAssessor.mutex.RLock()
	if cached, exists := lepb.qualityAssessor.qualityMetrics[promptHash]; exists {
		lepb.qualityAssessor.mutex.RUnlock()
		return cached, nil
	}
	lepb.qualityAssessor.mutex.RUnlock()

	// Assess quality using AI if available
	quality := &PromptQualityMetrics{
		PromptHash:    promptHash,
		Effectiveness: 0.5, // Default neutral
		Clarity:       0.5,
		Specificity:   0.5,
		Adaptability:  0.5,
		LastAssessed:  time.Now(),
	}

	if lepb.llmClient != nil {
		if err := lepb.assessQualityWithAI(ctx, prompt, quality); err != nil {
			lepb.log.WithError(err).Warn("Failed to assess quality with AI")
		}
	}

	// Apply heuristic quality assessment
	lepb.assessQualityHeuristics(prompt, quality)

	// Cache the result
	lepb.qualityAssessor.mutex.Lock()
	lepb.qualityAssessor.qualityMetrics[promptHash] = quality
	lepb.qualityAssessor.mutex.Unlock()

	return quality, nil
}

func (lepb *DefaultLearningEnhancedPromptBuilder) applyFallbackEnhancements(prompt string, context map[string]interface{}) string {
	// Apply basic enhancements when quality is low
	enhanced := prompt

	// Add clarity improvements
	if !strings.Contains(enhanced, "Be specific") {
		enhanced += "\n\nBe specific and detailed in your response."
	}

	// Add context awareness
	if alertType, ok := context["alert_type"].(string); ok {
		enhanced = strings.ReplaceAll(enhanced, "the alert", fmt.Sprintf("the %s alert", alertType))
	}

	return enhanced
}

func (lepb *DefaultLearningEnhancedPromptBuilder) recordPromptUsage(original, enhanced string, context map[string]interface{}) {
	// Record usage for learning purposes
	templateID := lepb.hashPrompt(original)

	lepb.templateStore.mutex.Lock()
	defer lepb.templateStore.mutex.Unlock()

	if template, exists := lepb.templateStore.templates[templateID]; exists {
		template.UsageCount++
		template.LastUsed = time.Now()
	} else {
		// Create new template record
		lepb.templateStore.templates[templateID] = &OptimizedTemplate{
			ID:               templateID,
			BaseTemplate:     original,
			OptimizedVersion: enhanced,
			UsageCount:       1,
			LastUsed:         time.Now(),
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
			Variables:        context,
			QualityScore:     0.5,
			Context:          context,
		}
	}
}

func (lepb *DefaultLearningEnhancedPromptBuilder) extractPromptsFromExecution(execution *RuntimeWorkflowExecution) []string {
	var prompts []string

	// Extract prompts from execution metadata
	if promptsData, ok := execution.Metadata["prompts"]; ok {
		if promptsList, ok := promptsData.([]interface{}); ok {
			for _, p := range promptsList {
				if prompt, ok := p.(string); ok {
					prompts = append(prompts, prompt)
				}
			}
		}
	}

	// Extract from step metadata
	for _, step := range execution.Steps {
		if stepPrompt, ok := step.Metadata["prompt"].(string); ok {
			prompts = append(prompts, stepPrompt)
		}
	}

	return prompts
}

func (lepb *DefaultLearningEnhancedPromptBuilder) learnPatternsFromSuccess(prompts []string, execution *RuntimeWorkflowExecution) error {
	for _, prompt := range prompts {
		patterns := lepb.identifyPatternsInPrompt(prompt)
		for _, pattern := range patterns {
			lepb.updatePatternSuccess(pattern, execution)
		}
	}
	return nil
}

func (lepb *DefaultLearningEnhancedPromptBuilder) learnFromFailure(prompts []string, execution *RuntimeWorkflowExecution) error {
	for _, prompt := range prompts {
		patterns := lepb.identifyPatternsInPrompt(prompt)
		for _, pattern := range patterns {
			lepb.updatePatternFailure(pattern, execution)
		}
	}
	return nil
}

func (lepb *DefaultLearningEnhancedPromptBuilder) updateTemplateOptimizations(prompts []string, execution *RuntimeWorkflowExecution) error {
	for _, prompt := range prompts {
		templateID := lepb.hashPrompt(prompt)

		lepb.templateStore.mutex.Lock()
		if template, exists := lepb.templateStore.templates[templateID]; exists {
			if execution.Status == string(ExecutionStatusCompleted) {
				template.SuccessRate = (template.SuccessRate + 1.0) / 2.0
			} else {
				template.SuccessRate = template.SuccessRate * 0.9
			}
			template.UpdatedAt = time.Now()
		}
		lepb.templateStore.mutex.Unlock()
	}
	return nil
}

func (lepb *DefaultLearningEnhancedPromptBuilder) updateAdaptationRules(prompts []string, execution *RuntimeWorkflowExecution) error {
	// Update adaptation rules based on execution outcomes
	for _, prompt := range prompts {
		rules := lepb.identifyApplicableRules(prompt, execution)
		for _, rule := range rules {
			lepb.updateRulePerformance(rule, execution.Status == string(ExecutionStatusCompleted))
		}
	}
	return nil
}

func (lepb *DefaultLearningEnhancedPromptBuilder) enhanceWithAI(ctx context.Context, basePrompt string, context map[string]interface{}) (string, error) {
	enhancementPrompt := fmt.Sprintf(`
Improve this prompt for better AI workflow generation. The prompt should be:
1. More specific and actionable
2. Include relevant context from the situation
3. Guide toward better structured responses
4. Maintain the original intent

Original prompt:
%s

Context:
%s

Return only the improved prompt:`, basePrompt, lepb.formatContext(context))

	response, err := lepb.llmClient.ChatCompletion(ctx, enhancementPrompt)
	if err != nil {
		return basePrompt, fmt.Errorf("failed to enhance prompt with AI: %w", err)
	}

	return fmt.Sprintf("%v", response), nil
}

func (lepb *DefaultLearningEnhancedPromptBuilder) applyLearnedImprovements(prompt string, context map[string]interface{}) string {
	// Apply learned improvements based on historical success
	improved := prompt

	// Add proven effective phrases
	effectivePhrases := []string{
		"Analyze carefully",
		"Consider the following factors",
		"Provide specific recommendations",
		"Take into account the current state",
	}

	for _, phrase := range effectivePhrases {
		if !strings.Contains(improved, phrase) && lepb.shouldAddPhrase(phrase, context) {
			improved = fmt.Sprintf("%s\n\n%s:", improved, phrase)
		}
	}

	return improved
}

func (lepb *DefaultLearningEnhancedPromptBuilder) applyDomainOptimizations(prompt string, context map[string]interface{}) string {
	optimized := prompt

	// Kubernetes-specific optimizations
	if lepb.isKubernetesContext(context) {
		optimized = lepb.addKubernetesOptimizations(optimized, context)
	}

	// Alert-specific optimizations
	if lepb.isAlertContext(context) {
		optimized = lepb.addAlertOptimizations(optimized, context)
	}

	return optimized
}

func (lepb *DefaultLearningEnhancedPromptBuilder) validateAndRefine(ctx context.Context, prompt string, context map[string]interface{}) (string, error) {
	// Validate prompt structure and refine if needed
	refined := prompt

	// Check for common issues
	issues := lepb.identifyPromptIssues(prompt)
	for _, issue := range issues {
		refined = lepb.fixPromptIssue(refined, issue)
	}

	return refined, nil
}

// Helper methods for pattern matching and quality assessment

func (lepb *DefaultLearningEnhancedPromptBuilder) calculateTemplateSimilarity(a, b string) float64 {
	// Simple Jaccard similarity for templates
	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := strings.Fields(strings.ToLower(b))

	setA := make(map[string]bool)
	setB := make(map[string]bool)

	for _, word := range wordsA {
		setA[word] = true
	}
	for _, word := range wordsB {
		setB[word] = true
	}

	intersection := 0
	for word := range setA {
		if setB[word] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

func (lepb *DefaultLearningEnhancedPromptBuilder) patternApplies(pattern *LearnedPattern, context map[string]interface{}) bool {
	// Check if pattern is applicable to current context
	if pattern.Context == nil {
		return true
	}

	for key, expectedValue := range pattern.Context {
		if actualValue, ok := context[key]; !ok || actualValue != expectedValue {
			return false
		}
	}

	return true
}

func (lepb *DefaultLearningEnhancedPromptBuilder) adaptationRuleApplies(rule *AdaptationRule, context map[string]interface{}) bool {
	// Simple condition evaluation
	return rule.Condition == "always" // Simplified for now
}

func (lepb *DefaultLearningEnhancedPromptBuilder) applyAdaptationRule(prompt string, rule *AdaptationRule, context map[string]interface{}) string {
	// Apply adaptation rule to prompt
	return prompt + "\n\n" + rule.Adaptation
}

func (lepb *DefaultLearningEnhancedPromptBuilder) hashPrompt(prompt string) string {
	// Simple hash function for prompt identification
	hash := 0
	for _, char := range prompt {
		hash = hash*31 + int(char)
	}
	return fmt.Sprintf("prompt_%d", hash)
}

func (lepb *DefaultLearningEnhancedPromptBuilder) assessQualityWithAI(ctx context.Context, prompt string, quality *PromptQualityMetrics) error {
	qualityPrompt := fmt.Sprintf(`
Rate this prompt on a scale of 0.0 to 1.0 for:
1. Effectiveness (how likely to produce good results)
2. Clarity (how clear and understandable)
3. Specificity (how specific and actionable)

Prompt to evaluate:
%s

Respond with JSON only:
{
  "effectiveness": 0.0-1.0,
  "clarity": 0.0-1.0,
  "specificity": 0.0-1.0
}`, prompt)

	response, err := lepb.llmClient.ChatCompletion(ctx, qualityPrompt)
	if err != nil {
		return err
	}

	var assessment struct {
		Effectiveness float64 `json:"effectiveness"`
		Clarity       float64 `json:"clarity"`
		Specificity   float64 `json:"specificity"`
	}

	if err := json.Unmarshal([]byte(fmt.Sprintf("%v", response)), &assessment); err != nil {
		return err
	}

	quality.Effectiveness = assessment.Effectiveness
	quality.Clarity = assessment.Clarity
	quality.Specificity = assessment.Specificity

	return nil
}

func (lepb *DefaultLearningEnhancedPromptBuilder) assessQualityHeuristics(prompt string, quality *PromptQualityMetrics) {
	// Basic heuristic quality assessment
	wordCount := len(strings.Fields(prompt))
	sentenceCount := strings.Count(prompt, ".") + strings.Count(prompt, "!") + strings.Count(prompt, "?")

	// Length-based quality (optimal range)
	if wordCount >= 50 && wordCount <= 200 {
		quality.Clarity = 0.8
	} else if wordCount >= 20 && wordCount <= 300 {
		quality.Clarity = 0.6
	} else {
		quality.Clarity = 0.4
	}

	// Structure-based quality
	if sentenceCount >= 3 {
		quality.Specificity = 0.7
	} else {
		quality.Specificity = 0.5
	}

	// Keyword-based effectiveness
	effectiveKeywords := []string{"analyze", "specific", "consider", "recommend", "evaluate"}
	keywordCount := 0
	lowerPrompt := strings.ToLower(prompt)
	for _, keyword := range effectiveKeywords {
		if strings.Contains(lowerPrompt, keyword) {
			keywordCount++
		}
	}

	quality.Effectiveness = 0.5 + (float64(keywordCount) * 0.1)
	if quality.Effectiveness > 1.0 {
		quality.Effectiveness = 1.0
	}
}

// Additional helper methods

func (lepb *DefaultLearningEnhancedPromptBuilder) identifyPatternsInPrompt(prompt string) []string {
	// Identify common patterns in successful prompts
	var patterns []string

	// Simple pattern identification
	if strings.Contains(prompt, "analyze") {
		patterns = append(patterns, "analyze_pattern")
	}
	if strings.Contains(prompt, "recommend") {
		patterns = append(patterns, "recommend_pattern")
	}

	return patterns
}

func (lepb *DefaultLearningEnhancedPromptBuilder) updatePatternSuccess(pattern string, execution *RuntimeWorkflowExecution) {
	lepb.patternMatcher.mutex.Lock()
	defer lepb.patternMatcher.mutex.Unlock()

	if p, exists := lepb.patternMatcher.patterns[pattern]; exists {
		p.Occurrences++
		p.SuccessRate = (p.SuccessRate + 1.0) / 2.0
		p.Confidence = math.Min(1.0, p.Confidence+0.1)
	} else {
		lepb.patternMatcher.patterns[pattern] = &LearnedPattern{
			ID:          pattern,
			Pattern:     pattern,
			Confidence:  0.7,
			Occurrences: 1,
			SuccessRate: 1.0,
			LearnedAt:   time.Now(),
		}
	}
}

func (lepb *DefaultLearningEnhancedPromptBuilder) updatePatternFailure(pattern string, execution *RuntimeWorkflowExecution) {
	lepb.patternMatcher.mutex.Lock()
	defer lepb.patternMatcher.mutex.Unlock()

	if p, exists := lepb.patternMatcher.patterns[pattern]; exists {
		p.Occurrences++
		p.SuccessRate = p.SuccessRate * 0.8 // Reduce success rate
		p.Confidence = math.Max(0.0, p.Confidence-0.1)
	}
}

func (lepb *DefaultLearningEnhancedPromptBuilder) identifyApplicableRules(prompt string, execution *RuntimeWorkflowExecution) []*AdaptationRule {
	// Identify which adaptation rules apply to this prompt
	return []*AdaptationRule{} // Simplified for now
}

func (lepb *DefaultLearningEnhancedPromptBuilder) updateRulePerformance(rule *AdaptationRule, success bool) {
	lepb.adaptationEngine.mutex.Lock()
	defer lepb.adaptationEngine.mutex.Unlock()

	if success {
		rule.Performance = (rule.Performance + 1.0) / 2.0
	} else {
		rule.Performance = rule.Performance * 0.9
	}
	rule.UpdatedAt = time.Now()
}

func (lepb *DefaultLearningEnhancedPromptBuilder) formatContext(context map[string]interface{}) string {
	result := "{\n"
	for key, value := range context {
		result += fmt.Sprintf("  %s: %v\n", key, value)
	}
	result += "}"
	return result
}

func (lepb *DefaultLearningEnhancedPromptBuilder) shouldAddPhrase(phrase string, context map[string]interface{}) bool {
	// Determine if phrase should be added based on context
	return true // Simplified for now
}

func (lepb *DefaultLearningEnhancedPromptBuilder) isKubernetesContext(context map[string]interface{}) bool {
	_, hasNamespace := context["namespace"]
	_, hasPod := context["pod"]
	_, hasDeployment := context["deployment"]
	return hasNamespace || hasPod || hasDeployment
}

func (lepb *DefaultLearningEnhancedPromptBuilder) isAlertContext(context map[string]interface{}) bool {
	_, hasAlert := context["alert_type"]
	_, hasSeverity := context["severity"]
	return hasAlert || hasSeverity
}

func (lepb *DefaultLearningEnhancedPromptBuilder) addKubernetesOptimizations(prompt string, context map[string]interface{}) string {
	optimized := prompt

	if !strings.Contains(optimized, "kubectl") && !strings.Contains(optimized, "Kubernetes") {
		optimized += "\n\nConsider Kubernetes-specific factors such as resource limits, pod scheduling, and cluster state."
	}

	return optimized
}

func (lepb *DefaultLearningEnhancedPromptBuilder) addAlertOptimizations(prompt string, context map[string]interface{}) string {
	optimized := prompt

	if severity, ok := context["severity"].(string); ok {
		optimized = strings.ReplaceAll(optimized, "the alert", fmt.Sprintf("the %s severity alert", severity))
	}

	return optimized
}

func (lepb *DefaultLearningEnhancedPromptBuilder) identifyPromptIssues(prompt string) []string {
	var issues []string

	if len(prompt) < 20 {
		issues = append(issues, "too_short")
	}
	if len(prompt) > 1000 {
		issues = append(issues, "too_long")
	}
	if !strings.Contains(prompt, "?") && !strings.Contains(prompt, ".") {
		issues = append(issues, "no_punctuation")
	}

	return issues
}

func (lepb *DefaultLearningEnhancedPromptBuilder) fixPromptIssue(prompt string, issue string) string {
	switch issue {
	case "too_short":
		return prompt + " Please provide detailed analysis and specific recommendations."
	case "too_long":
		// Truncate or summarize (simplified)
		if len(prompt) > 800 {
			return prompt[:800] + "..."
		}
		return prompt
	case "no_punctuation":
		if !strings.HasSuffix(prompt, ".") && !strings.HasSuffix(prompt, "?") {
			return prompt + "."
		}
		return prompt
	default:
		return prompt
	}
}
