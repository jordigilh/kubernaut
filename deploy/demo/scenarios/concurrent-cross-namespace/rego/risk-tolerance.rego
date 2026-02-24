package signalprocessing.customlabels

import rego.v1

default labels := {}

labels := result if {
	rt := input.kubernetes.namespace.labels["kubernaut.ai/risk-tolerance"]
	rt != ""
	result := {"risk_tolerance": [rt]}
}
