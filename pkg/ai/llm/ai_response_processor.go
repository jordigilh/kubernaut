package llm

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/infrastructure/types"
)

// AIResponseProcessor provides AI-powered analysis and validation of SLM responses
type AIResponseProcessor interface {
	// ProcessResponse analyzes and enhances SLM responses with AI insights
	ProcessResponse(ctx context.Context, rawResponse string, originalAlert types.Alert) (*EnhancedActionRecommendation, error)

	// ValidateRecommendation performs AI-powered validation of action recommendations
	ValidateRecommendation(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*ValidationResult, error)

	// AnalyzeReasoning performs AI analysis of the reasoning quality and coherence
	AnalyzeReasoning(ctx context.Context, reasoning *types.ReasoningDetails, alert types.Alert) (*ReasoningAnalysis, error)

	// AssessConfidence performs AI-powered confidence assessment and calibration
	AssessConfidence(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*ConfidenceAssessment, error)

	// EnhanceContext adds AI-powered contextual analysis to recommendations
	EnhanceContext(ctx context.Context, recommendation *types.ActionRecommendation, alert types.Alert) (*ContextualEnhancement, error)

	// IsHealthy returns the health status of the AI response processor
	IsHealthy() bool
}

// EnhancedActionRecommendation extends the basic recommendation with AI analysis
type EnhancedActionRecommendation struct {
	*types.ActionRecommendation
	ValidationResult      *ValidationResult      `json:"validation_result"`
	ReasoningAnalysis     *ReasoningAnalysis     `json:"reasoning_analysis"`
	ConfidenceAssessment  *ConfidenceAssessment  `json:"confidence_assessment"`
	ContextualEnhancement *ContextualEnhancement `json:"contextual_enhancement"`
	ProcessingMetadata    *ProcessingMetadata    `json:"processing_metadata"`
}

// ValidationResult contains AI-powered validation analysis
type ValidationResult struct {
	IsValid            bool                  `json:"is_valid"`
	ValidationScore    float64               `json:"validation_score"`
	ActionAppropriate  bool                  `json:"action_appropriate"`
	ParametersComplete bool                  `json:"parameters_complete"`
	RiskAssessment     *RiskAssessment       `json:"risk_assessment"`
	Violations         []ValidationViolation `json:"violations"`
	Recommendations    []string              `json:"recommendations"`
	AlternativeActions []string              `json:"alternative_actions"`
}

// RiskAssessment provides AI analysis of action risk levels
type RiskAssessment struct {
	RiskLevel          string                 `json:"risk_level"`          // "low", "medium", "high", "critical"
	BlastRadius        string                 `json:"blast_radius"`        // "pod", "deployment", "namespace", "cluster"
	ReversibilityScore float64                `json:"reversibility_score"` // 0.0-1.0, higher = more reversible
	ImpactAnalysis     map[string]interface{} `json:"impact_analysis"`
	SafetyChecks       []string               `json:"safety_checks"`
	PreconditionsMet   bool                   `json:"preconditions_met"`
}

// ValidationViolation represents a validation rule violation
type ValidationViolation struct {
	Type       string `json:"type"`
	Severity   string `json:"severity"` // "warning", "error", "critical"
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
	RuleID     string `json:"rule_id"`
}

// ReasoningAnalysis provides AI analysis of reasoning quality
type ReasoningAnalysis struct {
	QualityScore       float64         `json:"quality_score"`      // 0.0-1.0
	CoherenceScore     float64         `json:"coherence_score"`    // 0.0-1.0
	CompletenessScore  float64         `json:"completeness_score"` // 0.0-1.0
	LogicalConsistency bool            `json:"logical_consistency"`
	EvidenceSupport    float64         `json:"evidence_support"` // 0.0-1.0
	BiasDetection      []BiasIndicator `json:"bias_detection"`
	ReasoningChain     []ReasoningStep `json:"reasoning_chain"`
	Gaps               []string        `json:"gaps"`
	Strengths          []string        `json:"strengths"`
}

// BiasIndicator represents detected reasoning biases
type BiasIndicator struct {
	Type        string  `json:"type"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description"`
	Impact      string  `json:"impact"`
}

// ReasoningStep represents a step in the reasoning chain
type ReasoningStep struct {
	Step        int     `json:"step"`
	Description string  `json:"description"`
	Evidence    string  `json:"evidence"`
	Conclusion  string  `json:"conclusion"`
	Confidence  float64 `json:"confidence"`
}

// ConfidenceAssessment provides AI-powered confidence analysis
type ConfidenceAssessment struct {
	CalibratedConfidence  float64             `json:"calibrated_confidence"`
	OriginalConfidence    float64             `json:"original_confidence"`
	ConfidenceReliability float64             `json:"confidence_reliability"` // How reliable is the confidence score
	UncertaintyFactors    []UncertaintyFactor `json:"uncertainty_factors"`
	ConfidenceInterval    *ConfidenceInterval `json:"confidence_interval"`
	CalibrationNotes      string              `json:"calibration_notes"`
	SuggestedThreshold    float64             `json:"suggested_threshold"`
}

// UncertaintyFactor represents factors affecting confidence
type UncertaintyFactor struct {
	Factor      string  `json:"factor"`
	Impact      string  `json:"impact"`    // "increases", "decreases"
	Magnitude   float64 `json:"magnitude"` // 0.0-1.0
	Description string  `json:"description"`
}

// ConfidenceInterval provides uncertainty bounds
type ConfidenceInterval struct {
	Lower       float64 `json:"lower"`
	Upper       float64 `json:"upper"`
	Width       float64 `json:"width"`
	Reliability float64 `json:"reliability"`
}

// ContextualEnhancement provides AI-powered context analysis
type ContextualEnhancement struct {
	SituationalContext  *SituationalContext  `json:"situational_context"`
	HistoricalPatterns  []HistoricalPattern  `json:"historical_patterns"`
	RelatedIncidents    []RelatedIncident    `json:"related_incidents"`
	SystemStateAnalysis *SystemStateAnalysis `json:"system_state_analysis"`
	CascadingEffects    []CascadingEffect    `json:"cascading_effects"`
	TimelineAnalysis    *TimelineAnalysis    `json:"timeline_analysis"`
	SuggestedMonitoring []MonitoringPoint    `json:"suggested_monitoring"`
}

// SituationalContext provides current situational awareness
type SituationalContext struct {
	Urgency           string                 `json:"urgency"` // "low", "medium", "high", "critical"
	BusinessImpact    string                 `json:"business_impact"`
	MaintenanceWindow bool                   `json:"maintenance_window"`
	PeakTraffic       bool                   `json:"peak_traffic"`
	RelatedAlerts     int                    `json:"related_alerts"`
	SystemLoad        map[string]interface{} `json:"system_load"`
}

// HistoricalPattern represents patterns from historical data
type HistoricalPattern struct {
	Pattern       string    `json:"pattern"`
	Frequency     int       `json:"frequency"`
	LastSeen      time.Time `json:"last_seen"`
	Effectiveness float64   `json:"effectiveness"`
	Context       string    `json:"context"`
}

// RelatedIncident represents related incidents or alerts
type RelatedIncident struct {
	IncidentID string    `json:"incident_id"`
	Similarity float64   `json:"similarity"`
	Timestamp  time.Time `json:"timestamp"`
	Resolution string    `json:"resolution"`
	Outcome    string    `json:"outcome"`
}

// SystemStateAnalysis provides AI analysis of current system state
type SystemStateAnalysis struct {
	HealthScore         float64            `json:"health_score"`
	StabilityScore      float64            `json:"stability_score"`
	CapacityUtilization map[string]float64 `json:"capacity_utilization"`
	BottleneckAnalysis  []Bottleneck       `json:"bottleneck_analysis"`
	Dependencies        []string           `json:"dependencies"`
	CriticalPath        []string           `json:"critical_path"`
}

// CascadingEffect represents potential cascading effects
type CascadingEffect struct {
	Effect      string  `json:"effect"`
	Probability float64 `json:"probability"`
	Impact      string  `json:"impact"`
	Mitigation  string  `json:"mitigation"`
	Timeline    string  `json:"timeline"`
}

// TimelineAnalysis provides temporal analysis
type TimelineAnalysis struct {
	ExpectedDuration time.Duration `json:"expected_duration"`
	CriticalWindow   time.Duration `json:"critical_window"`
	OptimalTiming    string        `json:"optimal_timing"`
	Dependencies     []string      `json:"dependencies"`
	Milestones       []Milestone   `json:"milestones"`
}

// MonitoringPoint suggests monitoring points
type MonitoringPoint struct {
	Metric    string        `json:"metric"`
	Threshold interface{}   `json:"threshold"`
	Duration  time.Duration `json:"duration"`
	Rationale string        `json:"rationale"`
}

// Bottleneck represents system bottlenecks
type Bottleneck struct {
	Resource    string  `json:"resource"`
	Utilization float64 `json:"utilization"`
	Impact      string  `json:"impact"`
	Resolution  string  `json:"resolution"`
}

// Milestone represents timeline milestones
type Milestone struct {
	Name        string        `json:"name"`
	Time        time.Duration `json:"time"`
	Description string        `json:"description"`
	Critical    bool          `json:"critical"`
}

// ProcessingMetadata contains metadata about AI processing
type ProcessingMetadata struct {
	ProcessingTime      time.Duration `json:"processing_time"`
	AIModelUsed         string        `json:"ai_model_used"`
	ProcessingSteps     []string      `json:"processing_steps"`
	ConfidenceThreshold float64       `json:"confidence_threshold"`
	ValidationsPassed   int           `json:"validations_passed"`
	ValidationsFailed   int           `json:"validations_failed"`
	EnhancementsApplied []string      `json:"enhancements_applied"`
	ProcessingErrors    []string      `json:"processing_errors"`
}

// AIResponseProcessorConfig holds configuration for the AI response processor
type AIResponseProcessorConfig struct {
	EnableAdvancedValidation    bool          `yaml:"enable_advanced_validation" default:"true"`
	EnableReasoningAnalysis     bool          `yaml:"enable_reasoning_analysis" default:"true"`
	EnableConfidenceCalibration bool          `yaml:"enable_confidence_calibration" default:"true"`
	EnableContextualEnhancement bool          `yaml:"enable_contextual_enhancement" default:"true"`
	ConfidenceThreshold         float64       `yaml:"confidence_threshold" default:"0.75"`
	ValidationTimeout           time.Duration `yaml:"validation_timeout" default:"10s"`
	MaxProcessingTime           time.Duration `yaml:"max_processing_time" default:"30s"`
	EnableDetailedLogging       bool          `yaml:"enable_detailed_logging" default:"false"`
}

// DefaultAIResponseProcessor implements AIResponseProcessor using an AI client
type DefaultAIResponseProcessor struct {
	aiClient        Client // Uses existing SLM client for AI analysis
	config          *AIResponseProcessorConfig
	validationRules []ValidationRule
	knowledgeBase   KnowledgeBase
	healthy         bool
}

// ValidationRule represents a validation rule for AI analysis
type ValidationRule struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"` // "action", "parameter", "confidence", "reasoning"
	Severity   string   `json:"severity"`
	Pattern    string   `json:"pattern"`
	Conditions []string `json:"conditions"`
	Message    string   `json:"message"`
	Suggestion string   `json:"suggestion"`
}

// KnowledgeBase provides domain knowledge for AI analysis
type KnowledgeBase interface {
	GetActionRisks(action string) *RiskAssessment
	GetHistoricalPatterns(alert types.Alert) []HistoricalPattern
	GetValidationRules() []ValidationRule
	GetSystemState(ctx context.Context) (*SystemStateAnalysis, error)
}

// NewDefaultAIResponseProcessor creates a new AI-powered response processor
func NewDefaultAIResponseProcessor(
	aiClient Client,
	knowledgeBase KnowledgeBase,
	config *AIResponseProcessorConfig,
) *DefaultAIResponseProcessor {
	if config == nil {
		config = &AIResponseProcessorConfig{
			EnableAdvancedValidation:    true,
			EnableReasoningAnalysis:     true,
			EnableConfidenceCalibration: true,
			EnableContextualEnhancement: true,
			ConfidenceThreshold:         0.75,
			ValidationTimeout:           10 * time.Second,
			MaxProcessingTime:           30 * time.Second,
			EnableDetailedLogging:       false,
		}
	}

	return &DefaultAIResponseProcessor{
		aiClient:        aiClient,
		config:          config,
		validationRules: loadDefaultValidationRules(),
		knowledgeBase:   knowledgeBase,
		healthy:         true,
	}
}

// IsHealthy returns the health status of the AI response processor
func (p *DefaultAIResponseProcessor) IsHealthy() bool {
	return p.healthy && p.aiClient != nil && p.aiClient.IsHealthy()
}

// loadDefaultValidationRules loads default validation rules
func loadDefaultValidationRules() []ValidationRule {
	return []ValidationRule{
		{
			ID:         "action_exists",
			Name:       "Action Exists",
			Type:       "action",
			Severity:   "error",
			Pattern:    "^[a-z_]+$",
			Message:    "Action must be a valid action type",
			Suggestion: "Use one of the supported action types",
		},
		{
			ID:         "confidence_range",
			Name:       "Confidence Range",
			Type:       "confidence",
			Severity:   "error",
			Message:    "Confidence must be between 0.0 and 1.0",
			Suggestion: "Adjust confidence to valid range",
		},
		{
			ID:         "reasoning_length",
			Name:       "Reasoning Length",
			Type:       "reasoning",
			Severity:   "warning",
			Message:    "Reasoning should be substantive and detailed",
			Suggestion: "Provide more detailed reasoning for the action choice",
		},
		{
			ID:         "parameters_required",
			Name:       "Required Parameters",
			Type:       "parameter",
			Severity:   "error",
			Message:    "Required parameters missing for action",
			Suggestion: "Include all required parameters for the chosen action",
		},
	}
}
