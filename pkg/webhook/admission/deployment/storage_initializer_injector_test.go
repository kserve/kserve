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
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/kubeflow/kfserving/pkg/constants"
	"github.com/kubeflow/kfserving/pkg/controller/inferenceservice/resources/credentials"
	"github.com/kubeflow/kfserving/pkg/controller/inferenceservice/resources/credentials/gcs"
	"github.com/kubeflow/kfserving/pkg/controller/inferenceservice/resources/credentials/s3"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStorageInitializerInjector(t *testing.T) {
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
								{
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
								{
									Name: "user-container",
								},
							},
						},
					},
				},
			},
		},
		"AlreadyInjected": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name: "user-container",
								},
							},
							InitContainers: []v1.Container{
								{
									Name: "storage-initializer",
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name: "user-container",
								},
							},
							InitContainers: []v1.Container{
								{
									Name: "storage-initializer",
								},
							},
						},
					},
				},
			},
		},
		"StorageInitializerInjected": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
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
								{
									Name:  "storage-initializer",
									Image: StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
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
								{
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
		"StorageInitializerInjectedAndMountsPvc": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
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
								{
									Name:  "storage-initializer",
									Image: StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
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
								{
									Name: "kfserving-pvc-source",
									VolumeSource: v1.VolumeSource{
										PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
											ClaimName: "mypvcname",
											ReadOnly:  false,
										},
									},
								},
								{
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
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBulder(c, &v1.ConfigMap{
				Data: map[string]string{},
			}),
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff := cmp.Diff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestStorageInitializerFailureCases(t *testing.T) {
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
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
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBulder(c, &v1.ConfigMap{
				Data: map[string]string{},
			}),
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			if !strings.HasPrefix(err.Error(), scenario.expectedErrorPrefix) {
				t.Errorf("Test %q unexpected failure [%s], expected: %s", name, err.Error(), scenario.expectedErrorPrefix)
			}
		} else {
			t.Errorf("Test %q should have failed with: %s", name, scenario.expectedErrorPrefix)
		}
	}
}

func TestCustomSpecStorageUriInjection(t *testing.T) {
	scenarios := map[string]struct {
		original                      *appsv1.Deployment
		expectedStorageUriEnvVariable *v1.EnvVar
	}{
		"CustomSpecStorageUriSet": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "user-container",
									Env: []v1.EnvVar{
										v1.EnvVar{
											Name:  constants.CustomSpecStorageUriEnvVarKey,
											Value: "pvc://mypvcname/some/path/on/pvc",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedStorageUriEnvVariable: &v1.EnvVar{
				Name:  constants.CustomSpecStorageUriEnvVarKey,
				Value: constants.DefaultModelLocalMountPath,
			},
		},
		"CustomSpecStorageUriEmpty": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "user-container",
									Env: []v1.EnvVar{
										v1.EnvVar{
											Name:  constants.CustomSpecStorageUriEnvVarKey,
											Value: "",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedStorageUriEnvVariable: &v1.EnvVar{
				Name:  constants.CustomSpecStorageUriEnvVarKey,
				Value: "",
			},
		},
		"CustomSpecStorageUriNotSet": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								v1.Container{
									Name: "user-container",
									Env: []v1.EnvVar{
										v1.EnvVar{
											Name:  "TestRandom",
											Value: "val",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedStorageUriEnvVariable: nil,
		},
	}

	for name, scenario := range scenarios {
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBulder(c, &v1.ConfigMap{
				Data: map[string]string{},
			}),
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}

		var originalEnvVar *v1.EnvVar
		for _, envVar := range scenario.original.Spec.Template.Spec.Containers[0].Env {
			if envVar.Name == constants.CustomSpecStorageUriEnvVarKey {
				originalEnvVar = &envVar
			}
		}
		if diff := cmp.Diff(scenario.expectedStorageUriEnvVariable, originalEnvVar); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func makeDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "user-container",
						},
					},
				},
			},
		},
	}
}

func TestCredentialInjection(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	scenarios := map[string]struct {
		sa       *v1.ServiceAccount
		secret   *v1.Secret
		original *appsv1.Deployment
		expected *appsv1.Deployment
	}{
		"Test s3 secrets injection": {
			sa: &v1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: "default",
				},
				Secrets: []v1.ObjectReference{
					{
						Name:      "s3-secret",
						Namespace: "default",
					},
				},
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-secret",
					Namespace: "default",
					Annotations: map[string]string{
						s3.InferenceServiceS3SecretEndpointAnnotation: "s3.aws.com",
					},
				},
				Data: map[string][]byte{
					"awsAccessKeyID":     {},
					"awsSecretAccessKey": {},
				},
			},
			original: makeDeployment(),
			expected: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
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
								{
									Name:  "storage-initializer",
									Image: StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
									Args:  []string{"gs://foo", constants.DefaultModelLocalMountPath},
									VolumeMounts: []v1.VolumeMount{
										{
											Name:      "kfserving-provision-location",
											MountPath: constants.DefaultModelLocalMountPath,
										},
									},
									Env: []v1.EnvVar{
										{
											Name: s3.AWSAccessKeyId,
											ValueFrom: &v1.EnvVarSource{
												SecretKeyRef: &v1.SecretKeySelector{
													LocalObjectReference: v1.LocalObjectReference{
														Name: "s3-secret",
													},
													Key: "awsAccessKeyID",
												},
											},
										},
										{
											Name: s3.AWSSecretAccessKey,
											ValueFrom: &v1.EnvVarSource{
												SecretKeyRef: &v1.SecretKeySelector{
													LocalObjectReference: v1.LocalObjectReference{
														Name: "s3-secret",
													},
													Key: "awsSecretAccessKey",
												},
											},
										},
										{
											Name:  s3.S3Endpoint,
											Value: "s3.aws.com",
										},
										{
											Name:  s3.AWSEndpointUrl,
											Value: "https://s3.aws.com",
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
		"Test GCS secrets injection": {
			sa: &v1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: "default",
				},
				Secrets: []v1.ObjectReference{
					{
						Name:      "user-gcp-sa",
						Namespace: "default",
					},
				},
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "user-gcp-sa",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"gcloud-application-credentials.json": {},
				},
			},
			original: makeDeployment(),
			expected: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
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
								{
									Name:  "storage-initializer",
									Image: StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
									Args:  []string{"gs://foo", constants.DefaultModelLocalMountPath},
									VolumeMounts: []v1.VolumeMount{
										{
											Name:      "kfserving-provision-location",
											MountPath: constants.DefaultModelLocalMountPath,
										},
										{
											Name:      gcs.GCSCredentialVolumeName,
											ReadOnly:  true,
											MountPath: gcs.GCSCredentialVolumeMountPath,
										},
									},
									Env: []v1.EnvVar{
										{
											Name:  gcs.GCSCredentialEnvKey,
											Value: gcs.GCSCredentialVolumeMountPath + "gcloud-application-credentials.json",
										},
									},
								},
							},
							Volumes: []v1.Volume{
								{
									Name: "kfserving-provision-location",
									VolumeSource: v1.VolumeSource{
										EmptyDir: &v1.EmptyDirVolumeSource{},
									},
								},
								{
									Name: gcs.GCSCredentialVolumeName,
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
	}

	var configMap = &v1.ConfigMap{
		Data: map[string]string{
			"credentials": `{
				"gcs" : {"gcsCredentialFileName": "gcloud-application-credentials.json"},
				"s3" : {
					"s3AccessKeyIDName": "awsAccessKeyID",
					"s3SecretAccessKeyName": "awsSecretAccessKey"
				}
			}`,
		},
	}

	builder := credentials.NewCredentialBulder(c, configMap)
	for name, scenario := range scenarios {
		g.Expect(c.Create(context.TODO(), scenario.sa)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Create(context.TODO(), scenario.secret)).NotTo(gomega.HaveOccurred())

		injector := &StorageInitializerInjector{
			credentialBuilder: builder,
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected failure [%s]", name, err.Error())
		}
		if diff := cmp.Diff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}

		g.Expect(c.Delete(context.TODO(), scenario.sa)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Delete(context.TODO(), scenario.secret)).NotTo(gomega.HaveOccurred())
	}
}

func TestStorageInitializerConfigmap(t *testing.T) {
	scenarios := map[string]struct {
		original *appsv1.Deployment
		expected *appsv1.Deployment
	}{
		"StorageInitializerConfig": {
			original: &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: v1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: map[string]string{
								constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
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
								constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
							},
						},
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
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
								{
									Name:  "storage-initializer",
									Image: "kfserving/storage-initializer@sha256:xxx",
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
								{
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
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBulder(c, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: &StorageInitializerConfig{
				Image: "kfserving/storage-initializer@sha256:xxx",
			},
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff := cmp.Diff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
