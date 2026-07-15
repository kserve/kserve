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

package llmisvc

import (
	"context"
	"testing"

	kedav1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"knative.dev/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	lwsapi "sigs.k8s.io/lws/api/leaderworkerset/v1"

	"github.com/kserve/kserve/pkg/apis/serving/v1alpha2"
)

func deploymentScaleTargetRef(name string) autoscalingv2.CrossVersionObjectReference {
	return autoscalingv2.CrossVersionObjectReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       name,
	}
}

func lwsScaleTargetRef(name string) autoscalingv2.CrossVersionObjectReference {
	return autoscalingv2.CrossVersionObjectReference{
		APIVersion: lwsapi.GroupVersion.String(),
		Kind:       "LeaderWorkerSet",
		Name:       name,
	}
}

// newTestLLMISVC creates a minimal LLMInferenceService for testing.
func newTestLLMISVC(name, namespace string) *v1alpha2.LLMInferenceService {
	return &v1alpha2.LLMInferenceService{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "serving.kserve.io/v1alpha2",
			Kind:       "LLMInferenceService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid-1234",
		},
		Spec: v1alpha2.LLMInferenceServiceSpec{
			Model: v1alpha2.LLMModelSpec{
				URI:  apis.URL{Scheme: "hf", Host: "meta-llama/Llama-3.1-8B"},
				Name: ptr.To("meta-llama/Llama-3.1-8B"),
			},
		},
	}
}

func TestExpectedHPA(t *testing.T) {
	tests := []struct {
		name           string
		llmSvc         *v1alpha2.LLMInferenceService
		scaling        *v1alpha2.ScalingSpec
		scaleTargetRef autoscalingv2.CrossVersionObjectReference
		hpaName        string
		workloadLabels map[string]string
		validate       func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler)
	}{
		{
			name:           "default minReplicas when nil",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.NotNil(t, hpa.Spec.MinReplicas)
				assert.Equal(t, int32(1), *hpa.Spec.MinReplicas, "default minReplicas should be 1")
			},
		},
		{
			name:   "explicit minReplicas",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MinReplicas: ptr.To(int32(3)),
				MaxReplicas: 10,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}},
			},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.NotNil(t, hpa.Spec.MinReplicas)
				assert.Equal(t, int32(3), *hpa.Spec.MinReplicas)
				assert.Equal(t, int32(10), hpa.Spec.MaxReplicas)
			},
		},
		{
			name:           "scaleTargetRef points to correct deployment",
			llmSvc:         newTestLLMISVC("my-model", "prod"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("my-model-kserve"),
			hpaName:        "my-model-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Equal(t, "apps/v1", hpa.Spec.ScaleTargetRef.APIVersion)
				assert.Equal(t, "Deployment", hpa.Spec.ScaleTargetRef.Kind)
				assert.Equal(t, "my-model-kserve", hpa.Spec.ScaleTargetRef.Name)
			},
		},
		{
			name:           "scaleTargetRef points to LeaderWorkerSet for multi-node",
			llmSvc:         newTestLLMISVC("my-model", "prod"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: lwsScaleTargetRef("my-model-kserve-mn"),
			hpaName:        "my-model-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Equal(t, lwsapi.GroupVersion.String(), hpa.Spec.ScaleTargetRef.APIVersion)
				assert.Equal(t, "LeaderWorkerSet", hpa.Spec.ScaleTargetRef.Kind)
				assert.Equal(t, "my-model-kserve-mn", hpa.Spec.ScaleTargetRef.Name)
			},
		},
		{
			name:           "metric selector uses variant_name from HPA name",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.Len(t, hpa.Spec.Metrics, 1)
				metric := hpa.Spec.Metrics[0]
				assert.Equal(t, autoscalingv2.ExternalMetricSourceType, metric.Type)
				require.NotNil(t, metric.External)
				assert.Equal(t, "wva_desired_replicas", metric.External.Metric.Name)
				require.NotNil(t, metric.External.Metric.Selector)
				assert.Equal(t, "test-svc-kserve-hpa", metric.External.Metric.Selector.MatchLabels["variant_name"])
				assert.Equal(t, autoscalingv2.ValueMetricType, metric.External.Target.Type)
				assert.Equal(t, resource.NewQuantity(1, resource.DecimalSI), metric.External.Target.Value)
			},
		},
		{
			name:   "custom HPA behavior is forwarded",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA: &v1alpha2.WVASpec{
					ActuatorSpec: v1alpha2.ActuatorSpec{
						HPA: &v1alpha2.HPAScalingSpec{
							Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
								ScaleDown: &autoscalingv2.HPAScalingRules{
									StabilizationWindowSeconds: ptr.To(int32(120)),
								},
							},
						},
					},
				},
			},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.NotNil(t, hpa.Spec.Behavior)
				require.NotNil(t, hpa.Spec.Behavior.ScaleDown)
				assert.Equal(t, int32(120), *hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds)
			},
		},
		{
			name:           "nil behavior when not specified",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Nil(t, hpa.Spec.Behavior)
			},
		},
		{
			name:           "owner reference is set",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				require.Len(t, hpa.OwnerReferences, 1)
				assert.Equal(t, "test-svc", hpa.OwnerReferences[0].Name)
				assert.Equal(t, "LLMInferenceService", hpa.OwnerReferences[0].Kind)
				assert.True(t, *hpa.OwnerReferences[0].Controller)
			},
		},
		{
			name:           "WVA managed annotation is set",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Equal(t, "true", hpa.Annotations[wvaManagedAnnotation])
			},
		},
		{
			name:           "WVA model-id annotation uses model name",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Equal(t, "meta-llama/Llama-3.1-8B", hpa.Annotations[wvaModelIDAnnotation])
			},
		},
		{
			name: "WVA model-id annotation falls back to URI when name is nil",
			llmSvc: func() *v1alpha2.LLMInferenceService {
				svc := newTestLLMISVC("test-svc", "test-ns")
				svc.Spec.Model.Name = nil
				return svc
			}(),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				expectedURI := apis.URL{Scheme: "hf", Host: "meta-llama/Llama-3.1-8B"}
				assert.Equal(t, expectedURI.String(), hpa.Annotations[wvaModelIDAnnotation])
			},
		},
		{
			name:   "WVA variant-cost annotation set when variantCost specified",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{VariantCost: "42.5", ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}},
			},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Equal(t, "42.5", hpa.Annotations[wvaVariantCostAnnotation])
			},
		},
		{
			name:           "WVA variant-cost annotation absent when variantCost empty",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				_, exists := hpa.Annotations[wvaVariantCostAnnotation]
				assert.False(t, exists, "variant-cost annotation should not be present when variantCost is empty")
			},
		},
		{
			name:           "acceleratorName label taken from workload labels when present",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			workloadLabels: map[string]string{
				acceleratorNameLabelKey: "H100",
			},
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Equal(t, "H100", hpa.Labels[acceleratorNameLabelKey])
			},
		},
		{
			name:           "acceleratorName label is unknown when not present in workload labels",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			workloadLabels: nil,
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Equal(t, "unknown", hpa.Labels[acceleratorNameLabelKey])
			},
		},
		{
			name:           "acceleratorName label is unknown when present but empty in workload labels",
			llmSvc:         newTestLLMISVC("test-svc", "test-ns"),
			scaling:        &v1alpha2.ScalingSpec{MaxReplicas: 5, WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{HPA: &v1alpha2.HPAScalingSpec{}}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			hpaName:        "test-svc-kserve-hpa",
			workloadLabels: map[string]string{
				acceleratorNameLabelKey: "",
			},
			validate: func(t *testing.T, hpa *autoscalingv2.HorizontalPodAutoscaler) {
				assert.Equal(t, "unknown", hpa.Labels[acceleratorNameLabelKey])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hpa := expectedHPA(tt.llmSvc, tt.scaling, tt.scaleTargetRef, tt.hpaName, tt.workloadLabels)
			assert.Equal(t, tt.hpaName, hpa.Name)
			assert.Equal(t, tt.llmSvc.Namespace, hpa.Namespace)
			tt.validate(t, hpa)
		})
	}
}

func TestExpectedScaledObject(t *testing.T) {
	tests := []struct {
		name           string
		llmSvc         *v1alpha2.LLMInferenceService
		scaling        *v1alpha2.ScalingSpec
		config         *Config
		scaleTargetRef autoscalingv2.CrossVersionObjectReference
		soName         string
		workloadLabels map[string]string
		validate       func(t *testing.T, so *kedav1alpha1.ScaledObject)
	}{
		{
			name:   "default minReplicas when nil",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.NotNil(t, so.Spec.MinReplicaCount)
				assert.Equal(t, int32(1), *so.Spec.MinReplicaCount)
			},
		},
		{
			name:   "explicit minReplicas and maxReplicas",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MinReplicas: ptr.To(int32(2)),
				MaxReplicas: 8,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, int32(2), *so.Spec.MinReplicaCount)
				require.NotNil(t, so.Spec.MaxReplicaCount)
				assert.Equal(t, int32(8), *so.Spec.MaxReplicaCount)
			},
		},
		{
			name:   "scaleTargetRef points to correct deployment",
			llmSvc: newTestLLMISVC("my-model", "prod"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("my-model-kserve-prefill"),
			soName:         "my-model-kserve-prefill-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.NotNil(t, so.Spec.ScaleTargetRef)
				assert.Equal(t, "apps/v1", so.Spec.ScaleTargetRef.APIVersion)
				assert.Equal(t, "Deployment", so.Spec.ScaleTargetRef.Kind)
				assert.Equal(t, "my-model-kserve-prefill", so.Spec.ScaleTargetRef.Name)
			},
		},
		{
			name:   "scaleTargetRef points to LeaderWorkerSet for multi-node",
			llmSvc: newTestLLMISVC("my-model", "prod"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: lwsScaleTargetRef("my-model-kserve-mn"),
			soName:         "my-model-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.NotNil(t, so.Spec.ScaleTargetRef)
				assert.Equal(t, lwsapi.GroupVersion.String(), so.Spec.ScaleTargetRef.APIVersion)
				assert.Equal(t, "LeaderWorkerSet", so.Spec.ScaleTargetRef.Kind)
				assert.Equal(t, "my-model-kserve-mn", so.Spec.ScaleTargetRef.Name)
			},
		},
		{
			name:   "prometheus trigger has correct metadata",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "https://prom.monitoring:9090", TLSInsecureSkipVerify: true}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.Len(t, so.Spec.Triggers, 1)
				trigger := so.Spec.Triggers[0]
				assert.Equal(t, "prometheus", trigger.Type)
				assert.Equal(t, "wva-desired-replicas", trigger.Name)
				assert.Equal(t, "https://prom.monitoring:9090", trigger.Metadata["serverAddress"])
				assert.Equal(t, `wva_desired_replicas{variant_name="test-svc-kserve-keda",exported_namespace="test-ns"}`, trigger.Metadata["query"])
				assert.Equal(t, "1", trigger.Metadata["threshold"])
				assert.Equal(t, "true", trigger.Metadata["unsafeSsl"])
			},
		},
		{
			name:   "unsafeSsl is false when TLS verification enabled",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090", TLSInsecureSkipVerify: false}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "false", so.Spec.Triggers[0].Metadata["unsafeSsl"])
			},
		},
		{
			name:   "optional KEDA fields forwarded",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA: &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{
					PollingInterval: ptr.To(int32(15)),
					CooldownPeriod:  ptr.To(int32(60)),
				}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.NotNil(t, so.Spec.PollingInterval)
				assert.Equal(t, int32(15), *so.Spec.PollingInterval)
				require.NotNil(t, so.Spec.CooldownPeriod)
				assert.Equal(t, int32(60), *so.Spec.CooldownPeriod)
			},
		},
		{
			name:   "owner reference is set",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.Len(t, so.OwnerReferences, 1)
				assert.Equal(t, "test-svc", so.OwnerReferences[0].Name)
				assert.Equal(t, "LLMInferenceService", so.OwnerReferences[0].Kind)
				assert.True(t, *so.OwnerReferences[0].Controller)
			},
		},
		{
			name:   "no auth fields when not configured",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				trigger := so.Spec.Triggers[0]
				assert.NotContains(t, trigger.Metadata, "authModes")
				assert.Nil(t, trigger.AuthenticationRef)
			},
		},
		{
			name:   "TriggerAuthentication auth fields are wired into trigger",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config: &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom.monitoring:9090",
					AuthModes:       "bearer",
					TriggerAuthName: "prom-bearer-auth",
					TriggerAuthKind: "TriggerAuthentication",
				},
			}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				trigger := so.Spec.Triggers[0]
				assert.Equal(t, "bearer", trigger.Metadata["authModes"])
				require.NotNil(t, trigger.AuthenticationRef)
				assert.Equal(t, "prom-bearer-auth", trigger.AuthenticationRef.Name)
				assert.Equal(t, "TriggerAuthentication", trigger.AuthenticationRef.Kind)
			},
		},
		{
			name:   "ClusterTriggerAuthentication auth fields are wired into trigger",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config: &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom.monitoring:9090",
					AuthModes:       "bearer",
					TriggerAuthName: "cluster-prom-auth",
					TriggerAuthKind: "ClusterTriggerAuthentication",
				},
			}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				trigger := so.Spec.Triggers[0]
				assert.Equal(t, "bearer", trigger.Metadata["authModes"])
				require.NotNil(t, trigger.AuthenticationRef)
				assert.Equal(t, "cluster-prom-auth", trigger.AuthenticationRef.Name)
				assert.Equal(t, "ClusterTriggerAuthentication", trigger.AuthenticationRef.Kind)
			},
		},
		{
			name:   "WVA managed annotation is set",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "true", so.Annotations[wvaManagedAnnotation])
			},
		},
		{
			name:   "WVA model-id annotation uses model name",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "meta-llama/Llama-3.1-8B", so.Annotations[wvaModelIDAnnotation])
			},
		},
		{
			name: "WVA model-id annotation falls back to URI when name is nil",
			llmSvc: func() *v1alpha2.LLMInferenceService {
				svc := newTestLLMISVC("test-svc", "test-ns")
				svc.Spec.Model.Name = nil
				return svc
			}(),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				expectedURI := apis.URL{Scheme: "hf", Host: "meta-llama/Llama-3.1-8B"}
				assert.Equal(t, expectedURI.String(), so.Annotations[wvaModelIDAnnotation])
			},
		},
		{
			name:   "WVA variant-cost annotation set when variantCost specified",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{VariantCost: "42.5", ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "42.5", so.Annotations[wvaVariantCostAnnotation])
			},
		},
		{
			name:   "WVA variant-cost annotation absent when variantCost empty",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				_, exists := so.Annotations[wvaVariantCostAnnotation]
				assert.False(t, exists, "variant-cost annotation should not be present when variantCost is empty")
			},
		},
		{
			name:   "acceleratorName label taken from workload labels when present",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			workloadLabels: map[string]string{
				acceleratorNameLabelKey: "H100",
			},
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "H100", so.Labels[acceleratorNameLabelKey])
			},
		},
		{
			name:   "acceleratorName label is unknown when not present in workload labels",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			workloadLabels: nil,
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "unknown", so.Labels[acceleratorNameLabelKey])
			},
		},
		{
			name:   "acceleratorName label is unknown when present but empty in workload labels",
			llmSvc: newTestLLMISVC("test-svc", "test-ns"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 5,
				WVA:         &v1alpha2.WVASpec{ActuatorSpec: v1alpha2.ActuatorSpec{KEDA: &v1alpha2.KEDAScalingSpec{}}},
			},
			config:         &Config{WVAAutoscalingConfig: &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}}},
			scaleTargetRef: deploymentScaleTargetRef("test-svc-kserve"),
			soName:         "test-svc-kserve-keda",
			workloadLabels: map[string]string{
				acceleratorNameLabelKey: "",
			},
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "unknown", so.Labels[acceleratorNameLabelKey])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			so := expectedScaledObject(tt.llmSvc, tt.scaling, tt.config, tt.scaleTargetRef, tt.soName, tt.workloadLabels)
			assert.Equal(t, tt.soName, so.Name)
			assert.Equal(t, tt.llmSvc.Namespace, so.Namespace)
			tt.validate(t, so)
		})
	}
}

func TestExpectedDirectScaledObject(t *testing.T) {
	tests := []struct {
		name           string
		llmSvc         *v1alpha2.LLMInferenceService
		scaling        *v1alpha2.ScalingSpec
		scaleTargetRef autoscalingv2.CrossVersionObjectReference
		soName         string
		validate       func(t *testing.T, so *kedav1alpha1.ScaledObject)
	}{
		{
			name:   "uses user-defined triggers",
			llmSvc: newTestLLMISVC("my-model", "prod"),
			scaling: &v1alpha2.ScalingSpec{
				MinReplicas: ptr.To(int32(1)),
				MaxReplicas: 5,
				KEDA: &v1alpha2.DirectKEDAScalingSpec{
					KEDAScalingSpec: v1alpha2.KEDAScalingSpec{
						PollingInterval: ptr.To(int32(30)),
					},
					Triggers: []kedav1alpha1.ScaleTriggers{
						{
							Type: "cpu",
							Metadata: map[string]string{
								"value": "80",
							},
						},
					},
				},
			},
			scaleTargetRef: deploymentScaleTargetRef("my-model-kserve"),
			soName:         "my-model-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				require.Len(t, so.Spec.Triggers, 1)
				assert.Equal(t, "cpu", so.Spec.Triggers[0].Type)
				assert.Equal(t, "80", so.Spec.Triggers[0].Metadata["value"])
				assert.NotEqual(t, "prometheus", so.Spec.Triggers[0].Type)
				assert.Equal(t, int32(30), *so.Spec.PollingInterval)
				assert.Equal(t, int32(1), *so.Spec.MinReplicaCount)
				assert.Equal(t, int32(5), *so.Spec.MaxReplicaCount)
				assert.Empty(t, so.Annotations)
			},
		},
		{
			name:   "scale target ref points to deployment",
			llmSvc: newTestLLMISVC("sim-llama", "default"),
			scaling: &v1alpha2.ScalingSpec{
				MaxReplicas: 3,
				KEDA: &v1alpha2.DirectKEDAScalingSpec{
					Triggers: []kedav1alpha1.ScaleTriggers{
						{Type: "prometheus", Metadata: map[string]string{"serverAddress": "http://prom:9090", "query": "up", "threshold": "1"}},
					},
				},
			},
			scaleTargetRef: deploymentScaleTargetRef("sim-llama-kserve"),
			soName:         "sim-llama-kserve-keda",
			validate: func(t *testing.T, so *kedav1alpha1.ScaledObject) {
				assert.Equal(t, "apps/v1", so.Spec.ScaleTargetRef.APIVersion)
				assert.Equal(t, "Deployment", so.Spec.ScaleTargetRef.Kind)
				assert.Equal(t, "sim-llama-kserve", so.Spec.ScaleTargetRef.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			so := expectedDirectScaledObject(tt.llmSvc, tt.scaling, tt.scaleTargetRef, tt.soName)
			assert.Equal(t, tt.soName, so.Name)
			assert.Equal(t, tt.llmSvc.Namespace, so.Namespace)
			tt.validate(t, so)
		})
	}
}

func TestValidateAutoscalingConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *WVAAutoscalingConfig
		wantErr string
	}{
		{
			name:    "nil config returns error",
			cfg:     nil,
			wantErr: "autoscaling-wva-controller-config.prometheus.url is required",
		},
		{
			name:    "missing prometheus.url returns error",
			cfg:     &WVAAutoscalingConfig{},
			wantErr: "autoscaling-wva-controller-config.prometheus.url is required",
		},
		{
			name: "no auth fields is valid",
			cfg:  &WVAAutoscalingConfig{Prometheus: PrometheusConfig{URL: "http://prom:9090"}},
		},
		{
			name: "both auth fields set is valid",
			cfg: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom:9090",
					AuthModes:       "bearer",
					TriggerAuthName: "prom-auth",
				},
			},
		},
		{
			name: "authModes set without triggerAuthName returns error",
			cfg: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:       "https://prom:9090",
					AuthModes: "bearer",
				},
			},
			wantErr: "autoscaling-wva-controller-config.prometheus.authModes and autoscaling-wva-controller-config.prometheus.triggerAuthName must both be set or both be empty",
		},
		{
			name: "triggerAuthName set without authModes returns error",
			cfg: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom:9090",
					TriggerAuthName: "prom-auth",
				},
			},
			wantErr: "autoscaling-wva-controller-config.prometheus.authModes and autoscaling-wva-controller-config.prometheus.triggerAuthName must both be set or both be empty",
		},
		{
			name: "ClusterTriggerAuthentication kind with both auth fields is valid",
			cfg: &WVAAutoscalingConfig{
				Prometheus: PrometheusConfig{
					URL:             "https://prom:9090",
					AuthModes:       "bearer",
					TriggerAuthName: "cluster-prom-auth",
					TriggerAuthKind: "ClusterTriggerAuthentication",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAutoscalingConfig(tt.cfg)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSemanticHPAIsEqual(t *testing.T) {
	base := func() *autoscalingv2.HorizontalPodAutoscaler {
		return &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MaxReplicas: 5,
				MinReplicas: ptr.To(int32(1)),
			},
		}
	}

	t.Run("equal specs returns true", func(t *testing.T) {
		assert.True(t, semanticHPAIsEqual(base(), base()))
	})

	t.Run("different maxReplicas returns false", func(t *testing.T) {
		modified := base()
		modified.Spec.MaxReplicas = 10
		assert.False(t, semanticHPAIsEqual(base(), modified))
	})

	t.Run("different labels returns false", func(t *testing.T) {
		modified := base()
		modified.Labels = map[string]string{"app": "other"}
		assert.False(t, semanticHPAIsEqual(base(), modified))
	})

	t.Run("removed field in expected is detected", func(t *testing.T) {
		expected := base()
		expected.Spec.MinReplicas = nil
		assert.False(t, semanticHPAIsEqual(expected, base()))
	})

	t.Run("extra label on curr is detected", func(t *testing.T) {
		curr := base()
		curr.Labels["extra"] = "value"
		assert.False(t, semanticHPAIsEqual(base(), curr))
	})

	t.Run("removed annotation in expected is detected", func(t *testing.T) {
		expected := base()
		expected.Annotations = nil
		curr := base()
		curr.Annotations = map[string]string{"note": "old"}
		assert.False(t, semanticHPAIsEqual(expected, curr))
	})
}

func TestSemanticScaledObjectIsEqual(t *testing.T) {
	base := func() *kedav1alpha1.ScaledObject {
		return &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
			Spec: kedav1alpha1.ScaledObjectSpec{
				MaxReplicaCount: ptr.To(int32(5)),
				Triggers: []kedav1alpha1.ScaleTriggers{
					{Type: "prometheus", Metadata: map[string]string{"query": "up"}},
				},
			},
		}
	}

	t.Run("equal specs returns true", func(t *testing.T) {
		assert.True(t, semanticScaledObjectIsEqual(base(), base()))
	})

	t.Run("different trigger returns false", func(t *testing.T) {
		modified := base()
		modified.Spec.Triggers[0].Metadata["query"] = "down"
		assert.False(t, semanticScaledObjectIsEqual(base(), modified))
	})

	t.Run("different labels returns false", func(t *testing.T) {
		modified := base()
		modified.Labels = map[string]string{"app": "other"}
		assert.False(t, semanticScaledObjectIsEqual(base(), modified))
	})

	t.Run("removed optional field in expected is detected", func(t *testing.T) {
		expected := base()
		expected.Spec.MaxReplicaCount = nil
		assert.False(t, semanticScaledObjectIsEqual(expected, base()))
	})

	t.Run("extra label on curr is detected", func(t *testing.T) {
		curr := base()
		curr.Labels["extra"] = "value"
		assert.False(t, semanticScaledObjectIsEqual(base(), curr))
	})
}

func TestPreserveKEDAManagedMetadata(t *testing.T) {
	hook := PreserveKEDAManagedMetadata()

	// Extract the AfterDryRunFunc from the UpdateOption by applying it to updateOptions.
	opts := &updateOptions[*kedav1alpha1.ScaledObject]{}
	hook(opts)
	require.Len(t, opts.afterDryRunFns, 1)
	fn := opts.afterDryRunFns[0]

	t.Run("copies KEDA label from curr into expected", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				"app":                                    "test",
				kedav1alpha1.ScaledObjectOwnerAnnotation: "my-so",
			}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, "my-so", expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation])
		assert.Equal(t, "test", expected.Labels["app"])
	})

	t.Run("no-op when curr has no KEDA label", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, map[string]string{"app": "test"}, expected.Labels)
	})

	t.Run("initializes nil labels map when KEDA label present on curr", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				kedav1alpha1.ScaledObjectOwnerAnnotation: "my-so",
			}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, "my-so", expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation])
	})

	t.Run("copies finalizers from curr into expected", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{Finalizers: []string{"finalizer.keda.sh"}},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, []string{"finalizer.keda.sh"}, expected.Finalizers)
	})

	t.Run("no-op when curr has no finalizers", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{}
		fn(expected, expected.DeepCopy(), curr)
		assert.Empty(t, expected.Finalizers)
	})

	t.Run("preserves both KEDA label and finalizers together", func(t *testing.T) {
		expected := &kedav1alpha1.ScaledObject{}
		curr := &kedav1alpha1.ScaledObject{
			ObjectMeta: metav1.ObjectMeta{
				Labels:     map[string]string{kedav1alpha1.ScaledObjectOwnerAnnotation: "my-so"},
				Finalizers: []string{"finalizer.keda.sh"},
			},
		}
		fn(expected, expected.DeepCopy(), curr)
		assert.Equal(t, "my-so", expected.Labels[kedav1alpha1.ScaledObjectOwnerAnnotation])
		assert.Equal(t, []string{"finalizer.keda.sh"}, expected.Finalizers)
	})
}

func TestNamingHelpers(t *testing.T) {
	svc := newTestLLMISVC("sim-llama", "llm-d-dev")

	t.Run("main deployment name (standard)", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve", mainDeploymentName(svc))
	})

	t.Run("prefill deployment name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill", prefillDeploymentName(svc))
	})

	t.Run("main HPA name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-hpa", mainHPAName(svc))
	})

	t.Run("prefill HPA name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill-hpa", prefillHPAName(svc))
	})

	t.Run("main ScaledObject name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-keda", mainScaledObjectName(svc))
	})

	t.Run("prefill ScaledObject name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-prefill-keda", prefillScaledObjectName(svc))
	})

	t.Run("main LWS name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-mn", mainLWSName(svc))
	})

	t.Run("prefill LWS name", func(t *testing.T) {
		assert.Equal(t, "sim-llama-kserve-mn-prefill", prefillLWSName(svc))
	})
}

func TestScaleTargetRefHelpers(t *testing.T) {
	t.Run("mainScaleTargetRef returns Deployment when no worker", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		ref := mainScaleTargetRef(svc)
		assert.Equal(t, "apps/v1", ref.APIVersion)
		assert.Equal(t, "Deployment", ref.Kind)
		assert.Equal(t, mainDeploymentName(svc), ref.Name)
	})

	t.Run("mainScaleTargetRef returns LWS when worker is set", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		svc.Spec.Worker = &corev1.PodSpec{}
		ref := mainScaleTargetRef(svc)
		assert.Equal(t, lwsapi.GroupVersion.String(), ref.APIVersion)
		assert.Equal(t, "LeaderWorkerSet", ref.Kind)
		assert.Equal(t, mainLWSName(svc), ref.Name)
	})

	t.Run("prefillScaleTargetRef returns Deployment when no prefill worker", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		ref := prefillScaleTargetRef(svc)
		assert.Equal(t, "apps/v1", ref.APIVersion)
		assert.Equal(t, "Deployment", ref.Kind)
		assert.Equal(t, prefillDeploymentName(svc), ref.Name)
	})

	t.Run("prefillScaleTargetRef returns LWS when prefill worker is set", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		svc.Spec.Prefill = &v1alpha2.WorkloadSpec{
			Worker: &corev1.PodSpec{},
		}
		ref := prefillScaleTargetRef(svc)
		assert.Equal(t, lwsapi.GroupVersion.String(), ref.APIVersion)
		assert.Equal(t, "LeaderWorkerSet", ref.Kind)
		assert.Equal(t, prefillLWSName(svc), ref.Name)
	})

	t.Run("prefillScaleTargetRef returns Deployment when prefill has no worker", func(t *testing.T) {
		svc := newTestLLMISVC("test-svc", "test-ns")
		svc.Spec.Prefill = &v1alpha2.WorkloadSpec{}
		ref := prefillScaleTargetRef(svc)
		assert.Equal(t, "apps/v1", ref.APIVersion)
		assert.Equal(t, "Deployment", ref.Kind)
		assert.Equal(t, prefillDeploymentName(svc), ref.Name)
	})
}

func newReconcilerWithHPA(hpa *autoscalingv2.HorizontalPodAutoscaler) *LLMISVCReconciler {
	scheme := runtime.NewScheme()
	_ = autoscalingv2.AddToScheme(scheme)
	cb := fake.NewClientBuilder().WithScheme(scheme)
	if hpa != nil {
		cb = cb.WithObjects(hpa)
	}
	return &LLMISVCReconciler{
		Client:        cb.Build(),
		EventRecorder: record.NewFakeRecorder(10),
	}
}

func newReconcilerWithScaledObject(so *kedav1alpha1.ScaledObject) *LLMISVCReconciler {
	scheme := runtime.NewScheme()
	_ = kedav1alpha1.AddToScheme(scheme)
	cb := fake.NewClientBuilder().WithScheme(scheme)
	if so != nil {
		cb = cb.WithObjects(so)
	}
	return &LLMISVCReconciler{
		Client:        cb.Build(),
		EventRecorder: record.NewFakeRecorder(10),
	}
}

func TestPropagateHPAStatus(t *testing.T) {
	expectedHPAObj := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hpa", Namespace: "test-ns"},
	}

	tests := []struct {
		name         string
		hpa          *autoscalingv2.HorizontalPodAutoscaler
		wantReady    bool
		wantNotReady bool
		wantReason   string
		wantErr      bool
	}{
		{
			name: "AbleToScale=True and ScalingActive=True -> ready",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hpa", Namespace: "test-ns"},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{Type: autoscalingv2.AbleToScale, Status: corev1.ConditionTrue},
						{Type: autoscalingv2.ScalingActive, Status: corev1.ConditionTrue},
					},
				},
			},
			wantReady: true,
		},
		{
			name: "AbleToScale=False -> not ready with reason",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hpa", Namespace: "test-ns"},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{Type: autoscalingv2.AbleToScale, Status: corev1.ConditionFalse, Reason: "FailedGetScale", Message: "cannot get scale"},
						{Type: autoscalingv2.ScalingActive, Status: corev1.ConditionTrue},
					},
				},
			},
			wantNotReady: true,
			wantReason:   "FailedGetScale",
		},
		{
			name: "ScalingActive=False -> not ready with reason",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hpa", Namespace: "test-ns"},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{Type: autoscalingv2.AbleToScale, Status: corev1.ConditionTrue},
						{Type: autoscalingv2.ScalingActive, Status: corev1.ConditionFalse, Reason: "FailedGetExternalMetric", Message: "metric not found"},
					},
				},
			},
			wantNotReady: true,
			wantReason:   "FailedGetExternalMetric",
		},
		{
			name: "no conditions yet -> not ready HPAProgressing",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hpa", Namespace: "test-ns"},
			},
			wantNotReady: true,
			wantReason:   "HPAProgressing",
		},
		{
			name: "ScalingLimited=True with healthy conditions -> ready",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{Name: "test-hpa", Namespace: "test-ns"},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{Type: autoscalingv2.AbleToScale, Status: corev1.ConditionTrue},
						{Type: autoscalingv2.ScalingActive, Status: corev1.ConditionTrue},
						{Type: autoscalingv2.ScalingLimited, Status: corev1.ConditionTrue, Message: "at max replicas"},
					},
				},
			},
			wantReady: true,
		},
		{
			name:         "HPA not found -> not ready HPAProgressing (cache lag)",
			hpa:          nil,
			wantNotReady: true,
			wantReason:   "HPAProgressing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconcilerWithHPA(tt.hpa)
			var readyCalled, notReadyCalled bool
			var notReadyReason string

			err := r.propagateHPAStatus(context.Background(), expectedHPAObj,
				func() { readyCalled = true },
				func(reason, msg string, a ...interface{}) {
					notReadyCalled = true
					notReadyReason = reason
				},
			)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantReady, readyCalled, "ready callback")
			assert.Equal(t, tt.wantNotReady, notReadyCalled, "notReady callback")
			if tt.wantReason != "" {
				assert.Equal(t, tt.wantReason, notReadyReason)
			}
		})
	}
}

func TestPropagateScaledObjectStatus(t *testing.T) {
	expectedSO := &kedav1alpha1.ScaledObject{
		ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
	}

	tests := []struct {
		name         string
		so           *kedav1alpha1.ScaledObject
		wantReady    bool
		wantNotReady bool
		wantReason   string
		wantErr      bool
	}{
		{
			name: "Ready=True -> ready",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionFalse},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionFalse},
					},
				},
			},
			wantReady: true,
		},
		{
			name: "Ready=False -> not ready with reason",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionFalse, Reason: "TriggerError", Message: "prometheus query failed"},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionFalse},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionFalse},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionFalse},
					},
				},
			},
			wantNotReady: true,
			wantReason:   "TriggerError",
		},
		{
			name: "Ready=Unknown -> not ready ScaledObjectProgressing",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionUnknown},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionUnknown},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionUnknown},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionUnknown},
					},
				},
			},
			wantNotReady: true,
			wantReason:   "ScaledObjectProgressing",
		},
		{
			name: "Paused=True with Ready=True -> ready (pause not surfaced yet)",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionFalse},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionTrue, Reason: "ScaledObjectPaused", Message: "ScaledObject is paused"},
					},
				},
			},
			wantReady: true,
		},
		{
			name: "Fallback=True with Ready=True -> ready (soft warning)",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
				Status: kedav1alpha1.ScaledObjectStatus{
					Conditions: kedav1alpha1.Conditions{
						{Type: kedav1alpha1.ConditionReady, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionActive, Status: metav1.ConditionTrue},
						{Type: kedav1alpha1.ConditionFallback, Status: metav1.ConditionTrue, Message: "using fallback replicas"},
						{Type: kedav1alpha1.ConditionPaused, Status: metav1.ConditionFalse},
					},
				},
			},
			wantReady: true,
		},
		{
			name: "no conditions (nil) -> not ready ScaledObjectProgressing",
			so: &kedav1alpha1.ScaledObject{
				ObjectMeta: metav1.ObjectMeta{Name: "test-so", Namespace: "test-ns"},
			},
			wantNotReady: true,
			wantReason:   "ScaledObjectProgressing",
		},
		{
			name:         "ScaledObject not found -> not ready ScaledObjectProgressing (cache lag)",
			so:           nil,
			wantNotReady: true,
			wantReason:   "ScaledObjectProgressing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReconcilerWithScaledObject(tt.so)
			var readyCalled, notReadyCalled bool
			var notReadyReason string

			err := r.propagateScaledObjectStatus(context.Background(), expectedSO,
				func() { readyCalled = true },
				func(reason, msg string, a ...interface{}) {
					notReadyCalled = true
					notReadyReason = reason
				},
			)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantReady, readyCalled, "ready callback")
			assert.Equal(t, tt.wantNotReady, notReadyCalled, "notReady callback")
			if tt.wantReason != "" {
				assert.Equal(t, tt.wantReason, notReadyReason)
			}
		})
	}
}
