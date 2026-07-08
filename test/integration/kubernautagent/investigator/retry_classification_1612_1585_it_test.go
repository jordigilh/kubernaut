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
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-logr/logr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/investigator"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/backoff"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/fault"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/handlers"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// Issues #1612 (streaming retry) and #1585 (non-retryable error
// classification): the unit-tier tests for these issues (chat_helpers,
// streaming_retry_1612_test.go, classify_1585_test.go) all mock or
// scaffold the LLM client's error, proving the retry/classification LOGIC
// is correct given a scripted failure — but none of them exercise the
// actual boundary where a real provider error originates: a real HTTP
// response -> real openaicompat.APIError parsing -> real StatusCode
// extraction -> classification -> retry-with-backoff. A bug in that
// translation layer would pass every unit test and only surface in
// production.
//
// These IT tests close that gap by driving a REAL kaopenai.New client
// through a REAL llm.SwappableClient + investigator.DefaultPhaseResolver
// (the exact production assembly cmd/kubernautagent/bootstrap.go uses),
// against the REAL mock-llm HTTP service's fault-injection API
// (test/services/mock-llm/handlers/fault.go), so the failures reproduced
// here are indistinguishable — from the investigator's point of view —
// from a real LLM provider outage or a real invalid-credentials response.
var retryClassificationFastBackoff = &backoff.Config{
	BasePeriod:    1 * time.Millisecond,
	MaxPeriod:     5 * time.Millisecond,
	Multiplier:    2.0,
	JitterPercent: 0,
}

var retryClassificationSignal = katypes.SignalContext{
	Name:          "test-pod",
	Namespace:     "default",
	Severity:      "critical",
	Message:       "OOMKilled",
	Environment:   "Production",
	Priority:      "P1",
	RemediationID: "rem-retry-classification-it",
}

// newRetryClassificationInvestigator wires a REAL openai-compatible LLM
// client (pointed at the mock-llm httptest server) through the same
// SwappableClient + DefaultPhaseResolver assembly production uses — not a
// hand-rolled test double — so retry-with-backoff and error classification
// run exactly as they do in cmd/kubernautagent.
func newRetryClassificationInvestigator(mockLLMURL string, maxRetries int, auditStore audit.AuditStore) *investigator.Investigator {
	logger := logr.Discard()
	client := kaopenai.New("gpt-4o", mockLLMURL, "test-key")
	sw, err := llm.NewSwappableClient(client, "gpt-4o", llm.RuntimeParams{
		MaxRetries:   maxRetries,
		RetryBackoff: retryClassificationFastBackoff,
	})
	Expect(err).NotTo(HaveOccurred())
	builder, err := prompt.NewBuilder()
	Expect(err).NotTo(HaveOccurred())
	return investigator.New(investigator.Config{
		PhaseResolver: investigator.NewDefaultPhaseResolver(sw, nil),
		Builder:       builder,
		ResultParser:  parser.NewResultParser(),
		AuditStore:    auditStore,
		Logger:        logger,
		MaxTurns:      1,
		PhaseTools:    investigator.DefaultPhaseToolMap(),
	})
}

// configureMockLLMFault and mockLLMRequestCount drive the mock-llm
// service's real HTTP verification/fault-injection API — the same API
// test/integration/mockllm's own tests use — so retry counts are proven
// via requests that actually crossed the wire, not an in-process counter.
func configureMockLLMFault(serverURL string, cfg fault.Config) {
	GinkgoHelper()
	body, err := json.Marshal(cfg)
	Expect(err).NotTo(HaveOccurred())
	resp, err := http.Post(serverURL+"/api/test/fault", "application/json", bytes.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
}

func mockLLMRequestCount(serverURL string) int {
	GinkgoHelper()
	resp, err := http.Get(serverURL + "/api/test/request-count")
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	var out struct {
		Count int `json:"count"`
	}
	Expect(json.NewDecoder(resp.Body).Decode(&out)).To(Succeed())
	return out.Count
}

var _ = Describe("KA investigator retry-with-backoff + error classification through a REAL LLM HTTP boundary — #1612, #1585", func() {
	var mockServer *httptest.Server

	BeforeEach(func() {
		mockServer = httptest.NewServer(handlers.NewFullRouter(scenarios.DefaultRegistry(), false, "", "", nil))
	})

	AfterEach(func() {
		mockServer.Close()
	})

	Describe("IT-KA-1612-101: transient real HTTP 503s recover via retry-with-backoff", func() {
		It("retries through the real openai-compatible transport and the investigation completes", func() {
			configureMockLLMFault(mockServer.URL, fault.Config{
				Enabled: true, StatusCode: http.StatusServiceUnavailable, Message: "intermittent", Count: 2,
			})

			inv := newRetryClassificationInvestigator(mockServer.URL, 3, audit.NopAuditStore{})
			result, err := inv.Investigate(context.Background(), retryClassificationSignal)

			Expect(err).NotTo(HaveOccurred(),
				"retry-with-backoff must recover from 2 real transient 503 responses and let the investigation proceed")
			Expect(result).NotTo(BeNil())
			Expect(mockLLMRequestCount(mockServer.URL)).To(BeNumerically(">=", 3),
				"must show >=2 failed real HTTP attempts + >=1 successful retry — proving the retry loop crossed the real transport boundary, not an in-process mock")
		})
	})

	Describe("IT-KA-1585-101: non-retryable real HTTP 401 fails fast without consuming the retry budget", func() {
		It("makes exactly one real HTTP request despite a 5-attempt budget, and audits ResponseFailed (AU-3)", func() {
			configureMockLLMFault(mockServer.URL, fault.Config{
				Enabled: true, StatusCode: http.StatusUnauthorized, Message: "invalid api key",
			})

			spy := newCapturingAuditStore(suiteAuditStore)
			inv := newRetryClassificationInvestigator(mockServer.URL, 5, spy)
			result, err := inv.Investigate(context.Background(), retryClassificationSignal)

			Expect(err).To(HaveOccurred(), "a real 401 must fail the investigation, not hang retrying")
			Expect(result).To(BeNil())
			Expect(mockLLMRequestCount(mockServer.URL)).To(Equal(1),
				"a real 401 classified via openaicompat.APIError.StatusCode must fail fast — zero retries consumed despite 5 in budget")

			failEvents := filterEvents(spy.events, audit.EventTypeResponseFailed)
			Expect(failEvents).To(HaveLen(1), "exactly one response-failed audit event expected (AU-3)")
			Expect(failEvents[0].EventAction).To(Equal(audit.ActionResponseFailed))
			Expect(failEvents[0].EventOutcome).To(Equal(audit.OutcomeFailure))
			Expect(failEvents[0].CorrelationID).To(Equal(retryClassificationSignal.RemediationID))
		})
	})
})
