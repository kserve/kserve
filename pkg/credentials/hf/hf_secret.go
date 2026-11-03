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

package hf

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	HFTokenKey = "HF_TOKEN"
)

func BuildSecretEnvs(secret *corev1.Secret) []corev1.EnvVar {
	envs := make([]corev1.EnvVar, 0)

	if _, ok := secret.Data[HFTokenKey]; ok {
		envs = append(envs, []corev1.EnvVar{
			{
				Name: HFTokenKey,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret.Name,
						},
						Key: HFTokenKey,
					},
				},
			},
		}...)
	}
	return envs
}
