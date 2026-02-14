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

// Package datastorage contains unit tests for the DataStorage service.
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// DD-HAPI-016 v1.1: HTTP handler tests for GET /api/v1/remediation-history/context.
package datastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockRemediationHistoryQuerier implements server.RemediationHistoryQuerier for testing.
// Each method delegates to a configurable function field, allowing per-test behavior.
type mockRemediationHistoryQuerier struct {
	queryROEventsByTargetFn    func(ctx context.Context, targetResource string, since time.Time) ([]repository.RawAuditRow, error)
	queryEffectivenessEventsFn func(ctx context.Context, correlationIDs []string) (map[string][]*server.EffectivenessEvent, error)
	queryROEventsBySpecHashFn  func(ctx context.Context, specHash string, since, until time.Time) ([]repository.RawAuditRow, error)
}

func (m *mockRemediationHistoryQuerier) QueryROEventsByTarget(ctx context.Context, targetResource string, since time.Time) ([]repository.RawAuditRow, error) {
	if m.queryROEventsByTargetFn != nil {
		return m.queryROEventsByTargetFn(ctx, targetResource, since)
	}
	return nil, nil
}

func (m *mockRemediationHistoryQuerier) QueryEffectivenessEventsBatch(ctx context.Context, correlationIDs []string) (map[string][]*server.EffectivenessEvent, error) {
	if m.queryEffectivenessEventsFn != nil {
		return m.queryEffectivenessEventsFn(ctx, correlationIDs)
	}
	return nil, nil
}

func (m *mockRemediationHistoryQuerier) QueryROEventsBySpecHash(ctx context.Context, specHash string, since, until time.Time) ([]repository.RawAuditRow, error) {
	if m.queryROEventsBySpecHashFn != nil {
		return m.queryROEventsBySpecHashFn(ctx, specHash, since, until)
	}
	return nil, nil
}

var _ = Describe("Remediation History Handler (DD-HAPI-016 v1.1)", func() {
	var (
		handler *server.Handler
		rec     *httptest.ResponseRecorder
		mock    *mockRemediationHistoryQuerier
	)

	// Base URL with all required params
	baseURL := "/api/v1/remediation-history/context" +
		"?targetKind=Deployment&targetName=nginx&targetNamespace=default&currentSpecHash=sha256:abc123"

	BeforeEach(func() {
		mock = &mockRemediationHistoryQuerier{}
		handler = server.NewHandler(nil,
			server.WithRemediationHistoryQuerier(mock),
		)
		rec = httptest.NewRecorder()
	})

	// ========================================
	// Parameter Validation (400 errors)
	// ========================================
	Describe("Parameter Validation", func() {

		It("UT-RH-HANDLER-001: should return 400 when targetKind is missing", func() {
			req := httptest.NewRequest("GET",
				"/api/v1/remediation-history/context?targetName=nginx&targetNamespace=default&currentSpecHash=sha256:abc",
				nil)
			handler.HandleGetRemediationHistoryContext(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))

			var problem map[string]interface{}
			Expect(json.Unmarshal(rec.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["title"]).To(Equal("Missing Required Parameter"))
		})

		It("UT-RH-HANDLER-002: should return 400 when targetName is missing", func() {
			req := httptest.NewRequest("GET",
				"/api/v1/remediation-history/context?targetKind=Deployment&targetNamespace=default&currentSpecHash=sha256:abc",
				nil)
			handler.HandleGetRemediationHistoryContext(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		})

		It("UT-RH-HANDLER-003: should return 400 when targetNamespace is missing", func() {
			req := httptest.NewRequest("GET",
				"/api/v1/remediation-history/context?targetKind=Deployment&targetName=nginx&currentSpecHash=sha256:abc",
				nil)
			handler.HandleGetRemediationHistoryContext(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		})

		It("UT-RH-HANDLER-004: should return 400 when currentSpecHash is missing", func() {
			req := httptest.NewRequest("GET",
				"/api/v1/remediation-history/context?targetKind=Deployment&targetName=nginx&targetNamespace=default",
				nil)
			handler.HandleGetRemediationHistoryContext(rec, req)

			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))
		})
	})

	// ========================================
	// Successful Responses
	// ========================================
	Describe("Successful Responses", func() {

		It("UT-RH-HANDLER-005: should return 200 with empty chains when no RO events", func() {
			mock.queryROEventsByTargetFn = func(_ context.Context, _ string, _ time.Time) ([]repository.RawAuditRow, error) {
				return nil, nil
			}

			req := httptest.NewRequest("GET", baseURL, nil)
			handler.HandleGetRemediationHistoryContext(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Header().Get("Content-Type")).To(ContainSubstring("application/json"))

			var resp map[string]interface{}
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["targetResource"]).To(Equal("default/Deployment/nginx"))
			Expect(resp["currentSpecHash"]).To(Equal("sha256:abc123"))
			Expect(resp["regressionDetected"]).To(BeFalse())

			tier1 := resp["tier1"].(map[string]interface{})
			Expect(tier1["chain"]).To(BeEmpty())
		})

		It("UT-RH-HANDLER-006: should return 200 with populated tier1 chain when RO+EM data exists", func() {
			fixedTime := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)

			mock.queryROEventsByTargetFn = func(_ context.Context, targetResource string, _ time.Time) ([]repository.RawAuditRow, error) {
				Expect(targetResource).To(Equal("default/Deployment/nginx"))
				return []repository.RawAuditRow{
					{
						EventType:      "remediation.workflow_created",
						CorrelationID:  "rr-test-001",
						EventTimestamp: fixedTime,
						EventData: map[string]interface{}{
							"pre_remediation_spec_hash": "sha256:pre111",
							"outcome":                  "success",
							"signal_type":              "alert",
							"signal_fingerprint":       "fp-001",
							"workflow_type":            "restart",
							"target_resource":          "default/Deployment/nginx",
						},
					},
				}, nil
			}
			mock.queryEffectivenessEventsFn = func(_ context.Context, ids []string) (map[string][]*server.EffectivenessEvent, error) {
				Expect(ids).To(ConsistOf("rr-test-001"))
				return map[string][]*server.EffectivenessEvent{
					"rr-test-001": {
						{
							EventData: map[string]interface{}{
								"event_type": "effectiveness.health.assessed",
								"assessed":   true,
								"score":      1.0,
								"health_checks": map[string]interface{}{
									"pod_running":    true,
									"readiness_pass": true,
									"restart_delta":  float64(0),
								},
							},
						},
						{
							EventData: map[string]interface{}{
								"event_type": "effectiveness.alert.assessed",
								"assessed":   true,
								"score":      1.0,
								"alert_resolution": map[string]interface{}{
									"alert_resolved": true,
								},
							},
						},
						{
							EventData: map[string]interface{}{
								"event_type":                 "effectiveness.hash.computed",
								"pre_remediation_spec_hash":  "sha256:pre111",
								"post_remediation_spec_hash": "sha256:post222",
								"hash_match":                 false,
							},
						},
						{
							EventData: map[string]interface{}{
								"event_type": "effectiveness.assessment.completed",
								"reason":     "full",
							},
						},
					},
				}, nil
			}

			req := httptest.NewRequest("GET", baseURL, nil)
			handler.HandleGetRemediationHistoryContext(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))

			var resp map[string]interface{}
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())

			Expect(resp["regressionDetected"]).To(BeFalse())

			tier1 := resp["tier1"].(map[string]interface{})
			chain := tier1["chain"].([]interface{})
			Expect(chain).To(HaveLen(1))

			entry := chain[0].(map[string]interface{})
			Expect(entry["remediationUID"]).To(Equal("rr-test-001"))
			Expect(entry["outcome"]).To(Equal("success"))
			Expect(entry["signalType"]).To(Equal("alert"))

			// Effectiveness score should be populated
			Expect(entry["effectivenessScore"]).ToNot(BeNil())

			// Health checks should be present
			hc := entry["healthChecks"].(map[string]interface{})
			Expect(hc["podRunning"]).To(BeTrue())
		})

		It("UT-RH-HANDLER-007: should set regressionDetected=true when current matches preHash", func() {
			fixedTime := time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC)

			mock.queryROEventsByTargetFn = func(_ context.Context, _ string, _ time.Time) ([]repository.RawAuditRow, error) {
				return []repository.RawAuditRow{
					{
						EventType:      "remediation.workflow_created",
						CorrelationID:  "rr-regress",
						EventTimestamp: fixedTime,
						EventData: map[string]interface{}{
							// preHash matches currentSpecHash (sha256:abc123) -> regression
							"pre_remediation_spec_hash": "sha256:abc123",
							"outcome":                  "success",
							"signal_type":              "alert",
							"signal_fingerprint":       "fp-reg",
							"workflow_type":            "restart",
							"target_resource":          "default/Deployment/nginx",
						},
					},
				}, nil
			}
			mock.queryEffectivenessEventsFn = func(_ context.Context, _ []string) (map[string][]*server.EffectivenessEvent, error) {
				return map[string][]*server.EffectivenessEvent{
					"rr-regress": {
						{
							EventData: map[string]interface{}{
								"event_type":                 "effectiveness.hash.computed",
								"pre_remediation_spec_hash":  "sha256:abc123",
								"post_remediation_spec_hash": "sha256:fixed",
								"hash_match":                 false,
							},
						},
					},
				}, nil
			}
			// Regression triggers Tier 2 query
			mock.queryROEventsBySpecHashFn = func(_ context.Context, specHash string, _ time.Time, _ time.Time) ([]repository.RawAuditRow, error) {
				Expect(specHash).To(Equal("sha256:abc123"))
				return nil, nil // empty Tier 2
			}

			req := httptest.NewRequest("GET", baseURL, nil)
			handler.HandleGetRemediationHistoryContext(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))

			var resp map[string]interface{}
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["regressionDetected"]).To(BeTrue())
		})
	})

	// ========================================
	// Default Window Durations
	// ========================================
	Describe("Default Window Durations", func() {

		It("UT-RH-HANDLER-008: should use default 24h tier1 and 2160h tier2 windows when omitted", func() {
			var capturedSince time.Time

			mock.queryROEventsByTargetFn = func(_ context.Context, _ string, since time.Time) ([]repository.RawAuditRow, error) {
				capturedSince = since
				return nil, nil
			}

			req := httptest.NewRequest("GET", baseURL, nil)
			handler.HandleGetRemediationHistoryContext(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			// Tier 1 default: 24h lookback
			expectedSince := time.Now().Add(-24 * time.Hour)
			Expect(capturedSince).To(BeTemporally("~", expectedSince, 5*time.Second))
		})
	})

	// ========================================
	// Error Handling
	// ========================================
	Describe("Error Handling", func() {

		It("UT-RH-HANDLER-009: should return 500 when repository returns error", func() {
			mock.queryROEventsByTargetFn = func(_ context.Context, _ string, _ time.Time) ([]repository.RawAuditRow, error) {
				return nil, fmt.Errorf("database connection lost")
			}

			req := httptest.NewRequest("GET", baseURL, nil)
			handler.HandleGetRemediationHistoryContext(rec, req)

			Expect(rec.Code).To(Equal(http.StatusInternalServerError))
			Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))

			var problem map[string]interface{}
			Expect(json.Unmarshal(rec.Body.Bytes(), &problem)).To(Succeed())
			Expect(problem["title"]).To(Equal("Internal Server Error"))
		})
	})
})
