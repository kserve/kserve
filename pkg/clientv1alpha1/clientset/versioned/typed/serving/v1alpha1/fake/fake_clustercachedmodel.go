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

// FakeClusterCachedModels implements ClusterCachedModelInterface
type FakeClusterCachedModels struct {
	Fake *FakeServingV1alpha1
	ns   string
}

var clustercachedmodelsResource = v1alpha1.SchemeGroupVersion.WithResource("clustercachedmodels")

var clustercachedmodelsKind = v1alpha1.SchemeGroupVersion.WithKind("ClusterCachedModel")

// Get takes name of the clusterCachedModel, and returns the corresponding clusterCachedModel object, and an error if there is any.
func (c *FakeClusterCachedModels) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ClusterCachedModel, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(clustercachedmodelsResource, c.ns, name), &v1alpha1.ClusterCachedModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterCachedModel), err
}

// List takes label and field selectors, and returns the list of ClusterCachedModels that match those selectors.
func (c *FakeClusterCachedModels) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ClusterCachedModelList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(clustercachedmodelsResource, clustercachedmodelsKind, c.ns, opts), &v1alpha1.ClusterCachedModelList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.ClusterCachedModelList{ListMeta: obj.(*v1alpha1.ClusterCachedModelList).ListMeta}
	for _, item := range obj.(*v1alpha1.ClusterCachedModelList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested clusterCachedModels.
func (c *FakeClusterCachedModels) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(clustercachedmodelsResource, c.ns, opts))

}

// Create takes the representation of a clusterCachedModel and creates it.  Returns the server's representation of the clusterCachedModel, and an error, if there is any.
func (c *FakeClusterCachedModels) Create(ctx context.Context, clusterCachedModel *v1alpha1.ClusterCachedModel, opts v1.CreateOptions) (result *v1alpha1.ClusterCachedModel, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(clustercachedmodelsResource, c.ns, clusterCachedModel), &v1alpha1.ClusterCachedModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterCachedModel), err
}

// Update takes the representation of a clusterCachedModel and updates it. Returns the server's representation of the clusterCachedModel, and an error, if there is any.
func (c *FakeClusterCachedModels) Update(ctx context.Context, clusterCachedModel *v1alpha1.ClusterCachedModel, opts v1.UpdateOptions) (result *v1alpha1.ClusterCachedModel, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(clustercachedmodelsResource, c.ns, clusterCachedModel), &v1alpha1.ClusterCachedModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterCachedModel), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeClusterCachedModels) UpdateStatus(ctx context.Context, clusterCachedModel *v1alpha1.ClusterCachedModel, opts v1.UpdateOptions) (*v1alpha1.ClusterCachedModel, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(clustercachedmodelsResource, "status", c.ns, clusterCachedModel), &v1alpha1.ClusterCachedModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterCachedModel), err
}

// Delete takes name of the clusterCachedModel and deletes it. Returns an error if one occurs.
func (c *FakeClusterCachedModels) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(clustercachedmodelsResource, c.ns, name, opts), &v1alpha1.ClusterCachedModel{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeClusterCachedModels) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(clustercachedmodelsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.ClusterCachedModelList{})
	return err
}

// Patch applies the patch and returns the patched clusterCachedModel.
func (c *FakeClusterCachedModels) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClusterCachedModel, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(clustercachedmodelsResource, c.ns, name, pt, data, subresources...), &v1alpha1.ClusterCachedModel{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterCachedModel), err
}