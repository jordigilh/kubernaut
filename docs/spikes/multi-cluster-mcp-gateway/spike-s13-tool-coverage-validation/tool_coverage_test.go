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

// Package spike_s13 validates the actual tool surface of kubernetes-mcp-server
// against KA's investigation tool needs (S1 paper analysis).
// This code is NOT production code — it lives under docs/spikes/ per project convention.
package spike_s13

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestSpikeS13(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spike S13 — K8s MCP Server Tool Coverage Validation")
}

var (
	testEnv    *envtest.Environment
	restCfg    *rest.Config
	kubeconfig string
)

var _ = BeforeSuite(func() {
	By("Starting envtest")
	testEnv = &envtest.Environment{}
	var err error
	restCfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(restCfg).ToNot(BeNil())

	By("Writing kubeconfig for envtest")
	kubeconfig = writeKubeconfig(restCfg)
})

var _ = AfterSuite(func() {
	By("Stopping envtest")
	if testEnv != nil {
		_ = testEnv.Stop()
	}
	By("Cleaning up kubeconfig")
	if kubeconfig != "" {
		_ = os.Remove(kubeconfig)
	}
})

// startMCPServer starts kubernetes-mcp-server with the given toolsets and returns
// the endpoint URL, the running cmd, and a cleanup function.
func startMCPServer(toolsets string) (endpoint string, cmd *exec.Cmd, cleanup func()) {
	mcpBinary, err := exec.LookPath("kubernetes-mcp-server")
	Expect(err).ToNot(HaveOccurred(), "kubernetes-mcp-server binary must be in PATH")

	port := findFreePort()

	args := []string{
		"--kubeconfig", kubeconfig,
		"--port", port,
		"--read-only",
		"--stateless",
		"--disable-multi-cluster",
	}
	if toolsets != "" {
		args = append(args, "--toolsets", toolsets)
	}

	cmd = exec.Command(mcpBinary, args...)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	err = cmd.Start()
	Expect(err).ToNot(HaveOccurred())

	Eventually(func() error {
		conn, dialErr := net.DialTimeout("tcp", "127.0.0.1:"+port, 500*time.Millisecond)
		if dialErr != nil {
			return dialErr
		}
		conn.Close()
		return nil
	}, 10*time.Second, 200*time.Millisecond).Should(Succeed())

	endpoint = fmt.Sprintf("http://127.0.0.1:%s", port)
	cleanup = func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}
	return endpoint, cmd, cleanup
}

// toolInventory captures a tool's full metadata for analysis.
type toolInventory struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Properties  []string // top-level property names from input schema
}

func enumerateTools(ctx context.Context, endpoint string) []toolInventory {
	session := mustConnect(ctx, endpoint)
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	Expect(err).ToNot(HaveOccurred())

	inventory := make([]toolInventory, 0, len(result.Tools))
	for _, t := range result.Tools {
		ti := toolInventory{
			Name:        t.Name,
			Description: t.Description,
		}

		if t.InputSchema != nil {
			schemaBytes, marshalErr := json.Marshal(t.InputSchema)
			if marshalErr == nil {
				ti.InputSchema = schemaBytes
				ti.Properties = extractPropertyNames(schemaBytes)
			}
		}

		inventory = append(inventory, ti)
	}

	sort.Slice(inventory, func(i, j int) bool {
		return inventory[i].Name < inventory[j].Name
	})
	return inventory
}

func extractPropertyNames(schema json.RawMessage) []string {
	var s struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return nil
	}
	names := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// kaInvestigationTools are the tools KA needs for remote cluster investigation.
// From S1's coverage matrix.
type kaToolRequirement struct {
	KAToolName         string
	Description        string
	ExpectedMCPTool    string   // tool name we expect to find
	CriticalParams     []string // params that must exist in input schema
	InvestigationValue string   // HIGH/MEDIUM/LOW
}

var kaInvestigationTools = []kaToolRequirement{
	// K8s Resource Query Tools
	{KAToolName: "kubectl_describe", ExpectedMCPTool: "resources_get", CriticalParams: []string{"kind", "name", "namespace"}, InvestigationValue: "HIGH"},
	{KAToolName: "kubectl_get_by_name", ExpectedMCPTool: "resources_get", CriticalParams: []string{"kind", "name", "namespace"}, InvestigationValue: "HIGH"},
	{KAToolName: "kubectl_get_by_kind_in_namespace", ExpectedMCPTool: "resources_list", CriticalParams: []string{"kind", "namespace"}, InvestigationValue: "HIGH"},
	{KAToolName: "kubectl_get_by_kind_in_cluster", ExpectedMCPTool: "resources_list", CriticalParams: []string{"kind"}, InvestigationValue: "HIGH"},
	{KAToolName: "kubectl_find_resource", ExpectedMCPTool: "resources_list", CriticalParams: []string{"labelSelector"}, InvestigationValue: "MEDIUM"},
	{KAToolName: "kubectl_events", ExpectedMCPTool: "events_list", CriticalParams: []string{}, InvestigationValue: "HIGH"},

	// Log Tools
	{KAToolName: "kubectl_logs", ExpectedMCPTool: "pods_log", CriticalParams: []string{"name", "namespace"}, InvestigationValue: "HIGH"},
	{KAToolName: "kubectl_previous_logs", ExpectedMCPTool: "pods_log", CriticalParams: []string{"previous"}, InvestigationValue: "HIGH"},
	{KAToolName: "kubectl_container_logs", ExpectedMCPTool: "pods_log", CriticalParams: []string{"container"}, InvestigationValue: "HIGH"},
	{KAToolName: "kubectl_logs_grep", ExpectedMCPTool: "pods_log", CriticalParams: []string{}, InvestigationValue: "HIGH", Description: "grep not expected — verify no grep param exists"},

	// Metrics Tools
	{KAToolName: "kubectl_top_pods", ExpectedMCPTool: "pods_top", CriticalParams: []string{}, InvestigationValue: "MEDIUM"},
	{KAToolName: "kubectl_top_nodes", ExpectedMCPTool: "nodes_top", CriticalParams: []string{}, InvestigationValue: "MEDIUM"},

	// Bonus Tools (not in KA but available in K8s MCP Server)
	{KAToolName: "(bonus) namespaces_list", ExpectedMCPTool: "namespaces_list", CriticalParams: []string{}, InvestigationValue: "MEDIUM"},

	// WE Remote Execution Tools
	{KAToolName: "we_create_resource", ExpectedMCPTool: "resources_create_or_update", CriticalParams: []string{}, InvestigationValue: "HIGH"},
	{KAToolName: "we_delete_resource", ExpectedMCPTool: "resources_delete", CriticalParams: []string{"kind", "name", "namespace"}, InvestigationValue: "HIGH"},
}

var _ = Describe("Spike S13 — K8s MCP Server Tool Coverage Validation", Ordered, func() {
	var ctx context.Context
	var cancel context.CancelFunc

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
	})
	AfterEach(func() {
		cancel()
	})

	// --- S13-001: Default toolsets (core, config) ---

	It("S13-001: enumerates all tools from default toolsets (core, config)", func() {
		endpoint, _, cleanup := startMCPServer("")
		defer cleanup()

		inventory := enumerateTools(ctx, endpoint)

		GinkgoWriter.Printf("\n=== S13-001: Default toolsets (core, config) — %d tools ===\n\n", len(inventory))
		GinkgoWriter.Printf("%-35s | %-6s | %s\n", "TOOL NAME", "PARAMS", "DESCRIPTION")
		GinkgoWriter.Printf("%s\n", strings.Repeat("-", 120))
		for _, t := range inventory {
			desc := t.Description
			if len(desc) > 70 {
				desc = desc[:70] + "..."
			}
			GinkgoWriter.Printf("%-35s | %-6s | %s\n",
				t.Name, fmt.Sprintf("[%d]", len(t.Properties)), desc)
		}

		GinkgoWriter.Printf("\n--- Detailed schemas ---\n\n")
		for _, t := range inventory {
			GinkgoWriter.Printf("Tool: %s\n", t.Name)
			GinkgoWriter.Printf("  Description: %s\n", t.Description)
			GinkgoWriter.Printf("  Parameters:  %v\n", t.Properties)
			if len(t.InputSchema) > 0 {
				var pretty json.RawMessage
				if json.Unmarshal(t.InputSchema, &pretty) == nil {
					formatted, _ := json.MarshalIndent(pretty, "  ", "  ")
					GinkgoWriter.Printf("  Schema:\n  %s\n", string(formatted))
				}
			}
			GinkgoWriter.Println()
		}

		Expect(inventory).ToNot(BeEmpty(), "server must expose at least one tool")
	})

	// --- S13-002: core + tekton toolsets ---

	It("S13-002: enumerates all tools from core + tekton toolsets", func() {
		endpoint, _, cleanup := startMCPServer("core,tekton")
		defer cleanup()

		inventory := enumerateTools(ctx, endpoint)

		GinkgoWriter.Printf("\n=== S13-002: core + tekton toolsets — %d tools ===\n\n", len(inventory))
		GinkgoWriter.Printf("%-35s | %-6s | %s\n", "TOOL NAME", "PARAMS", "DESCRIPTION")
		GinkgoWriter.Printf("%s\n", strings.Repeat("-", 120))
		for _, t := range inventory {
			desc := t.Description
			if len(desc) > 70 {
				desc = desc[:70] + "..."
			}
			GinkgoWriter.Printf("%-35s | %-6s | %s\n",
				t.Name, fmt.Sprintf("[%d]", len(t.Properties)), desc)
		}

		// Check for tekton-specific tools
		toolNames := make(map[string]bool)
		for _, t := range inventory {
			toolNames[t.Name] = true
		}
		hasTekton := false
		for name := range toolNames {
			if strings.HasPrefix(name, "tekton") {
				hasTekton = true
				break
			}
		}
		GinkgoWriter.Printf("\nTekton tools present: %v\n", hasTekton)

		Expect(inventory).ToNot(BeEmpty())
	})

	// --- S13-003: all available toolsets ---

	It("S13-003: enumerates all tools from all available toolsets", func() {
		endpoint, _, cleanup := startMCPServer("core,config,helm,tekton")
		defer cleanup()

		inventory := enumerateTools(ctx, endpoint)

		GinkgoWriter.Printf("\n=== S13-003: all toolsets (core,config,helm,tekton) — %d tools ===\n\n", len(inventory))
		GinkgoWriter.Printf("%-35s | %-6s | %s\n", "TOOL NAME", "PARAMS", "DESCRIPTION")
		GinkgoWriter.Printf("%s\n", strings.Repeat("-", 120))
		for _, t := range inventory {
			desc := t.Description
			if len(desc) > 70 {
				desc = desc[:70] + "..."
			}
			GinkgoWriter.Printf("%-35s | %-6s | %s\n",
				t.Name, fmt.Sprintf("[%d]", len(t.Properties)), desc)
		}

		Expect(inventory).ToNot(BeEmpty())
	})

	// --- S13-004: Map actual tools to KA investigation needs ---

	It("S13-004: maps actual tools to KA investigation requirements", func() {
		endpoint, _, cleanup := startMCPServer("core,config,tekton")
		defer cleanup()

		inventory := enumerateTools(ctx, endpoint)

		toolMap := make(map[string]toolInventory)
		for _, t := range inventory {
			toolMap[t.Name] = t
		}

		GinkgoWriter.Printf("\n=== S13-004: KA Tool Coverage Matrix (Empirical) ===\n\n")
		GinkgoWriter.Printf("%-40s | %-30s | %-8s | %s\n",
			"KA TOOL", "EXPECTED MCP TOOL", "STATUS", "NOTES")
		GinkgoWriter.Printf("%s\n", strings.Repeat("-", 130))

		var fullCount, partialCount, gapCount int

		for _, req := range kaInvestigationTools {
			actual, exists := toolMap[req.ExpectedMCPTool]

			status := "GAP"
			notes := "tool not found in tools/list"

			if exists {
				missingParams := findMissingParams(actual.Properties, req.CriticalParams)
				if len(missingParams) == 0 {
					status = "FULL"
					notes = fmt.Sprintf("params: %v", actual.Properties)
				} else {
					status = "PARTIAL"
					notes = fmt.Sprintf("missing params: %v (has: %v)", missingParams, actual.Properties)
				}
			}

			switch status {
			case "FULL":
				fullCount++
			case "PARTIAL":
				partialCount++
			case "GAP":
				gapCount++
			}

			if req.Description != "" {
				notes += " | " + req.Description
			}

			GinkgoWriter.Printf("%-40s | %-30s | %-8s | %s\n",
				req.KAToolName, req.ExpectedMCPTool, status, notes)
		}

		total := fullCount + partialCount + gapCount
		coveragePct := 0
		if total > 0 {
			coveragePct = (fullCount + partialCount) * 100 / total
		}

		GinkgoWriter.Printf("\n--- Coverage Summary ---\n")
		GinkgoWriter.Printf("  FULL:    %d/%d (%d%%)\n", fullCount, total, fullCount*100/total)
		GinkgoWriter.Printf("  PARTIAL: %d/%d (%d%%)\n", partialCount, total, partialCount*100/total)
		GinkgoWriter.Printf("  GAP:     %d/%d (%d%%)\n", gapCount, total, gapCount*100/total)
		GinkgoWriter.Printf("  FULL+PARTIAL: %d/%d (%d%%)\n", fullCount+partialCount, total, coveragePct)
	})

	// --- S13-005: Validate critical parameter support ---

	It("S13-005: validates critical parameters on key tools", func() {
		endpoint, _, cleanup := startMCPServer("core,tekton")
		defer cleanup()

		inventory := enumerateTools(ctx, endpoint)

		toolMap := make(map[string]toolInventory)
		for _, t := range inventory {
			toolMap[t.Name] = t
		}

		type paramCheck struct {
			tool   string
			param  string
			reason string
		}

		criticalChecks := []paramCheck{
			{"resources_list", "labelSelector", "FMC scope sync requires filtering by kubernaut.ai/managed=true"},
			{"resources_list", "namespace", "scoped listing requires namespace param"},
			{"resources_list", "kind", "must specify resource kind"},
			{"resources_get", "name", "must identify resource by name"},
			{"resources_get", "namespace", "must scope to namespace"},
			{"resources_get", "kind", "must specify resource kind"},
			{"pods_log", "name", "must identify pod by name"},
			{"pods_log", "namespace", "must scope to namespace"},
			{"pods_log", "previous", "KA needs previous container logs for crash investigation"},
			{"pods_log", "container", "KA needs per-container log access"},
			{"pods_log", "tail", "tail limits output volume — critical for LLM context limits"},
		}

		GinkgoWriter.Printf("\n=== S13-005: Critical Parameter Validation ===\n\n")
		GinkgoWriter.Printf("%-25s | %-15s | %-6s | %s\n",
			"TOOL", "PARAMETER", "EXISTS", "REASON")
		GinkgoWriter.Printf("%s\n", strings.Repeat("-", 100))

		allPass := true
		for _, check := range criticalChecks {
			tool, exists := toolMap[check.tool]
			paramExists := false
			if exists {
				for _, p := range tool.Properties {
					if p == check.param {
						paramExists = true
						break
					}
				}
			}

			status := "YES"
			if !exists {
				status = "NO TOOL"
				allPass = false
			} else if !paramExists {
				status = "NO"
				allPass = false
			}

			GinkgoWriter.Printf("%-25s | %-15s | %-6s | %s\n",
				check.tool, check.param, status, check.reason)
		}

		GinkgoWriter.Printf("\nAll critical parameters present: %v\n", allPass)
	})
})

func findMissingParams(actual []string, required []string) []string {
	set := make(map[string]bool)
	for _, p := range actual {
		set[p] = true
	}
	var missing []string
	for _, r := range required {
		if !set[r] {
			missing = append(missing, r)
		}
	}
	return missing
}

// --- Helpers (same as S8) ---

func mustConnect(ctx context.Context, url string) *mcp.ClientSession {
	client := mcp.NewClient(&mcp.Implementation{Name: "spike-s13-test", Version: "v1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: url + "/mcp"}
	session, err := client.Connect(ctx, transport, nil)
	Expect(err).ToNot(HaveOccurred(), "failed to connect to K8s MCP Server at %s", url)
	return session
}

func writeKubeconfig(cfg *rest.Config) string {
	dir := os.TempDir()
	path := filepath.Join(dir, fmt.Sprintf("spike-s13-kubeconfig-%d", time.Now().UnixNano()))

	kubeConfig := clientcmdapi.NewConfig()
	kubeConfig.Clusters["envtest"] = &clientcmdapi.Cluster{
		Server:                   cfg.Host,
		CertificateAuthorityData: cfg.CAData,
	}
	kubeConfig.AuthInfos["envtest"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: cfg.CertData,
		ClientKeyData:         cfg.KeyData,
	}
	kubeConfig.Contexts["envtest"] = &clientcmdapi.Context{
		Cluster:  "envtest",
		AuthInfo: "envtest",
	}
	kubeConfig.CurrentContext = "envtest"

	err := clientcmd.WriteToFile(*kubeConfig, path)
	Expect(err).ToNot(HaveOccurred())
	return path
}

func findFreePort() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).ToNot(HaveOccurred())
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return fmt.Sprintf("%d", port)
}

// addScheme registers core types — only needed if we create test resources.
func addScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	return scheme
}
