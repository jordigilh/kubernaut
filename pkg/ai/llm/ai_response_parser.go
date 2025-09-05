package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
)

// Response parsing methods for AI analysis

func (p *DefaultAIResponseProcessor) parseValidationResponse(response string) (*ValidationResult, error) {
	jsonData := p.extractJSONFromResponse(response)
	if jsonData == "" {
		return nil, fmt.Errorf("no valid JSON found in validation response")
	}

	var result ValidationResult
	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		return nil, fmt.Errorf("failed to parse validation response: %w", err)
	}

	return &result, nil
}

func (p *DefaultAIResponseProcessor) parseReasoningAnalysisResponse(response string) (*ReasoningAnalysis, error) {
	jsonData := p.extractJSONFromResponse(response)
	if jsonData == "" {
		return nil, fmt.Errorf("no valid JSON found in reasoning analysis response")
	}

	var result ReasoningAnalysis
	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		return nil, fmt.Errorf("failed to parse reasoning analysis response: %w", err)
	}

	return &result, nil
}

func (p *DefaultAIResponseProcessor) parseConfidenceAssessmentResponse(response string) (*ConfidenceAssessment, error) {
	jsonData := p.extractJSONFromResponse(response)
	if jsonData == "" {
		return nil, fmt.Errorf("no valid JSON found in confidence assessment response")
	}

	var rawResult map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &rawResult); err != nil {
		return nil, fmt.Errorf("failed to parse confidence assessment response: %w", err)
	}

	result := &ConfidenceAssessment{}

	// Parse calibrated confidence
	if val, ok := rawResult["calibrated_confidence"].(float64); ok {
		result.CalibratedConfidence = val
	}

	// Parse confidence reliability
	if val, ok := rawResult["confidence_reliability"].(float64); ok {
		result.ConfidenceReliability = val
	}

	// Parse uncertainty factors
	if factors, ok := rawResult["uncertainty_factors"].([]interface{}); ok {
		for _, factor := range factors {
			if factorMap, ok := factor.(map[string]interface{}); ok {
				uf := UncertaintyFactor{}
				if val, ok := factorMap["factor"].(string); ok {
					uf.Factor = val
				}
				if val, ok := factorMap["impact"].(string); ok {
					uf.Impact = val
				}
				if val, ok := factorMap["magnitude"].(float64); ok {
					uf.Magnitude = val
				}
				if val, ok := factorMap["description"].(string); ok {
					uf.Description = val
				}
				result.UncertaintyFactors = append(result.UncertaintyFactors, uf)
			}
		}
	}

	// Parse confidence interval
	if interval, ok := rawResult["confidence_interval"].(map[string]interface{}); ok {
		ci := &ConfidenceInterval{}
		if val, ok := interval["lower"].(float64); ok {
			ci.Lower = val
		}
		if val, ok := interval["upper"].(float64); ok {
			ci.Upper = val
		}
		ci.Width = ci.Upper - ci.Lower
		ci.Reliability = 0.8 // Default reliability
		result.ConfidenceInterval = ci
	}

	// Parse calibration notes
	if val, ok := rawResult["calibration_notes"].(string); ok {
		result.CalibrationNotes = val
	}

	// Parse suggested threshold
	if val, ok := rawResult["suggested_threshold"].(float64); ok {
		result.SuggestedThreshold = val
	}

	return result, nil
}

func (p *DefaultAIResponseProcessor) parseContextualEnhancementResponse(response string) (*ContextualEnhancement, error) {
	jsonData := p.extractJSONFromResponse(response)
	if jsonData == "" {
		return nil, fmt.Errorf("no valid JSON found in contextual enhancement response")
	}

	var rawResult map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &rawResult); err != nil {
		return nil, fmt.Errorf("failed to parse contextual enhancement response: %w", err)
	}

	result := &ContextualEnhancement{}

	// Parse situational context
	if situationalCtx, ok := rawResult["situational_context"].(map[string]interface{}); ok {
		sc := &SituationalContext{}
		if val, ok := situationalCtx["urgency"].(string); ok {
			sc.Urgency = val
		}
		if val, ok := situationalCtx["business_impact"].(string); ok {
			sc.BusinessImpact = val
		}
		if val, ok := situationalCtx["maintenance_window"].(bool); ok {
			sc.MaintenanceWindow = val
		}
		if val, ok := situationalCtx["peak_traffic"].(bool); ok {
			sc.PeakTraffic = val
		}
		result.SituationalContext = sc
	}

	// Parse cascading effects
	if effects, ok := rawResult["cascading_effects"].([]interface{}); ok {
		for _, effect := range effects {
			if effectMap, ok := effect.(map[string]interface{}); ok {
				ce := CascadingEffect{}
				if val, ok := effectMap["effect"].(string); ok {
					ce.Effect = val
				}
				if val, ok := effectMap["probability"].(float64); ok {
					ce.Probability = val
				}
				if val, ok := effectMap["impact"].(string); ok {
					ce.Impact = val
				}
				if val, ok := effectMap["mitigation"].(string); ok {
					ce.Mitigation = val
				}
				result.CascadingEffects = append(result.CascadingEffects, ce)
			}
		}
	}

	// Parse timeline analysis
	if timeline, ok := rawResult["timeline_analysis"].(map[string]interface{}); ok {
		ta := &TimelineAnalysis{}
		if val, ok := timeline["expected_duration"].(string); ok {
			if duration, err := time.ParseDuration(val); err == nil {
				ta.ExpectedDuration = duration
			}
		}
		if val, ok := timeline["optimal_timing"].(string); ok {
			ta.OptimalTiming = val
		}
		result.TimelineAnalysis = ta
	}

	// Parse suggested monitoring
	if monitoring, ok := rawResult["suggested_monitoring"].([]interface{}); ok {
		for _, monitor := range monitoring {
			if monitorMap, ok := monitor.(map[string]interface{}); ok {
				mp := MonitoringPoint{}
				if val, ok := monitorMap["metric"].(string); ok {
					mp.Metric = val
				}
				if val, ok := monitorMap["threshold"]; ok {
					mp.Threshold = val
				}
				if val, ok := monitorMap["rationale"].(string); ok {
					mp.Rationale = val
				}
				if val, ok := monitorMap["duration"].(string); ok {
					if duration, err := time.ParseDuration(val); err == nil {
						mp.Duration = duration
					}
				}
				result.SuggestedMonitoring = append(result.SuggestedMonitoring, mp)
			}
		}
	}

	return result, nil
}

// Fallback methods for when AI analysis fails

func (p *DefaultAIResponseProcessor) performBasicValidation(recommendation *types.ActionRecommendation, _ types.Alert) *ValidationResult {
	violations := []ValidationViolation{}

	// Basic action validation
	if recommendation.Action == "" {
		violations = append(violations, ValidationViolation{
			Type:       "action",
			Severity:   "error",
			Message:    "Action is empty",
			Suggestion: "Provide a valid action type",
			RuleID:     "action_required",
		})
	}

	// Basic confidence validation
	if recommendation.Confidence < 0.0 || recommendation.Confidence > 1.0 {
		violations = append(violations, ValidationViolation{
			Type:       "confidence",
			Severity:   "error",
			Message:    "Confidence out of valid range",
			Suggestion: "Confidence must be between 0.0 and 1.0",
			RuleID:     "confidence_range",
		})
	}

	// Basic reasoning validation
	if recommendation.Reasoning == nil || recommendation.Reasoning.Summary == "" {
		violations = append(violations, ValidationViolation{
			Type:       "reasoning",
			Severity:   "warning",
			Message:    "No reasoning provided",
			Suggestion: "Include reasoning for the recommendation",
			RuleID:     "reasoning_required",
		})
	}

	isValid := len(violations) == 0 || (len(violations) == 1 && violations[0].Severity == "warning")

	// Basic risk assessment
	riskLevel := "medium"
	blastRadius := "deployment"
	reversibilityScore := 0.7

	// Adjust risk based on action type
	switch recommendation.Action {
	case "restart_pod", "increase_resources":
		riskLevel = "low"
		blastRadius = "pod"
		reversibilityScore = 0.9
	case "scale_deployment", "rollback_deployment":
		riskLevel = "medium"
		blastRadius = "deployment"
		reversibilityScore = 0.8
	case "drain_node", "restart_network":
		riskLevel = "high"
		blastRadius = "cluster"
		reversibilityScore = 0.5
	case "quarantine_pod", "rotate_secrets":
		riskLevel = "critical"
		blastRadius = "namespace"
		reversibilityScore = 0.3
	}

	return &ValidationResult{
		IsValid:            isValid,
		ValidationScore:    0.7,  // Basic validation score
		ActionAppropriate:  true, // Assume appropriate without advanced analysis
		ParametersComplete: recommendation.Parameters != nil,
		RiskAssessment: &RiskAssessment{
			RiskLevel:          riskLevel,
			BlastRadius:        blastRadius,
			ReversibilityScore: reversibilityScore,
			ImpactAnalysis:     map[string]interface{}{"method": "basic"},
			SafetyChecks:       []string{"basic_validation"},
			PreconditionsMet:   true,
		},
		Violations:         violations,
		Recommendations:    []string{"Consider additional validation with AI analysis"},
		AlternativeActions: []string{}, // Would need domain knowledge to suggest alternatives
	}
}

func (p *DefaultAIResponseProcessor) performBasicReasoningAnalysis(reasoning *types.ReasoningDetails) *ReasoningAnalysis {
	if reasoning == nil || reasoning.Summary == "" {
		return &ReasoningAnalysis{
			QualityScore:       0.3,
			CoherenceScore:     0.3,
			CompletenessScore:  0.2,
			LogicalConsistency: false,
			EvidenceSupport:    0.2,
			BiasDetection:      []BiasIndicator{},
			ReasoningChain:     []ReasoningStep{},
			Gaps:               []string{"No reasoning provided"},
			Strengths:          []string{},
		}
	}

	// Basic analysis based on reasoning length and content
	reasoningLength := len(reasoning.Summary)
	hasEvidence := reasoning.PrimaryReason != "" || strings.Contains(strings.ToLower(reasoning.Summary), "because")
	hasContext := reasoning.HistoricalContext != ""

	qualityScore := 0.5
	if reasoningLength > 100 {
		qualityScore += 0.2
	}
	if hasEvidence {
		qualityScore += 0.2
	}
	if hasContext {
		qualityScore += 0.1
	}

	return &ReasoningAnalysis{
		QualityScore:       qualityScore,
		CoherenceScore:     0.6, // Default moderate coherence
		CompletenessScore:  0.5,
		LogicalConsistency: true, // Assume consistent without deep analysis
		EvidenceSupport: func() float64 {
			if hasEvidence {
				return 0.7
			}
			return 0.3
		}(),
		BiasDetection: []BiasIndicator{},
		ReasoningChain: []ReasoningStep{
			{
				Step:        1,
				Description: "Basic reasoning analysis",
				Evidence:    reasoning.Summary,
				Conclusion:  "Reasoning provided",
				Confidence:  qualityScore,
			},
		},
		Gaps: func() []string {
			gaps := []string{}
			if !hasEvidence {
				gaps = append(gaps, "Lack of supporting evidence")
			}
			if !hasContext {
				gaps = append(gaps, "Missing contextual information")
			}
			return gaps
		}(),
		Strengths: func() []string {
			strengths := []string{}
			if reasoningLength > 50 {
				strengths = append(strengths, "Substantive reasoning provided")
			}
			if hasEvidence {
				strengths = append(strengths, "Contains supporting evidence")
			}
			return strengths
		}(),
	}
}

func (p *DefaultAIResponseProcessor) performBasicConfidenceAssessment(recommendation *types.ActionRecommendation) *ConfidenceAssessment {
	originalConfidence := recommendation.Confidence

	// Basic confidence calibration - slightly conservative
	calibratedConfidence := originalConfidence * 0.9

	// Identify basic uncertainty factors
	uncertaintyFactors := []UncertaintyFactor{}

	if recommendation.Reasoning == nil || recommendation.Reasoning.Summary == "" {
		uncertaintyFactors = append(uncertaintyFactors, UncertaintyFactor{
			Factor:      "missing_reasoning",
			Impact:      "decreases",
			Magnitude:   0.2,
			Description: "No reasoning provided for the recommendation",
		})
		calibratedConfidence -= 0.1
	}

	if len(recommendation.Parameters) == 0 {
		uncertaintyFactors = append(uncertaintyFactors, UncertaintyFactor{
			Factor:      "missing_parameters",
			Impact:      "decreases",
			Magnitude:   0.1,
			Description: "No parameters specified for the action",
		})
		calibratedConfidence -= 0.05
	}

	// Ensure calibrated confidence stays in valid range
	if calibratedConfidence < 0.0 {
		calibratedConfidence = 0.0
	}
	if calibratedConfidence > 1.0 {
		calibratedConfidence = 1.0
	}

	return &ConfidenceAssessment{
		CalibratedConfidence:  calibratedConfidence,
		OriginalConfidence:    originalConfidence,
		ConfidenceReliability: 0.6, // Moderate reliability for basic assessment
		UncertaintyFactors:    uncertaintyFactors,
		ConfidenceInterval: &ConfidenceInterval{
			Lower:       calibratedConfidence - 0.1,
			Upper:       calibratedConfidence + 0.1,
			Width:       0.2,
			Reliability: 0.6,
		},
		CalibrationNotes:   "Basic confidence assessment without AI analysis",
		SuggestedThreshold: 0.7,
	}
}

func (p *DefaultAIResponseProcessor) performBasicContextualEnhancement(recommendation *types.ActionRecommendation, alert types.Alert) *ContextualEnhancement {
	// Basic urgency assessment based on alert severity
	urgency := "medium"
	switch strings.ToLower(alert.Severity) {
	case "critical":
		urgency = "critical"
	case "warning":
		urgency = "high"
	case "info":
		urgency = "low"
	}

	// Basic business impact assessment
	businessImpact := "moderate"
	if strings.Contains(strings.ToLower(alert.Namespace), "prod") {
		businessImpact = "high"
	}

	return &ContextualEnhancement{
		SituationalContext: &SituationalContext{
			Urgency:           urgency,
			BusinessImpact:    businessImpact,
			MaintenanceWindow: false, // Would need external data to determine
			PeakTraffic:       false, // Would need external data to determine
			RelatedAlerts:     1,     // At least this alert
			SystemLoad:        map[string]interface{}{"method": "basic"},
		},
		HistoricalPatterns: []HistoricalPattern{}, // Would need historical data
		RelatedIncidents:   []RelatedIncident{},   // Would need incident data
		SystemStateAnalysis: &SystemStateAnalysis{
			HealthScore:         0.7, // Default moderate health
			StabilityScore:      0.7,
			CapacityUtilization: map[string]float64{"overall": 0.6},
			BottleneckAnalysis:  []Bottleneck{},
			Dependencies:        []string{},
			CriticalPath:        []string{},
		},
		CascadingEffects: []CascadingEffect{
			{
				Effect:      "Service degradation",
				Probability: 0.3,
				Impact:      "medium",
				Mitigation:  "Monitor service metrics",
				Timeline:    "immediate",
			},
		},
		TimelineAnalysis: &TimelineAnalysis{
			ExpectedDuration: 15 * time.Minute, // Default estimation
			CriticalWindow:   5 * time.Minute,
			OptimalTiming:    "immediate",
			Dependencies:     []string{},
			Milestones:       []Milestone{},
		},
		SuggestedMonitoring: []MonitoringPoint{
			{
				Metric:    "response_time",
				Threshold: "5s",
				Duration:  10 * time.Minute,
				Rationale: "Monitor service recovery",
			},
		},
	}
}

// Helper methods

func (p *DefaultAIResponseProcessor) extractJSONFromResponse(response string) string {
	// Find JSON blocks in the response
	start := strings.Index(response, "{")
	if start == -1 {
		return ""
	}

	// Find the matching closing bracket
	depth := 0
	for i, char := range response[start:] {
		if char == '{' {
			depth++
		} else if char == '}' {
			depth--
			if depth == 0 {
				return response[start : start+i+1]
			}
		}
	}

	return ""
}

func (p *DefaultAIResponseProcessor) applyValidationRules(validation *ValidationResult, recommendation *types.ActionRecommendation, alert types.Alert) {
	// Apply knowledge base validation rules
	for _, rule := range p.validationRules {
		switch rule.Type {
		case "action":
			if recommendation.Action == "" && rule.ID == "action_exists" {
				validation.Violations = append(validation.Violations, ValidationViolation{
					Type:       rule.Type,
					Severity:   rule.Severity,
					Message:    rule.Message,
					Suggestion: rule.Suggestion,
					RuleID:     rule.ID,
				})
			}
		case "confidence":
			if (recommendation.Confidence < 0.0 || recommendation.Confidence > 1.0) && rule.ID == "confidence_range" {
				validation.Violations = append(validation.Violations, ValidationViolation{
					Type:       rule.Type,
					Severity:   rule.Severity,
					Message:    rule.Message,
					Suggestion: rule.Suggestion,
					RuleID:     rule.ID,
				})
			}
		case "reasoning":
			if (recommendation.Reasoning == nil || len(recommendation.Reasoning.Summary) < 20) && rule.ID == "reasoning_length" {
				validation.Violations = append(validation.Violations, ValidationViolation{
					Type:       rule.Type,
					Severity:   rule.Severity,
					Message:    rule.Message,
					Suggestion: rule.Suggestion,
					RuleID:     rule.ID,
				})
			}
		}
	}

	// Update validation status based on violations
	hasErrors := false
	for _, violation := range validation.Violations {
		if violation.Severity == "error" || violation.Severity == "critical" {
			hasErrors = true
			break
		}
	}
	validation.IsValid = !hasErrors
}

func (p *DefaultAIResponseProcessor) enrichWithKnowledgeBase(enhancement *ContextualEnhancement, recommendation *types.ActionRecommendation, alert types.Alert) {
	if p.knowledgeBase == nil {
		return
	}

	// Enrich with historical patterns
	patterns := p.knowledgeBase.GetHistoricalPatterns(alert)
	enhancement.HistoricalPatterns = patterns

	// Enrich with risk assessment from knowledge base
	if enhancement.SituationalContext != nil {
		riskData := p.knowledgeBase.GetActionRisks(recommendation.Action)
		if riskData != nil {
			// Use risk data to enhance situational context
			if riskData.RiskLevel == "high" || riskData.RiskLevel == "critical" {
				enhancement.SituationalContext.Urgency = "high"
			}
		}
	}
}
