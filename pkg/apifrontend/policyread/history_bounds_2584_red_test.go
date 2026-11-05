package policyread_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
)

var _ = Describe("BR-SAFETY-001: bounded policy history", func() {
	It("UT-AF-2584-HISTORY-001: uses streaming history reads instead of retaining legacy full lists", func() {
		history := &streamingHistoryStub{}
		resource := inventory.Resource{
			ID:        "r-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			ClusterID: "local",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "payments",
			Name:      "checkout",
			Scope:     inventory.Scope{Included: true},
		}
		service, err := policyread.NewService(policyread.Config{
			Namespace: "kubernaut-system",
		}, policyread.Dependencies{
			LocalInventory: &inventoryStub{page: inventory.Page{State: inventory.StateSuccess, Completeness: inventory.CompletenessComplete, Items: []inventory.Resource{resource}}},
			CurrentPolicies: &currentPolicyStub{policies: []policyread.PolicyMetadata{
				{Type: "aiAnalysis", ConfigMapName: "aianalysis-policy", ConfigMapKey: "approval.rego", Revision: "sha256:ai", Threshold: &policyread.Threshold{Value: 0.8, Source: "runtime-config", Kind: "rego-approval"}},
				{Type: "signalProcessing", ConfigMapName: "signalprocessing-policy", ConfigMapKey: "policy.rego", Revision: "sha256:sp"},
			}},
			History:    history,
			Authorizer: allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := service.ListPolicyScope(context.Background(), &auth.UserIdentity{Username: "alice"}, inventory.Filters{ClusterID: "local"})

		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(HaveLen(1))
		Expect(history.streamCalls).To(Equal(2))
		Expect(history.legacyCalls).To(BeZero())
	})

	It("UT-AF-2584-HISTORY-002: propagates a streaming history failure without returning policy rows", func() {
		history := &streamingHistoryStub{streamErr: errors.New("history page unavailable")}
		service, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &inventoryStub{page: inventory.Page{State: inventory.StateSuccess, Completeness: inventory.CompletenessComplete, Items: []inventory.Resource{{ID: "r-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", ClusterID: "local", Group: "apps", Version: "v1", Kind: "Deployment", Name: "checkout", Scope: inventory.Scope{Included: true}}}}},
			History:        history,
			Authorizer:     allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = service.ListPolicyScope(context.Background(), &auth.UserIdentity{Username: "alice"}, inventory.Filters{ClusterID: "local"})

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
	})
})

type streamingHistoryStub struct {
	legacyCalls int
	streamCalls int
	streamErr   error
}

func (s *streamingHistoryStub) ListAIAnalysis(context.Context) ([]aianalysisv1.AIAnalysis, error) {
	s.legacyCalls++
	return nil, nil
}

func (s *streamingHistoryStub) ListSignalProcessing(context.Context) ([]spv1.SignalProcessing, error) {
	s.legacyCalls++
	return nil, nil
}

func (s *streamingHistoryStub) ForEachAIAnalysis(_ context.Context, visit func(aianalysisv1.AIAnalysis) error) error {
	s.streamCalls++
	if s.streamErr != nil {
		return s.streamErr
	}
	return visit(aianalysisv1.AIAnalysis{})
}

func (s *streamingHistoryStub) ForEachSignalProcessing(_ context.Context, visit func(spv1.SignalProcessing) error) error {
	s.streamCalls++
	if s.streamErr != nil {
		return s.streamErr
	}
	return visit(spv1.SignalProcessing{})
}

var _ policyread.HistorySource = (*streamingHistoryStub)(nil)
var _ policyread.Authorizer = allowAuthorizer{}
