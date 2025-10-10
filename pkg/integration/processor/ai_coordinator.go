/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package processor

import (
	"context"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/ai/llm"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/sirupsen/logrus"
)

// AICoordinator coordinates AI analysis for alert processing
// Following Rule 12: Uses existing AI interface (llm.Client)
type AICoordinator struct {
	llmClient llm.Client // Existing interface - MANDATORY
	config    *AIConfig
	logger    *logrus.Logger
}

// AIAnalysis represents the result of AI analysis
type AIAnalysis struct {
	Confidence         float64
	RecommendedActions []string
	Reasoning          string
	RiskAssessment     *RiskAssessment
}

// NewAICoordinator creates a new AI coordinator
// Following Rule 12: Uses existing AI interface (llm.Client)
func NewAICoordinator(llmClient llm.Client, config *AIConfig) *AICoordinator {
	return &AICoordinator{
		llmClient: llmClient,
		config:    config,
		logger:    logrus.New(),
	}
}

// AnalyzeAlert performs AI analysis on the given alert
// Following Rule 12: Uses existing AI interface methods
func (c *AICoordinator) AnalyzeAlert(ctx context.Context, alert types.Alert) (*AIAnalysis, error) {
	// Prepare context for AI analysis
	analysisContext := c.prepareAnalysisContext(alert)

	// Call existing AI client (MUST use existing interface)
	response, err := c.llmClient.AnalyzeAlert(ctx, analysisContext)
	if err != nil {
		return nil, fmt.Errorf("AI analysis failed: %w", err)
	}

	// Validate AI response
	if err := c.validateAIResponse(response); err != nil {
		return nil, fmt.Errorf("invalid AI response: %w", err)
	}

	// Convert to internal analysis format
	analysis := &AIAnalysis{
		Confidence:         response.Confidence,
		RecommendedActions: []string{response.Action},
		Reasoning:          c.extractReasoning(response),
		RiskAssessment:     c.assessRisk(response),
	}

	return analysis, nil
}

// prepareAnalysisContext prepares rich context for AI analysis
func (c *AICoordinator) prepareAnalysisContext(alert types.Alert) interface{} {
	// For now, return the alert directly
	// In REFACTOR phase, this will be enhanced with:
	// - Cluster context gathering
	// - Historical data retrieval
	// - Similar incident analysis
	return alert
}

// validateAIResponse validates the AI response quality
func (c *AICoordinator) validateAIResponse(response *llm.AnalyzeAlertResponse) error {
	if response == nil {
		return fmt.Errorf("nil response")
	}

	if response.Action == "" {
		return fmt.Errorf("empty action")
	}

	if response.Confidence < 0 || response.Confidence > 1 {
		return fmt.Errorf("invalid confidence: %f", response.Confidence)
	}

	return nil
}

// extractReasoning extracts reasoning from AI response
func (c *AICoordinator) extractReasoning(response *llm.AnalyzeAlertResponse) string {
	if reasoning, ok := response.Metadata["reasoning"].(string); ok {
		return reasoning
	}
	return "AI analysis completed"
}

// assessRisk performs risk assessment based on AI response
func (c *AICoordinator) assessRisk(response *llm.AnalyzeAlertResponse) *RiskAssessment {
	if riskLevel, ok := response.Metadata["risk_level"].(string); ok {
		return &RiskAssessment{Level: riskLevel}
	}

	// Default risk assessment based on confidence
	if response.Confidence >= 0.8 {
		return &RiskAssessment{Level: "low"}
	}
	return &RiskAssessment{Level: "medium"}
}
