package launcher

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schemas/*.json
var schemaFS embed.FS

var (
	schemaOnce    sync.Once
	schemaCache   map[string]*jsonschema.Schema
	schemaInitErr error
)

func initSchemas() {
	schemaOnce.Do(func() {
		schemaCache = make(map[string]*jsonschema.Schema)

		entries, err := schemaFS.ReadDir("schemas")
		if err != nil {
			schemaInitErr = fmt.Errorf("read schema dir: %w", err)
			return
		}

		c := jsonschema.NewCompiler()
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if err := compileSchemaEntry(c, entry.Name()); err != nil {
				schemaInitErr = err
				return
			}
		}
	})
}

// compileSchemaEntry reads, parses, and compiles a single embedded schema
// file, caching the result under its short name (see schemaCacheName).
func compileSchemaEntry(c *jsonschema.Compiler, entryName string) error {
	data, err := schemaFS.ReadFile("schemas/" + entryName)
	if err != nil {
		return fmt.Errorf("read schema %s: %w", entryName, err)
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse schema %s: %w", entryName, err)
	}
	url := "https://kubernaut.ai/schemas/a2a/" + entryName
	if err := c.AddResource(url, doc); err != nil {
		return fmt.Errorf("add schema %s: %w", entryName, err)
	}

	sch, err := c.Compile(url)
	if err != nil {
		return fmt.Errorf("compile schema %s: %w", entryName, err)
	}

	schemaCache[schemaCacheName(entryName)] = sch
	if entryName == "investigation_summary.v1.schema.json" {
		// DD-KA-2496: enforce the new allow-list independently of inherited
		// artifact fields, preserving existing presentation/recovery formats.
		for _, field := range []string{"selected_workflow", "selection_assessment"} {
			projection, err := c.Compile(url + "#/$defs/" + field)
			if err != nil {
				return fmt.Errorf("compile live selection schema %s: %w", field, err)
			}
			schemaCache["investigation_summary."+field] = projection
		}
	}
	return nil
}

// schemaCacheName strips the ".v1.schema.json" suffix from an embedded
// schema filename to get its cache key.
func schemaCacheName(entryName string) string {
	const suffix = ".v1.schema.json"
	if len(entryName) > len(suffix) {
		return entryName[:len(entryName)-len(suffix)]
	}
	return entryName
}

// ValidatePayload validates a structured payload against its named JSON Schema.
// Returns nil if valid, or an error with field path details if invalid.
func ValidatePayload(schemaName string, data map[string]any) error {
	initSchemas()
	if schemaInitErr != nil {
		return fmt.Errorf("schema init: %w", schemaInitErr)
	}

	sch, ok := schemaCache[schemaName]
	if !ok {
		return fmt.Errorf("unknown schema: %q", schemaName)
	}

	return sch.Validate(data)
}

func validateLiveSelectionContext(data map[string]any) error {
	initSchemas()
	if schemaInitErr != nil {
		return fmt.Errorf("live selection schema unavailable: %w", schemaInitErr)
	}
	for _, field := range []string{"selected_workflow", "selection_assessment"} {
		value, present := data[field]
		if !present {
			continue
		}
		schema := schemaCache["investigation_summary."+field]
		if err := schema.Validate(value); err != nil {
			// Schema errors can include the rejected value. Never copy private
			// contents into logs, metadata or the human-readable fallback.
			return fmt.Errorf("invalid live %s context", field)
		}
	}
	return nil
}
