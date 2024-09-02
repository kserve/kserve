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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// InferenceGraphLister helps list InferenceGraphs.
// All objects returned here must be treated as read-only.
type InferenceGraphLister interface {
	// List lists all InferenceGraphs in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.InferenceGraph, err error)
	// InferenceGraphs returns an object that can list and get InferenceGraphs.
	InferenceGraphs(namespace string) InferenceGraphNamespaceLister
	InferenceGraphListerExpansion
}

// inferenceGraphLister implements the InferenceGraphLister interface.
type inferenceGraphLister struct {
	indexer cache.Indexer
}

// NewInferenceGraphLister returns a new InferenceGraphLister.
func NewInferenceGraphLister(indexer cache.Indexer) InferenceGraphLister {
	return &inferenceGraphLister{indexer: indexer}
}

// List lists all InferenceGraphs in the indexer.
func (s *inferenceGraphLister) List(selector labels.Selector) (ret []*v1alpha1.InferenceGraph, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.InferenceGraph))
	})
	return ret, err
}

// InferenceGraphs returns an object that can list and get InferenceGraphs.
func (s *inferenceGraphLister) InferenceGraphs(namespace string) InferenceGraphNamespaceLister {
	return inferenceGraphNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// InferenceGraphNamespaceLister helps list and get InferenceGraphs.
// All objects returned here must be treated as read-only.
type InferenceGraphNamespaceLister interface {
	// List lists all InferenceGraphs in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.InferenceGraph, err error)
	// Get retrieves the InferenceGraph from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.InferenceGraph, error)
	InferenceGraphNamespaceListerExpansion
}

// inferenceGraphNamespaceLister implements the InferenceGraphNamespaceLister
// interface.
type inferenceGraphNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all InferenceGraphs in the indexer for a given namespace.
func (s inferenceGraphNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.InferenceGraph, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.InferenceGraph))
	})
	return ret, err
}

// Get retrieves the InferenceGraph from the indexer for a given namespace and name.
func (s inferenceGraphNamespaceLister) Get(name string) (*v1alpha1.InferenceGraph, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("inferencegraph"), name)
	}
	return obj.(*v1alpha1.InferenceGraph), nil
}
