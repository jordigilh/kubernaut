package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Investigation Streaming (G3)", Ordered, ContinueOnFailure, Label("e2e", "phase3", "g3"), func() {
	var (
		sreToken       string
		kubeconfigPath string
	)

	BeforeAll(func() {
		var err error
		sreToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token")
		Expect(sreToken).NotTo(BeEmpty())

		kubeconfigPath = os.Getenv("HOME") + "/.kube/apifrontend-e2e-config"
	})

	kubectlOut := func(ctx context.Context, args ...string) ([]byte, error) {
		all := append([]string{"--kubeconfig", kubeconfigPath}, args...)
		cmd := exec.CommandContext(ctx, "kubectl", all...)
		return cmd.CombinedOutput()
	}

	sessionNameSnapshot := func(ctx context.Context) map[string]struct{} {
		out, err := kubectlOut(ctx, "get", "investigationsessions", "-n", e2eNamespace,
			"-o", "jsonpath={.items[*].metadata.name}")
		Expect(err).NotTo(HaveOccurred(), string(out))
		names := strings.Fields(strings.TrimSpace(string(out)))
		m := make(map[string]struct{}, len(names))
		for _, n := range names {
			m[n] = struct{}{}
		}
		return m
	}

	type investigationSessionItem struct {
		Metadata struct {
			Name              string `json:"name"`
			CreationTimestamp string `json:"creationTimestamp"`
		} `json:"metadata"`
		Spec struct {
			A2ATaskID    string `json:"a2aTaskID"`
			UserIdentity struct {
				Username string `json:"username"`
			} `json:"userIdentity"`
		} `json:"spec"`
		Status struct {
			Phase           string `json:"phase"`
			ConnectionState string `json:"connectionState"`
		} `json:"status"`
	}
	type investigationSessionList struct {
		Items []investigationSessionItem `json:"items"`
	}

	listInvestigationSessions := func(ctx context.Context) investigationSessionList {
		out, err := kubectlOut(ctx, "get", "investigationsessions", "-n", e2eNamespace, "-o", "json")
		Expect(err).NotTo(HaveOccurred(), string(out))
		var list investigationSessionList
		Expect(json.Unmarshal(out, &list)).To(Succeed())
		return list
	}

	a2aSSEPost := func(ctx context.Context, body string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/a2a/invoke", strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+sreToken)
		return httpClient.Do(req)
	}

	It("TC-E2E-STREAM-01: A2A invoke with Accept: text/event-stream receives SSE frames", func() {
		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		resp, err := a2aSSEPost(readCtx, a2aMessageStream("stream-01", "list pods in kubernaut-system"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		ct := resp.Header.Get("Content-Type")
		Expect(ct).To(ContainSubstring("text/event-stream"))

		sc := bufio.NewScanner(resp.Body)
		// SSE lines can be very long; allow larger token than default.
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		foundData := false
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if strings.HasPrefix(strings.TrimSpace(line), "data:") {
				foundData = true
				break
			}
		}
		Expect(sc.Err()).NotTo(HaveOccurred())
		Expect(foundData).To(BeTrue(), "expected at least one SSE data: line")
	})

	It("TC-E2E-STREAM-02: During investigation, session phase transitions to Connected", func() {
		kctlCtx := context.Background()

		_, checkErr := kubectlOut(kctlCtx, "get", "crd", "investigationsessions.apifrontend.kubernaut.ai")
		if checkErr != nil {
			Skip("InvestigationSession CRD not installed — session lifecycle tests require CRD infrastructure")
		}

		before := sessionNameSnapshot(kctlCtx)

		streamCtx, streamCancel := context.WithCancel(context.Background())
		defer streamCancel()

		resp, err := a2aSSEPost(streamCtx, a2aMessageStream("stream-02", "list pods in kubernaut-system"))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

		go func() {
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)
		}()

		Eventually(func(g Gomega) {
			list := listInvestigationSessions(kctlCtx)
			for _, it := range list.Items {
				if _, seen := before[it.Metadata.Name]; seen {
					continue
				}
				g.Expect(it.Status.Phase).To(Equal("Active"), "session %s phase", it.Metadata.Name)
				g.Expect(it.Status.ConnectionState).To(Equal("Connected"), "session %s connectionState", it.Metadata.Name)
				return
			}
			g.Expect(false).To(BeTrue(), "expected new InvestigationSession with Active phase and Connected connectionState")
		}, 90*time.Second, 2*time.Second).Should(Succeed())
	})

	It("TC-E2E-STREAM-03: Client disconnect -> session phase transitions to Disconnected", func() {
		kctlCtx := context.Background()
		if _, err := kubectlOut(kctlCtx, "get", "crd", "investigationsessions.apifrontend.kubernaut.ai"); err != nil {
			Skip("InvestigationSession CRD not installed — skipping session lifecycle test")
		}
		before := sessionNameSnapshot(kctlCtx)

		streamCtx, streamCancel := context.WithCancel(context.Background())
		resp, err := a2aSSEPost(streamCtx, a2aMessageStream("stream-03", "list pods in kubernaut-system"))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)
		}()

		var sessionName string
		Eventually(func(g Gomega) {
			list := listInvestigationSessions(kctlCtx)
			for _, it := range list.Items {
				if _, seen := before[it.Metadata.Name]; seen {
					continue
				}
				if it.Status.Phase == "Active" && it.Status.ConnectionState == "Connected" {
					sessionName = it.Metadata.Name
					return
				}
			}
			g.Expect(sessionName).NotTo(BeEmpty(), "timed out waiting for Active+Connected session")
		}, 90*time.Second, 2*time.Second).Should(Succeed())

		time.Sleep(2 * time.Second)
		streamCancel()
		Eventually(func() bool {
			select {
			case <-readDone:
				return true
			default:
				return false
			}
		}, 30*time.Second, 100*time.Millisecond).Should(BeTrue(), "SSE body reader should stop after context cancel")

		Eventually(func(g Gomega) {
			out, err := kubectlOut(kctlCtx, "get", "investigationsession", sessionName,
				"-n", e2eNamespace, "-o", "jsonpath={.status.phase}")
			g.Expect(err).NotTo(HaveOccurred(), string(out))
			g.Expect(strings.TrimSpace(string(out))).To(Equal("Disconnected"))
		}, 60*time.Second, 2*time.Second).Should(Succeed())
	})

	It("TC-E2E-STREAM-04 / TC-E2E-SSE-CAP-01: Connection cap enforcement", func() {
		maxStr := getEnvOrDefault("AF_E2E_MAX_SSE", "5")
		maxSSE := 5
		var parsed int
		if n, _ := fmt.Sscanf(strings.TrimSpace(maxStr), "%d", &parsed); n == 1 && parsed > 0 {
			maxSSE = parsed
		}

		var mu sync.Mutex
		cancels := make([]context.CancelFunc, maxSSE)
		ready := make(chan int, maxSSE)
		var wg sync.WaitGroup

		for i := 0; i < maxSSE; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				sctx, scancel := context.WithCancel(context.Background())
				mu.Lock()
				cancels[idx] = scancel
				mu.Unlock()

				body := a2aMessageStream(fmt.Sprintf("stream-cap-%d", idx), "list pods in kubernaut-system")
				req, rerr := http.NewRequestWithContext(sctx, http.MethodPost, baseURL+"/a2a/invoke", strings.NewReader(body))
				if rerr != nil {
					ready <- -1
					scancel()
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Accept", "text/event-stream")
				req.Header.Set("Authorization", "Bearer "+sreToken)

				resp, derr := httpClient.Do(req)
				if derr != nil {
					ready <- -1
					scancel()
					return
				}
				if resp.StatusCode != http.StatusOK {
					_ = resp.Body.Close()
					ready <- -1
					scancel()
					return
				}

				ready <- idx
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				scancel()
			}(i)
		}

		for i := 0; i < maxSSE; i++ {
			Expect(<-ready).To(BeNumerically(">=", 0), "expected concurrent SSE slot %d to connect", i)
		}

		extraReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, baseURL+"/a2a/invoke",
			strings.NewReader(a2aMessageStream("stream-cap-overflow", "ping")))
		Expect(err).NotTo(HaveOccurred())
		extraReq.Header.Set("Content-Type", "application/json")
		extraReq.Header.Set("Accept", "text/event-stream")
		extraReq.Header.Set("Authorization", "Bearer "+sreToken)

		exResp, exErr := httpClient.Do(extraReq)
		Expect(exErr).NotTo(HaveOccurred())
		defer func() { _ = exResp.Body.Close() }()
		Expect(exResp.StatusCode).To(Equal(http.StatusServiceUnavailable))
		extraBody, rerr := io.ReadAll(exResp.Body)
		Expect(rerr).NotTo(HaveOccurred())
		Expect(strings.ToLower(string(extraBody))).To(ContainSubstring("too many concurrent connections"))

		mu.Lock()
		for _, c := range cancels {
			if c != nil {
				c()
			}
		}
		mu.Unlock()

		wg.Wait()
	})
})
