# AI Context Orchestration Architecture

## Overview

This document describes the AI Context Orchestration architecture for the Kubernaut system, enabling intelligent, dynamic context gathering for AI-driven investigations. This architecture evolves the system from static context injection to AI-orchestrated, on-demand context retrieval, allowing AI services to fetch precisely the context data needed for each investigation.

## Business Requirements Addressed

- **BR-CONTEXT-001 to BR-CONTEXT-043**: Dynamic context orchestration and optimization
- **BR-HOLMES-025 to BR-HOLMES-030**: HolmesGPT toolset integration patterns
- **BR-API-015 to BR-API-025**: Context API services and integration
- **BR-PERF-010 to BR-PERF-015**: Context performance optimization
- **BR-AI-011 to BR-AI-015**: AI-powered investigation capabilities

## Architecture Principles

### Design Philosophy
- **AI-Driven Context Discovery**: AI services dynamically discover and request needed context
- **On-Demand Retrieval**: Context data fetched only when needed, reducing resource overhead
- **Intelligent Caching**: Multi-level caching with context-aware invalidation strategies
- **Performance Optimization**: 40-60% improvement in investigation efficiency
- **Scalable Integration**: Zero-configuration adaptability to cluster environments

## System Architecture Overview

### High-Level Architecture

```ascii
┌─────────────────────────────────────────────────────────────────┐
│                  AI CONTEXT ORCHESTRATION                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ AI Investigation Services                                       │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ HolmesGPT       │  │ Direct LLM      │  │ Rule-based      │ │
│ │ Investigation   │  │ Analysis        │  │ Fallback        │ │
│ │ (Primary)       │  │ (Secondary)     │  │ (Tertiary)      │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│          │                     │                     │         │
│          ▼                     ▼                     ▼         │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │              Context Orchestration Layer                   │ │
│ │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│ │  │ Discovery   │  │ Cache       │  │ Performance         │ │ │
│ │  │ Engine      │  │ Manager     │  │ Optimizer           │ │ │
│ │  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│ └─────────────────────────────────────────────────────────────┘ │
│          │                     │                     │         │
│          ▼                     ▼                     ▼         │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │                  Context API Layer                         │ │
│ │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│ │  │ REST API    │  │ Validation  │  │ Response            │ │ │
│ │  │ Endpoints   │  │ & Security  │  │ Optimization        │ │ │
│ │  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│ └─────────────────────────────────────────────────────────────┘ │
│          │                     │                     │         │
│          ▼                     ▼                     ▼         │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │                Context Data Sources                         │ │
│ │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│ │  │ Kubernetes  │  │ Metrics     │  │ Action History      │ │ │
│ │  │ Cluster     │  │ (Prometheus)│  │ & Patterns          │ │ │
│ │  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│ │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │ │
│ │  │ Logs &      │  │ Events &    │  │ Vector Database     │ │ │
│ │  │ Traces      │  │ Audit Logs  │  │ (Similarity)        │ │ │
│ │  └─────────────┘  └─────────────┘  └─────────────────────┘ │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Context Discovery Engine

**Purpose**: Enable AI services to dynamically discover available context types for investigation scenarios.

**Key Capabilities**:
- Dynamic context type discovery based on investigation goals
- Context metadata provision (freshness, relevance, costs)
- Context dependency resolution for complex workflows
- Usage pattern tracking for optimization

**Architecture**:
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                  CONTEXT DISCOVERY ENGINE                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Discovery API                                                   │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ GET /api/v1/context/discover                               │ │
│ │ ?alertType={type}&namespace={ns}&investigationType={type}  │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Context Type Registry                                           │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ kubernetes      │  │ metrics         │  │ action-history  │ │
│ │ Priority: 100   │  │ Priority: 85    │  │ Priority: 60    │ │
│ │ Cost: 50ms      │  │ Cost: 150ms     │  │ Cost: 100ms     │ │
│ │ Relevance: 0.9  │  │ Relevance: 0.85 │  │ Relevance: 0.7  │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ logs            │  │ patterns        │  │ events          │ │
│ │ Priority: 70    │  │ Priority: 40    │  │ Priority: 30    │ │
│ │ Cost: 120ms     │  │ Cost: 200ms     │  │ Cost: 80ms      │ │
│ │ Relevance: 0.75 │  │ Relevance: 0.6  │  │ Relevance: 0.5  │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Dynamic Prioritization                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Alert type analysis (SecurityBreach → boost all types)   │ │
│ │ • Investigation complexity assessment                       │ │
│ │ • Resource constraint consideration                         │ │
│ │ • Historical effectiveness scoring                          │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation** (`pkg/api/context/context_controller.go:1019-1103`):
```go
func (cc *ContextController) DiscoverContextTypes(w http.ResponseWriter, r *http.Request) {
    // Dynamic context type discovery with intelligent prioritization
    alertType := r.URL.Query().Get("alertType")
    namespace := r.URL.Query().Get("namespace")
    investigationType := r.URL.Query().Get("investigationType")

    // Get available context types with dynamic relevance scoring
    availableTypes := cc.discovery.GetAvailableTypes(alertType, namespace)

    // Apply business requirement optimizations
    if investigationType != "" && r.URL.Query().Get("scoreSufficiency") == "true" {
        availableTypes = cc.ensureInvestigationRequirements(availableTypes, investigationType, alertType, namespace)
    }

    response := ContextDiscoveryResponse{
        AvailableTypes: availableTypes,
        TotalTypes:     len(availableTypes),
        Timestamp:      time.Now().UTC(),
    }

    cc.writeJSONResponse(w, http.StatusOK, response)
}
```

### 2. Context Cache Manager

**Purpose**: Provide intelligent caching with context-aware invalidation strategies to achieve 80%+ cache hit rates.

**Multi-Level Caching Strategy**:
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                    MULTI-LEVEL CACHING                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Level 1: In-Memory Cache (L1)                                  │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • 5-minute default TTL                                      │ │
│ │ • Context type specific TTL (kubernetes: 30s, metrics: 1m) │ │
│ │ │ • LRU eviction policy                                     │ │
│ │ • >95% hit rate for repeated queries                       │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Level 2: Context Data Cache (L2)                               │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • 10-minute maximum TTL                                     │ │
│ │ • Content-aware invalidation                               │ │
│ │ • Compressed storage for large contexts                    │ │
│ │ • 80%+ hit rate target (BR-CONTEXT-010)                   │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Level 3: Distributed Cache (L3) - Future Enhancement          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Redis/Memcached integration                              │ │
│ │ • Cross-instance cache sharing                             │ │
│ │ • Persistent cache with durability                        │ │
│ │ • 99.9% availability target                               │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation** (`pkg/api/context/context_controller.go:726-827`):
```go
type ContextCache struct {
    data       map[string]*CacheEntry
    defaultTTL time.Duration
    maxTTL     time.Duration
    mu         sync.RWMutex
    hitCount   int64
    missCount  int64
}

func (c *ContextCache) Get(key string) (interface{}, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    entry, exists := c.data[key]
    if !exists {
        c.missCount++
        return nil, false
    }

    // Check if expired
    if time.Since(entry.Timestamp) > entry.TTL {
        c.missCount++
        return nil, false
    }

    c.hitCount++
    return entry.Data, true
}

func (c *ContextCache) GetHitRate() float64 {
    c.mu.RLock()
    defer c.mu.RUnlock()

    total := c.hitCount + c.missCount
    if total == 0 {
        return 0.0
    }
    return float64(c.hitCount) / float64(total)
}
```

### 3. Context API Layer

**Purpose**: Provide RESTful endpoints for real-time context access with performance optimization.

**API Endpoints Architecture**:
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                       CONTEXT API ENDPOINTS                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Core Context Endpoints                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ GET /api/v1/context/discover                               │ │
│ │ GET /api/v1/context/kubernetes/{namespace}/{resource}     │ │
│ │ GET /api/v1/context/metrics/{namespace}/{resource}        │ │
│ │ GET /api/v1/context/action-history/{alertType}            │ │
│ │ GET /api/v1/context/patterns/{signature}                  │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Optimization Endpoints                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ GET /api/v1/context/alert/{severity}/{alertName}          │ │
│ │ GET /api/v1/context/investigation/{investigationType}     │ │
│ │ POST /api/v1/context/monitor/llm-performance              │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Toolset Management Endpoints                                    │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ GET /api/v1/toolsets                                       │ │
│ │ GET /api/v1/toolsets/stats                                 │ │
│ │ POST /api/v1/toolsets/refresh                              │ │
│ │ GET /api/v1/service-discovery                              │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Health Monitoring Endpoints                                     │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ GET /api/v1/context/health                                 │ │
│ │ GET /api/v1/health/llm                                     │ │
│ │ GET /api/v1/health/llm/liveness                            │ │
│ │ GET /api/v1/health/llm/readiness                           │ │
│ │ GET /api/v1/health/dependencies                            │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Performance Targets**:
- **Cached Results**: <100ms response time (95% of requests)
- **Fresh Results**: <500ms response time (95% of requests)
- **Cache Hit Rate**: >80% across all context types
- **Concurrent Requests**: 1000+ simultaneous context API calls

### 4. Performance Optimization Layer

**Purpose**: Achieve 40-60% improvement in investigation efficiency through intelligent context optimization.

**Optimization Strategies**:
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                  PERFORMANCE OPTIMIZATION                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Complexity Assessment                                           │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Simple (0.75 reduction):   Basic pod restart scenarios     │ │
│ │ Moderate (0.55 reduction): Standard performance issues     │ │
│ │ Complex (0.25 reduction):  Multi-service dependencies      │ │
│ │ Critical (0.05 reduction): Security incidents             │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Graduated Context Reduction                                     │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Alert type classification                                 │ │
│ │ • Context relevance scoring                                 │ │
│ │ • Dynamic context type selection                           │ │
│ │ • Payload size optimization                                │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Adequacy Validation                                             │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Context sufficiency scoring                               │ │
│ │ • Missing context identification                            │ │
│ │ • Enrichment requirement assessment                         │ │
│ │ • Investigation quality prediction                          │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Performance Monitoring                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Response time tracking                                    │ │
│ │ • Context size monitoring                                   │ │
│ │ • Investigation effectiveness measurement                   │ │
│ │ • Automatic adjustment triggers                             │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation** (`pkg/ai/context/optimization_service.go`):
```go
type OptimizationService struct {
    config              *config.ContextOptimizationConfig
    complexityClassifier *ComplexityClassifier
    adequacyValidator   *AdequacyValidator
    performanceMonitor  *PerformanceMonitor
    log                 *logrus.Logger
}

func (os *OptimizationService) OptimizeContext(ctx context.Context, complexity *ComplexityAssessment, baseContext *ContextData) (*ContextData, error) {
    // Apply graduated optimization based on complexity tier
    optimizedContext := os.applyGraduatedReduction(complexity, baseContext)

    // Validate adequacy after optimization
    adequacy, err := os.ValidateAdequacy(ctx, optimizedContext, complexity.AlertType)
    if err != nil {
        return nil, fmt.Errorf("adequacy validation failed: %w", err)
    }

    // Apply enrichment if needed
    if adequacy.EnrichmentRequired {
        optimizedContext = os.enrichContext(optimizedContext, adequacy.MissingContextTypes)
    }

    return optimizedContext, nil
}
```

## Integration Patterns

### HolmesGPT Toolset Integration

**Purpose**: Enable HolmesGPT to dynamically orchestrate context gathering through custom toolsets.

**Integration Architecture**:
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                  HOLMESGPT INTEGRATION                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ HolmesGPT Investigation Process                                 │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ 1. Receive investigation request                            │ │
│ │ 2. Discover available context types                         │ │
│ │ 3. Select relevant context based on investigation goals     │ │
│ │ 4. Fetch context data dynamically                          │ │
│ │ 5. Perform AI-powered analysis                             │ │
│ │ 6. Generate recommendations                                 │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Dynamic Toolset Configuration                                   │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ • Automatic toolset generation based on cluster services   │ │
│ │ • Context API endpoint discovery                           │ │
│ │ • Service health validation                                │ │
│ │ • Configuration template population                        │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼                                  │
│ Toolset Management API                                          │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ GET /api/v1/toolsets          → Available toolsets         │ │
│ │ GET /api/v1/toolsets/stats    → Toolset statistics         │ │
│ │ POST /api/v1/toolsets/refresh → Force refresh              │ │
│ │ GET /api/v1/service-discovery → Service discovery status   │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation** (`pkg/api/context/context_controller.go:1324-1488`):
```go
func (cc *ContextController) GetAvailableToolsets(w http.ResponseWriter, r *http.Request) {
    // Get available toolsets from service integration
    toolsets := cc.serviceIntegration.GetAvailableToolsets()

    // Convert to API response format
    toolsetResponses := make([]map[string]interface{}, len(toolsets))
    for i, toolset := range toolsets {
        toolsetResponses[i] = map[string]interface{}{
            "name":         toolset.Name,
            "service_type": toolset.ServiceType,
            "description":  toolset.Description,
            "version":      toolset.Version,
            "capabilities": toolset.Capabilities,
            "enabled":      toolset.Enabled,
            "priority":     toolset.Priority,
            "last_updated": toolset.LastUpdated,
        }
    }

    response := map[string]interface{}{
        "toolsets":  toolsetResponses,
        "count":     len(toolsets),
        "timestamp": time.Now().UTC(),
    }

    cc.writeJSONResponse(w, http.StatusOK, response)
}
```

### Context Data Flow

**End-to-End Context Orchestration Flow**:
```ascii
┌─────────────────────────────────────────────────────────────────┐
│               CONTEXT ORCHESTRATION FLOW                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ 1. Investigation Request                                        │
│    ┌─────────────────┐                                         │
│    │ Alert Received  │                                         │
│    │ AlertType: Pod  │                                         │
│    │ Namespace: prod │                                         │
│    └─────────────────┘                                         │
│              │                                                  │
│              ▼                                                  │
│ 2. Context Discovery                                            │
│    ┌─────────────────────────────────────────────────────────┐ │
│    │ GET /api/v1/context/discover                           │ │
│    │ ?alertType=PodCrashLoop&namespace=prod&                │ │
│    │  investigationType=root_cause_analysis                │ │
│    └─────────────────────────────────────────────────────────┘ │
│              │                                                  │
│              ▼                                                  │
│ 3. Context Type Selection                                       │
│    ┌─────────────────────────────────────────────────────────┐ │
│    │ Response: [                                            │ │
│    │   {name: "kubernetes", priority: 100, cost: 50ms},    │ │
│    │   {name: "metrics", priority: 85, cost: 150ms},       │ │
│    │   {name: "action-history", priority: 60, cost: 100ms} │ │
│    │ ]                                                      │ │
│    └─────────────────────────────────────────────────────────┘ │
│              │                                                  │
│              ▼                                                  │
│ 4. Parallel Context Fetching                                   │
│    ┌─────────────────┐  ┌─────────────────┐  ┌─────────────┐ │
│    │ GET /api/v1/    │  │ GET /api/v1/    │  │ GET /api/v1/│ │
│    │ context/        │  │ context/        │  │ context/    │ │
│    │ kubernetes/     │  │ metrics/        │  │ action-     │ │
│    │ prod/pod        │  │ prod/pod        │  │ history/    │ │
│    └─────────────────┘  └─────────────────┘  │ PodCrash    │ │
│              │                    │           │ Loop        │ │
│              ▼                    ▼           └─────────────┘ │
│ 5. Context Aggregation & Optimization                          │
│    ┌─────────────────────────────────────────────────────────┐ │
│    │ • Merge context data from multiple sources             │ │
│    │ • Apply complexity-based optimization                  │ │
│    │ • Validate context adequacy                            │ │
│    │ • Compress and format for AI consumption               │ │
│    └─────────────────────────────────────────────────────────┘ │
│              │                                                  │
│              ▼                                                  │
│ 6. AI Investigation                                             │
│    ┌─────────────────────────────────────────────────────────┐ │
│    │ • Enhanced context enables precise analysis            │ │
│    │ • 40-60% faster investigation time                     │ │
│    │ • Higher accuracy recommendations                      │ │
│    │ • Reduced false positive rate                          │ │
│    └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Performance Characteristics

### Response Time Requirements
- **Context Discovery**: <50ms for context type enumeration
- **Kubernetes Context**: <100ms for cached cluster data, <500ms for fresh data
- **Metrics Context**: <150ms for cached metrics, <800ms for fresh Prometheus queries
- **Action History Context**: <100ms for pattern retrieval
- **Overall Investigation**: 40-60% faster than static enrichment

### Scalability Targets
- **Concurrent Context Requests**: 1000+ simultaneous API calls
- **Context Cache Hit Rate**: >80% across all context types
- **Memory Utilization**: 50-70% reduction vs. static pre-enrichment
- **Network Payload**: 40-60% reduction in investigation data transfer

### Business Value Metrics
- **Investigation Efficiency**: 40-60% improvement in investigation time
- **Setup Complexity**: 50-70% reduction through zero-configuration adaptability
- **Context Relevance**: 15-25% improvement in investigation accuracy
- **Resource Utilization**: 30-50% reduction in memory and network usage

## Error Handling and Resilience

### Fallback Strategies
1. **Context Discovery Failures**: Return cached context types with degraded metadata
2. **Context Fetch Failures**: Graceful degradation to available context sources
3. **Cache Misses**: Transparent fallback to fresh data retrieval
4. **Performance Degradation**: Automatic adjustment of context optimization parameters

### Circuit Breaker Patterns
- **Context API Protection**: Prevent cascade failures from context source outages
- **Cache Circuit Breaker**: Bypass caching when cache service is degraded
- **Performance Circuit Breaker**: Automatic context reduction under high load

## Security Considerations

### Authentication and Authorization
- **Bearer Token Validation**: All context API endpoints require valid authentication
- **RBAC Integration**: Context access controlled by Kubernetes RBAC permissions
- **Namespace Isolation**: Context data scoped to authorized namespaces
- **Audit Logging**: Complete audit trail for all context access

### Data Protection
- **Context Data Sanitization**: Removal of sensitive information from context responses
- **Encryption in Transit**: TLS encryption for all context API communications
- **Cache Security**: Encrypted cache storage and secure cache key generation
- **Rate Limiting**: Protection against context API abuse

## Future Enhancements

### Planned Improvements
- **Multi-Cluster Context**: Cross-cluster context aggregation and correlation
- **ML-Driven Optimization**: Machine learning for context relevance prediction
- **Advanced Caching**: Distributed cache with cross-instance sharing
- **Context Templates**: Pre-configured context bundles for common scenarios

### Research Areas
- **Predictive Context**: Proactive context pre-fetching based on usage patterns
- **Context Compression**: Advanced compression algorithms for large context payloads
- **Edge Computing**: Distributed context processing for reduced latency
- **Natural Language Context**: Conversational context discovery and selection

---

## Related Documentation

- [Alert Processing Flow](ALERT_PROCESSING_FLOW.md)
- [HolmesGPT REST API Architecture](HOLMESGPT_REST_API_ARCHITECTURE.md)
- [Dynamic Toolset Configuration Architecture](DYNAMIC_TOOLSET_CONFIGURATION_ARCHITECTURE.md)
- [Production Monitoring](PRODUCTION_MONITORING.md)
- [Resilience Patterns](RESILIENCE_PATTERNS.md)

---

*This document describes the AI Context Orchestration architecture for Kubernaut, enabling intelligent, dynamic context gathering for AI-driven investigations. The architecture supports the business requirements for context optimization and performance improvement while maintaining system resilience and security.*