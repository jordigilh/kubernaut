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
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	pkgaudit "github.com/jordigilh/kubernaut/pkg/audit"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/audit"
	emtypes "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/types"
)

// EM audit manager paths (BR-EM-009.4, DD-AUDIT-003) — RecordComponentAssessed, RecordHashComputed, RecordAssessmentScheduled.
var _ = Describe("EffectivenessMonitor audit manager coverage 668 (BR-EM-009.4)", func() {
	var (
		spy *spyAuditStore
		mgr *audit.Manager
		ea  *eav1.EffectivenessAssessment
		ctx context.Context
	)

	BeforeEach(func() {
		spy = &spyAuditStore{}
		mgr = audit.NewManager(spy, logr.Discard())
		ea = newTestEA()
		ea.Spec.Config = eav1.EAConfig{
			StabilizationWindow: metav1.Duration{Duration: 2 * time.Minute},
			HashComputeDelay:    &metav1.Duration{Duration: 30 * time.Second},
			AlertCheckDelay:     &metav1.Duration{Duration: 15 * time.Second},
		}
		ea.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC))
		t1 := metav1.NewTime(time.Date(2026, 4, 1, 13, 0, 0, 0, time.UTC))
		t2 := metav1.NewTime(time.Date(2026, 4, 1, 13, 30, 0, 0, time.UTC))
		t3 := metav1.NewTime(time.Date(2026, 4, 1, 14, 0, 0, 0, time.UTC))
		ea.Status = eav1.EffectivenessAssessmentStatus{
			ValidityDeadline:       &t1,
			PrometheusCheckAfter:   &t2,
			AlertManagerCheckAfter: &t3,
		}
		ctx = context.Background()
	})

	It("RecordComponentAssessed stores health component audit with assessed flag", func() {
		score := 0.75
		res := emtypes.ComponentResult{
			Component: emtypes.ComponentHealth,
			Assessed:  true,
			Score:     &score,
			Details:   "pods ready",
		}
		Expect(mgr.RecordComponentAssessed(ctx, ea, "health", res)).To(Succeed())
		Expect(spy.lastEvent.EventType).To(Equal(string(emtypes.AuditHealthAssessed)))
		payload := extractPayload(spy.lastEvent)
		Expect(payload.Component).To(Equal(ogenclient.EffectivenessAssessmentAuditPayloadComponentHealth))
		Expect(payload.Assessed.Value).To(BeTrue())
		Expect(payload.Score.Value).To(BeNumerically("~", 0.75, 0.001))
	})

	It("RecordComponentAssessed returns error for unknown component name", func() {
		res := emtypes.ComponentResult{Component: emtypes.ComponentHealth, Assessed: true}
		err := mgr.RecordComponentAssessed(ctx, ea, "unknown-component", res)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown component type"))
	})

	It("RecordHashComputed persists hash fields and failure outcome when result has error", func() {
		match := true
		res := emtypes.ComponentResult{
			Component: emtypes.ComponentHash,
			Assessed:  false,
			Details:   "hash step",
			Error:     errors.New("hash failed"),
		}
		hashData := audit.HashComputedData{
			PostHash: "sha256:aaa",
			PreHash:  "sha256:bbb",
			Match:    &match,
		}
		Expect(mgr.RecordHashComputed(ctx, ea, res, hashData)).To(Succeed())
		Expect(spy.lastEvent.EventType).To(Equal(string(emtypes.AuditHashComputed)))
		payload := extractPayload(spy.lastEvent)
		Expect(payload.PostRemediationSpecHash.Value).To(Equal("sha256:aaa"))
		Expect(payload.PreRemediationSpecHash.Value).To(Equal("sha256:bbb"))
		Expect(payload.HashMatch.Value).To(BeTrue())
		Expect(spy.lastEvent.EventOutcome).To(Equal(pkgaudit.OutcomeFailure))
	})

	It("RecordAssessmentScheduled writes scheduling payload with validity window string (BR-EM-009.4)", func() {
		vw := 45 * time.Minute
		Expect(mgr.RecordAssessmentScheduled(ctx, ea, vw)).To(Succeed())
		Expect(spy.lastEvent.EventType).To(Equal(string(emtypes.AuditAssessmentScheduled)))
		payload := extractPayload(spy.lastEvent)
		Expect(payload.Component).To(Equal(ogenclient.EffectivenessAssessmentAuditPayloadComponentScheduled))
		Expect(payload.ValidityWindow.Value).To(Equal(vw.String()))
		Expect(payload.StabilizationWindow.Value).To(Equal((2 * time.Minute).String()))
		Expect(payload.HashComputeDelay.Value).To(Equal((30 * time.Second).String()))
		Expect(payload.AlertCheckDelay.Value).To(Equal((15 * time.Second).String()))
	})
})
