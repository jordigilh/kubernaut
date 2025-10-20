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

from src.models.recovery_models import RecoveryRequest, RecoveryResponse, RecoveryStrategy

logger = logging.getLogger(__name__)

# HolmesGPT SDK imports
from holmes.config import Config
from holmes.core.models import InvestigateRequest, InvestigationResult
from holmes.core.investigation import investigate_issues

# Stateless DAL for HolmesGPT SDK integration (no Robusta Platform)
class MinimalDAL:
    """
    Stateless DAL for HolmesGPT SDK integration

    Architecture Decision (DD-HOLMESGPT-014):
    Kubernaut uses a stateless architecture that does NOT integrate with Robusta Platform.

    Kubernaut Provides Equivalent Features Via:
    - Historical data → Context API (not Supabase)
    - Custom investigation logic → Rego policies in WorkflowExecution Controller
    - LLM credentials → Kubernetes Secrets (not database)
    - Remediation state → CRDs (RemediationRequest, WorkflowExecution)

    Result: No Robusta Platform database integration needed.

    This MinimalDAL satisfies HolmesGPT SDK's DAL interface requirements
    without connecting to any database.

    Note: We still install supabase/postgrest dependencies (~50MB) because
    the SDK requires them, but this class ensures they're never used at runtime.

    See: docs/decisions/DD-HOLMESGPT-014-MinimalDAL-Stateless-Architecture.md
    """
    def __init__(self, cluster_name=None):
        self.cluster = cluster_name
        self.cluster_name = cluster_name  # Backwards compatibility
        self.enabled = False  # Disable Robusta platform features
        logger.info(f"Using MinimalDAL (stateless mode) for cluster={cluster_name}")

    def get_issue_data(self, issue_id):
        """
        Historical issue data (NOT USED)

        Kubernaut: Context API provides historical data via separate service
        """
        return None

    def get_resource_instructions(self, resource_type, issue_type):
        """
        Custom investigation runbooks (NOT USED)

        Kubernaut: Rego policies in WorkflowExecution Controller provide custom logic

        Returns None to signal no custom runbooks (SDK will use defaults)
        """
        return None

    def get_global_instructions_for_account(self):
        """
        Account-level investigation guidelines (NOT USED)

        Kubernaut: WorkflowExecution Controller manages investigation flow

        Returns None to signal no global instructions (SDK will use defaults)
        """
        return None

router = APIRouter()


def _get_holmes_config() -> Config:
    """
    Initialize HolmesGPT SDK Config from environment variables

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

    # Get model name - expect full litellm format from environment
    model_name = os.getenv("LLM_MODEL")
    if not model_name:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="LLM_MODEL environment variable is required"
        )

    # Create minimal config for SDK
    config_data = {
        "model": model_name,  # Pass through as-is
        "api_base": os.getenv("LLM_ENDPOINT"),
    }

    try:
        config = Config(**config_data)
        logger.info(f"Initialized HolmesGPT SDK config: model={model_name}, api_base={config.api_base}")
        return config
    except Exception as e:
        logger.error(f"Failed to initialize HolmesGPT config: {e}")
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail=f"LLM configuration error: {str(e)}"
        )


def _create_investigation_prompt(request_data: Dict[str, Any]) -> str:
    """
    Create investigation prompt for recovery analysis

    Maps recovery request to HolmesGPT investigation prompt format
    """
    failed_action = request_data.get("failed_action", {})
    failure_context = request_data.get("failure_context", {})
    investigation_result = request_data.get("investigation_result", {})
    context = request_data.get("context", {})
    constraints = request_data.get("constraints", {})

    # Build comprehensive prompt
    prompt = f"""# Recovery Analysis Request

## Failed Action
- Type: {failed_action.get('type')}
- Target: {failed_action.get('target')}
- Namespace: {failed_action.get('namespace', 'N/A')}

## Failure Context
- Error: {failure_context.get('error')}
- Error Message: {failure_context.get('error_message')}
"""

    # Add investigation results if available
    if investigation_result:
        prompt += f"""
## Investigation Results
- Root Cause: {investigation_result.get('root_cause', 'Unknown')}
- Symptoms: {', '.join(investigation_result.get('symptoms', []))}
"""

    # Add context
    if context:
        prompt += f"""
## Context
- Cluster: {context.get('cluster')}
- Namespace: {context.get('namespace')}
- Priority: {context.get('priority')}
- Recovery Attempts: {context.get('recovery_attempts', 0)}
"""

    # Add constraints
    if constraints:
        allowed_actions = constraints.get('allowed_actions', [])
        prompt += f"""
## Constraints
- Max Attempts: {constraints.get('max_attempts', 'N/A')}
- Timeout: {constraints.get('timeout', 'N/A')}
- Allowed Actions: {', '.join(allowed_actions) if allowed_actions else 'Any'}
"""

    # Add analysis request with structured output format
    prompt += """
## Required Analysis

Provide recovery strategies for this failed remediation action.

**OUTPUT FORMAT**: Respond with a JSON object containing your analysis:

```json
{
  "analysis_summary": "Brief summary of the failure and recommended approach",
  "root_cause_assessment": "Your assessment of the root cause",
  "strategies": [
    {
      "action_type": "specific_action_name",
      "confidence": 0.85,
      "rationale": "Detailed explanation of why this strategy will work",
      "estimated_risk": "low|medium|high",
      "prerequisites": ["prerequisite1", "prerequisite2"],
      "steps": ["step1", "step2"],
      "expected_outcome": "What success looks like",
      "rollback_plan": "How to revert if this fails"
    }
  ],
  "warnings": ["warning1", "warning2"],
  "context_used": {
    "cluster_state": "assessment of cluster health",
    "resource_availability": "assessment of resources",
    "blast_radius": "potential impact scope"
  }
}
```

**ANALYSIS GUIDANCE**:
- Prioritize strategies by confidence and risk
- Consider root cause, not just symptoms
- Assess cluster resource availability
- Factor in previous recovery attempts (avoid repeated failures)
- Evaluate business impact and priority
- Provide actionable, specific steps
- Include rollback plans for safety
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


async def analyze_recovery(request_data: Dict[str, Any]) -> Dict[str, Any]:
    """
    Core recovery analysis logic

    Business Requirements: BR-HAPI-001 to 050

    Uses HolmesGPT SDK for AI-powered recovery analysis
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
    config = _get_holmes_config()

    if config is None:
        # DEV_MODE: Use stub implementation
        logger.info("Using stub implementation (DEV_MODE=true)")
        return _stub_recovery_analysis(request_data)

    # Production mode: Use HolmesGPT SDK with enhanced error handling
    try:
        # Create investigation prompt
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

        # Create minimal DAL (no database needed for stateless analysis)
        dal = MinimalDAL(cluster_name=request_data.get("context", {}).get("cluster"))

        # Call HolmesGPT SDK
        logger.info("Calling HolmesGPT SDK for recovery analysis")
        investigation_result = investigate_issues(
            investigate_request=investigation_request,
            dal=dal,
            config=config
        )

        # Log the raw LLM response
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
        else:
            print("No result returned from SDK")
        print("="*80 + "\n")

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

    Called by: RemediationProcessor Controller (on remediation failure)
    """
    try:
        request_data = request.model_dump() if hasattr(request, 'model_dump') else request.dict()
        result = await analyze_recovery(request_data)
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
