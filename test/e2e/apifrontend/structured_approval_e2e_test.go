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

package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

// =============================================================================
// E2E-AF-1398: Structured Approval Request Events — Pyramid Invariant E2E tier
//
// These tests prove the full user journey for approval events: user prompt →
// mock-LLM → kubernaut_watch → AwaitingApproval → RAR GET → A2A SSE emission
// with structured approval_request and approval_request_resolved events.
//
// Mock-LLM scenario: af_approval_request
// Keyword trigger: "watch approval gate"
// =============================================================================

var _ = Describe("Structured Approval Events E2E — #1398", Ordered, Label("e2e", "structured-approval"), func() {
	var sreToken string
	const (
		rrName      = "rr-approval-e2e"
		rarName     = "rar-rr-approval-e2e"
		rrNamespace = "kubernaut-system"
	)

	BeforeEach(func() {
		var err error
		sreToken, err = fetchDEXTokenForPersona("sre")
		Expect(err).NotTo(HaveOccurred(), "SRE DEX token required")
		Expect(sreToken).NotTo(BeEmpty())
	})

	setupRRFixture := func() {
		By("Creating RR fixture for approval E2E")
		Expect(createRR(rrNamespace, rrName, "Deployment", "test-deploy-approval")).To(Succeed())
		DeferCleanup(func() { deleteRR(rrNamespace, rrName) })
	}

	setupRARFixture := func() {
		By("Creating RAR fixture for approval E2E")
		rar := buildRAR(rrNamespace, rarName, rrName)
		Expect(k8sClient.Create(context.Background(), rar)).To(Succeed())
		DeferCleanup(func() {
			obj := &remediationv1alpha1.RemediationApprovalRequest{
				ObjectMeta: metav1.ObjectMeta{Name: rarName, Namespace: rrNamespace},
			}
			_ = client.IgnoreNotFound(k8sClient.Delete(context.Background(), obj))
		})
	}

	patchRRToAwaitingApproval := func(ctx context.Context, delay time.Duration) {
		go func() {
			defer GinkgoRecover()
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			rr := &remediationv1alpha1.RemediationRequest{}
			err := k8sClient.Get(ctx, client.ObjectKey{
				Name: rrName, Namespace: rrNamespace,
			}, rr)
			if err != nil {
				return
			}
			patch := client.MergeFrom(rr.DeepCopy())
			rr.Status.OverallPhase = remediationv1alpha1.PhaseAwaitingApproval
			rr.Status.Message = "E2E: approval gate triggered"
			_ = k8sClient.Status().Patch(ctx, rr, patch)
		}()
	}

	patchRARDecision := func(ctx context.Context, decision, decidedBy string, delay time.Duration) {
		go func() {
			defer GinkgoRecover()
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
			rar := &remediationv1alpha1.RemediationApprovalRequest{}
			err := k8sClient.Get(ctx, client.ObjectKey{
				Name: rarName, Namespace: rrNamespace,
			}, rar)
			if err != nil {
				return
			}
			patch := client.MergeFrom(rar.DeepCopy())
			rar.Status.Decision = remediationv1alpha1.ApprovalDecision(decision)
			rar.Status.DecidedBy = decidedBy
			now := metav1.Now()
			rar.Status.DecidedAt = &now
			_ = k8sClient.Status().Patch(ctx, rar, patch)
		}()
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

	scanApprovalEvent := func(resp *http.Response, metaType string) (string, map[string]any) {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if !strings.HasPrefix(strings.TrimSpace(line), "data:") {
				continue
			}
			data := strings.TrimPrefix(strings.TrimSpace(line), "data:")
			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}

			var frame struct {
				Result struct {
					Kind   string `json:"kind"`
					Status struct {
						Message struct {
							Parts []struct {
								Text string `json:"text"`
							} `json:"parts"`
						} `json:"message"`
					} `json:"status"`
					Metadata map[string]any `json:"metadata"`
				} `json:"result"`
			}
			if json.Unmarshal([]byte(data), &frame) != nil {
				continue
			}
			if frame.Result.Kind != "status-update" {
				continue
			}
			if frame.Result.Metadata == nil {
				continue
			}
			if frame.Result.Metadata["type"] != metaType {
				continue
			}
			if len(frame.Result.Status.Message.Parts) == 0 {
				continue
			}
			text := frame.Result.Status.Message.Parts[0].Text
			if text == "" {
				continue
			}
			return text, frame.Result.Metadata
		}
		return "", nil
	}

	It("E2E-AF-1398-001: AU-3 — approval_request event with full RAR spec on AwaitingApproval", func() {
		setupRRFixture()
		setupRARFixture()

		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		patchRRToAwaitingApproval(readCtx, 3*time.Second)

		resp, err := a2aSSEPost(readCtx, a2aMessageStream(
			fmt.Sprintf("e2e-approval-001-%d", time.Now().UnixNano()),
			"watch approval gate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

		text, meta := scanApprovalEvent(resp, "approval_request")
		Expect(text).NotTo(BeEmpty(), "should receive approval_request event with JSON payload")
		Expect(meta["type"]).To(Equal("approval_request"))

		var payload struct {
			Name                   string  `json:"name"`
			Namespace              string  `json:"namespace"`
			RemediationRequestName string  `json:"remediationRequestName"`
			Confidence             float64 `json:"confidence"`
			ConfidenceLevel        string  `json:"confidenceLevel"`
			Reason                 string  `json:"reason"`
			InvestigationSummary   string  `json:"investigationSummary"`
			RecommendedWorkflow    *struct {
				WorkflowID string `json:"workflowId"`
			} `json:"recommendedWorkflow"`
			EvidenceCollected []string `json:"evidenceCollected"`
		}
		err = json.Unmarshal([]byte(text), &payload)
		Expect(err).NotTo(HaveOccurred(), "AU-3: approval payload must be valid JSON")

		By("Verifying approval request fields")
		Expect(payload.Name).To(Equal(rarName))
		Expect(payload.Namespace).To(Equal(rrNamespace))
		Expect(payload.RemediationRequestName).To(Equal(rrName),
			"AC-6: remediationRequestName provides breadcrumb context")
		Expect(payload.Confidence).To(BeNumerically(">", 0))
		Expect(payload.ConfidenceLevel).NotTo(BeEmpty())
		Expect(payload.Reason).NotTo(BeEmpty())
		Expect(payload.InvestigationSummary).NotTo(BeEmpty())
		Expect(payload.RecommendedWorkflow).NotTo(BeNil())
	})

	It("E2E-AF-1398-002: SI-4 — MCP approve triggers approval_request_resolved on SSE stream", func() {
		setupRRFixture()
		setupRARFixture()

		readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		patchRARDecision(readCtx, "Approved", "e2e-operator@acme.com", 2*time.Second)
		patchRRToAwaitingApproval(readCtx, 4*time.Second)

		resp, err := a2aSSEPost(readCtx, a2aMessageStream(
			fmt.Sprintf("e2e-approval-002-%d", time.Now().UnixNano()),
			"watch approval gate"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		text, meta := scanApprovalEvent(resp, "approval_request_resolved")
		Expect(text).NotTo(BeEmpty(), "should receive approval_request_resolved event")
		Expect(meta["type"]).To(Equal("approval_request_resolved"))

		var payload struct {
			Name      string `json:"name"`
			Decision  string `json:"decision"`
			DecidedBy string `json:"decidedBy"`
		}
		err = json.Unmarshal([]byte(text), &payload)
		Expect(err).NotTo(HaveOccurred(), "SI-4: resolution payload must be parseable")
		Expect(payload.Decision).To(Equal("Approved"))
		Expect(payload.DecidedBy).To(Equal("e2e-operator@acme.com"))
	})

	It("E2E-AF-1398-003: SI-17 — RAR timeout produces resolved with Expired decision", func() {
		setupRRFixture()
		setupRARFixture()

		readCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()

		patchRARDecision(readCtx, "Expired", "system", 2*time.Second)
		patchRRToAwaitingApproval(readCtx, 4*time.Second)

		resp, err := a2aSSEPost(readCtx, a2aMessageStream(
			fmt.Sprintf("e2e-approval-003-%d", time.Now().UnixNano()),
			"watch approval gate expire"))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		text, _ := scanApprovalEvent(resp, "approval_request_resolved")
		Expect(text).NotTo(BeEmpty(), "should receive expired resolution event")

		var payload struct {
			Decision string `json:"decision"`
		}
		err = json.Unmarshal([]byte(text), &payload)
		Expect(err).NotTo(HaveOccurred())
		Expect(payload.Decision).To(Equal("Expired"),
			"SI-17: timeout must produce Expired decision in resolution event")
	})
})
