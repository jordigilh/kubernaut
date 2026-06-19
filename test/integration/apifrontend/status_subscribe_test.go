package apifrontend_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
)

func newWatchClient() crclient.WithWatch {
	wc, err := crclient.NewWithWatch(restCfg, crclient.Options{Scheme: scheme})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return wc
}

func statusSubscribeBody(rrID string) []byte {
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      "sub-1",
		"method":  "status/subscribe",
		"params":  map[string]any{"rr_id": rrID},
	}
	raw, _ := json.Marshal(body)
	return raw
}

func createTestRR(ctx context.Context, name, ns, phase string) *remediationv1.RemediationRequest {
	h := sha256.Sum256([]byte("status-it-" + name))
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: remediationv1.RemediationRequestSpec{
			SignalFingerprint: hex.EncodeToString(h[:]),
			SignalName:        "StatusITAlert",
			Severity:          "critical",
			SignalType:        "alert",
			SignalSource:      "test-status-it",
			TargetType:        "kubernetes",
			FiringTime:        metav1.Now(),
			ReceivedTime:      metav1.Now(),
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      "Deployment",
				Name:      "api-server",
				Namespace: ns,
			},
		},
	}
	ExpectWithOffset(1, k8sClient.Create(ctx, rr)).To(Succeed())
	rr.Status.OverallPhase = remediationv1.RemediationPhase(phase)
	ExpectWithOffset(1, k8sClient.Status().Update(ctx, rr)).To(Succeed())
	return rr
}

func cleanupTestRR(ctx context.Context, name, ns string) {
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	_ = k8sClient.Delete(ctx, rr)
}

type sseEvent struct {
	Event string
	Data  string
}

func collectSSEEvents(resp *http.Response, count int, timeout time.Duration) []sseEvent {
	var events []sseEvent
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		defer close(done)
		scanner := bufio.NewScanner(resp.Body)
		var currentEvent, currentData string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				currentEvent = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				currentData = strings.TrimPrefix(line, "data: ")
			case line == "":
				if currentEvent != "" || currentData != "" {
					mu.Lock()
					events = append(events, sseEvent{Event: currentEvent, Data: currentData})
					if len(events) >= count {
						mu.Unlock()
						return
					}
					mu.Unlock()
					currentEvent = ""
					currentData = ""
				}
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}

	mu.Lock()
	defer mu.Unlock()
	result := make([]sseEvent, len(events))
	copy(result, events)
	return result
}

var _ = Describe("status/subscribe SSE endpoint (IT)", func() {
	var (
		ctx       context.Context
		cancel    context.CancelFunc
		wc        crclient.WithWatch
		statusSrv *httptest.Server
		testNS    string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		wc = newWatchClient()
		testNS = fmt.Sprintf("status-it-%d", time.Now().UnixNano()%100000)

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNS}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		sh := handler.NewStatusHandler(wc, testNS, logf.Log.WithName("status-it"))
		statusSrv = httptest.NewServer(sh)
	})

	AfterEach(func() {
		if statusSrv != nil {
			statusSrv.Close()
		}
		cancel()
	})

	It("IT-AF-1460-010: REQ-2 current-state delivery on subscribe", func() {
		rr := createTestRR(ctx, "rr-current-state", testNS, "Processing")
		defer cleanupTestRR(ctx, rr.Name, testNS)

		resp, err := http.Post(statusSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody(rr.Name)))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

		events := collectSSEEvents(resp, 1, 5*time.Second)
		Expect(events).To(HaveLen(1), "must receive at least 1 event (current state)")

		var params map[string]any
		Expect(json.Unmarshal([]byte(events[0].Data), &params)).To(Succeed())
		method, _ := params["method"].(string)
		Expect(method).To(Equal("status/update"))
		inner, _ := params["params"].(map[string]any)
		Expect(inner["phase"]).To(Equal("Processing"))
	})

	It("IT-AF-1460-011: phase transition emits SSE event with metadata", func() {
		rr := createTestRR(ctx, "rr-phase-trans", testNS, "Processing")
		defer cleanupTestRR(ctx, rr.Name, testNS)

		resp, err := http.Post(statusSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody(rr.Name)))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		time.Sleep(200 * time.Millisecond)
		Expect(k8sClient.Get(ctx, crclient.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseExecuting
		now := metav1.Now()
		rr.Status.ExecutingStartTime = &now
		rr.Status.SelectedWorkflowRef = &remediationv1.WorkflowReference{WorkflowID: "git-revert-v2"}
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		events := collectSSEEvents(resp, 2, 5*time.Second)
		Expect(len(events)).To(BeNumerically(">=", 2))

		var found bool
		for _, evt := range events {
			var data map[string]any
			if json.Unmarshal([]byte(evt.Data), &data) != nil {
				continue
			}
			inner, _ := data["params"].(map[string]any)
			if inner == nil {
				continue
			}
			if inner["phase"] == "Executing" {
				meta, _ := inner["metadata"].(map[string]any)
				Expect(meta).To(HaveKey("workflow_id"))
				found = true
			}
		}
		Expect(found).To(BeTrue(), "must receive Executing event with workflow_id metadata")
	})

	It("IT-AF-1460-012: EA sub-phase during Verifying via effectivenessAssessmentRef", func() {
		rr := createTestRR(ctx, "rr-ea-subphase", testNS, "Executing")
		defer cleanupTestRR(ctx, rr.Name, testNS)

		eaName := "ea-custom-ref-12"
		rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
			Kind:      "EffectivenessAssessment",
			Name:      eaName,
			Namespace: testNS,
		}
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		resp, err := http.Post(statusSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody(rr.Name)))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		time.Sleep(200 * time.Millisecond)

		Expect(k8sClient.Get(ctx, crclient.ObjectKeyFromObject(rr), rr)).To(Succeed())
		deadline := metav1.NewTime(time.Now().Add(10 * time.Minute))
		rr.Status.OverallPhase = remediationv1.PhaseVerifying
		rr.Status.VerificationDeadline = &deadline
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		time.Sleep(200 * time.Millisecond)

		ea := &eav1alpha1.EffectivenessAssessment{
			ObjectMeta: metav1.ObjectMeta{Name: eaName, Namespace: testNS},
			Spec: eav1alpha1.EffectivenessAssessmentSpec{
				CorrelationID:           rr.Name,
				RemediationRequestPhase: "Verifying",
				SignalTarget:            eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				RemediationTarget:       eav1alpha1.TargetResource{Kind: "Deployment", Name: "api-server"},
				Config: eav1alpha1.EAConfig{
					StabilizationWindow: metav1.Duration{Duration: 60 * time.Second},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ea)).To(Succeed())
		ea.Status.Phase = eav1alpha1.PhaseStabilizing
		stabDeadline := metav1.NewTime(time.Now().Add(60 * time.Second))
		ea.Status.PrometheusCheckAfter = &stabDeadline
		Expect(k8sClient.Status().Update(ctx, ea)).To(Succeed())

		events := collectSSEEvents(resp, 3, 10*time.Second)

		var hasEAPhase bool
		for _, evt := range events {
			var data map[string]any
			if json.Unmarshal([]byte(evt.Data), &data) != nil {
				continue
			}
			inner, _ := data["params"].(map[string]any)
			if inner == nil {
				continue
			}
			meta, _ := inner["metadata"].(map[string]any)
			if meta != nil && meta["ea_phase"] != nil {
				hasEAPhase = true
			}
		}
		Expect(hasEAPhase).To(BeTrue(),
			"must receive event with ea_phase in metadata during Verifying")
	})

	It("IT-AF-1460-015: terminal phase sends final:true and closes stream", func() {
		rr := createTestRR(ctx, "rr-terminal", testNS, "Executing")
		defer cleanupTestRR(ctx, rr.Name, testNS)

		resp, err := http.Post(statusSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody(rr.Name)))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		time.Sleep(200 * time.Millisecond)
		Expect(k8sClient.Get(ctx, crclient.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseCompleted
		rr.Status.Outcome = "Remediated"
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		events := collectSSEEvents(resp, 2, 5*time.Second)

		var hasFinal bool
		for _, evt := range events {
			var data map[string]any
			if json.Unmarshal([]byte(evt.Data), &data) != nil {
				continue
			}
			inner, _ := data["params"].(map[string]any)
			if inner == nil {
				continue
			}
			if inner["final"] == true {
				hasFinal = true
				Expect(inner).To(HaveKey("metadata"))
				meta, _ := inner["metadata"].(map[string]any)
				Expect(meta["outcome"]).To(Equal("Remediated"))
			}
		}
		Expect(hasFinal).To(BeTrue(), "terminal phase must emit final:true event")
	})

	It("IT-AF-1460-016: rr_not_found returns JSON-RPC error -32001", func() {
		resp, err := http.Post(statusSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody("nonexistent-rr")))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))

		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		errObj, ok := body["error"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(errObj["code"]).To(BeNumerically("==", -32001))
	})

	It("IT-AF-1460-017: already-terminal RR sends single final event and closes", func() {
		rr := createTestRR(ctx, "rr-already-done", testNS, "Completed")
		rr.Status.Outcome = "Remediated"
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())
		defer cleanupTestRR(ctx, rr.Name, testNS)

		resp, err := http.Post(statusSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody(rr.Name)))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		events := collectSSEEvents(resp, 1, 5*time.Second)
		Expect(events).To(HaveLen(1))

		var data map[string]any
		Expect(json.Unmarshal([]byte(events[0].Data), &data)).To(Succeed())
		inner, _ := data["params"].(map[string]any)
		Expect(inner["final"]).To(BeTrue())
		Expect(inner["phase"]).To(Equal("Completed"))
	})

	It("IT-AF-1460-013: watch reconnection re-establishes transparently", func() {
		rr := createTestRR(ctx, "rr-reconnect", testNS, "Processing")

		resp, err := http.Post(statusSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody(rr.Name)))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		time.Sleep(200 * time.Millisecond)

		Expect(k8sClient.Get(ctx, crclient.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseExecuting
		now := metav1.Now()
		rr.Status.ExecutingStartTime = &now
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		time.Sleep(200 * time.Millisecond)

		Expect(k8sClient.Get(ctx, crclient.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseCompleted
		rr.Status.Outcome = "Remediated"
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		events := collectSSEEvents(resp, 3, 5*time.Second)
		Expect(len(events)).To(BeNumerically(">=", 2),
			"stream must survive watch lifecycle and deliver multiple phase transitions")

		var hasFinal bool
		for _, evt := range events {
			var data map[string]any
			if json.Unmarshal([]byte(evt.Data), &data) != nil {
				continue
			}
			inner, _ := data["params"].(map[string]any)
			if inner != nil && inner["final"] == true {
				hasFinal = true
			}
		}
		Expect(hasFinal).To(BeTrue(), "stream must deliver terminal event")
	})

	It("IT-AF-1460-014: status/closing pre-warning before deadline", func() {
		// Deadline must exceed the 5s pre-warning offset so the timer fires.
		// With 8s deadline, pre-warning fires at ~3s into the stream.
		deadlineCtx, deadlineCancel := context.WithTimeout(ctx, 8*time.Second)
		defer deadlineCancel()

		sh := handler.NewStatusHandler(wc, testNS, logf.Log.WithName("status-it-deadline"))
		deadlineSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sh.ServeHTTP(w, r.WithContext(deadlineCtx))
		}))
		defer deadlineSrv.Close()

		rr := createTestRR(ctx, "rr-closing", testNS, "Processing")

		resp, err := http.Post(deadlineSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody(rr.Name)))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		events := collectSSEEvents(resp, 2, 10*time.Second)

		var hasClosing bool
		for _, evt := range events {
			if evt.Event == "status/closing" {
				hasClosing = true
				var data map[string]any
				Expect(json.Unmarshal([]byte(evt.Data), &data)).To(Succeed())
				inner, _ := data["params"].(map[string]any)
				Expect(inner["reason"]).To(Equal("token_expiry"))
				Expect(inner["reconnect"]).To(BeTrue())
			}
		}
		Expect(hasClosing).To(BeTrue(),
			"must receive status/closing event before context deadline kills stream")
	})

	It("IT-AF-1460-018: heartbeat received on idle stream", func() {
		shortHeartbeat := handler.NewStatusHandlerForTest(wc, testNS,
			logf.Log.WithName("status-it-hb"), 500*time.Millisecond)
		hbSrv := httptest.NewServer(shortHeartbeat)
		defer hbSrv.Close()

		rr := createTestRR(ctx, "rr-heartbeat", testNS, "Processing")

		resp, err := http.Post(hbSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody(rr.Name)))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		done := make(chan bool, 1)
		go func() {
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.TrimSpace(line) == ": heartbeat" {
					done <- true
					return
				}
			}
			done <- false
		}()

		select {
		case got := <-done:
			Expect(got).To(BeTrue(), "must receive heartbeat comment frame")
		case <-time.After(3 * time.Second):
			Fail("timed out waiting for heartbeat frame")
		}
	})

	It("IT-AF-1460-019: Blocked phase emits blocked_until, block_reason", func() {
		rr := createTestRR(ctx, "rr-blocked", testNS, "Executing")
		defer cleanupTestRR(ctx, rr.Name, testNS)

		resp, err := http.Post(statusSrv.URL, "application/json",
			bytes.NewReader(statusSubscribeBody(rr.Name)))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		time.Sleep(200 * time.Millisecond)
		Expect(k8sClient.Get(ctx, crclient.ObjectKeyFromObject(rr), rr)).To(Succeed())
		rr.Status.OverallPhase = remediationv1.PhaseBlocked
		rr.Status.BlockReason = remediationv1.BlockReasonConsecutiveFailures
		rr.Status.BlockMessage = "3 consecutive failures"
		blockedTime := metav1.NewTime(time.Now().Add(1 * time.Hour))
		rr.Status.BlockedUntil = &blockedTime
		Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

		events := collectSSEEvents(resp, 2, 5*time.Second)

		var hasBlocked bool
		for _, evt := range events {
			var data map[string]any
			if json.Unmarshal([]byte(evt.Data), &data) != nil {
				continue
			}
			inner, _ := data["params"].(map[string]any)
			if inner == nil {
				continue
			}
			if inner["phase"] == "Blocked" {
				meta, _ := inner["metadata"].(map[string]any)
				Expect(meta).To(HaveKey("blocked_until"))
				Expect(meta).To(HaveKey("block_reason"))
				hasBlocked = true
			}
		}
		Expect(hasBlocked).To(BeTrue(), "must receive Blocked event with metadata")
	})
})
