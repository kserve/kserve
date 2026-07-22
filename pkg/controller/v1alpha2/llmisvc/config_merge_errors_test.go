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

package llmisvc_test

// Most "config not found" coverage is provided by the envtest specs in
// controller_int_config_not_found_test.go, which exercise the real
// reconciliation path and assert on the user-facing Status condition
// Message. The unit-level tests below cover the two cases that don't
// fit naturally into an envtest cluster:
//
//   - the pure string-formatting contract of (*ConfigNotFoundError).Error()
//     across populated / empty / nil Available slices, where spinning up
//     an envtest apiserver would be overkill;
//
//   - the best-effort behaviour of listAvailableConfigs when a per-namespace
//     LIST fails (e.g. RBAC denial). Reproducing a List error scoped to a
//     single namespace is awkward under envtest, so we inject the error
//     via controller-runtime's fake client and interceptor.Funcs.

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/controller/v1alpha2/llmisvc"
)

func newConfigMergeTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha2 to scheme: %v", err)
	}
	return scheme
}

func newCfg(name, namespace string) *v1alpha2.LLMInferenceServiceConfig {
	return &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
}

func TestConfigNotFoundErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *llmisvc.ConfigNotFoundError
		want string
	}{
		{
			name: "available configs present - ns/name listed in sorted form",
			err: &llmisvc.ConfigNotFoundError{
				Name:       "kserve-config-llm-template",
				Namespaces: []string{"default", "kserve"},
				Available: []string{
					"default/my-llm-config",
					"kserve/kserve-config-llm-decode-template",
					"kserve/kserve-config-llm-scheduler",
				},
			},
			want: `LLMInferenceServiceConfig "kserve-config-llm-template" not found in namespaces [default kserve]; available configs: [default/my-llm-config kserve/kserve-config-llm-decode-template kserve/kserve-config-llm-scheduler]`,
		},
		{
			name: "empty available slice renders as []",
			err: &llmisvc.ConfigNotFoundError{
				Name:       "missing",
				Namespaces: []string{"default", "kserve"},
				Available:  []string{},
			},
			want: `LLMInferenceServiceConfig "missing" not found in namespaces [default kserve]; available configs: []`,
		},
		{
			name: "nil available slice also renders as []",
			err: &llmisvc.ConfigNotFoundError{
				Name:       "missing",
				Namespaces: []string{"default", "kserve"},
				Available:  nil,
			},
			want: `LLMInferenceServiceConfig "missing" not found in namespaces [default kserve]; available configs: []`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			g.Expect(tt.err.Error()).To(Equal(tt.want))
		})
	}
}

// TestListAvailableConfigs_SkipsFailingNamespace verifies that a LIST error
// scoped to a single namespace (e.g. an RBAC denial in that namespace only)
// does not poison the overall result — configs from the healthy namespaces
// are still returned. envtest cannot easily reproduce this without
// elaborate RBAC plumbing, so we inject the error via interceptor.Funcs.
func TestListAvailableConfigs_SkipsFailingNamespace(t *testing.T) {
	g := NewGomegaWithT(t)
	scheme := newConfigMergeTestScheme(t)

	listErr := errors.New("forbidden: user cannot list LLMInferenceServiceConfig in namespace \"forbidden-ns\"")

	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			newCfg("ok-config", "default"),
		).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				listOpts := &client.ListOptions{}
				for _, o := range opts {
					o.ApplyToList(listOpts)
				}
				if listOpts.Namespace == "forbidden-ns" {
					return listErr
				}
				return c.List(ctx, list, opts...)
			},
		}).
		Build()
	r := &llmisvc.LLMISVCReconciler{Client: fc}

	got := r.ListAvailableConfigsForTest(context.Background(), []string{"default", "forbidden-ns"})

	g.Expect(got).To(Equal([]string{"default/ok-config"}),
		"forbidden namespace should be silently dropped, default's configs preserved")
}
