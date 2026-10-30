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
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func TestAttachPVCEvictor(t *testing.T) {
	podSpec := &corev1.PodSpec{
		AutomountServiceAccountToken: ptr.To(true),
		Containers: []corev1.Container{
			{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
			{Name: pvcEvictorTemplateName, Image: "example.com/evictor:preset"},
		},
	}
	secondary := []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
		PVC: &v1alpha2.PVCTierSpec{
			Spec: &corev1.PersistentVolumeClaimSpec{},
		},
	}}}

	evictorTemplate := takePVCEvictorContainerTemplate(podSpec)
	attachKVCacheSecondaryTiers(podSpec, secondary, mainContainerName, evictorTemplate)

	if got, want := len(podSpec.Containers), 2; got != want {
		t.Fatalf("container count = %d, want %d", got, want)
	}
	if podSpec.AutomountServiceAccountToken == nil || !*podSpec.AutomountServiceAccountToken {
		t.Fatal("PVC evictor sidecar changed Pod ServiceAccount token automount")
	}
	evictor := podSpec.Containers[1]
	if got, want := evictor.Name, "pvc-evictor-kv-cache-secondary-0"; got != want {
		t.Errorf("evictor name = %q, want %q", got, want)
	}
	if got, want := evictor.Image, "example.com/evictor:preset"; got != want {
		t.Errorf("evictor image = %q, want %q", got, want)
	}
	if diff := cmp.Diff(pvcEvictorResources(), evictor.Resources); diff != "" {
		t.Errorf("evictor resources diff (-want +got):\n%s", diff)
	}
	if got, want := len(evictor.Env), 2; got != want {
		t.Fatalf("evictor env count = %d, want %d", got, want)
	}
	wantEnv := corev1.EnvVar{Name: "PVC_MOUNT_PATH", Value: "/mnt/kv-cache-0"}
	if diff := cmp.Diff(wantEnv, evictor.Env[0]); diff != "" {
		t.Errorf("evictor env diff (-want +got):\n%s", diff)
	}
	wantCacheDirectory := corev1.EnvVar{Name: "CACHE_DIRECTORY", Value: pvcEvictorCacheDirectory}
	if diff := cmp.Diff(wantCacheDirectory, evictor.Env[1]); diff != "" {
		t.Errorf("evictor cache directory env diff (-want +got):\n%s", diff)
	}
	if got, want := len(evictor.VolumeMounts), 1; got != want {
		t.Fatalf("evictor volume mount count = %d, want %d", got, want)
	}
	wantMount := corev1.VolumeMount{Name: "kv-cache-secondary-0", MountPath: "/mnt/kv-cache-0"}
	if diff := cmp.Diff(wantMount, evictor.VolumeMounts[0]); diff != "" {
		t.Errorf("evictor volume mount diff (-want +got):\n%s", diff)
	}
}

func TestAttachPVCEvictorDoesNotInventImage(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
			{Name: pvcEvictorTemplateName},
		},
	}
	secondary := []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
		PVC: &v1alpha2.PVCTierSpec{Spec: &corev1.PersistentVolumeClaimSpec{}},
	}}}

	evictorTemplate := takePVCEvictorContainerTemplate(podSpec)
	attachKVCacheSecondaryTiers(podSpec, secondary, mainContainerName, evictorTemplate)

	if got := podSpec.Containers[1].Image; got != "" {
		t.Fatalf("evictor image = %q, want empty when the template has no image", got)
	}
}

func TestFinalizePVCEvictorsSkipsConfigWithoutDeployments(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	serviceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:      pvcEvictorServiceAccount,
		Namespace: "test",
		Labels:    map[string]string{pvcEvictorLabel: "true"},
	}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(serviceAccount).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	llmSvc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Namespace: "test"}}

	if err := reconciler.finalizePVCEvictors(t.Context(), llmSvc); err != nil {
		t.Fatalf("finalizePVCEvictors() error = %v", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(serviceAccount), &corev1.ServiceAccount{}); err == nil {
		t.Fatal("PVC evictor ServiceAccount remains after finalization")
	}
}

func TestFinalizePVCEvictorsSkipsConfigForUnrelatedDeletingService(t *testing.T) {
	deletionTime := metav1.Now()
	deleting := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{
		Name:              "unrelated-deleting",
		Namespace:         "test",
		DeletionTimestamp: &deletionTime,
		Finalizers:        []string{"test.finalizer"},
	}}
	evictor := expectedPVCEvictorDeployment(deleting, pvcEvictorTarget{
		claimName: "shared-pvc",
		subPath:   "kv-cache",
	})

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{appsv1.AddToScheme, v1alpha2.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deleting, evictor).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	if err := reconciler.finalizePVCEvictors(t.Context(), deleting); err != nil {
		t.Fatalf("finalizePVCEvictors() error = %v, want nil", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(evictor), &appsv1.Deployment{}); err != nil {
		t.Fatalf("unrelated PVC evictor Deployment was removed: %v", err)
	}
}

func TestFinalizePVCEvictorsCleansDeploymentWithoutReadyStatus(t *testing.T) {
	deletionTime := metav1.Now()
	deleting := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting",
			Namespace:         "test",
			DeletionTimestamp: &deletionTime,
			Finalizers:        []string{"test.finalizer"},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{BaseRefs: []corev1.LocalObjectReference{{Name: "evictor-base"}}},
	}
	baseConfig := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "evictor-base", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName},
			}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
				PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"}},
			}}}},
		}},
	}
	evictor := expectedPVCEvictorDeployment(deleting, pvcEvictorTarget{
		claimName: "shared-pvc", subPath: "kv-cache",
		containerTemplate: &corev1.Container{Name: pvcEvictorTemplateName, Image: "example.com/evictor:preset"},
	})

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(deleting, baseConfig, evictor, testInferenceServiceConfigMap(), func() *v1alpha2.LLMInferenceServiceConfig {
			preset := loadPresetConfig(t, "config-llm-template.yaml")
			preset.Namespace = constants.KServeNamespace
			return preset
		}()).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	stored := deleting.Spec.DeepCopy()
	if err := reconciler.finalizePVCEvictors(t.Context(), deleting); err != nil {
		t.Fatalf("finalizePVCEvictors() error = %v", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(evictor), &appsv1.Deployment{}); err == nil {
		t.Fatal("PVC evictor Deployment remains after finalization")
	}
	// Reconcile removes the finalizer by updating this object, so its spec must stay as stored.
	if diff := cmp.Diff(*stored, deleting.Spec); diff != "" {
		t.Fatalf("finalizePVCEvictors() changed the stored spec (-want +got):\n%s", diff)
	}
}

func TestFinalizePVCEvictorsAllowsDeletionWhenBaseRefsCannotResolve(t *testing.T) {
	deletionTime := metav1.Now()
	deleting := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting",
			Namespace:         "test",
			DeletionTimestamp: &deletionTime,
			Finalizers:        []string{"test.finalizer"},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{BaseRefs: []corev1.LocalObjectReference{{Name: "missing-config"}}},
	}
	evictor := expectedPVCEvictorDeployment(deleting, pvcEvictorTarget{
		claimName: "shared-pvc", subPath: "kv-cache",
	})

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(deleting, evictor, testInferenceServiceConfigMap()).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	if err := reconciler.finalizePVCEvictors(t.Context(), deleting); err != nil {
		t.Fatalf("finalizePVCEvictors() error = %v, want deletion to continue", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(evictor), &appsv1.Deployment{}); err != nil {
		t.Fatalf("PVC evictor Deployment was removed while ownership was unresolved: %v", err)
	}
}

func TestFinalizePVCEvictorsAllowsDeletionWhenCleanupIsUnsafe(t *testing.T) {
	deletionTime := metav1.Now()
	deleting := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting",
			Namespace:         "test",
			DeletionTimestamp: &deletionTime,
			Finalizers:        []string{"test.finalizer"},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"},
					},
				}}},
			},
		}},
	}
	deleting.MarkPVCEvictorReady()
	unresolved := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "unresolved", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "missing-config"}},
		},
	}
	evictor := expectedPVCEvictorDeployment(deleting, pvcEvictorTarget{
		claimName: "shared-pvc",
		subPath:   "kv-cache",
	})

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deleting, unresolved, evictor, testInferenceServiceConfigMap()).
		Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	if err := reconciler.finalizePVCEvictors(t.Context(), deleting); err != nil {
		t.Fatalf("finalizePVCEvictors() error = %v, want deletion to continue", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(evictor), &appsv1.Deployment{}); err != nil {
		t.Fatalf("PVC evictor Deployment was removed while cleanup was unsafe: %v", err)
	}
}

func TestFinalizePVCEvictorsIgnoresUnsafeCleanupForUnrelatedService(t *testing.T) {
	deletionTime := metav1.Now()
	deleting := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "unrelated-deleting",
			Namespace:         "test",
			DeletionTimestamp: &deletionTime,
			Finalizers:        []string{"test.finalizer"},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
				PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"}},
			}}}},
		}},
	}
	unresolved := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "unresolved", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "missing-config"}},
		},
	}
	evictor := expectedPVCEvictorDeployment(deleting, pvcEvictorTarget{
		claimName: "shared-pvc",
		subPath:   "kv-cache",
	})

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(deleting, unresolved, evictor, testInferenceServiceConfigMap()).
		Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	if err := reconciler.finalizePVCEvictors(t.Context(), deleting); err != nil {
		t.Fatalf("finalizePVCEvictors() error = %v, want nil for unrelated service", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(evictor), &appsv1.Deployment{}); err != nil {
		t.Fatalf("PVC evictor Deployment was removed while cleanup was unsafe: %v", err)
	}
}

func TestReconcilePVCEvictorsContinuesWhenCleanupIsUnsafe(t *testing.T) {
	current := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName},
			}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"},
					},
				}}},
			},
		}},
	}
	unresolved := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "unresolved", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "missing-config"}},
		},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-pvc", Namespace: "test"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
	}

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(current, unresolved, testInferenceServiceConfigMap()).
		Build()
	pvcReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient, APIReader: pvcReader}

	if err := reconciler.reconcilePVCEvictors(t.Context(), current, &Config{}); err != nil {
		t.Fatalf("reconcilePVCEvictors() error = %v, want nil when cleanup is unsafe", err)
	}
	deployments := &appsv1.DeploymentList{}
	if err := fakeClient.List(t.Context(), deployments,
		client.InNamespace("test"),
		client.MatchingLabels{pvcEvictorLabel: "true"},
	); err != nil {
		t.Fatal(err)
	}
	if got, want := len(deployments.Items), 1; got != want {
		t.Fatalf("PVC evictor Deployment count = %d, want %d", got, want)
	}
}

func TestReconcilePVCEvictorsMarksReconcileError(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{{
				Name: mainContainerName,
				Env:  []corev1.EnvVar{{Name: kvTransferArgsEnvVar}},
			}, {Name: pvcEvictorTemplateName}}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{
						Name: "shared-pvc",
						Path: "kv-cache",
					}},
				}}},
			},
		}},
	}

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	if err := reconciler.reconcilePVCEvictors(t.Context(), svc, &Config{}); err == nil {
		t.Fatal("reconcilePVCEvictors() error = nil, want PVC API reader error")
	}
	condition := svc.Status.GetCondition(v1alpha2.PVCEvictorReady)
	if condition == nil {
		t.Fatal("PVCEvictorReady condition was not set")
	}
	if condition.Status != corev1.ConditionFalse || condition.Reason != "ReconcilePVCEvictorError" {
		t.Fatalf("PVCEvictorReady = %#v, want False/ReconcilePVCEvictorError", condition)
	}
}

func TestAttachPVCEvictorUsesContainerTemplate(t *testing.T) {
	customResources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("123m")},
	}
	customSecurityContext := &corev1.SecurityContext{
		RunAsNonRoot: ptr.To(true),
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{corev1.Capability("NET_BIND_SERVICE")},
		},
	}
	wantSecurityContext := &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Add:  []corev1.Capability{corev1.Capability("NET_BIND_SERVICE")},
			Drop: []corev1.Capability{corev1.Capability("ALL")},
		},
		RunAsNonRoot: ptr.To(true),
	}
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
			{
				Name:            pvcEvictorTemplateName,
				Image:           "example.com/evictor:v1.2.3",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         []string{"/custom-evictor"},
				Resources:       customResources,
				SecurityContext: customSecurityContext,
				Env: []corev1.EnvVar{
					{Name: "EXTRA", Value: "kept"},
					{Name: "PVC_MOUNT_PATH", Value: "wrong"},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "other", MountPath: "/other"},
					{Name: "kv-cache-secondary-0", MountPath: "/wrong"},
				},
			},
		},
	}
	secondary := []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
		PVC: &v1alpha2.PVCTierSpec{Spec: &corev1.PersistentVolumeClaimSpec{}},
	}}}

	evictorTemplate := takePVCEvictorContainerTemplate(podSpec)
	attachKVCacheSecondaryTiers(podSpec, secondary, mainContainerName, evictorTemplate)

	if got, want := len(podSpec.Containers), 2; got != want {
		t.Fatalf("container count = %d, want %d", got, want)
	}
	evictor := podSpec.Containers[1]
	if got, want := evictor.Name, "pvc-evictor-kv-cache-secondary-0"; got != want {
		t.Errorf("evictor name = %q, want %q", got, want)
	}
	if got, want := evictor.Image, "example.com/evictor:v1.2.3"; got != want {
		t.Errorf("evictor image = %q, want %q", got, want)
	}
	if got, want := evictor.ImagePullPolicy, corev1.PullIfNotPresent; got != want {
		t.Errorf("evictor pull policy = %q, want %q", got, want)
	}
	if diff := cmp.Diff(customResources, evictor.Resources); diff != "" {
		t.Errorf("evictor resources diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantSecurityContext, evictor.SecurityContext); diff != "" {
		t.Errorf("evictor security context diff (-want +got):\n%s", diff)
	}
	if len(evictor.Command) != 1 || evictor.Command[0] != "/custom-evictor" {
		t.Errorf("evictor command = %v, want [/custom-evictor]", evictor.Command)
	}
	env := map[string]string{}
	for _, variable := range evictor.Env {
		env[variable.Name] = variable.Value
	}
	if env["EXTRA"] != "kept" || env["PVC_MOUNT_PATH"] != "/mnt/kv-cache-0" || env["CACHE_DIRECTORY"] != pvcEvictorCacheDirectory {
		t.Errorf("evictor env = %v", evictor.Env)
	}
	mounts := map[string]corev1.VolumeMount{}
	for _, mount := range evictor.VolumeMounts {
		mounts[mount.Name] = mount
	}
	if mounts["other"].MountPath != "/other" {
		t.Errorf("custom evictor mount was not preserved: %v", evictor.VolumeMounts)
	}
	if mounts["kv-cache-secondary-0"].MountPath != "/mnt/kv-cache-0" {
		t.Errorf("dynamic evictor mount was not set: %v", evictor.VolumeMounts)
	}
}

func TestAttachPVCEvictorImageOnlyTemplateKeepsDefaults(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
			{Name: pvcEvictorTemplateName, Image: "example.com/evictor:v1.2.3"},
		},
	}
	secondary := []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
		PVC: &v1alpha2.PVCTierSpec{Spec: &corev1.PersistentVolumeClaimSpec{}},
	}}}

	evictorTemplate := takePVCEvictorContainerTemplate(podSpec)
	attachKVCacheSecondaryTiers(podSpec, secondary, mainContainerName, evictorTemplate)

	evictor := podSpec.Containers[1]
	if diff := cmp.Diff(pvcEvictorResources(), evictor.Resources); diff != "" {
		t.Errorf("evictor resources diff (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(pvcEvictorContainerSecurityContext(), evictor.SecurityContext); diff != "" {
		t.Errorf("evictor security context diff (-want +got):\n%s", diff)
	}
}

func TestValidatePVCEvictorCachePath(t *testing.T) {
	newService := func(env []corev1.EnvVar) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{{Name: mainContainerName, Env: env}},
					},
					KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
						Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
							PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc"}},
						}}},
					},
				},
			},
		}
	}

	if err := validatePVCEvictorContainerTemplates(newService(nil)); err != nil {
		t.Fatalf("legacy preset validation error = %v, want nil", err)
	}
	if err := validatePVCEvictorContainerTemplates(newService([]corev1.EnvVar{{Name: kvTransferArgsEnvVar}})); err != nil {
		t.Fatalf("modern preset validation error = %v", err)
	}
}

func TestPVCEvictorRequiresNewPreset(t *testing.T) {
	secondary := []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
		PVC: &v1alpha2.PVCTierSpec{Spec: &corev1.PersistentVolumeClaimSpec{}},
	}}}
	svc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}}},
				},
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{Secondary: secondary},
			},
		},
	}

	if err := validatePVCEvictorContainerTemplates(svc); err != nil {
		t.Fatalf("legacy preset validation error = %v, want nil", err)
	}
	if targets := collectPVCEvictorTargets(svc); len(targets) != 0 {
		t.Fatalf("legacy preset target count = %d, want 0", len(targets))
	}

	podSpec := svc.Spec.Template.DeepCopy()
	attachKVCacheSecondaryTiers(podSpec, secondary, mainContainerName, nil)
	if got, want := len(podSpec.Containers), 1; got != want {
		t.Fatalf("legacy preset container count = %d, want %d", got, want)
	}

	labels := addPVCEvictorConsumerLabels(nil, secondary, false)
	if len(labels) != 0 {
		t.Fatalf("legacy preset consumer labels = %v, want none", labels)
	}
}

func TestExpectedPVCEvictorDeploymentForPVCRef(t *testing.T) {
	gpuToleration := corev1.Toleration{
		Key:      "dedicated",
		Operator: corev1.TolerationOpEqual,
		Value:    "gpu",
		Effect:   corev1.TaintEffectNoSchedule,
	}
	workerToleration := corev1.Toleration{
		Key:      "accelerator",
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoSchedule,
	}
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					ServiceAccountName: "vllm-service-account",
					NodeSelector:       map[string]string{"accelerator": "gpu"},
					Tolerations:        []corev1.Toleration{gpuToleration},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:  ptr.To(int64(1000)),
						RunAsGroup: ptr.To(int64(1000)),
						FSGroup:    ptr.To(int64(1000)),
					},
					Containers: []corev1.Container{
						{Name: "main", Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
						{Name: pvcEvictorTemplateName},
					},
				},
				Worker: &corev1.PodSpec{Tolerations: []corev1.Toleration{workerToleration}},
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{
							Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache/"},
						},
					}}},
				},
			},
		},
	}

	targets := collectPVCEvictorTargets(svc)
	if len(targets) != 1 {
		t.Fatalf("PVC evictor target count = %d, want 1", len(targets))
	}
	target := targets[0]
	target.requiresNodeAffinity = true
	deployment := expectedPVCEvictorDeployment(svc, target)
	{
		if got, want := deployment.Name[:len(pvcEvictorNamePrefix)], pvcEvictorNamePrefix; got != want {
			t.Errorf("deployment name prefix = %q, want %q", got, want)
		}
		if got, want := deployment.Labels[pvcEvictorLabel], "true"; got != want {
			t.Errorf("evictor label = %q, want %q", got, want)
		}
		if got, want := deployment.Labels[pvcEvictorKeyLabel], pvcEvictorKey("shared-pvc", "kv-cache"); got != want {
			t.Errorf("evictor key label = %q, want %q", got, want)
		}
		if got, want := deployment.Spec.Selector.MatchLabels[pvcEvictorKeyLabel], pvcEvictorKey("shared-pvc", "kv-cache"); got != want {
			t.Errorf("selector evictor key = %q, want %q", got, want)
		}
		if len(deployment.OwnerReferences) != 0 {
			t.Fatalf("evictor Deployment must not have an owner reference")
		}
		if got, want := deployment.Annotations[pvcEvictorPVCName], "shared-pvc"; got != want {
			t.Errorf("PVC name annotation = %q, want %q", got, want)
		}
		if got, want := deployment.Annotations[pvcEvictorPVCPath], "kv-cache"; got != want {
			t.Errorf("PVC path annotation = %q, want %q", got, want)
		}
		if got, want := len(deployment.Spec.Template.Spec.Containers), 1; got != want {
			t.Fatalf("evictor container count = %d, want %d", got, want)
		}
		container := deployment.Spec.Template.Spec.Containers[0]
		if diff := cmp.Diff(pvcEvictorResources(), container.Resources); diff != "" {
			t.Errorf("evictor resources diff (-want +got):\n%s", diff)
		}
		if got, want := container.Env[0].Value, pvcEvictorMountPath; got != want {
			t.Errorf("PVC_MOUNT_PATH = %q, want %q", got, want)
		}
		if got, want := deployment.Spec.Template.Spec.ServiceAccountName, "kserve-llm-isvc-kv-pvc-evictor"; got != want {
			t.Errorf("service account = %q, want %q", got, want)
		}
		if deployment.Spec.Template.Spec.AutomountServiceAccountToken == nil || *deployment.Spec.Template.Spec.AutomountServiceAccountToken {
			t.Errorf("service account token automount must be disabled")
		}
		if deployment.Spec.Template.Spec.SecurityContext == nil || deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot == nil || !*deployment.Spec.Template.Spec.SecurityContext.RunAsNonRoot {
			t.Errorf("evictor PodSecurityContext must require non-root")
		}
		if got, want := *deployment.Spec.Template.Spec.SecurityContext.RunAsUser, int64(1000); got != want {
			t.Errorf("runAsUser = %d, want %d", got, want)
		}
		if got, want := *deployment.Spec.Template.Spec.SecurityContext.RunAsGroup, int64(1000); got != want {
			t.Errorf("runAsGroup = %d, want %d", got, want)
		}
		if got, want := *deployment.Spec.Template.Spec.SecurityContext.FSGroup, int64(1000); got != want {
			t.Errorf("fsGroup = %d, want %d", got, want)
		}
		if container.SecurityContext == nil || container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation {
			t.Errorf("evictor container must disallow privilege escalation")
		}
		if got, want := deployment.Spec.Template.Spec.Volumes[0].PersistentVolumeClaim.ClaimName, "shared-pvc"; got != want {
			t.Errorf("claimName = %q, want %q", got, want)
		}
		if got, want := deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].SubPath, "kv-cache"; got != want {
			t.Errorf("subPath = %q, want %q", got, want)
		}
		if diff := cmp.Diff([]corev1.Toleration{workerToleration, gpuToleration}, deployment.Spec.Template.Spec.Tolerations); diff != "" {
			t.Errorf("evictor tolerations diff (-want +got):\n%s", diff)
		}
		if got := deployment.Spec.Template.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution; len(got) != 1 {
			t.Fatalf("required pod affinity count = %d, want 1", len(got))
		} else if got, want := got[0].LabelSelector.MatchLabels[pvcEvictorConsumerLabelKey("shared-pvc", "kv-cache")], "true"; got != want {
			t.Errorf("consumer affinity label = %q, want %q", got, want)
		}
	}

	noAffinity := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:   "shared-pvc",
		subPath:     "kv-cache",
		tolerations: []corev1.Toleration{gpuToleration},
	})
	if noAffinity.Spec.Template.Spec.Affinity != nil {
		t.Fatalf("RWX evictor Deployment must not set node affinity")
	}
	if len(noAffinity.Spec.Template.Spec.Tolerations) != 0 {
		t.Fatalf("RWX evictor Deployment must not copy consumer tolerations")
	}
}

func TestExpectedPVCEvictorDeploymentUsesContainerTemplate(t *testing.T) {
	customResources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("321m")},
	}
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
						{
							Name:            pvcEvictorTemplateName,
							Image:           "example.com/evictor:v1.2.3",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Resources:       customResources,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "other", MountPath: "/other"},
							},
						},
					},
				},
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc"}},
					}}},
				},
			},
		},
	}

	targets := collectPVCEvictorTargets(svc)
	if len(targets) != 1 {
		t.Fatalf("PVC evictor target count = %d, want 1", len(targets))
	}
	deployment := expectedPVCEvictorDeployment(svc, targets[0])
	container := deployment.Spec.Template.Spec.Containers[0]
	if got, want := container.Image, "example.com/evictor:v1.2.3"; got != want {
		t.Errorf("evictor image = %q, want %q", got, want)
	}
	if got, want := container.ImagePullPolicy, corev1.PullIfNotPresent; got != want {
		t.Errorf("evictor pull policy = %q, want %q", got, want)
	}
	if diff := cmp.Diff(customResources, container.Resources); diff != "" {
		t.Errorf("evictor resources diff (-want +got):\n%s", diff)
	}
	if container.Name != "pvc-evictor" {
		t.Errorf("evictor name = %q, want pvc-evictor", container.Name)
	}
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].Name != "kv-cache-storage" {
		t.Errorf("standalone evictor volume mounts = %v, want only kv-cache-storage", container.VolumeMounts)
	}
}

func TestPVCEvictorDeploymentPredicateIncludesStatusUpdates(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	oldDeployment := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{claimName: "shared-pvc", subPath: "kv-cache"})
	newDeployment := oldDeployment.DeepCopy()
	newDeployment.Status.AvailableReplicas = 1

	if !pvcEvictorDeploymentPredicate().Update(event.UpdateEvent{ObjectOld: oldDeployment, ObjectNew: newDeployment}) {
		t.Fatal("status-only evictor Deployment update was filtered")
	}
}

func TestPVCEvictorServiceAccountPredicateIncludesLabelRemoval(t *testing.T) {
	oldServiceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{pvcEvictorLabel: "true"},
	}}
	newServiceAccount := oldServiceAccount.DeepCopy()
	newServiceAccount.Labels = nil

	if !pvcEvictorServiceAccountPredicate().Update(event.UpdateEvent{
		ObjectOld: oldServiceAccount,
		ObjectNew: newServiceAccount,
	}) {
		t.Fatal("ServiceAccount label removal was filtered")
	}
}

func TestMarkPVCEvictorReadinessMarksUnavailableDeployment(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	svc.MarkPVCEvictorReady()
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{claimName: "shared-pvc", subPath: "kv-cache"})
	current := expected.DeepCopy()
	current.Status.Conditions = []appsv1.DeploymentCondition{{
		Type:    appsv1.DeploymentAvailable,
		Status:  corev1.ConditionFalse,
		Message: "pods are unavailable",
	}}

	markPVCEvictorReadiness(svc, map[string]*appsv1.Deployment{expected.Name: expected}, []appsv1.Deployment{*current})

	condition := svc.Status.GetCondition(v1alpha2.PVCEvictorReady)
	if condition == nil {
		t.Fatal("PVCEvictorReady condition was not set")
	}
	if condition.Status != corev1.ConditionFalse || condition.Reason != "PVCEvictorNotReady" {
		t.Fatalf("PVCEvictorReady = %#v, want False/PVCEvictorNotReady", condition)
	}
}

func TestMarkPVCEvictorReadinessMarksReadyAndUnsets(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{claimName: "shared-pvc", subPath: "kv-cache"})
	current := expected.DeepCopy()
	current.Status.AvailableReplicas = 1

	markPVCEvictorReadiness(svc, map[string]*appsv1.Deployment{expected.Name: expected}, []appsv1.Deployment{*current})
	condition := svc.Status.GetCondition(v1alpha2.PVCEvictorReady)
	if condition == nil || condition.Status != corev1.ConditionTrue {
		t.Fatalf("PVCEvictorReady = %#v, want True", condition)
	}

	markPVCEvictorReadiness(svc, nil, nil)
	if condition := svc.Status.GetCondition(v1alpha2.PVCEvictorReady); condition != nil {
		t.Fatalf("PVCEvictorReady = %#v, want unset", condition)
	}
}

func TestEnsurePVCEvictorServiceAccountRepairsTokenAutomount(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	serviceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:      pvcEvictorServiceAccount,
		Namespace: "test",
		Labels:    map[string]string{pvcEvictorLabel: "true"},
	}}
	defaultServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "test",
		},
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-pull"}},
		Secrets:          []corev1.ObjectReference{{Name: "regular-secret"}},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(serviceAccount, defaultServiceAccount).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	if err := reconciler.ensurePVCEvictorServiceAccount(t.Context(), "test"); err != nil {
		t.Fatalf("ensurePVCEvictorServiceAccount() error = %v", err)
	}
	updated := &corev1.ServiceAccount{}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(serviceAccount), updated); err != nil {
		t.Fatal(err)
	}
	if updated.AutomountServiceAccountToken == nil || *updated.AutomountServiceAccountToken {
		t.Fatal("PVC evictor ServiceAccount token automount was not disabled")
	}
	if len(updated.ImagePullSecrets) != 1 || updated.ImagePullSecrets[0].Name != "registry-pull" {
		t.Fatalf("imagePullSecrets = %#v, want registry-pull", updated.ImagePullSecrets)
	}
	if len(updated.Secrets) != 0 {
		t.Fatalf("secrets = %#v, want no regular secrets", updated.Secrets)
	}
}

func TestReconcileExistingPVCEvictorDeploymentRepairsDrift(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName: "shared-pvc", subPath: "kv-cache",
		containerTemplate: &corev1.Container{Name: pvcEvictorTemplateName, Image: "example.com/evictor:preset"},
	})
	current := expected.DeepCopy()
	current.Spec.Template.Spec.Containers[0].Image = "quay.io/example/old-evictor:old"
	current.Labels[pvcEvictorKeyLabel] = "changed"
	current.Annotations[pvcEvictorSecurityHash] = "stale"
	current.Labels["example.com/keep"] = "label"
	current.Annotations["example.com/keep"] = "annotation"
	current.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "serving.kserve.io/v1alpha2",
		Kind:       "LLMInferenceService",
		Name:       "llm",
	}}

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	agreements := map[string]pvcEvictorTemplateAgreement{
		pvcEvictorIdentity("shared-pvc", "kv-cache"): {agreed: true},
	}
	if err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), agreements, current, expected); err != nil {
		t.Fatalf("reconcileExistingPVCEvictorDeployment() error = %v", err)
	}

	got := &appsv1.Deployment{}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), got); err != nil {
		t.Fatalf("get repaired Deployment: %v", err)
	}
	if got.Spec.Template.Spec.Containers[0].Image != "example.com/evictor:preset" {
		t.Fatalf("image = %q, want preset image", got.Spec.Template.Spec.Containers[0].Image)
	}
	if got.Labels[pvcEvictorKeyLabel] != expected.Labels[pvcEvictorKeyLabel] {
		t.Fatalf("key label = %q, want %q", got.Labels[pvcEvictorKeyLabel], expected.Labels[pvcEvictorKeyLabel])
	}
	if len(got.OwnerReferences) != 0 {
		t.Fatalf("owner references = %v, want none", got.OwnerReferences)
	}
	if got.Labels["example.com/keep"] != "label" || got.Annotations["example.com/keep"] != "annotation" {
		t.Fatalf("unmanaged metadata was not preserved: labels=%v annotations=%v", got.Labels, got.Annotations)
	}
	if got.Annotations[pvcEvictorSecurityHash] != expected.Annotations[pvcEvictorSecurityHash] {
		t.Fatalf("security hash = %q, want effective default hash %q", got.Annotations[pvcEvictorSecurityHash], expected.Annotations[pvcEvictorSecurityHash])
	}
}

func TestReconcileExistingPVCEvictorDeploymentInitializesLabels(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{claimName: "shared-pvc", subPath: "kv-cache"})
	current := expected.DeepCopy()
	current.Labels = nil

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	agreements := map[string]pvcEvictorTemplateAgreement{
		pvcEvictorIdentity("shared-pvc", "kv-cache"): {agreed: true},
	}

	if err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), agreements, current, expected); err != nil {
		t.Fatalf("reconcileExistingPVCEvictorDeployment() error = %v", err)
	}
	got := &appsv1.Deployment{}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), got); err != nil {
		t.Fatal(err)
	}
	if got.Labels[pvcEvictorLabel] != "true" || got.Labels[pvcEvictorKeyLabel] == "" {
		t.Fatalf("labels = %#v, want managed PVC evictor labels", got.Labels)
	}
}

func TestPVCEvictorSecurityContextHashUsesEffectiveDefaults(t *testing.T) {
	if got, want := pvcEvictorSecurityContextHash(nil), pvcEvictorSecurityContextHash(&corev1.PodSecurityContext{}); got != want {
		t.Fatalf("default security context hashes differ: nil=%q explicit=%q", got, want)
	}
	if got, want := pvcEvictorSecurityContextHash(nil), pvcEvictorSecurityContextHash(&corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(false),
	}); got != want {
		t.Fatalf("defaulted security context hashes differ: nil=%q explicit=%q", got, want)
	}
}

func TestReconcileExistingPVCEvictorDeploymentAppliesTolerationsDuringSecurityHashMigration(t *testing.T) {
	toleration := corev1.Toleration{Key: "admission", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:            "shared-pvc",
		subPath:              "kv-cache",
		requiresNodeAffinity: true,
		tolerations:          []corev1.Toleration{toleration},
	})
	current := expected.DeepCopy()
	delete(current.Annotations, pvcEvictorSecurityHash)
	current.Spec.Template.Spec.Tolerations = nil

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	agreements := map[string]pvcEvictorTemplateAgreement{
		pvcEvictorIdentity("shared-pvc", "kv-cache"): {
			agreed:      true,
			unresolved:  true,
			tolerations: []corev1.Toleration{toleration},
		},
	}

	if err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), agreements, current, expected); err != nil {
		t.Fatalf("reconcileExistingPVCEvictorDeployment() error = %v", err)
	}
	got := &appsv1.Deployment{}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), got); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]corev1.Toleration{toleration}, got.Spec.Template.Spec.Tolerations); diff != "" {
		t.Errorf("tolerations diff (-want +got):\n%s", diff)
	}
}

func TestReconcileExistingPVCEvictorDeploymentSkipsNoOpUpdate(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{claimName: "shared-pvc", subPath: "kv-cache"})
	current := expected.DeepCopy()
	nonDryRunUpdates := 0

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(current).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updateOptions := &client.UpdateOptions{}
				for _, opt := range opts {
					opt.ApplyToUpdate(updateOptions)
				}
				if len(updateOptions.DryRun) == 0 {
					nonDryRunUpdates++
				}
				return c.Update(ctx, obj, opts...)
			},
		}).
		Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	if err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), nil, current, expected); err != nil {
		t.Fatalf("reconcileExistingPVCEvictorDeployment() error = %v", err)
	}
	if nonDryRunUpdates != 0 {
		t.Fatalf("non-dry-run updates = %d, want 0", nonDryRunUpdates)
	}
}

func TestReconcileExistingPVCEvictorDeploymentRejectsSecurityDrift(t *testing.T) {
	newService := func(name string, fsGroup int64) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test"},
			Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(fsGroup)},
					Containers:      []corev1.Container{{Name: "main"}},
				},
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{
							Name: "shared-pvc", Path: "kv-cache",
						}},
					}}},
				},
			}},
		}
	}
	svc := newService("llm", 1000)
	other := newService("other", 2000)
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:       "shared-pvc",
		subPath:         "kv-cache",
		securityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000))},
	})
	current := expected.DeepCopy()
	current.Spec.Template.Spec.SecurityContext.FSGroup = ptr.To(int64(2000))
	current.Annotations[pvcEvictorSecurityHash] = pvcEvictorSecurityContextHash(current.Spec.Template.Spec.SecurityContext)
	preset := loadPresetConfig(t, "config-llm-template.yaml")
	preset.Namespace = "kserve"

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{appsv1.AddToScheme, v1alpha2.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, other, preset, current).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	agreements := map[string]pvcEvictorTemplateAgreement{
		pvcEvictorIdentity("shared-pvc", "kv-cache"): {agreed: false},
	}
	err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), agreements, current, expected)
	if err == nil {
		t.Fatal("security drift was silently overwritten")
	}
	if !errors.Is(err, reconcile.TerminalError(nil)) {
		t.Fatalf("security drift error = %v, want terminal error", err)
	}
}

func TestReconcileExistingPVCEvictorDeploymentUpdatesNodeHashWhenUnresolved(t *testing.T) {
	service := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(service, pvcEvictorTarget{
		claimName:            "shared-pvc",
		subPath:              "kv-cache",
		containerTemplate:    &corev1.Container{Name: pvcEvictorTemplateName, Image: "example.com/new"},
		requiresNodeAffinity: true,
	})
	current := expected.DeepCopy()
	current.Spec.Template.Spec.Containers[0].Image = "example.com/old"
	current.Annotations[pvcEvictorContainerHash] = pvcEvictorContainerTemplateHash(
		&corev1.Container{Name: pvcEvictorTemplateName, Image: "example.com/old"},
	)
	if current.Spec.Template.Annotations == nil {
		current.Spec.Template.Annotations = make(map[string]string)
	}
	current.Spec.Template.Annotations[pvcEvictorConsumerNodeHash] = "old-hash"

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	agreements := map[string]pvcEvictorTemplateAgreement{
		pvcEvictorIdentity("shared-pvc", "kv-cache"): {
			agreed:           true,
			unresolved:       true,
			consumerNodeHash: "new-hash",
		},
	}

	if err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), agreements, current, expected); err != nil {
		t.Fatalf("reconcileExistingPVCEvictorDeployment() error = %v", err)
	}
	updated := &appsv1.Deployment{}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(current), updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Spec.Template.Annotations[pvcEvictorConsumerNodeHash]; got != "new-hash" {
		t.Fatalf("consumer node hash = %q, want %q", got, "new-hash")
	}
	if got := updated.Spec.Template.Spec.Containers[0].Image; got != "example.com/old" {
		t.Fatalf("container image = %q, want preserved existing image", got)
	}
}

func TestReconcileExistingPVCEvictorDeploymentUpdatesAgreedSecurityContext(t *testing.T) {
	newService := func(name string) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test"},
			Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000))},
					Containers:      []corev1.Container{{Name: "main"}},
				},
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{
							Name: "shared-pvc", Path: "kv-cache",
						}},
					}}},
				},
			}},
		}
	}
	svc := newService("llm")
	other := newService("other")
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:       "shared-pvc",
		subPath:         "kv-cache",
		securityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000))},
	})
	current := expected.DeepCopy()
	current.Spec.Template.Spec.SecurityContext.FSGroup = ptr.To(int64(2000))
	current.Annotations[pvcEvictorSecurityHash] = pvcEvictorSecurityContextHash(current.Spec.Template.Spec.SecurityContext)
	preset := loadPresetConfig(t, "config-llm-template.yaml")
	preset.Namespace = "kserve"

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{appsv1.AddToScheme, v1alpha2.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, other, preset, current).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	agreements := map[string]pvcEvictorTemplateAgreement{
		pvcEvictorIdentity("shared-pvc", "kv-cache"): {agreed: true},
	}
	if err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), agreements, current, expected); err != nil {
		t.Fatalf("reconcileExistingPVCEvictorDeployment() error = %v", err)
	}

	got := &appsv1.Deployment{}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Template.Spec.SecurityContext.FSGroup == nil || *got.Spec.Template.Spec.SecurityContext.FSGroup != 1000 {
		t.Fatalf("fsGroup = %v, want 1000", got.Spec.Template.Spec.SecurityContext.FSGroup)
	}
	if got.Annotations[pvcEvictorSecurityHash] != expected.Annotations[pvcEvictorSecurityHash] {
		t.Fatalf("security hash = %q, want %q", got.Annotations[pvcEvictorSecurityHash], expected.Annotations[pvcEvictorSecurityHash])
	}
}

func TestReconcileExistingPVCEvictorDeploymentChecksConflictsBeforeUnresolvedPause(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName: "shared-pvc",
		subPath:   "kv-cache",
	})
	current := expected.DeepCopy()
	current.Spec.Template.Spec.SecurityContext.FSGroup = ptr.To(int64(2000))
	current.Annotations[pvcEvictorSecurityHash] = pvcEvictorSecurityContextHash(current.Spec.Template.Spec.SecurityContext)

	tests := []struct {
		name      string
		agreement pvcEvictorTemplateAgreement
		wantErr   bool
	}{
		{
			name:      "resolved conflict is reported even when another service is unresolved",
			agreement: pvcEvictorTemplateAgreement{agreed: false, unresolved: true},
			wantErr:   true,
		},
		{
			name:      "agreed deployment is preserved while another service is unresolved",
			agreement: pvcEvictorTemplateAgreement{agreed: true, unresolved: true},
			wantErr:   false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := appsv1.AddToScheme(scheme); err != nil {
				t.Fatal(err)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current.DeepCopy()).Build()
			reconciler := &LLMISVCReconciler{Client: fakeClient}
			agreements := map[string]pvcEvictorTemplateAgreement{
				pvcEvictorIdentity("shared-pvc", "kv-cache"): test.agreement,
			}

			err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), agreements, current.DeepCopy(), expected)
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("reconcileExistingPVCEvictorDeployment() error = %v, want error = %v", err, test.wantErr)
			}

			got := &appsv1.Deployment{}
			if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), got); err != nil {
				t.Fatal(err)
			}
			if got.Spec.Template.Spec.SecurityContext.FSGroup == nil || *got.Spec.Template.Spec.SecurityContext.FSGroup != 2000 {
				t.Fatalf("fsGroup = %v, want the existing Deployment to be preserved", got.Spec.Template.Spec.SecurityContext.FSGroup)
			}
		})
	}
}

func TestDesiredPVCEvictorTemplateAgreementsMarksUnresolved(t *testing.T) {
	admittedToleration := corev1.Toleration{Key: "admission", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
	current := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName},
			}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{
						Name: "shared-pvc", Path: "kv-cache",
					}},
				}}},
			},
		}},
	}
	unresolved := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "unresolved", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "missing-config"}},
		},
	}

	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	consumerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "consumer",
			Namespace: "test",
			Labels: map[string]string{
				pvcEvictorConsumerLabelKey("shared-pvc", "kv-cache"): "true",
			},
		},
		Spec: corev1.PodSpec{
			NodeName:    "node-a",
			Tolerations: []corev1.Toleration{admittedToleration},
		},
	}
	terminatingPod := consumerPod.DeepCopy()
	terminatingPod.Name = "terminating-consumer"
	terminatingPod.Finalizers = []string{"test.finalizer"}
	deletionTime := metav1.Now()
	terminatingPod.DeletionTimestamp = &deletionTime
	terminatingPod.Spec.Tolerations = []corev1.Toleration{{Key: "stale", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current, unresolved, consumerPod, terminatingPod).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	agreements, err := reconciler.desiredPVCEvictorTemplateAgreements(t.Context(), current, &Config{})
	if err != nil {
		t.Fatalf("desiredPVCEvictorTemplateAgreements() error = %v", err)
	}
	agreement, exists := agreements[pvcEvictorIdentity("shared-pvc", "kv-cache")]
	if !exists {
		t.Fatalf("agreements = %#v, want an entry for shared-pvc/kv-cache", agreements)
	}
	if !agreement.agreed || !agreement.unresolved {
		t.Fatalf("agreement = %#v, want agreed and unresolved to be true", agreement)
	}
	if diff := cmp.Diff([]corev1.Toleration{admittedToleration}, agreement.tolerations); diff != "" {
		t.Errorf("admitted tolerations diff (-want +got):\n%s", diff)
	}
	if got, want := agreement.consumerNodeHash, pvcEvictorConsumerNodeHashValue([]string{"node-a"}); got != want {
		t.Errorf("consumer node hash = %q, want %q", got, want)
	}
}

func TestReconcileExistingPVCEvictorDeploymentPreservesAndAddsTolerationsWhenUnresolved(t *testing.T) {
	currentToleration := corev1.Toleration{Key: "current", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
	desiredToleration := corev1.Toleration{Key: "desired", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:            "shared-pvc",
		subPath:              "kv-cache",
		requiresNodeAffinity: true,
		tolerations:          []corev1.Toleration{desiredToleration},
	})
	current := expected.DeepCopy()
	current.Spec.Template.Spec.Tolerations = []corev1.Toleration{currentToleration}

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	agreements := map[string]pvcEvictorTemplateAgreement{
		pvcEvictorIdentity("shared-pvc", "kv-cache"): {
			agreed:      true,
			unresolved:  true,
			tolerations: []corev1.Toleration{desiredToleration},
		},
	}

	if err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), agreements, current, expected); err != nil {
		t.Fatalf("reconcileExistingPVCEvictorDeployment() error = %v", err)
	}
	got := &appsv1.Deployment{}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), got); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]corev1.Toleration{currentToleration, desiredToleration}, got.Spec.Template.Spec.Tolerations); diff != "" {
		t.Errorf("tolerations diff (-want +got):\n%s", diff)
	}
}

func TestMergePVCEvictorTolerationsIsStable(t *testing.T) {
	first := corev1.Toleration{Key: "first", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
	second := corev1.Toleration{Key: "second", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}

	fromFirstOrder := mergePVCEvictorTolerations(nil, first, second, first)
	fromSecondOrder := mergePVCEvictorTolerations(nil, second, first, second)
	if diff := cmp.Diff(fromFirstOrder, fromSecondOrder); diff != "" {
		t.Fatalf("merge order is not stable (-first order +second order):\n%s", diff)
	}
	if got, want := len(fromFirstOrder), 2; got != want {
		t.Fatalf("merged toleration count = %d, want %d", got, want)
	}
}

func TestCollectPVCEvictorTargetsMergesDuplicateIdentityTolerations(t *testing.T) {
	first := corev1.Toleration{Key: "first", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
	second := corev1.Toleration{Key: "second", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}
	svc := &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{
		WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{
				Tolerations: []corev1.Toleration{first},
				Containers: []corev1.Container{
					{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
					{Name: pvcEvictorTemplateName},
				},
			},
			Worker: &corev1.PodSpec{Tolerations: []corev1.Toleration{second}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{Secondary: []v1alpha2.SecondaryTierSpec{{
				FileSystem: &v1alpha2.FileSystemTierSpec{PVC: &v1alpha2.PVCTierSpec{
					Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "cache"},
				}},
			}}},
		},
	}}

	targets := collectPVCEvictorTargets(svc)
	if len(targets) != 1 {
		t.Fatalf("target count = %d, want 1", len(targets))
	}
	if diff := cmp.Diff(mergePVCEvictorTolerations(nil, first, second), targets[0].tolerations); diff != "" {
		t.Errorf("duplicate identity tolerations diff (-want +got):\n%s", diff)
	}
}

func TestPVCEvictorConsumerPodPredicate(t *testing.T) {
	predicate := pvcEvictorConsumerPodPredicate()
	consumer := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		pvcEvictorConsumerLabelKey("shared-pvc", "cache"): "true",
	}}}
	nonConsumer := &corev1.Pod{}
	if !predicate.Create(event.CreateEvent{Object: consumer}) {
		t.Fatal("consumer Pod create was not accepted")
	}
	if predicate.Create(event.CreateEvent{Object: nonConsumer}) {
		t.Fatal("non-consumer Pod create was accepted")
	}
	updated := consumer.DeepCopy()
	updated.Spec.Tolerations = []corev1.Toleration{{Key: "admission", Operator: corev1.TolerationOpExists}}
	if !predicate.Update(event.UpdateEvent{ObjectOld: consumer, ObjectNew: updated}) {
		t.Fatal("consumer Pod toleration update was not accepted")
	}
	updated = consumer.DeepCopy()
	updated.Spec.NodeName = "node-b"
	if !predicate.Update(event.UpdateEvent{ObjectOld: consumer, ObjectNew: updated}) {
		t.Fatal("consumer Pod node movement was not accepted")
	}
}

func TestPVCEvictorServiceChangePredicate(t *testing.T) {
	predicate := pvcEvictorServiceChangePredicate()
	oldService := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Generation: 1}}
	newService := oldService.DeepCopy()
	if predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("unchanged service was accepted")
	}
	newService.Generation = 2
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("generation change was not accepted")
	}
	newService = oldService.DeepCopy()
	newService.Annotations = map[string]string{constants.StopAnnotationKey: "true"}
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("force-stop change was not accepted")
	}
}

func TestPVCEvictorPVCPredicate(t *testing.T) {
	predicate := pvcEvictorPVCPredicate()
	oldPVC := &corev1.PersistentVolumeClaim{}
	if predicate.Update(event.UpdateEvent{ObjectOld: oldPVC, ObjectNew: oldPVC.DeepCopy()}) {
		t.Fatal("unchanged PVC was accepted")
	}
	updatedPVC := oldPVC.DeepCopy()
	updatedPVC.Status.Phase = corev1.ClaimBound
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldPVC, ObjectNew: updatedPVC}) {
		t.Fatal("PVC status change was not accepted")
	}
}

func TestCollectPVCEvictorPVCNamesDoesNotRequireRenderedTemplate(t *testing.T) {
	service := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc"}},
					}}},
				},
			},
		},
	}

	if got, want := collectPVCEvictorPVCNames(service), []string{"shared-pvc"}; !cmp.Equal(got, want) {
		t.Fatalf("PVC names = %v, want %v", got, want)
	}
}

func TestEnqueueOnPVCEvictorPVCChangeTargetsDirectAndBaseRefServices(t *testing.T) {
	pvcRefWorkload := func() v1alpha2.WorkloadSpec {
		return v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{{
				Name: mainContainerName,
				Env:  []corev1.EnvVar{{Name: kvTransferArgsEnvVar}},
			}}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "cache"}},
				}}},
			},
		}
	}
	direct := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "direct", Namespace: "test"},
		Spec:       v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: pvcRefWorkload()},
	}
	baseConfig := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "base-cache", Namespace: "test"},
		Spec:       v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: pvcRefWorkload()},
	}
	implicitConfig := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "implicit-cache", Namespace: constants.KServeNamespace},
		Spec:       v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: pvcRefWorkload()},
		Status: v1alpha2.LLMInferenceServiceConfigStatus{ReferencedBy: []v1alpha2.UntypedObjectReference{{
			Name: "implicit", Namespace: "test",
		}}},
	}
	baseRef := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "base-ref", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: baseConfig.Name}},
		},
	}
	unrelated := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: "test"},
	}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "shared-pvc", Namespace: "test"}}

	scheme := newControllerHandlersScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		testInferenceServiceConfigMap(), direct, baseConfig, implicitConfig, baseRef, unrelated,
	).
		WithIndex(&v1alpha2.LLMInferenceService{}, pvcEvictorPVCIndex, func(object client.Object) []string {
			return collectPVCEvictorPVCNames(object.(*v1alpha2.LLMInferenceService))
		}).
		WithIndex(&v1alpha2.LLMInferenceService{}, pvcEvictorBaseRefsIndex, func(object client.Object) []string {
			if len(object.(*v1alpha2.LLMInferenceService).Spec.BaseRefs) == 0 {
				return nil
			}
			return []string{"true"}
		}).
		Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	requests := enqueueRequestsForObject(reconciler.enqueueOnPVCEvictorPVCChange(), pvc)
	got := make(map[string]struct{}, len(requests))
	for _, request := range requests {
		got[request.Namespace+"/"+request.Name] = struct{}{}
	}
	for _, name := range []string{"direct", "base-ref", "implicit"} {
		if _, ok := got["test/"+name]; !ok {
			t.Fatalf("PVC event did not enqueue test/%s; requests = %v", name, requests)
		}
	}
	if _, ok := got["test/unrelated"]; ok {
		t.Fatalf("PVC event enqueued unrelated service: %v", requests)
	}
}

func TestReconcileExistingPVCEvictorDeploymentRejectsContainerTemplateDrift(t *testing.T) {
	newService := func(name, image string) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test"},
			Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
						{Name: pvcEvictorTemplateName, Image: image},
					},
				},
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{
							Name: "shared-pvc", Path: "kv-cache",
						}},
					}}},
				},
			}},
		}
	}
	svc := newService("llm", "example.com/evictor:a")
	other := newService("other", "example.com/evictor:b")
	targets := collectPVCEvictorTargets(svc)
	if len(targets) != 1 {
		t.Fatalf("PVC evictor target count = %d, want 1", len(targets))
	}
	expected := expectedPVCEvictorDeployment(svc, targets[0])
	current := expected.DeepCopy()
	current.Spec.Template.Spec.Containers[0].Image = "example.com/evictor:b"
	current.Annotations[pvcEvictorContainerHash] = pvcEvictorContainerTemplateHash(
		&corev1.Container{Name: pvcEvictorTemplateName, Image: "example.com/evictor:b"},
	)
	preset := loadPresetConfig(t, "config-llm-template.yaml")
	preset.Namespace = "kserve"

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{appsv1.AddToScheme, v1alpha2.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, other, preset, current).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	agreements := map[string]pvcEvictorTemplateAgreement{
		pvcEvictorIdentity("shared-pvc", "kv-cache"): {agreed: false},
	}
	err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), agreements, current, expected)
	if err == nil {
		t.Fatal("container template drift was silently overwritten")
	}
	if !strings.Contains(err.Error(), "pvc-evictor container template") {
		t.Fatalf("error = %v, want container template conflict", err)
	}
}

func TestValidatePVCEvictorContainerTemplates(t *testing.T) {
	newWorkload := func(image string, resources corev1.ResourceRequirements, pvcRef bool) *v1alpha2.WorkloadSpec {
		tier := v1alpha2.SecondaryTierSpec{FileSystem: &v1alpha2.FileSystemTierSpec{
			PVC: &v1alpha2.PVCTierSpec{Spec: &corev1.PersistentVolumeClaimSpec{}},
		}}
		if pvcRef {
			tier.FileSystem.PVC = &v1alpha2.PVCTierSpec{
				Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"},
			}
		}
		return &v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
					{Name: pvcEvictorTemplateName, Image: image, Resources: resources},
				},
			},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{tier},
			},
		}
	}
	newRefWorkload := func(image string, resources corev1.ResourceRequirements, name, path string) *v1alpha2.WorkloadSpec {
		return &v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main", Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
					{Name: pvcEvictorTemplateName, Image: image, Resources: resources},
				},
			},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{
					FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{
							Ref: &v1alpha2.PVCRefTierSpec{Name: name, Path: path},
						},
					},
				}},
			},
		}
	}

	t.Run("same image with different resources is allowed for pvc.spec", func(t *testing.T) {
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *newWorkload("example.com/evictor:v1", corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				}, false),
				Prefill: newWorkload("example.com/evictor:v1", corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
				}, false),
			},
		}
		if err := validatePVCEvictorContainerTemplates(svc); err != nil {
			t.Fatalf("validatePVCEvictorContainerTemplates() error = %v", err)
		}
	})

	t.Run("different security contexts on the same pvc.ref path are rejected", func(t *testing.T) {
		main := newRefWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, "shared-pvc", "kv")
		prefill := newRefWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, "shared-pvc", "kv")
		main.Template.SecurityContext = &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000))}
		prefill.Template.SecurityContext = &corev1.PodSecurityContext{FSGroup: ptr.To(int64(2000))}
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *main,
				Prefill:      prefill,
			},
		}
		err := validatePVCEvictorContainerTemplates(svc)
		if err == nil {
			t.Fatal("different PodSecurityContexts on the same pvc.ref path were accepted")
		}
		if !strings.Contains(err.Error(), "same vLLM PodSecurityContext") {
			t.Fatalf("error = %v, want PodSecurityContext conflict", err)
		}
	})

	t.Run("different template and worker security contexts on the same pvc.ref path are rejected", func(t *testing.T) {
		workload := newRefWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, "shared-pvc", "kv")
		workload.Template.SecurityContext = &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000))}
		workload.Worker = &corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(2000))},
			Containers: []corev1.Container{{
				Name: mainContainerName,
				Env:  []corev1.EnvVar{{Name: kvTransferArgsEnvVar}},
			}},
		}
		svc := &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: *workload}}
		err := validatePVCEvictorContainerTemplates(svc)
		if err == nil {
			t.Fatal("different template and worker PodSecurityContexts were accepted")
		}
		if !strings.Contains(err.Error(), "same vLLM PodSecurityContext") {
			t.Fatalf("error = %v, want PodSecurityContext conflict", err)
		}
	})

	t.Run("legacy workloads ignore different security contexts", func(t *testing.T) {
		main := &v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000))},
				Containers:      []corev1.Container{{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}}},
			},
			Worker: &corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(2000))},
				Containers:      []corev1.Container{{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}}},
			},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
				PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv"}},
			}}}},
		}
		if err := validatePVCEvictorContainerTemplates(&v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: *main},
		}); err != nil {
			t.Fatalf("legacy workload security-context differences were rejected: %v", err)
		}
	})

	t.Run("different images on separate pvc.spec workloads are allowed", func(t *testing.T) {
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *newWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, false),
				Prefill:      newWorkload("example.com/evictor:v2", corev1.ResourceRequirements{}, false),
			},
		}
		if err := validatePVCEvictorContainerTemplates(svc); err != nil {
			t.Fatalf("validatePVCEvictorContainerTemplates() error = %v", err)
		}
	})

	t.Run("different images on separate pvc.ref PVCs are allowed", func(t *testing.T) {
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *newRefWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, "pvc-a", "kv"),
				Prefill:      newRefWorkload("example.com/evictor:v2", corev1.ResourceRequirements{}, "pvc-b", "kv"),
			},
		}
		if err := validatePVCEvictorContainerTemplates(svc); err != nil {
			t.Fatalf("validatePVCEvictorContainerTemplates() error = %v", err)
		}
	})

	t.Run("different images on separate pvc.ref paths are allowed", func(t *testing.T) {
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *newRefWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, "shared-pvc", "path-a"),
				Prefill:      newRefWorkload("example.com/evictor:v2", corev1.ResourceRequirements{}, "shared-pvc", "path-b"),
			},
		}
		if err := validatePVCEvictorContainerTemplates(svc); err != nil {
			t.Fatalf("validatePVCEvictorContainerTemplates() error = %v", err)
		}
	})

	t.Run("different images on the same pvc.ref path are rejected", func(t *testing.T) {
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *newRefWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, "shared-pvc", "kv"),
				Prefill:      newRefWorkload("example.com/evictor:v2", corev1.ResourceRequirements{}, "shared-pvc", "kv"),
			},
		}
		err := validatePVCEvictorContainerTemplates(svc)
		if err == nil {
			t.Fatal("different evictor images on the same pvc.ref path were accepted")
		}
		if !strings.Contains(err.Error(), "same pvc-evictor container template") {
			t.Fatalf("error = %v, want pvc.ref template conflict", err)
		}
	})

	t.Run("a legacy main workload conflicts with a new prefill workload on the same pvc.ref path", func(t *testing.T) {
		main := newRefWorkload("", corev1.ResourceRequirements{}, "shared-pvc", "kv")
		main.Template.Containers = []corev1.Container{{Name: "main", Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}}}
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *main,
				Prefill:      newRefWorkload("example.com/evictor:v2", corev1.ResourceRequirements{}, "shared-pvc", "kv"),
			},
		}
		err := validatePVCEvictorContainerTemplates(svc)
		if err == nil {
			t.Fatal("a legacy main workload and a new prefill workload were accepted")
		}
		if !strings.Contains(err.Error(), "same KV cache root") {
			t.Fatalf("error = %v, want KV cache root conflict", err)
		}
	})

	t.Run("a modern main workload cannot inherit into a legacy prefill preset", func(t *testing.T) {
		prefill := newWorkload("", corev1.ResourceRequirements{}, false)
		prefill.Template.Containers = []corev1.Container{{Name: "main"}}
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *newWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, false),
				Prefill:      prefill,
			},
		}
		err := validatePVCEvictorContainerTemplates(svc)
		if err == nil {
			t.Fatal("a modern main workload inherited the pvc-evictor into a legacy prefill preset")
		}
		if !strings.Contains(err.Error(), "prefill workload uses a legacy KV-cache preset") {
			t.Fatalf("error = %v, want legacy prefill preset rejection", err)
		}
	})

	t.Run("prefill inherits the main template on the same pvc.ref path", func(t *testing.T) {
		prefill := newRefWorkload("", corev1.ResourceRequirements{}, "shared-pvc", "kv")
		prefill.Template.Containers = []corev1.Container{{Name: "main", Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}}}
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *newRefWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, "shared-pvc", "kv"),
				Prefill:      prefill,
			},
		}
		if err := validatePVCEvictorContainerTemplates(svc); err != nil {
			t.Fatalf("validatePVCEvictorContainerTemplates() error = %v", err)
		}
		targets := collectPVCEvictorTargets(svc)
		if len(targets) != 1 {
			t.Fatalf("PVC evictor target count = %d, want 1", len(targets))
		}
		if targets[0].containerTemplate == nil || targets[0].containerTemplate.Image != "example.com/evictor:v1" {
			t.Fatalf("prefill did not inherit main template: %#v", targets[0].containerTemplate)
		}
	})

	t.Run("prefill inherits the main template for a prefill-only pvc.ref", func(t *testing.T) {
		main := newRefWorkload("example.com/evictor:v1", corev1.ResourceRequirements{}, "unused-pvc", "unused-path")
		main.KVCacheOffloading = nil
		prefill := newRefWorkload("", corev1.ResourceRequirements{}, "shared-pvc", "kv")
		prefill.Template.Containers = []corev1.Container{{Name: "main", Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}}}
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *main,
				Prefill:      prefill,
			},
		}

		targets := collectPVCEvictorTargets(svc)
		if len(targets) != 1 {
			t.Fatalf("PVC evictor target count = %d, want 1", len(targets))
		}
		if targets[0].containerTemplate == nil || targets[0].containerTemplate.Image != "example.com/evictor:v1" {
			t.Fatalf("prefill did not inherit main template: %#v", targets[0].containerTemplate)
		}
		names := collectPVCEvictorPVCNames(svc)
		if len(names) != 1 || names[0] != "shared-pvc" {
			t.Fatalf("PVC names = %v, want [shared-pvc]", names)
		}
	})

	t.Run("different pvc.ref templates are rejected", func(t *testing.T) {
		svc := &v1alpha2.LLMInferenceService{
			Spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: *newWorkload("example.com/evictor:v1", corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				}, true),
				Prefill: newWorkload("example.com/evictor:v1", corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
				}, true),
			},
		}
		err := validatePVCEvictorContainerTemplates(svc)
		if err == nil {
			t.Fatal("different pvc.ref evictor templates were accepted")
		}
		if !strings.Contains(err.Error(), "same pvc-evictor container template") {
			t.Fatalf("error = %v, want pvc.ref template conflict", err)
		}
	})
}

func TestValidatePVCEvictorContainerTemplatesRejectsWorkerOnlyMarker(t *testing.T) {
	workload := &v1alpha2.WorkloadSpec{
		Template: &corev1.PodSpec{
			Containers: []corev1.Container{{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}}},
		},
		Worker: &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName},
			},
		},
		KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
			Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
				PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"}},
			}}},
		},
	}
	svc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: *workload},
	}

	err := validatePVCEvictorContainerTemplates(svc)
	if err == nil {
		t.Fatal("a worker-only pvc-evictor marker was accepted")
	}
	if !strings.Contains(err.Error(), "main template too") {
		t.Fatalf("error = %v, want worker-only marker rejection", err)
	}
}

func TestValidatePVCEvictorContainerTemplatesRejectsLeaderWorkerDrift(t *testing.T) {
	workload := &v1alpha2.WorkloadSpec{
		Template: &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName, Image: "example.com/evictor:v1"},
			},
		},
		Worker: &corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName, Image: "example.com/evictor:v2"},
			},
		},
		KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
			Secondary: []v1alpha2.SecondaryTierSpec{{
				FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv"},
					},
				},
			}},
		},
	}
	svc := &v1alpha2.LLMInferenceService{
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: *workload},
	}

	err := validatePVCEvictorContainerTemplates(svc)
	if err == nil {
		t.Fatal("different leader and worker pvc-evictor templates were accepted")
	}
	if !strings.Contains(err.Error(), "same pvc-evictor container template") {
		t.Fatalf("error = %v, want leader/worker pvc-evictor template conflict", err)
	}
}

func TestReconcilePVCEvictorsDeletesDuplicateDeployments(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName},
			}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"},
					},
				}}},
			},
		}},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-pvc", Namespace: "test"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
	}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{claimName: "shared-pvc", subPath: "kv-cache"})
	duplicate := expected.DeepCopy()
	duplicate.Name = pvcEvictorNamePrefix + "-duplicate"

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, expected, duplicate).Build()
	pvcReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient, APIReader: pvcReader}
	if err := reconciler.reconcilePVCEvictors(t.Context(), svc, &Config{}); err != nil {
		t.Fatalf("reconcilePVCEvictors() error = %v", err)
	}

	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), &appsv1.Deployment{}); err != nil {
		t.Fatalf("expected Deployment was removed: %v", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(duplicate), &appsv1.Deployment{}); err == nil {
		t.Fatal("duplicate Deployment was not removed")
	}
}

func TestReconcilePVCEvictorsDeletesStaleDeploymentBeforeReplacement(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{{
				Name: mainContainerName,
				Env:  []corev1.EnvVar{{Name: kvTransferArgsEnvVar}},
			}}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{
						Name: "shared-pvc", Path: "new-path",
					}},
				}}},
			},
		}},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-pvc", Namespace: "test"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
	}
	stale := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:            "shared-pvc",
		subPath:              "old-path",
		requiresNodeAffinity: true,
	})
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:            "shared-pvc",
		subPath:              "new-path",
		requiresNodeAffinity: true,
	})

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	var deletedDeployment bool
	var createdDeployment bool
	var propagation *metav1.DeletionPropagation
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(svc, stale).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, object client.Object, options ...client.CreateOption) error {
				if _, ok := object.(*appsv1.Deployment); ok {
					createdDeployment = true
				}
				return c.Create(ctx, object, options...)
			},
			Delete: func(ctx context.Context, c client.WithWatch, object client.Object, options ...client.DeleteOption) error {
				if deployment, ok := object.(*appsv1.Deployment); ok && deployment.Name == stale.Name {
					deleteOptions := client.DeleteOptions{}
					for _, option := range options {
						option.ApplyToDelete(&deleteOptions)
					}
					propagation = deleteOptions.PropagationPolicy
					deletedDeployment = true
				}
				return c.Delete(ctx, object, options...)
			},
		}).
		Build()
	pvcReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient, APIReader: pvcReader}

	if err := reconciler.reconcilePVCEvictors(t.Context(), svc, &Config{}); err != nil {
		t.Fatalf("reconcilePVCEvictors() error = %v", err)
	}
	if !deletedDeployment {
		t.Fatal("stale evictor Deployment was not deleted")
	}
	if propagation == nil || *propagation != metav1.DeletePropagationForeground {
		t.Fatalf("deletion propagation = %v, want foreground", propagation)
	}
	if createdDeployment {
		t.Fatal("replacement evictor Deployment was created before stale Deployment deletion completed")
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), &appsv1.Deployment{}); err == nil {
		t.Fatal("replacement evictor Deployment exists while stale Deployment is being deleted")
	}
}

func TestReconcilePVCEvictorsDoesNotWaitForUnrelatedStaleDeployment(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName},
			}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "active-pvc", Path: "cache"}},
				}}},
			},
		}},
	}
	activePVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "active-pvc", Namespace: "test"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
	}
	stale := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:            "unrelated-pvc",
		subPath:              "cache",
		requiresNodeAffinity: true,
	})
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:            "active-pvc",
		subPath:              "cache",
		requiresNodeAffinity: true,
	})

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, stale).Build()
	pvcReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(activePVC).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient, APIReader: pvcReader}

	if err := reconciler.reconcilePVCEvictors(t.Context(), svc, &Config{}); err != nil {
		t.Fatalf("reconcilePVCEvictors() error = %v", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), &appsv1.Deployment{}); err != nil {
		t.Fatalf("active evictor Deployment was blocked by unrelated cleanup: %v", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(stale), &appsv1.Deployment{}); err == nil {
		t.Fatal("unrelated stale evictor Deployment was not deleted")
	}
}

func TestReconcilePVCEvictorsSharedPVCAndFinalCleanup(t *testing.T) {
	service := func(name string) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test"},
			Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{Containers: []corev1.Container{
					{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
					{Name: pvcEvictorTemplateName},
				}},
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{
							Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"},
						},
					}}},
				},
			}},
		}
	}
	first := service("first")
	second := service("second")
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-pvc", Namespace: "test"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
		},
	}

	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(first, second).Build()
	pvcReader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient, APIReader: pvcReader}

	if err := reconciler.reconcilePVCEvictors(t.Context(), first, &Config{}); err != nil {
		t.Fatalf("first reconcilePVCEvictors() error = %v", err)
	}
	if err := reconciler.reconcilePVCEvictors(t.Context(), second, &Config{}); err != nil {
		t.Fatalf("second reconcilePVCEvictors() error = %v", err)
	}

	deployments := &appsv1.DeploymentList{}
	if err := fakeClient.List(t.Context(), deployments, client.InNamespace("test"), client.MatchingLabels{pvcEvictorLabel: "true"}); err != nil {
		t.Fatal(err)
	}
	if got, want := len(deployments.Items), 1; got != want {
		t.Fatalf("shared evictor Deployment count = %d, want %d", got, want)
	}

	if err := fakeClient.Delete(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.reconcilePVCEvictors(t.Context(), first, &Config{}); err != nil {
		t.Fatalf("reconcile after removing second service: %v", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(&deployments.Items[0]), &appsv1.Deployment{}); err != nil {
		t.Fatalf("shared evictor was removed while first service remained: %v", err)
	}

	if err := fakeClient.Delete(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.cleanupUnusedPVCEvictors(t.Context(), "test", &Config{}, nil, ""); err != nil {
		t.Fatalf("final cleanup: %v", err)
	}
	if err := fakeClient.List(t.Context(), deployments, client.InNamespace("test"), client.MatchingLabels{pvcEvictorLabel: "true"}); err != nil {
		t.Fatal(err)
	}
	if len(deployments.Items) != 0 {
		t.Fatalf("evictor Deployments remain after final cleanup: %d", len(deployments.Items))
	}
	serviceAccount := &corev1.ServiceAccount{}
	if err := fakeClient.Get(t.Context(), client.ObjectKey{Namespace: "test", Name: pvcEvictorServiceAccount}, serviceAccount); err == nil {
		t.Fatal("evictor ServiceAccount remains after final cleanup")
	}
}

func TestPVCRequiresNodeAffinity(t *testing.T) {
	tests := []struct {
		name       string
		accessMode corev1.PersistentVolumeAccessMode
		want       bool
	}{
		{name: "ReadWriteOnce", accessMode: corev1.ReadWriteOnce, want: true},
		{name: "ReadWriteMany", accessMode: corev1.ReadWriteMany, want: false},
		{name: "ReadOnlyMany", accessMode: corev1.ReadOnlyMany, want: false},
		{name: "ReadWriteOncePod", accessMode: corev1.ReadWriteOncePod, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pvc := &corev1.PersistentVolumeClaim{
				Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{tt.accessMode}},
			}
			if got := pvcRequiresNodeAffinity(pvc); got != tt.want {
				t.Fatalf("pvcRequiresNodeAffinity() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestPVCEvictorRejectsReadOnlyMany(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadOnlyMany}},
	}
	if !pvcHasReadOnlyMany(pvc) {
		t.Fatal("ReadOnlyMany was not detected")
	}
}

func TestExpectedPVCEvictorDeploymentsRejectsUnsupportedAccessMode(t *testing.T) {
	service := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName},
			}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
				PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc"}},
			}}}},
		}},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-pvc", Namespace: "test"},
		Spec:       corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod}},
	}
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	reconciler := &LLMISVCReconciler{APIReader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()}

	_, err := reconciler.expectedPVCEvictorDeployments(t.Context(), service)
	if err == nil || !errors.Is(err, reconcile.TerminalError(nil)) {
		t.Fatalf("expected terminal unsupported-access-mode error, got %v", err)
	}
}

func TestReconcilePVCEvictorsMarksUnsupportedPVCAccessMode(t *testing.T) {
	service := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			Template: &corev1.PodSpec{Containers: []corev1.Container{
				{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
				{Name: pvcEvictorTemplateName},
			}},
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
				PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc"}},
			}}}},
		}},
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-pvc", Namespace: "test"},
		Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteMany,
			corev1.ReadOnlyMany,
		}},
	}
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{
		appsv1.AddToScheme,
		corev1.AddToScheme,
		v1alpha2.AddToScheme,
	} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	reconciler := &LLMISVCReconciler{
		Client:    fake.NewClientBuilder().WithScheme(scheme).Build(),
		APIReader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build(),
	}

	if err := reconciler.reconcilePVCEvictors(t.Context(), service, &Config{}); err == nil {
		t.Fatal("reconcilePVCEvictors() error = nil, want unsupported access-mode error")
	}
	condition := service.Status.GetCondition(v1alpha2.PVCEvictorReady)
	if condition == nil || condition.Reason != "UnsupportedPVCAccessMode" {
		t.Fatalf("PVCEvictorReady = %#v, want UnsupportedPVCAccessMode", condition)
	}
}

func TestAddPVCEvictorConsumerLabels(t *testing.T) {
	labels := addPVCEvictorConsumerLabels(nil, []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
		PVC: &v1alpha2.PVCTierSpec{
			Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache/"},
		},
	}}}, true)

	if got, want := labels[pvcEvictorConsumerLabelKey("shared-pvc", "kv-cache")], "true"; got != want {
		t.Fatalf("consumer label = %q, want %q", got, want)
	}
}

func TestKVCacheSecondaryRootDirForPVCEvictor(t *testing.T) {
	kv := &v1alpha2.KVCacheOffloadingSpec{
		CPU: resource.MustParse("10Gi"),
		Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
			PVC: &v1alpha2.PVCTierSpec{
				Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc"},
			},
		}}},
	}

	rendered, err := kvTransferJSON(kv, true)
	if err != nil {
		t.Fatalf("kvTransferJSON() error = %v", err)
	}
	var payload struct {
		Extra struct {
			Secondary []struct {
				RootDir string `json:"root_dir"`
			} `json:"secondary_tiers"`
		} `json:"kv_connector_extra_config"`
	}
	if err := json.Unmarshal([]byte(rendered), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := payload.Extra.Secondary[0].RootDir, "/mnt/kv-cache-0/"+pvcEvictorCacheDirectory; got != want {
		t.Errorf("root_dir = %q, want %q", got, want)
	}
}

func TestKVCacheSecondaryRootDirWithoutPVCEvictor(t *testing.T) {
	kv := &v1alpha2.KVCacheOffloadingSpec{
		CPU: resource.MustParse("10Gi"),
		Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
			PVC: &v1alpha2.PVCTierSpec{
				Ref: &v1alpha2.PVCRefTierSpec{Name: "shared-pvc"},
			},
		}}},
	}

	rendered, err := kvTransferJSON(kv, false)
	if err != nil {
		t.Fatalf("kvTransferJSON() error = %v", err)
	}
	var payload struct {
		Extra struct {
			Secondary []struct {
				RootDir string `json:"root_dir"`
			} `json:"secondary_tiers"`
		} `json:"kv_connector_extra_config"`
	}
	if err := json.Unmarshal([]byte(rendered), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got, want := payload.Extra.Secondary[0].RootDir, "/mnt/kv-cache-0"; got != want {
		t.Errorf("root_dir = %q, want %q", got, want)
	}
}

func TestPVCEvictorKeyNormalizesSubPath(t *testing.T) {
	if got, want := pvcEvictorKey("shared-pvc", "kv-cache/"), pvcEvictorKey("shared-pvc", "kv-cache"); got != want {
		t.Fatalf("PVC evictor key = %q, want %q", got, want)
	}
}

func TestReconcilePVCEvictorsSkipsPeerMergeWithoutEvictors(t *testing.T) {
	service := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "test"}}
	peer := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "peer", Namespace: "test"},
		Spec:       v1alpha2.LLMInferenceServiceSpec{BaseRefs: []corev1.LocalObjectReference{{Name: "peer-base"}}},
	}
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{appsv1.AddToScheme, corev1.AddToScheme, v1alpha2.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	serviceLists := 0
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(service, peer).
		WithInterceptorFuncs(interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			if _, ok := list.(*v1alpha2.LLMInferenceServiceList); ok {
				serviceLists++
			}
			return c.List(ctx, list, opts...)
		}}).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	if err := reconciler.reconcilePVCEvictors(t.Context(), service, &Config{}); err != nil {
		t.Fatalf("reconcilePVCEvictors() error = %v", err)
	}
	if serviceLists != 0 {
		t.Fatalf("reconcilePVCEvictors() listed LLMInferenceServices %d times in a namespace without evictors, want 0", serviceLists)
	}
	if condition := service.Status.GetCondition(v1alpha2.PVCEvictorReady); condition != nil {
		t.Fatalf("PVCEvictorReady = %+v, want unset", condition)
	}
}

func TestReconcilePVCEvictorsRemovesLeftoverServiceAccount(t *testing.T) {
	// The last evictor Deployment is gone but deleting the ServiceAccount failed on that pass.
	service := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "test"}}
	serviceAccount := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:      pvcEvictorServiceAccount,
		Namespace: "test",
		Labels:    map[string]string{pvcEvictorLabel: "true"},
	}}
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{appsv1.AddToScheme, corev1.AddToScheme, v1alpha2.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(service, serviceAccount).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

	if err := reconciler.reconcilePVCEvictors(t.Context(), service, &Config{}); err != nil {
		t.Fatalf("reconcilePVCEvictors() error = %v", err)
	}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(serviceAccount), &corev1.ServiceAccount{}); err == nil {
		t.Fatal("leftover PVC evictor ServiceAccount was not removed")
	}
}

func TestMarkPVCEvictorReadinessReportsDeploymentsInNameOrder(t *testing.T) {
	expected := make(map[string]*appsv1.Deployment)
	for _, name := range []string{"evictor-b", "evictor-a", "evictor-c"} {
		expected[name] = &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "test",
			Annotations: map[string]string{pvcEvictorPVCName: name, pvcEvictorPVCPath: "kv-cache"},
		}}
	}
	for range 50 {
		service := &v1alpha2.LLMInferenceService{}
		markPVCEvictorReadiness(service, expected, nil)
		condition := service.Status.GetCondition(v1alpha2.PVCEvictorReady)
		if condition == nil || !strings.Contains(condition.Message, "test/evictor-a ") {
			t.Fatalf("PVCEvictorReady = %+v, want the message to name test/evictor-a", condition)
		}
	}
}

func TestValidatePVCEvictorContainerTemplatesReportsConflictsInIdentityOrder(t *testing.T) {
	podSpec := func(image string) *corev1.PodSpec {
		return &corev1.PodSpec{Containers: []corev1.Container{
			{Name: mainContainerName, Env: []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}},
			{Name: pvcEvictorTemplateName, Image: image},
		}}
	}
	tiers := func() *v1alpha2.KVCacheOffloadingSpec {
		kv := &v1alpha2.KVCacheOffloadingSpec{}
		for _, name := range []string{"pvc-b", "pvc-a", "pvc-c"} {
			kv.Secondary = append(kv.Secondary, v1alpha2.SecondaryTierSpec{FileSystem: &v1alpha2.FileSystemTierSpec{
				PVC: &v1alpha2.PVCTierSpec{Ref: &v1alpha2.PVCRefTierSpec{Name: name, Path: "kv-cache"}},
			}})
		}
		return kv
	}
	service := &v1alpha2.LLMInferenceService{Spec: v1alpha2.LLMInferenceServiceSpec{
		WorkloadSpec: v1alpha2.WorkloadSpec{Template: podSpec("example.com/evictor:main"), KVCacheOffloading: tiers()},
		Prefill:      &v1alpha2.WorkloadSpec{Template: podSpec("example.com/evictor:prefill"), KVCacheOffloading: tiers()},
	}}
	for range 50 {
		err := validatePVCEvictorContainerTemplates(service)
		if err == nil || !strings.Contains(err.Error(), `PVC "pvc-a"`) {
			t.Fatalf("validatePVCEvictorContainerTemplates() error = %v, want the conflict on PVC \"pvc-a\"", err)
		}
	}
}

func TestPVCEvictorPeerRequestsNeedASharedEvictor(t *testing.T) {
	changed := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "changed", Namespace: "test"}}
	peer := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "peer", Namespace: "test"}}
	evictor := expectedPVCEvictorDeployment(peer, pvcEvictorTarget{
		claimName: "shared-pvc", subPath: "kv-cache",
		containerTemplate: &corev1.Container{Name: pvcEvictorTemplateName, Image: "example.com/evictor:preset"},
	})
	scheme := runtime.NewScheme()
	for _, addToScheme := range []func(*runtime.Scheme) error{appsv1.AddToScheme, v1alpha2.AddToScheme} {
		if err := addToScheme(scheme); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		name    string
		objects []client.Object
		want    int
	}{
		{name: "no shared evictor", objects: []client.Object{changed, peer}, want: 0},
		{name: "shared evictor", objects: []client.Object{changed, peer, evictor}, want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.objects...).Build()}
			if requests := reconciler.pvcEvictorPeerRequests(t.Context(), changed); len(requests) != tc.want {
				t.Fatalf("pvcEvictorPeerRequests() = %v, want %d requests", requests, tc.want)
			}
		})
	}
}
