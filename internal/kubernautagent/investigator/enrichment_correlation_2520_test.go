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

package investigator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

var _ = Describe("Enrichment audit correlation — #2520-2521", func() {
	It("IT-KA-AUDIT-2520-001: uses remediation correlation when context differs", func() {
		rcaClient := newPhaseScriptedRCA("sonnet")
		resolver := newRecordingPhaseResolver(rcaClient, "default-model")
		resolver.addPhase(katypes.PhaseRCA, rcaClient, "sonnet")
		workflowClient := &pinSubmitClient{}
		resolver.addPhase(katypes.PhaseWorkflowDiscovery, workflowClient, "haiku")

		store := &phaseAuditStore{}
		inv := buildInvestigatorWithResolver(rcaClient, store, resolver)
		_, err := inv.Investigate(context.Background(), katypes.SignalContext{
			Name:          "test-pod",
			Namespace:     "default",
			Severity:      "high",
			Message:       "OOMKilled",
			IncidentID:    "incident-2520-001",
			RemediationID: "remediation-2520-001",
		})
		Expect(err).NotTo(HaveOccurred())

		store.mu.Lock()
		defer store.mu.Unlock()
		var enrichmentEvent *audit.AuditEvent
		for _, event := range store.events {
			if event.EventType == audit.EventTypeEnrichmentCompleted {
				enrichmentEvent = event
				break
			}
		}
		Expect(enrichmentEvent).NotTo(BeNil(), "enrichment must emit a completed audit event")
		Expect(enrichmentEvent.CorrelationID).To(Equal("remediation-2520-001"),
			"enrichment correlation must join the remediation audit timeline")
	})
})
