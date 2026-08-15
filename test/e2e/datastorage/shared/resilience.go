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
	"net/http"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:staticcheck // Ginkgo DSL dot-import convention
	. "github.com/onsi/gomega"    //nolint:staticcheck // Ginkgo/Gomega DSL dot-import convention
)

// Target describes everything the #1985 DataStorage-resilience journey
// needs to know about one dedicated, throwaway instance of a
// service-under-test. The dedicated instance's `datastorage.url` (audit
// writes) stays pointed at the REAL DataStorage instance throughout --
// only its `datastorage.healthUrl` (readiness probe) is repointed at the
// fault-injectable bridge-proxy sidecar (localhost:BridgeProxyPort). This
// is deliberate: it is what lets the journey prove a genuine, gapless,
// queryable-by-correlation_id audit trail after recovery, not just a
// readyz flag flip.
type Target struct {
	// KubeconfigPath, Namespace: where the dedicated instance (and its
	// bridge-proxy sidecar, in the SAME Pod) lives.
	KubeconfigPath string
	Namespace      string

	// PodLabelSelector selects the single Pod running both the dedicated
	// instance's main container and its bridge-proxy sidecar (e.g.
	// "app=gateway-resilience"). Resolved once, right after Deploy
	// succeeds, to a concrete Pod name used for every subsequent
	// `kubectl exec`.
	PodLabelSelector string
	// BridgeContainerName is the bridge-proxy sidecar container's name
	// within that Pod (e.g. infrastructure.GatewayResilienceBridgeContainerName).
	BridgeContainerName string
	// BridgeProxyPort/DataStorageUpstreamAddr must exactly match the
	// socat TCP-LISTEN port and forward target already baked into the
	// sidecar's manifest by Deploy (see Deploy*ForDataStorageResilienceTest)
	// -- needed to reissue an identical `socat -d TCP-LISTEN:<port>,...
	// TCP:<upstream>` command when relaunching it after a simulated
	// partition.
	BridgeProxyPort         int
	DataStorageUpstreamAddr string

	// Deploy stands up the dedicated, throwaway instance (main container
	// plus the bridge-proxy sidecar, both already running and the
	// sidecar already forwarding), blocking until it reports Ready.
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
// Target: a socat TCP-forwarding sidecar (bridgeProxy) in the SAME Pod as
// Target's dedicated, throwaway service instance fronts the REAL
// DataStorage's health endpoint instead of the real data-storage-service.
// "Partition"/"recover" are implemented by killing/relaunching the socat
// process in-place via `kubectl exec` (not scaling, not signaling the
// whole container): killing it closes its listening socket outright, so
// new connections get an immediate ECONNREFUSED -- the same fail-fast
// symptom a real, sustained DataStorage outage produces from the probing
// pod's perspective. (A SIGSTOP-based pause was deliberately rejected: it
// would leave the socket bound, so the kernel completes the TCP handshake
// for new connections and the caller hangs until its own client timeout
// instead of failing fast, making the test's timing depend on an
// unrelated client-side timeout rather than exercising the fail-closed
// path directly.)
//
// REPLACES (#1985 follow-up, 2026-08-15) two earlier designs:
//  1. The original host-side InterruptibleProxy +
//     CreateServiceBridge/KindBridgeGatewayIP Podman-bridge mechanism: that
//     design relied on a Kind pod reaching a host-bound TCP listener via
//     the Podman bridge gateway IP, which only works under rootful Podman
//     -- confirmed broken under rootless Podman on both GitHub-hosted CI
//     runners and a Linux dev box (helios08), which is what this project's
//     CI actually runs (`.github/actions/install-podman` installs Podman
//     with no rootful configuration). A rootful-CI spike independently
//     confirmed the pod-to-host path itself works under rootful Podman,
//     but wiring rootful Podman into this CI pipeline's artifact-based (no
//     registry) image-loading model would require duplicating every
//     loaded image into a second, root Podman store just for the two
//     matrix entries that need it.
//  2. An interim in-cluster socat Deployment+Service (scaled 0/1 replicas
//     to partition/recover): correctly avoided the rootless-Podman problem
//     above, but added a second Deployment+Service per Target purely to
//     host a TCP forwarder. Folding the forwarder into a sidecar container
//     of Target's own dedicated-instance Pod achieves the same fail-fast,
//     runtime-agnostic partition semantics with zero extra Kubernetes
//     objects.
//
// Proves what no lower tier can:
//   - UT (pkg/audit/datastorage_prober_test.go) only proves Probe's
//     branching logic against an httptest.Server.
//   - IT (test/integration/<service>/datastorage_readiness_*_test.go, x10)
//     only proves each service's readyz surface is wired to a fake/unreachable
//     HTTP target -- never a real network partition to a real DataStorage.
//
// No Serial needed and zero blast radius to other concurrently-running
// specs: the real DataStorage instance is never touched (only dialed by the
// bridge-proxy sidecar's own forwarding), and only this Target's own
// dedicated, throwaway Pod is ever partitioned.
//
// Authority: Issue #1985, BR-AUDIT-005 v2.0, SOC2 CC8.1, AU-9.
func Journey(name string, targetFn func() Target) bool {
	return Describe(name, Ordered, func() {
		var (
			target Target
			bridge *bridgeProxy
		)

		BeforeAll(func() {
			target = targetFn()
			bgCtx := context.Background()

			Expect(target.Deploy(bgCtx)).To(Succeed(),
				"the dedicated throwaway instance (plus its bridge-proxy sidecar) must deploy and become Ready")

			bridge = &bridgeProxy{
				kubeconfigPath:   target.KubeconfigPath,
				namespace:        target.Namespace,
				podLabelSelector: target.PodLabelSelector,
				containerName:    target.BridgeContainerName,
				port:             target.BridgeProxyPort,
				upstreamAddr:     target.DataStorageUpstreamAddr,
				writer:           GinkgoWriter,
			}
			Expect(bridge.resolvePod(bgCtx)).To(Succeed(),
				"must resolve the dedicated instance's Pod name (selector %q) before it can be exec'd into", target.PodLabelSelector)
		})

		AfterAll(func() {
			bgCtx := context.Background()
			if target.Teardown != nil {
				target.Teardown(bgCtx)
			}
		})

		It("flips /readyz to 503 on a real DataStorage network partition and self-heals with a gapless post-recovery audit trail (SOC2 CC8.1, AU-9)", func() {
			By("confirming baseline: the dedicated instance's /readyz is healthy before the partition")
			Eventually(func() int {
				return readyzStatus(target.ReadyzURL)
			}, "60s", "2s").Should(Equal(http.StatusOK),
				"baseline /readyz must be healthy once the dedicated instance is deployed and the bridge-proxy sidecar is intact")

			By("killing the bridge-proxy sidecar's socat process: simulates a real, sustained network partition to DataStorage")
			Expect(bridge.pause(context.Background())).To(Succeed(), "killing the bridge-proxy sidecar's socat process must succeed")

			By("verifying the dedicated instance fails closed: /readyz reports 503 (no silent false-healthy)")
			Eventually(func() int {
				return readyzStatus(target.ReadyzURL)
			}, "45s", "1s").Should(Equal(http.StatusServiceUnavailable),
				"#1985: /readyz must report 503 while DataStorage is unreachable -- this closes the audit-loss "+
					"window at the root, since Kubernetes removes the pod from Service endpoints before it can "+
					"serve traffic (and generate audit events) against an unreachable DataStorage")

			By("relaunching the bridge-proxy sidecar's socat process: DataStorage becomes reachable again")
			Expect(bridge.resume(context.Background())).To(Succeed(), "relaunching the bridge-proxy sidecar's socat process must succeed")

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

// bridgeProxy controls the pre-existing bridge-proxy sidecar container
// (deployed as part of Target.Deploy, in the SAME Pod as the dedicated
// instance under test) that fronts the REAL DataStorage's health endpoint
// via socat (#1985 follow-up, 2026-08-15 -- see Journey's doc comment for
// the full history of what this replaced). "Partition" and "recover" are
// implemented by killing/relaunching socat in-place via `kubectl exec`,
// which works identically regardless of container runtime or
// rootful/rootless Podman, and does not disturb the main container's own
// process (and therefore its own /readyz handler) at all.
//
// The sidecar's own entrypoint (baked into its container manifest by
// Deploy*ForDataStorageResilienceTest) starts socat once at Pod startup,
// redirects its output to a log file, and records its PID to
// /tmp/socat.pid, e.g.:
//
//	socat -d TCP-LISTEN:<port>,fork,reuseaddr TCP:<upstream> > /tmp/socat.log 2>&1 & echo $! > /tmp/socat.pid; while true; do sleep 3600; done
//
// PID 1 in that container is the wrapping shell's infinite sleep loop, not
// socat itself, so killing socat never restarts or exits the container.
// The output redirect matters just as much for resume's relaunch (see
// resume's own doc comment) as it does here: leaving a backgrounded
// process's stdout/stderr attached to whichever stream started it (a
// `kubectl exec` session, in resume's case) prevents that stream from
// ever closing.
type bridgeProxy struct {
	kubeconfigPath   string
	namespace        string
	podLabelSelector string
	containerName    string
	port             int
	upstreamAddr     string
	writer           io.Writer

	podName string
}

// resolvePod finds the single Pod matching podLabelSelector and caches its
// name for every subsequent `kubectl exec`. Must be called once, after
// Target.Deploy has created the Pod.
func (b *bridgeProxy) resolvePod(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", b.kubeconfigPath, "-n", b.namespace,
		"get", "pods", "-l", b.podLabelSelector, "-o", "jsonpath={.items[0].metadata.name}")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to resolve pod for selector %q in namespace %s: %w", b.podLabelSelector, b.namespace, err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return fmt.Errorf("no pod found matching selector %q in namespace %s", b.podLabelSelector, b.namespace)
	}
	b.podName = name
	return nil
}

// pause kills the sidecar's socat process, closing its listening socket
// outright: new connection attempts from the dedicated instance's own
// readiness probe get an immediate ECONNREFUSED (fail-fast), the same
// symptom a real, sustained DataStorage outage produces.
func (b *bridgeProxy) pause(ctx context.Context) error {
	cmd := b.execCmd(ctx, "sh", "-c", "kill $(cat /tmp/socat.pid) 2>/dev/null; rm -f /tmp/socat.pid")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to kill bridge-proxy socat process in pod %s/%s: %w", b.namespace, b.podName, err)
	}
	return nil
}

// resume relaunches socat inside the sidecar with the exact same
// TCP-LISTEN/forward arguments it started with, restoring connectivity.
// The backgrounded socat's stdout/stderr are explicitly redirected to a
// log file, NOT left attached to this `kubectl exec` session's own
// stream: without that redirect, socat (still running after this
// short-lived shell exits) keeps that stream's pipe open, and `kubectl
// exec` hangs indefinitely waiting for it to close rather than returning
// once the parent shell exits (confirmed empirically on helios08).
func (b *bridgeProxy) resume(ctx context.Context) error {
	relaunch := fmt.Sprintf("socat -d TCP-LISTEN:%d,fork,reuseaddr TCP:%s > /tmp/socat.log 2>&1 & echo $! > /tmp/socat.pid", b.port, b.upstreamAddr)
	cmd := b.execCmd(ctx, "sh", "-c", relaunch)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to relaunch bridge-proxy socat process in pod %s/%s: %w", b.namespace, b.podName, err)
	}
	return nil
}

func (b *bridgeProxy) execCmd(ctx context.Context, command ...string) *exec.Cmd {
	args := append([]string{"--kubeconfig", b.kubeconfigPath, "-n", b.namespace,
		"exec", b.podName, "-c", b.containerName, "--"}, command...)
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdout = b.writer
	cmd.Stderr = b.writer
	return cmd
}
