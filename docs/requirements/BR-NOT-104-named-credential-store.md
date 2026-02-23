# BR-NOT-104: Named Credential Store for Per-Receiver Delivery

**Service**: Notification Controller
**Category**: Credential Management
**Priority**: P0 (CRITICAL)
**Version**: 1.0
**Date**: 2026-02-20
**Status**: Approved
**Related ADRs**: ADR-030 (Configuration Management)
**Related BRs**: BR-NOT-065 (Channel Routing), BR-NOT-067 (Hot-Reload)
**Issue**: [#104](https://github.com/jordigilh/kubernaut/issues/104)

---

## Overview

The Notification Service MUST decouple adapter credentials from delivery configuration by introducing a named credential store backed by Kubernetes Projected Volumes. Each receiver's `credentialRef` is resolved at routing config reload time against credential files mounted from individual Kubernetes Secrets into a shared directory.

**Business Value**: Enables per-receiver Slack webhook URLs (multi-channel alerting), credential rotation without pod restart, and eliminates the single-global-webhook limitation that prevents SREs from routing different alert severities to different Slack channels.

---

## BR-NOT-104-001: Named Credential Resolution

### Description

The system MUST provide a `CredentialResolver` that resolves named credential references to their secret values by reading files from a configured credentials directory. Each file in the directory represents one credential: the filename is the credential name and the file content (trimmed of leading/trailing whitespace) is the secret value.

### Acceptance Criteria

- `Resolve(name)` returns the credential value when a file with that name exists in the credentials directory
- `Resolve(name)` returns a descriptive error when no file with that name exists
- File content is trimmed of leading/trailing whitespace and newlines before returning
- The resolver caches credential values in memory for fast repeated lookups

---

## BR-NOT-104-002: Credential Hot-Reload via fsnotify

### Description

The system MUST detect changes to credential files (additions, modifications, deletions) in real-time using filesystem notifications and update its in-memory cache accordingly, without requiring a pod restart.

### Acceptance Criteria

- New credential files added to the directory become resolvable within 5 seconds
- Modified credential file content is reflected in subsequent `Resolve()` calls within 5 seconds
- Deleted credential files are no longer resolvable after deletion
- `Reload()` can be called manually to force a full re-read of the credentials directory

---

## BR-NOT-104-003: Credential Reference Validation (Fail-Fast)

### Description

When routing configuration is loaded or reloaded, the system MUST validate that all `credentialRef` values referenced by receivers can be resolved. If any credential reference is unresolvable, the new routing configuration MUST be rejected and the previous valid configuration preserved.

### Acceptance Criteria

- `ValidateRefs(refs)` returns `nil` when all references resolve successfully
- `ValidateRefs(refs)` returns an error listing ALL unresolvable references (not just the first)
- Routing config reload with unresolvable `credentialRef` preserves the previous valid routing configuration
- A log entry is emitted for each unresolvable credential reference

---

## BR-NOT-104-004: Per-Receiver Delivery Binding

### Description

Each receiver's Slack configuration MUST specify a `credentialRef` that resolves to a Slack webhook URL. At routing config reload time, the system creates a dedicated `SlackDeliveryService` instance per receiver, bound to the resolved webhook URL. The delivery orchestrator registers these services with receiver-qualified keys (`"slack:<receiver-name>"`).

### Acceptance Criteria

- `SlackConfig.CredentialRef` is the sole mechanism for specifying Slack webhook URLs (no `api_url` or environment variable fallback)
- Each receiver with `SlackConfigs` gets its own `SlackDeliveryService` instance
- Orchestrator registration uses receiver-qualified keys (e.g., `"slack:sre-critical"`)
- `receiverToChannels()` returns receiver-qualified channel names for correct delivery routing

---

## BR-NOT-104-005: Credential Rotation

### Description

When a credential file is updated (e.g., Kubernetes Secret rotation propagated to the projected volume), subsequent deliveries MUST use the updated credential value without requiring any configuration reload or pod restart.

### Acceptance Criteria

- Credential file update triggers resolver cache refresh via fsnotify
- Next `Resolve()` call after cache refresh returns the updated value
- In-flight deliveries using the old credential complete normally
- New deliveries use the rotated credential value

---

## Breaking Changes

This requirement introduces the following breaking changes (acceptable since the project has not been released):

- `SlackSettings.WebhookURL` removed from `pkg/notification/config/config.go`
- `SLACK_WEBHOOK_URL` environment variable no longer supported
- `SlackConfig.APIURL` replaced by `SlackConfig.CredentialRef` in routing configuration
- Orchestrator channel keys change from `"slack"` to `"slack:<receiver-name>"`
