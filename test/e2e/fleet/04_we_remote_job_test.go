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

package fleet

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// E2E-FLEET-005: WE dispatches remote Job via MCP gateway
// Authority: Issue #54, ADR-068
// FedRAMP: AC-3 (access enforcement -- remote execution boundary)
//
// This test validates that the MCP gateway exposes the tools needed for
// remote Job creation, and that a tool call routed through the gateway
// successfully creates a resource on the remote cluster.
var _ = Describe("E2E-FLEET-005 [AC-3]: WE dispatches remote Job via MCP gateway to remote cluster (BR-INTEGRATION-054)", Label("fleet"), func() {
	It("should discover job-creation tools via MCP gateway and verify tool availability", func() {
		mcpCtx := context.Background()
		authClient, err := fleetAuthenticatedHTTPClient()
		Expect(err).ToNot(HaveOccurred(), "should acquire Keycloak token for MCP gateway")
		mcpClient, err := mcpclient.New(mcpCtx, mcpGatewayURL, mcpclient.WithHTTPClient(authClient))
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway via NodePort")
		defer mcpClient.Close()

		// Issue #54 flakiness fix: the MCP gateway broker only exposes the
		// generic "discover_tools"/"select_tools" meta-tools until it finishes
		// syncing cluster-specific tools from kube-mcp-server (~60s observed in
		// spike S15 -- see suite_test.go's newFleetMCPClient for the identical
		// retry rationale). Poll instead of asserting on the first response.
		By("Verifying resource creation tools are available for remote execution (AC-3)")
		Eventually(func(g Gomega) {
			tools, listErr := mcpClient.Session().ListTools(mcpCtx, nil)
			g.Expect(listErr).ToNot(HaveOccurred())
			g.Expect(tools.Tools).ToNot(BeEmpty(),
				"MCP gateway should expose K8s MCP Server tools")

			toolNames := make(map[string]bool, len(tools.Tools))
			for _, tool := range tools.Tools {
				toolNames[tool.Name] = true
			}

			g.Expect(toolNames).To(HaveKey("remote-cluster__resources_create_or_update"),
				"AC-3: resources_create_or_update tool must be available for remote Job dispatch")
			g.Expect(toolNames).To(HaveKey("remote-cluster__resources_get"),
				"resources_get tool needed for WE status polling")
		}, 90*time.Second, 5*time.Second).Should(Succeed())
	})

	It("should execute a read operation on the remote cluster via MCP gateway", func() {
		mcpCtx := context.Background()
		mcpClient, err := newFleetMCPClient(mcpCtx)
		Expect(err).ToNot(HaveOccurred(), "should connect to MCP gateway via NodePort")
		defer mcpClient.Close()

		By("Reading a well-known resource (kube-system pods) via MCP gateway")
		podList := &unstructured.UnstructuredList{}
		podList.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "PodList"})
		err = mcpClient.List(mcpCtx, podList, client.InNamespace("kube-system"))
		Expect(err).ToNot(HaveOccurred(),
			"AC-3: MCP gateway must support remote list operations")
		Expect(podList.Items).ToNot(BeEmpty(),
			"remote cluster must have pods in kube-system")
	})

	It("E2E-FLEET-012 [AC-3, AC-4, AC-6, ASVS V4.1.1, V4.1.5]: should classify a deterministic Job collision from the remote cluster", func() {
		Expect(workflowUUIDs).To(HaveKey("crashloop-config-fix-v1:production"))

		const oldOwner = "wfe-remote-collision-owner"
		targetResource := fmt.Sprintf("default/deployment/e2e-remote-iscompleted-%s", uuid.New().String()[:8])
		jobName := weexecutor.ExecutionResourceName(targetResource)
		wfeName := "e2e-remote-iscompleted-" + uuid.New().String()[:8]

		newCollisionJob := func() *batchv1.Job {
			return &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: infrastructure.ExecutionNamespace,
					Labels: map[string]string{
						"kubernaut.ai/workflow-execution": oldOwner,
					},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: ptr.To[int32](0),
					Suspend:      ptr.To(true),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{{
								Name: "workflow", Image: "busybox:1.36", Command: []string{"sleep", "3600"},
							}},
						},
					},
				},
			}
		}

		By("Creating a completed Job on the remote cluster and a non-terminal hub shadow")
		remoteJob := newCollisionJob()
		Expect(remoteK8sClient.Create(ctx, remoteJob)).To(Succeed())
		Expect(retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			latest := &batchv1.Job{}
			if err := remoteK8sClient.Get(ctx, client.ObjectKeyFromObject(remoteJob), latest); err != nil {
				return err
			}
			latest.Status.Conditions = []batchv1.JobCondition{{
				Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
			}}
			return remoteK8sClient.Status().Update(ctx, latest)
		})).To(Succeed())

		hubShadow := newCollisionJob()
		Expect(k8sClient.Create(ctx, hubShadow)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(context.Background(), hubShadow)
			_ = remoteK8sClient.Delete(context.Background(), remoteJob)
		})

		wfe := &workflowexecutionv1alpha1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{Name: wfeName, Namespace: namespace},
			Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
				RemediationRequestRef: corev1.ObjectReference{Name: "rr-" + wfeName, Namespace: namespace},
				WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
						WorkflowID:      workflowUUIDs["crashloop-config-fix-v1:production"],
						WorkflowName:    "crashloop-config-fix-v1",
						ActionType:      "remediate",
						Version:         "v1.0.0",
						ExecutionBundle: "busybox:1.36",
						ExecutionEngine: "job",
					},
				},
				TargetResource: targetResource,
				ClusterID:      "remote-cluster",
			},
		}

		By("Submitting a WorkflowExecution whose deterministic Job already exists remotely")
		Expect(k8sClient.Create(ctx, wfe)).To(Succeed())
		DeferCleanup(func() {
			if err := k8sClient.Delete(context.Background(), wfe); err != nil && !apierrors.IsNotFound(err) {
				GinkgoWriter.Printf("failed to clean up WFE %s: %v\n", wfe.Name, err)
			}
		})

		By("Verifying the production collision path recreates the Job on the remote cluster")
		Eventually(func(g Gomega) {
			var recreated batchv1.Job
			g.Expect(remoteK8sClient.Get(ctx, client.ObjectKey{Name: jobName, Namespace: infrastructure.ExecutionNamespace}, &recreated)).To(Succeed())
			g.Expect(recreated.Labels["kubernaut.ai/workflow-execution"]).To(Equal(wfe.Name),
				"remote IsCompleted must classify the completed remote Job as stale and permit recreation")
		}, timeout, interval).Should(Succeed())
	})
})
