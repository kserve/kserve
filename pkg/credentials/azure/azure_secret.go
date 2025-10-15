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
	corev1 "k8s.io/api/core/v1"
)

const (
	AzureStorageAccessKey = "AZURE_STORAGE_ACCESS_KEY"
	// Legacy keys for backward compatibility
	LegacyAzureSubscriptionId = "AZ_SUBSCRIPTION_ID"
	LegacyAzureTenantId       = "AZ_TENANT_ID"
	LegacyAzureClientId       = "AZ_CLIENT_ID"
	LegacyAzureClientSecret   = "AZ_CLIENT_SECRET"

	// Conforms to Azure constants
	AzureSubscriptionId = "AZURE_SUBSCRIPTION_ID"
	AzureTenantId       = "AZURE_TENANT_ID"
	AzureClientId       = "AZURE_CLIENT_ID"
	AzureClientSecret   = "AZURE_CLIENT_SECRET" // #nosec G101

	AzureAccessToken                 = "AZURE_ACCESS_TOKEN"              // #nosec G101
	AzureAccessTokenExpiresOnSeconds = "AZURE_ACCESS_EXPIRES_ON_SECONDS" // #nosec G101
	AzureAccountName                 = "AZURE_ACCOUNT_NAME"
	AzureServiceUrl                  = "AZURE_SERVICE_URL"
)

var (
	LegacyAzureEnvKeys        = []string{LegacyAzureSubscriptionId, LegacyAzureTenantId, LegacyAzureClientId, LegacyAzureClientSecret}
	AzureEnvKeys              = []string{AzureSubscriptionId, AzureTenantId, AzureClientId, AzureClientSecret, AzureStorageAccessKey, AzureAccessToken, AzureAccessTokenExpiresOnSeconds, AzureAccountName, AzureServiceUrl}
	legacyAzureEnvKeyMappings = map[string]string{
		AzureSubscriptionId: LegacyAzureSubscriptionId,
		AzureTenantId:       LegacyAzureTenantId,
		AzureClientId:       LegacyAzureClientId,
		AzureClientSecret:   LegacyAzureClientSecret,
	}
	EmptyKeysToIgnore = []string{AzureClientId, AzureClientSecret, AzureSubscriptionId, AzureStorageAccessKey, AzureAccessToken, AzureAccessTokenExpiresOnSeconds, AzureAccountName, AzureServiceUrl}
)

func ignoreEmptyKey(key string) bool {
	for _, k := range EmptyKeysToIgnore {
		if k == key {
			return true
		}
	}
	return false
}

func BuildSecretEnvs(secret *corev1.Secret) []corev1.EnvVar {
	envs := make([]corev1.EnvVar, 0, len(AzureEnvKeys))
	for _, k := range AzureEnvKeys {
		dataKey := k
		legacyDataKey := legacyAzureEnvKeyMappings[k]
		if _, ok := secret.Data[legacyDataKey]; ok {
			dataKey = legacyDataKey
		}
		if _, ok := secret.Data[dataKey]; !(!ok && ignoreEmptyKey(dataKey)) {
			envs = append(envs, corev1.EnvVar{
				Name: k,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret.Name,
						},
						Key: dataKey,
					},
				},
			})
		}
	}

	return envs
}

func BuildStorageAccessKeySecretEnv(secret *corev1.Secret) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name: AzureStorageAccessKey,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret.Name,
					},
					Key: AzureStorageAccessKey,
				},
			},
		},
	}

	return envs
}
