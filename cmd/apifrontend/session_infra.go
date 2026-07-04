package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	authorizationv1 "k8s.io/api/authorization/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/apifrontend"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
	adksession "google.golang.org/adk/session"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/session"
)

// sessionInfra bundles the session-management components that buildSessionInfra
// produces. All fields are safe to use from multiple goroutines once built.
type sessionInfra struct {
	SessionService *session.CRDSessionService
	Reconciler     *controller.SessionCleanupReconciler
	Scheme         *k8sruntime.Scheme
	Healthy        *atomic.Bool
	StopFunc       func()
}

// buildSessionInfra creates the CRDSessionService, registers the
// InvestigationSession scheme, and instantiates the TTL reconciler.
// It creates a real ctrl.Manager, registers field indexes and reconcilers,
// and starts the manager in a goroutine.
func buildSessionInfra(cfg *config.Config, reg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) (*sessionInfra, error) {
	scheme, err := buildSessionScheme()
	if err != nil {
		return nil, err
	}
	registerSessionMetricLabels(reg)

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get kubeconfig: %w", err)
	}
	preflightSessionChecks(restCfg, cfg.Session.Namespace, auditor, logger)

	mgr, err := newSessionControllerManager(restCfg, scheme, cfg.Session.Namespace)
	if err != nil {
		return nil, err
	}

	svc, reconciler, err := registerSessionReconcilers(mgr, scheme, cfg, reg, auditor, logger)
	if err != nil {
		return nil, err
	}

	healthy, stopFunc := startSessionManager(mgr, logger)

	logger.Info("session controller manager started",
		"namespace", cfg.Session.Namespace,
		"disconnectTTL", cfg.Session.DisconnectTTL.String(),
		"retentionTTL", cfg.Session.RetentionTTL.String(),
	)

	return &sessionInfra{
		SessionService: svc,
		Reconciler:     reconciler,
		Scheme:         scheme,
		Healthy:        healthy,
		StopFunc:       stopFunc,
	}, nil
}

// buildSessionScheme constructs the runtime scheme used by the session
// controller manager, registering the coordination (Lease) and
// InvestigationSession API groups.
func buildSessionScheme() (*k8sruntime.Scheme, error) {
	scheme := k8sruntime.NewScheme()
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register coordination scheme: %w", err)
	}
	if err := isv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register InvestigationSession scheme: %w", err)
	}
	return scheme, nil
}

// registerSessionMetricLabels pre-registers the label combinations for the
// session-lifecycle metrics so they report zero (rather than being absent)
// before the first transition of each kind occurs.
func registerSessionMetricLabels(reg *metrics.Registry) {
	for _, phase := range []string{"Active", "Disconnected", "Completed", "Cancelled", "Failed"} {
		reg.SessionsActive.WithLabelValues(phase)
	}
	for _, action := range []string{"cancel", "delete"} {
		reg.SessionTTLActions.WithLabelValues(action)
	}
}

// newSessionControllerManager creates the controller-runtime Manager used to
// run the session cleanup and lease-sync reconcilers, scoped to namespace.
func newSessionControllerManager(restCfg *rest.Config, scheme *k8sruntime.Scheme, namespace string) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				namespace: {},
			},
		},
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "",
		LeaderElection:         false,
	})
	if err != nil {
		return nil, fmt.Errorf("create session controller manager: %w", err)
	}
	if err := session.RegisterFieldIndexes(context.Background(), mgr.GetFieldIndexer()); err != nil {
		return nil, fmt.Errorf("register InvestigationSession field index: %w", err)
	}
	return mgr, nil
}

// registerSessionReconcilers builds the CRDSessionService and registers the
// session-cleanup and lease-sync reconcilers with mgr.
func registerSessionReconcilers(mgr ctrl.Manager, scheme *k8sruntime.Scheme, cfg *config.Config, reg *metrics.Registry, auditor audit.Emitter, logger logr.Logger) (*session.CRDSessionService, *controller.SessionCleanupReconciler, error) {
	k8sClient := mgr.GetClient()

	svc := session.NewCRDSessionService(
		adksession.InMemoryService(),
		k8sClient,
		scheme,
		cfg.Session.Namespace,
		session.WithAuditor(auditor),
		session.WithSessionsActive(reg.SessionsActive),
		session.WithAPIReader(mgr.GetAPIReader()),
		session.WithLogger(logger.WithName("session-service")),
	)

	reconciler := controller.NewSessionCleanupReconciler(
		k8sClient,
		cfg.Session.DisconnectTTL,
		cfg.Session.RetentionTTL,
		logger.WithName("session-cleanup"),
		auditor,
		reg.SessionTTLActions,
		svc,
	)

	leaseSync := controller.NewLeaseSyncReconciler(
		k8sClient,
		cfg.Session.Namespace,
		logger.WithName("lease-sync"),
	)

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return nil, nil, fmt.Errorf("register session reconciler: %w", err)
	}
	if err := leaseSync.SetupWithManager(mgr); err != nil {
		return nil, nil, fmt.Errorf("register lease-sync reconciler: %w", err)
	}
	return svc, reconciler, nil
}

// startSessionManager starts mgr in a background goroutine and tracks its
// cache-sync health in the returned atomic.Bool. The returned stop func
// cancels the manager's context.
func startSessionManager(mgr ctrl.Manager, logger logr.Logger) (*atomic.Bool, func()) {
	healthy := &atomic.Bool{}
	mgrCtx, mgrCancel := context.WithCancel(context.Background()) //nolint:gosec // G118 false positive: mgrCancel is assigned to stopFunc below
	go func() {
		defer healthy.Store(false)
		if startErr := mgr.Start(mgrCtx); startErr != nil {
			logger.Error(startErr, "session controller manager exited with error — health degraded")
		}
	}()
	go func() {
		syncCtx, syncCancel := context.WithTimeout(mgrCtx, 60*time.Second)
		defer syncCancel()
		if mgr.GetCache().WaitForCacheSync(syncCtx) {
			healthy.Store(true)
			logger.Info("session controller cache synced")
		} else {
			logger.Error(nil, "session controller cache sync failed — session health degraded")
		}
	}()
	return healthy, mgrCancel
}

// preflightSessionChecks runs diagnostic checks before starting the session
// controller manager. These are non-blocking so a misconfigured cluster still
// boots the AF; SREs can diagnose from the log output and audit trail.
func preflightSessionChecks(restCfg *rest.Config, namespace string, auditor audit.Emitter, logger logr.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if !checkInvestigationSessionCRD(ctx, restCfg, auditor, logger) {
		return
	}
	checkInvestigationSessionRBAC(ctx, restCfg, namespace, auditor, logger)
}

// checkInvestigationSessionCRD verifies the InvestigationSession CRD is
// registered on the cluster, logging and auditing the result. Returns false
// (skip the RBAC check) only when the discovery client itself could not be
// created — an absent CRD is logged as a warning but does not block startup.
func checkInvestigationSessionCRD(ctx context.Context, restCfg *rest.Config, auditor audit.Emitter, logger logr.Logger) bool {
	gvr := "investigationsessions.kubernaut.ai/v1alpha1"

	dc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		logger.Error(err, "pre-flight: failed to create discovery client")
		return false
	}
	resources, err := dc.ServerResourcesForGroupVersion("kubernaut.ai/v1alpha1")
	crdFound := false
	if err == nil {
		for _, r := range resources.APIResources {
			if r.Name == "investigationsessions" {
				crdFound = true
				break
			}
		}
	}
	logger.Info("pre-flight CRD discovery", "gvr", gvr, "available", crdFound)
	if !crdFound {
		logger.Info("WARNING: InvestigationSession CRD not found — session controller may fail to start")
	}
	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type: audit.EventPreflightCRDCheck,
			Detail: map[string]string{
				"gvr":       gvr,
				"available": fmt.Sprintf("%t", crdFound),
			},
		})
	}
	return true
}

// checkInvestigationSessionRBAC verifies (via SelfSubjectAccessReview) that
// the current ServiceAccount can perform all verbs the session controller
// and CRDSessionService need on investigationsessions (AC-6), logging and
// auditing the result.
func checkInvestigationSessionRBAC(ctx context.Context, restCfg *rest.Config, namespace string, auditor audit.Emitter, logger logr.Logger) {
	k8s, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		logger.Error(err, "pre-flight: failed to create kubernetes client for SSAR")
		return
	}

	requiredVerbs := []string{"get", "list", "watch", "create", "update", "delete"}
	allAllowed := true
	var deniedVerbs []string
	for _, verb := range requiredVerbs {
		allowed, err := checkSSARVerb(ctx, k8s, namespace, verb, logger)
		if err != nil {
			allAllowed = false
			deniedVerbs = append(deniedVerbs, verb+"(error)")
			continue
		}
		if !allowed {
			allAllowed = false
			deniedVerbs = append(deniedVerbs, verb)
		}
	}
	if !allAllowed {
		logger.Info("WARNING: ServiceAccount lacks permissions on investigationsessions — session controller may fail",
			"denied_verbs", strings.Join(deniedVerbs, ","),
		)
	}
	if auditor != nil {
		auditor.Emit(ctx, &audit.Event{
			Type: audit.EventPreflightRBACCheck,
			Detail: map[string]string{
				"resource":     "investigationsessions",
				"namespace":    namespace,
				"all_allowed":  fmt.Sprintf("%t", allAllowed),
				"denied_verbs": strings.Join(deniedVerbs, ","),
			},
		})
	}
}

// checkSSARVerb runs a single SelfSubjectAccessReview for verb against the
// investigationsessions resource, logging the outcome.
func checkSSARVerb(ctx context.Context, k8s kubernetes.Interface, namespace, verb string, logger logr.Logger) (bool, error) {
	ssar := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      verb,
				Group:     "kubernaut.ai",
				Resource:  "investigationsessions",
			},
		},
	}
	result, err := k8s.AuthorizationV1().SelfSubjectAccessReviews().Create(
		ctx, ssar, metav1.CreateOptions{},
	)
	if err != nil {
		logger.Error(err, "pre-flight RBAC check failed", "verb", verb)
		return false, err
	}
	logger.Info("pre-flight RBAC check",
		"verb", verb,
		"resource", "investigationsessions",
		"namespace", namespace,
		"allowed", result.Status.Allowed,
	)
	return result.Status.Allowed, nil
}
