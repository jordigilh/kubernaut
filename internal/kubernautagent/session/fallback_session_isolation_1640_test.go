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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// #1640: createFallbackSession's placeholder investigation (mode=
// interactive_fallback, canned RCASummary "Interactive session — awaiting
// user direction") must never be surfaced as if it were a real RCA result by
// GetLatestRCASummaryByRemediationID/GetLatestRCAResultByRemediationID, once
// its metadata key is corrected from "rr_id" to "remediation_id" (matching
// every other by-RR-ID lookup) so it becomes discoverable by
// FindUserDrivingByRemediationID.
var _ = Describe("Fallback session isolation from RCA-summary lookups — #1640", func() {

	Describe("UT-KA-1640-001: fallback-mode session is excluded from GetLatestRCASummaryByRemediationID", func() {
		It("should not return the fallback placeholder as a real RCA summary", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					RCASummary:      "Interactive session — awaiting user direction",
					InteractiveHold: true,
				}, nil
			}, map[string]string{
				"remediation_id": "rr-1640-001",
				"mode":           "interactive_fallback",
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}).Should(Equal(session.StatusUserDriving))

			_, found := mgr.GetLatestRCASummaryByRemediationID("rr-1640-001")
			Expect(found).To(BeFalse(),
				"UT-KA-1640-001: a fallback session's canned placeholder must never be mistaken for a real RCA summary")
		})
	})

	Describe("UT-KA-1640-002: fallback-mode session is excluded from GetLatestRCAResultByRemediationID", func() {
		It("should not return the fallback placeholder result", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					RCASummary:      "Interactive session — awaiting user direction",
					InteractiveHold: true,
				}, nil
			}, map[string]string{
				"remediation_id": "rr-1640-002",
				"mode":           "interactive_fallback",
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}).Should(Equal(session.StatusUserDriving))

			_, found := mgr.GetLatestRCAResultByRemediationID("rr-1640-002")
			Expect(found).To(BeFalse(),
				"UT-KA-1640-002: a fallback session's canned placeholder result must never leak into workflow-discovery's RCA-result lookup")
		})
	})

	Describe("UT-KA-1640-003: a genuine (non-fallback) session is still returned by both lookups", func() {
		It("regression guard: normal autonomous RCA summaries must still be found", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					RCASummary: "OOMKilled due to memory leak",
					Confidence: 0.9,
				}, nil
			}, map[string]string{
				"remediation_id": "rr-1640-003",
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}).Should(Equal(session.StatusCompleted))

			summary, found := mgr.GetLatestRCASummaryByRemediationID("rr-1640-003")
			Expect(found).To(BeTrue())
			Expect(summary).To(Equal("OOMKilled due to memory leak"))

			result, foundResult := mgr.GetLatestRCAResultByRemediationID("rr-1640-003")
			Expect(foundResult).To(BeTrue())
			Expect(result.RCASummary).To(Equal("OOMKilled due to memory leak"))
		})
	})

	Describe("UT-KA-1640-004: fallback session is discoverable by FindUserDrivingByRemediationID (regression guard for the actual bug)", func() {
		It("should find the fallback session once its metadata key matches the remediation_id convention", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					RCASummary:      "Interactive session — awaiting user direction",
					InteractiveHold: true,
				}, nil
			}, map[string]string{
				"remediation_id": "rr-1640-004",
				"mode":           "interactive_fallback",
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, _ := mgr.GetSession(id)
				if sess == nil {
					return ""
				}
				return sess.Status
			}).Should(Equal(session.StatusUserDriving))

			foundID, ok := mgr.FindUserDrivingByRemediationID("rr-1640-004")
			Expect(ok).To(BeTrue(),
				"UT-KA-1640-004: fallback sessions must be discoverable by FindUserDrivingByRemediationID once the metadata key matches (#1640)")
			Expect(foundID).To(Equal(id))
		})
	})
})
