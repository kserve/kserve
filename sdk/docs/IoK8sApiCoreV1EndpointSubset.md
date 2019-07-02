# IoK8sApiCoreV1EndpointSubset

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**addresses** | [**list[IoK8sApiCoreV1EndpointAddress]**](IoK8sApiCoreV1EndpointAddress.md) | IP addresses which offer the related ports that are marked as ready. These endpoints should be considered safe for load balancers and clients to utilize. | [optional] 
**not_ready_addresses** | [**list[IoK8sApiCoreV1EndpointAddress]**](IoK8sApiCoreV1EndpointAddress.md) | IP addresses which offer the related ports but are not currently marked as ready because they have not yet finished starting, have recently failed a readiness check, or have recently failed a liveness check. | [optional] 
**ports** | [**list[IoK8sApiCoreV1EndpointPort]**](IoK8sApiCoreV1EndpointPort.md) | Port numbers available on the related IP addresses. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


