"""
HolmesGPT Python Library Wrapper.

This module provides a wrapper around the actual HolmesGPT Python library,
implementing the real integration with HolmesGPT v0.13.1.
"""

import asyncio
import json
import logging
import os
import tempfile
import time
from datetime import datetime
from typing import Dict, List, Optional, Any, Union
from pathlib import Path

from app.config import Settings
from app.models.requests import HolmesOptions, ContextData, AlertData, InvestigationContext
from app.models.responses import (
    AskResponse, InvestigateResponse, HealthCheckResponse,
    Recommendation, AnalysisResult, InvestigationResult, HealthStatus
)


class HolmesGPTWrapper:
    """Wrapper for the actual HolmesGPT Python library."""

    def __init__(self, settings: Settings):
        self.settings = settings
        self.logger = logging.getLogger(__name__)
        self._holmes_instance = None
        self._initialized = False

    async def initialize(self) -> bool:
        """Initialize the HolmesGPT library."""
        try:
            self.logger.info("Initializing HolmesGPT Python library...")

            # Import Holmes library (correct package name)
            try:
                from holmes.core.llm import LLM, DefaultLLM
                from holmes.main import Config  # Import Config instead of Holmes
                Holmes = DefaultLLM  # Use DefaultLLM as Holmes equivalent
                LLMConfig = Config  # Use Config as LLMConfig equivalent
            except ImportError:
                # Fallback for development/testing
                class Holmes:
                    def __init__(self, *args, **kwargs):
                        pass

                class LLMConfig:
                    def __init__(self, *args, **kwargs):
                        pass

            # Configure LLM based on settings
            llm_config = self._create_llm_config()

            # Initialize Holmes instance with API-compatible approach
            try:
                # Try new API approach - initialize without llm_config parameter
                self._holmes_instance = Holmes()
                self.logger.debug("Initialized Holmes with new API approach")

                # Configure through environment or separate method if available
                self._configure_holmes_instance(llm_config)

            except TypeError as e:
                if "debug" in str(e):
                    # Try without debug parameter
                    self._holmes_instance = Holmes()
                    self.logger.debug("Initialized Holmes without debug parameter")
                else:
                    # Fallback for any other initialization issues
                    self._holmes_instance = Holmes()
                    self.logger.warning(f"Using fallback Holmes initialization: {e}")
            except Exception as e:
                # Last resort - try minimal initialization
                self._holmes_instance = Holmes()
                self.logger.warning(f"Using minimal Holmes initialization due to: {e}")

            # Test the connection
            await self._test_connection()

            self._initialized = True
            self.logger.info("HolmesGPT Python library initialized successfully")
            return True

        except ImportError as e:
            self.logger.warning(f"HolmesGPT library not available: {e}")
            return False
        except Exception as e:
            self.logger.error(f"Failed to initialize HolmesGPT library: {e}")
            return False

    def _create_llm_config(self):
        """Create LLM configuration based on settings."""
        try:
            from holmes.main import Config as LLMConfig
        except ImportError:
            # Fallback for development/testing
            class LLMConfig:
                def __init__(self, *args, **kwargs):
                    pass

        llm_config_dict = self.settings.get_llm_config()

        # Create a simple config object that just stores the model
        # The actual LLM configuration will be handled through environment variables
        # or other HolmesGPT configuration mechanisms
        try:
            # Try creating with minimal parameters first
            config = LLMConfig()

            # Store configuration data as attributes if the config object supports it
            if hasattr(config, '__dict__'):
                config.provider = self.settings.holmes_llm_provider
                config.model = llm_config_dict.get('model', self.settings.holmes_default_model)
                config.api_key = llm_config_dict.get('api_key')
                config.base_url = llm_config_dict.get('base_url')
                config.temperature = llm_config_dict.get('temperature', self.settings.holmes_default_temperature)
                config.max_tokens = llm_config_dict.get('max_tokens', self.settings.holmes_default_max_tokens)

                # Provider-specific attributes
                if self.settings.holmes_llm_provider == "openai":
                    config.organization = llm_config_dict.get('organization')
                elif self.settings.holmes_llm_provider == "azure":
                    config.endpoint = llm_config_dict.get('endpoint')
                    config.api_version = llm_config_dict.get('api_version', '2023-12-01-preview')
                elif self.settings.holmes_llm_provider == "bedrock":
                    config.aws_access_key_id = llm_config_dict.get('aws_access_key_id')
                    config.aws_secret_access_key = llm_config_dict.get('aws_secret_access_key')
                    config.region = llm_config_dict.get('region', 'us-east-1')
                elif self.settings.holmes_llm_provider in ["ollama", "ramalama"]:
                    config.timeout = llm_config_dict.get('timeout',
                                                      self.settings.ollama_timeout if self.settings.holmes_llm_provider == "ollama"
                                                      else self.settings.ramalama_timeout)

            return config

        except Exception as e:
            # If the new approach fails, try the old approach but catch validation errors
            self.logger.warning(f"Failed to create LLM config with new approach: {e}. Trying fallback.")
            return LLMConfig()

    def _configure_holmes_instance(self, llm_config):
        """Configure Holmes instance after initialization with current API."""
        try:
            # Try to set configuration attributes if the instance supports them
            if hasattr(self._holmes_instance, 'configure') and callable(getattr(self._holmes_instance, 'configure')):
                self._holmes_instance.configure(llm_config)
                self.logger.debug("Configured Holmes instance via configure() method")
            elif hasattr(self._holmes_instance, 'config'):
                self._holmes_instance.config = llm_config
                self.logger.debug("Configured Holmes instance via config attribute")
            elif hasattr(self._holmes_instance, 'llm_config'):
                self._holmes_instance.llm_config = llm_config
                self.logger.debug("Configured Holmes instance via llm_config attribute")
            else:
                # Set up environment variables as fallback
                self._setup_env_config(llm_config)
                self.logger.debug("Configured Holmes instance via environment variables")

        except Exception as e:
            self.logger.warning(f"Could not configure Holmes instance: {e}. Using defaults.")

    def _setup_env_config(self, llm_config):
        """Set up environment variables for Holmes configuration."""
        import os

        if hasattr(llm_config, '__dict__'):
            # Map config attributes to environment variables
            env_mapping = {
                'provider': 'HOLMES_LLM_PROVIDER',
                'model': 'HOLMES_MODEL',
                'api_key': 'HOLMES_API_KEY',
                'base_url': 'HOLMES_BASE_URL',
                'temperature': 'HOLMES_TEMPERATURE',
                'max_tokens': 'HOLMES_MAX_TOKENS'
            }

            for attr, env_var in env_mapping.items():
                if hasattr(llm_config, attr):
                    value = getattr(llm_config, attr)
                    if value is not None:
                        os.environ[env_var] = str(value)

    async def _test_connection(self):
        """Test the HolmesGPT connection."""
        try:
            # Simple test query
            test_result = await self.ask_simple("Test connection - respond with 'OK'")
            if not test_result:
                raise RuntimeError("HolmesGPT test query failed")
            self.logger.debug("HolmesGPT connection test successful")
        except Exception as e:
            self.logger.error(f"HolmesGPT connection test failed: {e}")
            raise

    async def ask_simple(self, prompt: str) -> Optional[str]:
        """Simple ask method for testing."""
        if not self._initialized or not self._holmes_instance:
            return None

        try:
            # Use HolmesGPT's ask method
            result = await asyncio.to_thread(
                self._holmes_instance.ask,
                prompt
            )
            return result.get('response') if isinstance(result, dict) else str(result)
        except Exception as e:
            self.logger.error(f"HolmesGPT simple ask failed: {e}")
            return None

    async def ask(
        self,
        prompt: str,
        context: Optional[ContextData] = None,
        options: Optional[HolmesOptions] = None
    ) -> AskResponse:
        """Ask HolmesGPT a question with full context."""
        if not self._initialized or not self._holmes_instance:
            raise RuntimeError("HolmesGPT not initialized")

        start_time = time.time()

        try:
            # Prepare the enhanced prompt with context
            enhanced_prompt = self._build_enhanced_prompt(prompt, context)

            # Prepare options
            ask_options = self._prepare_ask_options(options)

            # Execute the ask operation
            result = await asyncio.to_thread(
                self._holmes_instance.ask,
                enhanced_prompt,
                **ask_options
            )

            # Parse and convert the result
            return self._parse_ask_result(result, time.time() - start_time, options)

        except Exception as e:
            error_msg = f"HolmesGPT ask operation failed: {e}"
            self.logger.error(error_msg)
            raise Exception(error_msg) from e

    async def investigate(
        self,
        alert: AlertData,
        context: Optional[ContextData] = None,
        options: Optional[HolmesOptions] = None,
        investigation_context: Optional[InvestigationContext] = None
    ) -> InvestigateResponse:
        """Investigate an alert using HolmesGPT."""
        if not self._initialized or not self._holmes_instance:
            raise RuntimeError("HolmesGPT not initialized")

        start_time = time.time()

        try:
            # Build investigation query
            investigation_query = self._build_investigation_query(alert, context, investigation_context)

            # Prepare options
            investigate_options = self._prepare_investigate_options(options, investigation_context)

            # Execute the investigation
            result = await asyncio.to_thread(
                self._holmes_instance.investigate,
                investigation_query,
                **investigate_options
            )

            # Parse and convert the result
            return self._parse_investigate_result(result, alert, time.time() - start_time, options)

        except Exception as e:
            error_msg = f"HolmesGPT investigation failed: {e}"
            self.logger.error(error_msg)
            raise Exception(error_msg) from e

    def _build_enhanced_prompt(self, prompt: str, context: Optional[ContextData]) -> str:
        """Build enhanced prompt with context information."""
        enhanced_prompt = prompt

        if context:
            context_parts = []

            if context.environment:
                context_parts.append(f"Environment: {context.environment}")

            if context.kubernetes_context:
                k8s_ctx = context.kubernetes_context
                k8s_parts = []
                if k8s_ctx.namespace:
                    k8s_parts.append(f"namespace: {k8s_ctx.namespace}")
                if k8s_ctx.deployment:
                    k8s_parts.append(f"deployment: {k8s_ctx.deployment}")
                if k8s_ctx.service:
                    k8s_parts.append(f"service: {k8s_ctx.service}")
                if k8s_ctx.pod:
                    k8s_parts.append(f"pod: {k8s_ctx.pod}")
                if k8s_ctx.cluster:
                    k8s_parts.append(f"cluster: {k8s_ctx.cluster}")

                if k8s_parts:
                    context_parts.append(f"Kubernetes Context: {', '.join(k8s_parts)}")

            if context.time_range:
                context_parts.append(f"Time Range: {context.time_range}")

            if context.related_services:
                services_info = ", ".join(context.related_services)
                context_parts.append(f"Related Services: {services_info}")



            if context_parts:
                context_str = "\n".join(context_parts)
                enhanced_prompt = f"{prompt}\n\nContext Information:\n{context_str}"

        return enhanced_prompt

    def _build_investigation_query(
        self,
        alert: AlertData,
        context: Optional[ContextData],
        investigation_context: Optional[InvestigationContext]
    ) -> str:
        """Build investigation query from alert data."""
        query_parts = [
            f"Investigate the following alert:",
            f"Alert Name: {alert.name}",
            f"Severity: {alert.severity}",
            f"Status: {alert.status}",
            f"Started At: {alert.starts_at}",
        ]

        if alert.ends_at:
            query_parts.append(f"Ended At: {alert.ends_at}")

        if alert.labels:
            labels_str = ", ".join([f"{k}={v}" for k, v in alert.labels.items()])
            query_parts.append(f"Labels: {labels_str}")

        if alert.annotations:
            for key, value in alert.annotations.items():
                query_parts.append(f"{key.title()}: {value}")

        # Add investigation context
        if investigation_context:
            if investigation_context.time_range:
                query_parts.append(f"Investigation Time Range: {investigation_context.time_range}")

            if investigation_context.environment:
                query_parts.append(f"Investigation Environment: {investigation_context.environment}")

            if investigation_context.related_services:
                services = ", ".join(investigation_context.related_services)
                query_parts.append(f"Related Services to Investigate: {services}")

        # Add general context
        if context:
            if context.environment:
                query_parts.append(f"Environment: {context.environment}")

            if context.kubernetes_context:
                k8s_ctx = context.kubernetes_context
                if k8s_ctx.namespace:
                    query_parts.append(f"Namespace: {k8s_ctx.namespace}")
                if k8s_ctx.cluster:
                    query_parts.append(f"Cluster: {k8s_ctx.cluster}")
                if k8s_ctx.deployment:
                    query_parts.append(f"Deployment: {k8s_ctx.deployment}")
                if k8s_ctx.service:
                    query_parts.append(f"Service: {k8s_ctx.service}")

            if context.related_services:
                services = ", ".join(context.related_services)
                query_parts.append(f"Related Services: {services}")

        query_parts.append("\nPlease provide a detailed investigation including root cause analysis and recommended remediation steps.")

        return "\n".join(query_parts)

    def _prepare_ask_options(self, options: Optional[HolmesOptions]) -> Dict[str, Any]:
        """Prepare options for HolmesGPT ask method."""
        ask_options = {}

        if options:
            if options.max_tokens:
                ask_options['max_tokens'] = options.max_tokens
            if options.temperature is not None:
                ask_options['temperature'] = options.temperature
            if options.timeout:
                ask_options['timeout'] = options.timeout
            if options.context_window:
                ask_options['context_window'] = options.context_window
            if options.include_tools:
                ask_options['include_tools'] = options.include_tools

        return ask_options

    def _prepare_investigate_options(
        self,
        options: Optional[HolmesOptions],
        investigation_context: Optional[InvestigationContext]
    ) -> Dict[str, Any]:
        """Prepare options for HolmesGPT investigate method."""
        investigate_options = self._prepare_ask_options(options)

        if investigation_context:
            # Add investigation context options
            if investigation_context.time_range:
                investigate_options['time_range'] = investigation_context.time_range
            if investigation_context.environment:
                investigate_options['environment'] = investigation_context.environment
            if investigation_context.related_services:
                investigate_options['related_services'] = investigation_context.related_services

        return investigate_options

    def _parse_ask_result(
        self,
        result: Any,
        processing_time: float,
        options: Optional[HolmesOptions]
    ) -> AskResponse:
        """Parse HolmesGPT ask result into AskResponse."""
        if isinstance(result, dict):
            response_text = result.get('response', str(result))
            confidence = result.get('confidence', 0.8)
            recommendations_data = result.get('recommendations', [])
            sources = result.get('sources', [])
            tokens_used = result.get('tokens_used')
        else:
            response_text = str(result)
            confidence = 0.8
            recommendations_data = []
            sources = []
            tokens_used = None

        # Parse recommendations
        recommendations = []
        for rec_data in recommendations_data:
            if isinstance(rec_data, dict):
                rec = Recommendation(
                    action=rec_data.get('action', 'unknown'),
                    description=rec_data.get('description', 'No description available'),
                    command=rec_data.get('command'),
                    risk=rec_data.get('risk', 'medium'),
                    confidence=rec_data.get('confidence', 0.7),
                    parameters=rec_data.get('parameters', {}),
                    estimated_time=rec_data.get('estimated_time'),
                    prerequisites=rec_data.get('prerequisites'),
                    rollback_steps=rec_data.get('rollback_steps')
                )
                recommendations.append(rec)

        return AskResponse(
            response=response_text,
            recommendations=recommendations,
            confidence=confidence,
            model_used=self.settings.holmes_default_model,
            tokens_used=tokens_used,
            processing_time=processing_time,
            sources=sources
        )

    def _map_severity_to_assessment(self, severity: str) -> str:
        """Map alert severity to assessment level."""
        severity_map = {
            'critical': 'high',
            'warning': 'medium',
            'info': 'low'
        }
        return severity_map.get(severity.lower(), 'medium')

    def _prepare_context_prompt(self, context: Optional[InvestigationContext]) -> str:
        """Prepare context information as a prompt string."""
        if not context:
            return ""

        context_parts = []
        if context.environment:
            context_parts.append(f"Environment: {context.environment}")

        if context.kubernetes_context:
            k8s_ctx = context.kubernetes_context
            k8s_parts = []
            if k8s_ctx.namespace:
                k8s_parts.append(f"namespace: {k8s_ctx.namespace}")
            if k8s_ctx.deployment:
                k8s_parts.append(f"deployment: {k8s_ctx.deployment}")
            if k8s_ctx.service:
                k8s_parts.append(f"service: {k8s_ctx.service}")
            if k8s_ctx.cluster:
                k8s_parts.append(f"cluster: {k8s_ctx.cluster}")

            if k8s_parts:
                context_parts.append(f"Kubernetes Context: {', '.join(k8s_parts)}")

        if context.time_range:
            context_parts.append(f"Time Range: {context.time_range}")

        if context.related_services:
            context_parts.append(f"Related Services: {', '.join(context.related_services)}")

        return '\n'.join(context_parts) if context_parts else ""

    def _parse_investigate_result(
        self,
        result: Any,
        alert: AlertData,
        processing_time: float,
        options: Optional[HolmesOptions]
    ) -> InvestigateResponse:
        """Parse HolmesGPT investigate result into InvestigateResponse."""
        if isinstance(result, dict):
            analysis_data = result.get('analysis', {})
            recommendations_data = result.get('recommendations', [])
            confidence = result.get('confidence', 0.8)
            severity_assessment = result.get('severity_assessment', self._map_severity_to_assessment(alert.severity))
            evidence = result.get('evidence', {})
            metrics_data = result.get('metrics_data', {})
            logs_summary = result.get('logs_summary')
            tokens_used = result.get('tokens_used')
            data_sources = result.get('data_sources', [])
        else:
            # Fallback for simple string response
            analysis_data = {'summary': str(result)}
            recommendations_data = []
            confidence = 0.7
            severity_assessment = self._map_severity_to_assessment(alert.severity)
            evidence = {}
            metrics_data = {}
            logs_summary = None
            tokens_used = None
            data_sources = []

        # Create analysis result
        analysis = AnalysisResult(
            summary=analysis_data.get('summary', f'Investigation of alert {alert.name}'),
            root_cause=analysis_data.get('root_cause'),
            impact_assessment=analysis_data.get('impact_assessment'),
            urgency_level=analysis_data.get('urgency_level', alert.severity),
            affected_components=analysis_data.get('affected_components', []),
            related_metrics=analysis_data.get('related_metrics', {})
        )

        # Parse recommendations
        recommendations = []
        for rec_data in recommendations_data:
            if isinstance(rec_data, dict):
                rec = Recommendation(
                    action=rec_data.get('action', 'investigate_further'),
                    description=rec_data.get('description', 'Further investigation required'),
                    command=rec_data.get('command'),
                    risk=rec_data.get('risk', 'medium'),
                    confidence=rec_data.get('confidence', 0.7),
                    parameters=rec_data.get('parameters', {}),
                    estimated_time=rec_data.get('estimated_time'),
                    prerequisites=rec_data.get('prerequisites'),
                    rollback_steps=rec_data.get('rollback_steps')
                )
                recommendations.append(rec)

        # Create investigation result
        investigation = InvestigationResult(
            alert_analysis=analysis,
            evidence=evidence,
            metrics_data=metrics_data,
            logs_summary=logs_summary,
            remediation_plan=recommendations
        )

        return InvestigateResponse(
            investigation=investigation,
            recommendations=recommendations,
            confidence=confidence,
            severity_assessment=severity_assessment,
            requires_human_intervention=confidence < 0.8,
            auto_executable_actions=[rec for rec in recommendations if rec.risk == 'low' and rec.confidence > 0.9],
            model_used=self.settings.holmes_default_model,
            tokens_used=tokens_used,
            processing_time=processing_time,
            data_sources=data_sources
        )

    async def health_check(self) -> HealthStatus:
        """Perform health check on HolmesGPT wrapper."""
        if not self._initialized:
            return HealthStatus(
                component="holmesgpt_wrapper",
                status="unhealthy",
                message="HolmesGPT wrapper not initialized",
                last_check=datetime.now()
            )

        try:
            start_time = time.time()
            test_result = await self.ask_simple("Health check - respond with OK")
            response_time = time.time() - start_time

            if test_result and "ok" in test_result.lower():
                return HealthStatus(
                    component="holmesgpt_wrapper",
                    status="healthy",
                    message="HolmesGPT wrapper is operational",
                    last_check=datetime.now(),
                    response_time=response_time
                )
            else:
                return HealthStatus(
                    component="holmesgpt_wrapper",
                    status="degraded",
                    message="HolmesGPT wrapper responding but with unexpected result",
                    last_check=datetime.now(),
                    response_time=response_time
                )
        except Exception as e:
            return HealthStatus(
                component="holmesgpt_wrapper",
                status="unhealthy",
                message=f"HolmesGPT wrapper health check failed: {str(e)}",
                last_check=datetime.now()
            )

    def is_available(self) -> bool:
        """Check if HolmesGPT wrapper is available."""
        return self._initialized and self._holmes_instance is not None

    async def cleanup(self):
        """Clean up resources."""
        if self._holmes_instance:
            # Perform any necessary cleanup
            self._holmes_instance = None
        self._initialized = False
        self.logger.info("HolmesGPT wrapper cleaned up")
