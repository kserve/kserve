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

package testing

import (
	"context"
	"testing"
)

func TestInnerDefaultResource_Validate(t *testing.T) {
	r := InnerDefaultResource{}
	if err := r.Validate(context.TODO()); err != nil {
		t.Fatalf("Expected no validation errors. Actual %v", err)
	}
}

func TestInnerDefaultResource_SetDefaults(t *testing.T) {
	r := InnerDefaultResource{}
	r.SetDefaults(context.TODO())
	if r.Spec.FieldWithDefault != "I'm a default." {
		t.Fatalf("Unexpected defaulted value: %v", r.Spec.FieldWithDefault)
	}
}
