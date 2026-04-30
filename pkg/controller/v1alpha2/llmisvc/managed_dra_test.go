package llmisvc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
	"github.com/kserve/kserve/pkg/constants"
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

func TestBuildDeviceRequests(t *testing.T) {
	t.Run("single GPU without CEL", func(t *testing.T) {
		reqs := buildDeviceRequests("gpu.nvidia.com", "", 1)
		require.Len(t, reqs, 1)
		assert.Equal(t, "gpu", reqs[0].Name)
		assert.Equal(t, "gpu.nvidia.com", reqs[0].Exactly.DeviceClassName)
		assert.Empty(t, reqs[0].Exactly.Selectors)
	})

	t.Run("single GPU with CEL", func(t *testing.T) {
		cel := "device.capacity['gpu.nvidia.com']['memory'].compareTo(quantity('40Gi')) > 0"
		reqs := buildDeviceRequests("gpu.nvidia.com", cel, 1)
		require.Len(t, reqs, 1)
		assert.Equal(t, "gpu", reqs[0].Name)
		require.Len(t, reqs[0].Exactly.Selectors, 1)
		assert.Equal(t, cel, reqs[0].Exactly.Selectors[0].CEL.Expression)
	})

	t.Run("multiple GPUs", func(t *testing.T) {
		reqs := buildDeviceRequests("gpu.nvidia.com", "", 3)
		require.Len(t, reqs, 3)
		assert.Equal(t, "gpu-1", reqs[0].Name)
		assert.Equal(t, "gpu-2", reqs[1].Name)
		assert.Equal(t, "gpu-3", reqs[2].Name)
		for _, req := range reqs {
			assert.Equal(t, "gpu.nvidia.com", req.Exactly.DeviceClassName)
		}
	})

	t.Run("multiple GPUs with CEL", func(t *testing.T) {
		cel := "device.attributes['gpu.nvidia.com']['type'] == 'mig'"
		reqs := buildDeviceRequests("mig-3g.40gb", cel, 2)
		require.Len(t, reqs, 2)
		for _, req := range reqs {
			assert.Equal(t, "mig-3g.40gb", req.Exactly.DeviceClassName)
			require.Len(t, req.Exactly.Selectors, 1)
			assert.Equal(t, cel, req.Exactly.Selectors[0].CEL.Expression)
		}
	})
}

func TestExpectedManagedDRATemplate(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("my-model", map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
	})
	reqs := buildDeviceRequests("gpu.nvidia.com", "", 1)
	tmpl := expectedManagedDRATemplate(llmSvc, reqs)

	assert.Equal(t, "my-model-managed-dra", tmpl.Name)
	assert.Equal(t, "default", tmpl.Namespace)
	require.Len(t, tmpl.Spec.Spec.Devices.Requests, 1)
	assert.Equal(t, "gpu", tmpl.Spec.Spec.Devices.Requests[0].Name)
	require.Len(t, tmpl.OwnerReferences, 1)
	assert.True(t, *tmpl.OwnerReferences[0].Controller)
}

func TestExpectedManagedDRAClaim(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("shared-model", map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
		constants.ManagedDRASharingAnnotationKey:     "true",
	})
	reqs := buildDeviceRequests("gpu.nvidia.com", "", 2)
	claim := expectedManagedDRAClaim(llmSvc, reqs)

	assert.Equal(t, "shared-model-managed-dra", claim.Name)
	require.Len(t, claim.Spec.Devices.Requests, 2)
	assert.Equal(t, "gpu-1", claim.Spec.Devices.Requests[0].Name)
	assert.Equal(t, "gpu-2", claim.Spec.Devices.Requests[1].Name)
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

// Dedicated GPU per pod, single container
func TestInjectManagedDRA_DedicatedSingleContainer(t *testing.T) {
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

// Intra-pod sharing — all containers get the claim
func TestInjectManagedDRA_AllContainersInjected(t *testing.T) {
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

	for _, ctr := range podSpec.Containers {
		require.Len(t, ctr.Resources.Claims, 1, "container %s should have 1 claim", ctr.Name)
		assert.Equal(t, managedDRAClaimName, ctr.Resources.Claims[0].Name)
	}
}

// Shared ResourceClaim across replicas
func TestInjectManagedDRA_SharedClaim(t *testing.T) {
	llmSvc := newManagedDRATestLLMISVC("test", map[string]string{
		constants.ManagedDRADeviceClassAnnotationKey: "gpu.example.com",
		constants.ManagedDRASharingAnnotationKey:     "true",
	})
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{Name: "main"}},
	}
	injectManagedDRA(llmSvc, podSpec)

	require.Len(t, podSpec.ResourceClaims, 1)
	assert.NotNil(t, podSpec.ResourceClaims[0].ResourceClaimName)
	assert.Equal(t, "test-managed-dra", *podSpec.ResourceClaims[0].ResourceClaimName)
	assert.Nil(t, podSpec.ResourceClaims[0].ResourceClaimTemplateName)
}

// calling inject twice should not duplicate entries
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
	assert.Len(t, podSpec.Containers[1].Resources.Claims, 1)
}

// Pre-existing user claims should not be clobbered
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

	t.Run("no managed DRA", func(t *testing.T) {
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

	t.Run("shared claim", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", map[string]string{
			constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
			constants.ManagedDRASharingAnnotationKey:     "true",
		})
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		r := &LLMISVCReconciler{
			Client:        fakeClient,
			EventRecorder: record.NewFakeRecorder(10),
		}

		err := r.reconcileManagedDRA(ctx, llmSvc)
		require.NoError(t, err)

		// Verify claim was created
		claim := &resourcev1.ResourceClaim{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-svc-managed-dra", Namespace: "default"}, claim)
		require.NoError(t, err)
		assert.Equal(t, "gpu", claim.Spec.Devices.Requests[0].Name)
	})

	t.Run("template claim", func(t *testing.T) {
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

		// Verify template was created
		tmpl := &resourcev1.ResourceClaimTemplate{}
		err = fakeClient.Get(ctx, types.NamespacedName{Name: "test-svc-managed-dra", Namespace: "default"}, tmpl)
		require.NoError(t, err)
		assert.Equal(t, "gpu", tmpl.Spec.Spec.Devices.Requests[0].Name)
	})
}

func TestSemanticResourceClaimIsEqual(t *testing.T) {
	makeClaim := func(className string) *resourcev1.ResourceClaim {
		return &resourcev1.ResourceClaim{
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
		}
	}

	assert.True(t, semanticResourceClaimIsEqual(makeClaim("gpu.nvidia.com"), makeClaim("gpu.nvidia.com")))
	assert.False(t, semanticResourceClaimIsEqual(makeClaim("gpu.nvidia.com"), makeClaim("gpu.amd.com")))
}

func TestReconcileManagedDRA_CreateError(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, resourcev1.AddToScheme(scheme))
	require.NoError(t, v1alpha2.AddToScheme(scheme))

	// Test template claim creation error
	t.Run("template claim error", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", map[string]string{
			constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
		})

		// A client that always returns an error on Create
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

	// Test shared claim creation error
	t.Run("shared claim error", func(t *testing.T) {
		llmSvc := newManagedDRATestLLMISVC("test-svc", map[string]string{
			constants.ManagedDRADeviceClassAnnotationKey: "gpu.nvidia.com",
			constants.ManagedDRASharingAnnotationKey:     "true",
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
		assert.Contains(t, err.Error(), "failed to reconcile Managed DRA ResourceClaim")
	})
}
