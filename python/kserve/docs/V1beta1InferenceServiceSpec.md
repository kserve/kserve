# V1beta1InferenceServiceSpec

InferenceServiceSpec is the top level type for this resource
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**canary** | [**list[V1beta1CanarySpec]**](V1beta1CanarySpec.md) | Canary defines optional canary deployments for progressive model rollout. Each canary&#39;s predictor.name drives the Deployment name: {isvc}-{name}-predictor. To promote a canary without restart, set predictor.name to the canary name and remove the canary entry. | [optional] 
**explainer** | [**V1beta1ExplainerSpec**](V1beta1ExplainerSpec.md) |  | [optional] 
**predictor** | [**V1beta1PredictorSpec**](V1beta1PredictorSpec.md) |  | 
**suspend** | **bool** | Suspend controls whether KServe creates serving workloads for this InferenceService. When true, the controller does not create child workloads and removes any that already exist. When false or unset, the InferenceService reconciles normally.  This field is the suspension point used by external queueing systems (for example Kueue) that admit an InferenceService against a quota. Suspension is all-or-nothing: a single value covers every workload the service manages, including the transformer, explainer and any canaries.  Note: this field is not yet honored by the KServe controller; the reconciliation behavior lands in a follow-up change. | [optional] 
**tracing** | [**V1beta1TracingSpec**](V1beta1TracingSpec.md) |  | [optional] 
**transformer** | [**V1beta1TransformerSpec**](V1beta1TransformerSpec.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


