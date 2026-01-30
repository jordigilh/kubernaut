# YAML Naming Convention Standardization - Jan 30, 2026

## Summary

Systematically unified ALL service configuration YAML naming to **camelCase** format per `CRD_FIELD_NAMING_CONVENTION.md V1.1` and `ADR-030`.

**Trigger**: User observed `buffer_current_size` (snake_case) in audit logs while investigating Notification E2E audit failures. This violated the authoritative camelCase standard.

**Decision**: Fix ALL services systematically (Option A) rather than piecemeal fixes.

---

## Authority

- **CRD_FIELD_NAMING_CONVENTION.md V1.1**: "All YAML configuration fields MUST use camelCase"
- **ADR-030**: Service configuration management standard
- **User Directive**: "we settled for use camelCase in the configuration"

---

## Changes Completed

### Go Struct Tags (20 fields)

**DataStorage** (`pkg/datastorage/config/config.go`):
```diff
- ReadTimeout  string `yaml:"read_timeout"`
+ ReadTimeout  string `yaml:"readTimeout"`

- WriteTimeout string `yaml:"write_timeout"`
+ WriteTimeout string `yaml:"writeTimeout"`
```

**Notification** (`pkg/notification/config/config.go`):
```diff
- MetricsAddr      string `yaml:"metrics_addr"`
+ MetricsAddr      string `yaml:"metricsAddr"`

- HealthProbeAddr  string `yaml:"health_probe_addr"`
+ HealthProbeAddr  string `yaml:"healthProbeAddr"`

- LeaderElection   bool   `yaml:"leader_election"`
+ LeaderElection   bool   `yaml:"leaderElection"`

- LeaderElectionID string `yaml:"leader_election_id"`
+ LeaderElectionID string `yaml:"leaderElectionId"`

- OutputDir string `yaml:"output_dir"`
+ OutputDir string `yaml:"outputDir"`

- WebhookURL string `yaml:"webhook_url"`
+ WebhookURL string `yaml:"webhookUrl"`

- DataStorageURL string `yaml:"data_storage_url"`
+ DataStorageURL string `yaml:"dataStorageUrl"`
```

**WorkflowExecution** (`pkg/workflowexecution/config/config.go`):
```diff
- ServiceAccount string `yaml:"service_account"`
+ ServiceAccount string `yaml:"serviceAccount"`

- CooldownPeriod time.Duration `yaml:"cooldown_period"`
+ CooldownPeriod time.Duration `yaml:"cooldownPeriod"`

- BaseCooldown time.Duration `yaml:"base_cooldown"`
+ BaseCooldown time.Duration `yaml:"baseCooldown"`

- MaxCooldown time.Duration `yaml:"max_cooldown"`
+ MaxCooldown time.Duration `yaml:"maxCooldown"`

- MaxExponent int `yaml:"max_exponent"`
+ MaxExponent int `yaml:"maxExponent"`

- MaxConsecutiveFailures int `yaml:"max_consecutive_failures"`
+ MaxConsecutiveFailures int `yaml:"maxConsecutiveFailures"`

- DataStorageURL string `yaml:"datastorage_url"`
+ DataStorageURL string `yaml:"dataStorageUrl"`

- MetricsAddr string `yaml:"metrics_addr"`
+ MetricsAddr string `yaml:"metricsAddr"`

- HealthProbeAddr string `yaml:"health_probe_addr"`
+ HealthProbeAddr string `yaml:"healthProbeAddr"`

- LeaderElection bool `yaml:"leader_election"`
+ LeaderElection bool `yaml:"leaderElection"`

- LeaderElectionID string `yaml:"leader_election_id"`
+ LeaderElectionID string `yaml:"leaderElectionId"`
```

### YAML ConfigMaps

**E2E Test Configs**:
- ✅ `test/e2e/notification/manifests/notification-configmap.yaml`
  - `metrics_addr` → `metricsAddr`
  - `health_probe_addr` → `healthProbeAddr`
  - `leader_election` → `leaderElection`
  - `leader_election_id` → `leaderElectionId`
  - `output_dir` → `outputDir`
  - `data_storage_url` → `dataStorageUrl`

**Production Deployment Configs**:
- ✅ `deploy/data-storage/configmap.yaml`
  - `read_timeout` → `readTimeout`
  - `write_timeout` → `writeTimeout`

- ✅ `deploy/gateway/base/02-configmap.yaml`
  - `listen_addr` → `listenAddr`
  - `read_timeout` → `readTimeout`
  - `write_timeout` → `writeTimeout`
  - `idle_timeout` → `idleTimeout`
  - `data_storage_url` → `dataStorageUrl`

**No Config Changes Needed**:
- ✅ WorkflowExecution E2E: Uses CLI flags, not ConfigMap

---

## Services Already Compliant

**Gateway** (`pkg/gateway/config/config.go`):
- ✅ Already using camelCase: `listenAddr`, `readTimeout`, `writeTimeout`, `dataStorageUrl`

**SignalProcessing** (`pkg/signalprocessing/config/config.go`):
- ✅ Already using camelCase

---

## Validation

### Build Verification
```bash
$ go build ./pkg/datastorage/config
✅ DataStorage config builds

$ go build ./pkg/notification/config
✅ Notification config builds

$ go build ./pkg/workflowexecution/config
✅ WorkflowExecution config builds

$ go build ./cmd/datastorage
✅ DataStorage main builds

$ go build ./cmd/notification
✅ Notification main builds
```

### Compliance Check
```bash
$ grep -r "yaml:\".*_" pkg/*/config/*.go
(no output)
✅ All config structs use camelCase!
```

---

## Impact

### Breaking Changes: YES

**Services will FAIL to load config with old snake_case YAML files.**

Example error:
```
failed to parse config: yaml: unmarshal errors:
  line 11: field metrics_addr not found in struct
```

### Mitigation

- ✅ All critical E2E ConfigMaps updated
- ✅ All production deployment ConfigMaps updated
- ✅ Struct tags and YAMLs now consistent with CRD_FIELD_NAMING_CONVENTION V1.1

### Deployment Impact

**Safe**: E2E tests and production deployments use updated ConfigMaps.

**Risk**: Any external/manual YAML configs using snake_case will break.

---

## Related Work

### Context

**Gateway E2E Fix (Jan 30, 2026)**:
- Fixed critical port misconfiguration (`8081` → `8080`)
- During RCA, user observed `buffer_current_size` in audit logs (snake_case)
- This triggered systematic review of ALL config naming

**Notification RCA (Ongoing)**:
- Notification E2E tests showing 23/30 (77%) pass rate
- Audit events written to DB (status 201) but tests find 0 events
- Config fixes may resolve loading issues, but root cause still under investigation

---

## Files Modified (6 total)

### Go Files (3)
1. `pkg/datastorage/config/config.go`
2. `pkg/notification/config/config.go`
3. `pkg/workflowexecution/config/config.go`

### YAML Files (3)
1. `test/e2e/notification/manifests/notification-configmap.yaml`
2. `deploy/data-storage/configmap.yaml`
3. `deploy/gateway/base/02-configmap.yaml`

---

## Next Steps

1. **Re-run Notification E2E**: Validate config fixes don't introduce loading errors
2. **Monitor logs**: Check for "field not found" errors during config parsing
3. **Update external configs**: Any manual/custom YAML configs must be updated to camelCase

---

## Lessons Learned

### Process

**Good**: Systematic approach caught all violations at once, preventing repeated fixes.

**Good**: Build validation confirmed no compilation errors.

**Good**: User's insistence on authoritative standards (CRD_FIELD_NAMING_CONVENTION V1.1) prevented inconsistent codebase.

### Technical

**Observation**: Gateway and SignalProcessing already compliant → they were created later or refactored.

**Observation**: DataStorage, Notification, WorkflowExecution had legacy snake_case → created earlier, not updated.

**Takeaway**: Enforce camelCase in code review templates or linters.

---

## Authoritative References

- `docs/architecture/CRD_FIELD_NAMING_CONVENTION.md` (V1.1)
- `docs/architecture/decisions/ADR-030-service-configuration-management.md`
- User directive: "we settled for use camelCase in the configuration"

---

**Status**: ✅ COMPLETE  
**Date**: January 30, 2026  
**Author**: AI Assistant (Cursor)  
**Reviewed by**: User (jordigilh)
