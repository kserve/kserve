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

package llmisvc

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"knative.dev/pkg/apis"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/utils"
)

func TestAttachModelScopeArtifacts(t *testing.T) {
	t.Parallel()
	const uri = "modelscope://Qwen/Qwen3-0.6B:main"
	modelURL, err := apis.ParseURL(uri)
	require.NoError(t, err)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ms-token", Namespace: "default"}, Data: map[string][]byte{"MODELSCOPE_API_TOKEN": []byte("test")}}
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "modelscope", Namespace: "default"}, Secrets: []corev1.ObjectReference{{Name: secret.Name}}}
	r := &LLMISVCReconciler{Client: selectorTestClient(t), Clientset: k8sfake.NewSimpleClientset(secret)}
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}, Spec: v1alpha2.LLMInferenceServiceSpec{Model: v1alpha2.LLMModelSpec{URI: *modelURL}}}
	config := &Config{StorageConfig: testStorageConfig(), CredentialConfig: &credentials.CredentialConfig{}}
	for _, containerName := range []string{"main", tokenizerContainerName} {
		t.Run(containerName, func(t *testing.T) {
			pod := &corev1.PodSpec{Containers: []corev1.Container{{Name: containerName}}}
			err := r.attachModelArtifacts(t.Context(), sa, svc, corev1.PodSpec{}, pod, config, containerName, constants.DefaultModelLocalMountPath, false)
			require.NoError(t, err)
			init := utils.GetInitContainerWithName(pod, constants.StorageInitializerContainerName)
			require.NotNil(t, init)
			require.Equal(t, []string{uri, "/mnt/models"}, init.Args)
			require.Contains(t, init.Env, corev1.EnvVar{Name: "MODELSCOPE_API_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "ms-token"}, Key: "MODELSCOPE_API_TOKEN"}}})
			require.Empty(t, hfEnv(init))
			if containerName == tokenizerContainerName {
				require.Contains(t, init.Env, tokenizerOnlyDownload)
			}
			require.NotEmpty(t, pod.Containers[0].VolumeMounts)
		})
	}
}

func TestAttachModelScopeWithHuggingFaceAdapter(t *testing.T) {
	t.Parallel()
	modelURL, err := apis.ParseURL("modelscope://Qwen/Qwen3-0.6B:main")
	require.NoError(t, err)
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec:       v1alpha2.LLMInferenceServiceSpec{Model: v1alpha2.LLMModelSpec{URI: *modelURL}},
	}
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"}}
	r := &LLMISVCReconciler{Client: selectorTestClient(t), Clientset: k8sfake.NewSimpleClientset()}
	config := &Config{
		StorageConfig:    testStorageConfig(),
		CredentialConfig: &credentials.CredentialConfig{},
		ResolvedLoRAAdapters: []resolvedLoRAAdapter{{
			name: "adapter", uri: "hf://org/adapter", scheme: constants.HfURIPrefix, mountPath: "/mnt/lora/adapter",
		}},
	}
	pod := &corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}
	require.NoError(t, r.attachModelArtifacts(t.Context(), sa, svc, corev1.PodSpec{}, pod, config, "main", "/mnt/models", true))
	init := utils.GetInitContainerWithName(pod, constants.StorageInitializerContainerName)
	require.NotNil(t, init)
	require.Equal(t, []string{"modelscope://Qwen/Qwen3-0.6B:main", "/mnt/models", "hf://org/adapter", "/mnt/lora/adapter"}, init.Args)
	require.NotEmpty(t, hfEnv(init))
}
