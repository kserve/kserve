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
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	kservetesting "github.com/kserve/kserve/pkg/testing"
	"github.com/kserve/kserve/pkg/utils"
)

func TestReconcileBaseRefs_DryRunValidatesRenderedSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{{Name: "custom-config"}},
			Model: v1alpha2.LLMModelSpec{
				Name: ptr.To("test-model"),
			},
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			AppliedConfigRefs: []v1alpha2.AppliedConfigRef{{
				Name:      "stale-config",
				Namespace: "test-ns",
				Source:    v1alpha2.AppliedConfigSourcePreset,
			}},
		},
	}
	template := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configTemplateName,
			Namespace: constants.KServeNamespace,
		},
	}
	// Carries a field the service does not set, so the assertions below can tell
	// the merged spec apart from the service's own.
	customConfig := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-config",
			Namespace: llmSvc.Namespace,
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{Replicas: ptr.To(int32(3))},
		},
	}

	validationErr := apierrors.NewInvalid(
		v1alpha2.LLMInferenceServiceGVK.GroupKind(),
		"test-llm-validation-x7k2p",
		field.ErrorList{
			field.Invalid(
				field.NewPath("spec", "router", "route", "http", "spec", "rules").Index(0).
					Child("matches").Index(0).Child("headers").Index(0).Child("name"),
				"invalid header",
				// The real gateway-api pattern, kept verbatim: the '%' in it is what
				// catches a message being used as a format string.
				`must match "^[A-Za-z0-9!#$%&'*+\-.^_|~]+$"`,
			),
		},
	)
	var validated *v1alpha2.LLMInferenceService
	var createOptions client.CreateOptions
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(template, customConfig).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				var ok bool
				validated, ok = obj.(*v1alpha2.LLMInferenceService)
				require.True(t, ok)
				for _, opt := range opts {
					opt.ApplyToCreate(&createOptions)
				}
				return validationErr
			},
		}).
		Build()
	reconciler := &LLMISVCReconciler{
		Client:        fakeClient,
		EventRecorder: record.NewFakeRecorder(10),
	}

	combined, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})

	require.ErrorIs(t, err, reconcile.TerminalError(nil),
		"a rejected spec is terminal - retrying it changes nothing")
	assert.Empty(t, combined, "a failed resolution returns the zero spec")
	require.NotNil(t, validated)
	assert.Empty(t, validated.Name, "a fixed name would collide with a real <name>-validation service")
	assert.Equal(t, "test-llm-validation-", validated.GenerateName)
	assert.Equal(t, llmSvc.Namespace, validated.Namespace)
	assert.Equal(t, llmSvc.Spec.Model, validated.Spec.Model)
	assert.Equal(t, ptr.To(int32(3)), validated.Spec.Replicas,
		"the merged spec must be validated, not the service's own spec")
	assert.Equal(t, []string{metav1.DryRunAll}, createOptions.DryRun)
	condition := llmSvc.Status.GetCondition(v1alpha2.PresetsCombined)
	require.NotNil(t, condition)
	assert.Equal(t, "InvalidRenderedConfig", condition.Reason)
	assert.Contains(t, condition.Message, "Preset:kserve/kserve-config-llm-template")
	assert.Contains(t, condition.Message, "UserRef:test-ns/custom-config")
	assert.Contains(t, condition.Message,
		"spec.router.route.http.spec.rules[0].matches[0].headers[0].name",
		"the offending field path should be surfaced")
	assert.NotContains(t, condition.Message, "test-llm-validation",
		"the synthetic dry-run object name must not leak into the user-facing message")
	assert.NotContains(t, condition.Message, "%!",
		"apiserver text must be passed as an argument, not as a format string")
	assert.NotEmpty(t, llmSvc.Status.AppliedConfigRefs, "AppliedConfigRefs should be retained for provenance")

	ready := llmSvc.Status.GetCondition(apis.ConditionReady)
	require.NotNil(t, ready)
	assert.True(t, ready.IsFalse(), "Ready should be False when the rendered config is invalid")
	assert.Equal(t, "InvalidRenderedConfig", ready.Reason)
	assert.Equal(t, condition.Message, ready.Message, "the exact cause should bubble up to Ready")
}

// TestSpecResolver_TransferSlot pins the upgrade contract end to end: a
// preset declaring the slot gets the argument filled in from the merged spec, and
// one without it comes back untouched.
func TestSpecResolver_TransferSlot(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	secondary := []v1alpha2.SecondaryTierSpec{{FileSystem: &v1alpha2.FileSystemTierSpec{
		EmptyDir: &v1alpha2.EmptyDirTierSpec{Size: resource.MustParse("10Gi")},
	}}}
	for _, tt := range []struct {
		name    string
		hasSlot bool
	}{
		{name: "preset without the slot", hasSlot: false},
		{name: "preset declaring the slot", hasSlot: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := []corev1.EnvVar(nil)
			if tt.hasSlot {
				env = []corev1.EnvVar{{Name: kvTransferArgsEnvVar}}
			}
			template := &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: configTemplateName, Namespace: constants.KServeNamespace},
				Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: mainContainerName,
							Env:  env,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
							},
						}},
						Volumes: []corev1.Volume{{
							Name: "dshm",
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
								SizeLimit: ptr.To(resource.MustParse("1Gi")),
							}},
						}},
					},
				}},
			}
			custom := &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "kv-config", Namespace: "test-ns"},
				Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
					KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse("10Gi"), Secondary: secondary},
				}},
			}
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: "test-ns"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{{Name: custom.Name}},
				},
			}
			reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template, custom).Build()}

			combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})
			require.NoError(t, err)
			require.NotNil(t, combined)
			require.NotNil(t, combined.Spec.Template)
			podSpec := combined.Spec.Template
			require.Len(t, podSpec.Containers, 1)
			main := &podSpec.Containers[0]

			// This preset asks for no headroom, so its declared size stands.
			// Secondary-tier volumes are attached later by the workload builders.
			require.Len(t, podSpec.Volumes, 1)
			assert.Equal(t, "dshm", podSpec.Volumes[0].Name)
			assert.Equal(t, "1Gi", podSpec.Volumes[0].EmptyDir.SizeLimit.String())
			assert.Equal(t, "4Gi", main.Resources.Requests.Memory().String())

			if !tt.hasSlot {
				_, present := utils.GetEnvVarValue(main.Env, kvTransferArgsEnvVar)
				assert.False(t, present)
				return
			}
			transfer, filled := utils.GetEnvVarValue(main.Env, kvTransferArgsEnvVar)
			require.True(t, filled)
			assert.Contains(t, transfer, `"spec_name":"TieringOffloadingSpec"`)
			assert.Contains(t, transfer, `"cpu_bytes_to_use":10737418240`)
		})
	}
}

// A cpu that makes no sense still has to render. Config merging runs on every
// reconcile of every service, so failing here would take down a workload that was
// running before the slot existed, on nothing more than a controller upgrade.
func TestSpecResolver_RendersDegenerateKVCacheCPUFromBaseRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	for _, cpu := range []string{"0", "-1Gi"} {
		t.Run(cpu, func(t *testing.T) {
			template := &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: configTemplateName, Namespace: constants.KServeNamespace},
				Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
					Template: &corev1.PodSpec{Containers: []corev1.Container{{
						Name: mainContainerName,
						Env:  []corev1.EnvVar{{Name: kvTransferArgsEnvVar}},
					}}},
				}},
			}
			custom := &v1alpha2.LLMInferenceServiceConfig{
				ObjectMeta: metav1.ObjectMeta{Name: "kv-config", Namespace: "test-ns"},
				Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
					KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse(cpu)},
				}},
			}
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: "test-ns"},
				Spec: v1alpha2.LLMInferenceServiceSpec{
					BaseRefs: []corev1.LocalObjectReference{{Name: custom.Name}},
				},
			}
			reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template, custom).Build()}

			combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})
			require.NoError(t, err)

			require.NotNil(t, combined.Spec.Template)
			require.Len(t, combined.Spec.Template.Containers, 1)
			transfer, filled := utils.GetEnvVarValue(combined.Spec.Template.Containers[0].Env, kvTransferArgsEnvVar)
			require.True(t, filled)
			assert.Contains(t, transfer, "cpu_bytes_to_use")
		})
	}
}

func TestReconcileBaseRefs_InvalidConfigClearsStaleReady(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: "test-ns"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
		},
	}
	// Simulate a service that reconciled successfully before its preset was edited.
	llmSvc.MarkPresetsCombinedReady()
	conditions := llmSvc.GetConditionSet().Manage(llmSvc.GetStatus())
	conditions.MarkTrue(v1alpha2.WorkloadReady)
	conditions.MarkTrue(v1alpha2.RouterReady)
	require.True(t, llmSvc.Status.GetCondition(apis.ConditionReady).IsTrue())

	template := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: configTemplateName, Namespace: constants.KServeNamespace},
	}
	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(template).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
					return apierrors.NewInvalid(
						v1alpha2.LLMInferenceServiceGVK.GroupKind(), "test-llm-validation",
						field.ErrorList{field.Required(field.NewPath("spec", "model", "uri"), "must be set")})
				},
			}).
			Build(),
		EventRecorder: record.NewFakeRecorder(1),
	}

	_, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})
	require.ErrorIs(t, err, reconcile.TerminalError(nil))

	ready := llmSvc.Status.GetCondition(apis.ConditionReady)
	require.NotNil(t, ready)
	assert.True(t, ready.IsFalse(),
		"Ready must not stay True off stale sub-conditions when the controller stops before reconciling children")
	assert.Contains(t, ready.Message, "spec.model.uri")
}

func TestReconcileBaseRefs_DryRunTransientFailureRequeues(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: "test-ns"},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
		},
	}
	template := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: configTemplateName, Namespace: constants.KServeNamespace},
	}

	transientErr := apierrors.NewInternalError(errors.New(
		`failed calling webhook "llminferenceservice.kserve-webhook-server.validator": connection refused`))
	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(template).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
					return transientErr
				},
			}).
			Build(),
		EventRecorder: record.NewFakeRecorder(10),
	}

	combined, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})

	require.Error(t, err, "transient dry-run failures must requeue")
	assert.ErrorIs(t, err, transientErr)
	assert.Empty(t, combined, "a failed resolution returns the zero spec")
	condition := llmSvc.Status.GetCondition(v1alpha2.PresetsCombined)
	require.NotNil(t, condition)
	assert.True(t, condition.IsUnknown(),
		"a dry-run that never reached a verdict is Unknown, not a rejection")
	assert.Equal(t, "ValidationUnavailable", condition.Reason)
	assert.Contains(t, condition.Message, "connection refused", "the transient cause should be reported")
	assert.NotEmpty(t, llmSvc.Status.AppliedConfigRefs, "AppliedConfigRefs should be retained for provenance")

	// Ready must not read False - the config did not change, only the check failed.
	ready := llmSvc.Status.GetCondition(apis.ConditionReady)
	require.NotNil(t, ready)
	assert.True(t, ready.IsUnknown())
	assert.Equal(t, "ValidationUnavailable", ready.Reason)
}

func TestSpecResolver_ResolvesAndClearsSchedulerConfigRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	const (
		namespace     = "test-ns"
		configMapName = "scheduler-config"
	)
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Pool: &v1alpha2.InferencePoolSpec{
						Ref: &corev1.LocalObjectReference{Name: "external-pool"},
					},
					Config: &v1alpha2.SchedulerConfigSpec{
						Ref: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
						},
					},
				},
			},
		},
	}
	template := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: configTemplateName, Namespace: constants.KServeNamespace},
	}
	schedulerConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: configMapName, Namespace: namespace},
		Data: map[string]string{"epp": `
apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins: []
`},
	}
	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(template, schedulerConfig).
			Build(),
	}

	combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})

	require.NoError(t, err)
	require.NotNil(t, combined.SchedulerConfigMap)
	assert.Equal(t, types.NamespacedName{Namespace: namespace, Name: configMapName}, *combined.SchedulerConfigMap)
	require.NotNil(t, combined.Spec.Router.Scheduler.Config.Inline)
	assert.Contains(t, string(combined.Spec.Router.Scheduler.Config.Inline.Raw), "EndpointPickerConfig")
	assert.Nil(t, combined.Spec.Router.Scheduler.Config.Ref)
}

func TestReconcileBaseRefs_MissingConfigDoesNotPanicOrPopulateAppliedRefs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{
				{Name: "missing-config"},
			},
		},
	}

	assert.NotPanics(t, func() {
		combined, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})
		assert.ErrorIs(t, err, reconcile.TerminalError(nil),
			"a missing config is fixed by creating it, not by requeuing")
		assert.Empty(t, combined, "a failed resolution returns the zero spec")
	})

	assert.Empty(t, llmSvc.Status.AppliedConfigRefs)
}

func TestReconcileBaseRefs_ClearsAppliedConfigRefsOnError(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{
				{Name: "missing-config"},
			},
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			AppliedConfigRefs: []v1alpha2.AppliedConfigRef{
				{Name: "stale-config", Namespace: "test-ns", Source: v1alpha2.AppliedConfigSourcePreset},
			},
		},
	}

	combined, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})
	assert.ErrorIs(t, err, reconcile.TerminalError(nil))
	assert.Empty(t, combined, "a failed resolution returns the zero spec")

	assert.Nil(t, llmSvc.Status.AppliedConfigRefs, "stale AppliedConfigRefs should be cleared on error")
}

func TestReconcileBaseRefs_PreservesAppliedConfigRefsWhenStopped(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	existingRefs := []v1alpha2.AppliedConfigRef{
		{Name: "prev-config", Namespace: "test-ns", Source: v1alpha2.AppliedConfigSourcePreset},
	}

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llm",
			Namespace: "test-ns",
			Annotations: map[string]string{
				constants.StopAnnotationKey: "true",
			},
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			BaseRefs: []corev1.LocalObjectReference{
				{Name: "missing-config"},
			},
		},
		Status: v1alpha2.LLMInferenceServiceStatus{
			AppliedConfigRefs: existingRefs,
		},
	}

	combined, err := reconciler.reconcileBaseRefs(t.Context(), llmSvc, &Config{})
	assert.NoError(t, err)
	assert.NotEmpty(t, combined)

	assert.Equal(t, existingRefs, llmSvc.Status.AppliedConfigRefs, "AppliedConfigRefs should be preserved when service is stopped")
}

func managedSchedulerService(namespace string) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model:  v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
			Router: &v1alpha2.RouterSpec{Scheduler: &v1alpha2.SchedulerSpec{}},
		},
	}
}

func TestSpecResolver_ConfigFlagInWellKnownPresetSuppressesInjection(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	const adminConfig = `
apiVersion: llm-d.ai/v1alpha1
kind: EndpointPickerConfig
plugins:
- type: admin-owned-plugin
`
	schedulerCfg := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: configRouterSchedulerName, Namespace: constants.KServeNamespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Annotations: map[string]string{"app.kubernetes.io/version": "0.11.0"},
					Template: &corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "main",
							Args: []string{"--config-text", adminConfig},
						}},
					},
				},
			},
		},
	}
	baselinePreset := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: configRouterSchedulerDefaultEPPConfigName, Namespace: constants.KServeNamespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Router: &v1alpha2.RouterSpec{
				Scheduler: &v1alpha2.SchedulerSpec{
					Config: &v1alpha2.SchedulerConfigSpec{
						Inline: &runtime.RawExtension{Raw: []byte(`{"plugins":[{"type":"token-load-scorer"}]}`)},
					},
				},
			},
		},
	}
	template := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: configTemplateName, Namespace: constants.KServeNamespace},
	}
	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(schedulerCfg, baselinePreset, template).
			Build(),
	}

	combined, err := reconciler.specResolver().Resolve(t.Context(), managedSchedulerService("test-ns"), &Config{})

	require.NoError(t, err)
	for _, ref := range combined.Applied {
		assert.NotEqual(t, configRouterSchedulerDefaultEPPConfigName, string(ref.Name),
			"preset must not be applied when the well-known scheduler config already supplies an EPPConfig")
	}
	assert.Nil(t, combined.Spec.Router.Scheduler.Config,
		"no EPPConfig should be injected on top of the admin-supplied one")
	assert.Equal(t, []string{"--config-text", adminConfig},
		combined.Spec.Router.Scheduler.Template.Containers[0].Args,
		"the admin-supplied config flag must survive untouched")
}

// TestSpecResolver_RendersAgainstBaseRefValues covers a topology config
// that carries the whole multi-node setup - worker plus parallelism - and is
// pulled in through baseRefs, leaving .spec on the LLMInferenceService itself
// empty. The data-parallel preset is selected from the merged spec, so it has to
// be rendered against the merged spec too.
func TestSpecResolver_RendersAgainstBaseRefValues(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	const namespace = "test-ns"

	// given: the whole multi-node topology lives in a baseRef, not on the service
	topology := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-node-data-parallel", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Worker: &corev1.PodSpec{},
				Parallelism: &v1alpha2.ParallelismSpec{
					Data:   ptr.To[int32](2),
					Expert: true,
				},
			},
		},
	}
	preset := loadPresetConfig(t, "config-llm-worker-data-parallel.yaml")
	preset.Namespace = namespace

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model:    v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
			BaseRefs: []corev1.LocalObjectReference{{Name: topology.Name}},
		},
	}

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(topology, preset).
			Build(),
	}

	// when
	combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})

	// then
	require.NoError(t, err)
	require.NotNil(t, combined.Spec.Template)
	require.NotEmpty(t, combined.Spec.Template.Containers)

	cmd := strings.Join(combined.Spec.Template.Containers[0].Command, " ")
	assert.Contains(t, cmd, "--data-parallel-size 2",
		"parallelism supplied through a baseRef must reach the rendered command")
	assert.Contains(t, cmd, "--enable-expert-parallel")
}

// TestSpecResolver_GracePeriodFromBaseRef covers the quiet half of the bug: a
// value that does not abort rendering, it just never arrives. The grace period reaches
// the pod either way - it is merged like any other field - but shutdownTimeout derives
// the engine's own timeout from it, and read the service alone it saw nothing and fell
// back to its default. The pod then drained for five minutes while telling the engine to
// give up after forty seconds.
//
// This is also the only case that restarts a running workload on upgrade, so the numbers
// are spelled out rather than left to a contains-check.
func TestSpecResolver_GracePeriodFromBaseRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	const namespace = "test-ns"
	const gracePeriod = int64(300)

	// given: the pod template, and its grace period, arrive through a baseRef
	baseRef := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-workload", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					TerminationGracePeriodSeconds: ptr.To(gracePeriod),
					Containers:                    []corev1.Container{{Name: "main"}},
				},
			},
		},
	}
	preset := loadPresetConfig(t, "config-llm-template.yaml")
	preset.Namespace = namespace

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model:    v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
			BaseRefs: []corev1.LocalObjectReference{{Name: baseRef.Name}},
		},
	}

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(baseRef, preset).Build(),
	}

	// when
	combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})

	// then: the pod carries the grace period the baseRef asked for
	require.NoError(t, err)
	require.NotNil(t, combined.Spec.Template)
	require.NotNil(t, combined.Spec.Template.TerminationGracePeriodSeconds)
	assert.Equal(t, gracePeriod, *combined.Spec.Template.TerminationGracePeriodSeconds)

	// and: the engine is told to stop within it, not within the default
	require.NotEmpty(t, combined.Spec.Template.Containers)
	cmd := strings.Join(combined.Spec.Template.Containers[0].Command, " ")
	assert.Contains(t, cmd, "--shutdown-timeout 280",
		"300s grace period, less the 15s preStop and a 5s signal buffer")
	assert.NotContains(t, cmd, "--shutdown-timeout 40",
		"40 is what the default 60s grace period yields, and the pod is not using it")
}

// TestSpecResolver_DisaggregatedBaseRefValues is the prefill/decode counterpart:
// a disaggregated service composed entirely from baseRefs, which is how the e2e suite
// builds one. The prefill presets read their own parallelism block, so this exercises a
// second dereference path that the single-node case leaves untouched.
func TestSpecResolver_DisaggregatedBaseRefValues(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	const namespace = "test-ns"

	// given: prefill and decode topologies both arrive through baseRefs
	workload := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "workload-dp-ep", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Worker:      &corev1.PodSpec{},
				Parallelism: &v1alpha2.ParallelismSpec{Data: ptr.To[int32](4), Expert: true},
			},
			Prefill: &v1alpha2.WorkloadSpec{
				Worker:      &corev1.PodSpec{},
				Parallelism: &v1alpha2.ParallelismSpec{Data: ptr.To[int32](2), Expert: true},
			},
		},
	}
	decodePreset := loadPresetConfig(t, "config-llm-decode-worker-data-parallel.yaml")
	decodePreset.Namespace = namespace
	prefillPreset := loadPresetConfig(t, "config-llm-prefill-worker-data-parallel.yaml")
	prefillPreset.Namespace = namespace

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model:    v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
			BaseRefs: []corev1.LocalObjectReference{{Name: workload.Name}},
		},
	}

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(workload, decodePreset, prefillPreset).
			Build(),
	}

	// when
	combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})

	// then
	require.NoError(t, err)
	require.NotNil(t, combined.Spec.Template)
	require.NotEmpty(t, combined.Spec.Template.Containers)
	require.NotNil(t, combined.Spec.Prefill)
	require.NotNil(t, combined.Spec.Prefill.Template)
	require.NotEmpty(t, combined.Spec.Prefill.Template.Containers)

	decodeCmd := strings.Join(combined.Spec.Template.Containers[0].Command, " ")
	assert.Contains(t, decodeCmd, "--data-parallel-size 4")
	assert.Contains(t, decodeCmd, "--enable-expert-parallel")

	prefillCmd := strings.Join(combined.Spec.Prefill.Template.Containers[0].Command, " ")
	assert.Contains(t, prefillCmd, "--data-parallel-size 2")
	assert.Contains(t, prefillCmd, "--enable-expert-parallel")
}

func loadPresetConfig(t *testing.T, name string) *v1alpha2.LLMInferenceServiceConfig {
	t.Helper()

	path := filepath.Join(kservetesting.ProjectRoot(), "config", "llmisvcconfig", name)
	data, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err)

	cfg := &v1alpha2.LLMInferenceServiceConfig{}
	require.NoError(t, yaml.Unmarshal(data, cfg))

	return cfg
}

// TestSpecResolver_ServiceWinsOverBaseRef pins the rendered command to the spec
// that actually gets deployed. Both are merged from the same inputs, but only if the
// service is applied last in each: a baseRef overrides the service while resolving what
// is enabled, and rendering off that view would put a value into the engine command that
// the service itself overrode.
func TestSpecResolver_ServiceWinsOverBaseRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	const namespace = "test-ns"

	// given: the service and one of its baseRefs disagree on tensor parallelism
	baseRef := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-defaults", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Parallelism: &v1alpha2.ParallelismSpec{Tensor: ptr.To[int32](2)},
			},
		},
	}
	preset := loadPresetConfig(t, "config-llm-template.yaml")
	preset.Namespace = namespace

	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model:    v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
			BaseRefs: []corev1.LocalObjectReference{{Name: baseRef.Name}},
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Parallelism: &v1alpha2.ParallelismSpec{Tensor: ptr.To[int32](8)},
			},
		},
	}

	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(baseRef, preset).Build(),
	}

	// when
	combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})

	// then
	require.NoError(t, err)
	require.NotNil(t, combined.Spec.Parallelism)
	assert.Equal(t, int32(8), *combined.Spec.Parallelism.Tensor,
		"the service overrides its baseRefs")

	require.NotNil(t, combined.Spec.Template)
	require.NotEmpty(t, combined.Spec.Template.Containers)
	cmd := strings.Join(combined.Spec.Template.Containers[0].Command, " ")
	assert.Contains(t, cmd, "--tensor-parallel-size 8",
		"the rendered command must match the deployed spec, not the overridden baseRef")
	assert.NotContains(t, cmd, "--tensor-parallel-size 2")
}

// TestSpecResolver_RendersAgainstCanonicalPreRenderSpec verifies the
// interface invariant that templates observe the canonical merged spec immediately
// before rendering. That includes merge-keyed list ordering and values contributed
// by the preset being rendered; either can affect the resulting PodTemplate.
func TestSpecResolver_RendersAgainstCanonicalPreRenderSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	const namespace = "test-ns"
	baseRef := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-defaults", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{Containers: []corev1.Container{{
					Name: "main",
					Env:  []corev1.EnvVar{{Name: "FROM_BASEREF", Value: "2"}},
				}}},
			},
		},
	}
	preset := &v1alpha2.LLMInferenceServiceConfig{
		ObjectMeta: metav1.ObjectMeta{Name: configTemplateName, Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					TerminationGracePeriodSeconds: ptr.To[int64](120),
					Containers: []corev1.Container{{
						Name: "main",
						Command: []string{
							`{{ range .Spec.Template.Containers }}{{ range .Env }}{{ .Name }}={{ .Value }};{{ end }}{{ end }}`,
							`{{ shutdownTimeout .Spec.Template 15 }}`,
						},
					}},
				},
			},
		},
	}
	llmSvc := &v1alpha2.LLMInferenceService{
		ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: namespace},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model:    v1alpha2.LLMModelSpec{Name: ptr.To("test-model")},
			BaseRefs: []corev1.LocalObjectReference{{Name: baseRef.Name}},
			WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{Containers: []corev1.Container{{
					Name: "main",
					Env:  []corev1.EnvVar{{Name: "FROM_SERVICE", Value: "1"}},
				}}},
			},
		},
	}
	reconciler := &LLMISVCReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(baseRef, preset).Build(),
	}

	combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})
	require.NoError(t, err)
	require.NotNil(t, combined.Spec.Template)
	require.Len(t, combined.Spec.Template.Containers, 1)

	container := combined.Spec.Template.Containers[0]
	wantEnv := make([]string, 0, len(container.Env))
	for _, env := range container.Env {
		wantEnv = append(wantEnv, env.Name+"="+env.Value+";")
	}
	require.Len(t, container.Command, 2)
	assert.Equal(t, strings.Join(wantEnv, ""), container.Command[0],
		"a custom template must observe the deployed environment ordering")
	assert.Equal(t, "100", container.Command[1],
		"a template must observe termination grace contributed by its own preset")

	// No preset may read a field that still holds unrendered template text; that is a
	// property of the presets themselves, asserted in TestPresetRenderingInvariants.

	// Catches nondeterminism - map iteration order reaching the desired spec.
	again, err := reconciler.specResolver().Resolve(t.Context(), llmSvc.DeepCopy(), &Config{})
	require.NoError(t, err)
	assert.Equal(t, combined, again, "an unchanged input must produce a stable desired spec")
}

// TestSpecResolver_BareServiceRendersUnchanged pins the upgrade-safety half of
// changing what presets render against: a service that references nothing must render
// exactly what it always did, so upgrading the controller alone cannot restart it.
//
// Each shape selects a different preset, and the multi-node ones are what size
// LeaderWorkerSet groups. TestPresetRenderingInvariants covers why the grace period
// asserted here is the same for all of them.
func TestSpecResolver_BareServiceRendersUnchanged(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	const namespace = "test-ns"

	shapes := map[string]struct {
		presets []string
		spec    v1alpha2.LLMInferenceServiceSpec
	}{
		"single-node": {
			presets: []string{"config-llm-template.yaml"},
			spec:    v1alpha2.LLMInferenceServiceSpec{},
		},
		"multi-node data parallel": {
			presets: []string{"config-llm-worker-data-parallel.yaml"},
			spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
				Worker:      &corev1.PodSpec{},
				Parallelism: &v1alpha2.ParallelismSpec{Data: ptr.To[int32](2)},
			}},
		},
		"prefill/decode": {
			presets: []string{"config-llm-decode-template.yaml", "config-llm-prefill-template.yaml"},
			spec:    v1alpha2.LLMInferenceServiceSpec{Prefill: &v1alpha2.WorkloadSpec{}},
		},
	}

	for name, shape := range shapes {
		t.Run(name, func(t *testing.T) {
			objs := []client.Object{}
			for _, p := range shape.presets {
				preset := loadPresetConfig(t, p)
				preset.Namespace = namespace
				objs = append(objs, preset)
			}

			// given: nothing set beyond what selects the preset
			llmSvc := &v1alpha2.LLMInferenceService{
				ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: namespace},
				Spec:       *shape.spec.DeepCopy(),
			}
			llmSvc.Spec.Model = v1alpha2.LLMModelSpec{Name: ptr.To("test-model")}

			reconciler := &LLMISVCReconciler{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
			}

			// when
			combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})

			// then
			require.NoError(t, err)

			pods := map[string]*corev1.PodSpec{"template": combined.Spec.Template, "worker": combined.Spec.Worker}
			if p := combined.Spec.Prefill; p != nil {
				pods["prefill.template"] = p.Template
				pods["prefill.worker"] = p.Worker
			}
			asserted := 0
			for where, pod := range pods {
				if pod == nil || len(pod.Containers) == 0 {
					continue
				}
				cmd := strings.Join(pod.Containers[0].Command, " ")
				if !strings.Contains(cmd, "--shutdown-timeout") {
					continue
				}
				asserted++
				assert.Contains(t, cmd, "--shutdown-timeout 40", where+
					": a preset grace period differing from shutdownTimeout's own default would roll every workload")
				assert.NotContains(t, cmd, "--tensor-parallel-size", where+
					": no preset supplies parallelism, so none may appear for a service that sets none")
				assert.Contains(t, cmd, `KV_TRANSFER_ARGS=""`, where+
					": no preset supplies kvCacheOffloading, so none may appear for a service that sets none")
			}
			require.NotZero(t, asserted, "expected at least one rendered command to assert on")
		})
	}
}

// The headroom percentage is carried by the preset, not by this package, so that
// retuning it ships as a new preset rather than re-rendering every service that
// already has a tier configured.
func TestSpecResolver_SizesSharedMemoryFromPresetAnnotation(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	preset := func(name, namespace, percent string) *v1alpha2.LLMInferenceServiceConfig {
		cfg := &v1alpha2.LLMInferenceServiceConfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
				Template: &corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:         mainContainerName,
						VolumeMounts: []corev1.VolumeMount{{Name: "dshm", MountPath: sharedMemoryMountPath}},
					}},
					Volumes: []corev1.Volume{{
						Name: "dshm",
						VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
							Medium:    corev1.StorageMediumMemory,
							SizeLimit: ptr.To(resource.MustParse("1Gi")),
						}},
					}},
				},
			}},
		}
		if percent != "" {
			cfg.Annotations = map[string]string{shmPercentOfCPUAnnotation: percent}
		}
		return cfg
	}
	sizeLimit := func(t *testing.T, combined *EffectiveSpec) string {
		t.Helper()
		require.NotNil(t, combined.Spec.Template)
		require.Len(t, combined.Spec.Template.Volumes, 1)
		return combined.Spec.Template.Volumes[0].EmptyDir.SizeLimit.String()
	}
	svc := func(baseRefs ...corev1.LocalObjectReference) *v1alpha2.LLMInferenceService {
		return &v1alpha2.LLMInferenceService{
			ObjectMeta: metav1.ObjectMeta{Name: "test-llm", Namespace: "test-ns"},
			Spec: v1alpha2.LLMInferenceServiceSpec{
				BaseRefs: baseRefs,
				WorkloadSpec: v1alpha2.WorkloadSpec{
					KVCacheOffloading: &v1alpha2.KVCacheOffloadingSpec{CPU: resource.MustParse("10Gi")},
				},
			},
		}
	}

	t.Run("the preset's percentage is applied", func(t *testing.T) {
		template := preset(configTemplateName, constants.KServeNamespace, "120")
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template).Build()}

		combined, err := reconciler.specResolver().Resolve(t.Context(), svc(), &Config{})
		require.NoError(t, err)
		assert.Equal(t, "13Gi", sizeLimit(t, combined))
	})

	t.Run("a preset that does not ask is left as declared", func(t *testing.T) {
		template := preset(configTemplateName, constants.KServeNamespace, "")
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template).Build()}

		combined, err := reconciler.specResolver().Resolve(t.Context(), svc(), &Config{})
		require.NoError(t, err)
		assert.Equal(t, "1Gi", sizeLimit(t, combined))
	})

	// The annotation is reserved for the configs KServe ships. Reading it off a
	// user's config would make an internal detail into an API, so it is ignored
	// there and the shipped preset's value still decides.
	t.Run("a user config cannot override the percentage", func(t *testing.T) {
		template := preset(configTemplateName, constants.KServeNamespace, "120")
		custom := preset("kv-config", "test-ns", "500")
		custom.Spec.Template = nil
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template, custom).Build()}

		combined, err := reconciler.specResolver().Resolve(t.Context(), svc(corev1.LocalObjectReference{Name: custom.Name}), &Config{})
		require.NoError(t, err)
		assert.Equal(t, "13Gi", sizeLimit(t, combined))
	})

	// The controller cannot check its arithmetic against the model being served,
	// so a size the user declared is the escape hatch from it and is used as
	// written - including a smaller one than the sizing would have produced.
	t.Run("a user-declared sizeLimit replaces the computed one", func(t *testing.T) {
		template := preset(configTemplateName, constants.KServeNamespace, "120")
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template).Build()}

		llmSvc := svc()
		llmSvc.Spec.Template = &corev1.PodSpec{Volumes: []corev1.Volume{{
			Name: "dshm",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium:    corev1.StorageMediumMemory,
				SizeLimit: ptr.To(resource.MustParse("32Gi")),
			}},
		}}}

		combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})
		require.NoError(t, err)
		assert.Equal(t, "32Gi", sizeLimit(t, combined))
	})

	for _, source := range []string{"service", "baseRef"} {
		for _, size := range []string{"1Gi", "1024Mi"} {
			t.Run("preserves an explicit preset-sized limit from "+source+"/"+size, func(t *testing.T) {
				template := preset(configTemplateName, constants.KServeNamespace, "120")
				custom := &v1alpha2.LLMInferenceServiceConfig{
					ObjectMeta: metav1.ObjectMeta{Name: "volume-config", Namespace: "test-ns"},
					Spec: v1alpha2.LLMInferenceServiceSpec{WorkloadSpec: v1alpha2.WorkloadSpec{
						// A partial override inherits its mount and medium from the preset.
						Template: &corev1.PodSpec{Volumes: []corev1.Volume{{
							Name: "dshm",
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{
								SizeLimit: ptr.To(resource.MustParse(size)),
							}},
						}}},
					}},
				}
				llmSvc := svc()
				if source == "service" {
					llmSvc.Spec.Template = custom.Spec.Template
				} else {
					llmSvc.Spec.BaseRefs = []corev1.LocalObjectReference{{Name: custom.Name}}
				}
				reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template, custom).Build()}

				combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})
				require.NoError(t, err)
				assert.Equal(t, "1Gi", sizeLimit(t, combined))
			})
		}
	}

	for _, tc := range []struct {
		name, prefillPercent, decodePercent, wantPrefill, wantDecode string
	}{
		{"legacy decode", "120", "", "13Gi", "1Gi"},
		{"legacy prefill", "", "120", "1Gi", "13Gi"},
		{"different policies", "150", "120", "16Gi", "13Gi"},
		{"invalid decode policy", "120", "invalid", "13Gi", "1Gi"},
	} {
		t.Run("keeps sizing with its workload/"+tc.name, func(t *testing.T) {
			prefill := preset(configPrefillWorkerDataParallelName, constants.KServeNamespace, tc.prefillPercent)
			prefill.Spec.Worker = prefill.Spec.Template.DeepCopy()
			prefill.Spec.Prefill = prefill.Spec.WorkloadSpec.DeepCopy()
			prefill.Spec.WorkloadSpec = v1alpha2.WorkloadSpec{}
			decode := preset(configDecodeWorkerDataParallelName, constants.KServeNamespace, tc.decodePercent)
			decode.Spec.Worker = decode.Spec.Template.DeepCopy()
			reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(prefill, decode).Build()}
			llmSvc := svc()
			llmSvc.Spec.Worker = &corev1.PodSpec{}
			llmSvc.Spec.Parallelism = &v1alpha2.ParallelismSpec{Data: ptr.To(int32(2))}
			llmSvc.Spec.Prefill = llmSvc.Spec.WorkloadSpec.DeepCopy()

			combined, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})
			require.NoError(t, err)
			again, err := reconciler.specResolver().Resolve(t.Context(), llmSvc, &Config{})
			require.NoError(t, err)
			assert.Equal(t, combined.Spec, again.Spec, "sizing must be stable across reconciles")

			assert.Equal(t, tc.wantDecode, sharedMemorySizeLimit(combined.Spec.Template).String())
			assert.Equal(t, tc.wantDecode, sharedMemorySizeLimit(combined.Spec.Worker).String())
			assert.Equal(t, tc.wantPrefill, sharedMemorySizeLimit(combined.Spec.Prefill.Template).String())
			assert.Equal(t, tc.wantPrefill, sharedMemorySizeLimit(combined.Spec.Prefill.Worker).String())
		})
	}

	// A well-known name resolves from the service's own namespace first, so a copy
	// a user puts there wins the slot. It must not be trusted with a reserved
	// annotation just for answering to the right name.
	t.Run("a config shadowing a well-known name is not a preset", func(t *testing.T) {
		shipped := preset(configTemplateName, constants.KServeNamespace, "120")
		shadow := preset(configTemplateName, "test-ns", "500")
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(shipped, shadow).Build()}

		combined, err := reconciler.specResolver().Resolve(t.Context(), svc(), &Config{})
		require.NoError(t, err)
		assert.Equal(t, "1Gi", sizeLimit(t, combined))

		require.Len(t, combined.Applied, 1)
		assert.Equal(t, v1alpha2.AppliedConfigSourceUserRef, combined.Applied[0].Source)
	})

	// A preset is not worth failing a reconcile over: an unreadable value leaves
	// the declared size alone rather than taking the service down.
	t.Run("an unreadable percentage leaves the declared size alone", func(t *testing.T) {
		template := preset(configTemplateName, constants.KServeNamespace, "not-a-number")
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template).Build()}

		combined, err := reconciler.specResolver().Resolve(t.Context(), svc(), &Config{})
		require.NoError(t, err)
		assert.Equal(t, "1Gi", sizeLimit(t, combined))
	})
}

// controller-runtime's terminal wrapping says whether the controller will
// requeue; the user reading the condition can only act on the cause.
func TestConditionMessageDropsTerminalPrefix(t *testing.T) {
	cause := errors.New("env KSERVE_KV_TRANSFER_ARGS cannot take a valueFrom")

	assert.Equal(t, cause.Error(), conditionMessage(reconcile.TerminalError(cause)))
	assert.Equal(t, cause.Error(), conditionMessage(cause))
	assert.Equal(t, "wrapped: "+cause.Error(),
		conditionMessage(fmt.Errorf("wrapped: %w", cause)))

	// errors.Is matches the terminal marker anywhere in the chain, so the layer
	// has to be located before it is unwrapped - peeling only the outermost would
	// leave the prefix on a wrapped terminal error.
	// The marker can sit at any depth. Removing it must not take the surrounding
	// context with it - "outer:" is what tells the user which step failed.
	assert.Equal(t, "outer: "+cause.Error(),
		conditionMessage(fmt.Errorf("outer: %w", reconcile.TerminalError(cause))))
}
