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
	operatorImage, operatorImageRepository, operatorImageTag     *string
	vertexProject, vertexLocation                                *string
	spokeWorkers                                                 *int
	autonomous                                                   *bool
}

// registerDemoFlags registers the command-line options accepted by setup-demo-infra.
func registerDemoFlags(fs *flag.FlagSet) demoFlagValues {
	values := demoFlagValues{
		mode:                     fs.String("mode", string(infrastructure.DemoModeLocal), "demo mode: local or fleet"),
		clusterName:              fs.String("cluster-name", "", "Kind cluster name (default: kubernaut-demo for local, kubernaut-hub for fleet)"),
		remoteClusterName:        fs.String("remote-cluster-name", "kubernaut-remote-cluster", "fleet spoke Kind cluster name"),
		gatewayType:              fs.String("gateway-type", string(registry.GatewayEAIGW), "fleet MCP Gateway implementation: eaigw or kuadrant"),
		dependencyOverrides:      fs.String("dependency-overrides-file", "", "optional YAML file with demo dependency overrides"),
		envoyGatewayVersion:      fs.String("envoy-gateway-version", "", "optional Envoy Gateway version override for the EAIGW demo"),
		agentRouterVersion:       fs.String("agent-router-version", "", "optional Agent Router/AI Gateway version override for the EAIGW demo"),
		mcpGatewayNamespace:      fs.String("mcp-gateway-namespace", infrastructure.DefaultMCPGatewayNamespace, "fleet MCP Gateway and hub kube-mcp-server namespace"),
		remoteMCPServerNamespace: fs.String("remote-mcp-server-namespace", infrastructure.DefaultRemoteMCPServerNamespace, "fleet remote kube-mcp-server namespace"),
		spokeWorkers:             fs.Int("spoke-workers", 0, "number of extra worker nodes for the fleet spoke cluster"),
		autonomous:               fs.Bool("autonomous", false, "enable Gateway-driven autonomous remediation"),
		llmProvider:              fs.String("llm-provider", "", "required LLM provider"),
		llmModel:                 fs.String("llm-model", "", "required LLM model"),
		llmEndpoint:              fs.String("llm-endpoint", "", "required except for vertex_ai"),
		llmCredentialsFile:       fs.String("llm-credentials-file", "", "required file containing the raw LLM credential"),
		llmReasoningEnabled:      fs.String("llm-reasoning-enabled", "", "optional reasoning enablement override: true or false"),
		llmReasoningEffort:       fs.String("llm-reasoning-effort", "", "optional reasoning effort override"),
		spPolicyFile:             fs.String("sp-policy-file", "", "optional SignalProcessing Rego policy file"),
		aaPolicyFile:             fs.String("aa-policy-file", "", "optional AIAnalysis Rego policy file"),
		imageTag:                 fs.String("image-tag", "", "optional shared Kubernaut image tag override"),
		imageRepository:          fs.String("image-repository", "", "optional Kubernaut image repository override (for example, quay.io/jordigilh or localhost/kubernaut)"),
		operatorImage:            fs.String("operator-image", "", "optional full operator image reference override"),
		operatorImageRepository:  fs.String("operator-image-repository", "", "optional operator image repository override"),
		operatorImageTag:         fs.String("operator-image-tag", "", "optional operator image tag override"),
		vertexProject:            fs.String("vertex-project", "", "required with -llm-provider=vertex_ai"),
		vertexLocation:           fs.String("vertex-location", "", "required with -llm-provider=vertex_ai"),
	}
	if fs.Lookup("kubeconfig") == nil {
		fs.String("kubeconfig", "", "path to write the cluster kubeconfig")
	}
	return values
}

// parseDemoFlags validates command-line options and resolves the effective demo config.
func parseDemoFlags() demoCLIConfig {
	flags := registerDemoFlags(flag.CommandLine)
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
	operatorArtifacts := infrastructure.MergeDemoArtifactOverrides(
		fileOverrides.ArtifactOverrides(),
		infrastructure.DemoArtifactOverrides{
			OperatorImage:           *flags.operatorImage,
			OperatorImageRepository: *flags.operatorImageRepository,
			OperatorImageTag:        *flags.operatorImageTag,
		},
	)
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
		OperatorArtifacts:   operatorArtifacts,
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
	const operatorNamespace = "kubernaut-system"
	exposure, err := infrastructure.OperatorExposurePlanForCluster(ctx, clusterName, operatorNamespace, "localhost", infrastructure.DemoExposureComponents(autonomous))
	if err != nil {
		return nil, err
	}
	if err := infrastructure.CreateKindClusterWithExtraMountsAndPortMappings(ctx, clusterName, kubeconfigPath, "test/infrastructure/kind-fullpipeline-config.yaml", nil, infrastructure.OperatorPortMappings(exposure), os.Stdout); err != nil {
		return nil, err
	}
	if err := infrastructure.CreateTestNamespace(ctx, operatorNamespace, kubeconfigPath, os.Stdout); err != nil {
		return nil, err
	}
	if err := infrastructure.EnsureOperatorInternalCA(ctx, kubeconfigPath, operatorNamespace, os.Stdout); err != nil {
		return nil, fmt.Errorf("operator internal CA preflight failed: %w", err)
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
