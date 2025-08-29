package slm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jordigilh/prometheus-alerts-slm/internal/config"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/metrics"
	"github.com/jordigilh/prometheus-alerts-slm/pkg/types"
	"github.com/sirupsen/logrus"
)

const (
	// basicPromptTemplate is the template for alerts without MCP context
	basicPromptTemplate = `<|system|>
You are a Kubernetes operations expert. Analyze alerts and recommend automated remediation actions. Always respond with valid JSON only.
<|user|>
Analyze this Kubernetes alert and recommend an action:

Alert: %s
Status: %s
Severity: %s
Description: %s
Namespace: %s
Resource: %s
Labels: %v
Annotations: %v

## CRITICAL DECISION RULES (check in order):

### 1. TERMINATION SIGNALS (highest priority):
- **"OOMKilled"** in labels/annotations → **increase_resources** (NEVER scale_deployment)
- **"CrashLoopBackOff"** → restart_pod OR rollback_deployment
- **"ImagePullBackOff"** → collect_diagnostics
- **"Evicted"** → migrate_workload OR drain_node

### 2. ERROR MESSAGE PATTERNS:
- **"memory limit exceeded"** → increase_resources
- **"disk space"** + percentage → cleanup_storage OR expand_pvc
- **"connection refused/timeout"** → restart_network OR update_network_policy
- **"permission denied"** → audit_logs OR rotate_secrets

### 3. RESOURCE SCOPE (check resource field and impact):
- **Resource is POD name** → increase_resources, restart_pod
- **Resource is NODE name** → drain_node, collect_diagnostics, notify_only
- **Multiple pods affected** → scale_deployment, migrate_workload
- **Cluster-wide** → collect_diagnostics, notify_only

### 4. NODE-LEVEL INDICATORS (override pod actions):
- **"kubelet"** in description → drain_node, collect_diagnostics
- **"network_reachable: false"** → drain_node, collect_diagnostics
- **"node_level_action_required"** → drain_node, collect_diagnostics
- **Resource field contains "node"** → drain_node, collect_diagnostics

## AVAILABLE ACTIONS:
- **scale_deployment**: Scale deployment replicas up/down
- **restart_pod**: Restart affected pod(s)
- **increase_resources**: Increase CPU/memory limits
- **notify_only**: No action, notify operators
- **rollback_deployment**: Rollback to previous revision
- **expand_pvc**: Expand storage volume
- **drain_node**: Drain node for maintenance
- **quarantine_pod**: Isolate pod for security
- **collect_diagnostics**: Gather diagnostic information
- **cleanup_storage**: Clean old data/logs
- **backup_data**: Emergency backups
- **update_hpa**: Modify autoscaler settings
- **rotate_secrets**: Rotate credentials/certificates
- **audit_logs**: Trigger security audit
- **update_network_policy**: Fix network connectivity
- **restart_network**: Restart CNI/DNS components
- **failover_database**: Database failover to replica
- **repair_database**: Database consistency checks
- **enable_debug_mode**: Enable debug logging
- **create_heap_dump**: Memory analysis dumps
- **optimize_resources**: Adjust resource requests/limits
- **migrate_workload**: Move workloads to different nodes

## RISK LEVELS:
- **Low Risk**: restart_pod, increase_resources, collect_diagnostics
- **Medium Risk**: scale_deployment, rollback_deployment, cleanup_storage
- **High Risk**: drain_node, restart_network, failover_database
- **Critical Risk**: quarantine_pod, rotate_secrets

## CRITICAL RULES:
1. **OOMKilled ALWAYS = increase_resources** (NEVER scale_deployment)
2. **Node resource name = drain_node/collect_diagnostics** (NEVER increase_resources)
3. **Check termination signals FIRST** - they override usage analysis
4. **Check resource scope SECOND** - node vs pod determines action type
5. **Unknown problems = collect_diagnostics**

CRITICAL: You MUST include a confidence score between 0.0 and 1.0 in your response.
Use the confidence guidelines provided to determine the appropriate score.

{
  "action": "one_of_the_available_actions",
  "parameters": {
    "replicas": 3,
    "cpu_limit": "500m",
    "memory_limit": "1Gi"
  },
  "confidence": 0.85,
  "reasoning": "Brief explanation of why this action was chosen"
}
<|assistant|>`

	// enhancedPromptTemplate is the template for alerts with MCP context
	enhancedPromptTemplate = `<|system|>
You are a Kubernetes operations expert specialized in analyzing alerts and recommending automated remediation actions. You have access to historical context and cluster state information. Use the structured analysis framework to ensure accurate root cause identification, especially considering historical patterns and action effectiveness. Always respond with valid JSON only.
<|user|>
Analyze this Kubernetes alert using the structured framework and historical context to recommend an action:

Alert: %s
Status: %s
Severity: %s
Description: %s
Namespace: %s
Resource: %s
Labels: %v
Annotations: %v
%s

## ALERT CONTEXT ANALYSIS FRAMEWORK:

### Step 1: Historical Pattern Analysis (PRIORITY)
WHEN historical context is available, FIRST analyze previous actions:

1. **Failed Action Detection**:
   - If scale_deployment failed and OOM kills occurred → Use increase_resources
   - If increase_resources failed repeatedly → Try restart_pod OR rollback_deployment
   - If restart_pod failed → Escalate to collect_diagnostics OR rollback_deployment

2. **Oscillation Prevention**:
   - Recent scale up/down cycles → Use notify_only OR optimize_resources
   - Repeated same actions → Try different approach OR escalate to manual intervention
   - Resource thrashing → Prioritize stability over performance

3. **Effectiveness Learning**:
   - Actions with >80%% effectiveness → Higher confidence in similar scenarios
   - Actions with <40%% effectiveness → Avoid unless no alternative
   - Unknown effectiveness → Moderate confidence, prefer safer actions

### Step 2: Root Cause Classification
Then apply standard analysis framework:

1. **Termination Reasons** (Check annotations/labels):
   - "OOMKilled" = Resource limits problem → increase_resources (NEVER scale_deployment)
   - "CrashLoopBackOff" = Application issue → restart_pod OR rollback_deployment
   - "Evicted" = Node pressure → migrate_workload OR drain_node

2. **Error Message Patterns**:
   - "memory limit exceeded" = Container limits → increase_resources
   - "insufficient memory" = Cluster capacity → scale_deployment OR migrate_workload
   - "disk space" + percentage = Storage issue → cleanup_storage OR expand_pvc

3. **Resource Usage Context**:
   - Single pod affected = Container-level → increase_resources, restart_pod
   - Multiple pods affected = Cluster-level → scale_deployment, migrate_workload
   - Cluster-wide = System-level → collect_diagnostics, notify_only

Available actions:
## Core Actions:
- scale_deployment: Scale deployment replicas up or down
- restart_pod: Restart the affected pod(s)
- increase_resources: Increase CPU/memory limits
- notify_only: No automated action, notify operators only
- rollback_deployment: Rollback deployment to previous working revision
- expand_pvc: Expand persistent volume claim size
- drain_node: Safely drain and cordon a node for maintenance
- quarantine_pod: Isolate pod with network policies for security
- collect_diagnostics: Gather detailed diagnostic information

## Storage & Persistence Actions:
- cleanup_storage: Clean up old data/logs when disk space is critical
- backup_data: Trigger emergency backups before disruptive actions
- compact_storage: Trigger storage compaction operations

## Application Lifecycle Actions:
- cordon_node: Mark nodes as unschedulable (without draining)
- update_hpa: Modify horizontal pod autoscaler settings
- restart_daemonset: Restart DaemonSet pods across nodes

## Security & Compliance Actions:
- rotate_secrets: Rotate compromised credentials/certificates
- audit_logs: Trigger detailed security audit collection

## Network & Connectivity Actions:
- update_network_policy: Modify network policies for connectivity issues
- restart_network: Restart network components (CNI, DNS)
- reset_service_mesh: Reset service mesh configuration

## Database & Stateful Services Actions:
- failover_database: Trigger database failover to replica
- repair_database: Run database repair/consistency checks
- scale_statefulset: Scale StatefulSets with proper ordering

## Monitoring & Observability Actions:
- enable_debug_mode: Enable debug logging temporarily
- create_heap_dump: Trigger memory dumps for analysis

## Resource Management Actions:
- optimize_resources: Intelligently adjust resource requests/limits
- migrate_workload: Move workloads to different nodes/zones

## CASCADING FAILURE PREVENTION (HIGHEST PRIORITY):
When multiple action failures are detected (>= 2 failures of same type):
1. **MANDATORY**: Choose only safe actions: notify_only, collect_diagnostics, increase_resources
2. **FORBIDDEN**: restart_pod, scale_deployment, rollback_deployment if they have failed recently
3. **REQUIRED**: Set confidence ≤ 0.6 when failure patterns exist
4. **EXPLANATION**: Must reference failure prevention in reasoning

IMPORTANT DECISION GUIDELINES:

1. **OSCILLATION PREVENTION**: If oscillation patterns are detected, consider conservative actions (notify_only, increase_resources) over aggressive scaling actions that could worsen oscillation.

2. **EFFECTIVENESS LEARNING**: Consider historical effectiveness data. If an action type has low effectiveness for this resource, choose a different approach.

3. **RESOURCE CONSTRAINTS**: Account for cluster capacity. If cluster is near capacity, consider resource optimization over scaling.

4. **ESCALATION PATTERNS**: If multiple similar actions have failed recently, consider escalating to different action types or alternative approaches.

5. **SECURITY CONTEXT**: For security alerts with poor containment history, prefer stronger isolation actions.

6. **CASCADING FAILURE PREVENTION**: If patterns indicate cascading failure risk (effectiveness < 20%%), strongly consider notify_only or collect_diagnostics to prevent worsening the situation.

Enhanced Production Guidelines (with Historical Context):

## OSCILLATION-AWARE DECISION MAKING:
- **Scale oscillation risk**: Avoid scale_deployment, prefer optimize_resources or notify_only
- **Resource thrashing**: Use increase_resources conservatively, consider migrate_workload
- **Network policy oscillation**: Prefer collect_diagnostics before update_network_policy
- **HPA oscillation**: Use update_hpa to adjust thresholds, not scale_deployment

## MEMORY SCENARIOS - Enhanced Decision Tree (with historical context):

### Memory Alert Decision Framework:
1. **Check termination_reason annotation FIRST**:
   - "OOMKilled" → increase_resources (NEVER scale_deployment)
   - No termination → analyze usage pattern

2. **Analyze memory usage pattern**:
   - Steady 90%%+ without kills → increase_resources
   - Gradual climb over time → potential leak → restart_pod + increase_resources
   - Spiky pattern → optimize_resources (right-size)
   - Cluster-wide pressure → migrate_workload or scale_deployment

3. **Cross-reference with error messages**:
   - "memory limit exceeded" → increase_resources
   - "insufficient memory" on cluster → scale_deployment
   - "out of memory" in container → increase_resources + restart_pod

4. **Historical Context Override**:
   - Previous scale_deployment failures for OOM → increase_resources with higher confidence
   - Oscillation risk detected → conservative actions only (notify_only, increase_resources)
   - Memory leak patterns → create_heap_dump + restart_pod

## STORAGE & PERSISTENCE SCENARIOS - Enhanced Decision Tree (effectiveness-aware):

### Storage Alert Decision Framework:
1. **Check storage type and usage pattern FIRST**:
   - "disk space" + percentage → disk space issue
   - "inode" + exhaustion → inode exhaustion (different from disk space)
   - "PVC" + capacity → persistent volume issue
   - "database" + corruption → data integrity issue

2. **Analyze urgency and criticality**:
   - >95%% usage → cleanup_storage (immediate)
   - 85-95%% usage → expand_pvc (long-term) + cleanup_storage (immediate)
   - Performance degraded → compact_storage, migrate_workload
   - Data corruption detected → backup_data + repair_database

3. **Historical Effectiveness Override**:
   - Previous expand_pvc failures → prefer cleanup_storage + backup_data
   - Successful cleanup history → cleanup_storage with higher confidence
- **Database issues**: backup_data + repair_database (minor), failover_database (severe)
- **PVC expansion failures in history**: prefer cleanup_storage + backup_data over expand_pvc

## NETWORK & CONNECTIVITY (context-aware):
- **Service unreachable**: update_network_policy (if policy-related), restart_network (CNI issues)
- **DNS failures**: restart_network (infrastructure), collect_diagnostics (complex issues)
- **Service mesh problems**: reset_service_mesh (config), collect_diagnostics (investigation)
- **Network policy effectiveness low**: prefer collect_diagnostics over repeated policy changes

## APPLICATION LIFECYCLE (pattern-aware):
- **HPA not scaling**: update_hpa (thresholds), scale_deployment (manual override)
- **Node issues**: cordon_node (prevent scheduling), drain_node (maintenance)
- **DaemonSet failures**: restart_daemonset (rolling), collect_diagnostics (investigation)
- **Performance degradation**: enable_debug_mode + optimize_resources, create_heap_dump if memory-related

## SECURITY & COMPLIANCE (threat-aware):
- **Security breach**: quarantine_pod (immediate), audit_logs (investigation)
- **Certificate issues**: rotate_secrets (renewal), audit_logs (compliance)
- **Credential compromise**: rotate_secrets + quarantine_pod (containment)
- **Repeated security failures**: escalate to quarantine_pod regardless of history

## DATABASE & STATEFUL SERVICES (data-safe):
- **Primary DB failure**: backup_data + failover_database (promote replica)
- **DB corruption**: backup_data + repair_database (minor), failover_database (severe)
- **StatefulSet scaling**: backup_data + scale_statefulset (ordered)
- **Storage issues**: backup_data + expand_pvc or cleanup_storage

## MONITORING & TROUBLESHOOTING (intelligence-guided):
- **Performance issues**: enable_debug_mode (analysis), create_heap_dump (memory problems)
- **Unknown errors**: collect_diagnostics (comprehensive), enable_debug_mode (patterns)
- **Memory analysis needed**: create_heap_dump (JVM/runtime), optimize_resources (right-sizing)
- **Pattern unclear**: prefer collect_diagnostics over action attempts

## RESOURCE OPTIMIZATION (efficiency-focused):
- **Over-provisioned**: optimize_resources (right-size), migrate_workload (consolidate)
- **Resource contention**: migrate_workload (redistribute), optimize_resources (efficiency)
- **Cost concerns**: optimize_resources (reduce waste), scale_deployment (only if needed)
- **Capacity planning**: collect_diagnostics + optimize_resources

## FAILURE PATTERN RESPONSES (learning from history):
- **Pod crashes with failed restart history**: consider rollback_deployment or increase_resources
- **Deployment failures**: backup_data + rollback_deployment
- **Storage expansion failures**: cleanup_storage + backup_data, avoid expand_pvc
- **Network policy ineffective**: collect_diagnostics before more policy changes
- **Database issues with repair failures**: prefer failover_database over repair_database
- **Security containment failures**: escalate to quarantine_pod + audit_logs + collect_diagnostics
- **Complex issues, cascading failure risk, or high oscillation risk**: strongly prefer collect_diagnostics or notify_only

## CONFIDENCE SCORING WITH HISTORICAL CONTEXT:

### Very High Confidence (0.9-1.0):
- Clear termination reasons + historical pattern match
- Previous same action with >80%% effectiveness
- Low blast radius + successful precedent

### High Confidence (0.8-0.9):
- Clear root cause + some historical evidence
- Medium risk action with good track record
- Standard scenario with effectiveness data

### Medium Confidence (0.6-0.8):
- Multiple possible causes but historical guidance
- New approach after previous action failures
- Medium blast radius with mixed history

### Low Confidence (0.3-0.6):
- Ambiguous symptoms despite historical context
- High blast radius action
- Previous actions show poor effectiveness

### Very Low Confidence (0.0-0.3):
- Conflicting signals + poor action history
- Unknown patterns + failed previous attempts
- ALWAYS recommend notify_only or collect_diagnostics

CRITICAL RULES:
1. **Never scale_deployment for OOM kills** - this creates more failing containers!
2. **If action failed recently (effectiveness <40%%), try different approach**
3. **Oscillation patterns detected → Conservative actions only**
4. **Historical context overrides current alert severity for action selection**

CRITICAL: You MUST include a confidence score between 0.0 and 1.0 in your response.
Use the confidence guidelines provided to determine the appropriate score.

Include confidence score (0.0-1.0) and detailed reasoning that MUST reference historical patterns when available

Respond with valid JSON in this exact format:
{
  "action": "one_of_the_available_actions",
  "parameters": {
    "replicas": 3,
    "cpu_limit": "500m",
    "memory_limit": "1Gi"
  },
  "confidence": 0.85,
  "reasoning": "Brief explanation that specifically mentions how historical context influenced this decision"
}
<|assistant|>`
)

type Client interface {
	AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error)
	IsHealthy() bool
}

type client struct {
	config     config.SLMConfig
	httpClient *http.Client
	log        *logrus.Logger
	mcpClient  MCPClient // Optional MCP client for contextual analysis
}

// LocalAI Chat Completion API structures
type LocalAIRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float32   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LocalAIResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage,omitempty"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewClient(cfg config.SLMConfig, log *logrus.Logger) (Client, error) {
	if cfg.Provider != "localai" {
		return nil, fmt.Errorf("only LocalAI provider supported, got: %s", cfg.Provider)
	}

	c := &client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		log: log,
	}

	log.WithFields(logrus.Fields{
		"provider": "LocalAI",
		"endpoint": cfg.Endpoint,
		"model":    cfg.Model,
	}).Info("SLM client initialized with LocalAI")

	return c, nil
}

// NewClientWithMCP creates a new SLM client with MCP context capabilities
func NewClientWithMCP(cfg config.SLMConfig, mcpClient MCPClient, log *logrus.Logger) (Client, error) {
	if cfg.Provider != "localai" {
		return nil, fmt.Errorf("only LocalAI provider supported, got: %s", cfg.Provider)
	}

	c := &client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		log:       log,
		mcpClient: mcpClient,
	}

	log.WithFields(logrus.Fields{
		"provider":    "LocalAI",
		"endpoint":    cfg.Endpoint,
		"model":       cfg.Model,
		"mcp_enabled": true,
	}).Info("SLM client initialized with LocalAI and MCP context")

	return c, nil
}

func (c *client) AnalyzeAlert(ctx context.Context, alert types.Alert) (*types.ActionRecommendation, error) {
	// Get contextual information from MCP if available
	var mcpContext *MCPContext
	var err error
	if c.mcpClient != nil {
		mcpContext, err = c.mcpClient.GetActionContext(ctx, alert)
		if err != nil {
			c.log.WithError(err).Warn("Failed to get MCP context, proceeding without it")
		}
	}

	prompt := c.generatePromptWithContext(alert, mcpContext)

	// Record context size metrics
	metrics.RecordSLMContextSize(c.config.Provider, len(prompt))

	c.log.WithFields(logrus.Fields{
		"alert":       alert.Name,
		"namespace":   alert.Namespace,
		"severity":    alert.Severity,
		"provider":    c.config.Provider,
		"mcp_enabled": c.mcpClient != nil,
		"has_context": mcpContext != nil,
	}).Debug("Analyzing alert with SLM")

	var lastErr error
	for attempt := 0; attempt <= c.config.RetryCount; attempt++ {
		if attempt > 0 {
			c.log.WithFields(logrus.Fields{
				"attempt": attempt,
				"alert":   alert.Name,
			}).Warn("Retrying LocalAI request")

			// Exponential backoff
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		// Record SLM API call attempt
		metrics.RecordSLMAPICall(c.config.Provider)

		recommendation, err := c.makeLocalAIRequest(ctx, prompt)
		if err != nil {
			// Record SLM API error
			metrics.RecordSLMAPIError(c.config.Provider, "request_failed")
			lastErr = err
			continue
		}

		c.log.WithFields(logrus.Fields{
			"alert":  alert.Name,
			"action": recommendation.Action,
		}).Info("LocalAI analysis completed")

		return recommendation, nil
	}

	return nil, fmt.Errorf("failed to analyze alert after %d attempts: %w", c.config.RetryCount+1, lastErr)
}

func (c *client) makeLocalAIRequest(ctx context.Context, prompt string) (*types.ActionRecommendation, error) {
	reqBody := LocalAIRequest{
		Model: c.config.Model,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: c.config.Temperature,
		MaxTokens:   c.config.MaxTokens,
		Stream:      false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal LocalAI request: %w", err)
	}

	// LocalAI chat completions endpoint
	endpoint := fmt.Sprintf("%s/v1/chat/completions", strings.TrimSuffix(c.config.Endpoint, "/"))

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create LocalAI request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	// LocalAI typically doesn't require authentication, but add if configured
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	c.log.WithFields(logrus.Fields{
		"endpoint": endpoint,
		"model":    c.config.Model,
	}).Debug("Making LocalAI request")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make LocalAI request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LocalAI API returned status %d: %s", resp.StatusCode, string(body))
	}

	var localAIResp LocalAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&localAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode LocalAI response: %w", err)
	}

	if len(localAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in LocalAI response")
	}

	// Parse the action recommendation from the response content
	content := localAIResp.Choices[0].Message.Content
	recommendation, err := c.parseActionRecommendation(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse action recommendation: %w", err)
	}

	// Log usage statistics if available
	if localAIResp.Usage.TotalTokens > 0 {
		c.log.WithFields(logrus.Fields{
			"prompt_tokens":     localAIResp.Usage.PromptTokens,
			"completion_tokens": localAIResp.Usage.CompletionTokens,
			"total_tokens":      localAIResp.Usage.TotalTokens,
		}).Debug("LocalAI token usage")
	}

	return recommendation, nil
}

// generatePromptWithContext creates an enhanced prompt that includes MCP context
func (c *client) generatePromptWithContext(alert types.Alert, mcpContext *MCPContext) string {
	var prompt string
	// Use enhanced prompt if we have MCP context, otherwise fall back to basic prompt
	if mcpContext != nil {
		prompt = c.generateEnhancedPrompt(alert, mcpContext)
	} else {
		prompt = c.generatePrompt(alert)
	}

	// Apply adaptive context size limitation
	contextSize := c.determineOptimalContextSize(prompt, mcpContext)
	if contextSize > 0 {
		return c.truncatePromptToContextSize(prompt, contextSize)
	}

	return prompt
}

func (c *client) generatePrompt(alert types.Alert) string {
	return fmt.Sprintf(basicPromptTemplate,
		alert.Name,
		alert.Status,
		alert.Severity,
		alert.Description,
		alert.Namespace,
		alert.Resource,
		alert.Labels,
		alert.Annotations,
	)
}

func (c *client) parseActionRecommendation(content string) (*types.ActionRecommendation, error) {
	// Try to extract JSON from the content
	start := -1
	braceCount := 0
	end := -1

	for i, char := range content {
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
		return nil, fmt.Errorf("no JSON found in response content: %s", content)
	}

	jsonStr := content[start:end]

	// First try to parse with flexible reasoning field
	var rawRecommendation map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &rawRecommendation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	// Build the recommendation with proper type conversion
	recommendation := &types.ActionRecommendation{}

	if action, ok := rawRecommendation["action"].(string); ok {
		recommendation.Action = action
	} else {
		return nil, fmt.Errorf("action field missing or not a string")
	}

	if params, ok := rawRecommendation["parameters"].(map[string]interface{}); ok {
		recommendation.Parameters = params
	}

	// Confidence is required - fail if not provided or invalid
	if confidence, ok := rawRecommendation["confidence"].(float64); ok {
		if confidence >= 0.0 && confidence <= 1.0 {
			recommendation.Confidence = confidence
		} else {
			return nil, fmt.Errorf("invalid confidence value: %f (must be between 0.0 and 1.0)", confidence)
		}
	} else {
		return nil, fmt.Errorf("missing required confidence field in SLM response - check prompt template compliance")
	}

	// Always create structured reasoning
	structuredReasoning := &types.ReasoningDetails{}

	// Handle reasoning field - could be string or structured object
	if reasoning, ok := rawRecommendation["reasoning"].(string); ok {
		// Convert simple string to structured reasoning
		structuredReasoning.Summary = reasoning
	} else if reasoningObj, ok := rawRecommendation["reasoning"].(map[string]interface{}); ok {
		// Parse structured reasoning
		if primary, ok := reasoningObj["primary_reason"].(string); ok {
			structuredReasoning.PrimaryReason = primary
		}
		if historical, ok := reasoningObj["historical_context"].(string); ok {
			structuredReasoning.HistoricalContext = historical
		}
		if oscillation, ok := reasoningObj["oscillation_risk"].(string); ok {
			structuredReasoning.OscillationRisk = oscillation
		}
		if summary, ok := reasoningObj["summary"].(string); ok {
			structuredReasoning.Summary = summary
		}

		// Handle alternative actions array
		if alts, ok := reasoningObj["alternative_actions"].([]interface{}); ok {
			for _, alt := range alts {
				if altStr, ok := alt.(string); ok {
					structuredReasoning.AlternativeActions = append(structuredReasoning.AlternativeActions, altStr)
				}
			}
		}

		// Handle confidence factors map
		if factors, ok := reasoningObj["confidence_factors"].(map[string]interface{}); ok {
			structuredReasoning.ConfidenceFactors = make(map[string]float64)
			for key, value := range factors {
				if floatVal, ok := value.(float64); ok {
					structuredReasoning.ConfidenceFactors[key] = floatVal
				}
			}
		}
	}

	recommendation.Reasoning = structuredReasoning

	// Validate action using shared validation
	if !types.IsValidAction(recommendation.Action) {
		return nil, fmt.Errorf("invalid action: %s", recommendation.Action)
	}

	return recommendation, nil
}

// generateEnhancedPrompt creates a context-aware prompt with MCP data
func (c *client) generateEnhancedPrompt(alert types.Alert, mcpContext *MCPContext) string {
	contextualInfo := ""

	// Add action history context with failure pattern detection
	if len(mcpContext.ActionHistory) > 0 {
		contextualInfo += "\n## HISTORICAL CONTEXT:\n"
		contextualInfo += fmt.Sprintf("Previous actions taken on this resource (%s/%s):\n", alert.Namespace, alert.Resource)

		// Track failure patterns
		failedActionTypes := make(map[string]int)

		for i, action := range mcpContext.ActionHistory {
			if i >= 5 { // Limit to most recent 5 actions
				break
			}
			effectivenessStr := "unknown"
			if action.Effectiveness != nil {
				effectivenessStr = fmt.Sprintf("%.1f%%", *action.Effectiveness*100)
			}
			contextualInfo += fmt.Sprintf("- %s: %s (confidence: %.2f, status: %s, effectiveness: %s) - %s\n",
				action.Timestamp.Format("2006-01-02 15:04"), action.ActionType, action.Confidence,
				action.ExecutionStatus, effectivenessStr, action.AlertName)

			// Count significantly failed action types (stricter criteria with real data)
			if action.ExecutionStatus == "failed" || (action.Effectiveness != nil && *action.Effectiveness < 0.2) {
				failedActionTypes[action.ActionType]++
			}
		}

		// Add failure pattern warnings with more nuanced guidance
		if len(failedActionTypes) > 0 {
			contextualInfo += "\n⚠️  FAILURE PATTERN DETECTED:\n"
			for actionType, count := range failedActionTypes {
				if count >= 2 {
					contextualInfo += fmt.Sprintf("- %s has failed %d times recently - consider alternative approaches\n", actionType, count)
				} else {
					contextualInfo += fmt.Sprintf("- %s has failed %d time recently - proceed with caution if choosing this action\n", actionType, count)
				}
			}
			// Only require escalation for repeated failures (count >= 2)
			hasRepeatedFailures := false
			for _, count := range failedActionTypes {
				if count >= 2 {
					hasRepeatedFailures = true
					break
				}
			}
			if hasRepeatedFailures {
				contextualInfo += "⚠️ SAFETY PRIORITY: Multiple failures detected - choose conservative actions only\n"
				contextualInfo += "MANDATORY: Select from [notify_only, collect_diagnostics, increase_resources] ONLY\n"
				contextualInfo += "REQUIRED: Use confidence ≤ 0.6 due to failure patterns\n"
			}
		}

		// Check for cascading failure patterns (multiple actions with very low effectiveness)
		lowEffectivenessCount := 0
		var recentLowEffectivenessActions []string
		for _, action := range mcpContext.ActionHistory {
			if action.Effectiveness != nil && *action.Effectiveness <= 0.2 { // More strict threshold for real concern
				lowEffectivenessCount++
				recentLowEffectivenessActions = append(recentLowEffectivenessActions, action.ActionType)
			}
		}
		if lowEffectivenessCount >= 2 && len(mcpContext.ActionHistory) >= 3 { // More aggressive threshold
			contextualInfo += "\n🚨 CASCADING FAILURE RISK: Multiple recent actions with effectiveness ≤ 20%\n"
			contextualInfo += fmt.Sprintf("Actions with poor effectiveness: %v\n", recentLowEffectivenessActions)
			contextualInfo += "⚠️ SAFETY PRIORITY: Choose conservative actions to break failure pattern\n"
			contextualInfo += "MANDATORY: Select from [notify_only, collect_diagnostics] ONLY\n"
			contextualInfo += "REQUIRED: Use confidence ≤ 0.4 due to cascading failures\n"
		}
	}

	// Add oscillation analysis
	if mcpContext.OscillationAnalysis != nil && mcpContext.OscillationAnalysis.Severity != "none" {
		contextualInfo += "\n## OSCILLATION RISK:\n"
		contextualInfo += fmt.Sprintf("⚠️  OSCILLATION DETECTED - Severity: %s (Confidence: %.2f)\n",
			mcpContext.OscillationAnalysis.Severity, mcpContext.OscillationAnalysis.Confidence)
		if mcpContext.OscillationAnalysis.ThrashingDetected {
			contextualInfo += "- Resource thrashing patterns detected\n"
		}
		if mcpContext.OscillationAnalysis.ScaleChanges > 0 {
			contextualInfo += fmt.Sprintf("- %d scale direction changes detected\n", mcpContext.OscillationAnalysis.ScaleChanges)
		}
		contextualInfo += fmt.Sprintf("- Risk level: %s\n", mcpContext.OscillationAnalysis.RiskLevel)
		contextualInfo += "RECOMMENDATION: Consider conservative actions to prevent worsening oscillation\n"
	}

	// Add effectiveness metrics
	if mcpContext.EffectivenessMetrics != nil {
		contextualInfo += "\n## ACTION EFFECTIVENESS:\n"
		contextualInfo += fmt.Sprintf("Historical effectiveness for %s actions: %.1f%% (based on %d attempts)\n",
			mcpContext.EffectivenessMetrics.ActionType,
			mcpContext.EffectivenessMetrics.AverageEffectiveness*100,
			mcpContext.EffectivenessMetrics.TotalAttempts)
		contextualInfo += fmt.Sprintf("Success rate: %.1f%%\n", mcpContext.EffectivenessMetrics.SuccessRate*100)
		if mcpContext.EffectivenessMetrics.RecommendedAction != "" {
			contextualInfo += fmt.Sprintf("Historically effective action: %s\n", mcpContext.EffectivenessMetrics.RecommendedAction)
		}
	}

	// Add cluster state information
	if mcpContext.ClusterState != nil {
		contextualInfo += "\n## CLUSTER STATE:\n"
		if mcpContext.ClusterState.NodeCapacity != nil {
			capacity := mcpContext.ClusterState.NodeCapacity
			contextualInfo += fmt.Sprintf("Cluster capacity: CPU %.1f%%, Memory %.1f%%, Storage %.1f%%\n",
				capacity.CPUUsage*100, capacity.MemoryUsage*100, capacity.StorageUsage*100)
			contextualInfo += fmt.Sprintf("Available nodes: %d\n", capacity.AvailableNodes)
		}
		if len(mcpContext.ClusterState.ResourceConstraints) > 0 {
			contextualInfo += "Resource constraints:\n"
			for _, constraint := range mcpContext.ClusterState.ResourceConstraints {
				contextualInfo += fmt.Sprintf("- %s\n", constraint)
			}
		}
	}

	//nolint:SA5009 // Template constant has correct number of format placeholders
	return fmt.Sprintf(enhancedPromptTemplate,
		alert.Name,
		alert.Status,
		alert.Severity,
		alert.Description,
		alert.Namespace,
		alert.Resource,
		alert.Labels,
		alert.Annotations,
		contextualInfo,
	)
}

// Mock functionality removed - only LocalAI supported

func (c *client) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use Ollama's API tags endpoint for health check
	healthEndpoint := fmt.Sprintf("%s/api/tags", strings.TrimSuffix(c.config.Endpoint, "/"))

	req, err := http.NewRequestWithContext(ctx, "GET", healthEndpoint, nil)
	if err != nil {
		return false
	}

	// Ollama doesn't require authentication for API endpoints
	if c.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// truncatePromptToContextSize truncates the prompt to fit within the specified context size
func (c *client) truncatePromptToContextSize(prompt string, maxContextTokens int) string {
	// Rough token estimation: approximately 4 characters per token
	estimatedTokens := len(prompt) / 4

	if estimatedTokens <= maxContextTokens {
		return prompt
	}

	// Calculate target character length based on token limit
	targetLength := maxContextTokens * 4

	// Split prompt into sections to preserve critical parts
	lines := strings.Split(prompt, "\n")

	// Always preserve system message and basic alert info (first ~20 lines)
	var preservedLines []string
	var contextualLines []string
	var foundContextStart bool

	for i, line := range lines {
		if i < 20 || strings.Contains(line, "<|system|>") || strings.Contains(line, "Alert:") ||
			strings.Contains(line, "Status:") || strings.Contains(line, "Severity:") ||
			strings.Contains(line, "Available actions:") || strings.Contains(line, "<|assistant|>") ||
			strings.Contains(line, "CRITICAL:") || strings.Contains(line, "confidence") ||
			strings.Contains(line, "\"action\":") || strings.Contains(line, "\"confidence\":") {
			preservedLines = append(preservedLines, line)
		} else if strings.Contains(line, "## HISTORICAL CONTEXT:") {
			foundContextStart = true
			contextualLines = append(contextualLines, line)
		} else if foundContextStart {
			contextualLines = append(contextualLines, line)
		} else {
			preservedLines = append(preservedLines, line)
		}
	}

	// Reconstruct prompt with preserved content
	preservedPrompt := strings.Join(preservedLines, "\n")

	// If we're still over the limit, truncate contextual information progressively
	if len(preservedPrompt)/4 > maxContextTokens {
		// Fallback to basic truncation but preserve JSON template and confidence requirement
		if len(preservedPrompt) > targetLength {
			// Find the last occurrence of critical confidence instruction
			confidenceIndex := strings.LastIndex(preservedPrompt, "CRITICAL: You MUST include a confidence score")
			if confidenceIndex > 0 && confidenceIndex < targetLength {
				// Keep everything up to confidence instruction
				return preservedPrompt[:confidenceIndex] + preservedPrompt[confidenceIndex:] + "\n<|assistant|>"
			} else {
				// Emergency fallback - add minimal confidence requirement
				minimalPrompt := preservedPrompt[:targetLength-200] // Reserve space for requirement
				return minimalPrompt + "\n\nCRITICAL: You MUST include a confidence score between 0.0 and 1.0.\n{\n  \"action\": \"notify_only\",\n  \"confidence\": 0.5,\n  \"reasoning\": \"Context truncated\"\n}\n<|assistant|>"
			}
		}
		return preservedPrompt
	}

	// Add as much contextual information as possible
	remainingTokens := maxContextTokens - (len(preservedPrompt) / 4)
	remainingChars := remainingTokens * 4

	if len(contextualLines) > 0 {
		contextualContent := strings.Join(contextualLines, "\n")
		if len(contextualContent) <= remainingChars {
			return preservedPrompt + "\n" + contextualContent
		} else {
			// Truncate contextual content and add summary
			truncatedContext := contextualContent[:remainingChars-200] // Reserve space for summary
			return preservedPrompt + "\n" + truncatedContext + "\n\n[Context truncated due to size limits]\n<|assistant|>"
		}
	}

	return preservedPrompt
}

// determineOptimalContextSize determines the best context size based on prompt length and complexity
func (c *client) determineOptimalContextSize(prompt string, mcpContext *MCPContext) int {
	// If user has configured a specific max context size, respect it
	if c.config.MaxContextSize > 0 {
		return c.config.MaxContextSize
	}

	// Calculate current prompt size in estimated tokens (4 chars ≈ 1 token)
	currentPromptTokens := len(prompt) / 4

	// Default base size for simple scenarios
	baseSize := 4000

	// If current prompt already exceeds base size, we need more context
	if currentPromptTokens > baseSize {
		baseSize = currentPromptTokens + 500 // Add buffer for response
	}

	// No MCP context = simple scenario, just ensure we have enough for current prompt
	if mcpContext == nil {
		// Still need to apply caps even for simple scenarios
		targetSize := baseSize

		// Apply absolute caps
		if targetSize > 16000 {
			targetSize = 16000 // Maximum for very complex scenarios (absolute limit)
		} else if targetSize < 3000 {
			targetSize = 3000 // Minimum for scenarios
		}

		return targetSize
	}

	// Calculate complexity score for additional context needs
	complexityScore := 0

	// Factor 1: Number of historical actions
	if len(mcpContext.ActionHistory) > 0 {
		complexityScore += len(mcpContext.ActionHistory) * 50 // 50 tokens per action
	}

	// Factor 2: Oscillation detection complexity
	if mcpContext.OscillationAnalysis != nil && mcpContext.OscillationAnalysis.Severity != "none" {
		complexityScore += 500 // Oscillation analysis adds complexity
	}

	// Factor 3: Multiple action types with failures (system instability)
	actionTypes := make(map[string]int)
	for _, action := range mcpContext.ActionHistory {
		if action.ExecutionStatus == "failed" || (action.Effectiveness != nil && *action.Effectiveness < 0.4) {
			actionTypes[action.ActionType]++
		}
	}
	if len(actionTypes) >= 3 {
		complexityScore += 1000 // Complex multi-failure scenarios
	}

	// Factor 4: Cluster state information
	if mcpContext.ClusterState != nil {
		complexityScore += 300
	}

	// Determine context size based on current prompt + complexity
	targetSize := baseSize + complexityScore

	// Ensure we never return less than current prompt needs, but respect absolute caps
	minRequired := currentPromptTokens + 200 // Reserve space for response
	if targetSize < minRequired {
		targetSize = minRequired
	}

	// Apply absolute caps (maximum takes priority over minimum requirements)
	if targetSize > 16000 {
		targetSize = 16000 // Maximum for very complex scenarios (absolute limit)
	}

	// Only apply minimum if we haven't hit the maximum cap
	if targetSize < 3000 && targetSize < 16000 {
		targetSize = 3000 // Minimum for MCP scenarios
	}

	return targetSize
}
