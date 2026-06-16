# V1beta1InferenceServiceSpec

InferenceServiceSpec is the top level type for this resource
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**canary** | [**list[V1beta1CanarySpec]**](V1beta1CanarySpec.md) | Canary defines optional canary deployments for progressive model rollout. Each canary&#39;s predictor.name drives the Deployment name: {isvc}-{name}-predictor. To promote a canary without restart, set predictor.name to the canary name and remove the canary entry. | [optional] 
**explainer** | [**V1beta1ExplainerSpec**](V1beta1ExplainerSpec.md) |  | [optional] 
**predictor** | [**V1beta1PredictorSpec**](V1beta1PredictorSpec.md) |  | 
**transformer** | [**V1beta1TransformerSpec**](V1beta1TransformerSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


