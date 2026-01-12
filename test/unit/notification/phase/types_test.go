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

package phase

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	notificationphase "github.com/jordigilh/kubernaut/pkg/notification/phase"
)

var _ = Describe("Notification Phase State Machine", func() {
	Context("Terminal State Detection", func() {
		It("should identify Sent as terminal", func() {
			Expect(notificationphase.IsTerminal(notificationphase.Sent)).To(BeTrue())
		})

		It("should identify PartiallySent as terminal", func() {
			Expect(notificationphase.IsTerminal(notificationphase.PartiallySent)).To(BeTrue())
		})

		It("should identify Failed as terminal", func() {
			Expect(notificationphase.IsTerminal(notificationphase.Failed)).To(BeTrue())
		})

		It("should identify Pending as non-terminal", func() {
			Expect(notificationphase.IsTerminal(notificationphase.Pending)).To(BeFalse())
		})

		It("should identify Sending as non-terminal", func() {
			Expect(notificationphase.IsTerminal(notificationphase.Sending)).To(BeFalse())
		})
	})

	Context("GetTerminalPhases", func() {
		It("should return all terminal phases", func() {
			terminalPhases := notificationphase.GetTerminalPhases()
			Expect(terminalPhases).To(HaveLen(3))
			Expect(terminalPhases).To(ContainElements(
				notificationphase.Sent,
				notificationphase.PartiallySent,
				notificationphase.Failed,
			))
		})
	})

	Context("Phase Transitions", func() {
		It("should allow Pending → Sending", func() {
			Expect(notificationphase.CanTransition(
				notificationphase.Pending,
				notificationphase.Sending,
			)).To(BeTrue())
		})

		It("should allow Pending → Failed", func() {
			Expect(notificationphase.CanTransition(
				notificationphase.Pending,
				notificationphase.Failed,
			)).To(BeTrue())
		})

		It("should allow Sending → Sent", func() {
			Expect(notificationphase.CanTransition(
				notificationphase.Sending,
				notificationphase.Sent,
			)).To(BeTrue())
		})

		It("should allow Sending → PartiallySent", func() {
			Expect(notificationphase.CanTransition(
				notificationphase.Sending,
				notificationphase.PartiallySent,
			)).To(BeTrue())
		})

		It("should allow Sending → Failed", func() {
			Expect(notificationphase.CanTransition(
				notificationphase.Sending,
				notificationphase.Failed,
			)).To(BeTrue())
		})

		It("should reject Pending → Sent (skipping Sending)", func() {
			Expect(notificationphase.CanTransition(
				notificationphase.Pending,
				notificationphase.Sent,
			)).To(BeFalse())
		})

		It("should reject Sent → Sending (terminal state)", func() {
			Expect(notificationphase.CanTransition(
				notificationphase.Sent,
				notificationphase.Sending,
			)).To(BeFalse())
		})

		It("should reject PartiallySent → Sending (terminal state)", func() {
			Expect(notificationphase.CanTransition(
				notificationphase.PartiallySent,
				notificationphase.Sending,
			)).To(BeFalse())
		})

		It("should reject Failed → Sending (terminal state)", func() {
			Expect(notificationphase.CanTransition(
				notificationphase.Failed,
				notificationphase.Sending,
			)).To(BeFalse())
		})
	})

	Context("Phase Validation", func() {
		It("should validate Pending", func() {
			Expect(notificationphase.Validate(notificationphase.Pending)).To(Succeed())
		})

		It("should validate Sending", func() {
			Expect(notificationphase.Validate(notificationphase.Sending)).To(Succeed())
		})

		It("should validate Sent", func() {
			Expect(notificationphase.Validate(notificationphase.Sent)).To(Succeed())
		})

		It("should validate PartiallySent", func() {
			Expect(notificationphase.Validate(notificationphase.PartiallySent)).To(Succeed())
		})

		It("should validate Failed", func() {
			Expect(notificationphase.Validate(notificationphase.Failed)).To(Succeed())
		})

		It("should reject invalid phase", func() {
			err := notificationphase.Validate(notificationphase.Phase("InvalidPhase"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid phase"))
		})
	})
})
