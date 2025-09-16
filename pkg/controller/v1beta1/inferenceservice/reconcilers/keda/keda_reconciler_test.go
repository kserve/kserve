/*
Copyright 2025 The KServe Authors.

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

package keda

import (
	"strconv"
	"testing"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
)

func TestNewKedaReconciler(t *testing.T) {
	client := fake.NewClientBuilder().Build()
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 3,
	}
	configMap := &corev1.ConfigMap{}

	r, err := NewKedaReconciler(client, scheme.Scheme, componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, "test-component", r.ScaledObject.Name)
	assert.Equal(t, "test-namespace", r.ScaledObject.Namespace)
}

func TestGetKedaMetrics_ResourceMetricSourceType(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := createComponentExtensionWithResourceMetric()
	configMap := &corev1.ConfigMap{}

	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "cpu", triggers[0].Type)
	assert.Equal(t, "50", triggers[0].Metadata["value"])
}

func TestGetKedaMetrics_ExternalMetricSourceType(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := createComponentExtensionWithExternalMetric()
	configMap := &corev1.ConfigMap{}

	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "prometheus", triggers[0].Type)
	assert.Equal(t, "http://prometheus-server", triggers[0].Metadata["serverAddress"])
	assert.Equal(t, "http_requests_total", triggers[0].Metadata["query"])
	assert.Equal(t, "100.000000", triggers[0].Metadata["threshold"])
}

func TestGetKedaMetrics_PodMetricSourceType(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := createComponentExtensionWithPodMetric()
	configMap := &corev1.ConfigMap{}

	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "external", triggers[0].Type)
	assert.Equal(t, "http://otel-server", triggers[0].Metadata["scalerAddress"])
	// The metricQuery should now include namespace and deployment selectors
	assert.Equal(t, "sum(otel_query{namespace=\"test-namespace\", deployment=\"test-component\"})", triggers[0].Metadata["metricQuery"])
	assert.Equal(t, "200", triggers[0].Metadata["targetValue"])
}

func TestCreateKedaScaledObject(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 3,
	}
	configMap := &corev1.ConfigMap{}

	scaledObject, err := createKedaScaledObject(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.NotNil(t, scaledObject)
	assert.Equal(t, "test-component", scaledObject.Name)
	assert.Equal(t, "test-namespace", scaledObject.Namespace)
	assert.Equal(t, int32(1), *scaledObject.Spec.MinReplicaCount)
	assert.Equal(t, int32(3), *scaledObject.Spec.MaxReplicaCount)
}

func TestSemanticScaledObjectEquals(t *testing.T) {
	desired := createScaledObject(1, 3)
	existing := createScaledObject(1, 3)

	assert.True(t, semanticScaledObjectEquals(desired, existing))

	existing.Spec.MaxReplicaCount = ptr.To(int32(5))
	assert.False(t, semanticScaledObjectEquals(desired, existing))
}

func TestReconcile(t *testing.T) {
	_ = kedav1alpha1.AddToScheme(scheme.Scheme)
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 3,
	}
	configMap := &corev1.ConfigMap{}

	r, err := NewKedaReconciler(client, scheme.Scheme, componentMeta, componentExt, configMap)
	require.NoError(t, err)

	err = r.Reconcile(t.Context())
	require.NoError(t, err)

	scaledObject := &kedav1alpha1.ScaledObject{}
	err = client.Get(t.Context(), types.NamespacedName{Name: "test-component", Namespace: "test-namespace"}, scaledObject)
	require.NoError(t, err)
	assert.Equal(t, "test-component", scaledObject.Name)
	assert.Equal(t, "test-namespace", scaledObject.Namespace)
}

func TestSetControllerReferences(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 3,
	}
	configMap := &corev1.ConfigMap{}

	r, err := NewKedaReconciler(client, scheme.Scheme, componentMeta, componentExt, configMap)
	require.NoError(t, err)

	owner := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "owner",
			Namespace: "test-namespace",
		},
	}
	err = r.SetControllerReferences(owner, scheme.Scheme)
	require.NoError(t, err)
	assert.Equal(t, owner.Name, r.ScaledObject.OwnerReferences[0].Name)
}

func TestReconcile_CreateScaledObject(t *testing.T) {
	_ = kedav1alpha1.AddToScheme(scheme.Scheme)
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 3,
	}
	configMap := &corev1.ConfigMap{}

	r, err := NewKedaReconciler(client, scheme.Scheme, componentMeta, componentExt, configMap)
	require.NoError(t, err)

	err = r.Reconcile(t.Context())
	require.NoError(t, err)

	scaledObject := &kedav1alpha1.ScaledObject{}
	err = client.Get(t.Context(), types.NamespacedName{Name: "test-component", Namespace: "test-namespace"}, scaledObject)
	require.NoError(t, err)
	assert.Equal(t, "test-component", scaledObject.Name)
	assert.Equal(t, "test-namespace", scaledObject.Namespace)
	assert.Equal(t, int32(1), *scaledObject.Spec.MinReplicaCount)
	assert.Equal(t, int32(3), *scaledObject.Spec.MaxReplicaCount)
}

func TestReconcile_UpdateScaledObject(t *testing.T) {
	_ = kedav1alpha1.AddToScheme(scheme.Scheme)
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 3,
	}
	configMap := &corev1.ConfigMap{}

	r, err := NewKedaReconciler(client, scheme.Scheme, componentMeta, componentExt, configMap)
	require.NoError(t, err)

	// Create an existing ScaledObject with different MaxReplicaCount
	existingScaledObject := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-component",
			Namespace: "test-namespace",
		},
		Spec: kedav1alpha1.ScaledObjectSpec{
			MinReplicaCount: ptr.To(int32(1)),
			MaxReplicaCount: ptr.To(int32(5)),
		},
	}
	err = client.Create(t.Context(), existingScaledObject)
	require.NoError(t, err)

	err = r.Reconcile(t.Context())
	require.NoError(t, err)

	updatedScaledObject := &kedav1alpha1.ScaledObject{}
	err = client.Get(t.Context(), types.NamespacedName{Name: "test-component", Namespace: "test-namespace"}, updatedScaledObject)
	require.NoError(t, err)
	assert.Equal(t, int32(1), *updatedScaledObject.Spec.MinReplicaCount)
	assert.Equal(t, int32(3), *updatedScaledObject.Spec.MaxReplicaCount)
}

func TestGetKedaMetrics_AverageValueMetricSourceType(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ResourceMetricSourceType,
					Resource: &v1beta1.ResourceMetricSource{
						Name: v1beta1.ResourceMetricCPU,
						Target: v1beta1.MetricTarget{
							Type:         v1beta1.AverageValueMetricType,
							AverageValue: ptr.To(resource.MustParse("150m")),
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}

	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "cpu", triggers[0].Type)
	assert.Equal(t, "150m", triggers[0].Metadata["value"])
}

func TestGetKedaMetrics_ValueMetricSourceType(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ResourceMetricSourceType,
					Resource: &v1beta1.ResourceMetricSource{
						Name: v1beta1.ResourceMetricMemory,
						Target: v1beta1.MetricTarget{
							Type:  v1beta1.ValueMetricType,
							Value: ptr.To(resource.MustParse("512Mi")),
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}

	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "memory", triggers[0].Type)
	assert.Equal(t, "512Mi", triggers[0].Metadata["value"])
}

func TestGetKedaMetrics_DefaultCPUUtilization(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ResourceMetricSourceType,
					Resource: &v1beta1.ResourceMetricSource{
						Name: v1beta1.ResourceMetricCPU,
						Target: v1beta1.MetricTarget{
							Type: v1beta1.UtilizationMetricType,
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}

	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "cpu", triggers[0].Type)
	assert.Equal(t, strconv.Itoa(int(constants.DefaultCPUUtilization)), triggers[0].Metadata["value"])
}

func TestReconcile_HandleGetError(t *testing.T) {
	_ = kedav1alpha1.AddToScheme(scheme.Scheme)
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 3,
	}
	configMap := &corev1.ConfigMap{}

	r, err := NewKedaReconciler(client, scheme.Scheme, componentMeta, componentExt, configMap)
	require.NoError(t, err)

	// Simulate a client error by using an invalid name for the ScaledObject
	r.ScaledObject.Name = ""

	err = r.Reconcile(t.Context())
	assert.Error(t, err)
}

func TestCreateKedaScaledObject_DefaultMinReplicas(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MaxReplicas: 3,
	}
	configMap := &corev1.ConfigMap{}

	scaledObject, err := createKedaScaledObject(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.NotNil(t, scaledObject)
	assert.Equal(t, "test-component", scaledObject.Name)
	assert.Equal(t, "test-namespace", scaledObject.Namespace)
	assert.Equal(t, constants.DefaultMinReplicas, *scaledObject.Spec.MinReplicaCount)
	assert.Equal(t, int32(3), *scaledObject.Spec.MaxReplicaCount)
}

func TestCreateKedaScaledObject_MaxReplicasLessThanMinReplicas(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(5)),
		MaxReplicas: 3,
	}
	configMap := &corev1.ConfigMap{}

	scaledObject, err := createKedaScaledObject(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.NotNil(t, scaledObject)
	assert.Equal(t, "test-component", scaledObject.Name)
	assert.Equal(t, "test-namespace", scaledObject.Namespace)
	assert.Equal(t, int32(5), *scaledObject.Spec.MinReplicaCount)
	assert.Equal(t, int32(5), *scaledObject.Spec.MaxReplicaCount)
}

func TestGetKedaMetrics_NilAutoScaling(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: nil,
	}
	configMap := &corev1.ConfigMap{}

	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Empty(t, triggers)
}

func TestGetKedaMetrics_ResourceMetricSourceType_Utilization(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ResourceMetricSourceType,
					Resource: &v1beta1.ResourceMetricSource{
						Name: v1beta1.ResourceMetricCPU,
						Target: v1beta1.MetricTarget{
							Type:               v1beta1.UtilizationMetricType,
							AverageUtilization: ptr.To(int32(75)),
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}
	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "cpu", triggers[0].Type)
	assert.Equal(t, "75", triggers[0].Metadata["value"])
}

func TestGetKedaMetrics_ResourceMetricSourceType_Utilization_DefaultCPU(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ResourceMetricSourceType,
					Resource: &v1beta1.ResourceMetricSource{
						Name: v1beta1.ResourceMetricCPU,
						Target: v1beta1.MetricTarget{
							Type: v1beta1.UtilizationMetricType,
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}
	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "cpu", triggers[0].Type)
	assert.Equal(t, strconv.Itoa(int(constants.DefaultCPUUtilization)), triggers[0].Metadata["value"])
}

func TestGetKedaMetrics_ResourceMetricSourceType_AverageValue(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ResourceMetricSourceType,
					Resource: &v1beta1.ResourceMetricSource{
						Name: v1beta1.ResourceMetricMemory,
						Target: v1beta1.MetricTarget{
							Type:         v1beta1.AverageValueMetricType,
							AverageValue: ptr.To(resource.MustParse("256Mi")),
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}
	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "memory", triggers[0].Type)
	assert.Equal(t, "256Mi", triggers[0].Metadata["value"])
}

func TestGetKedaMetrics_ResourceMetricSourceType_Value(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ResourceMetricSourceType,
					Resource: &v1beta1.ResourceMetricSource{
						Name: v1beta1.ResourceMetricMemory,
						Target: v1beta1.MetricTarget{
							Type:  v1beta1.ValueMetricType,
							Value: ptr.To(resource.MustParse("512Mi")),
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}
	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	assert.Equal(t, "memory", triggers[0].Type)
	assert.Equal(t, "512Mi", triggers[0].Metadata["value"])
}

func TestGetKedaMetrics_ExternalMetricSourceType_WithNamespaceAndAuth(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ExternalMetricSourceType,
					External: &v1beta1.ExternalMetricSource{
						Metric: v1beta1.ExternalMetrics{
							Backend:       v1beta1.PrometheusBackend,
							ServerAddress: "http://prometheus-server",
							Query:         "http_requests_total",
							Namespace:     "test-ns",
						},
						Target: v1beta1.MetricTarget{
							Value: ptr.To(resource.MustParse("123")),
						},
						Authentication: &v1beta1.ExtMetricAuthentication{
							AuthModes: "bearer",
							AuthenticationRef: v1beta1.AuthenticationRef{
								Name: "auth-secret",
							},
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}
	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	trigger := triggers[0]
	assert.Equal(t, "prometheus", trigger.Type)
	assert.Equal(t, "http://prometheus-server", trigger.Metadata["serverAddress"])
	assert.Equal(t, "http_requests_total", trigger.Metadata["query"])
	assert.Equal(t, "123.000000", trigger.Metadata["threshold"])
	assert.Equal(t, "test-ns", trigger.Metadata["namespace"])
	assert.Equal(t, "bearer", trigger.Metadata["authModes"])
	assert.NotNil(t, trigger.AuthenticationRef)
	assert.Equal(t, "auth-secret", trigger.AuthenticationRef.Name)
}

func TestGetKedaMetrics_ExternalMetricSourceType_WithoutNamespaceOrAuth(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ExternalMetricSourceType,
					External: &v1beta1.ExternalMetricSource{
						Metric: v1beta1.ExternalMetrics{
							Backend:       v1beta1.PrometheusBackend,
							ServerAddress: "http://prometheus-server",
							Query:         "http_requests_total",
						},
						Target: v1beta1.MetricTarget{
							Value: ptr.To(resource.MustParse("99")),
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}
	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	trigger := triggers[0]
	assert.Equal(t, "prometheus", trigger.Type)
	assert.Equal(t, "http://prometheus-server", trigger.Metadata["serverAddress"])
	assert.Equal(t, "http_requests_total", trigger.Metadata["query"])
	assert.Equal(t, "99.000000", trigger.Metadata["threshold"])
	assert.Nil(t, trigger.AuthenticationRef)
}

func TestGetKedaMetrics_PodMetricSourceType_Success(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "test-component",
		Namespace: "test-namespace",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.PodMetricSourceType,
					PodMetric: &v1beta1.PodMetricSource{
						Metric: v1beta1.PodMetrics{
							Backend:           v1beta1.OpenTelemetryBackend,
							Query:             "otel_query",
							ServerAddress:     "http://otel-server",
							OperationOverTime: "sum",
						},
						Target: v1beta1.MetricTarget{
							Value: ptr.To(resource.MustParse("200")),
						},
					},
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}
	triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, triggers, 1)
	trigger := triggers[0]
	assert.Equal(t, "external", trigger.Type)
	assert.Equal(t, "sum(otel_query{namespace=\"test-namespace\", deployment=\"test-component\"})", trigger.Metadata["metricQuery"])
	assert.Equal(t, "200", trigger.Metadata["targetValue"])
	assert.Equal(t, "http://otel-server", trigger.Metadata["scalerAddress"])
	assert.Equal(t, "sum", trigger.Metadata["operationOverTime"])
}

func TestGetKedaMetrics_PodMetricSourceType_QuantityFormatConversion(t *testing.T) {
	testCases := []struct {
		name           string
		inputValue     string
		expectedOutput string
		description    string
	}{
		{
			name:           "Decimal format 0.9",
			inputValue:     "0.9",
			expectedOutput: "0.9",
			description:    "Direct decimal input should preserve decimal format",
		},
		{
			name:           "Decimal format 0.90",
			inputValue:     "0.90",
			expectedOutput: "0.9",
			description:    "Decimal with trailing zero should normalize to 0.9",
		},
		{
			name:           "Milli format 900m",
			inputValue:     "900m",
			expectedOutput: "0.9",
			description:    "Milli format should convert to decimal equivalent",
		},
		{
			name:           "Integer format 1",
			inputValue:     "1",
			expectedOutput: "1",
			description:    "Integer should remain as integer string",
		},
		{
			name:           "Integer milli format 1000m",
			inputValue:     "1000m",
			expectedOutput: "1",
			description:    "1000m should convert to 1",
		},
		{
			name:           "Half value 0.5",
			inputValue:     "0.5",
			expectedOutput: "0.5",
			description:    "Half value should preserve decimal format",
		},
		{
			name:           "Half value 500m",
			inputValue:     "500m",
			expectedOutput: "0.5",
			description:    "500m should convert to 0.5",
		},
		{
			name:           "Large integer 200",
			inputValue:     "200",
			expectedOutput: "200",
			description:    "Large integer should remain unchanged",
		},
		{
			name:           "Scientific notation 9e-1",
			inputValue:     "9e-1",
			expectedOutput: "0.9",
			description:    "Scientific notation should convert to decimal",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			componentMeta := metav1.ObjectMeta{
				Name:      "test-component",
				Namespace: "test-namespace",
			}
			componentExt := &v1beta1.ComponentExtensionSpec{
				AutoScaling: &v1beta1.AutoScalingSpec{
					Metrics: []v1beta1.MetricsSpec{
						{
							Type: v1beta1.PodMetricSourceType,
							PodMetric: &v1beta1.PodMetricSource{
								Metric: v1beta1.PodMetrics{
									Backend: v1beta1.OpenTelemetryBackend,
									Query:   "test_metric",
								},
								Target: v1beta1.MetricTarget{
									Value: ptr.To(resource.MustParse(tc.inputValue)),
								},
							},
						},
					},
				},
			}

			configMap := &corev1.ConfigMap{}
			triggers, err := getKedaMetrics(componentMeta, componentExt, configMap)
			require.NoError(t, err)
			require.Len(t, triggers, 1)

			trigger := triggers[0]
			assert.Equal(t, "external", trigger.Type)
			assert.Equal(t, tc.expectedOutput, trigger.Metadata["targetValue"],
				"Input %s should produce targetValue %s: %s", tc.inputValue, tc.expectedOutput, tc.description)
			assert.Equal(t, "sum(test_metric{namespace=\"test-namespace\", deployment=\"test-component\"})", trigger.Metadata["metricQuery"])
		})
	}
}

func TestCreateKedaScaledObject_SetsBasicFields(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:        "basic-component",
		Namespace:   "basic-namespace",
		Labels:      map[string]string{"foo": "bar"},
		Annotations: map[string]string{"anno": "val"},
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(2)),
		MaxReplicas: 5,
	}
	configMap := &corev1.ConfigMap{}

	scaledObject, err := createKedaScaledObject(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Equal(t, "basic-component", scaledObject.Name)
	assert.Equal(t, "basic-namespace", scaledObject.Namespace)
	assert.Equal(t, map[string]string{"foo": "bar"}, scaledObject.Labels)
	assert.Equal(t, map[string]string{"anno": "val"}, scaledObject.Annotations)
	assert.Equal(t, int32(2), *scaledObject.Spec.MinReplicaCount)
	assert.Equal(t, int32(5), *scaledObject.Spec.MaxReplicaCount)
	assert.Equal(t, "basic-component", scaledObject.Spec.ScaleTargetRef.Name)
}

func TestCreateKedaScaledObject_SetsTriggers(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "trigger-component",
		Namespace: "trigger-namespace",
	}
	componentExt := createComponentExtensionWithResourceMetric()
	configMap := &corev1.ConfigMap{}

	scaledObject, err := createKedaScaledObject(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Len(t, scaledObject.Spec.Triggers, 1)
	assert.Equal(t, "cpu", scaledObject.Spec.Triggers[0].Type)
}

func TestCreateKedaScaledObject_UsesDefaultMinReplicas(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "default-min",
		Namespace: "default-ns",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MaxReplicas: 4,
	}
	configMap := &corev1.ConfigMap{}

	scaledObject, err := createKedaScaledObject(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	assert.Equal(t, constants.DefaultMinReplicas, *scaledObject.Spec.MinReplicaCount)
	assert.Equal(t, int32(4), *scaledObject.Spec.MaxReplicaCount)
}

func TestCreateKedaScaledObject_AdvancedConfigFromComponentExt(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "advanced",
		Namespace: "ns",
	}
	sdWin := int32(60)
	suWin := int32(30)
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 3,
		AutoScaling: &v1beta1.AutoScalingSpec{
			Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
				ScaleDown: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: &sdWin,
				},
				ScaleUp: &autoscalingv2.HPAScalingRules{
					StabilizationWindowSeconds: &suWin,
				},
			},
		},
	}
	configMap := &corev1.ConfigMap{}

	scaledObject, err := createKedaScaledObject(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	require.NotNil(t, scaledObject.Spec.Advanced)
	require.NotNil(t, scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig)
	require.NotNil(t, scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig.Behavior)
	assert.Equal(t, &sdWin, scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig.Behavior.ScaleDown.StabilizationWindowSeconds)
	assert.Equal(t, &suWin, scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig.Behavior.ScaleUp.StabilizationWindowSeconds)
}

func TestCreateKedaScaledObject_AdvancedConfigFromConfigMap(t *testing.T) {
	componentMeta := metav1.ObjectMeta{
		Name:      "from-cm",
		Namespace: "ns",
	}
	componentExt := &v1beta1.ComponentExtensionSpec{
		MinReplicas: ptr.To(int32(1)),
		MaxReplicas: 2,
		AutoScaling: &v1beta1.AutoScalingSpec{},
	}
	configMap := &corev1.ConfigMap{
		Data: map[string]string{
			"autoscaler": `{
				"scaleUpStabilizationWindowSeconds": "15",
				"scaleDownStabilizationWindowSeconds": "45"
			}`,
		},
	}

	scaledObject, err := createKedaScaledObject(componentMeta, componentExt, configMap)
	require.NoError(t, err)
	require.NotNil(t, scaledObject.Spec.Advanced)
	require.NotNil(t, scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig)
	require.NotNil(t, scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig.Behavior)
	assert.Equal(t, ptr.To(int32(45)), scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig.Behavior.ScaleDown.StabilizationWindowSeconds)
	assert.Equal(t, ptr.To(int32(15)), scaledObject.Spec.Advanced.HorizontalPodAutoscalerConfig.Behavior.ScaleUp.StabilizationWindowSeconds)
}

// Helper functions for creating test data
func createComponentExtensionWithResourceMetric() *v1beta1.ComponentExtensionSpec {
	return &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ResourceMetricSourceType,
					Resource: &v1beta1.ResourceMetricSource{
						Name: v1beta1.ResourceMetricCPU,
						Target: v1beta1.MetricTarget{
							Type:               v1beta1.UtilizationMetricType,
							AverageUtilization: ptr.To(int32(50)),
						},
					},
				},
			},
		},
	}
}

func createComponentExtensionWithExternalMetric() *v1beta1.ComponentExtensionSpec {
	return &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.ExternalMetricSourceType,
					External: &v1beta1.ExternalMetricSource{
						Metric: v1beta1.ExternalMetrics{
							Backend:       v1beta1.PrometheusBackend,
							ServerAddress: "http://prometheus-server",
							Query:         "http_requests_total",
							Namespace:     "test-namespace",
						},
						Target: v1beta1.MetricTarget{
							Value: ptr.To(resource.MustParse("100")),
						},
					},
				},
			},
		},
	}
}

func createComponentExtensionWithPodMetric() *v1beta1.ComponentExtensionSpec {
	return &v1beta1.ComponentExtensionSpec{
		AutoScaling: &v1beta1.AutoScalingSpec{
			Metrics: []v1beta1.MetricsSpec{
				{
					Type: v1beta1.PodMetricSourceType,
					PodMetric: &v1beta1.PodMetricSource{
						Metric: v1beta1.PodMetrics{
							Backend:           v1beta1.OpenTelemetryBackend,
							Query:             "otel_query",
							ServerAddress:     "http://otel-server",
							OperationOverTime: "sum",
						},
						Target: v1beta1.MetricTarget{
							Value: ptr.To(resource.MustParse("200")),
						},
					},
				},
			},
		},
	}
}

func createScaledObject(minReplicas, maxReplicas int32) *kedav1alpha1.ScaledObject {
	return &kedav1alpha1.ScaledObject{
		Spec: kedav1alpha1.ScaledObjectSpec{
			MinReplicaCount: ptr.To(minReplicas),
			MaxReplicaCount: ptr.To(maxReplicas),
		},
	}
}
