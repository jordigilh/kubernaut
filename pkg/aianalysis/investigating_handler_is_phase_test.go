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

package aianalysis_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/test/shared/mocks"
)

// mockISPhaseUpdater tracks SetActivePhase and SetTerminalPhase calls.
type mockISPhaseUpdater struct {
	mu                  sync.Mutex
	activePhaseRRNames  []string
	terminalPhaseCalls  []terminalPhaseCall
	setActiveErr        error
	setTerminalErr      error
}

type terminalPhaseCall struct {
	RRName string
	Phase  isv1alpha1.SessionPhase
}

func (m *mockISPhaseUpdater) SetActivePhase(_ context.Context, rrName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activePhaseRRNames = append(m.activePhaseRRNames, rrName)
	return m.setActiveErr
}

func (m *mockISPhaseUpdater) SetTerminalPhase(_ context.Context, rrName string, phase isv1alpha1.SessionPhase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.terminalPhaseCalls = append(m.terminalPhaseCalls, terminalPhaseCall{RRName: rrName, Phase: phase})
	return m.setTerminalErr
}

func (m *mockISPhaseUpdater) getTerminalPhaseCalls() []terminalPhaseCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]terminalPhaseCall, len(m.terminalPhaseCalls))
	copy(out, m.terminalPhaseCalls)
	return out
}

var _ = Describe("InvestigatingHandler IS Phase Completion — #1376, BR-INTERACTIVE-010", func() {

	var (
		ctx        context.Context
		mockClient *mocks.MockAgentClient
		auditSpy   *sessionAuditSpy
		recorder   *record.FakeRecorder
	)

	createAnalysisWithSession := func(rrName, sessionID string) *aianalysisv1.AIAnalysis {
		now := metav1.Now()
		return &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-is-phase-analysis",
				Namespace: "default",
			},
			Spec: aianalysisv1.AIAnalysisSpec{
				RemediationRequestRef: corev1.ObjectReference{
					Kind:      "RemediationRequest",
					Name:      rrName,
					Namespace: "default",
				},
				RemediationID: "test-remediation-1376",
				AnalysisRequest: aianalysisv1.AnalysisRequest{
					SignalContext: aianalysisv1.SignalContextInput{
						Fingerprint:      "test-fingerprint-1376",
						Severity:         "critical",
						SignalName:       "OOMKilled",
						Environment:      "production",
						BusinessPriority: "P0",
						TargetResource: aianalysisv1.TargetResource{
							Kind:      "Deployment",
							Name:      "api-server",
							Namespace: "demo-mesh",
						},
					},
					AnalysisTypes: []aianalysisv1.AnalysisType{aianalysisv1.AnalysisTypeInvestigation},
				},
			},
			Status: aianalysisv1.AIAnalysisStatus{
				Phase: aianalysis.PhaseInvestigating,
				KASession: &aianalysisv1.KASession{
					ID:        sessionID,
					PollCount: 3,
					CreatedAt: &now,
				},
			},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		mockClient = mocks.NewMockAgentClient()
		auditSpy = &sessionAuditSpy{}
		recorder = record.NewFakeRecorder(20)
	})

	Describe("UT-AA-1376-001: handleSessionPollCompleted calls SetTerminalPhase(Completed)", func() {
		It("should set IS phase to Completed when KA session completes successfully", func() {
			updater := &mockISPhaseUpdater{}
			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-is-complete"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithISPhaseUpdater(updater),
			)

			analysis := createAnalysisWithSession("rr-1376-complete", "session-complete-001")
			mockClient.WithSessionPollStatus("completed")
			mockClient.Response = &agentclient.IncidentResponse{
				IncidentID:        "mock-1376-001",
				Analysis:          "OOM caused by memory leak",
				RootCauseAnalysis: mocks.BuildMockRCA("OOM caused by memory leak", "high", nil),
				Confidence:        0.95,
				Timestamp:         "2026-06-06T10:00:00Z",
			}

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			calls := updater.getTerminalPhaseCalls()
			Expect(calls).To(HaveLen(1), "SetTerminalPhase should be called once on completed poll")
			Expect(calls[0].RRName).To(Equal("rr-1376-complete"))
			Expect(calls[0].Phase).To(Equal(isv1alpha1.SessionPhaseCompleted))
		})
	})

	Describe("UT-AA-1376-002: handleSessionPollFailed calls SetTerminalPhase(Failed)", func() {
		It("should set IS phase to Failed when KA session fails", func() {
			updater := &mockISPhaseUpdater{}
			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-is-failed"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithISPhaseUpdater(updater),
			)

			analysis := createAnalysisWithSession("rr-1376-failed", "session-failed-001")
			mockClient.WithSessionPollStatus("failed")
			mockClient.DefaultSessionStatus = &agentclient.SessionStatusResult{
				Status: "failed",
				Error:  "LLM timeout",
			}

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			calls := updater.getTerminalPhaseCalls()
			Expect(calls).To(HaveLen(1), "SetTerminalPhase should be called once on failed poll")
			Expect(calls[0].RRName).To(Equal("rr-1376-failed"))
			Expect(calls[0].Phase).To(Equal(isv1alpha1.SessionPhaseFailed))
		})
	})

	Describe("UT-AA-1376-003: SetTerminalPhase error is logged but does not block", func() {
		It("should not return error when SetTerminalPhase fails (best-effort)", func() {
			updater := &mockISPhaseUpdater{setTerminalErr: fmt.Errorf("API server unavailable")}
			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-is-phase-err"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithISPhaseUpdater(updater),
			)

			analysis := createAnalysisWithSession("rr-1376-err", "session-err-001")
			mockClient.WithSessionPollStatus("completed")
			mockClient.Response = &agentclient.IncidentResponse{
				IncidentID:        "mock-1376-003",
				Analysis:          "Root cause identified",
				RootCauseAnalysis: mocks.BuildMockRCA("Root cause identified", "warning", nil),
				Confidence:        0.90,
				Timestamp:         "2026-06-06T10:00:00Z",
			}

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred(),
				"SetTerminalPhase failure must not propagate — it is best-effort")

			calls := updater.getTerminalPhaseCalls()
			Expect(calls).To(HaveLen(1), "SetTerminalPhase should still be called")
		})
	})

	Describe("UT-AA-1376-004: No ISPhaseUpdater configured — no panic", func() {
		It("should complete normally when no ISPhaseUpdater is injected", func() {
			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-no-updater"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
			)

			analysis := createAnalysisWithSession("rr-1376-no-updater", "session-no-updater-001")
			mockClient.WithSessionPollStatus("completed")
			mockClient.Response = &agentclient.IncidentResponse{
				IncidentID:        "mock-1376-004",
				Analysis:          "Root cause identified",
				RootCauseAnalysis: mocks.BuildMockRCA("Root cause identified", "warning", nil),
				Confidence:        0.90,
				Timestamp:         "2026-06-06T10:00:00Z",
			}

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred(),
				"handler must not panic or fail when ISPhaseUpdater is nil")
		})
	})

	Describe("UT-AA-1376-005: Investigation timeout calls SetTerminalPhase(Failed)", func() {
		It("should set IS phase to Failed when investigation exceeds max duration", func() {
			updater := &mockISPhaseUpdater{}
			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-is-timeout"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithISPhaseUpdater(updater),
				handlers.WithMaxInvestigationDuration(1*time.Minute),
			)

			analysis := createAnalysisWithSession("rr-1376-timeout", "session-timeout-001")
			pastTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
			analysis.Status.KASession.CreatedAt = &pastTime

			mockClient.WithSessionPollStatus("investigating")

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed))

			calls := updater.getTerminalPhaseCalls()
			Expect(calls).To(HaveLen(1), "SetTerminalPhase should be called on timeout")
			Expect(calls[0].Phase).To(Equal(isv1alpha1.SessionPhaseFailed))
		})
	})

	Describe("UT-AA-1376-007: Interactive user_driving timeout calls SetTerminalPhase(Failed)", func() {
		It("should set IS phase to Failed when user_driving session exceeds max duration [BR-INTERACTIVE-010, AA-CRIT-1]", func() {
			updater := &mockISPhaseUpdater{}
			handler := handlers.NewInvestigatingHandler(
				mockClient, ctrl.Log.WithName("test-is-user-driving-timeout"), metrics.NewMetrics(), auditSpy,
				handlers.WithSessionMode(),
				handlers.WithRecorder(recorder),
				handlers.WithISPhaseUpdater(updater),
				handlers.WithMaxInvestigationDuration(1*time.Minute),
			)

			analysis := createAnalysisWithSession("rr-1376-ud-timeout", "session-ud-timeout-001")
			pastTime := metav1.NewTime(time.Now().Add(-2 * time.Hour))
			analysis.Status.KASession.CreatedAt = &pastTime

			mockClient.WithSessionPollStatus("user_driving")

			_, err := handler.Handle(ctx, analysis)
			Expect(err).NotTo(HaveOccurred())

			Expect(analysis.Status.Phase).To(Equal(aianalysis.PhaseFailed),
				"AA-CRIT-1: user_driving must NOT bypass MaxInvestigationDuration")

			calls := updater.getTerminalPhaseCalls()
			Expect(calls).To(HaveLen(1), "SetTerminalPhase should be called on user_driving timeout")
			Expect(calls[0].RRName).To(Equal("rr-1376-ud-timeout"))
			Expect(calls[0].Phase).To(Equal(isv1alpha1.SessionPhaseFailed))
		})
	})
})
