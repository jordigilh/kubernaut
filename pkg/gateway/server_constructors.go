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

package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr" // BR-GATEWAY-093: Circuit breaker detection
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jordigilh/kubernaut/pkg/audit" // DD-AUDIT-003: Audit integration
	// Ogen generated audit types
	// BR-AUDIT-005 Gap #7: Standardized error details
	"github.com/jordigilh/kubernaut/pkg/shared/auth" // BR-GATEWAY-036/037: Shared auth middleware
	// ADR-052 Addendum 001: Exponential backoff with jitter
	sharedhealth "github.com/jordigilh/kubernaut/pkg/shared/health" // Issue #753: Dedicated health server
	// Issue #756: FileWatcher for cert rotation
	sharedtls "github.com/jordigilh/kubernaut/pkg/shared/tls" // Issue #493/#678: Conditional TLS

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1" // BR-GATEWAY-190: Lease resources for distributed locking
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes" // BR-GATEWAY-036/037: K8s clientset for TokenReview/SAR
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet" // ADR-068: Federated scope checking factory
	"github.com/jordigilh/kubernaut/pkg/gateway/adapters"
	"github.com/jordigilh/kubernaut/pkg/gateway/config"
	"github.com/jordigilh/kubernaut/pkg/gateway/k8s"
	"github.com/jordigilh/kubernaut/pkg/gateway/metrics"    // BR-109: Request ID middleware
	"github.com/jordigilh/kubernaut/pkg/gateway/processing" // BR-HTTP-015: Shared CORS library
	// DD-005: Shared sanitization library
	"github.com/jordigilh/kubernaut/pkg/shared/scope" // BR-SCOPE-002: Resource scope management
)

// NewServer creates a new Gateway server with default metrics registry
//
// This initializes:
// - Redis client with connection pooling
// - Kubernetes client (controller-runtime)
// - Processing pipeline components (deduplication, CRD creation)
// - Middleware (authentication, rate limiting)
// - HTTP routes (adapters, health, metrics)
//
// Typical startup sequence:
// 1. Create server: server := NewServer(cfg, logger)
// 2. Register adapters: server.RegisterAdapter(prometheusAdapter)
// 3. Start server: server.Start(ctx)
// 4. Graceful shutdown on signal: server.Stop(ctx)
//
// For testing with isolated metrics, use NewServerWithMetrics() instead.
func NewServer(cfg *config.ServerConfig, logger logr.Logger) (*Server, error) {
	return NewServerWithMetrics(cfg, logger, nil)
}

// NewServerWithMetrics creates a new Gateway server with custom metrics instance
//
// This constructor allows tests to provide isolated Prometheus registries,
// preventing "duplicate metrics collector registration" panics when creating
// multiple Gateway servers in the same test suite.
//
// Usage in tests:
//
//	registry := prometheus.NewRegistry()
//	metricsInstance := metrics.NewMetricsWithRegistry(registry)
//	server, err := gateway.NewServerWithMetrics(cfg, logger, metricsInstance)
//
// If metricsInstance is nil, automatically creates a new metrics instance with
// the default Prometheus registry (production mode).
//
// NewServerWithK8sClient creates a Gateway server with an existing K8s client (for testing)
// This ensures the Gateway uses the same K8s client as the test, avoiding cache synchronization issues
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native service
// DD-STATUS-001: For tests, ctrlClient serves as both client and apiReader (direct API access)
func NewServerWithK8sClient(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics, ctrlClient client.Client, authenticator auth.Authenticator, authorizer auth.Authorizer) (*Server, error) {
	// Use provided Kubernetes client (shared with test)
	k8sClient := k8s.NewClient(ctrlClient)

	// DD-STATUS-001: Use ctrlClient as apiReader for cache-bypassed reads
	// In test environments, this provides direct K8s API access
	return createServerWithClients(serverClients{
		Config: cfg, Logger: logger, MetricsInstance: metricsInstance,
		CtrlClient: ctrlClient, APIReader: ctrlClient, K8sClient: k8sClient,
		Authenticator: authenticator, Authorizer: authorizer,
	})
}

// newAuthMiddleware creates the auth middleware if both authenticator and authorizer are provided.
// Returns nil when either dependency is nil (backward compat for tests not yet wired with auth).
func newAuthMiddleware(authenticator auth.Authenticator, authorizer auth.Authorizer, namespace string, logger logr.Logger) *auth.Middleware {
	if authenticator == nil || authorizer == nil {
		return nil
	}
	return auth.NewMiddleware(authenticator, authorizer, auth.MiddlewareConfig{
		Namespace:    namespace,
		Resource:     "services",
		ResourceName: "gateway-service",
		Verb:         "create",
	}, logger)
}

// ServerTestDeps groups the injected dependencies for NewServerForTesting.
// Extracted per AGENTS.md's 8+-param Options-pattern rule.
type ServerTestDeps struct {
	Config          *config.ServerConfig
	Logger          logr.Logger
	MetricsInstance *metrics.Metrics
	CtrlClient      client.Client
	AuditStore      audit.AuditStore
	ScopeChecker    scope.ScopeChecker
	Authenticator   auth.Authenticator
	Authorizer      auth.Authorizer
}

// NewServerForTesting creates a Gateway server with injected dependencies for testing.
// This constructor allows injecting a mock audit store for unit tests.
//
// USAGE: Unit tests only - allows testing audit failure scenarios
// PRODUCTION: Use NewServer() or NewServerWithK8sClient() instead
func NewServerForTesting(deps ServerTestDeps) (*Server, error) {
	cfg, logger, metricsInstance, ctrlClient, auditStore, scopeChecker, authenticator, authorizer :=
		deps.Config, deps.Logger, deps.MetricsInstance, deps.CtrlClient, deps.AuditStore, deps.ScopeChecker, deps.Authenticator, deps.Authorizer

	// Use provided Kubernetes client
	k8sClient := k8s.NewClient(ctrlClient)

	// Initialize adapter registry
	adapterRegistry := adapters.NewAdapterRegistry(logger)

	// Create phaseChecker (for deduplication)
	// DD-GATEWAY-011: Use ctrlClient as apiReader for deduplication (test environment uses direct API access)
	// This ensures concurrent requests see each other's CRD creations immediately (GW-DEDUP-002 fix)
	phaseChecker := processing.NewPhaseBasedDeduplicationChecker(ctrlClient, cfg.Processing.Deduplication.CooldownPeriod)

	// Create statusUpdater
	statusUpdater := processing.NewStatusUpdater(ctrlClient, ctrlClient)

	// BR-GATEWAY-093: Wrap K8s client with circuit breaker (must be consistent in production AND tests)
	// FIX: GW-INT-AUD-019 was failing because circuit breaker was missing in test mode
	// Without circuit breaker, errors.Is(err, gobreaker.ErrOpenState) always returns false
	// Result: Test expected ERR_CIRCUIT_BREAKER_OPEN but got ERR_K8S_UNKNOWN
	cbClient := k8s.NewClientWithCircuitBreaker(k8sClient, metricsInstance)

	// BR-GATEWAY-190: Initialize distributed lock manager for multi-replica safety (test environment)
	// Uses ctrlClient as apiReader (test clients don't have separate cache/apiReader)
	var lockManager *processing.DistributedLockManager
	podName := os.Getenv("POD_NAME")
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default" // Test environment default
	}
	if podName != "" {
		lockManager = processing.NewDistributedLockManager(ctrlClient, namespace, podName)
	}

	// ADR-057: Resolve controller namespace for CRD operations and deduplication
	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		return nil, fmt.Errorf("failed to determine controller namespace: %w", err)
	}

	// Create server first (crdCreator set below after observer wiring)
	// DD-STATUS-001: Use ctrlClient as apiReader for readiness/dedup (test env uses direct API, no cache)
	server := &Server{
		config:              cfg,
		adapterRegistry:     adapterRegistry,
		statusUpdater:       statusUpdater,
		phaseChecker:        phaseChecker,
		k8sClient:           k8sClient,
		ctrlClient:          ctrlClient,
		apiReader:           ctrlClient,   // Test env: ctrlClient is uncached, works for readiness List
		lockManager:         lockManager,  // BR-GATEWAY-190: Multi-replica deduplication safety
		auditStore:          auditStore,   // Injected for testing
		scopeChecker:        scopeChecker, // BR-SCOPE-002: nil = no scope filtering
		controllerNamespace: controllerNS, // Issue #195: Used by ShouldDeduplicate
		authMiddleware:      newAuthMiddleware(authenticator, authorizer, controllerNS, logger),
		metricsInstance:     metricsInstance,
		logger:              logger,
	}

	// Issue #852: Mark cache as ready in test constructor. Real production startup
	// calls MarkCacheReady() after WaitForCacheSync; tests bypass cache startup.
	server.cacheReady.Store(true)

	// Create CRD creator with retry observer wired to server audit emission
	// BR-GATEWAY-058: retryAuditObserver emits gateway.crd.failed per retry attempt
	server.crdCreator = processing.NewCRDCreator(cbClient, logger, metricsInstance, &cfg.Processing.Retry, &retryAuditObserver{server: server}, controllerNS)

	// Setup HTTP server with routes
	router := server.setupRoutes()
	server.router = router
	handler := server.wrapWithMiddleware(router)

	server.httpServer = &http.Server{
		Addr:              cfg.Server.ListenAddr,
		Handler:           handler,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		ReadHeaderTimeout: 5 * time.Second, // Issue #673 L-2: Slowloris mitigation (gosec G112)
	}

	// Issue #753: Dedicated health and metrics servers for testing
	server.healthServer = sharedhealth.NewHealthServer(
		cfg.Server.HealthAddr,
		server.LivenessHandler(),
		server.ReadinessHandler(),
		!cfg.Server.DisableProfiling,
	)

	metricsMux := http.NewServeMux()
	var metricsHandler http.Handler
	if metricsInstance.Registry() != nil {
		metricsHandler = promhttp.HandlerFor(metricsInstance.Registry(), promhttp.HandlerOpts{})
	} else {
		metricsHandler = promhttp.Handler()
	}
	metricsMux.Handle("/metrics", metricsHandler)
	server.metricsServer = &http.Server{
		Addr:              cfg.Server.MetricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	if cfg.Server.TLS.Enabled() {
		isTLS, reloader, tlsErr := sharedtls.ConfigureConditionalTLS(server.httpServer, cfg.Server.TLS.CertDir)
		if tlsErr != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", tlsErr)
		}
		if isTLS {
			server.certReloader = reloader
			server.tlsCertDir = cfg.Server.TLS.CertDir
			server.logger.Info("TLS configured for Gateway server", "certDir", cfg.Server.TLS.CertDir)
		}
	}

	return server, nil
}

func NewServerWithMetrics(cfg *config.ServerConfig, logger logr.Logger, metricsInstance *metrics.Metrics) (*Server, error) {
	// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native service

	// Initialize Kubernetes clients
	// Get kubeconfig with standard Kubernetes precedence:
	// 1. --kubeconfig flag
	// 2. KUBECONFIG environment variable (used in tests: ~/.kube/kind-config)
	// 3. In-cluster config (production)
	// 4. $HOME/.kube/config (default)
	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// Create scheme with RemediationRequest CRD + core K8s types
	// This is required for controller-runtime to work with custom CRDs
	scheme := k8sruntime.NewScheme()
	_ = remediationv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)         // Add core types (Namespace, Pod, etc.)
	_ = appsv1.AddToScheme(scheme)         // #270: Apps types (Deployment, ReplicaSet, StatefulSet, DaemonSet)
	_ = batchv1.AddToScheme(scheme)        // #270: Batch types (Job, CronJob)
	_ = coordinationv1.AddToScheme(scheme) // BR-GATEWAY-190: Add Lease type for distributed locking

	// ADR-057: Discover controller namespace for CRD watch restriction
	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		return nil, fmt.Errorf("failed to determine controller namespace: %w", err)
	}

	// ========================================
	// BR-GATEWAY-185 v1.1: Create cached client with field index
	// Use spec.signalFingerprint (immutable, 64-char SHA256) instead of truncated labels
	// ========================================

	// #270: Build ByObject map with metadata-only informers for owner chain resolution
	// OwnerResolver and ScopeManager need cached lookups for Pods, ReplicaSets, Deployments, etc.
	byObject := adapters.OwnerChainCacheObjects()
	byObject[&remediationv1alpha1.RemediationRequest{}] = cache.ByObject{
		Namespaces: map[string]cache.Config{
			controllerNS: {},
		},
	}

	// Create cache for efficient queries (ADR-057: restrict RR to controller namespace)
	k8sCache, err := cache.New(kubeConfig, cache.Options{
		Scheme:   scheme,
		ByObject: byObject,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes cache: %w", err)
	}

	// Add field index for spec.signalFingerprint (BR-GATEWAY-185 v1.1)
	// This enables O(1) lookup by fingerprint without label truncation
	ctx, cancel := context.WithCancel(context.Background())
	if err := k8sCache.IndexField(ctx, &remediationv1alpha1.RemediationRequest{},
		"spec.signalFingerprint",
		func(obj client.Object) []string {
			rr := obj.(*remediationv1alpha1.RemediationRequest)
			return []string{rr.Spec.SignalFingerprint}
		}); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create fingerprint field index: %w", err)
	}

	// Start cache in background
	go func() {
		if err := k8sCache.Start(ctx); err != nil {
			logger.Error(err, "BR-GATEWAY-185: Cache stopped unexpectedly")
		}
	}()

	// Wait for cache sync (timeout after 30s)
	syncCtx, syncCancel := context.WithTimeout(ctx, 30*time.Second)
	defer syncCancel()
	if !k8sCache.WaitForCacheSync(syncCtx) {
		cancel()
		return nil, fmt.Errorf("failed to sync Kubernetes cache (timeout)")
	}
	logger.Info("BR-GATEWAY-185: Kubernetes cache synced with spec.signalFingerprint index")

	// Create client backed by cache (reads go through cache, writes go to API)
	ctrlClient, err := client.New(kubeConfig, client.Options{
		Scheme: scheme,
		Cache: &client.CacheOptions{
			Reader: k8sCache,
		},
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	// DD-STATUS-001: Create UNCACHED client for fresh API reads (adopted from RO pattern)
	// This is critical for reading CRDs immediately after creation (bypasses cache sync delays)
	// RO uses mgr.GetAPIReader() which returns an uncached client - we replicate that here
	apiReader, err := client.New(kubeConfig, client.Options{
		Scheme: scheme,
		// NO Cache option = direct API server reads (no cache)
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create uncached API reader: %w", err)
	}

	// k8s client wrapper (for CRD operations)
	k8sClient := k8s.NewClient(ctrlClient)

	// BR-GATEWAY-036/037: Create K8s clientset for TokenReview/SAR auth
	k8sClientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create Kubernetes clientset for auth: %w", err)
	}
	authenticator := auth.NewK8sAuthenticator(k8sClientset)
	authorizer := auth.NewK8sAuthorizer(k8sClientset)

	// DD-STATUS-001: Pass separate cached client and uncached apiReader
	// ctrlClient: Cached reads/writes for normal operations
	// apiReader: Uncached reads for fresh data (status refetch after CRD creation)
	server, err := createServerWithClients(serverClients{
		Config: cfg, Logger: logger, MetricsInstance: metricsInstance,
		CtrlClient: ctrlClient, APIReader: apiReader, K8sClient: k8sClient,
		Authenticator: authenticator, Authorizer: authorizer,
	})
	if err != nil {
		cancel()
		return nil, err
	}

	// Store cache and cancel function for cleanup
	server.k8sCache = k8sCache
	server.cacheCancel = cancel

	return server, nil
}

// serverClients groups the constructor-injected dependencies for
// createServerWithClients. Extracted per AGENTS.md's 8+-param
// Options-pattern rule.
type serverClients struct {
	Config          *config.ServerConfig
	Logger          logr.Logger
	MetricsInstance *metrics.Metrics
	CtrlClient      client.Client
	APIReader       client.Client
	K8sClient       *k8s.Client
	Authenticator   auth.Authenticator
	Authorizer      auth.Authorizer
}

// createServerWithClients is the common server creation logic
// DD-005: Uses logr.Logger for unified logging interface
// DD-GATEWAY-012: Redis REMOVED - Gateway is now Redis-free, K8s-native service
// DD-AUDIT-003: Audit store initialization for P0 service compliance
// DD-STATUS-001: apiReader parameter added for cache-bypassed status refetch (adopted from RO)
// BR-GATEWAY-190: apiReader is client.Client (not just client.Reader) for distributed locking Create/Update/Delete operations
func createServerWithClients(deps serverClients) (*Server, error) {
	cfg, logger, metricsInstance, ctrlClient, apiReader, k8sClient, authenticator, authorizer :=
		deps.Config, deps.Logger, deps.MetricsInstance, deps.CtrlClient, deps.APIReader, deps.K8sClient, deps.Authenticator, deps.Authorizer

	// Metrics are mandatory for observability
	// If nil, create a new metrics instance with default registry (production mode)
	if metricsInstance == nil {
		metricsInstance = metrics.NewMetrics()
	}

	// ADR-057: Controller namespace for CRD creation
	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		return nil, fmt.Errorf("failed to determine controller namespace: %w", err)
	}

	// Initialize processing pipeline components
	adapterRegistry := adapters.NewAdapterRegistry(logger)

	// DD-GATEWAY-016 / BR-GATEWAY-093: Circuit Breaker Integration (TDD GREEN COMPLETE)
	// ========================================
	// Circuit breaker protects Gateway from K8s API cascading failures
	//
	// Implementation:
	//   - ClientWithCircuitBreaker wraps k8sClient with fail-fast protection
	//   - CRDCreator uses k8s.ClientInterface (supports both Client and ClientWithCircuitBreaker)
	//   - Circuit breaker metrics: gateway_circuit_breaker_state
	//
	// Behavior:
	//   - Closed (0): Normal operation, all requests pass through
	//   - Open (2): K8s API degraded, requests fail-fast (<10ms) with gobreaker.ErrOpenState
	//   - Half-Open (1): Testing recovery, limited requests allowed
	//
	// Configuration:
	//   - Threshold: 50% failure rate over 10 requests
	//   - Timeout: 30s (transitions to half-open after 30s in open state)
	//   - Max Requests: 3 (half-open state test requests)
	//
	// Business Requirements:
	//   - BR-GATEWAY-093-A: Fail-fast when K8s API unavailable
	//   - BR-GATEWAY-093-B: Prevent cascade failures during K8s API overload
	//   - BR-GATEWAY-093-C: Observable metrics for SRE response
	//
	// Design Decision: DD-GATEWAY-016 (K8s API Circuit Breaker Implementation)
	// ========================================
	cbClient := k8s.NewClientWithCircuitBreaker(k8sClient, metricsInstance)

	// DD-GATEWAY-011: Status-based deduplication
	// All state in K8s RR status - Redis fully deprecated
	// DD-STATUS-001: Pass apiReader for cache-bypassed status refetch (adopted from RO pattern)
	statusUpdater := processing.NewStatusUpdater(ctrlClient, apiReader)
	// DD-GATEWAY-011: Use apiReader for deduplication to eliminate race conditions (cache-bypassed reads)
	// This ensures concurrent requests see each other's CRD creations immediately (GW-DEDUP-002 fix)
	phaseChecker := processing.NewPhaseBasedDeduplicationChecker(apiReader, cfg.Processing.Deduplication.CooldownPeriod)

	// BR-GATEWAY-190: Initialize distributed lock manager for multi-replica safety
	// Uses K8s Lease resources for distributed locking (no external dependencies)
	//
	// CRITICAL: Uses apiReader (non-cached client) for immediate consistency
	// WHY: Cached client has 5-50ms sync delay → race condition → duplicate locks
	// IMPACT: 3-24 API req/sec (production load) - acceptable per impact analysis
	var lockManager *processing.DistributedLockManager
	podName := os.Getenv("POD_NAME")
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "kubernaut-system"
	}
	if podName != "" {
		lockManager = processing.NewDistributedLockManager(apiReader, namespace, podName)
	} else {
		logger.Error(nil, "WARNING: POD_NAME not set — distributed locking disabled. "+
			"Multi-replica deployments WILL create duplicate RemediationRequests. "+
			"Set POD_NAME via Kubernetes downward API in production.")
	}

	// DD-AUDIT-003: Initialize audit store for P0 service compliance
	// Gateway MUST emit audit events per DD-AUDIT-003: Service Audit Trace Requirements
	// ADR-032 §1.5: "Every alert/signal processed (SignalProcessing, Gateway)"
	// ADR-032 §3: Gateway is P0 (Business-Critical) - MUST crash if audit unavailable
	var auditStore audit.AuditStore
	if cfg.DataStorage.URL != "" {
		// DD-API-001: Use OpenAPI generated client (not direct HTTP)
		// DD-AUTH-005 DI: cfg.DataStorage.Transport overrides the default SA token transport
		// (used by integration tests to inject authenticated transports)
		dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(cfg.DataStorage.URL, cfg.DataStorage.Timeout, cfg.DataStorage.Transport)
		if err != nil {
			// ADR-032 §2: No fallback/recovery allowed - crash on init failure
			return nil, fmt.Errorf("FATAL: failed to create Data Storage client - audit is MANDATORY per ADR-032 §1.5 (Gateway is P0 service): %w", err)
		}
		// ADR-030: Use buffer config from YAML ConfigMap
		auditConfig := audit.Config{
			BufferSize:    cfg.DataStorage.Buffer.BufferSize,
			BatchSize:     cfg.DataStorage.Buffer.BatchSize,
			FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
			MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
		}

		auditStore, err = audit.NewBufferedStore(dsClient, auditConfig, "gateway", logger)
		if err != nil {
			// ADR-032 §2: No fallback/recovery allowed - crash on init failure
			return nil, fmt.Errorf("FATAL: failed to create audit store - audit is MANDATORY per ADR-032 §1.5 (Gateway is P0 service): %w", err)
		}
		logger.Info("DD-AUDIT-003: Audit store initialized for P0 compliance (ADR-032 §1.5)",
			"data_storage_url", cfg.DataStorage.URL,
			"buffer_size", auditConfig.BufferSize)
	} else {
		// ADR-032 §1.5: Data Storage URL is MANDATORY for P0 services (Gateway processes alerts/signals)
		// ADR-032 §3: Gateway is P0 (Business-Critical) - MUST crash if audit unavailable
		return nil, fmt.Errorf("FATAL: Data Storage URL not configured - audit is MANDATORY per ADR-032 §1.5 (Gateway is P0 service)")
	}

	// BR-SCOPE-002: Initialize scope manager for label-based resource opt-in filtering.
	// Uses ctrlClient (informer-backed) because namespace labels are stable — they are set
	// well before alerts arrive and don't change mid-flight. This avoids hitting the API
	// server with 1-2 Get calls per incoming alert under production load.
	// Unlike phaseChecker/lockManager (which need apiReader for immediate consistency to
	// prevent duplicate CRDs and lock races), scope checking tolerates informer sync delay.
	scopeMgr := scope.NewManager(ctrlClient)

	// ADR-068: Federated scope checking via fleet.NewScopeChecker factory.
	scopeCheckerInstance, err := fleet.NewScopeChecker(scopeMgr, cfg.Fleet, logger)
	if err != nil {
		return nil, fmt.Errorf("fleet scope checker: %w", err)
	}
	if cfg.Fleet.Enabled && cfg.Fleet.EffectiveEndpoint() != "" {
		logger.Info("ADR-068: Federated scope checker enabled",
			"backend", cfg.Fleet.Backend, "endpoint", cfg.Fleet.EffectiveEndpoint())
	}

	authMW := newAuthMiddleware(authenticator, authorizer, controllerNS, logger)
	if authMW != nil {
		logger.Info("BR-GATEWAY-036/037: Auth middleware enabled",
			"namespace", controllerNS,
			"resource", "services/gateway-service",
			"verb", "create")
	}

	// Create server first (crdCreator set below after observer wiring)
	server := &Server{
		config:              cfg,
		adapterRegistry:     adapterRegistry,
		statusUpdater:       statusUpdater,
		phaseChecker:        phaseChecker,
		lockManager:         lockManager,
		k8sClient:           k8sClient,
		ctrlClient:          ctrlClient,
		apiReader:           apiReader, // ADR-057: Used for readiness K8s check (bypasses cache)
		auditStore:          auditStore,
		scopeChecker:        scopeCheckerInstance, // BR-SCOPE-002 + ADR-065: Label-based scope filtering (federated when fleet enabled)
		controllerNamespace: controllerNS,         // Issue #195: Used by ShouldDeduplicate
		authMiddleware:      authMW,
		metricsInstance:     metricsInstance,
		logger:              logger,
	}

	// Issue #852: Mark cache as ready. createServerWithClients is only reached after
	// WaitForCacheSync succeeds (production) or with direct K8s client (tests).
	server.cacheReady.Store(true)

	// Create CRD creator with retry observer wired to server audit emission
	// BR-GATEWAY-058: retryAuditObserver emits gateway.crd.failed per retry attempt
	server.crdCreator = processing.NewCRDCreator(cbClient, logger, metricsInstance, &cfg.Processing.Retry, &retryAuditObserver{server: server}, controllerNS)

	// 6. Setup HTTP server with routes
	router := server.setupRoutes()

	// 7. Store router reference for adapter registration
	server.router = router

	// 8. Wrap with additional middleware
	// BUSINESS OUTCOME: Enable operators to trace requests across Gateway components
	// TDD GREEN: Minimal implementation to make BR-109 tests pass
	handler := server.wrapWithMiddleware(router)

	server.httpServer = &http.Server{
		Addr:              cfg.Server.ListenAddr,
		Handler:           handler,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
		ReadHeaderTimeout: 5 * time.Second, // Issue #673 L-2: Slowloris mitigation (gosec G112)
	}

	// Issue #753: Dedicated health probe server (plain HTTP, never TLS)
	server.healthServer = sharedhealth.NewHealthServer(
		cfg.Server.HealthAddr,
		server.LivenessHandler(),
		server.ReadinessHandler(),
		!cfg.Server.DisableProfiling,
	)

	// Issue #753: Dedicated metrics server (plain HTTP, never TLS)
	metricsMux := http.NewServeMux()
	var metricsHandler http.Handler
	if metricsInstance.Registry() != nil {
		metricsHandler = promhttp.HandlerFor(metricsInstance.Registry(), promhttp.HandlerOpts{})
	} else {
		metricsHandler = promhttp.Handler()
	}
	metricsMux.Handle("/metrics", metricsHandler)
	server.metricsServer = &http.Server{
		Addr:              cfg.Server.MetricsAddr,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	if cfg.Server.TLS.Enabled() {
		isTLS, reloader, tlsErr := sharedtls.ConfigureConditionalTLS(server.httpServer, cfg.Server.TLS.CertDir)
		if tlsErr != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", tlsErr)
		}
		if isTLS {
			server.certReloader = reloader
			server.tlsCertDir = cfg.Server.TLS.CertDir
			server.logger.Info("TLS configured for Gateway server", "certDir", cfg.Server.TLS.CertDir)
		}
	}

	return server, nil
}
