# IoK8sApiCoreV1NodeSpec

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**config_source** | [**IoK8sApiCoreV1NodeConfigSource**](IoK8sApiCoreV1NodeConfigSource.md) | If specified, the source to get node configuration from The DynamicKubeletConfig feature gate must be enabled for the Kubelet to use this field | [optional] 
**external_id** | **str** | Deprecated. Not all kubelets will set this field. Remove field after 1.13. see: https://issues.k8s.io/61966 | [optional] 
**pod_cidr** | **str** | PodCIDR represents the pod IP range assigned to the node. | [optional] 
**provider_id** | **str** | ID of the node assigned by the cloud provider in the format: &lt;ProviderName&gt;://&lt;ProviderSpecificNodeID&gt; | [optional] 
**taints** | [**list[IoK8sApiCoreV1Taint]**](IoK8sApiCoreV1Taint.md) | If specified, the node&#39;s taints. | [optional] 
**unschedulable** | **bool** | Unschedulable controls node schedulability of new pods. By default, node is schedulable. More info: https://kubernetes.io/docs/concepts/nodes/node/#manual-node-administration | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


