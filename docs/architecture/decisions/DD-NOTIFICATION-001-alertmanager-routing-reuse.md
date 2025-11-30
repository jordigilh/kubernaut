# DD-NOTIFICATION-001: Alertmanager Routing Library Reuse

**Status**: ✅ Approved
**Version**: 1.0
**Date**: 2025-11-28
**Confidence**: 99%

---

## Context

The Notification Controller needs to determine which delivery channel(s) to use for each `NotificationRequest` CRD. This routing decision must support:

1. **Type-based routing**: Approval notifications → Slack, failures → PagerDuty
2. **Environment-based routing**: Production → PagerDuty, staging → Slack
3. **Label matching**: Flexible routing based on arbitrary labels
4. **Multi-channel fanout**: Send to multiple channels simultaneously
5. **Familiar configuration**: SREs shouldn't learn a new routing syntax

---

## Problem

**Question**: How does the Notification Controller know which channel to deliver to?

**Options**:
1. **Custom routing logic**: Implement our own label matching and routing tree
2. **Alertmanager routing reuse**: Import and use Alertmanager's proven routing library

---

## Decision

**APPROVED: Reuse Alertmanager's routing library directly.**

We will import `github.com/prometheus/alertmanager` and use its routing algorithm rather than reimplementing it.

---

## Rationale

| Aspect | Custom Implementation | Alertmanager Reuse |
|--------|----------------------|-------------------|
| **Development effort** | 2-3 weeks | 2-3 days |
| **Bug risk** | High (routing edge cases) | Near-zero (10+ years battle-tested) |
| **User familiarity** | New syntax to learn | Already know Alertmanager config |
| **Maintenance burden** | Ongoing | Alertmanager team maintains |
| **Feature parity** | Must implement | Grouping, silencing, inhibition available |
| **Confidence** | 85% | **99%** |

**Key Insight**: Alertmanager's routing logic is available as a Go library. There is no reason to reimplement what already exists and is proven in production across millions of Kubernetes clusters.

---

## Implementation

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│  Notification Controller                                        │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                                                           │  │
│  │  1. Watch NotificationRequest CRDs        (our code)      │  │
│  │  2. Extract labels from CRD               (our code)      │  │
│  │  3. Route labels → receiver               (Alertmanager)  │  │
│  │  4. Deliver to channel                    (our code)      │  │
│  │  5. Update CRD status                     (our code)      │  │
│  │                                                           │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Router Implementation

```go
// pkg/notification/routing/router.go
package routing

import (
    "fmt"
    "os"

    "github.com/prometheus/alertmanager/config"
    "github.com/prometheus/alertmanager/dispatch"
    "github.com/prometheus/common/model"
)

// Router uses Alertmanager's routing algorithm for channel selection
type Router struct {
    tree      *dispatch.Route
    receivers map[string]*config.Receiver
}

// NewRouter creates a router from Alertmanager-format config file
func NewRouter(configPath string) (*Router, error) {
    cfg, err := config.LoadFile(configPath)
    if err != nil {
        return nil, fmt.Errorf("load alertmanager config: %w", err)
    }

    r := &Router{
        tree:      dispatch.NewRoute(cfg.Route, nil),
        receivers: make(map[string]*config.Receiver),
    }

    for _, recv := range cfg.Receivers {
        r.receivers[recv.Name] = recv
    }

    return r, nil
}

// Match uses Alertmanager's battle-tested matching logic to find receiver
func (r *Router) Match(labels map[string]string) (*config.Receiver, error) {
    // Convert to Alertmanager's LabelSet
    lset := make(model.LabelSet, len(labels))
    for k, v := range labels {
        lset[model.LabelName(k)] = model.LabelValue(v)
    }

    // Use Alertmanager's routing tree
    routes := r.tree.Match(lset)
    if len(routes) == 0 {
        if defaultRecv, ok := r.receivers["default"]; ok {
            return defaultRecv, nil
        }
        return nil, fmt.Errorf("no matching route and no default receiver")
    }

    receiverName := routes[0].RouteOpts.Receiver
    receiver, ok := r.receivers[receiverName]
    if !ok {
        return nil, fmt.Errorf("receiver %q not found", receiverName)
    }

    return receiver, nil
}

// GetChannelConfig extracts channel-specific config from receiver
func (r *Router) GetChannelConfig(receiver *config.Receiver) []ChannelConfig {
    var configs []ChannelConfig

    for _, sc := range receiver.SlackConfigs {
        configs = append(configs, ChannelConfig{
            Type:       "slack",
            WebhookURL: string(sc.APIURL),
            Channel:    sc.Channel,
        })
    }

    for _, pc := range receiver.PagerdutyConfigs {
        configs = append(configs, ChannelConfig{
            Type:       "pagerduty",
            RoutingKey: string(pc.RoutingKey),
        })
    }

    for _, ec := range receiver.EmailConfigs {
        configs = append(configs, ChannelConfig{
            Type:       "email",
            To:         ec.To,
            From:       ec.From,
            SmartHost:  ec.Smarthost.String(),
        })
    }

    for _, wc := range receiver.WebhookConfigs {
        configs = append(configs, ChannelConfig{
            Type: "webhook",
            URL:  wc.URL.String(),
        })
    }

    return configs
}

// ChannelConfig is our simplified channel config for delivery
type ChannelConfig struct {
    Type       string // slack, pagerduty, email, webhook
    WebhookURL string
    Channel    string
    RoutingKey string
    To         string
    From       string
    SmartHost  string
    URL        string
}
```

### Configuration Format

Users configure routing using **standard Alertmanager config format**:

```yaml
# config/notification-routing.yaml
route:
  receiver: default-slack
  group_by: [type, namespace]
  routes:
    # Production approvals → PagerDuty + Slack
    - match:
        type: approval_required
        environment: production
      receiver: sre-pagerduty
      continue: true  # Also send to next matching route

    - match:
        type: approval_required
        environment: production
      receiver: sre-slack

    # All other approvals → Slack only
    - match:
        type: approval_required
      receiver: ops-slack

    # Remediation failures → PagerDuty
    - match:
        type: remediation_failed
      receiver: sre-pagerduty

    # Remediation success → Slack
    - match:
        type: remediation_completed
      receiver: ops-slack

receivers:
  - name: sre-pagerduty
    pagerduty_configs:
      - routing_key: '${PAGERDUTY_ROUTING_KEY}'
        severity: critical

  - name: sre-slack
    slack_configs:
      - api_url: '${SLACK_WEBHOOK_SRE}'
        channel: '#sre-critical'

  - name: ops-slack
    slack_configs:
      - api_url: '${SLACK_WEBHOOK_OPS}'
        channel: '#kubernaut-alerts'

  - name: default-slack
    slack_configs:
      - api_url: '${SLACK_WEBHOOK_DEFAULT}'
        channel: '#kubernaut-default'
```

### Controller Integration

```go
// internal/controller/notification/notificationrequest_controller.go
package notification

import (
    "context"

    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/notification/routing"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type NotificationRequestReconciler struct {
    client.Client
    Router    *routing.Router
    Deliverer *Deliverer
}

func (r *NotificationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var notif notificationv1alpha1.NotificationRequest
    if err := r.Get(ctx, req.NamespacedName, &notif); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Extract labels for routing
    labels := map[string]string{
        "type":        notif.Spec.Type,
        "environment": notif.Spec.Environment,
        "severity":    notif.Spec.Severity,
        "namespace":   notif.Spec.Namespace,
    }

    // Use Alertmanager routing to find receiver
    receiver, err := r.Router.Match(labels)
    if err != nil {
        return ctrl.Result{}, err
    }

    // Get channel configs from receiver
    channelConfigs := r.Router.GetChannelConfig(receiver)

    // Deliver to all channels (fanout)
    for _, channelCfg := range channelConfigs {
        if err := r.Deliverer.Deliver(ctx, &notif, channelCfg); err != nil {
            // Handle delivery error, update status, etc.
        }
    }

    return ctrl.Result{}, nil
}
```

---

## Version Evolution

| Version | Routing Capability |
|---------|-------------------|
| **V1.0** | Type-based routing (simple match) |
| **V1.1** | Type + Environment routing |
| **V2.0** | Full Alertmanager routing (regex, grouping, inhibition) |

All versions use the same Alertmanager library - we just expose more features over time.

---

## What We Reuse vs Implement

| Component | Reuse from Alertmanager | Implement Ourselves |
|-----------|------------------------|---------------------|
| Config parsing | ✅ `config.LoadFile()` | |
| Routing tree | ✅ `dispatch.Route` | |
| Label matching | ✅ `route.Match()` | |
| CRD watching | | ✅ |
| Label extraction | | ✅ |
| Channel delivery | | ✅ |
| Retry logic | | ✅ |
| Status updates | | ✅ |
| Audit trail | | ✅ |

---

## Dependencies

```go
// go.mod
require (
    github.com/prometheus/alertmanager v0.27.0
    github.com/prometheus/common v0.45.0
)
```

**Note**: Alertmanager is a well-maintained CNCF project with stable APIs.

---

## Consequences

### Positive

- ✅ **Zero routing bugs**: Alertmanager's routing is battle-tested
- ✅ **Familiar config**: SREs already know Alertmanager syntax
- ✅ **Feature-rich**: Grouping, silencing, inhibition available for free
- ✅ **Maintained**: Alertmanager team fixes routing bugs
- ✅ **Fast implementation**: 2-3 days vs 2-3 weeks

### Negative

- ⚠️ **Dependency**: We depend on Alertmanager library
  - **Mitigation**: CNCF project, stable APIs, widely used
- ⚠️ **Config complexity**: Alertmanager config can be complex
  - **Mitigation**: V1.0 uses simple subset, document common patterns

### Neutral

- 🔄 **Config format**: Users must use Alertmanager YAML format
  - Acceptable: It's the industry standard

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| **ADR-017** | NotificationRequest CRD creator (RemediationOrchestrator) |
| **ADR-018** | Approval notification integration |
| **BR-NOT-056** | Priority-based routing requirement |

---

## Review & Evolution

### When to Revisit

- If Alertmanager library introduces breaking changes
- If we need routing features Alertmanager doesn't support
- If users request non-Alertmanager config format

### Success Metrics

- **Routing accuracy**: 100% of notifications reach correct channel
- **Config errors**: <1% of config changes cause routing failures
- **User adoption**: Users successfully configure routing without support tickets

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-11-28 | Initial DD: Alertmanager routing library reuse approved |

