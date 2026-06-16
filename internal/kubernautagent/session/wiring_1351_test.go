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

// IT-KA-1351-STORE: Proves SetResult + GetLatestRCA wiring through Manager

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("IT-KA-1351: Session wiring for user_driving guards", func() {

	Describe("IT-KA-1351-STORE: SetResult first-write-wins + overwrite guard (KA-HIGH-4, #1425)", func() {
		It("accepts first SetResult on UserDriving with nil result, blocks overwrite", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}, map[string]string{"remediation_id": "rr-guard-001"})
			Expect(err).NotTo(HaveOccurred())

			err = mgr.TransitionToUserDriving(id, "alice", nil)
			Expect(err).NotTo(HaveOccurred())

			first := &katypes.InvestigationResult{RCASummary: "preserved from investigation"}
			store.SetResult(id, first)

			sess, err := store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result).NotTo(BeNil(),
				"#1425: first SetResult on UserDriving with nil result must be accepted (first-write-wins)")
			Expect(sess.Result.RCASummary).To(Equal("preserved from investigation"))

			store.SetResult(id, &katypes.InvestigationResult{RCASummary: "stale overwrite"})

			sess, err = store.Get(id)
			Expect(err).NotTo(HaveOccurred())
			Expect(sess.Result.RCASummary).To(Equal("preserved from investigation"),
				"KA-HIGH-4: second SetResult on UserDriving must be blocked to prevent overwrite")
		})
	})

	Describe("IT-KA-1351-RCA: GetLatestRCAResultByRemediationID via Manager (KA-HIGH-5)", func() {
		It("returns completed RCA and excludes cancelled sessions", func() {
			store := session.NewStore(1 * time.Hour)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id1, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			store.SetMetadata(id1, map[string]string{"remediation_id": "rr-filter-001"})
			goodResult := &katypes.InvestigationResult{RCASummary: "good RCA"}
			store.SetResult(id1, goodResult)
			Expect(store.Update(id1, session.StatusCompleted, goodResult, nil)).To(Succeed())

			id2, err := store.Create()
			Expect(err).NotTo(HaveOccurred())
			store.SetMetadata(id2, map[string]string{"remediation_id": "rr-filter-001"})
			staleResult := &katypes.InvestigationResult{RCASummary: "stale from cancelled"}
			store.SetResult(id2, staleResult)
			Expect(store.Update(id2, session.StatusCancelled, staleResult, nil)).To(Succeed())

			result, found := mgr.GetLatestRCAResultByRemediationID("rr-filter-001")
			Expect(found).To(BeTrue())
			Expect(result.RCASummary).To(Equal("good RCA"),
				"GetLatestRCAResultByRemediationID must filter cancelled sessions (KA-HIGH-5)")
		})
	})
})
