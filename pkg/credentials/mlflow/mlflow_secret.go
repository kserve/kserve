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

package mlflow

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	MLFlowTrackingUri      = "MLFLOW_TRACKING_URI"      // #nosec G101
	MLFlowTrackingUsername = "MLFLOW_TRACKING_USERNAME" // #nosec G101
	MLFlowTrackingPassword = "MLFLOW_TRACKING_PASSWORD" // #nosec G101
	MLFlowTrackingToken    = "MLFLOW_TRACKING_TOKEN"    // #nosec G101
)

var MLFlowEnvVars = []string{
	MLFlowTrackingUri,
	MLFlowTrackingUsername,
	MLFlowTrackingPassword,
	MLFlowTrackingToken,
}

func BuildSecretEnvs(secret *corev1.Secret) []corev1.EnvVar {
	envs := make([]corev1.EnvVar, 0, len(MLFlowEnvVars))
	for _, envVar := range MLFlowEnvVars {
		if _, ok := secret.Data[envVar]; ok {
			envs = append(envs, corev1.EnvVar{
				Name: envVar,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secret.Name,
						},
						Key: envVar,
					},
				},
			})
		}
	}

	return envs
}
