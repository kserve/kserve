# IoK8sApiCoreV1Lifecycle

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**post_start** | [**IoK8sApiCoreV1Handler**](IoK8sApiCoreV1Handler.md) | PostStart is called immediately after a container is created. If the handler fails, the container is terminated and restarted according to its restart policy. Other management of the container blocks until the hook completes. More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks | [optional] 
**pre_stop** | [**IoK8sApiCoreV1Handler**](IoK8sApiCoreV1Handler.md) | PreStop is called immediately before a container is terminated. The container is terminated after the handler completes. The reason for termination is passed to the handler. Regardless of the outcome of the handler, the container is eventually terminated. Other management of the container blocks until the hook completes. More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


