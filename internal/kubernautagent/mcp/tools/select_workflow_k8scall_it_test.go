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
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// k8sCallFakeK8sClient implements enrichment.K8sClient with canned
// responses/errors. Unlike mockEnrichmentRunner (select_workflow_test.go),
// this fake sits one layer lower -- it lets a *real* enrichment.Enricher run
// its actual GetOwnerChain/GetSpecHash/audit logic, so these IT tests prove
// the real production call site (not a stand-in for it).
type k8sCallFakeK8sClient struct {
	ownerChain []enrichment.OwnerChainEntry
	specHash   string
	ownerErr   error
	specErr    error
}

func (f *k8sCallFakeK8sClient) GetOwnerChain(_ context.Context, _, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return f.ownerChain, f.ownerErr
}

func (f *k8sCallFakeK8sClient) GetSpecHash(_ context.Context, _, _, _, _ string) (string, error) {
	return f.specHash, f.specErr
}

// k8sCallStubDSClient is a no-op DataStorageClient -- these tests exercise
// the K8s-call audit path, not remediation history retrieval.
type k8sCallStubDSClient struct{}

func (k8sCallStubDSClient) GetRemediationHistory(_ context.Context, _, _, _, _ string) (*enrichment.RemediationHistoryResult, error) {
	return &enrichment.RemediationHistoryResult{}, nil
}

var _ = Describe("IT-KA-898-K8SCALL: Real enrichment.Enricher wired through select_workflow (BR-INTERACTIVE-003 #3, audit catalog gap follow-up)", func() {
	// These tests close a QE-identified gap: the unit tests in
	// select_workflow_test.go (UT-KA-898-008) exercise emitInteractiveK8sCall
	// against a mockEnrichmentRunner, which proves select_workflow's own
	// logic but not that the *real* enrichment.Enricher's K8s calls actually
	// drive that logic when wired via WithEnrichmentRunner in production
	// (cmd/kubernautagent/main.go). These IT tests wire the real Enricher
	// (with a fake K8sClient/DataStorageClient -- the only two external
	// dependencies per AGENTS.md's mock strategy) to close that gap.

	Describe("IT-KA-898-K8SCALL-001: success path", func() {
		It("should emit EventTypeInteractiveK8sCall (http_status_code=200) driven by the real Enricher's K8s lookup", func() {
			wfID := "wf-real-enricher-success"
			fakeK8s := &k8sCallFakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{{Kind: "ReplicaSet", Name: "web-rs"}},
				specHash:   "sha256:real123",
			}
			realEnricher := enrichment.NewEnricher(fakeK8s, k8sCallStubDSClient{}, audit.NopAuditStore{}, logr.Discard())

			store := &recordingAuditStore{}
			catalog := &mockWorkflowCatalog{workflow: &mcptools.CatalogWorkflow{WorkflowID: wfID}}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-real-enricher-001",
					CorrelationID:   "rr-real-enricher-001",
					ActingUser:      mcpinternal.UserInfo{Username: "alice@example.com"},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}

			tool := mcptools.NewSelectWorkflowTool(catalog, sessions,
				mcptools.WithEnrichmentRunner(realEnricher),
				mcptools.WithSelectWorkflowAuditStore(store),
			)

			output, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-real-enricher-001",
				WorkflowID: wfID,
				Kind:       "Deployment",
				Name:       "api-server",
				Namespace:  "production",
			}, mcpinternal.UserInfo{Username: "alice@example.com"})
			Expect(err).NotTo(HaveOccurred())
			Expect(output.Enrichment).NotTo(BeNil())
			Expect(output.Enrichment.OwnerChain).To(HaveLen(1),
				"the real Enricher must have resolved the owner chain via GetOwnerChain, not a mocked EnrichmentRunner")

			events := store.events
			Expect(events).To(HaveLen(1))
			ev := events[0]
			Expect(ev.EventType).To(Equal(audit.EventTypeInteractiveK8sCall))
			Expect(ev.CorrelationID).To(Equal("rr-real-enricher-001"))
			Expect(ev.SessionID).To(Equal("sess-real-enricher-001"))
			Expect(ev.ActingUser).To(Equal("alice@example.com"))
			Expect(ev.EventOutcome).To(Equal(audit.OutcomeSuccess))
			Expect(ev.Data["resource"]).To(Equal("Deployment"))
			Expect(ev.Data["verb"]).To(Equal("get"))
			Expect(ev.Data["namespace"]).To(Equal("production"))
			Expect(ev.Data["resource_name"]).To(Equal("api-server"))
			Expect(ev.Data["http_status_code"]).To(Equal(200))
		})
	})

	Describe("IT-KA-898-K8SCALL-002: RBAC-forbidden path", func() {
		It("should emit a failure event (http_status_code=403) when the real Enricher's GetSpecHash hits a K8s Forbidden error", func() {
			wfID := "wf-real-enricher-forbidden"
			forbiddenErr := apierrors.NewForbidden(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				"restricted-app",
				fmt.Errorf("RBAC: access denied"),
			)
			fakeK8s := &k8sCallFakeK8sClient{specErr: forbiddenErr}
			realEnricher := enrichment.NewEnricher(fakeK8s, k8sCallStubDSClient{}, audit.NopAuditStore{}, logr.Discard())

			store := &recordingAuditStore{}
			sessions := &mockSessionManager{
				isActive: true,
				getDriverResult: &mcpinternal.InteractiveSession{
					SessionID:       "sess-real-enricher-002",
					CorrelationID:   "rr-real-enricher-002",
					ActingUser:      mcpinternal.UserInfo{Username: "bob@example.com"},
					RCAResult:       &katypes.InvestigationResult{RCASummary: "test rca"},
					DiscoveryResult: discoveryWithWorkflow(wfID),
				},
			}

			tool := mcptools.NewSelectWorkflowTool(nil, sessions,
				mcptools.WithEnrichmentRunner(realEnricher),
				mcptools.WithSelectWorkflowAuditStore(store),
			)

			_, err := tool.Handle(context.Background(), mcptools.SelectWorkflowInput{
				RRID:       "rr-real-enricher-002",
				WorkflowID: wfID,
				Kind:       "Deployment",
				Name:       "restricted-app",
				Namespace:  "restricted-ns",
			}, mcpinternal.UserInfo{Username: "bob@example.com"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("forbidden"),
				"the real Enricher's wrapped ErrRBACForbidden must surface as ErrCodeForbidden through select_workflow")

			events := store.events
			Expect(events).To(HaveLen(1))
			Expect(events[0].EventOutcome).To(Equal(audit.OutcomeFailure))
			Expect(events[0].Data["http_status_code"]).To(Equal(403))
		})
	})
})
