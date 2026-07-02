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
	kaaudit "github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// ========================================
// GAP-13 (Issue #1505): correlation ID propagation on the investigation
// context, so deep call sites (e.g. the K8s resolver's secret-access
// observer) can emit correctly-correlated audit events.
// ========================================

var _ = Describe("GAP-13: correlation ID propagation via launchInvestigation", func() {
	It("makes the RemediationID retrievable from the investigation context via audit.CorrelationIDFromContext", func() {
		store := session.NewStore(5 * time.Minute)
		manager := session.NewManager(store, logr.Discard(), nil, nil)

		var observedCorrelationID string
		var observedOK bool

		metadata := map[string]string{"remediation_id": "rr-1505-secret-audit"}

		id, err := manager.StartInvestigation(context.Background(), func(ctx context.Context) (*katypes.InvestigationResult, error) {
			observedCorrelationID, observedOK = kaaudit.CorrelationIDFromContext(ctx)
			return &katypes.InvestigationResult{}, nil
		}, metadata)
		Expect(err).NotTo(HaveOccurred())
		Expect(id).NotTo(BeEmpty())

		Eventually(func() session.Status {
			sess, err := manager.GetSession(id)
			if err != nil {
				return ""
			}
			return sess.Status
		}, 2*time.Second, 10*time.Millisecond).Should(Equal(session.StatusCompleted))

		Expect(observedOK).To(BeTrue(), "investigation context must carry a correlation ID")
		Expect(observedCorrelationID).To(Equal("rr-1505-secret-audit"))
	})
})
