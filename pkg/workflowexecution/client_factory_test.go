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

package workflowexecution_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
	mockgw "github.com/jordigilh/kubernaut/test/services/mock-mcp-gateway/testutil"
)

var _ = Describe("ClientFactory (BR-FLEET-054)", func() {
	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("UT-WE-054-003a: localClientFactory returns the injected client for empty ClusterID", func() {
		It("returns the local client when clusterID is empty", func() {
			scheme := runtime.NewScheme()
			Expect(batchv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewLocalClientFactory(localClient)

			result, err := factory.ClientFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-job",
					Namespace: "default",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers:    []corev1.Container{{Name: "c", Image: "busybox"}},
						},
					},
				},
			}
			Expect(result.Create(ctx, job)).To(Succeed())

			var fetched batchv1.Job
			Expect(result.Get(ctx, client.ObjectKeyFromObject(job), &fetched)).To(Succeed())
			Expect(fetched.Name).To(Equal("test-job"))
		})
	})

	Describe("UT-WE-054-003b: localClientFactory returns error for non-empty ClusterID", func() {
		It("returns an error when clusterID is non-empty", func() {
			scheme := runtime.NewScheme()
			Expect(batchv1.AddToScheme(scheme)).To(Succeed())

			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewLocalClientFactory(localClient)

			_, err := factory.ClientFor(ctx, "prod-east")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("remote"))
		})
	})

	Describe("UT-WE-054-003c: mcpClientFactory returns remote client for non-empty ClusterID", func() {
		It("returns a composite read/write client for a remote cluster", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("prod-east"))
			defer gw.Close()

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			scheme := runtime.NewScheme()
			Expect(batchv1.AddToScheme(scheme)).To(Succeed())
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			factory := executor.NewMCPClientFactory(localClient, parentClient.Session())

			result, err := factory.ClientFor(ctx, "prod-east")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "remote-job",
					Namespace: "kubernaut-workflows",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers:    []corev1.Container{{Name: "c", Image: "busybox"}},
						},
					},
				},
			}
			job.SetGroupVersionKind(batchv1.SchemeGroupVersion.WithKind("Job"))

			Expect(result.Create(ctx, job)).To(Succeed())

			calls := gw.CallLog()
			var found bool
			for _, call := range calls {
				if call.ToolName == "prod-east__create_resource" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(),
				"mcpClientFactory must route Create calls through MCP gateway")
		})
	})

	Describe("UT-WE-054-003d: mcpClientFactory returns local client for empty ClusterID", func() {
		It("falls back to local client when clusterID is empty", func() {
			gw := mockgw.NewMockGateway(mockgw.WithMultiCluster("any"))
			defer gw.Close()

			parentClient, err := mcpclient.New(ctx, gw.URL())
			Expect(err).ToNot(HaveOccurred())
			defer parentClient.Close()

			scheme := runtime.NewScheme()
			Expect(batchv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()

			factory := executor.NewMCPClientFactory(localClient, parentClient.Session())

			result, err := factory.ClientFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-job",
					Namespace: "default",
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers:    []corev1.Container{{Name: "c", Image: "busybox"}},
						},
					},
				},
			}
			Expect(result.Create(ctx, job)).To(Succeed())

			var fetched batchv1.Job
			Expect(result.Get(ctx, client.ObjectKeyFromObject(job), &fetched)).To(Succeed())
			Expect(fetched.Name).To(Equal("local-job"),
				"empty ClusterID must use local K8s client, not MCP")
		})
	})
})
