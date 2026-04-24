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

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
)

// CredentialTypeField represents a single input field definition for an AWX credential type.
type CredentialTypeField struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	Secret    bool   `json:"secret,omitempty"`
	Multiline bool   `json:"multiline,omitempty"`
}

// CredentialTypeInputs defines the input schema for an AWX credential type.
type CredentialTypeInputs struct {
	Fields []CredentialTypeField `json:"fields"`
}

// CredentialTypeInjectors defines how AWX injects credential values into jobs.
// File templates are rendered by AWX's Jinja2 engine and written to temp files.
// Env vars reference either Jinja2 placeholders or tower.filename.* paths.
type CredentialTypeInjectors struct {
	File map[string]string `json:"file,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
}

// AWXClient defines the interface for AWX/AAP REST API operations.
// Mocked in unit tests; real implementation provided by AWXHTTPClient.
type AWXClient interface {
	LaunchJobTemplate(ctx context.Context, templateID int, extraVars map[string]interface{}) (int, error)
	GetJobStatus(ctx context.Context, jobID int) (*AWXJobStatus, error)
	CancelJob(ctx context.Context, jobID int) error
	FindJobTemplateByName(ctx context.Context, name string) (int, error)

	// Credential lifecycle for dependencies.secrets injection (BR-WE-015).
	// The executor dynamically creates AWX credential types per unique K8s Secret name,
	// then creates ephemeral credentials per WFE execution and cleans them up after completion.
	CreateCredentialType(ctx context.Context, name string, inputs CredentialTypeInputs, injectors CredentialTypeInjectors) (int, error)
	FindCredentialTypeByName(ctx context.Context, name string) (int, error)
	FindCredentialTypeByKind(ctx context.Context, kind string, managed bool) (int, error)
	CreateCredential(ctx context.Context, name string, credTypeID, orgID int, inputs map[string]string) (int, error)
	DeleteCredential(ctx context.Context, credentialID int) error
	LaunchJobTemplateWithCreds(ctx context.Context, templateID int, extraVars map[string]interface{}, credentialIDs []int) (int, error)
	GetJobTemplateCredentials(ctx context.Context, templateID int) ([]int, error)
}

// AWXJobStatus represents the status response from AWX GET /api/v2/jobs/{id}/
type AWXJobStatus struct {
	ID           int    `json:"id"`
	Status       string `json:"status"`
	Failed       bool   `json:"failed"`
	ResultStdout string `json:"result_stdout,omitempty"`
}

const (
	credentialTypePrefix = "kubernaut-secret-"
	credentialPrefix     = "kubernaut-ephemeral-"

	// K8s credential injection (#497, #500, #552).
	// The built-in AWX type was extended for Job Template use in ansible/awx#7629.
	// v2 uses kubeconfig-file injection instead of env vars (#552).
	k8sBuiltinCredTypeName    = "OpenShift or Kubernetes API Bearer Token"
	k8sFallbackCredTypeName   = "kubernaut-k8s-bearer-token"
	k8sFallbackCredTypeNameV2 = "kubernaut-k8s-bearer-token-v2"
	k8sCredentialPrefix       = "kubernaut-k8s-"

	inClusterTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	inClusterCAPath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

	// kubeconfigTemplate is a Jinja2 template rendered by AWX when injecting
	// credentials into an Ansible job. It generates a standard kubeconfig file
	// that kubernetes.core modules read via K8S_AUTH_KUBECONFIG. The template
	// conditionally uses certificate-authority-data or insecure-skip-tls-verify
	// depending on whether ssl_ca_cert is supplied.
	kubeconfigTemplate = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: {{host}}
{% if ssl_ca_cert %}
    certificate-authority-data: {{ssl_ca_cert | b64encode}}
{% else %}
    insecure-skip-tls-verify: true
{% endif %}
  name: target
contexts:
- context:
    cluster: target
    user: kubernaut
  name: default
current-context: default
users:
- name: kubernaut
  user:
    token: {{bearer_token}}`
)

// InClusterCredentials holds Kubernetes API credentials read from the
// controller's in-cluster service account mount.
type InClusterCredentials struct {
	Host   string
	Token  string
	CACert string
}

// ReadInClusterCredentials reads the controller's own K8s API credentials
// from the standard in-cluster mount paths. The token is read fresh on each
// call because projected tokens are rotated by the kubelet.
func ReadInClusterCredentials() (*InClusterCredentials, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, fmt.Errorf("in-cluster environment not detected: KUBERNETES_SERVICE_HOST or KUBERNETES_SERVICE_PORT not set")
	}

	token, err := os.ReadFile(inClusterTokenPath)
	if err != nil {
		return nil, fmt.Errorf("read in-cluster token from %s: %w", inClusterTokenPath, err)
	}

	caCert, err := os.ReadFile(inClusterCAPath)
	if err != nil {
		return nil, fmt.Errorf("read in-cluster CA cert from %s: %w", inClusterCAPath, err)
	}

	return &InClusterCredentials{
		Host:   fmt.Sprintf("https://%s:%s", host, port),
		Token:  string(token),
		CACert: string(caCert),
	}, nil
}

// AnsibleExecutor implements the Executor interface for AWX/AAP workflow execution.
// BR-WE-015: Launches AWX Job Templates and tracks execution status via the AWX REST API.
type AnsibleExecutor struct {
	AWXClient      AWXClient
	K8sClient      client.Client
	Clientset      kubernetes.Interface
	OrganizationID int
	Logger         logr.Logger

	// InClusterCredentialsFn reads K8s API credentials from the controller's
	// service account mount. Replaceable for unit testing.
	InClusterCredentialsFn func() (*InClusterCredentials, error)
}

// NewAnsibleExecutor creates a new AnsibleExecutor with the given AWX client.
// k8sClient is used to read K8s Secrets (dependencies.secrets) and update WFE status.
// clientset is used for TokenRequest API calls (Issue #501).
// orgID is the AWX organization ID for ephemeral credential creation.
func NewAnsibleExecutor(awxClient AWXClient, k8sClient client.Client, clientset kubernetes.Interface, orgID int, logger logr.Logger) *AnsibleExecutor {
	if orgID <= 0 {
		orgID = 1
	}
	return &AnsibleExecutor{
		AWXClient:              awxClient,
		K8sClient:              k8sClient,
		Clientset:              clientset,
		OrganizationID:         orgID,
		Logger:                 logger.WithName("ansible-executor"),
		InClusterCredentialsFn: ReadInClusterCredentials,
	}
}

// Engine returns "ansible".
func (a *AnsibleExecutor) Engine() string {
	return "ansible"
}

// Create launches an AWX Job Template from the WFE spec.
// DD-WE-006: When opts.Dependencies contains secrets, the executor reads them from
// Kubernetes, creates ephemeral AWX credentials, and attaches them to the job launch.
func (a *AnsibleExecutor) Create(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	namespace string,
	opts CreateOptions,
) (*CreateResult, error) {
	cfg, err := a.parseEngineConfig(wfe)
	if err != nil {
		return nil, fmt.Errorf("parse ansible engineConfig: %w", err)
	}

	templateID, err := a.resolveJobTemplate(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve AWX job template: %w", err)
	}

	filterLogger := a.Logger.WithValues("wfe", wfe.Name, "workflowID", wfe.Spec.WorkflowRef.WorkflowID)
	filteredParams := FilterDeclaredParameters(wfe.Spec.Parameters, opts.DeclaredParameterNames, filterLogger)
	extraVars := BuildExtraVars(filteredParams)
	if extraVars == nil {
		extraVars = make(map[string]interface{})
	}
	// System-injected extra_vars. These are set unconditionally AFTER parameter
	// filtering, intentionally overwriting any user-supplied values with the same
	// keys. This prevents spoofing of execution context metadata via schema params.
	extraVars["WFE_NAME"] = wfe.Name
	extraVars["WFE_NAMESPACE"] = wfe.Namespace
	extraVars["RR_NAME"] = wfe.Spec.RemediationRequestRef.Name
	extraVars["RR_NAMESPACE"] = wfe.Spec.RemediationRequestRef.Namespace

	if err := a.injectDependencyConfigMaps(ctx, opts.Dependencies, namespace, extraVars); err != nil {
		return nil, fmt.Errorf("inject dependency configmaps: %w", err)
	}

	credentialIDs, err := a.injectDependencySecrets(ctx, opts.Dependencies, namespace, wfe.Name)
	if err != nil {
		return nil, fmt.Errorf("inject dependency secrets: %w", err)
	}

	// Issue #501: injectK8sCredential now uses TokenRequest when
	// Status.ServiceAccountName is set, falling back to in-cluster creds.
	var warnings []Warning
	k8sCredID, k8sWarnings, k8sErr := a.injectK8sCredential(ctx, wfe, namespace)
	if k8sErr != nil {
		if wfe.Status.ServiceAccountName != "" {
			// Hard failure: operator explicitly requested per-workflow credentials.
			return nil, fmt.Errorf("inject per-workflow K8s credential: %w", k8sErr)
		}
		a.Logger.Error(k8sErr, "Failed to inject K8s credential — playbooks using kubernetes.core will not authenticate",
			"wfe", wfe.Name)
	} else {
		credentialIDs = append(credentialIDs, k8sCredID)
		warnings = append(warnings, k8sWarnings...)
	}

	if len(credentialIDs) > 0 {
		templateCreds, tcErr := a.AWXClient.GetJobTemplateCredentials(ctx, templateID)
		if tcErr != nil {
			a.Logger.Error(tcErr, "Failed to fetch template credentials, launching with ephemeral only",
				"templateID", templateID)
		} else {
			credentialIDs = mergeCredentialIDs(templateCreds, credentialIDs)
		}
	}

	a.Logger.Info("Launching AWX job",
		"templateID", templateID,
		"playbookPath", cfg.PlaybookPath,
		"wfe", wfe.Name,
		"totalCredentials", len(credentialIDs),
	)

	var jobID int
	if len(credentialIDs) > 0 {
		jobID, err = a.AWXClient.LaunchJobTemplateWithCreds(ctx, templateID, extraVars, credentialIDs)
	} else {
		jobID, err = a.AWXClient.LaunchJobTemplate(ctx, templateID, extraVars)
	}
	if err != nil {
		return nil, fmt.Errorf("launch AWX job template %d: %w", templateID, err)
	}

	if len(credentialIDs) > 0 {
		if storeErr := a.storeCredentialIDs(ctx, wfe, credentialIDs); storeErr != nil {
			a.Logger.Error(storeErr, "Failed to store ephemeral credential IDs in WFE status",
				"wfe", wfe.Name, "credentialIDs", credentialIDs)
		}
	}

	return &CreateResult{
		ResourceName: fmt.Sprintf("awx-job-%d", jobID),
		Warnings:     warnings,
	}, nil
}

// injectDependencySecrets reads K8s Secrets declared in dependencies, creates
// dynamic AWX credential types and ephemeral credentials, and returns the
// credential IDs to attach to the AWX job launch.
func (a *AnsibleExecutor) injectDependencySecrets(
	ctx context.Context,
	deps *models.WorkflowDependencies,
	namespace string,
	wfeName string,
) ([]int, error) {
	if deps == nil || len(deps.Secrets) == 0 {
		return nil, nil
	}

	var credentialIDs []int

	for _, dep := range deps.Secrets {
		var secret corev1.Secret
		if err := a.K8sClient.Get(ctx, client.ObjectKey{
			Name:      dep.Name,
			Namespace: namespace,
		}, &secret); err != nil {
			return nil, fmt.Errorf("read dependency secret %q in %q: %w", dep.Name, namespace, err)
		}

		credTypeID, err := a.ensureCredentialType(ctx, dep.Name, secret.Data)
		if err != nil {
			return nil, fmt.Errorf("ensure credential type for secret %q: %w", dep.Name, err)
		}

		inputs := make(map[string]string, len(secret.Data))
		for k, v := range secret.Data {
			inputs[k] = string(v)
		}

		credName := fmt.Sprintf("%s%s-%s", credentialPrefix, dep.Name, wfeName)
		credID, err := a.AWXClient.CreateCredential(ctx, credName, credTypeID, a.OrganizationID, inputs)
		if err != nil {
			return nil, fmt.Errorf("create ephemeral credential for secret %q: %w", dep.Name, err)
		}

		a.Logger.Info("Created ephemeral AWX credential",
			"secret", dep.Name, "credentialID", credID, "credentialType", credTypeID)
		credentialIDs = append(credentialIDs, credID)
	}

	return credentialIDs, nil
}

// injectDependencyConfigMaps reads K8s ConfigMaps declared in dependencies and
// merges their data into extra_vars with a KUBERNAUT_CONFIGMAP_{NAME}_{KEY} prefix.
// ConfigMaps are non-sensitive, so they use AWX extra_vars (not credentials).
func (a *AnsibleExecutor) injectDependencyConfigMaps(
	ctx context.Context,
	deps *models.WorkflowDependencies,
	namespace string,
	extraVars map[string]interface{},
) error {
	if deps == nil || len(deps.ConfigMaps) == 0 {
		return nil
	}

	for _, dep := range deps.ConfigMaps {
		var cm corev1.ConfigMap
		if err := a.K8sClient.Get(ctx, client.ObjectKey{
			Name:      dep.Name,
			Namespace: namespace,
		}, &cm); err != nil {
			return fmt.Errorf("read dependency configmap %q in %q: %w", dep.Name, namespace, err)
		}

		prefix := "KUBERNAUT_CONFIGMAP_" + sanitizeEnvSegment(dep.Name) + "_"
		for k, v := range cm.Data {
			extraVars[prefix+sanitizeEnvSegment(k)] = v
		}

		a.Logger.Info("Injected ConfigMap data into extra_vars",
			"configMap", dep.Name, "keyCount", len(cm.Data))
	}

	return nil
}

// ensureCredentialType finds or creates an AWX credential type for the given
// K8s Secret name. The credential type's env injector maps each secret key to
// KUBERNAUT_SECRET_{SECRET_NAME}_{KEY}.
func (a *AnsibleExecutor) ensureCredentialType(
	ctx context.Context,
	secretName string,
	secretData map[string][]byte,
) (int, error) {
	typeName := credentialTypePrefix + secretName

	id, err := a.AWXClient.FindCredentialTypeByName(ctx, typeName)
	if err == nil {
		return id, nil
	}

	envPrefix := "KUBERNAUT_SECRET_" + sanitizeEnvSegment(secretName) + "_"

	fields := make([]CredentialTypeField, 0, len(secretData))
	envMap := make(map[string]string, len(secretData))

	for key := range secretData {
		fields = append(fields, CredentialTypeField{
			ID:     key,
			Label:  key,
			Type:   "string",
			Secret: true,
		})
		envMap[envPrefix+sanitizeEnvSegment(key)] = "{{" + key + "}}"
	}

	inputs := CredentialTypeInputs{Fields: fields}
	injectors := CredentialTypeInjectors{Env: envMap}

	id, err = a.AWXClient.CreateCredentialType(ctx, typeName, inputs, injectors)
	if err != nil {
		return 0, fmt.Errorf("create AWX credential type %q: %w", typeName, err)
	}

	a.Logger.Info("Created AWX credential type", "name", typeName, "id", id)
	return id, nil
}

// resolveK8sCredentialTypeID finds the AWX credential type suitable for injecting
// K8S_AUTH_* environment variables into the playbook execution environment.
//
// Strategy (5-step, #552):
//  1. FindCredentialTypeByName(k8sBuiltinCredTypeName)   → return if found
//  2. FindCredentialTypeByKind("kubernetes", true)        → return if found (AAP renamed built-in)
//  3. FindCredentialTypeByName(k8sFallbackCredTypeName)   → return if found (existing v1 custom)
//  4. FindCredentialTypeByName(k8sFallbackCredTypeNameV2) → return if found (v2 custom)
//  5. CreateCredentialType(k8sFallbackCredTypeNameV2)     → create with kubeconfig-file injectors
func (a *AnsibleExecutor) resolveK8sCredentialTypeID(ctx context.Context) (int, error) {
	// Step 1: built-in by name
	if id, err := a.AWXClient.FindCredentialTypeByName(ctx, k8sBuiltinCredTypeName); err == nil {
		return id, nil
	}

	// Step 2: built-in by kind (AAP may use a different display name)
	if id, err := a.AWXClient.FindCredentialTypeByKind(ctx, "kubernetes", true); err == nil {
		a.Logger.Info("Resolved K8s credential type via kind-based lookup", "id", id)
		return id, nil
	}

	// Step 3: existing v1 custom type
	if id, err := a.AWXClient.FindCredentialTypeByName(ctx, k8sFallbackCredTypeName); err == nil {
		return id, nil
	}

	// Step 4: existing v2 custom type
	if id, err := a.AWXClient.FindCredentialTypeByName(ctx, k8sFallbackCredTypeNameV2); err == nil {
		return id, nil
	}

	// Step 5: create v2 custom type with kubeconfig-file injectors
	a.Logger.Info("All K8s credential type lookups failed, creating v2 custom type",
		"name", k8sFallbackCredTypeNameV2)

	inputs := CredentialTypeInputs{
		Fields: []CredentialTypeField{
			{ID: "host", Label: "Kubernetes API Host", Type: "string"},
			{ID: "bearer_token", Label: "API Bearer Token", Type: "string", Secret: true},
			{ID: "ssl_ca_cert", Label: "CA Certificate", Type: "string", Secret: true, Multiline: true},
		},
	}

	injectors := CredentialTypeInjectors{
		File: map[string]string{
			"template.kubeconfig": kubeconfigTemplate,
		},
		Env: map[string]string{
			"K8S_AUTH_KUBECONFIG": "{{tower.filename.kubeconfig}}",
		},
	}

	id, err := a.AWXClient.CreateCredentialType(ctx, k8sFallbackCredTypeNameV2, inputs, injectors)
	if err != nil {
		return 0, fmt.Errorf("create K8s credential type %q: %w", k8sFallbackCredTypeNameV2, err)
	}

	a.Logger.Info("Created v2 custom K8s credential type", "name", k8sFallbackCredTypeNameV2, "id", id)
	return id, nil
}

// k8sCredentialInputs holds the credential field values for a K8s API credential.
// Provides type-safe construction before conversion to the generic map expected
// by CreateCredential.
type k8sCredentialInputs struct {
	Host        string
	BearerToken string
	CACert      string
}

func (i k8sCredentialInputs) toMap() map[string]string {
	m := map[string]string{
		"host":         i.Host,
		"bearer_token": i.BearerToken,
	}
	if i.CACert != "" {
		m["ssl_ca_cert"] = i.CACert
	}
	return m
}

// injectK8sCredential creates an ephemeral AWX credential containing the WE
// controller's in-cluster K8s API credentials so that playbooks using
// kubernetes.core modules can authenticate to the cluster API.
// When the CA cert is empty (e.g. in dev clusters), the ssl_ca_cert input is
// omitted so the kubeconfig template falls back to insecure-skip-tls-verify (#552).
// injectK8sCredential obtains K8s API credentials and creates an ephemeral AWX
// credential. Issue #501: When wfe.Status.ServiceAccountName is set, a short-lived
// token is obtained via the TokenRequest API scoped to that SA. When empty, the
// controller's own in-cluster credentials are used (backward-compatible fallback
// from Issue #500).
func (a *AnsibleExecutor) injectK8sCredential(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	namespace string,
) (int, []Warning, error) {
	var creds *InClusterCredentials
	var warnings []Warning

	if wfe.Status.ServiceAccountName != "" {
		tokenCreds, tokenWarnings, err := a.requestTokenForSA(ctx, wfe, namespace)
		if err != nil {
			return 0, nil, fmt.Errorf("TokenRequest for SA %q in %q: %w", wfe.Status.ServiceAccountName, namespace, err)
		}
		creds = tokenCreds
		warnings = tokenWarnings
	} else {
		var err error
		creds, err = a.InClusterCredentialsFn()
		if err != nil {
			return 0, nil, fmt.Errorf("read in-cluster K8s credentials: %w", err)
		}
	}

	typeID, err := a.resolveK8sCredentialTypeID(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("resolve K8s credential type: %w", err)
	}

	k8sInputs := k8sCredentialInputs{
		Host:        creds.Host,
		BearerToken: creds.Token,
		CACert:      creds.CACert,
	}
	if creds.CACert == "" {
		a.Logger.Info("WARNING: Empty CA cert — playbook will use insecure TLS", "wfe", wfe.Name)
	}

	credName := k8sCredentialPrefix + wfe.Name
	credID, err := a.AWXClient.CreateCredential(ctx, credName, typeID, a.OrganizationID, k8sInputs.toMap())
	if err != nil {
		return 0, nil, fmt.Errorf("create K8s credential %q: %w", credName, err)
	}

	a.Logger.Info("Created ephemeral K8s credential",
		"credentialID", credID, "credentialType", typeID, "wfe", wfe.Name,
		"source", credSourceLabel(wfe.Status.ServiceAccountName))
	return credID, warnings, nil
}

const defaultTokenExpirationSeconds = 3600

// requestTokenForSA uses the TokenRequest API to obtain a short-lived token for
// the workflow's designated ServiceAccount. It also validates the granted TTL
// against the WFE execution timeout and returns a warning if the API server
// shortened it.
func (a *AnsibleExecutor) requestTokenForSA(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	namespace string,
) (*InClusterCredentials, []Warning, error) {
	requestedExpSeconds := int64(defaultTokenExpirationSeconds)
	var executionTimeout time.Duration
	if wfe.Spec.ExecutionConfig != nil && wfe.Spec.ExecutionConfig.Timeout != nil {
		executionTimeout = wfe.Spec.ExecutionConfig.Timeout.Duration
		if secs := int64(executionTimeout.Seconds()); secs > requestedExpSeconds {
			requestedExpSeconds = secs
		}
	}

	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &requestedExpSeconds,
		},
	}

	tokenResp, err := a.Clientset.CoreV1().ServiceAccounts(namespace).CreateToken(
		ctx, wfe.Status.ServiceAccountName, treq, metav1.CreateOptions{},
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create token for SA %q: %w", wfe.Status.ServiceAccountName, err)
	}

	if tokenResp.Status.Token == "" {
		return nil, nil, fmt.Errorf("TokenRequest returned empty token for SA %q/%q", namespace, wfe.Status.ServiceAccountName)
	}

	// TTL validation: compare granted expiration against execution timeout
	var warnings []Warning
	if executionTimeout > 0 && tokenResp.Status.ExpirationTimestamp.Time.Before(time.Now().Add(executionTimeout)) {
		grantedTTL := time.Until(tokenResp.Status.ExpirationTimestamp.Time).Truncate(time.Second)
		msg := fmt.Sprintf("granted token TTL %s is shorter than execution timeout %s — playbook may receive 401 errors if it outlives the token",
			grantedTTL, executionTimeout)
		a.Logger.Info("WARNING: "+msg,
			"wfe", wfe.Name, "sa", wfe.Status.ServiceAccountName,
			"grantedTTL", grantedTTL, "executionTimeout", executionTimeout)
		warnings = append(warnings, Warning{
			Type:    workflowexecutionv1alpha1.ConditionTokenTTLInsufficient,
			Reason:  workflowexecutionv1alpha1.ReasonTokenTTLShortened,
			Message: msg,
		})
	}

	host, caCert := readAPIServerEndpoint()

	return &InClusterCredentials{
		Host:   host,
		Token:  tokenResp.Status.Token,
		CACert: caCert,
	}, warnings, nil
}

func credSourceLabel(saName string) string {
	if saName != "" {
		return "TokenRequest(" + saName + ")"
	}
	return "in-cluster"
}

// readAPIServerEndpoint reads the K8s API server address and CA cert from the
// standard in-cluster environment. These values are shared between TokenRequest
// and in-cluster credential paths — only the bearer token differs.
func readAPIServerEndpoint() (host, caCert string) {
	h := os.Getenv("KUBERNETES_SERVICE_HOST")
	p := os.Getenv("KUBERNETES_SERVICE_PORT")
	if h != "" && p != "" {
		host = fmt.Sprintf("https://%s:%s", h, p)
	}
	ca, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	return host, string(ca)
}

// storeCredentialIDs persists ephemeral AWX credential IDs in the WFE status
// subresource so Cleanup() can delete them after execution. Uses the status
// subresource to avoid triggering spec immutability validation (ADR-001).
func (a *AnsibleExecutor) storeCredentialIDs(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	credentialIDs []int,
) error {
	wfe.Status.EphemeralCredentialIDs = credentialIDs
	return a.K8sClient.Status().Update(ctx, wfe)
}

// GetStatus polls AWX for the job status and maps it to an ExecutionResult.
func (a *AnsibleExecutor) GetStatus(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	namespace string,
) (*ExecutionResult, error) {
	if wfe.Status.ExecutionRef == nil {
		return nil, fmt.Errorf("no execution ref set on WFE %s/%s", wfe.Namespace, wfe.Name)
	}

	jobID, err := parseAWXJobID(wfe.Status.ExecutionRef.Name)
	if err != nil {
		return nil, fmt.Errorf("parse AWX job ID from executionRef %q: %w", wfe.Status.ExecutionRef.Name, err)
	}

	status, err := a.AWXClient.GetJobStatus(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("get AWX job %d status: %w", jobID, err)
	}

	return MapAWXStatusToResult(status), nil
}

// Cleanup deletes ephemeral AWX credentials (if any) and cancels the AWX job.
func (a *AnsibleExecutor) Cleanup(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
	namespace string,
) error {
	a.cleanupEphemeralCredentials(ctx, wfe)

	if wfe.Status.ExecutionRef == nil {
		return nil
	}

	jobID, err := parseAWXJobID(wfe.Status.ExecutionRef.Name)
	if err != nil {
		a.Logger.Info("Cannot parse AWX job ID for cleanup, skipping", "executionRef", wfe.Status.ExecutionRef.Name)
		return nil
	}

	if err := a.AWXClient.CancelJob(ctx, jobID); err != nil {
		if strings.Contains(err.Error(), "405") {
			a.Logger.Info("AWX job already completed, cancel not needed", "jobID", jobID)
			return nil
		}
		a.Logger.Error(err, "Failed to cancel AWX job during cleanup", "jobID", jobID)
		return fmt.Errorf("cancel AWX job %d: %w", jobID, err)
	}

	return nil
}

// cleanupEphemeralCredentials reads ephemeral AWX credential IDs from the WFE
// status and deletes each one. Errors are logged but do not fail the cleanup --
// the AWX job cancellation must still proceed.
func (a *AnsibleExecutor) cleanupEphemeralCredentials(
	ctx context.Context,
	wfe *workflowexecutionv1alpha1.WorkflowExecution,
) {
	if len(wfe.Status.EphemeralCredentialIDs) == 0 {
		return
	}

	for _, credID := range wfe.Status.EphemeralCredentialIDs {
		if err := a.AWXClient.DeleteCredential(ctx, credID); err != nil {
			a.Logger.Error(err, "Failed to delete ephemeral AWX credential", "credentialID", credID)
			continue
		}
		a.Logger.Info("Deleted ephemeral AWX credential", "credentialID", credID)
	}
}

func (a *AnsibleExecutor) parseEngineConfig(wfe *workflowexecutionv1alpha1.WorkflowExecution) (*models.AnsibleEngineConfig, error) {
	if wfe.Spec.WorkflowRef.EngineConfig == nil {
		return nil, fmt.Errorf("engineConfig is required for ansible engine")
	}

	parsed, err := models.ParseEngineConfig("ansible", wfe.Spec.WorkflowRef.EngineConfig.Raw)
	if err != nil {
		return nil, err
	}

	cfg, ok := parsed.(*models.AnsibleEngineConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected engineConfig type: %T", parsed)
	}

	return cfg, nil
}

func (a *AnsibleExecutor) resolveJobTemplate(ctx context.Context, cfg *models.AnsibleEngineConfig) (int, error) {
	if cfg.JobTemplateName == "" {
		return 0, fmt.Errorf("jobTemplateName is required in ansible engineConfig for v1.0")
	}
	return a.AWXClient.FindJobTemplateByName(ctx, cfg.JobTemplateName)
}

// BuildExtraVars converts workflow parameters (map[string]string) to typed JSON values
// for AWX extra_vars. Attempts type coercion: integers, booleans, floats, JSON arrays/objects
// are converted to their native types. Plain strings remain strings.
func BuildExtraVars(params map[string]string) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}

	extraVars := make(map[string]interface{}, len(params))
	for k, v := range params {
		extraVars[k] = coerceValue(v)
	}
	return extraVars
}

func coerceValue(s string) interface{} {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	if b, err := strconv.ParseBool(s); err == nil {
		return b
	}
	if (strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) ||
		(strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) {
		var parsed interface{}
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			return parsed
		}
	}
	return s
}

// MapAWXStatusToResult maps an AWX job status to an ExecutionResult.
// AWX states: pending, waiting, running, successful, failed, error, canceled
func MapAWXStatusToResult(status *AWXJobStatus) *ExecutionResult {
	phase, reason, message := mapAWXStatusToPhase(status.Status)

	if status.ResultStdout != "" && phase == workflowexecutionv1alpha1.PhaseFailed {
		message = status.ResultStdout
	}

	var condStatus corev1.ConditionStatus
	switch phase {
	case workflowexecutionv1alpha1.PhaseCompleted:
		condStatus = corev1.ConditionTrue
	case workflowexecutionv1alpha1.PhaseFailed:
		condStatus = corev1.ConditionFalse
	default:
		condStatus = corev1.ConditionUnknown
	}

	summary := &workflowexecutionv1alpha1.ExecutionStatusSummary{
		Status:  condStatus,
		Reason:  reason,
		Message: message,
	}

	return &ExecutionResult{
		Phase:   phase,
		Reason:  reason,
		Message: message,
		Summary: summary,
	}
}

func mapAWXStatusToPhase(awxStatus string) (phase, reason, message string) {
	switch awxStatus {
	case "pending", "waiting":
		return workflowexecutionv1alpha1.PhasePending, "AWXJobPending", "AWX job is queued"
	case "running":
		return workflowexecutionv1alpha1.PhaseRunning, "AWXJobRunning", "AWX job is executing"
	case "successful":
		return workflowexecutionv1alpha1.PhaseCompleted, "AWXJobSuccessful", "AWX job completed successfully"
	case "failed":
		return workflowexecutionv1alpha1.PhaseFailed, "AWXJobFailed", "AWX job execution failed"
	case "error":
		return workflowexecutionv1alpha1.PhaseFailed, "AWXJobError", "AWX job encountered an internal error"
	case "canceled":
		return workflowexecutionv1alpha1.PhaseFailed, "AWXJobCanceled", "AWX job was canceled"
	default:
		return workflowexecutionv1alpha1.PhasePending, "AWXJobUnknown", fmt.Sprintf("Unknown AWX job status: %s", awxStatus)
	}
}

// sanitizeEnvSegment converts a Kubernetes resource name segment into an
// environment-variable-safe uppercase format: hyphens become underscores.
func sanitizeEnvSegment(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, "-", "_"))
}

func parseAWXJobID(executionRefName string) (int, error) {
	const prefix = "awx-job-"
	if !strings.HasPrefix(executionRefName, prefix) {
		return 0, fmt.Errorf("execution ref %q does not have awx-job- prefix", executionRefName)
	}
	return strconv.Atoi(executionRefName[len(prefix):])
}

func mergeCredentialIDs(templateIDs, ephemeralIDs []int) []int {
	seen := make(map[int]struct{}, len(templateIDs)+len(ephemeralIDs))
	merged := make([]int, 0, len(templateIDs)+len(ephemeralIDs))
	for _, id := range templateIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			merged = append(merged, id)
		}
	}
	for _, id := range ephemeralIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			merged = append(merged, id)
		}
	}
	return merged
}
