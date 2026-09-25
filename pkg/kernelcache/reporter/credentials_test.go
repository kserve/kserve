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

package reporter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestScopedReporterNamesAreDeterministicAndBounded(t *testing.T) {
	captureName := strings.Repeat("model-", 20) + "kcc-abc123"
	for _, name := range []string{
		ServiceAccountName(captureName),
		RoleName(captureName),
		RoleBindingName(captureName),
		SecretName(captureName),
	} {
		require.LessOrEqual(t, len(name), 63)
		require.Equal(t, name, strings.ToLower(name))
	}
	require.Equal(t, ServiceAccountName(captureName), ServiceAccountName(captureName))
	require.NotEqual(t, ServiceAccountName(captureName), ServiceAccountName("other"))
}

func TestIsReporterForCapture(t *testing.T) {
	const namespace = "team"
	captureName := "model-kcc-abc123"
	dynamicUsername := "system:serviceaccount:" + namespace + ":" + ServiceAccountName(captureName)

	require.True(t, IsReporterForCapture(dynamicUsername, namespace, captureName))
	require.False(t, IsReporterForCapture(dynamicUsername, namespace, "other"))
	require.False(t, IsReporterForCapture(dynamicUsername, "other", captureName))
	require.False(t, IsReporterForCapture("system:serviceaccount:"+namespace+":"+ServiceAccountName("legacy"), namespace, captureName))
}

func TestIssueForCaptureReusesUnexpiredCredential(t *testing.T) {
	ctx := context.Background()
	captureName := "model-kcc-abc123"
	pod, owner := reporterTestPodAndOwner()
	expiresAt := time.Now().Add(5 * time.Minute)
	credential, err := json.Marshal(Credential{Token: "existing-token", ExpiresAt: metav1.NewTime(expiresAt)})
	require.NoError(t, err)
	secret := reporterTestSecret(owner, credential)
	clientset := kubernetesfake.NewSimpleClientset(pod, secret)
	tokenRequests := 0
	installReporterTokenReactor(clientset, &tokenRequests, "new-token", time.Now().Add(5*time.Minute))

	got, err := (&Credentials{Client: clientset}).IssueForCapture(ctx, pod, captureName, owner)
	require.NoError(t, err)
	require.Equal(t, secret.Data[AccessKey], got.Data[AccessKey])
	require.Zero(t, tokenRequests)
}

func TestIssueForCaptureRefreshesExpiringCredential(t *testing.T) {
	ctx := context.Background()
	captureName := "model-kcc-abc123"
	pod, owner := reporterTestPodAndOwner()
	oldCredential, err := json.Marshal(Credential{Token: "old-token", ExpiresAt: metav1.NewTime(time.Now().Add(30 * time.Second))})
	require.NoError(t, err)
	secret := reporterTestSecret(owner, oldCredential)
	clientset := kubernetesfake.NewSimpleClientset(pod, secret)
	tokenRequests := 0
	installReporterTokenReactor(clientset, &tokenRequests, "refreshed-token", time.Now().Add(5*time.Minute))

	got, err := (&Credentials{Client: clientset}).IssueForCapture(ctx, pod, captureName, owner)
	require.NoError(t, err)
	require.Equal(t, 1, tokenRequests)

	var refreshed Credential
	require.NoError(t, json.Unmarshal(got.Data[AccessKey], &refreshed))
	require.Equal(t, "refreshed-token", refreshed.Token)
	require.True(t, refreshed.ExpiresAt.After(time.Now().Add(4*time.Minute)))
}

func TestIssueForCaptureCreatesReporterSecret(t *testing.T) {
	ctx := context.Background()
	captureName := "model-kcc-abc123"
	pod, owner := reporterTestPodAndOwner()
	clientset := kubernetesfake.NewSimpleClientset(pod)
	tokenRequests := 0
	installReporterTokenReactor(clientset, &tokenRequests, "new-token", time.Now().Add(5*time.Minute))

	got, err := (&Credentials{Client: clientset}).IssueForCapture(ctx, pod, captureName, owner)
	require.NoError(t, err)
	require.Equal(t, 1, tokenRequests)
	require.Equal(t, SecretName(captureName), got.Name)

	var credential Credential
	require.NoError(t, json.Unmarshal(got.Data[AccessKey], &credential))
	require.Equal(t, "new-token", credential.Token)
}

func TestIssueForCaptureUsesSecretCreatedConcurrently(t *testing.T) {
	ctx := context.Background()
	captureName := "model-kcc-abc123"
	pod, owner := reporterTestPodAndOwner()
	clientset := kubernetesfake.NewSimpleClientset(pod)
	clientset.PrependReactor("create", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		secret := createAction.GetObject().(*corev1.Secret).DeepCopy()
		if err := clientset.Tracker().Create(corev1.SchemeGroupVersion.WithResource("secrets"), secret, secret.Namespace); err != nil {
			return true, nil, err
		}
		return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "secrets"}, secret.Name)
	})
	tokenRequests := 0
	installReporterTokenReactor(clientset, &tokenRequests, "new-token", time.Now().Add(5*time.Minute))

	got, err := (&Credentials{Client: clientset}).IssueForCapture(ctx, pod, captureName, owner)
	require.NoError(t, err)
	require.Equal(t, 1, tokenRequests)
	require.Equal(t, SecretName(captureName), got.Name)
}

func TestIssueForCaptureRejectsInactiveOrMismatchedPod(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*corev1.Pod)
		want   string
	}{
		{
			name: "stale uid",
			mutate: func(pod *corev1.Pod) {
				pod.UID = "new-pod-uid"
			},
			want: "capture Pod is no longer active",
		},
		{
			name: "pod is terminating",
			mutate: func(pod *corev1.Pod) {
				now := metav1.Now()
				pod.DeletionTimestamp = &now
			},
			want: "capture Pod is no longer active",
		},
		{
			name: "annotation mismatch",
			mutate: func(pod *corev1.Pod) {
				pod.Annotations[AccessSecretAnnotation] = SecretName("other-capture")
			},
			want: "capture Pod has no reporter access Secret reference",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			captureName := "model-kcc-abc123"
			pod, owner := reporterTestPodAndOwner()
			livePod := pod.DeepCopy()
			test.mutate(livePod)
			clientset := kubernetesfake.NewSimpleClientset(livePod)

			_, err := (&Credentials{Client: clientset}).IssueForCapture(ctx, pod, captureName, owner)
			require.EqualError(t, err, test.want)
		})
	}
}

func TestIssueForCaptureRejectsForeignSecret(t *testing.T) {
	tests := []struct {
		name         string
		foreignOwner bool
		missingLabel bool
	}{
		{name: "foreign owner", foreignOwner: true},
		{name: "missing managed label", missingLabel: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			captureName := "model-kcc-abc123"
			pod, owner := reporterTestPodAndOwner()
			secretOwner := owner
			if test.foreignOwner {
				secretOwner = owner.DeepCopy()
				secretOwner.UID = "different-capture-uid"
			}
			secret := reporterTestSecret(secretOwner, []byte("{}"))
			if test.missingLabel {
				delete(secret.Labels, ManagedLabel)
			}
			clientset := kubernetesfake.NewSimpleClientset(pod, secret)
			tokenRequests := 0
			installReporterTokenReactor(clientset, &tokenRequests, "new-token", time.Now().Add(5*time.Minute))

			_, err := (&Credentials{Client: clientset}).IssueForCapture(ctx, pod, captureName, owner)
			require.EqualError(t, err, "reporter access Secret does not belong to this capture")
			require.Zero(t, tokenRequests)
		})
	}
}

func TestIssueForCaptureDoesNotStoreInvalidTokenResponse(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		expiresAt time.Time
	}{
		{name: "empty token", token: "", expiresAt: time.Now().Add(5 * time.Minute)},
		{name: "expired token", token: "expired-token", expiresAt: time.Now().Add(-time.Minute)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			captureName := "model-kcc-abc123"
			pod, owner := reporterTestPodAndOwner()
			originalData := []byte("{}")
			secret := reporterTestSecret(owner, originalData)
			clientset := kubernetesfake.NewSimpleClientset(pod, secret)
			tokenRequests := 0
			installReporterTokenReactor(clientset, &tokenRequests, test.token, test.expiresAt)

			_, err := (&Credentials{Client: clientset}).IssueForCapture(ctx, pod, captureName, owner)
			require.EqualError(t, err, "TokenRequest returned an unacceptable reporter access")
			require.Equal(t, 1, tokenRequests)

			stored, getErr := clientset.CoreV1().Secrets(pod.Namespace).Get(ctx, secret.Name, metav1.GetOptions{})
			require.NoError(t, getErr)
			require.Equal(t, originalData, stored.Data[AccessKey])
		})
	}
}

func reporterTestPodAndOwner() (*corev1.Pod, *metav1.OwnerReference) {
	captureName := "model-kcc-abc123"
	controller := true
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "model-pod",
		Namespace: "team",
		UID:       "pod-uid",
		Annotations: map[string]string{
			AccessSecretAnnotation: SecretName(captureName),
		},
	}}
	owner := &metav1.OwnerReference{
		APIVersion: "serving.kserve.io/v1alpha1",
		Kind:       "KernelCacheCapture",
		Name:       captureName,
		UID:        "capture-uid",
		Controller: &controller,
	}
	return pod, owner
}

func reporterTestSecret(owner *metav1.OwnerReference, data []byte) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:            SecretName(owner.Name),
		Namespace:       "team",
		Labels:          map[string]string{ManagedLabel: "true"},
		OwnerReferences: []metav1.OwnerReference{*owner},
	}, Data: map[string][]byte{AccessKey: data}}
}

func installReporterTokenReactor(clientset *kubernetesfake.Clientset, calls *int, token string, expiresAt time.Time) {
	clientset.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "token" {
			return false, nil, nil
		}
		(*calls)++
		createAction := action.(k8stesting.CreateAction)
		request := createAction.GetObject().(*authenticationv1.TokenRequest).DeepCopy()
		request.Status.Token = token
		request.Status.ExpirationTimestamp = metav1.NewTime(expiresAt)
		return true, request, nil
	})
}
