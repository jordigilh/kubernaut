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

// validate-graphql-query validates a GraphQL query string against an SDL schema
// file using gqlparser. Used by scripts/check-acm-schema-drift.sh to verify
// adapter query compatibility across ACM release branches.
//
// Usage: go run ./scripts/validate-graphql-query -schema <path> -query <string>
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func main() {
	schemaPath := flag.String("schema", "", "path to GraphQL SDL schema file")
	query := flag.String("query", "", "GraphQL query string to validate")
	flag.Parse()

	if *schemaPath == "" || *query == "" {
		fmt.Fprintln(os.Stderr, "usage: validate-graphql-query -schema <path> -query <string>")
		os.Exit(2)
	}

	sdl, err := os.ReadFile(*schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read schema: %v\n", err)
		os.Exit(1)
	}

	schema, parseErr := gqlparser.LoadSchema(&ast.Source{
		Name:  *schemaPath,
		Input: string(sdl),
	})
	if parseErr != nil {
		fmt.Fprintf(os.Stderr, "parse schema: %v\n", parseErr)
		os.Exit(1)
	}

	_, queryErrs := gqlparser.LoadQuery(schema, *query)
	if len(queryErrs) > 0 {
		msgs := make([]string, len(queryErrs))
		for i, e := range queryErrs {
			msgs[i] = e.Message
		}
		fmt.Fprintf(os.Stderr, "query validation failed: %s\n", strings.Join(msgs, "; "))
		os.Exit(1)
	}
}
