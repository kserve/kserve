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
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"knative.dev/pkg/kmp"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/credentials/gcs"
	"github.com/kserve/kserve/pkg/credentials/s3"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	StorageInitializerDefaultCPURequest                 = "100m"
	StorageInitializerDefaultCPULimit                   = "1"
	StorageInitializerDefaultMemoryRequest              = "200Mi"
	StorageInitializerDefaultMemoryLimit                = "1Gi"
	StorageInitializerDefaultEnableDirectPvcVolumeMount = false
)

var (
	storageInitializerConfig = &StorageInitializerConfig{
		CpuRequest:                 StorageInitializerDefaultCPURequest,
		CpuLimit:                   StorageInitializerDefaultCPULimit,
		MemoryRequest:              StorageInitializerDefaultMemoryRequest,
		MemoryLimit:                StorageInitializerDefaultMemoryLimit,
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
			credentialBuilder: credentials.NewCredentialBuilder(c, &v1.ConfigMap{
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
			credentialBuilder: credentials.NewCredentialBuilder(c, &v1.ConfigMap{
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
			credentialBuilder: credentials.NewCredentialBuilder(c, &v1.ConfigMap{
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

	builder := credentials.NewCredentialBuilder(c, configMap)
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
			credentialBuilder: credentials.NewCredentialBuilder(c, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: &StorageInitializerConfig{
				Image:         "kserve/storage-initializer@sha256:xxx",
				CpuRequest:    StorageInitializerDefaultCPURequest,
				CpuLimit:      StorageInitializerDefaultCPULimit,
				MemoryRequest: StorageInitializerDefaultMemoryRequest,
				MemoryLimit:   StorageInitializerDefaultMemoryLimit,
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
						"MemoryLimit":   		 "1Gi"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&StorageInitializerConfig{
					Image:         "gcr.io/kserve/storage-initializer:latest",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "200Mi",
					MemoryLimit:   "1Gi",
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
						"MemoryLimit":   		 "1Gi"
					}`,
				},
				BinaryData: map[string][]byte{},
			},
			matchers: []types.GomegaMatcher{
				gomega.Equal(&StorageInitializerConfig{
					Image:         "gcr.io/kserve/storage-initializer:latest",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "200MC",
					MemoryLimit:   "1Gi",
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
	}

	for name, scenario := range scenarios {
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBuilder(c, &v1.ConfigMap{
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
							Name: constants.TransformerContainerName,
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
						{
							Name: constants.TransformerContainerName,
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
							Name: constants.TransformerContainerName,
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
									MountPath: "/mnt/models",
									SubPath:   "some/path/on/pvc",
									ReadOnly:  true,
								},
							},
						},
						{
							Name: constants.TransformerContainerName,
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
			credentialBuilder: credentials.NewCredentialBuilder(c, &v1.ConfigMap{
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
	customSpec := v1alpha1.StorageContainerSpec{
		StorageContainer: v1.Container{
			Image: "kserve/custom:latest",
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceMemory: resource.MustParse("200Mi"),
				},
			},
		},
		SupportedPrefixes: []string{"custom://"},
	}
	s3AzureSpec := v1alpha1.StorageContainerSpec{
		StorageContainer: v1.Container{
			Image: "kserve/storage-initializer:latest",
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceMemory: resource.MustParse("200Mi"),
				},
			},
		},
		SupportedPrefixes: []string{"s3://"},
		SupportedRegexes:  []string{"https://(.+?).blob.core.windows.net/(.+)"},
	}
	storageContainerSpecs := &v1alpha1.ClusterStorageContainerList{
		Items: []v1alpha1.ClusterStorageContainer{{ObjectMeta: metav1.ObjectMeta{
			Name: "custom",
		}, Spec: customSpec}, {ObjectMeta: metav1.ObjectMeta{
			Name: "s3",
		}, Spec: s3AzureSpec}},
	}

	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	mockClient := fake.NewClientBuilder().WithLists(storageContainerSpecs).WithScheme(s).Build()
	scenarios := map[string]struct {
		storageUri   string
		expectedSpec *v1alpha1.StorageContainerSpec
	}{
		"s3": {
			storageUri:   "s3://foo",
			expectedSpec: &s3AzureSpec,
		},
		"custom": {
			storageUri:   "custom://foo",
			expectedSpec: &customSpec,
		},
		"nonExistent": {
			storageUri:   "abc://",
			expectedSpec: nil,
		},
	}
	for name, scenario := range scenarios {
		var config *v1alpha1.StorageContainerSpec

		if config, err = getStorageContainerConfigForUri(scenario.storageUri, mockClient); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		g.Expect(config).To(gomega.Equal(scenario.expectedSpec))
	}
}

func TestStorageContainerCRDInjecrtion(t *testing.T) {
	customSpec := v1alpha1.StorageContainerSpec{
		StorageContainer: v1.Container{
			Image: "kserve/custom-storage-initializer:latest",
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceMemory: resource.MustParse("200Mi"),
				},
			},
		},
		SupportedPrefixes: []string{"custom://"},
	}
	s3AzureSpec := v1alpha1.StorageContainerSpec{
		StorageContainer: v1.Container{
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
		SupportedPrefixes: []string{"s3://"},
		SupportedRegexes:  []string{"https://(.+?).blob.core.windows.net/(.+)"},
	}
	storageContainerSpecs := &v1alpha1.ClusterStorageContainerList{
		Items: []v1alpha1.ClusterStorageContainer{{ObjectMeta: metav1.ObjectMeta{
			Name: "custom",
		}, Spec: customSpec}, {ObjectMeta: metav1.ObjectMeta{
			Name: "s3",
		}, Spec: s3AzureSpec}},
	}

	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	mockClient := fake.NewClientBuilder().WithLists(storageContainerSpecs).WithScheme(s).Build()
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
		"configMap": {
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
							Env: nil, // Env is not supported yet if no matching CR is found
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
			credentialBuilder: credentials.NewCredentialBuilder(mockClient, &v1.ConfigMap{
				Data: map[string]string{},
			}),
			config: storageInitializerConfig,
			client: mockClient,
		}

		if err = injector.InjectStorageInitializer(scenario.original); err != nil {
			t.Errorf("Test %q unexpected result: %s", name, err)
		}
		if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
			t.Errorf("Test %q unexpected result (-want +got): %v", name, diff)
		}
	}
}
