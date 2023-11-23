# V1alpha1InferenceStep

InferenceStep defines the inference target of the current step with condition, weights and data.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**condition** | **str** | routing based on the condition | [optional] 
**data** | **str** | request data sent to the next route with input/output from the previous step $request $response.predictions | [optional] 
**dependency** | **str** | to decide whether a step is a hard or a soft dependency in the Inference Graph | [optional] 
**name** | **str** | Unique name for the step within this node | [optional] 
**node_name** | **str** | The node name for routing as next step | [optional] 
**service_name** | **str** | named reference for InferenceService | [optional] 
**service_url** | **str** | InferenceService URL, mutually exclusive with ServiceName | [optional] 
**weight** | **int** | the weight for split of the traffic, only used for Split Router when weight is specified all the routing targets should be sum to 100 | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


