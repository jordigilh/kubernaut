✅ **COMPLETE** - February 6, 2026 (Different Implementation)

**Reason**: Mock LLM logic was successfully extracted to an external service, but implemented as a **Python service** (`test/services/mock-llm/`) rather than the Go service originally proposed in this document.

**Actual Implementation**: DD-TEST-011 v2.0 - File-based Mock LLM service using Python
- **Location**: `test/services/mock-llm/`
- **Configuration**: YAML-based scenario configuration (`test/services/mock-llm/config/scenarios.yaml`)
- **Integration**: Used by HAPI, AIAnalysis, and integration tests
- **Benefit**: Leverages existing Python ecosystem, file-based config allows easy scenario updates

**Key Differences from This Document**:
- ✅ **Goal Achieved**: Mock LLM extracted from HAPI business logic
- ✅ **Separation of Concerns**: Mock logic is external service
- ✅ **Reusability**: Shared across multiple services
- ❌ **Implementation Language**: Python (not Go as proposed)
- ✅ **Configuration**: File-based YAML (simpler than Go code scenarios)

This document is retained for historical context to show the original design thinking, but should not be implemented as specified.

---

# TD-HAPI-001: Extract Mock LLM Logic to External Service

**Created**: December 30, 2025  
**Priority**: ~~High~~ ✅ COMPLETE (implemented differently)
**Effort**: ~~2-3 days~~ Completed via DD-TEST-011 v2.0
**Owner**: HAPI Team  
**Status**: ~~Approved for next branch~~ **COMPLETE** (Python implementation)  

---

## 🎯 PROBLEM STATEMENT

### Current Architecture (V1.0 - Sub-Optimal)

**Issue**: Mock LLM logic is embedded within HAPI service business code, violating separation of concerns.

```
┌─────────────────────────────────────────┐
│            HAPI Service                 │
│  ┌────────────────────────────────┐    │
│  │   Business Logic               │    │
│  │   src/extensions/incident/     │    │
│  │   src/extensions/recovery/     │    │
│  └────────────────────────────────┘    │
│  ┌────────────────────────────────┐    │
│  │   Mock Logic (PROBLEM!)        │    │
│  │   src/mock_responses.py        │    │
│  │   - MOCK_NO_WORKFLOW_FOUND     │    │
│  │   - MOCK_LOW_CONFIDENCE        │    │
│  │   - Edge case handling         │    │
│  └────────────────────────────────┘    │
└─────────────────────────────────────────┘
```

### Problems with Current Implementation

1. **Separation of Concerns Violation**
   - HAPI contains business logic + mock logic
   - Single Responsibility Principle violated
   - Harder to maintain and reason about

2. **HAPI is Mock-Aware**
   ```python
   # src/mock_responses.py
   if signal_type.upper() == "MOCK_NO_WORKFLOW_FOUND":
       return mock_response(...)  # HAPI knows about mocking
   ```

3. **Environment Flag Coupling**
   ```python
   # HAPI checks MOCK_LLM_MODE flag
   if os.getenv("MOCK_LLM_MODE") == "true":
       return generate_mock_response(...)
   ```

4. **Reusability Issues**
   - Mock logic can't be shared with AIAnalysis service
   - Mock logic can't be shared with RemediationOrchestrator service
   - Each service would need its own mock implementation

5. **Testing Complexity**
   - Mock logic coupled to HAPI deployment
   - Can't easily swap mock/real LLM implementations
   - Infrastructure changes require HAPI code changes

---

## ✅ IDEAL ARCHITECTURE (V1.0 Next Branch)

### External Mock LLM Service

```
┌──────────────┐     ┌──────────────┐     ┌─────────────────┐
│   AIAnalysis │────▶│  HolmesGPT   │────▶│   Real LLM      │
│   Service    │     │     SDK      │     │   (OpenAI, etc) │
└──────────────┘     └──────────────┘     └─────────────────┘
                            │
┌──────────────┐            │              ┌─────────────────┐
│     HAPI     │────────────┤              │   Mock LLM      │
│   Service    │            │─────────────▶│   Server        │
└──────────────┘            │   (tests)    │  (External)     │
                            │              └─────────────────┘
┌──────────────┐            │
│ Remediation  │────────────┘
│ Orchestrator │
└──────────────┘
```

### Benefits

1. **Transparency** - Services don't know if LLM is real or mock
2. **Separation** - Mock logic lives in dedicated service
3. **Reusability** - All services use same mock server
4. **Testability** - Easy to swap implementations via configuration
5. **Infrastructure Control** - Mock server managed by Go test infrastructure

---

## 🏗️ IMPLEMENTATION PLAN

### Phase 1: Create External Mock LLM Server (New Component)

**Location**: `test/mock-llm-server/`

**Directory Structure**:
```
test/mock-llm-server/
├── main.go                    # Mock LLM HTTP server
├── handlers/
│   ├── incident.go           # Mock incident analysis responses
│   ├── recovery.go           # Mock recovery analysis responses
│   └── edge_cases.go         # Edge case handling (MOCK_* signals)
├── scenarios/
│   ├── happy_path.go         # Normal responses
│   ├── no_workflow.go        # MOCK_NO_WORKFLOW_FOUND
│   ├── low_confidence.go     # MOCK_LOW_CONFIDENCE
│   └── not_reproducible.go   # MOCK_NOT_REPRODUCIBLE
└── config.go                 # Port and endpoint configuration
```

**API Contract** (OpenAI-compatible):
```go
// POST /v1/chat/completions
type ChatCompletionRequest struct {
    Model    string    `json:"model"`
    Messages []Message `json:"messages"`
    Tools    []Tool    `json:"tools"`
}

type ChatCompletionResponse struct {
    ID      string   `json:"id"`
    Choices []Choice `json:"choices"`
}

type Choice struct {
    Message      Message       `json:"message"`
    FinishReason string        `json:"finish_reason"`
    ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
}
```

### Phase 2: Go Infrastructure Integration

**Location**: `test/infrastructure/mock_llm_server.go`

**Port Allocation Strategy** (Critical for Multi-Service):

```go
package infrastructure

import (
    "context"
    "fmt"
    "net"
    "time"
)

const (
    // Base port for mock LLM server
    // Each service gets its own mock LLM instance to avoid collisions
    MockLLMBasePort = 18760
    
    // Port offsets for different services
    MockLLMPortHAPI          = MockLLMBasePort + 0  // 18760
    MockLLMPortAIAnalysis    = MockLLMBasePort + 1  // 18761
    MockLLMPortRemediation   = MockLLMBasePort + 2  // 18762
)

// MockLLMServerConfig defines configuration for mock LLM server
type MockLLMServerConfig struct {
    ServiceName string
    Port        int
    ScenarioDir string // Path to scenario files
}

// StartMockLLMServer starts a mock LLM server for the specified service
func StartMockLLMServer(ctx context.Context, cfg MockLLMServerConfig) (*MockLLMServer, error) {
    server := &MockLLMServer{
        serviceName: cfg.ServiceName,
        port:        cfg.Port,
        scenarioDir: cfg.ScenarioDir,
    }
    
    // Verify port is available
    if err := server.checkPortAvailable(); err != nil {
        return nil, fmt.Errorf("port %d not available: %w", cfg.Port, err)
    }
    
    // Start HTTP server
    if err := server.start(ctx); err != nil {
        return nil, fmt.Errorf("failed to start mock LLM server: %w", err)
    }
    
    // Wait for server to be ready
    if err := server.waitForReady(30 * time.Second); err != nil {
        server.Stop()
        return nil, fmt.Errorf("mock LLM server not ready: %w", err)
    }
    
    return server, nil
}

type MockLLMServer struct {
    serviceName string
    port        int
    scenarioDir string
    server      *http.Server
}

func (m *MockLLMServer) checkPortAvailable() error {
    addr := fmt.Sprintf("127.0.0.1:%d", m.port)
    conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
    if err == nil {
        conn.Close()
        return fmt.Errorf("port already in use")
    }
    return nil
}

func (m *MockLLMServer) URL() string {
    return fmt.Sprintf("http://localhost:%d", m.port)
}

func (m *MockLLMServer) Stop() error {
    if m.server != nil {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        return m.server.Shutdown(ctx)
    }
    return nil
}
```

**Usage in Test Infrastructure**:

```go
// test/infrastructure/holmesgpt_api.go
func setupHAPITestInfrastructure(ctx context.Context) (*HAPITestInfra, error) {
    // Start mock LLM server for HAPI
    mockLLM, err := StartMockLLMServer(ctx, MockLLMServerConfig{
        ServiceName: "hapi",
        Port:        MockLLMPortHAPI,
        ScenarioDir: "test/mock-llm-server/scenarios",
    })
    if err != nil {
        return nil, fmt.Errorf("failed to start HAPI mock LLM: %w", err)
    }
    
    // Configure HAPI to use mock LLM
    hapiConfig := map[string]string{
        "LLM_ENDPOINT": mockLLM.URL(),
        "LLM_MODEL":    "mock-gpt-4",
        "OPENAI_API_KEY": "mock-key-for-testing",
    }
    
    // Deploy HAPI to Kind cluster
    // ...
    
    return &HAPITestInfra{
        MockLLM: mockLLM,
        // ...
    }, nil
}

// test/infrastructure/aianalysis.go
func setupAIAnalysisTestInfrastructure(ctx context.Context) (*AIAnalysisTestInfra, error) {
    // Start mock LLM server for AIAnalysis (different port!)
    mockLLM, err := StartMockLLMServer(ctx, MockLLMServerConfig{
        ServiceName: "aianalysis",
        Port:        MockLLMPortAIAnalysis,  // 18761
        ScenarioDir: "test/mock-llm-server/scenarios",
    })
    if err != nil {
        return nil, fmt.Errorf("failed to start AIAnalysis mock LLM: %w", err)
    }
    
    // Configure AIAnalysis to use its own mock LLM instance
    // ...
}
```

### Phase 3: Remove Mock Logic from HAPI

**Files to Modify**:

1. **DELETE**: `holmesgpt-api/src/mock_responses.py`
   - Remove all mock response generation logic
   - Remove edge case signal type handling
   - Remove MOCK_* constants

2. **MODIFY**: `holmesgpt-api/src/extensions/incident/endpoint.py`
   ```python
   # BEFORE (mock-aware):
   if os.getenv("MOCK_LLM_MODE") == "true":
       response = generate_mock_incident_response(request)
   else:
       response = self.llm_client.analyze(prompt)
   
   # AFTER (mock-agnostic):
   response = self.llm_client.analyze(prompt)  # Always use SDK
   ```

3. **MODIFY**: `holmesgpt-api/src/extensions/recovery/endpoint.py`
   ```python
   # Same pattern - remove all MOCK_LLM_MODE checks
   response = self.llm_client.analyze(prompt)
   ```

4. **MODIFY**: `holmesgpt-api/tests/conftest.py`
   ```python
   # BEFORE: Python-based mock server
   @pytest.fixture(scope="session")
   def mock_llm_server_e2e():
       from tests.mock_llm_server import MockLLMServer
       with MockLLMServer() as server:
           os.environ["LLM_ENDPOINT"] = server.url
           yield server
   
   # AFTER: Use Go-managed mock server
   @pytest.fixture(scope="session")
   def mock_llm_server_e2e():
       # Mock LLM server started by Go infrastructure
       # Just read environment variable set by Go
       mock_url = os.environ.get("LLM_ENDPOINT")
       if not mock_url:
           pytest.fail("LLM_ENDPOINT not set by test infrastructure")
       yield mock_url
   ```

### Phase 4: Update Test Infrastructure

**E2E Test Suite Setup**:

```go
// test/e2e/holmesgpt-api/holmesgpt_api_e2e_suite_test.go

var _ = BeforeSuite(func() {
    ctx := context.Background()
    
    // 1. Start mock LLM server (NEW!)
    mockLLMServer, err = infrastructure.StartMockLLMServer(ctx, 
        infrastructure.MockLLMServerConfig{
            ServiceName: "hapi",
            Port:        infrastructure.MockLLMPortHAPI,
            ScenarioDir: "test/mock-llm-server/scenarios",
        })
    Expect(err).ToNot(HaveOccurred())
    
    // 2. Create Kind cluster with HAPI
    hapiInfra, err = infrastructure.SetupHAPIInfrastructure(ctx, 
        infrastructure.HAPIConfig{
            LLMEndpoint: mockLLMServer.URL(),  // Point to mock
            // ...
        })
    Expect(err).ToNot(HaveOccurred())
    
    // 3. Set environment for Python tests
    os.Setenv("HAPI_BASE_URL", hapiInfra.ServiceURL)
    os.Setenv("LLM_ENDPOINT", mockLLMServer.URL())
    os.Setenv("DATA_STORAGE_URL", hapiInfra.DataStorageURL)
    
    // 4. Run Python pytest suite
    // ...
})

var _ = AfterSuite(func() {
    // Clean up mock LLM server
    if mockLLMServer != nil {
        mockLLMServer.Stop()
    }
    // Clean up HAPI infrastructure
    // ...
})
```

### Phase 5: Mock LLM Scenario Implementation

**Example**: `test/mock-llm-server/scenarios/no_workflow.go`

```go
package scenarios

import (
    "encoding/json"
    "fmt"
    "strings"
)

// NoWorkflowScenario handles MOCK_NO_WORKFLOW_FOUND edge case
type NoWorkflowScenario struct{}

func (s *NoWorkflowScenario) Matches(messages []Message) bool {
    // Check if any message contains MOCK_NO_WORKFLOW_FOUND
    for _, msg := range messages {
        if strings.Contains(msg.Content, "MOCK_NO_WORKFLOW_FOUND") {
            return true
        }
    }
    return false
}

func (s *NoWorkflowScenario) GenerateResponse(messages []Message) (*ChatCompletionResponse, error) {
    // Generate deterministic response for "no workflow found" scenario
    toolCall := ToolCall{
        ID:   "call_mock_no_workflow",
        Type: "function",
        Function: FunctionCall{
            Name: "search_workflows",
            Arguments: json.RawMessage(`{
                "signal_type": "MOCK_NO_WORKFLOW_FOUND",
                "severity": "critical",
                "component": "pod",
                "query": "No matching workflows for this signal type"
            }`),
        },
    }
    
    return &ChatCompletionResponse{
        ID: "mock-" + generateID(),
        Choices: []Choice{
            {
                Message: Message{
                    Role:      "assistant",
                    Content:   "",
                    ToolCalls: []ToolCall{toolCall},
                },
                FinishReason: "tool_calls",
            },
        },
    }, nil
}
```

---

## 📋 PORT ALLOCATION TABLE

**Critical**: Each service needs its own mock LLM instance to avoid port collisions during parallel test execution.

| Service | Port | Usage | Test Suite |
|---------|------|-------|------------|
| **HAPI** | 18760 | Mock LLM for HAPI E2E tests | `test/e2e/holmesgpt-api/` |
| **AIAnalysis** | 18761 | Mock LLM for AA integration tests | `test/integration/aianalysis/` |
| **RemediationOrchestrator** | 18762 | Mock LLM for RO integration tests | `test/integration/remediation/` |

**Port Range**: 18760-18799 (reserved for mock LLM servers)

**Collision Prevention**:
```go
// Each test suite checks port availability before starting
func checkPortAvailable(port int) error {
    addr := fmt.Sprintf("127.0.0.1:%d", port)
    conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
    if err == nil {
        conn.Close()
        return fmt.Errorf("port %d already in use", port)
    }
    return nil // Port is available
}
```

---

## 🎯 ACCEPTANCE CRITERIA

### Must Have (V1.0 Next Branch)

- [ ] Mock LLM server implemented in Go (`test/mock-llm-server/`)
- [ ] Port allocation strategy implemented with collision prevention
- [ ] HAPI mock logic removed (`mock_responses.py` deleted)
- [ ] HAPI is mock-agnostic (no `MOCK_LLM_MODE` checks)
- [ ] Go test infrastructure manages mock LLM lifecycle
- [ ] All 603 HAPI tests still passing
- [ ] AIAnalysis integration tests use same mock server (port 18761)
- [ ] RemediationOrchestrator tests use same mock server (port 18762)
- [ ] Mock LLM scenarios cover all edge cases:
  - MOCK_NO_WORKFLOW_FOUND
  - MOCK_LOW_CONFIDENCE
  - MOCK_NOT_REPRODUCIBLE
  - MOCK_MAX_RETRIES_EXHAUSTED
  - Happy path responses

### Nice to Have (Future)

- [ ] Mock LLM admin UI (view scenarios, inspect requests)
- [ ] Mock LLM metrics (request count, latency simulation)
- [ ] Scenario hot-reload (update scenarios without restart)
- [ ] Request/response logging for debugging

---

## 🧪 TESTING STRATEGY

### Unit Tests (Mock LLM Server)
- Test each scenario independently
- Verify OpenAI API compatibility
- Test error handling

### Integration Tests
- HAPI calls mock LLM (transparent to HAPI)
- AIAnalysis calls same mock LLM patterns
- RemediationOrchestrator calls same mock LLM patterns

### E2E Tests
- Full flow: HAPI → Mock LLM → Response processing
- Verify HAPI doesn't know LLM is mocked
- Parallel execution with separate mock instances

---

## 📊 METRICS

### Before Refactoring (Current)
- Mock logic: Embedded in HAPI (src/mock_responses.py)
- Reusability: 0% (HAPI-specific)
- Services using mocks: 1 (HAPI only)
- Mock-awareness: High (HAPI checks MOCK_LLM_MODE)

### After Refactoring (Target)
- Mock logic: External service (test/mock-llm-server/)
- Reusability: 100% (all services use same mock)
- Services using mocks: 3 (HAPI, AIAnalysis, RemediationOrchestrator)
- Mock-awareness: Zero (services don't know about mocking)

---

## 🚀 IMPLEMENTATION TIMELINE

### Sprint 1 (Days 1-2): Foundation
- Create `test/mock-llm-server/` structure
- Implement port allocation strategy
- Implement basic OpenAI-compatible endpoints

### Sprint 2 (Day 3): Scenario Migration
- Migrate edge case scenarios from HAPI
- Implement MOCK_NO_WORKFLOW_FOUND
- Implement MOCK_LOW_CONFIDENCE
- Implement MOCK_NOT_REPRODUCIBLE

### Sprint 3 (Day 4): HAPI Refactoring
- Remove `mock_responses.py`
- Remove `MOCK_LLM_MODE` checks
- Update test fixtures
- Verify all tests passing

### Sprint 4 (Day 5): Multi-Service Integration
- Integrate with AIAnalysis tests (port 18761)
- Integrate with RemediationOrchestrator tests (port 18762)
- Verify parallel execution works

---

## 🔒 RISKS & MITIGATION

### Risk 1: Port Collisions
**Mitigation**: 
- Reserved port range (18760-18799)
- Pre-flight port availability checks
- Clear port allocation table

### Risk 2: Test Flakiness During Migration
**Mitigation**:
- Migrate one service at a time
- Keep HAPI mock logic until new mock server validated
- Run full test suite after each migration step

### Risk 3: OpenAI API Compatibility
**Mitigation**:
- Use HolmesGPT SDK as reference implementation
- Test against real OpenAI API contract
- Validate with litellm library compatibility

---

## 📚 RELATED DOCUMENTS

- [BR-HAPI-197](../../docs/requirements/BR-HAPI-197-recovery-human-review.md) - Recovery human review
- [DD-API-001](../../docs/architecture/decisions/DD-API-001-openapi-client-mandate.md) - OpenAPI client usage
- [TESTING_GUIDELINES.md](../../TESTING_GUIDELINES.md) - Mock LLM strategy

---

## ✅ SIGN-OFF

**Technical Debt Owner**: HAPI Team  
**Approved By**: Technical Lead  
**Implementation Branch**: `feature/mock-llm-extraction` (next after current branch merge)  
**Target Release**: V1.0  

---

**Status**: 📋 **Documented** - Ready for implementation in next branch

