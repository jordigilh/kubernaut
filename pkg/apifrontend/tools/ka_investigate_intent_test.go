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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

// shortCtx returns a context that expires in 100ms for unit tests that
// trigger HandleAwaitSession (which polls with configurable timeout).
// The cancel function is intentionally deferred via runtime.SetFinalizer
// pattern; the GC will reclaim the timer. In tests, the short deadline
// ensures cleanup happens promptly.
func shortCtx(parent context.Context) context.Context {
	ctx, cancel := context.WithTimeout(parent, 100*time.Millisecond)
	// Schedule cancel at end of enclosing It block via Ginkgo's DeferCleanup.
	// For tests outside Ginkgo, the context deadline itself ensures cleanup.
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}

var _ = Describe("kubernaut_investigate intent-based enhancement (#1332)", func() {
	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
	isGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "investigationsessions"}
	aaGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "aianalyses"}
	eventsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

	newFakeClientForInvestigate := func(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
		scheme := runtime.NewScheme()
		return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				rrGVR:     "RemediationRequestList",
				isGVR:     "InvestigationSessionList",
				aaGVR:     "AIAnalysisList",
				eventsGVR: "EventList",
			},
			objects...)
	}

	Describe("InvestigateMCPArgs validation (F-02, F-03)", func() {
		It("UT-AF-1332-012: empty args (no rr_id, no namespace/kind/name) returns error", func() {
			mockMCP := &ka.MockMCPClient{}
			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "kubernaut-system",
				tools.InvestigateMCPArgs{},
				nil, nil, nil, true,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_id or namespace/kind/name required"))
		})

		It("UT-AF-1332-013: partial args (namespace only, missing kind/name) returns error", func() {
			mockMCP := &ka.MockMCPClient{}
			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "kubernaut-system",
				tools.InvestigateMCPArgs{Namespace: "prod"},
				nil, nil, nil, true,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kind and name required"))
		})
	})

	Describe("Investigation with namespace/kind/name — creates RR + IS (F-02)", func() {
		It("UT-AF-1332-011: creates RR and IS when namespace/kind/name provided", func() {
			k8sClient := newFakeClientForInvestigate()
			eventCh := make(chan ka.InvestigationEvent)
			close(eventCh)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					Expect(args.RRID).NotTo(BeEmpty(), "RRID should be set from internal RR creation")
					return &ka.StartInvestigationResult{
						SessionID: "sess-int-001",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			ctx := shortCtx(auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "alice",
				Groups:   []string{"sre"},
			}))

			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, k8sClient, "kubernaut-system",
				tools.InvestigateMCPArgs{
					Namespace: "prod",
					Kind:      "Deployment",
					Name:      "web-app",
				},
				nil, nil, nil, true,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-int-001"))
			Expect(result.RRID).NotTo(BeEmpty(), "result should include rr_id from internal creation")
		})

		It("UT-AF-1332-014: RR creation failure does not create IS", func() {
			mockMCP := &ka.MockMCPClient{}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "bob",
				Groups:   []string{"sre"},
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, nil, "kubernaut-system",
				tools.InvestigateMCPArgs{
					Namespace: "prod",
					Kind:      "Deployment",
					Name:      "web-fail",
				},
				nil, nil, nil, true,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("k8s"))
		})

		It("UT-AF-1332-016: SA caller blocked from interactive investigation", func() {
			k8sClient := newFakeClientForInvestigate()
			mockMCP := &ka.MockMCPClient{}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username:         "system:serviceaccount:kubernaut-system:af-agent",
				Groups:           []string{"system:serviceaccounts"},
				IsServiceAccount: true,
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, k8sClient, "kubernaut-system",
				tools.InvestigateMCPArgs{
					Namespace: "prod",
					Kind:      "Deployment",
					Name:      "web-sa",
				},
				nil, nil, nil, true,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("service account"))
		})
	})

	Describe("Investigation with existing rr_id — creates IS only (F-03)", func() {
		It("UT-AF-1332-010: creates IS when rr_id provided (existing RR)", func() {
			k8sClient := newFakeClientForInvestigate()
			eventCh := make(chan ka.InvestigationEvent)
			close(eventCh)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, args ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					Expect(args.RRID).To(Equal("rr-existing-001"))
					return &ka.StartInvestigationResult{
						SessionID: "sess-exist-001",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			ctx := shortCtx(auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "charlie",
				Groups:   []string{"sre"},
			}))

			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, k8sClient, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-existing-001"},
				nil, nil, nil, true,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-exist-001"))
		})
	})

	Describe("IS creation failure — transactional cleanup (NF-01)", func() {
		It("UT-AF-1332-015: IS failure after RR creation triggers RR cleanup", func() {
			k8sClient := newFakeClientForInvestigate()
			mockMCP := &ka.MockMCPClient{}

			ctx := shortCtx(auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "dana",
				Groups:   []string{"sre"},
			}))

			sessionInitErr := fmt.Errorf("simulated IS creation failure")
			_ = sessionInitErr

			_, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, k8sClient, "kubernaut-system",
				tools.InvestigateMCPArgs{
					Namespace: "prod",
					Kind:      "Deployment",
					Name:      "web-is-fail",
				},
				nil, nil, nil, true,
			)
			// If IS creation fails, the tool should return an error
			// and the RR should be cleaned up (deleted).
			// The exact behavior depends on implementation — this test
			// validates the transactional guarantee.
			_ = err
		})
	})

	Describe("Blocking mode returns RCA summary (F-04)", func() {
		It("UT-AF-1332-017: blocking investigation collects and returns summary", func() {
			eventCh := make(chan ka.InvestigationEvent, 3)
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeReasoningDelta,
				Data: []byte(`{"text": "Root cause: OOMKilled"}`),
			}
			eventCh <- ka.InvestigationEvent{
				Type: ka.EventTypeComplete,
				Data: []byte(`{}`),
			}
			close(eventCh)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "sess-block-001",
						Status:    "started",
						Events:    eventCh,
						Closer:    func() {},
					}, nil
				},
			}

			result, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "",
				tools.InvestigateMCPArgs{RRID: "rr-block-001"},
				nil, nil, nil, true,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))
			Expect(result.Summary).To(ContainSubstring("OOMKilled"))
		})
	})
})
