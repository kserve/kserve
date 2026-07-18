# V1beta1CanarySpec

CanarySpec defines a canary deployment for progressive rollout of a new model version. The canary uses fixed replicas (no autoscaling). The predictor.name field is required and used for Deployment/Service naming.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**predictor** | [**V1beta1PredictorSpec**](V1beta1PredictorSpec.md) |  | 
**traffic_percent** | **int** | TrafficPercent is the percentage of inference traffic routed to this canary. The sum of all canary TrafficPercent values must be &lt;&#x3D; 100. Set to 0 for dark launch (deploy without routing traffic). | [default to 0]

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


