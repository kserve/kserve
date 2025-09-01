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
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAzureSecret(t *testing.T) {
	scenarios := map[string]struct {
		secret   *corev1.Secret
		expected []corev1.EnvVar
	}{
		"AzureSecretEnvsWithClientSecret": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "azcreds",
				},
				Data: map[string][]byte{
					AzureSubscriptionId: []byte("AzureSubscriptionId"),
					AzureTenantId:       []byte("AzureTenantId"),
					AzureClientId:       []byte("AzureClientId"),
					AzureClientSecret:   []byte("AzureClientSecret"),
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: AzureSubscriptionId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureSubscriptionId,
						},
					},
				},
				{
					Name: AzureTenantId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureTenantId,
						},
					},
				},
				{
					Name: AzureClientId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureClientId,
						},
					},
				},
				{
					Name: AzureClientSecret,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureClientSecret,
						},
					},
				},
			},
		},
		"AzureSecretEnvsWithStorageAccessKey": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "azcreds",
				},
				Data: map[string][]byte{
					AzureSubscriptionId:   []byte("AzureSubscriptionId"),
					AzureTenantId:         []byte("AzureTenantId"),
					AzureClientId:         []byte("AzureClientId"),
					AzureStorageAccessKey: []byte("AzureStorageAccessKey"),
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: AzureSubscriptionId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureSubscriptionId,
						},
					},
				},
				{
					Name: AzureTenantId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureTenantId,
						},
					},
				},
				{
					Name: AzureClientId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureClientId,
						},
					},
				},
				{
					Name: AzureStorageAccessKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureStorageAccessKey,
						},
					},
				},
			},
		},
		"AzureSecretEnvsWithClientSecretAndStorageAccessKey": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "azcreds",
				},
				Data: map[string][]byte{
					AzureSubscriptionId:   []byte("AzureSubscriptionId"),
					AzureTenantId:         []byte("AzureTenantId"),
					AzureClientId:         []byte("AzureClientId"),
					AzureClientSecret:     []byte("AzureClientSecret"),
					AzureStorageAccessKey: []byte("AzureStorageAccessKey"),
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: AzureSubscriptionId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureSubscriptionId,
						},
					},
				},
				{
					Name: AzureTenantId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureTenantId,
						},
					},
				},
				{
					Name: AzureClientId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureClientId,
						},
					},
				},
				{
					Name: AzureClientSecret,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureClientSecret,
						},
					},
				},
				{
					Name: AzureStorageAccessKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureStorageAccessKey,
						},
					},
				},
			},
		},
		"AzureSecretEnvsWithoutClientSecret": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "azcreds",
				},
				Data: map[string][]byte{
					AzureSubscriptionId:   []byte("AzureSubscriptionId"),
					AzureTenantId:         []byte("AzureTenantId"),
					AzureClientId:         []byte("AzureClientId"),
					AzureStorageAccessKey: []byte("AzureStorageAccessKey"),
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: AzureSubscriptionId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureSubscriptionId,
						},
					},
				},
				{
					Name: AzureTenantId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureTenantId,
						},
					},
				},
				{
					Name: AzureClientId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureClientId,
						},
					},
				},
				{
					Name: AzureStorageAccessKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureStorageAccessKey,
						},
					},
				},
			},
		},
		"AzureSecretEnvsWithoutStorageAccessKey": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "azcreds",
				},
				Data: map[string][]byte{
					AzureSubscriptionId: []byte("AzureSubscriptionId"),
					AzureTenantId:       []byte("AzureTenantId"),
					AzureClientId:       []byte("AzureClientId"),
					AzureClientSecret:   []byte("AzureClientSecret"),
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: AzureSubscriptionId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureSubscriptionId,
						},
					},
				},
				{
					Name: AzureTenantId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureTenantId,
						},
					},
				},
				{
					Name: AzureClientId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureClientId,
						},
					},
				},
				{
					Name: AzureClientSecret,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureClientSecret,
						},
					},
				},
			},
		},
		"AzureSecretEnvsWithoutClientSecretAndStorageAccessKey": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "azcreds",
				},
				Data: map[string][]byte{
					AzureSubscriptionId: []byte("AzureSubscriptionId"),
					AzureTenantId:       []byte("AzureTenantId"),
					AzureClientId:       []byte("AzureClientId"),
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: AzureSubscriptionId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureSubscriptionId,
						},
					},
				},
				{
					Name: AzureTenantId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureTenantId,
						},
					},
				},
				{
					Name: AzureClientId,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureClientId,
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		envs := BuildSecretEnvs(scenario.secret)

		if diff := cmp.Diff(scenario.expected, envs); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestAzureStrorageAccessSecret(t *testing.T) {
	scenarios := map[string]struct {
		secret   *corev1.Secret
		expected []corev1.EnvVar
	}{
		"AzureSecretEnvs": {
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "azcreds",
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: AzureStorageAccessKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "azcreds",
							},
							Key: AzureStorageAccessKey,
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		envs := BuildStorageAccessKeySecretEnv(scenario.secret)

		if diff := cmp.Diff(scenario.expected, envs); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
