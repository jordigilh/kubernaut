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
	"k8s.io/client-go/rest"
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
	server := assembleServer(serverAssemblyInputs{
		cfg:             cfg,
		logger:          logger,
		metricsInstance: metricsInstance,
		adapterRegistry: adapterRegistry,
		statusUpdater:   statusUpdater,
		phaseChecker:    phaseChecker,
		lockManager:     lockManager, // BR-GATEWAY-190: Multi-replica deduplication safety
		k8sClient:       k8sClient,
		ctrlClient:      ctrlClient,
		apiReader:       ctrlClient,   // Test env: ctrlClient is uncached, works for readiness List
		auditStore:      auditStore,   // Injected for testing
		scopeChecker:    scopeChecker, // BR-SCOPE-002: nil = no scope filtering
		controllerNS:    controllerNS,
		authenticator:   authenticator,
		authorizer:      authorizer,
	})

	// Create CRD creator with retry observer wired to server audit emission
	// BR-GATEWAY-058: retryAuditObserver emits gateway.crd.failed per retry attempt
	server.crdCreator = processing.NewCRDCreator(cbClient, logger, metricsInstance, &cfg.Processing.Retry, &retryAuditObserver{server: server}, controllerNS)

	server.wireTestHTTPAndAncillaryServers(cfg, metricsInstance)

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

// buildGatewayScheme creates the runtime.Scheme with the RemediationRequest
// CRD plus the core/apps/batch/coordination K8s types Gateway needs for
// owner-chain resolution and distributed locking. Extracted from
// NewServerWithMetrics (funlen).
func buildGatewayScheme() *k8sruntime.Scheme {
	scheme := k8sruntime.NewScheme()
	_ = remediationv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)         // Add core types (Namespace, Pod, etc.)
	_ = appsv1.AddToScheme(scheme)         // #270: Apps types (Deployment, ReplicaSet, StatefulSet, DaemonSet)
	_ = batchv1.AddToScheme(scheme)        // #270: Batch types (Job, CronJob)
	_ = coordinationv1.AddToScheme(scheme) // BR-GATEWAY-190: Add Lease type for distributed locking
	return scheme
}

// buildGatewayCache creates the controller-runtime cache with a
// spec.signalFingerprint field index (BR-GATEWAY-185 v1.1), eagerly starts
// the scope/owner-resolution informers (Issue #54 SOC2 gap fix), and blocks
// until the cache syncs (30s timeout). Extracted from NewServerWithMetrics
// (funlen).
//
// Eager informer start rationale: without this, informers for scope-check and
// owner-resolution kinds (Deployment, Namespace, Pod, etc.) are created LAZILY
// on first use. pkg/shared/scope/manager.go's Get() call blocks until that
// informer syncs, bounded by a defensive 5s timeout — under CI/production
// load the first-ever List+Watch establishment can exceed 5s, causing the
// timeout to fire and a correctly-labeled resource to be misclassified as
// unmanaged (BR-SCOPE-001) until the informer catches up. Eagerly starting
// these informers here, covered by this function's own 30s WaitForCacheSync,
// ensures the readiness probe does not report ready until scope checks are
// reliable.
//
// ConfigMap and Secret are intentionally excluded: RBAC for these two kinds is
// not consistently granted cluster-wide (the Helm chart grants ConfigMap only
// namespace-scoped, and Secret access was deliberately removed entirely —
// Issue #673 H-3, "gateway has no business need for secret access"). Eagerly
// starting their informers would make WaitForCacheSync hang wherever that
// narrower RBAC applies, crash-looping Gateway. They keep lazy-informer behavior.
//
// The returned cancel func is the cache's lifecycle handle: callers MUST call
// it on any subsequent error, and store it for later shutdown on success.
func buildGatewayCache(
	kubeConfig *rest.Config,
	scheme *k8sruntime.Scheme,
	controllerNS string,
	logger logr.Logger,
) (cache.Cache, context.CancelFunc, error) {
	// #270: Build ByObject map with metadata-only informers for owner chain resolution
	// OwnerResolver and ScopeManager need cached lookups for Pods, ReplicaSets, Deployments, etc.
	byObject := adapters.OwnerChainCacheObjects()
	byObject[&remediationv1alpha1.RemediationRequest{}] = cache.ByObject{
		Namespaces: map[string]cache.Config{
			controllerNS: {}, // ADR-057: restrict RR to controller namespace
		},
	}

	k8sCache, err := cache.New(kubeConfig, cache.Options{
		Scheme:   scheme,
		ByObject: byObject,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Kubernetes cache: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Add field index for spec.signalFingerprint (BR-GATEWAY-185 v1.1):
	// enables O(1) lookup by fingerprint without label truncation.
	if err := k8sCache.IndexField(ctx, &remediationv1alpha1.RemediationRequest{},
		"spec.signalFingerprint",
		func(obj client.Object) []string {
			rr := obj.(*remediationv1alpha1.RemediationRequest)
			return []string{rr.Spec.SignalFingerprint}
		}); err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create fingerprint field index: %w", err)
	}

	for obj := range byObject {
		if kind := obj.GetObjectKind().GroupVersionKind().Kind; kind == "ConfigMap" || kind == "Secret" {
			continue
		}
		if _, err := k8sCache.GetInformer(ctx, obj); err != nil {
			cancel()
			return nil, nil, fmt.Errorf("failed to start informer for %T: %w", obj, err)
		}
	}

	go func() {
		if err := k8sCache.Start(ctx); err != nil {
			logger.Error(err, "BR-GATEWAY-185: Cache stopped unexpectedly")
		}
	}()

	syncCtx, syncCancel := context.WithTimeout(ctx, 30*time.Second)
	defer syncCancel()
	if !k8sCache.WaitForCacheSync(syncCtx) {
		cancel()
		return nil, nil, fmt.Errorf("failed to sync Kubernetes cache (timeout)")
	}
	logger.Info("BR-GATEWAY-185: Kubernetes cache synced with spec.signalFingerprint index")

	return k8sCache, cancel, nil
}

// buildGatewayClients creates the cached controller-runtime client (reads
// through cache, writes to API) and the uncached apiReader (DD-STATUS-001:
// fresh reads immediately after CRD creation, bypassing cache sync delay —
// adopted from RemediationOrchestrator's mgr.GetAPIReader() pattern).
// Extracted from NewServerWithMetrics (funlen).
func buildGatewayClients(kubeConfig *rest.Config, scheme *k8sruntime.Scheme, k8sCache cache.Cache) (ctrlClient, apiReader client.Client, err error) {
	ctrlClient, err = client.New(kubeConfig, client.Options{
		Scheme: scheme,
		Cache: &client.CacheOptions{
			Reader: k8sCache,
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	apiReader, err = client.New(kubeConfig, client.Options{
		Scheme: scheme,
		// NO Cache option = direct API server reads (no cache)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create uncached API reader: %w", err)
	}

	return ctrlClient, apiReader, nil
}

// buildGatewayAuth creates the K8s TokenReview/SAR-backed authenticator and
// authorizer (BR-GATEWAY-036/037). Extracted from NewServerWithMetrics (funlen).
func buildGatewayAuth(kubeConfig *rest.Config) (auth.Authenticator, auth.Authorizer, error) {
	k8sClientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Kubernetes clientset for auth: %w", err)
	}
	return auth.NewK8sAuthenticator(k8sClientset), auth.NewK8sAuthorizer(k8sClientset), nil
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

	scheme := buildGatewayScheme()

	// ADR-057: Discover controller namespace for CRD watch restriction
	controllerNS, err := scope.GetControllerNamespace()
	if err != nil {
		return nil, fmt.Errorf("failed to determine controller namespace: %w", err)
	}

	// BR-GATEWAY-185 v1.1: Cached client with spec.signalFingerprint field index
	// (immutable, 64-char SHA256) instead of truncated labels.
	k8sCache, cancel, err := buildGatewayCache(kubeConfig, scheme, controllerNS, logger)
	if err != nil {
		return nil, err
	}

	ctrlClient, apiReader, err := buildGatewayClients(kubeConfig, scheme, k8sCache)
	if err != nil {
		cancel()
		return nil, err
	}

	// k8s client wrapper (for CRD operations)
	k8sClient := k8s.NewClient(ctrlClient)

	// BR-GATEWAY-036/037: Auth (TokenReview/SAR)
	authenticator, authorizer, err := buildGatewayAuth(kubeConfig)
	if err != nil {
		cancel()
		return nil, err
	}

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
// buildLockManager initializes the distributed lock manager for multi-replica
// safety (BR-GATEWAY-190), or returns nil (with a loud warning) when POD_NAME
// is not set, since locking requires a stable per-pod identity. Extracted
// from createServerWithClients to keep its cognitive complexity low.
func buildLockManager(apiReader client.Client, logger logr.Logger) *processing.DistributedLockManager {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		logger.Error(nil, "WARNING: POD_NAME not set — distributed locking disabled. "+
			"Multi-replica deployments WILL create duplicate RemediationRequests. "+
			"Set POD_NAME via Kubernetes downward API in production.")
		return nil
	}

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "kubernaut-system"
	}
	return processing.NewDistributedLockManager(apiReader, namespace, podName)
}

// buildAuditStore initializes the audit store for P0 service compliance.
// DD-AUDIT-003 / ADR-032 §1.5/§3: Gateway MUST crash on init failure or when
// Data Storage is unconfigured — audit is mandatory, not best-effort, for a
// P0 (business-critical) service. Extracted from createServerWithClients to
// keep its cognitive complexity low.
func buildAuditStore(cfg *config.ServerConfig, logger logr.Logger) (audit.AuditStore, error) {
	if cfg.DataStorage.URL == "" {
		return nil, fmt.Errorf("FATAL: Data Storage URL not configured - audit is MANDATORY per ADR-032 §1.5 (Gateway is P0 service)")
	}

	// DD-API-001: Use OpenAPI generated client (not direct HTTP)
	// DD-AUTH-005 DI: cfg.DataStorage.Transport overrides the default SA token transport
	// (used by integration tests to inject authenticated transports)
	dsClient, err := audit.NewOpenAPIClientAdapterWithTransport(cfg.DataStorage.URL, cfg.DataStorage.Timeout, cfg.DataStorage.Transport)
	if err != nil {
		return nil, fmt.Errorf("FATAL: failed to create Data Storage client - audit is MANDATORY per ADR-032 §1.5 (Gateway is P0 service): %w", err)
	}

	// ADR-030: Use buffer config from YAML ConfigMap
	auditConfig := audit.Config{
		BufferSize:    cfg.DataStorage.Buffer.BufferSize,
		BatchSize:     cfg.DataStorage.Buffer.BatchSize,
		FlushInterval: cfg.DataStorage.Buffer.FlushInterval,
		MaxRetries:    cfg.DataStorage.Buffer.MaxRetries,
	}

	auditStore, err := audit.NewBufferedStore(dsClient, auditConfig, "gateway", logger)
	if err != nil {
		return nil, fmt.Errorf("FATAL: failed to create audit store - audit is MANDATORY per ADR-032 §1.5 (Gateway is P0 service): %w", err)
	}
	logger.Info("DD-AUDIT-003: Audit store initialized for P0 compliance (ADR-032 §1.5)",
		"data_storage_url", cfg.DataStorage.URL,
		"buffer_size", auditConfig.BufferSize)
	return auditStore, nil
}

// buildScopeChecker initializes label-based resource opt-in filtering
// (BR-SCOPE-002), federated across fleet members when enabled (ADR-068).
// Uses ctrlClient (informer-backed) because namespace labels are stable and
// tolerate informer sync delay, unlike phaseChecker/lockManager which need
// apiReader for immediate consistency. Extracted from createServerWithClients
// to keep its cognitive complexity low.
func buildScopeChecker(ctrlClient client.Client, cfg *config.ServerConfig, logger logr.Logger) (scope.ScopeChecker, error) {
	scopeMgr := scope.NewManager(ctrlClient)

	scopeCheckerInstance, err := fleet.NewScopeChecker(scopeMgr, cfg.Fleet, logger)
	if err != nil {
		return nil, fmt.Errorf("fleet scope checker: %w", err)
	}
	if cfg.Fleet.Enabled && cfg.Fleet.EffectiveEndpoint() != "" {
		logger.Info("ADR-068: Federated scope checker enabled",
			"backend", cfg.Fleet.Backend, "endpoint", cfg.Fleet.EffectiveEndpoint())
	}
	return scopeCheckerInstance, nil
}

// buildCorePipelineComponents constructs the adapter registry, circuit
// breaker-wrapped K8s client, status updater, and phase-based deduplication
// checker. Extracted from createServerWithClients to keep its line count low.
//
// DD-GATEWAY-016 / BR-GATEWAY-093: Circuit Breaker Integration.
// Circuit breaker protects Gateway from K8s API cascading failures:
//   - ClientWithCircuitBreaker wraps k8sClient with fail-fast protection
//   - CRDCreator uses k8s.ClientInterface (supports both Client and ClientWithCircuitBreaker)
//   - Circuit breaker metrics: gateway_circuit_breaker_state
//   - States: Closed(0)=normal, Open(2)=fail-fast (<10ms, gobreaker.ErrOpenState), Half-Open(1)=testing recovery
//   - Config: 50% failure threshold over 10 requests, 30s open timeout, 3 half-open test requests
//
// DD-GATEWAY-011: Status-based deduplication — all state lives in K8s RR
// status (Redis fully deprecated). Both statusUpdater and phaseChecker use
// apiReader (cache-bypassed) to eliminate race conditions where concurrent
// requests must see each other's CRD creations immediately (GW-DEDUP-002 fix).
func buildCorePipelineComponents(
	cfg *config.ServerConfig,
	logger logr.Logger,
	ctrlClient client.Client,
	apiReader client.Client,
	k8sClient *k8s.Client,
	metricsInstance *metrics.Metrics,
) (*adapters.AdapterRegistry, *k8s.ClientWithCircuitBreaker, *processing.StatusUpdater, *processing.PhaseBasedDeduplicationChecker) {
	adapterRegistry := adapters.NewAdapterRegistry(logger)
	cbClient := k8s.NewClientWithCircuitBreaker(k8sClient, metricsInstance)
	statusUpdater := processing.NewStatusUpdater(ctrlClient, apiReader)
	phaseChecker := processing.NewPhaseBasedDeduplicationChecker(apiReader, cfg.Processing.Deduplication.CooldownPeriod)
	return adapterRegistry, cbClient, statusUpdater, phaseChecker
}

// serverAssemblyInputs bundles the already-constructed dependencies needed to
// assemble a *Server, avoiding an 8+ parameter function signature (Go
// Anti-Pattern Checklist). Extracted from createServerWithClients (funlen).
type serverAssemblyInputs struct {
	cfg             *config.ServerConfig
	logger          logr.Logger
	metricsInstance *metrics.Metrics
	adapterRegistry *adapters.AdapterRegistry
	statusUpdater   *processing.StatusUpdater
	phaseChecker    *processing.PhaseBasedDeduplicationChecker
	lockManager     *processing.DistributedLockManager
	k8sClient       *k8s.Client
	ctrlClient      client.Client
	apiReader       client.Client
	auditStore      audit.AuditStore
	scopeChecker    scope.ScopeChecker
	controllerNS    string
	authenticator   auth.Authenticator
	authorizer      auth.Authorizer
}

// assembleServer builds the *Server struct from its already-constructed
// dependencies and marks the cache ready. Extracted from
// createServerWithClients (funlen).
func assembleServer(in serverAssemblyInputs) *Server {
	authMW := newAuthMiddleware(in.authenticator, in.authorizer, in.controllerNS, in.logger)
	if authMW != nil {
		in.logger.Info("BR-GATEWAY-036/037: Auth middleware enabled",
			"namespace", in.controllerNS,
			"resource", "services/gateway-service",
			"verb", "create")
	}

	server := &Server{
		config:              in.cfg,
		adapterRegistry:     in.adapterRegistry,
		statusUpdater:       in.statusUpdater,
		phaseChecker:        in.phaseChecker,
		lockManager:         in.lockManager,
		k8sClient:           in.k8sClient,
		ctrlClient:          in.ctrlClient,
		apiReader:           in.apiReader, // ADR-057: Used for readiness K8s check (bypasses cache)
		auditStore:          in.auditStore,
		scopeChecker:        in.scopeChecker, // BR-SCOPE-002 + ADR-065: Label-based scope filtering (federated when fleet enabled)
		controllerNamespace: in.controllerNS, // Issue #195: Used by ShouldDeduplicate
		authMiddleware:      authMW,
		metricsInstance:     in.metricsInstance,
		logger:              in.logger,
	}

	// Issue #852: Mark cache as ready. createServerWithClients is only reached after
	// WaitForCacheSync succeeds (production) or with direct K8s client (tests).
	server.cacheReady.Store(true)
	return server
}

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

	// Initialize processing pipeline components (BR-GATEWAY-093/DD-GATEWAY-016
	// circuit breaker, DD-GATEWAY-011 status-based deduplication)
	adapterRegistry, cbClient, statusUpdater, phaseChecker := buildCorePipelineComponents(cfg, logger, ctrlClient, apiReader, k8sClient, metricsInstance)

	// BR-GATEWAY-190: Initialize distributed lock manager for multi-replica safety.
	// Uses apiReader (non-cached client) for immediate consistency — cached client
	// has 5-50ms sync delay → race condition → duplicate locks.
	lockManager := buildLockManager(apiReader, logger)

	// DD-AUDIT-003 / ADR-032 §1.5/§3: Audit store is MANDATORY for Gateway (P0 service).
	auditStore, err := buildAuditStore(cfg, logger)
	if err != nil {
		return nil, err
	}

	// BR-SCOPE-002 / ADR-068: Label-based resource opt-in filtering, federated when fleet enabled.
	scopeCheckerInstance, err := buildScopeChecker(ctrlClient, cfg, logger)
	if err != nil {
		return nil, err
	}

	server := assembleServer(serverAssemblyInputs{
		cfg:             cfg,
		logger:          logger,
		metricsInstance: metricsInstance,
		adapterRegistry: adapterRegistry,
		statusUpdater:   statusUpdater,
		phaseChecker:    phaseChecker,
		lockManager:     lockManager,
		k8sClient:       k8sClient,
		ctrlClient:      ctrlClient,
		apiReader:       apiReader,
		auditStore:      auditStore,
		scopeChecker:    scopeCheckerInstance,
		controllerNS:    controllerNS,
		authenticator:   authenticator,
		authorizer:      authorizer,
	})

	// Create CRD creator with retry observer wired to server audit emission
	// BR-GATEWAY-058: retryAuditObserver emits gateway.crd.failed per retry attempt
	server.crdCreator = processing.NewCRDCreator(cbClient, logger, metricsInstance, &cfg.Processing.Retry, &retryAuditObserver{server: server}, controllerNS)

	server.wireHTTPAndAncillaryServers(cfg, metricsInstance)

	if err := server.configureTLS(cfg); err != nil {
		return nil, err
	}

	return server, nil
}

// wireHTTPAndAncillaryServers sets up the router, middleware chain, and main
// HTTP server, plus the dedicated health/metrics servers. Extracted from
// createServerWithClients (funlen).
func (server *Server) wireHTTPAndAncillaryServers(cfg *config.ServerConfig, metricsInstance *metrics.Metrics) {
	router := server.setupRoutes()
	server.router = router // Store router reference for adapter registration

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

	server.attachHealthAndMetricsServers(cfg, metricsInstance)
}

// wireTestHTTPAndAncillaryServers is wireHTTPAndAncillaryServers's test-only
// counterpart for NewServerForTesting. Extracted from NewServerForTesting
// (funlen). Deliberately omits IdleTimeout (matching this constructor's
// pre-existing httpServer construction) since test callers never rely on it.
func (server *Server) wireTestHTTPAndAncillaryServers(cfg *config.ServerConfig, metricsInstance *metrics.Metrics) {
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

	server.attachHealthAndMetricsServers(cfg, metricsInstance)
}

// attachHealthAndMetricsServers wires up the dedicated health-probe and
// Prometheus metrics HTTP servers (Issue #753: both plain HTTP, never TLS,
// so probes/scrapes never depend on cert rotation). Extracted from
// createServerWithClients (funlen).
func (server *Server) attachHealthAndMetricsServers(cfg *config.ServerConfig, metricsInstance *metrics.Metrics) {
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
}

// configureTLS enables TLS on the main HTTP server when configured,
// wiring up certificate hot-reload. Extracted from createServerWithClients
// (funlen).
func (server *Server) configureTLS(cfg *config.ServerConfig) error {
	if !cfg.Server.TLS.Enabled() {
		return nil
	}

	isTLS, reloader, tlsErr := sharedtls.ConfigureConditionalTLS(server.httpServer, cfg.Server.TLS.CertDir)
	if tlsErr != nil {
		return fmt.Errorf("failed to configure TLS: %w", tlsErr)
	}
	if isTLS {
		server.certReloader = reloader
		server.tlsCertDir = cfg.Server.TLS.CertDir
		server.logger.Info("TLS configured for Gateway server", "certDir", cfg.Server.TLS.CertDir)
	}
	return nil
}
