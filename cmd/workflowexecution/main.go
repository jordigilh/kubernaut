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

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	zaplog "go.uber.org/zap"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	internalconfig "github.com/jordigilh/kubernaut/internal/config"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	"github.com/jordigilh/kubernaut/internal/version"
	"github.com/jordigilh/kubernaut/pkg/audit"
	fleetclient "github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
	"github.com/jordigilh/kubernaut/pkg/shared/hotreload"
	scope "github.com/jordigilh/kubernaut/pkg/shared/scope"
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls"
	weaudit "github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	weclient "github.com/jordigilh/kubernaut/pkg/workflowexecution/client"
	weconfig "github.com/jordigilh/kubernaut/pkg/workflowexecution/config"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	wemetrics "github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
	westatus "github.com/jordigilh/kubernaut/pkg/workflowexecution/status"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(workflowexecutionv1alpha1.AddToScheme(scheme))
	// Issue #868: Registering Tekton types with the scheme is safe even when
	// Tekton CRDs are absent — it only teaches the serializer about Go types.
	// Actual CRD availability is checked at startup via tektonCRDsAvailable.
	utilruntime.Must(tektonv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// tektonCRDsAvailable checks whether Tekton Pipelines CRDs are installed in
// the cluster by probing the REST mapper for the PipelineRun resource.
// Issue #868: Used to gate Tekton executor registration at startup.
func tektonCRDsAvailable(mapper meta.RESTMapper) bool {
	_, err := mapper.RESTMapping(
		schema.GroupKind{Group: "tekton.dev", Kind: "PipelineRun"}, "v1",
	)
	return err == nil
}

// setupWorkflowExecutionConfig loads and validates the WorkflowExecution
// config (ADR-030), applies the config-driven log level (Issue #875),
// discovers the controller namespace for CRD watch restriction (ADR-057;
// PipelineRun and Job are watched in kubernaut-workflows and are not
// restricted), and builds the controller manager. Exits the process on any
// failure, matching main()'s original fail-fast behavior.
func setupWorkflowExecutionConfig(configPath string, atomicLevel zaplog.AtomicLevel) (*weconfig.Config, ctrl.Manager, string) {
	setupLog.Info("Starting WorkflowExecution Controller",
		"version", version.Version,
		"gitCommit", version.GitCommit,
		"buildDate", version.BuildDate,
	)

	// Load configuration (file if provided, otherwise defaults)
	cfg, err := loadWorkflowExecutionConfig(configPath, atomicLevel)
	if err != nil {
		setupLog.Error(err, "Failed to load WorkflowExecution configuration")
		os.Exit(1)
	}
	if configPath != "" {
		setupLog.Info("Configuration loaded from file", "path", configPath)
	} else {
		setupLog.Info("Using default configuration (no config file provided)")
	}
	setupLog.Info("Log level configured from config file", "level", cfg.Logging.Level)

	// ADR-057: Discover controller namespace for CRD watch restriction
	// Note: PipelineRun and Job (Tekton/K8s) are watched in kubernaut-workflows - not restricted
	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		setupLog.Error(err, "unable to determine controller namespace")
		os.Exit(1)
	}

	mgr, err := buildManager(cfg, controllerNS)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Log configuration
	setupLog.Info("WorkflowExecution controller configuration",
		"executionNamespace", cfg.Execution.Namespace,
		"cooldownPeriod", cfg.Execution.CooldownPeriod,
		"metricsAddr", cfg.Controller.MetricsAddr,
		"healthProbeAddr", cfg.Controller.HealthProbeAddr,
		"dataStorageURL", cfg.DataStorage.URL,
	)

	return cfg, mgr, controllerNS
}

// initWorkflowExecutionServices initializes the mandatory audit store
// (DD-AUDIT-003, DD-AUDIT-002, ADR-038; audit is P0/business-critical per
// ADR-032 §2/§3 — the controller MUST crash if audit is unavailable rather
// than degrade gracefully), the dependency-injected metrics (DD-METRICS-001),
// the atomic status manager (DD-PERF-001), the phase manager
// (CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §1), and the audit manager
// (CONTROLLER_REFACTORING_PATTERN_LIBRARY.md §7). Exits the process if the
// audit store cannot be created.
func initWorkflowExecutionServices(cfg *weconfig.Config, mgr ctrl.Manager) (audit.AuditStore, *wemetrics.Metrics, *westatus.Manager, *wephase.Manager, *weaudit.Manager) {
	setupLog.Info("Initializing audit store (DD-AUDIT-003, DD-AUDIT-002)",
		"dataStorageURL", cfg.DataStorage.URL,
	)

	// Audit is MANDATORY per ADR-032 §2 - controller MUST crash if audit unavailable.
	// Per ADR-032 §3: WorkflowExecution is P0 (Business-Critical) - NO graceful degradation.
	// Rationale: Audit unavailability is a deployment/configuration error, not a transient
	// failure. The correct response is to crash and let Kubernetes orchestration detect the
	// misconfiguration.
	auditStore, err := buildAuditStore(cfg)
	if err != nil {
		setupLog.Error(err, "FATAL: failed to create audit store - audit is MANDATORY per ADR-032 §2")
		os.Exit(1) // Crash on init failure - NO RECOVERY ALLOWED
	}
	setupLog.Info("Audit store initialized successfully",
		"buffer_size", cfg.DataStorage.Buffer.BufferSize,
		"batch_size", cfg.DataStorage.Buffer.BatchSize,
		"flush_interval", cfg.DataStorage.Buffer.FlushInterval,
	)

	weMetrics := wemetrics.NewMetrics()
	setupLog.Info("WorkflowExecution metrics initialized and registered (DD-METRICS-001)")

	statusManager := westatus.NewManager(mgr.GetClient())
	setupLog.Info("WorkflowExecution status manager initialized (DD-PERF-001)")

	phaseManager := wephase.NewManager()
	setupLog.Info("WorkflowExecution phase manager initialized (P0: Phase State Machine)")

	auditManager := weaudit.NewManager(auditStore, ctrl.Log.WithName("audit-manager"))
	setupLog.Info("WorkflowExecution audit manager initialized (P3: Audit Manager)")

	return auditStore, weMetrics, statusManager, phaseManager, auditManager
}

func main() {
	// gocritic:exitAfterDefer — run() returns an exit code instead of calling
	// os.Exit directly so deferred cleanup (stopTLSWatcher, stopLogLevelWatcher,
	// wireShutdownHooks, fleetGate.Stop) always runs.
	os.Exit(run())
}

func run() int {
	// ========================================
	// CONFIGURATION LOADING (ADR-030)
	// Only --config flag is supported. All other settings are in the YAML config file.
	// ========================================
	var configPath string
	flag.StringVar(&configPath, "config", weconfig.DefaultConfigPath, "Path to configuration file (optional, uses defaults if not provided)")

	flag.Parse()

	// Issue #875: Bootstrap logger at INFO for config loading
	atomicLevel := internalconfig.DefaultLoggingConfig().NewAtomicLevel()
	ctrl.SetLogger(zap.New(zap.Level(atomicLevel)))

	cfg, mgr, controllerNS := setupWorkflowExecutionConfig(configPath, atomicLevel)

	auditStore, weMetrics, statusManager, phaseManager, auditManager := initWorkflowExecutionServices(cfg, mgr)

	// Issue #902: Initialize signal context and TLS before executor registry,
	// so that DefaultBaseTransport() has the CA reloader ready when AWX client
	// is constructed.
	ctx := ctrl.SetupSignalHandler()

	stopTLSWatcher := wireTLS(ctx, cfg, setupLog)
	defer stopTLSWatcher()

	// BR-FLEET-054: ClientFactory for local/remote cluster routing. If fleet
	// MCP Gateway is configured, create an mcpClientFactory that can route
	// executor operations to remote clusters. Otherwise, use
	// localClientFactory (pre-fleet behavior).
	clientFactory, fleetResilientClient := buildClientFactory(ctx, cfg, mgr.GetClient(), setupLog)

	// #1553 / ADR-068 / BR-FLEET-054: fail closed on Fleet dependency
	// unreachability via readyz (pod-wide), instead of the previous
	// fail-open behavior of only logging an error.
	fleetGate := wireFleetReadinessGate(ctx, fleetResilientClient, setupLog)

	// BR-WE-014: Executor Registry (Strategy Pattern). Issue #868: engines
	// are registered based on availability (job always, tekton via CRD
	// auto-discovery, ansible config-gated). BR-FLEET-054: executors use
	// ClientFactory for cluster routing.
	executorRegistry := buildExecutorRegistry(cfg, mgr, controllerNS, clientFactory, setupLog)

	// DD-WE-006: Create WorkflowQuerier for fetching dependencies from DS
	workflowQuerier, err := weclient.NewOgenWorkflowQuerierFromConfig(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		setupLog.Error(err, "Failed to create workflow querier (DD-WE-006) - continuing without dependency injection")
		// Non-fatal: controller will run without dependency injection
	} else {
		setupLog.Info("Workflow querier initialized (DD-WE-006)", "dataStorageURL", cfg.DataStorage.URL)
	}

	// Setup WorkflowExecution controller using NewReconciler constructor
	// which extracts infrastructure fields (Client, APIReader, Scheme, Recorder)
	// from the manager automatically.
	//
	// Issue #1481: DependencyValidator pre-flight/execution-time check removed.
	// Dependency existence is now validated exclusively at runtime by
	// Kubernetes when the Job/PipelineRun attempts to mount the volume
	// (BR-WORKFLOW-008 covers the resulting fail-fast/observability guarantees).
	reconciler := workflowexecution.NewReconciler(mgr, workflowexecution.ReconcilerOptions{
		ExecutionNamespace: cfg.Execution.Namespace,
		CooldownPeriod:     cfg.Execution.CooldownPeriod,
		Metrics:            weMetrics,
		StatusManager:      statusManager,
		AuditStore:         auditStore,
		PhaseManager:       phaseManager,
		AuditManager:       auditManager,
		ExecutorRegistry:   executorRegistry,
		WorkflowQuerier:    workflowQuerier,
	})
	if err = reconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "WorkflowExecution")
		return 1
	}
	//+kubebuilder:scaffold:builder

	if err := registerHealthChecks(mgr, executorRegistry, fleetGate); err != nil {
		setupLog.Error(err, "unable to set up health checks")
		return 1
	}

	// Issue #875: Log level hot-reload via FileWatcher
	stopLogLevelWatcher := wireLogLevelHotReload(ctx, configPath, atomicLevel, setupLog)
	defer stopLogLevelWatcher()

	// DD-AUDIT-002 + BR-FLEET-054: flush audit events and close the fleet
	// MCP client on any exit path (including os.Exit via mgr.Start failure).
	defer wireShutdownHooks(auditStore, fleetResilientClient, setupLog)()
	// #1553: stop the Fleet readiness gate's probe loop on shutdown.
	if fleetGate != nil {
		defer fleetGate.Stop()
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		return 1
	}
	return 0
}

func readSecretKeyDirect(clientset kubernetes.Interface, namespace, name, key string) (string, error) {
	secret, err := clientset.CoreV1().Secrets(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}
	val, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %s/%s", key, namespace, name)
	}
	return string(val), nil
}

// loadWorkflowExecutionConfig loads the controller configuration from
// configPath (or defaults if empty), applies the config-driven log level
// (Issue #875) to atomicLevel, and validates the result.
func loadWorkflowExecutionConfig(configPath string, atomicLevel zaplog.AtomicLevel) (*weconfig.Config, error) {
	var cfg *weconfig.Config
	var err error
	if configPath != "" {
		cfg, err = weconfig.LoadFromFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration file %q: %w", configPath, err)
		}
	} else {
		cfg = weconfig.DefaultConfig()
	}

	// Issue #875: Apply config-driven log level
	atomicLevel.SetLevel(cfg.Logging.ZapLevel())

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}
	return cfg, nil
}

// buildManager constructs the controller-runtime manager, restricting the
// WorkflowExecution CRD watch to controllerNS (ADR-057) and Secret/ConfigMap
// watches to the configured execution namespace.
func buildManager(cfg *weconfig.Config, controllerNS string) (ctrl.Manager, error) {
	execNS := cfg.Execution.Namespace

	return ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&workflowexecutionv1alpha1.WorkflowExecution{}: {
					Namespaces: map[string]cache.Config{
						controllerNS: {},
					},
				},
				&corev1.Secret{}: {
					Namespaces: map[string]cache.Config{
						execNS: {},
					},
				},
				&corev1.ConfigMap{}: {
					Namespaces: map[string]cache.Config{
						execNS: {},
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
}

// buildAuditStore constructs the DD-AUDIT-002 buffered audit store backed by
// the Data Storage Service. Per ADR-032 §2/§3, audit is mandatory for
// WorkflowExecution (P0, Business-Critical) - callers MUST treat a non-nil
// error as fatal.
func buildAuditStore(cfg *weconfig.Config) (audit.AuditStore, error) {
	// Create OpenAPI client for Data Storage Service (DD-API-001 + DD-AUDIT-002 V2.0)
	// Uses generated OpenAPI client for type safety and contract validation
	dsClient, err := audit.NewOpenAPIClientAdapter(cfg.DataStorage.URL, cfg.DataStorage.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create data storage client (url=%s): %w", cfg.DataStorage.URL, err)
	}

	// Create buffered audit store using shared library (DD-AUDIT-002)
	// ADR-030: Audit buffer config from YAML (not RecommendedConfig)
	auditConfig := audit.Config{
		BufferSize:    cfg.DataStorage.Buffer.BufferSize,
		BatchSize:     cfg.DataStorage.Buffer.BatchSize,
		FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
		MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
	}
	auditStore, err := audit.NewBufferedStore(
		dsClient,
		auditConfig,
		"workflowexecution",
		ctrl.Log.WithName("audit"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create buffered audit store: %w", err)
	}
	return auditStore, nil
}

// wireTLS applies the config-driven TLS security profile and starts the
// shared CA file watcher (Issue #902). Callers must invoke the returned stop
// function on shutdown.
func wireTLS(ctx context.Context, cfg *weconfig.Config, logger logr.Logger) func() {
	if err := sharedtls.SetDefaultSecurityProfileFromConfig(cfg.TLSProfile); err != nil {
		logger.Error(err, "Invalid TLS security profile in config, using default TLS 1.2")
	} else if cfg.TLSProfile != "" {
		logger.Info("TLS security profile active", "profile", cfg.TLSProfile)
	}

	caWatcher, caWatchErr := sharedtls.StartCAFileWatcher(ctx, logger)
	if caWatchErr != nil {
		logger.Error(caWatchErr, "Failed to start CA file watcher")
		os.Exit(1)
	}
	if caWatcher != nil {
		return caWatcher.Stop
	}
	return func() {}
}

// buildClientFactory constructs the BR-FLEET-054 ClientFactory used for
// local/remote cluster routing. When cfg.Fleet.Endpoint is configured, it
// attempts to connect to the Fleet MCP Gateway and returns an MCP-routed
// factory; on connection failure (or when Fleet is unconfigured), it falls
// back to a local factory. The returned *fleetclient.ResilientClient is nil
// unless Fleet is configured; #1553: on a connection failure it is still
// returned (not discarded) so wireFleetReadinessGate can attach an
// MCPClientProber that keeps retrying via the periodic probe — this is
// what allows /readyz to recover once Fleet comes back, instead of
// requiring a pod restart (mirrors GW/RO/EM/SP's identical change).
// Callers should Close() a non-nil client on shutdown. localClient is
// pre-built by the caller (independently testable with a fake).
func buildClientFactory(ctx context.Context, cfg *weconfig.Config, localClient client.Client, logger logr.Logger) (weexecutor.ClientFactory, *fleetclient.ResilientClient) {
	if cfg.Fleet.Endpoint == "" {
		return weexecutor.NewLocalClientFactory(localClient), nil
	}

	logger.Info("Fleet MCP Gateway configured, connecting for remote execution...",
		"endpoint", cfg.Fleet.Endpoint,
		"oauth2Enabled", cfg.Fleet.OAuth2.Enabled)

	fleetLog := ctrl.Log.WithName("fleet-oauth2")
	fleetOpts := []fleetclient.Option{}
	if cfg.Fleet.OAuth2.Enabled {
		basePath := "/etc/workflowexecution/fleet-oauth2"
		if cfg.Fleet.OAuth2.CredentialsSecretRef != "" {
			basePath = "/etc/workflowexecution/" + cfg.Fleet.OAuth2.CredentialsSecretRef
		}
		reloadCfg := fleetclient.ReloadableOAuth2Config{
			TokenURL:         cfg.Fleet.OAuth2.TokenURL,
			ClientIDPath:     basePath + "/client-id",
			ClientSecretPath: basePath + "/client-secret",
			Scopes:           fleetclient.DefaultFleetScopes(cfg.Fleet.OAuth2.Scopes),
			TokenTimeout:     10 * time.Second,
			TlsCaFile:        cfg.Fleet.OAuth2.TLSCAFile,
		}
		fleetOpts = append(fleetOpts,
			fleetclient.WithReloadableOAuth2Transport(reloadCfg, fleetLog),
		)
		logger.Info("fleet OAuth2 authentication configured (hot-reloadable)",
			"tokenURL", cfg.Fleet.OAuth2.TokenURL,
			"secretPath", basePath)
	}

	resilienceCfg := fleetclient.DefaultResilienceConfig()
	fleetResilientClient, fleetErr := fleetclient.NewResilient(
		ctx, cfg.Fleet.Endpoint, resilienceCfg,
		ctrl.Log.WithName("fleet-client"), fleetOpts...,
	)
	if fleetErr != nil {
		logger.Error(fleetErr, "Fleet MCP Gateway connection failed at startup; readiness will report "+
			"NotReady and keep retrying in the background; remote execution disabled until reconnect",
			"endpoint", cfg.Fleet.Endpoint)
		return weexecutor.NewLocalClientFactory(localClient), fleetResilientClient
	}

	logger.Info("Fleet MCP Gateway connected, remote execution enabled",
		"endpoint", cfg.Fleet.Endpoint)
	return weexecutor.NewMCPClientFactory(localClient, fleetResilientClient.Session()), fleetResilientClient
}

// buildExecutorRegistry wires up the BR-WE-014 executor registry (Strategy
// Pattern). Issue #868: engines are registered based on availability - job
// always, tekton via CRD auto-discovery (or explicit config), ansible
// config-gated. BR-FLEET-054: executors use clientFactory for cluster
// routing.
func buildExecutorRegistry(cfg *weconfig.Config, mgr ctrl.Manager, controllerNS string, clientFactory weexecutor.ClientFactory, logger logr.Logger) *weexecutor.Registry {
	executorRegistry := weexecutor.NewRegistry()
	executorRegistry.Register("job", weexecutor.NewJobExecutorWithFactory(clientFactory))

	knownOptionalEngines := make([]string, 0, 2)

	// Tekton: auto-discover CRDs unless explicitly disabled (Issue #868)
	knownOptionalEngines = append(knownOptionalEngines, "tekton")
	if cfg.TektonEnabled() {
		if tektonCRDsAvailable(mgr.GetRESTMapper()) {
			executorRegistry.Register("tekton", weexecutor.NewTektonExecutorWithFactory(clientFactory))
			logger.Info("Tekton executor registered (CRDs discovered)")
		} else {
			logger.Info("Tekton executor not registered (CRDs not found)",
				"group", "tekton.dev", "kind", "PipelineRun", "version", "v1")
		}
	} else {
		logger.Info("Tekton executor disabled by configuration")
	}

	// BR-WE-015: Conditionally register Ansible executor if configured.
	knownOptionalEngines = append(knownOptionalEngines, "ansible")
	registerAnsibleExecutor(cfg, mgr, controllerNS, executorRegistry, logger)

	// Issue #868: Log engine availability summary at startup
	available, unavailable := executorRegistry.EngineAvailability(knownOptionalEngines)
	logger.Info("Executor registry initialized", "available", available, "unavailable", unavailable)

	return executorRegistry
}

// registerAnsibleExecutor conditionally registers the Ansible/AWX executor.
// Uses a direct clientset (not the cached mgr.GetClient()) because the
// controller-runtime cache is not started until mgr.Start().
func registerAnsibleExecutor(cfg *weconfig.Config, mgr ctrl.Manager, controllerNS string, executorRegistry *weexecutor.Registry, logger logr.Logger) {
	if cfg.Ansible == nil || cfg.Ansible.TokenSecretRef == nil {
		if cfg.Ansible != nil {
			logger.Info("Ansible config present but tokenSecretRef not set, ansible executor will not be available")
		}
		return
	}

	ns := cfg.Ansible.TokenSecretRef.Namespace
	if ns == "" {
		ns = controllerNS
	}
	directClientset, csErr := kubernetes.NewForConfig(mgr.GetConfig())
	if csErr != nil {
		logger.Error(csErr, "Failed to create direct clientset for AWX token read")
		return
	}
	token, readErr := readSecretKeyDirect(directClientset, ns, cfg.Ansible.TokenSecretRef.Name, cfg.Ansible.TokenSecretRef.Key)
	if readErr != nil {
		logger.Error(readErr, "Failed to read AWX token secret, ansible executor not available")
		return
	}
	awxClient, awxErr := weexecutor.NewAWXHTTPClient(cfg.Ansible.APIURL, token)
	if awxErr != nil {
		logger.Error(awxErr, "Failed to create AWX client")
		return
	}
	orgID := cfg.Ansible.OrganizationID
	if orgID <= 0 {
		orgID = 1
	}
	executorRegistry.Register("ansible", weexecutor.NewAnsibleExecutor(awxClient, mgr.GetClient(), directClientset, orgID, ctrl.Log.WithName("ansible-executor")))
	logger.Info("Ansible executor registered", "awxURL", cfg.Ansible.APIURL, "organizationID", orgID)
}

// registerHealthChecks wires the standard healthz/readyz probes plus the
// Issue #868 "engines" readyz sub-check, which reports execution engine
// availability (the job engine is always registered, so this passes in
// normal operation but provides a clear signal if the registry is
// misconfigured), and the #1553 Fleet readiness gate (a nil fleetGate is a
// no-op — Fleet unconfigured).
func registerHealthChecks(mgr ctrl.Manager, executorRegistry *weexecutor.Registry, fleetGate *readiness.Gate) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}
	if err := mgr.AddReadyzCheck("engines", healthz.Checker(func(_ *http.Request) error {
		if len(executorRegistry.Engines()) == 0 {
			return fmt.Errorf("no execution engines available")
		}
		return nil
	})); err != nil {
		return fmt.Errorf("unable to set up engines readyz check: %w", err)
	}
	if fleetGate != nil {
		if err := mgr.AddReadyzCheck("fleet", fleetGate.Check); err != nil {
			return fmt.Errorf("unable to set up fleet readiness check: %w", err)
		}
	}
	return nil
}

// fleetReadinessProbeInterval controls how often the Fleet readiness gate
// re-probes its dependencies once started (mirrors cmd/gateway/main.go,
// cmd/remediationorchestrator/main.go, cmd/effectivenessmonitor/main.go,
// cmd/signalprocessing/main.go).
const fleetReadinessProbeInterval = 15 * time.Second

// wireFleetReadinessGate builds and starts the Fleet dependency readiness
// gate (#1553, ADR-068, BR-FLEET-054): once Fleet is enabled, WE's
// pod-wide readyz must fail closed when the MCP Gateway becomes
// unreachable, instead of the previous fail-open behavior of only logging
// an error. WE has no scope-checker dependency (unlike GW/RO), so its
// gate only ever carries an MCPClientProber. Returns nil when
// fleetResilientClient is nil (buildClientFactory only returns a non-nil
// client when Fleet.Endpoint is configured). The caller registers the
// returned Gate's Check method via mgr.AddReadyzCheck and must Stop() it
// on shutdown.
func wireFleetReadinessGate(ctx context.Context, fleetResilientClient *fleetclient.ResilientClient, logger logr.Logger) *readiness.Gate {
	if fleetResilientClient == nil {
		return nil
	}

	prober := &readiness.MCPClientProber{Client: fleetResilientClient}
	gate := readiness.NewGate(fleetReadinessProbeInterval, logger.WithName("fleet-readiness"), prober)
	gate.Start(ctx)
	logger.Info("Fleet readiness gate started", "ready", gate.Ready())
	return gate
}

// wireShutdownHooks returns a single cleanup function that flushes the
// DD-AUDIT-002 buffered audit store and closes the BR-FLEET-054 fleet MCP
// client (when present). Callers should defer the returned function
// immediately so it runs on any exit path, including os.Exit via
// mgr.Start failure.
func wireShutdownHooks(auditStore audit.AuditStore, fleetResilientClient *fleetclient.ResilientClient, logger logr.Logger) func() {
	return func() {
		if auditStore != nil {
			logger.Info("Flushing audit events on shutdown (DD-AUDIT-002)")
			if closeErr := auditStore.Close(); closeErr != nil {
				logger.Error(closeErr, "Failed to close audit store")
			} else {
				logger.Info("Audit store closed successfully")
			}
		}
		if fleetResilientClient != nil {
			logger.Info("Closing fleet MCP Gateway connection")
			if err := fleetResilientClient.Close(); err != nil {
				logger.Error(err, "Failed to close fleet MCP client gracefully")
			}
		}
	}
}

// wireLogLevelHotReload starts the Issue #875 FileWatcher that applies
// config-driven log level changes to atomicLevel without a restart. The
// returned stop function is safe to call even if the watcher failed to
// start.
func wireLogLevelHotReload(ctx context.Context, configPath string, atomicLevel zaplog.AtomicLevel, logger logr.Logger) func() {
	logLevelWatcher, err := hotreload.NewFileWatcher(
		configPath,
		func(newContent string) error {
			var partial struct {
				Logging internalconfig.LoggingConfig `yaml:"logging"`
			}
			if err := yaml.Unmarshal([]byte(newContent), &partial); err != nil {
				return fmt.Errorf("failed to parse config for log level reload: %w", err)
			}
			return internalconfig.ParseAndSetLevel(atomicLevel, partial.Logging.Level)
		},
		logger.WithName("log-level-watcher"),
	)
	if err != nil {
		logger.Error(err, "Failed to create log level file watcher")
		return func() {}
	}
	if err := logLevelWatcher.Start(ctx); err != nil {
		logger.Info("Log level file watcher failed to start", "error", err)
		return func() {}
	}
	logger.Info("Log level hot-reload watcher started", "path", configPath)
	return logLevelWatcher.Stop
}
