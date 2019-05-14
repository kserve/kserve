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
	"github.com/google/go-cmp/cmp"
	"github.com/knative/serving/pkg/apis/serving/v1alpha1"
	"github.com/knative/serving/pkg/apis/serving/v1beta1"
	"github.com/kubeflow/kfserving/pkg/constants"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestGcsSecret(t *testing.T) {
	scenarios := map[string]struct {
		secret   *v1.Secret
		origin   *v1alpha1.Configuration
		expected *v1alpha1.Configuration
	}{
		"GCSSecretVolume": {
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "user-gcp-sa",
					Annotations: map[string]string{
						constants.KFServiceGCSSecretAnnotation: "",
					},
				},
			},
			origin: &v1alpha1.Configuration{
				Spec: v1alpha1.ConfigurationSpec{
					RevisionTemplate: &v1alpha1.RevisionTemplateSpec{
						Spec: v1alpha1.RevisionSpec{
							Container: &v1.Container{},
						},
					},
				},
			},
			expected: &v1alpha1.Configuration{
				Spec: v1alpha1.ConfigurationSpec{
					RevisionTemplate: &v1alpha1.RevisionTemplateSpec{
						Spec: v1alpha1.RevisionSpec{
							Container: &v1.Container{
								VolumeMounts: []v1.VolumeMount{
									{
										Name:      constants.GCSCredentialVolumeName,
										ReadOnly:  true,
										MountPath: constants.GCSCredentialVolumeMountPath,
									},
								},
								Env: []v1.EnvVar{
									{
										Name:  constants.GCSCredentialEnvKey,
										Value: constants.GCSCredentialVolumeMountPath,
									},
								},
							},
							RevisionSpec: v1beta1.RevisionSpec{
								PodSpec: v1beta1.PodSpec{
									Volumes: []v1.Volume{
										{
											Name: constants.GCSCredentialVolumeName,
											VolumeSource: v1.VolumeSource{
												Secret: &v1.SecretVolumeSource{
													SecretName: "user-gcp-sa",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		AttachSecretEnvsAndVolume(scenario.secret, scenario.origin)

		if diff := cmp.Diff(scenario.expected, scenario.origin); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
