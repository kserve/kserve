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
	"context"
	"encoding/json"
	"net/http"

	"github.com/kubeflow/kfserving/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"

	"github.com/kubeflow/kfserving/pkg/webhook/third_party"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// Mutator is a webhook that injects incoming pods
type Mutator struct {
	Client  client.Client
	Decoder types.Decoder
}

// These constants are used for detecting and applying GPU selectors
const (
	KFServingGkeAcceleratorAnnotation = "kfserving.kubeflow.org/gke-accelerator"
	GkeAcceleratorNodeSelector        = "cloud.google.com/gke-accelerator"
	NvidiaGPUResourceType             = "nvidia.com/gpu"
	NvidiaGPUTaintValue               = "present"
)

// Handle decodes the incoming Pod and executes mutation logic.
func (mutator *Mutator) Handle(ctx context.Context, req types.Request) types.Response {
	deployment := &appsv1.Deployment{}

	if err := mutator.Decoder.Decode(req, deployment); err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	if err := InjectGPUToleration(deployment); err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	if err := InjectGKEAcceleratorSelector(deployment); err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	patch, err := json.Marshal(deployment)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	return third_party.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, patch)
}

func InjectGKEAcceleratorSelector(deployment *appsv1.Deployment) error {
	if gpuSelector, ok := deployment.Annotations[KFServingGkeAcceleratorAnnotation]; ok {
		deployment.Spec.Template.Spec.NodeSelector = utils.Union(
			deployment.Spec.Template.Spec.NodeSelector,
			map[string]string{GkeAcceleratorNodeSelector: gpuSelector},
		)
	}
	return nil
}

func InjectGPUToleration(deployment *appsv1.Deployment) error {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if _, ok := container.Resources.Limits[NvidiaGPUResourceType]; ok {
			deployment.Spec.Template.Spec.Tolerations = append(
				deployment.Spec.Template.Spec.Tolerations,
				v1.Toleration{
					Key:      NvidiaGPUResourceType,
					Value:    NvidiaGPUTaintValue,
					Operator: v1.TolerationOpEqual,
					Effect:   v1.TaintEffectPreferNoSchedule,
				})
		}
	}
	return nil
}
