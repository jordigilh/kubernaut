package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
)

// ProcessResponse analyzes and enhances SLM responses with AI insights
func (p *DefaultAIResponseProcessor) ProcessResponse(ctx context.Context, rawResponse string, originalAlert types.Alert) (*EnhancedActionRecommendation, error) {
	startTime := time.Now()

	// First, parse the basic recommendation
	basicRecommendation, err := p.parseBasicRecommendation(rawResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse basic recommendation: %w", err)
	}

	// Create enhanced recommendation
	enhanced := &EnhancedActionRecommendation{
		ActionRecommendation: basicRecommendation,
		ProcessingMetadata: &ProcessingMetadata{
			ProcessingTime:      0, // Will be set at the end
			AIModelUsed:         "ai_response_processor",
			ProcessingSteps:     []string{"basic_parsing"},
			ConfidenceThreshold: p.config.ConfidenceThreshold,
		},
	}

	// Apply AI enhancements based on configuration
	if p.config.EnableAdvancedValidation {
		enhanced.ValidationResult, err = p.ValidateRecommendation(ctx, basicRecommendation, originalAlert)
		if err != nil {
			enhanced.ProcessingMetadata.ProcessingErrors = append(enhanced.ProcessingMetadata.ProcessingErrors,
				fmt.Sprintf("validation error: %v", err))
		} else {
			enhanced.ProcessingMetadata.ProcessingSteps = append(enhanced.ProcessingMetadata.ProcessingSteps, "validation")
			if enhanced.ValidationResult.IsValid {
				enhanced.ProcessingMetadata.ValidationsPassed++
			} else {
				enhanced.ProcessingMetadata.ValidationsFailed++
			}
		}
	}

	if p.config.EnableReasoningAnalysis && basicRecommendation.Reasoning != nil {
		enhanced.ReasoningAnalysis, err = p.AnalyzeReasoning(ctx, basicRecommendation.Reasoning, originalAlert)
		if err != nil {
			enhanced.ProcessingMetadata.ProcessingErrors = append(enhanced.ProcessingMetadata.ProcessingErrors,
				fmt.Sprintf("reasoning analysis error: %v", err))
		} else {
			enhanced.ProcessingMetadata.ProcessingSteps = append(enhanced.ProcessingMetadata.ProcessingSteps, "reasoning_analysis")
		}
	}

	if p.config.EnableConfidenceCalibration {
		enhanced.ConfidenceAssessment, err = p.AssessConfidence(ctx, basicRecommendation, originalAlert)
		if err != nil {
			enhanced.ProcessingMetadata.ProcessingErrors = append(enhanced.ProcessingMetadata.ProcessingErrors,
				fmt.Sprintf("confidence assessment error: %v", err))
		} else {
			enhanced.ProcessingMetadata.ProcessingSteps = append(enhanced.ProcessingMetadata.ProcessingSteps, "confidence_calibration")
		}
	}

	if p.config.EnableContextualEnhancement {
		enhanced.ContextualEnhancement, err = p.EnhanceContext(ctx, basicRecommendation, originalAlert)
		if err != nil {
			enhanced.ProcessingMetadata.ProcessingErrors = append(enhanced.ProcessingMetadata.ProcessingErrors,
				fmt.Sprintf("contextual enhancement error: %v", err))
		} else {
			enhanced.ProcessingMetadata.ProcessingSteps = append(enhanced.ProcessingMetadata.ProcessingSteps, "contextual_enhancement")
		}
	}

	// Update processing metadata
	enhanced.ProcessingMetadata.ProcessingTime = time.Since(startTime)
	enhanced.ProcessingMetadata.EnhancementsApplied = enhanced.ProcessingMetadata.ProcessingSteps[1:] // Skip basic_parsing

	return enhanced, nil
}

// ValidateRecommendation performs AI-powered validation of action recommendations
func (p *DefaultAIResponseProcessor) ValidateRecommendation(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*ValidationResult, error) {
	// Build validation prompt
	prompt := p.buildValidationPrompt(recommendation, alert)

	// Get AI analysis
	analysisResponse, err := p.aiClient.ChatCompletion(ctx, prompt)
	if err != nil {
		// Fall back to basic validation
		return p.performBasicValidation(recommendation, alert), nil
	}

	// Parse AI validation response
	validation, err := p.parseValidationResponse(analysisResponse)
	if err != nil {
		// Fall back to basic validation
		return p.performBasicValidation(recommendation, alert), nil
	}

	// Apply knowledge base rules
	p.applyValidationRules(validation, recommendation, alert)

	return validation, nil
}

// AnalyzeReasoning performs AI analysis of the reasoning quality and coherence
func (p *DefaultAIResponseProcessor) AnalyzeReasoning(ctx context.Context, reasoning *types.ReasoningDetails, alert types.Alert) (*ReasoningAnalysis, error) {
	// Build reasoning analysis prompt
	prompt := p.buildReasoningAnalysisPrompt(reasoning, alert)

	// Get AI analysis
	analysisResponse, err := p.aiClient.ChatCompletion(ctx, prompt)
	if err != nil {
		// Fall back to basic reasoning analysis
		return p.performBasicReasoningAnalysis(reasoning), nil
	}

	// Parse AI reasoning analysis response
	analysis, err := p.parseReasoningAnalysisResponse(analysisResponse)
	if err != nil {
		// Fall back to basic reasoning analysis
		return p.performBasicReasoningAnalysis(reasoning), nil
	}

	return analysis, nil
}

// AssessConfidence performs AI-powered confidence assessment and calibration
func (p *DefaultAIResponseProcessor) AssessConfidence(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*ConfidenceAssessment, error) {
	// Build confidence assessment prompt
	prompt := p.buildConfidenceAssessmentPrompt(recommendation, alert)

	// Get AI analysis
	analysisResponse, err := p.aiClient.ChatCompletion(ctx, prompt)
	if err != nil {
		// Fall back to basic confidence assessment
		return p.performBasicConfidenceAssessment(recommendation), nil
	}

	// Parse AI confidence assessment response
	assessment, err := p.parseConfidenceAssessmentResponse(analysisResponse)
	if err != nil {
		// Fall back to basic confidence assessment
		return p.performBasicConfidenceAssessment(recommendation), nil
	}

	return assessment, nil
}

// EnhanceContext adds AI-powered contextual analysis to recommendations
func (p *DefaultAIResponseProcessor) EnhanceContext(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*ContextualEnhancement, error) {
	// Build contextual enhancement prompt
	prompt := p.buildContextualEnhancementPrompt(recommendation, alert)

	// Get AI analysis
	analysisResponse, err := p.aiClient.ChatCompletion(ctx, prompt)
	if err != nil {
		// Fall back to basic contextual enhancement
		return p.performBasicContextualEnhancement(recommendation, alert), nil
	}

	// Parse AI contextual enhancement response
	enhancement, err := p.parseContextualEnhancementResponse(analysisResponse)
	if err != nil {
		// Fall back to basic contextual enhancement
		return p.performBasicContextualEnhancement(recommendation, alert), nil
	}

	// Enrich with knowledge base data
	if p.knowledgeBase != nil {
		p.enrichWithKnowledgeBase(enhancement, recommendation, alert)
	}

	return enhancement, nil
}

// Helper methods for parsing and fallback logic

func (p *DefaultAIResponseProcessor) parseBasicRecommendation(rawResponse string) (*types.ActionRecommendation, error) {
	// Extract JSON from response (similar to existing parseActionRecommendation logic)
	start := strings.Index(rawResponse, "{")
	if start == -1 {
		return nil, fmt.Errorf("no JSON found in response")
	}

	braceCount := 0
	end := -1
	for i, char := range rawResponse[start:] {
		if char == '{' {
			braceCount++
		} else if char == '}' {
			braceCount--
			if braceCount == 0 {
				end = start + i + 1
				break
			}
		}
	}

	if end == -1 {
		return nil, fmt.Errorf("malformed JSON in response")
	}

	jsonStr := rawResponse[start:end]

	var rawRecommendation map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &rawRecommendation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	recommendation := &types.ActionRecommendation{}

	// Extract action
	if action, ok := rawRecommendation["action"].(string); ok {
		recommendation.Action = action
	} else {
		return nil, fmt.Errorf("action field missing or invalid")
	}

	// Extract parameters
	if params, ok := rawRecommendation["parameters"].(map[string]interface{}); ok {
		recommendation.Parameters = params
	}

	// Extract confidence
	if confidence, ok := rawRecommendation["confidence"].(float64); ok {
		if confidence >= 0.0 && confidence <= 1.0 {
			recommendation.Confidence = confidence
		} else {
			return nil, fmt.Errorf("invalid confidence value: %f", confidence)
		}
	} else {
		return nil, fmt.Errorf("confidence field missing")
	}

	// Extract reasoning
	if reasoning, ok := rawRecommendation["reasoning"].(string); ok {
		recommendation.Reasoning = &types.ReasoningDetails{
			Summary: reasoning,
		}
	}

	return recommendation, nil
}

func (p *DefaultAIResponseProcessor) buildValidationPrompt(recommendation *types.ActionRecommendation, alert types.Alert) string {
	var prompt strings.Builder

	prompt.WriteString("Analyze and validate this action recommendation for a Kubernetes alert.\n\n")
	prompt.WriteString("ALERT CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Alert: %s\n", alert.Name))
	prompt.WriteString(fmt.Sprintf("- Severity: %s\n", alert.Severity))
	prompt.WriteString(fmt.Sprintf("- Description: %s\n", alert.Description))
	prompt.WriteString(fmt.Sprintf("- Namespace: %s\n", alert.Namespace))
	prompt.WriteString(fmt.Sprintf("- Resource: %s\n", alert.Resource))

	prompt.WriteString("\nRECOMMENDATION TO VALIDATE:\n")
	prompt.WriteString(fmt.Sprintf("- Action: %s\n", recommendation.Action))
	prompt.WriteString(fmt.Sprintf("- Confidence: %.3f\n", recommendation.Confidence))
	if recommendation.Reasoning != nil {
		prompt.WriteString(fmt.Sprintf("- Reasoning: %s\n", recommendation.Reasoning.Summary))
	}

	prompt.WriteString("\nVALIDATION ANALYSIS:\n")
	prompt.WriteString("1. Is the recommended action appropriate for this alert type?\n")
	prompt.WriteString("2. Are the required parameters complete and correct?\n")
	prompt.WriteString("3. What is the risk level and blast radius of this action?\n")
	prompt.WriteString("4. Are there any safety concerns or preconditions?\n")
	prompt.WriteString("5. What alternative actions should be considered?\n")

	prompt.WriteString("\nRESPONSE FORMAT:\n")
	prompt.WriteString("Provide a JSON object with validation results:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"is_valid\": boolean,\n")
	prompt.WriteString("  \"validation_score\": number_between_0_and_1,\n")
	prompt.WriteString("  \"action_appropriate\": boolean,\n")
	prompt.WriteString("  \"parameters_complete\": boolean,\n")
	prompt.WriteString("  \"risk_assessment\": {\n")
	prompt.WriteString("    \"risk_level\": \"low|medium|high|critical\",\n")
	prompt.WriteString("    \"blast_radius\": \"pod|deployment|namespace|cluster\",\n")
	prompt.WriteString("    \"reversibility_score\": number_between_0_and_1\n")
	prompt.WriteString("  },\n")
	prompt.WriteString("  \"violations\": [{\"type\": \"string\", \"severity\": \"warning|error|critical\", \"message\": \"string\"}],\n")
	prompt.WriteString("  \"recommendations\": [\"string\"],\n")
	prompt.WriteString("  \"alternative_actions\": [\"string\"]\n")
	prompt.WriteString("}\n")

	return prompt.String()
}

func (p *DefaultAIResponseProcessor) buildReasoningAnalysisPrompt(reasoning *types.ReasoningDetails, alert types.Alert) string {
	var prompt strings.Builder

	prompt.WriteString("Analyze the quality and coherence of this reasoning for a Kubernetes action recommendation.\n\n")
	prompt.WriteString("ALERT CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Alert: %s\n", alert.Name))
	prompt.WriteString(fmt.Sprintf("- Severity: %s\n", alert.Severity))
	prompt.WriteString(fmt.Sprintf("- Description: %s\n", alert.Description))

	prompt.WriteString("\nREASONING TO ANALYZE:\n")
	prompt.WriteString(fmt.Sprintf("Summary: %s\n", reasoning.Summary))
	if reasoning.HistoricalContext != "" {
		prompt.WriteString(fmt.Sprintf("Historical Context: %s\n", reasoning.HistoricalContext))
	}
	if reasoning.PrimaryReason != "" {
		prompt.WriteString(fmt.Sprintf("Primary Reason: %s\n", reasoning.PrimaryReason))
	}

	prompt.WriteString("\nANALYSIS CRITERIA:\n")
	prompt.WriteString("1. Quality: How well-structured and clear is the reasoning?\n")
	prompt.WriteString("2. Coherence: Do the different parts of the reasoning connect logically?\n")
	prompt.WriteString("3. Completeness: Does the reasoning address all relevant aspects?\n")
	prompt.WriteString("4. Evidence Support: Is the reasoning backed by evidence and facts?\n")
	prompt.WriteString("5. Bias Detection: Are there any obvious biases or logical fallacies?\n")
	prompt.WriteString("6. Reasoning Chain: Can you trace the logical steps clearly?\n")

	prompt.WriteString("\nRESPONSE FORMAT:\n")
	prompt.WriteString("Provide a JSON object with reasoning analysis:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"quality_score\": number_between_0_and_1,\n")
	prompt.WriteString("  \"coherence_score\": number_between_0_and_1,\n")
	prompt.WriteString("  \"completeness_score\": number_between_0_and_1,\n")
	prompt.WriteString("  \"logical_consistency\": boolean,\n")
	prompt.WriteString("  \"evidence_support\": number_between_0_and_1,\n")
	prompt.WriteString("  \"bias_detection\": [{\"type\": \"string\", \"confidence\": number, \"description\": \"string\"}],\n")
	prompt.WriteString("  \"reasoning_chain\": [{\"step\": number, \"description\": \"string\", \"confidence\": number}],\n")
	prompt.WriteString("  \"gaps\": [\"string\"],\n")
	prompt.WriteString("  \"strengths\": [\"string\"]\n")
	prompt.WriteString("}\n")

	return prompt.String()
}

func (p *DefaultAIResponseProcessor) buildConfidenceAssessmentPrompt(recommendation *types.ActionRecommendation, alert types.Alert) string {
	var prompt strings.Builder

	prompt.WriteString("Analyze and calibrate the confidence score for this action recommendation.\n\n")
	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Alert: %s (Severity: %s)\n", alert.Name, alert.Severity))
	prompt.WriteString(fmt.Sprintf("- Recommended Action: %s\n", recommendation.Action))
	prompt.WriteString(fmt.Sprintf("- Original Confidence: %.3f\n", recommendation.Confidence))
	if recommendation.Reasoning != nil {
		prompt.WriteString(fmt.Sprintf("- Reasoning: %s\n", recommendation.Reasoning.Summary))
	}

	prompt.WriteString("\nCONFIDENCE ASSESSMENT:\n")
	prompt.WriteString("1. How reliable is the original confidence score?\n")
	prompt.WriteString("2. What factors increase or decrease confidence?\n")
	prompt.WriteString("3. What uncertainties exist in this recommendation?\n")
	prompt.WriteString("4. What would be a calibrated confidence score?\n")
	prompt.WriteString("5. What confidence threshold should be used for this action?\n")

	prompt.WriteString("\nRESPONSE FORMAT:\n")
	prompt.WriteString("Provide a JSON object with confidence assessment:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"calibrated_confidence\": number_between_0_and_1,\n")
	prompt.WriteString("  \"confidence_reliability\": number_between_0_and_1,\n")
	prompt.WriteString("  \"uncertainty_factors\": [{\"factor\": \"string\", \"impact\": \"increases|decreases\", \"magnitude\": number}],\n")
	prompt.WriteString("  \"confidence_interval\": {\"lower\": number, \"upper\": number},\n")
	prompt.WriteString("  \"calibration_notes\": \"string\",\n")
	prompt.WriteString("  \"suggested_threshold\": number_between_0_and_1\n")
	prompt.WriteString("}\n")

	return prompt.String()
}

func (p *DefaultAIResponseProcessor) buildContextualEnhancementPrompt(recommendation *types.ActionRecommendation, alert types.Alert) string {
	var prompt strings.Builder

	prompt.WriteString("Provide contextual analysis and enhancement for this action recommendation.\n\n")
	prompt.WriteString("CONTEXT:\n")
	prompt.WriteString(fmt.Sprintf("- Alert: %s\n", alert.Name))
	prompt.WriteString(fmt.Sprintf("- Description: %s\n", alert.Description))
	prompt.WriteString(fmt.Sprintf("- Namespace: %s\n", alert.Namespace))
	prompt.WriteString(fmt.Sprintf("- Resource: %s\n", alert.Resource))
	prompt.WriteString(fmt.Sprintf("- Recommended Action: %s\n", recommendation.Action))
	prompt.WriteString(fmt.Sprintf("- Confidence: %.3f\n", recommendation.Confidence))

	prompt.WriteString("\nCONTEXTUAL ANALYSIS:\n")
	prompt.WriteString("1. What is the situational urgency and business impact?\n")
	prompt.WriteString("2. What potential cascading effects should be considered?\n")
	prompt.WriteString("3. What is the optimal timing for this action?\n")
	prompt.WriteString("4. What monitoring should be put in place?\n")
	prompt.WriteString("5. What are the expected timeline and milestones?\n")

	prompt.WriteString("\nRESPONSE FORMAT:\n")
	prompt.WriteString("Provide a JSON object with contextual enhancement:\n")
	prompt.WriteString("{\n")
	prompt.WriteString("  \"situational_context\": {\n")
	prompt.WriteString("    \"urgency\": \"low|medium|high|critical\",\n")
	prompt.WriteString("    \"business_impact\": \"string\",\n")
	prompt.WriteString("    \"maintenance_window\": boolean,\n")
	prompt.WriteString("    \"peak_traffic\": boolean\n")
	prompt.WriteString("  },\n")
	prompt.WriteString("  \"cascading_effects\": [{\"effect\": \"string\", \"probability\": number, \"impact\": \"string\"}],\n")
	prompt.WriteString("  \"timeline_analysis\": {\n")
	prompt.WriteString("    \"expected_duration\": \"string\",\n")
	prompt.WriteString("    \"optimal_timing\": \"string\"\n")
	prompt.WriteString("  },\n")
	prompt.WriteString("  \"suggested_monitoring\": [{\"metric\": \"string\", \"threshold\": \"string\", \"rationale\": \"string\"}]\n")
	prompt.WriteString("}\n")

	return prompt.String()
}
