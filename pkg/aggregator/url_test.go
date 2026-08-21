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
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

func mustURL(t *testing.T, raw string) *apis.URL {
	t.Helper()
	u, err := apis.ParseURL(raw)
	require.NoError(t, err)
	return u
}

func ptr(s string) *string { return &s }

func TestResolveBackendURLPrefersClusterLocal(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{
		Status: v1alpha2.LLMInferenceServiceStatus{
			URL: mustURL(t, "https://public.example.com"),
			Addresses: []v1alpha2.SourcedAddress{
				{Addressable: duckv1.Addressable{URL: mustURL(t, "https://public.example.com")}},
				{Addressable: duckv1.Addressable{URL: mustURL(t, "http://llama.default.svc.cluster.local")}},
			},
		},
	}
	assert.Equal(t, "http://llama.default.svc.cluster.local", ResolveBackendURL(svc))
}

func TestResolveBackendURLFallsBackToStatusURL(t *testing.T) {
	svc := &v1alpha2.LLMInferenceService{
		Status: v1alpha2.LLMInferenceServiceStatus{
			URL: mustURL(t, "http://only-status-url.example"),
		},
	}
	assert.Equal(t, "http://only-status-url.example", ResolveBackendURL(svc))
}

func TestBackendFromLLMInferenceService(t *testing.T) {
	ready := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "llama", Namespace: "ns"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{Name: ptr("llama")},
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			Status: duckv1.Status{
				Conditions: duckv1.Conditions{{
					Type:   apis.ConditionReady,
					Status: corev1.ConditionTrue,
				}},
			},
			Addresses: []v1alpha2.SourcedAddress{
				{
					Addressable: duckv1.Addressable{URL: mustURL(t, "http://llama.ns.svc.cluster.local")},
					Models:      []v1alpha2.ModelSourcedAddressStatus{{Name: "llama"}},
				},
			},
		},
	}
	b, ok := BackendFromLLMInferenceService(ready)
	require.True(t, ok)
	assert.Equal(t, "llama", b.Name)
	assert.Equal(t, "ns", b.Namespace)
	assert.Equal(t, "http://llama.ns.svc.cluster.local", b.URL)
	assert.Equal(t, []string{"llama"}, b.Models)

	stopped := ready.DeepCopy()
	stopped.Annotations = map[string]string{constants.StopAnnotationKey: "true"}
	_, ok = BackendFromLLMInferenceService(stopped)
	assert.False(t, ok)

	notReady := ready.DeepCopy()
	notReady.Status.Conditions = duckv1.Conditions{{
		Type:   apis.ConditionReady,
		Status: corev1.ConditionFalse,
	}}
	_, ok = BackendFromLLMInferenceService(notReady)
	assert.False(t, ok)

	noURL := ready.DeepCopy()
	noURL.Status.Addresses = nil
	noURL.Status.URL = nil
	_, ok = BackendFromLLMInferenceService(noURL)
	assert.False(t, ok)
}
