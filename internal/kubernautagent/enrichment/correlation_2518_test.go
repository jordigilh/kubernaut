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

package enrichment_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

type correlationCapturingAuditStore struct {
	events []*audit.AuditEvent
}

func (s *correlationCapturingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

var _ audit.AuditStore = (*correlationCapturingAuditStore)(nil)

var _ = Describe("Enrichment correlation preservation — #2518", func() {
	It("UT-KA-2521-006: uses the logical remediation correlation for enrichment audit events", func() {
		store := &correlationCapturingAuditStore{}
		enricher := enrichment.NewEnricher(
			&fakeK8sClient{specHash: "spec-1"},
			&fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}},
			store,
			logr.Discard(),
		)

		_, err := enricher.Enrich(context.Background(), enrichment.EnrichRequest{
			Kind:          "Deployment",
			Name:          "api",
			Namespace:     "prod",
			IncidentID:    "incident-1",
			CorrelationID: "rr-1",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(store.events).To(HaveLen(1))
		Expect(store.events[0].CorrelationID).To(Equal("rr-1"))
	})
})
