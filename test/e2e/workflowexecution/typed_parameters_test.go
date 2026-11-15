package workflowexecution

import (
	"strings"

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
// the direct-execution fixtures. The target resource format is
// namespace/kind/name, as used by WorkflowExecution.Spec.TargetResource.
func typedParametersForTarget(targetResource string, values map[string]string) sharedtypes.TypedParameterMap {
	params := make(map[string]string, len(values)+3)
	for key, value := range values {
		params[key] = value
	}
	parts := strings.Split(targetResource, "/")
	if len(parts) == 3 {
		kind := parts[1]
		if kind != "" {
			kind = strings.ToUpper(kind[:1]) + kind[1:]
		}
		params["TARGET_RESOURCE_NAMESPACE"] = parts[0]
		params["TARGET_RESOURCE_KIND"] = kind
		params["TARGET_RESOURCE_NAME"] = parts[2]
	}
	return typedStringParameters(params)
}
