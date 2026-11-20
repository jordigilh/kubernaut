package workflowexecution

import (
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

func typedStringParameters(values map[string]string) sharedtypes.TypedParameterMap {
	result := make(sharedtypes.TypedParameterMap, len(values))
	for key, value := range values {
		v := value
		result[key] = sharedtypes.TypedParameterValue{Kind: "string", StringValue: &v}
	}
	return result
}

// typedParametersForTarget adds the system-injected target fields declared by
// the direct-execution fixtures. The target resource format is the canonical
// identity (#2658), namespace/group/version/Kind/name (cluster-scoped:
// group/version/Kind/name), as used by WorkflowExecution.Spec.TargetResource.
func typedParametersForTarget(targetResource string, values map[string]string) sharedtypes.TypedParameterMap {
	params := make(map[string]string, len(values)+3)
	for key, value := range values {
		params[key] = value
	}
	// Parse the canonical identity so the part-params match the ones the
	// direct-execution fixtures declare, derived from the same canonical
	// target (a bare split would read the group slot as the Kind).
	if _, namespace, kind, name, err := scope.ParseTargetIdentity(targetResource); err == nil {
		params["TARGET_RESOURCE_NAMESPACE"] = namespace
		params["TARGET_RESOURCE_KIND"] = kind
		params["TARGET_RESOURCE_NAME"] = name
	}
	return typedStringParameters(params)
}
