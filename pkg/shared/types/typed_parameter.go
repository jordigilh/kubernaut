/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

const (
	typedParameterKindString  = "string"
	typedParameterKindInteger = "integer"
	typedParameterKindFloat   = "float"
	typedParameterKindBoolean = "boolean"
	typedParameterKindArray   = "array"

	persistedFieldString  = "stringValue"
	persistedFieldInteger = "integerValue"
	persistedFieldFloat   = "floatValue"
	persistedFieldBoolean = "booleanValue"
	persistedFieldArray   = "arrayValue"
)

// TypedPrimitiveParameter is one primitive element of a workflow parameter
// array. Arrays deliberately cannot contain arrays or objects.
// +kubebuilder:validation:XValidation:rule="(self.kind == 'string') == has(self.stringValue) && (self.kind == 'integer') == has(self.integerValue) && (self.kind == 'float') == has(self.floatValue) && (self.kind == 'boolean') == has(self.booleanValue)",message="kind must match exactly one primitive value"
type TypedPrimitiveParameter struct {
	// +kubebuilder:validation:Enum=string;integer;float;boolean
	Kind string `json:"kind"`
	// +optional
	StringValue *string `json:"stringValue,omitempty"`
	// +optional
	IntegerValue *int64 `json:"integerValue,omitempty"`
	// +optional
	FloatValue *float64 `json:"floatValue,omitempty"`
	// +optional
	BooleanValue *bool `json:"booleanValue,omitempty"`
}

// TypedParameterValue is the persisted structural discriminated union for one
// workflow parameter. The strict ordinary-JSON codec rejects null, objects,
// malformed values and nested arrays before this type reaches the API server.
// +kubebuilder:validation:XValidation:rule="(self.kind == 'string') == has(self.stringValue) && (self.kind == 'integer') == has(self.integerValue) && (self.kind == 'float') == has(self.floatValue) && (self.kind == 'boolean') == has(self.booleanValue) && (self.kind == 'array') == has(self.arrayValue)",message="kind must match exactly one value"
type TypedParameterValue struct {
	// +kubebuilder:validation:Enum=string;integer;float;boolean;array
	Kind string `json:"kind"`
	// +optional
	StringValue *string `json:"stringValue,omitempty"`
	// +optional
	IntegerValue *int64 `json:"integerValue,omitempty"`
	// +optional
	FloatValue *float64 `json:"floatValue,omitempty"`
	// +optional
	BooleanValue *bool `json:"booleanValue,omitempty"`
	// +optional
	ArrayValue []TypedPrimitiveParameter `json:"arrayValue,omitzero"`
}

// TypedParameterMap is the single persisted parameter representation shared by
// AgentSession, AIAnalysis, RemediationApprovalRequest and WorkflowExecution.
type TypedParameterMap map[string]TypedParameterValue

// UnmarshalJSON validates the persisted union at every JSON-to-CRD boundary.
// Constructors that receive ordinary LLM/tool JSON must still use
// DecodeTypedParameterMap; this method protects direct decoding of Kubernetes
// objects and typed CRD payloads from silently accepting malformed entries.
func (m *TypedParameterMap) UnmarshalJSON(raw []byte) error {
	fields, err := decodeStrictObjectFields(raw, "workflow parameters")
	if err != nil {
		return err
	}
	decoded := make(TypedParameterMap, len(fields))
	for name, valueRaw := range fields {
		value, err := decodePersistedParameterValue(valueRaw)
		if err != nil {
			return fmt.Errorf("parameter %q: %w", name, err)
		}
		decoded[name] = value
	}
	*m = decoded
	return nil
}

// MarshalJSON validates the persisted union before it can be serialized into
// an unstructured Kubernetes object or sent through a typed client. This is
// the outbound half of the direct-ingress guard; ordinary JSON values must not
// be constructed by assigning them directly to the persisted map.
func (m TypedParameterMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	raw, err := json.Marshal(map[string]TypedParameterValue(m))
	if err != nil {
		return nil, err
	}
	var validated TypedParameterMap
	if err := validated.UnmarshalJSON(raw); err != nil {
		return nil, fmt.Errorf("validate workflow parameters: %w", err)
	}
	return raw, nil
}

// UnmarshalJSON validates one persisted typed parameter value. The map-level
// method above is the normal CRD boundary; this method also prevents a value
// decoded independently from bypassing the same discriminated-union checks.
func (v *TypedParameterValue) UnmarshalJSON(raw []byte) error {
	decoded, err := decodePersistedParameterValue(raw)
	if err != nil {
		return err
	}
	*v = decoded
	return nil
}

// UnmarshalJSON validates one primitive element of a persisted array.
func (p *TypedPrimitiveParameter) UnmarshalJSON(raw []byte) error {
	decoded, err := decodePersistedPrimitiveParameter(raw)
	if err != nil {
		return err
	}
	*p = decoded
	return nil
}

// TypedParameterMapFromValues converts the parser's ordinary-JSON value map
// into the persisted structural union without mutating the input. Parser raw
// boundaries must populate numbers as json.Number; this function preserves
// those tokens while constructing the semantic typed values.
func TypedParameterMapFromValues(values map[string]interface{}) (TypedParameterMap, string, error) {
	if values == nil {
		return nil, "", nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, "", fmt.Errorf("marshal workflow parameters: %w", err)
	}
	return DecodeTypedParameterMap(raw)
}

// TypedParameterMapToEngineValues converts the persisted union at the legacy
// workflow-engine/schema-validator boundary. The conversion is intentionally
// one-way and non-persisted; callers must retain the typed map and evidence as
// the cross-service authority.
func TypedParameterMapToEngineValues(values TypedParameterMap) (map[string]string, error) {
	if values == nil {
		//nolint:nilnil // nil preserves the absent optional-map contract.
		return nil, nil
	}
	result := make(map[string]string, len(values))
	for name, value := range values {
		encoded, err := TypedParameterValueToEngineValue(value)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: %w", name, err)
		}
		result[name] = encoded
	}
	return result, nil
}

// TypedParameterMapToJSONValues converts the persisted union back to ordinary
// JSON values for LLM/tool input. Arrays remain flat primitive arrays; the
// structural CRD discriminator never crosses this boundary.
func TypedParameterMapToJSONValues(values TypedParameterMap) (map[string]interface{}, error) {
	if values == nil {
		//nolint:nilnil // nil preserves the absent optional-map contract.
		return nil, nil
	}
	result := make(map[string]interface{}, len(values))
	for name, value := range values {
		converted, err := TypedParameterValueToJSONValue(value)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: %w", name, err)
		}
		result[name] = converted
	}
	return result, nil
}

// TypedParameterValueToJSONValue converts one structural value to its
// ordinary-JSON representation for an LLM or tool boundary.
func TypedParameterValueToJSONValue(value TypedParameterValue) (interface{}, error) {
	if _, err := json.Marshal(TypedParameterMap{"value": value}); err != nil {
		return nil, fmt.Errorf("invalid typed parameter: %w", err)
	}
	switch value.Kind {
	case typedParameterKindString:
		if value.StringValue == nil {
			return nil, fmt.Errorf("string value is missing")
		}
		return *value.StringValue, nil
	case typedParameterKindInteger:
		if value.IntegerValue == nil {
			return nil, fmt.Errorf("integer value is missing")
		}
		return *value.IntegerValue, nil
	case typedParameterKindFloat:
		if value.FloatValue == nil {
			return nil, fmt.Errorf("float value is missing")
		}
		if math.IsNaN(*value.FloatValue) || math.IsInf(*value.FloatValue, 0) {
			return nil, fmt.Errorf("float value is not finite")
		}
		return *value.FloatValue, nil
	case typedParameterKindBoolean:
		if value.BooleanValue == nil {
			return nil, fmt.Errorf("boolean value is missing")
		}
		return *value.BooleanValue, nil
	case typedParameterKindArray:
		items := make([]interface{}, 0, len(value.ArrayValue))
		for index, primitive := range value.ArrayValue {
			item, err := typedPrimitiveToEngineValue(primitive)
			if err != nil {
				return nil, fmt.Errorf("array item %d: %w", index, err)
			}
			items = append(items, item)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported kind %q", value.Kind)
	}
}

// TypedParameterValueToEngineValue serializes one typed value for a component
// that still exposes the established string-valued workflow-engine contract.
func TypedParameterValueToEngineValue(value TypedParameterValue) (string, error) {
	if _, err := json.Marshal(TypedParameterMap{"value": value}); err != nil {
		return "", fmt.Errorf("invalid typed parameter: %w", err)
	}
	return typedParameterValueToEngineValue(value)
}

func typedParameterValueToEngineValue(value TypedParameterValue) (string, error) {
	switch value.Kind {
	case typedParameterKindString:
		if value.StringValue == nil {
			return "", fmt.Errorf("string value is missing")
		}
		return *value.StringValue, nil
	case typedParameterKindInteger:
		if value.IntegerValue == nil {
			return "", fmt.Errorf("integer value is missing")
		}
		return strconv.FormatInt(*value.IntegerValue, 10), nil
	case typedParameterKindFloat:
		if value.FloatValue == nil {
			return "", fmt.Errorf("float value is missing")
		}
		if math.IsNaN(*value.FloatValue) || math.IsInf(*value.FloatValue, 0) {
			return "", fmt.Errorf("float value is not finite")
		}
		return strconv.FormatFloat(*value.FloatValue, 'g', -1, 64), nil
	case typedParameterKindBoolean:
		if value.BooleanValue == nil {
			return "", fmt.Errorf("boolean value is missing")
		}
		return strconv.FormatBool(*value.BooleanValue), nil
	case typedParameterKindArray:
		items := make([]interface{}, 0, len(value.ArrayValue))
		for index, primitive := range value.ArrayValue {
			item, err := typedPrimitiveToEngineValue(primitive)
			if err != nil {
				return "", fmt.Errorf("array item %d: %w", index, err)
			}
			items = append(items, item)
		}
		raw, err := json.Marshal(items)
		if err != nil {
			return "", fmt.Errorf("encode array: %w", err)
		}
		return string(raw), nil
	default:
		return "", fmt.Errorf("unsupported kind %q", value.Kind)
	}
}

func typedPrimitiveToEngineValue(value TypedPrimitiveParameter) (interface{}, error) {
	switch value.Kind {
	case typedParameterKindString:
		if value.StringValue == nil {
			return nil, fmt.Errorf("string value is missing")
		}
		return *value.StringValue, nil
	case typedParameterKindInteger:
		if value.IntegerValue == nil {
			return nil, fmt.Errorf("integer value is missing")
		}
		return *value.IntegerValue, nil
	case typedParameterKindFloat:
		if value.FloatValue == nil {
			return nil, fmt.Errorf("float value is missing")
		}
		return *value.FloatValue, nil
	case typedParameterKindBoolean:
		if value.BooleanValue == nil {
			return nil, fmt.Errorf("boolean value is missing")
		}
		return *value.BooleanValue, nil
	default:
		return nil, fmt.Errorf("unsupported primitive kind %q", value.Kind)
	}
}

// DecodeTypedParameterMap validates an ordinary JSON object and converts it to
// the persisted structural union. It rejects null, objects, malformed or
// unsafe numbers, and nested/non-primitive array elements. The returned
// evidence is compact, lexicographically keyed JSON built from the original
// value tokens; it therefore distinguishes 1, 1.0 and 1e0.
func DecodeTypedParameterMap(raw []byte) (TypedParameterMap, string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var values map[string]json.RawMessage
	if err := decoder.Decode(&values); err != nil {
		return nil, "", fmt.Errorf("decode workflow parameters: %w", err)
	}
	if values == nil {
		return nil, "", fmt.Errorf("workflow parameters must be an object")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, "", err
	}

	result := make(TypedParameterMap, len(values))
	for name, valueRaw := range values {
		value, err := decodeTypedParameterValue(valueRaw)
		if err != nil {
			return nil, "", fmt.Errorf("parameter %q: %w", name, err)
		}
		result[name] = value
	}
	evidence, err := compactSortedObject(values)
	if err != nil {
		return nil, "", fmt.Errorf("encode parameter evidence: %w", err)
	}
	return result, evidence, nil
}

func decodePersistedParameterValue(raw []byte) (TypedParameterValue, error) {
	fields, kind, payload, err := decodePersistedUnionParts(raw, false)
	if err != nil {
		return TypedParameterValue{}, err
	}

	switch kind {
	case typedParameterKindString:
		value, err := decodePersistedString(fields[payload])
		if err != nil {
			return TypedParameterValue{}, fmt.Errorf("stringValue: %w", err)
		}
		return TypedParameterValue{Kind: kind, StringValue: &value}, nil
	case typedParameterKindInteger:
		value, err := decodePersistedInteger(fields[payload])
		if err != nil {
			return TypedParameterValue{}, fmt.Errorf("integerValue: %w", err)
		}
		return TypedParameterValue{Kind: kind, IntegerValue: &value}, nil
	case typedParameterKindFloat:
		value, err := decodePersistedFloat(fields[payload])
		if err != nil {
			return TypedParameterValue{}, fmt.Errorf("floatValue: %w", err)
		}
		return TypedParameterValue{Kind: kind, FloatValue: &value}, nil
	case typedParameterKindBoolean:
		value, err := decodePersistedBoolean(fields[payload])
		if err != nil {
			return TypedParameterValue{}, fmt.Errorf("booleanValue: %w", err)
		}
		return TypedParameterValue{Kind: kind, BooleanValue: &value}, nil
	case typedParameterKindArray:
		value, err := decodePersistedPrimitiveArray(fields[payload])
		if err != nil {
			return TypedParameterValue{}, fmt.Errorf("arrayValue: %w", err)
		}
		return TypedParameterValue{Kind: kind, ArrayValue: value}, nil
	}
	return TypedParameterValue{}, fmt.Errorf("unsupported kind %q", kind)
}

func decodePersistedPrimitiveParameter(raw []byte) (TypedPrimitiveParameter, error) {
	fields, kind, payload, err := decodePersistedUnionParts(raw, true)
	if err != nil {
		return TypedPrimitiveParameter{}, err
	}

	switch kind {
	case typedParameterKindString:
		value, err := decodePersistedString(fields[payload])
		if err != nil {
			return TypedPrimitiveParameter{}, fmt.Errorf("stringValue: %w", err)
		}
		return TypedPrimitiveParameter{Kind: kind, StringValue: &value}, nil
	case typedParameterKindInteger:
		value, err := decodePersistedInteger(fields[payload])
		if err != nil {
			return TypedPrimitiveParameter{}, fmt.Errorf("integerValue: %w", err)
		}
		return TypedPrimitiveParameter{Kind: kind, IntegerValue: &value}, nil
	case typedParameterKindFloat:
		value, err := decodePersistedFloat(fields[payload])
		if err != nil {
			return TypedPrimitiveParameter{}, fmt.Errorf("floatValue: %w", err)
		}
		return TypedPrimitiveParameter{Kind: kind, FloatValue: &value}, nil
	case typedParameterKindBoolean:
		value, err := decodePersistedBoolean(fields[payload])
		if err != nil {
			return TypedPrimitiveParameter{}, fmt.Errorf("booleanValue: %w", err)
		}
		return TypedPrimitiveParameter{Kind: kind, BooleanValue: &value}, nil
	}
	return TypedPrimitiveParameter{}, fmt.Errorf("unsupported primitive kind %q", kind)
}

func decodePersistedUnionParts(raw []byte, primitive bool) (map[string]json.RawMessage, string, string, error) {
	description := "typed parameter value"
	if primitive {
		description = "typed primitive parameter"
	}
	fields, err := decodeStrictObjectFields(raw, description)
	if err != nil {
		return nil, "", "", err
	}
	if err := validatePersistedFieldNames(fields, !primitive); err != nil {
		return nil, "", "", err
	}
	kind, err := requiredPersistedKind(fields)
	if err != nil {
		return nil, "", "", err
	}
	expectedPayload := persistedPayloadForKind(kind, primitive)
	if expectedPayload == "" {
		return nil, "", "", fmt.Errorf("unsupported persisted kind %q", kind)
	}
	payload, err := singlePersistedPayloadField(fields)
	if err != nil {
		return nil, "", "", err
	}
	if payload != expectedPayload {
		return nil, "", "", fmt.Errorf("kind %q requires %s", kind, expectedPayload)
	}
	return fields, kind, payload, nil
}

func persistedPayloadForKind(kind string, primitive bool) string {
	switch kind {
	case typedParameterKindString:
		return persistedFieldString
	case typedParameterKindInteger:
		return persistedFieldInteger
	case typedParameterKindFloat:
		return persistedFieldFloat
	case typedParameterKindBoolean:
		return persistedFieldBoolean
	case typedParameterKindArray:
		if !primitive {
			return persistedFieldArray
		}
	}
	return ""
}

func decodeStrictObjectFields(raw []byte, description string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil {
		return nil, fmt.Errorf("decode %s: %w", description, err)
	}
	if fields == nil {
		return nil, fmt.Errorf("%s must be an object", description)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	return fields, nil
}

func validatePersistedFieldNames(fields map[string]json.RawMessage, allowArray bool) error {
	for name := range fields {
		switch name {
		case "kind", persistedFieldString, persistedFieldInteger, persistedFieldFloat, persistedFieldBoolean:
		case persistedFieldArray:
			if !allowArray {
				return fmt.Errorf("unknown field %q on primitive parameter", name)
			}
		default:
			return fmt.Errorf("unknown field %q", name)
		}
	}
	return nil
}

func requiredPersistedKind(fields map[string]json.RawMessage) (string, error) {
	raw, ok := fields["kind"]
	if !ok || isJSONNull(raw) {
		return "", fmt.Errorf("kind is required")
	}
	var kind string
	if err := json.Unmarshal(raw, &kind); err != nil {
		return "", fmt.Errorf("decode kind: %w", err)
	}
	if kind == "" {
		return "", fmt.Errorf("kind must not be empty")
	}
	return kind, nil
}

func singlePersistedPayloadField(fields map[string]json.RawMessage) (string, error) {
	const payloadCount = 5
	payloads := [payloadCount]string{persistedFieldString, persistedFieldInteger, persistedFieldFloat, persistedFieldBoolean, persistedFieldArray}
	var found string
	for _, name := range payloads {
		if _, ok := fields[name]; !ok {
			continue
		}
		if found != "" {
			return "", fmt.Errorf("multiple typed value fields are set")
		}
		found = name
	}
	if found == "" {
		return "", fmt.Errorf("one typed value field is required")
	}
	return found, nil
}

func decodePersistedString(raw []byte) (string, error) {
	if isJSONNull(raw) {
		return "", fmt.Errorf("value must not be null")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func decodePersistedBoolean(raw []byte) (bool, error) {
	if isJSONNull(raw) {
		return false, fmt.Errorf("value must not be null")
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, err
	}
	return value, nil
}

func decodePersistedInteger(raw []byte) (int64, error) {
	lexeme, err := decodePersistedNumberLexeme(raw)
	if err != nil {
		return 0, err
	}
	if strings.ContainsAny(lexeme, ".eE") {
		return 0, fmt.Errorf("integer must be a base-10 integer")
	}
	value, err := strconv.ParseInt(lexeme, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("integer is outside signed int64: %w", err)
	}
	return value, nil
}

func decodePersistedFloat(raw []byte) (float64, error) {
	lexeme, err := decodePersistedNumberLexeme(raw)
	if err != nil {
		return 0, err
	}
	return finiteFloat(lexeme)
}

func decodePersistedNumberLexeme(raw []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return "", err
	}
	number, ok := value.(json.Number)
	if !ok {
		return "", fmt.Errorf("value must be a JSON number")
	}
	return number.String(), nil
}

func decodePersistedPrimitiveArray(raw []byte) ([]TypedPrimitiveParameter, error) {
	if isJSONNull(raw) {
		return nil, fmt.Errorf("value must not be null")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var items []json.RawMessage
	if err := decoder.Decode(&items); err != nil {
		return nil, fmt.Errorf("decode array: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	if items == nil {
		return nil, fmt.Errorf("value must be an array")
	}
	decoded := make([]TypedPrimitiveParameter, 0, len(items))
	for index, itemRaw := range items {
		item, err := decodePersistedPrimitiveParameter(itemRaw)
		if err != nil {
			return nil, fmt.Errorf("array item %d: %w", index, err)
		}
		decoded = append(decoded, item)
	}
	return decoded, nil
}

func isJSONNull(raw []byte) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func decodeTypedParameterValue(raw []byte) (TypedParameterValue, error) {
	var decoded interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return TypedParameterValue{}, fmt.Errorf("decode value: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return TypedParameterValue{}, err
	}

	switch value := decoded.(type) {
	case string:
		return TypedParameterValue{Kind: typedParameterKindString, StringValue: &value}, nil
	case bool:
		return TypedParameterValue{Kind: typedParameterKindBoolean, BooleanValue: &value}, nil
	case json.Number:
		return typedNumber(value.String())
	case []interface{}:
		array := make([]TypedPrimitiveParameter, 0, len(value))
		for index, item := range value {
			itemRaw, err := json.Marshal(item)
			if err != nil {
				return TypedParameterValue{}, fmt.Errorf("array item %d: %w", index, err)
			}
			primitive, err := decodeTypedPrimitive(itemRaw)
			if err != nil {
				return TypedParameterValue{}, fmt.Errorf("array item %d: %w", index, err)
			}
			array = append(array, primitive)
		}
		return TypedParameterValue{Kind: typedParameterKindArray, ArrayValue: array}, nil
	default:
		return TypedParameterValue{}, fmt.Errorf("value must be a string, integer, float, boolean, or flat primitive array")
	}
}

func decodeTypedPrimitive(raw []byte) (TypedPrimitiveParameter, error) {
	var decoded interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return TypedPrimitiveParameter{}, fmt.Errorf("decode primitive: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return TypedPrimitiveParameter{}, err
	}

	switch value := decoded.(type) {
	case string:
		return TypedPrimitiveParameter{Kind: typedParameterKindString, StringValue: &value}, nil
	case bool:
		return TypedPrimitiveParameter{Kind: typedParameterKindBoolean, BooleanValue: &value}, nil
	case json.Number:
		return typedPrimitiveNumber(value.String())
	default:
		return TypedPrimitiveParameter{}, fmt.Errorf("array items must be primitive strings, integers, floats, or booleans")
	}
}

func typedNumber(lexeme string) (TypedParameterValue, error) {
	if strings.ContainsAny(lexeme, ".eE") {
		value, err := finiteFloat(lexeme)
		if err != nil {
			return TypedParameterValue{}, err
		}
		return TypedParameterValue{Kind: typedParameterKindFloat, FloatValue: &value}, nil
	}
	value, err := strconv.ParseInt(lexeme, 10, 64)
	if err != nil {
		return TypedParameterValue{}, fmt.Errorf("integer is outside signed int64: %w", err)
	}
	return TypedParameterValue{Kind: typedParameterKindInteger, IntegerValue: &value}, nil
}

func typedPrimitiveNumber(lexeme string) (TypedPrimitiveParameter, error) {
	if strings.ContainsAny(lexeme, ".eE") {
		value, err := finiteFloat(lexeme)
		if err != nil {
			return TypedPrimitiveParameter{}, err
		}
		return TypedPrimitiveParameter{Kind: typedParameterKindFloat, FloatValue: &value}, nil
	}
	value, err := strconv.ParseInt(lexeme, 10, 64)
	if err != nil {
		return TypedPrimitiveParameter{}, fmt.Errorf("integer is outside signed int64: %w", err)
	}
	return TypedPrimitiveParameter{Kind: typedParameterKindInteger, IntegerValue: &value}, nil
}

func finiteFloat(lexeme string) (float64, error) {
	value, err := strconv.ParseFloat(lexeme, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("float is not finite or exceeds float64 range")
	}
	rational, ok := new(big.Rat).SetString(lexeme)
	if !ok || (value == 0 && rational.Sign() != 0) {
		return 0, fmt.Errorf("float has nonzero underflow")
	}
	return value, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON input")
		}
		return fmt.Errorf("decode trailing JSON input: %w", err)
	}
	return nil
}

func compactSortedObject(values map[string]json.RawMessage) (string, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var output bytes.Buffer
	output.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			output.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return "", err
		}
		output.Write(encodedKey)
		output.WriteByte(':')
		value := bytes.TrimSpace(values[key])
		var compact bytes.Buffer
		if err := json.Compact(&compact, value); err != nil {
			return "", err
		}
		output.Write(compact.Bytes())
	}
	output.WriteByte('}')
	return output.String(), nil
}
