/*
Copyright 2019 The Knative Authors

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

package networking

import (
	"context"

	"github.com/knative/pkg/apis"
)

// This files contains the versionless types and enums that are strongly
// unlikely to change from version to version.

// ProtocolType is an enumeration of the supported application-layer protocols
// See also: https://github.com/knative/serving/blob/master/docs/runtime-contract.md#protocols-and-ports
type ProtocolType string

const (
	// ProtocolHTTP1 maps to HTTP/1.1.
	ProtocolHTTP1 ProtocolType = "http1"
	// ProtocolH2C maps to HTTP/2 with Prior Knowledge.
	ProtocolH2C ProtocolType = "h2c"
)

// Validate validates that ProtocolType has a correct enum value.
func (p ProtocolType) Validate(context.Context) *apis.FieldError {
	switch p {
	case ProtocolH2C, ProtocolHTTP1:
		return nil
	}
	return apis.ErrInvalidValue(p, apis.CurrentField)
}
