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

package fmc_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

const (
	fmcRedisContainerName = "fmc_valkey_it_1"
	fmcRedisPort          = 16391
)

func TestFMC(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FMC Service Integration Suite")
}

var (
	valkeyAddr string
	dynClient  dynamic.Interface
	testEnv    *envtest.Environment
)

var _ = SynchronizedBeforeSuite(func() []byte {
	By("Starting Valkey container for FMC IT")
	cfg := infrastructure.RedisConfig{
		ContainerName: fmcRedisContainerName,
		Port:          fmcRedisPort,
	}
	infrastructure.CleanupContainers([]string{fmcRedisContainerName}, GinkgoWriter)
	Expect(infrastructure.StartRedis(cfg, GinkgoWriter)).To(Succeed(),
		"Failed to start Valkey container")
	Expect(infrastructure.WaitForRedisReady(fmcRedisContainerName, GinkgoWriter)).To(Succeed(),
		"Valkey failed to become ready")

	By("Starting envtest with MCPServerRegistration CRD")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../../../config/crd/bases",
			"../../../config/crd/external",
		},
		ErrorIfCRDPathMissing: true,
	}
	sharedK8sConfig, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred(), "envtest should start")
	GinkgoWriter.Printf("envtest started at %s\n", sharedK8sConfig.Host)

	kubeconfigPath, err := infrastructure.WriteEnvtestKubeconfigToFile(sharedK8sConfig, "fmc-it")
	Expect(err).ToNot(HaveOccurred(), "Failed to write envtest kubeconfig")

	addr := fmt.Sprintf("127.0.0.1:%d", fmcRedisPort)
	return []byte(addr + "\n" + kubeconfigPath)
}, func(data []byte) {
	parts := strings.SplitN(string(data), "\n", 2)
	valkeyAddr = parts[0]
	kubeconfigPath := parts[1]

	fmt.Fprintf(os.Stdout, "FMC IT using Valkey at %s\n", valkeyAddr)
	fmt.Fprintf(os.Stdout, "FMC IT using envtest kubeconfig at %s\n", kubeconfigPath)

	k8sCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "kubeconfig should be loadable")

	dynClient, err = dynamic.NewForConfig(k8sCfg)
	Expect(err).ToNot(HaveOccurred(), "dynamic client should be created")
})

var _ = SynchronizedAfterSuite(func() {}, func() {
	By("Stopping envtest")
	if testEnv != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}
	By("Stopping Valkey container")
	infrastructure.CleanupContainers([]string{fmcRedisContainerName}, GinkgoWriter)
})
