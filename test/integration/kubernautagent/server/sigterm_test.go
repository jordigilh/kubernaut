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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/internal/kubernautagent/types"
)

var _ = Describe("Graceful Shutdown — SIGTERM — #823", func() {

	// IT-WIRE-SIGTERM: Manager.Shutdown cancels all active investigations.
	// This is an in-process test (not subprocess) that validates the
	// integration between Manager.Shutdown and active investigations.
	Describe("IT-WIRE-SIGTERM: Manager.Shutdown cancels running investigations", func() {
		It("all running sessions transition to cancelled on Shutdown", func() {
			h := newTestHarness()
			defer h.Close()

			ids := make([]string, 3)
			for i := 0; i < 3; i++ {
				id, err := h.Manager.StartInvestigation(context.Background(), func(ctx context.Context) (interface{}, error) {
					<-ctx.Done()
					return &katypes.InvestigationResult{RCASummary: "shutdown"}, nil
				}, map[string]string{"remediation_id": "rr-sigterm"})
				Expect(err).NotTo(HaveOccurred())
				ids[i] = id
			}

			for _, id := range ids {
				Eventually(func() session.Status {
					s, _ := h.Manager.GetSession(id)
					if s == nil {
						return session.StatusPending
					}
					return s.Status
				}, 5*time.Second).Should(Equal(session.StatusRunning))
			}

			h.Manager.Shutdown()

			for _, id := range ids {
				Eventually(func() session.Status {
					s, _ := h.Manager.GetSession(id)
					if s == nil {
						return session.StatusPending
					}
					return s.Status
				}, 5*time.Second).Should(Equal(session.StatusCancelled),
					"session %s should be cancelled after Shutdown", id)
			}
		})

		It("is idempotent -- second Shutdown does not panic", func() {
			h := newTestHarness()
			defer h.Close()

			h.Manager.Shutdown()
			Expect(func() { h.Manager.Shutdown() }).NotTo(Panic(),
				"Shutdown must be idempotent")
		})
	})
})
