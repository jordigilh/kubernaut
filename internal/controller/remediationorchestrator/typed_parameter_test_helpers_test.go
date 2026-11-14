package controller_test

import (
	"encoding/json"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// typedParametersFromStrings keeps legacy fixture values as raw JSON tokens
// while constructing the typed contract used by the disposable spike.
func typedParametersFromStrings(values map[string]string) sharedtypes.TypedParameterMap {
	if values == nil {
		return nil
	}
	result := make(sharedtypes.TypedParameterMap, len(values))
	for name, rawValue := range values {
		encodedName, err := json.Marshal(name)
		if err != nil {
			panic(err)
		}
		raw := []byte{'{'}
		raw = append(raw, encodedName...)
		raw = append(raw, ':')
		raw = append(raw, rawValue...)
		raw = append(raw, '}')
		decoded, _, err := sharedtypes.DecodeTypedParameterMap(raw)
		if err == nil {
			result[name] = decoded[name]
			continue
		}
		value := rawValue
		result[name] = sharedtypes.TypedParameterValue{Kind: "string", StringValue: &value}
	}
	return result
}

func typedParameterValue(rawValue string) sharedtypes.TypedParameterValue {
	return typedParametersFromStrings(map[string]string{"value": rawValue})["value"]
}
