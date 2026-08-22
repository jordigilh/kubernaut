/*
Copyright 2026 Jordi Gil.

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
	"fmt"
	"os"

	"github.com/go-logr/logr"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// buildAgentSessionScheme builds the scheme for the AgentSession dispatcher's
// controller-runtime client: the built-in K8s scheme (which already
// registers coordination/v1 Lease) plus AgentSession and InvestigationSession.
// InvestigationSession is required here because Dispatcher.hasInvestigationSession
// (DD-AA-KA-001 Amendment Gap 1's dispatch-time interactivity check, the sole
// source of truth for Signal.Interactive) Lists InvestigationSessionList
// through this same client -- omitting it here made that List call fail on
// every single dispatch with "no kind is registered for the type
// v1alpha1.InvestigationSessionList", silently defaulting every investigation
// to autonomous dispatch regardless of the caller's actual interactive intent
// (caught via IT-AA integration failures, not a compile error, since the
// dispatcher fails open and only logs the error).
func buildAgentSessionScheme() *k8sruntime.Scheme {
	scheme := k8sruntime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(agentsessionv1.AddToScheme(scheme))
	utilruntime.Must(isv1alpha1.AddToScheme(scheme))
	return scheme
}

// buildAgentSessionDispatcherClient constructs the raw watch-capable
// controller-runtime client used by the AgentSession dispatcher
// (DD-AA-KA-001, #2170). Unconditional -- unlike buildMCPControllerClient,
// this is not gated on interactive.enabled, since every investigation
// (autonomous or interactive) now dispatches through AgentSession.
func buildAgentSessionDispatcherClient(infra *k8sInfra) (ctrlclient.WithWatch, error) {
	if infra == nil {
		return nil, fmt.Errorf("k8s infrastructure unavailable")
	}
	restConfig := *infra.kubeConfig
	// DELIBERATELY no restConfig.Timeout here (unlike buildMCPControllerClient's
	// plain, one-shot ctrlclient.New): rest.Config.Timeout becomes
	// http.Client.Timeout, which client-go enforces over a request's entire
	// lifetime -- including reading a long-lived Watch response body. Setting
	// it here would silently kill this client's Watch connection every time
	// the timeout elapsed (observed: every ~10s), forcing the dispatcher into
	// a permanent reconnect loop that never gets far enough to dispatch any
	// AgentSession. Call sites needing a bounded per-call deadline (resync's
	// List, the dispatch-Lease Create/Get/Update) apply their own
	// context.WithTimeout instead -- see dispatcher.go.
	restConfig.QPS = 20
	restConfig.Burst = 40

	return ctrlclient.NewWithWatch(&restConfig, ctrlclient.Options{Scheme: buildAgentSessionScheme()})
}

// buildAgentSessionManagerScheme builds the minimal scheme for the
// controller-runtime Manager driving the AgentSession dispatch Reconciler
// (#2231 / DD-AA-KA-001 Amendment: Dispatcher Reconciler adoption). Only
// AgentSession itself needs registering here -- the Manager's own cache is
// used exclusively to drive Reconcile dispatch (watch -> workqueue ->
// Reconcile); every actual read/write inside Reconcile still goes through
// Dispatcher's own raw, uncached client (buildAgentSessionDispatcherClient),
// preserving the existing dispatch-Lease race consistency semantics
// unchanged -- see dispatcher.go's Dispatcher doc comment.
func buildAgentSessionManagerScheme() *k8sruntime.Scheme {
	scheme := k8sruntime.NewScheme()
	utilruntime.Must(agentsessionv1.AddToScheme(scheme))
	return scheme
}

// newAgentSessionManager creates the controller-runtime Manager used to run
// the AgentSession dispatch Reconciler, scoped to namespace when non-empty
// (mirrors cmd/apifrontend/session_infra.go's newSessionControllerManager).
// Metrics/health-probe serving and leader election are disabled: KA already
// serves its own /healthz, /readyz, and metrics endpoints independently
// (health.go), and dispatch does not require exactly-one-active-replica --
// the dispatch Lease already arbitrates that per-AgentSession.
func newAgentSessionManager(restCfg *rest.Config, namespace string) (ctrl.Manager, error) {
	opts := ctrl.Options{
		Scheme:                 buildAgentSessionManagerScheme(),
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "",
		LeaderElection:         false,
	}
	if namespace != "" {
		opts.Cache = cache.Options{
			DefaultNamespaces: map[string]cache.Config{namespace: {}},
		}
	}
	mgr, err := ctrl.NewManager(restCfg, opts)
	if err != nil {
		return nil, fmt.Errorf("create agentsession dispatcher manager: %w", err)
	}
	return mgr, nil
}

// holderIdentity returns a stable per-replica identity for the dispatch
// Lease's HolderIdentity field, preferring the pod name (unique per
// replica) and falling back to a PID-based identity outside a real
// deployment (e.g. local dev without a mounted downward-API POD_NAME).
func holderIdentity() string {
	if podName, _ := karbac.DetectPodIdentity(); podName != "" {
		return podName
	}
	return fmt.Sprintf("kubernaut-agent-%d", os.Getpid())
}

// startAgentSessionDispatcher constructs and starts the AgentSession
// dispatch Reconciler (DD-AA-KA-001, #2170; controller-runtime Reconciler
// adoption, #2231 / DD-AA-KA-001 Amendment) unconditionally at KA startup,
// running its own controller-runtime Manager in a background goroutine
// until ctx is cancelled. Returns nil (dispatcher not started) with a
// logged error when K8s infrastructure is unavailable, or when the
// Manager/Reconciler registration fails -- this should never happen in a
// real deployment (KA always runs in-cluster), matching
// buildWorkflowCatalogCache's contract.
func startAgentSessionDispatcher(ctx context.Context, infra *k8sInfra, mgr *session.Manager, runner agentsession.InvestigationRunner, logger logr.Logger) *agentsession.Dispatcher {
	cli, err := buildAgentSessionDispatcherClient(infra)
	if err != nil {
		logger.Error(err, "AgentSession dispatcher: failed to build controller-runtime client -- dispatch disabled, KA cannot receive investigations from AA")
		return nil
	}

	namespace := detectNamespace()
	ctrlMgr, err := newAgentSessionManager(infra.kubeConfig, namespace)
	if err != nil {
		logger.Error(err, "AgentSession dispatcher: failed to build controller-runtime manager -- dispatch disabled, KA cannot receive investigations from AA")
		return nil
	}

	dispatcher := agentsession.NewDispatcher(cli, namespace, holderIdentity(), mgr, runner, logger)

	// DD-AA-KA-001 Amendment Gap 2: register the status-writer hooks before
	// starting the Manager, so no dispatch can race hook registration.
	// session.Manager has no CRD awareness; these hooks are the only
	// status-writing path for outcomes reached after dispatch.
	mgr.SetTerminalHook(dispatcher.OnTerminal)
	mgr.SetInteractiveUpgradeHook(dispatcher.OnInteractiveUpgrade)

	if err := dispatcher.SetupWithManager(ctrlMgr); err != nil {
		logger.Error(err, "AgentSession dispatcher: failed to register reconciler with manager -- dispatch disabled, KA cannot receive investigations from AA")
		return nil
	}

	go func() {
		if startErr := ctrlMgr.Start(ctx); startErr != nil {
			logger.Error(startErr, "AgentSession dispatcher manager exited with error")
		}
	}()
	logger.Info("AgentSession dispatcher started",
		"namespace", namespace,
		"holderIdentity", holderIdentity(),
	)
	return dispatcher
}
