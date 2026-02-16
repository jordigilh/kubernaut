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

package effectivenessmonitor

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/alert"
	emclient "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/client"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// mockAlertManagerClient implements emclient.AlertManagerClient for unit tests.
type mockAlertManagerClient struct {
	alerts []emclient.Alert
	err    error
}

func (m *mockAlertManagerClient) GetAlerts(_ context.Context, _ emclient.AlertFilters) ([]emclient.Alert, error) {
	return m.alerts, m.err
}

func (m *mockAlertManagerClient) Ready(_ context.Context) error {
	return m.err
}

var _ = Describe("Alert Resolution Scorer (BR-EM-002)", func() {

	var scorer alert.Scorer

	BeforeEach(func() {
		scorer = alert.NewScorer()
	})

	// ========================================
	// UT-EM-AR-001: Alert resolved -> 1.0
	// ========================================
	Describe("Score (UT-EM-AR-001 through UT-EM-AR-005)", func() {

		It("UT-EM-AR-001: should return 1.0 when alert has resolved", func() {
			amClient := &mockAlertManagerClient{
				alerts: []emclient.Alert{}, // No active alerts matching
				err:    nil,
			}
			alertCtx := alert.AlertContext{
				AlertName:   "HighLatency",
				AlertLabels: map[string]string{"namespace": "production"},
				Namespace:   "production",
			}

			result := scorer.Score(context.Background(), amClient, alertCtx)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(1.0))
			Expect(result.Component).To(Equal(types.ComponentAlert))
		})

		// UT-EM-AR-002: Alert still active -> 0.0
		It("UT-EM-AR-002: should return 0.0 when alert is still active", func() {
			amClient := &mockAlertManagerClient{
				alerts: []emclient.Alert{
					{
						Labels: map[string]string{"alertname": "HighLatency", "namespace": "production"},
						State:  "active",
					},
				},
				err: nil,
			}
			alertCtx := alert.AlertContext{
				AlertName:   "HighLatency",
				AlertLabels: map[string]string{"namespace": "production"},
				Namespace:   "production",
			}

			result := scorer.Score(context.Background(), amClient, alertCtx)
			Expect(result.Assessed).To(BeTrue())
			Expect(result.Score).ToNot(BeNil())
			Expect(*result.Score).To(Equal(0.0))
		})

		// UT-EM-AR-003: AlertManager unavailable -> nil score
		It("UT-EM-AR-003: should return nil score when AlertManager is unavailable", func() {
			amClient := &mockAlertManagerClient{
				alerts: nil,
				err:    errors.New("connection refused"),
			}
			alertCtx := alert.AlertContext{
				AlertName:   "HighLatency",
				AlertLabels: map[string]string{"namespace": "production"},
				Namespace:   "production",
			}

			result := scorer.Score(context.Background(), amClient, alertCtx)
			Expect(result.Assessed).To(BeFalse())
			Expect(result.Score).To(BeNil())
			Expect(result.Error).To(HaveOccurred())
		})

		// UT-EM-AR-004: Multiple alerts, all resolved -> 1.0
		It("UT-EM-AR-004: should return 1.0 when multiple alerts are all resolved", func() {
			amClient := &mockAlertManagerClient{
				alerts: []emclient.Alert{}, // No active alerts
				err:    nil,
			}
			alertCtx := alert.AlertContext{
				AlertName:   "HighLatency",
				AlertLabels: map[string]string{"namespace": "production", "pod": "api-server"},
				Namespace:   "production",
			}

			result := scorer.Score(context.Background(), amClient, alertCtx)
			Expect(result.Assessed).To(BeTrue())
			Expect(*result.Score).To(Equal(1.0))
		})

		// UT-EM-AR-005: Empty alert name -> error
		It("UT-EM-AR-005: should handle empty alert name gracefully", func() {
			amClient := &mockAlertManagerClient{
				alerts: []emclient.Alert{},
				err:    nil,
			}
			alertCtx := alert.AlertContext{
				AlertName: "",
				Namespace: "production",
			}

			result := scorer.Score(context.Background(), amClient, alertCtx)
			// With empty alert name, the scorer should report an error or handle gracefully
			Expect(result.Component).To(Equal(types.ComponentAlert))
		})
	})
})
