## HolmesGPT Toolsets & Dynamic Data Fetching (BR-AI-031)

### HolmesGPT Toolsets - No CRD Storage Needed

**Architectural Principle**: HolmesGPT fetches logs/metrics **dynamically** using built-in toolsets. RemediationProcessing provides **targeting data only** (<10KB).

**Why No Log/Metric Storage in CRDs**:
1. **Kubernetes etcd Limit**: 1.5MB per object (typical), 1MB recommended
2. **Data Freshness**: Logs stored in CRDs become stale; HolmesGPT needs real-time data
3. **HolmesGPT Design**: Built-in toolsets fetch data from live sources (Kubernetes API, Prometheus, etc.)

### Solution: HolmesGPT Built-in Toolsets

**Strategy**: RemediationProcessing stores **targeting data** (namespace, resource name), HolmesGPT fetches logs/metrics using toolsets

**HolmesGPT Toolsets Used in Kubernaut V1** (from [HolmesGPT official docs](https://github.com/robusta-dev/holmesgpt)):

| Toolset | Status | Capabilities | Kubernaut V1 Usage |
|---------|--------|--------------|-------------------|
| **kubernetes** | ✅ | Pod logs, K8s events, resource status (`kubectl describe`) | **PRIMARY** - Fetches pod logs and events dynamically |
| **prometheus** | ✅ | Query metrics, generate PromQL queries, investigate alerts | **PRIMARY** - Fetches metrics for alert context |
| **grafana** | ✅ | Investigate dashboards, download panels as images | Optional - Visual context for metrics |

**Additional Toolsets Available** (for future V2 enhancement):
- `alertmanager`, `datadog`, `aws`, `azure`, `gcp`, `jira`, `github`, `kafka`, `rabbitmq`, `opensearch`, `robusta`, `newrelic`, `slab`

#### BR-AI-031: HolmesGPT Toolset Integration

**Requirement**: AIAnalysis CRD provides targeting data for HolmesGPT to fetch logs/metrics dynamically

**Implementation**:
1. **Targeting Data** (<10KB): Namespace, resource kind/name, pod details in AlertContext
2. **HolmesGPT Fetches Dynamically**: Logs via `kubernetes` toolset, metrics via `prometheus` toolset
3. **No CRD Storage**: Logs/metrics NEVER stored in CRD status or spec
4. **Fresh Data**: HolmesGPT always gets real-time logs/metrics, not stale snapshots

#### AlertContext Schema (Targeting Data Only)

**What RemediationProcessing Provides to AIAnalysis** (~8-10KB total):

```yaml
spec:
  analysisRequest:
    alertContext:
      # Alert identification
      fingerprint: "abc123def456"
      severity: "critical"
      environment: "production"
      businessPriority: "p0"

      # Resource targeting for HolmesGPT toolsets
      namespace: "production-app"
      resourceKind: "Pod"
      resourceName: "web-app-789"

      # Kubernetes context (small data)
      kubernetesContext:
        podDetails:
          name: "web-app-789"
          status: "CrashLoopBackOff"
          containerNames: ["app", "sidecar"]
          restartCount: 47
        deploymentDetails:
          name: "web-app"
          replicas: 3
        nodeDetails:
          name: "node-1"
          conditions: {...}
```

**What HolmesGPT Toolsets Fetch Dynamically**:

```python
# HolmesGPT uses kubernetes toolset to fetch logs
kubernetes_toolset.get_pod_logs(
    namespace="production-app",
    pod_name="web-app-789",
    container="app",
    tail_lines=500  # Fresh, real-time logs
)

# HolmesGPT uses prometheus toolset to fetch metrics
prometheus_toolset.query_metrics(
    query='container_memory_usage_bytes{pod="web-app-789"}',
    time_range="1h"  # Fresh, real-time metrics
)
```

#### Implementation: Dynamic Toolset Configuration

**Toolset Management Architecture**:

```
Dynamic Toolset Service → HolmesGPT-API → HolmesGPT SDK
```

**Dynamic Toolset Service** (see `DYNAMIC_TOOLSET_CONFIGURATION_ARCHITECTURE.md`):
- Discovers cluster services (Prometheus, Grafana, Jaeger, Elasticsearch)
- Generates toolset configurations automatically
- Exposes toolset configuration via REST API (`GET /toolsets`)
- Updates configuration when services added/removed in cluster

**HolmesGPT-API Toolset Initialization**:

```python
# In HolmesGPT-API Service (Python wrapper)
from holmes import Client
import requests

# Query Dynamic Toolset Service for available toolsets
toolsets_response = requests.get(
    "http://dynamic-toolset-service.kubernaut-system.svc.cluster.local:8095/toolsets"
)
available_toolsets = toolsets_response.json()["toolsets"]
# Example: ["kubernetes", "prometheus", "grafana"] if Grafana deployed

# Initialize HolmesGPT with dynamic toolsets (system-wide, not per-investigation)
holmes_client = Client(
    api_key=llm_api_key,
    toolsets=available_toolsets  # Dynamically configured
)

# AIAnalysis controller sends investigation request (NO toolset config in request)
result = holmes_client.investigate(
    alert_name=alert_context.fingerprint,
    namespace=alert_context.namespace,  # Targeting data only
    resource_name=alert_context.resource_name,  # What pod to investigate
    # HolmesGPT toolsets automatically fetch data:
    # 1. kubectl logs -n production-app web-app-789 --tail 500 (kubernetes toolset)
    # 2. kubectl describe pod web-app-789 -n production-app (kubernetes toolset)
    # 3. kubectl get events -n production-app (kubernetes toolset)
    # 4. promql: container_memory_usage_bytes{pod="web-app-789"} (prometheus toolset)
    # 5. grafana dashboard query (grafana toolset, if available)
)
```

**Key Points**:
- ❌ AIAnalysis CRD does NOT contain `holmesGPTConfig` field
- ✅ Toolsets configured system-wide (per HolmesGPT instance)
- ✅ Dynamic Toolset Service manages toolset lifecycle
- ✅ HolmesGPT-API initialized once with available toolsets
- ✅ AIAnalysis controller sends investigation requests (targeting data only)

#### Metrics for Toolset Performance

```go
var (
    aiToolsetInvocationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name: "kubernaut_ai_toolset_invocation_duration_seconds",
        Help: "Duration of HolmesGPT toolset invocations",
        Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0},
    }, []string{"toolset", "operation"})  // toolset: kubernetes, prometheus

    aiToolsetErrorTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_ai_toolset_error_total",
        Help: "Total toolset invocation errors",
    }, []string{"toolset", "error_type"})
)
```

---


## Rego-Based Approval Policy (BR-AI-025, BR-AI-026)

### Manual Approval Workflow

**Trigger**: When AI recommendations require human approval based on policy evaluation

#### BR-AI-025: Policy-Driven Approval Requirements

**Requirement**: AI recommendations MUST be evaluated against configurable approval policies before execution

**Implementation**: OPA/Rego policy engine for approval decisions

#### BR-AI-026: RBAC-Secured Approval Process

**Requirement**: Approval actions MUST be secured with Kubernetes RBAC

**Implementation**: Separate `AIApprovalRequest` CRD with role-based access control

### Architecture: Rego Policy + Approval CRD

```
AIAnalysis (completed)
        ↓
    Evaluate Rego Policy
        ↓
    Requires Approval?
    /              \
  NO               YES
   ↓                ↓
Auto-Execute    Create AIApprovalRequest
                     ↓
                 Wait for Approval
                     ↓
                 (User with RBAC creates approval)
                     ↓
                 Execute Recommendation
```

### Rego Policy Structure

**Policy Storage**: ConfigMap `ai-approval-policies` in `kubernaut-system`

**Production Policy** (`policies/approval/production.rego`):
```rego
package kubernaut.approval.production

import future.keywords.if
import data.kubernaut.approval.common

# Default: Require approval in production
default require_approval := true
default auto_approve := false

# Auto-approve safe scaling actions
auto_approve if {
    input.environment == "production"
    common.is_safe_scaling_action
}

# High-risk during business hours → 1 approver, 2h timeout
require_approval if {
    input.environment == "production"
    common.is_high_risk_action
    common.is_business_hours
}

min_approvers := 1 if {
    input.environment == "production"
    common.is_high_risk_action
    common.is_business_hours
}

timeout := "2h" if {
    input.environment == "production"
    common.is_high_risk_action
    common.is_business_hours
}

# After hours → 2 approvers, 24h timeout
min_approvers := 2 if {
    input.environment == "production"
    common.is_high_risk_action
    not common.is_business_hours
}

timeout := "24h" if {
    input.environment == "production"
    common.is_high_risk_action
    not common.is_business_hours
}

# Critical severity override
min_approvers := 2 if {
    input.environment == "production"
    input.severity == "critical"
    common.is_high_risk_action
}

approver_groups := ["system:kubernaut:production-approvers", "system:kubernaut:platform-admin"] if {
    require_approval
}

decision := {
    "require_approval": require_approval,
    "auto_approve": auto_approve,
    "min_approvers": min_approvers,
    "timeout": timeout,
    "approver_groups": approver_groups,
    "policy_name": "production",
    "reason": reason,
}

reason := sprintf("Auto-approved: %s in %s", [input.action, input.environment]) if {
    auto_approve
}

reason := sprintf("Requires %d approval(s): %s in %s (%s severity)", [
    min_approvers,
    input.action,
    input.environment,
    input.severity,
]) if {
    require_approval
    not auto_approve
}
```

**Common Helpers** (`policies/approval/common.rego`):
```rego
package kubernaut.approval.common

import future.keywords.if

is_high_risk_action if {
    input.action in [
        "restart-pod",
        "delete-pod",
        "delete-deployment",
        "drain-node",
        "cordon-node",
    ]
}

is_safe_scaling_action if {
    input.action in [
        "increase-memory-limit",
        "increase-cpu-limit",
        "adjust-hpa",
    ]
}

is_business_hours if {
    time_obj := time.parse_rfc3339_ns(input.timestamp)
    weekday := time.weekday(time_obj)
    not weekday in ["Saturday", "Sunday"]
    hour := time.clock(time_obj)[0]
    hour >= 9
    hour < 17
}
```

### AIApprovalRequest CRD

**Schema**:
```yaml
apiVersion: aianalysis.kubernaut.io/v1
kind: AIApprovalRequest
metadata:
  name: approval-aianalysis-abc123
  namespace: kubernaut-system
spec:
  aiAnalysisRef:
    name: aianalysis-abc123
    namespace: kubernaut-system
    uid: "550e8400-e29b-41d4-a716-446655440000"

  recommendation:
    action: "restart-pod"
    targetResource:
      kind: Pod
      name: web-app-789
      namespace: production
    parameters:
      gracePeriodSeconds: 30
    riskLevel: high
    effectivenessProbability: 0.92

  timeout: "2h"

status:
  phase: "pending"  # pending, approved, rejected, timeout

  policyEvaluation:
    policyName: "production"
    reason: "Requires 1 approval(s): restart-pod in production (high severity)"
    approverGroups:
    - "system:kubernaut:production-approvers"
    - "system:kubernaut:platform-admin"
    minApprovers: 1

  approvals:
  - approver: "user@example.com"
    timestamp: "2025-01-15T11:00:00Z"
    groups: ["system:kubernaut:production-approvers"]

  approvalTime: "2025-01-15T11:00:00Z"
```

### Policy Evaluation in Controller

```go
// pkg/ai/analysis/policy/evaluator.go
package policy

import (
    "context"
    "time"
    "github.com/open-policy-agent/opa/rego"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
)

type PolicyEngine struct {
    query rego.PreparedEvalQuery
}

func NewPolicyEngine(ctx context.Context, k8sClient client.Client) (*PolicyEngine, error) {
    // Load Rego policies from ConfigMap
    var cm corev1.ConfigMap
    if err := k8sClient.Get(ctx, client.ObjectKey{
        Name:      "ai-approval-policies",
        Namespace: "kubernaut-system",
    }, &cm); err != nil {
        return nil, err
    }

    // Compile policies
    query, err := rego.New(
        rego.Query("data.kubernaut.approval.decision"),
        rego.Module("approval.rego", strings.Join(cm.Data, "\n")),
    ).PrepareForEval(ctx)

    if err != nil {
        return nil, err
    }

    return &PolicyEngine{query: query}, nil
}

func (p *PolicyEngine) Evaluate(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
    recommendation *Recommendation,
) (*PolicyDecision, error) {

    input := map[string]interface{}{
        "action":           recommendation.Action,
        "environment":      aiAnalysis.Spec.AnalysisRequest.AlertContext.Environment,
        "severity":         aiAnalysis.Spec.AnalysisRequest.AlertContext.Severity,
        "businessPriority": aiAnalysis.Spec.AnalysisRequest.AlertContext.BusinessPriority,
        "timestamp":        time.Now().Format(time.RFC3339),
    }

    results, err := p.query.Eval(ctx, rego.EvalInput(input))
    if err != nil {
        return nil, err
    }

    decisionMap := results[0].Expressions[0].Value.(map[string]interface{})
    timeout, _ := time.ParseDuration(decisionMap["timeout"].(string))

    return &PolicyDecision{
        RequiresApproval: decisionMap["require_approval"].(bool),
        AutoApprove:      decisionMap["auto_approve"].(bool),
        MinApprovers:     int(decisionMap["min_approvers"].(float64)),
        Timeout:          timeout,
        ApproverGroups:   parseApproverGroups(decisionMap["approver_groups"]),
        PolicyName:       decisionMap["policy_name"].(string),
        Reason:           decisionMap["reason"].(string),
    }, nil
}
```

### Controller Integration: Create Approval Request

```go
// In AIAnalysisReconciler
func (r *AIAnalysisReconciler) handleCompleted(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {

    if len(aiAnalysis.Status.Recommendations) == 0 {
        return ctrl.Result{}, fmt.Errorf("no recommendations")
    }

    topRecommendation := aiAnalysis.Status.Recommendations[0]

    // Evaluate Rego policy
    policyDecision, err := r.PolicyEngine.Evaluate(ctx, aiAnalysis, topRecommendation)
    if err != nil {
        return ctrl.Result{}, err
    }

    if policyDecision.RequiresApproval && !policyDecision.AutoApprove {
        // Create AIApprovalRequest
        approvalReq := &aianalysisv1.AIApprovalRequest{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("approval-%s", aiAnalysis.Name),
                Namespace: aiAnalysis.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(aiAnalysis, aianalysisv1.GroupVersion.WithKind("AIAnalysis")),
                },
            },
            Spec: aianalysisv1.AIApprovalRequestSpec{
                AIAnalysisRef: aianalysisv1.AIAnalysisReference{
                    Name:      aiAnalysis.Name,
                    Namespace: aiAnalysis.Namespace,
                    UID:       aiAnalysis.UID,
                },
                Recommendation: topRecommendation,
                Timeout:        policyDecision.Timeout.String(),
            },
            Status: aianalysisv1.AIApprovalRequestStatus{
                Phase: "pending",
                PolicyEvaluation: &aianalysisv1.PolicyEvaluation{
                    PolicyName:     policyDecision.PolicyName,
                    Reason:         policyDecision.Reason,
                    ApproverGroups: policyDecision.ApproverGroups,
                    MinApprovers:   policyDecision.MinApprovers,
                },
            },
        }

        if err := r.Create(ctx, approvalReq); err != nil {
            return ctrl.Result{}, err
        }

        // Update AIAnalysis
        aiAnalysis.Status.Phase = "awaiting_approval"
        aiAnalysis.Status.ApprovalRequestRef = &aianalysisv1.AIApprovalRequestReference{
            Name:      approvalReq.Name,
            Namespace: approvalReq.Namespace,
        }

        r.Recorder.Event(aiAnalysis, corev1.EventTypeNormal, "ApprovalRequired",
            fmt.Sprintf("Policy: %s - %s", policyDecision.PolicyName, policyDecision.Reason))

        return ctrl.Result{}, r.Status().Update(ctx, aiAnalysis)
    }

    // Auto-approved - create WorkflowExecution
    r.Recorder.Event(aiAnalysis, corev1.EventTypeNormal, "AutoApproved",
        fmt.Sprintf("Policy: %s - %s", policyDecision.PolicyName, policyDecision.Reason))

    return r.createWorkflowExecution(ctx, aiAnalysis, topRecommendation)
}
```

### RBAC for Approval

**Role Definitions**:
```yaml
# Senior SRE - Can approve high-risk production actions
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: ai-approval-senior-sre
  namespace: kubernaut-system
rules:
- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aiapprovalrequests"]
  verbs: ["get", "list", "watch", "update", "patch"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ai-approval-senior-sre
  namespace: kubernaut-system
subjects:
- kind: Group
  name: "system:kubernaut:production-approvers"
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: ai-approval-senior-sre
  apiGroup: rbac.authorization.k8s.io
```

### Approval Workflow: User Action

**User approves by patching AIApprovalRequest**:
```bash
kubectl patch aiapprovalrequest approval-aianalysis-abc123 -n kubernaut-system \
  --type=merge \
  -p '{"status":{"phase":"approved","approvals":[{"approver":"alice@example.com","timestamp":"2025-01-15T11:00:00Z","groups":["system:kubernaut:production-approvers"]}],"approvalTime":"2025-01-15T11:00:00Z"}}'
```

**AIAnalysisReconciler watches AIApprovalRequest and creates WorkflowExecution when approved**.

### Metrics for Approval

```go
var (
    aiApprovalRequestCount = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_ai_approval_requests_total",
        Help: "Total AI approval requests created",
    }, []string{"environment", "action_type", "policy_name"})

    aiApprovalDecisionCount = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_ai_approval_decisions_total",
        Help: "Total approval decisions",
    }, []string{"decision", "environment"})  // decision: approved, rejected, timeout, auto_approved
)
```

---

## Historical Success Rate Fallback (BR-AI-033)

### Problem: Missing Historical Data for New Actions

**Challenge**: When AI recommends a new action type with no historical success rate data

#### BR-AI-033: Historical Success Rate Fallback Strategy

**Requirement**: AI recommendation ranking MUST gracefully handle missing historical success rates

### Fallback Strategy

**Tier 1: Direct Action Match** (Primary)
- Query database for exact action type (e.g., "increase-memory-limit")
- Return historical success rate if available

**Tier 2: Action Category Match** (Fallback)
- Group similar actions by category (e.g., "resource-scaling", "pod-restart", "deployment-update")
- Use category-level success rate

**Tier 3: Environment-Specific Baseline** (Fallback)
- Use environment-specific baseline success rate (production: 0.75, development: 0.85)

**Tier 4: Global Baseline** (Final Fallback)
- Use global baseline: 0.70 (conservative estimate)

### Implementation: Success Rate Retrieval with Fallback

```go
// pkg/ai/analysis/integration/storage_fallback.go
package integration

import (
    "context"
    "fmt"
)

type StorageClientWithFallback struct {
    db          *PostgresClient
    vectorDB    *VectorDBClient
}

type SuccessRateResult struct {
    SuccessRate  float64
    FallbackTier string  // "direct", "category", "environment-baseline", "global-baseline"
    SampleSize   int     // Number of historical data points used
    Confidence   float64 // Confidence in the success rate (based on sample size)
}

func (s *StorageClientWithFallback) GetSuccessRate(
    ctx context.Context,
    action string,
    environment string,
) (*SuccessRateResult, error) {

    // Tier 1: Direct action match
    directRate, sampleSize, err := s.getDirectSuccessRate(ctx, action)
    if err == nil && sampleSize >= 5 {  // Minimum 5 data points for confidence
        return &SuccessRateResult{
            SuccessRate:  directRate,
            FallbackTier: "direct",
            SampleSize:   sampleSize,
            Confidence:   calculateConfidence(sampleSize),
        }, nil
    }

    // Tier 2: Action category match
    category := categorizeAction(action)
    categoryRate, catSampleSize, err := s.getCategorySuccessRate(ctx, category)
    if err == nil && catSampleSize >= 10 {
        return &SuccessRateResult{
            SuccessRate:  categoryRate,
            FallbackTier: fmt.Sprintf("category:%s", category),
            SampleSize:   catSampleSize,
            Confidence:   calculateConfidence(catSampleSize) * 0.9,  // Slightly lower confidence
        }, nil
    }

    // Tier 3: Environment-specific baseline
    envBaseline := getEnvironmentBaseline(environment)
    if envBaseline > 0 {
        return &SuccessRateResult{
            SuccessRate:  envBaseline,
            FallbackTier: fmt.Sprintf("environment-baseline:%s", environment),
            SampleSize:   0,
            Confidence:   0.5,  // Low confidence - it's a baseline
        }, nil
    }

    // Tier 4: Global baseline
    return &SuccessRateResult{
        SuccessRate:  0.70,
        FallbackTier: "global-baseline",
        SampleSize:   0,
        Confidence:   0.3,  // Very low confidence
    }, nil
}

func (s *StorageClientWithFallback) getDirectSuccessRate(
    ctx context.Context,
    action string,
) (float64, int, error) {

    var result struct {
        SuccessRate float64
        SampleSize  int
    }

    query := `
        SELECT
            AVG(CASE WHEN completion_status = 'success' THEN 1.0 ELSE 0.0 END) as success_rate,
            COUNT(*) as sample_size
        FROM workflow_execution_audit
        WHERE action = $1
          AND created_at > NOW() - INTERVAL '90 days'
    `

    err := s.db.QueryRow(ctx, query, action).Scan(&result.SuccessRate, &result.SampleSize)
    if err != nil {
        return 0, 0, err
    }

    return result.SuccessRate, result.SampleSize, nil
}

func (s *StorageClientWithFallback) getCategorySuccessRate(
    ctx context.Context,
    category string,
) (float64, int, error) {

    var result struct {
        SuccessRate float64
        SampleSize  int
    }

    query := `
        SELECT
            AVG(CASE WHEN completion_status = 'success' THEN 1.0 ELSE 0.0 END) as success_rate,
            COUNT(*) as sample_size
        FROM workflow_execution_audit
        WHERE action_category = $1
          AND created_at > NOW() - INTERVAL '90 days'
    `

    err := s.db.QueryRow(ctx, query, category).Scan(&result.SuccessRate, &result.SampleSize)
    if err != nil {
        return 0, 0, err
    }

    return result.SuccessRate, result.SampleSize, nil
}

// Action categorization
func categorizeAction(action string) string {
    categories := map[string][]string{
        "resource-scaling": {
            "increase-memory-limit",
            "increase-cpu-limit",
            "adjust-hpa",
            "scale-up-deployment",
        },
        "pod-restart": {
            "restart-pod",
            "restart-deployment",
            "rollout-restart",
        },
        "deployment-update": {
            "update-image",
            "update-config",
            "update-env-vars",
        },
        "node-operations": {
            "drain-node",
            "cordon-node",
            "uncordon-node",
        },
    }

    for category, actions := range categories {
        for _, a := range actions {
            if a == action {
                return category
            }
        }
    }

    return "uncategorized"
}

// Environment baseline success rates
func getEnvironmentBaseline(environment string) float64 {
    baselines := map[string]float64{
        "production":  0.75,  // Conservative for production
        "staging":     0.80,
        "development": 0.85,  // More permissive for dev
    }

    if baseline, ok := baselines[environment]; ok {
        return baseline
    }

    return 0.70  // Global baseline
}

// Confidence calculation based on sample size
func calculateConfidence(sampleSize int) float64 {
    switch {
    case sampleSize >= 50:
        return 0.95
    case sampleSize >= 20:
        return 0.85
    case sampleSize >= 10:
        return 0.75
    case sampleSize >= 5:
        return 0.60
    default:
        return 0.40
    }
}
```

### Recommendation Ranking with Fallback

```go
// pkg/ai/analysis/phases/recommending.go
func (p *RecommendingPhase) rankRecommendations(
    ctx context.Context,
    recommendations []Recommendation,
    environment string,
) ([]RankedRecommendation, error) {

    ranked := make([]RankedRecommendation, 0, len(recommendations))

    for _, rec := range recommendations {
        // Get success rate with fallback
        successRateResult, err := p.StorageClient.GetSuccessRate(ctx, rec.Action, environment)
        if err != nil {
            return nil, err
        }

        // Calculate final effectiveness score
        // Weight: AI confidence (60%) + Historical success rate (40%)
        effectivenessScore := (rec.AIConfidence * 0.6) + (successRateResult.SuccessRate * 0.4)

        // Adjust for confidence in success rate data
        if successRateResult.FallbackTier != "direct" {
            // Increase weight on AI confidence when using fallback
            effectivenessScore = (rec.AIConfidence * 0.75) + (successRateResult.SuccessRate * 0.25)
        }

        ranked = append(ranked, RankedRecommendation{
            Recommendation:          rec,
            EffectivenessProbability: effectivenessScore,
            HistoricalSuccessRate:   successRateResult.SuccessRate,
            HistoricalSampleSize:    successRateResult.SampleSize,
            HistoricalConfidence:    successRateResult.Confidence,
            FallbackTier:            successRateResult.FallbackTier,
        })
    }

    // Sort by effectiveness probability (descending)
    sort.Slice(ranked, func(i, j int) bool {
        return ranked[i].EffectivenessProbability > ranked[j].EffectivenessProbability
    })

    return ranked, nil
}
```

### Status Field with Fallback Metadata

```yaml
status:
  phase: recommending
  recommendations:
  - action: "increase-memory-limit"
    effectivenessProbability: 0.89
    historicalSuccessRate: 0.88
    historicalSampleSize: 15
    historicalConfidence: 0.75
    fallbackTier: "direct"  # Direct historical data used
    explanation: "Based on 15 similar cases, 88% success rate"

  - action: "restart-pod"
    effectivenessProbability: 0.82
    historicalSuccessRate: 0.75
    historicalSampleSize: 0
    historicalConfidence: 0.50
    fallbackTier: "environment-baseline:production"  # Fallback used
    explanation: "No historical data, using production baseline (75%)"
```

### Metrics for Success Rate Fallback

```go
var (
    aiSuccessRateFallbackCount = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_ai_success_rate_fallback_total",
        Help: "Total success rate fallback operations",
    }, []string{"fallback_tier", "action_category"})

    aiSuccessRateConfidence = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name: "kubernaut_ai_success_rate_confidence",
        Help: "Confidence level in historical success rate data",
        Buckets: []float64{0.3, 0.5, 0.6, 0.75, 0.85, 0.95},
    }, []string{"fallback_tier"})
)

// Record fallback usage
aiSuccessRateFallbackCount.WithLabelValues("environment-baseline", "resource-scaling").Inc()
aiSuccessRateConfidence.WithLabelValues("category").Observe(0.75)
```

### Database Schema Update

**Add action category to workflow_execution_audit**:
```sql
ALTER TABLE workflow_execution_audit
ADD COLUMN action_category VARCHAR(50);

CREATE INDEX idx_action_category ON workflow_execution_audit(action_category);

-- Populate existing data
UPDATE workflow_execution_audit
SET action_category = CASE
    WHEN action IN ('increase-memory-limit', 'increase-cpu-limit', 'adjust-hpa') THEN 'resource-scaling'
    WHEN action IN ('restart-pod', 'restart-deployment', 'rollout-restart') THEN 'pod-restart'
    WHEN action IN ('drain-node', 'cordon-node', 'uncordon-node') THEN 'node-operations'
    ELSE 'uncategorized'
END;
```


---

