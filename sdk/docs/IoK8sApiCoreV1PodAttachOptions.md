# IoK8sApiCoreV1PodAttachOptions

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**api_version** | **str** | APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#resources | [optional] 
**container** | **str** | The container in which to execute the command. Defaults to only container if there is only one container in the pod. | [optional] 
**kind** | **str** | Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/api-conventions.md#types-kinds | [optional] 
**stderr** | **bool** | Stderr if true indicates that stderr is to be redirected for the attach call. Defaults to true. | [optional] 
**stdin** | **bool** | Stdin if true, redirects the standard input stream of the pod for this call. Defaults to false. | [optional] 
**stdout** | **bool** | Stdout if true indicates that stdout is to be redirected for the attach call. Defaults to true. | [optional] 
**tty** | **bool** | TTY if true indicates that a tty will be allocated for the attach call. This is passed through the container runtime so the tty is allocated on the worker node by the container runtime. Defaults to false. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


