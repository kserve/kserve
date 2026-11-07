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

package reconcilers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/kernelcache/captureconfig"
	"github.com/kserve/kserve/pkg/kernelcache/reporter"
)

func TestCaptureConfigFromPodReadsGroupedConfiguration(t *testing.T) {
	want := newTestCaptureConfig("capture", "session-id")
	value, err := captureconfig.MarshalCaptureConfig(want)
	require.NoError(t, err)
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{
		Name: "mcv",
		Env:  []corev1.EnvVar{{Name: captureconfig.CaptureConfigEnv, Value: value}},
	}}}}

	got, found, err := captureConfigFromPod(pod)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, want, got)
}

func TestCaptureConfigFromPodRequiresGroupedConfiguration(t *testing.T) {
	pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "mcv"}}}}

	_, found, err := captureConfigFromPod(pod)
	require.NoError(t, err)
	require.False(t, found)
}

func newTestCaptureConfig(name, sessionID string) captureconfig.CaptureConfig {
	return captureconfig.CaptureConfig{
		Version:     captureconfig.CurrentVersion,
		CacheDir:    "/workspace/cache/0",
		TargetImage: "registry.example/team/cache:session",
		Capture: captureconfig.CaptureIdentity{
			Name:      name,
			Namespace: "team",
			SessionID: sessionID,
		},
		CachePaths: []v1alpha1.KernelCachePath{{
			ContainerName: "kserve-container",
			ContainerPath: "/tmp/vllm",
			OCIPath:       "io.vllm.cache",
		}},
	}
}

func TestCaptureControllerCreatesDeterministicCaptureAndReporterIdentity(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))

	const (
		namespace   = "team"
		captureName = "model-kcc-669889f77f"
		sessionID   = "session-id"
		podUID      = "pod-uid"
	)
	controller := true
	captureValue, err := captureconfig.MarshalCaptureConfig(newTestCaptureConfig(captureName, sessionID))
	require.NoError(t, err)
	inferenceService := &v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: namespace, UID: "isvc-uid"},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "model-pod",
			Namespace: namespace,
			UID:       podUID,
			Labels: map[string]string{
				constants.InferenceServicePodLabelKey: inferenceService.Name,
				"pod-template-hash":                   "669889f77f",
			},
			Annotations: map[string]string{
				reporter.AccessSecretAnnotation:             reporter.SecretName(captureName),
				constants.KernelCacheNodeGroupAnnotationKey: "gpu-workers",
			},
			OwnerReferences: []metav1.OwnerReference{{
				UID:        "replicaset-uid",
				Controller: &controller,
			}},
		},
		Spec: corev1.PodSpec{NodeName: "gpu-node", Containers: []corev1.Container{{
			Name: "mcv",
			Env:  []corev1.EnvVar{{Name: captureconfig.CaptureConfigEnv, Value: captureValue}},
		}}},
	}
	replicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: "model-predictor-rs", Namespace: namespace, UID: "replicaset-uid",
		OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "model-predictor", Namespace: namespace, UID: "deployment-uid"}},
			appsv1.SchemeGroupVersion.WithKind("Deployment"),
		)},
	}}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: "model-predictor", Namespace: namespace, UID: "deployment-uid",
		OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(inferenceService, v1beta1.SchemeGroupVersion.WithKind("InferenceService"))},
	}}
	pod.OwnerReferences[0].Name = replicaSet.Name
	pod.OwnerReferences[0].UID = replicaSet.UID
	pod.OwnerReferences[0].APIVersion = appsv1.SchemeGroupVersion.String()
	pod.OwnerReferences[0].Kind = "ReplicaSet"
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true}`},
	}
	namespaceObject := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(namespaceObject, configMap, inferenceService, deployment, replicaSet, pod).
		WithStatusSubresource(&v1alpha1.KernelCacheCapture{}).
		Build()
	clientset := kubernetesfake.NewSimpleClientset(pod.DeepCopy())
	clientset.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction, ok := action.(k8stesting.CreateAction)
		if !ok || action.GetSubresource() != "token" {
			return false, nil, nil
		}
		request := createAction.GetObject().(*authenticationv1.TokenRequest).DeepCopy()
		request.Status.Token = "reporter-token"
		request.Status.ExpirationTimestamp = metav1.NewTime(time.Now().Add(10 * time.Minute))
		return true, request, nil
	})
	reconciler := &KernelCacheCaptureControllerReconciler{
		Client:                 k8sClient,
		Reader:                 k8sClient,
		Clientset:              clientset,
		OperatorNamespace:      "kserve",
		OperatorServiceAccount: "operator",
	}

	_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pod)})
	require.NoError(t, err)

	capture := &v1alpha1.KernelCacheCapture{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: captureName}, capture))
	require.Equal(t, "true", capture.Labels[constants.KernelCacheCaptureGeneratedLabelKey])
	require.Empty(t, capture.Annotations)
	require.Equal(t, v1alpha1.KernelCacheSourceRef{Kind: "InferenceService", Name: inferenceService.Name}, capture.Spec.SourceRef)
	require.Equal(t, "registry.example/team/cache:session", capture.Spec.TargetImage)
	require.Equal(t, []v1alpha1.KernelCachePath{{ContainerName: "kserve-container", ContainerPath: "/tmp/vllm", OCIPath: "io.vllm.cache"}}, capture.Spec.CachePaths)
	require.Len(t, capture.OwnerReferences, 1)
	require.Equal(t, replicaSet.UID, capture.OwnerReferences[0].UID)
	require.Equal(t, "ReplicaSet", capture.OwnerReferences[0].Kind)
	require.NotNil(t, capture.OwnerReferences[0].BlockOwnerDeletion)
	require.False(t, *capture.OwnerReferences[0].BlockOwnerDeletion)
	require.Nil(t, capture.Status.ActiveSession)

	serviceAccount := &corev1.ServiceAccount{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: reporter.ServiceAccountName(captureName)}, serviceAccount))
	role := &rbacv1.Role{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: reporter.RoleName(captureName)}, role))
	binding := &rbacv1.RoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: reporter.RoleBindingName(captureName)}, binding))
	tokenBinding := &rbacv1.RoleBinding{}
	require.NoError(t, k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: reporter.TokenRequesterRole}, tokenBinding))
	require.Equal(t, reporter.TokenRequesterRole, tokenBinding.RoleRef.Name)
	secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, reporter.SecretName(captureName), metav1.GetOptions{})
	require.NoError(t, err)
	require.Contains(t, secret.Data, reporter.AccessKey)

	capture.Status.Phase = v1alpha1.KernelCacheCapturePhaseComplete
	require.NoError(t, k8sClient.Status().Update(ctx, capture))
	for _, object := range []client.Object{
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: reporter.ServiceAccountName(captureName), Namespace: namespace}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: reporter.RoleName(captureName), Namespace: namespace}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: reporter.RoleBindingName(captureName), Namespace: namespace}},
	} {
		require.NoError(t, k8sClient.Delete(ctx, object))
	}
	require.NoError(t, clientset.CoreV1().Secrets(namespace).Delete(ctx, reporter.SecretName(captureName), metav1.DeleteOptions{}))
	result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pod)})
	require.NoError(t, err)
	require.Equal(t, ctrl.Result{}, result)
	for _, object := range []client.Object{
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: reporter.ServiceAccountName(captureName), Namespace: namespace}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: reporter.RoleName(captureName), Namespace: namespace}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: reporter.RoleBindingName(captureName), Namespace: namespace}},
	} {
		require.True(t, apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(object), object)))
	}
	_, err = clientset.CoreV1().Secrets(namespace).Get(ctx, reporter.SecretName(captureName), metav1.GetOptions{})
	require.True(t, apierrors.IsNotFound(err))
}

func TestCaptureControllerCaptureOverride(t *testing.T) {
	const annotation = "serving.kserve.io/kernelcache-capture"
	for _, tc := range []struct {
		name    string
		mutate  func(*appsv1.ReplicaSet, *corev1.Pod)
		missing bool
	}{
		{name: "uses user capture without adopting it"},
		{name: "reuses referenced capture for a new ReplicaSet and Pod", mutate: func(rs *appsv1.ReplicaSet, pod *corev1.Pod) {
			rs.Name = "model-next-rs"
			rs.UID = "next-rs-uid"
			rs.Labels[appsv1.DefaultDeploymentUniqueLabelKey] = "779990a88a"
			pod.Name = "model-next-pod"
			pod.UID = "next-pod-uid"
			pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey] = "779990a88a"
			pod.OwnerReferences = []metav1.OwnerReference{*metav1.NewControllerRef(rs, appsv1.SchemeGroupVersion.WithKind("ReplicaSet"))}
		}},
		{name: "missing capture does not generate a replacement", missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			require.NoError(t, appsv1.AddToScheme(scheme))
			require.NoError(t, rbacv1.AddToScheme(scheme))
			require.NoError(t, v1alpha1.AddToScheme(scheme))
			require.NoError(t, v1beta1.AddToScheme(scheme))
			isvc := &v1beta1.InferenceService{ObjectMeta: metav1.ObjectMeta{
				Name: "model", Namespace: "team", UID: "isvc-uid", Annotations: map[string]string{annotation: "custom-capture"},
			}}
			deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name: "model-predictor", Namespace: "team", UID: "deployment-uid",
				OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(isvc, v1beta1.SchemeGroupVersion.WithKind("InferenceService"))},
			}}
			rs := &appsv1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "model-rs", Namespace: "team", UID: "rs-uid",
					Labels:          map[string]string{appsv1.DefaultDeploymentUniqueLabelKey: "669889f77f"},
					OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(deployment, appsv1.SchemeGroupVersion.WithKind("Deployment"))},
				},
				Spec: appsv1.ReplicaSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotation: "custom-capture"}}}},
			}
			value, err := captureconfig.MarshalCaptureConfig(newTestCaptureConfig("custom-capture", "session-id"))
			require.NoError(t, err)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "model-pod", Namespace: "team", UID: "pod-uid",
					Labels:          map[string]string{constants.InferenceServicePodLabelKey: "model", appsv1.DefaultDeploymentUniqueLabelKey: "669889f77f"},
					Annotations:     map[string]string{annotation: "custom-capture", reporter.AccessSecretAnnotation: reporter.SecretName("custom-capture")},
					OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(rs, appsv1.SchemeGroupVersion.WithKind("ReplicaSet"))},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "mcv", Env: []corev1.EnvVar{{Name: captureconfig.CaptureConfigEnv, Value: value}}}}},
			}
			capture := &v1alpha1.KernelCacheCapture{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-capture", Namespace: "team", UID: "capture-uid", Labels: map[string]string{"user-label": "keep"}},
				Spec: v1alpha1.KernelCacheCaptureSpec{
					SourceRef:   v1alpha1.KernelCacheSourceRef{Kind: "InferenceService", Name: isvc.Name},
					TargetImage: "registry.example/custom:v1",
					Signing:     &v1alpha1.KernelCacheSigningSpec{ProfileRef: &corev1.LocalObjectReference{Name: "user-profile"}},
				},
			}
			if tc.mutate != nil {
				tc.mutate(rs, pod)
			}
			objects := []client.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team"}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace}, Data: map[string]string{"kernelcache": `{"enabled":true}`}},
				isvc, deployment, rs, pod,
			}
			if !tc.missing {
				objects = append(objects, capture)
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithStatusSubresource(&v1alpha1.KernelCacheCapture{}).Build()
			clientset := kubernetesfake.NewSimpleClientset(pod.DeepCopy())
			clientset.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
				if action.GetSubresource() != "token" {
					return false, nil, nil
				}
				request := action.(k8stesting.CreateAction).GetObject().(*authenticationv1.TokenRequest).DeepCopy()
				request.Status.Token = "reporter-token"
				request.Status.ExpirationTimestamp = metav1.NewTime(time.Now().Add(10 * time.Minute))
				return true, request, nil
			})
			reconciler := &KernelCacheCaptureControllerReconciler{
				Client: k8sClient, Reader: k8sClient, Clientset: clientset, OperatorNamespace: "kserve", OperatorServiceAccount: "operator",
			}
			original := capture.DeepCopy()
			if !tc.missing {
				require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), original))
			}
			_, err = reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pod)})
			if tc.missing {
				require.ErrorContains(t, err, capture.Name)
			} else {
				require.NoError(t, err)
				// Reconciliation is idempotent and never adopts or rewrites the user resource.
				_, err = reconciler.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pod)})
				require.NoError(t, err)
				role := &rbacv1.Role{}
				require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKey{Namespace: "team", Name: reporter.RoleName(capture.Name)}, role))
				require.Equal(t, []string{capture.Name}, role.Rules[1].ResourceNames)
				require.Equal(t, capture.UID, metav1.GetControllerOf(role).UID)
				secret, getErr := clientset.CoreV1().Secrets("team").Get(t.Context(), reporter.SecretName(capture.Name), metav1.GetOptions{})
				require.NoError(t, getErr)
				require.Equal(t, capture.UID, metav1.GetControllerOf(secret).UID)
			}
			if !tc.missing {
				updated := &v1alpha1.KernelCacheCapture{}
				require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKeyFromObject(capture), updated))
				require.Equal(t, original, updated)
			}
			generated := &v1alpha1.KernelCacheCapture{}
			generatedName := constants.KernelCacheCaptureRevisionName(isvc.Name, pod.Labels[appsv1.DefaultDeploymentUniqueLabelKey])
			require.True(t, apierrors.IsNotFound(k8sClient.Get(t.Context(), client.ObjectKey{Namespace: "team", Name: generatedName}, generated)))
		})
	}
}

func TestCaptureControllerIgnoresPodWithoutVerifiedInferenceServiceOwner(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))

	const namespace = "team"
	configValue, err := captureconfig.MarshalCaptureConfig(newTestCaptureConfig("model-capture", "session"))
	require.NoError(t, err)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-created-pod",
			Namespace: namespace,
			Labels:    map[string]string{constants.InferenceServicePodLabelKey: "model"},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "mcv",
			Env:  []corev1.EnvVar{{Name: captureconfig.CaptureConfigEnv, Value: configValue}},
		}}},
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":true}`},
	}
	namespaceObject := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespaceObject, configMap, pod).Build()
	r := &KernelCacheCaptureControllerReconciler{Client: c, Reader: c}

	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(pod)})
	require.NoError(t, err)
	require.True(t, apierrors.IsNotFound(c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: "model-capture"}, &v1alpha1.KernelCacheCapture{})))
	require.True(t, apierrors.IsNotFound(c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: reporter.ServiceAccountName("model-capture")}, &corev1.ServiceAccount{})))
}

func TestCaptureControllerRemovesTokenRequesterBindingWhenDisabled(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, v1beta1.AddToScheme(scheme))
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: constants.InferenceServiceConfigMapName, Namespace: constants.KServeNamespace},
		Data:       map[string]string{"kernelcache": `{"enabled":false}`},
	}
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "team"}}
	service := &v1beta1.InferenceService{ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "team"}}
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reporter.TokenRequesterRole,
			Namespace: "team",
			Labels:    map[string]string{reporter.TokenRequesterManagedLabel: "true"},
		},
		RoleRef:  rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: reporter.TokenRequesterRole},
		Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: "operator", Namespace: "kserve"}},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap, namespace, service, binding).Build()
	r := &KernelCacheCaptureControllerReconciler{
		Client:                 c,
		Reader:                 c,
		OperatorNamespace:      "kserve",
		OperatorServiceAccount: "operator",
	}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(service)}

	_, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	require.True(t, apierrors.IsNotFound(c.Get(ctx, client.ObjectKey{Namespace: "team", Name: reporter.TokenRequesterRole}, binding)))
}

func TestTokenRequesterBindingRequestsEnqueueWorkloadsInNamespace(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&v1beta1.InferenceService{ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "team"}},
		&v1beta1.InferenceService{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "other"}},
	).Build()
	r := &KernelCacheCaptureControllerReconciler{Client: c, Reader: c}

	requests := r.tokenRequesterBindingRequests(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
		Name: reporter.TokenRequesterRole, Namespace: "team",
	}})
	require.Equal(t, []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: "team", Name: "model"}}}, requests)
}
