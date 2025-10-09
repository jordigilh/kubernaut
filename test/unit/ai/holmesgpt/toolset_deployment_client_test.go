/*
Copyright 2025 Jordi Gil.

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

package holmesgpt

import (
	"testing"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
)

// Business Requirement Tests for HolmesGPT Toolset Deployment Client
// BR-HOLMES-001: MUST provide HolmesGPT with custom Kubernaut toolset for context orchestration
// BR-HOLMES-002: MUST enable HolmesGPT to invoke specific context retrieval functions during investigations
// BR-EXTERNAL-001: MUST integrate with HolmesGPT v0.13.1+ custom toolset framework
var _ = Describe("HolmesGPT Toolset Deployment Client", func() {
	var (
		client     holmesgpt.ToolsetDeploymentClient
		testServer *httptest.Server
		logger     *logrus.Logger
		ctx        context.Context
	)

	BeforeEach(func() {
		logger = logrus.New()
		logger.SetLevel(logrus.WarnLevel)
		ctx = context.Background()
	})

	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
	})

	// BR-HOLMES-001: Custom Kubernaut toolset for context orchestration
	Context("Custom Toolset Deployment", func() {
		It("should deploy Kubernaut custom toolset to HolmesGPT", func() {
			// Mock HolmesGPT toolset deployment endpoint
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Validate the request
				Expect(r.Method).To(Equal("POST"), "BR-HOLMES-001: Must use POST for toolset deployment")
				Expect(r.URL.Path).To(Equal("/api/v1/toolsets"), "BR-HOLMES-001: Must use standard toolset endpoint")
				Expect(r.Header.Get("Content-Type")).To(ContainSubstring("application/json"), "BR-HOLMES-001: Must use JSON for toolset configuration")

				// Parse and validate toolset configuration
				var toolset holmesgpt.ToolsetConfig
				decoder := json.NewDecoder(r.Body)
				err := decoder.Decode(&toolset)
				Expect(err).ToNot(HaveOccurred(), "BR-HOLMES-001: Toolset configuration must be valid JSON")

				// BR-HOLMES-001: Validate custom Kubernaut toolset structure
				Expect(toolset.Name).To(ContainSubstring("kubernaut"), "BR-HOLMES-001: Must provide Kubernaut-specific toolset")
				Expect(toolset.ServiceType).To(Equal("kubernetes"), "BR-HOLMES-001: Must target Kubernetes context orchestration")
				Expect(toolset.Capabilities).To(ContainElement("context_orchestration"), "BR-HOLMES-001: Must provide context orchestration capability")
				Expect(toolset.Tools).ToNot(BeEmpty(), "BR-HOLMES-001: Must include context retrieval tools")

				// BR-HOLMES-002: Validate context retrieval functions
				contextTools := make([]holmesgpt.HolmesGPTTool, 0)
				for _, tool := range toolset.Tools {
					if tool.Category == "context_retrieval" {
						contextTools = append(contextTools, tool)
					}
				}
				Expect(contextTools).ToNot(BeEmpty(), "BR-HOLMES-002: Must enable specific context retrieval functions")

				// Validate at least one tool for each context type
				toolNames := make([]string, 0)
				for _, tool := range contextTools {
					toolNames = append(toolNames, tool.Name)
				}
				Expect(toolNames).To(ContainElement(ContainSubstring("kubernetes")), "BR-HOLMES-002: Must provide Kubernetes context retrieval")
				Expect(toolNames).To(ContainElement(ContainSubstring("metrics")), "BR-HOLMES-002: Must provide metrics context retrieval")
				Expect(toolNames).To(ContainElement(ContainSubstring("action_history")), "BR-HOLMES-002: Must provide action history retrieval")

				// Return successful deployment response
				response := holmesgpt.ToolsetDeploymentResponse{
					Success:     true,
					ToolsetName: toolset.Name,
					DeployedAt:  time.Now(),
					Message:     "Custom Kubernaut toolset deployed successfully",
					ToolsCount:  len(toolset.Tools),
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(response)
			}))

			// Create client pointing to test server
			client = holmesgpt.NewToolsetDeploymentClient(testServer.URL, logger)

			// Create sample Kubernaut toolset
			kubernautToolset := &holmesgpt.ToolsetConfig{
				Name:        "kubernaut-context-orchestration",
				ServiceType: "kubernetes",
				Description: "Custom Kubernaut toolset for dynamic context orchestration",
				Version:     "1.0.0",
				Capabilities: []string{
					"context_orchestration",
					"dynamic_context_retrieval",
					"investigation_optimization",
				},
				Tools: []holmesgpt.HolmesGPTTool{
					{
						Name:        "get_kubernetes_context",
						Description: "Retrieve Kubernetes cluster context for investigation",
						Command:     "kubernaut-context kubernetes --namespace {namespace} --resource {resource}",
						Category:    "context_retrieval",
						Parameters: []holmesgpt.ToolParameter{
							{
								Name:        "namespace",
								Description: "Kubernetes namespace to investigate",
								Required:    true,
								Type:        "string",
							},
							{
								Name:        "resource",
								Description: "Specific resource type or name",
								Required:    false,
								Type:        "string",
							},
						},
					},
					{
						Name:        "get_metrics_context",
						Description: "Retrieve performance metrics context for investigation",
						Command:     "kubernaut-context metrics --namespace {namespace} --timespan {timespan}",
						Category:    "context_retrieval",
						Parameters: []holmesgpt.ToolParameter{
							{
								Name:        "namespace",
								Description: "Kubernetes namespace for metrics",
								Required:    true,
								Type:        "string",
							},
							{
								Name:        "timespan",
								Description: "Time window for metrics collection",
								Required:    false,
								Type:        "string",
								Default:     "1h",
							},
						},
					},
					{
						Name:        "get_action_history_context",
						Description: "Retrieve historical action patterns for investigation",
						Command:     "kubernaut-context action-history --alert-type {alert_type}",
						Category:    "context_retrieval",
						Parameters: []holmesgpt.ToolParameter{
							{
								Name:        "alert_type",
								Description: "Type of alert to match historical patterns",
								Required:    true,
								Type:        "string",
							},
						},
					},
				},
				Enabled:  true,
				Priority: 100, // High priority for Kubernaut integration
			}

			// Test deployment
			response, err := client.DeployToolset(ctx, kubernautToolset)

			// BR-HOLMES-001: Validate successful deployment
			Expect(err).ToNot(HaveOccurred(), "BR-HOLMES-001: Custom toolset deployment must succeed")
			Expect(response.Success).To(BeTrue(), "BR-HOLMES-001: Deployment must be successful")
			Expect(response.ToolsetName).To(Equal(kubernautToolset.Name), "BR-HOLMES-001: Must confirm correct toolset name")
			Expect(response.ToolsCount).To(Equal(len(kubernautToolset.Tools)), "BR-HOLMES-001: Must deploy all toolset functions")
			Expect(response.Message).To(ContainSubstring("successfully"), "BR-HOLMES-001: Must confirm successful deployment")

			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-HOLMES-001",
				"toolset_name":         response.ToolsetName,
				"tools_deployed":       response.ToolsCount,
				"deployment_time":      response.DeployedAt,
			}).Info("BR-HOLMES-001: Custom Kubernaut toolset deployment validation completed")
		})

		It("should handle toolset deployment failures gracefully", func() {
			// Mock failed deployment scenario
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := holmesgpt.ToolsetDeploymentResponse{
					Success: false,
					Message: "Toolset validation failed: invalid tool configuration",
					Error:   "TOOLSET_VALIDATION_ERROR",
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(response)
			}))

			client = holmesgpt.NewToolsetDeploymentClient(testServer.URL, logger)

			invalidToolset := &holmesgpt.ToolsetConfig{
				Name: "invalid-toolset",
				// Missing required fields to trigger validation error
			}

			response, err := client.DeployToolset(ctx, invalidToolset)

			// BR-HOLMES-001: Must handle deployment failures
			Expect(err).To(HaveOccurred(), "BR-HOLMES-001: Must detect deployment failures")
			Expect(response.Success).To(BeFalse(), "BR-AI-002-RECOMMENDATION-CONFIDENCE: Toolset deployment response must indicate failure status for validation")
			Expect(response.Success).To(BeFalse(), "BR-HOLMES-001: Must indicate deployment failure")
			Expect(response.Message).To(ContainSubstring("validation failed"), "BR-HOLMES-001: Must provide meaningful error messages")
		})
	})

	// BR-EXTERNAL-001: Integration with HolmesGPT v0.13.1+ custom toolset framework
	Context("HolmesGPT v0.13.1+ Integration", func() {
		It("should support HolmesGPT v0.13.1+ toolset framework features", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Validate v0.13.1+ specific features
				var toolset holmesgpt.ToolsetConfig
				_ = json.NewDecoder(r.Body).Decode(&toolset)

				// BR-EXTERNAL-001: Validate v0.13.1+ framework compatibility
				Expect(toolset.Version).ToNot(BeEmpty(), "BR-EXTERNAL-001: Must include toolset version for v0.13.1+ compatibility")

				// Check for v0.13.1+ specific tool features
				for _, tool := range toolset.Tools {
					// BR-EXTERNAL-001: Must support parameter validation
					if len(tool.Parameters) > 0 {
						for _, param := range tool.Parameters {
							Expect(param.Name).ToNot(BeEmpty(), "BR-EXTERNAL-001: Must provide parameter names")
							Expect(param.Type).ToNot(BeEmpty(), "BR-EXTERNAL-001: Must specify parameter types")
						}
					}

					// BR-EXTERNAL-001: Must categorize tools for v0.13.1+ framework
					Expect(tool.Category).ToNot(BeEmpty(), "BR-EXTERNAL-001: Must provide tool categories")
				}

				response := holmesgpt.ToolsetDeploymentResponse{
					Success:           true,
					ToolsetName:       toolset.Name,
					FrameworkVersion:  "0.13.1",
					CompatibilityMode: "native",
					DeployedAt:        time.Now(),
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(response)
			}))

			client = holmesgpt.NewToolsetDeploymentClient(testServer.URL, logger)

			// Create v0.13.1+ compatible toolset
			v13Toolset := &holmesgpt.ToolsetConfig{
				Name:         "kubernaut-v13-toolset",
				ServiceType:  "kubernetes",
				Version:      "1.0.0",
				Capabilities: []string{"context_orchestration"},
				Tools: []holmesgpt.HolmesGPTTool{
					{
						Name:        "discover_context_types",
						Description: "Discover available context types for investigation",
						Category:    "discovery",
						Parameters: []holmesgpt.ToolParameter{
							{
								Name:        "alert_type",
								Description: "Type of alert being investigated",
								Type:        "string",
								Required:    false,
							},
							{
								Name:        "namespace",
								Description: "Kubernetes namespace context",
								Type:        "string",
								Required:    false,
							},
						},
					},
				},
			}

			response, err := client.DeployToolset(ctx, v13Toolset)

			// BR-EXTERNAL-001: Validate v0.13.1+ integration
			Expect(err).ToNot(HaveOccurred(), "BR-EXTERNAL-001: Must integrate with v0.13.1+ framework")
			Expect(response.FrameworkVersion).To(Equal("0.13.1"), "BR-EXTERNAL-001: Must confirm framework version compatibility")
			Expect(response.CompatibilityMode).To(Equal("native"), "BR-EXTERNAL-001: Must support native integration mode")
		})
	})

	// BR-HOLMES-003: Toolset function discovery and capability enumeration
	Context("Toolset Discovery and Capabilities", func() {
		It("should enable HolmesGPT to discover available toolset functions", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" && r.URL.Path == "/api/v1/toolsets" {
					// Return available toolsets for discovery
					toolsets := []holmesgpt.ToolsetInfo{
						{
							Name:         "kubernaut-context-orchestration",
							ServiceType:  "kubernetes",
							Capabilities: []string{"context_orchestration", "dynamic_retrieval"},
							ToolCount:    3,
							Status:       "active",
						},
					}

					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"toolsets": toolsets,
						"total":    len(toolsets),
					})
				} else if r.Method == "GET" && r.URL.Path == "/api/v1/toolsets/kubernaut-context-orchestration" {
					// Return detailed toolset capabilities
					toolset := holmesgpt.DetailedToolsetInfo{
						Name:         "kubernaut-context-orchestration",
						ServiceType:  "kubernetes",
						Capabilities: []string{"context_orchestration", "dynamic_retrieval"},
						Tools: []holmesgpt.HolmesGPTTool{
							{
								Name:        "get_kubernetes_context",
								Description: "Retrieve Kubernetes cluster context",
								Category:    "context_retrieval",
								Parameters:  []holmesgpt.ToolParameter{},
							},
							{
								Name:        "get_metrics_context",
								Description: "Retrieve performance metrics",
								Category:    "context_retrieval",
								Parameters:  []holmesgpt.ToolParameter{},
							},
						},
						Status: "active",
					}

					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(toolset); err != nil {
						http.Error(w, "Failed to encode response", http.StatusInternalServerError)
					}
				}
			}))

			client = holmesgpt.NewToolsetDeploymentClient(testServer.URL, logger)

			// Test toolset discovery
			toolsets, err := client.DiscoverToolsets(ctx)
			Expect(err).ToNot(HaveOccurred(), "BR-HOLMES-003: Toolset discovery must succeed")
			Expect(toolsets).ToNot(BeEmpty(), "BR-HOLMES-003: Must discover available toolsets")

			kubernautToolset := toolsets[0]
			Expect(kubernautToolset.Name).To(ContainSubstring("kubernaut"), "BR-HOLMES-003: Must discover Kubernaut toolset")
			Expect(kubernautToolset.Capabilities).To(ContainElement("context_orchestration"), "BR-HOLMES-003: Must enumerate capabilities")

			// Test detailed capability enumeration
			details, err := client.GetToolsetDetails(ctx, "kubernaut-context-orchestration")
			Expect(err).ToNot(HaveOccurred(), "BR-HOLMES-003: Capability enumeration must succeed")
			Expect(details.Tools).To(HaveLen(2), "BR-HOLMES-003: Must enumerate available functions")

			toolCategories := make([]string, 0)
			for _, tool := range details.Tools {
				toolCategories = append(toolCategories, tool.Category)
			}
			Expect(toolCategories).To(ContainElement("context_retrieval"), "BR-HOLMES-003: Must categorize toolset functions")

			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-HOLMES-003",
				"discovered_toolsets":  len(toolsets),
				"kubernaut_tools":      len(details.Tools),
			}).Info("BR-HOLMES-003: Toolset discovery and capability enumeration validation completed")
		})
	})

	// BR-HOLMES-004: Toolset function documentation and usage examples
	Context("Function Documentation and Examples", func() {
		It("should provide comprehensive toolset function documentation", func() {
			testServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/toolsets/kubernaut-context-orchestration/documentation" {
					docs := holmesgpt.ToolsetDocumentation{
						Name:        "kubernaut-context-orchestration",
						Description: "Comprehensive documentation for Kubernaut context orchestration toolset",
						Tools: []holmesgpt.ToolDocumentation{
							{
								Name:        "get_kubernetes_context",
								Description: "Retrieves Kubernetes cluster context for investigation",
								Usage:       "get_kubernetes_context --namespace production --resource deployment/web-app",
								Parameters: []holmesgpt.ParameterDocumentation{
									{
										Name:        "namespace",
										Description: "Kubernetes namespace to investigate",
										Type:        "string",
										Required:    true,
										Examples:    []string{"production", "staging", "default"},
									},
									{
										Name:        "resource",
										Description: "Specific Kubernetes resource",
										Type:        "string",
										Required:    false,
										Examples:    []string{"deployment/web-app", "pod/nginx-123", "service/api"},
									},
								},
								Examples: []holmesgpt.ToolExample{
									{
										Description: "Investigate production deployment",
										Command:     "get_kubernetes_context --namespace production --resource deployment/web-app",
										Expected:    "Returns deployment status, replica information, and recent events",
									},
									{
										Description: "General namespace investigation",
										Command:     "get_kubernetes_context --namespace staging",
										Expected:    "Returns namespace overview, resource counts, and health status",
									},
								},
							},
						},
						UsageGuidelines: "Use context orchestration tools to dynamically gather investigation-specific context data",
						BestPractices: []string{
							"Start with general context before drilling down to specifics",
							"Use namespace parameter to scope investigations appropriately",
							"Combine multiple context types for comprehensive analysis",
						},
					}

					w.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(w).Encode(docs); err != nil {
						http.Error(w, "Failed to encode response", http.StatusInternalServerError)
					}
				}
			}))

			client = holmesgpt.NewToolsetDeploymentClient(testServer.URL, logger)

			// Test documentation retrieval
			docs, err := client.GetToolsetDocumentation(ctx, "kubernaut-context-orchestration")

			// BR-HOLMES-004: Validate comprehensive documentation
			Expect(err).ToNot(HaveOccurred(), "BR-HOLMES-004: Documentation retrieval must succeed")
			Expect(docs.Name).To(ContainSubstring("kubernaut"), "BR-HOLMES-004: Must provide toolset documentation")
			Expect(docs.Tools).ToNot(BeEmpty(), "BR-HOLMES-004: Must document toolset functions")

			// Validate function documentation completeness
			tool := docs.Tools[0]
			Expect(tool.Name).ToNot(BeEmpty(), "BR-HOLMES-004: Must provide function names")
			Expect(tool.Description).ToNot(BeEmpty(), "BR-HOLMES-004: Must provide function descriptions")
			Expect(tool.Usage).ToNot(BeEmpty(), "BR-HOLMES-004: Must provide usage examples")
			Expect(tool.Parameters).ToNot(BeEmpty(), "BR-HOLMES-004: Must document parameters")
			Expect(tool.Examples).ToNot(BeEmpty(), "BR-HOLMES-004: Must provide usage examples")

			// Validate parameter documentation
			param := tool.Parameters[0]
			Expect(param.Name).ToNot(BeEmpty(), "BR-HOLMES-004: Must document parameter names")
			Expect(param.Description).ToNot(BeEmpty(), "BR-HOLMES-004: Must document parameter descriptions")
			Expect(param.Type).ToNot(BeEmpty(), "BR-HOLMES-004: Must document parameter types")
			Expect(param.Examples).ToNot(BeEmpty(), "BR-HOLMES-004: Must provide parameter examples")

			// Validate practical examples
			example := tool.Examples[0]
			Expect(example.Description).ToNot(BeEmpty(), "BR-HOLMES-004: Must provide example descriptions")
			Expect(example.Command).To(ContainSubstring("get_kubernetes_context"), "BR-HOLMES-004: Must provide executable commands")
			Expect(example.Expected).ToNot(BeEmpty(), "BR-HOLMES-004: Must describe expected results")

			// Validate usage guidelines
			Expect(docs.UsageGuidelines).ToNot(BeEmpty(), "BR-HOLMES-004: Must provide usage guidelines")
			Expect(docs.BestPractices).ToNot(BeEmpty(), "BR-HOLMES-004: Must provide best practices")

			logger.WithFields(logrus.Fields{
				"business_requirement": "BR-HOLMES-004",
				"documented_tools":     len(docs.Tools),
				"example_count":        len(tool.Examples),
				"parameter_count":      len(tool.Parameters),
			}).Info("BR-HOLMES-004: Function documentation and usage examples validation completed")
		})
	})
})

// TestRunner bootstraps the Ginkgo test suite
func TestUtoolsetUdeploymentUclient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UtoolsetUdeploymentUclient Suite")
}
