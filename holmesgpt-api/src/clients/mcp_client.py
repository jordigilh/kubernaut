"""
MCP Client for Playbook Catalog Integration

Business Requirement: BR-PLAYBOOK-001 (MCP Playbook Integration)

Provides interface to Mock MCP Server for playbook recommendations.
"""

import logging
import httpx
from typing import Dict, Any, List, Optional

logger = logging.getLogger(__name__)


class MCPClient:
    """
    Client for MCP Playbook Catalog service
    
    Searches playbooks based on DD-PLAYBOOK-001 mandatory labels:
    - signal_type: K8s event reason (OOMKilled, CrashLoopBackOff, etc.)
    - severity: critical, high, medium, low
    - component: pod, deployment, node, etc.
    - environment: production, staging, development, test
    - priority: P0, P1, P2, P3
    - risk_tolerance: low, medium, high
    - business_category: payment-service, analytics, etc.
    """
    
    def __init__(self, config: Dict[str, Any]):
        """
        Initialize MCP client
        
        Args:
            config: MCP configuration containing:
                - base_url: MCP server base URL (e.g., "http://mock-mcp-server:8081")
                - timeout: Request timeout in seconds (default: 30)
        """
        self.base_url = config.get("base_url", "http://mock-mcp-server:8081")
        self.timeout = config.get("timeout", 30)
        
        logger.info({
            "event": "mcp_client_initialized",
            "base_url": self.base_url,
            "timeout": self.timeout
        })
    
    async def search_playbooks(
        self,
        signal_type: str,
        severity: str,
        component: str,
        environment: str,
        priority: str,
        risk_tolerance: str,
        business_category: str,
        limit: int = 5
    ) -> List[Dict[str, Any]]:
        """
        Search for playbooks matching the given criteria
        
        Args:
            signal_type: K8s event reason (OOMKilled, CrashLoopBackOff, etc.)
            severity: critical, high, medium, low
            component: pod, deployment, node, etc.
            environment: production, staging, development, test
            priority: P0, P1, P2, P3
            risk_tolerance: low, medium, high
            business_category: payment-service, analytics, etc.
            limit: Maximum number of playbooks to return
            
        Returns:
            List of matching playbooks from MCP catalog
        """
        try:
            # Build search request per DD-PLAYBOOK-001
            search_request = {
                "signal_type": signal_type,
                "severity": severity,
                "component": component,
                "environment": environment,
                "priority": priority,
                "risk_tolerance": risk_tolerance,
                "business_category": business_category,
                "limit": limit
            }
            
            logger.debug({
                "event": "mcp_search_request",
                "request": search_request
            })
            
            # Call MCP server
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                response = await client.post(
                    f"{self.base_url}/mcp/tools/search_playbook_catalog",
                    json=search_request
                )
                response.raise_for_status()
                
                result = response.json()
                playbooks = result.get("playbooks", [])
                
                logger.info({
                    "event": "mcp_search_response",
                    "playbooks_found": len(playbooks),
                    "total_results": result.get("total_results", 0)
                })
                
                return playbooks
                
        except httpx.HTTPStatusError as e:
            logger.error({
                "event": "mcp_search_http_error",
                "status_code": e.response.status_code,
                "error": str(e)
            })
            # Graceful degradation - return empty list
            return []
            
        except httpx.RequestError as e:
            logger.error({
                "event": "mcp_search_request_error",
                "error": str(e),
                "base_url": self.base_url
            })
            # Graceful degradation - return empty list
            return []
            
        except Exception as e:
            logger.error({
                "event": "mcp_search_unexpected_error",
                "error": str(e)
            })
            # Graceful degradation - return empty list
            return []

