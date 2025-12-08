/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main is the entry point for the AIAnalysis controller.
// This controller orchestrates AI-based incident analysis using HolmesGPT-API.
//
// Business Requirements: BR-AI-001 to BR-AI-083 (V1.0)
// Architecture: DD-CONTRACT-002, DD-RECOVERY-002, DD-AIANALYSIS-001
package main

import (
	"flag"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/client"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/rego"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// Build information (set by ldflags) per DD-014
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(aianalysisv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var holmesGPTURL string
	var holmesGPTTimeout time.Duration
	var regoPolicyPath string
	var dataStorageURL string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.StringVar(&holmesGPTURL, "holmesgpt-api-url", getEnvOrDefault("HOLMESGPT_API_URL", "http://holmesgpt-api:8080"), "HolmesGPT-API base URL.")
	flag.DurationVar(&holmesGPTTimeout, "holmesgpt-api-timeout", 60*time.Second, "HolmesGPT-API request timeout.")
	flag.StringVar(&regoPolicyPath, "rego-policy-path", getEnvOrDefault("REGO_POLICY_PATH", "/etc/kubernaut/policies/approval.rego"), "Path to Rego approval policy file.")
	flag.StringVar(&dataStorageURL, "datastorage-url", getEnvOrDefault("DATASTORAGE_URL", "http://datastorage:8080"), "Data Storage Service URL for audit events.")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// DD-014: Log version information at startup
	setupLog.Info("Starting AI Analysis Controller",
		"version", Version,
		"gitCommit", GitCommit,
		"buildTime", BuildTime,
	)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "aianalysis.kubernaut.ai",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// ========================================
	// BR-AI-007: Wire HolmesGPT-API client for Investigating phase
	// ========================================
	setupLog.Info("Creating HolmesGPT-API client", "url", holmesGPTURL, "timeout", holmesGPTTimeout)
	holmesGPTClient := client.NewHolmesGPTClient(client.Config{
		BaseURL: holmesGPTURL,
		Timeout: holmesGPTTimeout,
	})

	// ========================================
	// BR-AI-012: Wire Rego evaluator for Analyzing phase
	// DD-AIANALYSIS-001: Rego policy loading
	// ========================================
	setupLog.Info("Creating Rego evaluator", "policyPath", regoPolicyPath)
	regoEvaluator := rego.NewEvaluator(rego.Config{
		PolicyPath: regoPolicyPath,
	})

	// ========================================
	// DD-AUDIT-003: Wire audit client for P0 audit traces
	// ========================================
	setupLog.Info("Creating audit client", "dataStorageURL", dataStorageURL)
	httpClient := &http.Client{Timeout: 5 * time.Second}
	dsClient := sharedaudit.NewHTTPDataStorageClient(dataStorageURL, httpClient)
	auditStore, err := sharedaudit.NewBufferedStore(
		dsClient,
		sharedaudit.DefaultConfig(),
		"aianalysis",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		setupLog.Error(err, "failed to create audit store, audit will be disabled")
		// Continue without audit - graceful degradation per DD-AUDIT-002
	}

	// Create service-specific audit client
	var auditClient *audit.AuditClient
	if auditStore != nil {
		auditClient = audit.NewAuditClient(auditStore, ctrl.Log.WithName("audit"))
	}

	// ========================================
	// Create phase handlers (BR-AI-007, BR-AI-012)
	// ========================================
	controllerLog := ctrl.Log.WithName("controllers").WithName("AIAnalysis")
	investigatingHandler := handlers.NewInvestigatingHandler(holmesGPTClient, controllerLog)
	analyzingHandler := handlers.NewAnalyzingHandler(regoEvaluator, controllerLog)

	if err = (&aianalysis.AIAnalysisReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		Recorder:             mgr.GetEventRecorderFor("aianalysis-controller"),
		Log:                  controllerLog,
		InvestigatingHandler: investigatingHandler, // BR-AI-007: HolmesGPT integration
		AnalyzingHandler:     analyzingHandler,     // BR-AI-012: Rego policy evaluation
		AuditClient:          auditClient,          // DD-AUDIT-003: P0 audit traces
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

// getEnvOrDefault returns the value of environment variable or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

