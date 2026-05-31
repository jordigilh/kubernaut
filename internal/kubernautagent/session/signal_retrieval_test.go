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

package session_test

import (
	"context"
	"github.com/go-logr/logr"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Signal Retrieval from Autonomous Session — PR5 Slice B", func() {

	Describe("UT-KA-PR5-B01: GetSessionContext returns typed context for existing session", func() {
		It("should return the full SessionContext including Signal for a running session", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)

			sctx := session.SessionContext{
				IncidentID:    "inc-b01",
				RemediationID: "rr-b01",
				Signal: katypes.SignalContext{
					Name:         "OOMKilled",
					Severity:     "critical",
					Environment:  "production",
					Priority:     "P1",
					ResourceKind: "Pod",
					ResourceName: "api-pod-abc",
					Namespace:    "default",
				},
			}

			id, err := manager.StartInvestigationWithContext(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, sctx)
			Expect(err).NotTo(HaveOccurred())

			retrieved, err := manager.GetSessionContext(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved).NotTo(BeNil())
			Expect(retrieved.IncidentID).To(Equal("inc-b01"))
			Expect(retrieved.RemediationID).To(Equal("rr-b01"))
			Expect(retrieved.Signal.Name).To(Equal("OOMKilled"))
			Expect(retrieved.Signal.Severity).To(Equal("critical"))
			Expect(retrieved.Signal.Environment).To(Equal("production"))
			Expect(retrieved.Signal.Priority).To(Equal("P1"))
			Expect(retrieved.Signal.ResourceKind).To(Equal("Pod"))
			Expect(retrieved.Signal.ResourceName).To(Equal("api-pod-abc"))
			Expect(retrieved.Signal.Namespace).To(Equal("default"))

			_ = manager.CancelInvestigation(id)
		})
	})

	Describe("UT-KA-PR5-B02: GetSessionContext returns error for non-existent session", func() {
		It("should return ErrSessionNotFound for unknown session ID", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)

			retrieved, err := manager.GetSessionContext("nonexistent-id")
			Expect(err).To(MatchError(session.ErrSessionNotFound))
			Expect(retrieved).To(BeNil())
		})
	})

	Describe("UT-KA-PR5-B03: GetSignalForRemediation retrieves signal via remediation_id lookup", func() {
		It("should find the autonomous session by rrID and return its SignalContext", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)

			sctx := session.SessionContext{
				IncidentID:    "inc-b03",
				RemediationID: "rr-b03-target",
				Signal: katypes.SignalContext{
					Name:             "HighLatency",
					Severity:         "warning",
					Environment:      "staging",
					BusinessCategory: "payment-service",
				},
			}

			_, err := manager.StartInvestigationWithContext(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, sctx)
			Expect(err).NotTo(HaveOccurred())

			signal, err := manager.GetSignalForRemediation("rr-b03-target")
			Expect(err).NotTo(HaveOccurred())
			Expect(signal).NotTo(BeNil())
			Expect(signal.Name).To(Equal("HighLatency"))
			Expect(signal.Severity).To(Equal("warning"))
			Expect(signal.Environment).To(Equal("staging"))
			Expect(signal.BusinessCategory).To(Equal("payment-service"))
		})
	})

	Describe("UT-KA-PR5-B04: GetSignalForRemediation returns error when no session found", func() {
		It("should return ErrSessionNotFound when no running session has the rrID", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)

			signal, err := manager.GetSignalForRemediation("nonexistent-rr")
			Expect(err).To(MatchError(session.ErrSessionNotFound))
			Expect(signal).To(BeNil())
		})
	})

	Describe("UT-KA-PR5-B05: GetSignalForRemediation gracefully handles session without Context", func() {
		It("should return ErrSessionNotFound when session has no typed context", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)

			// Start with old-style metadata (no SessionContext).
			// Production handler.go now always uses WithContext variants,
			// so sessions without a stored signal are not expected.
			metadata := map[string]string{
				"remediation_id": "rr-b05-legacy",
				"incident_id":    "inc-b05",
			}
			_, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, metadata)
			Expect(err).NotTo(HaveOccurred())

			_, err = manager.GetSignalForRemediation("rr-b05-legacy")
			Expect(err).To(MatchError(session.ErrSessionNotFound),
				"sessions without a stored signal should not be returned")
		})
	})
})
