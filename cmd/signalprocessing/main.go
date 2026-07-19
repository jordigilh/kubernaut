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

// ========================================
// SIGNAL PROCESSING CONTROLLER (DD-006)
// ========================================
//
// Design Decisions:
//   - DD-006: Controller Scaffolding Strategy
//   - DD-005: Observability Standards (Metrics & Logging)
//   - DD-CRD-001: API Group Domain (.kubernaut.ai)
//   - DD-014: Binary Version Logging
//
// Business Requirements:
//   - BR-SP-001: K8s Context Enrichment
//   - BR-SP-070: Priority Assignment (Rego)
//   - BR-SP-090: Categorization Audit Trail
//
// ========================================
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	zaplog "go.uber.org/zap"
	"gopkg.in/yaml.v3"

	// Standard Kubernetes imports
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// Kubernaut API imports
	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	"github.com/jordigilh/kubernaut/internal/version"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/audit"
	fleetclient "github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	fleetregistry "github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	scope "github.com/jordigilh/kubernaut/pkg/shared/scope"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/classifier"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/config"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(signalprocessingv1alpha1.AddToScheme(scheme))
	utilruntime.Must(remediationv1alpha1.AddToScheme(scheme))
}

// loadSignalProcessingConfig loads the YAML config (ADR-030), applies the
// config-driven log level (Issue #875), validates it, and discovers the
// controller namespace for CRD watch restriction (ADR-057). Exits the
// process on any failure, matching main()'s original fail-fast behavior.
func loadSignalProcessingConfig(configFile string, atomicLevel zaplog.AtomicLevel) (*config.Config, string) {
	setupLog.Info("Starting SignalProcessing Controller",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
	)

	cfg, err := config.LoadFromFile(configFile)
	if err != nil {
		setupLog.Error(err, "Failed to load configuration -- aborting startup",
			"configPath", configFile)
		os.Exit(1)
	}
	setupLog.Info("Configuration loaded successfully", "configPath", configFile)

	atomicLevel.SetLevel(cfg.Logging.ZapLevel())
	setupLog.Info("Log level configured from config file", "level", cfg.Logging.Level)

	if err := cfg.Validate(); err != nil {
		setupLog.Error(err, "invalid configuration")
		os.Exit(1)
	}

	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		setupLog.Error(err, "unable to determine controller namespace")
		os.Exit(1)
	}

	setupLog.Info("SignalProcessing controller configuration",
		"metricsAddr", cfg.Controller.MetricsAddr,
		"healthProbeAddr", cfg.Controller.HealthProbeAddr,
		"enrichmentTimeout", cfg.Enrichment.Timeout,
		"dataStorageURL", cfg.DataStorage.URL,
	)

	return cfg, controllerNS
}

// buildSignalProcessingManager creates the controller manager with the
// namespace-restricted SignalProcessing cache and metrics/health-probe/
// leader election settings from cfg (ADR-030). Exits the process on any
// failure, matching main()'s original fail-fast behavior.
func buildSignalProcessingManager(cfg *config.Config, controllerNS string) ctrl.Manager {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&signalprocessingv1alpha1.SignalProcessing{}: {
					Namespaces: map[string]cache.Config{
						controllerNS: {},
					},
				},
			},
		},
		Metrics: metricsserver.Options{
			BindAddress: cfg.Controller.MetricsAddr,
		},
		HealthProbeBindAddress: cfg.Controller.HealthProbeAddr,
		LeaderElection:         cfg.Controller.LeaderElection,
		LeaderElectionID:       cfg.Controller.LeaderElectionID,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	return mgr
}

// wireSignalProcessingAudit creates the mandatory buffered audit store
// (ADR-032, ADR-038, ADR-030) and the service-specific audit client
// (BR-SP-090). Exits the process on any failure, matching main()'s original
// fail-fast behavior, since audit is mandatory per ADR-032.
func wireSignalProcessingAudit(cfg *config.Config) (sharedaudit.AuditStore, *spaudit.AuditClient) {
	setupLog.Info("configuring audit client", "dataStorageURL", cfg.DataStorage.URL)

	dsClient, err := sharedaudit.NewOpenAPIClientAdapter(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		setupLog.Error(err, "FATAL: failed to create Data Storage client")
		os.Exit(1)
	}

	auditConfig := sharedaudit.Config{
		BufferSize:    cfg.DataStorage.Buffer.BufferSize,
		BatchSize:     cfg.DataStorage.Buffer.BatchSize,
		FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
		MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
	}

	auditStore, err := sharedaudit.NewBufferedStore(
		dsClient,
		auditConfig,
		"signalprocessing",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032")
		os.Exit(1)
	}

	setupLog.Info("Audit store initialized",
		"dataStorageURL", cfg.DataStorage.URL,
		"bufferSize", auditConfig.BufferSize,
		"batchSize", auditConfig.BatchSize,
		"flushInterval", auditConfig.FlushInterval,
	)

	auditClient := spaudit.NewAuditClient(auditStore, ctrl.Log.WithName("audit"))
	setupLog.Info("audit client configured successfully")

	return auditStore, auditClient
}

// wireSignalProcessingPolicyEvaluator starts the unified Rego evaluator
// (ADR-060), which replaces five separate classifiers with a single
// evaluator backed by one policy.rego file mounted from the ConfigMap
// (Issue #419). Exits the process on failure since the unified policy is
// mandatory, matching main()'s original fail-fast behavior.
func wireSignalProcessingPolicyEvaluator(ctx context.Context, cfg *config.Config) *evaluator.Evaluator {
	policyPath := filepath.Join("/etc/signalprocessing/policies", cfg.Classifier.RegoConfigMapKey)
	policyEvaluator := evaluator.New(
		policyPath,
		ctrl.Log.WithName("evaluator"),
	)
	if err := policyEvaluator.StartHotReload(ctx); err != nil {
		setupLog.Error(err, "FATAL: unified policy is mandatory but failed to load",
			"policyPath", policyPath,
			"configMapName", cfg.Classifier.RegoConfigMapName,
			"configMapKey", cfg.Classifier.RegoConfigMapKey,
			"hint", "Ensure the policy ConfigMap is mounted at /etc/signalprocessing/policies/")
		os.Exit(1)
	}
	setupLog.Info("Unified policy evaluator started",
		"policyPath", policyPath,
		"policyHash", policyEvaluator.GetPolicyHash())

	return policyEvaluator
}

// buildSignalModeClassifier loads the optional proactive signal mode
// mapping config (BR-SP-106, ADR-054). Missing config is non-fatal: all
// signals default to reactive mode.
func buildSignalModeClassifier() *classifier.SignalModeClassifier {
	signalModeClassifier := classifier.NewSignalModeClassifier(
		ctrl.Log.WithName("classifier.signalmode"),
	)

	signalModeConfigPath := "/etc/signalprocessing/proactive-signal-mappings.yaml"
	if envPath := os.Getenv("SIGNAL_MODE_CONFIG_PATH"); envPath != "" {
		signalModeConfigPath = envPath
	}
	if err := signalModeClassifier.LoadConfig(signalModeConfigPath); err != nil {
		setupLog.Info("signal mode config not found, all signals will default to reactive mode",
			"configPath", signalModeConfigPath,
			"error", err.Error())
	} else {
		setupLog.Info("signal mode classifier configured successfully",
			"configPath", signalModeConfigPath)
	}

	return signalModeClassifier
}

// buildFleetOAuth2Options constructs the hot-reloadable OAuth2 transport
// option for the Fleet MCP client when cfg.Fleet.OAuth2.Enabled, deriving
// the credentials mount path from CredentialsSecretRef when set.
func buildFleetOAuth2Options(cfg *config.Config) []fleetclient.Option {
	if !cfg.Fleet.OAuth2.Enabled {
		return nil
	}

	basePath := "/etc/signalprocessing/fleet-oauth2"
	if cfg.Fleet.OAuth2.CredentialsSecretRef != "" {
		basePath = "/etc/signalprocessing/" + cfg.Fleet.OAuth2.CredentialsSecretRef
	}
	reloadCfg := fleetclient.ReloadableOAuth2Config{
		TokenURL:         cfg.Fleet.OAuth2.TokenURL,
		ClientIDPath:     basePath + "/client-id",
		ClientSecretPath: basePath + "/client-secret",
		Scopes:           fleetclient.DefaultFleetScopes(cfg.Fleet.OAuth2.Scopes),
		TokenTimeout:     10 * time.Second,
		TlsCaFile:        cfg.Fleet.OAuth2.TLSCAFile,
	}
	setupLog.Info("fleet OAuth2 authentication configured (hot-reloadable)",
		"tokenURL", cfg.Fleet.OAuth2.TokenURL,
		"secretPath", basePath)

	return []fleetclient.Option{
		fleetclient.WithReloadableOAuth2Transport(reloadCfg, ctrl.Log.WithName("fleet-oauth2")),
	}
}

// wireFleetMCPClient connects to the optional Fleet MCP Gateway
// (BR-INTEGRATION-054) when cfg.Fleet.Endpoint is configured, and wires its
// reader factory into k8sEnricher for remote cluster enrichment. Returns
// nil when fleet is not configured. On a connection failure, enrichment
// falls back to local-cluster-only mode (non-fatal), but #1553: the
// (disconnected) client is still returned rather than discarded, so
// wireFleetReadinessGate can attach an MCPClientProber that keeps
// retrying via the periodic probe — this is what allows /readyz to
// recover once Fleet comes back, instead of requiring a pod restart
// (mirrors GW/RO/EM's identical Wave 2/3 change). localClient is
// pre-built by the caller (independently testable with a fake).
func wireFleetMCPClient(ctx context.Context, cfg *config.Config, localClient client.Client, k8sEnricher *enricher.K8sEnricher) *fleetclient.ResilientClient {
	if cfg.Fleet.Endpoint == "" {
		return nil
	}

	setupLog.Info("Fleet MCP Gateway configured, connecting...",
		"endpoint", cfg.Fleet.Endpoint,
		"oauth2Enabled", cfg.Fleet.OAuth2.Enabled)

	fleetOpts := buildFleetOAuth2Options(cfg) //nolint:contextcheck // OAuth2 token source refresh runs as a background reload, independent of any single request
	resilienceCfg := fleetclient.DefaultResilienceConfig()
	fleetResilientClient, fleetErr := fleetclient.NewResilient(
		ctx, cfg.Fleet.Endpoint, resilienceCfg,
		ctrl.Log.WithName("fleet-client"), fleetOpts...,
	)
	if fleetErr != nil {
		setupLog.Error(fleetErr, "Fleet MCP Gateway connection failed at startup; readiness will report "+
			"NotReady and keep retrying in the background; remote enrichment disabled until reconnect",
			"endpoint", cfg.Fleet.Endpoint)
		return fleetResilientClient
	}

	readerFactory := fleetclient.NewMCPReaderFactory(localClient, fleetResilientClient.Session())
	k8sEnricher.SetReaderFactory(readerFactory)
	setupLog.Info("Fleet MCP Gateway connected, remote enrichment enabled",
		"endpoint", cfg.Fleet.Endpoint)

	return fleetResilientClient
}

// wireClusterRegistry watches the MCP Gateway's cluster-registration CRDs
// (Backend/MCPServerRegistration) directly via a dynamic K8s client when
// cfg.Fleet.MCPGatewayType is configured (BR-FLEET-003 / #1511),
// independent of the MCP protocol client, wiring resolved
// ClusterInfo.Labels into k8sEnricher for the `cluster` Rego classification
// dimension. Returns nil when fleet cluster registry is not configured, no
// dynamic client is available, or construction fails outright (no object
// to retain). On a Start failure, #1553: the constructed registry is still
// returned (not discarded) so wireFleetReadinessGate can attach a
// ClusterRegistryProber — Ready() faithfully stays false until an operator
// restarts the pod (ClusterRegistry.Ready() is a one-shot flag with no
// internal retry, see pkg/fleet/readiness/probers.go's ClusterRegistry
// doc), which is still strictly better than silently not gating readiness
// on this dependency at all. Cluster classification enrichment itself
// remains disabled in this case (SetClusterRegistry is not called), same
// as before #1553. dynClient is pre-built by the caller (independently
// testable with a fake; nil is treated the same as a construction
// failure).
func wireClusterRegistry(ctx context.Context, cfg *config.Config, dynClient dynamic.Interface, k8sEnricher *enricher.K8sEnricher) fleetregistry.ClusterRegistry {
	if cfg.Fleet.MCPGatewayType == "" || dynClient == nil {
		return nil
	}

	clusterRegistry, crErr := fleetregistry.NewClusterRegistry(
		cfg.Fleet.MCPGatewayType,
		dynClient,
		fleetregistry.RegistryConfig{Namespace: cfg.Fleet.Namespace},
		fleetregistry.NewMetricsWithRegistry(ctrlmetrics.Registry),
		ctrl.Log.WithName("cluster-registry"),
	)
	if crErr != nil {
		setupLog.Error(crErr, "Failed to create cluster registry, cluster classification disabled",
			"gatewayType", cfg.Fleet.MCPGatewayType)
		return nil
	}

	if startErr := clusterRegistry.Start(ctx); startErr != nil {
		setupLog.Error(startErr, "Failed to start cluster registry at startup; cluster classification "+
			"disabled and readiness will report NotReady until a pod restart",
			"gatewayType", cfg.Fleet.MCPGatewayType)
		return clusterRegistry
	}

	k8sEnricher.SetClusterRegistry(clusterRegistry)
	setupLog.Info("Cluster registry started, cluster classification labels enabled",
		"gatewayType", cfg.Fleet.MCPGatewayType, "namespace", cfg.Fleet.Namespace)

	return clusterRegistry
}

// signalProcessingEnrichment bundles the metrics, K8s enricher, and
// optional Fleet MCP client / cluster registry wired for BR-SP-001
// enrichment, BR-INTEGRATION-054 remote enrichment, and BR-FLEET-003
// cluster classification.
type signalProcessingEnrichment struct {
	metrics         *spmetrics.Metrics
	k8sEnricher     *enricher.K8sEnricher
	fleetClient     *fleetclient.ResilientClient
	clusterRegistry fleetregistry.ClusterRegistry
}

// wireSignalProcessingEnrichment sets up the K8s context enricher
// (BR-SP-001) and its optional remote-cluster dependencies: the Fleet MCP
// Gateway client (BR-INTEGRATION-054) and the cluster registry
// (BR-FLEET-003 / #1511).
func wireSignalProcessingEnrichment(ctx context.Context, cfg *config.Config, mgr ctrl.Manager) *signalProcessingEnrichment {
	spMetrics := spmetrics.NewMetrics() // Uses ctrlmetrics.Registry (global)
	setupLog.Info("signalprocessing metrics configured")

	k8sEnricher := enricher.NewK8sEnricher(
		mgr.GetClient(),
		mgr.GetAPIReader(),
		ctrl.Log.WithName("enricher"),
		spMetrics,
		cfg.Enrichment.Timeout,
		cfg.Enrichment.CacheTTL,
	)
	setupLog.Info("k8s enricher configured",
		"enrichmentTimeout", cfg.Enrichment.Timeout,
		"cacheTTL", cfg.Enrichment.CacheTTL)

	fleetResilientClient := wireFleetMCPClient(ctx, cfg, mgr.GetClient(), k8sEnricher)

	var dynClient dynamic.Interface
	if dyn, dynErr := dynamic.NewForConfig(mgr.GetConfig()); dynErr != nil {
		setupLog.Error(dynErr, "K8s dynamic client unavailable, fleet cluster registry disabled")
	} else {
		dynClient = dyn
	}
	clusterRegistry := wireClusterRegistry(ctx, cfg, dynClient, k8sEnricher)

	return &signalProcessingEnrichment{
		metrics:         spMetrics,
		k8sEnricher:     k8sEnricher,
		fleetClient:     fleetResilientClient,
		clusterRegistry: clusterRegistry,
	}
}

// fleetReadinessProbeInterval controls how often the Fleet readiness gate
// re-probes its dependencies once started (mirrors cmd/gateway/main.go,
// cmd/remediationorchestrator/main.go, cmd/effectivenessmonitor/main.go).
const fleetReadinessProbeInterval = 15 * time.Second

// wireFleetReadinessGate builds and starts the Fleet dependency readiness
// gate (#1553, ADR-068, BR-INTEGRATION-054/BR-FLEET-003): once either the
// Fleet MCP Gateway client or the cluster registry is wired, SP's
// pod-wide readyz must fail closed when that dependency is unreachable,
// instead of the previous fail-open behavior of only logging an error.
// Unlike GW/RO/EM, SP has no single "Fleet enabled" flag — the MCP client
// and cluster registry are independent, optional features (confirmed via
// preflight: cfg.Fleet.Endpoint and cfg.Fleet.MCPGatewayType gate them
// separately) — so gating is simply "did wireFleetMCPClient /
// wireClusterRegistry actually produce an object". Returns nil when
// neither dependency is configured. The caller registers the returned
// Gate's Check method via mgr.AddReadyzCheck and must Stop() it on
// shutdown.
func wireFleetReadinessGate(
	ctx context.Context,
	fleetClient *fleetclient.ResilientClient,
	clusterRegistry fleetregistry.ClusterRegistry,
	logger logr.Logger,
) *readiness.Gate {
	var probers []readiness.Prober
	if fleetClient != nil {
		probers = append(probers, &readiness.MCPClientProber{Client: fleetClient})
	}
	if clusterRegistry != nil {
		probers = append(probers, &readiness.ClusterRegistryProber{Registry: clusterRegistry})
	}
	if len(probers) == 0 {
		return nil
	}

	gate := readiness.NewGate(fleetReadinessProbeInterval, logger.WithName("fleet-readiness"), probers...)
	gate.Start(ctx)
	logger.Info("Fleet readiness gate started", "prober_count", len(probers), "ready", gate.Ready())
	return gate
}

// setupSignalProcessingReconciler creates the atomic status manager
// (DD-PERF-001, SP-CACHE-001) and audit manager (Phase 3 refactoring,
// 2026-01-22), wires the SignalProcessingReconciler into mgr, and
// registers the healthz/readyz checks (including the #1553 Fleet
// readiness gate, a nil-safe no-op when neither Fleet dependency is
// configured). Exits the process on any failure, matching main()'s
// original fail-fast behavior.
func setupSignalProcessingReconciler(
	mgr ctrl.Manager,
	auditClient *spaudit.AuditClient,
	policyEvaluator *evaluator.Evaluator,
	signalModeClassifier *classifier.SignalModeClassifier,
	enrichment *signalProcessingEnrichment,
	fleetGate *readiness.Gate,
) {
	statusManager := spstatus.NewManager(mgr.GetClient(), mgr.GetAPIReader())
	setupLog.Info("SignalProcessing status manager initialized (DD-PERF-001 + SP-CACHE-001)")

	auditManager := spaudit.NewManager(auditClient)
	setupLog.Info("SignalProcessing audit manager initialized (Phase 3 refactoring)")

	if err := (&signalprocessing.SignalProcessingReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		AuditManager:         auditManager,
		Metrics:              enrichment.metrics,
		Recorder:             mgr.GetEventRecorderFor("signalprocessing-controller"),
		StatusManager:        statusManager,
		PolicyEvaluator:      policyEvaluator,      // ADR-060: Unified evaluator
		SignalModeClassifier: signalModeClassifier, // BR-SP-106: Proactive signal mode (ADR-054)
		K8sEnricher:          enrichment.k8sEnricher,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SignalProcessing")
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
	if fleetGate != nil {
		if err := mgr.AddReadyzCheck("fleet", fleetGate.Check); err != nil {
			setupLog.Error(err, "unable to register fleet readiness check")
			os.Exit(1)
		}
	}
}

// configureSignalProcessingTLSAndHotReload applies the OCP TLS security
// profile from config (Issue #748), starts the CA file watcher for
// client-side TLS hot-reload (Issue #756), and starts the log-level
// hot-reload watcher (Issue #875). Exits the process if the CA watcher
// fails to start, matching main()'s original fail-fast behavior. Returns a
// cleanup function that stops any watchers that were successfully started;
// callers should defer the returned function.
func configureSignalProcessingTLSAndHotReload(ctx context.Context, cfg *config.Config, configFile string, atomicLevel zaplog.AtomicLevel) func() {
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile); err != nil {
		setupLog.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.TLSProfile != "" {
		setupLog.Info("TLS security profile active", "profile", cfg.TLSProfile)
	}

	var cleanups []func()

	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, setupLog)
	if caWatchErr != nil {
		setupLog.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		cleanups = append(cleanups, caWatcher.Stop)
	}

	logLevelWatcher, logWatchErr := hotreload.NewFileWatcher(
		configFile,
		func(newContent string) error {
			var partial struct {
				Logging internalconfig.LoggingConfig `yaml:"logging"`
			}
			if err := yaml.Unmarshal([]byte(newContent), &partial); err != nil {
				return fmt.Errorf("failed to parse config for log level reload: %w", err)
			}
			return internalconfig.ParseAndSetLevel(atomicLevel, partial.Logging.Level)
		},
		setupLog.WithName("log-level-watcher"),
	)
	if logWatchErr != nil {
		setupLog.Error(logWatchErr, "Failed to create log level file watcher")
	} else if err := logLevelWatcher.Start(ctx); err != nil {
		setupLog.Info("Log level file watcher failed to start", "error", err)
	} else {
		setupLog.Info("Log level hot-reload watcher started", "path", configFile)
		cleanups = append(cleanups, logLevelWatcher.Stop)
	}

	return func() {
		for _, c := range cleanups {
			c()
		}
	}
}

func main() {
	// gocritic:exitAfterDefer — run() returns an exit code instead of calling
	// os.Exit directly so deferred cleanup (policyEvaluator.Stop, fleetGate.Stop,
	// cleanupHotReload, fleet client Close, cluster registry Stop, audit flush)
	// always runs.
	os.Exit(run())
}

func run() int {
	// ========================================
	// ADR-030: Configuration via YAML file
	// Single --config flag; all functional config in YAML ConfigMap
	// ========================================
	var configFile string
	flag.StringVar(&configFile, "config", config.DefaultConfigPath, "Path to configuration file")

	flag.Parse()

	// Issue #875: Bootstrap logger at INFO for config loading
	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	ctrl.SetLogger(zap.New(zap.Level(atomicLevel)))

	cfg, controllerNS := loadSignalProcessingConfig(configFile, atomicLevel)
	mgr := buildSignalProcessingManager(cfg, controllerNS)

	// ADR-032: Audit is MANDATORY - controller will crash if not configured
	auditStore, auditClient := wireSignalProcessingAudit(cfg)

	// Issue #419: Policy path is constructed from cfg.Classifier instead of hardcoded.
	ctx := ctrl.SetupSignalHandler()
	policyEvaluator := wireSignalProcessingPolicyEvaluator(ctx, cfg)
	defer policyEvaluator.Stop()

	signalModeClassifier := buildSignalModeClassifier()
	enrichment := wireSignalProcessingEnrichment(ctx, cfg, mgr)

	// #1553 / ADR-068 / BR-INTEGRATION-054 / BR-FLEET-003: fail closed on
	// Fleet dependency unreachability via readyz (pod-wide), instead of the
	// previous fail-open behavior of only logging an error.
	fleetGate := wireFleetReadinessGate(ctx, enrichment.fleetClient, enrichment.clusterRegistry, setupLog)
	if fleetGate != nil {
		defer fleetGate.Stop()
	}

	setupSignalProcessingReconciler(mgr, auditClient, policyEvaluator, signalModeClassifier, enrichment, fleetGate)

	cleanupHotReload := configureSignalProcessingTLSAndHotReload(ctx, cfg, configFile, atomicLevel)
	defer cleanupHotReload()

	// BR-INTEGRATION-054: Graceful shutdown for fleet MCP client
	if enrichment.fleetClient != nil {
		defer func() {
			setupLog.Info("Closing fleet MCP Gateway connection")
			if err := enrichment.fleetClient.Close(); err != nil {
				setupLog.Error(err, "Failed to close fleet MCP client gracefully")
			}
		}()
	}

	// BR-FLEET-003 (#1511): Graceful shutdown for cluster registry watcher
	if enrichment.clusterRegistry != nil {
		defer func() {
			setupLog.Info("Stopping cluster registry watcher")
			enrichment.clusterRegistry.Stop()
		}()
	}

	// ADR-032 §2: No Audit Loss - MUST flush pending events on any exit path.
	// Defer before mgr.Start so the flush runs even if Start returns an error.
	defer func() {
		setupLog.Info("Shutting down signalprocessing controller, flushing remaining audit events")
		if err := auditStore.Close(); err != nil {
			setupLog.Error(err, "Failed to close audit store gracefully")
		}
		setupLog.Info("Audit store closed successfully, all events flushed")
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		return 1
	}
	return 0
}
