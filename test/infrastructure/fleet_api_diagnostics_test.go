package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BR-FLEET-054 / BR-INTEGRATION-065: Service VIP remains the readiness gate", func() {
	It("IT-FLEET-2502-004: real collector subprocess retains only bounded named snapshots, never credential files", func() {
		dir := GinkgoT().TempDir()
		source := filepath.Join(dir, "owned-node")
		Expect(os.Mkdir(source, 0700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(source, "service-path-owned.log"), []byte("failure-time Service VIP refusal"), 0600)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(source, "kubeconfig"), []byte("synthetic-credential-marker-2502"), 0600)).To(Succeed())
		GinkgoT().Setenv("FLEET_DIAGNOSTICS_DIR", source)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", filepath.Join(getProjectRoot(), "cmd", "must-gather", "collectors", "fleet-service-path.sh"), filepath.Join(dir, "output"))
		output, err := cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), "collector output %s", output)
		captured, err := os.ReadFile(filepath.Join(dir, "output", "fleet-service-path", "service-path-owned.log"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(captured)).To(ContainSubstring("failure-time Service VIP refusal"))
		files, err := os.ReadDir(filepath.Join(dir, "output", "fleet-service-path"))
		Expect(err).ToNot(HaveOccurred())
		Expect(files).To(HaveLen(1))
	})
	It("IT-FLEET-2502-004: actual external-image invocation includes only its owned diagnostic directory", func() {
		dir := GinkgoT().TempDir()
		GinkgoT().Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		Expect(os.WriteFile(filepath.Join(dir, "kind"), []byte("#!/bin/bash\nprintf 'apiVersion: v1\\nkind: Config\\n'\n"), 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dir, "podman"), []byte("#!/bin/bash\nprintf '%s\\n' \"$@\"\n"), 0755)).To(Succeed())
		var log bytes.Buffer
		owned := filepath.Join(dir, "owned-2502-control-plane")
		Expect(os.Mkdir(owned, 0700)).To(Succeed())
		Expect(RunMustGatherImage(context.Background(), RunMustGatherImageOptions{
			ClusterName: "owned-2502", Image: "test-owned-image", OutputDir: filepath.Join(dir, "output"), FleetDiagnosticsDir: owned,
		}, &log)).To(Succeed())
		Expect(log.String()).To(ContainSubstring(owned + ":/fleet-diagnostics:ro,Z"))
		Expect(log.String()).To(ContainSubstring("FLEET_DIAGNOSTICS_DIR=/fleet-diagnostics"))
		Expect(log.String()).ToNot(ContainSubstring("another-cluster"))
	})
	It("IT-FLEET-2502-004: the archive cap keeps the newest owned failure snapshots and bounds each regular file", func() {
		dir := GinkgoT().TempDir()
		source := filepath.Join(dir, "owned-node")
		Expect(os.Mkdir(source, 0700)).To(Succeed())
		for i := 0; i < 11; i++ {
			name := fmt.Sprintf("service-path-20261011T%06d.log", i)
			Expect(os.WriteFile(filepath.Join(source, name), []byte(strings.Repeat("x", 140000)), 0600)).To(Succeed())
		}
		credential := filepath.Join(source, "token")
		Expect(os.WriteFile(credential, []byte("synthetic-credential-2502"), 0600)).To(Succeed())
		Expect(os.Symlink(credential, filepath.Join(source, "service-path-999999.log"))).To(Succeed())
		GinkgoT().Setenv("FLEET_DIAGNOSTICS_DIR", source)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		outputDir := filepath.Join(dir, "output")
		cmd := exec.CommandContext(ctx, "bash", filepath.Join(getProjectRoot(), "cmd", "must-gather", "collectors", "fleet-service-path.sh"), outputDir)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "collector output: %s", output)
		captured := filepath.Join(outputDir, "fleet-service-path")
		files, err := os.ReadDir(captured)
		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveLen(10))
		_, err = os.Stat(filepath.Join(captured, "service-path-20261011T000010.log"))
		Expect(err).NotTo(HaveOccurred(), "the current failure must not be displaced by old retained snapshots")
		_, err = os.Stat(filepath.Join(captured, "service-path-20261011T000000.log"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		for _, file := range files {
			body, err := os.ReadFile(filepath.Join(captured, file.Name()))
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(HaveLen(131072))
			Expect(string(body)).ToNot(ContainSubstring("synthetic-credential-2502"))
		}
	})
	It("IT-FLEET-2502-004: redaction cannot expand an archived snapshot past its byte cap", func() {
		dir := GinkgoT().TempDir()
		source := filepath.Join(dir, "owned-node")
		Expect(os.Mkdir(source, 0700)).To(Succeed())
		// A short marker expands to [REDACTED]; the cap must bound the archive
		// result, not only the bytes read before redaction.
		Expect(os.WriteFile(filepath.Join(source, "service-path-owned.log"), []byte(strings.Repeat("Bearer x\n", 17000)), 0600)).To(Succeed())
		GinkgoT().Setenv("FLEET_DIAGNOSTICS_DIR", source)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		outputDir := filepath.Join(dir, "output")
		cmd := exec.CommandContext(ctx, "bash", filepath.Join(getProjectRoot(), "cmd", "must-gather", "collectors", "fleet-service-path.sh"), outputDir)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "collector output: %s", output)
		body, err := os.ReadFile(filepath.Join(outputDir, "fleet-service-path", "service-path-owned.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(HaveLen(131072))
		Expect(string(body)).To(ContainSubstring("Bearer [REDACTED]"))
		Expect(string(body)).NotTo(ContainSubstring("Bearer x"))
	})
	It("UT-FLEET-2502-001: executor output is capped while read, with explicit truncation and invoke errors", func() {
		var output boundedFleetOutput
		written, err := output.Write([]byte(strings.Repeat("x", 40000)))
		Expect(err).ToNot(HaveOccurred())
		Expect(written).To(Equal(40000))
		Expect(output.data).To(HaveLen(8192))
		Expect(output.truncated).To(BeTrue())
		text, err := runBoundedFleetCommand(context.Background(), "missing-test-owned-executor-2502")
		Expect(err).To(HaveOccurred())
		Expect(text).To(BeEmpty())
		installFleetProbeExecutors(`printf '%10000s' ''; exit 0`, false)
		text, err = runBoundedFleetCommand(context.Background(), "podman")
		Expect(err).ToNot(HaveOccurred())
		Expect(text).To(HaveSuffix(" [truncated at 8192 bytes]"))
		Expect(text).To(HaveLen(8192 + len(" [truncated at 8192 bytes]")))
	})

	It("IT-FLEET-2502-004: persisted snapshots and primary errors exclude bearer credential markers", func() {
		installFleetProbeExecutors(`echo 'Authorization: Bearer synthetic-owned-secret-2502' >&2; exit 1`, false)
		var output bytes.Buffer
		err := waitForInClusterAPIServer(context.Background(), "owned-2502-control-plane", "/owned-2502/kubeconfig", 100*time.Millisecond, &output)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).ToNot(ContainSubstring("synthetic-owned-secret-2502"))
		Expect(output.String()).ToNot(ContainSubstring("synthetic-owned-secret-2502"))
		paths, err := filepath.Glob(filepath.Join(os.TempDir(), "kubernaut-must-gather", "fleet-api", "owned-2502-control-plane", "*.log"))
		Expect(err).ToNot(HaveOccurred())
		Expect(paths).To(HaveLen(1))
		body, err := os.ReadFile(paths[0])
		Expect(err).ToNot(HaveOccurred())
		Expect(string(body)).ToNot(ContainSubstring("synthetic-owned-secret-2502"))
		Expect(string(body)).To(ContainSubstring("First-failure authoritative probe:"))
		Expect(string(body)).To(ContainSubstring("[REDACTED]"))
	})
	It("IT-FLEET-2502-001/002: preserves refusal, command and secondary diagnostic failure within a bounded deadline", func() {
		installFleetProbeExecutors(`echo 'bash: connect: Connection refused' >&2; exit 1`, true)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var output bytes.Buffer
		start := time.Now()
		err := waitForInClusterAPIServer(ctx, "owned-2502-control-plane", "/owned-2502/kubeconfig", 100*time.Millisecond, &output)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Connection refused"))
		Expect(output.String()).To(ContainSubstring("exec 3<>/dev/tcp/10.96.0.1/443"))
		Expect(output.String()).To(ContainSubstring("diagnostic"))
		Expect(time.Since(start)).To(BeNumerically("<", 2*time.Second), "diagnostics cannot consume an unbounded readiness budget")
	})

	It("UT-FLEET-2502-002 / IT-FLEET-2502-001: direct backend success never substitutes for failed VIP readiness", func() {
		installFleetProbeExecutors(`case "$*" in *10.89.0.5/6443*) exit 0;; *) echo 'Connection refused' >&2; exit 1;; esac`, false)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var output bytes.Buffer
		// Allow the real executor fixture to complete the direct comparison;
		// the separate 300-ms spec proves cancellation at a short gate deadline.
		err := waitForInClusterAPIServer(ctx, "owned-2502-control-plane", "/owned-2502/kubeconfig", time.Second, &output)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("did not accept connections"))
		Expect(output.String()).To(ContainSubstring("direct backend"))
		Expect(output.String()).To(ContainSubstring("diagnostic only"))
		Expect(output.String()).To(ContainSubstring("exec 3<>/dev/tcp/10.89.0.5/6443"), "a deprecated Endpoints warning must not erase the direct comparison")
		Expect(output.String()).To(MatchRegexp(`direct backend \(diagnostic only\).*error=<nil>`))
	})

	It("IT-FLEET-2502-002: caller cancellation remains the primary error when an external executor hangs", func() {
		installFleetProbeExecutors(`: > "$TMPDIR/probe-started"
exec sleep 30`, true)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var output bytes.Buffer
		results := make(chan error, 1)
		go func() {
			defer GinkgoRecover()
			results <- waitForInClusterAPIServer(ctx, "owned-2502-control-plane", "/owned-2502/kubeconfig", time.Minute, &output)
		}()
		// Cancel only after the actual external probe starts. A fixed short
		// deadline can expire during ClusterIP lookup on a busy race-enabled CI.
		Eventually(func() error {
			_, err := os.Stat(filepath.Join(os.Getenv("TMPDIR"), "probe-started"))
			return err
		}, 5*time.Second, 20*time.Millisecond).Should(Succeed())
		cancel()
		var err error
		Eventually(results, 2*time.Second).Should(Receive(&err))
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		Expect(output.String()).To(ContainSubstring("context canceled"))
	})

	It("IT-FLEET-2502-002: a hung secondary snapshot cannot extend the original readiness deadline", func() {
		installFleetProbeExecutors(`if [[ ! -e "$TMPDIR/first-probe" ]]; then
  : > "$TMPDIR/first-probe"
  echo 'Connection refused' >&2
  exit 1
fi
exec sleep 30`, false)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var output bytes.Buffer
		start := time.Now()
		err := waitForInClusterAPIServer(ctx, "owned-2502-control-plane", "/owned-2502/kubeconfig", 300*time.Millisecond, &output)
		Expect(err).To(HaveOccurred())
		Expect(ctx.Err()).ToNot(HaveOccurred(), "secondary diagnostics must not consume the caller's separate budget")
		Expect(time.Since(start)).To(BeNumerically("<", time.Second), "the unchanged readiness deadline bounds diagnostic teardown too")
		Expect(output.String()).To(MatchRegexp("context (deadline exceeded|canceled)"))
	})

	It("IT-FLEET-2502-001: successful original VIP probe is the only success path", func() {
		installFleetProbeExecutors(`exit 0`, true)
		var output bytes.Buffer
		Expect(waitForInClusterAPIServer(context.Background(), "owned-2502-control-plane", "/owned-2502/kubeconfig", time.Second, &output)).To(Succeed())
		Expect(output.String()).To(ContainSubstring("accepting connections"))
	})
})

func installFleetProbeExecutors(podmanBody string, diagnosticsFail bool) {
	dir := GinkgoT().TempDir()
	GinkgoT().Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	GinkgoT().Setenv("TMPDIR", dir)
	kubectlBody := `case "$*" in
  *jsonpath=\{.spec.clusterIP\}*) printf '10.96.0.1';;
  *get\ endpoints\ *) printf 'Warning: v1 Endpoints is deprecated; use EndpointSlice\n' >&2; printf '10.89.0.5:6443';;
  *get\ endpointslices*) printf '10.89.0.5:6443';;
  *) printf 'test-owned diagnostic snapshot';;
esac`
	if diagnosticsFail {
		kubectlBody = `case "$*" in *jsonpath=\{.spec.clusterIP\}*) printf '10.96.0.1';; *) echo 'test-owned diagnostic failure' >&2; exit 1;; esac`
	}
	Expect(os.WriteFile(filepath.Join(dir, "kubectl"), []byte("#!/bin/bash\n"+kubectlBody+"\n"), 0755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(dir, "podman"), []byte("#!/bin/bash\n"+podmanBody+"\n"), 0755)).To(Succeed())
}
