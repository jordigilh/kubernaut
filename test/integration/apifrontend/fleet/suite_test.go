package fleet_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
)

type fleetEnvtestConnection struct {
	Host     string `json:"host"`
	CAData   []byte `json:"caData"`
	CertData []byte `json:"certData"`
	KeyData  []byte `json:"keyData"`
}

var (
	fleetEnvtestServer *envtest.Environment
	fleetRESTConfig    *rest.Config
	fleetTypedClient   crclient.WithWatch
	fleetDynamicClient dynamic.Interface
)

func TestAFFleet(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AF Fleet Integration Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	Expect(os.Getenv("KUBEBUILDER_ASSETS")).NotTo(BeEmpty(),
		"KUBEBUILDER_ASSETS must be provided by make test-integration-apifrontend")

	fleetEnvtestServer = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	restConfig, err := fleetEnvtestServer.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restConfig).NotTo(BeNil())

	payload, err := json.Marshal(fleetEnvtestConnection{
		Host:     restConfig.Host,
		CAData:   restConfig.CAData,
		CertData: restConfig.CertData,
		KeyData:  restConfig.KeyData,
	})
	Expect(err).NotTo(HaveOccurred())
	return payload
}, func(payload []byte) {
	var connection fleetEnvtestConnection
	Expect(json.Unmarshal(payload, &connection)).To(Succeed())
	fleetRESTConfig = &rest.Config{
		Host: connection.Host,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   connection.CAData,
			CertData: connection.CertData,
			KeyData:  connection.KeyData,
		},
	}

	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(remediationv1.AddToScheme(scheme)).To(Succeed())

	var err error
	fleetTypedClient, err = crclient.NewWithWatch(fleetRESTConfig, crclient.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	fleetDynamicClient, err = dynamic.NewForConfig(fleetRESTConfig)
	Expect(err).NotTo(HaveOccurred())
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	if fleetEnvtestServer == nil {
		return
	}
	Expect(fleetEnvtestServer.Stop()).To(Succeed())
})
