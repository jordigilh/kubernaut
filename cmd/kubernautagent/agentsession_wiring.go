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
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/agentsession"
	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
)

// buildAgentSessionScheme builds the scheme for the AgentSession dispatcher's
// controller-runtime client: the built-in K8s scheme (which already
// registers coordination/v1 Lease) plus AgentSession itself.
func buildAgentSessionScheme() *k8sruntime.Scheme {
	scheme := k8sruntime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(agentsessionv1.AddToScheme(scheme))
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
// dispatch watcher (DD-AA-KA-001, #2170) unconditionally at KA startup, in
// its own background goroutine that runs until ctx is cancelled. Returns
// nil (dispatcher not started) with a logged error when K8s infrastructure
// is unavailable -- this should never happen in a real deployment (KA
// always runs in-cluster), matching buildWorkflowCatalogCache's contract.
func startAgentSessionDispatcher(ctx context.Context, infra *k8sInfra, mgr *session.Manager, runner agentsession.InvestigationRunner, logger logr.Logger) *agentsession.Dispatcher {
	cli, err := buildAgentSessionDispatcherClient(infra)
	if err != nil {
		logger.Error(err, "AgentSession dispatcher: failed to build controller-runtime client -- dispatch disabled, KA cannot receive investigations from AA")
		return nil
	}

	namespace := detectNamespace()
	dispatcher := agentsession.NewDispatcher(cli, namespace, holderIdentity(), mgr, runner, logger)

	// DD-AA-KA-001 Amendment Gap 2: register the status-writer hooks before
	// starting the watch loop, so no dispatch can race hook registration.
	// session.Manager has no CRD awareness; these hooks are the only
	// status-writing path for outcomes reached after dispatch.
	mgr.SetTerminalHook(dispatcher.OnTerminal)
	mgr.SetInteractiveUpgradeHook(dispatcher.OnInteractiveUpgrade)

	go dispatcher.Start(ctx)
	logger.Info("AgentSession dispatcher started",
		"namespace", namespace,
		"holderIdentity", holderIdentity(),
	)
	return dispatcher
}
