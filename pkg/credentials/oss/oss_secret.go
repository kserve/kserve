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

package oss

import (
	v1 "k8s.io/api/core/v1"
)

const (
	OSSAccessKeyId     = "OSS_ACCESS_KEY_ID"
	OSSAccessKeySecret = "OSS_ACCESS_KEY_SECRET"
	OSSEndpointURL     = "OSS_ENDPOINT"
	OSSRegion          = "OSS_REGION"
)

func BuildSecretEnvs(secret *v1.Secret) []v1.EnvVar {
	envs := []v1.EnvVar{
		{
			Name: OSSAccessKeyId,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: OSSAccessKeyId,
				},
			},
		},
		{
			Name: OSSAccessKeySecret,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: OSSAccessKeySecret,
				},
			},
		},
		{
			Name: OSSEndpointURL,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: OSSEndpointURL,
				},
			},
		},
		{
			Name: OSSRegion,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: OSSRegion,
				},
			},
		},
	}

	return envs
}
