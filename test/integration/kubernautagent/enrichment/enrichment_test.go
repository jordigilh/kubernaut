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
	"errors"
	"log/slog"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

type recordingAuditStore struct {
	events []*audit.AuditEvent
}

func (r *recordingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	r.events = append(r.events, event)
	return nil
}

type fakeK8sClient struct {
	ownerChain []enrichment.OwnerChainEntry
	err        error
}

func (f *fakeK8sClient) GetOwnerChain(_ context.Context, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return f.ownerChain, f.err
}

type fakeDataStorageClient struct {
	history []enrichment.RemediationHistoryEntry
	err     error
}

func (f *fakeDataStorageClient) GetRemediationHistory(_ context.Context, _, _, _, _ string) ([]enrichment.RemediationHistoryEntry, error) {
	return f.history, f.err
}

var _ = Describe("Kubernaut Agent Enrichment Integration — #433", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
	})

	Describe("IT-KA-433-010: Enricher resolves owner chain via K8s client", func() {
		It("should return the owner chain from the K8s client", func() {
			k8s := &fakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
					{Kind: "ReplicaSet", Name: "api-server-abc", Namespace: "production"},
					{Kind: "Pod", Name: "api-server-abc-xyz", Namespace: "production"},
				},
			}
			ds := &fakeDataStorageClient{
				history: []enrichment.RemediationHistoryEntry{},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			result, err := e.Enrich(context.Background(), "Pod", "api-server-abc-xyz", "production", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(), "Enrich should return a result")
			Expect(result.OwnerChain).To(HaveLen(3))
			Expect(result.OwnerChain[0].Kind).To(Equal("Deployment"))
		})
	})

	Describe("IT-KA-433-011: Enricher fetches remediation history via DataStorage client", func() {
		It("should return remediation history from the DataStorage client", func() {
			k8s := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "api-server", Namespace: "production"}}}
			ds := &fakeDataStorageClient{
				history: []enrichment.RemediationHistoryEntry{
					{WorkflowID: "oom-increase-memory", Outcome: "success", Timestamp: "2026-03-01T10:00:00Z"},
					{WorkflowID: "restart-pod", Outcome: "failure", Timestamp: "2026-02-28T15:30:00Z"},
				},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			result, err := e.Enrich(context.Background(), "Pod", "api-server-abc", "production", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationHistory).To(HaveLen(2))
			Expect(result.RemediationHistory[0].WorkflowID).To(Equal("oom-increase-memory"))
		})
	})

	Describe("IT-KA-433-012: Enricher handles partial failure gracefully", func() {
		It("should return partial results when owner chain fails but history succeeds", func() {
			k8s := &fakeK8sClient{err: errors.New("K8s API unavailable")}
			ds := &fakeDataStorageClient{
				history: []enrichment.RemediationHistoryEntry{
					{WorkflowID: "oom-increase-memory", Outcome: "success", Timestamp: "2026-03-01T10:00:00Z"},
				},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			result, err := e.Enrich(context.Background(), "Pod", "api-server-abc", "production", "")
			Expect(err).NotTo(HaveOccurred(), "partial failure should not return error")
			Expect(result).NotTo(BeNil())
			Expect(result.OwnerChain).To(BeEmpty(), "owner chain should be empty on failure")
			Expect(result.RemediationHistory).To(HaveLen(1), "history should still be populated")
		})
	})

	Describe("IT-KA-433-013: Enricher emits enrichment audit events", func() {
		It("should emit enrichment.completed on success", func() {
			k8s := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "api-server", Namespace: "production"}}}
			ds := &fakeDataStorageClient{history: []enrichment.RemediationHistoryEntry{}}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			_, err := e.Enrich(context.Background(), "Pod", "api-server-abc", "production", "")
			Expect(err).NotTo(HaveOccurred())

			eventTypes := make([]string, len(auditStore.events))
			for i, ev := range auditStore.events {
				eventTypes[i] = ev.EventType
			}
			Expect(eventTypes).To(ContainElement(audit.EventTypeEnrichmentCompleted),
				"enricher should emit enrichment.completed audit event")
		})

		It("should emit enrichment.failed when both clients fail", func() {
			k8s := &fakeK8sClient{err: errors.New("K8s down")}
			ds := &fakeDataStorageClient{err: errors.New("DS down")}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			_, err := e.Enrich(context.Background(), "Pod", "api-server-abc", "production", "")
			// Even on total failure, enrich should not error (best-effort)
			Expect(err).NotTo(HaveOccurred())

			eventTypes := make([]string, len(auditStore.events))
			for i, ev := range auditStore.events {
				eventTypes[i] = ev.EventType
			}
			Expect(eventTypes).To(ContainElement(audit.EventTypeEnrichmentFailed),
				"enricher should emit enrichment.failed when all clients fail")
		})
	})
})
