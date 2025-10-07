/*
Copyright 2025 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"encoding/json"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/api/resource"
)

// MetricQuantity preserves both the original string and parsed Quantity
// +kubebuilder:validation:Type=""
// +kubebuilder:validation:XIntOrString
// +kubebuilder:validation:AnyOf={{type: "integer"},{type: "string"}}
// +kubebuilder:validation:Pattern=`^(\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\+|-)?(([0-9]+(\.[0-9]*)?)|(\.[0-9]+))))?$`
// +k8s:openapi-gen=false
type MetricQuantity struct {
	Original          string `json:"-"`
	resource.Quantity `json:",inline"`
}

// UnmarshalJSON implements custom JSON unmarshaling to preserve the original string
func (mq *MetricQuantity) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// Store the original string value
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		mq.Original = s
	} else {
		var i int64
		if err := json.Unmarshal(data, &i); err != nil {
			return fmt.Errorf("value must be string or integer: %w", err)
		}
		mq.Original = strconv.FormatInt(i, 10)
	}

	// Parse as resource.Quantity with better error context
	if err := json.Unmarshal(data, &mq.Quantity); err != nil {
		return fmt.Errorf("failed to parse quantity %q: %w", mq.Original, err)
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling using the original raw value for KEDA compatibility
func (mq MetricQuantity) MarshalJSON() ([]byte, error) {
	if mq.Original != "" {
		return json.Marshal(mq.Original)
	}
	return json.Marshal(mq.Quantity)
}

// Original returns the original string representation for KEDA compatibility
func (mq *MetricQuantity) GetOriginal() string {
	return mq.Original
}

// GetQuantity returns the parsed Quantity for validation
func (mq *MetricQuantity) GetQuantity() resource.Quantity {
	if mq == nil {
		return resource.Quantity{}
	}
	return mq.Quantity
}

// DeepCopy creates a deep copy of MetricQuantity
func (mq *MetricQuantity) DeepCopy() *MetricQuantity {
	if mq == nil {
		return nil
	}
	return &MetricQuantity{
		Original: mq.Original,
		Quantity: mq.Quantity.DeepCopy(),
	}
}

// NewMetricQuantity creates a new MetricQuantity from a string
// This is useful for creating instances where the raw value equals the quantity string
func NewMetricQuantity(s string) *MetricQuantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return nil
	}
	return &MetricQuantity{
		Original: s,
		Quantity: q,
	}
}
