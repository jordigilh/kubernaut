"""
Test data builders - Builder pattern for creating test objects.

This module provides builder classes to create consistent test data
and reduce boilerplate in test files.
"""

from datetime import datetime, timezone
from typing import Dict, List, Optional, Any
from app.models.requests import (
    AlertData, HolmesOptions, KubernetesContext,
    ContextData, InvestigationContext, AskRequest, InvestigateRequest
)
from app.models.responses import (
    AskResponse, InvestigateResponse, HealthStatus, HealthCheckResponse,
    Recommendation, AnalysisResult, InvestigationResult
)
from .test_constants import (
    VALID_SEVERITIES, VALID_ALERT_STATUSES, VALID_MODELS,
    get_test_confidence, get_test_processing_time,
    DEFAULT_K8S_NAMESPACES, DEFAULT_K8S_DEPLOYMENTS
)


class AlertBuilder:
    """Builder for AlertData test objects."""

    def __init__(self):
        self.reset()

    def reset(self):
        """Reset to default values."""
        self._name = "TestAlert"
        self._severity = "warning"
        self._status = "firing"
        self._starts_at = datetime.now(timezone.utc)
        self._ends_at = None
        self._labels = {"instance": "test-pod", "namespace": "default"}
        self._annotations = {"description": "Test alert description"}
        return self

    def with_name(self, name: str):
        self._name = name
        return self

    def with_severity(self, severity: str):
        if severity not in VALID_SEVERITIES:
            raise ValueError(f"Invalid severity: {severity}. Valid: {VALID_SEVERITIES}")
        self._severity = severity
        return self

    def with_status(self, status: str):
        if status not in VALID_ALERT_STATUSES:
            raise ValueError(f"Invalid status: {status}. Valid: {VALID_ALERT_STATUSES}")
        self._status = status
        return self

    def with_labels(self, labels: Dict[str, str]):
        self._labels = labels
        return self

    def with_annotations(self, annotations: Dict[str, str]):
        self._annotations = annotations
        return self

    def critical(self):
        """Shortcut for critical severity."""
        return self.with_severity("critical")

    def resolved(self):
        """Shortcut for resolved status."""
        self._status = "resolved"
        self._ends_at = datetime.now(timezone.utc)
        return self

    def build(self) -> AlertData:
        """Build the AlertData object."""
        return AlertData(
            name=self._name,
            severity=self._severity,
            status=self._status,
            starts_at=self._starts_at,
            ends_at=self._ends_at,
            labels=self._labels,
            annotations=self._annotations
        )


class HolmesOptionsBuilder:
    """Builder for HolmesOptions test objects."""

    def __init__(self):
        self.reset()

    def reset(self):
        """Reset to default values."""
        self._max_tokens = None
        self._temperature = None
        self._timeout = None
        self._context_window = None
        self._include_tools = None
        return self

    def with_max_tokens(self, tokens: int):
        self._max_tokens = tokens
        return self

    def with_temperature(self, temp: float):
        self._temperature = temp
        return self

    def with_timeout(self, timeout: int):
        self._timeout = timeout
        return self

    def with_context_window(self, window: int):
        self._context_window = window
        return self

    def with_tools(self, tools: List[str]):
        self._include_tools = tools
        return self

    def standard(self):
        """Standard configuration for most tests."""
        return (self.with_max_tokens(2000)
                   .with_temperature(0.7)
                   .with_timeout(60)
                   .with_context_window(4096))

    def minimal(self):
        """Minimal configuration."""
        return self.with_max_tokens(1000).with_temperature(0.1)

    def build(self) -> HolmesOptions:
        """Build the HolmesOptions object."""
        return HolmesOptions(
            max_tokens=self._max_tokens,
            temperature=self._temperature,
            timeout=self._timeout,
            context_window=self._context_window,
            include_tools=self._include_tools
        )


class KubernetesContextBuilder:
    """Builder for KubernetesContext test objects."""

    def __init__(self):
        self.reset()

    def reset(self):
        """Reset to default values."""
        self._namespace = None
        self._deployment = None
        self._service = None
        self._pod = None
        self._cluster = None
        return self

    def with_namespace(self, namespace: str):
        self._namespace = namespace
        return self

    def with_deployment(self, deployment: str):
        self._deployment = deployment
        return self

    def with_service(self, service: str):
        self._service = service
        return self

    def with_pod(self, pod: str):
        self._pod = pod
        return self

    def with_cluster(self, cluster: str):
        self._cluster = cluster
        return self

    def production(self):
        """Production environment setup."""
        return (self.with_namespace("production")
                   .with_deployment("api-server")
                   .with_service("api-service")
                   .with_cluster("prod-cluster"))

    def development(self):
        """Development environment setup."""
        return (self.with_namespace("default")
                   .with_deployment("dev-api")
                   .with_service("dev-service"))

    def build(self) -> KubernetesContext:
        """Build the KubernetesContext object."""
        return KubernetesContext(
            namespace=self._namespace,
            deployment=self._deployment,
            service=self._service,
            pod=self._pod,
            cluster=self._cluster
        )


class InvestigationContextBuilder:
    """Builder for InvestigationContext test objects."""

    def __init__(self):
        self.reset()

    def reset(self):
        """Reset to default values."""
        self._kubernetes_context = None
        self._time_range = None
        self._environment = None
        self._related_services = None
        return self

    def with_kubernetes_context(self, k8s_context: KubernetesContext):
        self._kubernetes_context = k8s_context
        return self

    def with_time_range(self, time_range: str):
        self._time_range = time_range
        return self

    def with_environment(self, environment: str):
        self._environment = environment
        return self

    def with_related_services(self, services: List[str]):
        self._related_services = services
        return self

    def production_context(self):
        """Production investigation context."""
        k8s_ctx = KubernetesContextBuilder().production().build()
        return (self.with_kubernetes_context(k8s_ctx)
                   .with_environment("production")
                   .with_time_range("1h")
                   .with_related_services(["database", "cache", "auth"]))

    def build(self) -> InvestigationContext:
        """Build the InvestigationContext object."""
        return InvestigationContext(
            kubernetes_context=self._kubernetes_context,
            time_range=self._time_range,
            environment=self._environment,
            related_services=self._related_services
        )


class AskResponseBuilder:
    """Builder for AskResponse test objects."""

    def __init__(self):
        self.reset()

    def reset(self):
        """Reset to default values."""
        self._response = "This is a test response from HolmesGPT"
        self._analysis = None
        self._recommendations = []
        self._confidence = get_test_confidence()
        self._model_used = "test-model"
        self._tokens_used = None
        self._processing_time = get_test_processing_time()
        self._sources = []
        self._limitations = None
        self._follow_up_questions = None
        return self

    def with_response(self, response: str):
        self._response = response
        return self

    def with_confidence(self, confidence: float):
        self._confidence = confidence
        return self

    def with_model(self, model: str):
        self._model_used = model
        return self

    def with_recommendations(self, recommendations: List[Recommendation]):
        self._recommendations = recommendations
        return self

    def with_sources(self, sources: List[str]):
        self._sources = sources
        return self

    def mock_successful(self):
        """Standard successful response."""
        return (self.with_response("Mock successful response")
                   .with_confidence(0.85)
                   .with_sources(["mock-prometheus", "mock-kubernetes"]))

    def build(self) -> AskResponse:
        """Build the AskResponse object."""
        return AskResponse(
            response=self._response,
            analysis=self._analysis,
            recommendations=self._recommendations,
            confidence=self._confidence,
            model_used=self._model_used,
            tokens_used=self._tokens_used,
            processing_time=self._processing_time,
            sources=self._sources,
            limitations=self._limitations,
            follow_up_questions=self._follow_up_questions
        )


class RecommendationBuilder:
    """Builder for Recommendation test objects."""

    def __init__(self):
        self.reset()

    def reset(self):
        """Reset to default values."""
        self._action = "test_action"
        self._description = "Test recommendation description"
        self._command = None
        self._risk = "low"
        self._confidence = get_test_confidence()
        self._parameters = {}
        self._estimated_time = None
        self._prerequisites = None
        self._rollback_steps = None
        return self

    def with_action(self, action: str):
        self._action = action
        return self

    def with_description(self, description: str):
        self._description = description
        return self

    def with_risk(self, risk: str):
        self._risk = risk
        return self

    def with_confidence(self, confidence: float):
        self._confidence = confidence
        return self

    def high_risk(self):
        """High risk recommendation."""
        return self.with_risk("high").with_confidence(0.95)

    def build(self) -> Recommendation:
        """Build the Recommendation object."""
        return Recommendation(
            action=self._action,
            description=self._description,
            command=self._command,
            risk=self._risk,
            confidence=self._confidence,
            parameters=self._parameters,
            estimated_time=self._estimated_time,
            prerequisites=self._prerequisites,
            rollback_steps=self._rollback_steps
        )


# Factory functions for common test objects
def create_test_alert(severity: str = "warning") -> AlertData:
    """Create a standard test alert."""
    return AlertBuilder().with_severity(severity).build()

def create_test_options() -> HolmesOptions:
    """Create standard test options."""
    return HolmesOptionsBuilder().standard().build()

def create_test_ask_response() -> AskResponse:
    """Create standard test ask response."""
    return AskResponseBuilder().mock_successful().build()

def create_production_context() -> InvestigationContext:
    """Create production investigation context."""
    return InvestigationContextBuilder().production_context().build()
