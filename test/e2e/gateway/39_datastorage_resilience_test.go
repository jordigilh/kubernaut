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

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	dsgen "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	dsshared "github.com/jordigilh/kubernaut/test/e2e/datastorage/shared"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// Test 39: DataStorage Resilience -- the sole E2E representative for the
// #1985 shared journey (2026-08-16 scope decision: the readyz fail/recover
// *mechanism* is already fully proven at UT tier, pkg/fleet/readiness's
// UT-FLEET-READY-004, and at IT tier for all 10 services,
// IT-AUDIT-1985-003..012 -- including a dedicated ctrl-runtime-vs-custom-
// aggregator EffectivenessMonitor/Gateway pair -- against each service's
// real production wiring entry point. A second full E2E journey would only
// re-prove that same mechanism a third time with no new assertion. What
// only E2E can add is the SOC2 CC8.1 business-outcome claim this test
// alone makes: a real request after recovery produces a complete, gapless,
// queryable-by-correlation_id audit trail -- see
// triggerGatewayResilienceSignalAndVerifyAudit below).
//
// Runs against a dedicated, throwaway "gateway-resilience" instance (never
// the shared "gateway" instance every other spec in this suite depends on)
// so this test carries zero blast radius and needs no Serial decorator.
//
// Business Requirements: BR-AUDIT-005 v2.0, BR-GATEWAY-190.
// Authority: Issue #1985, SOC2 CC8.1, AU-9.
var _ = dsshared.Journey("Test 39: DataStorage Resilience (#1985, BR-AUDIT-005 v2.0, SOC2 CC8.1)", func() dsshared.Target {
	return dsshared.Target{
		KubeconfigPath:       kubeconfigPath,
		DataStorageNamespace: infrastructure.GatewayResilienceDataStorageNamespace,
		Deploy: func(ctx context.Context) error {
			return infrastructure.DeployGatewayForDataStorageResilienceTest(
				ctx, kubeconfigPath, gatewayNamespace, GinkgoWriter)
		},
		Teardown: func(ctx context.Context) {
			infrastructure.TeardownGatewayForDataStorageResilienceTest(ctx, kubeconfigPath, gatewayNamespace, GinkgoWriter)
		},
		ReadyzURL:             fmt.Sprintf("http://127.0.0.1:%d/readyz", infrastructure.GatewayResilienceHealthHostPort),
		TriggerAndVerifyAudit: triggerGatewayResilienceSignalAndVerifyAudit,
	}
})

// triggerGatewayResilienceSignalAndVerifyAudit POSTs one real Prometheus
// alert through the now-recovered "gateway-resilience" dedicated instance
// and asserts a complete audit trail is queryable from the REAL DataStorage
// by correlation_id (SOC2 CC8.1) -- proving the readiness gate closed the
// #1985 audit-loss window rather than merely delaying it. Mirrors
// 15_audit_trace_validation_test.go's proven pattern against the shared
// Gateway instance, retargeted at the dedicated one.
func triggerGatewayResilienceSignalAndVerifyAudit(bgCtx context.Context) error {
	reqCtx, cancel := context.WithTimeout(bgCtx, 60*time.Second)
	defer cancel()

	// This single SA needs the union of two RBAC grants, mirroring the proven
	// two-token pattern in 15_audit_trace_validation_test.go: Gateway access
	// (services/gateway-service, verb create) to authorize the signal POST
	// below, and DataStorage access (data-storage-client CRUD) to authorize
	// the correlation_id audit query later in this function. Test 15 keeps
	// these as two separate SAs/tokens; granting both ClusterRoleBindings to
	// one SA here is equivalent and simpler for a single-purpose test helper.
	// Root-caused via must-gather log evidence (CI run 31883439910): POSTing
	// with a DataStorage-only token got a genuine 403 authorization_denied
	// from Gateway's auth middleware -- not a connection/readiness race.
	e2eSAName := "gateway-resilience-e2e-audit-client"
	if err := infrastructure.CreateE2EServiceAccountWithGatewayAccess(
		reqCtx, gatewayNamespace, kubeconfigPath, e2eSAName, GinkgoWriter,
	); err != nil {
		return fmt.Errorf("failed to grant resilience-test audit ServiceAccount Gateway access: %w", err)
	}
	if err := infrastructure.CreateE2EServiceAccountWithDataStorageAccess(
		reqCtx, gatewayNamespace, kubeconfigPath, e2eSAName, GinkgoWriter,
	); err != nil {
		return fmt.Errorf("failed to grant resilience-test audit ServiceAccount DataStorage access: %w", err)
	}
	// The above grant is scoped to gatewayNamespace (the SHARED DataStorage
	// instance's own auth.MiddlewareConfig.Namespace), but this test queries
	// the DEDICATED, ISOLATED instance instead -- whose own SAR namespace is
	// GatewayResilienceDataStorageNamespace. Without this second, explicitly
	// scoped RoleBinding, the query below gets a 403 on every attempt and
	// this Eventually spins for its full budget with total staying 0.
	if err := infrastructure.GrantDataStorageAccessInNamespace(
		reqCtx, kubeconfigPath, infrastructure.GatewayResilienceDataStorageNamespace, gatewayNamespace, e2eSAName, GinkgoWriter,
	); err != nil {
		return fmt.Errorf("failed to grant resilience-test audit ServiceAccount access scoped to the isolated DataStorage namespace: %w", err)
	}
	e2eToken, err := infrastructure.GetServiceAccountToken(reqCtx, gatewayNamespace, e2eSAName, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get resilience-test ServiceAccount token: %w", err)
	}

	// BR-SCOPE-002: Gateway rejects (HTTP 200, StatusRejected) signals for
	// resources it doesn't consider "managed" -- a Pod inherits managed
	// status from its namespace unless labeled itself (pkg/shared/scope).
	// kubernaut-system (gatewayNamespace) is the shared system namespace
	// and is never labeled kubernaut.ai/managed=true, so posting the alert
	// there (as this test previously did) is unconditionally rejected
	// regardless of RBAC/connectivity -- root-caused via must-gather log
	// from CI run 31889224478 (got HTTP 200, not the expected 201).
	// Every sibling spec in this suite (e.g. 15_audit_trace_validation_test.go,
	// 32_service_resilience_test.go) avoids this by targeting a dedicated,
	// freshly created namespace instead of kubernaut-system directly --
	// helpers.CreateTestNamespaceAndWait labels it kubernaut.ai/managed=true
	// by default. Mirror that pattern here rather than mutating the shared
	// system namespace's labels.
	alertNamespace := helpers.CreateTestNamespaceAndWait(k8sClient, "ds-resilience")
	podName := "ds-resilience-test-pod"
	helpers.EnsureTestPod(reqCtx, k8sClient, alertNamespace, podName)

	resilienceGatewayURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayResilienceAPIHostPort)
	alertPayload := createPrometheusWebhookPayload(PrometheusAlertPayload{
		AlertName: "DataStorageResilienceTestAlert",
		Namespace: alertNamespace,
		Severity:  "critical",
		PodName:   podName,
		Annotations: map[string]string{
			"summary":     "Post-recovery signal for #1985 DataStorage resilience E2E",
			"description": "Proves a gapless, queryable-by-correlation_id audit trail after the readiness gate self-heals",
		},
	})

	// The request is (re)built fresh on every Eventually attempt (mirroring
	// 15_audit_trace_validation_test.go), not once beforehand: a
	// *http.Request built with a bytes.Buffer body can only be sent once --
	// http.Client.Do drains the body reader, so a second Do() on the same
	// *http.Request silently fails client-side (ContentLength/body-length
	// mismatch) rather than reaching the server. Building it once outside
	// this closure made every retry after the first return status 0,
	// masking the real (now-fixed) first-attempt failures above.
	var resp *http.Response
	Eventually(func() int {
		req, buildErr := http.NewRequestWithContext(reqCtx, http.MethodPost,
			resilienceGatewayURL+"/api/v1/signals/prometheus", bytes.NewBuffer(alertPayload))
		if buildErr != nil {
			return 0
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", e2eToken))

		var doErr error
		resp, doErr = http.DefaultClient.Do(req) //nolint:bodyclose // closed below once loop exits
		if doErr != nil {
			return 0
		}
		return resp.StatusCode
	}, "30s", "1s").Should(Equal(http.StatusCreated),
		"the recovered gateway-resilience instance must accept and process the post-recovery signal")
	defer func() { _ = resp.Body.Close() }()

	var gatewayResp struct {
		RemediationRequestName string `json:"remediationRequestName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gatewayResp); err != nil {
		return fmt.Errorf("failed to decode gateway-resilience signal response: %w", err)
	}
	correlationID := gatewayResp.RemediationRequestName
	if correlationID == "" {
		return fmt.Errorf("gateway-resilience response did not include a correlation ID (remediationRequestName)")
	}

	// #1985 follow-up (2026-08-16): queries the DEDICATED, ISOLATED
	// DataStorage instance (GatewayResilienceDataStorageNamespace), not
	// the shared one -- gateway-resilience's datastorage.url points there
	// too (see DeployGatewayForDataStorageResilienceTest), so this is
	// where the signal's audit trail actually landed. Plain HTTP: the
	// isolated instance has no TLS cert configured.
	dataStorageURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayResilienceDataStorageAPIHostPort)
	saTransport := testauth.NewServiceAccountTransport(e2eToken)
	httpClient := &http.Client{Timeout: 20 * time.Second, Transport: saTransport}
	auditClient, err := dsgen.NewClient(dataStorageURL, dsgen.WithClient(httpClient))
	if err != nil {
		return fmt.Errorf("failed to create audit client: %w", err)
	}

	var total int
	Eventually(func() int {
		resp, queryErr := auditClient.QueryAuditEvents(reqCtx, dsgen.QueryAuditEventsParams{
			CorrelationID: dsgen.NewOptString(correlationID),
		})
		if queryErr != nil {
			return 0
		}
		if resp.Pagination.Set && resp.Pagination.Value.Total.Set {
			total = resp.Pagination.Value.Total.Value
		}
		return total
	}, "30s", "2s").Should(BeNumerically(">=", 1),
		"SOC2 CC8.1: the post-recovery signal's complete lifecycle must be reconstructable from audit traces "+
			"alone via correlation_id -- a gap here would mean the readiness gate merely delayed the #1985 "+
			"audit-loss race rather than eliminating it")

	return nil
}
