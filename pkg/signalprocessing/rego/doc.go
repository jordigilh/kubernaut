// Package rego provides OPA Rego policy evaluation for Signal Processing.
//
// # Purpose
//
// This package contains two Rego evaluation components:
//
// 1. Engine (engine.go) - General-purpose Rego policy evaluation
// 2. CustomLabelsExtractor (extractor.go) - CustomLabels extraction with security
//
// # Business Requirements
//
// BR-SP-080: CustomLabels support
// BR-SP-102: CustomLabels Rego Extraction
// BR-SP-103: CustomLabels Validation Limits
//
// # Design Decisions
//
// DD-WORKFLOW-001 v1.9: Security wrapper and validation limits
//
// # Components
//
// ## Engine (General Purpose)
//
// The Engine provides low-level Rego policy evaluation with:
//   - Prepared query compilation for performance
//   - Timeout enforcement (default 5s)
//   - Result conversion to map[string][]string
//   - Security wrapper option to strip system labels
//
// ## CustomLabelsExtractor (ConfigMap-based)
//
// The CustomLabelsExtractor provides high-level CustomLabels extraction:
//   - Loads policies from ConfigMap "signal-processing-policies" in "kubernaut-system"
//   - Applies validation limits (max keys, values, lengths)
//   - Strips reserved prefixes (kubernaut.ai/, system/)
//   - Hot-reload support via LoadPolicy()
//
// # Security Model
//
// System labels that cannot be overridden by customer Rego policies:
//   - environment (set by EnvironmentClassifier)
//   - priority (set by PriorityClassifier)
//   - severity (from signal source)
//   - namespace (from K8s context)
//   - service (from K8s context)
//
// Reserved prefixes that are stripped:
//   - kubernaut.ai/ (internal system use)
//   - system/ (reserved for system labels)
//
// # Validation Limits (DD-WORKFLOW-001 v1.9)
//
//   - Max 10 keys (subdomains) per extraction
//   - Max 5 values per key
//   - Max 63 character key length (K8s label compatibility)
//   - Max 100 character value length (prompt efficiency)
//
// # Usage - Engine
//
//	engine, err := rego.NewEngine(policyContent, "data.mypackage.result", logger)
//	if err != nil {
//	    return err
//	}
//	result, err := engine.Evaluate(ctx, inputMap)
//
// # Usage - CustomLabelsExtractor
//
//	extractor := rego.NewCustomLabelsExtractor(k8sClient, logger)
//	if err := extractor.LoadPolicy(ctx); err != nil {
//	    return err
//	}
//
//	input := &rego.LabelInput{
//	    Namespace: rego.NamespaceContext{
//	        Name:   "default",
//	        Labels: namespaceLabels,
//	    },
//	}
//	customLabels, err := extractor.Extract(ctx, input)
//
// # ConfigMap Format
//
// The CustomLabelsExtractor expects a ConfigMap with this structure:
//
//	apiVersion: v1
//	kind: ConfigMap
//	metadata:
//	  name: signal-processing-policies
//	  namespace: kubernaut-system
//	data:
//	  labels.rego: |
//	    package signalprocessing.labels
//
//	    labels["team"] := value if {
//	        value := [concat("=", ["name", input.namespace.labels.team])]
//	        input.namespace.labels.team
//	    }
//
// # Thread Safety
//
// Both Engine and CustomLabelsExtractor are safe for concurrent use.
// Each evaluation call is independent.
package rego

