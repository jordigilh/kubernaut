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

package executor_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/pkg/workflowexecution/executor"
)

// UT-WE-054-CF: ClientFactory unit tests
// Authority: BR-FLEET-054 (Fleet Multi-Cluster Execution)
// FedRAMP: AC-3 (Access Enforcement) -- local factory rejects remote clusters
var _ = Describe("UT-WE-054-CF: ClientFactory", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	Context("LocalClientFactory", func() {
		It("UT-WE-054-CF-001: should return local client for empty clusterID", func() {
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewLocalClientFactory(localClient)

			client, err := factory.ClientFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})

		It("UT-WE-054-CF-002: should reject non-empty clusterID when fleet not configured", func() {
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewLocalClientFactory(localClient)

			_, err := factory.ClientFor(ctx, "prod-east")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("remote execution not configured"))
			Expect(err.Error()).To(ContainSubstring("prod-east"))
		})
	})

	Context("MCPClientFactory", func() {
		It("UT-WE-054-CF-003: should return local client for empty clusterID", func() {
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewMCPClientFactory(localClient, nil)

			client, err := factory.ClientFor(ctx, "")
			Expect(err).ToNot(HaveOccurred())
			Expect(client).ToNot(BeNil())
		})

		It("UT-WE-054-CF-004: should panic when session is nil for remote clusterID (fail-fast contract)", func() {
			localClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			factory := executor.NewMCPClientFactory(localClient, nil)

			Expect(func() {
				_, _ = factory.ClientFor(ctx, "prod-west")
			}).To(PanicWith(ContainSubstring("session must not be nil")))
		})
	})
})
