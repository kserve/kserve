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
	scheme "github.com/kserve/kserve/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// InferenceGraphsGetter has a method to return a InferenceGraphInterface.
// A group's client should implement this interface.
type InferenceGraphsGetter interface {
	InferenceGraphs(namespace string) InferenceGraphInterface
}

// InferenceGraphInterface has methods to work with InferenceGraph resources.
type InferenceGraphInterface interface {
	Create(ctx context.Context, inferenceGraph *v1alpha1.InferenceGraph, opts v1.CreateOptions) (*v1alpha1.InferenceGraph, error)
	Update(ctx context.Context, inferenceGraph *v1alpha1.InferenceGraph, opts v1.UpdateOptions) (*v1alpha1.InferenceGraph, error)
	UpdateStatus(ctx context.Context, inferenceGraph *v1alpha1.InferenceGraph, opts v1.UpdateOptions) (*v1alpha1.InferenceGraph, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.InferenceGraph, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.InferenceGraphList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.InferenceGraph, err error)
	InferenceGraphExpansion
}

// inferenceGraphs implements InferenceGraphInterface
type inferenceGraphs struct {
	client rest.Interface
	ns     string
}

// newInferenceGraphs returns a InferenceGraphs
func newInferenceGraphs(c *ServingV1alpha1Client, namespace string) *inferenceGraphs {
	return &inferenceGraphs{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the inferenceGraph, and returns the corresponding inferenceGraph object, and an error if there is any.
func (c *inferenceGraphs) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.InferenceGraph, err error) {
	result = &v1alpha1.InferenceGraph{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("inferencegraphs").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of InferenceGraphs that match those selectors.
func (c *inferenceGraphs) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.InferenceGraphList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.InferenceGraphList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("inferencegraphs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested inferenceGraphs.
func (c *inferenceGraphs) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("inferencegraphs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a inferenceGraph and creates it.  Returns the server's representation of the inferenceGraph, and an error, if there is any.
func (c *inferenceGraphs) Create(ctx context.Context, inferenceGraph *v1alpha1.InferenceGraph, opts v1.CreateOptions) (result *v1alpha1.InferenceGraph, err error) {
	result = &v1alpha1.InferenceGraph{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("inferencegraphs").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(inferenceGraph).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a inferenceGraph and updates it. Returns the server's representation of the inferenceGraph, and an error, if there is any.
func (c *inferenceGraphs) Update(ctx context.Context, inferenceGraph *v1alpha1.InferenceGraph, opts v1.UpdateOptions) (result *v1alpha1.InferenceGraph, err error) {
	result = &v1alpha1.InferenceGraph{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("inferencegraphs").
		Name(inferenceGraph.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(inferenceGraph).
		Do(ctx).
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *inferenceGraphs) UpdateStatus(ctx context.Context, inferenceGraph *v1alpha1.InferenceGraph, opts v1.UpdateOptions) (result *v1alpha1.InferenceGraph, err error) {
	result = &v1alpha1.InferenceGraph{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("inferencegraphs").
		Name(inferenceGraph.Name).
		SubResource("status").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(inferenceGraph).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the inferenceGraph and deletes it. Returns an error if one occurs.
func (c *inferenceGraphs) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("inferencegraphs").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *inferenceGraphs) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("inferencegraphs").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched inferenceGraph.
func (c *inferenceGraphs) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.InferenceGraph, err error) {
	result = &v1alpha1.InferenceGraph{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("inferencegraphs").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
