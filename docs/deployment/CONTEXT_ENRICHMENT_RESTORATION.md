# Context Enrichment Restoration - Critical Missing Functionality

**Priority**: 🚨 **HIGH** - Core functionality lost during Phase 2 migration
**Impact**: Significant reduction in HolmesGPT investigation quality
**Status**: **MISSING** - Needs immediate implementation

---

## 🚨 **Problem Analysis**

### **What Was Lost in Phase 2 Migration**

The Python API provided sophisticated **Context Providers** that were **completely removed**:

```python
# REMOVED - Python API Context Enrichment
class KubernetesContextProvider:
    """Real-time cluster data collection"""
    - Pod status, resource usage, events
    - Node conditions, capacity, allocations
    - Deployment states, replica sets
    - Service endpoints, ingress configs
    - Resource quotas, limit ranges
    - Recent cluster events and warnings

class ActionHistoryContextProvider:
    """Historical action analysis"""
    - Past action success/failure patterns
    - Oscillation detection (repeated failures)
    - Context-specific effectiveness scores
    - Alternative action recommendations
    - Temporal pattern analysis
```

### **Current Go Implementation (INSUFFICIENT)**

```go
// pkg/ai/holmesgpt/client.go - Line 242
func (c *client) enrichContext(request *InvestigateRequest) *InvestigateRequest {
    // ONLY basic metadata - major functionality gap!
    request.Context["source"] = "kubernaut"
    request.Context["timestamp"] = time.Now().UTC()
    return request
}
```

---

## 🔧 **Required Implementation**

### **Step 1: Extend AIServiceIntegrator Structure**

```go
// pkg/workflow/engine/ai_service_integration.go
type AIServiceIntegrator struct {
    config        *config.Config
    llmClient     llm.Client
    holmesClient  holmesgpt.Client
    vectorDB      vector.VectorDatabase
    metricsClient *metrics.Client

    // ADD: Missing context sources
    k8sClient     interface{}    // Kubernetes cluster data
    actionRepo    interface{}    // Action history patterns

    log           *logrus.Logger
}
```

### **Step 2: Implement Context Enrichment Function**

```go
// enrichInvestigationContext - Restore Python API functionality
func (asi *AIServiceIntegrator) enrichInvestigationContext(
    ctx context.Context,
    request *holmesgpt.InvestigateRequest,
    alert types.Alert,
) (*holmesgpt.InvestigateRequest, error) {

    if request.Context == nil {
        request.Context = make(map[string]interface{})
    }

    // 1. Kubernetes Context Enrichment (replacing KubernetesContextProvider)
    if asi.k8sClient != nil {
        k8sContext, err := asi.gatherKubernetesContext(ctx, alert)
        if err == nil {
            request.Context["kubernetes"] = k8sContext
        }
    }

    // 2. Action History Context (replacing ActionHistoryContextProvider)
    if asi.actionRepo != nil {
        actionContext, err := asi.gatherActionHistoryContext(ctx, alert)
        if err == nil {
            request.Context["action_history"] = actionContext
        }
    }

    // 3. Metrics Context (current cluster performance)
    if asi.metricsClient != nil {
        metricsContext, err := asi.gatherMetricsContext(ctx, alert)
        if err == nil {
            request.Context["metrics"] = metricsContext
        }
    }

    // 4. Vector Database Pattern Context
    if asi.vectorDB != nil {
        patternContext, err := asi.gatherPatternContext(ctx, alert)
        if err == nil {
            request.Context["patterns"] = patternContext
        }
    }

    return request, nil
}
```

### **Step 3: Context Gathering Methods**

```go
// gatherKubernetesContext - Replace KubernetesContextProvider
func (asi *AIServiceIntegrator) gatherKubernetesContext(ctx context.Context, alert types.Alert) (map[string]interface{}, error) {
    context := make(map[string]interface{})

    // Pod information
    if alert.Namespace != "" && alert.Resource != "" {
        context["namespace"] = alert.Namespace
        context["resource_name"] = alert.Resource

        // TODO: Get pod status, resource usage, events
        // TODO: Get related deployments, services
        // TODO: Get resource quotas, limits
    }

    // Node information
    // TODO: Get node conditions, capacity

    // Recent events
    // TODO: Get cluster events for context

    return context, nil
}

// gatherActionHistoryContext - Replace ActionHistoryContextProvider
func (asi *AIServiceIntegrator) gatherActionHistoryContext(ctx context.Context, alert types.Alert) (map[string]interface{}, error) {
    context := make(map[string]interface{})

    // Historical patterns for this alert type
    // TODO: Query action history repository
    // TODO: Calculate success rates by action type
    // TODO: Detect oscillation patterns
    // TODO: Get alternative recommendations

    return context, nil
}

// gatherMetricsContext - Current performance data
func (asi *AIServiceIntegrator) gatherMetricsContext(ctx context.Context, alert types.Alert) (map[string]interface{}, error) {
    context := make(map[string]interface{})

    if asi.metricsClient != nil && alert.Namespace != "" {
        // Get current resource metrics
        metrics, err := asi.metricsClient.GetResourceMetrics(ctx, alert.Namespace, alert.Resource, 5*time.Minute)
        if err == nil {
            context["current_metrics"] = metrics
        }
    }

    return context, nil
}

// gatherPatternContext - Vector database similarity search
func (asi *AIServiceIntegrator) gatherPatternContext(ctx context.Context, alert types.Alert) (map[string]interface{}, error) {
    context := make(map[string]interface{})

    // TODO: Vector similarity search for similar alerts
    // TODO: Get historical resolution patterns
    // TODO: Extract learning insights

    return context, nil
}
```

### **Step 4: Update investigateWithHolmesGPT**

```go
// investigateWithHolmesGPT - Updated with context enrichment
func (asi *AIServiceIntegrator) investigateWithHolmesGPT(ctx context.Context, alert types.Alert) (*InvestigationResult, error) {
    // Convert alert to HolmesGPT request format
    request := holmesgpt.ConvertAlertToInvestigateRequest(alert)

    // 🔥 RESTORE: Context enrichment (replacing Python API functionality)
    enrichedRequest, err := asi.enrichInvestigationContext(ctx, request, alert)
    if err != nil {
        asi.log.WithError(err).Warn("Context enrichment failed, proceeding with basic request")
        enrichedRequest = request // Fallback to basic request
    }

    // Perform investigation with enriched context
    response, err := asi.holmesClient.Investigate(ctx, enrichedRequest)
    if err != nil {
        return nil, fmt.Errorf("holmesGPT investigation failed: %w", err)
    }

    // Convert response to our format
    return &InvestigationResult{
        Method:          "holmesgpt_enriched", // Distinguish enriched investigations
        Analysis:        response.Analysis,
        Recommendations: convertHolmesRecommendations(response.Recommendations),
        Confidence:      response.Confidence,
        ProcessingTime:  response.ProcessingTime,
        Source:          "HolmesGPT v0.13.1 (Kubernaut Context)",
        Context:         response.Context,
    }, nil
}
```

---

## 📊 **Expected Quality Improvement**

### **Before (Current State)**
```json
{
  "query": "Investigate warning alert: Pod memory usage high",
  "context": {
    "source": "kubernaut",
    "timestamp": "2025-01-15T10:30:00Z",
    "alert_name": "HighMemoryUsage"
  },
  "toolsets": ["kubernetes"]
}
```

### **After (With Context Enrichment)**
```json
{
  "query": "Investigate warning alert: Pod memory usage high",
  "context": {
    "source": "kubernaut",
    "timestamp": "2025-01-15T10:30:00Z",
    "alert_name": "HighMemoryUsage",
    "kubernetes": {
      "namespace": "production",
      "pod_status": "Running",
      "resource_usage": {"memory": "850Mi/1Gi", "cpu": "200m/500m"},
      "recent_events": ["BackOff", "Killing"],
      "deployment_replicas": "3/3"
    },
    "action_history": {
      "restart_pod_success_rate": 0.75,
      "scale_deployment_success_rate": 0.92,
      "recent_oscillation": false,
      "recommended_alternatives": ["increase_memory_limit", "scale_deployment"]
    },
    "metrics": {
      "memory_trend": "increasing",
      "cpu_utilization": 0.4,
      "network_errors": 0
    },
    "patterns": {
      "similar_cases": 15,
      "common_resolution": "scale_deployment",
      "pattern_confidence": 0.89
    }
  }
}
```

---

## ⚡ **Implementation Priority**

### **Phase 1: Critical Foundation**
1. ✅ Update `AIServiceIntegrator` constructor to accept k8sClient and actionRepo
2. ✅ Implement basic `enrichInvestigationContext` function
3. ✅ Update `investigateWithHolmesGPT` to use enriched context

### **Phase 2: Context Providers**
4. 🔄 Implement `gatherKubernetesContext` (pod status, events, resources)
5. 🔄 Implement `gatherActionHistoryContext` (success rates, patterns)
6. 🔄 Implement `gatherMetricsContext` (current performance data)

### **Phase 3: Advanced Features**
7. 🔄 Implement `gatherPatternContext` (vector similarity search)
8. 🔄 Add oscillation detection logic
9. 🔄 Add context caching for performance

---

## 🧪 **Testing Strategy**

### **Unit Tests**
- Test each context gathering method independently
- Test enrichment with various alert types
- Test fallback behavior when context gathering fails

### **Integration Tests**
- Compare investigation quality before/after enrichment
- Test with real Kubernetes cluster data
- Verify performance impact of context gathering

### **E2E Validation**
- Measure improvement in HolmesGPT recommendation accuracy
- Validate context data appears in HolmesGPT logs
- Test edge cases (missing data, permissions, timeouts)

---

## 🎯 **Success Criteria**

- ✅ **Context Completeness**: All 4 context types (K8s, ActionHistory, Metrics, Patterns) populated
- ✅ **Performance**: Context enrichment completes within 2 seconds
- ✅ **Quality**: Measurable improvement in HolmesGPT recommendation accuracy
- ✅ **Reliability**: Graceful fallback when context gathering fails
- ✅ **Observability**: Context enrichment status visible in logs/metrics

**🚨 This is critical missing functionality that significantly impacts HolmesGPT's effectiveness!**
