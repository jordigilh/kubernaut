package creator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
)

// TestAIAnalysisCreator_UseNormalizedSeverity verifies DD-SEVERITY-001 implementation
// Business Requirement: BR-SP-105 (Severity Determination via Rego Policy)
// Design Decision: DD-SEVERITY-001 Q1 (AIAnalysis uses normalized, Notifications use external)
func TestAIAnalysisCreator_UseNormalizedSeverity(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	_ = remediationv1.AddToScheme(scheme)
	_ = signalprocessingv1.AddToScheme(scheme)
	_ = aianalysisv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	metricsRecorder := metrics.NewOrchestrator()
	aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, metricsRecorder)

	tests := []struct {
		name                 string
		externalSeverity     string // Customer-specific (e.g., "Sev1", "P0")
		normalizedSeverity   string // Rego-determined ("critical", "warning", "info")
		expectedAISeverity   string // What AIAnalysis should receive
		scenarioDescription  string
	}{
		{
			name:                 "Sev1 maps to critical",
			externalSeverity:     "Sev1",
			normalizedSeverity:   "critical",
			expectedAISeverity:   "critical",
			scenarioDescription:  "Customer uses Sev1-4 scheme, Rego maps Sev1→critical",
		},
		{
			name:                 "P0 maps to critical",
			externalSeverity:     "P0",
			normalizedSeverity:   "critical",
			expectedAISeverity:   "critical",
			scenarioDescription:  "Customer uses P0-P4 scheme, Rego maps P0→critical",
		},
		{
			name:                 "Sev3 maps to warning",
			externalSeverity:     "Sev3",
			normalizedSeverity:   "warning",
			expectedAISeverity:   "warning",
			scenarioDescription:  "Customer uses Sev1-4 scheme, Rego maps Sev3→warning",
		},
		{
			name:                 "HIGH maps to warning",
			externalSeverity:     "HIGH",
			normalizedSeverity:   "warning",
			expectedAISeverity:   "warning",
			scenarioDescription:  "Customer uses Critical/High/Medium/Low, Rego maps HIGH→warning",
		},
		{
			name:                 "Standard critical passes through",
			externalSeverity:     "critical",
			normalizedSeverity:   "critical",
			expectedAISeverity:   "critical",
			scenarioDescription:  "Standard severity value (1:1 mapping in default Rego)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// GIVEN: RemediationRequest with external severity
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rr",
					Namespace: "default",
					UID:       "test-uid-123",
				},
				Spec: remediationv1.RemediationRequestSpec{
					Severity:          tt.externalSeverity, // External severity from customer
					SignalFingerprint: "test-fingerprint",
					SignalType:        "test-signal-type",
					TargetResource: remediationv1.ResourceReference{
						Kind:      "Pod",
						Name:      "test-pod",
						Namespace: "default",
					},
				},
			}

			// AND: SignalProcessing with normalized severity from Rego
			sp := &signalprocessingv1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sp",
					Namespace: "default",
				},
				Spec: signalprocessingv1.SignalProcessingSpec{
					Signal: signalprocessingv1.SignalData{
						Severity: tt.externalSeverity, // External (same as RR)
					},
				},
				Status: signalprocessingv1.SignalProcessingStatus{
					Severity: tt.normalizedSeverity, // ✅ Normalized by Rego policy
					EnvironmentClassification: &signalprocessingv1.EnvironmentClassification{
						Environment: "production",
					},
				},
			}

			// WHEN: Creating AIAnalysis
			analysis, err := aiCreator.Create(rr, sp)

			// THEN: AIAnalysis receives normalized severity
			assert.NoError(t, err, "AIAnalysis creation should succeed")
			assert.NotNil(t, analysis, "AIAnalysis should be created")
			assert.Equal(t, tt.expectedAISeverity, analysis.Spec.AnalysisRequest.SignalContext.Severity,
				"AIAnalysis should receive normalized severity from sp.Status.Severity (not external rr.Spec.Severity)\nScenario: %s",
				tt.scenarioDescription)

			// ALSO VERIFY: External severity is preserved in RR (for notifications)
			assert.Equal(t, tt.externalSeverity, rr.Spec.Severity,
				"RemediationRequest should preserve external severity for operator-facing messages")
		})
	}
}

// TestAIAnalysisCreator_SeverityFallback verifies defensive behavior when Status.Severity is empty
// Edge Case: SignalProcessing hasn't reached Classifying phase yet (should rarely happen in production)
func TestAIAnalysisCreator_SeverityFallback(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	_ = remediationv1.AddToScheme(scheme)
	_ = signalprocessingv1.AddToScheme(scheme)
	_ = aianalysisv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	metricsRecorder := metrics.NewOrchestrator()
	aiCreator := creator.NewAIAnalysisCreator(fakeClient, scheme, metricsRecorder)

	// GIVEN: SignalProcessing with empty Status.Severity (edge case)
	rr := &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rr",
			Namespace: "default",
			UID:       "test-uid-123",
		},
		Spec: remediationv1.RemediationRequestSpec{
			Severity:          "Sev1",
			SignalFingerprint: "test-fingerprint",
			SignalType:        "test-signal-type",
			TargetResource: remediationv1.ResourceReference{
				Kind:      "Pod",
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	}

	sp := &signalprocessingv1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sp",
			Namespace: "default",
		},
		Spec: signalprocessingv1.SignalProcessingSpec{
			Signal: signalprocessingv1.SignalData{
				Severity: "Sev1",
			},
		},
		Status: signalprocessingv1.SignalProcessingStatus{
			Severity: "", // Empty - SignalProcessing hasn't classified yet
			EnvironmentClassification: &signalprocessingv1.EnvironmentClassification{
				Environment: "production",
			},
		},
	}

	// WHEN: Creating AIAnalysis
	analysis, err := aiCreator.Create(rr, sp)

	// THEN: AIAnalysis creation succeeds with empty severity
	// (CRD validation will catch this if it's a problem, but we don't add defensive fallback here)
	assert.NoError(t, err, "AIAnalysis creation should succeed")
	assert.NotNil(t, analysis, "AIAnalysis should be created")
	assert.Equal(t, "", analysis.Spec.AnalysisRequest.SignalContext.Severity,
		"Empty Status.Severity should be passed through (CRD validation will handle invalid values)")
}
