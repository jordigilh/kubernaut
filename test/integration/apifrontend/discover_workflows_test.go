package apifrontend_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/ka"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/tools"
)

var _ = Describe("Integration: discover_workflows (#1176)", Label("integration", "discover-workflows"), func() {
	var (
		mockMCP *ka.MockMCPClient
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockMCP = &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, args ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				workflows := []ka.DiscoveredWorkflow{
					{
						WorkflowID:  "wf-restart",
						Name:        "Pod Restart",
						Description: "Restart a pod in the target namespace",
						Kind:        "remediation",
						Parameters: []ka.WorkflowParameterSchema{
							{Name: "namespace", Type: "string", Required: true, Description: "Target namespace"},
							{Name: "pod_name", Type: "string", Required: true, Description: "Pod to restart"},
							{Name: "grace_period", Type: "int", Required: false, Default: 30},
						},
					},
					{
						WorkflowID:  "wf-scale",
						Name:        "Scale Deployment",
						Description: "Scale a deployment up or down",
						Kind:        "remediation",
						Parameters: []ka.WorkflowParameterSchema{
							{Name: "namespace", Type: "string", Required: true},
							{Name: "deployment", Type: "string", Required: true},
							{Name: "replicas", Type: "int", Required: true},
						},
					},
				}
				if args.WorkflowID != "" {
					for _, w := range workflows {
						if w.WorkflowID == args.WorkflowID {
							return &ka.DiscoverWorkflowsResult{Workflows: []ka.DiscoveredWorkflow{w}}, nil
						}
					}
					return &ka.DiscoverWorkflowsResult{Workflows: []ka.DiscoveredWorkflow{}}, nil
				}
				if args.Kind != "" {
					var filtered []ka.DiscoveredWorkflow
					for _, w := range workflows {
						if w.Kind == args.Kind {
							filtered = append(filtered, w)
						}
					}
					return &ka.DiscoverWorkflowsResult{Workflows: filtered}, nil
				}
				return &ka.DiscoverWorkflowsResult{Workflows: workflows}, nil
			},
			SelectWorkflowFn: func(_ context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				if args.Parameters != nil {
					return &ka.SelectWorkflowResult{Status: "accepted", Message: fmt.Sprintf("workflow %s with %d params", args.WorkflowID, len(args.Parameters))}, nil
				}
				return &ka.SelectWorkflowResult{Status: "accepted", Message: "workflow selected"}, nil
			},
		}
	})

	It("IT-AF-WP-001: HandleDiscoverWorkflows returns full schema from mock KA", func() {
		result, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		Expect(result.Workflows[0].Parameters).To(HaveLen(3))
		Expect(result.Workflows[0].Parameters[0].Name).To(Equal("namespace"))
	})

	It("IT-AF-WP-002: discover → validate → select round-trip", func() {
		discoverResult, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{WorkflowID: "wf-restart"})
		Expect(err).NotTo(HaveOccurred())
		Expect(discoverResult.Count).To(Equal(1))

		wf := discoverResult.Workflows[0]
		params := map[string]any{"namespace": "prod", "pod_name": "api-server-0"}
		err = tools.ValidateWorkflowParameters(wf.Parameters, params)
		Expect(err).NotTo(HaveOccurred())

		selectResult, err := tools.HandleSelectWorkflow(ctx, mockMCP, tools.SelectWorkflowArgs{
			RRID:       "pay/rr-1",
			WorkflowID: wf.WorkflowID,
			Parameters: params,
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(selectResult.Status).To(Equal("accepted"))
	})

	It("IT-AF-WP-003: filter by kind returns subset", func() {
		result, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{Kind: "remediation"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(2))
		for _, w := range result.Workflows {
			Expect(w.Kind).To(Equal("remediation"))
		}
	})

	It("IT-AF-WP-004: validation rejects invalid parameters before select", func() {
		discoverResult, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{WorkflowID: "wf-scale"})
		Expect(err).NotTo(HaveOccurred())
		Expect(discoverResult.Count).To(Equal(1))

		wf := discoverResult.Workflows[0]
		badParams := map[string]any{"namespace": "prod"}
		err = tools.ValidateWorkflowParameters(wf.Parameters, badParams)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("deployment"))
	})

	It("IT-AF-WP-005: KA MCP unreachable returns ErrMCPUnavailable", func() {
		unreachableMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return nil, ka.ErrMCPUnavailable
			},
		}
		_, err := tools.HandleDiscoverWorkflows(ctx, unreachableMCP, tools.DiscoverWorkflowsArgs{})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unavailable"))
	})

	It("IT-AF-WP-006: concurrent discover calls are safe under -race", func() {
		const concurrency = 50
		errs := make(chan error, concurrency)
		for i := 0; i < concurrency; i++ {
			go func() {
				_, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{})
				errs <- err
			}()
		}
		for i := 0; i < concurrency; i++ {
			Expect(<-errs).NotTo(HaveOccurred())
		}
	})

	It("IT-AF-WP-007: numeric parameters (float64 from JSON) survive validate and select round-trip", func() {
		discoverResult, err := tools.HandleDiscoverWorkflows(ctx, mockMCP, tools.DiscoverWorkflowsArgs{WorkflowID: "wf-scale"})
		Expect(err).NotTo(HaveOccurred())
		Expect(discoverResult.Count).To(Equal(1))

		wf := discoverResult.Workflows[0]

		// Real JSON decode produces float64 for numeric values, not int
		params := map[string]any{
			"namespace":  "production",
			"deployment": "api-server",
			"replicas":   float64(5),
		}

		err = tools.ValidateWorkflowParameters(wf.Parameters, params)
		Expect(err).NotTo(HaveOccurred(), "float64 should satisfy int schema via type switch")

		var receivedParams map[string]any
		paramCaptureMCP := &ka.MockMCPClient{
			SelectWorkflowFn: func(_ context.Context, args ka.SelectWorkflowArgs) (*ka.SelectWorkflowResult, error) {
				receivedParams = args.Parameters
				return &ka.SelectWorkflowResult{Status: "accepted", Message: "ok"}, nil
			},
		}

		selectResult, err := tools.HandleSelectWorkflow(ctx, paramCaptureMCP, tools.SelectWorkflowArgs{
			RRID:       "it/rr-numeric-test",
			WorkflowID: wf.WorkflowID,
			Parameters: params,
		}, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(selectResult.Status).To(Equal("accepted"))
		Expect(receivedParams).To(HaveKeyWithValue("replicas", float64(5)))
		Expect(receivedParams).To(HaveKeyWithValue("namespace", "production"))
	})

	Context("HTTP MCP bridge integration", func() {
		var ts *httptest.Server

		BeforeEach(func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var rpcReq struct {
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				if err := json.Unmarshal(body, &rpcReq); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				switch rpcReq.Method {
				case "initialize":
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":"1","result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"test","version":"1.0"}}}`)
				case "tools/list":
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":"1","result":{"tools":[{"name":"kubernaut_discover_workflows","description":"Discover workflows"}]}}`)
				case "tools/call":
					var params struct {
						Name string `json:"name"`
					}
					_ = json.Unmarshal(rpcReq.Params, &params)
					if params.Name == "kubernaut_discover_workflows" {
						if !strings.Contains(r.Header.Get("Authorization"), "Bearer") {
							w.WriteHeader(http.StatusUnauthorized)
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":"1","result":{"content":[{"type":"text","text":"{\"workflows\":[{\"workflow_id\":\"wf-1\",\"name\":\"Test\",\"parameters\":[]}]}"}]}}`)
					}
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			ts = httptest.NewServer(mux)
		})

		AfterEach(func() {
			ts.Close()
		})

		It("IT-AF-WP-006b: HTTP-level MCP round-trip discover_workflows", func() {
			body := `{"jsonrpc":"2.0","id":"call-1","method":"tools/call","params":{"name":"kubernaut_discover_workflows","arguments":{}}}`
			req, err := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")

			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			respBody, _ := io.ReadAll(resp.Body)
			Expect(string(respBody)).To(SatisfyAny(
				ContainSubstring("kubernaut_discover_workflows"),
				ContainSubstring("wf-1"),
			))
		})
	})

	It("IT-AF-WP-008: full round-trip with divergent targets (#1437)", func() {
		targetMCP := &ka.MockMCPClient{
			DiscoverWorkflowsFn: func(_ context.Context, _ ka.DiscoverWorkflowsArgs) (*ka.DiscoverWorkflowsResult, error) {
				return &ka.DiscoverWorkflowsResult{
					Workflows: []ka.DiscoveredWorkflow{
						{WorkflowID: "fix-config", Name: "Fix Config", Description: "Fix misconfigured ConfigMap"},
					},
					SearchedTarget: &ka.DiscoveryTarget{
						APIVersion: "v1",
						Kind:       "ConfigMap",
						Name:       "worker-config",
						Namespace:  "demo-storefront",
					},
					SignalTarget: &ka.DiscoveryTarget{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "worker",
						Namespace:  "demo-storefront",
					},
				}, nil
			},
		}

		result, err := tools.HandleDiscoverWorkflows(ctx, targetMCP, tools.DiscoverWorkflowsArgs{RRID: "rr-it-008"})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Count).To(Equal(1))

		Expect(result.SearchedTarget).NotTo(BeNil(), "IT: SearchedTarget must survive full handler round-trip")
		Expect(result.SearchedTarget.Kind).To(Equal("ConfigMap"))
		Expect(result.SearchedTarget.APIVersion).To(Equal("v1"))

		Expect(result.SignalTarget).NotTo(BeNil(), "IT: SignalTarget must survive full handler round-trip")
		Expect(result.SignalTarget.Kind).To(Equal("Deployment"))
		Expect(result.SignalTarget.APIVersion).To(Equal("apps/v1"))

		data, err := json.Marshal(result)
		Expect(err).NotTo(HaveOccurred())
		var parsed map[string]json.RawMessage
		Expect(json.Unmarshal(data, &parsed)).To(Succeed())
		Expect(parsed).To(HaveKey("searched_target"), "JSON output must include searched_target")
		Expect(parsed).To(HaveKey("signal_target"), "JSON output must include signal_target")
	})
})
