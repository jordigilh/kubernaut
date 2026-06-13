# Test Plan: Provider-Aware Triage LLM Factory

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1404-v1
**Feature**: Independent LLM configuration for severity triage (`severityTriage.llm`)
**Version**: 1.0
**Created**: 2026-06-13
**Author**: AI Agent
**Status**: Implemented
**Branch**: `feat/structured-decision-payload`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the provider-aware factory routing for the severity triage
LLM. The AF supports an independent `severityTriage.llm` config section that can
specify a different model/provider than the main `agent.llm`, enabling cost-optimized
severity classification (e.g., using a lighter model for triage while keeping a
powerful model for the main agent).

### 1.2 Objectives

1. **Factory routing**: `vertex_ai` provider with Claude model → `AnthropicTriager`; Gemini → `GenAITriager`
2. **Config independence**: `severityTriage.llm` configured independently from `agent.llm`
3. **Config inheritance**: When `severityTriage.llm` is nil, inherits from `agent.llm`
4. **Validation**: Invalid provider/model combinations rejected at startup
5. **Provider-specific fields**: `vertexProject` required for vertex_ai provider

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/config/... -run "UT-AF-1404"` |
| Integration test pass rate | 100% | `go test ./cmd/apifrontend/... -run "IT-AF-1404"` |
| Config validation coverage | 100% | All provider/model combos tested |

---

## 2. References

### 2.1 Authority

- Issue #1404: Provider-aware triage LLM factory
- BR-AI-1404: Independent triage model configuration (code-level reference)

### 2.2 FedRAMP Controls

| Control | Intent | Application | Test ID |
|---------|--------|-------------|---------|
| SI-10 | Input validation | Invalid config rejected at startup | UT-AF-1404-006..009 |
| AC-6 | Least privilege | Triage model scoped to severity only | IT-AF-1404-001 |

---

## 3. Test Scenarios

### 3.1 Unit Tests (Config Validation)

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| UT-AF-1404-004 | Valid vertex_ai config accepted | No validation error | Implemented |
| UT-AF-1404-005 | Valid gemini config accepted | No validation error | Implemented |
| UT-AF-1404-006 | Missing model rejected | Validation error | Implemented |
| UT-AF-1404-007 | Invalid provider rejected | Validation error | Implemented |
| UT-AF-1404-008 | Nil severityTriage.llm (inherits agent.llm) skips validation | No error | Implemented |
| UT-AF-1404-009 | vertex_ai without vertexProject rejected | Validation error | Implemented |

### 3.2 Integration Tests (Factory Routing)

| ID | Scenario | Expected | Status |
|----|----------|----------|--------|
| IT-AF-1404-001 | vertex_ai + Claude model → AnthropicTriager | Factory produces correct type | Implemented |
| IT-AF-1404-002 | vertex_ai + Gemini model → GenAITriager | Factory produces correct type | Implemented |
| IT-AF-1404-003 | Config resolution prefers severityTriage.llm over agent.llm | Explicit source selected | Implemented |

---

## 4. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT/E2E Test ID |
|-----------|----------------------|---------------------|----------------|
| TriageLLMFactory | buildTriager() | cmd/apifrontend/main.go | IT-AF-1404-001 |
| Config validation | validateSeverityTriageLLM() | pkg/apifrontend/config/config.go | UT-AF-1404-006 |
| Config resolution | resolveLLMConfig() | cmd/apifrontend/main.go | IT-AF-1404-003 |

---

## 5. Execution

```bash
go test ./pkg/apifrontend/config/... -run "UT-AF-1404" -v -count=1
go test ./cmd/apifrontend/... -run "IT-AF-1404" -v -count=1
```
