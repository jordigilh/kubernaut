# Unified SignalProcessing Rego Policy (ADR-060)
#
# This single file contains all classification rules for the SignalProcessing controller.
# Deploy via Helm:
#   helm install kubernaut kubernaut/kubernaut \
#     --set-file signalprocessing.policy=signalprocessing-policy.rego
#
# Or create a ConfigMap and reference it:
#   kubectl create configmap signalprocessing-policy --from-file=policy.rego=signalprocessing-policy.rego
#   helm install kubernaut kubernaut/kubernaut \
#     --set signalprocessing.existingPolicyConfigMap=signalprocessing-policy
#
# Input schema (type-safe from Go):
#   input.namespace.name         string
#   input.namespace.labels       map[string]string
#   input.namespace.annotations  map[string]string
#   input.signal.severity        string
#   input.signal.type            string
#   input.signal.source          string
#   input.signal.labels          map[string]string
#   input.workload.kind          string
#   input.workload.name          string
#   input.workload.labels        map[string]string
#   input.workload.annotations   map[string]string

package signalprocessing

import rego.v1

# ========== Environment Classification (BR-SP-051-053) ==========
# Returns: {"environment": string, "source": string}
# Priority: namespace label > namespace name prefix > default

default environment := {"environment": "Unknown", "source": "default"}

# Normalize known tier names to PascalCase for output; pass through other label values (evaluator normalizes at boundary).
environment := {"environment": env_out, "source": "namespace-labels"} if {
    env := input.namespace.labels["kubernaut.ai/environment"]
    env != ""
    env_out := object.get(
        {"production": "Production", "staging": "Staging", "development": "Development", "test": "Test", "unknown": "Unknown"},
        lower(env),
        env,
    )
}
environment := {"environment": "Production", "source": "namespace-labels"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
    lower(input.namespace.labels["env"]) == "production"
}
environment := {"environment": "Staging", "source": "namespace-labels"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
    lower(input.namespace.labels["env"]) == "staging"
}
environment := {"environment": "Development", "source": "namespace-labels"} if {
    not input.namespace.labels["kubernaut.ai/environment"]
    lower(input.namespace.labels["env"]) == "development"
}

# ========== Severity Determination (BR-SP-105) ==========
# Returns: string (critical/high/medium/low/unknown)
# Maps external monitoring severity values to kubernaut-normalized values.
# Add else clauses for your monitoring tool's severity scheme.

default severity := "unknown"

severity := "critical" if { lower(input.signal.severity) == "critical" }
severity := "critical" if { lower(input.signal.severity) == "sev1" }
severity := "critical" if { lower(input.signal.severity) == "p0" }
severity := "high" if { lower(input.signal.severity) == "high" }
severity := "high" if { lower(input.signal.severity) == "sev2" }
severity := "high" if { lower(input.signal.severity) == "p2" }
severity := "medium" if { lower(input.signal.severity) == "medium" }
severity := "medium" if { lower(input.signal.severity) == "warning" }
severity := "medium" if { lower(input.signal.severity) == "sev3" }
severity := "low" if { lower(input.signal.severity) == "low" }
severity := "low" if { lower(input.signal.severity) == "info" }
severity := "low" if { lower(input.signal.severity) == "sev4" }

# ========== Priority Assignment (BR-SP-070) ==========
# Returns: {"priority": string, "policy_name": string}
# References `environment` and `severity` rules above -- Rego resolves internally.

default priority := {"priority": "P3", "policy_name": "default"}

priority := {"priority": "P0", "policy_name": "production-critical"} if {
    environment.environment == "Production"
    severity == "critical"
}
priority := {"priority": "P1", "policy_name": "production-high"} if {
    environment.environment == "Production"
    severity == "high"
}
priority := {"priority": "P1", "policy_name": "staging-critical"} if {
    environment.environment == "Staging"
    severity == "critical"
}
priority := {"priority": "P2", "policy_name": "staging-any"} if {
    environment.environment == "Staging"
    severity != "critical"
}

# ========== Custom Labels (BR-SP-102) ==========
# Returns: map[string][]string
# Extract operator-defined labels from namespace metadata.
# Reserved prefixes ("kubernaut.ai/", "system/") are stripped by Go after evaluation.

default labels := {}

labels := result if {
    team := input.namespace.labels["kubernaut.ai/team"]
    team != ""
    tier := object.get(input.namespace.labels, "kubernaut.ai/tier", "")
    result := object.union(
        {"team": [team]},
        {"tier": [tier]} if { tier != "" } else {}
    )
}
