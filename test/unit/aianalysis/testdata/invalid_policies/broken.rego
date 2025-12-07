# Invalid Rego policy for testing graceful degradation
package aianalysis.approval

# Syntax error - missing closing brace
rule_with_error {
    input.foo == "bar"
# Missing closing brace


