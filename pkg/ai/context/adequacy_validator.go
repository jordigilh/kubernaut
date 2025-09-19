package context

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ============================================================================
// STRUCTURED CONTEXT TYPES - Following project guideline: use structured field values instead of interface{}
// ============================================================================

// ContextData represents structured context data instead of map[string]interface{}
// Following project guideline: ALWAYS attempt to use structured field values and AVOID using any or interface{}
type ContextData struct {
	Kubernetes    *KubernetesContext    `json:"kubernetes,omitempty"`
	Metrics       *MetricsContext       `json:"metrics,omitempty"`
	Logs          *LogsContext          `json:"logs,omitempty"`
	ActionHistory *ActionHistoryContext `json:"action-history,omitempty"`
	Events        *EventsContext        `json:"events,omitempty"`
	Traces        *TracesContext        `json:"traces,omitempty"`
	NetworkFlows  *NetworkFlowsContext  `json:"network-flows,omitempty"`
	AuditLogs     *AuditLogsContext     `json:"audit-logs,omitempty"`
}

// KubernetesContext contains Kubernetes-specific context information
type KubernetesContext struct {
	Namespace    string            `json:"namespace,omitempty"`
	ResourceType string            `json:"resource_type,omitempty"`
	ResourceName string            `json:"resource_name,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	ClusterInfo  *ClusterInfo      `json:"cluster_info,omitempty"`
	CollectedAt  time.Time         `json:"collected_at"`
}

// ClusterInfo contains cluster-level information
type ClusterInfo struct {
	Version     string `json:"version,omitempty"`
	NodeCount   int    `json:"node_count,omitempty"`
	Region      string `json:"region,omitempty"`
	Environment string `json:"environment,omitempty"`
}

// MetricsContext contains metrics-specific context information
type MetricsContext struct {
	Source       string             `json:"source"`
	TimeRange    *types.TimeRange   `json:"time_range,omitempty"`
	MetricsData  map[string]float64 `json:"metrics_data,omitempty"`
	Aggregations map[string]string  `json:"aggregations,omitempty"`
	CollectedAt  time.Time          `json:"collected_at"`
}

// LogsContext contains logs-specific context information
type LogsContext struct {
	Source      string           `json:"source"`
	TimeRange   *types.TimeRange `json:"time_range,omitempty"` // Now uses shared TimeRange following project guidelines
	LogLevel    string           `json:"log_level,omitempty"`
	LogEntries  []LogEntry       `json:"log_entries,omitempty"`
	Patterns    []string         `json:"patterns,omitempty"`
	CollectedAt time.Time        `json:"collected_at"`
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Source    string            `json:"source,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// ActionHistoryContext contains action history context information
type ActionHistoryContext struct {
	TimeRange    *types.TimeRange `json:"time_range,omitempty"`
	Actions      []HistoryAction  `json:"actions,omitempty"`
	TotalActions int              `json:"total_actions"`
	CollectedAt  time.Time        `json:"collected_at"`
}

// HistoryAction represents a historical action
type HistoryAction struct {
	ActionID   string                 `json:"action_id"`
	Timestamp  time.Time              `json:"timestamp"`
	ActionType string                 `json:"action_type"`
	Success    bool                   `json:"success"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// EventsContext contains events-specific context information
type EventsContext struct {
	Source      string           `json:"source"`
	TimeRange   *types.TimeRange `json:"time_range,omitempty"` // Now uses shared TimeRange following project guidelines
	Events      []Event          `json:"events,omitempty"`
	EventTypes  []string         `json:"event_types,omitempty"`
	CollectedAt time.Time        `json:"collected_at"`
}

// Event represents a single event
type Event struct {
	Timestamp time.Time         `json:"timestamp"`
	Type      string            `json:"type"`
	Reason    string            `json:"reason"`
	Message   string            `json:"message"`
	Source    string            `json:"source,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// TracesContext contains distributed tracing context information
type TracesContext struct {
	Source      string           `json:"source"`
	TimeRange   *types.TimeRange `json:"time_range,omitempty"` // Now uses shared TimeRange following project guidelines
	TraceCount  int              `json:"trace_count"`
	SpanCount   int              `json:"span_count"`
	ErrorRate   float64          `json:"error_rate,omitempty"`
	CollectedAt time.Time        `json:"collected_at"`
}

// NetworkFlowsContext contains network flow context information
type NetworkFlowsContext struct {
	Source      string           `json:"source"`
	TimeRange   *types.TimeRange `json:"time_range,omitempty"` // Now uses shared TimeRange following project guidelines
	FlowCount   int              `json:"flow_count"`
	Connections []NetworkFlow    `json:"connections,omitempty"`
	CollectedAt time.Time        `json:"collected_at"`
}

// NetworkFlow represents a network flow
type NetworkFlow struct {
	SourceIP      string    `json:"source_ip"`
	DestinationIP string    `json:"destination_ip"`
	Port          int       `json:"port"`
	Protocol      string    `json:"protocol"`
	BytesIn       int64     `json:"bytes_in"`
	BytesOut      int64     `json:"bytes_out"`
	Timestamp     time.Time `json:"timestamp"`
}

// AuditLogsContext contains audit logs context information
type AuditLogsContext struct {
	Source      string           `json:"source"`
	TimeRange   *types.TimeRange `json:"time_range,omitempty"` // Now uses shared TimeRange following project guidelines
	AuditEvents []AuditEvent     `json:"audit_events,omitempty"`
	CollectedAt time.Time        `json:"collected_at"`
}

// AuditEvent represents a single audit event
type AuditEvent struct {
	Timestamp time.Time         `json:"timestamp"`
	User      string            `json:"user,omitempty"`
	Action    string            `json:"action"`
	Resource  string            `json:"resource,omitempty"`
	Result    string            `json:"result"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// Use shared TimeRange type following project guideline: REUSE existing code and AVOID duplication
// TimeRange is defined in pkg/shared/types/common.go

// ============================================================================
// ENHANCED INVESTIGATION REQUIREMENTS - Moving hard-coded data to structure
// ============================================================================

// AdequacyValidator implements BR-CONTEXT-021 to BR-CONTEXT-025
// Context adequacy validation and enrichment decisions
type AdequacyValidator struct {
	config                    *config.ContextOptimizationConfig
	investigationRequirements map[string]InvestigationRequirements
}

// InvestigationRequirements defines what context is needed for different investigation types
// Enhanced to include previously hard-coded configuration data following project guidelines
type InvestigationRequirements struct {
	RequiredContextTypes []string           `json:"required_context_types"`
	MinimumAdequacyScore float64            `json:"minimum_adequacy_score"`
	OptionalContextTypes []string           `json:"optional_context_types"`
	ComplexityFactors    map[string]float64 `json:"complexity_factors"`
	HighValueOptional    []string           `json:"high_value_optional"` // Moved from hard-coded list in identifyMissingContext
	AnalysisDepth        string             `json:"analysis_depth"`      // Moved from hard-coded depthMap in performSufficiencyAnalysis
}

// NewAdequacyValidator creates a new adequacy validator
func NewAdequacyValidator(cfg *config.ContextOptimizationConfig) *AdequacyValidator {
	validator := &AdequacyValidator{
		config:                    cfg,
		investigationRequirements: make(map[string]InvestigationRequirements),
	}

	// Initialize investigation requirements
	validator.initializeRequirements()

	return validator
}

// Validate assesses context adequacy for a given investigation type
// Updated to use structured ContextData following project guideline: use structured field values instead of interface{}
func (v *AdequacyValidator) Validate(ctx context.Context, contextData *ContextData, investigationType string) (*AdequacyAssessment, error) {
	// Get requirements for investigation type
	requirements, exists := v.investigationRequirements[investigationType]
	if !exists {
		// Default requirements for unknown investigation types
		requirements = InvestigationRequirements{
			RequiredContextTypes: []string{"kubernetes"},
			MinimumAdequacyScore: 0.60,
			OptionalContextTypes: []string{"metrics", "logs"},
			ComplexityFactors:    map[string]float64{"default": 1.0},
			HighValueOptional:    []string{"metrics"},
			AnalysisDepth:        "standard",
		}
	}

	// Calculate adequacy score
	adequacyScore := v.calculateAdequacyScore(contextData, requirements)

	// Determine if context is adequate
	isAdequate := adequacyScore >= requirements.MinimumAdequacyScore

	// Identify missing context types
	missingTypes := v.identifyMissingContext(contextData, requirements)

	// Determine if enrichment is required
	enrichmentRequired := !isAdequate || len(missingTypes) > 0

	// Calculate confidence level
	confidenceLevel := v.calculateConfidenceLevel(contextData, requirements, adequacyScore)

	// Perform sufficiency analysis
	sufficiencyAnalysis := v.performSufficiencyAnalysis(contextData, requirements)

	return &AdequacyAssessment{
		IsAdequate:          isAdequate,
		AdequacyScore:       adequacyScore,
		ConfidenceLevel:     confidenceLevel,
		EnrichmentRequired:  enrichmentRequired,
		MissingContextTypes: missingTypes,
		SufficiencyAnalysis: sufficiencyAnalysis,
		Metadata: map[string]interface{}{
			"investigation_type":     investigationType,
			"context_types_present":  v.countContextTypesPresent(contextData),
			"required_types_count":   len(requirements.RequiredContextTypes),
			"optional_types_count":   len(requirements.OptionalContextTypes),
			"minimum_score_required": requirements.MinimumAdequacyScore,
			"validation_method":      "requirement_based_scoring",
		},
	}, nil
}

// calculateAdequacyScore computes a score indicating context adequacy
// Updated to use structured ContextData following project guidelines
func (v *AdequacyValidator) calculateAdequacyScore(contextData *ContextData, requirements InvestigationRequirements) float64 {
	score := 0.0
	maxScore := 0.0

	// Score for required context types
	for _, requiredType := range requirements.RequiredContextTypes {
		maxScore += 1.0
		if v.hasContextType(contextData, requiredType) {
			score += 1.0
		}
	}

	// Score for optional context types (weighted less)
	for _, optionalType := range requirements.OptionalContextTypes {
		maxScore += 0.5
		if v.hasContextType(contextData, optionalType) {
			score += 0.5
		}
	}

	// Apply complexity factors
	for contextType, factor := range requirements.ComplexityFactors {
		if v.hasContextType(contextData, contextType) {
			score += factor * 0.2 // Bonus for complex context types
		}
	}

	// Normalize to 0-1 range
	if maxScore > 0 {
		normalizedScore := score / maxScore
		if normalizedScore > 1.0 {
			normalizedScore = 1.0
		}
		return normalizedScore
	}

	return 0.0
}

// identifyMissingContext finds context types that are missing but required/recommended
// Updated to use structured requirements and ContextData following project guidelines
func (v *AdequacyValidator) identifyMissingContext(contextData *ContextData, requirements InvestigationRequirements) []string {
	var missing []string

	// Check required context types
	for _, requiredType := range requirements.RequiredContextTypes {
		if !v.hasContextType(contextData, requiredType) {
			missing = append(missing, requiredType)
		}
	}

	// Check high-value optional types from structured requirements (no more hard-coded data)
	for _, optionalType := range requirements.HighValueOptional {
		if !v.hasContextType(contextData, optionalType) {
			// Only add if it's in the optional list for this investigation
			for _, reqOptional := range requirements.OptionalContextTypes {
				if reqOptional == optionalType {
					missing = append(missing, optionalType)
					break
				}
			}
		}
	}

	return missing
}

// calculateConfidenceLevel determines confidence in the adequacy assessment
// Updated to use ContextData and reuse existing helper methods following project guidelines
func (v *AdequacyValidator) calculateConfidenceLevel(contextData *ContextData, requirements InvestigationRequirements, adequacyScore float64) float64 {
	confidence := 0.7 // Base confidence

	// Higher confidence when all required types are present - reuse existing helper method
	requiredMet := v.countRequiredTypesMet(contextData, requirements)
	allRequiredPresent := requiredMet == len(requirements.RequiredContextTypes)

	if allRequiredPresent {
		confidence += 0.15
	}

	// Higher confidence with more context types
	contextTypesPresent := v.countContextTypesPresent(contextData)
	if contextTypesPresent >= len(requirements.RequiredContextTypes)*2 {
		confidence += 0.1
	}

	// Adjust based on adequacy score clarity
	if adequacyScore >= 0.9 || adequacyScore <= 0.3 {
		confidence += 0.05 // Clear high or low adequacy
	}

	// Ensure confidence is in valid range
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.5 {
		confidence = 0.5
	}

	return confidence
}

// performSufficiencyAnalysis provides detailed analysis of context sufficiency
// Uses structured requirements data following project guideline: AVOID duplication and REUSE existing code
func (v *AdequacyValidator) performSufficiencyAnalysis(contextData *ContextData, requirements InvestigationRequirements) map[string]interface{} {
	analysis := make(map[string]interface{})

	// Context completeness analysis
	completeness := make(map[string]bool)
	for _, requiredType := range requirements.RequiredContextTypes {
		completeness[requiredType] = v.hasContextType(contextData, requiredType)
	}
	analysis["context_completeness"] = completeness

	// Analysis depth assessment - use structured requirements instead of hard-coded map
	// Following project guideline: AVOID duplication and REUSE existing code
	if requirements.AnalysisDepth != "" {
		analysis["analysis_depth"] = requirements.AnalysisDepth
	} else {
		analysis["analysis_depth"] = "standard"
	}

	// Confidence level based on requirements fulfillment
	investigationConfidence := v.getInvestigationSpecificConfidence(contextData, requirements)
	analysis["confidence_level"] = investigationConfidence

	// Coverage assessment
	contextTypesPresent := v.countContextTypesPresent(contextData)
	totalPossibleTypes := len(requirements.RequiredContextTypes) + len(requirements.OptionalContextTypes)
	coverage := float64(contextTypesPresent) / float64(totalPossibleTypes)
	if coverage > 1.0 {
		coverage = 1.0
	}
	analysis["coverage_ratio"] = coverage

	// Quality indicators
	analysis["quality_indicators"] = map[string]interface{}{
		"required_types_met":     v.countRequiredTypesMet(contextData, requirements),
		"optional_types_present": v.countOptionalTypesPresent(contextData, requirements),
		"enrichment_potential":   v.assessEnrichmentPotential(contextData, requirements),
	}

	return analysis
}

// getInvestigationSpecificConfidence calculates confidence based on requirements fulfillment
// Uses structured requirements data following project guideline: AVOID duplication and REUSE existing code
func (v *AdequacyValidator) getInvestigationSpecificConfidence(contextData *ContextData, requirements InvestigationRequirements) float64 {
	baseConfidence := 0.7

	// Calculate confidence based on how well the context meets the structured requirements
	requiredTypesMet := v.countRequiredTypesMet(contextData, requirements)
	totalRequired := len(requirements.RequiredContextTypes)

	if totalRequired == 0 {
		return baseConfidence
	}

	// Base confidence adjustment based on requirement fulfillment ratio
	fulfillmentRatio := float64(requiredTypesMet) / float64(totalRequired)

	// Adjust confidence based on how well requirements are met
	if fulfillmentRatio >= 1.0 {
		// All required types present - high confidence
		baseConfidence = 0.85

		// Additional bonus for optional types present
		optionalPresent := v.countOptionalTypesPresent(contextData, requirements)
		if optionalPresent > 0 {
			bonusConfidence := float64(optionalPresent) * 0.02 // 2% per optional type
			baseConfidence += bonusConfidence
		}

		// Apply complexity factors for specialized context types
		for contextType, factor := range requirements.ComplexityFactors {
			if v.hasContextType(contextData, contextType) {
				baseConfidence += factor * 0.1 // Bonus for complex context availability
			}
		}
	} else if fulfillmentRatio >= 0.5 {
		// Partial requirements met - medium confidence
		baseConfidence = 0.65 + (fulfillmentRatio * 0.15)
	} else {
		// Few requirements met - lower confidence
		baseConfidence = 0.50 + (fulfillmentRatio * 0.15)
	}

	// Apply minimum adequacy score influence
	if requirements.MinimumAdequacyScore > 0.8 {
		// High adequacy requirements - be more conservative
		baseConfidence *= 0.95
	} else if requirements.MinimumAdequacyScore < 0.65 {
		// Lower adequacy requirements - can be more confident
		baseConfidence *= 1.05
	}

	// Ensure confidence stays within valid bounds
	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	} else if baseConfidence < 0.5 {
		baseConfidence = 0.5
	}

	return baseConfidence
}

// Helper methods

// hasContextType checks if a specific context type is present in ContextData
func (v *AdequacyValidator) hasContextType(contextData *ContextData, contextType string) bool {
	switch contextType {
	case "kubernetes":
		return contextData.Kubernetes != nil
	case "metrics":
		return contextData.Metrics != nil
	case "logs":
		return contextData.Logs != nil
	case "action-history":
		return contextData.ActionHistory != nil
	case "events":
		return contextData.Events != nil
	case "traces":
		return contextData.Traces != nil
	case "network-flows":
		return contextData.NetworkFlows != nil
	case "audit-logs":
		return contextData.AuditLogs != nil
	default:
		return false
	}
}

// countContextTypesPresent counts how many context types are present in ContextData
func (v *AdequacyValidator) countContextTypesPresent(contextData *ContextData) int {
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

func (v *AdequacyValidator) countRequiredTypesMet(contextData *ContextData, requirements InvestigationRequirements) int {
	count := 0
	for _, requiredType := range requirements.RequiredContextTypes {
		if v.hasContextType(contextData, requiredType) {
			count++
		}
	}
	return count
}

func (v *AdequacyValidator) countOptionalTypesPresent(contextData *ContextData, requirements InvestigationRequirements) int {
	count := 0
	for _, optionalType := range requirements.OptionalContextTypes {
		if v.hasContextType(contextData, optionalType) {
			count++
		}
	}
	return count
}

func (v *AdequacyValidator) assessEnrichmentPotential(contextData *ContextData, requirements InvestigationRequirements) string {
	missingRequired := 0
	for _, requiredType := range requirements.RequiredContextTypes {
		if !v.hasContextType(contextData, requiredType) {
			missingRequired++
		}
	}

	if missingRequired > 0 {
		return "high"
	}

	missingOptional := 0
	for _, optionalType := range requirements.OptionalContextTypes {
		if !v.hasContextType(contextData, optionalType) {
			missingOptional++
		}
	}

	if missingOptional > len(requirements.OptionalContextTypes)/2 {
		return "medium"
	}

	return "low"
}

// initializeRequirements sets up investigation type requirements
// Enhanced with previously hard-coded data following project guidelines
func (v *AdequacyValidator) initializeRequirements() {
	// Root cause analysis requirements
	v.investigationRequirements["root_cause_analysis"] = InvestigationRequirements{
		RequiredContextTypes: []string{"metrics", "kubernetes", "action-history"},
		MinimumAdequacyScore: 0.85,
		OptionalContextTypes: []string{"logs", "events", "traces"},
		ComplexityFactors: map[string]float64{
			"traces":         0.3,
			"action-history": 0.2,
		},
		HighValueOptional: []string{"metrics", "action-history"},
		AnalysisDepth:     "comprehensive",
	}

	// Performance optimization requirements
	v.investigationRequirements["performance_optimization"] = InvestigationRequirements{
		RequiredContextTypes: []string{"metrics", "kubernetes"},
		MinimumAdequacyScore: 0.75,
		OptionalContextTypes: []string{"action-history", "traces"},
		ComplexityFactors: map[string]float64{
			"metrics": 0.2,
			"traces":  0.15,
		},
		HighValueOptional: []string{"metrics", "action-history"},
		AnalysisDepth:     "detailed",
	}

	// Security incident response requirements
	v.investigationRequirements["security_incident_response"] = InvestigationRequirements{
		RequiredContextTypes: []string{"kubernetes", "logs", "events"},
		MinimumAdequacyScore: 0.80,
		OptionalContextTypes: []string{"metrics", "audit-logs", "network-flows"},
		ComplexityFactors: map[string]float64{
			"audit-logs":    0.25,
			"network-flows": 0.2,
		},
		HighValueOptional: []string{"audit-logs", "network-flows"},
		AnalysisDepth:     "thorough",
	}

	// Basic investigation requirements
	v.investigationRequirements["basic_investigation"] = InvestigationRequirements{
		RequiredContextTypes: []string{"kubernetes"},
		MinimumAdequacyScore: 0.60,
		OptionalContextTypes: []string{"metrics", "logs"},
		ComplexityFactors:    map[string]float64{},
		HighValueOptional:    []string{"metrics"},
		AnalysisDepth:        "basic",
	}

	// Basic troubleshooting (for backward compatibility with existing tests)
	v.investigationRequirements["basic_troubleshooting"] = InvestigationRequirements{
		RequiredContextTypes: []string{"kubernetes"},
		MinimumAdequacyScore: 0.60,
		OptionalContextTypes: []string{"metrics", "logs"},
		ComplexityFactors:    map[string]float64{},
		HighValueOptional:    []string{"metrics"},
		AnalysisDepth:        "basic",
	}
}
