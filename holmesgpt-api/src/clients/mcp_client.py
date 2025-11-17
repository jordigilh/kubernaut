"""
MCP Client for Workflow Catalog Integration

Business Requirement: BR-WORKFLOW-020 (MCP Workflow Catalog Tool)
Reference: DD-WORKFLOW-002 v1.0 (MCP Workflow Catalog Architecture)

Provides interface to MCP Server for workflow recommendations.
"""

import logging
import httpx
from typing import Dict, Any, List, Optional

logger = logging.getLogger(__name__)


class MCPClient:
    """
    Client for MCP Workflow Catalog service
    
    Implements DD-WORKFLOW-002 v1.0 specification for search_workflow_catalog tool.
    
    The LLM constructs a natural language query describing the problem and desired
    remediation, along with optional business context filters.
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
    
    async def search_workflows(
        self,
        query: str,
        filters: Optional[Dict[str, Any]] = None,
        top_k: int = 10
    ) -> List[Dict[str, Any]]:
        """
        Search for workflows using natural language query
        
        Implements DD-WORKFLOW-002 v1.0 search_workflow_catalog tool specification.
        
        Args:
            query: Natural language description of problem, root cause, and desired remediation
                   Example: "OOMKilled pod needs memory limit increase due to insufficient allocation"
            filters: Optional filters to narrow search results:
                - signal_types: List[str] - Filter by signal types (e.g., ['OOMKilled', 'MemoryLeak'])
                - business_category: str - Filter by business category (e.g., 'payments')
                - risk_tolerance: str - Filter by risk tolerance ('low', 'medium', 'high')
                - environment: str - Filter by environment ('production', 'staging', 'development')
                - exclude_keywords: List[str] - Keywords to exclude from results
            top_k: Maximum number of workflows to return (1-50, default: 10)
            
        Returns:
            List of workflow metadata dictionaries with fields:
            - workflow_id: str - Unique workflow identifier
            - version: str - Workflow version
            - title: str - Human-readable workflow name
            - description: str - Detailed workflow description
            - signal_types: List[str] - Signal types this workflow addresses
            - similarity_score: float - Semantic match score (0.0-1.0)
            - estimated_duration: str - Expected execution time
            - success_rate: float - Historical success rate (0.0-1.0)
            
        Reference: DD-WORKFLOW-002 v1.0, lines 60-132
        """
        try:
            # Build search request per DD-WORKFLOW-002
            search_request = {
                "query": query,
                "top_k": top_k
            }
            
            if filters:
                search_request["filters"] = filters
            
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # MCP TOOL CALL AUDIT LOGGING (Placeholder for future audit traces)
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
            # TODO: Convert these logs to structured audit traces in future iteration
            
            logger.info({
                "event": "mcp_tool_call_request",
                "tool_name": "search_workflow_catalog",
                "tool_arguments": search_request,
                "endpoint": f"{self.base_url}/mcp/tools/search_workflow_catalog",
                "audit_trace_placeholder": "TODO: Convert to structured audit trace"
            })
            
            # Call MCP server
            async with httpx.AsyncClient(timeout=self.timeout) as client:
                response = await client.post(
                    f"{self.base_url}/mcp/tools/search_workflow_catalog",
                    json=search_request
                )
                response.raise_for_status()
                
                # DD-WORKFLOW-002 returns array of workflows directly
                workflows = response.json()
                
                if not isinstance(workflows, list):
                    logger.error({
                        "event": "mcp_response_format_error",
                        "error": "Expected array of workflows, got non-array response"
                    })
                    return []
                
                logger.info({
                    "event": "mcp_tool_call_response",
                    "tool_name": "search_workflow_catalog",
                    "workflows_found": len(workflows),
                    "workflow_ids": [w.get("workflow_id") for w in workflows] if workflows else [],
                    "response_summary": {
                        "workflow_count": len(workflows),
                        "top_3_titles": [w.get("title", "unnamed") for w in workflows[:3]]
                    },
                    "audit_trace_placeholder": "TODO: Convert to structured audit trace with full response"
                })
            # ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
                
                return workflows
                
        except httpx.HTTPStatusError as e:
            logger.error({
                "event": "mcp_search_http_error",
                "status_code": e.response.status_code,
                "error": str(e),
                "response_body": e.response.text if hasattr(e.response, 'text') else None
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
