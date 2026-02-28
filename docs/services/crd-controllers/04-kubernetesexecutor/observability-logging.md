## Observability & Logging

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

### Structured Logging Patterns

**Log Levels**:

| Level | Purpose | Examples |
|-------|---------|----------|
| **ERROR** | Unrecoverable errors, requires intervention | Job creation failure, RBAC setup failure, invalid action |
| **WARN** | Recoverable errors, degraded mode | Job timeout (will mark step failed), dry-run validation warning |
| **INFO** | Normal operations, state transitions | Job creation, Job completion, action execution success |
| **DEBUG** | Detailed flow for troubleshooting | kubectl command construction, Job pod logs, RBAC creation details |

**Structured Logging Implementation**:

```go
package controller

import (
    "context"
    "time"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"

    "github.com/go-logr/logr"
    batchv1 "k8s.io/api/batch/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesExecutionReconciler struct {
    client.Client
    Scheme *runtime.Scheme
    Log    logr.Logger
}

func (r *KubernetesExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Create request-scoped logger with correlation ID
    log := r.Log.WithValues(
        "kubernetesexecution", req.NamespacedName,
        "correlationID", extractCorrelationID(ctx),
    )

    var ke kubernetesexecutionv1.KubernetesExecution
    if err := r.Get(ctx, req.NamespacedName, &ke); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Log phase transitions
    oldPhase := ke.Status.Phase
    log.Info("Reconciling KubernetesExecution",
        "phase", ke.Status.Phase,
        "action", ke.Spec.Action.Name,
        "targetResource", ke.Spec.Action.TargetResource,
    )

    // Execute reconciliation logic with structured logging
    result, err := r.reconcilePhase(ctx, &ke, log)

    // Log phase change
    if ke.Status.Phase != oldPhase {
        log.Info("Phase transition",
            "from", oldPhase,
            "to", ke.Status.Phase,
            "duration", time.Since(ke.Status.StartTime.Time),
        )
    }

    if err != nil {
        log.Error(err, "Reconciliation failed",
            "phase", ke.Status.Phase,
            "action", ke.Spec.Action.Name,
        )
        return result, err
    }

    return result, nil
}

func (r *KubernetesExecutionReconciler) prepareExecution(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
    log logr.Logger,
) error {
    log.V(1).Info("Preparing action execution",
        "action", ke.Spec.Action.Name,
        "targetNamespace", ke.Spec.Action.TargetNamespace,
        "targetResource", ke.Spec.Action.TargetResource,
    )

    // Create per-action ServiceAccount
    start := time.Now()
    sa, err := r.createServiceAccountForAction(ctx, ke)
    if err != nil {
        log.Error(err, "Failed to create ServiceAccount")
        return err
    }
    log.V(1).Info("ServiceAccount created",
        "serviceAccount", sa.Name,
        "duration", time.Since(start),
    )

    // Create per-action RBAC (Role + RoleBinding)
    start = time.Now()
    role, binding, err := r.createRBACForAction(ctx, ke)
    if err != nil {
        log.Error(err, "Failed to create RBAC")
        return err
    }
    log.V(1).Info("RBAC created",
        "role", role.Name,
        "roleBinding", binding.Name,
        "permissions", len(role.Rules),
        "duration", time.Since(start),
    )

    log.Info("Action execution prepared",
        "totalPreparationDuration", time.Since(ke.Status.StartTime.Time),
    )

    return nil
}

func (r *KubernetesExecutionReconciler) executeAction(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
    log logr.Logger,
) error {
    log.Info("Executing Kubernetes action",
        "action", ke.Spec.Action.Name,
        "targetResource", ke.Spec.Action.TargetResource,
        "dryRun", ke.Spec.DryRun,
    )

    // Build kubectl command
    command := r.buildKubectlCommand(ke.Spec.Action, ke.Spec.DryRun)

    // 🚨 CRITICAL: Sanitize command before logging
    sanitizedCommand := sanitizeCommand(command)
    log.V(1).Info("kubectl command prepared",
        "command", sanitizedCommand,  // Sanitized version only
        "serviceAccount", r.getServiceAccountForAction(ke.Spec.Action.Name),
    )

    // Create Kubernetes Job for execution
    start := time.Now()
    job, err := r.createExecutionJob(ctx, ke, command)
    if err != nil {
        log.Error(err, "Failed to create Kubernetes Job")
        return err
    }
    log.Info("Kubernetes Job created",
        "job", job.Name,
        "creationDuration", time.Since(start),
    )

    // Watch Job completion
    if err := r.watchJobCompletion(ctx, ke, job, log); err != nil {
        log.Error(err, "Job execution failed")
        return err
    }

    log.Info("Kubernetes action executed successfully",
        "action", ke.Spec.Action.Name,
        "totalDuration", time.Since(start),
    )

    return nil
}

func (r *KubernetesExecutionReconciler) watchJobCompletion(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
    job *batchv1.Job,
    log logr.Logger,
) error {
    jobLog := log.WithValues("job", job.Name)

    jobLog.V(2).Info("Watching Job status",
        "phase", job.Status.Active,
        "succeeded", job.Status.Succeeded,
        "failed", job.Status.Failed,
    )

    // Check if Job completed successfully
    if job.Status.Succeeded > 0 {
        duration := time.Since(job.Status.StartTime.Time)
        jobLog.Info("Job completed successfully",
            "duration", duration,
        )

        // Capture Job pod logs (sanitized)
        logs, err := r.captureJobLogs(ctx, ke, job)
        if err != nil {
            jobLog.Warn("Failed to capture Job logs",
                "error", err,
            )
        } else {
            // 🚨 CRITICAL: Sanitize logs before storing
            sanitizedLogs := sanitizeCommand(logs)
            ke.Status.ExecutionLogs = sanitizedLogs
            jobLog.V(1).Info("Job logs captured",
                "logSize", len(sanitizedLogs),
            )
        }

        return nil
    } else if job.Status.Failed > 0 {
        duration := time.Since(job.Status.StartTime.Time)
        jobLog.Error(nil, "Job failed",
            "duration", duration,
            "failureReason", job.Status.Conditions[0].Reason,
        )

        // Capture Job pod logs (sanitized)
        logs, err := r.captureJobLogs(ctx, ke, job)
        if err != nil {
            jobLog.Warn("Failed to capture Job logs",
                "error", err,
            )
        } else {
            // 🚨 CRITICAL: Sanitize logs before storing
            sanitizedLogs := sanitizeCommand(logs)
            ke.Status.ExecutionLogs = sanitizedLogs
            ke.Status.ErrorMessage = sanitizedLogs  // Sanitized error message
        }

        return fmt.Errorf("job execution failed: %s", job.Status.Conditions[0].Reason)
    }

    return nil
}

// Debug logging for troubleshooting
func (r *KubernetesExecutionReconciler) debugLogJobDetails(
    log logr.Logger,
    job *batchv1.Job,
) {
    log.V(2).Info("Job details",
        "podTemplate", job.Spec.Template.Spec.Containers[0].Image,
        "serviceAccount", job.Spec.Template.Spec.ServiceAccountName,
        "restartPolicy", job.Spec.Template.Spec.RestartPolicy,
        "backoffLimit", *job.Spec.BackoffLimit,
    )
}
```

**Log Correlation Example**:
```
INFO    Reconciling KubernetesExecution  {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "phase": "validating", "action": "restart-pod", "targetResource": "production/web-app-789"}
INFO    Preparing action execution       {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "action": "restart-pod", "targetNamespace": "production", "targetResource": "web-app-789"}
DEBUG   ServiceAccount created           {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "serviceAccount": "restart-pod-sa", "duration": "12ms"}
DEBUG   RBAC created                     {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "role": "restart-pod-role", "roleBinding": "restart-pod-binding", "permissions": 2, "duration": "18ms"}
INFO    Action execution prepared        {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "totalPreparationDuration": "35ms"}
INFO    Executing Kubernetes action      {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "action": "restart-pod", "targetResource": "production/web-app-789", "dryRun": false}
DEBUG   kubectl command prepared         {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "command": "kubectl delete pod web-app-789 -n production", "serviceAccount": "restart-pod-sa"}
INFO    Kubernetes Job created           {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "job": "ke-xyz-job", "creationDuration": "25ms"}
DEBUG   Watching Job status              {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "job": "ke-xyz-job", "phase": 1, "succeeded": 0, "failed": 0}
INFO    Job completed successfully       {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "job": "ke-xyz-job", "duration": "1.2s"}
INFO    Kubernetes action executed successfully {"kubernetesexecution": "default/ke-xyz", "correlationID": "abc-123-def", "action": "restart-pod", "totalDuration": "1.3s"}
```

---

### Distributed Tracing

**OpenTelemetry Integration**:

```go
package controller

import (
    "context"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
    batchv1 "k8s.io/api/batch/v1"
)

var tracer = otel.Tracer("kubernetesexecution-controller")

func (r *KubernetesExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ctx, span := tracer.Start(ctx, "KubernetesExecution.Reconcile",
        trace.WithAttributes(
            attribute.String("kubernetesexecution.name", req.Name),
            attribute.String("kubernetesexecution.namespace", req.Namespace),
        ),
    )
    defer span.End()

    var ke kubernetesexecutionv1.KubernetesExecution
    if err := r.Get(ctx, req.NamespacedName, &ke); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Failed to get KubernetesExecution")
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Add CRD attributes to span
    span.SetAttributes(
        attribute.String("action.name", ke.Spec.Action.Name),
        attribute.String("action.targetResource", ke.Spec.Action.TargetResource),
        attribute.Bool("dryRun", ke.Spec.DryRun),
        attribute.String("phase", ke.Status.Phase),
    )

    // Execute reconciliation with tracing
    result, err := r.reconcilePhase(ctx, &ke)

    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Reconciliation failed")
    } else {
        span.SetStatus(codes.Ok, "Reconciliation completed")
    }

    return result, err
}

func (r *KubernetesExecutionReconciler) prepareExecution(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
) error {
    ctx, span := tracer.Start(ctx, "KubernetesExecution.PrepareExecution",
        trace.WithAttributes(
            attribute.String("action.name", ke.Spec.Action.Name),
        ),
    )
    defer span.End()

    // Create ServiceAccount
    sa, err := r.createServiceAccountForActionWithTracing(ctx, ke)
    if err != nil {
        span.RecordError(err)
        return err
    }
    span.SetAttributes(
        attribute.String("serviceAccount.name", sa.Name),
    )

    // Create RBAC
    role, binding, err := r.createRBACForActionWithTracing(ctx, ke)
    if err != nil {
        span.RecordError(err)
        return err
    }
    span.SetAttributes(
        attribute.String("role.name", role.Name),
        attribute.String("roleBinding.name", binding.Name),
        attribute.Int("role.permissions", len(role.Rules)),
    )

    return nil
}

func (r *KubernetesExecutionReconciler) executeAction(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
) error {
    ctx, span := tracer.Start(ctx, "KubernetesExecution.ExecuteAction",
        trace.WithAttributes(
            attribute.String("action.name", ke.Spec.Action.Name),
            attribute.String("action.targetResource", ke.Spec.Action.TargetResource),
        ),
    )
    defer span.End()

    // Build kubectl command
    command := r.buildKubectlCommand(ke.Spec.Action, ke.Spec.DryRun)

    // 🚨 CRITICAL: Sanitize command before adding to trace
    sanitizedCommand := sanitizeCommand(command)
    span.SetAttributes(
        attribute.String("kubectl.command", sanitizedCommand),  // Sanitized version only
    )

    // Create and execute Job
    job, err := r.createExecutionJobWithTracing(ctx, ke, command)
    if err != nil {
        span.RecordError(err)
        return err
    }

    span.SetAttributes(
        attribute.String("job.name", job.Name),
    )

    // Watch Job completion
    if err := r.watchJobCompletionWithTracing(ctx, ke, job); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Job execution failed")
        return err
    }

    span.SetStatus(codes.Ok, "Action executed successfully")
    return nil
}

func (r *KubernetesExecutionReconciler) watchJobCompletionWithTracing(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
    job *batchv1.Job,
) error {
    ctx, span := tracer.Start(ctx, "KubernetesExecution.WatchJobCompletion",
        trace.WithAttributes(
            attribute.String("job.name", job.Name),
        ),
    )
    defer span.End()

    // Watch Job status
    if job.Status.Succeeded > 0 {
        span.SetAttributes(
            attribute.Int("job.succeeded", int(job.Status.Succeeded)),
        )
        span.SetStatus(codes.Ok, "Job completed successfully")

        // Capture Job logs (sanitized)
        logs, err := r.captureJobLogs(ctx, ke, job)
        if err == nil {
            sanitizedLogs := sanitizeCommand(logs)
            span.SetAttributes(
                attribute.Int("job.logSize", len(sanitizedLogs)),
            )
        }
    } else if job.Status.Failed > 0 {
        span.SetAttributes(
            attribute.Int("job.failed", int(job.Status.Failed)),
        )
        span.SetStatus(codes.Error, "Job failed")

        // Capture Job logs (sanitized)
        logs, err := r.captureJobLogs(ctx, ke, job)
        if err == nil {
            sanitizedLogs := sanitizeCommand(logs)
            span.SetAttributes(
                attribute.String("job.errorLogs", sanitizedLogs[:min(len(sanitizedLogs), 500)]),  // First 500 chars
            )
        }

        return fmt.Errorf("job execution failed")
    }

    return nil
}
```

**Trace Visualization** (Jaeger):
```
Trace ID: abc-123-def-456
Span: KubernetesExecution.Reconcile (1.5s)
  ├─ Span: KubernetesExecution.PrepareExecution (50ms)
  │   ├─ Span: KubernetesExecution.CreateServiceAccount (15ms)
  │   └─ Span: KubernetesExecution.CreateRBAC (35ms)
  │       ├─ Span: KubernetesAPI.CreateRole (18ms)
  │       └─ Span: KubernetesAPI.CreateRoleBinding (17ms)
  ├─ Span: KubernetesExecution.ExecuteAction (1.3s)
  │   ├─ Span: KubernetesExecution.CreateJob (25ms)
  │   └─ Span: KubernetesExecution.WatchJobCompletion (1.2s)
  │       ├─ Span: Job.Execution (kubectl delete pod) (1.0s)
  │       └─ Span: KubernetesExecution.CaptureJobLogs (200ms)
  └─ Span: KubernetesExecution.UpdateStatus (150ms)
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

// Extract correlation ID from incoming context (from WorkflowExecution)
func extractCorrelationID(ctx context.Context) string {
    if id, ok := ctx.Value(correlationIDKey{}).(string); ok {
        return id
    }
    // Generate new ID if not present
    return uuid.New().String()
}

// Add correlation ID to Job labels
func (r *KubernetesExecutionReconciler) createExecutionJob(
    ctx context.Context,
    ke *kubernetesexecutionv1.KubernetesExecution,
    command string,
) (*batchv1.Job, error) {
    correlationID := extractCorrelationID(ctx)

    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-job", ke.Name),
            Namespace: "kubernaut-system",
            Labels: map[string]string{
                "app":           "kubernetesexecution-job",
                "correlationID": correlationID,  // Propagate to Job
                "action":        ke.Spec.Action.Name,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(ke, kubernetesexecutionv1.GroupVersion.WithKind("KubernetesExecution")),
            },
        },
        Spec: batchv1.JobSpec{
            Template: corev1.PodTemplateSpec{
                Metadata: metav1.ObjectMeta{
                    Labels: map[string]string{
                        "app":           "kubernetesexecution-job",
                        "correlationID": correlationID,  // Propagate to Pod
                    },
                },
                Spec: corev1.PodSpec{
                    ServiceAccountName: r.getServiceAccountForAction(ke.Spec.Action.Name),
                    RestartPolicy:      corev1.RestartPolicyNever,
                    Containers: []corev1.Container{
                        {
                            Name:    "executor",
                            Image:   "bitnami/kubectl:latest",
                            Command: []string{"/bin/sh", "-c"},
                            Args:    []string{command},
                            Env: []corev1.EnvVar{
                                {
                                    Name:  "CORRELATION_ID",
                                    Value: correlationID,  // Pass to Job pod as env var
                                },
                            },
                        },
                    },
                },
            },
            BackoffLimit:            ptr.To(int32(0)),
            TTLSecondsAfterFinished: ptr.To(int32(300)),
        },
    }

    if err := r.Create(ctx, job); err != nil {
        return nil, err
    }

    return job, nil
}
```

**Correlation Flow**:
```
RemediationRequest (correlationID: abc-123)
    ↓
WorkflowExecution (correlationID: abc-123)
    ↓
KubernetesExecution (correlationID: abc-123)
    ↓ (creates Kubernetes Job with correlationID in label + env var)
Kubernetes Job (correlationID: abc-123)
    ↓ (Job pod logs with correlationID via env var)
kubectl execution (correlationID: abc-123)
```

**Query Logs by Correlation ID**:
```bash
# Query controller logs
kubectl logs -n kubernaut-system deployment/kubernetesexecution-controller | grep "correlationID: abc-123"

# Query Job pod logs
kubectl logs -l correlationID=abc-123 -n kubernaut-system
```

---

### Debug Configuration

**Enable Debug Logging**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernetesexecution-controller-config
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
kubectl set env deployment/kubernetesexecution-controller -n kubernaut-system LOG_LEVEL=debug

# View debug logs for specific KubernetesExecution
kubectl logs -n kubernaut-system deployment/kubernetesexecution-controller --tail=1000 | grep "ke-xyz"

# View kubectl command construction (V(1) logs)
kubectl logs -n kubernaut-system deployment/kubernetesexecution-controller --tail=1000 | grep "kubectl command prepared"

# View RBAC creation details (V(1) logs)
kubectl logs -n kubernaut-system deployment/kubernetesexecution-controller --tail=1000 | grep "RBAC created"

# View Job execution logs
kubectl logs -n kubernaut-system deployment/kubernetesexecution-controller --tail=1000 | grep "Job completed"

# View Job pod logs directly
kubectl get jobs -n kubernaut-system -l app=kubernetesexecution-job
kubectl logs job/<job-name> -n kubernaut-system
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
# deploy/kubernetes-executor-servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: kubernetes-executor-metrics
  namespace: kubernaut-system
  labels:
    app: kubernetes-executor
    prometheus: kubernaut
spec:
  selector:
    matchLabels:
      app: kubernetes-executor
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
# deploy/kubernetes-executor-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kubernetes-executor
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kubernetes-executor
  template:
    metadata:
      labels:
        app: kubernetes-executor
    spec:
      serviceAccountName: kubernetes-executor-sa
      containers:
      - name: controller
        image: kubernaut/kubernetes-executor:latest
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
# deploy/kubernetes-executor-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: kubernetes-executor-metrics
  namespace: kubernaut-system
  labels:
    app: kubernetes-executor
spec:
  selector:
    app: kubernetes-executor
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
  type: ClusterIP
```

### Implementation Code

```go
// cmd/kubernetesexecutor/main.go
package main

import (
    "flag"
    "os"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    "sigs.k8s.io/controller-runtime/pkg/metrics/server"
    "sigs.k8s.io/controller-runtime/pkg/healthz"

    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/pkg/apis/kubernetesexecution/v1"
    "github.com/jordigilh/kubernaut/pkg/kubernetesexecutor"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    _ = kubernetesexecutionv1.AddToScheme(scheme)
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
        LeaderElectionID:       "kubernetes-executor.kubernaut.io",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    if err = (&kubernetesexecutor.KubernetesExecutionReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
        Log:    ctrl.Log.WithName("controllers").WithName("KubernetesExecution"),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "KubernetesExecution")
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
