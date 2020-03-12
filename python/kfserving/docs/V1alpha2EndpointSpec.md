# V1alpha2EndpointSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**explainer** | [**V1alpha2ExplainerSpec**](V1alpha2ExplainerSpec.md) | Explainer defines the model explanation service spec, explainer service calls to predictor or transformer if it is specified. | [optional] 
**predictor** | [**V1alpha2PredictorSpec**](V1alpha2PredictorSpec.md) | Predictor defines the model serving spec | 
**transformer** | [**V1alpha2TransformerSpec**](V1alpha2TransformerSpec.md) | Transformer defines the pre/post processing before and after the predictor call, transformer service calls to predictor service. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


