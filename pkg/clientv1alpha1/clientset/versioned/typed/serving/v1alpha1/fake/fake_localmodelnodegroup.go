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

// FakeLocalModelNodeGroups implements LocalModelNodeGroupInterface
type FakeLocalModelNodeGroups struct {
	Fake *FakeServingV1alpha1
	ns   string
}

var localmodelnodegroupsResource = v1alpha1.SchemeGroupVersion.WithResource("localmodelnodegroups")

var localmodelnodegroupsKind = v1alpha1.SchemeGroupVersion.WithKind("LocalModelNodeGroup")

// Get takes name of the localModelNodeGroup, and returns the corresponding localModelNodeGroup object, and an error if there is any.
func (c *FakeLocalModelNodeGroups) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.LocalModelNodeGroup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(localmodelnodegroupsResource, c.ns, name), &v1alpha1.LocalModelNodeGroup{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.LocalModelNodeGroup), err
}

// List takes label and field selectors, and returns the list of LocalModelNodeGroups that match those selectors.
func (c *FakeLocalModelNodeGroups) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.LocalModelNodeGroupList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(localmodelnodegroupsResource, localmodelnodegroupsKind, c.ns, opts), &v1alpha1.LocalModelNodeGroupList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.LocalModelNodeGroupList{ListMeta: obj.(*v1alpha1.LocalModelNodeGroupList).ListMeta}
	for _, item := range obj.(*v1alpha1.LocalModelNodeGroupList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested localModelNodeGroups.
func (c *FakeLocalModelNodeGroups) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(localmodelnodegroupsResource, c.ns, opts))

}

// Create takes the representation of a localModelNodeGroup and creates it.  Returns the server's representation of the localModelNodeGroup, and an error, if there is any.
func (c *FakeLocalModelNodeGroups) Create(ctx context.Context, localModelNodeGroup *v1alpha1.LocalModelNodeGroup, opts v1.CreateOptions) (result *v1alpha1.LocalModelNodeGroup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(localmodelnodegroupsResource, c.ns, localModelNodeGroup), &v1alpha1.LocalModelNodeGroup{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.LocalModelNodeGroup), err
}

// Update takes the representation of a localModelNodeGroup and updates it. Returns the server's representation of the localModelNodeGroup, and an error, if there is any.
func (c *FakeLocalModelNodeGroups) Update(ctx context.Context, localModelNodeGroup *v1alpha1.LocalModelNodeGroup, opts v1.UpdateOptions) (result *v1alpha1.LocalModelNodeGroup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(localmodelnodegroupsResource, c.ns, localModelNodeGroup), &v1alpha1.LocalModelNodeGroup{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.LocalModelNodeGroup), err
}

// Delete takes name of the localModelNodeGroup and deletes it. Returns an error if one occurs.
func (c *FakeLocalModelNodeGroups) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteActionWithOptions(localmodelnodegroupsResource, c.ns, name, opts), &v1alpha1.LocalModelNodeGroup{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeLocalModelNodeGroups) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(localmodelnodegroupsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.LocalModelNodeGroupList{})
	return err
}

// Patch applies the patch and returns the patched localModelNodeGroup.
func (c *FakeLocalModelNodeGroups) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.LocalModelNodeGroup, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(localmodelnodegroupsResource, c.ns, name, pt, data, subresources...), &v1alpha1.LocalModelNodeGroup{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.LocalModelNodeGroup), err
}