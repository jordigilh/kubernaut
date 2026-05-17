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

package adapters_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

var _ = Describe("K8sSignalContextResolver — #1175", func() {

	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("UT-KA-1175-SCR-001: maps RR spec fields to SignalContext", func() {
		It("should populate Name, Severity, Namespace, ResourceKind, ResourceName from the RR", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-oom-001",
					Namespace: "kubernaut-system",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "aaaa",
					SignalName:        "OOMKilled",
					Severity:          "critical",
					SignalType:        "alert",
					TargetType:        "kubernetes",
					TargetResource: remediationv1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "api-server",
						Namespace: "production",
					},
					FiringTime:   metav1.Now(),
					ReceivedTime: metav1.Now(),
				},
			}

			cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr).Build()
			resolver := adapters.NewK8sSignalContextResolver(cli, "kubernaut-system")

			sc, err := resolver.ResolveSignalContext(context.Background(), "rr-oom-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(sc).NotTo(BeNil())
			Expect(sc.Name).To(Equal("OOMKilled"))
			Expect(sc.Severity).To(Equal("critical"))
			Expect(sc.ResourceKind).To(Equal("Deployment"))
			Expect(sc.ResourceName).To(Equal("api-server"))
			Expect(sc.Namespace).To(Equal("production"))
			Expect(sc.RemediationID).To(Equal("rr-oom-001"))
		})
	})

	Describe("UT-KA-1175-SCR-002: returns error when RR does not exist", func() {
		It("should return an error for a non-existent RR", func() {
			cli := fake.NewClientBuilder().WithScheme(scheme).Build()
			resolver := adapters.NewK8sSignalContextResolver(cli, "kubernaut-system")

			_, err := resolver.ResolveSignalContext(context.Background(), "rr-nonexistent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("UT-KA-1175-SCR-003: ResolveEnrichmentData returns empty data", func() {
		It("should return non-nil empty EnrichmentData", func() {
			cli := fake.NewClientBuilder().WithScheme(scheme).Build()
			resolver := adapters.NewK8sSignalContextResolver(cli, "kubernaut-system")

			ed, err := resolver.ResolveEnrichmentData(context.Background(), "rr-any")
			Expect(err).NotTo(HaveOccurred())
			Expect(ed).NotTo(BeNil())
		})
	})

	Describe("UT-KA-1175-SCR-004: compile-time interface check", func() {
		It("should satisfy tools.SignalContextResolver", func() {
			var _ tools.SignalContextResolver = &adapters.K8sSignalContextResolver{}
		})
	})
})
