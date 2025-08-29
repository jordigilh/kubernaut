package actionhistory

import (
	"time"
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// Core domain types for action history system

// ResourceReference represents a Kubernetes resource
type ResourceReference struct {
	ID          int64     `json:"id,omitempty" db:"id"`
	ResourceUID string    `json:"resource_uid" db:"resource_uid"`
	APIVersion  string    `json:"api_version" db:"api_version"`
	Kind        string    `json:"kind" db:"kind"`
	Name        string    `json:"name" db:"name"`
	Namespace   string    `json:"namespace" db:"namespace"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" db:"deleted_at"`
	LastSeen    time.Time `json:"last_seen" db:"last_seen"`
}

// ActionHistory represents the configuration and status for action tracking
type ActionHistory struct {
	ID                     int64      `json:"id" db:"id"`
	ResourceID             int64      `json:"resource_id" db:"resource_id"`
	MaxActions             int        `json:"max_actions" db:"max_actions"`
	MaxAgeDays             int        `json:"max_age_days" db:"max_age_days"`
	CompactionStrategy     string     `json:"compaction_strategy" db:"compaction_strategy"`
	OscillationWindowMins  int        `json:"oscillation_window_minutes" db:"oscillation_window_minutes"`
	EffectivenessThreshold float64    `json:"effectiveness_threshold" db:"effectiveness_threshold"`
	PatternMinOccurrences  int        `json:"pattern_min_occurrences" db:"pattern_min_occurrences"`
	TotalActions           int        `json:"total_actions" db:"total_actions"`
	LastActionAt           *time.Time `json:"last_action_at,omitempty" db:"last_action_at"`
	LastAnalysisAt         *time.Time `json:"last_analysis_at,omitempty" db:"last_analysis_at"`
	NextAnalysisAt         *time.Time `json:"next_analysis_at,omitempty" db:"next_analysis_at"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
}

// AlertContext represents the alert that triggered an action
type AlertContext struct {
	Name        string            `json:"name"`
	Severity    string            `json:"severity"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	FiringTime  time.Time         `json:"firing_time"`
}

// Value implements driver.Valuer for AlertContext
func (ac AlertContext) Value() (driver.Value, error) {
	return json.Marshal(ac)
}

// Scan implements sql.Scanner for AlertContext
func (ac *AlertContext) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into AlertContext", value)
	}
	
	return json.Unmarshal(bytes, ac)
}

// ActionAlternative represents an alternative action suggested by the AI
type ActionAlternative struct {
	ActionType string  `json:"action_type"`
	Confidence float64 `json:"confidence"`
	Reasoning  string  `json:"reasoning"`
}

// ActionAlternatives is a slice of ActionAlternative with JSON marshaling
type ActionAlternatives []ActionAlternative

// Value implements driver.Valuer for ActionAlternatives
func (aa ActionAlternatives) Value() (driver.Value, error) {
	if len(aa) == 0 {
		return nil, nil
	}
	return json.Marshal(aa)
}

// Scan implements sql.Scanner for ActionAlternatives
func (aa *ActionAlternatives) Scan(value interface{}) error {
	if value == nil {
		*aa = nil
		return nil
	}
	
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into ActionAlternatives", value)
	}
	
	return json.Unmarshal(bytes, aa)
}

// FollowUpAction represents a follow-up action relationship
type FollowUpAction struct {
	ActionID  string `json:"action_id"`
	Relation  string `json:"relation"` // correction, enhancement, rollback, escalation
	Timestamp time.Time `json:"timestamp"`
}

// FollowUpActions is a slice of FollowUpAction with JSON marshaling
type FollowUpActions []FollowUpAction

// Value implements driver.Valuer for FollowUpActions
func (fua FollowUpActions) Value() (driver.Value, error) {
	if len(fua) == 0 {
		return nil, nil
	}
	return json.Marshal(fua)
}

// Scan implements sql.Scanner for FollowUpActions
func (fua *FollowUpActions) Scan(value interface{}) error {
	if value == nil {
		*fua = nil
		return nil
	}
	
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into FollowUpActions", value)
	}
	
	return json.Unmarshal(bytes, fua)
}

// JSONMap represents a generic JSON object
type JSONMap map[string]interface{}

// StringMapToJSONMap converts a map[string]string to JSONMap
func StringMapToJSONMap(m map[string]string) JSONMap {
	if m == nil {
		return nil
	}
	result := make(JSONMap)
	for k, v := range m {
		result[k] = v
	}
	return result
}

// Value implements driver.Valuer for JSONMap
func (jm JSONMap) Value() (driver.Value, error) {
	if len(jm) == 0 {
		return nil, nil
	}
	return json.Marshal(jm)
}

// Scan implements sql.Scanner for JSONMap
func (jm *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*jm = nil
		return nil
	}
	
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into JSONMap", value)
	}
	
	return json.Unmarshal(bytes, jm)
}

// EffectivenessCriteria represents the criteria used to assess action effectiveness
type EffectivenessCriteria struct {
	AlertResolved          bool `json:"alert_resolved"`
	TargetMetricImproved   bool `json:"target_metric_improved"`
	NoNewAlertsGenerated   bool `json:"no_new_alerts_generated"`
	ResourceStabilized     bool `json:"resource_stabilized"`
	SideEffectsMinimal     bool `json:"side_effects_minimal"`
}

// Value implements driver.Valuer for EffectivenessCriteria
func (ec EffectivenessCriteria) Value() (driver.Value, error) {
	return json.Marshal(ec)
}

// Scan implements sql.Scanner for EffectivenessCriteria
func (ec *EffectivenessCriteria) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into EffectivenessCriteria", value)
	}
	
	return json.Unmarshal(bytes, ec)
}

// ResourceActionTrace represents a single action record
type ResourceActionTrace struct {
	ID                         int64                  `json:"id" db:"id"`
	ActionHistoryID           int64                  `json:"action_history_id" db:"action_history_id"`
	ActionID                  string                 `json:"action_id" db:"action_id"`
	CorrelationID             *string                `json:"correlation_id,omitempty" db:"correlation_id"`
	ActionTimestamp           time.Time              `json:"action_timestamp" db:"action_timestamp"`
	ExecutionStartTime        *time.Time             `json:"execution_start_time,omitempty" db:"execution_start_time"`
	ExecutionEndTime          *time.Time             `json:"execution_end_time,omitempty" db:"execution_end_time"`
	ExecutionDurationMs       *int                   `json:"execution_duration_ms,omitempty" db:"execution_duration_ms"`
	
	// Alert context
	AlertName        string   `json:"alert_name" db:"alert_name"`
	AlertSeverity    string   `json:"alert_severity" db:"alert_severity"`
	AlertLabels      JSONMap  `json:"alert_labels" db:"alert_labels"`
	AlertAnnotations JSONMap  `json:"alert_annotations" db:"alert_annotations"`
	AlertFiringTime  *time.Time `json:"alert_firing_time,omitempty" db:"alert_firing_time"`
	
	// AI model information
	ModelUsed           string              `json:"model_used" db:"model_used"`
	RoutingTier         *string             `json:"routing_tier,omitempty" db:"routing_tier"`
	ModelConfidence     float64             `json:"model_confidence" db:"model_confidence"`
	ModelReasoning      *string             `json:"model_reasoning,omitempty" db:"model_reasoning"`
	AlternativeActions  ActionAlternatives  `json:"alternative_actions" db:"alternative_actions"`
	
	// Action details
	ActionType       string  `json:"action_type" db:"action_type"`
	ActionParameters JSONMap `json:"action_parameters" db:"action_parameters"`
	
	// Resource state
	ResourceStateBefore JSONMap `json:"resource_state_before" db:"resource_state_before"`
	ResourceStateAfter  JSONMap `json:"resource_state_after" db:"resource_state_after"`
	
	// Execution tracking
	ExecutionStatus     string  `json:"execution_status" db:"execution_status"`
	ExecutionError      *string `json:"execution_error,omitempty" db:"execution_error"`
	KubernetesOperations JSONMap `json:"kubernetes_operations" db:"kubernetes_operations"`
	
	// Effectiveness assessment
	EffectivenessScore          *float64               `json:"effectiveness_score,omitempty" db:"effectiveness_score"`
	EffectivenessCriteria       *EffectivenessCriteria `json:"effectiveness_criteria,omitempty" db:"effectiveness_criteria"`
	EffectivenessAssessedAt     *time.Time             `json:"effectiveness_assessed_at,omitempty" db:"effectiveness_assessed_at"`
	EffectivenessAssessmentMethod *string              `json:"effectiveness_assessment_method,omitempty" db:"effectiveness_assessment_method"`
	EffectivenessNotes          *string                `json:"effectiveness_notes,omitempty" db:"effectiveness_notes"`
	
	// Follow-up tracking
	FollowUpActions FollowUpActions `json:"follow_up_actions" db:"follow_up_actions"`
	
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// OscillationPattern represents a detected oscillation pattern
type OscillationPattern struct {
	ID                    int64             `json:"id" db:"id"`
	PatternType          string            `json:"pattern_type" db:"pattern_type"`
	PatternName          string            `json:"pattern_name" db:"pattern_name"`
	Description          *string           `json:"description,omitempty" db:"description"`
	MinOccurrences       int               `json:"min_occurrences" db:"min_occurrences"`
	TimeWindowMinutes    int               `json:"time_window_minutes" db:"time_window_minutes"`
	ActionSequence       JSONMap           `json:"action_sequence" db:"action_sequence"`
	ThresholdConfig      JSONMap           `json:"threshold_config" db:"threshold_config"`
	ResourceTypes        []string          `json:"resource_types" db:"resource_types"`
	Namespaces           []string          `json:"namespaces" db:"namespaces"`
	LabelSelectors       JSONMap           `json:"label_selectors" db:"label_selectors"`
	PreventionStrategy   string            `json:"prevention_strategy" db:"prevention_strategy"`
	PreventionParameters JSONMap           `json:"prevention_parameters" db:"prevention_parameters"`
	AlertingEnabled      bool              `json:"alerting_enabled" db:"alerting_enabled"`
	AlertSeverity        string            `json:"alert_severity" db:"alert_severity"`
	AlertChannels        []string          `json:"alert_channels" db:"alert_channels"`
	TotalDetections      int               `json:"total_detections" db:"total_detections"`
	PreventionSuccessRate *float64         `json:"prevention_success_rate,omitempty" db:"prevention_success_rate"`
	FalsePositiveRate    *float64          `json:"false_positive_rate,omitempty" db:"false_positive_rate"`
	LastDetectionAt      *time.Time        `json:"last_detection_at,omitempty" db:"last_detection_at"`
	Active               bool              `json:"active" db:"active"`
	CreatedAt            time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at" db:"updated_at"`
}

// OscillationDetection represents an instance of pattern detection
type OscillationDetection struct {
	ID                   int64      `json:"id" db:"id"`
	PatternID           int64      `json:"pattern_id" db:"pattern_id"`
	ResourceID          int64      `json:"resource_id" db:"resource_id"`
	DetectedAt          time.Time  `json:"detected_at" db:"detected_at"`
	Confidence          float64    `json:"confidence" db:"confidence"`
	ActionCount         int        `json:"action_count" db:"action_count"`
	TimeSpanMinutes     int        `json:"time_span_minutes" db:"time_span_minutes"`
	MatchingActions     []int64    `json:"matching_actions" db:"matching_actions"`
	PatternEvidence     JSONMap    `json:"pattern_evidence" db:"pattern_evidence"`
	PreventionApplied   bool       `json:"prevention_applied" db:"prevention_applied"`
	PreventionAction    *string    `json:"prevention_action,omitempty" db:"prevention_action"`
	PreventionDetails   JSONMap    `json:"prevention_details" db:"prevention_details"`
	PreventionSuccessful *bool     `json:"prevention_successful,omitempty" db:"prevention_successful"`
	Resolved            bool       `json:"resolved" db:"resolved"`
	ResolvedAt          *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
	ResolutionMethod    *string    `json:"resolution_method,omitempty" db:"resolution_method"`
	ResolutionNotes     *string    `json:"resolution_notes,omitempty" db:"resolution_notes"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
}

// ActionRecord represents a new action to be stored
type ActionRecord struct {
	ResourceReference   ResourceReference
	ActionID           string
	CorrelationID      *string
	Timestamp          time.Time
	
	Alert AlertContext
	
	ModelUsed          string
	RoutingTier        *string
	Confidence         float64
	Reasoning          *string
	AlternativeActions ActionAlternatives
	
	ActionType       string
	Parameters       map[string]interface{}
	
	ResourceStateBefore map[string]interface{}
	ResourceStateAfter  map[string]interface{}
}

// TimeRange represents a time range for queries
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Contains checks if a time is within the range
func (tr TimeRange) Contains(t time.Time) bool {
	return t.After(tr.Start) && t.Before(tr.End)
}

// ActionQuery represents query parameters for action history
type ActionQuery struct {
	Namespace    string
	ResourceKind string
	ResourceName string
	ActionType   string
	ModelUsed    string
	TimeRange    TimeRange
	Limit        int
	Offset       int
}

// EffectivenessScope represents the scope for effectiveness metrics
type EffectivenessScope struct {
	Type  string // global, namespace, resource-type, alert-type, model
	Value string // specific value for the scope
}

// OscillationSeverity represents the severity level of an oscillation
type OscillationSeverity string

const (
	SeverityNone     OscillationSeverity = "none"
	SeverityLow      OscillationSeverity = "low"
	SeverityMedium   OscillationSeverity = "medium"
	SeverityHigh     OscillationSeverity = "high"
	SeverityCritical OscillationSeverity = "critical"
)

// PreventionAction represents the type of prevention action to take
type PreventionAction string

const (
	PreventionNone          PreventionAction = "none"
	PreventionBlock         PreventionAction = "block"
	PreventionEscalate      PreventionAction = "escalate"
	PreventionAlternative   PreventionAction = "alternative"
	PreventionCoolingPeriod PreventionAction = "cooling_period"
)