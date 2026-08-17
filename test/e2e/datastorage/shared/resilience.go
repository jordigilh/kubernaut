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

// Package shared implements the #1985 DataStorage-resilience E2E journey,
// reused by every representative service's own E2E suite (one ctrl-runtime
// service, one custom-aggregator service -- see plan's "Coverage note").
package shared

import (
	"context"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// Target describes everything the #1985 DataStorage-resilience journey
// needs to know about one dedicated, throwaway instance of a
// service-under-test. Both the dedicated instance's `datastorage.url`
// (audit writes) and `datastorage.healthUrl` (readiness probe) point at
// the SAME dedicated, isolated DataStorage instance (DD-PLATFORM-010 puts
// both on the same host:port anyway) -- a real, dedicated DataStorage
// instance is just as capable of proving a genuine, gapless,
// queryable-by-correlation_id post-recovery audit trail (SOC2 CC8.1) as
// the shared one, with the added hygiene benefit of never polluting the
// shared instance's audit table with resilience-test noise.
type Target struct {
	// KubeconfigPath is shared by the service-under-test's dedicated
	// instance and its dedicated DataStorage instance -- both live in the
	// SAME Kind cluster, different namespaces.
	KubeconfigPath string

	// DataStorageNamespace is where the dedicated, isolated DataStorage
	// instance (deployed via infrastructure.DeployIsolatedDataStorageInstance,
	// normally from within Deploy below) lives. Journey scales its
	// "datastorage" Deployment to 0/1 replicas directly via
	// infrastructure.ScaleIsolatedDataStorageDown/Up -- true black-box:
	// no sidecar, no proxy, no production code changes, a real Kubernetes
	// Service-endpoint removal exactly like
	// test/e2e/fleetmetadatacache/shared/resilience.go's proven Valkey
	// scale-to-0 pattern.
	DataStorageNamespace string

	// Deploy stands up BOTH the dedicated, isolated DataStorage instance
	// and the dedicated, throwaway service-under-test instance wired to
	// it (datastorage.url/healthUrl both pointed at DataStorageNamespace),
	// blocking until the service-under-test instance reports Ready.
	Deploy func(ctx context.Context) error
	// Teardown removes everything Deploy created (both instances).
	// Best-effort: failures are logged, not asserted (mirrors
	// TeardownIsolatedDataStorageInstance).
	Teardown func(ctx context.Context)

	// ReadyzURL is a host-reachable URL for the dedicated instance's own
	// /readyz (a NodePort unique to this dedicated instance, distinct
	// from the shared instance's own readyz NodePort).
	ReadyzURL string

	// UnavailableStatusCode is the HTTP status /readyz must return while
	// DataStorage is unreachable. Defaults to http.StatusServiceUnavailable
	// (503) if left zero -- correct for custom-aggregator services with a
	// hand-rolled readyz handler (e.g. Gateway, which explicitly writes
	// 503 -- pkg/gateway/server.go's writeReadinessUnavailable). A
	// ctrl-runtime service (e.g. EffectivenessMonitor) delegates its
	// entire /readyz to controller-runtime's own generic healthz.Handler,
	// which unconditionally writes http.StatusInternalServerError (500)
	// for ANY failed check regardless of cause (confirmed by
	// controller-runtime's own manager_test.go) -- such Targets MUST set
	// this to http.StatusInternalServerError explicitly.
	UnavailableStatusCode int

	// TriggerAndVerifyAudit, if non-nil, drives one real business request
	// through the now-recovered dedicated instance and asserts a
	// complete, gapless audit trail is queryable by correlation_id (SOC2
	// CC8.1) from the dedicated, isolated DataStorage instance. Optional:
	// services whose audit-write path is not simply HTTP-triggerable
	// (e.g. a CRD-reconciliation-driven controller like
	// EffectivenessMonitor, which reacts to EffectivenessAssessment
	// events rather than direct requests) may leave this nil -- the
	// readiness-flip half of the journey (the shared *mechanism* this E2E
	// tier exists to prove, per the plan's Coverage note) still runs
	// unconditionally for every Target.
	TriggerAndVerifyAudit func(ctx context.Context) error
}

// Journey runs the #1985 DataStorage-resilience E2E scenario against one
// Target: a dedicated, isolated DataStorage instance (deployed by
// Target.Deploy) backing Target's dedicated, throwaway service instance.
// "Partition"/"recover" are implemented by scaling the isolated instance's
// own "datastorage" Deployment to 0 then back to 1 replicas -- a real
// Kubernetes Service-endpoint removal, not a proxy or a code change.
//
// REPLACES (#1985 follow-up, 2026-08-16) two earlier designs:
//  1. The original host-side InterruptibleProxy +
//     CreateServiceBridge/KindBridgeGatewayIP Podman-bridge mechanism: that
//     design relied on a Kind pod reaching a host-bound TCP listener via
//     the Podman bridge gateway IP, which only works under rootful Podman
//     -- confirmed broken under rootless Podman on macOS dev boxes (the
//     concern that originally motivated this mechanism's replacement); it
//     was never actually confirmed broken on this project's Linux CI.
//  2. An in-pod socat "bridge-proxy" sidecar, killed/relaunched via
//     `kubectl exec` to simulate a partition of DataStorage's health
//     endpoint while `datastorage.url` (audit writes) stayed pointed at
//     the REAL, shared DataStorage instance throughout. This achieved
//     black-box fault injection but was not black-box *environment
//     fidelity*: it required a purpose-built test sidecar/process in the
//     Pod under test, and DD-PLATFORM-010's move of /readyz onto the
//     same HTTPS port as the main API introduced a TLS hostname-
//     verification wrinkle (dialing 127.0.0.1 through the sidecar vs. the
//     certificate's real SANs) that needed its own production-adjacent
//     workaround (an e2e-only build-tag override of the readiness gate's
//     HTTP client). Scaling a dedicated, isolated DataStorage Deployment
//     to 0/1 replicas needs none of that: zero sidecars, zero TLS
//     overrides, zero e2e-only code paths in cmd/gateway or
//     cmd/effectivenessmonitor -- exactly mirroring the proven pattern in
//     test/e2e/fleetmetadatacache/shared/resilience.go's real Valkey
//     outage induction.
//
// Proves what no lower tier can:
//   - UT (pkg/audit/datastorage_prober_test.go) only proves Probe's
//     branching logic against an httptest.Server.
//   - IT (test/integration/<service>/datastorage_readiness_*_test.go, x10)
//     only proves each service's readyz surface is wired to a fake/unreachable
//     HTTP target -- never a real network partition to a real DataStorage.
//
// No Serial needed and zero blast radius to other concurrently-running
// specs: the shared DataStorage instance is never touched, and only this
// Target's own dedicated, isolated DataStorage instance is ever scaled
// down.
//
// Authority: Issue #1985, BR-AUDIT-005 v2.0, SOC2 CC8.1, AU-9.
func Journey(name string, targetFn func() Target) bool {
	return Describe(name, Ordered, func() {
		var target Target

		BeforeAll(func() {
			target = targetFn()
			if target.UnavailableStatusCode == 0 {
				target.UnavailableStatusCode = http.StatusServiceUnavailable
			}
			bgCtx := context.Background()

			Expect(target.Deploy(bgCtx)).To(Succeed(),
				"the dedicated throwaway instance and its dedicated, isolated DataStorage instance must deploy and become Ready")
		})

		AfterAll(func() {
			bgCtx := context.Background()
			if target.Teardown != nil {
				target.Teardown(bgCtx)
			}
		})

		It("fails /readyz closed on a real DataStorage outage and self-heals with a gapless post-recovery audit trail (SOC2 CC8.1, AU-9)", func() {
			By("confirming baseline: the dedicated instance's /readyz is healthy before the outage")
			Eventually(func() int {
				return readyzStatus(target.ReadyzURL)
			}, "60s", "2s").Should(Equal(http.StatusOK),
				"baseline /readyz must be healthy once the dedicated instance and its isolated DataStorage instance are deployed")

			By("scaling the dedicated, isolated DataStorage instance to 0 replicas: a real, deterministic outage")
			Expect(infrastructure.ScaleIsolatedDataStorageDown(context.Background(), target.DataStorageNamespace, target.KubeconfigPath, GinkgoWriter)).
				To(Succeed(), "scaling the isolated DataStorage instance to 0 replicas must succeed")

			By(fmt.Sprintf("verifying the dedicated instance fails closed: /readyz reports %d (no silent false-healthy)", target.UnavailableStatusCode))
			Eventually(func() int {
				return readyzStatus(target.ReadyzURL)
			}, "45s", "1s").Should(Equal(target.UnavailableStatusCode),
				fmt.Sprintf("#1985: /readyz must report %d while DataStorage is unreachable -- this closes the "+
					"audit-loss window at the root, since Kubernetes removes the pod from Service endpoints "+
					"before it can serve traffic (and generate audit events) against an unreachable DataStorage",
					target.UnavailableStatusCode))

			By("scaling the dedicated, isolated DataStorage instance back to 1 replica to let it self-heal")
			Expect(infrastructure.ScaleIsolatedDataStorageUp(context.Background(), target.DataStorageNamespace, target.KubeconfigPath, GinkgoWriter)).
				To(Succeed(), "scaling the isolated DataStorage instance back to 1 replica must succeed")

			By("verifying the dedicated instance self-heals: /readyz recovers to 200 without a restart")
			Eventually(func() int {
				return readyzStatus(target.ReadyzURL)
			}, "45s", "1s").Should(Equal(http.StatusOK),
				"the readiness gate must recover once DataStorage is reachable again")

			if target.TriggerAndVerifyAudit != nil {
				By("BUSINESS OUTCOME (SOC2 CC8.1): a request made after recovery produces a complete, gapless audit trail")
				Expect(target.TriggerAndVerifyAudit(context.Background())).To(Succeed(),
					"a post-recovery request must be fully auditable end-to-end, proving the readiness gate "+
						"closed the audit-loss window rather than merely delaying it")
			}
		})
	})
}

func readyzStatus(url string) int {
	resp, err := http.Get(url) //nolint:gosec,noctx // E2E test polling a Kind-cluster NodePort, not user input
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}
