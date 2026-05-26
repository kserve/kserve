# V1beta1IngressConfig

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**additional_ingress_domains** | **list[str]** |  | [optional] 
**disable_http_route_timeout** | **bool** | Deprecated: use rawDeployment.disableHTTPRouteTimeout instead. Kept for backward compatibility; at runtime, either field set to true disables HTTPRoute timeouts. | [optional]
**disable_ingress_creation** | **bool** |  | [optional] 
**disable_istio_virtual_host** | **bool** |  | [optional] 
**domain_template** | **str** |  | [optional] 
**enable_gateway_api** | **bool** |  | [optional] 
**enable_llm_inference_service_tls** | **bool** |  | [optional] 
**ingress_class_name** | **str** |  | [optional] 
**ingress_domain** | **str** |  | [optional] 
**ingress_gateway** | **str** |  | [optional] 
**knative_local_gateway_service** | **str** |  | [optional] 
**kserve_ingress_gateway** | **str** |  | [optional] 
**local_gateway** | **str** |  | [optional] 
**local_gateway_service** | **str** |  | [optional] 
**model_based_routing_header_name** | **str** |  | [optional] 
**model_based_routing_mode** | **str** |  | [optional] 
**path_template** | **str** |  | [optional] 
**raw_deployment** | [**V1beta1RawDeploymentIngressConfig**](V1beta1RawDeploymentIngressConfig.md) |  | [optional] 
**url_scheme** | **str** |  | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)

