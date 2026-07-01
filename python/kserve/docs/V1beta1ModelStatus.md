# V1beta1ModelStatus

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**copies** | [**V1beta1ModelCopies**](V1beta1ModelCopies.md) |  | [optional] 
**last_failure_info** | [**V1beta1FailureInfo**](V1beta1FailureInfo.md) |  | [optional] 
**model_name** | **str** | The effective model name used in inference endpoint paths, which may differ from the InferenceService name. Resolved from the model server container args, environment variables, or falls back to the InferenceService name. | [optional] 
**states** | [**V1beta1ModelRevisionStates**](V1beta1ModelRevisionStates.md) |  | [optional] 
**supported_protocols** | **list[str]** | Inference protocols supported by the serving runtime backing this model (e.g. [\&quot;v1\&quot;, \&quot;v2\&quot;]). Lets consumers construct inference endpoint paths without separately looking up the ServingRuntime. | [optional] 
**transition_status** | **str** | Whether the available predictor endpoints reflect the current Spec or is in transition | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


