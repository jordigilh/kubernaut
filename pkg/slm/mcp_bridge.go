package slm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/mcp"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/k8s"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

// MCPBridge provides dynamic tool calling capabilities for LocalAI by simulating function calling through multi-turn conversations
type MCPBridge struct {
	localAIClient       LocalAIClientInterface
	actionHistoryServer *mcp.ActionHistoryMCPServer
	k8sClient           k8s.Client
	logger              *logrus.Logger
	config              MCPBridgeConfig
}

// MCPBridgeConfig configures the MCP bridge behavior
type MCPBridgeConfig struct {
	MaxToolRounds    int           `json:"max_tool_rounds"`
	Timeout          time.Duration `json:"timeout"`
	MaxParallelTools int           `json:"max_parallel_tools"`
}

// LocalAIClientInterface defines the interface for LocalAI communication
type LocalAIClientInterface interface {
	ChatCompletion(ctx context.Context, prompt string) (string, error)
}

// ToolRequest represents a request to execute an MCP tool
type ToolRequest struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

// ModelResponse represents the model's response, either requesting tools or providing a final decision
type ModelResponse struct {
	NeedTools    bool                   `json:"need_tools"`
	ToolRequests []ToolRequest          `json:"tool_requests,omitempty"`
	Action       string                 `json:"action,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	Confidence   float64                `json:"confidence,omitempty"`
	Reasoning    string                 `json:"reasoning"`
}

// ToolResult represents the result of executing an MCP tool
type ToolResult struct {
	Name   string      `json:"name"`
	Result interface{} `json:"result"`
	Error  string      `json:"error,omitempty"`
}

// NewMCPBridge creates a new MCP bridge instance
func NewMCPBridge(localAIClient LocalAIClientInterface, actionHistoryServer *mcp.ActionHistoryMCPServer, k8sClient k8s.Client, logger *logrus.Logger) *MCPBridge {
	return &MCPBridge{
		localAIClient:       localAIClient,
		actionHistoryServer: actionHistoryServer,
		k8sClient:           k8sClient,
		logger:              logger,
		config: MCPBridgeConfig{
			MaxToolRounds:    3,
			Timeout:          30 * time.Second,
			MaxParallelTools: 5,
		},
	}
}

// AnalyzeAlertWithDynamicMCP analyzes an alert using dynamic MCP tool calling
func (b *MCPBridge) AnalyzeAlertWithDynamicMCP(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	b.logger.WithFields(logrus.Fields{
		"alert":     alert.Name,
		"namespace": alert.Namespace,
		"severity":  alert.Severity,
	}).Info("Starting dynamic MCP analysis")

	// Start with tool-aware prompt
	initialPrompt := b.generateToolAwarePrompt(alert)

	// Begin multi-turn conversation
	return b.conductToolConversation(ctx, alert, initialPrompt, 0)
}

// conductToolConversation manages the multi-turn conversation with the model
func (b *MCPBridge) conductToolConversation(ctx context.Context, alert types.Alert, prompt string, round int) (*types.ActionRecommendation, error) {
	if round >= b.config.MaxToolRounds {
		b.logger.Warn("Maximum tool rounds reached, forcing decision")
		return b.forceDecision(ctx, alert, prompt)
	}

	// Send prompt to LocalAI
	response, err := b.localAIClient.ChatCompletion(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LocalAI request failed: %w", err)
	}

	// Parse model response
	modelResponse, err := b.parseModelResponse(response)
	if err != nil {
		b.logger.WithError(err).Warn("Failed to parse model response, attempting direct parsing")
		return b.parseDirectActionResponse(response)
	}

	// If model doesn't need tools, return final decision
	if !modelResponse.NeedTools {
		return b.convertToActionRecommendation(modelResponse), nil
	}

	// Validate tool requests
	if len(modelResponse.ToolRequests) == 0 {
		return nil, fmt.Errorf("model requested tools but provided no tool requests")
	}

	if len(modelResponse.ToolRequests) > b.config.MaxParallelTools {
		modelResponse.ToolRequests = modelResponse.ToolRequests[:b.config.MaxParallelTools]
		b.logger.Warnf("Limiting tool requests to %d", b.config.MaxParallelTools)
	}

	// Execute tools in parallel
	toolResults, err := b.executeToolsParallel(ctx, modelResponse.ToolRequests)
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	// Generate next prompt with tool results
	nextPrompt := b.generateToolResultsPrompt(alert, toolResults)

	// Continue conversation
	return b.conductToolConversation(ctx, alert, nextPrompt, round+1)
}

// executeToolsParallel executes multiple MCP tools in parallel
func (b *MCPBridge) executeToolsParallel(ctx context.Context, toolRequests []ToolRequest) ([]ToolResult, error) {
	results := make([]ToolResult, len(toolRequests))
	var wg sync.WaitGroup
	var mu sync.Mutex

	b.logger.Infof("Executing %d tools in parallel", len(toolRequests))

	for i, request := range toolRequests {
		wg.Add(1)
		go func(index int, req ToolRequest) {
			defer wg.Done()

			result, err := b.executeSingleTool(ctx, req)

			mu.Lock()
			results[index] = ToolResult{
				Name:   req.Name,
				Result: result,
			}
			if err != nil {
				results[index].Error = err.Error()
				b.logger.WithError(err).Warnf("Tool %s failed", req.Name)
			} else {
				b.logger.Debugf("Tool %s completed successfully", req.Name)
			}
			mu.Unlock()
		}(i, request)
	}

	// Wait for all tools to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return results, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(b.config.Timeout):
		return nil, fmt.Errorf("tool execution timeout after %v", b.config.Timeout)
	}
}

// executeSingleTool executes a single MCP tool based on its name
func (b *MCPBridge) executeSingleTool(ctx context.Context, request ToolRequest) (interface{}, error) {
	b.logger.Debugf("Executing tool: %s with args: %+v", request.Name, request.Args)

	switch request.Name {
	// Kubernetes MCP tools
	case "get_pod_status", "get_namespace_resources":
		return b.executeK8sTool(ctx, "get_namespace_resources", request.Args)
	case "check_node_capacity":
		return b.executeK8sTool(ctx, "check_node_capacity", request.Args)
	case "get_recent_events":
		return b.executeK8sTool(ctx, "get_recent_events", request.Args)
	case "check_resource_quotas":
		return b.executeK8sTool(ctx, "check_resource_quotas", request.Args)

	// Action History MCP tools
	case "get_action_history":
		return b.executeHistoryTool(ctx, "get_action_history", request.Args)
	case "check_oscillation_risk", "analyze_oscillation":
		return b.executeHistoryTool(ctx, "analyze_oscillation", request.Args)
	case "get_effectiveness_metrics":
		return b.executeHistoryTool(ctx, "get_action_effectiveness", request.Args)

	default:
		return nil, fmt.Errorf("unknown tool: %s", request.Name)
	}
}

// executeK8sTool executes a Kubernetes tool using direct client calls
func (b *MCPBridge) executeK8sTool(ctx context.Context, toolName string, args map[string]interface{}) (interface{}, error) {
	if b.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not configured")
	}

	b.logger.Debugf("Executing K8s tool: %s with args: %+v", toolName, args)

	switch toolName {
	case "get_pod_status", "get_namespace_resources":
		return b.getPodStatus(ctx, args)
	case "check_node_capacity":
		return b.checkNodeCapacity(ctx, args)
	case "get_recent_events":
		return b.getRecentEvents(ctx, args)
	case "check_resource_quotas":
		return b.checkResourceQuotas(ctx, args)
	default:
		return nil, fmt.Errorf("unknown K8s tool: %s", toolName)
	}
}

// getPodStatus retrieves pod status information for a namespace
func (b *MCPBridge) getPodStatus(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	namespace, _ := args["namespace"].(string)
	podName, _ := args["pod_name"].(string)

	if namespace == "" {
		return nil, fmt.Errorf("namespace is required for pod status")
	}

	if podName != "" {
		// Get specific pod
		pod, err := b.k8sClient.GetPod(ctx, namespace, podName)
		if err != nil {
			return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, podName, err)
		}

		return map[string]interface{}{
			"pod_name":           pod.Name,
			"namespace":          pod.Namespace,
			"phase":              string(pod.Status.Phase),
			"node_name":          pod.Spec.NodeName,
			"restart_count":      getTotalRestartCount(pod),
			"conditions":         formatPodConditions(pod.Status.Conditions),
			"container_statuses": formatContainerStatuses(pod.Status.ContainerStatuses),
			"resource_requests":  formatResourceRequests(pod.Spec.Containers),
			"resource_limits":    formatResourceLimits(pod.Spec.Containers),
		}, nil
	}

	// Get all pods in namespace
	podList, err := b.k8sClient.ListPodsWithLabel(ctx, namespace, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
	}

	var pods []map[string]interface{}
	runningCount, pendingCount, failedCount, succeededCount := 0, 0, 0, 0

	for _, pod := range podList.Items {
		podInfo := map[string]interface{}{
			"name":          pod.Name,
			"phase":         string(pod.Status.Phase),
			"node_name":     pod.Spec.NodeName,
			"restart_count": getTotalRestartCount(&pod),
		}
		pods = append(pods, podInfo)

		switch pod.Status.Phase {
		case corev1.PodRunning:
			runningCount++
		case corev1.PodPending:
			pendingCount++
		case corev1.PodFailed:
			failedCount++
		case corev1.PodSucceeded:
			succeededCount++
		}
	}

	return map[string]interface{}{
		"namespace":  namespace,
		"total_pods": len(podList.Items),
		"summary": map[string]interface{}{
			"running":   runningCount,
			"pending":   pendingCount,
			"failed":    failedCount,
			"succeeded": succeededCount,
		},
		"pods": pods,
	}, nil
}

// checkNodeCapacity retrieves node capacity and usage information
func (b *MCPBridge) checkNodeCapacity(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	// Get all nodes
	nodes, err := b.k8sClient.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var nodeInfos []map[string]interface{}
	totalCPU, totalMemory, allocatedCPU, allocatedMemory := int64(0), int64(0), int64(0), int64(0)
	readyNodes, totalNodes := 0, len(nodes.Items)

	for _, node := range nodes.Items {
		isReady := isNodeReady(node.Status.Conditions)
		if isReady {
			readyNodes++
		}

		nodeCPU := node.Status.Capacity.Cpu().MilliValue()
		nodeMemory := node.Status.Capacity.Memory().Value()
		totalCPU += nodeCPU
		totalMemory += nodeMemory

		// Get allocated resources for this node
		podList, err := b.k8sClient.ListPodsWithLabel(ctx, "", "")
		if err == nil {
			for _, pod := range podList.Items {
				if pod.Spec.NodeName == node.Name && pod.Status.Phase == corev1.PodRunning {
					for _, container := range pod.Spec.Containers {
						if cpu := container.Resources.Requests.Cpu(); cpu != nil {
							allocatedCPU += cpu.MilliValue()
						}
						if memory := container.Resources.Requests.Memory(); memory != nil {
							allocatedMemory += memory.Value()
						}
					}
				}
			}
		}

		nodeInfo := map[string]interface{}{
			"name":            node.Name,
			"ready":           isReady,
			"cpu_capacity":    nodeCPU,
			"memory_capacity": nodeMemory,
			"conditions":      formatNodeConditions(node.Status.Conditions),
		}
		nodeInfos = append(nodeInfos, nodeInfo)
	}

	cpuUsagePercent := float64(0)
	memoryUsagePercent := float64(0)
	if totalCPU > 0 {
		cpuUsagePercent = float64(allocatedCPU) / float64(totalCPU) * 100
	}
	if totalMemory > 0 {
		memoryUsagePercent = float64(allocatedMemory) / float64(totalMemory) * 100
	}

	return map[string]interface{}{
		"total_nodes": totalNodes,
		"ready_nodes": readyNodes,
		"cluster_capacity": map[string]interface{}{
			"cpu_millicores": totalCPU,
			"memory_bytes":   totalMemory,
		},
		"cluster_allocated": map[string]interface{}{
			"cpu_millicores": allocatedCPU,
			"memory_bytes":   allocatedMemory,
		},
		"cluster_usage": map[string]interface{}{
			"cpu_percent":    cpuUsagePercent,
			"memory_percent": memoryUsagePercent,
		},
		"nodes": nodeInfos,
	}, nil
}

// getRecentEvents retrieves recent events for a namespace
func (b *MCPBridge) getRecentEvents(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	namespace, _ := args["namespace"].(string)
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required for events")
	}

	events, err := b.k8sClient.GetEvents(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get events for namespace %s: %w", namespace, err)
	}

	var eventInfos []map[string]interface{}
	warningCount, normalCount := 0, 0

	for _, event := range events.Items {
		eventInfo := map[string]interface{}{
			"type":      event.Type,
			"reason":    event.Reason,
			"message":   event.Message,
			"object":    fmt.Sprintf("%s/%s", event.InvolvedObject.Kind, event.InvolvedObject.Name),
			"timestamp": event.LastTimestamp.Time,
			"count":     event.Count,
		}
		eventInfos = append(eventInfos, eventInfo)

		if event.Type == "Warning" {
			warningCount++
		} else {
			normalCount++
		}
	}

	return map[string]interface{}{
		"namespace":    namespace,
		"total_events": len(events.Items),
		"summary": map[string]interface{}{
			"warning_count": warningCount,
			"normal_count":  normalCount,
		},
		"events": eventInfos,
	}, nil
}

// checkResourceQuotas retrieves resource quota information for a namespace
func (b *MCPBridge) checkResourceQuotas(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	namespace, _ := args["namespace"].(string)
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required for resource quotas")
	}

	quotas, err := b.k8sClient.GetResourceQuotas(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource quotas for namespace %s: %w", namespace, err)
	}

	var quotaInfos []map[string]interface{}
	for _, quota := range quotas.Items {
		quotaInfo := map[string]interface{}{
			"name":        quota.Name,
			"hard_limits": quota.Status.Hard,
			"used":        quota.Status.Used,
		}
		quotaInfos = append(quotaInfos, quotaInfo)
	}

	return map[string]interface{}{
		"namespace":   namespace,
		"quota_count": len(quotas.Items),
		"quotas":      quotaInfos,
	}, nil
}

// Helper functions for formatting Kubernetes data

func getTotalRestartCount(pod *corev1.Pod) int32 {
	var total int32
	for _, status := range pod.Status.ContainerStatuses {
		total += status.RestartCount
	}
	return total
}

func formatPodConditions(conditions []corev1.PodCondition) []map[string]interface{} {
	var formatted []map[string]interface{}
	for _, condition := range conditions {
		formatted = append(formatted, map[string]interface{}{
			"type":    string(condition.Type),
			"status":  string(condition.Status),
			"reason":  condition.Reason,
			"message": condition.Message,
		})
	}
	return formatted
}

func formatContainerStatuses(statuses []corev1.ContainerStatus) []map[string]interface{} {
	var formatted []map[string]interface{}
	for _, status := range statuses {
		formatted = append(formatted, map[string]interface{}{
			"name":          status.Name,
			"ready":         status.Ready,
			"restart_count": status.RestartCount,
			"image":         status.Image,
		})
	}
	return formatted
}

func formatResourceRequests(containers []corev1.Container) map[string]interface{} {
	totalCPU, totalMemory := int64(0), int64(0)
	for _, container := range containers {
		if cpu := container.Resources.Requests.Cpu(); cpu != nil {
			totalCPU += cpu.MilliValue()
		}
		if memory := container.Resources.Requests.Memory(); memory != nil {
			totalMemory += memory.Value()
		}
	}
	return map[string]interface{}{
		"cpu_millicores": totalCPU,
		"memory_bytes":   totalMemory,
	}
}

func formatResourceLimits(containers []corev1.Container) map[string]interface{} {
	totalCPU, totalMemory := int64(0), int64(0)
	for _, container := range containers {
		if cpu := container.Resources.Limits.Cpu(); cpu != nil {
			totalCPU += cpu.MilliValue()
		}
		if memory := container.Resources.Limits.Memory(); memory != nil {
			totalMemory += memory.Value()
		}
	}
	return map[string]interface{}{
		"cpu_millicores": totalCPU,
		"memory_bytes":   totalMemory,
	}
}

func isNodeReady(conditions []corev1.NodeCondition) bool {
	for _, condition := range conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func formatNodeConditions(conditions []corev1.NodeCondition) []map[string]interface{} {
	var formatted []map[string]interface{}
	for _, condition := range conditions {
		formatted = append(formatted, map[string]interface{}{
			"type":    string(condition.Type),
			"status":  string(condition.Status),
			"reason":  condition.Reason,
			"message": condition.Message,
		})
	}
	return formatted
}

// executeHistoryTool executes an Action History MCP tool
func (b *MCPBridge) executeHistoryTool(ctx context.Context, toolName string, args map[string]interface{}) (interface{}, error) {
	if b.actionHistoryServer == nil {
		return nil, fmt.Errorf("action history MCP server not configured")
	}

	mcpRequest := mcp.MCPToolRequest{
		Method: "tools/call",
		Params: mcp.MCPToolParams{
			Name:      toolName,
			Arguments: args,
		},
	}

	response, err := b.actionHistoryServer.HandleToolCall(ctx, mcpRequest)
	if err != nil {
		return nil, fmt.Errorf("history MCP tool %s failed: %w", toolName, err)
	}

	// Extract data from MCP response
	if len(response.Content) > 0 && response.Content[0].Type == "application/json" {
		return response.Content[0].Data, nil
	}

	return map[string]interface{}{
		"message": "Tool completed but no JSON data returned",
		"content": response.Content,
	}, nil
}

// parseModelResponse parses the model's JSON response
func (b *MCPBridge) parseModelResponse(response string) (*ModelResponse, error) {
	// Extract JSON from mixed text/JSON response
	jsonStr, err := b.extractJSONFromResponse(response)
	if err != nil {
		b.logger.WithFields(logrus.Fields{
			"error":           err.Error(),
			"raw_response":    response,
			"response_length": len(response),
		}).Error("Failed to extract JSON from model response")
		return nil, fmt.Errorf("failed to extract JSON from model response: %w", err)
	}

	// Log the response for debugging
	b.logger.WithField("raw_response", response).Debug("Parsing model response")
	b.logger.WithField("extracted_json", jsonStr).Debug("Extracted JSON for parsing")

	var modelResponse ModelResponse
	if err := json.Unmarshal([]byte(jsonStr), &modelResponse); err != nil {
		b.logger.WithFields(logrus.Fields{
			"error":          err.Error(),
			"extracted_json": jsonStr,
			"json_length":    len(jsonStr),
		}).Error("Model response JSON parsing failed")
		return nil, fmt.Errorf("failed to parse model response as JSON: %w", err)
	}

	return &modelResponse, nil
}

// parseDirectActionResponse attempts to parse a direct action response when tool parsing fails
func (b *MCPBridge) parseDirectActionResponse(response string) (*types.ActionRecommendation, error) {
	// Extract JSON from mixed text/JSON response
	jsonStr, err := b.extractJSONFromResponse(response)
	if err != nil {
		b.logger.WithFields(logrus.Fields{
			"error":           err.Error(),
			"raw_response":    response,
			"response_length": len(response),
		}).Error("Failed to extract JSON from direct action response")
		return nil, fmt.Errorf("failed to extract JSON from direct action response: %w", err)
	}

	// Log the raw response for debugging
	b.logger.WithField("raw_response", response).Debug("Parsing direct action response")
	b.logger.WithField("extracted_json", jsonStr).Debug("Extracted JSON for parsing")

	var directResponse struct {
		Action     string                 `json:"action"`
		Parameters map[string]interface{} `json:"parameters"`
		Confidence float64                `json:"confidence"`
		Reasoning  string                 `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &directResponse); err != nil {
		b.logger.WithFields(logrus.Fields{
			"error":          err.Error(),
			"extracted_json": jsonStr,
			"json_length":    len(jsonStr),
		}).Error("JSON parsing failed - dumping response for analysis")
		return nil, fmt.Errorf("failed to parse direct action response: %w", err)
	}

	return &types.ActionRecommendation{
		Action:     directResponse.Action,
		Parameters: directResponse.Parameters,
		Confidence: directResponse.Confidence,
		Reasoning: &types.ReasoningDetails{
			PrimaryReason: directResponse.Reasoning,
		},
	}, nil
}

// convertToActionRecommendation converts a ModelResponse to ActionRecommendation
func (b *MCPBridge) convertToActionRecommendation(response *ModelResponse) *types.ActionRecommendation {
	return &types.ActionRecommendation{
		Action:     response.Action,
		Parameters: response.Parameters,
		Confidence: response.Confidence,
		Reasoning: &types.ReasoningDetails{
			PrimaryReason: response.Reasoning,
		},
	}
}

// forceDecision forces a decision when max rounds are reached
func (b *MCPBridge) forceDecision(ctx context.Context, alert types.Alert, lastPrompt string) (*types.ActionRecommendation, error) {
	forcePrompt := fmt.Sprintf(`%s

IMPORTANT: You have reached the maximum number of tool requests. You must make a final decision now based on the information available.

Respond with your final action decision in this format:
{
  "action": "one_of_the_available_actions",
  "parameters": {"cpu_limit": "500m", "memory_limit": "1Gi"},
  "confidence": 0.7,
  "reasoning": "Final decision based on available information..."
}`, lastPrompt)

	response, err := b.localAIClient.ChatCompletion(ctx, forcePrompt)
	if err != nil {
		return nil, fmt.Errorf("force decision failed: %w", err)
	}

	return b.parseDirectActionResponse(response)
}

// extractJSONFromResponse extracts JSON object from mixed text/JSON response
func (b *MCPBridge) extractJSONFromResponse(response string) (string, error) {
	// Clean up response - remove markdown code blocks if present
	cleaned := strings.TrimSpace(response)
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	} else if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	// If the cleaned response starts with '{', treat it as pure JSON
	if strings.HasPrefix(cleaned, "{") {
		return cleaned, nil
	}

	// Find JSON object in mixed text/JSON response
	start := -1
	braceCount := 0
	end := -1

	for i, char := range cleaned {
		if char == '{' {
			if start == -1 {
				start = i
			}
			braceCount++
		}
		if char == '}' {
			braceCount--
			if braceCount == 0 && start != -1 {
				end = i + 1
				break
			}
		}
	}

	if start == -1 || end == -1 {
		return "", fmt.Errorf("no JSON object found in response")
	}

	jsonStr := cleaned[start:end]

	// Validate that it's proper JSON by attempting to parse it
	var test interface{}
	if err := json.Unmarshal([]byte(jsonStr), &test); err != nil {
		return "", fmt.Errorf("extracted text is not valid JSON: %w", err)
	}

	return jsonStr, nil
}
