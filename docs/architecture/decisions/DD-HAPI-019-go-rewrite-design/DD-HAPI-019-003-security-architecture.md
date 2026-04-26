# DD-HAPI-019-003: Prompt Injection Security Architecture

**Status**: ✅ Approved
**Decision Date**: 2026-03-04
**Version**: 1.1
**Confidence**: 82%
**Deciders**: Architecture Team, Kubernaut Agent Team, Security Team
**Applies To**: Kubernaut Agent

**Related Business Requirements**:
- [BR-HAPI-433-004: Security Requirements](../../../requirements/BR-HAPI-433-go-language-migration/BR-HAPI-433-004-security-requirements.md)
- [BR-HAPI-211: LLM Input Sanitization](../../../requirements/BR-HAPI-211-llm-input-sanitization.md)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.1 | 2026-03-04 | Architecture Team | Renamed service references from HAPI to Kubernaut Agent, updated all file paths to kubernautagent |
| 1.0 | 2026-03-04 | Architecture Team | Initial design: layered defense for prompt injection in Kubernaut Agent tool pipeline |

---

## Context & Problem

### Current State

Current Python HAPI has limited prompt injection defense:
- BR-HAPI-211 (planned, not yet implemented): credential scrubbing
- HolmesGPT SDK: repeated tool call blocking
- No tool-output content sanitization
- Content-level delimiters (`<tool_result>`) for structural boundaries — vulnerable to closing-tag injection

### Problem Statement

Design a defense-in-depth security architecture for the Go rewrite that protects the investigation loop from indirect prompt injection via tool output, while preserving the LLM's ability to reason over legitimate data.

### Constraints

- Must not break legitimate investigation flows (low false-positive rate)
- Must be configurable (thresholds, patterns)
- Must be auditable (all sanitization events logged)
- CaMeL (dual-LLM) deferred to v1.4 — v1.3 layers must be sufficient without it

---

## Decision Drivers

1. **Defense in depth**: No single layer is sufficient. Multiple independent layers reduce the probability of successful injection.
2. **Structural over content-based**: API-level boundaries (`role: "tool"`) are harder to bypass than content-level delimiters.
3. **Least privilege per phase**: Tools available to the LLM should match the current investigation phase.
4. **Auditability**: Every defense action must be logged for post-incident analysis.

---

## Decision

### Layered Defense Architecture

```
┌─────────────────────────────────────────────────────────┐
│                  Investigation Loop                      │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ Phase Controller (I4)                              │  │
│  │ • Restricts tool set per investigation phase       │  │
│  │ • Rejects out-of-phase tool calls                  │  │
│  └──────────────────────┬─────────────────────────────┘  │
│                         │                                 │
│  ┌──────────────────────▼─────────────────────────────┐  │
│  │ Anomaly Detector (I7)                              │  │
│  │ • Excessive tool calls → abort                     │  │
│  │ • Repeated failures → abort                        │  │
│  │ • Suspicious arguments → flag                      │  │
│  └──────────────────────┬─────────────────────────────┘  │
│                         │                                 │
│  ┌──────────────────────▼─────────────────────────────┐  │
│  │ Tool Execution                                     │  │
│  └──────────────────────┬─────────────────────────────┘  │
│                         │                                 │
│  ┌──────────────────────▼─────────────────────────────┐  │
│  │ Sanitization Pipeline                              │  │
│  │ ┌────────────────────────────────────────────────┐ │  │
│  │ │ G4: Credential Scrubbing                       │ │  │
│  │ │ • 17 DD-005 pattern categories                 │ │  │
│  │ │ • Database URLs, API keys, tokens, secrets     │ │  │
│  │ └────────────────────────────────────────────────┘ │  │
│  │ ┌────────────────────────────────────────────────┐ │  │
│  │ │ I1: Injection Pattern Stripping                │ │  │
│  │ │ • Imperative sentences ("ignore", "select")    │ │  │
│  │ │ • JSON blocks mimicking LLM response format    │ │  │
│  │ │ • Closing tags (</tool_result>, </system>)     │ │  │
│  │ └────────────────────────────────────────────────┘ │  │
│  └──────────────────────┬─────────────────────────────┘  │
│                         │                                 │
│  ┌──────────────────────▼─────────────────────────────┐  │
│  │ API Role Wrapping (I3)                             │  │
│  │ • Result wrapped as role:"tool" message            │  │
│  │ • No content-level delimiters                      │  │
│  └──────────────────────┬─────────────────────────────┘  │
│                         │                                 │
│  ┌──────────────────────▼─────────────────────────────┐  │
│  │ Size Check + llm_summarize                         │  │
│  │ • If > threshold: secondary LLM call to summarize  │  │
│  │ • Reduces untrusted content volume in context      │  │
│  └──────────────────────┬─────────────────────────────┘  │
│                         │                                 │
│                         ▼                                 │
│              Conversation Messages                        │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │ Output Validation (I5)                             │  │
│  │ • JSON Schema validation                           │  │
│  │ • Workflow ID allowlist                             │  │
│  │ • Parameter bounds checking                        │  │
│  │ • Self-correction loop (3 attempts)                │  │
│  └────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

### Layer Details

#### I1: Tool-Output Content Sanitization

**Implementation**: `pkg/kubernautagent/sanitization/injection.go`

```go
type InjectionSanitizer struct {
    patterns []*regexp.Regexp
    logger   *slog.Logger
}

func (s *InjectionSanitizer) Sanitize(toolName string, content string) (string, int) {
    // Returns sanitized content and count of patterns matched
    // Each match is logged as an audit event
}
```

**Pattern categories**:

| Category | Example Patterns | Action |
|---|---|---|
| Imperative instructions | `(?i)ignore (all\|previous\|above) instructions` | Strip matching line |
| Role impersonation | `(?i)(system|assistant|user):` at line start | Strip line prefix |
| Workflow selection injection | `(?i)select workflow\|choose workflow\|use workflow` | Strip matching segment |
| JSON response mimicry | `\{"workflow_id":\|"confidence":\|"needs_human_review":` | Strip JSON block |
| Closing tag injection | `</tool_result>\|</system>\|</function>` | Strip tag |
| Prompt escape sequences | `\n\n---\n\n\|====\|####` (boundary markers) | Strip sequence |

**Configurable**: Patterns loaded from configuration, can be extended without code changes.

**False-positive handling**: Patterns are conservative (require specific instruction keywords, not just common words). If a legitimate log contains "ignore previous" as part of normal application output, the stripped version still preserves the surrounding context for RCA.

#### G4: Credential Scrubbing

**Implementation**: `pkg/kubernautagent/sanitization/credential.go`

Reimplements all 17 BR-HAPI-211 / DD-005 pattern categories in Go. Same regex patterns, same ordering (broad container patterns first, then specific).

#### I3: API Role Separation

**Implementation**: In `pkg/kubernautagent/llm/types.go` — tool results are always wrapped as:

```go
Message{
    Role:    "tool",
    Content: sanitizedResult,
    ToolCallID: callID,
}
```

No content-level XML/markdown delimiters are used. The LLM provider's API enforces the role boundary structurally.

#### I4: Per-Phase Tool Scoping

**Implementation**: `internal/kubernautagent/investigator/phases.go`

```go
type Phase int

const (
    PhaseRCA              Phase = iota  // K8s tools + Prometheus tools
    PhaseWorkflowDiscovery              // workflow discovery tools only
    PhaseValidation                     // no tools (structured output)
)

var phaseTools = map[Phase][]string{
    PhaseRCA: {
        "kubectl_describe", "kubectl_get_by_name", "kubectl_get_by_kind_in_namespace",
        "kubectl_events", "kubectl_logs", "kubectl_previous_logs",
        "kubectl_logs_all_containers", "kubectl_container_logs",
        "kubectl_container_previous_logs", "kubectl_previous_logs_all_containers",
        "kubectl_logs_grep",
        "execute_prometheus_instant_query", "execute_prometheus_range_query",
        "get_metric_names", "get_label_values", "get_all_labels", "get_metric_metadata",
        "get_resource_context",
    },
    PhaseWorkflowDiscovery: {
        "list_available_actions", "list_workflows", "get_workflow",
    },
    PhaseValidation: {},  // no tools — LLM must produce structured result
}
```

Phase transitions are controlled by the investigator loop based on conversation state (e.g., when `get_resource_context` has been called and RCA evidence is sufficient, transition to workflow discovery).

#### I5: Output Validation Hardening

**Implementation**: `internal/kubernautagent/result/validator.go`

| Validation | Description |
|---|---|
| JSON Schema | LLM response must match `InvestigationResult` schema |
| Workflow ID allowlist | Only workflow IDs returned by `list_workflows` in the current session are valid |
| Parameter bounds | Numeric parameters within schema-defined ranges, string lengths bounded |
| Self-correction | Up to 3 validation attempts. On failure, flag for human review (BR-HAPI-197). |

#### I7: Behavioral Anomaly Detection

**Implementation**: `internal/kubernautagent/investigator/anomaly.go`

```go
type AnomalyDetector struct {
    maxToolCallsPerTool int           // default: 10 (raised from 5 per #860 for pagination)
    maxTotalToolCalls   int           // default: 30
    maxRepeatedFailures int           // default: 3
    suspiciousPatterns  []*regexp.Regexp
    toolCallCounts      map[string]int
    failureTracker      map[string]int
}

func (d *AnomalyDetector) CheckToolCall(name string, args json.RawMessage) (AnomalyResult, error)
func (d *AnomalyDetector) RecordFailure(name string, args json.RawMessage) (AnomalyResult, error)
```

| Anomaly | Default Threshold | Response |
|---|---|---|
| Per-tool call limit exceeded | 10 calls per tool (pagination-exempt, #860) | Abort, flag human review |
| Total tool calls exceeded | 30 calls per investigation | Abort, flag human review |
| Repeated identical failures | 3 same-args failures | Abort, flag human review |
| Suspicious argument patterns | Regex match | Log warning, optionally reject |

---

## Consequences

### Positive Consequences

1. **Defense in depth**: 6 independent layers — compromising one doesn't bypass the others
2. **Structural boundaries**: API role separation is enforced by the LLM provider, not by content
3. **Least privilege**: Per-phase tool scoping limits blast radius of injection at any stage
4. **Auditable**: Every sanitization event, anomaly detection, and validation failure is logged
5. **Configurable**: Patterns, thresholds, and phase definitions are configuration-driven

### Negative Consequences

1. **Latency overhead**: Sanitization adds processing time per tool call (~1-5ms)
   - **Mitigation**: Regex compilation is done once at startup. Per-call overhead is minimal.
2. **False positives**: Legitimate tool output may contain patterns that match injection rules
   - **Mitigation**: Conservative patterns. Stripping a line preserves surrounding context. Monitor false-positive rate in production.
3. **Not a complete defense**: Sophisticated attacks may bypass pattern-based sanitization
   - **Mitigation**: CaMeL (v1.4) provides architectural defense. v1.3 layers significantly raise the attack bar.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Novel injection bypasses all patterns | Medium | High | CaMeL in v1.4; output validation (I5) as last line of defense |
| False positives break investigation | Low | Medium | Conservative patterns, monitoring, configurable |
| Anomaly thresholds too aggressive | Low | Medium | Configurable per-deployment |

---

## v1.4 Evolution Path

The v1.3 architecture is designed to make CaMeL integration straightforward:

1. **`llm.Client` interface already supports multiple instances** — create a privileged and quarantined client
2. **Investigator loop is Kubernaut-owned** — can route different conversation segments to different LLM instances
3. **Tool scoping is phase-based** — CaMeL's "quarantined LLM cannot call tools" maps to an empty tool set

---

## Validation Strategy

1. **Unit tests**: Each sanitization pattern tested with known injection payloads
2. **Integration tests**: Full investigation with injected payloads in container logs and Prometheus labels
3. **False-positive tests**: Legitimate tool output (real kubectl/Prometheus responses) must pass through unmodified
4. **Anomaly detection tests**: Verify abort behavior at threshold boundaries
5. **Pen testing**: Manual injection attempts against mock-llm-connected Kubernaut Agent

---

## References

- [BR-HAPI-433-004: Security Requirements](../../../requirements/BR-HAPI-433-go-language-migration/BR-HAPI-433-004-security-requirements.md)
- [BR-HAPI-211: LLM Input Sanitization](../../../requirements/BR-HAPI-211-llm-input-sanitization.md)
- [DD-HAPI-005: LLM Input Sanitization](../DD-HAPI-005-llm-input-sanitization.md)

---

**Document Version**: 1.1
**Last Updated**: 2026-03-04
