"""
Tests for Pydantic request and response models.
"""

import pytest
from datetime import datetime, timezone
from typing import Dict, Any
from pydantic import ValidationError

from app.models.requests import (
    HolmesOptions, ContextData, AskRequest, AlertData, InvestigationContext,
    InvestigateRequest, HealthCheckRequest, BatchRequest, ConfigUpdateRequest
)
from app.models.responses import (
    Recommendation, AnalysisResult, AskResponse, InvestigationResult,
    InvestigateResponse, HealthStatus, HealthCheckResponse, ErrorResponse,
    ServiceInfoResponse, BatchResponse, MetricsResponse
)


class TestRequestModels:
    """Test request model validation and serialization."""

    class TestHolmesOptions:
        """Test HolmesOptions model."""

        def test_valid_options(self):
            """Test valid options creation."""
            options = HolmesOptions(
                max_tokens=2000,
                temperature=0.5,
                timeout=30,
                context_window=4096,
                include_tools=["kubernetes", "prometheus"]
            )

            assert options.max_tokens == 2000
            assert options.temperature == 0.5
            assert options.timeout == 30
            assert options.context_window == 4096
            assert options.include_tools == ["kubernetes", "prometheus"]

        def test_default_values(self):
            """Test default values."""
            options = HolmesOptions()

            assert options.max_tokens is None
            assert options.temperature is None
            assert options.timeout is None
            assert options.context_window is None
            assert options.include_tools is None

        def test_max_tokens_validation(self):
            """Test max_tokens validation."""
            # Valid values
            HolmesOptions(max_tokens=1)
            HolmesOptions(max_tokens=10000)

            # Invalid values
            with pytest.raises(ValidationError):
                HolmesOptions(max_tokens=0)

            with pytest.raises(ValidationError):
                HolmesOptions(max_tokens=10001)

        def test_temperature_validation(self):
            """Test temperature validation."""
            # Valid values
            HolmesOptions(temperature=0.0)
            HolmesOptions(temperature=2.0)

            # Invalid values
            with pytest.raises(ValidationError):
                HolmesOptions(temperature=-0.1)

            with pytest.raises(ValidationError):
                HolmesOptions(temperature=2.1)

        def test_timeout_validation(self):
            """Test timeout validation."""
            # Valid values
            HolmesOptions(timeout=1)
            HolmesOptions(timeout=300)

            # Invalid values
            with pytest.raises(ValidationError):
                HolmesOptions(timeout=0)

            with pytest.raises(ValidationError):
                HolmesOptions(timeout=601)

    class TestContextData:
        """Test ContextData model."""

        def test_valid_context(self):
            """Test valid context creation."""
            from app.models.requests import KubernetesContext
            
            context = ContextData(
                kubernetes_context=KubernetesContext(
                    namespace="prod",
                    deployment="api",
                    cluster="main"
                ),
                time_range="1h",
                environment="production",
                related_services=["db", "cache"]
            )

            assert context.kubernetes_context.namespace == "prod"
            assert context.kubernetes_context.deployment == "api"
            assert context.kubernetes_context.cluster == "main"
            assert context.time_range == "1h"
            assert context.environment == "production"
            assert context.related_services == ["db", "cache"]

        def test_empty_context(self):
            """Test empty context creation."""
            context = ContextData()

            assert context.kubernetes_context is None
            assert context.time_range is None
            assert context.environment is None
            assert context.related_services is None

    class TestAskRequest:
        """Test AskRequest model."""

        def test_valid_request(self):
            """Test valid ask request."""
            from app.models.requests import InvestigationContext
            
            request = AskRequest(
                prompt="How do I debug pod crashes?",
                context=InvestigationContext(environment="production"),
                options=HolmesOptions(max_tokens=2000)
            )

            assert request.prompt == "How do I debug pod crashes?"
            assert request.context.environment == "production"
            assert request.options.max_tokens == 2000

        def test_prompt_validation(self):
            """Test prompt validation."""
            # Valid prompts
            AskRequest(prompt="Valid question")
            AskRequest(prompt="A" * 5000)  # Max length

            # Invalid prompts
            with pytest.raises(ValidationError):
                AskRequest(prompt="")  # Empty

            with pytest.raises(ValidationError):
                AskRequest(prompt="   ")  # Whitespace only

            with pytest.raises(ValidationError):
                AskRequest(prompt="A" * 10001)  # Too long

        def test_prompt_trimming(self):
            """Test prompt trimming."""
            request = AskRequest(prompt="  Test prompt  ")
            assert request.prompt == "Test prompt"

        def test_optional_fields(self):
            """Test optional fields."""
            request = AskRequest(prompt="Test")

            assert request.context is None
            assert request.options is None

    class TestAlertData:
        """Test AlertData model."""

        def test_valid_alert(self):
            """Test valid alert creation."""
            now = datetime.now(timezone.utc)
            alert = AlertData(
                name="HighMemoryUsage",
                severity="warning",
                status="firing",
                starts_at=now,
                ends_at=now,
                labels={"instance": "pod-1", "namespace": "prod"},
                annotations={"description": "Memory usage high"},
                fingerprint="abc123",
                generator_url="http://prometheus/graph"
            )

            assert alert.name == "HighMemoryUsage"
            assert alert.severity == "warning"
            assert alert.status == "firing"
            assert alert.starts_at == now
            assert alert.labels["instance"] == "pod-1"

        def test_severity_validation(self):
            """Test severity validation and normalization."""
            now = datetime.now(timezone.utc)

            # Valid severities (case insensitive)
            for severity in ["critical", "warning", "info"]:
                alert = AlertData(name="Test", severity=severity.upper(), status="firing", starts_at=now)
                assert alert.severity == severity.lower()

            # Invalid severity
            with pytest.raises(ValidationError):
                AlertData(name="Test", severity="invalid", status="firing", starts_at=now)

        def test_status_validation(self):
            """Test status validation and normalization."""
            now = datetime.now(timezone.utc)

            # Valid statuses (case insensitive)
            for status in ["firing", "resolved", "pending"]:
                alert = AlertData(name="Test", severity="warning", status=status.upper(), starts_at=now)
                assert alert.status == status.lower()

            # Invalid status
            with pytest.raises(ValidationError):
                AlertData(name="Test", severity="warning", status="invalid", starts_at=now)

        def test_default_collections(self):
            """Test default empty collections."""
            now = datetime.now(timezone.utc)
            alert = AlertData(name="Test", severity="warning", status="firing", starts_at=now)

            assert alert.labels == {}
            assert alert.annotations == {}

    class TestInvestigationContext:
        """Test InvestigationContext model."""

        def test_valid_context(self):
            """Test valid investigation context."""
            from app.models.requests import KubernetesContext
            
            context = InvestigationContext(
                kubernetes_context=KubernetesContext(
                    namespace="prod",
                    deployment="api",
                    service="api-service"
                ),
                time_range="2h",
                environment="production",
                related_services=["db", "cache", "auth"]
            )

            assert context.kubernetes_context.namespace == "prod"
            assert context.kubernetes_context.deployment == "api"
            assert context.time_range == "2h"
            assert context.environment == "production"
            assert len(context.related_services) == 3

        def test_default_values(self):
            """Test default values."""
            context = InvestigationContext()

            assert context.kubernetes_context is None
            assert context.time_range is None
            assert context.environment is None
            assert context.related_services is None

        def test_time_range_values(self):
            """Test time range values."""
            # Should accept any string value since there's no validation
            context = InvestigationContext(time_range="1h")
            assert context.time_range == "1h"
            
            context = InvestigationContext(time_range="custom_range")
            assert context.time_range == "custom_range"

    class TestInvestigateRequest:
        """Test InvestigateRequest model."""

        def test_valid_request(self):
            """Test valid investigate request."""
            alert = AlertData(
                name="TestAlert",
                severity="critical",
                status="firing",
                starts_at=datetime.now(timezone.utc)
            )

            request = InvestigateRequest(
                alert=alert,
                context=InvestigationContext(environment="production"),
                investigation_context=InvestigationContext(time_range="1h"),
                options=HolmesOptions(max_tokens=3000)
            )

            assert request.alert.name == "TestAlert"
            assert request.context.environment == "production"
            assert request.investigation_context.time_range == "1h"
            assert request.options.max_tokens == 3000

        def test_required_alert(self):
            """Test that alert is required."""
            with pytest.raises(ValidationError):
                InvestigateRequest()

        def test_optional_fields(self):
            """Test optional fields."""
            alert = AlertData(
                name="Test",
                severity="warning",
                status="firing",
                starts_at=datetime.now(timezone.utc)
            )

            request = InvestigateRequest(alert=alert)

            assert request.context is None
            assert request.investigation_context is None
            assert request.options is None

    class TestBatchRequest:
        """Test BatchRequest model."""

        def test_valid_batch_request(self):
            """Test valid batch request."""
            operations = [
                {
                    "type": "ask",
                    "prompt": "What's the status?",
                    "options": {"max_tokens": 1000}
                },
                {
                    "type": "investigate",
                    "alert": {
                        "name": "TestAlert",
                        "severity": "warning",
                        "status": "firing",
                        "starts_at": datetime.now(timezone.utc).isoformat()
                    }
                }
            ]

            request = BatchRequest(
                operations=operations,
                parallel=True,
                fail_fast=False,
                timeout=300
            )

            assert len(request.operations) == 2
            assert request.parallel is True
            assert request.fail_fast is False
            assert request.timeout == 300

        def test_operations_validation(self):
            """Test operations validation."""
            # Valid operations
            valid_ops = [
                {"type": "ask", "prompt": "test"},
                {"type": "investigate", "alert": {"name": "test", "severity": "warning", "status": "firing", "starts_at": "2024-01-01T00:00:00Z"}}
            ]
            BatchRequest(operations=valid_ops)

            # Invalid: no type
            with pytest.raises(ValidationError):
                BatchRequest(operations=[{"prompt": "test"}])

            # Invalid: wrong type
            with pytest.raises(ValidationError):
                BatchRequest(operations=[{"type": "invalid", "prompt": "test"}])

            # Invalid: empty operations
            with pytest.raises(ValidationError):
                BatchRequest(operations=[])

            # Invalid: too many operations
            with pytest.raises(ValidationError):
                BatchRequest(operations=[{"type": "ask", "prompt": "test"}] * 11)

        def test_timeout_validation(self):
            """Test timeout validation."""
            operations = [{"type": "ask", "prompt": "test"}]

            # Valid timeouts
            BatchRequest(operations=operations, timeout=1)
            BatchRequest(operations=operations, timeout=1800)

            # Invalid timeouts
            with pytest.raises(ValidationError):
                BatchRequest(operations=operations, timeout=0)

            with pytest.raises(ValidationError):
                BatchRequest(operations=operations, timeout=1801)


class TestResponseModels:
    """Test response model validation and serialization."""

    class TestRecommendation:
        """Test Recommendation model."""

        def test_valid_recommendation(self):
            """Test valid recommendation creation."""
            rec = Recommendation(
                action="scale_deployment",
                description="Scale deployment to handle load",
                command="kubectl scale deployment app --replicas=5",
                risk="low",
                confidence=0.95,
                parameters={"replicas": 5, "deployment": "app"},
                estimated_time="2-3 minutes",
                prerequisites=["Check current resource usage"],
                rollback_steps=["Scale back to original count"]
            )

            assert rec.action == "scale_deployment"
            assert rec.description == "Scale deployment to handle load"
            assert rec.risk == "low"
            assert rec.confidence == 0.95
            assert rec.parameters["replicas"] == 5

        def test_confidence_validation(self):
            """Test confidence validation."""
            # Valid confidence values
            Recommendation(action="test", description="test", risk="low", confidence=0.0)
            Recommendation(action="test", description="test", risk="low", confidence=1.0)

            # Invalid confidence values
            with pytest.raises(ValidationError):
                Recommendation(action="test", description="test", risk="low", confidence=-0.1)

            with pytest.raises(ValidationError):
                Recommendation(action="test", description="test", risk="low", confidence=1.1)

        def test_default_parameters(self):
            """Test default parameters."""
            rec = Recommendation(action="test", description="test", risk="low", confidence=0.8)
            assert rec.parameters == {}

    class TestAnalysisResult:
        """Test AnalysisResult model."""

        def test_valid_analysis(self):
            """Test valid analysis result."""
            analysis = AnalysisResult(
                summary="High memory usage detected",
                root_cause="Memory leak in session handling",
                impact_assessment="Moderate impact on performance",
                urgency_level="medium",
                affected_components=["api-server", "database"],
                related_metrics={"memory_usage": "85%", "cpu_usage": "45%"},
                timeline=[
                    {"time": "10:00", "event": "Memory usage started climbing"},
                    {"time": "10:15", "event": "First OOM kill observed"}
                ]
            )

            assert analysis.summary == "High memory usage detected"
            assert analysis.urgency_level == "medium"
            assert len(analysis.affected_components) == 2
            assert analysis.related_metrics["memory_usage"] == "85%"
            assert len(analysis.timeline) == 2

        def test_default_collections(self):
            """Test default empty collections."""
            analysis = AnalysisResult(summary="Test", urgency_level="low")

            assert analysis.affected_components == []
            assert analysis.related_metrics == {}
            assert analysis.timeline is None

    class TestAskResponse:
        """Test AskResponse model."""

        def test_valid_response(self):
            """Test valid ask response."""
            response = AskResponse(
                response="Based on the symptoms, this appears to be a memory leak",
                analysis=AnalysisResult(summary="Memory leak detected", urgency_level="medium"),
                recommendations=[
                    Recommendation(action="investigate_memory", description="Check memory usage", risk="low", confidence=0.9)
                ],
                confidence=0.85,
                model_used="gpt-4",
                tokens_used=1500,
                processing_time=2.3,
                sources=["prometheus", "kubernetes"],
                limitations=["Limited historical data"],
                follow_up_questions=["What's the current pod memory limit?"]
            )

            assert response.response.startswith("Based on the symptoms")
            assert response.confidence == 0.85
            assert response.model_used == "gpt-4"
            assert response.processing_time == 2.3
            assert len(response.recommendations) == 1
            assert len(response.sources) == 2

        def test_confidence_validation(self):
            """Test confidence validation."""
            # Valid confidence
            AskResponse(
                response="test",
                confidence=0.5,
                model_used="gpt-4",
                processing_time=1.0
            )

            # Invalid confidence
            with pytest.raises(ValidationError):
                AskResponse(
                    response="test",
                    confidence=1.5,
                    model_used="gpt-4",
                    processing_time=1.0
                )

        def test_default_lists(self):
            """Test default empty lists."""
            response = AskResponse(
                response="test",
                confidence=0.8,
                model_used="gpt-4",
                processing_time=1.0
            )

            assert response.recommendations == []
            assert response.sources == []

    class TestHealthStatus:
        """Test HealthStatus model."""

        def test_valid_health_status(self):
            """Test valid health status."""
            status = HealthStatus(
                component="database",
                status="healthy",
                message="Database is responsive",
                last_check=datetime.now(timezone.utc),
                response_time=0.5,
                details={"connections": 10, "queries_per_second": 50}
            )

            assert status.component == "database"
            assert status.status == "healthy"
            assert status.response_time == 0.5
            assert status.details["connections"] == 10

        def test_default_details(self):
            """Test default empty details."""
            status = HealthStatus(
                component="test",
                status="healthy",
                last_check=datetime.now(timezone.utc)
            )

            assert status.details == {}

    class TestHealthCheckResponse:
        """Test HealthCheckResponse model."""

        def test_valid_health_response(self):
            """Test valid health check response."""
            response = HealthCheckResponse(
                healthy=True,
                status="all_systems_operational",
                message="All components are healthy",
                checks={
                    "database": HealthStatus(
                        component="database",
                        status="healthy",
                        last_check=datetime.now(timezone.utc)
                    )
                },
                system_info={"memory": "4GB", "cpu": "2 cores"},
                timestamp=1234567890.0,
                version="1.0.0",
                uptime=3600.0
            )

            assert response.healthy is True
            assert response.status == "all_systems_operational"
            assert len(response.checks) == 1
            assert response.system_info["memory"] == "4GB"
            assert response.uptime == 3600.0

        def test_default_values(self):
            """Test default values."""
            response = HealthCheckResponse(
                healthy=False,
                status="error",
                message="System error",
                timestamp=1234567890.0
            )

            assert response.checks == {}
            assert response.system_info == {}
            assert response.version == "1.0.0"
            assert response.uptime is None

    class TestErrorResponse:
        """Test ErrorResponse model."""

        def test_valid_error_response(self):
            """Test valid error response."""
            error = ErrorResponse(
                error="holmes_service_unavailable",
                message="HolmesGPT service is currently unavailable",
                details={"retry_after": 30, "status": "initializing"},
                timestamp=1234567890.0,
                request_id="req_123456"
            )

            assert error.error == "holmes_service_unavailable"
            assert error.message == "HolmesGPT service is currently unavailable"
            assert error.details["retry_after"] == 30
            assert error.request_id == "req_123456"

        def test_default_timestamp(self):
            """Test default timestamp generation."""
            error = ErrorResponse(
                error="test_error",
                message="Test message"
            )

            # Should have a timestamp close to now
            import time
            assert abs(error.timestamp - time.time()) < 1.0

        def test_default_details(self):
            """Test default empty details."""
            error = ErrorResponse(
                error="test_error",
                message="Test message"
            )

            assert error.details == {}
            assert error.request_id is None


class TestModelSerialization:
    """Test model serialization and deserialization."""

    def test_request_serialization(self):
        """Test request model serialization."""
        request = AskRequest(
            prompt="Test prompt",
            context=InvestigationContext(environment="test"),
            options=HolmesOptions(max_tokens=1000)
        )

        # Serialize to dict
        data = request.model_dump()
        assert data["prompt"] == "Test prompt"
        assert data["context"]["environment"] == "test"
        assert data["options"]["max_tokens"] == 1000

        # JSON serialization
        json_str = request.model_dump_json()
        json_decoded = json_str.decode() if isinstance(json_str, bytes) else json_str
        assert "Test prompt" in json_decoded

        # Deserialize from dict
        new_request = AskRequest(**data)
        assert new_request.prompt == request.prompt
        assert new_request.context.environment == request.context.environment

    def test_response_serialization(self):
        """Test response model serialization."""
        response = AskResponse(
            response="Test response",
            confidence=0.8,
            model_used="gpt-4",
            processing_time=1.5,
            recommendations=[
                Recommendation(action="test", description="test", risk="low", confidence=0.9)
            ]
        )

        # Serialize to dict
        data = response.model_dump()
        assert data["response"] == "Test response"
        assert data["confidence"] == 0.8
        assert len(data["recommendations"]) == 1

        # JSON serialization
        json_str = response.model_dump_json()
        json_decoded = json_str.decode() if isinstance(json_str, bytes) else json_str
        assert "Test response" in json_decoded

        # Deserialize from dict
        new_response = AskResponse(**data)
        assert new_response.response == response.response
        assert new_response.confidence == response.confidence

    def test_nested_model_serialization(self):
        """Test nested model serialization."""
        from app.models.requests import KubernetesContext
        
        now = datetime.now(timezone.utc)
        request = InvestigateRequest(
            alert=AlertData(
                name="TestAlert",
                severity="critical",
                status="firing",
                starts_at=now,
                labels={"app": "web"},
                annotations={"desc": "Test alert"}
            ),
            context=InvestigationContext(
                kubernetes_context=KubernetesContext(namespace="prod"),
                environment="production"
            ),
            investigation_context=InvestigationContext(
                time_range="2h"
            ),
            options=HolmesOptions(max_tokens=3000, temperature=0.1)
        )

        # Serialize
        data = request.model_dump()

        # Verify nested structure
        assert data["alert"]["name"] == "TestAlert"
        assert data["alert"]["severity"] == "critical"
        assert data["context"]["environment"] == "production"
        assert data["investigation_context"]["time_range"] == "2h"
        assert data["options"]["max_tokens"] == 3000

        # Deserialize
        new_request = InvestigateRequest(**data)
        assert new_request.alert.name == request.alert.name
        assert new_request.context.environment == request.context.environment
        assert new_request.investigation_context.time_range == request.investigation_context.time_range

    def test_partial_model_creation(self):
        """Test creating models with only required fields."""
        # Ask request with minimal fields
        ask_request = AskRequest(prompt="Minimal prompt")
        assert ask_request.prompt == "Minimal prompt"
        assert ask_request.context is None
        assert ask_request.options is None

        # Alert with minimal fields
        alert = AlertData(
            name="MinimalAlert",
            severity="info",
            status="firing",
            starts_at=datetime.now(timezone.utc)
        )
        assert alert.name == "MinimalAlert"
        assert alert.labels == {}
        assert alert.annotations == {}

        # Recommendation with minimal fields
        rec = Recommendation(
            action="minimal_action",
            description="Minimal description",
            risk="medium",
            confidence=0.7
        )
        assert rec.action == "minimal_action"
        assert rec.parameters == {}
        assert rec.command is None

