/*
Copyright 2025 Jordi Gil.

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

package engine

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ExpressionEngine provides efficient expression evaluation with caching
type ExpressionEngine struct {
	compiledExpressions sync.Map // map[string]*CompiledExpression
	functions           map[string]ExpressionFunction
}

// CompiledExpression represents a pre-compiled expression for faster evaluation
type CompiledExpression struct {
	Original   string
	Tokens     []Token
	Operations []Operation
	Variables  []string
	CompiledAt time.Time
}

// Token represents a parsed token in an expression
type Token struct {
	Type  TokenType
	Value string
	Pos   int
}

// TokenType defines the type of a token
type TokenType int

const (
	TokenLiteral TokenType = iota
	TokenNumber
	TokenVariable
	TokenOperator
	TokenFunction
	TokenComparison
)

// Operation represents a compiled operation
type Operation struct {
	Type     OperationType
	Operator string
	Left     interface{}
	Right    interface{}
}

// OperationType defines the type of operation
type OperationType int

const (
	OpComparison OperationType = iota
	OpArithmetic
	OpLogical
	OpFunction
)

// ExpressionFunction defines a function that can be called in expressions
type ExpressionFunction func(ctx context.Context, args []interface{}, result *StepResult) (interface{}, error)

// ExpressionContext contains all available variables and functions for evaluation
type ExpressionContext struct {
	Result    *StepResult
	StepCtx   *StepContext
	Variables map[string]interface{}
	StartTime time.Time
}

// NewExpressionEngine creates a new optimized expression engine
func NewExpressionEngine() *ExpressionEngine {
	engine := &ExpressionEngine{
		functions: make(map[string]ExpressionFunction),
	}

	// Register built-in functions
	engine.registerBuiltinFunctions()

	return engine
}

// registerBuiltinFunctions registers optimized built-in functions
func (ee *ExpressionEngine) registerBuiltinFunctions() {
	ee.functions["contains"] = func(ctx context.Context, args []interface{}, result *StepResult) (interface{}, error) {
		if len(args) != 2 {
			return false, fmt.Errorf("contains() requires exactly 2 arguments")
		}
		haystack := fmt.Sprintf("%v", args[0])
		needle := fmt.Sprintf("%v", args[1])
		return strings.Contains(haystack, needle), nil
	}

	ee.functions["len"] = func(ctx context.Context, args []interface{}, result *StepResult) (interface{}, error) {
		if len(args) != 1 {
			return 0, fmt.Errorf("len() requires exactly 1 argument")
		}
		switch v := args[0].(type) {
		case string:
			return len(v), nil
		case []interface{}:
			return len(v), nil
		case map[string]interface{}:
			return len(v), nil
		default:
			return 0, fmt.Errorf("len() not supported for type %T", v)
		}
	}

	ee.functions["duration_seconds"] = func(ctx context.Context, args []interface{}, result *StepResult) (interface{}, error) {
		return result.Duration.Seconds(), nil
	}

	ee.functions["has_error"] = func(ctx context.Context, args []interface{}, result *StepResult) (interface{}, error) {
		return result.Error != "", nil
	}

	ee.functions["output_value"] = func(ctx context.Context, args []interface{}, result *StepResult) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("output_value() requires exactly 1 argument")
		}
		key := fmt.Sprintf("%v", args[0])
		if result.Output != nil {
			return result.Output[key], nil
		}
		return nil, nil
	}
}

// Compile pre-compiles an expression for faster repeated evaluation
func (ee *ExpressionEngine) Compile(expression string) (*CompiledExpression, error) {
	if cached, ok := ee.compiledExpressions.Load(expression); ok {
		return cached.(*CompiledExpression), nil
	}

	compiled, err := ee.compileExpression(expression)
	if err != nil {
		return nil, err
	}

	ee.compiledExpressions.Store(expression, compiled)
	return compiled, nil
}

// Evaluate efficiently evaluates a compiled expression
func (ee *ExpressionEngine) Evaluate(ctx context.Context, compiled *CompiledExpression, exprCtx *ExpressionContext) (interface{}, error) {
	// Fast path for simple comparisons
	if len(compiled.Operations) == 1 {
		return ee.evaluateOperation(ctx, compiled.Operations[0], exprCtx)
	}

	// Evaluate complex expressions
	var result interface{}
	for _, op := range compiled.Operations {
		val, err := ee.evaluateOperation(ctx, op, exprCtx)
		if err != nil {
			return nil, err
		}
		result = val
	}

	return result, nil
}

// EvaluateString evaluates a string expression (compiles and caches automatically)
func (ee *ExpressionEngine) EvaluateString(ctx context.Context, expression string, exprCtx *ExpressionContext) (interface{}, error) {
	compiled, err := ee.Compile(expression)
	if err != nil {
		return nil, err
	}

	return ee.Evaluate(ctx, compiled, exprCtx)
}

// GetCacheSize returns the number of compiled expressions currently cached
// Business Requirement: BR-EXPR-ENGINE-003 - Provide cache metrics for performance monitoring
func (ee *ExpressionEngine) GetCacheSize() int {
	size := 0
	ee.compiledExpressions.Range(func(_, _ interface{}) bool {
		size++
		return true
	})
	return size
}

// compileExpression parses and compiles an expression
func (ee *ExpressionEngine) compileExpression(expression string) (*CompiledExpression, error) {
	tokens, err := ee.tokenize(expression)
	if err != nil {
		return nil, err
	}

	operations, err := ee.parseTokens(tokens)
	if err != nil {
		return nil, err
	}

	variables := ee.extractVariables(tokens)

	return &CompiledExpression{
		Original:   expression,
		Tokens:     tokens,
		Operations: operations,
		Variables:  variables,
		CompiledAt: time.Now(),
	}, nil
}

// tokenize breaks an expression into tokens
func (ee *ExpressionEngine) tokenize(expression string) ([]Token, error) {
	var tokens []Token
	expr := strings.TrimSpace(expression)

	// Common patterns
	numberPattern := regexp.MustCompile(`^\d+(\.\d+)?`)
	variablePattern := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*`)
	operatorPattern := regexp.MustCompile(`^(>=|<=|!=|==|&&|\|\||[><=+\-*/])`)
	stringPattern := regexp.MustCompile(`^"([^"]*)"`)

	i := 0
	for i < len(expr) {
		// Skip whitespace
		if expr[i] == ' ' || expr[i] == '\t' {
			i++
			continue
		}

		remaining := expr[i:]

		// Handle parentheses
		if expr[i] == '(' || expr[i] == ')' {
			tokens = append(tokens, Token{
				Type:  TokenOperator,
				Value: string(expr[i]),
				Pos:   i,
			})
			i++
			continue
		}

		// String literals
		if match := stringPattern.FindString(remaining); match != "" {
			tokens = append(tokens, Token{
				Type:  TokenLiteral,
				Value: match[1 : len(match)-1], // Remove quotes
				Pos:   i,
			})
			i += len(match)
			continue
		}

		// Numbers
		if match := numberPattern.FindString(remaining); match != "" {
			tokens = append(tokens, Token{
				Type:  TokenNumber,
				Value: match,
				Pos:   i,
			})
			i += len(match)
			continue
		}

		// Operators
		if match := operatorPattern.FindString(remaining); match != "" {
			tokens = append(tokens, Token{
				Type:  TokenOperator,
				Value: match,
				Pos:   i,
			})
			i += len(match)
			continue
		}

		// Variables and functions
		if match := variablePattern.FindString(remaining); match != "" {
			tokenType := TokenVariable
			// Check if it's a function call
			if i+len(match) < len(expr) && expr[i+len(match)] == '(' {
				tokenType = TokenFunction
			}
			// Handle boolean literals as special tokens
			if match == "true" || match == "false" {
				tokenType = TokenLiteral
			}

			tokens = append(tokens, Token{
				Type:  tokenType,
				Value: match,
				Pos:   i,
			})
			i += len(match)
			continue
		}

		// Unknown character
		return nil, fmt.Errorf("unexpected character '%c' at position %d", expr[i], i)
	}

	return tokens, nil
}

// parseTokens converts tokens into operations
func (ee *ExpressionEngine) parseTokens(tokens []Token) ([]Operation, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty expression")
	}

	// Single value
	if len(tokens) == 1 {
		return []Operation{{
			Type: OpComparison,
			Left: tokens[0],
		}}, nil
	}

	// Function call without arguments
	if len(tokens) == 3 && tokens[0].Type == TokenFunction && tokens[1].Value == "(" && tokens[2].Value == ")" {
		return []Operation{{
			Type:     OpFunction,
			Operator: tokens[0].Value,
			Left:     []Token{}, // No arguments
		}}, nil
	}

	// Function call with arguments
	if len(tokens) >= 4 && tokens[0].Type == TokenFunction && tokens[1].Value == "(" {
		// Find the closing parenthesis
		closeParen := -1
		for i := 2; i < len(tokens); i++ {
			if tokens[i].Value == ")" {
				closeParen = i
				break
			}
		}

		if closeParen == -1 {
			return nil, fmt.Errorf("missing closing parenthesis for function call")
		}

		// Extract function arguments (simplified - no nested function support yet)
		args := []Token{}
		for i := 2; i < closeParen; i++ {
			if tokens[i].Value != "," {
				args = append(args, tokens[i])
			}
		}

		// If there are more tokens after the function call, parse them as additional operations
		if closeParen+1 < len(tokens) && closeParen+2 < len(tokens) {
			// This is a function call followed by an operator and another value
			return []Operation{{
				Type:     OpComparison,
				Operator: tokens[closeParen+1].Value,
				Left: Operation{
					Type:     OpFunction,
					Operator: tokens[0].Value,
					Left:     args,
				},
				Right: tokens[closeParen+2],
			}}, nil
		}

		// Just the function call
		return []Operation{{
			Type:     OpFunction,
			Operator: tokens[0].Value,
			Left:     args,
		}}, nil
	}

	// Simple binary operations
	if len(tokens) == 3 && tokens[1].Type == TokenOperator {
		return []Operation{{
			Type:     OpComparison,
			Operator: tokens[1].Value,
			Left:     tokens[0],
			Right:    tokens[2],
		}}, nil
	}

	return nil, fmt.Errorf("unsupported expression pattern: %d tokens", len(tokens))
}

// extractVariables extracts all variable names from tokens
func (ee *ExpressionEngine) extractVariables(tokens []Token) []string {
	var variables []string
	seen := make(map[string]bool)

	for _, token := range tokens {
		if token.Type == TokenVariable && !seen[token.Value] {
			variables = append(variables, token.Value)
			seen[token.Value] = true
		}
	}

	return variables
}

// evaluateOperation evaluates a single operation efficiently
func (ee *ExpressionEngine) evaluateOperation(ctx context.Context, op Operation, exprCtx *ExpressionContext) (interface{}, error) {
	switch op.Type {
	case OpFunction:
		funcName := op.Operator
		function, exists := ee.functions[funcName]
		if !exists {
			return nil, fmt.Errorf("unknown function: %s", funcName)
		}

		// Extract arguments
		var args []interface{}
		if argTokens, ok := op.Left.([]Token); ok {
			for _, token := range argTokens {
				val, err := ee.resolveToken(token, exprCtx)
				if err != nil {
					return nil, err
				}
				args = append(args, val)
			}
		}

		return function(ctx, args, exprCtx.Result)

	case OpComparison:
		if op.Right == nil {
			// Single value evaluation or nested operation
			if token, ok := op.Left.(Token); ok {
				return ee.resolveToken(token, exprCtx)
			} else if nestedOp, ok := op.Left.(Operation); ok {
				return ee.evaluateOperation(ctx, nestedOp, exprCtx)
			}
			return nil, fmt.Errorf("invalid left operand type")
		}

		// Handle left operand (could be token or operation)
		var left interface{}
		var err error
		if token, ok := op.Left.(Token); ok {
			left, err = ee.resolveToken(token, exprCtx)
		} else if nestedOp, ok := op.Left.(Operation); ok {
			left, err = ee.evaluateOperation(ctx, nestedOp, exprCtx)
		} else {
			return nil, fmt.Errorf("invalid left operand type")
		}
		if err != nil {
			return nil, err
		}

		// Handle right operand
		var right interface{}
		if token, ok := op.Right.(Token); ok {
			right, err = ee.resolveToken(token, exprCtx)
		} else if nestedOp, ok := op.Right.(Operation); ok {
			right, err = ee.evaluateOperation(ctx, nestedOp, exprCtx)
		} else {
			return nil, fmt.Errorf("invalid right operand type")
		}
		if err != nil {
			return nil, err
		}

		return ee.compareValues(left, right, op.Operator)

	default:
		return nil, fmt.Errorf("unsupported operation type")
	}
}

// resolveToken resolves a token to its actual value
func (ee *ExpressionEngine) resolveToken(token Token, exprCtx *ExpressionContext) (interface{}, error) {
	switch token.Type {
	case TokenNumber:
		if strings.Contains(token.Value, ".") {
			return strconv.ParseFloat(token.Value, 64)
		}
		return strconv.ParseInt(token.Value, 10, 64)

	case TokenLiteral:
		// Handle special string literals
		switch token.Value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return token.Value, nil
		}

	case TokenVariable:
		return ee.resolveVariable(token.Value, exprCtx), nil

	default:
		return nil, fmt.Errorf("cannot resolve token type %d", token.Type)
	}
}

// resolveVariable efficiently resolves a variable from context
func (ee *ExpressionEngine) resolveVariable(name string, exprCtx *ExpressionContext) interface{} {
	// Fast lookups for common variables
	switch name {
	case "success":
		return exprCtx.Result.Success
	case "error":
		return exprCtx.Result.Error
	case "confidence":
		return exprCtx.Result.Confidence
	case "duration":
		return exprCtx.Result.Duration.Seconds()
	case "duration_ms":
		return float64(exprCtx.Result.Duration.Nanoseconds()) / 1e6
	}

	// Check custom variables
	if exprCtx.Variables != nil {
		if val, exists := exprCtx.Variables[name]; exists {
			return val
		}
	}

	// Check step context variables
	if exprCtx.StepCtx != nil && exprCtx.StepCtx.Variables != nil {
		if val, exists := exprCtx.StepCtx.Variables[name]; exists {
			return val
		}
	}

	// Check output values
	if strings.HasPrefix(name, "output.") {
		key := name[7:] // Remove "output." prefix
		if exprCtx.Result.Output != nil {
			return exprCtx.Result.Output[key]
		}
	}

	return nil
}

// compareValues efficiently compares two values
func (ee *ExpressionEngine) compareValues(left, right interface{}, operator string) (interface{}, error) {
	switch operator {
	case "==":
		return ee.equals(left, right), nil
	case "!=":
		return !ee.equals(left, right), nil
	case ">":
		return ee.greater(left, right)
	case ">=":
		return ee.greaterEqual(left, right)
	case "<":
		return ee.less(left, right)
	case "<=":
		return ee.lessEqual(left, right)
	default:
		return nil, fmt.Errorf("unsupported operator: %s", operator)
	}
}

// equals efficiently compares equality
func (ee *ExpressionEngine) equals(left, right interface{}) bool {
	// Type-aware comparison
	switch l := left.(type) {
	case string:
		if r, ok := right.(string); ok {
			return l == r
		}
	case int64:
		switch r := right.(type) {
		case int64:
			return l == r
		case float64:
			return float64(l) == r
		}
	case float64:
		switch r := right.(type) {
		case float64:
			return l == r
		case int64:
			return l == float64(r)
		}
	case bool:
		if r, ok := right.(bool); ok {
			return l == r
		}
	}

	// Fallback to string comparison
	return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right)
}

// Numeric comparison helpers
func (ee *ExpressionEngine) greater(left, right interface{}) (interface{}, error) {
	l, r, err := ee.toNumbers(left, right)
	if err != nil {
		return false, err
	}
	return l > r, nil
}

func (ee *ExpressionEngine) greaterEqual(left, right interface{}) (interface{}, error) {
	l, r, err := ee.toNumbers(left, right)
	if err != nil {
		return false, err
	}
	return l >= r, nil
}

func (ee *ExpressionEngine) less(left, right interface{}) (interface{}, error) {
	l, r, err := ee.toNumbers(left, right)
	if err != nil {
		return false, err
	}
	return l < r, nil
}

func (ee *ExpressionEngine) lessEqual(left, right interface{}) (interface{}, error) {
	l, r, err := ee.toNumbers(left, right)
	if err != nil {
		return false, err
	}
	return l <= r, nil
}

// toNumbers converts values to float64 for numeric comparison
func (ee *ExpressionEngine) toNumbers(left, right interface{}) (float64, float64, error) {
	l, err := ee.toFloat64(left)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot convert left operand to number: %v", err)
	}

	r, err := ee.toFloat64(right)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot convert right operand to number: %v", err)
	}

	return l, r, nil
}

// toFloat64 converts various types to float64
func (ee *ExpressionEngine) toFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int64:
		return float64(v), nil
	case int:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to number", val)
	}
}

// ClearCache clears the compiled expression cache
func (ee *ExpressionEngine) ClearCache() {
	ee.compiledExpressions = sync.Map{}
}
