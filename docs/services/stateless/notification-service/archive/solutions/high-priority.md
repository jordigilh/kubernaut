# Notification Service - HIGH Priority Issues Solutions

**Date**: 2025-10-03
**Status**: 📝 **SOLUTIONS PROVIDED**

---

## ✅ **APPROVED HIGH PRIORITY ISSUES**

### **HIGH-2: Configurable Data Freshness Thresholds** ✅ **APPROVED**

#### **Solution**: Environment and Severity-Based Freshness Configuration

**ConfigMap Configuration**:
```yaml
# config/notification-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-config
  namespace: kubernaut-system
data:
  freshness-config.yaml: |
    # Data freshness thresholds
    data_freshness:
      # Default threshold for all notifications
      default_threshold_seconds: 30

      # Per-severity thresholds (override default)
      per_severity:
        critical: 10   # Critical alerts need very fresh data (10s)
        warning: 30    # Warning alerts use default (30s)
        info: 60       # Info alerts tolerate older data (60s)

      # Per-environment thresholds (override severity)
      per_environment:
        production:
          critical: 5   # Production critical: 5s
          warning: 15   # Production warning: 15s
        staging:
          critical: 10  # Staging critical: 10s
          warning: 30   # Staging warning: 30s
        development:
          critical: 30  # Dev critical: 30s
          warning: 60   # Dev warning: 60s

      # Action on stale data
      on_stale: "warn"  # Options: warn, error, proceed

      # Staleness warning message
      staleness_warning_template: "⚠️ Data is {{.Age}} old (threshold: {{.Threshold}}). Click 'Verify Current State' to refresh."
```

**Go Implementation**:
```go
// pkg/notification/freshness/config.go
package freshness

import (
    "fmt"
    "time"
)

type FreshnessConfig struct {
    DefaultThresholdSeconds int                            `yaml:"default_threshold_seconds"`
    PerSeverity            map[string]int                 `yaml:"per_severity"`
    PerEnvironment         map[string]map[string]int      `yaml:"per_environment"`
    OnStale                string                         `yaml:"on_stale"` // warn, error, proceed
    StalenessWarningTemplate string                       `yaml:"staleness_warning_template"`
}

type FreshnessChecker struct {
    config *FreshnessConfig
}

func NewFreshnessChecker(config *FreshnessConfig) *FreshnessChecker {
    return &FreshnessChecker{config: config}
}

// GetThreshold returns the freshness threshold for a given severity and environment
func (fc *FreshnessChecker) GetThreshold(severity, environment string) time.Duration {
    // Priority: environment+severity > severity > default

    // Check environment-specific threshold
    if envThresholds, exists := fc.config.PerEnvironment[environment]; exists {
        if threshold, exists := envThresholds[severity]; exists {
            return time.Duration(threshold) * time.Second
        }
    }

    // Check severity-specific threshold
    if threshold, exists := fc.config.PerSeverity[severity]; exists {
        return time.Duration(threshold) * time.Second
    }

    // Fall back to default
    return time.Duration(fc.config.DefaultThresholdSeconds) * time.Second
}

// CheckFreshness validates data freshness and returns result
func (fc *FreshnessChecker) CheckFreshness(dataTimestamp time.Time, severity, environment string) FreshnessResult {
    threshold := fc.GetThreshold(severity, environment)
    age := time.Since(dataTimestamp)

    isStale := age > threshold

    return FreshnessResult{
        DataTimestamp:    dataTimestamp,
        Age:              age,
        Threshold:        threshold,
        IsStale:          isStale,
        AgeHumanReadable: formatDuration(age),
        StalenessWarning: fc.generateWarning(age, threshold, isStale),
    }
}

type FreshnessResult struct {
    DataTimestamp    time.Time
    Age              time.Duration
    Threshold        time.Duration
    IsStale          bool
    AgeHumanReadable string
    StalenessWarning string
}

func (fc *FreshnessChecker) generateWarning(age, threshold time.Duration, isStale bool) string {
    if !isStale {
        return ""
    }

    switch fc.config.OnStale {
    case "warn":
        return fmt.Sprintf("⚠️ Data is %s old (threshold: %s). Click 'Verify Current State' to refresh.",
            formatDuration(age), formatDuration(threshold))
    case "error":
        return fmt.Sprintf("❌ Data is too stale (%s old, threshold: %s). Refresh required.",
            formatDuration(age), formatDuration(threshold))
    case "proceed":
        return "" // No warning
    default:
        return fmt.Sprintf("⚠️ Data freshness: %s", formatDuration(age))
    }
}

func formatDuration(d time.Duration) string {
    if d < time.Second {
        return fmt.Sprintf("%dms", d.Milliseconds())
    }
    if d < time.Minute {
        return fmt.Sprintf("%ds", int(d.Seconds()))
    }
    if d < time.Hour {
        return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
    }
    return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}
```

**Usage in Notification Service**:
```go
// pkg/notification/service.go
package notification

func (ns *NotificationService) SendEscalation(ctx context.Context, req *EscalationNotificationRequest) (*EscalationNotificationResponse, error) {
    // ... existing code ...

    // Check data freshness
    freshnessResult := ns.freshnessChecker.CheckFreshness(
        req.Payload.Alert.FiredAt,
        req.Payload.Alert.Severity,
        req.Payload.ImpactedResources.Environment,
    )

    if freshnessResult.IsStale {
        ns.logger.Warn("stale data in notification",
            zap.String("alert_uid", req.Payload.Alert.UID),
            zap.Duration("age", freshnessResult.Age),
            zap.Duration("threshold", freshnessResult.Threshold))

        if ns.freshnessConfig.OnStale == "error" {
            return nil, fmt.Errorf("data too stale: %s (threshold: %s)",
                freshnessResult.AgeHumanReadable, freshnessResult.Threshold)
        }
    }

    // Include freshness info in response
    response := &EscalationNotificationResponse{
        // ... existing fields ...
        DataFreshness: freshnessResult,
    }

    return response, nil
}
```

**Notification Template Integration**:
```html
<!-- Email template -->
<div class="freshness-indicator">
  {{if .DataFreshness.IsStale}}
    <div class="staleness-warning">
      {{.DataFreshness.StalenessWarning}}
      <a href="{{.VerifyCurrentStateURL}}" class="btn-verify">Verify Current State</a>
    </div>
  {{else}}
    <span class="freshness-ok">Data freshness: {{.DataFreshness.AgeHumanReadable}}</span>
  {{end}}
</div>
```

**Confidence**: **95%** - Configurable freshness thresholds provide flexibility

---

### **HIGH-3: Channel Retry + Fallback (No Health Checks)** ✅ **CLARIFIED**

#### **Solution**: Configurable Retry + Fallback (Builds on CRITICAL-2)

**Note**: This solution extends **CRITICAL-2** (Error Handling & Retry Logic) with adapter-specific configuration.

**ConfigMap Configuration**:
```yaml
# config/notification-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-config
  namespace: kubernaut-system
data:
  channels.yaml: |
    channels:
      slack:
        retry:
          max_attempts: 3
          initial_backoff: 1s
          max_backoff: 30s
          backoff_multiplier: 2.0
        fallback: "email"  # If Slack fails, try email

      email:
        retry:
          max_attempts: 5   # More retries for email (more reliable)
          initial_backoff: 2s
          max_backoff: 60s
          backoff_multiplier: 2.0
        fallback: "sms"     # If email fails, try SMS

      teams:
        retry:
          max_attempts: 3
          initial_backoff: 1s
          max_backoff: 30s
          backoff_multiplier: 2.0
        fallback: "email"

      sms:
        retry:
          max_attempts: 2   # Fewer retries for SMS (costly)
          initial_backoff: 5s
          max_backoff: 30s
          backoff_multiplier: 2.0
        fallback: null      # No fallback for SMS (last resort)
```

**Enhanced Service Logic**:
```go
// pkg/notification/service.go
package notification

func (ns *NotificationService) SendWithRetryAndFallback(ctx context.Context, req *NotificationRequest) error {
    // Primary channels (in order of preference from request)
    channels := req.Channels // e.g., ["slack", "email", "sms"]

    var lastErr error
    for _, channel := range channels {
        adapter := ns.adapters[channel]
        if adapter == nil {
            ns.logger.Warn("adapter not found, skipping",
                zap.String("channel", channel))
            continue
        }

        // Get channel-specific retry config
        retryConfig := ns.getRetryConfig(channel)

        // Try to send via this channel with retries
        err := ns.sendWithRetry(ctx, channel, adapter, req, retryConfig)
        if err == nil {
            ns.logger.Info("notification sent successfully",
                zap.String("channel", channel))

            // Emit success metric
            NotificationDeliverySuccessTotal.WithLabelValues(channel).Inc()
            return nil // Success
        }

        lastErr = err

        // Emit failure metric
        NotificationDeliveryFailureTotal.WithLabelValues(channel, "exhausted_retries").Inc()

        ns.logger.Error("channel delivery failed after retries",
            zap.String("channel", channel),
            zap.Int("max_attempts", retryConfig.MaxAttempts),
            zap.Error(err))

        // Check if there's a fallback channel configured
        fallbackChannel := ns.getFallbackChannel(channel)
        if fallbackChannel != "" {
            ns.logger.Info("attempting fallback channel",
                zap.String("from_channel", channel),
                zap.String("to_channel", fallbackChannel))

            // Add fallback channel to the list if not already present
            if !contains(channels, fallbackChannel) {
                channels = append(channels, fallbackChannel)
            }
        }
    }

    // All channels failed
    ns.logger.Error("all channels failed",
        zap.Strings("channels_tried", channels),
        zap.Error(lastErr))

    // Emit audit event for failure
    ns.emitAuditEvent(ctx, AuditEvent{
        EventType:       "notification_failed",
        CorrelationID:   req.CorrelationID,
        ChannelsTried:   channels,
        FinalError:      lastErr,
    })

    return fmt.Errorf("all channels failed: %w", lastErr)
}

func (ns *NotificationService) sendWithRetry(ctx context.Context, channel string, adapter ChannelAdapter, req *NotificationRequest, retryConfig RetryConfig) error {
    var lastErr error

    for attempt := 1; attempt <= retryConfig.MaxAttempts; attempt++ {
        err := adapter.Send(ctx, req.Recipient, req.Payload)
        if err == nil {
            if attempt > 1 {
                ns.logger.Info("delivery succeeded after retry",
                    zap.String("channel", channel),
                    zap.Int("attempt", attempt))
            }
            return nil // Success
        }

        lastErr = err

        // Check if error is retryable
        if !isRetryable(err) {
            ns.logger.Error("non-retryable error",
                zap.String("channel", channel),
                zap.Int("attempt", attempt),
                zap.Error(err))
            return fmt.Errorf("non-retryable error: %w", err)
        }

        // Last attempt, no more retries
        if attempt >= retryConfig.MaxAttempts {
            ns.logger.Error("max retry attempts exceeded",
                zap.String("channel", channel),
                zap.Int("max_attempts", retryConfig.MaxAttempts),
                zap.Error(err))
            break
        }

        // Calculate backoff and wait
        backoff := calculateBackoff(attempt, retryConfig)
        ns.logger.Warn("delivery failed, retrying",
            zap.String("channel", channel),
            zap.Int("attempt", attempt),
            zap.Duration("backoff", backoff),
            zap.Error(err))

        select {
        case <-time.After(backoff):
            // Continue to next retry
        case <-ctx.Done():
            return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
        }
    }

    return fmt.Errorf("max retry attempts (%d) exceeded for channel %s: %w",
        retryConfig.MaxAttempts, channel, lastErr)
}

func (ns *NotificationService) getRetryConfig(channel string) RetryConfig {
    if config, exists := ns.channelConfigs[channel]; exists {
        return config.Retry
    }

    // Default retry config
    return RetryConfig{
        MaxAttempts:       3,
        InitialBackoff:    1 * time.Second,
        MaxBackoff:        30 * time.Second,
        BackoffMultiplier: 2.0,
    }
}

func (ns *NotificationService) getFallbackChannel(channel string) string {
    if config, exists := ns.channelConfigs[channel]; exists {
        return config.Fallback
    }
    return ""
}
```

**Audit Logging**:
```go
// Log all retry attempts and fallbacks for audit
type NotificationAuditLog struct {
    NotificationID   string
    CorrelationID    string
    Timestamp        time.Time
    ChannelsTried    []ChannelAttempt
    FinalStatus      string // success, failed
    FinalChannel     string // Which channel succeeded
}

type ChannelAttempt struct {
    Channel      string
    Attempts     int
    LastError    string
    Duration     time.Duration
    Status       string // success, failed, fallback
}
```

**Confidence**: **95%** - Retry + fallback is simpler than health checks and sufficient for V1

---

### **HIGH-4: Notification Deduplication** ✅ **APPROVED**

#### **Solution**: Fingerprint-Based Deduplication with Configurable TTL

**ConfigMap Configuration**:
```yaml
# config/notification-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-config
  namespace: kubernaut-system
data:
  deduplication-config.yaml: |
    deduplication:
      enabled: true

      # Time window for deduplication
      ttl_seconds: 300  # 5 minutes

      # Per-severity TTL overrides
      per_severity:
        critical: 60    # Critical: 1 minute (allow re-notifications sooner)
        warning: 300    # Warning: 5 minutes (default)
        info: 900       # Info: 15 minutes (longer window)

      # Fingerprint components (what makes a notification unique)
      fingerprint_components:
        - alert_fingerprint  # Prometheus alert fingerprint
        - recipient          # Notification recipient
        - channels           # Notification channels (Slack, Email, etc.)

      # Cache size limit
      max_cache_entries: 10000

      # Cache cleanup interval
      cleanup_interval_seconds: 60
```

**Go Implementation**:
```go
// pkg/notification/deduplication/deduplicator.go
package deduplication

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "strings"
    "sync"
    "time"
)

type DeduplicationConfig struct {
    Enabled                 bool              `yaml:"enabled"`
    TTLSeconds              int               `yaml:"ttl_seconds"`
    PerSeverity             map[string]int    `yaml:"per_severity"`
    FingerprintComponents   []string          `yaml:"fingerprint_components"`
    MaxCacheEntries         int               `yaml:"max_cache_entries"`
    CleanupIntervalSeconds  int               `yaml:"cleanup_interval_seconds"`
}

type Deduplicator struct {
    config  *DeduplicationConfig
    cache   map[string]*CacheEntry
    mu      sync.RWMutex
    logger  *zap.Logger
}

type CacheEntry struct {
    Fingerprint string
    LastSent    time.Time
    Count       int // How many times we blocked this notification
}

func NewDeduplicator(config *DeduplicationConfig, logger *zap.Logger) *Deduplicator {
    d := &Deduplicator{
        config: config,
        cache:  make(map[string]*CacheEntry),
        logger: logger,
    }

    // Start background cleanup
    go d.startCleanup()

    return d
}

// ShouldSend determines if notification should be sent or deduplicated
func (d *Deduplicator) ShouldSend(notification *Notification) (bool, string) {
    if !d.config.Enabled {
        return true, "" // Deduplication disabled
    }

    fingerprint := d.generateFingerprint(notification)

    d.mu.Lock()
    defer d.mu.Unlock()

    entry, exists := d.cache[fingerprint]

    if !exists {
        // First time seeing this notification
        d.cache[fingerprint] = &CacheEntry{
            Fingerprint: fingerprint,
            LastSent:    time.Now(),
            Count:       0,
        }

        // Enforce cache size limit (LRU eviction)
        if len(d.cache) > d.config.MaxCacheEntries {
            d.evictOldest()
        }

        return true, "" // Send notification
    }

    // Check if TTL expired
    ttl := d.getTTL(notification.Alert.Severity)
    timeSinceLastSent := time.Since(entry.LastSent)

    if timeSinceLastSent > ttl {
        // TTL expired, can resend
        entry.LastSent = time.Now()
        entry.Count = 0

        d.logger.Info("notification deduplicated but TTL expired, sending",
            zap.String("fingerprint", fingerprint),
            zap.Duration("time_since_last_sent", timeSinceLastSent),
            zap.Duration("ttl", ttl))

        return true, "" // Send notification
    }

    // Duplicate within TTL window, block
    entry.Count++

    blockReason := fmt.Sprintf("Duplicate notification within %s (last sent %s ago, blocked %d times)",
        ttl, timeSinceLastSent.Round(time.Second), entry.Count)

    d.logger.Info("notification deduplicated",
        zap.String("fingerprint", fingerprint),
        zap.Duration("time_since_last_sent", timeSinceLastSent),
        zap.Duration("ttl", ttl),
        zap.Int("blocked_count", entry.Count))

    // Emit metric
    NotificationDeduplicatedTotal.WithLabelValues(notification.Alert.Severity).Inc()

    return false, blockReason // Block notification
}

func (d *Deduplicator) generateFingerprint(notification *Notification) string {
    // Build fingerprint from configured components
    var parts []string

    for _, component := range d.config.FingerprintComponents {
        switch component {
        case "alert_fingerprint":
            parts = append(parts, notification.Alert.Fingerprint)
        case "recipient":
            parts = append(parts, notification.Recipient)
        case "channels":
            parts = append(parts, strings.Join(notification.Channels, ","))
        }
    }

    // Hash the combined string
    combined := strings.Join(parts, ":")
    hash := sha256.Sum256([]byte(combined))
    return hex.EncodeToString(hash[:])
}

func (d *Deduplicator) getTTL(severity string) time.Duration {
    if ttl, exists := d.config.PerSeverity[severity]; exists {
        return time.Duration(ttl) * time.Second
    }
    return time.Duration(d.config.TTLSeconds) * time.Second
}

func (d *Deduplicator) evictOldest() {
    // Find oldest entry
    var oldestFingerprint string
    var oldestTime time.Time

    for fingerprint, entry := range d.cache {
        if oldestFingerprint == "" || entry.LastSent.Before(oldestTime) {
            oldestFingerprint = fingerprint
            oldestTime = entry.LastSent
        }
    }

    if oldestFingerprint != "" {
        delete(d.cache, oldestFingerprint)
        d.logger.Debug("evicted oldest cache entry",
            zap.String("fingerprint", oldestFingerprint),
            zap.Time("last_sent", oldestTime))
    }
}

func (d *Deduplicator) startCleanup() {
    ticker := time.NewTicker(time.Duration(d.config.CleanupIntervalSeconds) * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        d.cleanup()
    }
}

func (d *Deduplicator) cleanup() {
    d.mu.Lock()
    defer d.mu.Unlock()

    now := time.Now()
    deleted := 0

    for fingerprint, entry := range d.cache {
        ttl := time.Duration(d.config.TTLSeconds) * time.Second
        if now.Sub(entry.LastSent) > ttl*2 { // Keep for 2x TTL for safety
            delete(d.cache, fingerprint)
            deleted++
        }
    }

    if deleted > 0 {
        d.logger.Debug("cleaned up expired cache entries",
            zap.Int("deleted", deleted),
            zap.Int("remaining", len(d.cache)))
    }
}

// GetStats returns deduplication statistics
func (d *Deduplicator) GetStats() DeduplicationStats {
    d.mu.RLock()
    defer d.mu.RUnlock()

    totalBlocked := 0
    for _, entry := range d.cache {
        totalBlocked += entry.Count
    }

    return DeduplicationStats{
        CacheSize:    len(d.cache),
        TotalBlocked: totalBlocked,
    }
}

type DeduplicationStats struct {
    CacheSize    int
    TotalBlocked int
}
```

**Usage in Notification Service**:
```go
// pkg/notification/service.go
package notification

func (ns *NotificationService) SendEscalation(ctx context.Context, req *EscalationNotificationRequest) (*EscalationNotificationResponse, error) {
    // ... existing code ...

    // Check for duplicates
    shouldSend, blockReason := ns.deduplicator.ShouldSend(&Notification{
        Alert:      req.Payload.Alert,
        Recipient:  req.Recipient,
        Channels:   req.Channels,
    })

    if !shouldSend {
        ns.logger.Info("notification blocked by deduplication",
            zap.String("correlation_id", req.CorrelationID),
            zap.String("recipient", req.Recipient),
            zap.String("reason", blockReason))

        return &EscalationNotificationResponse{
            NotificationID: "",
            Status:         "deduplicated",
            BlockReason:    blockReason,
        }, nil
    }

    // ... continue with notification sending ...
}
```

**Prometheus Metrics**:
```go
var (
    NotificationDeduplicatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_notification_deduplicated_total",
        Help: "Total notifications blocked by deduplication",
    }, []string{"severity"})

    DeduplicationCacheSize = promauto.NewGauge(prometheus.GaugeOpts{
        Name: "kubernaut_notification_deduplication_cache_size",
        Help: "Current size of deduplication cache",
    })
)
```

**Confidence**: **90%** - Deduplication prevents spam and is well-tested pattern

---

### **HIGH-5: Label-Based Adapter Prioritization** ✅ **APPROVED**

#### **Solution**: Adapter Labeling + Severity-Based Prioritization

**ConfigMap Configuration**:
```yaml
# config/notification-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-config
  namespace: kubernaut-system
data:
  adapters.yaml: |
    adapters:
      slack:
        labels:
          - realtime       # Real-time delivery
          - interactive    # Supports interactive buttons
          - team-visible   # Visible to entire team
        priority: 10       # Base priority

      email:
        labels:
          - async          # Asynchronous delivery
          - reliable       # High reliability
          - detailed       # Supports rich content
        priority: 5

      teams:
        labels:
          - realtime
          - interactive
          - team-visible
        priority: 9

      sms:
        labels:
          - realtime
          - critical-only  # Only for critical alerts
          - concise        # Limited content
        priority: 15       # Highest priority for critical

      pagerduty:
        labels:
          - realtime
          - oncall         # Routes to on-call engineer
          - escalation     # Supports escalation policies
        priority: 20       # Highest for escalations

    # Severity-based adapter selection
    severity_adapter_preferences:
      critical:
        required_labels:
          - realtime       # Critical must be real-time
        preferred_order:
          - pagerduty      # 1st choice
          - sms            # 2nd choice
          - slack          # 3rd choice
          - teams          # 4th choice
          - email          # Last resort

      warning:
        required_labels:
          - realtime
        preferred_order:
          - slack
          - teams
          - email

      info:
        required_labels: []  # No requirements
        preferred_order:
          - email          # Email first (less disruptive)
          - slack
          - teams
```

**Go Implementation**:
```go
// pkg/notification/adapters/selector.go
package adapters

import (
    "fmt"
    "sort"
)

type AdapterConfig struct {
    Name     string
    Labels   []string
    Priority int
}

type SeverityPreferences struct {
    RequiredLabels []string
    PreferredOrder []string
}

type AdapterSelector struct {
    adapters            map[string]*AdapterConfig
    severityPreferences map[string]*SeverityPreferences
    logger              *zap.Logger
}

func NewAdapterSelector(config *AdapterSelectorConfig, logger *zap.Logger) *AdapterSelector {
    return &AdapterSelector{
        adapters:            config.Adapters,
        severityPreferences: config.SeverityPreferences,
        logger:              logger,
    }
}

// SelectAdapters returns ordered list of adapters for a given severity
func (as *AdapterSelector) SelectAdapters(severity string, requestedChannels []string) []string {
    prefs, exists := as.severityPreferences[severity]
    if !exists {
        // No preferences for this severity, use requested order
        return requestedChannels
    }

    // Filter adapters by required labels
    eligibleAdapters := as.filterByLabels(requestedChannels, prefs.RequiredLabels)

    if len(eligibleAdapters) == 0 {
        as.logger.Warn("no adapters match required labels, using all requested",
            zap.String("severity", severity),
            zap.Strings("required_labels", prefs.RequiredLabels),
            zap.Strings("requested_channels", requestedChannels))
        return requestedChannels
    }

    // Sort by preferred order
    orderedAdapters := as.sortByPreference(eligibleAdapters, prefs.PreferredOrder)

    as.logger.Info("selected adapters based on severity",
        zap.String("severity", severity),
        zap.Strings("ordered_adapters", orderedAdapters),
        zap.Strings("requested_channels", requestedChannels))

    return orderedAdapters
}

func (as *AdapterSelector) filterByLabels(channels []string, requiredLabels []string) []string {
    if len(requiredLabels) == 0 {
        return channels // No filtering needed
    }

    eligible := []string{}

    for _, channel := range channels {
        adapter, exists := as.adapters[channel]
        if !exists {
            continue
        }

        // Check if adapter has all required labels
        if as.hasAllLabels(adapter.Labels, requiredLabels) {
            eligible = append(eligible, channel)
        }
    }

    return eligible
}

func (as *AdapterSelector) hasAllLabels(adapterLabels, requiredLabels []string) bool {
    labelSet := make(map[string]bool)
    for _, label := range adapterLabels {
        labelSet[label] = true
    }

    for _, required := range requiredLabels {
        if !labelSet[required] {
            return false
        }
    }

    return true
}

func (as *AdapterSelector) sortByPreference(channels []string, preferredOrder []string) []string {
    // Create preference index map
    preferenceIndex := make(map[string]int)
    for i, channel := range preferredOrder {
        preferenceIndex[channel] = i
    }

    // Sort channels by preference
    sorted := make([]string, len(channels))
    copy(sorted, channels)

    sort.Slice(sorted, func(i, j int) bool {
        // Get preference indices (default to large number if not in preferences)
        indexI, existsI := preferenceIndex[sorted[i]]
        if !existsI {
            indexI = 1000
        }

        indexJ, existsJ := preferenceIndex[sorted[j]]
        if !existsJ {
            indexJ = 1000
        }

        // If same preference, use adapter priority
        if indexI == indexJ {
            adapterI := as.adapters[sorted[i]]
            adapterJ := as.adapters[sorted[j]]

            if adapterI != nil && adapterJ != nil {
                return adapterI.Priority > adapterJ.Priority // Higher priority first
            }
        }

        return indexI < indexJ // Lower index (earlier in preference list) first
    })

    return sorted
}

// GetAdapterInfo returns adapter configuration for debugging
func (as *AdapterSelector) GetAdapterInfo(channel string) *AdapterConfig {
    return as.adapters[channel]
}
```

**Usage in Notification Service**:
```go
// pkg/notification/service.go
package notification

func (ns *NotificationService) SendEscalation(ctx context.Context, req *EscalationNotificationRequest) (*EscalationNotificationResponse, error) {
    // ... existing code ...

    // Select and order adapters based on severity
    orderedChannels := ns.adapterSelector.SelectAdapters(
        req.Payload.Alert.Severity,
        req.Channels,
    )

    ns.logger.Info("adapter selection complete",
        zap.String("severity", req.Payload.Alert.Severity),
        zap.Strings("requested_channels", req.Channels),
        zap.Strings("ordered_channels", orderedChannels))

    // Try channels in prioritized order
    err := ns.SendWithRetryAndFallback(ctx, &NotificationRequest{
        Channels:      orderedChannels, // Use ordered channels
        Recipient:     req.Recipient,
        Payload:       req.Payload,
        CorrelationID: req.CorrelationID,
    })

    // ... continue ...
}
```

**Example Scenarios**:

**Scenario 1: Critical Alert**
```
Requested Channels: [slack, email, teams]
Severity: critical
Required Labels: [realtime]

Filtering:
  - slack: Has [realtime, interactive, team-visible] ✅
  - email: Has [async, reliable, detailed] ❌ (missing realtime)
  - teams: Has [realtime, interactive, team-visible] ✅

Eligible: [slack, teams]

Preferred Order (from config): [pagerduty, sms, slack, teams, email]
  - pagerduty: Not in eligible ❌
  - sms: Not in eligible ❌
  - slack: In eligible ✅ (1st)
  - teams: In eligible ✅ (2nd)

Final Order: [slack, teams]
```

**Scenario 2: Info Alert**
```
Requested Channels: [slack, email, teams]
Severity: info
Required Labels: [] (none)

Filtering: All eligible (no label requirements)
Eligible: [slack, email, teams]

Preferred Order (from config): [email, slack, teams]
  - email: In eligible ✅ (1st)
  - slack: In eligible ✅ (2nd)
  - teams: In eligible ✅ (3rd)

Final Order: [email, slack, teams]
```

**Prometheus Metrics**:
```go
var (
    AdapterSelectionTotal = promauto.NewCounterVec(prometheus.CounterOpts{
        Name: "kubernaut_notification_adapter_selection_total",
        Help: "Total adapter selections by severity and selected adapter",
    }, []string{"severity", "adapter", "position"}) // position: first, second, third
)
```

**Confidence**: **92%** - Label-based prioritization provides flexibility without over-complexity

---

## 📋 **DEFERRED TO V2**

### **HIGH-1: Async Progressive Notification** ⏳ **DEFERRED TO V2**
**Reason**: Adds complexity, UX benefit not essential for V1

### **HIGH-6: Notification Acknowledgment** ⏳ **DEFERRED TO V2**
**Reason**: Interactive features require webhook callbacks, not blocking for V1

### **HIGH-7: Localization/i18n Support** ⏳ **DEFERRED TO V2**
**Reason**: No immediate requirement for multi-language support

---

## 📊 **SUMMARY**

| Issue | Status | Confidence | Complexity |
|-------|--------|------------|------------|
| **HIGH-1** | ⏳ Deferred to V2 | N/A | High |
| **HIGH-2** | ✅ Approved & Solved | 95% | Low |
| **HIGH-3** | ✅ Clarified (uses CRITICAL-2) | 95% | Low |
| **HIGH-4** | ✅ Approved & Solved | 90% | Medium |
| **HIGH-5** | ✅ Approved & Solved | 92% | Medium |
| **HIGH-6** | ⏳ Deferred to V2 | N/A | High |
| **HIGH-7** | ⏳ Deferred to V2 | N/A | High |

**Overall V1 Readiness**: **94%** (3/7 approved and solved, 4/7 deferred)

---

## 🎯 **NEXT STEPS**

1. ✅ Review and approve these solutions
2. ✅ Update `06-notification-service.md` to include HIGH-2, HIGH-4, HIGH-5 solutions
3. ✅ Update business requirements (if needed) to reference these features
4. ⏳ Proceed to implementation phase

**Status**: ✅ **HIGH PRIORITY SOLUTIONS COMPLETE** with 94% confidence

