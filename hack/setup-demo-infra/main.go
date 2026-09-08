// Command setup-demo-infra creates a local or fleet Kind demo environment and
// installs Kubernaut with the supplied LLM configuration and policies.
//
// Local is the default topology. Pass -mode=fleet to add the fleet hub/spoke
// infrastructure and fleet Helm configuration.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

func main() {
	modeFlag := flag.String("mode", string(infrastructure.DemoModeLocal), "demo mode: local or fleet")
	clusterName := flag.String("cluster-name", "", "Kind cluster name (default: kubernaut-demo for local, kubernaut-hub for fleet)")
	remoteClusterName := flag.String("remote-cluster-name", "kubernaut-remote-cluster", "fleet spoke Kind cluster name")
	gatewayTypeFlag := flag.String("gateway-type", string(registry.GatewayEAIGW), "fleet MCP Gateway implementation: eaigw or kuadrant")
	spokeWorkers := flag.Int("spoke-workers", 0, "number of extra worker nodes for the fleet spoke cluster")
	autonomous := flag.Bool("autonomous", false, "enable Gateway-driven autonomous remediation")
	llmProvider := flag.String("llm-provider", "", "required LLM provider")
	llmModel := flag.String("llm-model", "", "required LLM model")
	llmEndpoint := flag.String("llm-endpoint", "", "required except for vertex_ai")
	llmCredentialsFile := flag.String("llm-credentials-file", "", "required file containing the raw LLM credential")
	spPolicyFile := flag.String("sp-policy-file", "", "optional SignalProcessing Rego policy file")
	aaPolicyFile := flag.String("aa-policy-file", "", "optional AIAnalysis Rego policy file")
	imageTag := flag.String("image-tag", "", "optional Kubernaut image tag override")
	vertexProject := flag.String("vertex-project", "", "required with -llm-provider=vertex_ai")
	vertexLocation := flag.String("vertex-location", "", "required with -llm-provider=vertex_ai")
	if flag.Lookup("kubeconfig") == nil {
		flag.String("kubeconfig", "", "path to write the cluster kubeconfig")
	}
	flag.Parse()
	mode := infrastructure.DemoMode(*modeFlag)
	if mode != infrastructure.DemoModeLocal && mode != infrastructure.DemoModeFleet {
		fail(fmt.Sprintf("invalid -mode %q; must be one of: local, fleet", *modeFlag))
	}
	fleet := mode == infrastructure.DemoModeFleet

	if *clusterName == "" {
		if fleet {
			*clusterName = "kubernaut-hub"
		} else {
			*clusterName = "kubernaut-demo"
		}
	}
	kubeconfigPath := flag.Lookup("kubeconfig").Value.String()
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fail(fmt.Sprintf("failed to resolve home directory: %v", err))
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", *clusterName+"-config")
	}

	gatewayType := registry.MCPGatewayType(*gatewayTypeFlag)
	if !registry.SupportedGateways[gatewayType] {
		fail(fmt.Sprintf("invalid -gateway-type %q; must be one of: eaigw, kuadrant", *gatewayTypeFlag))
	}

	demoOpts := infrastructure.DemoHelmOptions{
		Mode:               mode,
		Autonomous:         *autonomous,
		LLMProvider:        *llmProvider,
		LLMModel:           *llmModel,
		LLMEndpoint:        *llmEndpoint,
		LLMCredentialsFile: *llmCredentialsFile,
		SPPolicyFile:       *spPolicyFile,
		AAPolicyFile:       *aaPolicyFile,
		ImageTag:           *imageTag,
		VertexProject:      *vertexProject,
		VertexLocation:     *vertexLocation,
	}
	if err := demoOpts.Validate(); err != nil {
		fail(err.Error())
	}
	if *llmCredentialsFile == "" {
		fail("missing required flag: -llm-credentials-file")
	}

	ctx := context.Background()
	var fleetOpts *infrastructure.FleetHelmOptions
	var remoteKubeconfigPath string
	var err error
	if fleet {
		fleetOpts, remoteKubeconfigPath, err = infrastructure.SetupFleetCoreInfrastructureWithGateway(
			ctx, *clusterName, *remoteClusterName, kubeconfigPath,
			infrastructure.FleetCoreDemoOptions{
				GatewayType:  gatewayType,
				SpokeWorkers: *spokeWorkers,
			}, os.Stdout)
	} else {
		err = infrastructure.CreateKindClusterWithConfig(ctx, infrastructure.KindClusterOptions{
			ClusterName:             *clusterName,
			KubeconfigPath:          kubeconfigPath,
			ConfigPath:              "test/infrastructure/kind-fullpipeline-config.yaml",
			WaitTimeout:             "60s",
			ReuseExisting:           true,
			ProjectRootAsWorkingDir: true,
		}, os.Stdout)
		if err == nil {
			err = infrastructure.CreateTestNamespace(ctx, "kubernaut-system", kubeconfigPath, os.Stdout)
		}
	}
	if err != nil {
		fail(fmt.Sprintf("demo infrastructure setup failed: %v", err))
	}

	if err := infrastructure.InstallDemoHelmChart(ctx, kubeconfigPath, remoteKubeconfigPath, fleetOpts, demoOpts, os.Stdout); err != nil {
		fail(fmt.Sprintf("helm install failed: %v", err))
	}
}

func fail(message string) {
	fmt.Fprintf(os.Stderr, "❌ %s\n", message)
	os.Exit(1)
}
