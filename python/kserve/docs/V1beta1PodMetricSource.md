# V1beta1PodMetricSource

PodMetricSource indicates how to scale on a metric describing each pod in the current scale target (for example, transactions-processed-per-second). The values will be averaged together before being compared to the target value.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**metric** | [**V1beta1PodsMetricSource**](V1beta1PodsMetricSource.md) |  | [optional] 
**target** | [**V1beta1MetricTarget**](V1beta1MetricTarget.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


