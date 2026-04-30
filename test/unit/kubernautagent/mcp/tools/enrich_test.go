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

package tools_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

type mockEnrichmentRunner struct {
	result *enrichment.EnrichmentResult
	err    error
}

func (m *mockEnrichmentRunner) Enrich(_ context.Context, _, _, _, _, _ string) (*enrichment.EnrichmentResult, error) {
	return m.result, m.err
}

var _ = Describe("kubernaut_enrich tool — #703 BR-INTERACTIVE-003", func() {

	Describe("UT-KA-703-TOOL-003: Input validation", func() {
		It("should reject empty rr_id", func() {
			tool := mcptools.NewEnrichTool(nil, nil)
			_, err := tool.Handle(context.Background(), mcptools.EnrichInput{
				RRID:      "",
				Kind:      "Pod",
				Name:      "api-pod",
				Namespace: "default",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_id"))
		})

		It("should reject empty kind", func() {
			tool := mcptools.NewEnrichTool(nil, nil)
			_, err := tool.Handle(context.Background(), mcptools.EnrichInput{
				RRID:      "rr-001",
				Kind:      "",
				Name:      "api-pod",
				Namespace: "default",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kind"))
		})

		It("should reject empty name", func() {
			tool := mcptools.NewEnrichTool(nil, nil)
			_, err := tool.Handle(context.Background(), mcptools.EnrichInput{
				RRID:      "rr-001",
				Kind:      "Pod",
				Name:      "",
				Namespace: "default",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name"))
		})

		It("should reject empty namespace", func() {
			tool := mcptools.NewEnrichTool(nil, nil)
			_, err := tool.Handle(context.Background(), mcptools.EnrichInput{
				RRID:      "rr-001",
				Kind:      "Pod",
				Name:      "api-pod",
				Namespace: "",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("namespace"))
		})
	})

	Describe("UT-KA-703-TOOL-004: Successful enrichment returns structured result", func() {
		It("should call enricher and return enrichment data", func() {
			mockResult := &enrichment.EnrichmentResult{
				ResourceKind:      "Pod",
				ResourceName:      "api-pod",
				ResourceNamespace: "default",
				OwnerChain: []enrichment.OwnerChainEntry{
					{Kind: "ReplicaSet", Name: "api-rs-abc", Namespace: "default"},
					{Kind: "Deployment", Name: "api-deploy", Namespace: "default"},
				},
			}
			runner := &mockEnrichmentRunner{result: mockResult}
			sessions := &mockSessionManager{isActive: true, getDriverResult: &mcpinternal.InteractiveSession{
				SessionID:     "sess-001",
				CorrelationID: "rr-enrich-001",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}}

			tool := mcptools.NewEnrichTool(runner, sessions)
			output, err := tool.Handle(context.Background(), mcptools.EnrichInput{
				RRID:      "rr-enrich-001",
				Kind:      "Pod",
				Name:      "api-pod",
				Namespace: "default",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Status).To(Equal("enriched"))
			Expect(output.Result).NotTo(BeNil())
			Expect(output.Result.ResourceKind).To(Equal("Pod"))
			Expect(output.Result.OwnerChain).To(HaveLen(2))
		})
	})

	Describe("UT-KA-703-TOOL-004b: Enrichment error returns MCPError", func() {
		It("should return error when enricher fails", func() {
			runner := &mockEnrichmentRunner{err: errors.New("k8s API timeout")}
			sessions := &mockSessionManager{isActive: true, getDriverResult: &mcpinternal.InteractiveSession{
				SessionID:     "sess-002",
				CorrelationID: "rr-enrich-002",
				ActingUser:    mcpinternal.UserInfo{Username: "alice"},
			}}

			tool := mcptools.NewEnrichTool(runner, sessions)
			_, err := tool.Handle(context.Background(), mcptools.EnrichInput{
				RRID:      "rr-enrich-002",
				Kind:      "Pod",
				Name:      "api-pod",
				Namespace: "default",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("enrich"))
		})
	})

	Describe("UT-KA-703-TOOL-004c: Tool rejects requests when no active session", func() {
		It("should return error when no interactive session is active for rr_id", func() {
			runner := &mockEnrichmentRunner{}
			sessions := &mockSessionManager{isActive: false}

			tool := mcptools.NewEnrichTool(runner, sessions)
			_, err := tool.Handle(context.Background(), mcptools.EnrichInput{
				RRID:      "rr-no-session",
				Kind:      "Pod",
				Name:      "api-pod",
				Namespace: "default",
			}, mcpinternal.UserInfo{Username: "alice"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("session"))
		})
	})

	Describe("UT-KA-703-TOOL-004d: Tool enforces driver identity", func() {
		It("should reject requests from a user who is not the active driver", func() {
			runner := &mockEnrichmentRunner{result: &enrichment.EnrichmentResult{}}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:     "sess-003",
					CorrelationID: "rr-authz",
					ActingUser:    mcpinternal.UserInfo{Username: "alice"},
				},
			}

			tool := mcptools.NewEnrichTool(runner, sessions)
			_, err := tool.Handle(context.Background(), mcptools.EnrichInput{
				RRID:      "rr-authz",
				Kind:      "Pod",
				Name:      "api-pod",
				Namespace: "default",
			}, mcpinternal.UserInfo{Username: "bob"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("driver"))
		})
	})
})
