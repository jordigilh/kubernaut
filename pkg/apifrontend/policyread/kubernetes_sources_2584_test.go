package policyread_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/policyread"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("BR-AI-085 / UT-AF-2584-006: typed Kubernetes policy sources", func() {
	It("reads only configured ConfigMap keys and lists typed history records", func() {
		scheme := runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(spv1.AddToScheme(scheme)).To(Succeed())
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "policy", Namespace: "kubernaut-system"}, Data: map[string]string{"approval.rego": "package approval"}},
			&aianalysisv1.AIAnalysis{ObjectMeta: metav1.ObjectMeta{Name: "ai-1", Namespace: "kubernaut-system"}},
			&spv1.SignalProcessing{ObjectMeta: metav1.ObjectMeta{Name: "sp-1", Namespace: "kubernaut-system"}},
		).Build()
		source, err := policyread.NewKubernetesSource(reader)
		Expect(err).NotTo(HaveOccurred())

		value, err := source.Read(context.Background(), "kubernaut-system", "policy", "approval.rego")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(value)).To(Equal("package approval"))
		ai, err := source.ListAIAnalysis(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(ai).To(HaveLen(1))
		sp, err := source.ListSignalProcessing(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(sp).To(HaveLen(1))
	})
})
