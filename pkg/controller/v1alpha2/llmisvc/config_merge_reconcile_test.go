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

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
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
	assert.Nil(t, combined)
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

// TestCombineBaseRefsConfig_TransferSlot pins the upgrade contract end to end: a
// preset declaring the slot gets the argument filled in from the merged spec, and
// one without it comes back untouched.
func TestCombineBaseRefsConfig_TransferSlot(t *testing.T) {
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

			combined, err := reconciler.combineBaseRefsConfig(t.Context(), llmSvc, &Config{})
			require.NoError(t, err)
			require.NotNil(t, combined)
			require.NotNil(t, combined.Config.Spec.Template)
			podSpec := combined.Config.Spec.Template
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
func TestCombineBaseRefsConfig_RendersDegenerateKVCacheCPUFromBaseRef(t *testing.T) {
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

			combined, err := reconciler.combineBaseRefsConfig(t.Context(), llmSvc, &Config{})
			require.NoError(t, err)

			require.NotNil(t, combined.Config.Spec.Template)
			require.Len(t, combined.Config.Spec.Template.Containers, 1)
			transfer, filled := utils.GetEnvVarValue(combined.Config.Spec.Template.Containers[0].Env, kvTransferArgsEnvVar)
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
	assert.Nil(t, combined)
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

func TestCombineBaseRefsConfig_ResolvesAndClearsSchedulerConfigRef(t *testing.T) {
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

	combined, err := reconciler.combineBaseRefsConfig(t.Context(), llmSvc, &Config{})

	require.NoError(t, err)
	require.NotNil(t, combined.ResolvedSchedulerConfigMap)
	assert.Equal(t, types.NamespacedName{Namespace: namespace, Name: configMapName}, *combined.ResolvedSchedulerConfigMap)
	require.NotNil(t, combined.Config.Spec.Router.Scheduler.Config.Inline)
	assert.Contains(t, string(combined.Config.Spec.Router.Scheduler.Config.Inline.Raw), "EndpointPickerConfig")
	assert.Nil(t, combined.Config.Spec.Router.Scheduler.Config.Ref)
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
		assert.Nil(t, combined)
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
	assert.Nil(t, combined)

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
	assert.NotNil(t, combined)

	assert.Equal(t, existingRefs, llmSvc.Status.AppliedConfigRefs, "AppliedConfigRefs should be preserved when service is stopped")
}

// The headroom percentage is carried by the preset, not by this package, so that
// retuning it ships as a new preset rather than re-rendering every service that
// already has a tier configured.
func TestCombineBaseRefsConfig_SizesSharedMemoryFromPresetAnnotation(t *testing.T) {
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
			cfg.Annotations = map[string]string{shmHeadroomPercentAnnotation: percent}
		}
		return cfg
	}
	sizeLimit := func(t *testing.T, combined *CombinedConfig) string {
		t.Helper()
		require.NotNil(t, combined.Config.Spec.Template)
		require.Len(t, combined.Config.Spec.Template.Volumes, 1)
		return combined.Config.Spec.Template.Volumes[0].EmptyDir.SizeLimit.String()
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

		combined, err := reconciler.combineBaseRefsConfig(t.Context(), svc(), &Config{})
		require.NoError(t, err)
		assert.Equal(t, "13Gi", sizeLimit(t, combined))
	})

	t.Run("a preset that does not ask is left as declared", func(t *testing.T) {
		template := preset(configTemplateName, constants.KServeNamespace, "")
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template).Build()}

		combined, err := reconciler.combineBaseRefsConfig(t.Context(), svc(), &Config{})
		require.NoError(t, err)
		assert.Equal(t, "1Gi", sizeLimit(t, combined))
	})

	// The annotation is reserved for the configs KServe ships. Reading it off a
	// user's config would make an internal detail into an API, so it is ignored
	// there and the shipped preset's value still decides.
	t.Run("a user config cannot override the percentage", func(t *testing.T) {
		template := preset(configTemplateName, constants.KServeNamespace, "120")
		custom := preset("kv-config", "test-ns", "500")
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template, custom).Build()}

		combined, err := reconciler.combineBaseRefsConfig(t.Context(), svc(corev1.LocalObjectReference{Name: custom.Name}), &Config{})
		require.NoError(t, err)
		assert.Equal(t, "13Gi", sizeLimit(t, combined))
	})

	// Users raise the ceiling by declaring one; the tier is added on top of it.
	t.Run("a user-declared sizeLimit is added to, not replaced", func(t *testing.T) {
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

		combined, err := reconciler.combineBaseRefsConfig(t.Context(), llmSvc, &Config{})
		require.NoError(t, err)
		assert.Equal(t, "44Gi", sizeLimit(t, combined))
	})

	// A well-known name resolves from the service's own namespace first, so a copy
	// a user puts there wins the slot. It must not be trusted with a reserved
	// annotation just for answering to the right name.
	t.Run("a config shadowing a well-known name is not a preset", func(t *testing.T) {
		shipped := preset(configTemplateName, constants.KServeNamespace, "120")
		shadow := preset(configTemplateName, "test-ns", "500")
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(shipped, shadow).Build()}

		combined, err := reconciler.combineBaseRefsConfig(t.Context(), svc(), &Config{})
		require.NoError(t, err)
		assert.Equal(t, "1Gi", sizeLimit(t, combined))

		require.Len(t, combined.AppliedConfigRefs, 1)
		assert.Equal(t, v1alpha2.AppliedConfigSourceUserRef, combined.AppliedConfigRefs[0].Source)
	})

	// A preset is not worth failing a reconcile over: an unreadable value leaves
	// the declared size alone rather than taking the service down.
	t.Run("an unreadable percentage leaves the declared size alone", func(t *testing.T) {
		template := preset(configTemplateName, constants.KServeNamespace, "not-a-number")
		reconciler := &LLMISVCReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(template).Build()}

		combined, err := reconciler.combineBaseRefsConfig(t.Context(), svc(), &Config{})
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
}
