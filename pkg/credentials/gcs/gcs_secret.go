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

package gcs

import (
	v1 "k8s.io/api/core/v1"
)

const (
	GCSCredentialFileName        = "gcloud-application-credentials.json" // #nosec G101
	GCSCredentialVolumeName      = "user-gcp-sa"                         // #nosec G101
	GCSCredentialVolumeMountPath = "/var/secrets/"                       // #nosec G101
	GCSCredentialEnvKey          = "GOOGLE_APPLICATION_CREDENTIALS"      // #nosec G101
)

type GCSConfig struct {
	GCSCredentialFileName string `json:"gcsCredentialFileName,omitempty"`
}

func BuildSecretVolume(secret *v1.Secret) (v1.Volume, v1.VolumeMount) {
	volume := v1.Volume{
		Name: GCSCredentialVolumeName,
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName: secret.Name,
			},
		},
	}
	volumeMount := v1.VolumeMount{
		MountPath: GCSCredentialVolumeMountPath,
		Name:      GCSCredentialVolumeName,
		ReadOnly:  true,
	}
	return volume, volumeMount
}
