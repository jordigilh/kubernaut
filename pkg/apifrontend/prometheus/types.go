package prometheus

import "time"

// Alert represents a Prometheus alert from /api/v1/alerts.
type Alert struct {
	Labels      map[string]string
	Annotations map[string]string
	State       string // firing, pending, inactive
	ActiveAt    time.Time
}

// RuleGroup represents a group of alerting/recording rules from /api/v1/rules.
type RuleGroup struct {
	Name  string
	File  string
	Rules []Rule
}

// Rule represents an individual alerting rule within a RuleGroup.
type Rule struct {
	Name        string
	Query       string
	Duration    float64
	Labels      map[string]string
	Annotations map[string]string
	State       string // inactive, pending, firing
	Type        string // alerting, recording
}

// QueryResult holds the result of an instant PromQL query.
type QueryResult struct {
	Samples []Sample
}

// Sample is a single time series sample from a query result.
type Sample struct {
	Metric    map[string]string
	Value     float64
	Timestamp time.Time
}
