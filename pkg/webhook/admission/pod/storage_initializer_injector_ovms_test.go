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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/kmp"

	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
)

func newOVMSInjector(t *testing.T) *StorageInitializerInjector {
	t.Helper()
	ovmsConfig, err := getOVMSVersioningConfig(&corev1.ConfigMap{Data: map[string]string{}})
	if err != nil {
		t.Fatalf("failed to build default OVMS config: %v", err)
	}
	return &StorageInitializerInjector{
		credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &corev1.ConfigMap{
			Data: map[string]string{},
		}),
		config:     storageInitializerConfig,
		ovmsConfig: ovmsConfig,
		client:     c,
	}
}

func TestOVMSAutoVersioning(t *testing.T) {
	ovmsContainer := corev1.Container{
		Name:    constants.OVMSVersioningContainerName,
		Image:   OVMSVersioningDefaultImage,
		Command: []string{"/bin/sh"},
		Args: []string{
			"-c",
			`MODEL_DIR="/mnt/models"
VERSION="1"
VERSIONED_DIR="${MODEL_DIR}/${VERSION}"

if [ ! -d "${MODEL_DIR}" ] || [ -z "$(ls -A "${MODEL_DIR}" 2>/dev/null)" ]; then
  exit 0
fi

if [ -d "${VERSIONED_DIR}" ]; then
  exit 0
fi

mkdir -p "${VERSIONED_DIR}"

# Move regular files/dirs and hidden entries (dotfiles) - plain glob misses the latter.
for f in "${MODEL_DIR}"/* "${MODEL_DIR}"/.[!.]* "${MODEL_DIR}"/..?*; do
  [ -e "$f" ] && mv "$f" "${VERSIONED_DIR}/"
done
`,
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      constants.StorageInitializerVolumeName,
				MountPath: constants.DefaultModelLocalMountPath,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
	}

	scenarios := map[string]struct {
		original *corev1.Pod
		expected *corev1.Pod
	}{
		"annotation absent - no versioning container injected": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo/model.xml",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo/model.xml",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      constants.StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:  constants.StorageInitializerContainerName,
							Image: constants.StorageInitializerContainerImage + ":" + constants.StorageInitializerContainerImageVersion,
							Args:  []string{"gs://foo/model.xml", constants.DefaultModelLocalMountPath},
							Env: []corev1.EnvVar{
								{Name: "HF_HUB_ENABLE_HF_TRANSFER", Value: "1"},
								{Name: "HF_XET_HIGH_PERFORMANCE", Value: "1"},
								{Name: "HF_XET_NUM_CONCURRENT_RANGE_GETS", Value: "8"},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      constants.StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: constants.StorageInitializerVolumeName,
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
						},
					},
				},
			},
		},
		"annotation present - versioning container appended after storage initializer": {
			original: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo/model.xml",
						constants.OVMSAutoVersioningAnnotationKey:                  "1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
				},
			},
			expected: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo/model.xml",
						constants.OVMSAutoVersioningAnnotationKey:                  "1",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: constants.InferenceServiceContainerName,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      constants.StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
									ReadOnly:  true,
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:  constants.StorageInitializerContainerName,
							Image: constants.StorageInitializerContainerImage + ":" + constants.StorageInitializerContainerImageVersion,
							Args:  []string{"gs://foo/model.xml", constants.DefaultModelLocalMountPath},
							Env: []corev1.EnvVar{
								{Name: "HF_HUB_ENABLE_HF_TRANSFER", Value: "1"},
								{Name: "HF_XET_HIGH_PERFORMANCE", Value: "1"},
								{Name: "HF_XET_NUM_CONCURRENT_RANGE_GETS", Value: "8"},
							},
							Resources:                resourceRequirement,
							TerminationMessagePolicy: "FallbackToLogsOnError",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      constants.StorageInitializerVolumeName,
									MountPath: constants.DefaultModelLocalMountPath,
								},
							},
						},
						ovmsContainer,
					},
					Volumes: []corev1.Volume{
						{
							Name: constants.StorageInitializerVolumeName,
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
						},
					},
				},
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			injector := newOVMSInjector(t)
			if err := injector.InjectStorageInitializer(t.Context(), scenario.original); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff, _ := kmp.SafeDiff(scenario.expected.Spec, scenario.original.Spec); diff != "" {
				t.Errorf("unexpected pod spec (-want +got):\n%v", diff)
			}
		})
	}
}

func TestOVMSAutoVersioningInvalidAnnotationValues(t *testing.T) {
	cases := []struct {
		name        string
		value       string
		expectError bool
	}{
		{"not a number", "invalid", true},
		{"zero", "0", true},
		{"negative", "-1", true},
		{"version 1", "1", false},
		{"version 10", "10", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo/model.xml",
						constants.OVMSAutoVersioningAnnotationKey:                  tc.value,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
				},
			}

			err := newOVMSInjector(t).InjectStorageInitializer(t.Context(), pod)
			if tc.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetOVMSVersioningConfig(t *testing.T) {
	t.Run("empty configmap returns defaults", func(t *testing.T) {
		cfg, err := getOVMSVersioningConfig(&corev1.ConfigMap{Data: map[string]string{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Image != OVMSVersioningDefaultImage {
			t.Errorf("expected default image %q, got %q", OVMSVersioningDefaultImage, cfg.Image)
		}
		if cfg.CpuRequest != "50m" {
			t.Errorf("expected cpuRequest 50m, got %q", cfg.CpuRequest)
		}
		if cfg.MemoryRequest != "64Mi" {
			t.Errorf("expected memoryRequest 64Mi, got %q", cfg.MemoryRequest)
		}
	})

	t.Run("custom values override defaults", func(t *testing.T) {
		const customImage = "my-registry.example.com/ubi9/ubi-micro:custom"
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				constants.OVMSVersioningConfigMapKeyName: `{
					"image":         "` + customImage + `",
					"cpuRequest":    "200m",
					"cpuLimit":      "500m",
					"memoryRequest": "128Mi",
					"memoryLimit":   "256Mi"
				}`,
			},
		}
		cfg, err := getOVMSVersioningConfig(cm)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Image != customImage {
			t.Errorf("expected image %q, got %q", customImage, cfg.Image)
		}
		if cfg.CpuRequest != "200m" {
			t.Errorf("expected cpuRequest 200m, got %q", cfg.CpuRequest)
		}
		if cfg.MemoryLimit != "256Mi" {
			t.Errorf("expected memoryLimit 256Mi, got %q", cfg.MemoryLimit)
		}
	})

	t.Run("custom image is used in injected container", func(t *testing.T) {
		const customImage = "my-registry.example.com/ubi9/ubi-micro:custom"
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				constants.OVMSVersioningConfigMapKeyName: `{"image":"` + customImage + `","cpuRequest":"50m","cpuLimit":"100m","memoryRequest":"64Mi","memoryLimit":"128Mi"}`,
			},
		}
		cfg, err := getOVMSVersioningConfig(cm)
		if err != nil {
			t.Fatalf("unexpected error building config: %v", err)
		}
		injector := &StorageInitializerInjector{
			credentialBuilder: credentials.NewCredentialBuilder(c, clientset, &corev1.ConfigMap{Data: map[string]string{}}),
			config:            storageInitializerConfig,
			ovmsConfig:        cfg,
			client:            c,
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo/model.xml",
					constants.OVMSAutoVersioningAnnotationKey:                  "1",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
			},
		}
		if err := injector.InjectStorageInitializer(t.Context(), pod); err != nil {
			t.Fatalf("injection failed: %v", err)
		}
		var got string
		for _, c := range pod.Spec.InitContainers {
			if c.Name == constants.OVMSVersioningContainerName {
				got = c.Image
			}
		}
		if got != customImage {
			t.Errorf("expected injected image %q, got %q", customImage, got)
		}
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				constants.OVMSVersioningConfigMapKeyName: `{not valid json`,
			},
		}
		_, err := getOVMSVersioningConfig(cm)
		if err == nil {
			t.Error("expected error for malformed JSON, got nil")
		}
	})

	t.Run("invalid resource quantity returns error", func(t *testing.T) {
		cm := &corev1.ConfigMap{
			Data: map[string]string{
				constants.OVMSVersioningConfigMapKeyName: `{"image":"img","cpuRequest":"not-a-quantity","cpuLimit":"100m","memoryRequest":"64Mi","memoryLimit":"128Mi"}`,
			},
		}
		_, err := getOVMSVersioningConfig(cm)
		if err == nil {
			t.Error("expected error for invalid resource quantity, got nil")
		}
	})
}

func TestOVMSAutoVersioningIdempotent(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				constants.StorageInitializerSourceUriInternalAnnotationKey: "gs://foo/model.xml",
				constants.OVMSAutoVersioningAnnotationKey:                  "1",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
		},
	}

	injector := newOVMSInjector(t)

	if err := injector.InjectStorageInitializer(t.Context(), pod); err != nil {
		t.Fatalf("first injection failed: %v", err)
	}
	countAfterFirst := len(pod.Spec.InitContainers)

	if err := injector.InjectStorageInitializer(t.Context(), pod); err != nil {
		t.Fatalf("second injection failed: %v", err)
	}

	if len(pod.Spec.InitContainers) != countAfterFirst {
		t.Errorf("expected %d init containers after second injection, got %d",
			countAfterFirst, len(pod.Spec.InitContainers))
	}

	var ovmsCount int
	for _, c := range pod.Spec.InitContainers {
		if c.Name == constants.OVMSVersioningContainerName {
			ovmsCount++
		}
	}
	if ovmsCount != 1 {
		t.Errorf("expected exactly 1 OVMS versioning container, got %d", ovmsCount)
	}
}
