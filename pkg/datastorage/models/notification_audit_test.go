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

package models_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

func TestModels(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Models Suite")
}

func validNotificationAudit() *models.NotificationAudit {
	return &models.NotificationAudit{
		RemediationID:  "rr-2026-001",
		NotificationID: "notif-001",
		Recipient:      "oncall@example.com",
		Channel:        "email",
		MessageSummary: "Alert triggered for pod restart",
		Status:         "sent",
		SentAt:         time.Now(),
	}
}

var _ = Describe("NotificationAudit.Validate (#1048 Phase 4 / SI-10)", func() {

	It("UT-DS-1048-NV-001: should accept a valid notification", func() {
		n := validNotificationAudit()
		Expect(n.Validate()).To(Succeed())
	})

	DescribeTable("UT-DS-1048-NV-002: should reject invalid notifications",
		func(mutate func(*models.NotificationAudit), expectedErr string) {
			n := validNotificationAudit()
			mutate(n)
			err := n.Validate()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(expectedErr))
		},
		Entry("empty RemediationID",
			func(n *models.NotificationAudit) { n.RemediationID = "" },
			"remediation_id is required"),
		Entry("empty NotificationID",
			func(n *models.NotificationAudit) { n.NotificationID = "" },
			"notification_id is required"),
		Entry("empty Recipient",
			func(n *models.NotificationAudit) { n.Recipient = "" },
			"recipient is required"),
		Entry("invalid Channel",
			func(n *models.NotificationAudit) { n.Channel = "telegram" },
			"channel must be one of"),
		Entry("empty Channel",
			func(n *models.NotificationAudit) { n.Channel = "" },
			"channel must be one of"),
		Entry("empty MessageSummary",
			func(n *models.NotificationAudit) { n.MessageSummary = "" },
			"message_summary is required"),
		Entry("invalid Status",
			func(n *models.NotificationAudit) { n.Status = "delivered" },
			"status must be one of"),
		Entry("empty Status",
			func(n *models.NotificationAudit) { n.Status = "" },
			"status must be one of"),
		Entry("zero SentAt",
			func(n *models.NotificationAudit) { n.SentAt = time.Time{} },
			"sent_at is required"),
	)

	DescribeTable("UT-DS-1048-NV-003: should accept all valid channels",
		func(channel string) {
			n := validNotificationAudit()
			n.Channel = channel
			Expect(n.Validate()).To(Succeed())
		},
		Entry("email", "email"),
		Entry("slack", "slack"),
		Entry("pagerduty", "pagerduty"),
		Entry("teams", "teams"),
		Entry("sms", "sms"),
	)

	DescribeTable("UT-DS-1048-NV-004: should accept all valid statuses",
		func(status string) {
			n := validNotificationAudit()
			n.Status = status
			Expect(n.Validate()).To(Succeed())
		},
		Entry("sent", "sent"),
		Entry("failed", "failed"),
		Entry("acknowledged", "acknowledged"),
		Entry("escalated", "escalated"),
	)
})
