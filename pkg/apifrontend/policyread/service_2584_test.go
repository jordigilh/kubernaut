package policyread_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/fleet/acm"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var policyReadTestResourceID = inventory.CanonicalResourceID(scope.ResourceIdentity{ClusterID: "local", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments", Name: "checkout"})

type policyReadAuditRecorder struct {
	events []*audit.Event
}

func (r *policyReadAuditRecorder) Emit(_ context.Context, event *audit.Event) {
	r.events = append(r.events, event)
}

func TestPolicyRead(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Policy Read Suite")
}

var _ = Describe("BR-AI-085 / BR-AI-088 / UT-AF-2584-002/006/009: policy scope projection", func() {
	It("UT-AF-2593-PUBLIC-METADATA-001 [BR-API-001]: accepts an empty result without evaluating a freshness marker", func() {
		page := inventory.Page{State: inventory.StateEmpty, Completeness: inventory.CompletenessComplete, Freshness: inventory.Freshness{State: inventory.FreshnessStale}}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{LocalInventory: &inventoryStub{page: page}, Authorizer: allowAuthorizer{}})
		Expect(err).NotTo(HaveOccurred())

		result, err := svc.ListInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.State).To(Equal(inventory.StateEmpty))
		Expect(result.Items).To(BeEmpty())
	})
	It("UT-AF-2584-POLICY-EMPTY-001 [BR-AI-085]: fails closed when policy metadata is unavailable for an empty inventory", func() {
		page := inventory.Page{State: inventory.StateEmpty, Completeness: inventory.CompletenessComplete}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{LocalInventory: &inventoryStub{page: page}, Authorizer: allowAuthorizer{}})
		Expect(err).NotTo(HaveOccurred())

		result, err := svc.ListPolicyScope(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"})
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(result.State).To(Equal(inventory.StateUnavailable))
	})
	It("IT-AF-2592-EMPTY-001 [BR-SECURITY-016]: reports unavailable when the production ACM scope check cannot verify an exact target", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, err := io.WriteString(w, `{"data":{"searchResult":[{"count":0,"items":[]}]}}`); err != nil {
				GinkgoWriter.Printf("write Search response: %v\n", err)
			}
		}))
		DeferCleanup(server.Close)
		checker, err := fleet.NewScopeChecker(&recordingScopeChecker{}, fleet.FleetConfig{Enabled: true, Backend: fleet.BackendACM, Endpoint: server.URL}, logr.Discard())
		Expect(err).NotTo(HaveOccurred())
		resource := inventory.Resource{ID: "opaque-id", ClusterID: "prod-east", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout", Scope: inventory.Scope{Included: true}}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{RemoteInventory: &inventoryStub{resource: resource}, Authorizer: allowAuthorizer{}, ScopeChecker: checker})
		Expect(err).NotTo(HaveOccurred())

		result, err := svc.GetInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"}, "opaque-id")
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(result).To(Equal(inventory.Resource{}))
	})
	It("IT-AF-2584-REMOTE-LIST-003 [BR-SCOPE-001]: rejects remote inventory listing before invoking ACM Search", func() {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			_, err := io.WriteString(w, `{"data":{"searchResult":[{"count":2,"items":[{"cluster":"prod-east","apigroup":"apps","apiversion":"v1","kind":"Deployment","namespace":"payments","name":"checkout","label.kubernaut.ai/managed":"true"}]}]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		local := &inventoryStub{page: inventory.Page{State: inventory.StateSuccess, Completeness: inventory.CompletenessComplete}}
		auditor := &policyReadAuditRecorder{}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{LocalInventory: local, RemoteInventory: acm.NewClient(server.URL), Authorizer: allowAuthorizer{}, Auditor: auditor})
		Expect(err).NotTo(HaveOccurred())
		page, err := svc.ListInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"})
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(page.Items).To(BeEmpty())
		Expect(page.NextCursor).To(BeEmpty())
		Expect(local.listCalls).To(BeZero())
		Expect(requests.Load()).To(BeZero())
		Expect(auditor.events).To(HaveLen(1))
		Expect(auditor.events[0].Type).To(Equal(audit.EventConsoleManagedResourceListRead))
		Expect(auditor.events[0].UserID).To(Equal("alice"))
		Expect(auditor.events[0].ClusterID).To(Equal("prod-east"))
		Expect(auditor.events[0].Detail["result_state"]).To(Equal("unavailable"))
		Expect(auditor.events[0].Detail).NotTo(HaveKey("name"))
	})
	It("IT-AF-2584-008: scopes ACM exact reads to the requested cluster and checks managed scope", func() {
		var requestBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			requestBody, err = io.ReadAll(r.Body)
			Expect(err).NotTo(HaveOccurred())
			if strings.Contains(string(requestBody), `"property":"kind","values":["Namespace"]`) {
				response := `{"data":{"searchResult":[{"count":0,"items":[]}]}}`
				if strings.Contains(string(requestBody), `"property":"name","values":["payments"]`) {
					response = `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","apiversion":"v1","kind":"Namespace","name":"payments"}]}]}}`
				}
				_, err = io.WriteString(w, response)
				Expect(err).NotTo(HaveOccurred())
				return
			}
			_, err = io.WriteString(w, `{"data":{"searchResult":[{"count":1,"items":[{"cluster":"prod-east","_uid":"prod-east/12345","apigroup":"apps","apiversion":"v1","kind":"Deployment","namespace":"payments","name":"checkout","label":"kubernaut.ai/managed=true"}]}]}}`)
			Expect(err).NotTo(HaveOccurred())
		}))
		DeferCleanup(server.Close)
		checker := &recordingScopeChecker{}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			RemoteInventory: acm.NewClient(server.URL), Authorizer: allowAuthorizer{}, ScopeChecker: checker,
		})
		Expect(err).NotTo(HaveOccurred())
		resourceID := inventory.CanonicalResourceID(scope.ResourceIdentity{ClusterID: "prod-east", Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "payments", Name: "checkout"})
		resource, err := svc.GetInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"}, resourceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(resource.ClusterID).To(Equal("prod-east"))
		Expect(string(requestBody)).To(ContainSubstring(`"property":"cluster","values":["prod-east"]`))
		Expect(checker.identity).To(Equal(resource.Identity()))
	})

	It("UT-AF-2584-SCOPE-001 [BR-SECURITY-016]: maps a remote scope failure to unavailable", func() {
		resource := inventory.Resource{ID: "opaque-id", ClusterID: "prod-east", Version: "v1", Kind: "ConfigMap", Namespace: "payments", Name: "checkout", Scope: inventory.Scope{Included: true}}
		checker := &recordingScopeChecker{err: errors.New("scope backend unavailable")}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{RemoteInventory: &inventoryStub{resource: resource}, Authorizer: allowAuthorizer{}, ScopeChecker: checker})
		Expect(err).NotTo(HaveOccurred())
		_, err = svc.GetInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "prod-east"}, "opaque-id")
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
	})
	It("projects only sanitized current policy and exact historical metadata", func() {
		observedAt := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)
		resource := inventory.Resource{
			ID:         policyReadTestResourceID,
			ClusterID:  "local",
			Group:      "apps",
			Version:    "v1",
			Kind:       "Deployment",
			Namespace:  "payments",
			Name:       "checkout",
			Scope:      inventory.Scope{Included: true, Source: inventory.ScopeSourceResourceLabel, ResourceLabel: inventory.LabelTrue, NamespaceLabel: inventory.LabelTrue},
			ObservedAt: observedAt,
		}
		page := inventory.Page{
			State:        inventory.StateSuccess,
			ObservedAt:   observedAt,
			Freshness:    inventory.Freshness{State: inventory.FreshnessFresh, AsOf: observedAt},
			Completeness: inventory.CompletenessComplete,
			Items:        []inventory.Resource{resource},
		}
		ai := aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{Name: "ai-checkout", CreationTimestamp: metav1.Time{Time: observedAt.Add(-time.Minute)}},
			Spec: aianalysisv1.AIAnalysisSpec{
				ClusterID: "",
				AnalysisRequest: aianalysisv1.AnalysisRequest{SignalContext: aianalysisv1.SignalContextInput{
					TargetResource: aianalysisv1.TargetResource{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "payments", Name: "checkout"},
				}},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				CompletedAt: &metav1.Time{Time: observedAt.Add(-30 * time.Second)},
				Approval: &aianalysisv1.ApprovalStatus{
					ApprovalRequired: true,
					ApprovalContext: &aianalysisv1.ApprovalContext{PolicyEvaluation: &aianalysisv1.PolicyEvaluation{
						PolicyName:   "approval-policy",
						MatchedRules: []string{"production_requires_review"},
						Decision:     aianalysisv1.PolicyDecisionManualReviewRequired,
						PolicyHash:   "ai-policy-hash",
					}},
				},
				RCAResult: &aianalysisv1.RCAResult{},
			},
		}
		ai.Status.RCAResult.SelectionAssessment = nil
		ai.Spec.AnalysisRequest.SignalContext.TargetResource = aianalysisv1.TargetResource{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "payments", Name: "checkout"}
		// PolicyEvaluation is persisted in the RCA result's investigation data in
		// production; this fixture uses the public status shape and checks that
		// absent fields remain absent rather than being inferred.
		sp := spv1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{Name: "sp-checkout", CreationTimestamp: metav1.Time{Time: observedAt.Add(-2 * time.Minute)}},
			Spec: spv1.SignalProcessingSpec{Signal: spv1.SignalData{
				ClusterID:      "",
				TargetResource: spv1.ResourceIdentifier{Kind: "Deployment", Namespace: "payments", Name: "checkout"},
			}},
			Status: spv1.SignalProcessingStatus{CompletionTime: &metav1.Time{Time: observedAt.Add(-90 * time.Second)}, SignalClassification: &spv1.SignalClassification{PolicyHash: "sp-policy-hash"}},
		}

		svc, err := policyread.NewService(policyread.Config{
			Namespace: "kubernaut-system",
			BasePath:  "/api/v1/console",
		}, policyread.Dependencies{
			LocalInventory: &inventoryStub{page: page},
			CurrentPolicies: &currentPolicyStub{policies: []policyread.PolicyMetadata{
				{Type: "aiAnalysis", ConfigMapName: "aianalysis-policy", ConfigMapKey: "approval.rego", Revision: "sha256:ai", Threshold: &policyread.Threshold{Value: 0.85, Source: "runtime-config", Kind: "rego-approval"}},
				{Type: "signalProcessing", ConfigMapName: "signalprocessing-policy", ConfigMapKey: "policy.rego", Revision: "sha256:sp"},
			}},
			History:    &historyStub{ai: []aianalysisv1.AIAnalysis{ai}, sp: []spv1.SignalProcessing{sp}},
			Authorizer: allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := svc.ListPolicyScope(context.Background(), &auth.UserIdentity{Username: "alice", Groups: []string{"sre"}}, inventory.Filters{ClusterID: "local"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.State).To(Equal(inventory.StateSuccess))
		Expect(result.Items).To(HaveLen(1))
		Expect(result.Items[0].CurrentPolicies).To(HaveLen(2))
		Expect(result.Items[0].CurrentPolicies[0].Threshold.Value).To(Equal(0.85))
		Expect(result.Items[0].CurrentPolicies[0].Threshold.Source).To(Equal("runtime-config"))
		Expect(result.Items[0].CurrentPolicies[0].Revision).To(Equal("sha256:ai"))
		Expect(result.Items[0].HistoricalEvaluations).To(HaveLen(2))
		Expect(result.Items[0].HistoricalEvaluations[0].Source).To(Equal("AIAnalysis"))
		Expect(result.Items[0].HistoricalEvaluations[0].MatchedRules).To(ConsistOf("production_requires_review"))
		Expect(result.Items[0].InventoryLink).To(ContainSubstring("/inventory/managed-resources/" + resource.ID))

		encoded, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("package approval"))
		Expect(string(encoded)).NotTo(ContainSubstring("package signal"))
		Expect(string(encoded)).NotTo(ContainSubstring("secret"))
		Expect(string(encoded)).NotTo(ContainSubstring("total"))
	})

	It("returns a state-only stale result without records", func() {
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &inventoryStub{page: inventory.Page{State: inventory.StateStale, Completeness: inventory.CompletenessPartial}},
			Authorizer:     allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		result, err := svc.ListPolicyScope(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"})
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateStale))
		Expect(result.State).To(Equal(inventory.StateStale))
		Expect(result.Items).To(BeEmpty())
	})

	DescribeTable("rejects non-interactive callers before inventory reads", func(identity *auth.UserIdentity) {
		reader := &inventoryStub{page: inventory.Page{State: inventory.StateSuccess, Completeness: inventory.CompletenessComplete}}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{LocalInventory: reader, Authorizer: allowAuthorizer{}})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.ListInventory(context.Background(), identity, inventory.Filters{ClusterID: "local"})
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateForbidden))
		Expect(reader.listCalls).To(Equal(0))
	},
		Entry("missing identity", nil),
		Entry("service account", user("system:serviceaccount:kubernaut-system:af", true)),
	)

	It("keeps exact backend failures distinct from not-found", func() {
		resourceID := policyReadTestResourceID
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &inventoryStub{getErr: inventory.ErrUnavailable},
			Authorizer:     allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.GetInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"}, resourceID)
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
	})

	It("maps exact local target authorizer failures to unavailable", func() {
		resourceID := policyReadTestResourceID
		authorizer := &targetAuthorizer{err: errors.New("subject access review unavailable")}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &inventoryStub{resource: inventory.Resource{
				ID: resourceID, ClusterID: "local", Group: "apps", Version: "v1", Kind: "Deployment",
				Namespace: "payments", Name: "checkout", Scope: inventory.Scope{Included: true},
			}},
			Authorizer: authorizer,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.GetInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"}, resourceID)
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(authorizer.target.Resource).To(Equal("deployments"))
	})

	It("maps exact local target denials to not-found", func() {
		resourceID := policyReadTestResourceID
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &inventoryStub{resource: inventory.Resource{
				ID: resourceID, ClusterID: "local", Group: "apps", Version: "v1", Kind: "Deployment",
				Namespace: "payments", Name: "checkout", Scope: inventory.Scope{Included: true},
			}},
			Authorizer: &targetAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.GetInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"}, resourceID)
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateNotFound))
	})

	It("fails closed when an exact local resource kind is not allowlisted", func() {
		resourceID := policyReadTestResourceID
		authorizer := &targetAuthorizer{allowed: true}
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &inventoryStub{resource: inventory.Resource{
				ID: resourceID, ClusterID: "local", Version: "v1", Kind: "ConfigMap",
				Namespace: "payments", Name: "checkout", Scope: inventory.Scope{Included: true},
			}},
			Authorizer: authorizer,
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.GetInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"}, resourceID)
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
		Expect(authorizer.calls).To(BeZero())
	})

	It("maps malformed exact backend identity to unavailable", func() {
		resourceID := policyReadTestResourceID
		svc, err := policyread.NewService(policyread.Config{}, policyread.Dependencies{
			LocalInventory: &inventoryStub{resource: inventory.Resource{ID: resourceID, ClusterID: "other-cluster", Kind: "Deployment", Version: "v1", Name: "checkout"}},
			Authorizer:     allowAuthorizer{},
		})
		Expect(err).NotTo(HaveOccurred())

		_, err = svc.GetInventory(context.Background(), user("alice", false), inventory.Filters{ClusterID: "local"}, resourceID)
		Expect(err).To(HaveOccurred())
		Expect(policyread.StateOf(err)).To(Equal(inventory.StateUnavailable))
	})
})

func user(name string, serviceAccount bool) *auth.UserIdentity {
	return &auth.UserIdentity{Username: name, IsServiceAccount: serviceAccount}
}

type inventoryStub struct {
	page      inventory.Page
	resource  inventory.Resource
	getErr    error
	listCalls int
}

type recordingScopeChecker struct {
	identity scope.ResourceIdentity
	err      error
}

func (s *recordingScopeChecker) IsManagedResource(_ context.Context, identity scope.ResourceIdentity) (bool, error) {
	s.identity = identity
	if s.err != nil {
		return false, s.err
	}
	return true, nil
}

func (s *inventoryStub) List(context.Context, inventory.Filters) (inventory.Page, error) {
	s.listCalls++
	return s.page, nil
}

func (s *inventoryStub) Get(context.Context, string, string) (inventory.Resource, error) {
	if s.getErr != nil {
		return inventory.Resource{}, s.getErr
	}
	if s.resource.ID == "" {
		return inventory.Resource{}, inventory.ErrNotFound
	}
	return s.resource, nil
}

type currentPolicyStub struct {
	policies []policyread.PolicyMetadata
	err      error
}

func (s *currentPolicyStub) ReadCurrentPolicies(context.Context, string) ([]policyread.PolicyMetadata, error) {
	return s.policies, s.err
}

type historyStub struct {
	ai []aianalysisv1.AIAnalysis
	sp []spv1.SignalProcessing
}

func (s *historyStub) ListAIAnalysis(context.Context) ([]aianalysisv1.AIAnalysis, error) {
	return s.ai, nil
}

func (s *historyStub) ListSignalProcessing(context.Context) ([]spv1.SignalProcessing, error) {
	return s.sp, nil
}

type allowAuthorizer struct{}

func (allowAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}

func (allowAuthorizer) CheckTarget(context.Context, string, []string, auth.TargetAccess) (bool, error) {
	return true, nil
}

type targetAuthorizer struct {
	allowed bool
	err     error
	target  auth.TargetAccess
	calls   int
}

func (a *targetAuthorizer) Check(context.Context, string, []string, string) (bool, error) {
	return true, nil
}

func (a *targetAuthorizer) CheckTarget(_ context.Context, _ string, _ []string, target auth.TargetAccess) (bool, error) {
	a.calls++
	a.target = target
	return a.allowed, a.err
}

var _ policyread.InventoryReader = (*inventoryStub)(nil)
var _ policyread.CurrentPolicySource = (*currentPolicyStub)(nil)
var _ policyread.HistorySource = (*historyStub)(nil)
var _ policyread.Authorizer = allowAuthorizer{}
