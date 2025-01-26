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
	"context"

	v1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeTrainedModels implements TrainedModelInterface
type FakeTrainedModels struct {
	Fake *FakeServingV1alpha1
	ns   string
}

var trainedmodelsResource = v1alpha1.SchemeGroupVersion.WithResource("trainedmodels")

var trainedmodelsKind = v1alpha1.SchemeGroupVersion.WithKind("TrainedModel")

// Get takes name of the trainedModel, and returns the corresponding trainedModel object, and an error if there is any.
func (c *FakeTrainedModels) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.TrainedModel, err error) {
	emptyResult := &v1alpha1.TrainedModel{}
	obj, err := c.Fake.
		Invokes(testing.NewGetActionWithOptions(trainedmodelsResource, c.ns, name, options), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.TrainedModel), err
}

// List takes label and field selectors, and returns the list of TrainedModels that match those selectors.
func (c *FakeTrainedModels) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.TrainedModelList, err error) {
	emptyResult := &v1alpha1.TrainedModelList{}
	obj, err := c.Fake.
		Invokes(testing.NewListActionWithOptions(trainedmodelsResource, trainedmodelsKind, c.ns, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.TrainedModelList{ListMeta: obj.(*v1alpha1.TrainedModelList).ListMeta}
	for _, item := range obj.(*v1alpha1.TrainedModelList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested trainedModels.
func (c *FakeTrainedModels) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchActionWithOptions(trainedmodelsResource, c.ns, opts))

}

// Create takes the representation of a trainedModel and creates it.  Returns the server's representation of the trainedModel, and an error, if there is any.
func (c *FakeTrainedModels) Create(ctx context.Context, trainedModel *v1alpha1.TrainedModel, opts v1.CreateOptions) (result *v1alpha1.TrainedModel, err error) {
	emptyResult := &v1alpha1.TrainedModel{}
	obj, err := c.Fake.
		Invokes(testing.NewCreateActionWithOptions(trainedmodelsResource, c.ns, trainedModel, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.TrainedModel), err
}

// Update takes the representation of a trainedModel and updates it. Returns the server's representation of the trainedModel, and an error, if there is any.
func (c *FakeTrainedModels) Update(ctx context.Context, trainedModel *v1alpha1.TrainedModel, opts v1.UpdateOptions) (result *v1alpha1.TrainedModel, err error) {
	emptyResult := &v1alpha1.TrainedModel{}
	obj, err := c.Fake.
		Invokes(testing.NewUpdateActionWithOptions(trainedmodelsResource, c.ns, trainedModel, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.TrainedModel), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeTrainedModels) UpdateStatus(ctx context.Context, trainedModel *v1alpha1.TrainedModel, opts v1.UpdateOptions) (result *v1alpha1.TrainedModel, err error) {
	emptyResult := &v1alpha1.TrainedModel{}
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceActionWithOptions(trainedmodelsResource, "status", c.ns, trainedModel, opts), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.TrainedModel), err
}

// Delete takes name of the trainedModel and deletes it. Returns an error if one occurs.
func (c *FakeTrainedModels) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(trainedmodelsResource, c.ns, name, opts), &v1alpha1.TrainedModel{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeTrainedModels) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionActionWithOptions(trainedmodelsResource, c.ns, opts, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.TrainedModelList{})
	return err
}

// Patch applies the patch and returns the patched trainedModel.
func (c *FakeTrainedModels) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.TrainedModel, err error) {
	emptyResult := &v1alpha1.TrainedModel{}
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceActionWithOptions(trainedmodelsResource, c.ns, name, pt, data, opts, subresources...), emptyResult)

	if obj == nil {
		return emptyResult, err
	}
	return obj.(*v1alpha1.TrainedModel), err
}
