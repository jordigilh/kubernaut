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
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// IT-FMC-SRC-2661-005 (TEST_PLAN §8 Tier 2): a sources.acm block that fails
// production construction (here: a caFile that passes Validate's
// absoluteness check but does not exist when NewACMHTTPSearch loads it) must
// surface through the dependency-wiring choreography — main's
// "Failed to construct FMC dependencies" log and its bootstrap health-server
// shutdown — not through an os.Exit buried inside wireFMCDependencies that
// skips both (DD-FLEET-017 D4.1a rule 3: fail closed at startup, observably).
//
// The production command runs as a subprocess for the same reason as
// IT-FMC-WATCH-2637-003: the distinction under test is process-level
// (which code path performs the exit), which an in-process test cannot see.
var _ = Describe("FMC ACM source startup failure choreography [BR-SAFETY-001, DD-FLEET-017 D4.1a rule 3]", func() {
	It("IT-FMC-SRC-2661-005: an unbuildable ACM source fails through the dependency-wiring error path, not a bare os.Exit", func() {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("watch") == "true" {
				// A valid, established watch stream: client-go's decoder needs
				// the JSON content type, and the flushed chunked body keeps
				// the watch alive so the registry completes its initial sync
				// and wiring proceeds to the syncer construction under test.
				// The flush is load-bearing: without it Go's server holds the
				// headers until the handler returns, and the watch client
				// waits forever for a response that was never sent.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if flusher, canFlush := w.(http.Flusher); canFlush {
					flusher.Flush()
				}
				<-r.Context().Done()
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"apiVersion":"gateway.envoyproxy.io/v1alpha1","kind":"BackendList","metadata":{"resourceVersion":"10"},"items":[]}`))
		}))
		DeferCleanup(api.Close)
		valkey := miniredis.RunT(GinkgoT())
		dir := GinkgoT().TempDir()
		for _, name := range []string{"client-id", "client-secret", "token"} {
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
		// The caFile is absolute but absent: config validation passes (it
		// checks the path shape, not existence), and NewACMHTTPSearch's CA
		// load fails — the construction failure this spec pins.
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
sync:
  resourceKinds: [Pod]
sources:
  acm:
    endpoint: https://search.example:4010
    tokenPath: %s
    caFile: %s
`, valkey.Addr(), dir, filepath.Join(dir, "token"), filepath.Join(dir, "absent-ca.crt"))), 0600)).To(Succeed())
		binary := filepath.Join(dir, "fleetmetadatacache")
		buildOutput, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(buildOutput))
		command := exec.Command(binary, "-config", config)
		command.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig, "KUBERNAUT_CONTROLLER_NAMESPACE=kubernaut-system")
		var output bytes.Buffer
		command.Stdout, command.Stderr = &output, &output
		Expect(command.Start()).To(Succeed())
		var exited atomic.Bool
		done := make(chan error, 1)
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

		var processErr error
		// Poll a status function rather than the bare channel so a timeout
		// reports where the subprocess actually is, not just "no signal".
		processState := func() string {
			select {
			case err := <-done:
				processErr = err
				return fmt.Sprintf("exited: %v", err)
			default:
				return "running; output tail:\n" + output.String()
			}
		}
		Eventually(processState).WithTimeout(30*time.Second).WithPolling(500*time.Millisecond).Should(
			MatchRegexp(`^exited:`), "the process must reach a startup failure and exit")
		Expect(processErr).To(HaveOccurred(), "a failed dependency wiring must exit nonzero")
		Expect(output.String()).To(ContainSubstring("Failed to construct FMC dependencies"),
			"the failure must surface through main's dependency-wiring choreography, which also shuts down the bootstrap health server")
		Expect(output.String()).NotTo(ContainSubstring("Failed to build the FMC sync source"),
			"wireFMCDependencies must return the error, not exit the process itself")
	})
})
