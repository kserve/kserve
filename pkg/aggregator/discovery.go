/*
Copyright 2026 The KServe Authors.

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

package aggregator

import (
	"context"
	"errors"

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	listersv1alpha2 "github.com/kserve/kserve/pkg/client/listers/serving/v1alpha2"
)

// StaticDiscovery returns a fixed backend list.
// Useful for tests and for embedding the library without Kubernetes.
type StaticDiscovery struct {
	Backends []Backend
}

// List implements BackendDiscovery.
func (d StaticDiscovery) List(context.Context) ([]Backend, error) {
	out := make([]Backend, len(d.Backends))
	copy(out, d.Backends)
	return out, nil
}

// ClientDiscovery lists Ready LLMInferenceService backends from a controller-runtime client.
// This is the discovery implementation used when embedding the aggregator in cmd/llmisvc.
type ClientDiscovery struct {
	Client    client.Reader
	Namespace string // empty means all namespaces
}

// List implements BackendDiscovery.
func (d ClientDiscovery) List(ctx context.Context) ([]Backend, error) {
	if d.Client == nil {
		return nil, errors.New("client is required")
	}

	list := &v1alpha2.LLMInferenceServiceList{}
	opts := []client.ListOption{}
	if d.Namespace != "" {
		opts = append(opts, client.InNamespace(d.Namespace))
	}
	if err := d.Client.List(ctx, list, opts...); err != nil {
		return nil, err
	}

	items := make([]*v1alpha2.LLMInferenceService, 0, len(list.Items))
	for i := range list.Items {
		items = append(items, &list.Items[i])
	}
	return backendsFromLLMInferenceServices(items), nil
}

// InformerDiscovery lists Ready LLMInferenceService backends from a generated lister.
// Callers that already run a SharedInformerFactory can use this instead of ClientDiscovery.
type InformerDiscovery struct {
	Lister    listersv1alpha2.LLMInferenceServiceLister
	Namespace string // empty means all namespaces
}

// List implements BackendDiscovery.
func (d InformerDiscovery) List(_ context.Context) ([]Backend, error) {
	if d.Lister == nil {
		return nil, errors.New("lister is required")
	}

	if d.Namespace != "" {
		items, err := d.Lister.LLMInferenceServices(d.Namespace).List(labels.Everything())
		if err != nil {
			return nil, err
		}
		return backendsFromLLMInferenceServices(items), nil
	}

	items, err := d.Lister.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	return backendsFromLLMInferenceServices(items), nil
}

func backendsFromLLMInferenceServices(items []*v1alpha2.LLMInferenceService) []Backend {
	backends := make([]Backend, 0, len(items))
	for _, svc := range items {
		if b, ok := BackendFromLLMInferenceService(svc); ok {
			backends = append(backends, b)
		}
	}
	return backends
}
