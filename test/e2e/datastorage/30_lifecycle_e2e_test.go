/*
Copyright 2025 Jordi Gil.

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

package datastorage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ============================================================
// Phase 9 — Data Storage lifecycle (E2E behavioral spec)
//
// Maps: ET-DS-1088-LC-001 .. ET-DS-1088-LC-005
// BR-STORAGE-028 / DD-007 / DD-008 / DD-009: Startup readiness,
// steady-state ingest, query tamper-chain APIs, Redis/D readiness.
//
// Shutdown and DLQ drain are validated under load in integration/unit
// tiers (signal-driven, pod termination); this suite proves the deployed
// service has completed startup (health), accepts traffic (batch ingest),
// and exposes tamper-evident read paths (query, verify-chain, export)
// on the shared Kind deployment.
//
// Issue #753: Dedicated health listener exposes /healthz (liveness) and
// /readyz (readiness); callers use healthURL + /healthz for “health UP”.
// ============================================================

var _ = Describe("Data Storage lifecycle Phase 9 (ET-DS-1088-LC)", Ordered, func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		correlationID string

		httpHealth *http.Client
		eventCount int
	)

	BeforeAll(func() {
		testCtx, testCancel = context.WithCancel(ctx)
		correlationID = fmt.Sprintf("lifecycle-1088-%s", uuid.New().String()[:8])
		eventCount = 5
		httpHealth = &http.Client{Timeout: 5 * time.Second}
	})

	AfterAll(func() {
		testCancel()
	})

	BeforeEach(func() {
		Eventually(func() int {
			resp, err := httpHealth.Get(healthURL + "/readyz")
			if err != nil || resp == nil {
				return 0
			}
			defer func() { _ = resp.Body.Close() }()
			return resp.StatusCode
		}, "30s", "500ms").Should(Equal(http.StatusOK), "Data Storage should be ready before lifecycle steps")

		client, err := createOpenAPIClient(dataStorageURL)
		Expect(err).ToNot(HaveOccurred())
		Expect(client).NotTo(BeNil())
	})

	It("ET-DS-1088-LC-001: should expose a reachable dedicated health endpoint after startup", func() {
		resp, err := httpHealth.Get(healthURL + "/healthz")
		Expect(err).ToNot(HaveOccurred(), "Dedicated health listener must accept connections")
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		raw, readErr := io.ReadAll(resp.Body)
		Expect(readErr).ToNot(HaveOccurred())

		var body map[string]string
		Expect(json.Unmarshal(raw, &body)).To(Succeed())
		Expect(strings.ToLower(strings.TrimSpace(body["status"]))).To(Equal("healthy"))

		Expect(body["timestamp"]).NotTo(Equal(""))
		_, parseErr := time.Parse(time.RFC3339Nano, body["timestamp"])
		if parseErr != nil {
			_, parseErr = time.Parse(time.RFC3339, body["timestamp"])
		}
		Expect(parseErr).ToNot(HaveOccurred(), "Health payload should embed an RFC3339-style timestamp")

		respReady, err := httpHealth.Get(healthURL + "/readyz")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = respReady.Body.Close() }()
		Expect(respReady.StatusCode).To(Equal(http.StatusOK))

		bodyReady, readErr := io.ReadAll(respReady.Body)
		Expect(readErr).ToNot(HaveOccurred())
		Expect(strings.Contains(string(bodyReady), "ready")).To(BeTrue(), "Readiness probe should report ready JSON")
	})

	It("ET-DS-1088-LC-002: should accept a batched ingest of correlated audit events", func() {
		client, err := createOpenAPIClient(dataStorageURL)
		Expect(err).ToNot(HaveOccurred())

		baseTs := time.Now().UTC().Add(-10 * time.Minute)
		events := make([]dsgen.AuditEventRequest, 0, eventCount)

		for i := 0; i < eventCount; i++ {
			events = append(events, dsgen.AuditEventRequest{
				CorrelationID:  correlationID,
				EventAction:    "lifecycle_batch_ingest",
				EventCategory:  dsgen.AuditEventRequestEventCategoryGateway,
				EventOutcome:   dsgen.AuditEventRequestEventOutcomeSuccess,
				EventType:      "lifecycle.phase9.gateway.signal",
				EventTimestamp: baseTs.Add(time.Duration(i) * time.Second),
				Version:        "1.0",
				EventData:      newMinimalGatewayPayload("alert", "lifecycle-phase9"),
			})
		}

		ids, err := postAuditEventBatch(testCtx, client, events)
		Expect(err).ToNot(HaveOccurred())
		Expect(ids).To(HaveLen(eventCount), "Batch API returns one event ID per queued write")

		for _, idStr := range ids {
			Expect(idStr).To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`))
		}
	})

	It("ET-DS-1088-LC-003: should list ingested audit events via the query API with matching correlation ID", func() {
		client, err := createOpenAPIClient(dataStorageURL)
		Expect(err).ToNot(HaveOccurred())

		Eventually(func() int {
			resp, queryErr := client.QueryAuditEvents(testCtx, dsgen.QueryAuditEventsParams{
				CorrelationID: dsgen.NewOptString(correlationID),
				Limit:         dsgen.NewOptInt(100),
				Offset:        dsgen.NewOptInt(0),
			})
			Expect(queryErr).ToNot(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			return len(resp.Data)
		}, 45*time.Second, 750*time.Millisecond).Should(Equal(eventCount),
			"Query API returns all persisted events once batch processing completes")

		final, queryErr := client.QueryAuditEvents(testCtx, dsgen.QueryAuditEventsParams{
			CorrelationID: dsgen.NewOptString(correlationID),
			Limit:         dsgen.NewOptInt(100),
			Offset:        dsgen.NewOptInt(0),
		})
		Expect(queryErr).ToNot(HaveOccurred())

		for _, ev := range final.Data {
			Expect(ev.CorrelationID).To(Equal(correlationID))
			Expect(ev.EventType).To(Equal("lifecycle.phase9.gateway.signal"))
			Expect(ev.EventTimestamp).NotTo(Equal(time.Time{}))
		}
		Expect(final.Pagination.Set).To(BeTrue())
		pg := final.Pagination.Value
		Expect(pg.Total.Set).To(BeTrue(), "Pagination metadata exposes totals for audit queries")
		Expect(pg.Total.Value).To(BeNumerically(">=", eventCount))

		prev := final.Data[0].EventTimestamp.UTC()
		for idx := 1; idx < len(final.Data); idx++ {
			next := final.Data[idx].EventTimestamp.UTC()
			Expect(next.Before(prev) || next.Equal(prev)).To(BeTrue(),
				"Descending audit timeline should never increase into the future at later indexes")
			prev = next
		}
	})

	It("ET-DS-1088-LC-004: should report a valid tamper-evident chain from verify-chain", func() {
		client, err := createOpenAPIClient(dataStorageURL)
		Expect(err).ToNot(HaveOccurred())

		verifyRes, verifyErr := client.VerifyAuditChain(testCtx, &dsgen.VerifyChainRequest{
			CorrelationID: correlationID,
		})
		Expect(verifyErr).ToNot(HaveOccurred())

		verifyData, ok := verifyRes.(*dsgen.VerifyChainResponse)
		Expect(ok).To(BeTrue(), "Verify-chain must return typed success payloads")

		Expect(verifyData.CorrelationID).To(Equal(correlationID))
		Expect(verifyData.IsValid).To(BeTrue())
		Expect(verifyData.TotalEvents).To(Equal(eventCount))
		Expect(verifyData.VerifiedEvents).To(Equal(eventCount))
		Expect(verifyData.TamperedEvents).To(BeEmpty())
		Expect(strings.TrimSpace(verifyData.Message)).NotTo(Equal(""))
		Expect(verifyData.VerificationTime).NotTo(Equal(time.Time{}))
	})

	It("ET-DS-1088-LC-005: should export persisted events whose hash-chain metadata validates", func() {
		client, err := createOpenAPIClient(dataStorageURL)
		Expect(err).ToNot(HaveOccurred())

		exportEnvelope, exportErr := client.ExportAuditEvents(testCtx, dsgen.ExportAuditEventsParams{
			CorrelationID: dsgen.NewOptString(correlationID),
		})
		Expect(exportErr).ToNot(HaveOccurred())

		exportData, ok := exportEnvelope.(*dsgen.AuditExportResponse)
		Expect(ok).To(BeTrue(), "Exports must surface typed envelope")

		Expect(exportData.Events).To(HaveLen(eventCount))
		verify := exportData.HashChainVerification
		Expect(verify.TotalEventsVerified).To(Equal(eventCount))
		Expect(verify.ValidChainEvents).To(Equal(eventCount))
		Expect(verify.BrokenChainEvents).To(Equal(0))
		Expect(verify.ChainIntegrityPercentage.Set).To(BeTrue())
		Expect(verify.ChainIntegrityPercentage.Value).To(Equal(float32(100.0)))

		for idx, exported := range exportData.Events {
			Expect(exported.EventHash.Set).To(BeTrue(), "SOC2/CC8 exports always carry cryptographic hashes")
			Expect(strings.TrimSpace(exported.EventHash.Value)).To(HaveLen(64), "Event hash must mirror SHA256 hex length")
			Expect(exported.PreviousEventHash.Set).To(BeTrue(), "Chained audits always surface previous-hash metadata")
			if idx > 0 {
				Expect(strings.TrimSpace(exported.PreviousEventHash.Value)).NotTo(Equal(""), "Non-genesis links must cite the prior anchor")
			}

			Expect(exported.HashChainValid.Set).To(BeTrue(), "Consumers must observe explicit hash-valid flags")
			Expect(exported.HashChainValid.Value).To(BeTrue())
		}
	})
})
