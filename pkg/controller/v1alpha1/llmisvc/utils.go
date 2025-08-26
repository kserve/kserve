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

package llmisvc

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

// getInferencePoolWorkloadLabelSelector creates a label selector for identifying workload pods
// that belong to a specific LLMInferenceService.
func getInferencePoolWorkloadLabelSelector(meta metav1.ObjectMeta, _ *v1alpha1.LLMInferenceServiceSpec) map[string]string {
	s := map[string]string{
		"app.kubernetes.io/part-of": "llminferenceservice",
		"app.kubernetes.io/name":    meta.GetName(),
		"kserve.io/component":       "workload",
	}
	return s
}
