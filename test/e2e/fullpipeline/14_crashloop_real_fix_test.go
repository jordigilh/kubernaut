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

package fullpipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/scenarios"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	crashloopbackoff = "CrashLoopBackOff"
	backoff          = "backoff"
)

// E2E-FP-1542-001: crashloop-config-fix-v1 performs a real, verifiable fix.
// Authority: Issue #1542
//
// A real busybox Deployment reads APP_MODE from a ConfigMap and crashes
// (exit 1) with the bad value, producing a genuine CrashLoopBackOff that the
// kubernetes-event-exporter forwards to Gateway (no ClusterID — single
// cluster). This proves the full pipeline selects crashloop-config-fix-v1
// and that its Job actually patches the ConfigMap and restarts the
// Deployment — not a no-op simulation (see remediate.sh, Issue #1542).
var _ = Describe("E2E-FP-1542-001: CrashLoop config fix performs a real fix (single cluster)", Label("e2e", "fullpipeline", "crashloop"), func() {

	var (
		testNamespace string
		testCtx       context.Context
		testCancel    context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithTimeout(ctx, 10*time.Minute)
		Expect(workflowUUIDs).To(HaveKey("crashloop-config-fix-v1:production"))
	})

	AfterEach(func(specCtx SpecContext) {
		if testNamespace == "" {
			testCancel()
			return
		}
		if specCtx.SpecReport().Failed() {
			// The namespace is deleted below on success, and must-gather
			// only covers fixed namespaces -- so without this dump a wait
			// timeout leaves zero pod-level evidence in the CI log.
			dumpCrashLoopDiagnostics(testNamespace)
			GinkgoWriter.Printf("  ⚠️ spec failed: preserving namespace %s for triage (skipped AfterEach delete)\n", testNamespace)
		} else {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
			_ = k8sClient.Delete(ctx, ns)
		}
		testCancel()
	})

	It("should patch the offending ConfigMap and restart the deployment, recovering the real pod [E2E-FP-1542-001]", func() {
		By("Step 1: Creating managed test namespace (fp-e2e-* prefix required by event-exporter routing)")
		testNamespace = fmt.Sprintf("fp-e2e-crashloop-%d", time.Now().Unix())
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testNamespace,
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		By("Step 2: Deploying crashloop-app (bad ConfigMap, real CrashLoopBackOff)")
		Expect(infrastructure.DeployCrashLoopConfigApp(testCtx, testNamespace, kubeconfigPath, GinkgoWriter)).To(Succeed())

		By("Step 2b: Waiting for the real CrashLoopBackOff...")
		Eventually(func() bool {
			pods := &corev1.PodList{}
			if err := apiReader.List(ctx, pods, client.InNamespace(testNamespace),
				client.MatchingLabels{"app": infrastructure.CrashLoopAppName}); err != nil {
				return false
			}
			for _, pod := range pods.Items {
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.RestartCount > 0 && cs.State.Waiting != nil &&
						cs.State.Waiting.Reason == crashloopbackoff {
						GinkgoWriter.Printf("  ✅ CrashLoopBackOff detected: restarts=%d\n", cs.RestartCount)
						return true
					}
				}
			}
			return false
		}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "crashloop-app should reach CrashLoopBackOff")

		By("Step 3: Waiting for RemediationRequest created by Gateway (real K8s BackOff event)")
		var remediationRequest *remediationv1.RemediationRequest
		Eventually(func() bool {
			rrList := &remediationv1.RemediationRequestList{}
			if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
				return false
			}
			for i := range rrList.Items {
				rr := &rrList.Items[i]
				if rr.Spec.TargetResource.Namespace != testNamespace {
					continue
				}
				sig := strings.ToLower(rr.Spec.SignalName)
				if sig == backoff || strings.Contains(sig, "crashloop") {
					remediationRequest = rr
					GinkgoWriter.Printf("  ✅ RemediationRequest found: %s (signal: %s)\n", rr.Name, rr.Spec.SignalName)
					return true
				}
				GinkgoWriter.Printf("  ⏳ Skipping RR %s with signal %q (waiting for BackOff/CrashLoop)\n", rr.Name, rr.Spec.SignalName)
			}
			return false
		}, timeout, interval).Should(BeTrue(), "RemediationRequest should be created by Gateway")

		By("Steps 4-13: Driving the shared crashloop-config-fix-v1 assertion flow (Issue #1542)")
		scenarios.RunCrashLoopConfigFixScenario(scenarios.CrashLoopConfigFixScenarioConfig{
			Ctx:                    ctx,
			CRDClient:              apiReader,
			TargetClient:           k8sClient,
			JobClient:              k8sClient,
			CRDNamespace:           namespace,
			TargetNamespace:        testNamespace,
			JobNamespace:           infrastructure.ExecutionNamespace,
			RemediationRequestName: remediationRequest.Name,
			ExpectedWorkflowID:     workflowUUIDs["crashloop-config-fix-v1:production"],
			Timeout:                timeout,
			Interval:               interval,
		})
	})
})

// dumpCrashLoopDiagnostics lists the workload objects in a failed spec's test
// namespace and renders them to the Ginkgo log. Called from AfterEach only
// when the spec failed (the namespace is deleted on success and must-gather
// never covers fp-e2e-* namespaces, so this is the only pod-level evidence a
// recurrence leaves behind).
func dumpCrashLoopDiagnostics(testNamespace string) {
	listCtx := context.Background()

	pods := &corev1.PodList{}
	if err := apiReader.List(listCtx, pods, client.InNamespace(testNamespace),
		client.MatchingLabels{"app": infrastructure.CrashLoopAppName}); err != nil {
		GinkgoWriter.Printf("  diagnostics namespace=%s: pod list failed: %v\n", testNamespace, err)
	} else {
		GinkgoWriter.Printf("%s", infrastructure.SummarizePodsForDiagnostics(testNamespace, pods))
	}

	deps := &appsv1.DeploymentList{}
	if err := apiReader.List(listCtx, deps, client.InNamespace(testNamespace)); err != nil {
		GinkgoWriter.Printf("  diagnostics namespace=%s: deployment list failed: %v\n", testNamespace, err)
	}
	for _, d := range deps.Items {
		GinkgoWriter.Printf("  diagnostics deployment=%s replicas=%d available=%d updated=%d\n",
			d.Name, derefInt32CrashLoop(d.Spec.Replicas),
			d.Status.AvailableReplicas, d.Status.UpdatedReplicas)
	}

	rss := &appsv1.ReplicaSetList{}
	if err := apiReader.List(listCtx, rss, client.InNamespace(testNamespace)); err != nil {
		GinkgoWriter.Printf("  diagnostics namespace=%s: replicaset list failed: %v\n", testNamespace, err)
	}
	for _, rs := range rss.Items {
		GinkgoWriter.Printf("  diagnostics replicaset=%s replicas=%d ready=%d available=%d\n",
			rs.Name, derefInt32CrashLoop(rs.Spec.Replicas),
			rs.Status.ReadyReplicas, rs.Status.AvailableReplicas)
	}
}

func derefInt32CrashLoop(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
