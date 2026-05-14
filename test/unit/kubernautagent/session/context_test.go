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

var _ = Describe("SessionContext — PR5 Slice A", func() {

	Describe("UT-KA-PR5-A01: SessionContext.ToMap round-trip preserves all audit fields", func() {
		It("should produce a map with incident_id, remediation_id, created_by, signal_name, severity", func() {
			ctx := session.SessionContext{
				IncidentID:    "inc-001",
				RemediationID: "rr-001",
				CreatedBy:     "alice@corp",
				Signal: katypes.SignalContext{
					Name:     "OOMKilled",
					Severity: "critical",
				},
			}

			m := ctx.ToMap()
			Expect(m).To(HaveKeyWithValue("incident_id", "inc-001"))
			Expect(m).To(HaveKeyWithValue("remediation_id", "rr-001"))
			Expect(m).To(HaveKeyWithValue("created_by", "alice@corp"))
			Expect(m).To(HaveKeyWithValue("signal_name", "OOMKilled"))
			Expect(m).To(HaveKeyWithValue("severity", "critical"))
			Expect(m).To(HaveLen(5))
		})
	})

	Describe("UT-KA-PR5-A02: SessionContext.ToMap omits empty fields", func() {
		It("should not include keys for zero-value fields", func() {
			ctx := session.SessionContext{
				RemediationID: "rr-002",
			}

			m := ctx.ToMap()
			Expect(m).To(HaveKeyWithValue("remediation_id", "rr-002"))
			Expect(m).NotTo(HaveKey("incident_id"))
			Expect(m).NotTo(HaveKey("created_by"))
			Expect(m).NotTo(HaveKey("signal_name"))
			Expect(m).NotTo(HaveKey("severity"))
			Expect(m).To(HaveLen(1))
		})
	})

	Describe("UT-KA-PR5-A03: Session.Context stores and retrieves typed fields via SetContext", func() {
		It("should store SessionContext and make it retrievable via Get", func() {
			store := session.NewStore(5 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())

			store.SetContext(id, session.SessionContext{
				IncidentID:    "inc-typed",
				RemediationID: "rr-typed",
				CreatedBy:     "bob",
				Signal: katypes.SignalContext{
					Name:        "CrashLoop",
					Severity:    "warning",
					Environment: "staging",
					Priority:    "P2",
				},
			})

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Context.IncidentID).To(Equal("inc-typed"))
			Expect(sess.Context.RemediationID).To(Equal("rr-typed"))
			Expect(sess.Context.CreatedBy).To(Equal("bob"))
			Expect(sess.Context.Signal.Name).To(Equal("CrashLoop"))
			Expect(sess.Context.Signal.Severity).To(Equal("warning"))
			Expect(sess.Context.Signal.Environment).To(Equal("staging"))
			Expect(sess.Context.Signal.Priority).To(Equal("P2"))
		})
	})

	Describe("UT-KA-PR5-A04: clone isolates SessionContext (no shared mutable state)", func() {
		It("should not allow callers to mutate stored SessionContext via returned snapshot", func() {
			store := session.NewStore(5 * time.Minute)
			id, err := store.Create()
			Expect(err).NotTo(HaveOccurred())

			store.SetContext(id, session.SessionContext{
				IncidentID:    "inc-clone",
				RemediationID: "rr-clone",
				Signal: katypes.SignalContext{
					Name:     "OOMKilled",
					Severity: "critical",
				},
			})

			snap1, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			snap1.Context.IncidentID = "MUTATED"
			snap1.Context.Signal.Name = "MUTATED"

			snap2, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(snap2.Context.IncidentID).To(Equal("inc-clone"),
				"mutation of snapshot must not affect stored session")
			Expect(snap2.Context.Signal.Name).To(Equal("OOMKilled"),
				"mutation of signal in snapshot must not affect stored session")
		})
	})

	Describe("UT-KA-PR5-A05: Manager.StartInvestigationWithContext propagates SessionContext", func() {
		It("should store SessionContext and make it retrievable after investigation starts", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)

			signal := katypes.SignalContext{
				Name:         "OOMKilled",
				Namespace:    "production",
				Severity:     "critical",
				Message:      "Container exceeded memory limit",
				Environment:  "prod",
				Priority:     "P1",
				ResourceKind: "Pod",
				ResourceName: "api-server-xyz",
			}

			sctx := session.SessionContext{
				IncidentID:    "inc-mgr-001",
				RemediationID: "rr-mgr-001",
				Signal:        signal,
			}

			id, err := manager.StartInvestigationWithContext(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, sctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(id).NotTo(BeEmpty())

			sess, err := manager.GetSession(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Context.IncidentID).To(Equal("inc-mgr-001"))
			Expect(sess.Context.RemediationID).To(Equal("rr-mgr-001"))
			Expect(sess.Context.Signal.Name).To(Equal("OOMKilled"))
			Expect(sess.Context.Signal.Environment).To(Equal("prod"))
			Expect(sess.Context.Signal.ResourceKind).To(Equal("Pod"))

			_ = manager.CancelInvestigation(id)
		})
	})
})
