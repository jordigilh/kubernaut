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
			data, err := schemaFS.ReadFile("schemas/" + entry.Name())
			if err != nil {
				schemaInitErr = fmt.Errorf("read schema %s: %w", entry.Name(), err)
				return
			}
			var doc any
			if err := json.Unmarshal(data, &doc); err != nil {
				schemaInitErr = fmt.Errorf("parse schema %s: %w", entry.Name(), err)
				return
			}
			url := "https://kubernaut.ai/schemas/a2a/" + entry.Name()
			if err := c.AddResource(url, doc); err != nil {
				schemaInitErr = fmt.Errorf("add schema %s: %w", entry.Name(), err)
				return
			}

			sch, err := c.Compile(url)
			if err != nil {
				schemaInitErr = fmt.Errorf("compile schema %s: %w", entry.Name(), err)
				return
			}

			name := entry.Name()
			// Strip ".v1.schema.json" suffix to get the schema name
			if len(name) > len(".v1.schema.json") {
				name = name[:len(name)-len(".v1.schema.json")]
			}
			schemaCache[name] = sch
		}
	})
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
