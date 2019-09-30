/*
Copyright 2019 kubeflow.org.

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
package deployment

import (
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
)

// These constants are used for detecting and applying GPU selectors
const (
	GkeAcceleratorNodeSelector = "cloud.google.com/gke-accelerator"
	NvidiaGPUTaintValue        = "present"
)

func InjectGKEAcceleratorSelector(deployment *appsv1.Deployment) error {
	if gpuSelector, ok := deployment.Annotations[constants.KFServingGKEAcceleratorAnnotationKey]; ok {
		deployment.Spec.Template.Spec.NodeSelector = utils.Union(
			deployment.Spec.Template.Spec.NodeSelector,
			map[string]string{GkeAcceleratorNodeSelector: gpuSelector},
		)
	}
	return nil
}
