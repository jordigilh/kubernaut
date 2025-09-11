package vector

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/internal/actionhistory"
)

// VectorDatabase defines the interface for vector storage and similarity search
type VectorDatabase interface {
	// StoreActionPattern stores an action pattern as a vector
	StoreActionPattern(ctx context.Context, pattern *ActionPattern) error

	// FindSimilarPatterns finds patterns similar to the given one
	FindSimilarPatterns(ctx context.Context, pattern *ActionPattern, limit int, threshold float64) ([]*SimilarPattern, error)

	// UpdatePatternEffectiveness updates the effectiveness score of a stored pattern
	UpdatePatternEffectiveness(ctx context.Context, patternID string, effectiveness float64) error

	// SearchBySemantics performs semantic search for patterns
	SearchBySemantics(ctx context.Context, query string, limit int) ([]*ActionPattern, error)

	// DeletePattern removes a pattern from the vector database
	DeletePattern(ctx context.Context, patternID string) error

	// GetPatternAnalytics returns analytics about stored patterns
	GetPatternAnalytics(ctx context.Context) (*PatternAnalytics, error)

	// Health check
	IsHealthy(ctx context.Context) error
}

// ActionPattern represents an action pattern with its vector representation
type ActionPattern struct {
	ID                string                 `json:"id"`
	ActionType        string                 `json:"action_type"`
	AlertName         string                 `json:"alert_name"`
	AlertSeverity     string                 `json:"alert_severity"`
	Namespace         string                 `json:"namespace"`
	ResourceType      string                 `json:"resource_type"`
	ResourceName      string                 `json:"resource_name"`
	ActionParameters  map[string]interface{} `json:"action_parameters"`
	ContextLabels     map[string]string      `json:"context_labels"`
	PreConditions     map[string]interface{} `json:"pre_conditions"`
	PostConditions    map[string]interface{} `json:"post_conditions"`
	EffectivenessData *EffectivenessData     `json:"effectiveness_data"`
	Embedding         []float64              `json:"embedding"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	Metadata          map[string]interface{} `json:"metadata"`
}

// EffectivenessData represents the effectiveness metrics for a pattern
type EffectivenessData struct {
	Score                float64            `json:"score"`
	SuccessCount         int                `json:"success_count"`
	FailureCount         int                `json:"failure_count"`
	AverageExecutionTime time.Duration      `json:"average_execution_time"`
	SideEffectsCount     int                `json:"side_effects_count"`
	RecurrenceRate       float64            `json:"recurrence_rate"`
	CostImpact           *CostImpact        `json:"cost_impact,omitempty"`
	ContextualFactors    map[string]float64 `json:"contextual_factors"`
	LastAssessed         time.Time          `json:"last_assessed"`
}

// CostImpact represents the cost impact analysis of an action
type CostImpact struct {
	ResourceCostDelta   float64 `json:"resource_cost_delta"`   // Change in resource costs
	OperationalCost     float64 `json:"operational_cost"`      // Cost of executing the action
	SavingsPotential    float64 `json:"savings_potential"`     // Potential cost savings
	CostEfficiencyRatio float64 `json:"cost_efficiency_ratio"` // Effectiveness per dollar spent
}

// SimilarPattern represents a pattern found through similarity search
type SimilarPattern struct {
	Pattern    *ActionPattern `json:"pattern"`
	Similarity float64        `json:"similarity"`
	Rank       int            `json:"rank"`
}

// PatternAnalytics provides insights about stored patterns
type PatternAnalytics struct {
	TotalPatterns             int              `json:"total_patterns"`
	PatternsByActionType      map[string]int   `json:"patterns_by_action_type"`
	PatternsBySeverity        map[string]int   `json:"patterns_by_severity"`
	AverageEffectiveness      float64          `json:"average_effectiveness"`
	TopPerformingPatterns     []*ActionPattern `json:"top_performing_patterns"`
	RecentPatterns            []*ActionPattern `json:"recent_patterns"`
	EffectivenessDistribution map[string]int   `json:"effectiveness_distribution"`
	GeneratedAt               time.Time        `json:"generated_at"`
}

// PatternExtractor defines interface for extracting patterns from action traces
type PatternExtractor interface {
	// ExtractPattern creates an ActionPattern from a ResourceActionTrace
	ExtractPattern(ctx context.Context, trace *actionhistory.ResourceActionTrace) (*ActionPattern, error)

	// GenerateEmbedding creates vector embedding for a pattern
	GenerateEmbedding(ctx context.Context, pattern *ActionPattern) ([]float64, error)

	// ExtractFeatures extracts contextual features from the pattern
	ExtractFeatures(ctx context.Context, pattern *ActionPattern) (map[string]float64, error)

	// CalculateSimilarity computes similarity between two patterns
	CalculateSimilarity(pattern1, pattern2 *ActionPattern) float64
}

// SemanticAnalyzer provides semantic analysis capabilities
type SemanticAnalyzer interface {
	// AnalyzeActionSemantics analyzes the semantic meaning of an action
	AnalyzeActionSemantics(ctx context.Context, actionType string, parameters map[string]interface{}) (*SemanticAnalysis, error)

	// AnalyzeAlertSemantics analyzes the semantic meaning of an alert
	AnalyzeAlertSemantics(ctx context.Context, alertName, description string, labels map[string]string) (*SemanticAnalysis, error)

	// ComputeSemanticSimilarity computes semantic similarity between two analyses
	ComputeSemanticSimilarity(analysis1, analysis2 *SemanticAnalysis) float64

	// ExtractKeywords extracts important keywords from text
	ExtractKeywords(ctx context.Context, text string) ([]string, error)
}

// SemanticAnalysis represents the semantic analysis result
type SemanticAnalysis struct {
	Keywords   []string               `json:"keywords"`
	Concepts   []string               `json:"concepts"`
	Intent     string                 `json:"intent"`
	Confidence float64                `json:"confidence"`
	Category   string                 `json:"category"`
	Embedding  []float64              `json:"embedding"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// EmbeddingGenerator creates vector embeddings for different types of data
type EmbeddingGenerator interface {
	// GenerateTextEmbedding creates embedding from text
	GenerateTextEmbedding(ctx context.Context, text string) ([]float64, error)

	// GenerateActionEmbedding creates embedding from action data
	GenerateActionEmbedding(ctx context.Context, actionType string, parameters map[string]interface{}) ([]float64, error)

	// GenerateContextEmbedding creates embedding from context data
	GenerateContextEmbedding(ctx context.Context, labels map[string]string, metadata map[string]interface{}) ([]float64, error)

	// CombineEmbeddings combines multiple embeddings into one
	CombineEmbeddings(embeddings ...[]float64) []float64

	// GetEmbeddingDimension returns the dimension of generated embeddings
	GetEmbeddingDimension() int
}

// VectorSearchQuery represents a search query for patterns
type VectorSearchQuery struct {
	// Text-based search
	QueryText string `json:"query_text,omitempty"`

	// Vector-based search
	QueryVector []float64 `json:"query_vector,omitempty"`

	// Filters
	ActionTypes   []string               `json:"action_types,omitempty"`
	Severities    []string               `json:"severities,omitempty"`
	Namespaces    []string               `json:"namespaces,omitempty"`
	ResourceTypes []string               `json:"resource_types,omitempty"`
	DateRange     *DateRange             `json:"date_range,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`

	// Search parameters
	Limit               int     `json:"limit"`
	SimilarityThreshold float64 `json:"similarity_threshold"`
	IncludeMetadata     bool    `json:"include_metadata"`
}

// DateRange represents a time range for filtering
type DateRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// VectorSearchResult represents the result of a vector search
// Unified with new search result system for consistency
type VectorSearchResult = UnifiedSearchResultSet

// PatternSearchResult represents pattern-specific search results
type PatternSearchResult = UnifiedSearchResult
