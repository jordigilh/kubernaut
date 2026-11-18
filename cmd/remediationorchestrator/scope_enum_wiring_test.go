package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	config "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

func TestScopeEnumWiring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RO scope enum command wiring")
}

// Only the external Kubernetes manager accessors are controlled; the routing
// engine, fleet factory, transport and scope admission logic are real.
type scopeWiringManager struct {
	ctrl.Manager
	reader client.Client
}

func (m scopeWiringManager) GetClient() client.Client    { return m.reader }
func (m scopeWiringManager) GetAPIReader() client.Reader { return m.reader }

var _ = Describe("RO typed scope command wiring [BR-SCOPE-010]", func() {
	DescribeTable("IT-RO-PRES-005: buildRoutingEngine preserves the WFE scope-block/requeue decision",
		func(state scope.ScopeState, source scope.ScopeSource, status int, blocked bool, labelAdvice bool) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				defer GinkgoRecover()
				Expect(request.Header.Get("Authorization")).To(Equal("Bearer scope-test"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				Expect(json.NewEncoder(w).Encode(scope.ScopeResult{State: state, Provenance: source})).To(Succeed())
			}))
			defer server.Close()
			token := filepath.Join(GinkgoT().TempDir(), "token")
			Expect(os.WriteFile(token, []byte("scope-test"), 0o600)).To(Succeed())
			cfg := config.DefaultConfig()
			cfg.Fleet = fleet.FleetConfig{Enabled: true, Backend: fleet.BackendFMC, Endpoint: server.URL, TokenPath: token}
			engine, err := buildRoutingEngine(cfg, scopeWiringManager{reader: fake.NewClientBuilder().Build()}, logr.Discard())
			Expect(err).NotTo(HaveOccurred())
			rr := &remediationv1.RemediationRequest{Spec: remediationv1.RemediationRequestSpec{
				ClusterID: "hub", TargetResource: remediationv1.ResourceIdentifier{APIVersion: "v1", Kind: "Pod", Namespace: "payments", Name: "api"},
			}}
			condition := engine.CheckUnmanagedResource(context.Background(), rr)
			if !blocked {
				Expect(condition).To(BeNil())
				return
			}
			Expect(condition).NotTo(BeNil())
			Expect(condition.Blocked).To(BeTrue())
			Expect(condition.Reason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))
			Expect(condition.RequeueAfter).To(BeNumerically(">", 0))
			if labelAdvice {
				Expect(condition.Message).To(ContainSubstring("Add label"))
			} else {
				Expect(condition.Message).To(ContainSubstring("unavailable"))
				Expect(condition.Message).NotTo(ContainSubstring("Add label"))
			}
		},
		Entry("verified managed", scope.ScopeVerifiedManaged, scope.ScopeSourceResourceLabel, 200, false, false),
		Entry("verified unmanaged", scope.ScopeVerifiedUnmanaged, scope.ScopeSourceResourceLabel, 200, true, true),
		Entry("unavailable", scope.ScopeUnavailable, scope.ScopeSourceUnavailable, 503, true, false),
		Entry("unsupported", scope.ScopeUnsupported, scope.ScopeSourceUnavailable, 422, true, false),
		Entry("invalid", scope.ScopeInvalid, scope.ScopeSourceUnavailable, 422, true, false),
		Entry("unauthorized", scope.ScopeUnauthorized, scope.ScopeSourceUnavailable, 403, true, false),
	)
})
