# V1beta1LoggerSpec

LoggerSpec specifies optional payload logging available for all components
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**metadata_annotations** | **list[str]** | Matched inference service annotations for propagating to inference logger cloud events. | [optional] 
**metadata_headers** | **list[str]** | Matched metadata HTTP headers for propagating to inference logger cloud events. | [optional] 
**mode** | **str** | Specifies the scope of the loggers. &lt;br /&gt; Valid values are: &lt;br /&gt; - \&quot;all\&quot; (default): log both request and response; &lt;br /&gt; - \&quot;request\&quot;: log only request; &lt;br /&gt; - \&quot;response\&quot;: log only response &lt;br /&gt; | [optional] 
**url** | **str** | URL to send logging events | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


