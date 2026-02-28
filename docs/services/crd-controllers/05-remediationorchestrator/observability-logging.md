## Observability & Logging

### Structured Logging Patterns

**Log Levels**:

| Level | Purpose | Examples |
|-------|---------|----------|
| **ERROR** | Unrecoverable errors, requires intervention | Child CRD creation failure, timeout escalation failure |
| **WARN** | Recoverable errors, degraded mode | Phase timeout detected (will escalate), child CRD status unknown |
| **INFO** | Normal operations, state transitions | CRD creation, phase transitions, end-to-end completion |
| **DEBUG** | Detailed flow for troubleshooting | Child CRD status polling, timeout calculations, targeting data propagation |

**Structured Logging Implementation**:

```go
package controller

import (
    "context"
    "time"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    alertprocessorv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type RemediationRequestReconciler struct {
    client.Client
    Scheme *runtime.Scheme
    Log    logr.Logger
}

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Create request-scoped logger with correlation ID
    log := r.Log.WithValues(
        "remediationrequest", req.NamespacedName,
        "correlationID", extractCorrelationID(ctx),
    )

    var ar remediationv1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &ar); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Log phase transitions
    oldPhase := ar.Status.Phase
    log.Info("Reconciling RemediationRequest",
        "phase", ar.Status.Phase,
        "alertFingerprint", ar.Spec.TargetingData.Signal.Fingerprint,
        "environment", ar.Spec.TargetingData.Environment,
    )

    // Execute reconciliation logic with structured logging
    result, err := r.reconcilePhase(ctx, &ar, log)

    // Log phase change
    if ar.Status.Phase != oldPhase {
        log.Info("Phase transition",
            "from", oldPhase,
            "to", ar.Status.Phase,
            "duration", time.Since(ar.Status.StartTime.Time),
        )
    }

    if err != nil {
        log.Error(err, "Reconciliation failed",
            "phase", ar.Status.Phase,
        )
        return result, err
    }

    return result, nil
}

func (r *RemediationRequestReconciler) orchestrateRemediationProcessing(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    log logr.Logger,
) error {
    log.Info("Orchestrating RemediationProcessing phase",
        "alertFingerprint", ar.Spec.TargetingData.Signal.Fingerprint,
    )

    // Create SignalProcessing CRD
    start := time.Now()
    ap, err := r.createRemediationProcessing(ctx, ar)
    if err != nil {
        log.Error(err, "Failed to create SignalProcessing CRD")
        return err
    }
    log.Info("SignalProcessing CRD created",
        "alertprocessing", ap.Name,
        "creationDuration", time.Since(start),
    )

    // Watch RemediationProcessing status
    log.V(1).Info("Watching RemediationProcessing status",
        "alertprocessing", ap.Name,
    )

    return nil
}

func (r *RemediationRequestReconciler) watchRemediationProcessingCompletion(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    log logr.Logger,
) error {
    apLog := log.WithValues("alertprocessing", ar.Status.RemediationProcessingRef.Name)

    // Get SignalProcessing CRD
    var ap alertprocessorv1.RemediationProcessing
    if err := r.Get(ctx, client.ObjectKey{
        Name:      ar.Status.RemediationProcessingRef.Name,
        Namespace: ar.Namespace,
    }, &ap); err != nil {
        apLog.Error(err, "Failed to get RemediationProcessing status")
        return err
    }

    apLog.V(2).Info("Polling RemediationProcessing status",
        "phase", ap.Status.Phase,
        "degradedMode", ap.Status.DegradedMode,
        "duration", time.Since(ap.Status.StartTime.Time),
    )

    // Check if RemediationProcessing completed
    if ap.Status.Phase == "Ready" {
        duration := time.Since(ap.Status.StartTime.Time)
        apLog.Info("RemediationProcessing completed",
            "duration", duration,
            "degradedMode", ap.Status.DegradedMode,
        )

        // Proceed to AI Analysis phase
        return r.orchestrateAIAnalysis(ctx, ar, log)
    }

    // Check for timeout
    timeout := ar.Spec.PhaseTimeouts.RemediationProcessing
    if time.Since(ap.Status.StartTime.Time) > timeout {
        apLog.Warn("RemediationProcessing phase timeout detected",
            "timeout", timeout,
            "actualDuration", time.Since(ap.Status.StartTime.Time),
        )
        return fmt.Errorf("alert processing phase timeout")
    }

    return nil
}

func (r *RemediationRequestReconciler) orchestrateAIAnalysis(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    log logr.Logger,
) error {
    log.Info("Orchestrating AIAnalysis phase",
        "alertFingerprint", ar.Spec.TargetingData.Signal.Fingerprint,
    )

    // Create AIAnalysis CRD
    start := time.Now()
    aia, err := r.createAIAnalysis(ctx, ar)
    if err != nil {
        log.Error(err, "Failed to create AIAnalysis CRD")
        return err
    }
    log.Info("AIAnalysis CRD created",
        "aianalysis", aia.Name,
        "creationDuration", time.Since(start),
    )

    // Watch AIAnalysis status
    log.V(1).Info("Watching AIAnalysis status",
        "aianalysis", aia.Name,
    )

    return nil
}

func (r *RemediationRequestReconciler) watchAIAnalysisCompletion(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    log logr.Logger,
) error {
    aiaLog := log.WithValues("aianalysis", ar.Status.AIAnalysisRef.Name)

    // Get AIAnalysis CRD
    var aia aianalysisv1.AIAnalysis
    if err := r.Get(ctx, client.ObjectKey{
        Name:      ar.Status.AIAnalysisRef.Name,
        Namespace: ar.Namespace,
    }, &aia); err != nil {
        aiaLog.Error(err, "Failed to get AIAnalysis status")
        return err
    }

    aiaLog.V(2).Info("Polling AIAnalysis status",
        "phase", aia.Status.Phase,
        "approvalStatus", aia.Status.ApprovalStatus,
        "recommendationCount", len(aia.Status.Recommendations),
        "duration", time.Since(aia.Status.StartTime.Time),
    )

    // Check if AIAnalysis completed
    if aia.Status.Phase == "Ready" && aia.Status.ApprovalStatus == "Approved" {
        duration := time.Since(aia.Status.StartTime.Time)
        aiaLog.Info("AIAnalysis completed (approved)",
            "duration", duration,
            "topRecommendation", aia.Status.Recommendations[0].Action,
            "confidence", aia.Status.Recommendations[0].Confidence,
        )

        // Proceed to Workflow Execution phase
        return r.orchestrateWorkflowExecution(ctx, ar, log)
    } else if aia.Status.Phase == "Failed" || aia.Status.ApprovalStatus == "Rejected" {
        duration := time.Since(aia.Status.StartTime.Time)
        aiaLog.Warn("AIAnalysis rejected or failed",
            "duration", duration,
            "approvalStatus", aia.Status.ApprovalStatus,
            "phase", aia.Status.Phase,
        )

        // Escalate to Notification Service
        return r.escalateToNotification(ctx, ar, "AI analysis rejected or failed", log)
    }

    // Check for timeout
    timeout := ar.Spec.PhaseTimeouts.AIAnalysis
    if time.Since(aia.Status.StartTime.Time) > timeout {
        aiaLog.Warn("AIAnalysis phase timeout detected",
            "timeout", timeout,
            "actualDuration", time.Since(aia.Status.StartTime.Time),
        )
        return fmt.Errorf("AI analysis phase timeout")
    }

    return nil
}

func (r *RemediationRequestReconciler) orchestrateWorkflowExecution(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    log logr.Logger,
) error {
    log.Info("Orchestrating WorkflowExecution phase",
        "alertFingerprint", ar.Spec.TargetingData.Signal.Fingerprint,
    )

    // Create WorkflowExecution CRD
    start := time.Now()
    we, err := r.createWorkflowExecution(ctx, ar)
    if err != nil {
        log.Error(err, "Failed to create WorkflowExecution CRD")
        return err
    }
    log.Info("WorkflowExecution CRD created",
        "workflowexecution", we.Name,
        "totalSteps", len(we.Spec.WorkflowDefinition.Steps),
        "creationDuration", time.Since(start),
    )

    // Watch WorkflowExecution status
    log.V(1).Info("Watching WorkflowExecution status",
        "workflowexecution", we.Name,
    )

    return nil
}

func (r *RemediationRequestReconciler) watchWorkflowExecutionCompletion(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    log logr.Logger,
) error {
    weLog := log.WithValues("workflowexecution", ar.Status.WorkflowExecutionRef.Name)

    // Get WorkflowExecution CRD
    var we workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, client.ObjectKey{
        Name:      ar.Status.WorkflowExecutionRef.Name,
        Namespace: ar.Namespace,
    }, &we); err != nil {
        weLog.Error(err, "Failed to get WorkflowExecution status")
        return err
    }

    weLog.V(2).Info("Polling WorkflowExecution status",
        "phase", we.Status.Phase,
        "completedSteps", countCompletedSteps(we.Status.StepStatuses),
        "totalSteps", len(we.Spec.WorkflowDefinition.Steps),
        "duration", time.Since(we.Status.StartTime.Time),
    )

    // Check if WorkflowExecution completed
    if we.Status.Phase == "Completed" {
        duration := time.Since(we.Status.StartTime.Time)
        weLog.Info("WorkflowExecution completed",
            "duration", duration,
            "totalSteps", len(we.Spec.WorkflowDefinition.Steps),
        )

        // Mark RemediationRequest as Completed
        return r.completeRemediation(ctx, ar, log)
    } else if we.Status.Phase == "Failed" {
        duration := time.Since(we.Status.StartTime.Time)
        weLog.Warn("WorkflowExecution failed",
            "duration", duration,
            "failedStep", we.Status.CurrentStepName,
        )

        // Escalate to Notification Service
        return r.escalateToNotification(ctx, ar, "Workflow execution failed", log)
    }

    // Check for timeout
    timeout := ar.Spec.PhaseTimeouts.WorkflowExecution
    if time.Since(we.Status.StartTime.Time) > timeout {
        weLog.Warn("WorkflowExecution phase timeout detected",
            "timeout", timeout,
            "actualDuration", time.Since(we.Status.StartTime.Time),
        )
        return fmt.Errorf("workflow execution phase timeout")
    }

    return nil
}

func (r *RemediationRequestReconciler) escalateToNotification(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    reason string,
    log logr.Logger,
) error {
    log.Warn("Escalating to Notification Service",
        "reason", reason,
        "alertFingerprint", ar.Spec.TargetingData.Signal.Fingerprint,
        "currentPhase", ar.Status.Phase,
    )

    // Call Notification Service (HTTP POST /api/v1/notify/escalation)
    if err := r.notificationClient.SendEscalation(ctx, ar, reason); err != nil {
        log.Error(err, "Failed to send escalation notification")
        return err
    }

    log.Info("Escalation notification sent",
        "reason", reason,
    )

    return nil
}

func (r *RemediationRequestReconciler) completeRemediation(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    log logr.Logger,
) error {
    totalDuration := time.Since(ar.Status.StartTime.Time)
    log.Info("Alert remediation completed successfully",
        "totalDuration", totalDuration,
        "alertFingerprint", ar.Spec.TargetingData.Signal.Fingerprint,
    )

    // Update RemediationRequest status
    ar.Status.Phase = "Completed"
    ar.Status.CompletionTime = metav1.Now()
    if err := r.Status().Update(ctx, ar); err != nil {
        log.Error(err, "Failed to update RemediationRequest status")
        return err
    }

    return nil
}

// Debug logging for troubleshooting
func (r *RemediationRequestReconciler) debugLogTargetingData(
    log logr.Logger,
    targetingData *remediationv1.TargetingData,
) {
    log.V(2).Info("Targeting data details",
        "alertFingerprint", targetingData.Signal.Fingerprint,
        "environment", targetingData.Environment,
        "resourceNamespace", targetingData.KubernetesContext.ResourceNamespace,
        "resourceKind", targetingData.KubernetesContext.ResourceKind,
    )
}
```

**Log Correlation Example**:
```
INFO    Reconciling RemediationRequest      {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "phase": "processing", "alertFingerprint": "abc123", "environment": "production"}
INFO    Orchestrating RemediationProcessing phase {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "alertFingerprint": "abc123"}
INFO    SignalProcessing CRD created       {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "alertprocessing": "ar-xyz-ap", "creationDuration": "15ms"}
DEBUG   Polling RemediationProcessing status    {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "alertprocessing": "ar-xyz-ap", "phase": "enriching", "degradedMode": false, "duration": "150ms"}
INFO    RemediationProcessing completed         {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "alertprocessing": "ar-xyz-ap", "duration": "234ms", "degradedMode": false}
INFO    Orchestrating AIAnalysis phase    {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "alertFingerprint": "abc123"}
INFO    AIAnalysis CRD created            {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "aianalysis": "ar-xyz-aia", "creationDuration": "12ms"}
DEBUG   Polling AIAnalysis status         {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "aianalysis": "ar-xyz-aia", "phase": "investigating", "approvalStatus": "pending", "recommendationCount": 0, "duration": "2.3s"}
INFO    AIAnalysis completed (approved)   {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "aianalysis": "ar-xyz-aia", "duration": "5.2s", "topRecommendation": "restart-pod", "confidence": 0.92}
INFO    Orchestrating WorkflowExecution phase {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "alertFingerprint": "abc123"}
INFO    WorkflowExecution CRD created     {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "workflowexecution": "ar-xyz-we", "totalSteps": 3, "creationDuration": "18ms"}
DEBUG   Polling WorkflowExecution status  {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "workflowexecution": "ar-xyz-we", "phase": "executing", "completedSteps": 2, "totalSteps": 3, "duration": "3.5s"}
INFO    WorkflowExecution completed       {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "workflowexecution": "ar-xyz-we", "duration": "4.8s", "totalSteps": 3}
INFO    Alert remediation completed successfully {"remediationrequest": "default/ar-xyz", "correlationID": "abc-123-def", "totalDuration": "10.3s", "alertFingerprint": "abc123"}
```

---

### Distributed Tracing

**OpenTelemetry Integration**:

```go
package controller

import (
    "context"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("alertremediation-controller")

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ctx, span := tracer.Start(ctx, "RemediationRequest.Reconcile",
        trace.WithAttributes(
            attribute.String("alertremediation.name", req.Name),
            attribute.String("alertremediation.namespace", req.Namespace),
        ),
    )
    defer span.End()

    var ar remediationv1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &ar); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Failed to get RemediationRequest")
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Add CRD attributes to span
    span.SetAttributes(
        attribute.String("signal.fingerprint", ar.Spec.TargetingData.Signal.Fingerprint),
        attribute.String("alert.environment", ar.Spec.TargetingData.Environment),
        attribute.String("phase", ar.Status.Phase),
    )

    // Execute reconciliation with tracing
    result, err := r.reconcilePhase(ctx, &ar)

    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Reconciliation failed")
    } else {
        span.SetStatus(codes.Ok, "Reconciliation completed")
    }

    return result, err
}

func (r *RemediationRequestReconciler) orchestrateRemediationProcessing(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
) error {
    ctx, span := tracer.Start(ctx, "RemediationRequest.OrchestrateRemediationProcessing",
        trace.WithAttributes(
            attribute.String("signal.fingerprint", ar.Spec.TargetingData.Signal.Fingerprint),
        ),
    )
    defer span.End()

    // Create SignalProcessing CRD
    ap, err := r.createRemediationProcessing(ctx, ar)
    if err != nil {
        span.RecordError(err)
        return err
    }

    span.SetAttributes(
        attribute.String("alertprocessing.name", ap.Name),
    )

    return nil
}

func (r *RemediationRequestReconciler) orchestrateAIAnalysis(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
) error {
    ctx, span := tracer.Start(ctx, "RemediationRequest.OrchestrateAIAnalysis",
        trace.WithAttributes(
            attribute.String("signal.fingerprint", ar.Spec.TargetingData.Signal.Fingerprint),
        ),
    )
    defer span.End()

    // Create AIAnalysis CRD
    aia, err := r.createAIAnalysis(ctx, ar)
    if err != nil {
        span.RecordError(err)
        return err
    }

    span.SetAttributes(
        attribute.String("aianalysis.name", aia.Name),
    )

    return nil
}

func (r *RemediationRequestReconciler) orchestrateWorkflowExecution(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
) error {
    ctx, span := tracer.Start(ctx, "RemediationRequest.OrchestrateWorkflowExecution",
        trace.WithAttributes(
            attribute.String("signal.fingerprint", ar.Spec.TargetingData.Signal.Fingerprint),
        ),
    )
    defer span.End()

    // Create WorkflowExecution CRD
    we, err := r.createWorkflowExecution(ctx, ar)
    if err != nil {
        span.RecordError(err)
        return err
    }

    span.SetAttributes(
        attribute.String("workflowexecution.name", we.Name),
        attribute.Int("workflow.totalSteps", len(we.Spec.WorkflowDefinition.Steps)),
    )

    return nil
}
```

**Trace Visualization** (Jaeger) - End-to-End:
```
Trace ID: abc-123-def-456 (End-to-End Alert Remediation)
Span: RemediationRequest.Reconcile (10.5s)
  ├─ Span: RemediationRequest.OrchestrateRemediationProcessing (234ms)
  │   └─ Span: RemediationProcessing.Reconcile (234ms) [child service]
  │       ├─ Span: RemediationProcessing.EnrichAlert (180ms)
  │       └─ Span: RemediationProcessing.ClassifyEnvironment (54ms)
  ├─ Span: RemediationRequest.OrchestrateAIAnalysis (5.2s)
  │   └─ Span: AIAnalysis.Reconcile (5.2s) [child service]
  │       ├─ Span: AIAnalysis.InvestigateAlert (4.5s)
  │       │   └─ Span: HolmesGPT-API.Investigate (4.3s)
  │       └─ Span: AIAnalysis.ApproveRecommendation (700ms)
  │           └─ Span: RegoEngine.EvaluatePolicy (650ms)
  └─ Span: RemediationRequest.OrchestrateWorkflowExecution (4.8s)
      └─ Span: WorkflowExecution.Reconcile (4.8s) [child service]
          ├─ Span: WorkflowExecution.ExecuteStep [step-1] (1.2s)
          │   └─ Span: KubernetesExecution.Reconcile (1.2s) [child service] (DEPRECATED - ADR-025)
          │       └─ Span: Job.Execution (1.0s)
          ├─ Span: WorkflowExecution.ExecuteStep [step-2] (1.3s) [parallel]
          │   └─ Span: KubernetesExecution.Reconcile (1.3s) [child service] (DEPRECATED - ADR-025)
          └─ Span: WorkflowExecution.ExecuteStep [step-3] (2.3s)
              └─ Span: KubernetesExecution.Reconcile (2.3s) [child service] (DEPRECATED - ADR-025)
```

---

### Log Correlation IDs

**Propagating Correlation IDs Across Entire System**:

```go
package controller

import (
    "context"

    "github.com/google/uuid"
)

type correlationIDKey struct{}

// Extract correlation ID from incoming context (from Gateway Service)
func extractCorrelationID(ctx context.Context) string {
    if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
        return id
    }
    // Generate new ID if not present
    return uuid.New().String()
}

// Add correlation ID to all child CRD annotations
func (r *RemediationRequestReconciler) createChildCRDWithCorrelation(
    ctx context.Context,
    ar *remediationv1.RemediationRequest,
    childName string,
    childSpec interface{},
) error {
    correlationID := extractCorrelationID(ctx)

    // All child CRDs receive the same correlation ID
    annotations := map[string]string{
        "correlationID": correlationID,
    }

    // ... create child CRD with annotations
}
```

**Correlation Flow** (End-to-End):
```
Gateway Service (generate correlationID: abc-123)
    ↓
RemediationRequest (correlationID: abc-123) [ROOT]
    ↓ (creates RemediationProcessing with correlationID in annotation)
RemediationProcessing (correlationID: abc-123)
    ↓ (creates AIAnalysis with correlationID in annotation)
AIAnalysis (correlationID: abc-123)
    ↓ (creates WorkflowExecution with correlationID in annotation)
WorkflowExecution (correlationID: abc-123)
    ↓ (creates KubernetesExecution (DEPRECATED - ADR-025) with correlationID in annotation)
KubernetesExecution (correlationID: abc-123) (DEPRECATED - ADR-025)
    ↓ (creates Job with correlationID in label)
Kubernetes Job (correlationID: abc-123)
```

**Query Logs by Correlation ID** (Across All Services):
```bash
# Query all service logs with same correlation ID
for service in alertremediation alertprocessing aianalysis workflowexecution kubernetesexecution; do  # kubernetesexecution DEPRECATED - ADR-025
  echo "=== $service logs ==="
  kubectl logs -n kubernaut-system deployment/${service}-controller | grep "correlationID: abc-123"
done
```

---

### Debug Configuration

**Enable Debug Logging**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: alertremediation-controller-config
  namespace: kubernaut-system
data:
  log-level: "debug"  # error | warn | info | debug
  log-format: "json"  # json | console
  enable-tracing: "true"
  tracing-endpoint: "http://jaeger-collector.monitoring:14268/api/traces"
```

**Controller Startup with Debug Config**:

```go
package main

import (
    "flag"
    "os"

    "github.com/go-logr/zapr"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
    ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
    var logLevel string
    var logFormat string
    flag.StringVar(&logLevel, "log-level", "info", "Log level (error, warn, info, debug)")
    flag.StringVar(&logFormat, "log-format", "json", "Log format (json, console)")
    flag.Parse()

    // Configure zap logger
    zapLevel := parseLogLevel(logLevel)
    var zapConfig zap.Config
    if logFormat == "json" {
        zapConfig = zap.NewProductionConfig()
    } else {
        zapConfig = zap.NewDevelopmentConfig()
    }
    zapConfig.Level = zap.NewAtomicLevelAt(zapLevel)

    zapLog, err := zapConfig.Build()
    if err != nil {
        os.Exit(1)
    }

    ctrl.SetLogger(zapr.NewLogger(zapLog))

    // ... controller setup
}

func parseLogLevel(level string) zapcore.Level {
    switch level {
    case "debug":
        return zapcore.DebugLevel
    case "info":
        return zapcore.InfoLevel
    case "warn":
        return zapcore.WarnLevel
    case "error":
        return zapcore.ErrorLevel
    default:
        return zapcore.InfoLevel
    }
}
```

**Debug Query Examples**:

```bash
# Enable debug logging at runtime (requires restart)
kubectl set env deployment/alertremediation-controller -n kubernaut-system LOG_LEVEL=debug

# View debug logs for specific RemediationRequest
kubectl logs -n kubernaut-system deployment/alertremediation-controller --tail=1000 | grep "ar-xyz"

# View CRD orchestration logs
kubectl logs -n kubernaut-system deployment/alertremediation-controller --tail=1000 | grep "Orchestrating"

# View child CRD status polling (V(2) logs)
kubectl logs -n kubernaut-system deployment/alertremediation-controller --tail=1000 | grep "Polling.*status"

# View phase timeout detection
kubectl logs -n kubernaut-system deployment/alertremediation-controller --tail=1000 | grep "timeout detected"

# View end-to-end remediation completion
kubectl logs -n kubernaut-system deployment/alertremediation-controller --tail=1000 | grep "remediation completed"
```

---

## 📊 Metrics Endpoint

### Port Configuration

**Port**: 9090
**Path**: `/metrics`
**Format**: Prometheus
**Authentication**: TokenReviewer (Kubernetes-native)

### Prometheus Scrape Configuration

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: remediation-orchestrator
  namespace: kubernaut-system
spec:
  selector:
    matchLabels:
      app: remediation-orchestrator
  endpoints:
  - port: metrics
    path: /metrics
    interval: 30s
    bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    tlsConfig:
      insecureSkipVerify: true  # Use proper TLS in production
```

### Deployment Configuration

```yaml
# deploy/remediation-orchestrator-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: remediation-orchestrator
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: remediation-orchestrator
  template:
    metadata:
      labels:
        app: remediation-orchestrator
    spec:
      serviceAccountName: remediation-orchestrator-sa
      containers:
      - name: controller
        image: kubernaut/remediation-orchestrator:latest
        ports:
        - containerPort: 9090
          name: metrics
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080  # Health check endpoint (follows kube-apiserver pattern)
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
---
# deploy/remediation-orchestrator-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: remediation-orchestrator
  namespace: kubernaut-system
  labels:
    app: remediation-orchestrator
spec:
  selector:
    app: remediation-orchestrator
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
```

### Implementation Code

```go
// cmd/remediationorchestrator/main.go
package main

import (
    "flag"
    "os"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    "sigs.k8s.io/controller-runtime/pkg/metrics/server"

    remediationv1 "github.com/jordigilh/kubernaut/pkg/apis/remediation/v1"
    "github.com/jordigilh/kubernaut/pkg/remediationorchestrator"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    _ = remediationv1.AddToScheme(scheme)
}

func main() {
    var metricsAddr string
    var probeAddr string
    var enableLeaderElection bool

    flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "The address the metric endpoint binds to.")
    flag.StringVar(&probeAddr, "health-probe-bind-address", ":8080", "The address the probe endpoint binds to.")
    flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")

    // Zap logger options with controller-runtime integration (Split Strategy)
    opts := zap.Options{
        Development: true, // Set to false for production
    }
    opts.BindFlags(flag.CommandLine) // Adds --zap-log-level, --zap-encoder, etc.

    flag.Parse()

    ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme: scheme,
        Metrics: server.Options{
            BindAddress: metricsAddr,  // Port 9090 for metrics
        },
        HealthProbeBindAddress: probeAddr,  // Port 8080 for health checks
        LeaderElection:         enableLeaderElection,
        LeaderElectionID:       "remediation-orchestrator.kubernaut.io",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    if err = (&remediationorchestrator.RemediationRequestReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
        Log:    ctrl.Log.WithName("controllers").WithName("RemediationRequest"),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "RemediationRequest")
        os.Exit(1)
    }

    if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up health check")
        os.Exit(1)
    }

    if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
        setupLog.Error(err, "unable to set up ready check")
        os.Exit(1)
    }

    setupLog.Info("starting manager")
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        setupLog.Error(err, "problem running manager")
        os.Exit(1)
    }
}
```

**Key Configuration Points**:
- ✅ Metrics on port 9090 (standard for all CRD controllers)
- ✅ Health checks on port 8080 (follows kube-apiserver pattern)
- ✅ TokenReviewer authentication for secure Prometheus scraping
- ✅ ServiceMonitor for automatic discovery by Prometheus Operator

---
