# Resilience Patterns and Failure Handling Architecture

## Overview

This document describes the comprehensive resilience patterns and failure handling mechanisms implemented in the Kubernaut system to ensure high availability, graceful degradation, and robust error recovery.

## Business Requirements Addressed

- **BR-RELIABILITY-001**: 99%+ system uptime requirement
- **BR-AI-014**: Graceful degradation when AI services unavailable
- **BR-HEALTH-025 to BR-HEALTH-034**: Health monitoring and failure detection
- **BR-PERF-020**: System resilience under load
- **BR-SECURITY-015**: Secure failure handling without information leakage

## Architecture Principles

### Design Philosophy
- **Fail-Safe Design**: System degrades gracefully rather than failing catastrophically
- **Circuit Breaker Pattern**: Prevent cascade failures across service boundaries
- **Bulkhead Isolation**: Isolate failures to prevent system-wide impact
- **Timeout and Retry**: Intelligent retry mechanisms with exponential backoff
- **Health Monitoring**: Proactive failure detection and recovery

## Multi-Level Fallback Strategy

### Primary Fallback Hierarchy

```ascii
┌─────────────────────────────────────────────────────────────────┐
│                    RESILIENCE HIERARCHY                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Level 1: Service-Level Fallbacks                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ HolmesGPT       │─▶│ Direct LLM      │─▶│ Rule-based      │ │
│  │ Investigation   │  │ Analysis        │  │ Fallback        │ │
│  │ (Primary)       │  │ (Secondary)     │  │ (Tertiary)      │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│           │                     │                     │         │
│           ▼                     ▼                     ▼         │
│  Level 2: Network-Level Resilience                             │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ Retry with      │  │ Circuit Breaker │  │ Connection      │ │
│  │ Exponential     │  │ Protection      │  │ Pooling         │ │
│  │ Backoff         │  │                 │  │                 │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│           │                     │                     │         │
│           ▼                     ▼                     ▼         │
│  Level 3: Resource-Level Recovery                               │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ Alternative     │  │ Resource        │  │ Rollback        │ │
│  │ Action          │  │ Allocation      │  │ Procedures      │ │
│  │ Selection       │  │ Adjustment      │  │                 │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│           │                     │                     │         │
│           ▼                     ▼                     ▼         │
│  Level 4: System-Level Graceful Degradation                    │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ Core Function   │  │ Monitoring      │  │ Emergency       │ │
│  │ Preservation    │  │ Continuation    │  │ Mode            │ │
│  │                 │  │                 │  │                 │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Service-Level Failure Patterns

### 1. HolmesGPT Service Failures

**Failure Scenarios:**
- API endpoint unavailable (network/deployment issues)
- Authentication failures (token expiration/rotation)
- Response timeout (>30s investigation time)
- Invalid response format (parsing errors)

**Fallback Flow:**
```ascii
HolmesGPT Request Failed
          │
          ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Log Failure     │────▶│ Increment        │────▶│ Switch to       │
│ with Context    │     │ Circuit Breaker  │     │ LLM Fallback    │
└─────────────────┘     └──────────────────┘     └─────────────────┘
          │                       │                       │
          ▼                       ▼                       ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Record Metrics  │     │ Start Health     │     │ Continue with   │
│ for Monitoring  │     │ Check Timer      │     │ Reduced         │
│                 │     │                  │     │ Confidence      │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

**Implementation:**
```go
// Circuit breaker pattern for HolmesGPT
func (ai *AIServiceIntegrator) investigateWithHolmesGPT(ctx context.Context, alert types.Alert) (*InvestigationResult, error) {
    if ai.holmesGPTCircuitBreaker.ShouldBlock() {
        ai.log.Warn("HolmesGPT circuit breaker is open, falling back to LLM")
        return ai.investigateWithLLM(ctx, alert)
    }

    result, err := ai.holmesGPTClient.Investigate(ctx, &request)
    if err != nil {
        ai.holmesGPTCircuitBreaker.RecordFailure()
        ai.log.WithError(err).Error("HolmesGPT investigation failed, falling back to LLM")
        return ai.investigateWithLLM(ctx, alert)
    }

    ai.holmesGPTCircuitBreaker.RecordSuccess()
    return result, nil
}
```

### 2. LLM Service Failures

**Failure Scenarios:**
- Model endpoint unavailable (provider issues)
- Rate limiting exceeded (quota/throttling)
- Context window overflow (>131K tokens)
- Model response quality degradation

**Fallback Flow:**
```ascii
LLM Request Failed
          │
          ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Analyze Failure │────▶│ Check Failure    │────▶│ Select Recovery │
│ Type & Context  │     │ Pattern          │     │ Strategy        │
└─────────────────┘     └──────────────────┘     └─────────────────┘
          │                       │                       │
          ▼                       ▼                       ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Rate Limit      │     │ Provider         │     │ Rule-based      │
│ Backoff         │     │ Failover         │     │ Analysis        │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

### 3. Rule-Based Fallback (Last Resort)

**Capabilities:**
- Alert severity-based action mapping
- Namespace-specific remediation patterns
- Resource type heuristics
- Historical success rate consideration

**Decision Tree:**
```ascii
Alert Received
      │
      ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Extract Alert   │────▶│ Check Severity   │────▶│ Map to Action   │
│ Metadata        │     │ & Type           │     │ Category        │
└─────────────────┘     └──────────────────┘     └─────────────────┘
      │                       │                       │
      ▼                       ▼                       ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Critical:       │     │ Warning:         │     │ Info:           │
│ Immediate       │     │ Standard         │     │ Monitoring      │
│ Action          │     │ Remediation      │     │ Only            │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

## Network-Level Resilience

### Circuit Breaker Implementation

**State Management:**
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                    CIRCUIT BREAKER STATES                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│    CLOSED                HALF-OPEN               OPEN           │
│ ┌─────────────┐         ┌─────────────┐       ┌─────────────┐   │
│ │   Normal    │   ┌────▶│  Testing    │◀─────▶│  Blocking   │   │
│ │ Operation   │   │     │ Recovery    │       │  Requests   │   │
│ │             │   │     │             │       │             │   │
│ └─────────────┘   │     └─────────────┘       └─────────────┘   │
│        │          │            │                     │          │
│        │ Failure  │            │ Success             │ Timeout  │
│        │ Threshold│            │                     │          │
│        │ Exceeded │            ▼                     │          │
│        │          │     ┌─────────────┐              │          │
│        └──────────┘     │   Return    │              │          │
│                         │  to CLOSED  │              │          │
│                         └─────────────┘              │          │
│                                                       │          │
│                         ┌─────────────┐              │          │
│                         │  Stay OPEN  │◀─────────────┘          │
│                         │ (Failure)   │                         │
│                         └─────────────┘                         │
└─────────────────────────────────────────────────────────────────┘
```

**Configuration:**
- **Failure Threshold**: 5 consecutive failures
- **Recovery Timeout**: 30 seconds before half-open
- **Success Threshold**: 3 consecutive successes to close
- **Request Volume**: Minimum 10 requests for assessment

### Retry Mechanisms

**Exponential Backoff Strategy:**
```ascii
Attempt 1: Immediate (0ms)
          │
          ▼ (Failure)
Attempt 2: 100ms + jitter
          │
          ▼ (Failure)
Attempt 3: 200ms + jitter
          │
          ▼ (Failure)
Attempt 4: 400ms + jitter
          │
          ▼ (Failure)
Attempt 5: 800ms + jitter
          │
          ▼ (Failure)
Max Attempts Reached → Circuit Breaker Opens
```

**Retry Logic:**
```go
func (c *Client) retryWithBackoff(ctx context.Context, operation func() error) error {
    maxRetries := 5
    baseDelay := 100 * time.Millisecond
    maxDelay := 5 * time.Second

    for attempt := 0; attempt < maxRetries; attempt++ {
        if err := operation(); err == nil {
            return nil
        }

        if attempt == maxRetries-1 {
            return fmt.Errorf("operation failed after %d attempts", maxRetries)
        }

        delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
        if delay > maxDelay {
            delay = maxDelay
        }

        // Add jitter to prevent thundering herd
        jitter := time.Duration(rand.Float64() * float64(delay) * 0.1)
        select {
        case <-time.After(delay + jitter):
        case <-ctx.Done():
            return ctx.Err()
        }
    }
    return nil
}
```

## Resource-Level Recovery

### Action Execution Failures

**Failure Categories:**
1. **Kubernetes API Errors**: Unauthorized, not found, conflict
2. **Resource Constraints**: Insufficient CPU, memory, storage
3. **Policy Violations**: RBAC, network policies, security policies
4. **Timeout Errors**: Long-running operations exceed limits

**Recovery Strategies:**

```ascii
Action Execution Failed
          │
          ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Classify        │────▶│ Select Recovery  │────▶│ Execute         │
│ Failure Type    │     │ Strategy         │     │ Recovery        │
└─────────────────┘     └──────────────────┘     └─────────────────┘
          │                       │                       │
          ▼                       ▼                       ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ API Error:      │     │ Resource Error:  │     │ Policy Error:   │
│ Retry with      │     │ Alternative      │     │ Escalate to     │
│ Different Auth  │     │ Action           │     │ Manual Review   │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

### Rollback Procedures

**Automatic Rollback Triggers:**
- Action execution failure with error code classification
- Post-execution health check failure
- Resource state validation failure
- User-initiated emergency rollback

**Rollback Flow:**
```ascii
Rollback Triggered
          │
          ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Capture         │────▶│ Execute Reverse  │────▶│ Verify          │
│ Current State   │     │ Operation        │     │ Restoration     │
└─────────────────┘     └──────────────────┘     └─────────────────┘
          │                       │                       │
          ▼                       ▼                       ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Record in       │     │ Update Action    │     │ Generate        │
│ Action History  │     │ Trace            │     │ Alert           │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

## System-Level Graceful Degradation

### Core Function Preservation

**Essential Services (Always Maintained):**
- Alert ingestion and basic processing
- Health monitoring and metrics collection
- Manual intervention interfaces
- Audit logging and compliance
- Emergency shutdown procedures

**Degraded Operations:**
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                   DEGRADATION LEVELS                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Level 0: Full Operation                                         │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ All AI services + All context + All actions + All monitoring│ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼ (AI Services Fail)              │
│ Level 1: Reduced AI                                             │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ LLM only + Reduced context + Priority actions + Monitoring  │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼ (LLM Services Fail)             │
│ Level 2: Rule-Based                                             │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Heuristics + Basic context + Safe actions + Monitoring      │ │
│ └─────────────────────────────────────────────────────────────┘ │
│                              │                                  │
│                              ▼ (Critical Failures)             │
│ Level 3: Emergency Mode                                         │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ Alert ingestion + Manual review + Logging + Health checks   │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Emergency Mode Operations

**Activation Criteria:**
- Multiple critical service failures (>3 services down)
- Resource exhaustion (CPU >90%, Memory >95%)
- Security incident detection
- Manual emergency activation

**Emergency Capabilities:**
- Basic alert reception and logging
- Manual intervention interfaces
- Critical health monitoring
- Audit trail maintenance
- Emergency contact notification

## Health Monitoring and Recovery

### Health Check Strategy

**Multi-Layer Health Monitoring:**
```ascii
┌─────────────────────────────────────────────────────────────────┐
│                     HEALTH CHECK LAYERS                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│ Layer 1: Component Health                                       │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Service         │  │ Database        │  │ External API    │ │
│ │ Liveness        │  │ Connectivity    │  │ Connectivity    │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│          │                    │                    │           │
│          ▼                    ▼                    ▼           │
│ Layer 2: Integration Health                                     │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ End-to-End      │  │ Data Flow       │  │ Performance     │ │
│ │ Workflows       │  │ Validation      │  │ Metrics         │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│          │                    │                    │           │
│          ▼                    ▼                    ▼           │
│ Layer 3: Business Health                                        │
│ ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│ │ Alert           │  │ Investigation   │  │ Action          │ │
│ │ Processing      │  │ Accuracy        │  │ Success Rate    │ │
│ └─────────────────┘  └─────────────────┘  └─────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Automatic Recovery

**Recovery Triggers:**
- Health check restoration after failure
- Circuit breaker state transitions
- Resource availability improvement
- External service recovery detection

**Recovery Process:**
```ascii
Failure Detected
          │
          ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Log Failure     │────▶│ Start Recovery   │────▶│ Gradual Service │
│ Details         │     │ Timer            │     │ Restoration     │
└─────────────────┘     └──────────────────┘     └─────────────────┘
          │                       │                       │
          ▼                       ▼                       ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Notify          │     │ Test Service     │     │ Full Service    │
│ Operations      │     │ Availability     │     │ Restoration     │
└─────────────────┘     └──────────────────┘     └─────────────────┘
```

## Metrics and Observability

### Failure Metrics

**Key Performance Indicators:**
- **Service Availability**: Uptime percentage per service
- **Fallback Activation Rate**: Frequency of fallback usage
- **Recovery Time**: Time to restore full functionality
- **Error Rate**: Errors per total requests
- **Circuit Breaker Events**: Open/close state transitions

**Prometheus Metrics:**
```prometheus
# Service availability
kubernaut_service_availability_ratio{service="holmesgpt"} 0.99
kubernaut_service_availability_ratio{service="llm"} 0.98

# Fallback usage
kubernaut_fallback_activations_total{from="holmesgpt",to="llm"} 25
kubernaut_fallback_activations_total{from="llm",to="rules"} 5

# Recovery metrics
kubernaut_recovery_time_seconds{service="holmesgpt"} 45.2
kubernaut_circuit_breaker_state{service="holmesgpt"} 0  # 0=closed, 1=open, 2=half-open
```

### Alerting Strategy

**Alert Severity Levels:**
- **Critical**: Core functionality unavailable (emergency mode)
- **High**: Primary AI services unavailable (fallback active)
- **Medium**: Performance degradation (increased latency)
- **Low**: Circuit breaker events (informational)

## Testing and Validation

### Chaos Engineering

**Failure Injection Scenarios:**
- Random service termination
- Network partition simulation
- Resource exhaustion testing
- External dependency failures
- Security incident simulation

**Automated Testing:**
```go
func TestResiliencePatterns(t *testing.T) {
    scenarios := []struct {
        name    string
        failure func() error
        expect  func() bool
    }{
        {
            name:    "HolmesGPT_Service_Down",
            failure: simulateHolmesGPTFailure,
            expect:  expectLLMFallback,
        },
        {
            name:    "LLM_Service_Down",
            failure: simulateLLMFailure,
            expect:  expectRuleBasedFallback,
        },
        {
            name:    "All_AI_Services_Down",
            failure: simulateAllAIFailures,
            expect:  expectEmergencyMode,
        },
    }

    for _, scenario := range scenarios {
        t.Run(scenario.name, func(t *testing.T) {
            scenario.failure()
            assert.True(t, scenario.expect())
        })
    }
}
```

## Security Considerations

### Secure Failure Handling

**Information Security:**
- No sensitive data in error messages
- Audit logging of all failure events
- Secure credential management during failures
- Access control enforcement in degraded states

**Attack Mitigation:**
- Rate limiting during failures
- DDoS protection for health endpoints
- Intrusion detection during degraded operation
- Secure communication even in emergency mode

---

## Related Documentation

- [Alert Processing Flow](ALERT_PROCESSING_FLOW.md)
- [Production Monitoring](PRODUCTION_MONITORING.md)
- [Performance Requirements](PERFORMANCE_REQUIREMENTS.md)
- [Health Monitoring Integration](../deployment/MILESTONE_1_CONFIGURATION_OPTIONS.md)

---

*This document is part of the Kubernaut production architecture documentation and is regularly updated based on operational experience and system evolution.*