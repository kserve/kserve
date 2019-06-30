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
package deployment

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/constants"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestModelInitializerInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *appsv1.Deployment
		expected *appsv1.Deployment
	}{
		"MissingAnnotations": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "user-container",
								},
							},
						},
					},
				},
			},
			expected: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "user-container",
								},
							},
						},
					},
				},
			},
		},
		"ModelInitializerInjected": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.ModelInitializerSourceUriInternalAnnotationKey: "gs://foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "user-container",
								},
							},
						},
					},
				},
			},
			expected: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.ModelInitializerSourceUriInternalAnnotationKey: "gs://foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "user-container",
									VolumeMounts: []v1.VolumeMount{
										{
											Name:      "kfserving-provision-location",
											MountPath: constants.DefaultModelLocalMountPath,
											ReadOnly:  true,
										},
									},
								},
							},
							InitContainers: []v1.Container{
								v1.Container{
									Name:  "model-initializer",
									Image: ModelInitializerContainerImage + ":" + ModelInitializerContainerVersion,
									Args:  []string{"gs://foo", constants.DefaultModelLocalMountPath},
									VolumeMounts: []v1.VolumeMount{
										{
											Name:      "kfserving-provision-location",
											MountPath: constants.DefaultModelLocalMountPath,
										},
									},
								},
							},
							Volumes: []v1.Volume{
								v1.Volume{
									Name: "kfserving-provision-location",
									VolumeSource: v1.VolumeSource{
										EmptyDir: &v1.EmptyDirVolumeSource{},
									},
								},
							},
						},
					},
				},
			},
		},
		"ModelInitializerInjectedAndMountsPvc": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.ModelInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "user-container",
								},
							},
						},
					},
				},
			},
			expected: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.ModelInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "user-container",
									VolumeMounts: []v1.VolumeMount{
										{
											Name:      "kfserving-provision-location",
											MountPath: constants.DefaultModelLocalMountPath,
											ReadOnly:  true,
										},
									},
								},
							},
							InitContainers: []v1.Container{
								v1.Container{
									Name:  "model-initializer",
									Image: ModelInitializerContainerImage + ":" + ModelInitializerContainerVersion,
									Args:  []string{"/mnt/pvc/some/path/on/pvc", constants.DefaultModelLocalMountPath},
									VolumeMounts: []v1.VolumeMount{
										{
											Name:      "kfserving-pvc-source",
											MountPath: "/mnt/pvc",
											ReadOnly:  true,
										},
										{
											Name:      "kfserving-provision-location",
											MountPath: constants.DefaultModelLocalMountPath,
										},
									},
								},
							},
							Volumes: []v1.Volume{
								v1.Volume{
									Name: "kfserving-pvc-source",
									VolumeSource: v1.VolumeSource{
										PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
											ClaimName: "mypvcname",
											ReadOnly:  false,
										},
									},
								},
								v1.Volume{
									Name: "kfserving-provision-location",
									VolumeSource: v1.VolumeSource{
										EmptyDir: &v1.EmptyDirVolumeSource{},
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
		if err := InjectModelInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff := cmp.Diff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestModelInitializerFailureCases(t *testing.T) {
	scenarios := map[string]struct {
		original            *appsv1.Deployment
		expectedErrorPrefix string
	}{
		"MissingUserContainer": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.ModelInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "random-container",
								},
							},
						},
					},
				},
			},
			expectedErrorPrefix: "Invalid configuration: cannot find container",
		},
	}

	for name, scenario := range scenarios {
		if err := InjectModelInitializer(scenario.original); err != nil {
			if !strings.HasPrefix(err.Error(), scenario.expectedErrorPrefix) {
				t.Errorf("Test %q unexpected failure [%s], expected: %s", name, err.Error(), scenario.expectedErrorPrefix)
			}
		} else {
			t.Errorf("Test %q should have failed with: %s", name, scenario.expectedErrorPrefix)
		}
	}
}
