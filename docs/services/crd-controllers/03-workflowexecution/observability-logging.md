## Observability & Logging

### Structured Logging Patterns

**Log Levels**:

| Level | Purpose | Examples |
|-------|---------|----------|
| **ERROR** | Unrecoverable errors, requires intervention | Step execution failure, child CRD creation failure |
| **WARN** | Recoverable errors, degraded mode | Step timeout (will retry), dependency resolution conflict |
| **INFO** | Normal operations, state transitions | Step start/complete, phase transitions, workflow completion |
| **DEBUG** | Detailed flow for troubleshooting | Dependency graph resolution, child CRD status polling |

**Structured Logging Implementation**:

```go
package controller

import (
    "context"
    "time"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkflowExecutionReconciler struct {
    client.Client
    Scheme *runtime.Scheme
    Log    logr.Logger
}

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Create request-scoped logger with correlation ID
    log := r.Log.WithValues(
        "workflowexecution", req.NamespacedName,
        "correlationID", extractCorrelationID(ctx),
    )

    var we workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &we); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Log phase transitions
    oldPhase := we.Status.Phase
    log.Info("Reconciling WorkflowExecution",
        "phase", we.Status.Phase,
        "totalSteps", len(we.Spec.WorkflowDefinition.Steps),
        "completedSteps", countCompletedSteps(we.Status.StepStatuses),
    )

    // Execute reconciliation logic with structured logging
    result, err := r.reconcilePhase(ctx, &we, log)

    // Log phase change
    if we.Status.Phase != oldPhase {
        log.Info("Phase transition",
            "from", oldPhase,
            "to", we.Status.Phase,
            "duration", time.Since(we.Status.StartTime.Time),
        )
    }

    if err != nil {
        log.Error(err, "Reconciliation failed",
            "phase", we.Status.Phase,
            "failedStep", we.Status.CurrentStepName,
        )
        return result, err
    }

    return result, nil
}

func (r *WorkflowExecutionReconciler) orchestrateSteps(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    log logr.Logger,
) error {
    log.V(1).Info("Starting step orchestration",
        "totalSteps", len(we.Spec.WorkflowDefinition.Steps),
        "parallelCapable", hasParallelSteps(we.Spec.WorkflowDefinition),
    )

    // Resolve step dependencies
    start := time.Now()
    dependencyGraph := r.buildDependencyGraph(we.Spec.WorkflowDefinition.Steps)
    log.V(1).Info("Dependency graph resolved",
        "duration", time.Since(start),
        "independentSteps", countIndependentSteps(dependencyGraph),
    )

    // Execute ready steps (no pending dependencies)
    readySteps := r.getReadySteps(we, dependencyGraph)
    log.Info("Executing ready steps",
        "stepCount", len(readySteps),
        "parallel", len(readySteps) > 1,
    )

    for _, stepName := range readySteps {
        if err := r.executeStep(ctx, we, stepName, log); err != nil {
            log.Error(err, "Step execution failed",
                "stepName", stepName,
                "action", we.Spec.WorkflowDefinition.Steps[stepName].Action,
            )
            return err
        }
    }

    log.Info("Step orchestration completed",
        "executedSteps", len(readySteps),
        "totalDuration", time.Since(start),
    )

    return nil
}

func (r *WorkflowExecutionReconciler) executeStep(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    stepName string,
    log logr.Logger,
) error {
    step := we.Spec.WorkflowDefinition.Steps[stepName]
    stepLog := log.WithValues("stepName", stepName, "action", step.Action)

    stepLog.Info("Executing workflow step",
        "dependencies", step.Dependencies,
        "timeout", step.Timeout,
    )

    // Create KubernetesExecution CRD for step
    start := time.Now()
    ke, err := r.createKubernetesExecution(ctx, we, step)
    if err != nil {
        stepLog.Error(err, "Failed to create KubernetesExecution CRD")
        return err
    }

    stepLog.V(1).Info("KubernetesExecution CRD created",
        "kubernetesexecution", ke.Name,
        "creationDuration", time.Since(start),
    )

    // Update step status
    we.Status.StepStatuses[stepName] = workflowexecutionv1.StepStatus{
        Name:      stepName,
        Status:    "Running",
        StartTime: metav1.Now(),
    }
    if err := r.Status().Update(ctx, we); err != nil {
        stepLog.Error(err, "Failed to update step status")
        return err
    }

    stepLog.Info("Workflow step started",
        "kubernetesexecution", ke.Name,
    )

    return nil
}

func (r *WorkflowExecutionReconciler) watchStepCompletion(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    stepName string,
    log logr.Logger,
) error {
    stepLog := log.WithValues("stepName", stepName)

    // Get KubernetesExecution CRD for this step
    keName := fmt.Sprintf("%s-%s", we.Name, stepName)
    var ke kubernetesexecutionv1.KubernetesExecution
    if err := r.Get(ctx, client.ObjectKey{
        Name:      keName,
        Namespace: we.Namespace,
    }, &ke); err != nil {
        stepLog.Error(err, "Failed to get KubernetesExecution status")
        return err
    }

    stepLog.V(2).Info("Polling step status",
        "keStatus", ke.Status.Phase,
        "duration", time.Since(ke.Status.StartTime.Time),
    )

    // Check if step completed
    if ke.Status.Phase == "Completed" {
        duration := time.Since(we.Status.StepStatuses[stepName].StartTime.Time)
        stepLog.Info("Workflow step completed",
            "duration", duration,
            "result", ke.Status.Result,
        )

        // Update step status
        we.Status.StepStatuses[stepName] = workflowexecutionv1.StepStatus{
            Name:         stepName,
            Status:       "Completed",
            StartTime:    we.Status.StepStatuses[stepName].StartTime,
            CompleteTime: metav1.Now(),
            Result:       ke.Status.Result,
        }
        if err := r.Status().Update(ctx, we); err != nil {
            stepLog.Error(err, "Failed to update completed step status")
            return err
        }
    } else if ke.Status.Phase == "Failed" {
        duration := time.Since(we.Status.StepStatuses[stepName].StartTime.Time)
        stepLog.Error(nil, "Workflow step failed",
            "duration", duration,
            "errorMessage", ke.Status.ErrorMessage,
        )

        // Update step status
        we.Status.StepStatuses[stepName] = workflowexecutionv1.StepStatus{
            Name:         stepName,
            Status:       "Failed",
            StartTime:    we.Status.StepStatuses[stepName].StartTime,
            CompleteTime: metav1.Now(),
            ErrorMessage: ke.Status.ErrorMessage,
        }
        if err := r.Status().Update(ctx, we); err != nil {
            stepLog.Error(err, "Failed to update failed step status")
            return err
        }

        return fmt.Errorf("step execution failed: %s", ke.Status.ErrorMessage)
    }

    return nil
}

// Debug logging for troubleshooting
func (r *WorkflowExecutionReconciler) debugLogDependencyGraph(
    log logr.Logger,
    graph map[string][]string,
) {
    log.V(2).Info("Dependency graph details",
        "totalNodes", len(graph),
        "graph", fmt.Sprintf("%+v", graph),
    )
}
```

**Log Correlation Example**:
```
INFO    Reconciling WorkflowExecution    {"workflowexecution": "default/workflow-xyz", "correlationID": "abc-123-def", "phase": "executing", "totalSteps": 5, "completedSteps": 2}
INFO    Starting step orchestration      {"workflowexecution": "default/workflow-xyz", "correlationID": "abc-123-def", "totalSteps": 5, "parallelCapable": true}
DEBUG   Dependency graph resolved        {"workflowexecution": "default/workflow-xyz", "correlationID": "abc-123-def", "duration": "5ms", "independentSteps": 2}
INFO    Executing ready steps            {"workflowexecution": "default/workflow-xyz", "correlationID": "abc-123-def", "stepCount": 2, "parallel": true}
INFO    Executing workflow step          {"workflowexecution": "default/workflow-xyz", "correlationID": "abc-123-def", "stepName": "restart-pod", "action": "restart-pod", "dependencies": [], "timeout": "5m"}
INFO    Workflow step started            {"workflowexecution": "default/workflow-xyz", "correlationID": "abc-123-def", "stepName": "restart-pod", "kubernetesexecution": "workflow-xyz-restart-pod"}
INFO    Workflow step completed          {"workflowexecution": "default/workflow-xyz", "correlationID": "abc-123-def", "stepName": "restart-pod", "duration": "2.3s", "result": "success"}
```

---

### Distributed Tracing

**OpenTelemetry Integration**:

```go
package controller

import (
    "context"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("workflowexecution-controller")

func (r *WorkflowExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ctx, span := tracer.Start(ctx, "WorkflowExecution.Reconcile",
        trace.WithAttributes(
            attribute.String("workflowexecution.name", req.Name),
            attribute.String("workflowexecution.namespace", req.Namespace),
        ),
    )
    defer span.End()

    var we workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &we); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Failed to get WorkflowExecution")
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Add CRD attributes to span
    span.SetAttributes(
        attribute.Int("workflow.totalSteps", len(we.Spec.WorkflowDefinition.Steps)),
        attribute.Int("workflow.completedSteps", countCompletedSteps(we.Status.StepStatuses)),
        attribute.String("phase", we.Status.Phase),
    )

    // Execute reconciliation with tracing
    result, err := r.reconcilePhase(ctx, &we)

    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Reconciliation failed")
    } else {
        span.SetStatus(codes.Ok, "Reconciliation completed")
    }

    return result, err
}

func (r *WorkflowExecutionReconciler) orchestrateSteps(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
) error {
    ctx, span := tracer.Start(ctx, "WorkflowExecution.OrchestrateSteps",
        trace.WithAttributes(
            attribute.Int("steps.total", len(we.Spec.WorkflowDefinition.Steps)),
        ),
    )
    defer span.End()

    // Build dependency graph
    dependencyGraph := r.buildDependencyGraph(we.Spec.WorkflowDefinition.Steps)
    span.SetAttributes(
        attribute.Int("steps.independent", countIndependentSteps(dependencyGraph)),
    )

    // Execute ready steps with individual spans
    readySteps := r.getReadySteps(we, dependencyGraph)
    span.SetAttributes(
        attribute.Int("steps.ready", len(readySteps)),
        attribute.Bool("execution.parallel", len(readySteps) > 1),
    )

    for _, stepName := range readySteps {
        if err := r.executeStepWithTracing(ctx, we, stepName); err != nil {
            span.RecordError(err)
            return err
        }
    }

    return nil
}

func (r *WorkflowExecutionReconciler) executeStepWithTracing(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    stepName string,
) error {
    step := we.Spec.WorkflowDefinition.Steps[stepName]

    ctx, span := tracer.Start(ctx, "WorkflowExecution.ExecuteStep",
        trace.WithAttributes(
            attribute.String("step.name", stepName),
            attribute.String("step.action", step.Action),
            attribute.StringSlice("step.dependencies", step.Dependencies),
        ),
    )
    defer span.End()

    // Create KubernetesExecution CRD
    ke, err := r.createKubernetesExecution(ctx, we, step)
    if err != nil {
        span.RecordError(err)
        return err
    }

    // 🚨 CRITICAL: Sanitize step parameters before adding to trace
    sanitizedParams := sanitizeWorkflowPayload(step.Parameters.String())

    span.SetAttributes(
        attribute.String("kubernetesexecution.name", ke.Name),
        attribute.String("step.parameters", sanitizedParams),  // Sanitized version only
    )

    return nil
}

func (r *WorkflowExecutionReconciler) watchStepCompletionWithTracing(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    stepName string,
) error {
    ctx, span := tracer.Start(ctx, "WorkflowExecution.WatchStepCompletion",
        trace.WithAttributes(
            attribute.String("step.name", stepName),
        ),
    )
    defer span.End()

    // Get KubernetesExecution status
    keName := fmt.Sprintf("%s-%s", we.Name, stepName)
    var ke kubernetesexecutionv1.KubernetesExecution
    if err := r.Get(ctx, client.ObjectKey{
        Name:      keName,
        Namespace: we.Namespace,
    }, &ke); err != nil {
        span.RecordError(err)
        return err
    }

    span.SetAttributes(
        attribute.String("kubernetesexecution.status", ke.Status.Phase),
        attribute.Duration("step.duration", time.Since(ke.Status.StartTime.Time)),
    )

    if ke.Status.Phase == "Completed" {
        span.SetStatus(codes.Ok, "Step completed successfully")
    } else if ke.Status.Phase == "Failed" {
        span.SetStatus(codes.Error, ke.Status.ErrorMessage)
    }

    return nil
}
```

**Trace Visualization** (Jaeger):
```
Trace ID: abc-123-def-456
Span: WorkflowExecution.Reconcile (3.5s)
  ├─ Span: WorkflowExecution.OrchestrateSteps (3.2s)
  │   ├─ Span: WorkflowExecution.ExecuteStep [step-1] (1.2s)
  │   │   └─ Span: KubernetesExecution.CreateCRD (50ms)
  │   ├─ Span: WorkflowExecution.ExecuteStep [step-2] (1.2s) [parallel]
  │   │   └─ Span: KubernetesExecution.CreateCRD (45ms)
  │   ├─ Span: WorkflowExecution.WatchStepCompletion [step-1] (500ms)
  │   ├─ Span: WorkflowExecution.WatchStepCompletion [step-2] (600ms)
  │   └─ Span: WorkflowExecution.ExecuteStep [step-3] (800ms) [depends on 1,2]
  │       └─ Span: KubernetesExecution.CreateCRD (40ms)
  └─ Span: WorkflowExecution.UpdateStatus (300ms)
```

---

### Log Correlation IDs

**Propagating Correlation IDs Across Services**:

```go
package controller

import (
    "context"

    "github.com/google/uuid"
)

type correlationIDKey struct{}

// Extract correlation ID from incoming context (from RemediationRequest)
func extractCorrelationID(ctx context.Context) string {
    if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
        return id
    }
    // Generate new ID if not present
    return uuid.New().String()
}

// Add correlation ID to child CRD annotations
func (r *WorkflowExecutionReconciler) createKubernetesExecution(
    ctx context.Context,
    we *workflowexecutionv1.WorkflowExecution,
    step *workflowexecutionv1.WorkflowStep,
) (*kubernetesexecutionv1.KubernetesExecution, error) {
    correlationID := extractCorrelationID(ctx)

    ke := &kubernetesexecutionv1.KubernetesExecution{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-%s", we.Name, step.Name),
            Namespace: we.Namespace,
            Annotations: map[string]string{
                "correlationID": correlationID,  // Propagate to child CRD
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(we, workflowexecutionv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: kubernetesexecutionv1.KubernetesExecutionSpec{
            Action:     step.Action,
            Parameters: step.Parameters,
        },
    }

    if err := r.Create(ctx, ke); err != nil {
        return nil, err
    }

    return ke, nil
}
```

**Correlation Flow**:
```
RemediationRequest (correlationID: abc-123)
    ↓ (creates WorkflowExecution with correlationID in annotation)
WorkflowExecution Controller (correlationID: abc-123)
    ↓ (creates KubernetesExecution with correlationID in annotation)
KubernetesExecution Controller (correlationID: abc-123)
    ↓ (creates Kubernetes Job with correlationID in label)
Kubernetes Job (correlationID: abc-123)
    ↓ (logs with correlationID: abc-123)
```

**Query Logs by Correlation ID**:
```bash
kubectl logs -n kubernaut-system deployment/workflowexecution-controller | grep "correlationID: abc-123"
```

---

### Debug Configuration

**Enable Debug Logging**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: workflowexecution-controller-config
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
kubectl set env deployment/workflowexecution-controller -n kubernaut-system LOG_LEVEL=debug

# View debug logs for specific WorkflowExecution
kubectl logs -n kubernaut-system deployment/workflowexecution-controller --tail=1000 | grep "workflow-xyz"

# View step execution logs
kubectl logs -n kubernaut-system deployment/workflowexecution-controller --tail=1000 | grep "Executing workflow step"

# View dependency graph resolution (V(2) logs)
kubectl logs -n kubernaut-system deployment/workflowexecution-controller --tail=1000 | grep "Dependency graph"

# View parallel step execution
kubectl logs -n kubernaut-system deployment/workflowexecution-controller --tail=1000 | grep "parallel.*true"
```

---

## Metrics Endpoint

### Port Configuration

- **Port 9090**: Metrics endpoint
- **Port 8080**: Health probes (follows kube-apiserver pattern)
- **Endpoint**: `/metrics`
- **Format**: Prometheus text format
- **Authentication**: Kubernetes TokenReviewer API (validates ServiceAccount tokens)

### Prometheus ServiceMonitor

```yaml
# deploy/workflow-execution-servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: workflow-execution-metrics
  namespace: kubernaut-system
  labels:
    app: workflow-execution
    prometheus: kubernaut
spec:
  selector:
    matchLabels:
      app: workflow-execution
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
    scheme: https
    tlsConfig:
      insecureSkipVerify: true
    bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
```

### Deployment Configuration

```yaml
# deploy/workflow-execution-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: workflow-execution
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: workflow-execution
  template:
    metadata:
      labels:
        app: workflow-execution
    spec:
      serviceAccountName: workflow-execution-sa
      containers:
      - name: controller
        image: kubernaut/workflow-execution:latest
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
# deploy/workflow-execution-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: workflow-execution-metrics
  namespace: kubernaut-system
  labels:
    app: workflow-execution
spec:
  selector:
    app: workflow-execution
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
  type: ClusterIP
```

### Implementation Code

```go
// cmd/workflow-execution/main.go
package main

import (
    "flag"
    "os"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    "sigs.k8s.io/controller-runtime/pkg/metrics/server"
    "sigs.k8s.io/controller-runtime/pkg/healthz"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/pkg/apis/workflowexecution/v1"
    "github.com/jordigilh/kubernaut/pkg/workflowexecution"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    _ = workflowexecutionv1.AddToScheme(scheme)
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
        LeaderElectionID:       "workflow-execution.kubernaut.io",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    if err = (&workflowexecution.WorkflowExecutionReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
        Log:    ctrl.Log.WithName("controllers").WithName("WorkflowExecution"),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "WorkflowExecution")
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
