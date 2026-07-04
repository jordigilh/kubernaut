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

package mcpclient_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Gomega DSL convention

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// GVK inference (Issue #1542 follow-up): pkg/fleet/mcpclient historically
// required every caller to call SetGroupVersionKind explicitly before
// Get/List/Create/Update/Delete, unlike the cached controller-runtime client
// which infers GVK from its scheme. That footgun caused the same bug in three
// unrelated packages (WorkflowExecution's Job/Tekton executors, and
// EffectivenessMonitor's cross-cluster health checks). These tests pin the
// fix: typed objects registered in the default scheme (clientgoscheme.Scheme)
// no longer require an explicit SetGroupVersionKind call.
var _ = Describe("GVK inference (Issue #1542 follow-up)", func() {
	var (
		ctx context.Context
		gw  *mockgw.MockGateway
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if gw != nil {
			gw.Close()
		}
	})

	Describe("Client.Get / Client.List", func() {
		It("UT-FLEET-GVK-001: Get succeeds for a typed *corev1.Pod without explicit GVK", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-east"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			pod := &corev1.Pod{}
			err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, pod)
			Expect(err).ToNot(HaveOccurred())
			Expect(pod.Name).To(Equal("nginx"))

			calls := gw.CallLog()
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].ToolName).To(Equal("prod-east__resources_get"))
		})

		It("UT-FLEET-GVK-002: List succeeds for a typed *corev1.PodList without explicit GVK", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-west"))
			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-west"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			podList := &corev1.PodList{}
			err = c.List(ctx, podList)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-FLEET-GVK-003: Get succeeds for a typed *corev1.ConfigMap without explicit GVK", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-east"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			cm := &corev1.ConfigMap{}
			err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "app-config"}, cm)
			Expect(err).ToNot(HaveOccurred())
		})

		It("UT-FLEET-GVK-004: an explicitly-set GVK is preserved and still used verbatim", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			c, err := mcpclient.New(ctx, gw.URL(), mcpclient.WithClusterID("prod-east"))
			Expect(err).ToNot(HaveOccurred())
			defer c.Close()

			pod := &corev1.Pod{}
			pod.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Pod"))
			err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "nginx"}, pod)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("WriterClient.Create / Update / Delete", func() {
		It("UT-FLEET-GVK-005: Create succeeds for a typed *batchv1.Job without explicit GVK", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()
			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "prod-east")

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "wfe-no-gvk", Namespace: "kubernaut-workflows"},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers:    []corev1.Container{{Name: "workflow", Image: "busybox:latest"}},
						},
					},
				},
			}
			// Deliberately no SetGroupVersionKind call (Issue #1542: this is
			// exactly what caused "Object 'Kind' is missing" in production).
			Expect(job.GetObjectKind().GroupVersionKind().Kind).To(BeEmpty())

			err = writer.Create(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			var createCall *mockgw.ToolCall
			for i := range calls {
				if calls[i].ToolName == "prod-east__resources_create_or_update" {
					createCall = &calls[i]
					break
				}
			}
			Expect(createCall).ToNot(BeNil())
			var args map[string]interface{}
			Expect(json.Unmarshal(createCall.Arguments, &args)).To(Succeed())
			var manifest map[string]interface{}
			Expect(json.Unmarshal([]byte(args["resource"].(string)), &manifest)).To(Succeed())
			Expect(manifest["kind"]).To(Equal("Job"),
				"the serialized manifest must carry an inferred kind even though the caller never set one")
			Expect(manifest["apiVersion"]).To(Equal("batch/v1"))
		})

		It("UT-FLEET-GVK-006: Delete succeeds for a typed *batchv1.Job without explicit GVK", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()
			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "prod-east")

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "wfe-delete-no-gvk", Namespace: "kubernaut-workflows"},
			}

			err = writer.Delete(ctx, job)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

