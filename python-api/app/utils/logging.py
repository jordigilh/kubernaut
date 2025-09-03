"""
Logging utilities and configuration.
"""

import logging
import logging.handlers
import sys
import json
import traceback
from typing import Dict, Any, Optional
from datetime import datetime, timezone
from pathlib import Path


class JSONFormatter(logging.Formatter):
    """JSON formatter for structured logging."""

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

    def format(self, record: logging.LogRecord) -> str:
        """Format log record as JSON."""
        # Base log data
        log_data = {
            "timestamp": datetime.fromtimestamp(record.created, tz=timezone.utc).isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
            "module": record.module,
            "function": record.funcName,
            "line": record.lineno,
            "thread": record.thread,
            "thread_name": record.threadName,
            "process": record.process,
        }

        # Add extra fields from record
        if hasattr(record, '__dict__'):
            for key, value in record.__dict__.items():
                if key not in ['name', 'msg', 'args', 'levelname', 'levelno',
                              'pathname', 'filename', 'module', 'lineno', 'funcName',
                              'created', 'msecs', 'relativeCreated', 'thread',
                              'threadName', 'processName', 'process', 'getMessage',
                              'exc_info', 'exc_text', 'stack_info']:
                    log_data[key] = value

        # Add exception information
        if record.exc_info:
            log_data["exception"] = {
                "type": record.exc_info[0].__name__ if record.exc_info[0] else None,
                "message": str(record.exc_info[1]) if record.exc_info[1] else None,
                "traceback": traceback.format_exception(*record.exc_info)
            }

        # Add stack info
        if record.stack_info:
            log_data["stack_info"] = record.stack_info

        return json.dumps(log_data, default=str, ensure_ascii=False)


class ColoredFormatter(logging.Formatter):
    """Colored formatter for console output."""

    COLORS = {
        'DEBUG': '\033[94m',      # Blue
        'INFO': '\033[92m',       # Green
        'WARNING': '\033[93m',    # Yellow
        'ERROR': '\033[91m',      # Red
        'CRITICAL': '\033[95m',   # Magenta
    }
    RESET = '\033[0m'

    def format(self, record: logging.LogRecord) -> str:
        """Format log record with colors."""
        # Add color
        level_color = self.COLORS.get(record.levelname, '')
        record.levelname = f"{level_color}{record.levelname}{self.RESET}"

        # Format timestamp
        timestamp = datetime.fromtimestamp(record.created).strftime('%Y-%m-%d %H:%M:%S')

        # Format message
        message = super().format(record)

        return f"{timestamp} | {message}"


def setup_logging(level: str = "INFO", format_type: str = "json", log_file: Optional[str] = None) -> None:
    """
    Set up application logging.

    Args:
        level: Log level (DEBUG, INFO, WARNING, ERROR, CRITICAL)
        format_type: Log format ('json' or 'text')
        log_file: Optional log file path
    """
    # Convert string level to logging constant
    log_level = getattr(logging, level.upper(), logging.INFO)

    # Create root logger
    root_logger = logging.getLogger()
    root_logger.setLevel(log_level)

    # Clear existing handlers
    root_logger.handlers.clear()

    # Console handler
    console_handler = logging.StreamHandler(sys.stdout)
    console_handler.setLevel(log_level)

    if format_type.lower() == "json":
        console_formatter = JSONFormatter()
    else:
        console_formatter = ColoredFormatter(
            fmt='%(levelname)s | %(name)s | %(message)s'
        )

    console_handler.setFormatter(console_formatter)
    root_logger.addHandler(console_handler)

    # File handler (if specified)
    if log_file:
        log_path = Path(log_file)
        log_path.parent.mkdir(parents=True, exist_ok=True)

        file_handler = logging.handlers.RotatingFileHandler(
            filename=log_file,
            maxBytes=10 * 1024 * 1024,  # 10 MB
            backupCount=5,
            encoding='utf-8'
        )
        file_handler.setLevel(log_level)

        # Always use JSON format for file logs
        file_formatter = JSONFormatter()
        file_handler.setFormatter(file_formatter)
        root_logger.addHandler(file_handler)

    # Set up library loggers
    _configure_library_loggers(log_level)

    # Log startup message
    logger = logging.getLogger(__name__)
    logger.info(f"Logging configured: level={level}, format={format_type}, file={log_file}")


def _configure_library_loggers(level: int) -> None:
    """Configure logging levels for third-party libraries."""
    library_configs = {
        'aiohttp': logging.WARNING,
        'aiohttp.access': logging.WARNING,
        'asyncio': logging.WARNING,
        'urllib3': logging.WARNING,
        'requests': logging.WARNING,
        'kubernetes': logging.WARNING,
        'prometheus_client': logging.WARNING,
    }

    for logger_name, logger_level in library_configs.items():
        logging.getLogger(logger_name).setLevel(max(level, logger_level))


class StructuredLogger:
    """Wrapper for structured logging with consistent fields."""

    def __init__(self, name: str, **default_fields):
        """
        Initialize structured logger.

        Args:
            name: Logger name
            **default_fields: Default fields to include in all log messages
        """
        self._logger = logging.getLogger(name)
        self._default_fields = default_fields

    def _log(self, level: int, message: str, exc_info=None, **fields) -> None:
        """Log with structured fields."""
        # Extract exc_info from fields if present
        if 'exc_info' in fields:
            exc_info = fields.pop('exc_info')

        # Merge default fields with provided fields
        log_fields = {**self._default_fields, **fields}

        # Create extra dict for logging
        extra = {}
        for key, value in log_fields.items():
            # Ensure field names are valid Python identifiers
            clean_key = key.replace('-', '_').replace('.', '_')
            extra[clean_key] = value

        self._logger.log(level, message, extra=extra, exc_info=exc_info)

    def debug(self, message: str, **fields) -> None:
        """Log debug message."""
        self._log(logging.DEBUG, message, **fields)

    def info(self, message: str, **fields) -> None:
        """Log info message."""
        self._log(logging.INFO, message, **fields)

    def warning(self, message: str, **fields) -> None:
        """Log warning message."""
        self._log(logging.WARNING, message, **fields)

    def error(self, message: str, **fields) -> None:
        """Log error message."""
        self._log(logging.ERROR, message, **fields)

    def critical(self, message: str, **fields) -> None:
        """Log critical message."""
        self._log(logging.CRITICAL, message, **fields)

    def exception(self, message: str, **fields) -> None:
        """Log exception with traceback."""
        self._log(logging.ERROR, message, exc_info=True, **fields)


class RequestLogger:
    """Logger for HTTP requests with consistent formatting."""

    def __init__(self, logger_name: str = "request"):
        self.logger = StructuredLogger(logger_name)

    def log_request_start(self, method: str, path: str, **fields) -> None:
        """Log request start."""
        self.logger.info(
            f"Request started: {method} {path}",
            request_method=method,
            request_path=path,
            event_type="request_start",
            **fields
        )

    def log_request_end(
        self,
        method: str,
        path: str,
        status_code: int,
        duration: float,
        **fields
    ) -> None:
        """Log request completion."""
        self.logger.info(
            f"Request completed: {method} {path} {status_code} ({duration:.3f}s)",
            request_method=method,
            request_path=path,
            response_status=status_code,
            request_duration=duration,
            event_type="request_end",
            **fields
        )

    def log_request_error(
        self,
        method: str,
        path: str,
        error: Exception,
        duration: float,
        **fields
    ) -> None:
        """Log request error."""
        self.logger.error(
            f"Request failed: {method} {path} - {str(error)} ({duration:.3f}s)",
            request_method=method,
            request_path=path,
            request_duration=duration,
            error_type=type(error).__name__,
            error_message=str(error),
            event_type="request_error",
            **fields
        )


class HolmesLogger:
    """Logger for HolmesGPT operations with consistent formatting."""

    def __init__(self, logger_name: str = "holmes"):
        self.logger = StructuredLogger(logger_name)

    def log_operation_start(self, operation: str, **fields) -> None:
        """Log HolmesGPT operation start."""
        self.logger.info(
            f"HolmesGPT operation started: {operation}",
            operation=operation,
            event_type="holmes_operation_start",
            **fields
        )

    def log_operation_end(
        self,
        operation: str,
        duration: float,
        success: bool,
        confidence: Optional[float] = None,
        **fields
    ) -> None:
        """Log HolmesGPT operation completion."""
        status = "success" if success else "error"
        message = f"HolmesGPT operation {status}: {operation} ({duration:.3f}s)"

        log_fields = {
            "operation": operation,
            "operation_duration": duration,
            "operation_success": success,
            "event_type": "holmes_operation_end",
            **fields
        }

        if confidence is not None:
            log_fields["confidence"] = confidence

        if success:
            self.logger.info(message, **log_fields)
        else:
            self.logger.error(message, **log_fields)

    def log_direct_import_status(self, available: bool, **fields) -> None:
        """Log direct import status."""
        status = "available" if available else "unavailable"
        self.logger.info(
            f"HolmesGPT direct import: {status}",
            direct_import_available=available,
            event_type="holmes_direct_import_check",
            **fields
        )

    def log_cli_status(self, available: bool, cli_path: str, **fields) -> None:
        """Log CLI status."""
        status = "available" if available else "unavailable"
        self.logger.info(
            f"HolmesGPT CLI: {status} ({cli_path})",
            cli_available=available,
            cli_path=cli_path,
            event_type="holmes_cli_check",
            **fields
        )


def get_logger(name: str, **default_fields) -> StructuredLogger:
    """Get a structured logger instance."""
    return StructuredLogger(name, **default_fields)


def get_request_logger() -> RequestLogger:
    """Get a request logger instance."""
    return RequestLogger()


def get_holmes_logger() -> HolmesLogger:
    """Get a HolmesGPT logger instance."""
    return HolmesLogger()


# Context manager for request logging
class RequestLoggingContext:
    """Context manager for automatic request logging."""

    def __init__(self, method: str, path: str, **fields):
        self.method = method
        self.path = path
        self.fields = fields
        self.logger = get_request_logger()
        self._start_time = None

    def __enter__(self):
        """Enter context and log request start."""
        import time
        self._start_time = time.time()
        self.logger.log_request_start(self.method, self.path, **self.fields)
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Exit context and log request end or error."""
        import time
        duration = time.time() - self._start_time

        if exc_type is None:
            # Success - status code should be set by caller
            status_code = self.fields.get('status_code', 200)
            # Remove status_code from fields to avoid duplicate parameter
            fields_without_status = {k: v for k, v in self.fields.items() if k != 'status_code'}
            self.logger.log_request_end(
                self.method, self.path, status_code, duration, **fields_without_status
            )
        else:
            # Error
            self.logger.log_request_error(
                self.method, self.path, exc_val, duration, **self.fields
            )

        # Don't suppress exceptions
        return False


# Utility functions for common logging patterns
def log_startup(service_name: str, version: str, **config_info) -> None:
    """Log service startup information."""
    logger = get_logger("startup")
    logger.info(
        f"{service_name} v{version} starting up",
        service_name=service_name,
        service_version=version,
        event_type="service_startup",
        **config_info
    )


def log_shutdown(service_name: str) -> None:
    """Log service shutdown."""
    logger = get_logger("shutdown")
    logger.info(
        f"{service_name} shutting down",
        service_name=service_name,
        event_type="service_shutdown"
    )


def log_health_check(component: str, healthy: bool, response_time: float, **details) -> None:
    """Log health check results."""
    logger = get_logger("health")
    status = "healthy" if healthy else "unhealthy"
    logger.info(
        f"Health check {status}: {component} ({response_time:.3f}s)",
        component=component,
        health_status=healthy,
        response_time=response_time,
        event_type="health_check",
        **details
    )
