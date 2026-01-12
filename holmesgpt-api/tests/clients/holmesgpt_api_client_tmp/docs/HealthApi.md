# holmesgpt_api_client.HealthApi

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_config_config_get**](HealthApi.md#get_config_config_get) | **GET** /config | Get Config
[**health_check_health_get**](HealthApi.md#health_check_health_get) | **GET** /health | Health Check
[**readiness_check_ready_get**](HealthApi.md#readiness_check_ready_get) | **GET** /ready | Readiness Check


# **get_config_config_get**
> object get_config_config_get()

Get Config

Get service configuration (sanitized)

Business Requirement: BR-HAPI-128 (Configuration endpoint)

### Example


```python
import holmesgpt_api_client
from holmesgpt_api_client.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost
# See configuration.py for a list of all supported configuration parameters.
configuration = holmesgpt_api_client.Configuration(
    host = "http://localhost"
)


# Enter a context with an instance of the API client
with holmesgpt_api_client.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = holmesgpt_api_client.HealthApi(api_client)

    try:
        # Get Config
        api_response = api_instance.get_config_config_get()
        print("The response of HealthApi->get_config_config_get:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling HealthApi->get_config_config_get: %s\n" % e)
```



### Parameters

This endpoint does not need any parameter.

### Return type

**object**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successful Response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **health_check_health_get**
> object health_check_health_get()

Health Check

Liveness probe endpoint

Business Requirement: BR-HAPI-126 (Health check endpoint)

### Example


```python
import holmesgpt_api_client
from holmesgpt_api_client.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost
# See configuration.py for a list of all supported configuration parameters.
configuration = holmesgpt_api_client.Configuration(
    host = "http://localhost"
)


# Enter a context with an instance of the API client
with holmesgpt_api_client.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = holmesgpt_api_client.HealthApi(api_client)

    try:
        # Health Check
        api_response = api_instance.health_check_health_get()
        print("The response of HealthApi->health_check_health_get:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling HealthApi->health_check_health_get: %s\n" % e)
```



### Parameters

This endpoint does not need any parameter.

### Return type

**object**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successful Response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **readiness_check_ready_get**
> object readiness_check_ready_get()

Readiness Check

Readiness probe endpoint

Business Requirements:
- BR-HAPI-127 (Readiness check endpoint)
- BR-HAPI-201 (Graceful shutdown with DD-007 pattern)

TDD GREEN Phase: Check shutdown flag first
REFACTOR phase: Real dependency health checks

### Example


```python
import holmesgpt_api_client
from holmesgpt_api_client.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost
# See configuration.py for a list of all supported configuration parameters.
configuration = holmesgpt_api_client.Configuration(
    host = "http://localhost"
)


# Enter a context with an instance of the API client
with holmesgpt_api_client.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = holmesgpt_api_client.HealthApi(api_client)

    try:
        # Readiness Check
        api_response = api_instance.readiness_check_ready_get()
        print("The response of HealthApi->readiness_check_ready_get:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling HealthApi->readiness_check_ready_get: %s\n" % e)
```



### Parameters

This endpoint does not need any parameter.

### Return type

**object**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successful Response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

