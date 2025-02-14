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
	context "context"

	servingv1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	scheme "github.com/kserve/kserve/pkg/client/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// InferenceGraphsGetter has a method to return a InferenceGraphInterface.
// A group's client should implement this interface.
type InferenceGraphsGetter interface {
	InferenceGraphs(namespace string) InferenceGraphInterface
}

// InferenceGraphInterface has methods to work with InferenceGraph resources.
type InferenceGraphInterface interface {
	Create(ctx context.Context, inferenceGraph *servingv1alpha1.InferenceGraph, opts v1.CreateOptions) (*servingv1alpha1.InferenceGraph, error)
	Update(ctx context.Context, inferenceGraph *servingv1alpha1.InferenceGraph, opts v1.UpdateOptions) (*servingv1alpha1.InferenceGraph, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, inferenceGraph *servingv1alpha1.InferenceGraph, opts v1.UpdateOptions) (*servingv1alpha1.InferenceGraph, error)
	Delete(ctx context.Context, name string, opts v1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error
	Get(ctx context.Context, name string, opts v1.GetOptions) (*servingv1alpha1.InferenceGraph, error)
	List(ctx context.Context, opts v1.ListOptions) (*servingv1alpha1.InferenceGraphList, error)
	Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *servingv1alpha1.InferenceGraph, err error)
	InferenceGraphExpansion
}

// inferenceGraphs implements InferenceGraphInterface
type inferenceGraphs struct {
	*gentype.ClientWithList[*servingv1alpha1.InferenceGraph, *servingv1alpha1.InferenceGraphList]
}

// newInferenceGraphs returns a InferenceGraphs
func newInferenceGraphs(c *ServingV1alpha1Client, namespace string) *inferenceGraphs {
	return &inferenceGraphs{
		gentype.NewClientWithList[*servingv1alpha1.InferenceGraph, *servingv1alpha1.InferenceGraphList](
			"inferencegraphs",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *servingv1alpha1.InferenceGraph { return &servingv1alpha1.InferenceGraph{} },
			func() *servingv1alpha1.InferenceGraphList { return &servingv1alpha1.InferenceGraphList{} },
		),
	}
}
