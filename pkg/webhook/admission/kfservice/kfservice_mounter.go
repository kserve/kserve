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

package kfservice

import (
	"context"
	"encoding/json"
	"github.com/kubeflow/kfserving/pkg/constants"
	"net/http"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

// Mounting a PVC for containers.
type Mounter struct {
	Client  client.Client
	Decoder types.Decoder
}

const (
	PvcNameAnnotation = "pvcName"
	PvcMountPathAnnotation = "pvcMountPath"	
	pvcMountingName  = "kfserving-local-storage"
	defaultMountPath = "/mnt"
)

// Handle decodes the incoming deployment and executes mounting logic.
func (mounter *Mounter) Handle(ctx context.Context, req types.Request) types.Response {	
	deployment := &appsv1.Deployment{}

	if err := mounter.Decoder.Decode(req, deployment); err != nil {
		return admission.ErrorResponse(http.StatusBadRequest, err)
	}

	if err := pvcMounting(deployment); err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	patch, err := json.Marshal(deployment)
	if err != nil {
		return admission.ErrorResponse(http.StatusInternalServerError, err)
	}

	return patchResponseFromRaw(req.AdmissionRequest.Object.Raw, patch)
}

// Mount PVC to kfserving user container.
func pvcMounting(deployment *appsv1.Deployment) error {

	var claimName, mountPath string

	annotations := deployment.Spec.Template.ObjectMeta.Annotations
	podSpec     := &deployment.Spec.Template.Spec

	if _, ok := annotations[constants.PvcNameAnnotation]; ok {
		claimName = annotations[constants.PvcNameAnnotation]
	} else {
		return nil
	}

	if _, ok := annotations[constants.PvcMountPathAnnotation]; ok {
		mountPath = annotations[constants.PvcMountPathAnnotation]
	} else {
		mountPath = constants.PvcDefaultMountPath
	}

	volume, volumeMount := buildPersistentVolume(claimName, mountPath)

	podSpec.Volumes = append(podSpec.Volumes, volume)
	//TBD @jinchihe Any better way to get the user container?
	podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, volumeMount)

	return nil
}

// Generate Volume and VolumeMount
func buildPersistentVolume(claimName string, mountPath string) (v1.Volume, v1.VolumeMount) {
	volume := v1.Volume{
		Name: constants.PvcMountingName,
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: claimName,
			},
		},
	}

	volumeMount := v1.VolumeMount{
		Name: constants.PvcMountingName,
		MountPath: mountPath,
		ReadOnly:  true,
	}
	return volume, volumeMount
}
