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

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

const (
	downloaderSrcURIAnnotation    = "downloaderSrcUri"
	downloaderMountPathAnnotation = "downloaderMountPath"
	defaultMountName              = "kfserving-download-location"
	defaultMountPath              = "/mnt"
	userContainerName             = "user-container"
	downloadContainerImage        = "kcorer/downloader"
)

// InjectDownloader injects an init container to download data and mounts to the user container
func InjectDownloader(deployment *appsv1.Deployment) error {

	var srcURI, mountPath string

	annotations := deployment.Spec.Template.ObjectMeta.Annotations
	podSpec := &deployment.Spec.Template.Spec

	if _, ok := annotations[downloaderSrcURIAnnotation]; ok {
		srcURI = annotations[downloaderSrcURIAnnotation]
	} else {
		return nil
	}

	if _, ok := annotations[downloaderMountPathAnnotation]; ok {
		mountPath = annotations[downloaderMountPathAnnotation]
	} else {
		mountPath = defaultMountPath
	}

	userContainerIndex := -1
	for idx, container := range podSpec.Containers {
		if strings.Compare(container.Name, userContainerName) == 0 {
			userContainerIndex = idx
			break
		}
	}

	if userContainerIndex < 0 {
		return nil
	}

	initMount := buildVolumeMount(mountPath, false)
	initContianer := buildInitContainer(srcURI, mountPath, initMount)
	podSpec.Containers = append(podSpec.Containers, initContianer)

	userMount := buildVolumeMount(mountPath, true)
	podSpec.Containers[userContainerIndex].VolumeMounts = append(podSpec.Containers[userContainerIndex].VolumeMounts, userMount)

	return nil
}

func buildInitContainer(srcURI string, mountPath string, volumeMount v1.VolumeMount) v1.Container {
	initContianer := v1.Container{
		Image: downloadContainerImage,
		Args: []string{
			srcURI,
			mountPath,
		},
		VolumeMounts: []v1.VolumeMount{
			volumeMount,
		},
	}
	return initContianer
}

func buildVolumeMount(mountPath string, readOnly bool) v1.VolumeMount {
	volumeMount := v1.VolumeMount{
		Name:      defaultMountName,
		MountPath: mountPath,
		ReadOnly:  readOnly,
	}
	return volumeMount
}
