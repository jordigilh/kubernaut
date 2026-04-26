// check-openapi-crd-enum-drift compares enum values in CRD schemas (generated
// from Go types via controller-gen) against the DataStorage OpenAPI spec. It
// uses a declarative mapping file to pair CRD fields with their OpenAPI
// counterparts and exits non-zero when the CRD contains values that the OpenAPI
// spec (and therefore the ogen-generated client) would reject at runtime.
//
// Usage:
//
//	go run ./scripts/validation/check-openapi-crd-enum-drift [repo-root]
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Mapping struct {
	CRD           string `yaml:"crd"`
	CRDPath       string `yaml:"crd_path"`
	OpenAPISchema string `yaml:"openapi_schema"`
	OpenAPIField  string `yaml:"openapi_field"`
	SubsetOK      bool   `yaml:"subset_ok,omitempty"`
	Note          string `yaml:"note,omitempty"`
}

type MappingFile struct {
	Mappings []Mapping `yaml:"mappings"`
}

func main() {
	repoRoot := "."
	if len(os.Args) > 1 {
		repoRoot = os.Args[1]
	}

	crdDir := filepath.Join(repoRoot, "config", "crd", "bases")
	openapiPath := filepath.Join(repoRoot, "api", "openapi", "data-storage-v1.yaml")
	mappingPath := filepath.Join(repoRoot, "scripts", "validation", "openapi-crd-enum-mappings.yaml")

	crdEnums, err := extractAllCRDEnums(crdDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR extracting CRD enums: %v\n", err)
		os.Exit(2)
	}

	openapiEnums, err := extractOpenAPIEnums(openapiPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR extracting OpenAPI enums: %v\n", err)
		os.Exit(2)
	}

	mappings, err := loadMappings(mappingPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR loading mapping file: %v\n", err)
		os.Exit(2)
	}

	violations := 0
	warnings := 0

	for _, m := range mappings.Mappings {
		crdKey := m.CRD + ":" + m.CRDPath
		oaKey := m.OpenAPISchema + "." + m.OpenAPIField

		crdVals, crdOK := crdEnums[crdKey]
		oaVals, oaOK := openapiEnums[oaKey]

		if !crdOK {
			fmt.Printf("  WARN: CRD enum not found: %s\n", crdKey)
			warnings++
			continue
		}
		if !oaOK {
			fmt.Printf("  WARN: OpenAPI enum not found: %s\n", oaKey)
			warnings++
			continue
		}

		crdOnly := setDiff(crdVals, oaVals)
		oaOnly := setDiff(oaVals, crdVals)

		if len(crdOnly) == 0 && len(oaOnly) == 0 {
			fmt.Printf("  OK: %s <-> %s\n", crdKey, oaKey)
			continue
		}

		if len(crdOnly) > 0 {
			if m.SubsetOK {
				fmt.Printf("  WARN [subset_ok]: %s -> %s: CRD has %v not in OpenAPI\n", crdKey, oaKey, crdOnly)
				warnings++
			} else {
				fmt.Printf("  FAIL: %s -> %s: CRD has %v not in OpenAPI (ogen will reject)\n", crdKey, oaKey, crdOnly)
				violations++
			}
		}

		if len(oaOnly) > 0 {
			fmt.Printf("  WARN: %s -> %s: OpenAPI has %v not in CRD\n", crdKey, oaKey, oaOnly)
			warnings++
		}

		if m.Note != "" {
			fmt.Printf("         Note: %s\n", m.Note)
		}
	}

	// Check for unmapped CRD enums (potential blind spots)
	mappedCRDKeys := make(map[string]bool)
	for _, m := range mappings.Mappings {
		mappedCRDKeys[m.CRD+":"+m.CRDPath] = true
	}
	unmapped := 0
	for k := range crdEnums {
		// Skip generic Kubernetes condition status enums
		if strings.HasSuffix(k, "conditions[].status") {
			continue
		}
		if !mappedCRDKeys[k] {
			unmapped++
			if unmapped == 1 {
				fmt.Println("\n  Unmapped CRD enums (consider adding mappings):")
			}
			fmt.Printf("    - %s: %v\n", k, crdEnums[k])
		}
	}

	fmt.Printf("\n=== Summary: %d violation(s), %d warning(s), %d unmapped ===\n", violations, warnings, unmapped)
	if violations > 0 {
		fmt.Println("\nFAILED: CRD enum values missing from OpenAPI spec will cause ogen client rejection at runtime.")
		fmt.Println("Fix: Add missing values to api/openapi/data-storage-v1.yaml, then run 'make generate-datastorage-client'.")
		os.Exit(1)
	}
}

// extractAllCRDEnums parses all CRD YAMLs and returns enum values keyed by
// "filename:json.path".
func extractAllCRDEnums(dir string) (map[string][]string, error) {
	result := make(map[string][]string)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading CRD directory %s: %w", dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
		}

		var doc map[string]interface{}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", e.Name(), err)
		}

		versions, ok := navigateSlice(doc, "spec", "versions")
		if !ok {
			continue
		}

		for _, v := range versions {
			vMap, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			schema, ok := navigateMap(vMap, "schema", "openAPIV3Schema", "properties")
			if !ok {
				continue
			}
			propsMap, ok := schema.(map[string]interface{})
			if !ok {
				continue
			}
			for topKey, topVal := range propsMap {
				if topKey != "spec" && topKey != "status" {
					continue
				}
				topObj, ok := topVal.(map[string]interface{})
				if !ok {
					continue
				}
				if props, exists := topObj["properties"]; exists {
					walkCRDProperties(props, topKey, e.Name(), result)
				}
			}
		}
	}

	return result, nil
}

// walkCRDProperties recursively extracts enum arrays from a CRD properties tree,
// handling direct enum, allOf-wrapped enum, nested properties, and array items.
func walkCRDProperties(node interface{}, path string, filename string, result map[string][]string) {
	propsMap, ok := node.(map[string]interface{})
	if !ok {
		return
	}

	for fieldName, fieldVal := range propsMap {
		fieldMap, ok := fieldVal.(map[string]interface{})
		if !ok {
			continue
		}

		currentPath := path + "." + fieldName

		collectEnum(fieldMap, filename, currentPath, result)

		if subProps, exists := fieldMap["properties"]; exists {
			walkCRDProperties(subProps, currentPath, filename, result)
		}

		if items, exists := fieldMap["items"]; exists {
			if itemsMap, ok := items.(map[string]interface{}); ok {
				collectEnum(itemsMap, filename, currentPath+"[]", result)
				if itemProps, exists := itemsMap["properties"]; exists {
					walkCRDProperties(itemProps, currentPath+"[]", filename, result)
				}
			}
		}
	}
}

// collectEnum extracts enum values from a field map, handling both direct enum
// and allOf-wrapped enum patterns (controller-gen emits allOf for shared types).
func collectEnum(fieldMap map[string]interface{}, filename, path string, result map[string][]string) {
	if enumVal, exists := fieldMap["enum"]; exists {
		if vals := toStringSlice(enumVal); vals != nil {
			result[filename+":"+path] = vals
			return
		}
	}

	if allOf, exists := fieldMap["allOf"]; exists {
		if allOfSlice, ok := allOf.([]interface{}); ok {
			for _, item := range allOfSlice {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if enumVal, exists := itemMap["enum"]; exists {
						if vals := toStringSlice(enumVal); vals != nil {
							result[filename+":"+path] = vals
							return
						}
					}
				}
			}
		}
	}
}

// extractOpenAPIEnums parses an OpenAPI 3.x spec and returns enum values keyed
// by "SchemaName.fieldName" for component schemas and "param:path:method.name"
// for query/path parameters.
func extractOpenAPIEnums(path string) (map[string][]string, error) {
	result := make(map[string][]string)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	schemas, ok := navigateMap(doc, "components", "schemas")
	if !ok {
		return nil, fmt.Errorf("components.schemas not found")
	}
	schemasMap, ok := schemas.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("components.schemas is not a map")
	}

	for schemaName, schemaVal := range schemasMap {
		schemaMap, ok := schemaVal.(map[string]interface{})
		if !ok {
			continue
		}
		if props, exists := schemaMap["properties"]; exists {
			walkOpenAPIProperties(props, schemaName, result)
		}
	}

	return result, nil
}

func walkOpenAPIProperties(node interface{}, prefix string, result map[string][]string) {
	propsMap, ok := node.(map[string]interface{})
	if !ok {
		return
	}

	for fieldName, fieldVal := range propsMap {
		fieldMap, ok := fieldVal.(map[string]interface{})
		if !ok {
			continue
		}

		key := prefix + "." + fieldName

		if enumVal, exists := fieldMap["enum"]; exists {
			if vals := toStringSlice(enumVal); vals != nil {
				result[key] = vals
			}
		}

		if subProps, exists := fieldMap["properties"]; exists {
			walkOpenAPIProperties(subProps, key, result)
		}

		if items, exists := fieldMap["items"]; exists {
			if itemsMap, ok := items.(map[string]interface{}); ok {
				if itemProps, exists := itemsMap["properties"]; exists {
					walkOpenAPIProperties(itemProps, key+"[]", result)
				}
				if enumVal, exists := itemsMap["enum"]; exists {
					if vals := toStringSlice(enumVal); vals != nil {
						result[key+"[]"] = vals
					}
				}
			}
		}
	}
}

func loadMappings(path string) (*MappingFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var mf MappingFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, err
	}
	return &mf, nil
}

func navigateMap(m interface{}, keys ...string) (interface{}, bool) {
	current := m
	for _, k := range keys {
		cm, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current = cm[k]
		if current == nil {
			return nil, false
		}
	}
	return current, true
}

func navigateSlice(m interface{}, keys ...string) ([]interface{}, bool) {
	val, ok := navigateMap(m, keys...)
	if !ok {
		return nil, false
	}
	sl, ok := val.([]interface{})
	return sl, ok
}

func toStringSlice(v interface{}) []string {
	sl, ok := v.([]interface{})
	if !ok {
		return nil
	}
	vals := make([]string, 0, len(sl))
	for _, item := range sl {
		vals = append(vals, fmt.Sprintf("%v", item))
	}
	sort.Strings(vals)
	return vals
}

func setDiff(a, b []string) []string {
	bSet := make(map[string]bool, len(b))
	for _, v := range b {
		bSet[v] = true
	}
	var diff []string
	for _, v := range a {
		if !bSet[v] {
			diff = append(diff, v)
		}
	}
	return diff
}
