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
DataStorage Connection Pool Manager Singleton

Performance Fix: Reuse ServiceAccountAuthPoolManager across all HAPI components.

Problem:
--------
Multiple HAPI components (workflow_catalog, llm_integration, buffered_store) were
each creating NEW ServiceAccountAuthPoolManager instances. Each new instance creates
a NEW urllib3.PoolManager with 10 connection pools, requiring NEW TCP connections
to DataStorage (3-way handshake + auth = 2-3 seconds per connection).

Evidence (AIAnalysis integration tests):
- Request 1-2: 28-57ms (reusing connections) ✅
- Request 3+: 500-2900ms (new connection establishment) 🔴
- Progressive degradation over test run

Solution:
---------
Module-level singleton ensures ALL HAPI components share ONE pool manager, which
maintains persistent HTTP connections to DataStorage. Connection reuse reduces
latency from 2-3 seconds back to baseline 28-57ms.

Usage:
------
    from datastorage_pool_manager import get_shared_datastorage_pool_manager
    
    auth_pool = get_shared_datastorage_pool_manager()
    config = Configuration(host=data_storage_url)
    api_client = ApiClient(configuration=config)
    api_client.rest_client.pool_manager = auth_pool

Performance Impact:
-------------------
- WITHOUT singleton: Each component = NEW connections = 2-3 second delays
- WITH singleton: All components = REUSED connections = 28-57ms baseline

Thread Safety:
--------------
Uses threading.Lock for safe concurrent initialization.

Authority: Performance optimization discovered during AIAnalysis integration test triage
Related: DD-AUTH-014 (ServiceAccount authentication pattern)
"""

import logging
import threading
from typing import Optional

from datastorage_auth_session import ServiceAccountAuthPoolManager

logger = logging.getLogger(__name__)

# Module-level singleton
_shared_datastorage_pool_manager: Optional[ServiceAccountAuthPoolManager] = None
_datastorage_pool_manager_lock = threading.Lock()


def get_shared_datastorage_pool_manager() -> ServiceAccountAuthPoolManager:
    """
    Get singleton ServiceAccountAuthPoolManager for DataStorage connections.
    
    This singleton pattern ensures all HAPI components (workflow_catalog, llm_integration,
    buffered_store) reuse the same connection pool manager, avoiding repeated TCP
    connection establishment overhead.
    
    Performance Impact:
    - WITHOUT singleton: Each component instance = NEW connections = 2-3 second delays
    - WITH singleton: All component instances = REUSED connections = 28-57ms baseline
    
    Thread Safety:
    Uses threading.Lock for safe concurrent access during initialization.
    
    Returns:
        ServiceAccountAuthPoolManager or IntegrationTestPoolManager: Singleton pool manager
    
    Example:
        >>> from datastorage_pool_manager import get_shared_datastorage_pool_manager
        >>> from datastorage import ApiClient, Configuration
        >>> 
        >>> # All components share the same pool manager
        >>> auth_pool = get_shared_datastorage_pool_manager()
        >>> config = Configuration(host="http://data-storage:8080")
        >>> api_client = ApiClient(configuration=config)
        >>> api_client.rest_client.pool_manager = auth_pool
    """
    global _shared_datastorage_pool_manager
    
    with _datastorage_pool_manager_lock:
        if _shared_datastorage_pool_manager is None:
            logger.info("🔧 Creating singleton ServiceAccountAuthPoolManager for DataStorage (shared across all HAPI components)")
            # BR-HAPI-301: Increase pool size for parallel test execution (4 pytest workers)
            # Default num_pools=10, but also set maxsize for connections per pool
            # DD-AUTH-014: Token path defaults to /var/run/secrets/kubernetes.io/serviceaccount/token
            _shared_datastorage_pool_manager = ServiceAccountAuthPoolManager(
                num_pools=20,   # Number of connection pools
                maxsize=20,     # Max connections per pool (handles parallel tests)
                block=False     # Don't block when pool exhausted, raise error instead
            )
            logger.info(f"   Pool configuration: num_pools=20, maxsize=20, block=False")
        return _shared_datastorage_pool_manager
