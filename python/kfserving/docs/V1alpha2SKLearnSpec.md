# V1alpha2SKLearnSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**resources** | [**V1ResourceRequirements**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ResourceRequirements.md) | Defaults to requests and limits of 1CPU, 2Gb MEM. | [optional] 
**runtime_version** | **str** | SKLearn KFServer docker image version which defaults to latest release | [optional] 
**storage_uri** | **str** | The URI of the trained model which contains model.pickle, model.pkl or model.joblib | 
**method** | **str** | SKLearn prediction method. Either "predict" or "predict_proba". Defaults to "predict" | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


