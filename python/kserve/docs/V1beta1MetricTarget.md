# V1beta1MetricTarget

MetricTarget defines the target value, average value, or average utilization of a specific metric
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**average_utilization** | **int** | averageUtilization is the target value of the average of the resource metric across all relevant pods, represented as a percentage of the requested value of the resource for the pods. Currently only valid for Resource metric source type | [optional] 
**average_value** | [**ResourceQuantity**](ResourceQuantity.md) |  | [optional] 
**type** | **str** | type represents whether the metric type is Utilization, Value, or AverageValue | [optional] [default to '']
**value** | [**ResourceQuantity**](ResourceQuantity.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


