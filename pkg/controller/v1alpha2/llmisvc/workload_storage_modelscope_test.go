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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/credentials"
	"github.com/kserve/kserve/pkg/credentials/ms"
	kserveTypes "github.com/kserve/kserve/pkg/types"
	"github.com/kserve/kserve/pkg/utils"
)

func TestAttachModelScopeArtifacts(t *testing.T) {
	for _, tc := range []struct {
		name      string
		container string
		private   bool
		disabled  bool
	}{
		{name: "public model", container: mainContainerName},
		{name: "private model", container: mainContainerName, private: true},
		{name: "tokenizer", container: tokenizerContainerName, private: true},
		{name: "disabled initializer", container: mainContainerName, disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "modelscope-token", Namespace: "test"},
				Data:       map[string][]byte{ms.MSTokenKey: []byte("test-token")},
			}
			sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "modelscope", Namespace: "test"}}
			if tc.private {
				sa.Secrets = []corev1.ObjectReference{{Name: secret.Name}}
			}
			r := &LLMISVCReconciler{
				Clientset: k8sfake.NewClientset(secret),
				Client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(sa, secret).Build(),
			}
			modelURI := "modelscope://Qwen/Qwen3-0.6B:master"
			u, err := apis.ParseURL(modelURI)
			require.NoError(t, err)
			svc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
				Spec:       v1alpha2.LLMInferenceServiceSpec{Model: v1alpha2.LLMModelSpec{URI: *u}},
			}
			if tc.disabled {
				svc.Spec.StorageInitializer = &v1alpha2.StorageInitializerSpec{Enabled: ptr.To(false)}
			}
			cfg := &Config{
				StorageConfig: &kserveTypes.StorageInitializerConfig{
					Image:         "storage-initializer:test",
					CpuRequest:    "100m",
					CpuLimit:      "1",
					MemoryRequest: "100Mi",
					MemoryLimit:   "1Gi",
				},
				CredentialConfig: &credentials.CredentialConfig{},
			}
			pod := &corev1.PodSpec{Containers: []corev1.Container{{Name: tc.container}}}
			require.NoError(t, r.attachModelArtifacts(t.Context(), sa, svc, corev1.PodSpec{}, pod, cfg, tc.container, "/mnt/models", false))
			init := utils.GetInitContainerWithName(pod, constants.StorageInitializerContainerName)
			if tc.disabled {
				require.Nil(t, init)
				return
			}
			require.NotNil(t, init)
			require.Equal(t, []string{modelURI, "/mnt/models"}, init.Args)
			require.Contains(t, init.VolumeMounts, corev1.VolumeMount{Name: constants.StorageInitializerVolumeName, MountPath: "/mnt/models"})
			require.Contains(t, pod.Containers[0].VolumeMounts, corev1.VolumeMount{Name: constants.StorageInitializerVolumeName, MountPath: "/mnt/models", ReadOnly: true})
			for _, env := range init.Env {
				require.False(t, strings.HasPrefix(env.Name, "HF_"), "ModelScope should not receive HF defaults")
			}
			if tc.private {
				require.Contains(t, init.Env, corev1.EnvVar{
					Name: ms.MSTokenKey,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: secret.Name},
							Key:                  ms.MSTokenKey,
						},
					},
				})
			}
			if tc.container == tokenizerContainerName {
				require.Contains(t, init.Env, tokenizerOnlyDownload)
			}
		})
	}
}
