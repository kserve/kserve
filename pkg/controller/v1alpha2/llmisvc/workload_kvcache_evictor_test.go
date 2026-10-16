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
	"encoding/json"
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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestAttachPVCEvictor(t *testing.T) {
	podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: mainContainerName}}}
	secondary := []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
		PVC: &v1alpha2.PVCTierSpec{
			Evictor: ptr.To(true),
			Spec:    &corev1.PersistentVolumeClaimSpec{},
		},
	}}}

	attachKVCacheSecondaryTiers(podSpec, secondary, mainContainerName)

	if got, want := len(podSpec.Containers), 2; got != want {
		t.Fatalf("container count = %d, want %d", got, want)
	}
	evictor := podSpec.Containers[1]
	if got, want := evictor.Name, "pvc-evictor-kv-cache-secondary-0"; got != want {
		t.Errorf("evictor name = %q, want %q", got, want)
	}
	if got, want := evictor.Image, pvcEvictorImage; got != want {
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

func TestExpectedPVCEvictorDeploymentForPVCRef(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					ServiceAccountName: "vllm-service-account",
					NodeSelector:       map[string]string{"accelerator": "gpu"},
					SecurityContext:    &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000))},
					Containers:         []corev1.Container{{Name: "main"}},
				},
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{
							Evictor: ptr.To(true),
							Ref:     &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache/"},
						},
					}}},
				},
			},
		},
	}

	target := pvcEvictorTarget{claimName: "shared-pvc", subPath: "kv-cache", requiresNodeAffinity: true}
	target.securityContext = &corev1.PodSecurityContext{
		RunAsUser:  ptr.To(int64(1000)),
		RunAsGroup: ptr.To(int64(1000)),
		FSGroup:    ptr.To(int64(1000)),
	}
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
		if got := deployment.Spec.Template.Spec.Affinity.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution; len(got) != 1 {
			t.Fatalf("required pod affinity count = %d, want 1", len(got))
		} else if got, want := got[0].LabelSelector.MatchLabels[pvcEvictorConsumerLabelKey("shared-pvc", "kv-cache")], "true"; got != want {
			t.Errorf("consumer affinity label = %q, want %q", got, want)
		}
	}

	noAffinity := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{claimName: "shared-pvc", subPath: "kv-cache"})
	if noAffinity.Spec.Template.Spec.Affinity != nil {
		t.Fatalf("RWX evictor Deployment must not set node affinity")
	}
}

func TestReconcileExistingPVCEvictorDeploymentRepairsDrift(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{claimName: "shared-pvc", subPath: "kv-cache"})
	current := expected.DeepCopy()
	current.Spec.Template.Spec.Containers[0].Image = "quay.io/example/old-evictor:old"
	current.Labels[pvcEvictorKeyLabel] = "changed"
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
	if err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), current, expected); err != nil {
		t.Fatalf("reconcileExistingPVCEvictorDeployment() error = %v", err)
	}

	got := &appsv1.Deployment{}
	if err := fakeClient.Get(t.Context(), client.ObjectKeyFromObject(expected), got); err != nil {
		t.Fatalf("get repaired Deployment: %v", err)
	}
	if got.Spec.Template.Spec.Containers[0].Image != pvcEvictorImage {
		t.Fatalf("image = %q, want %q", got.Spec.Template.Spec.Containers[0].Image, pvcEvictorImage)
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
}

func TestReconcileExistingPVCEvictorDeploymentRejectsSecurityDrift(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"}}
	expected := expectedPVCEvictorDeployment(svc, pvcEvictorTarget{
		claimName:       "shared-pvc",
		subPath:         "kv-cache",
		securityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000))},
	})
	current := expected.DeepCopy()
	current.Spec.Template.Spec.SecurityContext.FSGroup = ptr.To(int64(2000))
	current.Annotations[pvcEvictorSecurityHash] = pvcEvictorSecurityContextHash(current.Spec.Template.Spec.SecurityContext)

	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
	if err := reconciler.reconcileExistingPVCEvictorDeployment(t.Context(), current, expected); err == nil {
		t.Fatal("security drift was silently overwritten")
	}
}

func TestReconcilePVCEvictorsDeletesDuplicateDeployments(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: "test"},
		Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
			KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
				Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
					PVC: &v1alpha2.PVCTierSpec{
						Evictor: ptr.To(true),
						Ref:     &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"},
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
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc, pvc, expected, duplicate).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}
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

func TestReconcilePVCEvictorsSharedPVCAndFinalCleanup(t *testing.T) {
	service := func(name string) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "test"},
			Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
				KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{
					Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
						PVC: &v1alpha2.PVCTierSpec{
							Evictor: ptr.To(true),
							Ref:     &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache"},
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
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(first, second, pvc).Build()
	reconciler := &LLMISVCReconciler{Client: fakeClient}

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

func TestAddPVCEvictorConsumerLabels(t *testing.T) {
	labels := addPVCEvictorConsumerLabels(nil, []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
		PVC: &v1alpha2.PVCTierSpec{
			Evictor: ptr.To(true),
			Ref:     &v1alpha2.PVCRefTierSpec{Name: "shared-pvc", Path: "kv-cache/"},
		},
	}}})

	if got, want := labels[pvcEvictorConsumerLabelKey("shared-pvc", "kv-cache")], "true"; got != want {
		t.Fatalf("consumer label = %q, want %q", got, want)
	}
}

func TestKVCacheSecondaryRootDirForPVCEvictor(t *testing.T) {
	kv := &v1alpha2.KVCacheOffloadingSpec{
		CPU: resource.MustParse("10Gi"),
		Secondary: []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
			PVC: &v1alpha2.PVCTierSpec{
				Evictor: ptr.To(true),
				Ref:     &v1alpha2.PVCRefTierSpec{Name: "shared-pvc"},
			},
		}}},
	}

	rendered, err := kvTransferJSON(kv)
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

func TestPVCEvictorKeyNormalizesSubPath(t *testing.T) {
	if got, want := pvcEvictorKey("shared-pvc", "kv-cache/"), pvcEvictorKey("shared-pvc", "kv-cache"); got != want {
		t.Fatalf("PVC evictor key = %q, want %q", got, want)
	}
}
