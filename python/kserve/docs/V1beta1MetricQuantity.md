# V1beta1MetricQuantity

MetricQuantity preserves both the original string and parsed Quantity
## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**format** | **str** | Change Format at will. See the comment for Canonicalize for more details. | [default to '']
**d** | [**ResourceInfDecAmount**](https://github.com/kubernetes/apimachinery/blob/master/pkg/api/resource/amount.go) |  |
**i** | [**ResourceInt64Amount**](https://github.com/kubernetes/apimachinery/blob/master/pkg/api/resource/amount.go) |  |
**s** | **str** | s is the generated value of this quantity to avoid recalculation | [default to '']

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


