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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aiav1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

func shortCtx(parent context.Context) context.Context {
	ctx, cancel := context.WithTimeout(parent, 100*time.Millisecond)
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}

func investigateTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = aiav1alpha1.AddToScheme(s)
	_ = remediationv1.AddToScheme(s)
	return s
}

func newTypedClientForInvestigate(objects ...crclient.Object) crclient.Client {
	return fake.NewClientBuilder().
		WithScheme(investigateTestScheme()).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()
}

func newTypedAIAnalysisWithSession(rrName, sessionID string) *aiav1alpha1.AIAnalysis {
	return &aiav1alpha1.AIAnalysis{
		ObjectMeta: objMeta("kubernaut-system", fmt.Sprintf("ai-%s", rrName)),
		Spec: aiav1alpha1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      rrName,
				Namespace: "kubernaut-system",
			},
			RemediationID: rrName,
		},
		Status: aiav1alpha1.AIAnalysisStatus{
			KASession: &aiav1alpha1.KASession{
				ID: sessionID,
			},
		},
	}
}

func newTypedAIAnalysisWithoutSession(rrName string) *aiav1alpha1.AIAnalysis {
	return &aiav1alpha1.AIAnalysis{
		ObjectMeta: objMeta("kubernaut-system", fmt.Sprintf("ai-%s", rrName)),
		Spec: aiav1alpha1.AIAnalysisSpec{
			RemediationRequestRef: corev1.ObjectReference{
				Name:      rrName,
				Namespace: "kubernaut-system",
			},
			RemediationID: rrName,
		},
		Status: aiav1alpha1.AIAnalysisStatus{
			KASession: &aiav1alpha1.KASession{},
		},
	}
}

var _ = Describe("kubernaut_investigate intent-based enhancement (#1332)", func() {

	Describe("InvestigateMCPArgs validation (F-02, F-03)", func() {
		It("UT-AF-1332-012: empty args (no rr_id, no api_version/kind/name) returns error", func() {
			mockMCP := &ka.MockMCPClient{}
			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "kubernaut-system",
				tools.InvestigateMCPArgs{},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rr_id or api_version/kind/name required"))
		})

		It("UT-AF-1332-013: partial args (namespace only, missing kind/name) returns error", func() {
			mockMCP := &ka.MockMCPClient{}
			_, err := tools.HandleInvestigationMCPWithRegistry(
				context.Background(), mockMCP, nil, "kubernaut-system",
				tools.InvestigateMCPArgs{Namespace: "prod", APIVersion: "apps/v1"},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kind and name required"))
		})
	})

	Describe("Investigation with namespace/kind/name — creates RR + IS (F-02)", func() {
		It("UT-AF-1332-011: creates RR and IS when namespace/kind/name provided", func() {
			tc := newTypedClientForInvestigate()
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
				ctx, mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{
					APIVersion: "apps/v1",
					Namespace:  "prod",
					Kind:       "Deployment",
					Name:       "web-app",
				},
				nil, nil, nil, true, nil, "", nil, defaultTestTriager("prod", "Deployment", "web-app"),
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
					APIVersion: "apps/v1",
					Namespace:  "prod",
					Kind:       "Deployment",
					Name:       "web-fail",
				},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("k8s"))
		})

		It("UT-AF-1332-016: SA caller blocked from interactive investigation", func() {
			tc := newTypedClientForInvestigate()
			mockMCP := &ka.MockMCPClient{}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username:         "system:serviceaccount:kubernaut-system:af-agent",
				Groups:           []string{"system:serviceaccounts"},
				IsServiceAccount: true,
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{
					APIVersion: "apps/v1",
					Namespace:  "prod",
					Kind:       "Deployment",
					Name:       "web-sa",
				},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("service account"))
		})
	})

	Describe("Investigation with existing rr_id — creates IS only (F-03)", func() {
		It("UT-AF-1332-010: creates IS when rr_id provided (existing RR)", func() {
			tc := newTypedClientForInvestigate()
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
				ctx, mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-existing-001"},
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.SessionID).To(Equal("sess-exist-001"))
		})
	})

	Describe("IS creation failure — transactional cleanup (NF-01)", func() {
		It("UT-AF-1332-015: IS failure after RR creation triggers RR cleanup", func() {
			tc := newTypedClientForInvestigate()
			mockMCP := &ka.MockMCPClient{}

			ctx := shortCtx(auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "dana",
				Groups:   []string{"sre"},
			}))

			sessionInitErr := fmt.Errorf("simulated IS creation failure")
			_ = sessionInitErr

			_, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{
					APIVersion: "apps/v1",
					Namespace:  "prod",
					Kind:       "Deployment",
					Name:       "web-is-fail",
				},
				nil, nil, nil, true, nil, "", nil, nil,
			)
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
				nil, nil, nil, true, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status).To(Equal("completed"))
			Expect(result.Summary).To(ContainSubstring("OOMKilled"))
		})
	})

	Describe("ISSignaler autonomous detection and early IS creation (#1332)", func() {

		It("UT-AF-1332-070: signals joinMode=takeover when AIA has active session (autonomous detection)", func() {
			aia := newTypedAIAnalysisWithSession("rr-takeover-001", "ka-sess-auto-001")
			tc := newTypedClientForInvestigate(aia)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "ka-sess-new-001",
						Status:    "investigation_started",
						Closer:    func() {},
					}, nil
				},
			}

			recorder := &recordingISSignaler{}
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre@kubernaut.ai",
				Groups:   []string{"sre"},
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(ctx), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-takeover-001"},
				nil, nil, nil, false, nil, "", recorder, nil,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.signalCalls).To(HaveLen(1))
			Expect(recorder.signalCalls[0].joinMode).To(Equal("takeover"),
				"autonomous AIA with session ID must trigger takeover joinMode")
			Expect(recorder.signalCalls[0].rrName).To(Equal("rr-takeover-001"))
			Expect(recorder.signalCalls[0].username).To(Equal("sre@kubernaut.ai"))
		})

		It("UT-AF-1332-071: signals joinMode=start when no AIA exists for the RR", func() {
			tc := newTypedClientForInvestigate()

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "ka-sess-fresh-001",
						Status:    "investigation_started",
						Closer:    func() {},
					}, nil
				},
			}

			recorder := &recordingISSignaler{}
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "dev@kubernaut.ai",
				Groups:   []string{"dev"},
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(ctx), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-fresh-001"},
				nil, nil, nil, false, nil, "", recorder, nil,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.signalCalls).To(HaveLen(1))
			Expect(recorder.signalCalls[0].joinMode).To(Equal("start"),
				"no active AIA must result in fresh start joinMode")
			Expect(recorder.signalCalls[0].rrName).To(Equal("rr-fresh-001"))
			Expect(recorder.signalCalls[0].username).To(Equal("dev@kubernaut.ai"))
		})

		It("UT-AF-1332-072: signals joinMode=start when AIA exists but has no session ID", func() {
			aia := newTypedAIAnalysisWithoutSession("rr-pending-001")
			tc := newTypedClientForInvestigate(aia)

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "ka-sess-pending-001",
						Status:    "investigation_started",
						Closer:    func() {},
					}, nil
				},
			}

			recorder := &recordingISSignaler{}
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre@kubernaut.ai",
				Groups:   []string{"sre"},
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(ctx), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-pending-001"},
				nil, nil, nil, false, nil, "", recorder, nil,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.signalCalls).To(HaveLen(1))
			Expect(recorder.signalCalls[0].joinMode).To(Equal("start"),
				"AIA without session ID is not autonomous — should use start")
		})

		It("UT-AF-1332-073: UpdateCorrelation called with KA session ID after MCP connect", func() {
			tc := newTypedClientForInvestigate()

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "ka-corr-sess-001",
						Status:    "investigation_started",
						Closer:    func() {},
					}, nil
				},
			}

			recorder := &recordingISSignaler{}
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre@kubernaut.ai",
				Groups:   []string{"sre"},
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(ctx), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-corr-001"},
				nil, nil, nil, false, nil, "", recorder, nil,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.correlationCalls).To(HaveLen(1))
			Expect(recorder.correlationCalls[0].crdName).To(Equal("is-rr-corr-001"))
			Expect(recorder.correlationCalls[0].kaSessionID).To(Equal("ka-corr-sess-001"))
		})

		It("UT-AF-2029-101: UpdateCorrelation uses InvestigationSessionID (pollable analysis session), not the driver-lease SessionID, when KA returns both (#2029 regression)", func() {
			// KA's kubernaut_investigate tool returns two distinct IDs: SessionID is the
			// MCP driver-lease ID (exclusive-control lock, never pollable via REST), while
			// InvestigationSessionID is the actual analysis session AA polls for RCA/workflow
			// results. Correlating the driver-lease ID into IS.Status.KACorrelationID causes
			// AA's handleSessionLost to 404-loop forever (see #2029 must-gather RCA).
			tc := newTypedClientForInvestigate()

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID:              "ka-driver-lease-001",
						InvestigationSessionID: "ka-analysis-sess-001",
						Status:                 "investigation_started",
						Closer:                 func() {},
					}, nil
				},
			}

			recorder := &recordingISSignaler{}
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre@kubernaut.ai",
				Groups:   []string{"sre"},
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(ctx), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-corr-002"},
				nil, nil, nil, false, nil, "", recorder, nil,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.correlationCalls).To(HaveLen(1))
			Expect(recorder.correlationCalls[0].crdName).To(Equal("is-rr-corr-002"))
			Expect(recorder.correlationCalls[0].kaSessionID).To(Equal("ka-analysis-sess-001"),
				"must correlate the pollable analysis session ID, not the driver-lease ID, or AA's session adoption (#2029 Part B) 404-loops forever")
		})

		It("UT-AF-2029-102: UpdateCorrelation falls back to SessionID when KA omits InvestigationSessionID (back-compat)", func() {
			tc := newTypedClientForInvestigate()

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "ka-driver-lease-002",
						Status:    "investigation_started",
						Closer:    func() {},
					}, nil
				},
			}

			recorder := &recordingISSignaler{}
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre@kubernaut.ai",
				Groups:   []string{"sre"},
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(ctx), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-corr-003"},
				nil, nil, nil, false, nil, "", recorder, nil,
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.correlationCalls).To(HaveLen(1))
			Expect(recorder.correlationCalls[0].kaSessionID).To(Equal("ka-driver-lease-002"))
		})

		It("UT-AF-1332-074: signaler not called when signaler is nil (backward compat)", func() {
			tc := newTypedClientForInvestigate()

			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{
						SessionID: "ka-nil-sig-001",
						Status:    "investigation_started",
						Closer:    func() {},
					}, nil
				},
			}

			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre@kubernaut.ai",
				Groups:   []string{"sre"},
			})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(ctx), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{RRID: "rr-nil-sig-001"},
				nil, nil, nil, false, nil, "", nil, nil,
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Scope validation before RR creation — #2022 (AC-6, SI-10, AU-3/AU-12, SI-11)", func() {
		scopeInvestigateArgs := func() tools.InvestigateMCPArgs {
			return tools.InvestigateMCPArgs{
				APIVersion: "apps/v1",
				Namespace:  "unmanaged-ns",
				Kind:       "Deployment",
				Name:       "legacy-app",
			}
		}
		identityCtx := func() context.Context {
			return auth.WithUserIdentity(context.Background(), &auth.UserIdentity{
				Username: "sre-carol",
				Groups:   []string{"sre"},
			})
		}

		It("UT-AF-2022-020: rejects an unmanaged resource without creating an RR or starting an MCP session", func() {
			tc := newTypedClientForInvestigate()
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					Fail("StartInvestigation must not be called for an unmanaged resource")
					return nil, nil
				},
			}
			ctx := tools.ContextWithScopeChecker(identityCtx(), &mocks.NeverManagedScopeChecker{})

			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, tc, "kubernaut-system",
				scopeInvestigateArgs(),
				nil, nil, nil, true, nil, "", nil, defaultTestTriager("unmanaged-ns", "Deployment", "legacy-app"),
			)

			Expect(err).NotTo(HaveOccurred(), "SI-11: rejection must be a clear result, not an opaque tool error")
			Expect(result.Managed).To(BeFalse())
			Expect(result.Status).To(Equal("unmanaged"))
			Expect(result.RRID).To(BeEmpty())
			Expect(result.Error).To(ContainSubstring("not managed by Kubernaut"))
		})

		It("UT-AF-2022-021: allows a managed resource to proceed to RR creation and MCP start", func() {
			tc := newTypedClientForInvestigate()
			eventCh := make(chan ka.InvestigationEvent)
			close(eventCh)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{SessionID: "sess-2022-021", Status: "started", Events: eventCh, Closer: func() {}}, nil
				},
			}
			ctx := tools.ContextWithScopeChecker(identityCtx(), &mocks.AlwaysManagedScopeChecker{})

			result, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(ctx), mockMCP, tc, "kubernaut-system",
				scopeInvestigateArgs(),
				nil, nil, nil, true, nil, "", nil, defaultTestTriager("unmanaged-ns", "Deployment", "legacy-app"),
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-2022-022: no ScopeChecker in context gracefully degrades to always-managed (backward compat)", func() {
			tc := newTypedClientForInvestigate()
			eventCh := make(chan ka.InvestigationEvent)
			close(eventCh)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{SessionID: "sess-2022-022", Status: "started", Events: eventCh, Closer: func() {}}, nil
				},
			}

			result, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(identityCtx()), mockMCP, tc, "kubernaut-system",
				scopeInvestigateArgs(),
				nil, nil, nil, true, nil, "", nil, defaultTestTriager("unmanaged-ns", "Deployment", "legacy-app"),
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RRID).NotTo(BeEmpty())
		})

		It("UT-AF-2022-023: emits an EventRRScopeRejected audit event on rejection (AU-3/AU-12)", func() {
			tc := newTypedClientForInvestigate()
			mockMCP := &ka.MockMCPClient{}
			recorder := &auditRecorder{}
			ctx := tools.ContextWithScopeChecker(identityCtx(), &mocks.NeverManagedScopeChecker{})

			_, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, mockMCP, tc, "kubernaut-system",
				scopeInvestigateArgs(),
				recorder, nil, nil, true, nil, "", nil, defaultTestTriager("unmanaged-ns", "Deployment", "legacy-app"),
			)
			Expect(err).NotTo(HaveOccurred())

			Expect(recorder.events).To(HaveLen(1))
			Expect(recorder.events[0].Type).To(Equal(audit.EventRRScopeRejected))
			Expect(recorder.events[0].UserID).To(Equal("sre-carol"))
		})
	})

	Describe("Ambiguous severity/signal correlation — DD-AF-012 (#2027)", func() {
		It("UT-AF-2027-006: surfaces Ambiguous/CandidateSignalName/CandidateSeverity when only a cluster-scoped alert correlates, then proceeds once confirmed", func() {
			tc := newTypedClientForInvestigate()
			ctx := auth.WithUserIdentity(context.Background(), &auth.UserIdentity{Username: "sre-erin", Groups: []string{"sre"}})

			result, err := tools.HandleInvestigationMCPWithRegistry(
				ctx, &ka.MockMCPClient{}, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{APIVersion: "apps/v1", Namespace: "prod", Kind: "Deployment", Name: "web-ambiguous"},
				nil, nil, nil, true, nil, "", nil, ambiguousTestTriager(),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Ambiguous).To(BeTrue())
			Expect(result.CandidateSignalName).To(Equal("TestDefaultAlert"))
			Expect(result.CandidateSeverity).To(Equal("warning"))
			Expect(result.RRID).To(BeEmpty())

			eventCh := make(chan ka.InvestigationEvent)
			close(eventCh)
			mockMCP := &ka.MockMCPClient{
				StartInvestigationFn: func(_ context.Context, _ ka.StartInvestigationArgs) (*ka.StartInvestigationResult, error) {
					return &ka.StartInvestigationResult{SessionID: "sess-2027-006", Status: "started", Events: eventCh, Closer: func() {}}, nil
				},
			}
			confirmed, err := tools.HandleInvestigationMCPWithRegistry(
				shortCtx(ctx), mockMCP, tc, "kubernaut-system",
				tools.InvestigateMCPArgs{APIVersion: "apps/v1", Namespace: "prod", Kind: "Deployment", Name: "web-ambiguous", ConfirmedSignalName: "TestDefaultAlert"},
				nil, nil, nil, true, nil, "", nil, ambiguousTestTriager(),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(confirmed.Ambiguous).To(BeFalse())
			Expect(confirmed.RRID).NotTo(BeEmpty())
		})
	})
})

type recordingISSignaler struct {
	signalCalls      []signalCall
	correlationCalls []corrCall
}

type signalCall struct {
	rrNamespace, rrName, taskID, username, joinMode string
	groups                                          []string
}

type corrCall struct {
	crdName, kaSessionID string
}

func (r *recordingISSignaler) SignalInteractive(_ context.Context, rrNamespace, rrName, taskID, username string, groups []string, joinMode string) (string, error) {
	r.signalCalls = append(r.signalCalls, signalCall{
		rrNamespace: rrNamespace, rrName: rrName, taskID: taskID,
		username: username, groups: groups, joinMode: joinMode,
	})
	return fmt.Sprintf("is-%s", rrName), nil
}

func (r *recordingISSignaler) UpdateCorrelation(_ context.Context, crdName, kaSessionID string) error {
	r.correlationCalls = append(r.correlationCalls, corrCall{crdName: crdName, kaSessionID: kaSessionID})
	return nil
}
