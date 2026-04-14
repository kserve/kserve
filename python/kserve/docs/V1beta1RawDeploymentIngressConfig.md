# V1beta1RawDeploymentIngressConfig

RawDeploymentIngressConfig holds Gateway API HTTPRoute generation options for RawDeployment InferenceServices. All fields are optional; zero values preserve existing behaviour exactly.
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**backend_request_timeout** | **str** | BackendRequestTimeout sets the backendRequest timeout for path-based rules (unset by default). | [optional] 
**disable_host_based_routing** | **bool** | DisableHostBasedRouting omits HTTPRoute hostnames and host catch-all rules when pathTemplate is set. | [optional] 
**gateway_listener_name** | **str** | GatewayListenerName sets sectionName on all parentRefs, pinning routes to a specific listener (e.g. \&quot;https\&quot;). | [optional] 
**path_match_type** | **str** | PathMatchType sets the path match type for path-based rules. Accepted: \&quot;PathPrefix\&quot;, \&quot;RegularExpression\&quot; (default). | [optional] 
**path_rewrite_target** | **str** | PathRewriteTarget adds a URLRewrite ReplacePrefixMatch filter when non-empty (requires PathPrefix). Typically \&quot;/\&quot;. | [optional] 
**request_timeout** | **str** | RequestTimeout overrides the per-component timeout for path-based rules (Gateway API duration string, e.g. \&quot;300s\&quot;). | [optional] 
**route_labels** | **dict(str, str)** | RouteLabels are merged onto every generated HTTPRoute, e.g. for SecurityPolicy targetSelectors. | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


