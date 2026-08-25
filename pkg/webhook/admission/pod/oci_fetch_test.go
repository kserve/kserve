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

package pod

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	kserveTypes "github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

// newFetchTestConfig returns a StorageInitializerConfig with the resource fields that
// CreateInitContainerWithConfig requires (resource.MustParse panics on empty strings).
func newFetchTestConfig() *kserveTypes.StorageInitializerConfig {
	return &kserveTypes.StorageInitializerConfig{
		CpuRequest:    StorageInitializerDefaultCPURequest,
		CpuLimit:      StorageInitializerDefaultCPULimit,
		MemoryRequest: StorageInitializerDefaultMemoryRequest,
		MemoryLimit:   StorageInitializerDefaultMemoryLimit,
	}
}

func findVolume(volumes []corev1.Volume, name string) *corev1.Volume {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}

func findVolumeMount(mounts []corev1.VolumeMount, name string) *corev1.VolumeMount {
	for i := range mounts {
		if mounts[i].Name == name {
			return &mounts[i]
		}
	}
	return nil
}

func TestConfigureOciFetchToContainer(t *testing.T) {
	t.Run("no imagePullSecrets injects init container without docker config", func(t *testing.T) {
		podSpec := corev1.PodSpec{
			Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
		}
		err := ConfigureOciFetchToContainer(
			constants.OciURIPrefix+"registry.io/mymodel:v1",
			&podSpec,
			constants.InferenceServiceContainerName,
			constants.DefaultModelLocalMountPath,
			newFetchTestConfig(),
			"default",
		)
		require.NoError(t, err)

		init := getStorageInitializerInitContainer(&podSpec)
		require.NotNil(t, init, "storage-initializer init container should be injected")
		assert.Equal(t, []string{constants.OciURIPrefix + "registry.io/mymodel:v1", constants.DefaultModelLocalMountPath}, init.Args)

		assert.Nil(t, findVolume(podSpec.Volumes, ociFetchDockerConfigVolumeName), "no docker config volume without imagePullSecrets")
		assert.Empty(t, init.Env, "no env vars added without imagePullSecrets or CA bundle")

		// Shared model emptyDir mounted on both the init and the user container.
		volName := utils.GetVolumeNameFromPath(constants.DefaultModelLocalMountPath)
		assert.NotNil(t, findVolumeMount(init.VolumeMounts, volName), "init container should mount the model volume")
		user := utils.GetContainerWithName(&podSpec, constants.InferenceServiceContainerName)
		require.NotNil(t, user)
		assert.NotNil(t, findVolumeMount(user.VolumeMounts, volName), "user container should mount the model volume")
		modelVol := findVolume(podSpec.Volumes, volName)
		require.NotNil(t, modelVol)
		assert.NotNil(t, modelVol.EmptyDir, "model volume should be an emptyDir")
	})

	t.Run("single imagePullSecret mounts docker config", func(t *testing.T) {
		podSpec := corev1.PodSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "my-reg-cred"}},
			Containers:       []corev1.Container{{Name: constants.InferenceServiceContainerName}},
		}
		err := ConfigureOciFetchToContainer(
			constants.OciURIPrefix+"registry.io/mymodel:v1",
			&podSpec,
			constants.InferenceServiceContainerName,
			constants.DefaultModelLocalMountPath,
			newFetchTestConfig(),
			"default",
		)
		require.NoError(t, err)

		init := getStorageInitializerInitContainer(&podSpec)
		require.NotNil(t, init)

		vol := findVolume(podSpec.Volumes, ociFetchDockerConfigVolumeName)
		require.NotNil(t, vol, "docker config volume should be present")
		require.NotNil(t, vol.Secret)
		assert.Equal(t, "my-reg-cred", vol.Secret.SecretName)
		require.Len(t, vol.Secret.Items, 1)
		assert.Equal(t, corev1.DockerConfigJsonKey, vol.Secret.Items[0].Key)
		assert.Equal(t, "config.json", vol.Secret.Items[0].Path)

		mount := findVolumeMount(init.VolumeMounts, ociFetchDockerConfigVolumeName)
		require.NotNil(t, mount, "init container should mount the docker config volume")
		assert.Equal(t, ociFetchDockerConfigDir, mount.MountPath)
		assert.True(t, mount.ReadOnly)

		env := findEnv(init.Env, ociFetchDockerConfigPathEnvVar)
		require.NotNil(t, env, "KSERVE_OCI_DOCKER_CONFIG should point the handler at the config")
		assert.Equal(t, ociFetchDockerConfigDir+"/config.json", env.Value)
	})

	t.Run("multiple imagePullSecrets use the first", func(t *testing.T) {
		podSpec := corev1.PodSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{
				{Name: "cred-a"}, {Name: "cred-b"}, {Name: "cred-c"},
			},
			Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
		}
		err := ConfigureOciFetchToContainer(
			constants.OciURIPrefix+"registry.io/mymodel:v1",
			&podSpec,
			constants.InferenceServiceContainerName,
			constants.DefaultModelLocalMountPath,
			newFetchTestConfig(),
			"default",
		)
		require.NoError(t, err)

		vol := findVolume(podSpec.Volumes, ociFetchDockerConfigVolumeName)
		require.NotNil(t, vol)
		require.NotNil(t, vol.Secret)
		assert.Equal(t, "cred-a", vol.Secret.SecretName, "first imagePullSecret should be used")
	})

	t.Run("CA bundle mounted in kserve namespace keeps configured configmap name", func(t *testing.T) {
		cfg := newFetchTestConfig()
		cfg.CaBundleConfigMapName = "my-ca-bundle"
		podSpec := corev1.PodSpec{
			Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
		}
		err := ConfigureOciFetchToContainer(
			constants.OciURIPrefix+"registry.io/mymodel:v1",
			&podSpec,
			constants.InferenceServiceContainerName,
			constants.DefaultModelLocalMountPath,
			cfg,
			constants.KServeNamespace,
		)
		require.NoError(t, err)

		init := getStorageInitializerInitContainer(&podSpec)
		require.NotNil(t, init)
		vol := findVolume(podSpec.Volumes, CaBundleVolumeName)
		require.NotNil(t, vol, "CA bundle volume should be present")
		require.NotNil(t, vol.ConfigMap)
		assert.Equal(t, "my-ca-bundle", vol.ConfigMap.Name)
		assert.NotNil(t, findVolumeMount(init.VolumeMounts, CaBundleVolumeName))
		assert.NotNil(t, findEnv(init.Env, constants.CaBundleConfigMapNameEnvVarKey))
	})

	t.Run("CA bundle outside kserve namespace uses the global configmap", func(t *testing.T) {
		cfg := newFetchTestConfig()
		cfg.CaBundleConfigMapName = "my-ca-bundle"
		podSpec := corev1.PodSpec{
			Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
		}
		err := ConfigureOciFetchToContainer(
			constants.OciURIPrefix+"registry.io/mymodel:v1",
			&podSpec,
			constants.InferenceServiceContainerName,
			constants.DefaultModelLocalMountPath,
			cfg,
			"user-namespace",
		)
		require.NoError(t, err)

		vol := findVolume(podSpec.Volumes, CaBundleVolumeName)
		require.NotNil(t, vol)
		require.NotNil(t, vol.ConfigMap)
		assert.Equal(t, constants.DefaultGlobalCaBundleConfigMapName, vol.ConfigMap.Name)
	})

	t.Run("repeated call for transformer does not duplicate init args", func(t *testing.T) {
		podSpec := corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: constants.InferenceServiceContainerName},
				{Name: constants.TransformerContainerName},
			},
		}
		uri := constants.OciURIPrefix + "registry.io/mymodel:v1"
		require.NoError(t, ConfigureOciFetchToContainer(uri, &podSpec, constants.InferenceServiceContainerName, constants.DefaultModelLocalMountPath, newFetchTestConfig(), "default"))
		require.NoError(t, ConfigureOciFetchToContainer(uri, &podSpec, constants.TransformerContainerName, constants.DefaultModelLocalMountPath, newFetchTestConfig(), "default"))

		inits := 0
		for _, c := range podSpec.InitContainers {
			if c.Name == constants.StorageInitializerContainerName {
				inits++
			}
		}
		assert.Equal(t, 1, inits, "only one storage-initializer init container expected")
		init := getStorageInitializerInitContainer(&podSpec)
		require.NotNil(t, init)
		assert.Equal(t, []string{uri, constants.DefaultModelLocalMountPath}, init.Args, "args should not be duplicated for the transformer")

		// Both containers should mount the shared model volume.
		volName := utils.GetVolumeNameFromPath(constants.DefaultModelLocalMountPath)
		transformer := utils.GetContainerWithName(&podSpec, constants.TransformerContainerName)
		require.NotNil(t, transformer)
		assert.NotNil(t, findVolumeMount(transformer.VolumeMounts, volName))
	})

	t.Run("multiple fetch URIs append distinct arg pairs to one init container", func(t *testing.T) {
		podSpec := corev1.PodSpec{
			Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
		}
		uri1 := constants.OciURIPrefix + "registry.io/base:v1"
		uri2 := constants.OciURIPrefix + "registry.io/adapter:v1"
		require.NoError(t, ConfigureOciFetchToContainer(uri1, &podSpec, constants.InferenceServiceContainerName, "/mnt/models/base", newFetchTestConfig(), "default"))
		require.NoError(t, ConfigureOciFetchToContainer(uri2, &podSpec, constants.InferenceServiceContainerName, "/mnt/models/adapter", newFetchTestConfig(), "default"))

		init := getStorageInitializerInitContainer(&podSpec)
		require.NotNil(t, init)
		assert.Equal(t, []string{uri1, "/mnt/models/base", uri2, "/mnt/models/adapter"}, init.Args)
	})

	t.Run("missing target container returns error", func(t *testing.T) {
		podSpec := corev1.PodSpec{
			Containers: []corev1.Container{{Name: "some-other-container"}},
		}
		err := ConfigureOciFetchToContainer(
			constants.OciURIPrefix+"registry.io/mymodel:v1",
			&podSpec,
			constants.InferenceServiceContainerName,
			constants.DefaultModelLocalMountPath,
			newFetchTestConfig(),
			"default",
		)
		require.Error(t, err)
	})
}

// TestConfigureOciFetchDockerConfig exercises the mountImagePullSecretsAsDockerConfig
// helper directly (TEST-C).
func TestConfigureOciFetchDockerConfig(t *testing.T) {
	t.Run("zero secrets is a no-op", func(t *testing.T) {
		container := &corev1.Container{Name: constants.StorageInitializerContainerName}
		var volumes []corev1.Volume
		require.NoError(t, mountImagePullSecretsAsDockerConfig(nil, container, &volumes))
		assert.Empty(t, volumes)
		assert.Empty(t, container.VolumeMounts)
		assert.Empty(t, container.Env)
	})

	t.Run("single secret adds volume, mount and config-path env", func(t *testing.T) {
		container := &corev1.Container{Name: constants.StorageInitializerContainerName}
		var volumes []corev1.Volume
		secrets := []corev1.LocalObjectReference{{Name: "reg-cred"}}
		require.NoError(t, mountImagePullSecretsAsDockerConfig(secrets, container, &volumes))

		require.Len(t, volumes, 1)
		require.NotNil(t, volumes[0].Secret)
		assert.Equal(t, "reg-cred", volumes[0].Secret.SecretName)
		require.Len(t, container.VolumeMounts, 1)
		assert.Equal(t, ociFetchDockerConfigDir, container.VolumeMounts[0].MountPath)
		require.Len(t, container.Env, 1)
		assert.Equal(t, ociFetchDockerConfigPathEnvVar, container.Env[0].Name)
		assert.Equal(t, ociFetchDockerConfigDir+"/config.json", container.Env[0].Value)
	})

	t.Run("multiple secrets use the first", func(t *testing.T) {
		container := &corev1.Container{Name: constants.StorageInitializerContainerName}
		var volumes []corev1.Volume
		secrets := []corev1.LocalObjectReference{{Name: "first"}, {Name: "second"}}
		require.NoError(t, mountImagePullSecretsAsDockerConfig(secrets, container, &volumes))
		require.Len(t, volumes, 1)
		require.NotNil(t, volumes[0].Secret)
		assert.Equal(t, "first", volumes[0].Secret.SecretName)
	})
}

// TestConfigureOciFetchViaCommonStorageInitialization verifies oci+fetch:// dispatch
// through the storageUris (non-legacy) path.
func TestConfigureOciFetchViaCommonStorageInitialization(t *testing.T) {
	t.Run("oci+fetch:// storageUri injects fetch init container", func(t *testing.T) {
		podSpec := corev1.PodSpec{
			Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
		}
		params := &StorageInitializerParams{
			Namespace: "default",
			StorageURIs: []v1beta1.StorageUri{
				{Uri: constants.OciFetchURIPrefix + "registry.io/mymodel:v1", MountPath: constants.DefaultModelLocalMountPath},
			},
			PodSpec:         &podSpec,
			Config:          newFetchTestConfig(),
			IsvcAnnotations: map[string]string{},
			IsLegacyURI:     false,
		}
		require.NoError(t, CommonStorageInitialization(t.Context(), params))

		init := getStorageInitializerInitContainer(&podSpec)
		require.NotNil(t, init, "fetch init container should be injected")
		// The Python initializer must receive the normalized oci:// URI, not oci+fetch://.
		assert.Equal(t, []string{constants.OciURIPrefix + "registry.io/mymodel:v1", constants.DefaultModelLocalMountPath}, init.Args)
	})

	t.Run("oci+fetch:// with imagePullSecret mounts docker config", func(t *testing.T) {
		podSpec := corev1.PodSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "reg-cred"}},
			Containers:       []corev1.Container{{Name: constants.InferenceServiceContainerName}},
		}
		params := &StorageInitializerParams{
			Namespace: "default",
			StorageURIs: []v1beta1.StorageUri{
				{Uri: constants.OciFetchURIPrefix + "registry.io/mymodel:v1", MountPath: constants.DefaultModelLocalMountPath},
			},
			PodSpec:         &podSpec,
			Config:          newFetchTestConfig(),
			IsvcAnnotations: map[string]string{},
			IsLegacyURI:     false,
		}
		require.NoError(t, CommonStorageInitialization(t.Context(), params))
		assert.NotNil(t, findVolume(podSpec.Volumes, ociFetchDockerConfigVolumeName))
	})
}

// TestConfigureOciFetchViaInjectModelcar verifies oci+fetch:// dispatch through the
// legacy annotation path (InjectModelcar).
func TestConfigureOciFetchViaInjectModelcar(t *testing.T) {
	t.Run("oci+fetch:// annotation injects fetch init container", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Annotations: map[string]string{
					constants.StorageInitializerSourceUriInternalAnnotationKey: constants.OciFetchURIPrefix + "registry.io/mymodel:v1",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
			},
		}
		mi := &StorageInitializerInjector{config: newFetchTestConfig()}
		require.NoError(t, mi.InjectModelcar(pod))

		init := getStorageInitializerInitContainer(&pod.Spec)
		require.NotNil(t, init, "fetch init container should be injected for legacy annotation path")
		assert.Equal(t, []string{constants.OciURIPrefix + "registry.io/mymodel:v1", constants.DefaultModelLocalMountPath}, init.Args)
	})

	t.Run("bare oci:// with OciModelMode=fetch injects fetch init container", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Annotations: map[string]string{
					constants.StorageInitializerSourceUriInternalAnnotationKey: constants.OciURIPrefix + "registry.io/mymodel:v1",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: constants.InferenceServiceContainerName}},
			},
		}
		cfg := newFetchTestConfig()
		cfg.OciModelMode = kserveTypes.OciModelModeFetch
		mi := &StorageInitializerInjector{config: cfg}
		require.NoError(t, mi.InjectModelcar(pod))

		init := getStorageInitializerInitContainer(&pod.Spec)
		require.NotNil(t, init)
		assert.Equal(t, []string{constants.OciURIPrefix + "registry.io/mymodel:v1", constants.DefaultModelLocalMountPath}, init.Args)
	})
}
