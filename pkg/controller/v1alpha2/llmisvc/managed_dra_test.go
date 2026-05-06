package llmisvc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
)

func newManagedDRATestLLMISVC(name string, annotations map[string]string) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "serving.kserve.io/v1alpha2",
			Kind:       "LLMInferenceService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			UID:         "test-uid-dra",
			Annotations: annotations,
		},
	}
}

func TestHasManagedDRA(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
			expected:    false,
		},
		{
			name:        "unrelated annotation",
			annotations: map[string]string{"foo": "bar"},
			expected:    false,
		},
		{
			name:        "device class set",
			annotations: map[string]string{constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com"},
			expected:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			llmSvc := newManagedDRATestLLMISVC("test", tt.annotations)
			assert.Equal(t, tt.expected, hasManagedDRA(llmSvc))
		})
	}
}

func TestParseManagedDRAGpuCount(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		expected  int
		expectErr bool
	}{
		{name: "not set", value: "", expected: 1},
		{name: "one", value: "1", expected: 1},
		{name: "two", value: "2", expected: 2},
		{name: "five", value: "5", expected: 5},
		{name: "zero", value: "0", expectErr: true},
		{name: "negative", value: "-1", expectErr: true},
		{name: "non-numeric", value: "abc", expectErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotations := map[string]string{
				constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
			}
			if tt.value != "" {
				annotations[constants.ManagedDRAGpuCountAnnotationKey] = tt.value
			}
			llmSvc := newManagedDRATestLLMISVC("test", annotations)
			count, err := parseManagedDRAGpuCount(llmSvc)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, count)
			}
		})
	}
}

// TestYAMLBlockScalarRoundTripsToSelectors verifies that the documented YAML `|`
// block scalar correctly parses into newline-separated CEL selectors.
func TestYAMLBlockScalarRoundTripsToSelectors(t *testing.T) {
	manifest := `
apiVersion: serving.kserve.io/v1alpha2
kind: LLMInferenceService
metadata:
  name: my-llm
  namespace: default
  annotations:
    serving.kserve.io/exp-dra-device-class: gpu.example.com
    serving.kserve.io/exp-dra-gpu-count: "1"
    serving.kserve.io/exp-dra-cel-selector: |
      device.attributes['gpu.example.com'].model == 'LATEST-GPU-MODEL'
      device.capacity['gpu.example.com'].memory.compareTo(quantity('4Gi')) >= 0
`

	llmSvc := &v1alpha2.LLMInferenceService{}
	require.NoError(t, yaml.Unmarshal([]byte(manifest), llmSvc))

	// Verify basic annotations are parsed as strings.
	assert.Equal(t, "1", llmSvc.Annotations[constants.ManagedDRAGpuCountAnnotationKey])
	assert.Equal(t, "gpu.example.com", llmSvc.Annotations[constants.ManagedDRADeviceClassAnnotationKey])

	// Verify the block scalar preserves newlines.
	raw := llmSvc.Annotations[constants.ManagedDRACelSelectorAnnotationKey]
	assert.Contains(t, raw, "\n")

	selectors := parseManagedDRACelSelectors(llmSvc)
	require.Len(t, selectors, 2)
	assert.Equal(t, "device.attributes['gpu.example.com'].model == 'LATEST-GPU-MODEL'", selectors[0])
	assert.Equal(t, "device.capacity['gpu.example.com'].memory.compareTo(quantity('4Gi')) >= 0", selectors[1])

	// Verify the device request builder applies all selectors.
	count, err := parseManagedDRAGpuCount(llmSvc)
	require.NoError(t, err)
	requests := buildDeviceRequests(
		llmSvc.Annotations[constants.ManagedDRADeviceClassAnnotationKey],
		selectors,
		count,
	)
	require.Len(t, requests, 1)
	require.Len(t, requests[0].Exactly.Selectors, 2)
}

func TestParseManagedDRACelSelectors(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected []string
	}{
		{name: "not set", value: "", expected: nil},
		{name: "whitespace only", value: "   \n  \n", expected: nil},
		{
			name:     "single expression",
			value:    "device.attributes['gpu.nvidia.com']['type'] == 'A100'",
			expected: []string{"device.attributes['gpu.nvidia.com']['type'] == 'A100'"},
		},
		{
			name: "multiple expressions newline-separated",
			value: "device.attributes['gpu.nvidia.com']['type'] == 'A100'\n" +
				"device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0",
			expected: []string{
				"device.attributes['gpu.nvidia.com']['type'] == 'A100'",
				"device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0",
			},
		},
		{
			name: "multiple expressions with blank lines and indentation",
			value: "\n  device.attributes['gpu.nvidia.com']['type'] == 'A100'  \n" +
				"\n  device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0\n",
			expected: []string{
				"device.attributes['gpu.nvidia.com']['type'] == 'A100'",
				"device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotations := map[string]string{
				constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
			}
			if tt.value != "" {
				annotations[constants.ManagedDRACelSelectorAnnotationKey] = tt.value
			}
			llmSvc := newManagedDRATestLLMISVC("test", annotations)
			assert.Equal(t, tt.expected, parseManagedDRACelSelectors(llmSvc))
		})
	}
}

func TestBuildDeviceRequests(t *testing.T) {
	t.Run("single GPU without CEL", func(t *testing.T) {
		reqs := buildDeviceRequests("gpu.nvidia.com", nil, 1)
		require.Len(t, reqs, 1)
		assert.Equal(t, "gpu", reqs[0].Name)
		assert.Equal(t, "gpu.nvidia.com", reqs[0].Exactly.DeviceClassName)
		assert.Empty(t, reqs[0].Exactly.Selectors)
	})

	t.Run("single GPU with single CEL", func(t *testing.T) {
		cel := "device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0"
		reqs := buildDeviceRequests("gpu.nvidia.com", []string{cel}, 1)
		require.Len(t, reqs, 1)
		assert.Equal(t, "gpu", reqs[0].Name)
		require.Len(t, reqs[0].Exactly.Selectors, 1)
		assert.Equal(t, cel, reqs[0].Exactly.Selectors[0].CEL.Expression)
	})

	t.Run("single GPU with multiple CEL selectors", func(t *testing.T) {
		cels := []string{
			"device.attributes['gpu.nvidia.com']['type'] == 'A100'",
			"device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0",
		}
		reqs := buildDeviceRequests("gpu.nvidia.com", cels, 1)
		require.Len(t, reqs, 1)
		require.Len(t, reqs[0].Exactly.Selectors, 2)
		assert.Equal(t, cels[0], reqs[0].Exactly.Selectors[0].CEL.Expression)
		assert.Equal(t, cels[1], reqs[0].Exactly.Selectors[1].CEL.Expression)
	})

	t.Run("multiple GPUs", func(t *testing.T) {
		reqs := buildDeviceRequests("gpu.nvidia.com", nil, 3)
		require.Len(t, reqs, 1)
		assert.Equal(t, "gpu", reqs[0].Name)
		assert.Equal(t, "gpu.nvidia.com", reqs[0].Exactly.DeviceClassName)
		assert.Equal(t, int64(3), reqs[0].Exactly.Count)
	})

	t.Run("multiple GPUs with multiple CEL selectors", func(t *testing.T) {
		cels := []string{
			"device.attributes['gpu.nvidia.com']['type'] == 'mig'",
			"device.attributes['gpu.nvidia.com']['profile'] == '3g.40gb'",
		}
		reqs := buildDeviceRequests("mig-3g.40gb", cels, 2)
		require.Len(t, reqs, 1)
		assert.Equal(t, "gpu", reqs[0].Name)
		assert.Equal(t, "mig-3g.40gb", reqs[0].Exactly.DeviceClassName)
		assert.Equal(t, int64(2), reqs[0].Exactly.Count)
		require.Len(t, reqs[0].Exactly.Selectors, 2)
		assert.Equal(t, cels[0], reqs[0].Exactly.Selectors[0].CEL.Expression)
		assert.Equal(t, cels[1], reqs[0].Exactly.Selectors[1].CEL.Expression)
	})
}

func TestExpectedManagedDRATemplate(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("my-model", map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
	})
	reqs := buildDeviceRequests("gpu.nvidia.com", nil, 1)
	tmpl := expectedManagedDRATemplate(llmSvc, reqs)

	assert.Equal(t, "my-model-managed-dra", tmpl.Name)
	assert.Equal(t, "default", tmpl.Namespace)
	require.Len(t, tmpl.Spec.Spec.Devices.Requests, 1)
	assert.Equal(t, "gpu", tmpl.Spec.Spec.Devices.Requests[0].Name)
	require.Len(t, tmpl.OwnerReferences, 1)
	assert.True(t, *tmpl.OwnerReferences[0].Controller)
}

func TestInjectManagedDRA_NoAnnotation(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("test", nil)
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}
	injectManagedDRA(llmSvc, podSpec)

	assert.Empty(t, podSpec.ResourceClaims)
	assert.Empty(t, podSpec.Containers[0].Resources.Claims)
}

// Single container named "main" should get the claim.
func TestInjectManagedDRA_MainContainerOnly(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("test", map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.example.com",
	})
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}
	injectManagedDRA(llmSvc, podSpec)

	require.Len(t, podSpec.ResourceClaims, 1)
	assert.Equal(t, managedDRAClaimName, podSpec.ResourceClaims[0].Name)
	assert.NotNil(t, podSpec.ResourceClaims[0].ResourceClaimTemplateName)
	assert.Equal(t, "test-managed-dra", *podSpec.ResourceClaims[0].ResourceClaimTemplateName)
	assert.Nil(t, podSpec.ResourceClaims[0].ResourceClaimName)

	require.Len(t, podSpec.Containers[0].Resources.Claims, 1)
	assert.Equal(t, managedDRAClaimName, podSpec.Containers[0].Resources.Claims[0].Name)
}

// Sidecar containers must NOT receive the GPU claim — only the "main" container does.
func TestInjectManagedDRA_OnlyMainContainerInjected(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("test", map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.example.com",
	})
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "main"},
			{Name: "sidecar"},
			{Name: "monitor"},
		},
	}
	injectManagedDRA(llmSvc, podSpec)

	require.Len(t, podSpec.ResourceClaims, 1)

	mainClaims := podSpec.Containers[0].Resources.Claims
	require.Len(t, mainClaims, 1, "main container should have the GPU claim")
	assert.Equal(t, managedDRAClaimName, mainClaims[0].Name)

	for _, ctr := range podSpec.Containers[1:] {
		assert.Empty(t, ctr.Resources.Claims, "sidecar %q must not receive the GPU claim", ctr.Name)
	}
}

// If there is no container called "main" the pod-level claim is still added,
// but no container claim is injected.
func TestInjectManagedDRA_NoMainContainer(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("test", map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.example.com",
	})
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{Name: "sidecar"},
			{Name: "monitor"},
		},
	}
	injectManagedDRA(llmSvc, podSpec)

	require.Len(t, podSpec.ResourceClaims, 1)
	for _, ctr := range podSpec.Containers {
		assert.Empty(t, ctr.Resources.Claims, "container %q must not receive the GPU claim", ctr.Name)
	}
}

// calling inject twice should not duplicate entries on the main container.
func TestInjectManagedDRA_Idempotent(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("test", map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.example.com",
	})
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}, {Name: "sidecar"}},
	}
	injectManagedDRA(llmSvc, podSpec)
	injectManagedDRA(llmSvc, podSpec)

	assert.Len(t, podSpec.ResourceClaims, 1)
	assert.Len(t, podSpec.Containers[0].Resources.Claims, 1)
	assert.Empty(t, podSpec.Containers[1].Resources.Claims)
}

// Pre-existing user claims on the main container should not be clobbered.
func TestInjectManagedDRA_PreservesExistingClaims(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("test", map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.example.com",
	})
	podSpec := &corev1.PodSpec{
		ResourceClaims: []corev1.PodResourceClaim{
			{Name: "user-claim"},
		},
		Containers: []corev1.Container{
			{
				Name: "main",
				Resources: corev1.ResourceRequirements{
					Claims: []corev1.ResourceClaim{{Name: "user-claim"}},
				},
			},
		},
	}
	injectManagedDRA(llmSvc, podSpec)

	assert.Len(t, podSpec.ResourceClaims, 2)
	assert.Len(t, podSpec.Containers[0].Resources.Claims, 2)
}

func TestSemanticResourceClaimTemplateIsEqual(t *testing.T) {
	makeTemplate := func(className string) *resourcev1.ResourceClaimTemplate {
		return &resourcev1.ResourceClaimTemplate{
			Spec: resourcev1.ResourceClaimTemplateSpec{
				Spec: resourcev1.ResourceClaimSpec{
					Devices: resourcev1.DeviceClaim{
						Requests: []resourcev1.DeviceRequest{
							{
								Name:    "gpu",
								Exactly: &resourcev1.ExactDeviceRequest{DeviceClassName: className},
							},
						},
					},
				},
			},
		}
	}

	assert.True(t, semanticResourceClaimTemplateIsEqual(makeTemplate("gpu.nvidia.com"), makeTemplate("gpu.nvidia.com")))
	assert.False(t, semanticResourceClaimTemplateIsEqual(makeTemplate("gpu.nvidia.com"), makeTemplate("gpu.amd.com")))
}

func TestReconcileManagedDRA(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, resourcev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	// Mock the CRD availability to avoid panics when r.Config is nil in tests
	utils.SetAvailableResourcesForApi(resourcev1.SchemeGroupVersion.String(), &metav1.APIResourceList{
		APIResources: []metav1.APIResource{
			{Kind: "ResourceClaimTemplate"},
		},
	})

	t.Run("no managed DRA, nothing to clean up", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", nil)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		err := r.reconcileManagedDRA(ctx, llmSvc)
		require.NoError(t, err)
	})

	t.Run("invalid gpu count", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", map[string]string{
			constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
			constants.ManagedDRAGpuCountAnnotationKey:    "invalid",
		})
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		err := r.reconcileManagedDRA(ctx, llmSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid syntax")
	})

	t.Run("template claim is created", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", map[string]string{
			constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
		})
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		err := r.reconcileManagedDRA(ctx, llmSvc)
		require.NoError(t, err)

		tmpl := &resourcev1.ResourceClaimTemplate{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-svc-managed-dra", Namespace: "default"}, tmpl)
		require.NoError(t, err)
		assert.Equal(t, "gpu", tmpl.Spec.Spec.Devices.Requests[0].Name)
	})

	t.Run("template claim with multiple CEL selectors", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc-cel", map[string]string{
			constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
			constants.ManagedDRACelSelectorAnnotationKey: "device.attributes['gpu.nvidia.com']['type'] == 'A100'\n" +
				"device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0",
		})
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		require.NoError(t, r.reconcileManagedDRA(ctx, llmSvc))

		tmpl := &resourcev1.ResourceClaimTemplate{}
		require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{Name: "test-svc-cel-managed-dra", Namespace: "default"}, tmpl))
		require.Len(t, tmpl.Spec.Spec.Devices.Requests, 1)
		require.Len(t, tmpl.Spec.Spec.Devices.Requests[0].Exactly.Selectors, 2)
	})

	t.Run("cleanup deletes orphaned template after DRA disabled", func(t *testing.T) {
		objNamespacedName := types.NamespacedName{Name: "test-svc-managed-dra", Namespace: "default"}

		llmSvcWithDRA := newManagedDRATestLLMISVC("test-svc", map[string]string{
			constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
		})

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		require.NoError(t, r.reconcileManagedDRA(ctx, llmSvcWithDRA))
		require.NoError(t, fakeClient.Get(ctx, objNamespacedName, &resourcev1.ResourceClaimTemplate{}),
			"template should exist after first reconcile")

		llmSvcWithoutDRA := newManagedDRATestLLMISVC("test-svc", nil)

		require.NoError(t, r.reconcileManagedDRA(ctx, llmSvcWithoutDRA))

		err := fakeClient.Get(ctx, objNamespacedName, &resourcev1.ResourceClaimTemplate{})
		require.Error(t, err, "orphaned template should be cleaned up after DRA is disabled")
		assert.True(t, apierrors.IsNotFound(err), "expected NotFound, got %v", err)
	})

	t.Run("cleanup is a no-op when there is no orphaned template", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("never-had-dra", nil)
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		require.NoError(t, r.reconcileManagedDRA(ctx, llmSvc))
	})
}

func TestReconcileManagedDRA_CreateError(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, resourcev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	// Mock the CRD availability to avoid panics when r.Config is nil in tests
	utils.SetAvailableResourcesForApi(resourcev1.SchemeGroupVersion.String(), &metav1.APIResourceList{
		APIResources: []metav1.APIResource{
			{Kind: "ResourceClaimTemplate"},
		},
	})

	t.Run("template claim error", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", map[string]string{
			constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
		})

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					return assert.AnError
				},
			}).
			Build()

		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		err := r.reconcileManagedDRA(ctx, llmSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to reconcile Managed DRA ResourceClaimTemplate")
	})

	t.Run("cleanup error", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", nil)

		// Create the existing object so Delete doesn't return nil early
		existingTemplate := &resourcev1.ResourceClaimTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      managedDRAResourceName(llmSvc),
				Namespace: llmSvc.GetNamespace(),
			},
		}
		require.NoError(t, controllerutil.SetControllerReference(llmSvc, existingTemplate, scheme))

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(existingTemplate).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					return errors.New("mock delete error")
				},
			}).
			Build()

		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		err := r.reconcileManagedDRA(ctx, llmSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to cleanup Managed DRA ResourceClaimTemplate")
	})

	t.Run("crd not available error", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", map[string]string{
			constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
		})

		// Temporarily clear the mock cache to simulate CRD missing
		utils.SetAvailableResourcesForApi(resourcev1.SchemeGroupVersion.String(), &metav1.APIResourceList{})

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		err := r.reconcileManagedDRA(ctx, llmSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "managed DRA is requested but the ResourceClaimTemplate API")

		// Restore the mock for other tests
		utils.SetAvailableResourcesForApi(resourcev1.SchemeGroupVersion.String(), &metav1.APIResourceList{
			APIResources: []metav1.APIResource{
				{Kind: "ResourceClaimTemplate"},
			},
		})
	})

	t.Run("cleanup crd not available", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", nil) // no DRA annotations -> triggers cleanup

		// Temporarily clear the mock cache to simulate CRD missing
		utils.SetAvailableResourcesForApi(resourcev1.SchemeGroupVersion.String(), &metav1.APIResourceList{})

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		err := r.reconcileManagedDRA(ctx, llmSvc)
		require.NoError(t, err)

		// Restore the mock for other tests
		utils.SetAvailableResourcesForApi(resourcev1.SchemeGroupVersion.String(), &metav1.APIResourceList{
			APIResources: []metav1.APIResource{
				{Kind: "ResourceClaimTemplate"},
			},
		})
	})
}
