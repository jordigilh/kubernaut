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

package authwebhook

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/authwebhook"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TDD RED Phase: Channel Extraction Unit Tests
// BR-AUTH-001: SOC2 CC8.1 Operator Attribution
// Test Plan: docs/testing/276/TEST_PLAN.md

var _ = Describe("BR-AUTH-001: Delivery Channel Extraction from DeliveryAttempts", func() {

	Describe("ExtractDeliveryChannels - sorted, deduplicated channel list", func() {

		It("UT-AW-276-001: produces sorted, deduplicated list from mixed deliveryAttempts", func() {
			attempts := []notificationv1.DeliveryAttempt{
				{Channel: notificationv1.DeliveryChannelName("slack"), Attempt: 1, Status: notificationv1.DeliveryAttemptStatusSuccess, Timestamp: metav1.Now()},
				{Channel: notificationv1.DeliveryChannelName("console"), Attempt: 1, Status: notificationv1.DeliveryAttemptStatusSuccess, Timestamp: metav1.Now()},
				{Channel: notificationv1.DeliveryChannelName("slack"), Attempt: 2, Status: notificationv1.DeliveryAttemptStatusFailed, Timestamp: metav1.Now()},
			}

			channels := authwebhook.ExtractDeliveryChannels(attempts)

			Expect(channels).To(HaveLen(2),
				"Should deduplicate: 2 unique channels from 3 attempts")
			Expect(channels).To(Equal([]string{"console", "slack"}),
				"Should be sorted alphabetically: console before slack")
		})

		It("UT-AW-276-002: handles nil deliveryAttempts without crash", func() {
			channels := authwebhook.ExtractDeliveryChannels(nil)

			Expect(channels).To(BeNil(),
				"Nil input should return nil (not empty slice)")
		})

		It("UT-AW-276-002b: handles empty deliveryAttempts without crash", func() {
			channels := authwebhook.ExtractDeliveryChannels([]notificationv1.DeliveryAttempt{})

			Expect(channels).To(BeNil(),
				"Empty input should return nil (no delivery occurred)")
		})

		It("UT-AW-276-003: deduplicates channels across multiple retries", func() {
			attempts := []notificationv1.DeliveryAttempt{
				{Channel: notificationv1.DeliveryChannelName("slack"), Attempt: 1, Status: notificationv1.DeliveryAttemptStatusFailed, Timestamp: metav1.Now()},
				{Channel: notificationv1.DeliveryChannelName("slack"), Attempt: 2, Status: notificationv1.DeliveryAttemptStatusFailed, Timestamp: metav1.Now()},
				{Channel: notificationv1.DeliveryChannelName("slack"), Attempt: 3, Status: notificationv1.DeliveryAttemptStatusFailed, Timestamp: metav1.Now()},
				{Channel: notificationv1.DeliveryChannelName("slack"), Attempt: 4, Status: notificationv1.DeliveryAttemptStatusFailed, Timestamp: metav1.Now()},
				{Channel: notificationv1.DeliveryChannelName("slack"), Attempt: 5, Status: notificationv1.DeliveryAttemptStatusSuccess, Timestamp: metav1.Now()},
			}

			channels := authwebhook.ExtractDeliveryChannels(attempts)

			Expect(channels).To(HaveLen(1),
				"5 attempts on same channel should produce 1 entry")
			Expect(channels).To(Equal([]string{"slack"}),
				"Should contain single 'slack' entry")
		})
	})
})
