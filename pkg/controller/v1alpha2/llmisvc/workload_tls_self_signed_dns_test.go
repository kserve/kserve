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

package llmisvc

import (
	"slices"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	igwapi "sigs.k8s.io/gateway-api-inference-extension/api/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func TestCollectDNSNamesIncludesEPPService(t *testing.T) {
	tests := []struct {
		name   string
		router *v1alpha2.RouterSpec
	}{
		{
			name: "default router",
		},
		{
			name: "inline pool",
			router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "models"},
				Spec:       v1alpha2.LLMInferenceServiceSpec{Router: tt.router},
			}

			got := (&LLMISVCReconciler{}).collectDNSNames(t.Context(), llmSvc)
			want := []string{
				"model-epp-service.models.svc.cluster.local",
				"model-epp-service.models.svc",
			}
			for _, dnsName := range want {
				if !slices.Contains(got, dnsName) {
					t.Errorf("collectDNSNames() missing %q: %v", dnsName, got)
				}
			}
		})
	}
}

func TestCollectDNSNamesCompactsEPPService(t *testing.T) {
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "model", Namespace: "models"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Spec: &igwapi.InferencePoolSpec{
							EndpointPickerRef: &igwapi.EndpointPickerRef{
								Name: "model-epp-service",
							},
						},
					},
				},
			},
		},
	}

	got := (&LLMISVCReconciler{}).collectDNSNames(t.Context(), llmSvc)
	count := 0
	for _, dnsName := range got {
		if dnsName == "model-epp-service.models.svc" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("collectDNSNames() returned %d copies of the EPP service DNS name: %v", count, got)
	}
}
