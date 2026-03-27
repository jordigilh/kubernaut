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
	"sync"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/notification/enrichment"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	notificationstatus "github.com/jordigilh/kubernaut/pkg/notification/status"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

const itTestUUID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

// capturingService captures the last delivered notification body for assertions.
type capturingService struct {
	mu      sync.Mutex
	bodies  []string
	lastErr error
}

func (c *capturingService) Deliver(_ context.Context, notification *notificationv1alpha1.NotificationRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastErr != nil {
		return c.lastErr
	}
	c.bodies = append(c.bodies, notification.Spec.Body)
	return nil
}

func (c *capturingService) getBodies() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.bodies))
	copy(out, c.bodies)
	return out
}

func buildITNotification(body string, metadata map[string]string) *notificationv1alpha1.NotificationRequest {
	return &notificationv1alpha1.NotificationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nr-it-enrichment",
			Namespace: "default",
			UID:       types.UID("it-uid-001"),
		},
		Spec: notificationv1alpha1.NotificationRequestSpec{
			Subject:  "Remediation Completed: test-signal",
			Body:     body,
			Metadata: metadata,
		},
	}
}

// buildTestOrchestrator creates a standalone delivery orchestrator for integration testing.
func buildTestOrchestrator(channels map[string]delivery.Service, enrich *enrichment.Enricher) *delivery.Orchestrator {
	sanitizer := sanitization.NewSanitizer()
	metricsRecorder := notificationmetrics.NewNoOpRecorder()
	statusManager := notificationstatus.NewManager(nil, nil)

	orch := delivery.NewOrchestrator(sanitizer, metricsRecorder, statusManager, logr.Discard())
	for name, svc := range channels {
		orch.RegisterChannel(name, svc)
	}
	if enrich != nil {
		orch.SetEnricher(enrich)
	}
	return orch
}

// noopCallback helpers for DeliverToChannels
func itAlreadySucceeded(_ *notificationv1alpha1.NotificationRequest, _ string) bool { return false }
func itHasPermanentError(_ *notificationv1alpha1.NotificationRequest, _ string) bool { return false }
func itGetAttemptCount(_ *notificationv1alpha1.NotificationRequest, _ string) int    { return 0 }
func itAuditSent(_ context.Context, _ *notificationv1alpha1.NotificationRequest, _ string) error {
	return nil
}
func itAuditFailed(_ context.Context, _ *notificationv1alpha1.NotificationRequest, _ string, _ error) error {
	return nil
}

var _ = Describe("#553: Workflow Name Enrichment — Delivery Pipeline Integration", func() {

	Context("Full pipeline with enrichment", func() {
		It("IT-NOT-553-001: resolves UUID to workflow name and delivers enriched body", func() {
			resolver := &itMockResolver{name: "oom-recovery"}
			enrich := enrichment.NewEnricher(resolver, logr.Discard())

			capture := &capturingService{}
			orch := buildTestOrchestrator(map[string]delivery.Service{
				"console": capture,
			}, enrich)

			body := fmt.Sprintf("Remediation Completed\n\n**Workflow Executed**: %s\n**Execution Engine**: job\n**Outcome**: Remediated", itTestUUID)
			nr := buildITNotification(body, map[string]string{"workflowId": itTestUUID})

			policy := &notificationv1alpha1.RetryPolicy{MaxAttempts: 3}
			_, err := orch.DeliverToChannels(
				context.Background(), nr,
				[]notificationv1alpha1.Channel{"console"},
				policy,
				itAlreadySucceeded, itHasPermanentError, itGetAttemptCount,
				itAuditSent, itAuditFailed, nil,
			)
			Expect(err).NotTo(HaveOccurred())

			bodies := capture.getBodies()
			Expect(bodies).To(HaveLen(1))
			Expect(bodies[0]).To(ContainSubstring("**Workflow Executed**: oom-recovery"))
			Expect(bodies[0]).NotTo(ContainSubstring(itTestUUID))
		})

		It("IT-NOT-553-002: delivery succeeds with UUID when DataStorage is unavailable", func() {
			resolver := &itMockResolver{err: fmt.Errorf("connection refused")}
			enrich := enrichment.NewEnricher(resolver, logr.Discard())

			capture := &capturingService{}
			orch := buildTestOrchestrator(map[string]delivery.Service{
				"console": capture,
			}, enrich)

			body := fmt.Sprintf("**Workflow Executed**: %s", itTestUUID)
			nr := buildITNotification(body, map[string]string{"workflowId": itTestUUID})

			policy := &notificationv1alpha1.RetryPolicy{MaxAttempts: 3}
			result, err := orch.DeliverToChannels(
				context.Background(), nr,
				[]notificationv1alpha1.Channel{"console"},
				policy,
				itAlreadySucceeded, itHasPermanentError, itGetAttemptCount,
				itAuditSent, itAuditFailed, nil,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.FailureCount).To(Equal(0))

			bodies := capture.getBodies()
			Expect(bodies).To(HaveLen(1))
			Expect(bodies[0]).To(ContainSubstring(itTestUUID), "UUID should be preserved on resolver failure")
		})

		It("IT-NOT-553-003: all channels receive enriched body", func() {
			resolver := &itMockResolver{name: "crashloop-config-fix"}
			enrich := enrichment.NewEnricher(resolver, logr.Discard())

			captureConsole := &capturingService{}
			captureLog := &capturingService{}
			orch := buildTestOrchestrator(map[string]delivery.Service{
				"console": captureConsole,
				"log":     captureLog,
			}, enrich)

			body := fmt.Sprintf("**Workflow Executed**: %s", itTestUUID)
			nr := buildITNotification(body, map[string]string{"workflowId": itTestUUID})

			policy := &notificationv1alpha1.RetryPolicy{MaxAttempts: 3}
			_, err := orch.DeliverToChannels(
				context.Background(), nr,
				[]notificationv1alpha1.Channel{"console", "log"},
				policy,
				itAlreadySucceeded, itHasPermanentError, itGetAttemptCount,
				itAuditSent, itAuditFailed, nil,
			)
			Expect(err).NotTo(HaveOccurred())

			consoleBodies := captureConsole.getBodies()
			logBodies := captureLog.getBodies()

			Expect(consoleBodies).To(HaveLen(1))
			Expect(logBodies).To(HaveLen(1))
			Expect(consoleBodies[0]).To(ContainSubstring("crashloop-config-fix"))
			Expect(consoleBodies[0]).NotTo(ContainSubstring(itTestUUID))
			Expect(logBodies[0]).To(ContainSubstring("crashloop-config-fix"))
			Expect(logBodies[0]).NotTo(ContainSubstring(itTestUUID))
		})
	})
})

// itMockResolver is a simple mock for integration tests.
type itMockResolver struct {
	name string
	err  error
}

func (m *itMockResolver) ResolveWorkflowName(_ context.Context, _ string) (string, error) {
	return m.name, m.err
}
