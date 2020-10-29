# KnativeURL

URL is an alias of url.URL. It has custom json marshal methods that enable it to be used in K8s CRDs such that the CRD resource will have the URL but operator code can can work with url.URL struct
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**force_query** | **bool** | encoded path hint (see EscapedPath method) | 
**fragment** | **str** | encoded query values, without &#39;?&#39; | 
**host** | **str** | username and password information | 
**opaque** | **str** |  | 
**path** | **str** | host or host:port | 
**raw_path** | **str** | path (relative paths may omit leading slash) | 
**raw_query** | **str** | append a query (&#39;?&#39;) even if RawQuery is empty | 
**scheme** | **str** |  | 
**user** | [**NetUrlUserinfo**](NetUrlUserinfo.md) | encoded opaque data | 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


