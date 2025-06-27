# V1beta1LoggerSpec

LoggerSpec specifies optional payload logging available for all components
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**log_schema** | **str** | Specifies the format for custom log generation.  The standard cloud events currently sent will not be compatible with a custom format Valid values are: - \&quot;JSON\&quot;: logs are generated and sent as a JSON payload following the format specified in inference-logging-configmap | [optional] 
**metadata_annotations** | **list[str]** | Matched inference service annotations for propagating to inference logger cloud events. | [optional] 
**metadata_headers** | **list[str]** | Matched metadata HTTP headers for propagating to inference logger cloud events. | [optional] 
**mode** | **str** | Specifies the scope of the loggers. &lt;br /&gt; Valid values are: &lt;br /&gt; - \&quot;all\&quot; (default): log both request and response; &lt;br /&gt; - \&quot;request\&quot;: log only request; &lt;br /&gt; - \&quot;response\&quot;: log only response &lt;br /&gt; | [optional] 
**storage** | [**V1beta1LoggerStorageSpec**](V1beta1LoggerStorageSpec.md) |  | [optional] 
**url** | **str** | URL to send logging events | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


