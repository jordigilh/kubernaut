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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("BR-INTERACTIVE-010: InteractiveHold → StatusUserDriving — #1293", func() {

	Describe("UT-KA-1293-011: Investigation returning InteractiveHold=true transitions to StatusUserDriving", func() {
		It("should set session to StatusUserDriving instead of StatusCompleted", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					RCASummary:      "OOMKilled due to memory limit breach",
					Confidence:      0.92,
					InteractiveHold: true,
				}, nil
			}, map[string]string{"remediation_id": "rem-hold-011"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusUserDriving),
				"session must transition to StatusUserDriving when InteractiveHold=true")

			sess, _ := manager.GetSession(id)
			Expect(sess.Result).NotTo(BeNil())
			Expect(sess.Result.RCASummary).To(Equal("OOMKilled due to memory limit breach"))
			Expect(sess.Result.InteractiveHold).To(BeTrue())
		})
	})

	Describe("UT-KA-1293-012: Investigation returning InteractiveHold=false transitions to StatusCompleted", func() {
		It("should set session to StatusCompleted as normal", func() {
			store := session.NewStore(5 * time.Minute)
			manager := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
				return &katypes.InvestigationResult{
					RCASummary:      "Autonomous RCA complete",
					Confidence:      0.88,
					InteractiveHold: false,
				}, nil
			}, map[string]string{"remediation_id": "rem-auto-012"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				s, _ := manager.GetSession(id)
				if s == nil {
					return ""
				}
				return s.Status
			}, 2*time.Second, 50*time.Millisecond).Should(Equal(session.StatusCompleted),
				"session must transition to StatusCompleted when InteractiveHold=false")
		})
	})
})
