package tools_test

import (
	"encoding/json"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

func typedParameters(raw string) sharedtypes.TypedParameterMap {
	values, _, err := sharedtypes.DecodeTypedParameterMap([]byte(raw))
	if err != nil {
		panic(err)
	}
	return values
}

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
		// Keep legacy invalid-value vectors representable in this in-memory test
		// helper. Production ordinary-JSON boundaries reject them before the
		// typed union is constructed.
		value := rawValue
		result[name] = sharedtypes.TypedParameterValue{Kind: "string", StringValue: &value}
	}
	return result
}
