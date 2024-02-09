# V1beta1LoggerSpec

LoggerSpec specifies optional payload logging available for all components

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**mode** | **str** | Specifies the scope of the loggers. &lt;br /&gt; Valid values are: &lt;br /&gt; - \&quot;all\&quot; (default): log both request and response; &lt;br /&gt; - \&quot;request\&quot;: log only request; &lt;br /&gt; - \&quot;response\&quot;: log only response &lt;br /&gt; | [optional] 
**url** | **str** | URL to send logging events | [optional] 

## Example

```python
from kserve.models.v1beta1_logger_spec import V1beta1LoggerSpec

# TODO update the JSON string below
json = "{}"
# create an instance of V1beta1LoggerSpec from a JSON string
v1beta1_logger_spec_instance = V1beta1LoggerSpec.from_json(json)
# print the JSON string representation of the object
print V1beta1LoggerSpec.to_json()

# convert the object into a dict
v1beta1_logger_spec_dict = v1beta1_logger_spec_instance.to_dict()
# create an instance of V1beta1LoggerSpec from a dict
v1beta1_logger_spec_form_dict = v1beta1_logger_spec.from_dict(v1beta1_logger_spec_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


