## Observability & Logging

**Version**: 3.1.0
**Last Updated**: 2025-12-02
**CRD API Group**: `workflowexecution.kubernaut.ai/v1alpha1`
**Health/Ready Port**: 8081 (per [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md))
**Status**: ✅ Updated for Tekton Architecture

---

## Changelog

### Version 3.1.0 (2025-12-02)
- ✅ **Updated**: Code examples for Tekton PipelineRun patterns

### Version 3.0.0 (2025-12-02)
- ✅ **Updated**: API group from `.io` to `.ai`
- ✅ **Updated**: Health port from 8080 to 8081
- ✅ **Updated**: LeaderElectionID to `workflowexecution.kubernaut.ai`

---

### Structured Logging Patterns

**Log Levels**:

| Level | Purpose | Examples |
|-------|---------|----------|
| **ERROR** | Unrecoverable errors, requires intervention | PipelineRun creation failure, resource lock conflict |
| **WARN** | Recoverable errors, degraded mode | PipelineRun timeout (will retry), target resource busy |
| **INFO** | Normal operations, state transitions | PipelineRun created, phase transitions, workflow completion |
| **DEBUG** | Detailed flow for troubleshooting | Resource lock check, PipelineRun status polling |

**Structured Logging Implementation**:

```go
package controller

import (
    "context"
    "time"

    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
    tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"

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

    var wfe workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Log phase transitions
    oldPhase := wfe.Status.Phase
    log.Info("Reconciling WorkflowExecution",
        "phase", wfe.Status.Phase,
        "workflowId", wfe.Spec.WorkflowRef.WorkflowID,
        "targetResource", wfe.Spec.TargetResource,
    )

    // Execute reconciliation logic with structured logging
    result, err := r.reconcilePhase(ctx, &wfe, log)

    // Log phase change
    if wfe.Status.Phase != oldPhase {
        log.Info("Phase transition",
            "from", oldPhase,
            "to", wfe.Status.Phase,
            "duration", time.Since(wfe.Status.StartTime.Time),
        )
    }

    if err != nil {
        log.Error(err, "Reconciliation failed",
            "phase", wfe.Status.Phase,
            "workflowId", wfe.Spec.WorkflowRef.WorkflowID,
        )
        return result, err
    }

    return result, nil
}

func (r *WorkflowExecutionReconciler) createPipelineRun(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    log logr.Logger,
) error {
    log.Info("Creating Tekton PipelineRun",
        "workflowId", wfe.Spec.WorkflowRef.WorkflowID,
        "containerImage", wfe.Spec.WorkflowRef.ContainerImage,
        "targetResource", wfe.Spec.TargetResource,
    )

    // Create PipelineRun using bundle resolver
    pipelineRun := r.buildPipelineRun(wfe)

    start := time.Now()
    if err := r.Create(ctx, pipelineRun); err != nil {
        log.Error(err, "Failed to create PipelineRun")
        return err
    }

    log.Info("PipelineRun created successfully",
        "pipelineRun", pipelineRun.Name,
        "creationDuration", time.Since(start),
    )

    return nil
}

func (r *WorkflowExecutionReconciler) watchPipelineRunStatus(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
    log logr.Logger,
) error {
    pipelineRunName := wfe.Name
    log = log.WithValues("pipelineRun", pipelineRunName)

    var pr tektonv1.PipelineRun
    if err := r.Get(ctx, client.ObjectKey{
        Name:      pipelineRunName,
        Namespace: wfe.Namespace,
    }, &pr); err != nil {
        log.Error(err, "Failed to get PipelineRun status")
        return err
    }

    // Log status based on conditions
    for _, condition := range pr.Status.Conditions {
        if condition.Type == "Succeeded" {
            switch condition.Status {
            case "True":
                log.Info("PipelineRun completed successfully",
                    "duration", pr.Status.CompletionTime.Sub(pr.Status.StartTime.Time),
                )
            case "False":
                log.Error(nil, "PipelineRun failed",
                    "reason", condition.Reason,
                    "message", condition.Message,
                )
            default:
                log.V(1).Info("PipelineRun still running",
                    "status", condition.Status,
                    "reason", condition.Reason,
                )
            }
        }
    }

    return nil
}
```

**Log Correlation Example**:
```
INFO    Reconciling WorkflowExecution    {"workflowexecution": "default/wfe-xyz", "correlationID": "abc-123-def", "phase": "Pending", "workflowId": "increase-memory-conservative", "targetResource": "production/deployment/payment-service"}
INFO    Creating Tekton PipelineRun      {"workflowexecution": "default/wfe-xyz", "correlationID": "abc-123-def", "workflowId": "increase-memory-conservative", "containerImage": "ghcr.io/kubernaut/workflows/increase-memory@sha256:abc123"}
INFO    PipelineRun created successfully {"workflowexecution": "default/wfe-xyz", "correlationID": "abc-123-def", "pipelineRun": "wfe-xyz", "creationDuration": "45ms"}
INFO    Phase transition                 {"workflowexecution": "default/wfe-xyz", "correlationID": "abc-123-def", "from": "Pending", "to": "Running"}
INFO    PipelineRun completed successfully {"workflowexecution": "default/wfe-xyz", "correlationID": "abc-123-def", "pipelineRun": "wfe-xyz", "duration": "2m15s"}
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

    var wfe workflowexecutionv1.WorkflowExecution
    if err := r.Get(ctx, req.NamespacedName, &wfe); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Failed to get WorkflowExecution")
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Add CRD attributes to span
    span.SetAttributes(
        attribute.String("workflow.id", wfe.Spec.WorkflowRef.WorkflowID),
        attribute.String("target.resource", wfe.Spec.TargetResource),
        attribute.String("phase", string(wfe.Status.Phase)),
    )

    // Execute reconciliation with tracing
    result, err := r.reconcilePhase(ctx, &wfe)

    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Reconciliation failed")
    } else {
        span.SetStatus(codes.Ok, "Reconciliation completed")
    }

    return result, err
}

func (r *WorkflowExecutionReconciler) createPipelineRunWithTracing(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) error {
    ctx, span := tracer.Start(ctx, "WorkflowExecution.CreatePipelineRun",
        trace.WithAttributes(
            attribute.String("workflow.id", wfe.Spec.WorkflowRef.WorkflowID),
            attribute.String("container.image", wfe.Spec.WorkflowRef.ContainerImage),
        ),
    )
    defer span.End()

    // Create PipelineRun
    pr := r.buildPipelineRun(wfe)
    if err := r.Create(ctx, pr); err != nil {
        span.RecordError(err)
        return err
    }

    span.SetAttributes(
        attribute.String("pipelinerun.name", pr.Name),
    )

    return nil
}
```

**Trace Visualization** (Jaeger):
```
Trace ID: abc-123-def-456
Span: WorkflowExecution.Reconcile (2.5s)
  ├─ Span: WorkflowExecution.CheckResourceLock (15ms)
  ├─ Span: WorkflowExecution.CreatePipelineRun (50ms)
  ├─ Span: WorkflowExecution.WatchPipelineRunStatus (2.3s)
  │   ├─ Poll: PipelineRun status check (100ms)
  │   ├─ Poll: PipelineRun status check (100ms)
  │   └─ Poll: PipelineRun completed (50ms)
  └─ Span: WorkflowExecution.UpdateStatus (50ms)
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

// Add correlation ID to PipelineRun labels for tracing
func (r *WorkflowExecutionReconciler) buildPipelineRun(
    wfe *workflowexecutionv1.WorkflowExecution,
) *tektonv1.PipelineRun {
    correlationID := wfe.Labels["kubernaut.ai/correlation-id"]

    return &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      wfe.Name,
            Namespace: wfe.Namespace,
            Labels: map[string]string{
                "kubernaut.ai/correlation-id":     correlationID,
                "kubernaut.ai/workflow-execution": wfe.Name,
                "kubernaut.ai/workflow-id":        wfe.Spec.WorkflowRef.WorkflowID,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(wfe, workflowexecutionv1.GroupVersion.WithKind("WorkflowExecution")),
            },
        },
        Spec: tektonv1.PipelineRunSpec{
            PipelineRef: &tektonv1.PipelineRef{
                ResolverRef: tektonv1.ResolverRef{
                    Resolver: "bundles",
                    Params: []tektonv1.Param{
                        {Name: "bundle", Value: tektonv1.ParamValue{StringVal: wfe.Spec.WorkflowRef.ContainerImage}},
                        {Name: "name", Value: tektonv1.ParamValue{StringVal: "workflow"}},
                    },
                },
            },
            Params: r.buildParams(wfe),
        },
    }
}
```

**Correlation Flow**:
```
RemediationRequest (correlationID: abc-123)
    ↓ (creates WorkflowExecution with correlationID in label)
WorkflowExecution Controller (correlationID: abc-123)
    ↓ (creates PipelineRun with correlationID in label)
Tekton PipelineRun (correlationID: abc-123)
    ↓ (TaskRuns inherit labels)
Tekton TaskRuns (correlationID: abc-123)
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
kubectl logs -n kubernaut-system deployment/workflowexecution-controller --tail=1000 | grep "wfe-xyz"

# View PipelineRun creation logs
kubectl logs -n kubernaut-system deployment/workflowexecution-controller --tail=1000 | grep "Creating Tekton PipelineRun"

# View resource lock check logs
kubectl logs -n kubernaut-system deployment/workflowexecution-controller --tail=1000 | grep "Resource lock"
```

---

## Metrics Endpoint

### Port Configuration

- **Port 9090**: Metrics endpoint
- **Port 8081**: Health probes (per [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md))
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
            port: 8081  # Health check endpoint (DD-TEST-001)
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
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
// cmd/workflowexecution/main.go
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
    flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
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
        HealthProbeBindAddress: probeAddr,  // Port 8081 for health checks
        LeaderElection:         enableLeaderElection,
        LeaderElectionID:       "workflowexecution.kubernaut.ai",
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
- ✅ Health checks on port 8081 (per [DD-TEST-001](../../../architecture/decisions/DD-TEST-001-port-allocation-strategy.md))
- ✅ TokenReviewer authentication for secure Prometheus scraping
- ✅ ServiceMonitor for automatic discovery by Prometheus Operator

---
