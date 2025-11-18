"""
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
"""

"""
Recovery Analysis Endpoint

Business Requirements: BR-HAPI-001 to 050 (Recovery Analysis)

Provides AI-powered recovery strategy recommendations for failed remediation actions.
"""

import logging
import os
import json
import re
from typing import Dict, Any, List, Optional
from fastapi import APIRouter, HTTPException, status

from src.clients.mcp_client import MCPClient
from src.models.recovery_models import RecoveryRequest, RecoveryResponse, RecoveryStrategy
from src.toolsets.workflow_catalog import WorkflowCatalogToolset

logger = logging.getLogger(__name__)

# HolmesGPT SDK imports
from holmes.config import Config
from holmes.core.models import InvestigateRequest, InvestigationResult
from holmes.core.investigation import investigate_issues

# Minimal DAL for HolmesGPT SDK integration (no Robusta Platform)
class MinimalDAL:
    """
    Minimal DAL for HolmesGPT SDK integration (no Robusta Platform)

    Architecture Decision (DD-HOLMESGPT-014):
    Kubernaut does NOT integrate with Robusta Platform.

    Kubernaut Provides Equivalent Features Via:
    - Workflow catalog → PostgreSQL with Data Storage Service (not Robusta Platform)
    - Historical data → Context API (not Supabase)
    - Custom investigation logic → Rego policies in RemediationExecution Controller
    - LLM credentials → Kubernetes Secrets (not database)
    - Remediation state → CRDs (RemediationRequest, AIAnalysis, RemediationExecution)

    Result: No Robusta Platform database integration needed.

    This MinimalDAL satisfies HolmesGPT SDK's DAL interface requirements
    without connecting to any Robusta Platform database.

    Note: We still install supabase/postgrest dependencies (~50MB) because
    the SDK requires them, but this class ensures they're never used at runtime.

    See: docs/decisions/DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md
    """
    def __init__(self, cluster_name=None):
        self.cluster = cluster_name
        self.cluster_name = cluster_name  # Backwards compatibility
        self.enabled = False  # Disable Robusta platform features
        logger.info(f"Using MinimalDAL (no Robusta Platform) for cluster={cluster_name}")

    def get_issue_data(self, issue_id):
        """
        Historical issue data (NOT USED)

        Kubernaut: Context API provides historical data via separate service
        """
        return None

    def get_resource_instructions(self, resource_type, issue_type):
        """
        Custom investigation runbooks (NOT USED)

        Kubernaut: Rego policies in RemediationExecution Controller provide custom logic

        Returns None to signal no custom runbooks (SDK will use defaults)
        """
        return None

    def get_global_instructions_for_account(self):
        """
        Account-level investigation guidelines (NOT USED)

        Kubernaut: RemediationExecution Controller manages investigation flow

        Returns None to signal no global instructions (SDK will use defaults)
        """
        return None

router = APIRouter()


def _get_holmes_config(app_config: Dict[str, Any] = None) -> Config:
    """
    Initialize HolmesGPT SDK Config from environment variables and app config

    Required environment variables:
    - LLM_MODEL: Full litellm-compatible model identifier (e.g., "provider/model-name")
    - LLM_ENDPOINT: Optional LLM API endpoint

    Note: LLM_MODEL should include the litellm provider prefix if needed
    Examples:
    - "gpt-4" (OpenAI - no prefix needed)
    - "provider_name/model-name" (other providers)
    """
    # Check if running in dev mode (stub implementation)
    dev_mode = os.getenv("DEV_MODE", "false").lower() == "true"
    if dev_mode:
        return None  # Signal to use stub implementation

    # Get formatted model name for litellm (supports Ollama, OpenAI, Claude, Vertex AI)
    from src.extensions.llm_config import (
        get_model_config_for_sdk,
        prepare_toolsets_config_for_sdk,
        register_workflow_catalog_toolset
    )

    try:
        model_name, provider = get_model_config_for_sdk(app_config)
    except ValueError as e:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=str(e)
        )

    # Prepare toolsets configuration (BR-HAPI-002: Enable toolsets by default, BR-HAPI-250: Workflow catalog)
    toolsets_config = prepare_toolsets_config_for_sdk(app_config)

    # Get MCP servers configuration from app_config
    # MCP servers are registered as toolsets by the SDK's ToolsetManager
    mcp_servers_config = None
    if app_config and "mcp_servers" in app_config:
        mcp_servers_config = app_config["mcp_servers"]
        logger.info(f"Registering MCP servers: {list(mcp_servers_config.keys())}")

    # Create HolmesGPT SDK Config
    config_data = {
        "model": model_name,
        "api_base": os.getenv("LLM_ENDPOINT"),
        "toolsets": toolsets_config,
        "mcp_servers": mcp_servers_config,
    }

    try:
        config = Config(**config_data)

        # BR-HAPI-250: Register workflow catalog toolset programmatically
        config = register_workflow_catalog_toolset(config, app_config)

        logger.info(f"Initialized HolmesGPT SDK config: model={model_name}, toolsets={list(config.toolset_manager.toolsets.keys())}")
        return config
    except Exception as e:
        logger.error(f"Failed to initialize HolmesGPT config: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"LLM configuration error: {str(e)}"
        )


def _create_investigation_prompt(request_data: Dict[str, Any]) -> str:
    """
    Create investigation prompt with complete ADR-041 v3.3 hybrid format.

    Reference: ADR-041 v3.3 - LLM Prompt and Response Contract
    """
    # Extract fields
    signal_type = request_data.get("signal_type", "Unknown")
    severity = request_data.get("severity", "unknown")
    namespace = request_data.get("resource_namespace", "unknown")
    resource_kind = request_data.get("resource_kind", "unknown")
    resource_name = request_data.get("resource_name", "unknown")
    environment = request_data.get("environment", "unknown")
    priority = request_data.get("priority", "P2")
    risk_tolerance = request_data.get("risk_tolerance", "medium")
    business_category = request_data.get("business_category", "standard")

    failed_action = request_data.get("failed_action", {})
    failure_context = request_data.get("failure_context", {})
    error_message = failure_context.get("error_message", "Unknown error")
    description = failure_context.get("description", "")

    # Timing information
    firing_time = request_data.get('firing_time', 'Unknown')
    received_time = request_data.get('received_time', 'Unknown')

    # Deduplication and storm
    is_duplicate = request_data.get('is_duplicate', False)
    occurrence_count = request_data.get('occurrence_count', 0)
    first_seen = request_data.get('first_seen', 'Unknown')
    last_seen = request_data.get('last_seen', 'Unknown')
    is_storm = request_data.get('is_storm', False)
    storm_alert_count = request_data.get('storm_alert_count', 0)
    storm_type = request_data.get('storm_type', 'Unknown')
    storm_window = request_data.get('storm_window', '5m')
    affected_resources = request_data.get('affected_resources', [])

    # Cluster context
    cluster_name = request_data.get('cluster_name', 'unknown')
    signal_source = request_data.get('signal_source', 'unknown')
    signal_labels = request_data.get('signal_labels', {})

    # Generate contextual descriptions
    priority_descriptions = {
        "P0": f"P0 (highest priority) - This is a {business_category} service requiring immediate attention",
        "P1": "P1 (high priority) - This service requires prompt attention",
        "P2": "P2 (medium priority) - This service requires timely resolution",
        "P3": "P3 (low priority) - This service can be addressed during normal operations"
    }

    risk_guidance = {
        "low": "low (conservative remediation required - avoid aggressive restarts or scaling)",
        "medium": "medium (balanced approach - standard remediation actions permitted)",
        "high": "high (aggressive remediation permitted - prioritize recovery speed)"
    }

    priority_desc = priority_descriptions.get(priority, f"{priority} - Standard priority")
    risk_desc = risk_guidance.get(risk_tolerance, f"{risk_tolerance} risk tolerance")

    # Build incident summary with natural language
    incident_summary = f"A **{severity} {signal_type} event** from **{signal_source}** has occurred in the **{namespace}/{resource_kind}/{resource_name}**."

    # Add deduplication fact if duplicate
    if is_duplicate and occurrence_count > 0:
        incident_summary += f" **Alert fired {occurrence_count} times**."

    # Add storm fact if storm detected
    if is_storm:
        resource_count = len(affected_resources) if affected_resources else "multiple"
        incident_summary += f" **Storm detected**: {storm_type} type, {storm_alert_count} alerts, {resource_count} resources."

    incident_summary += f"\n{error_message}"

    # Build complete ADR-041 v3.1 hybrid prompt
    prompt = f"""# Incident Analysis Request

## Incident Summary

{incident_summary}

**Business Impact Assessment**:
- **Priority**: {priority_desc}
- **Environment**: {environment}
- **Risk Tolerance**: {risk_desc}

**Technical Details**:
- Signal Type: {signal_type}
- Severity: {severity}
- Resource: {namespace}/{resource_kind}/{resource_name}
- Error: {error_message}
- Failed Action: {failed_action.get('type', 'N/A')} (target: {failed_action.get('target', 'N/A')})

## Error Details (FOR RCA INVESTIGATION)
- Error Message: {error_message}
- Description: {description if description else 'N/A'}
- Firing Time: {firing_time}
- Received Time: {received_time}
"""

    # Add Deduplication Context if applicable
    if is_duplicate and occurrence_count > 0:
        prompt += f"""
## Deduplication Context (FOR RCA INVESTIGATION)
- Is Duplicate: {is_duplicate}
- First Seen: {first_seen}
- Last Seen: {last_seen}
- Occurrence Count: {occurrence_count}

**What Deduplication Means**:
Deduplication tracks duplicate alerts from the monitoring system (Prometheus/Kubernetes). When the same
condition persists, Prometheus fires the same alert every evaluation interval (30-60 seconds). The Gateway
deduplicates these within a 5-minute window to avoid creating multiple RemediationRequest CRDs for the
same ongoing issue.

**RCA Implications**:
- `occurrence_count > 1` means the condition has been **continuously present** since `first_seen`
- This indicates a **persistent, ongoing issue** - not that remediation was attempted and failed
- Focus on understanding why the condition persists, not why remediation failed
- Higher occurrence counts suggest the condition is stable/consistent, not intermittent
"""

    # Add Storm Detection if applicable
    if is_storm:
        prompt += f"""
## Storm Detection (FOR RCA INVESTIGATION)
- Is Storm: {is_storm}
- Storm Type: {storm_type}
- Storm Window: {storm_window}
- Storm Alert Count: {storm_alert_count}
- Affected Resources: {len(affected_resources) if affected_resources else 'Unknown'}
"""
        if affected_resources and len(affected_resources) <= 10:
            prompt += "\n**Affected Resources List**:\n"
            for resource in affected_resources:
                prompt += f"- {resource}\n"
        elif affected_resources:
            prompt += f"\n**Affected Resources** (showing first 10 of {len(affected_resources)}):\n"
            for resource in affected_resources[:10]:
                prompt += f"- {resource}\n"

    # Add Cluster Context
    prompt += f"""
## Cluster Context (FOR RCA INVESTIGATION)
- Cluster: {cluster_name}
- Signal Source: {signal_source}
- Signal Labels: {signal_labels if signal_labels else 'N/A'}

## Business Context (FOR WORKFLOW SEARCH - NOT FOR RCA)
These fields are used by MCP workflow search tools to match workflows.
You do NOT need to consider these in your RCA analysis.

- Environment: {environment}
- Priority: {priority}
- Business Category: {business_category}
- Risk Tolerance: {risk_tolerance}

**Note**: When you call MCP workflow search tools (e.g., `search_workflow_catalog`), you must
pass these business context fields as parameters.

## Required Analysis

**INVESTIGATION APPROACH**:
Perform independent Root Cause Analysis (RCA) using available tools based on the signal type and incident context.

**Available Tools**:
- Kubernetes investigation tools (kubectl, API queries)
- Prometheus/metrics tools (if applicable to signal source)
- Log analysis tools
- Other tools as appropriate for the signal source

**Analysis Steps** (adapt based on signal source and incident):
1. Investigate the signal using appropriate tools for the signal source
2. Gather relevant context and evidence
3. Perform Root Cause Analysis based on your investigation
4. Formulate remediation strategies based on your RCA findings

**Guidance**:
- Use tools appropriate for the signal source (e.g., Kubernetes for pod failures, Prometheus for metric alerts)
- Base your analysis on actual investigation findings, not assumptions
- Consider cluster state and resource availability
- Focus on technical remediation based on RCA findings


## Your Investigation Workflow

**CRITICAL**: Follow this sequence in order. Do NOT search for workflows before investigating.

### Phase 1: Investigate the Incident
Use available tools to investigate the incident:
- Check pod status, events, and logs (kubectl)
- Review resource usage and limits
- Examine node conditions
- Analyze metrics from signal source (if prometheus-adapter)

**Goal**: Understand what actually happened and why.

**Input Signal Provided**: {signal_type} (starting point for investigation)

### Phase 2: Determine Root Cause (RCA)
Based on your investigation findings, identify the root cause.
Is the input signal the root cause, or just a symptom?

### Phase 3: Identify Signal Type That Describes the Effect
Based on your RCA, determine the signal_type that best describes the effect:

**If investigation confirms input signal is the root cause**:
- Input: OOMKilled → Investigation confirms memory limit exceeded → Use "OOMKilled"

**If investigation reveals different root cause**:
- Input: OOMKilled → Investigation shows node memory pressure → Use "NodePressure" or "Evicted"

**Important**: The signal_type for workflow search comes from YOUR investigation findings, not the input signal.

### Phase 4: Search for Workflow (MANDATORY)
**YOU MUST** call MCP `search_workflow_catalog` tool with:
- **Query**: `"<YOUR_RCA_SIGNAL_TYPE> <YOUR_RCA_SEVERITY>"`
- **Label Filters**: Business context values

**This step is REQUIRED** - you cannot skip workflow search. If the tool is available, you must invoke it.

### Phase 5: Return Summary + JSON Payload
Provide natural language summary + structured JSON with workflow and parameters.

**If MCP search succeeds**:
```json
{{
  "root_cause_analysis": {{
    "summary": "Brief summary of root cause from investigation",
    "severity": "critical|high|medium|low",
    "contributing_factors": ["factor1", "factor2"]
  }},
  "selected_workflow": {{
    "workflow_id": "workflow-id-from-mcp-search",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Why your RCA findings led to this workflow selection",
    "parameters": {{
      "PARAM_NAME": "value-from-investigation"
    }}
  }}
}}
```

**If MCP search fails or returns no workflows**:
```json
{{
  "root_cause_analysis": {{
    "summary": "Root cause from investigation",
    "severity": "critical|high|medium|low",
    "contributing_factors": ["factor1", "factor2"]
  }},
  "selected_workflow": null,
  "rationale": "MCP search failed: [error details]. RCA completed but workflow selection unavailable."
}}
```

## RCA Severity Assessment

After your investigation, assess the severity of the root cause using these levels.

**IMPORTANT**: Your RCA severity may differ from the input signal severity. Use your analysis to determine the actual severity based on business impact.

### Severity Levels:

**critical** - Immediate remediation required
- Production service completely unavailable
- Data loss or corruption occurring
- Security breach actively exploited
- SLA violation in progress
- Revenue-impacting outage
- Affects >50% of users

**high** - Urgent remediation needed
- Significant service degradation (>50% performance loss)
- High error rate (>10% of requests failing)
- Production issue escalating toward critical
- Affects 10-50% of users
- SLA at risk

**medium** - Remediation recommended
- Minor service degradation (<50% performance loss)
- Moderate error rate (1-10% of requests failing)
- Non-production critical issues
- Affects <10% of users
- Staging/development critical issues

**low** - Remediation optional
- Informational issues
- Optimization opportunities
- Development environment issues
- No user impact
- Capacity planning alerts

## MCP Workflow Search Guidance

When searching for remediation workflows, use this taxonomy:

**Query Format**: `<signal_type> <severity> [optional_keywords]`
- Example: `"OOMKilled critical"` or `"CrashLoopBackOff high"`
- Use canonical Kubernetes event reasons for signal_type (from your RCA assessment)
- Use your RCA severity assessment (may differ from input signal)

**Canonical Signal Types** (examples - use any canonical Kubernetes event reason):
- `OOMKilled`: Container exceeded memory limit and was killed
- `CrashLoopBackOff`: Container repeatedly crashing
- `ImagePullBackOff`: Cannot pull container image
- `Evicted`: Pod evicted due to resource pressure
- `NodeNotReady`: Node is not ready
- `PodPending`: Pod stuck in pending state
- `FailedScheduling`: Scheduler cannot place pod
- `BackoffLimitExceeded`: Job exceeded retry limit
- `DeadlineExceeded`: Job exceeded active deadline
- `FailedMount`: Volume mount failed

**Note**: These are common examples. Use any canonical Kubernetes event reason that matches your RCA findings.
For complete list, see: https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/event-v1/#Event

**Label Parameters** (for MCP workflow search):
1. **signal_type** (Technical - from your RCA assessment)
2. **severity** (Technical - from your RCA assessment)
3. **environment** (Business - pass-through: `{environment}`)
4. **priority** (Business - pass-through: `{priority}`)
5. **risk_tolerance** (Business - pass-through: `{risk_tolerance}`)
6. **business_category** (Business - pass-through: `{business_category}`)

**Search Optimization**:
- Exact label matching increases confidence score
- Workflow descriptions should start with `"<signal_type> <severity>:"`
- Use all 6 label parameters for filtering

## Expected Response Format

Provide your analysis in two parts:

### Part 1: Natural Language Analysis

Explain your investigation findings, root cause analysis, and reasoning for workflow selection.

### Part 2: Structured JSON

```json
{{
  "root_cause_analysis": {{
    "summary": "Brief summary of root cause",
    "severity": "critical|high|medium|low",
    "contributing_factors": ["factor1", "factor2"]
  }},
  "selected_workflow": {{
    "workflow_id": "workflow-id-from-mcp-search-results",
    "version": "1.0.0",
    "confidence": 0.95,
    "rationale": "Why your search parameters led to this workflow selection (based on RCA findings)",
    "parameters": {{
      "PARAM_NAME": "value",
      "ANOTHER_PARAM": "value"
    }}
  }}
}}
```

**IMPORTANT**:
- Select ONE workflow per incident
- Populate ALL required parameters from the workflow schema
- Use your RCA findings to determine parameter values
- Pass-through business context fields (environment, priority, risk_tolerance, business_category) to MCP search
"""

    return prompt


def _parse_investigation_result(investigation: InvestigationResult, request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Parse HolmesGPT InvestigationResult into RecoveryResponse format

    Extracts recovery strategies from LLM analysis
    """
    incident_id = request_data.get("incident_id")

    # Parse LLM analysis for recovery strategies
    analysis_text = investigation.analysis or ""

    # Extract strategies from analysis
    # For GREEN phase, use simple parsing
    # REFACTOR phase: Use structured output or more sophisticated parsing
    strategies = _extract_strategies_from_analysis(analysis_text)

    # Determine if recovery is possible
    can_recover = len(strategies) > 0
    primary_recommendation = strategies[0].action_type if strategies else None

    # Calculate overall confidence
    analysis_confidence = max([s.confidence for s in strategies]) if strategies else 0.0

    # Extract warnings from analysis
    warnings = _extract_warnings_from_analysis(analysis_text)

    result = RecoveryResponse(
        incident_id=incident_id,
        can_recover=can_recover,
        strategies=strategies,
        primary_recommendation=primary_recommendation,
        analysis_confidence=analysis_confidence,
        warnings=warnings,
        metadata={
            "analysis_time_ms": 2000,  # GREEN phase: static value
            "tool_calls": len(investigation.tool_calls),
            "sdk_version": "holmesgpt-0.1.0"
        }
    )

    return result.model_dump() if hasattr(result, 'model_dump') else result.dict()


def _extract_strategies_from_analysis(analysis_text: str) -> List[RecoveryStrategy]:
    """
    Extract recovery strategies from LLM analysis text

    REFACTOR phase: Attempts to parse structured JSON output, falls back to keyword extraction
    """
    strategies = []

    # REFACTOR Phase: Try to parse structured JSON output
    try:
        # LLM may wrap JSON in markdown code blocks
        json_match = re.search(r'```(?:json)?\s*(\{.*?\})\s*```', analysis_text, re.DOTALL)
        if json_match:
            json_text = json_match.group(1)
        else:
            # Try to find JSON object directly
            json_match = re.search(r'\{.*"strategies".*\}', analysis_text, re.DOTALL)
            json_text = json_match.group(0) if json_match else None

        if json_text:
            parsed = json.loads(json_text)

            # Extract strategies from structured output
            for strategy_data in parsed.get("strategies", []):
                strategies.append(RecoveryStrategy(
                    action_type=strategy_data.get("action_type", "unknown_action"),
                    confidence=float(strategy_data.get("confidence", 0.5)),
                    rationale=strategy_data.get("rationale", "LLM analysis"),
                    estimated_risk=strategy_data.get("estimated_risk", "medium"),
                    prerequisites=strategy_data.get("prerequisites", [])
                ))

            if strategies:
                logger.info(f"Successfully parsed {len(strategies)} strategies from structured JSON")
                return strategies
    except (json.JSONDecodeError, AttributeError, KeyError, ValueError) as e:
        logger.warning(f"Failed to parse structured JSON from LLM: {e}, falling back to keyword extraction")

    # Fallback: Keyword-based extraction (backward compatible with GREEN phase)
    logger.info("Using keyword-based strategy extraction (fallback)")

    if "rollback" in analysis_text.lower():
        strategies.append(RecoveryStrategy(
            action_type="rollback_to_previous_state",
            confidence=0.8,
            rationale="LLM recommends rollback based on analysis",
            estimated_risk="low",
            prerequisites=[]
        ))

    if "scale" in analysis_text.lower() or "retry" in analysis_text.lower():
        strategies.append(RecoveryStrategy(
            action_type="retry_with_modifications",
            confidence=0.7,
            rationale="LLM suggests retry with adjustments",
            estimated_risk="medium",
            prerequisites=[]
        ))

    # If no strategies extracted, provide default
    if not strategies:
        strategies.append(RecoveryStrategy(
            action_type="manual_intervention_required",
            confidence=0.5,
            rationale="Automated recovery not recommended",
            estimated_risk="low",
            prerequisites=["human_review"]
        ))

    return strategies


def _extract_warnings_from_analysis(analysis_text: str) -> List[str]:
    """
    Extract warnings from LLM analysis

    REFACTOR phase: Attempts to parse structured JSON output, falls back to keyword extraction
    """
    warnings = []

    # REFACTOR Phase: Try to parse warnings from structured JSON
    try:
        json_match = re.search(r'```(?:json)?\s*(\{.*?\})\s*```', analysis_text, re.DOTALL)
        if json_match:
            json_text = json_match.group(1)
        else:
            json_match = re.search(r'\{.*"warnings".*\}', analysis_text, re.DOTALL)
            json_text = json_match.group(0) if json_match else None

        if json_text:
            parsed = json.loads(json_text)
            extracted_warnings = parsed.get("warnings", [])
            if extracted_warnings:
                logger.info(f"Successfully parsed {len(extracted_warnings)} warnings from structured JSON")
                return extracted_warnings
    except (json.JSONDecodeError, AttributeError, KeyError) as e:
        logger.debug(f"Failed to parse warnings from JSON: {e}, using keyword extraction")

    # Fallback: Keyword-based extraction
    if "risk" in analysis_text.lower() or "caution" in analysis_text.lower():
        warnings.append("LLM identified potential risks - review carefully")

    if "high load" in analysis_text.lower() or "resource" in analysis_text.lower():
        warnings.append("Resource constraints may affect recovery")

    return warnings


async def _get_workflow_recommendations(request_data: Dict[str, Any], mcp_client: MCPClient) -> List[Dict[str, Any]]:
    """
    Get workflow recommendations from MCP Workflow Catalog

    BR-WORKFLOW-001: Workflow Catalog Integration

    Args:
        request_data: Recovery request data containing:
            - failed_action.type: Type of failed action
            - failure_context.error: Error type
            - context.priority: Priority level
            - context.cluster: Cluster name

    Returns:
        List of recommended workflows from MCP catalog
    """
    try:
        # Extract failure information
        failed_action = request_data.get("failed_action", {})
        failure_context = request_data.get("failure_context", {})
        context = request_data.get("context", {})

        # Extract ALL 7 mandatory labels per DD-WORKFLOW-001
        signal_type = failure_context.get("error", "unknown")
        severity = context.get("severity", "medium")
        component = context.get("component", "pod")
        environment = context.get("environment", "production")
        priority = context.get("priority", "P2")
        risk_tolerance = context.get("risk_tolerance", "medium")
        business_category = context.get("business_category", "*")

        # Search for workflows using all 7 fields
        workflows = await mcp_client.search_workflows(
            signal_type=signal_type,
            severity=severity,
            component=component,
            environment=environment,
            priority=priority,
            risk_tolerance=risk_tolerance,
            business_category=business_category,
            limit=5
        )

        logger.info({
            "event": "workflow_recommendations_retrieved",
            "workflows_found": len(workflows),
            "signal_type": signal_type,
            "severity": severity,
            "component": component,
            "environment": environment,
            "priority": priority,
            "risk_tolerance": risk_tolerance,
            "business_category": business_category
        })

        return workflows

    except Exception as e:
        logger.error({
            "event": "mcp_workflow_integration_error",
            "error": str(e)
        })
        # Graceful degradation - return empty list
        return []


async def analyze_recovery(request_data: Dict[str, Any], mcp_config: Optional[Dict[str, Any]] = None, app_config: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
    """
    Core recovery analysis logic

    Business Requirements: BR-HAPI-001 to 050, BR-WORKFLOW-001 (MCP Workflow Integration)

    Uses HolmesGPT SDK for AI-powered recovery analysis with workflow recommendations
    Falls back to stub implementation in DEV_MODE
    """
    incident_id = request_data.get("incident_id")
    failed_action = request_data.get("failed_action", {})
    failure_context = request_data.get("failure_context", {})

    logger.info({
        "event": "recovery_analysis_started",
        "incident_id": incident_id,
        "action_type": failed_action.get("type")
    })

    # Check if we should use SDK or stub
    config = _get_holmes_config(app_config)

    if config is None:
        # DEV_MODE: Use stub implementation
        logger.info("Using stub implementation (DEV_MODE=true)")
        return _stub_recovery_analysis(request_data)

    # Production mode: Use HolmesGPT SDK with enhanced error handling and workflow recommendations
    try:
        # BR-WORKFLOW-001: Get workflow recommendations from MCP Catalog
        workflow_recommendations = []
        if mcp_config:
            mcp_client = MCPClient(mcp_config)
            workflow_recommendations = await _get_workflow_recommendations(request_data, mcp_client)

        # Add workflow recommendations to request_data for prompt generation
        if workflow_recommendations:
            request_data["workflow_recommendations"] = workflow_recommendations
            logger.info({
                "event": "workflow_recommendations_retrieved",
                "workflows_found": len(workflow_recommendations)
            })

        # Create investigation prompt (now includes historical context if available)
        investigation_prompt = _create_investigation_prompt(request_data)

        # Log the prompt being sent to LLM
        print("\n" + "="*80)
        print("🔍 PROMPT TO LLM PROVIDER (via HolmesGPT SDK)")
        print("="*80)
        print(investigation_prompt)
        print("="*80 + "\n")

        # Create investigation request
        investigation_request = InvestigateRequest(
            source="kubernaut",
            title=f"Recovery analysis for {failed_action.get('type')} failure",
            description=investigation_prompt,
            subject={
                "type": "remediation_failure",
                "incident_id": incident_id,
                "failed_action": failed_action
            },
            context={
                "incident_id": incident_id,
                "issue_type": "remediation_failure"
            },
            source_instance_id="holmesgpt-api"
        )

        # Create minimal DAL (no Robusta Platform database needed)
        dal = MinimalDAL(cluster_name=request_data.get("context", {}).get("cluster"))

        # Debug: Log investigation details before SDK call
        logger.debug({
            "event": "calling_holmesgpt_sdk",
            "incident_id": incident_id,
            "prompt_length": len(investigation_prompt),
            "workflow_count": len(request_data.get("workflow_recommendations", [])),
            "toolsets_enabled": config.toolsets if config else None,
            "prompt_preview": investigation_prompt[:300] + "..." if len(investigation_prompt) > 300 else investigation_prompt
        })

        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # LLM INTERACTION AUDIT LOGGING (Placeholder for future audit traces)
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
        # TODO: Convert these logs to structured audit traces in future iteration
        # These logs capture LLM toolset interactions for monitoring and debugging

        # Log LLM request (prompt sent to model)
        logger.info({
            "event": "llm_request",
            "incident_id": incident_id,
            "model": config.model if config else "unknown",
            "prompt_length": len(investigation_prompt),
            "prompt_preview": investigation_prompt[:500] + "..." if len(investigation_prompt) > 500 else investigation_prompt,
            "toolsets_enabled": config.toolsets if config else [],
            "mcp_servers": list(config.mcp_servers.keys()) if config and hasattr(config, 'mcp_servers') and config.mcp_servers else [],
            "audit_trace_placeholder": "TODO: Convert to structured audit trace"
        })

        # Call HolmesGPT SDK
        logger.info("Calling HolmesGPT SDK for recovery analysis")
        investigation_result = investigate_issues(
            investigate_request=investigation_request,
            dal=dal,
            config=config
        )

        # Log LLM response and tool interactions
        logger.info({
            "event": "llm_response",
            "incident_id": incident_id,
            "has_analysis": bool(investigation_result and investigation_result.analysis),
            "analysis_length": len(investigation_result.analysis) if investigation_result and investigation_result.analysis else 0,
            "analysis_preview": investigation_result.analysis[:500] + "..." if investigation_result and investigation_result.analysis and len(investigation_result.analysis) > 500 else (investigation_result.analysis if investigation_result and investigation_result.analysis else ""),
            "has_tool_calls": hasattr(investigation_result, 'tool_calls') and bool(investigation_result.tool_calls) if investigation_result else False,
            "tool_call_count": len(investigation_result.tool_calls) if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls else 0,
            "audit_trace_placeholder": "TODO: Convert to structured audit trace"
        })

        # Log tool call details if available (SDK-dependent)
        if investigation_result and hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls:
            for idx, tool_call in enumerate(investigation_result.tool_calls):
                logger.info({
                    "event": "llm_tool_call",
                    "incident_id": incident_id,
                    "tool_call_index": idx,
                    "tool_name": getattr(tool_call, 'name', 'unknown'),
                    "tool_arguments": getattr(tool_call, 'arguments', {}),
                    "tool_result": getattr(tool_call, 'result', None),
                    "audit_trace_placeholder": "TODO: Convert to structured audit trace with full request/response"
                })

        # Log the raw LLM response (for debugging)
        print("\n" + "="*80)
        print("🤖 RAW LLM RESPONSE (from HolmesGPT SDK)")
        print("="*80)
        if investigation_result:
            if investigation_result.analysis:
                print(f"Analysis (full):\n{investigation_result.analysis}")
            else:
                print("No analysis returned")
            # Check if tool_calls attribute exists
            if hasattr(investigation_result, 'tool_calls') and investigation_result.tool_calls:
                print(f"\nTool Calls: {len(investigation_result.tool_calls)}")
                for idx, tool_call in enumerate(investigation_result.tool_calls):
                    print(f"  Tool {idx+1}: {getattr(tool_call, 'name', 'unknown')}")
        else:
            print("No result returned from SDK")
        print("="*80 + "\n")
        # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

        # Validate investigation result
        if not investigation_result or not investigation_result.analysis:
            logger.warning({
                "event": "sdk_empty_response",
                "incident_id": incident_id,
                "message": "SDK returned empty analysis"
            })
            return _stub_recovery_analysis(request_data)

        # Parse result into recovery response
        result = _parse_investigation_result(investigation_result, request_data)

        logger.info({
            "event": "recovery_analysis_completed",
            "incident_id": incident_id,
            "sdk_used": True,
            "strategy_count": len(result.get("strategies", [])),
            "confidence": result.get("analysis_confidence"),
            "analysis_length": len(investigation_result.analysis) if investigation_result.analysis else 0
        })

        return result

    except ValueError as e:
        # Configuration or validation errors
        logger.error({
            "event": "sdk_validation_error",
            "incident_id": incident_id,
            "error_type": "ValueError",
            "error": str(e),
            "failed_action": failed_action.get("type")
        })
        return _stub_recovery_analysis(request_data)

    except (ConnectionError, TimeoutError) as e:
        # Network/LLM provider errors
        logger.error({
            "event": "sdk_connection_error",
            "incident_id": incident_id,
            "error_type": type(e).__name__,
            "error": str(e),
            "provider": os.getenv("LLM_MODEL", "unknown")
        })
        return _stub_recovery_analysis(request_data)

    except Exception as e:
        # Catch-all for unexpected errors
        logger.error({
            "event": "sdk_analysis_failed",
            "incident_id": incident_id,
            "error_type": type(e).__name__,
            "error": str(e),
            "error_details": {
                "failed_action_type": failed_action.get("type"),
                "cluster": request_data.get("context", {}).get("cluster"),
                "namespace": failed_action.get("namespace")
            }
        }, exc_info=True)  # Include stack trace for unexpected errors

        # Fallback to stub on SDK failure
        logger.warning(f"Falling back to stub implementation due to {type(e).__name__}")
        return _stub_recovery_analysis(request_data)


def _stub_recovery_analysis(request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Stub implementation for dev mode or SDK fallback

    Provides deterministic responses for testing
    """
    incident_id = request_data.get("incident_id")
    failed_action = request_data.get("failed_action", {})
    failure_context = request_data.get("failure_context", {})

    # Simulate recovery strategy analysis
    strategies = []

    # Example strategy 1: Rollback
    if failed_action.get("type") == "scale_deployment":
        strategies.append(RecoveryStrategy(
            action_type="rollback_to_previous_state",
            confidence=0.85,
            rationale="Safe fallback to known-good state",
            estimated_risk="low",
            prerequisites=["verify_previous_state_available"]
        ))

    # Example strategy 2: Retry with modifications
    strategies.append(RecoveryStrategy(
        action_type="retry_with_reduced_scope",
        confidence=0.70,
        rationale="Attempt recovery with reduced resource requirements",
        estimated_risk="medium",
        prerequisites=["validate_cluster_resources"]
    ))

    # Determine if recovery is possible
    can_recover = len(strategies) > 0
    primary_recommendation = strategies[0].action_type if strategies else None

    # Calculate overall confidence
    analysis_confidence = max([s.confidence for s in strategies]) if strategies else 0.0

    # Generate warnings
    warnings = []
    if failure_context.get("cluster_state") == "high_load":
        warnings.append("Cluster is under high load - recovery may be slower")

    result = RecoveryResponse(
        incident_id=incident_id,
        can_recover=can_recover,
        strategies=strategies,
        primary_recommendation=primary_recommendation,
        analysis_confidence=analysis_confidence,
        warnings=warnings,
        metadata={"analysis_time_ms": 1500, "stub": True}
    )

    logger.info({
        "event": "recovery_analysis_completed",
        "incident_id": incident_id,
        "stub_used": True,
        "can_recover": can_recover,
        "strategy_count": len(strategies),
        "confidence": analysis_confidence
    })

    return result.model_dump() if hasattr(result, 'model_dump') else result.dict()


@router.post("/recovery/analyze", status_code=status.HTTP_200_OK)
async def recovery_analyze_endpoint(request: RecoveryRequest):
    """
    Analyze failed action and provide recovery strategies

    Business Requirement: BR-HAPI-001 (Recovery analysis endpoint)
    Business Requirement: BR-WORKFLOW-001 (MCP Workflow Integration)

    Called by: AIAnalysis Controller (for initial incident RCA and workflow selection)
    """
    try:
        request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()

        # Get MCP config and app config from router config
        mcp_config = None
        app_config = None
        if hasattr(router, 'config'):
            # Extract the specific MCP server config (workflow_catalog_mcp)
            # MCPClient expects config with base_url/timeout, not the entire mcp_servers dict
            mcp_servers = router.config.get("mcp_servers", {})
            if mcp_servers and "workflow_catalog_mcp" in mcp_servers:
                workflow_mcp = mcp_servers["workflow_catalog_mcp"]
                # Convert 'url' to 'base_url' for MCPClient compatibility
                mcp_config = {
                    "base_url": workflow_mcp.get("url"),
                    "timeout": workflow_mcp.get("timeout", 30)
                }
            app_config = router.config

        result = await analyze_recovery(request_data, mcp_config=mcp_config, app_config=app_config)
        return result
    except Exception as e:
        logger.error({
            "event": "recovery_analysis_failed",
            "incident_id": request.incident_id,
            "error": str(e)
        })
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"Recovery analysis failed: {str(e)}"
        )


