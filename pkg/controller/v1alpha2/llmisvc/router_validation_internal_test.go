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
	"k8s.io/utils/ptr"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func newLLMISVC(name string, opts ...func(*v1alpha2.LLMInferenceService)) v1alpha2.LLMInferenceService {
	svc := v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
	for _, o := range opts {
		o(&svc)
	}

	return svc
}

func withModelName(name string) func(*v1alpha2.LLMInferenceService) {
	return func(svc *v1alpha2.LLMInferenceService) {
		svc.Spec.Model.Name = ptr.To(name)
	}
}

func withLoRAAdapter(names ...string) func(*v1alpha2.LLMInferenceService) {
	return func(svc *v1alpha2.LLMInferenceService) {
		if svc.Spec.Model.LoRA == nil {
			svc.Spec.Model.LoRA = &v1alpha2.LoRASpec{}
		}
		for _, n := range names {
			svc.Spec.Model.LoRA.Adapters = append(svc.Spec.Model.LoRA.Adapters,
				v1alpha2.LLMModelSpec{Name: ptr.To(n)})
		}
	}
}

func withGroup(group string) func(*v1alpha2.LLMInferenceService) {
	return func(svc *v1alpha2.LLMInferenceService) {
		if svc.Spec.Router == nil {
			svc.Spec.Router = &v1alpha2.RouterSpec{}
		}
		if svc.Spec.Router.Route == nil {
			svc.Spec.Router.Route = &v1alpha2.GatewayRoutesSpec{}
		}
		svc.Spec.Router.Route.Group = ptr.To(group)
	}
}

// withStatusModels sets status.addresses with the given model routing names.
// This is what peers look like after their own reconciliation.
func withStatusModels(models ...string) func(*v1alpha2.LLMInferenceService) {
	return func(svc *v1alpha2.LLMInferenceService) {
		ms := make([]v1alpha2.ModelSourcedAddressStatus, len(models))
		for i, m := range models {
			ms[i] = v1alpha2.ModelSourcedAddressStatus{Name: m}
		}
		svc.Status.Addresses = []v1alpha2.SourcedAddress{{Models: ms}}
	}
}

func withDeleting() func(*v1alpha2.LLMInferenceService) {
	return func(svc *v1alpha2.LLMInferenceService) {
		now := metav1.Now()
		svc.DeletionTimestamp = &now
	}
}

func TestFindModelNameCollisions(t *testing.T) {
	tests := []struct {
		name   string
		self   v1alpha2.LLMInferenceService
		others []v1alpha2.LLMInferenceService
		want   []string
	}{
		{
			name:   "no others",
			self:   newLLMISVC("a", withModelName("gpt-4o")),
			others: nil,
		},
		{
			name: "no collision - different model names",
			self: newLLMISVC("a", withModelName("gpt-4o")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withStatusModels("publishers/default/models/llama")),
			},
		},
		{
			name: "base model collision",
			self: newLLMISVC("a", withModelName("gpt-4o")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withStatusModels("publishers/default/models/gpt-4o")),
			},
			want: []string{"b"},
		},
		{
			name: "multiple collisions sorted",
			self: newLLMISVC("a", withModelName("gpt-4o")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("c", withStatusModels("publishers/default/models/gpt-4o")),
				newLLMISVC("b", withStatusModels("publishers/default/models/gpt-4o")),
			},
			want: []string{"b", "c"},
		},
		{
			name: "same group excluded",
			self: newLLMISVC("a", withModelName("gpt-4o"), withGroup("my-group")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withGroup("my-group"),
					withStatusModels("publishers/default/models/gpt-4o")),
			},
		},
		{
			name: "different groups collide",
			self: newLLMISVC("a", withModelName("gpt-4o"), withGroup("g1")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withGroup("g2"),
					withStatusModels("publishers/default/models/gpt-4o")),
			},
			want: []string{"b"},
		},
		{
			name: "self grouped other standalone - collision",
			self: newLLMISVC("a", withModelName("gpt-4o"), withGroup("g1")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withStatusModels("publishers/default/models/gpt-4o")),
			},
			want: []string{"b"},
		},
		{
			name: "self standalone other grouped - collision",
			self: newLLMISVC("a", withModelName("gpt-4o")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withGroup("g1"),
					withStatusModels("publishers/default/models/gpt-4o")),
			},
			want: []string{"b"},
		},
		{
			name: "nil model.name defaults to metadata.name",
			self: newLLMISVC("shared-name"),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withStatusModels("publishers/default/models/shared-name")),
			},
			want: []string{"b"},
		},
		{
			name: "self excluded from results",
			self: newLLMISVC("a", withModelName("gpt-4o")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("a", withStatusModels("publishers/default/models/gpt-4o")),
			},
		},
		{
			name: "peer being deleted skipped",
			self: newLLMISVC("a", withModelName("gpt-4o")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withDeleting(),
					withStatusModels("publishers/default/models/gpt-4o")),
			},
		},
		{
			name: "peer with empty status falls back to spec.model.name",
			self: newLLMISVC("a", withModelName("gpt-4o")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withModelName("gpt-4o")),
			},
			want: []string{"b"},
		},
		{
			name: "peer with empty status and different model.name - no collision",
			self: newLLMISVC("a", withModelName("gpt-4o")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withModelName("llama")),
			},
		},
		{
			name: "lora adapter on self collides with base model on peer",
			self: newLLMISVC("a", withModelName("base"), withLoRAAdapter("shared-adapter")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withStatusModels("publishers/default/models/shared-adapter")),
			},
			want: []string{"b"},
		},
		{
			name: "lora adapter on peer collides with base model on self",
			self: newLLMISVC("a", withModelName("gpt-4o")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withStatusModels(
					"publishers/default/models/other-base",
					"publishers/default/models/gpt-4o",
				)),
			},
			want: []string{"b"},
		},
		{
			name: "no collision when peer model-routing names differ",
			self: newLLMISVC("a", withModelName("gpt-4o"), withLoRAAdapter("adapter-a")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withStatusModels(
					"publishers/default/models/llama",
					"publishers/default/models/adapter-b",
				)),
			},
		},
		{
			name: "peer with empty status - lora adapter fallback catches collision",
			self: newLLMISVC("a", withModelName("base")),
			others: []v1alpha2.LLMInferenceService{
				newLLMISVC("b", withModelName("other"), withLoRAAdapter("base")),
			},
			want: []string{"b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findModelNameCollisions(&tt.self, tt.others)
			if !slices.Equal(got, tt.want) {
				t.Errorf("findModelNameCollisions() = %v, want %v", got, tt.want)
			}
		})
	}
}
