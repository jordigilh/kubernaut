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

var _ = Describe("Session Manager Panic Recovery — #1078", func() {

	Describe("UT-KA-1078-001: Panicking InvestigateFunc transitions session to StatusFailed", func() {
		It("should transition to StatusFailed when the investigation panics", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				panic("simulated nil pointer dereference in tool execution")
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, getErr := mgr.GetSession(id)
				if getErr != nil {
					return session.StatusPending
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusFailed),
				"panicking investigation must transition to StatusFailed, not remain StatusRunning")
		})
	})

	Describe("UT-KA-1078-002: Panic error message is preserved in session result error field", func() {
		It("should preserve the panic value in the session error", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				panic("custom panic message for forensic traceability")
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				sess, getErr := mgr.GetSession(id)
				if getErr != nil || sess.Error == nil {
					return ""
				}
				return sess.Error.Error()
			}, 2*time.Second, 10*time.Millisecond).Should(ContainSubstring("custom panic message"),
				"panic value must be preserved in session error for forensic traceability")
		})
	})

	Describe("UT-KA-1078-003: Panic with nil recover value transitions to StatusFailed", func() {
		It("should transition to StatusFailed even when panic value is nil", func() {
			store := session.NewStore(30 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), nil, nil)

			id, err := mgr.StartInvestigation(context.Background(), func(_ context.Context) (*katypes.InvestigationResult, error) {
				panic(nil)
			}, nil)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() session.Status {
				sess, getErr := mgr.GetSession(id)
				if getErr != nil {
					return session.StatusPending
				}
				return sess.Status
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusFailed),
				"nil panic must still transition to StatusFailed")
		})
	})
})
