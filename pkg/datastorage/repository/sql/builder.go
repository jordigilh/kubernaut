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

package sql

import (
	"fmt"
	"strings"
)

// ========================================
// SQL QUERY BUILDER (V1.0 REFACTOR)
// 📋 Authority: [06-documentation-standards.mdc](mdc:.cursor/rules/06-documentation-standards.mdc)
// ========================================
//
// Fluent SQL query builder to eliminate string concatenation and reduce duplication.
//
// V1.0 REFACTOR Goals:
// - Type-safe query construction
// - Eliminate SQL string concatenation
// - Automatic parameter indexing ($1, $2, etc.)
// - Consistent query patterns across repositories
//
// Business Value:
// - Reduces SQL injection risk
// - Easier to test query logic
// - Consistent query building patterns
// - Reduced code duplication
//
// Usage:
//   query, args := sql.NewBuilder().
//       Select("*").
//       From("users").
//       Where("email = ?", email).
//       Where("status = ?", status).
//       OrderBy("created_at", sql.DESC).
//       Limit(10).
//       Offset(20).
//       Build()
//
// ========================================

// Builder provides fluent SQL query construction.
type Builder struct {
	selectCols []string
	from       string
	where      []whereClause
	orderBy    []orderClause
	limit      *int
	offset     *int
	args       []interface{}
	argIndex   int // Tracks current $N index
}

// whereClause represents a WHERE condition.
type whereClause struct {
	condition string
	args      []interface{}
}

// orderClause represents an ORDER BY clause.
type orderClause struct {
	column    string
	direction string // "ASC" or "DESC"
}

// OrderDirection represents SQL ORDER BY direction.
type OrderDirection string

const (
	ASC  OrderDirection = "ASC"
	DESC OrderDirection = "DESC"
)

// NewBuilder creates a new SQL query builder.
func NewBuilder() *Builder {
	return &Builder{
		selectCols: []string{},
		where:      []whereClause{},
		orderBy:    []orderClause{},
		args:       []interface{}{},
		argIndex:   1, // PostgreSQL parameters start at $1
	}
}

// Select sets the SELECT columns (e.g., "*", "id, name", "COUNT(*)").
func (b *Builder) Select(columns string) *Builder {
	b.selectCols = append(b.selectCols, columns)
	return b
}

// From sets the FROM table.
func (b *Builder) From(table string) *Builder {
	b.from = table
	return b
}

// Where adds a WHERE condition using "?" as placeholder.
// The builder will automatically replace "?" with $1, $2, etc.
//
// Examples:
//
//	.Where("status = ?", "active")
//	.Where("age > ?", 18)
//	.Where("email = ?", user.Email)
func (b *Builder) Where(condition string, args ...interface{}) *Builder {
	if condition == "" {
		return b // Skip empty conditions
	}

	// Replace "?" placeholders with PostgreSQL $N syntax
	parameterizedCondition := condition
	for _, arg := range args {
		placeholder := fmt.Sprintf("$%d", b.argIndex)
		parameterizedCondition = strings.Replace(parameterizedCondition, "?", placeholder, 1)
		b.args = append(b.args, arg)
		b.argIndex++
	}

	b.where = append(b.where, whereClause{
		condition: parameterizedCondition,
		args:      args,
	})
	return b
}

// WhereRaw adds a raw WHERE condition (already using $N syntax).
// Use this when you need custom logic like OR, IN, etc.
//
// Examples:
//
//	.WhereRaw("(status = $1 OR status = $2)", "active", "pending")
//	.WhereRaw("id IN ($1, $2, $3)", 1, 2, 3)
func (b *Builder) WhereRaw(condition string, args ...interface{}) *Builder {
	if condition == "" {
		return b
	}

	b.where = append(b.where, whereClause{
		condition: condition,
		args:      args,
	})
	b.args = append(b.args, args...)
	b.argIndex += len(args)
	return b
}

// OrderBy adds an ORDER BY clause.
//
// Examples:
//
//	.OrderBy("created_at", sql.DESC)
//	.OrderBy("name", sql.ASC)
func (b *Builder) OrderBy(column string, direction OrderDirection) *Builder {
	b.orderBy = append(b.orderBy, orderClause{
		column:    column,
		direction: string(direction),
	})
	return b
}

// Limit sets the LIMIT clause.
func (b *Builder) Limit(limit int) *Builder {
	b.limit = &limit
	return b
}

// Offset sets the OFFSET clause.
func (b *Builder) Offset(offset int) *Builder {
	b.offset = &offset
	return b
}

// Build constructs the final SQL query and returns it with arguments.
// Returns: (query string, args []interface{})
func (b *Builder) Build() (string, []interface{}) {
	var query strings.Builder

	// SELECT
	if len(b.selectCols) == 0 {
		query.WriteString("SELECT *")
	} else {
		query.WriteString("SELECT ")
		query.WriteString(strings.Join(b.selectCols, ", "))
	}

	// FROM
	if b.from != "" {
		query.WriteString(" FROM ")
		query.WriteString(b.from)
	}

	// WHERE
	if len(b.where) > 0 {
		whereParts := make([]string, len(b.where))
		for i, w := range b.where {
			whereParts[i] = w.condition
		}
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(whereParts, " AND "))
	}

	// ORDER BY
	if len(b.orderBy) > 0 {
		orderParts := make([]string, len(b.orderBy))
		for i, o := range b.orderBy {
			orderParts[i] = fmt.Sprintf("%s %s", o.column, o.direction)
		}
		query.WriteString(" ORDER BY ")
		query.WriteString(strings.Join(orderParts, ", "))
	}

	// LIMIT
	if b.limit != nil {
		query.WriteString(fmt.Sprintf(" LIMIT $%d", b.argIndex))
		b.args = append(b.args, *b.limit)
		b.argIndex++
	}

	// OFFSET
	if b.offset != nil {
		query.WriteString(fmt.Sprintf(" OFFSET $%d", b.argIndex))
		b.args = append(b.args, *b.offset)
		b.argIndex++
	}

	return query.String(), b.args
}

// BuildCount builds a COUNT(*) query using the same WHERE conditions.
// This is useful for pagination where you need both the results and total count.
//
// Returns: (count query string, args []interface{})
func (b *Builder) BuildCount() (string, []interface{}) {
	var query strings.Builder

	// SELECT COUNT(*)
	query.WriteString("SELECT COUNT(*)")

	// FROM
	if b.from != "" {
		query.WriteString(" FROM ")
		query.WriteString(b.from)
	}

	// WHERE (same as main query)
	if len(b.where) > 0 {
		whereParts := make([]string, len(b.where))
		for i, w := range b.where {
			whereParts[i] = w.condition
		}
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(whereParts, " AND "))
	}

	// Note: COUNT queries don't include ORDER BY, LIMIT, OFFSET

	// Return only the WHERE args (not LIMIT/OFFSET args)
	// Calculate how many args to exclude (LIMIT and/or OFFSET)
	argsToExclude := 0
	if b.limit != nil {
		argsToExclude++
	}
	if b.offset != nil {
		argsToExclude++
	}

	countArgs := b.args
	if argsToExclude > 0 && len(b.args) >= argsToExclude {
		countArgs = b.args[:len(b.args)-argsToExclude]
	}

	return query.String(), countArgs
}

// CurrentArgIndex returns the current parameter index ($N).
// Useful when you need to manually construct part of a query.
func (b *Builder) CurrentArgIndex() int {
	return b.argIndex
}

// Args returns the current argument list.
// Useful when you need to manually add additional arguments.
func (b *Builder) Args() []interface{} {
	return b.args
}
