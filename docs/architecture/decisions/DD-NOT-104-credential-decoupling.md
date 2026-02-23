# DD-NOT-104: Credential Decoupling via Named Credential Store

**Status**: Accepted
**Date**: 2026-02-20
**Author**: AI Assistant
**Deciders**: Jordi Gil
**Related BR**: BR-NOT-104

---

## Context

The current notification service uses a single global `SlackSettings.WebhookURL` loaded from the `SLACK_WEBHOOK_URL` environment variable (ADR-030 secret management). The routing configuration defines `SlackConfig.APIURL` per receiver, but this value is dead code -- all Slack deliveries use the single global webhook.

This means:
- All Slack notifications go to one channel regardless of routing rules
- Credential rotation requires a pod restart (env var reload)
- SREs cannot route different alert severities to different Slack channels

Issue #104 requires decoupling credentials from delivery so that each receiver can use a distinct Slack webhook URL.

---

## Decision

### Pattern: File-Based Credential Resolver with Projected Volumes

Introduce a `CredentialResolver` (`pkg/notification/credentials/resolver.go`) that:

1. Reads credential files from a configurable directory (default: `/etc/notification/credentials/`)
2. Caches values in memory with a `sync.RWMutex` for thread-safe access
3. Watches the directory for changes using `fsnotify` (same library used by `pkg/shared/hotreload/file_watcher.go`)
4. Provides `ValidateRefs(refs)` for fail-fast credential validation at routing config reload time

### Credential File Convention

Each Kubernetes Secret is projected into the credentials directory as a single file:
- **Filename**: credential name (e.g., `slack-sre-critical`)
- **Content**: secret value (e.g., `https://hooks.slack.com/services/T.../B.../xxx`)

```yaml
# Example Kubernetes configuration (deferred to deploy/demo)
volumes:
  - name: credentials
    projected:
      sources:
        - secret:
            name: slack-sre-critical
        - secret:
            name: slack-platform-alerts
```

### Routing Config: credentialRef Only

Replace `SlackConfig.APIURL` with `SlackConfig.CredentialRef`:

```yaml
receivers:
  - name: sre-critical
    slackConfigs:
      - channel: "#sre-critical"
        credentialRef: slack-sre-critical
  - name: platform-alerts
    slackConfigs:
      - channel: "#platform-alerts"
        credentialRef: slack-platform-alerts
```

No fallback chain. `credentialRef` is the sole mechanism (breaking change allowed since project is unreleased).

### Receiver-Qualified Orchestrator Keys

The delivery orchestrator changes from a single `"slack"` channel to receiver-qualified keys:

- Before: `orchestrator.RegisterChannel("slack", slackService)`
- After: `orchestrator.RegisterChannel("slack:sre-critical", slackServiceA)`
- After: `orchestrator.RegisterChannel("slack:platform-alerts", slackServiceB)`

The `receiverToChannels()` function returns qualified names (e.g., `"slack:sre-critical"`) instead of bare channel types.

### Credential Reload Flow

```
1. fsnotify detects file change in credentials directory
2. CredentialResolver.Reload() re-reads all files, updates cache
3. Next routing config reload validates refs against updated cache
4. Per-receiver SlackDeliveryService instances created with resolved URLs
5. Orchestrator re-registered with new services
```

### Config Changes

- Remove `SlackSettings.WebhookURL` from `pkg/notification/config/config.go`
- Remove `LoadFromEnv()` for `SLACK_WEBHOOK_URL`
- Add `CredentialsSettings` struct with `Dir string` to `DeliverySettings` (default: `/etc/notification/credentials/`)

### main.go Wiring Changes

- Initialize `credentials.NewResolver(cfg.Delivery.Credentials.Dir, logger)` at startup
- Start `resolver.StartWatching(ctx)` before the manager starts for fsnotify monitoring
- Remove single `SlackDeliveryService` creation at startup
- Per-receiver services created on routing config load (not at startup)
- Pass resolver to the reconciler for use during `loadRoutingConfigFromCluster()`

---

## Alternatives Considered

### A. Reuse `pkg/shared/hotreload/file_watcher.go`

The existing `FileWatcher` watches a single file and calls a callback on changes. The credential resolver needs to watch a directory and manage multiple files. While the fsnotify patterns are similar, the abstractions differ enough that a dedicated resolver is cleaner. The resolver may internally reuse fsnotify patterns from `FileWatcher` during REFACTOR.

### B. Store credentials in ConfigMap alongside routing config

Rejected: violates ADR-030 (secrets must not be in ConfigMaps).

### C. Maintain `api_url` + `credentialRef` fallback chain

Rejected per user direction: `credentialRef` only, no backward compatibility needed.

---

## Consequences

### Positive
- Per-receiver Slack webhook URLs enable multi-channel alerting
- Credential rotation without pod restart via fsnotify
- Clean separation of secrets (projected volumes) from config (ConfigMaps)
- Fail-fast validation prevents routing misconfigurations

### Negative
- Breaking changes to config format and environment variable handling
- Additional complexity in routing config reload (credential validation step)
- Filesystem dependency for credential storage (mitigated by Kubernetes projected volumes)

### Risks
- `receiverToChannels()` return type change ripples into reconciler and delivery orchestrator
- TDD RED phase will surface all affected call sites through compilation failures
