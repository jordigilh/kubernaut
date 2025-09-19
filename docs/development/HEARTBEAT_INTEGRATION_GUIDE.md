# Heartbeat Monitoring Integration Guide

**Document Version**: 1.0
**Date**: September 2025
**Status**: Implementation Guide
**Purpose**: Step-by-step guide for integrating heartbeat monitoring into Context API server

---

## 🎯 **Integration Overview**

This guide shows how to integrate the heartbeat monitoring system into your existing Context API server. The implementation follows the enterprise architecture recommendations and provides seamless failover capabilities.

---

## 📋 **Prerequisites**

1. ✅ **LLM Client**: Working 20B+ model LLM client
2. ✅ **Context API Server**: Existing Context API server framework
3. ✅ **Configuration**: Environment variables or config files
4. ✅ **Monitoring**: Prometheus metrics collection (optional)

---

## 🔧 **Step-by-Step Integration**

### **Step 1: Import the Monitoring Package**

```go
import (
    "github.com/jordigilh/kubernaut/pkg/ai/monitoring"
    "github.com/jordigilh/kubernaut/pkg/ai/llm"
)
```

### **Step 2: Add Heartbeat Monitoring to Context API Server**

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/jordigilh/kubernaut/pkg/ai/llm"
    "github.com/jordigilh/kubernaut/pkg/ai/monitoring"
    "github.com/sirupsen/logrus"
)

type ContextAPIServer struct {
    logger              *logrus.Logger
    llmClient          llm.Client
    heartbeatIntegration *monitoring.ContextAPIIntegration
}

func NewContextAPIServer() (*ContextAPIServer, error) {
    logger := logrus.New()

    // Create LLM client (your existing client)
    llmClient, err := llm.NewClient(yourLLMConfig, logger)
    if err != nil {
        return nil, err
    }

    // Create monitoring factory
    factory := monitoring.NewMonitoringFactory(logger)

    // Create heartbeat integration
    heartbeatIntegration, err := factory.CreateContextAPIIntegration(llmClient)
    if err != nil {
        return nil, err
    }

    return &ContextAPIServer{
        logger:               logger,
        llmClient:           llmClient,
        heartbeatIntegration: heartbeatIntegration,
    }, nil
}

func (s *ContextAPIServer) Start() error {
    ctx := context.Background()

    // Start heartbeat monitoring
    if err := s.heartbeatIntegration.Start(ctx); err != nil {
        return err
    }

    s.logger.Info("Context API server with heartbeat monitoring started")
    return nil
}

func (s *ContextAPIServer) Stop() error {
    return s.heartbeatIntegration.Stop()
}

func main() {
    server, err := NewContextAPIServer()
    if err != nil {
        log.Fatal(err)
    }

    if err := server.Start(); err != nil {
        log.Fatal(err)
    }

    // Wait for shutdown signal
    c := make(chan os.Signal, 1)
    signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
    <-c

    server.Stop()
}
```

### **Step 3: Configuration Setup**

Add these environment variables to your deployment:

```bash
# Heartbeat monitoring configuration
export HEARTBEAT_ENABLED=true
export HEARTBEAT_CHECK_INTERVAL=30s
export HEARTBEAT_FAILURE_THRESHOLD=3
export HEARTBEAT_HEALTHY_THRESHOLD=2
export HEARTBEAT_TIMEOUT=10s
export HEARTBEAT_HEALTH_PROMPT="System health check. Respond with: HEALTHY"

# Failover configuration
export FAILOVER_ENABLED=true
export FAILOVER_MODE=rule_based
export FAILOVER_CONFIDENCE_REDUCTION=0.2
export FAILOVER_PROCESS_LOWER_CONFIDENCE=true
export FAILOVER_MAX_DURATION=30m

# Context API configuration
export CONTEXT_API_HOST=0.0.0.0
export CONTEXT_API_PORT=8091
export CONTEXT_API_ENABLED=true
export CONTEXT_API_METRICS_ENABLED=true
```

### **Step 4: Optional - Custom Configuration Integration**

If you want to use YAML configuration instead of environment variables:

```go
// Custom configuration loading
func (s *ContextAPIServer) loadCustomConfig() error {
    // Load your YAML config
    config := loadYourConfig()

    // Create configurations manually
    heartbeatConfig := monitoring.HeartbeatConfig{
        Enabled:          config.Heartbeat.Enabled,
        CheckInterval:    config.Heartbeat.CheckInterval,
        FailureThreshold: config.Heartbeat.FailureThreshold,
        // ... other settings
    }

    failoverConfig := monitoring.FailoverConfig{
        Enabled:                    config.Failover.Enabled,
        Mode:                       config.Failover.Mode,
        ConfidenceReduction:        config.Failover.ConfidenceReduction,
        ProcessWithLowerConfidence: config.Failover.ProcessLowerConfidence,
        // ... other settings
    }

    apiConfig := monitoring.ContextAPIConfig{
        Host:    config.API.Host,
        Port:    config.API.Port,
        Enabled: config.API.Enabled,
        // ... other settings
    }

    // Create integration with custom config
    heartbeatIntegration, err := monitoring.NewContextAPIIntegration(
        s.llmClient, heartbeatConfig, failoverConfig, apiConfig, s.logger)
    if err != nil {
        return err
    }

    s.heartbeatIntegration = heartbeatIntegration
    return nil
}
```

---

## 🔍 **Testing the Integration**

### **Test Endpoints**

Once integrated, you can test the heartbeat monitoring using these endpoints:

```bash
# Basic health check
curl http://localhost:8091/health

# Detailed status
curl http://localhost:8091/status

# Heartbeat specific status
curl http://localhost:8091/heartbeat/status

# Configuration status
curl http://localhost:8091/heartbeat/config

# Manual health check trigger
curl -X POST http://localhost:8091/heartbeat/trigger

# Get current mode
curl http://localhost:8091/config/mode

# Manual failover (testing only)
curl -X POST http://localhost:8091/config/failover

# Manual recovery (testing only)
curl -X POST http://localhost:8091/config/recovery

# Force mode (testing only)
curl -X POST http://localhost:8091/config/force/fallback
curl -X POST http://localhost:8091/config/force/primary
```

### **Expected Responses**

**Health Check (Healthy):**
```json
{
  "status": "ok",
  "timestamp": "2025-09-15T13:30:00Z",
  "service": "context-api-heartbeat",
  "healthy": true,
  "mode": "primary"
}
```

**Status Endpoint:**
```json
{
  "timestamp": "2025-09-15T13:30:00Z",
  "service": "context-api-heartbeat",
  "heartbeat": {
    "is_healthy": true,
    "current_mode": "primary",
    "consecutive_failures": 0,
    "consecutive_successes": 5,
    "last_health_check": "2025-09-15T13:29:30Z",
    "model_available": true
  },
  "config": {
    "current_mode": "primary",
    "failover_count": 0,
    "in_fallback_mode": false,
    "fallback_duration": "0s"
  }
}
```

---

## 📊 **Monitoring and Alerting**

### **Prometheus Metrics**

The integration automatically exposes these metrics:

```
# Health check metrics
kubernaut_heartbeat_health_checks_total{status="success|failure"}
kubernaut_heartbeat_health_check_duration_seconds{status="success|failure"}

# Failover metrics
kubernaut_heartbeat_failover_events_total{type="triggered|recovered"}
kubernaut_heartbeat_current_mode{mode="primary|fallback"}

# Availability metrics
kubernaut_heartbeat_model_available
kubernaut_heartbeat_consecutive_failures
kubernaut_heartbeat_last_success_timestamp
```

### **Sample Prometheus Alerts**

```yaml
groups:
  - name: kubernaut-heartbeat
    rules:
      - alert: KubernautModelUnavailable
        expr: kubernaut_heartbeat_model_available == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "20B+ model is unavailable"
          description: "The 20B+ model has been unavailable for {{ $value }} seconds"

      - alert: KubernautInFallbackMode
        expr: kubernaut_heartbeat_current_mode{mode="fallback"} == 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Kubernaut is running in fallback mode"
          description: "System has been in rule-based fallback mode for {{ $value }} seconds"

      - alert: KubernautConsecutiveFailures
        expr: kubernaut_heartbeat_consecutive_failures >= 3
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Multiple consecutive health check failures"
          description: "{{ $value }} consecutive health check failures detected"
```

---

## 🔧 **Advanced Configuration**

### **Custom Health Check Logic**

You can register custom configuration callbacks for advanced scenarios:

```go
// Register a callback for configuration changes
s.heartbeatIntegration.RegisterConfigCallback(func(mode string, config monitoring.FailoverConfig) error {
    s.logger.WithField("mode", mode).Info("Configuration mode changed")

    // Your custom logic here
    if mode == "fallback" {
        // Notify external systems
        // Adjust other service configurations
        // Send alerts
    }

    return nil
})
```

### **Integration with Existing Services**

```go
// Get real-time status for integration with other services
status := s.heartbeatIntegration.GetHeartbeatStatus()
if !status.ModelAvailable {
    // Use alternative processing logic
    // Adjust service behavior
    // Update UI indicators
}

configStatus := s.heartbeatIntegration.GetConfigStatus()
if configStatus.InFallbackMode {
    // Log fallback duration
    // Send notifications
    // Adjust confidence thresholds
}
```

---

## 🚨 **Troubleshooting**

### **Common Issues**

1. **Port Conflicts**: Ensure port 8091 is available
   ```bash
   lsof -i :8091
   ```

2. **LLM Connection Issues**: Check endpoint connectivity
   ```bash
   curl http://192.168.1.169:8080/api/version
   ```

3. **Permission Issues**: Ensure environment variable access
   ```bash
   env | grep -E "(HEARTBEAT|FAILOVER|CONTEXT_API)"
   ```

### **Debug Mode**

Enable debug logging:
```go
logger.SetLevel(logrus.DebugLevel)
```

Check logs for:
- Health check requests and responses
- Failover triggers and configuration updates
- HTTP endpoint access and responses

---

## ✅ **Validation Checklist**

- [ ] Heartbeat monitoring starts successfully
- [ ] Health checks execute every 30 seconds
- [ ] Failover triggers after 3 consecutive failures
- [ ] Recovery triggers after 2 consecutive successes
- [ ] HTTP endpoints respond correctly
- [ ] Prometheus metrics are exposed
- [ ] Configuration callbacks work
- [ ] Graceful shutdown completes

---

## 🎯 **Next Steps**

1. **Deploy to Development**: Test with your 20B model
2. **Configure Monitoring**: Set up Prometheus alerts
3. **Test Failover Scenarios**: Simulate model failures
4. **Production Deployment**: Deploy with monitoring
5. **Anthropic Integration**: Prepare for Claude-4-Sonnet testing

This integration provides enterprise-grade reliability for your 20B+ model deployment with automatic failover and comprehensive monitoring capabilities.
