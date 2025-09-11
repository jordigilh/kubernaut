//go:build integration
// +build integration

package shared

import (
	"fmt"
	"strings"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// DecisionContext represents the context for making SLM decisions
type DecisionContext struct {
	Alert               types.Alert
	InfrastructureLevel bool
	ApplicationLevel    bool
	SecurityRelated     bool
	ResourceExhaustion  bool
	NetworkIssue        bool
	HistoricalFailures  int
}

// SophisticatedDecisionEngine provides enhanced decision making for the fake SLM client
type SophisticatedDecisionEngine struct {
	confidencePatterns map[string]float64
}

// NewSophisticatedDecisionEngine creates a new enhanced decision engine
func NewSophisticatedDecisionEngine() *SophisticatedDecisionEngine {
	return &SophisticatedDecisionEngine{
		confidencePatterns: make(map[string]float64),
	}
}

// SetConfidencePatterns sets the confidence patterns for the decision engine
func (sde *SophisticatedDecisionEngine) SetConfidencePatterns(patterns map[string]float64) {
	sde.confidencePatterns = patterns
}

// AnalyzeContext creates a decision context from an alert
func (sde *SophisticatedDecisionEngine) AnalyzeContext(alert types.Alert) *DecisionContext {
	ctx := &DecisionContext{
		Alert: alert,
	}

	// Analyze alert characteristics
	ctx.InfrastructureLevel = sde.isInfrastructureLevel(alert)
	ctx.ApplicationLevel = sde.isApplicationLevel(alert)
	ctx.SecurityRelated = sde.isSecurityRelated(alert)
	ctx.ResourceExhaustion = sde.isResourceExhaustion(alert)
	ctx.NetworkIssue = sde.isNetworkIssue(alert)
	ctx.HistoricalFailures = sde.estimateHistoricalFailures(alert)

	return ctx
}

// MakeDecision uses sophisticated logic to determine action and confidence
func (sde *SophisticatedDecisionEngine) MakeDecision(ctx *DecisionContext) (string, float64, string) {

	// Multi-factor decision tree
	if ctx.SecurityRelated {
		return sde.handleSecurityIncident(ctx)
	}

	if ctx.InfrastructureLevel && ctx.NetworkIssue {
		return sde.handleInfrastructureNetworkIssue(ctx)
	}

	if ctx.ResourceExhaustion {
		return sde.handleResourceExhaustion(ctx)
	}

	if ctx.InfrastructureLevel {
		return sde.handleInfrastructureLevel(ctx)
	}

	if ctx.ApplicationLevel {
		return sde.handleApplicationLevel(ctx)
	}

	// Fallback to severity-based decisions
	return sde.handleBySeverity(ctx)
}

// Infrastructure-level issue handlers
func (sde *SophisticatedDecisionEngine) handleInfrastructureNetworkIssue(ctx *DecisionContext) (string, float64, string) {
	// Node network issues should trigger node-level actions
	if strings.Contains(strings.ToLower(ctx.Alert.Description), "node") ||
		strings.Contains(strings.ToLower(ctx.Alert.Name), "node") {
		return "drain_node", 0.8, "Node-level network issues detected, draining node for maintenance"
	}

	if ctx.Alert.Severity == "critical" {
		return "escalate", 0.7, "Critical network infrastructure issue requires human intervention"
	}

	return "collect_diagnostics", 0.6, "Network issue detected, collecting diagnostics"
}

func (sde *SophisticatedDecisionEngine) handleInfrastructureLevel(ctx *DecisionContext) (string, float64, string) {
	if ctx.Alert.Severity == "critical" {
		return "escalate", 0.8, "Critical infrastructure issue requires immediate attention"
	}

	return "collect_diagnostics", 0.7, "Infrastructure monitoring alert, collecting system diagnostics"
}

// Application-level issue handlers
func (sde *SophisticatedDecisionEngine) handleApplicationLevel(ctx *DecisionContext) (string, float64, string) {
	// OOM and memory issues
	if ctx.Alert.Name == "OOMKilled" ||
		strings.Contains(ctx.Alert.Labels["reason"], "OOMKilled") ||
		strings.Contains(strings.ToLower(ctx.Alert.Description), "out of memory") {
		confidence := sde.getConfidence("OOMKilled", 0.85)
		return "increase_resources", confidence, "Out of memory condition detected, increasing resource allocation"
	}

	// Pod crash loops
	if ctx.Alert.Name == "PodCrashLooping" ||
		strings.Contains(strings.ToLower(ctx.Alert.Description), "crash") {
		confidence := sde.getConfidence("PodCrashLooping", 0.75)
		return "restart_pod", confidence, "Pod crash looping detected, attempting restart"
	}

	// High memory/CPU usage
	if strings.Contains(strings.ToLower(ctx.Alert.Name), "memory") ||
		strings.Contains(strings.ToLower(ctx.Alert.Name), "cpu") {
		if ctx.Alert.Severity == "critical" {
			confidence := sde.getConfidence("HighMemoryUsage", 0.8)
			return "increase_resources", confidence, "Critical resource usage detected"
		}
		confidence := sde.getConfidence("HighMemoryUsage", 0.6)
		return "collect_diagnostics", confidence, "Monitoring resource usage patterns"
	}

	return sde.handleBySeverity(ctx)
}

// Security incident handler
func (sde *SophisticatedDecisionEngine) handleSecurityIncident(ctx *DecisionContext) (string, float64, string) {
	confidence := sde.getConfidence("SecurityIncident", 0.9)

	if strings.Contains(strings.ToLower(ctx.Alert.Description), "malware") ||
		strings.Contains(strings.ToLower(ctx.Alert.Description), "breach") ||
		ctx.Alert.Name == "ActiveSecurityThreat" {
		return "quarantine_pod", confidence, "Active security threat detected, quarantining affected resources"
	}

	if ctx.Alert.Severity == "critical" {
		return "quarantine_pod", confidence, "Critical security alert, implementing containment measures"
	}

	return "collect_diagnostics", confidence * 0.8, "Security monitoring alert, collecting forensic data"
}

// Resource exhaustion handler
func (sde *SophisticatedDecisionEngine) handleResourceExhaustion(ctx *DecisionContext) (string, float64, string) {
	confidence := sde.getConfidence("ResourceExhaustion", 0.9)

	// Storage-specific exhaustion
	if strings.Contains(strings.ToLower(ctx.Alert.Name), "disk") ||
		strings.Contains(strings.ToLower(ctx.Alert.Name), "pvc") ||
		strings.Contains(strings.ToLower(ctx.Alert.Name), "storage") {
		if strings.Contains(strings.ToLower(ctx.Alert.Name), "pvc") {
			return "expand_pvc", confidence, "Storage exhaustion detected, expanding persistent volume"
		}
		return "expand_storage", confidence, "Disk space exhaustion detected, expanding storage"
	}

	// Memory exhaustion
	if strings.Contains(strings.ToLower(ctx.Alert.Name), "memory") {
		return "scale_and_increase_resources", confidence, "Memory exhaustion detected, scaling and increasing resources"
	}

	// Generic resource exhaustion
	return "scale_and_increase_resources", confidence, "Resource exhaustion detected, scaling resources"
}

// Severity-based fallback handler
func (sde *SophisticatedDecisionEngine) handleBySeverity(ctx *DecisionContext) (string, float64, string) {
	switch ctx.Alert.Severity {
	case "critical":
		confidence := sde.getConfidence("critical", 0.9)
		return "escalate", confidence, fmt.Sprintf("Critical alert for %s requires immediate escalation", ctx.Alert.Resource)

	case "warning":
		confidence := sde.getConfidence("warning", 0.6)
		return "collect_diagnostics", confidence, fmt.Sprintf("Warning level alert for %s, collecting diagnostics", ctx.Alert.Resource)

	default:
		confidence := sde.getConfidence("default", 0.5)
		return "notify_only", confidence, "Standard alert monitoring, no immediate action required"
	}
}

// Helper methods for context analysis
func (sde *SophisticatedDecisionEngine) isInfrastructureLevel(alert types.Alert) bool {
	infraKeywords := []string{"node", "cluster", "network", "dns", "load", "proxy", "gateway"}

	alertText := strings.ToLower(alert.Name + " " + alert.Description)

	for _, keyword := range infraKeywords {
		if strings.Contains(alertText, keyword) {
			return true
		}
	}

	// Check labels for infrastructure indicators
	if alert.Labels != nil {
		if _, exists := alert.Labels["node"]; exists {
			return true
		}
		if component, exists := alert.Labels["component"]; exists {
			infraComponents := []string{"kubelet", "kube-proxy", "etcd", "api-server"}
			for _, comp := range infraComponents {
				if strings.Contains(component, comp) {
					return true
				}
			}
		}
	}

	return false
}

func (sde *SophisticatedDecisionEngine) isApplicationLevel(alert types.Alert) bool {
	appKeywords := []string{"pod", "container", "deployment", "service", "application", "app"}

	alertText := strings.ToLower(alert.Name + " " + alert.Description)

	for _, keyword := range appKeywords {
		if strings.Contains(alertText, keyword) {
			return true
		}
	}

	// If not clearly infrastructure, assume application level
	return !sde.isInfrastructureLevel(alert)
}

func (sde *SophisticatedDecisionEngine) isSecurityRelated(alert types.Alert) bool {
	securityKeywords := []string{"security", "unauthorized", "breach", "malware", "intrusion", "attack", "threat"}

	alertText := strings.ToLower(alert.Name + " " + alert.Description)

	for _, keyword := range securityKeywords {
		if strings.Contains(alertText, keyword) {
			return true
		}
	}

	// Check labels for security indicators
	if alert.Labels != nil {
		if _, exists := alert.Labels["threat_type"]; exists {
			return true
		}
		if alertType, exists := alert.Labels["type"]; exists {
			if strings.Contains(strings.ToLower(alertType), "security") {
				return true
			}
		}
	}

	return false
}

func (sde *SophisticatedDecisionEngine) isResourceExhaustion(alert types.Alert) bool {
	exhaustionKeywords := []string{"exhaustion", "full", "limit", "quota", "capacity", "99%", "95%", "out of"}

	alertText := strings.ToLower(alert.Name + " " + alert.Description)

	for _, keyword := range exhaustionKeywords {
		if strings.Contains(alertText, keyword) {
			return true
		}
	}

	return false
}

func (sde *SophisticatedDecisionEngine) isNetworkIssue(alert types.Alert) bool {
	networkKeywords := []string{"network", "connection", "latency", "timeout", "unreachable", "dns"}

	alertText := strings.ToLower(alert.Name + " " + alert.Description)

	for _, keyword := range networkKeywords {
		if strings.Contains(alertText, keyword) {
			return true
		}
	}

	return false
}

func (sde *SophisticatedDecisionEngine) estimateHistoricalFailures(alert types.Alert) int {
	// Simple heuristic based on alert characteristics
	// In real implementation, this would query historical data

	if strings.Contains(strings.ToLower(alert.Description), "recurring") ||
		strings.Contains(strings.ToLower(alert.Description), "repeated") {
		return 3
	}

	return 0
}

// Helper to get confidence with pattern matching
func (sde *SophisticatedDecisionEngine) getConfidence(key string, defaultValue float64) float64 {
	if confidence, exists := sde.confidencePatterns[key]; exists {
		return confidence
	}

	// Pattern matching for partial keys
	for pattern, confidence := range sde.confidencePatterns {
		if strings.Contains(strings.ToLower(key), strings.ToLower(pattern)) {
			return confidence
		}
	}

	return defaultValue
}
