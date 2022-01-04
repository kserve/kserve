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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAzureSecret(t *testing.T) {
	scenarios := map[string]struct {
		secret   *v1.Secret
		expected []v1.EnvVar
	}{
		"AzureSecretEnvs": {
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "azcreds",
				},
			},
			expected: []v1.EnvVar{
				{
					Name: AzureSubscriptionId,
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "azcreds",
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
								Name: "azcreds",
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
								Name: "azcreds",
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
								Name: "azcreds",
							},
							Key: AzureClientSecret,
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
