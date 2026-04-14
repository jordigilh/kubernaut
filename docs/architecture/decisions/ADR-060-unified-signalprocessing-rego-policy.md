# ADR-060: Unified SignalProcessing Rego Policy

**Status**: Accepted  
**Date**: 2026-03-16  
**Issue**: [#415](https://github.com/jordigilh/kubernaut/issues/415)  
**Supersedes**: DD-INFRA-001 (partially), DD-SEVERITY-001 (partially), TRIAGE-SEVERITY-EXTENSIBILITY (partially)

## Context

The SignalProcessing controller used 5 separate Rego policy files, each loaded by a dedicated Go classifier:

| File | Package | Go Type |
|------|---------|---------|
| `environment.rego` | `signalprocessing.environment` | `classifier.EnvironmentClassifier` |
| `priority.rego` | `signalprocessing.priority` | `classifier.PriorityEngine` |
| `severity.rego` | `signalprocessing.severity` | `classifier.SeverityClassifier` |
| `business.rego` | `signalprocessing.business` | `classifier.BusinessClassifier` |
| `customlabels.rego` | `signalprocessing.customlabels` | `rego.Engine` |

This caused:

1. **Helm complexity**: Users had to provide a YAML bundle containing 5+ files via `--set-file`, parsed by `fromYaml` in the template. Error-prone and hard to document.
2. **Go sequencing dependency**: The priority classifier required the environment classifier's output as a Go parameter, coupling evaluation order in Go code rather than letting Rego handle it declaratively.
3. **5 hot-reload watchers**: Each classifier ran an independent `fsnotify` watcher, consuming 5 goroutines and 5 file descriptors.
4. **Dead code**: `BusinessClassifier` was wired in `main.go` but never called by the controller.

## Decision

Consolidate all classification rules into a single `policy.rego` file under `package signalprocessing`, evaluated by one Go `Evaluator` struct with 4 prepared OPA queries.

### Unified Input Schema

```go
type PolicyInput struct {
    Namespace sharedtypes.NamespaceContext `json:"namespace"`
    Signal    SignalInput                  `json:"signal"`
    Workload  sharedtypes.WorkloadDetails  `json:"workload"`
}
```

### Rego Structure

```rego
package signalprocessing
import rego.v1

# Environment (returns map)
default environment := {"environment": "unknown", "source": "default"}
environment := ... if { ... }

# Severity (returns string)
default severity := "unknown"
severity := "critical" if { ... }

# Priority references environment and severity internally
default priority := {"priority": "P3", "policy_name": "default"}
priority := {"priority": "P0", ...} if {
    environment.environment == "production"
    severity == "critical"
}

# Custom Labels (returns map)
default labels := {}
labels := ... if { ... }
```

### Go Evaluator

```
pkg/signalprocessing/evaluator/
├── evaluator.go   # Evaluator struct, LoadPolicy, 4 query methods, hot-reload
└── types.go       # PolicyInput, SignalInput, SeverityResult
```

Single `Evaluator` loads the policy once, prepares 4 queries (`data.signalprocessing.environment`, `.severity`, `.priority`, `.labels`), and exposes per-rule evaluation methods. One `hotreload.FileWatcher` reloads all queries atomically on file change.

### Helm Injection

```yaml
signalprocessing:
  policies:
    content: ""                   # --set-file signalprocessing.policies.content=policy.rego
    existingConfigMap: ""         # pre-created ConfigMap with key "policy.rego"
  proactiveSignalMappings:
    content: ""                   # --set-file (YAML, not Rego)
    existingConfigMap: ""
```

Proactive signal mappings are YAML (not Rego) and are injected separately.

## Consequences

### Positive

- **Simpler Helm UX**: One `--set-file` flag instead of a YAML bundle
- **No Go sequencing**: Priority references environment declaratively in Rego
- **Single hot-reload watcher**: 1 goroutine + 1 fd (was 5)
- **Dead code removed**: `BusinessClassifier` deleted (was wired but never called)
- **Unified PolicyHash**: Single SHA256 covers all rules (improved audit completeness)

### Negative

- **Hot-reload granularity loss**: Changing one rule requires replacing the entire file. Mitigated by clear documentation and structured examples with section headers.
- **Larger single file**: Operators manage one file instead of 5. Mitigated by section headers and the example file in `charts/kubernaut/examples/signalprocessing-policy.rego`.

## Files Changed

- **Created**: `pkg/signalprocessing/evaluator/{evaluator.go,types.go}`
- **Updated**: `internal/controller/signalprocessing/{interfaces.go,signalprocessing_controller.go}`, `cmd/signalprocessing/main.go`
- **Deleted**: `pkg/signalprocessing/classifier/{environment.go,priority.go,severity.go,business.go}`, `pkg/signalprocessing/rego/engine.go`
- **Helm**: `charts/kubernaut/values.yaml`, `templates/signalprocessing/signalprocessing.yaml`, `values.schema.json`
