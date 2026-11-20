package policyread_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This file closes the three IT-AF-2584-006 facets that TP-2584 v1.9
// declared unproven (§5.5, #2667): a runtime-policy read that hits its
// context deadline, a referenced policy ConfigMap that is absent even though
// the reference field is valid, and a history join that fails mid-stream
// after records were already visited.
//
// Each spec drives the production KubernetesSource and the production
// Service, not a stub that merely returns a pre-baked error: the failing
// behavior is the real source read path, with only the Kubernetes API itself
// replaced by a fixture (an external dependency).

var _ = Describe("BR-AI-085 / BR-SCOPE-001: declared P0 source-state evidence (#2667)", func() {
	It("IT-AF-2584-006 [BR-AI-085, FedRAMP SI-10]: returns a state-only unavailable page when the runtime policy read hits its context deadline", func() {
		reader := &deadlineReader{}
		source, err := policyread.NewKubernetesSource(reader)
		Expect(err).NotTo(HaveOccurred())

		svc, err := policyread.NewService(policyread.Config{Namespace: "kubernaut-system"}, policyread.Dependencies{
			LocalInventory:  &inventoryStub{page: policyScopePage()},
			CurrentPolicies: source,
			Authorizer:      allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		// The deadline belongs to the caller, so it is strictly shorter than
		// the service's own DefaultInventoryOperationTimeout. A read that
		// ignored the context would never return.
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		type outcome struct {
			result policyread.PolicyScopePage
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			result, readErr := svc.ListPolicyScope(ctx, user("alice", false), inventory.Filters{ClusterID: "local"})
			done <- outcome{result: result, err: readErr}
		}()

		var got outcome
		select {
		case got = <-done:
		case <-time.After(5 * time.Second):
			Fail("BR-AI-085: the runtime policy read did not observe the caller's deadline")
		}

		Expect(got.err).To(HaveOccurred())
		Expect(errors.Is(got.err, context.DeadlineExceeded)).To(BeTrue(),
			"the deadline must surface from the source read, not be swallowed or retried")
		Expect(policyread.StateOf(got.err)).To(Equal(inventory.StateUnavailable))

		Expect(got.result.State).To(Equal(inventory.StateUnavailable))
		Expect(got.result.Items).To(BeEmpty(), "a timed-out read must not return inventory rows")
		Expect(got.result.NextCursor).To(BeEmpty(), "a timed-out read must not return a cursor")

		encoded, marshalErr := json.Marshal(got.result)
		Expect(marshalErr).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring(`"items"`))
		Expect(string(encoded)).NotTo(ContainSubstring(`"nextCursor"`))
	})

	It("IT-AF-2584-006 [BR-AI-085, FedRAMP SI-10]: fails closed when the referenced policy ConfigMap is absent although the reference field is valid", func() {
		source := newRuntimePolicySource(absentReferencedPolicyConfigMaps()...)
		history := &streamingHistoryStub{}

		svc, err := policyread.NewService(policyread.Config{Namespace: "kubernaut-system"}, policyread.Dependencies{
			LocalInventory:  &inventoryStub{page: policyScopePage()},
			CurrentPolicies: source,
			History:         history,
			Authorizer:      allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := svc.ListPolicyScope(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"})

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		// The reference field is valid and parsed; the failure is the absent
		// object, which is exactly the facet UT-AF-2584-002 does not cover.
		Expect(err.Error()).To(ContainSubstring("read AIAnalysis policy metadata"))
		Expect(err.Error()).To(ContainSubstring(`"custom-ai-policy" not found`))

		Expect(result.State).To(Equal(inventory.StateUnavailable))
		Expect(result.Items).To(BeEmpty(), "an absent policy source must not return inventory rows")
		Expect(result.NextCursor).To(BeEmpty())
		Expect(history.streamCalls).To(BeZero(),
			"history must not be joined after the current policy source fails")

		encoded, marshalErr := json.Marshal(result)
		Expect(marshalErr).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring(`"items"`))
		Expect(string(encoded)).NotTo(ContainSubstring("custom-ai-policy"),
			"a failed policy read must not disclose the referenced ConfigMap identity")
	})

	It("IT-AF-2584-006 [BR-AI-085, FedRAMP SI-10]: returns a state-only failure when the history traversal fails after visiting records", func() {
		historyReader := &pagedHistoryReader{items: []aianalysisv1.AIAnalysis{policyScopeAIAnalysis()}}
		historySource, err := policyread.NewKubernetesSource(historyReader)
		Expect(err).NotTo(HaveOccurred())
		trackedHistory := &countingHistorySource{inner: historySource}

		svc, err := policyread.NewService(policyread.Config{Namespace: "kubernaut-system"}, policyread.Dependencies{
			LocalInventory: &inventoryStub{page: policyScopePage()},
			CurrentPolicies: &currentPolicyStub{policies: []policyread.PolicyMetadata{{
				Type: "aiAnalysis", ConfigMapName: "custom-ai-policy", ConfigMapKey: "approval.rego",
				Revision: "sha256:ai", Threshold: &policyread.Threshold{Value: 0.85, Source: "runtime-config", Kind: "rego-approval"},
			}}},
			History:    trackedHistory,
			Authorizer: allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := svc.ListPolicyScope(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"})

		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(historyReader.calls).To(Equal(2),
			"the first page must have been visited before the continuation failed")
		Expect(trackedHistory.visited).To(Equal(1),
			"the failure is mid-stream: one matching record was already visited")
		Expect(result.State).To(Equal(inventory.StateUnavailable))
		Expect(result.Items).To(BeEmpty(),
			"an incomplete join must not return rows whose history is partial")
		Expect(result.NextCursor).To(BeEmpty())

		encoded, marshalErr := json.Marshal(result)
		Expect(marshalErr).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring(`"items"`))
		Expect(string(encoded)).NotTo(ContainSubstring(`"historicalEvaluations"`))
	})
})

// policyScopePage is a complete one-resource local page, so a source failure
// has real rows and a real join available to withhold.
func policyScopePage() inventory.Page {
	return inventory.Page{
		State:        inventory.StateSuccess,
		Completeness: inventory.CompletenessComplete,
		Items: []inventory.Resource{{
			ID:        policyReadTestResourceID,
			ClusterID: "local",
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "payments",
			Name:      "checkout",
			Scope:     inventory.Scope{Included: true},
		}},
	}
}

// policyScopeAIAnalysis targets the same identity as policyScopePage, so a
// completed traversal would have produced a joined historical evaluation.
func policyScopeAIAnalysis() aianalysisv1.AIAnalysis {
	return aianalysisv1.AIAnalysis{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "ai-checkout",
			Namespace:         "kubernaut-system",
			CreationTimestamp: metav1.Time{Time: time.Date(2026, 11, 1, 19, 59, 0, 0, time.UTC)},
		},
		Spec: aianalysisv1.AIAnalysisSpec{
			AnalysisRequest: aianalysisv1.AnalysisRequest{SignalContext: aianalysisv1.SignalContextInput{
				TargetResource: aianalysisv1.TargetResource{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "payments", Name: "checkout"},
			}},
		},
	}
}

// absentReferencedPolicyConfigMaps is the operator-rendered runtime shape with
// a syntactically valid reference whose ConfigMap was never created.
func absentReferencedPolicyConfigMaps() []client.Object {
	return []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "aianalysis-config", Namespace: "kubernaut-system"},
			Data: map[string]string{"config.yaml": "rego:\n" +
				"  policyConfigMapName: custom-ai-policy\n" +
				"  policyConfigMapKey: approval.rego\n" +
				"  confidenceThreshold: \"0.85\"\n"},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "signalprocessing-config", Namespace: "kubernaut-system"},
			Data: map[string]string{"config.yaml": "classifier:\n" +
				"  regoConfigMapName: custom-signal-policy\n" +
				"  regoConfigMapKey: policy.rego\n"},
		},
	}
}

// deadlineReader blocks until the caller's context ends, so only a source
// read that honors the context deadline can return at all.
type deadlineReader struct{}

func (r *deadlineReader) Get(ctx context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	<-ctx.Done()
	return ctx.Err()
}

func (r *deadlineReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return errors.New("unexpected list")
}

// pagedHistoryReader serves one AIAnalysis page with a continuation and then
// fails, reproducing a traversal that breaks mid-stream rather than on its
// first page.
type pagedHistoryReader struct {
	calls int
	items []aianalysisv1.AIAnalysis
}

func (r *pagedHistoryReader) List(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
	r.calls++
	history, ok := list.(*aianalysisv1.AIAnalysisList)
	if !ok {
		return fmt.Errorf("unexpected list type %T", list)
	}
	if r.calls == 1 {
		history.Items = r.items
		history.Continue = "history-page-2"
		return nil
	}
	return errors.New("AIAnalysis history continuation unavailable")
}

func (r *pagedHistoryReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("unexpected get")
}

// countingHistorySource wraps the production source so the spec can prove a
// record was actually visited before the traversal broke, not merely listed.
type countingHistorySource struct {
	inner   *policyread.KubernetesSource
	visited int
}

func (c *countingHistorySource) ListAIAnalysis(ctx context.Context) ([]aianalysisv1.AIAnalysis, error) {
	return c.inner.ListAIAnalysis(ctx)
}

func (c *countingHistorySource) ForEachAIAnalysis(ctx context.Context, visit func(aianalysisv1.AIAnalysis) error) error {
	return c.inner.ForEachAIAnalysis(ctx, func(record aianalysisv1.AIAnalysis) error {
		c.visited++
		return visit(record)
	})
}

var _ client.Reader = (*deadlineReader)(nil)

var _ client.Reader = (*pagedHistoryReader)(nil)
