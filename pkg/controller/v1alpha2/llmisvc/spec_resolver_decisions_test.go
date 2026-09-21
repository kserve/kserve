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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
)

const resolverNamespace = "resolver-ns"

// decisions is the user-visible outcome of resolution: what status.appliedConfigRefs
// reports, and the model name a client addresses. Not what the selected presets render -
// that is mostly verbatim preset body, and config_merge_reconcile_test.go asserts it
// value by value.
//
// Compared as a whole struct, so a new field has to be stated by every case rather than
// defaulting silently.
type decisions struct {
	// applied is each config as "namespace/name Source", in application order. Namespace
	// is empty for cluster-scoped sources such as ClusterServingRuntime.
	applied []string
	// mergedModelName proves the copy-back happened: the service spec merges last, so its
	// webhook-defaulted name would otherwise always beat a baseRef-supplied one.
	mergedModelName string
	// renderedName is the --served-model-name that reached the engine command. Empty when
	// no selected config carries one.
	renderedName string
}

// resolverPresetFiles is every shipped preset, loaded for every case so that selection -
// not fixture availability - decides which ones apply.
var resolverPresetFiles = []string{
	"config-llm-template.yaml",
	"config-sglang-template.yaml",
	"config-llm-decode-template.yaml",
	"config-llm-prefill-template.yaml",
	"config-llm-worker-data-parallel.yaml",
	"config-llm-decode-worker-data-parallel.yaml",
	"config-llm-prefill-worker-data-parallel.yaml",
	"config-llm-scheduler.yaml",
	"config-llm-scheduler-eppconfig-default.yaml",
	"config-llm-scheduler-eppconfig-default-pd.yaml",
	"config-llm-router-route.yaml",
	"config-llm-tokenizer.yaml",
	"config-llm-tracing.yaml",
}

// resolverGlobalConfig is deliberately non-zero: these feed .GlobalConfig in the preset
// templates.
func resolverGlobalConfig() *Config {
	return &Config{
		SystemNamespace:             constants.KServeNamespace,
		IngressGatewayName:          "kserve-ingress-gateway",
		IngressGatewayNamespace:     "kserve",
		EnableTLS:                   true,
		ModelBasedRoutingHeaderName: "X-Gateway-Model-Name",
	}
}

func userConfig(name string, spec v1alpha2.LLMInferenceServiceSpec) *v1alpha2.LLMInferenceServiceConfig {
	return &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: resolverNamespace},
		Spec:       spec,
	}
}

func mustParseURL(raw string) *apis.URL {
	u, err := apis.ParseURL(raw)
	if err != nil {
		panic(err)
	}

	return u
}

func localRef(name string) corev1.LocalObjectReference {
	return corev1.LocalObjectReference{Name: name}
}

type resolverCase struct {
	// why states what this case pins that no other case does.
	why     string
	spec    v1alpha2.LLMInferenceServiceSpec
	extra   []client.Object
	want    decisions
	wantErr string
	// wantErrContains, for errors carrying a detail that is not ours to pin: a Go template
	// byte offset moves whenever any preset is edited.
	wantErrContains string
}

// TestSpecResolver_Decisions covers every preset-selection branch plus the precedence
// rules no other test pins. The corpus was sized by mutation testing - a flipped
// decision-merge order, a dropped model-name copy-back and a dropped tokenizer preset all
// passed the entire unit suite before these cases existed - so prune it against that
// rather than by eye.
func TestSpecResolver_Decisions(t *testing.T) {
	// The shipped scheduler preset declares 0.10.0 against a routerPresetMinVersion of
	// 0.11.0, so no default install reaches the EPPConfig injection branch. Shadowing it
	// with the version bumped is the only way in.
	bumpedScheduler := loadPresetConfig(t, "config-llm-scheduler.yaml")
	bumpedScheduler.Namespace = resolverNamespace
	bumpedScheduler.Spec.Router.Scheduler.Annotations["app.kubernetes.io/version"] = "0.11.0"

	cases := map[string]resolverCase{
		"single node": {
			why:  "baseline: the template preset selected for a service with no worker",
			spec: v1alpha2.LLMInferenceServiceSpec{},
			want: decisions{
				applied:         []string{"kserve/kserve-config-llm-template Preset"},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"single node with sglang runtime": {
			why: "selectSingleNodeTemplateName switches the template on spec.runtime",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Runtime: ptr.To(SGLangServingRuntimeName),
			},
			extra: []client.Object{
				&v1alpha1.ServingRuntime{
					ObjectMeta: metav1.ObjectMeta{Name: SGLangServingRuntimeName, Namespace: resolverNamespace},
					Spec: v1alpha1.ServingRuntimeSpec{
						ServingRuntimePodSpec: v1alpha1.ServingRuntimePodSpec{
							Containers: []corev1.Container{{Name: "main", Image: "sglang:pinned"}},
						},
					},
				},
			},
			want: decisions{
				applied: []string{
					"/kserve-llm-sglang ServingRuntime",
					"kserve/kserve-config-sglang-template Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"multi node data parallel": {
			why: "worker plus data parallelism selects the data-parallel preset",
			spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Worker:      &corev1.PodSpec{},
					Parallelism: &v1alpha2.ParallelismSpec{Data: ptr.To[int32](2)},
				},
			},
			want: decisions{
				applied:         []string{"kserve/kserve-config-llm-worker-data-parallel Preset"},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"prefill decode single node": {
			why:  "disaggregation splits into two template presets, one per role",
			spec: v1alpha2.LLMInferenceServiceSpec{Prefill: &v1alpha2.WorkloadSpec{}},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-prefill-template Preset",
					"kserve/kserve-config-llm-decode-template Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"prefill decode data parallel": {
			why: "disaggregation selects the data-parallel variant independently per role",
			spec: v1alpha2.LLMInferenceServiceSpec{
				WorkloadSpec: v1alpha2.WorkloadSpec{
					Worker:      &corev1.PodSpec{},
					Parallelism: &v1alpha2.ParallelismSpec{Data: ptr.To[int32](2)},
				},
				Prefill: &v1alpha2.WorkloadSpec{
					Worker:      &corev1.PodSpec{},
					Parallelism: &v1alpha2.ParallelismSpec{Data: ptr.To[int32](2)},
				},
			},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-prefill-worker-data-parallel Preset",
					"kserve/kserve-config-llm-decode-worker-data-parallel Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"router scheduler with shipped preset": {
			why: "the shipped scheduler preset is below routerPresetMinVersion, so no EPPConfig is injected",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{}},
			},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-scheduler Preset",
					"kserve/kserve-config-llm-template Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"router scheduler with shipped preset, disaggregated": {
			why: "same when disaggregated: still no EPPConfig, and both role templates apply",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Prefill: &v1alpha2.WorkloadSpec{},
				Router:  &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{}},
			},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-scheduler Preset",
					"kserve/kserve-config-llm-prefill-template Preset",
					"kserve/kserve-config-llm-decode-template Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"scheduler at min version injects the default EPPConfig": {
			why: "a scheduler config at or above routerPresetMinVersion reaches the injection branch",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{}},
			},
			extra: []client.Object{bumpedScheduler},
			want: decisions{
				applied: []string{
					"resolver-ns/kserve-config-llm-scheduler UserRef",
					"kserve/kserve-config-llm-scheduler-eppconfig-default Preset",
					"kserve/kserve-config-llm-template Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"scheduler at min version injects the disaggregated EPPConfig": {
			why: "the same branch selects the disaggregated EPPConfig when prefill is set",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Prefill: &v1alpha2.WorkloadSpec{},
				Router:  &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{}},
			},
			extra: []client.Object{bumpedScheduler},
			want: decisions{
				applied: []string{
					"resolver-ns/kserve-config-llm-scheduler UserRef",
					"kserve/kserve-config-llm-scheduler-eppconfig-default-pd Preset",
					"kserve/kserve-config-llm-prefill-template Preset",
					"kserve/kserve-config-llm-decode-template Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"route without refs": {
			why: "a route with no refs pulls the unversioned route preset",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{Route: &v1alpha2.GatewayRoutesSpec{}},
			},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-router-route Preset",
					"kserve/kserve-config-llm-template Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"tokenizer enabled on the service": {
			why: "spec.router.scheduler.tokenizer pulls the tokenizer preset",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{
					Tokenizer: &v1alpha2.TokenizerSpec{},
				}},
			},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-scheduler Preset",
					"kserve/kserve-config-llm-tokenizer Preset",
					"kserve/kserve-config-llm-template Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"tracing enabled": {
			why:  "a non-nil spec.tracing pulls the tracing preset",
			spec: v1alpha2.LLMInferenceServiceSpec{Tracing: &v1alpha2.TracingSpec{}},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-tracing Preset",
					"kserve/kserve-config-llm-template Preset",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"baseRef supplies the model name": {
			why: "the service spec merges last, so only the copy-back lets a baseRef supply model.name",
			spec: v1alpha2.LLMInferenceServiceSpec{
				BaseRefs: []corev1.LocalObjectReference{localRef("named-model")},
			},
			extra: []client.Object{
				userConfig("named-model", v1alpha2.LLMInferenceServiceSpec{
					Model: v1alpha2.LLMModelSpec{Name: ptr.To("from-baseref")},
				}),
			},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-template Preset",
					"resolver-ns/named-model UserRef",
				},
				mergedModelName: "from-baseref",
				renderedName:    "from-baseref",
			},
		},
		"baseRef changes which template is selected": {
			why: "the decision merge puts baseRefs last, so a baseRef can select a preset the service did not ask for",
			spec: v1alpha2.LLMInferenceServiceSpec{
				BaseRefs: []corev1.LocalObjectReference{localRef("adds-worker")},
			},
			extra: []client.Object{
				userConfig("adds-worker", v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Worker:      &corev1.PodSpec{},
						Parallelism: &v1alpha2.ParallelismSpec{Data: ptr.To[int32](4)},
					},
				}),
			},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-worker-data-parallel Preset",
					"resolver-ns/adds-worker UserRef",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"baseRef alone enables the tokenizer": {
			why: "tokenizer selection reads the decision merge, so a baseRef alone can pull the preset",
			spec: v1alpha2.LLMInferenceServiceSpec{
				BaseRefs: []corev1.LocalObjectReference{localRef("adds-tokenizer")},
			},
			extra: []client.Object{
				userConfig("adds-tokenizer", v1alpha2.LLMInferenceServiceSpec{
					Router: &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{
						Tokenizer: &v1alpha2.TokenizerSpec{},
					}},
				}),
			},
			want: decisions{
				applied: []string{
					"kserve/kserve-config-llm-scheduler Preset",
					"kserve/kserve-config-llm-tokenizer Preset",
					"kserve/kserve-config-llm-template Preset",
					"resolver-ns/adds-tokenizer UserRef",
				},
				mergedModelName: "defaulted-model",
				renderedName:    "defaulted-model",
			},
		},
		"namespace-local copy shadows the shipped preset": {
			why:  "getConfig prefers the service namespace, and a shadowing copy reports as UserRef",
			spec: v1alpha2.LLMInferenceServiceSpec{},
			extra: []client.Object{
				userConfig(configTemplateName, v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{Name: "main", Image: "tenant/vllm:shadow"}},
						},
					},
				}),
			},
			want: decisions{
				applied:         []string{"resolver-ns/kserve-config-llm-template UserRef"},
				mergedModelName: "defaulted-model",
				// The shadowing copy carries no engine command, so nothing renders a
				// served-model-name. Shadowing a preset replaces it rather than layering
				// onto it.
				renderedName: "",
			},
		},
		"baseRef template dereferences a field the service leaves unset": {
			why: "a template that renders against one service and not another must fail as an error, not a nil deref",
			spec: v1alpha2.LLMInferenceServiceSpec{
				BaseRefs: []corev1.LocalObjectReference{localRef("bad-template")},
			},
			extra: []client.Object{
				userConfig("bad-template", v1alpha2.LLMInferenceServiceSpec{
					WorkloadSpec: v1alpha2.WorkloadSpec{
						Template: &corev1.PodSpec{
							Containers: []corev1.Container{{
								Name: "main",
								Args: []string{"--tensor-parallel-size={{ .Spec.Parallelism.Data }}"},
							}},
						},
					},
				}),
			},
			wantErrContains: "nil pointer evaluating *v1alpha2.ParallelismSpec.Data",
		},
		"scheduler config ConfigMap is missing the referenced key": {
			why: "the ConfigMap resolves but carries no such key - a distinct error return from a missing ConfigMap",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Router: &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{
					Config: &v1alpha2.SchedulerConfigSpec{
						Ref: &corev1.ConfigMapKeySelector{
							LocalObjectReference: localRef("keyless-epp"),
							Key:                  "epp.yaml",
						},
					},
				}},
			},
			extra: []client.Object{&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "keyless-epp", Namespace: resolverNamespace},
				Data:       map[string]string{"something-else": "{}"},
			}},
			wantErr: `ConfigMap resolver-ns/keyless-epp doesn't have key "epp.yaml" in data`,
		},
		"LoRA adapter uses an unsupported URI scheme": {
			why: "adapter validation is the last error return in resolution, after rendering",
			spec: v1alpha2.LLMInferenceServiceSpec{
				Model: v1alpha2.LLMModelSpec{
					LoRA: &v1alpha2.LoRASpec{Adapters: []v1alpha2.LLMModelSpec{{
						Name: ptr.To("bad-adapter"),
						URI:  *mustParseURL("oci://registry.example.com/adapter:v1"),
					}}},
				},
			},
			wantErrContains: "oci:// is not supported for LoRA adapters",
		},
		"unresolvable baseRef": {
			why: "an unresolvable baseRef is a typed configNotFoundError naming both namespaces",
			spec: v1alpha2.LLMInferenceServiceSpec{
				BaseRefs: []corev1.LocalObjectReference{localRef("does-not-exist")},
			},
			wantErr: `LLMInferenceServiceConfig "does-not-exist" not found in namespaces [resolver-ns kserve]`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			require.NotEmpty(t, tc.why, "every case states what it pins")

			// given
			resolver, llmSvc := resolverFixture(t, tc)
			untouched := llmSvc.DeepCopy()

			// when
			got, err := resolver.Resolve(t.Context(), llmSvc, resolverGlobalConfig())

			// then
			assert.Equal(t, untouched, llmSvc,
				"Resolve must not touch the service it is given: a scrape or a watch mapper hands in an object it does not own")

			if tc.wantErr != "" || tc.wantErrContains != "" {
				require.Error(t, err)
				// A non-nil error always comes with a nil result, so a caller cannot
				// mistake a half-resolved spec for a whole one.
				assert.Nil(t, got, "resolution failed, so there is no effective spec to return")
				if tc.wantErr != "" {
					assert.Equal(t, tc.wantErr, err.Error(), tc.why)
				} else {
					assert.Contains(t, err.Error(), tc.wantErrContains, tc.why)
				}

				return
			}
			require.NoError(t, err)

			assert.Equal(t, tc.want, describeDecisions(got), tc.why)
		})
	}
}

func resolverFixture(t *testing.T, tc resolverCase) (*SpecResolver, *v1alpha2.LLMInferenceService) {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))
	// spec.runtime resolution does a typed Get, so these are needed even by cases that
	// name no runtime.
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	objs := make([]client.Object, 0, len(resolverPresetFiles)+len(tc.extra))
	for _, f := range resolverPresetFiles {
		preset := loadPresetConfig(t, f)
		preset.Namespace = constants.KServeNamespace
		objs = append(objs, preset)
	}
	objs = append(objs, tc.extra...)

	uri, err := apis.ParseURL("hf://meta-llama/Llama-3.1-8B-Instruct")
	require.NoError(t, err)

	spec := *tc.spec.DeepCopy()
	spec.Model.URI = *uri
	if spec.Model.Name == nil {
		// The defaulting webhook always leaves a name on the service. Mirroring that here
		// is what makes the baseRef model-name case meaningful.
		spec.Model.Name = ptr.To("defaulted-model")
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "resolver-llm", Namespace: resolverNamespace},
		Spec:       spec,
	}

	return &SpecResolver{
		Client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		Clientset: k8sfake.NewSimpleClientset(),
		Recorder:  record.NewFakeRecorder(100),
	}, llmSvc
}

func describeDecisions(got *EffectiveSpec) decisions {
	out := decisions{
		applied:         make([]string, 0, len(got.Applied)),
		mergedModelName: ptr.Deref(got.Spec.Model.Name, ""),
		renderedName:    renderedServedModelName(&got.Spec),
	}
	for _, ref := range got.Applied {
		out.applied = append(out.applied, fmt.Sprintf("%s/%s %s", ref.Namespace, ref.Name, ref.Source))
	}

	return out
}

// renderedServedModelName extracts --served-model-name from the first rendered engine
// command, the observable consequence of the model-name copy-back.
func renderedServedModelName(spec *v1alpha2.LLMInferenceServiceSpec) string {
	pods := []*corev1.PodSpec{spec.Template, spec.Worker}
	if spec.Prefill != nil {
		pods = append(pods, spec.Prefill.Template, spec.Prefill.Worker)
	}
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		for i := range pod.Containers {
			for _, part := range append(pod.Containers[i].Command, pod.Containers[i].Args...) {
				if name, found := servedModelNameFrom(part); found {
					return name
				}
			}
		}
	}

	return ""
}

func servedModelNameFrom(s string) (string, bool) {
	const flagName = "--served-model-name "
	_, after, found := strings.Cut(s, flagName)
	if !found {
		return "", false
	}
	value := after
	if idx := strings.IndexAny(value, " \n\\"); idx >= 0 {
		value = value[:idx]
	}

	return strings.Trim(value, `"'`), true
}
