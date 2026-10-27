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
	"strconv"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

type demoCLIConfig struct {
	mode                     infrastructure.DemoMode
	clusterName              string
	remoteClusterName        string
	kubeconfigPath           string
	gatewayType              registry.MCPGatewayType
	gatewayVersions          infrastructure.FleetGatewayVersionOverrides
	mcpGatewayNamespace      string
	remoteMCPServerNamespace string
	spokeWorkers             int
	autonomous               bool
	demoOpts                 infrastructure.DemoHelmOptions
}

// main parses demo options, provisions local or fleet infrastructure, and installs
// the operator-backed Kubernaut application.
func main() {
	config := parseDemoFlags()
	ctx := context.Background()
	fleetOpts, remoteKubeconfigPath, operatorExposure, err := setupDemoInfrastructure(ctx, config)
	if err != nil {
		fail(fmt.Sprintf("demo infrastructure setup failed: %v", err))
	}

	config.demoOpts.Exposure = operatorExposure
	if err := infrastructure.InstallDemoOperatorApplication(
		ctx,
		config.kubeconfigPath,
		remoteKubeconfigPath,
		config.clusterName,
		fleetOpts,
		config.demoOpts,
		os.Stdout,
	); err != nil {
		fail(fmt.Sprintf("operator deployment failed: %v", err))
	}
}

type demoFlagValues struct {
	mode, clusterName, remoteClusterName, gatewayType            *string
	dependencyOverrides, envoyGatewayVersion, agentRouterVersion *string
	mcpGatewayNamespace, remoteMCPServerNamespace                *string
	llmProvider, llmModel, llmEndpoint, llmCredentialsFile       *string
	llmReasoningEnabled, llmReasoningEffort                      *string
	spPolicyFile, aaPolicyFile, imageTag, imageRepository        *string
	vertexProject, vertexLocation                                *string
	spokeWorkers                                                 *int
	autonomous                                                   *bool
}

// registerDemoFlags registers the command-line options accepted by setup-demo-infra.
func registerDemoFlags() demoFlagValues {
	values := demoFlagValues{
		mode:                     flag.String("mode", string(infrastructure.DemoModeLocal), "demo mode: local or fleet"),
		clusterName:              flag.String("cluster-name", "", "Kind cluster name (default: kubernaut-demo for local, kubernaut-hub for fleet)"),
		remoteClusterName:        flag.String("remote-cluster-name", "kubernaut-remote-cluster", "fleet spoke Kind cluster name"),
		gatewayType:              flag.String("gateway-type", string(registry.GatewayEAIGW), "fleet MCP Gateway implementation: eaigw or kuadrant"),
		dependencyOverrides:      flag.String("dependency-overrides-file", "", "optional YAML file with demo dependency overrides"),
		envoyGatewayVersion:      flag.String("envoy-gateway-version", "", "optional Envoy Gateway version override for the EAIGW demo"),
		agentRouterVersion:       flag.String("agent-router-version", "", "optional Agent Router/AI Gateway version override for the EAIGW demo"),
		mcpGatewayNamespace:      flag.String("mcp-gateway-namespace", infrastructure.DefaultMCPGatewayNamespace, "fleet MCP Gateway and hub kube-mcp-server namespace"),
		remoteMCPServerNamespace: flag.String("remote-mcp-server-namespace", infrastructure.DefaultRemoteMCPServerNamespace, "fleet remote kube-mcp-server namespace"),
		spokeWorkers:             flag.Int("spoke-workers", 0, "number of extra worker nodes for the fleet spoke cluster"),
		autonomous:               flag.Bool("autonomous", false, "enable Gateway-driven autonomous remediation"),
		llmProvider:              flag.String("llm-provider", "", "required LLM provider"),
		llmModel:                 flag.String("llm-model", "", "required LLM model"),
		llmEndpoint:              flag.String("llm-endpoint", "", "required except for vertex_ai"),
		llmCredentialsFile:       flag.String("llm-credentials-file", "", "required file containing the raw LLM credential"),
		llmReasoningEnabled:      flag.String("llm-reasoning-enabled", "", "optional reasoning enablement override: true or false"),
		llmReasoningEffort:       flag.String("llm-reasoning-effort", "", "optional reasoning effort override"),
		spPolicyFile:             flag.String("sp-policy-file", "", "optional SignalProcessing Rego policy file"),
		aaPolicyFile:             flag.String("aa-policy-file", "", "optional AIAnalysis Rego policy file"),
		imageTag:                 flag.String("image-tag", "", "optional shared Kubernaut image tag override"),
		imageRepository:          flag.String("image-repository", "", "optional Kubernaut image repository override (for example, quay.io/jordigilh or localhost/kubernaut)"),
		vertexProject:            flag.String("vertex-project", "", "required with -llm-provider=vertex_ai"),
		vertexLocation:           flag.String("vertex-location", "", "required with -llm-provider=vertex_ai"),
	}
	if flag.Lookup("kubeconfig") == nil {
		flag.String("kubeconfig", "", "path to write the cluster kubeconfig")
	}
	return values
}

// parseDemoFlags validates command-line options and resolves the effective demo config.
func parseDemoFlags() demoCLIConfig {
	flags := registerDemoFlags()
	flag.Parse()
	fileOverrides, dependencyErr := infrastructure.LoadDemoDependencyOverrides(*flags.dependencyOverrides)
	if dependencyErr != nil {
		fail(dependencyErr.Error())
	}
	gatewayVersions := infrastructure.MergeFleetGatewayVersionOverrides(fileOverrides.FleetGatewayVersions(), infrastructure.FleetGatewayVersionOverrides{
		EnvoyGatewayVersion: *flags.envoyGatewayVersion,
		AgentRouterVersion:  *flags.agentRouterVersion,
	})
	reasoningValue, reasoningSet, parseErr := parseReasoningEnabled(*flags.llmReasoningEnabled)
	if parseErr != nil {
		fail(parseErr.Error())
	}
	var reasoningEnabled *bool
	if reasoningSet {
		reasoningEnabled = &reasoningValue
	}
	mode := infrastructure.DemoMode(*flags.mode)
	if mode != infrastructure.DemoModeLocal && mode != infrastructure.DemoModeFleet {
		fail(fmt.Sprintf("invalid -mode %q; must be one of: local, fleet", *flags.mode))
	}
	clusterName := defaultClusterName(*flags.clusterName, mode == infrastructure.DemoModeFleet)
	kubeconfigPath := flag.Lookup("kubeconfig").Value.String()
	if kubeconfigPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fail(fmt.Sprintf("failed to resolve home directory: %v", err))
		}
		kubeconfigPath = filepath.Join(homeDir, ".kube", clusterName+"-config")
	}

	gatewayType := registry.MCPGatewayType(*flags.gatewayType)
	if !registry.SupportedGateways[gatewayType] {
		fail(fmt.Sprintf("invalid -gateway-type %q; must be one of: eaigw, kuadrant", *flags.gatewayType))
	}
	demoOpts := infrastructure.DemoHelmOptions{
		Mode:                mode,
		Autonomous:          *flags.autonomous,
		LLMProvider:         *flags.llmProvider,
		LLMModel:            *flags.llmModel,
		LLMEndpoint:         *flags.llmEndpoint,
		LLMCredentialsFile:  *flags.llmCredentialsFile,
		LLMReasoningEnabled: reasoningEnabled,
		LLMReasoningEffort:  *flags.llmReasoningEffort,
		SPPolicyFile:        *flags.spPolicyFile,
		AAPolicyFile:        *flags.aaPolicyFile,
		ImageTag:            *flags.imageTag,
		ImageRepository:     *flags.imageRepository,
		VertexProject:       *flags.vertexProject,
		VertexLocation:      *flags.vertexLocation,
		OperatorArtifacts:   fileOverrides.ArtifactOverrides(),
	}
	if err := demoOpts.Validate(); err != nil {
		fail(err.Error())
	}
	if *flags.llmCredentialsFile == "" {
		fail("missing required flag: -llm-credentials-file")
	}
	return demoCLIConfig{
		mode:                     mode,
		clusterName:              clusterName,
		remoteClusterName:        *flags.remoteClusterName,
		kubeconfigPath:           kubeconfigPath,
		gatewayType:              gatewayType,
		gatewayVersions:          gatewayVersions,
		mcpGatewayNamespace:      *flags.mcpGatewayNamespace,
		remoteMCPServerNamespace: *flags.remoteMCPServerNamespace,
		spokeWorkers:             *flags.spokeWorkers,
		autonomous:               *flags.autonomous,
		demoOpts:                 demoOpts,
	}
}

// setupDemoInfrastructure provisions the selected local or fleet prerequisite topology.
func setupDemoInfrastructure(ctx context.Context, config demoCLIConfig) (*infrastructure.FleetHelmOptions, string, *infrastructure.OperatorExposurePlan, error) {
	var fleetOpts *infrastructure.FleetHelmOptions
	var remoteKubeconfigPath string
	var operatorExposure *infrastructure.OperatorExposurePlan
	var err error
	if config.mode == infrastructure.DemoModeFleet {
		fleetOpts, remoteKubeconfigPath, err = infrastructure.SetupFleetCoreInfrastructureWithGateway(
			ctx, config.clusterName, config.remoteClusterName, config.kubeconfigPath,
			infrastructure.FleetCoreDemoOptions{
				Autonomous:               config.autonomous,
				GatewayType:              config.gatewayType,
				GatewayVersions:          config.gatewayVersions,
				MCPGatewayNamespace:      config.mcpGatewayNamespace,
				RemoteMCPServerNamespace: config.remoteMCPServerNamespace,
				SpokeWorkers:             config.spokeWorkers,
				LLMCredentialsFile:       config.demoOpts.LLMCredentialsFile,
			}, os.Stdout)
		if fleetOpts != nil {
			operatorExposure = fleetOpts.OperatorExposure
		}
	} else {
		operatorExposure, err = setupLocalDemoInfrastructure(ctx, config.clusterName, config.kubeconfigPath, config.autonomous)
	}
	return fleetOpts, remoteKubeconfigPath, operatorExposure, err
}

func parseReasoningEnabled(raw string) (bool, bool, error) {
	if raw == "" {
		return false, false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, false, fmt.Errorf("invalid -llm-reasoning-enabled %q; use true or false", raw)
	}
	return value, true, nil
}

func defaultClusterName(current string, fleet bool) string {
	if current != "" {
		return current
	}
	if fleet {
		return "kubernaut-hub"
	}
	return "kubernaut-demo"
}

func setupLocalDemoInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, autonomous bool) (*infrastructure.OperatorExposurePlan, error) {
	exposure, err := infrastructure.OperatorExposurePlanForCluster(ctx, clusterName, "kubernaut-system", "localhost", infrastructure.DemoExposureComponents(autonomous))
	if err != nil {
		return nil, err
	}
	if err := infrastructure.CreateKindClusterWithExtraMountsAndPortMappings(ctx, clusterName, kubeconfigPath, "test/infrastructure/kind-fullpipeline-config.yaml", nil, infrastructure.OperatorPortMappings(exposure), os.Stdout); err != nil {
		return nil, err
	}
	if err := infrastructure.CreateTestNamespace(ctx, "kubernaut-system", kubeconfigPath, os.Stdout); err != nil {
		return nil, err
	}
	if err := infrastructure.SetupOIDCInfrastructure(ctx, kubeconfigPath, os.Stdout); err != nil {
		return nil, err
	}
	if err := infrastructure.SetupDemoMonitoringInfrastructure(ctx, kubeconfigPath, os.Stdout); err != nil {
		return nil, err
	}
	return &exposure, nil
}

func fail(message string) {
	fmt.Fprintf(os.Stderr, "❌ %s\n", message)
	os.Exit(1)
}
