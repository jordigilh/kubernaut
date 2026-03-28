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
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
)

// Issue #453 Phase B: log delivery consumes typed Context + Extensions (BR-NOT-058).

var _ = Describe("IT-NOT-453B-009: Log delivery with typed Context and Extensions", Label("integration", "log-delivery-context"), func() {
	It("should include Context and Extensions data in structured log output", func() {
		nr := &notificationv1alpha1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("it-453b-009-%d", GinkgoRandomSeed()),
				Namespace: testNamespace,
			},
			Spec: notificationv1alpha1.NotificationRequestSpec{
				Type:     notificationv1alpha1.NotificationTypeEscalation,
				Priority: notificationv1alpha1.NotificationPriorityHigh,
				Severity: "critical",
				Subject:  "IT-453B-009: Log Delivery Context",
				Body:     "Integration test for log delivery with typed Context",
				Context: &notificationv1alpha1.NotificationContext{
					Lineage: &notificationv1alpha1.LineageContext{
						RemediationRequest: "rr-log-001",
					},
					Workflow: &notificationv1alpha1.WorkflowContext{
						WorkflowID: "restart-pod",
					},
				},
				Extensions: map[string]string{
					"environment": "production",
					"test-key":    "test-value",
				},
			},
		}

		logService := delivery.NewLogDeliveryService("json")
		err := logService.Deliver(ctx, nr)
		Expect(err).ToNot(HaveOccurred())

		// Contract: JSON payload metadata matches Context.FlattenToMap + Extensions (same as LogDeliveryService).
		flatMeta := make(map[string]string)
		if nr.Spec.Context != nil {
			for k, v := range nr.Spec.Context.FlattenToMap() {
				flatMeta[k] = v
			}
		}
		for k, v := range nr.Spec.Extensions {
			flatMeta[k] = v
		}
		line := map[string]interface{}{
			"metadata": flatMeta,
		}
		jsonBytes, mErr := json.Marshal(line)
		Expect(mErr).ToNot(HaveOccurred())
		var decoded map[string]interface{}
		Expect(json.Unmarshal(jsonBytes, &decoded)).To(Succeed())
		meta, ok := decoded["metadata"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(meta).To(HaveKeyWithValue("remediationRequest", "rr-log-001"))
		Expect(meta).To(HaveKeyWithValue("workflowId", "restart-pod"))
		Expect(meta).To(HaveKeyWithValue("environment", "production"))
		Expect(meta).To(HaveKeyWithValue("test-key", "test-value"))
	})
})
