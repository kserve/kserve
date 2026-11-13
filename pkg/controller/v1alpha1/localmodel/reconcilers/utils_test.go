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
	"errors"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
)

// nodeListErrorClient fails every Node list so GetNodesFromNodeGroup returns an error.
type nodeListErrorClient struct {
	client.Client
	err error
}

func (c *nodeListErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*corev1.NodeList); ok {
		return c.err
	}
	return c.Client.List(ctx, list, opts...)
}

func newLocalModelCacheClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add KServe scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1alpha1.LocalModelCache{}).
		WithObjects(objs...).
		Build()
}

func isvcConsumer(name, namespace string) cacheConsumers {
	return cacheConsumers{isvcs: []v1beta1.InferenceService{{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}}}
}

func TestReconcileLocalModelNodeRecordsConsumersWithoutNodeGroups(t *testing.T) {
	cache := &v1alpha1.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache"}}
	cl := newLocalModelCacheClient(t, cache)

	err := ReconcileLocalModelNode(context.Background(), cl, logr.Discard(), cache, nil,
		map[string]*v1alpha1.LocalModelNodeGroup{}, isvcConsumer("isvc", "ns"))
	if err != nil {
		t.Fatalf("ReconcileLocalModelNode() error = %v", err)
	}

	persisted := &v1alpha1.LocalModelCache{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "cache"}, persisted); err != nil {
		t.Fatalf("get cache: %v", err)
	}
	want := v1alpha1.NamespacedName{Name: "isvc", Namespace: "ns"}
	if len(persisted.Status.InferenceServices) != 1 {
		t.Fatalf("Status.InferenceServices = %#v, want exactly [%#v]", persisted.Status.InferenceServices, want)
	}
	if got := persisted.Status.InferenceServices[0]; got != want {
		t.Fatalf("Status.InferenceServices[0] = %#v, want %#v", got, want)
	}
}

func TestReconcileLocalModelNodeRecordsConsumersOnNodeGroupError(t *testing.T) {
	cache := &v1alpha1.LocalModelCache{ObjectMeta: metav1.ObjectMeta{Name: "cache"}}
	listErr := errors.New("node list failed")
	cl := &nodeListErrorClient{Client: newLocalModelCacheClient(t, cache), err: listErr}
	nodeGroups := map[string]*v1alpha1.LocalModelNodeGroup{
		"gpu": {ObjectMeta: metav1.ObjectMeta{Name: "gpu"}},
	}

	err := ReconcileLocalModelNode(context.Background(), cl, logr.Discard(), cache, nil, nodeGroups, isvcConsumer("isvc", "ns"))
	if !errors.Is(err, listErr) {
		t.Fatalf("ReconcileLocalModelNode() error = %v, want %v", err, listErr)
	}

	persisted := &v1alpha1.LocalModelCache{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "cache"}, persisted); err != nil {
		t.Fatalf("get cache: %v", err)
	}
	if len(persisted.Status.InferenceServices) != 1 || persisted.Status.InferenceServices[0].Name != "isvc" {
		t.Fatalf("Status.InferenceServices = %#v, want the isvc consumer recorded despite the node-group error", persisted.Status.InferenceServices)
	}
}

func TestUpdateLocalModelNodeReplacesImagePullSecrets(t *testing.T) {
	node := &v1alpha1.LocalModelNode{
		ObjectMeta: metav1.ObjectMeta{Name: "worker"},
		Spec: v1alpha1.LocalModelNodeSpec{
			LocalModels: []v1alpha1.LocalModelInfo{{
				ModelName:        "cache",
				SourceModelUri:   "oci://registry.example/model:v1",
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "reg-cred-a"}},
			}},
		},
	}
	cache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri:   "oci://registry.example/model:v1",
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "reg-cred-b"}},
		},
	}
	cl := newLocalModelCacheClient(t, node, cache)
	if err := UpdateLocalModelNode(context.Background(), cl, logr.Discard(), node, cache, nil, "gpu"); err != nil {
		t.Fatalf("UpdateLocalModelNode() error = %v", err)
	}
	got := &v1alpha1.LocalModelNode{}
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "worker"}, got); err != nil {
		t.Fatalf("get node: %v", err)
	}
	if len(got.Spec.LocalModels) != 1 || len(got.Spec.LocalModels[0].ImagePullSecrets) != 1 ||
		got.Spec.LocalModels[0].ImagePullSecrets[0].Name != "reg-cred-b" {
		t.Fatalf("LocalModels = %#v, want imagePullSecrets [reg-cred-b]", got.Spec.LocalModels)
	}
}

func TestCreatePVCReleasesStaleClaimRefOnRetainedPV(t *testing.T) {
	ctx := context.Background()
	releasedPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu-ns"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef: &corev1.ObjectReference{
				Kind:      "PersistentVolumeClaim",
				Namespace: "ns",
				Name:      "cache-gpu",
				UID:       "deleted-claim-uid",
			},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeReleased},
	}
	cs := k8sfake.NewClientset(releasedPV)

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "cache-gpu-ns"},
	}
	if err := CreatePVC(ctx, cs, nil, logr.Discard(), pvc, "ns", nil, nil); err != nil {
		t.Fatalf("CreatePVC() error = %v", err)
	}

	gotPV, err := cs.CoreV1().PersistentVolumes().Get(ctx, "cache-gpu-ns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get PV: %v", err)
	}
	if gotPV.Spec.ClaimRef != nil {
		t.Fatalf("PV claimRef = %#v, want nil so the recreated claim can bind", gotPV.Spec.ClaimRef)
	}

	gotPVC, err := cs.CoreV1().PersistentVolumeClaims("ns").Get(ctx, "cache-gpu", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get PVC: %v", err)
	}
	if gotPVC.Spec.VolumeName != "cache-gpu-ns" {
		t.Fatalf("PVC volumeName = %q, want %q", gotPVC.Spec.VolumeName, "cache-gpu-ns")
	}
}

func TestCreatePVCLeavesClaimRefOfAnotherNamespaceAlone(t *testing.T) {
	ctx := context.Background()
	boundPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu-other"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef: &corev1.ObjectReference{
				Kind:      "PersistentVolumeClaim",
				Namespace: "other",
				Name:      "cache-gpu",
				UID:       "live-claim-uid",
			},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	cs := k8sfake.NewClientset(boundPV)

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "cache-gpu-other"},
	}
	if err := CreatePVC(ctx, cs, nil, logr.Discard(), pvc, "ns", nil, nil); err != nil {
		t.Fatalf("CreatePVC() error = %v", err)
	}

	gotPV, err := cs.CoreV1().PersistentVolumes().Get(ctx, "cache-gpu-other", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get PV: %v", err)
	}
	if gotPV.Spec.ClaimRef == nil || gotPV.Spec.ClaimRef.Namespace != "other" {
		t.Fatalf("PV claimRef = %#v, want the claim of namespace other untouched", gotPV.Spec.ClaimRef)
	}
}

// TestCreatePVCKeepsClaimRefWhenARecreatedClaimBindsFirst simulates another
// controller binding a freshly created claim to the retained PV between the
// controller's Get and its release request. The release must not clear that
// live reference.
func TestCreatePVCKeepsClaimRefWhenARecreatedClaimBindsFirst(t *testing.T) {
	ctx := context.Background()
	retainedPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu-ns"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef: &corev1.ObjectReference{
				Kind:      "PersistentVolumeClaim",
				Namespace: "ns",
				Name:      "cache-gpu",
				UID:       "deleted-claim-uid",
			},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeReleased},
	}
	livePV := retainedPV.DeepCopy()
	livePV.Spec.ClaimRef = &corev1.ObjectReference{
		Kind:      "PersistentVolumeClaim",
		Namespace: "ns",
		Name:      "cache-gpu",
		UID:       "live-claim-uid",
	}
	cs := k8sfake.NewClientset(retainedPV)
	cs.PrependReactor("patch", "persistentvolumes", func(k8stesting.Action) (bool, runtime.Object, error) {
		if err := cs.Tracker().Update(
			schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumes"},
			livePV, "",
		); err != nil {
			t.Fatalf("update PV in tracker: %v", err)
		}
		return false, nil, nil
	})

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "cache-gpu-ns"},
	}
	if err := CreatePVC(ctx, cs, nil, logr.Discard(), pvc, "ns", nil, nil); err == nil {
		t.Fatal("CreatePVC() error = nil, want the release to fail on the changed claimRef")
	}

	gotPV, err := cs.CoreV1().PersistentVolumes().Get(ctx, "cache-gpu-ns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get PV: %v", err)
	}
	if gotPV.Spec.ClaimRef == nil || gotPV.Spec.ClaimRef.UID != "live-claim-uid" {
		t.Fatalf("PV claimRef = %#v, want the newly bound claim reference kept", gotPV.Spec.ClaimRef)
	}
	if _, err := cs.CoreV1().PersistentVolumeClaims("ns").Get(ctx, "cache-gpu", metav1.GetOptions{}); !apierr.IsNotFound(err) {
		t.Fatalf("PVC lookup error = %v, want NotFound because the claim was not created", err)
	}
}

// TestReleaseStaleClaimRefRepairsRecreatedPendingClaim covers the window between
// the caller's claim lookup and the release: another controller can recreate the
// claim after it was seen absent. That claim is Pending and holds a different
// UID than the stale reference, so it is stuck on the same reference and the
// release repairs it.
func TestReleaseStaleClaimRefRepairsRecreatedPendingClaim(t *testing.T) {
	ctx := context.Background()
	retainedPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu-ns"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef: &corev1.ObjectReference{
				Kind:      "PersistentVolumeClaim",
				Namespace: "ns",
				Name:      "cache-gpu",
				UID:       "deleted-claim-uid",
			},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeReleased},
	}
	recreatedClaim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu", Namespace: "ns", UID: "live-claim-uid"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	}
	cs := k8sfake.NewClientset(retainedPV, recreatedClaim)

	if err := releaseStaleClaimRef(ctx, cs.CoreV1().PersistentVolumes(), cs.CoreV1().PersistentVolumeClaims("ns"),
		logr.Discard(), "cache-gpu-ns", "ns", "cache-gpu"); err != nil {
		t.Fatalf("releaseStaleClaimRef() error = %v", err)
	}

	gotPV, err := cs.CoreV1().PersistentVolumes().Get(ctx, "cache-gpu-ns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get PV: %v", err)
	}
	if gotPV.Spec.ClaimRef != nil {
		t.Fatalf("PV claimRef = %#v, want nil so the pending claim can bind", gotPV.Spec.ClaimRef)
	}
}

// TestReleaseStaleClaimRefKeepsBoundClaim checks that the release never clears
// the reference of a claim that is already bound.
func TestReleaseStaleClaimRefKeepsBoundClaim(t *testing.T) {
	ctx := context.Background()
	retainedPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu-ns"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef: &corev1.ObjectReference{
				Kind:      "PersistentVolumeClaim",
				Namespace: "ns",
				Name:      "cache-gpu",
				UID:       "live-claim-uid",
			},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	boundClaim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu", Namespace: "ns", UID: "live-claim-uid"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	}
	cs := k8sfake.NewClientset(retainedPV, boundClaim)

	if err := releaseStaleClaimRef(ctx, cs.CoreV1().PersistentVolumes(), cs.CoreV1().PersistentVolumeClaims("ns"),
		logr.Discard(), "cache-gpu-ns", "ns", "cache-gpu"); err != nil {
		t.Fatalf("releaseStaleClaimRef() error = %v", err)
	}

	gotPV, err := cs.CoreV1().PersistentVolumes().Get(ctx, "cache-gpu-ns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get PV: %v", err)
	}
	if gotPV.Spec.ClaimRef == nil || gotPV.Spec.ClaimRef.UID != "live-claim-uid" {
		t.Fatalf("PV claimRef = %#v, want the bound claim reference kept", gotPV.Spec.ClaimRef)
	}
}

// TestCreatePVCLeavesPreReservedClaimRefAlone checks that a volume reserved for
// the claim by name keeps its reference, because the claim can bind to it as is.
func TestCreatePVCLeavesPreReservedClaimRefAlone(t *testing.T) {
	ctx := context.Background()
	reservedPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu-ns"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef: &corev1.ObjectReference{
				Kind:      "PersistentVolumeClaim",
				Namespace: "ns",
				Name:      "cache-gpu",
			},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeAvailable},
	}
	cs := k8sfake.NewClientset(reservedPV)

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "cache-gpu-ns"},
	}
	if err := CreatePVC(ctx, cs, nil, logr.Discard(), pvc, "ns", nil, nil); err != nil {
		t.Fatalf("CreatePVC() error = %v", err)
	}

	gotPV, err := cs.CoreV1().PersistentVolumes().Get(ctx, "cache-gpu-ns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get PV: %v", err)
	}
	if gotPV.Spec.ClaimRef == nil || gotPV.Spec.ClaimRef.Name != "cache-gpu" {
		t.Fatalf("PV claimRef = %#v, want the pre-reserved reference kept", gotPV.Spec.ClaimRef)
	}
	if _, err := cs.CoreV1().PersistentVolumeClaims("ns").Get(ctx, "cache-gpu", metav1.GetOptions{}); err != nil {
		t.Fatalf("get PVC: %v", err)
	}
}

// TestCreatePVCRepairsExistingPendingClaim checks that a claim a previous
// controller already created and left Pending on the retained volume is
// repaired, not skipped because it exists.
func TestCreatePVCRepairsExistingPendingClaim(t *testing.T) {
	ctx := context.Background()
	retainedPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu-ns"},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			ClaimRef: &corev1.ObjectReference{
				Kind:      "PersistentVolumeClaim",
				Namespace: "ns",
				Name:      "cache-gpu",
				UID:       "deleted-claim-uid",
			},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeReleased},
	}
	stuckClaim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu", Namespace: "ns", UID: "recreated-claim-uid"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "cache-gpu-ns"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	}
	cs := k8sfake.NewClientset(retainedPV, stuckClaim)

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-gpu"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: "cache-gpu-ns"},
	}
	if err := CreatePVC(ctx, cs, nil, logr.Discard(), pvc, "ns", nil, nil); err != nil {
		t.Fatalf("CreatePVC() error = %v", err)
	}

	gotPV, err := cs.CoreV1().PersistentVolumes().Get(ctx, "cache-gpu-ns", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get PV: %v", err)
	}
	if gotPV.Spec.ClaimRef != nil {
		t.Fatalf("PV claimRef = %#v, want nil so the existing pending claim can bind", gotPV.Spec.ClaimRef)
	}
}
