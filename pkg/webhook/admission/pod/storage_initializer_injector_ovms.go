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
	"encoding/json"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/kserve/kserve/pkg/constants"
)

const (
	// OVMSVersioningDefaultImage is the default init container image used to reorganise
	// model files into the versioned directory structure that OVMS requires.
	// registry.access.redhat.com is the public Red Hat registry - no authentication needed.
	// The digest pin ensures deterministic behaviour across deployments.
	OVMSVersioningDefaultImage = "registry.access.redhat.com/ubi9/ubi-micro@sha256:2173487b3b72b1a7b11edc908e9bbf1726f9df46a4f78fd6d19a2bab0a701f38"
)

// OVMSVersioningConfig holds the configuration for the OVMS auto-versioning init container.
// It mirrors the agent/batcher pattern: values are read from the inferenceservice configmap
// under the "ovmsVersioning" key, and override the compiled-in defaults.
type OVMSVersioningConfig struct {
	Image         string `json:"image"`
	CpuRequest    string `json:"cpuRequest"`
	CpuLimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
}

// getOVMSVersioningConfig reads the OVMS versioning configuration from the inferenceservice
// configmap. When the key is absent the compiled-in defaults are returned unchanged.
func getOVMSVersioningConfig(configMap *corev1.ConfigMap) (*OVMSVersioningConfig, error) {
	cfg := &OVMSVersioningConfig{
		Image:         OVMSVersioningDefaultImage,
		CpuRequest:    "50m",
		CpuLimit:      "100m",
		MemoryRequest: "64Mi",
		MemoryLimit:   "128Mi",
	}

	if raw, ok := configMap.Data[constants.OVMSVersioningConfigMapKeyName]; ok {
		if err := json.Unmarshal([]byte(raw), cfg); err != nil {
			return cfg, fmt.Errorf("unable to unmarshal %q configmap key: %w",
				constants.OVMSVersioningConfigMapKeyName, err)
		}
	}

	for key, val := range map[string]string{
		"cpuRequest":    cfg.CpuRequest,
		"cpuLimit":      cfg.CpuLimit,
		"memoryRequest": cfg.MemoryRequest,
		"memoryLimit":   cfg.MemoryLimit,
	} {
		if _, err := resource.ParseQuantity(val); err != nil {
			return cfg, fmt.Errorf("failed to parse resource %q in %q: %w",
				key, constants.OVMSVersioningConfigMapKeyName, err)
		}
	}

	return cfg, nil
}

// injectOVMSAutoVersioning injects an init container that reorganises model files into
// the versioned directory structure that OpenVINO Model Server (OVMS) requires.
//
// OVMS expects models under a numbered subdirectory, e.g. /mnt/models/1/model.xml, but
// the storage initializer downloads files flat into /mnt/models. Without this step OVMS
// reports "No version found for model in path: /mnt/models" and refuses to start.
//
// The init container is only injected when the pod carries the
// storage.kserve.io/ovms-auto-versioning annotation set to a positive integer, which
// becomes the version directory name. If the versioned directory already exists the
// container exits immediately (idempotent).
func (mi *StorageInitializerInjector) injectOVMSAutoVersioning(pod *corev1.Pod) error {
	versionString, ok := pod.Annotations[constants.OVMSAutoVersioningAnnotationKey]
	if !ok {
		return nil
	}

	version, err := strconv.Atoi(versionString)
	if err != nil || version <= 0 {
		return fmt.Errorf("invalid value %q for annotation %s: must be a positive integer",
			versionString, constants.OVMSAutoVersioningAnnotationKey)
	}

	// Idempotency: skip if the container was already injected.
	for _, c := range pod.Spec.InitContainers {
		if c.Name == constants.OVMSVersioningContainerName {
			return nil
		}
	}

	cfg := mi.ovmsConfig

	pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
		Name:    constants.OVMSVersioningContainerName,
		Image:   cfg.Image,
		Command: []string{"/bin/sh"},
		Args: []string{
			"-c",
			fmt.Sprintf(`MODEL_DIR="%s"
VERSION="%s"
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
`, constants.DefaultModelLocalMountPath, versionString),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      constants.StorageInitializerVolumeName,
				MountPath: constants.DefaultModelLocalMountPath,
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cfg.CpuRequest),
				corev1.ResourceMemory: resource.MustParse(cfg.MemoryRequest),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cfg.CpuLimit),
				corev1.ResourceMemory: resource.MustParse(cfg.MemoryLimit),
			},
		},
	})

	return nil
}
