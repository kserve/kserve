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

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
)

func TestValidateStorageCapacity_AllowsWithinLimit(t *testing.T) {
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu1"},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			StorageLimit: resource.MustParse("10Gi"),
		},
	}

	err := ValidateStorageCapacity(
		context.Background(),
		newStorageCapacityTestClient(t, nodeGroup),
		StorageReservation{
			Name:           "iris",
			SourceModelUri: "s3://models/iris",
			ModelSize:      resource.MustParse("5Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateStorageCapacity_RejectsWhenLimitExceeded(t *testing.T) {
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu1"},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			StorageLimit: resource.MustParse("10Gi"),
		},
	}

	err := ValidateStorageCapacity(
		context.Background(),
		newStorageCapacityTestClient(t, nodeGroup),
		StorageReservation{
			Name:           "iris",
			SourceModelUri: "s3://models/iris",
			ModelSize:      resource.MustParse("11Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	)

	if err == nil {
		t.Fatal("expected storage capacity validation to fail")
	}
}

func TestValidateStorageCapacity_RejectsAggregateLimitExceeded(t *testing.T) {
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu1"},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			StorageLimit: resource.MustParse("10Gi"),
		},
	}

	existingCache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "existing"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://models/existing",
			ModelSize:      resource.MustParse("6Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}

	err := ValidateStorageCapacity(
		context.Background(),
		newStorageCapacityTestClient(t, nodeGroup, existingCache),
		StorageReservation{
			Name:           "new",
			SourceModelUri: "s3://models/new",
			ModelSize:      resource.MustParse("5Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	)

	if err == nil {
		t.Fatal("expected aggregate storage capacity validation to fail")
	}
}

func TestValidateStorageCapacity_IncludesBothCacheTypes(t *testing.T) {
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu1"},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			StorageLimit: resource.MustParse("10Gi"),
		},
	}

	existingCache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-cache"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://models/cluster",
			ModelSize:      resource.MustParse("6Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}

	existingNamespaceCache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "namespace-cache",
			Namespace: "default",
		},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "s3://models/namespace",
			ModelSize:      resource.MustParse("2Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}

	err := ValidateStorageCapacity(
		context.Background(),
		newStorageCapacityTestClient(t, nodeGroup, existingCache, existingNamespaceCache),
		StorageReservation{
			Name:           "new",
			SourceModelUri: "s3://models/new",
			ModelSize:      resource.MustParse("3Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	)

	if err == nil {
		t.Fatal("expected storage capacity validation to fail")
	}
}

func TestValidateStorageCapacity_DoesNotCountCurrentCache(t *testing.T) {
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu1"},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			StorageLimit: resource.MustParse("10Gi"),
		},
	}

	currentCache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "iris"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://models/iris",
			ModelSize:      resource.MustParse("6Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}

	err := ValidateStorageCapacity(
		context.Background(),
		newStorageCapacityTestClient(t, nodeGroup, currentCache),
		StorageReservation{
			Name:           "iris",
			SourceModelUri: "s3://models/iris",
			ModelSize:      resource.MustParse("6Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	)
	if err != nil {
		t.Fatalf("expected current cache to be excluded, got %v", err)
	}
}

func TestValidateStorageCapacity_DeduplicatesSharedSourceURI(t *testing.T) {
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu1"},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			StorageLimit: resource.MustParse("10Gi"),
		},
	}
	existingCache := &v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-model"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "s3://models/shared",
			ModelSize:      resource.MustParse("6Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	existingNamespaceCache := &v1alpha1.LocalModelNamespaceCache{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-model-namespace",
			Namespace: "default",
		},
		Spec: v1alpha1.LocalModelNamespaceCacheSpec{
			SourceModelUri: "s3://models/shared",
			ModelSize:      resource.MustParse("6Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	err := ValidateStorageCapacity(
		context.Background(),
		newStorageCapacityTestClient(t, nodeGroup, existingCache, existingNamespaceCache),
		StorageReservation{
			Name:           "new",
			SourceModelUri: "s3://models/new",
			ModelSize:      resource.MustParse("3Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	)
	if err != nil {
		t.Fatalf("expected shared source URI to be counted once, got %v", err)
	}
}

func TestValidateStorageCapacity_SkipsMissingNodeGroup(t *testing.T) {
	nodeGroup := &v1alpha1.LocalModelNodeGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "gpu1"},
		Spec: v1alpha1.LocalModelNodeGroupSpec{
			StorageLimit: resource.MustParse("10Gi"),
		},
	}
	err := ValidateStorageCapacity(
		context.Background(),
		newStorageCapacityTestClient(t, nodeGroup),
		StorageReservation{
			Name:           "iris",
			SourceModelUri: "s3://models/iris",
			ModelSize:      resource.MustParse("5Gi"),
			NodeGroups:     []string{"missing-group", "gpu1"},
		},
	)
	if err != nil {
		t.Fatalf("expected missing node group to be skipped while existing groups are validated, got %v", err)
	}
}

func newStorageCapacityTestClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha1 scheme: %v", err)
	}

	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
}
