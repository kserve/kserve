# V1alpha1SupportedModelFormat

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**auto_select** | **bool** | Set to true to allow the ServingRuntime to be used for automatic model placement if this model format is specified with no explicit runtime. | [optional] 
**name** | **str** | Name of the model format. | [optional] [default to '']
**priority** | **int** | Priority of this serving runtime for auto selection. This is used to select the serving runtime if more than one serving runtime supports the same model format. The value should be greater than zero.  The higher the value, the higher the priority. Priority is not considered if AutoSelect is either false or not specified. Priority can be overridden by specifying the runtime in the InferenceService. | [optional] 
**version** | **str** | Version of the model format. Used in validating that a predictor is supported by a runtime. Can be \&quot;major\&quot;, \&quot;major.minor\&quot; or \&quot;major.minor.patch\&quot;. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


