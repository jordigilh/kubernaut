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

package fullpipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// BR-496 v2: HAPI-Owned Target Resource Identity E2E Tests
//
// Validates that HAPI (not the LLM) owns the target resource identity:
//   - affectedResource is derived from get_namespaced_resource_context's root_owner
//   - TARGET_RESOURCE_* params are injected by HAPI into workflow parameters
//   - The mock LLM does NOT provide affectedResource (proving HAPI injection)
//
// Pipeline: OOMKill → Gateway → RO → SP → AA → HAPI(MockLLM) → WE(Job)
var _ = Describe("HAPI-Owned Target Resource [BR-496]", func() {

	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)

		Expect(workflowUUIDs).To(HaveKey("oomkill-increase-memory-v1:production"))
	})

	AfterEach(func() {
		if testNamespace != "" {
			By("Cleaning up test namespace")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		}
		testCancel()
	})

	// E2E-FP-496-001: CRD affectedResource matches root_owner
	// E2E-FP-496-002: WFE params contain injected TARGET_RESOURCE_*
	// E2E-FP-496-003: Mock LLM response lacks affectedResource (HAPI adds it)
	//
	// All three test IDs are validated in a single pipeline execution to avoid
	// duplicating the 5-10 minute Kind cluster pipeline setup.
	It("should populate affectedResource and TARGET_RESOURCE_* from root_owner, not from LLM [E2E-FP-496-001/002/003]", func() {

		// ================================================================
		// Step 1: Create a managed namespace and deploy memory-eater
		// ================================================================
		By("Step 1: Creating managed test namespace")
		testNamespace = fmt.Sprintf("fp-e2e-496-%d", time.Now().Unix())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
				Labels: map[string]string{
					"kubernaut.ai/managed": "true",
				},
			},
		}
		Expect(k8sClient.Create(testCtx, ns)).To(Succeed())
		GinkgoWriter.Printf("  Namespace created: %s\n", testNamespace)

		By("Step 1b: Deploying memory-eater pod (triggers OOMKill)")
		err := infrastructure.DeployMemoryEater(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Failed to deploy memory-eater")

		// ================================================================
		// Step 2: Wait for OOMKill event
		// ================================================================
		By("Step 2: Waiting for OOMKill event")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := apiReader.List(testCtx, pods, client.InNamespace(testNamespace),
				client.MatchingLabels{"app": "memory-eater"}); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.LastTerminationState.Terminated != nil &&
						cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
						return true
					}
					if cs.State.Terminated != nil &&
						cs.State.Terminated.Reason == "OOMKilled" {
						return true
					}
					if cs.RestartCount > 0 && cs.State.Waiting != nil &&
						cs.State.Waiting.Reason == "CrashLoopBackOff" {
						return true
					}
				}
			}
			return false
		}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "memory-eater should OOMKill")

		// ================================================================
		// Step 3: Wait for RemediationRequest
		// ================================================================
		By("Step 3: Waiting for RemediationRequest from Gateway")
		var remediationRequest *remediationv1.RemediationRequest
		Eventually(func() bool {
			rrList := &remediationv1.RemediationRequestList{}
			if err := apiReader.List(testCtx, rrList, client.InNamespace(namespace)); err != nil {
				return false
			}
		for i := range rrList.Items {
			rr := &rrList.Items[i]
			if rr.Spec.TargetResource.Namespace != testNamespace {
				continue
			}
			sig := strings.ToLower(rr.Spec.SignalName)
			if sig == "backoff" || sig == "oomkilled" || sig == "oomkill" || strings.Contains(sig, "oom") {
				remediationRequest = rr
				GinkgoWriter.Printf("  ✅ RemediationRequest found: %s (signal: %s)\n", rr.Name, rr.Spec.SignalName)
				return true
			}
			GinkgoWriter.Printf("  ⏳ Skipping RR %s with signal %q (waiting for OOMKill/BackOff)\n", rr.Name, rr.Spec.SignalName)
		}
			return false
		}, timeout, interval).Should(BeTrue(), "RemediationRequest should be created by Gateway")

		// ================================================================
		// Step 4: Wait for SignalProcessing
		// ================================================================
		By("Step 4: Waiting for SignalProcessing to complete")
		Eventually(func() string {
			spList := &signalprocessingv1.SignalProcessingList{}
			if err := apiReader.List(testCtx, spList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for _, sp := range spList.Items {
				if sp.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					return string(sp.Status.Phase)
				}
			}
			return ""
		}, timeout, interval).Should(Equal("Completed"),
			"SignalProcessing should reach Completed phase")

		// ================================================================
		// Step 5: Wait for AIAnalysis to complete
		// ================================================================
		By("Step 5: Waiting for AIAnalysis to complete")
		var aaName string
		Eventually(func() string {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(testCtx, aaList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for _, aa := range aaList.Items {
				if aa.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					aaName = aa.Name
					GinkgoWriter.Printf("  AA %s phase: %s\n", aa.Name, aa.Status.Phase)
					return aa.Status.Phase
				}
			}
			return ""
		}, timeout, interval).Should(Equal("Completed"),
			"AIAnalysis should reach Completed phase")

		// ================================================================
		// E2E-FP-496-001: CRD affectedResource matches root_owner
		// ================================================================
		By("[E2E-FP-496-001] Verifying AIAnalysis affectedResource matches K8s root_owner")
		aa := &aianalysisv1.AIAnalysis{}
		Expect(apiReader.Get(testCtx, client.ObjectKey{Name: aaName, Namespace: namespace}, aa)).To(Succeed())

		// E2E-FP-496-003: The mock LLM does NOT include affectedResource in its
		// JSON response (include_affected_resource=False). The presence of this
		// field in the CRD proves HAPI's _inject_target_resource populated it
		// from root_owner (K8s owner chain: Pod → ReplicaSet → Deployment).
		Expect(aa.Status.RootCauseAnalysis).ToNot(BeNil(),
			"AIAnalysis should have rootCauseAnalysis (mock LLM returns RCA)")
		Expect(aa.Status.RootCauseAnalysis.AffectedResource).ToNot(BeNil(),
			"[E2E-FP-496-003] affectedResource must be populated even though mock LLM omits it (HAPI injection)")

		ar := aa.Status.RootCauseAnalysis.AffectedResource

		// The memory-eater is a Deployment. get_namespaced_resource_context resolves the
		// owner chain: Pod → ReplicaSet → Deployment ("memory-eater").
		// HAPI's _inject_target_resource copies root_owner into affectedResource.
		Expect(ar.Kind).To(Equal("Deployment"),
			"[E2E-FP-496-001] affectedResource.kind should be Deployment (root_owner of memory-eater Pod)")
		Expect(ar.Name).To(Equal("memory-eater"),
			"[E2E-FP-496-001] affectedResource.name should be memory-eater (Deployment name)")
		Expect(ar.Namespace).To(Equal(testNamespace),
			"[E2E-FP-496-001] affectedResource.namespace should match the test namespace")

		GinkgoWriter.Printf("  [E2E-FP-496-001] affectedResource: %s/%s/%s (K8s-verified root_owner)\n",
			ar.Kind, ar.Name, ar.Namespace)

		// ================================================================
		// Step 6: Wait for WorkflowExecution to be created
		// ================================================================
		By("Step 6: Waiting for WorkflowExecution to be created")
		var weName string
		Eventually(func() string {
			weList := &workflowexecutionv1.WorkflowExecutionList{}
			if err := apiReader.List(testCtx, weList, client.InNamespace(namespace)); err != nil {
				return ""
			}
			for _, we := range weList.Items {
				if we.Spec.RemediationRequestRef.Name == remediationRequest.Name {
					weName = we.Name
					return we.Status.ExecutionEngine
				}
			}
			return ""
		}, timeout, interval).Should(Equal("job"),
			"WorkflowExecution should use job execution engine")

		// ================================================================
		// E2E-FP-496-002: WFE params contain injected TARGET_RESOURCE_*
		// ================================================================
		By("[E2E-FP-496-002] Verifying WorkflowExecution params contain TARGET_RESOURCE_* from root_owner")
		we := &workflowexecutionv1.WorkflowExecution{}
		Expect(apiReader.Get(testCtx, client.ObjectKey{Name: weName, Namespace: namespace}, we)).To(Succeed())

		params := we.Spec.Parameters
		Expect(params).ToNot(BeNil(),
			"WorkflowExecution should have parameters")

		Expect(params).To(HaveKeyWithValue("TARGET_RESOURCE_NAME", "memory-eater"),
			"[E2E-FP-496-002] TARGET_RESOURCE_NAME must match root_owner name")
		Expect(params).To(HaveKeyWithValue("TARGET_RESOURCE_KIND", "Deployment"),
			"[E2E-FP-496-002] TARGET_RESOURCE_KIND must match root_owner kind")
		Expect(params).To(HaveKeyWithValue("TARGET_RESOURCE_NAMESPACE", testNamespace),
			"[E2E-FP-496-002] TARGET_RESOURCE_NAMESPACE must match root_owner namespace")

		GinkgoWriter.Printf("  [E2E-FP-496-002] WFE params: TARGET_RESOURCE_NAME=%s, KIND=%s, NAMESPACE=%s\n",
			params["TARGET_RESOURCE_NAME"], params["TARGET_RESOURCE_KIND"], params["TARGET_RESOURCE_NAMESPACE"])

		// Cross-validation: WFE params must be consistent with AA affectedResource
		Expect(params["TARGET_RESOURCE_NAME"]).To(Equal(ar.Name),
			"TARGET_RESOURCE_NAME must match affectedResource.name")
		Expect(params["TARGET_RESOURCE_KIND"]).To(Equal(ar.Kind),
			"TARGET_RESOURCE_KIND must match affectedResource.kind")
		Expect(params["TARGET_RESOURCE_NAMESPACE"]).To(Equal(ar.Namespace),
			"TARGET_RESOURCE_NAMESPACE must match affectedResource.namespace")

		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("HAPI-OWNED TARGET RESOURCE TEST COMPLETE [BR-496]")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("  E2E-FP-496-001: affectedResource populated from K8s root_owner")
		GinkgoWriter.Println("  E2E-FP-496-002: TARGET_RESOURCE_* injected into WFE params")
		GinkgoWriter.Println("  E2E-FP-496-003: Mock LLM omits affectedResource, HAPI adds it")
	})
})
