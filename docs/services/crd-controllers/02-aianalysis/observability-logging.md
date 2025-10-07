## Observability & Logging

**Note**: For complete structured logging, distributed tracing, log correlation, and debug configuration patterns, refer to the Security & Observability sections in 01-alert-processor.md. AI Analysis follows the same patterns with AI-specific adaptations for HolmesGPT integration and approval workflow tracking.

**AI Analysis Specific Logging Examples**:

```go
// HolmesGPT investigation logging (sanitized)
log.V(1).Info("HolmesGPT investigation completed",
    "duration", time.Since(start),
    "confidenceScore", investigation.ConfidenceScore,
    "recommendationCount", len(investigation.Recommendations),
    // NEVER log full response (may contain secrets)
)

// Approval policy evaluation logging
log.Info("Approval policy evaluated",
    "autoApproved", approved,
    "actionsCount", len(ai.Spec.RecommendedActions),
    // NEVER log policy content
)
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
# deploy/ai-analysis-servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: ai-analysis-metrics
  namespace: kubernaut-system
  labels:
    app: ai-analysis
    prometheus: kubernaut
spec:
  selector:
    matchLabels:
      app: ai-analysis
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
# deploy/ai-analysis-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ai-analysis
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ai-analysis
  template:
    metadata:
      labels:
        app: ai-analysis
    spec:
      serviceAccountName: ai-analysis-sa
      containers:
      - name: controller
        image: kubernaut/ai-analysis:latest
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
# deploy/ai-analysis-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: ai-analysis-metrics
  namespace: kubernaut-system
  labels:
    app: ai-analysis
spec:
  selector:
    app: ai-analysis
  ports:
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
  type: ClusterIP
```

### Implementation Code

```go
// cmd/ai-analysis/main.go
package main

import (
    "flag"
    "os"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/log/zap"
    "sigs.k8s.io/controller-runtime/pkg/metrics/server"
    "sigs.k8s.io/controller-runtime/pkg/healthz"

    aianalysisv1 "github.com/jordigilh/kubernaut/pkg/apis/aianalysis/v1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis"
)

var (
    scheme   = runtime.NewScheme()
    setupLog = ctrl.Log.WithName("setup")
)

func init() {
    _ = aianalysisv1.AddToScheme(scheme)
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
        LeaderElectionID:       "ai-analysis.kubernaut.io",
    })
    if err != nil {
        setupLog.Error(err, "unable to start manager")
        os.Exit(1)
    }

    if err = (&aianalysis.AIAnalysisReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
        Log:    ctrl.Log.WithName("controllers").WithName("AIAnalysis"),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "AIAnalysis")
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

