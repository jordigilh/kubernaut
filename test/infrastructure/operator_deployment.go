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
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// operatorRevision is the only operator source revision accepted by the
	// #2492 qualification profile. Release workflows may replace it with an
	// explicitly supplied candidate manifest, but never with a mutable latest.
	operatorRevision           = "f6da13f8347860041c19f1a9eaf2e5029a42e99a9"
	operatorRepository         = "jordigilh/kubernaut-operator"
	applicationNamespace       = "kubernaut-system"
	operatorNamespace          = "kubernaut-operator-system"
	operatorCRName             = "kubernaut"
	operatorCRAPIVersion       = "kubernaut.ai/v1alpha2"
	operatorOwnerLabel         = "upstream-2492"
	operatorDataStorageOperand = "datastorage"
	operatorAPIFrontendOperand = "apifrontend"
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
	PostgreSQLImage string
	ValkeyImage     string
}

// BuildOperatorDependencyArgs keeps external dependency ownership separate from
// the operator chart and records exact image refs in the command.
func BuildOperatorDependencyArgs(options OperatorDependencyOptions) []string {
	args := []string{
		"upgrade", "--install", "kubernaut-dev-bootstrap", options.ChartPath,
		"--namespace", options.Namespace, "--create-namespace",
		"--wait", "--timeout", "10m",
		"--set", "postgresql.commandMode=official",
		"--set-json", "postgresql.securityContext.enabled=false",
		"--set-json", "valkey.securityContext.enabled=false",
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
		return []string{"--set-string", prefix + ".repository=" + imageRef[:lastColon], "--set-string", prefix + ".tag=" + imageRef[lastColon+1:]}
	}
	return []string{"--set-string", prefix + ".repository=" + imageRef}
}

// OperatorLLMOptions is the single profile used by the qualification fixture.
type OperatorLLMOptions struct {
	Provider          string
	Model             string
	Endpoint          string
	CredentialsSecret string
}

// OperatorTLSOptions selects development certificates and their initial host
// identity. ExtraSANs must be present before the first operator reconciliation.
type OperatorTLSOptions struct {
	Mode      string
	ExtraSANs []string
}

// OperatorPolicyOptions contains user-owned policy ConfigMap names. The
// dependency bootstrap chart can populate the corresponding fixture contents.
type OperatorPolicyOptions struct {
	AIAnalysis       string
	SignalProcessing string
}

// OperatorMonitoringOptions contains the explicit generic-Kubernetes endpoints.
type OperatorMonitoringOptions struct {
	PrometheusURL      string
	AlertManagerURL    string
	TLSSource          string
	TLSCAConfigMapName string
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
	PostgreSQLSSLMode     string
	ValkeyHost            string
	PostgreSQLSecret      string
	ValkeySecret          string
	LLM                   OperatorLLMOptions
	TLS                   OperatorTLSOptions
	PolicyConfigMaps      OperatorPolicyOptions
	Monitoring            OperatorMonitoringOptions
	NetworkPolicies       OperatorNetworkPolicyOptions
	Fleet                 *OperatorFleetOptions
	ImageOverrides        map[string]string
	GatewayEnabled        bool
	APIFrontendEnabled    bool
	ConsoleEnabled        bool
	ConsoleAuthSecretName string
	ConsoleIngressHost    string
	ConsoleIngressClass   string
	ConsoleTLSSecretName  string
	IssuerURL             string
	JWKSURL               string
	Audience              string
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
	Host       string `yaml:"host"`
	Port       int    `yaml:"port,omitempty"`
	SecretName string `yaml:"secretName"`
}

type operatorPolicySpec struct {
	Policy operatorPolicyRef `yaml:"policy"`
}

type operatorPolicyRef struct {
	ConfigMapName string `yaml:"configMapName"`
}

type operatorLLMProfile struct {
	Provider          string `yaml:"provider"`
	Model             string `yaml:"model"`
	Endpoint          string `yaml:"endpoint,omitempty"`
	CredentialsSecret string `yaml:"credentialsSecretName"`
}

type operatorAgentSpec struct {
	Investigation     *operatorInvestigationSpec     `yaml:"investigation,omitempty"`
	WorkflowDiscovery *operatorWorkflowDiscoverySpec `yaml:"workflowDiscovery,omitempty"`
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
	Ingress       *operatorIngressSpec   `yaml:"ingress,omitempty"`
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
	Ingress *operatorIngressSpec     `yaml:"ingress,omitempty"`
}

type operatorIngressSpec struct {
	Enabled          bool   `yaml:"enabled"`
	IngressClassName string `yaml:"ingressClassName"`
	Host             string `yaml:"host"`
	TLSSecretName    string `yaml:"tlsSecretName"`
}

type operatorConsoleAuthSpec struct {
	SecretName string `yaml:"secretName,omitempty"`
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
	Source             string                   `yaml:"source,omitempty"`
	CACertConfigMapRef *operatorMonitoringCARef `yaml:"caCertConfigMapRef,omitempty"`
}

type operatorMonitoringCARef struct {
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
	Mode                  string                    `yaml:"mode"`
	DevelopmentSelfSigned map[string]string         `yaml:"developmentSelfSigned"`
	Hooks                 *operatorTLSHooksManifest `yaml:"hooks,omitempty"`
}

type operatorTLSHooksManifest struct {
	TLSCerts *operatorTLSCertsManifest `yaml:"tlsCerts,omitempty"`
}

type operatorTLSCertsManifest struct {
	ExtraSANs []string `yaml:"extraSANs,omitempty"`
}

type operatorRemediationSpec struct {
	DryRun        bool                            `yaml:"dryRun"`
	Effectiveness operatorEffectivenessAssessment `yaml:"effectivenessAssessment,omitempty"`
}

type operatorEffectivenessAssessment struct {
	StabilizationWindow string `yaml:"stabilizationWindow,omitempty"`
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
	if options.Name == "" {
		options.Name = operatorCRName
	}
	if options.Namespace == "" {
		options.Namespace = applicationNamespace
	}
	if options.PostgreSQLSecret == "" || options.ValkeySecret == "" {
		return "", errors.New("kubernaut CR requires PostgreSQL and Valkey Secret names")
	}
	if options.LLM.Provider == "" || options.LLM.Model == "" || options.LLM.CredentialsSecret == "" {
		return "", errors.New("kubernaut CR requires a complete LLM profile")
	}
	if options.TLS.Mode == "" {
		return "", errors.New("kubernaut CR requires an explicit TLS mode")
	}
	if options.Fleet != nil && options.Fleet.Enabled && options.Fleet.WriteCredentialsSecret == "" {
		return "", errors.New("fleet kubernaut CR requires a write OAuth2 Secret")
	}
	if options.APIFrontendEnabled && options.IssuerURL == "" {
		return "", errors.New("kubernaut CR requires an API Frontend issuer URL when API Frontend is enabled")
	}
	audience := options.Audience
	if audience == "" {
		audience = "kubernaut-apifrontend"
	}

	var gateway *operatorGatewaySpec
	if !options.GatewayEnabled {
		gateway = &operatorGatewaySpec{Enabled: pointerToBool(false)}
	}

	var apiFrontend *operatorAPIFrontendSpec
	if options.APIFrontendEnabled {
		auth := &operatorAPIAuthSpec{IssuerURL: options.IssuerURL, Audience: audience}
		if options.JWKSURL != "" {
			auth.JWKSURL = options.JWKSURL
		}
		apiFrontend = &operatorAPIFrontendSpec{Auth: auth}
	} else {
		apiFrontend = &operatorAPIFrontendSpec{Enabled: pointerToBool(false)}
	}

	var monitoring *operatorMonitoringSpec
	if options.Monitoring.PrometheusURL != "" || options.Monitoring.AlertManagerURL != "" {
		monitoring = &operatorMonitoringSpec{}
		if options.Monitoring.PrometheusURL != "" {
			monitoring.Prometheus = &operatorMonitoringEndpoint{URL: options.Monitoring.PrometheusURL}
		}
		if options.Monitoring.AlertManagerURL != "" {
			monitoring.AlertManager = &operatorMonitoringEndpoint{URL: options.Monitoring.AlertManagerURL}
		}
		if options.Monitoring.TLSSource != "" {
			monitoring.TLS = &operatorMonitoringTLS{Source: options.Monitoring.TLSSource}
			if options.Monitoring.TLSCAConfigMapName != "" {
				monitoring.TLS.CACertConfigMapRef = &operatorMonitoringCARef{Name: options.Monitoring.TLSCAConfigMapName}
			}
		}
	}

	var networkPolicies *operatorNetworkPoliciesSpec
	if len(options.NetworkPolicies.IngressCIDRs) > 0 {
		cidrs := append([]string(nil), options.NetworkPolicies.IngressCIDRs...)
		networkPolicies = &operatorNetworkPoliciesSpec{
			Gateway:        &operatorNetworkPolicyIngress{IngressCIDRs: append([]string(nil), cidrs...)},
			APIFrontend:    &operatorNetworkPolicyIngress{IngressCIDRs: append([]string(nil), cidrs...)},
			DataStorage:    &operatorNetworkPolicyIngress{IngressCIDRs: append([]string(nil), cidrs...)},
			KubernautAgent: &operatorNetworkPolicyIngress{IngressCIDRs: append([]string(nil), cidrs...)},
		}
	}

	tls := operatorTLSManifest{
		Mode:                  options.TLS.Mode,
		DevelopmentSelfSigned: map[string]string{},
	}
	if len(options.TLS.ExtraSANs) > 0 {
		tls.Hooks = &operatorTLSHooksManifest{TLSCerts: &operatorTLSCertsManifest{ExtraSANs: options.TLS.ExtraSANs}}
	}

	cr := operatorCRManifest{
		APIVersion: operatorCRAPIVersion,
		Kind:       "Kubernaut",
		Metadata:   operatorMetadata{Name: options.Name, Namespace: options.Namespace},
		Spec: operatorCRSpec{
			Image: operatorImageSpec{Overrides: options.ImageOverrides},
			// The disposable Kind profile may need a profile-specific SSL mode;
			// keep it explicit instead of copying the operator's broader
			// defaulted PostgreSQL block into every CR.
			PostgreSQL:       operatorPostgreSQLSpec{Host: options.PostgreSQLHost, SecretName: options.PostgreSQLSecret, SSLMode: options.PostgreSQLSSLMode},
			Valkey:           operatorValkeySpec{Host: options.ValkeyHost, SecretName: options.ValkeySecret},
			AIAnalysis:       operatorPolicySpec{Policy: operatorPolicyRef{ConfigMapName: options.PolicyConfigMaps.AIAnalysis}},
			SignalProcessing: operatorPolicySpec{Policy: operatorPolicyRef{ConfigMapName: options.PolicyConfigMaps.SignalProcessing}},
			LLMProfiles:      map[string]operatorLLMProfile{"primary": {Provider: options.LLM.Provider, Model: options.LLM.Model, Endpoint: options.LLM.Endpoint, CredentialsSecret: options.LLM.CredentialsSecret}},
			KubernautAgent: operatorAgentSpec{
				Investigation:     &operatorInvestigationSpec{LLMProfileRef: "primary", MaxTurns: 40},
				WorkflowDiscovery: &operatorWorkflowDiscoverySpec{LLMProfileRef: "primary"},
				Interactive:       &operatorInteractiveSpec{Enabled: true, RateLimitPerUser: 100},
				Audit:             &operatorAuditSpec{Enabled: true},
			},
			Gateway:         gateway,
			APIFrontend:     apiFrontend,
			Monitoring:      monitoring,
			NetworkPolicies: networkPolicies,
			TLS:             tls,
		},
	}
	if options.ConsoleEnabled {
		console := &operatorConsoleSpec{Enabled: pointerToBool(true)}
		if options.ConsoleAuthSecretName != "" {
			console.Auth = &operatorConsoleAuthSpec{SecretName: options.ConsoleAuthSecretName}
		}
		if options.ConsoleIngressHost != "" {
			console.Ingress = &operatorIngressSpec{Enabled: true, IngressClassName: options.ConsoleIngressClass, Host: options.ConsoleIngressHost, TLSSecretName: options.ConsoleTLSSecretName}
		}
		cr.Spec.Console = console
	}
	if options.Fleet != nil && options.Fleet.Enabled {
		cr.Spec.Fleet = &operatorFleetSpec{Enabled: true, MCPGateway: operatorFleetGatewaySpec{Endpoint: options.Fleet.GatewayEndpoint, Type: options.Fleet.GatewayType, Namespace: options.Fleet.GatewayNamespace}, ScopeCheck: operatorFleetScopeSpec{Backend: options.Fleet.ScopeBackend}, OAuth2: operatorFleetOAuth2Spec{TokenURL: options.Fleet.TokenURL, CredentialsSecretRef: options.Fleet.ReadCredentialsSecret, Scopes: options.Fleet.Scopes}}
		cr.Spec.WorkflowExecution = &operatorWorkflowExecutionSpec{Fleet: &operatorWorkflowFleetSpec{OAuth2CredentialsSecretRef: options.Fleet.WriteCredentialsSecret}}
	}
	data, err := yaml.Marshal(cr)
	if err != nil {
		return "", fmt.Errorf("marshal Kubernaut CR: %w", err)
	}
	return string(data), nil
}

func pointerToBool(value bool) *bool {
	return &value
}

// KindHostIngressCIDR returns the /32 peer used by the Kind node's container
// network gateway. Podman/Docker host-port DNAT enters the node from this
// gateway address; admitting it through the operator's component
// NetworkPolicies is narrower than admitting all external traffic.
func KindHostIngressCIDR(ctx context.Context, clusterName string) (string, error) {
	if clusterName == "" {
		return "", errors.New("kind host ingress CIDR requires a cluster name")
	}
	runtime := "docker"
	if os.Getenv("KIND_EXPERIMENTAL_PROVIDER") == "podman" {
		runtime = "podman"
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
	OperatorVersion     string `yaml:"version,omitempty"`
	OperatorImage       string `yaml:"image,omitempty"`
	OperatorDigest      string `yaml:"imageDigest,omitempty"`
	OperatorChart       string `yaml:"chart,omitempty"`
	OperatorChartDigest string `yaml:"chartDigest,omitempty"`
	OperatorRef         string `yaml:"ref,omitempty"`
	KubernautVersion    string `yaml:"kubernautVersion,omitempty"`
	KubernautImage      string `yaml:"kubernautImage,omitempty"`
	KubernautDigest     string `yaml:"kubernautImageDigest,omitempty"`
}

type DemoArtifactResolutionInput struct {
	ReleaseLine string
	Explicit    DemoArtifactOverrides
}

type ResolvedDemoOperatorArtifacts struct {
	OperatorVersion  string
	OperatorImage    string
	OperatorChart    string
	KubernautVersion string
	KubernautImage   string
}

func ResolveDemoOperatorArtifacts(input DemoArtifactResolutionInput) (ResolvedDemoOperatorArtifacts, error) {
	version := strings.TrimSpace(input.ReleaseLine)
	if version == "" {
		version = strings.TrimSpace(os.Getenv("KUBERNAUT_RELEASE_VERSION"))
	}
	if version == "" {
		version = "1.6.0-rc26"
	}
	if version == "1.6" {
		version = "1.6.0-rc26"
	}
	operatorVersion := input.Explicit.OperatorVersion
	if operatorVersion == "" {
		operatorVersion = version
	}
	kubernautVersion := input.Explicit.KubernautVersion
	if kubernautVersion == "" {
		kubernautVersion = version
	}
	operatorImage := input.Explicit.OperatorImage
	if operatorImage == "" {
		operatorImage = "quay.io/kubernaut-ai/kubernaut-operator:" + operatorVersion
	}
	operatorChart := input.Explicit.OperatorChart
	if operatorChart == "" {
		operatorChart = "oci://quay.io/kubernaut-ai/charts/kubernaut-operator"
	}
	kubernautImage := input.Explicit.KubernautImage
	if kubernautImage == "" {
		kubernautImage = "quay.io/kubernaut-ai/gateway:" + kubernautVersion
	}
	return ResolvedDemoOperatorArtifacts{OperatorVersion: operatorVersion, OperatorImage: operatorImage, OperatorChart: operatorChart, KubernautVersion: kubernautVersion, KubernautImage: kubernautImage}, nil
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

// ScanForbiddenLiveApplicationChartDeployments distinguishes live deployment
// commands from temporarily allowed offline legacy chart checks.
func ScanForbiddenLiveApplicationChartDeployments(lines []string) []string {
	var violations []string
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if !strings.Contains(lower, "charts/kubernaut") {
			continue
		}
		if strings.Contains(lower, "helm unittest") || strings.Contains(lower, "helm lint") || strings.Contains(lower, "helm template") {
			continue
		}
		if strings.Contains(lower, "helm install") || strings.Contains(lower, "helm upgrade") || strings.Contains(lower, "helm repo") {
			violations = append(violations, strings.TrimSpace(line))
		}
	}
	return violations
}

// ScanForbiddenDeploymentPaths scans the narrowly scoped deployment paths
// used by issue #2492. Offline legacy chart checks remain allowed; live Helm
// install/upgrade commands for charts/kubernaut are returned with file/line
// context for CI to fail closed.
func ScanForbiddenDeploymentPaths(paths []string) ([]string, error) {
	var violations []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read deployment boundary path %q: %w", path, err)
		}
		lines := strings.Split(string(data), "\n")
		for index, line := range ScanForbiddenLiveApplicationChartDeployments(lines) {
			violations = append(violations, fmt.Sprintf("%s:%d: %s", path, index+1, line))
		}
	}
	return violations, nil
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
	seenContainer := make(map[int]struct{}, len(mappings))
	seenHost := make(map[int]struct{}, len(mappings))
	filtered := make([]KindPortMapping, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.ContainerPort < 30000 || mapping.ContainerPort > 32767 {
			return "", fmt.Errorf("container port %d is outside the NodePort range", mapping.ContainerPort)
		}
		if mapping.HostPort < 1 || mapping.HostPort > 65535 {
			return "", fmt.Errorf("host port %d is outside the valid range", mapping.HostPort)
		}
		if _, exists := seenContainer[mapping.ContainerPort]; exists {
			return "", fmt.Errorf("duplicate container port %d", mapping.ContainerPort)
		}
		if _, exists := seenHost[mapping.HostPort]; exists {
			return "", fmt.Errorf("duplicate host port %d", mapping.HostPort)
		}
		if existingHost, exists := existing[mapping.ContainerPort]; exists {
			if existingHost != mapping.HostPort {
				return "", fmt.Errorf("container port %d is already mapped to host port %d", mapping.ContainerPort, existingHost)
			}
			continue
		}
		for existingContainer, existingHost := range existing {
			if existingHost == mapping.HostPort && existingContainer != mapping.ContainerPort {
				return "", fmt.Errorf("host port %d is already mapped from container port %d", mapping.HostPort, existingContainer)
			}
		}
		seenContainer[mapping.ContainerPort] = struct{}{}
		seenHost[mapping.HostPort] = struct{}{}
		filtered = append(filtered, mapping)
	}
	if len(filtered) == 0 {
		return config, nil
	}
	var rendered strings.Builder
	for _, mapping := range filtered {
		protocol := mapping.Protocol
		if protocol == "" {
			protocol = "TCP"
		}
		fmt.Fprintf(&rendered, "  - containerPort: %d\n    hostPort: %d\n    protocol: %s\n", mapping.ContainerPort, mapping.HostPort, protocol)
	}
	needle := "  kubeadmConfigPatches:"
	if strings.Contains(config, "  extraPortMappings:") {
		if index := strings.Index(config, needle); index >= 0 {
			return config[:index] + rendered.String() + config[index:], nil
		}
		return config + "\n" + rendered.String(), nil
	}
	if index := strings.Index(config, needle); index >= 0 {
		return config[:index] + "  extraPortMappings:\n" + rendered.String() + config[index:], nil
	}
	return config + "\n  extraPortMappings:\n" + rendered.String(), nil
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

// OperatorFixtureFiles identifies the dependency-chart fixture inputs. The
// dependency chart owns the resulting Secrets/ConfigMaps; this helper only
// supplies their content through Helm values.
type OperatorFixtureFiles struct {
	LLMCredentials         string
	AIAnalysisPolicy       string
	SignalProcessingPolicy string
}

// OperatorDeploymentConfig is the complete input contract for a live
// operator-managed application deployment.
type OperatorDeploymentConfig struct {
	ClusterName         string
	Namespace           string
	OperatorNamespace   string
	KubeconfigPath      string
	OperatorChartPath   string
	DependencyChartPath string
	OperatorImage       string
	RelatedImages       map[string]string
	PostgreSQLImage     string
	ValkeyImage         string
	CR                  OperatorCROptions
	CRManifestPath      string
	HostIngressCIDR     string
	MonitoringNamespace string
	FixtureFiles        OperatorFixtureFiles
	Exposure            *OperatorExposurePlan
	DiagnosticsDir      string
	Provenance          OperatorArtifactProvenance
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

// InstallOperatorDependencies installs only the independently owned
// PostgreSQL/Valkey and fixture chart. It never creates a Kubernaut CR.
func InstallOperatorDependencies(ctx context.Context, config OperatorDeploymentConfig, writer io.Writer) error {
	if config.KubeconfigPath != "" {
		if err := os.Setenv("KUBECONFIG", config.KubeconfigPath); err != nil {
			return fmt.Errorf("set dependency deployment kubeconfig: %w", err)
		}
	}
	if err := runOperatorHelm(ctx, BuildOperatorDependencyArgs(OperatorDependencyOptions{
		Namespace: config.Namespace, ChartPath: config.DependencyChartPath,
		PostgreSQLImage: config.PostgreSQLImage, ValkeyImage: config.ValkeyImage,
	}), config.FixtureFiles, writer); err != nil {
		return fmt.Errorf("install operator dependencies: %w", err)
	}
	if strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_SHA")) != "" {
		if err := normalizeOperatorValkeyCredentials(ctx, config.KubeconfigPath, config.Namespace, writer); err != nil {
			return err
		}
	}
	if config.FixtureFiles.LLMCredentials != "" {
		if err := ensureOperatorLLMCredentials(ctx, config.KubeconfigPath, config.Namespace, config.FixtureFiles.LLMCredentials, writer); err != nil {
			return err
		}
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

// BuildOperatorUnauthenticatedValkeySecretManifest returns the disposable
// qualification-only Valkey credential shape. The operator-managed FMC
// runtime in the pinned RC26 operand has no Valkey password field; keeping the
// dev cache unauthenticated avoids a credential that no consumer can mount.
// This is never used by production deployment paths.
func BuildOperatorUnauthenticatedValkeySecretManifest(namespace string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: valkey-secret
  namespace: %s
type: Opaque
stringData:
  valkey-secrets.yaml: |
    password: ""
  VALKEY_PASSWORD: ""
`, namespace)
}

func normalizeOperatorValkeyCredentials(ctx context.Context, kubeconfigPath, namespace string, writer io.Writer) error {
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, BuildOperatorUnauthenticatedValkeySecretManifest(namespace)); err != nil {
		return fmt.Errorf("normalize disposable Valkey credentials: %w", err)
	}
	if err := runKubectl(ctx, kubeconfigPath, writer, "rollout", "restart", "deployment/valkey", "-n", namespace); err != nil {
		return fmt.Errorf("restart disposable Valkey after credential normalization: %w", err)
	}
	if err := waitForDeployment(ctx, "valkey", namespace, kubeconfigPath, 120*time.Second, writer); err != nil {
		return fmt.Errorf("wait for normalized Valkey: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ disposable Valkey restarted without authentication for operator FMC compatibility")
	return nil
}

func operatorNamespaceForConfig(config OperatorDeploymentConfig) string {
	if config.OperatorNamespace != "" {
		return config.OperatorNamespace
	}
	return operatorNamespace
}

func ensureOperatorLLMCredentials(ctx context.Context, kubeconfigPath, namespace, credentialsPath string, writer io.Writer) error {
	credentials, err := os.ReadFile(credentialsPath)
	if err != nil {
		return fmt.Errorf("read operator LLM credentials: %w", err)
	}
	manifest := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: llm-credentials-primary
  namespace: %s
type: Opaque
data:
  api_key: %s
`, namespace, base64.StdEncoding.EncodeToString(credentials))
	if err := kubectlApplyManifest(ctx, kubeconfigPath, writer, manifest); err != nil {
		return fmt.Errorf("create user-owned llm-credentials-primary Secret: %w", err)
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
		cmd := exec.CommandContext(ctx, "kind", "load", "docker-image", config.OperatorImage, "--name", config.ClusterName)
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("load operator image into Kind: %w", err)
		}
	}
	operatorNamespace := operatorNamespaceForConfig(config)
	operatorArgs := BuildOperatorChartArgs(OperatorChartInstallOptions{
		Namespace: operatorNamespace, ChartPath: config.OperatorChartPath,
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
	crManifest, err := buildOperatorCRManifest(config)
	if err != nil {
		return OperatorExposureEvidence{}, fmt.Errorf("build Kubernaut CR: %w", err)
	}
	crPath, err := writeOperatorArtifact(config.DiagnosticsDir, "kubernaut-cr.yaml", []byte(crManifest))
	if err != nil {
		return OperatorExposureEvidence{}, fmt.Errorf("write Kubernaut CR: %w", err)
	}
	if err := kubectlApplyFile(ctx, config.KubeconfigPath, crPath, writer); err != nil {
		return OperatorExposureEvidence{}, operatorDeploymentFailure(ctx, config, "IT-2492-CR-001", err, writer)
	}

	if err := waitForOperatorCR(ctx, config); err != nil {
		return OperatorExposureEvidence{}, operatorDeploymentFailure(ctx, config, "IT-2492-CR-001", err, writer)
	}
	if err := verifyOperatorDeploymentImages(ctx, config, writer); err != nil {
		return OperatorExposureEvidence{}, operatorDeploymentFailure(ctx, config, "IT-2492-IMAGE-001", err, writer)
	}
	if config.HostIngressCIDR != "" {
		if err := kubectlApplyManifest(ctx, config.KubeconfigPath, writer,
			BuildOperatorHostIngressNetworkPolicyManifest(config.Namespace, config.HostIngressCIDR)); err != nil {
			return OperatorExposureEvidence{}, operatorDeploymentFailure(ctx, config, "IT-2492-NETWORKPOLICY-001", err, writer)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ test-owned host ingress NetworkPolicy applied for %s\n", config.HostIngressCIDR)
	}
	if config.MonitoringNamespace != "" {
		if err := kubectlApplyManifest(ctx, config.KubeconfigPath, writer,
			BuildOperatorMonitoringIngressNetworkPolicyManifest(config.Namespace, config.MonitoringNamespace)); err != nil {
			return OperatorExposureEvidence{}, operatorDeploymentFailure(ctx, config, "IT-2492-NETWORKPOLICY-002", err, writer)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ test-owned monitoring ingress NetworkPolicy applied for %s\n", config.MonitoringNamespace)
	}

	evidence := OperatorExposureEvidence{CAPath: InterServiceCAPath(config.KubeconfigPath)}
	if config.Exposure != nil && len(config.Exposure.Adapters) > 0 {
		manifest, manifestErr := BuildOperatorExposureManifest(*config.Exposure)
		if manifestErr != nil {
			return OperatorExposureEvidence{}, manifestErr
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
	}
	if err := writeOperatorInterServiceCA(ctx, config.KubeconfigPath, config.Namespace, writer); err != nil {
		return OperatorExposureEvidence{}, fmt.Errorf("write operator CA: %w", err)
	}
	if len(evidence.Adapters) > 0 {
		if err := verifyOperatorExposureTLS(ctx, evidence, writer); err != nil {
			return OperatorExposureEvidence{}, operatorDeploymentFailure(ctx, config, "IT-2492-EXPOSURE-001", err, writer)
		}
	}
	if config.DiagnosticsDir != "" {
		evidenceData, evidenceErr := BuildOperatorExposureEvidenceJSON(evidence)
		if evidenceErr == nil {
			_, _ = writeOperatorArtifact(config.DiagnosticsDir, "operator-exposure.json", evidenceData)
		}
	}
	return evidence, nil
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

func runOperatorHelm(ctx context.Context, args []string, fixtures OperatorFixtureFiles, writer io.Writer) error {
	if fixtures.LLMCredentials != "" {
		args = append(args, "--set-file", "fixtures.llm.apiKey="+fixtures.LLMCredentials)
	}
	if fixtures.AIAnalysisPolicy != "" {
		args = append(args, "--set-file", "fixtures.policies.aiAnalysis="+fixtures.AIAnalysisPolicy)
	}
	if fixtures.SignalProcessingPolicy != "" {
		args = append(args, "--set-file", "fixtures.policies.signalProcessing="+fixtures.SignalProcessingPolicy)
	}
	return runHelmCommand(ctx, args, writer)
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
	if plan, ok := staticOperatorExposurePlan(namespace, hostname, components); ok {
		return plan, nil
	}
	if clusterName != "" && clusterHasLiveNode(ctx, clusterName) {
		return existingKindOperatorExposurePlan(ctx, clusterName, namespace, hostname, components)
	}
	return AllocateOperatorExposurePlan(ctx, namespace, hostname, components)
}

func existingKindOperatorExposurePlan(ctx context.Context, clusterName, namespace, hostname string, components []string) (OperatorExposurePlan, error) {
	runtime := "docker"
	if os.Getenv("KIND_EXPERIMENTAL_PROVIDER") == "podman" {
		runtime = "podman"
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
func OperatorChartPathsFromEnvironment(ctx context.Context) (operatorChartPath, dependencyChartPath string, err error) {
	operatorChartPath = strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_CHART_PATH"))
	dependencyChartPath = strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_DEV_BOOTSTRAP_CHART_PATH"))
	if operatorChartPath != "" && dependencyChartPath != "" {
		return operatorChartPath, dependencyChartPath, nil
	}
	root := strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_ROOT"))
	if root == "" {
		return "", "", errors.New("operator chart artifacts are not configured; set KUBERNAUT_OPERATOR_CHART_PATH and KUBERNAUT_OPERATOR_DEV_BOOTSTRAP_CHART_PATH or explicitly set KUBERNAUT_OPERATOR_ROOT")
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
	if expectedSHA == "" {
		expectedSHA = operatorRevision
	}
	if strings.TrimSpace(string(resolvedSHA)) != expectedSHA && strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_ALLOW_NON_PINNED_DEV")) != "true" {
		return "", "", fmt.Errorf("operator source resolved to %s, want %s", strings.TrimSpace(string(resolvedSHA)), expectedSHA)
	}
	return filepath.Join(root, "charts", "kubernaut-operator"), filepath.Join(root, "charts", "kubernaut-dev-bootstrap"), nil
}

// OperatorOperandImagesFromBuiltImages maps upstream image-builder names to
// the operator's exact image-name contract. Auxiliary images are immutable
// profile inputs and may be overridden by the qualification environment.
func OperatorOperandImagesFromBuiltImages(builtImages map[string]string) (map[string]string, error) {
	images := map[string]string{
		"console":          os.Getenv("KUBERNAUT_CONSOLE_IMAGE"),
		"init-ubi-minimal": "registry.access.redhat.com/ubi10/ubi-minimal@sha256:7dc60d7777e010c50f5e041ff069112b379c3d5eef2823d20871c67cf663f10",
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

func resolveDemoOperatorDeploymentInputs(ctx context.Context, options DemoHelmOptions) (operatorChart, dependencyChart, operatorImage string, version string, err error) {
	resolved, err := ResolveDemoOperatorArtifacts(DemoArtifactResolutionInput{ReleaseLine: os.Getenv("KUBERNAUT_RELEASE_LINE"), Explicit: options.OperatorArtifacts})
	if err != nil {
		return "", "", "", "", err
	}
	operatorImage = resolved.OperatorImage
	if options.OperatorArtifacts.OperatorDigest != "" && !strings.Contains(operatorImage, "@sha256:") {
		operatorImage += "@" + options.OperatorArtifacts.OperatorDigest
	}
	if explicit := strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_IMAGE")); explicit != "" {
		operatorImage = explicit
	}
	if strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_ROOT")) != "" || strings.TrimSpace(os.Getenv("KUBERNAUT_OPERATOR_CHART_PATH")) != "" {
		operatorChart, dependencyChart, err = OperatorChartPathsFromEnvironment(ctx)
		return operatorChart, dependencyChart, operatorImage, resolved.KubernautVersion, err
	}
	operatorChart = resolved.OperatorChart
	if options.OperatorArtifacts.OperatorChartDigest != "" {
		operatorChart += "@" + options.OperatorArtifacts.OperatorChartDigest
	}
	dependencyChart = "oci://quay.io/kubernaut-ai/charts/kubernaut-dev-bootstrap"
	return operatorChart, dependencyChart, operatorImage, resolved.KubernautVersion, nil
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
