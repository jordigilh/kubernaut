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

package server_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/server"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	"github.com/jordigilh/kubernaut/pkg/agentclient"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("BR-INTERACTIVE-010: Interactive Signal Mapping — #1293", func() {

	Describe("UT-KA-1293-001: MapIncidentRequestToSignal maps interactive=true to SignalContext", func() {
		It("should set Interactive=true on SignalContext when request has interactive=true", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "int-test-001",
				RemediationID:     "rem-interactive-001",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityCritical,
				ResourceNamespace: "prod",
				ResourceKind:      "Pod",
				ResourceName:      "api-server",
				ErrorMessage:      "OOMKilled",
				Environment:       "production",
				Priority:          "high",
				RiskTolerance:     "medium",
				BusinessCategory:  "core",
				ClusterName:       "prod-1",
				SignalSource:      "kubernetes",
			}
			req.Interactive.SetTo(true)

			signal := server.MapIncidentRequestToSignal(req)
			Expect(signal.Interactive).To(BeTrue(),
				"SignalContext.Interactive must be true when request.Interactive is set to true")
		})
	})

	Describe("UT-KA-1293-002: MapIncidentRequestToSignal defaults interactive=false", func() {
		It("should leave Interactive=false on SignalContext when request omits interactive", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "int-test-002",
				RemediationID:     "rem-interactive-002",
				SignalName:        "CrashLoopBackOff",
				Severity:          agentclient.SeverityHigh,
				ResourceNamespace: "default",
				ResourceKind:      "Pod",
				ResourceName:      "worker",
				ErrorMessage:      "CrashLoopBackOff",
				Environment:       "staging",
				Priority:          "medium",
				RiskTolerance:     "medium",
				BusinessCategory:  "platform",
				ClusterName:       "staging-1",
				SignalSource:      "kubernetes",
			}

			signal := server.MapIncidentRequestToSignal(req)
			Expect(signal.Interactive).To(BeFalse(),
				"SignalContext.Interactive must default to false when not set in request")
		})
	})

	Describe("UT-KA-1293-003: MapIncidentRequestToSignal maps interactive=false explicitly", func() {
		It("should leave Interactive=false on SignalContext when request has interactive=false", func() {
			req := &agentclient.IncidentRequest{
				IncidentID:        "int-test-003",
				RemediationID:     "rem-interactive-003",
				SignalName:        "HighMemory",
				Severity:          agentclient.SeverityMedium,
				ResourceNamespace: "prod",
				ResourceKind:      "Deployment",
				ResourceName:      "api",
				ErrorMessage:      "Memory above 90%",
				Environment:       "production",
				Priority:          "medium",
				RiskTolerance:     "medium",
				BusinessCategory:  "core",
				ClusterName:       "prod-1",
				SignalSource:      "prometheus",
			}
			req.Interactive.SetTo(false)

			signal := server.MapIncidentRequestToSignal(req)
			Expect(signal.Interactive).To(BeFalse(),
				"SignalContext.Interactive must be false when explicitly set to false")
		})
	})

	Describe("UT-KA-1293-004: Handler creates interactive session in pending state", func() {
		It("should create session in StatusPending without launching investigation goroutine", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)
			investigator := &interactiveTestInvestigator{called: false}
			handler := server.NewHandler(manager, investigator, logr.Discard(), nil)

			req := &agentclient.IncidentRequest{
				IncidentID:        "int-test-004",
				RemediationID:     "rem-interactive-004",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityCritical,
				ResourceNamespace: "prod",
				ResourceKind:      "Pod",
				ResourceName:      "api-server",
				ErrorMessage:      "OOMKilled",
				Environment:       "production",
				Priority:          "high",
				RiskTolerance:     "medium",
				BusinessCategory:  "core",
				ClusterName:       "prod-1",
				SignalSource:      "kubernetes",
			}
			req.Interactive.SetTo(true)

			resp, err := handler.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			accepted, ok := resp.(*agentclient.AnalyzeAccepted)
			Expect(ok).To(BeTrue(), "expected AnalyzeAccepted for interactive submission")
			sessionID := accepted.SessionID.String()
			Expect(sessionID).NotTo(BeEmpty())

			// Session should remain in pending state — investigation not launched
			sess, sErr := manager.GetSession(sessionID)
			Expect(sErr).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusPending),
				"interactive session must remain in StatusPending until MCP action=start")

			// Give time to confirm investigator was NOT called
			Consistently(func() bool {
				return investigator.called
			}, 200*time.Millisecond, 50*time.Millisecond).Should(BeFalse(),
				"Investigate() must NOT be called for interactive sessions")
		})
	})

	Describe("UT-KA-1293-005: Handler launches investigation normally for non-interactive requests", func() {
		It("should create session in StatusRunning and call Investigate for autonomous flow", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)
			investigator := &interactiveTestInvestigator{called: false}
			handler := server.NewHandler(manager, investigator, logr.Discard(), nil)

			req := &agentclient.IncidentRequest{
				IncidentID:        "int-test-005",
				RemediationID:     "rem-autonomous-005",
				SignalName:        "OOMKilled",
				Severity:          agentclient.SeverityCritical,
				ResourceNamespace: "prod",
				ResourceKind:      "Pod",
				ResourceName:      "api-server",
				ErrorMessage:      "OOMKilled",
				Environment:       "production",
				Priority:          "high",
				RiskTolerance:     "medium",
				BusinessCategory:  "core",
				ClusterName:       "prod-1",
				SignalSource:      "kubernetes",
			}

			resp, err := handler.IncidentAnalyzeEndpointAPIV1IncidentAnalyzePost(context.Background(), req)
			Expect(err).NotTo(HaveOccurred())

			accepted, ok := resp.(*agentclient.AnalyzeAccepted)
			Expect(ok).To(BeTrue(), "expected AnalyzeAccepted for autonomous submission")
			sessionID := accepted.SessionID.String()
			Expect(sessionID).NotTo(BeEmpty())

			// Session should transition to running — investigation launched
			Eventually(func() session.Status {
				sess, _ := manager.GetSession(sessionID)
				if sess == nil {
					return ""
				}
				return sess.Status
			}, 2*time.Second, 50*time.Millisecond).Should(
				SatisfyAny(Equal(session.StatusRunning), Equal(session.StatusCompleted)),
				"autonomous session must transition to Running/Completed")

			Eventually(func() bool {
				return investigator.called
			}, 2*time.Second, 50*time.Millisecond).Should(BeTrue(),
				"Investigate() must be called for autonomous sessions")
		})
	})

	Describe("UT-KA-1293-012: mapSessionStatusToAPI returns pending for StatusPending", func() {
		It("should return status 'pending' from the session status endpoint", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)
			handler := server.NewHandler(manager, nil, logr.Discard(), nil)

			id, err := manager.StartInteractiveSession(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{RCASummary: "test RCA", Confidence: 0.85}, nil
			}, map[string]string{"remediation_id": "rem-pending-012"})
			Expect(err).NotTo(HaveOccurred())

			sess, sErr := manager.GetSession(id)
			Expect(sErr).NotTo(HaveOccurred())
			Expect(sess.Status).To(Equal(session.StatusPending),
				"StartInteractiveSession must create session in StatusPending")

			params := agentclient.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGetParams{SessionID: id}
			resp, err := handler.IncidentSessionStatusEndpointAPIV1IncidentSessionSessionIDGet(context.Background(), params)
			Expect(err).NotTo(HaveOccurred())

			ss, ok := resp.(*agentclient.SessionStatus)
			Expect(ok).To(BeTrue(), "response should be *SessionStatus")
			Expect(ss.Status).To(Equal("pending"),
				"mapSessionStatusToAPI must map StatusPending to 'pending'")
		})
	})
})

// interactiveTestInvestigator implements the Investigator interface for testing
// interactive vs autonomous session routing.
type interactiveTestInvestigator struct {
	called bool
}

func (i *interactiveTestInvestigator) Investigate(_ context.Context, _ katypes.SignalContext) (*katypes.InvestigationResult, error) {
	i.called = true
	return &katypes.InvestigationResult{
		RCASummary: "test RCA",
		Confidence: 0.85,
	}, nil
}
