package signalprocessing

import (
	"crypto/sha256"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// GenerateTestFingerprint creates a valid 64-character hex fingerprint for testing.
// Per spec.signal.fingerprint validation: ^[a-f0-9]{64}$
// This generates deterministic SHA256 hashes from seed strings for test reproducibility.
//
// Example:
//
//	GenerateTestFingerprint("test-signal-001") // returns 64-char hex string
func GenerateTestFingerprint(seed string) string {
	hash := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("%x", hash)
}

// ValidTestFingerprints provides pre-generated valid fingerprints for common test scenarios.
// All fingerprints are valid 64-character hex strings matching ^[a-f0-9]{64}$
var ValidTestFingerprints = map[string]string{
	// Hot reload tests
	"hr-file-watch-01":   GenerateTestFingerprint("hr-file-watch-test-1"),
	"hr-file-watch-02":   GenerateTestFingerprint("hr-file-watch-test-2"),
	"hr-reload-valid-01": GenerateTestFingerprint("hr-reload-valid-test-1"),
	"hr-reload-valid-02": GenerateTestFingerprint("hr-reload-valid-test-2"),
	"hr-graceful-01":     GenerateTestFingerprint("hr-graceful-test-1"),
	"hr-graceful-02":     GenerateTestFingerprint("hr-graceful-test-2"),

	// Component integration tests
	"enrich-pod":        GenerateTestFingerprint("enrich-pod-test"),
	"enrich-deploy":     GenerateTestFingerprint("enrich-deploy-test"),
	"enrich-sts":        GenerateTestFingerprint("enrich-sts-test"),
	"enrich-svc":        GenerateTestFingerprint("enrich-svc-test"),
	"enrich-ns":         GenerateTestFingerprint("enrich-ns-test"),
	"enrich-degraded":   GenerateTestFingerprint("enrich-degraded-test"),
	"env-configmap":     GenerateTestFingerprint("env-configmap-test"),
	"env-label":         GenerateTestFingerprint("env-label-priority-test"),
	"priority-rego":     GenerateTestFingerprint("priority-rego-test"),
	"priority-fallback": GenerateTestFingerprint("priority-fallback-test"),
	"priority-cm":       GenerateTestFingerprint("priority-configmap-test"),
	"business-label":    GenerateTestFingerprint("business-label-test"),
	"business-pattern":  GenerateTestFingerprint("business-pattern-test"),
	"ownerchain":        GenerateTestFingerprint("ownerchain-real-test"),
	"detect-pdb":        GenerateTestFingerprint("detect-pdb-test"),
	"detect-hpa":        GenerateTestFingerprint("detect-hpa-test"),
	"detect-netpol":     GenerateTestFingerprint("detect-netpol-test"),

	// Reconciler integration tests
	"reconciler-01": GenerateTestFingerprint("reconciler-test-01"),
	"reconciler-02": GenerateTestFingerprint("reconciler-test-02"),
	"reconciler-03": GenerateTestFingerprint("reconciler-test-03"),
	"reconciler-04": GenerateTestFingerprint("reconciler-test-04"),
	"reconciler-05": GenerateTestFingerprint("reconciler-test-05"),
	"reconciler-06": GenerateTestFingerprint("reconciler-test-06"),
	"reconciler-07": GenerateTestFingerprint("reconciler-test-07"),
	"reconciler-08": GenerateTestFingerprint("reconciler-test-08"),
	"reconciler-09": GenerateTestFingerprint("reconciler-test-09"),
	"reconciler-10": GenerateTestFingerprint("reconciler-test-10"),
	"edge-case-01":  GenerateTestFingerprint("edge-case-test-01"),
	"edge-case-02":  GenerateTestFingerprint("edge-case-test-02"),
	"edge-case-04":  GenerateTestFingerprint("edge-case-test-04"),
	"edge-case-05":  GenerateTestFingerprint("edge-case-test-05"),
	"edge-case-06":  GenerateTestFingerprint("edge-case-test-06"),
	"edge-case-07":  GenerateTestFingerprint("edge-case-test-07"),
	"edge-case-08":  GenerateTestFingerprint("edge-case-test-08"),
	"error-02":      GenerateTestFingerprint("error-handling-test-02"),
	"error-04":      GenerateTestFingerprint("error-handling-test-04"),
	"error-06":      GenerateTestFingerprint("error-handling-test-06"),
	"audit-001":     GenerateTestFingerprint("audit-test-001"),
	"audit-002":     GenerateTestFingerprint("audit-test-002"),
	"audit-003":     GenerateTestFingerprint("audit-test-003"),
	"audit-004":     GenerateTestFingerprint("audit-test-004"),
	"audit-005":     GenerateTestFingerprint("audit-test-005"),
	"audit-006":     GenerateTestFingerprint("audit-test-006"), // AUDIT-06: Business classification

	// Component integration tests - HPA via owner chain (COMPONENT-04)
	"hpa-ownerchain": GenerateTestFingerprint("hpa-ownerchain-test"),

	// Metrics integration tests (V1.0 Maturity)
	"metrics-001": GenerateTestFingerprint("metrics-test-001"),
	"metrics-002": GenerateTestFingerprint("metrics-test-002"),
	"metrics-003": GenerateTestFingerprint("metrics-test-003"),
	"enrich-001":  GenerateTestFingerprint("enrich-metrics-test-001"),
	"error-001":   GenerateTestFingerprint("error-metrics-test-001"),

	// Backoff integration tests (BR-SP-111)
	"backoff-01": GenerateTestFingerprint("backoff-test-01"),
	"backoff-02": GenerateTestFingerprint("backoff-test-02"),
	"backoff-03": GenerateTestFingerprint("backoff-test-03"),
	"backoff-04": GenerateTestFingerprint("backoff-test-04"),

	// Rego integration tests
	"rego-env-01": GenerateTestFingerprint("rego-env-test-01"),
	"rego-pri-01": GenerateTestFingerprint("rego-pri-test-01"),
	"rego-lbl-01": GenerateTestFingerprint("rego-lbl-test-01"),
	"rego-eve-01": GenerateTestFingerprint("rego-eval-env-test-01"),
	"rego-evp-01": GenerateTestFingerprint("rego-eval-priority-test-01"),
	"rego-evl-01": GenerateTestFingerprint("rego-eval-labels-test-01"),
	"rego-sec-01": GenerateTestFingerprint("rego-security-test-01"),
	"rego-fin-01": GenerateTestFingerprint("rego-fallback-invalid-test-01"),
	"rego-fms-01": GenerateTestFingerprint("rego-fallback-missing-test-01"),
	"rego-tim-01": GenerateTestFingerprint("rego-timeout-test-01"),
	"rego-vlk-01": GenerateTestFingerprint("rego-validation-key-test-01"),
	"rego-vlv-01": GenerateTestFingerprint("rego-validation-value-test-01"),
	"rego-vmk-01": GenerateTestFingerprint("rego-validation-max-keys-test-01"),
}

// GenerateConcurrentFingerprint generates a unique fingerprint for concurrent tests.
// Ensures each concurrent test has a unique, valid fingerprint.
func GenerateConcurrentFingerprint(baseSeed string, index int) string {
	return GenerateTestFingerprint(fmt.Sprintf("%s-%d", baseSeed, index))
}

// CreateTestRemediationRequest creates a minimal RemediationRequest for integration tests.
// Authority: SignalProcessing CRs MUST have parent RemediationRequest (pkg/remediationorchestrator/creator/signalprocessing.go:77-114)
//
// This helper creates the parent RR that RO would normally create in production.
// Integration tests use this to match production architecture where RO always creates SP with RemediationRequestRef.
//
// Parameters:
//   - name: RemediationRequest name (will be used for SP's RemediationRequestRef.Name)
//   - namespace: Kubernetes namespace
//   - fingerprint: Signal fingerprint (64-char hex string from ValidTestFingerprints)
//   - targetResource: Target resource identifier (Pod, Deployment, etc.)
//
// Returns: *remediationv1.RemediationRequest with minimal required fields
func CreateTestRemediationRequest(name, namespace, fingerprint, severity string, targetResource signalprocessingv1alpha1.ResourceIdentifier) *remediationv1.RemediationRequest {
	return &remediationv1.RemediationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"kubernaut.ai/test": "true",
			},
		},
		Spec: remediationv1.RemediationRequestSpec{
			// Core signal identification (REQUIRED per BR-ORCH-025)
			SignalFingerprint: fingerprint,
			SignalName:        "TestSignal",
			Severity:          severity,
			SignalType:        "alert",
			SignalSource:      "test-adapter",
			TargetType:        "kubernetes",

			// Target resource (REQUIRED per RR CRD validation)
			TargetResource: remediationv1.ResourceIdentifier{
				Kind:      targetResource.Kind,
				Name:      targetResource.Name,
				Namespace: targetResource.Namespace,
			},

			// Temporal data
			FiringTime:   metav1.Now(),
			ReceivedTime: metav1.Now(),

			// Deduplication (uses shared type from pkg/shared/types/deduplication.go)
			Deduplication: sharedtypes.DeduplicationInfo{
				IsDuplicate:     false,
				FirstOccurrence: metav1.Now(),
				LastOccurrence:  metav1.Now(),
				OccurrenceCount: 1,
			},

			// Storm detection (optional for tests)
			IsStorm: false,
		},
	}
}

// CreateTestSignalProcessingWithParent creates a SignalProcessing CR with proper RemediationRequestRef.
// Authority: Production architecture requires SP to reference parent RR (pkg/remediationorchestrator/creator/signalprocessing.go:91-97)
//
// This helper creates SP the same way RO does in production - with RemediationRequestRef populated.
// This ensures integration tests match production architecture.
//
// Parameters:
//   - name: SignalProcessing name
//   - namespace: Kubernetes namespace
//   - parentRR: Parent RemediationRequest CR (created with CreateTestRemediationRequest)
//   - fingerprint: Signal fingerprint (must match parent RR's fingerprint)
//   - targetResource: Target resource identifier
//
// Returns: *signalprocessingv1alpha1.SignalProcessing with RemediationRequestRef set
func CreateTestSignalProcessingWithParent(name, namespace string, parentRR *remediationv1.RemediationRequest, fingerprint string, targetResource signalprocessingv1alpha1.ResourceIdentifier) *signalprocessingv1alpha1.SignalProcessing {
	return &signalprocessingv1alpha1.SignalProcessing{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: signalprocessingv1alpha1.SignalProcessingSpec{
			// Reference to parent RemediationRequest (REQUIRED for audit trail)
			// Authority: RO sets this in production (pkg/remediationorchestrator/creator/signalprocessing.go:91-97)
			RemediationRequestRef: signalprocessingv1alpha1.ObjectReference{
				APIVersion: remediationv1.GroupVersion.String(),
				Kind:       "RemediationRequest",
				Name:       parentRR.Name,
				Namespace:  parentRR.Namespace,
				UID:        string(parentRR.UID),
			},

			// Signal data (matching parent RR)
			Signal: signalprocessingv1alpha1.SignalData{
				Fingerprint:    fingerprint,
				Name:           parentRR.Spec.SignalName,
				Severity:       parentRR.Spec.Severity,
				Type:           parentRR.Spec.SignalType,
				Source:         parentRR.Spec.SignalSource,
				TargetType:     parentRR.Spec.TargetType,
				TargetResource: targetResource,
				ReceivedTime:   parentRR.Spec.ReceivedTime,
			},
		},
	}
}
