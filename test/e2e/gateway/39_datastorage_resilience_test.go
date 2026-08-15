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

// Test 39: DataStorage Resilience -- the custom-aggregator representative
// service for the #1985 shared E2E journey (see plan's "Coverage note":
// one ctrl-runtime service (EffectivenessMonitor,
// test/e2e/effectivenessmonitor/06_datastorage_resilience_test.go) + one
// custom-aggregator service prove the shared *mechanism*; the other 8
// in-scope services are proven at IT tier, IT-AUDIT-1985-003..012).
//
// Runs against a dedicated, throwaway "gateway-resilience" instance (never
// the shared "gateway" instance every other spec in this suite depends on)
// so this test carries zero blast radius and needs no Serial decorator.
//
// Business Requirements: BR-AUDIT-005 v2.0, BR-GATEWAY-190.
// Authority: Issue #1985, SOC2 CC8.1, AU-9.
var _ = dsshared.Journey("Test 39: DataStorage Resilience (#1985, BR-AUDIT-005 v2.0, SOC2 CC8.1)", func() dsshared.Target {
	return dsshared.Target{
		KubeconfigPath:      kubeconfigPath,
		Namespace:           gatewayNamespace,
		PodLabelSelector:    "app=gateway-resilience",
		BridgeContainerName: infrastructure.GatewayResilienceBridgeContainerName,
		BridgeProxyPort:     infrastructure.GatewayResilienceBridgeProxyPort,
		// DataStorageUpstreamAddr: the REAL, shared DataStorage instance's
		// health endpoint, reachable from the bridge-proxy sidecar via
		// ordinary namespace-qualified cluster DNS -- see
		// dsshared.Journey's doc comment for why a sidecar replaced the
		// earlier host-bridge and in-cluster-Deployment mechanisms (#1985
		// follow-up, 2026-08-15). Computed here (not as a package-level
		// var) since gatewayNamespace is only populated inside
		// SynchronizedBeforeSuite, after package-level var initializers
		// would already have run.
		DataStorageUpstreamAddr: fmt.Sprintf("data-storage-service.%s.svc.cluster.local:8081", gatewayNamespace),
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
	e2eToken, err := infrastructure.GetServiceAccountToken(reqCtx, gatewayNamespace, e2eSAName, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get resilience-test ServiceAccount token: %w", err)
	}

	// The owner-resolver (BR-GATEWAY-185) queries the real K8s API for the
	// Pod named below and drops the signal (batch parse failure) if it
	// doesn't exist -- mirrors 15_audit_trace_validation_test.go's
	// helpers.EnsureTestPod call, without which this alert is unconditionally
	// rejected regardless of Gateway/DataStorage RBAC (root-caused via
	// must-gather log from CI run 31886951674: "Pod \"ds-resilience-test-pod\"
	// not found").
	podName := "ds-resilience-test-pod"
	helpers.EnsureTestPod(reqCtx, k8sClient, gatewayNamespace, podName)

	resilienceGatewayURL := fmt.Sprintf("http://127.0.0.1:%d", infrastructure.GatewayResilienceAPIHostPort)
	alertPayload := createPrometheusWebhookPayload(PrometheusAlertPayload{
		AlertName: "DataStorageResilienceTestAlert",
		Namespace: gatewayNamespace,
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

	dataStorageURL := fmt.Sprintf("https://127.0.0.1:%d", infrastructure.DataStorageE2EHostPort)
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
