package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// =============================================================================
// E2E-AF-1403: Execution Progress Artifacts in SSE Stream
// Proves that kubernaut_watch emits TaskArtifactUpdateEvent with
// execution_progress type during phase transitions.
// =============================================================================

var _ = Describe("Execution Progress Streaming (#1403)", Ordered, Label("e2e", "phase3", "g3", "1403"), func() {
	var sreToken string

	BeforeEach(func() {
		var err error
		sreToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred())
		Expect(sreToken).NotTo(BeEmpty())
	})

	a2aSSEPostReq := func(ctx context.Context, body string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/a2a/invoke", strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+sreToken)
		return httpClient.Do(req)
	}

	It("E2E-AF-1403-001: SSE stream emits execution_progress artifact on phase transitions", NodeTimeout(90*time.Second), func(_ SpecContext) {
		const rrName = "rr-progress-e2e-1403"

		By("Creating an RR that will transition phases")
		Expect(createRR(e2eNamespace, rrName, "Deployment", "progress-deploy")).To(Succeed())
		DeferCleanup(func() { deleteRR(e2eNamespace, rrName) })

		By("Simulating phase transitions after 3s delay")
		go func() {
			defer GinkgoRecover()
			time.Sleep(3 * time.Second)
			rr := &remediationv1alpha1.RemediationRequest{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKey{
				Namespace: e2eNamespace, Name: rrName,
			}, rr)).To(Succeed())
			rr.Status.OverallPhase = "Executing"
			rr.Status.Message = "E2E: simulated Executing phase"
			Expect(k8sClient.Status().Update(context.Background(), rr)).To(Succeed())

			time.Sleep(2 * time.Second)
			Expect(k8sClient.Get(context.Background(), client.ObjectKey{
				Namespace: e2eNamespace, Name: rrName,
			}, rr)).To(Succeed())
			rr.Status.OverallPhase = "Completed"
			rr.Status.Outcome = "Success"
			rr.Status.Message = "E2E: simulated Completed phase"
			Expect(k8sClient.Status().Update(context.Background(), rr)).To(Succeed())
		}()

		By("Invoking A2A message/stream triggering kubernaut_watch via mock-LLM")
		streamCtx, streamCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer streamCancel()

		resp, err := a2aSSEPostReq(streamCtx, a2aMessageStream("progress-e2e-1403", "watch progress e2e 1403"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var wg sync.WaitGroup
		var mu sync.Mutex
		var events []json.RawMessage

		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(resp.Body)
			sc.Buffer(make([]byte, 64*1024), 1024*1024)
			for sc.Scan() {
				line := strings.TrimRight(sc.Text(), "\r")
				if strings.HasPrefix(strings.TrimSpace(line), "data:") {
					data := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
					if data != "" {
						mu.Lock()
						events = append(events, json.RawMessage(data))
						mu.Unlock()
					}
				}
			}
		}()
		wg.Wait()

		By("Asserting at least one execution_progress artifact in SSE events")
		var progressArtifacts []map[string]any
		for _, raw := range events {
			var evt map[string]any
			if err := json.Unmarshal(raw, &evt); err != nil {
				continue
			}
			artifact, hasArtifact := evt["artifact"].(map[string]any)
			if !hasArtifact {
				continue
			}
			meta, _ := artifact["metadata"].(map[string]any)
			if meta == nil || meta["type"] != "execution_progress" {
				continue
			}
			parts, _ := artifact["parts"].([]any)
			if len(parts) == 0 {
				continue
			}
			for _, part := range parts {
				pm, _ := part.(map[string]any)
				if pm == nil {
					continue
				}
				if data, ok := pm["data"].(map[string]any); ok {
					if data["type"] == "execution_progress" {
						progressArtifacts = append(progressArtifacts, data)
					}
				}
			}
		}
		Expect(progressArtifacts).NotTo(BeEmpty(),
			"expected at least one execution_progress artifact in the SSE stream")

		lastProgress := progressArtifacts[len(progressArtifacts)-1]
		Expect(lastProgress["rr_name"]).To(Equal(rrName))
		Expect(lastProgress["started_at"]).NotTo(BeEmpty())
		Expect(lastProgress).To(HaveKey("current_phase"))
		fmt.Fprintf(GinkgoWriter, "Progress artifacts received: %d, last phase: %s\n",
			len(progressArtifacts), lastProgress["current_phase"])
	})
})
