package fullpipeline

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ────────────────────────────────────────────────────────────────────────────
// JSON-RPC helpers (adapted from test/e2e/apifrontend/helpers_test.go)
// ────────────────────────────────────────────────────────────────────────────

func fpBuildJSONRPC(id, method string, params map[string]interface{}) string {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"id":      id,
		"params":  params,
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// fpA2ATasksSend builds a message/send JSON-RPC payload with a user text message.
func fpA2ATasksSend(id, text string) string {
	return fpBuildJSONRPC(id, "message/send", map[string]interface{}{
		"message": map[string]interface{}{
			"messageId": "msg-" + id,
			"role":      "user",
			"parts": []map[string]interface{}{
				{"kind": "text", "text": text},
			},
		},
	})
}

// fpA2ATasksSendWithTask continues an existing A2A task by including taskId.
func fpA2ATasksSendWithTask(id, taskID, text string) string {
	return fpBuildJSONRPC(id, "message/send", map[string]interface{}{
		"id": taskID,
		"message": map[string]interface{}{
			"messageId": "msg-" + id,
			"role":      "user",
			"parts": []map[string]interface{}{
				{"kind": "text", "text": text},
			},
		},
	})
}

type fpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *fpRPCError     `json:"error"`
}

type fpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type fpA2ATaskResult struct {
	ID     string `json:"id"`
	Status struct {
		State   string          `json:"state"`
		Message json.RawMessage `json:"message,omitempty"`
	} `json:"status"`
}

// fpA2AInvoke sends a JSON-RPC request to POST /a2a/invoke.
func fpA2AInvoke(body string) (*http.Response, error) {
	return fpA2AInvokeWithTimeout(body, 0)
}

// fpA2AInvokeWithTimeout sends a JSON-RPC request to POST /a2a/invoke with a
// custom timeout. Use for multi-turn conversations where the session may still
// be processing a prior turn's tool chain (AF → MCP → KA → mock-LLM).
// A zero timeout uses the default afHTTPClient (30s).
func fpA2AInvokeWithTimeout(body string, timeout time.Duration) (*http.Response, error) {
	token := getAFToken()
	req, err := http.NewRequest(http.MethodPost, afBaseURL+"/a2a/invoke", strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if timeout > 0 {
		client := &http.Client{
			Transport: afHTTPClient.Transport,
			Timeout:   timeout,
		}
		return client.Do(req)
	}
	return afHTTPClient.Do(req)
}

func fpParseRPC(resp *http.Response) (fpRPCResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fpRPCResponse{}, err
	}
	var r fpRPCResponse
	err = json.Unmarshal(body, &r)
	return r, err
}

func fpExtractTask(raw json.RawMessage) (fpA2ATaskResult, error) {
	var task fpA2ATaskResult
	err := json.Unmarshal(raw, &task)
	return task, err
}

// ────────────────────────────────────────────────────────────────────────────
// Pipeline polling helpers
// ────────────────────────────────────────────────────────────────────────────

// fpWaitForRR polls for a RemediationRequest whose targetResource.name contains
// nameSubstring. Returns the RR name or fails after timeout.
func fpWaitForRR(nameSubstring string, timeout time.Duration) string {
	var rrName string
	Eventually(func() bool {
		rrList := &remediationv1.RemediationRequestList{}
		if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
			return false
		}
		for _, rr := range rrList.Items {
			if strings.Contains(rr.Spec.TargetResource.Name, nameSubstring) {
				rrName = rr.Name
				return true
			}
		}
		return false
	}, timeout, 2*time.Second).Should(BeTrue(), "RemediationRequest targeting %q not found", nameSubstring)
	return rrName
}

// fpWaitForRRWithTargetNS polls for a RemediationRequest whose targetResource.name
// contains nameSubstring AND whose spec.targetResource.namespace equals targetNS.
// This avoids picking up RRs from parallel tests that share the same name pattern
// but target different namespaces.
func fpWaitForRRWithTargetNS(nameSubstring, targetNS string, timeout time.Duration) string {
	var rrName string
	Eventually(func() bool {
		rrList := &remediationv1.RemediationRequestList{}
		if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
			return false
		}
		for _, rr := range rrList.Items {
			if strings.Contains(rr.Spec.TargetResource.Name, nameSubstring) && rr.Spec.TargetResource.Namespace == targetNS {
				rrName = rr.Name
				return true
			}
		}
		return false
	}, timeout, 2*time.Second).Should(BeTrue(),
		"RemediationRequest targeting %q in namespace %q not found", nameSubstring, targetNS)
	return rrName
}

// rrFingerprint computes the signal fingerprint for a target resource, matching
// the production logic in HandleCreateRR (pkg/apifrontend/tools/af_create_rr.go).
func rrFingerprint(namespace, kind, name string) string {
	h := sha256.Sum256([]byte(namespace + "/" + kind + "/" + name))
	return fmt.Sprintf("%x", h)
}

// fpWaitForRRByFingerprint polls for a RemediationRequest whose spec.signalFingerprint
// matches the given fingerprint. This uniquely identifies an RR by its target resource
// (namespace+kind+name hash) regardless of deployment name collisions across tests.
func fpWaitForRRByFingerprint(fingerprint string, timeout time.Duration) string {
	var rrName string
	Eventually(func() bool {
		rrList := &remediationv1.RemediationRequestList{}
		if err := apiReader.List(ctx, rrList, client.InNamespace(namespace)); err != nil {
			return false
		}
		for _, rr := range rrList.Items {
			if rr.Spec.SignalFingerprint == fingerprint {
				rrName = rr.Name
				return true
			}
		}
		return false
	}, timeout, 2*time.Second).Should(BeTrue(),
		"RemediationRequest with fingerprint %q not found", fingerprint[:12])
	return rrName
}

// fpWaitForWEComplete waits until a WorkflowExecution for the given RR reaches Completed phase.
// Fails fast if the WE enters Failed phase, and logs pipeline state periodically for diagnostics.
func fpWaitForWEComplete(rrName string, timeout time.Duration) {
	lastLog := time.Time{}
	logInterval := 30 * time.Second
	Eventually(func() bool {
		now := time.Now()
		shouldLog := now.Sub(lastLog) >= logInterval

		weList := &workflowexecutionv1.WorkflowExecutionList{}
		if err := apiReader.List(ctx, weList, client.InNamespace(namespace)); err != nil {
			if shouldLog {
				GinkgoWriter.Printf("  [fpWaitForWEComplete] failed to list WEs: %v\n", err)
				lastLog = now
			}
			return false
		}
		for _, we := range weList.Items {
			if strings.Contains(we.Name, rrName) || we.Spec.RemediationRequestRef.Name == rrName {
				phase := string(we.Status.Phase)
				if phase == "Failed" {
					Fail(fmt.Sprintf("WorkflowExecution %s failed (phase=Failed)", we.Name))
				}
				if shouldLog {
					GinkgoWriter.Printf("  WE %s phase: %s, engine: %s\n", we.Name, phase, we.Status.ExecutionEngine)
					lastLog = now
				}
				return phase == "Completed"
			}
		}

		// No WE found yet -- check upstream pipeline state for diagnostics.
		if shouldLog {
			aaList := &aianalysisv1.AIAnalysisList{}
			if err := apiReader.List(ctx, aaList, client.InNamespace(namespace)); err == nil {
				found := false
				for _, aa := range aaList.Items {
					if strings.Contains(aa.Name, rrName) || aa.Spec.RemediationRequestRef.Name == rrName {
						GinkgoWriter.Printf("  AA %s phase: %s (WE not yet created)\n", aa.Name, aa.Status.Phase)
						found = true
						break
					}
				}
				if !found {
					GinkgoWriter.Printf("  [fpWaitForWEComplete] no WE or AA found for RR %q (pipeline may not have started)\n", rrName)
				}
			}
			lastLog = now
		}
		return false
	}, timeout, 3*time.Second).Should(BeTrue(), "WorkflowExecution for %q did not complete", rrName)
}

// fpWaitForPodCrash waits until at least one pod matching the given label has
// entered a crash state (OOMKilled or CrashLoopBackOff). AF's deriveSignalName
// uses DominantEventReason to produce a grounded signal name; if called before
// the pod crashes, only Normal lifecycle events (ScalingReplicaSet, Scheduled)
// exist and DominantEventReason correctly returns "" (F-SIG-08). Waiting for a
// crash ensures Warning events like BackOff are present, giving KA a meaningful
// signal to drive investigation.
func fpWaitForPodCrash(appLabel string, timeout time.Duration) {
	fpWaitForPodCrashInNS(appLabel, namespace, timeout)
}

// fpWaitForPodCrashInNS waits until at least one pod matching the given label
// in the specified namespace has entered a crash state (OOMKilled or CrashLoopBackOff).
func fpWaitForPodCrashInNS(appLabel, ns string, timeout time.Duration) {
	Eventually(func() bool {
		pods := &corev1.PodList{}
		if err := apiReader.List(ctx, pods,
			client.InNamespace(ns),
			client.MatchingLabels{"app": appLabel}); err != nil {
			return false
		}
		for _, pod := range pods.Items {
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.LastTerminationState.Terminated != nil &&
					cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
					GinkgoWriter.Printf("  OOMKill detected for %s in %s (restarts=%d)\n", appLabel, ns, cs.RestartCount)
					return true
				}
				if cs.State.Terminated != nil &&
					cs.State.Terminated.Reason == "OOMKilled" {
					GinkgoWriter.Printf("  OOMKill detected for %s in %s\n", appLabel, ns)
					return true
				}
				if cs.RestartCount > 0 && cs.State.Waiting != nil &&
					cs.State.Waiting.Reason == "CrashLoopBackOff" {
					GinkgoWriter.Printf("  CrashLoopBackOff detected for %s in %s\n", appLabel, ns)
					return true
				}
			}
		}
		return false
	}, timeout, 2*time.Second).Should(BeTrue(),
		"pod with label app=%s in namespace %s should crash (OOMKill or CrashLoopBackOff)", appLabel, ns)
}

// fpAssertNoISForRR asserts that no InvestigationSession exists for the given RR.
// Issue #1332: Autonomous flow (kubernaut_remediate) must NOT create an IS.
func fpAssertNoISForRR(rrName, ns string) {
	var isList isv1alpha1.InvestigationSessionList
	err := k8sClient.List(ctx, &isList, client.InNamespace(ns))
	Expect(err).NotTo(HaveOccurred(), "failed to list InvestigationSessions")

	for _, is := range isList.Items {
		for _, ref := range is.OwnerReferences {
			Expect(ref.Name).NotTo(Equal(rrName),
				"autonomous flow must not create InvestigationSession for RR %s", rrName)
		}
	}
}

