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

package gcs

import (
	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/api/core/v1"
)

func CreateGCSSecretVolume(secret *v1.Secret) (v1.Volume, v1.VolumeMount) {
	return v1.Volume{
			Name: constants.GCSCredentialVolumeName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: secret.Name,
				},
			},
		}, v1.VolumeMount{
			MountPath: constants.GCSCredentialVolumeMountPath,
			Name:      constants.GCSCredentialVolumeName,
			ReadOnly:  true,
		}
}
