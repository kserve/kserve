/*
Copyright 2021 The KServe Authors.

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

package pod

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

// These constants are used for detecting and applying GPU selectors
const (
	GkeAcceleratorNodeSelector = "cloud.google.com/gke-accelerator"
	NvidiaGPUTaintValue        = "present"
)

func InjectGKEAcceleratorSelector(pod *corev1.Pod) error {
	gpuEnabled := false
	for _, container := range pod.Spec.Containers {
		if _, ok := container.Resources.Limits[constants.NvidiaGPUResourceType]; ok {
			gpuEnabled = true
		}
	}
	// check if GPU is specified on container resource before applying the node selector
	if gpuEnabled {
		if gpuSelector, ok := pod.Annotations[constants.InferenceServiceGKEAcceleratorAnnotationKey]; ok {
			pod.Spec.NodeSelector = utils.Union(
				pod.Spec.NodeSelector,
				map[string]string{GkeAcceleratorNodeSelector: gpuSelector},
			)
		}
	}
	return nil
}
