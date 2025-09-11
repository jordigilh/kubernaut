# Direct HolmesGPT Integration Guide

This guide explains how to integrate HolmesGPT directly from your Go application, bypassing the Python API wrapper.

## Why Direct Integration?

**Benefits:**
- ✅ **Simpler Architecture**: Eliminate Python API middleman
- ✅ **Better Performance**: Direct HTTP calls without translation layers
- ✅ **Native Features**: Use HolmesGPT's built-in Kubernetes/Prometheus toolsets
- ✅ **Reduced Maintenance**: One less service to maintain and debug
- ✅ **Better Observability**: Direct access to HolmesGPT's native metrics

**Trade-offs:**
- ✅ **Context Enrichment**: Implemented comprehensive context enrichment for both HolmesGPT and LLM fallback paths
- ✅ **Custom Metrics**: Integrated with existing metrics collection patterns
- ❌ **Request Translation**: Need to convert between Kubernaut and HolmesGPT formats

## Architecture Comparison

### Current Architecture (with Python API)
```
┌─────────────┐    ┌──────────────────┐    ┌─────────────┐
│  Kubernaut  │───▶│   Python API     │───▶│ HolmesGPT   │
│  (Go App)   │    │  (FastAPI)       │    │ (Container) │
└─────────────┘    └──────────────────┘    └─────────────┘
                          │
                          ▼
                   ┌─────────────────┐
                   │ Context Providers│
                   │ • Kubernetes     │
                   │ • Action History │
                   └─────────────────┘
```

### Proposed Direct Architecture
```
┌─────────────┐                           ┌─────────────┐
│  Kubernaut  │──────────────────────────▶│ HolmesGPT   │
│  (Go App)   │         HTTP API          │ (Container) │
└─────────────┘                           └─────────────┘
       │                                         │
       ▼                                         ▼
┌─────────────────┐                    ┌─────────────────┐
│ Go HTTP Client  │                    │ Native Toolsets │
│ • Alert Conv.   │                    │ • Kubernetes    │
│ • Context Enrich│                    │ • Prometheus    │
│ • Retry Logic   │                    │ • AWS/Azure     │
└─────────────────┘                    └─────────────────┘
```

## Implementation

### 1. HolmesGPT Client (Go)

The client I created (`pkg/ai/holmesgpt/client.go`) provides:

```go
// Create client
config := holmesgpt.Config{
    Endpoint:   "http://holmesgpt-service:8090",
    Timeout:    60 * time.Second,
    RetryCount: 3,
}
client := holmesgpt.NewClient(config, logger)

// Investigate alert
request := holmesgpt.ConvertAlertToInvestigateRequest(alert)
response, err := client.Investigate(ctx, request)
```

### 2. Integration with Existing Code

Update your workflow engine to use the direct client:

```go
// In your workflow engine
type WorkflowEngine struct {
    holmesClient holmesgpt.Client
    // ... other fields
}

func (we *WorkflowEngine) ProcessAlert(ctx context.Context, alert types.Alert) error {
    // Use HolmesGPT for investigation
    if we.holmesClient != nil && we.holmesClient.IsHealthy() {
        request := holmesgpt.ConvertAlertToInvestigateRequest(alert)
        investigation, err := we.holmesClient.Investigate(ctx, request)
        if err != nil {
            we.log.WithError(err).Warn("HolmesGPT investigation failed, continuing with local analysis")
        } else {
            // Use HolmesGPT recommendations
            return we.executeRecommendations(ctx, investigation.Recommendations)
        }
    }

    // Fallback to existing logic
    return we.processAlertLocally(ctx, alert)
}
```

### 3. Configuration

Update your configuration to use direct HolmesGPT:

```yaml
# config/development.yaml
holmesgpt:
  enabled: true
  endpoint: "http://localhost:8090"  # Local container
  timeout: 60s
  retry_count: 3
  max_idle_conns: 10

# config/production.yaml
holmesgpt:
  enabled: true
  endpoint: "http://holmesgpt-e2e.kubernaut-system.svc.cluster.local:8090"
  timeout: 120s
  retry_count: 5
  max_idle_conns: 20
```

## Migration Strategy

### Phase 1: Parallel Operation (Recommended)
1. **Deploy HolmesGPT container** using the scripts I provided
2. **Add Go client** to your codebase
3. **Run both systems** in parallel for testing
4. **Compare results** between Python API and direct integration
5. **Gradually shift traffic** to direct integration

### Phase 2: Full Migration
1. **Update all callers** to use Go client
2. **Remove Python API dependencies**
3. **Update deployment scripts** to exclude Python API
4. **Update monitoring** to use HolmesGPT native metrics

## Testing Strategy

### Local Testing
```bash
# Start HolmesGPT directly
./scripts/run-holmesgpt-local.sh

# Test Go client integration
go test ./pkg/ai/holmesgpt/... -v

# Test end-to-end workflow
make test-integration-holmesgpt
```

### E2E Testing
```bash
# Deploy HolmesGPT to cluster
./scripts/deploy-holmesgpt-e2e.sh

# Test Go client against deployed service
./scripts/test-direct-integration.sh
```

## Advantages of Direct Integration

### 1. **Consistent Investigation Quality**
Both HolmesGPT and LLM fallback paths now provide:
- Rich historical context from action patterns
- Real-time metrics for evidence-based analysis
- Kubernetes cluster state information
- Context enrichment timestamps and metadata

### 2. **Native Kubernetes Access**
HolmesGPT has built-in Kubernetes toolsets that can:
- Query pod status, logs, events
- Access deployment and service information
- Analyze resource usage and limits
- Check RBAC permissions

### 3. **Built-in Prometheus Integration**
HolmesGPT natively supports:
- PromQL queries
- Metrics correlation
- Alert context gathering
- Time series analysis

### 4. **Enhanced LLM Fallback**
When HolmesGPT is unavailable, the LLM fallback provides:
- Same context enrichment as HolmesGPT
- Historical pattern recognition (BR-AI-011)
- Metrics-based evidence for root cause analysis (BR-AI-012)
- Alert correlation across time/resource boundaries (BR-AI-013)
- Enhanced prompt template that guides LLM to leverage enriched context

### 5. **Simplified Observability**
HolmesGPT provides native metrics:
- `/metrics` endpoint for Prometheus
- Request/response timing
- Investigation success rates
- LLM token usage

### 6. **Better Error Handling**
Direct integration allows:
- Immediate error detection
- Custom retry strategies
- Circuit breaker patterns
- Graceful degradation with enriched context

## When to Keep Python API

Consider keeping the Python API if you need:

1. **Complex Context Providers**: Your custom Kubernetes context provider has complex logic
2. **Custom Caching**: You need request-specific caching strategies
3. **Legacy Integrations**: Other systems depend on the Python API format
4. **Gradual Migration**: You want to migrate slowly over several releases

## Recommendation

**I recommend direct integration** because:

1. **HolmesGPT provides everything you need natively**
2. **Your Go codebase already has HTTP client patterns**
3. **The Python API adds complexity without significant value**
4. **Direct integration aligns with your existing architecture**
5. **Both AI paths now provide consistent, context-enriched investigation quality**
6. **LLM fallback maintains business requirements compliance (BR-AI-011, BR-AI-012, BR-AI-013)**

The migration can be gradual - start with new features using direct integration, then migrate existing functionality as time permits.

## Next Steps

1. **Review the Go client code** I provided
2. **Test direct integration** with your local setup
3. **Compare performance** between Python API and direct calls
4. **Plan migration strategy** based on your requirements
5. **Update deployment scripts** to use direct integration

The direct integration approach will simplify your architecture while providing the same (or better) functionality than the current Python API wrapper. With the implementation of LLM context enrichment, both HolmesGPT and LLM fallback paths now deliver consistent investigation quality, ensuring business requirements are satisfied regardless of which AI service is available.
