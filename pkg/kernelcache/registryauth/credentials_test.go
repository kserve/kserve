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

package registryauth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

func TestSecretBoundAccessLifecycle(t *testing.T) {
	ctx := context.Background()
	const captureName = "model-kcc-revision"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "runtime", Namespace: "team", UID: types.UID("pod-uid"),
			Annotations: map[string]string{AccessSecretAnnotation: "mcv-registry-test"},
		},
		Spec: corev1.PodSpec{ServiceAccountName: "default"},
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: "mcv-registry-test", Namespace: "team", UID: types.UID("secret-uid"),
		Labels: map[string]string{ManagedLabel: "true"}, OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(pod, corev1.SchemeGroupVersion.WithKind("Pod"))},
	}}
	client := fake.NewSimpleClientset(pod, secret)
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	controllerClient := controllerfake.NewClientBuilder().WithScheme(scheme).WithObjects(pod, secret).Build()
	requests := 0
	client.PrependReactor("create", "serviceaccounts", func(action clienttesting.Action) (bool, runtime.Object, error) {
		require.Equal(t, "token", action.GetSubresource())
		create := action.(clienttesting.CreateAction)
		req := create.GetObject().(*authenticationv1.TokenRequest)
		require.Equal(t, PusherServiceAccountName(captureName), action.(clienttesting.CreateActionImpl).Name)
		require.Equal(t, int64(600), *req.Spec.ExpirationSeconds)
		require.Equal(t, "Secret", req.Spec.BoundObjectRef.Kind)
		require.Equal(t, secret.UID, req.Spec.BoundObjectRef.UID)
		requests++
		return true, &authenticationv1.TokenRequest{Status: authenticationv1.TokenRequestStatus{
			Token: "test-token", ExpirationTimestamp: metav1.NewTime(time.Now().Add(10 * time.Minute)),
		}}, nil
	})
	manager := &Credentials{Client: client}
	cfg := v1beta1.KernelCacheRegistryConfig{
		Endpoint: "registry.example:5000",
		Auth: v1beta1.KernelCacheRegistryAuth{
			Type:        v1beta1.KernelCacheRegistryAuthTypeServiceAccountToken,
			PushRoleRef: &v1beta1.KernelCacheRegistryRoleRef{Kind: "ClusterRole", Name: "registry-pusher"},
			PullRoleRef: &v1beta1.KernelCacheRegistryRoleRef{Kind: "ClusterRole", Name: "registry-puller"},
		},
	}
	result, err := manager.IssueForCapture(ctx, pod, captureName, cfg)
	require.NoError(t, err)
	var credential RegistryCredential
	require.NoError(t, json.Unmarshal(result.Data[AccessKey], &credential))
	require.Equal(t, "test-token", credential.Token)
	_, err = manager.IssueForCapture(ctx, pod, captureName, cfg)
	require.NoError(t, err)
	require.Equal(t, 1, requests)
	live, err := client.CoreV1().Pods("team").Get(ctx, "runtime", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "default", live.Spec.ServiceAccountName)
	other := pod.DeepCopy()
	other.UID = "different-pod"
	require.Error(t, Revoke(ctx, controllerClient, other))
	require.NoError(t, Revoke(ctx, controllerClient, pod))
	require.NoError(t, Revoke(ctx, controllerClient, pod))
}

func TestNoAuthDoesNotAccessKubernetes(t *testing.T) {
	client := fake.NewSimpleClientset()
	manager := &Credentials{Client: client}
	secret, err := manager.IssueForCapture(context.Background(), &corev1.Pod{}, "capture", v1beta1.KernelCacheRegistryConfig{})
	require.NoError(t, err)
	require.Nil(t, secret)
	require.Empty(t, client.Actions())
}
