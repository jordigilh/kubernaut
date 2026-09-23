package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Fleet-mode API Frontend contracts [BR-FLEET-054, BR-INTEGRATION-065]", Label("e2e", "fleet-mode-af", "issue-2462"), func() {
	var authToken string

	BeforeEach(func() {
		Expect(fleetAFEnabled).To(BeTrue(), "Fleet AF infrastructure must be enabled for Fleet-mode contracts")
		token, err := fetchFleetAFA2AToken()
		Expect(err).NotTo(HaveOccurred(), "Fleet AF SRE Keycloak password token")
		authToken = token
	})

	It("E2E-AF-FLEET-2462-001 [BR-FLEET-054, BR-INTEGRATION-065]: preserves hub severity and filters a remote-cluster collision", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		Expect(waitForFleetAFPrometheusRule(ctx, "AFHubClusterTriage2394", kinfra.RuleStateFiring)).To(Succeed())
		Expect(waitForFleetAFPrometheusRule(ctx, "AFRemoteClusterTriageCollision2394", kinfra.RuleStateFiring)).To(Succeed())

		rr := invokeFleetAFRemediate(ctx, authToken,
			"fleet e2e hub cluster triage", "fleet-af-hub-triage", "af-hub-triage-target")
		Expect(rr.Spec.ClusterID).To(Equal("hub"), "BR-FLEET-054: RR attribution must retain the registered hub ID")
		Expect(rr.Spec.Severity).To(Equal("warning"),
			"the more severe critical alert belongs to remote-cluster and must not affect the hub-targeted RR")
	})

	It("E2E-AF-FLEET-2462-001a [BR-FLEET-054, BR-INTEGRATION-065]: firing-alert severity is grounded through the hub Gateway", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		Expect(injectMetricForTier2(ctx, fleetAFPrometheusURL, "e2e_cpu_usage_percent", 95, map[string]string{
			"namespace": "sev-tier1-ns", "kind": "Deployment", "name": "test-firing-target",
		})).To(Succeed())
		Expect(waitForFleetAFPrometheusRule(ctx, "HighCPU", kinfra.RuleStateFiring)).To(Succeed())
		rr := invokeFleetAFRemediate(ctx, authToken,
			"fleet e2e severity tier 1", "sev-tier1-ns", "test-firing-target")
		Expect(rr.Spec.ClusterID).To(Equal("hub"))
		Expect(rr.Spec.Severity).To(Equal("critical"))
		Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("severity_source", "firing_alert"))
	})

	It("E2E-AF-FLEET-2462-001b [BR-FLEET-054, BR-INTEGRATION-065]: pending-alert severity remains attributable to the hub", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		Expect(injectMetricForTier2(ctx, fleetAFPrometheusURL, "e2e_memory_usage_percent", 90, map[string]string{
			"namespace": "sev-tier15-ns", "kind": "Deployment", "name": "test-pending-target",
		})).To(Succeed())
		Expect(waitForFleetAFPrometheusRule(ctx, "HighMemory", kinfra.RuleStatePending)).To(Succeed())
		rr := invokeFleetAFRemediate(ctx, authToken,
			"fleet e2e severity tier 15", "sev-tier15-ns", "test-pending-target")
		Expect(rr.Spec.ClusterID).To(Equal("hub"))
		Expect(rr.Spec.Severity).NotTo(BeEmpty())
		Expect(rr.Spec.SignalLabels["severity_source"]).To(BeElementOf(
			"pending_alert", "firing_alert", "ns_pending_alert", "ns_firing_alert",
			"cluster_pending_alert", "cluster_firing_alert", "llm_rule_informed", "llm_triage"))
	})

	It("E2E-AF-FLEET-2462-001c [BR-FLEET-054, BR-INTEGRATION-065]: inactive rules with live metric data retain Tier 2 triage", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		Expect(injectMetricForTier2(ctx, fleetAFPrometheusURL, "e2e_disk_usage_percent", 80, map[string]string{
			"namespace": "sev-tier2-ns", "kind": "Deployment", "name": "test-inactive-target",
		})).To(Succeed())

		rr := invokeFleetAFRemediate(ctx, authToken,
			"fleet e2e severity tier 2", "sev-tier2-ns", "test-inactive-target")
		Expect(rr.Spec.ClusterID).To(Equal("hub"))
		Expect(rr.Spec.SignalLabels["severity_source"]).To(BeElementOf("rule_evaluation", "llm_rule_informed"))
	})

	It("E2E-AF-FLEET-2462-001d [BR-FLEET-054, BR-INTEGRATION-065]: no-data severity fallback remains attributed to the hub", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		rr := invokeFleetAFRemediate(ctx, authToken,
			"fleet e2e severity tier 25", "no-data-ns", "test-nodata-target")
		Expect(rr.Spec.ClusterID).To(Equal("hub"))
		Expect(rr.Spec.SignalLabels).To(HaveKeyWithValue("severity_source", "llm_rule_informed"))
	})

	It("E2E-AF-FLEET-2462-001e [BR-FLEET-054, BR-AI-056]: missing alert and rule evidence fails closed", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		resp, err := fleetA2AStream(ctx, authToken,
			a2aMessageStream(fleetAFUniqueTaskID("fleet-af-no-rules"), "fleet e2e severity tier 5"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.ToLower(string(body))).To(ContainSubstring("severity"),
			"the streamed response must explain why triage could not determine severity")

		Consistently(func(g Gomega) {
			exists, listErr := fleetAFHasRR(context.Background(), "no-rules-ns", "test-norules-target", "hub")
			g.Expect(listErr).NotTo(HaveOccurred())
			g.Expect(exists).To(BeFalse(),
				"a target with no correlated alert or rule must not produce an RR")
		}, 5*time.Second, time.Second).Should(Succeed())
	})

	It("E2E-AF-FLEET-2462-001f [BR-FLEET-054, BR-INTEGRATION-065]: a user severity hint cannot bypass cluster-attributed triage", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		rr := invokeFleetAFRemediate(ctx, authToken,
			"fleet e2e severity tier 6 with severity low", "sev-userhint-ns", "test-user-severity-bypass")
		Expect(rr.Spec.ClusterID).To(Equal("hub"))
		Expect(rr.Spec.Severity).NotTo(BeEmpty())
		Expect(rr.Spec.SignalLabels).To(HaveKey("severity_source"))
	})

	It("E2E-AF-FLEET-2462-002 [BR-FLEET-054, BR-INTEGRATION-065]: structured RCA and workflow options arrive intact over Fleet SSE", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		const contextID = "ctx-fleet-af-structured-2462"

		groundFleetAFSession(ctx, authToken, "fleet structured grounding one", contextID)
		resp, err := fleetA2AStream(ctx, authToken,
			a2aMessageStreamWithContext(fleetAFUniqueTaskID("fleet-af-structured-decision"), contextID, "fleet present structured rca decision"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

		text, metadata := findFleetAFDecisionEvent(resp)
		Expect(metadata["type"]).To(Equal("decision"))
		Expect(len(text)).To(BeNumerically(">", 512), "SI-10: the structured decision payload must not be truncated")
		Expect(text).NotTo(HaveSuffix("..."))

		var decision struct {
			RCA struct {
				Severity    string   `json:"severity"`
				Confidence  float64  `json:"confidence"`
				CausalChain []string `json:"causal_chain"`
			} `json:"rca"`
			Options []struct {
				WorkflowID     string            `json:"workflow_id"`
				Recommended    bool              `json:"recommended"`
				Parameters     map[string]string `json:"parameters"`
				RuledOutReason string            `json:"ruled_out_reason"`
			} `json:"options"`
		}
		Expect(json.Unmarshal([]byte(text), &decision)).To(Succeed(), "AU-3: decision must be valid structured JSON")
		Expect(decision.RCA.Severity).To(Equal("critical"))
		Expect(decision.RCA.Confidence).To(BeNumerically("~", 0.92, 0.01))
		Expect(decision.RCA.CausalChain).To(HaveLen(3))
		Expect(decision.Options).To(HaveLen(3))
		Expect(decision.Options[0].Recommended).To(BeTrue())
		Expect(decision.Options[0].Parameters).To(HaveKeyWithValue("deployment", "data-processor"))
		Expect(decision.Options[2].RuledOutReason).To(ContainSubstring("No previous revision"))
	})

	It("E2E-AF-FLEET-2462-003 [BR-AI-056, BR-INTEGRATION-065]: progressive investigation emits early RCA and investigation summary", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		resp, err := fleetA2AStream(ctx, authToken,
			a2aMessageStream(fleetAFUniqueTaskID("fleet-af-progressive"), "fleet progressive investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		earlyRCAFound, earlySeverity, earlyConfidence, summaryFound := false, false, false, false
		for _, event := range readFleetAFEvents(resp) {
			metadata, _ := event["metadata"].(map[string]any)
			if metadata != nil && metadata["schema"] == "early_rca" {
				earlyRCAFound = true
				status, _ := event["status"].(map[string]any)
				message, _ := status["message"].(map[string]any)
				parts, _ := message["parts"].([]any)
				if len(parts) > 0 {
					part, _ := parts[0].(map[string]any)
					payloadText, _ := part["text"].(string)
					var payload map[string]any
					if json.Unmarshal([]byte(payloadText), &payload) == nil {
						_, earlySeverity = payload["severity"]
						_, earlyConfidence = payload["confidence"]
					}
				}
			}
			if fleetAFHasInvestigationSummary(event) {
				summaryFound = true
			}
		}
		Expect(earlyRCAFound).To(BeTrue(), "SI-4: the Fleet investigation must emit an early RCA decision event")
		Expect(earlySeverity).To(BeTrue(), "AU-3: early RCA must carry severity")
		Expect(earlyConfidence).To(BeTrue(), "AU-3: early RCA must carry confidence")
		Expect(summaryFound).To(BeTrue(), "SI-10: the stream must include the investigation_summary DataPart")
	})

	It("E2E-AF-FLEET-2462-004 [BR-INTERACTIVE-004, BR-INTEGRATION-065]: concurrent session_active fallback retains a renderable causal chain", func() {
		firstCtx, firstCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer firstCancel()
		firstStarted := make(chan error, 1)
		go func() {
			resp, err := fleetA2AStream(firstCtx, authToken,
				a2aMessageStream(fleetAFUniqueTaskID("fleet-af-session-active-first"), "fleet session active investigate"))
			if err != nil {
				firstStarted <- err
				return
			}
			firstStarted <- nil
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)
		}()

		select {
		case err := <-firstStarted:
			Expect(err).NotTo(HaveOccurred(), "first Fleet investigation stream must start")
		case <-time.After(15 * time.Second):
			Fail("first Fleet investigation did not begin streaming within 15 seconds")
		}
		time.Sleep(3 * time.Second)

		secondCtx, secondCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer secondCancel()
		resp, err := fleetA2AStream(secondCtx, authToken,
			a2aMessageStream(fleetAFUniqueTaskID("fleet-af-session-active-second"), "fleet session active investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		foundSummary, hasCausalChain := false, false
		for _, event := range readFleetAFEvents(resp) {
			if fleetAFHasInvestigationSummary(event) {
				foundSummary = true
				hasCausalChain = fleetAFSummaryHasCausalChain(event)
				break
			}
		}
		Expect(foundSummary).To(BeTrue(), "AC-4: rejected concurrent caller must receive an investigation_summary fallback")
		Expect(hasCausalChain).To(BeTrue(), "AC-4: fallback RCA must carry a non-empty causal chain")
	})

	It("E2E-AF-FLEET-2462-005 [BR-FLEET-054, BR-INTEGRATION-065]: unregistered cluster identity fails closed despite a same-named hub target", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		Expect(waitForFleetAFPrometheusRule(ctx, "FleetUnregisteredClusterGrounding", kinfra.RuleStateFiring)).To(Succeed())

		resp, err := fleetA2AStream(ctx, authToken,
			a2aMessageStream(fleetAFUniqueTaskID("fleet-af-unregistered"), "fleet e2e unregistered cluster investigate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.ToLower(string(body))).To(ContainSubstring("cluster"),
			"the streamed error must explain that the requested cluster identity is unavailable")

		Consistently(func(g Gomega) {
			exists, listErr := fleetAFHasRR(context.Background(), "fleet-unregistered-cluster-e2e", "fleet-unregistered-target", "unregistered-cluster-2462")
			g.Expect(listErr).NotTo(HaveOccurred())
			g.Expect(exists).To(BeFalse(),
				"AF must not fall back to the same-named object in the physical hub cluster")
		}, 5*time.Second, time.Second).Should(Succeed())
	})
})

func fetchFleetAFA2AToken() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return kinfra.GetKeycloakPasswordToken(ctx,
		kinfra.DefaultKeycloakAFA2AConfig(fleetAFKeycloakHostPort, fleetAFKubeconfigPath))
}

func fleetA2AInvoke(ctx context.Context, token, body string) (*http.Response, error) {
	return fleetAFRequest(ctx, token, body, false)
}

func fleetA2AStream(ctx context.Context, token, body string) (*http.Response, error) {
	return fleetAFRequest(ctx, token, body, true)
}

func fleetAFRequest(ctx context.Context, token, body string, stream bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fleetAFBaseURL+"/a2a/invoke", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build Fleet AF request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := fleetAFHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send Fleet AF request: %w", err)
	}
	return resp, nil
}

func invokeFleetAFRemediate(ctx context.Context, token, prompt, namespace, targetName string) *remediationv1alpha1.RemediationRequest {
	var found *remediationv1alpha1.RemediationRequest
	attempt := 0
	Eventually(func(g Gomega) {
		attempt++
		resp, err := fleetA2AInvoke(ctx, token, a2aTasksSend(fleetAFUniqueTaskID("fleet-af-remediate"), prompt))
		g.Expect(err).NotTo(HaveOccurred())
		if err != nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()
		g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
		rpc, parseErr := parseRPCResponse(resp)
		g.Expect(parseErr).NotTo(HaveOccurred())
		g.Expect(rpc.Error).To(BeNil())

		var requests remediationv1alpha1.RemediationRequestList
		g.Expect(fleetAFK8sClient.List(ctx, &requests, client.InNamespace(e2eNamespace))).To(Succeed())
		for i := range requests.Items {
			request := &requests.Items[i]
			if request.Spec.ClusterID == "hub" && request.Spec.TargetResource.Namespace == namespace && request.Spec.TargetResource.Name == targetName {
				found = request.DeepCopy()
				break
			}
		}
		g.Expect(found).NotTo(BeNil(), "hub-scoped RR for %s/%s must appear after attempt %d", namespace, targetName, attempt)
	}, 90*time.Second, 2*time.Second).Should(Succeed(), "Fleet AF remediation must become visible through the hub registration")
	return found
}

func fleetAFHasRR(ctx context.Context, namespace, targetName, clusterID string) (bool, error) {
	var requests remediationv1alpha1.RemediationRequestList
	if err := fleetAFK8sClient.List(ctx, &requests, client.InNamespace(e2eNamespace)); err != nil {
		return false, fmt.Errorf("list Fleet AF remediation requests: %w", err)
	}
	for i := range requests.Items {
		request := &requests.Items[i]
		if request.Spec.ClusterID == clusterID && request.Spec.TargetResource.Namespace == namespace && request.Spec.TargetResource.Name == targetName {
			return true, nil
		}
	}
	return false, nil
}

func groundFleetAFSession(ctx context.Context, token, prompt, contextID string) {
	resp, err := fleetA2AStream(ctx, token,
		a2aMessageStreamWithContext(fleetAFUniqueTaskID("fleet-af-ground"), contextID, prompt))
	Expect(err).NotTo(HaveOccurred(), "Fleet AF grounding call must succeed")
	defer func() { _ = resp.Body.Close() }()
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	_, err = io.Copy(io.Discard, resp.Body)
	Expect(err).NotTo(HaveOccurred(), "grounding stream must reach EOF")
}

func findFleetAFDecisionEvent(resp *http.Response) (string, map[string]any) {
	for _, event := range readFleetAFEvents(resp) {
		if event["kind"] == "artifact-update" {
			artifact, _ := event["artifact"].(map[string]any)
			if artifact == nil {
				continue
			}
			metadata, _ := artifact["metadata"].(map[string]any)
			if metadata == nil || metadata["type"] != "decision" {
				continue
			}
			parts, _ := artifact["parts"].([]any)
			for _, rawPart := range parts {
				part, _ := rawPart.(map[string]any)
				if part == nil {
					continue
				}
				if data, ok := part["data"].(map[string]any); ok {
					encoded, err := json.Marshal(data)
					Expect(err).NotTo(HaveOccurred())
					return string(encoded), metadata
				}
				if text, ok := part["text"].(string); ok && text != "" {
					return text, metadata
				}
			}
		}
		if event["kind"] == statusUpdate {
			metadata, _ := event["metadata"].(map[string]any)
			if metadata == nil || metadata["type"] != "decision" {
				continue
			}
			status, _ := event["status"].(map[string]any)
			message, _ := status["message"].(map[string]any)
			parts, _ := message["parts"].([]any)
			if len(parts) > 0 {
				part, _ := parts[0].(map[string]any)
				if text, ok := part["text"].(string); ok && text != "" && !strings.Contains(text, "Presenting decision") {
					return text, metadata
				}
			}
		}
	}
	return "", nil
}

func readFleetAFEvents(resp *http.Response) []map[string]any {
	var events []map[string]any
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimRight(scanner.Text(), "\r"))
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || !strings.HasPrefix(data, "{") {
			continue
		}
		var envelope struct {
			Result map[string]any `json:"result"`
		}
		if err := json.Unmarshal([]byte(data), &envelope); err == nil && envelope.Result != nil {
			events = append(events, envelope.Result)
		}
	}
	Expect(scanner.Err()).NotTo(HaveOccurred(), "Fleet AF SSE stream must parse to EOF")
	return events
}

func fleetAFHasInvestigationSummary(event map[string]any) bool {
	if event["kind"] != artifactUpdate {
		return false
	}
	artifact, _ := event["artifact"].(map[string]any)
	if artifact == nil {
		return false
	}
	metadata, _ := artifact["metadata"].(map[string]any)
	if metadata == nil || metadata["schema"] != investigationSummarySchema || metadata["schema_version"] != "1.0" {
		return false
	}
	parts, _ := artifact["parts"].([]any)
	for _, rawPart := range parts {
		part, _ := rawPart.(map[string]any)
		data, _ := part["data"].(map[string]any)
		if data != nil {
			if _, hasSummary := data["summary"]; hasSummary {
				return true
			}
		}
	}
	return false
}

func fleetAFSummaryHasCausalChain(event map[string]any) bool {
	artifact, _ := event["artifact"].(map[string]any)
	if artifact == nil {
		return false
	}
	parts, _ := artifact["parts"].([]any)
	for _, rawPart := range parts {
		part, _ := rawPart.(map[string]any)
		data, _ := part["data"].(map[string]any)
		rca, _ := data["rca"].(map[string]any)
		chain, _ := rca["causal_chain"].([]any)
		if len(chain) > 0 {
			return true
		}
	}
	return false
}
