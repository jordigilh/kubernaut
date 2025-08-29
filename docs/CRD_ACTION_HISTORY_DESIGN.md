# Hybrid Database + CRD Design for Action History Storage

**Objective**: Design scalable persistent storage system for AI action tracking and oscillation detection  
**Scope**: Primary database storage with optional CRD integration for Kubernetes-native visibility  
**Target**: Foundation for loop prevention and intelligent decision making at enterprise scale

## 🎯 **Design Overview - Hybrid Architecture**

### **Core Concept**
**Primary Storage**: Relational database for scalable, high-performance action history  
**Secondary Integration**: Lightweight CRDs for Kubernetes-native visibility and management

### **Why Database-First Approach?**
✅ **Scalability**: Handle millions of action records without ETCD pressure  
✅ **Performance**: Optimized queries with proper indexing and relationships  
✅ **Complex Analytics**: Advanced SQL queries for pattern detection and analysis  
✅ **Data Retention**: Efficient archiving and purging capabilities  
✅ **ACID Compliance**: Guaranteed data consistency for critical action tracking  
✅ **Reporting**: Native integration with business intelligence tools  

### **Hybrid Architecture**
```
PostgreSQL Database (Primary) ──┐
├── action_history               │
├── resource_action_traces       │  
├── oscillation_patterns         │
├── action_effectiveness         │
└── retention_policies           │
                                 │
Kubernetes CRDs (Secondary) ─────┤
├── ActionHistorySummary         │ ← Lightweight summaries
├── OscillationAlert             │ ← Active pattern alerts  
└── EffectivenessMetric          │ ← Current metric snapshots
```

## 📋 **Primary CRDs**

### **1. ActionHistory CRD**
**Purpose**: Store comprehensive action history for each Kubernetes resource

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: actionhistories.ai.prometheus-alerts-slm.io
spec:
  group: ai.prometheus-alerts-slm.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              resourceReference:
                type: object
                properties:
                  apiVersion:
                    type: string
                    description: "API version of the target resource"
                  kind:
                    type: string
                    description: "Kind of the target resource (Deployment, Pod, etc.)"
                  name:
                    type: string
                    description: "Name of the target resource"
                  namespace:
                    type: string
                    description: "Namespace of the target resource"
                  uid:
                    type: string
                    description: "UID of the target resource for lifecycle tracking"
                required: [apiVersion, kind, name, namespace, uid]
              
              retentionPolicy:
                type: object
                properties:
                  maxActions:
                    type: integer
                    minimum: 10
                    maximum: 1000
                    default: 100
                    description: "Maximum number of actions to retain"
                  maxAge:
                    type: string
                    pattern: '^(\d+h|\d+d|\d+w)$'
                    default: "30d"
                    description: "Maximum age of actions to retain (e.g., 30d, 24h)"
                  compactionStrategy:
                    type: string
                    enum: ["oldest-first", "effectiveness-based", "pattern-aware"]
                    default: "pattern-aware"
                    description: "Strategy for removing old actions"
              
              analysisConfig:
                type: object
                properties:
                  oscillationWindow:
                    type: string
                    pattern: '^(\d+m|\d+h|\d+d)$'
                    default: "2h"
                    description: "Time window for oscillation detection"
                  effectivenessThreshold:
                    type: number
                    minimum: 0.0
                    maximum: 1.0
                    default: 0.7
                    description: "Minimum effectiveness score to consider action successful"
                  patternMinOccurrences:
                    type: integer
                    minimum: 2
                    default: 3
                    description: "Minimum occurrences to classify as a pattern"
          
          status:
            type: object
            properties:
              totalActions:
                type: integer
                description: "Total number of actions recorded"
              
              lastActionTime:
                type: string
                format: date-time
                description: "Timestamp of the most recent action"
              
              detectedPatterns:
                type: array
                items:
                  type: object
                  properties:
                    patternType:
                      type: string
                      enum: ["scale-oscillation", "resource-thrashing", "ineffective-loop", "cascading-failure"]
                    severity:
                      type: string
                      enum: ["low", "medium", "high", "critical"]
                    occurrences:
                      type: integer
                    lastSeen:
                      type: string
                      format: date-time
                    confidence:
                      type: number
                      minimum: 0.0
                      maximum: 1.0
              
              effectivenessMetrics:
                type: object
                properties:
                  overallScore:
                    type: number
                    minimum: 0.0
                    maximum: 1.0
                  actionTypeScores:
                    type: object
                    additionalProperties:
                      type: number
                  recentTrend:
                    type: string
                    enum: ["improving", "stable", "declining"]
              
              analysisStatus:
                type: object
                properties:
                  lastAnalyzed:
                    type: string
                    format: date-time
                  nextAnalysis:
                    type: string
                    format: date-time
                  status:
                    type: string
                    enum: ["pending", "analyzing", "completed", "error"]
                  errorMessage:
                    type: string
  scope: Namespaced
  names:
    plural: actionhistories
    singular: actionhistory
    kind: ActionHistory
    shortNames: ["ah", "actionhist"]
```

### **2. ResourceActionTrace CRD**
**Purpose**: Store individual action records with full context and outcomes

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: resourceactiontraces.ai.prometheus-alerts-slm.io
spec:
  group: ai.prometheus-alerts-slm.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              actionHistoryRef:
                type: object
                properties:
                  name:
                    type: string
                  namespace:
                    type: string
                required: [name, namespace]
              
              actionMetadata:
                type: object
                properties:
                  actionId:
                    type: string
                    description: "Unique identifier for this action"
                  correlationId:
                    type: string
                    description: "Correlation ID for tracing across systems"
                  timestamp:
                    type: string
                    format: date-time
                    description: "When the action was initiated"
                  modelUsed:
                    type: string
                    description: "AI model that recommended this action"
                  routingTier:
                    type: string
                    enum: ["route1", "route2", "route3"]
                    description: "Multi-modal routing tier used"
                required: [actionId, timestamp, modelUsed]
              
              alertContext:
                type: object
                properties:
                  alertName:
                    type: string
                    description: "Name of the Prometheus alert that triggered this action"
                  severity:
                    type: string
                    enum: ["info", "warning", "critical"]
                  labels:
                    type: object
                    additionalProperties:
                      type: string
                  annotations:
                    type: object
                    additionalProperties:
                      type: string
                  firingTime:
                    type: string
                    format: date-time
                required: [alertName, severity]
              
              actionDetails:
                type: object
                properties:
                  actionType:
                    type: string
                    enum: [
                      "scale_deployment", "restart_pod", "rollback_deployment",
                      "expand_pvc", "drain_node", "quarantine_pod", 
                      "collect_diagnostics", "notify_only", "increase_resources"
                    ]
                  parameters:
                    type: object
                    additionalProperties:
                      type: string
                    description: "Action-specific parameters (replicas, memory, etc.)"
                  confidence:
                    type: number
                    minimum: 0.0
                    maximum: 1.0
                    description: "AI model confidence in this recommendation"
                  reasoning:
                    type: string
                    description: "AI model's reasoning for this action"
                  alternatives:
                    type: array
                    items:
                      type: object
                      properties:
                        actionType:
                          type: string
                        confidence:
                          type: number
                        reasoning:
                          type: string
                required: [actionType, confidence]
              
              resourceState:
                type: object
                properties:
                  before:
                    type: object
                    properties:
                      resourceVersion:
                        type: string
                      generation:
                        type: integer
                      replicas:
                        type: integer
                      resources:
                        type: object
                        additionalProperties:
                          type: string
                      conditions:
                        type: array
                        items:
                          type: object
                          additionalProperties:
                            type: string
                  
                  after:
                    type: object
                    properties:
                      resourceVersion:
                        type: string
                      generation:
                        type: integer
                      replicas:
                        type: integer
                      resources:
                        type: object
                        additionalProperties:
                          type: string
                      conditions:
                        type: array
                        items:
                          type: object
                          additionalProperties:
                            type: string
          
          status:
            type: object
            properties:
              executionStatus:
                type: string
                enum: ["pending", "executing", "completed", "failed", "rolled-back"]
                description: "Current status of action execution"
              
              executionDetails:
                type: object
                properties:
                  startTime:
                    type: string
                    format: date-time
                  endTime:
                    type: string
                    format: date-time
                  duration:
                    type: string
                    pattern: '^(\d+(\.\d+)?(ns|us|ms|s|m|h))$'
                  error:
                    type: string
                  kubernetesOperations:
                    type: array
                    items:
                      type: object
                      properties:
                        operation:
                          type: string
                        resource:
                          type: string
                        result:
                          type: string
                        timestamp:
                          type: string
                          format: date-time
              
              effectivenessAssessment:
                type: object
                properties:
                  score:
                    type: number
                    minimum: 0.0
                    maximum: 1.0
                    description: "Effectiveness score (0.0 = failed, 1.0 = perfect)"
                  criteria:
                    type: object
                    properties:
                      alertResolved:
                        type: boolean
                      targetMetricImproved:
                        type: boolean
                      noNewAlertsGenerated:
                        type: boolean
                      resourceStabilized:
                        type: boolean
                      sideEffectsMinimal:
                        type: boolean
                  assessmentTime:
                    type: string
                    format: date-time
                  assessmentMethod:
                    type: string
                    enum: ["automated", "manual", "ml-derived"]
                  notes:
                    type: string
              
              followUpActions:
                type: array
                items:
                  type: object
                  properties:
                    actionId:
                      type: string
                    actionType:
                      type: string
                    timestamp:
                      type: string
                      format: date-time
                    relation:
                      type: string
                      enum: ["correction", "enhancement", "rollback", "escalation"]
  scope: Namespaced
  names:
    plural: resourceactiontraces
    singular: resourceactiontrace
    kind: ResourceActionTrace
    shortNames: ["rat", "actiontrace"]
```

### **3. OscillationPattern CRD**
**Purpose**: Store detected oscillation patterns for system-wide learning

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: oscillationpatterns.ai.prometheus-alerts-slm.io
spec:
  group: ai.prometheus-alerts-slm.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              patternDefinition:
                type: object
                properties:
                  patternType:
                    type: string
                    enum: ["scale-oscillation", "resource-thrashing", "ineffective-loop", "cascading-failure"]
                  description:
                    type: string
                  detectionCriteria:
                    type: object
                    properties:
                      minOccurrences:
                        type: integer
                        minimum: 2
                      timeWindow:
                        type: string
                        pattern: '^(\d+m|\d+h|\d+d)$'
                      actionSequence:
                        type: array
                        items:
                          type: string
                      thresholds:
                        type: object
                        additionalProperties:
                          type: number
                required: [patternType, minOccurrences, timeWindow]
              
              resourceScope:
                type: object
                properties:
                  resourceTypes:
                    type: array
                    items:
                      type: string
                  namespaces:
                    type: array
                    items:
                      type: string
                  labels:
                    type: object
                    additionalProperties:
                      type: string
              
              preventionStrategy:
                type: object
                properties:
                  strategy:
                    type: string
                    enum: ["block-action", "escalate-human", "alternative-action", "cooling-period"]
                  parameters:
                    type: object
                    additionalProperties:
                      type: string
                  alerting:
                    type: object
                    properties:
                      enabled:
                        type: boolean
                      severity:
                        type: string
                        enum: ["info", "warning", "critical"]
                      channels:
                        type: array
                        items:
                          type: string
          
          status:
            type: object
            properties:
              detectionHistory:
                type: array
                items:
                  type: object
                  properties:
                    detectedAt:
                      type: string
                      format: date-time
                    resourceRef:
                      type: object
                      properties:
                        name:
                          type: string
                        namespace:
                          type: string
                        kind:
                          type: string
                    confidence:
                      type: number
                      minimum: 0.0
                      maximum: 1.0
                    actionCount:
                      type: integer
                    preventionApplied:
                      type: boolean
                    outcome:
                      type: string
                      enum: ["prevented", "allowed", "escalated"]
              
              statistics:
                type: object
                properties:
                  totalDetections:
                    type: integer
                  preventionSuccessRate:
                    type: number
                    minimum: 0.0
                    maximum: 1.0
                  falsePositiveRate:
                    type: number
                    minimum: 0.0
                    maximum: 1.0
                  lastDetection:
                    type: string
                    format: date-time
  scope: Cluster
  names:
    plural: oscillationpatterns
    singular: oscillationpattern
    kind: OscillationPattern
    shortNames: ["op", "oscillation"]
```

### **4. ActionEffectiveness CRD**
**Purpose**: Store aggregated effectiveness metrics for continuous learning

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: actioneffectiveness.ai.prometheus-alerts-slm.io
spec:
  group: ai.prometheus-alerts-slm.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              scope:
                type: object
                properties:
                  scopeType:
                    type: string
                    enum: ["global", "namespace", "resource-type", "alert-type"]
                  scopeValue:
                    type: string
                    description: "Specific value for the scope (namespace name, resource type, etc.)"
              
              aggregationPeriod:
                type: string
                pattern: '^(\d+h|\d+d|\d+w|\d+m)$'
                default: "24h"
                description: "Time period for aggregating effectiveness data"
              
              analysisConfig:
                type: object
                properties:
                  minSampleSize:
                    type: integer
                    minimum: 5
                    default: 10
                    description: "Minimum number of actions needed for reliable analysis"
                  confidenceInterval:
                    type: number
                    minimum: 0.8
                    maximum: 0.99
                    default: 0.95
                    description: "Statistical confidence interval for effectiveness scores"
          
          status:
            type: object
            properties:
              effectiveness:
                type: object
                properties:
                  byActionType:
                    type: object
                    additionalProperties:
                      type: object
                      properties:
                        sampleSize:
                          type: integer
                        averageScore:
                          type: number
                        confidenceInterval:
                          type: object
                          properties:
                            lower:
                              type: number
                            upper:
                              type: number
                        trend:
                          type: string
                          enum: ["improving", "stable", "declining"]
                        lastUpdated:
                          type: string
                          format: date-time
                  
                  byAlertType:
                    type: object
                    additionalProperties:
                      type: object
                      properties:
                        sampleSize:
                          type: integer
                        averageScore:
                          type: number
                        mostEffectiveAction:
                          type: string
                        leastEffectiveAction:
                          type: string
                  
                  byModel:
                    type: object
                    additionalProperties:
                      type: object
                      properties:
                        sampleSize:
                          type: integer
                        averageScore:
                          type: number
                        accuracyTrend:
                          type: string
                        recommendedUsage:
                          type: string
                          enum: ["primary", "secondary", "avoid"]
              
              recommendations:
                type: array
                items:
                  type: object
                  properties:
                    type:
                      type: string
                      enum: ["model-preference", "action-preference", "routing-adjustment", "threshold-change"]
                    priority:
                      type: string
                      enum: ["low", "medium", "high", "critical"]
                    recommendation:
                      type: string
                    evidence:
                      type: string
                    confidence:
                      type: number
                      minimum: 0.0
                      maximum: 1.0
              
              lastAnalysis:
                type: string
                format: date-time
              
              nextAnalysis:
                type: string
                format: date-time
  scope: Namespaced
  names:
    plural: actioneffectiveness
    singular: actioneffectiveness
    kind: ActionEffectiveness
    shortNames: ["ae", "effectiveness"]
```

## 🛠 **Implementation Architecture**

### **CRD Controller Design**

```go
// Action History Controller
type ActionHistoryController struct {
    client.Client
    Scheme               *runtime.Scheme
    OscillationDetector  OscillationDetector
    EffectivenessTracker EffectivenessTracker
    MetricsCollector     MetricsCollector
}

// Core reconciliation loop
func (r *ActionHistoryController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var actionHistory aipariv1.ActionHistory
    if err := r.Get(ctx, req.NamespacedName, &actionHistory); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Analyze action patterns
    patterns, err := r.OscillationDetector.AnalyzePatterns(ctx, &actionHistory)
    if err != nil {
        return ctrl.Result{}, err
    }
    
    // Update effectiveness metrics
    effectiveness, err := r.EffectivenessTracker.CalculateEffectiveness(ctx, &actionHistory)
    if err != nil {
        return ctrl.Result{}, err
    }
    
    // Update status
    actionHistory.Status.DetectedPatterns = patterns
    actionHistory.Status.EffectivenessMetrics = *effectiveness
    actionHistory.Status.AnalysisStatus.LastAnalyzed = metav1.Now()
    
    if err := r.Status().Update(ctx, &actionHistory); err != nil {
        return ctrl.Result{}, err
    }
    
    // Schedule next analysis
    return ctrl.Result{RequeueAfter: time.Hour}, nil
}

// Oscillation Detection Interface
type OscillationDetector interface {
    AnalyzePatterns(ctx context.Context, history *aipariv1.ActionHistory) ([]aipariv1.DetectedPattern, error)
    DetectScaleOscillation(actions []aipariv1.ResourceActionTrace) (*aipariv1.DetectedPattern, error)
    DetectResourceThrashing(actions []aipariv1.ResourceActionTrace) (*aipariv1.DetectedPattern, error)
    DetectIneffectiveLoops(actions []aipariv1.ResourceActionTrace) (*aipariv1.DetectedPattern, error)
}

// Effectiveness Tracking Interface
type EffectivenessTracker interface {
    CalculateEffectiveness(ctx context.Context, history *aipariv1.ActionHistory) (*aipariv1.EffectivenessMetrics, error)
    AssessActionOutcome(action *aipariv1.ResourceActionTrace) (float64, error)
    TrackLongTermTrends(actionType string, scores []float64) aipariv1.TrendDirection
}
```

### **Storage Integration Patterns**

```go
// Action Storage Manager
type ActionStorageManager struct {
    client           client.Client
    retentionManager RetentionManager
    compactionEngine CompactionEngine
    indexManager     IndexManager
}

// Store new action with automatic history management
func (asm *ActionStorageManager) StoreAction(ctx context.Context, action *ActionRecord) error {
    // Create ResourceActionTrace
    trace := &aipariv1.ResourceActionTrace{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-%s", action.ResourceKey, action.ActionID),
            Namespace: action.Namespace,
            Labels: map[string]string{
                "ai.prometheus-alerts-slm.io/resource-kind": action.ResourceKind,
                "ai.prometheus-alerts-slm.io/action-type":   action.ActionType,
                "ai.prometheus-alerts-slm.io/model-used":    action.ModelUsed,
            },
        },
        Spec: convertToActionTraceSpec(action),
    }
    
    if err := asm.client.Create(ctx, trace); err != nil {
        return fmt.Errorf("failed to create action trace: %w", err)
    }
    
    // Ensure ActionHistory exists
    history, err := asm.ensureActionHistory(ctx, action.ResourceReference)
    if err != nil {
        return fmt.Errorf("failed to ensure action history: %w", err)
    }
    
    // Apply retention policy
    if err := asm.retentionManager.ApplyRetention(ctx, history); err != nil {
        return fmt.Errorf("failed to apply retention: %w", err)
    }
    
    return nil
}

// Query interface for action history
func (asm *ActionStorageManager) QueryActionHistory(ctx context.Context, query ActionQuery) ([]aipariv1.ResourceActionTrace, error) {
    var traces aipariv1.ResourceActionTraceList
    
    listOpts := []client.ListOption{
        client.InNamespace(query.Namespace),
    }
    
    // Add label selectors based on query
    if query.ResourceKind != "" {
        listOpts = append(listOpts, client.MatchingLabels{
            "ai.prometheus-alerts-slm.io/resource-kind": query.ResourceKind,
        })
    }
    
    if err := asm.client.List(ctx, &traces, listOpts...); err != nil {
        return nil, fmt.Errorf("failed to list action traces: %w", err)
    }
    
    // Apply time-based filtering
    filtered := make([]aipariv1.ResourceActionTrace, 0)
    for _, trace := range traces.Items {
        if query.TimeRange.Contains(trace.Spec.ActionMetadata.Timestamp.Time) {
            filtered = append(filtered, trace)
        }
    }
    
    return filtered, nil
}
```

## 🔗 **Integration Points**

### **MCP Server Integration**

```go
// Action History MCP Server
type ActionHistoryMCPServer struct {
    client           client.Client
    storageManager   *ActionStorageManager
    patternDetector  OscillationDetector
    tools           []MCPTool
}

// MCP Tools for AI model access
func (server *ActionHistoryMCPServer) GetAvailableTools() []MCPTool {
    return []MCPTool{
        {
            Name:        "check_action_history",
            Description: "Retrieve action history for a Kubernetes resource",
            Parameters: map[string]interface{}{
                "namespace":     "string",
                "resource_kind": "string", 
                "resource_name": "string",
                "time_range":   "string (e.g., '2h', '1d')",
            },
        },
        {
            Name:        "detect_oscillation_patterns",
            Description: "Check for oscillation patterns in recent actions",
            Parameters: map[string]interface{}{
                "namespace":     "string",
                "resource_name": "string",
                "pattern_types": "array of pattern types to check",
            },
        },
        {
            Name:        "get_action_effectiveness",
            Description: "Get effectiveness scores for different action types",
            Parameters: map[string]interface{}{
                "action_type":  "string",
                "scope":        "string (global, namespace, resource-type)",
                "time_period": "string",
            },
        },
        {
            Name:        "validate_action_safety",
            Description: "Validate if proposed action is safe based on history",
            Parameters: map[string]interface{}{
                "namespace":     "string",
                "resource_name": "string", 
                "proposed_action": "string",
                "current_state": "object",
            },
        },
    }
}

// Tool implementation example
func (server *ActionHistoryMCPServer) CheckActionHistory(params map[string]interface{}) (interface{}, error) {
    namespace := params["namespace"].(string)
    resourceKind := params["resource_kind"].(string)
    resourceName := params["resource_name"].(string)
    timeRange := params["time_range"].(string)
    
    // Parse time range
    duration, err := time.ParseDuration(timeRange)
    if err != nil {
        return nil, fmt.Errorf("invalid time range: %w", err)
    }
    
    // Query action history
    query := ActionQuery{
        Namespace:    namespace,
        ResourceKind: resourceKind,
        ResourceName: resourceName,
        TimeRange: TimeRange{
            Start: time.Now().Add(-duration),
            End:   time.Now(),
        },
    }
    
    actions, err := server.storageManager.QueryActionHistory(context.Background(), query)
    if err != nil {
        return nil, fmt.Errorf("failed to query action history: %w", err)
    }
    
    // Format response for AI model
    response := map[string]interface{}{
        "resource": map[string]string{
            "namespace": namespace,
            "kind":      resourceKind,
            "name":      resourceName,
        },
        "time_range": map[string]interface{}{
            "duration": timeRange,
            "start":    query.TimeRange.Start,
            "end":      query.TimeRange.End,
        },
        "total_actions": len(actions),
        "actions": formatActionsForAI(actions),
        "patterns": server.patternDetector.AnalyzePatternsQuick(actions),
        "effectiveness_summary": calculateEffectivenessSummary(actions),
    }
    
    return response, nil
}
```

## 📊 **Retention and Lifecycle Management**

### **Retention Policy Implementation**

```go
// Retention Manager
type RetentionManager struct {
    client client.Client
}

func (rm *RetentionManager) ApplyRetention(ctx context.Context, history *aipariv1.ActionHistory) error {
    policy := history.Spec.RetentionPolicy
    
    // Get all action traces for this resource
    var traces aipariv1.ResourceActionTraceList
    if err := rm.client.List(ctx, &traces, client.MatchingFields{
        "spec.actionHistoryRef.name": history.Name,
    }); err != nil {
        return fmt.Errorf("failed to list action traces: %w", err)
    }
    
    // Apply retention based on strategy
    switch policy.CompactionStrategy {
    case "oldest-first":
        return rm.applyOldestFirstRetention(ctx, traces.Items, policy.MaxActions)
    case "effectiveness-based":
        return rm.applyEffectivenessBasedRetention(ctx, traces.Items, policy.MaxActions)
    case "pattern-aware":
        return rm.applyPatternAwareRetention(ctx, traces.Items, policy.MaxActions)
    default:
        return fmt.Errorf("unknown compaction strategy: %s", policy.CompactionStrategy)
    }
}

// Pattern-aware retention preserves oscillation examples
func (rm *RetentionManager) applyPatternAwareRetention(ctx context.Context, traces []aipariv1.ResourceActionTrace, maxActions int) error {
    if len(traces) <= maxActions {
        return nil // No retention needed
    }
    
    // Categorize traces
    patternExamples := make([]aipariv1.ResourceActionTrace, 0)
    highEffectiveness := make([]aipariv1.ResourceActionTrace, 0)
    lowEffectiveness := make([]aipariv1.ResourceActionTrace, 0)
    recent := make([]aipariv1.ResourceActionTrace, 0)
    
    for _, trace := range traces {
        if isPatternExample(trace) {
            patternExamples = append(patternExamples, trace)
        } else if trace.Status.EffectivenessAssessment.Score > 0.8 {
            highEffectiveness = append(highEffectiveness, trace)
        } else if trace.Status.EffectivenessAssessment.Score < 0.3 {
            lowEffectiveness = append(lowEffectiveness, trace)
        } else if isRecent(trace, 24*time.Hour) {
            recent = append(recent, trace)
        }
    }
    
    // Preserve important traces and remove least valuable
    preserve := make([]aipariv1.ResourceActionTrace, 0, maxActions)
    preserve = append(preserve, patternExamples...)
    preserve = append(preserve, recent...)
    preserve = append(preserve, highEffectiveness[:min(len(highEffectiveness), maxActions/4)]...)
    preserve = append(preserve, lowEffectiveness[:min(len(lowEffectiveness), maxActions/10)]...) // Keep some failures for learning
    
    // Remove excess traces
    if len(preserve) > maxActions {
        preserve = preserve[:maxActions]
    }
    
    // Delete non-preserved traces
    preserveMap := make(map[string]bool)
    for _, trace := range preserve {
        preserveMap[trace.Name] = true
    }
    
    for _, trace := range traces {
        if !preserveMap[trace.Name] {
            if err := rm.client.Delete(ctx, &trace); err != nil {
                return fmt.Errorf("failed to delete trace %s: %w", trace.Name, err)
            }
        }
    }
    
    return nil
}
```

## 📈 **Metrics and Observability**

### **Prometheus Metrics Integration**

```go
// Metrics for CRD usage and effectiveness
var (
    actionHistoryTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ai_action_history_total",
            Help: "Total number of action history records created",
        },
        []string{"namespace", "resource_kind", "action_type"},
    )
    
    oscillationDetectionTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ai_oscillation_detection_total", 
            Help: "Total number of oscillation patterns detected",
        },
        []string{"pattern_type", "namespace", "severity"},
    )
    
    actionEffectivenessGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ai_action_effectiveness_score",
            Help: "Current effectiveness score for action types",
        },
        []string{"action_type", "scope", "model_used"},
    )
    
    retentionOperationsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ai_retention_operations_total",
            Help: "Total number of retention operations performed",
        },
        []string{"strategy", "namespace", "result"},
    )
)
```

## 🎯 **Next Steps**

This CRD design provides:

1. **Comprehensive Action Tracking** - Full context and outcome recording
2. **Intelligent Retention** - Pattern-aware data preservation  
3. **Oscillation Detection** - Built-in loop prevention capabilities
4. **Effectiveness Learning** - Continuous improvement through analysis
5. **MCP Integration** - AI model access to historical intelligence

**Ready for implementation** with:
- Complete CRD definitions
- Controller architecture
- Storage management patterns
- MCP server integration
- Retention and lifecycle policies

Would you like me to proceed with implementing the oscillation detection algorithms that will work with these CRDs?