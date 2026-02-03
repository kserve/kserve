/*
Copyright 2021 The KServe Authors.

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
package webhook

import (
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

var storageURI = "gs://testbucket/testmodel"

func makeTestInferenceService() v1beta1.InferenceService {
	inferenceservice := v1beta1.InferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sklearn-iris",
			Namespace: "default",
			Labels: map[string]string{
				constants.LocalModelLabel: "iris",
			},
		},
		Spec: v1beta1.InferenceServiceSpec{
			Predictor: v1beta1.PredictorSpec{
				Model: &v1beta1.ModelSpec{
					ModelFormat: v1beta1.ModelFormat{
						Name: "sklearn",
					},
					PredictorExtensionSpec: v1beta1.PredictorExtensionSpec{
						StorageURI: proto.String(storageURI),
					},
				},
			},
		},
	}
	return inferenceservice
}

func makeTestLocalModelCache() v1alpha1.LocalModelCache {
	localModelCache := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "iris",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
			SourceModelUri: storageURI,
		},
		Status: v1alpha1.LocalModelCacheStatus{
			InferenceServices: []v1alpha1.NamespacedName{
				{
					Namespace: "default",
					Name:      "sklearn-iris",
				},
			},
		},
	}
	return localModelCache
}

func makeTestLocalModelCacheWithSameStorageURI() v1alpha1.LocalModelCache {
	localModelCache := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "blah",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
			SourceModelUri: storageURI,
		},
	}
	return localModelCache
}

func makeTestLocalModelCacheWithVersion(version int32) v1alpha1.LocalModelCache {
	localModelCache := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "iris",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
			SourceModelUri: storageURI,
			Version:        version,
		},
	}
	return localModelCache
}

func TestUnableToDeleteLocalModelCacheWithActiveIsvc(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	lmc := makeTestLocalModelCache()
	isvc := makeTestInferenceService()
	s := runtime.NewScheme()
	err := v1beta1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(&isvc).WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}
	warnings, err := validator.ValidateDelete(t.Context(), &lmc)
	g.Expect(warnings).NotTo(gomega.BeNil())
	g.Expect(err).To(gomega.MatchError(fmt.Errorf("LocalModelCache %s is being used by InferenceService %s", lmc.Name, isvc.Name)))
}

func makeTestLocalModelCacheWithDifferentStorageURI() v1alpha1.LocalModelCache {
	localModelCache := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{
			Name: "different",
		},
		Spec: v1alpha1.LocalModelCacheSpec{
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
			SourceModelUri: "gs://testbucket/differentmodel",
		},
	}
	return localModelCache
}

func TestValidateCreate_DuplicateVersion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingLmc := makeTestLocalModelCacheWithVersion(1)
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(&existingLmc).WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}
	
	// Try to create another cache with same URI and version
	newLmc := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "iris-v2"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: storageURI,
			Version:        1, // Same version
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	warnings, err := validator.ValidateCreate(t.Context(), &newLmc)
	g.Expect(warnings).NotTo(gomega.BeNil())
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("cannot create version 1"))
}

func TestValidateCreate_NewerVersion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingLmc := makeTestLocalModelCacheWithVersion(1)
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(&existingLmc).WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}
	
	// Create cache with same URI but newer version
	newLmc := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "iris-v2"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: storageURI,
			Version:        2,
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	warnings, err := validator.ValidateCreate(t.Context(), &newLmc)
	g.Expect(len(warnings)).To(gomega.Equal(0))
	g.Expect(err).ToNot(gomega.HaveOccurred())
}

func TestValidateCreate_OlderVersion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingLmc := makeTestLocalModelCacheWithVersion(2)
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(&existingLmc).WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}
	
	// Try to create cache with older version
	newLmc := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "iris-v1"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: storageURI,
			Version:        1,
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	warnings, err := validator.ValidateCreate(t.Context(), &newLmc)
	g.Expect(warnings).NotTo(gomega.BeNil())
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("cannot create version 1"))
}

func TestValidateCreate_DifferentURISameVersion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingLmc := makeTestLocalModelCacheWithVersion(1)
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(&existingLmc).WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}
	
	// Different URI with same version should be allowed
	newLmc := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "other-model"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: "gs://otherbucket/othermodel",
			Version:        1,
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	warnings, err := validator.ValidateCreate(t.Context(), &newLmc)
	g.Expect(len(warnings)).To(gomega.Equal(0))
	g.Expect(err).ToNot(gomega.HaveOccurred())
}

func TestValidateCreate_FirstLocalModelCache(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}
	
	newLmc := makeTestLocalModelCache()
	warnings, err := validator.ValidateCreate(t.Context(), &newLmc)
	g.Expect(len(warnings)).To(gomega.Equal(0))
	g.Expect(err).ToNot(gomega.HaveOccurred())
}

func TestValidateUpdate_DuplicateVersion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingLmc := makeTestLocalModelCacheWithVersion(1)
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(&existingLmc).WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}

	// Try to update another cache to same version
	oldLmc := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "iris-v2"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: storageURI,
			Version:        2,
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	newLmc := oldLmc.DeepCopy()
	newLmc.Spec.Version = 1 // Change to duplicate version
	warnings, err := validator.ValidateUpdate(t.Context(), &oldLmc, newLmc)
	g.Expect(warnings).NotTo(gomega.BeNil())
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("cannot update to version 1"))
}

func TestValidateUpdate_OlderVersion(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingLmc := makeTestLocalModelCacheWithVersion(3)
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(&existingLmc).WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}

	// Try to update to older version than existing
	oldLmc := v1alpha1.LocalModelCache{
		ObjectMeta: metav1.ObjectMeta{Name: "iris-v2"},
		Spec: v1alpha1.LocalModelCacheSpec{
			SourceModelUri: storageURI,
			Version:        4,
			ModelSize:      resource.MustParse("1Gi"),
			NodeGroups:     []string{"gpu1"},
		},
	}
	newLmc := oldLmc.DeepCopy()
	newLmc.Spec.Version = 2 // Change to older version
	warnings, err := validator.ValidateUpdate(t.Context(), &oldLmc, newLmc)
	g.Expect(warnings).NotTo(gomega.BeNil())
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("cannot update to version 2"))
}

func TestValidateUpdate_LocalModelCacheWithUniqueStorageURI(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	existingLmc := makeTestLocalModelCache()
	s := runtime.NewScheme()
	err := v1alpha1.AddToScheme(s)
	if err != nil {
		t.Errorf("unable to add scheme : %v", err)
	}
	fakeClient := fake.NewClientBuilder().WithObjects(&existingLmc).WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}
	// newLmc has a unique StorageURI
	newLmc := makeTestLocalModelCacheWithDifferentStorageURI()
	oldLmc := makeTestLocalModelCacheWithSameStorageURI()
	warnings, err := validator.ValidateUpdate(t.Context(), &oldLmc, &newLmc)
	g.Expect(warnings).To(gomega.BeNil())
	g.Expect(err).ToNot(gomega.HaveOccurred())
}

func TestValidateUpdate_InvalidObjectType(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	s := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	validator := LocalModelCacheValidator{fakeClient}
	invalidObj := &v1beta1.InferenceService{}
	oldLmc := makeTestLocalModelCache()
	warnings, err := validator.ValidateUpdate(t.Context(), &oldLmc, invalidObj)
	g.Expect(warnings).To(gomega.BeNil())
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(err.Error()).To(gomega.ContainSubstring("expected *v1alpha1.LocalModelCache"))
}