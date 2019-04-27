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

package serving

import (
	"github.com/knative/pkg/apis"
	"github.com/knative/serving/pkg/apis/autoscaling"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ValidateObjectMetadata validates that `metadata` stanza of the
// resources is correct.
func ValidateObjectMetadata(meta metav1.Object) *apis.FieldError {
	return apis.ValidateObjectMetadata(meta).Also(
		autoscaling.ValidateAnnotations(meta.GetAnnotations()).ViaField("annotations"))
}
