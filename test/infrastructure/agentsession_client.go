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

package infrastructure

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
)

// NewKubeconfigAgentSessionClient builds a throwaway controller-runtime
// client.Client scoped to the AgentSession CRD from a kubeconfig file.
// Mirrors NewKubeconfigWorkflowClient's pattern (see
// workflow_seeding_direct_crd.go) for E2E suites that need to Create/watch
// AgentSession directly against a real cluster.
func NewKubeconfigAgentSessionClient(kubeconfigPath string) (client.Client, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("build rest.Config from kubeconfig %s: %w", kubeconfigPath, err)
	}

	scheme := runtime.NewScheme()
	if err := agentsessionv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register AgentSession scheme: %w", err)
	}

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create controller-runtime client: %w", err)
	}
	return c, nil
}

// InvestigateViaAgentSession replaces the retired
// agentclient.KubernautAgentClient.Investigate(ctx, req) sync wrapper for
// KA-focused E2E tests (issue #2190, DD-AA-KA-001): Create the AgentSession
// directly (playing AA's role, since no AA controller is in the loop for
// these tests) -- poll Status.Phase -- return the curated Result once
// KA reaches a terminal phase. Mirrors
// pkg/aianalysis/creator/agentsession.go's GetOrCreate content-wise, minus
// the AIAnalysis owner reference.
//
// A manual polling loop (not Gomega's Eventually) keeps this helper usable
// from any caller, matching this package's existing convention (e.g.
// ensureAWXPodTypeHealthy) of not depending on the Ginkgo/Gomega test
// framework in test/infrastructure.
func InvestigateViaAgentSession(ctx context.Context, k8sClient client.Client, namespace string, spec agentsessionv1.AgentSessionSpec, timeout time.Duration) (*agentsessionv1.AgentSessionResult, error) {
	name := "as-" + spec.IncidentID
	as := &agentsessionv1.AgentSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
	if err := k8sClient.Create(ctx, as); err != nil {
		return nil, fmt.Errorf("failed to create AgentSession %s/%s: %w", namespace, name, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(as), as); err != nil {
			return nil, fmt.Errorf("failed to poll AgentSession %s/%s: %w", namespace, name, err)
		}
		switch as.Status.Phase {
		case agentsessionv1.AgentSessionPhaseCompleted:
			return as.Status.Result, nil
		case agentsessionv1.AgentSessionPhaseFailed:
			return nil, fmt.Errorf("investigation %s/%s failed: %s (reason=%s)", namespace, name, as.Status.Error, as.Status.Reason)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("timed out after %s waiting for AgentSession %s/%s to reach a terminal phase (last phase: %s)", timeout, namespace, name, as.Status.Phase)
}
