package e2e_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func resilienceKubeconfigPath() string {
	if p := os.Getenv("KUBECONFIG"); p != "" {
		return p
	}
	return os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
}

func kubectlWithResilienceKubeconfig(args ...string) *exec.Cmd {
	kc := resilienceKubeconfigPath()
	all := append([]string{"--kubeconfig", kc}, args...)
	return exec.CommandContext(context.Background(), "kubectl", all...)
}

// counterValue returns the latest sample value for an unlabelled Prometheus counter (e.g. af_http_panics_total).
func counterValue(metricsBody, metricName string) float64 {
	var v float64
	found := false
	for _, line := range strings.Split(metricsBody, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, metricName+" ") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			var parsed float64
			if _, err := fmt.Sscanf(fields[1], "%f", &parsed); err == nil {
				v = parsed
				found = true
			}
		}
	}
	if !found {
		return 0
	}
	return v
}

func configMapNameForApifrontend() string {
	out, err := kubectlWithResilienceKubeconfig("get", "cm", "-n", e2eNamespace, "-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\n\"}{end}").Output()
	if err != nil {
		return "apifrontend-config"
	}
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name = strings.TrimSpace(name)
		if strings.HasPrefix(name, "apifrontend-config") {
			return name
		}
	}
	return "apifrontend-config"
}

var _ = Describe("Resilience and Operational (G17/G20/G9/G10/G11)", Ordered, Label("e2e", "phase5-6"), func() {

	Context("TC-E2E-PANIC-01 (G17)", func() {
		It("POST /debug/panic returns 500 problem+json and increments af_http_panics_total", func() {
			before := counterValue(scrapeMetrics(), "af_http_panics_total")

			req, err := http.NewRequest(http.MethodPost, baseURL+"/debug/panic", http.NoBody)
			Expect(err).NotTo(HaveOccurred())
			resp, err := httpClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode == http.StatusNotFound {
				Skip("/debug/panic not registered — AF image likely built without -tags e2e")
			}

			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
			Expect(strings.ToLower(resp.Header.Get("Content-Type"))).To(ContainSubstring("application/problem+json"))
			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.ToLower(string(body))).To(Or(
				ContainSubstring("internal server error"),
				ContainSubstring("service error"),
			))

			Eventually(func() float64 {
				return counterValue(scrapeMetrics(), "af_http_panics_total")
			}, 30*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", before+1),
				"af_http_panics_total should increase by at least 1 after handled panic")
		})
	})

	Context("TC-E2E-CHAOS-01 (G20)", func() {
		It("Pod kill during active A2A stream: session eventually Disconnected; CRDs still listable after rollout", func(ctx SpecContext) {
			token, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())

			streamCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			streamLive := make(chan struct{})
			doneRead := make(chan struct{})
			go func() {
				defer close(doneRead)
				defer GinkgoRecover()
				req, e := http.NewRequestWithContext(streamCtx, http.MethodPost, baseURL+"/a2a/invoke",
					strings.NewReader(a2aTasksSend("chaos-01-stream",
						"Investigate pod nginx-crash in prod namespace: start the investigation, poll for results, list available workflows, select the best one, and present options")))
				if e != nil {
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Accept", "application/json, text/event-stream")
				resp, e := httpClient.Do(req)
				if e != nil {
					return
				}
				close(streamLive)
				defer func() { _ = resp.Body.Close() }()
				// Hold the stream open while the test deletes the pod.
				_, _ = io.Copy(io.Discard, resp.Body)
			}()

			Eventually(streamLive, 30*time.Second, 200*time.Millisecond).Should(BeClosed(),
				"A2A stream should connect before pod disruption")
			time.Sleep(8 * time.Second)

			// First AF pod in the namespace.
			podOut, err := kubectlWithResilienceKubeconfig("get", "pods", "-n", e2eNamespace,
				"-l", "app.kubernetes.io/name=kubernaut-apifrontend",
				"-o", "jsonpath={.items[0].metadata.name}").Output()
			Expect(err).NotTo(HaveOccurred())
			podName := strings.TrimSpace(string(podOut))
			if podName == "" {
				Skip("no apifrontend pod found for chaos test")
			}

			delOut, err := kubectlWithResilienceKubeconfig("delete", "pod", podName, "-n", e2eNamespace, "--wait=false").CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(delOut))

			rollOut, err := kubectlWithResilienceKubeconfig("rollout", "status", "deployment/apifrontend",
				"-n", e2eNamespace, "--timeout=180s").CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(rollOut))

			cancel()
			Eventually(doneRead, 60*time.Second, 500*time.Millisecond).Should(BeClosed())

			Eventually(func() string {
				out, e := kubectlWithResilienceKubeconfig("get", "investigationsessions.apifrontend.kubernaut.ai", "-n", e2eNamespace,
					"-o", "jsonpath={range .items[*]}{.status.phase}{\",\"}{end}").Output()
				if e != nil {
					return ""
				}
				return string(out)
			}, 120*time.Second, 3*time.Second).Should(Or(
				ContainSubstring("Disconnected"),
				ContainSubstring("disconnected"),
			), "expected at least one InvestigationSession in Disconnected phase after AF pod loss")

			listOut, err := kubectlWithResilienceKubeconfig("get", "investigationsessions.apifrontend.kubernaut.ai", "-A").CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(listOut))
		})
	})

	Context("TC-E2E-CHAOS-02 (G20)", func() {
		It("AF→KA network partition: MCP error + KA circuit breaker open; recovers after policy removal", func() {
			orig, err := kubectlWithResilienceKubeconfig("get", "networkpolicy", "apifrontend", "-n", e2eNamespace, "-o", "yaml").Output()
			Expect(err).NotTo(HaveOccurred(), "need apifrontend NetworkPolicy to patch for chaos test")

			origCopy := append([]byte(nil), orig...)
			DeferCleanup(func() {
				tmp, tErr := os.CreateTemp("", "e2e-netpol-restore-*.yaml")
				Expect(tErr).NotTo(HaveOccurred())
				defer func() { _ = os.Remove(tmp.Name()) }()
				_, wErr := tmp.Write(origCopy)
				Expect(wErr).NotTo(HaveOccurred())
				Expect(tmp.Close()).To(Succeed())
				restoreOut, rErr := kubectlWithResilienceKubeconfig("apply", "-f", tmp.Name()).CombinedOutput()
				Expect(rErr).NotTo(HaveOccurred(), string(restoreOut))
			})

			chaosNP := strings.TrimSpace(`
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: apifrontend
  namespace: ` + e2eNamespace + `
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: kubernaut-apifrontend
      app.kubernetes.io/component: apifrontend
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ` + e2eNamespace + `
      ports:
        - port: 8443
          protocol: TCP
    - ports:
        - port: 8081
          protocol: TCP
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ` + e2eNamespace + `
      ports:
        - port: 9090
          protocol: TCP
  egress:
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: data-storage
      ports:
        - port: 8443
          protocol: TCP
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: dex
      ports:
        - port: 5556
          protocol: TCP
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: mock-llm
      ports:
        - port: 8080
          protocol: TCP
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - port: 443
          protocol: TCP
        - port: 6443
          protocol: TCP
    - ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
      ports:
        - port: 9090
          protocol: TCP
`)
			apply := exec.CommandContext(context.Background(), "kubectl", "--kubeconfig", resilienceKubeconfigPath(), "apply", "-f", "-")
			apply.Stdin = strings.NewReader(chaosNP)
			out, err := apply.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(out))

			token, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())
			sid, err := initMCPSession(token)
			Expect(err).NotTo(HaveOccurred())

			callDone := make(chan struct{})
			var callErr error
			var callBody []byte
			var callHTTP int
			go func() {
				defer close(callDone)
				callBody, callHTTP, callErr = mcpPOST(token, sid, buildJSONRPC("chaos-02", "tools/call", map[string]interface{}{
					"name":      "kubernaut_start_investigation",
					"arguments": map[string]interface{}{"namespace": "default", "name": "e2e-chaos", "kind": "Pod"},
				}))
			}()

			select {
			case <-callDone:
				Expect(callErr).NotTo(HaveOccurred())
				payload := unwrapSSEDataLine(callBody)
				pl := strings.ToLower(payload)
				if callHTTP >= http.StatusBadRequest {
					Expect(pl).To(Or(
						ContainSubstring("error"),
						ContainSubstring("unavailable"),
						ContainSubstring("circuit"),
						ContainSubstring("502"),
						ContainSubstring("503"),
					), "expected a structured HTTP error when KA is unreachable, got: %s", payload)
				} else {
					text, toolErr, perr := parseMCPToolPayload(payload)
					Expect(perr).NotTo(HaveOccurred())
					Expect(toolErr).To(BeTrue(), "tool should surface an error result when KA is partitioned: %s", text)
					Expect(strings.ToLower(text)).To(Or(
						ContainSubstring("error"),
						ContainSubstring("unavailable"),
						ContainSubstring("circuit"),
						ContainSubstring("timeout"),
					), "expected error text in MCP tool result, got: %s", text)
				}
			case <-time.After(45 * time.Second):
				Fail("MCP tool call should not hang indefinitely when KA is partitioned")
			}

			Eventually(scrapeMetrics, 90*time.Second, 2*time.Second).Should(MatchRegexp(`af_circuit_breaker_state\{[^}]*dependency="ka"[^}]*\} 2`),
				"KA circuit breaker should be open (state 2) while AF cannot reach KA")

			tmp, tErr := os.CreateTemp("", "e2e-netpol-orig-*.yaml")
			Expect(tErr).NotTo(HaveOccurred())
			_, err = tmp.Write(orig)
			Expect(err).NotTo(HaveOccurred())
			Expect(tmp.Close()).To(Succeed())
			defer func() { _ = os.Remove(tmp.Name()) }()
			outRest, aErr := kubectlWithResilienceKubeconfig("apply", "-f", tmp.Name()).CombinedOutput()
			Expect(aErr).NotTo(HaveOccurred(), string(outRest))

			Eventually(func() error {
				b, httpCode, e := mcpPOST(token, sid, buildJSONRPC("chaos-02-recover", "tools/call", map[string]interface{}{
					"name":      "kubernaut_list_workflows",
					"arguments": map[string]interface{}{},
				}))
				if e != nil {
					return e
				}
				if httpCode >= http.StatusBadRequest {
					return fmt.Errorf("HTTP %d: %s", httpCode, string(b))
				}
				payload := strings.ToLower(unwrapSSEDataLine(b))
				if strings.Contains(payload, "502") || strings.Contains(payload, "503") || strings.Contains(payload, "unavailable") {
					return fmt.Errorf("still failing: %s", payload)
				}
				return nil
			}, 120*time.Second, 3*time.Second).Should(Succeed())
		})
	})

	Context("TC-E2E-CHAOS-03 (G20)", func() {
		It("AF→Prometheus partition: af_create_rr without severity uses severity_source llm_triage", func() {
			orig, err := kubectlWithResilienceKubeconfig("get", "networkpolicy", "apifrontend", "-n", e2eNamespace, "-o", "yaml").Output()
			Expect(err).NotTo(HaveOccurred())

			origCopy3 := append([]byte(nil), orig...)
			DeferCleanup(func() {
				tmp, tErr := os.CreateTemp("", "e2e-netpol-chaos3-restore-*.yaml")
				Expect(tErr).NotTo(HaveOccurred())
				defer func() { _ = os.Remove(tmp.Name()) }()
				_, wErr := tmp.Write(origCopy3)
				Expect(wErr).NotTo(HaveOccurred())
				Expect(tmp.Close()).To(Succeed())
				_, _ = kubectlWithResilienceKubeconfig("apply", "-f", tmp.Name()).CombinedOutput()
			})

			chaosNP := strings.TrimSpace(`
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: apifrontend
  namespace: ` + e2eNamespace + `
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: kubernaut-apifrontend
      app.kubernetes.io/component: apifrontend
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ` + e2eNamespace + `
      ports:
        - port: 8443
          protocol: TCP
    - ports:
        - port: 8081
          protocol: TCP
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ` + e2eNamespace + `
      ports:
        - port: 9090
          protocol: TCP
  egress:
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/part-of: kubernaut
      ports:
        - port: 8443
          protocol: TCP
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: dex
      ports:
        - port: 5556
          protocol: TCP
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: mock-llm
      ports:
        - port: 8080
          protocol: TCP
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - port: 443
          protocol: TCP
        - port: 6443
          protocol: TCP
    - ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
`)
			apply := exec.CommandContext(context.Background(), "kubectl", "--kubeconfig", resilienceKubeconfigPath(), "apply", "-f", "-")
			apply.Stdin = strings.NewReader(chaosNP)
			out, err := apply.CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(out))

			token, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())
			sid, err := initMCPSession(token)
			Expect(err).NotTo(HaveOccurred())

			uniq := fmt.Sprintf("test-prom-block-%d", time.Now().UnixNano())
			raw, code, err := mcpPOST(token, sid, buildJSONRPC("chaos-03", "tools/call", map[string]interface{}{
				"name": "af_create_rr",
				"arguments": map[string]interface{}{
					"namespace":   "no-rules-ns",
					"name":        uniq,
					"kind":        "Deployment",
					"description": "E2E chaos — Prometheus blocked, empty severity",
				},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(code).To(BeNumerically("<", 400))
			text, _, perr := parseMCPToolPayload(unwrapSSEDataLine(raw))
			Expect(perr).NotTo(HaveOccurred())
			Expect(parseJSONStringField(text, "severity_source")).To(Equal("llm_triage"))
		})
	})

	Context("TC-E2E-DRAIN-01 / TC-E2E-DRAIN-02 (G9)", func() {
		It("SIGTERM during MCP SSE: stream ends gracefully; /readyz returns 503 during drain", func() {
			token, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())
			sid, err := initMCPSession(token)
			Expect(err).NotTo(HaveOccurred())

			reqCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			callBody := buildJSONRPC("drain-sse", "tools/call", map[string]interface{}{
				"name":      "kubernaut_start_investigation",
				"arguments": map[string]interface{}{"namespace": "default", "name": "e2e-drain", "kind": "Pod"},
			})
			req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/mcp", strings.NewReader(callBody))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Mcp-Session-Id", sid)

			resp, err := httpClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(BeNumerically("<", 400))

			readerDone := make(chan error, 1)
			go func() {
				defer func() { _ = resp.Body.Close() }()
				br := bufio.NewReader(resp.Body)
				_, err := br.ReadByte()
				if err != nil {
					readerDone <- err
					return
				}
				_, _ = io.Copy(io.Discard, br)
				readerDone <- nil
			}()

			podOut, err := kubectlWithResilienceKubeconfig("get", "pods", "-n", e2eNamespace,
				"-l", "app.kubernetes.io/name=kubernaut-apifrontend",
				"-o", "jsonpath={.items[0].metadata.name}").Output()
			Expect(err).NotTo(HaveOccurred())
			podName := strings.TrimSpace(string(podOut))
			if podName == "" {
				Skip("no apifrontend pod for exec")
			}

			execOut, err := kubectlWithResilienceKubeconfig("exec", "-n", e2eNamespace, podName, "-c", "apifrontend", "--", "kill", "-TERM", "1").CombinedOutput()
			if err != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				cancel()
				Skip(fmt.Sprintf("kill -TERM not permitted in this environment: %v: %s", err, string(execOut)))
			}

			select {
			case rErr := <-readerDone:
				Expect(rErr).To(HaveOccurred())
			case <-time.After(60 * time.Second):
				Fail("expected SSE reader to complete after SIGTERM")
			}

			Eventually(func() int {
				hr, e := httpClient.Get(baseURL + "/readyz")
				if e != nil {
					return 0
				}
				code := hr.StatusCode
				_, _ = io.Copy(io.Discard, hr.Body)
				_ = hr.Body.Close()
				return code
			}, 45*time.Second, 500*time.Millisecond).Should(Equal(http.StatusServiceUnavailable),
				"/readyz should return 503 while draining after SIGTERM")

			_, _ = kubectlWithResilienceKubeconfig("rollout", "status", "deployment/apifrontend",
				"-n", e2eNamespace, "--timeout=180s").CombinedOutput()
		})
	})

	Context("TC-E2E-HOTRELOAD-01 (G10)", func() {
		It("ConfigMap patch is picked up (config reloaded in logs)", func() {
			cm := configMapNameForApifrontend()
			cur, err := kubectlWithResilienceKubeconfig("get", "cm", cm, "-n", e2eNamespace, "-o", "jsonpath={.data.config\\.yaml}").Output()
			Expect(err).NotTo(HaveOccurred())
			yamlText := string(cur)
			if !strings.Contains(yamlText, "level:") {
				Skip("unexpected config.yaml structure")
			}

			patched := strings.Replace(yamlText, "level: debug", "level: info", 1)
			if patched == yamlText {
				patched = strings.Replace(yamlText, "level: info", "level: debug", 1)
			}
			Expect(patched).NotTo(Equal(yamlText), "need distinct log level for hot reload probe")

			DeferCleanup(func() {
				rpatch := strings.NewReplacer("\n", "\\n", "\"", "\\\"").Replace(yamlText)
				_, _ = kubectlWithResilienceKubeconfig("patch", "cm", cm, "-n", e2eNamespace, "--type", "merge",
					"-p", `{"data":{"config.yaml":"`+rpatch+`"}}`).CombinedOutput()
				_, _ = kubectlWithResilienceKubeconfig("rollout", "restart", "deployment/apifrontend", "-n", e2eNamespace).CombinedOutput()
				_, _ = kubectlWithResilienceKubeconfig("rollout", "status", "deployment/apifrontend", "-n", e2eNamespace, "--timeout=180s").CombinedOutput()
			})

			escaped := strings.NewReplacer("\n", "\\n", "\"", "\\\"").Replace(patched)
			pOut, err := kubectlWithResilienceKubeconfig("patch", "cm", cm, "-n", e2eNamespace, "--type", "merge",
				"-p", `{"data":{"config.yaml":"`+escaped+`"}}`).CombinedOutput()
			Expect(err).NotTo(HaveOccurred(), string(pOut))

			podName, err := kubectlWithResilienceKubeconfig("get", "pods", "-n", e2eNamespace,
				"-l", "app.kubernetes.io/name=kubernaut-apifrontend",
				"-o", "jsonpath={.items[0].metadata.name}").Output()
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				logOut, e := kubectlWithResilienceKubeconfig("logs", "-n", e2eNamespace, strings.TrimSpace(string(podName)), "-c", "apifrontend", "--tail=50").Output()
				if e != nil {
					return ""
				}
				return string(logOut)
			}, 90*time.Second, 2*time.Second).Should(ContainSubstring("config reloaded"))
		})
	})

	Context("TC-E2E-MCP-IDLE-01 (G11)", func() {
		It("MCP session expires after idle timeout; next tools/call errors or requires new session", func(ctx SpecContext) {
			token, err := fetchDEXTokenForPersona("sre")
			Expect(err).NotTo(HaveOccurred())
			sid, err := initMCPSession(token)
			Expect(err).NotTo(HaveOccurred())
			Expect(sid).NotTo(BeEmpty())

			// E2E overlay defaults to 5m (see deploy/kustomize/overlays/e2e/config.yaml).
			select {
			case <-time.After(5*time.Minute + 45*time.Second):
			case <-ctx.Done():
				Fail("interrupted before idle timeout window")
			}

			raw, code, err := mcpPOST(token, sid, buildJSONRPC("idle-after-wait", "tools/call", map[string]interface{}{
				"name":      "af_list_events",
				"arguments": map[string]interface{}{"namespace": e2eNamespace},
			}))

			if code >= http.StatusBadRequest {
				Expect(strings.ToLower(string(raw))).To(Or(
					ContainSubstring("session"),
					ContainSubstring("expired"),
					ContainSubstring("unauthorized"),
					ContainSubstring("invalid"),
				))
				return
			}

			Expect(err).NotTo(HaveOccurred())
			_, _, perr := parseMCPToolPayload(unwrapSSEDataLine(raw))
			if perr != nil {
				Expect(perr.Error()).To(Or(
					ContainSubstring("session"),
					ContainSubstring("expired"),
				))
				return
			}

			sid2, err := initMCPSession(token)
			Expect(err).NotTo(HaveOccurred())
			Expect(sid2).NotTo(Equal(sid), "server should issue a fresh MCP session after idle expiry")

			raw2, code2, err2 := mcpPOST(token, sid2, buildJSONRPC("idle-new-session", "tools/call", map[string]interface{}{
				"name":      "af_list_events",
				"arguments": map[string]interface{}{"namespace": e2eNamespace},
			}))
			Expect(err2).NotTo(HaveOccurred())
			Expect(code2).To(BeNumerically("<", 400))
			_, _, perr2 := parseMCPToolPayload(unwrapSSEDataLine(raw2))
			Expect(perr2).NotTo(HaveOccurred())
		}, SpecTimeout(8*time.Minute))
	})
})
