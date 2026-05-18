package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ds"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
	"github.com/jordigilh/kubernaut-apifrontend/internal/ka"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApiFrontend Integration Suite — Real Containers")
}

const (
	afITPostgresPort    = 14400
	afITRedisPort       = 14401
	afITDataStoragePort = 14402
	afITMetricsPort     = 14403
	afITKAPort          = 14404
	afITKAHealthPort    = 14405
	afITMockLLMPort     = 14406
	afITKAMetricsPort   = 14407
)

var (
	dsInfra      *infrastructure.DSBootstrapInfra
	kaContainer  *infrastructure.ContainerInstance
	mcpHandler   http.Handler
	testUser     *auth.UserIdentity
	auditCapture *testAuditor
	itDynClient  dynamic.Interface
	itMetrics    *handler.MCPBridgeMetrics
	itKAToken    string
	itEnvtestCfg *rest.Config
)

type testAuditor struct {
	mu     sync.Mutex
	events []*audit.Event
}

func (a *testAuditor) Emit(_ context.Context, event *audit.Event) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
}

func (a *testAuditor) Events() []*audit.Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := make([]*audit.Event, len(a.events))
	copy(cp, a.events)
	return cp
}

func (a *testAuditor) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = nil
}

var _ = SynchronizedBeforeSuite(
	func() []byte {
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		_, _ = fmt.Fprintln(GinkgoWriter, "ApiFrontend IT - PHASE 1: Container Infrastructure Setup")
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		specCtx := context.Background()

		// --- envtest: real K8s API with kubernaut CRDs (IT-ENV-001, DD-AUTH-014) ---
		By("Starting envtest with kubernaut + apifrontend CRDs")
		kubernautRoot := findKubernautModuleRoot()
		kubernautCRDPath := filepath.Join(kubernautRoot, "config", "crd", "bases")
		_, thisFile, _, ok := runtime.Caller(0)
		Expect(ok).To(BeTrue())
		projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
		afCRDPath := filepath.Join(projectRoot, "config", "crd", "bases")

		envtestBinDir := os.Getenv("KUBEBUILDER_ASSETS")
		if envtestBinDir == "" {
			out, setupErr := exec.Command("setup-envtest", "use", "-p", "path").Output()
			if setupErr == nil {
				envtestBinDir = strings.TrimSpace(string(out))
			}
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "envtest binary dir: %s\n", envtestBinDir)

		testEnv := &envtest.Environment{
			CRDDirectoryPaths:     []string{kubernautCRDPath, afCRDPath},
			ErrorIfCRDPathMissing: true,
			BinaryAssetsDirectory: envtestBinDir,
		}
		k8sCfg, err := testEnv.Start()
		Expect(err).ToNot(HaveOccurred(), "envtest should start")
		_, _ = fmt.Fprintf(GinkgoWriter, "envtest started at %s (CRDs: %s, %s)\n", k8sCfg.Host, kubernautCRDPath, afCRDPath)

		By("Creating ServiceAccount for DataStorage authentication")
		authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
			k8sCfg, "apifrontend-it-sa", "default", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred(), "ServiceAccount creation should succeed")

		// --- Image build/pull (parallel) ---
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

		Expect(dsErr).ToNot(HaveOccurred(), "DataStorage image must build/pull successfully")
		Expect(mockErr).ToNot(HaveOccurred(), "Mock LLM image must build/pull successfully")
		Expect(kaErr).ToNot(HaveOccurred(), "KA image must build/pull successfully")
		_, _ = fmt.Fprintf(GinkgoWriter, "All images ready: DS=%s, MockLLM=%s, KA=%s\n", dsImageName, mockLLMImageName, kaImageName)

		// --- DS Bootstrap (PG + Redis + DS with auth) ---
		By("Starting DataStorage infrastructure (PostgreSQL + Redis + DS)")
		configDir := resolveConfigDir()
		dsCfg := infrastructure.NewDSBootstrapConfigWithAuth(
			"apifrontend",
			afITPostgresPort, afITRedisPort, afITDataStoragePort, afITMetricsPort,
			configDir,
			authConfig,
		)
		dsInfra, err = infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "DS infrastructure must start")
		dsInfra.SharedTestEnv = testEnv
		_, _ = fmt.Fprintf(GinkgoWriter, "DataStorage ready at %s\n", dsInfra.ServiceURL)

		// --- Mock LLM ---
		By("Starting Mock LLM container")
		useHostNetwork := runtime.GOOS == "linux"
		mockLLMCfg := infrastructure.MockLLMConfig{
			ServiceName:   "apifrontend",
			Port:          afITMockLLMPort,
			ContainerName: "apifrontend_mockllm_test",
			ImageTag:      mockLLMImageName,
		}
		if useHostNetwork {
			mockLLMCfg.Network = "host"
		} else {
			mockLLMCfg.Network = "apifrontend_test_network"
		}
		_, err = infrastructure.StartMockLLMContainer(specCtx, mockLLMCfg, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Mock LLM must start")
		_, _ = fmt.Fprintf(GinkgoWriter, "Mock LLM ready at port %d\n", afITMockLLMPort)

		// --- KA ---
		By("Creating KA configuration files")
		var llmEndpoint, dsURL string
		if useHostNetwork {
			llmEndpoint = fmt.Sprintf("http://127.0.0.1:%d", afITMockLLMPort)
			dsURL = fmt.Sprintf("http://127.0.0.1:%d", afITDataStoragePort)
		} else {
			llmEndpoint = infrastructure.GetMockLLMContainerEndpoint(mockLLMCfg)
			dsURL = fmt.Sprintf("http://host.containers.internal:%d", afITDataStoragePort)
		}

		kaConfigDir, err := os.MkdirTemp("", "apifrontend-ka-config-*")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.Chmod(kaConfigDir, 0o755)).To(Succeed())

		kaConfigContent := fmt.Sprintf(`runtime:
  logging:
    level: "debug"
  server:
    port: %d
    healthAddr: ":%d"
    metricsAddr: ":%d"
  audit:
    flushIntervalSeconds: 0.1
    bufferSize: 10000
    batchSize: 50
ai:
  llm:
    provider: "openai"
integrations:
  dataStorage:
    url: "%s"
`, afITKAPort, afITKAHealthPort, afITKAMetricsPort, dsURL)
		Expect(os.WriteFile(filepath.Join(kaConfigDir, "config.yaml"), []byte(kaConfigContent), 0o644)).To(Succeed())

		kaLLMRuntimeDir, err := os.MkdirTemp("", "apifrontend-ka-llm-runtime-*")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.Chmod(kaLLMRuntimeDir, 0o755)).To(Succeed())
		kaLLMRuntimeContent := fmt.Sprintf(`model: "mock-model"
endpoint: "%s"
apiKey: "mock-api-key-for-integration-tests"
temperature: 0.7
maxRetries: 3
timeoutSeconds: 120
`, llmEndpoint)
		Expect(os.WriteFile(filepath.Join(kaLLMRuntimeDir, "llm-runtime.yaml"), []byte(kaLLMRuntimeContent), 0o644)).To(Succeed())

		// KA SA token directory (DD-AUTH-014)
		kaSATokenDir, err := os.MkdirTemp("", "apifrontend-ka-sa-*")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.Chmod(kaSATokenDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(kaSATokenDir, "token"), []byte(authConfig.Token), 0o644)).To(Succeed())

		By("Starting Kubernaut Agent container")
		kaContainerConfig := infrastructure.GenericContainerConfig{
			Name:  "apifrontend_ka_test",
			Image: kaImageName,
			Env: map[string]string{
				"KUBECONFIG":    "/tmp/kubeconfig",
				"POD_NAMESPACE": "default",
			},
			Cmd: []string{"-config", "/etc/kubernautagent/config.yaml", "-llm-runtime", "/etc/kubernautagent-llm-runtime/llm-runtime.yaml"},
			Volumes: map[string]string{
				kaConfigDir:               "/etc/kubernautagent:ro",
				kaLLMRuntimeDir:           "/etc/kubernautagent-llm-runtime:ro",
				authConfig.KubeconfigPath: "/tmp/kubeconfig:ro",
				kaSATokenDir:              "/var/run/secrets/kubernetes.io/serviceaccount:ro",
			},
			HealthCheck: &infrastructure.HealthCheckConfig{
				URL:     fmt.Sprintf("http://127.0.0.1:%d/healthz", afITKAHealthPort),
				Timeout: 120 * time.Second,
			},
		}

		if useHostNetwork {
			kaContainerConfig.Network = "host"
		} else {
			kaContainerConfig.Network = "apifrontend_test_network"
			kaContainerConfig.Ports = map[int]int{
				afITKAPort:        afITKAPort,
				afITKAHealthPort:  afITKAHealthPort,
				afITKAMetricsPort: afITKAMetricsPort,
			}
			kaContainerConfig.ExtraHosts = []string{
				"host.containers.internal:host-gateway",
			}
		}

		kaContainer, err = infrastructure.StartGenericContainer(kaContainerConfig, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "KA container must start")
		_, _ = fmt.Fprintf(GinkgoWriter, "KA ready at http://127.0.0.1:%d (container: %s)\n", afITKAPort, kaContainer.ID)

		_, _ = fmt.Fprintln(GinkgoWriter, "Phase 1 complete - all containers running")

		payload := fmt.Sprintf("%s\n%s\n%s\n%s\n%s",
			authConfig.Token,
			k8sCfg.Host,
			base64.StdEncoding.EncodeToString(k8sCfg.CertData),
			base64.StdEncoding.EncodeToString(k8sCfg.KeyData),
			base64.StdEncoding.EncodeToString(k8sCfg.CAData),
		)
		return []byte(payload)
	},

	func(data []byte) {
		By("Phase 2: Wiring AF in-process with real clients")
		parts := strings.SplitN(string(data), "\n", 5)
		Expect(len(parts)).To(BeNumerically(">=", 5), "payload must contain token, host, certData, keyData, caData")
		dsToken := parts[0]
		itKAToken = dsToken
		envtestHost := parts[1]

		certData, err := base64.StdEncoding.DecodeString(parts[2])
		Expect(err).ToNot(HaveOccurred(), "decode certData")
		keyData, err := base64.StdEncoding.DecodeString(parts[3])
		Expect(err).ToNot(HaveOccurred(), "decode keyData")
		caData, err := base64.StdEncoding.DecodeString(parts[4])
		Expect(err).ToNot(HaveOccurred(), "decode caData")

		envtestCfg := &rest.Config{
			Host: envtestHost,
			TLSClientConfig: rest.TLSClientConfig{
				CertData: certData,
				KeyData:  keyData,
				CAData:   caData,
			},
		}
		itEnvtestCfg = envtestCfg
		itDynClient, err = dynamic.NewForConfig(envtestCfg)
		Expect(err).ToNot(HaveOccurred(), "real dynamic client from envtest host")
		_, _ = fmt.Fprintf(GinkgoWriter, "Phase 2: real dynamic client connected to envtest at %s\n", envtestHost)

		kaURL := fmt.Sprintf("http://127.0.0.1:%d", afITKAPort)
		dsURL := fmt.Sprintf("http://127.0.0.1:%d", afITDataStoragePort)

		kaClient := ka.NewClient(ka.Config{
			BaseURL:            kaURL,
			Token:              dsToken,
			Timeout:            10 * time.Second,
			CBFailureThreshold: 5,
			CBMaxRequests:      3,
			CBInterval:         30 * time.Second,
			CBTimeout:          10 * time.Second,
			RetryMax:           2,
			RetryInitBackoff:   100 * time.Millisecond,
			RetryMaxBackoff:    1 * time.Second,
			RetryableStatuses:  []int{503},
		})

		// DS OgenClient with Bearer token auth transport
		dsTransport := &bearerTransport{
			base:  http.DefaultTransport,
			token: dsToken,
		}
		dsClient, err := ds.NewOgenClient(ds.OgenClientConfig{
			BaseURL:   dsURL,
			Transport: dsTransport,
			Timeout:   10 * time.Second,
		})
		Expect(err).ToNot(HaveOccurred(), "DS OgenClient must initialize")

		auditCapture = &testAuditor{}
		testUser = &auth.UserIdentity{Username: "sre@kubernaut.ai", Groups: []string{"sre"}}
		itMetrics = newITBridgeMetrics()

		cfg := handler.MCPConfig{
			ServerName:    "af-it-containers",
			ServerVersion: "0.0.1-it",
			Enabled:       true,
			Bridge: &handler.MCPBridgeConfig{
				DynFactory: auth.StaticDynamicFactory(itDynClient),
				KAClient:   kaClient,
				KAMCPClient: &ka.MockMCPClient{
					SelectWorkflowFn: func(_ context.Context, _ ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
						return &ka.SelectWorkflowResult{Status: "selected", Message: "workflow selected"}, nil
					},
				},
				DSClient:           dsClient,
				RBACRoles:          map[string][]string{"sre": {"*"}},
				Logger:             logr.Discard(),
				Auditor:            auditCapture,
				Metrics:            itMetrics,
				ToolTimeout:        15 * time.Second,
				MaxConcurrentTools: 10,
			},
		}

		mcpHandler, err = handler.NewMCPHandler(cfg)
		Expect(err).ToNot(HaveOccurred(), "MCP handler must initialize")

		// Seed K8s fixtures for CRD and triage tool specs (IT-ENV-003)
		seedK8sFixtures(context.Background(), itDynClient)

		_, _ = fmt.Fprintln(GinkgoWriter, "Phase 2 complete - AF MCP handler wired with real KA + DS + envtest K8s")
	},
)

var _ = SynchronizedAfterSuite(
	func() {},
	func() {
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		_, _ = fmt.Fprintln(GinkgoWriter, "ApiFrontend IT - Infrastructure Cleanup")
		_, _ = fmt.Fprintln(GinkgoWriter, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		gatherCtx, gatherCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer gatherCancel()
		mustGatherDone := make(chan struct{})
		go func() {
			defer close(mustGatherDone)
			if kaContainer != nil {
				infrastructure.MustGatherContainerLogs("apifrontend", []string{kaContainer.Name}, GinkgoWriter)
			}
			infrastructure.MustGatherContainerLogs("apifrontend", []string{"apifrontend_mockllm_test"}, GinkgoWriter)
			if dsInfra != nil {
				infrastructure.MustGatherContainerLogs("apifrontend", []string{
					dsInfra.DataStorageContainer,
					dsInfra.PostgresContainer,
					dsInfra.RedisContainer,
				}, GinkgoWriter)
			}
		}()
		select {
		case <-mustGatherDone:
			_, _ = fmt.Fprintln(GinkgoWriter, "✅ Must-gather collection complete")
		case <-gatherCtx.Done():
			_, _ = fmt.Fprintln(GinkgoWriter, "⚠️  Must-gather timed out after 60s, proceeding with cleanup")
		}

		if kaContainer != nil {
			_ = infrastructure.StopGenericContainer(kaContainer, GinkgoWriter)
		}
		stopContainer("apifrontend_mockllm_test")

		if dsInfra != nil {
			_ = infrastructure.StopDSBootstrap(dsInfra, GinkgoWriter)
		}

		_, _ = fmt.Fprintln(GinkgoWriter, "Suite complete")
	},
)

// bearerTransport injects a Bearer token into every outbound HTTP request.
type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(r)
}

func stopContainer(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "podman", "rm", "-f", name)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	_ = cmd.Run()
}

// resolveConfigDir returns the ConfigDir value for DSBootstrapConfig.
// Because StartDSBootstrap joins ConfigDir with the kubernaut module root
// (via an unexported getProjectRoot()), we express ConfigDir as a relative
// path from that root to our actual config directory.
func resolveConfigDir() string {
	kubernautRoot := findKubernautModuleRoot()
	_, thisFile, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	ourConfigDir := filepath.Join(filepath.Dir(thisFile), "config")

	relPath, err := filepath.Rel(kubernautRoot, ourConfigDir)
	Expect(err).ToNot(HaveOccurred(), "should compute relative path from kubernaut root to our config")
	_, _ = fmt.Fprintf(GinkgoWriter, "ConfigDir resolved: %s (relative from %s)\n", relPath, kubernautRoot)
	return relPath
}

func findKubernautModuleRoot() string {
	// Use `go list -m -json` to get the exact module version from go.mod,
	// avoiding ambiguity when multiple versions exist in the module cache.
	out, err := exec.Command("go", "list", "-m", "-json", "github.com/jordigilh/kubernaut").Output()
	if err == nil {
		var modInfo struct {
			Dir string `json:"Dir"`
		}
		if json.Unmarshal(out, &modInfo) == nil && modInfo.Dir != "" {
			return modInfo.Dir
		}
	}

	// Fallback: scan module cache for the latest version
	out, err = exec.Command("go", "env", "GOMODCACHE").Output()
	Expect(err).ToNot(HaveOccurred(), "go env GOMODCACHE should succeed")
	modCache := strings.TrimSpace(string(out))

	parentDir := filepath.Join(modCache, "github.com", "jordigilh")
	entries, err := os.ReadDir(parentDir)
	Expect(err).ToNot(HaveOccurred())

	var best string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "kubernaut@") && !strings.Contains(e.Name(), "apifrontend") {
			candidate := filepath.Join(parentDir, e.Name())
			if best == "" || e.Name() > filepath.Base(best) {
				best = candidate
			}
		}
	}
	Expect(best).ToNot(BeEmpty(), "kubernaut module not found in module cache at "+parentDir)
	return best
}

func newITBridgeMetrics() *handler.MCPBridgeMetrics {
	return &handler.MCPBridgeMetrics{
		ToolCallsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "it_tool_calls_total",
		}, []string{"tool", "result"}),
		ToolCallDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "it_tool_call_duration_seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"tool", "type"}),
		RBACDeniedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "it_rbac_denied_total",
		}, []string{"tool"}),
	}
}
