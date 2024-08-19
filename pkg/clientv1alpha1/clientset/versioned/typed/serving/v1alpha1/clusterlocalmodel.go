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

package v1alpha1

import (
	"context"
	"time"

	v1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	scheme "github.com/kserve/kserve/pkg/clientv1alpha1/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ClusterLocalModelsGetter has a method to return a ClusterLocalModelInterface.
// A group's client should implement this interface.
type ClusterLocalModelsGetter interface {
	ClusterLocalModels(namespace string) ClusterLocalModelInterface
}

// ClusterLocalModelInterface has methods to work with ClusterLocalModel resources.
type ClusterLocalModelInterface interface {
	Create(ctx context.Context, clusterLocalModel *v1alpha1.ClusterLocalModel, opts v1.CreateOptions) (*v1alpha1.ClusterLocalModel, error)
	Update(ctx context.Context, clusterLocalModel *v1alpha1.ClusterLocalModel, opts v1.UpdateOptions) (*v1alpha1.ClusterLocalModel, error)
	UpdateStatus(ctx context.Context, clusterLocalModel *v1alpha1.ClusterLocalModel, opts v1.UpdateOptions) (*v1alpha1.ClusterLocalModel, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.ClusterLocalModel, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.ClusterLocalModelList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClusterLocalModel, err error)
	ClusterLocalModelExpansion
}

// clusterLocalModels implements ClusterLocalModelInterface
type clusterLocalModels struct {
	client rest.Interface
	ns     string
}

// newClusterLocalModels returns a ClusterLocalModels
func newClusterLocalModels(c *ServingV1alpha1Client, namespace string) *clusterLocalModels {
	return &clusterLocalModels{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the clusterLocalModel, and returns the corresponding clusterLocalModel object, and an error if there is any.
func (c *clusterLocalModels) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ClusterLocalModel, err error) {
	result = &v1alpha1.ClusterLocalModel{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("clusterlocalmodels").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ClusterLocalModels that match those selectors.
func (c *clusterLocalModels) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ClusterLocalModelList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.ClusterLocalModelList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("clusterlocalmodels").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested clusterLocalModels.
func (c *clusterLocalModels) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("clusterlocalmodels").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a clusterLocalModel and creates it.  Returns the server's representation of the clusterLocalModel, and an error, if there is any.
func (c *clusterLocalModels) Create(ctx context.Context, clusterLocalModel *v1alpha1.ClusterLocalModel, opts v1.CreateOptions) (result *v1alpha1.ClusterLocalModel, err error) {
	result = &v1alpha1.ClusterLocalModel{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("clusterlocalmodels").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterLocalModel).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a clusterLocalModel and updates it. Returns the server's representation of the clusterLocalModel, and an error, if there is any.
func (c *clusterLocalModels) Update(ctx context.Context, clusterLocalModel *v1alpha1.ClusterLocalModel, opts v1.UpdateOptions) (result *v1alpha1.ClusterLocalModel, err error) {
	result = &v1alpha1.ClusterLocalModel{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("clusterlocalmodels").
		Name(clusterLocalModel.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterLocalModel).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *clusterLocalModels) UpdateStatus(ctx context.Context, clusterLocalModel *v1alpha1.ClusterLocalModel, opts v1.UpdateOptions) (result *v1alpha1.ClusterLocalModel, err error) {
	result = &v1alpha1.ClusterLocalModel{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("clusterlocalmodels").
		Name(clusterLocalModel.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterLocalModel).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the clusterLocalModel and deletes it. Returns an error if one occurs.
func (c *clusterLocalModels) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("clusterlocalmodels").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *clusterLocalModels) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("clusterlocalmodels").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched clusterLocalModel.
func (c *clusterLocalModels) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClusterLocalModel, err error) {
	result = &v1alpha1.ClusterLocalModel{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("clusterlocalmodels").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
