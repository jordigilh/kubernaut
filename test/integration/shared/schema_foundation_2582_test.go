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

package shared

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const schemaFoundationNamespace = "schema-foundation-2582"

var (
	schemaFoundationClient client.Client
	schemaFoundationEnv    *envtest.Environment
	schemaFoundationCtx    context.Context
)

func TestSchemaFoundationIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Schema Foundation Integration Suite (#2582)")
}

var _ = Describe("Schema foundation", func() {
	BeforeEach(func() {
		schemaFoundationCtx = context.Background()
		schemaFoundationEnv = &envtest.Environment{
			CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
			ErrorIfCRDPathMissing: true,
		}
		cfg, err := schemaFoundationEnv.Start()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			Expect(schemaFoundationEnv.Stop()).To(Succeed())
		})

		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		schemaFoundationClient, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(schemaFoundationClient.Create(schemaFoundationCtx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: schemaFoundationNamespace},
		})).To(Succeed())
	})

	It("IT-SCHEMA-2582-001 [BR-AI-009, SI-10, OWASP ASVS v5.0.0 V2.2.1]: persists the AgentSession recovery reference through the real API server", func() {
		analysis := newSchemaFoundationObject("schema-2582-aianalysis", "AIAnalysis", aiAnalysisSpec())
		Expect(schemaFoundationClient.Create(schemaFoundationCtx, analysis)).To(Succeed())
		Expect(unstructured.SetNestedField(analysis.Object, "Cancelled", "status", "phase")).To(Succeed())
		Expect(unstructured.SetNestedField(analysis.Object, int64(0), "status", "investigationSession", "generation")).To(Succeed())
		Expect(unstructured.SetNestedField(analysis.Object, map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "AgentSession",
			"name":       "agent-session-2582",
			"namespace":  schemaFoundationNamespace,
			"uid":        "agent-session-uid-2582",
		}, "status", "investigationSession", "agentSessionRef")).To(Succeed())
		Expect(schemaFoundationClient.Status().Update(schemaFoundationCtx, analysis)).To(Succeed())

		persisted := newSchemaFoundationObject(analysis.GetName(), "AIAnalysis", nil)
		Expect(schemaFoundationClient.Get(schemaFoundationCtx, client.ObjectKey{
			Name:      analysis.GetName(),
			Namespace: schemaFoundationNamespace,
		}, persisted)).To(Succeed())
		Expect(nestedString(persisted, "status", "phase")).To(Equal("Cancelled"))
		Expect(nestedString(persisted, "status", "investigationSession", "agentSessionRef", "name")).To(Equal("agent-session-2582"))
		Expect(nestedString(persisted, "status", "investigationSession", "agentSessionRef", "uid")).To(Equal("agent-session-uid-2582"))
	})

	It("IT-SCHEMA-2582-002 [BR-WE-014, BR-ORCH-026/036, SI-10, OWASP ASVS v5.0.0 V2.2.1/V2.2.2]: admits new lifecycle values and rejects invalid values", func() {
		for _, testCase := range []struct {
			name       string
			kind       string
			spec       map[string]interface{}
			statusPath []string
			valid      []string
		}{
			{name: "effectivenessassessment", kind: "EffectivenessAssessment", spec: effectivenessAssessmentSpec(), statusPath: []string{"status", "phase"}, valid: []string{"Cancelled"}},
			{name: "remediationapprovalrequest", kind: "RemediationApprovalRequest", spec: remediationApprovalRequestSpec(), statusPath: []string{"status", "decision"}, valid: []string{"Cancelled"}},
			{name: "remediationrequest", kind: "RemediationRequest", spec: remediationRequestSpec(), statusPath: []string{"status", "completionStatus", "failurePhase"}, valid: []string{"EffectivenessAssessment"}},
			{name: "signalprocessing", kind: "SignalProcessing", spec: signalProcessingSpec(), statusPath: []string{"status", "phase"}, valid: []string{"Cancelled"}},
			{name: "workflowexecution", kind: "WorkflowExecution", spec: workflowExecutionSpec(), statusPath: []string{"status", "phase"}, valid: []string{"Cancelling", "Cancelled"}},
		} {
			By("admitting " + testCase.kind + " schema values")
			resource := newSchemaFoundationObject("schema-2582-"+testCase.name, testCase.kind, testCase.spec)
			Expect(schemaFoundationClient.Create(schemaFoundationCtx, resource)).To(Succeed())

			for _, valid := range testCase.valid {
				current := newSchemaFoundationObject(resource.GetName(), testCase.kind, nil)
				Expect(schemaFoundationClient.Get(schemaFoundationCtx, client.ObjectKey{
					Name:      resource.GetName(),
					Namespace: schemaFoundationNamespace,
				}, current)).To(Succeed())
				Expect(unstructured.SetNestedField(current.Object, valid, testCase.statusPath...)).To(Succeed())
				Expect(schemaFoundationClient.Status().Update(schemaFoundationCtx, current)).To(Succeed(),
					"%s should accept %q", testCase.kind, valid)
			}

			invalid := newSchemaFoundationObject(resource.GetName(), testCase.kind, nil)
			Expect(schemaFoundationClient.Get(schemaFoundationCtx, client.ObjectKey{
				Name:      resource.GetName(),
				Namespace: schemaFoundationNamespace,
			}, invalid)).To(Succeed())
			Expect(unstructured.SetNestedField(invalid.Object, "InvalidSchemaFoundation2582", testCase.statusPath...)).To(Succeed())
			err := schemaFoundationClient.Status().Update(schemaFoundationCtx, invalid)
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "%s should reject an invalid enum value: %v", testCase.kind, err)
		}
	})
})

func newSchemaFoundationObject(name, kind string, spec map[string]interface{}) *unstructured.Unstructured {
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kubernaut.ai/v1alpha1",
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": schemaFoundationNamespace,
		},
	}}
	if spec != nil {
		object.Object["spec"] = spec
	}
	object.SetGroupVersionKind(schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: kind})
	return object
}

func nestedString(object *unstructured.Unstructured, fields ...string) string {
	value, found, err := unstructured.NestedString(object.Object, fields...)
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue(), "expected field %q", strings.Join(fields, "."))
	return value
}

func aiAnalysisSpec() map[string]interface{} {
	return map[string]interface{}{
		"remediationRequestRef": map[string]interface{}{"apiVersion": "kubernaut.ai/v1alpha1", "kind": "RemediationRequest", "name": "rr-2582"},
		"remediationId":         "rr-2582",
		"analysisRequest": map[string]interface{}{
			"analysisTypes": []interface{}{"RootCause"},
			"signalContext": map[string]interface{}{
				"fingerprint":       strings.Repeat("a", 64),
				"severity":          "high",
				"signalName":        "SchemaFoundation",
				"signalMode":        "reactive",
				"environment":       "test",
				"businessPriority":  "P1",
				"targetResource":    map[string]interface{}{"kind": "Deployment", "name": "schema-target"},
				"enrichmentResults": map[string]interface{}{},
			},
		},
	}
}

func effectivenessAssessmentSpec() map[string]interface{} {
	return map[string]interface{}{
		"correlationID":           "rr-2582",
		"remediationRequestPhase": "Failed",
		"signalTarget":            map[string]interface{}{"kind": "Deployment", "name": "schema-target"},
		"remediationTarget":       map[string]interface{}{"kind": "Deployment", "name": "schema-target"},
		"config":                  map[string]interface{}{"stabilizationWindow": "1m"},
	}
}

func remediationApprovalRequestSpec() map[string]interface{} {
	return map[string]interface{}{
		"remediationRequestRef": map[string]interface{}{"apiVersion": "kubernaut.ai/v1alpha1", "kind": "RemediationRequest", "name": "rr-2582"},
		"aiAnalysisRef":         map[string]interface{}{"name": "aa-2582"},
		"confidence":            0.8,
		"confidenceLevel":       "high",
		"reason":                "schema foundation",
		"recommendedWorkflow": map[string]interface{}{
			"workflowId":      "workflow-2582",
			"version":         "1.0.0",
			"executionBundle": "oci://example.invalid/workflow@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"rationale":       "schema foundation",
		},
		"investigationSummary": "schema foundation",
		"recommendedActions": []interface{}{map[string]interface{}{
			"action":    "validate schema",
			"rationale": "schema foundation",
		}},
		"whyApprovalRequired": "schema foundation",
		"requiredBy":          "2026-01-01T00:00:00Z",
	}
}

func remediationRequestSpec() map[string]interface{} {
	return map[string]interface{}{
		"signalFingerprint": strings.Repeat("b", 64),
		"signalName":        "SchemaFoundation",
		"severity":          "high",
		"signalType":        "alert",
		"targetType":        "kubernetes",
		"targetResource":    map[string]interface{}{"kind": "Deployment", "name": "schema-target"},
		"firingTime":        "2026-01-01T00:00:00Z",
		"receivedTime":      "2026-01-01T00:00:00Z",
	}
}

func signalProcessingSpec() map[string]interface{} {
	return map[string]interface{}{
		"remediationRequestRef": map[string]interface{}{"name": "rr-2582"},
		"signal": map[string]interface{}{
			"fingerprint":    strings.Repeat("c", 64),
			"name":           "SchemaFoundation",
			"receivedTime":   "2026-01-01T00:00:00Z",
			"severity":       "high",
			"targetResource": map[string]interface{}{"kind": "Deployment", "name": "schema-target"},
			"targetType":     "kubernetes",
			"type":           "alert",
		},
	}
}

func workflowExecutionSpec() map[string]interface{} {
	return map[string]interface{}{
		"remediationRequestRef": map[string]interface{}{"name": "rr-2582"},
		"targetResource":        "default/deployment/schema-target",
		"workflowRef": map[string]interface{}{
			"workflowId":      "workflow-2582",
			"workflowName":    "schema-foundation",
			"actionType":      "ValidateSchema",
			"version":         "1.0.0",
			"executionBundle": "oci://example.invalid/workflow@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}
}
