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
Workflow Response Validator

Validates workflow responses from LLM automatically (not a separate tool).
Validation happens when HAPI parses the LLM's JSON response, enabling
self-correction while context is still available.

Business Requirements:
- BR-AI-023: Hallucination detection (workflow existence validation)
- BR-HAPI-191: Parameter validation in chat session
- BR-HAPI-196: Container image consistency validation

Design Decision: DD-HAPI-002 v1.2 - Workflow Response Validation Architecture

Validation Steps:
1. Workflow Existence: Verify workflow_id exists in catalog
2. Container Image Consistency: Verify container_image matches catalog
3. Parameter Schema: Verify parameters conform to schema (type, required, length, range, enum)
"""

import logging
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)


class _SecurityGateRejection(Exception):
    """Raised internally when DS security gate rejects a workflow (404 with context filters)."""
    def __init__(self, workflow_id: str, context_filters: Dict[str, str]):
        self.workflow_id = workflow_id
        self.context_filters = context_filters
        super().__init__(f"Workflow '{workflow_id}' rejected by security gate")


@dataclass
class ValidationResult:
    """
    Result of workflow response validation.

    Attributes:
        is_valid: True if all validations passed
        errors: List of error messages for failed validations
        validated_container_image: Container image from catalog (always use this)
        schema_hint: Formatted schema hint for LLM self-correction
    """
    is_valid: bool
    errors: List[str] = field(default_factory=list)
    validated_container_image: Optional[str] = None
    schema_hint: Optional[str] = None


class WorkflowResponseValidator:
    """
    Validates workflow response from LLM automatically.

    This is NOT a separate tool - validation happens when parsing
    the LLM's JSON response, enabling self-correction while context
    is still available.

    Business Requirements:
    - BR-AI-023: Hallucination detection
    - BR-HAPI-191: Parameter validation
    - BR-HAPI-196: Container image consistency

    Design Decision: DD-HAPI-002 v1.2
    """

    def __init__(
        self,
        data_storage_client,
        *,
        severity: Optional[str] = None,
        component: Optional[str] = None,
        environment: Optional[str] = None,
        priority: Optional[str] = None,
    ):
        """
        Initialize validator with Data Storage client and optional context filters.

        When context filters are provided, the validator passes them to the DS
        get_workflow_by_id call, enabling the DS security gate (DD-HAPI-017).
        A 404 response with context filters is treated as a context mismatch
        rather than a simple "not found".

        Args:
            data_storage_client: Client for Data Storage service
            severity: Signal severity level (critical/high/medium/low)
            component: Kubernetes resource type (pod/deployment/node/etc.)
            environment: Target environment (production/staging/development)
            priority: Business priority level (P0/P1/P2/P3)
        """
        self.ds_client = data_storage_client
        self._context_filters: Dict[str, str] = {}
        if severity:
            self._context_filters["severity"] = severity
        if component:
            self._context_filters["component"] = component
        if environment:
            self._context_filters["environment"] = environment
        if priority:
            self._context_filters["priority"] = priority

    def validate(
        self,
        workflow_id: str,
        container_image: Optional[str],
        parameters: Dict[str, Any]
    ) -> ValidationResult:
        """
        Comprehensive workflow response validation.

        Validates in order:
        1. Workflow existence (BR-AI-023)
        2. Container image consistency (BR-HAPI-196)
        3. Parameter schema (BR-HAPI-191)

        Args:
            workflow_id: Workflow ID from LLM response
            container_image: Container image from LLM response (can be None)
            parameters: Parameters from LLM response

        Returns:
            ValidationResult with is_valid=True or errors list
        """
        errors: List[str] = []

        # STEP 1: Workflow Existence Validation (BR-AI-023, BR-HAPI-017-003)
        try:
            workflow = self._validate_workflow_exists(workflow_id)
        except _SecurityGateRejection:
            # DD-HAPI-017: Context filter mismatch — different error message
            errors.append(
                f"Workflow '{workflow_id}' does not match your current signal context "
                f"(severity, component, environment, priority). This workflow may exist "
                f"but is not compatible with the current incident context. "
                f"Please select a different workflow using the three-step discovery tools."
            )
            return ValidationResult(is_valid=False, errors=errors)

        if workflow is None:
            errors.append(
                f"Workflow '{workflow_id}' not found in catalog. "
                f"Please select a different workflow from the search results."
            )
            # Can't continue validation without workflow
            return ValidationResult(is_valid=False, errors=errors)

        # STEP 1b: Action-Type Cross-Check (DD-WORKFLOW-016, Gap 3)
        action_type_errors = self._validate_action_type_crosscheck(workflow)
        errors.extend(action_type_errors)

        # STEP 2: Container Image Consistency (BR-HAPI-196)
        image_errors = self._validate_container_image(
            container_image,
            workflow.container_image,
            workflow_id
        )
        errors.extend(image_errors)

        # STEP 3: Parameter Schema Validation (BR-HAPI-191)
        param_errors = self._validate_parameters(parameters, workflow)
        errors.extend(param_errors)

        if errors:
            return ValidationResult(
                is_valid=False,
                errors=errors,
                validated_container_image=workflow.container_image,
                schema_hint=self._format_schema_hint(workflow)
            )

        return ValidationResult(
            is_valid=True,
            errors=[],
            validated_container_image=workflow.container_image
        )

    def _validate_workflow_exists(self, workflow_id: str):
        """
        STEP 1: Validate workflow exists in catalog.

        Business Requirement: BR-AI-023 (Hallucination Detection)
        Business Requirement: BR-HAPI-017-003 (Context Filter Security Gate)

        Calls: GET /api/v1/workflows/{workflow_id} with optional context filters.
        When context filters are provided, the DS security gate evaluates
        workflow compatibility and returns 404 on mismatch (DD-HAPI-017).

        Args:
            workflow_id: Workflow ID to validate

        Returns:
            RemediationWorkflow if found, None if not found

        Raises:
            _SecurityGateRejection: When DS returns 404 and context filters are active
        """
        logger.debug(f"Validating workflow exists: {workflow_id}")

        try:
            # Generated OpenAPI client method: get_workflow_by_id
            # Maps to: GET /api/v1/workflows/{workflowID}
            # DD-WORKFLOW-002 v3.0: workflow_id is UUID primary key
            # DD-HAPI-017: Context filters passed as query params for security gate
            workflow = self.ds_client.get_workflow_by_id(
                workflow_id, **self._context_filters
            )
            if workflow is None:
                logger.info(f"Workflow not found in catalog: {workflow_id}")
            return workflow
        except Exception as e:
            # DD-HAPI-017: Check if this is a 404 from the DS security gate
            if hasattr(e, 'status') and e.status == 404 and self._context_filters:
                logger.info(
                    f"Workflow '{workflow_id}' rejected by security gate — "
                    f"does not match signal context. Filters: {self._context_filters}"
                )
                raise _SecurityGateRejection(workflow_id, self._context_filters) from e
            logger.error(f"Error checking workflow existence: {e}")
            return None

    def _validate_action_type_crosscheck(self, workflow) -> List[str]:
        """
        STEP 1b: Cross-check workflow's action_type against available actions.

        DD-WORKFLOW-016 Gap 3: When context filters are set, queries DS for
        available action types and verifies the selected workflow's action_type
        is in that set. This is a belt-and-suspenders check on top of the
        security gate.

        Only performed when context filters are active (otherwise skip).
        Gracefully degrades on DS errors (returns empty errors list).

        Args:
            workflow: Workflow fetched from DS (has action_type attribute)

        Returns:
            List of error messages (empty if valid or skipped)
        """
        # Skip if no context filters (nothing to cross-check against)
        if not self._context_filters:
            return []

        # Skip if workflow has no action_type
        action_type = getattr(workflow, "action_type", None)
        if not action_type:
            return []

        try:
            # Query DS directly for available action types with context filters
            response = self.ds_client.list_available_actions(**self._context_filters)

            # Extract action_type strings from response
            action_types_data = response.get("action_types", []) if isinstance(response, dict) else []
            available_types = {
                at.get("action_type", "") if isinstance(at, dict) else str(at)
                for at in action_types_data
            }

            if action_type not in available_types:
                logger.info(
                    f"DD-WORKFLOW-016 Gap 3: action_type '{action_type}' not in available "
                    f"actions for context {self._context_filters}. "
                    f"Available: {sorted(available_types)}"
                )
                return [
                    f"Workflow '{workflow.workflow_id}' has action_type '{action_type}' "
                    f"which was not in the available actions for this context. "
                    f"Please use list_available_actions to discover valid actions."
                ]

            logger.debug(
                f"DD-WORKFLOW-016 Gap 3: action_type '{action_type}' confirmed in available actions"
            )
            return []

        except Exception as e:
            # Graceful degradation: don't block validation on DS errors
            logger.warning(
                f"DD-WORKFLOW-016 Gap 3: action_type cross-check failed (graceful degradation): {e}"
            )
            return []

    def _validate_container_image(
        self,
        llm_image: Optional[str],
        catalog_image: str,
        workflow_id: str
    ) -> List[str]:
        """
        STEP 2: Validate container image consistency.

        Business Requirement: BR-HAPI-196 (Container Image Consistency)

        Cases:
        - LLM provides matching image → OK
        - LLM provides null/empty → Use catalog image (OK)
        - LLM provides mismatched image → Error (hallucination)

        Args:
            llm_image: Container image from LLM response
            catalog_image: Container image from workflow catalog
            workflow_id: Workflow ID for error messages

        Returns:
            List of error messages (empty if valid)
        """
        errors: List[str] = []

        if llm_image is None or llm_image == "":
            # LLM didn't specify - we'll use catalog value (OK)
            logger.debug(f"Container image not specified, using catalog: {catalog_image}")
            return []

        if llm_image != catalog_image:
            logger.info(f"Container image mismatch: LLM={llm_image}, Catalog={catalog_image}")
            errors.append(
                f"Container image mismatch for workflow '{workflow_id}': "
                f"you provided '{llm_image}' but catalog has '{catalog_image}'. "
                f"Please use the correct image from the workflow catalog or leave it null."
            )

        return errors

    def _validate_parameters(
        self,
        params: Dict[str, Any],
        workflow
    ) -> List[str]:
        """
        STEP 3: Validate parameters against workflow schema.

        Business Requirement: BR-HAPI-191 (Parameter Validation)

        Validates:
        - Required parameters present
        - Type correctness (string, int, bool, float)
        - String length constraints (min/max)
        - Numeric range constraints (min/max)
        - Enum value validation

        Args:
            params: Parameters from LLM response
            workflow: Workflow with parameter schema

        Returns:
            List of error messages (empty if valid)
        """
        errors: List[str] = []

        # Get parameter schema from workflow
        param_schema = self._get_parameter_schema(workflow)
        if not param_schema:
            logger.debug("No parameter schema found, skipping parameter validation")
            return []

        for param_def in param_schema:
            name = param_def.get("name")
            if not name:
                continue

            value = params.get(name)
            param_type = param_def.get("type", "string")
            required = param_def.get("required", False)

            # Required check
            if required and value is None:
                errors.append(f"Missing required parameter: '{name}'")
                continue

            if value is None:
                # Optional and not provided - OK
                continue

            # Type check
            type_error = self._validate_type(value, param_type, name)
            if type_error:
                errors.append(type_error)
                continue  # Skip other checks if type is wrong

            # String length check
            if param_type == "string" and isinstance(value, str):
                min_length = param_def.get("min_length")
                max_length = param_def.get("max_length")

                if min_length is not None and len(value) < min_length:
                    errors.append(
                        f"Parameter '{name}': length must be >= {min_length}, got {len(value)}"
                    )
                if max_length is not None and len(value) > max_length:
                    errors.append(
                        f"Parameter '{name}': length must be <= {max_length}, got {len(value)}"
                    )

            # Numeric range check
            if param_type in ("int", "float") and isinstance(value, (int, float)):
                minimum = param_def.get("minimum")
                maximum = param_def.get("maximum")

                if minimum is not None and value < minimum:
                    errors.append(
                        f"Parameter '{name}': must be >= {minimum}, got {value}"
                    )
                if maximum is not None and value > maximum:
                    errors.append(
                        f"Parameter '{name}': must be <= {maximum}, got {value}"
                    )

            # Enum check
            enum_values = param_def.get("enum")
            if enum_values and value not in enum_values:
                errors.append(
                    f"Parameter '{name}': must be one of {enum_values}, got '{value}'"
                )

        return errors

    def _get_parameter_schema(self, workflow) -> List[Dict[str, Any]]:
        """
        Extract parameter schema from workflow.

        Args:
            workflow: RemediationWorkflow object

        Returns:
            List of parameter definitions
        """
        if not workflow.parameters:
            return []

        # Handle nested schema structure
        if isinstance(workflow.parameters, dict):
            schema = workflow.parameters.get("schema", {})
            if isinstance(schema, dict):
                return schema.get("parameters", [])

        return []

    def _validate_type(
        self,
        value: Any,
        expected_type: str,
        param_name: str
    ) -> Optional[str]:
        """
        Validate parameter type.

        Args:
            value: Parameter value
            expected_type: Expected type (string, int, bool, float)
            param_name: Parameter name for error messages

        Returns:
            Error message if type invalid, None if valid
        """
        type_map = {
            "string": str,
            "int": int,
            "float": (int, float),  # Accept int as float
            "bool": bool
        }

        expected = type_map.get(expected_type)
        if expected is None:
            # Unknown type - skip validation
            return None

        # Special case: int type should not accept bool (Python considers bool subclass of int)
        if expected_type == "int" and isinstance(value, bool):
            return (
                f"Parameter '{param_name}': expected int, got bool"
            )

        if not isinstance(value, expected):
            return (
                f"Parameter '{param_name}': expected {expected_type}, "
                f"got {type(value).__name__}"
            )

        return None

    def _format_schema_hint(self, workflow) -> str:
        """
        Format schema hint for LLM self-correction.

        Args:
            workflow: Workflow with parameter schema

        Returns:
            Formatted schema hint string
        """
        param_schema = self._get_parameter_schema(workflow)
        if not param_schema:
            return "No parameter schema available."

        hints = ["Parameter schema:"]
        for param in param_schema:
            name = param.get("name", "unknown")
            param_type = param.get("type", "string")
            required = param.get("required", False)

            hint = f"  - {name}: {param_type}"
            if required:
                hint += " (required)"

            # Add constraints
            constraints = []
            if param.get("min_length"):
                constraints.append(f"min_length={param['min_length']}")
            if param.get("max_length"):
                constraints.append(f"max_length={param['max_length']}")
            if param.get("minimum") is not None:
                constraints.append(f"min={param['minimum']}")
            if param.get("maximum") is not None:
                constraints.append(f"max={param['maximum']}")
            if param.get("enum"):
                constraints.append(f"enum={param['enum']}")

            if constraints:
                hint += f" [{', '.join(constraints)}]"

            hints.append(hint)

        return "\n".join(hints)


