## Observability & Logging

### Structured Logging Patterns

**Log Levels**:

| Level | Purpose | Examples |
|-------|---------|----------|
| **ERROR** | Unrecoverable errors, requires intervention | CRD validation failure, API server unreachable |
| **WARN** | Recoverable errors, degraded mode | Context Service timeout (fallback to basic enrichment) |
| **INFO** | Normal operations, state transitions | Phase transitions, enrichment completion |
| **DEBUG** | Detailed flow for troubleshooting | Kubernetes API queries, enrichment logic details |

**Structured Logging Implementation**:

```go
package controller

import (
    "context"
    "time"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    "github.com/go-logr/logr"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type RemediationProcessingReconciler struct {
    client.Client
    Scheme *runtime.Scheme
    Log    logr.Logger
}

func (r *RemediationProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Create request-scoped logger with correlation ID
    log := r.Log.WithValues(
        "alertprocessing", req.NamespacedName,
        "correlationID", extractCorrelationID(ctx),
    )

    var ap alertprocessorv1.RemediationProcessing
    if err := r.Get(ctx, req.NamespacedName, &ap); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Log phase transitions
    oldPhase := ap.Status.Phase
    log.Info("Reconciling RemediationProcessing",
        "phase", ap.Status.Phase,
        "fingerprint", ap.Spec.Alert.Fingerprint,
    )

    // Execute reconciliation logic with structured logging
    result, err := r.reconcilePhase(ctx, &ap, log)

    // Log phase change
    if ap.Status.Phase != oldPhase {
        log.Info("Phase transition",
            "from", oldPhase,
            "to", ap.Status.Phase,
            "duration", time.Since(ap.Status.StartTime.Time),
        )
    }

    if err != nil {
        log.Error(err, "Reconciliation failed",
            "phase", ap.Status.Phase,
            "retryCount", ap.Status.RetryCount,
        )
        return result, err
    }

    return result, nil
}

func (r *RemediationProcessingReconciler) enrichAlert(
    ctx context.Context,
    ap *alertprocessorv1.RemediationProcessing,
    log logr.Logger,
) error {
    log.V(1).Info("Starting alert enrichment",
        "fingerprint", ap.Spec.Alert.Fingerprint,
        "namespace", ap.Spec.Alert.Annotations["namespace"],
    )

    // Kubernetes context enrichment
    start := time.Now()
    kubeContext, err := r.enrichKubernetesContext(ctx, ap)
    if err != nil {
        log.Error(err, "Kubernetes enrichment failed (fallback to basic)",
            "namespace", ap.Spec.Alert.Annotations["namespace"],
        )
        // Continue with degraded mode
        ap.Status.DegradedMode = true
    } else {
        log.V(1).Info("Kubernetes enrichment completed",
            "duration", time.Since(start),
            "resourceKind", kubeContext.ResourceKind,
        )
    }

    // Context Service enrichment
    start = time.Now()
    historicalContext, err := r.contextClient.GetHistoricalContext(ctx, ap.Spec.Alert.Fingerprint)
    if err != nil {
        log.Warn("Context Service enrichment failed (using defaults)",
            "error", err,
            "fingerprint", ap.Spec.Alert.Fingerprint,
        )
        // Continue without historical context
    } else {
        log.V(1).Info("Context Service enrichment completed",
            "duration", time.Since(start),
            "historicalOccurrences", historicalContext.OccurrenceCount,
        )
    }

    log.Info("Alert enrichment completed",
        "degradedMode", ap.Status.DegradedMode,
        "totalDuration", time.Since(ap.Status.StartTime.Time),
    )

    return nil
}

// Debug logging for troubleshooting
func (r *RemediationProcessingReconciler) debugLogKubernetesQuery(
    log logr.Logger,
    query string,
    result interface{},
    duration time.Duration,
) {
    log.V(2).Info("Kubernetes API query",
        "query", query,
        "duration", duration,
        "resultCount", resultCount(result),
    )
}
```

**Log Correlation Example**:
```
INFO    Reconciling RemediationProcessing    {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "phase": "enriching", "fingerprint": "abc123"}
INFO    Starting alert enrichment      {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "fingerprint": "abc123", "namespace": "production"}
DEBUG   Kubernetes API query           {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "query": "get pod production/web-app-789", "duration": "15ms"}
INFO    Alert enrichment completed     {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "degradedMode": false, "totalDuration": "234ms"}
INFO    Phase transition               {"alertprocessing": "default/alert-processing-xyz", "correlationID": "abc-123-def", "from": "enriching", "to": "classifying", "duration": "234ms"}
```

---

### Distributed Tracing

**OpenTelemetry Integration**:

```go
package controller

import (
    "context"

    alertprocessorv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/codes"
    "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("alertprocessing-controller")

func (r *RemediationProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    ctx, span := tracer.Start(ctx, "RemediationProcessing.Reconcile",
        trace.WithAttributes(
            attribute.String("alertprocessing.name", req.Name),
            attribute.String("alertprocessing.namespace", req.Namespace),
        ),
    )
    defer span.End()

    var ap alertprocessorv1.RemediationProcessing
    if err := r.Get(ctx, req.NamespacedName, &ap); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Failed to get RemediationProcessing")
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Add CRD attributes to span
    span.SetAttributes(
        attribute.String("alert.fingerprint", ap.Spec.Alert.Fingerprint),
        attribute.String("alert.severity", ap.Spec.Alert.Severity),
        attribute.String("phase", ap.Status.Phase),
    )

    // Execute reconciliation with tracing
    result, err := r.reconcilePhase(ctx, &ap)

    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, "Reconciliation failed")
    } else {
        span.SetStatus(codes.Ok, "Reconciliation completed")
    }

    return result, err
}

func (r *RemediationProcessingReconciler) enrichAlert(
    ctx context.Context,
    ap *alertprocessorv1.RemediationProcessing,
) error {
    ctx, span := tracer.Start(ctx, "RemediationProcessing.EnrichAlert")
    defer span.End()

    // Kubernetes enrichment span
    kubeContext, err := r.enrichKubernetesContextWithTracing(ctx, ap)
    if err != nil {
        span.RecordError(err)
        // Continue in degraded mode
    }

    // Context Service enrichment span
    historicalContext, err := r.enrichHistoricalContextWithTracing(ctx, ap)
    if err != nil {
        span.RecordError(err)
        // Continue without historical context
    }

    span.SetAttributes(
        attribute.Bool("degraded_mode", ap.Status.DegradedMode),
        attribute.Int("enrichment_steps", 2),
    )

    return nil
}

func (r *RemediationProcessingReconciler) enrichKubernetesContextWithTracing(
    ctx context.Context,
    ap *alertprocessorv1.RemediationProcessing,
) (*alertprocessorv1.KubernetesContext, error) {
    ctx, span := tracer.Start(ctx, "RemediationProcessing.EnrichKubernetesContext",
        trace.WithAttributes(
            attribute.String("namespace", ap.Spec.Alert.Annotations["namespace"]),
            attribute.String("resourceKind", ap.Spec.Alert.Annotations["kind"]),
        ),
    )
    defer span.End()

    // Get Pod details
    pod, err := r.getPodDetails(ctx, ap)
    if err != nil {
        span.RecordError(err)
        return nil, err
    }

    // 🚨 CRITICAL: Sanitize pod annotations before adding to trace
    sanitizedAnnotations := sanitizeMapValues(pod.Annotations)

    span.SetAttributes(
        attribute.String("pod.name", pod.Name),
        attribute.String("pod.status", string(pod.Status.Phase)),
        attribute.Int("pod.restartCount", int(pod.Status.ContainerStatuses[0].RestartCount)),
        // Only include sanitized annotations (secrets scrambled)
        attribute.String("pod.annotations", fmt.Sprintf("%v", sanitizedAnnotations)),
    )

    return &alertprocessorv1.KubernetesContext{
        ResourceKind: "Pod",
        ResourceName: pod.Name,
        // ... other fields
    }, nil
}

// Sanitize map values to prevent secret leakage in traces
func sanitizeMapValues(m map[string]string) map[string]string {
    sanitized := make(map[string]string)
    for k, v := range m {
        sanitized[k] = sanitizeAlertPayload(v)
    }
    return sanitized
}
```

**Trace Visualization** (Jaeger):
```
Trace ID: abc-123-def-456
Span: RemediationProcessing.Reconcile (234ms)
  ├─ Span: RemediationProcessing.EnrichAlert (180ms)
  │   ├─ Span: RemediationProcessing.EnrichKubernetesContext (120ms)
  │   │   ├─ Span: KubernetesAPI.GetPod (50ms)
  │   │   └─ Span: KubernetesAPI.GetDeployment (40ms)
  │   └─ Span: ContextService.GetHistoricalContext (60ms)
  │       └─ Span: HTTP.POST /context (55ms)
  └─ Span: RemediationProcessing.ClassifyEnvironment (54ms)
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

// Add correlation ID to outgoing requests
func (r *RemediationProcessingReconciler) callContextService(
    ctx context.Context,
    fingerprint string,
) (*ContextResponse, error) {
    correlationID := extractCorrelationID(ctx)

    req, err := http.NewRequestWithContext(ctx, "POST", r.contextServiceURL, body)
    if err != nil {
        return nil, err
    }

    // Propagate correlation ID via header
    req.Header.Set("X-Correlation-ID", correlationID)
    req.Header.Set("Content-Type", "application/json")

    resp, err := r.httpClient.Do(req)
    // ... handle response
}
```

**Correlation Flow**:
```
RemediationRequest (correlationID: abc-123)
    ↓ (creates RemediationProcessing with correlationID in annotation)
RemediationProcessing Controller (correlationID: abc-123)
    ↓ (HTTP header: X-Correlation-ID: abc-123)
Context Service (correlationID: abc-123)
    ↓ (logs with correlationID: abc-123)
```

**Query Logs by Correlation ID**:
```bash
kubectl logs -n kubernaut-system deployment/alertprocessing-controller | grep "correlationID: abc-123"
```

---

### Debug Configuration

**Enable Debug Logging**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: alertprocessing-controller-config
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
kubectl set env deployment/alertprocessing-controller -n kubernaut-system LOG_LEVEL=debug

# View debug logs for specific RemediationProcessing
kubectl logs -n kubernaut-system deployment/alertprocessing-controller --tail=1000 | grep "alert-processing-xyz"

# View Kubernetes API queries (V(2) logs)
kubectl logs -n kubernaut-system deployment/alertprocessing-controller --tail=1000 | grep "Kubernetes API query"

# View Context Service calls
kubectl logs -n kubernaut-system deployment/alertprocessing-controller --tail=1000 | grep "Context Service"
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
  name: remediation-processor
  namespace: kubernaut-system
spec:
  selector:
    matchLabels:
      app: remediation-processor
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
# deploy/remediation-processor-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: remediation-processor
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: remediation-processor
  template:
    metadata:
      labels:
        app: remediation-processor
    spec:
      serviceAccountName: remediation-processor-sa
      containers:
      - name: controller
        image: kubernaut/remediation-processor:latest
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
# deploy/remediation-processor-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: remediation-processor
  namespace: kubernaut-system
  labels:
    app: remediation-processor
spec:
  selector:
    app: remediation-processor
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
```

### Implementation Code

```go
// cmd/remediationprocessor/main.go
package main

import (
    "flag"
    "os"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    "sigs.k8s.io/controller-runtime/pkg/metrics/server"
    "sigs.k8s.io/controller-runtime/pkg/healthz"

    remediationprocessingv1 "github.com/jordigilh/kubernaut/pkg/apis/remediationprocessing/v1"
    "github.com/jordigilh/kubernaut/pkg/remediationprocessor"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    _ = remediationprocessingv1.AddToScheme(scheme)
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
        LeaderElectionID:       "remediation-processor.kubernaut.io",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    if err = (&remediationprocessor.RemediationProcessingReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
        Log:    ctrl.Log.WithName("controllers").WithName("RemediationProcessing"),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "RemediationProcessing")
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

