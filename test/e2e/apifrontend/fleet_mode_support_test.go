package e2e_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	kinfra "github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

const (
	fleetAFDefaultClusterName = "apifrontend-fleet-e2e"
	fleetAFKeycloakHostPort   = 30557
	fleetAFPrometheusURL      = "http://localhost:9190"
	fleetAFLocalPortOffset    = 10000
)

var (
	fleetAFClusterName    = getEnvOrDefault("AF_E2E_FLEET_CLUSTER_NAME", fleetAFDefaultClusterName)
	fleetAFKubeconfigPath string
	fleetAFBaseURL        string
	fleetAFHTTPClient     *http.Client
	fleetAFK8sClient      crclient.Client
	fleetAFEnabled        bool
)

type afE2ESuiteSetup struct {
	LocalKubeconfigPath string            `json:"local_kubeconfig_path"`
	LocalCertDir        string            `json:"local_cert_dir"`
	FleetKubeconfigPath string            `json:"fleet_kubeconfig_path,omitempty"`
	FleetCertDir        string            `json:"fleet_cert_dir,omitempty"`
	FleetClusterName    string            `json:"fleet_cluster_name"`
	LocalHostPortOffset int               `json:"local_host_port_offset"`
	PersonaTokens       map[string]string `json:"persona_tokens,omitempty"`
}

type fleetAFToolCall struct {
	Name              string           `yaml:"name"`
	Arguments         map[string]any   `yaml:"arguments"`
	NextToolCall      *fleetAFToolCall `yaml:"next_tool_call,omitempty"`
	FallbackArguments map[string]any   `yaml:"fallback_arguments,omitempty"`
}

type fleetAFScenarioSelector struct {
	Name           string           `yaml:"name"`
	Keywords       []string         `yaml:"keywords"`
	MatchLastOnly  bool             `yaml:"match_last_only"`
	RepeatToolCall bool             `yaml:"repeat_tool_call,omitempty"`
	ToolCall       fleetAFToolCall  `yaml:"tool_call"`
	NextToolCall   *fleetAFToolCall `yaml:"next_tool_call,omitempty"`
}

func prepareFleetAFFixtures(ctx context.Context, kubeconfigPath string) error {
	client, err := newFleetAFSetupClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("build Fleet AF fixture client: %w", err)
	}

	if err := prepareSeverityAFFixtures(ctx, client); err != nil {
		return err
	}

	fixtures := map[string][]string{
		e2eNamespace:                     nil,
		"af-investigate-e2e":             {"af-investigate-target"},
		"af-structured-decision-e2e":     nil,
		"fleet-af-hub-triage":            {"af-hub-triage-target"},
		"fleet-unregistered-cluster-e2e": {"fleet-unregistered-target"},
		"fleet-session-active-e2e":       {"fleet-session-active-target"},
	}
	for namespace, deployments := range fixtures {
		if err := kinfra.EnsureManagedNamespace(ctx, client, namespace); err != nil {
			return fmt.Errorf("prepare managed Fleet AF namespace %s: %w", namespace, err)
		}
		for _, name := range deployments {
			if err := ensureFleetAFDeployment(ctx, client, namespace, name); err != nil {
				return err
			}
		}
	}

	helpers.EnsureTestPods(ctx, client, "af-investigate-e2e", "af-investigate-target")
	helpers.EnsureTestPods(ctx, client, "af-structured-decision-e2e",
		"structured-decision-target",
		"structured-decision-target-2",
		"structured-decision-target-3",
		"structured-decision-target-4")
	helpers.EnsureTestPods(ctx, client, "fleet-session-active-e2e", "fleet-session-active-target")

	return nil
}

func prepareSeverityAFFixtures(ctx context.Context, client crclient.Client) error {
	fixtures := map[string][]string{
		"sev-tier1-ns":    {"test-firing-target"},
		"sev-tier15-ns":   {"test-pending-target"},
		"sev-tier2-ns":    {"test-inactive-target"},
		"no-data-ns":      {"test-nodata-target"},
		"no-rules-ns":     {"test-norules-target"},
		"sev-userhint-ns": {"test-user-severity-bypass"},
	}
	for namespace, deployments := range fixtures {
		if err := kinfra.EnsureManagedNamespace(ctx, client, namespace); err != nil {
			return fmt.Errorf("prepare managed severity namespace %s: %w", namespace, err)
		}
		for _, name := range deployments {
			if err := ensureFleetAFDeployment(ctx, client, namespace, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func configureFleetAFMockLLM(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	client, err := newFleetAFSetupClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("build Fleet AF mock-LLM client: %w", err)
	}
	return configureAFMockLLM(ctx, kubeconfigPath, client, fleetAFScenarioSelectors(), writer)
}

func configureSeverityAFMockLLM(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	client, err := newFleetAFSetupClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("build severity AF mock-LLM client: %w", err)
	}
	return configureAFMockLLM(ctx, kubeconfigPath, client, severityAFScenarioSelectors(), writer)
}

func configureAFMockLLM(ctx context.Context, kubeconfigPath string, client crclient.Client, newSelectors []fleetAFScenarioSelector, writer io.Writer) error {

	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "mock-llm-config", Namespace: e2eNamespace}}
	if err := client.Get(ctx, crclient.ObjectKeyFromObject(configMap), configMap); err != nil {
		return fmt.Errorf("get standalone AF mock-LLM ConfigMap: %w", err)
	}
	config, err := addAFScenarioSelectors(configMap.Data["config.yaml"], newSelectors)
	if err != nil {
		return fmt.Errorf("add Fleet AF mock-LLM scenarios: %w", err)
	}
	if err := validateFleetAFMockLLMConfig(config); err != nil {
		return err
	}
	configMap.Data["config.yaml"] = config
	if err := client.Update(ctx, configMap); err != nil {
		return fmt.Errorf("update Fleet AF mock-LLM ConfigMap: %w", err)
	}

	for _, args := range [][]string{
		{"--kubeconfig", kubeconfigPath, "-n", e2eNamespace, "rollout", "restart", "deployment/mock-llm"},
		{"--kubeconfig", kubeconfigPath, "-n", e2eNamespace, "rollout", "status", "deployment/mock-llm", "--timeout=120s"},
	} {
		cmd := exec.CommandContext(ctx, "kubectl", args...) //nolint:gosec // G204: test infrastructure
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("restart Fleet AF mock-LLM with test scenarios: %w", err)
		}
	}
	return nil
}

func addFleetAFScenarioSelectors(config string) (string, error) {
	return addAFScenarioSelectors(config, fleetAFScenarioSelectors())
}

func addAFScenarioSelectors(config string, newSelectors []fleetAFScenarioSelector) (string, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(config), &document); err != nil {
		return "", fmt.Errorf("parse mock-LLM scenarios: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return "", fmt.Errorf("mock-LLM config must have a top-level mapping")
	}
	root := document.Content[0]
	var selectors *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "scenario_selectors" {
			selectors = root.Content[i+1]
			break
		}
	}
	if selectors == nil || selectors.Kind != yaml.SequenceNode {
		return "", fmt.Errorf("mock-LLM config must contain scenario_selectors")
	}

	selectorNodes := make([]*yaml.Node, 0, len(newSelectors)+len(selectors.Content))
	newSelectorNames := make(map[string]struct{}, len(newSelectors))
	for _, selector := range newSelectors {
		newSelectorNames[selector.Name] = struct{}{}
		encoded, err := yaml.Marshal(selector)
		if err != nil {
			return "", fmt.Errorf("marshal mock-LLM selector %q: %w", selector.Name, err)
		}
		var selectorDocument yaml.Node
		if err := yaml.Unmarshal(encoded, &selectorDocument); err != nil {
			return "", fmt.Errorf("parse mock-LLM selector %q: %w", selector.Name, err)
		}
		if len(selectorDocument.Content) != 1 {
			return "", fmt.Errorf("mock-LLM selector %q did not produce one YAML node", selector.Name)
		}
		selectorNodes = append(selectorNodes, selectorDocument.Content[0])
	}

	// Retained-cluster runs may invoke this setup repeatedly. Replace the
	// previous copies of our selectors and remove any older duplicate names
	// before handing the ConfigMap back to Mock LLM's strict YAML validator.
	seenNames := make(map[string]struct{}, len(selectors.Content))
	uniqueExisting := make([]*yaml.Node, 0, len(selectors.Content))
	for _, selector := range selectors.Content {
		name := ""
		if selector.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(selector.Content); i += 2 {
				if selector.Content[i].Value == "name" {
					name = selector.Content[i+1].Value
					break
				}
			}
		}
		if name != "" {
			if _, replaces := newSelectorNames[name]; replaces {
				continue
			}
			if _, duplicate := seenNames[name]; duplicate {
				continue
			}
			seenNames[name] = struct{}{}
		}
		uniqueExisting = append(uniqueExisting, selector)
	}
	selectors.Content = append(selectorNodes, uniqueExisting...)

	encoded, err := yaml.Marshal(&document)
	if err != nil {
		return "", fmt.Errorf("marshal Fleet AF mock-LLM config: %w", err)
	}
	return string(encoded), nil
}

func fleetAFScenarioSelectors() []fleetAFScenarioSelector {
	selectors := []fleetAFScenarioSelector{
		fleetAFInvestigateSelector("fleet-af-progressive-investigate", "fleet progressive investigate", "af-investigate-e2e", "af-investigate-target"),
		fleetAFInvestigateSelector("fleet-af-structured-ground-1", "fleet structured grounding one", "af-structured-decision-e2e", "structured-decision-target"),
		fleetAFInvestigateSelector("fleet-af-structured-ground-2", "fleet structured grounding two", "af-structured-decision-e2e", "structured-decision-target-2"),
		fleetAFInvestigateSelector("fleet-af-structured-ground-3", "fleet structured grounding three", "af-structured-decision-e2e", "structured-decision-target-3"),
		fleetAFInvestigateSelector("fleet-af-structured-ground-4", "fleet structured grounding four", "af-structured-decision-e2e", "structured-decision-target-4"),
		fleetAFStructuredDecisionSelector(),
		fleetAFInvestigateSelector("fleet-af-session-active-grounding", "fleet session active investigate", "fleet-session-active-e2e", "fleet-session-active-target"),
		fleetAFUnregisteredClusterSelector(),
		fleetAFHubTriageSelector(),
	}

	severityFixtures := []struct {
		number    string
		namespace string
		name      string
	}{
		{"1", "sev-tier1-ns", "test-firing-target"},
		{"15", "sev-tier15-ns", "test-pending-target"},
		{"2", "sev-tier2-ns", "test-inactive-target"},
		{"25", "no-data-ns", "test-nodata-target"},
		{"5", "no-rules-ns", "test-norules-target"},
		{"6", "sev-userhint-ns", "test-user-severity-bypass"},
	}
	for _, fixture := range severityFixtures {
		selectors = append(selectors, fleetAFRemediateSelector(
			"fleet-af-severity-tier-"+fixture.number,
			"fleet e2e severity tier "+fixture.number,
			fixture.namespace,
			fixture.name,
		))
	}
	return selectors
}

func severityAFScenarioSelectors() []fleetAFScenarioSelector {
	fixtures := []struct {
		number    string
		namespace string
		name      string
	}{
		{"1", "sev-tier1-ns", "test-firing-target"},
		{"15", "sev-tier15-ns", "test-pending-target"},
		{"2", "sev-tier2-ns", "test-inactive-target"},
		{"25", "no-data-ns", "test-nodata-target"},
		{"5", "no-rules-ns", "test-norules-target"},
		{"6", "sev-userhint-ns", "test-user-severity-bypass"},
	}
	selectors := make([]fleetAFScenarioSelector, 0, len(fixtures))
	for _, fixture := range fixtures {
		selector := fleetAFRemediateSelector(
			"af-severity-tier-"+fixture.number,
			"Create a remediation request for deployment "+fixture.name+" in "+fixture.namespace+" namespace",
			fixture.namespace,
			fixture.name,
		)
		delete(selector.ToolCall.Arguments, "cluster_id")
		selectors = append(selectors, selector)
	}
	return selectors
}

func fleetAFInvestigateSelector(name, keyword, namespace, target string) fleetAFScenarioSelector {
	return fleetAFScenarioSelector{
		Name:          name,
		Keywords:      []string{keyword},
		MatchLastOnly: true,
		ToolCall: fleetAFToolCall{
			Name: "kubernaut_investigate",
			Arguments: map[string]any{
				"api_version": "v1",
				"namespace":   namespace,
				"name":        target,
				"kind":        "Pod",
				"cluster_id":  "hub",
			},
		},
	}
}

func fleetAFRemediateSelector(name, keyword, namespace, target string) fleetAFScenarioSelector {
	return fleetAFScenarioSelector{
		Name:          name,
		Keywords:      []string{keyword},
		MatchLastOnly: true,
		ToolCall: fleetAFToolCall{
			Name: "kubernaut_remediate",
			Arguments: map[string]any{
				"namespace":   namespace,
				"kind":        "Deployment",
				"name":        target,
				"api_version": "apps/v1",
				"cluster_id":  "hub",
				"description": "Fleet AF severity-tier E2E",
			},
		},
	}
}

func fleetAFStructuredDecisionSelector() fleetAFScenarioSelector {
	return fleetAFScenarioSelector{
		Name:           "fleet-af-structured-decision",
		Keywords:       []string{"fleet present structured rca decision"},
		MatchLastOnly:  true,
		RepeatToolCall: true,
		ToolCall: fleetAFToolCall{
			Name: "kubernaut_reconnect",
			Arguments: map[string]any{
				"rr_id": "$from_tool:kubernaut_investigate:rr_id",
			},
			FallbackArguments: map[string]any{
				"rr_id": "rr-test",
			},
		},
		NextToolCall: &fleetAFToolCall{
			Name:      "kubernaut_discover_workflows",
			Arguments: map[string]any{"rr_id": "$from_tool:kubernaut_investigate:rr_id"},
			NextToolCall: &fleetAFToolCall{
				Name: "kubernaut_present_decision",
				Arguments: map[string]any{
					"session_id": "sess-fleet-structured-2462",
					"summary":    "Hub-scoped OOMKill investigation with critical severity and high confidence",
					"rca": map[string]any{
						"severity":         "critical",
						"confidence":       0.92,
						"causal_chain":     []string{"Memory leak in worker goroutine", "Container exceeded its memory limit", "Kernel terminated the container"},
						"target":           "Deployment/data-processor in production",
						"tool_calls_count": 19,
						"llm_turns":        17,
					},
					"options": []map[string]any{
						{
							"workflow_id": "wf-restart-pod", "name": "Restart Pod",
							"description": "Rolling restart of affected deployment pods to recover from OOM state",
							"risk":        "low", "recommended": true,
							"parameters": map[string]string{"namespace": "production", "deployment": "data-processor"},
						},
						{
							"workflow_id": "wf-increase-memory", "name": "Increase Memory Limit",
							"description": "Scale memory limit from 512Mi to 1Gi to prevent future OOMKill events",
							"risk":        "medium", "parameters": map[string]string{"new_limit": "1Gi"},
						},
						{
							"workflow_id": "wf-rollback", "name": "Rollback Deployment",
							"description": "Roll back to previous stable revision without memory leak",
							"risk":        "low", "ruled_out_reason": "No previous revision available in cluster deployment history",
						},
					},
				},
			},
		},
	}
}

func fleetAFHubTriageSelector() fleetAFScenarioSelector {
	return fleetAFRemediateSelector(
		"fleet-af-hub-triage-2462",
		"fleet e2e hub cluster triage",
		"fleet-af-hub-triage",
		"af-hub-triage-target",
	)
}

func fleetAFUnregisteredClusterSelector() fleetAFScenarioSelector {
	return fleetAFScenarioSelector{
		Name:          "fleet-af-unregistered-cluster-2462",
		Keywords:      []string{"fleet e2e unregistered cluster investigate"},
		MatchLastOnly: true,
		ToolCall: fleetAFToolCall{
			Name: "kubernaut_investigate",
			Arguments: map[string]any{
				"api_version": "apps/v1",
				"namespace":   "fleet-unregistered-cluster-e2e",
				"name":        "fleet-unregistered-target",
				"kind":        "Deployment",
				"cluster_id":  "unregistered-cluster-2462",
			},
		},
	}
}

func newFleetAFSetupClient(kubeconfigPath string) (crclient.Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("build REST config: %w", err)
	}
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register core Kubernetes API: %w", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register apps Kubernetes API: %w", err)
	}
	client, err := crclient.New(config, crclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	return client, nil
}

func ensureFleetAFDeployment(ctx context.Context, client crclient.Client, namespace, name string) error {
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "pause", Image: "registry.k8s.io/pause:3.10"}}},
			},
		},
	}
	if err := client.Create(ctx, deployment); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create test Deployment %s/%s: %w", namespace, name, err)
	}
	return nil
}

func waitForFleetAFPrometheusRule(ctx context.Context, ruleName string, state kinfra.PrometheusRuleState) error {
	return kinfra.WaitForPrometheusRuleState(ctx, fleetAFPrometheusURL, ruleName, state, 90*time.Second)
}

func fleetAFIsolatedLocalPortOffset() (int, error) {
	baseOffset := 0
	if raw := strings.TrimSpace(os.Getenv("AF_E2E_HOST_PORT_OFFSET")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid AF_E2E_HOST_PORT_OFFSET %q: %w", raw, err)
		}
		if parsed < 0 {
			return 0, fmt.Errorf("AF_E2E_HOST_PORT_OFFSET must be non-negative")
		}
		baseOffset = parsed
	}
	offset := baseOffset + fleetAFLocalPortOffset
	if offset+30443 >= 65536 {
		return 0, fmt.Errorf("isolated AF host-port offset %d exceeds the Kind host-port range", offset)
	}
	return offset, nil
}

func verifyFleetAFHubOnlyTopology(ctx context.Context, localClusterName, fleetClusterName string) error {
	clusters, err := kindClusterNames(ctx)
	if err != nil {
		return err
	}
	if _, ok := clusters[localClusterName]; !ok {
		return fmt.Errorf("local AF Kind cluster %q was not created", localClusterName)
	}
	if _, ok := clusters[fleetClusterName]; !ok {
		return fmt.Errorf("fleet AF Kind cluster %q was not created", fleetClusterName)
	}
	if _, ok := clusters[fleetClusterName+"-remote"]; ok {
		return fmt.Errorf("hub-only Fleet AF setup unexpectedly created remote cluster %q", fleetClusterName+"-remote")
	}
	return nil
}

func kindClusterExists(ctx context.Context, clusterName string) bool {
	clusters, err := kindClusterNames(ctx)
	if err != nil {
		return false
	}
	_, exists := clusters[clusterName]
	return exists
}

func kindClusterNames(ctx context.Context) (map[string]struct{}, error) {
	cmd := exec.CommandContext(ctx, "kind", "get", "clusters") //nolint:gosec // G204: test infrastructure
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list Kind clusters: %w", err)
	}

	clusters := make(map[string]struct{})
	for _, cluster := range strings.Fields(string(output)) {
		clusters[cluster] = struct{}{}
	}
	return clusters, nil
}

func validateFleetAFMockLLMConfig(config string) error {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(config), &document); err != nil {
		return fmt.Errorf("validate mock-LLM config YAML: %w", err)
	}
	if len(document.Content) != 1 {
		return fmt.Errorf("mock-LLM config must contain one YAML document")
	}
	return nil
}

func fleetAFUniqueTaskID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
