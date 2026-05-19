package infrastructure

import (
	"fmt"
	"io"
	"os"

	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
)

// CollectE2EBinaryCoverage collects Go coverage data from the Kind node's
// hostPath volume and merges it into a text profile. This delegates to the
// shared DD-TEST-007 pipeline in test/infrastructure/coverage.go which handles
// scale-down, extraction, path remapping, and profile generation.
//
// Prerequisites:
//   - AF built with GOFLAGS=-cover (E2E_COVERAGE=true)
//   - Pod deployed with GOCOVERDIR=/coverdata
//   - Kind node has hostPath /coverdata mounted
func CollectE2EBinaryCoverage(clusterName string, writer io.Writer) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir for kubeconfig: %w", err)
	}
	kcPath := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)

	return kinfra.CollectE2EBinaryCoverage(kinfra.E2ECoverageOptions{
		ServiceName:    "apifrontend",
		ClusterName:    clusterName,
		DeploymentName: "apifrontend",
		Namespace:      "kubernaut-system",
		KubeconfigPath: kcPath,
	}, writer)
}
