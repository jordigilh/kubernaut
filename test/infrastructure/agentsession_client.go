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
	"crypto/sha256"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
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
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register core Kubernetes scheme: %w", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register apps Kubernetes scheme: %w", err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register coordination Kubernetes scheme: %w", err)
	}
	if err := agentsessionv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register AgentSession scheme: %w", err)
	}
	if err := aianalysisv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register AIAnalysis scheme: %w", err)
	}
	if err := remediationv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register RemediationRequest scheme: %w", err)
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
	// KA-focused E2E suites create AgentSession objects directly instead of
	// exercising the production SP -> RO -> AIA path. Keep those behavior tests
	// compatible with the SP-owned fields introduced by #2467 while leaving
	// schema-validation tests (which call k8sClient.Create directly) strict.
	if spec.Environment == "" {
		spec.Environment = "test"
	}
	if spec.Priority == "" {
		spec.Priority = "P3"
	}
	if spec.SignalMode == "" {
		spec.SignalMode = "reactive"
	}
	analysis, err := EnsureAgentSessionParents(ctx, k8sClient, k8sClient.Scheme(), namespace, spec)
	if err != nil {
		return nil, err
	}

	name := "as-" + spec.IncidentID
	as := &agentsessionv1.AgentSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
	if err := controllerutil.SetControllerReference(analysis, as, k8sClient.Scheme()); err != nil {
		return nil, fmt.Errorf("set AIAnalysis owner on AgentSession %s/%s: %w", namespace, name, err)
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

// agentSessionParentDetails contains normalized values used by direct
// AgentSession E2E parent fixtures.
type agentSessionParentDetails struct {
	rrName, targetNamespace, targetKind, targetName string
	severity, signalName, fingerprint               string
}

// EnsureAgentSessionParents creates the authorized RR -> AIAnalysis parent
// chain used by direct AgentSession E2E fixtures. It deliberately leaves the
// production parent gate enabled and returns the AIAnalysis whose UID must own
// the AgentSession.
func EnsureAgentSessionParents(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, namespace string, spec agentsessionv1.AgentSessionSpec) (*aianalysisv1.AIAnalysis, error) {
	return ensureAgentSessionParentsWithPhases(ctx, k8sClient, scheme, namespace, spec, remediationv1.PhaseAnalyzing, aianalysisv1.PhaseInvestigating)
}

// EnsureAgentSessionParentsWaiting creates the same parent chain in the
// documented create-before-phase handoff window. Callers must authorize the RR
// after persisting the child identity.
func EnsureAgentSessionParentsWaiting(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, namespace string, spec agentsessionv1.AgentSessionSpec) (*aianalysisv1.AIAnalysis, error) {
	return ensureAgentSessionParentsWithPhases(ctx, k8sClient, scheme, namespace, spec, remediationv1.PhasePending, aianalysisv1.PhasePending)
}

func ensureAgentSessionParentsWithPhases(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, namespace string, spec agentsessionv1.AgentSessionSpec, parentPhase remediationv1.RemediationPhase, analysisPhase string) (*aianalysisv1.AIAnalysis, error) {
	details, err := newAgentSessionParentDetails(namespace, spec)
	if err != nil {
		return nil, err
	}
	rr, err := ensureAgentSessionRemediationRequest(ctx, k8sClient, namespace, details, parentPhase)
	if err != nil {
		return nil, err
	}
	return ensureAgentSessionAIAnalysis(ctx, k8sClient, scheme, namespace, spec, details, rr, analysisPhase)
}

func newAgentSessionParentDetails(namespace string, spec agentsessionv1.AgentSessionSpec) (agentSessionParentDetails, error) {
	if spec.RemediationRequestRef.Name == "" {
		return agentSessionParentDetails{}, fmt.Errorf("AgentSession %s has an empty RemediationRequestRef.Name", spec.IncidentID)
	}
	targetNamespace := spec.ResourceNamespace
	if targetNamespace == "" {
		targetNamespace = namespace
	}
	targetKind := spec.ResourceKind
	if targetKind == "" {
		targetKind = "Pod"
	}
	targetName := spec.ResourceName
	if targetName == "" {
		targetName = "e2e-test-pod"
	}
	severity := spec.Severity
	if severity == "" {
		severity = "critical"
	}
	signalName := spec.SignalName
	if signalName == "" {
		signalName = "E2ETestSignal"
	}
	return agentSessionParentDetails{
		rrName: spec.RemediationRequestRef.Name, targetNamespace: targetNamespace,
		targetKind: targetKind, targetName: targetName, severity: severity,
		signalName: signalName, fingerprint: fmt.Sprintf("%x", sha256.Sum256([]byte(spec.IncidentID))),
	}, nil
}

func ensureAgentSessionRemediationRequest(ctx context.Context, k8sClient client.Client, namespace string, details agentSessionParentDetails, parentPhase remediationv1.RemediationPhase) (*remediationv1.RemediationRequest, error) {
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{Name: details.rrName, Namespace: namespace},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: details.fingerprint, SignalName: details.signalName,
			Severity: details.severity, SignalType: "alert", TargetType: "kubernetes",
			TargetResource: remediationv1.ResourceIdentifier{Kind: details.targetKind, Name: details.targetName, Namespace: details.targetNamespace},
			FiringTime:     metav1.Now(), ReceivedTime: metav1.Now(),
		},
	}
	if err := k8sClient.Create(ctx, rr); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("create AgentSession parent RemediationRequest %s/%s: %w", namespace, details.rrName, err)
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr); err != nil {
			return nil, fmt.Errorf("read AgentSession parent RemediationRequest %s/%s: %w", namespace, details.rrName, err)
		}
	}
	if rr.Status.OverallPhase == "" {
		rr.Status.OverallPhase = parentPhase
		if err := k8sClient.Status().Update(ctx, rr); err != nil {
			return nil, fmt.Errorf("persist AgentSession parent RemediationRequest %s/%s phase: %w", namespace, details.rrName, err)
		}
	}
	return rr, nil
}

func ensureAgentSessionAIAnalysis(ctx context.Context, k8sClient client.Client, scheme *runtime.Scheme, namespace string, spec agentsessionv1.AgentSessionSpec, details agentSessionParentDetails, rr *remediationv1.RemediationRequest, analysisPhase string) (*aianalysisv1.AIAnalysis, error) {
	remediationID := spec.RemediationID
	if remediationID == "" {
		remediationID = details.rrName
	}
	analysis := &aianalysisv1.AIAnalysis{
		// Direct KA fixtures use IncidentID as the AIA identity, matching the
		// production RequestBuilder's lossless AgentSession mapping.
		ObjectMeta: metav1.ObjectMeta{Name: spec.IncidentID, Namespace: namespace},
		Spec: aianalysisv1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{Name: details.rrName, Namespace: namespace}, RemediationID: remediationID,
			AnalysisRequest: aianalysisv1.AnalysisRequest{
				SignalContext: aianalysisv1.SignalContextInput{
					Fingerprint: details.fingerprint, Severity: details.severity, SignalName: details.signalName,
					SignalMode: spec.SignalMode, Environment: spec.Environment, BusinessPriority: spec.Priority,
					TargetResource:    aianalysisv1.TargetResource{Kind: details.targetKind, Name: details.targetName, Namespace: details.targetNamespace, APIVersion: spec.ResourceAPIVersion},
					EnrichmentResults: sharedtypes.EnrichmentResults{},
				},
				AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
			},
		},
	}
	if err := controllerutil.SetControllerReference(rr, analysis, scheme); err != nil {
		return nil, fmt.Errorf("set RemediationRequest owner on AIAnalysis %s/%s: %w", namespace, analysis.Name, err)
	}
	if err := k8sClient.Create(ctx, analysis); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("create AgentSession parent AIAnalysis %s/%s: %w", namespace, analysis.Name, err)
		}
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis); err != nil {
			return nil, fmt.Errorf("read AgentSession parent AIAnalysis %s/%s: %w", namespace, analysis.Name, err)
		}
		return analysis, nil
	}
	analysis.Status.Phase = analysisPhase
	if err := k8sClient.Status().Update(ctx, analysis); err != nil {
		return nil, fmt.Errorf("persist AgentSession parent AIAnalysis %s/%s phase: %w", namespace, analysis.Name, err)
	}
	return analysis, nil
}
