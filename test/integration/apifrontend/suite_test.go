package apifrontend_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	v1alpha1 "github.com/jordigilh/kubernaut/api/apifrontend/apifrontend/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	testEnv   *envtest.Environment
	k8sClient client.Client
	scheme    *runtime.Scheme
)

func TestAPIfrontendIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Frontend Integration Suite")
}

var _ = BeforeSuite(func() {
	Expect(os.Getenv("KUBEBUILDER_ASSETS")).NotTo(BeEmpty(),
		"KUBEBUILDER_ASSETS must be set — run 'make setup-envtest' first")

	scheme = runtime.NewScheme()
	Expect(v1alpha1.AddToScheme(scheme)).To(Succeed())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{"../../../config/crd/bases"},
		Scheme:            scheme,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if testEnv != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}
})
