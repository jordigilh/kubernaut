# Priority Assignment Policy — Score-Based Aggregation
# BR-SP-070: Priority assignment via Rego policy
# Issue #98: Refactored from N*M combinatorial rules to score-based aggregation
#
# IMPORTANT: This policy is MANDATORY. Go code has NO fallback logic.
# Operators MUST customize this policy to match their workflow priorities.
#
# Design: Each dimension (severity, environment, tier) is scored independently.
# Scores are summed into a composite score, then mapped to priority levels
# via threshold rules. Adding a new severity or environment is a single-line change.
#
# Input Schema (per BR-SP-070):
# {
#   "signal": { "severity": "critical", "source": "prometheus" },
#   "environment": "production",
#   "namespace_labels": { "tier": "critical" },
#   "deployment_labels": { "app": "payment-service" }
# }
#
# Output Schema:
# {
#   "priority": "P0",
#   "policy_name": "score-based"
# }
#
# Priority Levels (P0-P3):
# - P0: Critical production issues requiring immediate attention (composite >= 6)
# - P1: High priority issues (composite == 5)
# - P2: Medium priority (composite == 4)
# - P3: Low priority (composite <= 3)

package signalprocessing.priority

import rego.v1

# ============================================================================
# DIMENSION 1: Severity Score (independent)
# ============================================================================

severity_score := 3 if { lower(input.signal.severity) == "critical" }
severity_score := 2 if { lower(input.signal.severity) == "warning" }
severity_score := 1 if { lower(input.signal.severity) == "info" }
default severity_score := 0

# ============================================================================
# DIMENSION 2: Environment Score (independent, max of env + tier)
# ============================================================================
# Tier labels and environment both contribute to the env dimension.
# When both are present, the highest score wins (e.g., staging namespace
# with tier=critical gets env_score=3, not 2).

env_scores contains 3 if { lower(input.environment) == "production" }
env_scores contains 2 if { lower(input.environment) == "staging" }
env_scores contains 1 if { lower(input.environment) == "development" }
env_scores contains 1 if { lower(input.environment) == "test" }

# Tier labels boost the environment score
env_scores contains 3 if { input.namespace_labels["tier"] == "critical" }
env_scores contains 2 if { input.namespace_labels["tier"] == "high" }

env_score := max(env_scores) if { count(env_scores) > 0 }
default env_score := 0

# ============================================================================
# AGGREGATION: Composite Score
# ============================================================================

composite_score := severity_score + env_score

# ============================================================================
# THRESHOLD RULES: Composite Score -> Priority Level
# ============================================================================
# Thresholds tuned to preserve backward compatibility with the previous
# N*M combinatorial policy:
#
# P0 (>=6): critical(3)+production(3)=6, critical(3)+tier:critical(3)=6
# P1 (==5): critical(3)+staging(2)=5, warning(2)+production(3)=5
# P2 (==4): critical(3)+dev(1)=4, warning(2)+staging(2)=4, info(1)+production(3)=4
# P3 (<=3): warning(2)+dev(1)=3, info(1)+staging(2)=3, info(1)+dev(1)=2

result := {"priority": "P0", "policy_name": "score-based"} if { composite_score >= 6 }
result := {"priority": "P1", "policy_name": "score-based"} if { composite_score == 5 }
result := {"priority": "P2", "policy_name": "score-based"} if { composite_score == 4 }
result := {"priority": "P3", "policy_name": "score-based"} if { composite_score < 4; composite_score > 0 }

# ============================================================================
# DEFAULT CATCH-ALL (MANDATORY)
# ============================================================================
# Fires when no severity or environment matches (composite_score == 0).
# Operators SHOULD customize this to match their workflow priorities.

default result := {"priority": "P3", "policy_name": "default-catch-all"}
