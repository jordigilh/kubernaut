# datastorage.ActionTypeTaxonomyAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_action_type**](ActionTypeTaxonomyAPIApi.md#create_action_type) | **POST** /api/v1/action-types | Create or re-enable an action type
[**disable_action_type**](ActionTypeTaxonomyAPIApi.md#disable_action_type) | **PATCH** /api/v1/action-types/{name}/disable | Soft-disable an action type
[**update_action_type**](ActionTypeTaxonomyAPIApi.md#update_action_type) | **PATCH** /api/v1/action-types/{name} | Update action type description


# **create_action_type**
> ActionTypeCreateResponse create_action_type(action_type_create_request)

Create or re-enable an action type

Idempotent CREATE: creates a new action type, returns existing if active, or re-enables if previously disabled.  **Business Requirement**: BR-WORKFLOW-007.1 (Idempotent CREATE) **Design Decision**: DD-ACTIONTYPE-001 (ActionType CRD Lifecycle Design) 

### Example


```python
import time
import os
import datastorage
from datastorage.models.action_type_create_request import ActionTypeCreateRequest
from datastorage.models.action_type_create_response import ActionTypeCreateResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.ActionTypeTaxonomyAPIApi(api_client)
    action_type_create_request = datastorage.ActionTypeCreateRequest() # ActionTypeCreateRequest | 

    try:
        # Create or re-enable an action type
        api_response = api_instance.create_action_type(action_type_create_request)
        print("The response of ActionTypeTaxonomyAPIApi->create_action_type:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling ActionTypeTaxonomyAPIApi->create_action_type: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **action_type_create_request** | [**ActionTypeCreateRequest**](ActionTypeCreateRequest.md)|  | 

### Return type

[**ActionTypeCreateResponse**](ActionTypeCreateResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**201** | Action type created |  -  |
**200** | Action type already exists (NOOP) or re-enabled |  -  |
**400** | Validation error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **disable_action_type**
> ActionTypeDisableResponse disable_action_type(name, action_type_disable_request)

Soft-disable an action type

Soft-disables an action type. Denied with 409 if active workflows reference it. The denial response includes the count and names of dependent workflows.  **Business Requirement**: BR-WORKFLOW-007.3 (DELETE with dependency guard) 

### Example


```python
import time
import os
import datastorage
from datastorage.models.action_type_disable_request import ActionTypeDisableRequest
from datastorage.models.action_type_disable_response import ActionTypeDisableResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.ActionTypeTaxonomyAPIApi(api_client)
    name = 'name_example' # str | PascalCase action type name (e.g., RestartPod)
    action_type_disable_request = datastorage.ActionTypeDisableRequest() # ActionTypeDisableRequest | 

    try:
        # Soft-disable an action type
        api_response = api_instance.disable_action_type(name, action_type_disable_request)
        print("The response of ActionTypeTaxonomyAPIApi->disable_action_type:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling ActionTypeTaxonomyAPIApi->disable_action_type: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **name** | **str**| PascalCase action type name (e.g., RestartPod) | 
 **action_type_disable_request** | [**ActionTypeDisableRequest**](ActionTypeDisableRequest.md)|  | 

### Return type

[**ActionTypeDisableResponse**](ActionTypeDisableResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Action type disabled successfully |  -  |
**409** | Cannot disable — active workflows depend on this action type |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **update_action_type**
> ActionTypeUpdateResponse update_action_type(name, action_type_update_request)

Update action type description

Updates the description fields of an active action type. Only spec.description is mutable; spec.name is immutable.  **Business Requirement**: BR-WORKFLOW-007.2 (Description UPDATE with audit) 

### Example


```python
import time
import os
import datastorage
from datastorage.models.action_type_update_request import ActionTypeUpdateRequest
from datastorage.models.action_type_update_response import ActionTypeUpdateResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.ActionTypeTaxonomyAPIApi(api_client)
    name = 'name_example' # str | PascalCase action type name (e.g., RestartPod)
    action_type_update_request = datastorage.ActionTypeUpdateRequest() # ActionTypeUpdateRequest | 

    try:
        # Update action type description
        api_response = api_instance.update_action_type(name, action_type_update_request)
        print("The response of ActionTypeTaxonomyAPIApi->update_action_type:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling ActionTypeTaxonomyAPIApi->update_action_type: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **name** | **str**| PascalCase action type name (e.g., RestartPod) | 
 **action_type_update_request** | [**ActionTypeUpdateRequest**](ActionTypeUpdateRequest.md)|  | 

### Return type

[**ActionTypeUpdateResponse**](ActionTypeUpdateResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Action type description updated |  -  |
**404** | Action type not found or disabled |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

