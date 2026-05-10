package severity

import (
	"fmt"
	"strings"

	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
)

// Source identifies which triage tier determined the severity.
type Source string

// Triage source constants identify which pipeline tier produced the severity.
const (
	SourceFiringAlert   Source = "firing_alert"
	SourcePendingAlert  Source = "pending_alert"
	SourceRuleEval      Source = "rule_evaluation"
	SourceLLMRuleInform Source = "llm_rule_informed"
	SourceLLMTriage     Source = "llm_triage"
	SourceDefault       Source = "default_fallback"
)

// TriageInput holds the resource context for severity triage.
type TriageInput struct {
	Namespace   string
	Kind        string
	Name        string
	Description string
	Labels      map[string]string
}

// TriageResult holds the outcome of the severity triage pipeline.
type TriageResult struct {
	Severity  string
	Source    Source
	AlertName string
	RuleName  string
}

var severityRank = map[string]int{
	"critical": 5,
	"high":     4,
	"medium":   3,
	"low":      2,
	"info":     1,
}

var validSeverities = map[string]bool{
	"critical": true,
	"high":     true,
	"medium":   true,
	"low":      true,
	"info":     true,
}

// ValidateSeverity checks if the string is a valid canonical severity value.
func ValidateSeverity(s string) bool {
	return validSeverities[s]
}

// NormalizeSeverity lowercases and validates the severity string.
// Returns "medium" as default for invalid/empty input.
func NormalizeSeverity(s string) string {
	lower := strings.TrimSpace(strings.ToLower(s))
	if validSeverities[lower] {
		return lower
	}
	return "medium"
}

// CompareSeverity returns > 0 if a is higher severity than b, < 0 if lower, 0 if equal.
func CompareSeverity(a, b string) int {
	return severityRank[a] - severityRank[b]
}

// HighestSeverity returns the highest severity string from a slice.
// Returns empty string for empty input.
func HighestSeverity(severities []string) string {
	if len(severities) == 0 {
		return ""
	}
	best := severities[0]
	for _, s := range severities[1:] {
		if CompareSeverity(s, best) > 0 {
			best = s
		}
	}
	return best
}

// sensitiveKeys lists label keys that should not appear in LLM prompts.
var sensitiveKeys = map[string]bool{
	"password": true, "token": true, "secret": true,
	"key": true, "credential": true, "bearer": true,
}

// BuildTriagePrompt constructs the LLM prompt for severity triage.
// Filters sensitive labels from the input.
func BuildTriagePrompt(input TriageInput, rules interface{}) string {
	var sb strings.Builder
	sb.WriteString("Classify the severity of the following Kubernetes incident.\n\n")
	fmt.Fprintf(&sb, "Resource: %s/%s in namespace %s\n", input.Kind, input.Name, input.Namespace)
	fmt.Fprintf(&sb, "Description: %s\n\n", input.Description)

	sb.WriteString("Resource labels:\n")
	for k, v := range input.Labels {
		if sensitiveKeys[strings.ToLower(k)] {
			continue
		}
		fmt.Fprintf(&sb, "  %s: %s\n", k, v)
	}

	if ruleSlice, ok := rules.([]prom.Rule); ok && len(ruleSlice) > 0 {
		sb.WriteString("\nMatching alerting rules (could not evaluate due to insufficient data):\n")
		for _, r := range ruleSlice {
			fmt.Fprintf(&sb, "  - %s: %s (configured severity: %s)\n", r.Name, r.Query, r.Labels["severity"])
			if summary, exists := r.Annotations["summary"]; exists {
				fmt.Fprintf(&sb, "    Summary: %s\n", summary)
			}
		}
	}

	sb.WriteString("\nRespond with exactly one of: critical, high, medium, low, info\n")
	return sb.String()
}
