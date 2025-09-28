# HolmesGPT Integration Guide

## Overview

This document describes the integration of [HolmesGPT](https://github.com/robusta-dev/holmesgpt) with the kubernaut project. HolmesGPT is a 24/7 On-Call AI Agent that solves alerts faster with automatic correlations, investigations, and more.

## Architecture

The HolmesGPT integration enhances the existing Enhanced Assessor with AI-powered decision-making capabilities while preserving all existing sophisticated analysis and historical data.

### Integration Components

```
┌─────────────────────────────────────────────────────────────────┐
│                    Enhanced Assessor                            │
├─────────────────────────────────────────────────────────────────┤
│  Traditional Assessment + Phase 2 Analytics                    │
│  ├── Pattern Analysis & Vector DB                              │
│  ├── Predictive Analytics Engine                               │
│  ├── Cost Analysis & ROI Calculation                           │
│  ├── Historical Effectiveness Data                             │
│  └── Assessment Metrics & Monitoring                           │
├─────────────────────────────────────────────────────────────────┤
│                 HolmesGPT Integration                           │
│  ├── Context-Enriched Prompt Generation                        │
│  ├── Historical Data Synthesis                                 │
│  ├── AI-Powered Decision Making                                │
│  └── Structured Remediation Recommendations                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      HolmesGPT API                             │
│  ├── 15+ Data Source Integrations                              │
│  ├── Kubernetes & Prometheus Toolsets                          │
│  ├── Investigation & Analysis Engine                            │
│  └── Structured Response Generation                             │
└─────────────────────────────────────────────────────────────────┘
```

## Key Features

### 1. **Context-Enriched Decision Making**
- **Historical Effectiveness Analysis**: Includes effectiveness scores, confidence levels, and learning contributions
- **Similar Pattern Analysis**: Top 3 most relevant historical patterns with success rates and execution counts
- **Predictive Insights**: Model predictions, confidence scores, anomaly detection, and trend analysis
- **Cost-Benefit Analysis**: Estimated costs, expected savings, ROI projections, and alternative action costs
- **System Performance Context**: Assessment metrics, success rates, and processing times
- **Consistent Quality Across AI Services**: Both HolmesGPT and LLM fallback receive identical context enrichment

### 2. **Intelligent Approval Logic**
The system automatically determines when human approval is required based on:
- **Low Confidence**: Decisions with confidence < 0.7
- **High Risk**: Actions marked as high-risk by HolmesGPT
- **Low Historical Effectiveness**: Actions with effectiveness score < 0.6
- **Novel Actions**: No similar historical patterns available

### 3. **Comprehensive Prompt Engineering**
Each HolmesGPT request includes a structured prompt with:
- Current alert details and context
- Historical effectiveness analysis
- Similar pattern analysis with success rates
- Predictive analytics insights
- Cost-benefit analysis
- System performance metrics
- Specific decision requirements

## Configuration

### Enhanced Assessor Configuration

```yaml
# Enable HolmesGPT integration
enable_holmes_gpt: true

# Other existing configuration
enable_pattern_learning: true
enable_predictive_analytics: true
enable_cost_analysis: true
min_similarity_threshold: 0.3
max_stored_patterns: 1000
pattern_retention_days: 90
prediction_model: "similarity"
analytics_update_interval: "1h"
model_retraining_interval: "24h"
async_processing: false
batch_size: 10
worker_pool_size: 5
```

### HolmesGPT Client Configuration

```go
// HTTP Client Configuration
holmesClient := analytics.NewHTTPHolmesClient(
    "https://api.holmesgpt.dev", // HolmesGPT API endpoint
    "your-api-key",              // API key
)

// Integration Service Configuration
holmesIntegration := analytics.NewHolmesGPTIntegrationService(
    holmesClient,
    enhancedAssessor,
    analyticsEngine,
    vectorDB,
    logger,
)

// Set integration in Enhanced Assessor
enhancedAssessor.SetHolmesGPTIntegration(holmesIntegration)
```

## Usage Examples

### Basic Integration

```go
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/jordigilh/kubernaut/pkg/effectiveness"
    "github.com/jordigilh/kubernaut/pkg/effectiveness/analytics"
    "github.com/jordigilh/kubernaut/internal/actionhistory"
)

func main() {
    ctx := context.Background()

    // Setup Enhanced Assessor (existing code)
    assessor, err := effectiveness.NewEnhancedAssessor(
        repo, monitoringClients, vectorDB, patternExtractor,
        analyticsEngine, config, logger,
    )
    if err != nil {
        panic(err)
    }

    // Setup HolmesGPT integration
    holmesClient := analytics.NewHTTPHolmesClient(
        "https://api.holmesgpt.dev",
        "your-api-key",
    )

    holmesIntegration := analytics.NewHolmesGPTIntegrationService(
        holmesClient, assessor, analyticsEngine, vectorDB, logger,
    )

    assessor.SetHolmesGPTIntegration(holmesIntegration)

    // Process alert with HolmesGPT
    trace := &actionhistory.ResourceActionTrace{
        ActionID:      "alert-123",
        ActionType:    "investigate_alert",
        AlertName:     "HighMemoryUsage",
        AlertSeverity: "warning",
        AlertLabels: actionhistory.JSONMap{
            "namespace": "production",
            "pod":       "api-server-123",
        },
    }

    investigation, err := assessor.AssessTraceWithHolmesGPT(ctx, trace)
    if err != nil {
        panic(err)
    }

    // Process the decision
    decision := investigation.FinalDecision
    fmt.Printf("Recommended Action: %s\n", decision.RecommendedAction)
    fmt.Printf("Confidence: %.1f%%\n", decision.Confidence*100)
    fmt.Printf("Requires Approval: %t\n", decision.RequiresApproval)
    fmt.Printf("Justification: %s\n", decision.Justification)
}
```

### Advanced Workflow Integration

```go
// Integration with existing workflow engine
func (ers *EnhancedRemediationService) ProcessAlert(
    ctx context.Context,
    alertName string,
    alertLabels map[string]string,
    severity string,
) (*ProcessingResult, error) {

    // Convert alert to internal format
    alert := &analytics.Alert{
        Name:     alertName,
        Severity: severity,
        Labels:   alertLabels,
        StartsAt: time.Now(),
    }

    // Create action trace
    trace := &actionhistory.ResourceActionTrace{
        ActionID:      fmt.Sprintf("alert-%d", time.Now().Unix()),
        ActionType:    "investigate_alert",
        AlertName:     alertName,
        AlertSeverity: severity,
        AlertLabels:   convertMapToJSONMap(alertLabels),
    }

    // Use HolmesGPT with historical context
    investigation, err := ers.holmesIntegration.InvestigateWithHistoricalContext(ctx, alert, trace)
    if err != nil {
        return nil, fmt.Errorf("investigation failed: %w", err)
    }

    // Process the decision
    result := &ProcessingResult{
        AlertName:       alertName,
        Investigation:   investigation,
        ProcessingTime:  investigation.ProcessingTime,
        DecisionSummary: ers.holmesIntegration.GetDecisionSummary(investigation),
    }

    return result, nil
}
```

## API Reference

### Core Types

#### `Investigation`
```go
type Investigation struct {
    Alert          *Alert                       `json:"alert"`
    HolmesAnalysis *HolmesResponse              `json:"holmes_analysis"`
    HistoricalData *HistoricalAnalysisContext   `json:"historical_data"`
    FinalDecision  *RemediationDecision         `json:"final_decision"`
    ProcessingTime time.Duration                `json:"processing_time"`
}
```

#### `RemediationDecision`
```go
type RemediationDecision struct {
    RecommendedAction  string            `json:"recommended_action"`
    Justification      string            `json:"justification"`
    ExpectedOutcome    string            `json:"expected_outcome"`
    RiskAssessment     string            `json:"risk_assessment"`
    AlternativeActions []string          `json:"alternative_actions"`
    Parameters         map[string]string `json:"parameters"`
    Confidence         float64           `json:"confidence"`
    RequiresApproval   bool              `json:"requires_approval"`
}
```

#### `HistoricalAnalysisContext`
```go
type HistoricalAnalysisContext struct {
    SimilarPatterns      []*vector.SimilarPattern `json:"similar_patterns"`
    PatternAnalytics     *PatternAnalytics        `json:"pattern_analytics"`
    PredictiveInsights   *PredictiveInsightResult `json:"predictive_insights"`
    CostAnalysis         *CostAnalysisResult      `json:"cost_analysis"`
    AssessmentMetrics    *AssessmentMetrics       `json:"assessment_metrics"`
    EffectivenessScore   float64                  `json:"effectiveness_score"`
    ConfidenceLevel      float64                  `json:"confidence_level"`
    LearningContribution float64                  `json:"learning_contribution"`
}
```

### Key Methods

#### `AssessTraceWithHolmesGPT`
```go
func (ea *EnhancedAssessor) AssessTraceWithHolmesGPT(
    ctx context.Context,
    trace *actionhistory.ResourceActionTrace,
) (*analytics.Investigation, error)
```
Performs assessment using HolmesGPT integration with full historical context.

#### `InvestigateWithHistoricalContext`
```go
func (h *HolmesGPTIntegrationService) InvestigateWithHistoricalContext(
    ctx context.Context,
    alert *Alert,
    trace *actionhistory.ResourceActionTrace,
) (*Investigation, error)
```
Core integration method that combines historical analysis with HolmesGPT decision-making.

#### `SetHolmesGPTIntegration`
```go
func (ea *EnhancedAssessor) SetHolmesGPTIntegration(
    holmesIntegration *analytics.HolmesGPTIntegrationService,
)
```
Configures HolmesGPT integration for the Enhanced Assessor.

## Example Enriched Prompt

Here's an example of the comprehensive prompt sent to HolmesGPT:

```markdown
# Alert Remediation Decision Request

You are an expert SRE making remediation decisions based on comprehensive historical analysis.

## Current Alert
- **Name**: HighMemoryUsage
- **Severity**: warning
- **Started**: 2025-08-31T19:45:10Z
- **Labels**:
  - namespace: production
  - pod: api-server-123

## Historical Effectiveness Analysis
- **Overall Effectiveness Score**: 0.84/1.0
- **Confidence Level**: 0.89/1.0
- **Learning Contribution**: 0.23/1.0

## Similar Historical Patterns
### Pattern 1 (Similarity: 0.94)
- **Action Type**: scale_deployment
- **Alert**: HighMemoryUsage
- **Historical Success Rate**: 87.5%
- **Execution Count**: 14 successful, 2 failed

## Predictive Analysis
- **Predicted Effectiveness**: 0.89/1.0
- **Model Confidence**: 0.91/1.0
- **Model Used**: similarity
- **Anomaly Score**: 0.12
- **Trend Analysis**: strongly_improving

## Cost-Benefit Analysis
- **Estimated Cost**: $2.45
- **Expected Savings**: $15.30
- **Cost Efficiency Rating**: high
- **Alternative Action Costs**:
  - restart_pod: $0.50
  - increase_resources: $8.20

## System Performance Context
- **Total Assessments**: 100
- **Success Rate**: 85.0%
- **Average Assessment Duration**: 2s

## Decision Request
Based on this comprehensive historical analysis, please provide:

1. **Recommended Action**: The specific remediation action to take
2. **Justification**: Why this action is recommended based on the historical data
3. **Expected Outcome**: What you expect to happen
4. **Risk Assessment**: Potential risks and mitigation strategies
5. **Alternative Actions**: Other viable options ranked by preference
6. **Parameters**: Specific parameters for the recommended action
7. **Confidence Level**: Your confidence in this recommendation (0.0-1.0)
8. **Approval Required**: Whether this action requires human approval

Focus on leveraging the historical effectiveness data and similar patterns to make the most informed decision.
```

## Testing

### Unit Tests

The integration includes comprehensive unit tests using Ginkgo + Gomega:

```bash
# Run HolmesGPT integration tests
go test -v ./pkg/effectiveness/analytics/ --ginkgo.focus="HolmesGPT Integration"

# Run Enhanced Assessor integration tests
go test -v ./pkg/effectiveness/ --ginkgo.focus="HolmesGPT Integration"

# Run all effectiveness tests
go test -v ./pkg/effectiveness/
```

### Mock Testing

The integration provides mock clients for testing:

```go
// Create mock HolmesGPT client
mockClient := analytics.NewMockHolmesClient()
mockClient.AskResponse = &analytics.HolmesResponse{
    Analysis: "Mock analysis",
    Recommendations: []analytics.Recommendation{
        {
            Action:      "scale_deployment",
            Description: "Scale deployment to handle increased load",
            Priority:    "high",
            Risk:        "low",
            Parameters:  map[string]string{"replicas": "3"},
            Confidence:  0.85,
        },
    },
    Confidence: 0.85,
}

// Use in tests
investigation, err := mockClient.Investigate(ctx, alert, investigationCtx)
```

## Monitoring and Observability

### Metrics

The integration tracks comprehensive metrics:

- **Assessment Performance**: Total assessments, success/failure rates, processing duration
- **Feature Usage**: Pattern learning, predictive analysis, cost analysis usage counts
- **Resource Usage**: Vector DB queries, analytics engine calls, worker pool utilization
- **Quality Metrics**: Average confidence levels, enhanced scores
- **Error Tracking**: Errors by component and severity

### Logging

Structured logging provides visibility into:
- Decision reasoning and justification
- Historical evidence used in decisions
- Processing times and performance metrics
- Error conditions and recovery

### Example Log Output

```json
{
  "level": "info",
  "msg": "Completed HolmesGPT investigation with historical context",
  "alert_name": "HighMemoryUsage",
  "processing_time": "2.3s",
  "confidence": 0.89,
  "recommended_action": "scale_deployment",
  "requires_approval": false,
  "effectiveness_score": 0.84,
  "similar_patterns": 3,
  "timestamp": "2025-08-31T19:45:10Z"
}
```

## Benefits

### 1. **Enhanced Decision Quality**
- Combines 15+ data sources from HolmesGPT with sophisticated historical analysis
- Leverages proven AI investigation capabilities with domain-specific effectiveness data
- Provides structured, auditable decision-making process
- **Consistent investigation quality across HolmesGPT and LLM fallback paths**

### 2. **Reduced Development Overhead**
- Eliminates need to build custom AI investigation engine
- Leverages battle-tested HolmesGPT with 1.3k GitHub stars and active community
- Maintains existing investment in effectiveness assessment logic
- **LLM fallback automatically inherits context enrichment patterns**

### 3. **Improved Alert Response**
- Faster investigation with comprehensive context
- Higher confidence decisions based on historical data
- Automatic approval logic for risk management
- **No degraded experience when HolmesGPT is unavailable**

### 4. **Better Cost Management**
- Integrated cost-benefit analysis in decision-making
- ROI-aware recommendations
- Alternative action cost comparisons

### 5. **Business Requirements Compliance**
- **BR-AI-011**: Historical pattern investigation available in both AI paths
- **BR-AI-012**: Evidence-based root cause analysis across all AI services
- **BR-AI-013**: Alert correlation capabilities maintained during fallback scenarios

## Migration Guide

### From Traditional Assessment

1. **Enable HolmesGPT**: Set `enable_holmes_gpt: true` in configuration
2. **Setup Integration**: Configure HolmesGPT client and integration service
3. **Update Workflows**: Replace `AssessTraceWithEnhancement` calls with `AssessTraceWithHolmesGPT`
4. **Monitor Performance**: Track metrics and adjust configuration as needed

### Backward Compatibility

The integration is fully backward compatible:
- Traditional assessment methods remain unchanged
- HolmesGPT integration is optional and disabled by default
- Existing workflows continue to work without modification

## Troubleshooting

### Common Issues

#### 1. **Integration Not Configured**
```
Error: HolmesGPT integration not configured
```
**Solution**: Call `SetHolmesGPTIntegration()` with a valid integration service.

#### 2. **Integration Disabled**
```
Error: HolmesGPT integration is disabled
```
**Solution**: Set `enable_holmes_gpt: true` in configuration.

#### 3. **API Connection Issues**
```
Error: HolmesGPT investigation failed: connection timeout
```
**Solution**: Check HolmesGPT API endpoint and network connectivity.

#### 4. **Historical Data Missing**
```
Warning: No similar patterns found for decision context
```
**Solution**: Allow time for pattern learning or seed with historical data.

### Debug Mode

Enable detailed logging for troubleshooting:

```go
logger.SetLevel(logrus.DebugLevel)
```

This provides detailed information about:
- Prompt generation process
- Historical data collection
- HolmesGPT API interactions
- Decision processing logic

## Future Enhancements

### Planned Features

1. **Real-time Learning**: Continuous model improvement based on decision outcomes
2. **Multi-Model Support**: Integration with multiple AI providers
3. **Advanced Approval Workflows**: Configurable approval rules and escalation
4. **Performance Optimization**: Caching and batch processing improvements
5. **Enhanced Metrics**: More detailed performance and quality metrics

### Contributing

To contribute to the HolmesGPT integration:

1. Follow existing code patterns and testing practices
2. Use Ginkgo + Gomega for all new tests
3. Ensure fake Kubernetes clients for K8s interactions
4. Add comprehensive documentation for new features
5. Include performance and error handling considerations

## Conclusion

The HolmesGPT integration provides a powerful enhancement to the kubernaut project, combining sophisticated historical analysis with proven AI investigation capabilities. This integration maintains backward compatibility while providing significant improvements in decision quality, development efficiency, and operational effectiveness.

For more information, see:
- [HolmesGPT Documentation](https://holmesgpt.dev/)
- [Enhanced Assessor Documentation](./ENHANCED_ASSESSOR.md)
- [API Reference](./API_REFERENCE.md)
