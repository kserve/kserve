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

package localmodelcache

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func testNSCacheWithPVC(pvcUID k8stypes.UID) *v1alpha1.LocalModelNamespaceCache {
	pvcRef := "claim"
	return &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "default"},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "hf://org/model",
			ModelSize:      resource.MustParse("1Gi"),
			PVCRef:         &pvcRef,
		},
		Status: v1alpha1.LocalModelCacheStatus{
			SharedPVCImport: &v1alpha1.SharedPVCImportStatus{PVCUID: pvcUID},
		},
	}
}

func fakeClientWithPVC(pvc *corev1.PersistentVolumeClaim) client.Client {
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		panic(err)
	}
	if err := v1alpha1.AddToScheme(s); err != nil {
		panic(err)
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(pvc).Build()
}

func TestClaimHoldsImportedModel(t *testing.T) {
	g := gomega.NewWithT(t)
	now := metav1.Now()

	live := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "claim", Namespace: "default", UID: "uid-1",
	}}

	t.Run("terminating claim never holds the model", func(t *testing.T) {
		terminating := live.DeepCopy()
		terminating.DeletionTimestamp = &now
		terminating.Finalizers = []string{"test.kserve.io/hold"}
		c := fakeClientWithPVC(terminating)

		holds, err := ClaimHoldsImportedModel(context.Background(), c, testNSCacheWithPVC("uid-1"))
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(holds).To(gomega.BeFalse())
	})

	t.Run("import record naming the live claim holds the model", func(t *testing.T) {
		c := fakeClientWithPVC(live)

		holds, err := ClaimHoldsImportedModel(context.Background(), c, testNSCacheWithPVC("uid-1"))
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(holds).To(gomega.BeTrue())
	})

	t.Run("import record naming a recreated claim does not hold the model", func(t *testing.T) {
		c := fakeClientWithPVC(live)

		holds, err := ClaimHoldsImportedModel(context.Background(), c, testNSCacheWithPVC("uid-old"))
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(holds).To(gomega.BeFalse())
	})
}
