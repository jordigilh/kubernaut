package common

import (
	"context"
	"time"
)

// Core AI Analysis Interfaces - shared across all AI services
type AIAnalyzer interface {
	IsHealthy() bool
	GetMetadata() map[string]interface{}
}

type AIService interface {
	AIAnalyzer
	ProcessRequest(ctx context.Context, request *AIRequest) (*AIResponse, error)
}

// Core shared types for AI operations
type AIRequest struct {
	ID                string                 `json:"id"`
	Type              string                 `json:"type"`
	Prompt            string                 `json:"prompt"`
	Context           map[string]interface{} `json:"context"`
	Options           *AIOptions             `json:"options"`
	Timeout           time.Duration          `json:"timeout"`
	RequiredFields    []string               `json:"required_fields,omitempty"`
	ContextualFactors map[string]interface{} `json:"contextual_factors,omitempty"`
}

type AIResponse struct {
	ID              string                 `json:"id"`
	RequestID       string                 `json:"request_id"`
	Content         string                 `json:"content"`
	Confidence      float64                `json:"confidence"`
	Metadata        map[string]interface{} `json:"metadata"`
	ProcessingTime  time.Duration          `json:"processing_time"`
	UsedTools       []string               `json:"used_tools,omitempty"`
	Recommendations []Recommendation       `json:"recommendations,omitempty"`
}

type AIOptions struct {
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Temperature    float64           `json:"temperature,omitempty"`
	Model          string            `json:"model,omitempty"`
	CustomContext  map[string]string `json:"custom_context,omitempty"`
	IncludeTools   []string          `json:"include_tools,omitempty"`
	ResponseFormat string            `json:"response_format,omitempty"`
	MaxRetries     int               `json:"max_retries,omitempty"`
}

// Shared recommendation structure
type Recommendation struct {
	ID          string            `json:"id"`
	Action      string            `json:"action"`
	Description string            `json:"description"`
	Priority    string            `json:"priority"`
	Risk        string            `json:"risk"`
	Parameters  map[string]string `json:"parameters"`
	Confidence  float64           `json:"confidence"`
	Reasoning   string            `json:"reasoning,omitempty"`
	Impact      string            `json:"impact,omitempty"`
}

// Common Alert structure used across AI services
type Alert struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Summary     string            `json:"summary"`
	Description string            `json:"description"`
	Severity    string            `json:"severity"`
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      *time.Time        `json:"ends_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Common analysis result structures
type AnalysisResult struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type"`
	Summary        string                 `json:"summary"`
	Confidence     float64                `json:"confidence"`
	Evidence       []Evidence             `json:"evidence"`
	Metadata       map[string]interface{} `json:"metadata"`
	GeneratedAt    time.Time              `json:"generated_at"`
	ProcessingTime time.Duration          `json:"processing_time"`
}

type Evidence struct {
	Type        string      `json:"type"`
	Source      string      `json:"source"`
	Value       interface{} `json:"value"`
	Confidence  float64     `json:"confidence"`
	Description string      `json:"description"`
	Timestamp   time.Time   `json:"timestamp"`
}

// Common configuration structures
type ServiceConfig struct {
	MaxAnalysisTime     time.Duration `yaml:"max_analysis_time" default:"30s"`
	ConfidenceThreshold float64       `yaml:"confidence_threshold" default:"0.7"`
	EnableDetailedLogs  bool          `yaml:"enable_detailed_logs" default:"false"`
	MaxRetries          int           `yaml:"max_retries" default:"3"`
	RetryDelay          time.Duration `yaml:"retry_delay" default:"1s"`
}

// Time-based analysis structures
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type TimeWindow struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Value  float64   `json:"value"`
	Labels []string  `json:"labels,omitempty"`
}

// Statistical helpers
type StatisticalRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type ConfidenceInterval struct {
	Lower      float64 `json:"lower"`
	Upper      float64 `json:"upper"`
	Confidence float64 `json:"confidence"`
}

// Error handling
type AIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Component string `json:"component"`
	Operation string `json:"operation"`
	Retryable bool   `json:"retryable"`
	Cause     error  `json:"-"`
}

func (e *AIError) Error() string {
	return e.Message
}

func (e *AIError) Unwrap() error {
	return e.Cause
}
