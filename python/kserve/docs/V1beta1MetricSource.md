# V1beta1MetricSource

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**backend** | **str** | MetricsBackend defines the scaling metric type watched by autoscaler possible values are prometheus, graphite, opentelemetry. | [optional] 
**namespace** | **str** | For namespaced query | [optional] 
**operation_over_time** | **str** | OperationOverTime specifies the operation to aggregate the metrics over time | [optional] 
**query** | **str** | Query to run to get metrics from MetricsBackend | [optional] 
**server_address** | **str** | Address of MetricsBackend server. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


