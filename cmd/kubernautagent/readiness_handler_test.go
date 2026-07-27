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

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	karbac "github.com/jordigilh/kubernaut/internal/kubernautagent/rbac"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/workflowcatalog"
	"github.com/jordigilh/kubernaut/pkg/fleet/readiness"
)

// readyWfCatalog returns a *workflowcatalog.LazyCatalog that is immediately
// Ready, for tests exercising readinessHandler branches unrelated to the
// workflow catalog dependency itself (#1677 hardening, DD-WORKFLOW-019).
func readyWfCatalog() *workflowcatalog.LazyCatalog {
	return workflowcatalog.NewLazyCatalogReady(workflowcatalog.NewCacheFromReader(nil), logr.Discard())
}

// fakeUnreadyProber is a readiness.Prober test double that always reports
// its dependency as unreachable, used to exercise the fleetGate branch of
// readinessHandler without depending on a real MCP Gateway connection.
type fakeUnreadyProber struct{}

func (fakeUnreadyProber) Probe(context.Context) error {
	return errors.New("fake: fleet dependency unreachable")
}

var _ = Describe("readinessHandler", func() {
	var shutdownFlag, apiServerReady int32

	BeforeEach(func() {
		shutdownFlag = 0
		apiServerReady = 0
	})

	doRequest := func(handler http.HandlerFunc) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}

	// UT-KA-JWKS-001: readinessHandler reports not_ready before the main API
	// server has started listening, even when shutdownFlag and the LLM client
	// are otherwise ready. This guards against the race fixed alongside the
	// JWKS pre-warm startup reorder: the health server now starts before the
	// route-setup closure (which performs the JWKS pre-warm), so /readyz must
	// not report ready until the main API server is actually about to listen.
	It("UT-KA-JWKS-001: reports not_ready before the API server has started listening", func() {
		swappable := setupSwappable(GinkgoTB())
		interactive := karbac.NewInteractiveReadiness()

		handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil, readyWfCatalog())

		rec := doRequest(handler)
		Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
	})

	// UT-KA-JWKS-002: readinessHandler reports ready once apiServerReady is
	// set (and all other dependencies are healthy).
	It("UT-KA-JWKS-002: reports ready once the API server is listening", func() {
		atomic.StoreInt32(&apiServerReady, 1)
		swappable := setupSwappable(GinkgoTB())
		interactive := karbac.NewInteractiveReadiness()

		handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil, readyWfCatalog())

		rec := doRequest(handler)
		Expect(rec.Code).To(Equal(http.StatusOK))
	})

	// UT-KA-JWKS-003: shutdownFlag takes priority over apiServerReady -- once
	// shutdown begins, the probe must fail even if the API server was ready.
	It("UT-KA-JWKS-003: shutdownFlag takes priority over apiServerReady", func() {
		atomic.StoreInt32(&apiServerReady, 1)
		atomic.StoreInt32(&shutdownFlag, 1)
		swappable := setupSwappable(GinkgoTB())
		interactive := karbac.NewInteractiveReadiness()

		handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil, readyWfCatalog())

		rec := doRequest(handler)
		Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
	})

	// UT-KA-1553-001: readinessHandler must report 503 when the injected Fleet
	// readiness gate is NotReady, even though every other dependency (API
	// server, LLM client, interactive mode) is healthy. This is the pod-wide
	// fail-closed behavior mandated by ADR-068 decision #11 / BR-INTEGRATION-054
	// (#1553): an unreachable Fleet MCP Gateway must remove the whole pod from
	// Service endpoints, not just silently degrade fleet-scoped functionality.
	It("UT-KA-1553-001: reports 503 when the Fleet readiness gate is NotReady", func() {
		atomic.StoreInt32(&apiServerReady, 1)
		swappable := setupSwappable(GinkgoTB())
		interactive := karbac.NewInteractiveReadiness()

		gate := readiness.NewGate(time.Hour, logr.Discard(), fakeUnreadyProber{})
		gate.Start(context.Background())
		DeferCleanup(gate.Stop)

		handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, gate, readyWfCatalog())

		rec := doRequest(handler)
		Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
	})

	// UT-KA-1553-002: readinessHandler must report 200 when the injected Fleet
	// readiness gate is nil (fleet mode not configured), matching the existing
	// soft-dependency convention used for ds.
	It("UT-KA-1553-002: reports 200 when the Fleet readiness gate is nil (fleet mode not configured)", func() {
		atomic.StoreInt32(&apiServerReady, 1)
		swappable := setupSwappable(GinkgoTB())
		interactive := karbac.NewInteractiveReadiness()

		handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil, readyWfCatalog())

		rec := doRequest(handler)
		Expect(rec.Code).To(Equal(http.StatusOK))
	})

	// UT-KA-1677-READY-001/002/003: #1677 hardening (DD-WORKFLOW-019). Unlike
	// ds/fleetGate, the workflow catalog is a hard (non-optional) dependency:
	// KA always runs in-cluster and always constructs a wfCatalog, so the pod
	// must never claim readiness before its first successful cache sync.
	It("UT-KA-1677-READY-001: reports 503 when wfCatalog has not yet completed its first sync", func() {
		atomic.StoreInt32(&apiServerReady, 1)
		swappable := setupSwappable(GinkgoTB())
		interactive := karbac.NewInteractiveReadiness()

		notReady := workflowcatalog.NewLazyCatalog(logr.Discard()) // never Start()ed

		handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil, notReady)

		rec := doRequest(handler)
		Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
	})

	It("UT-KA-1677-READY-002: reports 503 when wfCatalog is nil (wiring bug guard)", func() {
		atomic.StoreInt32(&apiServerReady, 1)
		swappable := setupSwappable(GinkgoTB())
		interactive := karbac.NewInteractiveReadiness()

		handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil, nil)

		rec := doRequest(handler)
		Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
	})

	It("UT-KA-1677-READY-003: reports 200 once wfCatalog has completed its first sync", func() {
		atomic.StoreInt32(&apiServerReady, 1)
		swappable := setupSwappable(GinkgoTB())
		interactive := karbac.NewInteractiveReadiness()

		handler := readinessHandler(&shutdownFlag, &apiServerReady, swappable, nil, interactive, nil, readyWfCatalog())

		rec := doRequest(handler)
		Expect(rec.Code).To(Equal(http.StatusOK))
	})
})
