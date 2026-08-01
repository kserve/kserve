# V1beta1CanaryStatus

CanaryStatus represents the observed state of a canary deployment.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**model_status** | [**V1beta1ModelStatus**](V1beta1ModelStatus.md) |  | [optional] 
**name** | **str** | Name of the canary variant (from predictor.name). | [default to '']
**ready** | **bool** | Ready indicates the canary deployment is available and serving traffic. | [default to False]
**traffic_percent** | **int** | TrafficPercent is the current traffic percentage routed to this canary. | [default to 0]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


