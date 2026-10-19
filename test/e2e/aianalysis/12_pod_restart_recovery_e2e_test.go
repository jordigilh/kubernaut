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

package aianalysis

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-AA-2527-001 exercises the real KA AgentSession dispatch boundary across
// a KA pod restart. The isolated KA owns only the unique test namespace, so
// this spec is intentionally non-Serial and cannot delete the shared KA pod.
//
// Business requirements and controls:
//   - BR-AA-KA-065.3/.4/.9: one immutable AgentSession, Lease-guarded dispatch,
//     and KA-owned status continuity
//   - BR-AUDIT-005 / AU-2 / AU-3 / CC7.2: the terminal investigation trace is
//     reconstructable by the remediation correlation ID without duplicate
//     terminal records
//   - AC-6: the isolated fixture uses namespace-scoped service resources and
//     unique cluster-scoped bindings
var _ = Describe("E2E-AA-2527: AgentSession recovery after KA pod restart", Label("e2e", "aa", "lease-recovery", "aa-2527"), func() {
	It("E2E-AA-2527-001: preserves the immutable AgentSession and completes one recovered attempt without manual Lease deletion", func() {
		testCtx, testCancel := context.WithTimeout(ctx, 4*time.Minute)
		defer testCancel()

		suffix := randomSuffix()
		sharedNamespace := controllerNamespace
		isolatedNamespace := "ka-restart-e2e-" + suffix
		deploymentName := "ka-restart-agent-" + suffix
		serviceName := "ka-restart-svc-" + suffix
		serviceAccountName := "ka-restart-agent-sa-" + suffix
		agentSessionName := "as-ka-restart-" + suffix
		remediationID := "rem-ka-restart-" + suffix
		leaseName := "dispatch-" + agentSessionName

		By("Resolving the already-deployed KA image instead of building a second image")
		sharedDeployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(testCtx, client.ObjectKey{
			Name:      "kubernaut-agent",
			Namespace: sharedNamespace,
		}, sharedDeployment)).To(Succeed(), "the shared E2E KA Deployment must exist")

		var kubernautAgentImage string
		for _, container := range sharedDeployment.Spec.Template.Spec.Containers {
			if container.Name == "kubernaut-agent" {
				kubernautAgentImage = container.Image
				break
			}
		}
		Expect(kubernautAgentImage).NotTo(BeEmpty(), "the shared KA Deployment must expose the deployed image")

		fixture := infrastructure.IsolatedKubernautAgentFixture{
			Namespace:            isolatedNamespace,
			DeploymentName:       deploymentName,
			ServiceName:          serviceName,
			PodSelector:          "app=" + deploymentName,
			DataStorageNamespace: sharedNamespace,
			ServiceAccountName:   serviceAccountName,
		}

		// Register cleanup before deployment so a failure after namespace creation
		// still attempts to remove the namespace and all cluster-scoped bindings.
		DeferCleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cleanupCancel()
			if err := infrastructure.TeardownIsolatedKubernautAgent(cleanupCtx, fixture, kubeconfigPath, GinkgoWriter); err != nil {
				GinkgoWriter.Printf("isolated KA cleanup failed for namespace %s: %v\n", fixture.Namespace, err)
				Expect(err).ToNot(HaveOccurred(), "isolated KA namespace and bindings must be cleaned up")
			}
		})

		By("Deploying an isolated KA with the shared DataStorage, Mock LLM, monitoring, CA, and TLS dependencies")
		deployedFixture, deployErr := infrastructure.DeployIsolatedKubernautAgent(testCtx, infrastructure.IsolatedKubernautAgentConfig{
			Namespace:            isolatedNamespace,
			SharedNamespace:      sharedNamespace,
			DataStorageNamespace: sharedNamespace,
			TLSSourceNamespace:   sharedNamespace,
			CAPath:               infrastructure.InterServiceCAPath(kubeconfigPath),
			DeploymentName:       deploymentName,
			ServiceName:          serviceName,
			ServiceAccountName:   serviceAccountName,
			Image:                kubernautAgentImage,
			DataStorageURL:       "https://data-storage-service." + sharedNamespace + ".svc.cluster.local:8080",
			DataStorageHealthURL: "https://data-storage-service." + sharedNamespace + ".svc.cluster.local:8080/readyz",
			LLMEndpoint:          "http://mock-llm." + sharedNamespace + ".svc.cluster.local:8080",
			ShadowLLMEndpoint:    "http://mock-llm-shadow." + sharedNamespace + ".svc.cluster.local:8080",
			PrometheusURL:        "http://prometheus-svc." + sharedNamespace + ".svc.cluster.local:9090",
			AlertManagerURL:      "http://alertmanager-svc." + sharedNamespace + ".svc.cluster.local:9093",
		}, kubeconfigPath, GinkgoWriter)
		if deployErr == nil {
			fixture = deployedFixture
		}
		Expect(deployErr).ToNot(HaveOccurred(), "isolated KA must become ready before the AgentSession is created")

		// Register this cleanup after the fixture cleanup above. Ginkgo runs
		// DeferCleanup callbacks in reverse order, so the AgentSession is removed
		// while KA still has its Lease/status RBAC before the namespace teardown.
		DeferCleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
			defer cleanupCancel()
			current := &agentsessionv1.AgentSession{}
			err := k8sClient.Get(cleanupCtx, client.ObjectKey{Name: agentSessionName, Namespace: isolatedNamespace}, current)
			if apierrors.IsNotFound(err) {
				return
			}
			Expect(err).ToNot(HaveOccurred(), "read AgentSession during cleanup")
			if err := k8sClient.Delete(cleanupCtx, current); err != nil && !apierrors.IsNotFound(err) {
				Expect(err).ToNot(HaveOccurred(), "delete AgentSession during cleanup")
			}
			Eventually(func() bool {
				remaining := &agentsessionv1.AgentSession{}
				return apierrors.IsNotFound(k8sClient.Get(cleanupCtx, client.ObjectKey{Name: agentSessionName, Namespace: isolatedNamespace}, remaining))
			}, 45*time.Second, 500*time.Millisecond).Should(BeTrue(), "AgentSession must be removed before isolated RBAC is torn down")
		})

		By("Creating one non-terminal AgentSession with the existing deterministic slow Mock LLM scenario")
		agentSession := &agentsessionv1.AgentSession{
			ObjectMeta: metav1.ObjectMeta{
				Name:      agentSessionName,
				Namespace: isolatedNamespace,
			},
			Spec: agentsessionv1.AgentSessionSpec{
				RemediationRequestRef: agentsessionv1.ObjectRef{
					Name:      "rr-" + suffix,
					Namespace: isolatedNamespace,
				},
				IncidentID:        "incident-" + suffix,
				RemediationID:     remediationID,
				SignalName:        "slow-investigation-test",
				Severity:          "warning",
				SignalSource:      "e2e",
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "investigation-target",
				ErrorMessage:      "KA pod restart recovery test",
				Environment:       "test",
				Priority:          "P2",
				RiskTolerance:     "medium",
				BusinessCategory:  "standard",
				SignalMode:        "reactive",
			},
		}
		Expect(k8sClient.Create(testCtx, agentSession)).To(Succeed())

		By("Capturing the original AgentSession identity, immutable Spec, SessionID, and dispatch Lease")
		var initial agentsessionv1.AgentSession
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(agentSession), &initial)).To(Succeed())
			g.Expect(initial.Status.Phase).To(Equal(agentsessionv1.AgentSessionPhaseInvestigating),
				"the isolated KA must acquire the dispatch Lease and start the investigation")
			g.Expect(initial.Status.SessionID).NotTo(BeEmpty(), "KA must publish the first attempt identity")
		}, 90*time.Second, 500*time.Millisecond).Should(Succeed())

		agentSessionUID := initial.UID
		originalSpec := initial.Spec.DeepCopy()
		oldSessionID := initial.Status.SessionID

		var oldPod corev1.Pod
		Eventually(func(g Gomega) {
			pods := &corev1.PodList{}
			g.Expect(k8sClient.List(testCtx, pods, client.InNamespace(fixture.Namespace), client.MatchingLabels{
				"app": fixture.DeploymentName,
			})).To(Succeed())
			g.Expect(pods.Items).To(HaveLen(1), "the isolated Deployment must have exactly one pod")
			oldPod = *pods.Items[0].DeepCopy()
			g.Expect(oldPod.Status.Phase).To(Equal(corev1.PodRunning))
			ready := false
			for _, condition := range oldPod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			g.Expect(ready).To(BeTrue(), "the isolated KA pod must be ready before restart")
		}, 60*time.Second, 500*time.Millisecond).Should(Succeed())

		oldLease := &coordinationv1.Lease{}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(testCtx, client.ObjectKey{Name: leaseName, Namespace: isolatedNamespace}, oldLease)).To(Succeed())
			g.Expect(oldLease.Spec.HolderIdentity).NotTo(BeNil())
		}, 30*time.Second, 500*time.Millisecond).Should(Succeed(),
			"the first attempt must have a dispatch Lease before the pod is restarted")
		oldLease = oldLease.DeepCopy()
		Expect(oldLease.Spec.HolderIdentity).NotTo(BeNil())
		Expect(*oldLease.Spec.HolderIdentity).To(HavePrefix(oldPod.Name+"-"),
			"the dispatch Lease must identify the isolated KA pod epoch that owns the first attempt")

		// The built-in slow-investigation scenario emits a tool call and then
		// blocks its second turn for 30s. Waiting for that first tool-call audit
		// makes the pod deletion deterministic without inventing a production seam.
		By("Waiting until the deterministic slow Mock LLM investigation is in-flight")
		Eventually(func(g Gomega) {
			response, err := dsClient.QueryAuditEvents(testCtx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(remediationID),
				EventType:     dsgen.NewOptString(kaaudit.EventTypeLLMToolCall),
			})
			g.Expect(err).To(Succeed())
			g.Expect(response.Data).NotTo(BeEmpty(), "the slow scenario must issue its first tool call before restart")
		}, 30*time.Second, time.Second).Should(Succeed())

		By("Deleting only the isolated KA pod and waiting for its ready replacement")
		Expect(infrastructure.RestartIsolatedKubernautAgentPod(testCtx, fixture, kubeconfigPath, GinkgoWriter)).To(Succeed())

		var replacementPod corev1.Pod
		Eventually(func(g Gomega) {
			pods := &corev1.PodList{}
			g.Expect(k8sClient.List(testCtx, pods, client.InNamespace(fixture.Namespace), client.MatchingLabels{
				"app": fixture.DeploymentName,
			})).To(Succeed())
			g.Expect(pods.Items).To(HaveLen(1), "the isolated Deployment must still have exactly one pod")
			replacementPod = *pods.Items[0].DeepCopy()
			g.Expect(replacementPod.UID).NotTo(Equal(oldPod.UID), "the Deployment must create a new pod")
			g.Expect(replacementPod.Status.Phase).To(Equal(corev1.PodRunning))
			ready := false
			for _, condition := range replacementPod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			g.Expect(ready).To(BeTrue(), "the replacement isolated KA pod must be ready")
		}, 60*time.Second, 500*time.Millisecond).Should(Succeed())

		// This Get is deliberately observational: the test never deletes or
		// patches the Lease. Recovery must reclaim/reuse it on its own.
		observedLease := &coordinationv1.Lease{}
		Eventually(func(g Gomega) {
			err := k8sClient.Get(testCtx, client.ObjectKey{Name: leaseName, Namespace: isolatedNamespace}, observedLease)
			g.Expect(err).To(Succeed(), "the original dispatch Lease must remain observable after pod deletion")
			g.Expect(observedLease.Spec.HolderIdentity).NotTo(BeNil())
			g.Expect(*observedLease.Spec.HolderIdentity).To(HavePrefix(replacementPod.Name+"-"),
				"the replacement KA epoch must own the recovered dispatch Lease")
			g.Expect(*observedLease.Spec.HolderIdentity).NotTo(Equal(*oldLease.Spec.HolderIdentity),
				"recovery must fence the previous KA epoch")
		}, 60*time.Second, 500*time.Millisecond).Should(Succeed())
		Expect(oldLease.Spec.HolderIdentity).NotTo(BeNil())
		Expect(observedLease.Spec.HolderIdentity).NotTo(BeNil())

		By("Waiting for recovery to publish a new attempt without deleting the dispatch Lease")
		var recoveredSessionID string
		Eventually(func(g Gomega) {
			current := &agentsessionv1.AgentSession{}
			g.Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(agentSession), current)).To(Succeed())
			g.Expect(current.UID).To(Equal(agentSessionUID), "recovery must not replace the AgentSession object")
			g.Expect(current.Spec).To(Equal(*originalSpec), "recovery must not mutate the immutable AgentSession Spec")
			g.Expect(current.Status.SessionID).NotTo(BeEmpty())
			g.Expect(current.Status.SessionID).NotTo(Equal(oldSessionID), "recovery must create a new investigation attempt")
			g.Expect(current.Status.Phase).NotTo(Equal(agentsessionv1.AgentSessionPhaseFailed),
				"pod restart recovery must not surface as a permanent dispatch failure")
			recoveredSessionID = current.Status.SessionID
		}, 90*time.Second, time.Second).Should(Succeed(),
			"the replacement KA must recover the existing non-terminal AgentSession without manual Lease deletion")

		By("Waiting for the recovered attempt to complete and retain its new identity")
		Eventually(func(g Gomega) {
			current := &agentsessionv1.AgentSession{}
			g.Expect(k8sClient.Get(testCtx, client.ObjectKeyFromObject(agentSession), current)).To(Succeed())
			g.Expect(current.UID).To(Equal(agentSessionUID))
			g.Expect(current.Spec).To(Equal(*originalSpec))
			g.Expect(current.Status.SessionID).To(Equal(recoveredSessionID))
			g.Expect(current.Status.Phase).To(Equal(agentsessionv1.AgentSessionPhaseCompleted))
			g.Expect(current.Status.Result).NotTo(BeNil(), "the recovered terminal attempt must publish a result")
		}, 90*time.Second, time.Second).Should(Succeed(),
			"the recovered attempt must reach Completed rather than duplicate or corrupt the original attempt")

		By("Verifying the correlation trace has one terminal response and one RCA completion")
		Eventually(func(g Gomega) {
			events, err := infrastructure.QueryAuditSnapshot(testCtx, dsClient, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(remediationID),
				EventCategory: dsgen.NewOptString(kaaudit.EventCategory),
				Limit:         dsgen.NewOptInt(100),
			})
			g.Expect(err).To(Succeed())
			counts := map[string]int{}
			for _, event := range events {
				g.Expect(event.CorrelationID).To(Equal(remediationID))
				counts[event.EventType]++
			}
			g.Expect(counts[kaaudit.EventTypeResponseComplete]).To(Equal(1),
				"one recovered attempt must produce one terminal response audit event")
			g.Expect(counts[kaaudit.EventTypeRCAComplete]).To(Equal(1),
				"one recovered attempt must produce one RCA completion audit event")
			g.Expect(counts[kaaudit.EventTypeSessionResumed]).To(Equal(1),
				"the audit trace must contain one explicit recovery boundary")
		}, 60*time.Second, 2*time.Second).Should(Succeed(),
			"BR-AUDIT-005/AU-2/AU-3/CC7.2: the recovered correlation trace must not contain duplicate terminal records")
	})
})
