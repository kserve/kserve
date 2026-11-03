# V1alpha1LLMInferenceService

LLMInferenceService is the Schema for the llminferenceservices API, representing a single LLM deployment. It orchestrates the creation of underlying Kubernetes resources like Deployments and Services, and configures networking for exposing the model.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**api_version** | **str** | APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources | [optional] 
**kind** | **str** | Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds | [optional] 
**metadata** | [**V1ObjectMeta**](https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/V1ObjectMeta.md) |  | [optional] 
**spec** | [**V1alpha1LLMInferenceServiceSpec**](V1alpha1LLMInferenceServiceSpec.md) |  | [optional] 
**status** | [**V1alpha1LLMInferenceServiceStatus**](V1alpha1LLMInferenceServiceStatus.md) |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


