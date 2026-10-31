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

package infrastructure

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const (
	demoHostPortOffsetEnv = "KUBERNAUT_DEMO_HOST_PORT_OFFSET"
	// operatorRevision is the only operator source revision accepted by the
	// #2492 qualification profile. Release workflows may replace it with an
	// explicitly supplied candidate manifest, but never with a mutable latest.
	operatorRevision             = "9dc9394fd1844be4f4d4adb89db9a881dbc066ed"
	operatorRepository           = "jordigilh/kubernaut-operator"
	applicationNamespace         = "kubernaut-system"
	operatorNamespace            = "kubernaut-operator-system"
	operatorCRName               = "kubernaut"
	operatorCRAPIVersion         = "kubernaut.ai/v1alpha2"
	operatorOwnerLabel           = "upstream-2492"
	demoOperatorChartOCI         = "oci://quay.io/kubernaut-ai/kubernaut-operator@sha256:8c28efdef6c2f13650146487e1254df8cf6dafc579046ac73d9d24da64987814"
	demoOperatorImageRepository  = "quay.io/kubernaut-ai/kubernaut-operator-image"
	demoOperatorImageRC27        = "quay.io/kubernaut-ai/kubernaut-operator-image@sha256:bb98764927daef2e8e5feb871187799840174ca1ec35cc991247e94f34931c3a"
	demoDependencyChartOCI       = "oci://quay.io/kubernaut-ai/kubernaut-development-dependencies@sha256:53a3155522cab4d743d304badab45863d2b099edbcc30e7a48861a2f5c9bcb2d"
	demoDependencyChartVersion   = "0.1.1-run.4"
	demoReleaseVersionRC27       = "1.6.0-rc27"
	operatorDataStorageOperand   = "datastorage"
	operatorAPIFrontendOperand   = "apifrontend"
	operatorRuntimeTempMountPath = "/tmp"
)

// operatorOperandImageKeys is the exact image contract exercised by the
// operator Kind harness. Keep the list explicit: silently adding a fallback
// image would make a qualification run non-reproducible.
var operatorOperandImageKeys = []string{
	"gateway",
	operatorDataStorageOperand,
	"aianalysis",
	"signalprocessing",
	"remediationorchestrator",
	"workflowexecution",
	"effectivenessmonitor",
	"notification",
	"kubernautagent",
	"authwebhook",
	operatorAPIFrontendOperand,
	"db-migrate",
	"console",
	"fleetmetadatacache",
	"init-ubi-minimal",
	"oauth2-proxy",
}

var operatorRelatedImageKeys = map[string]string{
	"gateway":                  "GATEWAY",
	operatorDataStorageOperand: "DATA_STORAGE",
	"aianalysis":               "AIANALYSIS",
	"signalprocessing":         "SIGNALPROCESSING",
	"remediationorchestrator":  "REMEDIATIONORCHESTRATOR",
	"workflowexecution":        "WORKFLOWEXECUTION",
	"effectivenessmonitor":     "EFFECTIVENESSMONITOR",
	"notification":             "NOTIFICATION",
	"kubernautagent":           "KUBERNAUT_AGENT",
	"authwebhook":              "AUTHWEBHOOK",
	operatorAPIFrontendOperand: "API_FRONTEND",
	"db-migrate":               "DB_MIGRATE",
	"console":                  "CONSOLE",
	"fleetmetadatacache":       "FLEETMETADATACACHE",
	"init-ubi-minimal":         "INIT_UBI_MINIMAL",
	"oauth2-proxy":             "OAUTH2_PROXY",
}

// OperatorArtifactProvenance is the machine-readable identity record for one
// operator qualification input. It deliberately contains both requested and
// resolved source references so a branch/ref lookup cannot be mistaken for the
// bytes that were built.
type OperatorArtifactProvenance struct {
	Repository    string `json:"repository"`
	RequestedRef  string `json:"requestedRef"`
	ResolvedSHA   string `json:"resolvedSHA"`
	ImageRef      string `json:"imageRef"`
	ImageDigest   string `json:"imageDigest"`
	ChartRevision string `json:"chartRevision"`
	UpstreamSHA   string `json:"upstreamSHA"`
	WorkflowURL   string `json:"workflowURL"`
	Profile       string `json:"profile"`
}

// BuildOperatorProvenanceJSON serializes the exact source/build identity used
// by a qualification lane.
func BuildOperatorProvenanceJSON(provenance OperatorArtifactProvenance) ([]byte, error) {
	if provenance.Repository == "" || provenance.RequestedRef == "" || provenance.ResolvedSHA == "" {
		return nil, errors.New("operator provenance requires repository, requestedRef, and resolvedSHA")
	}
	return json.MarshalIndent(provenance, "", "  ")
}

// DemoHostPortOffset returns the optional host-only port offset used by the
// local demo. Kind NodePorts and in-cluster service ports remain unchanged;
// only host bindings move so an owned demo cannot collide with another team's
// cluster on a shared Podman host.
func DemoHostPortOffset() (int, error) {
	raw := strings.TrimSpace(os.Getenv(demoHostPortOffsetEnv))
	if raw == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", demoHostPortOffsetEnv, raw, err)
	}
	if offset < 0 {
		return 0, fmt.Errorf("%s must be non-negative", demoHostPortOffsetEnv)
	}
	return offset, nil
}

// DemoHostPort returns the host-facing port for a demo endpoint while keeping
// the endpoint's in-cluster NodePort unchanged.
func DemoHostPort(defaultPort int) int {
	offset, err := DemoHostPortOffset()
	if err != nil {
		return defaultPort
	}
	return defaultPort + offset
}

// ValidateOperatorOperandImageMap rejects incomplete maps before the CR is
// applied. The operator's RELATED_IMAGE contract is intentionally all-or-none
// for this qualification profile.
func ValidateOperatorOperandImageMap(images map[string]string) error {
	for _, name := range operatorOperandImageKeys {
		if strings.TrimSpace(images[name]) == "" {
			return fmt.Errorf("missing operator operand image %q", name)
		}
	}
	return nil
}

// OperatorChartInstallOptions describes the operator-only Helm bootstrap.
type OperatorChartInstallOptions struct {
	Namespace     string
	ChartPath     string
	ChartVersion  string
	OperatorRef   string
	RelatedImages map[string]string
}

// BuildOperatorChartArgs builds the operator chart command. The chart creates
// only the operator/CRD bootstrap; relatedImages are consumed later by CR
// reconciliation through RELATED_IMAGE_* environment variables.
func BuildOperatorChartArgs(options OperatorChartInstallOptions) []string {
	args := []string{
		"upgrade", "--install", "kubernaut-operator", options.ChartPath,
		"--namespace", options.Namespace, "--create-namespace",
		"--wait", "--timeout", "10m",
		"--set-string", "webhook.tls.mode=development",
	}
	if strings.TrimSpace(options.ChartVersion) != "" && strings.HasPrefix(options.ChartPath, "oci://") && !strings.Contains(options.ChartPath, "@sha256:") {
		args = append(args, "--version", options.ChartVersion)
	}
	args = append(args, operatorImageHelmArgs(options.OperatorRef)...)
	for _, name := range operatorOperandImageKeys {
		key := operatorRelatedImageKeys[name]
		args = append(args, "--set-string", "relatedImages."+key+"="+options.RelatedImages[name])
	}
	return args
}

// OperatorBootstrapOptions describes the minimal chart-only contract used by
// offline render tests and diagnostics.
type OperatorBootstrapOptions struct {
	Namespace string
	ChartPath string
}

// BuildOperatorBootstrapArgs returns the chart-only bootstrap arguments.
func BuildOperatorBootstrapArgs(options OperatorBootstrapOptions) []string {
	return []string{
		"upgrade", "--install", "kubernaut-operator", options.ChartPath,
		"--namespace", options.Namespace, "--create-namespace", "--wait", "--timeout", "10m",
		"--set-string", "webhook.tls.mode=development",
	}
}

// OperatorDependencyOptions describes the disposable PostgreSQL/Valkey chart.
type OperatorDependencyOptions struct {
	Namespace       string
	ChartPath       string
	ChartVersion    string
	PostgreSQLImage string
	ValkeyImage     string
}

// BuildOperatorDependencyArgs keeps external dependency ownership separate from
// the operator chart and records exact image refs in the command.
func BuildOperatorDependencyArgs(options OperatorDependencyOptions) []string {
	args := []string{
		"upgrade", "--install", "kubernaut-development-dependencies", options.ChartPath,
		"--namespace", options.Namespace, "--create-namespace",
		"--wait", "--timeout", "10m",
		"--set", "postgresql.commandMode=official",
		"--set-json", "postgresql.securityContext.enabled=false",
		"--set-json", "valkey.securityContext.enabled=false",
	}
	if strings.TrimSpace(options.ChartVersion) != "" && strings.HasPrefix(options.ChartPath, "oci://") && !strings.Contains(options.ChartPath, "@sha256:") {
		args = append(args, "--version", options.ChartVersion)
	}
	args = append(args, imageReferenceHelmArgs("postgresql.image", options.PostgreSQLImage)...)
	args = append(args, imageReferenceHelmArgs("valkey.image", options.ValkeyImage)...)
	return args
}

func operatorImageHelmArgs(imageRef string) []string {
	return imageReferenceHelmArgs("image", imageRef)
}

func imageReferenceHelmArgs(prefix, imageRef string) []string {
	if strings.Contains(imageRef, "@sha256:") {
		parts := strings.SplitN(imageRef, "@", 2)
		return []string{"--set-string", prefix + ".repository=" + parts[0], "--set-string", prefix + ".digest=" + parts[1]}
	}
	lastSlash := strings.LastIndex(imageRef, "/")
	lastColon := strings.LastIndex(imageRef, ":")
	if lastColon > lastSlash {
		return []string{
			"--set-string", prefix + ".repository=" + imageRef[:lastColon],
			"--set-string", prefix + ".tag=" + imageRef[lastColon+1:],
			"--set-string", prefix + ".digest=",
		}
	}
	return []string{"--set-string", prefix + ".repository=" + imageRef}
}

// OperatorLLMOptions is the single profile used by the qualification fixture.
type OperatorLLMOptions struct {
	Provider          string
	Model             string
	Endpoint          string
	CredentialsSecret string
	VertexProject     string
	VertexLocation    string
	Reasoning         *OperatorLLMReasoning
}

type OperatorLLMReasoning struct {
	Enabled            bool
	Effort             string
	CapabilityOverride string
}

// OperatorTLSOptions selects the externally prepared manual TLS material used
// by the latest v1alpha2 operator contract.
type OperatorTLSOptions struct {
	Mode                   string
	ManualInternalCASecret string
	ServiceTLSSecretNames  map[string]string
}

type OperatorValkeyTLSOptions struct {
	Enabled              bool
	CASecretName         string
	ClientCertSecretName string
}

// OperatorPolicyOptions contains user-owned policy ConfigMap names. Fixture
// contents are provisioned separately from the dependency chart.
type OperatorPolicyOptions struct {
	AIAnalysis       string
	SignalProcessing string
}

// OperatorMonitoringOptions contains the explicit generic-Kubernetes endpoints.
type OperatorMonitoringOptions struct {
	PrometheusURL   string
	AlertManagerURL string
	TLSCASecretName string
}

// OperatorNetworkPolicyOptions contains the runtime host peers that must be
// admitted to test-exposed operator Services. The operator owns the resulting
// NetworkPolicies; this is an optional generic-Kind qualification setting, not
// a production default.
type OperatorNetworkPolicyOptions struct {
	IngressCIDRs []string
}

// OperatorFleetOptions mirrors the v1alpha2 nested Fleet contract. Read and
// write credentials intentionally remain separate for least privilege.
type OperatorFleetOptions struct {
	Enabled                bool
	GatewayEndpoint        string
	GatewayType            string
	GatewayNamespace       string
	ScopeBackend           string
	ReadCredentialsSecret  string
	WriteCredentialsSecret string
	TokenURL               string
	Scopes                 []string
}

// OperatorCROptions is the application configuration rendered into the
// user-owned Kubernaut CR.
type OperatorCROptions struct {
	Name           string
	Namespace      string
	PostgreSQLHost string
	// PostgreSQLSSLMode is an optional profile override. Leave empty to use
	// the operator default; disposable Kind bootstrap profiles may set
	// "require" when their CA is not part of the operator migration trust
	// bundle.
	PostgreSQLSSLMode                string
	ValkeyHost                       string
	PostgreSQLSecret                 string
	ValkeySecret                     string
	ValkeyTLS                        *OperatorValkeyTLSOptions
	LLM                              OperatorLLMOptions
	TLS                              OperatorTLSOptions
	PolicyConfigMaps                 OperatorPolicyOptions
	Monitoring                       OperatorMonitoringOptions
	NetworkPolicies                  OperatorNetworkPolicyOptions
	Fleet                            *OperatorFleetOptions
	ImageOverrides                   map[string]string
	MaxConcurrentInvestigations      int
	InteractiveRateLimitPerUser      int
	RemediationStabilizationWindow   string
	EffectivenessStabilizationWindow string
	EffectivenessValidityWindow      string
	GatewayEnabled                   bool
	APIFrontendEnabled               bool
	ConsoleEnabled                   bool
	ConsoleAuthSecretName            string
	ConsoleRedirectURL               string
	ConsoleProviderLogoutURL         string
	IssuerURL                        string
	JWKSURL                          string
	Audience                         string
}

type operatorCRManifest struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   operatorMetadata `yaml:"metadata"`
	Spec       operatorCRSpec   `yaml:"spec"`
}

type operatorMetadata struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

type operatorCRSpec struct {
	Image                   operatorImageSpec              `yaml:"image,omitempty"`
	PostgreSQL              operatorPostgreSQLSpec         `yaml:"postgresql"`
	Valkey                  operatorValkeySpec             `yaml:"valkey"`
	AIAnalysis              operatorPolicySpec             `yaml:"aiAnalysis"`
	SignalProcessing        operatorPolicySpec             `yaml:"signalProcessing"`
	LLMProfiles             map[string]operatorLLMProfile  `yaml:"llmProfiles"`
	KubernautAgent          operatorAgentSpec              `yaml:"kubernautAgent"`
	Gateway                 *operatorGatewaySpec           `yaml:"gateway,omitempty"`
	APIFrontend             *operatorAPIFrontendSpec       `yaml:"apiFrontend,omitempty"`
	Console                 *operatorConsoleSpec           `yaml:"console,omitempty"`
	Monitoring              *operatorMonitoringSpec        `yaml:"monitoring,omitempty"`
	NetworkPolicies         *operatorNetworkPoliciesSpec   `yaml:"networkPolicies,omitempty"`
	TLS                     operatorTLSManifest            `yaml:"tls"`
	RemediationOrchestrator *operatorRemediationSpec       `yaml:"remediationOrchestrator,omitempty"`
	WorkflowExecution       *operatorWorkflowExecutionSpec `yaml:"workflowExecution,omitempty"`
	EffectivenessMonitor    *operatorEffectivenessSpec     `yaml:"effectivenessMonitor,omitempty"`
	Notification            operatorNotificationSpec       `yaml:"notification,omitempty"`
	AuthWebhook             operatorAuthWebhookSpec        `yaml:"authWebhook,omitempty"`
	Fleet                   *operatorFleetSpec             `yaml:"fleet,omitempty"`
}

type operatorImageSpec struct {
	PullPolicy string            `yaml:"pullPolicy,omitempty"`
	Overrides  map[string]string `yaml:"overrides,omitempty"`
}

type operatorPostgreSQLSpec struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port,omitempty"`
	SecretName string `yaml:"secretName"`
	SSLMode    string `yaml:"sslMode,omitempty"`
}

type operatorValkeySpec struct {
	Host       string             `yaml:"host"`
	Port       int                `yaml:"port,omitempty"`
	SecretName string             `yaml:"secretName"`
	TLS        *operatorValkeyTLS `yaml:"tls,omitempty"`
}

type operatorValkeyTLS struct {
	Enabled              bool   `yaml:"enabled"`
	CASecretName         string `yaml:"caSecretName,omitempty"`
	ClientCertSecretName string `yaml:"clientCertSecretName,omitempty"`
}

type operatorPolicySpec struct {
	Policy operatorPolicyRef `yaml:"policy"`
}

type operatorPolicyRef struct {
	ConfigMapName string `yaml:"configMapName"`
}

type operatorLLMProfile struct {
	Provider          string                `yaml:"provider"`
	Model             string                `yaml:"model"`
	Endpoint          string                `yaml:"endpoint,omitempty"`
	CredentialsSecret string                `yaml:"credentialsSecretName"`
	VertexProject     string                `yaml:"vertexProject,omitempty"`
	VertexLocation    string                `yaml:"vertexLocation,omitempty"`
	Reasoning         *operatorLLMReasoning `yaml:"reasoning,omitempty"`
}

type operatorLLMReasoning struct {
	Enabled            bool   `yaml:"enabled,omitempty"`
	Effort             string `yaml:"effort,omitempty"`
	CapabilityOverride string `yaml:"capabilityOverride,omitempty"`
}

type operatorAgentSpec struct {
	Investigation     *operatorInvestigationSpec     `yaml:"investigation,omitempty"`
	WorkflowDiscovery *operatorWorkflowDiscoverySpec `yaml:"workflowDiscovery,omitempty"`
	Session           *operatorSessionSpec           `yaml:"session,omitempty"`
	Interactive       *operatorInteractiveSpec       `yaml:"interactive,omitempty"`
	Audit             *operatorAuditSpec             `yaml:"audit,omitempty"`
}

type operatorInvestigationSpec struct {
	LLMProfileRef string `yaml:"llmProfileRef"`
	MaxTurns      int    `yaml:"maxTurns,omitempty"`
}

type operatorWorkflowDiscoverySpec struct {
	LLMProfileRef string `yaml:"llmProfileRef,omitempty"`
}

type operatorSessionSpec struct {
	MaxConcurrentInvestigations int `yaml:"maxConcurrentInvestigations,omitempty"`
}

type operatorInteractiveSpec struct {
	Enabled          bool `yaml:"enabled"`
	RateLimitPerUser int  `yaml:"rateLimitPerUser,omitempty"`
}

type operatorAuditSpec struct {
	Enabled bool `yaml:"enabled"`
}

type operatorGatewaySpec struct {
	Enabled *bool `yaml:"enabled,omitempty"`
}

type operatorAPIFrontendSpec struct {
	Enabled       *bool                  `yaml:"enabled,omitempty"`
	LLMProfileRef string                 `yaml:"llmProfileRef,omitempty"`
	Auth          *operatorAPIAuthSpec   `yaml:"auth,omitempty"`
	RateLimit     *operatorRateLimitSpec `yaml:"rateLimit,omitempty"`
}

type operatorAPIAuthSpec struct {
	IssuerURL  string `yaml:"issuerURL,omitempty"`
	JWKSURL    string `yaml:"jwksURL,omitempty"`
	Audience   string `yaml:"audience,omitempty"`
	OIDCCAFile string `yaml:"oidcCaFile,omitempty"`
}

type operatorRateLimitSpec struct {
	UserRequestsPerSec    int `yaml:"userRequestsPerSec,omitempty"`
	MaxConcurrentSessions int `yaml:"maxConcurrentSessions,omitempty"`
}

type operatorConsoleSpec struct {
	Enabled *bool                    `yaml:"enabled,omitempty"`
	Auth    *operatorConsoleAuthSpec `yaml:"auth,omitempty"`
}

type operatorConsoleAuthSpec struct {
	SecretName        string `yaml:"secretName,omitempty"`
	RedirectURL       string `yaml:"redirectURL,omitempty"`
	ProviderLogoutURL string `yaml:"providerLogoutURL,omitempty"`
}

type operatorMonitoringSpec struct {
	Prometheus   *operatorMonitoringEndpoint `yaml:"prometheus,omitempty"`
	AlertManager *operatorMonitoringEndpoint `yaml:"alertManager,omitempty"`
	TLS          *operatorMonitoringTLS      `yaml:"tls,omitempty"`
}

type operatorMonitoringEndpoint struct {
	Enabled *bool  `yaml:"enabled,omitempty"`
	URL     string `yaml:"url,omitempty"`
}

type operatorMonitoringTLS struct {
	CACertSecretRef *operatorMonitoringCASecretRef `yaml:"caCertSecretRef,omitempty"`
}

type operatorMonitoringCASecretRef struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key,omitempty"`
}

type operatorNetworkPolicyIngress struct {
	IngressCIDRs []string `yaml:"ingressCIDRs,omitempty"`
}

type operatorNetworkPoliciesSpec struct {
	Gateway        *operatorNetworkPolicyIngress `yaml:"gateway,omitempty"`
	APIFrontend    *operatorNetworkPolicyIngress `yaml:"apifrontend,omitempty"`
	DataStorage    *operatorNetworkPolicyIngress `yaml:"datastorage,omitempty"`
	KubernautAgent *operatorNetworkPolicyIngress `yaml:"kubernautAgent,omitempty"`
}

type operatorTLSManifest struct {
	Mode   string                   `yaml:"mode"`
	Manual *operatorManualTLSConfig `yaml:"manual,omitempty"`
}

type operatorManualTLSConfig struct {
	InternalCASecretName  string            `yaml:"internalCASecretName,omitempty"`
	ServiceTLSSecretNames map[string]string `yaml:"serviceTLSSecretNames,omitempty"`
}

type operatorRemediationSpec struct {
	DryRun        bool                            `yaml:"dryRun"`
	Effectiveness operatorEffectivenessAssessment `yaml:"effectivenessAssessment,omitempty"`
}

type operatorEffectivenessAssessment struct {
	StabilizationWindow string `yaml:"stabilizationWindow,omitempty"`
	ValidityWindow      string `yaml:"validityWindow,omitempty"`
}

type operatorWorkflowExecutionSpec struct {
	WorkflowNamespace      string                     `yaml:"workflowNamespace,omitempty"`
	CooldownPeriod         string                     `yaml:"cooldownPeriod,omitempty"`
	RetainFailedExecutions bool                       `yaml:"retainFailedExecutions,omitempty"`
	Fleet                  *operatorWorkflowFleetSpec `yaml:"fleet,omitempty"`
}

type operatorWorkflowFleetSpec struct {
	OAuth2CredentialsSecretRef string `yaml:"oauth2CredentialsSecretRef,omitempty"`
}

type operatorEffectivenessSpec struct {
	Assessment operatorEffectivenessAssessment `yaml:"assessment,omitempty"`
}

type operatorNotificationSpec struct{}
type operatorAuthWebhookSpec struct{}

type operatorFleetSpec struct {
	Enabled    bool                     `yaml:"enabled"`
	MCPGateway operatorFleetGatewaySpec `yaml:"mcpGateway"`
	ScopeCheck operatorFleetScopeSpec   `yaml:"scopeCheck"`
	OAuth2     operatorFleetOAuth2Spec  `yaml:"oauth2"`
}

type operatorFleetGatewaySpec struct {
	Endpoint  string `yaml:"endpoint"`
	Type      string `yaml:"type"`
	Namespace string `yaml:"namespace,omitempty"`
}

type operatorFleetScopeSpec struct {
	Backend string `yaml:"backend"`
}

type operatorFleetOAuth2Spec struct {
	TokenURL             string   `yaml:"tokenURL"`
	CredentialsSecretRef string   `yaml:"credentialsSecretRef"`
	Scopes               []string `yaml:"scopes,omitempty"`
}

// BuildKubernautCRManifest renders the operator's minimal v1alpha2 user
// contract. Defaults intentionally remain absent so the operator's admission
// defaulting owns them; only BYO references, policies, the single LLM profile,
// generic-cluster TLS, and explicit test/demo integrations are rendered.
func BuildKubernautCRManifest(options OperatorCROptions) (string, error) {
	if err := validateOperatorCROptions(&options); err != nil {
		return "", err
	}
	audience := options.Audience
	if audience == "" {
		audience = "kubernaut-apifrontend"
	}

	cr := operatorCRManifest{
		APIVersion: operatorCRAPIVersion,
		Kind:       "Kubernaut",
		Metadata:   operatorMetadata{Name: options.Name, Namespace: options.Namespace},
		Spec:       buildOperatorCRSpec(options, audience),
	}
	if err := applyOperatorCRFeatureOverrides(&cr.Spec, options); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(cr)
	if err != nil {
		return "", fmt.Errorf("marshal Kubernaut CR: %w", err)
	}
	return string(data), nil
}

func validateOperatorCROptions(options *OperatorCROptions) error {
	if options.Name == "" {
		options.Name = operatorCRName
	}
	if options.Namespace == "" {
		options.Namespace = applicationNamespace
	}
	if options.PostgreSQLSecret == "" || options.ValkeySecret == "" {
		return errors.New("kubernaut CR requires PostgreSQL and Valkey Secret names")
	}
	if options.LLM.Provider == "" || options.LLM.Model == "" || options.LLM.CredentialsSecret == "" {
		return errors.New("kubernaut CR requires a complete LLM profile")
	}
	if options.TLS.Mode == "" {
		return errors.New("kubernaut CR requires an explicit TLS mode")
	}
	if options.TLS.Mode != "manual" {
		return fmt.Errorf("kubernaut CR supports only the v1alpha2 manual TLS mode, got %q", options.TLS.Mode)
	}
	if options.Fleet != nil && options.Fleet.Enabled && options.Fleet.WriteCredentialsSecret == "" {
		return errors.New("fleet kubernaut CR requires a write OAuth2 Secret")
	}
	if options.APIFrontendEnabled && options.IssuerURL == "" {
		return errors.New("kubernaut CR requires an API Frontend issuer URL when API Frontend is enabled")
	}
	return nil
}

func buildOperatorCRSpec(options OperatorCROptions, audience string) operatorCRSpec {
	return operatorCRSpec{
		Image:            operatorImageSpec{Overrides: options.ImageOverrides},
		PostgreSQL:       operatorPostgreSQLSpec{Host: options.PostgreSQLHost, SecretName: options.PostgreSQLSecret, SSLMode: options.PostgreSQLSSLMode},
		Valkey:           operatorValkeySpec{Host: options.ValkeyHost, SecretName: options.ValkeySecret, TLS: operatorValkeyTLSManifest(options.ValkeyTLS)},
		AIAnalysis:       operatorPolicySpec{Policy: operatorPolicyRef{ConfigMapName: options.PolicyConfigMaps.AIAnalysis}},
		SignalProcessing: operatorPolicySpec{Policy: operatorPolicyRef{ConfigMapName: options.PolicyConfigMaps.SignalProcessing}},
		LLMProfiles:      map[string]operatorLLMProfile{"primary": {Provider: options.LLM.Provider, Model: options.LLM.Model, Endpoint: options.LLM.Endpoint, CredentialsSecret: options.LLM.CredentialsSecret, VertexProject: options.LLM.VertexProject, VertexLocation: options.LLM.VertexLocation, Reasoning: operatorReasoningManifest(options.LLM.Reasoning)}},
		KubernautAgent:   operatorAgentSpec{},
		Gateway:          operatorGatewaySpecFor(options),
		APIFrontend:      operatorAPIFrontendSpecFor(options, audience),
		Monitoring:       operatorMonitoringSpecFor(options),
		NetworkPolicies:  operatorNetworkPoliciesSpecFor(options),
		TLS:              operatorTLSManifestFor(options),
	}
}

func operatorGatewaySpecFor(options OperatorCROptions) *operatorGatewaySpec {
	if options.GatewayEnabled {
		return nil
	}
	return &operatorGatewaySpec{Enabled: pointerToBool(false)}
}

func operatorAPIFrontendSpecFor(options OperatorCROptions, audience string) *operatorAPIFrontendSpec {
	if !options.APIFrontendEnabled {
		return &operatorAPIFrontendSpec{Enabled: pointerToBool(false)}
	}
	auth := &operatorAPIAuthSpec{IssuerURL: options.IssuerURL, Audience: audience}
	if options.JWKSURL != "" {
		auth.JWKSURL = options.JWKSURL
	}
	return &operatorAPIFrontendSpec{Enabled: pointerToBool(true), Auth: auth}
}

func operatorMonitoringSpecFor(options OperatorCROptions) *operatorMonitoringSpec {
	if options.Monitoring.PrometheusURL == "" && options.Monitoring.AlertManagerURL == "" {
		return nil
	}
	monitoring := &operatorMonitoringSpec{}
	if options.Monitoring.PrometheusURL != "" {
		monitoring.Prometheus = &operatorMonitoringEndpoint{Enabled: pointerToBool(true), URL: options.Monitoring.PrometheusURL}
	}
	if options.Monitoring.AlertManagerURL != "" {
		monitoring.AlertManager = &operatorMonitoringEndpoint{Enabled: pointerToBool(true), URL: options.Monitoring.AlertManagerURL}
	}
	if options.Monitoring.TLSCASecretName != "" {
		monitoring.TLS = &operatorMonitoringTLS{CACertSecretRef: &operatorMonitoringCASecretRef{Name: options.Monitoring.TLSCASecretName, Key: "ca.crt"}}
	}
	return monitoring
}

func operatorNetworkPoliciesSpecFor(options OperatorCROptions) *operatorNetworkPoliciesSpec {
	if len(options.NetworkPolicies.IngressCIDRs) == 0 {
		return nil
	}
	cidrs := options.NetworkPolicies.IngressCIDRs
	return &operatorNetworkPoliciesSpec{
		Gateway:        &operatorNetworkPolicyIngress{IngressCIDRs: append([]string(nil), cidrs...)},
		APIFrontend:    &operatorNetworkPolicyIngress{IngressCIDRs: append([]string(nil), cidrs...)},
		DataStorage:    &operatorNetworkPolicyIngress{IngressCIDRs: append([]string(nil), cidrs...)},
		KubernautAgent: &operatorNetworkPolicyIngress{IngressCIDRs: append([]string(nil), cidrs...)},
	}
}

func operatorTLSManifestFor(options OperatorCROptions) operatorTLSManifest {
	internalCASecret := options.TLS.ManualInternalCASecret
	if internalCASecret == "" {
		internalCASecret = operatorInternalCASecretName
	}
	return operatorTLSManifest{Mode: "manual", Manual: &operatorManualTLSConfig{InternalCASecretName: internalCASecret, ServiceTLSSecretNames: operatorManualTLSSecretNames(options.Fleet != nil && options.Fleet.Enabled, options.TLS.ServiceTLSSecretNames)}}
}

func applyOperatorCRFeatureOverrides(spec *operatorCRSpec, options OperatorCROptions) error {
	if options.InteractiveRateLimitPerUser > 0 {
		spec.KubernautAgent.Interactive = &operatorInteractiveSpec{Enabled: true, RateLimitPerUser: options.InteractiveRateLimitPerUser}
	}
	if options.MaxConcurrentInvestigations > 0 {
		spec.KubernautAgent.Session = &operatorSessionSpec{MaxConcurrentInvestigations: options.MaxConcurrentInvestigations}
	}
	if strings.TrimSpace(options.RemediationStabilizationWindow) != "" {
		spec.RemediationOrchestrator = &operatorRemediationSpec{Effectiveness: operatorEffectivenessAssessment{StabilizationWindow: options.RemediationStabilizationWindow}}
	}
	if strings.TrimSpace(options.EffectivenessStabilizationWindow) != "" || strings.TrimSpace(options.EffectivenessValidityWindow) != "" {
		spec.EffectivenessMonitor = &operatorEffectivenessSpec{Assessment: operatorEffectivenessAssessment{StabilizationWindow: options.EffectivenessStabilizationWindow, ValidityWindow: options.EffectivenessValidityWindow}}
	}
	if err := applyOperatorConsoleSpec(spec, options); err != nil {
		return err
	}
	if options.Fleet != nil && options.Fleet.Enabled {
		spec.Fleet = &operatorFleetSpec{Enabled: true, MCPGateway: operatorFleetGatewaySpec{Endpoint: options.Fleet.GatewayEndpoint, Type: options.Fleet.GatewayType, Namespace: options.Fleet.GatewayNamespace}, ScopeCheck: operatorFleetScopeSpec{Backend: options.Fleet.ScopeBackend}, OAuth2: operatorFleetOAuth2Spec{TokenURL: options.Fleet.TokenURL, CredentialsSecretRef: options.Fleet.ReadCredentialsSecret, Scopes: options.Fleet.Scopes}}
		spec.WorkflowExecution = &operatorWorkflowExecutionSpec{Fleet: &operatorWorkflowFleetSpec{OAuth2CredentialsSecretRef: options.Fleet.WriteCredentialsSecret}}
	}
	return nil
}

func applyOperatorConsoleSpec(spec *operatorCRSpec, options OperatorCROptions) error {
	if !options.ConsoleEnabled {
		return nil
	}
	if options.ConsoleAuthSecretName == "" {
		return errors.New("kubernaut CR requires a Console OIDC Secret when Console is enabled")
	}
	if err := validateOperatorConsoleRedirectURL(options.ConsoleRedirectURL); err != nil {
		return err
	}
	spec.Console = &operatorConsoleSpec{Enabled: pointerToBool(true), Auth: &operatorConsoleAuthSpec{SecretName: options.ConsoleAuthSecretName, RedirectURL: options.ConsoleRedirectURL, ProviderLogoutURL: options.ConsoleProviderLogoutURL}}
	return nil
}

func validateOperatorConsoleRedirectURL(raw string) error {
	if strings.TrimSpace(raw) != raw || raw == "" {
		return errors.New("kubernaut CR requires an HTTPS Console redirect URL when Console is enabled")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.EscapedPath() != "/oauth2/callback" {
		return fmt.Errorf("kubernaut CR requires an HTTPS Console redirect URL ending in /oauth2/callback, got %q", raw)
	}
	return nil
}

func pointerToBool(value bool) *bool {
	return &value
}

func operatorManualTLSSecretNames(includeFleetMetadataCache bool, configured map[string]string) map[string]string {
	names := map[string]string{
		"gateway":        "gateway-tls",
		"datastorage":    "datastorage-tls",
		"kubernautagent": "kubernautagent-tls",
		"apifrontend":    "apifrontend-tls",
		"authwebhook":    "authwebhook-tls",
	}
	if includeFleetMetadataCache {
		names["fleetmetadatacache"] = "fleetmetadatacache-tls"
	}
	for key, value := range configured {
		if strings.TrimSpace(value) != "" {
			names[key] = value
		}
	}
	return names
}

func operatorValkeyTLSManifest(options *OperatorValkeyTLSOptions) *operatorValkeyTLS {
	if options == nil {
		return nil
	}
	return &operatorValkeyTLS{Enabled: options.Enabled, CASecretName: options.CASecretName, ClientCertSecretName: options.ClientCertSecretName}
}

func operatorReasoningManifest(reasoning *OperatorLLMReasoning) *operatorLLMReasoning {
	if reasoning == nil {
		return nil
	}
	return &operatorLLMReasoning{Enabled: reasoning.Enabled, Effort: reasoning.Effort, CapabilityOverride: reasoning.CapabilityOverride}
}

// KindHostIngressCIDR returns the /32 peer used by the Kind node's container
// network gateway. Podman/Docker host-port DNAT enters the node from this
// gateway address; admitting it through the operator's component
// NetworkPolicies is narrower than admitting all external traffic.
func KindHostIngressCIDR(ctx context.Context, clusterName string) (string, error) {
	if clusterName == "" {
		return "", errors.New("kind host ingress CIDR requires a cluster name")
	}
	runtime := containerRuntimeDocker
	if os.Getenv("KIND_EXPERIMENTAL_PROVIDER") == containerRuntimePodman {
		runtime = containerRuntimePodman
	}
	nodeName := clusterName + "-control-plane"
	output, err := exec.CommandContext(ctx, runtime, "exec", nodeName, "ip", "-4", "route", "show", "default").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect Kind node network gateway: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	gateway, err := parseKindIPv4DefaultGateway(string(output))
	if err != nil {
		return "", err
	}
	return gateway + "/32", nil
}

func parseKindIPv4DefaultGateway(routeOutput string) (string, error) {
	for _, line := range strings.Split(routeOutput, "\n") {
		fields := strings.Fields(line)
		for index := 0; index+1 < len(fields); index++ {
			if fields[index] != "via" {
				continue
			}
			gateway := net.ParseIP(fields[index+1])
			if gateway != nil && gateway.To4() != nil {
				return gateway.To4().String(), nil
			}
		}
	}
	return "", errors.New("kind node has no IPv4 default gateway")
}

// OperatorStatusSnapshot is the small status projection used by the readiness
// gate. It intentionally has no generic Ready condition.
type OperatorStatusSnapshot struct {
	Phase          string
	Conditions     map[string]string
	Services       map[string]bool
	EndpointSlices map[string]bool
}

// OperatorStatusReady applies the operator qualification readiness contract.
func OperatorStatusReady(snapshot OperatorStatusSnapshot, activeServices, endpointServices []string) error {
	if snapshot.Phase != "Running" {
		return fmt.Errorf("kubernaut phase is %q, want Running", snapshot.Phase)
	}
	for _, condition := range []string{"BYOValidated", "MigrationComplete", "ServicesDeployed", "TLSReady"} {
		if snapshot.Conditions[condition] != "True" {
			return fmt.Errorf("kubernaut condition %s is not True", condition)
		}
	}
	for _, service := range activeServices {
		if !snapshot.Services[service] {
			return fmt.Errorf("kubernetes service %s is not ready", service)
		}
	}
	for _, service := range endpointServices {
		if !snapshot.EndpointSlices[service] {
			return fmt.Errorf("EndpointSlice for %s is not ready", service)
		}
	}
	return nil
}

// OperatorExposureAdapter describes one upstream test-owned NodePort Service.
type OperatorExposureAdapter struct {
	Name        string `json:"name" yaml:"name"`
	Component   string `json:"component" yaml:"component"`
	ServiceName string `json:"serviceName,omitempty" yaml:"serviceName,omitempty"`
	NodePort    int    `json:"nodePort" yaml:"nodePort"`
	HostPort    int    `json:"hostPort" yaml:"hostPort"`
}

// OperatorExposurePlan is allocated before Kind creation and rendered after
// the operator has made the selected Pods/EndpointSlices ready.
type OperatorExposurePlan struct {
	Namespace string                    `json:"namespace" yaml:"namespace"`
	Hostname  string                    `json:"hostname" yaml:"hostname"`
	Adapters  []OperatorExposureAdapter `json:"adapters" yaml:"adapters"`
}

// BuildOperatorExposureManifest renders only test-owned adapter Services.
func BuildOperatorExposureManifest(plan OperatorExposurePlan) (string, error) {
	if plan.Namespace == "" {
		return "", errors.New("operator exposure requires a namespace")
	}
	if len(plan.Adapters) == 0 {
		return "", errors.New("operator exposure requires at least one adapter")
	}
	var documents []string
	for _, adapter := range plan.Adapters {
		if adapter.Name == "" || adapter.Component == "" || adapter.NodePort < 30000 || adapter.NodePort > 32767 {
			return "", fmt.Errorf("invalid operator exposure adapter %q", adapter.Name)
		}
		documents = append(documents, fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/part-of: kubernaut-e2e
    kubernaut.ai/e2e-owner: %s
spec:
  type: NodePort
  externalTrafficPolicy: Cluster
  selector:
    app: %s
    app.kubernetes.io/component: %s
    app.kubernetes.io/instance: kubernaut
    app.kubernetes.io/managed-by: kubernaut-operator
  ports:
  - name: https
    port: 8443
    targetPort: https
    nodePort: %d`, adapter.Name, plan.Namespace, operatorOwnerLabel, operatorComponentLabel(adapter.Component), operatorComponentLabel(adapter.Component), adapter.NodePort))
	}
	return strings.Join(documents, "\n---\n") + "\n", nil
}

func operatorComponentLabel(component string) string {
	switch component {
	case operatorDataStorageOperand:
		return "data-storage"
	case "kubernautagent":
		return "kubernaut-agent"
	default:
		return component
	}
}

func operatorServiceName(component string) string {
	switch component {
	case "gateway":
		return "gateway-service"
	case operatorDataStorageOperand:
		return "data-storage-service"
	case operatorAPIFrontendOperand:
		return operatorAPIFrontendOperand
	case "kubernautagent":
		return "kubernaut-agent"
	default:
		return component
	}
}

// OperatorExposureEvidence is persisted with qualification artifacts.
type OperatorExposureEvidence struct {
	NodeAddress string                    `json:"nodeAddress"`
	Hostname    string                    `json:"hostname"`
	CAPath      string                    `json:"caPath"`
	Adapters    []OperatorExposureAdapter `json:"adapters"`
}

func BuildOperatorExposureEvidenceJSON(evidence OperatorExposureEvidence) ([]byte, error) {
	if evidence.NodeAddress == "" || evidence.CAPath == "" {
		return nil, errors.New("operator exposure evidence requires nodeAddress and caPath")
	}
	return json.MarshalIndent(evidence, "", "  ")
}

// BuildOperatorDiagnostics returns a stable human-readable failure record.
type OperatorDiagnosticInput struct {
	Scenario      string
	OperatorSHA   string
	OperatorImage string
	CRManifest    string
	Status        string
	Conditions    string
	Events        string
	Dependencies  string
}

func BuildOperatorDiagnostics(input OperatorDiagnosticInput) string {
	return fmt.Sprintf("scenario: %s\noperatorSHA: %s\noperatorImage: %s\n--- CR manifest ---\n%s\n--- status ---\n%s\n--- conditions ---\n%s\n--- events ---\n%s\n--- dependencies ---\n%s\n", input.Scenario, input.OperatorSHA, input.OperatorImage, input.CRManifest, input.Status, input.Conditions, input.Events, input.Dependencies)
}

// DemoArtifactOverrides are user-facing explicit release/source overrides.
type DemoArtifactOverrides struct {
	OperatorVersion         string `yaml:"version,omitempty"`
	OperatorImage           string `yaml:"image,omitempty"`
	OperatorImageRepository string `yaml:"imageRepository,omitempty"`
	OperatorImageTag        string `yaml:"imageTag,omitempty"`
	OperatorDigest          string `yaml:"imageDigest,omitempty"`
	OperatorChart           string `yaml:"chart,omitempty"`
	OperatorChartDigest     string `yaml:"chartDigest,omitempty"`
	OperatorRef             string `yaml:"ref,omitempty"`
	DependencyChart         string `yaml:"dependencyChart,omitempty"`
	DependencyChartVersion  string `yaml:"dependencyChartVersion,omitempty"`
	DependencyChartDigest   string `yaml:"dependencyChartDigest,omitempty"`
	KubernautVersion        string `yaml:"kubernautVersion,omitempty"`
	KubernautImage          string `yaml:"kubernautImage,omitempty"`
	KubernautDigest         string `yaml:"kubernautImageDigest,omitempty"`
}

type DemoArtifactResolutionInput struct {
	ReleaseLine string
	Explicit    DemoArtifactOverrides
}

type ResolvedDemoOperatorArtifacts struct {
	OperatorVersion        string
	OperatorImage          string
	OperatorChart          string
	OperatorRef            string
	DependencyChart        string
	DependencyChartVersion string
	KubernautVersion       string
	KubernautImage         string
}

// DemoArtifactResolutionRecord is emitted before live reconciliation so a
// demo failure can be reconstructed without guessing which release inputs won.
type DemoArtifactResolutionRecord struct {
	ReleaseLine            string            `json:"releaseLine"`
	OperatorVersion        string            `json:"operatorVersion"`
	OperatorRef            string            `json:"operatorRef,omitempty"`
	OperatorImage          string            `json:"operatorImage"`
	OperatorChart          string            `json:"operatorChart"`
	DependencyChart        string            `json:"dependencyChart"`
	DependencyChartVersion string            `json:"dependencyChartVersion,omitempty"`
	KubernautVersion       string            `json:"kubernautVersion"`
	RelatedImages          map[string]string `json:"relatedImages"`
}

func BuildDemoArtifactResolutionJSON(record DemoArtifactResolutionRecord) ([]byte, error) {
	if record.OperatorImage == "" || record.OperatorChart == "" || record.DependencyChart == "" || record.KubernautVersion == "" {
		return nil, errors.New("demo artifact resolution requires operator image/chart, dependency chart, and Kubernaut version")
	}
	return json.MarshalIndent(record, "", "  ")
}

func ResolveDemoOperatorArtifacts(input DemoArtifactResolutionInput) (ResolvedDemoOperatorArtifacts, error) {
	version := resolveDemoReleaseVersion(input)
	operatorVersion := strings.TrimSpace(input.Explicit.OperatorVersion)
	if operatorVersion == "" {
		operatorVersion = version
	}
	if operatorVersion != demoReleaseVersionRC27 && !hasExplicitDemoOperatorChart(input) {
		return ResolvedDemoOperatorArtifacts{}, fmt.Errorf("operator version %q requires an explicit operator chart override", operatorVersion)
	}
	kubernautVersion := strings.TrimSpace(input.Explicit.KubernautVersion)
	if kubernautVersion == "" {
		kubernautVersion = version
	}
	operatorImage := resolveDemoOperatorImage(input.Explicit, operatorVersion)
	operatorChart := resolveDemoArtifactReference(input.Explicit.OperatorChart, demoOperatorChartOCI, input.Explicit.OperatorChartDigest)
	dependencyChart := resolveDemoArtifactReference(input.Explicit.DependencyChart, demoDependencyChartOCI, input.Explicit.DependencyChartDigest)
	dependencyChartVersion := strings.TrimSpace(input.Explicit.DependencyChartVersion)
	if dependencyChartVersion == "" {
		dependencyChartVersion = demoDependencyChartVersion
	}
	kubernautImage := input.Explicit.KubernautImage
	if kubernautImage == "" {
		kubernautImage = "quay.io/kubernaut-ai/gateway:" + kubernautVersion
	}
	return ResolvedDemoOperatorArtifacts{OperatorVersion: operatorVersion, OperatorImage: operatorImage, OperatorChart: operatorChart, OperatorRef: strings.TrimSpace(input.Explicit.OperatorRef), DependencyChart: dependencyChart, DependencyChartVersion: dependencyChartVersion, KubernautVersion: kubernautVersion, KubernautImage: kubernautImage}, nil
}

func resolveDemoReleaseVersion(input DemoArtifactResolutionInput) string {
	version := strings.TrimSpace(input.ReleaseLine)
	if version == "" {
		version = strings.TrimSpace(os.Getenv("KUBERNAUT_RELEASE_VERSION"))
	}
	if version == "" || version == "1.6" {
		return demoReleaseVersionRC27
	}
	return version
}

func hasExplicitDemoOperatorChart(input DemoArtifactResolutionInput) bool {
	return strings.TrimSpace(input.Explicit.OperatorChart) != "" ||
		strings.TrimSpace(input.Explicit.OperatorChartDigest) != "" ||
		strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_CHART_PATH")) != "" ||
		strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_ROOT")) != ""
}

// resolveDemoOperatorImage selects the operator image from explicit selectors
// or the pinned release profile and then applies an optional digest.
func resolveDemoOperatorImage(overrides DemoArtifactOverrides, operatorVersion string) string {
	operatorImage := strings.TrimSpace(overrides.OperatorImage)
	if operatorImage == "" {
		repository := overrides.OperatorImageRepository
		if strings.TrimSpace(repository) == "" && strings.TrimSpace(overrides.OperatorImageTag) != "" {
			repository = demoOperatorImageRepository
		}
		operatorImage = composeImageReference(repository, overrides.OperatorImageTag)
	}
	if operatorImage == "" {
		if operatorVersion == demoReleaseVersionRC27 {
			operatorImage = demoOperatorImageRC27
		} else {
			operatorImage = "quay.io/kubernaut-ai/kubernaut-operator-image:" + operatorVersion
		}
	}
	return applyDemoDigest(operatorImage, overrides.OperatorDigest)
}

func resolveDemoArtifactReference(explicit, defaultReference, digest string) string {
	reference := strings.TrimSpace(explicit)
	if reference == "" {
		reference = defaultReference
	}
	return applyDemoDigest(reference, digest)
}

func applyDemoDigest(reference, digest string) string {
	digest = strings.TrimPrefix(strings.TrimSpace(digest), "@")
	if digest == "" {
		return reference
	}
	if at := strings.Index(reference, "@"); at >= 0 {
		return reference[:at] + "@" + digest
	}
	return reference + "@" + digest
}

func composeImageReference(repository, tag string) string {
	repository = strings.TrimRight(strings.TrimSpace(repository), "/")
	tag = strings.TrimSpace(tag)
	if repository == "" {
		return ""
	}
	if tag == "" {
		return repository
	}
	return repository + ":" + tag
}

func ValidateOperatorSourceCheckout(root, statusOutput string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("operator source checkout path is empty")
	}
	if strings.TrimSpace(statusOutput) != "" {
		return fmt.Errorf("operator source checkout is dirty: %s", strings.TrimSpace(statusOutput))
	}
	return nil
}

// QualificationArtifact links release integrity metadata for one image/chart.
type QualificationArtifact struct {
	Name       string `json:"name"`
	Digest     string `json:"digest"`
	SBOM       string `json:"sbom"`
	Signature  string `json:"signature"`
	Provenance string `json:"provenance"`
}

type QualificationResult struct {
	Lane   string `json:"lane"`
	Passed bool   `json:"passed"`
}

type QualificationManifestInput struct {
	OperatorImageDigest string
	Artifacts           []QualificationArtifact
	Results             []QualificationResult
}

func BuildQualificationManifest(input QualificationManifestInput) ([]byte, error) {
	if strings.TrimSpace(input.OperatorImageDigest) == "" {
		return nil, errors.New("qualification manifest requires operator image digest")
	}
	for _, artifact := range input.Artifacts {
		if artifact.Digest == "" || artifact.SBOM == "" || artifact.Signature == "" || artifact.Provenance == "" {
			return nil, fmt.Errorf("qualification artifact %q is missing integrity metadata", artifact.Name)
		}
	}
	return json.MarshalIndent(struct {
		SchemaVersion       int                     `json:"schemaVersion"`
		OperatorImageDigest string                  `json:"operatorImageDigest"`
		Artifacts           []QualificationArtifact `json:"artifacts"`
		Results             []QualificationResult   `json:"results"`
	}{1, input.OperatorImageDigest, input.Artifacts, input.Results}, "", "  ")
}

func BuildOperatorCleanupPlan(input OperatorCleanupInput) []string {
	cleanup := make([]string, 0, len(input.Adapters)+len(input.DependencyReleases))
	for _, adapter := range input.Adapters {
		cleanup = append(cleanup, "delete service/"+adapter)
	}
	for _, release := range input.DependencyReleases {
		cleanup = append(cleanup, "helm uninstall "+release)
	}
	sort.Strings(cleanup)
	return cleanup
}

type OperatorCleanupInput struct {
	Adapters           []string
	DependencyReleases []string
}

// KindPortMapping is a pre-creation Kind extraPortMappings entry.
type KindPortMapping struct {
	ContainerPort int
	HostPort      int
	Protocol      string
}

func appendKindPortMappings(config string, mappings []KindPortMapping) (string, error) {
	if len(mappings) == 0 {
		return config, nil
	}
	existing := existingKindPortMappings(config)
	filtered, err := filterKindPortMappings(existing, mappings)
	if err != nil {
		return "", err
	}
	if len(filtered) == 0 {
		return config, nil
	}
	rendered := renderKindPortMappings(filtered)
	return insertKindPortMappings(config, rendered), nil
}

func filterKindPortMappings(existing map[int]int, mappings []KindPortMapping) ([]KindPortMapping, error) {
	seenContainer := make(map[int]struct{}, len(mappings))
	seenHost := make(map[int]struct{}, len(mappings))
	filtered := make([]KindPortMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.ContainerPort < 30000 || mapping.ContainerPort > 32767 {
			return nil, fmt.Errorf("container port %d is outside the NodePort range", mapping.ContainerPort)
		}
		if mapping.HostPort < 1 || mapping.HostPort > 65535 {
			return nil, fmt.Errorf("host port %d is outside the valid range", mapping.HostPort)
		}
		if _, exists := seenContainer[mapping.ContainerPort]; exists {
			return nil, fmt.Errorf("duplicate container port %d", mapping.ContainerPort)
		}
		if _, exists := seenHost[mapping.HostPort]; exists {
			return nil, fmt.Errorf("duplicate host port %d", mapping.HostPort)
		}
		if existingHost, exists := existing[mapping.ContainerPort]; exists {
			if existingHost != mapping.HostPort {
				return nil, fmt.Errorf("container port %d is already mapped to host port %d", mapping.ContainerPort, existingHost)
			}
			continue
		}
		for existingContainer, existingHost := range existing {
			if existingHost == mapping.HostPort && existingContainer != mapping.ContainerPort {
				return nil, fmt.Errorf("host port %d is already mapped from container port %d", mapping.HostPort, existingContainer)
			}
		}
		seenContainer[mapping.ContainerPort] = struct{}{}
		seenHost[mapping.HostPort] = struct{}{}
		filtered = append(filtered, mapping)
	}
	return filtered, nil
}

func renderKindPortMappings(mappings []KindPortMapping) string {
	var rendered strings.Builder
	for _, mapping := range mappings {
		protocol := mapping.Protocol
		if protocol == "" {
			protocol = "TCP"
		}
		fmt.Fprintf(&rendered, "  - containerPort: %d\n    hostPort: %d\n    protocol: %s\n", mapping.ContainerPort, mapping.HostPort, protocol)
	}
	return rendered.String()
}

func insertKindPortMappings(config, rendered string) string {
	needle := "  kubeadmConfigPatches:"
	if strings.Contains(config, "  extraPortMappings:") {
		if index := strings.Index(config, needle); index >= 0 {
			return config[:index] + rendered + config[index:]
		}
		return config + "\n" + rendered
	}
	if index := strings.Index(config, needle); index >= 0 {
		return config[:index] + "  extraPortMappings:\n" + rendered + config[index:]
	}
	return config + "\n  extraPortMappings:\n" + rendered
}

func existingKindPortMappings(config string) map[int]int {
	pairs := make(map[int]int)
	lines := strings.Split(config, "\n")
	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmedLine, "- containerPort:") {
			continue
		}
		containerValue := strings.Fields(strings.TrimSpace(strings.TrimPrefix(trimmedLine, "- containerPort:")))
		if len(containerValue) == 0 {
			continue
		}
		containerPort, err := strconv.Atoi(containerValue[0])
		if err != nil || i+1 >= len(lines) {
			continue
		}
		hostLine := strings.TrimSpace(lines[i+1])
		if !strings.HasPrefix(hostLine, "hostPort:") {
			continue
		}
		hostValue := strings.Fields(strings.TrimSpace(strings.TrimPrefix(hostLine, "hostPort:")))
		if len(hostValue) == 0 {
			continue
		}
		hostPort, err := strconv.Atoi(hostValue[0])
		if err == nil {
			pairs[containerPort] = hostPort
		}
	}
	return pairs
}

// OperatorFixtureFiles identifies application-owned fixture inputs that must be
// provisioned separately from the PostgreSQL/Valkey dependency chart.
type OperatorFixtureFiles struct {
	LLMCredentials         string
	AIAnalysisPolicy       string
	SignalProcessingPolicy string
}

// OperatorDeploymentConfig is the complete input contract for a live
// operator-managed application deployment.
type OperatorDeploymentConfig struct {
	ClusterName            string
	Namespace              string
	OperatorNamespace      string
	KubeconfigPath         string
	OperatorChartPath      string
	OperatorChartVersion   string
	DependencyChartPath    string
	DependencyChartVersion string
	OperatorImage          string
	RelatedImages          map[string]string
	PostgreSQLImage        string
	ValkeyImage            string
	CR                     OperatorCROptions
	CRManifestPath         string
	HostIngressCIDR        string
	MonitoringNamespace    string
	FixtureFiles           OperatorFixtureFiles
	Exposure               *OperatorExposurePlan
	DiagnosticsDir         string
	Provenance             OperatorArtifactProvenance
	DemoArtifacts          *DemoArtifactResolutionRecord
}

// DeployOperatorManagedApplication installs external dependencies and the
// operator bootstrap, then applies the user-owned CR. No application Helm
// release is created by this function.
func DeployOperatorManagedApplication(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) (OperatorExposureEvidence, error) {
	if config.Namespace == "" {
		config.Namespace = applicationNamespace
	}
	if config.KubeconfigPath != "" {
		if err := os.Setenv("KUBECONFIG", config.KubeconfigPath); err != nil {
			return OperatorExposureEvidence{}, fmt.Errorf("set operator deployment kubeconfig: %w", err)
		}
	}
	if err := ValidateOperatorOperandImageMap(config.RelatedImages); err != nil {
		return OperatorExposureEvidence{}, err
	}
	if config.DemoArtifacts != nil {
		data, err := BuildDemoArtifactResolutionJSON(*config.DemoArtifacts)
		if err != nil {
			return OperatorExposureEvidence{}, fmt.Errorf("build demo artifact provenance: %w", err)
		}
		path, err := writeOperatorArtifact(config.DiagnosticsDir, "demo-artifacts.json", data)
		if err != nil {
			return OperatorExposureEvidence{}, fmt.Errorf("write demo artifact provenance: %w", err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ demo artifact provenance recorded at %s\n", path)
	}
	if config.DependencyChartPath == "" || config.OperatorChartPath == "" {
		return OperatorExposureEvidence{}, errors.New("operator deployment requires dependency and operator chart paths")
	}
	if err := InstallOperatorDependencies(ctx, config, writer); err != nil {
		return OperatorExposureEvidence{}, err
	}
	if err := InstallOperatorBootstrap(ctx, config, writer); err != nil {
		return OperatorExposureEvidence{}, err
	}
	return ApplyOperatorCRAndWait(ctx, config, writer)
}

// InstallOperatorDependencies installs the independently owned PostgreSQL/Valkey
// chart and then provisions any explicitly supplied application fixtures. It
// never creates a Kubernaut CR and never passes application credentials or
// policies through dependency-chart Helm values.
func InstallOperatorDependencies(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) error {
	if config.KubeconfigPath != "" {
		if err := os.Setenv("KUBECONFIG", config.KubeconfigPath); err != nil {
			return fmt.Errorf("set dependency deployment kubeconfig: %w", err)
		}
	}
	if err := runHelmCommand(ctx, BuildOperatorDependencyArgs(OperatorDependencyOptions{
		Namespace: config.Namespace, ChartPath: config.DependencyChartPath, ChartVersion: config.DependencyChartVersion,
		PostgreSQLImage: config.PostgreSQLImage, ValkeyImage: config.ValkeyImage,
	}), writer); err != nil {
		return fmt.Errorf("install operator dependencies: %w", err)
	}
	if err := ensureOperatorApplicationFixtures(ctx, config, writer); err != nil {
		return fmt.Errorf("provision application fixtures: %w", err)
	}
	return nil
}

// EnsureOperatorDataStorageClientRBAC restores the shared application
// contract that the deprecated application Helm chart used to provide. The
// operator owns workloads and their RoleBindings, but the qualification
// bootstrap still needs this small ClusterRole before the Kubernaut CR is
// reconciled; otherwise every DataStorage client binding references a missing
// role and the middleware correctly returns 403.
func EnsureOperatorDataStorageClientRBAC(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	const manifest = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: data-storage-client
  labels:
    kubernaut.ai/e2e-owner: upstream-2492
rules:
- apiGroups: [""]
  resources: ["services"]
  resourceNames: ["data-storage-service"]
  verbs: ["create", "get", "list", "update", "delete"]
`
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("ensure DataStorage client ClusterRole: %w", err)
	}
	const monitoringManifest = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cluster-monitoring-view
  labels:
    kubernaut.ai/e2e-owner: upstream-2492
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints", "nodes", "nodes/proxy", "namespaces", "persistentvolumeclaims"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs", "cronjobs"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["events.k8s.io"]
  resources: ["events"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["policy"]
  resources: ["poddisruptionbudgets"]
  verbs: ["get", "list"]
- apiGroups: ["autoscaling"]
  resources: ["horizontalpodautoscalers"]
  verbs: ["get", "list"]
- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies"]
  verbs: ["get", "list"]
- apiGroups: ["metrics.k8s.io"]
  resources: ["pods", "nodes"]
  verbs: ["get", "list"]
`
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, monitoringManifest); err != nil {
		return fmt.Errorf("ensure monitoring view ClusterRole: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ data-storage-client ClusterRole ready for operator-managed RoleBindings")
	_, _ = fmt.Fprintln(writer, "  ✅ cluster-monitoring-view ClusterRole ready for operator-managed bindings")
	return nil
}

func operatorNamespaceForConfig(config OperatorDeploymentConfig) string {
	if config.OperatorNamespace != "" {
		return config.OperatorNamespace
	}
	return operatorNamespace
}

func ensureOperatorApplicationFixtures(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) error {
	if config.FixtureFiles.LLMCredentials != "" {
		secretName := strings.TrimSpace(config.CR.LLM.CredentialsSecret)
		if secretName == "" {
			secretName = demoLLMSecretName
		}
		if err := ensureOperatorLLMCredentials(ctx, config.KubeconfigPath, config.Namespace, secretName, config.FixtureFiles.LLMCredentials, writer); err != nil {
			return err
		}
	}
	if config.FixtureFiles.AIAnalysisPolicy != "" {
		name := strings.TrimSpace(config.CR.PolicyConfigMaps.AIAnalysis)
		if name == "" {
			return errors.New("ai analysis policy fixture requires a configmap name in the kubernaut cr options")
		}
		if err := ensureOperatorPolicyConfigMap(ctx, config.KubeconfigPath, config.Namespace, name, "approval.rego", config.FixtureFiles.AIAnalysisPolicy, writer); err != nil {
			return err
		}
	}
	if config.FixtureFiles.SignalProcessingPolicy != "" {
		name := strings.TrimSpace(config.CR.PolicyConfigMaps.SignalProcessing)
		if name == "" {
			return errors.New("signal processing policy fixture requires a configmap name in the kubernaut cr options")
		}
		if err := ensureOperatorPolicyConfigMap(ctx, config.KubeconfigPath, config.Namespace, name, "policy.rego", config.FixtureFiles.SignalProcessingPolicy, writer); err != nil {
			return err
		}
	}
	return nil
}

func applicationFixtureLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by": "setup-demo-infra",
		"kubernaut.ai/demo-owner":      "setup-demo-infra",
		"kubernaut.ai/e2e-owner":       operatorOwnerLabel,
	}
}

func buildOperatorLLMCredentialsManifest(namespace, secretName string, credentials []byte) (string, error) {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(secretName) == "" {
		return "", errors.New("application llm secret requires namespace and name")
	}
	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    applicationFixtureLabels(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"api_key": credentials,
		},
	}
	manifest, err := json.Marshal(secret)
	if err != nil {
		return "", fmt.Errorf("marshal application llm secret: %w", err)
	}
	return string(manifest), nil
}

func ensureOperatorLLMCredentials(ctx context.Context, kubeconfigPath, namespace, secretName, credentialsPath string, writer io.Writer) error {
	credentials, err := os.ReadFile(credentialsPath)
	if err != nil {
		return fmt.Errorf("read operator LLM credentials: %w", err)
	}
	manifest, err := buildOperatorLLMCredentialsManifest(namespace, secretName, credentials)
	if err != nil {
		return err
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("create user-owned %s secret: %w", secretName, err)
	}
	return nil
}

func buildOperatorPolicyConfigMapManifest(namespace, name, dataKey, policy string) (string, error) {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(dataKey) == "" {
		return "", errors.New("application policy configmap requires namespace, name, and data key")
	}
	configMap := corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    applicationFixtureLabels(),
		},
		Data: map[string]string{
			dataKey: policy,
		},
	}
	manifest, err := json.Marshal(configMap)
	if err != nil {
		return "", fmt.Errorf("marshal application policy configmap: %w", err)
	}
	return string(manifest), nil
}

func ensureOperatorPolicyConfigMap(ctx context.Context, kubeconfigPath, namespace, name, dataKey, policyPath string, writer io.Writer) error {
	policy, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("read %s policy: %w", name, err)
	}
	manifest, err := buildOperatorPolicyConfigMapManifest(namespace, name, dataKey, string(policy))
	if err != nil {
		return err
	}
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("create user-owned %s configmap: %w", name, err)
	}
	return nil
}

func operatorImageArtifactPath() string {
	return strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_IMAGE_TAR"))
}

func ensureOperatorImageAvailable(ctx context.Context, imageRef string, writer io.Writer) error {
	check := exec.CommandContext(ctx, "podman", "image", "exists", imageRef)
	if err := check.Run(); err == nil {
		return nil
	}

	tarPath := operatorImageArtifactPath()
	if tarPath == "" {
		return fmt.Errorf("operator image %q is not available locally", imageRef)
	}
	_, _ = fmt.Fprintf(writer, "  📦 Reloading operator image artifact: %s\n", tarPath)
	load := exec.CommandContext(ctx, "podman", "load", "-i", tarPath)
	load.Stdout = writer
	load.Stderr = writer
	if err := load.Run(); err != nil {
		return fmt.Errorf("reload operator image artifact: %w", err)
	}

	check = exec.CommandContext(ctx, "podman", "image", "exists", imageRef)
	if err := check.Run(); err != nil {
		return fmt.Errorf("operator image %q is still unavailable after loading %s: %w", imageRef, tarPath, err)
	}
	return nil
}

// InstallOperatorBootstrap installs the operator-only chart and waits for its
// manager. The bootstrap chart does not create the application CR.
func InstallOperatorBootstrap(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) error {
	if config.KubeconfigPath != "" {
		if err := os.Setenv("KUBECONFIG", config.KubeconfigPath); err != nil {
			return fmt.Errorf("set operator bootstrap kubeconfig: %w", err)
		}
	}
	if config.ClusterName != "" && strings.HasPrefix(config.OperatorImage, "localhost/") {
		if err := ensureOperatorImageAvailable(ctx, config.OperatorImage, writer); err != nil {
			return fmt.Errorf("ensure operator image is available: %w", err)
		}
		if err := LoadImageToKind(ctx, config.OperatorImage, "kubernaut-operator", config.ClusterName, writer); err != nil {
			return fmt.Errorf("load operator image into Kind via Podman archive: %w", err)
		}
	}
	operatorNamespace := operatorNamespaceForConfig(config)
	operatorArgs := BuildOperatorChartArgs(OperatorChartInstallOptions{
		Namespace: operatorNamespace, ChartPath: config.OperatorChartPath, ChartVersion: config.OperatorChartVersion,
		OperatorRef: config.OperatorImage, RelatedImages: config.RelatedImages,
	})
	if err := runHelmCommand(ctx, operatorArgs, writer); err != nil {
		return fmt.Errorf("install operator bootstrap: %w", err)
	}
	if err := waitForOperatorManager(ctx, config.KubeconfigPath, operatorNamespace, writer); err != nil {
		return operatorDeploymentFailure(ctx, config, "IT-2492-BOOTSTRAP-001", err, writer)
	}
	return nil
}

// ApplyOperatorCRAndWait applies the user-owned CR after external Fleet
// infrastructure is ready, then proves reconciliation and host exposure.
func ApplyOperatorCRAndWait(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) (OperatorExposureEvidence, error) {
	if err := prepareOperatorCRAndWait(ctx, config, writer); err != nil {
		return OperatorExposureEvidence{}, err
	}

	evidence, err := applyOperatorExposure(ctx, config, writer)
	if err != nil {
		return OperatorExposureEvidence{}, err
	}
	if err := writeOperatorInterServiceCA(ctx, config.KubeconfigPath, config.Namespace, writer); err != nil {
		return OperatorExposureEvidence{}, fmt.Errorf("write operator CA: %w", err)
	}
	if err := verifyAndPersistOperatorExposure(ctx, config, evidence, writer); err != nil {
		return OperatorExposureEvidence{}, err
	}
	return evidence, nil
}

func prepareOperatorCRAndWait(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) error {
	crManifest, err := buildOperatorCRManifest(config)
	if err != nil {
		return fmt.Errorf("build Kubernaut CR: %w", err)
	}
	crPath, err := writeOperatorArtifact(config.DiagnosticsDir, "kubernaut-cr.yaml", []byte(crManifest))
	if err != nil {
		return fmt.Errorf("write Kubernaut CR: %w", err)
	}
	if err := kubectlApplyFile(ctx, config.KubeconfigPath, crPath, writer); err != nil {
		return operatorDeploymentFailure(ctx, config, "IT-2492-CR-001", err, writer)
	}
	if err := EnsureOperatorAuthWebhookCABundle(ctx, config.KubeconfigPath, config.Namespace, writer); err != nil {
		return operatorDeploymentFailure(ctx, config, "IT-2492-CR-001", err, writer)
	}
	if err := EnsureOperatorWritableTempVolumes(ctx, config.KubeconfigPath, config.Namespace, writer); err != nil {
		return operatorDeploymentFailure(ctx, config, "IT-2492-CR-001", err, writer)
	}
	if err := waitForOperatorCR(ctx, config); err != nil {
		return operatorDeploymentFailure(ctx, config, "IT-2492-CR-001", err, writer)
	}
	if err := verifyOperatorDeploymentImages(ctx, config, writer); err != nil {
		return operatorDeploymentFailure(ctx, config, "IT-2492-IMAGE-001", err, writer)
	}
	return applyOperatorNetworkPolicies(ctx, config, writer)
}

func applyOperatorNetworkPolicies(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) error {
	if config.HostIngressCIDR != "" {
		if err := kubectlApplyManifest(ctx, config.KubeconfigPath, writer, BuildOperatorHostIngressNetworkPolicyManifest(config.Namespace, config.HostIngressCIDR)); err != nil {
			return operatorDeploymentFailure(ctx, config, "IT-2492-NETWORKPOLICY-001", err, writer)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ test-owned host ingress NetworkPolicy applied for %s\n", config.HostIngressCIDR)
	}
	if config.MonitoringNamespace != "" {
		if err := kubectlApplyManifest(ctx, config.KubeconfigPath, writer, BuildOperatorMonitoringIngressNetworkPolicyManifest(config.Namespace, config.MonitoringNamespace)); err != nil {
			return operatorDeploymentFailure(ctx, config, "IT-2492-NETWORKPOLICY-002", err, writer)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ test-owned monitoring ingress NetworkPolicy applied for %s\n", config.MonitoringNamespace)
	}
	return nil
}

func applyOperatorExposure(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) (OperatorExposureEvidence, error) {
	evidence := OperatorExposureEvidence{CAPath: InterServiceCAPath(config.KubeconfigPath)}
	if config.Exposure == nil || len(config.Exposure.Adapters) == 0 {
		return evidence, nil
	}
	manifest, err := BuildOperatorExposureManifest(*config.Exposure)
	if err != nil {
		return OperatorExposureEvidence{}, err
	}
	if err := kubectlApplyManifest(ctx, config.KubeconfigPath, writer, manifest); err != nil {
		return OperatorExposureEvidence{}, operatorDeploymentFailure(ctx, config, "IT-2492-EXPOSURE-001", err, writer)
	}
	if err := waitForOperatorExposure(ctx, config.KubeconfigPath, config.Namespace, *config.Exposure, writer); err != nil {
		return OperatorExposureEvidence{}, operatorDeploymentFailure(ctx, config, "IT-2492-EXPOSURE-001", err, writer)
	}
	evidence.NodeAddress, err = operatorNodeAddress(ctx, config.KubeconfigPath)
	if err != nil {
		return OperatorExposureEvidence{}, err
	}
	evidence.Hostname = config.Exposure.Hostname
	evidence.Adapters = config.Exposure.Adapters
	return evidence, nil
}

func verifyAndPersistOperatorExposure(ctx context.Context, config OperatorDeploymentConfig, evidence OperatorExposureEvidence, writer io.Writer) error {
	if len(evidence.Adapters) > 0 {
		if err := verifyOperatorExposureTLS(ctx, evidence, writer); err != nil {
			return operatorDeploymentFailure(ctx, config, "IT-2492-EXPOSURE-001", err, writer)
		}
	}
	if config.DiagnosticsDir == "" {
		return nil
	}
	evidenceData, err := BuildOperatorExposureEvidenceJSON(evidence)
	if err != nil {
		return err
	}
	_, _ = writeOperatorArtifact(config.DiagnosticsDir, "operator-exposure.json", evidenceData)
	return nil
}

const operatorRuntimeTempVolumeName = "operator-runtime-tmp"

// ensureWritableTempVolume adds the test-owned compatibility volume to every
// regular container in an operator-managed deployment. It is idempotent so a
// preserved cluster can be safely retried.
func ensureWritableTempVolume(deployment *appsv1.Deployment) bool {
	if deployment == nil || deployment.Labels["app.kubernetes.io/managed-by"] != "kubernaut-operator" {
		return false
	}
	needsVolume := false
	for _, container := range deployment.Spec.Template.Spec.Containers {
		hasWritableTemp := false
		for _, mount := range container.VolumeMounts {
			if mount.MountPath == operatorRuntimeTempMountPath {
				hasWritableTemp = true
				break
			}
		}
		if !hasWritableTemp {
			needsVolume = true
			break
		}
	}
	if !needsVolume {
		return false
	}
	volumeExists := false
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == operatorRuntimeTempVolumeName {
			volumeExists = true
			break
		}
	}
	if !volumeExists {
		deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
			Name:         operatorRuntimeTempVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	}
	changed := false
	for index := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[index]
		found := false
		for _, mount := range container.VolumeMounts {
			if mount.MountPath == operatorRuntimeTempMountPath {
				found = true
				break
			}
		}
		if !found {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: operatorRuntimeTempVolumeName, MountPath: operatorRuntimeTempMountPath})
			changed = true
		}
	}
	return changed
}

// EnsureOperatorWritableTempVolumes applies the legacy operand compatibility
// mount after the operator has rendered its application Deployments.
func EnsureOperatorWritableTempVolumes(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("create Kubernetes client for operator temp volumes: %w", err)
	}
	updated := 0
	stableSince := time.Time{}
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
	for {
		deployments, listErr := clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/managed-by=kubernaut-operator"})
		if listErr != nil {
			return fmt.Errorf("list operator-managed deployments: %w", listErr)
		}
		allMounted := len(deployments.Items) > 0
		for index := range deployments.Items {
			deploymentName := deployments.Items[index].Name
			deployment, changed, updateErr := ensureWritableTempVolumeWithRetry(ctx, clientset, namespace, deploymentName)
			if updateErr != nil {
				return fmt.Errorf("patch writable /tmp on deployment %s: %w", deploymentName, updateErr)
			}
			if changed {
				updated++
			}
			if !deploymentHasWritableTemp(deployment) {
				allMounted = false
			}
		}
		if allMounted {
			if stableSince.IsZero() {
				stableSince = time.Now()
			} else if time.Since(stableSince) >= 10*time.Second {
				break
			}
		} else {
			stableSince = time.Time{}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("timed out waiting for operator-managed deployments to receive writable /tmp")
		case <-time.After(2 * time.Second):
		}
	}
	_, _ = fmt.Fprintf(writer, "  ✅ test-owned writable /tmp compatibility mounted on %d operator deployments\n", updated)
	return nil
}

// ensureWritableTempVolumeWithRetry re-fetches the Deployment for every
// optimistic-concurrency attempt because the operator may reconcile the same
// object between the readiness list and this test-owned compatibility update.
func ensureWritableTempVolumeWithRetry(ctx context.Context, clientset kubernetes.Interface, namespace, deploymentName string) (*appsv1.Deployment, bool, error) {
	var latest *appsv1.Deployment
	changed := false
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := clientset.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		changed = ensureWritableTempVolume(deployment)
		if !changed {
			latest = deployment
			return nil
		}
		latest, err = clientset.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		return err
	})
	return latest, changed, err
}

func deploymentHasWritableTemp(deployment *appsv1.Deployment) bool {
	if deployment == nil || len(deployment.Spec.Template.Spec.Containers) == 0 {
		return false
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		mounted := false
		for _, mount := range container.VolumeMounts {
			if mount.MountPath == operatorRuntimeTempMountPath {
				mounted = true
				break
			}
		}
		if !mounted {
			return false
		}
	}
	return true
}

// EnsureOperatorAuthWebhookCABundle supplies the administrator-owned CA bundle
// for manual TLS mode. The operator intentionally preserves this field rather
// than owning it, but its current generic-cluster reconciliation creates the
// AuthWebhook configurations with an empty bundle until an administrator
// patches them.
func EnsureOperatorAuthWebhookCABundle(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("create Kubernetes client for AuthWebhook CA bundle: %w", err)
	}
	caBundle, err := readOperatorAuthWebhookCA(ctx, kubeconfigPath, namespace)
	if err != nil {
		return err
	}
	return waitAndPatchOperatorAuthWebhookCA(ctx, clientset, namespace, caBundle, writer)
}

func readOperatorAuthWebhookCA(ctx context.Context, kubeconfigPath, namespace string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "get", "secret/authwebhook-tls", "-n", namespace, "-o", "jsonpath={.data.ca\\.crt}")
	encodedCA, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("read AuthWebhook CA bundle: %w", err)
	}
	caBundle, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedCA)))
	if err != nil || len(caBundle) == 0 {
		return nil, fmt.Errorf("decode AuthWebhook CA bundle: %w", err)
	}
	return caBundle, nil
}

func waitAndPatchOperatorAuthWebhookCA(ctx context.Context, clientset kubernetes.Interface, namespace string, caBundle []byte, writer io.Writer) error {
	mutatingName := namespace + "-authwebhook-mutating"
	validatingName := namespace + "-authwebhook-validating"
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	for {
		mutating, validating, ready, err := getOperatorAuthWebhookConfigurations(ctx, clientset, mutatingName, validatingName)
		if err != nil {
			return err
		}
		if ready {
			patched, err := patchOperatorAuthWebhookConfigurations(ctx, clientset, mutating, validating, caBundle)
			if err != nil {
				return err
			}
			if patched {
				_, _ = fmt.Fprintln(writer, "  ✅ AuthWebhook webhook CA bundles patched for manual TLS")
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("timed out waiting for AuthWebhook webhook configurations")
		case <-time.After(2 * time.Second):
		}
	}
}

func getOperatorAuthWebhookConfigurations(ctx context.Context, clientset kubernetes.Interface, mutatingName, validatingName string) (*admissionregistrationv1.MutatingWebhookConfiguration, *admissionregistrationv1.ValidatingWebhookConfiguration, bool, error) {
	mutating, mutatingErr := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, mutatingName, metav1.GetOptions{})
	validating, validatingErr := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, validatingName, metav1.GetOptions{})
	if mutatingErr != nil && !apierrors.IsNotFound(mutatingErr) {
		return nil, nil, false, fmt.Errorf("read AuthWebhook MutatingWebhookConfiguration: %w", mutatingErr)
	}
	if validatingErr != nil && !apierrors.IsNotFound(validatingErr) {
		return nil, nil, false, fmt.Errorf("read AuthWebhook ValidatingWebhookConfiguration: %w", validatingErr)
	}
	ready := mutatingErr == nil && validatingErr == nil && len(mutating.Webhooks) > 0 && len(validating.Webhooks) > 0
	return mutating, validating, ready, nil
}

func patchOperatorAuthWebhookConfigurations(ctx context.Context, clientset kubernetes.Interface, mutating *admissionregistrationv1.MutatingWebhookConfiguration, validating *admissionregistrationv1.ValidatingWebhookConfiguration, caBundle []byte) (bool, error) {
	for index := range mutating.Webhooks {
		mutating.Webhooks[index].ClientConfig.CABundle = append([]byte(nil), caBundle...)
	}
	for index := range validating.Webhooks {
		validating.Webhooks[index].ClientConfig.CABundle = append([]byte(nil), caBundle...)
	}
	if _, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Update(ctx, mutating, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsConflict(err) {
			return false, nil
		}
		return false, fmt.Errorf("patch AuthWebhook MutatingWebhookConfiguration CA bundle: %w", err)
	}
	if _, err := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(ctx, validating, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsConflict(err) {
			return false, nil
		}
		return false, fmt.Errorf("patch AuthWebhook ValidatingWebhookConfiguration CA bundle: %w", err)
	}
	return true, nil
}

func buildOperatorCRManifest(config OperatorDeploymentConfig) (string, error) {
	if config.CRManifestPath != "" {
		return RenderStaticOperatorCRManifest(config.CRManifestPath, nil)
	}
	return BuildKubernautCRManifest(config.CR)
}

// RenderStaticOperatorCRManifest loads an authored CR fixture and applies only
// explicitly supplied environment substitutions. The YAML structure remains a
// checked-in operator contract; callers cannot accidentally copy operator
// defaults into generated Go structs.
func RenderStaticOperatorCRManifest(path string, replacements map[string]string) (string, error) {
	if path == "" {
		return "", errors.New("static operator CR path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(getProjectRoot(), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read static operator CR %s: %w", path, err)
	}
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	manifest := string(data)
	for _, key := range keys {
		manifest = strings.ReplaceAll(manifest, key, replacements[key])
	}
	return manifest, nil
}

// BuildOperatorHostIngressNetworkPolicyManifest returns the qualification-only
// NetworkPolicy that admits Kind host-port traffic. The CIDR is deliberately
// not part of the authored Kubernaut CR: it is a runtime property of the
// Docker/Podman bridge and is not a portable operator default.
func BuildOperatorHostIngressNetworkPolicyManifest(namespace, cidr string) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kubernaut-e2e-host-ingress
  namespace: %s
  labels:
    kubernaut.ai/e2e-owner: %s
    app.kubernetes.io/managed-by: kubernaut-e2e
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/managed-by: kubernaut-operator
      app.kubernetes.io/instance: kubernaut
    matchExpressions:
    - key: app
      operator: In
      values:
      - gateway
      - apifrontend
      - data-storage
      - kubernaut-agent
  policyTypes:
  - Ingress
  ingress:
  - from:
    - ipBlock:
        cidr: %s
`, namespace, operatorOwnerLabel, cidr)
}

func BuildOperatorMonitoringIngressNetworkPolicyManifest(namespace, monitoringNamespace string) string {
	return fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kubernaut-e2e-monitoring-ingress
  namespace: %s
  labels:
    kubernaut.ai/e2e-owner: %s
    app.kubernetes.io/managed-by: kubernaut-e2e
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/managed-by: kubernaut-operator
      app.kubernetes.io/instance: kubernaut
      app: gateway
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: %s
`, namespace, operatorOwnerLabel, monitoringNamespace)
}

const operatorExposureReadinessTimeout = 6 * time.Minute

func operatorExposureHTTPReady(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}

func operatorExposureRequiresHTTPReady(component string) bool {
	return component == operatorDataStorageOperand || component == operatorAPIFrontendOperand
}

func verifyOperatorExposureTLS(ctx context.Context, evidence OperatorExposureEvidence, writer io.Writer) error {
	caPEM, err := os.ReadFile(evidence.CAPath)
	if err != nil {
		return fmt.Errorf("read operator exposure CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return errors.New("operator exposure CA contains no certificates")
	}
	tlsConfig := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}, Timeout: 5 * time.Second}
	for _, adapter := range evidence.Adapters {
		url := fmt.Sprintf("https://%s:%d/readyz", evidence.HostnameOrLoopback(), adapter.HostPort)
		deadline := time.NewTimer(operatorExposureReadinessTimeout)
		var lastErr error
		for {
			request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if requestErr != nil {
				deadline.Stop()
				return fmt.Errorf("build TLS probe for %s: %w", adapter.Component, requestErr)
			}
			response, requestErr := client.Do(request)
			if requestErr == nil {
				statusCode := response.StatusCode
				_ = response.Body.Close()
				if !operatorExposureRequiresHTTPReady(adapter.Component) || operatorExposureHTTPReady(statusCode) {
					deadline.Stop()
					_, _ = fmt.Fprintf(writer, "  ✅ CA-verified exposure probe: %s returned HTTP %d\n", adapter.Component, statusCode)
					break
				}
				lastErr = fmt.Errorf("readiness endpoint returned HTTP %d", statusCode)
			} else {
				lastErr = requestErr
			}
			select {
			case <-ctx.Done():
				deadline.Stop()
				return fmt.Errorf("CA-verified host probe for %s at %s: %w", adapter.Component, url, ctx.Err())
			case <-deadline.C:
				return fmt.Errorf("CA-verified host probe for %s at %s: %w", adapter.Component, url, lastErr)
			case <-time.After(2 * time.Second):
			}
		}
	}
	return nil
}

func runHelmCommand(ctx context.Context, args []string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "helm", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm command failed: %w", err)
	}
	return nil
}

func kubectlApplyFile(ctx context.Context, kubeconfigPath, filePath string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", filePath)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply %s failed: %w", filePath, err)
	}
	return nil
}

func waitForOperatorManager(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace, "rollout", "status", "deployment/kubernaut-operator-controller-manager", "--timeout=10m")
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("operator manager is not ready: %w", err)
	}
	return nil
}

type operatorStatusWire struct {
	Status struct {
		Phase      string `json:"phase"`
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
		Services []struct {
			Name  string `json:"name"`
			Ready bool   `json:"ready"`
		} `json:"services"`
	} `json:"status"`
}

func waitForOperatorCR(ctx context.Context, config OperatorDeploymentConfig) error {
	deadline := time.NewTimer(12 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		snapshot, err := readOperatorStatus(ctx, config.KubeconfigPath, config.Namespace)
		if err == nil {
			conditions := make(map[string]string, len(snapshot.Status.Conditions))
			services := make(map[string]bool, len(snapshot.Status.Services))
			for _, condition := range snapshot.Status.Conditions {
				conditions[condition.Type] = condition.Status
			}
			for _, service := range snapshot.Status.Services {
				services[service.Name] = service.Ready
			}
			endpoints := make(map[string]bool)
			if config.Exposure != nil {
				for _, adapter := range config.Exposure.Adapters {
					serviceName := adapter.ServiceName
					if serviceName == "" {
						serviceName = operatorServiceName(adapter.Component)
					}
					// The adapter is intentionally created only after this gate
					// succeeds. Check the operator-owned Service EndpointSlice
					// here; waitForOperatorExposure checks the test-owned adapter
					// EndpointSlice after the adapter manifest is applied.
					endpoints[serviceName] = endpointSliceReady(ctx, config.KubeconfigPath, config.Namespace, serviceName)
				}
			}
			active := make([]string, 0, len(services))
			for name := range services {
				active = append(active, name)
			}
			endpointNames := make([]string, 0, len(endpoints))
			for name := range endpoints {
				endpointNames = append(endpointNames, name)
			}
			lastErr = OperatorStatusReady(OperatorStatusSnapshot{Phase: snapshot.Status.Phase, Conditions: conditions, Services: services, EndpointSlices: endpoints}, active, endpointNames)
			if lastErr == nil {
				return nil
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Kubernaut CR canceled: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("wait for Kubernaut CR timed out: %w", lastErr)
		case <-ticker.C:
		}
	}
}

func readOperatorStatus(ctx context.Context, kubeconfigPath, namespace string) (operatorStatusWire, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace, "get", "kubernaut", operatorCRName, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return operatorStatusWire{}, fmt.Errorf("read Kubernaut status: %w", err)
	}
	var status operatorStatusWire
	if err := json.Unmarshal(output, &status); err != nil {
		return operatorStatusWire{}, fmt.Errorf("decode Kubernaut status: %w", err)
	}
	return status, nil
}

func endpointSliceReady(ctx context.Context, kubeconfigPath, namespace, serviceName string) bool {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace, "get", "endpointslice", "-l", "kubernetes.io/service-name="+serviceName, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	var payload struct {
		Items []struct {
			Endpoints []struct {
				Addresses []string `json:"addresses"`
			} `json:"endpoints"`
		} `json:"items"`
	}
	if json.Unmarshal(output, &payload) != nil {
		return false
	}
	for _, item := range payload.Items {
		for _, endpoint := range item.Endpoints {
			if len(endpoint.Addresses) > 0 {
				return true
			}
		}
	}
	return false
}

func verifyOperatorDeploymentImages(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", config.KubeconfigPath, "-n", config.Namespace, "get", "deployments", "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("read operator Deployments: %w", err)
	}
	var payload struct {
		Items []struct {
			Metadata struct {
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Spec struct {
				Template struct {
					Metadata struct {
						Labels map[string]string `json:"labels"`
					} `json:"metadata"`
					Spec struct {
						Containers []struct {
							Image string `json:"image"`
						} `json:"containers"`
					} `json:"spec"`
				} `json:"template"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return fmt.Errorf("decode operator Deployments: %w", err)
	}
	wanted := make(map[string]string, len(config.RelatedImages))
	for name, image := range config.RelatedImages {
		wanted[operatorComponentLabel(name)] = image
	}
	for _, deployment := range payload.Items {
		if deployment.Metadata.Labels["app.kubernetes.io/managed-by"] != "kubernaut-operator" {
			continue
		}
		component := deployment.Metadata.Labels["app.kubernetes.io/component"]
		if component == "" {
			component = deployment.Spec.Template.Metadata.Labels["app.kubernetes.io/component"]
		}
		expected, ok := wanted[component]
		if !ok {
			continue
		}
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if container.Image != expected {
				return fmt.Errorf("operator Deployment component %s uses %q, want %q", component, container.Image, expected)
			}
		}
	}
	_, _ = fmt.Fprintln(writer, "  ✅ operator Deployment image inventory matches the relatedImages map")
	return nil
}

func waitForOperatorExposure(ctx context.Context, kubeconfigPath, namespace string, plan OperatorExposurePlan, writer io.Writer) error {
	for _, adapter := range plan.Adapters {
		deadline := time.NewTimer(5 * time.Minute)
		for {
			if endpointSliceReady(ctx, kubeconfigPath, namespace, adapter.Name) {
				deadline.Stop()
				break
			}
			select {
			case <-ctx.Done():
				deadline.Stop()
				return ctx.Err()
			case <-deadline.C:
				return fmt.Errorf("EndpointSlice for adapter %s did not become ready", adapter.Name)
			case <-time.After(2 * time.Second):
			}
		}
	}
	_, _ = fmt.Fprintf(writer, "  ✅ operator exposure EndpointSlices ready for %d adapters\n", len(plan.Adapters))
	return nil
}

func operatorNodeAddress(ctx context.Context, kubeconfigPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "get", "nodes", "-o", "jsonpath={.items[0].status.addresses[?(@.type==\"InternalIP\")].address}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve Kind node address: %w", err)
	}
	address := strings.TrimSpace(string(output))
	if address == "" {
		return "", errors.New("kind node has no internal IP")
	}
	return address, nil
}

func writeOperatorInterServiceCA(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	paths := []string{
		"secret/kubernaut-internal-ca",
		"configmap/inter-service-trust-bundle",
	}
	for _, resource := range paths {
		field := "data.ca\\.crt"
		if strings.HasPrefix(resource, "configmap/") {
			field = "data.ca\\.crt"
		}
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace, "get", resource, "-o", "jsonpath={."+field+"}")
		output, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(output)) == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(output)))
		if err != nil {
			// ConfigMap values are returned as plain text by kubectl.
			decoded = []byte(strings.TrimSpace(string(output)))
		}
		path := InterServiceCAPath(kubeconfigPath)
		if err := writeCombinedInterServiceCAPEM(path, decoded); err != nil {
			return fmt.Errorf("write inter-service CA %s: %w", path, err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ operator inter-service CA written to %s\n", path)
		return nil
	}
	return errors.New("operator inter-service CA was not found")
}

func writeOperatorArtifact(directory, name string, data []byte) (string, error) {
	if directory == "" {
		file, err := os.CreateTemp("", "operator-2492-*")
		if err != nil {
			return "", err
		}
		path := file.Name()
		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return "", err
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(path)
			return "", err
		}
		return path, nil
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func operatorDeploymentFailure(ctx context.Context, config OperatorDeploymentConfig, scenario string, cause error, writer io.Writer) error {
	diagnostic := OperatorDiagnosticInput{Scenario: scenario, OperatorSHA: config.Provenance.ResolvedSHA, OperatorImage: config.Provenance.ImageRef}
	if crPath := filepath.Join(config.DiagnosticsDir, "kubernaut-cr.yaml"); config.DiagnosticsDir != "" {
		if data, err := os.ReadFile(crPath); err == nil {
			diagnostic.CRManifest = string(data)
		}
	}
	diagnostic.Status = kubectlDiagnostic(ctx, config.KubeconfigPath, "-n", config.Namespace, "get", "kubernaut", operatorCRName, "-o", "yaml")
	diagnostic.Conditions = diagnostic.Status
	diagnostic.Events = kubectlDiagnostic(ctx, config.KubeconfigPath, "-n", config.Namespace, "get", "events", "--sort-by=.lastTimestamp")
	diagnostic.Dependencies = kubectlDiagnostic(ctx, config.KubeconfigPath, "-n", config.Namespace, "get", "deployments,services,pods")
	data := []byte(BuildOperatorDiagnostics(diagnostic))
	if config.DiagnosticsDir != "" {
		_, _ = writeOperatorArtifact(config.DiagnosticsDir, "operator-diagnostics.txt", data)
	}
	_, _ = fmt.Fprintln(writer, string(data))
	return fmt.Errorf("%s: %w", scenario, cause)
}

func kubectlDiagnostic(ctx context.Context, kubeconfigPath string, args ...string) string {
	commandArgs := append([]string{"--kubeconfig", kubeconfigPath}, args...)
	output, err := exec.CommandContext(ctx, "kubectl", commandArgs...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(output)) + ": " + err.Error()
	}
	return strings.TrimSpace(string(output))
}

// AllocateOperatorExposurePlan reserves a collision-free NodePort/host-port
// block before Kind creation. The listener is closed after selection; Kind owns
// the port once the cluster is created.
func AllocateOperatorExposurePlan(ctx context.Context, namespace, hostname string, components []string) (OperatorExposurePlan, error) {
	if namespace == "" {
		namespace = applicationNamespace
	}
	if hostname == "" {
		hostname = "kubernaut-e2e.local"
	}
	plan := OperatorExposurePlan{Namespace: namespace, Hostname: hostname}
	used := make(map[int]struct{}, len(components))
	for _, component := range components {
		port, err := allocateNodePort(ctx, used)
		if err != nil {
			return OperatorExposurePlan{}, err
		}
		used[port] = struct{}{}
		plan.Adapters = append(plan.Adapters, OperatorExposureAdapter{Name: "kubernaut-e2e-" + component + "-nodeport", Component: component, ServiceName: operatorServiceName(component), NodePort: port, HostPort: port})
	}
	return plan, nil
}

// staticOperatorExposurePorts follows DD-TEST-001's authoritative FullPipeline
// host/NodePort mappings. Operator adapters use existing mappings rather than
// inventing a second dynamic NodePort block.
type staticOperatorExposurePort struct {
	nodePort int
	hostPort int
}

var staticOperatorExposurePorts = map[string]staticOperatorExposurePort{
	"gateway":        {nodePort: 30080, hostPort: 30080},
	"datastorage":    {nodePort: 30081, hostPort: 30081},
	"apifrontend":    {nodePort: 30443, hostPort: 18443},
	"kubernautagent": {nodePort: 30088, hostPort: 8088},
}

func staticOperatorExposurePlan(namespace, hostname string, components []string) (OperatorExposurePlan, bool) {
	plan := OperatorExposurePlan{Namespace: namespace, Hostname: hostname}
	for _, component := range components {
		port, ok := staticOperatorExposurePorts[component]
		if !ok {
			return OperatorExposurePlan{}, false
		}
		plan.Adapters = append(plan.Adapters, OperatorExposureAdapter{
			Name: "kubernaut-e2e-" + component + "-nodeport", Component: component,
			ServiceName: operatorServiceName(component), NodePort: port.nodePort, HostPort: port.hostPort,
		})
	}
	return plan, true
}

// OperatorExposurePlanForCluster preserves the pre-created Kind port mapping
// when a retry reuses an existing cluster. A fresh cluster still allocates a
// collision-free block before creation; a reused cluster must not silently
// select a new host-port block that Kind never mapped.
func OperatorExposurePlanForCluster(ctx context.Context, clusterName, namespace, hostname string, components []string) (OperatorExposurePlan, error) {
	offset, err := DemoHostPortOffset()
	if err != nil {
		return OperatorExposurePlan{}, err
	}
	if plan, ok := staticOperatorExposurePlan(namespace, hostname, components); ok {
		return offsetOperatorExposurePlanHostPorts(plan, offset), nil
	}
	if clusterName != "" && clusterHasLiveNode(ctx, clusterName) {
		return existingKindOperatorExposurePlan(ctx, clusterName, namespace, hostname, components)
	}
	plan, err := AllocateOperatorExposurePlan(ctx, namespace, hostname, components)
	if err != nil {
		return OperatorExposurePlan{}, err
	}
	return offsetOperatorExposurePlanHostPorts(plan, offset), nil
}

func offsetOperatorExposurePlanHostPorts(plan OperatorExposurePlan, offset int) OperatorExposurePlan {
	if offset == 0 {
		return plan
	}
	offsetPlan := plan
	offsetPlan.Adapters = append([]OperatorExposureAdapter(nil), plan.Adapters...)
	for index := range offsetPlan.Adapters {
		offsetPlan.Adapters[index].HostPort += offset
	}
	return offsetPlan
}

func existingKindOperatorExposurePlan(ctx context.Context, clusterName, namespace, hostname string, components []string) (OperatorExposurePlan, error) {
	runtime := "docker"
	if os.Getenv("KIND_EXPERIMENTAL_PROVIDER") == containerRuntimePodman {
		runtime = containerRuntimePodman
	}
	nodeName := clusterName + "-control-plane"
	output, err := exec.CommandContext(ctx, runtime, "inspect", nodeName, "--format", "{{json .NetworkSettings.Ports}}").Output()
	if err != nil {
		return OperatorExposurePlan{}, fmt.Errorf("inspect existing Kind node port mappings: %w", err)
	}
	var portBindings map[string][]struct {
		HostPort string `json:"HostPort"`
	}
	if err := json.Unmarshal(output, &portBindings); err != nil {
		return OperatorExposurePlan{}, fmt.Errorf("decode existing Kind node port mappings: %w", err)
	}
	ports := make([]int, 0, len(portBindings))
	for binding := range portBindings {
		containerPort, _, found := strings.Cut(binding, "/")
		if !found {
			continue
		}
		port, parseErr := strconv.Atoi(containerPort)
		if parseErr != nil || port < 31000 || port > 32767 {
			continue
		}
		entries := portBindings[binding]
		for _, entry := range entries {
			hostPort, hostErr := strconv.Atoi(entry.HostPort)
			if hostErr == nil && hostPort == port {
				ports = append(ports, port)
				break
			}
		}
	}
	sort.Ints(ports)
	if len(ports) < len(components) {
		return OperatorExposurePlan{}, fmt.Errorf("existing Kind cluster %q has %d reusable operator host-port mappings, need %d", clusterName, len(ports), len(components))
	}
	plan := OperatorExposurePlan{Namespace: namespace, Hostname: hostname}
	for index, component := range components {
		port := ports[index]
		plan.Adapters = append(plan.Adapters, OperatorExposureAdapter{Name: "kubernaut-e2e-" + component + "-nodeport", Component: component, ServiceName: operatorServiceName(component), NodePort: port, HostPort: port})
	}
	return plan, nil
}

func allocateNodePort(ctx context.Context, used map[int]struct{}) (int, error) {
	for port := 31000; port <= 32767; port++ {
		if _, exists := used[port]; exists {
			continue
		}
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		_ = listener.Close()
		return port, nil
	}
	return 0, errors.New("no free NodePort available for operator exposure")
}

var operatorExposureRegistry = struct {
	sync.RWMutex
	items map[string]OperatorExposureEvidence
}{items: make(map[string]OperatorExposureEvidence)}

// RememberOperatorExposure associates host-access evidence with the isolated
// kubeconfig so parallel Ginkgo processes can serialize the endpoint contract
// in their SynchronizedBeforeSuite payload.
func RememberOperatorExposure(kubeconfigPath string, evidence OperatorExposureEvidence) {
	operatorExposureRegistry.Lock()
	defer operatorExposureRegistry.Unlock()
	operatorExposureRegistry.items[kubeconfigPath] = evidence
}

// OperatorExposureForKubeconfig returns the evidence recorded by the setup
// process for one isolated cluster.
func OperatorExposureForKubeconfig(kubeconfigPath string) (OperatorExposureEvidence, bool) {
	operatorExposureRegistry.RLock()
	defer operatorExposureRegistry.RUnlock()
	evidence, ok := operatorExposureRegistry.items[kubeconfigPath]
	return evidence, ok
}

// OperatorExposureURL returns the CA-verified host URL for a component adapter.
// The caller supplies the transport that trusts evidence.CAPath; this helper
// only resolves the address and never disables TLS verification.
func OperatorExposureURL(evidence OperatorExposureEvidence, component string) string {
	for _, adapter := range evidence.Adapters {
		if adapter.Component == component {
			return fmt.Sprintf("https://%s:%d", evidence.HostnameOrLoopback(), adapter.HostPort)
		}
	}
	return ""
}

// DeleteOperatorExposureAdapters removes only Services carrying the upstream
// test-owner label. Operator-owned Services and CRD state are never mutated.
func DeleteOperatorExposureAdapters(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "-n", namespace, "delete", "service", "-l", "kubernaut.ai/e2e-owner="+operatorOwnerLabel, "--ignore-not-found=true")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("delete operator exposure adapters: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if trimmed := strings.TrimSpace(string(output)); trimmed != "" {
		_, _ = fmt.Fprintf(writer, "  🗑️  operator exposure cleanup: %s\n", trimmed)
	}
	return nil
}

func (e OperatorExposureEvidence) HostnameOrLoopback() string {
	if e.Hostname != "" {
		return e.Hostname
	}
	return "localhost"
}

// OperatorChartPathsFromEnvironment resolves only explicit qualification
// artifacts. CI supplies unpacked chart artifacts; an explicit source root is
// allowed for development qualification and is rejected when dirty.
func OperatorChartPathsFromEnvironment(ctx context.Context, requestedRefs ...string) (operatorChartPath, dependencyChartPath string, err error) {
	operatorChartPath = strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_CHART_PATH"))
	dependencyChartPath = strings.TrimSpace(os.Getenv("KUBERNAUT_DEPENDENCY_CHART_PATH"))
	if operatorChartPath != "" && dependencyChartPath != "" {
		return operatorChartPath, dependencyChartPath, nil
	}
	root := strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_ROOT"))
	if root == "" {
		return "", "", errors.New("operator chart artifacts are not configured; set KUBERNAUT_OPERATOR_CHART_PATH and KUBERNAUT_DEPENDENCY_CHART_PATH or explicitly set KUBERNAUT_OPERATOR_ROOT")
	}
	statusOutput, statusErr := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain").CombinedOutput()
	if statusErr != nil {
		return "", "", fmt.Errorf("inspect operator source checkout: %w", statusErr)
	}
	if err := ValidateOperatorSourceCheckout(root, string(statusOutput)); err != nil {
		return "", "", err
	}
	resolvedSHA, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", "", fmt.Errorf("resolve operator source SHA: %w", err)
	}
	expectedSHA := strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_SHA"))
	if expectedSHA == "" && len(requestedRefs) > 0 {
		expectedSHA = strings.TrimSpace(requestedRefs[0])
	}
	if expectedSHA == "" {
		expectedSHA = operatorRevision
	}
	if strings.TrimSpace(string(resolvedSHA)) != expectedSHA && strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_ALLOW_NON_PINNED_DEV")) != "true" {
		return "", "", fmt.Errorf("operator source resolved to %s, want %s", strings.TrimSpace(string(resolvedSHA)), expectedSHA)
	}
	return filepath.Join(root, "charts", "kubernaut-operator"), filepath.Join(root, "charts", "kubernaut-development-dependencies"), nil
}

// OperatorOperandImagesFromBuiltImages maps upstream image-builder names to
// the operator's exact image-name contract. Auxiliary images are immutable
// profile inputs and may be overridden by the qualification environment.
func OperatorOperandImagesFromBuiltImages(builtImages map[string]string) (map[string]string, error) {
	images := map[string]string{
		"console": os.Getenv("KUBERNAUT_CONSOLE_IMAGE"),
		// The pinned operator RC marks this helper runAsNonRoot. Its source
		// UBI digest is malformed and the UBI minimal image defaults to root;
		// reuse the pinned non-root console UBI image, which contains the same
		// shell/CA tooling required by build-ca-bundle.
		"init-ubi-minimal": "quay.io/kubernaut-ai/kubernaut-console@sha256:68cf5aebbca09c079fccda92b4c86f4a019a8f6878cf30e5b6724f4e2e48657b",
		"oauth2-proxy":     "quay.io/oauth2-proxy/oauth2-proxy@sha256:37c1570c0427e02fc7c947ef2c04e8995b8347b7abc9fcf1dbb4e376a4b221a7",
	}
	if images["console"] == "" {
		images["console"] = "quay.io/kubernaut-ai/kubernaut-console@sha256:68cf5aebbca09c079fccda92b4c86f4a019a8f6878cf30e5b6724f4e2e48657b"
	}
	for _, name := range operatorOperandImageKeys {
		if _, exists := images[name]; exists && images[name] != "" {
			continue
		}
		image, ok := builtImages[name]
		if !ok || strings.TrimSpace(image) == "" {
			return nil, fmt.Errorf("no stage-matched image artifact for operator operand %q", name)
		}
		images[name] = image
	}
	if err := ValidateOperatorOperandImageMap(images); err != nil {
		return nil, err
	}
	return images, nil
}

// OperatorPortMappings converts adapter evidence into the Kind config entries
// that must exist before the cluster is created.
func OperatorPortMappings(plan OperatorExposurePlan) []KindPortMapping {
	mappings := make([]KindPortMapping, 0, len(plan.Adapters))
	for _, adapter := range plan.Adapters {
		mappings = append(mappings, KindPortMapping{ContainerPort: adapter.NodePort, HostPort: adapter.HostPort, Protocol: "TCP"})
	}
	return mappings
}

// DemoExposureComponents omits the Gateway adapter when Console-first mode
// intentionally disables Gateway. Waiting for a disabled service would make a
// valid user-owned CR look unready (BR-PLATFORM-014).
func DemoExposureComponents(autonomous bool) []string {
	components := []string{"datastorage", "apifrontend", "kubernautagent"}
	if autonomous {
		components = append([]string{"gateway"}, components...)
	}
	return components
}

// resolveDemoOperatorDeploymentInputs combines release artifacts, environment
// paths, and explicit image selectors for a demo deployment.
func resolveDemoOperatorDeploymentInputs(ctx context.Context, options DemoHelmOptions) (operatorChart, dependencyChart, operatorImage, applicationVersion, operatorChartVersion, dependencyChartVersion string, err error) {
	resolved, err := ResolveDemoOperatorArtifacts(DemoArtifactResolutionInput{ReleaseLine: os.Getenv("KUBERNAUT_RELEASE_LINE"), Explicit: options.OperatorArtifacts})
	if err != nil {
		return "", "", "", "", "", "", err
	}
	operatorImage = resolved.OperatorImage
	if !hasOperatorImageOverride(options.OperatorArtifacts) {
		if explicit := strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_IMAGE")); explicit != "" {
			operatorImage = explicit
		}
	}
	if options.OperatorArtifacts.OperatorChart != "" {
		operatorChart = resolved.OperatorChart
		dependencyChart = resolved.DependencyChart
		return operatorChart, dependencyChart, operatorImage, resolved.KubernautVersion, chartVersionOrDefault(operatorChart, resolved.OperatorVersion), chartVersionOrDefault(dependencyChart, resolved.DependencyChartVersion), nil
	}
	if explicitChart := strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_CHART_PATH")); explicitChart != "" && strings.TrimSpace(os.Getenv("KUBERNAUT_DEPENDENCY_CHART_PATH")) == "" && strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_ROOT")) == "" {
		return explicitChart, resolved.DependencyChart, operatorImage, resolved.KubernautVersion, chartVersionOrDefault(explicitChart, resolved.OperatorVersion), resolved.DependencyChartVersion, nil
	}
	if strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_ROOT")) != "" || strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_CHART_PATH")) != "" {
		if requiresExplicitOperatorImage(options.OperatorArtifacts) {
			return "", "", "", "", "", "", errors.New("explicit operator source checkout requires KUBERNAUT_OPERATOR_IMAGE; source is never built implicitly")
		}
		operatorChart, dependencyChart, err = OperatorChartPathsFromEnvironment(ctx, resolved.OperatorRef)
		if err != nil {
			return "", "", "", "", "", "", err
		}
		if options.OperatorArtifacts.DependencyChart != "" {
			dependencyChart = resolved.DependencyChart
		}
		operatorChartVersion = chartVersionOrDefault(operatorChart, resolved.OperatorVersion)
		dependencyChartVersion = chartVersionOrDefault(dependencyChart, resolved.DependencyChartVersion)
		return operatorChart, dependencyChart, operatorImage, resolved.KubernautVersion, operatorChartVersion, dependencyChartVersion, nil
	}
	operatorChart = resolved.OperatorChart
	dependencyChart = resolved.DependencyChart
	return operatorChart, dependencyChart, operatorImage, resolved.KubernautVersion, chartVersionOrDefault(operatorChart, resolved.OperatorVersion), chartVersionOrDefault(dependencyChart, resolved.DependencyChartVersion), nil
}

// requiresExplicitOperatorImage reports whether a source checkout lacks an
// operator image supplied by either the environment or CLI selectors.
func requiresExplicitOperatorImage(artifacts DemoArtifactOverrides) bool {
	return strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_IMAGE")) == "" && !hasOperatorImageOverride(artifacts)
}

func chartVersionOrDefault(chartPath, fallback string) string {
	if strings.HasPrefix(strings.TrimSpace(chartPath), "oci://") {
		return fallback
	}
	data, err := os.ReadFile(filepath.Join(chartPath, "Chart.yaml"))
	if err != nil {
		return fallback
	}
	var chart struct {
		Version string `yaml:"version"`
	}
	if yaml.Unmarshal(data, &chart) != nil || strings.TrimSpace(chart.Version) == "" {
		return fallback
	}
	return strings.TrimSpace(chart.Version)
}

func operatorOperandImagesForDemo(options DemoHelmOptions, version string) (map[string]string, error) {
	repository := strings.TrimRight(options.ImageRepository, "/")
	if repository == "" {
		repository = strings.TrimRight(os.Getenv("KUBERNAUT_IMAGE_REPOSITORY"), "/")
	}
	if repository == "" {
		repository = "quay.io/kubernaut-ai"
	}
	tag := options.ImageTag
	if tag == "" {
		tag = version
	}
	built := make(map[string]string, len(demoLocalImageServices))
	for _, service := range demoLocalImageServices {
		built[service] = fmt.Sprintf("%s/%s:%s", repository, service, tag)
	}
	if options.OperatorArtifacts.KubernautImage != "" {
		built["gateway"] = options.OperatorArtifacts.KubernautImage
	}
	if options.OperatorArtifacts.KubernautDigest != "" && !strings.Contains(built["gateway"], "@sha256:") {
		built["gateway"] += "@" + options.OperatorArtifacts.KubernautDigest
	}
	return OperatorOperandImagesFromBuiltImages(built)
}

func operatorFleetOptionsFromHelm(options *FleetHelmOptions) *OperatorFleetOptions {
	if options == nil {
		return nil
	}
	return &OperatorFleetOptions{
		Enabled:                true,
		GatewayEndpoint:        options.MCPGatewayEndpoint,
		GatewayType:            options.MCPGatewayType,
		GatewayNamespace:       options.MCPGatewayNamespace,
		ScopeBackend:           "fleetmetadatacache",
		ReadCredentialsSecret:  options.OAuth2CredentialsSecret,
		WriteCredentialsSecret: options.WEOAuth2CredentialsSecret,
		TokenURL:               options.OAuth2TokenURL,
		Scopes:                 options.OAuth2Scopes,
	}
}
