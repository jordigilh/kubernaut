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

package remediationorchestrator

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	emconditions "github.com/jordigilh/kubernaut/pkg/effectivenessmonitor/conditions"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/remediationrequest"
)

func float64Ptr(f float64) *float64 { return &f }

var _ = Describe("Verification Summary Builder (#318)", func() {

	Context("UT-RO-318-001: Full verification summary", func() {
		It("should return passed outcome with affirmative message when AssessmentReason is full", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
					Components: eav1.EAComponents{
						HealthAssessed:  true,
						HealthScore:     float64Ptr(1.0),
						AlertAssessed:   true,
						AlertScore:      float64Ptr(1.0),
						MetricsAssessed: true,
						MetricsScore:    float64Ptr(1.0),
						HashComputed:    true,
					},
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)
			Expect(summary).To(ContainSubstring("Verification passed"))
			Expect(vCtx.Assessed).To(BeTrue())
			Expect(vCtx.Outcome).To(Equal("passed"))
			Expect(vCtx.Reason).To(Equal("full"))
		})
	})

	Context("UT-RO-318-002: Spec drift warning", func() {
		It("should return inconclusive outcome with drift message when AssessmentReason is spec_drift", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonSpecDrift,
					Components: eav1.EAComponents{
						HashComputed:            true,
						PostRemediationSpecHash: "abc123",
						CurrentSpecHash:         "def456",
					},
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)
			Expect(summary).To(ContainSubstring("modified by an external entity"))
			Expect(vCtx.Assessed).To(BeTrue())
			Expect(vCtx.Outcome).To(Equal("inconclusive"))
			Expect(vCtx.Reason).To(Equal("spec_drift"))

			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).To(ContainSubstring("Resource integrity: spec modified externally after remediation"))
		})
	})

	Context("UT-RO-318-003: Partial verification with selective bullets", func() {
		It("should show only non-passing assessed components", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonPartial,
					Components: eav1.EAComponents{
						HealthAssessed:  true,
						HealthScore:     float64Ptr(1.0),
						AlertAssessed:   true,
						AlertScore:      float64Ptr(0.0),
						MetricsAssessed: false,
					},
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)
			Expect(summary).To(ContainSubstring("some checks could not be performed"))
			Expect(vCtx.Outcome).To(Equal("partial"))

			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).To(ContainSubstring("Related alerts: still firing"))
			Expect(bullets).NotTo(ContainSubstring("Pod health"))
			Expect(bullets).NotTo(ContainSubstring("Metrics"))
		})
	})

	Context("UT-RO-318-004: Alert decay timeout", func() {
		It("should return inconclusive outcome for alert_decay_timeout", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonAlertDecayTimeout,
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)
			Expect(summary).To(ContainSubstring("related alerts persisted beyond the assessment window"))
			Expect(vCtx.Outcome).To(Equal("inconclusive"))
			Expect(vCtx.Reason).To(Equal("alert_decay_timeout"))
		})
	})

	Context("UT-RO-318-005: Metrics timed out", func() {
		It("should return partial outcome for metrics_timed_out", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonMetricsTimedOut,
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)
			Expect(summary).To(ContainSubstring("metrics were not available"))
			Expect(vCtx.Outcome).To(Equal("partial"))
			Expect(vCtx.Reason).To(Equal("metrics_timed_out"))
		})
	})

	Context("UT-RO-318-006: Expired assessment", func() {
		It("should return unavailable outcome for expired", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonExpired,
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)
			Expect(summary).To(ContainSubstring("assessment window expired"))
			Expect(vCtx.Outcome).To(Equal("unavailable"))
			Expect(vCtx.Reason).To(Equal("expired"))
		})
	})

	Context("UT-RO-318-007: No execution", func() {
		It("should return unavailable outcome for no_execution", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonNoExecution,
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)
			Expect(summary).To(ContainSubstring("no workflow execution was found"))
			Expect(vCtx.Outcome).To(Equal("unavailable"))
			Expect(vCtx.Reason).To(Equal("no_execution"))
		})
	})

	Context("UT-RO-318-008: Nil EA graceful degradation", func() {
		It("should return not available message when EA is nil", func() {
			summary, vCtx := creator.BuildVerificationSummary(nil, nil)
			Expect(summary).To(Equal("Verification: not available."))
			Expect(vCtx.Assessed).To(BeFalse())
			Expect(vCtx.Outcome).To(Equal("unavailable"))
			Expect(vCtx.Reason).To(BeEmpty())
		})
	})

	Context("UT-RO-318-009: Node resource omits unassessed component bullets", func() {
		It("should only produce bullets for assessed non-passing components", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonPartial,
					Components: eav1.EAComponents{
						HealthAssessed:  true,
						HealthScore:     float64Ptr(0.0),
						MetricsAssessed: false,
						AlertAssessed:   false,
						HashComputed:    false,
					},
				},
			}
			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).To(ContainSubstring("Pod health: not recovered"))
			Expect(bullets).NotTo(ContainSubstring("Metrics"))
			Expect(bullets).NotTo(ContainSubstring("alerts"))
			Expect(bullets).NotTo(ContainSubstring("Resource integrity"))
		})
	})

	Context("UT-RO-318-010: Full verification has no component bullets", func() {
		It("should return empty string when all components pass", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
					Components: eav1.EAComponents{
						HealthAssessed:          true,
						HealthScore:             float64Ptr(1.0),
						AlertAssessed:           true,
						AlertScore:              float64Ptr(1.0),
						MetricsAssessed:         true,
						MetricsScore:            float64Ptr(1.0),
						HashComputed:            true,
						PostRemediationSpecHash: "sha256:abc123",
						CurrentSpecHash:         "sha256:abc123",
					},
				},
			}
			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).To(BeEmpty())
		})
	})

	// ========================================================================
	// Issue #546: Hash-capture degradation in verification summary
	// ========================================================================

	Context("UT-RO-546-001: RR condition set to False when pre-hash degraded", func() {
		It("should set PreRemediationHashCaptured=False with degradation reason", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-001", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, false,
				"failed to fetch target resource Certificate/demo-app-cert: Forbidden", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionPreRemediationHashCaptured)
			Expect(cond).ToNot(BeNil(), "PreRemediationHashCaptured condition should exist")
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("HashCaptureFailed"))
			Expect(cond.Message).To(ContainSubstring("Forbidden"))
			Expect(cond.LastTransitionTime.IsZero()).To(BeFalse(), "LastTransitionTime should be set")
		})
	})

	Context("UT-RO-546-002: RR condition set to True when pre-hash succeeds", func() {
		It("should set PreRemediationHashCaptured=True on success", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-002", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, true,
				"Pre-remediation hash captured for Certificate/demo-app-cert", nil)

			cond := remediationrequest.GetCondition(rr, remediationrequest.ConditionPreRemediationHashCaptured)
			Expect(cond).ToNot(BeNil(), "PreRemediationHashCaptured condition should exist")
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("HashCaptured"))
		})
	})

	Context("UT-RO-546-003: Completion body includes pre-hash degradation warning", func() {
		It("should include degradation warning when RR has PreRemediationHashCaptured=False", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-003", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, false,
				"failed to fetch target resource Certificate/demo-app-cert: Forbidden", nil)

			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
				},
			}

			summary, vCtx := creator.BuildVerificationSummary(ea, rr)
			Expect(summary).To(ContainSubstring("Effectiveness Assessment: Degraded"))
			Expect(summary).To(ContainSubstring("Pre-remediation hash"))
			Expect(summary).To(ContainSubstring("Forbidden"))
			Expect(summary).To(ContainSubstring("view"))
			Expect(vCtx.Degraded).To(BeTrue())
			Expect(vCtx.DegradedReason).To(ContainSubstring("Pre-remediation hash"))
		})
	})

	Context("UT-RO-546-004: Completion body includes post-hash degradation warning", func() {
		It("should include degradation warning when EA has PostHashCaptured=False", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-004", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, true, "Hash captured OK", nil)

			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
				},
			}
			emconditions.SetCondition(ea, emconditions.ConditionPostHashCaptured,
				metav1.ConditionFalse, emconditions.ReasonPostHashCaptureFailed,
				"failed to fetch target resource Deployment/nginx: Forbidden")

			summary, vCtx := creator.BuildVerificationSummary(ea, rr)
			Expect(summary).To(ContainSubstring("Effectiveness Assessment: Degraded"))
			Expect(summary).To(ContainSubstring("Post-remediation hash"))
			Expect(summary).To(ContainSubstring("Forbidden"))
			Expect(vCtx.Degraded).To(BeTrue())
			Expect(vCtx.DegradedReason).To(ContainSubstring("Post-remediation hash"))
		})
	})

	Context("UT-RO-546-005: Completion body shows both degradation warnings", func() {
		It("should include both pre- and post-hash degradation warnings", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-005", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, false,
				"failed to fetch Certificate/cert: Forbidden", nil)

			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
				},
			}
			emconditions.SetCondition(ea, emconditions.ConditionPostHashCaptured,
				metav1.ConditionFalse, emconditions.ReasonPostHashCaptureFailed,
				"failed to fetch Deployment/nginx: Forbidden")

			summary, vCtx := creator.BuildVerificationSummary(ea, rr)
			Expect(summary).To(ContainSubstring("Pre-remediation hash"))
			Expect(summary).To(ContainSubstring("Post-remediation hash"))
			Expect(vCtx.Degraded).To(BeTrue())
			Expect(vCtx.DegradedReason).To(ContainSubstring("Pre-remediation hash"))
			Expect(vCtx.DegradedReason).To(ContainSubstring("Post-remediation hash"))
		})
	})

	Context("UT-RO-546-006: No degradation section when hashes are OK", func() {
		It("should not include any degradation text when both hashes captured successfully", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-006", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, true, "Hash captured OK", nil)

			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
				},
			}
			emconditions.SetCondition(ea, emconditions.ConditionPostHashCaptured,
				metav1.ConditionTrue, emconditions.ReasonPostHashCaptured, "Hash captured")

			summary, vCtx := creator.BuildVerificationSummary(ea, rr)
			Expect(summary).NotTo(ContainSubstring("Degraded"))
			Expect(summary).NotTo(ContainSubstring("degradation"))
			Expect(vCtx.Degraded).To(BeFalse())
			Expect(vCtx.DegradedReason).To(BeEmpty())
			Expect(vCtx.Assessed).To(BeTrue())
			Expect(vCtx.Outcome).To(Equal("passed"))
		})
	})

	Context("UT-RO-546-007: VerificationContext.Degraded=true for pre-hash failure", func() {
		It("should populate Degraded and DegradedReason from RR condition", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-007", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, false,
				"failed to fetch Certificate/cert: Forbidden", nil)

			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonPartial,
				},
			}

			_, vCtx := creator.BuildVerificationSummary(ea, rr)
			Expect(vCtx.Degraded).To(BeTrue())
			Expect(vCtx.DegradedReason).To(ContainSubstring("Pre-remediation hash"))
			Expect(vCtx.DegradedReason).To(ContainSubstring("Forbidden"))
			Expect(vCtx.Assessed).To(BeTrue())
			Expect(vCtx.Outcome).To(Equal("partial"))
		})
	})

	Context("UT-RO-546-008: VerificationContext.Degraded=true for post-hash failure", func() {
		It("should populate Degraded and DegradedReason from EA condition", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-008", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, true, "OK", nil)

			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
				},
			}
			emconditions.SetCondition(ea, emconditions.ConditionPostHashCaptured,
				metav1.ConditionFalse, emconditions.ReasonPostHashCaptureFailed,
				"failed to fetch Deployment/nginx: context deadline exceeded")

			_, vCtx := creator.BuildVerificationSummary(ea, rr)
			Expect(vCtx.Degraded).To(BeTrue())
			Expect(vCtx.DegradedReason).To(ContainSubstring("Post-remediation hash"))
			Expect(vCtx.DegradedReason).To(ContainSubstring("context deadline exceeded"))
		})
	})

	Context("UT-RO-546-009: VerificationContext.Degraded=false when hashes OK", func() {
		It("should not set Degraded when both conditions are True", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-009", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, true, "OK", nil)

			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
				},
			}
			emconditions.SetCondition(ea, emconditions.ConditionPostHashCaptured,
				metav1.ConditionTrue, emconditions.ReasonPostHashCaptured, "OK")

			_, vCtx := creator.BuildVerificationSummary(ea, rr)
			Expect(vCtx.Degraded).To(BeFalse())
			Expect(vCtx.DegradedReason).To(BeEmpty())
		})
	})

	Context("UT-RO-546-010: FlattenToMap includes degradation keys", func() {
		It("should include verificationDegraded and verificationDegradedReason in flat map", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "rr-546-010", Namespace: "default"},
			}
			remediationrequest.SetPreRemediationHashCaptured(rr, false, "Forbidden", nil)

			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
				},
			}

			_, vCtx := creator.BuildVerificationSummary(ea, rr)
			ctx := &notificationv1.NotificationContext{Verification: vCtx}
			flat := ctx.FlattenToMap()

			Expect(flat["verificationDegraded"]).To(Equal("true"))
			Expect(flat["verificationDegradedReason"]).To(ContainSubstring("Pre-remediation hash"))
		})
	})

	// ========================================================================
	// Issue #596: Context-aware verification summary
	// ========================================================================

	Context("UT-RO-596-001: Full assessment with failing components produces qualified message", func() {
		It("should return completed outcome with qualified message when reason=full but components fail", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
					Components: eav1.EAComponents{
						HealthAssessed:  true,
						HealthScore:     float64Ptr(1.0),
						AlertAssessed:   true,
						AlertScore:      float64Ptr(0.0),
						MetricsAssessed: true,
						MetricsScore:    float64Ptr(0.5),
						HashComputed:    true,
					},
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)

			Expect(summary).To(ContainSubstring("all checks were performed, but some indicate the remediation was not fully effective"))
			Expect(summary).NotTo(ContainSubstring("Verification passed"))
			Expect(summary).To(ContainSubstring("Related alerts: still firing"))
			Expect(summary).To(ContainSubstring("Metrics: partial improvement (score: 0.50)"))
			Expect(vCtx.Outcome).To(Equal("completed"))
			Expect(vCtx.Reason).To(Equal("full"))
			Expect(vCtx.Summary).To(ContainSubstring("all checks were performed"))
			Expect(vCtx.Summary).NotTo(ContainSubstring("Verification passed"))
		})
	})

	Context("UT-RO-596-003: Disabled components (nil scores) with full assessment produce passed", func() {
		It("should return passed outcome when components are assessed but scores are nil (disabled)", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
					Components: eav1.EAComponents{
						HealthAssessed:          true,
						HealthScore:             float64Ptr(1.0),
						AlertAssessed:           true,
						MetricsAssessed:         true,
						HashComputed:            true,
						PostRemediationSpecHash: "sha256:abc",
						CurrentSpecHash:         "sha256:abc",
					},
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)

			Expect(summary).To(ContainSubstring("Verification passed"))
			Expect(vCtx.Outcome).To(Equal("passed"))
			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).To(BeEmpty())
		})
	})

	Context("UT-RO-596-004: Exact crashloop demo reproduction", func() {
		It("should produce qualified message with alert and metrics bullets for crashloop scenario", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Phase:            eav1.PhaseCompleted,
					AssessmentReason: eav1.AssessmentReasonFull,
					Components: eav1.EAComponents{
						HealthAssessed:          true,
						HealthScore:             float64Ptr(1.0),
						AlertAssessed:           true,
						AlertScore:              float64Ptr(0.0),
						MetricsAssessed:         true,
						MetricsScore:            float64Ptr(0.0),
						HashComputed:            true,
						PostRemediationSpecHash: "sha256:abc",
						CurrentSpecHash:         "sha256:abc",
					},
				},
			}
			summary, vCtx := creator.BuildVerificationSummary(ea, nil)

			Expect(summary).To(ContainSubstring("all checks were performed, but some indicate the remediation was not fully effective"))
			Expect(summary).NotTo(ContainSubstring("Verification passed"))
			Expect(summary).To(ContainSubstring("Related alerts: still firing"))
			Expect(summary).To(ContainSubstring("Metrics: no improvement detected"))
			Expect(summary).NotTo(ContainSubstring("Pod health"))
			Expect(vCtx.Outcome).To(Equal("completed"))
			Expect(vCtx.Reason).To(Equal("full"))
		})
	})

	// ========================================
	// ISSUE #639: GRADUATED METRICS NOTIFICATION WORDING
	// ========================================

	Context("Issue #639: BuildComponentBullets graduated metrics messages", func() {

		It("UT-RO-639-004: should produce 'no improvement detected' for score 0.0", func() {
			score := 0.0
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Components: eav1.EAComponents{
						MetricsAssessed: true,
						MetricsScore:    &score,
					},
				},
			}
			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).To(ContainSubstring("Metrics: no improvement detected"))
			Expect(bullets).NotTo(ContainSubstring("anomaly persists"))
		})

		It("UT-RO-639-005: should produce 'minimal improvement (score: 0.30)' for score 0.3", func() {
			score := 0.3
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Components: eav1.EAComponents{
						MetricsAssessed: true,
						MetricsScore:    &score,
					},
				},
			}
			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).To(ContainSubstring("Metrics: minimal improvement (score: 0.30)"))
			Expect(bullets).NotTo(ContainSubstring("anomaly persists"))
		})

		It("UT-RO-639-006: should produce 'partial improvement (score: 0.75)' for score 0.75", func() {
			score := 0.75
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Components: eav1.EAComponents{
						MetricsAssessed: true,
						MetricsScore:    &score,
					},
				},
			}
			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).To(ContainSubstring("Metrics: partial improvement (score: 0.75)"))
			Expect(bullets).NotTo(ContainSubstring("anomaly persists"))
		})

		It("UT-RO-639-007: should produce no metrics bullet for perfect score 1.0", func() {
			score := 1.0
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Components: eav1.EAComponents{
						MetricsAssessed: true,
						MetricsScore:    &score,
					},
				},
			}
			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).NotTo(ContainSubstring("Metrics"))
		})

		It("UT-RO-639-008: should produce no metrics bullet when metrics not assessed", func() {
			ea := &eav1.EffectivenessAssessment{
				Status: eav1.EffectivenessAssessmentStatus{
					Components: eav1.EAComponents{
						MetricsAssessed: false,
					},
				},
			}
			bullets := creator.BuildComponentBullets(ea)
			Expect(bullets).NotTo(ContainSubstring("Metrics"))
		})
	})
})
