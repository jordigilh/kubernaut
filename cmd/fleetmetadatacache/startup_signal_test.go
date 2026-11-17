package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FMC production startup shutdown [BR-INTEGRATION-065]", func() {
	It("W08 IT-FMC-WATCH-2637-003: SIGTERM cancels dependency startup while LIST succeeds and WATCH is denied", func() {
		// Launch the production command, not a test subprocess: Ginkgo installs
		// its own signal handler, which would obscure startup signal ownership.
		var watches atomic.Int32
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("watch") == "true" {
				watches.Add(1)
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403}`))
				return
			}
			_, _ = w.Write([]byte(`{"apiVersion":"gateway.envoyproxy.io/v1alpha1","kind":"BackendList","metadata":{"resourceVersion":"10"},"items":[]}`))
		}))
		DeferCleanup(api.Close)
		valkey := miniredis.RunT(GinkgoT())
		dir := GinkgoT().TempDir()
		for _, name := range []string{"client-id", "client-secret"} {
			Expect(os.WriteFile(filepath.Join(dir, name), []byte("test-only"), 0600)).To(Succeed())
		}
		kubeconfig := filepath.Join(dir, "kubeconfig")
		Expect(os.WriteFile(kubeconfig, []byte(fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: fixture
  cluster:
    server: %s
contexts:
- name: fixture
  context:
    cluster: fixture
    user: fixture
current-context: fixture
users:
- name: fixture
  user: {}
`, api.URL)), 0600)).To(Succeed())
		config := filepath.Join(dir, "config.yaml")
		Expect(os.WriteFile(config, []byte(fmt.Sprintf(`server:
  apiAddr: 127.0.0.1:0
  healthAddr: 127.0.0.1:0
  metricsAddr: 127.0.0.1:0
mcpGateway:
  endpoint: http://127.0.0.1:1/mcp
  gatewayType: eaigw
  namespace: registrations
  resilience:
    initialInterval: 1ms
    maxInterval: 1ms
    maxElapsedTime: 10ms
    connectTimeout: 10ms
valkey:
  addr: %s
oauth2:
  tokenUrl: http://127.0.0.1:1/token
  credentialsDir: %s
`, valkey.Addr(), dir)), 0600)).To(Succeed())
		binary := filepath.Join(dir, "fleetmetadatacache")
		buildOutput, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(buildOutput))
		command := exec.Command(binary, "-config", config)
		command.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig, "KUBERNAUT_CONTROLLER_NAMESPACE=kubernaut-system")
		var output bytes.Buffer
		command.Stdout, command.Stderr = &output, &output
		Expect(command.Start()).To(Succeed())
		done := make(chan error, 1)
		var exited atomic.Bool
		go func() {
			err := command.Wait()
			exited.Store(true)
			done <- err
		}()
		DeferCleanup(func() {
			if !exited.Load() {
				_ = command.Process.Kill()
				<-done
			}
		})
		Eventually(watches.Load).WithTimeout(5 * time.Second).Should(BeNumerically(">=", 1))
		Expect(command.Process.Signal(syscall.SIGTERM)).To(Succeed())
		var processErr error
		Eventually(done).WithTimeout(2 * time.Second).Should(Receive(&processErr))
		Expect(processErr).NotTo(HaveOccurred(), output.String())
		attempts := watches.Load()
		Consistently(watches.Load).WithTimeout(1200 * time.Millisecond).Should(Equal(attempts))
	})
})
