"""
SDK ConfigMap loader for HolmesGPT API.

Issue #390: Split HAPI configuration into service and SDK ConfigMaps.

The SDK config contains LLM provider settings, toolsets (e.g. Prometheus),
and MCP servers. It is mounted at a well-known path by the Helm chart
or Kustomize deployment.

Design decision (G1 resolution): Uses a well-known fixed file path
instead of a ConfigMap name reference, since Python reads local files
and the mount path is Helm-controlled.
"""

import logging
from pathlib import Path
from typing import Any, Dict

import yaml

logger = logging.getLogger(__name__)

SDK_CONFIG_DEFAULT_PATH = "/etc/holmesgpt/sdk/sdk-config.yaml"

SDK_MERGE_KEYS = ("llm", "toolsets", "mcp_servers")


def merge_sdk_config(
    main_config: Dict[str, Any],
    sdk_config_path: str,
) -> Dict[str, Any]:
    """Merge SDK config into main config.

    Loads the SDK config YAML from *sdk_config_path* and deep-merges
    its keys into *main_config*.  For dict-valued keys that exist in
    both configs the SDK values take precedence (update semantics);
    for keys that only exist in one side they are preserved as-is.

    Args:
        main_config: The already-loaded HAPI service config dict.
        sdk_config_path: Absolute path to the SDK config YAML file.

    Returns:
        A new dict containing the merged configuration.

    Raises:
        FileNotFoundError: If *sdk_config_path* does not exist.
        ValueError: If the SDK config file is empty or missing
            the required ``llm`` section.
    """
    config = _deep_copy_dict(main_config)
    sdk_path = Path(sdk_config_path)

    if not sdk_path.exists():
        raise FileNotFoundError(
            f"SDK config not found: {sdk_config_path}\n"
            f"Ensure the HolmesGPT SDK ConfigMap is mounted at "
            f"{SDK_CONFIG_DEFAULT_PATH} or set SDK_CONFIG_FILE."
        )

    with open(sdk_path, "r") as f:
        sdk_config = yaml.safe_load(f)

    if not sdk_config:
        raise ValueError(
            f"SDK config file is empty: {sdk_config_path}\n"
            f"The SDK ConfigMap must contain at least an 'llm' section."
        )

    if "llm" not in sdk_config:
        raise ValueError(
            f"SDK config missing required 'llm' section: {sdk_config_path}\n"
            f"HAPI requires LLM provider configuration to function. "
            f"Add at minimum: llm.provider, llm.model, llm.endpoint"
        )

    for key in SDK_MERGE_KEYS:
        if key not in sdk_config:
            continue
        if key in config and isinstance(config[key], dict) and isinstance(sdk_config[key], dict):
            config[key].update(sdk_config[key])
        else:
            config[key] = sdk_config[key]

    logger.info({
        "event": "sdk_config_loaded",
        "path": sdk_config_path,
        "merged_keys": [k for k in SDK_MERGE_KEYS if k in sdk_config],
    })

    return config


def _deep_copy_dict(d: Dict[str, Any]) -> Dict[str, Any]:
    """Shallow-copy top-level and one level of nested dicts."""
    result = {}
    for k, v in d.items():
        if isinstance(v, dict):
            result[k] = dict(v)
        else:
            result[k] = v
    return result
