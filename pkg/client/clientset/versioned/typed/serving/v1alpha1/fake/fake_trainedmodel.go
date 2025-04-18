/*
Copyright 2023 The KServe Authors.

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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	servingv1alpha1 "github.com/kserve/kserve/pkg/client/clientset/versioned/typed/serving/v1alpha1"
	gentype "k8s.io/client-go/gentype"
)

// fakeTrainedModels implements TrainedModelInterface
type fakeTrainedModels struct {
	*gentype.FakeClientWithList[*v1alpha1.TrainedModel, *v1alpha1.TrainedModelList]
	Fake *FakeServingV1alpha1
}

func newFakeTrainedModels(fake *FakeServingV1alpha1, namespace string) servingv1alpha1.TrainedModelInterface {
	return &fakeTrainedModels{
		gentype.NewFakeClientWithList[*v1alpha1.TrainedModel, *v1alpha1.TrainedModelList](
			fake.Fake,
			namespace,
			v1alpha1.SchemeGroupVersion.WithResource("trainedmodels"),
			v1alpha1.SchemeGroupVersion.WithKind("TrainedModel"),
			func() *v1alpha1.TrainedModel { return &v1alpha1.TrainedModel{} },
			func() *v1alpha1.TrainedModelList { return &v1alpha1.TrainedModelList{} },
			func(dst, src *v1alpha1.TrainedModelList) { dst.ListMeta = src.ListMeta },
			func(list *v1alpha1.TrainedModelList) []*v1alpha1.TrainedModel {
				return gentype.ToPointerSlice(list.Items)
			},
			func(list *v1alpha1.TrainedModelList, items []*v1alpha1.TrainedModel) {
				list.Items = gentype.FromPointerSlice(items)
			},
		),
		fake,
	}
}
