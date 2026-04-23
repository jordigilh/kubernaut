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
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

var _ = Describe("DataStorage Adapter — TP-433-WIR Phase 1a", func() {

	Describe("UT-KA-433W-001: DS adapter maps ogen Tier1 + Tier2 to enrichment domain types", func() {
		It("should map all Tier1 fields including HealthChecks and MetricDeltas", func() {
			score := 0.85
			resolved := true
			client := &stubDSClient{
				response: &ogenclient.RemediationHistoryContext{
					TargetResource:     "default/Deployment/api-server",
					CurrentSpecHash:    "abc123",
					RegressionDetected: true,
					Tier1: ogenclient.RemediationHistoryTier1{
						Window: "24h",
						Chain: []ogenclient.RemediationHistoryEntry{
							{
								RemediationUID:          "wf-oom-recovery-001",
								Outcome:                 ogenclient.NewOptString("Success"),
								CompletedAt:             time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
								ActionType:              ogenclient.NewOptNilString("increase_memory"),
								SignalType:              ogenclient.NewOptString("OOMKilled"),
								EffectivenessScore:      ogenclient.OptNilFloat64{Value: score, Set: true},
								SignalResolved:          ogenclient.OptNilBool{Value: resolved, Set: true},
								HashMatch:               ogenclient.NewOptRemediationHistoryEntryHashMatch(ogenclient.RemediationHistoryEntryHashMatchPreRemediation),
								PreRemediationSpecHash:  ogenclient.NewOptString("sha256:aaa"),
								PostRemediationSpecHash: ogenclient.NewOptString("sha256:bbb"),
								AssessmentReason:        ogenclient.NewOptNilRemediationHistoryEntryAssessmentReason(ogenclient.RemediationHistoryEntryAssessmentReasonFull),
								HealthChecks: ogenclient.NewOptRemediationHealthChecks(ogenclient.RemediationHealthChecks{
									PodRunning:    ogenclient.NewOptBool(true),
									ReadinessPass: ogenclient.NewOptBool(true),
									RestartDelta:  ogenclient.NewOptInt(0),
								}),
								MetricDeltas: ogenclient.NewOptRemediationMetricDeltas(ogenclient.RemediationMetricDeltas{
									CpuBefore: ogenclient.NewOptFloat64(0.85),
									CpuAfter:  ogenclient.NewOptFloat64(0.45),
								}),
							},
						},
					},
					Tier2: ogenclient.RemediationHistoryTier2{
						Window: "2160h",
						Chain: []ogenclient.RemediationHistorySummary{
							{
								RemediationUID:     "wf-old-001",
								Outcome:            ogenclient.NewOptString("Failed"),
								SignalType:          ogenclient.NewOptString("HighCPU"),
								ActionType:          ogenclient.NewOptNilString("restart_pod"),
								EffectivenessScore: ogenclient.OptNilFloat64{Value: 0.2, Set: true},
								CompletedAt:        time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			}

			adapter := enrichment.NewDSAdapter(client)
			result, err := adapter.GetRemediationHistory(context.Background(), "Deployment", "api-server", "default", "abc123")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.TargetResource).To(Equal("default/Deployment/api-server"))
			Expect(result.RegressionDetected).To(BeTrue())
			Expect(result.Tier1Window).To(Equal("24h"))
			Expect(result.Tier2Window).To(Equal("2160h"))

			By("Verifying Tier1 entry")
			Expect(result.Tier1).To(HaveLen(1))
			t1 := result.Tier1[0]
			Expect(t1.RemediationUID).To(Equal("wf-oom-recovery-001"))
			Expect(t1.Outcome).To(Equal("Success"))
			Expect(t1.ActionType).To(Equal("increase_memory"))
			Expect(t1.SignalType).To(Equal("OOMKilled"))
			Expect(*t1.EffectivenessScore).To(BeNumerically("~", 0.85))
			Expect(*t1.SignalResolved).To(BeTrue())
			Expect(t1.HashMatch).To(Equal("preRemediation"))
			Expect(t1.PreRemediationSpecHash).To(Equal("sha256:aaa"))
			Expect(t1.PostRemediationSpecHash).To(Equal("sha256:bbb"))
			Expect(t1.AssessmentReason).To(Equal("Full"))
			Expect(t1.HealthChecks).NotTo(BeNil())
			Expect(*t1.HealthChecks.PodRunning).To(BeTrue())
			Expect(*t1.HealthChecks.ReadinessPass).To(BeTrue())
			Expect(*t1.HealthChecks.RestartDelta).To(Equal(0))
			Expect(t1.MetricDeltas).NotTo(BeNil())
			Expect(*t1.MetricDeltas.CpuBefore).To(BeNumerically("~", 0.85))
			Expect(*t1.MetricDeltas.CpuAfter).To(BeNumerically("~", 0.45))

			By("Verifying Tier2 summary")
			Expect(result.Tier2).To(HaveLen(1))
			t2 := result.Tier2[0]
			Expect(t2.RemediationUID).To(Equal("wf-old-001"))
			Expect(t2.Outcome).To(Equal("Failed"))
			Expect(t2.SignalType).To(Equal("HighCPU"))
			Expect(t2.ActionType).To(Equal("restart_pod"))
			Expect(*t2.EffectivenessScore).To(BeNumerically("~", 0.2))
		})
	})

	Describe("UT-KA-433W-002: DS adapter returns empty result for empty DS history", func() {
		It("should return empty Tier1/Tier2 slices, not nil", func() {
			client := &stubDSClient{
				response: &ogenclient.RemediationHistoryContext{
					TargetResource:  "default/Deployment/api-server",
					CurrentSpecHash: "abc123",
					Tier1: ogenclient.RemediationHistoryTier1{
						Window: "24h",
						Chain:  []ogenclient.RemediationHistoryEntry{},
					},
					Tier2: ogenclient.RemediationHistoryTier2{
						Window: "2160h",
						Chain:  []ogenclient.RemediationHistorySummary{},
					},
				},
			}

			adapter := enrichment.NewDSAdapter(client)
			result, err := adapter.GetRemediationHistory(context.Background(), "Deployment", "api-server", "default", "abc123")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Tier1).NotTo(BeNil())
			Expect(result.Tier1).To(BeEmpty())
			Expect(result.Tier2).NotTo(BeNil())
			Expect(result.Tier2).To(BeEmpty())
		})
	})

	Describe("UT-KA-433W-003: DS adapter handles nil Opt* fields without panic", func() {
		It("should not panic when optional fields are unset", func() {
			client := &stubDSClient{
				response: &ogenclient.RemediationHistoryContext{
					TargetResource:  "default/Deployment/api-server",
					CurrentSpecHash: "",
					Tier1: ogenclient.RemediationHistoryTier1{
						Window: "24h",
						Chain: []ogenclient.RemediationHistoryEntry{
							{
								RemediationUID: "wf-123",
								CompletedAt:    time.Date(2026, 2, 28, 15, 30, 0, 0, time.UTC),
							},
						},
					},
					Tier2: ogenclient.RemediationHistoryTier2{Window: "2160h"},
				},
			}

			adapter := enrichment.NewDSAdapter(client)
			Expect(func() {
				result, err := adapter.GetRemediationHistory(context.Background(), "Deployment", "api-server", "default", "")
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Tier1).To(HaveLen(1))
				Expect(result.Tier1[0].RemediationUID).To(Equal("wf-123"))
				Expect(result.Tier1[0].Outcome).To(BeEmpty())
				Expect(result.Tier1[0].EffectivenessScore).To(BeNil())
				Expect(result.Tier1[0].HealthChecks).To(BeNil())
			}).NotTo(Panic())
		})
	})

	Describe("UT-KA-433W-004b: DS adapter sends correct endpoint and query params via ogen client", func() {
		It("should send targetKind, targetName, targetNamespace, currentSpecHash to the correct endpoint", func() {
			var capturedURL string
			var capturedParams map[string]string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedURL = r.URL.Path
				capturedParams = map[string]string{
					"targetKind":      r.URL.Query().Get("targetKind"),
					"targetName":      r.URL.Query().Get("targetName"),
					"targetNamespace": r.URL.Query().Get("targetNamespace"),
					"currentSpecHash": r.URL.Query().Get("currentSpecHash"),
				}

				resp := map[string]interface{}{
					"targetResource":     "staging/StatefulSet/redis",
					"currentSpecHash":    "hash789",
					"regressionDetected": false,
					"tier1": map[string]interface{}{
						"window": "24h",
						"chain":  []interface{}{},
					},
					"tier2": map[string]interface{}{
						"window": "168h",
						"chain":  []interface{}{},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				Expect(json.NewEncoder(w).Encode(resp)).To(Succeed())
			}))
			defer server.Close()

			ogenClient, err := ogenclient.NewClient(server.URL)
			Expect(err).NotTo(HaveOccurred())

			adapter := enrichment.NewDSAdapter(ogenClient)
			_, err = adapter.GetRemediationHistory(context.Background(), "StatefulSet", "redis", "staging", "hash789")
			Expect(err).NotTo(HaveOccurred())

			Expect(capturedURL).To(ContainSubstring("remediation-history"))
			Expect(capturedParams["targetKind"]).To(Equal("StatefulSet"))
			Expect(capturedParams["targetName"]).To(Equal("redis"))
			Expect(capturedParams["targetNamespace"]).To(Equal("staging"))
			Expect(capturedParams["currentSpecHash"]).To(Equal("hash789"))
		})
	})

	Describe("UT-KA-433W-013: DS adapter returns error on BadRequest (#762)", func() {
		It("should return error when DS responds with 400 (not silently swallow)", func() {
			client := &stubDSClient{
				response: &ogenclient.GetRemediationHistoryContextBadRequest{},
			}

			adapter := enrichment.NewDSAdapter(client)
			result, err := adapter.GetRemediationHistory(context.Background(), "Deployment", "api-server", "default", "")
			Expect(err).To(HaveOccurred(), "#762: 400 responses must surface as errors")
			Expect(err.Error()).To(ContainSubstring("bad request"))
			Expect(result).To(BeNil())
		})
	})

	Describe("UT-KA-433W-005: DS adapter returns structured error on HTTP 500", func() {
		It("should return error wrapping the HTTP failure", func() {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			ogenClient, err := ogenclient.NewClient(server.URL)
			Expect(err).NotTo(HaveOccurred())

			adapter := enrichment.NewDSAdapter(ogenClient)
			result, err := adapter.GetRemediationHistory(context.Background(), "Deployment", "api", "default", "")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ds adapter"))
			Expect(result).To(BeNil())
		})
	})
})

// stubDSClient is a test double for enrichment.HistoryContextClient that returns
// preconfigured responses. This is a stub (not a mock) — used for unit tests
// that validate type mapping, not I/O behavior.
type stubDSClient struct {
	response ogenclient.GetRemediationHistoryContextRes
	err      error
}

func (s *stubDSClient) GetRemediationHistoryContext(
	_ context.Context,
	_ ogenclient.GetRemediationHistoryContextParams,
) (ogenclient.GetRemediationHistoryContextRes, error) {
	return s.response, s.err
}
