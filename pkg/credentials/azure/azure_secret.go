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

package azure

import (
	v1 "k8s.io/api/core/v1"
)

const (
	AzureSubscriptionId = "AZ_SUBSCRIPTION_ID"
	AzureTenantId       = "AZ_TENANT_ID"
	AzureClientId       = "AZ_CLIENT_ID"
	AzureClientSecret   = "AZ_CLIENT_SECRET"
)

func BuildSecretEnvs(secret *v1.Secret) []v1.EnvVar {
	envs := []v1.EnvVar{
		{
			Name: AzureSubscriptionId,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: AzureSubscriptionId,
				},
			},
		},
		{
			Name: AzureTenantId,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: AzureTenantId,
				},
			},
		},
		{
			Name: AzureClientId,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: AzureClientId,
				},
			},
		},
		{
			Name: AzureClientSecret,
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: AzureClientSecret,
				},
			},
		},
	}

	return envs
}
