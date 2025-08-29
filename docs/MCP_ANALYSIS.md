# Kubernetes MCP Server Integration Analysis

**Concept**: Enable models to directly query Kubernetes cluster state during decision-making process
**Impact**: Transform from static alert-based decisions to dynamic, context-aware remediation
**Innovation Level**: Real-time cluster intelligence for AI models

## 🎯 **Current vs. MCP-Enhanced Architecture**

### Current Architecture (Static Context)
```
AlertManager → App → Static Prompt → Model → Action Decision → App → K8s Execution
                     ↑
              Limited context from alert only
```

### MCP-Enhanced Architecture (Dynamic Context)
```
AlertManager → App → Alert + MCP Tools → Model ←→ K8s MCP Server → Live Cluster State
                                         ↓
                              Context-Aware Decision → App → K8s Execution
```

## 🚀 **Benefits**

### 1. Real-Time Cluster Intelligence
```yaml
# Instead of static alert context:
alert: "HighMemoryUsage on webapp-123"
severity: "warning"
description: "Pod using 95% memory"

# Model gets live cluster state:
current_state:
  pod: "webapp-123"
  memory_usage: "7.6Gi/8Gi (95%)"
  cpu_usage: "1.2/2 cores (60%)"
  deployment_replicas: 3
  available_nodes: 5
  node_capacity:
    - node-1: "memory: 16Gi available, cpu: 4 cores available"
    - node-2: "memory: 8Gi available, cpu: 2 cores available"
  recent_scaling_history: "Last scaled 2 hours ago (2→3 replicas)"
  related_alerts: ["NetworkLatencyHigh on same deployment"]
```

### 2. Intelligent Decision Making
The model can now ask questions like:
- "Are there available nodes with sufficient resources for scaling?"
- "Has this deployment been scaled recently?"
- "Are there other pods in this namespace also experiencing issues?"
- "What's the current HPA configuration and its recent behavior?"
- "Are there any network policies that might affect scaling?"

### 3. Validation Before Action
```go
// Model can validate assumptions:
mcp_query: "Check if deployment webapp has capacity to scale to 5 replicas"
response: {
  "feasible": true,
  "available_nodes": 3,
  "resource_requirements": "2Gi memory, 0.5 CPU per pod",
  "total_needed": "10Gi memory, 2.5 CPU",
  "cluster_available": "24Gi memory, 8 CPU",
  "recommendation": "Scaling is safe"
}
```

## 🔧 **Technical Implementation**

### Using Existing containers/kubernetes-mcp-server ⭐ **ADOPTED SOLUTION**

**Decision**: Instead of implementing a custom Kubernetes MCP server, we will use the existing **containers/kubernetes-mcp-server** from the containers organization.

**Repository**: `github.com/containers/kubernetes-mcp-server`

### Integration Architecture
```go
// Our MCP client will connect to external servers
type mcpClient struct {
    config              MCPClientConfig
    actionHistoryServer *mcp.ActionHistoryMCPServer     // Our custom server
    kubernetesServer    MCPServerConnection             // External K8s MCP server
    logger              *logrus.Logger
}

// External Kubernetes MCP Server (containers/kubernetes-mcp-server)
// Provides these capabilities out-of-the-box:
type ExternalKubernetesMCPCapabilities struct {
    // Native Kubernetes API integration
    PodOperations        []string  // get, list, describe pods
    NamespaceOperations  []string  // get, list namespaces
    EventOperations      []string  // get, list events
    GenericCRUD          []string  // create, read, update, delete resources
    HelmChartOperations  []string  // helm operations
    OpenShiftSupport     bool      // explicit OpenShift compatibility
}
```

### Available MCP Tools (from containers/kubernetes-mcp-server)
The external server provides these capabilities that match our requirements:

| **Our Requirement** | **containers/k8s-mcp Tool** | **Status** |
|---------------------|------------------------------|------------|
| `get_pod_status` | Native pod operations | ✅ Available |
| `check_node_capacity` | Node resource queries | ✅ Available |
| `get_deployment_history` | Deployment operations | ✅ Available |
| `check_resource_quotas` | ResourceQuota operations | ✅ Available |
| `get_hpa_status` | HPA operations | ✅ Available |
| `list_related_alerts` | Event/label queries | ✅ Available |

### Enhanced Model Prompt with MCP
```
<|system|>
You are a Kubernetes operations expert with real-time cluster access. You can query the cluster state using the provided MCP tools to make informed decisions.

Available tools:
- get_pod_status: Check current pod state and resource usage
- check_node_capacity: Verify available cluster resources
- get_deployment_history: Check recent scaling/update history
- list_related_alerts: Find correlated issues
- check_resource_quotas: Verify namespace limits
- get_hpa_status: Check autoscaler configuration

<|user|>
Alert: HighMemoryUsage on webapp-123 (95% memory usage)
Namespace: production
Severity: warning

Please analyze the situation using the available tools and recommend an action.

<|assistant|>
I'll analyze this memory usage alert by gathering real-time cluster information.

<tool_call>
{"tool": "get_pod_status", "parameters": {"namespace": "production", "pod_name": "webapp-123"}}
</tool_call>

<tool_result>
{
  "pod": "webapp-123",
  "memory_usage": "7.6Gi/8Gi (95%)",
  "cpu_usage": "1.2/2 cores (60%)",
  "status": "Running",
  "node": "worker-02",
  "restart_count": 0,
  "age": "4h32m"
}
</tool_result>

<tool_call>
{"tool": "check_node_capacity", "parameters": {"resource_type": "memory"}}
</tool_call>

<tool_result>
{
  "total_nodes": 5,
  "available_memory": "48Gi",
  "nodes": [
    {"name": "worker-01", "available_memory": "12Gi"},
    {"name": "worker-02", "available_memory": "4Gi"},
    {"name": "worker-03", "available_memory": "16Gi"},
    {"name": "worker-04", "available_memory": "8Gi"},
    {"name": "worker-05", "available_memory": "8Gi"}
  ]
}
</tool_result>

<tool_call>
{"tool": "get_deployment_history", "parameters": {"namespace": "production", "deployment": "webapp", "hours_back": 24}}
</tool_call>

<tool_result>
{
  "deployment": "webapp",
  "current_replicas": 3,
  "recent_changes": [
    {"time": "2h ago", "action": "scaled", "from": 2, "to": 3, "reason": "HighCPUUsage"},
    {"time": "6h ago", "action": "updated", "reason": "image update"}
  ],
  "hpa_enabled": true
}
</tool_result>

Based on my analysis:

1. **Current State**: Pod webapp-123 is using 95% of allocated memory (7.6Gi/8Gi) but only 60% CPU
2. **Cluster Capacity**: Sufficient memory available (48Gi total) across cluster
3. **Recent History**: Deployment was scaled up 2h ago due to CPU issues, now has memory pressure

**Recommendation**:
{
  "action": "increase_resources",
  "parameters": {
    "deployment": "webapp",
    "namespace": "production",
    "memory_limit": "12Gi",
    "memory_request": "10Gi"
  },
  "confidence": 0.95,
  "reasoning": "Memory increase is preferred over scaling because: (1) CPU usage is normal (60%), (2) Recent scaling was for CPU, not memory, (3) Cluster has sufficient memory capacity, (4) Resource increase is less disruptive than adding more replicas"
}
```

## 🎯 **Model Capability Assessment**

### Can Current Models Handle MCP?

#### Granite 3.1 Dense 8B ✅ **Strong Candidate**
- **Strong reasoning**: Can handle multi-step analysis with API responses
- **Tool usage**: Demonstrated ability to use structured tools effectively
- **Context management**: Large context window can handle cluster state data
- **JSON processing**: Good at parsing and reasoning about structured data

#### Granite 3.1 Dense 2B ⚠️ **Promising with Testing Needed**
- **Moderate reasoning**: Should handle basic MCP queries
- **Tool usage**: Needs testing to verify tool usage capabilities
- **Context limits**: May struggle with very large cluster state responses
- **Performance**: Additional API calls might impact our 1.94s response time

#### Smaller Models (MoE 1B, etc.) ❌ **Likely Insufficient**
- **Limited reasoning**: Struggle with multi-step API reasoning
- **Tool complexity**: May not handle structured tool responses well
- **Context constraints**: Limited ability to process large cluster state data

### Enhanced Model Comparison with MCP
```go
// New evaluation criteria for MCP capability
type MCPEvaluation struct {
    Model              string
    ToolUsageAccuracy  float64  // Can it use tools correctly?
    ContextManagement  float64  // Can it handle large API responses?
    ReasoningWithData  float64  // Can it reason about cluster state?
    ResponseTime       time.Duration // Including MCP query overhead
    TokenEfficiency    float64  // How efficiently does it use context?
}
```

## 🏗️ **Integration into Current Roadmap**

### Phase 1.4: MCP Server Integration (UPDATED)
**Status**: 🔄 Pending
**Duration**: 2-3 weeks
**Priority**: High Innovation Value

#### Integration Tasks:
- [ ] **External MCP Server Setup**: Deploy containers/kubernetes-mcp-server
- [ ] **MCP Client Updates**: Extend client to connect to external K8s MCP server
- [ ] **Security & RBAC**: Configure RBAC for external MCP server access
- [ ] **Connection Management**: Handle connections to multiple MCP servers
- [ ] **Deployment Integration**: Add K8s MCP server to deployment manifests

#### Model Integration:
- [ ] **Prompt Engineering**: Design MCP-aware prompts using external server tools
- [ ] **Tool Mapping**: Map our requirements to containers/k8s-mcp capabilities
- [ ] **Context Management**: Handle responses from external MCP server
- [ ] **Fallback Logic**: Graceful degradation when external MCP unavailable

#### Benefits of Using Existing Server:
- ✅ **4-5 weeks development time saved**
- ✅ **Production-ready implementation**
- ✅ **OpenShift compatibility verified**
- ✅ **Community maintenance and updates**
- ✅ **Security hardening already implemented**

### Phase 1.5: MCP-Enhanced Model Comparison (NEW)
**Status**: 🔄 Pending
**Duration**: 3-4 weeks
**Priority**: Critical for Innovation

#### Evaluation Framework:
```bash
# Test each model with MCP capabilities
for model in granite-8b granite-2b phi-2 gemma-2b qwen2-2b; do
    MCP_ENABLED=true OLLAMA_MODEL=$model \
    go test -v -tags=integration,mcp ./test/integration/...
done

# New metrics to capture:
# - Tool usage accuracy (does model use tools correctly?)
# - Decision quality improvement (better decisions with cluster context?)
# - Response time impact (how much slower with MCP?)
# - Context efficiency (how well does model manage large responses?)
```

## 🚀 **Use Cases Enabled**

### 1. Intelligent Scaling Decisions
```yaml
Traditional: "High memory → scale deployment"
MCP-Enhanced:
  - Check current cluster capacity
  - Verify recent scaling history
  - Analyze resource quotas
  - Consider node affinity rules
  - Decision: "Increase memory limits instead of scaling - more efficient given current cluster state"
```

### 2. Cascading Failure Prevention
```yaml
Traditional: "Pod failing → restart pod"
MCP-Enhanced:
  - Check if other pods in deployment also failing
  - Verify node health and resource availability
  - Check for related alerts (network, storage)
  - Decision: "Node drain required - multiple pods failing on same node due to hardware issue"
```

### 3. Resource Optimization
```yaml
Traditional: "CPU high → scale up"
MCP-Enhanced:
  - Check HPA configuration and current metrics
  - Verify if scale-up already in progress
  - Analyze historical scaling patterns
  - Check resource quotas and cluster capacity
  - Decision: "Wait for HPA to complete current scaling before manual intervention"
```

## ⚠️ **Challenges & Mitigations**

### 1. Performance Impact
**Challenge**: MCP queries add latency to model responses
**Mitigation**:
- Intelligent caching of cluster state
- Parallel MCP queries where possible
- Tool selection based on alert type
- Fallback to non-MCP mode for time-critical scenarios

### 2. Security Concerns
**Challenge**: Model has direct cluster access
**Mitigation**:
- Read-only RBAC permissions for MCP server
- Rate limiting and query validation
- Audit logging of all MCP queries
- Network policies for MCP server isolation

### 3. Complexity Management
**Challenge**: Much more complex system architecture
**Mitigation**:
- Testing with MCP integration
- Graceful fallback when MCP unavailable
- Clear documentation of MCP tool usage
- Monitoring of MCP server performance

## 📊 **Expected Impact**

### Accuracy Improvements
- **15-25% better decision quality** with real-time context
- **Reduced false positives** through state validation
- **Smarter resource allocation** based on actual capacity
- **Better handling of edge cases** with full cluster visibility

### New Capabilities Enabled
- **Predictive recommendations** based on cluster trends
- **Multi-resource correlation** for complex issues
- **Capacity-aware scaling** decisions
- **Historical pattern recognition** for recurring issues

## 🎯 **Recommendation**

**YES - This should absolutely be added to the roadmap!**

This MCP integration would:
1. **Differentiate our system** from any existing solutions
2. **Dramatically improve decision quality** with real-time context
3. **Enable advanced use cases** impossible with static prompts
4. **Future-proof the architecture** for more sophisticated AI operations

**Suggested Implementation**:
- Add as **Phase 1.4** in roadmap (high innovation value)
- Start with **Granite Dense 2B + 8B models** (most capable)
- **Parallel development** with model comparison testing
- **Gradual rollout** with fallback to non-MCP mode

This could be a differentiating feature for our system in the AI-powered infrastructure space! 🚀

## 📦 **Deployment Integration**

### Sample Deployment with External MCP Server
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus-alerts-slm
  namespace: monitoring
spec:
  replicas: 2
  selector:
    matchLabels:
      app: prometheus-alerts-slm
  template:
    metadata:
      labels:
        app: prometheus-alerts-slm
    spec:
      serviceAccountName: prometheus-alerts-slm
      containers:
      # Main application
      - name: prometheus-alerts-slm
        image: quay.io/jordigilh/prometheus-alerts-slm:latest
        ports:
        - containerPort: 8080
        env:
        - name: SLM_PROVIDER
          value: "localai"
        - name: SLM_ENDPOINT
          value: "http://ollama-service:11434"
        - name: MCP_K8S_SERVER
          value: "http://localhost:8080"
        - name: MCP_ACTION_HISTORY_SERVER
          value: "http://localhost:8081"
      # External Kubernetes MCP Server
      - name: k8s-mcp-server
        image: ghcr.io/containers/kubernetes-mcp-server:latest
        ports:
        - containerPort: 8080
        env:
        - name: MCP_PORT
          value: "8080"
        securityContext:
          runAsNonRoot: true
          readOnlyRootFilesystem: true
      # Our Custom Action History MCP Server
      - name: action-history-mcp
        image: quay.io/jordigilh/prometheus-alerts-slm-mcp:latest
        ports:
        - containerPort: 8081
        env:
        - name: MCP_PORT
          value: "8081"
        - name: DATABASE_HOST
          valueFrom:
            secretKeyRef:
              name: database-secret
              key: host
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: prometheus-alerts-slm
rules:
# For main application
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "patch", "update"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "delete"]
# For K8s MCP Server
- apiGroups: [""]
  resources: ["pods", "nodes", "namespaces", "events", "configmaps", "secrets"]
  verbs: ["get", "list"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list"]
- apiGroups: ["metrics.k8s.io"]
  resources: ["pods", "nodes"]
  verbs: ["get", "list"]
```

### Configuration Integration
```yaml
# config/app.yaml
slm:
  provider: localai
  endpoint: http://localhost:11434
  model: granite3.1-dense:8b

mcp:
  servers:
    kubernetes:
      endpoint: http://localhost:8080
      type: external
      provider: containers/kubernetes-mcp-server
    action_history:
      endpoint: http://localhost:8081
      type: internal
      provider: prometheus-alerts-slm
```

### Next Steps for Integration

#### Phase 1: Evaluation (1-2 weeks)
- [ ] **Deploy containers/kubernetes-mcp-server** in test environment
- [ ] **Test compatibility** with our cluster configuration
- [ ] **Evaluate tool capabilities** against our requirements
- [ ] **Performance benchmarking** with our workloads

#### Phase 2: Client Integration (2-3 weeks)
- [ ] **Extend MCP client** to support multiple server connections
- [ ] **Implement connection management** for external servers
- [ ] **Add tool discovery** for dynamic capability detection
- [ ] **Error handling** for external server failures

#### Phase 3: Production Deployment (1-2 weeks)
- [ ] **Update deployment manifests** with external MCP server
- [ ] **Configure RBAC** for external server access
- [ ] **Implement monitoring** for MCP server health
- [ ] **Documentation updates** for operators

## 🔄 **Hybrid MCP Action History Interface - IMPLEMENTED**

### Dual Response Format for LLM Processing

**Implementation**: The MCP Action History Server provides both structured JSON data and human-readable text in a single response, supporting different LLM processing needs.

### Response Structure
```go
type MCPToolResponse struct {
    Content []MCPContent `json:"content"`
}

type MCPContent struct {
    Type string      `json:"type"`  // "application/json" or "text"
    Text string      `json:"text,omitempty"`  // Human explanation
    Data interface{} `json:"data,omitempty"`  // Structured data
}
```

### Dual Response Format

#### **For LLM Processing:**
```json
{
  "content": [
    {
      "type": "application/json",
      "data": {
        "resource_info": {"namespace": "production", "kind": "Deployment", "name": "webapp"},
        "overall_severity": "medium",
        "confidence": 0.95,
        "is_safe": false,
        "scale_oscillation": {
          "direction_changes": 5,
          "avg_effectiveness": 0.3
        }
      }
    },
    {
      "type": "text",
      "text": "Scale Oscillation Detected: 5 direction changes with 30% average effectiveness. Recommend investigating root cause rather than continued scaling."
    }
  ]
}
```

#### **Benefits for AI Decision Making:**
1. **Precise Threshold Checks**: `if confidence >= 0.8 AND is_safe == false`
2. **Numeric Comparisons**: `scale_oscillation.direction_changes > 3`
3. **Boolean Logic**: `is_safe ? "proceed" : "block"`
4. **Array Processing**: Loop through `ineffective_loops[]` for pattern analysis
5. **Nested Access**: `scale_oscillation.avg_effectiveness < 0.5`

#### **Benefits for Human Understanding:**
- Natural language explanations in the `text` field
- Contextual reasoning for decisions
- User-friendly summaries for dashboards
- Clear safety recommendations

### Implementation in Action History Tools

**All MCP tools now provide hybrid responses:**

1. **`get_action_history`** - Action history with both structured metadata and readable summaries
2. **`analyze_oscillation_patterns`** - Pattern analysis with precise metrics + explanations
3. **`check_action_safety`** - Boolean safety decisions + detailed reasoning
4. **`get_action_effectiveness`** - Numerical effectiveness scores + trend analysis

### Model Usage Example

```
<tool_call>
{"tool": "check_action_safety", "parameters": {"namespace": "prod", "kind": "Deployment", "name": "webapp", "actionType": "scale_deployment"}}
</tool_call>

<tool_result>
{
  "content": [
    {
      "type": "application/json",
      "data": {
        "is_safe": false,
        "overall_severity": "high",
        "confidence": 0.92,
        "scale_oscillation": {"direction_changes": 7, "avg_effectiveness": 0.25}
      }
    },
    {
      "type": "text",
      "text": "⚠️ WARNING: HIGH severity oscillation patterns detected. Scale oscillation: 7 direction changes with 25% effectiveness. 🚫 RECOMMENDATION: Block this action to prevent further oscillation."
    }
  ]
}
</tool_result>

Based on structured analysis: confidence=0.92 > 0.8 AND is_safe=false AND direction_changes=7 > 5
Decision: BLOCK scaling action due to severe oscillation pattern
Reasoning: Multiple failed scaling attempts indicate root cause investigation needed
```

### **Impact on AI Operations**

This hybrid approach provides precision in AI decision-making:

- **Reduces ambiguity** - Boolean flags and numeric thresholds provide clear values
- **Enables complex logic** - Structured data supports reasoning patterns
- **Maintains explainability** - Human text provides context and reasoning
- **Supports automation** - Clear decision boundaries for programmatic responses
- **Reduces interpretation errors** - Structured data prevents model misunderstanding

This implementation makes the MCP interface suitable for both AI processing and human understanding.

---

*This analysis demonstrates how MCP integration could transform static alert responses into intelligent, context-aware cluster operations*