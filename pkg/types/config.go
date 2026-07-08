/*
Copyright 2025 The KServe Authors.

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

package types

import corev1 "k8s.io/api/core/v1"

type StorageInitializerConfig struct {
	Image                   string `json:"image"`
	CpuRequest              string `json:"cpuRequest"`
	CpuLimit                string `json:"cpuLimit"`
	CpuModelcar             string `json:"cpuModelcar"`
	MemoryRequest           string `json:"memoryRequest"`
	MemoryLimit             string `json:"memoryLimit"`
	CaBundleConfigMapName   string `json:"caBundleConfigMapName"`
	CaBundleVolumeMountPath string `json:"caBundleVolumeMountPath"`
	MemoryModelcar          string `json:"memoryModelcar"`
	EnableOciImageSource    bool   `json:"enableModelcar"`
	UidModelcar             *int64 `json:"uidModelcar"`
	// ModelVolumeSource overrides the default emptyDir volume used as the shared
	// model staging area between the storage-initializer init container and the
	// serving container. When nil, emptyDir is used. Any valid corev1.VolumeSource
	// that supports ReadWriteOnce (or better) may be specified — e.g. ephemeral,
	// persistentVolumeClaim, or hostPath.
	ModelVolumeSource *corev1.VolumeSource `json:"modelVolumeSource,omitempty"`
}
