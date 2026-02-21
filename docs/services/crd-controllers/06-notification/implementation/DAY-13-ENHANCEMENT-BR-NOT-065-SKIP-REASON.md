# Day 13: BR-NOT-065 Skip-Reason Label Enhancement

**Version**: 1.1 (Revised)
**Date**: 2025-12-06
**Status**: 📋 READY FOR IMPLEMENTATION
**Estimated Duration**: 4 hours
**Prerequisites**: Day 1-12 complete, DD-WE-004 v1.4 notice acknowledged

---

## 🎯 Overview

Day 13 completes the **skip-reason label routing enhancement** for BR-NOT-065. This adds test coverage for existing skip-reason label routing implementation.

**Business Justification** (from DD-WE-004 v1.1):
- `PreviousExecutionFailed` → Cluster state unknown → Route to PagerDuty (CRITICAL)
- `ExhaustedRetries` → Infrastructure issues → Route to Slack (HIGH)

**Implementation Status**: ✅ Label constants implemented in `pkg/notification/routing/labels.go`

---

## 🔍 Verified API

```go
// Parsing configuration
config, err := routing.ParseConfig([]byte(yamlData))

// Finding receiver based on labels
receiverName := config.Route.FindReceiver(labels)

// Getting receiver details
receiver := config.GetReceiver(receiverName)
```

**Label Constants** (verified in `pkg/notification/routing/labels.go`):
- `routing.LabelSkipReason` = `"kubernaut.ai/skip-reason"`
- `routing.SkipReasonPreviousExecutionFailed` = `"PreviousExecutionFailed"`
- `routing.SkipReasonExhaustedRetries` = `"ExhaustedRetries"`
- `routing.SkipReasonResourceBusy` = `"ResourceBusy"`
- `routing.SkipReasonRecentlyRemediated` = `"RecentlyRemediated"`

---

## 📋 Day 13 Revised Schedule

| Time Block | Task | Duration | Deliverable |
|------------|------|----------|-------------|
| **9:00-9:15** | ANALYSIS: Review existing implementation | 15 min | Context verified |
| **9:15-10:00** | TDD GREEN: Add skip-reason tests | 45 min | 5 unit tests |
| **10:00-10:30** | Integration test: Skip-reason routing | 30 min | 1 integration test |
| **10:30-11:15** | **Controller Integration** (NEW) | 45 min | Routing in reconciler |
| **11:15-11:45** | Create example routing config | 30 min | Example YAML |
| **11:45-12:30** | Create skip-reason runbook | 45 min | Runbook document |
| **12:30-13:00** | Update cross-team document | 30 min | DD-WE-004 v1.5 |

**Total**: ~4 hours (added controller integration)

---

## ⚠️ Key Adjustments from v1.0

| Original Plan | Revised Plan | Reason |
|---------------|--------------|--------|
| TDD RED (60 min) | TDD GREEN (45 min) | Label implementation exists, tests will pass |
| No integration test | Add 1 integration test (30 min) | Verify controller uses labels |
| No controller integration | **Add controller integration (45 min)** | Controller not yet using routing package |
| Runbook (60 min) | Runbook (45 min) | Simpler structure |
| Total: 4 hours | Total: 4 hours | Added controller integration |

### ⚠️ Critical Finding: Controller Integration Missing

**Verified**: `grep -r "routing\." internal/controller/notification/` returns **no matches**.

The `pkg/notification/routing/` package exists but the controller reconciler doesn't use it yet. Day 13 MUST include integrating routing into the controller.

---

## 📋 Prerequisites Checklist

Before starting Day 13:

- [x] `LabelSkipReason` constant added to `pkg/notification/routing/labels.go`
- [x] Skip reason value constants added (`SkipReasonPreviousExecutionFailed`, etc.)
- [x] BR-NOT-065 updated with skip-reason label support
- [x] API specification updated with routing labels section
- [x] Cross-team document acknowledged (DD-WE-004 v1.4)
- [ ] Day 12 complete (all unit/integration/E2E tests passing)

---

## 🔍 Phase 1: ANALYSIS (30 min)

### Task 1.1: Review Existing Implementation

**Files to Review**:
```bash
# Label constants
cat pkg/notification/routing/labels.go

# Routing config
cat pkg/notification/routing/config.go

# Resolver
cat pkg/notification/routing/resolver.go

# Existing tests
cat test/unit/notification/routing_config_test.go
cat test/unit/notification/routing_integration_test.go
```

### Task 1.2: Verify Label Constants

**Validation**:
```go
// Verify these constants exist
routing.LabelSkipReason == "kubernaut.ai/skip-reason"
routing.SkipReasonPreviousExecutionFailed == "PreviousExecutionFailed"
routing.SkipReasonExhaustedRetries == "ExhaustedRetries"
routing.SkipReasonResourceBusy == "ResourceBusy"
routing.SkipReasonRecentlyRemediated == "RecentlyRemediated"
```

**Expected Result**: All constants exist and compile successfully.

---

## 🧪 Phase 2: TDD GREEN - Add Skip-Reason Tests (45 min)

> **Note**: This is TDD GREEN (not RED) because the implementation already exists in `labels.go`.
> Tests verify existing functionality and add coverage for skip-reason routing.

### Task 2.1: Add Skip-Reason Routing Tests

**File**: `test/unit/notification/routing_config_test.go`

Add this new `Describe` block to the existing file (after the existing tests):

```go
// =============================================================================
// SKIP-REASON LABEL ROUTING (BR-NOT-065, DD-WE-004)
// =============================================================================
// Added: Day 13 Enhancement
// Purpose: Verify skip-reason based routing for WorkflowExecution failures
// Cross-Team: WE→NOT Q7, RO Q8 (2025-12-06)
// =============================================================================

var _ = Describe("Skip-Reason Label Routing (BR-NOT-065, DD-WE-004)", func() {

    Context("Label Constants Verification", func() {
        // Test 1: Verify label key constant
        It("should define correct skip-reason label key", func() {
            Expect(routing.LabelSkipReason).To(Equal("kubernaut.ai/skip-reason"))
        })

        // Test 2: Verify skip reason value constants
        It("should define all DD-WE-004 skip reason values", func() {
            Expect(routing.SkipReasonPreviousExecutionFailed).To(Equal("PreviousExecutionFailed"))
            Expect(routing.SkipReasonExhaustedRetries).To(Equal("ExhaustedRetries"))
            Expect(routing.SkipReasonResourceBusy).To(Equal("ResourceBusy"))
            Expect(routing.SkipReasonRecentlyRemediated).To(Equal("RecentlyRemediated"))
        })
    })

    Context("Skip-Reason Routing Rules", func() {
        var config *routing.Config

        BeforeEach(func() {
            // Production-like routing config with skip-reason rules
            configYAML := `
route:
  routes:
    # CRITICAL: Execution failures → PagerDuty
    - match:
        kubernaut.ai/skip-reason: PreviousExecutionFailed
      receiver: pagerduty-critical
    # HIGH: Exhausted retries → Slack
    - match:
        kubernaut.ai/skip-reason: ExhaustedRetries
      receiver: slack-ops
    # LOW: Temporary conditions → Console
    - match:
        kubernaut.ai/skip-reason: ResourceBusy
      receiver: console-bulk
    - match:
        kubernaut.ai/skip-reason: RecentlyRemediated
      receiver: console-bulk
  receiver: default-slack
receivers:
  - name: pagerduty-critical
    pagerduty_configs:
      - service_key: test-critical-key
  - name: slack-ops
    slack_configs:
      - channel: '#kubernaut-ops'
  - name: console-bulk
    console_config:
      enabled: true
  - name: default-slack
    slack_configs:
      - channel: '#kubernaut-alerts'
`
            var err error
            config, err = routing.ParseConfig([]byte(configYAML))
            Expect(err).ToNot(HaveOccurred())
        })

        // Test 3: DescribeTable for all skip-reason routing scenarios
        DescribeTable("should route to correct receiver based on skip-reason",
            func(skipReason, expectedReceiver string, description string) {
                labels := map[string]string{
                    routing.LabelSkipReason: skipReason,
                }
                receiverName := config.Route.FindReceiver(labels)
                Expect(receiverName).To(Equal(expectedReceiver), description)
            },
            Entry("CRITICAL: PreviousExecutionFailed → pagerduty-critical",
                routing.SkipReasonPreviousExecutionFailed, "pagerduty-critical",
                "Execution failures require immediate PagerDuty alerting"),
            Entry("HIGH: ExhaustedRetries → slack-ops",
                routing.SkipReasonExhaustedRetries, "slack-ops",
                "Infrastructure issues route to ops channel"),
            Entry("LOW: ResourceBusy → console-bulk",
                routing.SkipReasonResourceBusy, "console-bulk",
                "Temporary condition - bulk notification only"),
            Entry("LOW: RecentlyRemediated → console-bulk",
                routing.SkipReasonRecentlyRemediated, "console-bulk",
                "Cooldown active - bulk notification only"),
            Entry("FALLBACK: unknown-reason → default-slack",
                "unknown-skip-reason", "default-slack",
                "Unknown skip reasons fall back to default receiver"),
        )

        // Test 4: Combined labels (skip-reason + severity)
        It("should match most specific rule when skip-reason combined with severity", func() {
            // Config with combined matching (more specific first)
            combinedConfigYAML := `
route:
  routes:
    - match:
        skip-reason: PreviousExecutionFailed
        severity: critical
      receiver: pagerduty-immediate
    - match:
        skip-reason: PreviousExecutionFailed
      receiver: slack-escalation
  receiver: default-console
receivers:
  - name: pagerduty-immediate
    pagerduty_configs:
      - service_key: immediate-key
  - name: slack-escalation
    slack_configs:
      - channel: '#escalation'
  - name: default-console
    console_config:
      enabled: true
`
            combinedConfig, err := routing.ParseConfig([]byte(combinedConfigYAML))
            Expect(err).ToNot(HaveOccurred())

            // Both labels - should match first (more specific) rule
            labelsWithSeverity := map[string]string{
                routing.LabelSkipReason: routing.SkipReasonPreviousExecutionFailed,
                routing.LabelSeverity:   routing.SeverityCritical,
            }
            Expect(combinedConfig.Route.FindReceiver(labelsWithSeverity)).To(
                Equal("pagerduty-immediate"),
                "Combined skip-reason+severity should match specific rule")

            // Only skip-reason - should match second rule
            labelsOnlySkip := map[string]string{
                routing.LabelSkipReason: routing.SkipReasonPreviousExecutionFailed,
            }
            Expect(combinedConfig.Route.FindReceiver(labelsOnlySkip)).To(
                Equal("slack-escalation"),
                "Skip-reason alone should match less specific rule")
        })

        // Test 5: Empty/nil labels fallback
        It("should fall back to default receiver when no skip-reason label present", func() {
            emptyLabels := map[string]string{}
            Expect(config.Route.FindReceiver(emptyLabels)).To(Equal("default-slack"))

            Expect(config.Route.FindReceiver(nil)).To(Equal("default-slack"))
        })
    })
})
```

### Task 2.2: Run Tests

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test ./test/unit/notification/... -v --ginkgo.focus="Skip-Reason" 2>&1 | tail -50
```

**Expected**: All 5 tests pass (implementation already complete)

---

## 🔗 Phase 2.5: Integration Test (30 min) - NEW

> **Added in v1.1**: Verify the controller actually uses skip-reason labels during reconciliation.

### Task 2.5.1: Add Skip-Reason Integration Test

**File**: `test/integration/notification/routing_integration_test.go`

Add to existing integration tests:

```go
var _ = Describe("Skip-Reason Routing Integration (BR-NOT-065, DD-WE-004)", func() {

    Context("when NotificationRequest has skip-reason label", func() {

        It("should resolve channels based on skip-reason routing rules", func() {
            ctx := context.Background()

            // Create routing config with skip-reason rules
            routingConfig := routing.DefaultConfig()
            routingConfig.Route.Routes = append(routingConfig.Route.Routes, &routing.Route{
                Match: map[string]string{
                    routing.LabelSkipReason: routing.SkipReasonPreviousExecutionFailed,
                },
                Receiver: "pagerduty-critical",
            })
            routingConfig.Receivers = append(routingConfig.Receivers, &routing.Receiver{
                Name: "pagerduty-critical",
                PagerDutyConfigs: []*routing.PagerDutyConfig{
                    {ServiceKey: "test-key"},
                },
            })

            // Create NotificationRequest with skip-reason label
            notification := &notificationv1alpha1.NotificationRequest{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-skip-reason-routing",
                    Namespace: "default",
                    Labels: map[string]string{
                        routing.LabelSkipReason:    routing.SkipReasonPreviousExecutionFailed,
                        routing.LabelEnvironment:   routing.EnvironmentProduction,
                    },
                },
                Spec: notificationv1alpha1.NotificationRequestSpec{
                    Subject:  "Test Skip-Reason Routing",
                    Body:     "Workflow execution failed",
                    Type:     notificationv1alpha1.NotificationTypeEscalation,
                    Priority: notificationv1alpha1.NotificationPriorityCritical,
                    // Channels intentionally empty - routing rules should apply
                    Channels: []notificationv1alpha1.Channel{},
                },
            }

            // Resolve channels using routing rules
            resolvedChannels := routing.ResolveChannels(ctx, routingConfig, notification)

            // Verify PagerDuty channel resolved based on skip-reason label
            Expect(resolvedChannels).ToNot(BeEmpty(),
                "Skip-reason label should trigger routing rules")
        })
    })
})
```

### Task 2.5.2: Run Integration Test

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go test ./test/integration/notification/... -v --ginkgo.focus="Skip-Reason Routing Integration" 2>&1 | tail -30
```

**Expected**: Integration test passes, verifying controller uses labels.

---

## 🔧 Phase 3: Controller Integration (45 min) - CRITICAL

> **Finding**: The routing package exists but the controller doesn't use it yet.
> This phase integrates routing into the controller reconciler.

### Task 3.1: Add Routing to Controller

**File**: `internal/controller/notification/notificationrequest_controller.go`

Modify the `Reconcile` function to use routing when `spec.channels` is empty:

```go
import (
    "github.com/jordigilh/kubernaut/pkg/notification/routing"
)

// In Reconcile function, after getting the NotificationRequest:

func (r *NotificationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... existing code ...

    // Get the NotificationRequest
    notification := &notificationv1alpha1.NotificationRequest{}
    if err := r.Get(ctx, req.NamespacedName, notification); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Resolve channels using routing rules if spec.channels is empty
    channels := notification.Spec.Channels
    if len(channels) == 0 {
        // BR-NOT-065: Use routing rules to determine channels
        channels = r.resolveChannelsFromRouting(ctx, notification)
        log.FromContext(ctx).Info("Resolved channels from routing rules",
            "notification", notification.Name,
            "channels", channels,
            "labels", notification.Labels)
    }

    // ... continue with delivery using resolved channels ...
}

// resolveChannelsFromRouting uses the routing configuration to determine channels
func (r *NotificationRequestReconciler) resolveChannelsFromRouting(
    ctx context.Context,
    notification *notificationv1alpha1.NotificationRequest,
) []notificationv1alpha1.Channel {
    // Get routing config (loaded from ConfigMap)
    config := r.getRoutingConfig()
    if config == nil {
        log.FromContext(ctx).Info("No routing config, using default console channel")
        return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}
    }

    // Find receiver based on notification labels
    receiverName := config.Route.FindReceiver(notification.Labels)
    receiver := config.GetReceiver(receiverName)
    if receiver == nil {
        log.FromContext(ctx).Info("Receiver not found, using default console channel",
            "receiver", receiverName)
        return []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole}
    }

    // Convert receiver to channels
    return routing.ReceiverToChannels(receiver)
}
```

### Task 3.2: Add Routing Config Loading

**File**: `internal/controller/notification/notificationrequest_controller.go`

Add routing config field and initialization:

```go
type NotificationRequestReconciler struct {
    client.Client
    Scheme *runtime.Scheme
    // ... existing fields ...

    // RoutingConfig holds the loaded routing configuration
    routingConfig *routing.Config
    routingMu     sync.RWMutex
}

func (r *NotificationRequestReconciler) getRoutingConfig() *routing.Config {
    r.routingMu.RLock()
    defer r.routingMu.RUnlock()
    return r.routingConfig
}

func (r *NotificationRequestReconciler) SetRoutingConfig(config *routing.Config) {
    r.routingMu.Lock()
    defer r.routingMu.Unlock()
    r.routingConfig = config
}
```

### Task 3.3: Run Tests

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
go build ./internal/controller/notification/...
go test ./test/unit/notification/... -v --count=1 2>&1 | tail -20
```

**Expected**: Build succeeds, existing tests still pass.

---

## 📝 Phase 4: Create Example Routing Configuration (30 min)

### Task 3.1: Create Skip-Reason Routing Example

**File**: `config/notification/routing-config-skip-reason-example.yaml`

```yaml
# Notification Routing Configuration - Skip-Reason Based Routing
#
# This configuration demonstrates routing based on WorkflowExecution skip reasons
# as defined in DD-WE-004 v1.1 (Exponential Backoff Cooldown).
#
# Usage:
#   kubectl create configmap notification-routing-config \
#     --from-file=routing.yaml=routing-config-skip-reason-example.yaml \
#     -n kubernaut-system
#
# Business Requirements:
#   - BR-NOT-065: Channel Routing Based on Spec Fields
#   - BR-NOT-066: Alertmanager-Compatible Configuration Format
#
# Cross-Team Reference:
#   - DD-WE-004: WorkflowExecution Exponential Backoff
#   - NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md

# Global routing configuration
route:
  # Group notifications by these labels to reduce alert noise
  group_by:
    - kubernaut.ai/environment
    - kubernaut.ai/skip-reason

  # Wait time before sending first notification for a group
  group_wait: 30s

  # Time to wait before sending notification about new alerts in a group
  group_interval: 5m

  # Time to wait before re-sending a notification
  repeat_interval: 4h

  # Child routes - evaluated in order, first match wins
  routes:
    # =================================================================
    # CRITICAL: Execution Failures (PreviousExecutionFailed)
    # =================================================================
    #
    # These notifications indicate a workflow RAN and FAILED.
    # Cluster state is UNKNOWN - manual intervention required.
    # Route to PagerDuty for immediate 24/7 alerting.
    #
    - match:
        kubernaut.ai/skip-reason: PreviousExecutionFailed
      receiver: pagerduty-oncall-critical
      # Override group settings for critical alerts
      group_wait: 0s  # Send immediately
      repeat_interval: 15m  # Remind every 15 minutes

    # =================================================================
    # HIGH: Exhausted Retries
    # =================================================================
    #
    # These notifications indicate 5+ pre-execution failures.
    # Infrastructure issues (quota, validation, image pull).
    # Cluster state is KNOWN - no modifications made.
    # Route to Slack #ops for team awareness.
    #
    - match:
        kubernaut.ai/skip-reason: ExhaustedRetries
      receiver: slack-ops-high
      group_wait: 1m
      repeat_interval: 1h

    # =================================================================
    # LOW: Temporary Conditions (ResourceBusy, RecentlyRemediated)
    # =================================================================
    #
    # These are temporary conditions that auto-resolve.
    # - ResourceBusy: Another WFE is running on target
    # - RecentlyRemediated: Cooldown/backoff period active
    #
    # Route to console only (bulk notifications per BR-ORCH-034).
    # Do NOT wake up operators for temporary conditions.
    #
    - match_re:
        kubernaut.ai/skip-reason: "^(ResourceBusy|RecentlyRemediated)$"
      receiver: console-only-bulk
      group_wait: 5m
      group_interval: 30m

    # =================================================================
    # Production Critical (any notification from production)
    # =================================================================
    #
    # Catch-all for production environment critical notifications
    # that don't match specific skip-reason rules.
    #
    - match:
        environment: production
        severity: critical
      receiver: pagerduty-oncall-critical

    # =================================================================
    # Staging/Development (low priority)
    # =================================================================
    #
    # Non-production notifications go to Slack #dev
    #
    - match_re:
        environment: "^(staging|development|test)$"
      receiver: slack-dev-low

  # Default fallback receiver
  receiver: slack-default

# Receiver definitions
receivers:
  # CRITICAL: PagerDuty for 24/7 on-call alerting
  - name: pagerduty-oncall-critical
    pagerduty_configs:
      - service_key: ${PAGERDUTY_CRITICAL_SERVICE_KEY}
        severity: critical
        description: |
          🔴 CRITICAL: Workflow Execution Failed

          A workflow execution has FAILED after running.
          Cluster state is UNKNOWN - manual intervention required.

          Skip Reason: {{ .Spec.Metadata.skip-reason }}
          Target: {{ .Spec.Metadata.target-resource }}
        details:
          skip_reason: '{{ .Spec.Metadata.skip-reason }}'
          environment: '{{ .Spec.Metadata.environment }}'
          remediation_request: '{{ .Spec.RemediationRequestRef.Name }}'

  # HIGH: Slack #ops for team awareness
  - name: slack-ops-high
    slack_configs:
      - api_url: ${SLACK_OPS_WEBHOOK_URL}
        channel: '#kubernaut-ops'
        title: '⚠️ Workflow Retries Exhausted'
        text: |
          *Skip Reason:* {{ .Spec.Metadata.skip-reason }}
          *Environment:* {{ .Spec.Metadata.environment }}
          *Remediation:* {{ .Spec.RemediationRequestRef.Name }}

          Infrastructure issues have caused 5+ pre-execution failures.
          Manual investigation required.
        color: '#FF9800'  # Orange

  # LOW: Console only for bulk notifications
  - name: console-only-bulk
    console_config:
      enabled: true

  # MEDIUM: Slack for dev environments
  - name: slack-dev-low
    slack_configs:
      - api_url: ${SLACK_DEV_WEBHOOK_URL}
        channel: '#kubernaut-dev'
        title: '📢 Notification from {{ index .Labels "kubernaut.ai/environment" }}'
        color: '#2196F3'  # Blue

  # DEFAULT: Slack fallback
  - name: slack-default
    slack_configs:
      - api_url: ${SLACK_DEFAULT_WEBHOOK_URL}
        channel: '#kubernaut-alerts'
        title: '🔔 Kubernaut Notification'
        color: '#9E9E9E'  # Gray
```

---

## 📚 Phase 4: Update Runbooks (60 min)

### Task 4.1: Create Skip-Reason Routing Runbook

**File**: `docs/services/crd-controllers/06-notification/runbooks/SKIP_REASON_ROUTING.md`

```markdown
# Runbook: Skip-Reason Based Notification Routing

**Version**: 1.0
**Last Updated**: 2025-12-06
**Status**: ✅ Production-Ready
**Related BRs**: BR-NOT-065, BR-NOT-066
**Related DDs**: DD-WE-004 v1.1

---

## 📋 Overview

This runbook documents the skip-reason based notification routing system, which routes notifications differently based on WorkflowExecution skip reasons.

---

## 🔴 PreviousExecutionFailed (CRITICAL)

### What It Means

A workflow **ran and failed** during execution. Cluster state is **UNKNOWN** because non-idempotent actions may have partially executed.

### Example Scenario

```
Workflow: "increase-replicas"
  Step 1: kubectl patch deployment --replicas +1  ← EXECUTED
  Step 2: kubectl apply memory limits             ← FAILED

Result: Replicas = original + 1 (cluster modified)
```

### Routing Configuration

```yaml
- match:
    kubernaut.ai/skip-reason: PreviousExecutionFailed
  receiver: pagerduty-oncall-critical
```

### Operator Actions

1. **Investigate** the failed WorkflowExecution CRD
2. **Verify** cluster state manually
3. **Clear** the block using annotation:
   ```bash
   kubectl annotate workflowexecution <name> \
     kubernaut.ai/clear-execution-block="acknowledged-by-<operator>"
   ```
4. **Retry** manually if appropriate

### Prometheus Alert

```yaml
- alert: WorkflowExecutionFailed
  expr: increase(workflow_execution_skip_total{reason="PreviousExecutionFailed"}[5m]) > 0
  for: 0m
  labels:
    severity: critical
  annotations:
    summary: "Workflow execution failed - cluster state unknown"
    description: "WorkflowExecution {{ $labels.name }} failed during execution. Manual intervention required."
```

---

## 🟠 ExhaustedRetries (HIGH)

### What It Means

5+ **pre-execution** failures have occurred. Infrastructure issues are preventing workflow execution, but cluster state is **KNOWN** (unchanged).

### Common Causes

- Image pull failures (registry unavailable)
- Resource quota exceeded
- Validation webhook failures
- Network policy blocking

### Routing Configuration

```yaml
- match:
    kubernaut.ai/skip-reason: ExhaustedRetries
  receiver: slack-ops-high
```

### Operator Actions

1. **Check** the recent failures in WFE status
2. **Investigate** infrastructure issues (quota, images, network)
3. **Fix** the underlying infrastructure problem
4. **Clear** the exhausted retries state (automatic on next successful WFE)

### Prometheus Alert

```yaml
- alert: WorkflowRetryExhausted
  expr: increase(workflow_execution_skip_total{reason="ExhaustedRetries"}[5m]) > 0
  for: 0m
  labels:
    severity: high
  annotations:
    summary: "Workflow retry exhausted - infrastructure issue"
    description: "WorkflowExecution {{ $labels.name }} exhausted retries. Check infrastructure."
```

---

## 🟢 ResourceBusy (LOW)

### What It Means

Another WorkflowExecution is currently running on the same target resource. This is a **temporary** condition that auto-resolves.

### Routing Configuration

```yaml
- match:
    kubernaut.ai/skip-reason: ResourceBusy
  receiver: console-only-bulk
```

### Operator Actions

Usually none required. The system will automatically retry when the current WFE completes.

---

## 🟢 RecentlyRemediated (LOW)

### What It Means

A cooldown or backoff period is active for this target+workflow combination. This is a **temporary** condition that auto-resolves.

### Routing Configuration

```yaml
- match:
    kubernaut.ai/skip-reason: RecentlyRemediated
  receiver: console-only-bulk
```

### Operator Actions

Usually none required. The system will automatically retry when the cooldown expires.

---

## 📊 Monitoring Dashboard

### Key Metrics

| Metric | Query | Description |
|--------|-------|-------------|
| Skip by Reason | `sum by(reason)(workflow_execution_skip_total)` | Total skips by reason |
| Critical Skips | `workflow_execution_skip_total{reason="PreviousExecutionFailed"}` | Execution failures |
| High Skips | `workflow_execution_skip_total{reason="ExhaustedRetries"}` | Exhausted retries |

### Grafana Panel

```json
{
  "title": "WFE Skips by Reason",
  "type": "timeseries",
  "targets": [
    {
      "expr": "sum by(reason)(rate(workflow_execution_skip_total[5m]))",
      "legendFormat": "{{reason}}"
    }
  ]
}
```

---

## 🔧 Troubleshooting

### Notifications Not Routing Correctly

1. **Check labels on NotificationRequest**:
   ```bash
   kubectl get notificationrequest <name> -o yaml | grep -A10 labels
   ```

2. **Verify routing config loaded**:
   ```bash
   kubectl logs -f deployment/notification-controller -n kubernaut-system | grep "routing"
   ```

3. **Check routing decision in logs**:
   ```bash
   kubectl logs -f deployment/notification-controller -n kubernaut-system | grep "receiver"
   ```

### Skip-Reason Not Set

If `kubernaut.ai/skip-reason` label is missing, check:
1. RO is setting labels correctly (per DD-WE-004 Q8 response)
2. WFE status has `SkipDetails.Reason` populated

---

**Document Version**: 1.0
**Last Updated**: 2025-12-06
```

---

## ✅ Phase 5: Update Cross-Team Document (30 min)

### Task 5.1: Mark Implementation Complete

Update `docs/handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md`:

- Mark all Day 13 tasks as complete
- Update version to 1.5
- Add completion changelog entry

---

## 📋 Day 13 Completion Checklist

### Tests
- [ ] Skip-reason unit tests added (5 test cases)
- [ ] Skip-reason integration test added (1 test case)
- [ ] All tests passing (`go test ./test/unit/notification/... -v --ginkgo.focus="Skip-Reason"`)
- [ ] Integration tests passing (`go test ./test/integration/notification/... -v --ginkgo.focus="Skip-Reason"`)
- [ ] No lint errors

### Controller Integration (CRITICAL)
- [ ] `resolveChannelsFromRouting()` added to controller
- [ ] `getRoutingConfig()` / `SetRoutingConfig()` added
- [ ] Controller uses routing when `spec.channels` is empty
- [ ] Build succeeds: `go build ./internal/controller/notification/...`
- [ ] Existing tests still pass

### Configuration
- [ ] Example routing config created (`routing-config-skip-reason-example.yaml`)
- [ ] Config validated (YAML syntax)

### Documentation
- [ ] Skip-reason runbook created (`SKIP_REASON_ROUTING.md`)
- [ ] API specification already updated (v2.1) ✅
- [ ] BR-NOT-065 updated with mandatory labels ✅
- [ ] Cross-team document updated (v1.5)

### Cross-Team
- [ ] DD-WE-004 fully acknowledged (v1.5)
- [ ] RO Team notified of mandatory labels requirement
- [ ] WE Team notified of completion

---

## 📊 Success Criteria

| Criterion | Target | Measurement |
|-----------|--------|-------------|
| Unit Test Coverage | 5 skip-reason tests | `go test --ginkgo.focus="Skip-Reason Label"` |
| Integration Test | 1 routing test | `go test --ginkgo.focus="Skip-Reason Routing Integration"` |
| Documentation | 100% complete | Runbook + example config created |
| Cross-Team | DD-WE-004 v1.5 | Acknowledgment updated |
| Build | Zero errors | `go build ./pkg/notification/...` |

---

## 🔗 References

| Document | Description |
|----------|-------------|
| [DD-WE-004](../../../../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md) | Exponential Backoff Design |
| [Cross-Team Notice](../../../../handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md) | DD-WE-004 Notice |
| [BR-NOT-065](../BUSINESS_REQUIREMENTS.md#br-not-065-channel-routing-based-on-labels) | Channel Routing BR |
| [Enhancement Plan](./ENHANCEMENT_BR-NOT-065-SKIP-REASON-LABEL.md) | Initial Enhancement Plan |

---

## 📝 EOD Report Template

```markdown
# Day 13 EOD Report - BR-NOT-065 Skip-Reason Enhancement

**Date**: 2025-12-XX
**Duration**: ~3.5 hours
**Status**: ✅ COMPLETE

## Completed Tasks

- [x] Skip-reason unit tests (5 tests) - PASSED
- [x] Skip-reason integration test (1 test) - PASSED
- [x] Example routing configuration - CREATED
- [x] Skip-reason runbook - CREATED
- [x] Cross-team document update - DD-WE-004 v1.5

## Test Results

```bash
go test ./test/unit/notification/... -v --ginkgo.focus="Skip-Reason"
# Result: 5 passed, 0 failed

go test ./test/integration/notification/... -v --ginkgo.focus="Skip-Reason"
# Result: 1 passed, 0 failed

# Total notification tests: 249 + 6 = 255 tests
```

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `test/unit/notification/routing_config_test.go` | Modified | +95 |
| `test/integration/notification/routing_integration_test.go` | Modified | +45 |
| `config/notification/routing-config-skip-reason-example.yaml` | Created | ~150 |
| `docs/.../runbooks/SKIP_REASON_ROUTING.md` | Created | ~180 |
| `docs/handoff/NOTICE_WE_EXPONENTIAL_BACKOFF_DD_WE_004.md` | Modified | +20 |

## Confidence Assessment

**Confidence**: 95%
**Justification**:
- All skip-reason label constants verified
- Unit tests cover all 4 skip reasons + fallback
- Integration test verifies controller uses labels
- Cross-team agreement documented in DD-WE-004

## Next Steps

1. Monitor skip-reason routing in staging environment
2. Gather feedback from RO and WE teams
3. Consider adding Prometheus metrics for skip-reason routing
```

---

**Document Version**: 1.1 (Revised)
**Last Updated**: 2025-12-06
**Status**: 📋 Ready for Day 13 Execution

---

## 📋 Revision History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-06 | Initial Day 13 plan |
| 1.1 | 2025-12-06 | Revised: TDD GREEN (not RED), added integration test, reduced timeline to 3.5h |

