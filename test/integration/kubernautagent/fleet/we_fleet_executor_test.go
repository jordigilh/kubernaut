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

package fleet_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	weexecutor "github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

// IT-WE-054: Workflow Execution Fleet Executor Integration Tests
//
// Pyramid Invariant: IT proves wiring.
// These tests prove the wiring path:
//   MCPClientFactory.ClientFor(clusterID)
//     -> remoteClient (reader + writer composite)
//       -> WriterClient.Create/Delete -> MCP session -> correct tool names
//
// Wiring Manifest:
//   MCPClientFactory      -> pkg/workflowexecution/executor/client_factory.go -> IT-WE-054-001
//   WriterClient.Create   -> pkg/fleet/mcpclient/writer.go                   -> IT-WE-054-002
//   WriterClient.Delete   -> pkg/fleet/mcpclient/writer.go                   -> IT-WE-054-003
//   localClientFactory    -> pkg/workflowexecution/executor/client_factory.go -> IT-WE-054-004
var _ = Describe("WE Fleet Executor Integration (BR-FLEET-054)", func() {
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

	Describe("IT-WE-054-001: MCPClientFactory routes remote ClusterID through MCP", func() {
		It("returns an ExecutorClient that calls resources_create_or_update via MCP for remote clusters", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("spoke-cluster"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			scheme := runtime.NewScheme()
			Expect(batchv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			factory := weexecutor.NewMCPClientFactory(localClient, parentClient.Session())

			execClient, err := factory.ClientFor(ctx, "spoke-cluster")
			Expect(err).ToNot(HaveOccurred())
			Expect(execClient).ToNot(BeNil())

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-remote-001",
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

			err = execClient.Create(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			Expect(calls).ToNot(BeEmpty())

			var found bool
			for _, call := range calls {
				if call.ToolName == mcpclient.ClusterTool("spoke-cluster", mcpclient.ToolCreateOrUpdate) {
					found = true
					var args map[string]interface{}
					Expect(json.Unmarshal(call.Arguments, &args)).To(Succeed())
					Expect(args).To(HaveKey("manifest"),
						"Create must send manifest argument to resources_create_or_update")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"MCPClientFactory must route Create through spoke-cluster__resources_create_or_update")
		})
	})

	Describe("IT-WE-054-002: MCPClientFactory routes Delete through MCP resources_delete", func() {
		It("sends delete request with correct tool name and arguments", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("spoke-cluster"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			scheme := runtime.NewScheme()
			Expect(batchv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			factory := weexecutor.NewMCPClientFactory(localClient, parentClient.Session())
			execClient, err := factory.ClientFor(ctx, "spoke-cluster")
			Expect(err).ToNot(HaveOccurred())

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wfe-cleanup-001",
					Namespace: "kubernaut-workflows",
				},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			err = execClient.Delete(ctx, job)
			Expect(err).ToNot(HaveOccurred())

			calls := gw.CallLog()
			var found bool
			for _, call := range calls {
				if call.ToolName == mcpclient.ClusterTool("spoke-cluster", mcpclient.ToolDelete) {
					found = true
					var args map[string]interface{}
					Expect(json.Unmarshal(call.Arguments, &args)).To(Succeed())
					Expect(args["kind"]).To(Equal("Job"))
					Expect(args["name"]).To(Equal("wfe-cleanup-001"))
					Expect(args["namespace"]).To(Equal("kubernaut-workflows"))
					break
				}
			}
			Expect(found).To(BeTrue(),
				"MCPClientFactory must route Delete through spoke-cluster__resources_delete")
		})
	})

	Describe("IT-WE-054-003: MCPClientFactory returns local client for empty ClusterID", func() {
		It("does not route through MCP when ClusterID is empty", func() {
			gw = mockgw.NewMockGateway(mockgw.WithMultiCluster("spoke-cluster"))

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			scheme := runtime.NewScheme()
			Expect(batchv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			factory := weexecutor.NewMCPClientFactory(localClient, parentClient.Session())

			execClient, err := factory.ClientFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(execClient).ToNot(BeNil(),
				"empty ClusterID must return the local client, not nil")

			Expect(gw.CallLog()).To(BeEmpty(),
				"local client path must not generate any MCP tool calls")
		})
	})

	Describe("IT-WE-054-004: localClientFactory rejects remote ClusterID", func() {
		It("returns error when fleet is not configured but ClusterID is set", func() {
			scheme := runtime.NewScheme()
			Expect(batchv1.AddToScheme(scheme)).To(Succeed())
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			factory := weexecutor.NewLocalClientFactory(localClient)

			_, err := factory.ClientFor(ctx, "remote-cluster")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("remote execution not configured"),
				"localClientFactory must reject non-empty ClusterID with clear error")
		})
	})
})
