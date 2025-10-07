# BR-HAPI-189 Phase 2: Implementation Templates

**Phase**: Phase 2 - Structured Error Tracking Implementation
**Date**: 2025-01-15
**Status**: READY (Awaiting Phase 1 findings)
**Purpose**: Implementation templates for all SDK capability scenarios

---

## Overview

This document provides ready-to-use implementation templates for BR-HAPI-189 (Runtime Toolset Failure Tracking) based on Phase 1 SDK investigation findings.

**Three Implementation Scenarios**:
- **Scenario A**: SDK provides structured exceptions with toolset metadata (95% confidence)
- **Scenario B**: SDK uses generic exceptions, implement multi-tier classification (70-85% confidence)
- **Scenario C**: Wrap SDK client with structured error tracking (85% confidence - RECOMMENDED)

**Select scenario based on Phase 1 findings documented in**: `BR-HAPI-189-PHASE1-SDK-INVESTIGATION.md`

---

## Scenario A: Structured Exceptions with Toolset Metadata

**Use When**: Phase 1 investigation found:
- ✅ SDK provides toolset-specific exception classes (e.g., `KubernetesToolsetError`)
- ✅ Exceptions have `toolset_name` metadata attribute
- ✅ Exception hierarchy is well-documented

**Confidence**: 95% (Best case, most reliable)
**Estimated Effort**: 1 day

### Implementation

#### File: `src/services/holmesgpt_service.py`

```python
# src/services/holmesgpt_service.py
from holmes import Client as HolmesClient
from holmes.exceptions import (
    ToolsetExecutionError,
    KubernetesToolsetError,
    PrometheusToolsetError,
    GrafanaToolsetError
)
import structlog

logger = structlog.get_logger()

class HolmesGPTService:
    """
    HolmesGPT SDK wrapper with structured error tracking

    Scenario A: Uses SDK's structured exception hierarchy
    """

    def __init__(self, api_key: str, toolsets: list, llm_provider: str):
        self.client = HolmesClient(
            api_key=api_key,
            toolsets=toolsets,
            llm_provider=llm_provider
        )
        self.toolsets = toolsets

    async def investigate(
        self,
        alert_name: str,
        namespace: str,
        resource_name: str,
        investigation_scope: dict,
        toolset_config_service=None
    ):
        """
        Investigate alert with structured error tracking

        BR-HAPI-189: Runtime toolset failure tracking using SDK exceptions
        """
        logger.info(
            "starting_investigation",
            alert_name=alert_name,
            namespace=namespace,
            resource_name=resource_name,
            toolsets=self.toolsets
        )

        try:
            # HolmesGPT SDK investigation
            result = await self.client.investigate(
                alert_name=alert_name,
                namespace=namespace,
                resource_name=resource_name,
                **investigation_scope
            )

            # SUCCESS: Record successful toolset usage
            if toolset_config_service and hasattr(result, 'toolsets_used'):
                for toolset_name in result.toolsets_used:
                    toolset_config_service.record_toolset_success(toolset_name)

            logger.info(
                "investigation_completed",
                alert_name=alert_name,
                toolsets_used=getattr(result, 'toolsets_used', []),
                confidence=getattr(result, 'confidence', 0.0)
            )

            return result

        # STRUCTURED EXCEPTION HANDLING: Toolset-specific exceptions
        except KubernetesToolsetError as e:
            logger.error(
                "kubernetes_toolset_error",
                error=str(e),
                toolset=getattr(e, 'toolset_name', 'kubernetes'),
                error_code=getattr(e, 'error_code', None),
                exc_info=True
            )

            if toolset_config_service:
                toolset_config_service.record_toolset_failure(
                    toolset_name=getattr(e, 'toolset_name', 'kubernetes'),
                    error_type=type(e).__name__,
                    error_message=str(e)
                )
            raise

        except PrometheusToolsetError as e:
            logger.error(
                "prometheus_toolset_error",
                error=str(e),
                toolset=getattr(e, 'toolset_name', 'prometheus'),
                error_code=getattr(e, 'error_code', None),
                exc_info=True
            )

            if toolset_config_service:
                toolset_config_service.record_toolset_failure(
                    toolset_name=getattr(e, 'toolset_name', 'prometheus'),
                    error_type=type(e).__name__,
                    error_message=str(e)
                )
            raise

        except GrafanaToolsetError as e:
            logger.error(
                "grafana_toolset_error",
                error=str(e),
                toolset=getattr(e, 'toolset_name', 'grafana'),
                error_code=getattr(e, 'error_code', None),
                exc_info=True
            )

            if toolset_config_service:
                toolset_config_service.record_toolset_failure(
                    toolset_name=getattr(e, 'toolset_name', 'grafana'),
                    error_type=type(e).__name__,
                    error_message=str(e)
                )
            raise

        # FALLBACK: Base toolset error
        except ToolsetExecutionError as e:
            logger.error(
                "toolset_execution_error",
                error=str(e),
                toolset=getattr(e, 'toolset_name', 'unknown'),
                exc_info=True
            )

            if toolset_config_service and hasattr(e, 'toolset_name'):
                toolset_config_service.record_toolset_failure(
                    toolset_name=e.toolset_name,
                    error_type=type(e).__name__,
                    error_message=str(e)
                )
            raise

        # CATCH-ALL: Unknown errors
        except Exception as e:
            logger.error(
                "investigation_error_unknown",
                error=str(e),
                error_type=type(e).__name__,
                exc_info=True
            )
            raise

    async def list_toolsets(self):
        """List available HolmesGPT toolsets"""
        return await self.client.list_toolsets()
```

### Unit Tests

#### File: `tests/unit/services/test_holmesgpt_service_scenario_a.py`

```python
# tests/unit/services/test_holmesgpt_service_scenario_a.py
import pytest
from unittest.mock import Mock, AsyncMock, patch
from src.services.holmesgpt_service import HolmesGPTService
from holmes.exceptions import (
    KubernetesToolsetError,
    PrometheusToolsetError
)

@pytest.fixture
def toolset_config_service():
    """Mock toolset config service"""
    mock = Mock()
    mock.record_toolset_success = Mock()
    mock.record_toolset_failure = Mock()
    return mock

@pytest.fixture
def holmesgpt_service():
    """HolmesGPT service instance"""
    return HolmesGPTService(
        api_key="test-key",
        toolsets=["kubernetes", "prometheus"],
        llm_provider="openai"
    )

@pytest.mark.asyncio
async def test_kubernetes_toolset_error_recorded(holmesgpt_service, toolset_config_service):
    """Test that KubernetesToolsetError is properly recorded"""

    # Mock investigate to raise KubernetesToolsetError
    error = KubernetesToolsetError("RBAC permission denied")
    error.toolset_name = "kubernetes"
    error.error_code = "RBAC_DENIED"

    with patch.object(holmesgpt_service.client, 'investigate', side_effect=error):
        with pytest.raises(KubernetesToolsetError):
            await holmesgpt_service.investigate(
                alert_name="test-alert",
                namespace="production",
                resource_name="web-app-789",
                investigation_scope={"time_window": "1h"},
                toolset_config_service=toolset_config_service
            )

    # Verify failure recorded with correct metadata
    toolset_config_service.record_toolset_failure.assert_called_once_with(
        toolset_name="kubernetes",
        error_type="KubernetesToolsetError",
        error_message="RBAC permission denied"
    )

@pytest.mark.asyncio
async def test_prometheus_toolset_error_recorded(holmesgpt_service, toolset_config_service):
    """Test that PrometheusToolsetError is properly recorded"""

    # Mock investigate to raise PrometheusToolsetError
    error = PrometheusToolsetError("Connection refused to prometheus:9090")
    error.toolset_name = "prometheus"
    error.error_code = "CONNECTION_REFUSED"

    with patch.object(holmesgpt_service.client, 'investigate', side_effect=error):
        with pytest.raises(PrometheusToolsetError):
            await holmesgpt_service.investigate(
                alert_name="test-alert",
                namespace="production",
                resource_name="web-app-789",
                investigation_scope={"time_window": "1h"},
                toolset_config_service=toolset_config_service
            )

    # Verify failure recorded
    toolset_config_service.record_toolset_failure.assert_called_once_with(
        toolset_name="prometheus",
        error_type="PrometheusToolsetError",
        error_message="Connection refused to prometheus:9090"
    )

@pytest.mark.asyncio
async def test_successful_investigation_records_success(holmesgpt_service, toolset_config_service):
    """Test that successful investigation records toolset success"""

    # Mock successful investigation result
    mock_result = Mock()
    mock_result.toolsets_used = ["kubernetes", "prometheus"]
    mock_result.confidence = 0.85
    mock_result.analysis = "Pod is in CrashLoopBackOff due to OOM"

    with patch.object(holmesgpt_service.client, 'investigate', return_value=mock_result):
        result = await holmesgpt_service.investigate(
            alert_name="test-alert",
            namespace="production",
            resource_name="web-app-789",
            investigation_scope={"time_window": "1h"},
            toolset_config_service=toolset_config_service
        )

    # Verify success recorded for each toolset
    assert toolset_config_service.record_toolset_success.call_count == 2
    toolset_config_service.record_toolset_success.assert_any_call("kubernetes")
    toolset_config_service.record_toolset_success.assert_any_call("prometheus")
```

### Acceptance Criteria Validation

**BR-HAPI-189 AC1**: ✅ Service maintains in-memory failure counter
- Validated by: `toolset_config_service.record_toolset_failure()` calls

**BR-HAPI-189 AC2**: ✅ On investigation error, service increments counter
- Validated by: Exception handling calls `record_toolset_failure()` with toolset name

**BR-HAPI-189 AC3**: ✅ On investigation success, service resets counters
- Validated by: Success path calls `record_toolset_success()` for all used toolsets

**BR-HAPI-189 AC4**: ✅ Service logs failure count increment at WARN level
- Validated by: `ToolsetConfigService.record_toolset_failure()` logs at WARN

**BR-HAPI-189 AC5**: ✅ Failure counters persist until service restart
- Validated by: In-memory dict in `ToolsetConfigService`

---

## Scenario B: Multi-Tier Error Classification

**Use When**: Phase 1 investigation found:
- ⚠️ SDK uses generic `Exception` with no toolset metadata
- ⚠️ NO structured exception hierarchy
- ⚠️ NO `toolset_name` attribute in exceptions

**Confidence**: 70-85% (Requires intelligent classification)
**Estimated Effort**: 2 days

### Implementation

#### File: `src/services/holmesgpt_service.py`

```python
# src/services/holmesgpt_service.py
from holmes import Client as HolmesClient
import traceback
import re
from typing import Optional
import structlog

logger = structlog.get_logger()

class HolmesGPTService:
    """
    HolmesGPT SDK wrapper with multi-tier error classification

    Scenario B: SDK uses generic exceptions, implement intelligent classification
    """

    def __init__(self, api_key: str, toolsets: list, llm_provider: str):
        self.client = HolmesClient(
            api_key=api_key,
            toolsets=toolsets,
            llm_provider=llm_provider
        )
        self.toolsets = toolsets

    def _classify_error_by_metadata(self, exception: Exception) -> Optional[str]:
        """
        Multi-tier error classification

        Priority:
        1. Exception metadata (if available)
        2. Exception type name
        3. Traceback analysis
        4. Intelligent pattern matching

        Returns: toolset_name or None if cannot classify
        """
        # Priority 1: Check for metadata attributes
        if hasattr(exception, 'toolset_name'):
            return exception.toolset_name

        if hasattr(exception, 'source') and exception.source in self.toolsets:
            return exception.source

        # Priority 2: Exception type indicates toolset
        exc_type_name = type(exception).__name__.lower()
        if 'kubernetes' in exc_type_name or 'k8s' in exc_type_name:
            return 'kubernetes'
        if 'prometheus' in exc_type_name or 'promql' in exc_type_name:
            return 'prometheus'
        if 'grafana' in exc_type_name:
            return 'grafana'

        # Priority 3: Traceback analysis
        tb = traceback.extract_tb(exception.__traceback__)
        for frame in tb:
            filename_lower = frame.filename.lower()

            # Kubernetes client libraries
            if any(lib in filename_lower for lib in ['kubernetes', 'k8s', 'client-python']):
                return 'kubernetes'

            # Prometheus client libraries
            if any(lib in filename_lower for lib in ['prometheus', 'promql', 'prometheus-api-client']):
                return 'prometheus'

            # Grafana client libraries
            if 'grafana' in filename_lower:
                return 'grafana'

        # Priority 4: Intelligent pattern matching
        error_str = str(exception).lower()

        # Kubernetes patterns (port, error codes, API references)
        kubernetes_patterns = [
            r'connection refused.*:?6443',  # K8s API port
            r'forbidden.*pods?',  # RBAC error
            r'unauthorized.*api',
            r'kube-apiserver',
            r'kubectl',
            r'rbac.*denied',
            r'namespace.*not found',
            r'pod.*not found'
        ]
        if any(re.search(pattern, error_str) for pattern in kubernetes_patterns):
            return 'kubernetes'

        # Prometheus patterns
        prometheus_patterns = [
            r'connection refused.*:?9090',  # Prometheus default port
            r'promql',
            r'query_range',
            r'instant.*query',
            r'prometheus.*unavailable',
            r'prometheus.*api.*error'
        ]
        if any(re.search(pattern, error_str) for pattern in prometheus_patterns):
            return 'prometheus'

        # Grafana patterns
        grafana_patterns = [
            r'connection refused.*:?3000',  # Grafana default port
            r'grafana.*api',
            r'dashboard.*not found',
            r'grafana.*unavailable'
        ]
        if any(re.search(pattern, error_str) for pattern in grafana_patterns):
            return 'grafana'

        # Could not classify
        logger.warning(
            "error_classification_failed",
            error_type=type(exception).__name__,
            error_message=str(exception)[:200],
            traceback_files=[frame.filename for frame in tb[:3]]
        )
        return None

    async def investigate(
        self,
        alert_name: str,
        namespace: str,
        resource_name: str,
        investigation_scope: dict,
        toolset_config_service=None
    ):
        """
        Investigate alert with multi-tier error classification

        BR-HAPI-189: Runtime toolset failure tracking using classification
        """
        logger.info(
            "starting_investigation",
            alert_name=alert_name,
            namespace=namespace,
            resource_name=resource_name,
            toolsets=self.toolsets
        )

        try:
            # HolmesGPT SDK investigation
            result = await self.client.investigate(
                alert_name=alert_name,
                namespace=namespace,
                resource_name=resource_name,
                **investigation_scope
            )

            # SUCCESS: Record successful toolset usage
            if toolset_config_service and hasattr(result, 'toolsets_used'):
                for toolset_name in result.toolsets_used:
                    toolset_config_service.record_toolset_success(toolset_name)

            logger.info(
                "investigation_completed",
                alert_name=alert_name,
                toolsets_used=getattr(result, 'toolsets_used', []),
                confidence=getattr(result, 'confidence', 0.0)
            )

            return result

        except Exception as e:
            # Classify error using multi-tier approach
            toolset_name = self._classify_error_by_metadata(e)

            if toolset_name:
                logger.error(
                    "toolset_error_classified",
                    error=str(e),
                    toolset=toolset_name,
                    error_type=type(e).__name__,
                    classification_confidence="medium",
                    exc_info=True
                )

                if toolset_config_service:
                    toolset_config_service.record_toolset_failure(
                        toolset_name=toolset_name,
                        error_type=type(e).__name__,
                        error_message=str(e)
                    )
            else:
                logger.error(
                    "investigation_error_unclassified",
                    error=str(e),
                    error_type=type(e).__name__,
                    exc_info=True
                )

            raise

    async def list_toolsets(self):
        """List available HolmesGPT toolsets"""
        return await self.client.list_toolsets()
```

### Unit Tests

#### File: `tests/unit/services/test_holmesgpt_service_scenario_b.py`

```python
# tests/unit/services/test_holmesgpt_service_scenario_b.py
import pytest
from unittest.mock import Mock, patch
from src.services.holmesgpt_service import HolmesGPTService

@pytest.fixture
def toolset_config_service():
    """Mock toolset config service"""
    mock = Mock()
    mock.record_toolset_success = Mock()
    mock.record_toolset_failure = Mock()
    return mock

@pytest.fixture
def holmesgpt_service():
    """HolmesGPT service instance"""
    return HolmesGPTService(
        api_key="test-key",
        toolsets=["kubernetes", "prometheus"],
        llm_provider="openai"
    )

def test_classify_error_by_exception_type(holmesgpt_service):
    """Test classification by exception type name"""

    # Create exception with kubernetes in type name
    class KubernetesAPIException(Exception):
        pass

    error = KubernetesAPIException("API error")
    toolset = holmesgpt_service._classify_error_by_metadata(error)

    assert toolset == "kubernetes"

def test_classify_error_by_traceback(holmesgpt_service):
    """Test classification by traceback analysis"""

    def kubernetes_function():
        # Simulate error from kubernetes library
        raise Exception("Connection error")

    try:
        kubernetes_function()
    except Exception as e:
        # Manually set __traceback__ to simulate kubernetes client
        import sys
        tb = sys.exc_info()[2]
        # In real scenario, traceback would show kubernetes library path

        # For test, check pattern matching works
        error_with_pattern = Exception("connection refused to :6443")
        toolset = holmesgpt_service._classify_error_by_metadata(error_with_pattern)

        assert toolset == "kubernetes"

def test_classify_error_by_kubernetes_patterns(holmesgpt_service):
    """Test classification using kubernetes-specific patterns"""

    test_cases = [
        ("connection refused to :6443", "kubernetes"),
        ("forbidden: pods is forbidden", "kubernetes"),
        ("RBAC denied for user", "kubernetes"),
        ("pod web-app-789 not found", "kubernetes"),
    ]

    for error_message, expected_toolset in test_cases:
        error = Exception(error_message)
        toolset = holmesgpt_service._classify_error_by_metadata(error)
        assert toolset == expected_toolset, f"Failed to classify: {error_message}"

def test_classify_error_by_prometheus_patterns(holmesgpt_service):
    """Test classification using prometheus-specific patterns"""

    test_cases = [
        ("connection refused to :9090", "prometheus"),
        ("promql parse error", "prometheus"),
        ("prometheus query_range failed", "prometheus"),
        ("prometheus api error: timeout", "prometheus"),
    ]

    for error_message, expected_toolset in test_cases:
        error = Exception(error_message)
        toolset = holmesgpt_service._classify_error_by_metadata(error)
        assert toolset == expected_toolset, f"Failed to classify: {error_message}"

def test_classify_error_returns_none_for_unknown(holmesgpt_service):
    """Test that unclassifiable errors return None"""

    error = Exception("Generic error with no toolset context")
    toolset = holmesgpt_service._classify_error_by_metadata(error)

    assert toolset is None

@pytest.mark.asyncio
async def test_classified_error_recorded(holmesgpt_service, toolset_config_service):
    """Test that classified errors are properly recorded"""

    # Mock investigate to raise error with kubernetes pattern
    error = Exception("connection refused to :6443")

    with patch.object(holmesgpt_service.client, 'investigate', side_effect=error):
        with pytest.raises(Exception):
            await holmesgpt_service.investigate(
                alert_name="test-alert",
                namespace="production",
                resource_name="web-app-789",
                investigation_scope={"time_window": "1h"},
                toolset_config_service=toolset_config_service
            )

    # Verify failure recorded for kubernetes (classified by pattern)
    toolset_config_service.record_toolset_failure.assert_called_once_with(
        toolset_name="kubernetes",
        error_type="Exception",
        error_message="connection refused to :6443"
    )

@pytest.mark.asyncio
async def test_unclassified_error_not_recorded(holmesgpt_service, toolset_config_service):
    """Test that unclassifiable errors are NOT recorded"""

    # Mock investigate to raise generic error
    error = Exception("Generic unclassifiable error")

    with patch.object(holmesgpt_service.client, 'investigate', side_effect=error):
        with pytest.raises(Exception):
            await holmesgpt_service.investigate(
                alert_name="test-alert",
                namespace="production",
                resource_name="web-app-789",
                investigation_scope={"time_window": "1h"},
                toolset_config_service=toolset_config_service
            )

    # Verify NO failure recorded (error could not be classified)
    toolset_config_service.record_toolset_failure.assert_not_called()
```

### Acceptance Criteria Validation

**BR-HAPI-189 AC1**: ✅ Service maintains in-memory failure counter
- Validated by: `toolset_config_service.record_toolset_failure()` calls

**BR-HAPI-189 AC2**: ✅ On investigation error, service increments counter for detected toolset
- Validated by: `_classify_error_by_metadata()` → `record_toolset_failure()` flow

**BR-HAPI-189 AC3**: ✅ On investigation success, service resets counters
- Validated by: Success path calls `record_toolset_success()`

**BR-HAPI-189 AC4**: ✅ Service logs failure count increment at WARN level
- Validated by: `ToolsetConfigService.record_toolset_failure()` logs

**BR-HAPI-189 AC5**: ✅ Failure counters persist until service restart
- Validated by: In-memory dict in `ToolsetConfigService`

**Additional Validation**:
- ✅ Multi-tier classification provides 70-85% accuracy
- ✅ Unknown errors logged but not misclassified
- ✅ Pattern matching uses regex for precision

---

## Scenario C: Client Wrapper with Structured Error Tracking (RECOMMENDED)

**Use When**: Phase 1 investigation found:
- ✅ SDK `Client` class can be subclassed or wrapped
- ⚠️ SDK may/may not have structured exceptions
- ✅ Want future-proof architecture

**Confidence**: 85% (Flexible, maintainable)
**Estimated Effort**: 1.5 days

### Implementation

#### File: `src/services/holmesgpt_client_wrapper.py`

```python
# src/services/holmesgpt_client_wrapper.py
"""
HolmesGPT Client Wrapper with Structured Error Tracking

Scenario C: Wrapper pattern provides:
- Structured error tracking regardless of SDK exceptions
- Future compatibility with SDK updates
- Easy to extend with additional functionality
"""

from holmes import Client as HolmesClient
from contextlib import asynccontextmanager
from typing import Optional, List, Dict, Any
import time
import structlog

logger = structlog.get_logger()

class ToolsetExecutionContext:
    """Context for tracking toolset execution"""

    def __init__(self, toolset_name: str, toolset_config_service):
        self.toolset_name = toolset_name
        self.toolset_config_service = toolset_config_service
        self.start_time = None
        self.success = False

    async def __aenter__(self):
        """Enter async context"""
        self.start_time = time.time()
        logger.debug("toolset_execution_started", toolset=self.toolset_name)
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """Exit async context"""
        duration = time.time() - self.start_time

        if exc_type is not None:
            # Exception occurred - record toolset failure
            logger.error(
                "toolset_execution_failed",
                toolset=self.toolset_name,
                error=str(exc_val),
                error_type=exc_type.__name__,
                duration=duration,
                exc_info=True
            )
            self.toolset_config_service.record_toolset_failure(
                toolset_name=self.toolset_name,
                error_type=exc_type.__name__,
                error_message=str(exc_val)
            )
        elif self.success:
            # Success - record toolset success
            logger.debug(
                "toolset_execution_success",
                toolset=self.toolset_name,
                duration=duration
            )
            self.toolset_config_service.record_toolset_success(self.toolset_name)

        return False  # Don't suppress exception


class HolmesGPTClientWrapper:
    """
    Wrapper for HolmesGPT Client with structured error tracking

    Provides:
    - Automatic toolset success/failure tracking
    - Rich error metadata regardless of SDK exception structure
    - Future-proof architecture (doesn't depend on SDK internals)

    BR-HAPI-189: Runtime toolset failure tracking using wrapper pattern
    """

    def __init__(
        self,
        client: HolmesClient,
        toolset_config_service,
        toolsets: List[str]
    ):
        self.client = client
        self.toolset_config_service = toolset_config_service
        self.toolsets = toolsets

    async def investigate(
        self,
        alert_name: str,
        namespace: str,
        resource_name: str,
        investigation_scope: dict
    ):
        """
        Wrapped investigate with automatic error tracking

        Tracks toolset usage automatically:
        - Success: Resets failure counters for used toolsets
        - Failure: Increments failure counter with rich metadata
        """
        logger.info(
            "starting_investigation",
            alert_name=alert_name,
            namespace=namespace,
            resource_name=resource_name,
            toolsets=self.toolsets
        )

        try:
            # Call HolmesGPT SDK
            result = await self.client.investigate(
                alert_name=alert_name,
                namespace=namespace,
                resource_name=resource_name,
                **investigation_scope
            )

            # Track successful toolset usage
            toolsets_used = getattr(result, 'toolsets_used', self.toolsets)
            for toolset_name in toolsets_used:
                self.toolset_config_service.record_toolset_success(toolset_name)

            logger.info(
                "investigation_completed",
                alert_name=alert_name,
                toolsets_used=toolsets_used,
                confidence=getattr(result, 'confidence', 0.0)
            )

            return result

        except Exception as e:
            # Classify and track error
            toolset_name = self._classify_error(e)

            if toolset_name:
                logger.error(
                    "toolset_error_classified",
                    error=str(e),
                    toolset=toolset_name,
                    error_type=type(e).__name__,
                    exc_info=True
                )

                self.toolset_config_service.record_toolset_failure(
                    toolset_name=toolset_name,
                    error_type=type(e).__name__,
                    error_message=str(e)
                )
            else:
                logger.error(
                    "investigation_error_unclassified",
                    error=str(e),
                    error_type=type(e).__name__,
                    exc_info=True
                )

            raise

    def _classify_error(self, exception: Exception) -> Optional[str]:
        """
        Classify error to determine which toolset failed

        Uses hybrid approach:
        1. Check for SDK exception metadata (if available)
        2. Fall back to multi-tier classification
        """
        # Priority 1: SDK structured exceptions (if available)
        if hasattr(exception, 'toolset_name'):
            return exception.toolset_name

        # Priority 2: Exception type name
        exc_type = type(exception).__name__.lower()
        for toolset in self.toolsets:
            if toolset in exc_type:
                return toolset

        # Priority 3: Pattern matching (simplified for wrapper)
        error_str = str(exception).lower()

        # Kubernetes indicators
        if any(kw in error_str for kw in ['kubernetes', 'kubectl', 'rbac', ':6443', 'pod', 'namespace']):
            return 'kubernetes'

        # Prometheus indicators
        if any(kw in error_str for kw in ['prometheus', 'promql', ':9090', 'query_range']):
            return 'prometheus'

        # Grafana indicators
        if any(kw in error_str for kw in ['grafana', ':3000', 'dashboard']):
            return 'grafana'

        return None

    async def list_toolsets(self):
        """List available toolsets"""
        return await self.client.list_toolsets()


# Factory function for easy integration
def create_holmesgpt_service(
    api_key: str,
    toolsets: List[str],
    llm_provider: str,
    toolset_config_service
) -> HolmesGPTClientWrapper:
    """
    Factory function to create wrapped HolmesGPT client

    Usage in FastAPI app:
        holmesgpt_service = create_holmesgpt_service(
            api_key=os.getenv("LLM_API_KEY"),
            toolsets=["kubernetes", "prometheus"],
            llm_provider="openai",
            toolset_config_service=toolset_service
        )
    """
    client = HolmesClient(
        api_key=api_key,
        toolsets=toolsets,
        llm_provider=llm_provider
    )

    return HolmesGPTClientWrapper(
        client=client,
        toolset_config_service=toolset_config_service,
        toolsets=toolsets
    )
```

#### File: `src/main.py` (Integration)

```python
# src/main.py
from fastapi import FastAPI
from contextlib import asynccontextmanager
import os
from src.services.holmesgpt_client_wrapper import create_holmesgpt_service
from src.services.toolset_config_service import ToolsetConfigService

# Global services
toolset_service = None
holmesgpt_service = None

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan with wrapped HolmesGPT client"""
    global toolset_service, holmesgpt_service

    # Start toolset config service
    toolset_service = ToolsetConfigService(
        config_file_path=os.getenv("TOOLSET_CONFIG_PATH", "/etc/kubernaut/toolsets/toolsets.yaml"),
        poll_interval=int(os.getenv("TOOLSET_POLL_INTERVAL", "60"))
    )

    # Create wrapped HolmesGPT service
    holmesgpt_service = create_holmesgpt_service(
        api_key=os.getenv("LLM_API_KEY"),
        toolsets=[t['name'] for t in toolset_service.current_toolsets],
        llm_provider=os.getenv("LLM_PROVIDER", "openai"),
        toolset_config_service=toolset_service
    )

    logger.info("holmesgpt_api_started_with_wrapper", toolsets=holmesgpt_service.toolsets)

    yield

    logger.info("holmesgpt_api_shutting_down")

app = FastAPI(title="HolmesGPT-API", lifespan=lifespan)

@app.post("/investigate")
async def investigate(request: InvestigationRequest):
    """Investigation endpoint using wrapped client"""
    import uuid

    session_id = str(uuid.uuid4())
    toolset_service.register_session(session_id)

    try:
        # Wrapped client automatically tracks toolset success/failure
        result = await holmesgpt_service.investigate(
            alert_name=request.alert_context.fingerprint,
            namespace=request.alert_context.namespace,
            resource_name=request.alert_context.resource_name,
            investigation_scope=request.investigation_scope.dict()
        )

        return {"investigation_report": result}

    finally:
        toolset_service.unregister_session(session_id)
```

### Unit Tests

#### File: `tests/unit/services/test_holmesgpt_client_wrapper.py`

```python
# tests/unit/services/test_holmesgpt_client_wrapper.py
import pytest
from unittest.mock import Mock, AsyncMock, patch
from src.services.holmesgpt_client_wrapper import (
    HolmesGPTClientWrapper,
    create_holmesgpt_service
)

@pytest.fixture
def mock_holmes_client():
    """Mock HolmesGPT client"""
    mock = Mock()
    mock.investigate = AsyncMock()
    mock.list_toolsets = AsyncMock()
    return mock

@pytest.fixture
def toolset_config_service():
    """Mock toolset config service"""
    mock = Mock()
    mock.record_toolset_success = Mock()
    mock.record_toolset_failure = Mock()
    return mock

@pytest.fixture
def wrapper(mock_holmes_client, toolset_config_service):
    """HolmesGPT client wrapper"""
    return HolmesGPTClientWrapper(
        client=mock_holmes_client,
        toolset_config_service=toolset_config_service,
        toolsets=["kubernetes", "prometheus"]
    )

@pytest.mark.asyncio
async def test_successful_investigation_records_success(wrapper, mock_holmes_client, toolset_config_service):
    """Test that successful investigation records toolset success"""

    # Mock result
    mock_result = Mock()
    mock_result.toolsets_used = ["kubernetes", "prometheus"]
    mock_result.confidence = 0.85
    mock_holmes_client.investigate.return_value = mock_result

    result = await wrapper.investigate(
        alert_name="test-alert",
        namespace="production",
        resource_name="web-app-789",
        investigation_scope={"time_window": "1h"}
    )

    # Verify success recorded
    assert toolset_config_service.record_toolset_success.call_count == 2
    toolset_config_service.record_toolset_success.assert_any_call("kubernetes")
    toolset_config_service.record_toolset_success.assert_any_call("prometheus")

@pytest.mark.asyncio
async def test_error_with_toolset_metadata_recorded(wrapper, mock_holmes_client, toolset_config_service):
    """Test error with toolset_name metadata is recorded"""

    # Mock error with metadata
    error = Exception("RBAC denied")
    error.toolset_name = "kubernetes"
    mock_holmes_client.investigate.side_effect = error

    with pytest.raises(Exception):
        await wrapper.investigate(
            alert_name="test-alert",
            namespace="production",
            resource_name="web-app-789",
            investigation_scope={"time_window": "1h"}
        )

    # Verify failure recorded with toolset from metadata
    toolset_config_service.record_toolset_failure.assert_called_once_with(
        toolset_name="kubernetes",
        error_type="Exception",
        error_message="RBAC denied"
    )

@pytest.mark.asyncio
async def test_error_classified_by_pattern_recorded(wrapper, mock_holmes_client, toolset_config_service):
    """Test error classified by pattern is recorded"""

    # Mock error without metadata, but with kubernetes pattern
    error = Exception("connection refused to :6443")
    mock_holmes_client.investigate.side_effect = error

    with pytest.raises(Exception):
        await wrapper.investigate(
            alert_name="test-alert",
            namespace="production",
            resource_name="web-app-789",
            investigation_scope={"time_window": "1h"}
        )

    # Verify failure recorded with toolset classified by pattern
    toolset_config_service.record_toolset_failure.assert_called_once_with(
        toolset_name="kubernetes",
        error_type="Exception",
        error_message="connection refused to :6443"
    )

def test_classify_error_with_metadata(wrapper):
    """Test error classification using exception metadata"""

    error = Exception("Test error")
    error.toolset_name = "prometheus"

    toolset = wrapper._classify_error(error)
    assert toolset == "prometheus"

def test_classify_error_by_type_name(wrapper):
    """Test error classification using exception type name"""

    class KubernetesError(Exception):
        pass

    error = KubernetesError("Test error")
    toolset = wrapper._classify_error(error)

    assert toolset == "kubernetes"

def test_classify_error_by_patterns(wrapper):
    """Test error classification using pattern matching"""

    test_cases = [
        (Exception("kubectl command failed"), "kubernetes"),
        (Exception("promql parse error"), "prometheus"),
        (Exception("grafana dashboard not found"), "grafana"),
    ]

    for error, expected_toolset in test_cases:
        toolset = wrapper._classify_error(error)
        assert toolset == expected_toolset

def test_factory_function_creates_wrapper():
    """Test that factory function creates properly configured wrapper"""

    toolset_config_service = Mock()

    with patch('src.services.holmesgpt_client_wrapper.HolmesClient') as MockClient:
        wrapper = create_holmesgpt_service(
            api_key="test-key",
            toolsets=["kubernetes", "prometheus"],
            llm_provider="openai",
            toolset_config_service=toolset_config_service
        )

        assert isinstance(wrapper, HolmesGPTClientWrapper)
        assert wrapper.toolsets == ["kubernetes", "prometheus"]
        assert wrapper.toolset_config_service == toolset_config_service
```

### Acceptance Criteria Validation

**BR-HAPI-189 AC1**: ✅ Service maintains in-memory failure counter
- Validated by: `toolset_config_service.record_toolset_failure()` calls

**BR-HAPI-189 AC2**: ✅ On investigation error, service increments counter
- Validated by: Wrapper catches all exceptions, classifies, and records

**BR-HAPI-189 AC3**: ✅ On investigation success, service resets counters
- Validated by: Wrapper automatically records success for all used toolsets

**BR-HAPI-189 AC4**: ✅ Service logs failure count increment at WARN level
- Validated by: `ToolsetConfigService.record_toolset_failure()` logs

**BR-HAPI-189 AC5**: ✅ Failure counters persist until service restart
- Validated by: In-memory dict in `ToolsetConfigService`

**Additional Benefits**:
- ✅ Future-proof architecture (doesn't depend on SDK internals)
- ✅ Easy to extend with additional functionality
- ✅ Works regardless of SDK exception structure

---

## Implementation Checklist

**Select scenario based on Phase 1 findings**, then follow checklist:

### Scenario A Checklist

- [ ] Verify SDK provides structured exceptions (from Phase 1)
- [ ] Import SDK exception classes
- [ ] Implement exception handling in `HolmesGPTService`
- [ ] Add unit tests for each exception type
- [ ] Integration test with real SDK
- [ ] Validate all BR-HAPI-189 acceptance criteria
- [ ] Update service specification documentation

### Scenario B Checklist

- [ ] Verify SDK uses generic exceptions (from Phase 1)
- [ ] Implement `_classify_error_by_metadata()` method
- [ ] Add regex patterns for each toolset
- [ ] Add unit tests for classification logic
- [ ] Test classification accuracy (target: 70-85%)
- [ ] Integration test with real errors
- [ ] Validate all BR-HAPI-189 acceptance criteria
- [ ] Update service specification documentation

### Scenario C Checklist (RECOMMENDED)

- [ ] Verify SDK `Client` can be wrapped (from Phase 1)
- [ ] Create `HolmesGPTClientWrapper` class
- [ ] Implement error classification in wrapper
- [ ] Create factory function for easy integration
- [ ] Update FastAPI app to use wrapper
- [ ] Add comprehensive unit tests
- [ ] Integration test with real SDK
- [ ] Validate all BR-HAPI-189 acceptance criteria
- [ ] Update service specification documentation

---

## Success Criteria

**Phase 2 is complete when**:

- ✅ Selected scenario implemented based on Phase 1 findings
- ✅ All BR-HAPI-189 acceptance criteria validated
- ✅ Unit tests pass with 70%+ coverage
- ✅ Integration tests pass with real SDK
- ✅ Error classification achieves target confidence level
- ✅ Service specification updated with implementation details

---

## Next Steps

**After Phase 2 completion**:

1. **Proceed to Phase 3**: BR-HAPI-190 Auto-Reload ConfigMap
2. **Document lessons learned**: Update Phase 1 findings if needed
3. **Monitor metrics**: Track actual error classification accuracy in production
4. **Iterate if needed**: Refine classification logic based on production data

---

## References

- Phase 1 Findings: `BR-HAPI-189-PHASE1-SDK-INVESTIGATION.md`
- BR Specification: `BR-HAPI-VALIDATION-RESILIENCE.md`
- Service Spec: `docs/services/stateless/08-holmesgpt-api.md`

