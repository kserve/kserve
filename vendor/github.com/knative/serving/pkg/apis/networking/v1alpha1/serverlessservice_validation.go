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

package v1alpha1

import (
	"context"

	"github.com/knative/pkg/apis"
	"k8s.io/apimachinery/pkg/api/equality"
)

// Validate inspects and validates ClusterServerlessService object.
func (ci *ServerlessService) Validate(ctx context.Context) *apis.FieldError {
	return ci.Spec.Validate(ctx).ViaField("spec")
}

// Validate inspects and validates ServerlessServiceSpec object.
func (spec *ServerlessServiceSpec) Validate(ctx context.Context) *apis.FieldError {
	// Spec must not be empty.
	if equality.Semantic.DeepEqual(spec, &ServerlessServiceSpec{}) {
		return apis.ErrMissingField(apis.CurrentField)
	}
	var all *apis.FieldError
	// Spec mode must be from the enum and
	switch spec.Mode {
	case SKSOperationModeProxy, SKSOperationModeServe:
		break
	case "":
		all = all.Also(apis.ErrMissingField("mode"))
	default:
		all = all.Also(apis.ErrInvalidValue(string(spec.Mode), "mode"))
	}
	if len(spec.Selector) == 0 {
		all = all.Also(apis.ErrMissingField("selector"))
	} else {
		for k, v := range spec.Selector {
			if k == "" {
				all = all.Also(apis.ErrInvalidKeyName(k, "selector", "empty key is not permitted"))
			}
			if v == "" {
				all = all.Also(apis.ErrInvalidValue(v, apis.CurrentField).ViaKey(k).ViaField("selector"))
			}
		}
	}

	return all.Also(spec.ProtocolType.Validate().ViaField("protocolType"))
}
