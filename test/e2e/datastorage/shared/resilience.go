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
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// PortForward manages a kubectl port-forward subprocess. Exported (unlike
// the near-identical private helper in test/infrastructure/migrations.go,
// which is hardcoded to PostgreSQL on port 5432) so this package can tunnel
// to an arbitrary Service:port -- here, the REAL DataStorage's health
// Service, so the host-side InterruptibleProxy below has a genuine
// dependency to forward to.
type PortForward struct {
	cmd       *exec.Cmd
	LocalPort int
}

// StartPortForward opens a `kubectl port-forward` tunnel from a random local
// port to target (e.g. "service/data-storage-service") on remotePort inside
// namespace, blocking until the tunnel actually accepts TCP connections.
func StartPortForward(ctx context.Context, kubeconfigPath, namespace, target string, remotePort int, writer io.Writer) (*PortForward, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to find available local port: %w", err)
	}
	localPort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	_, _ = fmt.Fprintf(writer, "   🔌 Starting port-forward to %s/%s (localhost:%d -> %d)...\n", namespace, target, localPort, remotePort)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"port-forward", "-n", namespace, target,
		fmt.Sprintf("%d:%d", localPort, remotePort))
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start port-forward: %w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", localPort))
		if dialErr == nil {
			_ = conn.Close()
			_, _ = fmt.Fprintf(writer, "   ✅ Port-forward ready (localhost:%d)\n", localPort)
			return &PortForward{cmd: cmd, LocalPort: localPort}, nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	return nil, fmt.Errorf("port-forward to %s/%s not ready after 30 seconds", namespace, target)
}

// Close terminates the port-forward subprocess.
func (pf *PortForward) Close() {
	if pf == nil || pf.cmd == nil || pf.cmd.Process == nil {
		return
	}
	_ = pf.cmd.Process.Kill()
	_ = pf.cmd.Wait()
}

// Target describes everything the #1985 DataStorage-resilience journey
// needs to know about one dedicated, throwaway instance of a
// service-under-test. The dedicated instance's `datastorage.url` (audit
// writes) stays pointed at the REAL DataStorage instance throughout --
// only its `datastorage.healthUrl` (readiness probe) is repointed at the
// fault-injectable bridge. This is deliberate: it is what lets the journey
// prove a genuine, gapless, queryable-by-correlation_id audit trail after
// recovery, not just a readyz flag flip.
type Target struct {
	// KubeconfigPath, Namespace: where the dedicated instance + the
	// CreateServiceBridge Service/Endpoints live.
	KubeconfigPath string
	Namespace      string

	// DataStorageHealthHostAddr, if set, is a host-reachable "host:port"
	// for the REAL DataStorage's health endpoint (e.g. "127.0.0.1:28091",
	// when the suite's Kind config already NodePort-maps DataStorage's
	// health port -- see kind-gateway-config.yaml's 30281->28091 mapping).
	// Preferred when available: no extra subprocess needed.
	DataStorageHealthHostAddr string

	// DataStorageNamespace/DataStorageHealthService identify the REAL,
	// shared DataStorage instance whose health Service is port-forwarded
	// from the host when DataStorageHealthHostAddr is empty (so the
	// InterruptibleProxy still has a genuine, live dependency to forward
	// to -- this journey never touches or disrupts that real instance
	// itself either way).
	DataStorageNamespace     string
	DataStorageHealthService string

	// BridgeServiceName is the in-cluster DNS name CreateServiceBridge
	// registers in Namespace (e.g. "datastorage-fault-proxy-gw"). Must be
	// unique per concurrently-running Target, since CreateServiceBridge's
	// hand-authored Endpoints object is keyed by this name.
	BridgeServiceName string
	// BridgePort is the port both the bridge Service and the dedicated
	// instance's healthUrl dial (DataStorage's real health port, 8081).
	BridgePort int

	// Deploy stands up the dedicated, throwaway instance with
	// datastorage.healthUrl already pointed at
	// "http://<BridgeServiceName>:<BridgePort>/readyz", blocking until it
	// reports Ready. Called only after the bridge is already live, so the
	// very first readiness probe succeeds against the (currently
	// unpaused) proxy.
	Deploy func(ctx context.Context) error
	// Teardown removes everything Deploy created. Best-effort: failures
	// are logged, not asserted (mirrors TeardownIsolatedDataStorageInstance).
	Teardown func(ctx context.Context)

	// ReadyzURL is a host-reachable URL for the dedicated instance's own
	// /readyz (a NodePort unique to this dedicated instance, distinct
	// from the shared instance's own readyz NodePort).
	ReadyzURL string

	// TriggerAndVerifyAudit, if non-nil, drives one real business request
	// through the now-recovered dedicated instance and asserts a
	// complete, gapless audit trail is queryable by correlation_id (SOC2
	// CC8.1). Optional: services whose audit-write path is not simply
	// HTTP-triggerable (e.g. a CRD-reconciliation-driven controller like
	// EffectivenessMonitor, which reacts to EffectivenessAssessment
	// events rather than direct requests) may leave this nil -- the
	// readiness-flip half of the journey (the shared *mechanism* this E2E
	// tier exists to prove, per the plan's Coverage note) still runs
	// unconditionally for every Target.
	TriggerAndVerifyAudit func(ctx context.Context) error
}

// Journey runs the #1985 DataStorage-resilience E2E scenario against one
// Target: a goroutine-hosted InterruptibleProxy (bound to 0.0.0.0, forwarding
// to the REAL DataStorage's health endpoint via a kubectl port-forward) is
// bridged into the cluster via CreateServiceBridge + KindBridgeGatewayIP
// (Spike S19/DD-TEST-013, empirically re-validated for host-to-pod
// reachability 2026-08-14), fronting Target's dedicated, throwaway service
// instance instead of the real data-storage-service.
//
// Proves what no lower tier can:
//   - UT (pkg/audit/datastorage_prober_test.go) only proves Probe's
//     branching logic against an httptest.Server.
//   - IT (test/integration/<service>/datastorage_readiness_*_test.go, x10)
//     only proves each service's readyz surface is wired to a fake/unreachable
//     HTTP target -- never a real network partition to a real DataStorage.
//
// The proxy's Pause()/Resume() (not just DisconnectAll()) is required
// because DataStorageProber opens a fresh HTTP connection every probe
// cycle: a bare DisconnectAll() only breaks connections already in flight,
// so the very next probe would reconnect and succeed. Pause forces every
// new connection attempt to fail closed too, which is what a real,
// sustained DataStorage outage actually looks like.
//
// No Serial needed and zero blast radius to other concurrently-running
// specs: the real DataStorage instance is never touched (only a
// port-forwarded READ of its health endpoint), and only this Target's own
// dedicated, throwaway instance is ever partitioned.
//
// Authority: Issue #1985, BR-AUDIT-005 v2.0, SOC2 CC8.1, AU-9.
func Journey(name string, targetFn func() Target) bool {
	return Describe(name, Ordered, func() {
		var (
			target Target
			proxy  *infrastructure.InterruptibleProxy
			pf     *PortForward
		)

		BeforeAll(func() {
			// ENVIRONMENT-GATED SKIP, NOT a TDD "deferred implementation"
			// anti-pattern (AGENTS.md TDD Anti-Patterns table: "Pending
			// Tests | Using XIt or Skip()"). This test is fully implemented
			// and passes on Linux (CI and Linux dev boxes, e.g. helios08) --
			// confirmed by empirical spike, 2026-08-14 (see #1985 plan
			// notes). Skipped ONLY on macOS: Podman Machine's gvproxy
			// network backend (used by CreateServiceBridge to reach the
			// host-side InterruptibleProxy) never forwards arbitrary
			// guest-to-host TCP connections -- confirmed with a VPN both
			// present and fully disconnected (identical failure both times,
			// including via the host's real LAN IP, not just
			// host.containers.internal); gvproxy's own log shows zero dial
			// attempts for the test port even with UserModeNetworking
			// enabled, meaning it fast-rejects unregistered ports by design
			// rather than being blocked by a fixable local misconfiguration.
			// This is a structural macOS/Podman-Machine limitation, not an
			// untested or unimplemented code path. Precedent:
			// test/infrastructure/notification_e2e.go's GetE2EFileOutputDir
			// has the same runtime.GOOS == "darwin" / Podman VM limitation
			// gate.
			if runtime.GOOS == "darwin" {
				Skip("requires a real Linux Podman host for CreateServiceBridge host-forwarding -- " +
					"Podman Machine's gvproxy on macOS does not forward arbitrary guest-to-host TCP " +
					"connections; run on Linux (CI or a Linux dev box) instead")
			}

			target = targetFn()

			bgCtx := context.Background()
			var err error

			realDSAddr := target.DataStorageHealthHostAddr
			if realDSAddr == "" {
				pf, err = StartPortForward(bgCtx, target.KubeconfigPath, target.DataStorageNamespace,
					"service/"+target.DataStorageHealthService, target.BridgePort, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred(), "must be able to port-forward to the real DataStorage health service")
				realDSAddr = fmt.Sprintf("127.0.0.1:%d", pf.LocalPort)
			}

			proxy, err = infrastructure.NewInterruptibleProxyOn("0.0.0.0:0", realDSAddr)
			Expect(err).NotTo(HaveOccurred(), "the host-side fault-injection proxy must bind successfully")

			gatewayIP, err := infrastructure.KindBridgeGatewayIP(bgCtx, "kind")
			Expect(err).NotTo(HaveOccurred(), "must resolve the podman 'kind' bridge network's gateway IP")

			_, portStr, err := net.SplitHostPort(proxy.Addr())
			Expect(err).NotTo(HaveOccurred())
			proxyPort, err := strconv.Atoi(portStr)
			Expect(err).NotTo(HaveOccurred())

			Expect(infrastructure.CreateServiceBridge(bgCtx, target.KubeconfigPath, target.Namespace,
				target.BridgeServiceName, target.BridgePort, gatewayIP, proxyPort, GinkgoWriter)).To(Succeed(),
				"bridging the host-side proxy into the cluster must succeed")

			Expect(target.Deploy(bgCtx)).To(Succeed(),
				"the dedicated throwaway instance must deploy and become Ready before the partition is induced")
		})

		AfterAll(func() {
			bgCtx := context.Background()
			if target.Teardown != nil {
				target.Teardown(bgCtx)
			}
			if proxy != nil {
				proxy.Close()
			}
			if pf != nil {
				pf.Close()
			}
		})

		It("flips /readyz to 503 on a real DataStorage network partition and self-heals with a gapless post-recovery audit trail (SOC2 CC8.1, AU-9)", func() {
			By("confirming baseline: the dedicated instance's /readyz is healthy before the partition")
			Eventually(func() int {
				return readyzStatus(target.ReadyzURL)
			}, "60s", "2s").Should(Equal(http.StatusOK),
				"baseline /readyz must be healthy once the dedicated instance is deployed and the bridge is intact")

			By("pausing the proxy: simulates a real, sustained network partition to DataStorage")
			proxy.Pause()

			By("verifying the dedicated instance fails closed: /readyz reports 503 (no silent false-healthy)")
			Eventually(func() int {
				return readyzStatus(target.ReadyzURL)
			}, "45s", "1s").Should(Equal(http.StatusServiceUnavailable),
				"#1985: /readyz must report 503 while DataStorage is unreachable -- this closes the audit-loss "+
					"window at the root, since Kubernetes removes the pod from Service endpoints before it can "+
					"serve traffic (and generate audit events) against an unreachable DataStorage")

			By("resuming the proxy: DataStorage becomes reachable again")
			proxy.Resume()

			By("verifying the dedicated instance self-heals: /readyz recovers to 200 without a restart")
			Eventually(func() int {
				return readyzStatus(target.ReadyzURL)
			}, "45s", "1s").Should(Equal(http.StatusOK),
				"the readiness gate must recover once DataStorage's health endpoint is reachable again")

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
