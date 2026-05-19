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

import (
	"context"
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
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

func TestListAvailableConfigs(t *testing.T) {
	t.Run("returns sorted ns/name pairs across namespaces", func(t *testing.T) {
		g := NewGomegaWithT(t)
		scheme := newConfigMergeTestScheme(t)

		fc := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				newCfg("zeta", "default"),
				newCfg("alpha", "default"),
				newCfg("kserve-config-llm-decode-template", "kserve"),
				newCfg("kserve-config-llm-scheduler", "kserve"),
			).
			Build()
		r := &llmisvc.LLMISVCReconciler{Client: fc}

		got := r.ListAvailableConfigsForTest(context.Background(), []string{"default", "kserve"})

		g.Expect(got).To(Equal([]string{
			"default/alpha",
			"default/zeta",
			"kserve/kserve-config-llm-decode-template",
			"kserve/kserve-config-llm-scheduler",
		}))
	})

	t.Run("disambiguates same-named configs in different namespaces", func(t *testing.T) {
		g := NewGomegaWithT(t)
		scheme := newConfigMergeTestScheme(t)

		fc := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				newCfg("foo", "default"),
				newCfg("foo", "kserve"),
			).
			Build()
		r := &llmisvc.LLMISVCReconciler{Client: fc}

		got := r.ListAvailableConfigsForTest(context.Background(), []string{"default", "kserve"})

		g.Expect(got).To(Equal([]string{"default/foo", "kserve/foo"}))
	})

	t.Run("empty result when no configs exist in any namespace", func(t *testing.T) {
		g := NewGomegaWithT(t)
		scheme := newConfigMergeTestScheme(t)

		fc := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &llmisvc.LLMISVCReconciler{Client: fc}

		got := r.ListAvailableConfigsForTest(context.Background(), []string{"default", "kserve"})

		g.Expect(got).To(BeEmpty())
	})

	t.Run("best-effort: namespace with failing list is skipped, others returned", func(t *testing.T) {
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
	})
}

// TestGetConfig_NotFound_PopulatesAvailable is the end-to-end check that the
// configNotFoundError returned by getConfig carries the namespace/name
// formatted Available list — the user-facing payoff of listAvailableConfigs.
func TestGetConfig_NotFound_PopulatesAvailable(t *testing.T) {
	g := NewGomegaWithT(t)
	scheme := newConfigMergeTestScheme(t)

	fc := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			newCfg("kserve-config-llm-decode-template", constants.KServeNamespace),
			newCfg("kserve-config-llm-scheduler", constants.KServeNamespace),
			newCfg("user-custom-config", "user-ns"),
		).
		Build()
	r := &llmisvc.LLMISVCReconciler{Client: fc}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: "user-ns"},
	}

	_, err := r.GetConfigForTest(context.Background(), llmSvc, "does-not-exist")

	g.Expect(err).To(HaveOccurred())

	var cfgNotFound *llmisvc.ConfigNotFoundError
	g.Expect(errors.As(err, &cfgNotFound)).To(BeTrue(), "error should unwrap to *ConfigNotFoundError")
	g.Expect(cfgNotFound.Name).To(Equal("does-not-exist"))
	g.Expect(cfgNotFound.Namespaces).To(Equal([]string{"user-ns", constants.KServeNamespace}))
	g.Expect(cfgNotFound.Available).To(ConsistOf(
		fmt.Sprintf("%s/%s", constants.KServeNamespace, "kserve-config-llm-decode-template"),
		fmt.Sprintf("%s/%s", constants.KServeNamespace, "kserve-config-llm-scheduler"),
		"user-ns/user-custom-config",
	))
	g.Expect(cfgNotFound.Error()).To(ContainSubstring(`"does-not-exist" not found`))
	g.Expect(cfgNotFound.Error()).To(ContainSubstring("user-ns/user-custom-config"))
}
