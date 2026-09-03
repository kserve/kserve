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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	listersv1alpha2 "github.com/kserve/kserve/pkg/client/listers/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func newTestLister(t *testing.T, items ...*v1alpha2.LLMInferenceService) listersv1alpha2.LLMInferenceServiceLister {
	t.Helper()
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		cache.NamespaceIndex: cache.MetaNamespaceIndexFunc,
	})
	for _, item := range items {
		require.NoError(t, indexer.Add(item))
	}
	return listersv1alpha2.NewLLMInferenceServiceLister(indexer)
}

func llmSvc(t *testing.T, name, namespace, url string, ready bool, stopped bool) *v1alpha2.LLMInferenceService {
	t.Helper()
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	svc := &v1alpha2.LLMInferenceService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       "LLMInferenceService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   apis.ConditionReady,
					Status: status,
				}},
			},
		},
	}
	if url != "" {
		svc.Status.Addresses = []v1alpha2.SourcedAddress{
			{Addressable: duckv1.Addressable{URL: mustURL(t, url)}},
		}
	}
	if stopped {
		svc.Annotations = map[string]string{constants.StopAnnotationKey: "true"}
	}
	return svc
}

func TestInformerDiscoveryList(t *testing.T) {
	readyA := llmSvc(t, "a", "ns1", "http://a.ns1.svc.cluster.local", true, false)
	readyB := llmSvc(t, "b", "ns2", "http://b.ns2.svc.cluster.local", true, false)
	notReady := llmSvc(t, "c", "ns1", "http://c.ns1.svc.cluster.local", false, false)
	stopped := llmSvc(t, "d", "ns1", "http://d.ns1.svc.cluster.local", true, true)
	noURL := llmSvc(t, "e", "ns1", "", true, false)

	lister := newTestLister(t, readyA, readyB, notReady, stopped, noURL)

	allNS, err := InformerDiscovery{Lister: lister}.List(t.Context())
	require.NoError(t, err)
	require.Len(t, allNS, 2)
	ids := []string{allNS[0].ID(), allNS[1].ID()}
	assert.ElementsMatch(t, []string{"ns1/a", "ns2/b"}, ids)

	ns1Only, err := InformerDiscovery{Lister: lister, Namespace: "ns1"}.List(t.Context())
	require.NoError(t, err)
	require.Len(t, ns1Only, 1)
	assert.Equal(t, "ns1/a", ns1Only[0].ID())
	assert.Equal(t, "http://a.ns1.svc.cluster.local", ns1Only[0].URL)
}

func TestInformerDiscoveryRequiresLister(t *testing.T) {
	_, err := InformerDiscovery{}.List(t.Context())
	require.Error(t, err)
}

func TestClientDiscoveryList(t *testing.T) {
	readyA := llmSvc(t, "a", "ns1", "http://a.ns1.svc.cluster.local", true, false)
	readyB := llmSvc(t, "b", "ns2", "http://b.ns2.svc.cluster.local", true, false)
	notReady := llmSvc(t, "c", "ns1", "http://c.ns1.svc.cluster.local", false, false)

	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(readyA, readyB, notReady).Build()

	allNS, err := ClientDiscovery{Client: c}.List(t.Context())
	require.NoError(t, err)
	require.Len(t, allNS, 2)
	ids := []string{allNS[0].ID(), allNS[1].ID()}
	assert.ElementsMatch(t, []string{"ns1/a", "ns2/b"}, ids)

	ns1Only, err := ClientDiscovery{Client: c, Namespace: "ns1"}.List(t.Context())
	require.NoError(t, err)
	require.Len(t, ns1Only, 1)
	assert.Equal(t, "ns1/a", ns1Only[0].ID())
}

func TestClientDiscoveryRequiresClient(t *testing.T) {
	_, err := ClientDiscovery{}.List(t.Context())
	require.Error(t, err)
}

// Ensure runtime.Object compliance for indexer helpers in this package's tests.
var _ runtime.Object = &v1alpha2.LLMInferenceService{}
