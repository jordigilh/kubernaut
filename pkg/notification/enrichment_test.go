/*
Copyright 2025 Jordi Gil.

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

package notification_test

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/enrichment"
)

const testUUID = "53c7c5d3-ee13-42e5-a920-43f3df75ec6d"

func buildTestNotification(body string, metadata map[string]string) *notificationv1alpha1.NotificationRequest {
	nctx := &notificationv1alpha1.NotificationContext{}
	if wfID := metadata["workflowId"]; wfID != "" {
		if nctx.Workflow == nil {
			nctx.Workflow = &notificationv1alpha1.WorkflowContext{}
		}
		nctx.Workflow.WorkflowID = wfID
	}
	if sw := metadata["selectedWorkflow"]; sw != "" {
		if nctx.Workflow == nil {
			nctx.Workflow = &notificationv1alpha1.WorkflowContext{}
		}
		nctx.Workflow.SelectedWorkflow = sw
	}
	if ee := metadata["executionEngine"]; ee != "" {
		if nctx.Workflow == nil {
			nctx.Workflow = &notificationv1alpha1.WorkflowContext{}
		}
		nctx.Workflow.ExecutionEngine = ee
	}
	if wn := metadata["workflowName"]; wn != "" {
		if nctx.Workflow == nil {
			nctx.Workflow = &notificationv1alpha1.WorkflowContext{}
		}
		nctx.Workflow.WorkflowName = wn
	}
	if rr := metadata["remediationRequest"]; rr != "" {
		nctx.Lineage = &notificationv1alpha1.LineageContext{RemediationRequest: rr}
	}
	if rc := metadata["rootCause"]; rc != "" {
		nctx.Analysis = &notificationv1alpha1.AnalysisContext{RootCause: rc}
	}
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nr-test",
			Namespace: "default",
			UID:       types.UID("test-uid"),
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Subject: "Remediation Completed: high-memory-alert",
			Body:    body,
			Context: nctx,
		},
	}
}

// #553: Workflow Name Enrichment.
//
// #1677 follow-up cleanup: this suite originally exercised a live
// DataStorage-backed WorkflowNameResolver, with WorkflowName deliberately
// left unpopulated so the resolver mock was the only source of the name.
// That resolver/fallback mechanism has since been deleted outright
// (pkg/notification/enrichment/resolver.go, and the fallback branch in
// Enricher.EnrichNotification): RemediationOrchestrator has populated
// WorkflowName directly from AIAnalysis.Status.SelectedWorkflow.WorkflowName
// since Issue #1677 Phase 1, and that field is +kubebuilder:validation:Required
// on the underlying WorkflowSnapshot type -- always equal to
// RemediationWorkflow.metadata.name, a Kubernetes-guaranteed non-empty value
// -- whenever a workflow ID is present at all. So "WorkflowID/SelectedWorkflow
// present but WorkflowName absent" cannot occur via either of RO's two
// notification-creation call sites (pkg/remediationorchestrator/creator/
// notification.go), and every spec below now pre-populates workflowName the
// same way those real callers do, instead of a resolver mock.
var _ = Describe("#553: Workflow Name Enrichment", func() {

	newEnricher := func() *enrichment.Enricher {
		return enrichment.NewEnricher(logr.Discard())
	}

	Context("Completion notifications", func() {
		It("UT-NOT-553-001: shows workflow name instead of UUID in completion body", func() {
			e := newEnricher()

			body := fmt.Sprintf("Remediation Completed Successfully\n\n**Workflow Executed**: %s\n**Execution Engine**: job", testUUID)
			nr := buildTestNotification(body, map[string]string{
				"workflowId":   testUUID,
				"workflowName": "oom-recovery",
			})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("**Workflow Executed**: oom-recovery"))
			Expect(result.Spec.Body).NotTo(ContainSubstring(testUUID))
		})

		It("UT-NOT-553-007: metadata key workflowId takes priority over selectedWorkflow for completion notifications", func() {
			e := newEnricher()

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{
				"workflowId":       testUUID,
				"selectedWorkflow": "should-not-use-this",
				"workflowName":     "oom-recovery",
			})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("**Workflow Executed**: oom-recovery"))
			Expect(result.Spec.Body).NotTo(ContainSubstring(testUUID))
		})
	})

	Context("Approval notifications", func() {
		It("UT-NOT-553-002: shows workflow name instead of UUID in approval body", func() {
			e := newEnricher()

			body := fmt.Sprintf("**Proposed Workflow**: %s\n\n**Approval Reason**: LowConfidence", testUUID)
			nr := buildTestNotification(body, map[string]string{
				"selectedWorkflow": testUUID,
				"workflowName":     "crashloop-config-fix",
			})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("**Proposed Workflow**: crashloop-config-fix"))
			Expect(result.Spec.Body).NotTo(ContainSubstring(testUUID))
		})

		It("UT-NOT-553-003: substitutes the name inline alongside surrounding text", func() {
			e := newEnricher()

			body := fmt.Sprintf("**Proposed Workflow**: ScaleReplicas (%s)", testUUID)
			nr := buildTestNotification(body, map[string]string{
				"selectedWorkflow": testUUID,
				"workflowName":     "scale-replicas-v1",
			})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("**Proposed Workflow**: ScaleReplicas (scale-replicas-v1)"))
			Expect(result.Spec.Body).NotTo(ContainSubstring(testUUID))
		})

		It("UT-NOT-553-008: metadata key selectedWorkflow drives ID extraction when workflowId is absent", func() {
			e := newEnricher()

			body := fmt.Sprintf("**Proposed Workflow**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{
				"selectedWorkflow": testUUID,
				"workflowName":     "crashloop-config-fix",
			})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("crashloop-config-fix"))
			Expect(result.Spec.Body).NotTo(ContainSubstring(testUUID))
		})
	})

	Context("Graceful degradation", func() {
		It("UT-NOT-553-004: keeps the UUID in body when WorkflowName is not populated", func() {
			// Consolidates the old resolver-failure/empty-name/nil-resolver
			// specs: without a live resolver, every "no name available" case
			// (WorkflowName simply absent -- the only way this can happen now)
			// is indistinguishable, so a single spec covers it.
			e := newEnricher()

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{"workflowId": testUUID})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring(testUUID))
		})

		It("UT-NOT-553-006: no-op when no workflow metadata present", func() {
			e := newEnricher()

			body := "Remediation completed.\n\n**Duplicate Remediations**: 3"
			nr := buildTestNotification(body, map[string]string{"remediationRequest": "rr-abc"})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(Equal(body))
		})
	})

	Context("Safety guarantees", func() {
		It("UT-NOT-553-009: subject line is never modified by enrichment", func() {
			e := newEnricher()

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{
				"workflowId":   testUUID,
				"workflowName": "oom-recovery",
			})
			originalSubject := nr.Spec.Subject

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Subject).To(Equal(originalSubject))
		})

		It("UT-NOT-553-010: all metadata fields preserved after enrichment", func() {
			e := newEnricher()

			metadata := map[string]string{
				"workflowId":      testUUID,
				"executionEngine": "job",
				"rootCause":       "OOM",
				"workflowName":    "oom-recovery",
			}
			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, metadata)

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Context.Workflow.WorkflowID).To(Equal(testUUID))
			Expect(result.Spec.Context.Workflow.ExecutionEngine).To(Equal("job"))
			Expect(result.Spec.Context.Analysis.RootCause).To(Equal("OOM"))
		})

		It("UT-NOT-553-012: enrichment operates on a copy — original notification is not mutated", func() {
			e := newEnricher()

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{
				"workflowId":   testUUID,
				"workflowName": "oom-recovery",
			})
			originalBody := nr.Spec.Body

			result := e.EnrichNotification(context.Background(), nr)

			Expect(nr.Spec.Body).To(Equal(originalBody), "original notification must not be mutated")
			Expect(result).NotTo(BeIdenticalTo(nr), "returned notification must be a different object")
			Expect(result.Spec.Body).To(ContainSubstring("oom-recovery"))
		})
	})
})
