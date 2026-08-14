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

package effectivenessmonitor

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention

	dsshared "github.com/jordigilh/kubernaut/test/e2e/datastorage/shared"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// DataStorage Resilience -- the ctrl-runtime representative service for the
// #1985 shared E2E journey (see plan's "Coverage note": one ctrl-runtime
// service (this one) + one custom-aggregator service
// (test/e2e/gateway/39_datastorage_resilience_test.go, which also proves
// the post-recovery audit-trail half) prove the shared *mechanism*; the
// other 8 in-scope services are proven at IT tier,
// IT-AUDIT-1985-003..012).
//
// This journey proves the readyz-flip half only (real network partition ->
// /readyz 503 -> reconnect -> self-heal to 200): EM's own audit-write path
// is CRD-reconciliation-driven (EffectivenessAssessment events), not
// simply HTTP-triggerable like Gateway's webhook endpoint, so driving a
// full post-recovery audit-trail assertion here would require standing up
// a real RemediationRequest/EffectivenessAssessment fixture in the
// dedicated instance's own isolated reconciliation loop -- disproportionate
// to what this E2E tier needs to add on top of Gateway's already-complete
// full journey and EM's own IT-tier wiring proof (IT-AUDIT-1985-003).
//
// Runs against a dedicated, throwaway "effectivenessmonitor-resilience"
// instance (never the shared EM controller every other spec in this suite
// depends on) so this test carries zero blast radius and needs no Serial
// decorator.
//
// Business Requirements: BR-AUDIT-005 v2.0.
// Authority: Issue #1985, SOC2 CC8.1, AU-9.
const (
	emResilienceBridgeServiceName = "datastorage-fault-proxy-em"
	emResilienceBridgePort        = 8081
	// emResilienceDataStorageHealthHostAddr: DataStorage's real health
	// NodePort (30281) is already host-mapped 1:1 for this suite
	// (kind-effectivenessmonitor-config.yaml) -- reused directly, no
	// kubectl port-forward subprocess needed.
	emResilienceDataStorageHealthHostAddr = "127.0.0.1:30281"
)

var _ = dsshared.Journey("DataStorage Resilience (#1985, BR-AUDIT-005 v2.0, SOC2 CC8.1)", func() dsshared.Target {
	return dsshared.Target{
		KubeconfigPath:            kubeconfigPath,
		Namespace:                 controllerNamespace,
		DataStorageHealthHostAddr: emResilienceDataStorageHealthHostAddr,
		BridgeServiceName:         emResilienceBridgeServiceName,
		BridgePort:                emResilienceBridgePort,
		Deploy: func(ctx context.Context) error {
			return infrastructure.DeployEMForDataStorageResilienceTest(
				ctx, kubeconfigPath, controllerNamespace, emResilienceBridgeServiceName, emResilienceBridgePort, GinkgoWriter)
		},
		Teardown: func(ctx context.Context) {
			infrastructure.TeardownEMForDataStorageResilienceTest(ctx, kubeconfigPath, controllerNamespace, GinkgoWriter)
		},
		ReadyzURL: fmt.Sprintf("http://127.0.0.1:%d/readyz", infrastructure.EMResilienceHealthHostPort),
		// TriggerAndVerifyAudit intentionally left nil -- see package doc above.
	}
})
