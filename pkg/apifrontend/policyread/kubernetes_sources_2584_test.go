package policyread_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// runtimePolicyConfigMaps is the operator-rendered runtime shape: the
// component ConfigMaps carry policy references, and the referenced ConfigMaps
// carry the policy source under a single configured key.
func runtimePolicyConfigMaps(aiThreshold string) []client.Object {
	return []client.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "aianalysis-config", Namespace: "kubernaut-system"},
			Data:       map[string]string{"config.yaml": "rego:\n  policyPath: /etc/aianalysis/policies/approval.rego\n  policyConfigMapName: custom-ai-policy\n  policyConfigMapKey: approval.rego\n  confidenceThreshold: \"" + aiThreshold + "\"\n"},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "signalprocessing-config", Namespace: "kubernaut-system"},
			Data:       map[string]string{"config.yaml": "classifier:\n  regoConfigMapName: custom-signal-policy\n  regoConfigMapKey: policy.rego\n"},
		},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "custom-ai-policy", Namespace: "kubernaut-system"}, Data: map[string]string{"approval.rego": "package approval"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "custom-signal-policy", Namespace: "kubernaut-system"}, Data: map[string]string{"policy.rego": "package signal"}},
	}
}

func newRuntimePolicySource(objects ...client.Object) *policyread.KubernetesSource {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	source, err := policyread.NewKubernetesSource(reader)
	Expect(err).NotTo(HaveOccurred())
	return source
}

var _ = Describe("BR-AI-085: typed Kubernetes policy sources", func() {
	It("UT-AF-2584-002 [BR-AI-085]: projects current policy identity and revision without the policy source", func() {
		source := newRuntimePolicySource(runtimePolicyConfigMaps("0.85")...)

		policies, err := source.ReadCurrentPolicies(context.Background(), "kubernaut-system")
		Expect(err).NotTo(HaveOccurred())
		Expect(policies).To(HaveLen(2))
		Expect(policies[0].Type).To(Equal("aiAnalysis"))
		Expect(policies[0].ConfigMapName).To(Equal("custom-ai-policy"))
		Expect(policies[0].ConfigMapKey).To(Equal("approval.rego"))
		Expect(policies[0].Revision).To(MatchRegexp(`^sha256:[0-9a-f]{64}$`))
		Expect(policies[1].Type).To(Equal("signalProcessing"))
		Expect(policies[1].ConfigMapName).To(Equal("custom-signal-policy"))
		Expect(policies[1].ConfigMapKey).To(Equal("policy.rego"))
		Expect(policies[1].Revision).To(MatchRegexp(`^sha256:[0-9a-f]{64}$`))
	})

	It("UT-AF-2584-001 [BR-AI-088]: represents the runtime confidence threshold exactly", func() {
		source := newRuntimePolicySource(runtimePolicyConfigMaps("0.85")...)

		policies, err := source.ReadCurrentPolicies(context.Background(), "kubernaut-system")
		Expect(err).NotTo(HaveOccurred())
		Expect(policies[0].Threshold).NotTo(BeNil())
		Expect(policies[0].Threshold.Value).To(Equal(0.85))
		Expect(policies[0].Threshold.Source).To(Equal("runtime-config"))
		Expect(policies[0].Threshold.Kind).To(Equal("rego-approval"))
	})

	It("UT-AF-2584-001 [BR-AI-088]: fails closed when the runtime threshold is missing", func() {
		source := newRuntimePolicySource(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "aianalysis-config", Namespace: "kubernaut-system"},
				Data:       map[string]string{"config.yaml": "rego:\n  policyPath: /etc/aianalysis/policies/approval.rego\n"},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "signalprocessing-config", Namespace: "kubernaut-system"},
				Data:       map[string]string{"config.yaml": "classifier:\n  regoConfigMapName: signalprocessing-policy\n  regoConfigMapKey: policy.rego\n"},
			},
		)

		_, err := source.ReadCurrentPolicies(context.Background(), "kubernaut-system")
		Expect(err).To(MatchError(ContainSubstring("confidenceThreshold")))
	})

	It("UT-AF-2584-001 [BR-AI-088]: fails closed when the runtime threshold is out of range", func() {
		source := newRuntimePolicySource(runtimePolicyConfigMaps("1.5")...)

		_, err := source.ReadCurrentPolicies(context.Background(), "kubernaut-system")
		Expect(err).To(MatchError(ContainSubstring("confidenceThreshold")))
	})

	It("UT-AF-2584-002 [BR-AI-085]: fails closed when the AIAnalysis runtime policy reference is missing", func() {
		source := newRuntimePolicySource(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "aianalysis-config", Namespace: "kubernaut-system"},
			Data:       map[string]string{"config.yaml": "rego:\n  confidenceThreshold: \"0.85\"\n"},
		})

		_, err := source.ReadCurrentPolicies(context.Background(), "kubernaut-system")
		Expect(err).To(MatchError(ContainSubstring("AIAnalysis policy ConfigMap is required")))
	})

	It("UT-AF-2584-009 [BR-AI-085]: reads only the configured ConfigMap key", func() {
		source := newRuntimePolicySource(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "policy", Namespace: "kubernaut-system"},
			Data:       map[string]string{"approval.rego": "package approval", "unrelated.rego": "package unrelated"},
		})

		value, err := source.Read(context.Background(), "kubernaut-system", "policy", "approval.rego")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(value)).To(Equal("package approval"))
		Expect(string(value)).NotTo(ContainSubstring("package unrelated"))
	})

	It("UT-AF-2584-006 [BR-AI-085]: lists typed history records", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&aianalysisv1.AIAnalysis{ObjectMeta: metav1.ObjectMeta{Name: "ai-1", Namespace: "kubernaut-system"}},
		).Build()
		source, err := policyread.NewKubernetesSource(reader)
		Expect(err).NotTo(HaveOccurred())

		ai, err := source.ListAIAnalysis(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(ai).To(HaveLen(1))
		Expect(ai[0].Name).To(Equal("ai-1"))
	})
})
