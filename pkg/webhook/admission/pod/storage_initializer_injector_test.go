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

package pod

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmp"
	"knative.dev/pkg/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/credentials/gcs"
	"github.com/kserve/kserve/pkg/credentials/s3"
)

const (
	StorageInitializerDefaultCPURequest                 = "100m"
	StorageInitializerDefaultCPULimit                   = "1"
	StorageInitializerDefaultMemoryRequest              = "200Mi"
	StorageInitializerDefaultMemoryLimit                = "1Gi"
	StorageInitializerDefaultCaBundleConfigMapName      = ""
	StorageInitializerDefaultCaBundleVolumeMountPath    = "/etc/ssl/custom-certs"
	StorageInitializerDefaultEnableDirectPvcVolumeMount = false
)

var (
	storageInitializerConfig = &StorageInitializerConfig{
		CpuRequest:                 StorageInitializerDefaultCPURequest,
		CpuLimit:                   StorageInitializerDefaultCPULimit,
		MemoryRequest:              StorageInitializerDefaultMemoryRequest,
		MemoryLimit:                StorageInitializerDefaultMemoryLimit,
		CaBundleConfigMapName:      StorageInitializerDefaultCaBundleConfigMapName,
		CaBundleVolumeMountPath:    StorageInitializerDefaultCaBundleVolumeMountPath,
		EnableDirectPvcVolumeMount: StorageInitializerDefaultEnableDirectPvcVolumeMount,
	}

	resourceRequirement = v1.ResourceRequirements{
		Limits: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(StorageInitializerDefaultCPULimit),
			v1.ResourceMemory: resource.MustParse(StorageInitializerDefaultMemoryLimit),
		},
		Requests: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resource.MustParse(StorageInitializerDefaultCPURequest),
			v1.ResourceMemory: resource.MustParse(StorageInitializerDefaultMemoryRequest),
		},
	}
)

func TestStorageInitializerInjector(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"MissingAnnotations": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
		},
		"AlreadyInjected": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
					InitContainers: []v1.Container{
						{
							Name: "storage-initializer",
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
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
		"StorageInitializerInjected": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},

		"StorageInitializerInjectedReadOnlyUnset": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: StorageInitializerVolumeName,
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},

		"StorageInitializerInjectedReadOnlyFalse": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.StorageReadonlyAnnotationKey:                     "false",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  false,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: StorageInitializerVolumeName,
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},

		"StorageInitializerInjectedReadOnlyTrue": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.StorageReadonlyAnnotationKey:                     "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: StorageInitializerVolumeName,
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},

		"StorageInitializerInjectedAndMountsPvc": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/pvc",
									ReadOnly:  true,
								},
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"/mnt/pvc/some/path/on/pvc", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/pvc",
									ReadOnly:  true,
								},
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-pvc-source",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "mypvcname",
									ReadOnly:  false,
								},
							},
						},
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageSpecInjected": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "<scheme-placeholder>://foo/bar",
						constants.StorageSpecAnnotationKey:                         "true",
						constants.StorageSpecParamAnnotationKey:                    `{"type": "s3", "bucket": "my-bucket"}`,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "<scheme-placeholder>://foo/bar",
						constants.StorageSpecAnnotationKey:                         "true",
						constants.StorageSpecParamAnnotationKey:                    `{"type": "s3", "bucket": "my-bucket"}`,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
							Args:  []string{"s3://my-bucket/foo/bar", constants.DefaultModelLocalMountPath},
							Env: []v1.EnvVar{
								{
									Name:  credentials.StorageOverrideConfigEnvKey,
									Value: `{"bucket":"my-bucket","type":"s3"}`,
								},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: storageInitializerConfig,
			client: c,
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestStorageInitializerFailureCases(t *testing.T) {
	scenarios := map[string]struct {
		original            *v1.Pod
		expectedErrorPrefix string
	}{
		"MissingUserContainer": {
			original: &v1.Pod{
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
			expectedErrorPrefix: "Invalid configuration: cannot find container",
		},
	}

	for name, scenario := range scenarios {
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: storageInitializerConfig,
			client: c,
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
		original                      *v1.Pod
		expectedStorageUriEnvVariable *v1.EnvVar
	}{
		"CustomSpecStorageUriSet": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: "pvc://mypvcname/some/path/on/pvc",
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
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: "",
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
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  "TestRandom",
									Value: "val",
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
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: storageInitializerConfig,
			client: c,
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}

		var originalEnvVar *v1.EnvVar
		for _, envVar := range scenario.original.Spec.Containers[0].Env {
			if envVar.Name == constants.CustomSpecStorageUriEnvVarKey {
				originalEnvVar = &envVar
			}
		}
		if diff, _ := kmp.SafeDiff(scenario.expectedStorageUriEnvVariable, originalEnvVar); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func makePod() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: constants.InferenceServiceContainerName,
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
		original *v1.Pod
		expected *v1.Pod
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
			original: makePod(),
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
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
			original: makePod(),
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
							Name: "kserve-provision-location",
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
		"TestStorageSpecSecretInjection": {
			sa: &v1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{ // Service account not used
					Name:      "default",
					Namespace: "default",
				},
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-config",
					Namespace: "default",
				},
				StringData: map[string]string{
					"my-storage": `{"type": "s3", "bucket": "my-bucket", "region": "na"}`,
				},
			},
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "<scheme-placeholder>://foo/bar",
						constants.StorageSpecAnnotationKey:                         "true",
						constants.StorageSpecParamAnnotationKey:                    `{"some-param": "some-val"}`,
						constants.StorageSpecKeyAnnotationKey:                      "my-storage",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "<scheme-placeholder>://foo/bar",
						constants.StorageSpecAnnotationKey:                         "true",
						constants.StorageSpecParamAnnotationKey:                    `{"some-param":"some-val"}`,
						constants.StorageSpecKeyAnnotationKey:                      "my-storage",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
							Args:  []string{"s3://my-bucket/foo/bar", constants.DefaultModelLocalMountPath},
							Env: []v1.EnvVar{
								{
									Name: credentials.StorageConfigEnvKey,
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											LocalObjectReference: v1.LocalObjectReference{Name: "storage-config"},
											Key:                  "my-storage",
										},
									},
								},
								{
									Name:  credentials.StorageOverrideConfigEnvKey,
									Value: `{"some-param":"some-val"}`,
								},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"TestStorageSpecDefaultSecretInjection": {
			sa: &v1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{ // Service account not used
					Name:      "default",
					Namespace: "default",
				},
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "storage-config",
					Namespace: "default",
				},
				StringData: map[string]string{
					credentials.DefaultStorageSecretKey: `{"type": "s3", "bucket": "my-bucket", "region": "na"}`,
				},
			},
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "<scheme-placeholder>://foo/bar",
						constants.StorageSpecAnnotationKey:                         "true",
						constants.StorageSpecParamAnnotationKey:                    `{"some-param": "some-val"}`,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "<scheme-placeholder>://foo/bar",
						constants.StorageSpecAnnotationKey:                         "true",
						constants.StorageSpecParamAnnotationKey:                    `{"some-param":"some-val"}`,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
							Args:  []string{"s3://my-bucket/foo/bar", constants.DefaultModelLocalMountPath},
							Env: []v1.EnvVar{
								{
									Name: credentials.StorageConfigEnvKey,
									ValueFrom: &v1.EnvVarSource{
										SecretKeyRef: &v1.SecretKeySelector{
											LocalObjectReference: v1.LocalObjectReference{Name: "storage-config"},
											Key:                  credentials.DefaultStorageSecretKey,
										},
									},
								},
								{
									Name:  credentials.StorageOverrideConfigEnvKey,
									Value: `{"some-param":"some-val"}`,
								},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	configMap := &v1.ConfigMap{
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

	builder := credentials.NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		g.Expect(c.Create(context.TODO(), scenario.sa)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Create(context.TODO(), scenario.secret)).NotTo(gomega.HaveOccurred())

		injector := &StorageInitializerInjector{
			credentialBuilder: builder,
			config:            storageInitializerConfig,
			client:            c,
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected failure [%s]", name, err.Error())
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}

		g.Expect(c.Delete(context.TODO(), scenario.sa)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Delete(context.TODO(), scenario.secret)).NotTo(gomega.HaveOccurred())
	}
}

func TestStorageInitializerConfigmap(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"StorageInitializerConfig": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    "kserve/storage-initializer@sha256:xxx",
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: &StorageInitializerConfig{
				Image:                   "kserve/storage-initializer@sha256:xxx",
				CpuRequest:              StorageInitializerDefaultCPURequest,
				CpuLimit:                StorageInitializerDefaultCPULimit,
				MemoryRequest:           StorageInitializerDefaultMemoryRequest,
				MemoryLimit:             StorageInitializerDefaultMemoryLimit,
				CaBundleConfigMapName:   StorageInitializerDefaultCaBundleConfigMapName,
				CaBundleVolumeMountPath: StorageInitializerDefaultCaBundleVolumeMountPath,
			},
			client: c,
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestGetStorageInitializerConfigs(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cases := []struct {
		name      string
		configMap *v1.ConfigMap
		matchers  []types.GomegaMatcher
	}{
		{
			name: "Valid Storage Initializer Config",
			configMap: &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string]string{
					StorageInitializerConfigMapKeyName: `{
						"Image":        		 "gcr.io/kserve/storage-initializer:latest",
						"CpuRequest":   		 "100m",
						"CpuLimit":      		 "1",
						"MemoryRequest": 		 "200Mi",
						"MemoryLimit":   		 "1Gi",
						"CaBundleConfigMapName":      "",
						"CaBundleVolumeMountPath": "/etc/ssl/custom-certs"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&StorageInitializerConfig{
					Image:                   "gcr.io/kserve/storage-initializer:latest",
					CpuRequest:              "100m",
					CpuLimit:                "1",
					MemoryRequest:           "200Mi",
					MemoryLimit:             "1Gi",
					CaBundleConfigMapName:   "",
					CaBundleVolumeMountPath: "/etc/ssl/custom-certs",
				}),
				gomega.BeNil(),
			},
		},
		{
			name: "Invalid Resource Value",
			configMap: &v1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Data: map[string]string{
					StorageInitializerConfigMapKeyName: `{
						"Image":        		 "gcr.io/kserve/storage-initializer:latest",
						"CpuRequest":   		 "100m",
						"CpuLimit":      		 "1",
						"MemoryRequest": 		 "200MC",
						"MemoryLimit":   		 "1Gi",
						"CaBundleConfigMapName":      "",
						"CaBundleVolumeMountPath": "/etc/ssl/custom-certs"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&StorageInitializerConfig{
					Image:                   "gcr.io/kserve/storage-initializer:latest",
					CpuRequest:              "100m",
					CpuLimit:                "1",
					MemoryRequest:           "200MC",
					MemoryLimit:             "1Gi",
					CaBundleConfigMapName:   "",
					CaBundleVolumeMountPath: "/etc/ssl/custom-certs",
				}),
				gomega.HaveOccurred(),
			},
		},
	}

	for _, tc := range cases {
		loggerConfigs, err := getStorageInitializerConfigs(tc.configMap)
		g.Expect(err).Should(tc.matchers[1])
		g.Expect(loggerConfigs).Should(tc.matchers[0])
	}
}

func TestParsePvcURI(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	cases := []struct {
		name     string
		uri      string
		matchers []types.GomegaMatcher
	}{
		{
			name: "Valid PVC URI",
			uri:  "pvc://test/model/model1",
			matchers: []types.GomegaMatcher{
				gomega.Equal("test"),
				gomega.Equal("model/model1"),
				gomega.BeNil(),
			},
		},
		{
			name: "Valid PVC URI with Shortest Path",
			uri:  "pvc://test",
			matchers: []types.GomegaMatcher{
				gomega.Equal("test"),
				gomega.Equal(""),
				gomega.BeNil(),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pvcName, pvcPath, err := parsePvcURI(tc.uri)
			g.Expect(pvcName).Should(tc.matchers[0])
			g.Expect(pvcPath).Should(tc.matchers[1])
			g.Expect(err).Should(tc.matchers[2])
		})
	}
}

func TestCaBundleConfigMapVolumeMountInStorageInitializer(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	configMap := &v1.ConfigMap{
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
	scenarios := map[string]struct {
		storageConfig *StorageInitializerConfig
		secret        *v1.Secret
		sa            *v1.ServiceAccount
		original      *v1.Pod
		expected      *v1.Pod
	}{
		"DoNotMountWithCaBundleConfigMapVolumeWhenCaBundleConfigMapNameNotSet": {
			storageConfig: storageInitializerConfig,
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"awsAccessKeyID":     {},
					"awsSecretAccessKey": {},
				},
			},
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
			original: makePod(),
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"MountsCaBundleConfigMapVolumeWhenCaBundleConfigMapNameSet": {
			storageConfig: &StorageInitializerConfig{
				Image:                 "kserve/storage-initializer:latest",
				CpuRequest:            "100m",
				CpuLimit:              "1",
				MemoryRequest:         "200Mi",
				MemoryLimit:           "1Gi",
				CaBundleConfigMapName: "custom-certs", // enable CA bundle config volume mount
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"awsAccessKeyID":     {},
					"awsSecretAccessKey": {},
				},
			},
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
			original: makePod(),
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
								{Name: "CA_BUNDLE_CONFIGMAP_NAME", Value: constants.DefaultGlobalCaBundleConfigMapName},
								{Name: "CA_BUNDLE_VOLUME_MOUNT_POINT", Value: "/etc/ssl/custom-certs"},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
								{
									Name:      CaBundleVolumeName,
									MountPath: constants.DefaultCaBundleVolumeMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: CaBundleVolumeName,
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: constants.DefaultGlobalCaBundleConfigMapName,
									},
								},
							},
						},
					},
				},
			},
		},
		"MountsCaBundleConfigMapVolumeByAnnotation": {
			storageConfig: &StorageInitializerConfig{
				Image:         "kserve/storage-initializer:latest",
				CpuRequest:    "100m",
				CpuLimit:      "1",
				MemoryRequest: "200Mi",
				MemoryLimit:   "1Gi",
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-secret",
					Namespace: "default",
					Annotations: map[string]string{
						s3.InferenceServiceS3CABundleConfigMapAnnotation: "cabundle-annotation",
					},
				},
				Data: map[string][]byte{
					"awsAccessKeyID":     {},
					"awsSecretAccessKey": {},
				},
			},
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
			original: makePod(),
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
								{Name: "AWS_CA_BUNDLE_CONFIGMAP", Value: "cabundle-annotation"},
								{Name: "CA_BUNDLE_CONFIGMAP_NAME", Value: "cabundle-annotation"},
								{Name: "CA_BUNDLE_VOLUME_MOUNT_POINT", Value: "/etc/ssl/custom-certs"},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
								{
									Name:      CaBundleVolumeName,
									MountPath: constants.DefaultCaBundleVolumeMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: CaBundleVolumeName,
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "cabundle-annotation",
									},
								},
							},
						},
					},
				},
			},
		},
		"MountsCaBundleConfigMapVolumeByAnnotationInstreadOfConfigMap": {
			storageConfig: &StorageInitializerConfig{
				Image:                 "kserve/storage-initializer:latest",
				CpuRequest:            "100m",
				CpuLimit:              "1",
				MemoryRequest:         "200Mi",
				MemoryLimit:           "1Gi",
				CaBundleConfigMapName: "custom-certs", // enable CA bundle configmap volume mount
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-secret",
					Namespace: "default",
					Annotations: map[string]string{
						s3.InferenceServiceS3CABundleConfigMapAnnotation: "cabundle-annotation",
					},
				},
				Data: map[string][]byte{
					"awsAccessKeyID":     {},
					"awsSecretAccessKey": {},
				},
			},
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
			original: makePod(),
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
								{Name: "AWS_CA_BUNDLE_CONFIGMAP", Value: "cabundle-annotation"},
								{Name: "CA_BUNDLE_CONFIGMAP_NAME", Value: "cabundle-annotation"},
								{Name: "CA_BUNDLE_VOLUME_MOUNT_POINT", Value: "/etc/ssl/custom-certs"},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
								{
									Name:      CaBundleVolumeName,
									MountPath: constants.DefaultCaBundleVolumeMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: CaBundleVolumeName,
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "cabundle-annotation",
									},
								},
							},
						},
					},
				},
			},
		},
		"DoNotSetMountsCaBundleConfigMapVolumePathByAnnotationIfCaBundleConfigMapNameDidNotSet": {
			storageConfig: storageInitializerConfig,
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-secret",
					Namespace: "default",
					Annotations: map[string]string{
						s3.InferenceServiceS3CABundleAnnotation: "/path/to/ca.crt",
					},
				},
				Data: map[string][]byte{
					"awsAccessKeyID":     {},
					"awsSecretAccessKey": {},
				},
			},
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
			original: makePod(),
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
								{Name: "AWS_CA_BUNDLE", Value: "/path/to/ca.crt"},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"SetMountsCaBundleConfigMapVolumePathByAnnotationInstreadOfConfigMap": {
			storageConfig: &StorageInitializerConfig{
				Image:                   "kserve/storage-initializer:latest",
				CpuRequest:              "100m",
				CpuLimit:                "1",
				MemoryRequest:           "200Mi",
				MemoryLimit:             "1Gi",
				CaBundleConfigMapName:   "custom-certs", // enable CA bundle configmap volume mount
				CaBundleVolumeMountPath: "/path/to",     // set CA bundle configmap volume mount path
			},
			secret: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "s3-secret",
					Namespace: "default",
					Annotations: map[string]string{
						s3.InferenceServiceS3CABundleAnnotation: "/annotation/path/to/annotation-ca.crt",
					},
				},
				Data: map[string][]byte{
					"awsAccessKeyID":     {},
					"awsSecretAccessKey": {},
				},
			},
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
			original: makePod(),
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
								{Name: "AWS_CA_BUNDLE", Value: "/annotation/path/to/annotation-ca.crt"},
								{Name: "CA_BUNDLE_CONFIGMAP_NAME", Value: constants.DefaultGlobalCaBundleConfigMapName},
								{Name: "CA_BUNDLE_VOLUME_MOUNT_POINT", Value: "/annotation/path/to"},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
								{
									Name:      CaBundleVolumeName,
									MountPath: "/annotation/path/to",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: CaBundleVolumeName,
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: constants.DefaultGlobalCaBundleConfigMapName,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	builder := credentials.NewCredentialBuilder(c, clientset, configMap)
	for name, scenario := range scenarios {
		g.Expect(c.Create(context.TODO(), scenario.sa)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Create(context.TODO(), scenario.secret)).NotTo(gomega.HaveOccurred())

		injector := &StorageInitializerInjector{
			credentialBuilder: builder,
			config:            scenario.storageConfig,
			client:            c,
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected failure [%s]", name, err.Error())
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}

		g.Expect(c.Delete(context.TODO(), scenario.secret)).NotTo(gomega.HaveOccurred())
		g.Expect(c.Delete(context.TODO(), scenario.sa)).NotTo(gomega.HaveOccurred())
	}
}

func TestDirectVolumeMountForPvc(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"StorageInitializerNotInjectedAndMountsPvcViaVolumeMount": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/models",
									SubPath:   "some/path/on/pvc",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-pvc-source",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "mypvcname",
									ReadOnly:  false,
								},
							},
						},
					},
				},
			},
		},
		"StorageInitializerNotInjectedAndMountsPvcViaVolumeMountShortestPath": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/models",
									SubPath:   "", // volume's root
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-pvc-source",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "mypvcname",
									ReadOnly:  false,
								},
							},
						},
					},
				},
			},
		},
		"StorageInitializerNotInjectedAndMountsPvcViaVolumeMountReadOnlyFalse": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
						constants.StorageReadonlyAnnotationKey:                     "false",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/models",
									SubPath:   "some/path/on/pvc",
									ReadOnly:  false,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-pvc-source",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "mypvcname",
									ReadOnly:  false,
								},
							},
						},
					},
				},
			},
		},
		"StorageInitializerNotInjectedAndMountsPvcViaVolumeMountReadOnlyTrue": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
						constants.StorageReadonlyAnnotationKey:                     "true",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/models",
									SubPath:   "some/path/on/pvc",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-pvc-source",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "mypvcname",
									ReadOnly:  false,
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
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: &StorageInitializerConfig{
				EnableDirectPvcVolumeMount: true, // enable direct volume mount for PVC
			},
			client: c,
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestTransformerCollocation(t *testing.T) {
	scenarios := map[string]struct {
		storageConfig *StorageInitializerConfig
		original      *v1.Pod
		expected      *v1.Pod
	}{
		"Transformer collocation with pvc": {
			storageConfig: storageInitializerConfig,
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: "pvc://mypvcname/some/path/on/pvc",
								},
							},
						},
						{
							Name:  constants.TransformerContainerName,
							Image: "test/image:latest",
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: constants.DefaultModelLocalMountPath,
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/pvc",
									ReadOnly:  true,
								},
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name:  constants.TransformerContainerName,
							Image: "test/image:latest",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/pvc",
									ReadOnly:  true,
								},
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"/mnt/pvc/some/path/on/pvc", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/pvc",
									ReadOnly:  true,
								},
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-pvc-source",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "mypvcname",
									ReadOnly:  false,
								},
							},
						},
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"Transformer collocation with pvc direct mount": {
			storageConfig: &StorageInitializerConfig{
				EnableDirectPvcVolumeMount: true, // enable direct volume mount for PVC
			},
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: "pvc://mypvcname/some/path/on/pvc",
								},
							},
						},
						{
							Name:  constants.TransformerContainerName,
							Image: "test/image:latest",
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: constants.DefaultModelLocalMountPath,
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/models",
									SubPath:   "some/path/on/pvc",
									ReadOnly:  true,
								},
							},
						},
						{
							Name:  constants.TransformerContainerName,
							Image: "test/image:latest",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/models",
									SubPath:   "some/path/on/pvc",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-pvc-source",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "mypvcname",
									ReadOnly:  false,
								},
							},
						},
					},
				},
			},
		},
		"No collocation": {
			storageConfig: storageInitializerConfig,
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "pvc://mypvcname/some/path/on/pvc",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: "pvc://mypvcname/some/path/on/pvc",
								},
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							Env: []v1.EnvVar{
								{
									Name:  constants.CustomSpecStorageUriEnvVarKey,
									Value: constants.DefaultModelLocalMountPath,
								},
							},
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/pvc",
									ReadOnly:  true,
								},
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"/mnt/pvc/some/path/on/pvc", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-pvc-source",
									MountPath: "/mnt/pvc",
									ReadOnly:  true,
								},
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-pvc-source",
							VolumeSource: v1.VolumeSource{
								PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
									ClaimName: "mypvcname",
									ReadOnly:  false,
								},
							},
						},
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: scenario.storageConfig,
			client: c,
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestGetStorageContainerSpec(t *testing.T) {
	g := gomega.NewWithT(t)
	customSpec := v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom",
		},
		Spec: v1alpha1.StorageContainerSpec{
			Container: v1.Container{
				Image: "kserve/custom:latest",
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("200Mi"),
					},
				},
				SecurityContext: &v1.SecurityContext{
					RunAsNonRoot: ptr.Bool(true),
				},
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "custom://"}},
		},
	}
	s3AzureSpec := v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "s3-azure",
		},
		Spec: v1alpha1.StorageContainerSpec{
			Container: v1.Container{
				Image: "kserve/storage-initializer:latest",
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("200Mi"),
					},
				},
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "s3://"}, {Regex: "https://(.+?).blob.core.windows.net/(.+)"}},
		},
	}

	if err := c.Create(context.TODO(), &s3AzureSpec); err != nil {
		t.Fatalf("unable to create cluster storage container: %v", err)
	}
	if err := c.Create(context.TODO(), &customSpec); err != nil {
		t.Fatalf("unable to create cluster storage container: %v", err)
	}
	defer func() {
		if err := c.Delete(context.TODO(), &s3AzureSpec); err != nil {
			t.Errorf("unable to delete cluster storage container: %v", err)
		}
		if err := c.Delete(context.TODO(), &customSpec); err != nil {
			t.Errorf("unable to delete cluster storage container: %v", err)
		}
	}()
	scenarios := map[string]struct {
		storageUri   string
		expectedSpec *v1.Container
	}{
		"s3": {
			storageUri:   "s3://foo",
			expectedSpec: &s3AzureSpec.Spec.Container,
		},
		"custom": {
			storageUri:   "custom://foo",
			expectedSpec: &customSpec.Spec.Container,
		},
		"nonExistent": {
			storageUri:   "abc://",
			expectedSpec: nil,
		},
	}
	for name, scenario := range scenarios {
		var container *v1.Container
		var err error

		if container, err = GetContainerSpecForStorageUri(context.Background(), scenario.storageUri, c); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		g.Expect(container).To(gomega.Equal(scenario.expectedSpec))
	}
}

func TestStorageContainerCRDInjection(t *testing.T) {
	customSpec := v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom",
		},
		Spec: v1alpha1.StorageContainerSpec{
			Container: v1.Container{
				Image: "kserve/storage-initializer:latest",
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("200Mi"),
					},
				},
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "custom://"}},
		},
	}
	s3AzureSpec := v1alpha1.ClusterStorageContainer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "s3-azure",
		},
		Spec: v1alpha1.StorageContainerSpec{
			Container: v1.Container{
				Image: "kserve/storage-initializer:latest",
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("500Mi"),
					},
				},
				Env: []v1.EnvVar{
					{Name: "name", Value: "value"},
				},
			},
			SupportedUriFormats: []v1alpha1.SupportedUriFormat{{Prefix: "s3://"}, {Regex: "https://(.+?).blob.core.windows.net/(.+)"}},
		},
	}
	if err := c.Create(context.TODO(), &s3AzureSpec); err != nil {
		t.Fatalf("unable to create cluster storage container: %v", err)
	}
	if err := c.Create(context.TODO(), &customSpec); err != nil {
		t.Fatalf("unable to create cluster storage container: %v", err)
	}
	defer func() {
		if err := c.Delete(context.TODO(), &s3AzureSpec); err != nil {
			t.Errorf("unable to delete cluster storage container: %v", err)
		}
		if err := c.Delete(context.TODO(), &customSpec); err != nil {
			t.Errorf("unable to delete cluster storage container: %v", err)
		}
	}()

	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"s3": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
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
							Args:  []string{"s3://foo", constants.DefaultModelLocalMountPath},
							Resources: v1.ResourceRequirements{
								Limits: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse(StorageInitializerDefaultCPULimit),
									v1.ResourceMemory: resource.MustParse("500Mi"), // From CRD
								},
								Requests: map[v1.ResourceName]resource.Quantity{
									v1.ResourceCPU:    resource.MustParse(StorageInitializerDefaultCPURequest),
									v1.ResourceMemory: resource.MustParse(StorageInitializerDefaultMemoryRequest),
								},
							},
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
							Env: []v1.EnvVar{
								{Name: "name", Value: "value"},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"Default config if storage uri not matched in CRs": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "https://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "https://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"https://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement, // from configMap instead of the CR
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
	for name, scenario := range scenarios {
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: storageInitializerConfig,
			client: c,
		}

		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestAddOrReplaceEnv(t *testing.T) {
	tests := []struct {
		name       string
		container  *v1.Container
		envKey     string
		envValue   string
		wantEnvLen int
		wantValue  string
	}{
		{
			name:       "nil env array",
			container:  &v1.Container{},
			envKey:     "TEST_KEY",
			envValue:   "test_value",
			wantEnvLen: 1,
			wantValue:  "test_value",
		},
		{
			name: "env array without key",
			container: &v1.Container{
				Env: []v1.EnvVar{
					{Name: "EXISTING_KEY", Value: "existing_value"},
				},
			},
			envKey:     "TEST_KEY",
			envValue:   "test_value",
			wantEnvLen: 2,
			wantValue:  "test_value",
		},
		{
			name: "env array with existing key",
			container: &v1.Container{
				Env: []v1.EnvVar{
					{Name: "TEST_KEY", Value: "old_value"},
				},
			},
			envKey:     "TEST_KEY",
			envValue:   "new_value",
			wantEnvLen: 1,
			wantValue:  "new_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addOrReplaceEnv(tt.container, tt.envKey, tt.envValue)

			if len(tt.container.Env) != tt.wantEnvLen {
				t.Errorf("Expected env length %d, but got %d", tt.wantEnvLen, len(tt.container.Env))
			}

			for _, envVar := range tt.container.Env {
				if envVar.Name == tt.envKey && envVar.Value != tt.wantValue {
					t.Errorf("Expected value for %s to be %s, but got %s", tt.envKey, tt.wantValue, envVar.Value)
				}
			}
		})
	}
}

func TestInjectModelcar(t *testing.T) {
	// Test when annotation key is not set
	{
		pod := &v1.Pod{}
		mi := &StorageInitializerInjector{}
		err := mi.InjectModelcar(pod)
		if err != nil {
			t.Errorf("Expected nil error but got %v", err)
		}
		if len(pod.Spec.Containers) != 0 {
			t.Errorf("Expected no containers but got %d", len(pod.Spec.Containers))
		}
	}

	// Test when srcURI does not start with OciURIPrefix
	{
		pod := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.StorageInitializerSourceUriInternalAnnotationKey: "s3://bla/blub",
				},
			},
		}
		mi := &StorageInitializerInjector{}
		err := mi.InjectModelcar(pod)
		if err != nil {
			t.Errorf("Expected nil error but got %v", err)
		}
		if len(pod.Spec.Containers) != 0 {
			t.Errorf("Expected no containers but got %d", len(pod.Spec.Containers))
		}
	}

	// Test when srcURI starts with OciURIPrefix
	{
		pod := createTestPodForModelcar()
		mi := &StorageInitializerInjector{
			config: &StorageInitializerConfig{},
		}
		err := mi.InjectModelcar(pod)
		if err != nil {
			t.Errorf("Expected nil error but got %v", err)
		}

		// Check that an emptyDir volume has been attached
		if len(pod.Spec.Volumes) != 1 || pod.Spec.Volumes[0].Name != StorageInitializerVolumeName {
			t.Errorf("Expected one volume with name %s, but got %v", StorageInitializerVolumeName, pod.Spec.Volumes)
		}

		// Check that a sidecar container has been injected
		if len(pod.Spec.Containers) != 2 {
			t.Errorf("Expected two containers but got %d", len(pod.Spec.Containers))
		}

		// Check that an init container has been injected, and it is the model container
		switch {
		case len(pod.Spec.InitContainers) != 1:
			t.Errorf("Expected one init container but got %d", len(pod.Spec.InitContainers))
		case pod.Spec.InitContainers[0].Name != ModelcarInitContainerName:
			t.Errorf("Expected the init container to be the model but got %s", pod.Spec.InitContainers[0].Name)
		default:
			// Check that resources are correctly set.
			if _, ok := pod.Spec.InitContainers[0].Resources.Limits[v1.ResourceCPU]; !ok {
				t.Error("The model container does not have CPU limit set")
			}
			if _, ok := pod.Spec.InitContainers[0].Resources.Limits[v1.ResourceMemory]; !ok {
				t.Error("The model container does not have Memory limit set")
			}
			if _, ok := pod.Spec.InitContainers[0].Resources.Requests[v1.ResourceCPU]; !ok {
				t.Error("The model container does not have CPU request set")
			}
			if _, ok := pod.Spec.InitContainers[0].Resources.Requests[v1.ResourceMemory]; !ok {
				t.Error("The model container does not have Memory request set")
			}

			// Check args
			joinedArgs := strings.Join(pod.Spec.InitContainers[0].Args, " ")
			if !strings.Contains(joinedArgs, "Prefetched") {
				t.Errorf("The model container args are not correctly setup. Got: %s", joinedArgs)
			}
		}

		// Check that the user-container has an env var set
		found := false
		if pod.Spec.Containers[0].Env != nil {
			for _, env := range pod.Spec.Containers[0].Env {
				if env.Name == ModelInitModeEnv && env.Value == "async" {
					found = true
					break
				}
			}
		}
		if !found {
			t.Errorf("Expected env var %s=async but did not find it", ModelInitModeEnv)
		}

		// Check volume mounts in both containers
		if len(pod.Spec.Containers[0].VolumeMounts) != 1 || len(pod.Spec.Containers[1].VolumeMounts) != 1 {
			t.Errorf("Expected one volume mount in each container but got user-container: %d, sidecar-container: %d",
				len(pod.Spec.Containers[0].VolumeMounts), len(pod.Spec.Containers[1].VolumeMounts))
		}

		// Check ShareProcessNamespace
		if pod.Spec.ShareProcessNamespace == nil || *pod.Spec.ShareProcessNamespace != true {
			t.Errorf("Expected ShareProcessNamespace to be true but got %v", pod.Spec.ShareProcessNamespace)
		}
	}
}

func createTestPodForModelcar() *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				constants.StorageInitializerSourceUriInternalAnnotationKey: OciURIPrefix + "myrepo/mymodelimage",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{Name: constants.InferenceServiceContainerName},
			},
		},
	}
	return pod
}

func createTestPodForModelcarWithTransformer() *v1.Pod {
	pod := createTestPodForModelcar()
	pod.Spec.Containers = append(pod.Spec.Containers, v1.Container{Name: constants.TransformerContainerName})
	return pod
}

func TestModelcarVolumeMounts(t *testing.T) {
	t.Run("Test that volume mounts has been added (no transformer)", func(t *testing.T) {
		pod := createTestPodForModelcar()
		assert.Nil(t, getContainerWithName(pod, constants.TransformerContainerName))
		checkVolumeMounts(t, pod, []string{ModelcarContainerName, constants.InferenceServiceContainerName})
	})

	t.Run("Test that volume mounts has been added (with transformer)", func(t *testing.T) {
		pod := createTestPodForModelcarWithTransformer()
		checkVolumeMounts(t, pod, []string{ModelcarContainerName, constants.InferenceServiceContainerName, constants.TransformerContainerName})
	})
}

func checkVolumeMounts(t *testing.T, pod *v1.Pod, containerNames []string) {
	injector := &StorageInitializerInjector{config: &StorageInitializerConfig{}}
	err := injector.InjectModelcar(pod)
	assert.Nil(t, err)

	for _, containerName := range containerNames {
		container := getContainerWithName(pod, containerName)
		assert.NotNil(t, container)
		volumeMounts := container.VolumeMounts
		assert.NotEmpty(t, volumeMounts)
		assert.Len(t, volumeMounts, 1)
		assert.Equal(t, volumeMounts[0].MountPath, getParentDirectory(constants.DefaultModelLocalMountPath))
	}
}

func TestModelcarIdempotency(t *testing.T) {
	t.Run("Test that calling the modelcar injector twice results with the same input pod, the injected pod is the same", func(t *testing.T) {
		podReference := createTestPodForModelcarWithTransformer()
		pod := createTestPodForModelcarWithTransformer()

		injector := &StorageInitializerInjector{config: &StorageInitializerConfig{}}

		// Inject modelcar twice
		err := injector.InjectModelcar(pod)
		assert.Nil(t, err)
		err = injector.InjectModelcar(pod)
		assert.Nil(t, err)

		// Reference modelcar
		err = injector.InjectModelcar(podReference)
		assert.Nil(t, err)

		// It should not make a difference if the modelcar is injected once or twice
		assert.True(t, reflect.DeepEqual(podReference, pod))
	})
}

func TestStorageInitializerInjectorWithModelcarConfig(t *testing.T) {
	t.Run("Test empty config", func(t *testing.T) {
		config := &StorageInitializerConfig{}
		injector := &StorageInitializerInjector{config: config}

		pod := createTestPodForModelcar()
		err := injector.InjectModelcar(pod)
		assert.Nil(t, err)

		// Assertions
		modelcarContainer := getContainerWithName(pod, ModelcarContainerName)
		assert.NotNil(t, modelcarContainer)
		assert.Equal(t, resource.MustParse(CpuModelcarDefault), modelcarContainer.Resources.Limits["cpu"])
		assert.Equal(t, resource.MustParse(MemoryModelcarDefault), modelcarContainer.Resources.Limits["memory"])
		assert.Equal(t, resource.MustParse(CpuModelcarDefault), modelcarContainer.Resources.Requests["cpu"])
		assert.Equal(t, resource.MustParse(MemoryModelcarDefault), modelcarContainer.Resources.Requests["memory"])
		assert.Nil(t, modelcarContainer.SecurityContext)
	})

	t.Run("Test uidModelcar config", func(t *testing.T) {
		config := &StorageInitializerConfig{UidModelcar: ptr.Int64(10)}
		injector := &StorageInitializerInjector{config: config}

		pod := createTestPodForModelcar()
		err := injector.InjectModelcar(pod)
		assert.Nil(t, err)

		// Assertions
		modelcarContainer := getContainerWithName(pod, ModelcarContainerName)
		userContainer := getContainerWithName(pod, constants.InferenceServiceContainerName)
		assert.NotNil(t, modelcarContainer)
		assert.NotNil(t, userContainer)
		assert.Equal(t, int64(10), *modelcarContainer.SecurityContext.RunAsUser)
		assert.Equal(t, int64(10), *userContainer.SecurityContext.RunAsUser)
	})

	t.Run("Test CPU and Memory config", func(t *testing.T) {
		config := &StorageInitializerConfig{CpuModelcar: "50m", MemoryModelcar: "50Mi"}
		injector := &StorageInitializerInjector{config: config}

		pod := createTestPodForModelcar()
		err := injector.InjectModelcar(pod)
		assert.Nil(t, err)

		// Assertions
		modelcarContainer := getContainerWithName(pod, ModelcarContainerName)
		assert.NotNil(t, modelcarContainer)
		assert.Equal(t, resource.MustParse("50m"), modelcarContainer.Resources.Limits["cpu"])
		assert.Equal(t, resource.MustParse("50Mi"), modelcarContainer.Resources.Requests["memory"])
		assert.Equal(t, resource.MustParse("50m"), modelcarContainer.Resources.Limits["cpu"])
		assert.Equal(t, resource.MustParse("50Mi"), modelcarContainer.Resources.Requests["memory"])
	})
}

func TestGetContainerWithName(t *testing.T) {
	// Test case: Container exists
	{
		pod := &v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Name: "container-1"},
					{Name: "container-2"},
				},
			},
		}

		seekName := "container-1"
		got := getContainerWithName(pod, seekName)

		if got == nil {
			t.Errorf("Expected a container, but got nil")
		} else if got.Name != seekName {
			t.Errorf("Expected container name %s, but got %s", seekName, got.Name)
		}
	}

	// Test case: Container does not exist
	{
		pod := &v1.Pod{
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{Name: "container-1"},
					{Name: "container-2"},
				},
			},
		}

		seekName := "non-existent-container"
		got := getContainerWithName(pod, seekName)

		if got != nil {
			t.Errorf("Expected nil, but got a container")
		}
	}
}

func TestStorageInitializerUIDForIstioCNI(t *testing.T) {
	scenarios := map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	}{
		"StorageInitializerCniUidSet": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioSidecarStatusAnnotation:                     "{\"containers\": [\"istio-sidecar\"]}",
						constants.IstioInterceptionModeAnnotation:                  constants.IstioInterceptModeRedirect,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageInitializerCniUidDefault": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioSidecarStatusAnnotation:                     "{\"containers\": [\"istio-sidecar\"]}",
						constants.IstioInterceptionModeAnnotation:                  constants.IstioInterceptModeRedirect,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "istio-sidecar",
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name: "istio-sidecar",
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(1337),
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageInitializerUidNotSetIfIstioInitPresent": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioSidecarStatusAnnotation:                     "{\"containers\": [\"istio-sidecar\"]}",
						constants.IstioInterceptionModeAnnotation:                  constants.IstioInterceptModeRedirect,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name: constants.IstioInitContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name: constants.IstioInitContainerName,
						},
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageInitializerUidNotSetIfProxyMissing": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioSidecarStatusAnnotation:                     "{\"containers\": [\"istio-sidecar\"]}",
						constants.IstioInterceptionModeAnnotation:                  constants.IstioInterceptModeRedirect,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageInitializerUidNotSetIfProxyNameMissing": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioSidecarStatusAnnotation:                     "{\"containers\": []}",
						constants.IstioInterceptionModeAnnotation:                  constants.IstioInterceptModeRedirect,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageInitializerUidNotSetIfIstioStatusBlank": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioSidecarStatusAnnotation:                     "{}",
						constants.IstioInterceptionModeAnnotation:                  constants.IstioInterceptModeRedirect,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageInitializerUidNotSetIfInterceptModeNotRedirect": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioSidecarStatusAnnotation:                     "{\"containers\": [\"istio-sidecar\"]}",
						constants.IstioInterceptionModeAnnotation:                  "OTHER_REDIRECT",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageInitializerUidNotSetIfInterceptModeMissing": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioSidecarStatusAnnotation:                     "{\"containers\": [\"istio-sidecar\"]}",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageInitializerUidNotSetIfIstioStatusMissing": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioInterceptionModeAnnotation:                  constants.IstioInterceptModeRedirect,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
		"StorageInitializerUidNotSetIfIstioStatusEmpty": {
			original: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
						constants.IstioSidecarStatusAnnotation:                     "{\"containers\": []}",
						constants.IstioInterceptionModeAnnotation:                  constants.IstioInterceptModeRedirect,
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
				},
			},
			expected: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
						{
							Name: "istio-sidecar",
							SecurityContext: &v1.SecurityContext{
								RunAsUser: ptr.Int64(501),
							},
						},
					},
					InitContainers: []v1.Container{
						{
							Name:                     "storage-initializer",
							Image:                    StorageInitializerContainerImage + ":" + StorageInitializerContainerImageVersion,
							Args:                     []string{"gs://foo", constants.DefaultModelLocalMountPath},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "kserve-provision-location",
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "kserve-provision-location",
							VolumeSource: v1.VolumeSource{
								EmptyDir: &v1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: storageInitializerConfig,
			client: c,
		}
		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if err := injector.SetIstioCniSecurityContext(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}

func TestLocalModelPVC(t *testing.T) {
	storageConfig := &StorageInitializerConfig{
		EnableDirectPvcVolumeMount: true, // enable direct volume mount for PVC
	}
	scenarios := map[string]struct {
		storageUri               string
		localModelLabel          string
		localModelSourceUriLabel string
		pvcName                  string
		expectedSubPath          string
	}{
		"basic": {
			storageUri:               "s3://foo",
			localModelLabel:          "bar",
			localModelSourceUriLabel: "s3://foo",
			pvcName:                  "model-h100",
			expectedSubPath:          "models/bar/",
		},
		"extra / at the end": {
			storageUri:               "s3://foo/",
			localModelLabel:          "bar",
			localModelSourceUriLabel: "s3://foo",
			pvcName:                  "model-h100",
			expectedSubPath:          "models/bar/",
		},
		"subfolder": {
			storageUri:               "s3://foo/model1",
			localModelLabel:          "bar",
			localModelSourceUriLabel: "s3://foo",
			pvcName:                  "model-h100",
			expectedSubPath:          "models/bar/model1",
		},
		"subfolder2": {
			storageUri:               "s3://foo/model1",
			localModelLabel:          "bar",
			localModelSourceUriLabel: "s3://foo/",
			pvcName:                  "model-h100",
			expectedSubPath:          "models/bar/model1",
		},
	}

	podScenarios := make(map[string]struct {
		original *v1.Pod
		expected *v1.Pod
	})

	for name, scenario := range scenarios {
		original := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.StorageInitializerSourceUriInternalAnnotationKey: scenario.storageUri,
					constants.LocalModelSourceUriAnnotationKey:                 scenario.localModelSourceUriLabel,
					constants.LocalModelPVCNameAnnotationKey:                   scenario.pvcName,
				},
				Labels: map[string]string{
					constants.LocalModelLabel: scenario.localModelLabel,
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name: constants.InferenceServiceContainerName,
					},
				},
			},
		}
		expected := &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.StorageInitializerSourceUriInternalAnnotationKey: scenario.storageUri,
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name: constants.InferenceServiceContainerName,
						VolumeMounts: []v1.VolumeMount{
							{
								Name:      "kserve-pvc-source",
								MountPath: constants.DefaultModelLocalMountPath,
								ReadOnly:  true,
								SubPath:   scenario.expectedSubPath,
							},
						},
					},
				},
				Volumes: []v1.Volume{
					{
						Name: "kserve-pvc-source",
						VolumeSource: v1.VolumeSource{
							PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{ClaimName: scenario.pvcName, ReadOnly: false},
						},
					},
				},
			},
		}

		podScenarios[name] = struct {
			original *v1.Pod
			expected *v1.Pod
		}{original: original, expected: expected}
	}
	for name, scenario := range podScenarios {
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: storageConfig,
			client: c,
		}

		if err := injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
