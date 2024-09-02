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
	scheme "github.com/kserve/kserve/serving/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// ClusterStorageContainersGetter has a method to return a ClusterStorageContainerInterface.
// A group's client should implement this interface.
type ClusterStorageContainersGetter interface {
	ClusterStorageContainers(namespace string) ClusterStorageContainerInterface
}

// ClusterStorageContainerInterface has methods to work with ClusterStorageContainer resources.
type ClusterStorageContainerInterface interface {
	Create(ctx context.Context, clusterStorageContainer *v1alpha1.ClusterStorageContainer, opts v1.CreateOptions) (*v1alpha1.ClusterStorageContainer, error)
	Update(ctx context.Context, clusterStorageContainer *v1alpha1.ClusterStorageContainer, opts v1.UpdateOptions) (*v1alpha1.ClusterStorageContainer, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*v1alpha1.ClusterStorageContainer, error)
	List(ctx context.Context, opts v1.ListOptions) (*v1alpha1.ClusterStorageContainerList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClusterStorageContainer, err error)
	ClusterStorageContainerExpansion
}

// clusterStorageContainers implements ClusterStorageContainerInterface
type clusterStorageContainers struct {
	client rest.Interface
	ns     string
}

// newClusterStorageContainers returns a ClusterStorageContainers
func newClusterStorageContainers(c *ServingV1alpha1Client, namespace string) *clusterStorageContainers {
	return &clusterStorageContainers{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the clusterStorageContainer, and returns the corresponding clusterStorageContainer object, and an error if there is any.
func (c *clusterStorageContainers) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.ClusterStorageContainer, err error) {
	result = &v1alpha1.ClusterStorageContainer{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("clusterstoragecontainers").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do(ctx).
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of ClusterStorageContainers that match those selectors.
func (c *clusterStorageContainers) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.ClusterStorageContainerList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.ClusterStorageContainerList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("clusterstoragecontainers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do(ctx).
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested clusterStorageContainers.
func (c *clusterStorageContainers) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("clusterstoragecontainers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch(ctx)
}

// Create takes the representation of a clusterStorageContainer and creates it.  Returns the server's representation of the clusterStorageContainer, and an error, if there is any.
func (c *clusterStorageContainers) Create(ctx context.Context, clusterStorageContainer *v1alpha1.ClusterStorageContainer, opts v1.CreateOptions) (result *v1alpha1.ClusterStorageContainer, err error) {
	result = &v1alpha1.ClusterStorageContainer{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("clusterstoragecontainers").
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterStorageContainer).
		Do(ctx).
		Into(result)
	return
}

// Update takes the representation of a clusterStorageContainer and updates it. Returns the server's representation of the clusterStorageContainer, and an error, if there is any.
func (c *clusterStorageContainers) Update(ctx context.Context, clusterStorageContainer *v1alpha1.ClusterStorageContainer, opts v1.UpdateOptions) (result *v1alpha1.ClusterStorageContainer, err error) {
	result = &v1alpha1.ClusterStorageContainer{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("clusterstoragecontainers").
		Name(clusterStorageContainer.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(clusterStorageContainer).
		Do(ctx).
		Into(result)
	return
}

// Delete takes name of the clusterStorageContainer and deletes it. Returns an error if one occurs.
func (c *clusterStorageContainers) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("clusterstoragecontainers").
		Name(name).
		Body(&opts).
		Do(ctx).
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *clusterStorageContainers) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	var timeout time.Duration
	if listOpts.TimeoutSeconds != nil {
		timeout = time.Duration(*listOpts.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("clusterstoragecontainers").
		VersionedParams(&listOpts, scheme.ParameterCodec).
		Timeout(timeout).
		Body(&opts).
		Do(ctx).
		Error()
}

// Patch applies the patch and returns the patched clusterStorageContainer.
func (c *clusterStorageContainers) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.ClusterStorageContainer, err error) {
	result = &v1alpha1.ClusterStorageContainer{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("clusterstoragecontainers").
		Name(name).
		SubResource(subresources...).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(data).
		Do(ctx).
		Into(result)
	return
}
