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

package infrastructure

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AlertManager alert cleanup [BR-EM-012]", func() {
	It("UT-EM-012-SCOPED-CLEANUP-001: resolves only alerts matching the owned label set", func() {
		targetLabels := map[string]string{
			"alertname": "MemoryExceedsLimit",
			"namespace": "owned-namespace",
			"pod":       "owned-pod",
			"severity":  "critical",
		}
		activeAlerts := []AMAlert{
			{
				Labels:   targetLabels,
				StartsAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
			},
			{
				Labels: map[string]string{
					"alertname": "MemoryExceedsLimit",
					"namespace": "owned-namespace",
					"pod":       "another-pod",
					"severity":  "critical",
				},
				StartsAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
			},
		}
		mock := NewMockAlertManager(MockAlertManagerConfig{AlertsResponse: activeAlerts})
		DeferCleanup(mock.Close)

		Expect(ResolveAlertsByLabels(mock.URL(), targetLabels)).To(Succeed())

		posted := mock.GetPostedAlerts()
		Expect(posted).To(HaveLen(1))
		Expect(posted[0].Labels).To(Equal(targetLabels))
		Expect(posted[0].EndsAt).NotTo(BeEmpty())
	})
})
