# datastorage.IncidentsApi

All URIs are relative to *http://data-storage.kubernaut-system.svc.cluster.local:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**get_incident_by_id**](IncidentsApi.md#get_incident_by_id) | **GET** /api/v1/incidents/{id} | Get incident by ID
[**list_incidents**](IncidentsApi.md#list_incidents) | **GET** /api/v1/incidents | List incidents with filters


# **get_incident_by_id**
> Incident get_incident_by_id(id)

Get incident by ID

Retrieve a single incident by its unique identifier

### Example


```python
import datastorage
from datastorage.models.incident import Incident
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://data-storage.kubernaut-system.svc.cluster.local:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://data-storage.kubernaut-system.svc.cluster.local:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.IncidentsApi(api_client)
    id = 12345 # int | Unique incident ID

    try:
        # Get incident by ID
        api_response = api_instance.get_incident_by_id(id)
        print("The response of IncidentsApi->get_incident_by_id:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling IncidentsApi->get_incident_by_id: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **id** | **int**| Unique incident ID | 

### Return type

[**Incident**](Incident.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successful response with incident details |  -  |
**404** | Incident not found |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **list_incidents**
> IncidentListResponse list_incidents(alert_name=alert_name, severity=severity, action_type=action_type, namespace=namespace, limit=limit, offset=offset)

List incidents with filters

Retrieve a paginated list of incidents filtered by alert name, severity, action type, etc.

Supports:
- Filter by alert_name, severity, action_type
- Pagination with limit/offset
- Sorting by action_timestamp (DESC)


### Example


```python
import datastorage
from datastorage.models.incident_list_response import IncidentListResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://data-storage.kubernaut-system.svc.cluster.local:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://data-storage.kubernaut-system.svc.cluster.local:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.IncidentsApi(api_client)
    alert_name = 'test-integration-prod-cpu-high' # str | Filter by alert name pattern (exact match) (optional)
    severity = 'critical' # str | Filter by alert severity (optional)
    action_type = 'scale' # str | Filter by action type (e.g., scale, restart, check) (optional)
    namespace = 'production' # str | Filter by Kubernetes namespace (optional)
    limit = 100 # int | Maximum number of results to return (optional) (default to 100)
    offset = 0 # int | Number of results to skip (for pagination) (optional) (default to 0)

    try:
        # List incidents with filters
        api_response = api_instance.list_incidents(alert_name=alert_name, severity=severity, action_type=action_type, namespace=namespace, limit=limit, offset=offset)
        print("The response of IncidentsApi->list_incidents:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling IncidentsApi->list_incidents: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **alert_name** | **str**| Filter by alert name pattern (exact match) | [optional] 
 **severity** | **str**| Filter by alert severity | [optional] 
 **action_type** | **str**| Filter by action type (e.g., scale, restart, check) | [optional] 
 **namespace** | **str**| Filter by Kubernetes namespace | [optional] 
 **limit** | **int**| Maximum number of results to return | [optional] [default to 100]
 **offset** | **int**| Number of results to skip (for pagination) | [optional] [default to 0]

### Return type

[**IncidentListResponse**](IncidentListResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Successful response with incident list |  -  |
**400** | Bad request - Invalid filter parameters |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

