package remediationorchestrator

import sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

func typedStringParameters(values map[string]string) sharedtypes.TypedParameterMap {
	result := make(sharedtypes.TypedParameterMap, len(values))
	for key, value := range values {
		v := value
		result[key] = sharedtypes.TypedParameterValue{Kind: "string", StringValue: &v}
	}
	return result
}
