# BR-HAPI-433: Reimplement HolmesGPT SDK and HAPI in Go

**Business Requirement ID**: BR-HAPI-433
**Category**: HolmesGPT-API Service
**Priority**: P0 (CRITICAL)
**Target Version**: v1.3
**Status**: ✅ Approved
**Date**: 2026-03-04

**Related Design Decisions**:
- [DD-HAPI-019: Go Rewrite Design](../../architecture/decisions/DD-HAPI-019-go-rewrite-design/)

**Related Business Requirements**:
- BR-HAPI-211: LLM Input Sanitization (carried forward, reimplemented in Go)
- BR-HAPI-197: Human Review Required Flag (preserved, no regression)
- BR-HAPI-212: RCA Target Resource (preserved, no regression)

---

## 📋 **Business Need**

### **Problem Statement**

HolmesGPT-API (HAPI) is implemented in Python, wrapping the HolmesGPT Python SDK. This creates multiple business-critical problems that cannot be resolved within the Python stack.

**Current Limitations**:
- ❌ **Open CVEs**: The Python dependency chain (HolmesGPT SDK, prometrix, etc.) introduces CVEs we cannot fix because upstream libraries pin vulnerable versions
- ❌ **Container image size**: ~2.5GB image due to Python runtime, pip dependencies, and bundled CLI binaries (kubectl, helm, etc.)
- ❌ **Memory footprint**: Python's runtime overhead is significantly higher than the Go-based Kubernaut services
- ❌ **No toolset control**: HolmesGPT's toolset surface area is inherited wholesale — we cannot curate which integrations we support
- ❌ **Shell execution risk**: All toolsets execute via `subprocess.run(cmd, shell=True, executable="/bin/bash")`, creating shell injection attack surface
- ❌ **CI/CD friction**: 40+ minute arm64 pip installs under QEMU for multi-architecture builds
- ❌ **Language island**: HAPI is the only Python service in an otherwise pure Go codebase, requiring separate expertise and tooling

**Impact**:
- Security audit exposure from unresolvable CVEs
- Operator complaints about image pull times and memory usage
- Shell injection as an attack vector through tool call arguments
- Developer friction maintaining two language stacks

---

## 🎯 **Business Objective**

Rewrite HAPI as a native Go service with **no feature regression** in v1.3, eliminating all Python dependencies, shell execution, and CLI binaries while gaining control over the toolset catalog, LLM provider support, and security posture.

### **Success Criteria**

1. ✅ Zero Python code in HAPI's container image
2. ✅ Zero shell execution — all toolsets use Go bindings (client-go, net/http)
3. ✅ Container image size reduced to ~50-80MB (from ~2.5GB)
4. ✅ All open HolmesGPT-inherited CVEs eliminated
5. ✅ Feature parity with current HAPI for core investigation flows
6. ✅ Same REST API contract — no consumer changes required (RemediationOrchestrator, Gateway)
7. ✅ Kubernetes and Prometheus toolsets reimplemented with Go bindings
8. ✅ Multi-provider LLM support (OpenAI, Ollama, Azure OpenAI, Vertex AI, Anthropic, AWS Bedrock, Hugging Face, Mistral)
9. ✅ Fail-closed startup when DataStorage is configured but workflow validator cannot be created (DD-HAPI-002 v1.5, TP-433-ADV GAP-007)

---

## 📊 **Use Cases**

### **Use Case 1: Incident Investigation (Feature Parity)**

**Scenario**: AlertManager fires a CrashLoopBackOff alert. The signal pipeline delivers it to HAPI as an IncidentRequest.

**Current Flow (Python)**:
1. HAPI receives IncidentRequest
2. Constructs investigation prompt from signal context
3. Calls LLM via HolmesGPT SDK (Python)
4. LLM invokes tools via `subprocess.run("kubectl ...")` (shell execution)
5. Multi-turn agentic loop until workflow selected
6. Returns structured result with workflow recommendation

**Desired Flow (Go, BR-HAPI-433)**:
1. HAPI receives IncidentRequest (same REST API contract)
2. Constructs investigation prompt via Go `text/template`
3. Calls LLM via LangChainGo (Go)
4. LLM invokes tools via client-go / net/http (no shell, no kubectl binary)
5. Multi-turn agentic loop with per-turn tool scoping (new capability)
6. Returns structured result with workflow recommendation (same response format)

### **Use Case 2: Security Posture Improvement**

**Scenario**: Security audit identifies shell injection risk in HAPI toolsets.

**Current Flow**:
1. Auditor discovers `subprocess.run(cmd, shell=True)` in HolmesGPT source
2. ❌ Cannot remediate — HolmesGPT SDK controls execution model
3. ❌ CVEs in Python dependencies cannot be fixed (upstream pins)

**Desired Flow (Go, BR-HAPI-433)**:
1. Auditor reviews Go service — no shell execution, no subprocess calls
2. ✅ All K8s operations via client-go (type-safe, no injection vector)
3. ✅ All Prometheus queries via Go net/http (no Python dependencies)
4. ✅ Distroless container image — no shell, no package manager

---

## 🔧 **Functional Requirements**

### **FR-HAPI-433-01: Core Agentic Loop**

**Requirement**: HAPI SHALL implement a multi-turn agentic loop (LLM call → tool execution → feed results → repeat) as a Kubernaut-owned component, not delegated to a framework.

**Acceptance Criteria**:
- ✅ Investigation loop supports configurable maximum turns
- ✅ Tool results fed back as `role: "tool"` messages (API-level separation)
- ✅ Loop terminates on structured result or max-turn exhaustion

### **FR-HAPI-433-02: LLM Provider Support**

**Requirement**: HAPI SHALL support multiple LLM providers via LangChainGo, abstracted behind a Kubernaut-owned `llm.Client` interface.

**Acceptance Criteria**:
- ✅ OpenAI, Ollama, Azure OpenAI, Google Vertex AI, Anthropic, AWS Bedrock, Hugging Face, Mistral supported (all 8 implemented in v1.3)
- ✅ Provider selection via configuration (`llm.provider` field, no code changes)
- ✅ Azure requires `azure_api_version`; Vertex requires `vertex_project`; Bedrock uses AWS SDK credential chain (optional `bedrock_region` override)
- ✅ Framework-specific code isolated to a single adapter file (~120 LOC)
- ✅ Air-gapped/on-prem: Ollama + OpenAI-compatible endpoints documented. LangChainGo `local` provider rejected (subprocess execution violates security requirements).
- ⏳ AWS SigV4 signing for Prometheus deferred to v1.4 (see FR-HAPI-433-04)

### **FR-HAPI-433-03: Kubernetes Toolset (Go Bindings)**

**Requirement**: HAPI SHALL reimplement the Kubernetes toolset using `k8s.io/client-go`. No kubectl binary, no shell execution.

**Scope**: See [BR-HAPI-433-002: Kubernetes Toolset](BR-HAPI-433-002-kubernetes-toolset.md) for tier analysis and 11-tool selection.

**Acceptance Criteria**:
- ✅ 11 tools (Tier 1 + Tier 2) implemented via client-go
- ✅ Structured output (Go structs → JSON) instead of kubectl text tables
- ✅ Built-in size control (TailLines, LimitBytes) for log tools

### **FR-HAPI-433-04: Prometheus Toolset (Go HTTP)**

**Requirement**: HAPI SHALL reimplement the Prometheus toolset using Go `net/http` against the Prometheus HTTP API. No Python dependencies, no prometrix.

**Scope**: See [BR-HAPI-433-003: Prometheus Toolset](BR-HAPI-433-003-prometheus-toolset.md) for tier analysis and 6-tool selection.

**Acceptance Criteria**:
- ✅ 6 tools (Tier 1 + Tier 2) implemented via Go net/http
- ✅ `list_prometheus_rules` dropped (redundant — signal pipeline already synthesizes alert context; injection risk from rule annotations)
- ⏳ AWS AMP (SigV4) support deferred to v1.4 (dependency on aws-sdk-go-v2 not yet integrated)

### **FR-HAPI-433-05: HAPI-Custom Toolsets (Port to Go)**

**Requirement**: HAPI SHALL port the two HAPI-custom toolsets from Python to Go.

**Acceptance Criteria**:
- ✅ Workflow Discovery (list_available_actions, list_workflows, get_workflow) ported — DD-HAPI-017 three-step protocol preserved
- ✅ Resource Context (get_resource_context) ported — owner chain resolution, spec hash, remediation history

### **FR-HAPI-433-06: Per-Turn Tool Scoping (New Capability)**

**Requirement**: HAPI SHOULD support dynamic per-turn tool scoping — only providing relevant tools to the LLM at each turn of the investigation loop.

**Acceptance Criteria**:
- ✅ Investigation phases (RCA, workflow discovery, validation) have phase-appropriate tool subsets
- ✅ Reduces token usage and limits attack surface per turn

### **FR-HAPI-433-07: REST API Contract Preservation**

**Requirement**: HAPI SHALL preserve the existing REST API contract. Consumers (RemediationOrchestrator, Gateway) SHALL require no changes.

**Acceptance Criteria**:
- ✅ Same endpoints (`POST /analyze`, `GET /session/{id}`, `GET /result`)
- ✅ Same request/response schemas
- ✅ Same authentication model (TokenReview/SAR)

### **FR-HAPI-433-08: Prompt System Migration**

**Requirement**: HAPI SHALL migrate the prompt system from Jinja2 to Go `text/template`.

**Acceptance Criteria**:
- ✅ All investigation prompts migrated
- ✅ Prometheus LLM instructions migrated
- ✅ PromQL guidance preserved

### **FR-HAPI-433-09: Session Management**

**Requirement**: HAPI SHALL implement async session management using goroutines, replacing Python BackgroundTasks.

**Acceptance Criteria**:
- ✅ `POST /analyze` returns 202 with session ID
- ✅ Investigation runs in background goroutine
- ✅ Result retrievable via `GET /result`

---

## 📈 **Non-Functional Requirements**

### **NFR-HAPI-433-01: Container Image Size**

**Target**: ≤80MB (from ~2.5GB)
**Measure**: `docker images` output for HAPI image

### **NFR-HAPI-433-02: Memory Footprint**

**Target**: ≤200MB RSS at idle, ≤500MB under peak load
**Measure**: Prometheus `process_resident_memory_bytes`

### **NFR-HAPI-433-03: Startup Time**

**Target**: ≤5 seconds (from ~20s single-worker Python)
**Measure**: Time from container start to health endpoint returning 200

### **NFR-HAPI-433-04: Build Time**

**Target**: ≤5 minutes for multi-architecture build (from 40+ minutes with QEMU pip)
**Measure**: CI pipeline duration

### **NFR-HAPI-433-05: Zero CVEs from Python Dependencies**

**Target**: 0 CVEs inherited from Python dependency chain
**Measure**: `trivy image` scan

---

## 🔗 **Dependencies**

### **Upstream Dependencies**
- ✅ LangChainGo framework (selected — see [BR-HAPI-433-001](BR-HAPI-433-001-framework-evaluation.md))
- ✅ `k8s.io/client-go` for Kubernetes toolset
- ✅ `aws-sdk-go-v2` for AWS AMP SigV4 signing
- ✅ DataStorage Service OpenAPI client (existing, no changes)

### **Downstream Impacts**
- ✅ RemediationOrchestrator — no changes (same REST API contract)
- ✅ Gateway — no changes (same audit emission format)
- ✅ Helm chart — container image reference changes, env vars may simplify
- ✅ CI/CD — Python build stage removed, Go build stage added

---

## 🚀 **Implementation Phases**

### **Phase 1: Core Engine** (v1.3)
- Kubernaut-owned agentic loop and LLM interface
- LangChainGo adapter
- Session management (goroutine-based)
- REST API endpoints (same contract)
- Auth middleware (TokenReview/SAR)
- Config hot-reload
- Health/ready/metrics endpoints

### **Phase 2: Toolsets** (v1.3)
- Kubernetes toolset (11 tools, client-go)
- Prometheus toolset (6 tools, net/http)
- HAPI-custom toolsets (workflow discovery, resource context)
- Prompt system (Go text/template)

### **Phase 3: Security Hardening** (v1.3)
- Tool-output sanitization (BR-HAPI-211 in Go)
- Per-phase tool scoping
- Output validation hardening
- Behavioral anomaly detection

### **Phase 4: Advanced Capabilities** (v1.4, deferred)
- CaMeL prompt injection defense (dual-LLM architecture)
- Multi-LLM support (audit/guardrail LLM)
- MCP extensibility for custom tools
- Eino framework re-evaluation for multi-agent scenarios
- AWS AMP SigV4 signing for Prometheus toolset

---

## 📊 **Success Metrics**

### **Image Size Reduction**
- **Target**: ≥95% reduction (2.5GB → ≤80MB)
- **Measure**: Container image size comparison

### **CVE Elimination**
- **Target**: 0 Python-inherited CVEs
- **Measure**: Trivy scan before/after

### **Build Time Improvement**
- **Target**: ≥85% reduction (40min → ≤5min)
- **Measure**: CI pipeline comparison

### **Investigation Correctness**
- **Target**: 100% parity with Python HAPI on mock-llm test scenarios
- **Measure**: Same mock-llm conversations produce same workflow selections

---

## 🔄 **Alternatives Considered**

### **Alternative 1: Keep Python HAPI, Fix CVEs Upstream**
**Approach**: Contribute fixes to HolmesGPT SDK and its dependencies.
**Rejected Because**: Upstream pins vulnerable versions; we don't control their release cycle. Shell execution risk remains. Image size remains. Language island remains.

### **Alternative 2: Rewrite HAPI with Two-Phase Toolkit Layer (Python first, then Go)**
**Approach**: Implement new toolset abstraction in Python HAPI, then rewrite to Go.
**Rejected Because**: Double implementation effort. Security issues (shell execution, CVEs) persist during the Python phase. Image size problem not addressed until Go phase.

### **Alternative 3: Use kagent as Go LLM Framework**
**Approach**: Use kagent's Kubernetes-native CRD-based agent platform.
**Rejected Because**: Agent runtime is Python-based (AutoGen 0.4). Does not eliminate Python dependency. See [#505](https://github.com/jordigilh/kubernaut/issues/505).

---

## ✅ **Approval**

**Status**: ✅ Approved
**Date**: 2026-03-04
**Decision**: Proceed with Go rewrite using LangChainGo for v1.3 scope
**Approved By**: Architecture Team
**Related DD**: [DD-HAPI-019: Go Rewrite Design](../../architecture/decisions/DD-HAPI-019-go-rewrite-design/)

---

## 📚 **References**

### **Related Business Requirements**
- [BR-HAPI-211](../BR-HAPI-211-llm-input-sanitization.md): LLM Input Sanitization
- [BR-HAPI-197](../BR-HAPI-197-needs-human-review-field.md): Human Review Required Flag

### **Related Documents**
- [13_HOLMESGPT_REST_API_WRAPPER.md](../13_HOLMESGPT_REST_API_WRAPPER.md): Original HAPI business requirements (Python era)
- [DD-HAPI-017](../../architecture/decisions/DD-HAPI-017-three-step-workflow-discovery-integration.md): Three-Step Workflow Discovery (preserved in Go)
- [DD-HAPI-005](../../architecture/decisions/DD-HAPI-005-llm-input-sanitization.md): LLM Input Sanitization design (reimplemented in Go)

### **Subdocuments**
- [BR-HAPI-433-001: Framework Evaluation](BR-HAPI-433-001-framework-evaluation.md)
- [BR-HAPI-433-002: Kubernetes Toolset](BR-HAPI-433-002-kubernetes-toolset.md)
- [BR-HAPI-433-003: Prometheus Toolset](BR-HAPI-433-003-prometheus-toolset.md)
- [BR-HAPI-433-004: Security Requirements](BR-HAPI-433-004-security-requirements.md)

---

**Document Version**: 1.2
**Last Updated**: 2026-04-03
**Status**: ✅ Approved

**Change Log**:
- v1.2 (2026-04-03): Updated FR-HAPI-433-02 to reflect 8 implemented providers (OpenAI, Ollama, Azure, Vertex, Anthropic, Bedrock, Hugging Face, Mistral). Updated adapter LOC estimate (~120 LOC). Added air-gapped/on-prem guidance. Documented rejection of LangChainGo `local` provider.
- v1.1 (2026-04-03): Updated FR-HAPI-433-02 to reflect 4 implemented providers (OpenAI, Azure, Vertex, Ollama). Documented SigV4 deferral to v1.4 in FR-HAPI-433-04. Updated adapter LOC estimate (~80 LOC). Added SigV4 to Phase 4 scope.
