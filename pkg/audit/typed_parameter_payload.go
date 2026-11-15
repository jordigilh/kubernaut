package audit

import (
	"encoding/json"
	"fmt"

	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// ToOgenParameterMap converts the shared persisted parameter union to the
// generated DataStorage audit payload union. It is a transport adapter only;
// callers retain the original typed map and parameter evidence separately.
func ToOgenParameterMap(values sharedtypes.TypedParameterMap) (map[string]api.WorkflowParameterValue, error) {
	if values == nil {
		//nolint:nilnil // nil preserves the absent optional-map contract.
		return nil, nil
	}
	result := make(map[string]api.WorkflowParameterValue, len(values))
	for name, value := range values {
		converted, err := ToOgenParameterValue(value)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: %w", name, err)
		}
		result[name] = converted
	}
	return result, nil
}

// ToOgenParameterValue converts one shared typed parameter value to its
// generated audit representation without passing through float64 or strings.
func ToOgenParameterValue(value sharedtypes.TypedParameterValue) (api.WorkflowParameterValue, error) {
	if _, err := json.Marshal(sharedtypes.TypedParameterMap{"value": value}); err != nil {
		return api.WorkflowParameterValue{}, fmt.Errorf("invalid typed parameter: %w", err)
	}
	switch value.Kind {
	case "string":
		if value.StringValue == nil {
			return api.WorkflowParameterValue{}, fmt.Errorf("string value is missing")
		}
		return api.NewWorkflowParameterStringWorkflowParameterValue(api.WorkflowParameterString{
			Kind:        api.WorkflowParameterStringKindString,
			StringValue: *value.StringValue,
		}), nil
	case "integer":
		if value.IntegerValue == nil {
			return api.WorkflowParameterValue{}, fmt.Errorf("integer value is missing")
		}
		return api.NewWorkflowParameterIntegerWorkflowParameterValue(api.WorkflowParameterInteger{
			Kind:         api.WorkflowParameterIntegerKindInteger,
			IntegerValue: *value.IntegerValue,
		}), nil
	case "float":
		if value.FloatValue == nil {
			return api.WorkflowParameterValue{}, fmt.Errorf("float value is missing")
		}
		return api.NewWorkflowParameterFloatWorkflowParameterValue(api.WorkflowParameterFloat{
			Kind:       api.WorkflowParameterFloatKindFloat,
			FloatValue: *value.FloatValue,
		}), nil
	case "boolean":
		if value.BooleanValue == nil {
			return api.WorkflowParameterValue{}, fmt.Errorf("boolean value is missing")
		}
		return api.NewWorkflowParameterBooleanWorkflowParameterValue(api.WorkflowParameterBoolean{
			Kind:         api.WorkflowParameterBooleanKindBoolean,
			BooleanValue: *value.BooleanValue,
		}), nil
	case "array":
		converted := make([]api.WorkflowParameterPrimitive, 0, len(value.ArrayValue))
		for index, item := range value.ArrayValue {
			primitive, err := toOgenPrimitive(item)
			if err != nil {
				return api.WorkflowParameterValue{}, fmt.Errorf("array item %d: %w", index, err)
			}
			converted = append(converted, primitive)
		}
		return api.NewWorkflowParameterArrayWorkflowParameterValue(api.WorkflowParameterArray{
			Kind:       api.WorkflowParameterArrayKindArray,
			ArrayValue: converted,
		}), nil
	default:
		return api.WorkflowParameterValue{}, fmt.Errorf("unsupported kind %q", value.Kind)
	}
}

func toOgenPrimitive(value sharedtypes.TypedPrimitiveParameter) (api.WorkflowParameterPrimitive, error) {
	switch value.Kind {
	case "string":
		if value.StringValue == nil {
			return api.WorkflowParameterPrimitive{}, fmt.Errorf("string value is missing")
		}
		return api.NewWorkflowParameterStringWorkflowParameterPrimitive(api.WorkflowParameterString{
			Kind:        api.WorkflowParameterStringKindString,
			StringValue: *value.StringValue,
		}), nil
	case "integer":
		if value.IntegerValue == nil {
			return api.WorkflowParameterPrimitive{}, fmt.Errorf("integer value is missing")
		}
		return api.NewWorkflowParameterIntegerWorkflowParameterPrimitive(api.WorkflowParameterInteger{
			Kind:         api.WorkflowParameterIntegerKindInteger,
			IntegerValue: *value.IntegerValue,
		}), nil
	case "float":
		if value.FloatValue == nil {
			return api.WorkflowParameterPrimitive{}, fmt.Errorf("float value is missing")
		}
		return api.NewWorkflowParameterFloatWorkflowParameterPrimitive(api.WorkflowParameterFloat{
			Kind:       api.WorkflowParameterFloatKindFloat,
			FloatValue: *value.FloatValue,
		}), nil
	case "boolean":
		if value.BooleanValue == nil {
			return api.WorkflowParameterPrimitive{}, fmt.Errorf("boolean value is missing")
		}
		return api.NewWorkflowParameterBooleanWorkflowParameterPrimitive(api.WorkflowParameterBoolean{
			Kind:         api.WorkflowParameterBooleanKindBoolean,
			BooleanValue: *value.BooleanValue,
		}), nil
	default:
		return api.WorkflowParameterPrimitive{}, fmt.Errorf("unsupported primitive kind %q", value.Kind)
	}
}
