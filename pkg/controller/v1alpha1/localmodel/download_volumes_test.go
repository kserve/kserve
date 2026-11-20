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

package localmodel

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/controller/v1alpha1/localmodel/reconcilers"
)

func checkDownloadVolumeRetry(ctx SpecContext, validGroup v1alpha1.LocalModelNodeGroupSpec, namespaced, rejectPV bool) {
	tenant := fmt.Sprintf("test-volume-retry-%d", time.Now().UnixNano())
	ns := createTestNamespace(ctx, tenant)
	DeferCleanup(func(ctx SpecContext) { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) })

	broken := &v1alpha1.LocalModelNodeGroup{ObjectMeta: metav1.ObjectMeta{Name: "broken"}, Spec: *validGroup.DeepCopy()}
	healthy := &v1alpha1.LocalModelNodeGroup{ObjectMeta: metav1.ObjectMeta{Name: "healthy"}, Spec: *validGroup.DeepCopy()}
	if rejectPV {
		broken.Spec.PersistentVolumeSpec.PersistentVolumeSource = corev1.PersistentVolumeSource{}
	} else {
		broken.Spec.PersistentVolumeClaimSpec.AccessModes = nil
	}
	groups := []string{broken.Name, healthy.Name}
	metadata := metav1.ObjectMeta{Name: tenant, UID: types.UID(tenant)}
	var cache client.Object = &v1alpha1.LocalModelCache{
		ObjectMeta: metadata, Spec: v1alpha1.LocalModelCacheSpec{NodeGroups: groups, SourceModelUri: "s3://bucket/model"},
	}
	index := reconcilers.LocalModelKey
	if namespaced {
		metadata.Namespace = tenant
		cache = &v1alpha1.LocalModelNamespaceCache{
			ObjectMeta: metadata, Spec: v1alpha1.LocalModelNamespaceCacheSpec{NodeGroups: groups, SourceModelUri: "s3://bucket/model"},
		}
		index = reconcilers.LocalModelNamespaceKey
	}
	consumer := &v1beta1.InferenceService{ObjectMeta: metav1.ObjectMeta{
		Name: "consumer", Namespace: tenant, Annotations: map[string]string{constants.NodeGroupAnnotationKey: healthy.Name},
	}}
	// The direct reconcile is the only writer for this cache. Its PVs and PVCs
	// go to the suite's real API server so it validates the rejected create.
	cacheClient := fake.NewClientBuilder().WithScheme(testScheme).
		WithObjects(cache, broken, healthy, consumer).WithStatusSubresource(cache).
		WithIndex(consumer, index, func(client.Object) []string { return []string{cache.GetName()} }).Build()
	clientset := kubernetes.NewForConfigOrDie(cfg)
	reconcile := (&LocalModelReconciler{Client: cacheClient, Scheme: testScheme, Clientset: clientset}).Reconcile
	if namespaced {
		reconcile = (&LocalModelNamespaceCacheReconciler{Client: cacheClient, Scheme: testScheme, Clientset: clientset}).Reconcile
	}
	request := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cache)}

	type volumePair struct {
		pv  *corev1.PersistentVolume
		pvc *corev1.PersistentVolumeClaim
	}
	pair := func(pvName, pvcName, namespace string) volumePair {
		pv := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: pvName}}
		pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: namespace}}
		for _, obj := range []client.Object{pv, pvc} {
			DeferCleanup(func(ctx SpecContext) error { return client.IgnoreNotFound(k8sClient.Delete(ctx, obj)) })
		}
		return volumePair{pv, pvc}
	}
	download := func(group string) volumePair {
		claimName := metadata.Name + "-" + group
		pvName := claimName + "-download"
		if namespaced {
			pvName = claimName + "-" + tenant + "-download"
			claimName = pvName
		}
		return pair(pvName, claimName, "kserve-localmodel-jobs")
	}
	failedGroup := download(broken.Name)
	healthyGroup := download(healthy.Name)
	servingClaim := metadata.Name + "-" + healthy.Name
	serving := pair(servingClaim+"-"+tenant, servingClaim, tenant)
	observe := func(volumes volumePair) [2]types.UID {
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(volumes.pv), volumes.pv)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(volumes.pvc), volumes.pvc)).To(Succeed())
		Expect(volumes.pvc.Spec.VolumeName).To(Equal(volumes.pv.Name))
		return [2]types.UID{volumes.pv.UID, volumes.pvc.UID}
	}
	_, err := reconcile(ctx, request)
	Expect(errors.IsInvalid(err)).To(BeTrue(), "reconcile must return the API server's volume create error: %v", err)
	healthyUIDs := observe(healthyGroup)
	servingUIDs := observe(serving)

	present, missing := client.Object(failedGroup.pv), client.Object(failedGroup.pvc)
	if rejectPV {
		present, missing = missing, present
	}
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(present), present)).To(Succeed())
	Expect(errors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(missing), missing))).To(BeTrue())
	firstUID := present.GetUID()

	Expect(cacheClient.Get(ctx, client.ObjectKeyFromObject(broken), broken)).To(Succeed())
	broken.Spec = *validGroup.DeepCopy()
	Expect(cacheClient.Update(ctx, broken)).To(Succeed())
	_, err = reconcile(ctx, request)
	Expect(err).NotTo(HaveOccurred())
	recoveredUIDs := observe(failedGroup)
	Expect(present.GetUID()).To(Equal(firstUID))
	_, err = reconcile(ctx, request)
	Expect(err).NotTo(HaveOccurred())
	Expect(observe(failedGroup)).To(Equal(recoveredUIDs))
	Expect(observe(healthyGroup)).To(Equal(healthyUIDs))
	Expect(observe(serving)).To(Equal(servingUIDs))
}
