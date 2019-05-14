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
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/api/core/v1"
)

func AttachSecretEnvsAndVolume(secret *v1.Secret, configuration *v1alpha1.Configuration) {
	volume := v1.Volume{
		Name: constants.GCSCredentialVolumeName,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: secret.Name,
			},
		},
	}
	volumeMount := v1.VolumeMount{
		MountPath: constants.GCSCredentialVolumeMountPath,
		Name:      constants.GCSCredentialVolumeName,
		ReadOnly:  true,
	}
	configuration.Spec.RevisionTemplate.Spec.Volumes =
		append(configuration.Spec.RevisionTemplate.Spec.Volumes, volume)
	configuration.Spec.RevisionTemplate.Spec.Container.VolumeMounts =
		append(configuration.Spec.RevisionTemplate.Spec.Container.VolumeMounts, volumeMount)
	configuration.Spec.RevisionTemplate.Spec.Container.Env = append(configuration.Spec.RevisionTemplate.Spec.Container.Env,
		v1.EnvVar{
			Name:  constants.GCSCredentialEnvKey,
			Value: constants.GCSCredentialVolumeMountPath,
		})
}
