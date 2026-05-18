package prometheus

import (
	"fmt"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

// ExtractLabelMatchers parses a PromQL expression and extracts all VectorSelector
// label matchers from the AST. Returns an error for invalid PromQL (does not panic).
func ExtractLabelMatchers(expr string) ([]*labels.Matcher, error) {
	p := parser.NewParser(parser.Options{})
	ast, err := p.ParseExpr(expr)
	if err != nil {
		return nil, fmt.Errorf("parsing PromQL: %w", err)
	}

	var matchers []*labels.Matcher
	parser.Inspect(ast, func(node parser.Node, _ []parser.Node) error {
		if vs, ok := node.(*parser.VectorSelector); ok {
			for _, m := range vs.LabelMatchers {
				if m.Name == "__name__" {
					continue
				}
				matchers = append(matchers, m)
			}
		}
		return nil
	})
	return matchers, nil
}

// MatchesResource checks whether a set of PromQL label matchers are satisfied
// by the given resource labels. All matchers must match for the function to
// return true. Empty matchers always returns false.
func MatchesResource(matchers []*labels.Matcher, resourceLabels map[string]string) bool {
	if len(matchers) == 0 {
		return false
	}
	for _, m := range matchers {
		val, exists := resourceLabels[m.Name]
		if !exists {
			return false
		}
		if !m.Matches(val) {
			return false
		}
	}
	return true
}
