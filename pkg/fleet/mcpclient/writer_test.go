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

	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// goconst dedup: test-fixture literals deduplicated below.
const (
	prodEastResourcesCreateOrUpdate = "prod-east__resources_create_or_update"
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
				if call.ToolName == prodEastResourcesCreateOrUpdate {
					found = true
					var args map[string]interface{}
					Expect(json.Unmarshal(call.Arguments, &args)).To(Succeed())
					Expect(args).To(HaveKey("resource"))
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

			manifestStr, ok := args["resource"].(string)
			Expect(ok).To(BeTrue(), "resource must be a JSON string")

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

		It("sends the manifest under the 'resource' argument key required by the upstream K8s MCP Server", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "prod-east")

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "wfe-arg-key-check", Namespace: "kubernaut-workflows"},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			Expect(writer.Create(ctx, job)).To(Succeed())

			calls := gw.CallLog()
			var found bool
			for _, call := range calls {
				if call.ToolName == prodEastResourcesCreateOrUpdate {
					found = true
					var args map[string]interface{}
					Expect(json.Unmarshal(call.Arguments, &args)).To(Succeed())
					// The upstream K8s MCP Server (github.com/containers/kubernetes-mcp-server)
					// requires the argument key "resource", not "manifest". Sending the wrong
					// key causes the real server to reject the call with
					// "missing argument resource" (isError: true).
					Expect(args).To(HaveKey("resource"))
					Expect(args).ToNot(HaveKey("manifest"))
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("returns a Go error when the MCP server reports isError:true (BR-INTEGRATION-054 regression)", func() {
			// Regression test for a bug where WriterClient.Create silently treated
			// isError:true tool results as success, because it never checked
			// result.IsError before calling populateFromResponse. This caused the
			// WorkflowExecution controller to believe a remote Job was created when
			// the K8s MCP Server had actually rejected the request.
			gw = mockgw.NewMockGateway(mockgw.WithTool(
				mcpclient.ToolCreateOrUpdate,
				"Create or update a Kubernetes resource",
				json.RawMessage(`{"type":"object","properties":{"resource":{"type":"string"}},"required":["resource"]}`),
				func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return &mcp.CallToolResult{
						Content: []mcp.Content{&mcp.TextContent{Text: "failed to create or update resources, missing argument resource"}},
						IsError: true,
					}, nil
				},
			))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			// clusterID is empty so resolveToolName leaves the tool name unprefixed,
			// matching the WithTool registration above.
			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "")

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{Name: "wfe-error-check", Namespace: "kubernaut-workflows"},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			err = writer.Create(ctx, job)
			Expect(err).To(HaveOccurred(),
				"Create must surface isError:true tool results as a Go error instead of silently succeeding")
			Expect(err.Error()).To(ContainSubstring("missing argument resource"))
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

	Describe("UT-WE-054-003 [AC-3]: WriterClient.Update serializes and routes update to correct remote cluster via MCP gateway (BR-INTEGRATION-054)", func() {
		It("sends an updated Job object via resources_create_or_update", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "prod-east")

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-update-me",
					Namespace: "kubernaut-workflows",
					Labels:    map[string]string{"version": "v2"},
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{Name: "workflow", Image: "busybox:v2"},
							},
						},
					},
				},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			err = writer.Update(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			var found bool
			for _, call := range calls {
				if call.ToolName == prodEastResourcesCreateOrUpdate {
					found = true
					var args map[string]interface{}
					Expect(json.Unmarshal(call.Arguments, &args)).To(Succeed())
					Expect(args).To(HaveKey("resource"))

					manifestStr, ok := args["resource"].(string)
					Expect(ok).To(BeTrue(), "resource must be a JSON string")

					var manifest map[string]interface{}
					Expect(json.Unmarshal([]byte(manifestStr), &manifest)).To(Succeed())
					Expect(manifest["metadata"].(map[string]interface{})["name"]).To(Equal("wfe-update-me"))
					Expect(manifest["metadata"].(map[string]interface{})["labels"].(map[string]interface{})["version"]).To(Equal("v2"))
					break
				}
			}
			Expect(found).To(BeTrue(),
				"WriterClient.Update must call {clusterID}__resources_create_or_update")
		})
	})

	Describe("UT-WE-KUA-001: WriterClient.Create uses Kuadrant prefix when WithToolPrefix is set", func() {
		It("emits {prefix}resources_create_or_update instead of {id}__resources_create_or_update", func() {
			gw = mockgw.NewMockGateway(mockgw.WithKuadrantCluster("spoke-c", "spoke_c_"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "spoke-c",
				mcpclient.WithToolPrefix("spoke_c_"))

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kuadrant-job",
					Namespace: "default",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers:    []corev1.Container{{Name: "test", Image: "alpine"}},
						},
					},
				},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			err = writer.Create(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).ToNot(BeEmpty())
			Expect(calls[0].ToolName).To(Equal("spoke_c_resources_create_or_update"),
				"WriterClient.Create must use Kuadrant prefix")
		})
	})

	Describe("UT-WE-KUA-002: WriterClient.Delete uses Kuadrant prefix when WithToolPrefix is set", func() {
		It("emits {prefix}resources_delete instead of {id}__resources_delete", func() {
			gw = mockgw.NewMockGateway(mockgw.WithKuadrantCluster("spoke-d", "spoke_d_"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "spoke-d",
				mcpclient.WithToolPrefix("spoke_d_"))

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "delete-me",
					Namespace: "workflows",
				},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			err = writer.Delete(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).ToNot(BeEmpty())
			Expect(calls[0].ToolName).To(Equal("spoke_d_resources_delete"),
				"WriterClient.Delete must use Kuadrant prefix")
		})
	})

	Describe("UT-WE-KUA-003: WriterClient.Update uses Kuadrant prefix when WithToolPrefix is set", func() {
		It("emits {prefix}resources_create_or_update for Update calls", func() {
			gw = mockgw.NewMockGateway(mockgw.WithKuadrantCluster("spoke-e", "spoke_e_"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "spoke-e",
				mcpclient.WithToolPrefix("spoke_e_"))

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "update-me",
					Namespace: "workflows",
					Labels:    map[string]string{"version": "v2"},
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers:    []corev1.Container{{Name: "test", Image: "alpine:v2"}},
						},
					},
				},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			err = writer.Update(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).ToNot(BeEmpty())
			Expect(calls[0].ToolName).To(Equal("spoke_e_resources_create_or_update"),
				"WriterClient.Update must use Kuadrant prefix")
		})
	})

	Describe("UT-WE-KUA-004: WriterClient without WithToolPrefix falls back to EAIGW convention", func() {
		It("emits {id}__resources_create_or_update when no prefix is set", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("eaigw-cluster"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			writer := mcpclient.NewWriterFromSession(parentClient.Session(), "eaigw-cluster")

			obj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata":   map[string]interface{}{"name": "fallback-job", "namespace": "default"},
				},
			}

			err = writer.Create(ctx, obj)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).ToNot(BeEmpty())
			Expect(calls[0].ToolName).To(Equal("eaigw-cluster__resources_create_or_update"),
				"without WithToolPrefix, must use EAIGW {id}__ convention")
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
