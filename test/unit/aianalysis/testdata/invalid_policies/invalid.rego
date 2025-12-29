# Invalid Rego policy for testing graceful degradation
# This has intentional syntax errors (OPA v1 syntax)

package aianalysis.approval

# Syntax error - unclosed brace
require_approval if {
    input.environment == "production"
# Missing closing brace - will cause parse error

