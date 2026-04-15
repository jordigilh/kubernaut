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
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

var _ = Describe("Kubernaut Agent Enrichment — #433", func() {

	Describe("UT-KA-433-028: EnrichmentResult serializes owner chain + history", func() {
		It("should round-trip serialize baseline enrichment fields", func() {
			original := enrichment.EnrichmentResult{
				OwnerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "default"},
					{Kind: "ReplicaSet", Name: "api-server-abc123", Namespace: "default"},
				},
				RemediationHistory: &enrichment.RemediationHistoryResult{
					TargetResource:     "default/Deployment/api-server",
					RegressionDetected: false,
					Tier1: []enrichment.Tier1Entry{
						{RemediationUID: "oom-increase-memory", Outcome: "success", CompletedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)},
						{RemediationUID: "restart-pod", Outcome: "failure", CompletedAt: time.Date(2026, 2, 28, 15, 30, 0, 0, time.UTC)},
					},
				},
			}

			data, err := json.Marshal(original)
			Expect(err).NotTo(HaveOccurred())

			var restored enrichment.EnrichmentResult
			err = json.Unmarshal(data, &restored)
			Expect(err).NotTo(HaveOccurred())

			Expect(restored.OwnerChain).To(HaveLen(2))
			Expect(restored.OwnerChain[0].Kind).To(Equal("Deployment"))
			Expect(restored.OwnerChain[0].Name).To(Equal("api-server"))
			Expect(restored.OwnerChain[1].Kind).To(Equal("ReplicaSet"))
			Expect(restored.RemediationHistory).NotTo(BeNil())
			Expect(restored.RemediationHistory.Tier1).To(HaveLen(2))
			Expect(restored.RemediationHistory.Tier1[0].RemediationUID).To(Equal("oom-increase-memory"))
			Expect(restored.RemediationHistory.Tier1[1].Outcome).To(Equal("failure"))
		})
	})

	Describe("UT-KA-433-130: OwnerChainEntry struct has Kind, Name, Namespace fields", func() {
		It("should serialize OwnerChainEntry with all fields", func() {
			entry := enrichment.OwnerChainEntry{
				Kind:      "Deployment",
				Name:      "api-server",
				Namespace: "production",
			}
			data, err := json.Marshal(entry)
			Expect(err).NotTo(HaveOccurred())

			var restored enrichment.OwnerChainEntry
			Expect(json.Unmarshal(data, &restored)).To(Succeed())
			Expect(restored.Kind).To(Equal("Deployment"))
			Expect(restored.Name).To(Equal("api-server"))
			Expect(restored.Namespace).To(Equal("production"))
		})

		It("should omit namespace when empty (cluster-scoped)", func() {
			entry := enrichment.OwnerChainEntry{Kind: "Node", Name: "worker-1"}
			data, err := json.Marshal(entry)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("namespace"))
		})
	})

	Describe("UT-KA-433-131: DetectedLabels struct matches KA LabelDetector output", func() {
		It("should round-trip serialize all 10 DetectedLabels fields", func() {
			labels := enrichment.DetectedLabels{
				FailedDetections:         []string{"hpaEnabled"},
				GitOpsManaged:            true,
				GitOpsTool:               "argocd",
				PDBProtected:             true,
				HPAEnabled:               false,
				Stateful:                 false,
				HelmManaged:              true,
				NetworkIsolated:          false,
				ServiceMesh:              "istio",
				ResourceQuotaConstrained: true,
			}

			data, err := json.Marshal(labels)
			Expect(err).NotTo(HaveOccurred())

			var restored enrichment.DetectedLabels
			Expect(json.Unmarshal(data, &restored)).To(Succeed())
			Expect(restored.GitOpsManaged).To(BeTrue())
			Expect(restored.GitOpsTool).To(Equal("argocd"))
			Expect(restored.PDBProtected).To(BeTrue())
			Expect(restored.HPAEnabled).To(BeFalse())
			Expect(restored.Stateful).To(BeFalse())
			Expect(restored.HelmManaged).To(BeTrue())
			Expect(restored.NetworkIsolated).To(BeFalse())
			Expect(restored.ServiceMesh).To(Equal("istio"))
			Expect(restored.ResourceQuotaConstrained).To(BeTrue())
			Expect(restored.FailedDetections).To(ConsistOf("hpaEnabled"))
		})

		It("should default boolean fields to false and string fields to empty", func() {
			var labels enrichment.DetectedLabels
			Expect(labels.GitOpsManaged).To(BeFalse())
			Expect(labels.GitOpsTool).To(BeEmpty())
			Expect(labels.PDBProtected).To(BeFalse())
			Expect(labels.ServiceMesh).To(BeEmpty())
			Expect(labels.FailedDetections).To(BeNil())
		})
	})

	Describe("UT-KA-433-132: EnrichmentResult includes DetectedLabels and QuotaDetails", func() {
		It("should serialize EnrichmentResult with all enrichment fields", func() {
			result := enrichment.EnrichmentResult{
				OwnerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				},
				DetectedLabels: &enrichment.DetectedLabels{
					GitOpsManaged: true,
					GitOpsTool:    "argocd",
					PDBProtected:  true,
				},
				QuotaDetails: map[string]string{
					"cpu_hard":    "4",
					"memory_hard": "8Gi",
				},
				RemediationHistory: &enrichment.RemediationHistoryResult{
					Tier1: []enrichment.Tier1Entry{
						{RemediationUID: "oom-recovery", Outcome: "success", CompletedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)},
					},
				},
			}

			data, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())

			var restored enrichment.EnrichmentResult
			Expect(json.Unmarshal(data, &restored)).To(Succeed())

			Expect(restored.OwnerChain).To(HaveLen(1))
			Expect(restored.OwnerChain[0].Kind).To(Equal("Deployment"))
			Expect(restored.DetectedLabels).NotTo(BeNil())
			Expect(restored.DetectedLabels.GitOpsManaged).To(BeTrue())
			Expect(restored.DetectedLabels.GitOpsTool).To(Equal("argocd"))
			Expect(restored.QuotaDetails).To(HaveKeyWithValue("cpu_hard", "4"))
			Expect(restored.QuotaDetails).To(HaveKeyWithValue("memory_hard", "8Gi"))
			Expect(restored.RemediationHistory).NotTo(BeNil())
			Expect(restored.RemediationHistory.Tier1).To(HaveLen(1))
		})

		It("should omit DetectedLabels and QuotaDetails when nil", func() {
			result := enrichment.EnrichmentResult{
				OwnerChain: []enrichment.OwnerChainEntry{{Kind: "Pod", Name: "web", Namespace: "default"}},
			}
			data, err := json.Marshal(result)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring("detected_labels"))
			Expect(string(data)).NotTo(ContainSubstring("quota_details"))
		})
	})

	Describe("UT-KA-433-133: DataStorageClient accepts kind and specHash parameters", func() {
		It("should compile with kind, name, namespace, specHash parameters", func() {
			var client enrichment.DataStorageClient = &fakeDS{}
			result, err := client.GetRemediationHistory(nil, "Deployment", "api-server", "production", "abc123hash")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})
	})

	Describe("UT-KA-433-134: K8sClient.GetOwnerChain returns []OwnerChainEntry", func() {
		It("should compile with []OwnerChainEntry return type", func() {
			var client enrichment.K8sClient = &fakeK8s{}
			chain, err := client.GetOwnerChain(nil, "Pod", "web-abc", "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(1))
			Expect(chain[0].Kind).To(Equal("Deployment"))
		})
	})
})

var _ = Describe("Kubernaut Agent Enricher Coordination — #433 (reclassified from IT)", func() {

	var (
		logger     *slog.Logger
		auditStore *recordingAuditStore
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		auditStore = &recordingAuditStore{}
	})

	Describe("UT-KA-433-010: Enricher resolves owner chain via K8s client", func() {
		It("should return the owner chain from the K8s client", func() {
			k8s := &fakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{
					{Kind: "Deployment", Name: "api-server", Namespace: "production"},
					{Kind: "ReplicaSet", Name: "api-server-abc", Namespace: "production"},
					{Kind: "Pod", Name: "api-server-abc-xyz", Namespace: "production"},
				},
			}
			ds := &fakeDataStorageClient{
				history: &enrichment.RemediationHistoryResult{},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			result, err := e.Enrich(context.Background(), "Pod", "api-server-abc-xyz", "production", "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(), "Enrich should return a result")
			Expect(result.OwnerChain).To(HaveLen(3))
			Expect(result.OwnerChain[0].Kind).To(Equal("Deployment"))
		})
	})

	Describe("UT-KA-433-011: Enricher fetches remediation history via DataStorage client", func() {
		It("should return remediation history from the DataStorage client", func() {
			k8s := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}}
			ds := &fakeDataStorageClient{
				history: &enrichment.RemediationHistoryResult{
					Tier1: []enrichment.Tier1Entry{
						{RemediationUID: "oom-increase-memory", Outcome: "success", CompletedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)},
						{RemediationUID: "restart-pod", Outcome: "failure", CompletedAt: time.Date(2026, 2, 28, 15, 30, 0, 0, time.UTC)},
					},
				},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			result, err := e.Enrich(context.Background(), "Pod", "api-server-abc", "production", "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RemediationHistory).NotTo(BeNil())
			Expect(result.RemediationHistory.Tier1).To(HaveLen(2))
			Expect(result.RemediationHistory.Tier1[0].RemediationUID).To(Equal("oom-increase-memory"))
		})
	})

	Describe("UT-KA-433-012: Enricher handles partial failure gracefully", func() {
		It("should return partial results when owner chain fails but history succeeds", func() {
			k8s := &fakeK8sClient{err: errors.New("K8s API unavailable")}
			ds := &fakeDataStorageClient{
				history: &enrichment.RemediationHistoryResult{
					Tier1: []enrichment.Tier1Entry{
						{RemediationUID: "oom-increase-memory", Outcome: "success", CompletedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)},
					},
				},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			result, err := e.Enrich(context.Background(), "Pod", "api-server-abc", "production", "", "")
			Expect(err).NotTo(HaveOccurred(), "partial failure should not return error")
			Expect(result).NotTo(BeNil())
			Expect(result.OwnerChain).To(BeEmpty(), "owner chain should be empty on failure")
			Expect(result.RemediationHistory).NotTo(BeNil())
			Expect(result.RemediationHistory.Tier1).To(HaveLen(1), "history should still be populated")
		})
	})

	Describe("UT-KA-433-014: Enricher auto-computes specHash when caller passes empty string", func() {
		It("should call GetSpecHash and forward computed hash to DS", func() {
			k8s := &fakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{},
				specHash:   "sha256:auto-computed-hash",
			}
			ds := &fakeDataStorageClient{
				history: &enrichment.RemediationHistoryResult{},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			_, err := e.Enrich(context.Background(), "Deployment", "api-server", "default", "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(ds.capturedSpecHash).To(Equal("sha256:auto-computed-hash"),
				"enricher should auto-compute specHash when empty and forward to DS")
		})

		It("should use caller-provided specHash without calling GetSpecHash", func() {
			k8s := &fakeK8sClient{
				ownerChain: []enrichment.OwnerChainEntry{},
				specHash:   "sha256:should-not-be-used",
			}
			ds := &fakeDataStorageClient{
				history: &enrichment.RemediationHistoryResult{},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			_, err := e.Enrich(context.Background(), "Deployment", "api-server", "default", "sha256:caller-provided", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(ds.capturedSpecHash).To(Equal("sha256:caller-provided"),
				"enricher should not override caller-provided specHash")
		})

		It("should proceed with empty specHash when GetSpecHash fails (graceful degradation)", func() {
			k8s := &fakeK8sClient{
				ownerChain:  []enrichment.OwnerChainEntry{},
				specHashErr: errors.New("K8s API unavailable"),
			}
			ds := &fakeDataStorageClient{
				history: &enrichment.RemediationHistoryResult{},
			}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			result, err := e.Enrich(context.Background(), "Deployment", "api-server", "default", "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(ds.capturedSpecHash).To(BeEmpty(),
				"enricher should proceed with empty specHash on failure")
		})
	})

	Describe("UT-KA-693-004: Enrich populates resource identity on result", func() {
		It("should set ResourceKind, ResourceName, ResourceNamespace from input params", func() {
			k8s := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{}}
			ds := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{}}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			result, err := e.Enrich(context.Background(), "Deployment", "worker", "demo-crashloop", "", "inc-1")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.ResourceKind).To(Equal("Deployment"),
				"UT-KA-693-004: Enrich must populate ResourceKind from input params")
			Expect(result.ResourceName).To(Equal("worker"),
				"UT-KA-693-004: Enrich must populate ResourceName from input params")
			Expect(result.ResourceNamespace).To(Equal("demo-crashloop"),
				"UT-KA-693-004: Enrich must populate ResourceNamespace from input params")
		})
	})

	Describe("UT-KA-433-013: Enricher emits enrichment audit events", func() {
		It("should emit enrichment.completed with structured EventData on success", func() {
			k8s := &fakeK8sClient{ownerChain: []enrichment.OwnerChainEntry{
				{Kind: "ReplicaSet", Name: "api-server-abc", Namespace: "production"},
				{Kind: "Deployment", Name: "api-server", Namespace: "production"},
			}}
			ds := &fakeDataStorageClient{history: &enrichment.RemediationHistoryResult{
				Tier1: []enrichment.Tier1Entry{
					{RemediationUID: "oom-recovery", Outcome: "success", CompletedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)},
				},
			}}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			_, err := e.Enrich(context.Background(), "Pod", "api-server-abc", "production", "", "test-incident-001")
			Expect(err).NotTo(HaveOccurred())

			Expect(auditStore.events).To(HaveLen(1))
			ev := auditStore.events[0]
			Expect(ev.EventType).To(Equal(audit.EventTypeEnrichmentCompleted))
			Expect(ev.EventAction).To(Equal("enriched"))
			Expect(ev.EventOutcome).To(Equal("success"))
			Expect(ev.CorrelationID).To(Equal("test-incident-001"))
			Expect(ev.Data["incident_id"]).To(Equal("test-incident-001"))
			Expect(ev.Data["root_owner_kind"]).To(Equal("Deployment"))
			Expect(ev.Data["root_owner_name"]).To(Equal("api-server"))
			Expect(ev.Data["root_owner_namespace"]).To(Equal("production"))
			Expect(ev.Data["owner_chain_length"]).To(Equal(2))
			Expect(ev.Data["remediation_history_fetched"]).To(BeTrue())
			Expect(ev.Data["event_id"]).NotTo(BeEmpty())
		})

		It("should emit enrichment.failed with structured EventData when both clients fail", func() {
			k8s := &fakeK8sClient{err: errors.New("K8s down")}
			ds := &fakeDataStorageClient{err: errors.New("DS down")}
			e := enrichment.NewEnricher(k8s, ds, auditStore, logger)
			_, err := e.Enrich(context.Background(), "Pod", "api-server-abc", "production", "", "test-incident-002")
			Expect(err).NotTo(HaveOccurred())

			Expect(auditStore.events).To(HaveLen(1))
			ev := auditStore.events[0]
			Expect(ev.EventType).To(Equal(audit.EventTypeEnrichmentFailed))
			Expect(ev.EventAction).To(Equal("enriched"))
			Expect(ev.EventOutcome).To(Equal("failure"))
			Expect(ev.CorrelationID).To(Equal("test-incident-002"))
			Expect(ev.Data["incident_id"]).To(Equal("test-incident-002"))
			Expect(ev.Data["reason"]).To(Equal("all_enrichment_sources_failed"))
			Expect(ev.Data["detail"]).To(ContainSubstring("K8s down"))
			Expect(ev.Data["detail"]).To(ContainSubstring("DS down"))
			Expect(ev.Data["affected_resource_kind"]).To(Equal("Pod"))
			Expect(ev.Data["affected_resource_name"]).To(Equal("api-server-abc"))
			Expect(ev.Data["affected_resource_namespace"]).To(Equal("production"))
			Expect(ev.Data["event_id"]).NotTo(BeEmpty())
		})
	})
})

// fakeDS satisfies the updated DataStorageClient interface for compile-time verification.
type fakeDS struct{}

func (f *fakeDS) GetRemediationHistory(_ context.Context, _, _, _, _ string) (*enrichment.RemediationHistoryResult, error) {
	return &enrichment.RemediationHistoryResult{}, nil
}

// fakeK8s satisfies the updated K8sClient interface for compile-time verification.
type fakeK8s struct{}

func (f *fakeK8s) GetOwnerChain(_ context.Context, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return []enrichment.OwnerChainEntry{{Kind: "Deployment", Name: "api-server", Namespace: "default"}}, nil
}

func (f *fakeK8s) GetSpecHash(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}

// recordingAuditStore captures audit events for assertion.
type recordingAuditStore struct {
	events []*audit.AuditEvent
}

func (r *recordingAuditStore) StoreAudit(_ context.Context, event *audit.AuditEvent) error {
	r.events = append(r.events, event)
	return nil
}

// fakeK8sClient is a configurable K8sClient stub for enricher coordination tests.
type fakeK8sClient struct {
	ownerChain  []enrichment.OwnerChainEntry
	specHash    string
	specHashErr error
	err         error
}

func (f *fakeK8sClient) GetOwnerChain(_ context.Context, _, _, _ string) ([]enrichment.OwnerChainEntry, error) {
	return f.ownerChain, f.err
}

func (f *fakeK8sClient) GetSpecHash(_ context.Context, _, _, _ string) (string, error) {
	return f.specHash, f.specHashErr
}

// fakeDataStorageClient is a configurable DataStorageClient stub for enricher coordination tests.
type fakeDataStorageClient struct {
	history          *enrichment.RemediationHistoryResult
	err              error
	capturedSpecHash string
}

func (f *fakeDataStorageClient) GetRemediationHistory(_ context.Context, _, _, _, specHash string) (*enrichment.RemediationHistoryResult, error) {
	f.capturedSpecHash = specHash
	return f.history, f.err
}
