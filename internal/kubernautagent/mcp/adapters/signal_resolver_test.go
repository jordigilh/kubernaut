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
			resolver := adapters.NewSessionSignalContextResolver(provider)

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
			resolver := adapters.NewSessionSignalContextResolver(provider)

			_, err := resolver.ResolveSignalContext(context.Background(), "rr-nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session not found"))
		})
	})

	Describe("UT-KA-1175-SCR-003: ResolveEnrichmentData returns empty data", func() {
		It("should return non-nil empty EnrichmentData", func() {
			provider := &stubSignalProvider{}
			resolver := adapters.NewSessionSignalContextResolver(provider)

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
})
