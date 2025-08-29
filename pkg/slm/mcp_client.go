package slm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/actionhistory"
	"github.com/jordigilh/prometheus-alerts-slm/internal/mcp"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
)

// MCPContext contains contextual information from MCP queries
type MCPContext struct {
	ActionHistory        []ActionSummary       `json:"action_history"`
	OscillationAnalysis  *OscillationSummary   `json:"oscillation_analysis,omitempty"`
	EffectivenessMetrics *EffectivenessMetrics `json:"effectiveness_metrics,omitempty"`
	SafetyAssessment     *SafetyAssessment     `json:"safety_assessment,omitempty"`
	ClusterState         *ClusterState         `json:"cluster_state,omitempty"`
}

// ActionSummary provides a summary of previous actions
type ActionSummary struct {
	ActionType      string    `json:"action_type"`
	Timestamp       time.Time `json:"timestamp"`
	Confidence      float64   `json:"confidence"`
	ExecutionStatus string    `json:"execution_status"`
	Effectiveness   *float64  `json:"effectiveness,omitempty"`
	AlertName       string    `json:"alert_name"`
	AlertSeverity   string    `json:"alert_severity"`
}

// OscillationSummary provides oscillation analysis results
type OscillationSummary struct {
	Severity            string     `json:"severity"`
	Confidence          float64    `json:"confidence"`
	ScaleChanges        int        `json:"scale_changes,omitempty"`
	ThrashingDetected   bool       `json:"thrashing_detected"`
	LastOscillationTime *time.Time `json:"last_oscillation_time,omitempty"`
	RiskLevel           string     `json:"risk_level"`
}

// EffectivenessMetrics provides action effectiveness data
type EffectivenessMetrics struct {
	ActionType           string  `json:"action_type"`
	AverageEffectiveness float64 `json:"average_effectiveness"`
	SuccessRate          float64 `json:"success_rate"`
	TotalAttempts        int     `json:"total_attempts"`
	RecommendedAction    string  `json:"recommended_action,omitempty"`
}

// SafetyAssessment provides safety analysis for proposed actions
type SafetyAssessment struct {
	IsSafe             bool     `json:"is_safe"`
	RiskFactors        []string `json:"risk_factors,omitempty"`
	RecommendedAction  string   `json:"recommended_action,omitempty"`
	ConfidenceLevel    float64  `json:"confidence_level"`
	AlternativeActions []string `json:"alternative_actions,omitempty"`
}

// ClusterState provides current Kubernetes cluster information
type ClusterState struct {
	NodeCapacity        *ResourceMetrics `json:"node_capacity,omitempty"`
	PodStatus           *PodStatusInfo   `json:"pod_status,omitempty"`
	RecentEvents        []EventSummary   `json:"recent_events,omitempty"`
	ResourceConstraints []string         `json:"resource_constraints,omitempty"`
}

// ResourceMetrics provides resource usage information
type ResourceMetrics struct {
	CPUUsage       float64 `json:"cpu_usage"`
	MemoryUsage    float64 `json:"memory_usage"`
	StorageUsage   float64 `json:"storage_usage"`
	AvailableNodes int     `json:"available_nodes"`
}

// PodStatusInfo provides pod status information
type PodStatusInfo struct {
	Running   int `json:"running"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
	Succeeded int `json:"succeeded"`
}

// EventSummary provides recent Kubernetes events
type EventSummary struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// MCPClientConfig configures the MCP client
type MCPClientConfig struct {
	ActionHistoryServerEndpoint string        `json:"action_history_server_endpoint"`
	KubernetesServerEndpoint    string        `json:"kubernetes_server_endpoint"`
	Timeout                     time.Duration `json:"timeout"`
	MaxRetries                  int           `json:"max_retries"`
}

// MCPClient provides access to MCP servers for contextual information
type MCPClient interface {
	GetActionContext(ctx context.Context, alert types.Alert) (*MCPContext, error)
	CheckActionSafety(ctx context.Context, alert types.Alert, proposedAction string) (*SafetyAssessment, error)
}

// K8sMCPServer interface for external Kubernetes MCP server
type K8sMCPServer interface {
	HandleToolCall(ctx context.Context, request mcp.MCPToolRequest) (mcp.MCPToolResponse, error)
}

// mcpClient implements the MCPClient interface
type mcpClient struct {
	config              MCPClientConfig
	actionHistoryServer *mcp.ActionHistoryMCPServer
	k8sServer           K8sMCPServer
	logger              *logrus.Logger
}

// NewMCPClient creates a new MCP client with action history server
func NewMCPClient(config MCPClientConfig, actionHistoryServer *mcp.ActionHistoryMCPServer, logger *logrus.Logger) MCPClient {
	return &mcpClient{
		config:              config,
		actionHistoryServer: actionHistoryServer,
		k8sServer:           nil, // Will be set later if needed
		logger:              logger,
	}
}

// NewMCPClientWithK8sServer creates a new MCP client with both action history and K8s servers
func NewMCPClientWithK8sServer(config MCPClientConfig, actionHistoryServer *mcp.ActionHistoryMCPServer, k8sServer K8sMCPServer, logger *logrus.Logger) MCPClient {
	return &mcpClient{
		config:              config,
		actionHistoryServer: actionHistoryServer,
		k8sServer:           k8sServer,
		logger:              logger,
	}
}

// SetK8sServer sets the Kubernetes MCP server after client creation
func (c *mcpClient) SetK8sServer(k8sServer K8sMCPServer) {
	c.k8sServer = k8sServer
}

// GetActionContext retrieves contextual information from MCP servers
func (c *mcpClient) GetActionContext(ctx context.Context, alert types.Alert) (*MCPContext, error) {
	resourceRef := actionhistory.ResourceReference{
		Namespace: alert.Namespace,
		Kind:      "Deployment", // Default for alerts, could be derived from alert labels
		Name:      alert.Resource,
	}

	mcpContext := &MCPContext{}

	// Get action history
	if actionHistory, err := c.getActionHistory(ctx, resourceRef); err == nil {
		mcpContext.ActionHistory = actionHistory
	} else {
		c.logger.WithError(err).Warn("Failed to get action history from MCP")
	}

	// Get oscillation analysis
	if oscillationAnalysis, err := c.getOscillationAnalysis(ctx, resourceRef); err == nil {
		mcpContext.OscillationAnalysis = oscillationAnalysis
	} else {
		c.logger.WithError(err).Warn("Failed to get oscillation analysis from MCP")
	}

	// Get effectiveness metrics
	if effectivenessMetrics, err := c.getEffectivenessMetrics(ctx, resourceRef); err == nil {
		mcpContext.EffectivenessMetrics = effectivenessMetrics
	} else {
		c.logger.WithError(err).Warn("Failed to get effectiveness metrics from MCP")
	}

	// Get cluster state (would require K8s MCP server implementation)
	if clusterState, err := c.getClusterState(ctx, resourceRef); err == nil {
		mcpContext.ClusterState = clusterState
	} else {
		c.logger.WithError(err).Debug("Failed to get cluster state from MCP (may not be implemented)")
	}

	return mcpContext, nil
}

// CheckActionSafety checks if a proposed action is safe
func (c *mcpClient) CheckActionSafety(ctx context.Context, alert types.Alert, proposedAction string) (*SafetyAssessment, error) {
	resourceRef := actionhistory.ResourceReference{
		Namespace: alert.Namespace,
		Kind:      "Deployment",
		Name:      alert.Resource,
	}

	// Build tool request
	request := mcp.MCPToolRequest{
		Method: "tools/call",
		Params: mcp.MCPToolParams{
			Name: "check_action_safety",
			Arguments: map[string]interface{}{
				"namespace":  resourceRef.Namespace,
				"kind":       resourceRef.Kind,
				"name":       resourceRef.Name,
				"actionType": proposedAction,
				"parameters": "{}",
			},
		},
	}

	response, err := c.actionHistoryServer.HandleToolCall(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to check action safety: %w", err)
	}

	// Parse the response content
	safety := &SafetyAssessment{}
	if len(response.Content) > 0 {
		// Simple parsing - in a real implementation you'd parse the structured response
		content := response.Content[0].Text
		if len(content) > 0 {
			// For now, return a basic assessment
			safety.IsSafe = true
			safety.ConfidenceLevel = 0.8
			safety.RecommendedAction = proposedAction
		}
	}

	return safety, nil
}

// getActionHistory retrieves action history from MCP
func (c *mcpClient) getActionHistory(ctx context.Context, resourceRef actionhistory.ResourceReference) ([]ActionSummary, error) {
	request := mcp.MCPToolRequest{
		Method: "tools/call",
		Params: mcp.MCPToolParams{
			Name: "get_action_history",
			Arguments: map[string]interface{}{
				"namespace": resourceRef.Namespace,
				"kind":      resourceRef.Kind,
				"name":      resourceRef.Name,
				"limit":     "10",
				"timeRange": "24h",
			},
		},
	}

	response, err := c.actionHistoryServer.HandleToolCall(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to get action history: %w", err)
	}

	// Parse structured JSON response from MCP server
	var summaries []ActionSummary
	if len(response.Content) > 0 && response.Content[0].Type == "application/json" {
		// Unmarshal the structured JSON data
		var actionHistoryResponse mcp.ActionHistoryResponse
		if err := c.unmarshalMCPData(response.Content[0].Data, &actionHistoryResponse); err != nil {
			return nil, fmt.Errorf("failed to parse action history response: %w", err)
		}

		c.logger.Debugf("MCP action history response: %d actions found", actionHistoryResponse.TotalActions)

		// Convert MCP action summaries to SLM action summaries
		summaries = make([]ActionSummary, len(actionHistoryResponse.Actions))
		for i, action := range actionHistoryResponse.Actions {
			summaries[i] = ActionSummary{
				ActionType:      action.ActionType,
				Timestamp:       action.Timestamp,
				Confidence:      action.Confidence,
				ExecutionStatus: action.ExecutionStatus,
				Effectiveness:   action.Effectiveness,
				AlertName:       action.AlertName,
				AlertSeverity:   action.AlertSeverity,
			}
		}
	}

	return summaries, nil
}

// getOscillationAnalysis retrieves oscillation analysis from MCP
func (c *mcpClient) getOscillationAnalysis(ctx context.Context, resourceRef actionhistory.ResourceReference) (*OscillationSummary, error) {
	request := mcp.MCPToolRequest{
		Method: "tools/call",
		Params: mcp.MCPToolParams{
			Name: "analyze_oscillation_patterns",
			Arguments: map[string]interface{}{
				"namespace":     resourceRef.Namespace,
				"kind":          resourceRef.Kind,
				"name":          resourceRef.Name,
				"windowMinutes": "120",
			},
		},
	}

	response, err := c.actionHistoryServer.HandleToolCall(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze oscillation patterns: %w", err)
	}

	// Parse structured JSON response from MCP server
	summary := &OscillationSummary{
		Severity:   "none",
		Confidence: 0.0,
		RiskLevel:  "low",
	}

	if len(response.Content) > 0 && response.Content[0].Type == "application/json" {
		// Unmarshal the structured JSON data
		var oscillationResponse mcp.OscillationAnalysisResponse
		if err := c.unmarshalMCPData(response.Content[0].Data, &oscillationResponse); err != nil {
			return nil, fmt.Errorf("failed to parse oscillation analysis response: %w", err)
		}

		c.logger.Debugf("MCP oscillation analysis: severity=%s, confidence=%.3f",
			oscillationResponse.OverallSeverity, oscillationResponse.Confidence)

		// Convert structured response to summary
		summary.Severity = oscillationResponse.OverallSeverity
		summary.Confidence = oscillationResponse.Confidence

		// Check for oscillation patterns
		if oscillationResponse.ScaleOscillation != nil {
			summary.ThrashingDetected = true
			summary.ScaleChanges = oscillationResponse.ScaleOscillation.DirectionChanges
			if oscillationResponse.ScaleOscillation.Severity != "none" {
				summary.Severity = oscillationResponse.ScaleOscillation.Severity
			}
		}

		if oscillationResponse.ResourceThrashing != nil {
			summary.ThrashingDetected = true
			if oscillationResponse.ResourceThrashing.Severity != "none" {
				summary.Severity = oscillationResponse.ResourceThrashing.Severity
			}
		}

		// Check for high severity patterns
		if len(oscillationResponse.IneffectiveLoops) > 0 || len(oscillationResponse.CascadingFailures) > 0 {
			summary.ThrashingDetected = true
			if summary.Severity == "none" {
				summary.Severity = "high"
			}
		}

		// Set risk level based on severity
		switch summary.Severity {
		case "high", "critical":
			summary.RiskLevel = "high"
		case "medium":
			summary.RiskLevel = "medium"
		default:
			summary.RiskLevel = "low"
		}

		// If we detected patterns but no explicit confidence, set a reasonable default
		if summary.ThrashingDetected && summary.Confidence == 0.0 {
			summary.Confidence = 0.75
		}
	}

	return summary, nil
}

// unmarshalMCPData unmarshals MCP response data to the target struct
func (c *mcpClient) unmarshalMCPData(data interface{}, target interface{}) error {
	// The data from MCP is already a Go interface{}, but we need to convert it to JSON and back
	// to properly unmarshal into our target struct
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal MCP data: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal MCP data: %w", err)
	}

	return nil
}

// Helper function to create float pointer
func floatPtr(f float64) *float64 {
	return &f
}

// getEffectivenessMetrics retrieves action effectiveness metrics from MCP
func (c *mcpClient) getEffectivenessMetrics(ctx context.Context, resourceRef actionhistory.ResourceReference) (*EffectivenessMetrics, error) {
	request := mcp.MCPToolRequest{
		Method: "tools/call",
		Params: mcp.MCPToolParams{
			Name: "get_action_effectiveness",
			Arguments: map[string]interface{}{
				"namespace": resourceRef.Namespace,
				"kind":      resourceRef.Kind,
				"name":      resourceRef.Name,
				"timeRange": "7d",
			},
		},
	}

	response, err := c.actionHistoryServer.HandleToolCall(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to get effectiveness metrics: %w", err)
	}

	// Parse structured JSON response from MCP server
	metrics := &EffectivenessMetrics{
		AverageEffectiveness: 0.0,
		SuccessRate:          0.0,
		TotalAttempts:        0,
	}

	if len(response.Content) > 0 && response.Content[0].Type == "application/json" {
		// Unmarshal the structured JSON data
		var effectivenessResponse mcp.EffectivenessMetricsResponse
		if err := c.unmarshalMCPData(response.Content[0].Data, &effectivenessResponse); err != nil {
			return nil, fmt.Errorf("failed to parse effectiveness metrics response: %w", err)
		}

		c.logger.Debugf("MCP effectiveness metrics: %d total actions, %d action types",
			effectivenessResponse.TotalActions, len(effectivenessResponse.ActionEffectiveness))

		metrics.TotalAttempts = effectivenessResponse.TotalActions

		// Calculate overall effectiveness from all action types
		if len(effectivenessResponse.ActionEffectiveness) > 0 {
			var totalEffectiveness float64
			var totalSamples int
			var bestActionType string
			var bestEffectiveness float64

			for actionType, effectiveness := range effectivenessResponse.ActionEffectiveness {
				totalEffectiveness += effectiveness.AvgEffectiveness * float64(effectiveness.SampleSize)
				totalSamples += effectiveness.SampleSize

				// Track the most effective action type
				if effectiveness.AvgEffectiveness > bestEffectiveness {
					bestEffectiveness = effectiveness.AvgEffectiveness
					bestActionType = actionType
				}
			}

			if totalSamples > 0 {
				metrics.AverageEffectiveness = totalEffectiveness / float64(totalSamples)
			}

			metrics.ActionType = bestActionType

			// Calculate success rate (assume actions with effectiveness > 0.5 are successful)
			if metrics.AverageEffectiveness > 0.5 {
				metrics.SuccessRate = 0.8 // High success rate for effective actions
			} else {
				metrics.SuccessRate = 0.3 // Low success rate for ineffective actions
			}

			// Set recommended action based on effectiveness
			if metrics.AverageEffectiveness < 0.3 {
				metrics.RecommendedAction = "notify_only" // Low effectiveness, escalate
			} else if metrics.AverageEffectiveness > 0.7 {
				metrics.RecommendedAction = bestActionType // Continue with most effective action
			}
		}
	}

	return metrics, nil
}

// getClusterState retrieves current cluster state from K8s MCP server
func (c *mcpClient) getClusterState(ctx context.Context, resourceRef actionhistory.ResourceReference) (*ClusterState, error) {
	if c.k8sServer == nil {
		return nil, fmt.Errorf("kubernetes MCP server not configured")
	}

	clusterState := &ClusterState{}

	// Get node capacity information
	nodeCapacityRequest := mcp.MCPToolRequest{
		Method: "tools/call",
		Params: mcp.MCPToolParams{
			Name: "check_node_capacity",
			Arguments: map[string]interface{}{
				"resource_type": "",
			},
		},
	}

	nodeResponse, err := c.k8sServer.HandleToolCall(ctx, nodeCapacityRequest)
	if err != nil {
		c.logger.WithError(err).Warn("Failed to get node capacity from K8s MCP server")
	} else if len(nodeResponse.Content) > 0 && nodeResponse.Content[0].Type == "application/json" {
		// Parse node capacity data
		if nodeData, ok := nodeResponse.Content[0].Data.(map[string]interface{}); ok {
			if clusterCapacity, ok := nodeData["cluster_capacity"].(map[string]interface{}); ok {
				clusterState.NodeCapacity = &ResourceMetrics{
					CPUUsage:       parseFloat64(clusterCapacity, "used_cpu_percentage", 0.0),
					MemoryUsage:    parseFloat64(clusterCapacity, "used_memory_percentage", 0.0),
					StorageUsage:   parseFloat64(clusterCapacity, "used_storage_percentage", 0.0),
					AvailableNodes: parseInt(clusterCapacity, "ready_nodes", 0),
				}
			}
		}
	}

	// Get pod status information
	podStatusRequest := mcp.MCPToolRequest{
		Method: "tools/call",
		Params: mcp.MCPToolParams{
			Name: "get_namespace_resources",
			Arguments: map[string]interface{}{
				"namespace": resourceRef.Namespace,
			},
		},
	}

	podResponse, err := c.k8sServer.HandleToolCall(ctx, podStatusRequest)
	if err != nil {
		c.logger.WithError(err).Warn("Failed to get pod status from K8s MCP server")
	} else if len(podResponse.Content) > 0 && podResponse.Content[0].Type == "application/json" {
		// Parse pod status data
		if podData, ok := podResponse.Content[0].Data.(map[string]interface{}); ok {
			if resources, ok := podData["resources"].(map[string]interface{}); ok {
				if pods, ok := resources["pods"].(map[string]interface{}); ok {
					clusterState.PodStatus = &PodStatusInfo{
						Running:   parseInt(pods, "running", 0),
						Pending:   parseInt(pods, "pending", 0),
						Failed:    parseInt(pods, "failed", 0),
						Succeeded: parseInt(pods, "succeeded", 0),
					}
				}
			}
		}
	}

	// Get recent events
	eventsRequest := mcp.MCPToolRequest{
		Method: "tools/call",
		Params: mcp.MCPToolParams{
			Name: "get_recent_events",
			Arguments: map[string]interface{}{
				"namespace": resourceRef.Namespace,
			},
		},
	}

	eventsResponse, err := c.k8sServer.HandleToolCall(ctx, eventsRequest)
	if err != nil {
		c.logger.WithError(err).Warn("Failed to get recent events from K8s MCP server")
	} else if len(eventsResponse.Content) > 0 && eventsResponse.Content[0].Type == "application/json" {
		// Parse events data
		if eventData, ok := eventsResponse.Content[0].Data.(map[string]interface{}); ok {
			if events, ok := eventData["events"].([]interface{}); ok {
				var eventSummaries []EventSummary
				for _, event := range events {
					if eventMap, ok := event.(map[string]interface{}); ok {
						eventSummary := EventSummary{
							Type:    parseString(eventMap, "type"),
							Reason:  parseString(eventMap, "reason"),
							Message: parseString(eventMap, "message"),
						}
						// Parse timestamp
						if timestampStr := parseString(eventMap, "timestamp"); timestampStr != "" {
							if timestamp, err := time.Parse(time.RFC3339, timestampStr); err == nil {
								eventSummary.Timestamp = timestamp
							}
						}
						eventSummaries = append(eventSummaries, eventSummary)
					}
				}
				clusterState.RecentEvents = eventSummaries
			}
		}
	}

	// Check resource quotas for constraints
	quotaRequest := mcp.MCPToolRequest{
		Method: "tools/call",
		Params: mcp.MCPToolParams{
			Name: "check_resource_quotas",
			Arguments: map[string]interface{}{
				"namespace": resourceRef.Namespace,
			},
		},
	}

	quotaResponse, err := c.k8sServer.HandleToolCall(ctx, quotaRequest)
	if err != nil {
		c.logger.WithError(err).Warn("Failed to get resource quotas from K8s MCP server")
	} else if len(quotaResponse.Content) > 0 && quotaResponse.Content[0].Type == "application/json" {
		// Parse quota data and identify constraints
		if quotaData, ok := quotaResponse.Content[0].Data.(map[string]interface{}); ok {
			var constraints []string
			if quotas, ok := quotaData["quotas"].([]interface{}); ok {
				for _, quota := range quotas {
					if quotaMap, ok := quota.(map[string]interface{}); ok {
						if utilization, ok := quotaMap["utilization"].(map[string]interface{}); ok {
							// Check for high utilization
							if cpuUtil := parseString(utilization, "cpu_requests"); cpuUtil != "" {
								if utilFloat := parseUtilizationPercentage(cpuUtil); utilFloat > 80.0 {
									constraints = append(constraints, fmt.Sprintf("High CPU utilization: %s", cpuUtil))
								}
							}
							if memUtil := parseString(utilization, "memory_requests"); memUtil != "" {
								if utilFloat := parseUtilizationPercentage(memUtil); utilFloat > 80.0 {
									constraints = append(constraints, fmt.Sprintf("High memory utilization: %s", memUtil))
								}
							}
						}
					}
				}
			}
			clusterState.ResourceConstraints = constraints
		}
	}

	return clusterState, nil
}

// Helper functions for parsing data from MCP responses
func parseFloat64(data map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case string:
			// Try to parse percentage strings
			if len(v) > 1 && v[len(v)-1] == '%' {
				if f, err := fmt.Sscanf(v, "%f%%", &defaultValue); f == 1 && err == nil {
					return defaultValue
				}
			}
		}
	}
	return defaultValue
}

func parseInt(data map[string]interface{}, key string, defaultValue int) int {
	if val, ok := data[key]; ok {
		switch v := val.(type) {
		case int:
			return v
		case float64:
			return int(v)
		case float32:
			return int(v)
		}
	}
	return defaultValue
}

func parseString(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func parseUtilizationPercentage(utilStr string) float64 {
	if len(utilStr) > 1 && utilStr[len(utilStr)-1] == '%' {
		var percent float64
		if n, err := fmt.Sscanf(utilStr, "%f%%", &percent); n == 1 && err == nil {
			return percent
		}
	}
	return 0.0
}
