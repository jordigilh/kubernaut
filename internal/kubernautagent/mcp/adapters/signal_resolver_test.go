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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/adapters"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

type stubSignalProvider struct {
	signal *katypes.SignalContext
	err    error
}

func (s *stubSignalProvider) GetSignalForRemediation(_ string) (*katypes.SignalContext, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.signal, nil
}

var _ = Describe("SessionSignalContextResolver", func() {

	Describe("UT-KA-1175-SCR-001: returns full signal context from session", func() {
		It("should populate all fields from the stored AA payload", func() {
			provider := &stubSignalProvider{
				signal: &katypes.SignalContext{
					Name:         "OOMKilled",
					Severity:     "critical",
					Environment:  "production",
					Priority:     "P1",
					ResourceKind: "Deployment",
					ResourceName: "api-server",
					Namespace:    "production",
					RemediationID: "rr-oom-001",
				},
			}
			resolver := adapters.NewSessionSignalContextResolver(provider, nil, "")

			sc, err := resolver.ResolveSignalContext(context.Background(), "rr-oom-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(sc).NotTo(BeNil())
			Expect(sc.Name).To(Equal("OOMKilled"))
			Expect(sc.Severity).To(Equal("critical"))
			Expect(sc.Environment).To(Equal("production"))
			Expect(sc.Priority).To(Equal("P1"))
			Expect(sc.ResourceKind).To(Equal("Deployment"))
			Expect(sc.ResourceName).To(Equal("api-server"))
			Expect(sc.Namespace).To(Equal("production"))
			Expect(sc.RemediationID).To(Equal("rr-oom-001"))
		})
	})

	Describe("UT-KA-1175-SCR-002: returns error when session not found", func() {
		It("should propagate the error from the signal provider", func() {
			provider := &stubSignalProvider{
				err: fmt.Errorf("session not found"),
			}
			resolver := adapters.NewSessionSignalContextResolver(provider, nil, "")

			_, err := resolver.ResolveSignalContext(context.Background(), "rr-nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session not found"))
		})
	})

	Describe("UT-KA-1175-SCR-003: ResolveEnrichmentData returns empty data", func() {
		It("should return non-nil empty EnrichmentData", func() {
			provider := &stubSignalProvider{}
			resolver := adapters.NewSessionSignalContextResolver(provider, nil, "")

			ed, err := resolver.ResolveEnrichmentData(context.Background(), "rr-any")
			Expect(err).NotTo(HaveOccurred())
			Expect(ed).NotTo(BeNil())
		})
	})

	Describe("UT-KA-1175-SCR-004: compile-time interface check", func() {
		It("should satisfy tools.SignalContextResolver", func() {
			var _ tools.SignalContextResolver = &adapters.SessionSignalContextResolver{}
		})
	})

	Describe("UT-KA-1175-SCR-005: CRD fallback when session signal is missing", func() {
		It("should read signal fields from the RR CRD", func() {
			provider := &stubSignalProvider{
				err: fmt.Errorf("session not found"),
			}

			scheme := runtime.NewScheme()
			Expect(remediationv1.AddToScheme(scheme)).To(Succeed())

			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rr-fallback-001",
					Namespace: "test-ns",
				},
				Spec: remediationv1.RemediationRequestSpec{
					SignalFingerprint: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
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

			cli := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				Build()

			resolver := adapters.NewSessionSignalContextResolver(provider, cli, "test-ns")

			sc, err := resolver.ResolveSignalContext(context.Background(), "rr-fallback-001")
			Expect(err).NotTo(HaveOccurred())
			Expect(sc).NotTo(BeNil())
			Expect(sc.Name).To(Equal("OOMKilled"))
			Expect(sc.Severity).To(Equal("critical"))
			Expect(sc.ResourceKind).To(Equal("Deployment"))
			Expect(sc.ResourceName).To(Equal("api-server"))
			Expect(sc.Namespace).To(Equal("production"))
			Expect(sc.RemediationID).To(Equal("rr-fallback-001"))
		})
	})
})
