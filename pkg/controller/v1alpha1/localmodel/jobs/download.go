/*
Copyright 2026 The KServe Authors.

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

// Package jobs holds download-Job building blocks shared by the node-agent (per-node hostPath
// fan-out) and the manager (shared-PVC import) paths: download-container resolution and
// credential construction. Keeping these in one leaf package avoids duplicating the container
// and credential wiring across the two reconcilers.
package jobs

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/credentials"
	pkgtypes "github.com/kserve/kserve/pkg/types"
)

const (
	// DownloadContainerName is the name of the model download container.
	DownloadContainerName = "kserve-localmodel-download"
	// PvcSourceMountName is the volume name for the destination PVC mount.
	PvcSourceMountName = "kserve-pvc-source"
	// MountPath is the path the model is written to inside the download container.
	MountPath = "/mnt/models"
	// DefaultJobImage is used when no StorageInitializerConfig image is set.
	DefaultJobImage = "kserve/storage-initializer:latest"
)

// ResolveDownloadContainer returns the download container for a source URI. It prefers a matching
// enabled ClusterStorageContainer (workload type LocalModelDownloadJob) for backward
// compatibility, and falls back to the StorageInitializerConfig image otherwise.
func ResolveDownloadContainer(ctx context.Context, cl client.Client, config *pkgtypes.StorageInitializerConfig, sourceModelUri string) (*corev1.Container, error) {
	container, err := containerFromStorageContainers(ctx, cl, sourceModelUri)
	if err != nil {
		return nil, err
	}
	if container == nil {
		container = ContainerFromConfig(config)
	}
	return container, nil
}

// ContainerFromConfig returns the fallback download container built from the
// StorageInitializerConfig image, or the default image when unset.
func ContainerFromConfig(config *pkgtypes.StorageInitializerConfig) *corev1.Container {
	image := DefaultJobImage
	if config != nil && config.Image != "" {
		image = config.Image
	}
	return &corev1.Container{
		Name:                     DownloadContainerName,
		Image:                    image,
		TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
	}
}

// containerFromStorageContainers returns the container spec of the first enabled
// ClusterStorageContainer that supports the source URI for local-model downloads, or nil.
func containerFromStorageContainers(ctx context.Context, cl client.Client, sourceModelUri string) (*corev1.Container, error) {
	storageContainers := &v1alpha1.ClusterStorageContainerList{}
	if err := cl.List(ctx, storageContainers); err != nil {
		return nil, err
	}
	for i := range storageContainers.Items {
		sc := &storageContainers.Items[i]
		if sc.IsDisabled() {
			continue
		}
		if sc.Spec.WorkloadType != v1alpha1.LocalModelDownloadJob {
			continue
		}
		supported, err := sc.Spec.IsStorageUriSupported(sourceModelUri)
		if err != nil {
			return nil, fmt.Errorf("error checking storage container %s: %w", sc.Name, err)
		}
		if supported {
			return sc.Spec.Container.DeepCopy(), nil
		}
	}
	return nil, nil
}

// InjectCredentials injects storage credentials into the download container. Storage-spec
// credentials take precedence over service-account credentials. Secrets are resolved in the
// given namespace (the cache/Job namespace). It is a no-op when cb is nil.
func InjectCredentials(
	ctx context.Context,
	cb *credentials.CredentialBuilder,
	log logr.Logger,
	container *corev1.Container,
	volumes *[]corev1.Volume,
	serviceAccountName string,
	storage *v1alpha1.LocalModelStorageSpec,
	namespace string,
) error {
	if cb == nil {
		log.Info("CredentialBuilder not initialized, skipping credential injection")
		return nil
	}

	if storage != nil && storage.StorageKey != nil {
		var params map[string]string
		if storage.Parameters != nil {
			params = *storage.Parameters
		}
		log.Info("Injecting storage spec credentials", "storageKey", *storage.StorageKey)
		return cb.CreateStorageSpecSecretEnvs(ctx, namespace, nil, *storage.StorageKey, params, container)
	}

	if serviceAccountName == "" {
		serviceAccountName = "default"
	}
	log.Info("Injecting service account credentials", "serviceAccountName", serviceAccountName)
	return cb.CreateSecretVolumeAndEnv(ctx, namespace, nil, serviceAccountName, container, volumes)
}
