# AI Integration Usage Example

## 🚀 Quick Start with Your LLM (192.168.1.169:8080)

### 1. Environment Setup
```bash
# Set your LLM endpoint
export SLM_ENDPOINT=http://192.168.1.169:8080
export SLM_PROVIDER=localai
export SLM_MODEL=granite3.1-dense:8b

# Run the application
./prometheus-alerts-slm
```

**Expected Output:**
```
INFO SLM client initialized provider=localai endpoint=http://192.168.1.169:8080
INFO Detecting AI service availability for intelligent workflow integration
INFO ✅ LLM integration validated successfully - AI features enabled
INFO Creating real AI metrics collector with LLM integration
INFO Creating real learning-enhanced prompt builder with LLM integration
```

### 2. Application Integration Code Example

```go
// Example: How to use the new AI integration in your application

package main

import (
    "context"
    "log"

    "github.com/jordigilh/kubernaut/internal/config"
    "github.com/jordigilh/kubernaut/pkg/ai/llm"
    "github.com/jordigilh/kubernaut/pkg/workflow/engine"
)

func main() {
    // Load configuration (will use your LLM endpoint from environment)
    cfg, err := config.Load("config/local-llm.yaml")
    if err != nil {
        log.Fatal(err)
    }

    // Create LLM client
    llmClient, err := llm.NewClient(cfg.SLM, logger)
    if err != nil {
        log.Printf("LLM client creation failed: %v", err)
        // Application continues with statistical fallbacks
    }

    // Create AI-integrated components (automatically detects your LLM)
    aiMetrics := engine.NewConfiguredAIMetricsCollector(
        cfg,
        llmClient,
        nil, // Vector DB (Phase 1C)
        nil, // Metrics client
        logger,
    )

    promptBuilder := engine.NewConfiguredLearningEnhancedPromptBuilder(
        cfg,
        llmClient,
        nil, // Vector DB
        executionRepo,
        logger,
    )

    // Components will automatically:
    // ✅ Use LLM when available (192.168.1.169:8080)
    // ✅ Fall back to statistical analysis when not
    // ✅ Log service status and connectivity

    log.Println("Application started with AI integration!")
}
```

### 3. Feature Behavior

#### With Your LLM Available:
```bash
# AI-powered features active
INFO [AnalyzePatternCorrelations] Analyzing pattern correlations using AI
INFO [GenerateAnomalyInsights] Generating AI-powered anomaly insights
INFO [PredictEffectivenessTrends] Predicting effectiveness trends using AI
```

#### Without LLM (Network/Service Issues):
```bash
# Statistical fallbacks active
WARN LLM service at http://192.168.1.169:8080 is not healthy
INFO Using statistical correlation analysis for pattern detection
INFO Using enhanced rule-based insight generation
```

### 4. Testing the Integration

```bash
# Test LLM connectivity manually
curl -X POST http://192.168.1.169:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-oss:20b",
    "messages": [{"role": "user", "content": "Test"}],
    "temperature": 0.3,
    "max_tokens": 10
  }'

# Should return JSON response with completion
```

### 5. Configuration Options

#### Option A: Environment Variables (Recommended)
```bash
export SLM_ENDPOINT=http://192.168.1.169:8080
export SLM_PROVIDER=localai
export SLM_MODEL=granite3.1-dense:8b
```

#### Option B: YAML Configuration
```yaml
# config/local-llm.yaml
slm:
  endpoint: "http://192.168.1.169:8080"
  provider: "localai"
  model: "gpt-oss:20b"
  timeout: 30s
```

#### Option C: Runtime Configuration
```go
cfg := &config.Config{
    SLM: config.LLMConfig{
        Endpoint: "http://192.168.1.169:8080",
        Provider: "localai",
        Model:    "granite3.1-dense:8b",
        Timeout:  30 * time.Second,
    },
}
```

## 🎯 Business Value Delivered

✅ **Intelligent Operation**: Automatically uses your LLM when available
✅ **Reliable Fallbacks**: Never fails due to LLM unavailability
✅ **Zero Configuration**: Works with environment variables
✅ **Health Monitoring**: Continuous service status monitoring
✅ **Performance Optimized**: 10-second timeout for service detection

The system is now **production-ready** with intelligent AI integration that enhances capabilities without creating dependencies!
