package apifrontend_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	eav1alpha1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	isv1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/handler"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/metrics"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/streaming"
	"github.com/jordigilh/kubernaut/test/infrastructure"
)

// goconst dedup: test-fixture literals deduplicated below.
const (
	linux = "linux"
)

// Per-process variables (DD-TEST-010 compliant)
var (
	testEnv   *envtest.Environment
	k8sClient client.Client
	scheme    *runtime.Scheme
	restCfg   *rest.Config

	// K8s clientsets for TokenReview and dynamic operations
	k8sClientset  kubernetes.Interface
	dynamicClient dynamic.Interface

	// Auth infrastructure
	serviceAccountToken string
	jwksKeyPair         *testKeyPair
	jwksServer          *httptest.Server
	jwtValidator        *auth.JWTValidator

	// Shared test infrastructure
	metricsRegistry *metrics.Registry
	testRouter      http.Handler
	routerServer    *httptest.Server

	// Recording audit emitter for test assertions
	auditRecorder *recordingEmitter

	// Shared infra references (process 1 only)
	dsInfra     *infrastructure.DSBootstrapInfra
	kaContainer *infrastructure.ContainerInstance

	// Shared envtest (process 1 only, for SA/RBAC/TokenReview)
	sharedTestEnv *envtest.Environment
	sharedCfg     *rest.Config
)

func TestAPIfrontendIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Frontend Integration Suite")
}

// ═══════════════════════════════════════════════════════════════
// Test Helpers: JWT signing, JWKS server, recording emitter
// ═══════════════════════════════════════════════════════════════

type testKeyPair struct {
	private *rsa.PrivateKey
	keyID   string
}

func newTestKeyPair(kid string) *testKeyPair {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())
	return &testKeyPair{private: key, keyID: kid}
}

func (kp *testKeyPair) jwks() jose.JSONWebKeySet {
	return jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{
			{
				Key:       &kp.private.PublicKey,
				KeyID:     kp.keyID,
				Algorithm: string(jose.RS256),
				Use:       "sig",
			},
		},
	}
}

func (kp *testKeyPair) signToken(claims any) string {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: kp.private},
		(&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), kp.keyID),
	)
	Expect(err).NotTo(HaveOccurred())
	raw, err := josejwt.Signed(signer).Claims(claims).Serialize()
	Expect(err).NotTo(HaveOccurred())
	return raw
}

func standardClaims(issuer, subject string, audiences []string, expiry time.Time) map[string]any {
	return map[string]any{
		"iss":                issuer,
		"sub":                subject,
		"aud":                audiences,
		"exp":                expiry.Unix(),
		"iat":                time.Now().Unix(),
		"preferred_username": subject,
	}
}

func expiredClaims(issuer, subject string, audiences []string) map[string]any {
	return map[string]any{
		"iss":                issuer,
		"sub":                subject,
		"aud":                audiences,
		"exp":                time.Now().Add(-1 * time.Hour).Unix(),
		"iat":                time.Now().Add(-2 * time.Hour).Unix(),
		"preferred_username": subject,
	}
}

func wrongAudienceClaims(issuer, subject string, expiry time.Time) map[string]any {
	return map[string]any{
		"iss":                issuer,
		"sub":                subject,
		"aud":                []string{"wrong-audience"},
		"exp":                expiry.Unix(),
		"iat":                time.Now().Unix(),
		"preferred_username": subject,
	}
}

type recordedEvent struct {
	Event *audit.Event
}

type recordingEmitter struct {
	mu     sync.Mutex
	events []recordedEvent
}

func newRecordingEmitter() *recordingEmitter {
	return &recordingEmitter{}
}

func (r *recordingEmitter) Emit(_ context.Context, event *audit.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *event
	r.events = append(r.events, recordedEvent{Event: &cp})
}

func (r *recordingEmitter) Events() []recordedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedEvent, len(r.events))
	copy(out, r.events)
	return out
}

func (r *recordingEmitter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = r.events[:0]
}

func (r *recordingEmitter) EventsOfType(t audit.EventType) []*audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*audit.Event
	for i := range r.events {
		if r.events[i].Event.Type == t {
			out = append(out, r.events[i].Event)
		}
	}
	return out
}

// newJWKSServer creates an httptest JWKS server serving the given key set.
func newJWKSServer(jwks jose.JSONWebKeySet) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
}

// ═══════════════════════════════════════════════════════════════
// SynchronizedBeforeSuite: DD-TEST-010 Multi-Process Pattern
// Phase 1 (process 1 only): Shared infra (envtest, DS, KA)
// Phase 2 (all processes): Per-process envtest + test harness
// ═══════════════════════════════════════════════════════════════

var _ = SynchronizedBeforeSuite(NodeTimeout(10*time.Minute), func(specCtx SpecContext) []byte {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("AF Integration Suite -- Phase 1: Shared Infrastructure")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Start shared envtest for ServiceAccount / TokenReview / RBAC
	By("Starting shared envtest for auth infrastructure")
	_ = os.Setenv("KUBEBUILDER_CONTROLPLANE_START_TIMEOUT", "60s")

	sharedTestEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		ControlPlane: envtest.ControlPlane{
			APIServer: &envtest.APIServer{
				SecureServing: envtest.SecureServing{
					ListenAddr: envtest.ListenAddr{
						Address: "127.0.0.1",
					},
				},
			},
		},
	}
	var err error
	sharedCfg, err = sharedTestEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(sharedCfg).NotTo(BeNil())
	GinkgoWriter.Printf("Shared envtest started at %s\n", sharedCfg.Host)

	// Create ServiceAccount + RBAC for AF integration tests
	By("Creating ServiceAccount with DataStorage RBAC")
	authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
		sharedCfg,
		"apifrontend-it-client",
		defaultFixture,
		GinkgoWriter,
	)
	Expect(err).ToNot(HaveOccurred())

	// Grant AF SA permission to call KA
	By("Granting AF SA permission to call Kubernaut Agent")
	sharedK8sClient, err := client.New(sharedCfg, client.Options{Scheme: k8sscheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	agentClientRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "kubernaut-agent-client"},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"services"},
				ResourceNames: []string{"kubernaut-agent"},
				Verbs:         []string{"create", "get"},
			},
		},
	}
	err = sharedK8sClient.Create(context.Background(), agentClientRole)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred())
	}

	agentClientBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "apifrontend-kubernaut-agent-client"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "kubernaut-agent-client",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "apifrontend-it-client",
				Namespace: defaultFixture,
			},
		},
	}
	err = sharedK8sClient.Create(context.Background(), agentClientBinding)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred())
	}

	// Create SA for KA service (TokenReview/SAR permissions)
	By("Creating SA for KA service with TokenReview/SAR")
	useHostNetworkForKA := goruntime.GOOS == linux
	kaServiceAuthConfig, err := infrastructure.CreateServiceAccountForHTTPService(
		sharedCfg,
		"kubernaut-agent-service",
		defaultFixture,
		useHostNetworkForKA,
		GinkgoWriter,
	)
	Expect(err).ToNot(HaveOccurred())

	// Grant KA SA DS write permission
	kaDSClientBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "ka-service-datastorage-client"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "data-storage-client",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "kubernaut-agent-service",
				Namespace: defaultFixture,
			},
		},
	}
	err = sharedK8sClient.Create(context.Background(), kaDSClientBinding)
	if !apierrors.IsAlreadyExists(err) {
		Expect(err).ToNot(HaveOccurred())
	}

	// Build images in parallel
	By("Building DS, Mock LLM, and KA images in parallel")
	var (
		dsImageName      string
		mockLLMImageName string
		kaImageName      string
		dsErr            error
		mockErr          error
		kaErr            error
		wg               sync.WaitGroup
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		defer GinkgoRecover()
		dsImageName, dsErr = infrastructure.BuildDataStorageImage(specCtx, "apifrontend", GinkgoWriter)
	}()
	go func() {
		defer wg.Done()
		defer GinkgoRecover()
		mockLLMImageName, mockErr = infrastructure.BuildMockLLMImage(specCtx, "apifrontend", GinkgoWriter)
	}()
	go func() {
		defer wg.Done()
		defer GinkgoRecover()
		kaImageName, kaErr = infrastructure.BuildKubernautAgentImage(specCtx, "apifrontend", GinkgoWriter)
	}()
	wg.Wait()

	Expect(dsErr).ToNot(HaveOccurred(), "DS image must build")
	Expect(mockErr).ToNot(HaveOccurred(), "Mock LLM image must build")
	Expect(kaErr).ToNot(HaveOccurred(), "KA image must build")
	GinkgoWriter.Printf("Images built: DS=%s, MockLLM=%s, KA=%s\n", dsImageName, mockLLMImageName, kaImageName)

	// Start DS infrastructure
	By("Starting DS infrastructure (PostgreSQL, Redis, DataStorage)")
	dsCfg := infrastructure.NewDSBootstrapConfigWithAuth(
		"apifrontend",
		15448, 16394, 18096, 19096,
		"test/integration/apifrontend/config",
		authConfig,
	)
	dsInfra, err = infrastructure.StartDSBootstrap(specCtx, dsCfg, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	// Start Mock LLM
	By("Starting Mock LLM service")
	mockLLMConfig := infrastructure.GetMockLLMConfigForAIAnalysis()
	mockLLMConfig.Port = 18151
	mockLLMConfig.ImageTag = mockLLMImageName
	if goruntime.GOOS == linux {
		mockLLMConfig.Network = "host"
	} else {
		mockLLMConfig.Network = "apifrontend_test_network"
	}
	mockLLMConfig.ServiceName = "mock-llm-apifrontend"
	_, err = infrastructure.StartMockLLMContainer(specCtx, mockLLMConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	// Write KA SA token to disk for container mount
	kaSATokenDir := filepath.Join(os.TempDir(), fmt.Sprintf("af-ka-sa-secrets-%d", time.Now().UnixNano()))
	Expect(os.MkdirAll(kaSATokenDir, 0755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(kaSATokenDir, "token"), []byte(kaServiceAuthConfig.Token), 0644)).To(Succeed())

	// KA config
	kaConfigDir := filepath.Join(os.TempDir(), fmt.Sprintf("af-ka-config-%d", time.Now().UnixNano()))
	Expect(os.MkdirAll(kaConfigDir, 0755)).To(Succeed())

	useHostNetwork := goruntime.GOOS == linux
	var llmEndpoint, dsURL string
	if useHostNetwork {
		llmEndpoint = fmt.Sprintf("http://127.0.0.1:%d", mockLLMConfig.Port)
		dsURL = "http://127.0.0.1:18096"
	} else {
		llmEndpoint = infrastructure.GetMockLLMContainerEndpoint(mockLLMConfig)
		dsURL = "http://host.containers.internal:18096"
	}

	kaConfigContent := fmt.Sprintf(`runtime:
  logging:
    level: "debug"
  server:
    port: 18130
    healthAddr: ":18131"
    metricsAddr: ":18132"
  audit:
    flushIntervalSeconds: 0.1
    bufferSize: 10000
    batchSize: 50
ai:
  llm:
    provider: "openai"
    apiKeyFile: "/etc/kubernautagent-llm-runtime/api-key"
integrations:
  dataStorage:
    url: "%s"
`, dsURL)
	Expect(os.WriteFile(filepath.Join(kaConfigDir, "config.yaml"), []byte(kaConfigContent), 0644)).To(Succeed())

	kaLLMRuntimeDir, err := os.MkdirTemp("", "af-ka-llm-runtime-*")
	Expect(err).ToNot(HaveOccurred())
	Expect(os.Chmod(kaLLMRuntimeDir, 0755)).To(Succeed())
	kaLLMContent := fmt.Sprintf(`model: "mock-model"
endpoint: "%s"
temperature: 0.7
maxRetries: 3
timeoutSeconds: 120
`, llmEndpoint)
	Expect(os.WriteFile(filepath.Join(kaLLMRuntimeDir, "llm-runtime.yaml"), []byte(kaLLMContent), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(kaLLMRuntimeDir, "api-key"), []byte("mock-api-key-for-af-integration-tests"), 0644)).To(Succeed())

	By("Starting KA container")
	kaContainerConfig := infrastructure.GenericContainerConfig{
		Name:  "apifrontend_ka_test",
		Image: kaImageName,
		Env: map[string]string{
			"KUBECONFIG":    "/tmp/kubeconfig",
			"POD_NAMESPACE": defaultFixture,
		},
		Cmd: []string{"-config", "/etc/kubernautagent/config.yaml", "-llm-runtime", "/etc/kubernautagent-llm-runtime/llm-runtime.yaml"},
		Volumes: map[string]string{
			kaConfigDir:                        "/etc/kubernautagent:ro",
			kaLLMRuntimeDir:                    "/etc/kubernautagent-llm-runtime:ro",
			kaServiceAuthConfig.KubeconfigPath: "/tmp/kubeconfig:ro",
			kaSATokenDir:                       "/var/run/secrets/kubernetes.io/serviceaccount:ro",
		},
		HealthCheck: &infrastructure.HealthCheckConfig{
			URL:     "http://127.0.0.1:18131/healthz",
			Timeout: 120 * time.Second,
		},
	}

	if useHostNetwork {
		kaContainerConfig.Network = "host"
	} else {
		kaContainerConfig.Network = "apifrontend_test_network"
		kaContainerConfig.Ports = map[int]int{18130: 18130, 18131: 18131}
		kaContainerConfig.ExtraHosts = []string{"host.containers.internal:host-gateway"}
	}
	kaContainer, err = infrastructure.StartGenericContainer(kaContainerConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())
	GinkgoWriter.Printf("KA started at http://127.0.0.1:18130 (container: %s)\n", kaContainer.ID)

	GinkgoWriter.Println("Phase 1 complete -- passing SA token to all processes")

	type Phase1Data struct {
		Token string `json:"token"`
	}
	data, err := json.Marshal(Phase1Data{Token: authConfig.Token})
	Expect(err).ToNot(HaveOccurred())
	return data
}, func(_ SpecContext, data []byte) {
	// Phase 2: Per-process setup (all processes)
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	type Phase1Data struct {
		Token string `json:"token"`
	}
	var phase1Data Phase1Data
	Expect(json.Unmarshal(data, &phase1Data)).To(Succeed())
	serviceAccountToken = phase1Data.Token
	Expect(serviceAccountToken).NotTo(BeEmpty(), "SA token from Phase 1 must not be empty")

	processNum := GinkgoParallelProcess()
	GinkgoWriter.Printf("[Process %d] Phase 2: Per-process setup\n", processNum)

	Expect(os.Getenv("KUBEBUILDER_ASSETS")).NotTo(BeEmpty(),
		"KUBEBUILDER_ASSETS must be set -- run 'make setup-envtest' first")

	scheme = runtime.NewScheme()
	Expect(isv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
	Expect(eav1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(rbacv1.AddToScheme(scheme)).To(Succeed())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	var err error
	restCfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(restCfg).NotTo(BeNil())

	k8sClient, err = client.New(restCfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	k8sClientset, err = kubernetes.NewForConfig(restCfg)
	Expect(err).NotTo(HaveOccurred())

	dynamicClient, err = dynamic.NewForConfig(restCfg)
	Expect(err).NotTo(HaveOccurred())

	// JWT/JWKS test infrastructure (httptest -- OIDC is truly external)
	jwksKeyPair = newTestKeyPair("af-it-key-1")
	jwksServer = newJWKSServer(jwksKeyPair.jwks())

	authCfg := auth.Config{
		JWT: []auth.ProviderConfig{
			{
				Issuer: auth.IssuerConfig{
					URL:       jwksServer.URL,
					JWKSURL:   jwksServer.URL,
					Audiences: []string{"kubernaut-af"},
				},
			},
		},
		AllowInsecureIssuers: true,
	}

	jwtValidator, err = auth.NewJWTValidator(authCfg,
		auth.WithHTTPClient(jwksServer.Client()),
	)
	Expect(err).NotTo(HaveOccurred())

	// Metrics and audit
	metricsRegistry = metrics.NewRegistry()
	auditRecorder = newRecordingEmitter()

	// Build router with real dependencies
	authMiddleware := auth.MiddlewareWithConfig(auth.MiddlewareConfig{
		Validator:    jwtValidator,
		Logger:       logf.Log.WithName("auth-it"),
		Auditor:      auditRecorder,
		AuthDuration: metricsRegistry.AuthDuration,
	})

	stubA2A := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"status":"ok"}}`))
	})
	stubMCP := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`))
	})

	agentCardHandler, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
		Name:    "kubernaut-af-it",
		URL:     "http://localhost:0",
		Version: "0.0.0-it",
	})
	Expect(err).NotTo(HaveOccurred())

	draining := &sync.Once{}
	_ = draining // suppress unused

	sseTracker := streaming.NewConnectionTracker(metricsRegistry.SSEActiveConnections, 30*time.Second)

	watchClient, wcErr := client.NewWithWatch(restCfg, client.Options{Scheme: scheme})
	Expect(wcErr).NotTo(HaveOccurred(), "WithWatch client must initialize for StatusHandler IT tests")

	statusHandler := handler.NewStatusHandler(watchClient, defaultFixture, logf.Log.WithName("status-it"))

	routerCfg := handler.RouterConfig{
		MetricsRegistry:  metricsRegistry,
		Logger:           logf.Log.WithName("router-it"),
		A2AHandler:       stubA2A,
		MCPHandler:       stubMCP,
		AgentCardHandler: agentCardHandler,
		AuthMiddleware:   authMiddleware,
		ReadyChecker:     func() bool { return true },
		MaxPayloadBytes:  1 << 20,
		SSETracker:       sseTracker,
		StatusHandler:    statusHandler,
	}

	testRouter, err = handler.NewRouter(routerCfg)
	Expect(err).NotTo(HaveOccurred())

	routerServer = httptest.NewServer(testRouter)

	GinkgoWriter.Printf("[Process %d] Router server at %s\n", processNum, routerServer.URL)
})

var _ = SynchronizedAfterSuite(func() {
	processNum := GinkgoParallelProcess()
	GinkgoWriter.Printf("[Process %d] Cleaning up per-process resources...\n", processNum)

	if routerServer != nil {
		routerServer.Close()
	}
	if jwksServer != nil {
		jwksServer.Close()
	}
	if testEnv != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}
}, func() {
	GinkgoWriter.Println("Cleaning up shared infrastructure...")

	if kaContainer != nil {
		_ = infrastructure.StopGenericContainer(kaContainer, GinkgoWriter)
	}
	if dsInfra != nil {
		_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
	}
	if sharedTestEnv != nil {
		_ = sharedTestEnv.Stop()
	}
})

// signValidToken creates a valid JWT signed by the test JWKS keypair.
func signValidToken(subject string) string {
	return jwksKeyPair.signToken(standardClaims(
		jwksServer.URL,
		subject,
		[]string{"kubernaut-af"},
		time.Now().Add(1*time.Hour),
	))
}

// signExpiredToken creates an expired JWT signed by the test JWKS keypair.
func signExpiredToken(subject string) string {
	return jwksKeyPair.signToken(expiredClaims(
		jwksServer.URL,
		subject,
		[]string{"kubernaut-af"},
	))
}

// signWrongAudienceToken creates a JWT with wrong audience.
func signWrongAudienceToken(subject string) string {
	return jwksKeyPair.signToken(wrongAudienceClaims(
		jwksServer.URL,
		subject,
		time.Now().Add(1*time.Hour),
	))
}
