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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

var _ = Describe("WriterClient (BR-FLEET-054)", func() {
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

	Describe("UT-WE-054-001: WriterClient.Create serializes object and calls MCP resources_create_or_update tool", func() {
		It("sends a Job object via resources_create_or_update and records the tool call", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "prod-east")

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-abc123",
					Namespace: "kubernaut-workflows",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{Name: "workflow", Image: "busybox:latest"},
							},
						},
					},
				},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			err = writer.Create(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).ToNot(BeEmpty())

			var found bool
			for _, call := range calls {
				if call.ToolName == "prod-east__resources_create_or_update" {
					found = true
					var args map[string]interface{}
					Expect(json.Unmarshal(call.Arguments, &args)).To(Succeed())
					Expect(args).To(HaveKey("manifest"))
					break
				}
			}
			Expect(found).To(BeTrue(),
				"WriterClient.Create must call {clusterID}__resources_create_or_update")
		})

		It("converts typed objects to unstructured before sending", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("staging"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "staging")

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{Name: "test", Image: "alpine"},
							},
						},
					},
				},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			err = writer.Create(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			var createCall *mockgw.ToolCall
			for i := range calls {
				if calls[i].ToolName == "staging__resources_create_or_update" {
					createCall = &calls[i]
					break
				}
			}
			Expect(createCall).ToNot(BeNil())

			var args map[string]interface{}
			Expect(json.Unmarshal(createCall.Arguments, &args)).To(Succeed())

			manifestStr, ok := args["manifest"].(string)
			Expect(ok).To(BeTrue(), "manifest must be a JSON string")

			var manifest map[string]interface{}
			Expect(json.Unmarshal([]byte(manifestStr), &manifest)).To(Succeed())
			Expect(manifest["kind"]).To(Equal("Job"))
			Expect(manifest["metadata"].(map[string]interface{})["name"]).To(Equal("test-job"))
		})

		It("works with unstructured objects directly", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("dev"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "dev")

			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]interface{}{
						"name":      "unstructured-job",
						"namespace": "test-ns",
					},
				},
			}

			err = writer.Create(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("UT-WE-054-002: WriterClient.Delete calls MCP resources_delete tool", func() {
		It("sends delete request with kind/namespace/name", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-west"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "prod-west")

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-delete-me",
					Namespace: "kubernaut-workflows",
				},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			err = writer.Delete(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			var found bool
			for _, call := range calls {
				if call.ToolName == "prod-west__resources_delete" {
					found = true
					var args map[string]interface{}
					Expect(json.Unmarshal(call.Arguments, &args)).To(Succeed())
					Expect(args["kind"]).To(Equal("Job"))
					Expect(args["name"]).To(Equal("wfe-delete-me"))
					Expect(args["namespace"]).To(Equal("kubernaut-workflows"))
					break
				}
			}
			Expect(found).To(BeTrue(),
				"WriterClient.Delete must call {clusterID}__resources_delete")
		})
	})

	Describe("WriterClient compile-time interface compliance", func() {
		It("satisfies ResourceWriter interface", func() {
			var _ mcpclient.ResourceWriter = (*mcpclient.WriterClient)(nil)
		})
	})

	Describe("WriterClient panics on nil session", func() {
		It("panics when constructed with nil session (fail-fast)", func() {
			Expect(func() {
				mcpclient.NewWriterFromSession(nil, "any-cluster")
			}).To(Panic())
		})
	})
})

// Ensure unused imports don't cause issues.
var _ = runtime.DefaultUnstructuredConverter
