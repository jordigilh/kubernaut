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

package notification

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

// mockWorkflowNameResolver implements enrichment.WorkflowNameResolver for unit tests.
type mockWorkflowNameResolver struct {
	name      string
	err       error
	callCount int
}

func (m *mockWorkflowNameResolver) ResolveWorkflowName(_ context.Context, _ string) (string, error) {
	m.callCount++
	return m.name, m.err
}

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

var _ = Describe("#553: Workflow Name Enrichment", func() {

	Context("Completion notifications", func() {
		It("UT-NOT-553-001: shows workflow name instead of UUID in completion body", func() {
			resolver := &mockWorkflowNameResolver{name: "oom-recovery"}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := fmt.Sprintf("Remediation Completed Successfully\n\n**Workflow Executed**: %s\n**Execution Engine**: job", testUUID)
			nr := buildTestNotification(body, map[string]string{"workflowId": testUUID})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("**Workflow Executed**: oom-recovery"))
			Expect(result.Spec.Body).NotTo(ContainSubstring(testUUID))
		})

		It("UT-NOT-553-007: metadata key workflowId drives resolution for completion notifications", func() {
			resolver := &mockWorkflowNameResolver{name: "oom-recovery"}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{
				"workflowId":      testUUID,
				"selectedWorkflow": "should-not-use-this",
			})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("**Workflow Executed**: oom-recovery"))
			Expect(result.Spec.Body).NotTo(ContainSubstring(testUUID))
		})
	})

	Context("Approval notifications", func() {
		It("UT-NOT-553-002: shows workflow name instead of UUID in approval body", func() {
			resolver := &mockWorkflowNameResolver{name: "crashloop-config-fix"}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := fmt.Sprintf("**Proposed Workflow**: %s\n\n**Approval Reason**: LowConfidence", testUUID)
			nr := buildTestNotification(body, map[string]string{"selectedWorkflow": testUUID})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("**Proposed Workflow**: crashloop-config-fix"))
			Expect(result.Spec.Body).NotTo(ContainSubstring(testUUID))
		})

		It("UT-NOT-553-003: preserves ActionType (name) format when ActionType is present", func() {
			resolver := &mockWorkflowNameResolver{name: "scale-replicas-v1"}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := fmt.Sprintf("**Proposed Workflow**: ScaleReplicas (%s)", testUUID)
			nr := buildTestNotification(body, map[string]string{"selectedWorkflow": testUUID})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("**Proposed Workflow**: ScaleReplicas (scale-replicas-v1)"))
			Expect(result.Spec.Body).NotTo(ContainSubstring(testUUID))
		})

		It("UT-NOT-553-008: metadata key selectedWorkflow drives resolution when workflowId absent", func() {
			resolver := &mockWorkflowNameResolver{name: "crashloop-config-fix"}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := fmt.Sprintf("**Proposed Workflow**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{"selectedWorkflow": testUUID})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring("crashloop-config-fix"))
			Expect(resolver.callCount).To(Equal(1))
		})
	})

	Context("Graceful degradation", func() {
		It("UT-NOT-553-004: resolver failure keeps UUID in body", func() {
			resolver := &mockWorkflowNameResolver{err: fmt.Errorf("connection refused")}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{"workflowId": testUUID})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring(testUUID))
		})

		It("UT-NOT-553-005: empty resolved name (workflow not found) preserves UUID", func() {
			resolver := &mockWorkflowNameResolver{name: ""}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{"workflowId": testUUID})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring(testUUID))
		})

		It("UT-NOT-553-006: no-op when no workflow metadata present", func() {
			resolver := &mockWorkflowNameResolver{name: "should-not-be-called"}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := "Remediation completed.\n\n**Duplicate Remediations**: 3"
			nr := buildTestNotification(body, map[string]string{"remediationRequest": "rr-abc"})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(Equal(body))
			Expect(resolver.callCount).To(Equal(0))
		})

		It("UT-NOT-553-011: nil resolver results in no-op enrichment", func() {
			e := enrichment.NewEnricher(nil, logr.Discard())

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{"workflowId": testUUID})

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Body).To(ContainSubstring(testUUID))
		})
	})

	Context("Safety guarantees", func() {
		It("UT-NOT-553-009: subject line is never modified by enrichment", func() {
			resolver := &mockWorkflowNameResolver{name: "oom-recovery"}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{"workflowId": testUUID})
			originalSubject := nr.Spec.Subject

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Subject).To(Equal(originalSubject))
		})

		It("UT-NOT-553-010: all metadata fields preserved after enrichment", func() {
			resolver := &mockWorkflowNameResolver{name: "oom-recovery"}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			metadata := map[string]string{
				"workflowId":      testUUID,
				"executionEngine": "job",
				"rootCause":       "OOM",
			}
			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, metadata)

			result := e.EnrichNotification(context.Background(), nr)

			Expect(result.Spec.Context.Workflow.WorkflowID).To(Equal(testUUID))
			Expect(result.Spec.Context.Workflow.ExecutionEngine).To(Equal("job"))
			Expect(result.Spec.Context.Analysis.RootCause).To(Equal("OOM"))
		})

		It("UT-NOT-553-012: enrichment operates on a copy — original notification is not mutated", func() {
			resolver := &mockWorkflowNameResolver{name: "oom-recovery"}
			e := enrichment.NewEnricher(resolver, logr.Discard())

			body := fmt.Sprintf("**Workflow Executed**: %s", testUUID)
			nr := buildTestNotification(body, map[string]string{"workflowId": testUUID})
			originalBody := nr.Spec.Body

			result := e.EnrichNotification(context.Background(), nr)

			Expect(nr.Spec.Body).To(Equal(originalBody), "original notification must not be mutated")
			Expect(result).NotTo(BeIdenticalTo(nr), "returned notification must be a different object")
			Expect(result.Spec.Body).To(ContainSubstring("oom-recovery"))
		})
	})
})
