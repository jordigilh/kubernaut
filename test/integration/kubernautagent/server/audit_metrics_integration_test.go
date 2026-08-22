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

package server_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	kametrics "github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/session"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// Extracted from the now-deleted wiring_test.go (issue #2190, DD-AA-KA-001):
// this test proves session.Manager's audit-metrics wiring directly against
// session.Manager/InstrumentedAuditStore -- it never touched the retired
// ogen HTTP surface those other tests exercised, so it survives the HTTP
// endpoint retirement unchanged.
var _ = Describe("BR-KA-OBSERVABILITY-001: Audit metrics wiring", func() {

	Describe("IT-KA-OBS-006: InstrumentedAuditStore increments audit_events_emitted_total", func() {
		It("records metric after session lifecycle generates audit events", func() {
			reg := prometheus.NewRegistry()
			m := kametrics.NewMetricsWithRegistry(reg)

			instrumentedStore := audit.NewInstrumentedAuditStore(
				audit.NopAuditStore{}, m.RecordAuditEventEmitted,
			)

			store := session.NewStore(5 * time.Minute)
			mgr := session.NewManager(store, logr.Discard(), instrumentedStore, m)

			id, err := mgr.StartInvestigation(context.Background(),
				func(_ context.Context) (*katypes.InvestigationResult, error) {
					return &katypes.InvestigationResult{RCASummary: "done"}, nil
				}, map[string]string{"remediation_id": "rr-obs-006"})
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() string {
				s, _ := mgr.GetSession(id)
				if s == nil {
					return ""
				}
				return string(s.Status)
			}, 5*time.Second).Should(Equal(string(session.StatusCompleted)))

			v := gatherCounterFromReg(reg, kametrics.MetricNameAuditEventsEmittedTotal, nil)
			Expect(v).To(BeNumerically(">=", 1),
				"audit_events_emitted_total should increment from session lifecycle audit events")
		})
	})
})

func gatherCounterFromReg(g prometheus.Gatherer, name string, labels map[string]string) float64 {
	families, err := g.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			matched := true
			found := make(map[string]string)
			for _, lp := range m.GetLabel() {
				found[lp.GetName()] = lp.GetValue()
			}
			for k, v := range labels {
				if found[k] != v {
					matched = false
					break
				}
			}
			if matched {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}
