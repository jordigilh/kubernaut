# Heartbeat Monitoring and Failover System Design

**Document Version**: 1.0
**Date**: September 2025
**Status**: Implementation Plan
**Purpose**: Design heartbeat monitoring for 20B+ model availability with automatic rule-based fallback

---

## 🎯 **System Overview**

The heartbeat monitoring system ensures continuous availability of enterprise AI capabilities by:
1. **Monitoring 20B+ model health** through periodic health checks
2. **Automatic configuration failover** when model becomes unavailable
3. **Seamless rule-based fallback** with lower confidence processing
4. **Self-healing recovery** when model becomes available again

---

## 🏗️ **Architecture Design**

### **Component Architecture**

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Alert Webhook │────│  LLM Client      │────│  20B Model      │
│                 │    │  (Primary)       │    │  (Ollama)       │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                              │                         │
                              │                    Heartbeat
                              │                         │
                       ┌──────▼──────┐           ┌─────▼─────┐
                       │ Heartbeat   │           │ Health    │
                       │ Monitor     │───────────│ Endpoint  │
                       │ Service     │  Periodic │ Checker   │
                       └──────┬──────┘   Check   └───────────┘
                              │
                              ▼
                       ┌──────────────┐
                       │ Config       │
                       │ Manager      │
                       │ (Failover)   │
                       └──────┬───────┘
                              │
                              ▼
                       ┌──────────────┐
                       │ Rule-Based   │
                       │ Fallback     │
                       │ Processor    │
                       └──────────────┘
```

---

## 📋 **Implementation Recommendations**

### **Option 1: Context API Server Integration (RECOMMENDED)**

**✅ Pros:**
- **Centralized monitoring**: Context API server already manages AI orchestration
- **Service isolation**: Heartbeat failures don't affect main webhook processing
- **Distributed architecture**: Health checks from dedicated service
- **Configuration management**: Natural place for failover logic

**Implementation:**
```go
// In Context API Server (port 8091)
type HeartbeatMonitor struct {
    llmClient     *llm.ClientImpl
    configManager *ConfigManager
    checkInterval time.Duration
    failureThreshold int
    healthyThreshold int
}

func (h *HeartbeatMonitor) StartMonitoring() {
    go h.monitoringLoop()
}

func (h *HeartbeatMonitor) monitoringLoop() {
    ticker := time.NewTicker(h.checkInterval) // Every 30 seconds
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            if err := h.checkModelHealth(); err != nil {
                h.handleFailure(err)
            } else {
                h.handleSuccess()
            }
        }
    }
}
```

### **Option 2: HolmesGPT API Integration (ALTERNATIVE)**

**⚠️ Pros:**
- **AI service proximity**: Already handles AI coordination
- **Existing monitoring**: May have health checks built-in

**❌ Cons:**
- **Service coupling**: HolmesGPT failures could affect heartbeat
- **Scope creep**: HolmesGPT focused on investigations, not infrastructure monitoring

---

## 🔧 **Detailed Implementation Plan**

### **1. Health Check Mechanism**

```go
// Health check implementation
func (h *HeartbeatMonitor) checkModelHealth() error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Simple health check prompt
    healthPrompt := "System health check. Respond with: HEALTHY"

    response, err := h.llmClient.ChatCompletion(ctx, healthPrompt)
    if err != nil {
        return fmt.Errorf("health check failed: %w", err)
    }

    if !strings.Contains(strings.ToUpper(response), "HEALTHY") {
        return fmt.Errorf("invalid health response: %s", response)
    }

    return nil
}
```

### **2. Failure Detection and Thresholds**

```yaml
heartbeat:
  check_interval: 30s              # Health check frequency
  failure_threshold: 3             # Consecutive failures before failover
  healthy_threshold: 2             # Consecutive successes before recovery
  timeout: 10s                     # Health check timeout
  health_prompt: "System health check. Respond with: HEALTHY"
```

### **3. Configuration Failover Mechanism**

```go
type ConfigManager struct {
    primaryConfig   *LLMClientConfig
    fallbackConfig  *LLMClientConfig
    currentMode     string // "primary" or "fallback"
    failoverTime    time.Time
}

func (c *ConfigManager) TriggerFailover() error {
    c.logger.Warn("Triggering failover to rule-based processing")

    // Update configuration atomically
    c.currentMode = "fallback"
    c.failoverTime = time.Now()

    // Notify all LLM clients about configuration change
    return c.broadcastConfigUpdate()
}

func (c *ConfigManager) TriggerRecovery() error {
    c.logger.Info("Triggering recovery to 20B+ model processing")

    c.currentMode = "primary"
    c.failoverTime = time.Time{}

    return c.broadcastConfigUpdate()
}
```

### **4. Configuration Update Broadcasting**

**Option A: Environment Variable Updates**
```go
func (c *ConfigManager) broadcastConfigUpdate() error {
    if c.currentMode == "fallback" {
        os.Setenv("LLM_ENABLE_RULE_FALLBACK", "true")
        os.Setenv("LLM_FORCE_FALLBACK", "true")
    } else {
        os.Setenv("LLM_FORCE_FALLBACK", "false")
    }

    // Signal all services to reload configuration
    return c.signalConfigReload()
}
```

**Option B: Shared Configuration Store**
```go
func (c *ConfigManager) broadcastConfigUpdate() error {
    config := map[string]interface{}{
        "mode": c.currentMode,
        "fallback_enabled": c.currentMode == "fallback",
        "updated_at": time.Now(),
    }

    // Update shared configuration (Redis, etcd, or database)
    return c.configStore.Set("llm_config", config)
}
```

---

## 🚨 **Monitoring and Alerting**

### **Metrics Collection**
```go
type HeartbeatMetrics struct {
    HealthCheckTotal     prometheus.CounterVec
    HealthCheckDuration  prometheus.HistogramVec
    FailoverEvents       prometheus.CounterVec
    CurrentMode          prometheus.GaugeVec
    ModelAvailability    prometheus.GaugeVec
}
```

### **Alert Conditions**
1. **Model Unavailable**: 3 consecutive health check failures
2. **Extended Downtime**: Model unavailable > 5 minutes
3. **Failover Activated**: Automatic rule-based fallback triggered
4. **Recovery Completed**: 20B+ model restored and active

### **Log Structure**
```json
{
  "timestamp": "2025-09-15T13:30:00Z",
  "component": "heartbeat_monitor",
  "event": "failover_triggered",
  "model_endpoint": "http://192.168.1.169:8080",
  "failure_count": 3,
  "last_success": "2025-09-15T13:28:30Z",
  "fallback_mode": "rule_based",
  "confidence_reduction": "0.8 -> 0.6 average"
}
```

---

## ⚙️ **Configuration Integration**

### **Updated config/local-llm.yaml**
```yaml
# Heartbeat monitoring configuration
heartbeat:
  enabled: true                    # Enable heartbeat monitoring
  monitor_service: "context_api"   # context_api or holmesgpt_api
  check_interval: "30s"           # Health check frequency
  failure_threshold: 3            # Failures before failover
  healthy_threshold: 2            # Successes before recovery
  timeout: "10s"                  # Health check timeout
  health_endpoint: "/health"      # Health check endpoint

# Failover configuration
failover:
  enabled: true                   # Enable automatic failover
  mode: "rule_based"             # Fallback mode: rule_based
  confidence_reduction: 0.2      # Reduce confidence by 20% in fallback
  notification_webhook: ""       # Optional webhook for failover notifications

# Enterprise 20B+ Model Configuration
llm:
  endpoint: "http://192.168.1.169:8080"
  provider: "ollama"
  model: "ggml-org/gpt-oss-20b-GGUF"
  max_concurrent_alerts: 5        # User specified limit
  enable_rule_fallback: true     # User specified fallback strategy
  min_parameter_count: 20000000000
```

---

## 🎯 **Implementation Timeline**

### **Phase 1: Basic Health Monitoring (1-2 days)**
- ✅ LLM client health check endpoint
- ✅ Basic heartbeat monitoring in Context API server
- ✅ Simple failover mechanism

### **Phase 2: Advanced Failover (2-3 days)**
- ✅ Configuration management system
- ✅ Automatic recovery detection
- ✅ Metrics and alerting integration

### **Phase 3: Production Hardening (1-2 days)**
- ✅ Comprehensive error handling
- ✅ Performance optimization
- ✅ Monitoring dashboard integration

---

## 🔍 **Testing Strategy**

### **Test Scenarios**
1. **Normal Operation**: Health checks pass, 20B model active
2. **Temporary Failure**: Single health check failure, no failover
3. **Model Unavailable**: Multiple failures trigger failover
4. **Network Issues**: Timeout-based failure detection
5. **Recovery**: Model restoration triggers recovery

### **Validation Criteria**
- ✅ Failover completes within 90 seconds of model failure
- ✅ Rule-based processing maintains <5 second response times
- ✅ Recovery restores 20B model processing automatically
- ✅ No alert processing disruption during failover/recovery
- ✅ Confidence scoring accurately reflects processing mode

---

## 💡 **Final Recommendation**

**Implement Option 1 (Context API Server Integration)** for the following reasons:

1. **Architecture Alignment**: Context API server is designed for AI orchestration
2. **Service Isolation**: Heartbeat monitoring separate from core webhook processing
3. **Scalability**: Can easily extend to monitor multiple AI services
4. **Configuration Management**: Natural place for centralized AI configuration
5. **Future-Proof**: Supports planned Anthropic Claude-4-Sonnet integration

**Next Steps:**
1. Start with basic health monitoring in Context API server
2. Implement simple rule-based fallback mechanism
3. Add configuration management and broadcasting
4. Integrate monitoring and alerting
5. Test with controlled failure scenarios

This design provides robust failover capabilities while maintaining enterprise-grade reliability for your 20B+ model deployment.
