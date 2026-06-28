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

package fleetmetadatacache_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

const (
	fmcRedisContainerName = "fmc_valkey_it_1"
	fmcRedisPort          = 16391

	kmcpTableContainerName = "fmc_kmcp_table_it"
	kmcpYAMLContainerName  = "fmc_kmcp_yaml_it"
	kmcpTablePort          = 18090
	kmcpYAMLPort           = 18091
)

func TestFMC(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "FMC Service Integration Suite")
}

var (
	valkeyAddr  string
	mcpTableURL string
	mcpYAMLURL  string
	dynClient   dynamic.Interface
	testEnv     *envtest.Environment
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

	By("Starting envtest with Backend CRD (Envoy AI Gateway)")
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

	By("Writing container-specific kubeconfig for kube-mcp-server")
	containerKubeconfigPath, err := writeContainerKubeconfig(sharedK8sConfig.Host, sharedK8sConfig.CertData, sharedK8sConfig.KeyData)
	Expect(err).ToNot(HaveOccurred(), "Failed to write container kubeconfig")
	GinkgoWriter.Printf("container kubeconfig: %s\n", containerKubeconfigPath)

	By("Starting kube-mcp-server containers (table + yaml)")
	infrastructure.CleanupContainers([]string{kmcpTableContainerName, kmcpYAMLContainerName}, GinkgoWriter)

	tableURL, yamlURL, startErr := startKMCPServerContainers(containerKubeconfigPath)
	if startErr != nil {
		GinkgoWriter.Printf("WARNING: kube-mcp-server containers failed: %v\n", startErr)
		GinkgoWriter.Printf("IT-FMC-PARSE tests will be skipped\n")
		tableURL = ""
		yamlURL = ""
	}

	addr := fmt.Sprintf("127.0.0.1:%d", fmcRedisPort)
	payload := strings.Join([]string{addr, kubeconfigPath, tableURL, yamlURL}, "\n")
	return []byte(payload)
}, func(data []byte) {
	parts := strings.SplitN(string(data), "\n", 4)
	valkeyAddr = parts[0]
	kubeconfigPath := parts[1]
	if len(parts) > 2 {
		mcpTableURL = parts[2]
	}
	if len(parts) > 3 {
		mcpYAMLURL = parts[3]
	}

	_, _ = fmt.Fprintf(os.Stdout, "FMC IT using Valkey at %s\n", valkeyAddr)
	_, _ = fmt.Fprintf(os.Stdout, "FMC IT using envtest kubeconfig at %s\n", kubeconfigPath)
	if mcpTableURL != "" {
		_, _ = fmt.Fprintf(os.Stdout, "FMC IT kube-mcp-server (table) at %s\n", mcpTableURL)
	}
	if mcpYAMLURL != "" {
		_, _ = fmt.Fprintf(os.Stdout, "FMC IT kube-mcp-server (yaml) at %s\n", mcpYAMLURL)
	}

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
	By("Stopping Valkey and kube-mcp-server containers")
	infrastructure.CleanupContainers([]string{
		fmcRedisContainerName,
		kmcpTableContainerName,
		kmcpYAMLContainerName,
	}, GinkgoWriter)
})

// writeContainerKubeconfig writes an envtest kubeconfig suitable for mounting
// into Podman containers. Follows DD-AUTH-014 macOS/Linux pattern:
//   - Linux: server URL unchanged (host network)
//   - macOS: rewrites 127.0.0.1 -> host.containers.internal, skips TLS verify
func writeContainerKubeconfig(apiServerURL string, certData, keyData []byte) (string, error) {
	containerAPIServer := apiServerURL
	skipTLS := false
	if runtime.GOOS != "linux" {
		containerAPIServer = strings.Replace(apiServerURL, "127.0.0.1", "host.containers.internal", 1)
		skipTLS = true
	}

	kubeconfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"envtest": {
				Server:                containerAPIServer,
				InsecureSkipTLSVerify: skipTLS,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"envtest": {
				ClientCertificateData: certData,
				ClientKeyData:         keyData,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"envtest": {
				Cluster:  "envtest",
				AuthInfo: "envtest",
			},
		},
		CurrentContext: "envtest",
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	kubeconfigDir := filepath.Join(homeDir, "tmp", "kubernaut-envtest")
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	kubeconfigPath := filepath.Join(kubeconfigDir, "envtest-kubeconfig-kmcp-it.yaml")
	if err := clientcmd.WriteToFile(kubeconfig, kubeconfigPath); err != nil {
		return "", fmt.Errorf("write kubeconfig: %w", err)
	}
	if err := os.Chmod(kubeconfigPath, 0644); err != nil {
		return "", fmt.Errorf("chmod: %w", err)
	}
	return kubeconfigPath, nil
}

// startKMCPServerContainers starts dual kube-mcp-server containers:
//   - table format on port kmcpTablePort
//   - yaml format on port kmcpYAMLPort
//
// Returns base URLs (e.g. "http://127.0.0.1:18090") or error.
func startKMCPServerContainers(kubeconfigPath string) (tableURL, yamlURL string, err error) {
	configs := []struct {
		name   string
		port   int
		format string
	}{
		{kmcpTableContainerName, kmcpTablePort, "table"},
		{kmcpYAMLContainerName, kmcpYAMLPort, "yaml"},
	}

	urls := make([]string, 2)
	for i, c := range configs {
		cfg := infrastructure.GenericContainerConfig{
			Name:  c.name,
			Image: infrastructure.KubeMCPServerImage,
			Ports: map[int]int{8080: c.port},
			Volumes: map[string]string{
				kubeconfigPath: "/tmp/kubeconfig.yaml",
			},
			ExtraHosts: []string{"host.containers.internal:host-gateway"},
			Cmd: []string{
				fmt.Sprintf("--port=%d", 8080),
				"--cluster-provider=kubeconfig",
				"--kubeconfig=/tmp/kubeconfig.yaml",
				fmt.Sprintf("--list-output=%s", c.format),
				"--toolsets=core",
				"--stateless",
				"--read-only",
				"--disable-multi-cluster",
			},
			HealthCheck: &infrastructure.HealthCheckConfig{
				URL:     fmt.Sprintf("http://127.0.0.1:%d/healthz", c.port),
				Timeout: 30 * time.Second,
			},
		}

		_, startErr := infrastructure.StartGenericContainer(cfg, GinkgoWriter)
		if startErr != nil {
			return "", "", fmt.Errorf("start %s: %w", c.name, startErr)
		}
		urls[i] = fmt.Sprintf("http://127.0.0.1:%d", c.port)
		GinkgoWriter.Printf("kube-mcp-server (%s) ready at %s\n", c.format, urls[i])
	}

	return urls[0], urls[1], nil
}
